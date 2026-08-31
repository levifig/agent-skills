package coordinator

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/levifig/loaf/vnext/continuity"
	continuitysqlite "github.com/levifig/loaf/vnext/continuity/sqlite"
	"github.com/levifig/loaf/vnext/sync/credential"
	"github.com/levifig/loaf/vnext/sync/crypto"
	"github.com/levifig/loaf/vnext/sync/protocol"
	"github.com/levifig/loaf/vnext/sync/relay"
)

func TestScanRecoveryPruneInventoryVerifiesAndOpensBoundedPages(t *testing.T) {
	fixture := newRecoveryDownloadFixture(t, 12)
	records := make([]relay.PruneInventoryRecord, 5)
	for index := range records {
		records[index] = testRecoveryPruneInventoryRecord(t, fixture, int64(index+1), byte(0x30+index))
	}
	snapshot := relay.PruneInventorySnapshot{
		MembershipGeneration: fixture.binding.MembershipGeneration,
		ArrivalHead:          fixture.binding.InventoryArrivalHead,
		PruneHead:            int64(len(records)),
	}
	fixture.remote.prune = recoveryPruneInventoryPages(fixture, snapshot, records)

	var pages []verifiedRecoveryPruneInventoryPage
	result, err := fixture.coordinator.scanRecoveryPruneInventory(
		context.Background(),
		fixture.recovery.ProjectID,
		fixture.prepared,
		fixture.binding,
		recoveryPruneInventoryScanOptions{
			onPage: func(page verifiedRecoveryPruneInventoryPage) error {
				pages = append(pages, cloneVerifiedRecoveryPruneInventoryPage(page))
				return nil
			},
		},
	)
	if err != nil {
		t.Fatalf("scan recovery prune inventory: %v", err)
	}
	if result.snapshot != snapshot || len(pages) != 2 || len(fixture.remote.pruneRequests) != 2 {
		t.Fatalf("scan result = {snapshot=%#v pages=%d requests=%d}, want {%#v 2 2}", result.snapshot, len(pages), len(fixture.remote.pruneRequests), snapshot)
	}
	if pages[0].afterPruneSequence != 0 || len(pages[0].prunes) != relay.MaxPruneInventoryPage || !pages[0].more ||
		pages[1].afterPruneSequence != relay.MaxPruneInventoryPage || len(pages[1].prunes) != 1 || pages[1].more {
		t.Fatalf("verified pages = %#v, want bounded 4+1 page stream", pages)
	}

	for index, request := range fixture.remote.pruneRequests {
		if request.Limit != relay.MaxPruneInventoryPage || request.Authorization.Owner != nil || request.Authorization.Environment == nil {
			t.Fatalf("request %d = %#v, want environment-authorized bounded request", index, request)
		}
		wantAuthorization := recoveryDownloadAuthorization(fixture.prepared)
		if *request.Authorization.Environment != wantAuthorization {
			t.Fatalf("request %d authorization differs from the prepared credential", index)
		}
		if index == 0 {
			if request.After != 0 || request.Snapshot != nil {
				t.Fatalf("first request = %#v, want unpinned cursor zero", request)
			}
		} else if request.After != relay.MaxPruneInventoryPage || request.Snapshot == nil || *request.Snapshot != snapshot {
			t.Fatalf("second request = %#v, want exact pinned continuation", request)
		}
	}

	first := pages[0].prunes[0]
	wantCertificate, err := protocol.ParsePruneCertificate(records[0].Certificate.CertificateBytes)
	if err != nil {
		t.Fatalf("parse fixture prune certificate: %v", err)
	}
	if first.pruneSequence != 1 || first.pruneID != recoveryPruneID(wantCertificate.PruneID) ||
		first.pruneCertificateID != recoveryPruneCertificateID(protocol.PruneCertificateID(wantCertificate)) ||
		first.pruneID == recoveryPruneID(first.pruneCertificateID) ||
		first.membershipGeneration != wantCertificate.MembershipGeneration ||
		first.barrierArrivalSequence != wantCertificate.BarrierArrivalSequence ||
		first.closure != verifiedRecoveryPruneReference(wantCertificate.Closure) ||
		first.scratchpadSubject != "scratchpad-1" || len(first.targets) != 1 ||
		first.targets[0].reference != verifiedRecoveryPruneReference(wantCertificate.Manifest.Targets[0]) ||
		first.targets[0].factKind != continuity.FactScratchpadMessageRecorded ||
		first.targets[0].hlc != (continuity.HybridTime{WallMillis: 101, Logical: 1}) {
		t.Fatalf("verified prune = %#v, want exact authenticated certificate and capsule projection", first)
	}
	for _, formatted := range []string{fmt.Sprint(first), fmt.Sprintf("%#v", first), fmt.Sprint(pages[0]), fmt.Sprintf("%#v", pages[0])} {
		root := fixture.prepared.ProjectRoot.Bytes()
		bearer := fixture.prepared.EnvironmentRelayAuthorization.Secret()
		if strings.Contains(formatted, "scratchpad-1") || strings.Contains(formatted, "fact-prune-target-1") ||
			strings.Contains(formatted, hex.EncodeToString(root[:])) || strings.Contains(formatted, hex.EncodeToString(bearer[:])) {
			t.Fatalf("verified prune formatting exposed authenticated metadata: %q", formatted)
		}
	}
}

func TestVerifiedRecoveryPruneProjectionExcludesCredentialsAndRawCryptographicObjects(t *testing.T) {
	wantPruneFields := []struct {
		name   string
		typeOf reflect.Type
	}{
		{name: "pruneSequence", typeOf: reflect.TypeOf(int64(0))},
		{name: "pruneID", typeOf: reflect.TypeOf(recoveryPruneID{})},
		{name: "pruneCertificateID", typeOf: reflect.TypeOf(recoveryPruneCertificateID{})},
		{name: "membershipGeneration", typeOf: reflect.TypeOf(uint32(0))},
		{name: "barrierArrivalSequence", typeOf: reflect.TypeOf(int64(0))},
		{name: "closure", typeOf: reflect.TypeOf(continuitysqlite.VerifiedPruneReference{})},
		{name: "scratchpadSubject", typeOf: reflect.TypeOf(continuity.SubjectID(""))},
		{name: "targets", typeOf: reflect.TypeOf([]verifiedRecoveryPruneTarget(nil))},
	}
	pruneType := reflect.TypeOf(verifiedRecoveryPrune{})
	if pruneType.NumField() != len(wantPruneFields) {
		t.Fatalf("verified prune fields = %d, want exact minimal schema of %d", pruneType.NumField(), len(wantPruneFields))
	}
	for index, want := range wantPruneFields {
		field := pruneType.Field(index)
		if field.Name != want.name || field.Type != want.typeOf {
			t.Fatalf("verified prune field %d = {%q %v}, want {%q %v}", index, field.Name, field.Type, want.name, want.typeOf)
		}
	}
	forbidden := []reflect.Type{
		reflect.TypeOf(protocol.PruneCertificate{}),
		reflect.TypeOf(protocol.PruneBootstrap{}),
		reflect.TypeOf(protocol.PruneBootstrapPlaintext{}),
		reflect.TypeOf(crypto.PruneBootstrapKey{}),
		reflect.TypeOf(relay.PruneInventoryRecord{}),
		reflect.TypeOf(relay.EnvironmentAuthorization{}),
	}
	for _, forbiddenType := range forbidden {
		for index := 0; index < pruneType.NumField(); index++ {
			if pruneType.Field(index).Type == forbiddenType {
				t.Fatalf("verified prune retains forbidden type %v", forbiddenType)
			}
		}
	}
}

