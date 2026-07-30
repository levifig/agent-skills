package cli

import (
	"errors"
	"os/exec"
	"path/filepath"
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

func TestChangeReceiptBlockMessagesNameFolderCauseRemedy(t *testing.T) {
	folder := filepath.Join("docs", "changes", "20260727-demo")
	cases := []struct {
		name    string
		verdict changeReceiptVerdict
		want    []string
		forbid  []string
	}{
		{
			name:    "drift",
			verdict: changeReceiptVerdict{Reason: changeReceiptContentDrift, DriftedSections: []string{"internal", "content"}},
			want:    []string{`change "demo"`, "1.0.0", "content changed under `internal`, `content`", "Run: loaf change verify", folder, "commit the receipt"},
			forbid:  []string{"exit status", "cannot inspect", "invalid", "corrupt", "later non-receipt"},
		},
		{
			name:    "criteria",
			verdict: changeReceiptVerdict{Reason: changeReceiptCriteriaMismatch},
			want:    []string{"criteria changed (receipt expired)", "Run: loaf change verify", folder},
			forbid:  []string{"exit status", "invalid", "corrupt"},
		},
		{
			name:    "schema",
			verdict: changeReceiptVerdict{Reason: changeReceiptUnsupportedSchema, SchemaVersion: 1},
			want:    []string{"unsupported receipt schema_version 1", "Run: loaf change verify", folder},
			forbid:  []string{"invalid", "corrupt", "exit status"},
		},
		{
			name:    "failing",
			verdict: changeReceiptVerdict{Reason: changeReceiptFailingResults, FailedIDs: []string{"V1", "V3"}},
			want:    []string{"receipt records failing criteria (V1, V3)", "Fix the failing criteria, then run: loaf change verify", folder, "and commit the receipt"},
			forbid:  []string{"exit status", "cannot inspect", "invalid", "corrupt"},
		},
		{
			name:    "boundary",
			verdict: changeReceiptVerdict{Reason: changeReceiptBoundaryChanged},
			want:    []string{"evidence boundary changed since verification (receipt expired)", "Run: loaf change verify", folder},
			forbid:  []string{"exit status", "invalid", "corrupt", "cannot inspect"},
		},
		{
			name:    "evidence-unavailable",
			verdict: changeReceiptVerdict{Reason: changeReceiptEvidenceUnavailable},
			want:    []string{"could not read evidence at HEAD (git error)", "Verification cannot proceed until git reads succeed", "git fsck", "re-clone"},
			forbid:  []string{"exit status", "cannot inspect", "Run: loaf change verify"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			msg := formatChangeReceiptBlock("demo", "1.0.0", tc.verdict, folder)
			for _, w := range tc.want {
				if !strings.Contains(msg, w) {
					t.Fatalf("msg=%q missing %q", msg, w)
				}
			}
			lower := strings.ToLower(msg)
			for _, f := range tc.forbid {
				if strings.Contains(lower, strings.ToLower(f)) {
					t.Fatalf("msg=%q must not contain %q", msg, f)
				}
			}
		})
	}
}
