package state

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestPreviewMarkdownMigrationCountsAgentsArtifacts(t *testing.T) {
	root := projectRoot(t)
	writeAgentsFile(t, root.Path(), "specs/SPEC-001-example.md", "# Spec\n")
	writeAgentsFile(t, root.Path(), "tasks/TASK-001-example.md", "# Task\n")
	writeAgentsFile(t, root.Path(), "ideas/20260528-idea.md", "# Idea\n")
	writeAgentsFile(t, root.Path(), "sessions/20260528-session.md", "[2026-05-28 10:00] spark(scope): capture this\n")
	writeAgentsFile(t, root.Path(), "reports/report.md", "# Report\n")
	writeAgentsFile(t, root.Path(), "drafts/20260528-brainstorm-topic.md", "# Brainstorm\n")
	writeAgentsFile(t, root.Path(), "tmp/unknown.txt", "skip me\n")
	writeAgentsFile(t, root.Path(), "TASKS.json", `{
  "tasks": {
    "TASK-001": {
      "spec": "SPEC-001",
      "depends_on": ["TASK-000", "TASK-002"]
    }
  }
}
`)

	plan, err := PreviewMarkdownMigration(root)
	if err != nil {
		t.Fatalf("PreviewMarkdownMigration() error = %v", err)
	}

	if plan.ContractVersion != StateJSONContractVersion {
		t.Fatalf("ContractVersion = %d, want %d", plan.ContractVersion, StateJSONContractVersion)
	}
	if plan.AgentsPath != filepath.Join(root.Path(), ".agents") {
		t.Fatalf("AgentsPath = %q, want project .agents", plan.AgentsPath)
	}
	if plan.Specs != 1 || plan.Tasks != 1 || plan.Ideas != 1 || plan.Sparks != 1 || plan.Brainstorms != 1 || plan.Sessions != 1 || plan.Reports != 1 {
		t.Fatalf("artifact counts = %#v, want one of each known artifact", plan)
	}
	if plan.Relationships != 3 {
		t.Fatalf("Relationships = %d, want 3", plan.Relationships)
	}
	wantSkipped := []string{".agents/tmp/unknown.txt"}
	if !reflect.DeepEqual(plan.SkippedFiles, wantSkipped) {
		t.Fatalf("SkippedFiles = %#v, want %#v", plan.SkippedFiles, wantSkipped)
	}
	wantIgnored := []MarkdownMigrationFileNote{{Path: ".agents/tmp/unknown.txt", Reason: "temporary enrichment artifact"}}
	if !reflect.DeepEqual(plan.IgnoredFiles, wantIgnored) {
		t.Fatalf("IgnoredFiles = %#v, want %#v", plan.IgnoredFiles, wantIgnored)
	}
	if len(plan.UnimportedFiles) != 0 {
		t.Fatalf("UnimportedFiles = %#v, want none", plan.UnimportedFiles)
	}
	if len(plan.Warnings) != 0 {
		t.Fatalf("Warnings = %#v, want none", plan.Warnings)
	}
	if plan.Warnings == nil {
		t.Fatal("Warnings = nil, want empty slice")
	}
}

func TestPreviewMarkdownMigrationFallsBackToTaskMarkdownRelationships(t *testing.T) {
	root := projectRoot(t)
	writeAgentsFile(t, root.Path(), "tasks/TASK-001-example.md", `---
spec: SPEC-001
depends_on:
  - TASK-000
  - TASK-002
---

## Notes

- TASK-999 is mentioned in the body but is not a dependency.
`)

	plan, err := PreviewMarkdownMigration(root)
	if err != nil {
		t.Fatalf("PreviewMarkdownMigration() error = %v", err)
	}

	if plan.Relationships != 3 {
		t.Fatalf("Relationships = %d, want 3", plan.Relationships)
	}
	if plan.SkippedFiles == nil {
		t.Fatal("SkippedFiles = nil, want empty slice")
	}
}

func TestPreviewMarkdownMigrationWarnsOnMalformedTasksJSON(t *testing.T) {
	root := projectRoot(t)
	writeAgentsFile(t, root.Path(), "TASKS.json", `{not json`)
	writeAgentsFile(t, root.Path(), "tasks/TASK-001-example.md", `---
spec: SPEC-001
---
# Task
`)

	plan, err := PreviewMarkdownMigration(root)
	if err != nil {
		t.Fatalf("PreviewMarkdownMigration() error = %v", err)
	}

	if plan.Relationships != 1 {
		t.Fatalf("Relationships = %d, want fallback frontmatter relationship", plan.Relationships)
	}
	if len(plan.Warnings) != 1 || !strings.Contains(plan.Warnings[0], "could not parse .agents/TASKS.json") {
		t.Fatalf("Warnings = %#v, want malformed TASKS.json warning", plan.Warnings)
	}
}

