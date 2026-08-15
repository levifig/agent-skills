package cli

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunnerReleaseHelpIsNative(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	var stdout bytes.Buffer

	err := Runner{
		Stdout:     &stdout,
		WorkingDir: workingDir,
	}.Run([]string{"release", "--help"})
	if err != nil {
		t.Fatalf("release --help error = %v", err)
	}
	output := stdout.String()
	if !strings.Contains(output, "Usage: loaf release [options]") || !strings.Contains(output, "--pre-merge") || !strings.Contains(output, "--post-merge") {
		t.Fatalf("output = %q, want native release help", output)
	}
}

func TestReleaseAllowsPrereleaseLineageBypassRequiresPrereleaseVersion(t *testing.T) {
	cases := []struct {
		name    string
		version string
		options releaseOptions
		want    bool
	}{
		{name: "explicit prerelease", version: "1.0.0-alpha.5", options: releaseOptions{bump: "prerelease"}, want: true},
		{name: "post merge prerelease", version: "1.0.0-alpha.5", options: releaseOptions{postMerge: true}, want: true},
		{name: "explicit prerelease stable version", version: "1.0.0", options: releaseOptions{bump: "prerelease"}},
		{name: "post merge stable version", version: "1.0.0", options: releaseOptions{postMerge: true}},
		{name: "post merge with bump", version: "1.0.0-alpha.5", options: releaseOptions{postMerge: true, bump: "release"}},
		{name: "bare prerelease version", version: "1.0.0-alpha.5", options: releaseOptions{}},
		{name: "release bump prerelease version", version: "1.0.0-alpha.5", options: releaseOptions{bump: "release"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := seedReleaseLineageFreezeRepo(t, tc.version)
			if got := releaseAllowsPrereleaseLineageBypass(repo, tc.options); got != tc.want {
				t.Fatalf("releaseAllowsPrereleaseLineageBypass(%q, %#v) = %v, want %v", tc.version, tc.options, got, tc.want)
			}
		})
	}
}

