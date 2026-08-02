package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"unicode"
)

// hardenFixtureRepoAgainstHostSigning makes a fixture git repo independent of
// host signing config. Matches seedReleaseApplyRepo: local commit.gpgsign and
// tag.gpgsign false. Also installs an ephemeral SSH signing key so production
// `git tag -s` succeeds without a host secret key — tag.gpgsign=false does not
// suppress -s, and CI has no key while local signing agents mask the gap.
func hardenFixtureRepoAgainstHostSigning(t *testing.T, repo string) {
	t.Helper()
	gitCLI(t, repo, "config", "commit.gpgsign", "false")
	gitCLI(t, repo, "config", "tag.gpgsign", "false")

	keyDir := t.TempDir()
	keyPath := filepath.Join(keyDir, "signing_key")
	cmd := exec.Command("ssh-keygen", "-t", "ed25519", "-N", "", "-f", keyPath, "-C", "loaf-test@example.test", "-q")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("ssh-keygen for fixture signing key: %v\n%s", err, out)
	}
	gitCLI(t, repo, "config", "gpg.format", "ssh")
	gitCLI(t, repo, "config", "user.signingkey", keyPath)
}

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
	hardenFixtureRepoAgainstHostSigning(t, repo)
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
	hardenFixtureRepoAgainstHostSigning(t, repo)
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
	hardenFixtureRepoAgainstHostSigning(t, repo)
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
			"version files remain at the candidate",
			"CHANGELOG.md is restored to HEAD",
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

// seedReleaseApplyRepoWithStalingBuild commits a package.json whose build
// rewrites dist/opencode/plugins/hooks.ts to a fixed body, staling the
// OpenCode installed-smoke receipt on the first rebuild.
func seedReleaseApplyRepoWithStalingBuild(t *testing.T, commitSubject string) string {
	t.Helper()
	repo := seedReleaseApplyRepoWithCapabilityEvidence(t, commitSubject)
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
	return repo
}

func countChangelogHeadings(body, version string) int {
	want := "## [" + version + "]"
	count := 0
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), want) {
			count++
		}
	}
	return count
}

func releaseCommitChangedPaths(t *testing.T, repo string) []string {
	t.Helper()
	out := gitOutputReleaseTest(t, repo, "diff-tree", "--no-commit-id", "--name-only", "-r", "HEAD")
	if out == "" {
		return nil
	}
	return strings.Split(out, "\n")
}

func TestReleaseApplyResumesPreparedTreeAfterEvidenceRerecord(t *testing.T) {
	repo := seedReleaseApplyRepoWithStalingBuild(t, "feat: first live evidence-gate resume")

	args := []string{"release", "--yes", "--no-gh"}
	first := (Runner{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}, WorkingDir: repo}).Run(args)
	if first == nil {
		t.Fatalf("first Run(%v) error = nil, want stale-evidence refusal", args)
	}
	if !strings.Contains(first.Error(), "Refusing to commit release artifacts") || !strings.Contains(first.Error(), "version files remain at the candidate") {
		t.Fatalf("first Run(%v) error = %q, want resume-loop refusal copy", args, first.Error())
	}
	beforeHEAD := gitOutputReleaseTest(t, repo, "rev-parse", "HEAD")
	if dirty := gitOutputReleaseTest(t, repo, "status", "--porcelain"); dirty == "" {
		t.Fatal("first refusal left a clean worktree, want prepared dirt")
	}
	// Gate refusal restores the changelog it wrote; version files stay at candidate.
	if n := countChangelogHeadings(string(mustReadFile(t, filepath.Join(repo, "CHANGELOG.md"))), "1.1.0"); n != 0 {
		t.Fatalf("after refusal CHANGELOG.md has %d headings for 1.1.0, want 0 (restored to HEAD)", n)
	}
	pkg := mustReadFile(t, filepath.Join(repo, "package.json"))
	if !strings.Contains(string(pkg), `"version": "1.1.0"`) {
		t.Fatalf("after refusal package.json = %s, want version left at candidate 1.1.0", pkg)
	}

	// Operator re-records against the rebuilt tree (version files stay at candidate).
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

	// (a) changelog contains exactly one heading for the candidate version.
	changelog, err := os.ReadFile(filepath.Join(repo, "CHANGELOG.md"))
	if err != nil {
		t.Fatal(err)
	}
	if n := countChangelogHeadings(string(changelog), "1.1.0"); n != 1 {
		t.Fatalf("CHANGELOG.md has %d headings for 1.1.0, want exactly 1\n%s", n, changelog)
	}

	// (b) exact committed path set — not a broad research-tree predicate.
	changed := releaseCommitChangedPaths(t, repo)
	for i, path := range changed {
		changed[i] = filepath.ToSlash(path)
	}
	wantPaths := []string{
		"CHANGELOG.md",
		"dist/opencode/plugins/hooks.ts",
		"docs/changes/20260710-journal-reliability-foundation/research/claude-code-2.1.220-plugin-startup-smoke.json",
		"docs/changes/20260710-journal-reliability-foundation/research/codex-0.146.0-isolated-startup-smoke.json",
		"docs/changes/20260710-journal-reliability-foundation/research/opencode-1.18.7-isolated-request-smoke.json",
		"package.json",
	}
	if len(changed) != len(wantPaths) {
		t.Fatalf("release commit paths = %v, want exactly %v", changed, wantPaths)
	}
	// Compare as sets: git path order is tree order, not required by the gate.
	wantSet := map[string]bool{}
	for _, p := range wantPaths {
		wantSet[p] = true
	}
	for _, path := range changed {
		if !wantSet[path] {
			t.Fatalf("release commit paths = %v, want exactly the set %v (unexpected %q)", changed, wantPaths, path)
		}
		delete(wantSet, path)
	}
	if len(wantSet) != 0 {
		t.Fatalf("release commit paths = %v, missing %v", changed, wantSet)
	}
}