func TestScanRecoveryPruneInventoryResumesPinnedSuffixAndPropagatesCallbackFailure(t *testing.T) {
	fixture := newRecoveryDownloadFixture(t, 8)
	records := []relay.PruneInventoryRecord{
		testRecoveryPruneInventoryRecord(t, fixture, 1, 0x41),
		testRecoveryPruneInventoryRecord(t, fixture, 2, 0x42),
	}
	snapshot := relay.PruneInventorySnapshot{
		MembershipGeneration: fixture.binding.MembershipGeneration,
		ArrivalHead:          fixture.binding.InventoryArrivalHead,
		PruneHead:            int64(len(records)),
	}
	fixture.remote.prune = recoveryPruneInventoryPages(fixture, snapshot, records)

	callbackFailure := errors.New("stop after verified suffix")
	_, err := fixture.coordinator.scanRecoveryPruneInventory(
		context.Background(), fixture.recovery.ProjectID, fixture.prepared, fixture.binding,
		recoveryPruneInventoryScanOptions{
			firstCheckpoint: testRecoveryPruneInventoryCheckpoint(
				t, fixture, snapshot, 1, fixture.binding.MembershipGeneration, 0x41,
			),
			onPage: func(page verifiedRecoveryPruneInventoryPage) error {
				if page.afterPruneSequence != 1 || len(page.prunes) != 1 || page.prunes[0].pruneSequence != 2 || page.more {
					t.Fatalf("verified suffix page = %#v, want only prune sequence 2", page)
				}
				return callbackFailure
			},
		},
	)
	if !errors.Is(err, callbackFailure) || len(fixture.remote.pruneRequests) != 1 {
		t.Fatalf("suffix callback result = {err=%v requests=%d}, want exact callback error after one request", err, len(fixture.remote.pruneRequests))
	}
	request := fixture.remote.pruneRequests[0]
	if request.After != 1 || request.Snapshot == nil || *request.Snapshot != snapshot {
		t.Fatalf("suffix request = %#v, want exact supplied snapshot and cursor", request)
	}

	fixture.remote.pruneRequests = nil
	_, err = fixture.coordinator.scanRecoveryPruneInventory(
		context.Background(), fixture.recovery.ProjectID, fixture.prepared, fixture.binding,
		recoveryPruneInventoryScanOptions{firstCheckpoint: &recoveryPruneInventoryCheckpoint{throughPruneSequence: 1}},
	)
	assertProblem(t, err, CodeInvalid, PhasePruneInventory, ActionRestartRecovery)
	if len(fixture.remote.pruneRequests) != 0 {
		t.Fatal("invalid unpinned resume reached the relay")
	}
}

func TestScanRecoveryPruneInventoryRejectsMaxSequenceDuplicateWithoutOverflow(t *testing.T) {
	fixture := newRecoveryDownloadFixture(t, math.MaxInt64)
	first := testRecoveryPruneInventoryRecord(t, fixture, 1, 0x43)
	second := testRecoveryPruneInventoryRecord(t, fixture, 2, 0x44)
	first.PruneSequence = math.MaxInt64
	second.PruneSequence = math.MaxInt64
	snapshot := relay.PruneInventorySnapshot{
		MembershipGeneration: fixture.binding.MembershipGeneration,
		ArrivalHead:          math.MaxInt64,
		PruneHead:            math.MaxInt64,
	}
	fixture.remote.prune = func(context.Context, relay.PruneInventoryRequest) (relay.PruneInventoryPage, error) {
		return relay.PruneInventoryPage{
			Channel: relay.ChannelAuthority{
				ChannelID:       relay.ChannelID(fixture.prepared.ChannelID),
				RelayGeneration: relay.RelayGeneration(fixture.prepared.RelayGeneration),
				AdminPublicKey:  relay.PublicKey(fixture.prepared.AdminPublicKey),
			},
			Snapshot: snapshot,
			Prunes:   []relay.PruneInventoryRecord{first, second},
		}, nil
	}
	var callbacks int
	_, err := fixture.coordinator.scanRecoveryPruneInventory(
		context.Background(), fixture.recovery.ProjectID, fixture.prepared, fixture.binding,
		recoveryPruneInventoryScanOptions{
			firstCheckpoint: testRecoveryPruneInventoryCheckpoint(
				t, fixture, snapshot, math.MaxInt64-1, fixture.binding.MembershipGeneration, 0x42,
			),
			onPage: func(verifiedRecoveryPruneInventoryPage) error {
				callbacks++
				return nil
			},
		},
	)
	assertProblem(t, err, CodeRemote, PhasePruneInventory, ActionRestartRecovery)
	if callbacks != 0 {
		t.Fatalf("callbacks = %d, want none for duplicate maximum sequence", callbacks)
	}
}

