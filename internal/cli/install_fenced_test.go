package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateFencedContentIsJournalFirst(t *testing.T) {
	content := generateFencedContent()
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
	if !strings.Contains(content, "See the Loaf `orchestration` skill for full details.") {
		t.Fatalf("fenced content missing orchestration skill pointer:\n%s", content)
	}
	if strings.Contains(content, "skills/orchestration/SKILL.md") {
		t.Fatalf("fenced content references repo-relative dead link skills/orchestration/SKILL.md:\n%s", content)
	}
	if !strings.Contains(content, "<!-- loaf:managed:start sha256=") {
		t.Fatalf("fenced content missing sha256-only start marker:\n%s", content)
	}
	if strings.Contains(content, "<!-- loaf:managed:start v") {
		t.Fatalf("fenced content still embeds a version stamp:\n%s", content)
	}
}

func TestInstallFencedSectionCreatesAppendsAndSkipsNoChurn(t *testing.T) {
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
	if !strings.Contains(body, "<!-- loaf:managed:start sha256=") || !strings.Contains(body, "## Loaf Framework") {
		t.Fatalf("created fenced body = %q, want managed section with sha256-only marker", body)
	}
	if strings.Contains(body, "<!-- loaf:managed:start v") {
		t.Fatalf("created body still has version stamp: %q", body)
	}
	afterCreate := body

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
	if !strings.HasPrefix(body, "# Project Notes\n\n<!-- loaf:managed:start sha256=") {
		t.Fatalf("appended body = %q, want notes before fenced section", body)
	}

	// Version change with identical body must not churn (Decision 1 / no-churn).
	result, err = installFencedSection(plain, "1.2.4-test.1", true)
	if err != nil {
		t.Fatalf("installFencedSection no-churn error = %v", err)
	}
	if result.Action != "skipped" || result.Version != "1.2.4-test.1" {
		t.Fatalf("no-churn result = %#v, want skipped with acting version", result)
	}
	afterSkip := string(readFileBytes(t, plain))
	if afterSkip != body {
		t.Fatalf("no-churn mutated file:\nbefore=%q\nafter=%q", body, afterSkip)
	}

	result, err = installFencedSection(target, "9.9.9", true)
	if err != nil {
		t.Fatalf("installFencedSection second skip error = %v", err)
	}
	if result.Action != "skipped" {
		t.Fatalf("second skip result = %#v, want skipped", result)
	}
	if string(readFileBytes(t, target)) != afterCreate {
		t.Fatal("second skip mutated created file")
	}
}

