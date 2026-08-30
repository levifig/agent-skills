package coordinator

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/levifig/loaf/vnext/continuity"
	continuitysqlite "github.com/levifig/loaf/vnext/continuity/sqlite"
	"github.com/levifig/loaf/vnext/sync/credential"
	"github.com/levifig/loaf/vnext/sync/crypto"
	"github.com/levifig/loaf/vnext/sync/protocol"
	"github.com/levifig/loaf/vnext/sync/relay"
)

func TestBindPreparedRecoveryRegistrationProducesDeterministicSecretFreeRegistration(t *testing.T) {
	writerID := testEnvironmentID(200)
	store := openCoordinatorStore(t, writerID)
	recovery := testBindableRecoveryCredential(t)
	prepared := testPreparedRecoveryCredential(t, recovery, writerID, 4, []uint32{recovery.WriteGeneration})
	remote := emptyInventoryRemote(recovery)
	coordinator := mustCoordinator(t, store, remote)

	if _, err := store.RegisterProject(context.Background(), recovery.ProjectID, "fact-registration-binding-root", continuity.ProjectRegistrationPayload{
		Observation: continuity.Observation{ObservedAtMillis: 1},
		Label:       "Registration binding",
	}); err != nil {
		t.Fatalf("register local project fixture: %v", err)
	}
	before, err := store.Snapshot(context.Background(), recovery.ProjectID, continuity.SnapshotRequest{})
	if err != nil {
		t.Fatalf("Snapshot(before) error = %v", err)
	}

	first, err := coordinator.bindPreparedRecoveryRegistration(recovery.ProjectID, recovery, prepared)
	if err != nil {
		t.Fatalf("bind prepared recovery registration: %v", err)
	}
	second, err := coordinator.bindPreparedRecoveryRegistration(recovery.ProjectID, recovery, prepared)
	if err != nil {
		t.Fatalf("bind prepared recovery registration again: %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("deterministic bindings differ:\n first %#v\nsecond %#v", first, second)
	}
	if err := first.environment.Validate(); err != nil {
		t.Fatalf("bound relay environment: %v", err)
	}
	certificateBytes, err := prepared.Certificate.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal prepared certificate: %v", err)
	}
	certificateID := relay.Digest(protocol.CertificateID(prepared.Certificate))
	tokenHash, err := relay.HashTokenSecret(relay.RelayTokenSecret(prepared.EnvironmentRelayAuthorization.Secret()))
	if err != nil {
		t.Fatalf("hash prepared relay secret: %v", err)
	}
	wantEnvironment := relay.Environment{
		ChannelID:     relay.ChannelID(recovery.ChannelID),
		EnvironmentID: relay.EnvironmentID(writerID),
		Token: relay.TokenRegistration{
			TokenID:   relay.RelayTokenID(prepared.EnvironmentRelayAuthorization.ID()),
			TokenHash: tokenHash,
		},
		CertificateID:             certificateID,
		CertificateBytes:          certificateBytes,
		Mode:                      relay.TrustedEnvironment,
		MembershipGeneration:      prepared.Certificate.MembershipGeneration,
		ExpiresAtMillis:           0,
		RelayTokenExpiresAtMillis: 0,
	}
	if !reflect.DeepEqual(first.environment, wantEnvironment) ||
		first.targetMembershipGeneration != prepared.Certificate.MembershipGeneration || first.certificateID != certificateID {
		t.Fatalf("bound registration = %#v, want environment %#v membership %d certificate %x", first, wantEnvironment, prepared.Certificate.MembershipGeneration, certificateID)
	}

	first.environment.CertificateBytes[0] ^= 0xff
	if !reflect.DeepEqual(second.environment.CertificateBytes, certificateBytes) {
		t.Fatal("repeat binding aliases certificate bytes from an earlier result")
	}
	third, err := coordinator.bindPreparedRecoveryRegistration(recovery.ProjectID, recovery, prepared)
	if err != nil || !reflect.DeepEqual(third.environment.CertificateBytes, certificateBytes) {
		t.Fatalf("binding after result mutation = (%#v, %v), want original canonical bytes", third, err)
	}

	after, err := store.Snapshot(context.Background(), recovery.ProjectID, continuity.SnapshotRequest{})
	if err != nil {
		t.Fatalf("Snapshot(after) error = %v", err)
	}
	if !reflect.DeepEqual(after, before) {
		t.Fatal("pure registration binding changed local continuity state")
	}
	assertNoRecoveryRegistrationWorkflowCalls(t, remote)
	assertPreparedRegistrationCarriesNoBearerSecretType(t, first)
	if got := fmt.Sprintf("%v", first); got != "[REDACTED prepared recovery registration]" {
		t.Fatalf("registration String format = %q", got)
	}
	if got := fmt.Sprintf("%#v", first); got != "coordinator.preparedRecoveryRegistration([REDACTED])" {
		t.Fatalf("registration GoString format = %q", got)
	}
}

