package state

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/levifig/loaf/internal/project"
)

func TestSchemaUpgradeSchema9PreviewAndApply(t *testing.T) {
	ctx := context.Background()
	root, err := project.ResolveRoot(t.TempDir())
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	resolver := PathResolver{StateHome: t.TempDir()}
	databasePath, err := resolver.DatabasePath(root)
	if err != nil {
		t.Fatalf("DatabasePath() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(databasePath), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	store, err := OpenStore(databasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	if err := ApplyMigrations(ctx, store.db, SchemaMigrations()[:9]); err != nil {
		store.Close()
		t.Fatalf("apply schema9 migrations: %v", err)
	}
	if err := store.UpsertProject(ctx, root); err != nil {
		store.Close()
		t.Fatalf("UpsertProject() error = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	preview, err := PreviewSchemaUpgrade(ctx, root, resolver)
	if err != nil {
		t.Fatalf("PreviewSchemaUpgrade() error = %v", err)
	}
	if preview.CurrentVersion != 9 || len(preview.PendingVersions) != 2 || preview.PendingVersions[0] != 11 || preview.PendingVersions[1] != 12 || preview.BackupPath != "" {
		t.Fatalf("preview = %#v, want schema9 pending [11 12] without backup", preview)
	}
	result, err := ApplySchemaUpgrade(ctx, root, resolver)
	if err != nil {
		t.Fatalf("ApplySchemaUpgrade() error = %v", err)
	}
	if !result.Applied || !result.Verified || !result.BackupVerified || result.BackupPath == "" || result.CurrentVersion != CurrentSchemaVersion() || result.SchemaVersion != CurrentSchemaVersion() {
		t.Fatalf("result = %#v, want verified applied backup", result)
	}
	backupStore, err := OpenStoreReadOnly(result.BackupPath)
	if err != nil {
		t.Fatalf("OpenStoreReadOnly(backup) error = %v", err)
	}
	backupVersion, err := backupStore.SchemaVersion(ctx)
	if err != nil {
		backupStore.Close()
		t.Fatalf("backup SchemaVersion() error = %v", err)
	}
	if backupVersion != 9 {
		backupStore.Close()
		t.Fatalf("backup schema version = %d, want 9", backupVersion)
	}
	if exists, err := sqliteTableExists(ctx, backupStore.db, "journal_origins"); err != nil || exists {
		backupStore.Close()
		t.Fatalf("backup journal_origins exists=%t err=%v, want absent pre-upgrade table", exists, err)
	}
	if err := backupStore.Close(); err != nil {
		t.Fatalf("Close(backup) error = %v", err)
	}
	status, err := Inspect(root, resolver)
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}
	if status.Mode != ModeSQLiteReady || status.SchemaVersion != CurrentSchemaVersion() {
		t.Fatalf("status = %q/%d, want ready/%d", status.Mode, status.SchemaVersion, CurrentSchemaVersion())
	}
	second, err := ApplySchemaUpgrade(ctx, root, resolver)
	if err != nil {
		t.Fatalf("idempotent ApplySchemaUpgrade() error = %v", err)
	}
	if second.Applied || second.BackupPath != "" || second.Action != SchemaUpgradeActionAlreadyReady {
		t.Fatalf("second = %#v, want current no-op without backup", second)
	}
}

func TestSchemaUpgradeFailureSeamsPreserveVerifiedBackupAndRollback(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name string
		ops  func(*testing.T, project.Root, PathResolver) *schemaUpgradeOperations
	}{
		{
			name: "after-backup",
			ops: func(t *testing.T, _ project.Root, _ PathResolver) *schemaUpgradeOperations {
				return &schemaUpgradeOperations{afterBackup: func(string) error { return errors.New("after backup") }}
			},
		},
		{
			name: "before-apply",
			ops: func(t *testing.T, _ project.Root, _ PathResolver) *schemaUpgradeOperations {
				return &schemaUpgradeOperations{beforeApply: func(*sql.Tx) error { return errors.New("before apply") }}
			},
		},
		{
			name: "after-apply",
			ops: func(t *testing.T, _ project.Root, _ PathResolver) *schemaUpgradeOperations {
				return &schemaUpgradeOperations{afterApply: func(*sql.Tx) error { return errors.New("after apply") }}
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, resolver, databasePath := seedSchema9UpgradeTarget(t)
			before := schemaVersionAndMigrationCount(t, databasePath)
			result, err := applySchemaUpgradeWithOps(ctx, root, resolver, tc.ops(t, root, resolver))
			if err == nil || result.BackupPath == "" || !result.BackupVerified {
				t.Fatalf("apply failure result=%#v err=%v, want error and verified backup", result, err)
			}
			backup, verifyErr := classifySchemaUpgradeSource(ctx, result.BackupPath, root)
			if verifyErr != nil || backup.Fingerprint.Version != 9 || len(backup.Pending) != 2 || backup.Pending[0] != 11 || backup.Pending[1] != 12 {
				t.Fatalf("backup source classification=%#v err=%v, want verified schema9 source", backup, verifyErr)
			}
			after := schemaVersionAndMigrationCount(t, databasePath)
			if before != after || after != "9/9" {
				t.Fatalf("live schema state changed across %s failure: before=%s after=%s", tc.name, before, after)
			}
		})
	}
}

func TestSchemaUpgradeRejectsStaleSourceAfterBackup(t *testing.T) {
	ctx := context.Background()
	root, resolver, databasePath := seedSchema9UpgradeTarget(t)
	result, err := applySchemaUpgradeWithOps(ctx, root, resolver, &schemaUpgradeOperations{afterBackup: func(string) error {
		store, openErr := OpenStore(databasePath)
		if openErr != nil {
			return openErr
		}
		defer store.Close()
		_, execErr := store.db.ExecContext(ctx, `UPDATE projects SET friendly_name = 'changed after backup'`)
		return execErr
	}})
	if err == nil || result.BackupPath == "" || !result.BackupVerified {
		t.Fatalf("stale source result=%#v err=%v, want verified backup and refusal", result, err)
	}
	if got := schemaVersionAndMigrationCount(t, databasePath); got != "9/9" {
		t.Fatalf("stale source applied migrations: %s", got)
	}
}

func TestSchemaUpgradeSchema10BackupPreservesPreOriginShape(t *testing.T) {
	ctx := context.Background()
	root, resolver, databasePath := seedSchema10UpgradeTarget(t)
	result, err := ApplySchemaUpgrade(ctx, root, resolver)
	if err != nil {
		t.Fatalf("ApplySchemaUpgrade(schema10) error = %v", err)
	}
	if !result.Applied || !result.Verified || !result.BackupVerified || result.BackupPath == "" {
		t.Fatalf("schema10 result=%#v, want applied verified backup", result)
	}
	backup, err := OpenStoreReadOnly(result.BackupPath)
	if err != nil {
		t.Fatalf("OpenStoreReadOnly(schema10 backup) error = %v", err)
	}
	defer backup.Close()
	version, err := backup.SchemaVersion(ctx)
	if err != nil || version != journalFirstMigrationVersion {
		t.Fatalf("schema10 backup version=%d err=%v, want %d", version, err, journalFirstMigrationVersion)
	}
	if exists, err := sqliteTableExists(ctx, backup.db, "journal_origins"); err != nil || exists {
		t.Fatalf("schema10 backup journal_origins exists=%t err=%v, want absent", exists, err)
	}
	if got := schemaVersionAndMigrationCount(t, databasePath); got != "12/12" {
		t.Fatalf("live schema after schema10 upgrade = %s, want 12/12", got)
	}
}

func TestSchemaUpgradeInvalidSchema10RefusesBeforeBackupOrMutation(t *testing.T) {
	ctx := context.Background()
	root, resolver, databasePath := seedSchema10UpgradeTarget(t)
	store, err := OpenStore(databasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `UPDATE schema_migrations SET checksum = 'invalid' WHERE version = ?`, journalFirstMigrationVersion); err != nil {
		store.Close()
		t.Fatalf("corrupt schema10 checksum: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	before, err := os.ReadFile(databasePath)
	if err != nil {
		t.Fatalf("ReadFile(before) error = %v", err)
	}
	result, err := ApplySchemaUpgrade(ctx, root, resolver)
	if err == nil || result.BackupPath != "" {
		t.Fatalf("ApplySchemaUpgrade(invalid schema10) result=%#v err=%v, want refusal before backup", result, err)
	}
	after, err := os.ReadFile(databasePath)
	if err != nil {
		t.Fatalf("ReadFile(after) error = %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("invalid schema10 source changed during refused upgrade")
	}
	backupDir := filepath.Join(filepath.Dir(databasePath), "backups")
	if entries, err := os.ReadDir(backupDir); err == nil && len(entries) != 0 {
		t.Fatalf("invalid schema10 reserved backup artifacts: %#v", entries)
	} else if err != nil && !os.IsNotExist(err) {
		t.Fatalf("ReadDir(backups) error = %v", err)
	}
}

func TestSchemaUpgradeBackupDestinationFailureLeavesLiveSourceUntouched(t *testing.T) {
	ctx := context.Background()
	root, resolver, databasePath := seedSchema9UpgradeTarget(t)
	before, err := os.ReadFile(databasePath)
	if err != nil {
		t.Fatalf("ReadFile(before) error = %v", err)
	}
	backupPath := filepath.Join(filepath.Dir(databasePath), "backups")
	if err := os.WriteFile(backupPath, []byte("not a directory"), 0o600); err != nil {
		t.Fatalf("write obstructing backups path: %v", err)
	}
	result, err := ApplySchemaUpgrade(ctx, root, resolver)
	if err == nil || result.BackupPath != "" || result.BackupVerified {
		t.Fatalf("ApplySchemaUpgrade(backup failure) result=%#v err=%v, want failure before backup", result, err)
	}
	after, err := os.ReadFile(databasePath)
	if err != nil {
		t.Fatalf("ReadFile(after) error = %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("live source changed after backup destination failure")
	}
}

func TestSchema10OrdinaryWritesRequireUpgradeWithoutMutation(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name string
		run  func(project.Root, PathResolver) error
	}{
		{"journal-log", func(root project.Root, resolver PathResolver) error {
			_, err := LogJournal(ctx, root, resolver, JournalLogOptions{Entry: "decision(test): blocked"})
			return err
		}},
		{"journal-defer", func(root project.Root, resolver PathResolver) error {
			_, err := DeferJournal(ctx, root, resolver, JournalDeferOptions{Intent: "later", Why: "test", Boundary: "not now", Trigger: "triage", OperationID: "schema10-test"})
			return err
		}},
		{"task-create", func(root project.Root, resolver PathResolver) error {
			_, err := CreateTask(ctx, root, resolver, TaskCreateOptions{Title: "blocked write"})
			return err
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, resolver, databasePath := seedSchema10UpgradeTarget(t)
			before := schema10MutableCounts(t, databasePath)
			err := tc.run(root, resolver)
			var required *SchemaUpgradeRequiredError
			if !errors.As(err, &required) || required.Code != SchemaUpgradeRequiredCode || len(required.PendingVersions) != 2 || required.PendingVersions[0] != 11 || required.PendingVersions[1] != 12 {
				t.Fatalf("%s error=%v required=%#v, want schema-upgrade-required pending [11 12]", tc.name, err, required)
			}
			after := schema10MutableCounts(t, databasePath)
			if before != after {
				t.Fatalf("%s mutated schema10 database: before=%s after=%s", tc.name, before, after)
			}
		})
	}
}

func seedSchema9UpgradeTarget(t *testing.T) (project.Root, PathResolver, string) {
	t.Helper()
	root, err := project.ResolveRoot(t.TempDir())
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	resolver := PathResolver{StateHome: t.TempDir()}
	databasePath, err := resolver.DatabasePath(root)
	if err != nil {
		t.Fatalf("DatabasePath() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(databasePath), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	store, err := OpenStore(databasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	if err := ApplyMigrations(context.Background(), store.db, SchemaMigrations()[:9]); err != nil {
		store.Close()
		t.Fatalf("apply schema9 migrations: %v", err)
	}
	if err := store.UpsertProject(context.Background(), root); err != nil {
		store.Close()
		t.Fatalf("UpsertProject() error = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	return root, resolver, databasePath
}

func seedSchema10UpgradeTarget(t *testing.T) (project.Root, PathResolver, string) {
	t.Helper()
	root, resolver, databasePath := seedSchema9UpgradeTarget(t)
	store, err := OpenStore(databasePath)
	if err != nil {
		t.Fatalf("OpenStore(schema10) error = %v", err)
	}
	if err := ApplyMigrations(context.Background(), store.db, []SchemaMigration{JournalFirstMigration()}); err != nil {
		store.Close()
		t.Fatalf("apply schema10 migration: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close(schema10) error = %v", err)
	}
	return root, resolver, databasePath
}

func schemaVersionAndMigrationCount(t *testing.T, databasePath string) string {
	t.Helper()
	store, err := OpenStoreReadOnly(databasePath)
	if err != nil {
		t.Fatalf("OpenStoreReadOnly() error = %v", err)
	}
	defer store.Close()
	var version, migrations int
	if err := store.db.QueryRow(`SELECT COALESCE(MAX(version), 0), COUNT(*) FROM schema_migrations`).Scan(&version, &migrations); err != nil {
		t.Fatalf("read schema state: %v", err)
	}
	return fmt.Sprintf("%d/%d", version, migrations)
}

func schema10MutableCounts(t *testing.T, databasePath string) string {
	t.Helper()
	store, err := OpenStoreReadOnly(databasePath)
	if err != nil {
		t.Fatalf("OpenStoreReadOnly() error = %v", err)
	}
	defer store.Close()
	counts := make([]int, 0, 5)
	for _, table := range []string{"journal_entries", "journal_search", "sparks", "tasks"} {
		var count int
		if err := store.db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&count); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		counts = append(counts, count)
	}
	for _, table := range []string{"journal_origins", "journal_deferrals"} {
		exists, err := sqliteTableExists(context.Background(), store.db, table)
		if err != nil {
			t.Fatalf("table %s: %v", table, err)
		}
		if exists {
			t.Fatalf("schema10 unexpectedly has %s", table)
		}
	}
	return fmt.Sprintf("%s/%v", schemaVersionAndMigrationCount(t, databasePath), counts)
}

func TestSpecArchiveRefusesSchema10WithoutMutation(t *testing.T) {
	ctx := context.Background()
	root, err := project.ResolveRoot(t.TempDir())
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	resolver := PathResolver{StateHome: t.TempDir()}
	databasePath, err := resolver.DatabasePath(root)
	if err != nil {
		t.Fatalf("DatabasePath() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(databasePath), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	store, err := OpenStore(databasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	if err := ApplyMigrations(ctx, store.db, SchemaMigrations()[:9]); err != nil {
		store.Close()
		t.Fatalf("apply schema9 migrations: %v", err)
	}
	if err := store.UpsertProject(ctx, root); err != nil {
		store.Close()
		t.Fatalf("UpsertProject() error = %v", err)
	}
	if err := ApplyMigrations(ctx, store.db, []SchemaMigration{JournalFirstMigration()}); err != nil {
		store.Close()
		t.Fatalf("apply schema10 migration: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	before, err := os.ReadFile(databasePath)
	if err != nil {
		t.Fatalf("read schema10 database: %v", err)
	}
	_, err = ArchiveSpecs(ctx, root, resolver, []string{"missing-spec"})
	var required *SchemaUpgradeRequiredError
	if !errors.As(err, &required) || required.Code != SchemaUpgradeRequiredCode {
		t.Fatalf("ArchiveSpecs() error = %v, want typed schema upgrade requirement", err)
	}
	after, err := os.ReadFile(databasePath)
	if err != nil {
		t.Fatalf("read schema10 database after archive refusal: %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("schema10 database changed after spec archive refusal")
	}
}

func TestListSpecsRefusesSchema10WithoutMutation(t *testing.T) {
	ctx := context.Background()
	root, err := project.ResolveRoot(t.TempDir())
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	resolver := PathResolver{StateHome: t.TempDir()}
	databasePath, err := resolver.DatabasePath(root)
	if err != nil {
		t.Fatalf("DatabasePath() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(databasePath), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	store, err := OpenStore(databasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	if err := ApplyMigrations(ctx, store.db, SchemaMigrations()[:9]); err != nil {
		store.Close()
		t.Fatalf("apply schema9 migrations: %v", err)
	}
	if err := store.UpsertProject(ctx, root); err != nil {
		store.Close()
		t.Fatalf("UpsertProject() error = %v", err)
	}
	if err := ApplyMigrations(ctx, store.db, []SchemaMigration{JournalFirstMigration()}); err != nil {
		store.Close()
		t.Fatalf("apply schema10 migration: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	before, err := os.ReadFile(databasePath)
	if err != nil {
		t.Fatalf("read schema10 database: %v", err)
	}
	_, err = ListSpecs(ctx, root, resolver)
	var required *SchemaUpgradeRequiredError
	if !errors.As(err, &required) || required.Code != SchemaUpgradeRequiredCode {
		t.Fatalf("ListSpecs() error = %v, want typed schema upgrade requirement", err)
	}
	after, err := os.ReadFile(databasePath)
	if err != nil {
		t.Fatalf("read schema10 database after list refusal: %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("schema10 database changed after list refusal")
	}
}
