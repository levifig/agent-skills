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
	"runtime"
	"slices"
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
	SchemaVersion int    `json:"schema_version"`
	Change        string `json:"change"`
	// VerifiedCommit is provenance only — never consulted for the freshness verdict (ADR-024).
	VerifiedCommit   string                        `json:"verified_commit"`
	VerifiedRootTree string                        `json:"verified_root_tree"`
	VerifiedAt       string                        `json:"verified_at"`
	CriteriaDigest   string                        `json:"criteria_digest"`
	ScopeDigest      string                        `json:"scope_digest"`
	ScopeSections    map[string]string             `json:"scope_sections"`
	Exclusions       []string                      `json:"exclusions"`
	DigestSpec       string                        `json:"digest_spec"`
	ToolVersion      string                        `json:"tool_version"`
	Toolchain        changeVerifyToolchain         `json:"toolchain"`
	WorktreeClean    bool                          `json:"worktree_clean"`
	TargetRelease    string                        `json:"target_release,omitempty"`
	Results          []changeVerifyCriterionResult `json:"results"`
}

// changeVerifyToolchain records the verify host environment for audit, never gating.
type changeVerifyToolchain struct {
	Go   string `json:"go"`
	OS   string `json:"os"`
	Arch string `json:"arch"`
}

// changeVerifyCriterionResult records one criterion's evidence.
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
	dirtyPaths, err := changeTrackedWorktreeDivergedPaths(rootPath)
	if err != nil {
		return fmt.Errorf("inspect worktree: %w", err)
	}
	if len(dirtyPaths) > 0 {
		return fmt.Errorf("working tree differs from HEAD; commit before verifying")
	}
	head, err := commandOutput(rootPath, "git", "rev-parse", "HEAD")
	if err != nil {
		return fmt.Errorf("resolve HEAD: %w", err)
	}
	head = strings.TrimSpace(head)
	rootTree, err := commandOutput(rootPath, "git", "rev-parse", "HEAD^{tree}")
	if err != nil {
		return fmt.Errorf("resolve HEAD tree: %w", err)
	}
	rootTree = strings.TrimSpace(rootTree)
	exclusions := ChangeEvidenceExclusions()
	scope, err := scopeDigest(rootPath, head, exclusions, nil)
	if err != nil {
		return fmt.Errorf("compute scope digest: %w", err)
	}
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
	// Post-run dirty check: criteria may mutate tracked files. Receipt/report
	// masks are exempt (same as pre-run); allowlist paths are not.
	postDirty, err := changeTrackedWorktreeDivergedPaths(rootPath)
	if err != nil {
		return fmt.Errorf("inspect worktree after criteria: %w", err)
	}
	worktreeClean := len(postDirty) == 0
	receipt := changeVerifyReceipt{
		SchemaVersion:    2,
		Change:           node.Slug,
		VerifiedCommit:   head,
		VerifiedRootTree: rootTree,
		VerifiedAt:       time.Now().UTC().Format(time.RFC3339),
		CriteriaDigest:   changeCriteriaDigest(criteria),
		ScopeDigest:      scope.Digest,
		ScopeSections:    scope.Sections,
		Exclusions:       exclusions,
		DigestSpec:       ChangeEvidenceDigestSpec,
		ToolVersion:      packageVersion(rootPath),
		Toolchain: changeVerifyToolchain{
			Go:   strings.TrimPrefix(runtime.Version(), "go"),
			OS:   runtime.GOOS,
			Arch: runtime.GOARCH,
		},
		WorktreeClean: worktreeClean,
		TargetRelease: node.TargetRelease,
		Results:       results,
	}
	// Write-on-failure: persist evidence even when criteria fail or the
	// worktree diverged mid-run; the cohort gate rejects both.
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
	fmt.Fprintf(out, "scope_digest: %s\n", receipt.ScopeDigest)
	fmt.Fprintf(out, "verified_commit: %s (provenance)\n", shortSHA(receipt.VerifiedCommit))
	if !worktreeClean {
		return fmt.Errorf("criteria mutated the tracked worktree (%s); receipt is void — restore or commit, then re-verify", strings.Join(postDirty, ", "))
	}
	if failed {
		return ExitError{Code: 1}
	}
	return nil
}

