package state

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/levifig/loaf/internal/project"
)

// markdownImportCanonicalTables is the Decision 4 / V4 business-state surface.
var markdownImportCanonicalTables = []string{
	"specs",
	"tasks",
	"ideas",
	"brainstorms",
	"shaping_drafts",
	"reports",
	"sparks",
	"journal_entries",
	"journal_origins",
	"aliases",
	"relationships",
	"sources",
	"artifact_bodies",
}

type gridsightFixture struct {
	root         project.Root
	resolver     PathResolver
	databasePath string
	projectID    string
	byMessage    map[string]string
}

func TestSimulateApplyImportReportParity(t *testing.T) {
	ctx := context.Background()
	fx := seedGridsightShapedFixture(t)

	sim, err := SimulateMarkdownMigration(ctx, fx.root, fx.resolver)
	if err != nil {
		t.Fatalf("SimulateMarkdownMigration() error = %v", err)
	}
	if sim.Mode != MarkdownMigrationModeSimulation || sim.ImportReport == nil {
		t.Fatalf("simulate mode/report = %q/%#v", sim.Mode, sim.ImportReport)
	}

	apply, err := ApplyMarkdownMigration(ctx, fx.root, fx.resolver)
	if err != nil {
		t.Fatalf("ApplyMarkdownMigration() error = %v", err)
	}
	if apply.ImportReport == nil {
		t.Fatal("apply ImportReport is nil")
	}

	assertImportReportsEqual(t, *sim.ImportReport, *apply.ImportReport)
	if apply.ImportReport.ReclaimedOrigins != 1 {
		t.Fatalf("ReclaimedOrigins = %d, want 1", apply.ImportReport.ReclaimedOrigins)
	}
	if len(apply.ImportReport.SkippedEntries) != 4 {
		t.Fatalf("SkippedEntries = %#v, want 4", apply.ImportReport.SkippedEntries)
	}
	store := openFixtureStore(t, fx)
	defer store.Close()
	assertEntityStatus(t, store, "specs", fx.projectID, "spec", "SPEC-007", "archived")
}

func TestSecondApplyBusinessStateIdempotence(t *testing.T) {
	ctx := context.Background()
	fx := seedGridsightShapedFixture(t)

	first, err := ApplyMarkdownMigration(ctx, fx.root, fx.resolver)
	if err != nil {
		t.Fatalf("first ApplyMarkdownMigration() error = %v", err)
	}
	if first.ImportReport == nil || first.ImportReport.ReclaimedOrigins != 1 {
		t.Fatalf("first report = %#v, want reclaimed=1", first.ImportReport)
	}

	store := openFixtureStore(t, fx)
	defer store.Close()
	before := logicalBusinessDump(t, store)
	assertJournalSearchRebuildParity(t, store)
	assertArtifactSearchBodyParity(t, store)

	second, err := ApplyMarkdownMigration(ctx, fx.root, fx.resolver)
	if err != nil {
		t.Fatalf("second ApplyMarkdownMigration() error = %v", err)
	}
	if second.ImportReport == nil {
		t.Fatal("second ImportReport is nil")
	}
	if second.ImportReport.ReclaimedOrigins != 0 {
		t.Fatalf("second ReclaimedOrigins = %d, want 0", second.ImportReport.ReclaimedOrigins)
	}
	assertImportSkippedStable(t, first.ImportReport.SkippedEntries, second.ImportReport.SkippedEntries)

	after := logicalBusinessDump(t, store)
	if before != after {
		t.Fatalf("canonical business dump drifted after second apply\nbefore:\n%s\nafter:\n%s", before, after)
	}
	assertJournalSearchRebuildParity(t, store)
	assertArtifactSearchBodyParity(t, store)
}

