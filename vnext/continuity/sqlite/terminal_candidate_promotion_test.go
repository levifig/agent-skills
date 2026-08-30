package sqlite

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"strconv"
	"sync"
	"testing"

	"github.com/levifig/loaf/vnext/continuity"
	"github.com/levifig/loaf/vnext/internal/continuitywire"
)

func TestPromoteTerminalCandidateMixedSuccessAndExactRetry(t *testing.T) {
	t.Parallel()

	_, store, projectID, authority, frames := terminalCandidateMixedFixtureV1(t, "terminal-candidate-promotion-mixed", 2)
	candidate, err := store.StageVerifiedTerminalCandidateChunk(context.Background(), projectID, currentSyncAuthorityBindingForTest(t, store, projectID), frames, 1_000, 100)
	if err != nil {
		t.Fatalf("StageVerifiedTerminalCandidateChunk() error = %v", err)
	}
	checkpoint := terminalCandidateCheckpointV1(candidate)
	receipt, err := store.PromoteTerminalCandidate(context.Background(), projectID, checkpoint)
	if err != nil {
		t.Fatalf("PromoteTerminalCandidate() error = %v", err)
	}
	if receipt.ProjectID != candidate.ProjectID || receipt.CandidateID != candidate.CandidateID ||
		receipt.ChannelID != candidate.ChannelID || receipt.RelayGeneration != candidate.RelayGeneration ||
		receipt.MembershipGeneration != candidate.MembershipGeneration || receipt.AuthorityDigest != candidate.AuthorityDigest ||
		receipt.StartArrivalSequence != candidate.StartArrivalSequence || receipt.ThroughArrivalSequence != candidate.ThroughArrivalSequence ||
		receipt.FrameCount != candidate.FrameCount || receipt.RollingCandidateDigest != candidate.RollingCandidateDigest ||
		receipt.PostPromotionCorpusDigest == ([32]byte{}) || receipt.ResultingAppliedCursor != candidate.ThroughArrivalSequence {
		t.Fatalf("promotion receipt = %#v, want candidate bindings and nonzero outcome", receipt)
	}
	replayed, err := store.PromoteTerminalCandidate(context.Background(), projectID, checkpoint)
	if err != nil || replayed != receipt {
		t.Fatalf("PromoteTerminalCandidate(retry) = (%#v, %v), want (%#v, nil)", replayed, err, receipt)
	}
	if _, err := store.db.Exec(`
UPDATE continuity_sync_projects
SET downloaded_cursor = 3, applied_cursor = 3, relay_head = 3
WHERE project_id = ?`, string(projectID)); err != nil {
		t.Fatalf("advance applied cursor after promotion: %v", err)
	}
	advancedAuthority := cloneTerminalCandidateAuthorityV1(authority)
	advancedAuthority.MembershipGeneration++
	advancedAuthority.Environments = append(advancedAuthority.Environments, SyncEnvironmentCertificate{
		EnvironmentID:            "environment-z",
		CertificateID:            testSyncCertificateID("environment-z"),
		CertificateBytes:         []byte("environment-z-certificate"),
		Mode:                     SyncEnvironmentTrusted,
		JoinMembershipGeneration: advancedAuthority.MembershipGeneration,
	})
	if _, err := store.InstallVerifiedSyncAuthority(context.Background(), projectID, advancedAuthority); err != nil {
		t.Fatalf("InstallVerifiedSyncAuthority(after promotion) error = %v", err)
	}
	replayed, err = store.PromoteTerminalCandidate(context.Background(), projectID, checkpoint)
	if err != nil || replayed != receipt {
		t.Fatalf("PromoteTerminalCandidate(retry after progress and authority advance) = (%#v, %v), want (%#v, nil)", replayed, err, receipt)
	}
	if _, found, err := store.CurrentTerminalCandidate(context.Background(), projectID); err != nil || found {
		t.Fatalf("CurrentTerminalCandidate(after promotion) = (_, %v, %v), want (_, false, nil)", found, err)
	}

	var applied, facts, receipts, tombstones, children, inbox, promoted int64
	if err := store.db.QueryRow(`SELECT applied_cursor FROM continuity_sync_projects WHERE project_id = ?`, string(projectID)).Scan(&applied); err != nil {
		t.Fatalf("read applied cursor: %v", err)
	}
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM continuity_facts WHERE project_id = ?`, string(projectID)).Scan(&facts); err != nil {
		t.Fatalf("count facts: %v", err)
	}
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM continuity_sync_receipts WHERE project_id = ?`, string(projectID)).Scan(&receipts); err != nil {
		t.Fatalf("count receipts: %v", err)
	}
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM continuity_sync_tombstones WHERE project_id = ?`, string(projectID)).Scan(&tombstones); err != nil {
		t.Fatalf("count tombstones: %v", err)
	}
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM continuity_sync_terminal_candidate_frames WHERE project_id = ?`, string(projectID)).Scan(&children); err != nil {
		t.Fatalf("count candidate children: %v", err)
	}
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM continuity_sync_inbox WHERE project_id = ?`, string(projectID)).Scan(&inbox); err != nil {
		t.Fatalf("count inbox: %v", err)
	}
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM continuity_sync_terminal_candidates WHERE project_id = ? AND candidate_id = ? AND state = 'promoted'`, string(projectID), candidate.CandidateID[:]).Scan(&promoted); err != nil {
		t.Fatalf("count promoted header: %v", err)
	}
	if applied != 3 || facts != 1 || receipts != 1 || tombstones != 1 || children != 0 || inbox != 0 || promoted != 1 {
		t.Fatalf("promoted state = applied:%d facts:%d receipts:%d tombstones:%d children:%d inbox:%d headers:%d", applied, facts, receipts, tombstones, children, inbox, promoted)
	}
}

