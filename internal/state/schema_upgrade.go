package state

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/levifig/loaf/internal/project"
)

const SchemaUpgradeRequiredCode = "schema-upgrade-required"

// SchemaUpgradeRequiredError identifies a clean, behind-schema database that
// must be upgraded explicitly before mutable state operations can proceed.
type SchemaUpgradeRequiredError struct {
	Code            string `json:"code"`
	DatabasePath    string `json:"database_path"`
	CurrentVersion  int    `json:"current_version"`
	RequiredVersion int    `json:"required_version"`
	PendingVersions []int  `json:"pending_versions"`
	Command         string `json:"command"`
}

func (e *SchemaUpgradeRequiredError) Error() string {
	if e == nil {
		return "schema upgrade required"
	}
	return fmt.Sprintf("schema upgrade required for %s (current=%d required=%d); run `%s`", e.DatabasePath, e.CurrentVersion, e.RequiredVersion, e.Command)
}

func pendingSchemaMigrationVersions(current int) []int {
	versions := []int{}
	for _, migration := range SchemaMigrations() {
		if migration.Version > current {
			versions = append(versions, migration.Version)
		}
	}
	return versions
}

func schemaUpgradeRequiredError(path string, current int) error {
	return &SchemaUpgradeRequiredError{
		Code:            SchemaUpgradeRequiredCode,
		DatabasePath:    path,
		CurrentVersion:  current,
		RequiredVersion: CurrentSchemaVersion(),
		PendingVersions: pendingSchemaMigrationVersions(current),
		Command:         "loaf state migrate schema --apply",
	}
}

// requireCurrentSchemaForDerivedRepair is reserved for explicit bulk paths
// that rebuild a derived index in the same transaction as their canonical
// writes. Such a path may begin with journal-search divergence, but it still
// requires a current, structurally valid schema and every other operational
// invariant before it can attach or merge data.
func requireCurrentSchemaForDerivedRepair(ctx context.Context, s *Store) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("state database is invalid: store is nil")
	}
	version, err := s.SchemaVersion(ctx)
	if err != nil {
		return fmt.Errorf("state database is invalid: %w", err)
	}
	if version != CurrentSchemaVersion() {
		behind, classifyErr := classifySchemaUpgradeTarget(s.path, version)
		if classifyErr != nil {
			return classifyErr
		}
		if behind {
			return schemaUpgradeRequiredError(s.path, version)
		}
		return fmt.Errorf("state database is invalid: schema version %d does not match expected version %d", version, CurrentSchemaVersion())
	}
	if _, err := s.ValidateCurrentSchema(ctx); err != nil {
		return fmt.Errorf("state database is invalid: %w", err)
	}
	if _, valid, err := inspectOperationalInvariants(ctx, s); err != nil {
		return fmt.Errorf("state database is invalid: inspect operational invariants: %w", err)
	} else if !valid {
		return fmt.Errorf("state database is invalid: operational invariants failed")
	}
	return nil
}

// RequireCurrentSchema validates an existing mutable store without applying
// migrations. Clean behind-schema state returns a typed upgrade requirement;
// drift, corruption, and operational invariant failures remain invalid.
func (s *Store) RequireCurrentSchema(ctx context.Context) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("state database is invalid: store is nil")
	}
	version, err := s.SchemaVersion(ctx)
	if err != nil {
		return fmt.Errorf("state database is invalid: %w", err)
	}
	if version != CurrentSchemaVersion() {
		behind, classifyErr := classifySchemaUpgradeTarget(s.path, version)
		if classifyErr != nil {
			return classifyErr
		}
		if behind {
			return schemaUpgradeRequiredError(s.path, version)
		}
		return fmt.Errorf("state database is invalid: schema version %d does not match expected version %d", version, CurrentSchemaVersion())
	}
	if _, err := s.ValidateCurrentSchema(ctx); err != nil {
		return fmt.Errorf("state database is invalid: %w", err)
	}
	if _, valid, err := inspectOperationalInvariants(ctx, s); err != nil {
		return fmt.Errorf("state database is invalid: inspect operational invariants: %w", err)
	} else if !valid {
		return fmt.Errorf("state database is invalid: operational invariants failed")
	}
	// Journal-search parity is a derived-index invariant, not a schema or
	// canonical-state invariant. Search and backup boundaries inspect it and
	// refuse unsafe derived reads; ordinary journal/context operations remain
	// available so canonical state can still be retrieved or repaired.
	return nil
}

