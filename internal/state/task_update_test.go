package state

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/levifig/loaf/internal/project"
)

func TestUpdateTaskStatusMutatesTaskAndRecordsEvent(t *testing.T) {
	repo := initGitRepo(t)
	root, err := project.ResolveRoot(repo)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	stateHome := t.TempDir()
	writeAgentsFile(t, root.Path(), "tasks/TASK-001-status.md", "# Status Task\n")
	writeAgentsFile(t, root.Path(), "TASKS.json", `{"tasks":{"TASK-001":{"title":"Status Task","status":"todo","priority":"P1"}}}`)
	if _, err := ApplyMarkdownMigration(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("ApplyMarkdownMigration() error = %v", err)
	}

	updated, err := UpdateTaskStatus(context.Background(), root, PathResolver{StateHome: stateHome}, "TASK-001", "in_progress")
	if err != nil {
		t.Fatalf("UpdateTaskStatus() error = %v", err)
	}
	if updated.Task.Alias != "TASK-001" || updated.Previous != "todo" || updated.Status != "in_progress" || updated.EventID == "" {
		t.Fatalf("updated = %#v, want TASK-001 todo -> in_progress with event", updated)
	}
	if updated.ContractVersion != StateJSONContractVersion {
		t.Fatalf("ContractVersion = %d, want %d", updated.ContractVersion, StateJSONContractVersion)
	}
	if updated.DatabaseScope != "global" {
		t.Fatalf("DatabaseScope = %q, want global", updated.DatabaseScope)
	}
	if updated.DatabasePath == "" {
		t.Fatal("DatabasePath is empty")
	}
	if updated.ProjectID == "" {
		t.Fatal("ProjectID is empty")
	}
	if updated.ProjectName != filepath.Base(root.Path()) {
		t.Fatalf("ProjectName = %q, want %q", updated.ProjectName, filepath.Base(root.Path()))
	}
	if updated.ProjectCurrentPath != root.Path() {
		t.Fatalf("ProjectCurrentPath = %q, want %q", updated.ProjectCurrentPath, root.Path())
	}

	tasks, err := ListTasks(context.Background(), root, PathResolver{StateHome: stateHome}, TaskListOptions{})
	if err != nil {
		t.Fatalf("ListTasks() error = %v", err)
	}
	if tasks.Tasks["TASK-001"].Status != "in_progress" {
		t.Fatalf("TASK-001 status = %q, want in_progress", tasks.Tasks["TASK-001"].Status)
	}

	trace, err := Trace(context.Background(), root, PathResolver{StateHome: stateHome}, "TASK-001")
	if err != nil {
		t.Fatalf("Trace() error = %v", err)
	}
	if trace.Entity.Status != "in_progress" {
		t.Fatalf("trace status = %q, want in_progress", trace.Entity.Status)
	}

	again, err := UpdateTaskStatus(context.Background(), root, PathResolver{StateHome: stateHome}, "TASK-001", "in_progress")
	if err != nil {
		t.Fatalf("idempotent UpdateTaskStatus() error = %v", err)
	}
	if again.EventID != "" {
		t.Fatalf("idempotent EventID = %q, want empty", again.EventID)
	}

	store, err := OpenStore(mustDatabasePath(t, root, stateHome))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()
	var events int
	if err := store.db.QueryRowContext(context.Background(), `
SELECT COUNT(*)
FROM events
WHERE project_id = ? AND entity_kind = 'task' AND event_type = 'status_changed' AND from_status = 'todo' AND to_status = 'in_progress'
`, projectIDForTest(t, store, root)).Scan(&events); err != nil {
		t.Fatalf("count task status events error = %v", err)
	}
	if events != 1 {
		t.Fatalf("events = %d, want one status_changed event after repeated update", events)
	}
}

