package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func seedCohortGateRepo(t *testing.T, version string) string {
	t.Helper()
	repo := initCLIGitRepo(t)
	writeReleaseVersionFiles(t, repo, version)
	return repo
}

func writeReleaseVersionFiles(t *testing.T, repo, version string) {
	t.Helper()
	pkg := "{\n  \"name\": \"demo\",\n  \"version\": \"" + version + "\"\n}\n"
	if err := os.WriteFile(filepath.Join(repo, "package.json"), []byte(pkg), 0o644); err != nil {
		t.Fatalf("WriteFile package.json: %v", err)
	}
}

func writeNewLayoutChange(t *testing.T, repo, folder, slug, target string, shapeBody string) string {
	t.Helper()
	dir := filepath.Join(repo, "docs", "changes", folder)
	if err := os.MkdirAll(filepath.Join(dir, "tasks"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	meta := "{\n  \"change\": \"" + slug + "\",\n  \"created\": \"2026-07-27\",\n  \"branch\": \"" + slug + "\""
	if target != "" {
		meta += ",\n  \"target_release\": \"" + target + "\""
	}
	meta += "\n}\n"
	if err := os.WriteFile(filepath.Join(dir, "change.json"), []byte(meta), 0o644); err != nil {
		t.Fatalf("WriteFile change.json: %v", err)
	}
	if shapeBody == "" {
		shapeBody = authoredShapeBody()
	}
	if err := os.WriteFile(filepath.Join(dir, "shape.md"), []byte(shapeBody), 0o644); err != nil {
		t.Fatalf("WriteFile shape.md: %v", err)
	}
	return dir
}

func authoredShapeBody() string {
	sections := append(productSections(),
		"## Planning Contract\n\n### Approach\n\nHow.",
		"## Implementation Units\n\n- U1 — do the thing.",
		"## Verification Contract\n\n- **V1.** Smoke.\n  - Command: `true`\n  - Expect: exit 0",
		"## Definition of Done\n\n- Gates pass.",
	)
	var b strings.Builder
	b.WriteString("# Demo\n\n")
	for _, s := range sections {
		b.WriteString(s)
		b.WriteString("\n\n")
	}
	return b.String()
}

func TestReleaseCohortGateBlocksUnexecutedStableTarget(t *testing.T) {
	repo := seedCohortGateRepo(t, "2.0.0-alpha.1")
	writeNewLayoutChange(t, repo, "20260727-cohort-member", "cohort-member", "2.0.0", "")
	commitAllChangeTest(t, repo, "docs: shape cohort member")

	err := releaseCohortPreflight(repo, "2.0.0", nil)
	if err == nil || !strings.Contains(err.Error(), "targets 2.0.0 but is not executed") {
		t.Fatalf("err = %v, want not executed", err)
	}

	// Prerelease candidate bypasses.
	if err := releaseCohortPreflight(repo, "2.0.0-alpha.2", nil); err != nil {
		t.Fatalf("prerelease candidate should bypass: %v", err)
	}

	// Minor candidate 2.1.0 does not gate 2.0.0 cohort (warn only).
	var warnings []string
	if err := releaseCohortPreflight(repo, "2.1.0", &warnings); err != nil {
		t.Fatalf("2.1.0 candidate should not block on 2.0.0 cohort: %v", err)
	}
	if !findingsContain(warnings, "incomplete lower cohort") {
		t.Fatalf("warnings = %v, want lower cohort warn", warnings)
	}
}

func TestReleaseCohortGateLegacyConvertFirst(t *testing.T) {
	repo := seedCohortGateRepo(t, "2.0.0-alpha.1")
	body := executableLineageDoc("legacy-member", "line", "", "")
	body = strings.Replace(body, "---\n", "---\ntarget_release: 2.0.0\n", 1)
	writeChangeFolder(t, repo, "20260727-legacy-member", body)
	commitAllChangeTest(t, repo, "docs: legacy cohort member")

	err := releaseCohortPreflight(repo, "2.0.0", nil)
	if err == nil || !strings.Contains(err.Error(), "legacy layout — convert first") {
		t.Fatalf("err = %v, want convert first", err)
	}
}

func TestReleaseCohortGateAcceptsFlipExecutedMember(t *testing.T) {
	repo := seedCohortGateRepo(t, "2.0.0-alpha.1")
	dir := writeNewLayoutChange(t, repo, "20260727-executed", "executed", "2.0.0", "")
	task := filepath.Join(dir, "tasks", "TASK-001-work.md")
	if err := os.WriteFile(task, []byte("---\nchange: executed\nid: TASK-001\ntitle: Work\n---\n\n# Work\n\n## Steps\n\n- [ ] Do it\n"), 0o644); err != nil {
		t.Fatalf("WriteFile task: %v", err)
	}
	commitAllChangeTest(t, repo, "docs: shape executed member")

	if err := os.WriteFile(filepath.Join(repo, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("WriteFile main.go: %v", err)
	}
	if err := os.WriteFile(task, []byte("---\nchange: executed\nid: TASK-001\ntitle: Work\n---\n\n# Work\n\n## Steps\n\n- [ ] Do it\n\nnote\n"), 0o644); err != nil {
		t.Fatalf("WriteFile task touch: %v", err)
	}
	commitAllChangeTest(t, repo, "chore: path grade only")
	err := releaseCohortPreflight(repo, "2.0.0", nil)
	if err == nil || !strings.Contains(err.Error(), "not executed") {
		t.Fatalf("path grade should not open gate: %v", err)
	}

	if err := os.WriteFile(task, []byte("---\nchange: executed\nid: TASK-001\ntitle: Work\n---\n\n# Work\n\n## Steps\n\n- [x] Do it\n"), 0o644); err != nil {
		t.Fatalf("WriteFile flip: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile main.go flip: %v", err)
	}
	commitAllChangeTest(t, repo, "feat: execute task")

	var stdout bytes.Buffer
	if err := (Runner{Stdout: &stdout, WorkingDir: repo}).Run([]string{"change", "verify", filepath.Join("docs", "changes", "20260727-executed")}); err != nil {
		t.Fatalf("verify: %v\n%s", err, stdout.String())
	}
	commitAllChangeTest(t, repo, "chore: commit verify receipt")

	if err := releaseCohortPreflight(repo, "2.0.0", nil); err != nil {
		t.Fatalf("flip-executed cohort with receipt should pass: %v", err)
	}
}

func TestResolveReleaseSnapshotFinalization(t *testing.T) {
	repo := seedCohortGateRepo(t, "2.0.0-alpha.14")
	snap, err := resolveReleaseSnapshot(repo, releaseOptions{bump: "release"})
	v := snap.Candidate
	if err != nil || v != "2.0.0" {
		t.Fatalf("release bump candidate = %q err=%v, want 2.0.0", v, err)
	}
	snap, err = resolveReleaseSnapshot(repo, releaseOptions{postMerge: true})
	v = snap.Candidate
	if err != nil || v != "2.0.0-alpha.14" {
		t.Fatalf("post-merge candidate = %q err=%v, want prepared 2.0.0-alpha.14", v, err)
	}
	snap, err = resolveReleaseSnapshot(repo, releaseOptions{bump: "minor"})
	v = snap.Candidate
	if err != nil || v != "2.1.0" {
		t.Fatalf("minor bump candidate = %q err=%v, want 2.1.0", v, err)
	}
	snap, err = resolveReleaseSnapshot(repo, releaseOptions{bump: "prerelease"})
	v = snap.Candidate
	if err != nil || v != "2.0.0-alpha.15" {
		t.Fatalf("prerelease bump candidate = %q err=%v, want 2.0.0-alpha.15", v, err)
	}
}

func TestReleaseSnapshotRefusesTimestampPatch(t *testing.T) {
	// A dev build stamps the moment it landed into the patch slot. No ceremony
	// can be cut from that number, and the snapshot is where every consumer
	// learns so.
	repo := seedCohortGateRepo(t, "0.2.1754476800")
	commitAllChangeTest(t, repo, "fix: carry a dev build stamp in the version file")

	for _, tc := range []struct {
		name    string
		options releaseOptions
	}{
		{"suggested bump", releaseOptions{}},
		{"explicit patch bump", releaseOptions{bump: "patch"}},
		{"post-merge", releaseOptions{postMerge: true}},
	} {
		snap, err := resolveReleaseSnapshot(repo, tc.options)
		if err == nil {
			t.Fatalf("%s: snapshot = %#v, want the ceremony guardrail to refuse", tc.name, snap)
		}
		msg := err.Error()
		if !strings.Contains(msg, "release ceremony guardrail") || !strings.Contains(msg, "plain 0.2.X version") {
			t.Fatalf("%s: err = %v, want the guardrail named and plain 0.2.X pointed at", tc.name, err)
		}
		if snap.Candidate != "" {
			t.Fatalf("%s: refused snapshot carries candidate %q, want an empty snapshot", tc.name, snap.Candidate)
		}
	}

	// The three doors a release runs through, all closed by that one refusal.
	for _, args := range [][]string{
		{"release", "--dry-run"},
		{"release", "-y", "--no-tag", "--no-gh"},
		{"release", "--post-merge"},
	} {
		var stdout bytes.Buffer
		err := (Runner{Stdout: &stdout, Stderr: &bytes.Buffer{}, WorkingDir: repo}).Run(args)
		if err == nil || !strings.Contains(err.Error(), "release ceremony guardrail") {
			t.Fatalf("loaf %s = %v, want the ceremony guardrail refusal\n%s", strings.Join(args, " "), err, stdout.String())
		}
	}

	// The guardrail refuses timestamps, not releases: the same paths resolve a
	// plain candidate untouched.
	plain := seedCohortGateRepo(t, "0.2.20")
	commitAllChangeTest(t, plain, "fix: carry a release version")
	for _, tc := range []struct {
		name    string
		options releaseOptions
		want    string
	}{
		{"suggested bump", releaseOptions{}, "0.2.21"},
		{"post-merge", releaseOptions{postMerge: true}, "0.2.20"},
	} {
		snap, err := resolveReleaseSnapshot(plain, tc.options)
		if err != nil || snap.Candidate != tc.want {
			t.Fatalf("%s on a plain version: candidate = %q err = %v, want %s", tc.name, snap.Candidate, err, tc.want)
		}
	}
}

// --- TASK-015: one candidate for the gate and the executor ---

func TestReleaseCohortGateNoBumpGatesSuggestedCandidate(t *testing.T) {
	repo := seedCohortGateRepo(t, "1.0.0")
	dir := writeNewLayoutChange(t, repo, "20260727-suggested", "suggested", "1.1.0", "")
	task := filepath.Join(dir, "tasks", "TASK-001-work.md")
	unchecked := "---\nchange: suggested\nid: TASK-001\ntitle: Work\n---\n\n# Work\n\n## Steps\n\n- [ ] Do it\n"
	if err := os.WriteFile(task, []byte(unchecked), 0o644); err != nil {
		t.Fatalf("WriteFile task: %v", err)
	}
	commitAllChangeTest(t, repo, "docs: shape suggested member")

	// A feat commit makes the suggested bump minor, so the no-flag invocation
	// would cut 1.1.0 — the version the incomplete cohort owns.
	if err := os.WriteFile(filepath.Join(repo, "feature.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("WriteFile feature.go: %v", err)
	}
	commitAllChangeTest(t, repo, "feat: unrelated feature")

	snap, err := resolveReleaseSnapshot(repo, releaseOptions{})
	candidate := snap.Candidate
	if err != nil {
		t.Fatalf("no-flag candidate: %v", err)
	}
	if candidate != "1.1.0" {
		t.Fatalf("no-flag candidate = %q, want 1.1.0 (suggested minor bump)", candidate)
	}
	gateErr := releaseCohortPreflight(repo, candidate, nil)
	if gateErr == nil || !strings.Contains(gateErr.Error(), `change "suggested" targets 1.1.0 but is not executed`) {
		t.Fatalf("gate err = %v, want 1.1.0 cohort block", gateErr)
	}

	var stdout bytes.Buffer
	runErr := (Runner{Stdout: &stdout, Stderr: &bytes.Buffer{}, WorkingDir: repo}).Run([]string{"release", "--dry-run"})
	if runErr == nil || !strings.Contains(runErr.Error(), "targets 1.1.0 but is not executed") {
		t.Fatalf("release --dry-run without --bump = %v, want cohort block\n%s", runErr, stdout.String())
	}

	// Cohort completes: the same flagless invocation proceeds to 1.1.0.
	checked := "---\nchange: suggested\nid: TASK-001\ntitle: Work\n---\n\n# Work\n\n## Steps\n\n- [x] Do it\n"
	if err := os.WriteFile(task, []byte(checked), 0o644); err != nil {
		t.Fatalf("WriteFile flip: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "feature.go"), []byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile feature.go flip: %v", err)
	}
	commitAllChangeTest(t, repo, "feat: execute suggested")
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: repo}).Run([]string{"change", "verify", filepath.Join("docs", "changes", "20260727-suggested")}); err != nil {
		t.Fatalf("verify: %v", err)
	}
	commitAllChangeTest(t, repo, "chore: commit verify receipt")

	snap, err = resolveReleaseSnapshot(repo, releaseOptions{})
	candidate = snap.Candidate
	if err != nil || candidate != "1.1.0" {
		t.Fatalf("candidate after completion = %q err=%v, want 1.1.0", candidate, err)
	}
	if err := releaseCohortPreflight(repo, candidate, nil); err != nil {
		t.Fatalf("completed cohort should open the gate: %v", err)
	}
	stdout.Reset()
	if err := (Runner{Stdout: &stdout, Stderr: &bytes.Buffer{}, WorkingDir: repo}).Run([]string{"release", "--dry-run"}); err != nil {
		t.Fatalf("release --dry-run after completion = %v\n%s", err, stdout.String())
	}
	if output := stripANSI(stdout.String()); !strings.Contains(output, "New version: 1.1.0") {
		t.Fatalf("dry-run output must cut the gated candidate; got:\n%s", output)
	}
}

func TestReleaseCohortGateNoBumpPrereleaseCandidateBypasses(t *testing.T) {
	repo := seedCohortGateRepo(t, "1.0.0-alpha.3")
	writeNewLayoutChange(t, repo, "20260727-prerelease-bypass", "prerelease-bypass", "1.0.0", "")
	commitAllChangeTest(t, repo, "docs: shape incomplete cohort member")
	gitCLI(t, repo, "-c", "tag.gpgsign=false", "-c", "tag.forceSignAnnotated=false", "tag", "v1.0.0-alpha.3")

	// Nothing unreleased: the flagless candidate stays on the prerelease the repo
	// carries, and a prerelease candidate never gates its cohort.
	snap, err := resolveReleaseSnapshot(repo, releaseOptions{})
	candidate := snap.Candidate
	if err != nil {
		t.Fatalf("no-flag candidate: %v", err)
	}
	if candidate != "1.0.0-alpha.3" || !releaseVersionIsPrerelease(candidate) {
		t.Fatalf("no-flag candidate = %q, want the current prerelease", candidate)
	}
	if err := releaseCohortPreflight(repo, candidate, nil); err != nil {
		t.Fatalf("prerelease candidate must bypass the incomplete cohort: %v", err)
	}
	var stdout bytes.Buffer
	if err := (Runner{Stdout: &stdout, Stderr: &bytes.Buffer{}, WorkingDir: repo}).Run([]string{"release", "--dry-run"}); err != nil {
		t.Fatalf("flagless dry run on a prerelease candidate = %v\n%s", err, stdout.String())
	}

	// The same fixture's --post-merge publishes the prepared prerelease through the valve.
	snap, err = resolveReleaseSnapshot(repo, releaseOptions{postMerge: true})
	post := snap.Candidate
	if err != nil || post != "1.0.0-alpha.3" {
		t.Fatalf("post-merge candidate = %q err=%v, want prepared 1.0.0-alpha.3", post, err)
	}
	if err := releaseCohortPreflight(repo, post, nil); err != nil {
		t.Fatalf("prepared prerelease post-merge must bypass the incomplete cohort: %v", err)
	}

	// Once commits exist, a suggested bump can only land on a stable candidate —
	// the flagless path cannot drift back into the bypass by accident.
	if err := os.WriteFile(filepath.Join(repo, "feature.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("WriteFile feature.go: %v", err)
	}
	commitAllChangeTest(t, repo, "feat: unrelated feature")
	snap, err = resolveReleaseSnapshot(repo, releaseOptions{})
	candidate = snap.Candidate
	if err != nil {
		t.Fatalf("candidate with commits: %v", err)
	}
	if candidate != "1.1.0" || releaseVersionIsPrerelease(candidate) {
		t.Fatalf("candidate with commits = %q, want stable 1.1.0", candidate)
	}
}

func flipExecuteChange(t *testing.T, repo, dir, slug string) {
	t.Helper()
	task := filepath.Join(dir, "tasks", "TASK-001-work.md")
	if err := os.MkdirAll(filepath.Dir(task), 0o755); err != nil {
		t.Fatalf("MkdirAll tasks: %v", err)
	}
	unchecked := "---\nchange: " + slug + "\nid: TASK-001\ntitle: Work\n---\n\n# Work\n\n## Steps\n\n- [ ] Do it\n"
	if err := os.WriteFile(task, []byte(unchecked), 0o644); err != nil {
		t.Fatalf("WriteFile task: %v", err)
	}
	commitAllChangeTest(t, repo, "docs: shape "+slug)

	if err := os.WriteFile(filepath.Join(repo, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("WriteFile main.go: %v", err)
	}
	checked := "---\nchange: " + slug + "\nid: TASK-001\ntitle: Work\n---\n\n# Work\n\n## Steps\n\n- [x] Do it\n"
	if err := os.WriteFile(task, []byte(checked), 0o644); err != nil {
		t.Fatalf("WriteFile flip: %v", err)
	}
	commitAllChangeTest(t, repo, "feat: execute "+slug)
}

// --- TASK-020/024: thread the snapshot; no re-derivation between gate and executor ---

func TestReleaseCandidateThreadedThroughExecutorDespiteDivergence(t *testing.T) {
	repo := seedCohortGateRepo(t, "1.0.0")
	gitCLI(t, repo, "-c", "tag.gpgsign=false", "-c", "tag.forceSignAnnotated=false", "tag", "v1.0.0")

	// A fix commit makes the suggested bump patch → candidate 1.0.1.
	if err := os.WriteFile(filepath.Join(repo, "fix.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("WriteFile fix.go: %v", err)
	}
	commitAllChangeTest(t, repo, "fix: seed patch bump")

	snap, err := resolveReleaseSnapshot(repo, releaseOptions{})
	if err != nil {
		t.Fatalf("preflight resolve: %v", err)
	}
	if snap.Candidate != "1.0.1" || snap.Bump != "patch" || snap.CurrentVersion != "1.0.0" {
		t.Fatalf("preflight = %#v, want 1.0.1/patch from 1.0.0", snap)
	}
	if err := releaseCohortPreflight(repo, snap.Candidate, nil); err != nil {
		t.Fatalf("preflight gate: %v", err)
	}

	// Seam: a feat lands after the gate judged 1.0.1. A fresh derivation would
	// suggest minor → 1.1.0; the threaded executor must still cut 1.0.1.
	if err := os.WriteFile(filepath.Join(repo, "feature.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("WriteFile feature.go: %v", err)
	}
	commitAllChangeTest(t, repo, "feat: land after preflight")

	fresh, err := resolveReleaseSnapshot(repo, releaseOptions{})
	if err != nil {
		t.Fatalf("fresh resolve: %v", err)
	}
	if fresh.Candidate != "1.1.0" || fresh.Bump != "minor" {
		t.Fatalf("fresh after feat = %#v, want 1.1.0/minor (proves the seam moves the derivation)", fresh)
	}

	var stdout bytes.Buffer
	opts := releaseOptions{
		dryRun:   true,
		snapshot: snap,
	}
	if err := runReleaseDryRun(repo, opts, &stdout, &bytes.Buffer{}); err != nil {
		t.Fatalf("dry-run with threaded candidate: %v\n%s", err, stdout.String())
	}
	output := stripANSI(stdout.String())
	if !strings.Contains(output, "New version: 1.0.1") {
		t.Fatalf("executor must cut preflight candidate 1.0.1; got:\n%s", output)
	}
	if strings.Contains(output, "New version: 1.1.0") {
		t.Fatalf("executor must not cut the post-seam re-derivation; got:\n%s", output)
	}
	if !strings.Contains(output, "Suggested bump: patch") {
		t.Fatalf("bump label must derive from the same resolution; got:\n%s", output)
	}
}

func TestReleaseApplyBlocksWhenVersionFileDriftsAfterPreflight(t *testing.T) {
	repo := seedCohortGateRepo(t, "1.0.0")
	gitCLI(t, repo, "-c", "tag.gpgsign=false", "-c", "tag.forceSignAnnotated=false", "tag", "v1.0.0")
	if err := os.WriteFile(filepath.Join(repo, "fix.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("WriteFile fix.go: %v", err)
	}
	commitAllChangeTest(t, repo, "fix: seed patch bump")

	snap, err := resolveReleaseSnapshot(repo, releaseOptions{})
	if err != nil {
		t.Fatalf("resolve snapshot: %v", err)
	}
	if snap.Candidate != "1.0.1" || snap.CurrentVersion != "1.0.0" {
		t.Fatalf("snapshot = %#v, want candidate 1.0.1 from 1.0.0", snap)
	}
	if err := releaseCohortPreflight(repo, snap.Candidate, nil); err != nil {
		t.Fatalf("preflight: %v", err)
	}

	// Version-file commit lands between preflight and apply.
	writeReleaseVersionFiles(t, repo, "1.0.1")
	commitAllChangeTest(t, repo, "chore: bump version underneath release")

	err = runReleaseApply(repo, releaseOptions{yes: true, tagSet: true, tag: false, ghSet: true, gh: false, snapshot: snap}, strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("apply error = nil, want version drift block")
	}
	msg := err.Error()
	if !strings.Contains(msg, "1.0.0") || !strings.Contains(msg, "1.0.1") || !strings.Contains(msg, "re-run release") {
		t.Fatalf("apply error = %v, want drift message naming both versions and re-run remedy", err)
	}
}

func TestReleasePostMergeBlocksWhenVersionFileDriftsAfterPreflight(t *testing.T) {
	repo := seedReleasePostMergeFiles(t, "1.2.3")
	snap, err := resolveReleaseSnapshot(repo, releaseOptions{postMerge: true})
	if err != nil {
		t.Fatalf("resolve snapshot: %v", err)
	}
	if snap.Candidate != "1.2.3" || snap.CurrentVersion != "1.2.3" {
		t.Fatalf("snapshot = %#v, want 1.2.3", snap)
	}

	// seedReleasePostMergeFiles is a file fixture (no git); drift is a filesystem rewrite the snapshot assert sees.
	writeReleaseVersionFiles(t, repo, "1.2.4")

	responses := releasePostMergeHappyResponses("1.2.3")
	runner, calls := scriptedReleasePostMergeRunner(responses)
	var stdout, stderr bytes.Buffer
	err = runReleasePostMergeWithRunner(repo, snap, &stdout, &stderr, runner)
	if err == nil {
		t.Fatal("post-merge error = nil, want version drift abort")
	}
	if !strings.Contains(err.Error(), "1.2.3") || !strings.Contains(err.Error(), "1.2.4") || !strings.Contains(err.Error(), "re-run release") {
		t.Fatalf("post-merge error = %v, want drift message", err)
	}
	for _, call := range releasePostMergeCallKeys(calls()) {
		if strings.HasPrefix(call, "git tag") {
			t.Fatalf("tagged despite drift: calls=%#v", releasePostMergeCallKeys(calls()))
		}
	}
}

func TestReleaseCohortGateRejectsFailingReceipt(t *testing.T) {
	repo := seedCohortGateRepo(t, "1.0.0-alpha.1")
	body := shapeWithVerification("- **V1.** Always fails. Command: `false`. Expect: exit 0\n- **V3.** Also fails. Command: `exit 1`. Expect: exit 0")
	dir := writeNewLayoutChange(t, repo, "20260727-failing-receipt", "failing-receipt", "1.0.0", body)
	flipExecuteChange(t, repo, dir, "failing-receipt")

	folderRel := filepath.Join("docs", "changes", "20260727-failing-receipt")
	var stdout bytes.Buffer
	err := (Runner{Stdout: &stdout, WorkingDir: repo}).Run([]string{"change", "verify", folderRel})
	if err == nil {
		t.Fatalf("verify should fail with failing criteria\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "Wrote receipt:") {
		t.Fatalf("write-on-failure expected receipt; stdout=%q", stdout.String())
	}
	commitAllChangeTest(t, repo, "chore: commit failing receipt")

	gateErr := releaseCohortPreflight(repo, "1.0.0", nil)
	if gateErr == nil || !strings.Contains(gateErr.Error(), "receipt records failing criteria (V1, V3)") {
		t.Fatalf("gate err = %v, want failing criteria V1, V3", gateErr)
	}

	// Same fixture with all criteria passing proceeds.
	bodyOK := shapeWithVerification("- **V1.** Root marker. Command: `true`. Expect: exit 0\n- **V3.** Also. Command: `true`. Expect: exit 0")
	if err := os.WriteFile(filepath.Join(dir, "shape.md"), []byte(bodyOK), 0o644); err != nil {
		t.Fatalf("WriteFile shape: %v", err)
	}
	commitAllChangeTest(t, repo, "docs: fix criteria")
	if err := (Runner{Stdout: &stdout, WorkingDir: repo}).Run([]string{"change", "verify", folderRel}); err != nil {
		t.Fatalf("verify pass: %v\n%s", err, stdout.String())
	}
	commitAllChangeTest(t, repo, "chore: commit passing receipt")
	if err := releaseCohortPreflight(repo, "1.0.0", nil); err != nil {
		t.Fatalf("passing receipt should open gate: %v", err)
	}
}

func TestReleaseCohortGateReceiptFreshnessBootstrap(t *testing.T) {
	repo := seedCohortGateRepo(t, "1.0.0-alpha.1")
	dir := writeNewLayoutChange(t, repo, "20260727-freshness", "freshness", "1.0.0", "")
	flipExecuteChange(t, repo, dir, "freshness")
	folderRel := filepath.Join("docs", "changes", "20260727-freshness")
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: repo}).Run([]string{"change", "verify", folderRel}); err != nil {
		t.Fatalf("verify: %v", err)
	}
	commitAllChangeTest(t, repo, "chore: commit verify receipt")

	if err := releaseCohortPreflight(repo, "1.0.0", nil); err != nil {
		t.Fatalf("receipt-only commit should not stale: %v", err)
	}

	// Decision 4 / ADR-024: touch-then-revert is deliberately undetectable —
	// byte-identical restore leaves the receipt fresh.
	other := filepath.Join(repo, "other.txt")
	if err := os.WriteFile(other, []byte("touch\n"), 0o644); err != nil {
		t.Fatalf("WriteFile other: %v", err)
	}
	commitAllChangeTest(t, repo, "chore: touch other")
	if err := os.Remove(other); err != nil {
		t.Fatalf("Remove other: %v", err)
	}
	commitAllChangeTest(t, repo, "chore: revert other")

	if err := releaseCohortPreflight(repo, "1.0.0", nil); err != nil {
		t.Fatalf("touch-then-revert must stay fresh under content digest: %v", err)
	}

	// A lasting content change stales with a typed drift reason.
	if err := os.WriteFile(filepath.Join(repo, "stale.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatalf("WriteFile stale: %v", err)
	}
	commitAllChangeTest(t, repo, "chore: lasting content change")
	gateErr := releaseCohortPreflight(repo, "1.0.0", nil)
	if gateErr == nil || !strings.Contains(gateErr.Error(), "content changed since verification") {
		t.Fatalf("lasting content change should drift: %v", gateErr)
	}
}

func TestChangeVerifyWritesReceiptOnFailure(t *testing.T) {
	repo := initCLIGitRepo(t)
	body := shapeWithVerification("- **V1.** Fail. Command: `false`. Expect: exit 0")
	dir := writeNewLayoutChange(t, repo, "20260727-write-fail", "write-fail", "", body)
	commitAllChangeTest(t, repo, "docs: shape write-fail")
	var stdout bytes.Buffer
	err := (Runner{Stdout: &stdout, WorkingDir: repo}).Run([]string{"change", "verify", filepath.Join("docs", "changes", "20260727-write-fail")})
	if err == nil {
		t.Fatal("expected verify failure")
	}
	receiptPath := filepath.Join(dir, "receipts", "verify.json")
	data, readErr := os.ReadFile(receiptPath)
	if readErr != nil {
		t.Fatalf("receipt should be written on failure: %v\n%s", readErr, stdout.String())
	}
	if !strings.Contains(string(data), `"ok": false`) {
		t.Fatalf("receipt = %s", data)
	}
}

// --- TASK-014: V1 / V2 / V5 residual Verification Contract fixtures ---

func TestReleaseCohortGateV1PathGradeIsNotFlipGrade(t *testing.T) {
	repo := seedCohortGateRepo(t, "2.0.0-alpha.1")
	dir := writeNewLayoutChange(t, repo, "20260727-v1-path-grade", "v1-path-grade", "2.0.0", "")
	task := filepath.Join(dir, "tasks", "TASK-001-work.md")
	if err := os.WriteFile(task, []byte("---\nchange: v1-path-grade\nid: TASK-001\ntitle: Work\n---\n\n# Work\n\n## Steps\n\n- [ ] Do it\n"), 0o644); err != nil {
		t.Fatalf("WriteFile task: %v", err)
	}
	commitAllChangeTest(t, repo, "docs: shape v1-path-grade")

	if err := os.WriteFile(filepath.Join(repo, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("WriteFile main.go: %v", err)
	}
	if err := os.WriteFile(task, []byte("---\nchange: v1-path-grade\nid: TASK-001\ntitle: Work\n---\n\n# Work\n\n## Steps\n\n- [ ] Do it\n\nnote\n"), 0o644); err != nil {
		t.Fatalf("WriteFile task touch: %v", err)
	}
	commitAllChangeTest(t, repo, "chore: path grade only")

	err := releaseCohortPreflight(repo, "2.0.0", nil)
	if err == nil || !strings.Contains(err.Error(), "not executed") {
		t.Fatalf("path grade without flip should block: %v", err)
	}
}

func TestReleaseCohortGateV1SecondShapedMemberBlocks(t *testing.T) {
	repo := seedCohortGateRepo(t, "2.0.0-alpha.1")
	dirA := writeNewLayoutChange(t, repo, "20260727-v1-member-a", "v1-member-a", "2.0.0", "")
	flipExecuteChange(t, repo, dirA, "v1-member-a")
	folderA := filepath.Join("docs", "changes", "20260727-v1-member-a")
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: repo}).Run([]string{"change", "verify", folderA}); err != nil {
		t.Fatalf("verify member-a: %v", err)
	}
	commitAllChangeTest(t, repo, "chore: verify member-a")

	writeNewLayoutChange(t, repo, "20260727-v1-member-b", "v1-member-b", "2.0.0", "")
	commitAllChangeTest(t, repo, "docs: shape member-b only")

	err := releaseCohortPreflight(repo, "2.0.0", nil)
	if err == nil || !strings.Contains(err.Error(), `change "v1-member-b" targets 2.0.0 but is not executed`) {
		t.Fatalf("second shaped-only member should block identically: %v", err)
	}
}

func TestReleaseCohortGateV1NoTargetNeverGates(t *testing.T) {
	repo := seedCohortGateRepo(t, "2.0.0-alpha.1")
	writeNewLayoutChange(t, repo, "20260727-v1-no-target", "v1-no-target", "", "")
	commitAllChangeTest(t, repo, "docs: shape untargeted change")

	if err := releaseCohortPreflight(repo, "2.0.0", nil); err != nil {
		t.Fatalf("change with no target_release must never gate: %v", err)
	}
}

func TestReleaseCohortGateV1LowerCohortWarnsWithoutBlocking(t *testing.T) {
	repo := seedCohortGateRepo(t, "2.0.0-alpha.1")
	writeNewLayoutChange(t, repo, "20260727-v1-lower", "v1-lower", "2.0.0", "")
	commitAllChangeTest(t, repo, "docs: shape lower cohort member")

	var warnings []string
	if err := releaseCohortPreflight(repo, "2.1.0", &warnings); err != nil {
		t.Fatalf("higher candidate must not block on lower cohort: %v", err)
	}
	if !findingsContain(warnings, "incomplete lower cohort") {
		t.Fatalf("warnings = %v, want lower-cohort warn without block", warnings)
	}
}

func assertPrereleaseBumpAndPostMergeBypass(t *testing.T, repo string) {
	t.Helper()
	snap, err := resolveReleaseSnapshot(repo, releaseOptions{bump: "prerelease"})
	pre := snap.Candidate
	if err != nil {
		t.Fatalf("compute prerelease candidate: %v", err)
	}
	if !releaseVersionIsPrerelease(pre) {
		t.Fatalf("prerelease candidate = %q, want prerelease", pre)
	}
	if err := releaseCohortPreflight(repo, pre, nil); err != nil {
		t.Fatalf("--bump prerelease should succeed: %v", err)
	}

	snap, err = resolveReleaseSnapshot(repo, releaseOptions{postMerge: true})
	post := snap.Candidate
	if err != nil {
		t.Fatalf("compute post-merge candidate: %v", err)
	}
	if !releaseVersionIsPrerelease(post) || post != snap.CurrentVersion {
		t.Fatalf("post-merge candidate = %q (current %q), want prepared prerelease", post, snap.CurrentVersion)
	}
	if err := releaseCohortPreflight(repo, post, nil); err != nil {
		t.Fatalf("--post-merge with prepared prerelease must bypass: %v", err)
	}
}

func TestReleaseCohortGateV2PrereleaseBypassEveryGateState(t *testing.T) {
	repo := seedCohortGateRepo(t, "1.0.0-alpha.1")
	body := shapeWithVerification("- **V1.** Smoke. Command: `true`. Expect: exit 0")
	dir := writeNewLayoutChange(t, repo, "20260727-v2-member", "v2-member", "1.0.0", body)
	commitAllChangeTest(t, repo, "docs: shape v2-member")
	folderRel := filepath.Join("docs", "changes", "20260727-v2-member")

	// Missing execution.
	assertPrereleaseBumpAndPostMergeBypass(t, repo)

	// Missing receipt (flip-executed, no verify).
	flipExecuteChange(t, repo, dir, "v2-member")
	assertPrereleaseBumpAndPostMergeBypass(t, repo)

	// Failing receipt.
	failBody := shapeWithVerification("- **V1.** Fail. Command: `false`. Expect: exit 0")
	if err := os.WriteFile(filepath.Join(dir, "shape.md"), []byte(failBody), 0o644); err != nil {
		t.Fatalf("WriteFile failing shape: %v", err)
	}
	commitAllChangeTest(t, repo, "docs: failing criteria")
	var stdout bytes.Buffer
	if err := (Runner{Stdout: &stdout, WorkingDir: repo}).Run([]string{"change", "verify", folderRel}); err == nil {
		t.Fatalf("verify should fail\n%s", stdout.String())
	}
	commitAllChangeTest(t, repo, "chore: commit failing receipt")
	assertPrereleaseBumpAndPostMergeBypass(t, repo)

	// Expired receipt (digest mismatch after criteria edit).
	okBody := shapeWithVerification("- **V1.** Smoke. Command: `true`. Expect: exit 0")
	if err := os.WriteFile(filepath.Join(dir, "shape.md"), []byte(okBody), 0o644); err != nil {
		t.Fatalf("WriteFile ok shape: %v", err)
	}
	commitAllChangeTest(t, repo, "docs: restore passing criteria")
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: repo}).Run([]string{"change", "verify", folderRel}); err != nil {
		t.Fatalf("verify pass: %v", err)
	}
	commitAllChangeTest(t, repo, "chore: commit passing receipt")

	expiredBody := shapeWithVerification("- **V1.** Smoke. Command: `true`. Expect: exit 0 and marker")
	if err := os.WriteFile(filepath.Join(dir, "shape.md"), []byte(expiredBody), 0o644); err != nil {
		t.Fatalf("WriteFile expired shape: %v", err)
	}
	commitAllChangeTest(t, repo, "docs: edit criteria expect")
	assertPrereleaseBumpAndPostMergeBypass(t, repo)

	// Cohort completes: prepared prerelease post-merge still publishes; --bump prerelease still bypasses.
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: repo}).Run([]string{"change", "verify", folderRel}); err != nil {
		t.Fatalf("re-verify after expiry: %v", err)
	}
	commitAllChangeTest(t, repo, "chore: re-verify after expiry")
	snap, err := resolveReleaseSnapshot(repo, releaseOptions{postMerge: true})
	post := snap.Candidate
	if err != nil {
		t.Fatalf("post-merge candidate: %v", err)
	}
	if err := releaseCohortPreflight(repo, post, nil); err != nil {
		t.Fatalf("completed cohort should allow post-merge: %v", err)
	}
	snap, err = resolveReleaseSnapshot(repo, releaseOptions{bump: "prerelease"})
	pre := snap.Candidate
	if err != nil {
		t.Fatalf("prerelease candidate: %v", err)
	}
	if err := releaseCohortPreflight(repo, pre, nil); err != nil {
		t.Fatalf("prerelease should still succeed after completion: %v", err)
	}
}

func TestReleaseCohortGateV5CriteriaEditExpiresReceipt(t *testing.T) {
	repo := seedCohortGateRepo(t, "1.0.0-alpha.1")
	body := shapeWithVerification("- **V1.** Smoke. Command: `true`. Expect: exit 0")
	dir := writeNewLayoutChange(t, repo, "20260727-v5-expire", "v5-expire", "1.0.0", body)
	flipExecuteChange(t, repo, dir, "v5-expire")
	folderRel := filepath.Join("docs", "changes", "20260727-v5-expire")
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: repo}).Run([]string{"change", "verify", folderRel}); err != nil {
		t.Fatalf("verify: %v", err)
	}
	commitAllChangeTest(t, repo, "chore: commit verify receipt")

	expired := shapeWithVerification("- **V1.** Smoke. Command: `true`. Expect: exit 0 changed")
	if err := os.WriteFile(filepath.Join(dir, "shape.md"), []byte(expired), 0o644); err != nil {
		t.Fatalf("WriteFile shape: %v", err)
	}
	commitAllChangeTest(t, repo, "docs: edit shape criteria")

	gateErr := releaseCohortPreflight(repo, "1.0.0", nil)
	if gateErr == nil || !strings.Contains(gateErr.Error(), "criteria changed (receipt expired)") {
		t.Fatalf("criteria edit should expire receipt: %v", gateErr)
	}
}