func classifySchemaUpgradeTarget(path string, version int) (bool, error) {
	return classifySchemaUpgradeTargetWithPolicy(path, version, false)
}

func classifySchemaUpgradeTargetWithPolicy(path string, version int, allowJournalSearchDivergence bool) (bool, error) {
	if version == journalFirstMigrationVersion {
		if allowJournalSearchDivergence {
			return classifyJournalFirstSchema10TargetWithPolicy(path, true)
		}
		return classifyJournalFirstSchema10Target(path)
	}
	if version < journalFirstMigrationVersion {
		return classifyBehindSchemaTarget(path)
	}
	return false, nil
}

// BootstrapIfEmpty applies the complete canonical schema only when the
// serialized database owner proves that no user schema exists yet. Existing
// databases are never mutated by this method.
func (s *Store) BootstrapIfEmpty(ctx context.Context) (bool, error) {
	deadline := time.Now().Add(openStoreRetryBudget)
	delay := openStoreRetryStart
	for {
		bootstrapped, err := s.bootstrapIfEmptyOnce(ctx)
		if err == nil {
			return bootstrapped, nil
		}
		if !retryableSQLiteOpenError(err) || time.Now().After(deadline) {
			return false, err
		}
		if sleepErr := sleepBeforeSQLiteRetry(deadline, delay); sleepErr != nil {
			return false, err
		}
		delay = nextSQLiteRetryDelay(delay)
	}
}

