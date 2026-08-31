package coordinator

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/levifig/loaf/vnext/continuity"
	continuitysqlite "github.com/levifig/loaf/vnext/continuity/sqlite"
	"github.com/levifig/loaf/vnext/sync/relay"
)

func TestScanRecoveryPruneInventoryDigestResumesToExactFullCommitment(t *testing.T) {
	fixture := newRecoveryDownloadFixture(t, 12)
	records := make([]relay.PruneInventoryRecord, 5)
	for index := range records {
		records[index] = testRecoveryPruneInventoryRecord(t, fixture, int64(index+1), byte(0x91+index))
	}
	snapshot := relay.PruneInventorySnapshot{
		MembershipGeneration: fixture.binding.MembershipGeneration,
		ArrivalHead:          fixture.binding.InventoryArrivalHead,
		PruneHead:            int64(len(records)),
	}
	fixture.remote.prune = recoveryPruneInventoryPages(fixture, snapshot, records)

	var fullPages []verifiedRecoveryPruneInventoryPage
	full, err := fixture.coordinator.scanRecoveryPruneInventory(
		context.Background(), fixture.recovery.ProjectID, fixture.prepared, fixture.binding,
		recoveryPruneInventoryScanOptions{onPage: func(page verifiedRecoveryPruneInventoryPage) error {
			fullPages = append(fullPages, cloneVerifiedRecoveryPruneInventoryPage(page))
			return nil
		}},
	)
	if err != nil {
		t.Fatalf("full prune inventory scan: %v", err)
	}
	if full.inventoryDigest == (recoveryPruneInventoryDigest{}) || len(fullPages) != 2 ||
		fullPages[0].checkpoint.rollingDigest == (recoveryPruneInventoryRollingDigest{}) ||
		fullPages[0].inventoryDigest != (recoveryPruneInventoryDigest{}) ||
		fullPages[1].inventoryDigest != full.inventoryDigest || fullPages[1].checkpoint != full.checkpoint {
		t.Fatalf("full digest checkpoints = {result=%v pages=%v}, want cumulative prefix and terminal final", full, fullPages)
	}
	if _, err := finalizeRecoveryPruneInventoryDigestV1(fullPages[0].checkpoint); err == nil {
		t.Fatal("nonterminal checkpoint finalized before the pinned prune head")
	}

	fixture.remote.pruneRequests = nil
	fixture.remote.prune = recoveryPruneInventoryPages(fixture, snapshot, records)
	resumeCheckpoint := fullPages[0].checkpoint
	var resumedPage verifiedRecoveryPruneInventoryPage
	resumed, err := fixture.coordinator.scanRecoveryPruneInventory(
		context.Background(), fixture.recovery.ProjectID, fixture.prepared, fixture.binding,
		recoveryPruneInventoryScanOptions{
			firstCheckpoint: &resumeCheckpoint,
			onPage: func(page verifiedRecoveryPruneInventoryPage) error {
				resumedPage = cloneVerifiedRecoveryPruneInventoryPage(page)
				return nil
			},
		},
	)
	if err != nil {
		t.Fatalf("resumed prune inventory scan: %v", err)
	}
	if resumed != full || resumedPage.afterPruneSequence != relay.MaxPruneInventoryPage ||
		resumedPage.inventoryDigest != full.inventoryDigest || len(fixture.remote.pruneRequests) != 1 {
		t.Fatalf("resumed result = %#v page=%#v requests=%d, want exact full-scan result %#v", resumed, resumedPage, len(fixture.remote.pruneRequests), full)
	}

	fixture.remote.pruneRequests = nil
	invalidCheckpoint := resumeCheckpoint
	invalidCheckpoint.headerDigest[0] ^= 0xff
	_, err = fixture.coordinator.scanRecoveryPruneInventory(
		context.Background(), fixture.recovery.ProjectID, fixture.prepared, fixture.binding,
		recoveryPruneInventoryScanOptions{firstCheckpoint: &invalidCheckpoint},
	)
	assertProblem(t, err, CodeInvalid, PhasePruneInventory, ActionRestartRecovery)
	if len(fixture.remote.pruneRequests) != 0 {
		t.Fatal("mismatched resume checkpoint reached the relay")
	}

	invalidCheckpoint = resumeCheckpoint
	invalidCheckpoint.rollingDigest = recoveryPruneInventoryRollingDigest{}
	_, err = fixture.coordinator.scanRecoveryPruneInventory(
		context.Background(), fixture.recovery.ProjectID, fixture.prepared, fixture.binding,
		recoveryPruneInventoryScanOptions{firstCheckpoint: &invalidCheckpoint},
	)
	assertProblem(t, err, CodeInvalid, PhasePruneInventory, ActionRestartRecovery)
	if len(fixture.remote.pruneRequests) != 0 {
		t.Fatal("zero rolling checkpoint reached the relay")
	}
}