func TestReleaseCohortGateV5FreshnessRerunAndReceiptOwnCommit(t *testing.T) {
	repo := seedCohortGateRepo(t, "1.0.0-alpha.1")
	dir := writeNewLayoutChange(t, repo, "20260727-v5-fresh", "v5-fresh", "1.0.0", "")
	flipExecuteChange(t, repo, dir, "v5-fresh")
	folderRel := filepath.Join("docs", "changes", "20260727-v5-fresh")
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: repo}).Run([]string{"change", "verify", folderRel}); err != nil {
		t.Fatalf("verify: %v", err)
	}
	commitAllChangeTest(t, repo, "chore: commit verify receipt")

	if err := releaseCohortPreflight(repo, "1.0.0", nil); err != nil {
		t.Fatalf("receipt's own commit alone must not stale: %v", err)
	}

	if err := os.WriteFile(filepath.Join(repo, "later.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatalf("WriteFile later: %v", err)
	}
	commitAllChangeTest(t, repo, "chore: later non-receipt path")
	gateErr := releaseCohortPreflight(repo, "1.0.0", nil)
	if gateErr == nil || !strings.Contains(gateErr.Error(), "content changed since verification") {
		t.Fatalf("non-receipt path should force re-run: %v", gateErr)
	}
}

func TestReleaseCohortGateV5PlanMdEditStalesNotExpires(t *testing.T) {
	repo := seedCohortGateRepo(t, "1.0.0-alpha.1")
	body := shapeWithVerification("- **V1.** Smoke. Command: `true`. Expect: exit 0")
	dir := writeNewLayoutChange(t, repo, "20260727-v5-plan-stale", "v5-plan-stale", "1.0.0", body)
	flipExecuteChange(t, repo, dir, "v5-plan-stale")
	folderRel := filepath.Join("docs", "changes", "20260727-v5-plan-stale")
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: repo}).Run([]string{"change", "verify", folderRel}); err != nil {
		t.Fatalf("verify: %v", err)
	}
	commitAllChangeTest(t, repo, "chore: commit verify receipt")

	receipt, err := loadChangeVerifyReceipt(dir)
	if err != nil {
		t.Fatalf("load receipt: %v", err)
	}
	digestBefore := receipt.CriteriaDigest

	if err := os.WriteFile(filepath.Join(dir, "plan.md"), []byte("# Approach\n\nChurn.\n"), 0o644); err != nil {
		t.Fatalf("WriteFile plan.md: %v", err)
	}
	commitAllChangeTest(t, repo, "docs: edit plan.md only")

	shapeData, err := os.ReadFile(filepath.Join(dir, "shape.md"))
	if err != nil {
		t.Fatalf("ReadFile shape: %v", err)
	}
	digestNow := changeCriteriaDigest(parseChangeExecutableCriteria(string(shapeData)))
	if digestNow != digestBefore {
		t.Fatalf("plan.md edit must not change criteria digest: before=%s after=%s", digestBefore, digestNow)
	}
	receiptAfter, err := loadChangeVerifyReceipt(dir)
	if err != nil {
		t.Fatalf("load receipt after: %v", err)
	}
	if receiptAfter.CriteriaDigest != digestNow {
		t.Fatalf("receipt digest drifted: receipt=%s shape=%s", receiptAfter.CriteriaDigest, digestNow)
	}

	gateErr := releaseCohortPreflight(repo, "1.0.0", nil)
	if gateErr == nil {
		t.Fatal("plan.md-only commit should stale the receipt")
	}
	msg := gateErr.Error()
	if !strings.Contains(msg, "content changed since verification") {
		t.Fatalf("want content-drift demand, got: %v", gateErr)
	}
	if strings.Contains(msg, "receipt expired") {
		t.Fatalf("must not report expiry for plan.md edit: %v", gateErr)
	}
	remedy := "Run: loaf change verify " + filepath.ToSlash(folderRel) + ", then commit the receipt"
	if !strings.Contains(msg, remedy) {
		t.Fatalf("want mechanical remedy %q, got: %v", remedy, gateErr)
	}
}