func (s *Store) bootstrapIfEmptyOnce(ctx context.Context) (bool, error) {
	if s == nil || s.db == nil {
		return false, fmt.Errorf("bootstrap state database: store is nil")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin state bootstrap: %w", err)
	}
	defer tx.Rollback()

	var schemaTableCount int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'schema_migrations'`).Scan(&schemaTableCount); err != nil {
		return false, fmt.Errorf("inspect state bootstrap schema: %w", err)
	}
	if schemaTableCount > 0 {
		var applied int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations`).Scan(&applied); err != nil {
			return false, fmt.Errorf("inspect state bootstrap migrations: %w", err)
		}
		if applied > 0 {
			return false, nil
		}
	}
	var userTables int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name NOT IN ('schema_migrations', 'sqlite_sequence')`).Scan(&userTables); err != nil {
		return false, fmt.Errorf("inspect state bootstrap tables: %w", err)
	}
	if userTables > 0 {
		return false, nil
	}
	if _, err := tx.ExecContext(ctx, schemaMigrationsDDL); err != nil {
		return false, fmt.Errorf("create state bootstrap schema ledger: %w", err)
	}
	for _, migration := range SchemaMigrations() {
		if err := applyMigration(ctx, tx, migration); err != nil {
			return false, fmt.Errorf("bootstrap schema migration: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit state bootstrap: %w", err)
	}
	return true, nil
}

// SchemaUpgradeResult is the common shape returned by explicit schema upgrade
// preview/apply APIs. The implementation lives below with the backup-first
// ordering and verification contract.
type SchemaUpgradeResult struct {
	ContractVersion int    `json:"contract_version"`
	DatabaseScope   string `json:"database_scope"`
	DatabasePath    string `json:"database_path"`
	CurrentVersion  int    `json:"current_version"`
	SchemaVersion   int    `json:"schema_version"`
	RequiredVersion int    `json:"required_version"`
	PendingVersions []int  `json:"pending_versions"`
	Action          string `json:"action"`
	Applied         bool   `json:"applied"`
	Verified        bool   `json:"verified"`
	BackupVerified  bool   `json:"backup_verified"`
	BackupPath      string `json:"backup_path,omitempty"`
}

const (
	SchemaUpgradeActionDryRun       = "dry-run"
	SchemaUpgradeActionApply        = "apply"
	SchemaUpgradeActionAlreadyReady = "already-current"
)

type schemaUpgradeFingerprint struct {
	Version                  int
	Checksums                map[int]string
	ProjectCount             int
	ProjectID                string
	ProjectName              string
	CurrentPath              string
	JournalParity            JournalSearchParity
	Watermark                JournalWatermark
	JournalDestructiveDigest string
}

type schemaUpgradeSource struct {
	Fingerprint schemaUpgradeFingerprint
	Pending     []int
}

type schemaUpgradeOperations struct {
	now         func() time.Time
	afterBackup func(string) error
	beforeApply func(*sql.Tx) error
	afterApply  func(*sql.Tx) error
}

// PreviewSchemaUpgrade classifies an existing behind-schema database without
// creating a backup or mutating any state.
func PreviewSchemaUpgrade(ctx context.Context, root project.Root, resolver PathResolver) (SchemaUpgradeResult, error) {
	path, err := resolver.DatabasePath(root)
	if err != nil {
		return SchemaUpgradeResult{}, err
	}
	source, err := classifySchemaUpgradeSource(ctx, path, root)
	if err != nil {
		return SchemaUpgradeResult{}, err
	}
	result := schemaUpgradeResult(source, path, SchemaUpgradeActionDryRun)
	if source.Fingerprint.Version == CurrentSchemaVersion() {
		result.Action = SchemaUpgradeActionAlreadyReady
		result.Verified = true
	}
	return result, nil
}

// ApplySchemaUpgrade performs an explicit, backup-first upgrade of clean
// behind-schema state. Migration 10 remains owned by journal-first migration;
// this API applies only canonical non-destructive migrations.
func ApplySchemaUpgrade(ctx context.Context, root project.Root, resolver PathResolver) (SchemaUpgradeResult, error) {
	return applySchemaUpgradeWithOps(ctx, root, resolver, nil)
}

func applySchemaUpgradeWithOps(ctx context.Context, root project.Root, resolver PathResolver, ops *schemaUpgradeOperations) (SchemaUpgradeResult, error) {
	path, err := resolver.DatabasePath(root)
	if err != nil {
		return SchemaUpgradeResult{}, err
	}
	source, err := classifySchemaUpgradeSource(ctx, path, root)
	if err != nil {
		return SchemaUpgradeResult{}, err
	}
	result := schemaUpgradeResult(source, path, SchemaUpgradeActionApply)
	if source.Fingerprint.Version == CurrentSchemaVersion() {
		result.Action = SchemaUpgradeActionAlreadyReady
		result.Verified = true
		return result, nil
	}
	backupPath, err := createSchemaUpgradeBackup(ctx, root, path, source.Fingerprint, ops)
	if err != nil {
		return result, err
	}
	result.BackupPath = backupPath
	result.BackupVerified = true
	if ops != nil && ops.afterBackup != nil {
		if err := ops.afterBackup(backupPath); err != nil {
			return result, err
		}
	}
	revalidated, err := classifySchemaUpgradeSource(ctx, path, root)
	if err != nil {
		return result, fmt.Errorf("revalidate schema upgrade source: %w", err)
	}
	if !schemaUpgradeFingerprintsEqual(source.Fingerprint, revalidated.Fingerprint) {
		return result, fmt.Errorf("schema upgrade source changed after backup; refusing stale apply")
	}
	store, err := OpenStore(path)
	if err != nil {
		return result, fmt.Errorf("open state database for schema upgrade: %w", err)
	}
	defer store.Close()
	if err := applySchemaUpgradeMigrations(ctx, store, revalidated.Fingerprint.Version, ops); err != nil {
		return result, err
	}
	if err := store.RequireCurrentSchema(ctx); err != nil {
		return result, fmt.Errorf("verify schema upgrade: %w", err)
	}
	result.CurrentVersion = CurrentSchemaVersion()
	result.SchemaVersion = CurrentSchemaVersion()
	result.Applied = true
	result.Verified = true
	return result, nil
}

func schemaUpgradeResult(source schemaUpgradeSource, path string, action string) SchemaUpgradeResult {
	return SchemaUpgradeResult{
		ContractVersion: StateJSONContractVersion,
		DatabaseScope:   "global",
		DatabasePath:    path,
		CurrentVersion:  source.Fingerprint.Version,
		SchemaVersion:   source.Fingerprint.Version,
		RequiredVersion: CurrentSchemaVersion(),
		PendingVersions: append([]int{}, source.Pending...),
		Action:          action,
	}
}

func classifySchemaUpgradeSource(ctx context.Context, path string, root project.Root) (schemaUpgradeSource, error) {
	return classifySchemaUpgradeSourceWithPolicy(ctx, path, root, false)
}

// classifyJournalFirstSource applies the journal-first bulk path's narrower
// exception: a structurally valid database may have only journal-search
// divergence because this migration rebuilds that derived index transactionally.
func classifyJournalFirstSource(ctx context.Context, path string, root project.Root) (schemaUpgradeSource, error) {
	return classifySchemaUpgradeSourceWithPolicy(ctx, path, root, true)
}

func classifySchemaUpgradeSourceWithPolicy(ctx context.Context, path string, root project.Root, allowJournalSearchDivergence bool) (schemaUpgradeSource, error) {
	store, err := OpenStoreReadOnly(path)
	if err != nil {
		return schemaUpgradeSource{}, fmt.Errorf("open schema upgrade source: %w", err)
	}
	defer store.Close()
	version, err := store.SchemaVersion(ctx)
	if err != nil {
		return schemaUpgradeSource{}, err
	}
	if version == CurrentSchemaVersion() {
		if _, err := store.ValidateCurrentSchema(ctx); err != nil {
			return schemaUpgradeSource{}, fmt.Errorf("state database is invalid: %w", err)
		}
	} else {
		behind, err := classifySchemaUpgradeTargetWithPolicy(path, version, allowJournalSearchDivergence)
		if err != nil {
			return schemaUpgradeSource{}, err
		}
		if !behind {
			return schemaUpgradeSource{}, fmt.Errorf("state database is invalid; schema upgrade requires clean behind-schema state")
		}
	}
	if _, valid, err := inspectOperationalInvariants(ctx, store); err != nil {
		return schemaUpgradeSource{}, fmt.Errorf("classify schema upgrade operational invariants: %w", err)
	} else if !valid {
		return schemaUpgradeSource{}, fmt.Errorf("state database is invalid: operational invariants failed")
	}
	var projectCount int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM projects`).Scan(&projectCount); err != nil {
		return schemaUpgradeSource{}, fmt.Errorf("count schema upgrade projects: %w", err)
	}
	identity, err := store.LookupProjectIdentityForRoot(ctx, root)
	if err != nil {
		return schemaUpgradeSource{}, fmt.Errorf("read schema upgrade project identity: %w", err)
	}
	parity, err := InspectJournalSearchParity(ctx, store)
	if err != nil {
		return schemaUpgradeSource{}, err
	}
	if !parity.Ready && !allowJournalSearchDivergence {
		return schemaUpgradeSource{}, fmt.Errorf("state database is invalid: journal search parity is not ready: %#v", parity)
	}
	watermark, err := readJournalWatermark(ctx, store)
	if err != nil {
		return schemaUpgradeSource{}, fmt.Errorf("read schema upgrade watermark: %w", err)
	}
	destructiveDigest, err := journalFirstDestructiveDigest(ctx, store.db)
	if err != nil {
		return schemaUpgradeSource{}, fmt.Errorf("read journal-first destructive fingerprint: %w", err)
	}
	checksums, err := readSchemaChecksums(ctx, store)
	if err != nil {
		return schemaUpgradeSource{}, err
	}
	return schemaUpgradeSource{
		Fingerprint: schemaUpgradeFingerprint{
			Version:                  version,
			Checksums:                checksums,
			ProjectCount:             projectCount,
			ProjectID:                identity.ID,
			ProjectName:              identity.FriendlyName,
			CurrentPath:              identity.CurrentPath,
			JournalParity:            parity,
			Watermark:                watermark,
			JournalDestructiveDigest: destructiveDigest,
		},
		Pending: pendingSchemaMigrationVersions(version),
	}, nil
}

