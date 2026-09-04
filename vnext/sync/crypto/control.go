package crypto

import (
	"crypto/ed25519"
	"errors"

	"github.com/levifig/loaf/vnext/sync/protocol"
)

var (
	// ErrInvalidControlSignature reports an invalid Ed25519 signature on a
	// signed acknowledgement or administrator control object.
	ErrInvalidControlSignature = errors.New("sync crypto: invalid control signature")
	// ErrControlBinding reports a control object that is not bound to the exact
	// channel, environment, or certificate supplied by its caller.
	ErrControlBinding = errors.New("sync crypto: control object binding mismatch")
)

// SignProgressAcknowledgement verifies the named environment certificate under
// the supplied administrator key, then signs the exact canonical progress
// body with that certificate's environment identity. Certificate expiry is an
// admission policy and is intentionally checked by the caller at the relevant
// time.
func SignProgressAcknowledgement(
	acknowledgement protocol.ProgressAcknowledgement,
	certificate protocol.EnvironmentCertificate,
	adminPublic protocol.PublicKey,
	environmentSeed EnvironmentSeed,
) (protocol.ProgressAcknowledgement, error) {
	if zeroBytes(environmentSeed[:]) {
		return protocol.ProgressAcknowledgement{}, ErrInvalidSigningKey
	}
	if err := VerifyEnvironmentCertificate(certificate, adminPublic); err != nil {
		return protocol.ProgressAcknowledgement{}, err
	}
	if acknowledgement.ChannelID != certificate.ChannelID ||
		acknowledgement.EnvironmentID != certificate.EnvironmentID ||
		acknowledgement.CertificateID != protocol.CertificateID(certificate) ||
		EnvironmentPublicKey(environmentSeed) != certificate.EnvironmentPublicKey {
		return protocol.ProgressAcknowledgement{}, ErrControlBinding
	}
	body, err := protocol.ProgressAcknowledgementBodyTranscript(acknowledgement)
	if err != nil {
		return protocol.ProgressAcknowledgement{}, err
	}
	signature := ed25519.Sign(ed25519.NewKeyFromSeed(environmentSeed[:]), body)
	copy(acknowledgement.EnvironmentSignature[:], signature)
	return acknowledgement, nil
}

// VerifyProgressAcknowledgement verifies the administrator certificate, exact
// channel/environment/certificate binding, and environment signature. It does
// not apply certificate expiry; relay admission supplies that policy.
func VerifyProgressAcknowledgement(
	acknowledgement protocol.ProgressAcknowledgement,
	certificate protocol.EnvironmentCertificate,
	adminPublic protocol.PublicKey,
) error {
	if err := acknowledgement.Validate(); err != nil {
		return err
	}
	if err := VerifyEnvironmentCertificate(certificate, adminPublic); err != nil {
		return err
	}
	if acknowledgement.ChannelID != certificate.ChannelID ||
		acknowledgement.EnvironmentID != certificate.EnvironmentID ||
		acknowledgement.CertificateID != protocol.CertificateID(certificate) {
		return ErrControlBinding
	}
	body, err := protocol.ProgressAcknowledgementBodyTranscript(acknowledgement)
	if err != nil {
		return err
	}
	if !ed25519.Verify(ed25519.PublicKey(certificate.EnvironmentPublicKey[:]), body, acknowledgement.EnvironmentSignature[:]) {
		return ErrInvalidControlSignature
	}
	return nil
}

// SignPruneAcknowledgement verifies the named environment certificate under
// the supplied administrator key, then signs the exact canonical prune-vote
// body with that certificate's environment identity. The embedded proposal
// bindings are validated by the protocol package and the
// environment/certificate identity is checked here before signing.
func SignPruneAcknowledgement(
	acknowledgement protocol.PruneAcknowledgement,
	certificate protocol.EnvironmentCertificate,
	adminPublic protocol.PublicKey,
	environmentSeed EnvironmentSeed,
) (protocol.PruneAcknowledgement, error) {
	if zeroBytes(environmentSeed[:]) {
		return protocol.PruneAcknowledgement{}, ErrInvalidSigningKey
	}
	if err := VerifyEnvironmentCertificate(certificate, adminPublic); err != nil {
		return protocol.PruneAcknowledgement{}, err
	}
	if acknowledgement.ChannelID != certificate.ChannelID ||
		acknowledgement.EnvironmentID != certificate.EnvironmentID ||
		acknowledgement.CertificateID != protocol.CertificateID(certificate) ||
		EnvironmentPublicKey(environmentSeed) != certificate.EnvironmentPublicKey {
		return protocol.PruneAcknowledgement{}, ErrControlBinding
	}
	body, err := protocol.PruneAcknowledgementBodyTranscript(acknowledgement)
	if err != nil {
		return protocol.PruneAcknowledgement{}, err
	}
	signature := ed25519.Sign(ed25519.NewKeyFromSeed(environmentSeed[:]), body)
	copy(acknowledgement.EnvironmentSignature[:], signature)
	return acknowledgement, nil
}

// VerifyPruneAcknowledgement verifies the administrator certificate, exact
// channel/environment/certificate binding, and environment signature. The
// referenced progress and prune proposal are validated by the protocol and
// service layers.
func VerifyPruneAcknowledgement(
	acknowledgement protocol.PruneAcknowledgement,
	certificate protocol.EnvironmentCertificate,
	adminPublic protocol.PublicKey,
) error {
	if err := acknowledgement.Validate(); err != nil {
		return err
	}
	if err := VerifyEnvironmentCertificate(certificate, adminPublic); err != nil {
		return err
	}
	if acknowledgement.ChannelID != certificate.ChannelID ||
		acknowledgement.EnvironmentID != certificate.EnvironmentID ||
		acknowledgement.CertificateID != protocol.CertificateID(certificate) {
		return ErrControlBinding
	}
	body, err := protocol.PruneAcknowledgementBodyTranscript(acknowledgement)
	if err != nil {
		return err
	}
	if !ed25519.Verify(ed25519.PublicKey(certificate.EnvironmentPublicKey[:]), body, acknowledgement.EnvironmentSignature[:]) {
		return ErrInvalidControlSignature
	}
	return nil
}

