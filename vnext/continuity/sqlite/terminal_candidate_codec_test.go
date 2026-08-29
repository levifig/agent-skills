package sqlite

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/levifig/loaf/vnext/continuity"
	"github.com/levifig/loaf/vnext/internal/continuitywire"
)

func TestTerminalCandidateCodecIdentityAndPrunedBodyAreDeterministicV1(t *testing.T) {
	t.Parallel()

	projectID := continuity.ProjectID("project-terminal-codec")
	authority := testSyncAuthority()
	authorityDigest, candidateID, err := deriveTerminalCandidateIdentityV1(projectID, authority, 7)
	if err != nil {
		t.Fatalf("deriveTerminalCandidateIdentityV1() error = %v", err)
	}
	secondAuthorityDigest, secondCandidateID, err := deriveTerminalCandidateIdentityV1(projectID, cloneSyncAuthority(authority), 7)
	if err != nil {
		t.Fatalf("deriveTerminalCandidateIdentityV1(retry) error = %v", err)
	}
	if authorityDigest == ([32]byte{}) || candidateID == ([32]byte{}) {
		t.Fatal("derived identity contains a zero digest")
	}
	if authorityDigest != secondAuthorityDigest || candidateID != secondCandidateID {
		t.Fatal("derived identity is not deterministic")
	}

	body := terminalCandidatePrunedBodyV1{
		ReferenceDigest: testAuthorityDigest(0x51),
		FactKind:        continuity.FactScratchpadMessageRecorded,
		Clock:           continuity.HybridTime{WallMillis: 1234, Logical: 5},
	}
	encoded, err := encodeTerminalCandidatePrunedBodyV1(body)
	if err != nil {
		t.Fatalf("encodeTerminalCandidatePrunedBodyV1() error = %v", err)
	}
	decoded, err := decodeTerminalCandidatePrunedBodyV1(encoded)
	if err != nil {
		t.Fatalf("decodeTerminalCandidatePrunedBodyV1() error = %v", err)
	}
	if decoded != body {
		t.Fatalf("decoded body = %#v, want %#v", decoded, body)
	}
	reencoded, err := encodeTerminalCandidatePrunedBodyV1(decoded)
	if err != nil {
		t.Fatalf("re-encode pruned body: %v", err)
	}
	if !bytes.Equal(reencoded, encoded) {
		t.Fatal("pruned body re-encoding changed canonical bytes")
	}
}