func readSchemaChecksums(ctx context.Context, store *Store) (map[int]string, error) {
	rows, err := store.db.QueryContext(ctx, `SELECT version, checksum FROM schema_migrations ORDER BY version`)
	if err != nil {
		return nil, fmt.Errorf("read schema upgrade checksums: %w", err)
	}
	defer rows.Close()
	checksums := map[int]string{}
	for rows.Next() {
		var version int
		var checksum string
		if err := rows.Scan(&version, &checksum); err != nil {
			return nil, fmt.Errorf("scan schema upgrade checksum: %w", err)
		}
		checksums[version] = checksum
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return checksums, nil
}

func schemaUpgradeFingerprintsEqual(left, right schemaUpgradeFingerprint) bool {
	if left.Version != right.Version || left.ProjectCount != right.ProjectCount || left.ProjectID != right.ProjectID || left.ProjectName != right.ProjectName || left.CurrentPath != right.CurrentPath || left.JournalParity != right.JournalParity || left.Watermark != right.Watermark || left.JournalDestructiveDigest != right.JournalDestructiveDigest {
		return false
	}
	if len(left.Checksums) != len(right.Checksums) {
		return false
	}
	for version, checksum := range left.Checksums {
		if right.Checksums[version] != checksum {
			return false
		}
	}
	return true
}

func applySchemaUpgradeMigrations(ctx context.Context, store *Store, current int, ops *schemaUpgradeOperations) error {
	migrations := make([]SchemaMigration, 0, len(SchemaMigrations()))
	for _, migration := range SchemaMigrations() {
		if migration.Version > current {
			migrations = append(migrations, migration)
		}
	}
	if len(migrations) == 0 {
		return nil
	}
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin schema upgrade: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, schemaMigrationsDDL); err != nil {
		return fmt.Errorf("ensure schema upgrade ledger: %w", err)
	}
	if ops != nil && ops.beforeApply != nil {
		if err := ops.beforeApply(tx); err != nil {
			return fmt.Errorf("schema upgrade before-apply seam: %w", err)
		}
	}
	for _, migration := range migrations {
		if err := applyMigration(ctx, tx, migration); err != nil {
			return fmt.Errorf("apply schema upgrade migration: %w", err)
		}
	}
	if ops != nil && ops.afterApply != nil {
		if err := ops.afterApply(tx); err != nil {
			return fmt.Errorf("schema upgrade after-apply seam: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit schema upgrade: %w", err)
	}
	return nil
}

func createSchemaUpgradeBackup(ctx context.Context, root project.Root, databasePath string, source schemaUpgradeFingerprint, ops *schemaUpgradeOperations) (string, error) {
	return createSchemaUpgradeBackupWithPolicy(ctx, root, databasePath, source, ops, false)
}

func createJournalFirstMigrationBackup(ctx context.Context, root project.Root, databasePath string, source schemaUpgradeFingerprint, ops *schemaUpgradeOperations) (string, error) {
	return createSchemaUpgradeBackupWithPolicy(ctx, root, databasePath, source, ops, true)
}

func createSchemaUpgradeBackupWithPolicy(ctx context.Context, root project.Root, databasePath string, source schemaUpgradeFingerprint, ops *schemaUpgradeOperations, allowJournalSearchDivergence bool) (string, error) {
	projectRoot, err := resolvedProjectRoot(root.Path())
	if err != nil {
		return "", err
	}
	backupDir := filepath.Join(filepath.Dir(databasePath), "backups")
	resolvedDir, err := prepareBackupDestination(backupDir, false, projectRoot, nil)
	if err != nil {
		return "", err
	}
	now := time.Now().UTC()
	if ops != nil && ops.now != nil {
		now = ops.now().UTC()
	}
	backupPath, err := reserveBackupPath(resolvedDir, now)
	if err != nil {
		return "", err
	}
	verified := false
	defer func() {
		if !verified {
			_ = os.Remove(backupPath)
		}
	}()
	store, err := openStoreReadOnlyForBackup(databasePath)
	if err != nil {
		return "", err
	}
	if _, err := store.db.ExecContext(ctx, `VACUUM INTO ?`, backupPath); err != nil {
		store.Close()
		return "", fmt.Errorf("backup schema upgrade source: %w", err)
	}
	if err := store.Close(); err != nil {
		return "", err
	}
	if err := os.Chmod(backupPath, 0o600); err != nil {
		return "", err
	}
	var backupSource schemaUpgradeSource
	if allowJournalSearchDivergence {
		backupSource, err = classifyJournalFirstSource(ctx, backupPath, root)
	} else {
		backupSource, err = classifySchemaUpgradeSource(ctx, backupPath, root)
	}
	if err != nil {
		return "", fmt.Errorf("verify schema upgrade backup: %w", err)
	}
	if !schemaUpgradeFingerprintsEqual(source, backupSource.Fingerprint) {
		return "", fmt.Errorf("schema upgrade backup fingerprint does not match source")
	}
	verified = true
	return backupPath, nil
}
