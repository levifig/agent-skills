package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/levifig/loaf/internal/project"
	"github.com/levifig/loaf/internal/state"
)

func releaseTrackFixture(t *testing.T) (repo, stateHome string) {
	t.Helper()
	repo = seedReleaseTaggedRepo(t)
	stateHome = t.TempDir()
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: repo, StateHome: stateHome}).Run([]string{"state", "init"}); err != nil {
		t.Fatalf("state init error = %v", err)
	}
	return repo, stateHome
}

func runReleaseTrack(t *testing.T, repo, stateHome string, args ...string) (string, error) {
	t.Helper()
	stdout, _, err := runReleaseTrackIO(t, repo, stateHome, args...)
	return stdout, err
}

func runReleaseTrackIO(t *testing.T, repo, stateHome string, args ...string) (string, string, error) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	err := Runner{Stdout: &stdout, Stderr: &stderr, WorkingDir: repo, StateHome: stateHome}.Run(append([]string{"release"}, args...))
	return stdout.String(), stderr.String(), err
}

func TestReleaseSuggestGroupsLandedWorkAndSuggestsBump(t *testing.T) {
	repo, stateHome := releaseTrackFixture(t)
	if _, err := runIssue(t, repo, stateHome, "new", "Ship auth"); err != nil {
		t.Fatalf("issue new error = %v", err)
	}
	writeFile(t, filepath.Join(repo, "auth.txt"), "auth\n")
	gitCLI(t, repo, "add", "auth.txt")
	gitCLI(t, repo, "commit", "-m", "feat: add auth for LOAF-1")

	out, err := runReleaseTrack(t, repo, stateHome, "suggest")
	if err != nil {
		t.Fatalf("release suggest error = %v\n%s", err, out)
	}
	if !strings.Contains(out, "LOAF-1 — Ship auth") {
		t.Fatalf("suggest missing landed issue:\n%s", out)
	}
	if !strings.Contains(out, "Suggested bump: minor → 1.1.0") {
		t.Fatalf("suggest bump = %q, want minor → 1.1.0", out)
	}
	if !strings.Contains(out, "feat: add auth for LOAF-1") {
		t.Fatalf("suggest missing commit:\n%s", out)
	}

	jsonOut, err := runReleaseTrack(t, repo, stateHome, "suggest", "--json")
	if err != nil {
		t.Fatalf("release suggest --json error = %v\n%s", err, jsonOut)
	}
	var suggestion releaseTrackSuggestion
	if err := json.Unmarshal([]byte(jsonOut), &suggestion); err != nil {
		t.Fatalf("unmarshal suggestion: %v\n%s", err, jsonOut)
	}
	if suggestion.SuggestedBump != "minor" || suggestion.SuggestedVersion != "1.1.0" {
		t.Fatalf("json bump/version = %s/%s", suggestion.SuggestedBump, suggestion.SuggestedVersion)
	}
	if len(suggestion.Landed) != 1 || suggestion.Landed[0].Alias != "LOAF-1" {
		t.Fatalf("json landed = %#v", suggestion.Landed)
	}
}

func TestReleaseSuggestReportsPartialParentWithoutBlocking(t *testing.T) {
	repo, stateHome := releaseTrackFixture(t)
	if _, err := runIssue(t, repo, stateHome, "new", "Parent"); err != nil {
		t.Fatalf("issue new parent error = %v", err)
	}
	if _, err := runIssue(t, repo, stateHome, "new", "Child A", "--parent", "LOAF-1"); err != nil {
		t.Fatalf("issue new child A error = %v", err)
	}
	if _, err := runIssue(t, repo, stateHome, "new", "Child B", "--parent", "LOAF-1"); err != nil {
		t.Fatalf("issue new child B error = %v", err)
	}
	if _, err := runIssue(t, repo, stateHome, "status", "LOAF-2", "done"); err != nil {
		t.Fatalf("status child A error = %v", err)
	}
	writeFile(t, filepath.Join(repo, "child-a.txt"), "a\n")
	gitCLI(t, repo, "add", "child-a.txt")
	gitCLI(t, repo, "commit", "-m", "feat: land child A LOAF-2")

	out, err := runReleaseTrack(t, repo, stateHome, "suggest")
	if err != nil {
		t.Fatalf("release suggest error = %v\n%s", err, out)
	}
	if !strings.Contains(out, "LOAF-1 — Parent") || !strings.Contains(out, "missing LOAF-3") {
		t.Fatalf("suggest missing partial parent:\n%s", out)
	}
}

func TestReleaseSuggestNeverDropsUnattributedCommits(t *testing.T) {
	repo, stateHome := releaseTrackFixture(t)
	if _, err := runIssue(t, repo, stateHome, "new", "Documented"); err != nil {
		t.Fatalf("issue new error = %v", err)
	}
	writeFile(t, filepath.Join(repo, "docs.txt"), "docs\n")
	gitCLI(t, repo, "add", "docs.txt")
	gitCLI(t, repo, "commit", "-m", "docs: mention LOAF-1")
	writeFile(t, filepath.Join(repo, "orphan.txt"), "orphan\n")
	gitCLI(t, repo, "add", "orphan.txt")
	gitCLI(t, repo, "commit", "-m", "chore: no issue at all")

	out, err := runReleaseTrack(t, repo, stateHome, "suggest")
	if err != nil {
		t.Fatalf("release suggest error = %v\n%s", err, out)
	}
	if !strings.Contains(out, "chore: no issue at all") {
		t.Fatalf("unattributed commit was dropped:\n%s", out)
	}
	if !strings.Contains(out, "LOAF-1 — Documented") {
		t.Fatalf("attributed commit missing:\n%s", out)
	}
}

