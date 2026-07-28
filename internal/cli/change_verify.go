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
	"strconv"
	"strings"
	"time"
)

const changeVerifyReceiptFile = "receipts/verify.json"

var (
	// Header accepts both `- **V1.** …` and the scaffold's checkbox form `- [**V1.** …]`.
	changeCriterionHeaderRE = regexp.MustCompile(`(?m)^-\s+(\[)?\*\*(V\d+)\.\*\*\s+(.*)$`)
	// Command/Expect match inline on the V-entry line or as a sub-bullet (`- Command: \`…\``).
	changeCriterionCommandRE = regexp.MustCompile(`(?i)Command:\s*` + "`" + `([^` + "`" + `]+)` + "`")
	changeCriterionExpectRE  = regexp.MustCompile(`(?i)Expect:\s*(.+)$`)
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
	Cwd            string                        `json:"cwd"`
	TargetRelease  string                        `json:"target_release,omitempty"`
	Results        []changeVerifyCriterionResult `json:"results"`
}

// changeVerifyCriterionResult records one criterion's evidence. Expect fields are
// additive on schema_version 1: older readers ignore them, and no receipt exists
// outside fixtures.
type changeVerifyCriterionResult struct {
	ID           string                    `json:"id"`
	Command      string                    `json:"command"`
	ExitCode     int                       `json:"exit_code"`
	OutputDigest string                    `json:"output_digest"`
	OK           bool                      `json:"ok"`
	Expect       string                    `json:"expect,omitempty"`
	ExpectChecks []changeVerifyExpectCheck `json:"expect_checks,omitempty"`
	Advisory     []string                  `json:"advisory_clauses,omitempty"`
}