func TestUpdateTaskStatusRejectsInvalidStatusAndNonTask(t *testing.T) {
	repo := initGitRepo(t)
	root, err := project.ResolveRoot(repo)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	stateHome := t.TempDir()
	writeAgentsFile(t, root.Path(), "specs/SPEC-001-status.md", "# Status Spec\n")
	writeAgentsFile(t, root.Path(), "tasks/TASK-001-status.md", "# Status Task\n")
	writeAgentsFile(t, root.Path(), "TASKS.json", `{"tasks":{"TASK-001":{"title":"Status Task","spec":"SPEC-001","status":"todo"}}}`)
	if _, err := ApplyMarkdownMigration(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("ApplyMarkdownMigration() error = %v", err)
	}

	if _, err := UpdateTaskStatus(context.Background(), root, PathResolver{StateHome: stateHome}, "TASK-001", "waiting"); err == nil {
		t.Fatal("UpdateTaskStatus(invalid status) error = nil, want error")
	}
	if _, err := UpdateTaskStatus(context.Background(), root, PathResolver{StateHome: stateHome}, "TASK-001", "archived"); err == nil {
		t.Fatal("UpdateTaskStatus(archived status) error = nil, want archive command to own archived transitions")
	}
	if _, err := UpdateTaskStatus(context.Background(), root, PathResolver{StateHome: stateHome}, "SPEC-001", "done"); err == nil {
		t.Fatal("UpdateTaskStatus(non-task) error = nil, want error")
	}
}

func TestUpdateTaskMetadataMutatesRelationships(t *testing.T) {
	repo := initGitRepo(t)
	root, err := project.ResolveRoot(repo)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	stateHome := t.TempDir()
	writeAgentsFile(t, root.Path(), "specs/SPEC-001-original.md", "# Original Spec\n")
	writeAgentsFile(t, root.Path(), "specs/SPEC-002-new.md", "# New Spec\n")
	writeAgentsFile(t, root.Path(), "tasks/TASK-001-update.md", "# Updated Task\n")
	writeAgentsFile(t, root.Path(), "tasks/TASK-002-dependency.md", "# Dependency Task\n")
	writeAgentsFile(t, root.Path(), "sessions/20260528-session.md", "# Session\n")
	writeAgentsFile(t, root.Path(), "TASKS.json", `{"tasks":{
  "TASK-001":{"title":"Updated Task","spec":"SPEC-001","status":"todo","priority":"P2"},
  "TASK-002":{"title":"Dependency Task","status":"todo","priority":"P3"}
}}`)
	if _, err := ApplyMarkdownMigration(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("ApplyMarkdownMigration() error = %v", err)
	}

	updated, err := UpdateTask(context.Background(), root, PathResolver{StateHome: stateHome}, TaskUpdateOptions{
		Ref:          "TASK-001",
		Priority:     "P0",
		SetPriority:  true,
		Spec:         "SPEC-002",
		SetSpec:      true,
		DependsOn:    []string{"TASK-002"},
		SetDependsOn: true,
		Session:      "20260528-session",
		SetSession:   true,
	})
	if err != nil {
		t.Fatalf("UpdateTask() error = %v", err)
	}
	if updated.Task.Alias != "TASK-001" || updated.Priority != "P0" || updated.Spec == nil || updated.Spec.Alias != "SPEC-002" || updated.Session == nil || updated.Session.Alias != "20260528-session" {
		t.Fatalf("updated = %#v, want priority/spec/session metadata", updated)
	}
	if len(updated.Depends) != 1 || updated.Depends[0].Alias != "TASK-002" {
		t.Fatalf("Depends = %#v, want TASK-002", updated.Depends)
	}

	show, err := ShowTask(context.Background(), root, PathResolver{StateHome: stateHome}, "TASK-001")
	if err != nil {
		t.Fatalf("ShowTask() error = %v", err)
	}
	if show.Task.Priority != "P0" || show.Task.Spec != "SPEC-002" || len(show.Task.DependsOn) != 1 || show.Task.DependsOn[0] != "TASK-002" || len(show.Task.Sessions) != 1 || show.Task.Sessions[0] != "20260528-session" {
		t.Fatalf("show = %#v, want updated metadata", show)
	}

	trace, err := Trace(context.Background(), root, PathResolver{StateHome: stateHome}, "TASK-001")
	if err != nil {
		t.Fatalf("Trace() error = %v", err)
	}
	if hasRelationship(trace.Relationships, "outbound", "implements", "spec", "SPEC-001") {
		t.Fatalf("trace relationships = %#v, still has old SPEC-001", trace.Relationships)
	}
	if !hasRelationship(trace.Relationships, "outbound", "implements", "spec", "SPEC-002") {
		t.Fatalf("trace relationships = %#v, want implements SPEC-002", trace.Relationships)
	}
	if !hasRelationship(trace.Relationships, "outbound", "blocked_by", "task", "TASK-002") {
		t.Fatalf("trace relationships = %#v, want blocked_by TASK-002", trace.Relationships)
	}
	if !hasRelationship(trace.Relationships, "outbound", "associated_with", "session", "20260528-session") {
		t.Fatalf("trace relationships = %#v, want associated_with session", trace.Relationships)
	}

	cleared, err := UpdateTask(context.Background(), root, PathResolver{StateHome: stateHome}, TaskUpdateOptions{
		Ref:          "TASK-001",
		Spec:         "none",
		SetSpec:      true,
		SetDependsOn: true,
		Session:      "none",
		SetSession:   true,
	})
	if err != nil {
		t.Fatalf("clearing UpdateTask() error = %v", err)
	}
	if cleared.Spec != nil || len(cleared.Depends) != 0 || cleared.Session != nil {
		t.Fatalf("cleared = %#v, want cleared spec, depends, and session", cleared)
	}
	show, err = ShowTask(context.Background(), root, PathResolver{StateHome: stateHome}, "TASK-001")
	if err != nil {
		t.Fatalf("ShowTask() after clear error = %v", err)
	}
	if show.Task.Spec != "" || len(show.Task.DependsOn) != 0 || len(show.Task.Sessions) != 0 {
		t.Fatalf("show after clear = %#v, want cleared metadata", show)
	}
}