// SignTerminalRetirement signs the exact canonical terminal-fence body with
// the project administrator's seed. The fence must name the exact environment
// certificate being retired.
func SignTerminalRetirement(
	retirement protocol.TerminalRetirement,
	certificate protocol.EnvironmentCertificate,
	adminSeed AdminSeed,
) (protocol.TerminalRetirement, error) {
	if zeroBytes(adminSeed[:]) {
		return protocol.TerminalRetirement{}, ErrInvalidSigningKey
	}
	if err := VerifyEnvironmentCertificate(certificate, AdminPublicKey(adminSeed)); err != nil {
		return protocol.TerminalRetirement{}, err
	}
	if retirement.ChannelID != certificate.ChannelID ||
		retirement.EnvironmentID != certificate.EnvironmentID ||
		retirement.CertificateID != protocol.CertificateID(certificate) {
		return protocol.TerminalRetirement{}, ErrControlBinding
	}
	body, err := protocol.TerminalRetirementBodyTranscript(retirement)
	if err != nil {
		return protocol.TerminalRetirement{}, err
	}
	signature := ed25519.Sign(ed25519.NewKeyFromSeed(adminSeed[:]), body)
	copy(retirement.AdminSignature[:], signature)
	return retirement, nil
}

// VerifyTerminalRetirement verifies the administrator certificate, exact
// channel/environment/certificate binding, and administrator signature.
func VerifyTerminalRetirement(
	retirement protocol.TerminalRetirement,
	certificate protocol.EnvironmentCertificate,
	adminPublic protocol.PublicKey,
) error {
	if err := retirement.Validate(); err != nil {
		return err
	}
	if err := VerifyEnvironmentCertificate(certificate, adminPublic); err != nil {
		return err
	}
	if retirement.ChannelID != certificate.ChannelID ||
		retirement.EnvironmentID != certificate.EnvironmentID ||
		retirement.CertificateID != protocol.CertificateID(certificate) {
		return ErrControlBinding
	}
	body, err := protocol.TerminalRetirementBodyTranscript(retirement)
	if err != nil {
		return err
	}
	if !ed25519.Verify(ed25519.PublicKey(adminPublic[:]), body, retirement.AdminSignature[:]) {
		return ErrInvalidControlSignature
	}
	return nil
}

// SignPruneCertificate verifies the exact active environment witness set, then
// signs the canonical self-contained prune certificate body with the project
// administrator's seed. environmentCertificates must be the authoritative
// environment-certificate set in the same canonical order as the embedded
// acknowledgements; missing, extra, duplicate, or reordered witnesses fail.
func SignPruneCertificate(
	certificate protocol.PruneCertificate,
	environmentCertificates []protocol.EnvironmentCertificate,
	adminSeed AdminSeed,
) (protocol.PruneCertificate, error) {
	if zeroBytes(adminSeed[:]) {
		return protocol.PruneCertificate{}, ErrInvalidSigningKey
	}
	if err := certificate.Validate(); err != nil {
		return protocol.PruneCertificate{}, err
	}
	if err := verifyPruneWitnesses(certificate, environmentCertificates, AdminPublicKey(adminSeed)); err != nil {
		return protocol.PruneCertificate{}, err
	}
	body, err := protocol.PruneCertificateBodyTranscript(certificate)
	if err != nil {
		return protocol.PruneCertificate{}, err
	}
	signature := ed25519.Sign(ed25519.NewKeyFromSeed(adminSeed[:]), body)
	copy(certificate.AdminSignature[:], signature)
	return certificate, nil
}

// VerifyPruneCertificate verifies the exact canonical prune certificate body,
// administrator signature, and complete authoritative environment witness
// set. environmentCertificates must have one exact, canonically ordered entry
// for every embedded acknowledgement.
func VerifyPruneCertificate(
	certificate protocol.PruneCertificate,
	environmentCertificates []protocol.EnvironmentCertificate,
	adminPublic protocol.PublicKey,
) error {
	if err := certificate.Validate(); err != nil {
		return err
	}
	if zeroBytes(adminPublic[:]) {
		return ErrInvalidControlSignature
	}
	body, err := protocol.PruneCertificateBodyTranscript(certificate)
	if err != nil {
		return err
	}
	if !ed25519.Verify(ed25519.PublicKey(adminPublic[:]), body, certificate.AdminSignature[:]) {
		return ErrInvalidControlSignature
	}
	return verifyPruneWitnesses(certificate, environmentCertificates, adminPublic)
}

func verifyPruneWitnesses(
	certificate protocol.PruneCertificate,
	environmentCertificates []protocol.EnvironmentCertificate,
	adminPublic protocol.PublicKey,
) error {
	if len(environmentCertificates) != len(certificate.Acknowledgements) {
		return ErrControlBinding
	}
	for index, acknowledgement := range certificate.Acknowledgements {
		environmentCertificate := environmentCertificates[index]
		if environmentCertificate.MembershipGeneration > certificate.MembershipGeneration {
			return ErrControlBinding
		}
		if err := VerifyPruneAcknowledgement(acknowledgement, environmentCertificate, adminPublic); err != nil {
			return err
		}
	}
	return nil
}
