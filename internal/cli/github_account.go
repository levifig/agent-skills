package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

const githubAccountHostname = "github.com"

var githubCommandRE = regexp.MustCompile(`(^|[;&|]\s*|&&\s*|\|\|\s*|\()\s*((env|command|sudo)\s+|\S+=\S+\s+)*gh(\s|$)`)

var errNoActiveGitHubAccount = errors.New("no active GitHub account found")

type githubAccountCommandResult struct {
	stdout   string
	stderr   string
	exitCode int
	notFound bool
}

// githubAccountState classifies the relationship between the configured account
// and the active gh account, so callers branch on intent rather than parsing a
// diagnostic string.
type githubAccountState int

const (
	// githubAccountMatch: the active account already matches the configured one.
	githubAccountMatch githubAccountState = iota
	// githubAccountMismatch: gh is available and reachable but the active account
	// differs (or none is active). This is switchable — `gh auth switch` can
	// converge the environment.
	githubAccountMismatch
	// githubAccountUnavailable: gh is missing or its status probe hard-failed, so
	// the account can neither be determined nor remedied by switching.
	githubAccountUnavailable
)

type githubAccountCheck struct {
	state      githubAccountState
	actual     string // active account login; "" when none is active
	diagnostic string // human-readable reason, populated for the unavailable state
}

// githubAccountOutcome is the result of trying to converge the environment on the
// configured account.
type githubAccountOutcome int

const (
	githubAccountOutcomeMatch        githubAccountOutcome = iota // already matched; no switch performed
	githubAccountOutcomeSwitched                                 // switched and converged on the configured account
	githubAccountOutcomeUnavailable                              // gh missing or status probe hard-failed
	githubAccountOutcomeSwitchFailed                             // switch attempted but did not converge
)

type githubAccountResolution struct {
	outcome  githubAccountOutcome
	expected string
	previous string // active account before switching; "" when none
	reason   string // populated for unavailable / switch-failed outcomes
}

