package sqlite

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/levifig/loaf/vnext/continuity"
	"github.com/levifig/loaf/vnext/internal/continuitywire"
)

func TestWriterEnvironmentIDReturnsImmutableWriterIdentity(t *testing.T) {
	t.Parallel()

	var nilStore *Store
	if got := nilStore.WriterEnvironmentID(); got != "" {
		t.Fatalf("nil Store.WriterEnvironmentID() = %q, want zero value", got)
	}

	stateRoot := filepath.Join(testTempDir(t), "writer-environment-id")
	store, err := Open(stateRoot, "environment-writer")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if got := store.WriterEnvironmentID(); got != "environment-writer" {
		t.Fatalf("WriterEnvironmentID() = %q, want environment-writer", got)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if got := store.WriterEnvironmentID(); got != "environment-writer" {
		t.Fatalf("WriterEnvironmentID(after Close) = %q, want immutable identity", got)
	}
}

func TestCurrentSyncEnvironmentStatesReturnsOrderedDefensivePointState(t *testing.T) {
	t.Parallel()

	store := openSyncStore(t, "attach-environment-states")
	projectID := continuity.ProjectID("project-attach-environment-states")
	authority := testSyncAuthority()
	authority.MembershipGeneration = 4
	authority.Environments = append(authority.Environments, SyncEnvironmentCertificate{
		EnvironmentID:            "environment-c",
		CertificateID:            testAuthorityDigest(0x51),
		CertificateBytes:         []byte("environment-c-certificate"),
		Mode:                     SyncEnvironmentTrusted,
		JoinMembershipGeneration: 4,
	})
	if _, err := store.InstallVerifiedSyncAuthority(context.Background(), projectID, authority); err != nil {
		t.Fatalf("InstallVerifiedSyncAuthority() error = %v", err)
	}
	binding := currentSyncAuthorityBindingForTest(t, store, projectID)

	a1 := syncAuthorityPromotionMetadataV2("attach-a-1", [32]byte{}, authority.Environments[0].CertificateID)
	a3 := syncAuthorityPromotionMetadataV2("attach-a-3", a1.digest, authority.Environments[0].CertificateID)
	b4 := syncAuthorityPromotionMetadataV2("attach-b-4", [32]byte{}, authority.Environments[1].CertificateID)
	insertSyncAuthorityPromotionReceiptV2(t, store, projectID, 1, "fact-attach-a-1", "environment-a", 1, a1)
	insertSyncAuthorityPromotionTombstoneV2(t, store, projectID, 2, "fact-attach-a-3", "environment-a", 3, a3)
	insertSyncAuthorityPromotionReceiptV2(t, store, projectID, 3, "fact-attach-b-4", "environment-b", 4, b4)

	environmentIDs := []continuity.EnvironmentID{"environment-b", "environment-c", "environment-a"}
	states, err := store.CurrentSyncEnvironmentStates(context.Background(), projectID, binding, environmentIDs)
	if err != nil {
		t.Fatalf("CurrentSyncEnvironmentStates() error = %v", err)
	}
	if len(states) != 3 || states[0].Certificate.EnvironmentID != "environment-b" || states[1].Certificate.EnvironmentID != "environment-c" || states[2].Certificate.EnvironmentID != "environment-a" {
		t.Fatalf("CurrentSyncEnvironmentStates() order = %#v, want environment-b, environment-c, environment-a", states)
	}
	if states[0].ConsumedSequence != 4 || states[1].ConsumedSequence != 0 || states[2].ConsumedSequence != 3 {
		t.Fatalf("consumed sequences = %d,%d,%d, want 4,0,3", states[0].ConsumedSequence, states[1].ConsumedSequence, states[2].ConsumedSequence)
	}
	if !syncEnvironmentCertificateEqual(states[0].Certificate, authority.Environments[1]) ||
		!syncEnvironmentCertificateEqual(states[1].Certificate, authority.Environments[2]) ||
		!syncEnvironmentCertificateEqual(states[2].Certificate, authority.Environments[0]) {
		t.Fatalf("certificates = %#v, want exact requested authority certificates", states)
	}
	states[0].Certificate.CertificateBytes[0] ^= 0xff
	states[1].Certificate.CertificateBytes[0] ^= 0xff
	states[2].Certificate.CertificateBytes[0] ^= 0xff
	states[2].Certificate.Retirement.RetirementBytes[0] ^= 0xff

	reloaded, err := store.CurrentSyncEnvironmentStates(context.Background(), projectID, binding, environmentIDs)
	if err != nil {
		t.Fatalf("CurrentSyncEnvironmentStates(after caller mutation) error = %v", err)
	}
	if !syncEnvironmentCertificateEqual(reloaded[0].Certificate, authority.Environments[1]) ||
		!syncEnvironmentCertificateEqual(reloaded[1].Certificate, authority.Environments[2]) ||
		!syncEnvironmentCertificateEqual(reloaded[2].Certificate, authority.Environments[0]) {
		t.Fatal("CurrentSyncEnvironmentStates() exposed mutable certificate bytes")
	}
	if !reflect.DeepEqual(environmentIDs, []continuity.EnvironmentID{"environment-b", "environment-c", "environment-a"}) {
		t.Fatalf("CurrentSyncEnvironmentStates() changed caller IDs: %#v", environmentIDs)
	}

	_, err = store.CurrentSyncEnvironmentStates(context.Background(), projectID, binding, []continuity.EnvironmentID{"environment-unknown"})
	assertSyncErrorCode(t, err, SyncErrorCertificate)
	if strings.Contains(err.Error(), "environment-unknown") {
		t.Fatalf("unknown-environment error exposed caller identity: %v", err)
	}
	var receiptRows, tombstoneRows int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM continuity_sync_receipts WHERE project_id = ?`, string(projectID)).Scan(&receiptRows); err != nil {
		t.Fatalf("count retained receipts: %v", err)
	}
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM continuity_sync_tombstones WHERE project_id = ?`, string(projectID)).Scan(&tombstoneRows); err != nil {
		t.Fatalf("count retained tombstones: %v", err)
	}
	if receiptRows != 2 || tombstoneRows != 1 {
		t.Fatalf("bounded state reads changed protected rows: receipts=%d tombstones=%d, want 2,1", receiptRows, tombstoneRows)
	}
}