func TestRecoveryPruneInventoryDigestCommitsVerifiedProjectionAndRejectsNoncanonicalOrder(t *testing.T) {
	fixture := newRecoveryDownloadFixture(t, 4)
	record := testRecoveryPruneInventoryRecord(t, fixture, 1, 0xa1)
	snapshot := relay.PruneInventorySnapshot{
		MembershipGeneration: fixture.binding.MembershipGeneration,
		ArrivalHead:          fixture.binding.InventoryArrivalHead,
		PruneHead:            1,
	}
	fixture.remote.prune = recoveryPruneInventoryPages(fixture, snapshot, []relay.PruneInventoryRecord{record})
	var verified verifiedRecoveryPrune
	_, err := fixture.coordinator.scanRecoveryPruneInventory(
		context.Background(), fixture.recovery.ProjectID, fixture.prepared, fixture.binding,
		recoveryPruneInventoryScanOptions{onPage: func(page verifiedRecoveryPruneInventoryPage) error {
			verified = cloneVerifiedRecoveryPrunes(page.prunes)[0]
			return nil
		}},
	)
	if err != nil {
		t.Fatalf("scan fixture inventory: %v", err)
	}

	baselineRecord, err := recoveryPruneRecordDigestV1(verified)
	if err != nil {
		t.Fatalf("digest verified prune: %v", err)
	}
	recordMutations := []struct {
		name   string
		mutate func(*verifiedRecoveryPrune)
	}{
		{name: "prune sequence", mutate: func(value *verifiedRecoveryPrune) { value.pruneSequence++ }},
		{name: "prune id", mutate: func(value *verifiedRecoveryPrune) { value.pruneID[0] ^= 0xff }},
		{name: "certificate id", mutate: func(value *verifiedRecoveryPrune) { value.pruneCertificateID[0] ^= 0xff }},
		{name: "membership generation", mutate: func(value *verifiedRecoveryPrune) { value.membershipGeneration++ }},
		{name: "barrier", mutate: func(value *verifiedRecoveryPrune) { value.barrierArrivalSequence++ }},
		{name: "scratchpad subject", mutate: func(value *verifiedRecoveryPrune) { value.scratchpadSubject = "scratchpad-mutated" }},
	}
	for _, test := range recordMutations {
		t.Run(test.name, func(t *testing.T) {
			mutated := cloneVerifiedRecoveryPrunes([]verifiedRecoveryPrune{verified})[0]
			test.mutate(&mutated)
			got, digestErr := recoveryPruneRecordDigestV1(mutated)
			if digestErr != nil {
				t.Fatalf("digest protocol-valid mutation: %v", digestErr)
			}
			if got == baselineRecord {
				t.Fatalf("mutating %s did not change the record commitment", test.name)
			}
		})
	}

	assertReferenceFieldsChangeDigest := func(t *testing.T, reference continuitysqlite.VerifiedPruneReference) {
		t.Helper()
		if reference.EnvironmentSequence == 1 {
			reference.EnvironmentSequence = 3
			reference.PreviousEnvelopeDigest[0] = 0x7f
		}
		baseline, digestErr := recoveryPruneReferenceDigestV1(reference)
		if digestErr != nil {
			t.Fatalf("digest baseline reference: %v", digestErr)
		}
		mutations := []struct {
			name   string
			mutate func(*continuitysqlite.VerifiedPruneReference)
		}{
			{name: "fact id", mutate: func(value *continuitysqlite.VerifiedPruneReference) { value.FactID = "fact-mutated" }},
			{name: "environment id", mutate: func(value *continuitysqlite.VerifiedPruneReference) { value.EnvironmentID = "environment-mutated" }},
			{name: "environment sequence", mutate: func(value *continuitysqlite.VerifiedPruneReference) { value.EnvironmentSequence++ }},
			{name: "arrival sequence", mutate: func(value *continuitysqlite.VerifiedPruneReference) { value.ArrivalSequence++ }},
			{name: "envelope digest", mutate: func(value *continuitysqlite.VerifiedPruneReference) { value.EnvelopeDigest[0] ^= 0xff }},
			{name: "certificate id", mutate: func(value *continuitysqlite.VerifiedPruneReference) { value.CertificateID[0] ^= 0xff }},
			{name: "previous digest", mutate: func(value *continuitysqlite.VerifiedPruneReference) { value.PreviousEnvelopeDigest[0] ^= 0xff }},
			{name: "key generation", mutate: func(value *continuitysqlite.VerifiedPruneReference) { value.KeyGeneration++ }},
			{name: "nonce", mutate: func(value *continuitysqlite.VerifiedPruneReference) { value.Nonce[0] ^= 0xff }},
		}
		for _, test := range mutations {
			mutated := reference
			test.mutate(&mutated)
			got, mutationErr := recoveryPruneReferenceDigestV1(mutated)
			if mutationErr != nil {
				t.Fatalf("digest %s reference mutation: %v", test.name, mutationErr)
			}
			if got == baseline {
				t.Fatalf("mutating reference %s did not change its canonical digest", test.name)
			}
		}
	}
	t.Run("closure reference", func(t *testing.T) { assertReferenceFieldsChangeDigest(t, verified.closure) })
	t.Run("target reference", func(t *testing.T) { assertReferenceFieldsChangeDigest(t, verified.targets[0].reference) })

	baselineTarget, err := recoveryPruneTargetDigestV1(verified, 1, verified.targets[0])
	if err != nil {
		t.Fatalf("digest baseline target: %v", err)
	}
	for _, test := range []struct {
		name   string
		mutate func(*verifiedRecoveryPruneTarget)
	}{
		{name: "fact kind", mutate: func(value *verifiedRecoveryPruneTarget) { value.factKind = continuity.FactScratchpadClaimRecorded }},
		{name: "wall clock", mutate: func(value *verifiedRecoveryPruneTarget) { value.hlc.WallMillis++ }},
		{name: "logical clock", mutate: func(value *verifiedRecoveryPruneTarget) { value.hlc.Logical++ }},
	} {
		t.Run("target "+test.name, func(t *testing.T) {
			mutated := verified.targets[0]
			test.mutate(&mutated)
			got, digestErr := recoveryPruneTargetDigestV1(verified, 1, mutated)
			if digestErr != nil || got == baselineTarget {
				t.Fatalf("target mutation %s = {%x %v}, want distinct valid digest", test.name, got, digestErr)
			}
		})
	}

	ordered := cloneVerifiedRecoveryPrunes([]verifiedRecoveryPrune{verified})[0]
	ordered.closure.EnvironmentSequence = 4
	ordered.closure.ArrivalSequence = 4
	ordered.barrierArrivalSequence = 4
	second := ordered.targets[0]
	second.reference.FactID = "fact-target-second"
	second.reference.EnvironmentSequence = 3
	second.reference.ArrivalSequence = 3
	second.reference.PreviousEnvelopeDigest[0] = 0x6f
	second.reference.EnvelopeDigest[0] ^= 0x55
	second.reference.Nonce[0] ^= 0x55
	ordered.targets = append(ordered.targets, second)
	if _, err := recoveryPruneRecordDigestV1(ordered); err != nil {
		t.Fatalf("digest canonical two-target projection: %v", err)
	}
	swapped := cloneVerifiedRecoveryPrunes([]verifiedRecoveryPrune{ordered})[0]
	swapped.targets[0], swapped.targets[1] = swapped.targets[1], swapped.targets[0]
	if _, err := recoveryPruneRecordDigestV1(swapped); err == nil {
		t.Fatal("noncanonical target reorder was hashed")
	}
}