func TestTerminalCandidateAuthorityDigestBindsCompleteSnapshotV1(t *testing.T) {
	t.Parallel()

	projectID := continuity.ProjectID("project-terminal-authority")
	base := testSyncAuthority()
	baseAuthorityDigest, baseCandidateID, err := deriveTerminalCandidateIdentityV1(projectID, base, 11)
	if err != nil {
		t.Fatalf("derive base identity: %v", err)
	}
	tests := map[string]func(*continuity.ProjectID, *SyncAuthority, *int64){
		"project": func(project *continuity.ProjectID, _ *SyncAuthority, _ *int64) {
			*project = "project-terminal-authority-other"
		},
		"channel": func(_ *continuity.ProjectID, authority *SyncAuthority, _ *int64) {
			authority.ChannelID = testSyncChannelID("terminal-authority-other-channel")
		},
		"relay generation": func(_ *continuity.ProjectID, authority *SyncAuthority, _ *int64) {
			authority.RelayGeneration = testAuthorityDigest(0x71)
			authority.Environments[0].Retirement.RelayGeneration = authority.RelayGeneration
		},
		"admin key": func(_ *continuity.ProjectID, authority *SyncAuthority, _ *int64) {
			authority.AdminPublicKey = testAuthorityDigest(0x72)
		},
		"certificate id": func(_ *continuity.ProjectID, authority *SyncAuthority, _ *int64) {
			authority.Environments[1].CertificateID = testAuthorityDigest(0x73)
		},
		"certificate bytes": func(_ *continuity.ProjectID, authority *SyncAuthority, _ *int64) {
			authority.Environments[1].CertificateBytes = []byte("changed-certificate-bytes")
		},
		"environment id": func(_ *continuity.ProjectID, authority *SyncAuthority, _ *int64) {
			authority.Environments[1].EnvironmentID = "environment-c"
		},
		"environment mode and expiry": func(_ *continuity.ProjectID, authority *SyncAuthority, _ *int64) {
			authority.Environments[1].Mode = SyncEnvironmentTrusted
			authority.Environments[1].ExpiresAtMillis = 0
		},
		"join generations": func(_ *continuity.ProjectID, authority *SyncAuthority, _ *int64) {
			authority.Environments[0].JoinMembershipGeneration = 2
			authority.Environments[1].JoinMembershipGeneration = 1
		},
		"retirement final sequence": func(_ *continuity.ProjectID, authority *SyncAuthority, _ *int64) {
			authority.Environments[0].Retirement.FinalEnvironmentSequence = 4
			authority.Environments[0].Retirement.FinalEnvelopeDigest = testAuthorityDigest(0x74)
		},
		"retirement final digest": func(_ *continuity.ProjectID, authority *SyncAuthority, _ *int64) {
			authority.Environments[0].Retirement.FinalEnvelopeDigest = testAuthorityDigest(0x77)
		},
		"retirement membership": func(_ *continuity.ProjectID, authority *SyncAuthority, _ *int64) {
			authority.Environments[0].Retirement.MembershipGeneration = 2
			authority.Environments[1].JoinMembershipGeneration = 3
		},
		"retirement id": func(_ *continuity.ProjectID, authority *SyncAuthority, _ *int64) {
			authority.Environments[0].Retirement.RetirementID = testAuthorityDigest(0x75)
		},
		"retirement bytes": func(_ *continuity.ProjectID, authority *SyncAuthority, _ *int64) {
			authority.Environments[0].Retirement.RetirementBytes = []byte("changed-retirement-bytes")
		},
		"retirement presence": func(_ *continuity.ProjectID, authority *SyncAuthority, _ *int64) {
			authority.MembershipGeneration = 2
			authority.Environments[0].Retirement = nil
		},
		"start arrival": func(_ *continuity.ProjectID, _ *SyncAuthority, start *int64) {
			*start = 12
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			variantProject := projectID
			variantAuthority := cloneSyncAuthority(base)
			start := int64(11)
			mutate(&variantProject, &variantAuthority, &start)
			authorityDigest, candidateID, err := deriveTerminalCandidateIdentityV1(variantProject, variantAuthority, start)
			if err != nil {
				t.Fatalf("derive mutated identity: %v", err)
			}
			if name == "start arrival" {
				if authorityDigest != baseAuthorityDigest || candidateID == baseCandidateID {
					t.Fatal("start arrival did not affect only candidate identity")
				}
				return
			}
			if authorityDigest == baseAuthorityDigest || candidateID == baseCandidateID {
				t.Fatal("authority mutation did not change both bound digests")
			}
		})
	}

	invalid := cloneSyncAuthority(base)
	invalid.Environments[0], invalid.Environments[1] = invalid.Environments[1], invalid.Environments[0]
	authorityDigest, candidateID, err := deriveTerminalCandidateIdentityV1(projectID, invalid, 11)
	if !terminalCandidateErrorIsV1(err, terminalCandidateInvalidErrorV1) || authorityDigest != ([32]byte{}) || candidateID != ([32]byte{}) {
		t.Fatalf("invalid authority result = (%x, %x, %v), want zero content-free refusal", authorityDigest, candidateID, err)
	}
}

func TestTerminalCandidateAuthorityTranscriptAcceptsExactMaximumInventoryV1(t *testing.T) {
	t.Parallel()

	relayGeneration := testAuthorityDigest(0x78)
	authority := SyncAuthority{
		ChannelID:            testSyncChannelID("terminal-authority-maximum"),
		RelayGeneration:      relayGeneration,
		AdminPublicKey:       testAuthorityDigest(0x79),
		MembershipGeneration: maximumSyncAuthorityEnvironments * 2,
		Environments:         make([]SyncEnvironmentCertificate, maximumSyncAuthorityEnvironments),
	}
	for index := range authority.Environments {
		prefix := fmt.Sprintf("environment-%03d-", index)
		environmentID := prefix + strings.Repeat("x", 128-len(prefix))
		authority.Environments[index] = SyncEnvironmentCertificate{
			EnvironmentID:            environmentID,
			CertificateID:            sha256.Sum256([]byte("certificate:" + environmentID)),
			CertificateBytes:         bytes.Repeat([]byte{byte(index)}, maximumEnvironmentCertificateBytes),
			Mode:                     SyncEnvironmentEphemeral,
			ExpiresAtMillis:          math.MaxInt64,
			JoinMembershipGeneration: uint32(index + 1),
			Retirement: &SyncEnvironmentRetirement{
				RelayGeneration:          relayGeneration,
				MembershipGeneration:     uint32(maximumSyncAuthorityEnvironments + index + 1),
				FinalEnvironmentSequence: 0,
				RetirementID:             sha256.Sum256([]byte("retirement:" + environmentID)),
				RetirementBytes:          bytes.Repeat([]byte{byte(255 - index)}, maximumEnvironmentRetirementBytes),
			},
		}
	}
	projectID := continuity.ProjectID(strings.Repeat("p", 128))
	transcript, err := terminalCandidateAuthorityTranscriptV1(projectID, authority)
	if err != nil {
		t.Fatalf("maximum authority transcript: %v", err)
	}
	if len(transcript) != maximumTerminalCandidateAuthorityBytesV1 {
		t.Fatalf("maximum authority transcript bytes = %d, want exact bound %d", len(transcript), maximumTerminalCandidateAuthorityBytesV1)
	}
	authorityDigest, candidateID, err := deriveTerminalCandidateIdentityV1(projectID, authority, math.MaxInt64)
	if err != nil || authorityDigest == ([32]byte{}) || candidateID == ([32]byte{}) {
		t.Fatalf("maximum authority identity = (%x, %x, %v), want valid", authorityDigest, candidateID, err)
	}
	before := append([]byte(nil), transcript...)
	authority.Environments[0].CertificateBytes[0] ^= 0xff
	if !bytes.Equal(transcript, before) {
		t.Fatal("authority transcript aliases input certificate bytes")
	}
}

