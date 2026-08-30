package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
)

func TestSyncAuthorityRecoveryAttemptIDGeneratorReturnsNonzeroUniqueIDs(t *testing.T) {
	t.Parallel()

	seen := make(map[[32]byte]struct{}, 64)
	for range 64 {
		attemptID, err := generateSyncAuthorityRecoveryAttemptIDV1()
		if err != nil {
			t.Fatalf("generate attempt ID: %v", err)
		}
		if attemptID == ([32]byte{}) {
			t.Fatal("generated zero attempt ID")
		}
		if _, duplicate := seen[attemptID]; duplicate {
			t.Fatal("generated duplicate attempt ID")
		}
		seen[attemptID] = struct{}{}
	}
}

func TestSyncAuthorityRecoveryTerminalReceiptDigestKnownAnswersAndFieldCoverage(t *testing.T) {
	t.Parallel()

	fixtures := []struct {
		name    string
		receipt SyncAuthorityRecoveryTerminalReceipt
		want    string
	}{
		{name: "aborted generation one", receipt: testSyncAuthorityRecoveryTerminalReceiptV1(0x10, SyncAuthorityRecoveryAborted), want: "bc2706c8b8c9d3f93aaa1d45cf528152b0235d96a0fe2cc05c507c629afa2e85"},
		{name: "aborted with predecessor", receipt: testSyncAuthorityRecoveryTerminalReceiptWithPredecessorV1(0x20), want: "a8aabe9631634b9d32d1b7708722a4eb1a0c8e247c40cd64f99190d58301dee7"},
		{name: "promoted", receipt: testSyncAuthorityRecoveryTerminalReceiptV1(0x30, SyncAuthorityRecoveryPromoted), want: "0391705e743d9880b301263e75ac9ff0d8f893363ee2cae8a9784def02a0ce06"},
	}
	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			got := fmt.Sprintf("%x", syncAuthorityRecoveryTerminalReceiptDigestV1(fixture.receipt))
			if got != fixture.want {
				t.Fatalf("terminal receipt digest = %s, want %s", got, fixture.want)
			}
		})
	}

	baseline := testSyncAuthorityRecoveryTerminalReceiptV1(0x60, SyncAuthorityRecoveryAborted)
	want := syncAuthorityRecoveryTerminalReceiptDigestV1(baseline)
	mutations := []struct {
		name   string
		mutate func(*SyncAuthorityRecoveryTerminalReceipt)
	}{
		{name: "outcome", mutate: func(receipt *SyncAuthorityRecoveryTerminalReceipt) { receipt.Outcome = SyncAuthorityRecoveryPromoted }},
		{name: "project-id", mutate: func(receipt *SyncAuthorityRecoveryTerminalReceipt) { receipt.Transition.ProjectID = "project-other" }},
		{name: "attempt-id", mutate: func(receipt *SyncAuthorityRecoveryTerminalReceipt) { receipt.Transition.AttemptID[0] ^= 0xff }},
		{name: "predecessor-candidate-id", mutate: func(receipt *SyncAuthorityRecoveryTerminalReceipt) {
			receipt.Transition.PredecessorCandidateID[0] ^= 0xff
		}},
		{name: "successor-candidate-id", mutate: func(receipt *SyncAuthorityRecoveryTerminalReceipt) {
			receipt.Transition.SuccessorCandidateID[0] ^= 0xff
		}},
		{name: "writer-environment-id", mutate: func(receipt *SyncAuthorityRecoveryTerminalReceipt) {
			receipt.Transition.WriterEnvironmentID = "environment-other"
		}},
		{name: "writer-certificate-id", mutate: func(receipt *SyncAuthorityRecoveryTerminalReceipt) { receipt.Transition.WriterCertificateID[0] ^= 0xff }},
		{name: "target-membership-generation", mutate: func(receipt *SyncAuthorityRecoveryTerminalReceipt) { receipt.Transition.TargetMembershipGeneration++ }},
		{name: "checkpoint-candidate-id", mutate: func(receipt *SyncAuthorityRecoveryTerminalReceipt) {
			receipt.SuccessorCheckpoint.CandidateID[0] ^= 0xff
		}},
		{name: "page-count", mutate: func(receipt *SyncAuthorityRecoveryTerminalReceipt) { receipt.SuccessorCheckpoint.PageCount++ }},
		{name: "environment-count", mutate: func(receipt *SyncAuthorityRecoveryTerminalReceipt) { receipt.SuccessorCheckpoint.EnvironmentCount++ }},
		{name: "through-environment-id", mutate: func(receipt *SyncAuthorityRecoveryTerminalReceipt) {
			receipt.SuccessorCheckpoint.ThroughEnvironmentID = "environment-other"
		}},
		{name: "rolling-environment-digest", mutate: func(receipt *SyncAuthorityRecoveryTerminalReceipt) {
			receipt.SuccessorCheckpoint.RollingEnvironmentDigest[0] ^= 0xff
		}},
		{name: "ready", mutate: func(receipt *SyncAuthorityRecoveryTerminalReceipt) { receipt.SuccessorCheckpoint.Ready = true }},
		{name: "authority-digest", mutate: func(receipt *SyncAuthorityRecoveryTerminalReceipt) {
			receipt.SuccessorCheckpoint.AuthorityDigest[0] ^= 0xff
		}},
	}
	for _, mutation := range mutations {
		mutation := mutation
		t.Run("field-"+mutation.name, func(t *testing.T) {
			changed := baseline
			mutation.mutate(&changed)
			if got := syncAuthorityRecoveryTerminalReceiptDigestV1(changed); got == want {
				t.Fatal("terminal receipt digest did not cover changed field")
			}
		})
	}
}