func TestReleaseAllowsPrereleaseLineageBypassRequiresConsistentPrereleaseVersionFiles(t *testing.T) {
	cases := []struct {
		name           string
		packageVersion string
		backendVersion string
		want           bool
	}{
		{name: "matching prereleases", packageVersion: "1.0.0-alpha.5", backendVersion: "1.0.0-alpha.5", want: true},
		{name: "prerelease and stable", packageVersion: "1.0.0-alpha.5", backendVersion: "1.0.0", want: false},
		{name: "mismatched prereleases", packageVersion: "1.0.0-alpha.5", backendVersion: "1.0.0-alpha.6", want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := seedReleaseLineageFreezeRepo(t, tc.packageVersion)
			mkdirAll(t, filepath.Join(repo, ".agents"))
			mkdirAll(t, filepath.Join(repo, "backend"))
			writeFile(t, filepath.Join(repo, ".agents", "loaf.json"), "{\n  \"release\": {\n    \"versionFiles\": [\"package.json\", \"backend/pyproject.toml\"]\n  }\n}\n")
			writeFile(t, filepath.Join(repo, "backend", "pyproject.toml"), fmt.Sprintf("[project]\nname = \"backend\"\nversion = %q\n", tc.backendVersion))
			options := releaseOptions{bump: "prerelease"}
			if got := releaseAllowsPrereleaseLineageBypass(repo, options); got != tc.want {
				t.Fatalf("releaseAllowsPrereleaseLineageBypass() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestRunnerReleasePostMergeFailsClosedOutsideGit(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	var stdout bytes.Buffer

	err := Runner{
		Stdout:     &stdout,
		WorkingDir: workingDir,
	}.Run([]string{"release", "--post-merge"})
	if err == nil {
		t.Fatalf("release --post-merge error = nil, want fail-closed outside git")
	}
	msg := err.Error()
	if !strings.Contains(msg, "cannot compute candidate version") && !strings.Contains(msg, "release blocked") {
		t.Fatalf("release --post-merge error = %v, want fail-closed release blocked", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want no release actions before ancestry inspection", stdout.String())
	}
}

func TestReleasePostMergeGuardrailsHappyPath(t *testing.T) {
	repo := seedReleasePostMergeFiles(t, "1.2.3")
	runner, calls := scriptedReleasePostMergeRunner(releasePostMergeHappyResponses("1.2.3"))
	snap := mustResolveReleaseSnapshot(t, repo, releaseOptions{postMerge: true})

	result := checkReleasePostMergeGuardrails(repo, snap, runner)
	if !result.ok {
		t.Fatalf("guardrail %d failed: %s", result.guardrail, result.message)
	}
	if result.version != "1.2.3" || result.base != "main" || result.featureBranch != "feat/cool-thing" {
		t.Fatalf("result = %#v, want version/base/feature branch", result)
	}
	if !strings.Contains(result.changelogBody, "New feature") {
		t.Fatalf("changelog body = %q, want release notes", result.changelogBody)
	}
	for _, call := range calls() {
		if call.name == "gh" && len(call.args) >= 5 && call.args[0] == "pr" && call.args[1] == "view" && strings.Contains(strings.Join(call.args, " "), "baseRefName,state") {
			t.Fatalf("post-merge base detection called open-PR lookup: %#v", call)
		}
	}
}

func TestReleasePostMergeGuardrailsAbortOnLocalTagCollision(t *testing.T) {
	repo := seedReleasePostMergeFiles(t, "1.2.3")
	responses := releasePostMergeHappyResponses("1.2.3")
	responses["git tag --list v1.2.3"] = releasePostMergeOK("v1.2.3")
	runner, _ := scriptedReleasePostMergeRunner(responses)
	snap := mustResolveReleaseSnapshot(t, repo, releaseOptions{postMerge: true})

	result := checkReleasePostMergeGuardrails(repo, snap, runner)
	if result.ok {
		t.Fatalf("result ok = true, want local tag collision failure")
	}
	if result.guardrail != 7 || result.message != "tag v1.2.3 already exists locally — run `git tag -d v1.2.3` and rerun" {
		t.Fatalf("result = %#v, want guardrail 7 local tag diagnostic", result)
	}
}

func TestReleasePostMergeGuardrailsAbortOnGitHubAccountMismatch(t *testing.T) {
	repo := seedReleasePostMergeFiles(t, "1.2.3")
	mkdirAll(t, filepath.Join(repo, ".agents"))
	writeFile(t, filepath.Join(repo, ".agents", "loaf.json"), `{"integrations":{"github":{"account":"levifig"}}}`+"\n")
	responses := releasePostMergeHappyResponses("1.2.3")
	responses["gh auth status --active --hostname github.com --json hosts"] = releasePostMergeOK(githubAuthStatusJSON("work-account"))
	runner, calls := scriptedReleasePostMergeRunner(responses)
	snap := mustResolveReleaseSnapshot(t, repo, releaseOptions{postMerge: true})

	result := checkReleasePostMergeGuardrails(repo, snap, runner)
	if result.ok {
		t.Fatalf("result ok = true, want GitHub account guardrail failure")
	}
	if result.guardrail != 2 || !strings.Contains(result.message, `project requires GitHub account "levifig"`) || !strings.Contains(result.message, `active gh account is "work-account"`) {
		t.Fatalf("result = %#v, want guardrail 2 account diagnostic", result)
	}
	for _, call := range releasePostMergeCallKeys(calls()) {
		if strings.HasPrefix(call, "gh repo view") || strings.HasPrefix(call, "gh release view") {
			t.Fatalf("calls = %#v, want account check to abort before GitHub repo/release lookup", releasePostMergeCallKeys(calls()))
		}
	}
}

// A self-carrying release — version flip already landed as Change content —
// leaves the release commit nothing to write but the changelog. Guardrail 5
// accepts that shape only under guardrail 4's proof that the version files
// already report the candidate.
func TestReleasePostMergeDiffFilesRelaxesVersionDemandOnlyUnderProof(t *testing.T) {
	cases := []struct {
		name                    string
		diff                    string
		versionFilesAtCandidate bool
		want                    string
	}{
		{
			name:                    "changelog-only under proof",
			diff:                    "CHANGELOG.md",
			versionFilesAtCandidate: true,
		},
		{
			name: "changelog-only without proof",
			diff: "CHANGELOG.md",
			want: "release commit is missing a version-file diff (expected one of: package.json)",
		},
		{
			name:                    "version file without changelog under proof",
			diff:                    "package.json",
			versionFilesAtCandidate: true,
			want:                    "release commit is missing a CHANGELOG.md diff — verify the changelog was updated",
		},
		{
			name:                    "neither under proof",
			diff:                    "README.md",
			versionFilesAtCandidate: true,
			want:                    "release commit is missing both CHANGELOG.md and any version file diffs — this does not look like a release commit",
		},
		{
			name:                    "conventional release commit under proof",
			diff:                    "CHANGELOG.md\npackage.json",
			versionFilesAtCandidate: true,
		},
		{
			name: "conventional release commit without proof",
			diff: "CHANGELOG.md\npackage.json",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := seedReleasePostMergeFiles(t, "1.2.3")
			versionFiles := mustResolveReleaseSnapshot(t, repo, releaseOptions{postMerge: true}).VersionFiles
			responses := releasePostMergeHappyResponses("1.2.3")
			responses["git diff HEAD^ HEAD --name-only"] = releasePostMergeOK(tc.diff)
			runner, _ := scriptedReleasePostMergeRunner(responses)

			got := checkReleasePostMergeDiffFiles(repo, runner, versionFiles, "HEAD^", "HEAD", tc.versionFilesAtCandidate)
			if got != tc.want {
				t.Fatalf("abort = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestReleasePostMergeGuardrailsAcceptSelfCarryingReleaseCommit(t *testing.T) {
	repo := seedReleasePostMergeFiles(t, "1.2.3")
	responses := releasePostMergeHappyResponses("1.2.3")
	responses["git diff HEAD^ HEAD --name-only"] = releasePostMergeOK("CHANGELOG.md")
	runner, _ := scriptedReleasePostMergeRunner(responses)
	snap := mustResolveReleaseSnapshot(t, repo, releaseOptions{postMerge: true})

	result := checkReleasePostMergeGuardrails(repo, snap, runner)
	if !result.ok {
		t.Fatalf("guardrail %d failed: %s", result.guardrail, result.message)
	}
	if result.version != "1.2.3" {
		t.Fatalf("result = %#v, want candidate version", result)
	}
}

// The relaxation cannot be reached without the proof: a candidate that diverges
// from the version files aborts at guardrail 4, before guardrail 5 reads a diff.
func TestReleasePostMergeGuardrailsBlockChangelogOnlyWhenCandidateDiverges(t *testing.T) {
	repo := seedReleasePostMergeFiles(t, "1.2.3")
	snap := mustResolveReleaseSnapshot(t, repo, releaseOptions{postMerge: true})
	snap.Candidate = "1.3.0"
	responses := releasePostMergeHappyResponses("1.3.0")
	responses["git diff HEAD^ HEAD --name-only"] = releasePostMergeOK("CHANGELOG.md")
	runner, calls := scriptedReleasePostMergeRunner(responses)

	result := checkReleasePostMergeGuardrails(repo, snap, runner)
	if result.ok || result.guardrail != 4 {
		t.Fatalf("result = %#v, want guardrail 4 abort", result)
	}
	if !strings.Contains(result.message, "does not match version-file version") {
		t.Fatalf("message = %q, want tag-equals-files diagnostic", result.message)
	}
	for _, call := range releasePostMergeCallKeys(calls()) {
		if strings.HasPrefix(call, "git diff HEAD^ HEAD") {
			t.Fatalf("calls = %#v, want abort before the diff-shape read", releasePostMergeCallKeys(calls()))
		}
	}
}

func TestReleasePostMergeActionsHappyPath(t *testing.T) {
	repo := seedReleasePostMergeFiles(t, "1.2.3")
	runner, calls := scriptedReleasePostMergeRunner(releasePostMergeHappyResponses("1.2.3"))
	var stdout, stderr bytes.Buffer

	result, err := executeReleasePostMergeActions(repo, releasePostMergeResult{
		ok:            true,
		version:       "1.2.3",
		base:          "main",
		featureBranch: "feat/cool-thing",
		changelogBody: "- A nifty change",
	}, runner, &stdout, &stderr)
	if err != nil {
		t.Fatalf("executeReleasePostMergeActions error = %v", err)
	}
	if !result.tagged || !result.pushed || !result.released || !result.pulled || result.deletedLocal == nil || !*result.deletedLocal || result.deletedRemote == nil || !*result.deletedRemote {
		t.Fatalf("result = %#v, want all action flags true", result)
	}
	wantCalls := []string{
		"git tag -s v1.2.3 -m Release 1.2.3",
		"git push origin v1.2.3",
		"gh release create v1.2.3 --title v1.2.3 --notes - A nifty change",
		"git pull --rebase origin main",
		"git branch -d feat/cool-thing",
		"git push origin --delete feat/cool-thing",
	}
	got := releasePostMergeCallKeys(calls())
	for _, want := range wantCalls {
		if !containsReleasePostMergeCall(got, want) {
			t.Fatalf("calls = %#v, want %q", got, want)
		}
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want no warnings", stderr.String())
	}
}

func TestReleasePostMergeActionWarnsAndContinuesAfterPullFailure(t *testing.T) {
	repo := seedReleasePostMergeFiles(t, "1.2.3")
	responses := releasePostMergeHappyResponses("1.2.3")
	responses["git pull --rebase origin main"] = releasePostMergeExit(1)
	runner, _ := scriptedReleasePostMergeRunner(responses)
	var stdout, stderr bytes.Buffer

	result, err := executeReleasePostMergeActions(repo, releasePostMergeResult{
		ok:            true,
		version:       "1.2.3",
		base:          "main",
		changelogBody: "- A nifty change",
	}, runner, &stdout, &stderr)
	if err != nil {
		t.Fatalf("executeReleasePostMergeActions error = %v", err)
	}
	if !result.tagged || !result.released || result.pulled {
		t.Fatalf("result = %#v, want tag/release true and pulled false", result)
	}
	if !strings.Contains(stderr.String(), "Failed to pull origin/main") {
		t.Fatalf("stderr = %q, want pull warning", stderr.String())
	}
}

func TestRunReleasePostMergeFinalizesNatively(t *testing.T) {
	repo := seedReleasePostMergeFiles(t, "1.2.3")
	responses := releasePostMergeHappyResponses("1.2.3")
	responses["gh release create v1.2.3 --title v1.2.3 --notes ### Added\n- New feature (abc1234)"] = releasePostMergeOK("")
	runner, _ := scriptedReleasePostMergeRunner(responses)
	var stdout, stderr bytes.Buffer

	snap := mustResolveReleaseSnapshot(t, repo, releaseOptions{postMerge: true})
	if err := runReleasePostMergeWithRunner(repo, snap, &stdout, &stderr, runner); err != nil {
		t.Fatalf("runReleasePostMergeWithRunner error = %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}
	for _, want := range []string{"Verifying post-merge state", "All 9 guardrails passed", "Executing:", "Created tag v1.2.3", "Release v1.2.3 finalized"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout = %q, want %q", stdout.String(), want)
		}
	}
}

func TestRunnerReleaseInteractiveConfirmationExecutesNatively(t *testing.T) {
	repo := seedReleaseApplyRepo(t, "feat: confirm native release execution")
	var stdout bytes.Buffer

	err := Runner{
		Stdout:     &stdout,
		Stdin:      strings.NewReader("y\n"),
		WorkingDir: repo,
	}.Run([]string{"release", "--no-tag", "--no-gh"})
	if err != nil {
		t.Fatalf("interactive release error = %v\n%s", err, stdout.String())
	}
	output := stdout.String()
	for _, want := range []string{"Proceed with release", "Executing:", "Committed release artifacts", "Release ", "v1.1.0", "complete"} {
		if !strings.Contains(output, want) {
			t.Fatalf("stdout = %q, want %q", output, want)
		}
	}
	subject := gitOutputReleaseTest(t, repo, "log", "-1", "--pretty=%s")
	if subject != "chore: release v1.1.0" {
		t.Fatalf("release commit subject = %q, want chore: release v1.1.0", subject)
	}
}

func TestRunnerReleaseMutatingModesRefuseDirtyChangeWithoutMutation(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{name: "apply", args: []string{"release", "--yes", "--no-gh"}},
		{name: "pre-merge", args: []string{"release", "--pre-merge", "--base", "HEAD~1", "--yes", "--no-gh"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := seedReleaseApplyRepo(t, "feat: keep dirty Change out of release")
			writeChangeFolder(t, repo, "20260710-dirty-lineage", executableLineageDoc("dirty-lineage", "line", "", "terminal"))
			beforeHEAD := gitOutputReleaseTest(t, repo, "rev-parse", "HEAD")
			beforePackage, err := os.ReadFile(filepath.Join(repo, "package.json"))
			if err != nil {
				t.Fatal(err)
			}
			beforeChangelog, err := os.ReadFile(filepath.Join(repo, "CHANGELOG.md"))
			if err != nil {
				t.Fatal(err)
			}
			err = (Runner{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}, WorkingDir: repo}).Run(tc.args)
			if err == nil || !strings.Contains(err.Error(), "require a clean unignored worktree") || !strings.Contains(err.Error(), "docs/changes/20260710-dirty-lineage/change.md") {
				t.Fatalf("Run(%v) error = %v, want dirty-Change refusal", tc.args, err)
			}
			afterPackage, _ := os.ReadFile(filepath.Join(repo, "package.json"))
			afterChangelog, _ := os.ReadFile(filepath.Join(repo, "CHANGELOG.md"))
			if !bytes.Equal(beforePackage, afterPackage) || !bytes.Equal(beforeChangelog, afterChangelog) {
				t.Fatalf("Run(%v) mutated version or changelog before dirty refusal", tc.args)
			}
			if head := gitOutputReleaseTest(t, repo, "rev-parse", "HEAD"); head != beforeHEAD {
				t.Fatalf("Run(%v) created release commit %s, want HEAD %s", tc.args, head, beforeHEAD)
			}
			if tag := gitOutputReleaseTest(t, repo, "tag", "--list", "v1.1.0"); tag != "" {
				t.Fatalf("Run(%v) created tag %q", tc.args, tag)
			}
			if tc.name == "apply" {
				if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: repo}).Run([]string{"release", "--dry-run"}); err != nil {
					t.Fatalf("dry-run should remain available for dirty inspection: %v", err)
				}
			}
		})
	}
}

func TestRunnerReleaseRefusesGeneratedChangeBeforeStaging(t *testing.T) {
	repo := seedReleaseApplyRepo(t, "feat: generate release artifacts")
	packageBody := strings.Join([]string{
		"{",
		`  "name": "release-fixture",`,
		`  "version": "1.0.0",`,
		`  "scripts": {`,
		`    "build": "mkdir -p docs/changes/20260710-generated && printf generated > docs/changes/20260710-generated/change.md"`,
		"  }",
		"}",
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Join(repo, "package.json"), []byte(packageBody), 0o644); err != nil {
		t.Fatal(err)
	}
	gitCLI(t, repo, "add", "package.json")
	gitCLI(t, repo, "commit", "-m", "fix: generate Change during build")
	beforeHEAD := gitOutputReleaseTest(t, repo, "rev-parse", "HEAD")
	err := (Runner{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}, WorkingDir: repo}).Run([]string{"release", "--yes", "--no-tag", "--no-gh"})
	if err == nil || !strings.Contains(err.Error(), "artifact generation modified docs/changes") || !strings.Contains(err.Error(), "docs/changes/20260710-generated/change.md") {
		t.Fatalf("release error = %v, want generated Change refusal", err)
	}
	if staged := gitOutputReleaseTest(t, repo, "diff", "--cached", "--name-only"); staged != "" {
		t.Fatalf("release staged files before refusal: %q", staged)
	}
	if head := gitOutputReleaseTest(t, repo, "rev-parse", "HEAD"); head != beforeHEAD {
		t.Fatalf("release created commit %s, want HEAD %s", head, beforeHEAD)
	}
	if tag := gitOutputReleaseTest(t, repo, "tag", "--list", "v1.1.0"); tag != "" {
		t.Fatalf("release created tag %q", tag)
	}
}

func TestRunnerReleaseInteractiveDeclineCancelsNatively(t *testing.T) {
	repo := seedReleaseApplyRepo(t, "feat: decline native release execution")
	var stdout bytes.Buffer

	err := Runner{
		Stdout:     &stdout,
		Stdin:      strings.NewReader("n\n"),
		WorkingDir: repo,
	}.Run([]string{"release", "--no-tag", "--no-gh"})
	if err != nil {
		t.Fatalf("interactive release decline error = %v\n%s", err, stdout.String())
	}
	output := stdout.String()
	for _, want := range []string{"Proceed with release", "Release cancelled."} {
		if !strings.Contains(output, want) {
			t.Fatalf("stdout = %q, want %q", output, want)
		}
	}
	if strings.Contains(output, "Executing:") {
		t.Fatalf("stdout = %q, should not execute after decline", output)
	}
	body, err := os.ReadFile(filepath.Join(repo, "package.json"))
	if err != nil {
		t.Fatalf("ReadFile(package.json) error = %v", err)
	}
	if !strings.Contains(string(body), `"version": "1.0.0"`) {
		t.Fatalf("package.json mutated after decline:\n%s", string(body))
	}
	subject := gitOutputReleaseTest(t, repo, "log", "-1", "--pretty=%s")
	if subject != "feat: decline native release execution" {
		t.Fatalf("HEAD subject = %q, want feature commit after cancellation", subject)
	}
}

func TestRunnerReleaseYesNoTagNoGhExecutesNatively(t *testing.T) {
	repo := seedReleaseApplyRepo(t, "feat: ship native release execution")
	var stdout bytes.Buffer

	err := Runner{
		Stdout:     &stdout,
		WorkingDir: repo,
	}.Run([]string{"release", "--yes", "--no-tag", "--no-gh"})
	if err != nil {
		t.Fatalf("release --yes --no-tag --no-gh error = %v\n%s", err, stdout.String())
	}
	output := stdout.String()
	for _, want := range []string{
		"loaf release",
		"Executing:",
		"Updated package.json (1.0.0 → 1.1.0)",
		"Updated CHANGELOG.md",
		"Ran npm run build",
		"Committed release artifacts",
		"Git tag skipped (--no-tag)",
		"GitHub release skipped (--no-gh)",
		"Release ",
		"v1.1.0",
		"complete",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("stdout = %q, want %q", output, want)
		}
	}
	body, err := os.ReadFile(filepath.Join(repo, "package.json"))
	if err != nil {
		t.Fatalf("ReadFile(package.json) error = %v", err)
	}
	if !strings.Contains(string(body), `"version": "1.1.0"`) {
		t.Fatalf("package.json = %s, want bumped version", string(body))
	}
	changelog, err := os.ReadFile(filepath.Join(repo, "CHANGELOG.md"))
	if err != nil {
		t.Fatalf("ReadFile(CHANGELOG.md) error = %v", err)
	}
	for _, want := range []string{"## [Unreleased]", "- _No unreleased changes yet._", "## [1.1.0] - ", "### Added", "- Ship native release execution"} {
		if !strings.Contains(string(changelog), want) {
			t.Fatalf("CHANGELOG.md = %s, want %q", string(changelog), want)
		}
	}
	subject := gitOutputReleaseTest(t, repo, "log", "-1", "--pretty=%s")
	if subject != "chore: release v1.1.0" {
		t.Fatalf("release commit subject = %q, want chore: release v1.1.0", subject)
	}
	tag := gitOutputReleaseTest(t, repo, "tag", "--list", "v1.1.0")
	if tag != "" {
		t.Fatalf("tag v1.1.0 = %q, want skipped", tag)
	}
	show := gitOutputReleaseTest(t, repo, "show", "--name-only", "--format=", "HEAD")
	for _, want := range []string{"package.json", "CHANGELOG.md", "build-marker.txt"} {
		if !strings.Contains(show, want) {
			t.Fatalf("release commit files = %q, want %q", show, want)
		}
	}
}

func TestRunnerReleaseDryRunIsNative(t *testing.T) {
	repo := seedReleaseDryRunRepo(t, "feat: add native release dry run")
	var stdout bytes.Buffer

	err := Runner{
		Stdout:     &stdout,
		WorkingDir: repo,
	}.Run([]string{"release", "--dry-run"})
	if err != nil {
		t.Fatalf("release --dry-run error = %v\n%s", err, stdout.String())
	}
	output := stdout.String()
	for _, want := range []string{
		"loaf release",
		"Commits since tag:",
		"feat: add native release dry run",
		"Generated changelog:",
		"### Added",
		"- Add native release dry run",
		"Version files:",
		"package.json (1.0.0 → 1.1.0)",
		"Suggested bump:",
		"New version:",
		"Actions:",
		"--dry-run:",
		"No changes made.",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("stdout = %q, want %q", output, want)
		}
	}
	body, err := os.ReadFile(filepath.Join(repo, "package.json"))
	if err != nil {
		t.Fatalf("ReadFile(package.json) error = %v", err)
	}
	if !strings.Contains(string(body), `"version": "1.0.0"`) {
		t.Fatalf("package.json mutated during dry-run:\n%s", string(body))
	}
	changelog, err := os.ReadFile(filepath.Join(repo, "CHANGELOG.md"))
	if err != nil {
		t.Fatalf("ReadFile(CHANGELOG.md) error = %v", err)
	}
	if strings.Contains(string(changelog), "## [1.1.0]") {
		t.Fatalf("CHANGELOG.md mutated during dry-run:\n%s", string(changelog))
	}
}

func TestRunnerReleaseDryRunStopsWhenNoUnreleasedChanges(t *testing.T) {
	repo := seedReleaseTaggedRepo(t)
	var stdout bytes.Buffer

	err := Runner{
		Stdout:     &stdout,
		WorkingDir: repo,
	}.Run([]string{"release", "--dry-run"})
	if err != nil {
		t.Fatalf("release --dry-run error = %v\n%s", err, stdout.String())
	}
	output := stdout.String()
	for _, want := range []string{
		"Commits since tag:",
		"No unreleased changes found.",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("stdout = %q, want %q", output, want)
		}
	}
	for _, unwanted := range []string{
		"Generated changelog:",
		"Version files:",
		"Suggested bump:",
		"New version:",
		"Actions:",
	} {
		if strings.Contains(output, unwanted) {
			t.Fatalf("stdout = %q, did not want %q for empty release range", output, unwanted)
		}
	}
}

func TestRunnerReleaseDryRunValidatesFlagsNatively(t *testing.T) {
	repo := seedReleaseDryRunRepo(t, "fix: keep validation native")
	err := Runner{
		Stdout:     &bytes.Buffer{},
		WorkingDir: repo,
	}.Run([]string{"release", "--dry-run", "--bump", "bogus"})
	if err == nil || !strings.Contains(err.Error(), `Invalid bump type "bogus"`) {
		t.Fatalf("release invalid bump error = %v, want native validation", err)
	}
}

func TestRunnerReleaseDryRunValidatesBaseAndVersionFileNatively(t *testing.T) {
	repo := seedReleaseDryRunRepo(t, "fix: validate release inputs natively")
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "missing base",
			args: []string{"release", "--dry-run", "--base", "definitely-not-a-ref"},
			want: "does not exist or is not reachable",
		},
		{
			name: "missing version file",
			args: []string{"release", "--dry-run", "--version-file", "missing/package.json"},
			want: "version file missing/package.json not found",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := Runner{
				Stdout:     &bytes.Buffer{},
				WorkingDir: repo,
			}.Run(tc.args)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Run(%v) error = %v, want %q", tc.args, err, tc.want)
			}
		})
	}
}