func TestReleaseApplyResumeClobbersHandEditedGeneratedFile(t *testing.T) {
	repo := seedReleaseApplyRepoWithStalingBuild(t, "feat: resume clobbers hand-edited dist")

	args := []string{"release", "--yes", "--no-tag", "--no-gh"}
	if err := (Runner{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}, WorkingDir: repo}).Run(args); err == nil {
		t.Fatalf("first Run(%v) error = nil, want stale-evidence refusal", args)
	}
	rewriteInstalledSmokeReceiptHashes(t, repo)

	// Hand-edit a tracked generated file after the refused prepare. Restore must
	// discard this; rebuild must produce the build script's content, not the edit.
	// The capability-evidence seed already tracks dist/opencode/plugins/hooks.ts.
	hooksPath := filepath.Join(repo, "dist", "opencode", "plugins", "hooks.ts")
	const handEdit = "HAND_EDIT_MUST_NOT_LAND\n"
	if err := os.WriteFile(hooksPath, []byte(handEdit), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	if err := (Runner{Stdout: &stdout, Stderr: &bytes.Buffer{}, WorkingDir: repo}).Run(args); err != nil {
		t.Fatalf("resume Run(%v) error = %v\n%s", args, err, stdout.String())
	}
	committed, err := os.ReadFile(hooksPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(committed) == handEdit {
		t.Fatal("hand-edited dist content was committed; want build output after restore-and-regenerate")
	}
	if string(committed) != "staled-hooks\n" {
		t.Fatalf("committed hooks.ts = %q, want build output %q", committed, "staled-hooks\n")
	}
	if n := countChangelogHeadings(string(mustReadFile(t, filepath.Join(repo, "CHANGELOG.md"))), "1.1.0"); n != 1 {
		t.Fatalf("CHANGELOG.md has %d headings for 1.1.0, want exactly 1", n)
	}
}

func TestReleaseApplyResumeRefusesUntrackedUnderDist(t *testing.T) {
	repo := seedReleaseApplyRepoWithStalingBuild(t, "feat: resume refuses untracked dist")

	args := []string{"release", "--yes", "--no-tag", "--no-gh"}
	if err := (Runner{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}, WorkingDir: repo}).Run(args); err == nil {
		t.Fatalf("first Run(%v) error = nil, want stale-evidence refusal", args)
	}
	rewriteInstalledSmokeReceiptHashes(t, repo)

	extra := filepath.Join(repo, "dist", "extra.js")
	if err := os.MkdirAll(filepath.Dir(extra), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(extra, []byte("not part of the build\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := (Runner{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}, WorkingDir: repo}).Run(args)
	if err == nil {
		t.Fatal("resume with untracked dist/extra.js error = nil, want refusal")
	}
	msg := err.Error()
	if !strings.Contains(msg, "untracked file under generated-output tree") || !strings.Contains(msg, "dist/extra.js") {
		t.Fatalf("error = %q, want untracked-generated refusal naming dist/extra.js", msg)
	}
}

func TestReleaseApplyRefusesPreparedTreeWithUnrelatedDirty(t *testing.T) {
	repo := seedReleaseApplyRepoWithStalingBuild(t, "feat: resume boundary keeps non-release dirt out")

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

func parentRegistryShowResponse(t *testing.T, repo string) releasePostMergeCommandResult {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(repo, filepath.FromSlash(TargetCapabilityEvidenceRecordPath)))
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", TargetCapabilityEvidenceRecordPath, err)
	}
	return releasePostMergeOK(string(data))
}

func nameStatusZ(paths ...string) string {
	var b strings.Builder
	for _, path := range paths {
		b.WriteString("M")
		b.WriteByte(0)
		b.WriteString(path)
		b.WriteByte(0)
	}
	return b.String()
}

func TestReleasePostMergeEvidenceOnlyRepairPasses(t *testing.T) {
	repo := seedReleasePostMergeFiles(t, "1.2.3")
	seedReleaseCapabilityEvidence(t, repo)
	receipt := "docs/changes/20260710-journal-reliability-foundation/research/opencode-1.18.7-isolated-request-smoke.json"
	responses := releasePostMergeHappyResponses("1.2.3")
	// Detect repair via HEAD^..HEAD receipt-only diff against the parent
	// registry; subject + release shape come from the parent release commit;
	// tag still lands on HEAD. Registry itself must not appear in the diff.
	responses["git rev-parse --verify HEAD^"] = releasePostMergeOK("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	responses["git diff --name-status --no-renames -z HEAD^ HEAD"] = releasePostMergeOK(nameStatusZ(receipt))
	responses["git show HEAD^:"+TargetCapabilityEvidenceRecordPath] = parentRegistryShowResponse(t, repo)
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

func TestReleasePostMergeRepairModifyingRegistryRefuses(t *testing.T) {
	repo := seedReleasePostMergeFiles(t, "1.2.3")
	seedReleaseCapabilityEvidence(t, repo)
	receipt := "docs/changes/20260710-journal-reliability-foundation/research/opencode-1.18.7-isolated-request-smoke.json"
	responses := releasePostMergeHappyResponses("1.2.3")
	responses["git rev-parse --verify HEAD^"] = releasePostMergeOK("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	// Registry in the repair commit → not receipt-only; guardrail 5 evaluates HEAD.
	responses["git diff --name-status --no-renames -z HEAD^ HEAD"] = releasePostMergeOK(nameStatusZ(TargetCapabilityEvidenceRecordPath, receipt))
	responses["git diff HEAD^ HEAD --name-only"] = releasePostMergeOK(TargetCapabilityEvidenceRecordPath + "\n" + receipt)
	responses["git show HEAD^:"+TargetCapabilityEvidenceRecordPath] = parentRegistryShowResponse(t, repo)
	runner, _ := scriptedReleasePostMergeRunner(responses)
	snap := mustResolveReleaseSnapshot(t, repo, releaseOptions{postMerge: true})
	result := checkReleasePostMergeGuardrails(repo, snap, runner)
	if result.ok || result.guardrail != 5 {
		t.Fatalf("result = %#v, want guardrail 5 failure when repair modifies the registry", result)
	}
}

func TestReleasePostMergeNonEvidenceRepairStillFailsDiffShape(t *testing.T) {
	repo := seedReleasePostMergeFiles(t, "1.2.3")
	seedReleaseCapabilityEvidence(t, repo)
	receipt := "docs/changes/20260710-journal-reliability-foundation/research/opencode-1.18.7-isolated-request-smoke.json"
	responses := releasePostMergeHappyResponses("1.2.3")
	responses["git rev-parse --verify HEAD^"] = releasePostMergeOK("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	// Touches a non-receipt path → not evidence-only; guardrail 5 evaluates HEAD.
	responses["git diff --name-status --no-renames -z HEAD^ HEAD"] = releasePostMergeOK(nameStatusZ(receipt, "README.md"))
	responses["git diff HEAD^ HEAD --name-only"] = releasePostMergeOK(receipt + "\nREADME.md")
	responses["git show HEAD^:"+TargetCapabilityEvidenceRecordPath] = parentRegistryShowResponse(t, repo)
	runner, _ := scriptedReleasePostMergeRunner(responses)
	snap := mustResolveReleaseSnapshot(t, repo, releaseOptions{postMerge: true})
	result := checkReleasePostMergeGuardrails(repo, snap, runner)
	if result.ok || result.guardrail != 5 {
		t.Fatalf("result = %#v, want guardrail 5 failure for non-evidence repair", result)
	}
}

func TestReleasePostMergeRepairTouchingFixtureSourceRefuses(t *testing.T) {
	repo := seedReleasePostMergeFiles(t, "1.2.3")
	seedReleaseCapabilityEvidence(t, repo)
	// level:fixture source is in the registry but is not a receipt.
	fixture := "internal/cli/journal_hook_claude_test.go"
	responses := releasePostMergeHappyResponses("1.2.3")
	responses["git rev-parse --verify HEAD^"] = releasePostMergeOK("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	responses["git diff --name-status --no-renames -z HEAD^ HEAD"] = releasePostMergeOK(nameStatusZ(fixture))
	responses["git diff HEAD^ HEAD --name-only"] = releasePostMergeOK(fixture)
	responses["git show HEAD^:"+TargetCapabilityEvidenceRecordPath] = parentRegistryShowResponse(t, repo)
	runner, _ := scriptedReleasePostMergeRunner(responses)
	snap := mustResolveReleaseSnapshot(t, repo, releaseOptions{postMerge: true})
	result := checkReleasePostMergeGuardrails(repo, snap, runner)
	if result.ok || result.guardrail != 5 {
		t.Fatalf("result = %#v, want guardrail 5 failure for fixture-level repair path", result)
	}
}

func TestReleasePostMergeRepairWhitespacePaddedFilenameRefuses(t *testing.T) {
	repo := seedReleasePostMergeFiles(t, "1.2.3")
	seedReleaseCapabilityEvidence(t, repo)
	receipt := "docs/changes/20260710-journal-reliability-foundation/research/opencode-1.18.7-isolated-request-smoke.json"
	// Leading spaces must not alias the real receipt path after TrimSpace.
	padded := "  " + receipt
	responses := releasePostMergeHappyResponses("1.2.3")
	responses["git rev-parse --verify HEAD^"] = releasePostMergeOK("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	responses["git diff --name-status --no-renames -z HEAD^ HEAD"] = releasePostMergeOK(nameStatusZ(padded))
	responses["git diff HEAD^ HEAD --name-only"] = releasePostMergeOK(padded)
	responses["git show HEAD^:"+TargetCapabilityEvidenceRecordPath] = parentRegistryShowResponse(t, repo)
	runner, _ := scriptedReleasePostMergeRunner(responses)
	snap := mustResolveReleaseSnapshot(t, repo, releaseOptions{postMerge: true})
	result := checkReleasePostMergeGuardrails(repo, snap, runner)
	if result.ok || result.guardrail != 5 {
		t.Fatalf("result = %#v, want guardrail 5 failure for whitespace-padded repair path", result)
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

	t.Run("symlinked config directory is present but unusable", func(t *testing.T) {
		root := t.TempDir()
		realConfig := filepath.Join(root, "real-config")
		if err := os.MkdirAll(realConfig, 0o755); err != nil {
			t.Fatal(err)
		}
		// Valid-looking leaf behind a symlinked intermediate component.
		if err := os.WriteFile(filepath.Join(realConfig, "target-capabilities.json"), []byte(`{"contract_version":3,"records":[],"deferred":[{"target":"pi","status":"deferred","not_a_build_target":true,"reason":"deferred"}]`+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(realConfig, filepath.Join(root, "config")); err != nil {
			t.Fatal(err)
		}
		present, err := checkReleaseCapabilityEvidence(root)
		if !present {
			t.Fatal("symlinked config/ classified as absent; want present-but-unusable")
		}
		if err == nil || !strings.Contains(err.Error(), "symlink") {
			t.Fatalf("error = %v, want symlink-component refusal", err)
		}
	})

	t.Run("absent remains a silent no-op", func(t *testing.T) {
		present, err := checkReleaseCapabilityEvidence(t.TempDir())
		if present || err != nil {
			t.Fatalf("present=%v err=%v, want absent no-op", present, err)
		}
	})
}

func TestReleaseApplyRefusesSymlinkedConfigDirectory(t *testing.T) {
	repo := seedReleaseApplyRepo(t, "feat: refuse symlinked config on apply")
	hardenFixtureRepoAgainstHostSigning(t, repo)
	// Real evidence under a temporary real-config, then replace config/ with a symlink.
	seedReleaseCapabilityEvidence(t, repo)
	realConfig := filepath.Join(repo, "real-config")
	if err := os.Rename(filepath.Join(repo, "config"), realConfig); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realConfig, filepath.Join(repo, "config")); err != nil {
		t.Fatal(err)
	}
	// Commit so the worktree is clean except for the symlink structure as HEAD.
	// Symlink itself must be what the apply path sees for the probe.
	writeFile(t, filepath.Join(repo, ".gitignore"), "bin/native/\nplugins/loaf/bin/native/\n")
	gitCLI(t, repo, "add", "-A")
	gitCLI(t, repo, "commit", "-m", "chore: record evidence behind symlinked config")

	err := (Runner{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}, WorkingDir: repo}).Run([]string{"release", "--yes", "--no-tag", "--no-gh"})
	if err == nil {
		t.Fatal("release error = nil, want symlinked-config refusal")
	}
	msg := err.Error()
	if !strings.Contains(msg, "Refusing to commit release artifacts") || !strings.Contains(msg, "symlink") {
		t.Fatalf("error = %q, want apply refusal naming symlink", msg)
	}
}

func TestReleaseApplyRefusesUnreferencedResearchFileTracked(t *testing.T) {
	repo := seedReleaseApplyRepoWithStalingBuild(t, "feat: refuse unreferenced tracked research")
	args := []string{"release", "--yes", "--no-tag", "--no-gh"}
	if err := (Runner{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}, WorkingDir: repo}).Run(args); err == nil {
		t.Fatalf("first Run(%v) error = nil, want stale-evidence refusal", args)
	}
	rewriteInstalledSmokeReceiptHashes(t, repo)

	// Tracked file under research/ that no registry references.
	orphan := "docs/changes/20260710-journal-reliability-foundation/research/orphan-notes.md"
	writeFile(t, filepath.Join(repo, filepath.FromSlash(orphan)), "not referenced\n")
	gitCLI(t, repo, "add", orphan)
	// Leave it staged/dirty relative to HEAD by amending? add alone stages; status shows staged as dirty.
	// Make it a committed-then-modified path so porcelain is " M" not just staged-new after we need dirt on resume.
	// Simpler: keep it uncommitted tracked-new (A in index). releaseUnignoredStatusEntries sees it as tracked dirt.
	// Actually `git add` of new file shows "A " in index — not untracked. deleted=false. Not in allowlist → refuse.

	err := (Runner{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}, WorkingDir: repo}).Run(args)
	if err == nil {
		t.Fatal("resume with unreferenced tracked research file error = nil, want refusal")
	}
	msg := err.Error()
	if !strings.Contains(msg, "require a clean unignored worktree") || !strings.Contains(msg, orphan) {
		t.Fatalf("error = %q, want clean-worktree refusal naming %s", msg, orphan)
	}
}

func TestReleaseApplyRefusesUnreferencedResearchFileUntracked(t *testing.T) {
	repo := seedReleaseApplyRepoWithStalingBuild(t, "feat: refuse unreferenced untracked research")
	args := []string{"release", "--yes", "--no-tag", "--no-gh"}
	if err := (Runner{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}, WorkingDir: repo}).Run(args); err == nil {
		t.Fatalf("first Run(%v) error = nil, want stale-evidence refusal", args)
	}
	rewriteInstalledSmokeReceiptHashes(t, repo)

	orphan := "docs/changes/20260710-journal-reliability-foundation/research/orphan-untracked.md"
	writeFile(t, filepath.Join(repo, filepath.FromSlash(orphan)), "not referenced\n")

	err := (Runner{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}, WorkingDir: repo}).Run(args)
	if err == nil {
		t.Fatal("resume with unreferenced untracked research file error = nil, want refusal")
	}
	msg := err.Error()
	if !strings.Contains(msg, "require a clean unignored worktree") || !strings.Contains(msg, orphan) {
		t.Fatalf("error = %q, want clean-worktree refusal naming %s", msg, orphan)
	}
}

func TestReleaseApplyRefusesVersionFileAtNonCandidateContent(t *testing.T) {
	repo := seedReleaseApplyRepoWithStalingBuild(t, "feat: refuse non-candidate version dirt")
	args := []string{"release", "--yes", "--no-tag", "--no-gh"}
	if err := (Runner{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}, WorkingDir: repo}).Run(args); err == nil {
		t.Fatalf("first Run(%v) error = nil, want stale-evidence refusal", args)
	}
	rewriteInstalledSmokeReceiptHashes(t, repo)

	// Hand-edit version to something other than the candidate rendering.
	writeFile(t, filepath.Join(repo, "package.json"), strings.Join([]string{
		"{",
		`  "name": "release-fixture",`,
		`  "version": "9.9.9",`,
		`  "scripts": {`,
		`    "build": "node -e \"require('fs').mkdirSync('dist/opencode/plugins',{recursive:true}); require('fs').writeFileSync('dist/opencode/plugins/hooks.ts','staled-hooks\\n')\""`,
		"  }",
		"}",
		"",
	}, "\n"))

	err := (Runner{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}, WorkingDir: repo}).Run(args)
	if err == nil {
		t.Fatal("resume with non-candidate version error = nil, want refusal")
	}
	msg := err.Error()
	if !strings.Contains(msg, "require a clean unignored worktree") || !strings.Contains(msg, "package.json") {
		t.Fatalf("error = %q, want clean-worktree refusal naming package.json", msg)
	}
}

func TestReleaseApplyAdmitsVersionFileByteEqualToCandidate(t *testing.T) {
	repo := seedReleaseApplyRepoWithStalingBuild(t, "feat: admit candidate version dirt")
	args := []string{"release", "--yes", "--no-tag", "--no-gh"}
	if err := (Runner{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}, WorkingDir: repo}).Run(args); err == nil {
		t.Fatalf("first Run(%v) error = nil, want stale-evidence refusal", args)
	}
	// package.json already at candidate from refusal; leave it. Re-record and resume.
	if !strings.Contains(string(mustReadFile(t, filepath.Join(repo, "package.json"))), `"version": "1.1.0"`) {
		t.Fatal("expected package.json at candidate after refusal")
	}
	rewriteInstalledSmokeReceiptHashes(t, repo)

	var stdout bytes.Buffer
	if err := (Runner{Stdout: &stdout, Stderr: &bytes.Buffer{}, WorkingDir: repo}).Run(args); err != nil {
		t.Fatalf("resume with candidate-matching version error = %v\n%s", err, stdout.String())
	}
	if subject := gitOutputReleaseTest(t, repo, "log", "-1", "--pretty=%s"); subject != "chore: release v1.1.0" {
		t.Fatalf("release commit subject = %q, want chore: release v1.1.0", subject)
	}
}

func TestReleaseApplyRefusesVersionFileReplacedBySymlinkToCandidateBytes(t *testing.T) {
	repo := seedReleaseApplyRepoWithStalingBuild(t, "feat: refuse version symlink admission")
	args := []string{"release", "--yes", "--no-tag", "--no-gh"}
	if err := (Runner{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}, WorkingDir: repo}).Run(args); err == nil {
		t.Fatalf("first Run(%v) error = nil, want stale-evidence refusal", args)
	}
	beforeHEAD := gitOutputReleaseTest(t, repo, "rev-parse", "HEAD")
	candidateBody := mustReadFile(t, filepath.Join(repo, "package.json"))
	if !bytes.Contains(candidateBody, []byte(`"version": "1.1.0"`)) {
		t.Fatalf("after refusal package.json = %s, want candidate version", candidateBody)
	}
	rewriteInstalledSmokeReceiptHashes(t, repo)

	// External file holds the exact candidate bytes; version path becomes a symlink.
	external := filepath.Join(t.TempDir(), "external-package.json")
	if err := os.WriteFile(external, candidateBody, 0o644); err != nil {
		t.Fatal(err)
	}
	pkgPath := filepath.Join(repo, "package.json")
	if err := os.Remove(pkgPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, pkgPath); err != nil {
		t.Fatal(err)
	}

	err := (Runner{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}, WorkingDir: repo}).Run(args)
	if err == nil {
		t.Fatal("resume with symlinked version file error = nil, want refusal")
	}
	msg := err.Error()
	if !strings.Contains(msg, "require a clean unignored worktree") || !strings.Contains(msg, "package.json") {
		t.Fatalf("error = %q, want clean-worktree refusal naming package.json", msg)
	}
	if head := gitOutputReleaseTest(t, repo, "rev-parse", "HEAD"); head != beforeHEAD {
		t.Fatalf("refused resume moved HEAD from %s to %s", beforeHEAD, head)
	}
	info, lerr := os.Lstat(pkgPath)
	if lerr != nil {
		t.Fatal(lerr)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("package.json was restored or rewritten; want symlink left in place")
	}
	// Nothing committed: HEAD package.json must still be a regular blob, not a symlink.
	mode := gitOutputReleaseTest(t, repo, "ls-tree", "HEAD", "--", "package.json")
	if !strings.HasPrefix(strings.Fields(mode)[0], "100") {
		t.Fatalf("HEAD package.json mode = %q, want regular blob", mode)
	}
}