func TestRecoveryPruneInventoryCheckpointRejectsGapsRollbackAndOverflowWithoutMutation(t *testing.T) {
	fixture := newRecoveryDownloadFixture(t, 4)
	record := testRecoveryPruneInventoryRecord(t, fixture, 1, 0xb1)
	snapshot := relay.PruneInventorySnapshot{
		MembershipGeneration: fixture.binding.MembershipGeneration,
		ArrivalHead:          fixture.binding.InventoryArrivalHead,
		PruneHead:            1,
	}
	fixture.remote.prune = recoveryPruneInventoryPages(fixture, snapshot, []relay.PruneInventoryRecord{record})
	var verified verifiedRecoveryPrune
	_, err := fixture.coordinator.scanRecoveryPruneInventory(
		context.Background(), fixture.recovery.ProjectID, fixture.prepared, fixture.binding,
		recoveryPruneInventoryScanOptions{onPage: func(page verifiedRecoveryPruneInventoryPage) error {
			verified = cloneVerifiedRecoveryPrunes(page.prunes)[0]
			return nil
		}},
	)
	if err != nil {
		t.Fatalf("scan fixture inventory: %v", err)
	}
	checkpoint, err := newRecoveryPruneInventoryCheckpointV1(fixture.recovery.ProjectID, fixture.binding, snapshot)
	if err != nil {
		t.Fatalf("create checkpoint: %v", err)
	}
	for _, test := range []struct {
		name   string
		state  recoveryPruneInventoryCheckpoint
		mutate func(*verifiedRecoveryPrune)
	}{
		{name: "sequence gap", state: checkpoint, mutate: func(value *verifiedRecoveryPrune) { value.pruneSequence = 2 }},
		{name: "membership beyond snapshot", state: checkpoint, mutate: func(value *verifiedRecoveryPrune) { value.membershipGeneration = snapshot.MembershipGeneration + 1 }},
		{name: "barrier beyond snapshot", state: checkpoint, mutate: func(value *verifiedRecoveryPrune) { value.barrierArrivalSequence = snapshot.ArrivalHead + 1 }},
		{name: "cursor overflow", state: recoveryPruneInventoryCheckpoint{
			snapshot:     relay.PruneInventorySnapshot{PruneHead: int64(^uint64(0) >> 1)},
			headerDigest: recoveryPruneInventoryHeaderDigest{1}, throughPruneSequence: int64(^uint64(0) >> 1),
			lastMembershipGeneration: 1, rollingDigest: recoveryPruneInventoryRollingDigest{1},
		}, mutate: func(*verifiedRecoveryPrune) {}},
	} {
		t.Run(test.name, func(t *testing.T) {
			before := test.state
			mutated := cloneVerifiedRecoveryPrunes([]verifiedRecoveryPrune{verified})[0]
			test.mutate(&mutated)
			if _, advanceErr := advanceRecoveryPruneInventoryCheckpointV1(test.state, mutated); advanceErr == nil {
				t.Fatal("invalid checkpoint advance succeeded")
			}
			if test.state != before {
				t.Fatal("failed checkpoint advance mutated its input")
			}
		})
	}
}

