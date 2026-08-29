package protocol

import (
	"errors"
	"strings"

	"github.com/levifig/loaf/vnext/continuity"
	"github.com/levifig/loaf/vnext/internal/continuitywire"
)

const (
	// ControlVersionV1 is the first signed sync control-object format.
	ControlVersionV1 uint16 = 1

	// MaxProgressAcknowledgementBytes bounds one signed progress assertion.
	MaxProgressAcknowledgementBytes = 4_096
	// MaxPruneAcknowledgementBytes bounds one environment-signed prune vote.
	MaxPruneAcknowledgementBytes = 4_096
	// MaxTerminalRetirementBytes bounds one administrator-signed terminal fence.
	MaxTerminalRetirementBytes = 4_096
	// MaxPruneReferenceBytes bounds one exact opaque arrival reference.
	MaxPruneReferenceBytes = continuitywire.MaxPruneReferenceBytes
	// MaxPruneTargets bounds verification work for one physical-prune manifest.
	MaxPruneTargets = 1_024
	// MaxPruneAcknowledgements bounds the active environment witness set.
	MaxPruneAcknowledgements = MaxPageFrames
)

var (
	// ErrInvalidAcknowledgement identifies a malformed progress assertion.
	ErrInvalidAcknowledgement = errors.New("sync protocol: invalid progress acknowledgement")
	// ErrInvalidPruneAcknowledgement identifies a malformed prune vote.
	ErrInvalidPruneAcknowledgement = errors.New("sync protocol: invalid prune acknowledgement")
	// ErrInvalidRetirement identifies a malformed terminal environment fence.
	ErrInvalidRetirement = errors.New("sync protocol: invalid terminal retirement")
	// ErrInvalidPruneReference identifies malformed opaque arrival identity.
	ErrInvalidPruneReference = errors.New("sync protocol: invalid prune reference")
	// ErrInvalidPruneManifest identifies a malformed or noncanonical target set.
	ErrInvalidPruneManifest = errors.New("sync protocol: invalid prune manifest")
	// ErrInvalidPruneCertificate identifies a malformed signed prune authority.
	ErrInvalidPruneCertificate = errors.New("sync protocol: invalid prune certificate")
)

// ProgressAcknowledgement is an environment-signed assertion of applied relay
// progress and its own exact producer frontier.
type ProgressAcknowledgement struct {
	Version                uint16
	ProtocolVersion        uint16
	CipherSuite            uint16
	ChannelID              ChannelID
	RelayGeneration        RelayGeneration
	EnvironmentID          continuity.EnvironmentID
	CertificateID          Digest
	MembershipGeneration   uint32
	AppliedArrivalSequence int64
	ProducerSequence       int64
	ProducerEnvelopeDigest Digest
	EnvironmentSignature   Signature
}

// Validate rejects unsupported, ambiguous, or noncanonical progress data.
// Signature authenticity is checked by the crypto layer.
func (acknowledgement ProgressAcknowledgement) Validate() error {
	if acknowledgement.Version != ControlVersionV1 {
		return ErrInvalidAcknowledgement
	}
	if acknowledgement.ProtocolVersion != ProtocolVersionV1 {
		return ErrUnsupportedProtocolVersion
	}
	if acknowledgement.CipherSuite != CipherSuiteXChaCha20Poly1305 {
		return ErrUnsupportedCipherSuite
	}
	if isZero(acknowledgement.ChannelID[:]) || isZero(acknowledgement.RelayGeneration[:]) ||
		acknowledgement.EnvironmentID.Validate() != nil || isZero(acknowledgement.CertificateID[:]) ||
		acknowledgement.MembershipGeneration < 1 || acknowledgement.AppliedArrivalSequence < 0 ||
		acknowledgement.ProducerSequence < 0 ||
		(acknowledgement.ProducerSequence == 0) != isZero(acknowledgement.ProducerEnvelopeDigest[:]) {
		return ErrInvalidAcknowledgement
	}
	return nil
}

