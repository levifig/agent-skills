package state

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/levifig/loaf/internal/project"
)

func TestCaptureSparkWritesEventFactNotSideEvent(t *testing.T) {
	requireGit(t)
	repo := initGitRepo(t)
	root, err := project.ResolveRoot(repo)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	resolver := PathResolver{StateHome: t.TempDir()}
	status, err := Initialize(context.Background(), root, resolver)
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	store, err := OpenStore(status.DatabasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()

	result, err := store.CaptureSpark(context.Background(), root, SparkCaptureOptions{Text: "event fact spark", Scope: "substrate"})
	if err != nil {
		t.Fatalf("CaptureSpark() error = %v", err)
	}
	if result.EventID == "" {
		t.Fatal("CaptureSpark EventID is empty")
	}
	var factKind string
	if err := store.db.QueryRowContext(context.Background(), `SELECT kind FROM facts WHERE id = ?`, result.EventID).Scan(&factKind); err != nil {
		t.Fatalf("read spark fact: %v", err)
	}
	if factKind != FactKindSparkCaptured {
		t.Fatalf("fact kind = %q, want %s", factKind, FactKindSparkCaptured)
	}
	var eventCount int
	if err := store.db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM events WHERE entity_kind = 'spark' AND entity_id = ?`, result.Spark.ID).Scan(&eventCount); err != nil {
		t.Fatalf("count spark events: %v", err)
	}
	if eventCount != 0 {
		t.Fatalf("spark events = %d, want 0 retired side-log writes", eventCount)
	}
	parity, err := InspectMutableCoreFactParity(context.Background(), store)
	if err != nil {
		t.Fatalf("InspectMutableCoreFactParity() error = %v", err)
	}
	if !parity.Ready || !parity.Sparks.Ready || parity.Sparks.ProjectionRows != 1 {
		t.Fatalf("spark parity = %#v, want ready", parity.Sparks)
	}
}

func TestSparkStatusFactsFoldLatestEventWins(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	resolver := PathResolver{StateHome: t.TempDir()}
	status, err := Initialize(ctx, root, resolver)
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	store, err := OpenStore(status.DatabasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()
	projectID, err := store.projectID(ctx, root)
	if err != nil {
		t.Fatalf("projectID() error = %v", err)
	}
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx() error = %v", err)
	}
	now := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	captured := CoreEventPayload{SubjectKind: "spark", SubjectID: "spark-race", Text: "race", Status: "open", CreatedAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339)}
	if _, err := appendCoreEventFactTx(ctx, tx, projectID, FactKindSparkCaptured, "fact-spark-open", captured, now, "env-a"); err != nil {
		tx.Rollback()
		t.Fatalf("append captured: %v", err)
	}
	archived := CoreEventPayload{SubjectKind: "spark", SubjectID: "spark-race", Status: LifecycleStatusDone, CreatedAt: now.Add(time.Minute).Format(time.RFC3339), UpdatedAt: now.Add(time.Minute).Format(time.RFC3339)}
	if _, err := appendCoreEventFactTx(ctx, tx, projectID, FactKindSparkArchived, "fact-spark-done", archived, now.Add(time.Minute), "env-b"); err != nil {
		tx.Rollback()
		t.Fatalf("append archived: %v", err)
	}
	promoted := CoreEventPayload{SubjectKind: "spark", SubjectID: "spark-race", Status: "open", RelatedKind: "idea", RelatedID: "idea-1", UpdatedAt: now.Add(30 * time.Second).Format(time.RFC3339)}
	if _, err := appendCoreEventFactTx(ctx, tx, projectID, FactKindSparkPromoted, "fact-spark-promoted", promoted, now.Add(30*time.Second), "env-a"); err != nil {
		tx.Rollback()
		t.Fatalf("append promoted: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
	folded, err := foldSparkFacts(ctx, store, projectID)
	if err != nil {
		t.Fatalf("foldSparkFacts() error = %v", err)
	}
	got := folded["spark-race"].Payload
	if got.Status != LifecycleStatusDone || got.Text != "race" {
		t.Fatalf("folded spark = %#v, want latest-wins done with captured text", got)
	}
}

func TestMutableCoreMigrationReplaysEventsAndBirthsOnDivergence(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	dbPath := filepath.Join(t.TempDir(), "state.sqlite")
	store, err := OpenStore(dbPath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()
	migrations := SchemaMigrations()
	if err := ApplyMigrations(ctx, store.db, migrations[:len(migrations)-1]); err != nil {
		t.Fatalf("ApplyMigrations(through 24) error = %v", err)
	}
	if err := store.UpsertProject(ctx, root); err != nil {
		t.Fatalf("UpsertProject() error = %v", err)
	}
	projectID, err := store.projectID(ctx, root)
	if err != nil {
		t.Fatalf("projectID() error = %v", err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	mustExec(t, store, `INSERT INTO sparks (id, project_id, scope, status, text, source_id, created_at, updated_at) VALUES (?, ?, 'core', 'done', 'migrated spark', NULL, ?, ?)`, "spark-legacy", projectID, now, now)
	mustExec(t, store, `INSERT INTO events (id, project_id, entity_kind, entity_id, event_type, from_status, to_status, note, created_at, updated_at) VALUES (?, ?, 'spark', ?, 'status_changed', NULL, 'open', 'legacy capture', ?, ?)`, "event-spark-open", projectID, "spark-legacy", now, now)
	mustExec(t, store, `INSERT INTO ideas (id, project_id, title, status, body_source_id, created_at, updated_at) VALUES (?, ?, 'migrated idea', 'open', NULL, ?, ?)`, "idea-legacy", projectID, now, now)
	mustExec(t, store, `INSERT INTO handoffs (id, project_id, session_id, task_id, title, status, body_source_id, created_at, updated_at) VALUES (?, ?, NULL, NULL, 'migrated handoff', 'draft', NULL, ?, ?)`, "handoff-legacy", projectID, now, now)
	mustExec(t, store, `INSERT INTO releases (id, project_id, version, tag, tagged_commit, notes, created_at, updated_at) VALUES (?, ?, '0.3.1', 'v0.3.1', 'abc1234', 'notes', ?, ?)`, "release-legacy", projectID, now, now)

	if err := ApplyMigrations(ctx, store.db, migrations[len(migrations)-1:]); err != nil {
		t.Fatalf("ApplyMigrations(25) error = %v", err)
	}

	var sparkFacts int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM facts WHERE kind IN (?, ?, ?) AND payload LIKE ?`, FactKindSparkCaptured, FactKindSparkPromoted, FactKindSparkArchived, "%spark-legacy%").Scan(&sparkFacts); err != nil {
		t.Fatalf("count spark facts: %v", err)
	}
	if sparkFacts < 2 {
		t.Fatalf("spark facts = %d, want replayed event plus birth fact", sparkFacts)
	}
	var discrepancies int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM fact_replay_discrepancies WHERE entity_id = ?`, "spark-legacy").Scan(&discrepancies); err != nil {
		t.Fatalf("count discrepancies: %v", err)
	}
	if discrepancies == 0 {
		t.Fatal("expected logged discrepancy for diverged spark fold")
	}
	parity, err := InspectMutableCoreFactParity(ctx, store)
	if err != nil {
		t.Fatalf("InspectMutableCoreFactParity() error = %v", err)
	}
	if !parity.Ready {
		t.Fatalf("parity after migration = %#v, want ready", parity)
	}
	folded, err := foldSparkFacts(ctx, store, projectID)
	if err != nil {
		t.Fatalf("foldSparkFacts() error = %v", err)
	}
	if folded["spark-legacy"].Payload.Status != "done" || folded["spark-legacy"].Payload.Text != "migrated spark" {
		t.Fatalf("folded spark = %#v, want birth snapshot", folded["spark-legacy"].Payload)
	}
}

func TestIdeaAndReleaseWritersUseFactChokepoint(t *testing.T) {
	requireGit(t)
	repo := initGitRepo(t)
	root, err := project.ResolveRoot(repo)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	resolver := PathResolver{StateHome: t.TempDir()}
	status, err := Initialize(context.Background(), root, resolver)
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	store, err := OpenStore(status.DatabasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()

	idea, err := store.CaptureIdea(context.Background(), root, IdeaCaptureOptions{Title: "event fact idea"})
	if err != nil {
		t.Fatalf("CaptureIdea() error = %v", err)
	}
	var ideaKind string
	if err := store.db.QueryRowContext(context.Background(), `SELECT kind FROM facts WHERE id = ?`, idea.EventID).Scan(&ideaKind); err != nil {
		t.Fatalf("read idea fact: %v", err)
	}
	if ideaKind != FactKindIdeaCreated {
		t.Fatalf("idea fact kind = %q, want %s", ideaKind, FactKindIdeaCreated)
	}

	handoff, err := store.CreateArtifactEntity(context.Background(), root, ArtifactEntityCreateOptions{
		Kind:  "handoff",
		Title: "event fact handoff",
		Body:  "handoff body",
	})
	if err != nil {
		t.Fatalf("CreateArtifactEntity(handoff) error = %v", err)
	}
	var handoffKind string
	if err := store.db.QueryRowContext(context.Background(), `SELECT kind FROM facts WHERE id = ?`, handoff.EventID).Scan(&handoffKind); err != nil {
		t.Fatalf("read handoff fact: %v", err)
	}
	if handoffKind != FactKindHandoffRecorded {
		t.Fatalf("handoff fact kind = %q, want %s", handoffKind, FactKindHandoffRecorded)
	}

	release, err := store.RecordRelease(context.Background(), root, RecordReleaseOptions{
		Version:      "0.4.0",
		Tag:          "v0.4.0",
		TaggedCommit: "deadbeef",
		Notes:        "event fact release",
	})
	if err != nil {
		t.Fatalf("RecordRelease() error = %v", err)
	}
	var releaseFacts int
	if err := store.db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM facts WHERE kind = ? AND payload LIKE ?`, FactKindReleaseRecorded, "%"+release.ID+"%").Scan(&releaseFacts); err != nil {
		t.Fatalf("count release facts: %v", err)
	}
	if releaseFacts != 1 {
		t.Fatalf("release facts = %d, want 1", releaseFacts)
	}

	parity, err := InspectMutableCoreFactParity(context.Background(), store)
	if err != nil {
		t.Fatalf("InspectMutableCoreFactParity() error = %v", err)
	}
	if !parity.Ready {
		t.Fatalf("parity = %#v, want ready", parity)
	}
}

