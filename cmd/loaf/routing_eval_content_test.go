package main

import (
	"os/exec"
	"path/filepath"
	"regexp"
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

	// Assert the invariant, never a snapshot: a pinned skill or case count turns
	// every skill addition into a test edit, and the number proves nothing that
	// the loaded/expected agreement does not already prove.
	loaded := regexp.MustCompile(`Loaded (\d+) skills`).FindStringSubmatch(output)
	expected := regexp.MustCompile(`Expected skill count: (\d+)`).FindStringSubmatch(output)
	if loaded == nil || expected == nil {
		t.Fatalf("dry-run output does not report both loaded and expected skill counts:\n%s", output)
	}
	if loaded[1] != expected[1] {
		t.Fatalf("dry-run loaded %s skills but the suite expects %s:\n%s", loaded[1], expected[1], output)
	}
	if !regexp.MustCompile(`Selected cases: [1-9]\d*`).MatchString(output) {
		t.Fatalf("dry-run selected no routing cases:\n%s", output)
	}
	if !strings.Contains(output, "Suite validation passed.") {
		t.Fatalf("dry-run output missing suite validation:\n%s", output)
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

func TestSkillArchitectureDescribesSemanticTaxonomy(t *testing.T) {
	root := repoRoot(t)
	body := readTextFile(t, filepath.Join(root, "docs", "knowledge", "skill-architecture.md"))
	for _, want := range []string{
		"## Categories",
		"| Category | `user-invocable` | Examples |",
		"| Reference/Knowledge | `false` |",
		"| Workflow/Process | `true` (default) |",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("skill architecture doc missing semantic taxonomy %q", want)
		}
	}
	if regexp.MustCompile(`(?i)\b[0-9]+\s+skills\s+total\b`).MatchString(body) {
		t.Fatal("skill architecture doc publishes a volatile exact skill-count snapshot")
	}
}
