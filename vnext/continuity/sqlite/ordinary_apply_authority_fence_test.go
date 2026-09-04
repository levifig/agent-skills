package sqlite

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"testing"

	"github.com/levifig/loaf/vnext/continuity"
	"github.com/levifig/loaf/vnext/internal/continuitywire"
)

func TestApplySyncBatchV2RequiresExactRelayFrontierEvenWhenEmpty(t *testing.T) {
	t.Parallel()

	for _, mode := range []string{"empty", "verified"} {
		for _, test := range []struct {
			name      string
			wantField string
			advance   func(*SyncRelayWatermark)
		}{
			{
				name:      "membership",
				wantField: "membership_generation",
				advance: func(watermark *SyncRelayWatermark) {
					watermark.MembershipGeneration++
				},
			},
			{
				name:      "arrival head",
				wantField: "inventory_arrival_head",
				advance: func(watermark *SyncRelayWatermark) {
					watermark.RelayHead++
				},
			},
		} {
			t.Run(mode+"/"+test.name, func(t *testing.T) {
				store, projectID, binding, opaque, verified := applySyncBatchV2AuthorityFencePrepared(
					t, mode+"-"+syncSlug(test.name), 1,
				)
				frames := []VerifiedSyncFrame(nil)
				wantRelayHead := int64(0)
				if mode == "verified" {
					if _, err := store.StageSyncPageUnderAuthority(
						context.Background(), projectID, binding, 0, binding.InventoryArrivalHead, opaque,
					); err != nil {
						t.Fatalf("StageSyncPageUnderAuthority() error = %v", err)
					}
					frames = verified
					wantRelayHead = binding.InventoryArrivalHead
				}
				advanced := syncRelayWatermarkFromAuthorityBindingV1(projectID, binding)
				test.advance(&advanced)
				if got, err := store.AdvanceSyncRelayWatermark(context.Background(), advanced); err != nil || got != advanced {
					t.Fatalf("AdvanceSyncRelayWatermark() = (%#v, %v), want (%#v, nil)", got, err, advanced)
				}

				_, err := store.ApplySyncBatch(context.Background(), projectID, binding, frames, 1_000, 100)
				assertApplySyncBatchAuthorityFenceProblem(t, err, SyncErrorCursor, test.wantField)
				assertApplySyncBatchAuthorityFenceUnchanged(t, store, projectID, 0, int64(len(frames)), int64(len(frames)), wantRelayHead)
			})
		}
	}
}

func TestApplySyncBatchV2AllowsExactEmptyAtAuthorityCutoff(t *testing.T) {
	t.Parallel()

	store, projectID, binding, _, _ := applySyncBatchV2AuthorityFencePrepared(t, "exact-empty", 1)
	if _, err := store.StageSyncPageUnderAuthority(
		context.Background(), projectID, binding, 0, binding.InventoryArrivalHead, nil,
	); err != nil {
		t.Fatalf("StageSyncPageUnderAuthority(empty exact cutoff) error = %v", err)
	}

	progress, err := store.ApplySyncBatch(context.Background(), projectID, binding, nil, 1_000, 100)
	if err != nil {
		t.Fatalf("ApplySyncBatch(exact empty) error = %v", err)
	}
	if progress.AppliedCursor != 0 || progress.DownloadedCursor != 0 || progress.RelayHead != binding.InventoryArrivalHead {
		t.Fatalf("ApplySyncBatch(exact empty) progress = %#v, want applied/downloaded 0 and relay head %d", progress, binding.InventoryArrivalHead)
	}
	assertApplySyncBatchAuthorityFenceUnchanged(t, store, projectID, 0, 0, 0, binding.InventoryArrivalHead)
}

func TestApplySyncBatchV2RequiresExactLocalRelayFrontierEvenWhenEmpty(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name          string
		stage         bool
		wantInbox     int64
		wantRelayHead int64
	}{
		{name: "below", wantInbox: 0, wantRelayHead: 0},
		{name: "above", stage: true, wantInbox: 1, wantRelayHead: 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			store, projectID, binding, opaque, _ := applySyncBatchV2AuthorityFencePrepared(t, "local-frontier-"+test.name, 1)
			if test.stage {
				if _, err := store.StageSyncPage(
					context.Background(), projectID, binding.ChannelID, 0, test.wantRelayHead, opaque,
				); err != nil {
					t.Fatalf("StageSyncPage() error = %v", err)
				}
			}

			_, err := store.ApplySyncBatch(context.Background(), projectID, binding, nil, 1_000, 100)
			assertApplySyncBatchAuthorityFenceProblem(t, err, SyncErrorCursor, "relay_head")
			assertApplySyncBatchAuthorityFenceUnchanged(t, store, projectID, 0, test.wantInbox, test.wantInbox, test.wantRelayHead)
		})
	}
}