func TestSyncAuthorityRecoveryTerminalReceiptRoundTripsAndComparesExactInput(t *testing.T) {
	t.Parallel()

	store, err := Open(filepath.Join(testTempDir(t), "state"), "environment-a")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	tests := []struct {
		name    string
		receipt SyncAuthorityRecoveryTerminalReceipt
	}{
		{name: "aborted", receipt: testSyncAuthorityRecoveryTerminalReceiptV1(0x10, SyncAuthorityRecoveryAborted)},
		{name: "aborted with predecessor", receipt: testSyncAuthorityRecoveryTerminalReceiptWithPredecessorV1(0x20)},
		{name: "promoted", receipt: testSyncAuthorityRecoveryTerminalReceiptV1(0x30, SyncAuthorityRecoveryPromoted)},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			tx, err := store.db.BeginTx(context.Background(), &sql.TxOptions{})
			if err != nil {
				t.Fatalf("begin receipt transaction: %v", err)
			}
			defer tx.Rollback()
			if err := insertSyncAuthorityRecoveryTerminalReceiptV1(context.Background(), tx, test.receipt); err != nil {
				t.Fatalf("insert receipt: %v", err)
			}
			if err := insertSyncAuthorityRecoveryTerminalReceiptV1(context.Background(), tx, test.receipt); err != nil {
				t.Fatalf("exact receipt replay: %v", err)
			}
			got, found, err := readAndAuditSyncAuthorityRecoveryTerminalReceiptV1(
				context.Background(), tx, test.receipt.Transition.ProjectID, test.receipt.Transition.AttemptID,
			)
			if err != nil || !found {
				t.Fatalf("read receipt = (%#v, %v, %v)", got, found, err)
			}
			if !syncAuthorityRecoveryTerminalReceiptMatchesV1(got, test.receipt) ||
				!syncAuthorityRecoveryTerminalReceiptMatchesInputV1(
					got, test.receipt.Outcome, test.receipt.Transition, test.receipt.SuccessorCheckpoint,
				) {
				t.Fatalf("receipt round trip = %#v, want %#v", got, test.receipt)
			}
			changed := test.receipt
			changed.Transition.WriterCertificateID[0]++
			if err := insertSyncAuthorityRecoveryTerminalReceiptV1(context.Background(), tx, changed); err == nil {
				t.Fatal("changed receipt replay error = nil")
			} else {
				var problem *SyncError
				if !errors.As(err, &problem) || problem.Code != SyncErrorConflict {
					t.Fatalf("changed receipt replay error = %#v, want conflict", err)
				}
			}
			if err := tx.Commit(); err != nil {
				t.Fatalf("commit receipt: %v", err)
			}
		})
	}
}