func TestRebuildMutableCoreProjectionsFromFacts(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	resolver := PathResolver{StateHome: t.TempDir()}
	status, err := Initialize(ctx, root, resolver)
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	store, err := OpenStore(status.DatabasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()
	if _, err := store.CaptureSpark(ctx, root, SparkCaptureOptions{Text: "rebuild spark"}); err != nil {
		t.Fatalf("CaptureSpark() error = %v", err)
	}
	projectID, err := store.projectID(ctx, root)
	if err != nil {
		t.Fatalf("projectID() error = %v", err)
	}
	mustExec(t, store, `DELETE FROM sparks WHERE project_id = ?`, projectID)
	count, err := RebuildMutableCoreProjectionsForProject(ctx, store, projectID)
	if err != nil {
		t.Fatalf("RebuildMutableCoreProjectionsForProject() error = %v", err)
	}
	if count == 0 {
		t.Fatal("rebuild count = 0, want restored spark")
	}
	parity, err := InspectMutableCoreFactParity(ctx, store)
	if err != nil {
		t.Fatalf("InspectMutableCoreFactParity() error = %v", err)
	}
	if !parity.Sparks.Ready || parity.Sparks.ProjectionRows != 1 {
		t.Fatalf("spark parity after rebuild = %#v", parity.Sparks)
	}
}