func TestScanRecoveryPruneInventoryUsesHistoricalWitnessesAndFencesResumeGeneration(t *testing.T) {
	fixture, later := newRecoveryPruneHistoricalFixture(t, 8)

	t.Run("generation one excludes later join", func(t *testing.T) {
		record := testRecoveryPruneInventoryRecordWithWitnesses(
			t, fixture, 1, 0x45, 1, []credential.TrustedProjectCredential{fixture.prepared},
		)
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
				verified = page.prunes[0]
				return nil
			}},
		)
		if err != nil || verified.membershipGeneration != 1 {
			t.Fatalf("historical scan = {verified=%#v err=%v}, want generation-one prune under generation-two binding", verified, err)
		}
	})

	t.Run("generation two cannot omit active later join", func(t *testing.T) {
		record := testRecoveryPruneInventoryRecordWithWitnesses(
			t, fixture, 1, 0x46, 2, []credential.TrustedProjectCredential{fixture.prepared},
		)
		snapshot := relay.PruneInventorySnapshot{
			MembershipGeneration: fixture.binding.MembershipGeneration,
			ArrivalHead:          fixture.binding.InventoryArrivalHead,
			PruneHead:            1,
		}
		fixture.remote.prune = recoveryPruneInventoryPages(fixture, snapshot, []relay.PruneInventoryRecord{record})
		var callbacks int
		_, err := fixture.coordinator.scanRecoveryPruneInventory(
			context.Background(), fixture.recovery.ProjectID, fixture.prepared, fixture.binding,
			recoveryPruneInventoryScanOptions{onPage: func(verifiedRecoveryPruneInventoryPage) error {
				callbacks++
				return nil
			}},
		)
		assertProblem(t, err, CodeRemote, PhasePruneInventory, ActionRestartRecovery)
		if callbacks != 0 {
			t.Fatalf("callbacks = %d, want none for incomplete historical witness set", callbacks)
		}
	})

	t.Run("resume rejects membership rollback", func(t *testing.T) {
		generationTwo := testRecoveryPruneInventoryRecordWithWitnesses(
			t, fixture, 1, 0x47, 2,
			[]credential.TrustedProjectCredential{fixture.prepared, later},
		)
		generationOne := testRecoveryPruneInventoryRecordWithWitnesses(
			t, fixture, 2, 0x48, 1, []credential.TrustedProjectCredential{fixture.prepared},
		)
		snapshot := relay.PruneInventorySnapshot{
			MembershipGeneration: fixture.binding.MembershipGeneration,
			ArrivalHead:          fixture.binding.InventoryArrivalHead,
			PruneHead:            2,
		}
		fixture.remote.prune = recoveryPruneInventoryPages(
			fixture, snapshot, []relay.PruneInventoryRecord{generationTwo, generationOne},
		)
		var callbacks int
		_, err := fixture.coordinator.scanRecoveryPruneInventory(
			context.Background(), fixture.recovery.ProjectID, fixture.prepared, fixture.binding,
			recoveryPruneInventoryScanOptions{
				firstCheckpoint: testRecoveryPruneInventoryCheckpoint(t, fixture, snapshot, 1, 2, 0x43),
				onPage: func(verifiedRecoveryPruneInventoryPage) error {
					callbacks++
					return nil
				},
			},
		)
		assertProblem(t, err, CodeRemote, PhasePruneInventory, ActionRestartRecovery)
		if callbacks != 0 {
			t.Fatalf("callbacks = %d, want none for resumed membership rollback", callbacks)
		}
	})
}

func TestScanRecoveryPruneInventoryRejectsMalformedPagesBeforeCallback(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*relay.PruneInventoryPage)
	}{
		{name: "channel", mutate: func(page *relay.PruneInventoryPage) { page.Channel.ChannelID[0] ^= 0xff }},
		{name: "snapshot membership", mutate: func(page *relay.PruneInventoryPage) { page.Snapshot.MembershipGeneration++ }},
		{name: "snapshot arrival head", mutate: func(page *relay.PruneInventoryPage) { page.Snapshot.ArrivalHead++ }},
		{name: "prune head beyond arrivals", mutate: func(page *relay.PruneInventoryPage) { page.Snapshot.PruneHead = page.Snapshot.ArrivalHead + 1 }},
		{name: "sequence gap", mutate: func(page *relay.PruneInventoryPage) { page.Prunes[0].PruneSequence++ }},
		{name: "short nonterminal page", mutate: func(page *relay.PruneInventoryPage) { page.More = true }},
		{name: "certificate id", mutate: func(page *relay.PruneInventoryPage) { page.Prunes[0].Certificate.CertificateID[0] ^= 0xff }},
		{name: "random prune id", mutate: func(page *relay.PruneInventoryPage) { page.Prunes[0].Certificate.PruneID[0] ^= 0xff }},
		{name: "wrapper target", mutate: func(page *relay.PruneInventoryPage) { page.Prunes[0].Certificate.Targets[0].EnvelopeDigest[0] ^= 0xff }},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newRecoveryDownloadFixture(t, 4)
			record := testRecoveryPruneInventoryRecord(t, fixture, 1, 0x51)
			page := relay.PruneInventoryPage{
				Channel: relay.ChannelAuthority{
					ChannelID:       relay.ChannelID(fixture.prepared.ChannelID),
					RelayGeneration: relay.RelayGeneration(fixture.prepared.RelayGeneration),
					AdminPublicKey:  relay.PublicKey(fixture.prepared.AdminPublicKey),
				},
				Snapshot: relay.PruneInventorySnapshot{
					MembershipGeneration: fixture.binding.MembershipGeneration,
					ArrivalHead:          fixture.binding.InventoryArrivalHead,
					PruneHead:            1,
				},
				Prunes: []relay.PruneInventoryRecord{record},
			}
			test.mutate(&page)
			fixture.remote.prune = func(context.Context, relay.PruneInventoryRequest) (relay.PruneInventoryPage, error) {
				return page, nil
			}
			var callbacks int
			_, err := fixture.coordinator.scanRecoveryPruneInventory(
				context.Background(), fixture.recovery.ProjectID, fixture.prepared, fixture.binding,
				recoveryPruneInventoryScanOptions{onPage: func(verifiedRecoveryPruneInventoryPage) error {
					callbacks++
					return nil
				}},
			)
			assertProblem(t, err, CodeRemote, PhasePruneInventory, ActionRestartRecovery)
			if callbacks != 0 {
				t.Fatalf("callbacks = %d, want none for malformed page", callbacks)
			}
		})
	}
}

