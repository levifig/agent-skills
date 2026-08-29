package relay

import (
	"fmt"
	"strings"
)

func (channel Channel) Validate() error {
	if zeroBytes(channel.ChannelID[:]) || zeroBytes(channel.RelayGeneration[:]) || zeroBytes(channel.AdminPublicKey[:]) {
		return fmt.Errorf("%w: channel identity", ErrInvalidArgument)
	}
	return channel.OwnerToken.validate()
}

func (authorization OwnerAuthorization) Validate() error {
	if zeroBytes(authorization.ChannelID[:]) || zeroBytes(authorization.RelayGeneration[:]) || zeroBytes(authorization.TokenID[:]) || zeroBytes(authorization.TokenSecret[:]) {
		return fmt.Errorf("%w: owner authorization", ErrInvalidArgument)
	}
	return nil
}

func (authorization EnvironmentAuthorization) Validate() error {
	if zeroBytes(authorization.ChannelID[:]) || zeroBytes(authorization.RelayGeneration[:]) ||
		!validOpaqueID(string(authorization.EnvironmentID)) || zeroBytes(authorization.CertificateID[:]) ||
		zeroBytes(authorization.TokenID[:]) || zeroBytes(authorization.TokenSecret[:]) {
		return fmt.Errorf("%w: environment authorization", ErrInvalidArgument)
	}
	return nil
}

func (environment Environment) Validate() error {
	if zeroBytes(environment.ChannelID[:]) || !validOpaqueID(string(environment.EnvironmentID)) ||
		zeroBytes(environment.CertificateID[:]) || environment.MembershipGeneration == 0 {
		return fmt.Errorf("%w: environment identity", ErrInvalidArgument)
	}
	if err := environment.Token.validate(); err != nil {
		return err
	}
	if len(environment.CertificateBytes) == 0 || len(environment.CertificateBytes) > MaxCertificateBytes {
		return fmt.Errorf("%w: certificate length", ErrInvalidArgument)
	}
	if environment.ExpiresAtMillis < 0 || environment.RelayTokenExpiresAtMillis < 0 {
		return fmt.Errorf("%w: environment expiry", ErrInvalidArgument)
	}
	switch environment.Mode {
	case TrustedEnvironment:
		if environment.ExpiresAtMillis != 0 || environment.RelayTokenExpiresAtMillis != 0 {
			return fmt.Errorf("%w: trusted environment expiry", ErrInvalidArgument)
		}
	case EphemeralEnvironment:
		if environment.ExpiresAtMillis == 0 || environment.RelayTokenExpiresAtMillis == 0 ||
			environment.RelayTokenExpiresAtMillis > environment.ExpiresAtMillis {
			return fmt.Errorf("%w: ephemeral environment expiry", ErrInvalidArgument)
		}
	default:
		return fmt.Errorf("%w: environment mode", ErrInvalidArgument)
	}
	return nil
}

func (envelope Envelope) Validate() error {
	if envelope.ProtocolVersion != ProtocolVersionV1 || envelope.CipherSuite != CipherSuiteV1 || zeroBytes(envelope.ChannelID[:]) ||
		!validOpaqueID(string(envelope.FactID)) || !validOpaqueID(string(envelope.EnvironmentID)) ||
		envelope.EnvironmentSequence <= 0 || envelope.KeyGeneration == 0 || zeroBytes(envelope.CertificateID[:]) ||
		zeroBytes(envelope.Signature[:]) || zeroBytes(envelope.EnvelopeDigest[:]) {
		return fmt.Errorf("%w: envelope header", ErrInvalidArgument)
	}
	if envelope.EnvironmentSequence == 1 {
		if !zeroBytes(envelope.PreviousEnvelopeDigest[:]) {
			return fmt.Errorf("%w: first envelope previous digest", ErrInvalidArgument)
		}
	} else if zeroBytes(envelope.PreviousEnvelopeDigest[:]) {
		return fmt.Errorf("%w: envelope previous digest", ErrInvalidArgument)
	}
	if len(envelope.Ciphertext) < MinimumCiphertextBytes || len(envelope.Ciphertext) > MaxCiphertextBytes {
		return fmt.Errorf("%w: ciphertext length", ErrInvalidArgument)
	}
	if envelopeStorageSize(envelope) > MaxEnvelopeBytes {
		return fmt.Errorf("%w: envelope length", ErrInvalidArgument)
	}
	return nil
}

func (request PageRequest) Validate() error {
	if err := request.Authorization.Validate(); err != nil {
		return err
	}
	if request.After < 0 || request.Limit < 1 || request.Limit > MaxPageSize {
		return fmt.Errorf("%w: page bounds", ErrInvalidArgument)
	}
	return nil
}

func (authorization InventoryAuthorization) Validate() error {
	switch {
	case authorization.Owner != nil && authorization.Environment == nil:
		return authorization.Owner.Validate()
	case authorization.Owner == nil && authorization.Environment != nil:
		return authorization.Environment.Validate()
	default:
		return fmt.Errorf("%w: inventory authorization", ErrInvalidArgument)
	}
}