func TestPromoteTerminalCandidateExactRetryRejectsCorruptReceiptState(t *testing.T) {
	t.Parallel()

	t.Run("header binding", func(t *testing.T) {
		_, store, projectID, _, frames := terminalCandidateMixedFixtureV1(t, "terminal-candidate-promotion-corrupt-header", 2)
		candidate, err := store.StageVerifiedTerminalCandidateChunk(context.Background(), projectID, currentSyncAuthorityBindingForTest(t, store, projectID), frames, 1_000, 100)
		if err != nil {
			t.Fatalf("stage candidate: %v", err)
		}
		checkpoint := terminalCandidateCheckpointV1(candidate)
		if _, err := store.PromoteTerminalCandidate(context.Background(), projectID, checkpoint); err != nil {
			t.Fatalf("promote candidate: %v", err)
		}
		corruptDigest := sha256.Sum256([]byte("corrupt retained authority digest"))
		if _, err := store.db.Exec(`
UPDATE continuity_sync_terminal_candidates
SET authority_digest = ?
WHERE project_id = ? AND candidate_id = ?`, corruptDigest[:], string(projectID), candidate.CandidateID[:]); err != nil {
			t.Fatalf("corrupt promoted header: %v", err)
		}
		_, err = store.PromoteTerminalCandidate(context.Background(), projectID, checkpoint)
		assertSyncErrorCode(t, err, SyncErrorStore)
	})

	t.Run("applied cursor regression", func(t *testing.T) {
		_, store, projectID, _, frames := terminalCandidateMixedFixtureV1(t, "terminal-candidate-promotion-regressed-applied", 2)
		candidate, err := store.StageVerifiedTerminalCandidateChunk(context.Background(), projectID, currentSyncAuthorityBindingForTest(t, store, projectID), frames, 1_000, 100)
		if err != nil {
			t.Fatalf("stage candidate: %v", err)
		}
		checkpoint := terminalCandidateCheckpointV1(candidate)
		if _, err := store.PromoteTerminalCandidate(context.Background(), projectID, checkpoint); err != nil {
			t.Fatalf("promote candidate: %v", err)
		}
		if _, err := store.db.Exec(`
UPDATE continuity_sync_projects
SET applied_cursor = 1
WHERE project_id = ?`, string(projectID)); err != nil {
			t.Fatalf("regress applied cursor: %v", err)
		}
		_, err = store.PromoteTerminalCandidate(context.Background(), projectID, checkpoint)
		assertSyncErrorCode(t, err, SyncErrorStore)
	})

	t.Run("retained child", func(t *testing.T) {
		_, store, projectID, _, frames := terminalCandidateMixedFixtureV1(t, "terminal-candidate-promotion-retained-child", 2)
		candidate, err := store.StageVerifiedTerminalCandidateChunk(context.Background(), projectID, currentSyncAuthorityBindingForTest(t, store, projectID), frames, 1_000, 100)
		if err != nil {
			t.Fatalf("stage candidate: %v", err)
		}
		if _, err := store.db.Exec(`
CREATE TEMP TABLE saved_terminal_candidate_child AS
SELECT * FROM continuity_sync_terminal_candidate_frames
WHERE project_id = ? AND candidate_id = ? AND arrival_sequence = 2`, string(projectID), candidate.CandidateID[:]); err != nil {
			t.Fatalf("save child fixture: %v", err)
		}
		checkpoint := terminalCandidateCheckpointV1(candidate)
		if _, err := store.PromoteTerminalCandidate(context.Background(), projectID, checkpoint); err != nil {
			t.Fatalf("promote candidate: %v", err)
		}
		if _, err := store.db.Exec(`
INSERT INTO continuity_sync_inbox(
  project_id, arrival_sequence, envelope_digest, frame_kind, frame_bytes, state
) VALUES(?, 2, ?, 'pruned', ?, 'staged')`, string(projectID), frames[1].Inbox.EnvelopeDigest[:], frames[1].Inbox.PrunedArrival); err != nil {
			t.Fatalf("restore inbox fixture: %v", err)
		}
		if _, err := store.db.Exec(`
INSERT INTO continuity_sync_terminal_candidate_frames
SELECT * FROM saved_terminal_candidate_child`); err != nil {
			t.Fatalf("restore child fixture: %v", err)
		}
		_, err = store.PromoteTerminalCandidate(context.Background(), projectID, checkpoint)
		assertSyncErrorCode(t, err, SyncErrorStore)
	})
}

func TestPromoteTerminalCandidateRejectsCheckpointAuthorityAndIncompleteFenceWithoutMutation(t *testing.T) {
	t.Parallel()

	t.Run("wrong checkpoint", func(t *testing.T) {
		_, store, projectID, _, frames := terminalCandidateMixedFixtureV1(t, "terminal-candidate-promotion-wrong-checkpoint", 2)
		candidate, err := store.StageVerifiedTerminalCandidateChunk(context.Background(), projectID, currentSyncAuthorityBindingForTest(t, store, projectID), frames, 1_000, 100)
		if err != nil {
			t.Fatalf("stage candidate: %v", err)
		}
		wrong := terminalCandidateCheckpointV1(candidate)
		wrong.RollingCandidateDigest[0] ^= 0xff
		_, err = store.PromoteTerminalCandidate(context.Background(), projectID, wrong)
		assertSyncErrorCode(t, err, SyncErrorConflict)
		assertTerminalCandidatePromotionUnchangedV1(t, store, projectID, candidate)
	})

	t.Run("authority drift", func(t *testing.T) {
		_, store, projectID, authority, frames := terminalCandidateMixedFixtureV1(t, "terminal-candidate-promotion-authority-drift", 2)
		candidate, err := store.StageVerifiedTerminalCandidateChunk(context.Background(), projectID, currentSyncAuthorityBindingForTest(t, store, projectID), frames, 1_000, 100)
		if err != nil {
			t.Fatalf("stage candidate: %v", err)
		}
		advanced := cloneTerminalCandidateAuthorityV1(authority)
		advanced.MembershipGeneration++
		advanced.Environments = append(advanced.Environments, SyncEnvironmentCertificate{
			EnvironmentID:            "environment-z",
			CertificateID:            testSyncCertificateID("environment-z"),
			CertificateBytes:         []byte("environment-z-certificate"),
			Mode:                     SyncEnvironmentTrusted,
			JoinMembershipGeneration: advanced.MembershipGeneration,
		})
		if _, err := store.InstallVerifiedSyncAuthority(context.Background(), projectID, advanced); err != nil {
			t.Fatalf("advance authority: %v", err)
		}
		_, err = store.PromoteTerminalCandidate(context.Background(), projectID, terminalCandidateCheckpointV1(candidate))
		assertSyncErrorCode(t, err, SyncErrorConflict)
		assertTerminalCandidatePromotionUnchangedV1(t, store, projectID, candidate)
	})

	t.Run("incomplete touched retirement", func(t *testing.T) {
		_, store, projectID, _, frames := terminalCandidateMixedFixtureV1(t, "terminal-candidate-promotion-incomplete-fence", 3)
		candidate, err := store.StageVerifiedTerminalCandidateChunk(context.Background(), projectID, currentSyncAuthorityBindingForTest(t, store, projectID), frames[:1], 1_000, 100)
		if err != nil {
			t.Fatalf("stage candidate prefix: %v", err)
		}
		_, err = store.PromoteTerminalCandidate(context.Background(), projectID, terminalCandidateCheckpointV1(candidate))
		assertSyncErrorCode(t, err, SyncErrorRecoveryRequired)
		assertTerminalCandidatePromotionUnchangedV1(t, store, projectID, candidate)
	})
}

