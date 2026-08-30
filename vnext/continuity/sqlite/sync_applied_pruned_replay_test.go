package sqlite

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"testing"
)

func TestStageSyncPageComparesAppliedPrunedReplayReceipt(t *testing.T) {
	t.Parallel()

	_, store, projectID, authority, frames := terminalCandidateMixedFixtureV1(
		t, "sync-applied-pruned-replay", 2,
	)
	candidate, err := store.StageVerifiedTerminalCandidateChunk(
		context.Background(), projectID, currentSyncAuthorityBindingForTest(t, store, projectID), frames, 1_000, 100,
	)
	if err != nil {
		t.Fatalf("StageVerifiedTerminalCandidateChunk() error = %v", err)
	}
	if _, err := store.PromoteTerminalCandidate(
		context.Background(), projectID, terminalCandidateCheckpointV1(candidate),
	); err != nil {
		t.Fatalf("PromoteTerminalCandidate() error = %v", err)
	}

	wantDigest := sha256.Sum256(frames[1].Inbox.PrunedArrival)
	var retainedDigest []byte
	if err := store.db.QueryRow(`
SELECT pruned_arrival_digest
FROM continuity_sync_tombstones
WHERE project_id = ? AND arrival_sequence = 2`, string(projectID)).Scan(&retainedDigest); err != nil {
		t.Fatalf("read promoted pruned arrival digest: %v", err)
	}
	if !bytes.Equal(retainedDigest, wantDigest[:]) {
		t.Fatalf("promoted pruned arrival digest = %x, want %x", retainedDigest, wantDigest)
	}
	reference := frames[1].Pruned.Reference
	if _, err := store.db.Exec(`
INSERT INTO continuity_sync_receipts(
  project_id, arrival_sequence, fact_id, environment_id, environment_sequence,
  previous_envelope_digest, envelope_digest, certificate_id, key_generation, nonce
) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		string(projectID), reference.ArrivalSequence, string(reference.FactID), string(reference.EnvironmentID), reference.EnvironmentSequence,
		reference.PreviousEnvelopeDigest[:], reference.EnvelopeDigest[:], reference.CertificateID[:], reference.KeyGeneration, reference.Nonce[:],
	); err != nil {
		t.Fatalf("seed shadowed sealed receipt: %v", err)
	}

	progress, err := store.StageSyncPage(
		context.Background(), projectID, authority.ChannelID, 1, 2,
		[]OpaqueSyncFrame{frames[1].Inbox},
	)
	if err != nil || progress.DownloadedCursor != 2 || progress.AppliedCursor != 2 {
		t.Fatalf("StageSyncPage(exact applied prune) = (%#v, %v), want exact idempotent success", progress, err)
	}

	altered := cloneOpaqueSyncFrameV1(frames[1].Inbox)
	altered.PrunedArrival[0] ^= 0xff
	_, err = store.StageSyncPage(
		context.Background(), projectID, authority.ChannelID, 1, 2,
		[]OpaqueSyncFrame{altered},
	)
	var problem *SyncError
	if !errors.As(err, &problem) || problem.Code != SyncErrorConflict || problem.Field != "frame_bytes" {
		t.Fatalf("StageSyncPage(altered applied prune) error = %v, want immutable byte conflict", err)
	}

	if _, err := store.db.Exec(`
UPDATE continuity_sync_tombstones
SET pruned_arrival_digest = NULL
WHERE project_id = ? AND arrival_sequence = 2`, string(projectID)); err != nil {
		t.Fatalf("clear promoted digest for legacy fixture: %v", err)
	}
	_, err = store.StageSyncPage(
		context.Background(), projectID, authority.ChannelID, 1, 2,
		[]OpaqueSyncFrame{frames[1].Inbox},
	)
	if !errors.As(err, &problem) || problem.Code != SyncErrorConflict || problem.Field != "applied_pruned_unverifiable" {
		t.Fatalf("StageSyncPage(unverifiable applied prune) error = %v, want fail-closed conflict", err)
	}
}

func TestPromoteTerminalCandidateFillsOrRejectsPrunedArrivalReceipt(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name         string
		suffix       string
		seedDigest   *[32]byte
		wantConflict bool
	}{
		{name: "fill legacy tombstone", suffix: "fill"},
		{name: "reject retained mismatch", suffix: "mismatch", seedDigest: func() *[32]byte {
			digest := sha256.Sum256([]byte("different-pruned-arrival"))
			return &digest
		}(), wantConflict: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			store, projectID, _, frame := terminalCandidatePrunedWithLocalRootV1(
				t, "sync-applied-pruned-receipt-"+test.suffix, false,
			)
			pruned := frame.Pruned
			tx, err := store.db.Begin()
			if err != nil {
				t.Fatalf("begin tombstone seed: %v", err)
			}
			if err := insertPruneTombstoneV1(
				context.Background(), tx, projectID, pruned.Reference,
				pruned.PruneCertificateID, test.seedDigest,
			); err != nil {
				tx.Rollback()
				t.Fatalf("seed tombstone: %v", err)
			}
			fact := storedFactV1{
				projectID: projectID, environmentID: pruned.Reference.EnvironmentID,
				environmentSequence: pruned.Reference.EnvironmentSequence, clock: pruned.HLC,
			}
			if err := advanceEnvironmentHeadV1(context.Background(), tx, fact); err != nil {
				tx.Rollback()
				t.Fatalf("advance tombstone environment head: %v", err)
			}
			if err := recordSealedEnvironmentHeadV1(context.Background(), tx, fact, sealedEnvelopeMetadataV1{
				previousDigest: pruned.Reference.PreviousEnvelopeDigest,
				digest:         pruned.Reference.EnvelopeDigest,
				certificateID:  pruned.Reference.CertificateID,
				keyGeneration:  pruned.Reference.KeyGeneration,
				nonce:          pruned.Reference.Nonce,
			}); err != nil {
				tx.Rollback()
				t.Fatalf("seal tombstone environment head: %v", err)
			}
			if err := tx.Commit(); err != nil {
				t.Fatalf("commit tombstone seed: %v", err)
			}

			candidate, err := store.StageVerifiedTerminalCandidateChunk(
				context.Background(), projectID, currentSyncAuthorityBindingForTest(t, store, projectID), []VerifiedTerminalCandidateFrame{frame}, 1_000, 100,
			)
			if err != nil {
				t.Fatalf("StageVerifiedTerminalCandidateChunk() error = %v", err)
			}
			_, err = store.PromoteTerminalCandidate(
				context.Background(), projectID, terminalCandidateCheckpointV1(candidate),
			)
			if test.wantConflict {
				var problem *SyncError
				if !errors.As(err, &problem) || problem.Code != SyncErrorConflict || problem.Field != "tombstone" {
					t.Fatalf("PromoteTerminalCandidate(mismatched receipt) error = %v, want tombstone conflict", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("PromoteTerminalCandidate(fill receipt) error = %v", err)
			}
			wantDigest := sha256.Sum256(frame.Inbox.PrunedArrival)
			var retainedDigest []byte
			if err := store.db.QueryRow(`
SELECT pruned_arrival_digest
FROM continuity_sync_tombstones
WHERE project_id = ? AND arrival_sequence = ?`, string(projectID), pruned.Reference.ArrivalSequence).Scan(&retainedDigest); err != nil {
				t.Fatalf("read filled pruned arrival digest: %v", err)
			}
			if !bytes.Equal(retainedDigest, wantDigest[:]) {
				t.Fatalf("filled pruned arrival digest = %x, want %x", retainedDigest, wantDigest)
			}
		})
	}
}

func TestStageSyncPageMakesAppliedPruneTombstoneCanonicalOverReceipt(t *testing.T) {
	t.Parallel()

	fixture := newPruneFixtureV1(t)
	if err := fixture.store.ApplyVerifiedPrune(context.Background(), fixture.projectID, fixture.plan); err != nil {
		t.Fatalf("ApplyVerifiedPrune() error = %v", err)
	}
	reference := fixture.message
	frame := OpaqueSyncFrame{
		ArrivalSequence: reference.ArrivalSequence,
		EnvelopeDigest:  reference.EnvelopeDigest,
		PrunedArrival:   []byte("pruned:apply-verified-receipt"),
	}
	stage := func(candidate OpaqueSyncFrame) (SyncProgress, error) {
		return fixture.store.StageSyncPage(
			context.Background(), fixture.projectID, fixture.plan.ChannelID,
			reference.ArrivalSequence-1, int64(len(fixture.frames)), []OpaqueSyncFrame{candidate},
		)
	}

	_, err := stage(frame)
	var problem *SyncError
	if !errors.As(err, &problem) || problem.Code != SyncErrorConflict || problem.Field != "applied_pruned_unverifiable" {
		t.Fatalf("StageSyncPage(unreceipted applied prune) error = %v, want fail-closed conflict", err)
	}

	digest := sha256.Sum256(frame.PrunedArrival)
	if _, err := fixture.store.db.Exec(`
UPDATE continuity_sync_tombstones
SET pruned_arrival_digest = ?
WHERE project_id = ? AND arrival_sequence = ?`, digest[:], string(fixture.projectID), reference.ArrivalSequence); err != nil {
		t.Fatalf("fill applied prune receipt: %v", err)
	}
	progress, err := stage(frame)
	if err != nil || progress.AppliedCursor != int64(len(fixture.frames)) || progress.DownloadedCursor != int64(len(fixture.frames)) {
		t.Fatalf("StageSyncPage(exact applied prune) = (%#v, %v), want exact idempotent success", progress, err)
	}

	altered := cloneOpaqueSyncFrameV1(frame)
	altered.PrunedArrival[0] ^= 0xff
	_, err = stage(altered)
	if !errors.As(err, &problem) || problem.Code != SyncErrorConflict || problem.Field != "frame_bytes" {
		t.Fatalf("StageSyncPage(altered applied prune) error = %v, want immutable byte conflict", err)
	}

	conflictingReceiptDigest := sha256.Sum256([]byte("conflicting-shadowed-receipt"))
	if _, err := fixture.store.db.Exec(`
UPDATE continuity_sync_receipts
SET envelope_digest = ?
WHERE project_id = ? AND arrival_sequence = ?`, conflictingReceiptDigest[:], string(fixture.projectID), reference.ArrivalSequence); err != nil {
		t.Fatalf("corrupt shadowed receipt: %v", err)
	}
	_, err = stage(frame)
	if !errors.As(err, &problem) || problem.Code != SyncErrorStore || problem.Field != "tombstone" {
		t.Fatalf("StageSyncPage(conflicting shadowed receipt) error = %v, want store corruption", err)
	}
}