func TestRecoveryPruneInventoryDigestBindsAuthoritySnapshotAndEmptyInventory(t *testing.T) {
	fixture := newRecoveryDownloadFixture(t, 0)
	snapshot := relay.PruneInventorySnapshot{MembershipGeneration: fixture.binding.MembershipGeneration}
	fixture.remote.prune = recoveryPruneInventoryPages(fixture, snapshot, nil)
	result, err := fixture.coordinator.scanRecoveryPruneInventory(
		context.Background(), fixture.recovery.ProjectID, fixture.prepared, fixture.binding,
		recoveryPruneInventoryScanOptions{},
	)
	if err != nil {
		t.Fatalf("scan empty prune inventory: %v", err)
	}
	if result.inventoryDigest == (recoveryPruneInventoryDigest{}) ||
		[32]byte(result.inventoryDigest) == [32]byte(result.checkpoint.rollingDigest) {
		t.Fatal("empty inventory did not finalize to a distinct nonzero commitment")
	}
	for _, formatted := range []string{
		fmt.Sprint(result.inventoryDigest), fmt.Sprintf("%#v", result.inventoryDigest),
		fmt.Sprint(result.checkpoint), fmt.Sprintf("%#v", result.checkpoint),
		fmt.Sprint(result), fmt.Sprintf("%#v", result),
	} {
		if !strings.Contains(formatted, "REDACTED") {
			t.Fatalf("inventory commitment formatting is not redacted: %q", formatted)
		}
	}

	baseline := result.inventoryDigest
	mutatedBinding := fixture.binding
	mutatedBinding.ChannelID[0] ^= 0xff
	checkpoint, err := newRecoveryPruneInventoryCheckpointV1(fixture.recovery.ProjectID, mutatedBinding, snapshot)
	if err != nil {
		t.Fatalf("create changed-authority checkpoint: %v", err)
	}
	changed, err := finalizeRecoveryPruneInventoryDigestV1(checkpoint)
	if err != nil || changed == baseline {
		t.Fatalf("changed authority final = {%v %v}, want distinct commitment", changed, err)
	}

	mutatedBinding = fixture.binding
	mutatedBinding.AuthorityDigest[0] ^= 0xff
	checkpoint, err = newRecoveryPruneInventoryCheckpointV1(fixture.recovery.ProjectID, mutatedBinding, snapshot)
	if err != nil {
		t.Fatalf("create changed-authority-digest checkpoint: %v", err)
	}
	changed, err = finalizeRecoveryPruneInventoryDigestV1(checkpoint)
	if err != nil || changed == baseline {
		t.Fatalf("changed authority digest final = {%v %v}, want distinct commitment", changed, err)
	}
}

