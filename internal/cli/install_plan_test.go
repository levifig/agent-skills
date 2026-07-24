package cli

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestParseInstallDryRunFlagGuards(t *testing.T) {
	if _, err := parseInstallArgs([]string{"--dry-run"}); err == nil || !strings.Contains(err.Error(), "requires --upgrade") {
		t.Fatalf("parseInstallArgs(--dry-run) error = %v, want requires --upgrade", err)
	}
	if _, err := parseInstallArgs([]string{"--json", "--upgrade"}); err == nil || !strings.Contains(err.Error(), "requires --dry-run") {
		t.Fatalf("parseInstallArgs(--json --upgrade) error = %v, want requires --dry-run", err)
	}
	options, err := parseInstallArgs([]string{"--upgrade", "--dry-run", "--json"})
	if err != nil {
		t.Fatalf("parseInstallArgs(--upgrade --dry-run --json) error = %v", err)
	}
	if !options.upgrade || !options.dryRun || !options.json {
		t.Fatalf("options = %#v, want upgrade/dryRun/json all set", options)
	}
}

// TestRunnerInstallUpgradeDryRunNonMutatingAcrossSurfaces hashes the fixture
// trees before and after a dry-run and requires byte-identical trees across the
// acceptance scenarios.
func TestRunnerInstallUpgradeDryRunNonMutatingAcrossSurfaces(t *testing.T) {
	t.Run("installed-target-stale-modified-foreign", func(t *testing.T) {
		root, home := setupInstallCommandFixture(t)
		writeInstallFile(t, filepath.Join(root, "dist", "cursor", "skills", "foundations", "SKILL.md"), "# Foundations\n")
		writeInstallFile(t, filepath.Join(root, "dist", "cursor", "skills", "go-development", "SKILL.md"), "# Go v1\n")
		writeInstallFile(t, filepath.Join(home, ".cursor", loafInstallMarkerFile), "old\n")
		runInstallFixture(t, root, "install", "--to", "cursor", "--yes")

		sharedSkills := filepath.Join(home, ".agents", "skills")
		// Stale owned: dist advances go-development, installed copy stays v1.
		writeInstallFile(t, filepath.Join(root, "dist", "cursor", "skills", "go-development", "SKILL.md"), "# Go v2\n")
		// Locally modified owned content -> conflict on apply.
		writeInstallFile(t, filepath.Join(sharedSkills, "foundations", "SKILL.md"), "# Foundations LOCAL EDIT\n")
		// Foreign unowned content that Loaf must never touch.
		writeInstallFile(t, filepath.Join(sharedSkills, "foreign", "SKILL.md"), "# Mine\n")

		plan := assertDryRunNonMutating(t, root, home, "install", "--to", "cursor", "--upgrade", "--dry-run", "--json")
		cursor := findTargetPlan(t, plan, "cursor")
		if got := skillAction(cursor, "go-development"); got != planActionUpdate {
			t.Fatalf("go-development action = %q, want update (stale owned)", got)
		}
		if got := skillAction(cursor, "foundations"); got != planActionConflict {
			t.Fatalf("foundations action = %q, want conflict (locally modified)", got)
		}
		if !cursor.Blocked {
			t.Fatalf("cursor plan Blocked = false, want true when a conflict is present")
		}
		if hasSkillDecision(cursor, "foreign") {
			t.Fatalf("plan referenced foreign unowned skill; it must be ignored")
		}
	})

	t.Run("no-installed-targets", func(t *testing.T) {
		root, home := setupInstallCommandFixture(t)
		plan := assertDryRunNonMutating(t, root, home, "install", "--upgrade", "--dry-run", "--json")
		if len(plan.Targets) != 0 {
			t.Fatalf("targets = %#v, want none for a fixture with no installed targets", plan.Targets)
		}
	})

	t.Run("deprecation-without-consent", func(t *testing.T) {
		root, home := setupInstallCommandFixture(t)
		retired := filepath.Join(home, ".retired-tool")
		writeInstallFile(t, filepath.Join(retired, loafInstallMarkerFile), "old\n")
		writeInstallFile(t, filepath.Join(retired, "skills", "stale", "SKILL.md"), "stale\n")
		writeInstallDeprecationManifest(t, root, `{
  "version": 1,
  "retired_targets": [
    {
      "target": "retired-tool",
      "since": "v9.9.0",
      "window": "one-release",
      "reason": "retired by test manifest",
      "paths": ["${HOME}/.retired-tool"]
    }
  ],
  "retired_skills": [],
  "relocations": [],
  "aliases": []
}`)

		plan := assertDryRunNonMutating(t, root, home, "install", "--upgrade", "--dry-run", "--json")
		if !plan.ConsentRequired {
			t.Fatalf("consent_required = false, want true for a destructive deprecation without --yes")
		}
		if len(plan.Deprecations) != 1 || plan.Deprecations[0].Action != "remove" || !plan.Deprecations[0].ConsentRequired {
			t.Fatalf("deprecations = %#v, want one remove entry needing consent", plan.Deprecations)
		}
		// The retired path must still be present after the dry-run.
		if _, err := os.Stat(retired); err != nil {
			t.Fatalf("retired target stat = %v, want still present after dry-run", err)
		}
		foundApply := false
		for _, command := range plan.FollowUpCommands {
			if command == "loaf install --upgrade --yes" {
				foundApply = true
			}
		}
		if !foundApply {
			t.Fatalf("follow_up_commands = %#v, want the exact --yes apply command", plan.FollowUpCommands)
		}
	})
}

