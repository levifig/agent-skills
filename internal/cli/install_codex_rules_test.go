package cli

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildNativeCodexCopiesJournalRuleArtifact(t *testing.T) {
	root := setupBuildCommandLoafRoot(t)
	seedNativeCodexBuildFixture(t, root)
	rule := "# generated Codex authority\n{{LOAF_BASIC_RULES}}\n"
	writeInstallFile(t, filepath.Join(root, "content", "codex", "rules", "loaf.rules.tmpl"), rule)

	if err := (Runner{WorkingDir: root}).Run([]string{"build", "--target", "codex"}); err != nil {
		t.Fatalf("build --target codex error = %v", err)
	}
	template := readBuildFileString(t, filepath.Join(root, "dist", "codex", ".codex", "rules", "loaf.rules.tmpl"))
	if template != rule || !strings.Contains(template, codexBasicRulesPlaceholder) {
		t.Fatalf("Codex rule template = %q, want exact unresolved source template", template)
	}
}

func TestBuildNativeCodexFailsWhenJournalRuleTemplateMissing(t *testing.T) {
	root := setupBuildCommandLoafRoot(t)
	seedNativeCodexBuildFixture(t, root)
	if err := os.Remove(filepath.Join(root, "content", "codex", "rules", "loaf.rules.tmpl")); err != nil {
		t.Fatalf("remove rule template: %v", err)
	}
	if err := (Runner{WorkingDir: root}).Run([]string{"build", "--target", "codex"}); err == nil || !strings.Contains(err.Error(), "journal rule template missing") {
		t.Fatalf("build error = %v, want missing-template failure", err)
	}
}

func TestRenderCodexJournalRuleRendersOnePinnedPrefixPerBasicLeaf(t *testing.T) {
	template := "# policy\n" + codexBasicRulesPlaceholder + "\n"
	rendered, err := renderCodexJournalRule(template, "/opt/loaf/bin/loaf")
	if err != nil {
		t.Fatalf("render Codex policy: %v", err)
	}
	if strings.Contains(rendered, codexBasicRulesPlaceholder) || strings.Contains(rendered, codexJournalExecutablePlaceholder) {
		t.Fatalf("rendered policy = %q, want no unresolved placeholders", rendered)
	}
	for _, prefix := range BasicCommandAuthorityPrefixes() {
		needle := "pattern = [\"/opt/loaf/bin/loaf\""
		for _, token := range prefix {
			needle += ", " + fmt.Sprintf("%q", token)
		}
		if !strings.Contains(rendered, needle+"]") {
			t.Fatalf("rendered policy missing prefix %v", prefix)
		}
	}
	if got := strings.Count(rendered, "prefix_rule("); got != len(BasicCommandAuthorityPrefixes()) {
		t.Fatalf("rendered prefix_rule count = %d, want %d", got, len(BasicCommandAuthorityPrefixes()))
	}
}

func TestParseInstallArgsCodexBasicCommandsRequiresCodexTarget(t *testing.T) {
	options, err := parseInstallArgs([]string{"--to", "codex", "--codex-basic-commands"})
	if err != nil {
		t.Fatalf("parse Codex opt-in = %v", err)
	}
	if !options.codexBasicCommands || options.target != "codex" {
		t.Fatalf("options = %#v, want Codex opt-in", options)
	}
	if _, err := parseInstallArgs([]string{"--to", "cursor", "--codex-basic-commands"}); err == nil || !strings.Contains(err.Error(), "requires --to codex or --to all") {
		t.Fatalf("invalid target error = %v, want explicit target refusal", err)
	}
}

func TestValidateCodexJournalExecutableRejectsMissingAndDisposablePaths(t *testing.T) {
	t.Run("missing", func(t *testing.T) {
		t.Setenv("PATH", t.TempDir())
		if err := validateCodexJournalExecutable("/project"); err == nil || !strings.Contains(err.Error(), "not on PATH") {
			t.Fatalf("trust validation error = %v, want missing executable refusal", err)
		}
	})
	t.Run("temporary", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "loaf")
		writeInstallFile(t, path, "#!/bin/sh\nexit 0\n")
		if err := os.Chmod(path, 0o755); err != nil {
			t.Fatalf("chmod fake loaf: %v", err)
		}
		t.Setenv("PATH", dir)
		if err := validateCodexJournalExecutable("/project"); err == nil || !strings.Contains(err.Error(), "forbidden path") {
			t.Fatalf("trust validation error = %v, want disposable-path refusal", err)
		}
	})
}