func TestReleaseCohortGateV5RetargetAfterVerifyRequiresRerun(t *testing.T) {
	repo := seedCohortGateRepo(t, "2.0.0-alpha.1")
	body := shapeWithVerification("- **V1.** Smoke. Command: `true`. Expect: exit 0")
	dir := writeNewLayoutChange(t, repo, "20260727-v5-retarget", "v5-retarget", "2.0.0", body)
	flipExecuteChange(t, repo, dir, "v5-retarget")
	folderRel := filepath.Join("docs", "changes", "20260727-v5-retarget")
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: repo}).Run([]string{"change", "verify", folderRel}); err != nil {
		t.Fatalf("verify at 2.0.0: %v", err)
	}
	commitAllChangeTest(t, repo, "chore: commit verify receipt")

	meta := "{\n  \"change\": \"v5-retarget\",\n  \"created\": \"2026-07-27\",\n  \"branch\": \"v5-retarget\",\n  \"target_release\": \"2.1.0\"\n}\n"
	if err := os.WriteFile(filepath.Join(dir, "change.json"), []byte(meta), 0o644); err != nil {
		t.Fatalf("WriteFile change.json: %v", err)
	}
	commitAllChangeTest(t, repo, "chore: retarget 2.0.0 to 2.1.0")

	// Blind trust would accept the pre-retarget receipt; freshness must force re-run.
	gateErr := releaseCohortPreflight(repo, "2.1.0", nil)
	if gateErr == nil || !strings.Contains(gateErr.Error(), "content changed since verification") {
		t.Fatalf("retarget should trigger content drift: %v", gateErr)
	}

	// Not permanent invalidation: re-verify opens the new cohort.
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: repo}).Run([]string{"change", "verify", folderRel}); err != nil {
		t.Fatalf("re-verify after retarget: %v", err)
	}
	commitAllChangeTest(t, repo, "chore: re-verify after retarget")
	if err := releaseCohortPreflight(repo, "2.1.0", nil); err != nil {
		t.Fatalf("re-verify after retarget should open gate: %v", err)
	}
}