func TestScanRecoveryPruneInventoryCollapsesCapsuleAuthenticationFailures(t *testing.T) {
	fixture := newRecoveryDownloadFixture(t, 4)
	record := testRecoveryPruneInventoryRecord(t, fixture, 1, 0x61)
	snapshot := relay.PruneInventorySnapshot{
		MembershipGeneration: fixture.binding.MembershipGeneration,
		ArrivalHead:          fixture.binding.InventoryArrivalHead,
		PruneHead:            1,
	}
	fixture.remote.prune = recoveryPruneInventoryPages(fixture, snapshot, []relay.PruneInventoryRecord{record})
	prepared := fixture.prepared
	rootBytes := prepared.ProjectRoot.Bytes()
	rootBytes[0] ^= 0xff
	wrongRoot, err := crypto.ProjectRootFromBytes(rootBytes[:])
	if err != nil {
		t.Fatalf("construct alternate project root: %v", err)
	}
	prepared.ProjectRoot = wrongRoot
	if err := prepared.Validate(); err != nil {
		t.Fatalf("alternate trusted credential: %v", err)
	}

	var callbacks int
	_, err = fixture.coordinator.scanRecoveryPruneInventory(
		context.Background(), fixture.recovery.ProjectID, prepared, fixture.binding,
		recoveryPruneInventoryScanOptions{onPage: func(verifiedRecoveryPruneInventoryPage) error {
			callbacks++
			return nil
		}},
	)
	assertProblem(t, err, CodeConflict, PhasePruneInventory, ActionRestartRecovery)
	if callbacks != 0 {
		t.Fatalf("callbacks = %d, want none for unauthenticated capsule", callbacks)
	}
}

func TestScanRecoveryPruneInventoryRejectsUnverifiedSignedAndBootstrapMaterial(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*testing.T, recoveryDownloadFixture, *relay.PruneInventoryRecord)
	}{
		{
			name: "administrator signature",
			mutate: func(t *testing.T, _ recoveryDownloadFixture, record *relay.PruneInventoryRecord) {
				certificate := parseRecoveryPruneCertificate(t, *record)
				certificate.AdminSignature[0] ^= 0xff
				replaceRecoveryPruneCertificate(t, record, certificate)
			},
		},
		{
			name: "bootstrap entry does not name manifest target",
			mutate: func(t *testing.T, fixture recoveryDownloadFixture, record *relay.PruneInventoryRecord) {
				certificate := parseRecoveryPruneCertificate(t, *record)
				key, err := crypto.DerivePruneBootstrapKey(
					fixture.prepared.ProjectRoot, fixture.recovery.ProjectID, protocol.PruneBootstrapPurposeVersionV1,
				)
				if err != nil {
					t.Fatalf("derive bootstrap key for manifest mismatch: %v", err)
				}
				plaintext, err := crypto.OpenPruneBootstrap(certificate.Capsule, key)
				if err != nil {
					t.Fatalf("open bootstrap for manifest mismatch: %v", err)
				}
				plaintext.Entries[0].PruneReferenceDigest = protocol.Digest(testArray32(0xf1))
				certificate.Capsule, err = crypto.SealPruneBootstrap(plaintext, key)
				if err != nil {
					t.Fatalf("reseal bootstrap for manifest mismatch: %v", err)
				}
				certificate.CapsuleDigest = protocol.PruneBootstrapDigest(certificate.Capsule)
				certificate.Acknowledgements[0].CapsuleDigest = certificate.CapsuleDigest
				certificate.Acknowledgements[0], err = crypto.SignPruneAcknowledgement(
					certificate.Acknowledgements[0], fixture.prepared.Certificate,
					fixture.prepared.AdminPublicKey, fixture.prepared.EnvironmentSeed,
				)
				if err != nil {
					t.Fatalf("resign prune acknowledgement for manifest mismatch: %v", err)
				}
				certificate, err = crypto.SignPruneCertificate(
					certificate, []protocol.EnvironmentCertificate{fixture.prepared.Certificate}, fixture.recovery.AdminSeed,
				)
				if err != nil {
					t.Fatalf("resign prune certificate for manifest mismatch: %v", err)
				}
				replaceRecoveryPruneCertificate(t, record, certificate)
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newRecoveryDownloadFixture(t, 4)
			record := testRecoveryPruneInventoryRecord(t, fixture, 1, 0x71)
			test.mutate(t, fixture, &record)
			snapshot := relay.PruneInventorySnapshot{
				MembershipGeneration: fixture.binding.MembershipGeneration,
				ArrivalHead:          fixture.binding.InventoryArrivalHead,
				PruneHead:            1,
			}
			fixture.remote.prune = recoveryPruneInventoryPages(fixture, snapshot, []relay.PruneInventoryRecord{record})
			var callbacks int
			_, err := fixture.coordinator.scanRecoveryPruneInventory(
				context.Background(), fixture.recovery.ProjectID, fixture.prepared, fixture.binding,
				recoveryPruneInventoryScanOptions{onPage: func(verifiedRecoveryPruneInventoryPage) error {
					callbacks++
					return nil
				}},
			)
			assertProblem(t, err, CodeRemote, PhasePruneInventory, ActionRestartRecovery)
			if callbacks != 0 {
				t.Fatalf("callbacks = %d, want none for unverified material", callbacks)
			}
		})
	}
}

