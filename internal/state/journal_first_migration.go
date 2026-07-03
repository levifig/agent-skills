package state

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/levifig/loaf/internal/project"
)

const (
	// JournalFirstMigrationActionDryRun previews the migration against a copy.
	JournalFirstMigrationActionDryRun = "dry-run"
	// JournalFirstMigrationActionApply runs the migration against the live database.
	JournalFirstMigrationActionApply = "apply"
)

// JournalFirstMigrationResult reports the outcome of a journal-first (SPEC-056)
// migration run. Counts are measured across the whole global database because
// the migration is schema-wide and touches every project's rows.
type JournalFirstMigrationResult struct {
	ContractVersion    int    `json:"contract_version"`
	DatabaseScope      string `json:"database_scope"`
	DatabasePath       string `json:"database_path"`
	ProjectID          string `json:"project_id,omitempty"`
	ProjectName        string `json:"project_name,omitempty"`
	ProjectCurrentPath string `json:"project_current_path,omitempty"`
	Action             string `json:"action"`
	Applied            bool   `json:"applied"`
	CopyRun            bool   `json:"copy_run"`
	BackupPath         string `json:"backup_path,omitempty"`
	SchemaVersion      int    `json:"schema_version"`

	JournalEntriesBefore   int `json:"journal_entries_before"`
	JournalEntriesAfter    int `json:"journal_entries_after"`
	NoiseEntriesPurged     int `json:"noise_entries_purged"`
	HarnessSessionBackfill int `json:"harness_session_ids_backfilled"`
	HandoffsRetargeted     int `json:"handoffs_retargeted"`
	SessionEventsDeleted   int `json:"session_events_deleted"`
	SessionAliasesDeleted  int `json:"session_aliases_deleted"`
	SessionsDropped        int `json:"sessions_dropped"`
	SnapshotsDropped       int `json:"snapshots_dropped"`
	JournalSearchRows      int `json:"journal_search_rows"`

	// PurgeBreakdown reports how many entry_type='session' rows each
	// machine-generated pattern family accounts for, in a stable order. The
	// counts sum to NoiseEntriesPurged. It lets --dry-run and --apply show
	// exactly which shapes the migration removes rather than a single opaque
	// total.
	PurgeBreakdown []JournalFirstPurgeFamily `json:"purge_breakdown"`
	// SessionRowsPreservedAsLegacy is the number of entry_type='session' rows
	// that did not match any machine shape and were renamed to
	// entry_type='legacy_session' (content untouched) instead of being purged.
	SessionRowsPreservedAsLegacy int `json:"session_rows_preserved_as_legacy"`
}

// JournalFirstPurgeFamily is one row of the purge breakdown: a named
// machine-generated pattern family and how many entry_type='session' rows it
// matched.
type JournalFirstPurgeFamily struct {
	Family string `json:"family"`
	Count  int    `json:"count"`
}