// TestRunnerInstallUpgradeDryRunJSONIsDeterministic requires byte-identical JSON
// across two independent runs over the same fixture state.
func TestRunnerInstallUpgradeDryRunJSONIsDeterministic(t *testing.T) {
	root, home := setupInstallCommandFixture(t)
	writeInstallFile(t, filepath.Join(root, "dist", "cursor", "skills", "foundations", "SKILL.md"), "# Foundations\n")
	writeInstallFile(t, filepath.Join(root, "dist", "opencode", "skills", "foundations", "SKILL.md"), "# Foundations\n")
	writeInstallFile(t, filepath.Join(home, ".cursor", loafInstallMarkerFile), "old\n")
	mkdirAll(t, filepath.Join(home, ".config", "opencode"))
	writeInstallFile(t, filepath.Join(home, ".config", "opencode", loafInstallMarkerFile), "old\n")

	first := runInstallCapture(t, root, "install", "--upgrade", "--dry-run", "--json")
	second := runInstallCapture(t, root, "install", "--upgrade", "--dry-run", "--json")
	if first != second {
		t.Fatalf("dry-run JSON is not deterministic:\n--- first ---\n%s\n--- second ---\n%s", first, second)
	}
	var plan installDryRunPlan
	if err := json.Unmarshal([]byte(first), &plan); err != nil {
		t.Fatalf("unmarshal dry-run JSON: %v\n%s", err, first)
	}
	if plan.ContractVersion != installPlanContractVersion || plan.Command != "install" || !plan.DryRun {
		t.Fatalf("plan envelope = %#v, want contract %d command install dry_run true", plan, installPlanContractVersion)
	}
}