func TestPromoteTerminalCandidateRejectsCorruptCandidateOrInboxAtomically(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(t *testing.T, store *Store, projectID continuity.ProjectID, candidate TerminalCandidate)
	}{
		{
			name: "candidate body",
			mutate: func(t *testing.T, store *Store, projectID continuity.ProjectID, candidate TerminalCandidate) {
				t.Helper()
				var body []byte
				if err := store.db.QueryRow(`
SELECT candidate_bytes
FROM continuity_sync_terminal_candidate_frames
WHERE project_id = ? AND candidate_id = ? AND arrival_sequence = 1`, string(projectID), candidate.CandidateID[:]).Scan(&body); err != nil {
					t.Fatalf("read candidate body: %v", err)
				}
				body[len(body)-1] ^= 0xff
				if _, err := store.db.Exec(`
UPDATE continuity_sync_terminal_candidate_frames
SET candidate_bytes = ?
WHERE project_id = ? AND candidate_id = ? AND arrival_sequence = 1`, body, string(projectID), candidate.CandidateID[:]); err != nil {
					t.Fatalf("corrupt candidate body: %v", err)
				}
			},
		},
		{
			name: "inbox bytes",
			mutate: func(t *testing.T, store *Store, projectID continuity.ProjectID, _ TerminalCandidate) {
				t.Helper()
				var frameBytes []byte
				if err := store.db.QueryRow(`
SELECT frame_bytes
FROM continuity_sync_inbox
WHERE project_id = ? AND arrival_sequence = 1`, string(projectID)).Scan(&frameBytes); err != nil {
					t.Fatalf("read inbox bytes: %v", err)
				}
				frameBytes = append(frameBytes, 0)
				if _, err := store.db.Exec(`
UPDATE continuity_sync_inbox
SET frame_bytes = ?
WHERE project_id = ? AND arrival_sequence = 1`, frameBytes, string(projectID)); err != nil {
					t.Fatalf("corrupt inbox bytes: %v", err)
				}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, store, projectID, _, frames := terminalCandidateMixedFixtureV1(t, "terminal-candidate-promotion-corrupt-"+test.name, 2)
			candidate, err := store.StageVerifiedTerminalCandidateChunk(context.Background(), projectID, currentSyncAuthorityBindingForTest(t, store, projectID), frames, 1_000, 100)
			if err != nil {
				t.Fatalf("stage candidate: %v", err)
			}
			test.mutate(t, store, projectID, candidate)
			_, err = store.PromoteTerminalCandidate(context.Background(), projectID, terminalCandidateCheckpointV1(candidate))
			if err == nil {
				t.Fatal("PromoteTerminalCandidate(corrupt state) error = nil")
			}
			assertTerminalCandidatePromotionUnchangedV1(t, store, projectID, candidate)
		})
	}
}

func TestPromoteTerminalCandidateAllowsActivePartialAndIgnoresUntouchedRetirementAndUnsealedTail(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name              string
		suffix            string
		retireEnvironment bool
	}{
		{name: "active partial", suffix: "active-partial"},
		{name: "untouched retired producer", suffix: "untouched-retired", retireEnvironment: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			store, projectID, _, frame := terminalCandidatePrunedWithLocalRootV1(t, "terminal-candidate-promotion-"+test.suffix, test.retireEnvironment)
			candidate, err := store.StageVerifiedTerminalCandidateChunk(context.Background(), projectID, currentSyncAuthorityBindingForTest(t, store, projectID), []VerifiedTerminalCandidateFrame{frame}, 1_000, 100)
			if err != nil {
				t.Fatalf("stage candidate: %v", err)
			}
			receipt, err := store.PromoteTerminalCandidate(context.Background(), projectID, terminalCandidateCheckpointV1(candidate))
			if err != nil {
				t.Fatalf("PromoteTerminalCandidate() error = %v", err)
			}
			if receipt.ResultingAppliedCursor != 1 {
				t.Fatalf("resulting applied cursor = %d, want 1", receipt.ResultingAppliedCursor)
			}
			var facts, tombstones, localSealedSequence int64
			if err := store.db.QueryRow(`SELECT COUNT(*) FROM continuity_facts WHERE project_id = ?`, string(projectID)).Scan(&facts); err != nil {
				t.Fatalf("count facts: %v", err)
			}
			if err := store.db.QueryRow(`SELECT COUNT(*) FROM continuity_sync_tombstones WHERE project_id = ?`, string(projectID)).Scan(&tombstones); err != nil {
				t.Fatalf("count tombstones: %v", err)
			}
			if err := store.db.QueryRow(`
SELECT sealed_sequence
FROM continuity_sync_environment_heads
WHERE project_id = ? AND environment_id = 'environment-local'`, string(projectID)).Scan(&localSealedSequence); err != nil {
				t.Fatalf("read local environment head: %v", err)
			}
			if facts != 1 || tombstones != 1 || localSealedSequence != 0 {
				t.Fatalf("promoted active prefix state: facts=%d tombstones=%d local sealed=%d", facts, tombstones, localSealedSequence)
			}
		})
	}
}

func TestPromoteTerminalCandidateInvalidResultingFoldRollsBack(t *testing.T) {
	t.Parallel()

	store := openSyncStore(t, "terminal-candidate-promotion-invalid-fold")
	projectID := continuity.ProjectID("project-terminal-candidate-promotion-invalid-fold")
	installTestSyncAuthority(t, store, projectID, testSyncChannelID("channel-a"))
	idea := syncIdeaCreatedFact(t, projectID, "fact-terminal-candidate-orphan-idea", "idea-orphan", "environment-local", 1, 101, "Orphan")
	frames := terminalCandidatePrunedThenSealedFramesV1(t, projectID, idea)
	opaque := []OpaqueSyncFrame{frames[0].Inbox, frames[1].Inbox}
	if _, err := store.StageSyncPage(context.Background(), projectID, testSyncChannelID("channel-a"), 0, 2, opaque); err != nil {
		t.Fatalf("StageSyncPage() error = %v", err)
	}
	_, err := store.CurrentSyncAuthority(context.Background(), projectID)
	if err != nil {
		t.Fatalf("CurrentSyncAuthority() error = %v", err)
	}
	candidate, err := store.StageVerifiedTerminalCandidateChunk(context.Background(), projectID, currentSyncAuthorityBindingForTest(t, store, projectID), frames, 1_000, 100)
	if err != nil {
		t.Fatalf("stage candidate: %v", err)
	}
	_, err = store.PromoteTerminalCandidate(context.Background(), projectID, terminalCandidateCheckpointV1(candidate))
	assertSyncErrorCode(t, err, SyncErrorCandidate)
	assertTerminalCandidatePromotionUnchangedV1(t, store, projectID, candidate)
}

