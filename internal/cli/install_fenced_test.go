package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateFencedContentIsJournalFirst(t *testing.T) {
	content := generateFencedContent("2.0.0-test.1")
	if strings.Contains(content, "loaf session") {
		t.Fatalf("fenced content references deleted `loaf session` command:\n%s", content)
	}
	if !strings.Contains(content, "loaf journal log") {
		t.Fatalf("fenced content missing `loaf journal log` guidance:\n%s", content)
	}
	if !strings.Contains(content, "exact path-pinned Loaf executable") || !strings.Contains(content, "CODEX_HOME/AGENTS.md") || !strings.Contains(content, "Codex Auto mode") {
		t.Fatalf("fenced content missing conditional Codex basic-command guidance:\n%s", content)
	}
	if !strings.Contains(content, "loaf journal log/recent/search/context") {
		t.Fatalf("fenced content missing journal CLI command listing:\n%s", content)
	}
}

func TestInstallFencedSectionCreatesAppendsUpdatesAndSkips(t *testing.T) {
	root := realpath(t, t.TempDir())
	target := filepath.Join(root, "AGENTS.md")

	result, err := installFencedSection(target, "1.2.3-test.1", false)
	if err != nil {
		t.Fatalf("installFencedSection create error = %v", err)
	}
	if result.Action != "created" || result.Version != "1.2.3-test.1" {
		t.Fatalf("create result = %#v, want created with version", result)
	}
	body := string(readFileBytes(t, target))
	if !strings.Contains(body, "<!-- loaf:managed:start v1.2.3-test.1 sha256=") || !strings.Contains(body, "## Loaf Framework") {
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
	if !strings.HasPrefix(body, "# Project Notes\n\n<!-- loaf:managed:start v1.2.3-test.1 sha256=") {
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
	if !strings.Contains(body, "<!-- loaf:managed:start v1.2.4-test.1 sha256=") || strings.Contains(body, "v1.2.3-test.1") {
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

func TestInstallFencedSectionMigratesLegacyAndProtectsFingerprintedBody(t *testing.T) {
	root := realpath(t, t.TempDir())
	target := filepath.Join(root, "AGENTS.md")
	legacy := "# Before\n\n<!-- loaf:managed:start v1.2.3 -->\nold managed content\n<!-- loaf:managed:end -->\n\n# After\n"
	writeInstallFile(t, target, legacy)
	result, err := installFencedSection(target, "1.2.3", true)
	if err != nil || result.Action != "updated" {
		t.Fatalf("legacy migration = %#v, %v, want updated", result, err)
	}
	body := string(readFileBytes(t, target))
	if !strings.HasPrefix(body, "# Before\n\n") || !strings.HasSuffix(body, "\n\n# After\n") || !strings.Contains(body, "sha256=") {
		t.Fatalf("legacy migration did not preserve prose or add fingerprint: %q", body)
	}

	section, ok := findFencedSectionRange(body)
	if !ok {
		t.Fatal("missing fenced section")
	}
	tampered := body[:section.bodyStart] + "tampered\n" + body[section.bodyStart:]
	writeInstallFile(t, target, tampered)
	if _, err := installFencedSection(target, "1.2.3", true); err == nil || !strings.Contains(err.Error(), "was modified") {
		t.Fatalf("tampered fingerprinted body error = %v, want conflict", err)
	}
}

func TestInstallFencedSectionUpdatesSameVersionWhenOwnedBodyDiffers(t *testing.T) {
	target := filepath.Join(t.TempDir(), "AGENTS.md")
	oldBody := fencedWarning + "\nold content\n" + fencedEndMarker
	writeInstallFile(t, target, "<!-- loaf:managed:start v1.2.3 sha256="+sha256Hex(oldBody)+" -->\n"+oldBody+"\n")
	result, err := installFencedSection(target, "1.2.3", true)
	if err != nil || result.Action != "updated" {
		t.Fatalf("same-version owned update = %#v, %v, want updated", result, err)
	}
	if strings.Contains(string(readFileBytes(t, target)), "old content") {
		t.Fatal("same-version owned body was not replaced")
	}
}

func TestInstallFencedSectionRejectsMalformedFingerprint(t *testing.T) {
	for _, token := range []string{"sha256", "sha256=bad", "sha256=AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", "sha256=" + strings.Repeat("a", 64) + " sha256=" + strings.Repeat("a", 64), "sha256x=" + strings.Repeat("a", 64)} {
		t.Run(token[:6], func(t *testing.T) {
			target := filepath.Join(t.TempDir(), "AGENTS.md")
			writeInstallFile(t, target, "<!-- loaf:managed:start v1.2.3 "+token+" -->\nbody\n<!-- loaf:managed:end -->\n")
			if _, err := installFencedSection(target, "1.2.3", true); err == nil || !strings.Contains(err.Error(), "malformed fingerprint") {
				t.Fatalf("malformed token %q error = %v, want conflict", token, err)
			}
		})
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
	body := string(readFileBytes(t, filepath.Join(root, "AGENTS.md")))
	if count := strings.Count(body, "<!-- loaf:managed:start"); count != 1 {
		t.Fatalf("AGENTS.md fenced count = %d, want 1\n%s", count, body)
	}
}

func TestInstallFencedSectionsForTargetsDedupesSymlinkedClaudeFile(t *testing.T) {
	root := realpath(t, t.TempDir())
	canonical := filepath.Join(root, "AGENTS.md")
	writeInstallFile(t, canonical, "# Canonical\n")
	link := filepath.Join(root, ".claude", "CLAUDE.md")
	mkdirAll(t, filepath.Dir(link))
	if err := os.Symlink("../AGENTS.md", link); err != nil {
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
	if info, err := os.Lstat(link); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("CLAUDE.md link = %v, %v, want preserved symlink", info, err)
	}
}

func TestInstallFencedSectionRejectsInvalidStructure(t *testing.T) {
	for _, content := range []string{"<!-- loaf:managed:start v1 -->\n", "<!-- loaf:managed:end -->\n", "<!-- loaf:managed:start v1 -->\n<!-- loaf:managed:end -->\n<!-- loaf:managed:start v1 -->\n<!-- loaf:managed:end -->"} {
		target := filepath.Join(t.TempDir(), "AGENTS.md")
		writeInstallFile(t, target, content)
		if _, err := installFencedSection(target, "1.2.3", true); err == nil || !strings.Contains(err.Error(), "invalid fence structure") {
			t.Fatalf("structure %q error = %v", content, err)
		}
	}
}

func TestInstallFencedSectionPreservesExistingPermissions(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "AGENTS.md")
	writeInstallFile(t, target, "notes\n")
	if err := os.Chmod(target, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := installFencedSection(target, "1.2.3", true); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(target); err != nil {
		t.Fatalf("stat direct target: %v", err)
	} else if info.Mode().Perm() != 0o600 {
		t.Fatalf("direct mode = %v, want 0600", info.Mode())
	}
	link := filepath.Join(root, "CLAUDE.md")
	if err := os.Symlink("AGENTS.md", link); err != nil {
		t.Fatal(err)
	}
	if _, err := installFencedSection(link, "1.2.4", true); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(target); err != nil {
		t.Fatalf("stat symlink target: %v", err)
	} else if info.Mode().Perm() != 0o600 {
		t.Fatalf("symlink target mode = %v, want 0600", info.Mode())
	}
}

func TestInstallFencedSectionRetriesPrereleaseContainingSHA256(t *testing.T) {
	target := filepath.Join(t.TempDir(), "AGENTS.md")
	version := "1.2.3-sha256.preview"
	if result, err := installFencedSection(target, version, true); err != nil || result.Action != "created" {
		t.Fatalf("create = %#v, %v", result, err)
	}
	if result, err := installFencedSection(target, version, true); err != nil || result.Action != "skipped" {
		t.Fatalf("retry = %#v, %v, want skipped", result, err)
	}
}

func TestInstallFencedSectionRejectsUnterminatedHeader(t *testing.T) {
	target := filepath.Join(t.TempDir(), "AGENTS.md")
	writeInstallFile(t, target, "<!-- loaf:managed:start v1\nbody\n<!-- loaf:managed:end -->")
	if _, err := installFencedSection(target, "1.2.3", true); err == nil {
		t.Fatal("unterminated header accepted")
	}
}

func TestInstallFencedSectionRejectsInvalidStartHeaderFields(t *testing.T) {
	for _, header := range []string{"vsha256=bad", "v1 v2", "v1 extra", "sha256=" + strings.Repeat("a", 64), "v1\n<!-- unrelated -->"} {
		target := filepath.Join(t.TempDir(), "AGENTS.md")
		writeInstallFile(t, target, "<!-- loaf:managed:start "+header+" -->\nbody\n<!-- loaf:managed:end -->")
		if _, err := installFencedSection(target, "1.2.3", true); err == nil {
			t.Fatalf("header %q accepted", header)
		}
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
