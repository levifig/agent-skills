package sqlite

import (
	"bytes"
	"context"
	"errors"
	"math"
	"path/filepath"
	"testing"
	"time"

	"github.com/levifig/loaf/vnext/sync/relay"
)

func TestRelayStoreContract(t *testing.T) {
	t.Parallel()

	var _ relay.Store = (*Store)(nil)
}

func TestRelayAppendPersistsOpaqueArrivalsAndEnforcesImmutableIdentityAndSequence(t *testing.T) {
	t.Parallel()

	store, owner, environment := newTestStoreWithEnvironment(t, relay.TrustedEnvironment, 0)

	first := testEnvelope(environment, "fact-a", 1, relay.Digest{}, 0x11)
	accepted, err := store.Append(t.Context(), relay.AppendRequest{Authorization: environment, Envelope: first})
	if err != nil {
		t.Fatalf("Append(first) error = %v", err)
	}
	if accepted.Disposition != relay.AppendAccepted || accepted.Arrival.ArrivalSequence != 1 || accepted.RelayHead != 1 {
		t.Fatalf("Append(first) = %#v, want accepted at arrival/head 1", accepted)
	}

	duplicate, err := store.Append(t.Context(), relay.AppendRequest{Authorization: environment, Envelope: first})
	if err != nil {
		t.Fatalf("Append(duplicate) error = %v", err)
	}
	if duplicate.Disposition != relay.AppendDuplicate || duplicate.Arrival.ArrivalSequence != 1 || duplicate.RelayHead != 1 {
		t.Fatalf("Append(duplicate) = %#v, want duplicate at arrival/head 1", duplicate)
	}

	conflictingFact := first
	conflictingFact.EnvelopeDigest = testDigest(0x22)
	if _, err := store.Append(t.Context(), relay.AppendRequest{Authorization: environment, Envelope: conflictingFact}); !errors.Is(err, relay.ErrImmutableConflict) {
		t.Fatalf("Append(conflicting fact id) error = %v, want ErrImmutableConflict", err)
	}
	conflictingSequence := testEnvelope(environment, "fact-b", 1, relay.Digest{}, 0x23)
	if _, err := store.Append(t.Context(), relay.AppendRequest{Authorization: environment, Envelope: conflictingSequence}); !errors.Is(err, relay.ErrImmutableConflict) {
		t.Fatalf("Append(conflicting source sequence) error = %v, want ErrImmutableConflict", err)
	}

	gap := testEnvelope(environment, "fact-c", 3, first.EnvelopeDigest, 0x33)
	if _, err := store.Append(t.Context(), relay.AppendRequest{Authorization: environment, Envelope: gap}); !errors.Is(err, relay.ErrSourceGap) {
		t.Fatalf("Append(source gap) error = %v, want ErrSourceGap", err)
	}
	wrongChain := testEnvelope(environment, "fact-b", 2, testDigest(0x99), 0x44)
	if _, err := store.Append(t.Context(), relay.AppendRequest{Authorization: environment, Envelope: wrongChain}); !errors.Is(err, relay.ErrPreviousDigest) {
		t.Fatalf("Append(wrong previous digest) error = %v, want ErrPreviousDigest", err)
	}
	second := testEnvelope(environment, "fact-b", 2, first.EnvelopeDigest, 0x44)
	if _, err := store.Append(t.Context(), relay.AppendRequest{Authorization: environment, Envelope: second}); err != nil {
		t.Fatalf("Append(second) error = %v", err)
	}

	wrongSecret := environment
	wrongSecret.TokenSecret = testSecret(0xee)
	if _, err := store.Page(t.Context(), relay.PageRequest{Authorization: wrongSecret, Limit: 10}); !errors.Is(err, relay.ErrUnauthenticated) {
		t.Fatalf("Page(wrong secret) error = %v, want ErrUnauthenticated", err)
	}

	page, err := store.Page(t.Context(), relay.PageRequest{Authorization: environment, Limit: 10})
	if err != nil {
		t.Fatalf("Page() error = %v", err)
	}
	if page.RelayGeneration != owner.RelayGeneration || page.Head != 2 || page.MembershipGeneration != 1 || len(page.Arrivals) != 2 {
		t.Fatalf("Page() = %#v, want generation/head 2/membership 1/two arrivals", page)
	}
	if !bytes.Equal(page.Arrivals[0].Ciphertext, first.Ciphertext) || page.Arrivals[0].EnvelopeDigest != first.EnvelopeDigest {
		t.Fatalf("Page()[0] = %#v, want exact first opaque envelope", page.Arrivals[0])
	}
}

func TestRelayAppendPersistsCanonicalZeroNonce(t *testing.T) {
	t.Parallel()

	store, _, environment := newTestStoreWithEnvironment(t, relay.TrustedEnvironment, 0)
	envelope := testEnvelope(environment, "fact-zero-nonce", 1, relay.Digest{}, 0x19)
	envelope.Nonce = relay.Nonce{}

	accepted, err := store.Append(t.Context(), relay.AppendRequest{Authorization: environment, Envelope: envelope})
	if err != nil {
		t.Fatalf("Append(zero nonce) error = %v", err)
	}
	if accepted.Disposition != relay.AppendAccepted || accepted.Arrival.Nonce != (relay.Nonce{}) {
		t.Fatalf("Append(zero nonce) = %#v, want accepted canonical nonce", accepted)
	}
}