// --- TASK-016: the gate's structural tier is the composite check reports ---

func cohortStructuralReportForSlug(t *testing.T, repo, slug string) changeCheckReport {
	t.Helper()
	nodes, err := loadChangeNodesAtHEAD(repo)
	if err != nil {
		t.Fatalf("loadChangeNodesAtHEAD: %v", err)
	}
	for _, node := range nodes {
		if node.Slug != slug {
			continue
		}
		report, reportErr := changeCohortStructuralReport(repo, node, nodes, commandOutput)
		if reportErr != nil {
			t.Fatalf("changeCohortStructuralReport: %v", reportErr)
		}
		return report
	}
	t.Fatalf("no committed change %q at HEAD", slug)
	return changeCheckReport{}
}

func TestReleaseCohortGateBlocksExecutabilityGap(t *testing.T) {
	repo := seedCohortGateRepo(t, "1.0.0-alpha.1")
	gapBody := strings.Replace(authoredShapeBody(),
		"## Planning Contract\n\n### Approach\n\nHow.",
		"## Planning Contract\n\n<!-- not shaped yet -->", 1)
	dir := writeNewLayoutChange(t, repo, "20260727-gap-member", "gap-member", "1.0.0", gapBody)
	flipExecuteChange(t, repo, dir, "gap-member")
	folderRel := filepath.Join("docs", "changes", "20260727-gap-member")
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: repo}).Run([]string{"change", "verify", folderRel}); err != nil {
		t.Fatalf("verify: %v", err)
	}
	commitAllChangeTest(t, repo, "chore: commit verify receipt")

	// Zero violations, one contract gap: the tier the old gate could not see.
	report := cohortStructuralReportForSlug(t, repo, "gap-member")
	if len(report.Violations) != 0 || report.Executable {
		t.Fatalf("report = %+v, want zero violations and a gap", report)
	}

	gateErr := releaseCohortPreflight(repo, "1.0.0", nil)
	if gateErr == nil || !strings.Contains(gateErr.Error(), `change "gap-member" targets 1.0.0 but is not executable (contract gaps: Planning Contract (empty))`) {
		t.Fatalf("gate err = %v, want the executability-gap block", gateErr)
	}
	if strings.Contains(gateErr.Error(), "structurally invalid") {
		t.Fatalf("a gap is not a violation; got: %v", gateErr)
	}
	if !strings.Contains(gateErr.Error(), "run: loaf change check "+filepath.ToSlash(folderRel)) {
		t.Fatalf("want mechanical remedy naming check; got: %v", gateErr)
	}

	// check agrees on the same folder — one composite, two consumers.
	var checkOut bytes.Buffer
	checkErr := (Runner{Stdout: &checkOut, WorkingDir: repo}).Run([]string{"change", "check", folderRel, "--require-executable", "--json"})
	if checkErr == nil {
		t.Fatalf("change check --require-executable should fail on the same folder\n%s", checkOut.String())
	}
	for _, want := range []string{`"executable": false`, "Planning Contract (empty)"} {
		if !strings.Contains(checkOut.String(), want) {
			t.Fatalf("check output = %s, want %q", checkOut.String(), want)
		}
	}

	// Shaping the contract closes the gap; re-verify and the gate opens.
	if err := os.WriteFile(filepath.Join(dir, "shape.md"), []byte(authoredShapeBody()), 0o644); err != nil {
		t.Fatalf("WriteFile shape: %v", err)
	}
	commitAllChangeTest(t, repo, "docs: author the Planning Contract")
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: repo}).Run([]string{"change", "verify", folderRel}); err != nil {
		t.Fatalf("re-verify: %v", err)
	}
	commitAllChangeTest(t, repo, "chore: re-verify after shaping")
	if err := releaseCohortPreflight(repo, "1.0.0", nil); err != nil {
		t.Fatalf("shaped, executed, verified member should proceed: %v", err)
	}
}

