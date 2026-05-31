package state

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestApplyMarkdownMigrationImportsArtifactsAndPreservesSources(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	taskBody := "# Task body\n"
	writeMarkdownImportFixture(t, root.Path(), taskBody)

	result, err := ApplyMarkdownMigration(context.Background(), root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("ApplyMarkdownMigration() error = %v", err)
	}

	if !result.Applied {
		t.Fatal("Applied = false, want true")
	}
	if result.DatabasePath == "" {
		t.Fatal("DatabasePath is empty")
	}
	if _, err := os.Stat(result.DatabasePath); err != nil {
		t.Fatalf("database was not created: %v", err)
	}
	if result.Specs != 1 || result.Tasks != 1 || result.Ideas != 1 || result.Brainstorms != 1 || result.Sessions != 1 || result.Reports != 1 || result.Sparks != 1 {
		t.Fatalf("result counts = %#v, want one imported artifact of each fixture kind", result.MarkdownMigrationPlan)
	}
	if result.Relationships != 2 {
		t.Fatalf("Relationships = %d, want dry-run relationship count 2", result.Relationships)
	}

	store, err := OpenStore(result.DatabasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()

	assertTableCount(t, store, "specs", 1)
	assertTableCount(t, store, "tasks", 1)
	assertTableCount(t, store, "ideas", 1)
	assertTableCount(t, store, "brainstorms", 1)
	assertTableCount(t, store, "sessions", 1)
	assertTableCount(t, store, "reports", 1)
	assertTableCount(t, store, "sparks", 1)
	assertTableCount(t, store, "journal_entries", 1)
	assertTableCount(t, store, "relationships", 2)

	var sourceHash string
	err = store.db.QueryRowContext(
		context.Background(),
		`SELECT hash FROM sources WHERE path = ?`,
		".agents/tasks/TASK-001-example.md",
	).Scan(&sourceHash)
	if err != nil {
		t.Fatalf("read source hash error = %v", err)
	}
	sum := sha256.Sum256([]byte(taskBody))
	if sourceHash != hex.EncodeToString(sum[:]) {
		t.Fatalf("source hash = %q, want task body hash", sourceHash)
	}
	taskPath := filepath.Join(root.Path(), ".agents", "tasks", "TASK-001-example.md")
	contentAfterApply, err := os.ReadFile(taskPath)
	if err != nil {
		t.Fatalf("ReadFile(task) error = %v", err)
	}
	if string(contentAfterApply) != taskBody {
		t.Fatalf("task source was mutated: %q", string(contentAfterApply))
	}

	second, err := ApplyMarkdownMigration(context.Background(), root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("second ApplyMarkdownMigration() error = %v", err)
	}
	if second.DatabasePath != result.DatabasePath {
		t.Fatalf("DatabasePath changed: %q -> %q", result.DatabasePath, second.DatabasePath)
	}
	assertTableCount(t, store, "specs", 1)
	assertTableCount(t, store, "tasks", 1)
	assertTableCount(t, store, "relationships", 2)
}

func TestFrontmatterListItemsPreserveCommas(t *testing.T) {
	frontmatter := parseFrontmatterMap([]byte(`---
implements:
  - .agents/specs/SPEC-000-target, with comma.md
  - SPEC-001
---
# Source Spec
`))

	got := splitFrontmatterList(frontmatter["implements"])
	want := []string{".agents/specs/SPEC-000-target, with comma.md", "SPEC-001"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("splitFrontmatterList() = %#v, want %#v", got, want)
	}
}

func writeMarkdownImportFixture(t *testing.T, root string, taskBody string) {
	t.Helper()
	writeAgentsFile(t, root, "specs/SPEC-001-example.md", `---
id: SPEC-001
title: Example Spec
status: implementing
---
# Example Spec
`)
	writeAgentsFile(t, root, "tasks/TASK-001-example.md", taskBody)
	writeAgentsFile(t, root, "ideas/20260528-idea.md", "# Example Idea\n")
	writeAgentsFile(t, root, "drafts/20260528-brainstorm-topic.md", "# Example Brainstorm\n")
	writeAgentsFile(t, root, "sessions/20260528-session.md", `---
branch: feature/example
status: active
---
[2026-05-28 10:00] spark(scope): capture this
`)
	writeAgentsFile(t, root, "reports/report.md", `---
kind: session
title: Example Report
status: final
---
# Example Report
`)
	writeAgentsFile(t, root, "TASKS.json", `{
  "tasks": {
    "TASK-001": {
      "title": "Example Task",
      "spec": "SPEC-001",
      "status": "todo",
      "priority": "P1",
      "depends_on": ["TASK-000"]
    }
  }
}
`)
}

func assertTableCount(t *testing.T, store *Store, table string, want int) {
	t.Helper()
	var got int
	if err := store.db.QueryRowContext(context.Background(), fmt.Sprintf(`SELECT COUNT(*) FROM %s`, table)).Scan(&got); err != nil {
		t.Fatalf("count %s error = %v", table, err)
	}
	if got != want {
		t.Fatalf("count %s = %d, want %d", table, got, want)
	}
}
