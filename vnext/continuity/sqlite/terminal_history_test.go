package sqlite

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"testing"

	"github.com/levifig/loaf/vnext/continuity"
	"github.com/levifig/loaf/vnext/internal/continuitywire"
)

func TestApplySyncBatchRejectsFirstSeenTerminalProducerWithoutMutation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		environmentID    continuity.EnvironmentID
		trustedNowMillis int64
		retire           bool
	}{
		{name: "retired trusted", environmentID: "environment-a", trustedNowMillis: 1_000, retire: true},
		{name: "expired ephemeral at expiry", environmentID: "environment-b", trustedNowMillis: 10_000},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			store := openSyncStore(t, "terminal-gate-"+syncSlug(test.name))
			projectID := continuity.ProjectID("project-terminal-gate-" + syncSlug(test.name))
			frames := stageSyncFacts(t, store, projectID, 1, []continuitywire.Fact{
				syncProjectFact(t, projectID, "fact-project", test.environmentID, 1, 100),
			})
			if test.retire {
				retireSyncEnvironmentForGateV1(t, store, projectID, test.environmentID, 1, frames[0].EnvelopeDigest)
			}
			before := captureTerminalMutationStateV1(t, store, projectID)
			_, err := store.ApplySyncBatch(context.Background(), projectID, currentSyncAuthorityBindingForTest(t, store, projectID), frames, test.trustedNowMillis, 100)
			assertContentFreeSyncCodeV1(t, err, SyncErrorTerminalHistoryRequired)
			assertTerminalMutationStateV1(t, store, projectID, before)
		})
	}
}

func TestApplySyncBatchGatesFirstSeenEnvelopeWhenPlaintextAlreadyExists(t *testing.T) {
	t.Parallel()

	store := openAppendStoreV1(t, testTempDir(t), "environment-local", 100)
	projectID := continuity.ProjectID("project-terminal-local-plaintext")
	mustAppendV1(t)(store.RegisterProject(context.Background(), projectID, "fact-project", continuity.ProjectRegistrationPayload{Observation: appendObservationV1(), Label: "Local"}))
	fact, err := store.ExportFact(context.Background(), "fact-project")
	if err != nil {
		t.Fatalf("ExportFact() error = %v", err)
	}
	installTestSyncAuthority(t, store, projectID, testSyncChannelID("channel-a"))
	encoded, err := continuitywire.Encode(fact)
	if err != nil {
		t.Fatalf("encode local fact: %v", err)
	}
	sealed := append([]byte("sealed:"), encoded...)
	digest := sha256.Sum256(sealed)
	frame := VerifiedSyncFrame{
		ArrivalSequence: 1,
		EnvelopeDigest:  digest,
		CertificateID:   sha256.Sum256([]byte("local certificate")),
		KeyGeneration:   1,
		Nonce:           testNonce("terminal-local-plaintext"),
		Fact:            fact,
	}
	if _, err := store.StageSyncPage(context.Background(), projectID, testSyncChannelID("channel-a"), 0, 1, []OpaqueSyncFrame{{ArrivalSequence: 1, EnvelopeDigest: digest, SealedEnvelope: sealed}}); err != nil {
		t.Fatalf("StageSyncPage() error = %v", err)
	}
	retireSyncEnvironmentForGateV1(t, store, projectID, "environment-local", 1, digest)
	before := captureTerminalMutationStateV1(t, store, projectID)
	_, err = store.ApplySyncBatch(context.Background(), projectID, currentSyncAuthorityBindingForTest(t, store, projectID), []VerifiedSyncFrame{frame}, 1_000, 100)
	assertContentFreeSyncCodeV1(t, err, SyncErrorTerminalHistoryRequired)
	assertTerminalMutationStateV1(t, store, projectID, before)
}