func TestCodexJournalRuleExecpolicyClassification(t *testing.T) {
	codex, err := exec.LookPath("codex")
	if err != nil {
		t.Skip("codex command is not installed; execpolicy classification is not verified")
	}
	rulePath := filepath.Join(realpath(t, t.TempDir()), "loaf.rules")
	body, err := os.ReadFile(filepath.Join("..", "..", "content", "codex", "rules", "loaf.rules.tmpl"))
	if err != nil {
		t.Fatalf("read source rule: %v", err)
	}
	rendered, err := renderCodexJournalRule(string(body), "/usr/local/bin/loaf")
	if err != nil {
		t.Fatalf("render Codex rule: %v", err)
	}
	writeFile(t, rulePath, rendered)
	trusted := "/usr/local/bin/loaf"
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{name: "manual", args: []string{trusted, "journal", "log", "--execpolicy-safe", "decision(scope): message"}, want: "allow"},
		{name: "ordinary-log", args: []string{trusted, "journal", "log", "decision(scope): message"}, want: ""},
		{name: "from-hook", args: []string{trusted, "journal", "log", "--execpolicy-safe", "--from-hook"}, want: "allow"},
		{name: "detect-linear", args: []string{trusted, "journal", "log", "--execpolicy-safe", "--detect-linear"}, want: "allow"},
		{name: "bare-loaf", args: []string{"loaf", "journal", "log", "--execpolicy-safe", "decision(scope): message"}, want: ""},
		{name: "alternate-absolute", args: []string{"/usr/bin/loaf", "journal", "log", "--execpolicy-safe", "decision(scope): message"}, want: ""},
		{name: "unsafed-log", args: []string{"loaf", "journal", "log", "decision(scope): message"}, want: ""},
		{name: "journal", args: []string{trusted, "journal"}, want: ""},
		{name: "defer", args: []string{trusted, "journal", "defer", "later"}, want: "allow"},
		{name: "state", args: []string{trusted, "state", "status"}, want: "allow"},
		{name: "task", args: []string{trusted, "task", "list"}, want: "allow"},
		{name: "idea", args: []string{trusted, "idea", "list"}, want: "allow"},
		{name: "state-export-all", args: []string{trusted, "state", "export", "all", "--format", "json"}, want: "allow"},
		{name: "state-export-unknown", args: []string{trusted, "state", "export", "unknown", "--format", "json"}, want: ""},
		{name: "report-generate-triage", args: []string{trusted, "report", "generate", "triage", "--format", "markdown"}, want: "allow"},
		{name: "report-generate-unknown", args: []string{trusted, "report", "generate", "unknown", "--format", "markdown"}, want: ""},
		{name: "unsafe-report-body-file", args: []string{trusted, "report", "create", "report", "--body-file", "/etc/passwd"}, want: ""},
		{name: "unsafe-finding-import", args: []string{trusted, "finding", "import-json", "--report", "report", "/etc/passwd"}, want: ""},
		{name: "unsafe-spec-body-file", args: []string{trusted, "spec", "new", "spec", "--body-file", "/etc/passwd"}, want: ""},
		{name: "unsafe-change-path", args: []string{trusted, "change", "check", "/outside/change.md"}, want: ""},
		{name: "unsafe-state-doctor", args: []string{trusted, "state", "doctor", "--fix"}, want: ""},
		{name: "unsafe-release", args: []string{trusted, "release"}, want: ""},
		{name: "unsafe-spec-finalize", args: []string{trusted, "spec", "finalize", "SPEC-001"}, want: ""},
		{name: "env-wrapper", args: []string{"env", "LOAF_DB=/outside/target.sqlite", trusted, "journal", "log", "--execpolicy-safe", "decision(scope): message"}, want: ""},
		{name: "shell-wrapper", args: []string{"sh", "-c", trusted + " journal log --execpolicy-safe 'decision(scope): message'"}, want: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmdArgs := append([]string{"execpolicy", "check", "--rules", rulePath}, tc.args...)
			output, runErr := exec.Command(codex, cmdArgs...).CombinedOutput()
			if runErr != nil {
				t.Fatalf("codex execpolicy check error = %v\n%s", runErr, output)
			}
			var result struct {
				Decision     string `json:"decision"`
				MatchedRules []any  `json:"matchedRules"`
			}
			if err := json.Unmarshal(output, &result); err != nil {
				t.Fatalf("decode execpolicy output: %v\n%s", err, output)
			}
			if tc.want == "allow" {
				if result.Decision != "allow" || len(result.MatchedRules) == 0 {
					t.Fatalf("result = %s, want an allowed match", output)
				}
			} else if result.Decision == "allow" || len(result.MatchedRules) != 0 {
				t.Fatalf("result = %s, want no matching allow rule", output)
			}
		})
	}
}