// TestRunnerInstallUpgradeDryRunSkillPlanMatchesApply verifies plan/apply
// parity: the per-artifact actions predicted by the dry-run are exactly what a
// subsequent apply performs.
func TestRunnerInstallUpgradeDryRunSkillPlanMatchesApply(t *testing.T) {
	root, home := setupInstallCommandFixture(t)
	distSkills := filepath.Join(root, "dist", "cursor", "skills")
	writeInstallFile(t, filepath.Join(distSkills, "foundations", "SKILL.md"), "# Foundations\n")
	writeInstallFile(t, filepath.Join(distSkills, "go-development", "SKILL.md"), "# Go v1\n")
	writeInstallFile(t, filepath.Join(distSkills, "legacy-skill", "SKILL.md"), "# Legacy\n")
	writeInstallFile(t, filepath.Join(home, ".cursor", loafInstallMarkerFile), "old\n")
	runInstallFixture(t, root, "install", "--to", "cursor", "--yes")

	sharedSkills := filepath.Join(home, ".agents", "skills")
	writeInstallFile(t, filepath.Join(sharedSkills, "foreign", "SKILL.md"), "# Mine\n")

	// Prepare the upgrade: foundations unchanged (preserve), go-development
	// advances (update), legacy-skill removed (retire), new-skill added (create).
	writeInstallFile(t, filepath.Join(distSkills, "go-development", "SKILL.md"), "# Go v2\n")
	if err := os.RemoveAll(filepath.Join(distSkills, "legacy-skill")); err != nil {
		t.Fatalf("remove legacy-skill from dist: %v", err)
	}
	writeInstallFile(t, filepath.Join(distSkills, "new-skill", "SKILL.md"), "# New\n")

	plan := parseInstallPlanJSON(t, runInstallCapture(t, root, "install", "--to", "cursor", "--upgrade", "--dry-run", "--json"))
	cursor := findTargetPlan(t, plan, "cursor")
	want := map[string]string{
		"foundations":    planActionPreserve,
		"go-development": planActionUpdate,
		"legacy-skill":   planActionRetire,
		"new-skill":      planActionCreate,
	}
	for skill, action := range want {
		if got := skillAction(cursor, skill); got != action {
			t.Fatalf("predicted %s action = %q, want %q", skill, got, action)
		}
	}

	// Apply and confirm the predicted effects actually happened.
	runInstallFixture(t, root, "install", "--to", "cursor", "--upgrade", "--yes")
	assertInstallFile(t, filepath.Join(sharedSkills, "foundations", "SKILL.md"), "# Foundations\n")
	assertInstallFile(t, filepath.Join(sharedSkills, "go-development", "SKILL.md"), "# Go v2\n")
	assertInstallFile(t, filepath.Join(sharedSkills, "new-skill", "SKILL.md"), "# New\n")
	assertInstallPathMissing(t, filepath.Join(sharedSkills, "legacy-skill"))
	assertInstallFile(t, filepath.Join(sharedSkills, "foreign", "SKILL.md"), "# Mine\n")
}

// --- helpers -------------------------------------------------------------

func runInstallFixture(t *testing.T, root string, args ...string) {
	t.Helper()
	var stdout bytes.Buffer
	err := Runner{Stdout: &stdout, WorkingDir: root, Executable: distributionFixtureExecutable(root)}.Run(args)
	if err != nil {
		t.Fatalf("%v error = %v\n%s", args, err, stdout.String())
	}
}

func runInstallCapture(t *testing.T, root string, args ...string) string {
	t.Helper()
	var stdout bytes.Buffer
	err := Runner{Stdout: &stdout, WorkingDir: root, Executable: distributionFixtureExecutable(root)}.Run(args)
	if err != nil {
		t.Fatalf("%v error = %v\n%s", args, err, stdout.String())
	}
	return stdout.String()
}

func assertDryRunNonMutating(t *testing.T, root string, home string, args ...string) installDryRunPlan {
	t.Helper()
	before := hashInstallFixtureTrees(t, root, home)
	output := runInstallCapture(t, root, args...)
	after := hashInstallFixtureTrees(t, root, home)
	if before != after {
		t.Fatalf("dry-run mutated the fixture trees: before=%s after=%s", before, after)
	}
	return parseInstallPlanJSON(t, output)
}

func parseInstallPlanJSON(t *testing.T, output string) installDryRunPlan {
	t.Helper()
	var plan installDryRunPlan
	if err := json.Unmarshal([]byte(output), &plan); err != nil {
		t.Fatalf("unmarshal dry-run JSON: %v\n%s", err, output)
	}
	return plan
}