func TestRunnerReleaseDryRunNormalizesSkipFlagsNatively(t *testing.T) {
	repo := seedReleaseDryRunRepo(t, "fix: skip gh when tag is skipped")
	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		WorkingDir: repo,
	}.Run([]string{"release", "--dry-run", "--no-tag"})
	if err != nil {
		t.Fatalf("release --dry-run --no-tag error = %v\n%s", err, stdout.String())
	}
	output := stdout.String()
	for _, want := range []string{"Create git tag v1.0.1 (--no-tag — skipped)", "Create GitHub release draft (--no-gh — skipped)"} {
		if !strings.Contains(output, want) {
			t.Fatalf("stdout = %q, want %q", output, want)
		}
	}
}

func TestRunnerReleaseDryRunPreMergeOverridesAreNative(t *testing.T) {
	repo := seedReleaseDryRunRepo(t, "fix: prepare release branch natively")
	gitCLI(t, repo, "config", "loaf.release.base", "v1.0.0")

	tests := []struct {
		name       string
		args       []string
		wantOut    []string
		wantErr    []string
		notWantOut []string
	}{
		{
			name:    "default skips tag and gh",
			args:    []string{"release", "--dry-run", "--pre-merge"},
			wantOut: []string{"Auto-detected base:", "--no-tag — skipped", "--no-gh — skipped"},
		},
		{
			name:       "tag override",
			args:       []string{"release", "--dry-run", "--pre-merge", "--tag", "--base", "v1.0.0"},
			wantErr:    []string{"--tag overrides --pre-merge default"},
			wantOut:    []string{"--no-gh — skipped"},
			notWantOut: []string{"--no-tag — skipped"},
		},
		{
			name:    "gh override warning",
			args:    []string{"release", "--dry-run", "--pre-merge", "--gh", "--base", "v1.0.0"},
			wantErr: []string{"--gh overrides --pre-merge default"},
		},
		{
			name:       "tag and gh override",
			args:       []string{"release", "--dry-run", "--pre-merge", "--tag", "--gh", "--base", "v1.0.0"},
			wantErr:    []string{"--tag overrides --pre-merge default", "--gh overrides --pre-merge default"},
			notWantOut: []string{"--no-tag — skipped", "--no-gh — skipped"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			err := Runner{
				Stdout:     &stdout,
				Stderr:     &stderr,
				WorkingDir: repo,
			}.Run(tc.args)
			if err != nil {
				t.Fatalf("Run(%v) error = %v\nstdout:\n%s\nstderr:\n%s", tc.args, err, stdout.String(), stderr.String())
			}
			for _, want := range tc.wantOut {
				if !strings.Contains(stdout.String(), want) {
					t.Fatalf("stdout = %q, want %q", stdout.String(), want)
				}
			}
			for _, want := range tc.wantErr {
				if !strings.Contains(stderr.String(), want) {
					t.Fatalf("stderr = %q, want %q", stderr.String(), want)
				}
			}
			for _, notWant := range tc.notWantOut {
				if strings.Contains(stdout.String(), notWant) {
					t.Fatalf("stdout = %q, should not contain %q", stdout.String(), notWant)
				}
			}
		})
	}
}