func TestInstallCodexJournalRuleFirstInstallRequiresOptIn(t *testing.T) {
	fixture := newCodexRuleInstallFixture(t)
	if err := installCodexJournalRule(fixture.options(false, false), fixture.codexHome); err != nil {
		t.Fatalf("install without opt-in error = %v", err)
	}
	assertInstallPathMissing(t, fixture.dest())
	assertInstallPathMissing(t, fixture.manifest())
}

func TestInstallCodexJournalRuleExplicitOptInOwnsAndUpgrades(t *testing.T) {
	fixture := newCodexRuleInstallFixture(t)
	if err := fixture.install(t, true, false); err != nil {
		t.Fatalf("explicit install error = %v", err)
	}
	assertInstallFile(t, fixture.dest(), fixture.renderedBody())
	manifest := readCodexRuleManifestTest(t, fixture.manifest())
	if digest, ok := manifest.ownedDigest(codexJournalRuleRelativePath); !ok || digest != sha256Bytes([]byte(fixture.renderedBody())) {
		t.Fatalf("manifest = %#v, want source digest ownership", manifest)
	}

	updated := fixture.sourceBody() + "# updated\n"
	writeFile(t, fixture.source(), updated)
	if err := fixture.install(t, false, true); err != nil {
		t.Fatalf("owned upgrade error = %v", err)
	}
	expected, err := renderCodexJournalRule(updated, filepath.Join(fixture.root, "trusted-bin", "loaf"))
	if err != nil {
		t.Fatalf("render updated rule: %v", err)
	}
	assertInstallFile(t, fixture.dest(), expected)
}

func TestInstallCodexJournalRulePreservesUnrelatedRulesAndRejectsConflicts(t *testing.T) {
	fixture := newCodexRuleInstallFixture(t)
	other := filepath.Join(fixture.codexHome, "rules", "other.rules")
	writeInstallFile(t, other, "prefix_rule(pattern=[\"git\"], decision=\"allow\")\n")
	writeInstallFile(t, fixture.dest(), "user-owned\n")
	if err := fixture.install(t, true, false); err == nil || !strings.Contains(err.Error(), "unowned") {
		t.Fatalf("unowned conflict error = %v, want refusal", err)
	}
	assertInstallFile(t, other, "prefix_rule(pattern=[\"git\"], decision=\"allow\")\n")
	assertInstallFile(t, fixture.dest(), "user-owned\n")
}

func TestInstallCodexJournalRuleRejectsModifiedOwnedAndRemovesStaleSafely(t *testing.T) {
	fixture := newCodexRuleInstallFixture(t)
	if err := fixture.install(t, true, false); err != nil {
		t.Fatalf("explicit install error = %v", err)
	}
	writeInstallFile(t, fixture.dest(), "locally modified\n")
	if err := fixture.install(t, false, true); err == nil || !strings.Contains(err.Error(), "modified") {
		t.Fatalf("modified owned error = %v, want refusal", err)
	}
	writeInstallFile(t, fixture.dest(), fixture.renderedBody())
	if err := os.Remove(fixture.source()); err != nil {
		t.Fatalf("remove generated source: %v", err)
	}
	if err := fixture.install(t, false, true); err != nil {
		t.Fatalf("stale owned removal error = %v", err)
	}
	assertInstallPathMissing(t, fixture.dest())
	manifest := readCodexRuleManifestTest(t, fixture.manifest())
	if _, ok := manifest.ownedDigest(codexJournalRuleRelativePath); ok {
		t.Fatalf("manifest = %#v, want stale ownership removed", manifest)
	}
}

func TestInstallCodexJournalRuleDoesNotInferOwnershipFromMarker(t *testing.T) {
	fixture := newCodexRuleInstallFixture(t)
	writeInstallFile(t, filepath.Join(fixture.options(false, true).ConfigDir, loafInstallMarkerFile), "9.9.9\n")
	writeInstallFile(t, fixture.dest(), "user-owned\n")
	if err := installCodexJournalRule(fixture.options(false, true), fixture.codexHome); err != nil {
		t.Fatalf("upgrade with marker-only install error = %v", err)
	}
	assertInstallFile(t, fixture.dest(), "user-owned\n")
}

