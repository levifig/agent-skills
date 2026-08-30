package sqlite

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"math"

	"github.com/levifig/loaf/vnext/continuity"
	"github.com/levifig/loaf/vnext/internal/continuitywire"
)

const (
	terminalCandidateCodecVersionV1      uint16 = 1
	terminalCandidatePrunedBodyVersionV2 uint16 = 2

	terminalCandidateAuthorityDomainV1         = "loaf.continuity.sync.authority-snapshot.v1"
	terminalCandidateEnvironmentDomainV1       = "loaf.continuity.sync.authority-snapshot.environment.v1"
	terminalCandidateRetirementDomainV1        = "loaf.continuity.sync.authority-snapshot.retirement.v1"
	terminalCandidateIdentityDomainV1          = "loaf.continuity.sync.terminal-candidate.id.v1"
	terminalCandidatePrunedBodyDomainV1        = "loaf.continuity.sync.terminal-candidate.pruned-anchor.v1"
	terminalCandidateFrameDomainV1             = "loaf.continuity.sync.terminal-candidate.frame.v1"
	terminalCandidateRollingSeedDomainV1       = "loaf.continuity.sync.terminal-candidate.rolling-seed.v1"
	terminalCandidateRollingStepDomainV1       = "loaf.continuity.sync.terminal-candidate.rolling-step.v1"
	terminalCandidateAuthorityFieldCountV1     = 8
	terminalCandidateEnvironmentFieldCountV1   = 9
	terminalCandidateRetirementFieldCountV1    = 7
	terminalCandidateIdentityFieldCountV1      = 7
	terminalCandidatePrunedBodyFieldCountV1    = 6
	terminalCandidateFrameFieldCountV1         = 18
	terminalCandidateRollingSeedFieldCountV1   = 2
	terminalCandidateRollingStepFieldCountV1   = 5
	maximumTerminalCandidateTranscriptFieldsV1 = terminalCandidateFrameFieldCountV1
	maximumTerminalCandidateAuthorityBytesV1   = 3_270_208
	maximumTerminalCandidateIdentityBytesV1    = 512
	maximumTerminalCandidatePrunedBodyBytesV1  = 256
	maximumTerminalCandidateFrameBytesV1       = 1_053_512
	maximumTerminalCandidateRollingBytesV1     = 256
	maximumTerminalCandidateChunkFramesV1      = 16
	maximumTerminalCandidateChunkBytesV1       = 16_842_752
	maximumTerminalCandidateReferencedInboxV1  = 17_632_000
	terminalCandidateFrameKindSealedV1         = "sealed"
	terminalCandidateFrameKindPrunedV1         = "pruned"
)

const (
	terminalCandidateInvalidErrorV1  = "invalid continuity terminal candidate codec"
	terminalCandidateTooLargeErrorV1 = "continuity terminal candidate exceeds fixed limit"
)

func invalidTerminalCandidateCodecV1() error { return errors.New(terminalCandidateInvalidErrorV1) }

func terminalCandidateTooLargeV1() error { return errors.New(terminalCandidateTooLargeErrorV1) }

type terminalCandidatePrunedBodyV1 struct {
	ReferenceDigest  [32]byte
	InboxFrameDigest [32]byte
	FactKind         continuity.FactKind
	Clock            continuity.HybridTime
}

type terminalCandidateFrameV1 struct {
	projectID              continuity.ProjectID
	candidateID            [32]byte
	arrivalSequence        int64
	frameKind              string
	factID                 continuity.FactID
	environmentID          continuity.EnvironmentID
	environmentSequence    int64
	clock                  continuity.HybridTime
	previousEnvelopeDigest [32]byte
	envelopeDigest         [32]byte
	certificateID          [32]byte
	keyGeneration          uint32
	nonce                  [24]byte
	pruneCertificateID     *[32]byte
	candidateBytes         []byte
}

type terminalCandidateChunkBudgetV1 struct {
	frameCount           int
	candidateBytes       uint64
	referencedInboxBytes uint64
}

