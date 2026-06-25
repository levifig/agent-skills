package state

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/levifig/loaf/internal/project"
)

func TestResolveSparkMarksResolvedAndRecordsRelationshipEvent(t *testing.T) {
	repo := initGitRepo(t)
	root, err := project.ResolveRoot(repo)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	stateHome := t.TempDir()
	writeAgentsFile(t, root.Path(), "sessions/20260528-session.md", "[2026-05-28 10:00] spark(sqlite): smoke spark\n")
	writeAgentsFile(t, root.Path(), "ideas/20260528-target-idea.md", "# Target Idea\n")
	writeAgentsFile(t, root.Path(), "TASKS.json", `{"tasks":{}}`)
	if _, err := ApplyMarkdownMigration(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("ApplyMarkdownMigration() error = %v", err)
	}

	before, err := ListSparks(context.Background(), root, PathResolver{StateHome: stateHome}, SparkListOptions{})
	if err != nil {
		t.Fatalf("ListSparks() before error = %v", err)
	}
	if before.Sparks["SPARK-smoke"].Status != "open" {
		t.Fatalf("before.Sparks = %#v, want imported open spark", before.Sparks)
	}
	assertSparkProjectContext(t, root, before.ContractVersion, before.DatabaseScope, before.DatabasePath, before.ProjectID, before.ProjectName, before.ProjectCurrentPath)

	result, err := ResolveSparkWithOptions(context.Background(), root, PathResolver{StateHome: stateHome}, SparkResolveOptions{
		Spark:  "SPARK-smoke",
		By:     "20260528-target-idea",
		Reason: "triaged into target idea",
	})
	if err != nil {
		t.Fatalf("ResolveSparkWithOptions() error = %v", err)
	}
	if result.Spark.Status != "done" || result.ResolvedBy.Alias != "20260528-target-idea" || result.Relationship == "" || result.EventID == "" || result.Reason != "triaged into target idea" {
		t.Fatalf("result = %#v, want done spark, target idea, relationship, and event", result)
	}
	assertSparkProjectContext(t, root, result.ContractVersion, result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)

	after, err := ListSparks(context.Background(), root, PathResolver{StateHome: stateHome}, SparkListOptions{})
	if err != nil {
		t.Fatalf("ListSparks() after error = %v", err)
	}
	if _, ok := after.Sparks["SPARK-smoke"]; ok {
		t.Fatalf("after.Sparks = %#v, want done spark omitted from default list", after.Sparks)
	}
	all, err := ListSparks(context.Background(), root, PathResolver{StateHome: stateHome}, SparkListOptions{All: true})
	if err != nil {
		t.Fatalf("ListSparks(All) error = %v", err)
	}
	if all.Sparks["SPARK-smoke"].Status != "done" {
		t.Fatalf("all.Sparks = %#v, want done spark included with status", all.Sparks)
	}
	assertSparkProjectContext(t, root, all.ContractVersion, all.DatabaseScope, all.DatabasePath, all.ProjectID, all.ProjectName, all.ProjectCurrentPath)
	resolvedOnly, err := ListSparks(context.Background(), root, PathResolver{StateHome: stateHome}, SparkListOptions{Status: "done"})
	if err != nil {
		t.Fatalf("ListSparks(Status done) error = %v", err)
	}
	if resolvedOnly.Sparks["SPARK-smoke"].Status != "done" {
		t.Fatalf("resolvedOnly.Sparks = %#v, want explicit status filter to include done spark", resolvedOnly.Sparks)
	}

	trace, err := Trace(context.Background(), root, PathResolver{StateHome: stateHome}, "SPARK-smoke")
	if err != nil {
		t.Fatalf("Trace() error = %v", err)
	}
	if !hasStateTraceRelationship(trace.Relationships, "outbound", "resolved_by", "idea", "20260528-target-idea") {
		t.Fatalf("Relationships = %#v, want spark resolved_by target idea", trace.Relationships)
	}

	store, err := OpenStore(mustDatabasePath(t, root, stateHome))
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()
	var events int
	var eventNote string
	err = store.db.QueryRowContext(context.Background(), `
SELECT COUNT(*), COALESCE(MAX(note), '')
FROM events
WHERE project_id = ? AND entity_kind = 'spark' AND event_type = 'status_changed' AND from_status = 'open' AND to_status = 'done'
`, projectIDForTest(t, store, root)).Scan(&events, &eventNote)
	if err != nil {
		t.Fatalf("count events error = %v", err)
	}
	if events != 1 {
		t.Fatalf("events = %d, want one status_changed event", events)
	}
	if eventNote != "triaged into target idea" {
		t.Fatalf("event note = %q, want resolve reason", eventNote)
	}

	result, err = ResolveSparkWithOptions(context.Background(), root, PathResolver{StateHome: stateHome}, SparkResolveOptions{
		Spark:  "SPARK-smoke",
		By:     "20260528-target-idea",
		Reason: "updated rationale",
	})
	if err != nil {
		t.Fatalf("ResolveSparkWithOptions() repeat error = %v", err)
	}
	if result.EventID != "" || result.Reason != "updated rationale" {
		t.Fatalf("repeat result = %#v, want relationship update without duplicate event", result)
	}
	assertSparkProjectContext(t, root, result.ContractVersion, result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
	err = store.db.QueryRowContext(context.Background(), `
SELECT COUNT(*)
FROM events
WHERE project_id = ? AND entity_kind = 'spark' AND event_type = 'status_changed' AND from_status = 'open' AND to_status = 'done'
`, projectIDForTest(t, store, root)).Scan(&events)
	if err != nil {
		t.Fatalf("count events after repeat error = %v", err)
	}
	if events != 1 {
		t.Fatalf("events after repeat = %d, want one status_changed event", events)
	}
	var relationshipReason string
	err = store.db.QueryRowContext(context.Background(), `
SELECT reason
FROM relationships
WHERE id = ?
`, result.Relationship).Scan(&relationshipReason)
	if err != nil {
		t.Fatalf("read relationship reason error = %v", err)
	}
	if relationshipReason != "updated rationale" {
		t.Fatalf("relationship reason = %q, want updated rationale", relationshipReason)
	}
}

func TestResolveSparkRejectsNonSparkReference(t *testing.T) {
	repo := initGitRepo(t)
	root, err := project.ResolveRoot(repo)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	stateHome := t.TempDir()
	writeAgentsFile(t, root.Path(), "ideas/20260528-target-idea.md", "# Target Idea\n")
	writeAgentsFile(t, root.Path(), "TASKS.json", `{"tasks":{}}`)
	if _, err := ApplyMarkdownMigration(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("ApplyMarkdownMigration() error = %v", err)
	}

	_, err = ResolveSpark(context.Background(), root, PathResolver{StateHome: stateHome}, "20260528-target-idea", "20260528-target-idea")
	if err == nil {
		t.Fatal("ResolveSpark() error = nil, want non-spark rejection")
	}
}

func TestShowSparkReadsImportedSQLiteSpark(t *testing.T) {
	repo := initGitRepo(t)
	root, err := project.ResolveRoot(repo)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	stateHome := t.TempDir()
	writeAgentsFile(t, root.Path(), "sessions/20260528-session.md", "[2026-05-28 10:00] spark(sqlite): smoke spark\n")
	writeAgentsFile(t, root.Path(), "ideas/20260528-target-idea.md", "# Target Idea\n")
	writeAgentsFile(t, root.Path(), "TASKS.json", `{"tasks":{}}`)
	if _, err := ApplyMarkdownMigration(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("ApplyMarkdownMigration() error = %v", err)
	}
	if _, err := PromoteSpark(context.Background(), root, PathResolver{StateHome: stateHome}, SparkPromoteOptions{
		Spark:  "SPARK-smoke",
		ToIdea: "20260528-target-idea",
	}); err != nil {
		t.Fatalf("PromoteSpark() error = %v", err)
	}

	result, err := ShowSpark(context.Background(), root, PathResolver{StateHome: stateHome}, "SPARK-smoke")
	if err != nil {
		t.Fatalf("ShowSpark() error = %v", err)
	}

	spark := result.Spark
	if result.Query != "SPARK-smoke" {
		t.Fatalf("Query = %q, want SPARK-smoke", result.Query)
	}
	assertSparkProjectContext(t, root, result.ContractVersion, result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
	if spark.Alias != "SPARK-smoke" || spark.Text != "smoke spark" || spark.Scope != "sqlite" || spark.Status != "open" {
		t.Fatalf("Spark = %#v, want imported spark metadata", spark)
	}
	if len(spark.Sources) != 1 || spark.Sources[0].Path != ".agents/sessions/20260528-session.md" || spark.Sources[0].Hash == "" {
		t.Fatalf("Sources = %#v, want session source with hash", spark.Sources)
	}
	if !hasStateTraceRelationship(spark.Relationships, "outbound", "promoted_to", "idea", "20260528-target-idea") {
		t.Fatalf("Relationships = %#v, want promoted_to target idea", spark.Relationships)
	}
	if spark.CreatedAt == "" || spark.UpdatedAt == "" {
		t.Fatalf("CreatedAt/UpdatedAt = %q/%q, want timestamps", spark.CreatedAt, spark.UpdatedAt)
	}
}

func TestShowSparkRejectsNonSparkReference(t *testing.T) {
	repo := initGitRepo(t)
	root, err := project.ResolveRoot(repo)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	stateHome := t.TempDir()
	writeAgentsFile(t, root.Path(), "ideas/20260528-target-idea.md", "# Target Idea\n")
	writeAgentsFile(t, root.Path(), "TASKS.json", `{"tasks":{}}`)
	if _, err := ApplyMarkdownMigration(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("ApplyMarkdownMigration() error = %v", err)
	}

	_, err = ShowSpark(context.Background(), root, PathResolver{StateHome: stateHome}, "20260528-target-idea")
	if err == nil {
		t.Fatal("ShowSpark() error = nil, want non-spark rejection")
	}
	if !strings.Contains(err.Error(), "not spark") {
		t.Fatalf("error = %v, want non-spark rejection", err)
	}
}

func TestPromoteSparkRecordsPromotedToRelationship(t *testing.T) {
	repo := initGitRepo(t)
	root, err := project.ResolveRoot(repo)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	stateHome := t.TempDir()
	writeAgentsFile(t, root.Path(), "sessions/20260528-session.md", "[2026-05-28 10:00] spark(sqlite): smoke spark\n")
	writeAgentsFile(t, root.Path(), "ideas/20260528-target-idea.md", "# Target Idea\n")
	writeAgentsFile(t, root.Path(), "TASKS.json", `{"tasks":{}}`)
	if _, err := ApplyMarkdownMigration(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("ApplyMarkdownMigration() error = %v", err)
	}

	result, err := PromoteSpark(context.Background(), root, PathResolver{StateHome: stateHome}, SparkPromoteOptions{
		Spark:  "SPARK-smoke",
		ToIdea: "20260528-target-idea",
	})
	if err != nil {
		t.Fatalf("PromoteSpark() error = %v", err)
	}
	if result.Spark.Alias != "SPARK-smoke" || result.Spark.Status != "open" || result.Idea.Alias != "20260528-target-idea" || result.Relationship == "" {
		t.Fatalf("result = %#v, want open spark promoted to target idea with relationship", result)
	}
	assertSparkProjectContext(t, root, result.ContractVersion, result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)

	sparks, err := ListSparks(context.Background(), root, PathResolver{StateHome: stateHome}, SparkListOptions{})
	if err != nil {
		t.Fatalf("ListSparks() error = %v", err)
	}
	if sparks.Sparks["SPARK-smoke"].Status != "open" {
		t.Fatalf("sparks = %#v, want promotion to leave spark open", sparks.Sparks)
	}

	trace, err := Trace(context.Background(), root, PathResolver{StateHome: stateHome}, "SPARK-smoke")
	if err != nil {
		t.Fatalf("Trace() error = %v", err)
	}
	if !hasStateTraceRelationship(trace.Relationships, "outbound", "promoted_to", "idea", "20260528-target-idea") {
		t.Fatalf("Relationships = %#v, want spark promoted_to target idea", trace.Relationships)
	}

	links, err := ListLinks(context.Background(), root, PathResolver{StateHome: stateHome}, "SPARK-smoke")
	if err != nil {
		t.Fatalf("ListLinks() error = %v", err)
	}
	if !hasStateTraceRelationship(links.Relationships, "outbound", "promoted_to", "idea", "20260528-target-idea") {
		t.Fatalf("Links = %#v, want spark promoted_to target idea", links.Relationships)
	}

	result, err = PromoteSpark(context.Background(), root, PathResolver{StateHome: stateHome}, SparkPromoteOptions{
		Spark:  "SPARK-smoke",
		ToIdea: "20260528-target-idea",
	})
	if err != nil {
		t.Fatalf("PromoteSpark() repeat error = %v", err)
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

func TestPromoteSparkRejectsWrongKinds(t *testing.T) {
	repo := initGitRepo(t)
	root, err := project.ResolveRoot(repo)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	stateHome := t.TempDir()
	writeAgentsFile(t, root.Path(), "sessions/20260528-session.md", "[2026-05-28 10:00] spark(sqlite): smoke spark\n")
	writeAgentsFile(t, root.Path(), "ideas/20260528-target-idea.md", "# Target Idea\n")
	writeAgentsFile(t, root.Path(), "specs/SPEC-001-target.md", "# Target Spec\n")
	writeAgentsFile(t, root.Path(), "TASKS.json", `{"tasks":{}}`)
	if _, err := ApplyMarkdownMigration(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("ApplyMarkdownMigration() error = %v", err)
	}

	_, err = PromoteSpark(context.Background(), root, PathResolver{StateHome: stateHome}, SparkPromoteOptions{
		Spark:  "20260528-target-idea",
		ToIdea: "20260528-target-idea",
	})
	if err == nil || !strings.Contains(err.Error(), "not spark") {
		t.Fatalf("PromoteSpark() non-spark error = %v, want not spark", err)
	}

	_, err = PromoteSpark(context.Background(), root, PathResolver{StateHome: stateHome}, SparkPromoteOptions{
		Spark:  "SPARK-smoke",
		ToIdea: "SPEC-001",
	})
	if err == nil || !strings.Contains(err.Error(), "not idea") {
		t.Fatalf("PromoteSpark() non-idea error = %v, want not idea", err)
	}
}

func TestCaptureSparkCreatesOpenSparkWithAliasAndEvent(t *testing.T) {
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

	first, err := CaptureSpark(context.Background(), root, PathResolver{StateHome: stateHome}, SparkCaptureOptions{
		Text:  "Repeat Spark",
		Scope: "architecture",
	})
	if err != nil {
		t.Fatalf("CaptureSpark() first error = %v", err)
	}
	if first.Spark.Alias != "SPARK-repeat-spark" || first.Spark.Status != "open" || first.Scope != "architecture" || first.EventID == "" {
		t.Fatalf("first = %#v, want open spark with slug alias, scope, and event", first)
	}
	assertSparkProjectContext(t, root, first.ContractVersion, first.DatabaseScope, first.DatabasePath, first.ProjectID, first.ProjectName, first.ProjectCurrentPath)
	second, err := CaptureSpark(context.Background(), root, PathResolver{StateHome: stateHome}, SparkCaptureOptions{
		Text: "Repeat Spark",
	})
	if err != nil {
		t.Fatalf("CaptureSpark() second error = %v", err)
	}
	if second.Spark.Alias != "SPARK-repeat-spark-2" {
		t.Fatalf("second alias = %q, want collision suffix", second.Spark.Alias)
	}

	sparks, err := ListSparks(context.Background(), root, PathResolver{StateHome: stateHome}, SparkListOptions{})
	if err != nil {
		t.Fatalf("ListSparks() error = %v", err)
	}
	if sparks.Sparks["SPARK-repeat-spark"].Status != "open" || sparks.Sparks["SPARK-repeat-spark"].Scope != "architecture" {
		t.Fatalf("sparks = %#v, want captured spark visible in default list", sparks.Sparks)
	}
	assertSparkProjectContext(t, root, sparks.ContractVersion, sparks.DatabaseScope, sparks.DatabasePath, sparks.ProjectID, sparks.ProjectName, sparks.ProjectCurrentPath)
	trace, err := Trace(context.Background(), root, PathResolver{StateHome: stateHome}, "SPARK-repeat-spark")
	if err != nil {
		t.Fatalf("Trace() error = %v", err)
	}
	if trace.Entity.Status != "open" || trace.Entity.Alias != "SPARK-repeat-spark" {
		t.Fatalf("trace entity = %#v, want captured open spark", trace.Entity)
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
WHERE project_id = ? AND entity_kind = 'spark' AND event_type = 'status_changed' AND from_status IS NULL AND to_status = 'open'
`, projectIDForTest(t, store, root)).Scan(&events)
	if err != nil {
		t.Fatalf("count capture events error = %v", err)
	}
	if events != 2 {
		t.Fatalf("events = %d, want one status event per captured spark", events)
	}
}

func assertSparkProjectContext(t *testing.T, root project.Root, contractVersion int, databaseScope string, databasePath string, projectID string, projectName string, projectCurrentPath string) {
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