func TestInstallCodexJournalRuleRecoversInterruptedOwnershipWrites(t *testing.T) {
	t.Run("adopts exact generated body after first install interruption", func(t *testing.T) {
		fixture := newCodexRuleInstallFixture(t)
		writeInstallFile(t, fixture.dest(), fixture.renderedBody())
		if err := fixture.install(t, true, false); err != nil {
			t.Fatalf("adopt exact generated body: %v", err)
		}
		manifest := readCodexRuleManifestTest(t, fixture.manifest())
		if digest, ok := manifest.ownedDigest(codexJournalRuleRelativePath); !ok || digest != sha256Bytes([]byte(fixture.renderedBody())) {
			t.Fatalf("manifest = %#v, want adopted generated digest", manifest)
		}
	})

	t.Run("adopts exact guidance body after first install interruption", func(t *testing.T) {
		fixture := newCodexRuleInstallFixture(t)
		guidancePath := filepath.Join(fixture.codexHome, codexJournalGuidanceRelativePath)
		block := generateCodexJournalGuidance(filepath.Join(fixture.root, "trusted-bin", "loaf"))
		writeInstallFile(t, guidancePath, block)
		if err := fixture.install(t, true, false); err != nil {
			t.Fatalf("adopt exact guidance: %v", err)
		}
		manifest := readCodexRuleManifestTest(t, fixture.manifest())
		if digest, ok := manifest.ownedDigest(codexJournalGuidanceRelativePath); !ok || digest != sha256Bytes([]byte(block)) {
			t.Fatalf("manifest = %#v, want adopted guidance digest", manifest)
		}
	})

	t.Run("heals stale manifest after upgrade interruption", func(t *testing.T) {
		fixture := newCodexRuleInstallFixture(t)
		if err := fixture.install(t, true, false); err != nil {
			t.Fatalf("initial install: %v", err)
		}
		updated := fixture.sourceBody() + "# updated\n"
		writeFile(t, fixture.source(), updated)
		renderedUpdated, err := renderCodexJournalRule(updated, filepath.Join(fixture.root, "trusted-bin", "loaf"))
		if err != nil {
			t.Fatalf("render updated rule: %v", err)
		}
		writeInstallFile(t, fixture.dest(), renderedUpdated)
		if err := fixture.install(t, false, true); err != nil {
			t.Fatalf("heal stale manifest: %v", err)
		}
		manifest := readCodexRuleManifestTest(t, fixture.manifest())
		if digest, ok := manifest.ownedDigest(codexJournalRuleRelativePath); !ok || digest != sha256Bytes([]byte(renderedUpdated)) {
			t.Fatalf("manifest = %#v, want healed updated digest", manifest)
		}
	})
}

func TestInstallCodexJournalRuleManagesGlobalGuidanceAndPreservesUserContent(t *testing.T) {
	fixture := newCodexRuleInstallFixture(t)
	guidancePath := filepath.Join(fixture.codexHome, codexJournalGuidanceRelativePath)
	writeInstallFile(t, guidancePath, "# My Codex instructions\n\nKeep this text.\n")
	if err := fixture.installWithExecutable(t, filepath.Join(fixture.root, "trusted-bin", "loaf-v1"), true, false); err != nil {
		t.Fatalf("explicit install: %v", err)
	}
	body, err := os.ReadFile(guidancePath)
	if err != nil {
		t.Fatalf("read global guidance: %v", err)
	}
	if !strings.Contains(string(body), "Keep this text.") || !strings.Contains(string(body), journalContextShellQuote(filepath.Join(fixture.root, "trusted-bin", "loaf-v1"))+" journal log --execpolicy-safe") {
		t.Fatalf("global guidance = %q, want preserved user content and absolute command", body)
	}
	r, ok := findCodexJournalGuidance(string(body))
	if !ok {
		t.Fatal("global guidance missing managed block")
	}
	manifest := readCodexRuleManifestTest(t, fixture.manifest())
	if digest, ok := manifest.ownedDigest(codexJournalGuidanceRelativePath); !ok || digest != sha256Bytes([]byte(string(body)[r.start:r.end])) {
		t.Fatalf("guidance manifest = %#v, want managed block digest", manifest)
	}

	if err := fixture.installWithExecutable(t, filepath.Join(fixture.root, "trusted-bin", "loaf-v2"), false, true); err != nil {
		t.Fatalf("owned guidance update: %v", err)
	}
	body, err = os.ReadFile(guidancePath)
	if err != nil {
		t.Fatalf("read updated global guidance: %v", err)
	}
	if !strings.Contains(string(body), journalContextShellQuote(filepath.Join(fixture.root, "trusted-bin", "loaf-v2"))+" journal log --execpolicy-safe") || !strings.Contains(string(body), "Keep this text.") {
		t.Fatalf("updated global guidance = %q, want user content and new absolute command", body)
	}
}