func TestBindPreparedRecoveryRegistrationAcceptsDecodedEquivalentCredential(t *testing.T) {
	writerID := testEnvironmentID(200)
	recovery := testBindableRecoveryCredential(t)
	prepared := testPreparedRecoveryCredential(t, recovery, writerID, 4, []uint32{recovery.WriteGeneration})
	encoded, err := credential.EncodeTrusted(prepared)
	if err != nil {
		t.Fatalf("encode prepared credential: %v", err)
	}
	decoded, err := credential.DecodeTrusted(encoded)
	if err != nil {
		t.Fatalf("decode prepared credential: %v", err)
	}
	store := openCoordinatorStore(t, writerID)
	remote := emptyInventoryRemote(recovery)
	coordinator := mustCoordinator(t, store, remote)

	direct, err := coordinator.bindPreparedRecoveryRegistration(recovery.ProjectID, recovery, prepared)
	if err != nil {
		t.Fatalf("bind direct credential: %v", err)
	}
	roundTripped, err := coordinator.bindPreparedRecoveryRegistration(recovery.ProjectID, recovery, decoded)
	if err != nil {
		t.Fatalf("bind decoded credential: %v", err)
	}
	if !reflect.DeepEqual(roundTripped, direct) {
		t.Fatal("decoded equivalent credential produced a different registration binding")
	}
	assertNoRecoveryRegistrationWorkflowCalls(t, remote)
}

func TestBindPreparedRecoveryRegistrationHashesTheExactPreparedBearer(t *testing.T) {
	writerID := testEnvironmentID(200)
	recovery := testBindableRecoveryCredential(t)
	prepared := testPreparedRecoveryCredential(t, recovery, writerID, 4, []uint32{recovery.WriteGeneration})
	store := openCoordinatorStore(t, writerID)
	remote := emptyInventoryRemote(recovery)
	coordinator := mustCoordinator(t, store, remote)

	first, err := coordinator.bindPreparedRecoveryRegistration(recovery.ProjectID, recovery, prepared)
	if err != nil {
		t.Fatalf("bind first bearer: %v", err)
	}
	tokenID := prepared.EnvironmentRelayAuthorization.ID()
	var otherSecret credential.RelayTokenSecret
	copy(otherSecret[:], testBytes(0x64, len(otherSecret)))
	otherBearer, err := credential.NewRelayBearer(tokenID, otherSecret)
	if err != nil {
		t.Fatalf("other prepared bearer: %v", err)
	}
	prepared.EnvironmentRelayAuthorization = otherBearer
	second, err := coordinator.bindPreparedRecoveryRegistration(recovery.ProjectID, recovery, prepared)
	if err != nil {
		t.Fatalf("bind second bearer: %v", err)
	}
	if first.environment.Token.TokenID != second.environment.Token.TokenID ||
		first.environment.Token.TokenHash == second.environment.Token.TokenHash {
		t.Fatal("registration did not bind the exact prepared bearer secret to its deterministic token hash")
	}
	assertNoRecoveryRegistrationWorkflowCalls(t, remote)
}

