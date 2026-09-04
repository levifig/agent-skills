package credential

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
)

type credentialVectorV1 struct {
	Prefix       string `json:"prefix"`
	BodySHA256   string `json:"body_sha256_hex"`
	Checksum     string `json:"checksum_hex"`
	FramedLength int    `json:"framed_length"`
}

func TestRecoveryCredentialPublishedVectorsV1(t *testing.T) {
	t.Parallel()

	expectedBytes, err := os.ReadFile("testdata/vectors_v1.json")
	if err != nil {
		t.Fatalf("read credential vector: %v", err)
	}
	var expected map[string]credentialVectorV1
	if err := json.Unmarshal(expectedBytes, &expected); err != nil {
		t.Fatalf("decode credential vector: %v", err)
	}

	recovery, trusted, ephemeral := testCredentials(t)
	recoveryWire, err := EncodeRecovery(recovery)
	if err != nil {
		t.Fatalf("encode recovery: %v", err)
	}
	trustedWire, err := EncodeTrusted(trusted)
	if err != nil {
		t.Fatalf("encode trusted: %v", err)
	}
	ephemeralWire, err := EncodeEphemeral(ephemeral)
	if err != nil {
		t.Fatalf("encode ephemeral: %v", err)
	}
	actual := map[string]credentialVectorV1{
		"recovery":  summarizeCredentialVector(t, recoveryWire, RecoveryPrefix),
		"trusted":   summarizeCredentialVector(t, trustedWire, TrustedPrefix),
		"ephemeral": summarizeCredentialVector(t, ephemeralWire, EphemeralPrefix),
	}
	if !reflect.DeepEqual(actual, expected) {
		actualJSON, marshalErr := json.MarshalIndent(actual, "", "  ")
		if marshalErr != nil {
			t.Fatalf("marshal actual vectors: %v", marshalErr)
		}
		t.Fatalf("credential vector mismatch; actual:\n%s", actualJSON)
	}
}

func summarizeCredentialVector(t *testing.T, wire, prefix string) credentialVectorV1 {
	t.Helper()
	framed, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(wire, prefix))
	if err != nil {
		t.Fatalf("decode credential frame: %v", err)
	}
	if len(framed) <= credentialChecksumBytes {
		t.Fatal("credential frame is shorter than checksum")
	}
	body := framed[:len(framed)-credentialChecksumBytes]
	checksum := framed[len(framed)-credentialChecksumBytes:]
	bodyDigest := sha256.Sum256(body)
	return credentialVectorV1{
		Prefix:       prefix,
		BodySHA256:   hex.EncodeToString(bodyDigest[:]),
		Checksum:     hex.EncodeToString(checksum),
		FramedLength: len(framed),
	}
}
