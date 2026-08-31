package state

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/levifig/loaf/vnext/continuity"
	"github.com/levifig/loaf/vnext/migration/archive"
)

func TestExportVNextRehearsalArchiveReadsOnlyVerifiedBackupAndSeparatesWraps(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	resolver := PathResolver{StateHome: t.TempDir()}
	status, err := Initialize(ctx, root, resolver)
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	discover, err := LogJournal(ctx, root, resolver, JournalLogOptions{
		Entry: "discover(migration): learned", ObservedBranch: "main", ObservedWorktree: root.Path(), HarnessSessionID: "session-export",
		Origin: &JournalOriginInput{EnvelopeVersion: JournalOriginEnvelopeVersion, CaptureMechanism: JournalOriginMechanismManual},
	})
	if err != nil {
		t.Fatalf("LogJournal(discover) error = %v", err)
	}
	wrap, err := LogJournal(ctx, root, resolver, JournalLogOptions{
		Entry: "wrap(migration): next is staged import", ObservedBranch: "main", ObservedWorktree: root.Path(), HarnessSessionID: "session-export",
	})
	if err != nil {
		t.Fatalf("LogJournal(wrap) error = %v", err)
	}
	store, err := OpenStore(status.DatabasePath)
	if err != nil {
		t.Fatalf("OpenStore(revision seed) error = %v", err)
	}
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin revision seed: %v", err)
	}
	folded, err := foldJournalFacts(ctx, tx, status.ProjectID)
	if err != nil {
		t.Fatalf("fold revision seed: %v", err)
	}
	revision := folded[discover.ID].payload
	revision.Message = "learned and revised"
	if err := appendJournalFactRevisionForImportTx(ctx, tx, status.ProjectID, discover.ID, revision, time.Now().UTC()); err != nil {
		t.Fatalf("append revision seed: %v", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE journal_search SET message = ? WHERE journal_entry_id = ?`, revision.Message, discover.ID); err != nil {
		t.Fatalf("update revision search projection: %v", err)
	}
	linkedAt := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, `INSERT INTO specs (id, project_id, title, status, created_at, updated_at) VALUES (?, ?, 'sentinel spec', 'active', ?, ?)`, "spec-migration-sentinel", status.ProjectID, linkedAt, linkedAt); err != nil {
		t.Fatalf("insert linked spec sentinel: %v", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO tasks (id, project_id, spec_id, title, status, created_at, updated_at) VALUES (?, ?, ?, 'sentinel task', 'todo', ?, ?)`, "task-migration-sentinel", status.ProjectID, "spec-migration-sentinel", linkedAt, linkedAt); err != nil {
		t.Fatalf("insert linked task sentinel: %v", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE journal_entries SET spec_id = ?, task_id = ? WHERE id = ?`, "spec-migration-sentinel", "task-migration-sentinel", discover.ID); err != nil {
		t.Fatalf("link journal projection sentinels: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit revision seed: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close revision seed: %v", err)
	}
	otherRoot := projectRoot(t)
	if _, err := Initialize(ctx, otherRoot, resolver); err != nil {
		t.Fatalf("Initialize(other project) error = %v", err)
	}
	if _, err := LogJournal(ctx, otherRoot, resolver, JournalLogOptions{
		Entry:  "note(migration): other-project-only",
		Origin: &JournalOriginInput{EnvelopeVersion: JournalOriginEnvelopeVersion, CaptureMechanism: JournalOriginMechanismManual},
	}); err != nil {
		t.Fatalf("LogJournal(other project) error = %v", err)
	}
	backup, err := Backup(ctx, root, resolver)
	if err != nil {
		t.Fatalf("Backup() error = %v", err)
	}
	before := testFileSHA256(t, backup.BackupPath)

	encoded, err := ExportVNextRehearsalArchive(ctx, VNextRehearsalExportOptions{
		Backup: backup, ProjectID: status.ProjectID, Root: root, Resolver: resolver,
	})
	if err != nil {
		t.Fatalf("ExportVNextRehearsalArchive() error = %v", err)
	}
	replayed, err := ExportVNextRehearsalArchive(ctx, VNextRehearsalExportOptions{
		Backup: backup, ProjectID: status.ProjectID, Root: root, Resolver: resolver,
	})
	if err != nil || !bytes.Equal(encoded, replayed) {
		t.Fatalf("deterministic replay = (%d bytes, %v), want %d identical bytes", len(replayed), err, len(encoded))
	}
	if after := testFileSHA256(t, backup.BackupPath); after != before {
		t.Fatalf("backup digest changed during export: %s -> %s", before, after)
	}
	sealed, err := archive.Parse(encoded)
	if err != nil {
		t.Fatalf("archive.Parse(export) error = %v", err)
	}
	if sealed.Content.Project.ProjectID != continuity.ProjectID(status.ProjectID) ||
		sealed.Content.Project.LegacyProjectID != status.ProjectID || sealed.Content.Project.Label != status.ProjectName {
		t.Fatalf("exported project mapping = %#v", sealed.Content.Project)
	}
	if sealed.Content.Source.BackupSHA256 != before || sealed.Content.Source.JournalFactRows != 3 ||
		sealed.Content.Source.JournalProjectionRows != 2 || sealed.Content.Source.CollapsedRevisionRows != 1 ||
		sealed.Content.Source.JournalOriginRows != 1 || sealed.Content.Source.DroppedSpecLinks != 1 ||
		sealed.Content.Source.DroppedTaskLinks != 1 {
		t.Fatalf("exported source manifest = %#v", sealed.Content.Source)
	}
	if len(sealed.Content.Records) != 3 || sealed.Content.Records[0].Kind != archive.RecordProject ||
		sealed.Content.Records[1].Kind != archive.RecordJournal || sealed.Content.Records[1].SourceID != discover.ID ||
		sealed.Content.Records[1].Journal.Text != revision.Message ||
		sealed.Content.Records[2].Kind != archive.RecordWrap || sealed.Content.Records[2].SourceID != wrap.ID {
		t.Fatalf("exported records = %#v", sealed.Content.Records)
	}
	if sealed.Content.Expected.EffectiveJournalCount != 1 || sealed.Content.Expected.WrapCount != 1 {
		t.Fatalf("exported expected projection = %#v", sealed.Content.Expected)
	}
	for _, forbidden := range []string{
		"spec_id", "task_id", "backend", "credential", "sync_authority",
		"spec-migration-sentinel", "task-migration-sentinel", "other-project-only",
	} {
		if bytes.Contains(encoded, []byte(forbidden)) {
			t.Fatalf("archive contains excluded authority field %q", forbidden)
		}
	}
}

func TestExportVNextRehearsalArchivePreservesRootFactOrderAcrossDisplayTimesAndRevisions(t *testing.T) {
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
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin order seed: %v", err)
	}
	firstID := "zz-root-fact"
	secondID := "aa-root-fact"
	first := JournalFactPayload{
		EntryType: "note", Scope: "order", Message: "first root",
		CreatedAt: "2030-01-01T00:00:00Z", UpdatedAt: "2030-01-01T00:00:00Z",
	}
	second := JournalFactPayload{
		EntryType: "note", Scope: "order", Message: "second root",
		CreatedAt: "2020-01-01T00:00:00Z", UpdatedAt: "2020-01-01T00:00:00Z",
	}
	if _, err := appendJournalFactWithEnvTx(ctx, tx, status.ProjectID, firstID, first, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), "order-env"); err != nil {
		t.Fatalf("append first root: %v", err)
	}
	if err := insertJournalProjectionTx(ctx, tx, status.ProjectID, firstID, first); err != nil {
		t.Fatalf("insert first projection: %v", err)
	}
	if err := insertJournalSearchTx(ctx, tx, status.ProjectID, firstID, "", first.EntryType, first.Scope, first.Message); err != nil {
		t.Fatalf("insert first search row: %v", err)
	}
	if _, err := appendJournalFactWithEnvTx(ctx, tx, status.ProjectID, secondID, second, time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), "order-env"); err != nil {
		t.Fatalf("append second root: %v", err)
	}
	if err := insertJournalProjectionTx(ctx, tx, status.ProjectID, secondID, second); err != nil {
		t.Fatalf("insert second projection: %v", err)
	}
	if err := insertJournalSearchTx(ctx, tx, status.ProjectID, secondID, "", second.EntryType, second.Scope, second.Message); err != nil {
		t.Fatalf("insert second search row: %v", err)
	}
	revised := first
	revised.Message = "first root revised"
	revised.UpdatedAt = "2031-01-01T00:00:00Z"
	if err := appendJournalFactRevisionForImportTx(ctx, tx, status.ProjectID, firstID, revised, time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("append first revision: %v", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE journal_search SET message = ? WHERE journal_entry_id = ?`, revised.Message, firstID); err != nil {
		t.Fatalf("update revised search row: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit order seed: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close order seed: %v", err)
	}
	backup, err := Backup(ctx, root, resolver)
	if err != nil {
		t.Fatalf("Backup() error = %v", err)
	}
	encoded, err := ExportVNextRehearsalArchive(ctx, VNextRehearsalExportOptions{Backup: backup, ProjectID: status.ProjectID, Root: root, Resolver: resolver})
	if err != nil {
		t.Fatalf("ExportVNextRehearsalArchive() error = %v", err)
	}
	sealed, err := archive.Parse(encoded)
	if err != nil {
		t.Fatalf("archive.Parse() error = %v", err)
	}
	if len(sealed.Content.Records) != 3 || sealed.Content.Records[1].SourceID != firstID ||
		sealed.Content.Records[1].Journal.Text != revised.Message || sealed.Content.Records[2].SourceID != secondID {
		t.Fatalf("root fact order = %#v, want project, revised first root, second root", sealed.Content.Records)
	}
}

func TestVNextRehearsalIDV1UsesStableRoleSeparatedLengthFraming(t *testing.T) {
	got := vnextRehearsalIDV1("fact_migration_", "role", "ab", "c")
	want := "fact_migration_ac9bbd1934d1fdb74d8415fb5e52c471b2a2dcecda6c9c65e99d53e7aaebc29a"
	if got != want {
		t.Fatalf("vnextRehearsalIDV1() = %q, want fixed vector %q", got, want)
	}
	for _, separated := range []string{
		vnextRehearsalIDV1("fact_migration_", "role", "a", "bc"),
		vnextRehearsalIDV1("fact_migration_", "other-role", "ab", "c"),
		vnextRehearsalIDV1("journal_migration_", "role", "ab", "c"),
	} {
		if separated == got {
			t.Fatalf("domain-separated id %q collided with fixed vector", separated)
		}
	}
}

func TestParseVNextRehearsalTimeV1RejectsMalformedAndPreservesPreEpoch(t *testing.T) {
	if _, err := parseVNextRehearsalTimeV1("not-a-time", "journal created_at"); err == nil || !strings.Contains(err.Error(), "journal created_at") {
		t.Fatalf("malformed timestamp error = %v", err)
	}
	got, err := parseVNextRehearsalTimeV1("1969-12-31T23:59:59.999Z", "journal created_at")
	if err != nil || got != -1 {
		t.Fatalf("pre-epoch timestamp = (%d, %v), want (-1, nil)", got, err)
	}
	content, err := buildVNextRehearsalContentV1(
		BackupVerificationResult{SchemaVersion: CurrentSchemaVersion(), SHA256: strings.Repeat("a", 64), Bytes: 1},
		vnextRehearsalProjectV1{ID: "project-pre-epoch", Label: "Pre Epoch", CreatedAt: "1969-12-31T23:59:59.999Z"},
		vnextRehearsalFoldV1{Order: []string{}, Payloads: map[string]JournalFactPayload{}},
		map[string]vnextRehearsalProjectionV1{}, 0, 0, 0,
	)
	if err != nil {
		t.Fatalf("build pre-epoch rehearsal content: %v", err)
	}
	if _, err := archive.Seal(content); err == nil || !strings.Contains(err.Error(), "observed_at_millis") {
		t.Fatalf("archive.Seal(pre-epoch content) error = %v, want observation refusal", err)
	}
}

func TestDecodeVNextRehearsalJournalFactPayloadV1RejectsLossyJSON(t *testing.T) {
	valid := `{"entry_type":"note","message":"kept","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}`
	payload, err := decodeVNextRehearsalJournalFactPayloadV1(valid)
	if err != nil || payload.Message != "kept" {
		t.Fatalf("strict valid payload = (%#v, %v)", payload, err)
	}
	for name, raw := range map[string]string{
		"unknown":          strings.TrimSuffix(valid, "}") + `,"future_field":"must-not-drop"}`,
		"duplicate":        strings.Replace(valid, `"message":"kept"`, `"message":"first","message":"kept"`, 1),
		"trailing":         valid + `{}`,
		"missing required": strings.Replace(valid, `,"updated_at":"2026-01-01T00:00:00Z"`, "", 1),
		"null required":    strings.Replace(valid, `"message":"kept"`, `"message":null`, 1),
		"null optional":    strings.TrimSuffix(valid, "}") + `,"scope":null}`,
		"numeric member":   strings.Replace(valid, `"message":"kept"`, `"message":42`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := decodeVNextRehearsalJournalFactPayloadV1(raw); err == nil {
				t.Fatalf("decode lossy payload %q error = nil", raw)
			}
		})
	}
}

