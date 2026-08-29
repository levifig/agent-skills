package crypto

import (
	"crypto/ed25519"
	"crypto/subtle"

	"github.com/levifig/loaf/vnext/sync/protocol"
)

// AdminPublicKey derives the public verification key for one admin seed.
func AdminPublicKey(seed AdminSeed) protocol.PublicKey {
	return publicKeyFromSeed(seed[:])
}

// EnvironmentPublicKey derives the public verification key for one
// environment seed.
func EnvironmentPublicKey(seed EnvironmentSeed) protocol.PublicKey {
	return publicKeyFromSeed(seed[:])
}

// SignEnvironmentCertificate applies the project administrator's Ed25519
// signature to the complete canonical certificate body.
func SignEnvironmentCertificate(certificate protocol.EnvironmentCertificate, seed AdminSeed) (protocol.EnvironmentCertificate, error) {
	if zeroBytes(seed[:]) {
		return protocol.EnvironmentCertificate{}, ErrInvalidSigningKey
	}
	if err := certificate.Validate(); err != nil {
		return protocol.EnvironmentCertificate{}, err
	}
	transcript, err := protocol.CertificateBodyTranscript(certificate)
	if err != nil {
		return protocol.EnvironmentCertificate{}, err
	}
	signature := ed25519.Sign(ed25519.NewKeyFromSeed(seed[:]), transcript)
	copy(certificate.AdminSignature[:], signature)
	return certificate, nil
}

// VerifyEnvironmentCertificate verifies the project-admin signature over every
// certificate body field. Expiry is checked separately with ValidateAt.
func VerifyEnvironmentCertificate(certificate protocol.EnvironmentCertificate, adminPublic protocol.PublicKey) error {
	if zeroBytes(adminPublic[:]) {
		return ErrInvalidCertificateSignature
	}
	if err := certificate.Validate(); err != nil {
		return err
	}
	transcript, err := protocol.CertificateBodyTranscript(certificate)
	if err != nil {
		return err
	}
	if !ed25519.Verify(ed25519.PublicKey(adminPublic[:]), transcript, certificate.AdminSignature[:]) {
		return ErrInvalidCertificateSignature
	}
	return nil
}

func publicKeyFromSeed(seed []byte) protocol.PublicKey {
	privateKey := ed25519.NewKeyFromSeed(seed)
	publicBytes := privateKey.Public().(ed25519.PublicKey)
	var publicKey protocol.PublicKey
	copy(publicKey[:], publicBytes)
	return publicKey
}

func seedMatchesPublic(seed []byte, public protocol.PublicKey) bool {
	derived := publicKeyFromSeed(seed)
	return subtle.ConstantTimeCompare(derived[:], public[:]) == 1
}