func TestTerminalCandidateAuthorityDigestBindsMembershipGenerationV1(t *testing.T) {
	t.Parallel()

	projectID := continuity.ProjectID("project-terminal-membership")
	base := testActiveSyncAuthority()
	baseDigest, baseCandidate, err := deriveTerminalCandidateIdentityV1(projectID, base, 1)
	if err != nil {
		t.Fatalf("derive base identity: %v", err)
	}
	advanced := cloneSyncAuthority(base)
	advanced.MembershipGeneration = 4
	advanced.Environments[2].Retirement = &SyncEnvironmentRetirement{
		RelayGeneration:          advanced.RelayGeneration,
		MembershipGeneration:     4,
		FinalEnvironmentSequence: 0,
		RetirementID:             testAuthorityDigest(0x76),
		RetirementBytes:          []byte("terminal-membership-retirement"),
	}
	advancedDigest, advancedCandidate, err := deriveTerminalCandidateIdentityV1(projectID, advanced, 1)
	if err != nil {
		t.Fatalf("derive advanced identity: %v", err)
	}
	if advancedDigest == baseDigest || advancedCandidate == baseCandidate {
		t.Fatal("membership advance did not change bound digests")
	}
}

func TestTerminalCandidateSealedBodyIsExactClosedFactWireV1(t *testing.T) {
	t.Parallel()

	projectID := continuity.ProjectID("project-terminal-sealed")
	fact := syncProjectFact(t, projectID, "fact-terminal-sealed", "environment-a", 1, 101)
	want, err := continuitywire.Encode(fact)
	if err != nil {
		t.Fatalf("encode fact fixture: %v", err)
	}
	encoded, err := encodeTerminalCandidateSealedBodyV1(projectID, fact)
	if err != nil {
		t.Fatalf("encodeTerminalCandidateSealedBodyV1() error = %v", err)
	}
	if !bytes.Equal(encoded, want) {
		t.Fatal("sealed candidate bytes differ from canonical continuity wire")
	}

	decoded, err := decodeTerminalCandidateSealedBodyV1(projectID, encoded)
	if err != nil {
		t.Fatalf("decodeTerminalCandidateSealedBodyV1() error = %v", err)
	}
	if !continuitywire.Equal(decoded, fact) {
		t.Fatalf("decoded fact = %#v, want %#v", decoded, fact)
	}
	encoded[0] ^= 0xff
	if !continuitywire.Equal(decoded, fact) {
		t.Fatal("decoded fact aliases encoded input")
	}

	wrongProject := fact
	wrongProject.ProjectID = "project-terminal-sealed-other"
	if _, err := encodeTerminalCandidateSealedBodyV1(projectID, wrongProject); !terminalCandidateErrorIsV1(err, terminalCandidateInvalidErrorV1) {
		t.Fatalf("wrong-project error = %v, want codec refusal", err)
	}
	invalidPayload := fact
	invalidPayload.CanonicalPayload = []byte("{}")
	if err := continuitywire.Validate(invalidPayload); err != nil {
		t.Fatalf("structural fixture unexpectedly invalid: %v", err)
	}
	if _, err := encodeTerminalCandidateSealedBodyV1(projectID, invalidPayload); !terminalCandidateErrorIsV1(err, terminalCandidateInvalidErrorV1) {
		t.Fatalf("closed-payload error = %v, want codec refusal", err)
	}
	trailing := append(append([]byte(nil), want...), ' ')
	if _, err := decodeTerminalCandidateSealedBodyV1(projectID, trailing); !terminalCandidateErrorIsV1(err, terminalCandidateInvalidErrorV1) {
		t.Fatalf("noncanonical sealed error = %v, want codec refusal", err)
	}
}

