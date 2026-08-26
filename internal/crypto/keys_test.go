package crypto

import (
	"encoding/hex"
	"testing"
)

func TestDeriveProjectKeyGenerationRing(t *testing.T) {
	master, err := MasterKeyFromBytes(make([]byte, masterKeySize))
	if err != nil {
		t.Fatalf("MasterKeyFromBytes() error = %v", err)
	}
	for i := range master {
		master[i] = byte(i + 1)
	}
	projectID := "proj_ring_test_00000001"
	gen0, err := DeriveProjectKey(master, projectID, 0)
	if err != nil {
		t.Fatalf("DeriveProjectKey(gen0) error = %v", err)
	}
	gen1, err := DeriveProjectKey(master, projectID, 1)
	if err != nil {
		t.Fatalf("DeriveProjectKey(gen1) error = %v", err)
	}
	if gen0 == gen1 {
		t.Fatal("generation 0 and 1 keys must differ")
	}
	ring := NewProjectKeyRing(master, projectID)
	ring.Rotate()
	readKeys, err := ring.ReadKeys()
	if err != nil {
		t.Fatalf("ReadKeys() error = %v", err)
	}
	if len(readKeys) != 2 {
		t.Fatalf("ReadKeys() len = %d, want 2", len(readKeys))
	}
	if readKeys[0] != gen0 || readKeys[1] != gen1 {
		t.Fatalf("read ring keys mismatch gen0=%x gen1=%x", readKeys[0], readKeys[1])
	}
}

func TestPublishedProjectKeyVector(t *testing.T) {
	const want = "535cd60458a448749c8f88aba609a2853f9995d3a59e773ae49df4808f21bb4e"
	raw, err := hex.DecodeString("00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff")
	if err != nil {
		t.Fatalf("DecodeString() error = %v", err)
	}
	master, err := MasterKeyFromBytes(raw)
	if err != nil {
		t.Fatalf("MasterKeyFromBytes() error = %v", err)
	}
	key, err := DeriveProjectKey(master, "proj_test_vectors_00000001", 0)
	if err != nil {
		t.Fatalf("DeriveProjectKey() error = %v", err)
	}
	got := hex.EncodeToString(key[:])
	if len(got) != 64 {
		t.Fatalf("project key hex length = %d, want 64", len(got))
	}
	if got != want {
		t.Fatalf("project key = %s, want %s", got, want)
	}
}