func TestApplySyncBatchV2AllowsDownloadedPrefixBeforeAuthorityCutoff(t *testing.T) {
	t.Parallel()

	store, projectID, binding, opaque, verified := applySyncBatchV2AuthorityFencePrepared(t, "downloaded-prefix", 2)
	if _, err := store.StageSyncPageUnderAuthority(
		context.Background(), projectID, binding, 0, binding.InventoryArrivalHead, opaque[:1],
	); err != nil {
		t.Fatalf("StageSyncPageUnderAuthority() error = %v", err)
	}
	progress, err := store.ApplySyncBatch(context.Background(), projectID, binding, verified[:1], 1_000, 100)
	if err != nil {
		t.Fatalf("ApplySyncBatch(prefix) error = %v", err)
	}
	if progress.AppliedCursor != 1 || progress.DownloadedCursor != 1 || progress.RelayHead != binding.InventoryArrivalHead {
		t.Fatalf("ApplySyncBatch(prefix) progress = %#v, want applied/downloaded 1 and relay head %d", progress, binding.InventoryArrivalHead)
	}

	if _, err := store.StageSyncPageUnderAuthority(
		context.Background(), projectID, binding, 1, binding.InventoryArrivalHead, opaque[1:],
	); err != nil {
		t.Fatalf("StageSyncPageUnderAuthority(suffix) error = %v", err)
	}
	progress, err = store.ApplySyncBatch(context.Background(), projectID, binding, verified[1:], 1_000, 100)
	if err != nil {
		t.Fatalf("ApplySyncBatch(suffix) error = %v", err)
	}
	if progress.AppliedCursor != 2 || progress.DownloadedCursor != 2 || progress.RelayHead != binding.InventoryArrivalHead {
		t.Fatalf("ApplySyncBatch(suffix) progress = %#v, want applied/downloaded/relay head 2", progress)
	}
}

func TestApplySyncBatchPreservesV1PositiveLegacyFrontier(t *testing.T) {
	t.Parallel()

	store := openSyncStore(t, "ordinary-apply-v1-positive-frontier")
	projectID := continuity.ProjectID("project-ordinary-apply-v1-positive-frontier")
	fact := syncProjectFact(t, projectID, "fact-ordinary-apply-v1-root", "environment-a", 1, 100)
	verified := stageSyncFacts(t, store, projectID, 1, []continuitywire.Fact{fact})
	binding := currentSyncAuthorityBindingForTest(t, store, projectID)
	if binding.AuthorityDigestVersion != 1 || binding.InventoryArrivalHead != 0 {
		t.Fatalf("legacy binding = %#v, want digest version 1 and authority head 0", binding)
	}

	progress, err := store.ApplySyncBatch(context.Background(), projectID, binding, verified, 1_000, 100)
	if err != nil {
		t.Fatalf("ApplySyncBatch(v1 positive frontier) error = %v", err)
	}
	if progress.AppliedCursor != 1 || progress.DownloadedCursor != 1 || progress.RelayHead != 1 {
		t.Fatalf("ApplySyncBatch(v1 positive frontier) progress = %#v, want applied/downloaded/relay head 1", progress)
	}
}

func TestApplySyncBatchV2RejectsProgressBeyondAuthorityCutoff(t *testing.T) {
	t.Parallel()

	store, projectID, binding, opaque, verified := applySyncBatchV2AuthorityFencePrepared(t, "progress-beyond", 1)
	if _, err := store.StageSyncPageUnderAuthority(
		context.Background(), projectID, binding, 0, binding.InventoryArrivalHead, opaque,
	); err != nil {
		t.Fatalf("StageSyncPageUnderAuthority() error = %v", err)
	}
	postCutoff := []byte("ordinary apply post-cutoff arrival")
	postCutoffDigest := sha256.Sum256(postCutoff)
	if _, err := store.StageSyncPage(
		context.Background(), projectID, binding.ChannelID, binding.InventoryArrivalHead, binding.InventoryArrivalHead+1,
		[]OpaqueSyncFrame{{
			ArrivalSequence: binding.InventoryArrivalHead + 1,
			EnvelopeDigest:  postCutoffDigest,
			SealedEnvelope:  postCutoff,
		}},
	); err != nil {
		t.Fatalf("StageSyncPage(post-cutoff) error = %v", err)
	}

	_, err := store.ApplySyncBatch(context.Background(), projectID, binding, verified, 1_000, 100)
	assertApplySyncBatchAuthorityFenceProblem(t, err, SyncErrorCursor, "relay_head")
	assertApplySyncBatchAuthorityFenceUnchanged(t, store, projectID, 0, 2, 2, binding.InventoryArrivalHead+1)
}

