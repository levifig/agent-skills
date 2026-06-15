package state

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/levifig/loaf/internal/project"
)

func TestResolveIdeaMarksResolvedAndRecordsRelationshipEvent(t *testing.T) {
	repo := initGitRepo(t)
	root, err := project.ResolveRoot(repo)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	stateHome := t.TempDir()
	writeAgentsFile(t, root.Path(), "ideas/20260528-sqlite-state.md", `---
title: SQLite State
status: open
---
# SQLite State
`)
	writeAgentsFile(t, root.Path(), "specs/SPEC-001-sqlite.md", "# SQLite Spec\n")
	writeAgentsFile(t, root.Path(), "TASKS.json", `{"tasks":{}}`)
	if _, err := ApplyMarkdownMigration(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("ApplyMarkdownMigration() error = %v", err)
	}

	before, err := ListIdeas(context.Background(), root, PathResolver{StateHome: stateHome}, IdeaListOptions{})
	if err != nil {
		t.Fatalf("ListIdeas() before error = %v", err)
	}
	if _, ok := before.Ideas["20260528-sqlite-state"]; !ok {
		t.Fatalf("before.Ideas = %#v, want imported idea in default list", before.Ideas)
	}
	assertIdeaProjectContext(t, root, before.ContractVersion, before.DatabaseScope, before.DatabasePath, before.ProjectID, before.ProjectName, before.ProjectCurrentPath)

	result, err := ResolveIdea(context.Background(), root, PathResolver{StateHome: stateHome}, "20260528-sqlite-state", "SPEC-001")
	if err != nil {
		t.Fatalf("ResolveIdea() error = %v", err)
	}
	if result.Idea.Status != "resolved" || result.ResolvedBy.Alias != "SPEC-001" || result.Relationship == "" || result.EventID == "" {
		t.Fatalf("result = %#v, want resolved idea, SPEC-001 target, relationship, and event", result)
	}
	assertIdeaProjectContext(t, root, result.ContractVersion, result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)

	after, err := ListIdeas(context.Background(), root, PathResolver{StateHome: stateHome}, IdeaListOptions{})
	if err != nil {
		t.Fatalf("ListIdeas() after error = %v", err)
	}
	if _, ok := after.Ideas["20260528-sqlite-state"]; ok {
		t.Fatalf("after.Ideas = %#v, want resolved idea omitted from default list", after.Ideas)
	}
	all, err := ListIdeas(context.Background(), root, PathResolver{StateHome: stateHome}, IdeaListOptions{All: true})
	if err != nil {
		t.Fatalf("ListIdeas(All) error = %v", err)
	}
	if all.Ideas["20260528-sqlite-state"].Status != "resolved" {
		t.Fatalf("all.Ideas = %#v, want resolved idea included with status", all.Ideas)
	}
	assertIdeaProjectContext(t, root, all.ContractVersion, all.DatabaseScope, all.DatabasePath, all.ProjectID, all.ProjectName, all.ProjectCurrentPath)
	resolvedOnly, err := ListIdeas(context.Background(), root, PathResolver{StateHome: stateHome}, IdeaListOptions{Status: "resolved"})
	if err != nil {
		t.Fatalf("ListIdeas(Status resolved) error = %v", err)
	}
	if resolvedOnly.Ideas["20260528-sqlite-state"].Status != "resolved" {
		t.Fatalf("resolvedOnly.Ideas = %#v, want explicit status filter to include resolved idea", resolvedOnly.Ideas)
	}

	trace, err := Trace(context.Background(), root, PathResolver{StateHome: stateHome}, "20260528-sqlite-state")
	if err != nil {
		t.Fatalf("Trace() error = %v", err)
	}
	if !hasStateTraceRelationship(trace.Relationships, "outbound", "resolved_by", "spec", "SPEC-001") {
		t.Fatalf("Relationships = %#v, want idea resolved_by SPEC-001", trace.Relationships)
	}

	store, err := OpenStore(mustDatabasePath(t, root, stateHome))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()
	var events int
	err = store.db.QueryRowContext(context.Background(), `
SELECT COUNT(*)
FROM events
WHERE project_id = ? AND entity_kind = 'idea' AND event_type = 'status_changed' AND from_status = 'open' AND to_status = 'resolved'
`, projectIDForTest(t, store, root)).Scan(&events)
	if err != nil {
		t.Fatalf("count events error = %v", err)
	}
	if events != 1 {
		t.Fatalf("events = %d, want one status_changed event", events)
	}
}