// PruneAcknowledgement is an environment-signed vote for one immutable prune
// proposal. It references a previously verified progress acknowledgement.
type PruneAcknowledgement struct {
	Version                       uint16
	ProtocolVersion               uint16
	CipherSuite                   uint16
	ChannelID                     ChannelID
	RelayGeneration               RelayGeneration
	EnvironmentID                 continuity.EnvironmentID
	CertificateID                 Digest
	MembershipGeneration          uint32
	ProgressAcknowledgementDigest Digest
	AppliedArrivalSequence        int64
	ProducerSequence              int64
	ProducerEnvelopeDigest        Digest
	PruneID                       Digest
	BarrierArrivalSequence        int64
	ClosureReferenceDigest        Digest
	ManifestCount                 uint32
	ManifestDigest                Digest
	CapsuleDigest                 Digest
	EnvironmentSignature          Signature
}

// Validate rejects unsupported, ambiguous, or noncanonical prune-vote data.
// Signature authenticity and the referenced progress object are checked later.
func (acknowledgement PruneAcknowledgement) Validate() error {
	if acknowledgement.Version != ControlVersionV1 {
		return ErrInvalidPruneAcknowledgement
	}
	if acknowledgement.ProtocolVersion != ProtocolVersionV1 {
		return ErrUnsupportedProtocolVersion
	}
	if acknowledgement.CipherSuite != CipherSuiteXChaCha20Poly1305 {
		return ErrUnsupportedCipherSuite
	}
	if isZero(acknowledgement.ChannelID[:]) || isZero(acknowledgement.RelayGeneration[:]) ||
		acknowledgement.EnvironmentID.Validate() != nil || isZero(acknowledgement.CertificateID[:]) ||
		acknowledgement.MembershipGeneration < 1 || isZero(acknowledgement.ProgressAcknowledgementDigest[:]) ||
		acknowledgement.AppliedArrivalSequence < 0 || acknowledgement.ProducerSequence < 0 ||
		(acknowledgement.ProducerSequence == 0) != isZero(acknowledgement.ProducerEnvelopeDigest[:]) ||
		isZero(acknowledgement.PruneID[:]) || acknowledgement.BarrierArrivalSequence < 1 ||
		acknowledgement.AppliedArrivalSequence < acknowledgement.BarrierArrivalSequence ||
		isZero(acknowledgement.ClosureReferenceDigest[:]) || acknowledgement.ManifestCount < 1 ||
		acknowledgement.ManifestCount > MaxPruneTargets || isZero(acknowledgement.ManifestDigest[:]) ||
		isZero(acknowledgement.CapsuleDigest[:]) {
		return ErrInvalidPruneAcknowledgement
	}
	return nil
}

// TerminalRetirement is an administrator-signed terminal source-chain fence.
type TerminalRetirement struct {
	Version                  uint16
	ProtocolVersion          uint16
	CipherSuite              uint16
	ChannelID                ChannelID
	RelayGeneration          RelayGeneration
	EnvironmentID            continuity.EnvironmentID
	CertificateID            Digest
	MembershipGeneration     uint32
	FinalEnvironmentSequence int64
	FinalEnvelopeDigest      Digest
	AdminSignature           Signature
}

// Validate rejects unsupported or ambiguous retirement data. An empty producer
// uses sequence zero and the all-zero digest; all other fences require a digest.
func (retirement TerminalRetirement) Validate() error {
	if retirement.Version != ControlVersionV1 {
		return ErrInvalidRetirement
	}
	if retirement.ProtocolVersion != ProtocolVersionV1 {
		return ErrUnsupportedProtocolVersion
	}
	if retirement.CipherSuite != CipherSuiteXChaCha20Poly1305 {
		return ErrUnsupportedCipherSuite
	}
	if isZero(retirement.ChannelID[:]) || isZero(retirement.RelayGeneration[:]) ||
		retirement.EnvironmentID.Validate() != nil || isZero(retirement.CertificateID[:]) ||
		retirement.MembershipGeneration < 1 || retirement.FinalEnvironmentSequence < 0 ||
		(retirement.FinalEnvironmentSequence == 0) != isZero(retirement.FinalEnvelopeDigest[:]) {
		return ErrInvalidRetirement
	}
	return nil
}

