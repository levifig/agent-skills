package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestWorkIdentityInArtifactNameClassifiesReferenceVersusIdentity(t *testing.T) {
	cases := []struct {
		name    string
		rel     string
		flagged bool
		label   string
	}{
		// Outward references: the name points at the work that produced the file.
		{"implementation unit prefix", "cli/scripts/u8-claude-smoke.mjs", true, "implementation unit"},
		{"implementation unit mid-name", "docs/changes/x/research/claude-u8-smoke.json", true, "implementation unit"},
		{"task record", ".agents/reports/archive/TASK-074-sidecar-audit-report.md", true, "task record"},
		{"spec record mid-name", ".agents/reports/report-spec-053-taxonomy-signoff.md", true, "spec record"},
		{"pull request mid-name", ".agents/handoffs/20260713-222603-land-pr-106-and-continue.md", true, "pull request"},
		{"issue", ".agents/plans/issue-42-triage.md", true, "issue"},
		{"underscore separator", ".agents/reports/u12_findings.md", true, "implementation unit"},

		// Identity, not reference: the artifact IS the numbered entity, in the
		// directory that owns that entity.
		{"spec in specs dir", ".agents/specs/SPEC-016-council-advisory-redesign.md", false, ""},
		{"archived spec in specs dir", ".agents/specs/archive/SPEC-001-loaf-self-sufficiency.md", false, ""},
		{"adr in decisions dir", "docs/decisions/ADR-007-project-config-location.md", false, ""},
		{"task in tasks dir", ".agents/tasks/TASK-001-configure-build-phases.md", false, ""},
		{"task in a relocated tasks dir", "docs/tasks/TASK-042-slug.md", false, ""},

		// A spec identity outside the directory that owns specs is a reference again.
		{"spec outside specs dir", ".agents/reports/SPEC-016-review.md", true, "spec record"},
		{"task outside tasks dir", ".agents/reports/TASK-074-audit.md", true, "task record"},
		{"task file naming a spec is still a reference", ".agents/tasks/spec-053-followup.md", true, "spec record"},

		// Versions are identity and timestamps record when, not which work unit.
		{"harness version", "docs/changes/x/research/claude-code-2.1.218-plugin-startup-smoke.json", false, ""},
		{"codex version", "docs/changes/x/research/codex-0.145.0-isolated-startup-smoke.json", false, ""},
		{"cursor build id", "docs/changes/x/research/cursor-agent-2026.05.09-0afadcc-isolation-preflight.json", false, ""},
		{"timestamp prefix", ".agents/reports/20260620-214448-audit-loaf-skills-deep-audit.md", false, ""},

		// Ordinary words that merely contain the letters must not trip the guard.
		{"startup is not a unit", "docs/changes/x/research/opencode-1.18.4-isolated-request-smoke.json", false, ""},
		{"u2f is not a unit", ".agents/plans/u2f-auth-rollout.md", false, ""},
		{"utf8 is not a unit", ".agents/reports/utf8-normalization.md", false, ""},
		{"semantic name", "docs/changes/x/research/target-capability-survey.md", false, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			label, flagged := workIdentityInArtifactName(tc.rel)
			if flagged != tc.flagged {
				t.Fatalf("workIdentityInArtifactName(%q) flagged = %v, want %v (label %q)", tc.rel, flagged, tc.flagged, label)
			}
			if flagged && label != tc.label {
				t.Fatalf("workIdentityInArtifactName(%q) label = %q, want %q", tc.rel, label, tc.label)
			}
		})
	}
}

func TestIsLoafAuthoredArtifactMatchesDirectoriesByBasename(t *testing.T) {
	// Matching by basename is what keeps the guard correct when artifact
	// directories move; none of these paths may depend on a .agents/ prefix.
	authored := []string{
		".agents/reports/note.md",
		".agents/specs/archive/note.md",
		"docs/reports/note.md",
		"workspace/nested/handoffs/note.md",
		"docs/changes/20260710-slug/research/evidence.json",
	}
	for _, rel := range authored {
		if !isLoafAuthoredArtifact(rel) {
			t.Errorf("isLoafAuthoredArtifact(%q) = false, want true", rel)
		}
	}

	notAuthored := []string{
		"src/main.go",
		"README.md",
		"internal/cli/check.go",
		"docs/changes/20260710-slug/change.md",
		"toplevel.md",
	}
	for _, rel := range notAuthored {
		if isLoafAuthoredArtifact(rel) {
			t.Errorf("isLoafAuthoredArtifact(%q) = true, want false", rel)
		}
	}
}

