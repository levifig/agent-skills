package state

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"
)

func TestInspectJournalProvenanceIntegrityReportsProjectScopedOrphans(t *testing.T) {
	ctx := context.Background()
	resolver := PathResolver{StateHome: t.TempDir()}
	rootA := projectRoot(t)
	rootB := projectRoot(t)
	if _, err := Initialize(ctx, rootA, resolver); err != nil {
		t.Fatalf("Initialize(A) error = %v", err)
	}
	if _, err := Initialize(ctx, rootB, resolver); err != nil {
		t.Fatalf("Initialize(B) error = %v", err)
	}
	store := openTestStore(t, rootA, resolver.StateHome)
	defer store.Close()
	journalA, err := store.LogJournal(ctx, rootA, JournalLogOptions{Entry: "decision(integrity): A"})
	if err != nil {
		t.Fatalf("LogJournal(A) error = %v", err)
	}
	journalB, err := store.LogJournal(ctx, rootB, JournalLogOptions{Entry: "decision(integrity): B"})
	if err != nil {
		t.Fatalf("LogJournal(B) error = %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `INSERT INTO sparks (id, project_id, status, text, created_at, updated_at) VALUES ('integrity-spark-a', ?, 'open', 'A', ?, ?)`, journalA.ProjectID, "2026-01-01T00:00:00Z", "2026-01-01T00:00:00Z"); err != nil {
		t.Fatalf("insert A spark: %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `INSERT INTO sparks (id, project_id, status, text, created_at, updated_at) VALUES ('integrity-spark-b', ?, 'open', 'B', ?, ?)`, journalB.ProjectID, "2026-01-01T00:00:00Z", "2026-01-01T00:00:00Z"); err != nil {
		t.Fatalf("insert B spark: %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `INSERT INTO journal_origins (journal_entry_id, project_id, envelope_version, capture_mechanism, created_at) VALUES (?, ?, 1, 'manual', ?)`, journalB.ID, journalA.ProjectID, "2026-01-01T00:00:00Z"); err != nil {
		t.Fatalf("insert mismatched origin: %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `INSERT INTO journal_deferrals (project_id, operation_key, journal_entry_id, spark_id, stored_digest, created_at) VALUES (?, 'integrity-op', ?, 'integrity-spark-b', ?, ?)`, journalA.ProjectID, journalB.ID, strings.Repeat("a", 64), "2026-01-01T00:00:00Z"); err != nil {
		t.Fatalf("insert mismatched deferral: %v", err)
	}
	integrity, err := inspectJournalProvenanceIntegrity(ctx, store)
	if err != nil {
		t.Fatalf("inspectJournalProvenanceIntegrity() error = %v", err)
	}
	if integrity.Ready || integrity.OriginMissingJournal == 0 || integrity.OriginProjectMismatches == 0 || integrity.DeferralMissingDecision == 0 || integrity.DeferralMissingSpark == 0 || integrity.DeferralProjectMismatches == 0 {
		t.Fatalf("integrity = %#v, want project-scoped orphan findings", integrity)
	}
}

func TestInspectJournalProvenanceIntegrityUsesOneReadSnapshot(t *testing.T) {
	ctx := context.Background()
	stateHome := t.TempDir()
	resolver := PathResolver{StateHome: stateHome}
	rootA := projectRoot(t)
	rootB := projectRoot(t)
	if _, err := Initialize(ctx, rootA, resolver); err != nil {
		t.Fatalf("Initialize(A) error = %v", err)
	}
	if _, err := Initialize(ctx, rootB, resolver); err != nil {
		t.Fatalf("Initialize(B) error = %v", err)
	}
	store := openTestStore(t, rootA, stateHome)
	defer store.Close()
	writer := openTestStore(t, rootA, stateHome)
	defer writer.Close()
	journalA, err := store.LogJournal(ctx, rootA, JournalLogOptions{Entry: "decision(snapshot): A", Origin: &JournalOriginInput{EnvelopeVersion: 1, CaptureMechanism: "manual"}})
	if err != nil {
		t.Fatalf("LogJournal(A) error = %v", err)
	}
	journalB, err := store.LogJournal(ctx, rootB, JournalLogOptions{Entry: "decision(snapshot): B", Origin: &JournalOriginInput{EnvelopeVersion: 1, CaptureMechanism: "manual"}})
	if err != nil {
		t.Fatalf("LogJournal(B) error = %v", err)
	}
	tx, err := store.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		t.Fatalf("BeginTx(read-only) error = %v", err)
	}
	defer tx.Rollback()
	var originRows int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM journal_origins`).Scan(&originRows); err != nil {
		t.Fatalf("establish provenance snapshot: %v", err)
	}
	if originRows != 2 {
		t.Fatalf("initial origin rows = %d, want 2", originRows)
	}
	writerDone := make(chan error, 1)
	go func() {
		_, updateErr := writer.db.ExecContext(ctx, `UPDATE journal_origins SET project_id = ? WHERE journal_entry_id = ?`, journalB.ProjectID, journalA.ID)
		writerDone <- updateErr
	}()
	select {
	case err := <-writerDone:
		if err != nil {
			t.Fatalf("concurrent provenance update error = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("concurrent provenance update did not commit while read snapshot was open")
	}
	snapshot, err := inspectJournalProvenanceIntegrity(ctx, tx)
	if err != nil {
		t.Fatalf("inspect snapshot error = %v", err)
	}
	if !snapshot.Ready || snapshot.OriginRows != 2 || snapshot.OriginMissingJournal != 0 || snapshot.OriginProjectMismatches != 0 {
		t.Fatalf("snapshot integrity = %#v, want the pre-update ready state", snapshot)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatalf("rollback snapshot error = %v", err)
	}
	latest, err := InspectJournalProvenanceIntegrity(ctx, store)
	if err != nil {
		t.Fatalf("inspect latest integrity error = %v", err)
	}
	if latest.Ready || latest.OriginMissingJournal != 1 || latest.OriginProjectMismatches != 1 {
		t.Fatalf("latest integrity = %#v, want the post-update mismatch state", latest)
	}
}

func TestJournalProvenanceViolationsGateStatusBackupAndVerification(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	resolver := PathResolver{StateHome: t.TempDir()}
	if _, err := Initialize(ctx, root, resolver); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	logged, err := LogJournal(ctx, root, resolver, JournalLogOptions{Entry: "decision(provenance): canonical"})
	if err != nil {
		t.Fatalf("LogJournal() error = %v", err)
	}
	backup, err := Backup(ctx, root, resolver)
	if err != nil {
		t.Fatalf("Backup(valid) error = %v", err)
	}
	store, err := OpenStore(backup.BackupPath)
	if err != nil {
		t.Fatalf("OpenStore(backup) error = %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `INSERT INTO journal_origins (journal_entry_id, project_id, envelope_version, capture_mechanism, created_at) VALUES ('missing-journal', ?, 1, 'manual', ?)`, logged.ProjectID, "2026-01-01T00:00:00Z"); err != nil {
		store.Close()
		t.Fatalf("insert orphan origin: %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `INSERT INTO journal_deferrals (project_id, operation_key, journal_entry_id, spark_id, stored_digest, created_at) VALUES (?, 'missing-decision', 'missing-decision', 'missing-spark', ?, ?)`, logged.ProjectID, strings.Repeat("b", 64), "2026-01-01T00:00:00Z"); err != nil {
		store.Close()
		t.Fatalf("insert orphan deferral: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close(backup) error = %v", err)
	}
	verified, err := VerifyBackup(ctx, backup.BackupPath)
	if err != nil {
		t.Fatalf("VerifyBackup(corrupt provenance) error = %v", err)
	}
	if verified.RecoveryReady || verified.JournalProvenanceIntegrity.Ready || verified.JournalProvenanceIntegrity.OriginMissingJournal != 1 || verified.JournalProvenanceIntegrity.DeferralMissingDecision != 1 || verified.JournalProvenanceIntegrity.DeferralMissingSpark != 1 {
		t.Fatalf("verification = %#v, want honest non-recovery provenance result", verified)
	}

	live := openTestStore(t, root, resolver.StateHome)
	if _, err := live.db.ExecContext(ctx, `INSERT INTO journal_origins (journal_entry_id, project_id, envelope_version, capture_mechanism, created_at) VALUES ('missing-live-journal', ?, 1, 'manual', ?)`, logged.ProjectID, "2026-01-01T00:00:00Z"); err != nil {
		live.Close()
		t.Fatalf("insert live orphan origin: %v", err)
	}
	live.Close()
	status, err := Inspect(root, resolver)
	if err != nil || status.Mode != ModeInvalid || !containsDiagnosticCode(status.Diagnostics, "journal-provenance-invalid") {
		t.Fatalf("Inspect() status=%#v err=%v, want provenance-invalid mode", status, err)
	}
	if _, err := Backup(ctx, root, resolver); err == nil {
		t.Fatal("Backup(invalid provenance) error = nil, want refusal")
	}
}

func containsDiagnosticCode(diagnostics []Diagnostic, code string) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == code {
			return true
		}
	}
	return false
}
