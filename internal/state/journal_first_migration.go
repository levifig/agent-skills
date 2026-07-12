package state

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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

	// runJournalFirstMigration applies the pre-journal migrations (1..9),
	// journal-first migration (10), and origins migration (11) in order, so a
	// behind-schema copy is brought current and previewed in a single pass. The live database is
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
	return applyJournalFirstMigrationWithHooks(ctx, root, resolver, nil)
}

type journalFirstApplyHooks struct {
	afterBackup func(string) error
}

func applyJournalFirstMigrationWithHooks(ctx context.Context, root project.Root, resolver PathResolver, hooks *journalFirstApplyHooks) (JournalFirstMigrationResult, error) {
	gate, err := requireJournalFirstMigrationStatus(root, resolver)
	if err != nil {
		return JournalFirstMigrationResult{}, err
	}
	source, err := classifyJournalFirstSource(ctx, gate.status.DatabasePath, root)
	if err != nil {
		return JournalFirstMigrationResult{}, err
	}
	backupPath, err := createJournalFirstMigrationBackup(ctx, root, gate.status.DatabasePath, source.Fingerprint, nil)
	if err != nil {
		return JournalFirstMigrationResult{}, err
	}
	result := journalFirstMigrationBaseResult(gate.status, JournalFirstMigrationActionApply)
	result.BackupPath = backupPath
	if hooks != nil && hooks.afterBackup != nil {
		if err := hooks.afterBackup(backupPath); err != nil {
			return result, err
		}
	}
	store, err := OpenStore(gate.status.DatabasePath)
	if err != nil {
		return JournalFirstMigrationResult{}, err
	}
	defer store.Close()

	if err := runJournalFirstMigrationWithSource(ctx, store, &result, source.Fingerprint, root); err != nil {
		return result, err
	}
	result.Applied = true
	return result, nil
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
}

// requireJournalFirstMigrationStatus classifies the target database for a
// journal-first migration. A schema-current database is ready as-is. A database
// that is only *behind* schema — pending known non-destructive migrations, with
// no checksum drift, corruption, or invariant divergence — is also accepted.
// Genuinely invalid or uninitialized databases are refused with a clear error.
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
		if status.SchemaVersion == journalFirstMigrationVersion {
			// Journal-first is the sole destructive bulk path that rebuilds this
			// derived FTS index transactionally. Its gate permits only that known
			// divergence; checksum, integrity, and all other invariants remain
			// strict in the classifier.
			behind, err := classifyJournalFirstSchema10TargetWithPolicy(status.DatabasePath, true)
			if err != nil {
				return journalFirstMigrationGate{}, err
			}
			if behind {
				return journalFirstMigrationGate{status: status}, nil
			}
			return journalFirstMigrationGate{}, fmt.Errorf("state database is invalid; run `loaf state doctor`")
		}
		behind, err := classifyBehindSchemaTarget(status.DatabasePath)
		if err != nil {
			return journalFirstMigrationGate{}, err
		}
		if behind {
			return journalFirstMigrationGate{status: status}, nil
		}
		return journalFirstMigrationGate{}, fmt.Errorf("state database is invalid; run `loaf state doctor`")
	default:
		return journalFirstMigrationGate{}, fmt.Errorf("state database is not ready; run `loaf state status`")
	}
}

func classifyJournalFirstSchema10Target(databasePath string) (bool, error) {
	return classifyJournalFirstSchema10TargetWithPolicy(databasePath, false)
}

