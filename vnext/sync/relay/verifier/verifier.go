// Package verifier implements the cryptographic admission boundary for the
// opaque vNext relay. It authenticates signed protocol objects without
// opening continuity plaintext or receiving bearer token material.
package verifier

import (
	"bytes"
	"context"
	"crypto/subtle"
	"errors"
	"fmt"

	"github.com/levifig/loaf/vnext/continuity"
	"github.com/levifig/loaf/vnext/sync/crypto"
	"github.com/levifig/loaf/vnext/sync/protocol"
	"github.com/levifig/loaf/vnext/sync/relay"
)

var (
	// ErrInvalidObject identifies malformed or noncanonical signed bytes.
	ErrInvalidObject = errors.New("relay verifier: invalid authenticated object")
	// ErrBinding identifies disagreement between signed bytes and relay
	// metadata or between an object and its supplied authority.
	ErrBinding = errors.New("relay verifier: authenticated binding mismatch")
)

// Verifier is the production relay verifier. It has no mutable state and
// receives only token-free authority views from the relay persistence layer.
type Verifier struct{}

var _ relay.Verifier = (*Verifier)(nil)

// New returns a stateless production relay verifier.
func New() *Verifier { return &Verifier{} }

// VerifyEnvironmentCertificate parses and verifies one administrator-signed
// environment certificate. The supplied authority is deliberately a
// token-free registration view.
func (Verifier) VerifyEnvironmentCertificate(ctx context.Context, authority relay.EnvironmentCertificateAuthority) error {
	if err := checkContext(ctx); err != nil {
		return err
	}
	_, err := verifyCertificate(ctx, authority.ChannelAuthority, authority.EnvironmentID, authority.CertificateID,
		authority.CertificateBytes, authority.Mode, authority.ExpiresAtMillis, authority.MembershipGeneration)
	return err
}

// VerifyEnvelope authenticates an opaque sealed fact and its exact envelope
// digest. It performs no decryption and does not apply certificate expiry;
// relay admission controls expiry while retained-history verification remains
// valid after expiry.
func (Verifier) VerifyEnvelope(ctx context.Context, authority relay.EnvironmentAuthority, envelope relay.Envelope) error {
	if err := checkContext(ctx); err != nil {
		return err
	}
	certificate, err := verifyEnvironmentAuthority(ctx, authority)
	if err != nil {
		return err
	}
	sealed, err := reconstructEnvelope(envelope)
	if err != nil {
		return err
	}
	if err := compareEnvelope(sealed, envelope); err != nil {
		return err
	}
	if err := crypto.VerifySealedFactSignature(sealed, certificate, protocol.PublicKey(authority.AdminPublicKey)); err != nil {
		return err
	}
	return nil
}

// VerifyAcknowledgement parses and verifies one stored, environment-signed
// progress acknowledgement and checks its relay-facing digest and frontier
// fields.
func (Verifier) VerifyAcknowledgement(ctx context.Context, authority relay.EnvironmentAuthority, acknowledgement relay.Acknowledgement) error {
	if err := checkContext(ctx); err != nil {
		return err
	}
	if err := acknowledgement.Validate(); err != nil {
		return invalidObject(err)
	}
	certificate, err := verifyEnvironmentAuthority(ctx, authority)
	if err != nil {
		return err
	}
	progress, err := protocol.ParseProgressAcknowledgement(acknowledgement.AcknowledgementBytes)
	if err != nil {
		return invalidObject(err)
	}
	if err := compareProgressAcknowledgement(progress, acknowledgement, authority); err != nil {
		return err
	}
	if !equalDigest(protocol.ProgressAcknowledgementDigest(progress), acknowledgement.AcknowledgementDigest) {
		return ErrBinding
	}
	if err := crypto.VerifyProgressAcknowledgement(progress, certificate, protocol.PublicKey(authority.AdminPublicKey)); err != nil {
		return err
	}
	return nil
}

