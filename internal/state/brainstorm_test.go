package state

import (
	"context"
	"testing"

	"github.com/levifig/loaf/internal/project"
)

func TestListBrainstormsReadsImportedSQLiteBrainstorms(t *testing.T) {
	repo := initGitRepo(t)
	root, err := project.ResolveRoot(repo)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	stateHome := t.TempDir()
	writeAgentsFile(t, root.Path(), "drafts/20260528-brainstorm-open.md", `---
title: Open Brainstorm
status: open
---
# Open Brainstorm
`)
	writeAgentsFile(t, root.Path(), "drafts/20260528-brainstorm-resolved.md", `---
title: Resolved Brainstorm
status: resolved
---
# Resolved Brainstorm
`)
	writeAgentsFile(t, root.Path(), "drafts/20260528-brainstorm-archived.md", `---
title: Archived Brainstorm
status: archived
---
# Archived Brainstorm
`)
	writeAgentsFile(t, root.Path(), "TASKS.json", `{"tasks":{}}`)
	if _, err := ApplyMarkdownMigration(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("ApplyMarkdownMigration() error = %v", err)
	}

	defaultList, err := ListBrainstorms(context.Background(), root, PathResolver{StateHome: stateHome}, BrainstormListOptions{})
	if err != nil {
		t.Fatalf("ListBrainstorms() error = %v", err)
	}
	if defaultList.Brainstorms["20260528-brainstorm-open"].Status != "open" {
		t.Fatalf("defaultList = %#v, want open brainstorm", defaultList.Brainstorms)
	}
	if _, ok := defaultList.Brainstorms["20260528-brainstorm-resolved"]; ok {
		t.Fatalf("defaultList = %#v, want resolved brainstorm hidden", defaultList.Brainstorms)
	}
	if _, ok := defaultList.Brainstorms["20260528-brainstorm-archived"]; ok {
		t.Fatalf("defaultList = %#v, want archived brainstorm hidden", defaultList.Brainstorms)
	}
	open := defaultList.Brainstorms["20260528-brainstorm-open"]
	if open.Title != "Open Brainstorm" || open.SourcePath != ".agents/drafts/20260528-brainstorm-open.md" {
		t.Fatalf("open = %#v, want imported title and source path", open)
	}

	all, err := ListBrainstorms(context.Background(), root, PathResolver{StateHome: stateHome}, BrainstormListOptions{All: true})
	if err != nil {
		t.Fatalf("ListBrainstorms(All) error = %v", err)
	}
	if len(all.Brainstorms) != 3 {
		t.Fatalf("all = %#v, want all three brainstorms", all.Brainstorms)
	}

	archived, err := ListBrainstorms(context.Background(), root, PathResolver{StateHome: stateHome}, BrainstormListOptions{Status: "archived"})
	if err != nil {
		t.Fatalf("ListBrainstorms(Status archived) error = %v", err)
	}
	if len(archived.Brainstorms) != 1 || archived.Brainstorms["20260528-brainstorm-archived"].Status != "archived" {
		t.Fatalf("archived = %#v, want archived brainstorm only", archived.Brainstorms)
	}
}

func TestShowBrainstormReadsImportedSQLiteBrainstorm(t *testing.T) {
	repo := initGitRepo(t)
	root, err := project.ResolveRoot(repo)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	stateHome := t.TempDir()
	writeAgentsFile(t, root.Path(), "drafts/20260528-brainstorm-sqlite.md", `---
title: SQLite Brainstorm
status: open
promoted_to: .agents/ideas/20260528-target-idea.md
---
# SQLite Brainstorm

Imported brainstorm prose.
`)
	writeAgentsFile(t, root.Path(), "ideas/20260528-target-idea.md", `---
title: Target Idea
status: open
---
# Target Idea
`)
	writeAgentsFile(t, root.Path(), "TASKS.json", `{"tasks":{}}`)
	if _, err := ApplyMarkdownMigration(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("ApplyMarkdownMigration() error = %v", err)
	}

	show, err := ShowBrainstorm(context.Background(), root, PathResolver{StateHome: stateHome}, "20260528-brainstorm-sqlite")
	if err != nil {
		t.Fatalf("ShowBrainstorm() error = %v", err)
	}
	if show.Brainstorm.Alias != "20260528-brainstorm-sqlite" || show.Brainstorm.Title != "SQLite Brainstorm" || show.Brainstorm.Status != "open" {
		t.Fatalf("show = %#v, want imported brainstorm metadata", show)
	}
	if len(show.Brainstorm.Sources) != 1 || show.Brainstorm.Sources[0].Path != ".agents/drafts/20260528-brainstorm-sqlite.md" || show.Brainstorm.Sources[0].Hash == "" {
		t.Fatalf("Sources = %#v, want imported brainstorm source", show.Brainstorm.Sources)
	}
	if show.Brainstorm.Body != "# SQLite Brainstorm\n\nImported brainstorm prose." {
		t.Fatalf("Body = %q, want frontmatter-stripped imported body", show.Brainstorm.Body)
	}
	if !hasStateTraceRelationship(show.Brainstorm.Relationships, "outbound", "promoted_to", "idea", "20260528-target-idea") {
		t.Fatalf("Relationships = %#v, want promoted_to target idea", show.Brainstorm.Relationships)
	}
}