func TestReleaseSuggestAttributesMergeBranchAndJournal(t *testing.T) {
	repo, stateHome := releaseTrackFixture(t)
	if _, err := runIssue(t, repo, stateHome, "new", "Merge work"); err != nil {
		t.Fatalf("issue new merge error = %v", err)
	}
	if _, err := runIssue(t, repo, stateHome, "new", "Journal work"); err != nil {
		t.Fatalf("issue new journal error = %v", err)
	}

	gitCLI(t, repo, "checkout", "-b", "feat/loaf-1-merge")
	writeFile(t, filepath.Join(repo, "merge.txt"), "merge\n")
	gitCLI(t, repo, "add", "merge.txt")
	gitCLI(t, repo, "commit", "-m", "feat: do the thing")
	gitCLI(t, repo, "checkout", "main")
	gitCLI(t, repo, "merge", "--no-ff", "-m", "Merge branch 'feat/loaf-1-merge'", "feat/loaf-1-merge")

	writeFile(t, filepath.Join(repo, "journal.txt"), "journal\n")
	gitCLI(t, repo, "add", "journal.txt")
	gitCLI(t, repo, "commit", "-m", "fix: mapped only in the journal")
	short := gitOutputReleaseTest(t, repo, "log", "-1", "--pretty=%h")
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: repo, StateHome: stateHome}).Run([]string{
		"journal", "log", "commit(" + short + "): LOAF-2",
	}); err != nil {
		t.Fatalf("journal log error = %v", err)
	}

	out, err := runReleaseTrack(t, repo, stateHome, "suggest")
	if err != nil {
		t.Fatalf("release suggest error = %v\n%s", err, out)
	}
	if !strings.Contains(out, "LOAF-1 — Merge work") {
		t.Fatalf("merge branch alias not attributed:\n%s", out)
	}
	if !strings.Contains(out, "LOAF-2 — Journal work") {
		t.Fatalf("journal mapping not attributed:\n%s", out)
	}
}

func TestReleaseSuggestFullyLandedParentDerivesMinor(t *testing.T) {
	repo, stateHome := releaseTrackFixture(t)
	if _, err := runIssue(t, repo, stateHome, "new", "Parent"); err != nil {
		t.Fatalf("issue new parent error = %v", err)
	}
	if _, err := runIssue(t, repo, stateHome, "new", "Child A", "--parent", "LOAF-1"); err != nil {
		t.Fatalf("issue new child A error = %v", err)
	}
	if _, err := runIssue(t, repo, stateHome, "new", "Child B", "--parent", "LOAF-1"); err != nil {
		t.Fatalf("issue new child B error = %v", err)
	}
	for _, alias := range []string{"LOAF-1", "LOAF-2", "LOAF-3"} {
		if _, err := runIssue(t, repo, stateHome, "status", alias, "done"); err != nil {
			t.Fatalf("status %s error = %v", alias, err)
		}
	}
	writeFile(t, filepath.Join(repo, "a.txt"), "a\n")
	gitCLI(t, repo, "add", "a.txt")
	gitCLI(t, repo, "commit", "-m", "chore: land LOAF-2")
	writeFile(t, filepath.Join(repo, "b.txt"), "b\n")
	gitCLI(t, repo, "add", "b.txt")
	gitCLI(t, repo, "commit", "-m", "chore: land LOAF-3")

	out, err := runReleaseTrack(t, repo, stateHome, "suggest")
	if err != nil {
		t.Fatalf("release suggest error = %v\n%s", err, out)
	}
	if !strings.Contains(out, "Suggested bump: minor → 1.1.0") {
		t.Fatalf("fully landed parent should bump minor:\n%s", out)
	}
	if !strings.Contains(out, "closed multi-child parent LOAF-1 fully landed") {
		t.Fatalf("missing parent evidence:\n%s", out)
	}
}

func TestReleaseSuggestBreakingCommitDerivesMajor(t *testing.T) {
	repo, stateHome := releaseTrackFixture(t)
	if _, err := runIssue(t, repo, stateHome, "new", "Break"); err != nil {
		t.Fatalf("issue new error = %v", err)
	}
	writeFile(t, filepath.Join(repo, "break.txt"), "break\n")
	gitCLI(t, repo, "add", "break.txt")
	gitCLI(t, repo, "commit", "-m", "feat!: drop the old API LOAF-1")

	out, err := runReleaseTrack(t, repo, stateHome, "suggest")
	if err != nil {
		t.Fatalf("release suggest error = %v\n%s", err, out)
	}
	if !strings.Contains(out, "Suggested bump: major → 2.0.0") {
		t.Fatalf("breaking commit should bump major:\n%s", out)
	}
}