func TestReleaseCohortGateBlocksTaskHygieneAndNeverBlocksOnWarnings(t *testing.T) {
	repo := seedCohortGateRepo(t, "1.0.0-alpha.1")
	dir := writeNewLayoutChange(t, repo, "20260727-hygiene-member", "hygiene-member", "1.0.0", "")
	flipExecuteChange(t, repo, dir, "hygiene-member")
	folderRel := filepath.Join("docs", "changes", "20260727-hygiene-member")
	later := filepath.Join(dir, "tasks", "TASK-002-later.md")
	if err := os.WriteFile(later, []byte("---\nchange: hygiene-member\nid: TASK-002\ntitle: Later\nstatus: in-progress\n---\n\n# Later\n\n## Steps\n\n- [ ] Descoped\n"), 0o644); err != nil {
		t.Fatalf("WriteFile TASK-002: %v", err)
	}
	commitAllChangeTest(t, repo, "docs: add a later task")
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: repo}).Run([]string{"change", "verify", folderRel}); err != nil {
		t.Fatalf("verify: %v", err)
	}
	commitAllChangeTest(t, repo, "chore: commit verify receipt")

	gateErr := releaseCohortPreflight(repo, "1.0.0", nil)
	if gateErr == nil || !strings.Contains(gateErr.Error(), `change "hygiene-member" targets 1.0.0 but is structurally invalid`) {
		t.Fatalf("gate err = %v, want the task-hygiene block", gateErr)
	}
	for _, want := range []string{"TASK-002-later.md", `task frontmatter key "status" is banned`} {
		if !strings.Contains(gateErr.Error(), want) {
			t.Fatalf("gate err = %v, want %q named", gateErr, want)
		}
	}

	// Drop the banned key, keep TASK-002 unchecked (legal descoped work) and add a
	// zero-checkbox coordination task (warning only).
	if err := os.WriteFile(later, []byte("---\nchange: hygiene-member\nid: TASK-002\ntitle: Later\n---\n\n# Later\n\n## Steps\n\n- [ ] Descoped\n"), 0o644); err != nil {
		t.Fatalf("WriteFile TASK-002 repair: %v", err)
	}
	parent := filepath.Join(dir, "tasks", "TASK-003-coordination.md")
	if err := os.WriteFile(parent, []byte("---\nchange: hygiene-member\nid: TASK-003\ntitle: Coordination\n---\n\n# Coordination\n\nNo boxes here.\n"), 0o644); err != nil {
		t.Fatalf("WriteFile TASK-003: %v", err)
	}
	commitAllChangeTest(t, repo, "docs: drop the banned key")
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: repo}).Run([]string{"change", "verify", folderRel}); err != nil {
		t.Fatalf("re-verify: %v", err)
	}
	commitAllChangeTest(t, repo, "chore: re-verify after repair")

	report := cohortStructuralReportForSlug(t, repo, "hygiene-member")
	if len(report.Violations) != 0 || !report.Executable {
		t.Fatalf("report = %+v, want a clean composite", report)
	}
	if len(report.Warnings) == 0 {
		t.Fatal("fixture must carry at least one warning for the never-block claim to bite")
	}
	if err := releaseCohortPreflight(repo, "1.0.0", nil); err != nil {
		t.Fatalf("warnings and unchecked descoped work must never block: %v", err)
	}
}

