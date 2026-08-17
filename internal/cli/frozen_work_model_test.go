package cli

import (
	"bytes"
	"strings"
	"testing"
)

func assertFrozenWorkModel(t *testing.T, err error, output string) {
	t.Helper()
	if err == nil {
		t.Fatal("error = nil, want frozen work-model refusal")
	}
	msg := err.Error() + output
	if !strings.Contains(msg, "frozen pending migration") || !strings.Contains(msg, "loaf issue") || !strings.Contains(msg, "LOAF-47") || !strings.Contains(msg, "0.5.0") {
		t.Fatalf("error = %v\n%s, want freeze redirect naming loaf issue, 0.5.0, and LOAF-47", err, output)
	}
}

func runFrozenTaskWrite(t *testing.T, workingDir, stateHome string, args ...string) {
	t.Helper()
	var stdout bytes.Buffer
	err := Runner{Stdout: &stdout, WorkingDir: workingDir, StateHome: stateHome}.Run(args)
	assertFrozenWorkModel(t, err, stdout.String())
}

func TestFrozenWorkModelTaskWriteVerbsRefuseAndReadsSucceed(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"state", "init"}); err != nil {
		t.Fatalf("state init error = %v", err)
	}

	for _, args := range [][]string{
		{"task", "create", "--title", "Frozen"},
		{"task", "update", "TASK-001", "--status", "done"},
		{"task", "archive", "TASK-001"},
	} {
		var stdout bytes.Buffer
		err := Runner{Stdout: &stdout, WorkingDir: workingDir, StateHome: stateHome}.Run(args)
		if err == nil {
			t.Fatalf("%v error = nil, want freeze", args)
		}
		msg := err.Error() + stdout.String()
		if !strings.Contains(msg, "frozen pending migration") || !strings.Contains(msg, "loaf issue") || !strings.Contains(msg, "LOAF-47") || !strings.Contains(msg, "0.5.0") {
			t.Fatalf("%v error = %v\n%s, want freeze redirect", args, err, stdout.String())
		}
	}

	for _, args := range [][]string{
		{"task", "list"},
		{"task", "status"},
		{"task", "refresh"},
		{"task", "sync"},
	} {
		var stdout bytes.Buffer
		if err := (Runner{Stdout: &stdout, WorkingDir: workingDir, StateHome: stateHome}).Run(args); err != nil {
			t.Fatalf("%v error = %v\n%s", args, err, stdout.String())
		}
	}

	var showOut bytes.Buffer
	showErr := Runner{Stdout: &showOut, WorkingDir: workingDir, StateHome: stateHome}.Run([]string{"task", "show", "TASK-001"})
	if showErr == nil {
		t.Fatal("task show TASK-001 error = nil, want missing-task error after freeze (not a write)")
	}
	if strings.Contains(showErr.Error(), "frozen pending migration") {
		t.Fatalf("task show was frozen: %v", showErr)
	}
}

func TestFrozenWorkModelIntentWriteVerbsRefuseAndReadsSucceed(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"state", "init"}); err != nil {
		t.Fatalf("state init error = %v", err)
	}

	for _, args := range [][]string{
		{"intent", "create", "--title", "Frozen", "--body", "body"},
		{"intent", "defer", "INT-1", "--why", "why", "--boundary", "boundary", "--trigger", "later", "--operation-id", "op-1"},
		{"intent", "resume", "INT-1", "--reason", "now"},
		{"intent", "resolve", "INT-1", "--reason", "done"},
	} {
		var stdout bytes.Buffer
		err := Runner{Stdout: &stdout, WorkingDir: workingDir, StateHome: stateHome}.Run(args)
		if err == nil {
			t.Fatalf("%v error = nil, want freeze", args)
		}
		msg := err.Error() + stdout.String()
		if !strings.Contains(msg, "frozen pending migration") || !strings.Contains(msg, "loaf issue") || !strings.Contains(msg, "LOAF-47") || !strings.Contains(msg, "0.5.0") {
			t.Fatalf("%v error = %v\n%s, want freeze redirect", args, err, stdout.String())
		}
	}

	var listOut bytes.Buffer
	if err := (Runner{Stdout: &listOut, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"intent", "list"}); err != nil {
		t.Fatalf("intent list error = %v\n%s", err, listOut.String())
	}

	var showOut bytes.Buffer
	showErr := Runner{Stdout: &showOut, WorkingDir: workingDir, StateHome: stateHome}.Run([]string{"intent", "show", "INT-1"})
	if showErr == nil {
		t.Fatal("intent show INT-1 error = nil, want missing-intent error after freeze (not a write)")
	}
	if strings.Contains(showErr.Error(), "frozen pending migration") {
		t.Fatalf("intent show was frozen: %v", showErr)
	}
}

func TestFrozenWorkModelHelpStatesDeprecation(t *testing.T) {
	var taskOut bytes.Buffer
	if err := (Runner{Stdout: &taskOut}).Run([]string{"task", "--help"}); err != nil {
		t.Fatalf("task --help error = %v", err)
	}
	if !strings.Contains(taskOut.String(), "frozen pending migration") || !strings.Contains(taskOut.String(), "LOAF-47") || !strings.Contains(taskOut.String(), "0.5.0") {
		t.Fatalf("task help missing deprecation:\n%s", taskOut.String())
	}

	var intentOut bytes.Buffer
	if err := (Runner{Stdout: &intentOut}).Run([]string{"intent", "--help"}); err != nil {
		t.Fatalf("intent --help error = %v", err)
	}
	if !strings.Contains(intentOut.String(), "frozen pending migration") || !strings.Contains(intentOut.String(), "LOAF-47") || !strings.Contains(intentOut.String(), "0.5.0") {
		t.Fatalf("intent help missing deprecation:\n%s", intentOut.String())
	}
}