func TestReleaseSuggestReadsBucketsWithoutConstraining(t *testing.T) {
	repo, stateHome := releaseTrackFixture(t)
	if _, err := runIssue(t, repo, stateHome, "new", "Now work"); err != nil {
		t.Fatalf("issue new now error = %v", err)
	}
	if _, err := runIssue(t, repo, stateHome, "new", "Later work"); err != nil {
		t.Fatalf("issue new later error = %v", err)
	}
	if _, err := runIssue(t, repo, stateHome, "bucket", "LOAF-1", "now"); err != nil {
		t.Fatalf("bucket now error = %v", err)
	}
	if _, err := runIssue(t, repo, stateHome, "bucket", "LOAF-2", "later"); err != nil {
		t.Fatalf("bucket later error = %v", err)
	}
	writeFile(t, filepath.Join(repo, "now.txt"), "now\n")
	gitCLI(t, repo, "add", "now.txt")
	gitCLI(t, repo, "commit", "-m", "feat: land now work LOAF-1")

	out, err := runReleaseTrack(t, repo, stateHome, "suggest")
	if err != nil {
		t.Fatalf("release suggest error = %v\n%s", err, out)
	}
	if !strings.Contains(out, "bucket:now LOAF-1") || !strings.Contains(out, "(landed)") {
		t.Fatalf("missing planned landed bucket:\n%s", out)
	}
	if !strings.Contains(out, "bucket:later LOAF-2") || !strings.Contains(out, "(not landed)") {
		t.Fatalf("missing planned unlanded bucket:\n%s", out)
	}
}

