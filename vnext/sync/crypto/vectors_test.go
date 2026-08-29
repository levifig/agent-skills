package crypto

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"

	"github.com/levifig/loaf/vnext/internal/continuitywire"
	"github.com/levifig/loaf/vnext/sync/protocol"
)

type cryptoVectorV1 struct {
	HKDFSalt                  string `json:"hkdf_salt_hex"`
	HKDFInfo                  string `json:"hkdf_info_hex"`
	GenerationKey             string `json:"generation_key_hex"`
	AdminPublicKey            string `json:"admin_public_key_hex"`
	EnvironmentPublicKey      string `json:"environment_public_key_hex"`
	CertificateBody           string `json:"certificate_body_hex"`
	CertificateSignature      string `json:"certificate_signature_hex"`
	CertificateID             string `json:"certificate_id_hex"`
	Nonce                     string `json:"nonce_hex"`
	Ciphertext                string `json:"ciphertext_hex"`
	FactSignature             string `json:"fact_signature_hex"`
	EnvelopeDigest            string `json:"envelope_digest_hex"`
	PruneBootstrapKeySalt     string `json:"prune_bootstrap_key_salt_hex"`
	PruneBootstrapKeyInfo     string `json:"prune_bootstrap_key_info_hex"`
	PruneBootstrapKey         string `json:"prune_bootstrap_key_hex"`
	PruneBootstrapAEADKeySalt string `json:"prune_bootstrap_aead_key_salt_hex"`
	PruneBootstrapAEADKeyInfo string `json:"prune_bootstrap_aead_key_info_hex"`
	PruneBootstrapAEADKey     string `json:"prune_bootstrap_aead_key_hex"`
	PruneBootstrapAAD         string `json:"prune_bootstrap_aad_hex"`
	PruneBootstrapPlaintext   string `json:"prune_bootstrap_plaintext_hex"`
	PruneBootstrapNonce       string `json:"prune_bootstrap_nonce_hex"`
	PruneBootstrapCiphertext  string `json:"prune_bootstrap_ciphertext_hex"`
	PruneBootstrapCapsule     string `json:"prune_bootstrap_capsule_hex"`
	PruneBootstrapDigest      string `json:"prune_bootstrap_digest_hex"`
	PrunedArrival             string `json:"pruned_arrival_hex"`
}