func TestInstallCodexJournalRuleRefusesModifiedGlobalGuidance(t *testing.T) {
	fixture := newCodexRuleInstallFixture(t)
	if err := fixture.installWithExecutable(t, filepath.Join(fixture.root, "trusted-bin", "loaf-v1"), true, false); err != nil {
		t.Fatalf("explicit install: %v", err)
	}
	guidancePath := filepath.Join(fixture.codexHome, codexJournalGuidanceRelativePath)
	body, err := os.ReadFile(guidancePath)
	if err != nil {
		t.Fatalf("read guidance: %v", err)
	}
	modified := strings.Replace(string(body), "exact command", "locally changed command", 1)
	writeInstallFile(t, guidancePath, modified)
	if err := fixture.installWithExecutable(t, filepath.Join(fixture.root, "trusted-bin", "loaf-v2"), false, true); err == nil || !strings.Contains(err.Error(), "modified Loaf-owned Codex guidance") {
		t.Fatalf("modified guidance error = %v, want refusal", err)
	}
}

func TestInstallCodexJournalRuleRetiresRuleAndGuidanceWithoutResolvingExecutable(t *testing.T) {
	fixture := newCodexRuleInstallFixture(t)
	guidancePath := filepath.Join(fixture.codexHome, codexJournalGuidanceRelativePath)
	writeInstallFile(t, guidancePath, "# user guidance\n")
	if err := fixture.installWithExecutable(t, filepath.Join(fixture.root, "trusted-bin", "loaf-v1"), true, false); err != nil {
		t.Fatalf("explicit install: %v", err)
	}
	if err := os.Remove(fixture.source()); err != nil {
		t.Fatalf("remove template: %v", err)
	}
	noPath := &codexRuleInstallOperations{lookPath: func(string) (string, error) { return "", fmt.Errorf("PATH intentionally unavailable") }}
	if err := installCodexJournalRuleWithOperations(fixture.options(false, true), fixture.codexHome, noPath); err != nil {
		t.Fatalf("stale removal: %v", err)
	}
	assertInstallPathMissing(t, fixture.dest())
	body, err := os.ReadFile(guidancePath)
	if err != nil {
		t.Fatalf("read retained user guidance: %v", err)
	}
	if strings.Contains(string(body), codexJournalGuidanceStart) || strings.Contains(string(body), "Codex Auto journal capture") || !strings.Contains(string(body), "# user guidance") {
		t.Fatalf("global guidance = %q, want managed block removed", body)
	}
	manifest := readCodexRuleManifestTest(t, fixture.manifest())
	if len(manifest.Files) != 0 {
		t.Fatalf("manifest = %#v, want no remaining ownership", manifest)
	}
}

func TestRetirementRemovesOwnedRuleBeforeModifiedGuidanceConflict(t *testing.T) {
	fixture := newCodexRuleInstallFixture(t)
	if err := fixture.installWithExecutable(t, filepath.Join(fixture.root, "trusted-bin", "loaf-v1"), true, false); err != nil {
		t.Fatalf("explicit install: %v", err)
	}
	guidancePath := filepath.Join(fixture.codexHome, codexJournalGuidanceRelativePath)
	body, err := os.ReadFile(guidancePath)
	if err != nil {
		t.Fatalf("read guidance: %v", err)
	}
	writeInstallFile(t, guidancePath, strings.Replace(string(body), "exact command", "locally modified command", 1))
	if err := os.Remove(fixture.source()); err != nil {
		t.Fatalf("remove template: %v", err)
	}
	noPath := &codexRuleInstallOperations{lookPath: func(string) (string, error) { return "", fmt.Errorf("PATH intentionally unavailable") }}
	err = installCodexJournalRuleWithOperations(fixture.options(false, true), fixture.codexHome, noPath)
	if err == nil || !strings.Contains(err.Error(), "modified Loaf-owned Codex guidance") {
		t.Fatalf("retirement error = %v, want modified-guidance conflict", err)
	}
	assertInstallPathMissing(t, fixture.dest())
	manifest := readCodexRuleManifestTest(t, fixture.manifest())
	if _, ok := manifest.ownedDigest(codexJournalRuleRelativePath); ok {
		t.Fatalf("manifest = %#v, want retired rule ownership removed despite guidance conflict", manifest)
	}
	if _, ok := manifest.ownedDigest(codexJournalGuidanceRelativePath); !ok {
		t.Fatalf("manifest = %#v, want conflicted guidance ownership retained", manifest)
	}
}