// VerifyRetirement parses and verifies an administrator-signed terminal
// retirement against the complete token-free environment authority.
func (Verifier) VerifyRetirement(ctx context.Context, authority relay.EnvironmentAuthority, retirement relay.Retirement) error {
	if err := checkContext(ctx); err != nil {
		return err
	}
	if err := retirement.Validate(); err != nil {
		return invalidObject(err)
	}
	certificate, err := verifyEnvironmentAuthority(ctx, authority)
	if err != nil {
		return err
	}
	fence, err := protocol.ParseTerminalRetirement(retirement.RetirementBytes)
	if err != nil {
		return invalidObject(err)
	}
	if !equalDigest(protocol.TerminalRetirementID(fence), retirement.RetirementID) {
		return ErrBinding
	}
	if err := compareRetirement(fence, retirement, authority); err != nil {
		return err
	}
	if err := crypto.VerifyTerminalRetirement(fence, certificate, protocol.PublicKey(authority.AdminPublicKey)); err != nil {
		return err
	}
	return nil
}

// VerifyPruneCertificate authenticates a complete administrator-signed prune
// certificate, every opaque closure/target field, the exact active witness
// set, and each stored progress acknowledgement bound into each environment
// prune vote.
func (Verifier) VerifyPruneCertificate(ctx context.Context, authority relay.PruneAuthority, certificate relay.PruneCertificate) error {
	if err := checkContext(ctx); err != nil {
		return err
	}
	if err := certificate.Validate(); err != nil {
		return invalidObject(err)
	}
	if err := validateChannelAuthority(authority.Channel); err != nil {
		return err
	}
	parsed, err := protocol.ParsePruneCertificate(certificate.CertificateBytes)
	if err != nil {
		return invalidObject(err)
	}
	if err := comparePruneCertificate(parsed, certificate, authority.Channel); err != nil {
		return err
	}

	if len(authority.Environments) == 0 || len(authority.Environments) > relay.MaxPruneAuthorityEnvironments ||
		len(authority.Environments) != len(authority.Acknowledgements) ||
		len(authority.Environments) != len(parsed.Acknowledgements) {
		return ErrBinding
	}
	environmentCertificates := make([]protocol.EnvironmentCertificate, 0, len(authority.Environments))
	var previousEnvironment relay.EnvironmentID
	for index, environment := range authority.Environments {
		if index > 0 && environment.EnvironmentID <= previousEnvironment {
			return ErrBinding
		}
		previousEnvironment = environment.EnvironmentID
		if !equalChannel(environment.ChannelAuthority, authority.Channel) ||
			environment.EnvironmentID != relay.EnvironmentID(parsed.Acknowledgements[index].EnvironmentID) ||
			environment.CertificateID != relay.Digest(parsed.Acknowledgements[index].CertificateID) {
			return ErrBinding
		}
		environmentCertificate, err := verifyEnvironmentAuthority(ctx, environment)
		if err != nil {
			return err
		}
		environmentCertificates = append(environmentCertificates, environmentCertificate)
		progress, err := verifyStoredProgress(ctx, authority.Acknowledgements[index], environment, environmentCertificate)
		if err != nil {
			return err
		}
		vote := parsed.Acknowledgements[index]
		if err := compareProgressFrontier(vote, progress); err != nil {
			return err
		}
		if err := crypto.VerifyPruneAcknowledgement(vote, environmentCertificate, protocol.PublicKey(authority.Channel.AdminPublicKey)); err != nil {
			return err
		}
	}
	if err := crypto.VerifyPruneCertificate(parsed, environmentCertificates, protocol.PublicKey(authority.Channel.AdminPublicKey)); err != nil {
		return err
	}
	return nil
}

func checkContext(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("%w: nil context", relay.ErrInvalidArgument)
	}
	return ctx.Err()
}

func invalidObject(err error) error {
	if err == nil {
		return ErrInvalidObject
	}
	return fmt.Errorf("%w: %w", ErrInvalidObject, err)
}

func validateChannelAuthority(authority relay.ChannelAuthority) error {
	if zero32(authority.ChannelID[:]) || zero32(authority.RelayGeneration[:]) || zero32(authority.AdminPublicKey[:]) {
		return fmt.Errorf("%w: channel authority", ErrInvalidObject)
	}
	return nil
}

func verifyEnvironmentAuthority(ctx context.Context, authority relay.EnvironmentAuthority) (protocol.EnvironmentCertificate, error) {
	return verifyCertificate(ctx, authority.ChannelAuthority, authority.EnvironmentID, authority.CertificateID,
		authority.CertificateBytes, authority.Mode, authority.ExpiresAtMillis, authority.MembershipGeneration)
}

