package sqlite

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"math"

	"github.com/levifig/loaf/vnext/continuity"
)

const (
	syncAuthorityCandidateCodecVersionV2 uint16 = 2

	syncAuthorityHeaderDomainV2      = "loaf.continuity.sync.authority.header.v2"
	syncAuthorityCandidateDomainV2   = "loaf.continuity.sync.authority-candidate.id.v2"
	syncAuthorityRetirementDomainV2  = "loaf.continuity.sync.authority.retirement.v2"
	syncAuthorityEnvironmentDomainV2 = "loaf.continuity.sync.authority.environment.v2"
	syncAuthorityRollingSeedDomainV2 = "loaf.continuity.sync.authority.rolling-seed.v2"
	syncAuthorityRollingStepDomainV2 = "loaf.continuity.sync.authority.rolling-step.v2"
	syncAuthorityPageDomainV2        = "loaf.continuity.sync.authority-candidate.page.v2"
	syncAuthorityFinalDomainV2       = "loaf.continuity.sync.authority.digest.v2"

	maximumSyncAuthorityHeaderBytesV2      = 1_024
	maximumSyncAuthorityCandidateIDBytesV2 = 512
	maximumSyncAuthorityRetirementBytesV2  = maximumEnvironmentRetirementBytes + 512
	maximumSyncAuthorityEnvironmentBytesV2 = maximumEnvironmentCertificateBytes + maximumEnvironmentRetirementBytes + 1_024
	maximumSyncAuthorityRollingBytesV2     = 512
	maximumSyncAuthorityPageBytesV2        = 1_024
	maximumSyncAuthorityFinalBytesV2       = 512
)

func invalidSyncAuthorityCandidateCodecV2() error {
	return errors.New("invalid continuity sync authority candidate codec")
}

func deriveSyncAuthorityCandidateIdentityV2(projectID continuity.ProjectID, snapshot SyncAuthoritySnapshot) ([32]byte, [32]byte, error) {
	headerDigest, err := syncAuthorityHeaderDigestV2(projectID, snapshot)
	if err != nil {
		return [32]byte{}, [32]byte{}, err
	}
	basePresent := []byte{0}
	var baseVersion, baseDigest []byte
	if snapshot.BaseAuthorityDigestVersion != 0 {
		basePresent[0] = 1
		baseVersion = terminalCandidateUint16BytesV1(snapshot.BaseAuthorityDigestVersion)
		baseDigest = snapshot.BaseAuthorityDigest[:]
	}
	transcript, err := encodeTerminalCandidateTranscriptV1(
		syncAuthorityCandidateDomainV2,
		maximumSyncAuthorityCandidateIDBytesV2,
		terminalCandidateUint16BytesV1(syncAuthorityCandidateCodecVersionV2),
		[]byte(projectID),
		headerDigest[:],
		basePresent,
		baseVersion,
		baseDigest,
	)
	if err != nil {
		return [32]byte{}, [32]byte{}, err
	}
	candidateID := sha256.Sum256(transcript)
	if candidateID == ([32]byte{}) {
		return [32]byte{}, [32]byte{}, invalidSyncAuthorityCandidateCodecV2()
	}
	return candidateID, headerDigest, nil
}

func syncAuthorityHeaderDigestV2(projectID continuity.ProjectID, snapshot SyncAuthoritySnapshot) ([32]byte, error) {
	if err := validateSyncAuthoritySnapshotV2(projectID, snapshot); err != nil {
		return [32]byte{}, err
	}
	transcript, err := encodeTerminalCandidateTranscriptV1(
		syncAuthorityHeaderDomainV2,
		maximumSyncAuthorityHeaderBytesV2,
		terminalCandidateUint16BytesV1(syncAuthorityCandidateCodecVersionV2),
		[]byte(projectID),
		snapshot.ChannelID[:],
		snapshot.RelayGeneration[:],
		snapshot.AdminPublicKey[:],
		terminalCandidateUint32BytesV1(snapshot.MembershipGeneration),
		terminalCandidateInt64BytesV1(snapshot.InventoryArrivalHead),
	)
	if err != nil {
		return [32]byte{}, err
	}
	digest := sha256.Sum256(transcript)
	if digest == ([32]byte{}) {
		return [32]byte{}, invalidSyncAuthorityCandidateCodecV2()
	}
	return digest, nil
}

func syncAuthorityCandidateRollingSeedV2(headerDigest [32]byte) ([32]byte, error) {
	if headerDigest == ([32]byte{}) {
		return [32]byte{}, invalidSyncAuthorityCandidateCodecV2()
	}
	transcript, err := encodeTerminalCandidateTranscriptV1(
		syncAuthorityRollingSeedDomainV2,
		maximumSyncAuthorityRollingBytesV2,
		terminalCandidateUint16BytesV1(syncAuthorityCandidateCodecVersionV2),
		headerDigest[:],
	)
	if err != nil {
		return [32]byte{}, err
	}
	digest := sha256.Sum256(transcript)
	if digest == ([32]byte{}) {
		return [32]byte{}, invalidSyncAuthorityCandidateCodecV2()
	}
	return digest, nil
}