func TestExportVNextRehearsalArchiveRejectsUnknownLegacyJournalCategory(t *testing.T) {
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
	now := time.Now().UTC()
	payload := JournalFactPayload{
		EntryType: "legacy_session", Scope: "migration", Message: "must not coerce",
		CreatedAt: now.Format(time.RFC3339Nano), UpdatedAt: now.Format(time.RFC3339Nano),
	}
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin unknown-category seed: %v", err)
	}
	if _, err := appendJournalFactTx(ctx, tx, status.ProjectID, "legacy-journal-unknown", payload, now); err != nil {
		t.Fatalf("append unknown-category fact: %v", err)
	}
	if err := insertJournalProjectionTx(ctx, tx, status.ProjectID, "legacy-journal-unknown", payload); err != nil {
		t.Fatalf("insert unknown-category projection: %v", err)
	}
	if err := insertJournalSearchTx(ctx, tx, status.ProjectID, "legacy-journal-unknown", "", payload.EntryType, payload.Scope, payload.Message); err != nil {
		t.Fatalf("insert unknown-category search: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit unknown-category seed: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close unknown-category store: %v", err)
	}
	backup, err := Backup(ctx, root, resolver)
	if err != nil {
		t.Fatalf("Backup() error = %v", err)
	}

	_, err = ExportVNextRehearsalArchive(ctx, VNextRehearsalExportOptions{
		Backup: backup, ProjectID: status.ProjectID, Root: root, Resolver: resolver,
	})
	if err == nil || !strings.Contains(err.Error(), "legacy_session") || !strings.Contains(err.Error(), "legacy-journal-unknown") {
		t.Fatalf("unknown category export error = %v", err)
	}
}

