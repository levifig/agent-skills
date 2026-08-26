package crypto

import (
	"testing"
)

func TestEmergencyKitSetupAndRecovery(t *testing.T) {
	master, code, err := EmergencyKitSetup()
	if err != nil {
		t.Fatalf("EmergencyKitSetup() error = %v", err)
	}
	if code == "" {
		t.Fatal("recovery code is empty")
	}
	recovered, err := RecoverMasterKeyFromEmergencyKit(code)
	if err != nil {
		t.Fatalf("RecoverMasterKeyFromEmergencyKit() error = %v", err)
	}
	if recovered != master {
		t.Fatalf("recovered master mismatch")
	}
}

func TestEmergencyKitDecryptsExistingSubstrate(t *testing.T) {
	master, code, err := EmergencyKitSetup()
	if err != nil {
		t.Fatalf("EmergencyKitSetup() error = %v", err)
	}
	ring := NewProjectKeyRing(master, "proj_kit_test_00000001")
	writeKey, err := ring.WriteKey()
	if err != nil {
		t.Fatalf("WriteKey() error = %v", err)
	}
	plain := FactPlaintext{Kind: "journal", Payload: "existing-substrate", EnvID: "env-a", Seq: 1, HLC: "1:0", EnvelopeV: 1, KeyGen: 0}
	envelope, err := EncryptFactEnvelope(writeKey, "fact-kit-1", plain)
	if err != nil {
		t.Fatalf("EncryptFactEnvelope() error = %v", err)
	}
	recoveredMaster, err := RecoverMasterKeyFromEmergencyKit(code)
	if err != nil {
		t.Fatalf("RecoverMasterKeyFromEmergencyKit() error = %v", err)
	}
	recoveredRing := NewProjectKeyRing(recoveredMaster, ring.ProjectID)
	readKeys, err := recoveredRing.ReadKeys()
	if err != nil {
		t.Fatalf("ReadKeys() error = %v", err)
	}
	got, err := DecryptFactEnvelope(readKeys, envelope)
	if err != nil {
		t.Fatalf("DecryptFactEnvelope() error = %v", err)
	}
	if got.Payload != plain.Payload {
		t.Fatalf("payload = %q, want %q", got.Payload, plain.Payload)
	}
}

func TestMasterKeyFingerprintIsDigestNotRawKey(t *testing.T) {
	master, err := MasterKeyFromBytes(make([]byte, masterKeySize))
	if err != nil {
		t.Fatalf("MasterKeyFromBytes() error = %v", err)
	}
	for i := range master {
		master[i] = byte(i + 3)
	}
	fp := MasterKeyFingerprint(master)
	if fp == master.Hex() {
		t.Fatal("MasterKeyFingerprint() must not equal raw master hex")
	}
	if len(fp) != 64 {
		t.Fatalf("fingerprint length = %d, want 64 hex chars", len(fp))
	}
	if again := MasterKeyFingerprint(master); again != fp {
		t.Fatalf("fingerprint unstable: %q vs %q", fp, again)
	}
	other := master
	other[0] ^= 0xff
	if MasterKeyFingerprint(other) == fp {
		t.Fatal("distinct keys must not share fingerprint")
	}
}