func verifyCertificate(
	ctx context.Context,
	channel relay.ChannelAuthority,
	environmentID relay.EnvironmentID,
	certificateID relay.Digest,
	certificateBytes []byte,
	mode relay.EnvironmentMode,
	expiresAtMillis int64,
	membershipGeneration uint32,
) (protocol.EnvironmentCertificate, error) {
	if err := checkContext(ctx); err != nil {
		return protocol.EnvironmentCertificate{}, err
	}
	if err := validateChannelAuthority(channel); err != nil {
		return protocol.EnvironmentCertificate{}, err
	}
	if !validEnvironmentID(environmentID) || zero32(certificateID[:]) || len(certificateBytes) == 0 || len(certificateBytes) > protocol.MaxCertificateBytes ||
		membershipGeneration < 1 || expiresAtMillis < 0 {
		return protocol.EnvironmentCertificate{}, fmt.Errorf("%w: environment certificate authority", ErrInvalidObject)
	}
	certificate, err := protocol.ParseEnvironmentCertificate(certificateBytes)
	if err != nil {
		return protocol.EnvironmentCertificate{}, invalidObject(err)
	}
	canonical, err := certificate.MarshalBinary()
	if err != nil || !bytes.Equal(canonical, certificateBytes) {
		return protocol.EnvironmentCertificate{}, invalidObject(err)
	}
	if !equal32(certificate.ChannelID, protocol.ChannelID(channel.ChannelID)) ||
		certificate.EnvironmentID != protocolEnvironmentID(environmentID) ||
		!equalDigest(protocol.CertificateID(certificate), certificateID) ||
		certificate.Mode != protocolEnvironmentMode(mode) ||
		certificate.ExpiresAtMillis != expiresAtMillis ||
		certificate.MembershipGeneration != membershipGeneration {
		return protocol.EnvironmentCertificate{}, ErrBinding
	}
	if err := crypto.VerifyEnvironmentCertificate(certificate, protocol.PublicKey(channel.AdminPublicKey)); err != nil {
		return protocol.EnvironmentCertificate{}, err
	}
	return certificate, nil
}

func reconstructEnvelope(envelope relay.Envelope) (protocol.SealedFact, error) {
	if err := envelope.Validate(); err != nil {
		return protocol.SealedFact{}, invalidObject(err)
	}
	sealed := protocol.SealedFact{
		Header: protocol.FactHeader{
			ProtocolVersion:        envelope.ProtocolVersion,
			CipherSuite:            envelope.CipherSuite,
			ChannelID:              protocol.ChannelID(envelope.ChannelID),
			FactID:                 protocolFactID(envelope.FactID),
			EnvironmentID:          protocolEnvironmentID(envelope.EnvironmentID),
			EnvironmentSequence:    envelope.EnvironmentSequence,
			KeyGeneration:          envelope.KeyGeneration,
			PreviousEnvelopeDigest: protocol.Digest(envelope.PreviousEnvelopeDigest),
			CertificateID:          protocol.Digest(envelope.CertificateID),
			Nonce:                  protocol.Nonce(envelope.Nonce),
		},
		Ciphertext: append([]byte(nil), envelope.Ciphertext...),
		Signature:  protocol.Signature(envelope.Signature),
	}
	encoded, err := sealed.MarshalBinary()
	if err != nil {
		return protocol.SealedFact{}, invalidObject(err)
	}
	parsed, err := protocol.ParseSealedFact(encoded)
	if err != nil {
		return protocol.SealedFact{}, invalidObject(err)
	}
	return parsed, nil
}

func compareEnvelope(sealed protocol.SealedFact, envelope relay.Envelope) error {
	if sealed.Header.ProtocolVersion != envelope.ProtocolVersion || sealed.Header.CipherSuite != envelope.CipherSuite ||
		!equal32(sealed.Header.ChannelID, protocol.ChannelID(envelope.ChannelID)) || sealed.Header.FactID != protocolFactID(envelope.FactID) ||
		sealed.Header.EnvironmentID != protocolEnvironmentID(envelope.EnvironmentID) || sealed.Header.EnvironmentSequence != envelope.EnvironmentSequence ||
		sealed.Header.KeyGeneration != envelope.KeyGeneration ||
		!equalDigest(sealed.Header.PreviousEnvelopeDigest, envelope.PreviousEnvelopeDigest) ||
		!equalDigest(sealed.Header.CertificateID, envelope.CertificateID) ||
		!equalNonce(sealed.Header.Nonce, envelope.Nonce) || !bytes.Equal(sealed.Ciphertext, envelope.Ciphertext) ||
		!equalSignature(sealed.Signature, envelope.Signature) ||
		!equalDigest(protocol.EnvelopeDigest(sealed), envelope.EnvelopeDigest) {
		return ErrBinding
	}
	return nil
}