func classifyJournalFirstSchema10TargetWithPolicy(databasePath string, allowJournalSearchDivergence bool) (bool, error) {
	store, err := OpenStoreReadOnly(databasePath)
	if err != nil {
		return false, nil
	}
	defer store.Close()
	ctx := context.Background()
	version, err := store.SchemaVersion(ctx)
	if err != nil || version != journalFirstMigrationVersion {
		return false, nil
	}
	known := make(map[int]SchemaMigration, len(SchemaMigrations())+1)
	for _, migration := range SchemaMigrations() {
		known[migration.Version] = migration
	}
	known[journalFirstMigrationVersion] = JournalFirstMigration()
	rows, err := store.db.QueryContext(ctx, `SELECT version, checksum FROM schema_migrations ORDER BY version`)
	if err != nil {
		return false, nil
	}
	defer rows.Close()
	recorded := 0
	for rows.Next() {
		var recordedVersion int
		var recordedChecksum string
		if err := rows.Scan(&recordedVersion, &recordedChecksum); err != nil {
			return false, fmt.Errorf("scan schema-10 migration row: %w", err)
		}
		migration, ok := known[recordedVersion]
		if !ok || migration.Checksum() != recordedChecksum {
			return false, nil
		}
		recorded++
	}
	if err := rows.Err(); err != nil {
		return false, fmt.Errorf("iterate schema-10 migration rows: %w", err)
	}
	if recorded != journalFirstMigrationVersion {
		return false, nil
	}
	_, integrityValid, err := inspectSQLiteIntegrity(ctx, store)
	if err != nil {
		return false, fmt.Errorf("classify schema-10 integrity: %w", err)
	}
	if !integrityValid {
		return false, nil
	}
	if _, operationalValid, err := inspectOperationalInvariants(ctx, store); err != nil {
		return false, fmt.Errorf("classify schema-10 operational invariants: %w", err)
	} else if !operationalValid {
		return false, nil
	}
	parity, err := inspectJournalSearchParity(ctx, store)
	if err != nil {
		return false, fmt.Errorf("classify schema-10 journal search parity: %w", err)
	}
	if !parity.Ready && !allowJournalSearchDivergence {
		return false, nil
	}
	return true, nil
}

// runJournalFirstMigration measures pre-migration counts, applies migration 10
// through the shared migration runner (recording it in schema_migrations), and
// measures the post-migration shape.
func runJournalFirstMigration(ctx context.Context, store *Store, result *JournalFirstMigrationResult) error {
	return runJournalFirstMigrationWithHooks(ctx, store, result, nil)
}

// runJournalFirstMigrationWithSource binds an apply to the exact source that
// was backed up. It takes SQLite's write lock before comparing the transaction
// snapshot, so a concurrent writer is either represented in that snapshot and
// backup fingerprint or blocked until this migration commits or rolls back.
func runJournalFirstMigrationWithSource(ctx context.Context, store *Store, result *JournalFirstMigrationResult, source schemaUpgradeFingerprint, root project.Root) error {
	return runJournalFirstMigrationWithHooksAndSource(ctx, store, result, nil, &source, root)
}

type journalFirstMigrationHooks struct {
	afterMigration10 func(*sql.Tx) error
	afterPrune       func(*sql.Tx) error
}

func runJournalFirstMigrationWithHooks(ctx context.Context, store *Store, result *JournalFirstMigrationResult, hooks *journalFirstMigrationHooks) error {
	return runJournalFirstMigrationWithHooksAndSource(ctx, store, result, hooks, nil, project.Root{})
}

