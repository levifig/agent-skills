package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/levifig/loaf/vnext/sync/relay"
)

func TestEnvironmentRegistrationClassifierIsReadOnlyAndExactAcrossAdvances(t *testing.T) {
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

	firstAuthorization := testEnvironmentAuthorization(owner, "environment-a", 0x21, 0x81)
	firstRegistration := relay.RegisterEnvironmentRequest{
		Authorization: owner,
		Environment:   testEnvironment(firstAuthorization, relay.TrustedEnvironment, 0, 1),
	}
	beforeAbsent := registrationPersistenceSnapshotOf(t, store, owner.ChannelID)
	beforeAbsentVerifier := verifierCallSnapshotOf(verifier)
	absent, err := store.ClassifyEnvironmentRegistration(t.Context(), firstRegistration)
	if err != nil {
		t.Fatalf("ClassifyEnvironmentRegistration(absent) error = %v", err)
	}
	if absent.Disposition != relay.EnvironmentRegistrationAbsent ||
		absent.State.ChannelID != owner.ChannelID ||
		absent.State.RelayGeneration != owner.RelayGeneration ||
		absent.State.MembershipGeneration != 0 || absent.State.Head != 0 || absent.State.CreatedAt.IsZero() {
		t.Fatalf("ClassifyEnvironmentRegistration(absent) = %#v, want absent at current G0/H0", absent)
	}
	if got := registrationPersistenceSnapshotOf(t, store, owner.ChannelID); got != beforeAbsent {
		t.Fatalf("classification persistence changed: before %#v, after %#v", beforeAbsent, got)
	}
	if got := verifierCallSnapshotOf(verifier); got != beforeAbsentVerifier {
		t.Fatalf("classification invoked verifier: before %#v, after %#v", beforeAbsentVerifier, got)
	}

	registered, err := store.RegisterEnvironment(t.Context(), firstRegistration)
	if err != nil {
		t.Fatalf("RegisterEnvironment(first) error = %v", err)
	}
	beforeExact := registrationPersistenceSnapshotOf(t, store, owner.ChannelID)
	beforeExactVerifier := verifierCallSnapshotOf(verifier)
	exact, err := store.ClassifyEnvironmentRegistration(t.Context(), firstRegistration)
	if err != nil {
		t.Fatalf("ClassifyEnvironmentRegistration(exact) error = %v", err)
	}
	if exact.Disposition != relay.EnvironmentRegistrationExact || exact.State != registered {
		t.Fatalf("ClassifyEnvironmentRegistration(exact) = %#v, want exact %#v", exact, registered)
	}
	if got := registrationPersistenceSnapshotOf(t, store, owner.ChannelID); got != beforeExact {
		t.Fatalf("exact classification persistence changed: before %#v, after %#v", beforeExact, got)
	}
	if got := verifierCallSnapshotOf(verifier); got != beforeExactVerifier {
		t.Fatalf("exact classification invoked verifier: before %#v, after %#v", beforeExactVerifier, got)
	}
	if _, err := store.Append(t.Context(), relay.AppendRequest{
		Authorization: firstAuthorization,
		Envelope:      testEnvelope(firstAuthorization, "fact-before-second-registration", 1, relay.Digest{}, 0x31),
	}); err != nil {
		t.Fatalf("Append(first environment) error = %v", err)
	}

	secondAuthorization := testEnvironmentAuthorization(owner, "environment-b", 0x31, 0x91)
	secondRegistration := relay.RegisterEnvironmentRequest{
		Authorization: owner,
		Environment:   testEnvironment(secondAuthorization, relay.TrustedEnvironment, 0, 2),
	}
	beforeSecondAbsent := registrationPersistenceSnapshotOf(t, store, owner.ChannelID)
	beforeSecondAbsentVerifier := verifierCallSnapshotOf(verifier)
	secondAbsent, err := store.ClassifyEnvironmentRegistration(t.Context(), secondRegistration)
	if err != nil {
		t.Fatalf("ClassifyEnvironmentRegistration(second absent) error = %v", err)
	}
	if secondAbsent.Disposition != relay.EnvironmentRegistrationAbsent ||
		secondAbsent.State.ChannelID != registered.ChannelID ||
		secondAbsent.State.RelayGeneration != registered.RelayGeneration ||
		secondAbsent.State.CreatedAt != registered.CreatedAt ||
		secondAbsent.State.MembershipGeneration != 1 || secondAbsent.State.Head != 1 {
		t.Fatalf("ClassifyEnvironmentRegistration(second absent) = %#v, want absent with exact channel state at G1/H1", secondAbsent)
	}
	if got := registrationPersistenceSnapshotOf(t, store, owner.ChannelID); got != beforeSecondAbsent {
		t.Fatalf("second absent classification persistence changed: before %#v, after %#v", beforeSecondAbsent, got)
	}
	if got := verifierCallSnapshotOf(verifier); got != beforeSecondAbsentVerifier {
		t.Fatalf("second absent classification invoked verifier: before %#v, after %#v", beforeSecondAbsentVerifier, got)
	}
	if _, err := store.RegisterEnvironment(t.Context(), secondRegistration); err != nil {
		t.Fatalf("RegisterEnvironment(second) error = %v", err)
	}
	if _, err := store.Append(t.Context(), relay.AppendRequest{
		Authorization: secondAuthorization,
		Envelope:      testEnvelope(secondAuthorization, "fact-after-registration", 1, relay.Digest{}, 0x41),
	}); err != nil {
		t.Fatalf("Append() error = %v", err)
	}
	beforeAdvancedExact := registrationPersistenceSnapshotOf(t, store, owner.ChannelID)
	beforeAdvancedExactVerifier := verifierCallSnapshotOf(verifier)
	advancedExact, err := store.ClassifyEnvironmentRegistration(t.Context(), firstRegistration)
	if err != nil {
		t.Fatalf("ClassifyEnvironmentRegistration(exact after advance) error = %v", err)
	}
	if advancedExact.Disposition != relay.EnvironmentRegistrationExact ||
		advancedExact.State.MembershipGeneration != 2 || advancedExact.State.Head != 2 {
		t.Fatalf("ClassifyEnvironmentRegistration(exact after advance) = %#v, want exact at current G2/H2", advancedExact)
	}
	if got := registrationPersistenceSnapshotOf(t, store, owner.ChannelID); got != beforeAdvancedExact {
		t.Fatalf("advanced exact classification persistence changed: before %#v, after %#v", beforeAdvancedExact, got)
	}
	if got := verifierCallSnapshotOf(verifier); got != beforeAdvancedExactVerifier {
		t.Fatalf("advanced exact classification invoked verifier: before %#v, after %#v", beforeAdvancedExactVerifier, got)
	}
}

