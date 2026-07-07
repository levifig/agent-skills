package main

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRoutingEvalDryRunValidatesCurrentSkillSuite(t *testing.T) {
	root := repoRoot(t)
	if _, err := exec.LookPath("node"); err != nil {
		t.Skipf("node not found: %v", err)
	}

	cmd := exec.Command("node", "cli/scripts/eval-skill-routing.mjs", "--dry-run")
	cmd.Dir = root
	cmd.Env = envWith("ANTHROPIC_API_KEY=")
	outputBytes, err := cmd.CombinedOutput()
	output := string(outputBytes)
	if err != nil {
		t.Fatalf("dry-run routing eval failed: %v\n%s", err, output)
	}

	for _, want := range []string{
		"Loaded 34 skills",
		"Selected cases: 119",
		"Suite validation passed.",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("dry-run output missing %q:\n%s", want, output)
		}
	}
}

func TestRoutingEvalContentHasNoPhantomSkillCases(t *testing.T) {
	root := repoRoot(t)
	body := readTextFile(t, filepath.Join(root, "cli", "scripts", "eval-skill-routing.mjs"))
	for _, forbidden := range []string{
		"council-session",
		"cleanup",
		"resume-session",
		"reference-session",
		"thermo-nuclear-code-quality-review",
	} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("routing eval still references phantom or absent skill %q", forbidden)
		}
	}
}

func TestSkillArchitectureCountMatchesCurrentTaxonomy(t *testing.T) {
	root := repoRoot(t)
	body := readTextFile(t, filepath.Join(root, "docs", "knowledge", "skill-architecture.md"))
	if !strings.Contains(body, "34 skills total: 19 workflow/default-invocable, 15 reference/knowledge") {
		t.Fatalf("skill architecture doc missing current 34-skill taxonomy")
	}
	if strings.Contains(body, "33 skills") || strings.Contains(body, "35 skills") {
		t.Fatalf("skill architecture doc still contains stale skill count")
	}
}