func TestBindPreparedRecoveryRegistrationRejectsMismatchesAndInvalidAuthority(t *testing.T) {
	writerID := testEnvironmentID(200)
	baseRecovery := testBindableRecoveryCredential(t)
	basePrepared := testPreparedRecoveryCredential(t, baseRecovery, writerID, 4, []uint32{baseRecovery.WriteGeneration})

	tests := []struct {
		name   string
		mutate func(*testing.T, *continuity.ProjectID, *credential.ProjectRecoveryCredential, *credential.TrustedProjectCredential)
	}{
		{name: "expected project", mutate: func(_ *testing.T, expected *continuity.ProjectID, _ *credential.ProjectRecoveryCredential, _ *credential.TrustedProjectCredential) {
			*expected = testProjectID(77)
		}},
		{name: "recovery project", mutate: func(_ *testing.T, _ *continuity.ProjectID, recovery *credential.ProjectRecoveryCredential, _ *credential.TrustedProjectCredential) {
			recovery.ProjectID = testProjectID(77)
		}},
		{name: "prepared project", mutate: func(t *testing.T, _ *continuity.ProjectID, recovery *credential.ProjectRecoveryCredential, prepared *credential.TrustedProjectCredential) {
			other := *recovery
			other.ProjectID = testProjectID(77)
			*prepared = testPreparedRecoveryCredential(t, other, writerID, 4, []uint32{other.WriteGeneration})
		}},
		{name: "recovery relay URL", mutate: func(_ *testing.T, _ *continuity.ProjectID, recovery *credential.ProjectRecoveryCredential, _ *credential.TrustedProjectCredential) {
			recovery.RelayURL = "https://other-recovery.example.test"
		}},
		{name: "prepared relay URL", mutate: func(_ *testing.T, _ *continuity.ProjectID, _ *credential.ProjectRecoveryCredential, prepared *credential.TrustedProjectCredential) {
			prepared.RelayURL = "https://other-prepared.example.test"
		}},
		{name: "recovery relay generation", mutate: func(_ *testing.T, _ *continuity.ProjectID, recovery *credential.ProjectRecoveryCredential, _ *credential.TrustedProjectCredential) {
			recovery.RelayGeneration[0] ^= 0xff
		}},
		{name: "prepared relay generation", mutate: func(_ *testing.T, _ *continuity.ProjectID, _ *credential.ProjectRecoveryCredential, prepared *credential.TrustedProjectCredential) {
			prepared.RelayGeneration[0] ^= 0xff
		}},
		{name: "channel", mutate: func(t *testing.T, _ *continuity.ProjectID, recovery *credential.ProjectRecoveryCredential, prepared *credential.TrustedProjectCredential) {
			other := *recovery
			other.ChannelID[0] ^= 0xff
			*prepared = testPreparedRecoveryCredential(t, other, writerID, 4, []uint32{other.WriteGeneration})
		}},
		{name: "admin public key", mutate: func(t *testing.T, _ *continuity.ProjectID, recovery *credential.ProjectRecoveryCredential, prepared *credential.TrustedProjectCredential) {
			other := *recovery
			seed, err := crypto.AdminSeedFromBytes(testBytes(0x72, len(crypto.AdminSeed{})))
			if err != nil {
				t.Fatalf("other admin seed: %v", err)
			}
			other.AdminSeed = seed
			*prepared = testPreparedRecoveryCredential(t, other, writerID, 4, []uint32{other.WriteGeneration})
		}},
		{name: "project root", mutate: func(t *testing.T, _ *continuity.ProjectID, _ *credential.ProjectRecoveryCredential, prepared *credential.TrustedProjectCredential) {
			root, err := crypto.ProjectRootFromBytes(testBytes(0x73, len(crypto.ProjectRoot{})))
			if err != nil {
				t.Fatalf("other project root: %v", err)
			}
			prepared.ProjectRoot = root
		}},
		{name: "write generation", mutate: func(t *testing.T, _ *continuity.ProjectID, recovery *credential.ProjectRecoveryCredential, prepared *credential.TrustedProjectCredential) {
			other := *recovery
			other.WriteGeneration++
			*prepared = testPreparedRecoveryCredential(t, other, writerID, 4, []uint32{other.WriteGeneration})
		}},
		{name: "writer environment", mutate: func(t *testing.T, _ *continuity.ProjectID, recovery *credential.ProjectRecoveryCredential, prepared *credential.TrustedProjectCredential) {
			*prepared = testPreparedRecoveryCredential(t, *recovery, testEnvironmentID(201), 4, []uint32{recovery.WriteGeneration})
		}},
		{name: "certificate mode", mutate: func(_ *testing.T, _ *continuity.ProjectID, _ *credential.ProjectRecoveryCredential, prepared *credential.TrustedProjectCredential) {
			prepared.Certificate.Mode = protocol.EnvironmentEphemeral
			prepared.Certificate.ExpiresAtMillis = 1
		}},
		{name: "certificate protocol", mutate: func(_ *testing.T, _ *continuity.ProjectID, _ *credential.ProjectRecoveryCredential, prepared *credential.TrustedProjectCredential) {
			prepared.Certificate.ProtocolVersion++
		}},
		{name: "minimum protocol", mutate: func(_ *testing.T, _ *continuity.ProjectID, _ *credential.ProjectRecoveryCredential, prepared *credential.TrustedProjectCredential) {
			prepared.MinimumProtocolVersion++
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recovery := baseRecovery
			prepared := basePrepared
			expectedProjectID := recovery.ProjectID
			test.mutate(t, &expectedProjectID, &recovery, &prepared)
			store := openCoordinatorStore(t, writerID)
			remote := emptyInventoryRemote(baseRecovery)
			coordinator := mustCoordinator(t, store, remote)

			got, err := coordinator.bindPreparedRecoveryRegistration(expectedProjectID, recovery, prepared)
			if !reflect.DeepEqual(got, preparedRecoveryRegistration{}) {
				t.Fatal("invalid authority returned a registration binding")
			}
			assertProblem(t, err, CodeInvalid, PhaseRecoveryValidation, ActionCorrectInput)
			assertNoRecoveryRegistrationWorkflowCalls(t, remote)
		})
	}
}