// PruneReference is the exact opaque identity retained for one relay arrival.
// It intentionally contains no decrypted continuity subject or fact kind.
type PruneReference struct {
	FactID                 continuity.FactID
	EnvironmentID          continuity.EnvironmentID
	EnvironmentSequence    int64
	ArrivalSequence        int64
	EnvelopeDigest         Digest
	CertificateID          Digest
	PreviousEnvelopeDigest Digest
	KeyGeneration          uint32
	Nonce                  Nonce
}

// Validate rejects malformed opaque arrival identity.
func (reference PruneReference) Validate() error {
	if err := pruneReferenceWireV1(reference).Validate(); err != nil {
		return ErrInvalidPruneReference
	}
	return nil
}

func pruneReferenceWireV1(reference PruneReference) continuitywire.PruneReference {
	return continuitywire.PruneReference{
		FactID:                 reference.FactID,
		EnvironmentID:          reference.EnvironmentID,
		EnvironmentSequence:    reference.EnvironmentSequence,
		ArrivalSequence:        reference.ArrivalSequence,
		EnvelopeDigest:         reference.EnvelopeDigest,
		CertificateID:          reference.CertificateID,
		PreviousEnvelopeDigest: reference.PreviousEnvelopeDigest,
		KeyGeneration:          reference.KeyGeneration,
		Nonce:                  reference.Nonce,
	}
}

func pruneReferenceFromWireV1(reference continuitywire.PruneReference) PruneReference {
	return PruneReference{
		FactID:                 reference.FactID,
		EnvironmentID:          reference.EnvironmentID,
		EnvironmentSequence:    reference.EnvironmentSequence,
		ArrivalSequence:        reference.ArrivalSequence,
		EnvelopeDigest:         reference.EnvelopeDigest,
		CertificateID:          reference.CertificateID,
		PreviousEnvelopeDigest: reference.PreviousEnvelopeDigest,
		KeyGeneration:          reference.KeyGeneration,
		Nonce:                  reference.Nonce,
	}
}

// PruneManifest is a strict arrival-ordered set of exact prune targets.
type PruneManifest struct {
	Targets []PruneReference
}

// Validate rejects empty, oversized, unsorted, or duplicate target sets.
func (manifest PruneManifest) Validate() error {
	if len(manifest.Targets) < 1 || len(manifest.Targets) > MaxPruneTargets {
		return ErrInvalidPruneManifest
	}
	seenFacts := make(map[continuity.FactID]struct{}, len(manifest.Targets))
	type sourceIdentity struct {
		environment continuity.EnvironmentID
		sequence    int64
	}
	seenSources := make(map[sourceIdentity]struct{}, len(manifest.Targets))
	var previousArrival int64
	for index, target := range manifest.Targets {
		if err := target.Validate(); err != nil || (index > 0 && target.ArrivalSequence <= previousArrival) {
			return ErrInvalidPruneManifest
		}
		if _, exists := seenFacts[target.FactID]; exists {
			return ErrInvalidPruneManifest
		}
		source := sourceIdentity{environment: target.EnvironmentID, sequence: target.EnvironmentSequence}
		if _, exists := seenSources[source]; exists {
			return ErrInvalidPruneManifest
		}
		seenFacts[target.FactID] = struct{}{}
		seenSources[source] = struct{}{}
		previousArrival = target.ArrivalSequence
	}
	return nil
}

// PruneCertificate is an administrator-signed, self-contained physical-prune
// authority. Embedded acknowledgements prove the exact active witness set.
type PruneCertificate struct {
	Version                    uint16
	ProtocolVersion            uint16
	CipherSuite                uint16
	ChannelID                  ChannelID
	RelayGeneration            RelayGeneration
	PruneID                    Digest
	MembershipGeneration       uint32
	BarrierArrivalSequence     int64
	Closure                    PruneReference
	ClosureDigest              Digest
	ManifestCount              uint32
	ManifestDigest             Digest
	Manifest                   PruneManifest
	CapsuleDigest              Digest
	Capsule                    PruneBootstrap
	ActiveAcknowledgementCount uint32
	Acknowledgements           []PruneAcknowledgement
	AdminSignature             Signature
}