func TestPromoteBrainstormRecordsPromotedToRelationship(t *testing.T) {
	repo := initGitRepo(t)
	root, err := project.ResolveRoot(repo)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	stateHome := t.TempDir()
	writeAgentsFile(t, root.Path(), "drafts/20260528-brainstorm-sqlite.md", `---
title: SQLite Brainstorm
status: open
---
# SQLite Brainstorm
`)
	writeAgentsFile(t, root.Path(), "ideas/20260528-target-idea.md", `---
title: Target Idea
status: open
---
# Target Idea
`)
	writeAgentsFile(t, root.Path(), "TASKS.json", `{"tasks":{}}`)
	if _, err := ApplyMarkdownMigration(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("ApplyMarkdownMigration() error = %v", err)
	}

	result, err := PromoteBrainstorm(context.Background(), root, PathResolver{StateHome: stateHome}, BrainstormPromoteOptions{
		Brainstorm: "20260528-brainstorm-sqlite",
		ToIdea:     "20260528-target-idea",
	})
	if err != nil {
		t.Fatalf("PromoteBrainstorm() error = %v", err)
	}
	if result.Brainstorm.Alias != "20260528-brainstorm-sqlite" || result.Idea.Alias != "20260528-target-idea" || result.Relationship == "" {
		t.Fatalf("result = %#v, want brainstorm promoted to target idea with relationship", result)
	}

	trace, err := Trace(context.Background(), root, PathResolver{StateHome: stateHome}, "20260528-brainstorm-sqlite")
	if err != nil {
		t.Fatalf("Trace() error = %v", err)
	}
	if !hasStateTraceRelationship(trace.Relationships, "outbound", "promoted_to", "idea", "20260528-target-idea") {
		t.Fatalf("trace relationships = %#v, want promoted_to target idea", trace.Relationships)
	}

	show, err := ShowBrainstorm(context.Background(), root, PathResolver{StateHome: stateHome}, "20260528-brainstorm-sqlite")
	if err != nil {
		t.Fatalf("ShowBrainstorm() error = %v", err)
	}
	if !hasStateTraceRelationship(show.Brainstorm.Relationships, "outbound", "promoted_to", "idea", "20260528-target-idea") {
		t.Fatalf("show relationships = %#v, want promoted_to target idea", show.Brainstorm.Relationships)
	}

	result, err = PromoteBrainstorm(context.Background(), root, PathResolver{StateHome: stateHome}, BrainstormPromoteOptions{
		Brainstorm: "20260528-brainstorm-sqlite",
		ToIdea:     "20260528-target-idea",
	})
	if err != nil {
		t.Fatalf("PromoteBrainstorm() repeat error = %v", err)
	}
	store, err := OpenStore(mustDatabasePath(t, root, stateHome))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()
	var relationships int
	err = store.db.QueryRowContext(context.Background(), `
SELECT COUNT(*)
FROM relationships
WHERE id = ? AND relationship_type = 'promoted_to'
`, result.Relationship).Scan(&relationships)
	if err != nil {
		t.Fatalf("count relationships error = %v", err)
	}
	if relationships != 1 {
		t.Fatalf("relationships = %d, want one upserted promotion relationship", relationships)
	}
}

func TestPromoteBrainstormRejectsWrongKinds(t *testing.T) {
	repo := initGitRepo(t)
	root, err := project.ResolveRoot(repo)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	stateHome := t.TempDir()
	writeAgentsFile(t, root.Path(), "drafts/20260528-brainstorm-sqlite.md", "# SQLite Brainstorm\n")
	writeAgentsFile(t, root.Path(), "ideas/20260528-target-idea.md", "# Target Idea\n")
	writeAgentsFile(t, root.Path(), "specs/SPEC-001-target.md", "# Target Spec\n")
	writeAgentsFile(t, root.Path(), "TASKS.json", `{"tasks":{}}`)
	if _, err := ApplyMarkdownMigration(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("ApplyMarkdownMigration() error = %v", err)
	}

	_, err = PromoteBrainstorm(context.Background(), root, PathResolver{StateHome: stateHome}, BrainstormPromoteOptions{
		Brainstorm: "20260528-target-idea",
		ToIdea:     "20260528-target-idea",
	})
	if err == nil || err.Error() != `"20260528-target-idea" resolves to idea, not brainstorm` {
		t.Fatalf("PromoteBrainstorm() non-brainstorm error = %v, want not brainstorm", err)
	}

	_, err = PromoteBrainstorm(context.Background(), root, PathResolver{StateHome: stateHome}, BrainstormPromoteOptions{
		Brainstorm: "20260528-brainstorm-sqlite",
		ToIdea:     "SPEC-001",
	})
	if err == nil || err.Error() != `"SPEC-001" resolves to spec, not idea` {
		t.Fatalf("PromoteBrainstorm() non-idea error = %v, want not idea", err)
	}
}

