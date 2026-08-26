package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Cloud project-environment bootstrap for Cursor Cloud Agents and Amp Orbs.
//
// Harness surfaces ride `loaf install` under LOAF_PROJECT_ENV=1 (project-local
// .cursor/, .amp/, .agents/skills/), not user-level ~/.cursor or hooks.json.
//
// Project-environment layout (LOAF_PROJECT_ENV):
//   - Must be exported in every cloud session entry that runs loaf after
//     bootstrap (Cursor start script; Amp setup/resume). Cursor Cloud does not
//     carry install-time shell exports into agent sessions.
//   - Optional durable project env var with the same name keeps layout without
//     relying on start/resume scripts alone.
//
// LOAF-67 attach client token (wired in bootstrap start/resume scripts):
//   - Mint locally: loaf auth link --project
//   - Cursor Cloud Agent: add project environment vars LOAF_CLIENT_TOKEN and
//     preferably LOAF_PROJECT_ENV=1 (project-scoped client wire; never the
//     operator master key). Start script runs `loaf attach` when the token is set.
//   - Amp Orb: add project env LOAF_CLIENT_TOKEN (and LOAF_PROJECT_ENV=1) via
//     amp project configuration. Resume script runs `loaf attach` when set.
//   - Optional sync endpoint (when configured): LOAF_SYNC_URL
//
// Human-review setup guide: docs/cloud/project-environment-attach.md
//
// Cursor Cloud harness install runs in the install/build script so hooks and
// skills exist before the agent starts. Attach pre-warm runs in start/resume
// when LOAF_CLIENT_TOKEN is present.
const (
	// projectEnvironmentClientTokenEnv is the per-project env var name for the
	// LOAF-67 client wire in Cursor Cloud and Amp Orb environments.
	projectEnvironmentClientTokenEnv = "LOAF_CLIENT_TOKEN"
	// projectEnvironmentSyncURLEnv is optional when a project sync endpoint is configured.
	projectEnvironmentSyncURLEnv = "LOAF_SYNC_URL"
)

const (
	cursorCloudEnvironmentFile = ".cursor/environment.json"
	cursorCloudInstallScript   = ".cursor/loaf-cloud-install.sh"
	cursorCloudStartScript     = ".cursor/loaf-cloud-start.sh"
	ampOrbSetupScript          = ".agents/setup"
	ampOrbResumeScript         = ".agents/resume"
)

type cursorCloudEnvironment struct {
	Install string `json:"install"`
	Start   string `json:"start"`
}

var requiredCloudBootstrapMarkers = map[string][]string{
	cursorCloudInstallScript: {
		"npm run build:go",
		"bin/native/",
		"bin/loaf",
		projectEnvironmentEnv + "=1",
		"loaf install --to cursor",
	},
	cursorCloudStartScript: {
		projectEnvironmentEnv + "=1",
		"loaf attach",
	},
	ampOrbSetupScript: {
		"npm run build:go",
		"bin/native/",
		"bin/loaf",
		projectEnvironmentEnv + "=1",
		"loaf install --to amp",
	},
	ampOrbResumeScript: {
		projectEnvironmentEnv + "=1",
		"loaf install --to amp",
		"loaf attach",
	},
}

func loafRepositoryRoot() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for dir := wd; ; dir = filepath.Dir(dir) {
		if fileExistsForInstall(filepath.Join(dir, "go.mod")) &&
			fileExistsForInstall(filepath.Join(dir, "package.json")) &&
			dirExistsForInstall(filepath.Join(dir, "cmd", "loaf")) {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
	}
	return "", fmt.Errorf("loaf repository root not found from %s", wd)
}

func stripShellComments(body string) string {
	var b strings.Builder
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}

func readCloudBootstrapFile(root, rel string) (string, error) {
	body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func validateCloudBootstrapScript(root, rel string) error {
	body, err := readCloudBootstrapFile(root, rel)
	if err != nil {
		return err
	}
	for _, marker := range requiredCloudBootstrapMarkers[rel] {
		if !strings.Contains(body, marker) {
			return fmt.Errorf("%s missing %q", rel, marker)
		}
	}
	if rel == cursorCloudInstallScript || rel == ampOrbSetupScript {
		executable := stripShellComments(body)
		loafIdx := strings.Index(executable, "loaf ")
		buildIdx := strings.Index(executable, "npm run build:go")
		if buildIdx < 0 {
			return fmt.Errorf("%s must install the Loaf CLI before the first loaf command", rel)
		}
		if loafIdx >= 0 && buildIdx > loafIdx {
			return fmt.Errorf("%s runs loaf before CLI install", rel)
		}
	}
	return nil
}

func validateCursorCloudEnvironment(root string) error {
	raw, err := readCloudBootstrapFile(root, cursorCloudEnvironmentFile)
	if err != nil {
		return err
	}
	var env cursorCloudEnvironment
	if err := json.Unmarshal([]byte(raw), &env); err != nil {
		return fmt.Errorf("parse %s: %w", cursorCloudEnvironmentFile, err)
	}
	if env.Install != cursorCloudInstallScript {
		return fmt.Errorf("%s install = %q, want %q", cursorCloudEnvironmentFile, env.Install, cursorCloudInstallScript)
	}
	if env.Start != cursorCloudStartScript {
		return fmt.Errorf("%s start = %q, want %q", cursorCloudEnvironmentFile, env.Start, cursorCloudStartScript)
	}
	return nil
}

func validateCloudEnvironmentBootstrap(root string) error {
	if err := validateCursorCloudEnvironment(root); err != nil {
		return err
	}
	for _, rel := range []string{cursorCloudInstallScript, cursorCloudStartScript, ampOrbSetupScript, ampOrbResumeScript} {
		if err := validateCloudBootstrapScript(root, rel); err != nil {
			return err
		}
	}
	return nil
}
