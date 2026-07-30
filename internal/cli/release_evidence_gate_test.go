package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode"
)

// seedReleaseCapabilityEvidence copies the repository's real capability
// evidence registry plus every file it references — evidence sources,
// installed-smoke receipts, and the pinned artifacts those receipts hash —
// into root, so the full evidence loader passes against that tree.
func seedReleaseCapabilityEvidence(t *testing.T, root string) {
	t.Helper()
	repoRoot := testRepositoryRoot(t)
	registry, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(TargetCapabilityEvidenceRecordPath)))
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", TargetCapabilityEvidenceRecordPath, err)
	}
	contract, err := DecodeTargetCapabilityEvidence(registry)
	if err != nil {
		t.Fatalf("DecodeTargetCapabilityEvidence() error = %v", err)
	}
	copied := map[string]bool{}
	copyPath := func(relative string) {
		relative = filepath.ToSlash(relative)
		if relative == "" || copied[relative] {
			return
		}
		copied[relative] = true
		content, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(relative)))
		if err != nil {
			t.Fatalf("ReadFile(%s) error = %v", relative, err)
		}
		destination := filepath.Join(root, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
			t.Fatalf("MkdirAll(%s) error = %v", filepath.Dir(destination), err)
		}
		if err := os.WriteFile(destination, content, 0o644); err != nil {
			t.Fatalf("WriteFile(%s) error = %v", destination, err)
		}
	}
	copyEvidence := func(evidence TargetCapabilityEvidenceRecord) {
		relative, err := safeEvidenceRelativePath(evidence.Source)
		if err != nil {
			t.Fatalf("safeEvidenceRelativePath(%q) error = %v", evidence.Source, err)
		}
		copyPath(relative)
		if evidence.Level != "installed-smoke" {
			return
		}
		receipt, err := os.ReadFile(filepath.Join(repoRoot, relative))
		if err != nil {
			t.Fatalf("ReadFile(%s) error = %v", relative, err)
		}
		var smoke struct {
			CandidateArtifacts TargetCapabilitySmokeArtifacts `json:"candidate_artifacts"`
		}
		if err := json.Unmarshal(receipt, &smoke); err != nil {
			t.Fatalf("Unmarshal(%s) error = %v", relative, err)
		}
		copyPath(smoke.CandidateArtifacts.HooksPath)
		copyPath(smoke.CandidateArtifacts.NativeBinaryPath)
	}
	copyPath(TargetCapabilityEvidenceRecordPath)
	for _, record := range contract.Records {
		for _, mode := range record.Context.Modes {
			copyEvidence(mode.Evidence)
		}
		copyEvidence(record.Completion.Evidence)
	}
}

// seedReleaseApplyRepoWithCapabilityEvidence extends the apply fixture with
// the real capability evidence tree, committed so the release preflight sees
// a clean worktree. Native binaries stay ignored: the gate reads the
// filesystem, and keeping ~26MB blobs out of git keeps the fixture fast.
func seedReleaseApplyRepoWithCapabilityEvidence(t *testing.T, commitSubject string) string {
	t.Helper()
	repo := seedReleaseApplyRepo(t, commitSubject)
	seedReleaseCapabilityEvidence(t, repo)
	writeFile(t, filepath.Join(repo, ".gitignore"), "bin/native/\nplugins/loaf/bin/native/\n")
	gitCLI(t, repo, "add", ".")
	gitCLI(t, repo, "commit", "-m", "chore: record capability evidence")
	return repo
}

