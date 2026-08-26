package state

import (
	"context"
	"strings"
	"testing"
)

func TestListReportsRefusesSQLiteAuthority(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	_, err := ListReports(context.Background(), root, PathResolver{StateHome: stateHome}, ReportListOptions{})
	if err == nil || !strings.Contains(err.Error(), "file-backed") {
		t.Fatalf("ListReports() error = %v, want file-backed refusal", err)
	}
}
