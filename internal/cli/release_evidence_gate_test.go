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
		"re-record against the merged tree",
		"single evidence-only commit",
		"rerun loaf release --post-merge",
	} {
		if !strings.Contains(result.message, want) {
			t.Fatalf("message = %q, want %q", result.message, want)
		}
	}
	for _, forbidden := range []string{"tag -d", "re-point", "repoint"} {
		if strings.Contains(result.message, forbidden) {
			t.Fatalf("message = %q, must not contain %q", result.message, forbidden)
		}
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

	t.Run("apply refusal names the runners and the executable resume loop", func(t *testing.T) {
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
			"prepared tree stays in place",
			"rerun the release",
			"release-prepared worktree",
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
			"re-record against the merged tree",
			"single evidence-only commit",
			"rerun loaf release --post-merge",
		} {
			if !strings.Contains(msg, want) {
				t.Fatalf("message = %q, want %q", msg, want)
			}
		}
		for _, forbidden := range []string{"tag -d", "delete", "re-point", "repoint"} {
			if strings.Contains(msg, forbidden) {
				t.Fatalf("message = %q, must not contain %q", msg, forbidden)
			}
		}
	})
}

// rewriteInstalledSmokeReceiptHashes sets each installed-smoke receipt's pinned
// artifact digests to match the files currently on disk under root — the
// mechanical stand-in for re-recording after a refused release rebuild.
func rewriteInstalledSmokeReceiptHashes(t *testing.T, root string) {
	t.Helper()
	registry, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(TargetCapabilityEvidenceRecordPath)))
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", TargetCapabilityEvidenceRecordPath, err)
	}
	contract, err := DecodeTargetCapabilityEvidence(registry)
	if err != nil {
		t.Fatalf("DecodeTargetCapabilityEvidence() error = %v", err)
	}
	rewritten := map[string]bool{}
	rewriteReceipt := func(source string) {
		relative, err := safeEvidenceRelativePath(source)
		if err != nil || rewritten[relative] {
			return
		}
		if filepath.Ext(relative) != ".json" {
			return
		}
		path := filepath.Join(root, filepath.FromSlash(relative))
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile(%s) error = %v", relative, err)
		}
		var raw map[string]any
		if err := json.Unmarshal(data, &raw); err != nil {
			return
		}
		artifacts, _ := raw["candidate_artifacts"].(map[string]any)
		if artifacts == nil {
			return
		}
		for field, key := range map[string]string{"hooks_path": "hooks_sha256", "native_binary_path": "native_binary_sha256"} {
			rel, _ := artifacts[field].(string)
			if rel == "" {
				continue
			}
			digest, err := sha256File(filepath.Join(root, filepath.FromSlash(rel)))
			if err != nil {
				// Native binaries are gitignored in the fixture; leave pinned.
				continue
			}
			artifacts[key] = digest
		}
		encoded, err := json.MarshalIndent(raw, "", "  ")
		if err != nil {
			t.Fatalf("MarshalIndent(%s) error = %v", relative, err)
		}
		if err := os.WriteFile(path, append(encoded, '\n'), 0o644); err != nil {
			t.Fatalf("WriteFile(%s) error = %v", relative, err)
		}
		rewritten[relative] = true
	}
	for _, record := range contract.Records {
		for _, mode := range record.Context.Modes {
			if mode.Evidence.Level == "installed-smoke" {
				rewriteReceipt(mode.Evidence.Source)
			}
		}
	}
}

