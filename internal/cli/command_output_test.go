package cli

import (
	"errors"
	"os/exec"
	"strings"
	"testing"
)

func TestCommandOutputCapturesStderr(t *testing.T) {
	dir := t.TempDir()
	_, err := commandOutput(dir, "git", "rev-parse", "not-a-ref")
	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "exit status") {
		t.Fatalf("want wrapped exit status, got %q", msg)
	}
	// git writes the cause to stderr — must appear in the returned error.
	if !strings.Contains(strings.ToLower(msg), "unknown revision") && !strings.Contains(strings.ToLower(msg), "bad revision") && !strings.Contains(msg, "fatal:") {
		t.Fatalf("stderr cause missing from error: %q", msg)
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("want errors.As ExitError, got %T %v", err, err)
	}
}