func TestTerminalCandidatePrunedBodyIsStrictAndSchemaBoundedV1(t *testing.T) {
	t.Parallel()

	for _, kind := range []continuity.FactKind{
		continuity.FactScratchpadParticipantIntroduced,
		continuity.FactScratchpadMessageRecorded,
		continuity.FactScratchpadClaimRecorded,
		continuity.FactScratchpadClaimReleased,
	} {
		body := terminalCandidatePrunedBodyV1{
			ReferenceDigest: sha256.Sum256([]byte("reference:" + string(kind))),
			FactKind:        kind,
			Clock:           continuity.HybridTime{WallMillis: math.MaxInt64, Logical: math.MaxInt32},
		}
		encoded, err := encodeTerminalCandidatePrunedBodyV1(body)
		if err != nil {
			t.Fatalf("encode %q: %v", kind, err)
		}
		if len(encoded) > maximumTerminalCandidatePrunedBodyBytesV1 {
			t.Fatalf("encoded %q bytes = %d, schema maximum = %d", kind, len(encoded), maximumTerminalCandidatePrunedBodyBytesV1)
		}
		decoded, err := decodeTerminalCandidatePrunedBodyV1(encoded)
		if err != nil || decoded != body {
			t.Fatalf("decode %q = (%#v, %v), want %#v", kind, decoded, err, body)
		}
		for _, malformed := range [][]byte{
			nil,
			encoded[:len(encoded)-1],
			append(append([]byte(nil), encoded...), 0),
			append([]byte(nil), encoded...),
		} {
			if len(malformed) == len(encoded) {
				malformed[4] ^= 0x01
			}
			if _, err := decodeTerminalCandidatePrunedBodyV1(malformed); !terminalCandidateErrorIsV1(err, terminalCandidateInvalidErrorV1) {
				t.Fatalf("malformed %q error = %v, want codec refusal", kind, err)
			}
		}
	}

	invalid := []terminalCandidatePrunedBodyV1{
		{FactKind: continuity.FactScratchpadMessageRecorded, Clock: continuity.HybridTime{}},
		{ReferenceDigest: testAuthorityDigest(0x61), FactKind: continuity.FactScratchpadOpened, Clock: continuity.HybridTime{}},
		{ReferenceDigest: testAuthorityDigest(0x62), FactKind: continuity.FactScratchpadMessageRecorded, Clock: continuity.HybridTime{WallMillis: -1}},
		{ReferenceDigest: testAuthorityDigest(0x63), FactKind: continuity.FactScratchpadMessageRecorded, Clock: continuity.HybridTime{Logical: -1}},
	}
	for _, body := range invalid {
		if _, err := encodeTerminalCandidatePrunedBodyV1(body); !terminalCandidateErrorIsV1(err, terminalCandidateInvalidErrorV1) {
			t.Fatalf("invalid body %#v error = %v, want codec refusal", body, err)
		}
	}
	oversized := make([]byte, maximumTerminalCandidatePrunedBodyBytesV1+1)
	if _, err := decodeTerminalCandidatePrunedBodyV1(oversized); !terminalCandidateErrorIsV1(err, terminalCandidateInvalidErrorV1) {
		t.Fatalf("oversized pruned body error = %v, want bounded refusal", err)
	}
	tooManyFields := make([][]byte, maximumTerminalCandidateTranscriptFieldsV1+1)
	if _, err := encodeTerminalCandidateTranscriptV1("bounded-test", 1024, tooManyFields...); !terminalCandidateErrorIsV1(err, terminalCandidateInvalidErrorV1) {
		t.Fatalf("oversized transcript field count error = %v, want bounded refusal", err)
	}
	oversizedFieldHeader := binary.BigEndian.AppendUint32(nil, uint32(len("bounded-test")))
	oversizedFieldHeader = append(oversizedFieldHeader, "bounded-test"...)
	oversizedFieldHeader = binary.BigEndian.AppendUint32(oversizedFieldHeader, uint32(len(tooManyFields)))
	for range tooManyFields {
		oversizedFieldHeader = binary.BigEndian.AppendUint32(oversizedFieldHeader, 0)
	}
	if _, err := parseTerminalCandidateTranscriptV1(oversizedFieldHeader, "bounded-test", len(tooManyFields)); !terminalCandidateErrorIsV1(err, terminalCandidateInvalidErrorV1) {
		t.Fatalf("oversized parse field count error = %v, want pre-allocation refusal", err)
	}
}