func TestScanRecoveryPruneInventoryHandlesEmptySnapshotsAndCancellation(t *testing.T) {
	t.Run("empty inventory", func(t *testing.T) {
		fixture := newRecoveryDownloadFixture(t, 0)
		snapshot := relay.PruneInventorySnapshot{MembershipGeneration: fixture.binding.MembershipGeneration}
		fixture.remote.prune = recoveryPruneInventoryPages(fixture, snapshot, nil)
		var callbacks int
		result, err := fixture.coordinator.scanRecoveryPruneInventory(
			context.Background(), fixture.recovery.ProjectID, fixture.prepared, fixture.binding,
			recoveryPruneInventoryScanOptions{onPage: func(page verifiedRecoveryPruneInventoryPage) error {
				callbacks++
				if len(page.prunes) != 0 || page.more || page.afterPruneSequence != 0 || page.snapshot != snapshot {
					t.Fatalf("empty verified page = %#v, want exact empty terminal snapshot", page)
				}
				return nil
			}},
		)
		if err != nil || result.snapshot != snapshot || callbacks != 1 {
			t.Fatalf("empty scan = {result=%#v err=%v callbacks=%d}, want exact snapshot, nil, one callback", result, err, callbacks)
		}
	})

	t.Run("already at pinned head", func(t *testing.T) {
		fixture := newRecoveryDownloadFixture(t, 2)
		record := testRecoveryPruneInventoryRecord(t, fixture, 1, 0x81)
		snapshot := relay.PruneInventorySnapshot{
			MembershipGeneration: fixture.binding.MembershipGeneration,
			ArrivalHead:          fixture.binding.InventoryArrivalHead,
			PruneHead:            1,
		}
		fixture.remote.prune = recoveryPruneInventoryPages(fixture, snapshot, []relay.PruneInventoryRecord{record})
		var callbacks int
		_, err := fixture.coordinator.scanRecoveryPruneInventory(
			context.Background(), fixture.recovery.ProjectID, fixture.prepared, fixture.binding,
			recoveryPruneInventoryScanOptions{
				firstCheckpoint: testRecoveryPruneInventoryCheckpoint(
					t, fixture, snapshot, 1, fixture.binding.MembershipGeneration, 0x44,
				),
				onPage: func(page verifiedRecoveryPruneInventoryPage) error {
					callbacks++
					if len(page.prunes) != 0 || page.afterPruneSequence != 1 {
						t.Fatalf("at-head page = %#v, want empty cursor one", page)
					}
					return nil
				},
			},
		)
		if err != nil || callbacks != 1 || len(fixture.remote.pruneRequests) != 1 {
			t.Fatalf("at-head scan = {err=%v callbacks=%d requests=%d}, want nil, one, one", err, callbacks, len(fixture.remote.pruneRequests))
		}
	})

	t.Run("pre-cancelled", func(t *testing.T) {
		fixture := newRecoveryDownloadFixture(t, 0)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := fixture.coordinator.scanRecoveryPruneInventory(
			ctx, fixture.recovery.ProjectID, fixture.prepared, fixture.binding, recoveryPruneInventoryScanOptions{},
		)
		if !errors.Is(err, context.Canceled) || len(fixture.remote.pruneRequests) != 0 {
			t.Fatalf("pre-cancelled scan = {err=%v requests=%d}, want context cancellation before relay", err, len(fixture.remote.pruneRequests))
		}
	})

	t.Run("cancelled remote success", func(t *testing.T) {
		fixture := newRecoveryDownloadFixture(t, 2)
		record := testRecoveryPruneInventoryRecord(t, fixture, 1, 0x82)
		ctx, cancel := context.WithCancel(context.Background())
		fixture.remote.prune = func(context.Context, relay.PruneInventoryRequest) (relay.PruneInventoryPage, error) {
			cancel()
			return relay.PruneInventoryPage{
				Channel: relay.ChannelAuthority{
					ChannelID: relay.ChannelID(fixture.prepared.ChannelID), RelayGeneration: relay.RelayGeneration(fixture.prepared.RelayGeneration),
					AdminPublicKey: relay.PublicKey(fixture.prepared.AdminPublicKey),
				},
				Snapshot: relay.PruneInventorySnapshot{MembershipGeneration: fixture.binding.MembershipGeneration, ArrivalHead: fixture.binding.InventoryArrivalHead, PruneHead: 1},
				Prunes:   []relay.PruneInventoryRecord{record},
			}, nil
		}
		var callbacks int
		_, err := fixture.coordinator.scanRecoveryPruneInventory(
			ctx, fixture.recovery.ProjectID, fixture.prepared, fixture.binding,
			recoveryPruneInventoryScanOptions{onPage: func(verifiedRecoveryPruneInventoryPage) error { callbacks++; return nil }},
		)
		if !errors.Is(err, context.Canceled) || callbacks != 0 {
			t.Fatalf("cancelled successful response = {err=%v callbacks=%d}, want context cancellation before callback", err, callbacks)
		}
	})
}

func TestVerifyRecoveryPruneInventoryRecordGivesCancellationPrecedenceOnCryptoFailures(t *testing.T) {
	fixture := newRecoveryDownloadFixture(t, 4)
	wantChannel := relay.ChannelAuthority{
		ChannelID:       relay.ChannelID(fixture.binding.ChannelID),
		RelayGeneration: relay.RelayGeneration(fixture.binding.RelayGeneration),
		AdminPublicKey:  relay.PublicKey(fixture.binding.AdminPublicKey),
	}
	snapshot := relay.PruneInventorySnapshot{
		MembershipGeneration: fixture.binding.MembershipGeneration,
		ArrivalHead:          fixture.binding.InventoryArrivalHead,
		PruneHead:            1,
	}
	witnesses := map[uint32][]protocol.EnvironmentCertificate{
		fixture.binding.MembershipGeneration: {fixture.prepared.Certificate},
	}
	bootstrapKey, err := crypto.DerivePruneBootstrapKey(
		fixture.prepared.ProjectRoot, fixture.recovery.ProjectID, protocol.PruneBootstrapPurposeVersionV1,
	)
	if err != nil {
		t.Fatalf("derive cancellation test bootstrap key: %v", err)
	}

	t.Run("signature verification", func(t *testing.T) {
		record := testRecoveryPruneInventoryRecord(t, fixture, 1, 0x83)
		certificate := parseRecoveryPruneCertificate(t, record)
		certificate.AdminSignature[0] ^= 0xff
		replaceRecoveryPruneCertificate(t, &record, certificate)
		ctx := newRecoveryPruneCancelAfterChecksContext(3)
		_, err := fixture.coordinator.verifyRecoveryPruneInventoryRecord(
			ctx, fixture.recovery.ProjectID, fixture.binding, bootstrapKey,
			wantChannel, snapshot, record, witnesses,
		)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("signature/cancellation error = %v, want context.Canceled", err)
		}
	})

	t.Run("capsule authentication", func(t *testing.T) {
		record := testRecoveryPruneInventoryRecord(t, fixture, 1, 0x84)
		root := fixture.prepared.ProjectRoot.Bytes()
		root[0] ^= 0xff
		wrongRoot, err := crypto.ProjectRootFromBytes(root[:])
		if err != nil {
			t.Fatalf("construct cancellation test alternate root: %v", err)
		}
		wrongKey, err := crypto.DerivePruneBootstrapKey(
			wrongRoot, fixture.recovery.ProjectID, protocol.PruneBootstrapPurposeVersionV1,
		)
		if err != nil {
			t.Fatalf("derive cancellation test alternate key: %v", err)
		}
		ctx := newRecoveryPruneCancelAfterChecksContext(4)
		_, err = fixture.coordinator.verifyRecoveryPruneInventoryRecord(
			ctx, fixture.recovery.ProjectID, fixture.binding, wrongKey,
			wantChannel, snapshot, record, witnesses,
		)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("capsule/cancellation error = %v, want context.Canceled", err)
		}
	})
}

