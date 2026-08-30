package coordinator

import (
	"github.com/levifig/loaf/vnext/continuity"
	"github.com/levifig/loaf/vnext/sync/credential"
	"github.com/levifig/loaf/vnext/sync/crypto"
	"github.com/levifig/loaf/vnext/sync/protocol"
	"github.com/levifig/loaf/vnext/sync/relay"
)

// preparedRecoveryRegistration is the deterministic, authorization-free relay
// registration derived from an externally protected prepared credential. It
// contains a token hash, but never retains either recovery-owner or environment
// bearer authority.
type preparedRecoveryRegistration struct {
	targetMembershipGeneration uint32
	certificateID              relay.Digest
	environment                relay.Environment
}

// String prevents routine formatting from expanding token or certificate
// material carried by the registration.
func (preparedRecoveryRegistration) String() string {
	return "[REDACTED prepared recovery registration]"
}

// GoString prevents %#v from expanding token or certificate material.
func (preparedRecoveryRegistration) GoString() string {
	return "coordinator.preparedRecoveryRegistration([REDACTED])"
}

// bindPreparedRecoveryRegistration validates that one previously prepared
// trusted credential is the exact next recovery authority for this coordinator
// and derives its authorization-free relay registration. It performs no relay
// workflow call, persistence write, registration, promotion, or randomness.
func (coordinator *Coordinator) bindPreparedRecoveryRegistration(
	expectedProjectID continuity.ProjectID,
	recovery credential.ProjectRecoveryCredential,
	prepared credential.TrustedProjectCredential,
) (preparedRecoveryRegistration, error) {
	if coordinator == nil || coordinator.store == nil || nilRemote(coordinator.remote) {
		return preparedRecoveryRegistration{}, newProblem(CodeInvalid, PhaseConstruction, ActionConfigure)
	}
	if expectedProjectID.Validate() != nil || recovery.Validate() != nil || prepared.Validate() != nil {
		return preparedRecoveryRegistration{}, newProblem(CodeInvalid, PhaseRecoveryValidation, ActionCorrectInput)
	}

	writerEnvironmentID := coordinator.store.WriterEnvironmentID()
	if writerEnvironmentID.Validate() != nil {
		return preparedRecoveryRegistration{}, newProblem(CodeInvalid, PhaseRecoveryValidation, ActionRepairLocalStore)
	}
	if expectedProjectID != recovery.ProjectID || prepared.ProjectID != recovery.ProjectID ||
		prepared.RelayURL != recovery.RelayURL || prepared.RelayGeneration != recovery.RelayGeneration ||
		prepared.ChannelID != recovery.ChannelID || prepared.AdminPublicKey != crypto.AdminPublicKey(recovery.AdminSeed) ||
		prepared.ProjectRoot != recovery.ProjectRoot || prepared.WriteGeneration != recovery.WriteGeneration ||
		prepared.Certificate.EnvironmentID != writerEnvironmentID || coordinator.remote.Endpoint() != recovery.RelayURL {
		return preparedRecoveryRegistration{}, newProblem(CodeInvalid, PhaseRecoveryValidation, ActionCorrectInput)
	}
	if len(recovery.LastSignedCheckpoint) != 0 || len(prepared.LastObservedCheckpoint) != 0 ||
		prepared.MinimumProtocolVersion != protocol.ProtocolVersionV1 ||
		prepared.Certificate.Mode != protocol.EnvironmentTrusted || prepared.Certificate.ExpiresAtMillis != 0 ||
		len(prepared.Certificate.AllowedKeyGenerations) != 1 ||
		prepared.Certificate.AllowedKeyGenerations[0] != recovery.WriteGeneration {
		return preparedRecoveryRegistration{}, newProblem(CodeInvalid, PhaseRecoveryValidation, ActionCorrectInput)
	}

	certificateBytes, err := prepared.Certificate.MarshalBinary()
	if err != nil {
		return preparedRecoveryRegistration{}, newProblem(CodeInvalid, PhaseRecoveryValidation, ActionCorrectInput)
	}
	tokenHash, err := relay.HashTokenSecret(relay.RelayTokenSecret(prepared.EnvironmentRelayAuthorization.Secret()))
	if err != nil {
		return preparedRecoveryRegistration{}, newProblem(CodeInvalid, PhaseRecoveryValidation, ActionCorrectInput)
	}
	environment := relay.Environment{
		ChannelID:                 relay.ChannelID(prepared.ChannelID),
		EnvironmentID:             relay.EnvironmentID(prepared.Certificate.EnvironmentID),
		Token:                     relay.TokenRegistration{TokenID: relay.RelayTokenID(prepared.EnvironmentRelayAuthorization.ID()), TokenHash: tokenHash},
		CertificateID:             relay.Digest(protocol.CertificateID(prepared.Certificate)),
		CertificateBytes:          append([]byte(nil), certificateBytes...),
		Mode:                      relay.TrustedEnvironment,
		ExpiresAtMillis:           0,
		RelayTokenExpiresAtMillis: 0,
		MembershipGeneration:      prepared.Certificate.MembershipGeneration,
	}
	if err := environment.Validate(); err != nil {
		return preparedRecoveryRegistration{}, newProblem(CodeInvalid, PhaseRecoveryValidation, ActionCorrectInput)
	}
	return preparedRecoveryRegistration{
		targetMembershipGeneration: environment.MembershipGeneration,
		certificateID:              environment.CertificateID,
		environment:                environment,
	}, nil
}