func TestReleaseUnignoredStatusEntriesClassifiesTypechange(t *testing.T) {
	repo := seedReleaseApplyRepoWithStalingBuild(t, "feat: classify porcelain T")
	// Establish a regular tracked version file, then replace with a symlink so
	// porcelain reports T (typechange).
	pkgPath := filepath.Join(repo, "package.json")
	body := mustReadFile(t, pkgPath)
	external := filepath.Join(t.TempDir(), "ext.json")
	if err := os.WriteFile(external, body, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(pkgPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, pkgPath); err != nil {
		t.Fatal(err)
	}

	entries, err := releaseUnignoredStatusEntries(repo, "package.json")
	if err != nil {
		t.Fatalf("releaseUnignoredStatusEntries: %v", err)
	}
	var found *releaseStatusEntry
	for i := range entries {
		if entries[i].path == "package.json" {
			found = &entries[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("entries = %+v, want package.json", entries)
	}
	if !found.typechange {
		t.Fatalf("package.json entry = %+v, want typechange=true", *found)
	}
	if found.deleted || found.untracked {
		t.Fatalf("package.json entry = %+v, want only typechange", *found)
	}

	// Classification must refuse the typechanged version path by name.
	err = requireReleaseCleanWorktree(repo, releaseOptions{})
	if err == nil {
		t.Fatal("requireReleaseCleanWorktree error = nil, want typechange refusal")
	}
	msg := err.Error()
	if !strings.Contains(msg, "require a clean unignored worktree") || !strings.Contains(msg, "package.json") {
		t.Fatalf("error = %q, want clean-worktree refusal naming package.json", msg)
	}
}

func TestReleaseApplyRefusesVersionFileExecutableBitFlip(t *testing.T) {
	repo := seedReleaseApplyRepoWithStalingBuild(t, "feat: refuse version mode flip")
	args := []string{"release", "--yes", "--no-tag", "--no-gh"}
	if err := (Runner{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}, WorkingDir: repo}).Run(args); err == nil {
		t.Fatalf("first Run(%v) error = nil, want stale-evidence refusal", args)
	}
	beforeHEAD := gitOutputReleaseTest(t, repo, "rev-parse", "HEAD")
	pkgPath := filepath.Join(repo, "package.json")
	if !strings.Contains(string(mustReadFile(t, pkgPath)), `"version": "1.1.0"`) {
		t.Fatal("expected package.json at candidate after refusal")
	}
	rewriteInstalledSmokeReceiptHashes(t, repo)

	// Flip executable bit only; candidate bytes stay byte-identical.
	if err := os.Chmod(pkgPath, 0o755); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(pkgPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o111 == 0 {
		t.Fatal("chmod +x did not set executable bit")
	}
	headMode, err := releaseGitHeadBlobMode(repo, "package.json")
	if err != nil {
		t.Fatal(err)
	}
	if headMode != "100644" {
		t.Fatalf("HEAD package.json mode = %q, want 100644 for this fixture", headMode)
	}
	if releaseWorktreeBlobMode(info) == headMode {
		t.Fatal("worktree mode still matches HEAD after +x; test setup broken")
	}

	runErr := (Runner{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}, WorkingDir: repo}).Run(args)
	if runErr == nil {
		t.Fatal("resume with executable-bit-flipped version file error = nil, want refusal")
	}
	msg := runErr.Error()
	if !strings.Contains(msg, "require a clean unignored worktree") || !strings.Contains(msg, "package.json") {
		t.Fatalf("error = %q, want clean-worktree refusal naming package.json", msg)
	}
	if head := gitOutputReleaseTest(t, repo, "rev-parse", "HEAD"); head != beforeHEAD {
		t.Fatalf("refused resume moved HEAD from %s to %s", beforeHEAD, head)
	}
}

func TestReleaseApplyRefusesDirtyChangelogOnRerun(t *testing.T) {
	repo := seedReleaseApplyRepoWithStalingBuild(t, "feat: refuse dirty changelog on resume")
	args := []string{"release", "--yes", "--no-tag", "--no-gh"}
	if err := (Runner{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}, WorkingDir: repo}).Run(args); err == nil {
		t.Fatalf("first Run(%v) error = nil, want stale-evidence refusal", args)
	}
	rewriteInstalledSmokeReceiptHashes(t, repo)

	// Operator (or hand) dirties CHANGELOG after refusal restored it — sacred.
	writeFile(t, filepath.Join(repo, "CHANGELOG.md"), strings.Join([]string{
		"# Changelog",
		"",
		"## [Unreleased]",
		"",
		"- hand curated entry that must not be clobbered",
		"",
	}, "\n"))

	err := (Runner{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}, WorkingDir: repo}).Run(args)
	if err == nil {
		t.Fatal("resume with dirty CHANGELOG error = nil, want refusal")
	}
	msg := err.Error()
	if !strings.Contains(msg, "require a clean unignored worktree") || !strings.Contains(msg, "CHANGELOG.md") {
		t.Fatalf("error = %q, want clean-worktree refusal naming CHANGELOG.md", msg)
	}
	// And the hand-curated content must still be on disk (never restored by classification).
	body := string(mustReadFile(t, filepath.Join(repo, "CHANGELOG.md")))
	if !strings.Contains(body, "hand curated entry that must not be clobbered") {
		t.Fatalf("CHANGELOG.md was altered by the refused resume; body = %q", body)
	}
}

