package state

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/levifig/loaf/internal/project"
)

func TestExportAllJSONReturnsInternalSnapshot(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	writeMarkdownImportFixture(t, root.Path(), "# Task body\n")
	if _, err := ApplyMarkdownMigration(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("ApplyMarkdownMigration() error = %v", err)
	}

	snapshot, err := ExportAllJSON(context.Background(), root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("ExportAllJSON() error = %v", err)
	}

	if snapshot.ExportKind != ExportKindAll {
		t.Fatalf("ExportKind = %q, want %q", snapshot.ExportKind, ExportKindAll)
	}
	if snapshot.Format != ExportFormatJSON {
		t.Fatalf("Format = %q, want %q", snapshot.Format, ExportFormatJSON)
	}
	if snapshot.Audience != ExportAudienceLocal {
		t.Fatalf("Audience = %q, want %q", snapshot.Audience, ExportAudienceLocal)
	}
	if snapshot.ProjectID != ProjectID(root) {
		t.Fatalf("ProjectID = %q, want project id", snapshot.ProjectID)
	}
	if snapshot.DatabasePath == "" {
		t.Fatal("DatabasePath is empty")
	}
	if snapshot.SchemaVersion != 1 {
		t.Fatalf("SchemaVersion = %d, want 1", snapshot.SchemaVersion)
	}
	if snapshot.GeneratedAt == "" {
		t.Fatal("GeneratedAt is empty")
	}
	if len(snapshot.Tables["schema_migrations"]) != 1 {
		t.Fatalf("schema_migrations rows = %d, want 1", len(snapshot.Tables["schema_migrations"]))
	}
	if len(snapshot.Tables["projects"]) != 1 {
		t.Fatalf("projects rows = %d, want 1", len(snapshot.Tables["projects"]))
	}
	if len(snapshot.Tables["tasks"]) != 1 {
		t.Fatalf("tasks rows = %d, want 1", len(snapshot.Tables["tasks"]))
	}
	if len(snapshot.Tables["relationships"]) != 2 {
		t.Fatalf("relationships rows = %d, want 2", len(snapshot.Tables["relationships"]))
	}
	if snapshot.Tables["tasks"][0]["title"] != "Example Task" {
		t.Fatalf("task title = %#v, want imported title", snapshot.Tables["tasks"][0]["title"])
	}
}