func TestUpdateTaskMetadataRejectsInvalidRefs(t *testing.T) {
	repo := initGitRepo(t)
	root, err := project.ResolveRoot(repo)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	stateHome := t.TempDir()
	writeAgentsFile(t, root.Path(), "specs/SPEC-001-status.md", "# Status Spec\n")
	writeAgentsFile(t, root.Path(), "tasks/TASK-001-status.md", "# Status Task\n")
	writeAgentsFile(t, root.Path(), "sessions/20260528-session.md", "# Session\n")
	writeAgentsFile(t, root.Path(), "TASKS.json", `{"tasks":{"TASK-001":{"title":"Status Task","spec":"SPEC-001","status":"todo"}}}`)
	if _, err := ApplyMarkdownMigration(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("ApplyMarkdownMigration() error = %v", err)
	}

	cases := []struct {
		name    string
		options TaskUpdateOptions
	}{
		{name: "invalid priority", options: TaskUpdateOptions{Ref: "TASK-001", Priority: "P9", SetPriority: true}},
		{name: "missing spec", options: TaskUpdateOptions{Ref: "TASK-001", Spec: "SPEC-999", SetSpec: true}},
		{name: "wrong kind spec", options: TaskUpdateOptions{Ref: "TASK-001", Spec: "20260528-session", SetSpec: true}},
		{name: "missing dependency", options: TaskUpdateOptions{Ref: "TASK-001", DependsOn: []string{"TASK-999"}, SetDependsOn: true}},
		{name: "wrong kind dependency", options: TaskUpdateOptions{Ref: "TASK-001", DependsOn: []string{"SPEC-001"}, SetDependsOn: true}},
		{name: "wrong kind session", options: TaskUpdateOptions{Ref: "TASK-001", Session: "SPEC-001", SetSession: true}},
		{name: "empty update", options: TaskUpdateOptions{Ref: "TASK-001"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := UpdateTask(context.Background(), root, PathResolver{StateHome: stateHome}, tc.options); err == nil {
				t.Fatal("UpdateTask() error = nil, want validation error")
			}
		})
	}
}