func TestRetirementWithGuidanceOnlyOwnershipPreservesUserRuleBytes(t *testing.T) {
	fixture := newCodexRuleInstallFixture(t)
	if err := fixture.installWithExecutable(t, filepath.Join(fixture.root, "trusted-bin", "loaf-v1"), true, false); err != nil {
		t.Fatalf("explicit install: %v", err)
	}
	userRule := "# user-owned rule\nprefix_rule(pattern=[\"git\"], decision=\"allow\")\n"
	writeInstallFile(t, fixture.dest(), userRule)
	manifest := readCodexRuleManifestTest(t, fixture.manifest())
	manifest.remove(codexJournalRuleRelativePath)
	if err := writeCodexManagedRuleManifest(fixture.manifest(), manifest); err != nil {
		t.Fatalf("write guidance-only manifest: %v", err)
	}
	if err := os.Remove(fixture.source()); err != nil {
		t.Fatalf("remove template: %v", err)
	}
	noPath := &codexRuleInstallOperations{lookPath: func(string) (string, error) { return "", fmt.Errorf("PATH intentionally unavailable") }}
	if err := installCodexJournalRuleWithOperations(fixture.options(false, true), fixture.codexHome, noPath); err != nil {
		t.Fatalf("guidance-only retirement: %v", err)
	}
	assertInstallFile(t, fixture.dest(), userRule)
}

func TestCodexJournalGuidanceEditsPreserveUserBytesExactly(t *testing.T) {
	block := generateCodexJournalGuidance("/opt/loaf/bin/loaf")
	for _, original := range []string{
		"",
		"prefix \t\n\n  \n",
		"prefix \t  ",
		"\n\nprefix\n\nsuffix \t\n",
	} {
		appended := appendCodexJournalGuidance(original, block)
		if !strings.HasPrefix(appended, original) {
			t.Fatalf("append changed user prefix: original=%q appended=%q", original, appended)
		}
		r, ok := findCodexJournalGuidance(appended)
		if !ok {
			t.Fatalf("appended guidance missing managed range: %q", appended)
		}
		replaced := replaceCodexJournalGuidance(appended, r, generateCodexJournalGuidance("/opt/loaf/bin/loaf-v2"))
		if !strings.HasPrefix(replaced, original) {
			t.Fatalf("replace changed user bytes: original=%q replaced=%q", original, replaced)
		}
		r, ok = findCodexJournalGuidance(replaced)
		if !ok {
			t.Fatalf("replaced guidance missing managed range: %q", replaced)
		}
		removed := removeCodexJournalGuidance(replaced, r)
		if removed != original {
			t.Fatalf("remove changed user bytes: original=%q removed=%q", original, removed)
		}
	}
}

func TestRetirementPreservesPreexistingEmptyGuidanceFile(t *testing.T) {
	fixture := newCodexRuleInstallFixture(t)
	guidancePath := filepath.Join(fixture.codexHome, codexJournalGuidanceRelativePath)
	writeInstallFile(t, guidancePath, "")
	if err := fixture.installWithExecutable(t, filepath.Join(fixture.root, "trusted-bin", "loaf-v1"), true, false); err != nil {
		t.Fatalf("explicit install: %v", err)
	}
	if err := os.Remove(fixture.source()); err != nil {
		t.Fatalf("remove template: %v", err)
	}
	noPath := &codexRuleInstallOperations{lookPath: func(string) (string, error) { return "", fmt.Errorf("PATH intentionally unavailable") }}
	if err := installCodexJournalRuleWithOperations(fixture.options(false, true), fixture.codexHome, noPath); err != nil {
		t.Fatalf("retirement: %v", err)
	}
	body, err := os.ReadFile(guidancePath)
	if err != nil {
		t.Fatalf("read preserved empty guidance: %v", err)
	}
	if len(body) != 0 {
		t.Fatalf("preserved empty guidance = %q, want empty", body)
	}
}

