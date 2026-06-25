package state

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderDurableArtifactWritesSpecToBranchCache(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	cacheHome := t.TempDir()
	writeAgentsFile(t, root.Path(), "specs/SPEC-001-render.md", `---
id: SPEC-001
title: Render Spec
status: implementing
---
# Render Spec

Spec body.
`)
	if _, err := ApplyMarkdownMigration(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("ApplyMarkdownMigration() error = %v", err)
	}

	result, err := RenderDurableArtifact(context.Background(), root, PathResolver{StateHome: stateHome, CacheHome: cacheHome}, DurableRenderOptions{
		Kind:   "spec",
		Ref:    "SPEC-001",
		Branch: "feature/render",
	})
	if err != nil {
		t.Fatalf("RenderDurableArtifact() error = %v", err)
	}
	cacheRoot := filepath.Join(cacheHome, "loaf", "renders")
	if !strings.HasPrefix(result.Path, cacheRoot+string(filepath.Separator)) || !strings.Contains(result.Path, string(filepath.Separator)+"feature-render"+string(filepath.Separator)) {
		t.Fatalf("Path = %q, want project/branch namespaced cache under %q", result.Path, cacheRoot)
	}
	content, err := os.ReadFile(result.Path)
	if err != nil {
		t.Fatalf("read render path error = %v", err)
	}
	text := string(content)
	if !strings.Contains(text, "# Render Spec") || !strings.Contains(text, "<!-- loaf:render kind=spec contract=durable-doc-v1 -->") {
		t.Fatalf("render content = %q, want spec body and stamp", text)
	}
	if strings.Contains(result.Path, root.Path()) {
		t.Fatalf("Path = %q, want out-of-tree cache", result.Path)
	}
	if _, err := os.Stat(filepath.Join(root.Path(), "spec-SPEC-001.md")); !os.IsNotExist(err) {
		t.Fatalf("unexpected in-tree render stat = %v", err)
	}
}

func TestRenderDurableArtifactWritesReportToCache(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	cacheHome := t.TempDir()
	if _, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	created, err := CreateReport(context.Background(), root, PathResolver{StateHome: stateHome}, ReportCreateOptions{
		Slug:    "cache-render",
		Kind:    "audit",
		Source:  "test",
		Body:    "# Cache Render\n\nReport body.",
		SetBody: true,
	})
	if err != nil {
		t.Fatalf("CreateReport() error = %v", err)
	}

	result, err := RenderDurableArtifact(context.Background(), root, PathResolver{StateHome: stateHome, CacheHome: cacheHome}, DurableRenderOptions{
		Kind:   "report",
		Ref:    created.Report.Alias,
		Branch: "feature/report",
	})
	if err != nil {
		t.Fatalf("RenderDurableArtifact(report) error = %v", err)
	}
	content, err := os.ReadFile(result.Path)
	if err != nil {
		t.Fatalf("read report render error = %v", err)
	}
	text := string(content)
	if result.Kind != "report" || result.Ref != created.Report.Alias || result.Contract != DurableRenderContract || result.ContentHash != artifactBodyHash(text) {
		t.Fatalf("result = %#v, want report render metadata", result)
	}
	if !strings.Contains(text, "report_kind: audit") || !strings.Contains(text, "<!-- loaf:render kind=report contract=durable-doc-v1 -->") {
		t.Fatalf("report render content = %q, want report kind and stamp", text)
	}
}

func TestFinalizeDurableArtifactWritesSpecToTrackedSource(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	specRel := filepath.ToSlash(filepath.Join(".agents", "specs", "SPEC-001-finalize.md"))
	writeAgentsFile(t, root.Path(), "specs/SPEC-001-finalize.md", `---
id: SPEC-001
title: Finalize Spec
status: implementing
---
# Finalize Spec

Original body.
`)
	if _, err := ApplyMarkdownMigration(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("ApplyMarkdownMigration() error = %v", err)
	}

	result, err := FinalizeDurableArtifact(context.Background(), root, PathResolver{StateHome: stateHome}, DurableFinalizeOptions{Kind: "spec", Ref: "SPEC-001"})
	if err != nil {
		t.Fatalf("FinalizeDurableArtifact(spec) error = %v", err)
	}
	if result.RelativePath != specRel {
		t.Fatalf("RelativePath = %q, want %q", result.RelativePath, specRel)
	}
	content, err := os.ReadFile(filepath.Join(root.Path(), filepath.FromSlash(specRel)))
	if err != nil {
		t.Fatalf("read finalized spec error = %v", err)
	}
	text := string(content)
	if !strings.Contains(text, "<!-- loaf:render kind=spec contract=durable-doc-v1 -->") || !strings.Contains(text, "# Finalize Spec") {
		t.Fatalf("finalized spec = %q, want body and stamp", text)
	}
	if strings.Contains(text, "created_at") || strings.Contains(text, "updated_at") {
		t.Fatalf("finalized spec contains volatile metadata:\n%s", text)
	}
}

func TestFinalizeDurableArtifactWritesReportFallbackPath(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	created, err := CreateReport(context.Background(), root, PathResolver{StateHome: stateHome}, ReportCreateOptions{
		Slug:    "finalize-render",
		Kind:    "audit",
		Source:  "test",
		Body:    "# Finalize Render\n\nReport body.",
		SetBody: true,
	})
	if err != nil {
		t.Fatalf("CreateReport() error = %v", err)
	}

	result, err := FinalizeDurableArtifact(context.Background(), root, PathResolver{StateHome: stateHome}, DurableFinalizeOptions{Kind: "report", Ref: created.Report.Alias})
	if err != nil {
		t.Fatalf("FinalizeDurableArtifact(report) error = %v", err)
	}
	wantRel := filepath.ToSlash(filepath.Join(".agents", "reports", created.Report.Alias+".md"))
	if result.RelativePath != wantRel {
		t.Fatalf("RelativePath = %q, want %q", result.RelativePath, wantRel)
	}
	content, err := os.ReadFile(filepath.Join(root.Path(), filepath.FromSlash(wantRel)))
	if err != nil {
		t.Fatalf("read finalized report error = %v", err)
	}
	text := string(content)
	if !strings.Contains(text, "report_kind: audit") || !strings.Contains(text, "<!-- loaf:render kind=report contract=durable-doc-v1 -->") {
		t.Fatalf("finalized report = %q, want report kind and stamp", text)
	}
}