func TestTerminalCandidateFrameDigestBindsNormalizedSealedAndPrunedRowsV1(t *testing.T) {
	t.Parallel()

	sealed := terminalCodecSealedFrameV1(t)
	sealedDigest, err := terminalCandidateFrameDigestV1(sealed)
	if err != nil {
		t.Fatalf("sealed frame digest: %v", err)
	}
	if sealedDigest == ([32]byte{}) {
		t.Fatal("sealed frame digest is zero")
	}
	sealedMutations := map[string]func(*terminalCandidateFrameV1){
		"candidate id": func(frame *terminalCandidateFrameV1) { frame.candidateID = testAuthorityDigest(0x81) },
		"arrival":      func(frame *terminalCandidateFrameV1) { frame.arrivalSequence++ },
		"previous":     func(frame *terminalCandidateFrameV1) { frame.previousEnvelopeDigest = testAuthorityDigest(0x82) },
		"envelope":     func(frame *terminalCandidateFrameV1) { frame.envelopeDigest = testAuthorityDigest(0x83) },
		"certificate":  func(frame *terminalCandidateFrameV1) { frame.certificateID = testAuthorityDigest(0x84) },
		"key generation": func(frame *terminalCandidateFrameV1) {
			frame.keyGeneration++
		},
		"nonce": func(frame *terminalCandidateFrameV1) { frame.nonce[0] ^= 0xff },
	}
	for name, mutate := range sealedMutations {
		t.Run("sealed "+name, func(t *testing.T) {
			variant := cloneTerminalCandidateFrameV1(sealed)
			mutate(&variant)
			digest, err := terminalCandidateFrameDigestV1(variant)
			if err != nil {
				t.Fatalf("digest mutated frame: %v", err)
			}
			if digest == sealedDigest {
				t.Fatal("normalized row mutation did not change frame digest")
			}
		})
	}

	for name, mutateFact := range map[string]func(*continuitywire.Fact){
		"project": func(fact *continuitywire.Fact) {
			fact.ProjectID = "project-terminal-frame-other"
			fact.SubjectID = continuity.SubjectID(fact.ProjectID)
		},
		"fact id":              func(fact *continuitywire.Fact) { fact.FactID = "fact-terminal-frame-other" },
		"environment":          func(fact *continuitywire.Fact) { fact.EnvironmentID = "environment-b" },
		"environment sequence": func(fact *continuitywire.Fact) { fact.EnvironmentSequence++ },
		"hlc wall":             func(fact *continuitywire.Fact) { fact.HLCWallMillis++ },
		"hlc logical":          func(fact *continuitywire.Fact) { fact.HLCLogical++ },
	} {
		t.Run("sealed "+name+" and body", func(t *testing.T) {
			variant := cloneTerminalCandidateFrameV1(sealed)
			fact, err := decodeTerminalCandidateSealedBodyV1(variant.projectID, variant.candidateBytes)
			if err != nil {
				t.Fatalf("decode sealed fixture: %v", err)
			}
			mutateFact(&fact)
			variant.projectID = fact.ProjectID
			variant.factID = fact.FactID
			variant.environmentID = fact.EnvironmentID
			variant.environmentSequence = fact.EnvironmentSequence
			variant.clock = continuity.HybridTime{WallMillis: fact.HLCWallMillis, Logical: fact.HLCLogical}
			if variant.environmentSequence == 1 {
				variant.previousEnvelopeDigest = [32]byte{}
			} else if variant.previousEnvelopeDigest == ([32]byte{}) {
				variant.previousEnvelopeDigest = testAuthorityDigest(0x85)
			}
			variant.candidateBytes, err = encodeTerminalCandidateSealedBodyV1(variant.projectID, fact)
			if err != nil {
				t.Fatalf("encode mutated sealed body: %v", err)
			}
			digest, err := terminalCandidateFrameDigestV1(variant)
			if err != nil {
				t.Fatalf("digest coupled mutation: %v", err)
			}
			if digest == sealedDigest {
				t.Fatal("coupled row/body mutation did not change frame digest")
			}
		})
	}

	pruned := terminalCodecPrunedFrameV1(t)
	prunedDigest, err := terminalCandidateFrameDigestV1(pruned)
	if err != nil {
		t.Fatalf("pruned frame digest: %v", err)
	}
	if prunedDigest == sealedDigest {
		t.Fatal("sealed and pruned frame digests collide for distinct normalized rows")
	}
	changedCertificate := cloneTerminalCandidateFrameV1(pruned)
	certificateID := testAuthorityDigest(0x86)
	changedCertificate.pruneCertificateID = &certificateID
	changedDigest, err := terminalCandidateFrameDigestV1(changedCertificate)
	if err != nil || changedDigest == prunedDigest {
		t.Fatalf("changed prune certificate digest = (%x, %v), want distinct valid digest", changedDigest, err)
	}

	badReference := cloneTerminalCandidateFrameV1(pruned)
	badReference.envelopeDigest = testAuthorityDigest(0x87)
	if _, err := terminalCandidateFrameDigestV1(badReference); !terminalCandidateErrorIsV1(err, terminalCandidateInvalidErrorV1) {
		t.Fatalf("mismatched prune reference error = %v, want codec refusal", err)
	}
	badBody := cloneTerminalCandidateFrameV1(sealed)
	badBody.candidateBytes[len(badBody.candidateBytes)-1] ^= 0x01
	if _, err := terminalCandidateFrameDigestV1(badBody); !terminalCandidateErrorIsV1(err, terminalCandidateInvalidErrorV1) {
		t.Fatalf("mismatched sealed body error = %v, want codec refusal", err)
	}
}