func TestRunnerReleasePostMergeRejectsIncompatibleFlagsNatively(t *testing.T) {
	repo := seedReleaseDryRunRepo(t, "fix: validate post merge flags")
	tests := []struct {
		args []string
		want string
	}{
		{args: []string{"release", "--post-merge", "--bump", "patch"}, want: "--post-merge is incompatible with --bump"},
		{args: []string{"release", "--post-merge", "--dry-run"}, want: "--post-merge is incompatible with --dry-run"},
		{args: []string{"release", "--post-merge", "--no-tag"}, want: "--post-merge is incompatible with --no-tag"},
		{args: []string{"release", "--post-merge", "--no-gh"}, want: "--post-merge is incompatible with --no-gh"},
		{args: []string{"release", "--post-merge", "--base", "main"}, want: "--post-merge is incompatible with --base"},
		{args: []string{"release", "--post-merge", "--pre-merge"}, want: "--post-merge is incompatible with --pre-merge"},
		{args: []string{"release", "--post-merge", "--version-file", "package.json"}, want: "--post-merge is incompatible with --version-file"},
		{args: []string{"release", "--post-merge", "--yes"}, want: "--post-merge is incompatible with --yes"},
	}
	for _, tc := range tests {
		t.Run(strings.Join(tc.args[2:], "_"), func(t *testing.T) {
			err := Runner{
				Stdout:     &bytes.Buffer{},
				WorkingDir: repo,
			}.Run(tc.args)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Run(%v) error = %v, want %q", tc.args, err, tc.want)
			}
		})
	}
}