func advanceSyncAuthorityCandidateRollingV2(headerDigest, previous [32]byte, ordinal int64, environment SyncEnvironmentCertificate) ([32]byte, [32]byte, error) {
	if headerDigest == ([32]byte{}) || previous == ([32]byte{}) || ordinal < 1 {
		return [32]byte{}, [32]byte{}, invalidSyncAuthorityCandidateCodecV2()
	}
	environmentDigest, err := syncAuthorityEnvironmentDigestV2(environment)
	if err != nil {
		return [32]byte{}, [32]byte{}, err
	}
	transcript, err := encodeTerminalCandidateTranscriptV1(
		syncAuthorityRollingStepDomainV2,
		maximumSyncAuthorityRollingBytesV2,
		terminalCandidateUint16BytesV1(syncAuthorityCandidateCodecVersionV2),
		headerDigest[:],
		terminalCandidateInt64BytesV1(ordinal),
		previous[:],
		environmentDigest[:],
	)
	if err != nil {
		return [32]byte{}, [32]byte{}, err
	}
	rolling := sha256.Sum256(transcript)
	if rolling == ([32]byte{}) {
		return [32]byte{}, [32]byte{}, invalidSyncAuthorityCandidateCodecV2()
	}
	return rolling, environmentDigest, nil
}

func syncAuthorityEnvironmentDigestV2(environment SyncEnvironmentCertificate) ([32]byte, error) {
	if err := validateSyncAuthorityCandidateEnvironmentV2(environment, -1); err != nil {
		return [32]byte{}, err
	}
	retirementPresent := []byte{0}
	var retirementTranscript []byte
	if environment.Retirement != nil {
		retirementPresent[0] = 1
		retirement := environment.Retirement
		var err error
		retirementTranscript, err = encodeTerminalCandidateTranscriptV1(
			syncAuthorityRetirementDomainV2,
			maximumSyncAuthorityRetirementBytesV2,
			terminalCandidateUint16BytesV1(syncAuthorityCandidateCodecVersionV2),
			retirement.RelayGeneration[:],
			terminalCandidateUint32BytesV1(retirement.MembershipGeneration),
			terminalCandidateInt64BytesV1(retirement.FinalEnvironmentSequence),
			retirement.FinalEnvelopeDigest[:],
			retirement.RetirementID[:],
			retirement.RetirementBytes,
		)
		if err != nil {
			return [32]byte{}, err
		}
	}
	transcript, err := encodeTerminalCandidateTranscriptV1(
		syncAuthorityEnvironmentDomainV2,
		maximumSyncAuthorityEnvironmentBytesV2,
		terminalCandidateUint16BytesV1(syncAuthorityCandidateCodecVersionV2),
		[]byte(environment.EnvironmentID),
		environment.CertificateID[:],
		environment.CertificateBytes,
		[]byte(environment.Mode),
		terminalCandidateInt64BytesV1(environment.ExpiresAtMillis),
		terminalCandidateUint32BytesV1(environment.JoinMembershipGeneration),
		retirementPresent,
		retirementTranscript,
	)
	if err != nil {
		return [32]byte{}, err
	}
	digest := sha256.Sum256(transcript)
	if digest == ([32]byte{}) {
		return [32]byte{}, invalidSyncAuthorityCandidateCodecV2()
	}
	return digest, nil
}

func syncAuthorityCandidatePageDigestV2(candidateID [32]byte, pageNumber int64, page SyncAuthorityPage, resultingEnvironmentCount int64, resultingRolling [32]byte, environmentDigests [][32]byte) ([32]byte, error) {
	if candidateID == ([32]byte{}) || pageNumber < 1 || resultingEnvironmentCount < 1 || resultingRolling == ([32]byte{}) ||
		len(environmentDigests) < 1 || len(environmentDigests) > maximumSyncAuthorityCandidatePageEnvironments {
		return [32]byte{}, invalidSyncAuthorityCandidateCodecV2()
	}
	digestFields := make([][]byte, len(environmentDigests))
	for index := range environmentDigests {
		if environmentDigests[index] == ([32]byte{}) {
			return [32]byte{}, invalidSyncAuthorityCandidateCodecV2()
		}
		digestFields[index] = environmentDigests[index][:]
	}
	digestList, err := encodeTerminalCandidateListV1(digestFields, maximumSyncAuthorityCandidatePageEnvironments, 256)
	if err != nil {
		return [32]byte{}, err
	}
	more := []byte{0}
	if page.More {
		more[0] = 1
	}
	transcript, err := encodeTerminalCandidateTranscriptV1(
		syncAuthorityPageDomainV2,
		maximumSyncAuthorityPageBytesV2,
		terminalCandidateUint16BytesV1(syncAuthorityCandidateCodecVersionV2),
		candidateID[:],
		terminalCandidateInt64BytesV1(pageNumber),
		[]byte(page.AfterEnvironmentID),
		[]byte(page.ThroughEnvironmentID),
		more,
		terminalCandidateInt64BytesV1(resultingEnvironmentCount),
		resultingRolling[:],
		digestList,
	)
	if err != nil {
		return [32]byte{}, err
	}
	digest := sha256.Sum256(transcript)
	if digest == ([32]byte{}) {
		return [32]byte{}, invalidSyncAuthorityCandidateCodecV2()
	}
	return digest, nil
}

