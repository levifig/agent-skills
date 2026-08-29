package crypto

import (
	"bytes"
	"errors"
	"testing"

	"github.com/levifig/loaf/vnext/continuity"
	"github.com/levifig/loaf/vnext/internal/continuitywire"
	"github.com/levifig/loaf/vnext/sync/protocol"
)

const testAtMillis int64 = 1_900_000_000_000

func TestGenerationKeyDerivationIsProjectAndGenerationScoped(t *testing.T) {
	t.Parallel()

	root := testProjectRoot(t)
	first, err := DeriveGenerationKey(root, "project-1", 7)
	if err != nil {
		t.Fatalf("derive first generation key: %v", err)
	}
	repeated, err := DeriveGenerationKey(root, "project-1", 7)
	if err != nil {
		t.Fatalf("repeat generation key derivation: %v", err)
	}
	if first != repeated {
		t.Fatal("same derivation inputs produced different keys")
	}

	differentProject, err := DeriveGenerationKey(root, "project-2", 7)
	if err != nil {
		t.Fatalf("derive different-project key: %v", err)
	}
	differentGeneration, err := DeriveGenerationKey(root, "project-1", 8)
	if err != nil {
		t.Fatalf("derive different-generation key: %v", err)
	}
	if first == differentProject || first == differentGeneration || differentProject == differentGeneration {
		t.Fatal("distinct project/generation contexts produced the same key")
	}
}

func TestEnvironmentCertificateRequiresAdminSignature(t *testing.T) {
	t.Parallel()

	adminSeed := testAdminSeed(t, 0x10)
	adminPublic := AdminPublicKey(adminSeed)
	environmentSeed := testEnvironmentSeed(t, 0x40)
	certificate := testUnsignedCertificate(EnvironmentPublicKey(environmentSeed))

	signed, err := SignEnvironmentCertificate(certificate, adminSeed)
	if err != nil {
		t.Fatalf("sign certificate: %v", err)
	}
	if err := VerifyEnvironmentCertificate(signed, adminPublic); err != nil {
		t.Fatalf("verify certificate: %v", err)
	}

	tampered := signed
	tampered.AllowedKeyGenerations = []uint32{7, 9}
	if err := VerifyEnvironmentCertificate(tampered, adminPublic); !errors.Is(err, ErrInvalidCertificateSignature) {
		t.Fatalf("tampered certificate error = %v, want %v", err, ErrInvalidCertificateSignature)
	}

	wrongAdmin := AdminPublicKey(testAdminSeed(t, 0x11))
	if err := VerifyEnvironmentCertificate(signed, wrongAdmin); !errors.Is(err, ErrInvalidCertificateSignature) {
		t.Fatalf("wrong-admin error = %v, want %v", err, ErrInvalidCertificateSignature)
	}
}

func TestSealOpenFactAuthenticatesWriterAndOuterInnerBindings(t *testing.T) {
	t.Parallel()

	root := testProjectRoot(t)
	key, err := DeriveGenerationKey(root, "project-1", 7)
	if err != nil {
		t.Fatalf("derive key: %v", err)
	}
	adminSeed := testAdminSeed(t, 0x10)
	adminPublic := AdminPublicKey(adminSeed)
	environmentSeed := testEnvironmentSeed(t, 0x40)
	certificate, err := SignEnvironmentCertificate(testUnsignedCertificate(EnvironmentPublicKey(environmentSeed)), adminSeed)
	if err != nil {
		t.Fatalf("sign certificate: %v", err)
	}
	fact := testPlaintextFact()

	sealed, err := SealFact(fact, key, certificate, adminPublic, environmentSeed, protocol.Digest{}, testAtMillis)
	if err != nil {
		t.Fatalf("seal fact: %v", err)
	}
	opened, err := OpenFact(sealed, key, certificate, adminPublic)
	if err != nil {
		t.Fatalf("open fact: %v", err)
	}
	if !continuitywire.Equal(opened, fact) {
		t.Fatalf("opened fact = %#v, want %#v", opened, fact)
	}

	tampered := sealed
	tampered.Header.FactID = "fact-2"
	if _, err := OpenFact(tampered, key, certificate, adminPublic); !errors.Is(err, ErrInvalidEnvironmentSignature) {
		t.Fatalf("tampered header error = %v, want %v", err, ErrInvalidEnvironmentSignature)
	}

	wrongKey, err := DeriveGenerationKey(root, "project-1", 8)
	if err != nil {
		t.Fatalf("derive wrong key: %v", err)
	}
	wrongGenerationMaterial, err := NewGenerationKey("project-1", 7, wrongKey.Bytes())
	if err != nil {
		t.Fatalf("relabel wrong key material: %v", err)
	}
	if _, err := OpenFact(sealed, wrongGenerationMaterial, certificate, adminPublic); !errors.Is(err, ErrAuthenticationFailed) {
		t.Fatalf("wrong-key error = %v, want %v", err, ErrAuthenticationFailed)
	}
}

