package crypto

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

type publishedVectorFile struct {
	MasterKeyHex  string        `json:"master_key_hex"`
	ProjectID     string        `json:"project_id"`
	Generation    int           `json:"generation"`
	FactID        string        `json:"fact_id"`
	Plaintext     FactPlaintext `json:"plaintext"`
	NonceHex      string        `json:"nonce_hex"`
	ProjectKeyHex string        `json:"project_key_hex"`
	CiphertextHex string        `json:"ciphertext_hex"`
}

func loadPublishedVectors(t *testing.T) publishedVectorFile {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	path := filepath.Join(filepath.Dir(file), "testdata", "published_vectors.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	var vectors publishedVectorFile
	if err := json.Unmarshal(raw, &vectors); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	return vectors
}

func TestPublishedEnvelopeVectorsRoundTrip(t *testing.T) {
	vectors := loadPublishedVectors(t)
	masterRaw, err := hex.DecodeString(vectors.MasterKeyHex)
	if err != nil {
		t.Fatalf("DecodeString(master) error = %v", err)
	}
	master, err := MasterKeyFromBytes(masterRaw)
	if err != nil {
		t.Fatalf("MasterKeyFromBytes() error = %v", err)
	}
	projectKey, err := DeriveProjectKey(master, vectors.ProjectID, vectors.Generation)
	if err != nil {
		t.Fatalf("DeriveProjectKey() error = %v", err)
	}
	if vectors.ProjectKeyHex != "" {
		want, err := hex.DecodeString(vectors.ProjectKeyHex)
		if err != nil {
			t.Fatalf("DecodeString(project key) error = %v", err)
		}
		if string(projectKey[:]) != string(want) {
			t.Fatalf("project key = %x, want %x", projectKey, want)
		}
	}
	nonce, err := hex.DecodeString(vectors.NonceHex)
	if err != nil {
		t.Fatalf("DecodeString(nonce) error = %v", err)
	}
	envelope, err := encryptFactEnvelopeWithNonce(projectKey, vectors.FactID, vectors.Plaintext, nonce)
	if err != nil {
		t.Fatalf("encryptFactEnvelopeWithNonce() error = %v", err)
	}
	if vectors.CiphertextHex != "" {
		want, err := hex.DecodeString(vectors.CiphertextHex)
		if err != nil {
			t.Fatalf("DecodeString(ciphertext) error = %v", err)
		}
		if string(envelope.Ciphertext) != string(want) {
			t.Fatalf("ciphertext = %x, want %x", envelope.Ciphertext, want)
		}
	}
	ring := NewProjectKeyRing(master, vectors.ProjectID)
	readKeys, err := ring.ReadKeys()
	if err != nil {
		t.Fatalf("ReadKeys() error = %v", err)
	}
	got, err := DecryptFactEnvelope(readKeys, envelope)
	if err != nil {
		t.Fatalf("DecryptFactEnvelope() error = %v", err)
	}
	if got != vectors.Plaintext {
		t.Fatalf("plaintext = %#v, want %#v", got, vectors.Plaintext)
	}
}

func TestEnvelopeRoundTripAcrossRotatedGeneration(t *testing.T) {
	master, err := GenerateMasterKey()
	if err != nil {
		t.Fatalf("GenerateMasterKey() error = %v", err)
	}
	ring := NewProjectKeyRing(master, "proj_rotate_test_00000001")
	writeKey, err := ring.WriteKey()
	if err != nil {
		t.Fatalf("WriteKey() error = %v", err)
	}
	plain := FactPlaintext{Kind: "journal", Payload: "{}", EnvID: "env-a", Seq: 1, HLC: "1:0", EnvelopeV: 1, KeyGen: 0}
	envelope, err := EncryptFactEnvelope(writeKey, "fact-rotate-1", plain)
	if err != nil {
		t.Fatalf("EncryptFactEnvelope() error = %v", err)
	}
	ring.Rotate()
	newWrite, err := ring.WriteKey()
	if err != nil {
		t.Fatalf("WriteKey() after rotate error = %v", err)
	}
	plain.KeyGen = 1
	plain.Seq = 2
	envelope2, err := EncryptFactEnvelope(newWrite, "fact-rotate-2", plain)
	if err != nil {
		t.Fatalf("EncryptFactEnvelope(gen1) error = %v", err)
	}
	readKeys, err := ring.ReadKeys()
	if err != nil {
		t.Fatalf("ReadKeys() error = %v", err)
	}
	if _, err := DecryptFactEnvelope(readKeys, envelope); err != nil {
		t.Fatalf("decrypt gen0 envelope error = %v", err)
	}
	if _, err := DecryptFactEnvelope(readKeys, envelope2); err != nil {
		t.Fatalf("decrypt gen1 envelope error = %v", err)
	}
}