func writeChangeVerifyHelp(out io.Writer) {
	writeUsageHelp(out, "loaf change verify [folder]",
		"Run executable criteria declared in shape.md and write receipts/verify.json (schema v2 content digest, criteria digest, per-criterion evidence). New-layout-only. Refuses a dirty tracked worktree.",
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
	ExitCode     int
	exitSeen     bool
	ExitConflict string // non-empty when a second exit atom contradicts the first
	Contains     []string
	Advisory     []string
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
			code, _ := strconv.Atoi(value)
			if parsed.exitSeen {
				if parsed.ExitConflict == "" {
					parsed.ExitConflict = fmt.Sprintf("%d and %d", parsed.ExitCode, code)
				} else {
					parsed.ExitConflict += fmt.Sprintf(" and %d", code)
				}
			} else {
				parsed.ExitCode = code
				parsed.exitSeen = true
			}
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
// the criterion declared no Expect at all. An exit conflict is recorded alongside
// every declared contains atom — never instead of them — and keeps the criterion
// false regardless of the other atoms' outcomes.
func evaluateChangeExpectation(expectation changeExpectation, exitCode int, output string) []changeVerifyExpectCheck {
	var checks []changeVerifyExpectCheck
	if expectation.ExitConflict != "" {
		checks = append(checks, changeVerifyExpectCheck{
			Kind:  "exit-conflict",
			Value: expectation.ExitConflict,
			OK:    false,
		})
	} else {
		checks = append(checks, changeVerifyExpectCheck{
			Kind:  "exit",
			Value: fmt.Sprintf("%d", expectation.ExitCode),
			OK:    exitCode == expectation.ExitCode,
		})
	}
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
		if check.Kind == "exit-conflict" {
			return fmt.Sprintf("  (contradictory exit atoms: %s)", check.Value)
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
		fmt.Fprintf(&b, "%s\n%s\n%s\n%s\n", c.ID, c.Text, c.Command, c.Expect)
	}
	return sha256HexBytes([]byte(b.String()))
}

func runChangeCriterionCommand(rootPath, command string) (int, string, error) {
	cmd := exec.Command("bash", "-c", command)
	cmd.Dir = rootPath
	output, err := cmd.CombinedOutput()
	if err == nil {
		return 0, string(output), nil
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		return exitErr.ExitCode(), string(output), nil
	}
	return 1, string(output), err
}

// changeTrackedWorktreeDirty reports whether tracked or staged files differ from
// HEAD after exempting receipt and report masks. Untracked files do not count —
// verify may write the receipt into an untracked receipts/ path. The
// release-metadata allowlist is NOT exempt: digest-excluded paths like dist/**
// stay dirty-checked because criteria may mutate them.
func changeTrackedWorktreeDirty(rootPath string) (bool, error) {
	paths, err := changeTrackedWorktreeDivergedPaths(rootPath)
	if err != nil {
		return false, err
	}
	return len(paths) > 0, nil
}

// changeTrackedWorktreeDivergedPaths lists tracked/staged paths that differ from
// HEAD and are not receipt/report-mask exempt. Paths are slash-normalized and sorted.
func changeTrackedWorktreeDivergedPaths(rootPath string) ([]string, error) {
	out, err := commandOutput(rootPath, "git", "status", "--porcelain=v1", "-uno")
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var paths []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}
		for _, path := range porcelainTrackedPaths(line) {
			if path == "" || changeDirtyCheckExempt(path) || seen[path] {
				continue
			}
			seen[path] = true
			paths = append(paths, path)
		}
	}
	slices.Sort(paths)
	return paths, nil
}

// changeDirtyCheckExempt is true only for receipt and report mask paths — never
// the release-metadata allowlist.
func changeDirtyCheckExempt(path string) bool {
	for _, pattern := range ChangeEvidenceReceiptMasks {
		if matchEvidenceGlob(path, pattern) {
			return true
		}
	}
	for _, pattern := range ChangeEvidenceReportMasks {
		if matchEvidenceGlob(path, pattern) {
			return true
		}
	}
	return false
}

// porcelainTrackedPaths extracts path(s) from one git status --porcelain=v1 line.
// Rename/copy records are `XY old -> new`; both sides are returned.
func porcelainTrackedPaths(line string) []string {
	if len(line) < 4 || line[2] != ' ' {
		return nil
	}
	rest := line[3:]
	if idx := strings.Index(rest, " -> "); idx >= 0 {
		return []string{unquotePorcelainPath(rest[:idx]), unquotePorcelainPath(rest[idx+4:])}
	}
	return []string{unquotePorcelainPath(rest)}
}

func unquotePorcelainPath(path string) string {
	path = strings.TrimSpace(path)
	if len(path) >= 2 && path[0] == '"' {
		if unquoted, err := strconv.Unquote(path); err == nil {
			return filepath.ToSlash(unquoted)
		}
	}
	return filepath.ToSlash(path)
}

func sha256HexBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// loadChangeVerifyReceipt reads the receipt from the working tree. This is
// verify's own surface — it writes that file — and never the gate's: gate-context
// reads go through changeReceiptStatus, which reads the committed receipt at HEAD
// via readCommittedOptional (ADR-023 / ADR-024).
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

func receiptFailingCriterionIDs(receipt changeVerifyReceipt) []string {
	var failed []string
	for _, result := range receipt.Results {
		if !result.OK {
			failed = append(failed, result.ID)
		}
	}
	return failed
}