// changeVerifyExpectCheck is one enforced Expect atom and its outcome.
type changeVerifyExpectCheck struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
	OK    bool   `json:"ok"`
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
		exitCode, output, runErr := runChangeCriterionCommand(rootPath, criterion.Command)
		digest := sha256HexBytes([]byte(output))
		expectation := parseChangeExpectation(criterion.Expect)
		checks := evaluateChangeExpectation(expectation, exitCode, output)
		ok := runErr == nil && changeExpectChecksPass(checks)
		if !ok {
			failed = true
		}
		results = append(results, changeVerifyCriterionResult{
			ID:           criterion.ID,
			Command:      criterion.Command,
			ExitCode:     exitCode,
			OutputDigest: digest,
			OK:           ok,
			Expect:       criterion.Expect,
			ExpectChecks: checks,
			Advisory:     expectation.Advisory,
		})
		status := ansiGreen("ok")
		if !ok {
			status = ansiRed("fail")
		}
		fmt.Fprintf(out, "%s %s  %s%s\n", status, criterion.ID, criterion.Command, changeExpectFailureNote(runErr, exitCode, checks))
		// Nothing silent in either direction: a clause the grammar cannot enforce
		// is named here and recorded as advisory, and never touches ok.
		for _, clause := range expectation.Advisory {
			fmt.Fprintf(out, "%s %s  unenforceable Expect clause %q — recorded as advisory, never checked\n",
				ansiYellow("warn"), criterion.ID, clause)
		}
	}
	receipt := changeVerifyReceipt{
		SchemaVersion:  1,
		Change:         node.Slug,
		VerifiedCommit: head,
		VerifiedAt:     time.Now().UTC().Format(time.RFC3339),
		CriteriaDigest: changeCriteriaDigest(criteria),
		Cwd:            rootPath,
		TargetRelease:  node.TargetRelease,
		Results:        results,
	}
	// Write-on-failure: persist evidence even when criteria fail; the cohort
	// gate rejects receipts with any results[].ok == false (TASK-007).
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
			boxed := match[1] == "["
			text := strings.TrimSpace(match[3])
			if boxed {
				text = strings.TrimSuffix(text, "]")
				text = strings.TrimSpace(text)
			}
			current = &changeCriterion{ID: match[2], Text: text}
			// Inline Command:/Expect: on the same header line (scaffold form).
			if cmd := changeCriterionCommandRE.FindStringSubmatch(line); cmd != nil {
				current.Command = cmd[1]
			}
			if exp := changeCriterionExpectRE.FindStringSubmatch(line); exp != nil {
				expect := strings.TrimSpace(exp[1])
				if boxed {
					expect = strings.TrimSuffix(expect, "]")
					expect = strings.TrimSpace(expect)
				}
				current.Expect = expect
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

// changeExpectation is a parsed Expect declaration. The grammar is deliberately
// minimal: atoms joined by " and ", either `exit <N>` (required exit code) or
// “ contains `text` “ (combined stdout+stderr contains the text, repeatable).
// An absent Expect — or an Expect with no exit atom — means exit 0, which is
// exactly what verify enforced before the grammar existed. Every other clause is
// unenforceable: it lands in Advisory, is warned about, and never affects ok.
type changeExpectation struct {
	ExitCode int
	Contains []string
	Advisory []string
}

func parseChangeExpectation(expect string) changeExpectation {
	parsed := changeExpectation{}
	for _, clause := range splitChangeExpectClauses(expect) {
		kind, value, enforceable := parseChangeExpectClause(clause)
		switch {
		case kind == "":
			continue // empty clause
		case !enforceable:
			parsed.Advisory = append(parsed.Advisory, value)
		case kind == "exit":
			parsed.ExitCode, _ = strconv.Atoi(value)
		case kind == "contains":
			parsed.Contains = append(parsed.Contains, value)
		}
	}
	return parsed
}

// splitChangeExpectClauses splits on " and " outside backticks, so a
// “ contains `a and b` “ literal survives intact.
func splitChangeExpectClauses(expect string) []string {
	lower := strings.ToLower(expect)
	inTick := false
	start := 0
	var clauses []string
	for i := 0; i < len(expect); i++ {
		if expect[i] == '`' {
			inTick = !inTick
			continue
		}
		if inTick {
			continue
		}
		if strings.HasPrefix(lower[i:], " and ") {
			clauses = append(clauses, expect[start:i])
			i += len(" and ") - 1
			start = i + 1
		}
	}
	return append(clauses, expect[start:])
}

// parseChangeExpectClause classifies one clause. kind is "" for an empty clause;
// enforceable is false for anything outside the grammar, and value then carries
// the clause verbatim for the warning and the advisory record.
func parseChangeExpectClause(clause string) (kind string, value string, enforceable bool) {
	trimmed := strings.TrimSpace(clause)
	// Authors end sentences; punctuation outside backticks is not part of an atom.
	trimmed = strings.TrimSpace(strings.TrimRight(trimmed, ".,;"))
	if trimmed == "" {
		return "", "", false
	}
	lower := strings.ToLower(trimmed)
	if strings.HasPrefix(lower, "exit ") {
		code := strings.TrimSpace(trimmed[len("exit "):])
		if n, err := strconv.Atoi(code); err == nil && n >= 0 {
			return "exit", code, true
		}
		return "exit", trimmed, false
	}
	if strings.HasPrefix(lower, "contains ") {
		rest := strings.TrimSpace(trimmed[len("contains "):])
		if len(rest) >= 3 && rest[0] == '`' && rest[len(rest)-1] == '`' {
			if text := rest[1 : len(rest)-1]; !strings.Contains(text, "`") {
				return "contains", text, true
			}
		}
		return "contains", trimmed, false
	}
	return "clause", trimmed, false
}

// evaluateChangeExpectation records the enforced atoms and their outcomes. The
// exit atom is always recorded, so the receipt states what was enforced even when
// the criterion declared no Expect at all.
func evaluateChangeExpectation(expectation changeExpectation, exitCode int, output string) []changeVerifyExpectCheck {
	checks := []changeVerifyExpectCheck{{
		Kind:  "exit",
		Value: fmt.Sprintf("%d", expectation.ExitCode),
		OK:    exitCode == expectation.ExitCode,
	}}
	for _, text := range expectation.Contains {
		checks = append(checks, changeVerifyExpectCheck{
			Kind:  "contains",
			Value: text,
			OK:    strings.Contains(output, text),
		})
	}
	return checks
}

func changeExpectChecksPass(checks []changeVerifyExpectCheck) bool {
	for _, check := range checks {
		if !check.OK {
			return false
		}
	}
	return true
}

// changeExpectFailureNote names the first unmet atom so a failure reads without
// opening the receipt.
func changeExpectFailureNote(runErr error, exitCode int, checks []changeVerifyExpectCheck) string {
	if runErr != nil {
		return fmt.Sprintf("  (command did not run: %v)", runErr)
	}
	for _, check := range checks {
		if check.OK {
			continue
		}
		if check.Kind == "contains" {
			return fmt.Sprintf("  (output missing: contains `%s`)", check.Value)
		}
		return fmt.Sprintf("  (want exit %s, got %d)", check.Value, exitCode)
	}
	return ""
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

// loadChangeVerifyReceipt reads the receipt from the working tree. This is
// verify's own surface — it writes that file — and never the gate's: gate-context
// reads go through changeReceiptAtHEAD so evidence is always committed before it
// is read (ADR-023).
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

func changeReceiptRelPath(folderRel string) string {
	return filepath.ToSlash(filepath.Join(folderRel, changeVerifyReceiptFile))
}

// changeReceiptAtHEAD loads the receipt as committed at HEAD. found=false means
// the HEAD tree carries no receipt; the working tree is never consulted for
// content, so a receipt that exists on one machine only cannot satisfy the gate.
func changeReceiptAtHEAD(rootPath, folderRel string, outputCommand changeGitOutput) (changeVerifyReceipt, bool, error) {
	if outputCommand == nil {
		outputCommand = commandOutput
	}
	receiptRel := changeReceiptRelPath(folderRel)
	content, found, err := readCommittedOptional(rootPath, "HEAD", receiptRel, outputCommand)
	if err != nil {
		return changeVerifyReceipt{}, false, err
	}
	if !found {
		return changeVerifyReceipt{}, false, nil
	}
	var receipt changeVerifyReceipt
	if err := json.Unmarshal([]byte(content), &receipt); err != nil {
		return changeVerifyReceipt{}, false, fmt.Errorf("parse committed receipt %s: %w", receiptRel, err)
	}
	return receipt, true, nil
}

func changeReceiptExistsInWorkingTree(rootPath, folderRel string) bool {
	folderAbs := filepath.Join(rootPath, filepath.FromSlash(folderRel))
	_, err := os.Stat(filepath.Join(folderAbs, filepath.FromSlash(changeVerifyReceiptFile)))
	return err == nil
}

// changeReceiptStatus reports whether a receipt attests successful verification
// that still covers HEAD. The receipt is read from committed HEAD, never from the
// working tree: an uncommitted receipt is evidence on one machine only and blocks
// with its own reason. Failing criteria block even when the receipt is fresh; the
// receipt's own commit never stales it; any later commit that touches a
// non-receipt path stales the receipt with a re-verify demand (Decision 13).
// Preflight never executes criteria — only loaf change verify does.
func changeReceiptStatus(rootPath, folderRel string, node changeNode, outputCommand changeGitOutput) (ok bool, reason string, err error) {
	if outputCommand == nil {
		outputCommand = commandOutput
	}
	receipt, found, err := changeReceiptAtHEAD(rootPath, folderRel, outputCommand)
	if err != nil {
		return false, "", err
	}
	if !found {
		if changeReceiptExistsInWorkingTree(rootPath, folderRel) {
			return false, "receipt not committed at HEAD", nil
		}
		return false, "missing receipt", nil
	}
	criteria := parseChangeExecutableCriteria(node.Content)
	digest := changeCriteriaDigest(criteria)
	if digest != receipt.CriteriaDigest {
		return false, "criteria digest mismatch (receipt expired)", nil
	}
	if failed := receiptFailingCriterionIDs(receipt); len(failed) > 0 {
		return false, fmt.Sprintf("receipt records failing criteria (%s)", strings.Join(failed, ", ")), nil
	}
	head, err := outputCommand(rootPath, "git", "rev-parse", "HEAD")
	if err != nil {
		return false, "", err
	}
	head = strings.TrimSpace(head)
	if head == receipt.VerifiedCommit {
		return true, "", nil
	}
	// Commit-by-commit: a touch-then-revert pair still stales, unlike a
	// verified..HEAD tree diff that would cancel out.
	logOut, err := outputCommand(rootPath, "git", "log", "--format=%H", receipt.VerifiedCommit+"..HEAD")
	if err != nil {
		return false, "", err
	}
	receiptRel := changeReceiptRelPath(folderRel)
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
	return true, "", nil
}

func receiptFailingCriterionIDs(receipt changeVerifyReceipt) []string {
	var failed []string
	for _, result := range receipt.Results {
		if !result.OK {
			failed = append(failed, result.ID)
		}
	}
	return failed
}