func TestApplySyncBatchRejectsActiveRecoveryTransition(t *testing.T) {
	t.Parallel()

	fixture := stageCanonicalBoundedRecoverySuccessorV1(t, "ordinary-apply-fence")
	_, terminalFrames, _ := terminalHotPathFramesV2(t, fixture.projectID, testSyncAuthority().Environments[0], 1)
	binding := SyncAuthorityBinding{
		ChannelID:              fixture.predecessor.Snapshot.ChannelID,
		RelayGeneration:        fixture.predecessor.Snapshot.RelayGeneration,
		AdminPublicKey:         fixture.predecessor.Snapshot.AdminPublicKey,
		MembershipGeneration:   fixture.predecessor.Snapshot.MembershipGeneration,
		InventoryArrivalHead:   fixture.predecessor.Snapshot.InventoryArrivalHead,
		AuthorityDigestVersion: fixture.predecessor.Snapshot.BaseAuthorityDigestVersion,
		AuthorityDigest:        fixture.predecessor.Snapshot.BaseAuthorityDigest,
	}

	var beforeInbox, beforeQuarantined int64
	if err := fixture.store.db.QueryRow(`SELECT
  COUNT(*),
  COUNT(*) FILTER (WHERE state = 'quarantined')
FROM continuity_sync_inbox
WHERE project_id = ?`, string(fixture.projectID)).Scan(&beforeInbox, &beforeQuarantined); err != nil {
		t.Fatalf("read recovery inbox before ordinary apply: %v", err)
	}
	var beforeDownloaded, beforeApplied, beforeRelayHead int64
	if err := fixture.store.db.QueryRow(`SELECT downloaded_cursor, applied_cursor, relay_head
FROM continuity_sync_projects
WHERE project_id = ?`, string(fixture.projectID)).Scan(&beforeDownloaded, &beforeApplied, &beforeRelayHead); err != nil {
		t.Fatalf("read progress before recovery fence: %v", err)
	}

	for _, test := range []struct {
		name             string
		frames           []VerifiedSyncFrame
		trustedNowMillis int64
		maxFutureSkew    int64
	}{
		{name: "empty", trustedNowMillis: 1_000, maxFutureSkew: 100},
		{name: "verified", frames: []VerifiedSyncFrame{*terminalFrames[0].Sealed}, trustedNowMillis: 1_000, maxFutureSkew: 100},
		{name: "future", frames: []VerifiedSyncFrame{*terminalFrames[0].Sealed}, trustedNowMillis: 0, maxFutureSkew: 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := fixture.store.ApplySyncBatch(
				context.Background(), fixture.projectID, binding, test.frames, test.trustedNowMillis, test.maxFutureSkew,
			)
			assertApplySyncBatchAuthorityFenceProblem(t, err, SyncErrorConflict, "sync_authority_recovery_transition")
			var afterDownloaded, afterApplied, afterRelayHead int64
			if queryErr := fixture.store.db.QueryRow(`SELECT downloaded_cursor, applied_cursor, relay_head
FROM continuity_sync_projects
WHERE project_id = ?`, string(fixture.projectID)).Scan(&afterDownloaded, &afterApplied, &afterRelayHead); queryErr != nil {
				t.Fatalf("read progress after recovery fence: %v", queryErr)
			}
			if afterDownloaded != beforeDownloaded || afterApplied != beforeApplied || afterRelayHead != beforeRelayHead {
				t.Fatalf(
					"recovery-fenced progress = %d/%d/%d, want unchanged %d/%d/%d",
					afterDownloaded, afterApplied, afterRelayHead,
					beforeDownloaded, beforeApplied, beforeRelayHead,
				)
			}
			var afterInbox, afterQuarantined int64
			if queryErr := fixture.store.db.QueryRow(`SELECT
  COUNT(*),
  COUNT(*) FILTER (WHERE state = 'quarantined')
FROM continuity_sync_inbox
WHERE project_id = ?`, string(fixture.projectID)).Scan(&afterInbox, &afterQuarantined); queryErr != nil {
				t.Fatalf("read recovery inbox after ordinary apply: %v", queryErr)
			}
			if afterInbox != beforeInbox || afterQuarantined != beforeQuarantined {
				t.Fatalf("recovery-fenced inbox = %d/%d, want unchanged %d/%d", afterInbox, afterQuarantined, beforeInbox, beforeQuarantined)
			}
		})
	}
}