// journalFirstPurgeFamilies enumerates the machine-generated entry_type='session'
// shapes in stable reporting order. Each predicate mirrors exactly the
// corresponding branch of the migration SQL (0010_journal_first.sql step 2a/2b);
// the two MUST stay in lockstep. Any entry_type='session' row not matched by one
// of these is preserved as entry_type='legacy_session'.
//
// NULL-safety: scope is NULLABLE, so these predicates coalesce every scope
// reference to the empty string — matching the migration SQL exactly. Without
// that coalesce a NULL-scope row's scope = 'start' yields SQL NULL, the family
// match becomes NULL rather than false, and the dry-run purge count would
// diverge from what the migration's NOT(...) demotion actually preserves.
// Coalescing NULL scope to the empty string means such a row matches no family
// and is counted as preserved-as-legacy, exactly as the SQL demotes it.
// (message is NOT NULL, so it needs no coalesce.)
var journalFirstPurgeFamilies = []struct {
	name string
	sql  string
}{
	{"start_marker", `COALESCE(scope, '') = 'start' AND (message = '=== SESSION STARTED ===' OR message GLOB '=== SESSION STARTED === (session *)')`},
	{"resume_marker", `COALESCE(scope, '') = 'resume' AND (message = '=== SESSION RESUMED ===' OR message GLOB '=== SESSION RESUMED === (session *)')`},
	{"stop_marker", `COALESCE(scope, '') = 'stop' AND message IN ('=== SESSION STOPPED ===', '=== SESSION COMPLETE ===')`},
	{"context_cleared", `COALESCE(scope, '') = 'clear' AND message = '=== CONTEXT CLEARED ==='`},
	{"commit_summary", `COALESCE(scope, '') IN ('end', 'conclude', 'wrap') AND message LIKE 'at commit %'`},
	{"session_ended", `COALESCE(scope, '') IN ('end', 'conclude', 'wrap') AND message = 'session ended'`},
	{"end_status_marker", `COALESCE(scope, '') IN ('end', 'conclude', 'wrap') AND message IN ('closed by new conversation', 'session handed off, pending final status update')`},
	{"context_arrival", `COALESCE(scope, '') = 'context' AND message LIKE 'from commit %'`},
	{"merge_consolidated", `COALESCE(scope, '') = 'merge' AND message LIKE 'consolidated from %'`},
	{"test_fixture", `COALESCE(scope, '') = 'test' AND message = 'verify session type'`},
	{"enrich_checkpoint", `COALESCE(scope, '') = 'enrich' AND message = 'recorded native SQLite enrichment checkpoint'`},
}

// zeroJournalFirstPurgeBreakdown returns the family list with all counts zero,
// preserving the stable reporting order. Used when there is nothing to purge
// (an already-migrated database) so re-run output keeps a consistent shape.
func zeroJournalFirstPurgeBreakdown() []JournalFirstPurgeFamily {
	breakdown := make([]JournalFirstPurgeFamily, 0, len(journalFirstPurgeFamilies))
	for _, family := range journalFirstPurgeFamilies {
		breakdown = append(breakdown, JournalFirstPurgeFamily{Family: family.name, Count: 0})
	}
	return breakdown
}

// PreviewJournalFirstMigration runs the journal-first migration against a
// temporary copy of the live database and reports row counts without mutating
// live state.
func PreviewJournalFirstMigration(ctx context.Context, root project.Root, resolver PathResolver) (JournalFirstMigrationResult, error) {
	gate, err := requireJournalFirstMigrationStatus(root, resolver)
	if err != nil {
		return JournalFirstMigrationResult{}, err
	}
	source, err := OpenStoreReadOnly(gate.status.DatabasePath)
	if err != nil {
		return JournalFirstMigrationResult{}, err
	}
	defer source.Close()

	tempDir, err := os.MkdirTemp("", "loaf-journal-first-migration-*")
	if err != nil {
		return JournalFirstMigrationResult{}, fmt.Errorf("create journal-first migration temp dir: %w", err)
	}
	defer os.RemoveAll(tempDir)
	copyPath := filepath.Join(tempDir, "state.sqlite")
	if err := copySQLiteDatabase(ctx, source, copyPath, 0o600); err != nil {
		return JournalFirstMigrationResult{}, err
	}
	copyStore, err := OpenStore(copyPath)
	if err != nil {
		return JournalFirstMigrationResult{}, err
	}
	defer copyStore.Close()

	// runJournalFirstMigration applies SchemaMigrations() (1..9) plus the
	// journal-first migration (10) in one transaction, so a behind-schema copy
	// is brought current and previewed in a single pass. The live database is
	// never touched — only this temporary copy.
	result := journalFirstMigrationBaseResult(gate.status, JournalFirstMigrationActionDryRun)
	if err := runJournalFirstMigration(ctx, copyStore, &result); err != nil {
		return JournalFirstMigrationResult{}, err
	}
	result.CopyRun = true
	return result, nil
}