func compareProgressAcknowledgement(progress protocol.ProgressAcknowledgement, acknowledgement relay.Acknowledgement, authority relay.EnvironmentAuthority) error {
	if !equal32(progress.ChannelID, protocol.ChannelID(acknowledgement.ChannelID)) ||
		!equal32(progress.RelayGeneration, protocol.RelayGeneration(authority.RelayGeneration)) ||
		progress.EnvironmentID != protocolEnvironmentID(acknowledgement.EnvironmentID) ||
		!equalDigest(progress.CertificateID, acknowledgement.CertificateID) ||
		progress.MembershipGeneration != acknowledgement.MembershipGeneration ||
		progress.AppliedArrivalSequence != acknowledgement.AppliedArrivalSequence ||
		progress.ProducerSequence != acknowledgement.ProducerSequence ||
		!equalDigest(progress.ProducerEnvelopeDigest, acknowledgement.ProducerEnvelopeDigest) {
		return ErrBinding
	}
	return nil
}

func compareRetirement(fence protocol.TerminalRetirement, retirement relay.Retirement, authority relay.EnvironmentAuthority) error {
	if !equal32(fence.ChannelID, protocol.ChannelID(retirement.ChannelID)) ||
		!equal32(fence.RelayGeneration, protocol.RelayGeneration(retirement.RelayGeneration)) ||
		fence.EnvironmentID != protocolEnvironmentID(retirement.EnvironmentID) ||
		!equalDigest(fence.CertificateID, retirement.CertificateID) ||
		fence.MembershipGeneration != retirement.MembershipGeneration ||
		fence.FinalEnvironmentSequence != retirement.FinalEnvironmentSequence ||
		!equalDigest(fence.FinalEnvelopeDigest, retirement.FinalEnvelopeDigest) ||
		!equal32(fence.ChannelID, protocol.ChannelID(authority.ChannelID)) ||
		!equal32(fence.RelayGeneration, protocol.RelayGeneration(authority.RelayGeneration)) ||
		fence.EnvironmentID != protocolEnvironmentID(authority.EnvironmentID) ||
		!equalDigest(fence.CertificateID, authority.CertificateID) ||
		!equalDigest(protocol.TerminalRetirementID(fence), retirement.RetirementID) {
		return ErrBinding
	}
	return nil
}

func comparePruneCertificate(parsed protocol.PruneCertificate, certificate relay.PruneCertificate, channel relay.ChannelAuthority) error {
	if !equal32(parsed.ChannelID, protocol.ChannelID(certificate.ChannelID)) ||
		!equal32(parsed.ChannelID, protocol.ChannelID(channel.ChannelID)) ||
		!equal32(parsed.RelayGeneration, protocol.RelayGeneration(channel.RelayGeneration)) ||
		!equalDigest(parsed.PruneID, certificate.PruneID) ||
		parsed.MembershipGeneration != certificate.MembershipGeneration ||
		parsed.BarrierArrivalSequence != certificate.Barrier ||
		!equalPruneReference(parsed.Closure, certificate.Closure) ||
		!equalDigest(protocol.PruneCertificateID(parsed), certificate.CertificateID) ||
		len(parsed.Manifest.Targets) != len(certificate.Targets) {
		return ErrBinding
	}
	for index, target := range parsed.Manifest.Targets {
		if !equalPruneReference(target, certificate.Targets[index]) {
			return ErrBinding
		}
	}
	return nil
}

func equalPruneReference(reference protocol.PruneReference, target relay.PruneTarget) bool {
	return reference.FactID == protocolFactID(target.FactID) && reference.EnvironmentID == protocolEnvironmentID(target.EnvironmentID) &&
		reference.EnvironmentSequence == target.EnvironmentSequence && reference.ArrivalSequence == target.ArrivalSequence &&
		equalDigest(reference.EnvelopeDigest, target.EnvelopeDigest) && equalDigest(reference.CertificateID, target.CertificateID) &&
		equalDigest(reference.PreviousEnvelopeDigest, target.PreviousEnvelopeDigest) && reference.KeyGeneration == target.KeyGeneration &&
		equalNonce(reference.Nonce, target.Nonce)
}