func TestRecoveryPruneInventoryErrorMappingsAreSecretFree(t *testing.T) {
	const secretMarker = "prune-inventory-secret-marker"
	for _, test := range []struct {
		name       string
		remoteErr  error
		wantCode   ProblemCode
		wantAction ProblemAction
	}{
		{name: "unauthenticated", remoteErr: relay.ErrUnauthenticated, wantCode: CodeAuthorization, wantAction: ActionCheckRecoveryAuthority},
		{name: "not found", remoteErr: relay.ErrNotFound, wantCode: CodeAuthorization, wantAction: ActionCheckRecoveryAuthority},
		{name: "membership changed", remoteErr: relay.ErrMembershipChanged, wantCode: CodeConflict, wantAction: ActionRetry},
		{name: "generation mismatch", remoteErr: relay.ErrGenerationMismatch, wantCode: CodeConflict, wantAction: ActionRestartRecovery},
		{name: "rollback", remoteErr: relay.ErrRollback, wantCode: CodeConflict, wantAction: ActionRestartRecovery},
		{name: "retired", remoteErr: relay.ErrRetired, wantCode: CodeConflict, wantAction: ActionRestartRecovery},
		{name: "expired", remoteErr: relay.ErrExpired, wantCode: CodeConflict, wantAction: ActionRestartRecovery},
		{name: "invalid", remoteErr: relay.ErrInvalidArgument, wantCode: CodeRemote, wantAction: ActionRestartRecovery},
		{name: "unverified", remoteErr: relay.ErrUnverified, wantCode: CodeRemote, wantAction: ActionRestartRecovery},
		{name: "closed", remoteErr: relay.ErrClosed, wantCode: CodeUnavailable, wantAction: ActionRetry},
		{name: "unknown", remoteErr: errors.New(secretMarker), wantCode: CodeUnavailable, wantAction: ActionRetry},
	} {
		t.Run("relay/"+test.name, func(t *testing.T) {
			err := mapRecoveryPruneInventoryRelayError(context.Background(), fmt.Errorf("%s: %w", secretMarker, test.remoteErr))
			assertProblem(t, err, test.wantCode, PhasePruneInventory, test.wantAction)
			if strings.Contains(err.Error(), secretMarker) || strings.Contains(fmt.Sprintf("%#v", err), secretMarker) {
				t.Fatal("relay mapping exposed its cause")
			}
		})
	}

	for _, test := range []struct {
		name       string
		storeErr   error
		wantCode   ProblemCode
		wantAction ProblemAction
	}{
		{name: "authority race", storeErr: &continuitysqlite.SyncError{Code: continuitysqlite.SyncErrorConflict, Field: "sync_authority", Detail: secretMarker}, wantCode: CodeConflict, wantAction: ActionRetry},
		{name: "candidate race", storeErr: &continuitysqlite.SyncError{Code: continuitysqlite.SyncErrorConflict, Field: "sync_authority_candidate", Detail: secretMarker}, wantCode: CodeConflict, wantAction: ActionRetry},
		{name: "witness overflow", storeErr: &continuitysqlite.SyncError{Code: continuitysqlite.SyncErrorConflict, Field: "prune_witness_authority", Detail: secretMarker}, wantCode: CodeConflict, wantAction: ActionRestartRecovery},
		{name: "other conflict", storeErr: &continuitysqlite.SyncError{Code: continuitysqlite.SyncErrorConflict, Field: "other", Detail: secretMarker}, wantCode: CodeConflict, wantAction: ActionRestartRecovery},
		{name: "corruption", storeErr: &continuitysqlite.SyncError{Code: continuitysqlite.SyncErrorStore, Field: "sync_authority", Detail: secretMarker}, wantCode: CodeInternal, wantAction: ActionRepairLocalStore},
		{name: "unavailable", storeErr: &continuitysqlite.SyncError{Code: continuitysqlite.SyncErrorStore, Detail: secretMarker}, wantCode: CodeUnavailable, wantAction: ActionRetry},
		{name: "unknown", storeErr: errors.New(secretMarker), wantCode: CodeInternal, wantAction: ActionRepairLocalStore},
	} {
		t.Run("store/"+test.name, func(t *testing.T) {
			err := mapRecoveryPruneInventoryStoreError(context.Background(), test.storeErr)
			assertProblem(t, err, test.wantCode, PhasePruneInventory, test.wantAction)
			if strings.Contains(err.Error(), secretMarker) || strings.Contains(fmt.Sprintf("%#v", err), secretMarker) {
				t.Fatal("store mapping exposed its cause")
			}
		})
	}
}

func testRecoveryPruneInventoryRecord(
	t *testing.T,
	fixture recoveryDownloadFixture,
	sequence int64,
	seed byte,
) relay.PruneInventoryRecord {
	t.Helper()
	return testRecoveryPruneInventoryRecordWithWitnesses(
		t, fixture, sequence, seed, fixture.binding.MembershipGeneration,
		[]credential.TrustedProjectCredential{fixture.prepared},
	)
}