func TestInstallFencedSectionMatrixRows(t *testing.T) {
	generated := generateFencedContent()
	generatedFP := fencedContentFingerprint(generated)
	section, ok := findFencedSectionRange(generated)
	if !ok {
		t.Fatal("generated content missing section")
	}
	generatedBody := generated[section.bodyStart:section.end]

	oldBody := fencedWarning + "\nold content\n" + fencedEndMarker
	oldFP := sha256Hex(oldBody)

	type row struct {
		name       string
		seed       string
		wantAction string
		wantErr    string
		checkAfter func(t *testing.T, body string)
	}
	rows := []row{
		{
			name:       "new_form_match_match_skipped",
			seed:       generated + "\n",
			wantAction: "skipped",
		},
		{
			name:       "new_form_match_differ_updated",
			seed:       "<!-- loaf:managed:start sha256=" + oldFP + " -->\n" + oldBody + "\n",
			wantAction: "updated",
			checkAfter: func(t *testing.T, body string) {
				t.Helper()
				if strings.Contains(body, "old content") || strings.Contains(body, "<!-- loaf:managed:start v") {
					t.Fatalf("updated body = %q, want generated content with sha256-only header", body)
				}
				if !strings.Contains(body, "<!-- loaf:managed:start sha256="+generatedFP) {
					t.Fatalf("updated body missing generated fingerprint: %q", body)
				}
			},
		},
		{
			name:    "new_form_tampered_refused",
			seed:    "<!-- loaf:managed:start sha256=" + generatedFP + " -->\ntampered\n" + fencedEndMarker + "\n",
			wantErr: "was modified",
		},
		{
			name:    "legacy_sha_tampered_refused",
			seed:    "<!-- loaf:managed:start v1.2.3 sha256=" + generatedFP + " -->\ntampered\n" + fencedEndMarker + "\n",
			wantErr: "was modified",
		},
		{
			name:       "legacy_sha_match_match_transition_updated",
			seed:       "<!-- loaf:managed:start v1.2.3 sha256=" + generatedFP + " -->\n" + generatedBody + "\n",
			wantAction: "updated",
			checkAfter: func(t *testing.T, body string) {
				t.Helper()
				if strings.Contains(body, "<!-- loaf:managed:start v") {
					t.Fatalf("transition left version stamp: %q", body)
				}
				if !strings.Contains(body, "<!-- loaf:managed:start sha256="+generatedFP) {
					t.Fatalf("transition missing new-form header: %q", body)
				}
			},
		},
		{
			name:       "legacy_sha_match_differ_updated",
			seed:       "<!-- loaf:managed:start v1.2.3 sha256=" + oldFP + " -->\n" + oldBody + "\n",
			wantAction: "updated",
		},
		{
			name:       "legacy_v_only_always_updated",
			seed:       "<!-- loaf:managed:start v1.2.3 -->\nold managed content\n" + fencedEndMarker + "\n",
			wantAction: "updated",
			checkAfter: func(t *testing.T, body string) {
				t.Helper()
				if strings.Contains(body, "old managed content") || strings.Contains(body, "<!-- loaf:managed:start v") {
					t.Fatalf("v-only migration incomplete: %q", body)
				}
			},
		},
	}

	for _, tc := range rows {
		t.Run(tc.name, func(t *testing.T) {
			target := filepath.Join(t.TempDir(), "AGENTS.md")
			writeInstallFile(t, target, tc.seed)
			result, err := installFencedSection(target, "9.9.9", true)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error = %v, want containing %q", err, tc.wantErr)
				}
				return
			}
			if err != nil || result.Action != tc.wantAction {
				t.Fatalf("result = %#v, err = %v, want action %q", result, err, tc.wantAction)
			}
			if tc.checkAfter != nil {
				tc.checkAfter(t, string(readFileBytes(t, target)))
			}
		})
	}
}

func TestInstallFencedSectionLegacyTransitionThenIdempotent(t *testing.T) {
	generated := generateFencedContent()
	fp := fencedContentFingerprint(generated)
	section, ok := findFencedSectionRange(generated)
	if !ok {
		t.Fatal("generated content missing section")
	}
	body := generated[section.bodyStart:section.end]
	target := filepath.Join(t.TempDir(), "AGENTS.md")
	writeInstallFile(t, target, "<!-- loaf:managed:start v2.0.0-alpha.11 sha256="+fp+" -->\n"+body+"\n")

	result, err := installFencedSection(target, "2.0.0-alpha.13", true)
	if err != nil || result.Action != "updated" {
		t.Fatalf("transition = %#v, %v, want updated", result, err)
	}
	afterTransition := string(readFileBytes(t, target))
	if strings.Contains(afterTransition, "<!-- loaf:managed:start v") || !strings.Contains(afterTransition, "<!-- loaf:managed:start sha256="+fp) {
		t.Fatalf("after transition = %q, want sha256-only header", afterTransition)
	}

	result, err = installFencedSection(target, "2.0.0-alpha.14", true)
	if err != nil || result.Action != "skipped" {
		t.Fatalf("second run = %#v, %v, want skipped", result, err)
	}
	if string(readFileBytes(t, target)) != afterTransition {
		t.Fatal("second run mutated file after transition")
	}
}

