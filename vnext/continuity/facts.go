package continuity

// FactID is the opaque mint-once identity of a persisted continuity fact.
type FactID string

// ProjectID is the opaque mint-once identity of a Loaf project.
type ProjectID string

// SubjectID is the opaque mint-once identity of a continuity subject.
type SubjectID string

// EnvironmentID identifies one fact-producing environment without deriving it
// from a machine name or path.
type EnvironmentID string

// FactKind identifies one closed continuity mutation.
type FactKind string

const (
	FactProjectRegistered               FactKind = "project.registered"
	FactProjectLabelRevised             FactKind = "project.label-revised"
	FactJournalRecorded                 FactKind = "journal.recorded"
	FactJournalCorrectionRecorded       FactKind = "journal.correction-recorded"
	FactWrapRecorded                    FactKind = "wrap.recorded"
	FactSparkCaptured                   FactKind = "spark.captured"
	FactSparkDismissed                  FactKind = "spark.dismissed"
	FactSparkPromotedToIdea             FactKind = "spark.promoted-to-idea"
	FactIdeaCreated                     FactKind = "idea.created"
	FactIdeaRevised                     FactKind = "idea.revised"
	FactIdeaResolved                    FactKind = "idea.resolved"
	FactIdeaArchived                    FactKind = "idea.archived"
	FactIdeaPromotedToExternalReference FactKind = "idea.promoted-to-external-reference"
	FactDecisionOpened                  FactKind = "decision.opened"
	FactDecisionResolved                FactKind = "decision.resolved"
	FactDecisionSuperseded              FactKind = "decision.superseded"
	FactExplorationStarted              FactKind = "exploration.started"
	FactCheckpointRecorded              FactKind = "checkpoint.recorded"
	FactFindingRecorded                 FactKind = "finding.recorded"
	FactFindingCorrected                FactKind = "finding.corrected"
	FactFindingRetracted                FactKind = "finding.retracted"
	FactHandoffRecorded                 FactKind = "handoff.recorded"
	FactScratchpadOpened                FactKind = "scratchpad.opened"
	FactScratchpadParticipantIntroduced FactKind = "scratchpad.participant-introduced"
	FactScratchpadMessageRecorded       FactKind = "scratchpad.message-recorded"
	FactScratchpadClaimRecorded         FactKind = "scratchpad.claim-recorded"
	FactScratchpadClaimReleased         FactKind = "scratchpad.claim-released"
	FactScratchpadClosed                FactKind = "scratchpad.closed"
	FactExternalReferenceRegistered     FactKind = "external-reference.registered"
	FactExternalReferenceAttached       FactKind = "external-reference.attached"
	FactExternalReferenceDetached       FactKind = "external-reference.detached"
	FactVerificationEvidenceRecorded    FactKind = "verification-evidence.recorded"
)

// FactDefinition binds one fact kind to its only valid record family.
type FactDefinition struct {
	Kind   FactKind
	Record RecordKind
}

// FactCatalog returns the closed persisted-fact catalog in canonical order.
func FactCatalog() []FactDefinition {
	return []FactDefinition{
		{Kind: FactProjectRegistered, Record: RecordProjectIdentity},
		{Kind: FactProjectLabelRevised, Record: RecordProjectIdentity},
		{Kind: FactJournalRecorded, Record: RecordJournalEntry},
		{Kind: FactJournalCorrectionRecorded, Record: RecordJournalEntry},
		{Kind: FactWrapRecorded, Record: RecordWrap},
		{Kind: FactSparkCaptured, Record: RecordSpark},
		{Kind: FactSparkDismissed, Record: RecordSpark},
		{Kind: FactSparkPromotedToIdea, Record: RecordSpark},
		{Kind: FactIdeaCreated, Record: RecordIdea},
		{Kind: FactIdeaRevised, Record: RecordIdea},
		{Kind: FactIdeaResolved, Record: RecordIdea},
		{Kind: FactIdeaArchived, Record: RecordIdea},
		{Kind: FactIdeaPromotedToExternalReference, Record: RecordIdea},
		{Kind: FactDecisionOpened, Record: RecordDecision},
		{Kind: FactDecisionResolved, Record: RecordDecision},
		{Kind: FactDecisionSuperseded, Record: RecordDecision},
		{Kind: FactExplorationStarted, Record: RecordExploration},
		{Kind: FactCheckpointRecorded, Record: RecordCheckpoint},
		{Kind: FactFindingRecorded, Record: RecordFinding},
		{Kind: FactFindingCorrected, Record: RecordFinding},
		{Kind: FactFindingRetracted, Record: RecordFinding},
		{Kind: FactHandoffRecorded, Record: RecordHandoff},
		{Kind: FactScratchpadOpened, Record: RecordScratchpad},
		{Kind: FactScratchpadParticipantIntroduced, Record: RecordScratchpad},
		{Kind: FactScratchpadMessageRecorded, Record: RecordScratchpad},
		{Kind: FactScratchpadClaimRecorded, Record: RecordScratchpad},
		{Kind: FactScratchpadClaimReleased, Record: RecordScratchpad},
		{Kind: FactScratchpadClosed, Record: RecordScratchpad},
		{Kind: FactExternalReferenceRegistered, Record: RecordExternalReference},
		{Kind: FactExternalReferenceAttached, Record: RecordExternalReference},
		{Kind: FactExternalReferenceDetached, Record: RecordExternalReference},
		{Kind: FactVerificationEvidenceRecorded, Record: RecordVerificationEvidence},
	}
}

// DefinitionFor returns the definition for kind.
func DefinitionFor(kind FactKind) (FactDefinition, bool) {
	for _, definition := range FactCatalog() {
		if definition.Kind == kind {
			return definition, true
		}
	}
	return FactDefinition{}, false
}