func TestCryptoPublishedVectorV1(t *testing.T) {
	t.Parallel()

	expectedBytes, err := os.ReadFile("testdata/vectors_v1.json")
	if err != nil {
		t.Fatalf("read vector: %v", err)
	}
	var expected cryptoVectorV1
	if err := json.Unmarshal(expectedBytes, &expected); err != nil {
		t.Fatalf("decode vector: %v", err)
	}

	root := testProjectRoot(t)
	key, err := DeriveGenerationKey(root, "project-1", 7)
	if err != nil {
		t.Fatalf("derive generation key: %v", err)
	}
	salt, err := protocol.GenerationKeySalt("project-1")
	if err != nil {
		t.Fatalf("build HKDF salt: %v", err)
	}
	info, err := protocol.GenerationKeyInfo(protocol.ProtocolVersionV1, protocol.CipherSuiteXChaCha20Poly1305, 7)
	if err != nil {
		t.Fatalf("build HKDF info: %v", err)
	}
	adminSeed := testAdminSeed(t, 0x10)
	adminPublic := AdminPublicKey(adminSeed)
	environmentSeed := testEnvironmentSeed(t, 0x40)
	environmentPublic := EnvironmentPublicKey(environmentSeed)
	certificate, err := SignEnvironmentCertificate(testUnsignedCertificate(environmentPublic), adminSeed)
	if err != nil {
		t.Fatalf("sign certificate: %v", err)
	}
	certificateBody, err := protocol.CertificateBodyTranscript(certificate)
	if err != nil {
		t.Fatalf("build certificate body: %v", err)
	}
	certificateID := protocol.CertificateID(certificate)
	plaintext, err := continuitywire.Encode(testPlaintextFact())
	if err != nil {
		t.Fatalf("encode plaintext: %v", err)
	}
	header := protocol.FactHeader{
		ProtocolVersion:     protocol.ProtocolVersionV1,
		CipherSuite:         protocol.CipherSuiteXChaCha20Poly1305,
		ChannelID:           certificate.ChannelID,
		FactID:              "fact-1",
		EnvironmentID:       "environment-a",
		EnvironmentSequence: 1,
		KeyGeneration:       7,
		CertificateID:       certificateID,
		Nonce:               testNonce(),
	}
	sealed, err := sealEncoded(header, plaintext, key, environmentSeed)
	if err != nil {
		t.Fatalf("seal vector fact: %v", err)
	}
	envelopeDigest := protocol.EnvelopeDigest(sealed)
	keyBytes := key.Bytes()

	bootstrapPlaintext := testBootstrapPlaintext()
	bootstrapBase, err := DerivePruneBootstrapKey(root, bootstrapPlaintext.ProjectID, protocol.PruneBootstrapPurposeVersionV1)
	if err != nil {
		t.Fatalf("derive prune bootstrap key: %v", err)
	}
	bootstrapKeySalt, err := protocol.PruneBootstrapKeySalt(bootstrapPlaintext.ProjectID)
	if err != nil {
		t.Fatalf("build prune bootstrap key salt: %v", err)
	}
	bootstrapKeyInfo, err := protocol.PruneBootstrapKeyInfo(
		bootstrapPlaintext.ProtocolVersion,
		bootstrapPlaintext.CipherSuite,
		bootstrapPlaintext.BootstrapPurposeVersion,
	)
	if err != nil {
		t.Fatalf("build prune bootstrap key info: %v", err)
	}
	bootstrapHeader := bootstrapOuter(bootstrapPlaintext)
	bootstrapHeader.Nonce = controlNonce(0xe0)
	bootstrapAEADKeySalt, err := protocol.PruneBootstrapAEADKeySalt(
		bootstrapPlaintext.ProjectID,
		bootstrapHeader.ChannelID,
		bootstrapHeader.RelayGeneration,
		bootstrapHeader.PruneID,
	)
	if err != nil {
		t.Fatalf("build per-prune key salt: %v", err)
	}
	bootstrapAEADKeyInfo, err := protocol.PruneBootstrapAEADKeyInfo(
		bootstrapHeader.CapsuleVersion,
		bootstrapHeader.ProtocolVersion,
		bootstrapHeader.CipherSuite,
		bootstrapHeader.BootstrapPurposeVersion,
		bootstrapHeader.MembershipGeneration,
		bootstrapHeader.BarrierArrivalSequence,
		bootstrapHeader.ClosureReferenceDigest,
		bootstrapHeader.ManifestCount,
		bootstrapHeader.ManifestDigest,
	)
	if err != nil {
		t.Fatalf("build per-prune key info: %v", err)
	}
	bootstrapAEADKey, err := derivePruneBootstrapAEADKey(bootstrapBase, bootstrapHeader)
	if err != nil {
		t.Fatalf("derive per-prune key: %v", err)
	}
	bootstrapAAD, err := protocol.PruneBootstrapAAD(bootstrapHeader, bootstrapPlaintext.ProjectID)
	if err != nil {
		t.Fatalf("build prune bootstrap AAD: %v", err)
	}
	bootstrapPlaintextWire, err := bootstrapPlaintext.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal prune bootstrap plaintext: %v", err)
	}
	bootstrapCapsule, err := sealPruneBootstrapEncoded(bootstrapHeader, bootstrapPlaintextWire, bootstrapBase)
	if err != nil {
		t.Fatalf("seal prune bootstrap vector: %v", err)
	}
	bootstrapCapsuleWire, err := bootstrapCapsule.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal prune bootstrap capsule: %v", err)
	}
	bootstrapDigest := protocol.PruneBootstrapDigest(bootstrapCapsule)
	pruneReference := protocol.PruneReference{
		FactID:                 "fact-pruned",
		EnvironmentID:          "environment-a",
		EnvironmentSequence:    2,
		ArrivalSequence:        11,
		EnvelopeDigest:         controlDigest(0xf0),
		CertificateID:          certificateID,
		PreviousEnvelopeDigest: controlDigest(0xef),
		KeyGeneration:          7,
		Nonce:                  controlNonce(0xf1),
	}
	prunedArrival := protocol.PrunedArrival{
		ChannelID:       bootstrapHeader.ChannelID,
		RelayGeneration: bootstrapHeader.RelayGeneration,
		PruneID:         bootstrapHeader.PruneID,
		Reference:       pruneReference,
	}
	prunedArrivalWire, err := prunedArrival.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal pruned arrival: %v", err)
	}
	bootstrapBaseBytes := bootstrapBase.Bytes()

	actual := cryptoVectorV1{
		HKDFSalt:                  hex.EncodeToString(salt),
		HKDFInfo:                  hex.EncodeToString(info),
		GenerationKey:             hex.EncodeToString(keyBytes[:]),
		AdminPublicKey:            hex.EncodeToString(adminPublic[:]),
		EnvironmentPublicKey:      hex.EncodeToString(environmentPublic[:]),
		CertificateBody:           hex.EncodeToString(certificateBody),
		CertificateSignature:      hex.EncodeToString(certificate.AdminSignature[:]),
		CertificateID:             hex.EncodeToString(certificateID[:]),
		Nonce:                     hex.EncodeToString(sealed.Header.Nonce[:]),
		Ciphertext:                hex.EncodeToString(sealed.Ciphertext),
		FactSignature:             hex.EncodeToString(sealed.Signature[:]),
		EnvelopeDigest:            hex.EncodeToString(envelopeDigest[:]),
		PruneBootstrapKeySalt:     hex.EncodeToString(bootstrapKeySalt),
		PruneBootstrapKeyInfo:     hex.EncodeToString(bootstrapKeyInfo),
		PruneBootstrapKey:         hex.EncodeToString(bootstrapBaseBytes[:]),
		PruneBootstrapAEADKeySalt: hex.EncodeToString(bootstrapAEADKeySalt),
		PruneBootstrapAEADKeyInfo: hex.EncodeToString(bootstrapAEADKeyInfo),
		PruneBootstrapAEADKey:     hex.EncodeToString(bootstrapAEADKey.material[:]),
		PruneBootstrapAAD:         hex.EncodeToString(bootstrapAAD),
		PruneBootstrapPlaintext:   hex.EncodeToString(bootstrapPlaintextWire),
		PruneBootstrapNonce:       hex.EncodeToString(bootstrapCapsule.Nonce[:]),
		PruneBootstrapCiphertext:  hex.EncodeToString(bootstrapCapsule.Ciphertext),
		PruneBootstrapCapsule:     hex.EncodeToString(bootstrapCapsuleWire),
		PruneBootstrapDigest:      hex.EncodeToString(bootstrapDigest[:]),
		PrunedArrival:             hex.EncodeToString(prunedArrivalWire),
	}

	if actual != expected {
		actualJSON, marshalErr := json.MarshalIndent(actual, "", "  ")
		if marshalErr != nil {
			t.Fatalf("marshal actual vector: %v", marshalErr)
		}
		t.Fatalf("crypto vector mismatch; actual:\n%s", actualJSON)
	}
}