func TestOpenFactRejectsAuthenticatedOuterInnerMismatch(t *testing.T) {
	t.Parallel()

	root := testProjectRoot(t)
	key, err := DeriveGenerationKey(root, "project-1", 7)
	if err != nil {
		t.Fatalf("derive key: %v", err)
	}
	adminSeed := testAdminSeed(t, 0x10)
	adminPublic := AdminPublicKey(adminSeed)
	environmentSeed := testEnvironmentSeed(t, 0x40)
	certificate, err := SignEnvironmentCertificate(testUnsignedCertificate(EnvironmentPublicKey(environmentSeed)), adminSeed)
	if err != nil {
		t.Fatalf("sign certificate: %v", err)
	}
	inner := testPlaintextFact()
	inner.FactID = "fact-2"
	encoded, err := continuitywire.Encode(inner)
	if err != nil {
		t.Fatalf("encode mismatched inner fact: %v", err)
	}
	header := protocol.FactHeader{
		ProtocolVersion:     protocol.ProtocolVersionV1,
		CipherSuite:         protocol.CipherSuiteXChaCha20Poly1305,
		ChannelID:           certificate.ChannelID,
		FactID:              "fact-1",
		EnvironmentID:       certificate.EnvironmentID,
		EnvironmentSequence: 1,
		KeyGeneration:       key.Generation(),
		CertificateID:       protocol.CertificateID(certificate),
		Nonce:               testNonce(),
	}
	sealed, err := sealEncoded(header, encoded, key, environmentSeed)
	if err != nil {
		t.Fatalf("seal mismatched fact: %v", err)
	}
	if _, err := OpenFact(sealed, key, certificate, adminPublic); !errors.Is(err, ErrPlaintextBinding) {
		t.Fatalf("binding error = %v, want %v", err, ErrPlaintextBinding)
	}
}

func TestSealFactRejectsCertificateOrGenerationAuthorityMismatch(t *testing.T) {
	t.Parallel()

	root := testProjectRoot(t)
	key, err := DeriveGenerationKey(root, "project-1", 7)
	if err != nil {
		t.Fatalf("derive key: %v", err)
	}
	adminSeed := testAdminSeed(t, 0x10)
	environmentSeed := testEnvironmentSeed(t, 0x40)
	certificate, err := SignEnvironmentCertificate(testUnsignedCertificate(EnvironmentPublicKey(environmentSeed)), adminSeed)
	if err != nil {
		t.Fatalf("sign certificate: %v", err)
	}

	wrongProject := testPlaintextFact()
	wrongProject.ProjectID = "project-2"
	wrongProject.SubjectID = "project-2"
	adminPublic := AdminPublicKey(adminSeed)
	if _, err := SealFact(wrongProject, key, certificate, adminPublic, environmentSeed, protocol.Digest{}, testAtMillis); !errors.Is(err, ErrCertificateBinding) {
		t.Fatalf("wrong-project error = %v, want %v", err, ErrCertificateBinding)
	}

	key8, err := DeriveGenerationKey(root, "project-1", 8)
	if err != nil {
		t.Fatalf("derive generation 8: %v", err)
	}
	if _, err := SealFact(testPlaintextFact(), key8, certificate, adminPublic, environmentSeed, protocol.Digest{}, testAtMillis); !errors.Is(err, ErrGenerationNotAllowed) {
		t.Fatalf("generation error = %v, want %v", err, ErrGenerationNotAllowed)
	}
}

func TestCryptoSealAndAdmissionOpenEnforceEphemeralCertificateExpiry(t *testing.T) {
	t.Parallel()

	root := testProjectRoot(t)
	key, err := DeriveGenerationKey(root, "project-1", 7)
	if err != nil {
		t.Fatalf("derive key: %v", err)
	}
	adminSeed := testAdminSeed(t, 0x10)
	adminPublic := AdminPublicKey(adminSeed)
	environmentSeed := testEnvironmentSeed(t, 0x40)
	unsigned := testUnsignedCertificate(EnvironmentPublicKey(environmentSeed))
	unsigned.Mode = protocol.EnvironmentEphemeral
	unsigned.ExpiresAtMillis = 10_000
	certificate, err := SignEnvironmentCertificate(unsigned, adminSeed)
	if err != nil {
		t.Fatalf("sign certificate: %v", err)
	}
	sealed, err := SealFact(testPlaintextFact(), key, certificate, adminPublic, environmentSeed, protocol.Digest{}, 9_999)
	if err != nil {
		t.Fatalf("seal before expiry: %v", err)
	}
	if _, err := SealFact(testPlaintextFact(), key, certificate, adminPublic, environmentSeed, protocol.Digest{}, 10_000); !errors.Is(err, protocol.ErrCertificateExpired) {
		t.Fatalf("seal at expiry error = %v, want %v", err, protocol.ErrCertificateExpired)
	}
	if _, err := OpenFact(sealed, key, certificate, adminPublic); err != nil {
		t.Fatalf("historical open after expiry: %v", err)
	}
	if _, err := OpenFactForAdmission(sealed, key, certificate, adminPublic, 10_000); !errors.Is(err, protocol.ErrCertificateExpired) {
		t.Fatalf("admission open at expiry error = %v, want %v", err, protocol.ErrCertificateExpired)
	}
}

