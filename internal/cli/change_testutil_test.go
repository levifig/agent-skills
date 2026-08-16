package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func changeDoc(frontmatter string, sections ...string) string {
	var b strings.Builder
	b.WriteString(frontmatter)
	b.WriteString("\n# Title\n\n")
	for _, section := range sections {
		b.WriteString(section)
		b.WriteString("\n\n")
	}
	return b.String()
}

func changeFrontmatter(change, created, branch string) string {
	return strings.Join([]string{
		"---",
		"change: " + change,
		"created: " + created,
		"branch: " + branch,
		"---",
	}, "\n")
}

func lineageFrontmatter(change, created, branch, lineage, predecessor, releaseAfter string) string {
	lines := []string{"---", "change: " + change, "created: " + created, "branch: " + branch, "lineage: " + lineage}
	if predecessor != "" {
		lines = append(lines, "predecessor: "+predecessor)
	}
	if releaseAfter != "" {
		lines = append(lines, "release-after: "+releaseAfter)
	}
	return strings.Join(append(lines, "---"), "\n")
}

func productSections() []string {
	return []string{
		"## Problem\n\nThe friction.",
		"## Hypothesis\n\nThe bet.",
		"## Scope\n\nIn and out.",
		"## Observable Workflow\n\nWhat ships.",
		"## Rabbit Holes and No-Gos\n\nBoundaries.",
	}
}

func executableSections() []string {
	return []string{
		"## Planning Contract\n\n### Approach\n\nHow.",
		"## Implementation Units\n\n- U1 — do the thing.",
		"## Verification Contract\n\n- V1. command exits non-zero.",
		"## Definition of Done\n\n- Gates pass.",
	}
}

func writeChangeFolder(t *testing.T, root, folder, content string) string {
	t.Helper()
	dir := filepath.Join(root, "docs", "changes", folder)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "change.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(change.md) error = %v", err)
	}
	return dir
}

func executableLineageDoc(slug, lineage, predecessor, releaseAfter string) string {
	sections := append(productSections(), executableSections()...)
	return changeDoc(lineageFrontmatter(slug, "2026-07-10", slug, lineage, predecessor, releaseAfter), sections...)
}

func findingsContain(findings []string, substr string) bool {
	for _, finding := range findings {
		if strings.Contains(finding, substr) {
			return true
		}
	}
	return false
}

func commitAllChangeTest(t *testing.T, repo, message string) {
	t.Helper()
	gitCLI(t, repo, "add", ".")
	gitCLI(t, repo, "-c", "user.name=Loaf Test", "-c", "user.email=loaf@example.test", "commit", "-m", message)
}

func writeNewLayoutChange(t *testing.T, repo, folder, slug, target string, shapeBody string) string {
	t.Helper()
	dir := filepath.Join(repo, "docs", "changes", folder)
	if err := os.MkdirAll(filepath.Join(dir, "tasks"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	meta := "{\n  \"change\": \"" + slug + "\",\n  \"created\": \"2026-07-27\",\n  \"branch\": \"" + slug + "\""
	if target != "" {
		meta += ",\n  \"target_release\": \"" + target + "\""
	}
	meta += "\n}\n"
	if err := os.WriteFile(filepath.Join(dir, "change.json"), []byte(meta), 0o644); err != nil {
		t.Fatalf("WriteFile change.json: %v", err)
	}
	if shapeBody == "" {
		shapeBody = authoredShapeBody()
	}
	if err := os.WriteFile(filepath.Join(dir, "shape.md"), []byte(shapeBody), 0o644); err != nil {
		t.Fatalf("WriteFile shape.md: %v", err)
	}
	return dir
}

func authoredShapeBody() string {
	sections := append(productSections(),
		"## Planning Contract\n\n### Approach\n\nHow.",
		"## Implementation Units\n\n- U1 — do the thing.",
		"## Verification Contract\n\n- **V1.** Smoke.\n  - Command: `true`\n  - Expect: exit 0",
		"## Definition of Done\n\n- Gates pass.",
	)
	var b strings.Builder
	b.WriteString("# Demo\n\n")
	for _, s := range sections {
		b.WriteString(s)
		b.WriteString("\n\n")
	}
	return b.String()
}