func deriveTerminalCandidateIdentityV1(projectID continuity.ProjectID, authority SyncAuthority, startArrivalSequence int64) ([32]byte, [32]byte, error) {
	authorityTranscript, err := terminalCandidateAuthorityTranscriptV1(projectID, authority)
	if err != nil {
		return [32]byte{}, [32]byte{}, err
	}
	authorityDigest := sha256.Sum256(authorityTranscript)
	binding := SyncAuthorityBinding{
		ChannelID:              authority.ChannelID,
		RelayGeneration:        authority.RelayGeneration,
		AdminPublicKey:         authority.AdminPublicKey,
		MembershipGeneration:   authority.MembershipGeneration,
		InventoryArrivalHead:   authority.InventoryArrivalHead,
		AuthorityDigestVersion: 1,
		AuthorityDigest:        authorityDigest,
	}
	candidateID, err := deriveTerminalCandidateIDFromAuthorityBindingV1(projectID, binding, startArrivalSequence)
	if err != nil {
		return [32]byte{}, [32]byte{}, err
	}
	return authorityDigest, candidateID, nil
}

func deriveTerminalCandidateIDFromAuthorityBindingV1(projectID continuity.ProjectID, binding SyncAuthorityBinding, startArrivalSequence int64) ([32]byte, error) {
	if projectID.Validate() != nil || validateSyncAuthorityBindingV2(binding) != nil {
		return [32]byte{}, invalidTerminalCandidateCodecV1()
	}
	if startArrivalSequence < 1 {
		return [32]byte{}, invalidTerminalCandidateCodecV1()
	}
	identityTranscript, err := encodeTerminalCandidateTranscriptV1(
		terminalCandidateIdentityDomainV1,
		maximumTerminalCandidateIdentityBytesV1,
		terminalCandidateUint16BytesV1(terminalCandidateCodecVersionV1),
		[]byte(projectID),
		binding.ChannelID[:],
		binding.RelayGeneration[:],
		terminalCandidateUint32BytesV1(binding.MembershipGeneration),
		binding.AuthorityDigest[:],
		terminalCandidateInt64BytesV1(startArrivalSequence),
	)
	if err != nil {
		return [32]byte{}, err
	}
	return sha256.Sum256(identityTranscript), nil
}

func terminalCandidateAuthorityTranscriptV1(projectID continuity.ProjectID, authority SyncAuthority) ([]byte, error) {
	if projectID.Validate() != nil || validateSyncAuthority(authority) != nil || authority.InventoryArrivalHead != 0 {
		return nil, invalidTerminalCandidateCodecV1()
	}
	environments := make([][]byte, 0, len(authority.Environments))
	for _, environment := range authority.Environments {
		retirementPresent := []byte{0}
		var retirementBytes []byte
		if environment.Retirement != nil {
			retirementPresent[0] = 1
			retirement := environment.Retirement
			var err error
			retirementBytes, err = encodeTerminalCandidateTranscriptV1(
				terminalCandidateRetirementDomainV1,
				maximumEnvironmentRetirementBytes+512,
				terminalCandidateUint16BytesV1(terminalCandidateCodecVersionV1),
				retirement.RelayGeneration[:],
				terminalCandidateUint32BytesV1(retirement.MembershipGeneration),
				terminalCandidateInt64BytesV1(retirement.FinalEnvironmentSequence),
				retirement.FinalEnvelopeDigest[:],
				retirement.RetirementID[:],
				retirement.RetirementBytes,
			)
			if err != nil {
				return nil, err
			}
		}
		encoded, err := encodeTerminalCandidateTranscriptV1(
			terminalCandidateEnvironmentDomainV1,
			maximumEnvironmentCertificateBytes+maximumEnvironmentRetirementBytes+1_024,
			terminalCandidateUint16BytesV1(terminalCandidateCodecVersionV1),
			[]byte(environment.EnvironmentID),
			environment.CertificateID[:],
			environment.CertificateBytes,
			[]byte(environment.Mode),
			terminalCandidateInt64BytesV1(environment.ExpiresAtMillis),
			terminalCandidateUint32BytesV1(environment.JoinMembershipGeneration),
			retirementPresent,
			retirementBytes,
		)
		if err != nil {
			return nil, err
		}
		environments = append(environments, encoded)
	}
	environmentList, err := encodeTerminalCandidateListV1(environments, maximumSyncAuthorityEnvironments, maximumTerminalCandidateAuthorityBytesV1)
	if err != nil {
		return nil, err
	}
	return encodeTerminalCandidateTranscriptV1(
		terminalCandidateAuthorityDomainV1,
		maximumTerminalCandidateAuthorityBytesV1,
		terminalCandidateUint16BytesV1(terminalCandidateCodecVersionV1),
		[]byte(projectID),
		authority.ChannelID[:],
		authority.RelayGeneration[:],
		authority.AdminPublicKey[:],
		terminalCandidateUint32BytesV1(authority.MembershipGeneration),
		terminalCandidateUint32BytesV1(uint32(len(authority.Environments))),
		environmentList,
	)
}