func TestPromoteTerminalCandidateConsumesExactOutboxEchoWithoutTouchingDuplicateFence(t *testing.T) {
	t.Parallel()

	store := openAppendStoreV1(t, filepath.Join(testTempDir(t), "state-terminal-candidate-promotion-outbox-echo"), "environment-b", 100)
	projectID := continuity.ProjectID("project-terminal-candidate-promotion-outbox-echo")
	mustAppendV1(t)(store.RegisterProject(context.Background(), projectID, "fact-local-root", continuity.ProjectRegistrationPayload{Observation: appendObservationV1(), Label: "Loaf"}))
	installTestSyncAuthority(t, store, projectID, testSyncChannelID("channel-a"))
	if _, err := store.StageSyncPage(context.Background(), projectID, testSyncChannelID("channel-a"), 0, 0, nil); err != nil {
		t.Fatalf("StageSyncPage(empty) error = %v", err)
	}
	if _, err := store.ActivateStagedSync(context.Background(), projectID, testSyncChannelID("channel-a")); err != nil {
		t.Fatalf("ActivateStagedSync() error = %v", err)
	}
	root, found, err := store.NextUnsealedLocalFact(context.Background(), projectID)
	if err != nil || !found {
		t.Fatalf("NextUnsealedLocalFact(root) = (_, %v, %v)", found, err)
	}
	rootBytes := []byte("sealed-local-root")
	rootOutbox := SealedOutboxFrame{
		FactID:         root.Fact.FactID,
		EnvelopeDigest: sha256.Sum256(rootBytes),
		CertificateID:  testSyncCertificateID("environment-b"),
		KeyGeneration:  1,
		Nonce:          testNonce("terminal-candidate-promotion-outbox-root"),
		SealedEnvelope: rootBytes,
	}
	if err := store.PersistSealedOutbox(context.Background(), projectID, testSyncChannelID("channel-a"), rootOutbox); err != nil {
		t.Fatalf("PersistSealedOutbox(root) error = %v", err)
	}
	mustAppendV1(t)(store.CreateIdea(context.Background(), projectID, "fact-local-second", "idea-local", continuity.IdeaCreatedPayload{Observation: appendObservationV1(), Content: continuity.IdeaContent{Label: "Local"}}))
	second, found, err := store.NextUnsealedLocalFact(context.Background(), projectID)
	if err != nil || !found {
		t.Fatalf("NextUnsealedLocalFact(second) = (_, %v, %v)", found, err)
	}
	secondBytes := []byte("sealed-local-second")
	secondOutbox := SealedOutboxFrame{
		FactID:                 second.Fact.FactID,
		PreviousEnvelopeDigest: rootOutbox.EnvelopeDigest,
		EnvelopeDigest:         sha256.Sum256(secondBytes),
		CertificateID:          testSyncCertificateID("environment-b"),
		KeyGeneration:          1,
		Nonce:                  testNonce("terminal-candidate-promotion-outbox-second"),
		SealedEnvelope:         secondBytes,
	}
	if err := store.PersistSealedOutbox(context.Background(), projectID, testSyncChannelID("channel-a"), secondOutbox); err != nil {
		t.Fatalf("PersistSealedOutbox(second) error = %v", err)
	}

	remoteFact := syncIdeaCreatedFact(t, projectID, "fact-remote-terminal", "idea-remote", "environment-a", 1, 200, "Remote")
	remoteEncoded, err := continuitywire.Encode(remoteFact)
	if err != nil {
		t.Fatalf("encode remote fact: %v", err)
	}
	remoteBytes := append([]byte("sealed:"), remoteEncoded...)
	remoteDigest := sha256.Sum256(remoteBytes)
	remote := VerifiedSyncFrame{
		ArrivalSequence: 1,
		EnvelopeDigest:  remoteDigest,
		CertificateID:   testSyncCertificateID("environment-a"),
		KeyGeneration:   1,
		Nonce:           testNonce("terminal-candidate-promotion-outbox-remote"),
		Fact:            remoteFact,
	}
	rootEcho := VerifiedSyncFrame{
		ArrivalSequence:        2,
		PreviousEnvelopeDigest: rootOutbox.PreviousEnvelopeDigest,
		EnvelopeDigest:         rootOutbox.EnvelopeDigest,
		CertificateID:          rootOutbox.CertificateID,
		KeyGeneration:          rootOutbox.KeyGeneration,
		Nonce:                  rootOutbox.Nonce,
		Fact:                   root.Fact,
	}
	remoteInbox := OpaqueSyncFrame{ArrivalSequence: 1, EnvelopeDigest: remoteDigest, SealedEnvelope: remoteBytes}
	rootInbox := OpaqueSyncFrame{ArrivalSequence: 2, EnvelopeDigest: rootOutbox.EnvelopeDigest, SealedEnvelope: rootOutbox.SealedEnvelope}
	if _, err := store.StageSyncPage(context.Background(), projectID, testSyncChannelID("channel-a"), 0, 2, []OpaqueSyncFrame{remoteInbox, rootInbox}); err != nil {
		t.Fatalf("StageSyncPage(candidate) error = %v", err)
	}
	retireSyncEnvironmentForGateV1(t, store, projectID, "environment-a", 1, remoteDigest)
	retireSyncEnvironmentForGateV1(t, store, projectID, "environment-b", 3, sha256.Sum256([]byte("unseen retired endpoint")))
	_, err = store.CurrentSyncAuthority(context.Background(), projectID)
	if err != nil {
		t.Fatalf("CurrentSyncAuthority() error = %v", err)
	}
	candidate, err := store.StageVerifiedTerminalCandidateChunk(context.Background(), projectID, currentSyncAuthorityBindingForTest(t, store, projectID), []VerifiedTerminalCandidateFrame{
		{Inbox: remoteInbox, Sealed: &remote},
		{Inbox: rootInbox, Sealed: &rootEcho},
	}, 1_000, 100)
	if err != nil {
		t.Fatalf("stage candidate: %v", err)
	}
	if _, err := store.PromoteTerminalCandidate(context.Background(), projectID, terminalCandidateCheckpointV1(candidate)); err != nil {
		t.Fatalf("PromoteTerminalCandidate() error = %v", err)
	}
	var rootOutboxCount, secondOutboxCount, rootReceiptCount, remoteFactCount int64
	checks := []struct {
		query string
		arg   any
		want  *int64
	}{
		{`SELECT COUNT(*) FROM continuity_sync_outbox WHERE project_id = ? AND fact_id = ?`, rootOutbox.FactID, &rootOutboxCount},
		{`SELECT COUNT(*) FROM continuity_sync_outbox WHERE project_id = ? AND fact_id = ?`, secondOutbox.FactID, &secondOutboxCount},
		{`SELECT COUNT(*) FROM continuity_sync_receipts WHERE project_id = ? AND fact_id = ?`, rootOutbox.FactID, &rootReceiptCount},
		{`SELECT COUNT(*) FROM continuity_facts WHERE project_id = ? AND fact_id = ?`, remoteFact.FactID, &remoteFactCount},
	}
	for _, check := range checks {
		if err := store.db.QueryRow(check.query, string(projectID), string(check.arg.(continuity.FactID))).Scan(check.want); err != nil {
			t.Fatalf("read promoted echo state: %v", err)
		}
	}
	if rootOutboxCount != 0 || secondOutboxCount != 1 || rootReceiptCount != 1 || remoteFactCount != 1 {
		t.Fatalf("echo state: root outbox=%d second outbox=%d root receipt=%d remote fact=%d", rootOutboxCount, secondOutboxCount, rootReceiptCount, remoteFactCount)
	}
}