func TestApplySyncBatchBindsEveryFrameToPinnedCertificateInventory(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		environmentID continuity.EnvironmentID
		wrongCert     bool
	}{
		{name: "unknown environment", environmentID: "environment-unknown"},
		{name: "wrong certificate", environmentID: "environment-a", wrongCert: true},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			store := openSyncStore(t, "terminal-cert-"+syncSlug(test.name))
			projectID := continuity.ProjectID("project-terminal-cert-" + syncSlug(test.name))
			frames := stageSyncFacts(t, store, projectID, 1, []continuitywire.Fact{
				syncProjectFact(t, projectID, "fact-project", test.environmentID, 1, 100),
			})
			if test.wrongCert {
				frames[0].CertificateID = sha256.Sum256([]byte("wrong pinned certificate"))
			}
			before := captureTerminalMutationStateV1(t, store, projectID)
			if _, err := store.ApplySyncBatch(context.Background(), projectID, currentSyncAuthorityBindingForTest(t, store, projectID), frames, 1_000, 100); err == nil {
				t.Fatal("ApplySyncBatch() error = nil")
			} else {
				assertSyncErrorCode(t, err, SyncErrorCertificate)
			}
			assertTerminalMutationStateV1(t, store, projectID, before)
		})
	}
}

func TestApplySyncBatchAllowsExactRetainedSealedEnvelopeFromRetiredProducer(t *testing.T) {
	t.Parallel()

	store := openAppendStoreV1(t, testTempDir(t), "environment-local", 100)
	projectID := continuity.ProjectID("project-terminal-sealed-echo")
	mustAppendV1(t)(store.RegisterProject(context.Background(), projectID, "fact-project", continuity.ProjectRegistrationPayload{Observation: appendObservationV1(), Label: "Local"}))
	installTestSyncAuthority(t, store, projectID, testSyncChannelID("channel-a"))
	if _, err := store.ActivateStagedSync(context.Background(), projectID, testSyncChannelID("channel-a")); err != nil {
		t.Fatalf("ActivateStagedSync() error = %v", err)
	}
	fact, err := store.ExportFact(context.Background(), "fact-project")
	if err != nil {
		t.Fatalf("ExportFact() error = %v", err)
	}
	sealed := []byte("sealed terminal local echo")
	outbox := SealedOutboxFrame{
		FactID:         fact.FactID,
		EnvelopeDigest: sha256.Sum256(sealed),
		CertificateID:  sha256.Sum256([]byte("local certificate")),
		KeyGeneration:  1,
		Nonce:          testNonce("terminal sealed echo"),
		SealedEnvelope: sealed,
	}
	if err := store.PersistSealedOutbox(context.Background(), projectID, testSyncChannelID("channel-a"), outbox); err != nil {
		t.Fatalf("PersistSealedOutbox() error = %v", err)
	}
	retireSyncEnvironmentForGateV1(t, store, projectID, "environment-local", 1, outbox.EnvelopeDigest)
	if _, err := store.StageSyncPage(context.Background(), projectID, testSyncChannelID("channel-a"), 0, 1, []OpaqueSyncFrame{{ArrivalSequence: 1, EnvelopeDigest: outbox.EnvelopeDigest, SealedEnvelope: outbox.SealedEnvelope}}); err != nil {
		t.Fatalf("StageSyncPage() error = %v", err)
	}
	frame := VerifiedSyncFrame{
		ArrivalSequence:        1,
		PreviousEnvelopeDigest: outbox.PreviousEnvelopeDigest,
		EnvelopeDigest:         outbox.EnvelopeDigest,
		CertificateID:          outbox.CertificateID,
		KeyGeneration:          outbox.KeyGeneration,
		Nonce:                  outbox.Nonce,
		Fact:                   fact,
	}
	progress, err := store.ApplySyncBatch(context.Background(), projectID, currentSyncAuthorityBindingForTest(t, store, projectID), []VerifiedSyncFrame{frame}, 1_000, 100)
	if err != nil {
		t.Fatalf("ApplySyncBatch(exact retained sealed echo) error = %v", err)
	}
	if progress.AppliedCursor != 1 || progress.DownloadedCursor != 1 {
		t.Fatalf("progress = %#v, want cursors at 1", progress)
	}
	var receipts, inbox, pendingOutbox int
	for query, target := range map[string]*int{
		`SELECT COUNT(*) FROM continuity_sync_receipts WHERE project_id = ?`: &receipts,
		`SELECT COUNT(*) FROM continuity_sync_inbox WHERE project_id = ?`:    &inbox,
		`SELECT COUNT(*) FROM continuity_sync_outbox WHERE project_id = ?`:   &pendingOutbox,
	} {
		if err := store.db.QueryRow(query, string(projectID)).Scan(target); err != nil {
			t.Fatalf("count retained echo rows: %v", err)
		}
	}
	if receipts != 1 || inbox != 0 || pendingOutbox != 0 {
		t.Fatalf("retained echo rows = receipts %d inbox %d outbox %d, want 1,0,0", receipts, inbox, pendingOutbox)
	}
}

