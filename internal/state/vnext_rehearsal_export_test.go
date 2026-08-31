package state

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/levifig/loaf/internal/project"
	"github.com/levifig/loaf/vnext/continuity"
	continuitysqlite "github.com/levifig/loaf/vnext/continuity/sqlite"
	"github.com/levifig/loaf/vnext/migration/archive"
	"github.com/levifig/loaf/vnext/migration/rehearsal"
)

func TestExportVNextRehearsalArchiveExportsUnparsedLegacyHandoffsDeterministically(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	resolver := PathResolver{StateHome: t.TempDir()}
	status, err := Initialize(ctx, root, resolver)
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	body := "  # Exact Markdown\n\n```text\nkeep trailing spaces  \n```\n\n- café\n"
	store, err := OpenStore(status.DatabasePath)
	if err != nil {
		t.Fatalf("OpenStore(harness provenance seed) error = %v", err)
	}
	now := "2026-01-01T00:00:00Z"
	mustExec(t, store, `INSERT INTO sessions (id, project_id, harness_session_id, status, created_at, updated_at) VALUES ('session-handoff-export', ?, 'session-handoff-export', 'active', ?, ?)`, status.ProjectID, now, now)
	if err := store.Close(); err != nil {
		t.Fatalf("close harness provenance seed: %v", err)
	}
	created, err := CreateArtifactEntity(ctx, root, resolver, ArtifactEntityCreateOptions{
		Kind: "handoff", Title: "Exact title", Body: body, HarnessSessionID: "session-handoff-export",
	})
	if err != nil {
		t.Fatalf("CreateArtifactEntity(handoff) error = %v", err)
	}
	backup, err := Backup(ctx, root, resolver)
	if err != nil {
		t.Fatalf("Backup() error = %v", err)
	}

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
		t.Fatalf("deterministic handoff replay = (%d bytes, %v), want %d identical bytes", len(replayed), err, len(encoded))
	}
	sealed, err := archive.Parse(encoded)
	if err != nil {
		t.Fatalf("archive.Parse(export) error = %v", err)
	}
	if sealed.Content.Source.HandoffRows != 1 ||
		sealed.Content.Source.HandoffMapping != archive.HandoffMappingUnparsedLegacyV1 ||
		!sealed.Content.Families.Handoffs || sealed.Content.Expected.HandoffCount != 1 {
		t.Fatalf("handoff source/family manifest = source %#v families %#v expected %#v", sealed.Content.Source, sealed.Content.Families, sealed.Content.Expected)
	}
	if len(sealed.Content.Records) != 2 {
		t.Fatalf("records = %#v, want project plus handoff", sealed.Content.Records)
	}
	record := sealed.Content.Records[1]
	if record.Kind != archive.RecordHandoff || record.SourceID != created.Entity.Alias ||
		record.Handoff == nil || record.Handoff.Purpose != "Exact title" || record.Handoff.Situation != body ||
		record.Handoff.NextActions != "" || record.Handoff.QuestionsAndRisks != "" ||
		record.Handoff.SuggestedSkills == nil || len(record.Handoff.SuggestedSkills) != 0 ||
		record.Observation.HarnessSessionID != "session-handoff-export" || record.Observation.Branch != "" ||
		record.Observation.Worktree != "" {
		t.Fatalf("exported handoff record = %#v", record)
	}
	wantFactID := continuity.FactID(vnextRehearsalIDV1("fact_migration_", "handoff-fact", status.ProjectID, created.Entity.ID))
	wantSubjectID := continuity.SubjectID(vnextRehearsalIDV1("handoff_migration_", "handoff-subject", status.ProjectID, created.Entity.ID))
	if record.FactID != wantFactID || record.SubjectID != wantSubjectID || string(record.FactID) == string(record.SubjectID) {
		t.Fatalf("handoff ids = %q/%q, want %q/%q", record.FactID, record.SubjectID, wantFactID, wantSubjectID)
	}

	destination, err := continuitysqlite.Open(filepath.Join(t.TempDir(), "destination"), "environment-handoff-export")
	if err != nil {
		t.Fatalf("sqlite.Open(destination) error = %v", err)
	}
	t.Cleanup(func() { _ = destination.Close() })
	if _, err := rehearsal.Import(ctx, encoded, destination); err != nil {
		t.Fatalf("rehearsal.Import(export) error = %v", err)
	}
	snapshot, err := destination.Snapshot(ctx, continuity.ProjectID(status.ProjectID), continuity.SnapshotRequest{})
	if err != nil {
		t.Fatalf("destination.Snapshot() error = %v", err)
	}
	if len(snapshot.LatestHandoffs.Handoffs) != 1 || snapshot.LatestHandoffs.Handoffs[0].Purpose != "Exact title" ||
		snapshot.LatestHandoffs.Handoffs[0].Situation != body {
		t.Fatalf("imported handoff snapshot = %#v", snapshot.LatestHandoffs)
	}
}