func TestBindPreparedRecoveryRegistrationRejectsCheckpointsAndMultipleGenerations(t *testing.T) {
	writerID := testEnvironmentID(200)
	baseRecovery := testBindableRecoveryCredential(t)
	basePrepared := testPreparedRecoveryCredential(t, baseRecovery, writerID, 4, []uint32{baseRecovery.WriteGeneration})

	for _, test := range []struct {
		name   string
		mutate func(*testing.T, *credential.ProjectRecoveryCredential, *credential.TrustedProjectCredential)
	}{
		{name: "signed recovery checkpoint", mutate: func(_ *testing.T, recovery *credential.ProjectRecoveryCredential, _ *credential.TrustedProjectCredential) {
			recovery.LastSignedCheckpoint = []byte("signed recovery checkpoint")
		}},
		{name: "observed prepared checkpoint", mutate: func(_ *testing.T, _ *credential.ProjectRecoveryCredential, prepared *credential.TrustedProjectCredential) {
			prepared.LastObservedCheckpoint = []byte("observed prepared checkpoint")
		}},
		{name: "multiple allowed generations", mutate: func(t *testing.T, recovery *credential.ProjectRecoveryCredential, prepared *credential.TrustedProjectCredential) {
			*prepared = testPreparedRecoveryCredential(t, *recovery, writerID, 4, []uint32{recovery.WriteGeneration, recovery.WriteGeneration + 1})
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			recovery := baseRecovery
			prepared := basePrepared
			test.mutate(t, &recovery, &prepared)
			store := openCoordinatorStore(t, writerID)
			remote := emptyInventoryRemote(baseRecovery)
			coordinator := mustCoordinator(t, store, remote)

			got, err := coordinator.bindPreparedRecoveryRegistration(recovery.ProjectID, recovery, prepared)
			if !reflect.DeepEqual(got, preparedRecoveryRegistration{}) {
				t.Fatal("unsafe recovery authority returned a registration binding")
			}
			assertProblem(t, err, CodeInvalid, PhaseRecoveryValidation, ActionCorrectInput)
			assertNoRecoveryRegistrationWorkflowCalls(t, remote)
		})
	}
}

func TestBindPreparedRecoveryRegistrationErrorsAreStaticAndSecretFree(t *testing.T) {
	const secretMarker = "prepared-registration-secret-marker"
	writerID := testEnvironmentID(200)
	recovery := testBindableRecoveryCredential(t)
	prepared := testPreparedRecoveryCredential(t, recovery, writerID, 4, []uint32{recovery.WriteGeneration})
	store := openCoordinatorStore(t, writerID)
	remote := emptyInventoryRemote(recovery)
	remote.endpoint = "https://" + secretMarker + ".example.test"
	coordinator := mustCoordinator(t, store, remote)

	got, err := coordinator.bindPreparedRecoveryRegistration(recovery.ProjectID, recovery, prepared)
	if !reflect.DeepEqual(got, preparedRecoveryRegistration{}) {
		t.Fatal("endpoint mismatch returned a registration binding")
	}
	assertProblem(t, err, CodeInvalid, PhaseRecoveryValidation, ActionCorrectInput)
	for _, formatted := range []string{err.Error(), fmt.Sprintf("%v", err), fmt.Sprintf("%#v", err)} {
		if strings.Contains(formatted, secretMarker) {
			t.Fatalf("binding error leaked input value: %q", formatted)
		}
	}
	assertNoRecoveryRegistrationWorkflowCalls(t, remote)
}

