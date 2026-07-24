package state

import (
	"context"
	"strings"
	"testing"
)

func TestClassifyImportLifecycleStatusNoOpinion(t *testing.T) {
	for _, tc := range []struct {
		name            string
		kind            string
		raw             string
		noOpinionInsert string
		want            string
	}{
		{name: "task absent", kind: LifecycleEntityTask, raw: "", noOpinionInsert: "unknown", want: "unknown"},
		{name: "task explicit unknown", kind: LifecycleEntityTask, raw: "unknown", noOpinionInsert: "unknown", want: "unknown"},
		{name: "task blank padded unknown", kind: LifecycleEntityTask, raw: "  unknown  ", noOpinionInsert: "unknown", want: "unknown"},
		{name: "report absent", kind: LifecycleEntityReport, raw: "", noOpinionInsert: "unknown", want: "unknown"},
		{name: "idea absent inserts open", kind: LifecycleEntityIdea, raw: "", noOpinionInsert: LifecycleStatusOpen, want: LifecycleStatusOpen},
		{name: "idea explicit unknown inserts open", kind: LifecycleEntityIdea, raw: "unknown", noOpinionInsert: LifecycleStatusOpen, want: LifecycleStatusOpen},
		{name: "brainstorm explicit unknown inserts open", kind: LifecycleEntityBrainstorm, raw: "unknown", noOpinionInsert: LifecycleStatusOpen, want: LifecycleStatusOpen},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyImportLifecycleStatus(tc.kind, "id", tc.raw, tc.noOpinionInsert)
			if got.Status != tc.want {
				t.Fatalf("Status = %q, want %q", got.Status, tc.want)
			}
			if got.OutOfVocabulary || got.Warning != "" {
				t.Fatalf("no-opinion must not warn: %#v", got)
			}
		})
	}
}

func TestClassifyImportLifecycleStatusNormalized(t *testing.T) {
	for _, tc := range []struct {
		name string
		kind string
		raw  string
		want string
	}{
		{name: "task canonical", kind: LifecycleEntityTask, raw: "in_progress", want: "in_progress"},
		{name: "task done", kind: LifecycleEntityTask, raw: "done", want: "done"},
		{name: "report legacy final", kind: LifecycleEntityReport, raw: "final", want: "done"},
		{name: "report archived directory-derived", kind: LifecycleEntityReport, raw: "archived", want: "archived"},
		{name: "idea legacy resolved", kind: LifecycleEntityIdea, raw: "resolved", want: "done"},
		{name: "brainstorm open", kind: LifecycleEntityBrainstorm, raw: "open", want: "open"},
		{name: "spec legacy complete", kind: LifecycleEntitySpec, raw: "complete", want: "done"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyImportLifecycleStatus(tc.kind, "id", tc.raw, "unknown")
			if got.Status != tc.want || got.OutOfVocabulary {
				t.Fatalf("got %#v, want status %q normalized", got, tc.want)
			}
		})
	}
}

func TestClassifyImportLifecycleStatusOutOfVocabulary(t *testing.T) {
	for _, tc := range []struct {
		name string
		kind string
		raw  string
	}{
		{name: "task accepted", kind: LifecycleEntityTask, raw: "accepted"},
		{name: "report active", kind: LifecycleEntityReport, raw: "active"},
		{name: "idea pending", kind: LifecycleEntityIdea, raw: "pending"},
		{name: "brainstorm cooking", kind: LifecycleEntityBrainstorm, raw: "cooking"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyImportLifecycleStatus(tc.kind, "id", tc.raw, "unknown")
			if got.Status != tc.raw {
				t.Fatalf("Status = %q, want raw %q", got.Status, tc.raw)
			}
			if !got.OutOfVocabulary {
				t.Fatal("OutOfVocabulary = false, want true")
			}
			if !strings.Contains(got.Warning, tc.raw) || !strings.Contains(got.Warning, tc.kind) {
				t.Fatalf("Warning = %q, want kind+raw", got.Warning)
			}
		})
	}
}