func TestReleaseCutRecordsMembersAndDryRunWritesNothing(t *testing.T) {
	repo, stateHome := releaseTrackFixture(t)
	if _, err := runIssue(t, repo, stateHome, "new", "Ship auth"); err != nil {
		t.Fatalf("issue new error = %v", err)
	}
	if _, err := runIssue(t, repo, stateHome, "status", "LOAF-1", "done"); err != nil {
		t.Fatalf("status error = %v", err)
	}
	writeFile(t, filepath.Join(repo, "auth.txt"), "auth\n")
	gitCLI(t, repo, "add", "auth.txt")
	gitCLI(t, repo, "commit", "-m", "feat: add auth LOAF-1")

	beforePkg, err := os.ReadFile(filepath.Join(repo, "package.json"))
	if err != nil {
		t.Fatalf("ReadFile package.json: %v", err)
	}
	beforeLog, err := os.ReadFile(filepath.Join(repo, "CHANGELOG.md"))
	if err != nil {
		t.Fatalf("ReadFile CHANGELOG.md: %v", err)
	}
	beforeHEAD := gitOutputReleaseTest(t, repo, "rev-parse", "HEAD")

	dryOut, err := runReleaseTrack(t, repo, stateHome, "cut", "--dry-run", "--no-gh")
	if err != nil {
		t.Fatalf("release cut --dry-run error = %v\n%s", err, dryOut)
	}
	if !strings.Contains(dryOut, "nothing written") {
		t.Fatalf("dry-run output = %q", dryOut)
	}
	afterPkg, err := os.ReadFile(filepath.Join(repo, "package.json"))
	if err != nil {
		t.Fatal(err)
	}
	afterLog, err := os.ReadFile(filepath.Join(repo, "CHANGELOG.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(beforePkg, afterPkg) || !bytes.Equal(beforeLog, afterLog) {
		t.Fatal("dry-run mutated version files or changelog")
	}
	if got := gitOutputReleaseTest(t, repo, "rev-parse", "HEAD"); got != beforeHEAD {
		t.Fatalf("dry-run moved HEAD from %s to %s", beforeHEAD, got)
	}
	if tags := gitOutputReleaseTest(t, repo, "tag", "--list"); tags != "v1.0.0" {
		t.Fatalf("dry-run tags = %q", tags)
	}
	root, err := project.ResolveRoot(repo)
	if err != nil {
		t.Fatalf("ResolveRoot: %v", err)
	}
	listed, err := state.ListReleases(t.Context(), root, state.PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("ListReleases after dry-run: %v", err)
	}
	if len(listed) != 0 {
		t.Fatalf("dry-run wrote %d release rows", len(listed))
	}

	gitCLI(t, repo, "tag", "v1.1.0")
	tagCommit := gitOutputReleaseTest(t, repo, "rev-parse", "v1.1.0^{commit}")
	cutOut, err := runReleaseTrack(t, repo, stateHome, "cut", "--no-tag", "--no-gh", "--base", "v1.0.0")
	if err != nil {
		t.Fatalf("release cut error = %v\n%s", err, cutOut)
	}
	if !strings.Contains(cutOut, "Recorded release v1.1.0") {
		t.Fatalf("cut output = %q", cutOut)
	}
	pkg, err := os.ReadFile(filepath.Join(repo, "package.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(pkg), `"version": "1.1.0"`) {
		t.Fatalf("package.json = %s", pkg)
	}
	logBody, err := os.ReadFile(filepath.Join(repo, "CHANGELOG.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(logBody), "## [1.1.0]") {
		t.Fatalf("CHANGELOG.md missing release section:\n%s", logBody)
	}
	if tags := gitOutputReleaseTest(t, repo, "tag", "--list"); !strings.Contains(tags, "v1.0.0") || !strings.Contains(tags, "v1.1.0") {
		t.Fatalf("cut --no-tag tags = %q", tags)
	}
	recorded, err := state.GetRelease(t.Context(), root, state.PathResolver{StateHome: stateHome}, "v1.1.0")
	if err != nil {
		t.Fatalf("GetRelease: %v", err)
	}
	if recorded.TaggedCommit != tagCommit {
		t.Fatalf("tagged_commit = %q, want tag commit %q", recorded.TaggedCommit, tagCommit)
	}
	if len(recorded.Members) != 1 || recorded.Members[0].Kind != state.ReleaseMemberKindIssue {
		t.Fatalf("members = %#v", recorded.Members)
	}
	issue, err := state.GetIssue(t.Context(), root, state.PathResolver{StateHome: stateHome}, "LOAF-1")
	if err != nil {
		t.Fatalf("GetIssue: %v", err)
	}
	if recorded.Members[0].MemberID != issue.ID {
		t.Fatalf("member id = %q, want %q", recorded.Members[0].MemberID, issue.ID)
	}
}

func TestReleaseCutIncludesPrereleaseByReference(t *testing.T) {
	repo, stateHome := releaseTrackFixture(t)
	if _, err := runIssue(t, repo, stateHome, "new", "Alpha"); err != nil {
		t.Fatalf("issue new error = %v", err)
	}
	writeFile(t, filepath.Join(repo, "alpha.txt"), "alpha\n")
	gitCLI(t, repo, "add", "alpha.txt")
	gitCLI(t, repo, "commit", "-m", "feat: alpha LOAF-1")
	root, err := project.ResolveRoot(repo)
	if err != nil {
		t.Fatal(err)
	}
	pre, err := state.RecordRelease(t.Context(), root, state.PathResolver{StateHome: stateHome}, state.RecordReleaseOptions{
		Version:      "1.1.0-alpha.1",
		Tag:          "v1.1.0-alpha.1",
		TaggedCommit: gitOutputReleaseTest(t, repo, "rev-parse", "HEAD"),
	})
	if err != nil {
		t.Fatalf("seed prerelease: %v", err)
	}
	writeFile(t, filepath.Join(repo, "stable.txt"), "stable\n")
	gitCLI(t, repo, "add", "stable.txt")
	gitCLI(t, repo, "commit", "-m", "feat: stabilize LOAF-1")
	gitCLI(t, repo, "tag", "v1.1.0")

	out, err := runReleaseTrack(t, repo, stateHome, "cut", "--no-tag", "--no-gh", "--includes", "v1.1.0-alpha.1", "--base", "v1.0.0")
	if err != nil {
		t.Fatalf("release cut --includes error = %v\n%s", err, out)
	}
	stable, err := state.GetRelease(t.Context(), root, state.PathResolver{StateHome: stateHome}, "1.1.0")
	if err != nil {
		t.Fatalf("GetRelease(stable): %v", err)
	}
	foundReleaseRef := false
	foundIssue := false
	for _, member := range stable.Members {
		if member.Kind == state.ReleaseMemberKindRelease && member.MemberID == pre.ID {
			foundReleaseRef = true
		}
		if member.Kind == state.ReleaseMemberKindIssue {
			foundIssue = true
		}
	}
	if !foundReleaseRef || !foundIssue {
		t.Fatalf("stable members = %#v, want issue + prerelease reference", stable.Members)
	}
	if len(pre.Members) != 0 {
		t.Fatalf("includes must not union prerelease members; prerelease members = %#v", pre.Members)
	}
}

func TestReleaseSuggestDoesNotMaterializeIdentity(t *testing.T) {
	repo, stateHome := releaseTrackFixture(t)
	root, err := project.ResolveRoot(repo)
	if err != nil {
		t.Fatalf("ResolveRoot: %v", err)
	}
	resolver := state.PathResolver{StateHome: stateHome}
	status, err := state.Inspect(root, resolver)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if _, ok, err := state.LookupIssueIdentity(t.Context(), root, resolver); err != nil || ok {
		t.Fatalf("fixture identity present: ok=%v err=%v", ok, err)
	}
	before, err := os.ReadFile(status.DatabasePath)
	if err != nil {
		t.Fatalf("ReadFile db: %v", err)
	}

	out, err := runReleaseTrack(t, repo, stateHome, "suggest")
	if err != nil {
		t.Fatalf("release suggest error = %v\n%s", err, out)
	}
	after, err := os.ReadFile(status.DatabasePath)
	if err != nil {
		t.Fatalf("ReadFile db after suggest: %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("suggest mutated the database")
	}
	if _, ok, err := state.LookupIssueIdentity(t.Context(), root, resolver); err != nil || ok {
		t.Fatalf("suggest materialized identity: ok=%v err=%v", ok, err)
	}
}

func TestReleaseCutNoTagRequiresExistingTag(t *testing.T) {
	repo, stateHome := releaseTrackFixture(t)
	if _, err := runIssue(t, repo, stateHome, "new", "Ship auth"); err != nil {
		t.Fatalf("issue new error = %v", err)
	}
	writeFile(t, filepath.Join(repo, "auth.txt"), "auth\n")
	gitCLI(t, repo, "add", "auth.txt")
	gitCLI(t, repo, "commit", "-m", "feat: add auth LOAF-1")

	out, err := runReleaseTrack(t, repo, stateHome, "cut", "--no-tag", "--no-gh")
	if err == nil || !strings.Contains(err.Error(), "v1.1.0") || !strings.Contains(err.Error(), "--no-tag") {
		t.Fatalf("error = %v, want missing tag v1.1.0\n%s", err, out)
	}
	root, err := project.ResolveRoot(repo)
	if err != nil {
		t.Fatal(err)
	}
	listed, err := state.ListReleases(t.Context(), root, state.PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("ListReleases: %v", err)
	}
	if len(listed) != 0 {
		t.Fatalf("refused cut wrote %d release rows", len(listed))
	}
}

func TestReleaseCutGitHubFailureAfterRecordIsWarning(t *testing.T) {
	repo, stateHome := releaseTrackFixture(t)
	if _, err := runIssue(t, repo, stateHome, "new", "Ship auth"); err != nil {
		t.Fatalf("issue new error = %v", err)
	}
	writeFile(t, filepath.Join(repo, "auth.txt"), "auth\n")
	gitCLI(t, repo, "add", "auth.txt")
	gitCLI(t, repo, "commit", "-m", "feat: add auth LOAF-1")
	gitCLI(t, repo, "tag", "v1.1.0")
	tagCommit := gitOutputReleaseTest(t, repo, "rev-parse", "v1.1.0^{commit}")
	prependFailingGh(t)

	stdout, stderr, err := runReleaseTrackIO(t, repo, stateHome, "cut", "--no-tag", "--base", "v1.0.0")
	if err != nil {
		t.Fatalf("cut should warn, not fail, after recording: %v\n%s\n%s", err, stdout, stderr)
	}
	if !strings.Contains(stderr, "warning:") || !strings.Contains(stderr, "retry: "+state.PosixSingleQuote("gh")) {
		t.Fatalf("stderr = %q, want warning with quoted retry command", stderr)
	}
	if !strings.Contains(stderr, state.PosixSingleQuote("v1.1.0")) {
		t.Fatalf("stderr = %q, want POSIX-quoted tag in retry", stderr)
	}
	root, err := project.ResolveRoot(repo)
	if err != nil {
		t.Fatal(err)
	}
	recorded, err := state.GetRelease(t.Context(), root, state.PathResolver{StateHome: stateHome}, "v1.1.0")
	if err != nil {
		t.Fatalf("GetRelease: %v", err)
	}
	if recorded.TaggedCommit != tagCommit {
		t.Fatalf("tagged_commit = %q, want %q", recorded.TaggedCommit, tagCommit)
	}
}

func TestReleaseSuggestAttributesSquashBodyAlias(t *testing.T) {
	repo, stateHome := releaseTrackFixture(t)
	if _, err := runIssue(t, repo, stateHome, "new", "Auth"); err != nil {
		t.Fatalf("issue new error = %v", err)
	}
	gitCLI(t, repo, "commit", "--allow-empty", "-m", "feat: add auth (#42)", "-m", "* feat: implement login on loaf-1")

	out, err := runReleaseTrack(t, repo, stateHome, "suggest")
	if err != nil {
		t.Fatalf("release suggest error = %v\n%s", err, out)
	}
	if !strings.Contains(out, "LOAF-1 — Auth") {
		t.Fatalf("squash body alias not attributed:\n%s", out)
	}
}

func TestReleaseSuggestIgnoresPreBaselineParentDone(t *testing.T) {
	repo, stateHome := releaseTrackFixture(t)
	if _, err := runIssue(t, repo, stateHome, "new", "Parent"); err != nil {
		t.Fatalf("issue new parent error = %v", err)
	}
	if _, err := runIssue(t, repo, stateHome, "new", "Child A", "--parent", "LOAF-1"); err != nil {
		t.Fatalf("issue new child A error = %v", err)
	}
	if _, err := runIssue(t, repo, stateHome, "new", "Child B", "--parent", "LOAF-1"); err != nil {
		t.Fatalf("issue new child B error = %v", err)
	}
	for _, alias := range []string{"LOAF-1", "LOAF-2", "LOAF-3"} {
		if _, err := runIssue(t, repo, stateHome, "status", alias, "done"); err != nil {
			t.Fatalf("status %s error = %v", alias, err)
		}
	}
	writeFile(t, filepath.Join(repo, "baseline.txt"), "after parent done\n")
	gitCLI(t, repo, "add", "baseline.txt")
	gitCommitDated(t, repo, "2099-01-01T00:00:00+00:00", "chore: baseline after parent done")
	gitCLI(t, repo, "tag", "v-after-parent")

	writeFile(t, filepath.Join(repo, "a.txt"), "a\n")
	gitCLI(t, repo, "add", "a.txt")
	gitCLI(t, repo, "commit", "-m", "chore: land LOAF-2")
	writeFile(t, filepath.Join(repo, "b.txt"), "b\n")
	gitCLI(t, repo, "add", "b.txt")
	gitCLI(t, repo, "commit", "-m", "chore: land LOAF-3")

	out, err := runReleaseTrack(t, repo, stateHome, "suggest", "--base", "v-after-parent")
	if err != nil {
		t.Fatalf("release suggest error = %v\n%s", err, out)
	}
	if strings.Contains(out, "closed multi-child parent") {
		t.Fatalf("pre-baseline parent done should not derive minor:\n%s", out)
	}
	if !strings.Contains(out, "Suggested bump: patch") {
		t.Fatalf("want patch bump from chore children only:\n%s", out)
	}
}

func TestReleaseTrackJournalPrefixMustResolveUniquely(t *testing.T) {
	commits := []releaseTrackCommit{
		{Hash: "abc1234", Subject: "one"},
		{Hash: "abc9999", Subject: "two"},
	}
	journal := []state.JournalEntryRecord{
		{Scope: "abc", Message: "LOAF-1"},
	}
	byHash := resolveReleaseTrackJournalAliases(commits, "LOAF", journal)
	if len(byHash) != 0 {
		t.Fatalf("ambiguous prefix attributed %#v", byHash)
	}

	journal = append(journal, state.JournalEntryRecord{Scope: "abc1234", Message: "LOAF-2"})
	byHash = resolveReleaseTrackJournalAliases(commits, "LOAF", journal)
	if got := byHash["abc1234"]; len(got) != 1 || got[0] != "LOAF-2" {
		t.Fatalf("unique prefix = %#v, want [LOAF-2]", byHash["abc1234"])
	}
	if _, ok := byHash["abc9999"]; ok {
		t.Fatalf("ambiguous prefix should not attribute abc9999: %#v", byHash)
	}
}

func TestReleaseSuggestKeepsEmptySubjectCommits(t *testing.T) {
	repo, stateHome := releaseTrackFixture(t)
	gitCLI(t, repo, "commit", "--allow-empty", "--allow-empty-message", "--file=/dev/null")
	hash := gitOutputReleaseTest(t, repo, "log", "-1", "--pretty=%h")

	out, err := runReleaseTrack(t, repo, stateHome, "suggest")
	if err != nil {
		t.Fatalf("release suggest error = %v\n%s", err, out)
	}
	if !strings.Contains(out, hash) {
		t.Fatalf("empty-subject commit %s was dropped:\n%s", hash, out)
	}
}

func TestReleaseCutRefusesDisagreeingVersionFiles(t *testing.T) {
	repo, stateHome := releaseTrackFixture(t)
	writeFile(t, filepath.Join(repo, "pyproject.toml"), "[project]\nname = \"fixture\"\nversion = \"2.0.0\"\n")
	gitCLI(t, repo, "add", "pyproject.toml")
	gitCLI(t, repo, "commit", "-m", "chore: add disagreeing version file")

	out, err := runReleaseTrack(t, repo, stateHome, "cut", "--no-tag", "--no-gh")
	if err == nil {
		t.Fatalf("cut succeeded, want version-file disagreement\n%s", out)
	}
	message := err.Error()
	if !strings.Contains(message, "package.json") || !strings.Contains(message, "1.0.0") {
		t.Fatalf("error missing package.json=1.0.0: %v", err)
	}
	if !strings.Contains(message, "pyproject.toml") || !strings.Contains(message, "2.0.0") {
		t.Fatalf("error missing pyproject.toml=2.0.0: %v", err)
	}
}

func TestReleaseTrackRetryCommandQuotesShellMetacharacters(t *testing.T) {
	notes := "see $(reboot) and `id` and it's\nreleased"
	got := formatReleaseTrackRetryCommand(state.Release{Tag: "v1.1.0", Version: "1.1.0"}, notes)
	want := strings.Join([]string{
		state.PosixSingleQuote("gh"),
		state.PosixSingleQuote("release"),
		state.PosixSingleQuote("create"),
		state.PosixSingleQuote("v1.1.0"),
		state.PosixSingleQuote("--draft"),
		state.PosixSingleQuote("--title"),
		state.PosixSingleQuote("v1.1.0"),
		state.PosixSingleQuote("--notes"),
		state.PosixSingleQuote(notes),
	}, " ")
	if got != want {
		t.Fatalf("retry = %q, want %q", got, want)
	}
	if !strings.Contains(got, state.PosixSingleQuote(notes)) {
		t.Fatalf("notes were not POSIX-quoted: %q", got)
	}
}

func TestWarnReleaseTrackGitHubFailureQuotesRetryNotes(t *testing.T) {
	var stderr bytes.Buffer
	notes := "see $(reboot) and `id` and it's\nreleased"
	err := (Runner{Stderr: &stderr}).warnReleaseTrackGitHubFailure(&bytes.Buffer{}, state.Release{
		Tag:          "v1.1.0",
		Version:      "1.1.0",
		TaggedCommit: "abc1234",
	}, notes, fmt.Errorf("simulated"))
	if err != nil {
		t.Fatalf("warnReleaseTrackGitHubFailure() error = %v", err)
	}
	got := stderr.String()
	if !strings.Contains(got, "warning:") || !strings.Contains(got, "retry:") {
		t.Fatalf("stderr = %q, want warning with retry", got)
	}
	if !strings.Contains(got, state.PosixSingleQuote(notes)) {
		t.Fatalf("retry notes not POSIX-quoted:\n%s", got)
	}
	if strings.Contains(got, strconv.Quote(notes)) {
		t.Fatalf("retry used Go Quote:\n%s", got)
	}
}

func TestReleaseCutNoTagIgnoresBranchNamedLikeTag(t *testing.T) {
	repo, stateHome := releaseTrackFixture(t)
	if _, err := runIssue(t, repo, stateHome, "new", "Ship"); err != nil {
		t.Fatalf("issue new error = %v", err)
	}
	writeFile(t, filepath.Join(repo, "ship.txt"), "ship\n")
	gitCLI(t, repo, "add", "ship.txt")
	gitCLI(t, repo, "commit", "-m", "fix: ship LOAF-1")
	gitCLI(t, repo, "branch", "v1.0.1")

	if got := gitOutputReleaseTest(t, repo, "rev-parse", "v1.0.1^{commit}"); got == "" {
		t.Fatal("branch v1.0.1 must resolve via unqualified rev-parse")
	}
	if out, err := exec.Command("git", "-C", repo, "rev-parse", "refs/tags/v1.0.1^{commit}").CombinedOutput(); err == nil {
		t.Fatalf("tag refs/tags/v1.0.1 unexpectedly exists:\n%s", out)
	}

	out, err := runReleaseTrack(t, repo, stateHome, "cut", "--no-tag", "--no-gh")
	if err == nil || !strings.Contains(err.Error(), "v1.0.1") || !strings.Contains(err.Error(), "--no-tag") {
		t.Fatalf("error = %v, want missing tag v1.0.1\n%s", err, out)
	}
}

func TestReleaseSuggestDoesNotAttributeURLOrCodeSpanAliases(t *testing.T) {
	repo, stateHome := releaseTrackFixture(t)
	if _, err := runIssue(t, repo, stateHome, "new", "Tracked"); err != nil {
		t.Fatalf("issue new error = %v", err)
	}
	writeFile(t, filepath.Join(repo, "docs.txt"), "docs\n")
	gitCLI(t, repo, "add", "docs.txt")
	gitCLI(t, repo, "commit", "-m", "docs: see https://tracker/LOAF-1 and `LOAF-1`")

	out, err := runReleaseTrack(t, repo, stateHome, "suggest")
	if err != nil {
		t.Fatalf("release suggest error = %v\n%s", err, out)
	}
	if strings.Contains(out, "LOAF-1 — Tracked") {
		t.Fatalf("URL/code-span alias was attributed:\n%s", out)
	}
	if !strings.Contains(out, "docs: see https://tracker/LOAF-1") {
		t.Fatalf("commit missing from unattributed:\n%s", out)
	}
}

func TestReleaseCutDryRunNoTagRequiresExistingTag(t *testing.T) {
	repo, stateHome := releaseTrackFixture(t)
	if _, err := runIssue(t, repo, stateHome, "new", "Ship auth"); err != nil {
		t.Fatalf("issue new error = %v", err)
	}
	writeFile(t, filepath.Join(repo, "auth.txt"), "auth\n")
	gitCLI(t, repo, "add", "auth.txt")
	gitCLI(t, repo, "commit", "-m", "feat: add auth LOAF-1")

	out, err := runReleaseTrack(t, repo, stateHome, "cut", "--dry-run", "--no-tag", "--no-gh")
	if err == nil || !strings.Contains(err.Error(), "v1.1.0") || !strings.Contains(err.Error(), "--no-tag") {
		t.Fatalf("error = %v, want missing tag v1.1.0\n%s", err, out)
	}
	root, err := project.ResolveRoot(repo)
	if err != nil {
		t.Fatal(err)
	}
	listed, err := state.ListReleases(t.Context(), root, state.PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("ListReleases: %v", err)
	}
	if len(listed) != 0 {
		t.Fatalf("refused dry-run wrote %d release rows", len(listed))
	}
}

func TestReleaseCutMissingGhWarnsWithRetry(t *testing.T) {
	repo, stateHome := releaseTrackFixture(t)
	if _, err := runIssue(t, repo, stateHome, "new", "Ship auth"); err != nil {
		t.Fatalf("issue new error = %v", err)
	}
	writeFile(t, filepath.Join(repo, "auth.txt"), "auth\n")
	gitCLI(t, repo, "add", "auth.txt")
	gitCLI(t, repo, "commit", "-m", "feat: add auth LOAF-1")
	gitCLI(t, repo, "tag", "v1.1.0")
	tagCommit := gitOutputReleaseTest(t, repo, "rev-parse", "refs/tags/v1.1.0^{commit}")
	t.Setenv("PATH", pathWithoutGh(t))

	stdout, stderr, err := runReleaseTrackIO(t, repo, stateHome, "cut", "--no-tag", "--base", "v1.0.0")
	if err != nil {
		t.Fatalf("cut should warn, not fail, when gh is missing: %v\n%s\n%s", err, stdout, stderr)
	}
	if !strings.Contains(stderr, "warning:") || !strings.Contains(stderr, "retry:") {
		t.Fatalf("stderr = %q, want warning with retry command", stderr)
	}
	if !strings.Contains(stderr, state.PosixSingleQuote("gh")+" "+state.PosixSingleQuote("release")+" "+state.PosixSingleQuote("create")+" "+state.PosixSingleQuote("v1.1.0")) {
		t.Fatalf("stderr = %q, want POSIX-quoted retry command", stderr)
	}
	if !strings.Contains(stderr, "gh not found") {
		t.Fatalf("stderr = %q, want gh-not-found cause", stderr)
	}
	root, err := project.ResolveRoot(repo)
	if err != nil {
		t.Fatal(err)
	}
	recorded, err := state.GetRelease(t.Context(), root, state.PathResolver{StateHome: stateHome}, "v1.1.0")
	if err != nil {
		t.Fatalf("GetRelease: %v", err)
	}
	if recorded.TaggedCommit != tagCommit {
		t.Fatalf("tagged_commit = %q, want %q", recorded.TaggedCommit, tagCommit)
	}
}

func pathWithoutGh(t *testing.T) string {
	t.Helper()
	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.Symlink(gitPath, filepath.Join(dir, "git")); err != nil {
		t.Fatalf("symlink git: %v", err)
	}
	return dir
}

func prependFailingGh(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	script := filepath.Join(dir, "gh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho 'simulated gh failure' >&2\nexit 1\n"), 0o755); err != nil {
		t.Fatalf("write failing gh: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func gitCommitDated(t *testing.T, repo, date, message string) {
	t.Helper()
	cmd := exec.Command("git", "commit", "-m", message)
	cmd.Dir = repo
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_DATE="+date, "GIT_COMMITTER_DATE="+date)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git commit: %v\n%s", err, out)
	}
}

func TestReleaseSuggestUsesTagBaselineNotWorkingTreeVersion(t *testing.T) {
	repo, stateHome := releaseTrackFixture(t)
	if _, err := runIssue(t, repo, stateHome, "new", "Ship"); err != nil {
		t.Fatalf("issue new error = %v", err)
	}
	writeFile(t, filepath.Join(repo, "ship.txt"), "ship\n")
	gitCLI(t, repo, "add", "ship.txt")
	gitCLI(t, repo, "commit", "-m", "feat: ship LOAF-1")

	out, err := runReleaseTrack(t, repo, stateHome, "suggest")
	if err != nil {
		t.Fatalf("release suggest error = %v\n%s", err, out)
	}
	if !strings.Contains(out, "1.1.0") {
		t.Fatalf("suggest = %q, want bump from tag baseline 1.0.0 → 1.1.0", out)
	}
}

func TestReleaseSuggestRefusesDirtyVersionFiles(t *testing.T) {
	repo, stateHome := releaseTrackFixture(t)
	writeFile(t, filepath.Join(repo, "package.json"), "{\n  \"name\": \"release-fixture\",\n  \"version\": \"9.9.9\",\n  \"scripts\": {\n    \"build\": \"echo build\"\n  }\n}\n")

	out, err := runReleaseTrack(t, repo, stateHome, "suggest")
	if err == nil {
		t.Fatalf("suggest succeeded, want dirty version-file refusal\n%s", out)
	}
	if !strings.Contains(err.Error(), "package.json") || !strings.Contains(err.Error(), "uncommitted") {
		t.Fatalf("error = %v, want dirty package.json refusal", err)
	}
}

func TestReleaseCutResumesRecordWhenTagExistsWithoutRow(t *testing.T) {
	repo, stateHome := releaseTrackFixture(t)
	if _, err := runIssue(t, repo, stateHome, "new", "Ship auth"); err != nil {
		t.Fatalf("issue new error = %v", err)
	}
	writeFile(t, filepath.Join(repo, "auth.txt"), "auth\n")
	gitCLI(t, repo, "add", "auth.txt")
	gitCLI(t, repo, "commit", "-m", "feat: add auth LOAF-1")

	orig := recordReleaseFn
	t.Cleanup(func() { recordReleaseFn = orig })
	recordReleaseFn = func(ctx context.Context, root project.Root, resolver state.PathResolver, options state.RecordReleaseOptions) (state.Release, error) {
		return state.Release{}, fmt.Errorf("injected record failure")
	}
	out, err := runReleaseTrack(t, repo, stateHome, "cut", "--no-gh")
	recordReleaseFn = orig
	if err == nil || !strings.Contains(err.Error(), "injected record failure") {
		t.Fatalf("cut error = %v\n%s, want injected record failure", err, out)
	}
	if tags := gitOutputReleaseTest(t, repo, "tag", "--list"); !strings.Contains(tags, "v1.1.0") {
		t.Fatalf("tags after failed record = %q, want v1.1.0", tags)
	}

	retry, err := runReleaseTrack(t, repo, stateHome, "cut", "--no-gh")
	if err != nil {
		t.Fatalf("cut retry error = %v\n%s", err, retry)
	}
	if !strings.Contains(retry, "Recorded release v1.1.0") && !strings.Contains(retry, "already recorded") {
		t.Fatalf("retry output = %q, want record completed", retry)
	}
	root, err := project.ResolveRoot(repo)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.GetRelease(t.Context(), root, state.PathResolver{StateHome: stateHome}, "v1.1.0"); err != nil {
		t.Fatalf("GetRelease after retry: %v", err)
	}
}

func TestReleaseLegacyFlagPathStillDispatches(t *testing.T) {
	var stdout bytes.Buffer
	err := Runner{Stdout: &stdout, WorkingDir: t.TempDir()}.Run([]string{"release", "--help"})
	if err != nil {
		t.Fatalf("release --help error = %v", err)
	}
	output := stdout.String()
	if !strings.Contains(output, "Usage: loaf release <subcommand>") || !strings.Contains(output, "suggest") || !strings.Contains(output, "cut") {
		t.Fatalf("release help missing suggest/cut:\n%s", output)
	}
}