func TestInstallCodexJournalRuleRetiresOrphanedOwnedGuidance(t *testing.T) {
	fixture := newCodexRuleInstallFixture(t)
	guidancePath := filepath.Join(fixture.codexHome, codexJournalGuidanceRelativePath)
	writeInstallFile(t, guidancePath, "# user guidance\n")
	if err := fixture.installWithExecutable(t, filepath.Join(fixture.root, "trusted-bin", "loaf-v1"), true, false); err != nil {
		t.Fatalf("explicit install: %v", err)
	}
	manifest := readCodexRuleManifestTest(t, fixture.manifest())
	manifest.remove(codexJournalRuleRelativePath)
	if err := writeCodexManagedRuleManifest(fixture.manifest(), manifest); err != nil {
		t.Fatalf("write orphaned manifest: %v", err)
	}
	if err := os.Remove(fixture.dest()); err != nil {
		t.Fatalf("remove installed rule: %v", err)
	}
	if err := os.Remove(fixture.source()); err != nil {
		t.Fatalf("remove template: %v", err)
	}
	noPath := &codexRuleInstallOperations{lookPath: func(string) (string, error) { return "", fmt.Errorf("PATH intentionally unavailable") }}
	if err := installCodexJournalRuleWithOperations(fixture.options(false, true), fixture.codexHome, noPath); err != nil {
		t.Fatalf("retire orphaned guidance: %v", err)
	}
	body, err := os.ReadFile(guidancePath)
	if err != nil {
		t.Fatalf("read retained user guidance: %v", err)
	}
	if strings.Contains(string(body), codexJournalGuidanceStart) || !strings.Contains(string(body), "# user guidance") {
		t.Fatalf("global guidance = %q, want only user guidance", body)
	}
}

func TestValidateCodexJournalGuidanceStructureRejectsMalformedOrDuplicateBlocks(t *testing.T) {
	block := generateCodexJournalGuidance("/usr/local/bin/loaf")
	for _, test := range []struct {
		name string
		body string
	}{
		{name: "start only", body: codexJournalGuidanceStart},
		{name: "end only", body: codexJournalGuidanceEnd},
		{name: "duplicate", body: block + "\n" + block},
		{name: "reversed", body: codexJournalGuidanceEnd + "\n" + codexJournalGuidanceStart},
		{name: "legacy reversed", body: codexLegacyGuidanceEnd + "\n" + codexLegacyGuidanceStart},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := validateCodexJournalGuidanceStructure(test.body); err == nil {
				t.Fatalf("validate malformed guidance = nil for %q", test.body)
			}
		})
	}
}

func TestFindCodexJournalGuidanceRecognizesLegacyJournalOnlyMarker(t *testing.T) {
	legacy := strings.Join([]string{codexLegacyGuidanceStart, "legacy journal-only guidance", codexLegacyGuidanceEnd}, "\n") + "\n"
	if err := validateCodexJournalGuidanceStructure(legacy); err != nil {
		t.Fatalf("validate legacy guidance: %v", err)
	}
	rangeValue, ok := findCodexJournalGuidance(legacy)
	if !ok || legacy[rangeValue.start:rangeValue.end] != legacy {
		t.Fatalf("legacy guidance range = %#v, found=%t, want complete block", rangeValue, ok)
	}
}

func TestInstallCodexBasicCommandsUpgradeRetiresLegacyJournalOnlyCapabilityWithoutOptIn(t *testing.T) {
	fixture := newCodexRuleInstallFixture(t)
	legacyRule := "prefix_rule(pattern=[\"/opt/legacy/loaf\", \"journal\", \"log\", \"--execpolicy-safe\"], decision=\"allow\")\n"
	legacyGuidance := strings.Join([]string{codexLegacyGuidanceStart, "legacy journal-only guidance", codexLegacyGuidanceEnd}, "\n") + "\n"
	writeInstallFile(t, fixture.dest(), legacyRule)
	writeInstallFile(t, filepath.Join(fixture.codexHome, codexJournalGuidanceRelativePath), legacyGuidance)
	manifest := codexManagedRuleManifest{Version: 1}
	manifest.set(codexJournalRuleRelativePath, sha256Bytes([]byte(legacyRule)))
	manifest.set(codexJournalGuidanceRelativePath, sha256Bytes([]byte(legacyGuidance)))
	if err := writeCodexManagedRuleManifest(fixture.manifest(), manifest); err != nil {
		t.Fatalf("write legacy manifest: %v", err)
	}
	if err := fixture.install(t, false, true); err != nil {
		t.Fatalf("legacy upgrade retirement: %v", err)
	}
	assertInstallPathMissing(t, fixture.dest())
	body, err := os.ReadFile(filepath.Join(fixture.codexHome, codexJournalGuidanceRelativePath))
	if err != nil {
		t.Fatalf("read retired guidance: %v", err)
	}
	if strings.Contains(string(body), codexLegacyGuidanceStart) || strings.Contains(string(body), codexLegacyGuidanceEnd) {
		t.Fatalf("retired guidance = %q, want legacy block removed", body)
	}
}