func TestFindWorkIdentifierArtifactNamesWalksAndSkips(t *testing.T) {
	root := t.TempDir()
	write := func(rel string) {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("MkdirAll(%s) error = %v", rel, err)
		}
		if err := os.WriteFile(path, []byte("body\n"), 0o644); err != nil {
			t.Fatalf("WriteFile(%s) error = %v", rel, err)
		}
	}

	write(".agents/reports/TASK-074-audit.md")
	write(".agents/reports/semantic-audit.md")
	write(".agents/specs/SPEC-016-redesign.md")
	write("docs/changes/20260710-slug/research/u8-evidence.json")
	write("docs/changes/20260710-slug/change.md")
	write("src/u9-handler.go")              // outside artifact dirs: not this check's business
	write("dist/reports/u8-generated.md")   // build output is skipped wholesale
	write("node_modules/reports/u8-dep.md") // dependencies are skipped wholesale

	violations, err := findWorkIdentifierArtifactNames(root)
	if err != nil {
		t.Fatalf("findWorkIdentifierArtifactNames error = %v", err)
	}
	got := make([]string, 0, len(violations))
	for _, violation := range violations {
		got = append(got, violation.rel)
	}
	want := []string{
		".agents/reports/TASK-074-audit.md",
		"docs/changes/20260710-slug/research/u8-evidence.json",
	}
	if len(got) != len(want) {
		t.Fatalf("violations = %v, want %v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("violations = %v, want %v", got, want)
		}
	}
}

func TestFindWorkIdentifierArtifactNamesJudgesTrackedPathsOnly(t *testing.T) {
	root := t.TempDir()
	write := func(rel string) {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("MkdirAll(%s) error = %v", rel, err)
		}
		if err := os.WriteFile(path, []byte("body\n"), 0o644); err != nil {
			t.Fatalf("WriteFile(%s) error = %v", rel, err)
		}
	}
	git := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com")
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v error = %v\n%s", args, err, output)
		}
	}

	write(".agents/reports/TASK-074-tracked.md")
	write(".agents/reports/u8-untracked-scratch.md")
	git("init", "-q")
	git("add", ".agents/reports/TASK-074-tracked.md")

	violations, err := findWorkIdentifierArtifactNames(root)
	if err != nil {
		t.Fatalf("findWorkIdentifierArtifactNames error = %v", err)
	}
	if len(violations) != 1 || violations[0].rel != ".agents/reports/TASK-074-tracked.md" {
		var got []string
		for _, violation := range violations {
			got = append(got, violation.rel)
		}
		t.Fatalf("violations = %v, want only the tracked path", got)
	}
}

func TestFindWorkIdentifierArtifactNamesGrandfathersClosedArtifacts(t *testing.T) {
	root := t.TempDir()
	write := func(rel string, body string) {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("MkdirAll(%s) error = %v", rel, err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatalf("WriteFile(%s) error = %v", rel, err)
		}
	}

	write(".agents/reports/u8-draft.md", "---\nstatus: draft\n---\n\nbody\n")
	write(".agents/reports/u8-final.md", "---\nstatus: final\n---\n\nbody\n")
	write(".agents/reports/u8-archived.md", "---\nstatus: archived\n---\n\nbody\n")
	write(".agents/reports/u8-quoted-final.md", "---\nstatus: \"final\"\n---\n\nbody\n")
	// A terminal state may be nested: a council records it under `council:`.
	write(".agents/reports/u8-nested-final.md", "---\ncouncil:\n  status: done\n---\n\nbody\n")
	write(".agents/reports/u8-nested-completed.md", "---\nmeta:\n  status: completed\n---\n\nbody\n")
	// A non-terminal status must not grandfather, nested or not.
	write(".agents/reports/u8-nested-draft.md", "---\ncouncil:\n  status: decided\n---\n\nbody\n")
	// A status word in the body must not silence the guard.
	write(".agents/reports/u8-body-claims-final.md", "# Notes\n\nstatus: final\n")
	// Nor may a file with no front matter at all.
	write(".agents/reports/u8-bare.md", "just prose\n")

	violations, err := findWorkIdentifierArtifactNames(root)
	if err != nil {
		t.Fatalf("findWorkIdentifierArtifactNames error = %v", err)
	}
	got := make([]string, 0, len(violations))
	for _, violation := range violations {
		got = append(got, violation.rel)
	}
	want := []string{
		".agents/reports/u8-bare.md",
		".agents/reports/u8-body-claims-final.md",
		".agents/reports/u8-draft.md",
		".agents/reports/u8-nested-draft.md",
	}
	if len(got) != len(want) {
		t.Fatalf("violations = %v, want %v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("violations = %v, want %v", got, want)
		}
	}
}