func TestReleaseCohortGateBlocksConversionFinding(t *testing.T) {
	repo := seedCohortGateRepo(t, "2.0.0-alpha.1")
	writeChangeFolder(t, repo, "20260727-prechecked-convert", changeDoc(
		"---\nchange: prechecked-convert\ncreated: 2026-07-27\nbranch: prechecked-convert\ntarget_release: 2.0.0\n---\n",
		append(productSections(), executableSections()...)...,
	))
	commitAllChangeTest(t, repo, "docs: add legacy targeted member")
	atomicConvertFolder(t, repo, "20260727-prechecked-convert", "prechecked-convert", true)
	commitAllChangeTest(t, repo, "docs: convert with a pre-checked box")

	// Manufactured execution: a conversion finding is a violation at check, so it
	// is a violation at the gate too.
	gateErr := releaseCohortPreflight(repo, "2.0.0", nil)
	if gateErr == nil || !strings.Contains(gateErr.Error(), "is structurally invalid") {
		t.Fatalf("gate err = %v, want the conversion block", gateErr)
	}
	if !strings.Contains(gateErr.Error(), "checked task checkbox") {
		t.Fatalf("gate err = %v, want the conversion finding named", gateErr)
	}
}

// --- TASK-021: lineage validation joins the gate composite ---