func TestRelayStorePersistsAcrossReopen(t *testing.T) {
	t.Parallel()

	databasePath := filepath.Join(t.TempDir(), "relay.sqlite")
	store, err := Open(databasePath, allowTestVerifier{})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	owner := testOwnerAuthorization(store)
	if _, err := store.CreateChannel(t.Context(), testChannel(owner)); err != nil {
		t.Fatalf("CreateChannel() error = %v", err)
	}
	environment := testEnvironmentAuthorization(owner, "environment-a", 0x31, 0x81)
	if _, err := store.RegisterEnvironment(t.Context(), relay.RegisterEnvironmentRequest{
		Authorization: owner,
		Environment:   testEnvironment(environment, relay.TrustedEnvironment, 0, 1),
	}); err != nil {
		t.Fatalf("RegisterEnvironment() error = %v", err)
	}
	envelope := testEnvelope(environment, "fact-a", 1, relay.Digest{}, 0x41)
	if _, err := store.Append(t.Context(), relay.AppendRequest{Authorization: environment, Envelope: envelope}); err != nil {
		t.Fatalf("Append() error = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	reopened, err := Open(databasePath, allowTestVerifier{})
	if err != nil {
		t.Fatalf("Open(reopen) error = %v", err)
	}
	t.Cleanup(func() { closeTestStore(t, reopened) })
	if reopened.RelayGeneration() != owner.RelayGeneration {
		t.Fatalf("RelayGeneration(reopen) = %x, want persisted %x", reopened.RelayGeneration(), owner.RelayGeneration)
	}
	page, err := reopened.Page(t.Context(), relay.PageRequest{Authorization: environment, Limit: 10})
	if err != nil {
		t.Fatalf("Page(reopen) error = %v", err)
	}
	if page.Head != 1 || len(page.Arrivals) != 1 || page.Arrivals[0].EnvelopeDigest != envelope.EnvelopeDigest {
		t.Fatalf("Page(reopen) = %#v, want persisted arrival", page)
	}
}

func TestRelayEnvironmentExpiryAndRetirementFenceNewArrivals(t *testing.T) {
	t.Parallel()

	store, owner, environment := newTestStoreWithEnvironment(t, relay.EphemeralEnvironment, 2_000)
	store.now = func() time.Time { return time.UnixMilli(1_000) }
	first := testEnvelope(environment, "fact-a", 1, relay.Digest{}, 0x51)
	if _, err := store.Append(t.Context(), relay.AppendRequest{Authorization: environment, Envelope: first}); err != nil {
		t.Fatalf("Append(before expiry) error = %v", err)
	}

	store.now = func() time.Time { return time.UnixMilli(2_000) }
	second := testEnvelope(environment, "fact-b", 2, first.EnvelopeDigest, 0x52)
	if _, err := store.Append(t.Context(), relay.AppendRequest{Authorization: environment, Envelope: second}); !errors.Is(err, relay.ErrExpired) {
		t.Fatalf("Append(expired) error = %v, want ErrExpired", err)
	}
	registrationReplay := relay.RegisterEnvironmentRequest{
		Authorization: owner,
		Environment:   testEnvironment(environment, relay.EphemeralEnvironment, 2_000, 1),
	}
	if _, err := store.RegisterEnvironment(t.Context(), registrationReplay); !errors.Is(err, relay.ErrExpired) {
		t.Fatalf("RegisterEnvironment(expired replay) error = %v, want ErrExpired", err)
	}

	if _, err := store.RetireEnvironment(t.Context(), relay.RetireEnvironmentRequest{
		Authorization: owner,
		Retirement: relay.Retirement{
			ChannelID:                owner.ChannelID,
			RelayGeneration:          owner.RelayGeneration,
			EnvironmentID:            environment.EnvironmentID,
			CertificateID:            environment.CertificateID,
			MembershipGeneration:     2,
			FinalEnvironmentSequence: 1,
			FinalEnvelopeDigest:      first.EnvelopeDigest,
			RetirementID:             testDigest(0x53),
			RetirementBytes:          []byte("opaque-signed-retirement"),
		},
	}); err != nil {
		t.Fatalf("RetireEnvironment() error = %v", err)
	}
	if _, err := store.RegisterEnvironment(t.Context(), registrationReplay); !errors.Is(err, relay.ErrRetired) {
		t.Fatalf("RegisterEnvironment(retired replay) error = %v, want ErrRetired", err)
	}
	if _, err := store.Append(t.Context(), relay.AppendRequest{Authorization: environment, Envelope: second}); !errors.Is(err, relay.ErrRetired) {
		t.Fatalf("Append(retired) error = %v, want ErrRetired", err)
	}
	if _, err := store.RetireEnvironment(t.Context(), relay.RetireEnvironmentRequest{
		Authorization: owner,
		Retirement: relay.Retirement{
			ChannelID:                owner.ChannelID,
			RelayGeneration:          owner.RelayGeneration,
			EnvironmentID:            environment.EnvironmentID,
			CertificateID:            environment.CertificateID,
			MembershipGeneration:     3,
			FinalEnvironmentSequence: 1,
			FinalEnvelopeDigest:      first.EnvelopeDigest,
			RetirementID:             testDigest(0x54),
			RetirementBytes:          []byte("replacement-retirement"),
		},
	}); !errors.Is(err, relay.ErrImmutableConflict) {
		t.Fatalf("RetireEnvironment(different replay) error = %v, want ErrImmutableConflict", err)
	}
}

func TestRelayAcknowledgementExactRetrySurvivesProducerAdvanceAndRetirement(t *testing.T) {
	t.Parallel()

	store, owner, environment := newTestStoreWithEnvironment(t, relay.TrustedEnvironment, 0)
	first := testEnvelope(environment, "fact-a", 1, relay.Digest{}, 0x57)
	if _, err := store.Append(t.Context(), relay.AppendRequest{Authorization: environment, Envelope: first}); err != nil {
		t.Fatalf("Append(first) error = %v", err)
	}
	acknowledgement := relay.AcknowledgeRequest{
		Authorization: environment,
		Acknowledgement: relay.Acknowledgement{
			ChannelID:              owner.ChannelID,
			EnvironmentID:          environment.EnvironmentID,
			MembershipGeneration:   1,
			AppliedArrivalSequence: 1,
			ProducerSequence:       1,
			ProducerEnvelopeDigest: first.EnvelopeDigest,
			CertificateID:          environment.CertificateID,
			AcknowledgementDigest:  testDigest(0x58),
			AcknowledgementBytes:   []byte("opaque-signed-acknowledgement"),
		},
	}
	if err := store.Acknowledge(t.Context(), acknowledgement); err != nil {
		t.Fatalf("Acknowledge(first) error = %v", err)
	}
	second := testEnvelope(environment, "fact-b", 2, first.EnvelopeDigest, 0x59)
	if _, err := store.Append(t.Context(), relay.AppendRequest{Authorization: environment, Envelope: second}); err != nil {
		t.Fatalf("Append(second) error = %v", err)
	}
	if _, err := store.RetireEnvironment(t.Context(), relay.RetireEnvironmentRequest{
		Authorization: owner,
		Retirement: relay.Retirement{
			ChannelID:                owner.ChannelID,
			RelayGeneration:          owner.RelayGeneration,
			EnvironmentID:            environment.EnvironmentID,
			CertificateID:            environment.CertificateID,
			MembershipGeneration:     2,
			FinalEnvironmentSequence: 2,
			FinalEnvelopeDigest:      second.EnvelopeDigest,
			RetirementID:             testDigest(0x5a),
			RetirementBytes:          []byte("opaque-signed-retirement"),
		},
	}); err != nil {
		t.Fatalf("RetireEnvironment() error = %v", err)
	}

	if err := store.Acknowledge(t.Context(), acknowledgement); err != nil {
		t.Fatalf("Acknowledge(exact retry after producer advance and retirement) error = %v, want nil", err)
	}
	conflicting := acknowledgement
	conflicting.Acknowledgement.AcknowledgementDigest = testDigest(0x5b)
	if err := store.Acknowledge(t.Context(), conflicting); !errors.Is(err, relay.ErrRetired) {
		t.Fatalf("Acknowledge(conflicting retry after retirement) error = %v, want ErrRetired", err)
	}
}

func TestRelayRetirementFinalFenceMustMatchProducerIdentityHeadAndDigest(t *testing.T) {
	t.Parallel()

	verifier := &gatedTestVerifier{}
	store, err := Open(filepath.Join(t.TempDir(), "relay.sqlite"), verifier)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { closeTestStore(t, store) })
	owner := testOwnerAuthorization(store)
	if _, err := store.CreateChannel(t.Context(), testChannel(owner)); err != nil {
		t.Fatalf("CreateChannel() error = %v", err)
	}
	environment := testEnvironmentAuthorization(owner, "environment-a", 0x21, 0x81)
	if _, err := store.RegisterEnvironment(t.Context(), relay.RegisterEnvironmentRequest{
		Authorization: owner,
		Environment:   testEnvironment(environment, relay.TrustedEnvironment, 0, 1),
	}); err != nil {
		t.Fatalf("RegisterEnvironment() error = %v", err)
	}
	envelope := testEnvelope(environment, "fact-a", 1, relay.Digest{}, 0x55)
	if _, err := store.Append(t.Context(), relay.AppendRequest{Authorization: environment, Envelope: envelope}); err != nil {
		t.Fatalf("Append() error = %v", err)
	}
	retirement := relay.Retirement{
		ChannelID:                owner.ChannelID,
		RelayGeneration:          owner.RelayGeneration,
		EnvironmentID:            environment.EnvironmentID,
		CertificateID:            environment.CertificateID,
		MembershipGeneration:     2,
		FinalEnvironmentSequence: 1,
		FinalEnvelopeDigest:      envelope.EnvelopeDigest,
		RetirementID:             testDigest(0x56),
		RetirementBytes:          []byte("opaque-admin-signed-final-retirement"),
	}
	cases := []struct {
		name string
		edit func(*relay.Retirement)
		want error
	}{
		{name: "relay generation", edit: func(value *relay.Retirement) { value.RelayGeneration = testRelayGeneration(0x57) }, want: relay.ErrGenerationMismatch},
		{name: "certificate", edit: func(value *relay.Retirement) { value.CertificateID = testDigest(0x58) }, want: relay.ErrImmutableConflict},
		{name: "producer head", edit: func(value *relay.Retirement) { value.FinalEnvironmentSequence = 2 }, want: relay.ErrImmutableConflict},
		{name: "producer digest", edit: func(value *relay.Retirement) { value.FinalEnvelopeDigest = testDigest(0x59) }, want: relay.ErrImmutableConflict},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			candidate := retirement
			testCase.edit(&candidate)
			if _, err := store.RetireEnvironment(t.Context(), relay.RetireEnvironmentRequest{
				Authorization: owner,
				Retirement:    candidate,
			}); !errors.Is(err, testCase.want) {
				t.Fatalf("RetireEnvironment(mismatch) error = %v, want %v", err, testCase.want)
			}
		})
	}
	if verifier.retirementCalls != 0 {
		t.Fatalf("retirement verifier calls for clear-field mismatches = %d, want 0", verifier.retirementCalls)
	}
	retired, err := store.RetireEnvironment(t.Context(), relay.RetireEnvironmentRequest{
		Authorization: owner,
		Retirement:    retirement,
	})
	if err != nil {
		t.Fatalf("RetireEnvironment(exact fence) error = %v", err)
	}
	replayed, err := store.RetireEnvironment(t.Context(), relay.RetireEnvironmentRequest{
		Authorization: owner,
		Retirement:    retirement,
	})
	if err != nil || replayed != retired {
		t.Fatalf("RetireEnvironment(exact retry) = %#v, %v, want %#v", replayed, err, retired)
	}
	if verifier.retirementCalls != 1 {
		t.Fatalf("retirement verifier calls = %d, want 1", verifier.retirementCalls)
	}
	conflictingRetry := retirement
	conflictingRetry.FinalEnvelopeDigest = testDigest(0x5d)
	if _, err := store.RetireEnvironment(t.Context(), relay.RetireEnvironmentRequest{
		Authorization: owner,
		Retirement:    conflictingRetry,
	}); !errors.Is(err, relay.ErrImmutableConflict) {
		t.Fatalf("RetireEnvironment(conflicting final-fence retry) error = %v, want ErrImmutableConflict", err)
	}
	if _, err := store.Append(t.Context(), relay.AppendRequest{Authorization: environment, Envelope: envelope}); !errors.Is(err, relay.ErrRetired) {
		t.Fatalf("Append(exact post-retirement retry) error = %v, want ErrRetired", err)
	}
	next := testEnvelope(environment, "fact-b", 2, envelope.EnvelopeDigest, 0x5a)
	if _, err := store.Append(t.Context(), relay.AppendRequest{Authorization: environment, Envelope: next}); !errors.Is(err, relay.ErrRetired) {
		t.Fatalf("Append(new post-retirement envelope) error = %v, want ErrRetired", err)
	}

	page, err := store.EnvironmentInventory(t.Context(), relay.EnvironmentInventoryRequest{
		Authorization: relay.InventoryAuthorization{Owner: &owner},
		Limit:         1,
	})
	if err != nil {
		t.Fatalf("EnvironmentInventory() error = %v", err)
	}
	if len(page.Environments) != 1 || page.Environments[0].Retirement == nil ||
		page.Environments[0].Retirement.RelayGeneration != retirement.RelayGeneration ||
		page.Environments[0].Retirement.CertificateID != retirement.CertificateID ||
		page.Environments[0].Retirement.FinalEnvironmentSequence != retirement.FinalEnvironmentSequence ||
		page.Environments[0].Retirement.FinalEnvelopeDigest != retirement.FinalEnvelopeDigest {
		t.Fatalf("EnvironmentInventory(retirement) = %#v, want exact final fence", page)
	}
}

func TestRelayEmptyProducerFenceAndExpiredUnfencedResidualState(t *testing.T) {
	t.Parallel()

	store, owner, environment := newTestStoreWithEnvironment(t, relay.EphemeralEnvironment, 2_000)
	store.now = func() time.Time { return time.UnixMilli(2_000) }
	newEnvelope := testEnvelope(environment, "fact-after-expiry", 1, relay.Digest{}, 0x5b)
	if _, err := store.Append(t.Context(), relay.AppendRequest{Authorization: environment, Envelope: newEnvelope}); !errors.Is(err, relay.ErrExpired) {
		t.Fatalf("Append(expired unfenced producer) error = %v, want ErrExpired", err)
	}
	beforeFence, err := store.EnvironmentInventory(t.Context(), relay.EnvironmentInventoryRequest{
		Authorization: relay.InventoryAuthorization{Owner: &owner},
		Limit:         1,
	})
	if err != nil {
		t.Fatalf("EnvironmentInventory(unfenced expired producer) error = %v", err)
	}
	if len(beforeFence.Environments) != 1 || beforeFence.Environments[0].Retirement != nil {
		t.Fatalf("expired producer inventory = %#v, want explicit absence of a trusted final fence", beforeFence)
	}
	retirement := relay.Retirement{
		ChannelID:            owner.ChannelID,
		RelayGeneration:      owner.RelayGeneration,
		EnvironmentID:        environment.EnvironmentID,
		CertificateID:        environment.CertificateID,
		MembershipGeneration: 2,
		RetirementID:         testDigest(0x5c),
		RetirementBytes:      []byte("opaque-admin-signed-empty-final-retirement"),
	}
	if _, err := store.RetireEnvironment(t.Context(), relay.RetireEnvironmentRequest{
		Authorization: owner,
		Retirement:    retirement,
	}); err != nil {
		t.Fatalf("RetireEnvironment(empty expired producer) error = %v", err)
	}
	afterFence, err := store.EnvironmentInventory(t.Context(), relay.EnvironmentInventoryRequest{
		Authorization: relay.InventoryAuthorization{Owner: &owner},
		Limit:         1,
	})
	if err != nil {
		t.Fatalf("EnvironmentInventory(fenced empty producer) error = %v", err)
	}
	got := afterFence.Environments[0].Retirement
	if got == nil || got.FinalEnvironmentSequence != 0 || got.FinalEnvelopeDigest != (relay.Digest{}) {
		t.Fatalf("empty producer final fence = %#v, want zero sequence and digest", got)
	}
}