func TestBindPreparedRecoveryRegistrationRequiresUsableCoordinatorDependencies(t *testing.T) {
	recovery := testBindableRecoveryCredential(t)
	writerID := testEnvironmentID(200)
	prepared := testPreparedRecoveryCredential(t, recovery, writerID, 4, []uint32{recovery.WriteGeneration})
	store := openCoordinatorStore(t, writerID)
	remote := emptyInventoryRemote(recovery)

	for _, test := range []struct {
		name        string
		coordinator *Coordinator
	}{
		{name: "nil coordinator"},
		{name: "nil store", coordinator: &Coordinator{remote: remote}},
		{name: "nil remote", coordinator: &Coordinator{store: store}},
		{name: "typed nil remote", coordinator: &Coordinator{store: store, remote: (*remoteFixture)(nil)}},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := test.coordinator.bindPreparedRecoveryRegistration(recovery.ProjectID, recovery, prepared)
			if !reflect.DeepEqual(got, preparedRecoveryRegistration{}) {
				t.Fatal("invalid coordinator returned a registration binding")
			}
			assertProblem(t, err, CodeInvalid, PhaseConstruction, ActionConfigure)
		})
	}

	invalidWriterRemote := emptyInventoryRemote(recovery)
	invalidWriter := &Coordinator{store: &continuitysqlite.Store{}, remote: invalidWriterRemote}
	got, err := invalidWriter.bindPreparedRecoveryRegistration(recovery.ProjectID, recovery, prepared)
	if !reflect.DeepEqual(got, preparedRecoveryRegistration{}) {
		t.Fatal("invalid writer returned a registration binding")
	}
	assertProblem(t, err, CodeInvalid, PhaseRecoveryValidation, ActionRepairLocalStore)
	if invalidWriterRemote.endpointCalls != 0 {
		t.Fatal("invalid writer reached remote endpoint")
	}
	assertNoRecoveryRegistrationWorkflowCalls(t, invalidWriterRemote)
}

func testBindableRecoveryCredential(t *testing.T) credential.ProjectRecoveryCredential {
	t.Helper()
	recovery := testRecoveryCredential(t)
	recovery.LastSignedCheckpoint = nil
	if err := recovery.Validate(); err != nil {
		t.Fatalf("bindable recovery credential: %v", err)
	}
	return recovery
}

func testPreparedRecoveryCredential(
	t *testing.T,
	recovery credential.ProjectRecoveryCredential,
	environmentID continuity.EnvironmentID,
	membershipGeneration uint32,
	allowedGenerations []uint32,
) credential.TrustedProjectCredential {
	t.Helper()
	seed, err := crypto.EnvironmentSeedFromBytes(testBytes(0x61, len(crypto.EnvironmentSeed{})))
	if err != nil {
		t.Fatalf("prepared environment seed: %v", err)
	}
	var tokenID credential.RelayTokenID
	copy(tokenID[:], testBytes(0x62, len(tokenID)))
	var tokenSecret credential.RelayTokenSecret
	copy(tokenSecret[:], testBytes(0x63, len(tokenSecret)))
	bearer, err := credential.NewRelayBearer(tokenID, tokenSecret)
	if err != nil {
		t.Fatalf("prepared relay bearer: %v", err)
	}
	certificate, err := crypto.SignEnvironmentCertificate(protocol.EnvironmentCertificate{
		Version:               protocol.CertificateVersionV1,
		ProtocolVersion:       protocol.ProtocolVersionV1,
		CipherSuite:           protocol.CipherSuiteXChaCha20Poly1305,
		ProjectID:             recovery.ProjectID,
		ChannelID:             recovery.ChannelID,
		EnvironmentID:         environmentID,
		EnvironmentPublicKey:  crypto.EnvironmentPublicKey(seed),
		Mode:                  protocol.EnvironmentTrusted,
		MembershipGeneration:  membershipGeneration,
		AllowedKeyGenerations: append([]uint32(nil), allowedGenerations...),
	}, recovery.AdminSeed)
	if err != nil {
		t.Fatalf("sign prepared certificate: %v", err)
	}
	prepared := credential.TrustedProjectCredential{
		ProjectID:                     recovery.ProjectID,
		RelayURL:                      recovery.RelayURL,
		RelayGeneration:               recovery.RelayGeneration,
		ChannelID:                     recovery.ChannelID,
		AdminPublicKey:                crypto.AdminPublicKey(recovery.AdminSeed),
		Certificate:                   certificate,
		EnvironmentSeed:               seed,
		EnvironmentRelayAuthorization: bearer,
		ProjectRoot:                   recovery.ProjectRoot,
		WriteGeneration:               recovery.WriteGeneration,
		MinimumProtocolVersion:        protocol.ProtocolVersionV1,
	}
	if err := prepared.Validate(); err != nil {
		t.Fatalf("prepared credential: %v", err)
	}
	return prepared
}