func TestApplyImportLifecycleStatusRecordsOOVWarning(t *testing.T) {
	report := emptyImportReport()
	m := markdownImporter{report: &report}
	got := m.applyImportLifecycleStatus(LifecycleEntityTask, "task-1", "accepted", "unknown")
	if got != "accepted" {
		t.Fatalf("status = %q, want accepted", got)
	}
	if len(report.Warnings) != 1 || !strings.Contains(report.Warnings[0], "accepted") || !strings.Contains(report.Warnings[0], "task-1") {
		t.Fatalf("warnings = %#v, want one accepted OOV warning", report.Warnings)
	}
	got = m.applyImportLifecycleStatus(LifecycleEntityIdea, "idea-1", "unknown", LifecycleStatusOpen)
	if got != LifecycleStatusOpen {
		t.Fatalf("explicit unknown idea = %q, want open", got)
	}
	if len(report.Warnings) != 1 {
		t.Fatalf("no-opinion must not append warning: %#v", report.Warnings)
	}
}

func TestImportMarkdownNormalizesTaskReportIdeaBrainstormStatuses(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	stateHome := t.TempDir()

	writeAgentsFile(t, root.Path(), "tasks/TASK-010-legacy.md", `---
id: TASK-010
title: Legacy Task
status: done
---
# Legacy Task
`)
	writeAgentsFile(t, root.Path(), "tasks/TASK-011-oov.md", `---
id: TASK-011
title: OOV Task
status: accepted
---
# OOV Task
`)
	writeAgentsFile(t, root.Path(), "tasks/TASK-012-unknown.md", `---
id: TASK-012
title: Unknown Task
status: unknown
---
# Unknown Task
`)
	writeAgentsFile(t, root.Path(), "reports/report-final.md", `---
id: report-final
title: Final Report
status: final
---
# Final Report
`)
	writeAgentsFile(t, root.Path(), "reports/report-bare.md", `---
id: report-bare
title: Bare Report
---
# Bare Report
`)
	writeAgentsFile(t, root.Path(), "reports/archive/report-archived.md", `---
id: report-archived
title: Archived Report
status: draft
---
# Archived Report
`)
	writeAgentsFile(t, root.Path(), "ideas/20260724-resolved.md", `---
id: idea-resolved
title: Resolved Idea
status: resolved
---
# Resolved Idea
`)
	writeAgentsFile(t, root.Path(), "ideas/20260724-unknown.md", `---
id: idea-unknown
title: Unknown Idea
status: unknown
---
# Unknown Idea
`)
	writeAgentsFile(t, root.Path(), "ideas/20260724-oov.md", `---
id: idea-oov
title: OOV Idea
status: pending
---
# OOV Idea
`)
	writeAgentsFile(t, root.Path(), "drafts/20260724-brainstorm-open.md", `---
id: brainstorm-open
title: Open Brainstorm
status: open
---
# Open Brainstorm
`)
	writeAgentsFile(t, root.Path(), "drafts/20260724-brainstorm-unknown.md", `---
id: brainstorm-unknown
title: Unknown Brainstorm
status: unknown
---
# Unknown Brainstorm
`)
	writeAgentsFile(t, root.Path(), "drafts/20260724-brainstorm-oov.md", `---
id: brainstorm-oov
title: OOV Brainstorm
status: cooking
---
# OOV Brainstorm
`)
	// Shaping draft must keep raw non-vocabulary status (excluded from normalization).
	writeAgentsFile(t, root.Path(), "drafts/20260724-shape.md", `---
id: shape-raw
title: Shape Draft
status: grilling
kind: shaping
---
# Shape Draft
`)

	result, err := ApplyMarkdownMigration(ctx, root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("ApplyMarkdownMigration() error = %v", err)
	}
	store, err := OpenStore(result.DatabasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()

	report, err := store.importMarkdown(ctx, root)
	if err != nil {
		t.Fatalf("second importMarkdown() error = %v", err)
	}
	// Existing-row OOV input keeps stored status and still warns.
	for _, needle := range []string{"accepted", "pending", "cooking"} {
		if !containsWarningNeedle(report.Warnings, needle) {
			t.Fatalf("second-import warnings %#v missing %s", report.Warnings, needle)
		}
	}

	assertEntityStatus(t, store, "tasks", result.ProjectID, "task", "TASK-010", "done")
	assertEntityStatus(t, store, "tasks", result.ProjectID, "task", "TASK-011", "accepted")
	assertEntityStatus(t, store, "tasks", result.ProjectID, "task", "TASK-012", "unknown")
	assertEntityStatus(t, store, "reports", result.ProjectID, "report", "report-final", "done")
	assertEntityStatus(t, store, "reports", result.ProjectID, "report", "report-bare", "unknown")
	assertEntityStatus(t, store, "reports", result.ProjectID, "report", "report-archived", "archived")
	assertEntityStatus(t, store, "ideas", result.ProjectID, "idea", "idea-resolved", "done")
	assertEntityStatus(t, store, "ideas", result.ProjectID, "idea", "idea-unknown", "open")
	assertEntityStatus(t, store, "ideas", result.ProjectID, "idea", "idea-oov", "pending")
	assertEntityStatus(t, store, "brainstorms", result.ProjectID, "brainstorm", "brainstorm-open", "open")
	assertEntityStatus(t, store, "brainstorms", result.ProjectID, "brainstorm", "brainstorm-unknown", "open")
	assertEntityStatus(t, store, "brainstorms", result.ProjectID, "brainstorm", "brainstorm-oov", "cooking")
	assertEntityStatus(t, store, "shaping_drafts", result.ProjectID, "shaping_draft", "shape-raw", "grilling")
}

func TestImportMarkdownFirstApplyRecordsOOVWarnings(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	stateHome := t.TempDir()
	writeAgentsFile(t, root.Path(), "tasks/TASK-020-oov.md", `---
id: TASK-020
title: OOV
status: accepted
---
# OOV
`)
	writeAgentsFile(t, root.Path(), "ideas/20260724-oov-only.md", `---
id: idea-oov-only
title: OOV
status: pending
---
# OOV
`)

	status, err := Initialize(ctx, root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	store, err := OpenStore(status.DatabasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()

	report, err := store.importMarkdown(ctx, root)
	if err != nil {
		t.Fatalf("importMarkdown() error = %v", err)
	}
	if !containsWarningNeedle(report.Warnings, "accepted") || !containsWarningNeedle(report.Warnings, "pending") {
		t.Fatalf("warnings = %#v, want accepted and pending OOV", report.Warnings)
	}
	if containsWarningNeedle(report.Warnings, "unknown") {
		t.Fatalf("warnings = %#v must not warn on no-opinion", report.Warnings)
	}
	assertEntityStatus(t, store, "tasks", status.ProjectID, "task", "TASK-020", "accepted")
	assertEntityStatus(t, store, "ideas", status.ProjectID, "idea", "idea-oov-only", "pending")
}

func assertEntityStatus(t *testing.T, store *Store, table string, projectID string, kind string, alias string, want string) {
	t.Helper()
	var got string
	err := store.db.QueryRowContext(
		context.Background(),
		`SELECT status FROM `+table+` WHERE project_id = ? AND id = ?`,
		projectID,
		stableMigrationID(kind, projectID, alias),
	).Scan(&got)
	if err != nil {
		t.Fatalf("read %s %s status: %v", table, alias, err)
	}
	if got != want {
		t.Fatalf("%s %s status = %q, want %q", table, alias, got, want)
	}
}

func containsWarningNeedle(warnings []string, needle string) bool {
	for _, warning := range warnings {
		if strings.Contains(warning, needle) {
			return true
		}
	}
	return false
}