func TestApplySyncBatchExactFinalCASRollsBack(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name    string
		trigger string
	}{
		{
			name: "progress",
			trigger: `
CREATE TEMP TRIGGER mutate_ordinary_apply_progress
AFTER INSERT ON continuity_sync_receipts
BEGIN
  UPDATE continuity_sync_projects
  SET relay_head = relay_head + 1
  WHERE project_id = NEW.project_id;
END`,
		},
		{
			name: "authority",
			trigger: `
CREATE TEMP TRIGGER mutate_ordinary_apply_authority
AFTER INSERT ON continuity_sync_receipts
BEGIN
  UPDATE continuity_sync_authorities
  SET inventory_arrival_head = inventory_arrival_head + 1
  WHERE project_id = NEW.project_id;
END`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			store, projectID, binding, opaque, verified := applySyncBatchV2AuthorityFencePrepared(t, "final-cas-"+test.name, 1)
			if _, err := store.StageSyncPageUnderAuthority(
				context.Background(), projectID, binding, 0, binding.InventoryArrivalHead, opaque,
			); err != nil {
				t.Fatalf("StageSyncPageUnderAuthority() error = %v", err)
			}
			if _, err := store.db.Exec(test.trigger); err != nil {
				t.Fatalf("create ordinary apply %s trigger: %v", test.name, err)
			}

			_, err := store.ApplySyncBatch(context.Background(), projectID, binding, verified, 1_000, 100)
			assertApplySyncBatchAuthorityFenceProblem(t, err, SyncErrorConflict, "sync_progress")
			assertApplySyncBatchAuthorityFenceUnchanged(t, store, projectID, 0, 1, 1, binding.InventoryArrivalHead)
			if afterBinding := currentSyncAuthorityBindingForTest(t, store, projectID); afterBinding != binding {
				t.Fatalf("canonical authority after CAS rollback = %#v, want %#v", afterBinding, binding)
			}
		})
	}
}

func TestApplySyncBatchFutureOnlyExactFinalCASRollsBack(t *testing.T) {
	t.Parallel()

	store, projectID, binding, opaque, verified := applySyncBatchV2AuthorityFencePrepared(t, "future-final-cas", 1)
	if _, err := store.StageSyncPageUnderAuthority(
		context.Background(), projectID, binding, 0, binding.InventoryArrivalHead, opaque,
	); err != nil {
		t.Fatalf("StageSyncPageUnderAuthority() error = %v", err)
	}
	if _, err := store.db.Exec(`
CREATE TEMP TRIGGER mutate_ordinary_apply_progress_on_quarantine
AFTER UPDATE OF state ON continuity_sync_inbox
WHEN NEW.state = 'quarantined'
BEGIN
  UPDATE continuity_sync_projects
  SET relay_head = relay_head + 1
  WHERE project_id = NEW.project_id;
END`); err != nil {
		t.Fatalf("create ordinary apply quarantine trigger: %v", err)
	}

	_, err := store.ApplySyncBatch(context.Background(), projectID, binding, verified, 0, 0)
	assertApplySyncBatchAuthorityFenceProblem(t, err, SyncErrorConflict, "sync_progress")
	assertApplySyncBatchAuthorityFenceUnchanged(t, store, projectID, 0, 1, 1, binding.InventoryArrivalHead)
}