func TestResolveIdeaRejectsNonIdeaReference(t *testing.T) {
	repo := initGitRepo(t)
	root, err := project.ResolveRoot(repo)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	stateHome := t.TempDir()
	writeAgentsFile(t, root.Path(), "specs/SPEC-001-sqlite.md", "# SQLite Spec\n")
	writeAgentsFile(t, root.Path(), "TASKS.json", `{"tasks":{}}`)
	if _, err := ApplyMarkdownMigration(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("ApplyMarkdownMigration() error = %v", err)
	}

	_, err = ResolveIdea(context.Background(), root, PathResolver{StateHome: stateHome}, "SPEC-001", "SPEC-001")
	if err == nil {
		t.Fatal("ResolveIdea() error = nil, want non-idea rejection")
	}
}

func TestShowIdeaReadsImportedSQLiteIdea(t *testing.T) {
	repo := initGitRepo(t)
	root, err := project.ResolveRoot(repo)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	stateHome := t.TempDir()
	writeAgentsFile(t, root.Path(), "ideas/20260528-sqlite-state.md", `---
title: SQLite State
status: open
resolved_by:
  - SPEC-001
---
# SQLite State

Imported idea prose.
`)
	writeAgentsFile(t, root.Path(), "specs/SPEC-001-sqlite-state.md", "# SQLite Spec\n")
	writeAgentsFile(t, root.Path(), "TASKS.json", `{"tasks":{}}`)
	if _, err := ApplyMarkdownMigration(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("ApplyMarkdownMigration() error = %v", err)
	}

	result, err := ShowIdea(context.Background(), root, PathResolver{StateHome: stateHome}, "20260528-sqlite-state")
	if err != nil {
		t.Fatalf("ShowIdea() error = %v", err)
	}

	idea := result.Idea
	if result.Query != "20260528-sqlite-state" {
		t.Fatalf("Query = %q, want 20260528-sqlite-state", result.Query)
	}
	assertIdeaProjectContext(t, root, result.ContractVersion, result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
	if idea.Alias != "20260528-sqlite-state" || idea.Title != "SQLite State" || idea.Status != "open" {
		t.Fatalf("Idea = %#v, want imported idea metadata", idea)
	}
	if len(idea.Sources) != 1 || idea.Sources[0].Path != ".agents/ideas/20260528-sqlite-state.md" || idea.Sources[0].Hash == "" {
		t.Fatalf("Sources = %#v, want idea source with hash", idea.Sources)
	}
	if !strings.Contains(idea.Body, "# SQLite State") || !strings.Contains(idea.Body, "Imported idea prose.") {
		t.Fatalf("Body = %q, want imported source body without frontmatter", idea.Body)
	}
	if strings.Contains(idea.Body, "resolved_by") || strings.Contains(idea.Body, "---") {
		t.Fatalf("Body = %q, want frontmatter stripped", idea.Body)
	}
	if !hasStateTraceRelationship(idea.Relationships, "outbound", "resolved_by", "spec", "SPEC-001") {
		t.Fatalf("Relationships = %#v, want resolved_by SPEC-001", idea.Relationships)
	}
	if idea.CreatedAt == "" || idea.UpdatedAt == "" {
		t.Fatalf("CreatedAt/UpdatedAt = %q/%q, want timestamps", idea.CreatedAt, idea.UpdatedAt)
	}
}

func TestShowIdeaReadsCapturedIdeaWithoutSource(t *testing.T) {
	repo := initGitRepo(t)
	root, err := project.ResolveRoot(repo)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	stateHome := t.TempDir()
	writeAgentsFile(t, root.Path(), "TASKS.json", `{"tasks":{}}`)
	if _, err := ApplyMarkdownMigration(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("ApplyMarkdownMigration() error = %v", err)
	}
	captured, err := CaptureIdea(context.Background(), root, PathResolver{StateHome: stateHome}, IdeaCaptureOptions{Title: "Captured Idea"})
	if err != nil {
		t.Fatalf("CaptureIdea() error = %v", err)
	}

	result, err := ShowIdea(context.Background(), root, PathResolver{StateHome: stateHome}, captured.Idea.Alias)
	if err != nil {
		t.Fatalf("ShowIdea() captured error = %v", err)
	}
	if result.Idea.Alias != captured.Idea.Alias || result.Idea.Title != "Captured Idea" || result.Idea.Status != "open" {
		t.Fatalf("Idea = %#v, want captured idea metadata", result.Idea)
	}
	assertIdeaProjectContext(t, root, result.ContractVersion, result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
	if len(result.Idea.Sources) != 0 || result.Idea.Body != "" {
		t.Fatalf("Idea = %#v, want no source/body for captured idea", result.Idea)
	}
}

func TestShowIdeaRejectsMissingAndNonIdeaTargets(t *testing.T) {
	repo := initGitRepo(t)
	root, err := project.ResolveRoot(repo)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	stateHome := t.TempDir()
	writeAgentsFile(t, root.Path(), "ideas/20260528-sqlite-state.md", "# SQLite State\n")
	writeAgentsFile(t, root.Path(), "specs/SPEC-001-sqlite-state.md", "# SQLite Spec\n")
	writeAgentsFile(t, root.Path(), "TASKS.json", `{"tasks":{}}`)
	if _, err := ApplyMarkdownMigration(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("ApplyMarkdownMigration() error = %v", err)
	}

	_, err = ShowIdea(context.Background(), root, PathResolver{StateHome: stateHome}, "SPEC-001")
	if err == nil {
		t.Fatal("ShowIdea(SPEC-001) error = nil, want non-idea rejection")
	}
	if !strings.Contains(err.Error(), "not idea") {
		t.Fatalf("error = %v, want non-idea rejection", err)
	}

	_, err = ShowIdea(context.Background(), root, PathResolver{StateHome: stateHome}, "20260528-missing")
	if err == nil {
		t.Fatal("ShowIdea(missing) error = nil, want missing-target rejection")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("error = %v, want not found", err)
	}
}

func TestPromoteIdeaRecordsPromotedToRelationship(t *testing.T) {
	repo := initGitRepo(t)
	root, err := project.ResolveRoot(repo)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	stateHome := t.TempDir()
	writeAgentsFile(t, root.Path(), "ideas/20260528-sqlite-state.md", `---
title: SQLite State
status: open
---
# SQLite State
`)
	writeAgentsFile(t, root.Path(), "specs/SPEC-001-sqlite-state.md", "# SQLite Spec\n")
	writeAgentsFile(t, root.Path(), "TASKS.json", `{"tasks":{}}`)
	if _, err := ApplyMarkdownMigration(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("ApplyMarkdownMigration() error = %v", err)
	}

	result, err := PromoteIdea(context.Background(), root, PathResolver{StateHome: stateHome}, IdeaPromoteOptions{
		Idea:   "20260528-sqlite-state",
		ToSpec: "SPEC-001",
	})
	if err != nil {
		t.Fatalf("PromoteIdea() error = %v", err)
	}
	if result.Idea.Alias != "20260528-sqlite-state" || result.Idea.Status != "open" || result.Spec.Alias != "SPEC-001" || result.Relationship == "" {
		t.Fatalf("result = %#v, want open idea promoted to target spec with relationship", result)
	}
	assertIdeaProjectContext(t, root, result.ContractVersion, result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)

	ideas, err := ListIdeas(context.Background(), root, PathResolver{StateHome: stateHome}, IdeaListOptions{})
	if err != nil {
		t.Fatalf("ListIdeas() error = %v", err)
	}
	if ideas.Ideas["20260528-sqlite-state"].Status != "open" {
		t.Fatalf("ideas = %#v, want promotion to leave idea open", ideas.Ideas)
	}

	trace, err := Trace(context.Background(), root, PathResolver{StateHome: stateHome}, "20260528-sqlite-state")
	if err != nil {
		t.Fatalf("Trace() error = %v", err)
	}
	if !hasStateTraceRelationship(trace.Relationships, "outbound", "promoted_to", "spec", "SPEC-001") {
		t.Fatalf("Relationships = %#v, want idea promoted_to target spec", trace.Relationships)
	}

	links, err := ListLinks(context.Background(), root, PathResolver{StateHome: stateHome}, "20260528-sqlite-state")
	if err != nil {
		t.Fatalf("ListLinks() error = %v", err)
	}
	if !hasStateTraceRelationship(links.Relationships, "outbound", "promoted_to", "spec", "SPEC-001") {
		t.Fatalf("Links = %#v, want idea promoted_to target spec", links.Relationships)
	}

	show, err := ShowIdea(context.Background(), root, PathResolver{StateHome: stateHome}, "20260528-sqlite-state")
	if err != nil {
		t.Fatalf("ShowIdea() error = %v", err)
	}
	if !hasStateTraceRelationship(show.Idea.Relationships, "outbound", "promoted_to", "spec", "SPEC-001") {
		t.Fatalf("ShowIdea relationships = %#v, want promoted_to target spec", show.Idea.Relationships)
	}

	result, err = PromoteIdea(context.Background(), root, PathResolver{StateHome: stateHome}, IdeaPromoteOptions{
		Idea:   "20260528-sqlite-state",
		ToSpec: "SPEC-001",
	})
	if err != nil {
		t.Fatalf("PromoteIdea() repeat error = %v", err)
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

func TestPromoteIdeaRejectsWrongKinds(t *testing.T) {
	repo := initGitRepo(t)
	root, err := project.ResolveRoot(repo)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	stateHome := t.TempDir()
	writeAgentsFile(t, root.Path(), "ideas/20260528-sqlite-state.md", "# SQLite State\n")
	writeAgentsFile(t, root.Path(), "specs/SPEC-001-sqlite-state.md", "# SQLite Spec\n")
	writeAgentsFile(t, root.Path(), "sessions/20260528-session.md", "[2026-05-28 10:00] spark(sqlite): smoke spark\n")
	writeAgentsFile(t, root.Path(), "TASKS.json", `{"tasks":{}}`)
	if _, err := ApplyMarkdownMigration(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("ApplyMarkdownMigration() error = %v", err)
	}

	_, err = PromoteIdea(context.Background(), root, PathResolver{StateHome: stateHome}, IdeaPromoteOptions{
		Idea:   "SPEC-001",
		ToSpec: "SPEC-001",
	})
	if err == nil || !strings.Contains(err.Error(), "not idea") {
		t.Fatalf("PromoteIdea() non-idea error = %v, want not idea", err)
	}

	_, err = PromoteIdea(context.Background(), root, PathResolver{StateHome: stateHome}, IdeaPromoteOptions{
		Idea:   "20260528-sqlite-state",
		ToSpec: "SPARK-smoke",
	})
	if err == nil || !strings.Contains(err.Error(), "not spec") {
		t.Fatalf("PromoteIdea() non-spec error = %v, want not spec", err)
	}
}

func TestCaptureIdeaCreatesOpenIdeaWithAliasAndEvent(t *testing.T) {
	repo := initGitRepo(t)
	root, err := project.ResolveRoot(repo)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	stateHome := t.TempDir()
	writeAgentsFile(t, root.Path(), "TASKS.json", `{"tasks":{}}`)
	if _, err := ApplyMarkdownMigration(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("ApplyMarkdownMigration() error = %v", err)
	}

	first, err := CaptureIdea(context.Background(), root, PathResolver{StateHome: stateHome}, IdeaCaptureOptions{Title: "Repeat Idea"})
	if err != nil {
		t.Fatalf("CaptureIdea() first error = %v", err)
	}
	if first.Idea.Status != "open" || first.Idea.Title != "Repeat Idea" || !strings.Contains(first.Idea.Alias, "repeat-idea") || !strings.HasPrefix(first.Idea.Alias, "IDEA-") || first.EventID == "" {
		t.Fatalf("first = %#v, want open idea with dated slug alias and event", first)
	}
	assertIdeaProjectContext(t, root, first.ContractVersion, first.DatabaseScope, first.DatabasePath, first.ProjectID, first.ProjectName, first.ProjectCurrentPath)
	second, err := CaptureIdea(context.Background(), root, PathResolver{StateHome: stateHome}, IdeaCaptureOptions{Title: "Repeat Idea"})
	if err != nil {
		t.Fatalf("CaptureIdea() second error = %v", err)
	}
	if second.Idea.Alias != first.Idea.Alias+"-2" {
		t.Fatalf("second alias = %q, want collision suffix after %q", second.Idea.Alias, first.Idea.Alias)
	}

	ideas, err := ListIdeas(context.Background(), root, PathResolver{StateHome: stateHome}, IdeaListOptions{})
	if err != nil {
		t.Fatalf("ListIdeas() error = %v", err)
	}
	if ideas.Ideas[first.Idea.Alias].Status != "open" || ideas.Ideas[first.Idea.Alias].Title != "Repeat Idea" {
		t.Fatalf("ideas = %#v, want captured idea visible in default list", ideas.Ideas)
	}
	assertIdeaProjectContext(t, root, ideas.ContractVersion, ideas.DatabaseScope, ideas.DatabasePath, ideas.ProjectID, ideas.ProjectName, ideas.ProjectCurrentPath)
	trace, err := Trace(context.Background(), root, PathResolver{StateHome: stateHome}, first.Idea.Alias)
	if err != nil {
		t.Fatalf("Trace() error = %v", err)
	}
	if trace.Entity.Status != "open" || trace.Entity.Alias != first.Idea.Alias {
		t.Fatalf("trace entity = %#v, want captured open idea", trace.Entity)
	}

	store, err := OpenStore(mustDatabasePath(t, root, stateHome))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()
	var events int
	err = store.db.QueryRowContext(context.Background(), `
SELECT COUNT(*)
FROM events
WHERE project_id = ? AND entity_kind = 'idea' AND event_type = 'status_changed' AND from_status IS NULL AND to_status = 'open'
`, projectIDForTest(t, store, root)).Scan(&events)
	if err != nil {
		t.Fatalf("count capture events error = %v", err)
	}
	if events != 2 {
		t.Fatalf("events = %d, want one status event per captured idea", events)
	}
}

func TestArchiveIdeasArchivesAndSkipsRefs(t *testing.T) {
	repo := initGitRepo(t)
	root, err := project.ResolveRoot(repo)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	stateHome := t.TempDir()
	writeAgentsFile(t, root.Path(), "ideas/20260528-open-idea.md", `---
title: Open Idea
status: open
---
# Open Idea
`)
	writeAgentsFile(t, root.Path(), "ideas/20260528-archived-idea.md", `---
title: Archived Idea
status: archived
---
# Archived Idea
`)
	writeAgentsFile(t, root.Path(), "specs/SPEC-001-sqlite.md", "# SQLite Spec\n")
	writeAgentsFile(t, root.Path(), "TASKS.json", `{"tasks":{}}`)
	if _, err := ApplyMarkdownMigration(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("ApplyMarkdownMigration() error = %v", err)
	}

	result, err := ArchiveIdeas(context.Background(), root, PathResolver{StateHome: stateHome}, IdeaArchiveOptions{
		Refs:   []string{"20260528-open-idea", "20260528-archived-idea", "SPEC-001", "20260528-missing"},
		Reason: "covered by SPEC-001",
	})
	if err != nil {
		t.Fatalf("ArchiveIdeas() error = %v", err)
	}
	if len(result.Archived) != 1 || result.Archived[0].Idea == nil || result.Archived[0].Idea.Alias != "20260528-open-idea" || result.Archived[0].Previous != "open" || result.Archived[0].EventID == "" || result.Archived[0].Note != "covered by SPEC-001" {
		t.Fatalf("Archived = %#v, want open idea archived with event", result.Archived)
	}
	assertIdeaProjectContext(t, root, result.ContractVersion, result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
	if len(result.Skipped) != 3 {
		t.Fatalf("Skipped = %#v, want already archived, wrong-kind, and missing refs", result.Skipped)
	}

	defaultList, err := ListIdeas(context.Background(), root, PathResolver{StateHome: stateHome}, IdeaListOptions{})
	if err != nil {
		t.Fatalf("ListIdeas() default error = %v", err)
	}
	if _, ok := defaultList.Ideas["20260528-open-idea"]; ok {
		t.Fatalf("defaultList.Ideas = %#v, want archived idea hidden", defaultList.Ideas)
	}
	archivedOnly, err := ListIdeas(context.Background(), root, PathResolver{StateHome: stateHome}, IdeaListOptions{Status: "archived"})
	if err != nil {
		t.Fatalf("ListIdeas(Status archived) error = %v", err)
	}
	if archivedOnly.Ideas["20260528-open-idea"].Status != "archived" || archivedOnly.Ideas["20260528-archived-idea"].Status != "archived" {
		t.Fatalf("archivedOnly.Ideas = %#v, want both archived ideas", archivedOnly.Ideas)
	}

	trace, err := Trace(context.Background(), root, PathResolver{StateHome: stateHome}, "20260528-open-idea")
	if err != nil {
		t.Fatalf("Trace() error = %v", err)
	}
	if trace.Entity.Status != "archived" {
		t.Fatalf("trace status = %q, want archived", trace.Entity.Status)
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
WHERE project_id = ? AND entity_kind = 'idea' AND event_type = 'status_changed' AND from_status = 'open' AND to_status = 'archived'
`, projectIDForTest(t, store, root)).Scan(&events, &note)
	if err != nil {
		t.Fatalf("count archive events error = %v", err)
	}
	if events != 1 {
		t.Fatalf("events = %d, want one status_changed event", events)
	}
	if note != "covered by SPEC-001" {
		t.Fatalf("event note = %q, want archive reason", note)
	}
}

func mustDatabasePath(t *testing.T, root project.Root, stateHome string) string {
	t.Helper()
	path, err := (PathResolver{StateHome: stateHome}).DatabasePath(root)
	if err != nil {
		t.Fatalf("DatabasePath() error = %v", err)
	}
	return path
}

func hasStateTraceRelationship(relationships []TraceRelationship, direction string, relationshipType string, kind string, alias string) bool {
	for _, relationship := range relationships {
		if relationship.Direction == direction && relationship.Type == relationshipType && relationship.Entity.Kind == kind && relationship.Entity.Alias == alias {
			return true
		}
	}
	return false
}

func assertIdeaProjectContext(t *testing.T, root project.Root, contractVersion int, databaseScope string, databasePath string, projectID string, projectName string, projectCurrentPath string) {
	t.Helper()
	if contractVersion != StateJSONContractVersion {
		t.Fatalf("ContractVersion = %d, want %d", contractVersion, StateJSONContractVersion)
	}
	if databaseScope != "global" {
		t.Fatalf("DatabaseScope = %q, want global", databaseScope)
	}
	if databasePath == "" {
		t.Fatal("DatabasePath is empty")
	}
	if projectID == "" {
		t.Fatal("ProjectID is empty")
	}
	if projectName != filepath.Base(root.Path()) {
		t.Fatalf("ProjectName = %q, want %q", projectName, filepath.Base(root.Path()))
	}
	if projectCurrentPath != root.Path() {
		t.Fatalf("ProjectCurrentPath = %q, want %q", projectCurrentPath, root.Path())
	}
}