func TestPromoteTerminalCandidateConcurrentExactRetry(t *testing.T) {
	t.Parallel()

	_, store, projectID, _, frames := terminalCandidateMixedFixtureV1(t, "terminal-candidate-promotion-concurrent", 2)
	candidate, err := store.StageVerifiedTerminalCandidateChunk(context.Background(), projectID, currentSyncAuthorityBindingForTest(t, store, projectID), frames, 1_000, 100)
	if err != nil {
		t.Fatalf("stage candidate: %v", err)
	}
	checkpoint := terminalCandidateCheckpointV1(candidate)
	var results [2]struct {
		receipt TerminalCandidateReceipt
		err     error
	}
	ready := make(chan struct{})
	var wait sync.WaitGroup
	wait.Add(len(results))
	for index := range results {
		go func(index int) {
			defer wait.Done()
			<-ready
			results[index].receipt, results[index].err = store.PromoteTerminalCandidate(context.Background(), projectID, checkpoint)
		}(index)
	}
	close(ready)
	wait.Wait()
	if results[0].err != nil || results[1].err != nil || results[0].receipt != results[1].receipt {
		t.Fatalf("concurrent promotion results = %#v", results)
	}
}

func TestPromoteTerminalCandidateRejectsPrunedLiveFactWithoutMutation(t *testing.T) {
	t.Parallel()

	store := openSyncStore(t, "terminal-candidate-promotion-pruned-live")
	projectID := continuity.ProjectID("project-terminal-candidate-promotion-pruned-live")
	installTestSyncAuthority(t, store, projectID, testSyncChannelID("channel-a"))
	root := syncProjectFact(t, projectID, "fact-terminal-candidate-live-root", "environment-local", 1, 103)
	frames := terminalCandidatePrunedThenSealedFramesV1(t, projectID, root)
	if _, err := store.StageSyncPage(context.Background(), projectID, testSyncChannelID("channel-a"), 0, 2, []OpaqueSyncFrame{frames[0].Inbox, frames[1].Inbox}); err != nil {
		t.Fatalf("StageSyncPage() error = %v", err)
	}
	_, err := store.CurrentSyncAuthority(context.Background(), projectID)
	if err != nil {
		t.Fatalf("CurrentSyncAuthority() error = %v", err)
	}
	candidate, err := store.StageVerifiedTerminalCandidateChunk(context.Background(), projectID, currentSyncAuthorityBindingForTest(t, store, projectID), frames, 1_000, 100)
	if err != nil {
		t.Fatalf("stage candidate: %v", err)
	}
	insertSnapshotStoredFactV1(t, store, storedFactFromWireForPromotionTestV1(root))
	open := syncScratchpadFactV1(t, projectID, "fact-terminal-candidate-live-open", "scratchpad-live", continuity.FactScratchpadOpened, "environment-scratchpad-live", 1, 100)
	participant := syncScratchpadFactV1(t, projectID, "fact-terminal-candidate-live-participant", "scratchpad-live", continuity.FactScratchpadParticipantIntroduced, "environment-scratchpad-live", 2, 101)
	message := syncScratchpadFactV1(t, projectID, frames[0].Pruned.Reference.FactID, "scratchpad-live", continuity.FactScratchpadMessageRecorded, frames[0].Pruned.Reference.EnvironmentID, frames[0].Pruned.Reference.EnvironmentSequence, frames[0].Pruned.HLC.WallMillis)
	insertSnapshotStoredFactV1(t, store, storedFactFromWireForPromotionTestV1(open))
	insertSnapshotStoredFactV1(t, store, storedFactFromWireForPromotionTestV1(participant))
	insertSnapshotStoredFactV1(t, store, storedFactFromWireForPromotionTestV1(message))
	before := captureTerminalMutationStateV1(t, store, projectID)
	_, err = store.PromoteTerminalCandidate(context.Background(), projectID, terminalCandidateCheckpointV1(candidate))
	assertSyncErrorCode(t, err, SyncErrorConflict)
	assertTerminalMutationStateV1(t, store, projectID, before)
	current, found, currentErr := store.CurrentTerminalCandidate(context.Background(), projectID)
	if currentErr != nil || !found || current != candidate {
		t.Fatalf("candidate after pruned/live rejection = (%#v, %v, %v), want %#v", current, found, currentErr, candidate)
	}
}

func TestPromoteTerminalCandidateSealedTombstoneDuplicateDoesNotResurrect(t *testing.T) {
	t.Parallel()

	store := openSyncStore(t, "terminal-candidate-promotion-sealed-tombstone")
	projectID := continuity.ProjectID("project-terminal-candidate-promotion-sealed-tombstone")
	installTestSyncAuthority(t, store, projectID, testSyncChannelID("channel-a"))
	message := syncScratchpadFactV1(t, projectID, "fact-terminal-candidate-tombstoned-message", "scratchpad-tombstoned", continuity.FactScratchpadMessageRecorded, "environment-local", 1, 101)
	frames := terminalCandidatePrunedThenSealedFramesV1(t, projectID, message)
	if _, err := store.StageSyncPage(context.Background(), projectID, testSyncChannelID("channel-a"), 0, 2, []OpaqueSyncFrame{frames[0].Inbox, frames[1].Inbox}); err != nil {
		t.Fatalf("StageSyncPage() error = %v", err)
	}
	_, err := store.CurrentSyncAuthority(context.Background(), projectID)
	if err != nil {
		t.Fatalf("CurrentSyncAuthority() error = %v", err)
	}
	candidate, err := store.StageVerifiedTerminalCandidateChunk(context.Background(), projectID, currentSyncAuthorityBindingForTest(t, store, projectID), frames, 1_000, 100)
	if err != nil {
		t.Fatalf("stage candidate: %v", err)
	}
	root := syncProjectFact(t, projectID, "fact-terminal-candidate-retained-root", "environment-root", 1, 99)
	insertSnapshotStoredFactV1(t, store, storedFactFromWireForPromotionTestV1(root))
	sealed := frames[1].Sealed
	pruneID := sha256.Sum256([]byte("terminal-candidate-existing-prune"))
	if _, err := store.db.Exec(`
INSERT INTO continuity_sync_environment_heads(
  project_id, environment_id, highest_sequence, hlc_wall_millis, hlc_logical,
  sealed_sequence, previous_envelope_digest, envelope_digest, certificate_id,
  key_generation, nonce
) VALUES(?, ?, 1, ?, ?, 1, ?, ?, ?, ?, ?)`,
		string(projectID), string(sealed.Fact.EnvironmentID), sealed.Fact.HLCWallMillis, sealed.Fact.HLCLogical,
		sealed.PreviousEnvelopeDigest[:], sealed.EnvelopeDigest[:], sealed.CertificateID[:], sealed.KeyGeneration, sealed.Nonce[:]); err != nil {
		t.Fatalf("insert tombstoned source head: %v", err)
	}
	if _, err := store.db.Exec(`
INSERT INTO continuity_sync_tombstones(
  fact_id, project_id, environment_id, environment_sequence, arrival_sequence,
  previous_envelope_digest, envelope_digest, certificate_id, key_generation,
  nonce, prune_certificate_id
) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		string(sealed.Fact.FactID), string(projectID), string(sealed.Fact.EnvironmentID), sealed.Fact.EnvironmentSequence, sealed.ArrivalSequence,
		sealed.PreviousEnvelopeDigest[:], sealed.EnvelopeDigest[:], sealed.CertificateID[:], sealed.KeyGeneration, sealed.Nonce[:], pruneID[:]); err != nil {
		t.Fatalf("insert exact tombstone: %v", err)
	}
	if _, err := store.PromoteTerminalCandidate(context.Background(), projectID, terminalCandidateCheckpointV1(candidate)); err != nil {
		t.Fatalf("PromoteTerminalCandidate() error = %v", err)
	}
	var live, receipt, tombstone int64
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM continuity_facts WHERE fact_id = ?`, string(sealed.Fact.FactID)).Scan(&live); err != nil {
		t.Fatalf("count resurrected fact: %v", err)
	}
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM continuity_sync_receipts WHERE project_id = ? AND fact_id = ?`, string(projectID), string(sealed.Fact.FactID)).Scan(&receipt); err != nil {
		t.Fatalf("count sealed receipt: %v", err)
	}
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM continuity_sync_tombstones WHERE project_id = ? AND fact_id = ? AND prune_certificate_id = ?`, string(projectID), string(sealed.Fact.FactID), pruneID[:]).Scan(&tombstone); err != nil {
		t.Fatalf("count preserved tombstone: %v", err)
	}
	if live != 0 || receipt != 1 || tombstone != 1 {
		t.Fatalf("sealed/tombstone duplicate state: live=%d receipt=%d tombstone=%d", live, receipt, tombstone)
	}
}

