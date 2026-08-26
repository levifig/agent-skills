package state

import (
	"context"
	"strings"
	"testing"
)

func TestReportsAreFileBackedAfterDocumentLayerDemotion(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	_, err := CreateReport(context.Background(), root, PathResolver{StateHome: stateHome}, ReportCreateOptions{Slug: "demo", Kind: "audit"})
	if err == nil || !strings.Contains(err.Error(), "file-backed") {
		t.Fatalf("CreateReport() error = %v, want file-backed refusal", err)
	}
}