func encodeTerminalCandidateSealedBodyV1(projectID continuity.ProjectID, fact continuitywire.Fact) ([]byte, error) {
	if err := validateTerminalCandidateFactV1(projectID, fact); err != nil {
		return nil, err
	}
	encoded, err := continuitywire.Encode(fact)
	if err != nil {
		return nil, invalidTerminalCandidateCodecV1()
	}
	return encoded, nil
}

func decodeTerminalCandidateSealedBodyV1(projectID continuity.ProjectID, encoded []byte) (continuitywire.Fact, error) {
	fact, err := continuitywire.Decode(encoded)
	if err != nil || validateTerminalCandidateFactV1(projectID, fact) != nil {
		return continuitywire.Fact{}, invalidTerminalCandidateCodecV1()
	}
	return fact, nil
}

func validateTerminalCandidateFactV1(projectID continuity.ProjectID, fact continuitywire.Fact) error {
	if projectID.Validate() != nil || fact.ProjectID != projectID || continuitywire.Validate(fact) != nil {
		return invalidTerminalCandidateCodecV1()
	}
	canonical, err := canonicalizeStoredContentV1(fact.FactKind, int(fact.PayloadVersion), string(fact.CanonicalPayload))
	if err != nil || !bytes.Equal([]byte(canonical), fact.CanonicalPayload) {
		return invalidTerminalCandidateCodecV1()
	}
	stored := storedFactV1{
		factID:              fact.FactID,
		projectID:           fact.ProjectID,
		subject:             continuity.SubjectRef{Kind: fact.SubjectKind, ID: fact.SubjectID},
		kind:                fact.FactKind,
		payloadVersion:      int(fact.PayloadVersion),
		content:             canonical,
		environmentID:       fact.EnvironmentID,
		environmentSequence: fact.EnvironmentSequence,
		clock:               continuity.HybridTime{WallMillis: fact.HLCWallMillis, Logical: fact.HLCLogical},
		envelopeVersion:     int(fact.EnvelopeVersion),
	}
	if validateStoredFactV1(stored) != nil {
		return invalidTerminalCandidateCodecV1()
	}
	return nil
}

func encodeTerminalCandidatePrunedBodyV1(body terminalCandidatePrunedBodyV1) ([]byte, error) {
	if body.ReferenceDigest == ([32]byte{}) || body.InboxFrameDigest == ([32]byte{}) || !prunableScratchpadFactKindV1(body.FactKind) ||
		body.Clock.WallMillis < 0 || body.Clock.Logical < 0 {
		return nil, invalidTerminalCandidateCodecV1()
	}
	return encodeTerminalCandidateTranscriptV1(
		terminalCandidatePrunedBodyDomainV1,
		maximumTerminalCandidatePrunedBodyBytesV1,
		terminalCandidateUint16BytesV1(terminalCandidatePrunedBodyVersionV2),
		body.ReferenceDigest[:],
		body.InboxFrameDigest[:],
		[]byte(body.FactKind),
		terminalCandidateInt64BytesV1(body.Clock.WallMillis),
		terminalCandidateInt32BytesV1(body.Clock.Logical),
	)
}