func TestReleaseCohortGateBlocksDuplicateSlugLineageFinding(t *testing.T) {
	repo := seedCohortGateRepo(t, "1.0.0-alpha.1")
	dirA := writeNewLayoutChange(t, repo, "20260727-dup-slug", "dup-slug", "1.0.0", "")
	flipExecuteChange(t, repo, dirA, "dup-slug")
	folderA := filepath.Join("docs", "changes", "20260727-dup-slug")
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: repo}).Run([]string{"change", "verify", folderA}); err != nil {
		t.Fatalf("verify A: %v", err)
	}
	commitAllChangeTest(t, repo, "chore: commit A receipt")

	dirB := writeNewLayoutChange(t, repo, "20260728-dup-slug", "dup-slug", "1.0.0", "")
	flipExecuteChange(t, repo, dirB, "dup-slug")
	folderB := filepath.Join("docs", "changes", "20260728-dup-slug")
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: repo}).Run([]string{"change", "verify", folderB}); err != nil {
		t.Fatalf("verify B: %v", err)
	}
	commitAllChangeTest(t, repo, "chore: commit B receipt")

	gateErr := releaseCohortPreflight(repo, "1.0.0", nil)
	if gateErr == nil || !strings.Contains(gateErr.Error(), "structurally invalid") {
		t.Fatalf("gate err = %v, want structural block for duplicate slug", gateErr)
	}
	if !strings.Contains(gateErr.Error(), "duplicate Change slug") {
		t.Fatalf("gate err = %v, want the duplication named", gateErr)
	}

	for _, folder := range []string{folderA, folderB} {
		var stdout bytes.Buffer
		checkErr := (Runner{Stdout: &stdout, WorkingDir: repo}).Run([]string{"change", "check", folder, "--json"})
		if checkErr == nil {
			t.Fatalf("check %s should fail for duplicate slug\n%s", folder, stdout.String())
		}
		if !strings.Contains(stdout.String(), "duplicate Change slug") {
			t.Fatalf("check %s output = %q, want duplicate Change slug", folder, stdout.String())
		}
	}
}

func TestReleaseCohortGateLineageHappyPathUnaffectedByForeignLineageFindings(t *testing.T) {
	// Single-member cohort in a clean repo stays green: lineage findings that
	// belong to other changes must not over-block (matching check's scoping).
	repo := seedCohortGateRepo(t, "1.0.0-alpha.1")
	dir := writeNewLayoutChange(t, repo, "20260727-lineage-happy", "lineage-happy", "1.0.0", "")
	flipExecuteChange(t, repo, dir, "lineage-happy")
	folderRel := filepath.Join("docs", "changes", "20260727-lineage-happy")
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: repo}).Run([]string{"change", "verify", folderRel}); err != nil {
		t.Fatalf("verify: %v", err)
	}
	commitAllChangeTest(t, repo, "chore: commit lineage-happy receipt")

	report := cohortStructuralReportForSlug(t, repo, "lineage-happy")
	if len(report.Violations) != 0 || !report.Executable {
		t.Fatalf("report = %+v, want a clean composite", report)
	}
	if err := releaseCohortPreflight(repo, "1.0.0", nil); err != nil {
		t.Fatalf("single-member happy path must stay green: %v", err)
	}
}

func TestReleaseAndStateIgnoreUncommittedDuplicateSlugRename(t *testing.T) {
	repo := seedCohortGateRepo(t, "1.0.0-alpha.1")
	dirA := writeNewLayoutChange(t, repo, "20260727-dup-slug", "dup-slug", "1.0.0", "")
	flipExecuteChange(t, repo, dirA, "dup-slug")
	folderA := filepath.Join("docs", "changes", "20260727-dup-slug")
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: repo}).Run([]string{"change", "verify", folderA}); err != nil {
		t.Fatalf("verify A: %v", err)
	}
	commitAllChangeTest(t, repo, "chore: commit A receipt")

	dirB := writeNewLayoutChange(t, repo, "20260728-dup-slug", "dup-slug", "1.0.0", "")
	flipExecuteChange(t, repo, dirB, "dup-slug")
	folderB := filepath.Join("docs", "changes", "20260728-dup-slug")
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: repo}).Run([]string{"change", "verify", folderB}); err != nil {
		t.Fatalf("verify B: %v", err)
	}
	commitAllChangeTest(t, repo, "chore: commit B receipt")

	gateErr := releaseCohortPreflight(repo, "1.0.0", nil)
	if gateErr == nil || !strings.Contains(gateErr.Error(), "duplicate Change slug") {
		t.Fatalf("committed gate err = %v, want duplicate slug block", gateErr)
	}
	stateA := deriveChangeState(repo, mustAssembleNode(t, repo, folderA), commandOutput)
	if stateA == "verified" {
		t.Fatalf("committed duplicate must keep state off verified; got %q", stateA)
	}

	// Uncommitted move of one duplicate folder out of docs/changes: check's working-tree load loses the duplicate; gate/state still see HEAD.
	parked := filepath.Join(repo, "parked-dup-slug")
	if err := os.Rename(dirB, parked); err != nil {
		t.Fatalf("Rename: %v", err)
	}

	gateErr = releaseCohortPreflight(repo, "1.0.0", nil)
	if gateErr == nil || !strings.Contains(gateErr.Error(), "duplicate Change slug") {
		t.Fatalf("after WT rename gate err = %v, want committed duplicate still blocking", gateErr)
	}
	stateA = deriveChangeState(repo, mustAssembleNode(t, repo, folderA), commandOutput)
	if stateA == "verified" {
		t.Fatalf("after WT rename state must stay off verified; got %q", stateA)
	}

	var stdout bytes.Buffer
	checkErr := (Runner{Stdout: &stdout, WorkingDir: repo}).Run([]string{"change", "check", folderA, "--json"})
	if strings.Contains(stdout.String(), "duplicate Change slug") {
		t.Fatalf("check should see the working tree (duplicate parked away); got err=%v out=%s", checkErr, stdout.String())
	}
}