func TestExportVNextRehearsalArchiveAcceptsMigrationBornHandoffWithoutFactAlias(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	resolver := PathResolver{StateHome: t.TempDir()}
	status, err := Initialize(ctx, root, resolver)
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	handoff, err := CreateArtifactEntity(ctx, root, resolver, ArtifactEntityCreateOptions{Kind: "handoff", Title: "Migration-born", Body: "# Legacy body\n"})
	if err != nil {
		t.Fatalf("CreateArtifactEntity(handoff) error = %v", err)
	}
	store, err := OpenStore(status.DatabasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	var raw string
	if err := store.db.QueryRow(`SELECT payload FROM facts WHERE id = ?`, handoff.EventID).Scan(&raw); err != nil {
		t.Fatalf("read handoff fact: %v", err)
	}
	payload, err := decodeCoreEventPayload(raw)
	if err != nil {
		t.Fatalf("decode handoff fact: %v", err)
	}
	payload.Alias = ""
	payload.Note = ""
	raw, err = encodeCoreEventPayload(payload)
	if err != nil {
		t.Fatalf("encode migration-born handoff fact: %v", err)
	}
	mustExec(t, store, `UPDATE facts SET payload = ? WHERE id = ?`, raw, handoff.EventID)
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
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
	if len(sealed.Content.Records) != 2 || sealed.Content.Records[1].SourceID != handoff.Entity.Alias {
		t.Fatalf("migration-born handoff records = %#v", sealed.Content.Records)
	}
}

func TestExportVNextRehearsalArchiveMergesJournalAndHandoffRootFactOrder(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	resolver := PathResolver{StateHome: t.TempDir()}
	status, err := Initialize(ctx, root, resolver)
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	handoff, err := CreateArtifactEntity(ctx, root, resolver, ArtifactEntityCreateOptions{Kind: "handoff", Title: "First root", Body: "handoff body"})
	if err != nil {
		t.Fatalf("CreateArtifactEntity(handoff) error = %v", err)
	}
	journal, err := LogJournal(ctx, root, resolver, JournalLogOptions{Entry: "discover(order): second root"})
	if err != nil {
		t.Fatalf("LogJournal() error = %v", err)
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
		t.Fatalf("fold journal facts: %v", err)
	}
	revised := folded[journal.ID].payload
	revised.Message = "second root revised after handoff"
	if err := appendJournalFactRevisionForImportTx(ctx, tx, status.ProjectID, journal.ID, revised, time.Now().UTC().Add(time.Second)); err != nil {
		t.Fatalf("append journal revision: %v", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE journal_search SET message = ? WHERE journal_entry_id = ?`, revised.Message, journal.ID); err != nil {
		t.Fatalf("update journal search: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit revision seed: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close revision seed: %v", err)
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
	if len(sealed.Content.Records) != 3 || sealed.Content.Records[1].Kind != archive.RecordHandoff ||
		sealed.Content.Records[1].SourceID != handoff.Entity.Alias || sealed.Content.Records[2].Kind != archive.RecordJournal ||
		sealed.Content.Records[2].SourceID != journal.ID || sealed.Content.Records[2].Journal.Text != revised.Message {
		t.Fatalf("global root fact order = %#v", sealed.Content.Records)
	}
}

func TestExportVNextRehearsalArchiveRejectsMalformedAndNoncanonicalFactHLC(t *testing.T) {
	for name, tc := range map[string]struct {
		create func(context.Context, project.Root, PathResolver) (string, error)
		rawHLC string
	}{
		"journal noncanonical": {
			create: func(ctx context.Context, root project.Root, resolver PathResolver) (string, error) {
				logged, err := LogJournal(ctx, root, resolver, JournalLogOptions{Entry: "note(hlc): journal"})
				return logged.ID, err
			},
			rawHLC: "1:0",
		},
		"handoff malformed": {
			create: func(ctx context.Context, root project.Root, resolver PathResolver) (string, error) {
				created, err := CreateArtifactEntity(ctx, root, resolver, ArtifactEntityCreateOptions{Kind: "handoff", Title: "HLC", Body: "body"})
				return created.EventID, err
			},
			rawHLC: "not-an-hlc",
		},
	} {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			root := projectRoot(t)
			resolver := PathResolver{StateHome: t.TempDir()}
			status, err := Initialize(ctx, root, resolver)
			if err != nil {
				t.Fatalf("Initialize() error = %v", err)
			}
			factID, err := tc.create(ctx, root, resolver)
			if err != nil {
				t.Fatalf("create fixture: %v", err)
			}
			store, err := OpenStore(status.DatabasePath)
			if err != nil {
				t.Fatalf("OpenStore() error = %v", err)
			}
			mustExec(t, store, `UPDATE facts SET hlc = ? WHERE id = ?`, tc.rawHLC, factID)
			if err := store.Close(); err != nil {
				t.Fatalf("Close() error = %v", err)
			}
			backup, err := Backup(ctx, root, resolver)
			if err != nil {
				t.Fatalf("Backup() error = %v", err)
			}
			_, err = ExportVNextRehearsalArchive(ctx, VNextRehearsalExportOptions{Backup: backup, ProjectID: status.ProjectID, Root: root, Resolver: resolver})
			if err == nil || !strings.Contains(strings.ToLower(err.Error()), "hlc") {
				t.Fatalf("ExportVNextRehearsalArchive() error = %v, want HLC refusal", err)
			}
		})
	}
}

func TestValidateVNextRehearsalHandoffFactBoundsV1RejectsBeforeDecode(t *testing.T) {
	for name, tc := range map[string]struct {
		rows     int
		rawBytes int64
		budget   vnextRehearsalArchiveBudgetV1
		want     string
	}{
		"selected fact bytes": {rows: 1, rawBytes: vnextRehearsalMaxAggregatePayloadBytesV1 + 1, want: "handoff fact selected bytes"},
		"archive-wide records": {
			rows: 2, budget: vnextRehearsalArchiveBudgetV1{RecordCount: vnextRehearsalMaxRecordsV1 - 1}, want: "record limit",
		},
		"archive-wide journal payload": {
			rows: 1, budget: vnextRehearsalArchiveBudgetV1{PayloadBytes: vnextRehearsalMaxAggregatePayloadBytesV1 + 1}, want: "journal projection already exceeds",
		},
	} {
		t.Run(name, func(t *testing.T) {
			err := validateVNextRehearsalHandoffFactBoundsV1(tc.rows, tc.rawBytes, tc.budget)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("validateVNextRehearsalHandoffFactBoundsV1() error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestReadVNextRehearsalHandoffProjectionV1IncludesStartingArchiveBudget(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	resolver := PathResolver{StateHome: t.TempDir()}
	status, err := Initialize(ctx, root, resolver)
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	_, err = CreateArtifactEntity(ctx, root, resolver, ArtifactEntityCreateOptions{Kind: "handoff", Title: "Bounded", Body: "body"})
	if err != nil {
		t.Fatalf("CreateArtifactEntity(handoff) error = %v", err)
	}
	store, err := OpenStore(status.DatabasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	tx, err := store.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		t.Fatalf("BeginTx() error = %v", err)
	}
	defer tx.Rollback()
	_, err = readVNextRehearsalHandoffProjectionV1(ctx, tx, status.ProjectID, vnextRehearsalArchiveBudgetV1{
		PayloadBytes: vnextRehearsalMaxAggregatePayloadBytesV1 - 1,
	})
	if err == nil || !strings.Contains(err.Error(), "archive aggregate payload") {
		t.Fatalf("readVNextRehearsalHandoffProjectionV1() error = %v, want aggregate bound", err)
	}
}

func TestReadVNextRehearsalHandoffProjectionV1PreflightsOversizedSelectedBody(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	resolver := PathResolver{StateHome: t.TempDir()}
	status, err := Initialize(ctx, root, resolver)
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	handoff, err := CreateArtifactEntity(ctx, root, resolver, ArtifactEntityCreateOptions{Kind: "handoff", Title: "Oversized", Body: "body"})
	if err != nil {
		t.Fatalf("CreateArtifactEntity(handoff) error = %v", err)
	}
	store, err := OpenStore(status.DatabasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()
	mustExec(t, store, `UPDATE artifact_bodies SET content = zeroblob(?) WHERE project_id = ? AND entity_kind = 'handoff' AND entity_id = ?`,
		vnextRehearsalMaxAggregatePayloadBytesV1+1, status.ProjectID, handoff.Entity.ID)
	tx, err := store.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		t.Fatalf("BeginTx() error = %v", err)
	}
	defer tx.Rollback()
	_, err = readVNextRehearsalHandoffProjectionV1(ctx, tx, status.ProjectID, vnextRehearsalArchiveBudgetV1{})
	if err == nil || !strings.Contains(err.Error(), "handoff projection selected bytes") {
		t.Fatalf("readVNextRehearsalHandoffProjectionV1() error = %v, want selected-byte preflight refusal", err)
	}
}

func TestReadVNextRehearsalHandoffFactsV1PreflightsOversizedSelectedEnvironmentID(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	resolver := PathResolver{StateHome: t.TempDir()}
	status, err := Initialize(ctx, root, resolver)
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	handoff, err := CreateArtifactEntity(ctx, root, resolver, ArtifactEntityCreateOptions{Kind: "handoff", Title: "Oversized", Body: "body"})
	if err != nil {
		t.Fatalf("CreateArtifactEntity(handoff) error = %v", err)
	}
	store, err := OpenStore(status.DatabasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()
	mustExec(t, store, `UPDATE facts SET env_id = printf('%*s', ?, 'x') WHERE id = ?`,
		vnextRehearsalMaxAggregatePayloadBytesV1+1, handoff.EventID)
	tx, err := store.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		t.Fatalf("BeginTx() error = %v", err)
	}
	defer tx.Rollback()
	_, err = readVNextRehearsalHandoffFactsV1(ctx, tx, status.ProjectID, vnextRehearsalArchiveBudgetV1{})
	if err == nil || !strings.Contains(err.Error(), "handoff fact selected bytes") {
		t.Fatalf("readVNextRehearsalHandoffFactsV1() error = %v, want selected-byte preflight refusal", err)
	}
}

func TestExportVNextRehearsalArchiveRejectsInvalidUTF8MigrationBornAlias(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	resolver := PathResolver{StateHome: t.TempDir()}
	status, err := Initialize(ctx, root, resolver)
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	handoff, err := CreateArtifactEntity(ctx, root, resolver, ArtifactEntityCreateOptions{Kind: "handoff", Title: "Migration-born", Body: "body"})
	if err != nil {
		t.Fatalf("CreateArtifactEntity(handoff) error = %v", err)
	}
	store, err := OpenStore(status.DatabasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	var raw string
	if err := store.db.QueryRow(`SELECT payload FROM facts WHERE id = ?`, handoff.EventID).Scan(&raw); err != nil {
		t.Fatalf("read handoff fact: %v", err)
	}
	payload, err := decodeCoreEventPayload(raw)
	if err != nil {
		t.Fatalf("decode handoff fact: %v", err)
	}
	payload.Alias = ""
	raw, err = encodeCoreEventPayload(payload)
	if err != nil {
		t.Fatalf("encode migration-born handoff fact: %v", err)
	}
	mustExec(t, store, `UPDATE facts SET payload = ? WHERE id = ?`, raw, handoff.EventID)
	mustExec(t, store, `UPDATE aliases SET alias = CAST(x'ff' AS TEXT) WHERE project_id = ? AND entity_kind = 'handoff' AND entity_id = ?`, status.ProjectID, handoff.Entity.ID)
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	backup, err := Backup(ctx, root, resolver)
	if err != nil {
		t.Fatalf("Backup() error = %v", err)
	}
	_, err = ExportVNextRehearsalArchive(ctx, VNextRehearsalExportOptions{Backup: backup, ProjectID: status.ProjectID, Root: root, Resolver: resolver})
	if err == nil || !strings.Contains(err.Error(), "UTF-8") {
		t.Fatalf("ExportVNextRehearsalArchive() error = %v, want invalid UTF-8 refusal", err)
	}
}

func TestExportVNextRehearsalArchiveOrdersHandoffsByCanonicalFactOrder(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	resolver := PathResolver{StateHome: t.TempDir()}
	status, err := Initialize(ctx, root, resolver)
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	first, err := CreateArtifactEntity(ctx, root, resolver, ArtifactEntityCreateOptions{Kind: "handoff", Title: "First fact", Body: "first body"})
	if err != nil {
		t.Fatalf("CreateArtifactEntity(first) error = %v", err)
	}
	second, err := CreateArtifactEntity(ctx, root, resolver, ArtifactEntityCreateOptions{Kind: "handoff", Title: "Second fact", Body: "second body"})
	if err != nil {
		t.Fatalf("CreateArtifactEntity(second) error = %v", err)
	}
	store, err := OpenStore(status.DatabasePath)
	if err != nil {
		t.Fatalf("OpenStore(order seed) error = %v", err)
	}
	setDisplayTime := func(handoff ArtifactEntityCreateResult, value string) {
		t.Helper()
		mustExec(t, store, `UPDATE handoffs SET created_at = ?, updated_at = ? WHERE project_id = ? AND id = ?`, value, value, status.ProjectID, handoff.Entity.ID)
		mustExec(t, store, `UPDATE aliases SET created_at = ?, updated_at = ? WHERE project_id = ? AND entity_kind = 'handoff' AND entity_id = ?`, value, value, status.ProjectID, handoff.Entity.ID)
		mustExec(t, store, `UPDATE artifact_bodies SET created_at = ?, updated_at = ? WHERE project_id = ? AND entity_kind = 'handoff' AND entity_id = ?`, value, value, status.ProjectID, handoff.Entity.ID)
		var raw string
		if err := store.db.QueryRow(`SELECT payload FROM facts WHERE id = ?`, handoff.EventID).Scan(&raw); err != nil {
			t.Fatalf("read handoff fact: %v", err)
		}
		payload, err := decodeCoreEventPayload(raw)
		if err != nil {
			t.Fatalf("decode handoff fact: %v", err)
		}
		payload.CreatedAt, payload.UpdatedAt = value, value
		raw, err = encodeCoreEventPayload(payload)
		if err != nil {
			t.Fatalf("encode handoff fact: %v", err)
		}
		mustExec(t, store, `UPDATE facts SET payload = ? WHERE id = ?`, raw, handoff.EventID)
	}
	setDisplayTime(first, "2030-01-01T00:00:00Z")
	setDisplayTime(second, "2020-01-01T00:00:00Z")
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
	if len(sealed.Content.Records) != 3 || sealed.Content.Records[1].SourceID != first.Entity.Alias ||
		sealed.Content.Records[2].SourceID != second.Entity.Alias {
		t.Fatalf("handoff fact order = %#v, want first-created fact before older display timestamp", sealed.Content.Records)
	}
}

func TestDecodeVNextRehearsalHandoffFactPayloadV1RejectsLossyJSON(t *testing.T) {
	valid := `{"subject_kind":"handoff","subject_id":"handoff-one","alias":"HANDOFF-1","status":"draft","title":"Title","body":"Body","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}`
	for name, raw := range map[string]string{
		"unknown":              strings.TrimSuffix(valid, "}") + `,"future_field":"must-not-drop"}`,
		"duplicate":            strings.Replace(valid, `"title":"Title"`, `"title":"First","title":"Title"`, 1),
		"non-string":           strings.Replace(valid, `"title":"Title"`, `"title":42`, 1),
		"unsupported nonempty": strings.TrimSuffix(valid, "}") + `,"text":"must-not-coerce"}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := decodeVNextRehearsalHandoffFactPayloadV1(raw); err == nil {
				t.Fatalf("decode lossy handoff payload %q error = nil", raw)
			}
		})
	}
	invalidUTF8 := string(append([]byte(strings.Replace(valid, `"title":"Title"`, `"title":"`, 1)), 0xff, '"', '}'))
	if _, err := decodeVNextRehearsalHandoffFactPayloadV1(invalidUTF8); err == nil || !strings.Contains(err.Error(), "UTF-8") {
		t.Fatalf("decode invalid UTF-8 handoff payload error = %v", err)
	}
}

func TestExportVNextRehearsalArchiveRejectsUnsupportedLegacyHandoffShapes(t *testing.T) {
	for name, mutate := range map[string]func(*testing.T, *Store, string, ArtifactEntityCreateResult){
		"divergent body and fact": func(t *testing.T, store *Store, projectID string, handoff ArtifactEntityCreateResult) {
			mustExec(t, store, `UPDATE artifact_bodies SET content = 'diverged', content_hash = ? WHERE project_id = ? AND entity_kind = 'handoff' AND entity_id = ?`, artifactBodyHash("diverged"), projectID, handoff.Entity.ID)
		},
		"non-draft status": func(t *testing.T, store *Store, projectID string, handoff ArtifactEntityCreateResult) {
			mustExec(t, store, `UPDATE handoffs SET status = 'done' WHERE project_id = ? AND id = ?`, projectID, handoff.Entity.ID)
		},
		"task metadata": func(t *testing.T, store *Store, projectID string, handoff ArtifactEntityCreateResult) {
			now := "2026-01-01T00:00:00Z"
			mustExec(t, store, `INSERT INTO specs (id, project_id, title, status, created_at, updated_at) VALUES ('spec-handoff', ?, 'Spec', 'active', ?, ?)`, projectID, now, now)
			mustExec(t, store, `INSERT INTO tasks (id, project_id, spec_id, title, status, created_at, updated_at) VALUES ('task-handoff', ?, 'spec-handoff', 'Task', 'todo', ?, ?)`, projectID, now, now)
			mustExec(t, store, `UPDATE handoffs SET task_id = 'task-handoff' WHERE project_id = ? AND id = ?`, projectID, handoff.Entity.ID)
		},
		"body source metadata": func(t *testing.T, store *Store, projectID string, handoff ArtifactEntityCreateResult) {
			now := "2026-01-01T00:00:00Z"
			mustExec(t, store, `INSERT INTO sources (id, project_id, source_kind, path, created_at, updated_at) VALUES ('source-handoff', ?, 'file', 'handoff.md', ?, ?)`, projectID, now, now)
			mustExec(t, store, `UPDATE handoffs SET body_source_id = 'source-handoff' WHERE project_id = ? AND id = ?`, projectID, handoff.Entity.ID)
		},
		"relationship metadata": func(t *testing.T, store *Store, projectID string, handoff ArtifactEntityCreateResult) {
			now := "2026-01-01T00:00:00Z"
			mustExec(t, store, `INSERT INTO relationships (id, project_id, from_entity_kind, from_entity_id, to_entity_kind, to_entity_id, relationship_type, created_at, updated_at) VALUES ('rel-handoff', ?, 'handoff', ?, 'project', ?, 'relates_to', ?, ?)`, projectID, handoff.Entity.ID, projectID, now, now)
		},
		"entity tag metadata": func(t *testing.T, store *Store, projectID string, handoff ArtifactEntityCreateResult) {
			now := "2026-01-01T00:00:00Z"
			mustExec(t, store, `INSERT INTO tags (id, project_id, name, created_at, updated_at) VALUES ('tag-handoff', ?, 'handoff-tag', ?, ?)`, projectID, now, now)
			mustExec(t, store, `INSERT INTO entity_tags (id, project_id, tag_id, entity_kind, entity_id, created_at, updated_at) VALUES ('entity-tag-handoff', ?, 'tag-handoff', 'handoff', ?, ?, ?)`, projectID, handoff.Entity.ID, now, now)
		},
		"backend mapping metadata": func(t *testing.T, store *Store, projectID string, handoff ArtifactEntityCreateResult) {
			now := "2026-01-01T00:00:00Z"
			mustExec(t, store, `INSERT INTO backend_mappings (id, project_id, backend, entity_kind, entity_id, external_kind, external_id, sync_status, created_at, updated_at) VALUES ('mapping-handoff', ?, 'linear', 'handoff', ?, 'issue', 'EXT-1', 'synced', ?, ?)`, projectID, handoff.Entity.ID, now, now)
		},
		"status event metadata": func(t *testing.T, store *Store, projectID string, handoff ArtifactEntityCreateResult) {
			now := "2026-01-01T00:00:00Z"
			mustExec(t, store, `INSERT INTO events (id, project_id, entity_kind, entity_id, event_type, to_status, created_at, updated_at) VALUES ('event-handoff', ?, 'handoff', ?, 'status_changed', 'draft', ?, ?)`, projectID, handoff.Entity.ID, now, now)
		},
		"alternate alias metadata": func(t *testing.T, store *Store, projectID string, handoff ArtifactEntityCreateResult) {
			now := "2026-01-01T00:00:00Z"
			mustExec(t, store, `INSERT INTO aliases (id, project_id, entity_kind, entity_id, namespace, alias, created_at, updated_at) VALUES ('alias-handoff-extra', ?, 'handoff', ?, 'alternate', 'HANDOFF-EXTRA', ?, ?)`, projectID, handoff.Entity.ID, now, now)
		},
		"bundle member metadata": func(t *testing.T, store *Store, projectID string, handoff ArtifactEntityCreateResult) {
			now := "2026-01-01T00:00:00Z"
			mustExec(t, store, `INSERT INTO bundles (id, project_id, slug, title, created_at, updated_at) VALUES ('bundle-handoff', ?, 'handoff-bundle', 'Handoff bundle', ?, ?)`, projectID, now, now)
			mustExec(t, store, `INSERT INTO bundle_members (id, project_id, bundle_id, entity_kind, entity_id, created_at, updated_at) VALUES ('bundle-member-handoff', ?, 'bundle-handoff', 'handoff', ?, ?, ?)`, projectID, handoff.Entity.ID, now, now)
		},
		"artifact body source metadata": func(t *testing.T, store *Store, projectID string, handoff ArtifactEntityCreateResult) {
			now := "2026-01-01T00:00:00Z"
			mustExec(t, store, `INSERT INTO sources (id, project_id, source_kind, path, created_at, updated_at) VALUES ('source-body-handoff', ?, 'file', 'handoff-body.md', ?, ?)`, projectID, now, now)
			mustExec(t, store, `UPDATE artifact_bodies SET source_id = 'source-body-handoff' WHERE project_id = ? AND entity_kind = 'handoff' AND entity_id = ?`, projectID, handoff.Entity.ID)
		},
		"incorrect body SHA-256": func(t *testing.T, store *Store, projectID string, handoff ArtifactEntityCreateResult) {
			mustExec(t, store, `UPDATE artifact_bodies SET content_hash = ? WHERE project_id = ? AND entity_kind = 'handoff' AND entity_id = ?`, strings.Repeat("0", 64), projectID, handoff.Entity.ID)
		},
		"collapsed fact revision": func(t *testing.T, store *Store, _ string, handoff ArtifactEntityCreateResult) {
			mustExec(t, store, `INSERT INTO facts (id, project_id, kind, payload, env_id, seq, hlc, envelope_v) SELECT id || '-revision', project_id, kind, payload, env_id || '-revision', seq, hlc, envelope_v FROM facts WHERE id = ?`, handoff.EventID)
		},
		"unknown fact field": func(t *testing.T, store *Store, _ string, handoff ArtifactEntityCreateResult) {
			var raw string
			if err := store.db.QueryRow(`SELECT payload FROM facts WHERE id = ?`, handoff.EventID).Scan(&raw); err != nil {
				t.Fatalf("read handoff fact: %v", err)
			}
			raw = strings.TrimSuffix(raw, "}") + `,"future_field":"must-not-drop"}`
			mustExec(t, store, `UPDATE facts SET payload = ? WHERE id = ?`, raw, handoff.EventID)
		},
		"non-millisecond timestamp": func(t *testing.T, store *Store, projectID string, handoff ArtifactEntityCreateResult) {
			value := "2026-01-01T00:00:00.000000001Z"
			mustExec(t, store, `UPDATE handoffs SET created_at = ?, updated_at = ? WHERE project_id = ? AND id = ?`, value, value, projectID, handoff.Entity.ID)
			mustExec(t, store, `UPDATE aliases SET created_at = ?, updated_at = ? WHERE project_id = ? AND entity_kind = 'handoff' AND entity_id = ?`, value, value, projectID, handoff.Entity.ID)
			mustExec(t, store, `UPDATE artifact_bodies SET created_at = ?, updated_at = ? WHERE project_id = ? AND entity_kind = 'handoff' AND entity_id = ?`, value, value, projectID, handoff.Entity.ID)
			var raw string
			if err := store.db.QueryRow(`SELECT payload FROM facts WHERE id = ?`, handoff.EventID).Scan(&raw); err != nil {
				t.Fatalf("read handoff fact: %v", err)
			}
			payload, err := decodeCoreEventPayload(raw)
			if err != nil {
				t.Fatalf("decode handoff fact: %v", err)
			}
			payload.CreatedAt, payload.UpdatedAt = value, value
			encoded, err := encodeCoreEventPayload(payload)
			if err != nil {
				t.Fatalf("encode handoff fact: %v", err)
			}
			mustExec(t, store, `UPDATE facts SET payload = ? WHERE id = ?`, encoded, handoff.EventID)
		},
	} {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			root := projectRoot(t)
			resolver := PathResolver{StateHome: t.TempDir()}
			status, err := Initialize(ctx, root, resolver)
			if err != nil {
				t.Fatalf("Initialize() error = %v", err)
			}
			handoff, err := CreateArtifactEntity(ctx, root, resolver, ArtifactEntityCreateOptions{Kind: "handoff", Title: "Strict handoff", Body: "# Body\n"})
			if err != nil {
				t.Fatalf("CreateArtifactEntity(handoff) error = %v", err)
			}
			store, err := OpenStore(status.DatabasePath)
			if err != nil {
				t.Fatalf("OpenStore() error = %v", err)
			}
			mutate(t, store, status.ProjectID, handoff)
			if err := store.Close(); err != nil {
				t.Fatalf("Close() error = %v", err)
			}
			backup, err := Backup(ctx, root, resolver)
			if err != nil {
				t.Fatalf("Backup() error = %v", err)
			}
			if _, err := ExportVNextRehearsalArchive(ctx, VNextRehearsalExportOptions{Backup: backup, ProjectID: status.ProjectID, Root: root, Resolver: resolver}); err == nil {
				t.Fatal("ExportVNextRehearsalArchive() error = nil")
			}
		})
	}
}

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
		"handoff_mapping", "spec-migration-sentinel", "task-migration-sentinel", "other-project-only",
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
		vnextRehearsalFoldV1{Payloads: map[string]JournalFactPayload{}, Roots: map[string]vnextRehearsalFactRootV1{}},
		map[string]vnextRehearsalProjectionV1{},
		vnextRehearsalHandoffFoldV1{Facts: map[string]CoreEventPayload{}, Roots: map[string]vnextRehearsalFactRootV1{}},
		map[string]vnextRehearsalHandoffProjectionV1{}, 0, 0, 0,
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
