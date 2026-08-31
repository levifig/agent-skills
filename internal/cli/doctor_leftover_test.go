package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/levifig/loaf/internal/project"
	"github.com/levifig/loaf/internal/state"
)

func leftoverDoctorProject(t *testing.T) (string, string) {
	t.Helper()
	root := writeDoctorFixture(t, "9.8.7-test.1")
	writeDoctorAgents(t, root, doctorFence("9.8.7-test.1"))
	symlinkFile(t, "../AGENTS.md", filepath.Join(root, ".claude", "CLAUDE.md"))
	stateHome := t.TempDir()
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: root, StateHome: stateHome}).Run([]string{"state", "init"}); err != nil {
		t.Fatalf("state init error = %v", err)
	}
	return root, stateHome
}

func leftoverDoctorRoot(t *testing.T, workingDir string) project.Root {
	t.Helper()
	root, err := project.ResolveRoot(workingDir)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	return root
}

func TestLeftoverAbsorbCheckIsRegisteredAndReportOnly(t *testing.T) {
	for _, check := range doctorChecks() {
		if check.Name != "leftover-absorb" {
			continue
		}
		if check.Fix != nil || check.RepairID != "" || check.Repair != "" {
			t.Fatalf("leftover-absorb check = %#v, want no repair surface", check)
		}
		return
	}
	t.Fatal("doctorChecks() does not register leftover-absorb")
}

func TestRunnerDoctorSkipsLeftoverAbsorbWithoutSQLite(t *testing.T) {
	root := writeDoctorFixture(t, "9.8.7-test.1")
	writeDoctorAgents(t, root, doctorFence("9.8.7-test.1"))
	symlinkFile(t, "../AGENTS.md", filepath.Join(root, ".claude", "CLAUDE.md"))
	var stdout bytes.Buffer
	if err := (Runner{Stdout: &stdout, WorkingDir: root, StateHome: t.TempDir()}).Run([]string{"doctor", "--verbose"}); err != nil {
		t.Fatalf("doctor --verbose error = %v", err)
	}
	output := stripANSI(stdout.String())
	if !strings.Contains(output, "leftover-absorb") || !strings.Contains(output, "No initialized SQLite state") {
		t.Fatalf("doctor output = %q, want leftover-absorb skip for uninitialized SQLite", output)
	}
}

func TestRunnerDoctorWarnsLeftoverOpenWorkAndDoesNotAbsorbOnFix(t *testing.T) {
	workingDir, stateHome := leftoverDoctorProject(t)
	root := leftoverDoctorRoot(t, workingDir)
	resolver := state.PathResolver{StateHome: stateHome}
	if _, err := state.CreateTask(context.Background(), root, resolver, state.TaskCreateOptions{Title: "Open leftover"}); err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}

	var stdout bytes.Buffer
	err := (Runner{Stdout: &stdout, Stdin: strings.NewReader("y\n"), WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"doctor", "--fix"})
	if err != nil {
		t.Fatalf("doctor --fix error = %v\n%s", err, stdout.String())
	}
	output := stripANSI(stdout.String())
	if !strings.Contains(output, "leftover-absorb") || !strings.Contains(output, state.LeftoverAbsorbPreviewCommand) {
		t.Fatalf("doctor output = %q, want leftover-absorb warn naming absorb --all --dry-run", output)
	}
	if strings.Contains(output, "failed") {
		t.Fatalf("doctor output = %q, want leftover warn without failure", output)
	}

	shown, err := state.ShowTask(context.Background(), root, resolver, "TASK-001")
	if err != nil {
		t.Fatalf("ShowTask() error = %v", err)
	}
	if shown.Task.Status != state.LifecycleStatusTodo {
		t.Fatalf("task status = %q, want todo after doctor --fix", shown.Task.Status)
	}
	listed, err := state.ListIssues(context.Background(), root, resolver, state.IssueListOptions{})
	if err != nil {
		t.Fatalf("ListIssues() error = %v", err)
	}
	if len(listed.Issues) != 0 {
		t.Fatalf("issues = %#v, want none after doctor --fix", listed.Issues)
	}
}

func TestRunnerDoctorWarnsProjectableHistory(t *testing.T) {
	workingDir, stateHome := leftoverDoctorProject(t)
	root := leftoverDoctorRoot(t, workingDir)
	resolver := state.PathResolver{StateHome: stateHome}
	created, err := state.CreateTask(context.Background(), root, resolver, state.TaskCreateOptions{Title: "Done leftover"})
	if err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}
	if _, err := state.UpdateTaskStatus(context.Background(), root, resolver, created.Task.Alias, state.LifecycleStatusDone); err != nil {
		t.Fatalf("UpdateTaskStatus() error = %v", err)
	}

	var stdout bytes.Buffer
	if err := (Runner{Stdout: &stdout, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"doctor"}); err != nil {
		t.Fatalf("doctor error = %v\n%s", err, stdout.String())
	}
	output := stripANSI(stdout.String())
	if !strings.Contains(output, "leftover-absorb") || !strings.Contains(output, state.LeftoverAbsorbHistoryPreviewCommand) {
		t.Fatalf("doctor output = %q, want leftover history preview", output)
	}
}

