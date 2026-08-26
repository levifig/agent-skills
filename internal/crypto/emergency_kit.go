package crypto

import (
	"crypto/sha256"
	"encoding/base32"
	"encoding/hex"
	"fmt"
	"strings"
)

const (
	recoveryCodeWords  = 16
	recoveryKitVersion = 1
)

var recoveryWordEncoder = base32.StdEncoding.WithPadding(base32.NoPadding)

// EmergencyKitSetup mints a master key and a one-time printable recovery code.
func EmergencyKitSetup() (MasterKey, string, error) {
	master, err := GenerateMasterKey()
	if err != nil {
		return MasterKey{}, "", err
	}
	code, err := EncodeEmergencyKit(master)
	if err != nil {
		return MasterKey{}, "", err
	}
	return master, code, nil
}

// EncodeEmergencyKit renders a master key as a grouped recovery code shown once at setup.
func EncodeEmergencyKit(master MasterKey) (string, error) {
	payload := append([]byte{recoveryKitVersion}, master[:]...)
	encoded := recoveryWordEncoder.EncodeToString(payload)
	chunk := (len(encoded) + recoveryCodeWords - 1) / recoveryCodeWords
	words := make([]string, recoveryCodeWords)
	for i := 0; i < recoveryCodeWords; i++ {
		start := i * chunk
		end := start + chunk
		if start >= len(encoded) {
			words[i] = ""
			continue
		}
		if end > len(encoded) {
			end = len(encoded)
		}
		words[i] = encoded[start:end]
	}
	return strings.Join(words, "-"), nil
}

// RecoverMasterKeyFromEmergencyKit restores the master key from the printable code alone.
func RecoverMasterKeyFromEmergencyKit(code string) (MasterKey, error) {
	normalized := strings.ToUpper(strings.TrimSpace(code))
	normalized = strings.ReplaceAll(normalized, " ", "")
	words := strings.Split(normalized, "-")
	if len(words) != recoveryCodeWords {
		return MasterKey{}, fmt.Errorf("recover master key: expected %d words, got %d", recoveryCodeWords, len(words))
	}
	encoded := strings.Join(words, "")
	raw, err := recoveryWordEncoder.DecodeString(encoded)
	if err != nil {
		return MasterKey{}, fmt.Errorf("recover master key: decode recovery code: %w", err)
	}
	if len(raw) != 1+masterKeySize || raw[0] != recoveryKitVersion {
		return MasterKey{}, fmt.Errorf("recover master key: invalid recovery kit payload")
	}
	return MasterKeyFromBytes(raw[1:])
}

// MasterKeyFingerprint returns a stable SHA-256 hex digest for diagnostics.
// It never returns the raw master key material.
func MasterKeyFingerprint(master MasterKey) string {
	sum := sha256.Sum256(master[:])
	return hex.EncodeToString(sum[:])
}