func testRecoveryPruneInventoryRecordWithWitnesses(
	t *testing.T,
	fixture recoveryDownloadFixture,
	sequence int64,
	seed byte,
	membershipGeneration uint32,
	witnesses []credential.TrustedProjectCredential,
) relay.PruneInventoryRecord {
	t.Helper()
	target := protocol.PruneReference{
		FactID:              continuity.FactID(fmt.Sprintf("fact-prune-target-%d", sequence)),
		EnvironmentID:       fixture.prepared.Certificate.EnvironmentID,
		EnvironmentSequence: sequence*2 - 1,
		ArrivalSequence:     sequence*2 - 1,
		EnvelopeDigest:      protocol.Digest(testArray32(seed)),
		CertificateID:       protocol.CertificateID(fixture.prepared.Certificate),
		KeyGeneration:       fixture.prepared.WriteGeneration,
		Nonce:               protocol.Nonce(testArray24(seed + 1)),
	}
	if target.EnvironmentSequence > 1 {
		target.PreviousEnvelopeDigest = protocol.Digest(testArray32(seed + 6))
	}
	closure := protocol.PruneReference{
		FactID:                 continuity.FactID(fmt.Sprintf("fact-prune-closure-%d", sequence)),
		EnvironmentID:          fixture.prepared.Certificate.EnvironmentID,
		EnvironmentSequence:    sequence * 2,
		ArrivalSequence:        sequence * 2,
		EnvelopeDigest:         protocol.Digest(testArray32(seed + 2)),
		CertificateID:          protocol.CertificateID(fixture.prepared.Certificate),
		PreviousEnvelopeDigest: target.EnvelopeDigest,
		KeyGeneration:          fixture.prepared.WriteGeneration,
		Nonce:                  protocol.Nonce(testArray24(seed + 3)),
	}
	manifest := protocol.PruneManifest{Targets: []protocol.PruneReference{target}}
	pruneID := protocol.Digest(testArray32(seed + 4))
	plaintext := protocol.PruneBootstrapPlaintext{
		CapsuleVersion:          protocol.PruneBootstrapCapsuleVersionV1,
		ProtocolVersion:         protocol.ProtocolVersionV1,
		CipherSuite:             protocol.CipherSuiteXChaCha20Poly1305,
		BootstrapPurposeVersion: protocol.PruneBootstrapPurposeVersionV1,
		ProjectID:               fixture.recovery.ProjectID,
		ChannelID:               fixture.prepared.ChannelID,
		RelayGeneration:         fixture.prepared.RelayGeneration,
		PruneID:                 pruneID,
		MembershipGeneration:    membershipGeneration,
		BarrierArrivalSequence:  closure.ArrivalSequence,
		ClosureReferenceDigest:  protocol.PruneReferenceDigest(closure),
		ManifestCount:           1,
		ManifestDigest:          protocol.PruneManifestDigest(manifest),
		ScratchpadSubject:       continuity.SubjectID(fmt.Sprintf("scratchpad-%d", sequence)),
		EntryCount:              1,
		Entries: []protocol.PruneBootstrapEntry{{
			PruneReferenceDigest: protocol.PruneReferenceDigest(target),
			FactKind:             continuity.FactScratchpadMessageRecorded,
			HLC:                  continuity.HybridTime{WallMillis: 100 + sequence, Logical: int32(sequence)},
		}},
	}
	key, err := crypto.DerivePruneBootstrapKey(
		fixture.prepared.ProjectRoot,
		fixture.recovery.ProjectID,
		protocol.PruneBootstrapPurposeVersionV1,
	)
	if err != nil {
		t.Fatalf("derive fixture prune bootstrap key: %v", err)
	}
	capsule, err := crypto.SealPruneBootstrap(plaintext, key)
	if err != nil {
		t.Fatalf("seal fixture prune bootstrap: %v", err)
	}
	acknowledgements := make([]protocol.PruneAcknowledgement, 0, len(witnesses))
	environmentCertificates := make([]protocol.EnvironmentCertificate, 0, len(witnesses))
	for index, witness := range witnesses {
		progressDigest := protocol.Digest(testArray32(seed + 5 + byte(index)))
		acknowledgement, err := crypto.SignPruneAcknowledgement(protocol.PruneAcknowledgement{
			Version:                       protocol.ControlVersionV1,
			ProtocolVersion:               protocol.ProtocolVersionV1,
			CipherSuite:                   protocol.CipherSuiteXChaCha20Poly1305,
			ChannelID:                     fixture.prepared.ChannelID,
			RelayGeneration:               fixture.prepared.RelayGeneration,
			EnvironmentID:                 witness.Certificate.EnvironmentID,
			CertificateID:                 protocol.CertificateID(witness.Certificate),
			MembershipGeneration:          membershipGeneration,
			ProgressAcknowledgementDigest: progressDigest,
			AppliedArrivalSequence:        closure.ArrivalSequence,
			ProducerSequence:              closure.EnvironmentSequence,
			ProducerEnvelopeDigest:        closure.EnvelopeDigest,
			PruneID:                       pruneID,
			BarrierArrivalSequence:        closure.ArrivalSequence,
			ClosureReferenceDigest:        protocol.PruneReferenceDigest(closure),
			ManifestCount:                 1,
			ManifestDigest:                protocol.PruneManifestDigest(manifest),
			CapsuleDigest:                 protocol.PruneBootstrapDigest(capsule),
		}, witness.Certificate, fixture.prepared.AdminPublicKey, witness.EnvironmentSeed)
		if err != nil {
			t.Fatalf("sign fixture prune acknowledgement %d: %v", index, err)
		}
		acknowledgements = append(acknowledgements, acknowledgement)
		environmentCertificates = append(environmentCertificates, witness.Certificate)
	}
	certificate, err := crypto.SignPruneCertificate(protocol.PruneCertificate{
		Version:                    protocol.ControlVersionV1,
		ProtocolVersion:            protocol.ProtocolVersionV1,
		CipherSuite:                protocol.CipherSuiteXChaCha20Poly1305,
		ChannelID:                  fixture.prepared.ChannelID,
		RelayGeneration:            fixture.prepared.RelayGeneration,
		PruneID:                    pruneID,
		MembershipGeneration:       membershipGeneration,
		BarrierArrivalSequence:     closure.ArrivalSequence,
		Closure:                    closure,
		ClosureDigest:              protocol.PruneReferenceDigest(closure),
		ManifestCount:              1,
		ManifestDigest:             protocol.PruneManifestDigest(manifest),
		Manifest:                   manifest,
		CapsuleDigest:              protocol.PruneBootstrapDigest(capsule),
		Capsule:                    capsule,
		ActiveAcknowledgementCount: uint32(len(acknowledgements)),
		Acknowledgements:           acknowledgements,
	}, environmentCertificates, fixture.recovery.AdminSeed)
	if err != nil {
		t.Fatalf("sign fixture prune certificate: %v", err)
	}
	certificateBytes, err := certificate.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal fixture prune certificate: %v", err)
	}
	return relay.PruneInventoryRecord{
		PruneSequence: sequence,
		Certificate: relay.PruneCertificate{
			ChannelID:            relay.ChannelID(certificate.ChannelID),
			PruneID:              relay.Digest(certificate.PruneID),
			MembershipGeneration: certificate.MembershipGeneration,
			Barrier:              certificate.BarrierArrivalSequence,
			Closure:              recoveryPruneRelayTarget(certificate.Closure),
			CertificateID:        relay.Digest(protocol.PruneCertificateID(certificate)),
			CertificateBytes:     certificateBytes,
			Targets:              []relay.PruneTarget{recoveryPruneRelayTarget(certificate.Manifest.Targets[0])},
		},
		CreatedAt: time.UnixMilli(1_000 + sequence).UTC(),
	}
}

func recoveryPruneRelayTarget(reference protocol.PruneReference) relay.PruneTarget {
	return relay.PruneTarget{
		FactID:                 relay.FactID(reference.FactID),
		EnvironmentID:          relay.EnvironmentID(reference.EnvironmentID),
		EnvironmentSequence:    reference.EnvironmentSequence,
		ArrivalSequence:        reference.ArrivalSequence,
		EnvelopeDigest:         relay.Digest(reference.EnvelopeDigest),
		CertificateID:          relay.Digest(reference.CertificateID),
		PreviousEnvelopeDigest: relay.Digest(reference.PreviousEnvelopeDigest),
		KeyGeneration:          reference.KeyGeneration,
		Nonce:                  relay.Nonce(reference.Nonce),
	}
}