func TestRunnerDoctorPassesWhenHistoryIsFrozen(t *testing.T) {
	workingDir, stateHome := leftoverDoctorProject(t)
	root := leftoverDoctorRoot(t, workingDir)
	resolver := state.PathResolver{StateHome: stateHome}
	created, err := state.CreateTask(context.Background(), root, resolver, state.TaskCreateOptions{Title: "Done leftover"})
	if err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}
	if _, err := state.UpdateTaskStatus(context.Background(), root, resolver, created.Task.Alias, state.LifecycleStatusDone); err != nil {
		t.Fatalf("UpdateTaskStatus() error = %v", err)
	}
	if _, err := state.CreateIssue(context.Background(), root, resolver, state.IssueCreateOptions{Title: "Hand-made"}); err != nil {
		t.Fatalf("CreateIssue() error = %v", err)
	}

	var stdout bytes.Buffer
	if err := (Runner{Stdout: &stdout, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"doctor", "--verbose"}); err != nil {
		t.Fatalf("doctor error = %v\n%s", err, stdout.String())
	}
	output := stripANSI(stdout.String())
	if !strings.Contains(output, "history stays frozen until 0.5.0 (LOAF-47) (1 row(s), 1 independently created issue(s))") {
		t.Fatalf("doctor output = %q, want frozen-history pass with row count", output)
	}
	if strings.Contains(output, state.LeftoverAbsorbHistoryPreviewCommand) {
		t.Fatalf("doctor output = %q, want no history absorb nag", output)
	}
}

func TestRunnerStateDoctorNamesLeftoverOpenAbsorb(t *testing.T) {
	workingDir, stateHome := issueCLIFixture(t)
	root := leftoverDoctorRoot(t, workingDir)
	if _, err := state.CreateTask(context.Background(), root, state.PathResolver{StateHome: stateHome}, state.TaskCreateOptions{Title: "Open leftover"}); err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}

	var stdout bytes.Buffer
	if err := (Runner{Stdout: &stdout, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"state", "doctor", "--json"}); err != nil {
		t.Fatalf("state doctor --json error = %v\n%s", err, stdout.String())
	}
	status := decodeStateStatus(t, stdout.Bytes())
	if !hasDiagnostic(status.Diagnostics, state.LeftoverOpenWorkCode) {
		t.Fatalf("diagnostics = %#v, want leftover-open-work", status.Diagnostics)
	}
	action := findStateRepairAction(t, status.RepairPlan, "preview-leftover-absorb")
	if action.Safe || action.Applied || action.Command != state.LeftoverAbsorbPreviewCommand {
		t.Fatalf("repair action = %#v, want manual leftover absorb preview", action)
	}
}

func TestRunnerStateStatusOmitsLeftoverAbsorb(t *testing.T) {
	workingDir, stateHome := issueCLIFixture(t)
	root := leftoverDoctorRoot(t, workingDir)
	if _, err := state.CreateTask(context.Background(), root, state.PathResolver{StateHome: stateHome}, state.TaskCreateOptions{Title: "Open leftover"}); err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}

	var stdout bytes.Buffer
	if err := (Runner{Stdout: &stdout, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"state", "status", "--json"}); err != nil {
		t.Fatalf("state status --json error = %v\n%s", err, stdout.String())
	}
	status := decodeStateStatus(t, stdout.Bytes())
	if hasDiagnostic(status.Diagnostics, state.LeftoverOpenWorkCode) {
		t.Fatalf("state status diagnostics = %#v, want leftover scan off the hot path", status.Diagnostics)
	}
}

func TestRunnerStateDoctorFixDoesNotAbsorb(t *testing.T) {
	workingDir, stateHome := issueCLIFixture(t)
	root := leftoverDoctorRoot(t, workingDir)
	resolver := state.PathResolver{StateHome: stateHome}
	if _, err := state.CreateTask(context.Background(), root, resolver, state.TaskCreateOptions{Title: "Open leftover"}); err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}

	var stdout bytes.Buffer
	if err := (Runner{Stdout: &stdout, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"state", "doctor", "--fix"}); err != nil {
		t.Fatalf("state doctor --fix error = %v\n%s", err, stdout.String())
	}
	shown, err := state.ShowTask(context.Background(), root, resolver, "TASK-001")
	if err != nil {
		t.Fatalf("ShowTask() error = %v", err)
	}
	if shown.Task.Status != state.LifecycleStatusTodo {
		t.Fatalf("task status = %q, want todo after state doctor --fix", shown.Task.Status)
	}
	listed, err := state.ListIssues(context.Background(), root, resolver, state.IssueListOptions{})
	if err != nil {
		t.Fatalf("ListIssues() error = %v", err)
	}
	if len(listed.Issues) != 0 {
		t.Fatalf("issues = %#v, want none after state doctor --fix", listed.Issues)
	}
}