func decodeTerminalCandidatePrunedBodyV1(encoded []byte) (terminalCandidatePrunedBodyV1, error) {
	if len(encoded) < 1 || len(encoded) > maximumTerminalCandidatePrunedBodyBytesV1 {
		return terminalCandidatePrunedBodyV1{}, invalidTerminalCandidateCodecV1()
	}
	fields, err := parseTerminalCandidateTranscriptV1(encoded, terminalCandidatePrunedBodyDomainV1, terminalCandidatePrunedBodyFieldCountV1)
	if err != nil || len(fields[0]) != 2 || len(fields[1]) != 32 || len(fields[2]) != 32 || len(fields[4]) != 8 || len(fields[5]) != 4 ||
		binary.BigEndian.Uint16(fields[0]) != terminalCandidatePrunedBodyVersionV2 {
		return terminalCandidatePrunedBodyV1{}, invalidTerminalCandidateCodecV1()
	}
	body := terminalCandidatePrunedBodyV1{
		FactKind: continuity.FactKind(string(fields[3])),
		Clock: continuity.HybridTime{
			WallMillis: int64(binary.BigEndian.Uint64(fields[4])),
			Logical:    int32(binary.BigEndian.Uint32(fields[5])),
		},
	}
	copy(body.ReferenceDigest[:], fields[1])
	copy(body.InboxFrameDigest[:], fields[2])
	canonical, encodeErr := encodeTerminalCandidatePrunedBodyV1(body)
	if encodeErr != nil || !bytes.Equal(canonical, encoded) {
		return terminalCandidatePrunedBodyV1{}, invalidTerminalCandidateCodecV1()
	}
	return body, nil
}

func terminalCandidateFrameDigestV1(frame terminalCandidateFrameV1) ([32]byte, error) {
	if err := validateTerminalCandidateFrameV1(frame); err != nil {
		return [32]byte{}, err
	}
	prunePresent := []byte{0}
	var pruneCertificateID []byte
	if frame.pruneCertificateID != nil {
		prunePresent[0] = 1
		pruneCertificateID = frame.pruneCertificateID[:]
	}
	transcript, err := encodeTerminalCandidateTranscriptV1(
		terminalCandidateFrameDomainV1,
		maximumTerminalCandidateFrameBytesV1,
		terminalCandidateUint16BytesV1(terminalCandidateCodecVersionV1),
		[]byte(frame.projectID),
		frame.candidateID[:],
		terminalCandidateInt64BytesV1(frame.arrivalSequence),
		[]byte(frame.frameKind),
		[]byte(frame.factID),
		[]byte(frame.environmentID),
		terminalCandidateInt64BytesV1(frame.environmentSequence),
		terminalCandidateInt64BytesV1(frame.clock.WallMillis),
		terminalCandidateInt32BytesV1(frame.clock.Logical),
		frame.previousEnvelopeDigest[:],
		frame.envelopeDigest[:],
		frame.certificateID[:],
		terminalCandidateUint32BytesV1(frame.keyGeneration),
		frame.nonce[:],
		prunePresent,
		pruneCertificateID,
		frame.candidateBytes,
	)
	if err != nil {
		return [32]byte{}, err
	}
	return sha256.Sum256(transcript), nil
}