func TestGenerationKeyCannotCrossProjectBinding(t *testing.T) {
	t.Parallel()

	root := testProjectRoot(t)
	projectBKey, err := DeriveGenerationKey(root, "project-2", 7)
	if err != nil {
		t.Fatalf("derive project-B key: %v", err)
	}
	adminSeed := testAdminSeed(t, 0x10)
	adminPublic := AdminPublicKey(adminSeed)
	environmentSeed := testEnvironmentSeed(t, 0x40)
	certificate, err := SignEnvironmentCertificate(testUnsignedCertificate(EnvironmentPublicKey(environmentSeed)), adminSeed)
	if err != nil {
		t.Fatalf("sign certificate: %v", err)
	}
	if _, err := SealFact(testPlaintextFact(), projectBKey, certificate, adminPublic, environmentSeed, protocol.Digest{}, testAtMillis); !errors.Is(err, ErrGenerationBinding) {
		t.Fatalf("cross-project key error = %v, want %v", err, ErrGenerationBinding)
	}
}

func TestCryptoErrorsDoNotContainSecretsOrPlaintext(t *testing.T) {
	t.Parallel()

	root := testProjectRoot(t)
	key, err := DeriveGenerationKey(root, "project-1", 7)
	if err != nil {
		t.Fatalf("derive key: %v", err)
	}
	adminSeed := testAdminSeed(t, 0x10)
	adminPublic := AdminPublicKey(adminSeed)
	environmentSeed := testEnvironmentSeed(t, 0x40)
	certificate, err := SignEnvironmentCertificate(testUnsignedCertificate(EnvironmentPublicKey(environmentSeed)), adminSeed)
	if err != nil {
		t.Fatalf("sign certificate: %v", err)
	}
	sealed, err := SealFact(testPlaintextFact(), key, certificate, adminPublic, environmentSeed, protocol.Digest{}, testAtMillis)
	if err != nil {
		t.Fatalf("seal fact: %v", err)
	}
	sealed.Ciphertext[0] ^= 0xff
	_, openErr := OpenFact(sealed, key, certificate, adminPublic)
	if openErr == nil {
		t.Fatal("tampered ciphertext error = nil")
	}
	keyBytes := key.Bytes()
	for _, forbidden := range [][]byte{testPlaintextFact().CanonicalPayload, keyBytes[:]} {
		if bytes.Contains([]byte(openErr.Error()), forbidden) {
			t.Fatalf("error leaked protected bytes: %q", openErr)
		}
	}
}

func TestCryptoOpenRejectsSignedEnvelopeMutations(t *testing.T) {
	t.Parallel()

	root := testProjectRoot(t)
	key, err := DeriveGenerationKey(root, "project-1", 7)
	if err != nil {
		t.Fatalf("derive key: %v", err)
	}
	adminSeed := testAdminSeed(t, 0x10)
	adminPublic := AdminPublicKey(adminSeed)
	environmentSeed := testEnvironmentSeed(t, 0x40)
	certificate, err := SignEnvironmentCertificate(testUnsignedCertificate(EnvironmentPublicKey(environmentSeed)), adminSeed)
	if err != nil {
		t.Fatalf("sign certificate: %v", err)
	}
	sealed, err := SealFact(testPlaintextFact(), key, certificate, adminPublic, environmentSeed, protocol.Digest{}, testAtMillis)
	if err != nil {
		t.Fatalf("seal fact: %v", err)
	}
	mutations := []struct {
		name   string
		mutate func(*protocol.SealedFact)
	}{
		{name: "channel", mutate: func(value *protocol.SealedFact) { value.Header.ChannelID[0] ^= 1 }},
		{name: "fact", mutate: func(value *protocol.SealedFact) { value.Header.FactID = "fact-2" }},
		{name: "environment", mutate: func(value *protocol.SealedFact) { value.Header.EnvironmentID = "environment-b" }},
		{name: "generation", mutate: func(value *protocol.SealedFact) { value.Header.KeyGeneration = 8 }},
		{name: "certificate", mutate: func(value *protocol.SealedFact) { value.Header.CertificateID[0] ^= 1 }},
		{name: "nonce", mutate: func(value *protocol.SealedFact) { value.Header.Nonce[0] ^= 1 }},
		{name: "ciphertext", mutate: func(value *protocol.SealedFact) { value.Ciphertext[0] ^= 1 }},
		{name: "signature", mutate: func(value *protocol.SealedFact) { value.Signature[0] ^= 1 }},
	}
	for _, test := range mutations {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			candidate := sealed
			candidate.Ciphertext = append([]byte(nil), sealed.Ciphertext...)
			test.mutate(&candidate)
			if _, err := OpenFact(candidate, key, certificate, adminPublic); err == nil {
				t.Fatal("OpenFact() error = nil, want refusal")
			}
		})
	}
}