func finalizeSyncAuthorityDigestV2(headerDigest [32]byte, environmentCount int64, rolling [32]byte) ([32]byte, error) {
	if headerDigest == ([32]byte{}) || environmentCount < 1 || rolling == ([32]byte{}) {
		return [32]byte{}, invalidSyncAuthorityCandidateCodecV2()
	}
	transcript, err := encodeTerminalCandidateTranscriptV1(
		syncAuthorityFinalDomainV2,
		maximumSyncAuthorityFinalBytesV2,
		terminalCandidateUint16BytesV1(syncAuthorityCandidateCodecVersionV2),
		headerDigest[:],
		terminalCandidateInt64BytesV1(environmentCount),
		rolling[:],
	)
	if err != nil {
		return [32]byte{}, err
	}
	digest := sha256.Sum256(transcript)
	if digest == ([32]byte{}) {
		return [32]byte{}, invalidSyncAuthorityCandidateCodecV2()
	}
	return digest, nil
}

func validateSyncAuthoritySnapshotV2(projectID continuity.ProjectID, snapshot SyncAuthoritySnapshot) error {
	if err := validateSyncProjectID(projectID); err != nil {
		return err
	}
	authority := SyncAuthority{
		ChannelID:            snapshot.ChannelID,
		RelayGeneration:      snapshot.RelayGeneration,
		AdminPublicKey:       snapshot.AdminPublicKey,
		MembershipGeneration: snapshot.MembershipGeneration,
		InventoryArrivalHead: snapshot.InventoryArrivalHead,
	}
	if err := validateSyncAuthorityIdentity(authority); err != nil {
		return err
	}
	baseAbsent := snapshot.BaseAuthorityDigestVersion == 0 && snapshot.BaseAuthorityDigest == ([32]byte{})
	basePresent := (snapshot.BaseAuthorityDigestVersion == 1 || snapshot.BaseAuthorityDigestVersion == 2) && snapshot.BaseAuthorityDigest != ([32]byte{})
	if !baseAbsent && !basePresent {
		return syncProblem(SyncErrorInvalid, "base_authority_digest", "version and digest must be an exact absent or nonzero pair")
	}
	return nil
}

func validateSyncAuthorityCandidateEnvironmentV2(environment SyncEnvironmentCertificate, index int) error {
	prefix := "environment"
	if index >= 0 {
		prefix = fmt.Sprintf("environments[%d]", index)
	}
	if !validOpaqueID(environment.EnvironmentID) {
		return syncProblem(SyncErrorInvalid, prefix+".environment_id", "is invalid")
	}
	if environment.CertificateID == ([32]byte{}) {
		return syncProblem(SyncErrorInvalid, prefix+".certificate_id", "must be nonzero")
	}
	if len(environment.CertificateBytes) < 1 || len(environment.CertificateBytes) > maximumEnvironmentCertificateBytes {
		return syncProblem(SyncErrorInvalid, prefix+".certificate_bytes", "size is outside the protocol bound")
	}
	if environment.JoinMembershipGeneration == 0 {
		return syncProblem(SyncErrorInvalid, prefix+".join_membership_generation", "must begin at one")
	}
	switch environment.Mode {
	case SyncEnvironmentTrusted:
		if environment.ExpiresAtMillis != 0 {
			return syncProblem(SyncErrorInvalid, prefix+".expires_at_millis", "trusted environments must not expire")
		}
	case SyncEnvironmentEphemeral:
		if environment.ExpiresAtMillis <= 0 {
			return syncProblem(SyncErrorInvalid, prefix+".expires_at_millis", "ephemeral environments require a positive expiry")
		}
	default:
		return syncProblem(SyncErrorInvalid, prefix+".mode", "is invalid")
	}
	if environment.Retirement != nil {
		retirement := environment.Retirement
		if retirement.RelayGeneration == ([32]byte{}) || retirement.MembershipGeneration == 0 ||
			retirement.MembershipGeneration < environment.JoinMembershipGeneration || retirement.FinalEnvironmentSequence < 0 ||
			(retirement.FinalEnvironmentSequence == 0) != (retirement.FinalEnvelopeDigest == ([32]byte{})) ||
			retirement.RetirementID == ([32]byte{}) || len(retirement.RetirementBytes) < 1 ||
			len(retirement.RetirementBytes) > maximumEnvironmentRetirementBytes {
			return syncProblem(SyncErrorInvalid, prefix+".retirement", "is invalid")
		}
	}
	return nil
}

func checkedSyncAuthorityCandidateAdvanceV2(value, delta int64) (int64, error) {
	if value < 0 || delta < 0 || value > math.MaxInt64-delta {
		return 0, invalidSyncAuthorityCandidateCodecV2()
	}
	return value + delta, nil
}