func validateTerminalCandidateFrameV1(frame terminalCandidateFrameV1) error {
	if frame.projectID.Validate() != nil || frame.candidateID == ([32]byte{}) || frame.arrivalSequence < 1 ||
		frame.factID.Validate() != nil || frame.environmentID.Validate() != nil || frame.environmentSequence < 1 ||
		frame.clock.WallMillis < 0 || frame.clock.Logical < 0 || frame.envelopeDigest == ([32]byte{}) ||
		frame.certificateID == ([32]byte{}) || frame.keyGeneration < 1 ||
		(frame.environmentSequence == 1) != (frame.previousEnvelopeDigest == ([32]byte{})) {
		return invalidTerminalCandidateCodecV1()
	}
	switch frame.frameKind {
	case terminalCandidateFrameKindSealedV1:
		if frame.pruneCertificateID != nil || len(frame.candidateBytes) < 2 || len(frame.candidateBytes) > continuitywire.MaxFactBytes {
			return invalidTerminalCandidateCodecV1()
		}
		fact, err := decodeTerminalCandidateSealedBodyV1(frame.projectID, frame.candidateBytes)
		if err != nil || fact.FactID != frame.factID || fact.EnvironmentID != frame.environmentID ||
			fact.EnvironmentSequence != frame.environmentSequence || fact.HLCWallMillis != frame.clock.WallMillis ||
			fact.HLCLogical != frame.clock.Logical {
			return invalidTerminalCandidateCodecV1()
		}
	case terminalCandidateFrameKindPrunedV1:
		if frame.pruneCertificateID == nil || *frame.pruneCertificateID == ([32]byte{}) ||
			len(frame.candidateBytes) < 1 || len(frame.candidateBytes) > maximumTerminalCandidatePrunedBodyBytesV1 {
			return invalidTerminalCandidateCodecV1()
		}
		body, err := decodeTerminalCandidatePrunedBodyV1(frame.candidateBytes)
		if err != nil || body.Clock != frame.clock {
			return invalidTerminalCandidateCodecV1()
		}
		referenceDigest, err := continuitywire.PruneReferenceDigest(continuitywire.PruneReference{
			FactID:                 frame.factID,
			EnvironmentID:          frame.environmentID,
			EnvironmentSequence:    frame.environmentSequence,
			ArrivalSequence:        frame.arrivalSequence,
			EnvelopeDigest:         frame.envelopeDigest,
			CertificateID:          frame.certificateID,
			PreviousEnvelopeDigest: frame.previousEnvelopeDigest,
			KeyGeneration:          frame.keyGeneration,
			Nonce:                  frame.nonce,
		})
		if err != nil || referenceDigest != body.ReferenceDigest {
			return invalidTerminalCandidateCodecV1()
		}
	default:
		return invalidTerminalCandidateCodecV1()
	}
	return nil
}

func terminalCandidateRollingSeedV1(candidateID [32]byte) ([32]byte, error) {
	if candidateID == ([32]byte{}) {
		return [32]byte{}, invalidTerminalCandidateCodecV1()
	}
	transcript, err := encodeTerminalCandidateTranscriptV1(
		terminalCandidateRollingSeedDomainV1,
		maximumTerminalCandidateRollingBytesV1,
		terminalCandidateUint16BytesV1(terminalCandidateCodecVersionV1),
		candidateID[:],
	)
	if err != nil {
		return [32]byte{}, err
	}
	return sha256.Sum256(transcript), nil
}

func terminalCandidateRollingStepV1(candidateID [32]byte, resultingFrameCount int64, previousDigest, frameDigest [32]byte) ([32]byte, error) {
	if candidateID == ([32]byte{}) || resultingFrameCount < 1 || previousDigest == ([32]byte{}) || frameDigest == ([32]byte{}) {
		return [32]byte{}, invalidTerminalCandidateCodecV1()
	}
	transcript, err := encodeTerminalCandidateTranscriptV1(
		terminalCandidateRollingStepDomainV1,
		maximumTerminalCandidateRollingBytesV1,
		terminalCandidateUint16BytesV1(terminalCandidateCodecVersionV1),
		candidateID[:],
		terminalCandidateInt64BytesV1(resultingFrameCount),
		previousDigest[:],
		frameDigest[:],
	)
	if err != nil {
		return [32]byte{}, err
	}
	return sha256.Sum256(transcript), nil
}

func (budget *terminalCandidateChunkBudgetV1) admit(candidateBytes, referencedInboxBytes int) error {
	if budget == nil || candidateBytes < 1 || referencedInboxBytes < 1 || budget.frameCount < 0 ||
		candidateBytes > continuitywire.MaxFactBytes || referencedInboxBytes > maximumSealedEnvelopeBytes ||
		budget.frameCount >= maximumTerminalCandidateChunkFramesV1 || budget.candidateBytes > maximumTerminalCandidateChunkBytesV1 ||
		budget.referencedInboxBytes > maximumTerminalCandidateReferencedInboxV1 {
		return invalidTerminalCandidateCodecV1()
	}
	candidateSize := uint64(candidateBytes)
	inboxSize := uint64(referencedInboxBytes)
	if candidateSize > maximumTerminalCandidateChunkBytesV1-budget.candidateBytes ||
		inboxSize > maximumTerminalCandidateReferencedInboxV1-budget.referencedInboxBytes {
		return terminalCandidateTooLargeV1()
	}
	budget.frameCount++
	budget.candidateBytes += candidateSize
	budget.referencedInboxBytes += inboxSize
	return nil
}