func TestRelayEnvironmentTokenExpiryPrecedesCertificateExpiry(t *testing.T) {
	t.Parallel()

	store, err := Open(filepath.Join(t.TempDir(), "relay.sqlite"), allowTestVerifier{})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { closeTestStore(t, store) })
	store.now = func() time.Time { return time.UnixMilli(1_000) }
	owner := testOwnerAuthorization(store)
	if _, err := store.CreateChannel(t.Context(), testChannel(owner)); err != nil {
		t.Fatalf("CreateChannel() error = %v", err)
	}
	environment := testEnvironmentAuthorization(owner, "environment-a", 0x61, 0x91)
	registration := testEnvironment(environment, relay.EphemeralEnvironment, 3_000, 1)
	registration.RelayTokenExpiresAtMillis = 2_000
	if _, err := store.RegisterEnvironment(t.Context(), relay.RegisterEnvironmentRequest{
		Authorization: owner,
		Environment:   registration,
	}); err != nil {
		t.Fatalf("RegisterEnvironment() error = %v", err)
	}
	envelope := testEnvelope(environment, "fact-a", 1, relay.Digest{}, 0x62)
	if _, err := store.Append(t.Context(), relay.AppendRequest{Authorization: environment, Envelope: envelope}); err != nil {
		t.Fatalf("Append(before token expiry) error = %v", err)
	}

	store.now = func() time.Time { return time.UnixMilli(2_000) }
	if _, err := store.Append(t.Context(), relay.AppendRequest{Authorization: environment, Envelope: envelope}); !errors.Is(err, relay.ErrExpired) {
		t.Fatalf("Append(at token expiry) error = %v, want ErrExpired", err)
	}
	if _, err := store.Page(t.Context(), relay.PageRequest{Authorization: environment, Limit: 1}); !errors.Is(err, relay.ErrExpired) {
		t.Fatalf("Page(at token expiry) error = %v, want ErrExpired", err)
	}
	if err := store.Acknowledge(t.Context(), relay.AcknowledgeRequest{
		Authorization: environment,
		Acknowledgement: relay.Acknowledgement{
			ChannelID:              owner.ChannelID,
			EnvironmentID:          environment.EnvironmentID,
			MembershipGeneration:   1,
			AppliedArrivalSequence: 1,
			ProducerSequence:       1,
			ProducerEnvelopeDigest: envelope.EnvelopeDigest,
			CertificateID:          environment.CertificateID,
			AcknowledgementDigest:  testDigest(0x63),
			AcknowledgementBytes:   []byte("opaque-signed-acknowledgement"),
		},
	}); !errors.Is(err, relay.ErrExpired) {
		t.Fatalf("Acknowledge(at token expiry) error = %v, want ErrExpired", err)
	}
	if _, err := store.EnvironmentInventory(t.Context(), relay.EnvironmentInventoryRequest{
		Authorization: relay.InventoryAuthorization{Environment: &environment},
		Limit:         1,
	}); !errors.Is(err, relay.ErrExpired) {
		t.Fatalf("EnvironmentInventory(at token expiry) error = %v, want ErrExpired", err)
	}
	if _, err := store.RegisterEnvironment(t.Context(), relay.RegisterEnvironmentRequest{
		Authorization: owner,
		Environment:   registration,
	}); !errors.Is(err, relay.ErrExpired) {
		t.Fatalf("RegisterEnvironment(replay at token expiry) error = %v, want ErrExpired", err)
	}
}

func TestRelayAcknowledgementsGatePruneAndTombstonesPreserveOpaqueIdentity(t *testing.T) {
	t.Parallel()

	verifier := &gatedTestVerifier{}
	store, owner, environmentA := newTestStoreWithEnvironmentAndVerifier(t, relay.TrustedEnvironment, 0, verifier)
	environmentB := testEnvironmentAuthorization(owner, "environment-b", 0x62, 0x82)
	if _, err := store.RegisterEnvironment(t.Context(), relay.RegisterEnvironmentRequest{
		Authorization: owner,
		Environment:   testEnvironment(environmentB, relay.TrustedEnvironment, 0, 2),
	}); err != nil {
		t.Fatalf("RegisterEnvironment(B) error = %v", err)
	}

	first := testEnvelope(environmentA, "fact-a", 1, relay.Digest{}, 0x63)
	firstResult, err := store.Append(t.Context(), relay.AppendRequest{Authorization: environmentA, Envelope: first})
	if err != nil {
		t.Fatalf("Append(A) error = %v", err)
	}
	second := testEnvelope(environmentB, "fact-b", 1, relay.Digest{}, 0x64)
	secondResult, err := store.Append(t.Context(), relay.AppendRequest{Authorization: environmentB, Envelope: second})
	if err != nil {
		t.Fatalf("Append(B) error = %v", err)
	}

	prune := relay.TombstoneRequest{
		Authorization: owner,
		Certificate: relay.PruneCertificate{
			ChannelID:            owner.ChannelID,
			PruneID:              testDigest(0x71),
			MembershipGeneration: 2,
			Barrier:              2,
			Closure:              testPruneTarget(secondResult.Arrival),
			CertificateID:        testDigest(0x72),
			CertificateBytes:     []byte("opaque-signed-prune-certificate"),
			Targets: []relay.PruneTarget{{
				FactID:                 first.FactID,
				EnvironmentID:          first.EnvironmentID,
				EnvironmentSequence:    first.EnvironmentSequence,
				ArrivalSequence:        firstResult.Arrival.ArrivalSequence,
				EnvelopeDigest:         first.EnvelopeDigest,
				CertificateID:          first.CertificateID,
				PreviousEnvelopeDigest: first.PreviousEnvelopeDigest,
				KeyGeneration:          first.KeyGeneration,
				Nonce:                  first.Nonce,
			}},
		},
	}
	if _, err := store.Tombstone(t.Context(), prune); !errors.Is(err, relay.ErrAcknowledgementRequired) {
		t.Fatalf("Tombstone(without acknowledgements) error = %v, want ErrAcknowledgementRequired", err)
	}

	acknowledgeTestHead(t, store, environmentA, 2, 2, 1, first.EnvelopeDigest, 0x73)
	acknowledgeTestHead(t, store, environmentB, 2, 2, 1, second.EnvelopeDigest, 0x74)
	result, err := store.Tombstone(t.Context(), prune)
	if err != nil {
		t.Fatalf("Tombstone() error = %v", err)
	}
	if result.Duplicate || result.Tombstoned != 1 || result.RelayHead != 2 {
		t.Fatalf("Tombstone() = %#v, want one new tombstone at head 2", result)
	}
	if len(verifier.pruneAuthority.Environments) != 2 || len(verifier.pruneAuthority.Acknowledgements) != 2 {
		t.Fatalf("prune authority witness counts = %d environments, %d acknowledgements, want 2 and 2",
			len(verifier.pruneAuthority.Environments), len(verifier.pruneAuthority.Acknowledgements))
	}
	if verifier.pruneAuthority.Environments[0].EnvironmentID != environmentA.EnvironmentID ||
		verifier.pruneAuthority.Environments[1].EnvironmentID != environmentB.EnvironmentID ||
		verifier.pruneAuthority.Acknowledgements[0].EnvironmentID != environmentA.EnvironmentID ||
		verifier.pruneAuthority.Acknowledgements[1].EnvironmentID != environmentB.EnvironmentID ||
		verifier.pruneAuthority.Acknowledgements[0].ProducerEnvelopeDigest != first.EnvelopeDigest ||
		verifier.pruneAuthority.Acknowledgements[1].ProducerEnvelopeDigest != second.EnvelopeDigest {
		t.Fatalf("prune authority = %#v, want sorted, index-matched active witness set", verifier.pruneAuthority)
	}

	page, err := store.Page(t.Context(), relay.PageRequest{Authorization: environmentA, Limit: 10})
	if err != nil {
		t.Fatalf("Page(after prune) error = %v", err)
	}
	got := page.Arrivals[0]
	if got.Ciphertext != nil || got.PruneID == nil || *got.PruneID != prune.Certificate.PruneID || got.EnvelopeDigest != first.EnvelopeDigest || got.CertificateID != first.CertificateID || got.ArrivalSequence != 1 || got.EnvironmentSequence != 1 {
		t.Fatalf("tombstoned arrival = %#v, want nil ciphertext and retained identity metadata", got)
	}
	if page.Arrivals[1].Ciphertext == nil {
		t.Fatal("untargeted arrival ciphertext = nil, want retained")
	}

	duplicate, err := store.Append(t.Context(), relay.AppendRequest{Authorization: environmentA, Envelope: first})
	if err != nil || duplicate.Disposition != relay.AppendDuplicate {
		t.Fatalf("Append(tombstoned exact replay) = %#v, %v, want duplicate", duplicate, err)
	}
	conflict := first
	conflict.EnvelopeDigest = testDigest(0x75)
	if _, err := store.Append(t.Context(), relay.AppendRequest{Authorization: environmentA, Envelope: conflict}); !errors.Is(err, relay.ErrImmutableConflict) {
		t.Fatalf("Append(tombstoned conflict) error = %v, want ErrImmutableConflict", err)
	}

	duplicatePrune, err := store.Tombstone(t.Context(), prune)
	if err != nil || !duplicatePrune.Duplicate || duplicatePrune.Tombstoned != 1 {
		t.Fatalf("Tombstone(exact replay) = %#v, %v, want duplicate", duplicatePrune, err)
	}
}

