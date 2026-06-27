package state

import (
	"bytes"
	"context"
	"os"
	"testing"
)

// TestRenderDurableArtifactByteIdenticalAcrossXDGDataHomes guards the SPEC-044
// reproducibility guarantee: a durable render's bytes depend only on the source
// artifact, never on where the operational state database happens to live. Two
// distinct $XDG_DATA_HOME values resolve to two separate SQLite databases, yet
// rendering the same spec through each must produce byte-identical output so CI
// can re-render committed files against a fresh database in any location.
func TestRenderDurableArtifactByteIdenticalAcrossXDGDataHomes(t *testing.T) {
	root := projectRoot(t)
	writeAgentsFile(t, root.Path(), "specs/SPEC-044-xdg.md", `---
id: SPEC-044
title: "XDG Render: Reproducible"
status: implementing
---
# XDG Render

Body that must render identically regardless of the state home location.
`)
	cacheHome := t.TempDir()

	render := func(dataHome string) (DurableRenderResult, []byte) {
		t.Helper()
		// Clear the resolver overrides so dataHome is derived from the
		// $XDG_DATA_HOME environment variable under test.
		t.Setenv("XDG_DATA_HOME", dataHome)
		resolver := PathResolver{CacheHome: cacheHome}
		if _, err := ApplyMarkdownMigration(context.Background(), root, resolver); err != nil {
			t.Fatalf("ApplyMarkdownMigration() error = %v", err)
		}
		result, err := RenderDurableArtifact(context.Background(), root, resolver, DurableRenderOptions{
			Kind:   "spec",
			Ref:    "SPEC-044",
			Branch: "feature/xdg",
		})
		if err != nil {
			t.Fatalf("RenderDurableArtifact() error = %v", err)
		}
		content, err := os.ReadFile(result.Path)
		if err != nil {
			t.Fatalf("read render path %q error = %v", result.Path, err)
		}
		return result, content
	}

	firstResult, firstBytes := render(t.TempDir())
	secondResult, secondBytes := render(t.TempDir())

	// Confirm the two homes actually resolved to distinct state databases,
	// otherwise the byte-equality assertion would be vacuous.
	if firstResult.DatabasePath == "" || firstResult.DatabasePath == secondResult.DatabasePath {
		t.Fatalf("expected distinct database paths across XDG homes, got %q and %q", firstResult.DatabasePath, secondResult.DatabasePath)
	}

	if !bytes.Equal(firstBytes, secondBytes) {
		t.Fatalf("durable render not byte-identical across XDG data homes:\n--- first (%s) ---\n%s\n--- second (%s) ---\n%s",
			firstResult.DatabasePath, firstBytes, secondResult.DatabasePath, secondBytes)
	}
	if firstResult.ContentHash != secondResult.ContentHash {
		t.Fatalf("content hash differs across XDG data homes: %q vs %q", firstResult.ContentHash, secondResult.ContentHash)
	}
}