func TestApplySyncBatchTreatsAlteredRetainedTerminalEnvelopeAsFirstSeen(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		environmentID     continuity.EnvironmentID
		initialNowMillis  int64
		terminalNowMillis int64
		retire            bool
	}{
		{name: "retired trusted", environmentID: "environment-a", initialNowMillis: 1_000, terminalNowMillis: 1_000, retire: true},
		{name: "expired ephemeral", environmentID: "environment-b", initialNowMillis: 9_999, terminalNowMillis: 10_000},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			store := openSyncStore(t, "terminal-altered-retained-"+syncSlug(test.name))
			projectID := continuity.ProjectID("project-terminal-altered-retained-" + syncSlug(test.name))
			fact := syncProjectFact(t, projectID, "fact-project", test.environmentID, 1, 100)
			retained := stageSyncFacts(t, store, projectID, 1, []continuitywire.Fact{fact})
			if _, err := store.ApplySyncBatch(context.Background(), projectID, currentSyncAuthorityBindingForTest(t, store, projectID), retained, test.initialNowMillis, 100); err != nil {
				t.Fatalf("ApplySyncBatch(retained) error = %v", err)
			}
			if test.retire {
				retireSyncEnvironmentForGateV1(t, store, projectID, test.environmentID, 1, retained[0].EnvelopeDigest)
			}

			sealed := []byte("altered retained terminal envelope:" + test.name)
			digest := sha256.Sum256(sealed)
			if _, err := store.StageSyncPage(context.Background(), projectID, testSyncChannelID("channel-a"), 1, 2, []OpaqueSyncFrame{{
				ArrivalSequence: 2,
				EnvelopeDigest:  digest,
				SealedEnvelope:  sealed,
			}}); err != nil {
				t.Fatalf("StageSyncPage(altered retained envelope) error = %v", err)
			}
			candidate := VerifiedSyncFrame{
				ArrivalSequence: 2,
				EnvelopeDigest:  digest,
				CertificateID:   testSyncCertificateID(string(test.environmentID)),
				KeyGeneration:   1,
				Nonce:           testNonce("altered retained terminal envelope:" + test.name),
				Fact:            fact,
			}
			before := captureTerminalMutationStateV1(t, store, projectID)
			_, err := store.ApplySyncBatch(context.Background(), projectID, currentSyncAuthorityBindingForTest(t, store, projectID), []VerifiedSyncFrame{candidate}, test.terminalNowMillis, 100)
			assertContentFreeSyncCodeV1(t, err, SyncErrorTerminalHistoryRequired)
			assertTerminalMutationStateV1(t, store, projectID, before)
		})
	}
}

