package relay

import (
	"crypto/sha256"
	"crypto/subtle"
	"fmt"
)

const (
	MinimumTokenSecretBytes = 32
	MaximumTokenSecretBytes = 32
)

type TokenHash [sha256.Size]byte

func HashTokenSecret(secret RelayTokenSecret) (TokenHash, error) {
	if zeroBytes(secret[:]) {
		return TokenHash{}, fmt.Errorf("%w: zero token secret", ErrInvalidArgument)
	}
	return sha256.Sum256(secret[:]), nil
}

func VerifyTokenSecret(want TokenHash, presented RelayTokenSecret) bool {
	got := sha256.Sum256(presented[:])
	return subtle.ConstantTimeCompare(want[:], got[:]) == 1 && !zeroBytes(presented[:])
}
