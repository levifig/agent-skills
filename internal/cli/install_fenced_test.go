package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallFencedSectionCreatesAppendsUpdatesAndSkips(t *testing.T) {
	root := realpath(t, t.TempDir())
	target := filepath.Join(root, ".agents", "AGENTS.md")

	result, err := installFencedSection(target, "1.2.3-test.1", false)
	if err != nil {
		t.Fatalf("installFencedSection create error = %v", err)
	}
	if result.Action != "created" || result.Version != "1.2.3-test.1" {
		t.Fatalf("create result = %#v, want created with version", result)
	}
	body := string(readFileBytes(t, target))
	if !strings.Contains(body, "<!-- loaf:managed:start v1.2.3-test.1 -->") || !strings.Contains(body, "## Loaf Framework") {
		t.Fatalf("created fenced body = %q, want managed section", body)
	}

	plain := filepath.Join(root, "plain.md")
	writeInstallFile(t, plain, "# Project Notes\n")
	result, err = installFencedSection(plain, "1.2.3-test.1", false)
	if err != nil {
		t.Fatalf("installFencedSection append error = %v", err)
	}
	if result.Action != "appended" {
		t.Fatalf("append result = %#v, want appended", result)
	}
	body = string(readFileBytes(t, plain))
	if !strings.HasPrefix(body, "# Project Notes\n\n<!-- loaf:managed:start v1.2.3-test.1 -->") {
		t.Fatalf("appended body = %q, want notes before fenced section", body)
	}

	result, err = installFencedSection(plain, "1.2.4-test.1", true)
	if err != nil {
		t.Fatalf("installFencedSection update error = %v", err)
	}
	if result.Action != "updated" || result.Version != "1.2.4-test.1" {
		t.Fatalf("update result = %#v, want updated", result)
	}
	body = string(readFileBytes(t, plain))
	if !strings.Contains(body, "<!-- loaf:managed:start v1.2.4-test.1 -->") || strings.Contains(body, "v1.2.3-test.1") {
		t.Fatalf("updated body = %q, want new version only", body)
	}

	result, err = installFencedSection(plain, "1.2.4-test.1", true)
	if err != nil {
		t.Fatalf("installFencedSection skip error = %v", err)
	}
	if result.Action != "skipped" {
		t.Fatalf("skip result = %#v, want skipped", result)
	}
}

func TestInstallFencedSectionsForTargetsDedupesSharedCanonicalPath(t *testing.T) {
	root := realpath(t, t.TempDir())
	results := installFencedSectionsForTargets([]string{"cursor", "codex", "opencode"}, root, "2.0.0-test.1", false)

	if results["cursor"].Action != "created" {
		t.Fatalf("cursor result = %#v, want created", results["cursor"])
	}
	for _, target := range []string{"codex", "opencode"} {
		if results[target].Action != "skipped" {
			t.Fatalf("%s result = %#v, want skipped shared file", target, results[target])
		}
	}
	body := string(readFileBytes(t, filepath.Join(root, ".agents", "AGENTS.md")))
	if count := strings.Count(body, "<!-- loaf:managed:start"); count != 1 {
		t.Fatalf("AGENTS.md fenced count = %d, want 1\n%s", count, body)
	}
}

func TestInstallFencedSectionsForTargetsDedupesSymlinkedClaudeFile(t *testing.T) {
	root := realpath(t, t.TempDir())
	canonical := filepath.Join(root, ".agents", "AGENTS.md")
	writeInstallFile(t, canonical, "# Canonical\n")
	link := filepath.Join(root, ".claude", "CLAUDE.md")
	mkdirAll(t, filepath.Dir(link))
	if err := os.Symlink("../.agents/AGENTS.md", link); err != nil {
		t.Fatalf("Symlink(CLAUDE.md) error = %v", err)
	}

	results := installFencedSectionsForTargets([]string{"claude-code", "cursor"}, root, "2.0.0-test.1", false)
	if results["claude-code"].Action != "appended" {
		t.Fatalf("claude-code result = %#v, want appended through symlink", results["claude-code"])
	}
	if results["cursor"].Action != "skipped" {
		t.Fatalf("cursor result = %#v, want skipped after symlink write", results["cursor"])
	}
	body := string(readFileBytes(t, canonical))
	if count := strings.Count(body, "<!-- loaf:managed:start"); count != 1 {
		t.Fatalf("canonical fenced count = %d, want 1\n%s", count, body)
	}
}

func TestInstallFencedSectionsForTargetsReportsUnknownTarget(t *testing.T) {
	root := realpath(t, t.TempDir())
	results := installFencedSectionsForTargets([]string{"cursor", "wat"}, root, "2.0.0-test.1", false)
	if results["cursor"].Action != "created" {
		t.Fatalf("cursor result = %#v, want created", results["cursor"])
	}
	if results["wat"].Action != "error" || !strings.Contains(results["wat"].Error, "Unknown target") {
		t.Fatalf("unknown target result = %#v, want error", results["wat"])
	}
}