func TestPromoteTerminalCandidateRejectsSealedFactTombstonedForAnotherProject(t *testing.T) {
	t.Parallel()

	_, store, projectID, _, frames := terminalCandidateMixedFixtureV1(t, "terminal-candidate-promotion-cross-project-tombstone", 2)
	candidate, err := store.StageVerifiedTerminalCandidateChunk(context.Background(), projectID, currentSyncAuthorityBindingForTest(t, store, projectID), frames, 1_000, 100)
	if err != nil {
		t.Fatalf("stage candidate: %v", err)
	}
	ownerProjectID := continuity.ProjectID("project-terminal-candidate-promotion-tombstone-owner")
	insertTerminalCandidateOtherProjectTombstoneV1(t, store, ownerProjectID, *frames[0].Sealed)

	_, err = store.PromoteTerminalCandidate(context.Background(), projectID, terminalCandidateCheckpointV1(candidate))
	assertSyncErrorCode(t, err, SyncErrorConflict)
	assertTerminalCandidatePromotionUnchangedV1(t, store, projectID, candidate)
	var retained int64
	if err := store.db.QueryRow(
		`SELECT COUNT(*) FROM continuity_sync_tombstones WHERE project_id = ? AND fact_id = ?`,
		string(ownerProjectID),
		string(frames[0].Sealed.Fact.FactID),
	).Scan(&retained); err != nil {
		t.Fatalf("count cross-project tombstone: %v", err)
	}
	if retained != 1 {
		t.Fatalf("cross-project tombstone count = %d, want 1", retained)
	}
}

func TestPromoteTerminalCandidateRejectsRepeatedSealedReceiptAtDifferentArrival(t *testing.T) {
	t.Parallel()

	store := openSyncStore(t, "terminal-candidate-promotion-repeated-receipt")
	projectID := continuity.ProjectID("project-terminal-candidate-promotion-repeated-receipt")
	root := syncProjectFact(t, projectID, "fact-terminal-candidate-repeated-root", "environment-a", 1, 100)
	appliedFrames := stageSyncFacts(t, store, projectID, 1, []continuitywire.Fact{root})
	if _, err := store.ApplySyncBatch(context.Background(), projectID, currentSyncAuthorityBindingForTest(t, store, projectID), appliedFrames, 1_000, 100); err != nil {
		t.Fatalf("apply original sealed frame: %v", err)
	}

	frames := terminalCandidatePrunedThenSealedFramesV1(t, projectID, root)
	frames[0].Inbox.ArrivalSequence = 2
	frames[0].Pruned.Reference.ArrivalSequence = 2
	frames[0].Pruned.Reference.EnvironmentID = "environment-b"
	frames[0].Pruned.Reference.CertificateID = testSyncCertificateID("environment-b")
	frames[1].Inbox.ArrivalSequence = 3
	frames[1].Inbox.EnvelopeDigest = appliedFrames[0].EnvelopeDigest
	repeated := appliedFrames[0]
	repeated.ArrivalSequence = 3
	frames[1].Sealed = &repeated
	if _, err := store.db.Exec(`
INSERT INTO continuity_sync_inbox(
  project_id, arrival_sequence, envelope_digest, frame_kind, frame_bytes, state
) VALUES
  (?, 2, ?, 'pruned', ?, 'staged'),
  (?, 3, ?, 'sealed', ?, 'staged')`,
		string(projectID), frames[0].Inbox.EnvelopeDigest[:], frames[0].Inbox.PrunedArrival,
		string(projectID), frames[1].Inbox.EnvelopeDigest[:], frames[1].Inbox.SealedEnvelope); err != nil {
		t.Fatalf("inject repeated relay inbox: %v", err)
	}
	if _, err := store.db.Exec(`
UPDATE continuity_sync_projects
SET downloaded_cursor = 3, relay_head = 3
WHERE project_id = ? AND applied_cursor = 1 AND downloaded_cursor = 1`, string(projectID)); err != nil {
		t.Fatalf("advance repeated relay download cursor: %v", err)
	}
	_, err := store.CurrentSyncAuthority(context.Background(), projectID)
	if err != nil {
		t.Fatalf("CurrentSyncAuthority() error = %v", err)
	}
	candidate, err := store.StageVerifiedTerminalCandidateChunk(context.Background(), projectID, currentSyncAuthorityBindingForTest(t, store, projectID), frames, 1_000, 100)
	if err != nil {
		t.Fatalf("stage repeated candidate: %v", err)
	}
	before := captureTerminalMutationStateV1(t, store, projectID)

	_, err = store.PromoteTerminalCandidate(context.Background(), projectID, terminalCandidateCheckpointV1(candidate))
	assertSyncErrorCode(t, err, SyncErrorConflict)
	assertTerminalMutationStateV1(t, store, projectID, before)
	current, found, currentErr := store.CurrentTerminalCandidate(context.Background(), projectID)
	if currentErr != nil || !found || current != candidate {
		t.Fatalf("candidate after receipt collision = (%#v, %v, %v), want %#v", current, found, currentErr, candidate)
	}
}