func TestRelayTombstoneRejectsFictionalOrPrunedClosureBeforeVerifier(t *testing.T) {
	t.Parallel()

	verifier := &gatedTestVerifier{}
	store, owner, environment := newTestStoreWithEnvironmentAndVerifier(t, relay.TrustedEnvironment, 0, verifier)
	targetEnvelope := testEnvelope(environment, "fact-target", 1, relay.Digest{}, 0x76)
	targetResult, err := store.Append(t.Context(), relay.AppendRequest{Authorization: environment, Envelope: targetEnvelope})
	if err != nil {
		t.Fatalf("Append(target) error = %v", err)
	}
	closureEnvelope := testEnvelope(environment, "fact-close", 2, targetEnvelope.EnvelopeDigest, 0x77)
	closureResult, err := store.Append(t.Context(), relay.AppendRequest{Authorization: environment, Envelope: closureEnvelope})
	if err != nil {
		t.Fatalf("Append(closure) error = %v", err)
	}
	acknowledgeTestHead(t, store, environment, 1, 2, 2, closureEnvelope.EnvelopeDigest, 0x78)

	target := testPruneTarget(targetResult.Arrival)
	closure := testPruneTarget(closureResult.Arrival)
	certificate := relay.PruneCertificate{
		ChannelID:            owner.ChannelID,
		PruneID:              testDigest(0x79),
		MembershipGeneration: 1,
		Barrier:              2,
		Closure:              closure,
		CertificateID:        testDigest(0x7a),
		CertificateBytes:     []byte("opaque-signed-prune-certificate-with-closure"),
		Targets:              []relay.PruneTarget{target},
	}
	fictional := certificate
	fictional.Closure.FactID = "fact-fictional-close"
	if _, err := store.Tombstone(t.Context(), relay.TombstoneRequest{Authorization: owner, Certificate: fictional}); !errors.Is(err, relay.ErrImmutableConflict) {
		t.Fatalf("Tombstone(fictional closure) error = %v, want ErrImmutableConflict", err)
	}
	if verifier.pruneCalls != 0 {
		t.Fatalf("prune verifier calls after fictional closure = %d, want 0", verifier.pruneCalls)
	}
	assertTableCount(t, store, "relay_prune_certificates", 0)
	assertArrivalCiphertextRetained(t, store, owner.ChannelID, target.ArrivalSequence)

	result, err := store.Tombstone(t.Context(), relay.TombstoneRequest{Authorization: owner, Certificate: certificate})
	if err != nil {
		t.Fatalf("Tombstone(valid closure) error = %v", err)
	}
	if result.Duplicate || result.Tombstoned != 1 || verifier.pruneCertificate.Closure != closure {
		t.Fatalf("Tombstone(valid closure) = %#v, verifier certificate %#v", result, verifier.pruneCertificate)
	}
	assertArrivalCiphertextRetained(t, store, owner.ChannelID, closure.ArrivalSequence)

	inventory, err := store.PruneInventory(t.Context(), relay.PruneInventoryRequest{
		Authorization: relay.InventoryAuthorization{Owner: &owner},
		Limit:         1,
	})
	if err != nil {
		t.Fatalf("PruneInventory() error = %v", err)
	}
	if len(inventory.Prunes) != 1 || inventory.Prunes[0].Certificate.Closure != closure {
		t.Fatalf("PruneInventory() = %#v, want exact closure %#v", inventory, closure)
	}

	changedReplay := certificate
	changedReplay.Closure.EnvelopeDigest[0] ^= 0xff
	if _, err := store.Tombstone(t.Context(), relay.TombstoneRequest{Authorization: owner, Certificate: changedReplay}); !errors.Is(err, relay.ErrImmutableConflict) {
		t.Fatalf("Tombstone(changed closure replay) error = %v, want ErrImmutableConflict", err)
	}

	thirdEnvelope := testEnvelope(environment, "fact-third", 3, closureEnvelope.EnvelopeDigest, 0x7b)
	thirdResult, err := store.Append(t.Context(), relay.AppendRequest{Authorization: environment, Envelope: thirdEnvelope})
	if err != nil {
		t.Fatalf("Append(third) error = %v", err)
	}
	acknowledgeTestHead(t, store, environment, 1, 3, 3, thirdEnvelope.EnvelopeDigest, 0x7c)
	priorClosureTarget := relay.PruneCertificate{
		ChannelID:            owner.ChannelID,
		PruneID:              testDigest(0x7d),
		MembershipGeneration: 1,
		Barrier:              3,
		Closure:              testPruneTarget(thirdResult.Arrival),
		CertificateID:        testDigest(0x7e),
		CertificateBytes:     []byte("opaque-prune-that-targets-a-prior-closure"),
		Targets:              []relay.PruneTarget{closure},
	}
	if _, err := store.Tombstone(t.Context(), relay.TombstoneRequest{Authorization: owner, Certificate: priorClosureTarget}); !errors.Is(err, relay.ErrImmutableConflict) {
		t.Fatalf("Tombstone(prior closure target) error = %v, want ErrImmutableConflict", err)
	}
	if verifier.pruneCalls != 1 {
		t.Fatalf("prune verifier calls after prior closure target = %d, want 1", verifier.pruneCalls)
	}
	assertArrivalCiphertextRetained(t, store, owner.ChannelID, closure.ArrivalSequence)

	prunedClosure := relay.PruneCertificate{
		ChannelID:            owner.ChannelID,
		PruneID:              testDigest(0x7f),
		MembershipGeneration: 1,
		Barrier:              3,
		Closure:              target,
		CertificateID:        testDigest(0x80),
		CertificateBytes:     []byte("opaque-signed-prune-certificate-with-pruned-closure"),
		Targets:              []relay.PruneTarget{testPruneTarget(thirdResult.Arrival)},
	}
	if _, err := store.Tombstone(t.Context(), relay.TombstoneRequest{Authorization: owner, Certificate: prunedClosure}); !errors.Is(err, relay.ErrImmutableConflict) {
		t.Fatalf("Tombstone(pruned closure) error = %v, want ErrImmutableConflict", err)
	}
	if verifier.pruneCalls != 1 {
		t.Fatalf("prune verifier calls after invalid closure attempts = %d, want 1", verifier.pruneCalls)
	}
	assertArrivalCiphertextRetained(t, store, owner.ChannelID, thirdResult.Arrival.ArrivalSequence)
}

func TestRelayControlMutationsAreIdempotentAfterLostResponses(t *testing.T) {
	t.Parallel()

	verifier := &gatedTestVerifier{}
	store, err := Open(filepath.Join(t.TempDir(), "relay.sqlite"), verifier)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { closeTestStore(t, store) })
	owner := testOwnerAuthorization(store)
	channel := testChannel(owner)
	created, err := store.CreateChannel(t.Context(), channel)
	if err != nil {
		t.Fatalf("CreateChannel(first) error = %v", err)
	}
	replayedCreate, err := store.CreateChannel(t.Context(), channel)
	if err != nil || replayedCreate != created {
		t.Fatalf("CreateChannel(replay) = %#v, %v, want %#v", replayedCreate, err, created)
	}
	changedChannel := channel
	changedChannel.AdminPublicKey = testPublicKey(0xf0)
	if _, err := store.CreateChannel(t.Context(), changedChannel); !errors.Is(err, relay.ErrImmutableConflict) {
		t.Fatalf("CreateChannel(conflict) error = %v, want ErrImmutableConflict", err)
	}
	wrongGeneration := channel
	wrongGeneration.ChannelID = testChannelID(0xf1)
	wrongGeneration.RelayGeneration = testRelayGeneration(0xf2)
	if _, err := store.CreateChannel(t.Context(), wrongGeneration); !errors.Is(err, relay.ErrGenerationMismatch) {
		t.Fatalf("CreateChannel(wrong database incarnation) error = %v, want ErrGenerationMismatch", err)
	}

	authorization := testEnvironmentAuthorization(owner, "environment-a", 0x21, 0x81)
	registration := relay.RegisterEnvironmentRequest{
		Authorization: owner,
		Environment:   testEnvironment(authorization, relay.TrustedEnvironment, 0, 1),
	}
	registered, err := store.RegisterEnvironment(t.Context(), registration)
	if err != nil {
		t.Fatalf("RegisterEnvironment(first) error = %v", err)
	}
	replayedRegistration, err := store.RegisterEnvironment(t.Context(), registration)
	if err != nil || replayedRegistration != registered {
		t.Fatalf("RegisterEnvironment(replay) = %#v, %v, want %#v", replayedRegistration, err, registered)
	}
	if verifier.certificateCalls != 1 {
		t.Fatalf("certificate verifier calls = %d, want 1 for first acceptance only", verifier.certificateCalls)
	}
	changedRegistration := registration
	changedRegistration.Environment.CertificateBytes = []byte("different-signed-certificate")
	if _, err := store.RegisterEnvironment(t.Context(), changedRegistration); !errors.Is(err, relay.ErrImmutableConflict) {
		t.Fatalf("RegisterEnvironment(conflict) error = %v, want ErrImmutableConflict", err)
	}

	retirement := relay.RetireEnvironmentRequest{
		Authorization: owner,
		Retirement: relay.Retirement{
			ChannelID:            owner.ChannelID,
			RelayGeneration:      owner.RelayGeneration,
			EnvironmentID:        authorization.EnvironmentID,
			CertificateID:        authorization.CertificateID,
			MembershipGeneration: 2,
			RetirementID:         testDigest(0xa1),
			RetirementBytes:      []byte("opaque-signed-retirement"),
		},
	}
	retired, err := store.RetireEnvironment(t.Context(), retirement)
	if err != nil {
		t.Fatalf("RetireEnvironment(first) error = %v", err)
	}
	replayedRetirement, err := store.RetireEnvironment(t.Context(), retirement)
	if err != nil || replayedRetirement != retired {
		t.Fatalf("RetireEnvironment(replay) = %#v, %v, want %#v", replayedRetirement, err, retired)
	}
	if verifier.retirementCalls != 1 {
		t.Fatalf("retirement verifier calls = %d, want 1 for first acceptance only", verifier.retirementCalls)
	}
	changedRetirement := retirement
	changedRetirement.Retirement.RetirementBytes = []byte("different-signed-retirement")
	if _, err := store.RetireEnvironment(t.Context(), changedRetirement); !errors.Is(err, relay.ErrImmutableConflict) {
		t.Fatalf("RetireEnvironment(conflict) error = %v, want ErrImmutableConflict", err)
	}
}