func configuredGitHubAccount(root string) (string, error) {
	body, err := os.ReadFile(filepath.Join(root, ".agents", "loaf.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("could not read .agents/loaf.json: %w", err)
	}

	var config struct {
		Integrations struct {
			GitHub struct {
				Account  string `json:"account"`
				Login    string `json:"login"`
				Username string `json:"username"`
			} `json:"github"`
		} `json:"integrations"`
	}
	if err := json.Unmarshal(body, &config); err != nil {
		return "", fmt.Errorf("cannot parse .agents/loaf.json: %w", err)
	}
	return strings.TrimSpace(firstNonEmpty(
		config.Integrations.GitHub.Account,
		config.Integrations.GitHub.Login,
		config.Integrations.GitHub.Username,
	)), nil
}

func shellCommandUsesGitHubCLI(command string) bool {
	return githubCommandRE.MatchString(strings.TrimSpace(command))
}

// shellCommandOnlyManagesGitHubAuth reports whether every gh invocation in the
// command is a `gh auth` administrative subcommand (login, logout, switch,
// refresh, status, ...). Identity administration is the user's domain, so those
// invocations bypass account convergence: converging first would misdirect them
// — e.g. `gh auth logout` under a mismatch would first force-switch the global
// pointer to the configured account and then log THAT account out, not the one
// the user targeted. A compound that mixes auth with resource-touching gh usage
// (e.g. `gh auth switch --user x && gh pr create`) is not exempt and still
// converges. Because githubCommandRE only matches gh at a command position, a
// `gh auth` substring living inside an argument (e.g. `echo "gh auth"`) is not
// mistaken for an invocation.
func shellCommandOnlyManagesGitHubAuth(command string) bool {
	trimmed := strings.TrimSpace(command)
	positions := githubCommandRE.FindAllStringIndex(trimmed, -1)
	if len(positions) == 0 {
		return false
	}
	for _, pos := range positions {
		rest := strings.TrimLeft(trimmed[pos[1]:], " \t")
		subcommand := rest
		if i := strings.IndexAny(rest, " \t"); i >= 0 {
			subcommand = rest[:i]
		}
		if subcommand != "auth" {
			return false
		}
	}
	return true
}

func activeGitHubAccount(root string) githubAccountCommandResult {
	cmd := exec.Command("gh", "auth", "status", "--active", "--hostname", githubAccountHostname, "--json", "hosts")
	cmd.Dir = root
	output, err := cmd.Output()
	if err == nil {
		return githubAccountCommandResult{stdout: string(output), exitCode: 0}
	}
	if errors.Is(err, exec.ErrNotFound) {
		return githubAccountCommandResult{exitCode: 127, notFound: true}
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return githubAccountCommandResult{stdout: string(output), stderr: string(exitErr.Stderr), exitCode: exitErr.ExitCode()}
	}
	return githubAccountCommandResult{exitCode: 1}
}

// switchGitHubAccount runs `gh auth switch` non-interactively to make the
// configured account active. Its stderr is captured so a failed switch can
// surface gh's own diagnostic to the human.
func switchGitHubAccount(root string, user string) githubAccountCommandResult {
	cmd := exec.Command("gh", "auth", "switch", "--hostname", githubAccountHostname, "--user", user)
	cmd.Dir = root
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	result := githubAccountCommandResult{stdout: stdout.String(), stderr: stderr.String()}
	if err == nil {
		return result
	}
	if errors.Is(err, exec.ErrNotFound) {
		result.exitCode = 127
		result.notFound = true
		return result
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		result.exitCode = exitErr.ExitCode()
		return result
	}
	result.exitCode = 1
	return result
}

func activeGitHubAccountFromJSON(output string) (string, error) {
	var payload struct {
		Hosts map[string][]struct {
			Active bool   `json:"active"`
			Login  string `json:"login"`
			State  string `json:"state"`
		} `json:"hosts"`
	}
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		return "", fmt.Errorf("could not parse gh auth status JSON: %w", err)
	}
	accounts := payload.Hosts[githubAccountHostname]
	for _, account := range accounts {
		if account.Active && strings.TrimSpace(account.Login) != "" {
			return strings.TrimSpace(account.Login), nil
		}
	}
	if len(accounts) == 1 && strings.TrimSpace(accounts[0].Login) != "" {
		return strings.TrimSpace(accounts[0].Login), nil
	}
	return "", fmt.Errorf("%w for %s", errNoActiveGitHubAccount, githubAccountHostname)
}

// classifyGitHubAccount maps a gh status probe onto the configured account. It
// separates the switchable mismatch case (gh reachable, wrong or no active
// account) from the unavailable case (gh missing or probe hard-failed) so the
// caller can decide whether converging is even possible.
func classifyGitHubAccount(expected string, result githubAccountCommandResult) githubAccountCheck {
	if result.notFound {
		return githubAccountCheck{
			state:      githubAccountUnavailable,
			diagnostic: fmt.Sprintf("project requires GitHub account %q, but gh CLI is not installed", expected),
		}
	}
	if result.exitCode != 0 {
		return githubAccountCheck{
			state:      githubAccountUnavailable,
			diagnostic: fmt.Sprintf("project requires GitHub account %q, but `gh auth status --active --hostname %s --json hosts` failed", expected, githubAccountHostname),
		}
	}
	actual, err := activeGitHubAccountFromJSON(result.stdout)
	if err != nil {
		if errors.Is(err, errNoActiveGitHubAccount) {
			// gh is reachable but no account is active — `gh auth switch` can fix it.
			return githubAccountCheck{state: githubAccountMismatch}
		}
		// Unparseable status output: cannot determine or remedy the account.
		return githubAccountCheck{
			state:      githubAccountUnavailable,
			diagnostic: fmt.Sprintf("project requires GitHub account %q, but %s", expected, err),
		}
	}
	if strings.EqualFold(actual, expected) {
		return githubAccountCheck{state: githubAccountMatch, actual: actual}
	}
	return githubAccountCheck{state: githubAccountMismatch, actual: actual}
}