func TestRunnerReleaseDryRunUsesConfiguredVersionFiles(t *testing.T) {
	repo := seedReleaseDryRunRepo(t, "fix: use configured version file")
	mkdirAll(t, filepath.Join(repo, ".agents"))
	mkdirAll(t, filepath.Join(repo, "backend"))
	writeFile(t, filepath.Join(repo, ".agents", "loaf.json"), "{\n  \"release\": {\n    \"versionFiles\": [\"backend/pyproject.toml\"]\n  }\n}\n")
	writeFile(t, filepath.Join(repo, "backend", "pyproject.toml"), "[project]\nname = \"backend\"\nversion = \"2.0.0\"\n")
	var stdout bytes.Buffer

	err := Runner{
		Stdout:     &stdout,
		WorkingDir: repo,
	}.Run([]string{"release", "--dry-run"})
	if err != nil {
		t.Fatalf("release --dry-run error = %v\n%s", err, stdout.String())
	}
	output := stdout.String()
	if !strings.Contains(output, "backend/pyproject.toml (2.0.0 → 2.0.1)") {
		t.Fatalf("stdout = %q, want configured version file", output)
	}
	if strings.Contains(output, "package.json (1.0.0 →") {
		t.Fatalf("stdout = %q, should not use root package.json when config overrides exist", output)
	}
}