func TestReleaseApplyBlocksWhenCapabilityEvidenceStale(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{name: "direct", args: []string{"release", "--yes", "--no-gh"}},
		{name: "pre-merge", args: []string{"release", "--pre-merge", "--base", "HEAD~1", "--yes", "--no-gh"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := seedReleaseApplyRepoWithCapabilityEvidence(t, "feat: gate stale capability evidence")
			// Reproduce the alpha.16/alpha.17 incident: the artifact rebuild
			// itself stales a SHA-pinned receipt.
			packageBody := strings.Join([]string{
				"{",
				`  "name": "release-fixture",`,
				`  "version": "1.0.0",`,
				`  "scripts": {`,
				`    "build": "node -e \"require('fs').appendFileSync('dist/opencode/plugins/hooks.ts','stale')\""`,
				"  }",
				"}",
				"",
			}, "\n")
			if err := os.WriteFile(filepath.Join(repo, "package.json"), []byte(packageBody), 0o644); err != nil {
				t.Fatal(err)
			}
			gitCLI(t, repo, "add", "package.json")
			gitCLI(t, repo, "commit", "-m", "fix: stale a pinned artifact during rebuild")
			beforeHEAD := gitOutputReleaseTest(t, repo, "rev-parse", "HEAD")

			err := (Runner{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}, WorkingDir: repo}).Run(tc.args)
			if err == nil {
				t.Fatalf("Run(%v) error = nil, want stale-evidence refusal", tc.args)
			}
			msg := err.Error()
			for _, want := range []string{
				"Refusing to commit release artifacts",
				"capability evidence is invalid or stale",
				"does not match current candidate",
				"cli/scripts/smoke-claude-code-startup.mjs",
				"cli/scripts/smoke-codex-startup.mjs",
				"cli/scripts/smoke-opencode-request-context.mjs",
				"after the artifact rebuild",
			} {
				if !strings.Contains(msg, want) {
					t.Fatalf("Run(%v) error = %q, want %q", tc.args, msg, want)
				}
			}
			if staged := gitOutputReleaseTest(t, repo, "diff", "--cached", "--name-only"); staged != "" {
				t.Fatalf("Run(%v) staged files before refusal: %q", tc.args, staged)
			}
			if head := gitOutputReleaseTest(t, repo, "rev-parse", "HEAD"); head != beforeHEAD {
				t.Fatalf("Run(%v) created release commit %s, want HEAD %s", tc.args, head, beforeHEAD)
			}
			if tags := gitOutputReleaseTest(t, repo, "tag", "--list"); tags != "v1.0.0" {
				t.Fatalf("Run(%v) tags = %q, want only v1.0.0", tc.args, tags)
			}
		})
	}
}

func TestReleaseApplyBlocksWhenCapabilityEvidenceInvalid(t *testing.T) {
	repo := seedReleaseApplyRepo(t, "feat: gate invalid capability evidence")
	if err := os.MkdirAll(filepath.Join(repo, "config"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(repo, filepath.FromSlash(TargetCapabilityEvidenceRecordPath)), `{"contract_version": 1}`+"\n")
	gitCLI(t, repo, "add", ".")
	gitCLI(t, repo, "commit", "-m", "chore: record broken capability evidence")
	beforeHEAD := gitOutputReleaseTest(t, repo, "rev-parse", "HEAD")

	err := (Runner{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}, WorkingDir: repo}).Run([]string{"release", "--yes", "--no-tag", "--no-gh"})
	if err == nil {
		t.Fatalf("release error = nil, want invalid-evidence refusal")
	}
	if !strings.Contains(err.Error(), "Refusing to commit release artifacts") || !strings.Contains(err.Error(), "capability evidence is invalid or stale") {
		t.Fatalf("release error = %q, want invalid-evidence refusal copy", err.Error())
	}
	if !strings.Contains(err.Error(), "unsupported target capability contract version") {
		t.Fatalf("release error = %q, want the loader error surfaced", err.Error())
	}
	if head := gitOutputReleaseTest(t, repo, "rev-parse", "HEAD"); head != beforeHEAD {
		t.Fatalf("release created commit %s, want HEAD %s", head, beforeHEAD)
	}
}

func TestReleaseApplyPassesWithFreshCapabilityEvidence(t *testing.T) {
	repo := seedReleaseApplyRepoWithCapabilityEvidence(t, "feat: release with fresh capability evidence")
	var stdout bytes.Buffer

	err := Runner{Stdout: &stdout, WorkingDir: repo}.Run([]string{"release", "--yes", "--no-tag", "--no-gh"})
	if err != nil {
		t.Fatalf("release error = %v\n%s", err, stdout.String())
	}
	if !strings.Contains(stdout.String(), "Capability evidence validated") {
		t.Fatalf("stdout = %q, want inline evidence validation report", stdout.String())
	}
	if subject := gitOutputReleaseTest(t, repo, "log", "-1", "--pretty=%s"); subject != "chore: release v1.1.0" {
		t.Fatalf("release commit subject = %q, want chore: release v1.1.0", subject)
	}
}

func TestReleaseApplySkipsCapabilityEvidenceWhenAbsent(t *testing.T) {
	repo := seedReleaseApplyRepo(t, "feat: release without capability evidence")
	var stdout bytes.Buffer

	err := Runner{Stdout: &stdout, WorkingDir: repo}.Run([]string{"release", "--yes", "--no-tag", "--no-gh"})
	if err != nil {
		t.Fatalf("release error = %v\n%s", err, stdout.String())
	}
	if strings.Contains(stdout.String(), "Capability evidence") {
		t.Fatalf("stdout = %q, want no evidence output for a project without the config", stdout.String())
	}
	if subject := gitOutputReleaseTest(t, repo, "log", "-1", "--pretty=%s"); subject != "chore: release v1.1.0" {
		t.Fatalf("release commit subject = %q, want chore: release v1.1.0", subject)
	}
}