func TestCurrentSyncEnvironmentStatesValidatesBoundedDistinctInputs(t *testing.T) {
	t.Parallel()

	store := openSyncStore(t, "attach-environment-state-inputs")
	projectID := continuity.ProjectID("project-attach-environment-state-inputs")
	if _, err := store.InstallVerifiedSyncAuthority(context.Background(), projectID, testActiveSyncAuthority()); err != nil {
		t.Fatalf("InstallVerifiedSyncAuthority() error = %v", err)
	}
	binding := currentSyncAuthorityBindingForTest(t, store, projectID)

	for name, environmentIDs := range map[string][]continuity.EnvironmentID{
		"empty":     nil,
		"invalid":   {"invalid environment"},
		"duplicate": {"environment-a", "environment-a"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := store.CurrentSyncEnvironmentStates(context.Background(), projectID, binding, environmentIDs)
			assertSyncErrorCode(t, err, SyncErrorInvalid)
		})
	}
	tooMany := make([]continuity.EnvironmentID, maximumSyncAuthorityEnvironments+1)
	for index := range tooMany {
		tooMany[index] = continuity.EnvironmentID(fmt.Sprintf("environment-%03d", index))
	}
	_, err := store.CurrentSyncEnvironmentStates(context.Background(), projectID, binding, tooMany)
	assertSyncErrorCode(t, err, SyncErrorInvalid)

	invalidBinding := binding
	invalidBinding.AuthorityDigest = [32]byte{}
	_, err = store.CurrentSyncEnvironmentStates(context.Background(), projectID, invalidBinding, []continuity.EnvironmentID{"environment-a"})
	assertSyncErrorCode(t, err, SyncErrorInvalid)
	_, err = store.CurrentSyncEnvironmentStates(nil, projectID, binding, []continuity.EnvironmentID{"environment-a"})
	assertSyncErrorCode(t, err, SyncErrorInvalid)
	_, err = (*Store)(nil).CurrentSyncEnvironmentStates(context.Background(), projectID, binding, []continuity.EnvironmentID{"environment-a"})
	assertSyncErrorCode(t, err, SyncErrorStore)
	_, err = store.CurrentSyncEnvironmentStates(context.Background(), "invalid project", binding, []continuity.EnvironmentID{"environment-a"})
	assertSyncErrorCode(t, err, SyncErrorInvalid)
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = store.CurrentSyncEnvironmentStates(canceled, projectID, binding, []continuity.EnvironmentID{"environment-a"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("CurrentSyncEnvironmentStates(canceled) error = %v, want context.Canceled", err)
	}
}