func TestTerminalCandidateRollingDigestIsChunkInvariantWithoutLifetimeCapV1(t *testing.T) {
	t.Parallel()

	candidateID := testAuthorityDigest(0x91)
	frameDigests := make([][32]byte, 5_001)
	for index := range frameDigests {
		frameDigests[index] = sha256.Sum256(terminalCandidateInt64BytesV1(int64(index + 1)))
	}
	want := rollTerminalCandidateDigestsV1(t, candidateID, frameDigests, []int{len(frameDigests)})
	for _, chunks := range [][]int{
		{1, 5_000},
		{16, 16, 16, 4_953},
		{2_048, 2_048, 905},
	} {
		if got := rollTerminalCandidateDigestsV1(t, candidateID, frameDigests, chunks); got != want {
			t.Fatalf("rolling digest for chunks %v = %x, want %x", chunks, got, want)
		}
	}
	seed, err := terminalCandidateRollingSeedV1(candidateID)
	if err != nil {
		t.Fatalf("rolling seed: %v", err)
	}
	maximumStep, err := terminalCandidateRollingStepV1(candidateID, math.MaxInt64, seed, frameDigests[0])
	if err != nil || maximumStep == ([32]byte{}) {
		t.Fatalf("maximum-count rolling step = (%x, %v), want valid", maximumStep, err)
	}
	changedCount, err := terminalCandidateRollingStepV1(candidateID, math.MaxInt64-1, seed, frameDigests[0])
	if err != nil || changedCount == maximumStep {
		t.Fatal("resulting frame count is not bound into rolling step")
	}
	if _, err := terminalCandidateRollingStepV1(candidateID, 0, seed, frameDigests[0]); !terminalCandidateErrorIsV1(err, terminalCandidateInvalidErrorV1) {
		t.Fatalf("zero-count rolling error = %v, want codec refusal", err)
	}
}

func TestTerminalCandidateChunkBudgetEnforcesPerTransactionLimitsV1(t *testing.T) {
	t.Parallel()

	var budget terminalCandidateChunkBudgetV1
	for range maximumTerminalCandidateChunkFramesV1 {
		if err := budget.admit(continuitywire.MaxFactBytes, maximumSealedEnvelopeBytes); err != nil {
			t.Fatalf("admit exact maximum frame %d: %v", budget.frameCount+1, err)
		}
	}
	if budget.frameCount != maximumTerminalCandidateChunkFramesV1 ||
		budget.candidateBytes != maximumTerminalCandidateChunkBytesV1 ||
		budget.referencedInboxBytes != maximumTerminalCandidateReferencedInboxV1 {
		t.Fatalf("exact budget = %#v", budget)
	}
	before := budget
	if err := budget.admit(1, 1); err == nil || budget != before {
		t.Fatalf("seventeenth-frame result = (%#v, %v), want unchanged refusal", budget, err)
	}

	for _, sizes := range [][2]int{
		{0, 1},
		{1, 0},
		{-1, 1},
		{continuitywire.MaxFactBytes + 1, 1},
		{1, maximumSealedEnvelopeBytes + 1},
		{math.MaxInt, math.MaxInt},
	} {
		var candidate terminalCandidateChunkBudgetV1
		if err := candidate.admit(sizes[0], sizes[1]); err == nil || candidate != (terminalCandidateChunkBudgetV1{}) {
			t.Fatalf("admit sizes %v result = (%#v, %v), want unchanged refusal", sizes, candidate, err)
		}
	}
	overflow := terminalCandidateChunkBudgetV1{
		frameCount:           1,
		candidateBytes:       maximumTerminalCandidateChunkBytesV1 - 1,
		referencedInboxBytes: maximumTerminalCandidateReferencedInboxV1 - 1,
	}
	before = overflow
	if err := overflow.admit(2, 2); !terminalCandidateErrorIsV1(err, terminalCandidateTooLargeErrorV1) || overflow != before {
		t.Fatalf("overflow result = (%#v, %v), want unchanged bounded refusal", overflow, err)
	}
}

func TestTerminalCandidateCodecErrorsDoNotExposeContentV1(t *testing.T) {
	t.Parallel()

	secret := "do-not-expose-terminal-candidate-content"
	authority := testSyncAuthority()
	authority.Environments[0].EnvironmentID = secret + "*"
	_, _, err := deriveTerminalCandidateIdentityV1("project-terminal-error", authority, 1)
	if !terminalCandidateErrorIsV1(err, terminalCandidateInvalidErrorV1) || strings.Contains(err.Error(), secret) {
		t.Fatalf("authority error = %q, want content-free codec refusal", err)
	}
	body := terminalCandidatePrunedBodyV1{
		ReferenceDigest: testAuthorityDigest(0xa1),
		FactKind:        continuity.FactKind(secret),
	}
	_, err = encodeTerminalCandidatePrunedBodyV1(body)
	if !terminalCandidateErrorIsV1(err, terminalCandidateInvalidErrorV1) || strings.Contains(err.Error(), secret) {
		t.Fatalf("pruned-body error = %q, want content-free codec refusal", err)
	}
}