func TestEnvironmentRegistrationClassifierMatchesImmutableRegistrationConflicts(t *testing.T) {
	t.Parallel()

	verifier := &gatedTestVerifier{}
	store, err := Open(filepath.Join(t.TempDir(), "relay.sqlite"), verifier)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { closeTestStore(t, store) })
	store.now = func() time.Time { return time.UnixMilli(1_000) }
	owner := testOwnerAuthorization(store)
	if _, err := store.CreateChannel(t.Context(), testChannel(owner)); err != nil {
		t.Fatalf("CreateChannel() error = %v", err)
	}
	authorization := testEnvironmentAuthorization(owner, "environment-a", 0x21, 0x81)
	registration := relay.RegisterEnvironmentRequest{
		Authorization: owner,
		Environment:   testEnvironment(authorization, relay.EphemeralEnvironment, 10_000, 1),
	}
	if _, err := store.RegisterEnvironment(t.Context(), registration); err != nil {
		t.Fatalf("RegisterEnvironment() error = %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*relay.RegisterEnvironmentRequest)
	}{
		{name: "certificate id", mutate: func(value *relay.RegisterEnvironmentRequest) {
			value.Environment.CertificateID = testDigest(0xa1)
		}},
		{name: "certificate bytes", mutate: func(value *relay.RegisterEnvironmentRequest) {
			value.Environment.CertificateBytes = []byte("different-opaque-signed-environment-certificate")
		}},
		{name: "mode", mutate: func(value *relay.RegisterEnvironmentRequest) {
			value.Environment.Mode = relay.TrustedEnvironment
			value.Environment.ExpiresAtMillis = 0
			value.Environment.RelayTokenExpiresAtMillis = 0
		}},
		{name: "certificate expiry", mutate: func(value *relay.RegisterEnvironmentRequest) {
			value.Environment.ExpiresAtMillis = 11_000
		}},
		{name: "membership generation", mutate: func(value *relay.RegisterEnvironmentRequest) {
			value.Environment.MembershipGeneration = 2
		}},
		{name: "token id", mutate: func(value *relay.RegisterEnvironmentRequest) {
			value.Environment.Token.TokenID = testTokenID(0xb1)
		}},
		{name: "token hash", mutate: func(value *relay.RegisterEnvironmentRequest) {
			value.Environment.Token.TokenHash[0] ^= 0xff
		}},
		{name: "relay token expiry", mutate: func(value *relay.RegisterEnvironmentRequest) {
			value.Environment.RelayTokenExpiresAtMillis = 9_000
		}},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			candidate := registration
			candidate.Environment.CertificateBytes = append([]byte(nil), registration.Environment.CertificateBytes...)
			test.mutate(&candidate)
			before := registrationPersistenceSnapshotOf(t, store, owner.ChannelID)
			if _, err := store.ClassifyEnvironmentRegistration(t.Context(), candidate); !errors.Is(err, relay.ErrImmutableConflict) {
				t.Fatalf("ClassifyEnvironmentRegistration() error = %v, want ErrImmutableConflict", err)
			}
			if _, err := store.RegisterEnvironment(t.Context(), candidate); !errors.Is(err, relay.ErrImmutableConflict) {
				t.Fatalf("RegisterEnvironment() error = %v, want ErrImmutableConflict", err)
			}
			if got := registrationPersistenceSnapshotOf(t, store, owner.ChannelID); got != before {
				t.Fatalf("conflict attempts changed persistence: before %#v, after %#v", before, got)
			}
		})
	}
	if verifier.certificateCalls != 1 {
		t.Fatalf("certificate verifier calls = %d, want accepted registration only", verifier.certificateCalls)
	}
}