// ApplyJournalFirstMigration takes a mandatory pre-migration backup and then
// applies the journal-first migration to the live database.
func ApplyJournalFirstMigration(ctx context.Context, root project.Root, resolver PathResolver) (JournalFirstMigrationResult, error) {
	gate, err := requireJournalFirstMigrationStatus(root, resolver)
	if err != nil {
		return JournalFirstMigrationResult{}, err
	}
	// Behind-schema install: apply the pending non-destructive schema migrations
	// (1..9) to the live database first, so the mandatory pre-migration backup
	// below captures a schema-current (and verifiable) snapshot before the
	// destructive journal-first step runs. These migrations are purely additive
	// — they preserve every existing session/handoff/journal row — so the backup
	// remains a complete, reversible pre-journal-first checkpoint.
	if gate.pendingSchema {
		if err := applyPendingSchemaMigrations(ctx, root, resolver); err != nil {
			return JournalFirstMigrationResult{}, err
		}
		// Re-inspect now that the database is schema-current so the result
		// carries resolved project identity.
		refreshed, err := Inspect(root, resolver)
		if err != nil {
			return JournalFirstMigrationResult{}, err
		}
		gate.status = refreshed
	}

	backup, err := Backup(ctx, root, resolver)
	if err != nil {
		return JournalFirstMigrationResult{}, err
	}
	store, err := openInitializedStore(root, resolver)
	if err != nil {
		return JournalFirstMigrationResult{}, err
	}
	defer store.Close()

	result := journalFirstMigrationBaseResult(gate.status, JournalFirstMigrationActionApply)
	result.BackupPath = backup.BackupPath
	if err := runJournalFirstMigration(ctx, store, &result); err != nil {
		return JournalFirstMigrationResult{}, err
	}
	result.Applied = true
	return result, nil
}

// applyPendingSchemaMigrations opens the live database and applies the
// auto-applied Go-owned schema migrations (SchemaMigrations(), versions 1..9).
// Already-applied migrations are checksum-skipped, so this is a no-op on a
// schema-current database and only fills the gap on a behind-schema install.
func applyPendingSchemaMigrations(ctx context.Context, root project.Root, resolver PathResolver) error {
	databasePath, err := resolver.DatabasePath(root)
	if err != nil {
		return err
	}
	store, err := OpenStore(databasePath)
	if err != nil {
		return err
	}
	defer store.Close()
	if err := store.ApplyMigrations(ctx); err != nil {
		return fmt.Errorf("apply pending schema migrations before journal-first migration: %w", err)
	}
	return nil
}

func journalFirstMigrationBaseResult(status Status, action string) JournalFirstMigrationResult {
	return JournalFirstMigrationResult{
		ContractVersion:    StateJSONContractVersion,
		DatabaseScope:      "global",
		DatabasePath:       status.DatabasePath,
		ProjectID:          status.ProjectID,
		ProjectName:        status.ProjectName,
		ProjectCurrentPath: status.ProjectCurrentPath,
		Action:             action,
	}
}

// journalFirstMigrationGate is the classified pre-flight state for a
// journal-first migration run.
type journalFirstMigrationGate struct {
	status Status
	// pendingSchema is true when the database is not yet schema-current but is
	// safely behind schema — every recorded migration matches the Go-owned
	// checksum, integrity is clean, and only known migrations (versions up to
	// the current baseline) remain to be applied. The migration runner applies
	// those pending migrations before the destructive journal-first step: on the
	// copy for dry-run, and on the live target BEFORE Backup for apply. The
	// pre-backup ordering is deliberate — Backup/VerifyBackup reject any schema
	// version that is not acceptableSchemaVersion (the baseline or the
	// journal-first version), so a behind-schema live database must be brought
	// current first for the mandatory pre-migration backup to verify. Those
	// pending migrations (1..9) are purely additive, so applying them before the
	// backup keeps it a complete, reversible pre-journal-first checkpoint.
	pendingSchema bool
}

