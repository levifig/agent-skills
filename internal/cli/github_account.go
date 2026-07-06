package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

const githubAccountHostname = "github.com"

var githubCommandRE = regexp.MustCompile(`(^|[;&|]\s*|&&\s*|\|\|\s*|\()\s*((env|command|sudo)\s+|\S+=\S+\s+)*gh(\s|$)`)

type githubAccountCommandResult struct {
	stdout   string
	exitCode int
	notFound bool
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
		return githubAccountCommandResult{stdout: string(output), exitCode: exitErr.ExitCode()}
	}
	return githubAccountCommandResult{exitCode: 1}
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
	return "", fmt.Errorf("no active GitHub account found for %s", githubAccountHostname)
}

func githubAccountDiagnostic(expected string, result githubAccountCommandResult) string {
	if expected == "" {
		return ""
	}
	if result.notFound {
		return fmt.Sprintf("project requires GitHub account %q, but gh CLI is not installed", expected)
	}
	if result.exitCode != 0 {
		return fmt.Sprintf("project requires GitHub account %q, but `gh auth status --active --hostname %s --json hosts` failed", expected, githubAccountHostname)
	}
	actual, err := activeGitHubAccountFromJSON(result.stdout)
	if err != nil {
		return fmt.Sprintf("project requires GitHub account %q, but %s", expected, err)
	}
	if strings.EqualFold(actual, expected) {
		return ""
	}
	return fmt.Sprintf("project requires GitHub account %q, but active gh account is %q; run `gh auth switch --hostname %s --user %s`", expected, actual, githubAccountHostname, expected)
}

func verifyConfiguredGitHubAccount(root string) error {
	expected, err := configuredGitHubAccount(root)
	if err != nil {
		return err
	}
	if expected == "" {
		return nil
	}
	if diagnostic := githubAccountDiagnostic(expected, activeGitHubAccount(root)); diagnostic != "" {
		return fmt.Errorf("%s", diagnostic)
	}
	return nil
}