func TestRunnerStateDoctorNamesFrozenHistoryWithoutRepair(t *testing.T) {
	workingDir, stateHome := issueCLIFixture(t)
	root := leftoverDoctorRoot(t, workingDir)
	resolver := state.PathResolver{StateHome: stateHome}
	created, err := state.CreateTask(context.Background(), root, resolver, state.TaskCreateOptions{Title: "Done leftover"})
	if err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}
	if _, err := state.UpdateTaskStatus(context.Background(), root, resolver, created.Task.Alias, state.LifecycleStatusDone); err != nil {
		t.Fatalf("UpdateTaskStatus() error = %v", err)
	}
	if _, err := state.CreateIssue(context.Background(), root, resolver, state.IssueCreateOptions{Title: "Hand-made"}); err != nil {
		t.Fatalf("CreateIssue() error = %v", err)
	}

	var stdout bytes.Buffer
	if err := (Runner{Stdout: &stdout, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"state", "doctor", "--json"}); err != nil {
		t.Fatalf("state doctor --json error = %v\n%s", err, stdout.String())
	}
	status := decodeStateStatus(t, stdout.Bytes())
	if !hasDiagnostic(status.Diagnostics, state.LeftoverHistoryFrozenCode) {
		t.Fatalf("diagnostics = %#v, want leftover-history-frozen", status.Diagnostics)
	}
	if hasDiagnostic(status.Diagnostics, state.LeftoverHistoryWorkCode) {
		t.Fatalf("diagnostics = %#v, want no leftover-history-work repair nag", status.Diagnostics)
	}
	for _, action := range status.RepairPlan {
		if action.Code == "preview-leftover-absorb-history" {
			t.Fatalf("frozen history grew a repair action: %#v", action)
		}
	}
}

func namedLeftoverDoctorProject(t *testing.T, name string) (string, string) {
	t.Helper()
	parent := writeDoctorFixture(t, "9.8.7-test.1")
	root := filepath.Join(parent, name)
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	writeDoctorAgents(t, root, doctorFence("9.8.7-test.1"))
	symlinkFile(t, "../AGENTS.md", filepath.Join(root, ".claude", "CLAUDE.md"))
	stateHome := t.TempDir()
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: root, StateHome: stateHome}).Run([]string{"state", "init"}); err != nil {
		t.Fatalf("state init error = %v", err)
	}
	return root, stateHome
}

func TestRunnerStateDoctorNamesIssuePrefixLeak(t *testing.T) {
	workingDir, stateHome := namedIssueCLIFixture(t, "vcam")
	root := leftoverDoctorRoot(t, workingDir)
	resolver := state.PathResolver{StateHome: stateHome}
	if _, err := state.SetIssueIdentity(context.Background(), root, resolver, state.IssueIdentityOptions{Prefix: "LOAF"}); err != nil {
		t.Fatalf("SetIssueIdentity() error = %v", err)
	}
	if _, err := state.CreateIssue(context.Background(), root, resolver, state.IssueCreateOptions{Title: "Leaked"}); err != nil {
		t.Fatalf("CreateIssue() error = %v", err)
	}

	var stdout bytes.Buffer
	if err := (Runner{Stdout: &stdout, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"state", "doctor", "--json"}); err != nil {
		t.Fatalf("state doctor --json error = %v\n%s", err, stdout.String())
	}
	status := decodeStateStatus(t, stdout.Bytes())
	if !hasDiagnostic(status.Diagnostics, state.IssuePrefixLeakCode) {
		t.Fatalf("diagnostics = %#v, want issue-prefix-default-leak", status.Diagnostics)
	}
	action := findStateRepairAction(t, status.RepairPlan, "align-issue-prefix")
	if action.Safe || action.Applied || action.Command != state.IssuePrefixAlignCommand {
		t.Fatalf("repair action = %#v, want manual prefix align preview", action)
	}
}

func TestRunnerStateDoctorNamesIssuePrefixConfigMissing(t *testing.T) {
	workingDir, stateHome := namedIssueCLIFixture(t, "vcam")
	root := leftoverDoctorRoot(t, workingDir)
	resolver := state.PathResolver{StateHome: stateHome}
	if _, err := state.CreateIssue(context.Background(), root, resolver, state.IssueCreateOptions{Title: "Derived"}); err != nil {
		t.Fatalf("CreateIssue() error = %v", err)
	}

	var stdout bytes.Buffer
	if err := (Runner{Stdout: &stdout, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"state", "doctor", "--json"}); err != nil {
		t.Fatalf("state doctor --json error = %v\n%s", err, stdout.String())
	}
	status := decodeStateStatus(t, stdout.Bytes())
	if !hasDiagnostic(status.Diagnostics, state.IssuePrefixConfigMissingCode) {
		t.Fatalf("diagnostics = %#v, want issue-prefix-config-missing", status.Diagnostics)
	}
	action := findStateRepairAction(t, status.RepairPlan, "persist-issue-prefix")
	if action.Safe || action.Command != state.IssuePrefixPersistCommand("VCAM") {
		t.Fatalf("repair action = %#v, want persist VCAM", action)
	}
}
