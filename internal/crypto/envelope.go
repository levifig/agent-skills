package crypto

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"golang.org/x/crypto/chacha20poly1305"
)

const wireEnvelopeVersion = 1

// FactPlaintext is the semantic body encrypted inside the E2E envelope.
type FactPlaintext struct {
	Kind      string `json:"kind"`
	Payload   string `json:"payload"`
	EnvID     string `json:"env_id"`
	Seq       int64  `json:"seq"`
	HLC       string `json:"hlc"`
	EnvelopeV int    `json:"envelope_v"`
	KeyGen    int    `json:"key_gen"`
}

// WireEnvelope is the on-wire E2E fact envelope. Cleartext carries only version
// and the fact id idempotency key; everything semantic rides in Ciphertext.
type WireEnvelope struct {
	Version    int    `json:"version"`
	FactID     string `json:"fact_id"`
	Nonce      []byte `json:"nonce"`
	Ciphertext []byte `json:"ciphertext"`
}

func encryptFactEnvelopeWithNonce(projectKey [projectKeySize]byte, factID string, plaintext FactPlaintext, nonce []byte) (WireEnvelope, error) {
	factID = strings.TrimSpace(factID)
	if factID == "" {
		return WireEnvelope{}, fmt.Errorf("encrypt fact envelope: fact id is empty")
	}
	if len(nonce) != chacha20poly1305.NonceSizeX {
		return WireEnvelope{}, fmt.Errorf("encrypt fact envelope: nonce must be %d bytes", chacha20poly1305.NonceSizeX)
	}
	if strings.TrimSpace(plaintext.Kind) == "" {
		return WireEnvelope{}, fmt.Errorf("encrypt fact envelope: kind is empty")
	}
	if strings.TrimSpace(plaintext.Payload) == "" {
		return WireEnvelope{}, fmt.Errorf("encrypt fact envelope: payload is empty")
	}
	body, err := json.Marshal(plaintext)
	if err != nil {
		return WireEnvelope{}, fmt.Errorf("marshal fact plaintext: %w", err)
	}
	aead, err := chacha20poly1305.NewX(projectKey[:])
	if err != nil {
		return WireEnvelope{}, fmt.Errorf("init xchacha20-poly1305: %w", err)
	}
	aad := []byte(fmt.Sprintf("loaf-e2e-v%d/%s", wireEnvelopeVersion, factID))
	ciphertext := aead.Seal(nil, nonce, body, aad)
	return WireEnvelope{
		Version:    wireEnvelopeVersion,
		FactID:     factID,
		Nonce:      nonce,
		Ciphertext: ciphertext,
	}, nil
}

// EncryptFactEnvelope seals a fact plaintext with the project key for keyGen.
func EncryptFactEnvelope(projectKey [projectKeySize]byte, factID string, plaintext FactPlaintext) (WireEnvelope, error) {
	nonce := make([]byte, chacha20poly1305.NonceSizeX)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return WireEnvelope{}, fmt.Errorf("generate envelope nonce: %w", err)
	}
	return encryptFactEnvelopeWithNonce(projectKey, factID, plaintext, nonce)
}

// DecryptFactEnvelope opens a wire envelope using a generation ring.
func DecryptFactEnvelope(readKeys [][projectKeySize]byte, envelope WireEnvelope) (FactPlaintext, error) {
	if envelope.Version != wireEnvelopeVersion {
		return FactPlaintext{}, fmt.Errorf("decrypt fact envelope: unsupported wire version %d", envelope.Version)
	}
	factID := strings.TrimSpace(envelope.FactID)
	if factID == "" {
		return FactPlaintext{}, fmt.Errorf("decrypt fact envelope: fact id is empty")
	}
	if len(envelope.Nonce) != chacha20poly1305.NonceSizeX {
		return FactPlaintext{}, fmt.Errorf("decrypt fact envelope: nonce must be %d bytes", chacha20poly1305.NonceSizeX)
	}
	aad := []byte(fmt.Sprintf("loaf-e2e-v%d/%s", wireEnvelopeVersion, factID))
	var lastErr error
	for _, key := range readKeys {
		aead, err := chacha20poly1305.NewX(key[:])
		if err != nil {
			return FactPlaintext{}, fmt.Errorf("init xchacha20-poly1305: %w", err)
		}
		body, err := aead.Open(nil, envelope.Nonce, envelope.Ciphertext, aad)
		if err != nil {
			lastErr = err
			continue
		}
		var plaintext FactPlaintext
		if err := json.Unmarshal(body, &plaintext); err != nil {
			return FactPlaintext{}, fmt.Errorf("unmarshal fact plaintext: %w", err)
		}
		return plaintext, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no read keys provided")
	}
	return FactPlaintext{}, fmt.Errorf("decrypt fact envelope: %w", lastErr)
}