func TestArchiveBrainstormsArchivesAndSkipsRefs(t *testing.T) {
	repo := initGitRepo(t)
	root, err := project.ResolveRoot(repo)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	stateHome := t.TempDir()
	writeAgentsFile(t, root.Path(), "drafts/20260528-brainstorm-open.md", `---
title: Open Brainstorm
status: open
---
# Open Brainstorm
`)
	writeAgentsFile(t, root.Path(), "drafts/20260528-brainstorm-archived.md", `---
title: Archived Brainstorm
status: archived
---
# Archived Brainstorm
`)
	writeAgentsFile(t, root.Path(), "ideas/20260528-target-idea.md", "# Target Idea\n")
	writeAgentsFile(t, root.Path(), "TASKS.json", `{"tasks":{}}`)
	if _, err := ApplyMarkdownMigration(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("ApplyMarkdownMigration() error = %v", err)
	}

	result, err := ArchiveBrainstorms(context.Background(), root, PathResolver{StateHome: stateHome}, BrainstormArchiveOptions{
		Refs:   []string{"20260528-brainstorm-open", "20260528-brainstorm-archived", "20260528-target-idea", "20260528-missing"},
		Reason: "promoted to idea",
	})
	if err != nil {
		t.Fatalf("ArchiveBrainstorms() error = %v", err)
	}
	if len(result.Archived) != 1 || result.Archived[0].Brainstorm == nil || result.Archived[0].Brainstorm.Alias != "20260528-brainstorm-open" || result.Archived[0].Previous != "open" || result.Archived[0].EventID == "" || result.Archived[0].Note != "promoted to idea" {
		t.Fatalf("Archived = %#v, want open brainstorm archived with event", result.Archived)
	}
	if len(result.Skipped) != 3 {
		t.Fatalf("Skipped = %#v, want already archived, wrong-kind, and missing refs", result.Skipped)
	}

	defaultList, err := ListBrainstorms(context.Background(), root, PathResolver{StateHome: stateHome}, BrainstormListOptions{})
	if err != nil {
		t.Fatalf("ListBrainstorms() default error = %v", err)
	}
	if _, ok := defaultList.Brainstorms["20260528-brainstorm-open"]; ok {
		t.Fatalf("defaultList.Brainstorms = %#v, want archived brainstorm hidden", defaultList.Brainstorms)
	}
	archivedOnly, err := ListBrainstorms(context.Background(), root, PathResolver{StateHome: stateHome}, BrainstormListOptions{Status: "archived"})
	if err != nil {
		t.Fatalf("ListBrainstorms(Status archived) error = %v", err)
	}
	if archivedOnly.Brainstorms["20260528-brainstorm-open"].Status != "archived" || archivedOnly.Brainstorms["20260528-brainstorm-archived"].Status != "archived" {
		t.Fatalf("archivedOnly.Brainstorms = %#v, want both archived brainstorms", archivedOnly.Brainstorms)
	}

	trace, err := Trace(context.Background(), root, PathResolver{StateHome: stateHome}, "20260528-brainstorm-open")
	if err != nil {
		t.Fatalf("Trace() error = %v", err)
	}
	if trace.Entity.Status != "archived" {
		t.Fatalf("trace status = %q, want archived", trace.Entity.Status)
	}
	show, err := ShowBrainstorm(context.Background(), root, PathResolver{StateHome: stateHome}, "20260528-brainstorm-open")
	if err != nil {
		t.Fatalf("ShowBrainstorm() error = %v", err)
	}
	if show.Brainstorm.Status != "archived" {
		t.Fatalf("show status = %q, want archived", show.Brainstorm.Status)
	}

	store, err := OpenStore(mustDatabasePath(t, root, stateHome))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()
	var events int
	var note string
	err = store.db.QueryRowContext(context.Background(), `
SELECT COUNT(*), COALESCE(MAX(note), '')
FROM events
WHERE project_id = ? AND entity_kind = 'brainstorm' AND event_type = 'status_changed' AND from_status = 'open' AND to_status = 'archived'
`, ProjectID(root)).Scan(&events, &note)
	if err != nil {
		t.Fatalf("count archive events error = %v", err)
	}
	if events != 1 {
		t.Fatalf("events = %d, want one status_changed event", events)
	}
	if note != "promoted to idea" {
		t.Fatalf("event note = %q, want archive reason", note)
	}
}