func TestExportVNextRehearsalArchiveRejectsSelectedProjectFactProjectionDivergence(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	resolver := PathResolver{StateHome: t.TempDir()}
	status, err := Initialize(ctx, root, resolver)
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	logged, err := LogJournal(ctx, root, resolver, JournalLogOptions{Entry: "note(migration): canonical"})
	if err != nil {
		t.Fatalf("LogJournal() error = %v", err)
	}
	backup, err := Backup(ctx, root, resolver)
	if err != nil {
		t.Fatalf("Backup() error = %v", err)
	}
	store, err := OpenStore(backup.BackupPath)
	if err != nil {
		t.Fatalf("OpenStore(backup) error = %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `UPDATE journal_entries SET message = 'diverged' WHERE id = ?`, logged.ID); err != nil {
		t.Fatalf("diverge journal projection: %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `UPDATE journal_search SET message = 'diverged' WHERE journal_entry_id = ?`, logged.ID); err != nil {
		t.Fatalf("keep search projection aligned: %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		t.Fatalf("checkpoint divergent backup: %v", err)
	}
	var journalMode string
	if err := store.db.QueryRowContext(ctx, `PRAGMA journal_mode=DELETE`).Scan(&journalMode); err != nil {
		t.Fatalf("restore divergent backup journal mode: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close divergent store: %v", err)
	}
	backup = reverifyVNextRehearsalBackupForTest(t, ctx, backup)

	_, err = ExportVNextRehearsalArchive(ctx, VNextRehearsalExportOptions{
		Backup: backup, ProjectID: status.ProjectID, Root: root, Resolver: resolver,
	})
	if err == nil || !strings.Contains(err.Error(), "fact/projection") {
		t.Fatalf("divergent export error = %v", err)
	}
}

func TestExportVNextRehearsalArchiveRejectsLiveDatabaseAndAliases(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	resolver := PathResolver{StateHome: t.TempDir()}
	status, err := Initialize(ctx, root, resolver)
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	backup, err := Backup(ctx, root, resolver)
	if err != nil {
		t.Fatalf("Backup() error = %v", err)
	}

	live := backup
	live.BackupPath = status.DatabasePath
	_, err = ExportVNextRehearsalArchive(ctx, VNextRehearsalExportOptions{Backup: live, ProjectID: status.ProjectID, Root: root, Resolver: resolver})
	if err == nil || !strings.Contains(err.Error(), "standalone backup") {
		t.Fatalf("live database export error = %v", err)
	}
	forged := backup
	forged.DatabasePath = filepath.Join(t.TempDir(), "invented-live.sqlite")
	_, err = ExportVNextRehearsalArchive(ctx, VNextRehearsalExportOptions{Backup: forged, ProjectID: status.ProjectID, Root: root, Resolver: resolver})
	if err == nil || !strings.Contains(err.Error(), "authoritative live database") {
		t.Fatalf("forged live provenance export error = %v", err)
	}

	aliasPath := filepath.Join(t.TempDir(), "live-alias.sqlite")
	if err := os.Link(status.DatabasePath, aliasPath); err != nil {
		t.Fatalf("link live database alias: %v", err)
	}
	alias := backup
	alias.BackupPath = aliasPath
	_, err = ExportVNextRehearsalArchive(ctx, VNextRehearsalExportOptions{Backup: alias, ProjectID: status.ProjectID, Root: root, Resolver: resolver})
	if err == nil || !strings.Contains(err.Error(), "aliases the live database") {
		t.Fatalf("live database alias export error = %v", err)
	}
}

func TestExportVNextRehearsalArchiveRejectsBackupSQLiteSidecars(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	resolver := PathResolver{StateHome: t.TempDir()}
	status, err := Initialize(ctx, root, resolver)
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	backup, err := Backup(ctx, root, resolver)
	if err != nil {
		t.Fatalf("Backup() error = %v", err)
	}
	if err := os.WriteFile(backup.BackupPath+"-wal", []byte("sentinel"), 0o600); err != nil {
		t.Fatalf("write backup WAL sentinel: %v", err)
	}

	_, err = ExportVNextRehearsalArchive(ctx, VNextRehearsalExportOptions{Backup: backup, ProjectID: status.ProjectID, Root: root, Resolver: resolver})
	if err == nil || !strings.Contains(err.Error(), "SQLite sidecar -wal") {
		t.Fatalf("backup sidecar export error = %v", err)
	}
}

func TestExportVNextRehearsalArchiveRejectsSidecarCreatedAfterVerification(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	resolver := PathResolver{StateHome: t.TempDir()}
	status, err := Initialize(ctx, root, resolver)
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	backup, err := Backup(ctx, root, resolver)
	if err != nil {
		t.Fatalf("Backup() error = %v", err)
	}

	encoded, err := exportVNextRehearsalArchiveV1(
		ctx,
		VNextRehearsalExportOptions{Backup: backup, ProjectID: status.ProjectID, Root: root, Resolver: resolver},
		vnextRehearsalExportOperationsV1{afterVerification: func(path string) error {
			return os.WriteFile(path+"-wal", []byte("late sidecar"), 0o600)
		}},
	)
	if err == nil || !strings.Contains(err.Error(), "SQLite sidecar -wal") || encoded != nil {
		t.Fatalf("late sidecar export = (%d bytes, %v), want nil bytes and sidecar refusal", len(encoded), err)
	}
}

func TestExportVNextRehearsalArchiveRejectsReplacementAfterVerification(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	resolver := PathResolver{StateHome: t.TempDir()}
	status, err := Initialize(ctx, root, resolver)
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	backup, err := Backup(ctx, root, resolver)
	if err != nil {
		t.Fatalf("Backup() error = %v", err)
	}

	encoded, err := exportVNextRehearsalArchiveV1(
		ctx,
		VNextRehearsalExportOptions{Backup: backup, ProjectID: status.ProjectID, Root: root, Resolver: resolver},
		vnextRehearsalExportOperationsV1{afterVerification: replaceVNextRehearsalBackupForTest(t)},
	)
	if err == nil || !strings.Contains(err.Error(), "changed after verification") || encoded != nil {
		t.Fatalf("replacement export = (%d bytes, %v), want nil bytes and change refusal", len(encoded), err)
	}
}

func TestExportVNextRehearsalArchiveRejectsMutationAfterReadSnapshot(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	resolver := PathResolver{StateHome: t.TempDir()}
	status, err := Initialize(ctx, root, resolver)
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	backup, err := Backup(ctx, root, resolver)
	if err != nil {
		t.Fatalf("Backup() error = %v", err)
	}

	encoded, err := exportVNextRehearsalArchiveV1(
		ctx,
		VNextRehearsalExportOptions{Backup: backup, ProjectID: status.ProjectID, Root: root, Resolver: resolver},
		vnextRehearsalExportOperationsV1{afterReadSnapshot: mutateVNextRehearsalBackupForTest(t)},
	)
	if err == nil || !strings.Contains(err.Error(), "changed during export") || encoded != nil {
		t.Fatalf("mutated export = (%d bytes, %v), want nil bytes and change refusal", len(encoded), err)
	}
}

func TestExportVNextRehearsalArchiveReportsPrivateCleanupFailuresOnEveryPath(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	resolver := PathResolver{StateHome: t.TempDir()}
	status, err := Initialize(ctx, root, resolver)
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	backup, err := Backup(ctx, root, resolver)
	if err != nil {
		t.Fatalf("Backup() error = %v", err)
	}

	removeWithError := func(path string) error {
		if err := os.RemoveAll(path); err != nil {
			return err
		}
		return errors.New("simulated cleanup failure")
	}
	encoded, err := exportVNextRehearsalArchiveV1(
		ctx,
		VNextRehearsalExportOptions{Backup: backup, ProjectID: status.ProjectID, Root: root, Resolver: resolver},
		vnextRehearsalExportOperationsV1{removePrivateSnapshot: removeWithError},
	)
	if err == nil || !strings.Contains(err.Error(), "remove private snapshot directory") || encoded != nil {
		t.Fatalf("cleanup failure export = (%d bytes, %v), want nil bytes and cleanup refusal", len(encoded), err)
	}
	encoded, err = exportVNextRehearsalArchiveV1(
		ctx,
		VNextRehearsalExportOptions{Backup: backup, ProjectID: status.ProjectID, Root: root, Resolver: resolver},
		vnextRehearsalExportOperationsV1{
			afterVerification:     func(string) error { return errors.New("simulated primary failure") },
			removePrivateSnapshot: removeWithError,
		},
	)
	if err == nil || !strings.Contains(err.Error(), "simulated primary failure") ||
		!strings.Contains(err.Error(), "remove private snapshot directory") || encoded != nil {
		t.Fatalf("joined cleanup failure export = (%d bytes, %v), want nil bytes and both errors", len(encoded), err)
	}
}

func reverifyVNextRehearsalBackupForTest(t *testing.T, ctx context.Context, backup BackupResult) BackupResult {
	t.Helper()
	verification, err := VerifyBackup(ctx, backup.BackupPath)
	if err != nil {
		t.Fatalf("VerifyBackup() error = %v", err)
	}
	backup.BackupPath = verification.BackupPath
	backup.ContractVersion = verification.ContractVersion
	backup.DatabaseScope = verification.DatabaseScope
	backup.Bytes = verification.Bytes
	backup.SHA256 = verification.SHA256
	backup.Verified = verification.Verified
	backup.SchemaVersion = verification.SchemaVersion
	backup.ProjectCount = verification.ProjectCount
	backup.IntegrityCheck = verification.IntegrityCheck
	backup.ForeignKeyCheck = verification.ForeignKeyCheck
	backup.JournalRetrievalReady = verification.JournalRetrievalReady
	backup.JournalSearchParity = verification.JournalSearchParity
	backup.JournalProvenanceIntegrity = verification.JournalProvenanceIntegrity
	backup.SQLiteValid = verification.SQLiteValid
	backup.RecoveryReady = verification.RecoveryReady
	backup.JournalWatermark = verification.JournalWatermark
	return backup
}

func replaceVNextRehearsalBackupForTest(t *testing.T) func(string) error {
	t.Helper()
	return func(path string) error {
		contents, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		replacement := path + ".replacement"
		if err := os.WriteFile(replacement, contents, 0o600); err != nil {
			return err
		}
		return os.Rename(replacement, path)
	}
}

func mutateVNextRehearsalBackupForTest(t *testing.T) func(string) error {
	t.Helper()
	return func(path string) error {
		file, err := os.OpenFile(path, os.O_RDWR, 0)
		if err != nil {
			return err
		}
		defer file.Close()
		info, err := file.Stat()
		if err != nil {
			return err
		}
		offset := info.Size() - 1
		var value [1]byte
		if _, err := file.ReadAt(value[:], offset); err != nil {
			return err
		}
		value[0] ^= 0xff
		_, err = file.WriteAt(value[:], offset)
		return err
	}
}