func TestCurrentSyncEnvironmentStatesRequiresExactStableAuthoritySnapshot(t *testing.T) {
	t.Parallel()

	stateRoot := filepath.Join(testTempDir(t), "attach-environment-state-snapshot")
	store, err := Open(stateRoot, "environment-local")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { store.Close() })
	projectID := continuity.ProjectID("project-attach-environment-state-snapshot")
	authority := testActiveSyncAuthority()
	if _, err := store.InstallVerifiedSyncAuthority(context.Background(), projectID, authority); err != nil {
		t.Fatalf("InstallVerifiedSyncAuthority() error = %v", err)
	}
	binding := currentSyncAuthorityBindingForTest(t, store, projectID)
	drifted := binding
	drifted.AuthorityDigest[0] ^= 0xff
	_, err = store.CurrentSyncEnvironmentStates(context.Background(), projectID, drifted, []continuity.EnvironmentID{"environment-a"})
	assertSyncErrorCode(t, err, SyncErrorConflict)

	ready := stageSyncAuthorityGuardCandidateV2(t, store, projectID, authority, true)
	_, err = store.CurrentSyncEnvironmentStates(context.Background(), projectID, binding, []continuity.EnvironmentID{"environment-a"})
	assertSyncErrorCode(t, err, SyncErrorConflict)

	concurrentStore, err := Open(stateRoot, "environment-local")
	if err != nil {
		t.Fatalf("Open(concurrent) error = %v", err)
	}
	t.Cleanup(func() { concurrentStore.Close() })
	start := make(chan struct{})
	promotionResult := make(chan error, 1)
	readResult := make(chan error, 1)
	go func() {
		<-start
		_, promoteErr := store.PromoteSyncAuthorityCandidate(context.Background(), projectID, ready.Checkpoint())
		promotionResult <- promoteErr
	}()
	go func() {
		<-start
		_, readErr := concurrentStore.CurrentSyncEnvironmentStates(context.Background(), projectID, binding, []continuity.EnvironmentID{"environment-a"})
		readResult <- readErr
	}()
	close(start)
	if err := <-promotionResult; err != nil {
		t.Fatalf("PromoteSyncAuthorityCandidate() error = %v", err)
	}
	assertSyncErrorCode(t, <-readResult, SyncErrorConflict)
	promotedBinding := currentSyncAuthorityBindingForTest(t, store, projectID)
	states, err := store.CurrentSyncEnvironmentStates(context.Background(), projectID, promotedBinding, []continuity.EnvironmentID{"environment-a"})
	if err != nil || len(states) != 1 {
		t.Fatalf("CurrentSyncEnvironmentStates(promoted receipt) = (%#v, %v), want one state", states, err)
	}

	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	_, err = store.CurrentSyncEnvironmentStates(context.Background(), projectID, promotedBinding, []continuity.EnvironmentID{"environment-a"})
	assertSyncErrorCode(t, err, SyncErrorStore)
	reopened, err := Open(stateRoot, "environment-local")
	if err != nil {
		t.Fatalf("Open(reopen) error = %v", err)
	}
	t.Cleanup(func() { reopened.Close() })
	states, err = reopened.CurrentSyncEnvironmentStates(context.Background(), projectID, promotedBinding, []continuity.EnvironmentID{"environment-b", "environment-a"})
	if err != nil || len(states) != 2 {
		t.Fatalf("CurrentSyncEnvironmentStates(reopen) = (%#v, %v), want two states", states, err)
	}
}

func TestCurrentSyncEnvironmentStateQueriesUseBoundedIndexes(t *testing.T) {
	t.Parallel()

	store := openSyncStore(t, "attach-environment-state-query-plan")
	for name, query := range map[string]string{
		"receipt":   syncEnvironmentReceiptConsumedSequenceQueryV2,
		"tombstone": syncEnvironmentTombstoneConsumedSequenceQueryV2,
	} {
		t.Run(name, func(t *testing.T) {
			rows, err := store.db.Query("EXPLAIN QUERY PLAN "+query, "project-query-plan", "environment-query-plan")
			if err != nil {
				t.Fatalf("EXPLAIN QUERY PLAN: %v", err)
			}
			defer rows.Close()
			var details []string
			for rows.Next() {
				var id, parent, notUsed int
				var detail string
				if err := rows.Scan(&id, &parent, &notUsed, &detail); err != nil {
					t.Fatalf("scan query plan: %v", err)
				}
				details = append(details, detail)
			}
			if err := rows.Err(); err != nil {
				t.Fatalf("read query plan: %v", err)
			}
			joined := strings.Join(details, "\n")
			if !strings.Contains(joined, "SEARCH continuity_sync_") ||
				!strings.Contains(joined, "project_id=? AND environment_id=?") ||
				strings.Contains(joined, "USE TEMP B-TREE") {
				t.Fatalf("query plan = %q, want bounded composite-index search without temp sorting", joined)
			}
		})
	}
}