// requireJournalFirstMigrationStatus classifies the target database for a
// journal-first migration. A schema-current database is ready as-is. A database
// that is only *behind* schema — pending known non-destructive migrations, with
// no checksum drift or corruption — is also accepted; the pending migrations are
// applied before the journal-first step. Genuinely invalid or uninitialized
// databases are refused with a clear error.
func requireJournalFirstMigrationStatus(root project.Root, resolver PathResolver) (journalFirstMigrationGate, error) {
	status, err := Inspect(root, resolver)
	if err != nil {
		return journalFirstMigrationGate{}, err
	}
	switch status.Mode {
	case ModeSQLiteReady:
		return journalFirstMigrationGate{status: status}, nil
	case ModeMarkdownOnly:
		return journalFirstMigrationGate{}, fmt.Errorf("SQLite state database is not initialized; run `loaf state init` first")
	case ModeInvalid:
		behind, err := classifyBehindSchemaTarget(status.DatabasePath)
		if err != nil {
			return journalFirstMigrationGate{}, err
		}
		if behind {
			return journalFirstMigrationGate{status: status, pendingSchema: true}, nil
		}
		return journalFirstMigrationGate{}, fmt.Errorf("state database is invalid; run `loaf state doctor`")
	default:
		return journalFirstMigrationGate{}, fmt.Errorf("state database is not ready; run `loaf state status`")
	}
}

// runJournalFirstMigration measures pre-migration counts, applies migration 10
// through the shared migration runner (recording it in schema_migrations), and
// measures the post-migration shape.
func runJournalFirstMigration(ctx context.Context, store *Store, result *JournalFirstMigrationResult) error {
	before, err := measureJournalFirstBefore(ctx, store)
	if err != nil {
		return err
	}
	result.JournalEntriesBefore = before.journalEntries
	result.NoiseEntriesPurged = before.noiseEntries
	result.PurgeBreakdown = before.purgeBreakdown
	result.SessionRowsPreservedAsLegacy = before.preservedAsLegacy
	result.HarnessSessionBackfill = before.backfillable
	result.HandoffsRetargeted = before.handoffs
	result.SessionEventsDeleted = before.sessionEvents
	result.SessionAliasesDeleted = before.sessionAliases
	result.SessionsDropped = before.sessions
	result.SnapshotsDropped = before.snapshots

	// Apply migrations 1-9 (idempotent, checksum-skipped) plus migration 10.
	// The shared runner records version 10 in schema_migrations.
	migrations := append(SchemaMigrations(), JournalFirstMigration())
	if err := ApplyMigrations(ctx, store.db, migrations); err != nil {
		return fmt.Errorf("apply journal-first migration: %w", err)
	}

	after, err := measureJournalFirstAfter(ctx, store)
	if err != nil {
		return err
	}
	result.JournalEntriesAfter = after.journalEntries
	result.JournalSearchRows = after.journalSearch
	result.SchemaVersion = after.schemaVersion
	return nil
}

type journalFirstBeforeCounts struct {
	journalEntries    int
	noiseEntries      int
	purgeBreakdown    []JournalFirstPurgeFamily
	preservedAsLegacy int
	backfillable      int
	handoffs          int
	sessionEvents     int
	sessionAliases    int
	sessions          int
	snapshots         int
}