func parseRecoveryPruneCertificate(t *testing.T, record relay.PruneInventoryRecord) protocol.PruneCertificate {
	t.Helper()
	certificate, err := protocol.ParsePruneCertificate(record.Certificate.CertificateBytes)
	if err != nil {
		t.Fatalf("parse recovery prune certificate: %v", err)
	}
	return certificate
}

func replaceRecoveryPruneCertificate(
	t *testing.T,
	record *relay.PruneInventoryRecord,
	certificate protocol.PruneCertificate,
) {
	t.Helper()
	wire, err := certificate.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal replacement prune certificate: %v", err)
	}
	record.Certificate.CertificateBytes = wire
	record.Certificate.CertificateID = relay.Digest(protocol.PruneCertificateID(certificate))
}

func newRecoveryPruneHistoricalFixture(
	t *testing.T,
	head int64,
) (recoveryDownloadFixture, credential.TrustedProjectCredential) {
	t.Helper()
	writerID := testEnvironmentID(200)
	store := openCoordinatorStore(t, writerID)
	recovery := testBindableRecoveryCredential(t)
	prepared := testPreparedRecoveryCredential(t, recovery, writerID, 1, []uint32{recovery.WriteGeneration})
	seedRemote := inventoryRemote(recovery, relay.EnvironmentInventorySnapshot{}, nil)
	registration, err := mustCoordinator(t, store, seedRemote).bindPreparedRecoveryRegistration(
		recovery.ProjectID, recovery, prepared,
	)
	if err != nil {
		t.Fatalf("bind historical fixture writer: %v", err)
	}
	later := testPreparedRecoveryCredential(
		t, recovery, testEnvironmentID(201), 2, []uint32{recovery.WriteGeneration},
	)
	laterCertificateBytes, err := later.Certificate.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal later historical witness certificate: %v", err)
	}
	records := []relay.EnvironmentInventoryRecord{
		recoveryRegistrationInventoryRecord(registration),
		{
			EnvironmentID:        relay.EnvironmentID(later.Certificate.EnvironmentID),
			CertificateID:        relay.Digest(protocol.CertificateID(later.Certificate)),
			CertificateBytes:     laterCertificateBytes,
			Mode:                 relay.TrustedEnvironment,
			ExpiresAtMillis:      later.Certificate.ExpiresAtMillis,
			MembershipGeneration: later.Certificate.MembershipGeneration,
		},
	}
	snapshot := relay.EnvironmentInventorySnapshot{MembershipGeneration: 2, ArrivalHead: head}
	remote := exactRecoveryRegistrationInventoryRemote(recovery, registration, snapshot, records)
	coordinator := mustCoordinator(t, store, remote)
	binding, err := coordinator.convergeRegisteredRecoveryAuthority(
		context.Background(), recovery.ProjectID, recovery, registration,
	)
	if err != nil {
		t.Fatalf("converge historical prune authority: %v", err)
	}
	if binding.MembershipGeneration != 2 {
		t.Fatalf("historical fixture binding generation = %d, want 2", binding.MembershipGeneration)
	}
	return recoveryDownloadFixture{
		store: store, coordinator: coordinator, remote: remote,
		recovery: recovery, prepared: prepared, binding: binding,
	}, later
}

func recoveryPruneInventoryPages(
	fixture recoveryDownloadFixture,
	snapshot relay.PruneInventorySnapshot,
	records []relay.PruneInventoryRecord,
) func(context.Context, relay.PruneInventoryRequest) (relay.PruneInventoryPage, error) {
	return func(_ context.Context, request relay.PruneInventoryRequest) (relay.PruneInventoryPage, error) {
		start := int(request.After)
		if start < 0 || start > len(records) || request.Limit != relay.MaxPruneInventoryPage {
			return relay.PruneInventoryPage{}, relay.ErrInvalidArgument
		}
		end := start + request.Limit
		if end > len(records) {
			end = len(records)
		}
		return relay.PruneInventoryPage{
			Channel: relay.ChannelAuthority{
				ChannelID:       relay.ChannelID(fixture.prepared.ChannelID),
				RelayGeneration: relay.RelayGeneration(fixture.prepared.RelayGeneration),
				AdminPublicKey:  relay.PublicKey(fixture.prepared.AdminPublicKey),
			},
			Snapshot: snapshot,
			Prunes:   append([]relay.PruneInventoryRecord(nil), records[start:end]...),
			More:     end < len(records),
		}, nil
	}
}

func cloneVerifiedRecoveryPruneInventoryPage(page verifiedRecoveryPruneInventoryPage) verifiedRecoveryPruneInventoryPage {
	cloned := page
	cloned.prunes = append([]verifiedRecoveryPrune(nil), page.prunes...)
	for index := range cloned.prunes {
		cloned.prunes[index].targets = append([]verifiedRecoveryPruneTarget(nil), page.prunes[index].targets...)
	}
	return cloned
}

func verifiedRecoveryPruneReference(reference protocol.PruneReference) continuitysqlite.VerifiedPruneReference {
	return continuitysqlite.VerifiedPruneReference{
		FactID:                 reference.FactID,
		EnvironmentID:          reference.EnvironmentID,
		EnvironmentSequence:    reference.EnvironmentSequence,
		ArrivalSequence:        reference.ArrivalSequence,
		EnvelopeDigest:         [32]byte(reference.EnvelopeDigest),
		CertificateID:          [32]byte(reference.CertificateID),
		PreviousEnvelopeDigest: [32]byte(reference.PreviousEnvelopeDigest),
		KeyGeneration:          reference.KeyGeneration,
		Nonce:                  [24]byte(reference.Nonce),
	}
}

type recoveryPruneCancelAfterChecksContext struct {
	checks      int
	cancelAfter int
	done        chan struct{}
	canceled    bool
}

func newRecoveryPruneCancelAfterChecksContext(cancelAfter int) *recoveryPruneCancelAfterChecksContext {
	return &recoveryPruneCancelAfterChecksContext{
		cancelAfter: cancelAfter,
		done:        make(chan struct{}),
	}
}

func (*recoveryPruneCancelAfterChecksContext) Deadline() (time.Time, bool) { return time.Time{}, false }

func (ctx *recoveryPruneCancelAfterChecksContext) Done() <-chan struct{} { return ctx.done }

func (ctx *recoveryPruneCancelAfterChecksContext) Err() error {
	if ctx.canceled {
		return context.Canceled
	}
	ctx.checks++
	if ctx.checks >= ctx.cancelAfter {
		ctx.canceled = true
		close(ctx.done)
		return context.Canceled
	}
	return nil
}

func (*recoveryPruneCancelAfterChecksContext) Value(any) any { return nil }