func TestReleasePostMergeGuardrailBlocksStaleCapabilityEvidence(t *testing.T) {
	repo := seedReleasePostMergeFiles(t, "1.2.3")
	seedReleaseCapabilityEvidence(t, repo)
	staleArtifact := filepath.Join(repo, "dist", "opencode", "plugins", "hooks.ts")
	handle, err := os.OpenFile(staleArtifact, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := handle.WriteString("stale"); err != nil {
		t.Fatal(err)
	}
	if err := handle.Close(); err != nil {
		t.Fatal(err)
	}
	runner, _ := scriptedReleasePostMergeRunner(releasePostMergeHappyResponses("1.2.3"))
	snap := mustResolveReleaseSnapshot(t, repo, releaseOptions{postMerge: true})

	result := checkReleasePostMergeGuardrails(repo, snap, runner)
	if result.ok || result.guardrail != 9 {
		t.Fatalf("result = %#v, want guardrail 9 failure", result)
	}
	for _, want := range []string{
		"capability evidence is invalid or stale",
		"does not match current candidate",
		"cli/scripts/smoke-opencode-request-context.mjs",
		"after the artifact rebuild",
	} {
		if !strings.Contains(result.message, want) {
			t.Fatalf("message = %q, want %q", result.message, want)
		}
	}
	if strings.Contains(result.message, "tag -d") {
		t.Fatalf("message = %q, must never advise tag deletion", result.message)
	}
}

func TestReleasePostMergeGuardrailPassesFreshCapabilityEvidence(t *testing.T) {
	repo := seedReleasePostMergeFiles(t, "1.2.3")
	seedReleaseCapabilityEvidence(t, repo)
	runner, _ := scriptedReleasePostMergeRunner(releasePostMergeHappyResponses("1.2.3"))
	snap := mustResolveReleaseSnapshot(t, repo, releaseOptions{postMerge: true})

	result := checkReleasePostMergeGuardrails(repo, snap, runner)
	if !result.ok {
		t.Fatalf("result = %#v, want all guardrails passed with fresh evidence", result)
	}
}

func TestReleaseCapabilityEvidenceRemediation(t *testing.T) {
	loaderErr := errors.New(`load target capability evidence "config/target-capabilities.json": OpenCode installed-smoke hooks SHA-256 aaa does not match current candidate bbb`)

	t.Run("apply refusal names the runners and the ordering rule", func(t *testing.T) {
		msg := releaseApplyCapabilityEvidenceRefusal(loaderErr).Error()
		for _, want := range []string{
			"Refusing to commit release artifacts:",
			"capability evidence is invalid or stale",
			loaderErr.Error(),
			"cli/scripts/smoke-claude-code-startup.mjs",
			"cli/scripts/smoke-codex-startup.mjs",
			"cli/scripts/smoke-opencode-request-context.mjs",
			"--client",
			"--expected-version",
			"--receipt",
			"after the artifact rebuild",
			"rerun the release",
		} {
			if !strings.Contains(msg, want) {
				t.Fatalf("message = %q, want %q", msg, want)
			}
		}
		if strings.Contains(msg, "go test") {
			t.Fatalf("message = %q, must not point at the Go test harness", msg)
		}
	})

	t.Run("post-merge guardrail message keeps the lowercase register", func(t *testing.T) {
		msg := releasePostMergeCapabilityEvidenceAbortMessage(loaderErr)
		if msg == "" || !unicode.IsLower(rune(msg[0])) {
			t.Fatalf("message = %q, want lowercase guardrail register", msg)
		}
		for _, want := range []string{
			"capability evidence is invalid or stale",
			loaderErr.Error(),
			" — ",
			"cli/scripts/smoke-claude-code-startup.mjs",
			"cli/scripts/smoke-codex-startup.mjs",
			"cli/scripts/smoke-opencode-request-context.mjs",
			"after the artifact rebuild",
		} {
			if !strings.Contains(msg, want) {
				t.Fatalf("message = %q, want %q", msg, want)
			}
		}
		for _, forbidden := range []string{"tag -d", "delete"} {
			if strings.Contains(msg, forbidden) {
				t.Fatalf("message = %q, must not contain %q", msg, forbidden)
			}
		}
	})
}