func TestEnvironmentRegistrationClassifierMatchesEligibleUniquenessConflicts(t *testing.T) {
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
	firstAuthorization := testEnvironmentAuthorization(owner, "environment-a", 0x21, 0x81)
	first := relay.RegisterEnvironmentRequest{
		Authorization: owner,
		Environment:   testEnvironment(firstAuthorization, relay.TrustedEnvironment, 0, 1),
	}
	if _, err := store.RegisterEnvironment(t.Context(), first); err != nil {
		t.Fatalf("RegisterEnvironment(first) error = %v", err)
	}

	uniqueAuthorization := testEnvironmentAuthorization(owner, "environment-b", 0x31, 0x91)
	unique := relay.RegisterEnvironmentRequest{
		Authorization: owner,
		Environment:   testEnvironment(uniqueAuthorization, relay.TrustedEnvironment, 0, 2),
	}
	tests := []struct {
		name   string
		mutate func(*relay.RegisterEnvironmentRequest)
	}{
		{name: "environment id with existing tuple", mutate: func(value *relay.RegisterEnvironmentRequest) {
			value.Environment.CertificateID = first.Environment.CertificateID
			value.Environment.CertificateBytes = append([]byte(nil), first.Environment.CertificateBytes...)
			value.Environment.Token = first.Environment.Token
		}},
		{name: "certificate id", mutate: func(value *relay.RegisterEnvironmentRequest) {
			value.Environment.CertificateID = first.Environment.CertificateID
		}},
		{name: "environment token id", mutate: func(value *relay.RegisterEnvironmentRequest) {
			value.Environment.Token.TokenID = first.Environment.Token.TokenID
		}},
		{name: "owner token id", mutate: func(value *relay.RegisterEnvironmentRequest) {
			value.Environment.Token.TokenID = owner.TokenID
		}},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			candidate := unique
			candidate.Environment.CertificateBytes = append([]byte(nil), unique.Environment.CertificateBytes...)
			test.mutate(&candidate)
			before := registrationPersistenceSnapshotOf(t, store, owner.ChannelID)
			if _, err := store.ClassifyEnvironmentRegistration(t.Context(), candidate); !errors.Is(err, relay.ErrImmutableConflict) {
				t.Fatalf("ClassifyEnvironmentRegistration() error = %v, want ErrImmutableConflict", err)
			}
			if _, err := store.RegisterEnvironment(t.Context(), candidate); !errors.Is(err, relay.ErrImmutableConflict) {
				t.Fatalf("RegisterEnvironment() error = %v, want ErrImmutableConflict", err)
			}
			if got := registrationPersistenceSnapshotOf(t, store, owner.ChannelID); got != before {
				t.Fatalf("conflict attempts changed persistence: before %#v, after %#v", before, got)
			}
		})
	}
	if verifier.certificateCalls != 1 {
		t.Fatalf("certificate verifier calls = %d, want accepted registration only", verifier.certificateCalls)
	}
}