func TestReleaseApplyResumesPreparedTreeAfterEvidenceRerecord(t *testing.T) {
	repo := seedReleaseApplyRepoWithCapabilityEvidence(t, "feat: first live evidence-gate resume")
	// Idempotent rebuild that stales the OpenCode receipt once, then stays put
	// on resume — mirrors the version-stamp staleness without compounding.
	packageBody := strings.Join([]string{
		"{",
		`  "name": "release-fixture",`,
		`  "version": "1.0.0",`,
		`  "scripts": {`,
		`    "build": "node -e \"require('fs').mkdirSync('dist/opencode/plugins',{recursive:true}); require('fs').writeFileSync('dist/opencode/plugins/hooks.ts','staled-hooks\\n')\""`,
		"  }",
		"}",
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Join(repo, "package.json"), []byte(packageBody), 0o644); err != nil {
		t.Fatal(err)
	}
	gitCLI(t, repo, "add", "package.json")
	gitCLI(t, repo, "commit", "-m", "fix: make rebuild stale OpenCode hooks once")

	args := []string{"release", "--yes", "--no-gh"}
	first := (Runner{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}, WorkingDir: repo}).Run(args)
	if first == nil {
		t.Fatalf("first Run(%v) error = nil, want stale-evidence refusal", args)
	}
	if !strings.Contains(first.Error(), "Refusing to commit release artifacts") || !strings.Contains(first.Error(), "prepared tree stays in place") {
		t.Fatalf("first Run(%v) error = %q, want resume-loop refusal copy", args, first.Error())
	}
	beforeHEAD := gitOutputReleaseTest(t, repo, "rev-parse", "HEAD")
	if dirty := gitOutputReleaseTest(t, repo, "status", "--porcelain"); dirty == "" {
		t.Fatal("first refusal left a clean worktree, want prepared dirt")
	}

	// Operator re-records against the rebuilt tree (prepared tree stays).
	rewriteInstalledSmokeReceiptHashes(t, repo)

	var stdout bytes.Buffer
	if err := (Runner{Stdout: &stdout, Stderr: &bytes.Buffer{}, WorkingDir: repo}).Run(args); err != nil {
		t.Fatalf("resume Run(%v) error = %v\n%s", args, err, stdout.String())
	}
	if !strings.Contains(stdout.String(), "Capability evidence validated") {
		t.Fatalf("stdout = %q, want evidence validation on resume", stdout.String())
	}
	if head := gitOutputReleaseTest(t, repo, "rev-parse", "HEAD"); head == beforeHEAD {
		t.Fatal("resume did not create a release commit")
	}
	if subject := gitOutputReleaseTest(t, repo, "log", "-1", "--pretty=%s"); subject != "chore: release v1.1.0" {
		t.Fatalf("release commit subject = %q, want chore: release v1.1.0", subject)
	}
	if tags := gitOutputReleaseTest(t, repo, "tag", "--list"); !strings.Contains(tags, "v1.1.0") {
		t.Fatalf("tags = %q, want v1.1.0 after resume with tagging enabled", tags)
	}
	if dirty := gitOutputReleaseTest(t, repo, "status", "--porcelain"); dirty != "" {
		t.Fatalf("resume left dirty worktree: %q", dirty)
	}
}

func TestReleaseApplyRefusesPreparedTreeWithUnrelatedDirty(t *testing.T) {
	repo := seedReleaseApplyRepoWithCapabilityEvidence(t, "feat: resume boundary keeps non-release dirt out")
	packageBody := strings.Join([]string{
		"{",
		`  "name": "release-fixture",`,
		`  "version": "1.0.0",`,
		`  "scripts": {`,
		`    "build": "node -e \"require('fs').mkdirSync('dist/opencode/plugins',{recursive:true}); require('fs').writeFileSync('dist/opencode/plugins/hooks.ts','staled-hooks\\n')\""`,
		"  }",
		"}",
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Join(repo, "package.json"), []byte(packageBody), 0o644); err != nil {
		t.Fatal(err)
	}
	gitCLI(t, repo, "add", "package.json")
	gitCLI(t, repo, "commit", "-m", "fix: stale hooks on rebuild")

	args := []string{"release", "--yes", "--no-tag", "--no-gh"}
	if err := (Runner{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}, WorkingDir: repo}).Run(args); err == nil {
		t.Fatalf("first Run(%v) error = nil, want stale-evidence refusal", args)
	}
	rewriteInstalledSmokeReceiptHashes(t, repo)
	writeFile(t, filepath.Join(repo, "unrelated.txt"), "not part of the release\n")

	err := (Runner{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}, WorkingDir: repo}).Run(args)
	if err == nil {
		t.Fatalf("resume with unrelated dirt error = nil, want clean-worktree refusal")
	}
	msg := err.Error()
	if !strings.Contains(msg, "require a clean unignored worktree") || !strings.Contains(msg, "unrelated.txt") {
		t.Fatalf("error = %q, want clean-worktree refusal naming unrelated.txt", msg)
	}
}