func TestReleaseApplyRefusesDeletedTrackedFile(t *testing.T) {
	repo := seedReleaseApplyRepoWithStalingBuild(t, "feat: refuse deleted tracked file")
	args := []string{"release", "--yes", "--no-tag", "--no-gh"}
	if err := (Runner{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}, WorkingDir: repo}).Run(args); err == nil {
		t.Fatalf("first Run(%v) error = nil, want stale-evidence refusal", args)
	}
	rewriteInstalledSmokeReceiptHashes(t, repo)

	if err := os.Remove(filepath.Join(repo, "feature.txt")); err != nil {
		t.Fatal(err)
	}

	err := (Runner{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}, WorkingDir: repo}).Run(args)
	if err == nil {
		t.Fatal("resume with deleted tracked file error = nil, want refusal")
	}
	msg := err.Error()
	if !strings.Contains(msg, "require a clean unignored worktree") || !strings.Contains(msg, "feature.txt") {
		t.Fatalf("error = %q, want clean-worktree refusal naming feature.txt", msg)
	}
}

func TestReleaseParseNameStatusZ(t *testing.T) {
	paths, ok := releaseParseNameStatusZ(nameStatusZ("a.json", "b.json"))
	if !ok || len(paths) != 2 || paths[0] != "a.json" || paths[1] != "b.json" {
		t.Fatalf("paths=%v ok=%v", paths, ok)
	}
	// Whitespace-padded path is preserved (not trimmed).
	padded := "M\x00  padded.json\x00"
	paths, ok = releaseParseNameStatusZ(padded)
	if !ok || len(paths) != 1 || paths[0] != "  padded.json" {
		t.Fatalf("padded paths=%v ok=%v", paths, ok)
	}
	// Type-change / delete / rename statuses refuse.
	for _, raw := range []string{"T\x00x\x00", "D\x00x\x00", "R100\x00new\x00old\x00"} {
		if _, ok := releaseParseNameStatusZ(raw); ok {
			t.Fatalf("raw %q parsed ok, want reject", raw)
		}
	}
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	return body
}