func TestTerminalCandidateCodecMatchesPublishedVectorsV1(t *testing.T) {
	t.Parallel()

	encoded, err := os.ReadFile(filepath.Join("testdata", "terminal_candidate_codec_v1.json"))
	if err != nil {
		t.Fatalf("read terminal candidate vector: %v", err)
	}
	var want terminalCandidateCodecVectorV1
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&want); err != nil {
		t.Fatalf("decode terminal candidate vector: %v", err)
	}
	got := terminalCandidateCodecVectorFixtureV1(t)
	if !reflect.DeepEqual(got, want) {
		formatted, marshalErr := json.MarshalIndent(got, "", "  ")
		if marshalErr != nil {
			t.Fatalf("marshal current terminal candidate vector: %v", marshalErr)
		}
		t.Fatalf("terminal candidate vector drifted; current vector:\n%s", formatted)
	}
}

type terminalCandidateCodecVectorV1 struct {
	Version                     uint16 `json:"version"`
	AuthorityTranscriptHex      string `json:"authority_transcript_hex"`
	AuthorityDigestHex          string `json:"authority_digest_hex"`
	CandidateIDHex              string `json:"candidate_id_hex"`
	PrunedBodyHex               string `json:"pruned_body_hex"`
	PrunedBodyDigestHex         string `json:"pruned_body_digest_hex"`
	SealedFrameDigestHex        string `json:"sealed_frame_digest_hex"`
	PrunedFrameDigestHex        string `json:"pruned_frame_digest_hex"`
	RollingSeedHex              string `json:"rolling_seed_hex"`
	RollingAfterSealedHex       string `json:"rolling_after_sealed_hex"`
	RollingAfterSealedPrunedHex string `json:"rolling_after_sealed_pruned_hex"`
}

func terminalCandidateCodecVectorFixtureV1(t *testing.T) terminalCandidateCodecVectorV1 {
	t.Helper()
	projectID := continuity.ProjectID("project-terminal-vector")
	authority := testSyncAuthority()
	authorityTranscript, err := terminalCandidateAuthorityTranscriptV1(projectID, authority)
	if err != nil {
		t.Fatalf("vector authority transcript: %v", err)
	}
	authorityDigest, candidateID, err := deriveTerminalCandidateIdentityV1(projectID, authority, 17)
	if err != nil {
		t.Fatalf("vector candidate identity: %v", err)
	}
	prunedBody, err := encodeTerminalCandidatePrunedBodyV1(terminalCandidatePrunedBodyV1{
		ReferenceDigest: sha256.Sum256([]byte("terminal-candidate-vector-reference")),
		FactKind:        continuity.FactScratchpadMessageRecorded,
		Clock:           continuity.HybridTime{WallMillis: 1_727_000_000_123, Logical: 17},
	})
	if err != nil {
		t.Fatalf("vector pruned body: %v", err)
	}
	sealedFrame := terminalCodecSealedFrameV1(t)
	sealedFrame.candidateID = candidateID
	sealedFrameDigest, err := terminalCandidateFrameDigestV1(sealedFrame)
	if err != nil {
		t.Fatalf("vector sealed frame: %v", err)
	}
	prunedFrame := terminalCodecPrunedFrameV1(t)
	prunedFrame.candidateID = candidateID
	prunedFrameDigest, err := terminalCandidateFrameDigestV1(prunedFrame)
	if err != nil {
		t.Fatalf("vector pruned frame: %v", err)
	}
	rollingSeed, err := terminalCandidateRollingSeedV1(candidateID)
	if err != nil {
		t.Fatalf("vector rolling seed: %v", err)
	}
	rollingAfterSealed, err := terminalCandidateRollingStepV1(candidateID, 1, rollingSeed, sealedFrameDigest)
	if err != nil {
		t.Fatalf("vector rolling sealed step: %v", err)
	}
	rollingAfterPruned, err := terminalCandidateRollingStepV1(candidateID, 2, rollingAfterSealed, prunedFrameDigest)
	if err != nil {
		t.Fatalf("vector rolling pruned step: %v", err)
	}
	prunedBodyDigest := sha256.Sum256(prunedBody)
	return terminalCandidateCodecVectorV1{
		Version:                     terminalCandidateCodecVersionV1,
		AuthorityTranscriptHex:      hex.EncodeToString(authorityTranscript),
		AuthorityDigestHex:          hex.EncodeToString(authorityDigest[:]),
		CandidateIDHex:              hex.EncodeToString(candidateID[:]),
		PrunedBodyHex:               hex.EncodeToString(prunedBody),
		PrunedBodyDigestHex:         hex.EncodeToString(prunedBodyDigest[:]),
		SealedFrameDigestHex:        hex.EncodeToString(sealedFrameDigest[:]),
		PrunedFrameDigestHex:        hex.EncodeToString(prunedFrameDigest[:]),
		RollingSeedHex:              hex.EncodeToString(rollingSeed[:]),
		RollingAfterSealedHex:       hex.EncodeToString(rollingAfterSealed[:]),
		RollingAfterSealedPrunedHex: hex.EncodeToString(rollingAfterPruned[:]),
	}
}

