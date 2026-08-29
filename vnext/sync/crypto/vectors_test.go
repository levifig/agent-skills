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
	HKDFSalt             string `json:"hkdf_salt_hex"`
	HKDFInfo             string `json:"hkdf_info_hex"`
	GenerationKey        string `json:"generation_key_hex"`
	AdminPublicKey       string `json:"admin_public_key_hex"`
	EnvironmentPublicKey string `json:"environment_public_key_hex"`
	CertificateBody      string `json:"certificate_body_hex"`
	CertificateSignature string `json:"certificate_signature_hex"`
	CertificateID        string `json:"certificate_id_hex"`
	Nonce                string `json:"nonce_hex"`
	Ciphertext           string `json:"ciphertext_hex"`
	FactSignature        string `json:"fact_signature_hex"`
	EnvelopeDigest       string `json:"envelope_digest_hex"`
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

	actual := cryptoVectorV1{
		HKDFSalt:             hex.EncodeToString(salt),
		HKDFInfo:             hex.EncodeToString(info),
		GenerationKey:        hex.EncodeToString(keyBytes[:]),
		AdminPublicKey:       hex.EncodeToString(adminPublic[:]),
		EnvironmentPublicKey: hex.EncodeToString(environmentPublic[:]),
		CertificateBody:      hex.EncodeToString(certificateBody),
		CertificateSignature: hex.EncodeToString(certificate.AdminSignature[:]),
		CertificateID:        hex.EncodeToString(certificateID[:]),
		Nonce:                hex.EncodeToString(sealed.Header.Nonce[:]),
		Ciphertext:           hex.EncodeToString(sealed.Ciphertext),
		FactSignature:        hex.EncodeToString(sealed.Signature[:]),
		EnvelopeDigest:       hex.EncodeToString(envelopeDigest[:]),
	}

	if actual != expected {
		actualJSON, marshalErr := json.MarshalIndent(actual, "", "  ")
		if marshalErr != nil {
			t.Fatalf("marshal actual vector: %v", marshalErr)
		}
		t.Fatalf("crypto vector mismatch; actual:\n%s", actualJSON)
	}
}