func (request EnvironmentInventoryRequest) Validate() error {
	if err := request.Authorization.Validate(); err != nil {
		return err
	}
	if request.AfterEnvironmentID != "" && (!validOpaqueID(string(request.AfterEnvironmentID)) || request.Snapshot == nil) {
		return fmt.Errorf("%w: environment inventory cursor or snapshot", ErrInvalidArgument)
	}
	if request.Snapshot != nil && (request.Snapshot.ArrivalHead < 0 || request.Snapshot.MembershipGeneration == 0) {
		return fmt.Errorf("%w: environment inventory snapshot", ErrInvalidArgument)
	}
	if request.Limit < 1 || request.Limit > MaxEnvironmentInventoryPage {
		return fmt.Errorf("%w: environment inventory bounds", ErrInvalidArgument)
	}
	return nil
}

func (request PruneInventoryRequest) Validate() error {
	if err := request.Authorization.Validate(); err != nil {
		return err
	}
	if request.After < 0 || (request.After != 0 && request.Snapshot == nil) ||
		(request.Snapshot != nil && (request.Snapshot.MembershipGeneration == 0 ||
			request.Snapshot.ArrivalHead < 0 || request.Snapshot.PruneHead < request.After)) ||
		request.Limit < 1 || request.Limit > MaxPruneInventoryPage {
		return fmt.Errorf("%w: prune inventory bounds", ErrInvalidArgument)
	}
	return nil
}

func (acknowledgement Acknowledgement) Validate() error {
	if zeroBytes(acknowledgement.ChannelID[:]) || !validOpaqueID(string(acknowledgement.EnvironmentID)) ||
		acknowledgement.MembershipGeneration == 0 || acknowledgement.AppliedArrivalSequence < 0 ||
		acknowledgement.ProducerSequence < 0 || zeroBytes(acknowledgement.CertificateID[:]) ||
		zeroBytes(acknowledgement.AcknowledgementDigest[:]) {
		return fmt.Errorf("%w: acknowledgement", ErrInvalidArgument)
	}
	if len(acknowledgement.AcknowledgementBytes) == 0 || len(acknowledgement.AcknowledgementBytes) > MaxControlObjectBytes {
		return fmt.Errorf("%w: acknowledgement length", ErrInvalidArgument)
	}
	return nil
}

func (request TombstoneRequest) Validate() error {
	if err := request.Authorization.Validate(); err != nil {
		return err
	}
	return request.Certificate.Validate()
}

func (certificate PruneCertificate) Validate() error {
	if zeroBytes(certificate.ChannelID[:]) || zeroBytes(certificate.PruneID[:]) || certificate.MembershipGeneration == 0 || certificate.Barrier < 1 ||
		zeroBytes(certificate.CertificateID[:]) || len(certificate.CertificateBytes) == 0 ||
		len(certificate.CertificateBytes) > MaxControlObjectBytes || len(certificate.Targets) == 0 ||
		len(certificate.Targets) > MaxPruneTargets {
		return fmt.Errorf("%w: prune certificate", ErrInvalidArgument)
	}
	seen := make(map[FactID]struct{}, len(certificate.Targets))
	for _, target := range certificate.Targets {
		if !validOpaqueID(string(target.FactID)) || !validOpaqueID(string(target.EnvironmentID)) ||
			target.EnvironmentSequence < 1 || target.ArrivalSequence < 1 ||
			zeroBytes(target.EnvelopeDigest[:]) || zeroBytes(target.CertificateID[:]) || target.ArrivalSequence > certificate.Barrier {
			return fmt.Errorf("%w: prune target", ErrInvalidArgument)
		}
		if _, exists := seen[target.FactID]; exists {
			return fmt.Errorf("%w: duplicate prune target", ErrInvalidArgument)
		}
		seen[target.FactID] = struct{}{}
	}
	return nil
}

func (retirement Retirement) Validate() error {
	if zeroBytes(retirement.ChannelID[:]) || zeroBytes(retirement.RelayGeneration[:]) ||
		!validOpaqueID(string(retirement.EnvironmentID)) || zeroBytes(retirement.CertificateID[:]) ||
		retirement.MembershipGeneration == 0 || retirement.FinalEnvironmentSequence < 0 ||
		zeroBytes(retirement.RetirementID[:]) || len(retirement.RetirementBytes) == 0 || len(retirement.RetirementBytes) > MaxControlObjectBytes {
		return fmt.Errorf("%w: retirement", ErrInvalidArgument)
	}
	if (retirement.FinalEnvironmentSequence == 0) != zeroBytes(retirement.FinalEnvelopeDigest[:]) {
		return fmt.Errorf("%w: retirement final envelope", ErrInvalidArgument)
	}
	return nil
}

func (registration TokenRegistration) validate() error {
	if zeroBytes(registration.TokenID[:]) || zeroBytes(registration.TokenHash[:]) {
		return fmt.Errorf("%w: token registration", ErrInvalidArgument)
	}
	return nil
}

func validOpaqueID(value string) bool {
	if len(value) < 1 || len(value) > 128 || strings.TrimSpace(value) != value {
		return false
	}
	for _, character := range value {
		switch {
		case character >= 'A' && character <= 'Z':
		case character >= 'a' && character <= 'z':
		case character >= '0' && character <= '9':
		case strings.ContainsRune("_.:-", character):
		default:
			return false
		}
	}
	return true
}

func zeroBytes(value []byte) bool {
	for _, item := range value {
		if item != 0 {
			return false
		}
	}
	return true
}

func envelopeStorageSize(envelope Envelope) int {
	const fixedBytes = 2 + 2 + 32 + 8 + 4 + 32 + 32 + 24 + 64 + 32
	return fixedBytes + len(envelope.FactID) + len(envelope.EnvironmentID) + len(envelope.Ciphertext)
}