func TestRunnerReleaseDryRunShowsUvArtifactCommandNatively(t *testing.T) {
	repo := seedReleaseDryRunRepo(t, "fix: show python release artifact command")
	mkdirAll(t, filepath.Join(repo, ".agents"))
	mkdirAll(t, filepath.Join(repo, "backend"))
	writeFile(t, filepath.Join(repo, ".agents", "loaf.json"), "{\n  \"release\": {\n    \"versionFiles\": [\"backend/pyproject.toml\"]\n  }\n}\n")
	writeFile(t, filepath.Join(repo, "backend", "pyproject.toml"), "[project]\nname = \"backend\"\nversion = \"2.0.0\"\n")
	writeFile(t, filepath.Join(repo, "backend", "uv.lock"), "# lock\n")
	var stdout bytes.Buffer

	err := Runner{
		Stdout:     &stdout,
		WorkingDir: repo,
	}.Run([]string{"release", "--dry-run"})
	if err != nil {
		t.Fatalf("release --dry-run error = %v\n%s", err, stdout.String())
	}
	for _, want := range []string{"backend/pyproject.toml", "Run uv sync (backend)"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout = %q, want %q", stdout.String(), want)
		}
	}
	if strings.Contains(stdout.String(), "Run loaf build") {
		t.Fatalf("stdout = %q, should use uv sync instead of loaf build", stdout.String())
	}
}

func TestRunnerReleaseRunsUvSyncForConfiguredPyprojectNatively(t *testing.T) {
	repo, base := seedReleasePyprojectApplyRepo(t, "feat: sync python release artifacts")
	fakeBin := realpath(t, t.TempDir())
	writeFile(t, filepath.Join(fakeBin, "uv"), strings.Join([]string{
		"#!/bin/sh",
		"test \"$1\" = \"sync\" || exit 64",
		"printf 'synced\\n' > uv-marker.txt",
		"printf '# lock\\nsynced\\n' > uv.lock",
		"",
	}, "\n"))
	if err := os.Chmod(filepath.Join(fakeBin, "uv"), 0o755); err != nil {
		t.Fatalf("Chmod(fake uv) error = %v", err)
	}
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))
	var stdout bytes.Buffer

	err := Runner{
		Stdout:     &stdout,
		WorkingDir: repo,
	}.Run([]string{"release", "--bump", "patch", "--yes", "--no-tag", "--no-gh", "--base", base})
	if err != nil {
		t.Fatalf("release pyproject error = %v\n%s", err, stdout.String())
	}
	for _, want := range []string{"backend/pyproject.toml", "Ran uv sync in backend"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout = %q, want %q", stdout.String(), want)
		}
	}
	marker, err := os.ReadFile(filepath.Join(repo, "backend", "uv-marker.txt"))
	if err != nil {
		t.Fatalf("ReadFile(uv-marker.txt) error = %v", err)
	}
	if string(marker) != "synced\n" {
		t.Fatalf("uv-marker.txt = %q, want synced marker", marker)
	}
	show := gitOutputReleaseTest(t, repo, "show", "--name-only", "--format=", "HEAD")
	for _, want := range []string{"backend/pyproject.toml", "backend/uv.lock", "backend/uv-marker.txt"} {
		if !strings.Contains(show, want) {
			t.Fatalf("release commit files = %q, want %q", show, want)
		}
	}
}