func TestEnvironmentRegistrationClassifierMatchesConsumedMembershipAndAuthorizationErrors(t *testing.T) {
	t.Parallel()

	verifier := &gatedTestVerifier{}
	store, owner, _ := newTestStoreWithEnvironmentAndVerifier(t, relay.TrustedEnvironment, 0, verifier)
	secondAuthorization := testEnvironmentAuthorization(owner, "environment-b", 0x31, 0x91)
	second := relay.RegisterEnvironmentRequest{
		Authorization: owner,
		Environment:   testEnvironment(secondAuthorization, relay.TrustedEnvironment, 0, 2),
	}
	if _, err := store.RegisterEnvironment(t.Context(), second); err != nil {
		t.Fatalf("RegisterEnvironment(second) error = %v", err)
	}

	missingAuthorization := testEnvironmentAuthorization(owner, "environment-c", 0x41, 0xa1)
	missingConsumed := relay.RegisterEnvironmentRequest{
		Authorization: owner,
		Environment:   testEnvironment(missingAuthorization, relay.TrustedEnvironment, 0, 2),
	}
	tests := []struct {
		name    string
		request relay.RegisterEnvironmentRequest
		want    error
	}{
		{name: "consumed membership", request: missingConsumed, want: relay.ErrMembershipChanged},
		{name: "wrong owner secret", request: func() relay.RegisterEnvironmentRequest {
			value := missingConsumed
			value.Authorization.TokenSecret = testSecret(0xee)
			return value
		}(), want: relay.ErrUnauthenticated},
		{name: "wrong owner token id", request: func() relay.RegisterEnvironmentRequest {
			value := missingConsumed
			value.Authorization.TokenID = testTokenID(0xee)
			return value
		}(), want: relay.ErrUnauthenticated},
		{name: "wrong relay generation", request: func() relay.RegisterEnvironmentRequest {
			value := missingConsumed
			value.Authorization.RelayGeneration = testRelayGeneration(0xee)
			return value
		}(), want: relay.ErrGenerationMismatch},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			before := registrationPersistenceSnapshotOf(t, store, owner.ChannelID)
			if _, err := store.ClassifyEnvironmentRegistration(t.Context(), test.request); !errors.Is(err, test.want) {
				t.Fatalf("ClassifyEnvironmentRegistration() error = %v, want %v", err, test.want)
			}
			if _, err := store.RegisterEnvironment(t.Context(), test.request); !errors.Is(err, test.want) {
				t.Fatalf("RegisterEnvironment() error = %v, want %v", err, test.want)
			}
			if got := registrationPersistenceSnapshotOf(t, store, owner.ChannelID); got != before {
				t.Fatalf("failed attempts changed persistence: before %#v, after %#v", before, got)
			}
		})
	}
	if verifier.certificateCalls != 2 {
		t.Fatalf("certificate verifier calls = %d, want accepted registrations only", verifier.certificateCalls)
	}
}

func TestEnvironmentRegistrationClassifierMatchesExpiredAndRetiredReplays(t *testing.T) {
	t.Parallel()

	t.Run("expired", func(t *testing.T) {
		verifier := &gatedTestVerifier{}
		store, owner, authorization := newTestStoreWithEnvironmentAndVerifier(t, relay.EphemeralEnvironment, 2_000, verifier)
		store.now = func() time.Time { return time.UnixMilli(2_000) }
		request := relay.RegisterEnvironmentRequest{
			Authorization: owner,
			Environment:   testEnvironment(authorization, relay.EphemeralEnvironment, 2_000, 1),
		}
		before := registrationPersistenceSnapshotOf(t, store, owner.ChannelID)
		if _, err := store.ClassifyEnvironmentRegistration(t.Context(), request); !errors.Is(err, relay.ErrExpired) {
			t.Fatalf("ClassifyEnvironmentRegistration() error = %v, want ErrExpired", err)
		}
		if _, err := store.RegisterEnvironment(t.Context(), request); !errors.Is(err, relay.ErrExpired) {
			t.Fatalf("RegisterEnvironment() error = %v, want ErrExpired", err)
		}
		if got := registrationPersistenceSnapshotOf(t, store, owner.ChannelID); got != before {
			t.Fatalf("expired attempts changed persistence: before %#v, after %#v", before, got)
		}
		if verifier.certificateCalls != 1 {
			t.Fatalf("certificate verifier calls = %d, want accepted registration only", verifier.certificateCalls)
		}
	})

	t.Run("retired", func(t *testing.T) {
		verifier := &gatedTestVerifier{}
		store, owner, authorization := newTestStoreWithEnvironmentAndVerifier(t, relay.TrustedEnvironment, 0, verifier)
		request := relay.RegisterEnvironmentRequest{
			Authorization: owner,
			Environment:   testEnvironment(authorization, relay.TrustedEnvironment, 0, 1),
		}
		if _, err := store.RetireEnvironment(t.Context(), relay.RetireEnvironmentRequest{
			Authorization: owner,
			Retirement: relay.Retirement{
				ChannelID:            owner.ChannelID,
				RelayGeneration:      owner.RelayGeneration,
				EnvironmentID:        authorization.EnvironmentID,
				CertificateID:        authorization.CertificateID,
				MembershipGeneration: 2,
				RetirementID:         testDigest(0xb1),
				RetirementBytes:      []byte("opaque-signed-retirement"),
			},
		}); err != nil {
			t.Fatalf("RetireEnvironment() error = %v", err)
		}
		before := registrationPersistenceSnapshotOf(t, store, owner.ChannelID)
		if _, err := store.ClassifyEnvironmentRegistration(t.Context(), request); !errors.Is(err, relay.ErrRetired) {
			t.Fatalf("ClassifyEnvironmentRegistration() error = %v, want ErrRetired", err)
		}
		if _, err := store.RegisterEnvironment(t.Context(), request); !errors.Is(err, relay.ErrRetired) {
			t.Fatalf("RegisterEnvironment() error = %v, want ErrRetired", err)
		}
		if got := registrationPersistenceSnapshotOf(t, store, owner.ChannelID); got != before {
			t.Fatalf("retired attempts changed persistence: before %#v, after %#v", before, got)
		}
		if verifier.certificateCalls != 1 {
			t.Fatalf("certificate verifier calls = %d, want accepted registration only", verifier.certificateCalls)
		}
	})
}