func TestSyncAuthorityRecoveryTerminalReceiptSchemaHasNoForeignKeysAndRejectsInvalidRows(t *testing.T) {
	t.Parallel()

	store, err := Open(filepath.Join(testTempDir(t), "state"), "environment-a")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()
	foreignKeys, err := store.db.Query(`PRAGMA foreign_key_list(continuity_sync_authority_recovery_terminal_receipts)`)
	if err != nil {
		t.Fatalf("inspect receipt foreign keys: %v", err)
	}
	defer foreignKeys.Close()
	if foreignKeys.Next() {
		t.Fatal("terminal receipt table has a foreign key")
	}

	receipt := testSyncAuthorityRecoveryTerminalReceiptV1(0x50, SyncAuthorityRecoveryAborted)
	if _, err := store.db.Exec(`
INSERT INTO continuity_sync_authority_recovery_terminal_receipts(
  project_id, attempt_id, outcome, predecessor_candidate_id,
  successor_candidate_id, writer_environment_id, writer_certificate_id,
  target_membership_generation, successor_page_count,
  successor_environment_count, successor_through_environment_id,
  successor_rolling_environment_digest, successor_ready,
  successor_authority_digest, receipt_digest_version, receipt_digest
) VALUES(?, zeroblob(32), 'aborted', NULL, ?, ?, ?, 1, 1, 1, ?, ?, 0, NULL, 1, ?)`,
		string(receipt.Transition.ProjectID), receipt.Transition.SuccessorCandidateID[:],
		string(receipt.Transition.WriterEnvironmentID), receipt.Transition.WriterCertificateID[:],
		receipt.SuccessorCheckpoint.ThroughEnvironmentID, receipt.SuccessorCheckpoint.RollingEnvironmentDigest[:],
		schemaDigestBytes(0x61),
	); err == nil {
		t.Fatal("zero attempt receipt insert error = nil")
	}
	if _, err := store.db.Exec(`
INSERT INTO continuity_sync_authority_recovery_terminal_receipts(
  project_id, attempt_id, outcome, predecessor_candidate_id,
  successor_candidate_id, writer_environment_id, writer_certificate_id,
  target_membership_generation, successor_page_count,
  successor_environment_count, successor_through_environment_id,
  successor_rolling_environment_digest, successor_ready,
  successor_authority_digest, receipt_digest_version, receipt_digest
) VALUES(?, ?, 'promoted', NULL, ?, ?, ?, 1, 1, 1, ?, ?, 0, NULL, 1, ?)`,
		string(receipt.Transition.ProjectID), receipt.Transition.AttemptID[:],
		receipt.Transition.SuccessorCandidateID[:], string(receipt.Transition.WriterEnvironmentID),
		receipt.Transition.WriterCertificateID[:], receipt.SuccessorCheckpoint.ThroughEnvironmentID,
		receipt.SuccessorCheckpoint.RollingEnvironmentDigest[:], schemaDigestBytes(0x62),
	); err == nil {
		t.Fatal("promoted staging receipt insert error = nil")
	}
}