func measureJournalFirstBefore(ctx context.Context, store *Store) (journalFirstBeforeCounts, error) {
	var counts journalFirstBeforeCounts

	// The pre-migration counts query legacy tables and columns (sessions,
	// session_state_snapshots, journal_entries.session_id) that migration 10
	// drops. On an already-migrated database those are gone, so there is
	// nothing left to transform: report all-zero counts for a clean no-op
	// re-run instead of failing on the missing tables.
	sessionsExists, err := sqliteTableExists(ctx, store.db, "sessions")
	if err != nil {
		return journalFirstBeforeCounts{}, fmt.Errorf("measure journal-first pre-migration counts: %w", err)
	}
	if !sessionsExists {
		if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM journal_entries`).Scan(&counts.journalEntries); err != nil {
			return journalFirstBeforeCounts{}, fmt.Errorf("measure journal-first pre-migration counts: %w", err)
		}
		// Already migrated: nothing to purge or preserve, but report a stable
		// all-zero breakdown so re-run output keeps the same JSON shape.
		counts.purgeBreakdown = zeroJournalFirstPurgeBreakdown()
		return counts, nil
	}

	var totalSessionRows int
	scans := []struct {
		dst *int
		sql string
	}{
		{&counts.journalEntries, `SELECT COUNT(*) FROM journal_entries`},
		{&totalSessionRows, `SELECT COUNT(*) FROM journal_entries WHERE entry_type = 'session'`},
		{&counts.backfillable, `
SELECT COUNT(*) FROM journal_entries j
WHERE j.harness_session_id IS NULL
  AND j.session_id IS NOT NULL
  AND EXISTS (SELECT 1 FROM sessions s WHERE s.id = j.session_id AND s.harness_session_id IS NOT NULL)`},
		{&counts.handoffs, `SELECT COUNT(*) FROM handoffs`},
		{&counts.sessionEvents, `SELECT COUNT(*) FROM events WHERE entity_kind = 'session'`},
		{&counts.sessionAliases, `SELECT COUNT(*) FROM aliases WHERE entity_kind = 'session'`},
		{&counts.sessions, `SELECT COUNT(*) FROM sessions`},
		{&counts.snapshots, `SELECT COUNT(*) FROM session_state_snapshots`},
	}
	for _, scan := range scans {
		if err := store.db.QueryRowContext(ctx, scan.sql).Scan(scan.dst); err != nil {
			return journalFirstBeforeCounts{}, fmt.Errorf("measure journal-first pre-migration counts: %w", err)
		}
	}

	// Break the entry_type='session' rows down by machine-generated family. The
	// families are mutually exclusive and each mirrors a branch of the migration
	// SQL, so their sum is exactly the set the DELETE removes. noiseEntries is
	// that purged total; any session row outside every family is preserved as
	// legacy_session (step 2a of the migration), so preservedAsLegacy is the
	// remainder.
	breakdown := make([]JournalFirstPurgeFamily, 0, len(journalFirstPurgeFamilies))
	purged := 0
	for _, family := range journalFirstPurgeFamilies {
		var n int
		query := `SELECT COUNT(*) FROM journal_entries WHERE entry_type = 'session' AND (` + family.sql + `)`
		if err := store.db.QueryRowContext(ctx, query).Scan(&n); err != nil {
			return journalFirstBeforeCounts{}, fmt.Errorf("measure journal-first purge family %q: %w", family.name, err)
		}
		breakdown = append(breakdown, JournalFirstPurgeFamily{Family: family.name, Count: n})
		purged += n
	}
	counts.purgeBreakdown = breakdown
	counts.noiseEntries = purged
	counts.preservedAsLegacy = totalSessionRows - purged
	return counts, nil
}

type journalFirstAfterCounts struct {
	journalEntries int
	journalSearch  int
	schemaVersion  int
}

func measureJournalFirstAfter(ctx context.Context, store *Store) (journalFirstAfterCounts, error) {
	var counts journalFirstAfterCounts
	scans := []struct {
		dst *int
		sql string
	}{
		{&counts.journalEntries, `SELECT COUNT(*) FROM journal_entries`},
		{&counts.journalSearch, `SELECT COUNT(*) FROM journal_search`},
	}
	for _, scan := range scans {
		if err := store.db.QueryRowContext(ctx, scan.sql).Scan(scan.dst); err != nil {
			return journalFirstAfterCounts{}, fmt.Errorf("measure journal-first post-migration counts: %w", err)
		}
	}
	version, err := store.SchemaVersion(ctx)
	if err != nil {
		return journalFirstAfterCounts{}, err
	}
	counts.schemaVersion = version
	return counts, nil
}
