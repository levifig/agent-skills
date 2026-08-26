package cli

// LOAF_PROJECT_ENV routes harness install to project-local config dirs on
// ephemeral cloud hosts. See cloud_environment_bootstrap.go for bootstrap scripts
// and LOAF-67 client-token env names.

import (
	"os"
	"path/filepath"

	"github.com/levifig/loaf/internal/project"
)

const projectEnvironmentEnv = "LOAF_PROJECT_ENV"

func projectEnvironmentActive() bool {
	return os.Getenv(projectEnvironmentEnv) == "1"
}

func projectEnvironmentConfigDirs(projectRoot string) map[string]string {
	return map[string]string{
		"opencode": filepath.Join(projectRoot, ".opencode"),
		"cursor":   filepath.Join(projectRoot, ".cursor"),
		"codex":    filepath.Join(projectRoot, ".codex"),
		"amp":      filepath.Join(projectRoot, ".amp"),
	}
}

func resolveInstallLayout(projectRoot string) (configDirs map[string]string, recordHome string) {
	if projectEnvironmentActive() {
		effectiveRoot := projectRoot
		if effectiveRoot == "" {
			if resolved, err := project.ResolveRoot("."); err == nil {
				effectiveRoot = resolved.Path()
			}
		}
		if effectiveRoot != "" {
			return projectEnvironmentConfigDirs(effectiveRoot), effectiveRoot
		}
	}
	return defaultInstallConfigDirs(), installHome()
}

func installLayoutConfigDirs(projectRoot string) map[string]string {
	dirs, _ := resolveInstallLayout(projectRoot)
	return dirs
}

func installLayoutHome(projectRoot string) string {
	_, home := resolveInstallLayout(projectRoot)
	return home
}