func TestReleasePostMergeEvidenceOnlyRepairPasses(t *testing.T) {
	repo := seedReleasePostMergeFiles(t, "1.2.3")
	seedReleaseCapabilityEvidence(t, repo)
	receipt := "docs/changes/20260710-journal-reliability-foundation/research/opencode-1.18.7-isolated-request-smoke.json"
	responses := releasePostMergeHappyResponses("1.2.3")
	// Detect repair via HEAD^..HEAD evidence-only diff; subject + release shape
	// come from the parent release commit; tag still lands on HEAD.
	responses["git rev-parse --verify HEAD^"] = releasePostMergeOK("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	responses["git diff HEAD^ HEAD --name-only"] = releasePostMergeOK("config/target-capabilities.json\n" + receipt)
	responses["git log -1 --pretty=%s HEAD^"] = releasePostMergeOK("chore: release v1.2.3 (#42)")
	delete(responses, "git log -1 --pretty=%s")
	responses["git diff HEAD~2 HEAD~1 --name-only"] = releasePostMergeOK("CHANGELOG.md\npackage.json")

	runner, calls := scriptedReleasePostMergeRunner(responses)
	snap := mustResolveReleaseSnapshot(t, repo, releaseOptions{postMerge: true})
	result := checkReleasePostMergeGuardrails(repo, snap, runner)
	if !result.ok {
		t.Fatalf("result = %#v, want evidence-only repair to pass guardrails", result)
	}
	if result.featureBranch != "feat/cool-thing" {
		t.Fatalf("featureBranch = %q, want PR branch extracted from release subject at HEAD^", result.featureBranch)
	}

	var out, errOut bytes.Buffer
	if err := runReleasePostMergeWithRunner(repo, snap, &out, &errOut, runner); err != nil {
		t.Fatalf("runReleasePostMergeWithRunner error = %v\n%s\n%s", err, out.String(), errOut.String())
	}
	keys := releasePostMergeCallKeys(calls())
	tagged := false
	for _, key := range keys {
		if strings.HasPrefix(key, "git tag -s v1.2.3") {
			tagged = true
		}
	}
	if !tagged {
		t.Fatalf("calls = %v, want tag created on HEAD after evidence-only repair", keys)
	}
}

func TestReleasePostMergeNonEvidenceRepairStillFailsDiffShape(t *testing.T) {
	repo := seedReleasePostMergeFiles(t, "1.2.3")
	seedReleaseCapabilityEvidence(t, repo)
	receipt := "docs/changes/20260710-journal-reliability-foundation/research/opencode-1.18.7-isolated-request-smoke.json"
	responses := releasePostMergeHappyResponses("1.2.3")
	responses["git rev-parse --verify HEAD^"] = releasePostMergeOK("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	// Touches a version file → not evidence-only; guardrail 5 evaluates HEAD.
	responses["git diff HEAD^ HEAD --name-only"] = releasePostMergeOK(receipt + "\nREADME.md")
	runner, _ := scriptedReleasePostMergeRunner(responses)
	snap := mustResolveReleaseSnapshot(t, repo, releaseOptions{postMerge: true})
	result := checkReleasePostMergeGuardrails(repo, snap, runner)
	if result.ok || result.guardrail != 5 {
		t.Fatalf("result = %#v, want guardrail 5 failure for non-evidence repair", result)
	}
}

func TestReleasePostMergeDirectReleaseCommitStillPasses(t *testing.T) {
	repo := seedReleasePostMergeFiles(t, "1.2.3")
	seedReleaseCapabilityEvidence(t, repo)
	// No HEAD^ rev-parse success → not a repair; default happy responses.
	runner, _ := scriptedReleasePostMergeRunner(releasePostMergeHappyResponses("1.2.3"))
	snap := mustResolveReleaseSnapshot(t, repo, releaseOptions{postMerge: true})
	result := checkReleasePostMergeGuardrails(repo, snap, runner)
	if !result.ok {
		t.Fatalf("result = %#v, want direct release commit path unchanged", result)
	}
}

func TestCheckReleaseCapabilityEvidenceSymlinkRefuses(t *testing.T) {
	t.Run("dangling symlink is present but unusable", func(t *testing.T) {
		root := t.TempDir()
		if err := os.MkdirAll(filepath.Join(root, "config"), 0o755); err != nil {
			t.Fatal(err)
		}
		link := filepath.Join(root, filepath.FromSlash(TargetCapabilityEvidenceRecordPath))
		if err := os.Symlink(filepath.Join(root, "config", "missing-target.json"), link); err != nil {
			t.Fatal(err)
		}
		present, err := checkReleaseCapabilityEvidence(root)
		if !present {
			t.Fatal("dangling symlink classified as absent; want present")
		}
		if err == nil || !strings.Contains(err.Error(), "not a regular file") {
			t.Fatalf("error = %v, want not-a-regular-file refusal", err)
		}
	})

	t.Run("symlink to a valid regular file still refuses", func(t *testing.T) {
		root := t.TempDir()
		if err := os.MkdirAll(filepath.Join(root, "config"), 0o755); err != nil {
			t.Fatal(err)
		}
		target := filepath.Join(root, "config", "external.json")
		// Minimal body — content is irrelevant; the probe must refuse before load.
		if err := os.WriteFile(target, []byte(`{"contract_version":3,"records":[],"deferred":[{"target":"pi","status":"deferred","not_a_build_target":true,"reason":"deferred"}]`+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		link := filepath.Join(root, filepath.FromSlash(TargetCapabilityEvidenceRecordPath))
		if err := os.Symlink(target, link); err != nil {
			t.Fatal(err)
		}
		present, err := checkReleaseCapabilityEvidence(root)
		if !present {
			t.Fatal("symlink to valid file classified as absent; want present")
		}
		if err == nil || !strings.Contains(err.Error(), "not a regular file") {
			t.Fatalf("error = %v, want not-a-regular-file refusal", err)
		}
	})

	t.Run("absent remains a silent no-op", func(t *testing.T) {
		present, err := checkReleaseCapabilityEvidence(t.TempDir())
		if present || err != nil {
			t.Fatalf("present=%v err=%v, want absent no-op", present, err)
		}
	})
}