// resolveGitHubAccount converges the environment on the configured account: it
// checks the active account, switches when it differs, and re-checks to confirm
// the switch took. The guard's job is to make the environment match the project,
// not to dead-end on a mismatch.
func resolveGitHubAccount(
	root string,
	expected string,
	status func(string) githubAccountCommandResult,
	switcher func(string, string) githubAccountCommandResult,
) githubAccountResolution {
	check := classifyGitHubAccount(expected, status(root))
	switch check.state {
	case githubAccountMatch:
		return githubAccountResolution{outcome: githubAccountOutcomeMatch, expected: expected, previous: check.actual}
	case githubAccountUnavailable:
		return githubAccountResolution{outcome: githubAccountOutcomeUnavailable, expected: expected, reason: check.diagnostic}
	}

	previous := check.actual
	switchResult := switcher(root, expected)
	if switchResult.exitCode == 0 && !switchResult.notFound &&
		classifyGitHubAccount(expected, status(root)).state == githubAccountMatch {
		return githubAccountResolution{outcome: githubAccountOutcomeSwitched, expected: expected, previous: previous}
	}
	return githubAccountResolution{
		outcome:  githubAccountOutcomeSwitchFailed,
		expected: expected,
		previous: previous,
		reason:   githubSwitchFailureReason(switchResult),
	}
}

func githubSwitchFailureReason(result githubAccountCommandResult) string {
	if result.notFound {
		return "gh CLI is not installed"
	}
	if reason := strings.TrimSpace(result.stderr); reason != "" {
		return reason
	}
	if result.exitCode == 0 {
		return "the active account still does not match after switching"
	}
	return fmt.Sprintf("gh exited %d", result.exitCode)
}

func githubAccountSwitchNotice(resolution githubAccountResolution) string {
	if resolution.previous == "" {
		return fmt.Sprintf("switched active gh account to %q (was none active) for this project", resolution.expected)
	}
	return fmt.Sprintf("switched active gh account to %q (was %q) for this project", resolution.expected, resolution.previous)
}

func githubAccountSwitchFailureMessages(resolution githubAccountResolution) []string {
	return []string{
		fmt.Sprintf("project requires GitHub account %q, but switching to it failed: %s", resolution.expected, resolution.reason),
		fmt.Sprintf("Authenticate it with `gh auth login --hostname %s` as %s, then rerun.", githubAccountHostname, resolution.expected),
	}
}

// githubAccountDiagnostic renders a read-only mismatch diagnostic. It is used by
// reporting paths (release post-merge readiness) that surface state without
// mutating it; the enforcement paths converge via resolveGitHubAccount instead.
func githubAccountDiagnostic(expected string, result githubAccountCommandResult) string {
	if expected == "" {
		return ""
	}
	check := classifyGitHubAccount(expected, result)
	switch check.state {
	case githubAccountMatch:
		return ""
	case githubAccountUnavailable:
		return check.diagnostic
	}
	if check.actual == "" {
		return fmt.Sprintf("project requires GitHub account %q, but no active GitHub account found for %s", expected, githubAccountHostname)
	}
	return fmt.Sprintf("project requires GitHub account %q, but active gh account is %q; run `gh auth switch --hostname %s --user %s`", expected, check.actual, githubAccountHostname, expected)
}

// verifyConfiguredGitHubAccount converges the environment on the configured
// account before a gh release operation, switching when needed. A performed
// switch is announced on out so the mutation is never silent; only an
// unremediable state returns an error.
func verifyConfiguredGitHubAccount(root string, out io.Writer) error {
	expected, err := configuredGitHubAccount(root)
	if err != nil {
		return err
	}
	if expected == "" {
		return nil
	}
	resolution := resolveGitHubAccount(root, expected, activeGitHubAccount, switchGitHubAccount)
	switch resolution.outcome {
	case githubAccountOutcomeMatch:
		return nil
	case githubAccountOutcomeSwitched:
		if out != nil {
			fmt.Fprintf(out, "    %s %s\n", ansiYellow("!"), githubAccountSwitchNotice(resolution))
		}
		return nil
	case githubAccountOutcomeUnavailable:
		return fmt.Errorf("%s", resolution.reason)
	default:
		return fmt.Errorf("%s", strings.Join(githubAccountSwitchFailureMessages(resolution), " "))
	}
}