func TestRecoveryPruneInventoryTranscriptKnownAnswerV1(t *testing.T) {
	digest, err := recoveryPruneInventoryHashV1("loaf.test.recovery-prune.v1", []byte("alpha"), []byte{0x00, 0x01, 0x02})
	if err != nil {
		t.Fatalf("hash known-answer transcript: %v", err)
	}
	const want = "48ecd084a971cae487a08d826dcc4e156fe6b0e7e063a801de24f07400d539f5"
	if got := fmt.Sprintf("%x", digest); got != want {
		t.Fatalf("known-answer digest = %s, want %s", got, want)
	}
}

func testRecoveryPruneInventoryCheckpoint(
	t *testing.T,
	fixture recoveryDownloadFixture,
	snapshot relay.PruneInventorySnapshot,
	through int64,
	lastMembershipGeneration uint32,
	rollingSeed byte,
) *recoveryPruneInventoryCheckpoint {
	t.Helper()
	header, err := recoveryPruneInventoryHeaderDigestV1(fixture.recovery.ProjectID, fixture.binding, snapshot)
	if err != nil {
		t.Fatalf("derive test recovery prune header: %v", err)
	}
	checkpoint := recoveryPruneInventoryCheckpoint{
		snapshot:                 snapshot,
		headerDigest:             header,
		throughPruneSequence:     through,
		lastMembershipGeneration: lastMembershipGeneration,
		rollingDigest:            recoveryPruneInventoryRollingDigest{rollingSeed},
	}
	if through == 0 {
		checkpoint.rollingDigest, err = recoveryPruneInventoryRollingSeedV1(header)
		if err != nil {
			t.Fatalf("derive test recovery prune rolling seed: %v", err)
		}
	}
	return &checkpoint
}