func TestEnvironmentRegistrationClassifierContextAndClosedSemantics(t *testing.T) {
	t.Parallel()

	store, owner, _ := newTestStoreWithEnvironment(t, relay.TrustedEnvironment, 0)
	authorization := testEnvironmentAuthorization(owner, "environment-b", 0x31, 0x91)
	request := relay.RegisterEnvironmentRequest{
		Authorization: owner,
		Environment:   testEnvironment(authorization, relay.TrustedEnvironment, 0, 2),
	}
	if _, err := store.ClassifyEnvironmentRegistration(nil, request); !errors.Is(err, relay.ErrInvalidArgument) {
		t.Fatalf("ClassifyEnvironmentRegistration(nil context) error = %v, want ErrInvalidArgument", err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := store.ClassifyEnvironmentRegistration(canceled, request); !errors.Is(err, context.Canceled) {
		t.Fatalf("ClassifyEnvironmentRegistration(canceled) error = %v, want context.Canceled", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if _, err := store.ClassifyEnvironmentRegistration(t.Context(), request); !errors.Is(err, relay.ErrClosed) {
		t.Fatalf("ClassifyEnvironmentRegistration(closed) error = %v, want ErrClosed", err)
	}
}

type registrationPersistenceSnapshot struct {
	totalChanges       int64
	channelCount       int
	environmentCount   int
	arrivalCount       int
	membership         uint32
	maximumArrivalHead int64
}

type verifierCallSnapshot struct {
	certificate     int
	envelope        int
	acknowledgement int
	retirement      int
	prune           int
}

func verifierCallSnapshotOf(verifier *gatedTestVerifier) verifierCallSnapshot {
	return verifierCallSnapshot{
		certificate:     verifier.certificateCalls,
		envelope:        verifier.envelopeCalls,
		acknowledgement: verifier.acknowledgementCalls,
		retirement:      verifier.retirementCalls,
		prune:           verifier.pruneCalls,
	}
}

func registrationPersistenceSnapshotOf(t *testing.T, store *Store, channelID relay.ChannelID) registrationPersistenceSnapshot {
	t.Helper()

	var snapshot registrationPersistenceSnapshot
	if err := store.db.QueryRow(`
SELECT total_changes(),
       (SELECT COUNT(*) FROM relay_channels),
       (SELECT COUNT(*) FROM relay_environments),
       (SELECT COUNT(*) FROM relay_arrivals),
       (SELECT membership_generation FROM relay_channels WHERE channel_id = ?),
       COALESCE((SELECT MAX(arrival_sequence) FROM relay_arrivals WHERE channel_id = ?), 0)`,
		channelID[:], channelID[:],
	).Scan(
		&snapshot.totalChanges,
		&snapshot.channelCount,
		&snapshot.environmentCount,
		&snapshot.arrivalCount,
		&snapshot.membership,
		&snapshot.maximumArrivalHead,
	); err != nil {
		t.Fatalf("read registration persistence snapshot: %v", err)
	}
	return snapshot
}