func TestRunnerReleaseRefusesUnignoredVirtualenvFromUvSyncNatively(t *testing.T) {
	repo, base := seedReleasePyprojectApplyRepo(t, "feat: catch virtualenv release artifact")
	fakeBin := realpath(t, t.TempDir())
	writeFile(t, filepath.Join(fakeBin, "uv"), strings.Join([]string{
		"#!/bin/sh",
		"test \"$1\" = \"sync\" || exit 64",
		"mkdir -p .venv/bin",
		"printf 'python\\n' > .venv/bin/python",
		"printf '# lock\\nsynced\\n' > uv.lock",
		"",
	}, "\n"))
	if err := os.Chmod(filepath.Join(fakeBin, "uv"), 0o755); err != nil {
		t.Fatalf("Chmod(fake uv) error = %v", err)
	}
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))
	var stdout bytes.Buffer

	err := Runner{
		Stdout:     &stdout,
		WorkingDir: repo,
	}.Run([]string{"release", "--bump", "patch", "--yes", "--no-tag", "--no-gh", "--base", base})
	if err == nil || !strings.Contains(err.Error(), "unignored virtual environment path detected") || !strings.Contains(err.Error(), "backend/.venv/bin/python") {
		t.Fatalf("release pyproject venv error = %v, want virtualenv refusal\n%s", err, stdout.String())
	}
	subject := gitOutputReleaseTest(t, repo, "log", "-1", "--pretty=%s")
	if subject != "feat: catch virtualenv release artifact" {
		t.Fatalf("HEAD subject = %q, want feature commit after refusal", subject)
	}
}

func seedReleaseDryRunRepo(t *testing.T, commitSubject string) string {
	t.Helper()
	repo := seedReleaseTaggedRepo(t)
	writeFile(t, filepath.Join(repo, "feature.txt"), commitSubject+"\n")
	gitCLI(t, repo, "add", "feature.txt")
	gitCLI(t, repo, "commit", "-m", commitSubject)
	return repo
}

func seedReleaseLineageFreezeRepo(t *testing.T, version string) string {
	t.Helper()
	repo := seedReleaseDryRunRepo(t, "feat: exercise prerelease lineage release")
	packagePath := filepath.Join(repo, "package.json")
	body, err := os.ReadFile(packagePath)
	if err != nil {
		t.Fatal(err)
	}
	updated := []byte(strings.Replace(string(body), `"version": "1.0.0"`, fmt.Sprintf(`"version": %q`, version), 1))
	if !bytes.Equal(body, updated) {
		if err := os.WriteFile(packagePath, updated, 0o644); err != nil {
			t.Fatal(err)
		}
		gitCLI(t, repo, "add", "package.json")
		gitCLI(t, repo, "commit", "-m", "chore: set release version")
	}
	writeChangeFolder(t, repo, "20260710-root", executableLineageDoc("root", "line", "", "terminal"))
	commitAllChangeTest(t, repo, "docs: add frozen release lineage")
	return repo
}

func seedReleaseTaggedRepo(t *testing.T) string {
	t.Helper()
	repo := realpath(t, t.TempDir())
	gitCLI(t, repo, "init", "-b", "main")
	gitCLI(t, repo, "config", "user.name", "Loaf Test")
	gitCLI(t, repo, "config", "user.email", "loaf@example.test")
	gitCLI(t, repo, "config", "commit.gpgsign", "false")
	gitCLI(t, repo, "config", "tag.gpgsign", "false")
	writeFile(t, filepath.Join(repo, "package.json"), "{\n  \"name\": \"release-fixture\",\n  \"version\": \"1.0.0\",\n  \"scripts\": {\n    \"build\": \"echo build\"\n  }\n}\n")
	writeFile(t, filepath.Join(repo, "CHANGELOG.md"), strings.Join([]string{
		"# Changelog",
		"",
		"## [Unreleased]",
		"",
		"- _No unreleased changes yet._",
		"",
	}, "\n"))
	gitCLI(t, repo, "add", ".")
	gitCLI(t, repo, "commit", "-m", "chore: initial release")
	gitCLI(t, repo, "tag", "v1.0.0")
	return repo
}

func seedReleasePyprojectApplyRepo(t *testing.T, commitSubject string) (string, string) {
	t.Helper()
	repo := realpath(t, t.TempDir())
	gitCLI(t, repo, "init", "-b", "main")
	gitCLI(t, repo, "config", "user.name", "Loaf Test")
	gitCLI(t, repo, "config", "user.email", "loaf@example.test")
	gitCLI(t, repo, "config", "commit.gpgsign", "false")
	gitCLI(t, repo, "config", "tag.gpgsign", "false")
	mkdirAll(t, filepath.Join(repo, ".agents"))
	mkdirAll(t, filepath.Join(repo, "backend"))
	writeFile(t, filepath.Join(repo, ".agents", "loaf.json"), "{\n  \"release\": {\n    \"versionFiles\": [\"backend/pyproject.toml\"]\n  }\n}\n")
	writeFile(t, filepath.Join(repo, "backend", "pyproject.toml"), "[project]\nname = \"backend\"\nversion = \"1.0.0\"\n")
	writeFile(t, filepath.Join(repo, "backend", "uv.lock"), "# lock\n")
	writeFile(t, filepath.Join(repo, "CHANGELOG.md"), strings.Join([]string{
		"# Changelog",
		"",
		"## [Unreleased]",
		"",
		"- Initial backend change",
		"",
		"## [1.0.0] - 2024-01-01",
		"",
		"- Initial release",
		"",
	}, "\n"))
	gitCLI(t, repo, "add", ".")
	gitCLI(t, repo, "commit", "-m", "chore: initial release")
	base := gitOutputReleaseTest(t, repo, "rev-parse", "HEAD")
	writeFile(t, filepath.Join(repo, "backend", "app.py"), commitSubject+"\n")
	gitCLI(t, repo, "add", "backend/app.py")
	gitCLI(t, repo, "commit", "-m", commitSubject)
	return repo, base
}