func TestPreviewMarkdownMigrationCountsShapingDraftsAndRelationshipFrontmatter(t *testing.T) {
	root := projectRoot(t)
	writeAgentsFile(t, root.Path(), "drafts/20260528-brainstorm-runtime.md", `---
promoted_to: .agents/ideas/20260528-runtime.md
---
# Runtime Brainstorm
`)
	writeAgentsFile(t, root.Path(), "ideas/20260528-runtime.md", `---
promoted_to: .agents/drafts/20260528-runtime-draft.md
---
# Runtime Idea
`)
	writeAgentsFile(t, root.Path(), "drafts/20260528-runtime-draft.md", `---
kind: shaping_draft
promoted_to: SPEC-001
resolved_by: TASK-001
---
# Runtime Shaping Draft
`)
	writeAgentsFile(t, root.Path(), "drafts/20260528-research-note.md", "# Research Note\n")

	plan, err := PreviewMarkdownMigration(root)
	if err != nil {
		t.Fatalf("PreviewMarkdownMigration() error = %v", err)
	}

	if plan.Brainstorms != 1 || plan.Ideas != 1 || plan.ShapingDrafts != 1 {
		t.Fatalf("plan = %#v, want brainstorm, idea, and shaping draft counts", plan)
	}
	if plan.Relationships != 4 {
		t.Fatalf("Relationships = %d, want frontmatter lineage count 4", plan.Relationships)
	}
	wantSkipped := []string{".agents/drafts/20260528-research-note.md"}
	if !reflect.DeepEqual(plan.SkippedFiles, wantSkipped) {
		t.Fatalf("SkippedFiles = %#v, want only generic draft skipped", plan.SkippedFiles)
	}
	wantUnimported := []MarkdownMigrationFileNote{{Path: ".agents/drafts/20260528-research-note.md", Reason: "draft is not classified as brainstorm or shaping draft"}}
	if !reflect.DeepEqual(plan.UnimportedFiles, wantUnimported) {
		t.Fatalf("UnimportedFiles = %#v, want generic draft note", plan.UnimportedFiles)
	}
	if len(plan.IgnoredFiles) != 0 {
		t.Fatalf("IgnoredFiles = %#v, want none", plan.IgnoredFiles)
	}
}

func TestPreviewMarkdownMigrationClassifiesSkippedFiles(t *testing.T) {
	root := projectRoot(t)
	writeAgentsFile(t, root.Path(), ".DS_Store", "metadata\n")
	writeAgentsFile(t, root.Path(), ".loaf-state", "{}\n")
	writeAgentsFile(t, root.Path(), "councils/20260615-mqtt-identity-model.md", "# Council\n")
	writeAgentsFile(t, root.Path(), "handoffs/20260617-security-wave-complete.md", "# Handoff\n")
	writeAgentsFile(t, root.Path(), "ideas/.gitkeep", "\n")
	writeAgentsFile(t, root.Path(), "plans/PLAN-010-break-glass-cli.md", "# Plan\n")
	writeAgentsFile(t, root.Path(), "reports/audit/STATE.json", "{}\n")
	writeAgentsFile(t, root.Path(), "skills/knowledge-base/SKILL.md", "# Skill\n")
	writeAgentsFile(t, root.Path(), "tmp/enrichment.txt", "temporary\n")

	plan, err := PreviewMarkdownMigration(root)
	if err != nil {
		t.Fatalf("PreviewMarkdownMigration() error = %v", err)
	}

	wantUnimported := []MarkdownMigrationFileNote{
		{Path: ".agents/councils/20260615-mqtt-identity-model.md", Reason: "unsupported artifact kind: council"},
		{Path: ".agents/handoffs/20260617-security-wave-complete.md", Reason: "unsupported artifact kind: handoff"},
		{Path: ".agents/plans/PLAN-010-break-glass-cli.md", Reason: "unsupported artifact kind: plan"},
		{Path: ".agents/reports/audit/STATE.json", Reason: "unsupported report support file; only Markdown reports are imported"},
		{Path: ".agents/skills/knowledge-base/SKILL.md", Reason: "project-local skill override is not imported into SQLite state"},
	}
	if !reflect.DeepEqual(plan.UnimportedFiles, wantUnimported) {
		t.Fatalf("UnimportedFiles = %#v, want %#v", plan.UnimportedFiles, wantUnimported)
	}

	wantIgnored := []MarkdownMigrationFileNote{
		{Path: ".agents/.DS_Store", Reason: "macOS metadata"},
		{Path: ".agents/.loaf-state", Reason: "legacy local state marker"},
		{Path: ".agents/ideas/.gitkeep", Reason: "directory placeholder"},
		{Path: ".agents/tmp/enrichment.txt", Reason: "temporary enrichment artifact"},
	}
	if !reflect.DeepEqual(plan.IgnoredFiles, wantIgnored) {
		t.Fatalf("IgnoredFiles = %#v, want %#v", plan.IgnoredFiles, wantIgnored)
	}
	if len(plan.SkippedFiles) != len(wantUnimported)+len(wantIgnored) {
		t.Fatalf("SkippedFiles = %#v, want legacy aggregate of unimported and ignored paths", plan.SkippedFiles)
	}
}

func TestPreviewMarkdownMigrationHandlesMissingAgentsDirectory(t *testing.T) {
	root := projectRoot(t)

	plan, err := PreviewMarkdownMigration(root)
	if err != nil {
		t.Fatalf("PreviewMarkdownMigration() error = %v", err)
	}

	if plan.AgentsPath != filepath.Join(root.Path(), ".agents") {
		t.Fatalf("AgentsPath = %q, want project .agents", plan.AgentsPath)
	}
	if plan.Specs != 0 || plan.Tasks != 0 || plan.Ideas != 0 || plan.Sparks != 0 || plan.Brainstorms != 0 || plan.ShapingDrafts != 0 || plan.Sessions != 0 || plan.Reports != 0 || plan.Relationships != 0 {
		t.Fatalf("plan = %#v, want zero counts", plan)
	}
	if len(plan.Warnings) != 1 || plan.Warnings[0] != ".agents directory not found" {
		t.Fatalf("Warnings = %#v, want missing .agents warning", plan.Warnings)
	}
	if plan.SkippedFiles == nil {
		t.Fatal("SkippedFiles = nil, want empty slice")
	}
}

func writeAgentsFile(t *testing.T, root string, rel string, content string) {
	t.Helper()
	path := filepath.Join(root, ".agents", filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}
}