func runJournalFirstMigrationWithHooksAndSource(ctx context.Context, store *Store, result *JournalFirstMigrationResult, hooks *journalFirstMigrationHooks, source *schemaUpgradeFingerprint, root project.Root) error {
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

	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin journal-first migration transaction: %w", err)
	}
	defer tx.Rollback()
	if source != nil {
		// Acquire the writer lock before reading the transaction snapshot. This
		// is intentionally a no-op ledger write; no destructive migration work
		// occurs until the backed-up source fingerprint is proven unchanged.
		if _, err := tx.ExecContext(ctx, `UPDATE schema_migrations SET checksum = checksum WHERE version = ?`, journalFirstMigrationVersion); err != nil {
			return fmt.Errorf("lock journal-first migration source: %w", err)
		}
		matches, err := journalFirstSourceMatchesTx(ctx, tx, root, *source)
		if err != nil {
			return fmt.Errorf("validate journal-first migration source: %w", err)
		}
		if !matches {
			return fmt.Errorf("journal-first migration source changed after backup; refusing stale apply")
		}
	}
	// Apply migrations 1-9 plus migration 10 in order, then prune optional
	// provenance and apply migration 11 if absent. All destructive work,
	// migration records, pruning, and parity verification remain uncommitted
	// until every pre-commit check succeeds.
	preJournalMigrations := make([]SchemaMigration, 0, len(SchemaMigrations())+1)
	for _, migration := range SchemaMigrations() {
		if migration.Version < journalFirstMigrationVersion {
			preJournalMigrations = append(preJournalMigrations, migration)
		}
	}
	preJournalMigrations = append(preJournalMigrations, JournalFirstMigration())
	if _, err := tx.ExecContext(ctx, schemaMigrationsDDL); err != nil {
		return fmt.Errorf("ensure schema_migrations: %w", err)
	}
	for _, migration := range preJournalMigrations {
		if err := applyMigration(ctx, tx, migration); err != nil {
			return fmt.Errorf("apply journal-first migration: %w", err)
		}
	}
	if hooks != nil && hooks.afterMigration10 != nil {
		if err := hooks.afterMigration10(tx); err != nil {
			return fmt.Errorf("journal-first after-migration-10 seam: %w", err)
		}
	}
	if err := pruneOptionalProvenanceTx(ctx, tx); err != nil {
		return fmt.Errorf("prune optional provenance after journal-first migration: %w", err)
	}
	if hooks != nil && hooks.afterPrune != nil {
		if err := hooks.afterPrune(tx); err != nil {
			return fmt.Errorf("journal-first after-prune seam: %w", err)
		}
	}
	if err := applyMigration(ctx, tx, journalOriginsMigration()); err != nil {
		return fmt.Errorf("apply journal origins migration after journal-first migration: %w", err)
	}
	if _, err := rebuildAndVerifyJournalSearch(ctx, tx); err != nil {
		return fmt.Errorf("rebuild journal search after journal-first migration: %w", err)
	}
	parity, err := inspectJournalSearchParity(ctx, tx)
	if err != nil {
		return fmt.Errorf("verify journal search parity after journal-first migration: %w", err)
	}
	if !parity.Ready {
		return fmt.Errorf("journal search parity after journal-first migration is not ready: canonical_rows=%d, index_rows=%d, missing=%d, extra=%d, changed=%d", parity.CanonicalRows, parity.IndexRows, parity.Missing, parity.Extra, parity.Changed)
	}
	after, err := measureJournalFirstAfterQuery(ctx, tx)
	if err != nil {
		return err
	}
	result.JournalEntriesAfter = after.journalEntries
	result.JournalSearchRows = after.journalSearch
	result.SchemaVersion = after.schemaVersion
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit journal-first migration: %w", err)
	}
	return nil
}

func journalOriginsMigration() SchemaMigration {
	for _, migration := range SchemaMigrations() {
		if migration.Version == journalOriginsMigrationVersion {
			return migration
		}
	}
	panic("journal origins migration is not registered")
}