// Validate rejects unsupported, ambiguous, or noncanonical prune authority.
// Cryptographic authenticity is checked by the crypto and service layers.
func (certificate PruneCertificate) Validate() error {
	if certificate.Version != ControlVersionV1 {
		return ErrInvalidPruneCertificate
	}
	if certificate.ProtocolVersion != ProtocolVersionV1 {
		return ErrUnsupportedProtocolVersion
	}
	if certificate.CipherSuite != CipherSuiteXChaCha20Poly1305 {
		return ErrUnsupportedCipherSuite
	}
	if err := certificate.Capsule.Validate(); err != nil {
		if errors.Is(err, ErrTooLarge) {
			return ErrTooLarge
		}
		return ErrInvalidPruneCertificate
	}
	if isZero(certificate.ChannelID[:]) || isZero(certificate.RelayGeneration[:]) ||
		isZero(certificate.PruneID[:]) || certificate.MembershipGeneration < 1 ||
		certificate.BarrierArrivalSequence < 1 || certificate.Closure.Validate() != nil ||
		certificate.Closure.ArrivalSequence > certificate.BarrierArrivalSequence ||
		certificate.ClosureDigest != PruneReferenceDigest(certificate.Closure) ||
		certificate.Manifest.Validate() != nil || certificate.ManifestCount != uint32(len(certificate.Manifest.Targets)) ||
		certificate.ManifestDigest != PruneManifestDigest(certificate.Manifest) ||
		isZero(certificate.CapsuleDigest[:]) || certificate.CapsuleDigest != PruneBootstrapDigest(certificate.Capsule) ||
		certificate.Capsule.ProtocolVersion != certificate.ProtocolVersion ||
		certificate.Capsule.CipherSuite != certificate.CipherSuite || certificate.Capsule.ChannelID != certificate.ChannelID ||
		certificate.Capsule.RelayGeneration != certificate.RelayGeneration || certificate.Capsule.PruneID != certificate.PruneID ||
		certificate.Capsule.MembershipGeneration != certificate.MembershipGeneration ||
		certificate.Capsule.BarrierArrivalSequence != certificate.BarrierArrivalSequence ||
		certificate.Capsule.ClosureReferenceDigest != certificate.ClosureDigest ||
		certificate.Capsule.ManifestCount != certificate.ManifestCount || certificate.Capsule.ManifestDigest != certificate.ManifestDigest ||
		certificate.ActiveAcknowledgementCount != uint32(len(certificate.Acknowledgements)) ||
		len(certificate.Acknowledgements) < 1 || len(certificate.Acknowledgements) > MaxPruneAcknowledgements {
		return ErrInvalidPruneCertificate
	}

	for _, target := range certificate.Manifest.Targets {
		if target.ArrivalSequence > certificate.BarrierArrivalSequence ||
			target.ArrivalSequence == certificate.Closure.ArrivalSequence ||
			target.FactID == certificate.Closure.FactID ||
			(target.EnvironmentID == certificate.Closure.EnvironmentID && target.EnvironmentSequence == certificate.Closure.EnvironmentSequence) {
			return ErrInvalidPruneCertificate
		}
	}

	var previousEnvironment string
	for index, acknowledgement := range certificate.Acknowledgements {
		if acknowledgement.Validate() != nil ||
			acknowledgement.ProtocolVersion != certificate.ProtocolVersion ||
			acknowledgement.CipherSuite != certificate.CipherSuite ||
			acknowledgement.ChannelID != certificate.ChannelID ||
			acknowledgement.RelayGeneration != certificate.RelayGeneration ||
			acknowledgement.MembershipGeneration != certificate.MembershipGeneration ||
			acknowledgement.PruneID != certificate.PruneID ||
			acknowledgement.BarrierArrivalSequence != certificate.BarrierArrivalSequence ||
			acknowledgement.ClosureReferenceDigest != certificate.ClosureDigest ||
			acknowledgement.ManifestCount != certificate.ManifestCount ||
			acknowledgement.ManifestDigest != certificate.ManifestDigest ||
			acknowledgement.CapsuleDigest != certificate.CapsuleDigest ||
			(index > 0 && strings.Compare(previousEnvironment, string(acknowledgement.EnvironmentID)) >= 0) {
			return ErrInvalidPruneCertificate
		}
		previousEnvironment = string(acknowledgement.EnvironmentID)
	}
	return nil
}