func findTargetPlan(t *testing.T, plan installDryRunPlan, target string) targetDistributionPlan {
	t.Helper()
	for _, entry := range plan.Targets {
		if entry.Target == target {
			return entry
		}
	}
	t.Fatalf("target %q not found in plan %#v", target, plan.Targets)
	return targetDistributionPlan{}
}

func skillAction(target targetDistributionPlan, skill string) string {
	for _, artifact := range target.Artifacts {
		if artifact.ID == "skill:"+skill {
			return artifact.Action
		}
	}
	return ""
}

func hasSkillDecision(target targetDistributionPlan, skill string) bool {
	return skillAction(target, skill) != ""
}

// hashInstallFixtureTrees hashes the project root and home trees including file
// contents, symlink targets, and directory structure so any mutation the
// dry-run might make is detected.
func hashInstallFixtureTrees(t *testing.T, roots ...string) string {
	t.Helper()
	digest := sha256.New()
	for _, root := range roots {
		var lines []string
		err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			info, err := os.Lstat(path)
			if err != nil {
				return err
			}
			switch {
			case info.Mode()&fs.ModeSymlink != 0:
				target, err := os.Readlink(path)
				if err != nil {
					return err
				}
				lines = append(lines, fmt.Sprintf("L\t%s\t%s", rel, target))
			case info.IsDir():
				lines = append(lines, fmt.Sprintf("D\t%s\t%#o", rel, info.Mode().Perm()))
			default:
				body, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				lines = append(lines, fmt.Sprintf("F\t%s\t%#o\t%s", rel, info.Mode().Perm(), hex.EncodeToString(sha256Sum(body))))
			}
			return nil
		})
		if err != nil {
			t.Fatalf("hash fixture tree %s: %v", root, err)
		}
		sort.Strings(lines)
		fmt.Fprintf(digest, "root=%s\n%s\n", filepath.Base(root), strings.Join(lines, "\n"))
	}
	return hex.EncodeToString(digest.Sum(nil))
}

func sha256Sum(body []byte) []byte {
	sum := sha256.Sum256(body)
	return sum[:]
}

func TestPlanFencedSectionMatrixActions(t *testing.T) {
	generated := generateFencedContent()
	generatedFP := fencedContentFingerprint(generated)
	section, ok := findFencedSectionRange(generated)
	if !ok {
		t.Fatal("generated content missing section")
	}
	generatedBody := generated[section.bodyStart:section.end]
	oldBody := fencedWarning + "\nold content\n" + fencedEndMarker
	oldFP := sha256Hex(oldBody)

	cases := []struct {
		name       string
		seed       string
		wantAction string
	}{
		{"new_match_skipped", generated + "\n", "skipped"},
		{"new_differ_updated", "<!-- loaf:managed:start sha256=" + oldFP + " -->\n" + oldBody + "\n", "updated"},
		{"new_tamper_error", "<!-- loaf:managed:start sha256=" + generatedFP + " -->\ntampered\n" + fencedEndMarker + "\n", "error"},
		{"legacy_sha_transition", "<!-- loaf:managed:start v1.2.3 sha256=" + generatedFP + " -->\n" + generatedBody + "\n", "updated"},
		{"legacy_v_only", "<!-- loaf:managed:start v1.2.3 -->\nold\n" + fencedEndMarker + "\n", "updated"},
		{"absent_created", "", "created"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			target := filepath.Join(t.TempDir(), "AGENTS.md")
			if tc.seed != "" || tc.name != "absent_created" {
				if tc.name != "absent_created" {
					writeInstallFile(t, target, tc.seed)
				}
			}
			action, detail := planFencedSection(target, "9.9.9")
			if action != tc.wantAction {
				t.Fatalf("action = %q detail = %q, want %q", action, detail, tc.wantAction)
			}
		})
	}
}