func assertNoRecoveryRegistrationWorkflowCalls(t *testing.T, remote *remoteFixture) {
	t.Helper()
	if remote.createCalls != 0 || remote.classifyCalls != 0 || remote.registerCalls != 0 || remote.pageCalls != 0 || remote.pruneCalls != 0 || len(remote.environmentRequests) != 0 {
		t.Fatalf("pure binding called remote workflow: create=%d classify=%d register=%d page=%d prune=%d inventory=%d",
			remote.createCalls, remote.classifyCalls, remote.registerCalls, remote.pageCalls, remote.pruneCalls, len(remote.environmentRequests))
	}
}

func assertPreparedRegistrationCarriesNoBearerSecretType(t *testing.T, registration preparedRecoveryRegistration) {
	t.Helper()
	registrationType := reflect.TypeOf(registration)
	wantFields := []struct {
		name   string
		typeOf reflect.Type
	}{
		{name: "targetMembershipGeneration", typeOf: reflect.TypeOf(uint32(0))},
		{name: "certificateID", typeOf: reflect.TypeOf(relay.Digest{})},
		{name: "environmentTokenID", typeOf: reflect.TypeOf(relay.RelayTokenID{})},
		{name: "environmentTokenHash", typeOf: reflect.TypeOf(relay.TokenHash{})},
		{name: "environment", typeOf: reflect.TypeOf(relay.Environment{})},
	}
	if registrationType.NumField() != len(wantFields) {
		t.Fatalf("prepared registration fields = %d, want exact authorization-free schema of %d fields", registrationType.NumField(), len(wantFields))
	}
	for index, want := range wantFields {
		field := registrationType.Field(index)
		if field.Name != want.name || field.Type != want.typeOf {
			t.Fatalf("prepared registration field %d = {%q %v}, want {%q %v}", index, field.Name, field.Type, want.name, want.typeOf)
		}
	}

	forbidden := map[reflect.Type]struct{}{
		reflect.TypeOf(relay.OwnerAuthorization{}):              {},
		reflect.TypeOf(relay.EnvironmentAuthorization{}):        {},
		reflect.TypeOf(relay.RegisterEnvironmentRequest{}):      {},
		reflect.TypeOf(relay.RelayTokenSecret{}):                {},
		reflect.TypeOf(credential.RelayBearer{}):                {},
		reflect.TypeOf(credential.RelayTokenSecret{}):           {},
		reflect.TypeOf(credential.ProjectRecoveryCredential{}):  {},
		reflect.TypeOf(credential.TrustedProjectCredential{}):   {},
		reflect.TypeOf(credential.EphemeralProjectCredential{}): {},
	}
	visited := make(map[reflect.Type]struct{})
	var inspect func(string, reflect.Type)
	inspect = func(path string, current reflect.Type) {
		if _, found := forbidden[current]; found {
			t.Fatalf("prepared registration retains bearer authority at %s (%v)", path, current)
		}
		if _, seen := visited[current]; seen {
			return
		}
		visited[current] = struct{}{}
		switch current.Kind() {
		case reflect.Array, reflect.Pointer, reflect.Slice:
			inspect(path+"[]", current.Elem())
		case reflect.Map:
			inspect(path+"{key}", current.Key())
			inspect(path+"{value}", current.Elem())
		case reflect.Struct:
			for index := 0; index < current.NumField(); index++ {
				field := current.Field(index)
				inspect(path+"."+field.Name, field.Type)
			}
		}
	}
	inspect("preparedRecoveryRegistration", registrationType)
}