func TestFencedPlanApplyParityMatrix(t *testing.T) {
	generated := generateFencedContent()
	generatedFP := fencedContentFingerprint(generated)
	section, ok := findFencedSectionRange(generated)
	if !ok {
		t.Fatal("generated content missing section")
	}
	generatedBody := generated[section.bodyStart:section.end]
	oldBody := fencedWarning + "\nold content\n" + fencedEndMarker
	oldFP := sha256Hex(oldBody)

	seeds := []struct {
		name string
		seed string
	}{
		{"new_match", generated + "\n"},
		{"new_differ", "<!-- loaf:managed:start sha256=" + oldFP + " -->\n" + oldBody + "\n"},
		{"new_tamper", "<!-- loaf:managed:start sha256=" + generatedFP + " -->\ntampered\n" + fencedEndMarker + "\n"},
		{"legacy_sha_match", "<!-- loaf:managed:start v1.2.3 sha256=" + generatedFP + " -->\n" + generatedBody + "\n"},
		{"legacy_sha_differ", "<!-- loaf:managed:start v1.2.3 sha256=" + oldFP + " -->\n" + oldBody + "\n"},
		{"legacy_sha_tamper", "<!-- loaf:managed:start v1.2.3 sha256=" + generatedFP + " -->\ntampered\n" + fencedEndMarker + "\n"},
		{"legacy_v_only", "<!-- loaf:managed:start v1.2.3 -->\nold\n" + fencedEndMarker + "\n"},
		{"absent", ""},
		{"plain_append", "# Notes\n"},
	}

	for _, tc := range seeds {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			planTarget := filepath.Join(root, "plan.md")
			applyTarget := filepath.Join(root, "apply.md")
			if tc.seed == "" && tc.name == "absent" {
				// leave both missing
			} else {
				writeInstallFile(t, planTarget, tc.seed)
				writeInstallFile(t, applyTarget, tc.seed)
			}

			planAction, _ := planFencedSection(planTarget, "9.9.9")
			applyResult, applyErr := installFencedSection(applyTarget, "9.9.9", true)
			applyAction := applyResult.Action
			if applyErr != nil {
				applyAction = "error"
			}
			if planAction != applyAction {
				t.Fatalf("plan action %q != apply action %q (applyErr=%v)", planAction, applyAction, applyErr)
			}
		})
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
	if strings.Contains(body, "<!-- loaf:managed:start v") {
		t.Fatalf("legacy migration left version stamp: %q", body)
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
	writeInstallFile(t, target, "<!-- loaf:managed:start sha256="+sha256Hex(oldBody)+" -->\n"+oldBody+"\n")
	result, err := installFencedSection(target, "1.2.3", true)
	if err != nil || result.Action != "updated" {
		t.Fatalf("owned body update = %#v, %v, want updated", result, err)
	}
	if strings.Contains(string(readFileBytes(t, target)), "old content") {
		t.Fatal("owned body was not replaced")
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

func TestParseFencedStartHeaderAcceptsThreeForms(t *testing.T) {
	sha := strings.Repeat("a", 64)
	cases := []struct {
		line        string
		wantVersion string
		wantSHA     string
		wantOK      bool
	}{
		{"<!-- loaf:managed:start sha256=" + sha + " -->", "", sha, true},
		{"<!-- loaf:managed:start v1.2.3 sha256=" + sha + " -->", "1.2.3", sha, true},
		{"<!-- loaf:managed:start v1.2.3 -->", "1.2.3", "", true},
		{"<!-- loaf:managed:start v1.2.3-sha256.preview sha256=" + sha + " -->", "1.2.3-sha256.preview", sha, true},
		{"<!-- loaf:managed:start sha256=bad -->", "", "", false},
		{"<!-- loaf:managed:start v1 v2 -->", "", "", false},
	}
	for _, tc := range cases {
		version, gotSHA, ok := parseFencedStartHeader(tc.line)
		if ok != tc.wantOK || version != tc.wantVersion || gotSHA != tc.wantSHA {
			t.Fatalf("parse %q = (%q, %q, %v), want (%q, %q, %v)", tc.line, version, gotSHA, ok, tc.wantVersion, tc.wantSHA, tc.wantOK)
		}
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
	for _, header := range []string{"vsha256=bad", "v1 v2", "v1 extra", "v1\n<!-- unrelated -->"} {
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

// legacyStampedFencedContent builds a legacy v+sha header around the current
// generated body so pre-U2 doctor fixtures can still assert version identity.
func legacyStampedFencedContent(version string) string {
	content := generateFencedContent()
	section, ok := findFencedSectionRange(content)
	if !ok {
		panic("generated content missing section")
	}
	body := content[section.bodyStart:section.end]
	return "<!-- loaf:managed:start v" + version + " sha256=" + section.fingerprint + " -->\n" + body
}