func applySyncBatchV2AuthorityFencePrepared(
	t *testing.T,
	suffix string,
	authorityHead int64,
) (*Store, continuity.ProjectID, SyncAuthorityBinding, []OpaqueSyncFrame, []VerifiedSyncFrame) {
	t.Helper()
	store := openSyncStore(t, "ordinary-apply-authority-fence-"+suffix)
	projectID := continuity.ProjectID("project-ordinary-apply-authority-fence-" + suffix)
	environments := syncAuthorityCandidateManyEnvironmentsV2(1)
	snapshot := syncAuthorityCandidateBootstrapSnapshotV2(1)
	snapshot.InventoryArrivalHead = authorityHead
	authority := syncAuthorityFromSnapshotForBindingTest(snapshot, environments)
	digest := seedCanonicalSyncAuthorityForBindingTest(t, store, projectID, authority)
	binding := syncAuthorityBindingForTest(authority, 2, digest)
	watermark := syncRelayWatermarkFromAuthorityBindingV1(projectID, binding)
	if got, err := store.AdvanceSyncRelayWatermark(context.Background(), watermark); err != nil || got != watermark {
		t.Fatalf("AdvanceSyncRelayWatermark() = (%#v, %v), want (%#v, nil)", got, err, watermark)
	}

	environmentID := continuity.EnvironmentID(environments[0].EnvironmentID)
	opaque := make([]OpaqueSyncFrame, 0, authorityHead)
	verified := make([]VerifiedSyncFrame, 0, authorityHead)
	var previousDigest [32]byte
	for sequence := int64(1); sequence <= authorityHead; sequence++ {
		var fact continuitywire.Fact
		if sequence == 1 {
			fact = syncProjectFact(t, projectID, "fact-ordinary-apply-root", environmentID, sequence, 99+sequence)
		} else {
			fact = syncIdeaCreatedFact(
				t,
				projectID,
				continuity.FactID(fmt.Sprintf("fact-ordinary-apply-%d", sequence)),
				continuity.SubjectID(fmt.Sprintf("idea-ordinary-apply-%d", sequence)),
				environmentID,
				sequence,
				99+sequence,
				fmt.Sprintf("Ordinary apply %d", sequence),
			)
		}
		encoded, err := continuitywire.Encode(fact)
		if err != nil {
			t.Fatalf("encode ordinary apply fact %d: %v", sequence, err)
		}
		sealed := append([]byte("sealed:"), encoded...)
		envelopeDigest := sha256.Sum256(sealed)
		opaque = append(opaque, OpaqueSyncFrame{
			ArrivalSequence: sequence,
			EnvelopeDigest:  envelopeDigest,
			SealedEnvelope:  sealed,
		})
		verified = append(verified, VerifiedSyncFrame{
			ArrivalSequence:        sequence,
			PreviousEnvelopeDigest: previousDigest,
			EnvelopeDigest:         envelopeDigest,
			CertificateID:          environments[0].CertificateID,
			KeyGeneration:          1,
			Nonce:                  testNonce(fmt.Sprintf("ordinary-apply-authority-fence:%s:%d", suffix, sequence)),
			Fact:                   fact,
		})
		previousDigest = envelopeDigest
	}
	return store, projectID, binding, opaque, verified
}

func assertApplySyncBatchAuthorityFenceUnchanged(
	t *testing.T,
	store *Store,
	projectID continuity.ProjectID,
	wantApplied,
	wantDownloaded,
	wantInbox,
	wantRelayHead int64,
) {
	t.Helper()
	progress, err := store.CurrentSyncProgress(context.Background(), projectID)
	if err != nil {
		t.Fatalf("CurrentSyncProgress() error = %v", err)
	}
	if progress.AppliedCursor != wantApplied || progress.DownloadedCursor != wantDownloaded || progress.RelayHead != wantRelayHead {
		t.Fatalf("progress = %#v, want applied/downloaded %d/%d and relay head %d", progress, wantApplied, wantDownloaded, wantRelayHead)
	}
	var facts, receipts, inbox, quarantined int64
	if err := store.db.QueryRow(`SELECT
  (SELECT COUNT(*) FROM continuity_facts WHERE project_id = ?),
  (SELECT COUNT(*) FROM continuity_sync_receipts WHERE project_id = ?),
  (SELECT COUNT(*) FROM continuity_sync_inbox WHERE project_id = ?),
  (SELECT COUNT(*) FROM continuity_sync_inbox WHERE project_id = ? AND state = 'quarantined')`,
		string(projectID), string(projectID), string(projectID), string(projectID),
	).Scan(&facts, &receipts, &inbox, &quarantined); err != nil {
		t.Fatalf("read ordinary apply mutation state: %v", err)
	}
	if facts != 0 || receipts != 0 || inbox != wantInbox || quarantined != 0 {
		t.Fatalf("ordinary apply mutated state: facts=%d receipts=%d inbox=%d quarantined=%d, want 0/0/%d/0", facts, receipts, inbox, quarantined, wantInbox)
	}
}

func assertApplySyncBatchAuthorityFenceProblem(
	t *testing.T,
	err error,
	wantCode SyncErrorCode,
	wantField string,
) {
	t.Helper()
	var problem *SyncError
	if !errors.As(err, &problem) {
		t.Fatalf("error = %v, want *SyncError code %q at %q", err, wantCode, wantField)
	}
	if problem.Code != wantCode || problem.Field != wantField {
		t.Fatalf("error = %#v, want code %q at %q", problem, wantCode, wantField)
	}
}