func TestSyncAuthorityRecoveryTerminalReceiptReaderRejectsRawCorruption(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate string
	}{
		{name: "digest", mutate: `UPDATE continuity_sync_authority_recovery_terminal_receipts SET receipt_digest = ?`},
		{name: "outcome", mutate: `UPDATE continuity_sync_authority_recovery_terminal_receipts SET outcome = 'unknown'`},
		{name: "checkpoint count", mutate: `UPDATE continuity_sync_authority_recovery_terminal_receipts SET successor_environment_count = 5`},
	}
	for index, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			store, err := Open(filepath.Join(testTempDir(t), "state"), "environment-a")
			if err != nil {
				t.Fatalf("Open() error = %v", err)
			}
			defer store.Close()
			receipt := testSyncAuthorityRecoveryTerminalReceiptV1(byte(0x70+index*8), SyncAuthorityRecoveryAborted)
			tx, err := store.db.Begin()
			if err != nil {
				t.Fatalf("begin receipt insert: %v", err)
			}
			if err := insertSyncAuthorityRecoveryTerminalReceiptV1(context.Background(), tx, receipt); err != nil {
				tx.Rollback()
				t.Fatalf("insert receipt: %v", err)
			}
			if err := tx.Commit(); err != nil {
				t.Fatalf("commit receipt: %v", err)
			}
			if _, err := store.db.Exec(`PRAGMA ignore_check_constraints = ON`); err != nil {
				t.Fatalf("disable check constraints: %v", err)
			}
			args := []any{}
			if test.name == "digest" {
				args = append(args, schemaDigestBytes(0xf0))
			}
			if _, err := store.db.Exec(test.mutate, args...); err != nil {
				t.Fatalf("corrupt receipt: %v", err)
			}
			if _, err := store.db.Exec(`PRAGMA ignore_check_constraints = OFF`); err != nil {
				t.Fatalf("restore check constraints: %v", err)
			}
			readTx, err := store.db.BeginTx(context.Background(), &sql.TxOptions{ReadOnly: true})
			if err != nil {
				t.Fatalf("begin receipt read: %v", err)
			}
			defer readTx.Rollback()
			_, found, err := readAndAuditSyncAuthorityRecoveryTerminalReceiptV1(
				context.Background(), readTx, receipt.Transition.ProjectID, receipt.Transition.AttemptID,
			)
			if err == nil || found {
				t.Fatalf("read corrupt receipt = (_, %v, %v), want store error", found, err)
			}
			var problem *SyncError
			if !errors.As(err, &problem) || problem.Code != SyncErrorStore || problem.Field != "sync_authority_recovery_terminal_receipt" {
				t.Fatalf("corrupt receipt error = %#v", err)
			}
		})
	}
}

func testSyncAuthorityRecoveryTerminalReceiptV1(
	seed byte,
	outcome SyncAuthorityRecoveryTerminalOutcome,
) SyncAuthorityRecoveryTerminalReceipt {
	transition := SyncAuthorityRecoveryTransition{
		ProjectID:                  "project-recovery-receipt",
		AttemptID:                  recoveryReceiptDigestV1(seed),
		SuccessorCandidateID:       recoveryReceiptDigestV1(seed + 1),
		WriterEnvironmentID:        "environment-writer",
		WriterCertificateID:        recoveryReceiptDigestV1(seed + 2),
		TargetMembershipGeneration: 1,
	}
	checkpoint := SyncAuthorityCandidateCheckpoint{
		CandidateID:              transition.SuccessorCandidateID,
		PageCount:                1,
		EnvironmentCount:         2,
		ThroughEnvironmentID:     "environment-through",
		RollingEnvironmentDigest: recoveryReceiptDigestV1(seed + 3),
	}
	if outcome == SyncAuthorityRecoveryPromoted {
		checkpoint.Ready = true
		checkpoint.AuthorityDigest = recoveryReceiptDigestV1(seed + 4)
	}
	receipt, err := newSyncAuthorityRecoveryTerminalReceiptV1(outcome, transition, checkpoint)
	if err != nil {
		panic(err)
	}
	return receipt
}

func testSyncAuthorityRecoveryTerminalReceiptWithPredecessorV1(seed byte) SyncAuthorityRecoveryTerminalReceipt {
	receipt := testSyncAuthorityRecoveryTerminalReceiptV1(seed, SyncAuthorityRecoveryAborted)
	receipt.Transition.PredecessorCandidateID = recoveryReceiptDigestV1(seed + 5)
	receipt.Transition.TargetMembershipGeneration = 2
	validated, err := newSyncAuthorityRecoveryTerminalReceiptV1(
		receipt.Outcome, receipt.Transition, receipt.SuccessorCheckpoint,
	)
	if err != nil {
		panic(err)
	}
	return validated
}

func recoveryReceiptDigestV1(seed byte) [32]byte {
	var digest [32]byte
	for index := range digest {
		digest[index] = seed + byte(index)
	}
	return digest
}