func seedReleaseApplyRepo(t *testing.T, commitSubject string) string {
	t.Helper()
	repo := realpath(t, t.TempDir())
	gitCLI(t, repo, "init", "-b", "main")
	gitCLI(t, repo, "config", "user.name", "Loaf Test")
	gitCLI(t, repo, "config", "user.email", "loaf@example.test")
	gitCLI(t, repo, "config", "commit.gpgsign", "false")
	gitCLI(t, repo, "config", "tag.gpgsign", "false")
	writeFile(t, filepath.Join(repo, "package.json"), strings.Join([]string{
		"{",
		`  "name": "release-fixture",`,
		`  "version": "1.0.0",`,
		`  "scripts": {`,
		`    "build": "node -e \"require('fs').writeFileSync('build-marker.txt','built')\""`,
		"  }",
		"}",
		"",
	}, "\n"))
	writeFile(t, filepath.Join(repo, "CHANGELOG.md"), strings.Join([]string{
		"# Changelog",
		"",
		"## [Unreleased]",
		"",
		"- _No unreleased changes yet._",
		"",
	}, "\n"))
	gitCLI(t, repo, "add", ".")
	gitCLI(t, repo, "commit", "-m", "chore: initial release")
	gitCLI(t, repo, "tag", "v1.0.0")
	writeFile(t, filepath.Join(repo, "feature.txt"), commitSubject+"\n")
	gitCLI(t, repo, "add", "feature.txt")
	gitCLI(t, repo, "commit", "-m", commitSubject)
	return repo
}

func gitOutputReleaseTest(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

type releasePostMergeCall struct {
	name string
	args []string
}

func mustResolveReleaseSnapshot(t *testing.T, repo string, options releaseOptions) releaseSnapshot {
	t.Helper()
	snap, err := resolveReleaseSnapshot(repo, options)
	if err != nil {
		t.Fatalf("resolveReleaseSnapshot: %v", err)
	}
	return snap
}

func seedReleasePostMergeFiles(t *testing.T, version string) string {
	t.Helper()
	repo := realpath(t, t.TempDir())
	writeFile(t, filepath.Join(repo, "package.json"), fmt.Sprintf("{\n  \"name\": \"release-fixture\",\n  \"version\": %q\n}\n", version))
	writeFile(t, filepath.Join(repo, "CHANGELOG.md"), strings.Join([]string{
		"# Changelog",
		"",
		"## [Unreleased]",
		"",
		"- _No unreleased changes yet._",
		"",
		"## [" + version + "] - 2026-04-29",
		"",
		"### Added",
		"- New feature (abc1234)",
		"",
	}, "\n"))
	return repo
}

func releasePostMergeHappyResponses(version string) map[string]releasePostMergeCommandResult {
	tag := "v" + version
	notes := "### Added\n- New feature (abc1234)"
	return map[string]releasePostMergeCommandResult{
		"git status --porcelain":                                                     releasePostMergeOK(""),
		"git symbolic-ref --short HEAD":                                              releasePostMergeOK("main"),
		"git config --get loaf.release.base":                                         releasePostMergeExit(1),
		"gh repo view --json defaultBranchRef -q .defaultBranchRef.name":             releasePostMergeOK("main"),
		"git symbolic-ref refs/remotes/origin/HEAD":                                  releasePostMergeOK("refs/remotes/origin/main"),
		"git log -1 --pretty=%s":                                                     releasePostMergeOK("feat: ship release-ready change (#42)"),
		"git diff HEAD^ HEAD --name-only":                                            releasePostMergeOK("CHANGELOG.md\npackage.json"),
		"git tag --list " + tag:                                                      releasePostMergeOK(""),
		"git ls-remote --tags origin refs/tags/" + tag:                               releasePostMergeOK(""),
		"gh release view " + tag:                                                     releasePostMergeExit(1),
		"git tag --points-at HEAD":                                                   releasePostMergeOK(""),
		"gh pr view 42 --json headRefName -q .headRefName":                           releasePostMergeOK("feat/cool-thing"),
		"git tag -s " + tag + " -m Release " + version:                               releasePostMergeOK(""),
		"git push origin " + tag:                                                     releasePostMergeOK(""),
		"gh release create " + tag + " --title " + tag + " --notes " + notes:         releasePostMergeOK(""),
		"gh release create " + tag + " --title " + tag + " --notes - A nifty change": releasePostMergeOK(""),
		"git pull --rebase origin main":                                              releasePostMergeOK(""),
		"git branch -d feat/cool-thing":                                              releasePostMergeOK(""),
		"git push origin --delete feat/cool-thing":                                   releasePostMergeOK(""),
	}
}

func scriptedReleasePostMergeRunner(responses map[string]releasePostMergeCommandResult) (releasePostMergeCommandRunner, func() []releasePostMergeCall) {
	var calls []releasePostMergeCall
	runner := func(root string, name string, args ...string) releasePostMergeCommandResult {
		calls = append(calls, releasePostMergeCall{name: name, args: append([]string{}, args...)})
		key := releasePostMergeCommandKey(name, args...)
		if result, ok := responses[key]; ok {
			return result
		}
		return releasePostMergeCommandResult{exitCode: 1}
	}
	return runner, func() []releasePostMergeCall {
		return append([]releasePostMergeCall{}, calls...)
	}
}

func releasePostMergeCommandKey(name string, args ...string) string {
	if len(args) == 0 {
		return name
	}
	return name + " " + strings.Join(args, " ")
}

func releasePostMergeCallKeys(calls []releasePostMergeCall) []string {
	keys := make([]string, 0, len(calls))
	for _, call := range calls {
		keys = append(keys, releasePostMergeCommandKey(call.name, call.args...))
	}
	return keys
}

func releasePostMergeOK(stdout string) releasePostMergeCommandResult {
	// Set raw and stdout identically so NUL-delimited fixtures survive the seam.
	return releasePostMergeCommandResult{stdout: stdout, raw: stdout, exitCode: 0}
}

func releasePostMergeExit(code int) releasePostMergeCommandResult {
	return releasePostMergeCommandResult{exitCode: code}
}

func containsReleasePostMergeCall(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