func TestRelayVerifierRefusalCannotReachPersistence(t *testing.T) {
	t.Parallel()

	verifier := &gatedTestVerifier{rejectCertificate: true}
	store, err := Open(filepath.Join(t.TempDir(), "relay.sqlite"), verifier)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { closeTestStore(t, store) })
	owner := testOwnerAuthorization(store)
	if _, err := store.CreateChannel(t.Context(), testChannel(owner)); err != nil {
		t.Fatalf("CreateChannel() error = %v", err)
	}
	authorization := testEnvironmentAuthorization(owner, "environment-a", 0x21, 0x81)
	registration := relay.RegisterEnvironmentRequest{
		Authorization: owner,
		Environment:   testEnvironment(authorization, relay.TrustedEnvironment, 0, 1),
	}
	if _, err := store.RegisterEnvironment(t.Context(), registration); !errors.Is(err, relay.ErrUnverified) {
		t.Fatalf("RegisterEnvironment(rejected) error = %v, want ErrUnverified", err)
	}
	assertTableCount(t, store, "relay_environments", 0)

	verifier.rejectCertificate = false
	if _, err := store.RegisterEnvironment(t.Context(), registration); err != nil {
		t.Fatalf("RegisterEnvironment(allowed) error = %v", err)
	}
	envelope := testEnvelope(authorization, "fact-a", 1, relay.Digest{}, 0xb1)
	verifier.rejectEnvelope = true
	if _, err := store.Append(t.Context(), relay.AppendRequest{Authorization: authorization, Envelope: envelope}); !errors.Is(err, relay.ErrUnverified) {
		t.Fatalf("Append(rejected) error = %v, want ErrUnverified", err)
	}
	assertTableCount(t, store, "relay_arrivals", 0)

	verifier.rejectEnvelope = false
	accepted, err := store.Append(t.Context(), relay.AppendRequest{Authorization: authorization, Envelope: envelope})
	if err != nil {
		t.Fatalf("Append(allowed) error = %v", err)
	}
	closureEnvelope := testEnvelope(authorization, "fact-close", 2, envelope.EnvelopeDigest, 0xb6)
	closureAccepted, err := store.Append(t.Context(), relay.AppendRequest{Authorization: authorization, Envelope: closureEnvelope})
	if err != nil {
		t.Fatalf("Append(closure) error = %v", err)
	}
	acknowledgement := relay.AcknowledgeRequest{
		Authorization: authorization,
		Acknowledgement: relay.Acknowledgement{
			ChannelID:              owner.ChannelID,
			EnvironmentID:          authorization.EnvironmentID,
			MembershipGeneration:   1,
			AppliedArrivalSequence: 2,
			ProducerSequence:       2,
			ProducerEnvelopeDigest: closureEnvelope.EnvelopeDigest,
			CertificateID:          authorization.CertificateID,
			AcknowledgementDigest:  testDigest(0xb2),
			AcknowledgementBytes:   []byte("opaque-signed-acknowledgement"),
		},
	}
	verifier.rejectAcknowledgement = true
	if err := store.Acknowledge(t.Context(), acknowledgement); !errors.Is(err, relay.ErrUnverified) {
		t.Fatalf("Acknowledge(rejected) error = %v, want ErrUnverified", err)
	}
	assertTableCount(t, store, "relay_acknowledgements", 0)
	verifier.rejectAcknowledgement = false
	if err := store.Acknowledge(t.Context(), acknowledgement); err != nil {
		t.Fatalf("Acknowledge(allowed) error = %v", err)
	}

	prune := relay.TombstoneRequest{
		Authorization: owner,
		Certificate: relay.PruneCertificate{
			ChannelID:            owner.ChannelID,
			PruneID:              testDigest(0xb3),
			MembershipGeneration: 1,
			Barrier:              2,
			Closure:              testPruneTarget(closureAccepted.Arrival),
			CertificateID:        testDigest(0xb4),
			CertificateBytes:     []byte("opaque-signed-prune-certificate"),
			Targets: []relay.PruneTarget{{
				FactID:                 envelope.FactID,
				EnvironmentID:          envelope.EnvironmentID,
				EnvironmentSequence:    envelope.EnvironmentSequence,
				ArrivalSequence:        accepted.Arrival.ArrivalSequence,
				EnvelopeDigest:         envelope.EnvelopeDigest,
				CertificateID:          envelope.CertificateID,
				PreviousEnvelopeDigest: envelope.PreviousEnvelopeDigest,
				KeyGeneration:          envelope.KeyGeneration,
				Nonce:                  envelope.Nonce,
			}},
		},
	}
	verifier.rejectPrune = true
	if _, err := store.Tombstone(t.Context(), prune); !errors.Is(err, relay.ErrUnverified) {
		t.Fatalf("Tombstone(rejected) error = %v, want ErrUnverified", err)
	}
	authority := verifier.pruneAuthority
	if authority.Channel.ChannelID != owner.ChannelID || authority.Channel.RelayGeneration != owner.RelayGeneration ||
		len(authority.Environments) != 1 || len(authority.Acknowledgements) != 1 {
		t.Fatalf("prune authority = %#v, want channel and one matched active witness", authority)
	}
	gotEnvironment := authority.Environments[0]
	if gotEnvironment.EnvironmentID != authorization.EnvironmentID ||
		gotEnvironment.CertificateID != registration.Environment.CertificateID ||
		!bytes.Equal(gotEnvironment.CertificateBytes, registration.Environment.CertificateBytes) ||
		gotEnvironment.Mode != registration.Environment.Mode ||
		gotEnvironment.ExpiresAtMillis != registration.Environment.ExpiresAtMillis ||
		gotEnvironment.RelayTokenExpiresAtMillis != registration.Environment.RelayTokenExpiresAtMillis ||
		gotEnvironment.MembershipGeneration != registration.Environment.MembershipGeneration {
		t.Fatalf("prune environment authority = %#v, want exact registered authority", gotEnvironment)
	}
	gotAcknowledgement := authority.Acknowledgements[0]
	wantAcknowledgement := acknowledgement.Acknowledgement
	if gotAcknowledgement.ChannelID != wantAcknowledgement.ChannelID ||
		gotAcknowledgement.EnvironmentID != wantAcknowledgement.EnvironmentID ||
		gotAcknowledgement.MembershipGeneration != wantAcknowledgement.MembershipGeneration ||
		gotAcknowledgement.AppliedArrivalSequence != wantAcknowledgement.AppliedArrivalSequence ||
		gotAcknowledgement.ProducerSequence != wantAcknowledgement.ProducerSequence ||
		gotAcknowledgement.ProducerEnvelopeDigest != wantAcknowledgement.ProducerEnvelopeDigest ||
		gotAcknowledgement.CertificateID != wantAcknowledgement.CertificateID ||
		gotAcknowledgement.AcknowledgementDigest != wantAcknowledgement.AcknowledgementDigest ||
		!bytes.Equal(gotAcknowledgement.AcknowledgementBytes, wantAcknowledgement.AcknowledgementBytes) {
		t.Fatalf("prune acknowledgement = %#v, want exact latest acknowledgement %#v", gotAcknowledgement, wantAcknowledgement)
	}
	assertTableCount(t, store, "relay_prune_certificates", 0)
	var ciphertext []byte
	if err := store.db.QueryRow(`SELECT ciphertext FROM relay_arrivals WHERE channel_id = ? AND arrival_sequence = 1`, owner.ChannelID[:]).Scan(&ciphertext); err != nil {
		t.Fatalf("read rejected prune arrival: %v", err)
	}
	if ciphertext == nil {
		t.Fatal("rejected prune nulled ciphertext")
	}

	retirement := relay.RetireEnvironmentRequest{
		Authorization: owner,
		Retirement: relay.Retirement{
			ChannelID:                owner.ChannelID,
			RelayGeneration:          owner.RelayGeneration,
			EnvironmentID:            authorization.EnvironmentID,
			CertificateID:            authorization.CertificateID,
			MembershipGeneration:     2,
			FinalEnvironmentSequence: 2,
			FinalEnvelopeDigest:      closureEnvelope.EnvelopeDigest,
			RetirementID:             testDigest(0xb5),
			RetirementBytes:          []byte("opaque-signed-retirement"),
		},
	}
	verifier.rejectRetirement = true
	if _, err := store.RetireEnvironment(t.Context(), retirement); !errors.Is(err, relay.ErrUnverified) {
		t.Fatalf("RetireEnvironment(rejected) error = %v, want ErrUnverified", err)
	}
	var retiredAt any
	if err := store.db.QueryRow(`SELECT retired_at_millis FROM relay_environments WHERE channel_id = ? AND environment_id = ?`, owner.ChannelID[:], string(authorization.EnvironmentID)).Scan(&retiredAt); err != nil {
		t.Fatalf("read rejected retirement: %v", err)
	}
	if retiredAt != nil {
		t.Fatalf("rejected retirement persisted retired_at = %v", retiredAt)
	}
}