func verifyStoredProgress(ctx context.Context, acknowledgement relay.Acknowledgement, authority relay.EnvironmentAuthority, certificate protocol.EnvironmentCertificate) (protocol.ProgressAcknowledgement, error) {
	if err := checkContext(ctx); err != nil {
		return protocol.ProgressAcknowledgement{}, err
	}
	progress, err := protocol.ParseProgressAcknowledgement(acknowledgement.AcknowledgementBytes)
	if err != nil {
		return protocol.ProgressAcknowledgement{}, invalidObject(err)
	}
	if err := acknowledgement.Validate(); err != nil {
		return protocol.ProgressAcknowledgement{}, invalidObject(err)
	}
	if err := compareProgressAcknowledgement(progress, acknowledgement, authority); err != nil {
		return protocol.ProgressAcknowledgement{}, err
	}
	if !equalDigest(protocol.ProgressAcknowledgementDigest(progress), acknowledgement.AcknowledgementDigest) {
		return protocol.ProgressAcknowledgement{}, ErrBinding
	}
	if err := crypto.VerifyProgressAcknowledgement(progress, certificate, protocol.PublicKey(authority.AdminPublicKey)); err != nil {
		return protocol.ProgressAcknowledgement{}, err
	}
	return progress, nil
}

func compareProgressFrontier(vote protocol.PruneAcknowledgement, progress protocol.ProgressAcknowledgement) error {
	if !equalProtocolDigest(vote.ProgressAcknowledgementDigest, protocol.ProgressAcknowledgementDigest(progress)) ||
		vote.ChannelID != progress.ChannelID || vote.RelayGeneration != progress.RelayGeneration ||
		vote.EnvironmentID != progress.EnvironmentID || vote.CertificateID != progress.CertificateID ||
		vote.MembershipGeneration != progress.MembershipGeneration ||
		vote.AppliedArrivalSequence != progress.AppliedArrivalSequence || vote.ProducerSequence != progress.ProducerSequence ||
		vote.ProducerEnvelopeDigest != progress.ProducerEnvelopeDigest {
		return ErrBinding
	}
	return nil
}

func equalChannel(left, right relay.ChannelAuthority) bool {
	return equal32(left.ChannelID, right.ChannelID) && equal32(left.RelayGeneration, right.RelayGeneration) &&
		equal32(left.AdminPublicKey, right.AdminPublicKey)
}

func validEnvironmentID(value relay.EnvironmentID) bool {
	return continuity.EnvironmentID(value).Validate() == nil
}

func protocolFactID(value relay.FactID) continuity.FactID { return continuity.FactID(value) }

func protocolEnvironmentID(value relay.EnvironmentID) continuity.EnvironmentID {
	return continuity.EnvironmentID(value)
}

func protocolEnvironmentMode(value relay.EnvironmentMode) protocol.EnvironmentMode {
	switch value {
	case relay.TrustedEnvironment:
		return protocol.EnvironmentTrusted
	case relay.EphemeralEnvironment:
		return protocol.EnvironmentEphemeral
	default:
		return 0
	}
}

func equal32[T ~[32]byte](left, right T) bool {
	return subtle.ConstantTimeCompare(left[:], right[:]) == 1
}

func equalDigest(left protocol.Digest, right relay.Digest) bool {
	return subtle.ConstantTimeCompare(left[:], right[:]) == 1
}

func equalProtocolDigest(left, right protocol.Digest) bool {
	return subtle.ConstantTimeCompare(left[:], right[:]) == 1
}

func equalNonce(left protocol.Nonce, right relay.Nonce) bool {
	return subtle.ConstantTimeCompare(left[:], right[:]) == 1
}

func equalSignature(left protocol.Signature, right relay.Signature) bool {
	return subtle.ConstantTimeCompare(left[:], right[:]) == 1
}

func zero32(value []byte) bool {
	var combined byte
	for _, item := range value {
		combined |= item
	}
	return combined == 0
}
