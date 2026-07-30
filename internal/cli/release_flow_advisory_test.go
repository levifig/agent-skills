package cli

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

const releaseFlowAdvisoryProbe = "releases are prepared on a release branch"

func TestReleaseFlowAdvisory(t *testing.T) {
	t.Run("prints for interactive mutating run on the default branch", func(t *testing.T) {
		repo := seedReleaseApplyRepo(t, "feat: advise the sanctioned flow")
		var stdout bytes.Buffer
		err := Runner{
			Stdout:     &stdout,
			Stdin:      strings.NewReader("n\n"),
			WorkingDir: repo,
		}.Run([]string{"release", "--no-tag", "--no-gh"})
		if err != nil {
			t.Fatalf("interactive release error = %v\n%s", err, stdout.String())
		}
		output := stdout.String()
		if !strings.Contains(output, releaseFlowAdvisoryProbe) {
			t.Fatalf("stdout = %q, want flow advisory", output)
		}
		if !strings.Contains(output, "--pre-merge") || !strings.Contains(output, "--post-merge") || !strings.Contains(output, "squash-merge the release PR") {
			t.Fatalf("stdout = %q, want advisory naming the two-phase sequence", output)
		}
		if advisoryIndex, analyzingIndex := strings.Index(output, releaseFlowAdvisoryProbe), strings.Index(output, "Analyzing"); analyzingIndex != -1 && advisoryIndex > analyzingIndex {
			t.Fatalf("stdout = %q, want advisory before the analysis phase", output)
		}
	})

	t.Run("prints for bump mutating run on the default branch", func(t *testing.T) {
		repo := seedReleaseApplyRepo(t, "feat: advise the sanctioned flow for bump")
		var stdout bytes.Buffer
		err := Runner{
			Stdout:     &stdout,
			Stdin:      strings.NewReader("n\n"),
			WorkingDir: repo,
		}.Run([]string{"release", "--bump", "patch", "--no-tag", "--no-gh"})
		if err != nil {
			t.Fatalf("release --bump error = %v\n%s", err, stdout.String())
		}
		if !strings.Contains(stdout.String(), releaseFlowAdvisoryProbe) {
			t.Fatalf("stdout = %q, want flow advisory", stdout.String())
		}
	})

	t.Run("absent for dry run", func(t *testing.T) {
		repo := seedReleaseApplyRepo(t, "feat: keep dry run silent")
		var stdout bytes.Buffer
		err := Runner{
			Stdout:     &stdout,
			WorkingDir: repo,
		}.Run([]string{"release", "--dry-run"})
		if err != nil {
			t.Fatalf("release --dry-run error = %v\n%s", err, stdout.String())
		}
		if strings.Contains(stdout.String(), releaseFlowAdvisoryProbe) {
			t.Fatalf("stdout = %q, advisory must not print for --dry-run", stdout.String())
		}
	})

	t.Run("absent for pre-merge", func(t *testing.T) {
		repo := seedReleaseApplyRepo(t, "feat: keep pre-merge silent")
		var stdout, stderr bytes.Buffer
		err := Runner{
			Stdout:     &stdout,
			Stderr:     &stderr,
			Stdin:      strings.NewReader("n\n"),
			WorkingDir: repo,
		}.Run([]string{"release", "--pre-merge", "--base", "v1.0.0"})
		if err != nil {
			t.Fatalf("release --pre-merge error = %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
		}
		if strings.Contains(stdout.String(), releaseFlowAdvisoryProbe) || strings.Contains(stderr.String(), releaseFlowAdvisoryProbe) {
			t.Fatalf("stdout = %q stderr = %q, advisory must not print for --pre-merge", stdout.String(), stderr.String())
		}
	})

	t.Run("predicate excludes two-phase and read-only modes on the default branch", func(t *testing.T) {
		repo := seedReleaseApplyRepo(t, "feat: gate the predicate")
		cases := []struct {
			name    string
			options releaseOptions
			want    bool
		}{
			{name: "interactive", options: releaseOptions{}, want: true},
			{name: "bump", options: releaseOptions{bump: "patch"}, want: true},
			{name: "dry-run", options: releaseOptions{dryRun: true}, want: false},
			{name: "pre-merge", options: releaseOptions{preMerge: true}, want: false},
			{name: "post-merge", options: releaseOptions{postMerge: true}, want: false},
			{name: "help", options: releaseOptions{help: true}, want: false},
		}
		for _, tc := range cases {
			if got := releaseInvocationWantsFlowAdvisory(repo, tc.options); got != tc.want {
				t.Fatalf("releaseInvocationWantsFlowAdvisory(%s) = %v, want %v", tc.name, got, tc.want)
			}
		}
	})

	t.Run("absent on a non-default branch", func(t *testing.T) {
		repo := seedReleaseApplyRepo(t, "feat: keep release branches silent")
		gitCLI(t, repo, "checkout", "-b", "release/v1.1.0")
		var stdout bytes.Buffer
		err := Runner{
			Stdout:     &stdout,
			Stdin:      strings.NewReader("n\n"),
			WorkingDir: repo,
		}.Run([]string{"release", "--bump", "patch", "--no-tag", "--no-gh"})
		if err != nil {
			t.Fatalf("release --bump on feature branch error = %v\n%s", err, stdout.String())
		}
		if strings.Contains(stdout.String(), releaseFlowAdvisoryProbe) {
			t.Fatalf("stdout = %q, advisory must not print off the default branch", stdout.String())
		}
	})

	t.Run("default branch resolves from local refs only", func(t *testing.T) {
		repo := seedReleaseApplyRepo(t, "feat: resolve default branch locally")
		if got := resolveReleaseDefaultBranch(repo); got != "main" {
			t.Fatalf("resolveReleaseDefaultBranch = %q, want main fallback", got)
		}
		head := gitOutputReleaseTest(t, repo, "rev-parse", "HEAD")
		gitCLI(t, repo, "update-ref", "refs/remotes/origin/trunk", head)
		gitCLI(t, repo, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/trunk")
		if got := resolveReleaseDefaultBranch(repo); got != "trunk" {
			t.Fatalf("resolveReleaseDefaultBranch = %q, want trunk via refs/remotes/origin/HEAD", got)
		}
		if releaseInvocationWantsFlowAdvisory(repo, releaseOptions{}) {
			t.Fatalf("predicate = true on main while origin/HEAD names trunk, want false")
		}
	})

	t.Run("predicate is false when no default branch resolves", func(t *testing.T) {
		repo := seedReleaseApplyRepo(t, "feat: tolerate unknown default branch")
		gitCLI(t, repo, "branch", "-m", "main", "develop")
		if got := resolveReleaseDefaultBranch(repo); got != "" {
			t.Fatalf("resolveReleaseDefaultBranch = %q, want empty without origin/HEAD or main/master", got)
		}
		if releaseInvocationWantsFlowAdvisory(repo, releaseOptions{}) {
			t.Fatalf("predicate = true without a resolvable default branch, want false")
		}
	})

	t.Run("prints once when preflight blocks on the default branch", func(t *testing.T) {
		// Incomplete stable cohort blocks before apply analysis; the advisory
		// must still print exactly once from the runRelease entry path.
		repo := seedReleaseApplyRepo(t, "feat: preflight still names the door")
		dir := writeNewLayoutChange(t, repo, "20260727-preflight-advisory", "preflight-advisory", "1.1.0", "")
		task := filepath.Join(dir, "tasks", "TASK-001-work.md")
		unchecked := "---\nchange: preflight-advisory\nid: TASK-001\ntitle: Work\n---\n\n# Work\n\n## Steps\n\n- [ ] Do it\n"
		writeFile(t, task, unchecked)
		gitCLI(t, repo, "add", ".")
		gitCLI(t, repo, "commit", "-m", "docs: shape incomplete stable cohort")

		var stdout bytes.Buffer
		err := Runner{
			Stdout:     &stdout,
			Stderr:     &bytes.Buffer{},
			WorkingDir: repo,
		}.Run([]string{"release", "--bump", "minor", "--yes", "--no-tag", "--no-gh"})
		if err == nil {
			t.Fatalf("release error = nil, want cohort preflight block")
		}
		if !strings.Contains(err.Error(), "targets 1.1.0 but is not executed") {
			t.Fatalf("error = %v, want incomplete cohort preflight block", err)
		}
		output := stdout.String()
		if !strings.Contains(output, releaseFlowAdvisoryProbe) {
			t.Fatalf("stdout = %q, want flow advisory before preflight failure", output)
		}
		if count := strings.Count(output, releaseFlowAdvisoryProbe); count != 1 {
			t.Fatalf("stdout advisory count = %d, want exactly once\n%s", count, output)
		}
		if strings.Contains(output, "Analyzing") {
			t.Fatalf("stdout = %q, preflight block must not reach apply analysis", output)
		}
	})
}

func TestReleaseGuardrailRemediation(t *testing.T) {
	t.Run("clean worktree refusal names the pre-merge flow", func(t *testing.T) {
		repo := seedReleaseApplyRepo(t, "feat: route curation to the release branch")
		writeFile(t, filepath.Join(repo, "notes.txt"), "uncommitted curation\n")
		err := (Runner{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}, WorkingDir: repo}).Run([]string{"release", "--bump", "patch", "--yes", "--no-tag", "--no-gh"})
		if err == nil {
			t.Fatalf("release on dirty worktree error = nil, want refusal")
		}
		msg := err.Error()
		if !strings.Contains(msg, "require a clean unignored worktree") {
			t.Fatalf("error = %q, want clean-worktree refusal", msg)
		}
		if !strings.Contains(msg, "changelog curation belongs on a release branch in the --pre-merge flow") {
			t.Fatalf("error = %q, want the pre-merge flow named as where curation belongs", msg)
		}
		if !strings.Contains(msg, "notes.txt") {
			t.Fatalf("error = %q, want dirty path listed", msg)
		}
	})

	t.Run("unpushed local tag keeps deletion advice", func(t *testing.T) {
		repo := seedReleasePostMergeFiles(t, "1.2.3")
		responses := releasePostMergeHappyResponses("1.2.3")
		responses["git tag --list v1.2.3"] = releasePostMergeOK("v1.2.3")
		runner, _ := scriptedReleasePostMergeRunner(responses)
		snap := mustResolveReleaseSnapshot(t, repo, releaseOptions{postMerge: true})

		result := checkReleasePostMergeGuardrails(repo, snap, runner)
		if result.ok || result.guardrail != 7 {
			t.Fatalf("result = %#v, want guardrail 7 failure", result)
		}
		if result.message != "tag v1.2.3 already exists locally — run `git tag -d v1.2.3` and rerun" {
			t.Fatalf("message = %q, want unpushed local tag deletion advice", result.message)
		}
	})

	t.Run("pushed local tag never gets deletion advice", func(t *testing.T) {
		repo := seedReleasePostMergeFiles(t, "1.2.3")
		responses := releasePostMergeHappyResponses("1.2.3")
		responses["git tag --list v1.2.3"] = releasePostMergeOK("v1.2.3")
		responses["git ls-remote --tags origin refs/tags/v1.2.3"] = releasePostMergeOK("deadbeef\trefs/tags/v1.2.3")
		runner, _ := scriptedReleasePostMergeRunner(responses)
		snap := mustResolveReleaseSnapshot(t, repo, releaseOptions{postMerge: true})

		result := checkReleasePostMergeGuardrails(repo, snap, runner)
		if result.ok || result.guardrail != 7 {
			t.Fatalf("result = %#v, want guardrail 7 failure", result)
		}
		if !strings.Contains(result.message, "tag v1.2.3 already exists and is pushed") || !strings.Contains(result.message, "do not delete a published tag") {
			t.Fatalf("message = %q, want pushed-tag non-destructive remediation", result.message)
		}
		if !strings.Contains(result.message, "re-run the Release workflow or recreate the release from the existing tag") {
			t.Fatalf("message = %q, want the non-destructive repair named", result.message)
		}
		if strings.Contains(result.message, "tag -d") {
			t.Fatalf("message = %q, must never advise deleting a pushed tag", result.message)
		}
	})

	t.Run("remote lookup failure degrades to the local wording", func(t *testing.T) {
		repo := seedReleasePostMergeFiles(t, "1.2.3")
		responses := releasePostMergeHappyResponses("1.2.3")
		responses["git tag --list v1.2.3"] = releasePostMergeOK("v1.2.3")
		responses["git ls-remote --tags origin refs/tags/v1.2.3"] = releasePostMergeExit(128)
		runner, _ := scriptedReleasePostMergeRunner(responses)
		snap := mustResolveReleaseSnapshot(t, repo, releaseOptions{postMerge: true})

		result := checkReleasePostMergeGuardrails(repo, snap, runner)
		if result.ok || result.guardrail != 7 {
			t.Fatalf("result = %#v, want guardrail 7 failure", result)
		}
		if result.message != "tag v1.2.3 already exists locally — run `git tag -d v1.2.3` and rerun" {
			t.Fatalf("message = %q, want degraded local wording when the remote lookup fails", result.message)
		}
	})

	t.Run("local tag with failed remote lookup still defers to an existing GH release", func(t *testing.T) {
		// Masking combination: local tag exists, ls-remote fails (remote unknown),
		// but gh release view succeeds — must never advise git tag -d.
		repo := seedReleasePostMergeFiles(t, "1.2.3")
		responses := releasePostMergeHappyResponses("1.2.3")
		responses["git tag --list v1.2.3"] = releasePostMergeOK("v1.2.3")
		responses["git ls-remote --tags origin refs/tags/v1.2.3"] = releasePostMergeExit(128)
		responses["gh release view v1.2.3"] = releasePostMergeOK("v1.2.3")
		runner, _ := scriptedReleasePostMergeRunner(responses)
		snap := mustResolveReleaseSnapshot(t, repo, releaseOptions{postMerge: true})

		result := checkReleasePostMergeGuardrails(repo, snap, runner)
		if result.ok || result.guardrail != 7 {
			t.Fatalf("result = %#v, want guardrail 7 failure", result)
		}
		if !strings.Contains(result.message, "GH release v1.2.3 already exists") || !strings.Contains(result.message, "do not delete a published release") {
			t.Fatalf("message = %q, want non-destructive GH release remediation", result.message)
		}
		if strings.Contains(result.message, "git tag -d") {
			t.Fatalf("message = %q, must not advise deletion when a GH release exists", result.message)
		}
	})

	t.Run("remote-only tag gets non-destructive advice", func(t *testing.T) {
		repo := seedReleasePostMergeFiles(t, "1.2.3")
		responses := releasePostMergeHappyResponses("1.2.3")
		responses["git ls-remote --tags origin refs/tags/v1.2.3"] = releasePostMergeOK("deadbeef\trefs/tags/v1.2.3")
		runner, _ := scriptedReleasePostMergeRunner(responses)
		snap := mustResolveReleaseSnapshot(t, repo, releaseOptions{postMerge: true})

		result := checkReleasePostMergeGuardrails(repo, snap, runner)
		if result.ok || result.guardrail != 7 {
			t.Fatalf("result = %#v, want guardrail 7 failure", result)
		}
		if !strings.Contains(result.message, "tag v1.2.3 already exists on remote") || !strings.Contains(result.message, "do not delete a published tag") {
			t.Fatalf("message = %q, want remote-tag non-destructive remediation", result.message)
		}
		if strings.Contains(result.message, ":refs/tags/") || strings.Contains(result.message, "tag -d") {
			t.Fatalf("message = %q, must never advise deleting a pushed tag", result.message)
		}
	})

	t.Run("existing GH release gets non-destructive advice", func(t *testing.T) {
		repo := seedReleasePostMergeFiles(t, "1.2.3")
		responses := releasePostMergeHappyResponses("1.2.3")
		responses["gh release view v1.2.3"] = releasePostMergeOK("v1.2.3 draft")
		runner, _ := scriptedReleasePostMergeRunner(responses)
		snap := mustResolveReleaseSnapshot(t, repo, releaseOptions{postMerge: true})

		result := checkReleasePostMergeGuardrails(repo, snap, runner)
		if result.ok || result.guardrail != 7 {
			t.Fatalf("result = %#v, want guardrail 7 failure", result)
		}
		if !strings.Contains(result.message, "GH release v1.2.3 already exists") || !strings.Contains(result.message, "do not delete a published release") {
			t.Fatalf("message = %q, want non-destructive GH release remediation", result.message)
		}
	})
}