func TestRelayAuthenticatedInventoryPagesExposeBoundedVerificationMaterial(t *testing.T) {
	t.Parallel()

	store, owner, environmentA := newTestStoreWithEnvironment(t, relay.TrustedEnvironment, 0)
	environmentB := testEnvironmentAuthorization(owner, "environment-b", 0x62, 0x82)
	registrationB := testEnvironment(environmentB, relay.TrustedEnvironment, 0, 2)
	registrationB.CertificateBytes = []byte("opaque-signed-environment-b-certificate")
	if _, err := store.RegisterEnvironment(t.Context(), relay.RegisterEnvironmentRequest{
		Authorization: owner,
		Environment:   registrationB,
	}); err != nil {
		t.Fatalf("RegisterEnvironment(B) error = %v", err)
	}

	first := testEnvelope(environmentA, "fact-a", 1, relay.Digest{}, 0xd1)
	accepted, err := store.Append(t.Context(), relay.AppendRequest{Authorization: environmentA, Envelope: first})
	if err != nil {
		t.Fatalf("Append(A) error = %v", err)
	}
	second := testEnvelope(environmentB, "fact-b", 1, relay.Digest{}, 0xd2)
	secondAccepted, err := store.Append(t.Context(), relay.AppendRequest{Authorization: environmentB, Envelope: second})
	if err != nil {
		t.Fatalf("Append(B) error = %v", err)
	}
	acknowledgeTestHead(t, store, environmentA, 2, 2, 1, first.EnvelopeDigest, 0xd3)
	acknowledgeTestHead(t, store, environmentB, 2, 2, 1, second.EnvelopeDigest, 0xd4)
	prune := relay.PruneCertificate{
		ChannelID:            owner.ChannelID,
		PruneID:              testDigest(0xd5),
		MembershipGeneration: 2,
		Barrier:              2,
		Closure:              testPruneTarget(secondAccepted.Arrival),
		CertificateID:        testDigest(0xd6),
		CertificateBytes:     []byte("opaque-signed-prune-certificate-for-attach"),
		Targets: []relay.PruneTarget{{
			FactID:                 first.FactID,
			EnvironmentID:          first.EnvironmentID,
			EnvironmentSequence:    first.EnvironmentSequence,
			ArrivalSequence:        accepted.Arrival.ArrivalSequence,
			EnvelopeDigest:         first.EnvelopeDigest,
			CertificateID:          first.CertificateID,
			PreviousEnvelopeDigest: first.PreviousEnvelopeDigest,
			KeyGeneration:          first.KeyGeneration,
			Nonce:                  first.Nonce,
		}},
	}
	if _, err := store.Tombstone(t.Context(), relay.TombstoneRequest{Authorization: owner, Certificate: prune}); err != nil {
		t.Fatalf("Tombstone() error = %v", err)
	}
	retirement := relay.Retirement{
		ChannelID:                owner.ChannelID,
		RelayGeneration:          owner.RelayGeneration,
		EnvironmentID:            environmentB.EnvironmentID,
		CertificateID:            environmentB.CertificateID,
		MembershipGeneration:     3,
		FinalEnvironmentSequence: 1,
		FinalEnvelopeDigest:      second.EnvelopeDigest,
		RetirementID:             testDigest(0xd7),
		RetirementBytes:          []byte("opaque-signed-environment-b-retirement"),
	}
	if _, err := store.RetireEnvironment(t.Context(), relay.RetireEnvironmentRequest{
		Authorization: owner,
		Retirement:    retirement,
	}); err != nil {
		t.Fatalf("RetireEnvironment(B) error = %v", err)
	}

	ownerInventory := relay.InventoryAuthorization{Owner: &owner}
	firstEnvironmentPage, err := store.EnvironmentInventory(t.Context(), relay.EnvironmentInventoryRequest{
		Authorization: ownerInventory,
		Limit:         1,
	})
	if err != nil {
		t.Fatalf("EnvironmentInventory(first) error = %v", err)
	}
	if firstEnvironmentPage.Channel.ChannelID != owner.ChannelID ||
		firstEnvironmentPage.Channel.RelayGeneration != owner.RelayGeneration ||
		firstEnvironmentPage.Channel.AdminPublicKey != testPublicKey(0x04) ||
		firstEnvironmentPage.Snapshot.MembershipGeneration != 3 || firstEnvironmentPage.Snapshot.ArrivalHead != 2 ||
		len(firstEnvironmentPage.Environments) != 1 || !firstEnvironmentPage.More {
		t.Fatalf("EnvironmentInventory(first) = %#v, want bounded first page and channel authority", firstEnvironmentPage)
	}
	if firstEnvironmentPage.Environments[0].EnvironmentID != environmentA.EnvironmentID ||
		firstEnvironmentPage.Environments[0].ProducerHead != 1 ||
		firstEnvironmentPage.Environments[0].Retirement != nil {
		t.Fatalf("EnvironmentInventory(first environment) = %#v", firstEnvironmentPage.Environments[0])
	}
	secondEnvironmentPage, err := store.EnvironmentInventory(t.Context(), relay.EnvironmentInventoryRequest{
		Authorization:      ownerInventory,
		AfterEnvironmentID: firstEnvironmentPage.Environments[0].EnvironmentID,
		Snapshot:           &firstEnvironmentPage.Snapshot,
		Limit:              1,
	})
	if err != nil {
		t.Fatalf("EnvironmentInventory(second) error = %v", err)
	}
	if len(secondEnvironmentPage.Environments) != 1 || secondEnvironmentPage.More {
		t.Fatalf("EnvironmentInventory(second) = %#v, want final singleton page", secondEnvironmentPage)
	}
	gotEnvironmentB := secondEnvironmentPage.Environments[0]
	if gotEnvironmentB.EnvironmentID != environmentB.EnvironmentID ||
		gotEnvironmentB.CertificateID != environmentB.CertificateID ||
		!bytes.Equal(gotEnvironmentB.CertificateBytes, registrationB.CertificateBytes) ||
		gotEnvironmentB.Retirement == nil ||
		gotEnvironmentB.Retirement.RelayGeneration != retirement.RelayGeneration ||
		gotEnvironmentB.Retirement.CertificateID != retirement.CertificateID ||
		gotEnvironmentB.Retirement.MembershipGeneration != retirement.MembershipGeneration ||
		gotEnvironmentB.Retirement.FinalEnvironmentSequence != retirement.FinalEnvironmentSequence ||
		gotEnvironmentB.Retirement.FinalEnvelopeDigest != retirement.FinalEnvelopeDigest ||
		gotEnvironmentB.Retirement.RetirementID != retirement.RetirementID ||
		!bytes.Equal(gotEnvironmentB.Retirement.RetirementBytes, retirement.RetirementBytes) {
		t.Fatalf("EnvironmentInventory(B) = %#v, want signed certificate and terminal retirement", gotEnvironmentB)
	}

	environmentInventory := relay.InventoryAuthorization{Environment: &environmentA}
	prunePage, err := store.PruneInventory(t.Context(), relay.PruneInventoryRequest{
		Authorization: environmentInventory,
		Limit:         1,
	})
	if err != nil {
		t.Fatalf("PruneInventory() error = %v", err)
	}
	if prunePage.Channel.ChannelID != owner.ChannelID || prunePage.Snapshot.ArrivalHead != 2 ||
		prunePage.Snapshot.PruneHead != 1 || len(prunePage.Prunes) != 1 || prunePage.More {
		t.Fatalf("PruneInventory() = %#v, want one signed prune at stable prune head", prunePage)
	}
	gotPrune := prunePage.Prunes[0]
	if gotPrune.PruneSequence != 1 || gotPrune.Certificate.PruneID != prune.PruneID ||
		gotPrune.Certificate.CertificateID != prune.CertificateID ||
		gotPrune.Certificate.Closure != prune.Closure ||
		!bytes.Equal(gotPrune.Certificate.CertificateBytes, prune.CertificateBytes) ||
		len(gotPrune.Certificate.Targets) != 1 || gotPrune.Certificate.Targets[0] != prune.Targets[0] {
		t.Fatalf("PruneInventory()[0] = %#v, want exact certificate and target manifest", gotPrune)
	}

	wrongEnvironment := environmentA
	wrongEnvironment.TokenSecret = testSecret(0xee)
	if _, err := store.PruneInventory(t.Context(), relay.PruneInventoryRequest{
		Authorization: relay.InventoryAuthorization{Environment: &wrongEnvironment},
		Limit:         1,
	}); !errors.Is(err, relay.ErrUnauthenticated) {
		t.Fatalf("PruneInventory(wrong secret) error = %v, want ErrUnauthenticated", err)
	}
	if _, err := store.EnvironmentInventory(t.Context(), relay.EnvironmentInventoryRequest{
		Authorization: ownerInventory,
		Limit:         relay.MaxEnvironmentInventoryPage + 1,
	}); !errors.Is(err, relay.ErrInvalidArgument) {
		t.Fatalf("EnvironmentInventory(oversize) error = %v, want ErrInvalidArgument", err)
	}
	if _, err := store.PruneInventory(t.Context(), relay.PruneInventoryRequest{
		Authorization: environmentInventory,
		Limit:         relay.MaxPruneInventoryPage + 1,
	}); !errors.Is(err, relay.ErrInvalidArgument) {
		t.Fatalf("PruneInventory(oversize) error = %v, want ErrInvalidArgument", err)
	}
}