func TestReadCodexManagedRuleManifestRejectsDuplicatePaths(t *testing.T) {
	path := filepath.Join(t.TempDir(), codexJournalRuleManifest)
	digest := strings.Repeat("a", sha256.Size*2)
	body := fmt.Sprintf(`{"version":1,"files":[{"path":"loaf.rules","sha256":"%s"},{"path":"loaf.rules","sha256":"%s"}]}`, digest, digest)
	writeInstallFile(t, path, body)
	if _, err := readCodexManagedRuleManifest(path); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("duplicate manifest error = %v, want refusal", err)
	}
}

type codexRuleInstallFixture struct {
	root      string
	codexHome string
	dist      string
}

func newCodexRuleInstallFixture(t *testing.T) codexRuleInstallFixture {
	t.Helper()
	root := realpath(t, t.TempDir())
	dist := filepath.Join(root, "dist", "codex")
	source := filepath.Join(dist, ".codex", "rules", codexJournalRuleTemplateRelativePath)
	writeInstallFile(t, source, "# Loaf Codex policy\n{{LOAF_BASIC_RULES}}\n")
	if err := os.MkdirAll(filepath.Join(root, "project"), 0o755); err != nil {
		t.Fatalf("create project fixture: %v", err)
	}
	return codexRuleInstallFixture{root: root, codexHome: filepath.Join(root, "codex-home"), dist: dist}
}

func (f codexRuleInstallFixture) options(autoJournal bool, upgrade bool) targetInstallOptions {
	return targetInstallOptions{Target: "codex", DistDir: f.dist, ConfigDir: filepath.Join(f.root, "reported-config"), CodexHome: f.codexHome, HomeDir: filepath.Join(f.root, "home"), ProjectRoot: filepath.Join(f.root, "project"), CodexBasicCommands: autoJournal, Upgrade: upgrade, Version: "9.9.9"}
}

func (f codexRuleInstallFixture) source() string {
	return filepath.Join(f.dist, ".codex", "rules", codexJournalRuleTemplateRelativePath)
}

func (f codexRuleInstallFixture) sourceBody() string {
	body, err := os.ReadFile(f.source())
	if err != nil {
		return ""
	}
	return string(body)
}

func (f codexRuleInstallFixture) renderedBody() string {
	body, err := renderCodexJournalRule(f.sourceBody(), filepath.Join(f.root, "trusted-bin", "loaf"))
	if err != nil {
		return ""
	}
	return body
}

func (f codexRuleInstallFixture) dest() string {
	return filepath.Join(f.codexHome, "rules", codexJournalRuleRelativePath)
}

func (f codexRuleInstallFixture) manifest() string {
	return filepath.Join(f.codexHome, "rules", codexJournalRuleManifest)
}

func (f codexRuleInstallFixture) install(t *testing.T, autoJournal bool, upgrade bool) error {
	t.Helper()
	return f.installWithExecutable(t, filepath.Join(f.root, "trusted-bin", "loaf"), autoJournal, upgrade)
}

func (f codexRuleInstallFixture) installWithExecutable(t *testing.T, path string, autoJournal bool, upgrade bool) error {
	t.Helper()
	writeInstallFile(t, path, "#!/bin/sh\nexit 0\n")
	if err := os.Chmod(path, 0o755); err != nil {
		t.Fatalf("chmod trusted fixture: %v", err)
	}
	operations := &codexRuleInstallOperations{
		lookPath: func(name string) (string, error) {
			if name != "loaf" {
				return "", fmt.Errorf("unexpected executable %q", name)
			}
			return path, nil
		},
		forbiddenRoots: []string{filepath.Join(f.root, "project")},
	}
	return installCodexJournalRuleWithOperations(f.options(autoJournal, upgrade), f.codexHome, operations)
}

func readCodexRuleManifestTest(t *testing.T, path string) codexManagedRuleManifest {
	t.Helper()
	manifest, err := readCodexManagedRuleManifest(path)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	return manifest
}