func TestExportTableValidationRejectsUnfilteredProjectTables(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	status, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	store, err := OpenStore(status.DatabasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()
	if _, err := store.db.ExecContext(context.Background(), `CREATE TABLE leaky_export (id TEXT PRIMARY KEY, project_id TEXT NOT NULL)`); err != nil {
		t.Fatalf("create leaky_export error = %v", err)
	}

	err = store.validateExportTableFilters(context.Background(), []exportTable{{Name: "leaky_export", OrderBy: "id"}})
	if err == nil {
		t.Fatal("validateExportTableFilters() error = nil, want missing filter rejection")
	}
	if !strings.Contains(err.Error(), "has project_id but no filter column") {
		t.Fatalf("error = %v, want missing filter column rejection", err)
	}
}

func TestExportAllJSONDoesNotMutateDatabase(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	first, err := ExportAllJSON(context.Background(), root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("first ExportAllJSON() error = %v", err)
	}
	second, err := ExportAllJSON(context.Background(), root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("second ExportAllJSON() error = %v", err)
	}

	if !reflect.DeepEqual(first.Tables, second.Tables) {
		t.Fatalf("tables changed after export:\nfirst=%#v\nsecond=%#v", first.Tables, second.Tables)
	}
	if len(first.Tables["exports"]) != 0 || len(second.Tables["exports"]) != 0 {
		t.Fatalf("exports table mutated: first=%#v second=%#v", first.Tables["exports"], second.Tables["exports"])
	}
}

func TestExportAllJSONRequiresInitializedSQLiteState(t *testing.T) {
	root := projectRoot(t)
	_, err := ExportAllJSON(context.Background(), root, PathResolver{StateHome: t.TempDir()})
	if err == nil {
		t.Fatal("ExportAllJSON() error = nil, want missing-state error")
	}
	if !strings.Contains(err.Error(), "SQLite state database is not initialized") {
		t.Fatalf("error = %v, want initialized-state message", err)
	}
}

func TestExportAllJSONRejectsInvalidSQLiteState(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	path := mustDatabasePath(t, root, stateHome)
	writeInvalidDatabaseFile(t, path)

	_, err := ExportAllJSON(context.Background(), root, PathResolver{StateHome: stateHome})
	if err == nil {
		t.Fatal("ExportAllJSON() error = nil, want invalid-state error")
	}
	if !strings.Contains(err.Error(), "state database is invalid; run `loaf state doctor`") {
		t.Fatalf("error = %v, want doctor message", err)
	}
}

func TestExportTriageMarkdownReturnsExternalSafeSummary(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	if _, err := CaptureIdea(context.Background(), root, PathResolver{StateHome: stateHome}, IdeaCaptureOptions{Title: "Ship SPEC-001 follow-up from Track A"}); err != nil {
		t.Fatalf("CaptureIdea() error = %v", err)
	}
	if _, err := CaptureSpark(context.Background(), root, PathResolver{StateHome: stateHome}, SparkCaptureOptions{Scope: "Phase 2", Text: "TASK-002 came from .agents/tasks/TASK-002.md"}); err != nil {
		t.Fatalf("CaptureSpark() error = %v", err)
	}
	insertBrainstormForExport(t, root, stateHome, "Brainstorm from .agents/drafts/topic.md")

	export, err := ExportTriageMarkdown(context.Background(), root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("ExportTriageMarkdown() error = %v", err)
	}

	if export.ExportKind != ExportKindTriage {
		t.Fatalf("ExportKind = %q, want %q", export.ExportKind, ExportKindTriage)
	}
	if export.Format != ExportFormatMarkdown {
		t.Fatalf("Format = %q, want %q", export.Format, ExportFormatMarkdown)
	}
	if export.Audience != ExportAudienceExternal {
		t.Fatalf("Audience = %q, want %q", export.Audience, ExportAudienceExternal)
	}
	for _, want := range []string{"# Triage Export", "Audience: external", "## Ideas", "## Sparks", "## Brainstorms"} {
		if !strings.Contains(export.Content, want) {
			t.Fatalf("content = %q, want %q", export.Content, want)
		}
	}
	for _, banned := range []string{"SPEC-001", "TASK-002", ".agents/", "Track A", "Phase 2"} {
		if strings.Contains(export.Content, banned) {
			t.Fatalf("content leaked %q:\n%s", banned, export.Content)
		}
	}
	if !strings.Contains(export.Content, "internal reference") {
		t.Fatalf("content = %q, want sanitized internal reference marker", export.Content)
	}
	if err := ValidateExternalMarkdownExport(export.Content); err != nil {
		t.Fatalf("ValidateExternalMarkdownExport() error = %v", err)
	}
}

func TestExportTriageMarkdownIsDeterministicAndDoesNotMutateDatabase(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	if _, err := CaptureIdea(context.Background(), root, PathResolver{StateHome: stateHome}, IdeaCaptureOptions{Title: "B idea"}); err != nil {
		t.Fatalf("CaptureIdea(B) error = %v", err)
	}
	if _, err := CaptureIdea(context.Background(), root, PathResolver{StateHome: stateHome}, IdeaCaptureOptions{Title: "A idea"}); err != nil {
		t.Fatalf("CaptureIdea(A) error = %v", err)
	}

	first, err := ExportTriageMarkdown(context.Background(), root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("first ExportTriageMarkdown() error = %v", err)
	}
	second, err := ExportTriageMarkdown(context.Background(), root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("second ExportTriageMarkdown() error = %v", err)
	}
	if first.Content != second.Content {
		t.Fatalf("content changed:\nfirst=%s\nsecond=%s", first.Content, second.Content)
	}
	if strings.Index(first.Content, "A idea") > strings.Index(first.Content, "B idea") {
		t.Fatalf("content is not sorted deterministically:\n%s", first.Content)
	}
	snapshot, err := ExportAllJSON(context.Background(), root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("ExportAllJSON() error = %v", err)
	}
	if len(snapshot.Tables["exports"]) != 0 {
		t.Fatalf("exports table mutated: %#v", snapshot.Tables["exports"])
	}
}

func TestValidateExternalMarkdownExportRejectsPrivateReferences(t *testing.T) {
	for _, content := range []string{
		"mentions SPEC-001",
		"mentions TASK-002",
		"mentions .agents/specs/example.md",
		"mentions Track A",
		"mentions Track-A",
		"mentions track-a",
		"mentions Phase 2",
		"mentions Phase-2",
		"mentions phase-2",
	} {
		if err := ValidateExternalMarkdownExport(content); err == nil {
			t.Fatalf("ValidateExternalMarkdownExport(%q) error = nil, want rejection", content)
		}
	}
}

func TestExportTriageMarkdownRequiresInitializedSQLiteState(t *testing.T) {
	root := projectRoot(t)
	_, err := ExportTriageMarkdown(context.Background(), root, PathResolver{StateHome: t.TempDir()})
	if err == nil {
		t.Fatal("ExportTriageMarkdown() error = nil, want missing-state error")
	}
	if !strings.Contains(err.Error(), "SQLite state database is not initialized") {
		t.Fatalf("error = %v, want initialized-state message", err)
	}
}

func TestExportTriageMarkdownRejectsInvalidSQLiteState(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	path := mustDatabasePath(t, root, stateHome)
	writeInvalidDatabaseFile(t, path)

	_, err := ExportTriageMarkdown(context.Background(), root, PathResolver{StateHome: stateHome})
	if err == nil {
		t.Fatal("ExportTriageMarkdown() error = nil, want invalid-state error")
	}
	if !strings.Contains(err.Error(), "state database is invalid; run `loaf state doctor`") {
		t.Fatalf("error = %v, want doctor message", err)
	}
}

func TestExportReleaseReadinessMarkdownReturnsExternalSafeSummary(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	writeAgentsFile(t, root.Path(), "specs/SPEC-001-example.md", `---
id: SPEC-001
title: Example Spec
status: implementing
---
# Example Spec
`)
	writeAgentsFile(t, root.Path(), "tasks/TASK-001-example.md", "# Example Task\n")
	writeAgentsFile(t, root.Path(), "sessions/20260528-session.md", `---
branch: feature/SPEC-001-Phase-2
status: active
---
[2026-05-28 10:00] decision(sqlite): release readiness
`)
	writeAgentsFile(t, root.Path(), "reports/release.md", `---
kind: session
title: Release SPEC-001 Track A report
status: final
---
# Release Report
`)
	writeAgentsFile(t, root.Path(), "TASKS.json", `{
  "tasks": {
    "TASK-001": {"title": "Example Task", "spec": "SPEC-001", "status": "todo", "priority": "P1"}
  }
}
`)
	if _, err := ApplyMarkdownMigration(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("ApplyMarkdownMigration() error = %v", err)
	}
	insertGeneratedExportForReadiness(t, root, stateHome)

	export, err := ExportReleaseReadinessMarkdown(context.Background(), root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("ExportReleaseReadinessMarkdown() error = %v", err)
	}

	if export.ExportKind != ExportKindReleaseReadiness {
		t.Fatalf("ExportKind = %q, want %q", export.ExportKind, ExportKindReleaseReadiness)
	}
	if export.Format != ExportFormatMarkdown {
		t.Fatalf("Format = %q, want %q", export.Format, ExportFormatMarkdown)
	}
	if export.Audience != ExportAudienceExternal {
		t.Fatalf("Audience = %q, want external marker", export.Audience)
	}
	for _, want := range []string{
		"# Release Readiness Export",
		"Audience: external",
		"Release readiness: not ready",
		"Specs: 1 active, 0 complete, 0 archived",
		"Tasks: 1 unresolved, 0 done, 0 archived",
		"Sessions: 1 active, 1 total",
		"Reports: 0 draft, 1 total",
		"Specs: 1/1 with source",
		"Tasks: 1/1 with source",
		"Total relationships:",
		"release-readiness/markdown: 1",
		"session/final: Release internal reference internal reference report",
		"active session on feature/internal reference-internal reference with 1 journal entry",
	} {
		if !strings.Contains(export.Content, want) {
			t.Fatalf("content = %q, want %q", export.Content, want)
		}
	}
	for _, banned := range []string{"SPEC-001", "TASK-001", ".agents/", "Track A", "Phase 2"} {
		if strings.Contains(export.Content, banned) {
			t.Fatalf("content leaked %q:\n%s", banned, export.Content)
		}
	}
	if err := ValidateExternalMarkdownExport(export.Content); err != nil {
		t.Fatalf("ValidateExternalMarkdownExport() error = %v", err)
	}
}

func TestExportReleaseReadinessMarkdownIsDeterministicAndDoesNotMutateDatabase(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	first, err := ExportReleaseReadinessMarkdown(context.Background(), root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("first ExportReleaseReadinessMarkdown() error = %v", err)
	}
	second, err := ExportReleaseReadinessMarkdown(context.Background(), root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("second ExportReleaseReadinessMarkdown() error = %v", err)
	}
	if first.Content != second.Content {
		t.Fatalf("content changed:\nfirst=%s\nsecond=%s", first.Content, second.Content)
	}
	snapshot, err := ExportAllJSON(context.Background(), root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("ExportAllJSON() error = %v", err)
	}
	if len(snapshot.Tables["exports"]) != 0 {
		t.Fatalf("exports table mutated: %#v", snapshot.Tables["exports"])
	}
}

func TestExportReleaseReadinessMarkdownRequiresInitializedSQLiteState(t *testing.T) {
	root := projectRoot(t)
	_, err := ExportReleaseReadinessMarkdown(context.Background(), root, PathResolver{StateHome: t.TempDir()})
	if err == nil {
		t.Fatal("ExportReleaseReadinessMarkdown() error = nil, want missing-state error")
	}
	if !strings.Contains(err.Error(), "SQLite state database is not initialized") {
		t.Fatalf("error = %v, want initialized-state message", err)
	}
}

func TestExportReleaseReadinessMarkdownRejectsInvalidSQLiteState(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	path := mustDatabasePath(t, root, stateHome)
	writeInvalidDatabaseFile(t, path)

	_, err := ExportReleaseReadinessMarkdown(context.Background(), root, PathResolver{StateHome: stateHome})
	if err == nil {
		t.Fatal("ExportReleaseReadinessMarkdown() error = nil, want invalid-state error")
	}
	if !strings.Contains(err.Error(), "state database is invalid; run `loaf state doctor`") {
		t.Fatalf("error = %v, want doctor message", err)
	}
}

func TestExportSpecMarkdownRendersSpecSnapshot(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	writeAgentsFile(t, root.Path(), "specs/SPEC-001-example.md", `---
id: SPEC-001
title: Example Spec
status: implementing
---
# Example Spec

Imported spec prose.
`)
	writeAgentsFile(t, root.Path(), "tasks/TASK-001-todo.md", "# Todo task\n")
	writeAgentsFile(t, root.Path(), "tasks/TASK-002-progress.md", "# Progress task\n")
	writeAgentsFile(t, root.Path(), "tasks/TASK-003-done.md", "# Done task\n")
	writeAgentsFile(t, root.Path(), "TASKS.json", `{
  "tasks": {
    "TASK-001": {"title": "Todo Task", "spec": "SPEC-001", "status": "todo", "priority": "P1"},
    "TASK-002": {"title": "Progress Task", "spec": "SPEC-001", "status": "in_progress", "priority": "P1"},
    "TASK-003": {"title": "Done Task", "spec": "SPEC-001", "status": "done", "priority": "P2"}
  }
}
`)
	if _, err := ApplyMarkdownMigration(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("ApplyMarkdownMigration() error = %v", err)
	}

	export, err := ExportSpecMarkdown(context.Background(), root, PathResolver{StateHome: stateHome}, "SPEC-001")
	if err != nil {
		t.Fatalf("ExportSpecMarkdown() error = %v", err)
	}

	if export.ExportKind != ExportKindSpec {
		t.Fatalf("ExportKind = %q, want %q", export.ExportKind, ExportKindSpec)
	}
	if export.Format != ExportFormatMarkdown {
		t.Fatalf("Format = %q, want %q", export.Format, ExportFormatMarkdown)
	}
	if export.Audience != ExportAudienceLocal {
		t.Fatalf("Audience = %q, want internal marker", export.Audience)
	}
	for _, want := range []string{
		"# Spec Export",
		"Audience: internal",
		"Spec: `SPEC-001`",
		"Title: Example Spec",
		"Status: implementing",
		"Tasks: 1 todo, 1 in progress, 1 done",
		"`.agents/specs/SPEC-001-example.md`",
		"inbound `implements` task `TASK-001`",
		"# Example Spec",
		"Imported spec prose.",
	} {
		if !strings.Contains(export.Content, want) {
			t.Fatalf("content = %q, want %q", export.Content, want)
		}
	}
	if strings.Contains(export.Content, "status: implementing") || strings.Contains(export.Content, "---") {
		t.Fatalf("content = %q, want stripped frontmatter", export.Content)
	}
}

func TestExportSpecMarkdownIsDeterministicAndDoesNotMutateDatabase(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	writeAgentsFile(t, root.Path(), "specs/SPEC-001-example.md", `---
id: SPEC-001
title: Example Spec
status: implementing
---
# Example Spec
`)
	writeAgentsFile(t, root.Path(), "TASKS.json", `{"tasks":{}}
`)
	if _, err := ApplyMarkdownMigration(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("ApplyMarkdownMigration() error = %v", err)
	}

	first, err := ExportSpecMarkdown(context.Background(), root, PathResolver{StateHome: stateHome}, "SPEC-001")
	if err != nil {
		t.Fatalf("first ExportSpecMarkdown() error = %v", err)
	}
	second, err := ExportSpecMarkdown(context.Background(), root, PathResolver{StateHome: stateHome}, "SPEC-001")
	if err != nil {
		t.Fatalf("second ExportSpecMarkdown() error = %v", err)
	}
	if first.Content != second.Content {
		t.Fatalf("content changed:\nfirst=%s\nsecond=%s", first.Content, second.Content)
	}
	snapshot, err := ExportAllJSON(context.Background(), root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("ExportAllJSON() error = %v", err)
	}
	if len(snapshot.Tables["exports"]) != 0 {
		t.Fatalf("exports table mutated: %#v", snapshot.Tables["exports"])
	}
}

func TestExportSpecMarkdownRequiresInitializedSQLiteState(t *testing.T) {
	root := projectRoot(t)
	_, err := ExportSpecMarkdown(context.Background(), root, PathResolver{StateHome: t.TempDir()}, "SPEC-001")
	if err == nil {
		t.Fatal("ExportSpecMarkdown() error = nil, want missing-state error")
	}
	if !strings.Contains(err.Error(), "SQLite state database is not initialized") {
		t.Fatalf("error = %v, want initialized-state message", err)
	}
}

func TestExportSpecMarkdownRejectsInvalidSQLiteState(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	path := mustDatabasePath(t, root, stateHome)
	writeInvalidDatabaseFile(t, path)

	_, err := ExportSpecMarkdown(context.Background(), root, PathResolver{StateHome: stateHome}, "SPEC-001")
	if err == nil {
		t.Fatal("ExportSpecMarkdown() error = nil, want invalid-state error")
	}
	if !strings.Contains(err.Error(), "state database is invalid; run `loaf state doctor`") {
		t.Fatalf("error = %v, want doctor message", err)
	}
}

func TestExportSessionMarkdownRendersSessionSummary(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	writeAgentsFile(t, root.Path(), "sessions/20260528-session.md", `---
branch: feature/session-export
status: active
claude_session_id: harness-export
---
[2026-05-28 10:00] decision(sqlite): render this session
`)
	writeAgentsFile(t, root.Path(), "tasks/TASK-001-session.md", "# Session Task\n")
	writeAgentsFile(t, root.Path(), "TASKS.json", `{"tasks":{
  "TASK-001":{"title":"Session Task","status":"todo","priority":"P2"}
}}`)
	if _, err := ApplyMarkdownMigration(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("ApplyMarkdownMigration() error = %v", err)
	}
	if _, err := UpdateTask(context.Background(), root, PathResolver{StateHome: stateHome}, TaskUpdateOptions{
		Ref:        "TASK-001",
		Session:    "20260528-session",
		SetSession: true,
	}); err != nil {
		t.Fatalf("UpdateTask() error = %v", err)
	}

	export, err := ExportSessionMarkdown(context.Background(), root, PathResolver{StateHome: stateHome}, "20260528-session")
	if err != nil {
		t.Fatalf("ExportSessionMarkdown() error = %v", err)
	}

	if export.ExportKind != ExportKindSession {
		t.Fatalf("ExportKind = %q, want %q", export.ExportKind, ExportKindSession)
	}
	if export.Format != ExportFormatMarkdown {
		t.Fatalf("Format = %q, want %q", export.Format, ExportFormatMarkdown)
	}
	if export.Audience != ExportAudienceLocal {
		t.Fatalf("Audience = %q, want internal marker", export.Audience)
	}
	for _, want := range []string{
		"# Session Export",
		"Audience: internal",
		"Session: `20260528-session`",
		"Branch: `feature/session-export`",
		"Harness session: `harness-export`",
		"`.agents/sessions/20260528-session.md`",
		"`decision(sqlite)`: render this session",
		"inbound `associated_with` task `TASK-001`",
	} {
		if !strings.Contains(export.Content, want) {
			t.Fatalf("content = %q, want %q", export.Content, want)
		}
	}
}

func TestExportSessionMarkdownIsDeterministicAndDoesNotMutateDatabase(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	writeAgentsFile(t, root.Path(), "sessions/20260528-session.md", `---
branch: feature/session-export
status: active
---
[2026-05-28 10:00] decision(sqlite): render this session
`)
	writeAgentsFile(t, root.Path(), "TASKS.json", `{"tasks":{}}
`)
	if _, err := ApplyMarkdownMigration(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("ApplyMarkdownMigration() error = %v", err)
	}

	first, err := ExportSessionMarkdown(context.Background(), root, PathResolver{StateHome: stateHome}, "20260528-session")
	if err != nil {
		t.Fatalf("first ExportSessionMarkdown() error = %v", err)
	}
	second, err := ExportSessionMarkdown(context.Background(), root, PathResolver{StateHome: stateHome}, "20260528-session")
	if err != nil {
		t.Fatalf("second ExportSessionMarkdown() error = %v", err)
	}
	if first.Content != second.Content {
		t.Fatalf("content changed:\nfirst=%s\nsecond=%s", first.Content, second.Content)
	}
	snapshot, err := ExportAllJSON(context.Background(), root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("ExportAllJSON() error = %v", err)
	}
	if len(snapshot.Tables["exports"]) != 0 {
		t.Fatalf("exports table mutated: %#v", snapshot.Tables["exports"])
	}
}

func TestExportSessionMarkdownRequiresInitializedSQLiteState(t *testing.T) {
	root := projectRoot(t)
	_, err := ExportSessionMarkdown(context.Background(), root, PathResolver{StateHome: t.TempDir()}, "20260528-session")
	if err == nil {
		t.Fatal("ExportSessionMarkdown() error = nil, want missing-state error")
	}
	if !strings.Contains(err.Error(), "SQLite state database is not initialized") {
		t.Fatalf("error = %v, want initialized-state message", err)
	}
}

func TestExportSessionMarkdownRejectsInvalidSQLiteState(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	path := mustDatabasePath(t, root, stateHome)
	writeInvalidDatabaseFile(t, path)

	_, err := ExportSessionMarkdown(context.Background(), root, PathResolver{StateHome: stateHome}, "20260528-session")
	if err == nil {
		t.Fatal("ExportSessionMarkdown() error = nil, want invalid-state error")
	}
	if !strings.Contains(err.Error(), "state database is invalid; run `loaf state doctor`") {
		t.Fatalf("error = %v, want doctor message", err)
	}
}

func insertBrainstormForExport(t *testing.T, root project.Root, stateHome string, title string) {
	t.Helper()
	store, err := OpenStore(mustDatabasePath(t, root, stateHome))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	projectID := ProjectID(root)
	_, err = store.db.ExecContext(context.Background(), `
INSERT INTO brainstorms (id, project_id, title, status, created_at, updated_at)
VALUES ('brainstorm-export', ?, ?, 'open', ?, ?)
`, projectID, title, now, now)
	if err != nil {
		t.Fatalf("insert brainstorm error = %v", err)
	}
	_, err = store.db.ExecContext(context.Background(), `
INSERT INTO aliases (id, project_id, entity_kind, entity_id, namespace, alias, created_at, updated_at)
VALUES ('alias-brainstorm-export', ?, 'brainstorm', 'brainstorm-export', 'brainstorm', 'brainstorm-export', ?, ?)
`, projectID, now, now)
	if err != nil {
		t.Fatalf("insert brainstorm alias error = %v", err)
	}
}

func insertGeneratedExportForReadiness(t *testing.T, root project.Root, stateHome string) {
	t.Helper()
	store, err := OpenStore(mustDatabasePath(t, root, stateHome))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	projectID := ProjectID(root)
	_, err = store.db.ExecContext(context.Background(), `
INSERT INTO exports (id, project_id, export_kind, format, path, state_version, generated_at, created_at, updated_at)
VALUES ('export-release-readiness', ?, 'release-readiness', 'markdown', '.agents/reports/SPEC-001-release.md', 1, ?, ?, ?)
`, projectID, now, now, now)
	if err != nil {
		t.Fatalf("insert export error = %v", err)
	}
}

func writeInvalidDatabaseFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(path, []byte("not sqlite"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}