func TestRelayInventoryContinuationsPinSnapshotsAndRejectMembershipChanges(t *testing.T) {
	t.Parallel()

	store, owner, environmentA := newTestStoreWithEnvironment(t, relay.TrustedEnvironment, 0)
	environmentB := testEnvironmentAuthorization(owner, "environment-b", 0x42, 0x82)
	if _, err := store.RegisterEnvironment(t.Context(), relay.RegisterEnvironmentRequest{
		Authorization: owner,
		Environment:   testEnvironment(environmentB, relay.TrustedEnvironment, 0, 2),
	}); err != nil {
		t.Fatalf("RegisterEnvironment(B) error = %v", err)
	}
	emptySnapshotPage, err := store.EnvironmentInventory(t.Context(), relay.EnvironmentInventoryRequest{
		Authorization: relay.InventoryAuthorization{Owner: &owner},
		Limit:         1,
	})
	if err != nil {
		t.Fatalf("EnvironmentInventory(empty snapshot) error = %v", err)
	}
	if emptySnapshotPage.Snapshot.ArrivalHead != 0 || !emptySnapshotPage.More {
		t.Fatalf("EnvironmentInventory(empty snapshot) = %#v, want stable zero head with continuation", emptySnapshotPage)
	}
	if _, err := store.EnvironmentInventory(t.Context(), relay.EnvironmentInventoryRequest{
		Authorization:      relay.InventoryAuthorization{Owner: &owner},
		AfterEnvironmentID: emptySnapshotPage.Environments[0].EnvironmentID,
		Limit:              1,
	}); !errors.Is(err, relay.ErrInvalidArgument) {
		t.Fatalf("EnvironmentInventory(cursor without snapshot) error = %v, want ErrInvalidArgument", err)
	}
	first := testEnvelope(environmentA, "fact-a", 1, relay.Digest{}, 0xe1)
	accepted, err := store.Append(t.Context(), relay.AppendRequest{Authorization: environmentA, Envelope: first})
	if err != nil {
		t.Fatalf("Append(A) error = %v", err)
	}
	emptySnapshotContinuation, err := store.EnvironmentInventory(t.Context(), relay.EnvironmentInventoryRequest{
		Authorization:      relay.InventoryAuthorization{Owner: &owner},
		AfterEnvironmentID: emptySnapshotPage.Environments[0].EnvironmentID,
		Snapshot:           &emptySnapshotPage.Snapshot,
		Limit:              1,
	})
	if err != nil {
		t.Fatalf("EnvironmentInventory(zero-head continuation) error = %v", err)
	}
	if emptySnapshotContinuation.Snapshot != emptySnapshotPage.Snapshot || len(emptySnapshotContinuation.Environments) != 1 ||
		emptySnapshotContinuation.Environments[0].ProducerHead != 0 {
		t.Fatalf("EnvironmentInventory(zero-head continuation) = %#v, want pinned zero frontier", emptySnapshotContinuation)
	}

	ownerInventory := relay.InventoryAuthorization{Owner: &owner}
	firstEnvironmentPage, err := store.EnvironmentInventory(t.Context(), relay.EnvironmentInventoryRequest{
		Authorization: ownerInventory,
		Limit:         1,
	})
	if err != nil {
		t.Fatalf("EnvironmentInventory(first) error = %v", err)
	}
	if firstEnvironmentPage.Snapshot.ArrivalHead != 1 || firstEnvironmentPage.Snapshot.MembershipGeneration != 2 || !firstEnvironmentPage.More {
		t.Fatalf("EnvironmentInventory(first) = %#v, want head 1, membership 2, more", firstEnvironmentPage)
	}
	second := testEnvelope(environmentB, "fact-b", 1, relay.Digest{}, 0xe2)
	secondAccepted, err := store.Append(t.Context(), relay.AppendRequest{Authorization: environmentB, Envelope: second})
	if err != nil {
		t.Fatalf("Append(B between inventory pages) error = %v", err)
	}
	continuedEnvironmentPage, err := store.EnvironmentInventory(t.Context(), relay.EnvironmentInventoryRequest{
		Authorization:      ownerInventory,
		AfterEnvironmentID: firstEnvironmentPage.Environments[0].EnvironmentID,
		Snapshot:           &firstEnvironmentPage.Snapshot,
		Limit:              1,
	})
	if err != nil {
		t.Fatalf("EnvironmentInventory(continued) error = %v", err)
	}
	if continuedEnvironmentPage.Snapshot != firstEnvironmentPage.Snapshot || len(continuedEnvironmentPage.Environments) != 1 ||
		continuedEnvironmentPage.Environments[0].EnvironmentID != environmentB.EnvironmentID ||
		continuedEnvironmentPage.Environments[0].ProducerHead != 0 {
		t.Fatalf("EnvironmentInventory(continued) = %#v, want producer heads bounded through arrival 1", continuedEnvironmentPage)
	}

	third := testEnvelope(environmentA, "fact-c", 2, first.EnvelopeDigest, 0xe9)
	thirdAccepted, err := store.Append(t.Context(), relay.AppendRequest{Authorization: environmentA, Envelope: third})
	if err != nil {
		t.Fatalf("Append(A second producer envelope) error = %v", err)
	}
	acknowledgeTestHead(t, store, environmentA, 2, 3, 2, third.EnvelopeDigest, 0xe3)
	acknowledgeTestHead(t, store, environmentB, 2, 3, 1, second.EnvelopeDigest, 0xe4)
	prune := relay.PruneCertificate{
		ChannelID:            owner.ChannelID,
		PruneID:              testDigest(0xe5),
		MembershipGeneration: 2,
		Barrier:              3,
		Closure:              testPruneTarget(secondAccepted.Arrival),
		CertificateID:        testDigest(0xe6),
		CertificateBytes:     []byte("opaque-signed-prune-certificate"),
		Targets: []relay.PruneTarget{{
			FactID:                 first.FactID,
			EnvironmentID:          first.EnvironmentID,
			EnvironmentSequence:    first.EnvironmentSequence,
			ArrivalSequence:        accepted.Arrival.ArrivalSequence,
			EnvelopeDigest:         first.EnvelopeDigest,
			CertificateID:          first.CertificateID,
			PreviousEnvelopeDigest: first.PreviousEnvelopeDigest,
			KeyGeneration:          first.KeyGeneration,
			Nonce:                  first.Nonce,
		}},
	}
	if _, err := store.Tombstone(t.Context(), relay.TombstoneRequest{Authorization: owner, Certificate: prune}); err != nil {
		t.Fatalf("Tombstone() error = %v", err)
	}
	secondPrune := relay.PruneCertificate{
		ChannelID:            owner.ChannelID,
		PruneID:              testDigest(0xe7),
		MembershipGeneration: 2,
		Barrier:              3,
		Closure:              testPruneTarget(secondAccepted.Arrival),
		CertificateID:        testDigest(0xe8),
		CertificateBytes:     []byte("second-opaque-signed-prune-certificate"),
		Targets:              []relay.PruneTarget{testPruneTarget(thirdAccepted.Arrival)},
	}
	if _, err := store.Tombstone(t.Context(), relay.TombstoneRequest{Authorization: owner, Certificate: secondPrune}); err != nil {
		t.Fatalf("Tombstone(second) error = %v", err)
	}
	firstPrunePage, err := store.PruneInventory(t.Context(), relay.PruneInventoryRequest{
		Authorization: relay.InventoryAuthorization{Environment: &environmentA},
		Limit:         1,
	})
	if err != nil {
		t.Fatalf("PruneInventory(first) error = %v", err)
	}
	if !firstPrunePage.More || firstPrunePage.Snapshot.ArrivalHead != 3 || firstPrunePage.Snapshot.PruneHead != 2 {
		t.Fatalf("PruneInventory(first) = %#v, want pinned arrival/prune heads and continuation", firstPrunePage)
	}
	fourth := testEnvelope(environmentA, "fact-d", 3, third.EnvelopeDigest, 0xea)
	if _, err := store.Append(t.Context(), relay.AppendRequest{Authorization: environmentA, Envelope: fourth}); err != nil {
		t.Fatalf("Append(A between prune inventory pages) error = %v", err)
	}
	continuedPrunePage, err := store.PruneInventory(t.Context(), relay.PruneInventoryRequest{
		Authorization: relay.InventoryAuthorization{Environment: &environmentA},
		After:         firstPrunePage.Prunes[0].PruneSequence,
		Snapshot:      &firstPrunePage.Snapshot,
		Limit:         1,
	})
	if err != nil {
		t.Fatalf("PruneInventory(continued) error = %v", err)
	}
	if continuedPrunePage.Snapshot != firstPrunePage.Snapshot || len(continuedPrunePage.Prunes) != 1 || continuedPrunePage.More {
		t.Fatalf("PruneInventory(continued) = %#v, want stable snapshot and final prune", continuedPrunePage)
	}

	environmentC := testEnvironmentAuthorization(owner, "environment-c", 0x52, 0x92)
	if _, err := store.RegisterEnvironment(t.Context(), relay.RegisterEnvironmentRequest{
		Authorization: owner,
		Environment:   testEnvironment(environmentC, relay.TrustedEnvironment, 0, 3),
	}); err != nil {
		t.Fatalf("RegisterEnvironment(C) error = %v", err)
	}
	if _, err := store.EnvironmentInventory(t.Context(), relay.EnvironmentInventoryRequest{
		Authorization:      ownerInventory,
		AfterEnvironmentID: firstEnvironmentPage.Environments[0].EnvironmentID,
		Snapshot:           &firstEnvironmentPage.Snapshot,
		Limit:              1,
	}); !errors.Is(err, relay.ErrMembershipChanged) {
		t.Fatalf("EnvironmentInventory(after membership change) error = %v, want ErrMembershipChanged", err)
	}
	if _, err := store.PruneInventory(t.Context(), relay.PruneInventoryRequest{
		Authorization: relay.InventoryAuthorization{Environment: &environmentA},
		After:         firstPrunePage.Snapshot.PruneHead,
		Snapshot:      &firstPrunePage.Snapshot,
		Limit:         1,
	}); !errors.Is(err, relay.ErrMembershipChanged) {
		t.Fatalf("PruneInventory(after membership change) error = %v, want ErrMembershipChanged", err)
	}
}

func TestRelayPublicMethodsRejectNilAndCancelledContexts(t *testing.T) {
	t.Parallel()

	store, err := Open(filepath.Join(t.TempDir(), "relay.sqlite"), allowTestVerifier{})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { closeTestStore(t, store) })
	calls := []struct {
		name string
		call func(context.Context) error
	}{
		{name: "create", call: func(ctx context.Context) error { _, err := store.CreateChannel(ctx, relay.Channel{}); return err }},
		{name: "register", call: func(ctx context.Context) error {
			_, err := store.RegisterEnvironment(ctx, relay.RegisterEnvironmentRequest{})
			return err
		}},
		{name: "retire", call: func(ctx context.Context) error {
			_, err := store.RetireEnvironment(ctx, relay.RetireEnvironmentRequest{})
			return err
		}},
		{name: "append", call: func(ctx context.Context) error { _, err := store.Append(ctx, relay.AppendRequest{}); return err }},
		{name: "page", call: func(ctx context.Context) error { _, err := store.Page(ctx, relay.PageRequest{}); return err }},
		{name: "acknowledge", call: func(ctx context.Context) error { return store.Acknowledge(ctx, relay.AcknowledgeRequest{}) }},
		{name: "tombstone", call: func(ctx context.Context) error { _, err := store.Tombstone(ctx, relay.TombstoneRequest{}); return err }},
		{name: "environment inventory", call: func(ctx context.Context) error {
			_, err := store.EnvironmentInventory(ctx, relay.EnvironmentInventoryRequest{})
			return err
		}},
		{name: "prune inventory", call: func(ctx context.Context) error {
			_, err := store.PruneInventory(ctx, relay.PruneInventoryRequest{})
			return err
		}},
	}
	for _, call := range calls {
		call := call
		t.Run(call.name, func(t *testing.T) {
			if err := call.call(nil); !errors.Is(err, relay.ErrInvalidArgument) {
				t.Fatalf("nil context error = %v, want ErrInvalidArgument", err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			if err := call.call(ctx); !errors.Is(err, context.Canceled) {
				t.Fatalf("cancelled context error = %v, want context.Canceled", err)
			}
		})
	}
}

func TestRelaySequenceIncrementRefusesInt64Exhaustion(t *testing.T) {
	t.Parallel()

	if got, err := nextSequence(math.MaxInt64 - 1); err != nil || got != math.MaxInt64 {
		t.Fatalf("nextSequence(MaxInt64-1) = %d, %v, want MaxInt64", got, err)
	}
	if _, err := nextSequence(math.MaxInt64); !errors.Is(err, relay.ErrInvalidArgument) {
		t.Fatalf("nextSequence(MaxInt64) error = %v, want ErrInvalidArgument", err)
	}
}

func newTestStoreWithEnvironment(t *testing.T, mode relay.EnvironmentMode, expiresAtMillis int64) (*Store, relay.OwnerAuthorization, relay.EnvironmentAuthorization) {
	t.Helper()
	return newTestStoreWithEnvironmentAndVerifier(t, mode, expiresAtMillis, allowTestVerifier{})
}

func newTestStoreWithEnvironmentAndVerifier(t *testing.T, mode relay.EnvironmentMode, expiresAtMillis int64, verifier relay.Verifier) (*Store, relay.OwnerAuthorization, relay.EnvironmentAuthorization) {
	t.Helper()

	store, err := Open(filepath.Join(t.TempDir(), "relay.sqlite"), verifier)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { closeTestStore(t, store) })
	if expiresAtMillis > 0 {
		store.now = func() time.Time { return time.UnixMilli(expiresAtMillis - 1_000) }
	}
	owner := testOwnerAuthorization(store)
	if _, err := store.CreateChannel(t.Context(), testChannel(owner)); err != nil {
		t.Fatalf("CreateChannel() error = %v", err)
	}
	environment := testEnvironmentAuthorization(owner, "environment-a", 0x21, 0x81)
	if _, err := store.RegisterEnvironment(t.Context(), relay.RegisterEnvironmentRequest{
		Authorization: owner,
		Environment:   testEnvironment(environment, mode, expiresAtMillis, 1),
	}); err != nil {
		t.Fatalf("RegisterEnvironment() error = %v", err)
	}
	return store, owner, environment
}

