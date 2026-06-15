package state

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTraceImportedTaskShowsSourcesAndRelationships(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	writeMarkdownImportFixture(t, root.Path(), "# Task body\n")
	result, err := ApplyMarkdownMigration(context.Background(), root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("ApplyMarkdownMigration() error = %v", err)
	}
	store, err := OpenStore(result.DatabasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()

	trace, err := store.Trace(context.Background(), root, "TASK-001")
	if err != nil {
		t.Fatalf("Trace(TASK-001) error = %v", err)
	}

	assertTaskProjectContext(t, root.Path(), trace.ContractVersion, trace.DatabaseScope, trace.DatabasePath, trace.ProjectID, trace.ProjectName, trace.ProjectCurrentPath)
	if trace.Entity.Kind != "task" || trace.Entity.Alias != "TASK-001" || trace.Entity.Title != "Example Task" || trace.Entity.Status != "todo" {
		t.Fatalf("Entity = %#v, want imported task metadata", trace.Entity)
	}
	if len(trace.Sources) != 1 || trace.Sources[0].Path != ".agents/tasks/TASK-001-example.md" || trace.Sources[0].Hash == "" {
		t.Fatalf("Sources = %#v, want task source path and hash", trace.Sources)
	}
	assertTraceRelationship(t, trace.Relationships, "outbound", "implements", "spec", "SPEC-001")
	assertTraceRelationship(t, trace.Relationships, "outbound", "blocked_by", "task", "TASK-000")

	byInternalID, err := store.Trace(context.Background(), root, trace.Entity.ID)
	if err != nil {
		t.Fatalf("Trace(internal task id) error = %v", err)
	}
	if byInternalID.Entity.Alias != "TASK-001" {
		t.Fatalf("Trace(internal id).Entity.Alias = %q, want TASK-001", byInternalID.Entity.Alias)
	}
}

func TestTraceImportedSpecShowsInboundTaskRelationship(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	writeMarkdownImportFixture(t, root.Path(), "# Task body\n")
	if _, err := ApplyMarkdownMigration(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("ApplyMarkdownMigration() error = %v", err)
	}

	trace, err := Trace(context.Background(), root, PathResolver{StateHome: stateHome}, "SPEC-001")
	if err != nil {
		t.Fatalf("Trace(SPEC-001) error = %v", err)
	}

	if trace.Entity.Kind != "spec" || trace.Entity.Alias != "SPEC-001" {
		t.Fatalf("Entity = %#v, want imported spec", trace.Entity)
	}
	assertTraceRelationship(t, trace.Relationships, "inbound", "implements", "task", "TASK-001")
}

func TestTraceImportedLineageShowsSparkIdeaSpecTaskRelationships(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	ideaBody := `---
title: SQLite State
status: absorbed
resolved_by:
  - SPEC-001
  - TASK-001
exported_as: .agents/reports/export.md
---
# SQLite State
`
	sessionBody := `[2026-05-28 10:00] spark(architecture): sqlite-state captured to .agents/ideas/20260528-sqlite-state.md
[2026-05-28 10:01] resolve(spark): sqlite-state -> promoted to .agents/ideas/20260528-sqlite-state.md
`
	writeAgentsFile(t, root.Path(), "ideas/20260528-sqlite-state.md", ideaBody)
	writeAgentsFile(t, root.Path(), "specs/SPEC-001-sqlite-state.md", `---
id: SPEC-001
title: SQLite State Spec
status: accepted
---
# SQLite State Spec
`)
	writeAgentsFile(t, root.Path(), "tasks/TASK-001-sqlite-state.md", "# SQLite State Task\n")
	writeAgentsFile(t, root.Path(), "reports/export.md", "# Export\n")
	writeAgentsFile(t, root.Path(), "sessions/20260528-lineage.md", sessionBody)
	writeAgentsFile(t, root.Path(), "TASKS.json", `{
  "tasks": {
    "TASK-001": {
      "title": "SQLite State Task",
      "spec": "SPEC-001",
      "status": "todo",
      "priority": "P1"
    }
  }
}
`)

	result, err := ApplyMarkdownMigration(context.Background(), root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("ApplyMarkdownMigration() error = %v", err)
	}
	store, err := OpenStore(result.DatabasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()

	sparkTrace, err := store.Trace(context.Background(), root, "SPARK-sqlite-state")
	if err != nil {
		t.Fatalf("Trace(SPARK-sqlite-state) error = %v", err)
	}
	if sparkTrace.Entity.Kind != "spark" || sparkTrace.Entity.Alias != "SPARK-sqlite-state" {
		t.Fatalf("spark trace entity = %#v, want aliased spark", sparkTrace.Entity)
	}
	assertTraceRelationship(t, sparkTrace.Relationships, "outbound", "promoted_to", "idea", "20260528-sqlite-state")

	ideaTrace, err := store.Trace(context.Background(), root, "20260528-sqlite-state")
	if err != nil {
		t.Fatalf("Trace(20260528-sqlite-state) error = %v", err)
	}
	assertTraceRelationship(t, ideaTrace.Relationships, "inbound", "promoted_to", "spark", "SPARK-sqlite-state")
	assertTraceRelationship(t, ideaTrace.Relationships, "outbound", "resolved_by", "spec", "SPEC-001")
	assertTraceRelationship(t, ideaTrace.Relationships, "outbound", "resolved_by", "task", "TASK-001")
	assertTraceRelationship(t, ideaTrace.Relationships, "outbound", "exported_as", "report", "export")

	assertFileContent(t, filepath.Join(root.Path(), ".agents", "ideas", "20260528-sqlite-state.md"), ideaBody)
	assertFileContent(t, filepath.Join(root.Path(), ".agents", "sessions", "20260528-lineage.md"), sessionBody)
}

func TestTraceImportedLineageShowsBrainstormIdeaShapingSpecTaskRelationships(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	brainstormBody := `---
title: Runtime Brainstorm
status: open
promoted_to: .agents/ideas/20260528-runtime.md
---
# Runtime Brainstorm
`
	ideaBody := `---
title: Runtime Idea
status: shaping
promoted_to: .agents/drafts/20260528-runtime-draft.md
---
# Runtime Idea
`
	shapingBody := `---
kind: shaping_draft
title: Runtime Shaping Draft
status: finalized
promoted_to: SPEC-002
resolved_by: TASK-002
---
# Runtime Shaping Draft
`
	writeAgentsFile(t, root.Path(), "drafts/20260528-brainstorm-runtime.md", brainstormBody)
	writeAgentsFile(t, root.Path(), "ideas/20260528-runtime.md", ideaBody)
	writeAgentsFile(t, root.Path(), "drafts/20260528-runtime-draft.md", shapingBody)
	writeAgentsFile(t, root.Path(), "specs/SPEC-002-runtime.md", `---
id: SPEC-002
title: Runtime Spec
status: complete
---
# Runtime Spec
`)
	writeAgentsFile(t, root.Path(), "tasks/TASK-002-runtime.md", "# Runtime Task\n")
	writeAgentsFile(t, root.Path(), "TASKS.json", `{
  "tasks": {
    "TASK-002": {
      "title": "Runtime Task",
      "spec": "SPEC-002",
      "status": "done",
      "priority": "P1"
    }
  }
}
`)

	result, err := ApplyMarkdownMigration(context.Background(), root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("ApplyMarkdownMigration() error = %v", err)
	}
	if result.ShapingDrafts != 1 {
		t.Fatalf("ShapingDrafts = %d, want 1", result.ShapingDrafts)
	}
	store, err := OpenStore(result.DatabasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()

	brainstormTrace, err := store.Trace(context.Background(), root, "20260528-brainstorm-runtime")
	if err != nil {
		t.Fatalf("Trace(20260528-brainstorm-runtime) error = %v", err)
	}
	assertTraceRelationship(t, brainstormTrace.Relationships, "outbound", "promoted_to", "idea", "20260528-runtime")

	ideaTrace, err := store.Trace(context.Background(), root, "20260528-runtime")
	if err != nil {
		t.Fatalf("Trace(20260528-runtime) error = %v", err)
	}
	assertTraceRelationship(t, ideaTrace.Relationships, "inbound", "promoted_to", "brainstorm", "20260528-brainstorm-runtime")
	assertTraceRelationship(t, ideaTrace.Relationships, "outbound", "promoted_to", "shaping_draft", "20260528-runtime-draft")

	shapingTrace, err := store.Trace(context.Background(), root, "20260528-runtime-draft")
	if err != nil {
		t.Fatalf("Trace(20260528-runtime-draft) error = %v", err)
	}
	if shapingTrace.Entity.Kind != "shaping_draft" || shapingTrace.Entity.Title != "Runtime Shaping Draft" {
		t.Fatalf("shaping trace entity = %#v, want imported shaping draft", shapingTrace.Entity)
	}
	assertTraceRelationship(t, shapingTrace.Relationships, "inbound", "promoted_to", "idea", "20260528-runtime")
	assertTraceRelationship(t, shapingTrace.Relationships, "outbound", "promoted_to", "spec", "SPEC-002")
	assertTraceRelationship(t, shapingTrace.Relationships, "outbound", "resolved_by", "task", "TASK-002")

	specTrace, err := store.Trace(context.Background(), root, "SPEC-002")
	if err != nil {
		t.Fatalf("Trace(SPEC-002) error = %v", err)
	}
	assertTraceRelationship(t, specTrace.Relationships, "inbound", "promoted_to", "shaping_draft", "20260528-runtime-draft")
	assertTraceRelationship(t, specTrace.Relationships, "inbound", "implements", "task", "TASK-002")

	assertFileContent(t, filepath.Join(root.Path(), ".agents", "drafts", "20260528-brainstorm-runtime.md"), brainstormBody)
	assertFileContent(t, filepath.Join(root.Path(), ".agents", "ideas", "20260528-runtime.md"), ideaBody)
	assertFileContent(t, filepath.Join(root.Path(), ".agents", "drafts", "20260528-runtime-draft.md"), shapingBody)
}

func TestTraceMissingDatabaseReturnsActionableError(t *testing.T) {
	root := projectRoot(t)
	_, err := Trace(context.Background(), root, PathResolver{StateHome: t.TempDir()}, "TASK-001")
	if err == nil {
		t.Fatal("Trace() error = nil, want missing DB error")
	}
	if !strings.Contains(err.Error(), "loaf state migrate markdown --apply") {
		t.Fatalf("Trace() error = %v, want migration hint", err)
	}
}

func assertTraceRelationship(t *testing.T, relationships []TraceRelationship, direction string, relationshipType string, kind string, alias string) {
	t.Helper()
	for _, relationship := range relationships {
		if relationship.Direction == direction && relationship.Type == relationshipType && relationship.Entity.Kind == kind && relationship.Entity.Alias == alias {
			return
		}
	}
	t.Fatalf("relationship %s %s %s %s not found in %#v", direction, relationshipType, kind, alias, relationships)
}

func assertFileContent(t *testing.T, path string, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}
	if string(got) != want {
		t.Fatalf("content %s = %q, want original content", path, string(got))
	}
}
