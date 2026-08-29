package crypto

import (
	"crypto/cipher"

	"golang.org/x/crypto/chacha20poly1305"
)

func newXChaCha(key GenerationKey) (cipher.AEAD, error) {
	return chacha20poly1305.NewX(key.material[:])
}