func testOwnerAuthorization(store *Store) relay.OwnerAuthorization {
	return relay.OwnerAuthorization{
		ChannelID:       testChannelID(0x01),
		RelayGeneration: store.RelayGeneration(),
		TokenID:         testTokenID(0x05),
		TokenSecret:     testSecret(0x03),
	}
}

func testChannel(owner relay.OwnerAuthorization) relay.Channel {
	hash, err := relay.HashTokenSecret(owner.TokenSecret)
	if err != nil {
		panic(err)
	}
	return relay.Channel{
		ChannelID:       owner.ChannelID,
		RelayGeneration: owner.RelayGeneration,
		AdminPublicKey:  testPublicKey(0x04),
		OwnerToken: relay.TokenRegistration{
			TokenID:   owner.TokenID,
			TokenHash: hash,
		},
	}
}

func testEnvironmentAuthorization(owner relay.OwnerAuthorization, environmentID relay.EnvironmentID, seed, certificateSeed byte) relay.EnvironmentAuthorization {
	return relay.EnvironmentAuthorization{
		ChannelID:       owner.ChannelID,
		RelayGeneration: owner.RelayGeneration,
		EnvironmentID:   environmentID,
		CertificateID:   testDigest(certificateSeed),
		TokenID:         testTokenID(seed + 1),
		TokenSecret:     testSecret(seed),
	}
}

func testEnvironment(authorization relay.EnvironmentAuthorization, mode relay.EnvironmentMode, expiresAtMillis int64, generation uint32) relay.Environment {
	hash, err := relay.HashTokenSecret(authorization.TokenSecret)
	if err != nil {
		panic(err)
	}
	return relay.Environment{
		ChannelID:     authorization.ChannelID,
		EnvironmentID: authorization.EnvironmentID,
		Token: relay.TokenRegistration{
			TokenID:   authorization.TokenID,
			TokenHash: hash,
		},
		CertificateID:             authorization.CertificateID,
		CertificateBytes:          []byte("opaque-signed-environment-certificate"),
		Mode:                      mode,
		ExpiresAtMillis:           expiresAtMillis,
		RelayTokenExpiresAtMillis: expiresAtMillis,
		MembershipGeneration:      generation,
	}
}

func testEnvelope(authorization relay.EnvironmentAuthorization, factID relay.FactID, sequence int64, previous relay.Digest, seed byte) relay.Envelope {
	return relay.Envelope{
		ProtocolVersion:        1,
		CipherSuite:            1,
		ChannelID:              authorization.ChannelID,
		FactID:                 factID,
		EnvironmentID:          authorization.EnvironmentID,
		EnvironmentSequence:    sequence,
		KeyGeneration:          1,
		PreviousEnvelopeDigest: previous,
		CertificateID:          authorization.CertificateID,
		Nonce:                  testNonce(seed),
		Ciphertext:             bytes.Repeat([]byte{seed}, relay.MinimumCiphertextBytes),
		Signature:              testSignature(seed),
		EnvelopeDigest:         testDigest(seed),
	}
}

func testPruneTarget(arrival relay.Arrival) relay.PruneTarget {
	return relay.PruneTarget{
		FactID:                 arrival.FactID,
		EnvironmentID:          arrival.EnvironmentID,
		EnvironmentSequence:    arrival.EnvironmentSequence,
		ArrivalSequence:        arrival.ArrivalSequence,
		EnvelopeDigest:         arrival.EnvelopeDigest,
		CertificateID:          arrival.CertificateID,
		PreviousEnvelopeDigest: arrival.PreviousEnvelopeDigest,
		KeyGeneration:          arrival.KeyGeneration,
		Nonce:                  arrival.Nonce,
	}
}

func assertArrivalCiphertextRetained(t *testing.T, store *Store, channelID relay.ChannelID, arrivalSequence int64) {
	t.Helper()
	var ciphertext, pruneID []byte
	if err := store.db.QueryRow(`
SELECT ciphertext, prune_id
FROM relay_arrivals
WHERE channel_id = ? AND arrival_sequence = ?`, channelID[:], arrivalSequence).Scan(&ciphertext, &pruneID); err != nil {
		t.Fatalf("read retained relay arrival %d: %v", arrivalSequence, err)
	}
	if ciphertext == nil || pruneID != nil {
		t.Fatalf("relay arrival %d retention = ciphertext %v, prune id %x; want retained ciphertext and no prune", arrivalSequence, ciphertext != nil, pruneID)
	}
}

func acknowledgeTestHead(t *testing.T, store *Store, authorization relay.EnvironmentAuthorization, membership uint32, applied, producer int64, producerDigest relay.Digest, seed byte) {
	t.Helper()
	if err := store.Acknowledge(t.Context(), relay.AcknowledgeRequest{
		Authorization: authorization,
		Acknowledgement: relay.Acknowledgement{
			ChannelID:              authorization.ChannelID,
			EnvironmentID:          authorization.EnvironmentID,
			MembershipGeneration:   membership,
			AppliedArrivalSequence: applied,
			ProducerSequence:       producer,
			ProducerEnvelopeDigest: producerDigest,
			CertificateID:          authorization.CertificateID,
			AcknowledgementDigest:  testDigest(seed),
			AcknowledgementBytes:   []byte{seed, seed + 1},
		},
	}); err != nil {
		t.Fatalf("Acknowledge(%s) error = %v", authorization.EnvironmentID, err)
	}
}

func testSecret(seed byte) relay.RelayTokenSecret {
	var value relay.RelayTokenSecret
	for index := range value {
		value[index] = seed + byte(index)
	}
	return value
}

func testTokenID(seed byte) relay.RelayTokenID {
	var value relay.RelayTokenID
	for index := range value {
		value[index] = seed + byte(index)
	}
	return value
}

func testChannelID(seed byte) relay.ChannelID {
	var value relay.ChannelID
	for index := range value {
		value[index] = seed + byte(index)
	}
	return value
}

func testRelayGeneration(seed byte) relay.RelayGeneration {
	var value relay.RelayGeneration
	for index := range value {
		value[index] = seed + byte(index)
	}
	return value
}

func testDigest(seed byte) relay.Digest {
	var value relay.Digest
	for index := range value {
		value[index] = seed + byte(index)
	}
	return value
}

func testPublicKey(seed byte) relay.PublicKey {
	var value relay.PublicKey
	for index := range value {
		value[index] = seed + byte(index)
	}
	return value
}

func testNonce(seed byte) relay.Nonce {
	var value relay.Nonce
	for index := range value {
		value[index] = seed + byte(index)
	}
	return value
}

func testSignature(seed byte) relay.Signature {
	var value relay.Signature
	for index := range value {
		value[index] = seed + byte(index)
	}
	return value
}

func closeTestStore(t *testing.T, store *Store) {
	t.Helper()
	if err := store.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}
}

type allowTestVerifier struct{}

func (allowTestVerifier) VerifyEnvironmentCertificate(_ context.Context, _ relay.ChannelAuthority, _ relay.Environment) error {
	return nil
}

func (allowTestVerifier) VerifyEnvelope(_ context.Context, _ relay.EnvironmentAuthority, _ relay.Envelope) error {
	return nil
}

func (allowTestVerifier) VerifyAcknowledgement(_ context.Context, _ relay.EnvironmentAuthority, _ relay.Acknowledgement) error {
	return nil
}

func (allowTestVerifier) VerifyRetirement(_ context.Context, _ relay.ChannelAuthority, _ relay.Retirement) error {
	return nil
}

func (allowTestVerifier) VerifyPruneCertificate(_ context.Context, _ relay.PruneAuthority, _ relay.PruneCertificate) error {
	return nil
}

type gatedTestVerifier struct {
	certificateCalls      int
	envelopeCalls         int
	acknowledgementCalls  int
	retirementCalls       int
	pruneCalls            int
	pruneAuthority        relay.PruneAuthority
	pruneCertificate      relay.PruneCertificate
	rejectCertificate     bool
	rejectEnvelope        bool
	rejectAcknowledgement bool
	rejectRetirement      bool
	rejectPrune           bool
}

func (verifier *gatedTestVerifier) VerifyEnvironmentCertificate(_ context.Context, _ relay.ChannelAuthority, _ relay.Environment) error {
	verifier.certificateCalls++
	if verifier.rejectCertificate {
		return errors.New("rejected test certificate")
	}
	return nil
}

func (verifier *gatedTestVerifier) VerifyEnvelope(_ context.Context, _ relay.EnvironmentAuthority, _ relay.Envelope) error {
	verifier.envelopeCalls++
	if verifier.rejectEnvelope {
		return errors.New("rejected test envelope")
	}
	return nil
}

func (verifier *gatedTestVerifier) VerifyAcknowledgement(_ context.Context, _ relay.EnvironmentAuthority, _ relay.Acknowledgement) error {
	verifier.acknowledgementCalls++
	if verifier.rejectAcknowledgement {
		return errors.New("rejected test acknowledgement")
	}
	return nil
}

func (verifier *gatedTestVerifier) VerifyRetirement(_ context.Context, _ relay.ChannelAuthority, _ relay.Retirement) error {
	verifier.retirementCalls++
	if verifier.rejectRetirement {
		return errors.New("rejected test retirement")
	}
	return nil
}

func (verifier *gatedTestVerifier) VerifyPruneCertificate(_ context.Context, authority relay.PruneAuthority, certificate relay.PruneCertificate) error {
	verifier.pruneCalls++
	verifier.pruneAuthority = authority
	verifier.pruneCertificate = certificate
	if verifier.rejectPrune {
		return errors.New("rejected test prune")
	}
	return nil
}

func assertTableCount(t *testing.T, store *Store, table string, want int) {
	t.Helper()
	allowed := map[string]bool{
		"relay_environments":       true,
		"relay_arrivals":           true,
		"relay_acknowledgements":   true,
		"relay_prune_certificates": true,
	}
	if !allowed[table] {
		t.Fatalf("test requested disallowed table %q", table)
	}
	var got int
	if err := store.db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&got); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	if got != want {
		t.Fatalf("count %s = %d, want %d", table, got, want)
	}
}