// TASK-030: a commit landing after snapshot resolution appears in neither the
// changelog nor the bump — both describe the snapshot's frozen history.
func TestReleaseSnapshotChangelogIgnoresPostResolveCommits(t *testing.T) {
	repo := seedCohortGateRepo(t, "1.0.0-alpha.1")
	if err := os.WriteFile(filepath.Join(repo, "feature.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("WriteFile feature.go: %v", err)
	}
	commitAllChangeTest(t, repo, "feat: pre-resolve work")

	snap, err := resolveReleaseSnapshot(repo, releaseOptions{bump: "prerelease"})
	if err != nil {
		t.Fatalf("resolve snapshot: %v", err)
	}
	if snap.Candidate != "1.0.0-alpha.2" {
		t.Fatalf("candidate = %q, want 1.0.0-alpha.2", snap.Candidate)
	}
	preHashes := map[string]bool{}
	for _, c := range snap.Commits {
		preHashes[c.Hash] = true
	}

	if err := os.WriteFile(filepath.Join(repo, "after.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("WriteFile after.go: %v", err)
	}
	commitAllChangeTest(t, repo, "feat: post-resolve must not enter changelog")

	var stdout bytes.Buffer
	opts := releaseOptions{bump: "prerelease", dryRun: true, tagSet: true, tag: false, ghSet: true, gh: false, snapshot: snap}
	if err := runReleaseDryRun(repo, opts, &stdout, &bytes.Buffer{}); err != nil {
		t.Fatalf("dry-run with frozen snapshot: %v\n%s", err, stdout.String())
	}
	out := stripANSI(stdout.String())
	if !strings.Contains(out, "New version: 1.0.0-alpha.2") {
		t.Fatalf("dry-run must keep snapshot candidate; got:\n%s", out)
	}
	if strings.Contains(out, "post-resolve must not enter changelog") {
		t.Fatalf("changelog must not include post-resolve commit; got:\n%s", out)
	}
	for _, c := range snap.Commits {
		if !preHashes[c.Hash] {
			t.Fatalf("snapshot commits mutated after resolve")
		}
	}
	if len(releaseCommitsSince(repo, snap.BaseRef)) <= len(snap.Commits) {
		t.Fatalf("expected HEAD to have grown past the snapshot commit list")
	}
}

// TASK-031: prepared prerelease post-merge publishes and would tag the prepared version (alpha-train).
func TestReleasePostMergePreparedPrereleasePublishesAlphaTrain(t *testing.T) {
	repo := seedCohortGateRepo(t, "2.0.0-alpha.15")
	writeNewLayoutChange(t, repo, "20260727-alpha-train", "alpha-train", "2.0.0", "")
	commitAllChangeTest(t, repo, "docs: open 2.0.0 cohort")

	snap, err := resolveReleaseSnapshot(repo, releaseOptions{postMerge: true})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if snap.Candidate != "2.0.0-alpha.15" {
		t.Fatalf("candidate = %q, want prepared 2.0.0-alpha.15", snap.Candidate)
	}
	if err := releaseCohortPreflight(repo, snap.Candidate, nil); err != nil {
		t.Fatalf("prepared prerelease must bypass open 2.0.0 cohort: %v", err)
	}

	files := seedReleasePostMergeFiles(t, "2.0.0-alpha.15")
	fileSnap := mustResolveReleaseSnapshot(t, files, releaseOptions{postMerge: true})
	responses := releasePostMergeHappyResponses("2.0.0-alpha.15")
	responses["gh release create v2.0.0-alpha.15 --title v2.0.0-alpha.15 --notes ### Added\n- New feature (abc1234) --prerelease"] = releasePostMergeOK("")
	runner, calls := scriptedReleasePostMergeRunner(responses)
	var stdout, stderr bytes.Buffer
	if err := runReleasePostMergeWithRunner(files, fileSnap, &stdout, &stderr, runner); err != nil {
		t.Fatalf("post-merge: %v\n%s\n%s", err, stdout.String(), stderr.String())
	}
	out := stripANSI(stdout.String())
	if !strings.Contains(out, "Created tag v2.0.0-alpha.15") {
		t.Fatalf("must tag prepared prerelease; got:\n%s", out)
	}
	keys := releasePostMergeCallKeys(calls())
	if !containsReleasePostMergeCall(keys, "git tag -s v2.0.0-alpha.15 -m Release 2.0.0-alpha.15") {
		t.Fatalf("missing prepared tag call; got %#v", keys)
	}
	for _, call := range keys {
		if call == "git tag -s v2.0.0 -m Release 2.0.0" {
			t.Fatalf("must not tag stable; calls=%#v", keys)
		}
	}
}

// TASK-031: prepared stable post-merge gates the cohort; verified cohort tags the prepared stable.
func TestReleasePostMergePreparedStableGatesThenTags(t *testing.T) {
	repo := seedCohortGateRepo(t, "2.0.0")
	dir := writeNewLayoutChange(t, repo, "20260727-stable-prep", "stable-prep", "2.0.0", "")
	commitAllChangeTest(t, repo, "docs: shape stable-prep")

	snap, err := resolveReleaseSnapshot(repo, releaseOptions{postMerge: true})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if snap.Candidate != "2.0.0" {
		t.Fatalf("candidate = %q, want prepared 2.0.0", snap.Candidate)
	}
	gateErr := releaseCohortPreflight(repo, snap.Candidate, nil)
	if gateErr == nil || !strings.Contains(gateErr.Error(), "not executed") {
		t.Fatalf("open cohort must block prepared-stable post-merge: %v", gateErr)
	}

	flipExecuteChange(t, repo, dir, "stable-prep")
	folderRel := relFromRoot(repo, dir)
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: repo}).Run([]string{"change", "verify", folderRel}); err != nil {
		t.Fatalf("verify: %v", err)
	}
	commitAllChangeTest(t, repo, "chore: commit stable-prep receipt")
	if err := releaseCohortPreflight(repo, snap.Candidate, nil); err != nil {
		t.Fatalf("verified cohort must open: %v", err)
	}

	files := seedReleasePostMergeFiles(t, "2.0.0")
	fileSnap := mustResolveReleaseSnapshot(t, files, releaseOptions{postMerge: true})
	runner, _ := scriptedReleasePostMergeRunner(releasePostMergeHappyResponses("2.0.0"))
	var stdout, stderr bytes.Buffer
	if err := runReleasePostMergeWithRunner(files, fileSnap, &stdout, &stderr, runner); err != nil {
		t.Fatalf("post-merge stable: %v\n%s", err, stdout.String())
	}
	if !strings.Contains(stripANSI(stdout.String()), "Created tag v2.0.0") {
		t.Fatalf("must tag prepared stable; got:\n%s", stdout.String())
	}
}

// TASK-031 converse hazard: stray ## [2.0.0] with prerelease files cannot cause a stable tag.
func TestReleasePostMergeConverseHazardStrayStableChangelog(t *testing.T) {
	repo := seedReleasePostMergeFiles(t, "2.0.0-alpha.15")
	// Replace changelog so only the stable section exists — prepared lookup must fail closed.
	writeFile(t, filepath.Join(repo, "CHANGELOG.md"), strings.Join([]string{
		"# Changelog",
		"",
		"## [Unreleased]",
		"",
		"- _No unreleased changes yet._",
		"",
		"## [2.0.0] - 2026-04-29",
		"",
		"### Added",
		"- Stray stable section (abc1234)",
		"",
	}, "\n"))
	snap := mustResolveReleaseSnapshot(t, repo, releaseOptions{postMerge: true})
	if snap.Candidate != "2.0.0-alpha.15" {
		t.Fatalf("candidate = %q, want prepared prerelease", snap.Candidate)
	}
	responses := releasePostMergeHappyResponses("2.0.0-alpha.15")
	runner, calls := scriptedReleasePostMergeRunner(responses)
	var stdout, stderr bytes.Buffer
	err := runReleasePostMergeWithRunner(repo, snap, &stdout, &stderr, runner)
	if err == nil {
		t.Fatal("post-merge error = nil, want missing prepared changelog section")
	}
	if !strings.Contains(err.Error(), "2.0.0-alpha.15") {
		t.Fatalf("error = %v, want prepared-version changelog demand", err)
	}
	for _, call := range releasePostMergeCallKeys(calls()) {
		if strings.Contains(call, "tag -s") {
			t.Fatalf("must not tag; calls=%#v", releasePostMergeCallKeys(calls()))
		}
	}
}

// TASK-031: guardrail 4 refuses when the snapshot candidate diverges from version files.
func TestReleasePostMergeGuardrail4TagEqualsVersionFiles(t *testing.T) {
	repo := seedReleasePostMergeFiles(t, "2.0.0-alpha.15")
	snap := mustResolveReleaseSnapshot(t, repo, releaseOptions{postMerge: true})
	snap.Candidate = "2.0.0" // simulate the old strip-to-stable bug
	runner, calls := scriptedReleasePostMergeRunner(releasePostMergeHappyResponses("2.0.0"))
	result := checkReleasePostMergeGuardrails(repo, snap, runner)
	if result.ok || result.guardrail != 4 {
		t.Fatalf("result = %#v, want guardrail 4 abort", result)
	}
	if !strings.Contains(result.message, "does not match version-file version") {
		t.Fatalf("message = %q, want tag-equals-files", result.message)
	}
	for _, call := range releasePostMergeCallKeys(calls()) {
		if strings.Contains(call, "tag -s") {
			t.Fatalf("must not tag; calls=%#v", releasePostMergeCallKeys(calls()))
		}
	}
}