func TestPendingSyncFramesAfterRepagesMoreThanOneRelayPageWithoutMutation(t *testing.T) {
	t.Parallel()

	store := openSyncStore(t, "terminal-repage")
	projectID := continuity.ProjectID("project-terminal-repage")
	installTestSyncAuthority(t, store, projectID, testSyncChannelID("channel-a"))
	all := make([]OpaqueSyncFrame, 300)
	for index := range all {
		all[index] = testOpaqueFrame(int64(index+1), "terminal-page-"+strconv.Itoa(index+1))
	}
	if _, err := store.StageSyncPage(context.Background(), projectID, testSyncChannelID("channel-a"), 0, 300, all[:256]); err != nil {
		t.Fatalf("StageSyncPage(first) error = %v", err)
	}
	if _, err := store.StageSyncPage(context.Background(), projectID, testSyncChannelID("channel-a"), 256, 300, all[256:]); err != nil {
		t.Fatalf("StageSyncPage(second) error = %v", err)
	}
	before := captureTerminalMutationStateV1(t, store, projectID)
	var got []OpaqueSyncFrame
	after := int64(0)
	for {
		page, err := store.PendingSyncFramesAfter(context.Background(), projectID, after, 97)
		if err != nil {
			t.Fatalf("PendingSyncFramesAfter(%d) error = %v", after, err)
		}
		if len(page) == 0 {
			break
		}
		got = append(got, page...)
		after = page[len(page)-1].ArrivalSequence
	}
	if len(got) != len(all) {
		t.Fatalf("repaged frames = %d, want %d", len(got), len(all))
	}
	for index := range all {
		if !opaqueSyncFrameEqual(got[index], all[index]) {
			t.Fatalf("repaged frame %d = %#v, want %#v", index, got[index], all[index])
		}
	}
	got[0].SealedEnvelope[0] ^= 0xff
	reloaded, err := store.PendingSyncFramesAfter(context.Background(), projectID, 0, 1)
	if err != nil {
		t.Fatalf("PendingSyncFramesAfter(reload) error = %v", err)
	}
	if !opaqueSyncFrameEqual(reloaded[0], all[0]) {
		t.Fatal("PendingSyncFramesAfter returned an alias into retained bytes")
	}
	for _, test := range []struct {
		after int64
		limit int
	}{
		{after: -1, limit: 1},
		{after: 301, limit: 1},
		{after: 0, limit: 0},
		{after: 0, limit: 257},
	} {
		if _, err := store.PendingSyncFramesAfter(context.Background(), projectID, test.after, test.limit); err == nil {
			t.Fatalf("PendingSyncFramesAfter(%d, %d) error = nil", test.after, test.limit)
		}
	}
	assertTerminalMutationStateV1(t, store, projectID, before)
}