func TestPromoteTerminalCandidateHasNoLifetimeFrameCap(t *testing.T) {
	const frameCount = 4_100

	store := openSyncStore(t, "terminal-candidate-promotion-no-lifetime-cap")
	projectID := continuity.ProjectID("project-terminal-candidate-promotion-no-lifetime-cap")
	installTestSyncAuthority(t, store, projectID, testSyncChannelID("channel-a"))
	certificateID := testSyncCertificateID("environment-a")
	pruneCertificateID := sha256.Sum256([]byte("terminal-candidate-promotion-long-prune"))
	frames := make([]VerifiedTerminalCandidateFrame, 0, frameCount)
	opaque := make([]OpaqueSyncFrame, 0, frameCount)
	previousDigest := [32]byte{}
	for sequence := 1; sequence <= frameCount; sequence++ {
		arrival := int64(sequence)
		label := strconv.Itoa(sequence)
		if sequence == 1 {
			fact := syncProjectFact(t, projectID, "fact-terminal-candidate-promotion-long-root", "environment-a", 1, 100)
			encoded, err := continuitywire.Encode(fact)
			if err != nil {
				t.Fatalf("encode long root: %v", err)
			}
			sealedBytes := append([]byte("sealed:"), encoded...)
			digest := sha256.Sum256(sealedBytes)
			inbox := OpaqueSyncFrame{ArrivalSequence: 1, EnvelopeDigest: digest, SealedEnvelope: sealedBytes}
			verified := VerifiedSyncFrame{
				ArrivalSequence: 1,
				EnvelopeDigest:  digest,
				CertificateID:   certificateID,
				KeyGeneration:   1,
				Nonce:           testNonce("terminal-candidate-promotion-long:1"),
				Fact:            fact,
			}
			opaque = append(opaque, inbox)
			frames = append(frames, VerifiedTerminalCandidateFrame{Inbox: inbox, Sealed: &verified})
			previousDigest = digest
			continue
		}
		digest := sha256.Sum256([]byte("terminal-candidate-promotion-long-envelope:" + label))
		reference := VerifiedPruneReference{
			FactID:                 continuity.FactID("fact-terminal-candidate-promotion-long-" + label),
			EnvironmentID:          "environment-a",
			EnvironmentSequence:    arrival,
			ArrivalSequence:        arrival,
			EnvelopeDigest:         digest,
			CertificateID:          certificateID,
			PreviousEnvelopeDigest: previousDigest,
			KeyGeneration:          1,
			Nonce:                  testNonce("terminal-candidate-promotion-long:" + label),
		}
		inbox := OpaqueSyncFrame{ArrivalSequence: arrival, EnvelopeDigest: digest, PrunedArrival: []byte("pruned:" + label)}
		pruned := VerifiedTerminalPrunedFrame{
			Reference:          reference,
			PruneCertificateID: pruneCertificateID,
			FactKind:           continuity.FactScratchpadMessageRecorded,
			HLC:                continuity.HybridTime{WallMillis: int64(99 + sequence)},
		}
		opaque = append(opaque, inbox)
		frames = append(frames, VerifiedTerminalCandidateFrame{Inbox: inbox, Pruned: &pruned})
		previousDigest = digest
	}
	for offset := 0; offset < len(opaque); offset += maximumSyncPageFrames {
		end := offset + maximumSyncPageFrames
		if end > len(opaque) {
			end = len(opaque)
		}
		if _, err := store.StageSyncPage(context.Background(), projectID, testSyncChannelID("channel-a"), int64(offset), frameCount, opaque[offset:end]); err != nil {
			t.Fatalf("StageSyncPage(%d:%d) error = %v", offset, end, err)
		}
	}
	retireSyncEnvironmentForGateV1(t, store, projectID, "environment-a", frameCount, previousDigest)
	_, err := store.CurrentSyncAuthority(context.Background(), projectID)
	if err != nil {
		t.Fatalf("CurrentSyncAuthority() error = %v", err)
	}
	var candidate TerminalCandidate
	for offset := 0; offset < len(frames); offset += maximumTerminalCandidateChunkFramesV1 {
		end := offset + maximumTerminalCandidateChunkFramesV1
		if end > len(frames) {
			end = len(frames)
		}
		candidate, err = store.StageVerifiedTerminalCandidateChunk(context.Background(), projectID, currentSyncAuthorityBindingForTest(t, store, projectID), frames[offset:end], frameCount+1_000, 100)
		if err != nil {
			t.Fatalf("StageVerifiedTerminalCandidateChunk(%d:%d) error = %v", offset, end, err)
		}
	}
	receipt, err := store.PromoteTerminalCandidate(context.Background(), projectID, terminalCandidateCheckpointV1(candidate))
	if err != nil {
		t.Fatalf("PromoteTerminalCandidate(%d frames) error = %v", frameCount, err)
	}
	if receipt.FrameCount != frameCount || receipt.ResultingAppliedCursor != frameCount {
		t.Fatalf("long promotion receipt = %#v", receipt)
	}
	var facts, tombstones, children, inbox int64
	for _, check := range []struct {
		table string
		want  *int64
	}{
		{"continuity_facts", &facts},
		{"continuity_sync_tombstones", &tombstones},
		{"continuity_sync_terminal_candidate_frames", &children},
		{"continuity_sync_inbox", &inbox},
	} {
		if err := store.db.QueryRow("SELECT COUNT(*) FROM "+check.table+" WHERE project_id = ?", string(projectID)).Scan(check.want); err != nil {
			t.Fatalf("count %s: %v", check.table, err)
		}
	}
	if facts != 1 || tombstones != frameCount-1 || children != 0 || inbox != 0 {
		t.Fatalf("long promotion state: facts=%d tombstones=%d children=%d inbox=%d", facts, tombstones, children, inbox)
	}
}

func TestTerminalCandidateCorpusDigestV1Vector(t *testing.T) {
	t.Parallel()

	projectID := continuity.ProjectID("project-terminal-candidate-corpus-vector")
	root := storedFactFromWireForPromotionTestV1(syncProjectFact(t, projectID, "fact-corpus-root", "environment-a", 1, 100))
	idea := storedFactFromWireForPromotionTestV1(syncIdeaCreatedFact(t, projectID, "fact-corpus-idea", "idea-corpus", "environment-b", 1, 101, "Corpus"))
	facts := []storedFactV1{root, idea}
	digest, err := terminalCandidateCorpusDigestV1(projectID, facts)
	if err != nil {
		t.Fatalf("terminalCandidateCorpusDigestV1() error = %v", err)
	}
	const want = "bdcbfc758f1020a2c0ebb1e28c8c801558768f52bd36592cf6859c307c970770"
	if got := hex.EncodeToString(digest[:]); got != want {
		t.Fatalf("corpus digest = %s, want %s", got, want)
	}
	second, err := terminalCandidateCorpusDigestV1(projectID, append([]storedFactV1(nil), facts...))
	if err != nil || second != digest {
		t.Fatalf("repeated corpus digest = (%x, %v), want %x", second, err, digest)
	}
	if _, err := terminalCandidateCorpusDigestV1(projectID, []storedFactV1{idea, root}); err == nil {
		t.Fatal("terminalCandidateCorpusDigestV1(unsorted) error = nil")
	}
}

func storedFactFromWireForPromotionTestV1(fact continuitywire.Fact) storedFactV1 {
	return storedFactV1{
		factID:              fact.FactID,
		projectID:           fact.ProjectID,
		subject:             continuity.SubjectRef{Kind: fact.SubjectKind, ID: fact.SubjectID},
		kind:                fact.FactKind,
		payloadVersion:      int(fact.PayloadVersion),
		content:             canonicalContentV1(string(fact.CanonicalPayload)),
		environmentID:       fact.EnvironmentID,
		environmentSequence: fact.EnvironmentSequence,
		clock:               continuity.HybridTime{WallMillis: fact.HLCWallMillis, Logical: fact.HLCLogical},
		envelopeVersion:     int(fact.EnvelopeVersion),
	}
}

