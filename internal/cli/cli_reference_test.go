package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunnerGenerateCLIReferenceWritesSkillNatively(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	var stdout bytes.Buffer

	err := Runner{
		Stdout:     &stdout,
		WorkingDir: workingDir,
	}.Run([]string{"__generate-cli-ref"})
	if err != nil {
		t.Fatalf("__generate-cli-ref error = %v", err)
	}

	outputPath := filepath.Join(workingDir, "content", "skills", "loaf-reference", "SKILL.md")
	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", outputPath, err)
	}
	content := string(data)
	for _, want := range []string{
		"name: loaf-reference",
		"Documents how agents operate the Loaf CLI",
		"**Note:** This file is auto-generated from native CLI reference metadata.",
		"## Operating Rules",
		"`loaf --help`",
		"`loaf <command> --help`",
		"`loaf config check --json`, `loaf state doctor --json`, `loaf change check --json`",
		"## Command Index",
		"| Command | Purpose | Subcommands |",
		"| `loaf build` | Build skill distributions for agent harnesses | — |",
		"| `loaf config` | Validate and refresh project Loaf config | check |",
		"| `loaf change` | Shape-first Change artifacts: git-canonical work context under docs/changes/ | init, check |",
		"| `loaf journal` | Record and read the project-scoped journal (the durable record across all conversations) | log, recent, search, show, context, export |",
		"| `loaf state` | Manage native SQLite state | path, status, init, doctor, repair legacy-project-database,",
		"| `loaf doctor` | Diagnose Loaf project alignment (symlinks, stale files, version drift) | — |",
		"## Topics",
		"[references/configuration.md](references/configuration.md)",
		"[references/command-routing.md](references/command-routing.md)",
		"[references/troubleshooting.md](references/troubleshooting.md)",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("generated CLI reference missing %q\n%s", want, content)
		}
	}
	// The thinned router drops the verbose per-command catalog, the command
	// substitution placeholders, and the global-command and decision-guide
	// prose that moved into references/.
	for _, unwanted := range []string{
		"{{",
		"## Global Commands",
		"## Command Substitution Reference",
		"## Quick Decision Guide",
		"## Build Management",
		"## State Management",
		"**Subcommands:**",
		"```bash",
	} {
		if strings.Contains(content, unwanted) {
			t.Fatalf("thinned CLI reference still contains %q\n%s", unwanted, content)
		}
	}
	if !strings.Contains(stdout.String(), outputPath) {
		t.Fatalf("stdout = %q, want generated path %q", stdout.String(), outputPath)
	}
}