func encodeTerminalCandidateTranscriptV1(domain string, limit int, fields ...[]byte) ([]byte, error) {
	if domain == "" || uint64(len(domain)) > math.MaxUint32 || limit < 1 || len(fields) > maximumTerminalCandidateTranscriptFieldsV1 {
		return nil, invalidTerminalCandidateCodecV1()
	}
	total := uint64(8 + len(domain))
	for _, field := range fields {
		if uint64(len(field)) > math.MaxUint32 {
			return nil, terminalCandidateTooLargeV1()
		}
		total += 4 + uint64(len(field))
		if total > uint64(limit) {
			return nil, terminalCandidateTooLargeV1()
		}
	}
	encoded := make([]byte, 0, int(total))
	encoded = binary.BigEndian.AppendUint32(encoded, uint32(len(domain)))
	encoded = append(encoded, domain...)
	encoded = binary.BigEndian.AppendUint32(encoded, uint32(len(fields)))
	for _, field := range fields {
		encoded = binary.BigEndian.AppendUint32(encoded, uint32(len(field)))
		encoded = append(encoded, field...)
	}
	return encoded, nil
}

func parseTerminalCandidateTranscriptV1(encoded []byte, domain string, fieldCount int) ([][]byte, error) {
	if len(encoded) < 8 || domain == "" || fieldCount < 0 || fieldCount > maximumTerminalCandidateTranscriptFieldsV1 {
		return nil, invalidTerminalCandidateCodecV1()
	}
	domainLength := binary.BigEndian.Uint32(encoded[:4])
	if uint64(domainLength) > uint64(len(encoded)-8) {
		return nil, invalidTerminalCandidateCodecV1()
	}
	offset := 4
	if string(encoded[offset:offset+int(domainLength)]) != domain {
		return nil, invalidTerminalCandidateCodecV1()
	}
	offset += int(domainLength)
	if offset+4 > len(encoded) || binary.BigEndian.Uint32(encoded[offset:offset+4]) != uint32(fieldCount) {
		return nil, invalidTerminalCandidateCodecV1()
	}
	offset += 4
	fields := make([][]byte, 0, fieldCount)
	for range fieldCount {
		if offset+4 > len(encoded) {
			return nil, invalidTerminalCandidateCodecV1()
		}
		fieldLength := binary.BigEndian.Uint32(encoded[offset : offset+4])
		offset += 4
		if uint64(fieldLength) > uint64(len(encoded)-offset) {
			return nil, invalidTerminalCandidateCodecV1()
		}
		fields = append(fields, append([]byte(nil), encoded[offset:offset+int(fieldLength)]...))
		offset += int(fieldLength)
	}
	if offset != len(encoded) {
		return nil, invalidTerminalCandidateCodecV1()
	}
	return fields, nil
}

func encodeTerminalCandidateListV1(values [][]byte, maximumCount, maximumBytes int) ([]byte, error) {
	if len(values) > maximumCount || maximumBytes < 4 {
		return nil, invalidTerminalCandidateCodecV1()
	}
	total := uint64(4)
	for _, value := range values {
		if uint64(len(value)) > math.MaxUint32 {
			return nil, terminalCandidateTooLargeV1()
		}
		total += 4 + uint64(len(value))
		if total > uint64(maximumBytes) {
			return nil, terminalCandidateTooLargeV1()
		}
	}
	encoded := make([]byte, 0, int(total))
	encoded = binary.BigEndian.AppendUint32(encoded, uint32(len(values)))
	for _, value := range values {
		encoded = binary.BigEndian.AppendUint32(encoded, uint32(len(value)))
		encoded = append(encoded, value...)
	}
	return encoded, nil
}

func terminalCandidateUint16BytesV1(value uint16) []byte {
	return binary.BigEndian.AppendUint16(nil, value)
}

func terminalCandidateUint32BytesV1(value uint32) []byte {
	return binary.BigEndian.AppendUint32(nil, value)
}

func terminalCandidateInt32BytesV1(value int32) []byte {
	return binary.BigEndian.AppendUint32(nil, uint32(value))
}

func terminalCandidateInt64BytesV1(value int64) []byte {
	return binary.BigEndian.AppendUint64(nil, uint64(value))
}