func TestImportStatusMatrixRemainingCells(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	stateHome := t.TempDir()
	resolver := PathResolver{StateHome: stateHome}

	// TASKS.json wins over conflicting frontmatter for tasks (spec covered elsewhere).
	writeAgentsFile(t, root.Path(), "tasks/TASK-030-conflict.md", `---
id: TASK-030
title: Conflict Task
status: todo
---
# Conflict Task
`)
	// No-opinion frontmatter over a real stored status must keep + not diverge.
	writeAgentsFile(t, root.Path(), "ideas/20260724-no-opinion.md", `---
id: idea-no-opinion
title: No Opinion
---
# No Opinion
`)
	// OOV must never fill a stored unknown.
	writeAgentsFile(t, root.Path(), "tasks/TASK-031-oov-unknown.md", `---
id: TASK-031
title: OOV Over Unknown
status: accepted
---
# OOV Over Unknown
`)
	// Pre-existing unknown idea remains fillable by normalized incoming status.
	writeAgentsFile(t, root.Path(), "ideas/20260724-fillable.md", `---
id: idea-fillable
title: Fillable
status: resolved
---
# Fillable
`)
	writeAgentsFile(t, root.Path(), "TASKS.json", `{
  "tasks": {
    "TASK-030": {"title": "Conflict Task", "status": "done", "file": "TASK-030-conflict.md"},
    "TASK-031": {"title": "OOV Over Unknown", "status": "accepted", "file": "TASK-031-oov-unknown.md"}
  }
}`)

	first, err := ApplyMarkdownMigration(ctx, root, resolver)
	if err != nil {
		t.Fatalf("ApplyMarkdownMigration() error = %v", err)
	}
	store := openStoreAt(t, first.DatabasePath)
	defer store.Close()
	assertEntityStatus(t, store, "tasks", first.ProjectID, "task", "TASK-030", "done")

	ideaNoOpinion := stableMigrationID("idea", first.ProjectID, "idea-no-opinion")
	ideaFillable := stableMigrationID("idea", first.ProjectID, "idea-fillable")
	taskOOV := stableMigrationID("task", first.ProjectID, "TASK-031")
	if _, err := store.db.ExecContext(ctx, `UPDATE ideas SET status = 'archived' WHERE id = ?`, ideaNoOpinion); err != nil {
		t.Fatalf("seed archived idea: %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `UPDATE ideas SET status = 'unknown' WHERE id = ?`, ideaFillable); err != nil {
		t.Fatalf("seed unknown idea: %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `UPDATE tasks SET status = 'unknown' WHERE id = ?`, taskOOV); err != nil {
		t.Fatalf("seed unknown task: %v", err)
	}

	second, err := ApplyMarkdownMigration(ctx, root, resolver)
	if err != nil {
		t.Fatalf("second apply error = %v", err)
	}
	assertEntityStatus(t, store, "ideas", first.ProjectID, "idea", "idea-no-opinion", "archived")
	assertEntityStatus(t, store, "ideas", first.ProjectID, "idea", "idea-fillable", "done")
	assertEntityStatus(t, store, "tasks", first.ProjectID, "task", "TASK-031", "unknown")

	if second.ImportReport == nil {
		t.Fatal("expected import report")
	}
	for _, d := range second.ImportReport.StatusDivergences {
		if d.EntityID == ideaNoOpinion {
			t.Fatalf("no-opinion over real status must not diverge: %#v", d)
		}
	}
	foundOOV := false
	for _, warning := range second.ImportReport.Warnings {
		if strings.Contains(warning, "accepted") && strings.Contains(warning, taskOOV) {
			foundOOV = true
			break
		}
	}
	if !foundOOV {
		t.Fatalf("warnings = %#v, want OOV accepted for TASK-031", second.ImportReport.Warnings)
	}
}

func TestSimulateApplyFTSFailureParity(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	stateHome := t.TempDir()
	resolver := PathResolver{StateHome: stateHome}
	writeAgentsFile(t, root.Path(), "sessions/fts-fail.md", `---
harness_session_id: hsid-fts
---
[2026-07-10 10:00] decision(fts): original content
`)
	result, err := ApplyMarkdownMigration(ctx, root, resolver)
	if err != nil {
		t.Fatalf("ApplyMarkdownMigration() error = %v", err)
	}

	store, err := OpenStore(result.DatabasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	var journalID, projectID, entryType, scope, message, harnessSessionID string
	var rowID int64
	if err := store.db.QueryRowContext(ctx, `
SELECT rowid, id, project_id, entry_type, COALESCE(scope, ''), message, COALESCE(harness_session_id, '')
FROM journal_entries`).Scan(&rowID, &journalID, &projectID, &entryType, &scope, &message, &harnessSessionID); err != nil {
		t.Fatalf("read journal row: %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `DROP TABLE journal_search`); err != nil {
		t.Fatalf("drop journal_search: %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `
CREATE TABLE journal_search (
  rowid INTEGER PRIMARY KEY,
  project_id TEXT NOT NULL,
  journal_entry_id TEXT NOT NULL,
  harness_session_id TEXT,
  entry_type TEXT NOT NULL,
  scope TEXT NOT NULL,
  message TEXT NOT NULL CHECK(message = 'original content')
)`); err != nil {
		t.Fatalf("create failing journal_search: %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `
INSERT INTO journal_search(rowid, project_id, journal_entry_id, harness_session_id, entry_type, scope, message)
VALUES (?, ?, ?, ?, ?, ?, ?)
`, rowID, projectID, journalID, harnessSessionID, entryType, scope, message); err != nil {
		t.Fatalf("seed journal_search: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	writeAgentsFile(t, root.Path(), "sessions/fts-fail.md", `---
harness_session_id: hsid-fts
---
[2026-07-10 10:00] decision(fts): changed and should fail
`)

	before := durableSQLiteFilesSnapshot(t, result.DatabasePath)
	_, simErr := SimulateMarkdownMigration(ctx, root, resolver)
	afterSim := durableSQLiteFilesSnapshot(t, result.DatabasePath)
	for path, contents := range before {
		if string(contents) != string(afterSim[path]) {
			t.Fatalf("failed simulate mutated durable bytes for %s", path)
		}
	}

	_, applyErr := ApplyMarkdownMigration(ctx, root, resolver)
	if simErr == nil || applyErr == nil {
		t.Fatalf("expected FTS failures, got sim=%v apply=%v", simErr, applyErr)
	}
	const needle = "rebuild journal search after markdown import"
	if !strings.Contains(simErr.Error(), needle) || !strings.Contains(applyErr.Error(), needle) {
		t.Fatalf("error class mismatch:\n  sim=%v\n  apply=%v", simErr, applyErr)
	}
	simUnderlying := innermostErrorMessage(simErr)
	applyUnderlying := innermostErrorMessage(applyErr)
	if simUnderlying != applyUnderlying {
		t.Fatalf("underlying SQLite errors differ:\n  sim=%q\n  apply=%q", simUnderlying, applyUnderlying)
	}
}

func innermostErrorMessage(err error) string {
	for {
		unwrapped := errors.Unwrap(err)
		if unwrapped == nil {
			return err.Error()
		}
		err = unwrapped
	}
}

func TestCreateMarkdownSimulationSnapshotUnwritableTemp(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	resolver := PathResolver{StateHome: t.TempDir()}
	status, err := Initialize(ctx, root, resolver)
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	blocked := filepath.Join(t.TempDir(), "blocked")
	if err := os.Mkdir(blocked, 0o555); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(blocked, 0o755) })
	t.Setenv("TMPDIR", blocked)

	_, cleanup, err := createMarkdownSimulationSnapshot(ctx, status.DatabasePath)
	if err == nil {
		cleanup()
		t.Fatal("createMarkdownSimulationSnapshot() error = nil, want unwritable-temp failure")
	}
	var snapErr *MarkdownSimulationSnapshotError
	if !errors.As(err, &snapErr) || snapErr.Code != MarkdownSimulationSnapshotFailedCode {
		t.Fatalf("error = %v (%T), want MarkdownSimulationSnapshotError", err, err)
	}
	entries, readErr := os.ReadDir(blocked)
	if readErr != nil {
		t.Fatalf("ReadDir() error = %v", readErr)
	}
	for _, entry := range entries {
		if strings.Contains(entry.Name(), "markdown-simulate-") || strings.Contains(entry.Name(), "loaf-markdown-simulate-") {
			t.Fatalf("partial snapshot residue left in unwritable temp: %s", entry.Name())
		}
	}
}

func TestSimulateMarkdownMigrationHonorsStateHomeAndLoafDB(t *testing.T) {
	ctx := context.Background()

	t.Run("StateHome", func(t *testing.T) {
		root := projectRoot(t)
		stateHome := t.TempDir()
		resolver := PathResolver{StateHome: stateHome}
		writeAgentsFile(t, root.Path(), "ideas/20260724-statehome.md", "# Idea\n")
		status, err := Initialize(ctx, root, resolver)
		if err != nil {
			t.Fatalf("Initialize() error = %v", err)
		}
		result, err := SimulateMarkdownMigration(ctx, root, resolver)
		if err != nil {
			t.Fatalf("SimulateMarkdownMigration() error = %v", err)
		}
		if result.Mode != MarkdownMigrationModeSimulation {
			t.Fatalf("Mode = %q, want simulation", result.Mode)
		}
		if result.DatabasePath != status.DatabasePath {
			t.Fatalf("DatabasePath = %q, want StateHome path %q", result.DatabasePath, status.DatabasePath)
		}
		if !strings.HasPrefix(result.DatabasePath, stateHome+string(filepath.Separator)) {
			t.Fatalf("DatabasePath = %q, want under StateHome %q", result.DatabasePath, stateHome)
		}
	})

	t.Run("LOAF_DB", func(t *testing.T) {
		root := projectRoot(t)
		dbPath := filepath.Join(t.TempDir(), "isolated.sqlite")
		t.Setenv("LOAF_DB", dbPath)
		writeAgentsFile(t, root.Path(), "ideas/20260724-loafdb.md", "# Idea\n")
		status, err := Initialize(ctx, root, PathResolver{})
		if err != nil {
			t.Fatalf("Initialize() error = %v", err)
		}
		if status.DatabasePath != dbPath {
			t.Fatalf("Initialize DatabasePath = %q, want LOAF_DB %q", status.DatabasePath, dbPath)
		}
		result, err := SimulateMarkdownMigration(ctx, root, PathResolver{})
		if err != nil {
			t.Fatalf("SimulateMarkdownMigration() error = %v", err)
		}
		if result.Mode != MarkdownMigrationModeSimulation {
			t.Fatalf("Mode = %q, want simulation", result.Mode)
		}
		if result.DatabasePath != dbPath {
			t.Fatalf("DatabasePath = %q, want LOAF_DB %q", result.DatabasePath, dbPath)
		}
	})
}

func seedGridsightShapedFixture(t *testing.T) gridsightFixture {
	t.Helper()
	ctx := context.Background()
	root := projectRoot(t)
	stateHome := t.TempDir()
	resolver := PathResolver{StateHome: stateHome}

	writeAgentsFile(t, root.Path(), "sessions/gridsight.md", `---
branch: feat/gridsight
harness_session_id: hsid-gridsight
---
[2026-07-10 10:00] decision(reclaim): clean fingerprint
[2026-07-10 10:01] decision(dirty): dirty origin
[2026-07-10 10:02] decision(v2): envelope v2
[2026-07-10 10:03] decision(mismatch): copy mismatch
[2026-07-10 10:04] decision(manual): foreign mechanism
[2026-07-10 10:05] spark(scope): rename-me captured to .agents/ideas/20260724-a.md
`)
	writeAgentsFile(t, root.Path(), "ideas/20260724-a.md", `---
id: idea-a
title: Idea A
---
# Idea A
`)
	writeAgentsFile(t, root.Path(), "ideas/20260724-b.md", `---
id: idea-b
title: Idea B
---
# Idea B
`)
	writeAgentsFile(t, root.Path(), "specs/SPEC-007-archived.md", `---
id: SPEC-007
title: Archived Spec
status: complete
---
# Archived Spec
`)
	writeAgentsFile(t, root.Path(), "TASKS.json", `{
  "specs": {
    "SPEC-007": {"title": "Archived Spec", "status": "complete", "file": "SPEC-007-archived.md"}
  },
  "tasks": {}
}`)

	result, err := ApplyMarkdownMigration(ctx, root, resolver)
	if err != nil {
		t.Fatalf("seed ApplyMarkdownMigration() error = %v", err)
	}
	store, err := OpenStore(result.DatabasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()

	byMessage := map[string]string{}
	rows, err := store.db.QueryContext(ctx, `SELECT id, message FROM journal_entries`)
	if err != nil {
		t.Fatalf("query journals: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id, message string
		if err := rows.Scan(&id, &message); err != nil {
			t.Fatalf("scan journal: %v", err)
		}
		byMessage[message] = id
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("journal rows: %v", err)
	}

	seedOriginFingerprint(t, store, byMessage["clean fingerprint"])
	if _, err := store.db.ExecContext(ctx, `UPDATE journal_origins SET capture_mechanism = 'unknown', dirty = 1, source_event = NULL WHERE journal_entry_id = ?`, byMessage["dirty origin"]); err != nil {
		t.Fatalf("seed dirty: %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `UPDATE journal_origins SET capture_mechanism = 'unknown', envelope_version = 2, source_event = NULL WHERE journal_entry_id = ?`, byMessage["envelope v2"]); err != nil {
		t.Fatalf("seed v2: %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `UPDATE journal_origins SET capture_mechanism = 'unknown', branch = 'other-branch', source_event = NULL WHERE journal_entry_id = ?`, byMessage["copy mismatch"]); err != nil {
		t.Fatalf("seed mismatch: %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `UPDATE journal_origins SET capture_mechanism = 'manual', source_event = 'manual' WHERE journal_entry_id = ?`, byMessage["foreign mechanism"]); err != nil {
		t.Fatalf("seed manual: %v", err)
	}

	specID := stableMigrationID("spec", result.ProjectID, "SPEC-007")
	if _, err := store.db.ExecContext(ctx, `UPDATE specs SET status = 'archived' WHERE id = ?`, specID); err != nil {
		t.Fatalf("seed archived SPEC-007: %v", err)
	}

	// Retarget spark so reclaim/apply also exercises promoted_to refresh.
	writeAgentsFile(t, root.Path(), "sessions/gridsight.md", `---
branch: feat/gridsight
harness_session_id: hsid-gridsight
---
[2026-07-10 10:00] decision(reclaim): clean fingerprint
[2026-07-10 10:01] decision(dirty): dirty origin
[2026-07-10 10:02] decision(v2): envelope v2
[2026-07-10 10:03] decision(mismatch): copy mismatch
[2026-07-10 10:04] decision(manual): foreign mechanism
[2026-07-10 10:05] spark(scope): rename-me captured to .agents/ideas/20260724-b.md
`)

	return gridsightFixture{
		root:         root,
		resolver:     resolver,
		databasePath: result.DatabasePath,
		projectID:    result.ProjectID,
		byMessage:    byMessage,
	}
}

func seedOriginFingerprint(t *testing.T, store *Store, journalID string) {
	t.Helper()
	if _, err := store.db.ExecContext(context.Background(), `
UPDATE journal_origins SET
  capture_mechanism = 'unknown',
  envelope_version = 1,
  observed_harness = NULL,
  observed_harness_version = NULL,
  agent_id = NULL,
  source_event = NULL,
  head = NULL,
  change_path = NULL,
  change_sha256 = NULL,
  dirty = NULL,
  reconstructable = NULL,
  durable_result_kind = NULL,
  durable_result_id = NULL,
  harness_session_id = (SELECT harness_session_id FROM journal_entries WHERE id = ?),
  branch = (SELECT observed_branch FROM journal_entries WHERE id = ?),
  worktree = (SELECT observed_worktree FROM journal_entries WHERE id = ?),
  created_at = (SELECT created_at FROM journal_entries WHERE id = ?)
WHERE journal_entry_id = ?
`, journalID, journalID, journalID, journalID, journalID); err != nil {
		t.Fatalf("seed fingerprint for %s: %v", journalID, err)
	}
}

func openFixtureStore(t *testing.T, fx gridsightFixture) *Store {
	t.Helper()
	return openStoreAt(t, fx.databasePath)
}

func openStoreAt(t *testing.T, databasePath string) *Store {
	t.Helper()
	store, err := OpenStore(databasePath)
	if err != nil {
		t.Fatalf("OpenStore(%s) error = %v", databasePath, err)
	}
	return store
}

func assertImportReportsEqual(t *testing.T, a, b ImportReport) {
	t.Helper()
	normalize := func(r ImportReport) ImportReport {
		sort.SliceStable(r.SkippedEntries, func(i, j int) bool {
			if r.SkippedEntries[i].JournalEntryID == r.SkippedEntries[j].JournalEntryID {
				return r.SkippedEntries[i].CaptureMechanism < r.SkippedEntries[j].CaptureMechanism
			}
			return r.SkippedEntries[i].JournalEntryID < r.SkippedEntries[j].JournalEntryID
		})
		sort.SliceStable(r.StatusDivergences, func(i, j int) bool {
			if r.StatusDivergences[i].EntityID == r.StatusDivergences[j].EntityID {
				return r.StatusDivergences[i].IncomingStatus < r.StatusDivergences[j].IncomingStatus
			}
			return r.StatusDivergences[i].EntityID < r.StatusDivergences[j].EntityID
		})
		sort.Strings(r.Warnings)
		return r
	}
	gotA, gotB := normalize(a), normalize(b)
	if !reflect.DeepEqual(gotA, gotB) {
		aj, _ := json.MarshalIndent(gotA, "", "  ")
		bj, _ := json.MarshalIndent(gotB, "", "  ")
		t.Fatalf("ImportReport mismatch\nsimulate:\n%s\napply:\n%s", aj, bj)
	}
}

func assertImportSkippedStable(t *testing.T, first, second []ImportSkippedEntry) {
	t.Helper()
	normalize := func(entries []ImportSkippedEntry) []ImportSkippedEntry {
		out := append([]ImportSkippedEntry(nil), entries...)
		sort.SliceStable(out, func(i, j int) bool {
			return out[i].JournalEntryID < out[j].JournalEntryID
		})
		return out
	}
	if !reflect.DeepEqual(normalize(first), normalize(second)) {
		t.Fatalf("skipped list drifted: first=%#v second=%#v", first, second)
	}
}

func logicalBusinessDump(t *testing.T, store *Store) string {
	t.Helper()
	ctx := context.Background()
	var b strings.Builder
	for _, table := range markdownImportCanonicalTables {
		columns := tableColumnsExcludingTimestamps(t, store, table)
		if len(columns) == 0 {
			t.Fatalf("table %s has no dumpable columns", table)
		}
		query := fmt.Sprintf(
			`SELECT %s FROM %s ORDER BY %s`,
			strings.Join(quoteIdentList(columns), ", "),
			table,
			strings.Join(quoteIdentList(columns), ", "),
		)
		rows, err := store.db.QueryContext(ctx, query)
		if err != nil {
			t.Fatalf("query %s: %v", table, err)
		}
		fmt.Fprintf(&b, "# %s\n", table)
		for rows.Next() {
			raw := make([]any, len(columns))
			ptrs := make([]any, len(columns))
			for i := range raw {
				ptrs[i] = &raw[i]
			}
			if err := rows.Scan(ptrs...); err != nil {
				rows.Close()
				t.Fatalf("scan %s: %v", table, err)
			}
			parts := make([]string, len(columns))
			for i, value := range raw {
				parts[i] = fmt.Sprintf("%s=%s", columns[i], sqlValueString(value))
			}
			b.WriteString(strings.Join(parts, "\t"))
			b.WriteByte('\n')
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			t.Fatalf("rows %s: %v", table, err)
		}
	}
	return b.String()
}

func tableColumnsExcludingTimestamps(t *testing.T, store *Store, table string) []string {
	t.Helper()
	rows, err := store.db.Query(`SELECT name FROM pragma_table_info(?) ORDER BY cid`, table)
	if err != nil {
		t.Fatalf("pragma_table_info(%s): %v", table, err)
	}
	defer rows.Close()
	var columns []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan column name: %v", err)
		}
		switch name {
		case "created_at", "updated_at", "imported_at":
			continue
		default:
			columns = append(columns, name)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("pragma rows: %v", err)
	}
	return columns
}

func quoteIdentList(names []string) []string {
	out := make([]string, len(names))
	for i, name := range names {
		out[i] = `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
	}
	return out
}

func sqlValueString(value any) string {
	switch v := value.(type) {
	case nil:
		return "<NULL>"
	case []byte:
		return string(v)
	default:
		return fmt.Sprint(v)
	}
}

func assertJournalSearchRebuildParity(t *testing.T, store *Store) {
	t.Helper()
	ctx := context.Background()
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx() error = %v", err)
	}
	defer tx.Rollback()
	if _, err := rebuildAndVerifyJournalSearch(ctx, tx); err != nil {
		t.Fatalf("rebuildAndVerifyJournalSearch() error = %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
	parity, err := InspectJournalSearchParity(ctx, store)
	if err != nil {
		t.Fatalf("InspectJournalSearchParity() error = %v", err)
	}
	if !parity.Ready {
		t.Fatalf("journal_search parity = %#v, want ready", parity)
	}
}

func assertArtifactSearchBodyParity(t *testing.T, store *Store) {
	t.Helper()
	ctx := context.Background()
	var bodies int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM artifact_bodies`).Scan(&bodies); err != nil {
		t.Fatalf("count artifact_bodies: %v", err)
	}
	var searchRows int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM artifact_search`).Scan(&searchRows); err != nil {
		t.Fatalf("count artifact_search: %v", err)
	}
	if bodies != searchRows {
		t.Fatalf("artifact_search rows = %d, want one per body (%d)", searchRows, bodies)
	}
	rows, err := store.db.QueryContext(ctx, `
SELECT b.project_id, b.entity_kind, b.entity_id, b.body_kind, b.content,
       (SELECT COUNT(*) FROM artifact_search AS s
        WHERE s.project_id = b.project_id
          AND s.entity_kind = b.entity_kind
          AND s.entity_id = b.entity_id
          AND s.body_kind = b.body_kind
          AND s.content = b.content)
FROM artifact_bodies AS b
`)
	if err != nil {
		t.Fatalf("query artifact body/search parity: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var projectID, entityKind, entityID, bodyKind, content string
		var matches int
		if err := rows.Scan(&projectID, &entityKind, &entityID, &bodyKind, &content, &matches); err != nil {
			t.Fatalf("scan artifact parity: %v", err)
		}
		if matches != 1 {
			t.Fatalf("artifact_search matches for %s/%s/%s = %d, want 1", entityKind, entityID, bodyKind, matches)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("artifact parity rows: %v", err)
	}
}
