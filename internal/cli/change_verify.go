package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const changeVerifyReceiptFile = "receipts/verify.json"

var (
	changeCriterionHeaderRE  = regexp.MustCompile(`(?m)^-\s+\*\*(V\d+)\.\*\*\s+(.*)$`)
	changeCriterionCommandRE = regexp.MustCompile(`(?mi)^\s*-\s*Command:\s*` + "`" + `([^` + "`" + `]+)` + "`")
	changeCriterionExpectRE  = regexp.MustCompile(`(?mi)^\s*-\s*Expect:\s*(.+)$`)
)

type changeCriterion struct {
	ID      string `json:"id"`
	Text    string `json:"text"`
	Command string `json:"command"`
	Expect  string `json:"expect"`
}

type changeVerifyReceipt struct {
	SchemaVersion  int                           `json:"schema_version"`
	Change         string                        `json:"change"`
	VerifiedCommit string                        `json:"verified_commit"`
	VerifiedAt     string                        `json:"verified_at"`
	CriteriaDigest string                        `json:"criteria_digest"`
	TargetRelease  string                        `json:"target_release,omitempty"`
	Results        []changeVerifyCriterionResult `json:"results"`
}

type changeVerifyCriterionResult struct {
	ID           string `json:"id"`
	Command      string `json:"command"`
	ExitCode     int    `json:"exit_code"`
	OutputDigest string `json:"output_digest"`
	OK           bool   `json:"ok"`
}

func (r Runner) runChangeVerify(args []string, out io.Writer, rootPath string) error {
	if isHelpArg(args) {
		writeChangeVerifyHelp(out)
		return nil
	}
	path := ""
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") {
			return fmt.Errorf("unknown change verify option %q", arg)
		}
		if path != "" {
			return fmt.Errorf("change verify accepts a single [folder] argument")
		}
		path = arg
	}
	folder, _, err := resolveChangeFolder(rootPath, path)
	if err != nil {
		return err
	}
	node, err := assembleChangeNodeFromFolder(rootPath, folder)
	if err != nil {
		return err
	}
	if node.Layout != changeLayoutNew {
		return fmt.Errorf("change verify is new-layout-only; convert %s first (sanctioned atomic replace)", relFromRoot(rootPath, folder))
	}
	if node.CapturedOnly || node.ContractFile == "" || strings.HasSuffix(node.ContractFile, "/"+changeBriefFile) {
		return fmt.Errorf("change verify requires shape.md with executable criteria")
	}
	criteria := parseChangeExecutableCriteria(node.Content)
	if len(criteria) == 0 {
		return fmt.Errorf("no executable criteria found in shape.md (need V-entries with Command: `...`)")
	}
	head, err := commandOutput(rootPath, "git", "rev-parse", "HEAD")
	if err != nil {
		return fmt.Errorf("resolve HEAD: %w", err)
	}
	head = strings.TrimSpace(head)
	results := make([]changeVerifyCriterionResult, 0, len(criteria))
	failed := false
	for _, criterion := range criteria {
		exitCode, output, runErr := runChangeCriterionCommand(folder, criterion.Command)
		digest := sha256HexBytes([]byte(output))
		ok := runErr == nil && exitCode == 0
		if !ok {
			failed = true
		}
		results = append(results, changeVerifyCriterionResult{
			ID:           criterion.ID,
			Command:      criterion.Command,
			ExitCode:     exitCode,
			OutputDigest: digest,
			OK:           ok,
		})
		status := ansiGreen("ok")
		if !ok {
			status = ansiRed("fail")
		}
		fmt.Fprintf(out, "%s %s  %s\n", status, criterion.ID, criterion.Command)
	}
	receipt := changeVerifyReceipt{
		SchemaVersion:  1,
		Change:         node.Slug,
		VerifiedCommit: head,
		VerifiedAt:     time.Now().UTC().Format(time.RFC3339),
		CriteriaDigest: changeCriteriaDigest(criteria),
		TargetRelease:  node.TargetRelease,
		Results:        results,
	}
	receiptPath := filepath.Join(folder, filepath.FromSlash(changeVerifyReceiptFile))
	if err := os.MkdirAll(filepath.Dir(receiptPath), 0o755); err != nil {
		return fmt.Errorf("create receipts/: %w", err)
	}
	data, err := json.MarshalIndent(receipt, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.WriteFile(receiptPath, data, 0o644); err != nil {
		return fmt.Errorf("write receipt: %w", err)
	}
	fmt.Fprintf(out, "\nWrote receipt: %s\n", relFromRoot(rootPath, receiptPath))
	fmt.Fprintf(out, "criteria_digest: %s\n", receipt.CriteriaDigest)
	fmt.Fprintf(out, "verified_commit: %s\n", shortSHA(receipt.VerifiedCommit))
	if failed {
		return ExitError{Code: 1}
	}
	return nil
}

func writeChangeVerifyHelp(out io.Writer) {
	writeUsageHelp(out, "loaf change verify [folder]",
		"Run executable criteria declared in shape.md and write receipts/verify.json (criteria digest, verified commit, per-criterion evidence). New-layout-only.",
		"[folder]  Change folder path; resolves from the current branch when omitted")
}

