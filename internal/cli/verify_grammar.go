package cli

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// changeExpectation is a parsed Expect declaration. The grammar is deliberately
// minimal: atoms joined by " and ", either `exit <N>` (required exit code) or
// “ contains `text` “ (combined stdout+stderr contains the text, repeatable).
// An absent Expect — or an Expect with no exit atom — means exit 0, which is
// exactly what verify enforced before the grammar existed. Every other clause is
// unenforceable: it lands in Advisory, is warned about, and never affects ok.
//
// This file is the shared home owned by the issue surface.
type changeExpectation struct {
	ExitCode     int
	exitSeen     bool
	ExitConflict string // non-empty when a second exit atom contradicts the first
	Contains     []string
	Advisory     []string
}

// changeVerifyExpectCheck is one enforced Expect atom and its outcome.
type changeVerifyExpectCheck struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
	OK    bool   `json:"ok"`
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