func terminalCodecSealedFrameV1(t *testing.T) terminalCandidateFrameV1 {
	t.Helper()
	projectID := continuity.ProjectID("project-terminal-frame")
	fact := syncIdeaCreatedFact(t, projectID, "fact-terminal-frame", "idea-terminal-frame", "environment-a", 2, 202, "Terminal frame")
	body, err := encodeTerminalCandidateSealedBodyV1(projectID, fact)
	if err != nil {
		t.Fatalf("encode sealed fixture: %v", err)
	}
	return terminalCandidateFrameV1{
		projectID:              projectID,
		candidateID:            testAuthorityDigest(0xb1),
		arrivalSequence:        7,
		frameKind:              terminalCandidateFrameKindSealedV1,
		factID:                 fact.FactID,
		environmentID:          fact.EnvironmentID,
		environmentSequence:    fact.EnvironmentSequence,
		clock:                  continuity.HybridTime{WallMillis: fact.HLCWallMillis, Logical: fact.HLCLogical},
		previousEnvelopeDigest: testAuthorityDigest(0xb2),
		envelopeDigest:         testAuthorityDigest(0xb3),
		certificateID:          testAuthorityDigest(0xb4),
		keyGeneration:          3,
		nonce:                  [24]byte{1, 2, 3, 4},
		candidateBytes:         body,
	}
}

func terminalCodecPrunedFrameV1(t *testing.T) terminalCandidateFrameV1 {
	t.Helper()
	frame := terminalCandidateFrameV1{
		projectID:              "project-terminal-frame",
		candidateID:            testAuthorityDigest(0xc1),
		arrivalSequence:        9,
		frameKind:              terminalCandidateFrameKindPrunedV1,
		factID:                 "fact-terminal-pruned",
		environmentID:          "environment-b",
		environmentSequence:    3,
		clock:                  continuity.HybridTime{WallMillis: 303, Logical: 4},
		previousEnvelopeDigest: testAuthorityDigest(0xc2),
		envelopeDigest:         testAuthorityDigest(0xc3),
		certificateID:          testAuthorityDigest(0xc4),
		keyGeneration:          4,
		nonce:                  [24]byte{5, 6, 7, 8},
	}
	pruneCertificateID := testAuthorityDigest(0xc5)
	frame.pruneCertificateID = &pruneCertificateID
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
	if err != nil {
		t.Fatalf("derive prune reference fixture: %v", err)
	}
	frame.candidateBytes, err = encodeTerminalCandidatePrunedBodyV1(terminalCandidatePrunedBodyV1{
		ReferenceDigest: referenceDigest,
		FactKind:        continuity.FactScratchpadClaimRecorded,
		Clock:           frame.clock,
	})
	if err != nil {
		t.Fatalf("encode pruned fixture: %v", err)
	}
	return frame
}

func cloneTerminalCandidateFrameV1(frame terminalCandidateFrameV1) terminalCandidateFrameV1 {
	clone := frame
	clone.candidateBytes = append([]byte(nil), frame.candidateBytes...)
	if frame.pruneCertificateID != nil {
		pruneCertificateID := *frame.pruneCertificateID
		clone.pruneCertificateID = &pruneCertificateID
	}
	return clone
}

func rollTerminalCandidateDigestsV1(t *testing.T, candidateID [32]byte, frameDigests [][32]byte, chunks []int) [32]byte {
	t.Helper()
	rolling, err := terminalCandidateRollingSeedV1(candidateID)
	if err != nil {
		t.Fatalf("rolling seed: %v", err)
	}
	offset := 0
	for _, chunk := range chunks {
		if chunk < 0 || offset+chunk > len(frameDigests) {
			t.Fatalf("invalid chunk plan %v", chunks)
		}
		for index := 0; index < chunk; index++ {
			rolling, err = terminalCandidateRollingStepV1(candidateID, int64(offset+index+1), rolling, frameDigests[offset+index])
			if err != nil {
				t.Fatalf("rolling step %d: %v", offset+index+1, err)
			}
		}
		offset += chunk
	}
	if offset != len(frameDigests) {
		t.Fatalf("chunk plan %v covers %d of %d frames", chunks, offset, len(frameDigests))
	}
	return rolling
}

func terminalCandidateErrorIsV1(err error, message string) bool {
	return err != nil && err.Error() == message
}