func journalFirstSourceMatchesTx(ctx context.Context, tx *sql.Tx, root project.Root, want schemaUpgradeFingerprint) (bool, error) {
	var version, projectCount int
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0), COUNT(*) FROM schema_migrations`).Scan(&version, &projectCount); err != nil {
		return false, err
	}
	checksums := map[int]string{}
	rows, err := tx.QueryContext(ctx, `SELECT version, checksum FROM schema_migrations ORDER BY version`)
	if err != nil {
		return false, err
	}
	for rows.Next() {
		var migrationVersion int
		var checksum string
		if err := rows.Scan(&migrationVersion, &checksum); err != nil {
			rows.Close()
			return false, err
		}
		checksums[migrationVersion] = checksum
	}
	if err := rows.Close(); err != nil {
		return false, err
	}
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM projects`).Scan(&projectCount); err != nil {
		return false, err
	}
	actual := schemaUpgradeFingerprint{Version: version, Checksums: checksums, ProjectCount: projectCount}
	if err := tx.QueryRowContext(ctx, `
SELECT p.id, p.friendly_name, COALESCE(cp.path, p.current_path, '')
FROM projects AS p
JOIN project_paths AS cp ON cp.project_id = p.id AND cp.is_current = 1
WHERE cp.path = ?`, root.Path()).Scan(&actual.ProjectID, &actual.ProjectName, &actual.CurrentPath); err != nil {
		return false, err
	}
	parity, err := inspectJournalSearchParity(ctx, tx)
	if err != nil {
		return false, err
	}
	actual.JournalParity = parity
	if err := tx.QueryRowContext(ctx, `SELECT id, created_at FROM journal_entries ORDER BY created_at DESC, id DESC LIMIT 1`).Scan(&actual.Watermark.JournalEntryID, &actual.Watermark.CreatedAt); err == nil {
		actual.Watermark.Present = true
	} else if err != sql.ErrNoRows {
		return false, err
	}
	digest, err := journalFirstDestructiveDigest(ctx, tx)
	if err != nil {
		return false, err
	}
	actual.JournalDestructiveDigest = digest
	return schemaUpgradeFingerprintsEqual(want, actual), nil
}

// journalFirstDestructiveDigest fingerprints every table that migration 10
// deletes, rebuilds, or retargets. Values are emitted with their Go/SQLite type
// marker in a stable table and rowid order, so equal counts or watermarks cannot
// hide a concurrent update to a surviving row.
func journalFirstDestructiveDigest(ctx context.Context, queryer queryContext) (string, error) {
	hash := sha256.New()
	for _, table := range []string{"journal_entries", "sessions", "handoffs", "session_state_snapshots", "events", "aliases", "journal_search", "journal_origins", "journal_deferrals"} {
		exists, err := provenanceTableExists(ctx, queryer, table)
		if err != nil {
			return "", err
		}
		if !exists {
			continue
		}
		rows, err := queryer.QueryContext(ctx, `SELECT rowid, * FROM `+table+` ORDER BY rowid`)
		if err != nil {
			return "", err
		}
		columns, err := rows.Columns()
		if err != nil {
			rows.Close()
			return "", err
		}
		_, _ = hash.Write([]byte(table + "\x00" + strings.Join(columns, "\x00") + "\n"))
		for rows.Next() {
			values := make([]any, len(columns))
			dest := make([]any, len(columns))
			for i := range values {
				dest[i] = &values[i]
			}
			if err := rows.Scan(dest...); err != nil {
				rows.Close()
				return "", err
			}
			for _, value := range values {
				switch typed := value.(type) {
				case nil:
					_, _ = hash.Write([]byte("nil\x00"))
				case []byte:
					_, _ = hash.Write([]byte("bytes:" + hex.EncodeToString(typed) + "\x00"))
				default:
					_, _ = hash.Write([]byte(fmt.Sprintf("%T:%v\x00", typed, typed)))
				}
			}
			_, _ = hash.Write([]byte("\n"))
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return "", err
		}
		if err := rows.Close(); err != nil {
			return "", err
		}
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
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
	return measureJournalFirstAfterQuery(ctx, store.db)
}

func measureJournalFirstAfterQuery(ctx context.Context, queryer queryContext) (journalFirstAfterCounts, error) {
	var counts journalFirstAfterCounts
	scans := []struct {
		dst *int
		sql string
	}{
		{&counts.journalEntries, `SELECT COUNT(*) FROM journal_entries`},
		{&counts.journalSearch, `SELECT COUNT(*) FROM journal_search`},
	}
	for _, scan := range scans {
		if err := queryer.QueryRowContext(ctx, scan.sql).Scan(scan.dst); err != nil {
			return journalFirstAfterCounts{}, fmt.Errorf("measure journal-first post-migration counts: %w", err)
		}
	}
	if err := queryer.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&counts.schemaVersion); err != nil {
		return journalFirstAfterCounts{}, fmt.Errorf("read schema version: %w", err)
	}
	return counts, nil
}