func parseChangeExecutableCriteria(shape string) []changeCriterion {
	sections := changeSections(shape)
	body, ok := sections["Verification Contract"]
	if !ok {
		return nil
	}
	lines := strings.Split(body, "\n")
	var criteria []changeCriterion
	var current *changeCriterion
	flush := func() {
		if current == nil {
			return
		}
		if current.Command != "" {
			criteria = append(criteria, *current)
		}
		current = nil
	}
	for _, line := range lines {
		if match := changeCriterionHeaderRE.FindStringSubmatch(line); match != nil {
			flush()
			current = &changeCriterion{ID: match[1], Text: strings.TrimSpace(match[2])}
			// Inline Command: `...` on the same header line
			if cmd := changeCriterionCommandRE.FindStringSubmatch(line); cmd != nil {
				current.Command = cmd[1]
			}
			continue
		}
		if current == nil {
			continue
		}
		if cmd := changeCriterionCommandRE.FindStringSubmatch(line); cmd != nil {
			current.Command = cmd[1]
			continue
		}
		if exp := changeCriterionExpectRE.FindStringSubmatch(line); exp != nil {
			current.Expect = strings.TrimSpace(exp[1])
		}
	}
	flush()
	return criteria
}

func changeCriteriaDigest(criteria []changeCriterion) string {
	var b strings.Builder
	for _, c := range criteria {
		fmt.Fprintf(&b, "%s\n%s\n%s\n", c.ID, c.Command, c.Expect)
	}
	return sha256HexBytes([]byte(b.String()))
}

func runChangeCriterionCommand(folder, command string) (int, string, error) {
	cmd := exec.Command("bash", "-lc", command)
	cmd.Dir = folder
	output, err := cmd.CombinedOutput()
	if err == nil {
		return 0, string(output), nil
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		return exitErr.ExitCode(), string(output), nil
	}
	return 1, string(output), err
}

func sha256HexBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func loadChangeVerifyReceipt(folderAbs string) (changeVerifyReceipt, error) {
	data, err := os.ReadFile(filepath.Join(folderAbs, filepath.FromSlash(changeVerifyReceiptFile)))
	if err != nil {
		return changeVerifyReceipt{}, err
	}
	var receipt changeVerifyReceipt
	if err := json.Unmarshal(data, &receipt); err != nil {
		return changeVerifyReceipt{}, err
	}
	return receipt, nil
}

// changeReceiptFreshness reports whether a receipt still covers HEAD for gate
// purposes. The receipt's own commit never stales it; any later commit that
// touches a non-receipt path forces a criteria re-run (caller decides).
func changeReceiptStatus(rootPath, folderRel string, node changeNode, outputCommand changeGitOutput) (fresh bool, reason string, err error) {
	if outputCommand == nil {
		outputCommand = commandOutput
	}
	folderAbs := filepath.Join(rootPath, filepath.FromSlash(folderRel))
	receipt, err := loadChangeVerifyReceipt(folderAbs)
	if err != nil {
		if os.IsNotExist(err) {
			return false, "missing receipt", nil
		}
		return false, "", err
	}
	criteria := parseChangeExecutableCriteria(node.Content)
	digest := changeCriteriaDigest(criteria)
	if digest != receipt.CriteriaDigest {
		return false, "criteria digest mismatch (receipt expired)", nil
	}
	head, err := outputCommand(rootPath, "git", "rev-parse", "HEAD")
	if err != nil {
		return false, "", err
	}
	head = strings.TrimSpace(head)
	if head == receipt.VerifiedCommit {
		return true, "", nil
	}
	// Commits after verified commit:
	logOut, err := outputCommand(rootPath, "git", "log", "--format=%H", receipt.VerifiedCommit+"..HEAD", "--", folderRel)
	if err != nil {
		return false, "", err
	}
	receiptRel := filepath.ToSlash(filepath.Join(folderRel, changeVerifyReceiptFile))
	for _, commit := range strings.Split(strings.TrimSpace(logOut), "\n") {
		commit = strings.TrimSpace(commit)
		if commit == "" {
			continue
		}
		pathsOut, err := outputCommand(rootPath, "git", "diff-tree", "--no-commit-id", "--name-only", "-r", commit)
		if err != nil {
			return false, "", err
		}
		for _, p := range strings.Split(pathsOut, "\n") {
			p = filepath.ToSlash(strings.TrimSpace(p))
			if p == "" || p == receiptRel {
				continue
			}
			return false, "later non-receipt path requires criteria re-run", nil
		}
	}
	// Also check repo-wide commits after verified that touch anything outside
	// the receipt — Decision 13: any later non-receipt path.
	logAll, err := outputCommand(rootPath, "git", "log", "--format=%H", receipt.VerifiedCommit+"..HEAD", "--name-only", "--pretty=format:%H")
	if err != nil {
		return false, "", err
	}
	// Simpler approach: list all changed paths since verified commit.
	diffOut, err := outputCommand(rootPath, "git", "diff", "--name-only", receipt.VerifiedCommit, "HEAD")
	if err != nil {
		return false, "", err
	}
	for _, p := range strings.Split(diffOut, "\n") {
		p = filepath.ToSlash(strings.TrimSpace(p))
		if p == "" || p == receiptRel {
			continue
		}
		return false, "later non-receipt path requires criteria re-run", nil
	}
	_ = logAll
	return true, "", nil
}