func terminalCandidatePrunedWithLocalRootV1(t *testing.T, suffix string, retireUntouched bool) (*Store, continuity.ProjectID, SyncAuthority, VerifiedTerminalCandidateFrame) {
	t.Helper()
	store := openAppendStoreV1(t, filepath.Join(testTempDir(t), "state-"+suffix), "environment-local", 100)
	projectID := continuity.ProjectID("project-" + suffix)
	mustAppendV1(t)(store.RegisterProject(context.Background(), projectID, continuity.FactID("fact-root-"+suffix), continuity.ProjectRegistrationPayload{
		Observation: appendObservationV1(),
		Label:       "Loaf",
	}))
	installTestSyncAuthority(t, store, projectID, testSyncChannelID("channel-a"))
	digest := sha256.Sum256([]byte("terminal-candidate-active-pruned:" + suffix))
	reference := VerifiedPruneReference{
		FactID:              continuity.FactID("fact-pruned-" + suffix),
		EnvironmentID:       "environment-a",
		EnvironmentSequence: 1,
		ArrivalSequence:     1,
		EnvelopeDigest:      digest,
		CertificateID:       testSyncCertificateID("environment-a"),
		KeyGeneration:       1,
		Nonce:               testNonce("terminal-candidate-active-pruned:" + suffix),
	}
	inbox := OpaqueSyncFrame{ArrivalSequence: 1, EnvelopeDigest: digest, PrunedArrival: []byte("pruned:" + suffix)}
	frame := VerifiedTerminalCandidateFrame{
		Inbox: inbox,
		Pruned: &VerifiedTerminalPrunedFrame{
			Reference:          reference,
			PruneCertificateID: sha256.Sum256([]byte("prune-certificate:" + suffix)),
			FactKind:           continuity.FactScratchpadMessageRecorded,
			HLC:                continuity.HybridTime{WallMillis: 200},
		},
	}
	if _, err := store.StageSyncPage(context.Background(), projectID, testSyncChannelID("channel-a"), 0, 1, []OpaqueSyncFrame{inbox}); err != nil {
		t.Fatalf("StageSyncPage() error = %v", err)
	}
	if retireUntouched {
		retireSyncEnvironmentForGateV1(t, store, projectID, "environment-b", 1, sha256.Sum256([]byte("untouched-retirement:"+suffix)))
	}
	authority, err := store.CurrentSyncAuthority(context.Background(), projectID)
	if err != nil {
		t.Fatalf("CurrentSyncAuthority() error = %v", err)
	}
	return store, projectID, authority, frame
}

func terminalCandidatePrunedThenSealedFramesV1(t *testing.T, projectID continuity.ProjectID, fact continuitywire.Fact) []VerifiedTerminalCandidateFrame {
	t.Helper()
	prunedDigest := sha256.Sum256([]byte("terminal-candidate-first-pruned:" + string(projectID)))
	prunedInbox := OpaqueSyncFrame{ArrivalSequence: 1, EnvelopeDigest: prunedDigest, PrunedArrival: []byte("pruned:" + string(projectID))}
	pruned := VerifiedTerminalPrunedFrame{
		Reference: VerifiedPruneReference{
			FactID:              continuity.FactID("fact-pruned-" + string(projectID)),
			EnvironmentID:       "environment-a",
			EnvironmentSequence: 1,
			ArrivalSequence:     1,
			EnvelopeDigest:      prunedDigest,
			CertificateID:       testSyncCertificateID("environment-a"),
			KeyGeneration:       1,
			Nonce:               testNonce("terminal-candidate-first-pruned:" + string(projectID)),
		},
		PruneCertificateID: sha256.Sum256([]byte("prune-certificate:" + string(projectID))),
		FactKind:           continuity.FactScratchpadMessageRecorded,
		HLC:                continuity.HybridTime{WallMillis: 100},
	}
	encoded, err := continuitywire.Encode(fact)
	if err != nil {
		t.Fatalf("encode sealed terminal fact: %v", err)
	}
	sealedBytes := append([]byte("sealed:"), encoded...)
	sealedDigest := sha256.Sum256(sealedBytes)
	sealedInbox := OpaqueSyncFrame{ArrivalSequence: 2, EnvelopeDigest: sealedDigest, SealedEnvelope: sealedBytes}
	certificateID := testSyncCertificateID(string(fact.EnvironmentID))
	if fact.EnvironmentID == "environment-local" {
		certificateID = sha256.Sum256([]byte("local certificate"))
	}
	sealed := VerifiedSyncFrame{
		ArrivalSequence: 2,
		EnvelopeDigest:  sealedDigest,
		CertificateID:   certificateID,
		KeyGeneration:   1,
		Nonce:           testNonce("terminal-candidate-second-sealed:" + string(projectID)),
		Fact:            fact,
	}
	return []VerifiedTerminalCandidateFrame{{Inbox: prunedInbox, Pruned: &pruned}, {Inbox: sealedInbox, Sealed: &sealed}}
}

func assertTerminalCandidatePromotionUnchangedV1(t *testing.T, store *Store, projectID continuity.ProjectID, candidate TerminalCandidate) {
	t.Helper()
	current, found, err := store.CurrentTerminalCandidate(context.Background(), projectID)
	if err != nil || !found || current != candidate {
		t.Fatalf("CurrentTerminalCandidate() = (%#v, %v, %v), want unchanged %#v", current, found, err, candidate)
	}
	var applied, facts, receipts, tombstones, children, inbox int64
	queries := []struct {
		label string
		query string
		want  *int64
	}{
		{"applied", `SELECT applied_cursor FROM continuity_sync_projects WHERE project_id = ?`, &applied},
		{"facts", `SELECT COUNT(*) FROM continuity_facts WHERE project_id = ?`, &facts},
		{"receipts", `SELECT COUNT(*) FROM continuity_sync_receipts WHERE project_id = ?`, &receipts},
		{"tombstones", `SELECT COUNT(*) FROM continuity_sync_tombstones WHERE project_id = ?`, &tombstones},
		{"children", `SELECT COUNT(*) FROM continuity_sync_terminal_candidate_frames WHERE project_id = ?`, &children},
		{"inbox", `SELECT COUNT(*) FROM continuity_sync_inbox WHERE project_id = ?`, &inbox},
	}
	for _, query := range queries {
		if err := store.db.QueryRow(query.query, string(projectID)).Scan(query.want); err != nil {
			t.Fatalf("read %s mutation state: %v", query.label, err)
		}
	}
	if applied != 0 || facts != 0 || receipts != 0 || tombstones != 0 || children != candidate.FrameCount || inbox < candidate.FrameCount {
		t.Fatalf("promotion failure mutated state: applied=%d facts=%d receipts=%d tombstones=%d children=%d inbox=%d", applied, facts, receipts, tombstones, children, inbox)
	}
}