func TestCryptoSealUsesFreshRandomNonce(t *testing.T) {
	t.Parallel()

	root := testProjectRoot(t)
	key, err := DeriveGenerationKey(root, "project-1", 7)
	if err != nil {
		t.Fatalf("derive key: %v", err)
	}
	adminSeed := testAdminSeed(t, 0x10)
	adminPublic := AdminPublicKey(adminSeed)
	environmentSeed := testEnvironmentSeed(t, 0x40)
	certificate, err := SignEnvironmentCertificate(testUnsignedCertificate(EnvironmentPublicKey(environmentSeed)), adminSeed)
	if err != nil {
		t.Fatalf("sign certificate: %v", err)
	}
	first, err := SealFact(testPlaintextFact(), key, certificate, adminPublic, environmentSeed, protocol.Digest{}, testAtMillis)
	if err != nil {
		t.Fatalf("seal first fact: %v", err)
	}
	second, err := SealFact(testPlaintextFact(), key, certificate, adminPublic, environmentSeed, protocol.Digest{}, testAtMillis)
	if err != nil {
		t.Fatalf("seal second fact: %v", err)
	}
	if first.Header.Nonce == second.Header.Nonce {
		t.Fatal("independent sealing reused a random nonce")
	}
	if protocol.CompareImmutable(first, second) != protocol.ImmutableConflict {
		t.Fatal("resealing the same immutable identity was not classified as conflict")
	}
}

func testProjectRoot(t *testing.T) ProjectRoot {
	t.Helper()
	material := make([]byte, 32)
	for index := range material {
		material[index] = byte(index + 1)
	}
	root, err := ProjectRootFromBytes(material)
	if err != nil {
		t.Fatalf("create test root: %v", err)
	}
	return root
}

func testAdminSeed(t *testing.T, start byte) AdminSeed {
	t.Helper()
	material := make([]byte, 32)
	for index := range material {
		material[index] = start + byte(index)
	}
	seed, err := AdminSeedFromBytes(material)
	if err != nil {
		t.Fatalf("create admin seed: %v", err)
	}
	return seed
}

func testEnvironmentSeed(t *testing.T, start byte) EnvironmentSeed {
	t.Helper()
	material := make([]byte, 32)
	for index := range material {
		material[index] = start + byte(index)
	}
	seed, err := EnvironmentSeedFromBytes(material)
	if err != nil {
		t.Fatalf("create environment seed: %v", err)
	}
	return seed
}

func testUnsignedCertificate(environmentPublic protocol.PublicKey) protocol.EnvironmentCertificate {
	var channel protocol.ChannelID
	for index := range channel {
		channel[index] = byte(0x80 + index)
	}
	return protocol.EnvironmentCertificate{
		Version:               protocol.CertificateVersionV1,
		ProtocolVersion:       protocol.ProtocolVersionV1,
		CipherSuite:           protocol.CipherSuiteXChaCha20Poly1305,
		ProjectID:             "project-1",
		ChannelID:             channel,
		EnvironmentID:         "environment-a",
		EnvironmentPublicKey:  environmentPublic,
		Mode:                  protocol.EnvironmentTrusted,
		MembershipGeneration:  2,
		AllowedKeyGenerations: []uint32{7},
	}
}

func testPlaintextFact() continuitywire.Fact {
	return continuitywire.Fact{
		WireVersion:         continuitywire.Version1,
		FactID:              "fact-1",
		ProjectID:           "project-1",
		SubjectKind:         continuity.RecordProjectIdentity,
		SubjectID:           "project-1",
		FactKind:            continuity.FactProjectRegistered,
		PayloadVersion:      1,
		CanonicalPayload:    []byte(`{"observation":{"observed_at_millis":1,"harness_session_id":"","branch":"","worktree":""},"label":"private phrase"}`),
		EnvironmentID:       "environment-a",
		EnvironmentSequence: 1,
		HLCWallMillis:       100,
		HLCLogical:          2,
		EnvelopeVersion:     1,
	}
}

func testNonce() protocol.Nonce {
	var nonce protocol.Nonce
	for index := range nonce {
		nonce[index] = byte(0xa0 + index)
	}
	return nonce
}
