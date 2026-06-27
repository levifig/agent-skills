package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/levifig/loaf/internal/state"
)

func migrateSpecFixture(t *testing.T, workingDir string, stateHome string) {
	t.Helper()
	writeCLIAgentsFile(t, workingDir, "specs/SPEC-001-target.md", `---
id: SPEC-001
title: Target Spec
status: complete
---
# Target Spec

Body content.
`)
	writeCLIAgentsFile(t, workingDir, "tasks/TASK-001-task.md", `---
id: TASK-001
title: Linked Task
status: todo
priority: P1
spec: SPEC-001
---
# Linked Task
`)
	writeCLIAgentsFile(t, workingDir, "TASKS.json", `{"tasks":{"TASK-001":{"title":"Linked Task","status":"todo","priority":"P1","spec":"SPEC-001"}}}`)
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"state", "migrate", "markdown", "--apply"}); err != nil {
		t.Fatalf("state migrate markdown --apply error = %v", err)
	}
}

func TestRunnerSpecDeleteRemovesSpecAndDependents(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	migrateSpecFixture(t, workingDir, stateHome)

	var jsonOut bytes.Buffer
	err := Runner{Stdout: &jsonOut, WorkingDir: workingDir, StateHome: stateHome}.Run([]string{"spec", "delete", "SPEC-001", "--yes", "--json"})
	if err != nil {
		t.Fatalf("spec delete --yes --json error = %v", err)
	}
	var result state.SpecDeleteResult
	if err := json.Unmarshal(jsonOut.Bytes(), &result); err != nil {
		t.Fatalf("json.Unmarshal(%q) error = %v", jsonOut.String(), err)
	}
	removed := map[string]int{}
	for _, count := range result.Removed {
		removed[count.Table] = count.Rows
	}
	if removed["specs"] != 1 || removed["aliases"] < 1 || removed["artifact_bodies"] < 1 {
		t.Fatalf("removed = %#v, want specs=1 and dependent rows removed", removed)
	}

	// The spec is gone.
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"spec", "show", "SPEC-001"}); err == nil {
		t.Fatal("spec show after delete = nil error, want not-found failure")
	}
	// The linked task survives.
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"task", "show", "TASK-001"}); err != nil {
		t.Fatalf("task show after spec delete error = %v, want task preserved", err)
	}
}

func TestRunnerSpecDeleteRequiresYes(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	migrateSpecFixture(t, workingDir, stateHome)

	var out bytes.Buffer
	err := Runner{Stdout: &out, WorkingDir: workingDir, StateHome: stateHome}.Run([]string{"spec", "delete", "SPEC-001"})
	if err == nil {
		t.Fatal("spec delete without --yes = nil error, want refusal")
	}
	if !strings.Contains(err.Error(), "confirmation-required") {
		t.Fatalf("error = %q, want confirmation-required message", err.Error())
	}
	// Nothing was deleted.
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"spec", "show", "SPEC-001"}); err != nil {
		t.Fatalf("spec show after refused delete error = %v, want spec intact", err)
	}
}

func TestRunnerProjectDeleteCascades(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	migrateSpecFixture(t, workingDir, stateHome)

	projectID := cliProjectID(t, workingDir, stateHome)

	var jsonOut bytes.Buffer
	err := Runner{Stdout: &jsonOut, WorkingDir: workingDir, StateHome: stateHome}.Run([]string{"project", "delete", projectID, "--yes", "--json"})
	if err != nil {
		t.Fatalf("project delete --yes --json error = %v", err)
	}
	var result state.ProjectDeleteResult
	if err := json.Unmarshal(jsonOut.Bytes(), &result); err != nil {
		t.Fatalf("json.Unmarshal(%q) error = %v", jsonOut.String(), err)
	}
	removed := map[string]int{}
	for _, count := range result.Removed {
		removed[count.Table] = count.Rows
	}
	if removed["projects"] != 1 || removed["specs"] != 1 || removed["tasks"] != 1 {
		t.Fatalf("removed = %#v, want project, spec, and task rows removed", removed)
	}

	var listOut bytes.Buffer
	if err := (Runner{Stdout: &listOut, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"project", "list", "--json"}); err != nil {
		t.Fatalf("project list --json error = %v", err)
	}
	var list state.ProjectList
	if err := json.Unmarshal(listOut.Bytes(), &list); err != nil {
		t.Fatalf("json.Unmarshal(%q) error = %v", listOut.String(), err)
	}
	for _, p := range list.Projects {
		if p.ID == projectID {
			t.Fatalf("deleted project %s still listed", projectID)
		}
	}
}

func TestRunnerProjectDeleteRequiresYes(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	migrateSpecFixture(t, workingDir, stateHome)
	projectID := cliProjectID(t, workingDir, stateHome)

	err := Runner{Stdout: &bytes.Buffer{}, WorkingDir: workingDir, StateHome: stateHome}.Run([]string{"project", "delete", projectID})
	if err == nil {
		t.Fatal("project delete without --yes = nil error, want refusal")
	}
	if !strings.Contains(err.Error(), "confirmation-required") {
		t.Fatalf("error = %q, want confirmation-required message", err.Error())
	}
	// Project still present.
	var listOut bytes.Buffer
	if err := (Runner{Stdout: &listOut, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"project", "list", "--json"}); err != nil {
		t.Fatalf("project list --json error = %v", err)
	}
	if !strings.Contains(listOut.String(), projectID) {
		t.Fatalf("project %s missing after refused delete: %q", projectID, listOut.String())
	}
}

func cliProjectID(t *testing.T, workingDir string, stateHome string) string {
	t.Helper()
	var out bytes.Buffer
	if err := (Runner{Stdout: &out, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"project", "show", "--json"}); err != nil {
		t.Fatalf("project show --json error = %v", err)
	}
	var identity state.ProjectIdentity
	if err := json.Unmarshal(out.Bytes(), &identity); err != nil {
		t.Fatalf("json.Unmarshal(%q) error = %v", out.String(), err)
	}
	if identity.ID == "" {
		t.Fatalf("project id empty in %q", out.String())
	}
	return identity.ID
}