func TestPendingSyncFramesAfterLivenessBoundaries(t *testing.T) {
	t.Parallel()

	store := openSyncStore(t, "terminal-pending-liveness")
	projectID := continuity.ProjectID("project-terminal-pending-liveness")
	installTestSyncAuthority(t, store, projectID, testSyncChannelID("channel-a"))
	before := captureTerminalMutationStateV1(t, store, projectID)
	if _, err := store.PendingSyncFramesAfter(nil, projectID, 0, 1); err == nil {
		t.Fatal("PendingSyncFramesAfter(nil context) error = nil")
	}
	if _, err := (*Store)(nil).PendingSyncFramesAfter(context.Background(), projectID, 0, 1); err == nil {
		t.Fatal("nil Store.PendingSyncFramesAfter() error = nil")
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := store.PendingSyncFramesAfter(canceled, projectID, 0, 1); !errors.Is(err, context.Canceled) {
		t.Fatalf("PendingSyncFramesAfter(canceled) error = %v, want context.Canceled", err)
	}
	assertTerminalMutationStateV1(t, store, projectID, before)

	closed := openSyncStore(t, "terminal-pending-closed")
	if err := closed.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if _, err := closed.PendingSyncFramesAfter(context.Background(), projectID, 0, 1); err == nil {
		t.Fatal("closed Store.PendingSyncFramesAfter() error = nil")
	}
}

func TestPendingSyncFramesAfterRejectsInboxCursorCorruption(t *testing.T) {
	t.Parallel()

	t.Run("row beyond downloaded cursor", func(t *testing.T) {
		store := openSyncStore(t, "terminal-pending-beyond-downloaded")
		projectID := continuity.ProjectID("project-terminal-pending-beyond-downloaded")
		installTestSyncAuthority(t, store, projectID, testSyncChannelID("channel-a"))
		first := testOpaqueFrame(1, "within-downloaded")
		if _, err := store.StageSyncPage(context.Background(), projectID, testSyncChannelID("channel-a"), 0, 1, []OpaqueSyncFrame{first}); err != nil {
			t.Fatalf("StageSyncPage() error = %v", err)
		}
		beyond := testOpaqueFrame(2, "beyond-downloaded")
		if _, err := store.db.Exec(`
INSERT INTO continuity_sync_inbox (
  project_id, arrival_sequence, envelope_digest, frame_kind, frame_bytes, state
) VALUES (?, ?, ?, 'sealed', ?, 'staged')`, string(projectID), beyond.ArrivalSequence, beyond.EnvelopeDigest[:], beyond.SealedEnvelope); err != nil {
			t.Fatalf("seed inbox row beyond downloaded cursor: %v", err)
		}
		before := captureTerminalMutationStateV1(t, store, projectID)
		_, err := store.PendingSyncFramesAfter(context.Background(), projectID, 1, 1)
		assertSyncErrorCode(t, err, SyncErrorStore)
		assertTerminalMutationStateV1(t, store, projectID, before)
	})

	t.Run("short page before downloaded cursor", func(t *testing.T) {
		store := openSyncStore(t, "terminal-pending-short-page")
		projectID := continuity.ProjectID("project-terminal-pending-short-page")
		installTestSyncAuthority(t, store, projectID, testSyncChannelID("channel-a"))
		frames := []OpaqueSyncFrame{
			testOpaqueFrame(1, "short-page-one"),
			testOpaqueFrame(2, "short-page-two"),
			testOpaqueFrame(3, "short-page-three"),
		}
		if _, err := store.StageSyncPage(context.Background(), projectID, testSyncChannelID("channel-a"), 0, 3, frames); err != nil {
			t.Fatalf("StageSyncPage() error = %v", err)
		}
		if _, err := store.db.Exec(`
DELETE FROM continuity_sync_inbox
WHERE project_id = ? AND arrival_sequence >= 2`, string(projectID)); err != nil {
			t.Fatalf("truncate staged inbox: %v", err)
		}
		before := captureTerminalMutationStateV1(t, store, projectID)
		_, err := store.PendingSyncFramesAfter(context.Background(), projectID, 0, 3)
		assertSyncErrorCode(t, err, SyncErrorStore)
		assertTerminalMutationStateV1(t, store, projectID, before)
	})
}

func TestPendingSyncFramesAfterHandlesMaximumCursorWithoutOverflow(t *testing.T) {
	t.Parallel()

	t.Run("exhausted maximum cursor", func(t *testing.T) {
		store := openSyncStore(t, "terminal-pending-max-exhausted")
		projectID := continuity.ProjectID("project-terminal-pending-max-exhausted")
		installTestSyncAuthority(t, store, projectID, testSyncChannelID("channel-a"))
		if _, err := store.db.Exec(`
UPDATE continuity_sync_projects
SET downloaded_cursor = ?, applied_cursor = ?, relay_head = ?
WHERE project_id = ?`, int64(math.MaxInt64), int64(math.MaxInt64), int64(math.MaxInt64), string(projectID)); err != nil {
			t.Fatalf("seed exhausted maximum cursor: %v", err)
		}
		before := captureTerminalMutationStateV1(t, store, projectID)
		frames, err := store.PendingSyncFramesAfter(context.Background(), projectID, math.MaxInt64, 2)
		if err != nil {
			t.Fatalf("PendingSyncFramesAfter(maximum cursor) error = %v", err)
		}
		if len(frames) != 0 {
			t.Fatalf("PendingSyncFramesAfter(maximum cursor) frames = %d, want 0", len(frames))
		}
		assertTerminalMutationStateV1(t, store, projectID, before)
	})

	t.Run("final maximum arrival with larger page", func(t *testing.T) {
		store := openSyncStore(t, "terminal-pending-max-final")
		projectID := continuity.ProjectID("project-terminal-pending-max-final")
		installTestSyncAuthority(t, store, projectID, testSyncChannelID("channel-a"))
		frame := testOpaqueFrame(math.MaxInt64, "maximum-arrival")
		if _, err := store.db.Exec(`
UPDATE continuity_sync_projects
SET downloaded_cursor = ?, applied_cursor = ?, relay_head = ?
WHERE project_id = ?`, int64(math.MaxInt64), int64(math.MaxInt64-1), int64(math.MaxInt64), string(projectID)); err != nil {
			t.Fatalf("seed final maximum cursor: %v", err)
		}
		if _, err := store.db.Exec(`
INSERT INTO continuity_sync_inbox (
  project_id, arrival_sequence, envelope_digest, frame_kind, frame_bytes, state
) VALUES (?, ?, ?, 'sealed', ?, 'staged')`, string(projectID), frame.ArrivalSequence, frame.EnvelopeDigest[:], frame.SealedEnvelope); err != nil {
			t.Fatalf("seed final maximum arrival: %v", err)
		}
		before := captureTerminalMutationStateV1(t, store, projectID)
		frames, err := store.PendingSyncFramesAfter(context.Background(), projectID, math.MaxInt64-1, 2)
		if err != nil {
			t.Fatalf("PendingSyncFramesAfter(final maximum arrival) error = %v", err)
		}
		if len(frames) != 1 || !opaqueSyncFrameEqual(frames[0], frame) {
			t.Fatalf("PendingSyncFramesAfter(final maximum arrival) = %#v, want %#v", frames, []OpaqueSyncFrame{frame})
		}
		assertTerminalMutationStateV1(t, store, projectID, before)
	})
}

func retireSyncEnvironmentForGateV1(t *testing.T, store *Store, projectID continuity.ProjectID, environmentID continuity.EnvironmentID, finalSequence int64, finalDigest [32]byte) {
	t.Helper()
	authority, err := store.CurrentSyncAuthority(context.Background(), projectID)
	if err != nil {
		t.Fatalf("CurrentSyncAuthority() error = %v", err)
	}
	found := false
	for index := range authority.Environments {
		if authority.Environments[index].EnvironmentID != string(environmentID) {
			continue
		}
		found = true
		authority.MembershipGeneration++
		authority.Environments[index].Retirement = &SyncEnvironmentRetirement{
			RelayGeneration:          authority.RelayGeneration,
			MembershipGeneration:     authority.MembershipGeneration,
			FinalEnvironmentSequence: finalSequence,
			FinalEnvelopeDigest:      finalDigest,
			RetirementID:             sha256.Sum256([]byte("retirement:" + string(environmentID))),
			RetirementBytes:          []byte("retirement-bytes:" + string(environmentID)),
		}
		break
	}
	if !found {
		t.Fatalf("environment %q is not in test authority", environmentID)
	}
	if _, err := store.InstallVerifiedSyncAuthority(context.Background(), projectID, authority); err != nil {
		t.Fatalf("InstallVerifiedSyncAuthority(retire %q) error = %v", environmentID, err)
	}
}

type terminalMutationStateV1 struct {
	progress SyncProgress
	facts    int
	receipts int
	tombs    int
	inbox    int
	heads    int
	outbox   int
	rows     []string
}

func captureTerminalMutationStateV1(t *testing.T, store *Store, projectID continuity.ProjectID) terminalMutationStateV1 {
	t.Helper()
	state := terminalMutationStateV1{}
	var err error
	state.progress, err = store.CurrentSyncProgress(context.Background(), projectID)
	if err != nil {
		t.Fatalf("CurrentSyncProgress() error = %v", err)
	}
	queries := []struct {
		table string
		value *int
	}{
		{table: "continuity_facts", value: &state.facts},
		{table: "continuity_sync_receipts", value: &state.receipts},
		{table: "continuity_sync_tombstones", value: &state.tombs},
		{table: "continuity_sync_inbox", value: &state.inbox},
		{table: "continuity_sync_environment_heads", value: &state.heads},
		{table: "continuity_sync_outbox", value: &state.outbox},
	}
	for _, query := range queries {
		if err := store.db.QueryRow("SELECT COUNT(*) FROM "+query.table+" WHERE project_id = ?", string(projectID)).Scan(query.value); err != nil {
			t.Fatalf("count %s: %v", query.table, err)
		}
	}
	state.rows = terminalLogicalRowsV1(t, store, projectID)
	return state
}

func terminalLogicalRowsV1(t *testing.T, store *Store, projectID continuity.ProjectID) []string {
	t.Helper()
	queries := []struct {
		table string
		order string
	}{
		{table: "continuity_facts", order: "fact_id"},
		{table: "continuity_sync_projects", order: "project_id"},
		{table: "continuity_sync_inbox", order: "arrival_sequence"},
		{table: "continuity_sync_receipts", order: "arrival_sequence"},
		{table: "continuity_sync_environment_heads", order: "environment_id"},
		{table: "continuity_sync_outbox", order: "fact_id"},
		{table: "continuity_sync_tombstones", order: "arrival_sequence"},
		{table: "continuity_sync_environment_certificates", order: "environment_id"},
		{table: "continuity_sync_terminal_candidates", order: "candidate_id"},
		{table: "continuity_sync_terminal_candidate_frames", order: "candidate_id, arrival_sequence"},
	}
	result := make([]string, 0)
	for _, query := range queries {
		rows, err := store.db.Query("SELECT * FROM "+query.table+" WHERE project_id = ? ORDER BY "+query.order, string(projectID))
		if err != nil {
			t.Fatalf("read exact %s rows: %v", query.table, err)
		}
		columns, err := rows.Columns()
		if err != nil {
			rows.Close()
			t.Fatalf("read %s columns: %v", query.table, err)
		}
		for rows.Next() {
			values := make([]any, len(columns))
			pointers := make([]any, len(columns))
			for index := range values {
				pointers[index] = &values[index]
			}
			if err := rows.Scan(pointers...); err != nil {
				rows.Close()
				t.Fatalf("scan exact %s row: %v", query.table, err)
			}
			row := query.table
			for index, value := range values {
				switch value := value.(type) {
				case []byte:
					row += fmt.Sprintf("|%s=bytes:%x", columns[index], value)
				case string:
					row += fmt.Sprintf("|%s=string:%q", columns[index], value)
				case int64:
					row += fmt.Sprintf("|%s=int64:%d", columns[index], value)
				case nil:
					row += "|" + columns[index] + "=nil"
				default:
					rows.Close()
					t.Fatalf("unexpected %s.%s type %T", query.table, columns[index], value)
				}
			}
			result = append(result, row)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			t.Fatalf("iterate exact %s rows: %v", query.table, err)
		}
		if err := rows.Close(); err != nil {
			t.Fatalf("close exact %s rows: %v", query.table, err)
		}
	}
	return result
}

func assertTerminalMutationStateV1(t *testing.T, store *Store, projectID continuity.ProjectID, want terminalMutationStateV1) {
	t.Helper()
	if got := captureTerminalMutationStateV1(t, store, projectID); !reflect.DeepEqual(got, want) {
		t.Fatalf("terminal mutation state changed:\n got %#v\nwant %#v", got, want)
	}
}

func assertContentFreeSyncCodeV1(t *testing.T, err error, code SyncErrorCode) {
	t.Helper()
	assertSyncErrorCode(t, err, code)
	var problem *SyncError
	if !errors.As(err, &problem) {
		t.Fatalf("error = %v, want *SyncError", err)
	}
	if problem.Field != "" || problem.Detail != "" {
		t.Fatalf("error = %#v, want content-free machine class", problem)
	}
}
