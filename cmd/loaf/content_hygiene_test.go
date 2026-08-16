package main

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

func TestSkillContentHygieneStaleReferences(t *testing.T) {
	root := repoRoot(t)
	cases := []struct {
		rel       string
		forbidden []string
		required  []string
	}{
		{
			rel:       "content/skills/database-design/SKILL.md",
			forbidden: []string{"`infrastructure`"},
			required:  []string{"`infrastructure-management`"},
		},
		{
			rel:       "content/skills/power-systems-modeling/SKILL.md",
			forbidden: []string{"`database-patterns`"},
			required:  []string{"`database-design`"},
		},
		{
			rel: "content/skills/foundations/references/code-style.md",
			forbidden: []string{
				"`python` skill",
				"`typescript` skill",
				"`rails` skill",
			},
			required: []string{
				"`python-development` skill",
				"`typescript-development` skill",
				"`ruby-development` skill",
			},
		},
		{
			rel:       "content/skills/knowledge-base/SKILL.md",
			forbidden: []string{"CLAUDE.md", ".agents/AGENTS.md"},
			required:  []string{"AGENTS.md"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.rel, func(t *testing.T) {
			body := readTextFile(t, filepath.Join(root, filepath.FromSlash(tc.rel)))
			for _, forbidden := range tc.forbidden {
				if strings.Contains(body, forbidden) {
					t.Fatalf("%s still contains stale reference %q", tc.rel, forbidden)
				}
			}
			for _, required := range tc.required {
				if !strings.Contains(body, required) {
					t.Fatalf("%s missing replacement reference %q", tc.rel, required)
				}
			}
		})
	}
}

func TestSkillContentHygieneMarkdownStructure(t *testing.T) {
	root := repoRoot(t)
	skillsRoot := filepath.Join(root, "content", "skills")
	var failures []string
	err := filepath.WalkDir(skillsRoot, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || entry.Name() != "SKILL.md" {
			return nil
		}
		body := readTextFile(t, path)
		lines := strings.Split(body, "\n")
		if len(lines) > 100 && !strings.Contains(body, "\n## Contents\n") {
			failures = append(failures, relToRoot(t, root, path)+": missing ## Contents for "+strconv.Itoa(len(lines))+" lines")
		}
		if fences := strings.Count(body, "```"); fences%2 != 0 {
			failures = append(failures, relToRoot(t, root, path)+": unbalanced fenced code blocks")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WalkDir(%s) error = %v", skillsRoot, err)
	}
	sort.Strings(failures)
	if len(failures) > 0 {
		t.Fatalf("skill markdown structure hygiene failures:\n%s", strings.Join(failures, "\n"))
	}
}

func TestDocumentationStandardsDoesNotDuplicateADRTemplate(t *testing.T) {
	root := repoRoot(t)
	rel := filepath.FromSlash("content/skills/documentation-standards/references/documentation.md")
	body := readTextFile(t, filepath.Join(root, rel))
	for _, forbidden := range []string{"### ADR Template", "# ADR-XXX: Title"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("%s still publishes duplicate ADR authority %q", filepath.ToSlash(rel), forbidden)
		}
	}
	for _, required := range []string{"architecture", "templates/adr.md"} {
		if !strings.Contains(body, required) {
			t.Fatalf("%s missing ADR authority pointer %q", filepath.ToSlash(rel), required)
		}
	}
}

func TestSkillHelperExecutableContracts(t *testing.T) {
	root := repoRoot(t)

	validatorRel := filepath.FromSlash("content/skills/infrastructure-management/scripts/validate-k8s-manifest.py")
	validator := readTextFile(t, filepath.Join(root, validatorRel))
	for _, forbidden := range []string{"import yaml", "yaml.safe_load"} {
		if strings.Contains(validator, forbidden) {
			t.Fatalf("%s still depends on undeclared PyYAML via %q", filepath.ToSlash(validatorRel), forbidden)
		}
	}

	powerSkillDir := filepath.Join(root, "content", "skills", "power-systems-modeling")
	sidecar := readTextFile(t, filepath.Join(powerSkillDir, "SKILL.claude-code.yaml"))
	if hasShellScripts(t, filepath.Join(powerSkillDir, "scripts")) && !allowedToolsCanRunShellScripts(sidecar) {
		t.Fatalf("power-systems-modeling ships .sh helpers but sidecar allowed-tools cannot run shell scripts")
	}
}

func TestOrchestrationScriptSurfaceClassifiesEveryHelper(t *testing.T) {
	root := repoRoot(t)
	scriptsDir := filepath.Join(root, "content", "skills", "orchestration", "scripts")
	surfaceRel := filepath.FromSlash("content/skills/orchestration/references/script-surface.md")
	surface := readTextFile(t, filepath.Join(root, surfaceRel))

	var missing []string
	err := filepath.WalkDir(scriptsDir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		name := entry.Name()
		if !strings.Contains(surface, name) {
			missing = append(missing, name)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WalkDir(%s) error = %v", scriptsDir, err)
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Fatalf("%s does not classify orchestration helpers: %s", filepath.ToSlash(surfaceRel), strings.Join(missing, ", "))
	}
}

func TestOrchestrationDuplicateAuthorityReferencesRetired(t *testing.T) {
	root := repoRoot(t)
	orchestration := readTextFile(t, filepath.Join(root, "content", "skills", "orchestration", "SKILL.md"))
	for _, retired := range []string{
		"references/councils.md",
		"references/planning.md",
		"references/specs.md",
		"references/product-development.md",
	} {
		if strings.Contains(orchestration, retired) {
			t.Fatalf("orchestration router still links retired duplicate authority %q", retired)
		}
		if _, err := os.Stat(filepath.Join(root, "content", "skills", "orchestration", retired)); !os.IsNotExist(err) {
			t.Fatalf("%s stat = %v, want retired duplicate authority file missing", retired, err)
		}
	}
	for _, owner := range []string{
		"../council/SKILL.md",
		"../shape/SKILL.md",
	} {
		if !strings.Contains(orchestration, owner) {
			t.Fatalf("orchestration router missing owning skill link %q", owner)
		}
	}
}

func TestCliReferenceCatalogsJournalFamily(t *testing.T) {
	root := repoRoot(t)
	rel := filepath.FromSlash("content/skills/loaf-reference/SKILL.md")
	body := readTextFile(t, filepath.Join(root, rel))
	// The thinned router lists each command once with its subcommands
	// comma-joined; the journal row must catalog the whole family.
	if !strings.Contains(body, "`loaf journal`") {
		t.Fatalf("%s missing the `loaf journal` command index row", filepath.ToSlash(rel))
	}
	if !strings.Contains(body, "log, recent, search, show, context, export") {
		t.Fatalf("%s missing the journal subcommand family in the command index", filepath.ToSlash(rel))
	}
	// The journal is the supported conversation-continuity namespace; no retired
	// lifecycle command should survive in the generated CLI reference.
	if strings.Contains(body, "loaf session") {
		t.Fatalf("%s still references the deleted `loaf session` namespace", filepath.ToSlash(rel))
	}
}

func TestPlanningVocabularyConverged(t *testing.T) {
	root := repoRoot(t)
	cases := []struct {
		rel       string
		forbidden []string
		required  []string
	}{
		{
			rel: "AGENTS.md",
			forbidden: []string{
				"TRANSITIONAL:",
				"Transition in progress — the Change model is landing.",
				"until the conversion pass",
				"spec-conversion-and-guidance-sweep",
			},
			required: []string{
				"**New work is an Issue.**",
				"`loaf issue check` validates readiness",
				"Releases are retroactive",
			},
		},
		{
			rel: "README.md",
			forbidden: []string{
				"**Spec-first pipeline**",
				"## The Pipeline",
				"PHASE 1:",
				"PHASE 2:",
				"PHASE 3:",
				"### Phase 1:",
				"### Phase 2:",
				"### Phase 3:",
			},
			required: []string{
				"**Issue workflow**",
				"## Workflow",
				"`loaf issue new`",
				"`loaf issue promote`",
				"`loaf release suggest`",
				"/pitch",
			},
		},
		{
			rel: "docs/VISION.md",
			forbidden: []string{
				"Every change flows through a deliberate pipeline",
				"Idea, Spec, Tasks, Code, Learnings",
				"pipeline's three-artifact model",
				"A pipeline that prevents scope creep",
			},
			required: []string{
				"The Loaf Flow runs pitch → shape → implement → ship → release, at two scales.",
				"`/shape` turns a brief into a bounded Change",
				"Change artifacts and compatible task records keep intent and execution inspectable",
				"Changes define the intended outcome, boundaries, and proof before implementation",
			},
		},
		{
			rel: "docs/STRATEGY.md",
			forbidden: []string{
				"## What Has Been Proven",
				"24 specs shipped, 6 in progress.",
				"1. **Journal continuity** (proven: SPEC-056",
				"4. **Agent routing enforcement** (next: SPEC-022)",
				"## What We Do Not Know Yet",
			},
			required: []string{
				"## Proven Principles",
				"**Continuity belongs to the project journal, not a session lifecycle.**",
				"**Loaf Flow completeness.**",
				"Existing spec and task records remain supported compatibility surfaces",
				"## Open Questions",
			},
		},
		{
			rel: "docs/knowledge/task-system.md",
			forbidden: []string{
				"# Task System",
				"Loaf implements a Shape Up-inspired task management system",
				"## Pipeline",
				"/shape → SPEC file",
				"## Journal Model (SPEC-056)",
				"One concern per task.",
			},
			required: []string{
				"# Work Records",
				"Loaf uses Change artifacts for new bounded work.",
				"## Current Workflow",
				"Primary bounded-work contract for new work",
				"These identifiers document provenance, not current workflow instructions.",
			},
		},
		{
			rel: "docs/ARCHITECTURE.md",
			forbidden: []string{
				"### Stateful Runtime Migration (ADR-014)",
				"The transition shape is a Go front controller",
				"### Mode-Aware Skills (Linear-Native Mode, ADR-011)",
				"The project journal is the **only** session-related structure (SPEC-056).",
			},
			required: []string{
				"### Native Stateful Runtime (ADR-014)",
				"ADR and SPEC identifiers cited in this document serve only as decision and work provenance.",
				"### Work Records and Optional Linear Tasks (ADR-011)",
				"## Change-First Execution Model",
				"New bounded work uses a Change as its primary contract.",
			},
		},
		{
			rel:       ".github/workflows/release.yml",
			forbidden: []string{"v2.0.0-dev.49", "for example v2.0.0"},
			required:  []string{"Release tag to publish, for example v0.2.20", "if: needs.resolve.outputs.dev != 'true'"},
		},
		{
			rel:       "content/skills/loaf-reference/SKILL.md",
			forbidden: []string{"Markdown-to-native transition"},
			required:  []string{"`transitional-tasks`", "Open task-board records retained for compatibility."},
		},
		{
			rel: "content/skills/loaf-reference/references/command-routing.md",
			forbidden: []string{
				"TRANSITIONAL",
				"conversion pass",
				"spec-conversion-and-guidance-sweep",
				"CLAUDE.md",
			},
			required: []string{
				"Shape new bounded work",
				"Start implementing new bounded work",
				"Continue an existing task or spec record",
				"`loaf task` and `loaf spec` remain readable for legacy records; new work is issues",
			},
		},
		{
			rel:       "content/skills/orchestration/references/context-management.md",
			forbidden: []string{"transitional tasks", "Markdown-to-native transition"},
			required:  []string{"`transitional-tasks`", "Leftover board records retained for compatibility", "Issues, reports, ADRs, and commits"},
		},
		{
			rel:       "content/skills/orchestration/references/parallel-agents.md",
			forbidden: []string{"dependency-wave orchestration"},
			required:  []string{"dependency-aware orchestration"},
		},
		{
			rel:       "config/hooks.yaml",
			forbidden: []string{"Journal-first (SPEC-056)", "active specs"},
			required:  []string{"Journal-first continuity has no session entity", "Block tracked ephemeral Markdown and dangling references from retained spec records"},
		},
		{
			rel:       "internal/state/journal_context.go",
			forbidden: []string{"during U6", "open transitional task"},
			// Split around the alignment padding: gofmt re-pads this block whenever a
			// longer constant name joins it, and the padding is not the contract.
			required: []string{"JournalContextLayerTasks", `= "transitional_tasks"`, `json:"transitional_tasks"`, "these keep existing callers source-compatible.", "open task-board record retained for compatibility"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.rel, func(t *testing.T) {
			body := readTextFile(t, filepath.Join(root, filepath.FromSlash(tc.rel)))
			for _, forbidden := range tc.forbidden {
				if strings.Contains(body, forbidden) {
					t.Fatalf("%s still contains leaked planning vocabulary %q", tc.rel, forbidden)
				}
			}
			for _, required := range tc.required {
				if !strings.Contains(body, required) {
					t.Fatalf("%s missing semantic replacement %q", tc.rel, required)
				}
			}
		})
	}
}

func TestOperationalArtifactsUseSemanticFilenames(t *testing.T) {
	root := repoRoot(t)
	implementationUnitPrefix := regexp.MustCompile(`(?i)^u[0-9]+[-_]`)
	developmentUnitFixtureLabel := regexp.MustCompile(`(?i)loaf-u[0-9]+`)
	var filenameFailures []string
	var fixtureFailures []string

	for _, relRoot := range []string{"cli/scripts", "docs/changes", "internal"} {
		walkRoot := filepath.Join(root, filepath.FromSlash(relRoot))
		err := filepath.WalkDir(walkRoot, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				return nil
			}
			rel := relToRoot(t, root, path)
			isResearchEvidence := relRoot == "docs/changes" && strings.Contains(filepath.ToSlash(rel), "/research/")
			checkFilename := relRoot == "cli/scripts" || isResearchEvidence
			checkFixtureContent := relRoot == "cli/scripts" || relRoot == "internal"
			if !checkFilename && !checkFixtureContent {
				return nil
			}
			if checkFilename && implementationUnitPrefix.MatchString(entry.Name()) {
				filenameFailures = append(filenameFailures, rel)
			}
			if checkFixtureContent {
				content, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				if developmentUnitFixtureLabel.Match(content) {
					fixtureFailures = append(fixtureFailures, rel)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("WalkDir(%s) error = %v", walkRoot, err)
		}
	}

	// Change contracts and historical SPEC/ADR provenance remain valid; only operational runners and retained research evidence must use semantic filenames.
	sort.Strings(filenameFailures)
	sort.Strings(fixtureFailures)
	if len(filenameFailures) > 0 || len(fixtureFailures) > 0 {
		var failures []string
		if len(filenameFailures) > 0 {
			failures = append(failures, "implementation-unit filename prefixes:\n"+strings.Join(filenameFailures, "\n"))
		}
		if len(fixtureFailures) > 0 {
			failures = append(failures, "development-unit fixture labels:\n"+strings.Join(fixtureFailures, "\n"))
		}
		t.Fatalf("operational artifacts still expose planning vocabulary:\n%s", strings.Join(failures, "\n"))
	}
}

func hasShellScripts(t *testing.T, dir string) bool {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir(%s) error = %v", dir, err)
	}
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".sh") {
			return true
		}
	}
	return false
}

func allowedToolsCanRunShellScripts(sidecar string) bool {
	for _, line := range strings.Split(sidecar, "\n") {
		if !strings.HasPrefix(strings.TrimSpace(line), "allowed-tools:") {
			continue
		}
		tools := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "allowed-tools:"))
		for _, rawTool := range strings.Split(tools, ",") {
			tool := strings.TrimSpace(rawTool)
			if tool == "Bash" || strings.Contains(tool, ".sh") || strings.Contains(tool, "bash") {
				return true
			}
		}
	}
	return false
}

func relToRoot(t *testing.T, root string, path string) string {
	t.Helper()
	rel, err := filepath.Rel(root, path)
	if err != nil {
		t.Fatalf("Rel(%s, %s) error = %v", root, path, err)
	}
	return filepath.ToSlash(rel)
}