func TestActivateStagedSyncRequiresExactAuthorityBindingAndCutoff(t *testing.T) {
	t.Parallel()

	store := openSyncStore(t, "activation-exact-authority-cutoff")
	projectID := continuity.ProjectID("project-activation-exact-authority-cutoff")
	root := syncProjectFact(t, projectID, "fact-project", "environment-a", 1, 100)
	verified := stageSyncFacts(t, store, projectID, 1, []continuitywire.Fact{root})
	staleBinding := currentSyncAuthorityBindingForTest(t, store, projectID)
	if _, err := store.ApplySyncBatch(context.Background(), projectID, staleBinding, verified, 1_000, 100); err != nil {
		t.Fatalf("ApplySyncBatch() error = %v", err)
	}

	before, err := store.CurrentSyncProgress(context.Background(), projectID)
	if err != nil {
		t.Fatalf("CurrentSyncProgress(before) error = %v", err)
	}
	_, err = store.ActivateStagedSync(context.Background(), projectID, staleBinding)
	assertSyncErrorCode(t, err, SyncErrorConflict)
	after, err := store.CurrentSyncProgress(context.Background(), projectID)
	if err != nil || after != before {
		t.Fatalf("progress after stale-cutoff activation = (%#v, %v), want unchanged %#v", after, err, before)
	}

	attachedBinding := promoteSyncAuthorityArrivalHeadForTest(t, store, projectID, 1)
	drifted := attachedBinding
	drifted.AuthorityDigest[0] ^= 0xff
	_, err = store.ActivateStagedSync(context.Background(), projectID, drifted)
	assertSyncErrorCode(t, err, SyncErrorConflict)
	progress, err := store.ActivateStagedSync(context.Background(), projectID, attachedBinding)
	if err != nil || progress.ActivationState != SyncActivationAttached {
		t.Fatalf("ActivateStagedSync(exact authority) = (%#v, %v), want attached", progress, err)
	}
}

func TestActivateStagedSyncExactRetryIgnoresLaterProtectedHistoryDamage(t *testing.T) {
	t.Parallel()

	store, projectID := storeWithLocalRoot(t, "activation-exact-retry-damage")
	authority := testActiveSyncAuthority()
	if _, err := store.InstallVerifiedSyncAuthority(context.Background(), projectID, authority); err != nil {
		t.Fatalf("InstallVerifiedSyncAuthority() error = %v", err)
	}
	binding := currentSyncAuthorityBindingForTest(t, store, projectID)
	want, err := store.ActivateStagedSync(context.Background(), projectID, binding)
	if err != nil {
		t.Fatalf("ActivateStagedSync(initial) error = %v", err)
	}
	if _, err := store.db.Exec(`UPDATE continuity_facts SET content_json = '{}' WHERE project_id = ?`, string(projectID)); err != nil {
		t.Fatalf("corrupt protected fact: %v", err)
	}
	if _, err := store.db.Exec(`
UPDATE continuity_sync_projects
SET downloaded_cursor = 1, applied_cursor = 1, relay_head = 1
WHERE project_id = ?`, string(projectID)); err != nil {
		t.Fatalf("advance attached progress: %v", err)
	}
	got, err := store.ActivateStagedSync(context.Background(), projectID, binding)
	if err != nil || got.ActivationState != want.ActivationState || got.AppliedCursor != 1 {
		t.Fatalf("ActivateStagedSync(exact retry after damage) = (%#v, %v), want attached no-op", got, err)
	}
	var retainedContent string
	if err := store.db.QueryRow(`SELECT content_json FROM continuity_facts WHERE project_id = ?`, string(projectID)).Scan(&retainedContent); err != nil {
		t.Fatalf("read damaged protected fact: %v", err)
	}
	if retainedContent != "{}" {
		t.Fatalf("exact activation retry rewrote protected fact to %q", retainedContent)
	}
	drifted := binding
	drifted.AuthorityDigest[0] ^= 0xff
	_, err = store.ActivateStagedSync(context.Background(), projectID, drifted)
	assertSyncErrorCode(t, err, SyncErrorConflict)
}
