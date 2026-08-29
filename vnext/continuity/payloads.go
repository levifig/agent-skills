package continuity

import "fmt"

// HybridTime is the deterministic logical time assigned to one fact.
type HybridTime struct {
	WallMillis int64
	Logical    int32
}

// SubjectRef identifies one continuity subject inside a project.
type SubjectRef struct {
	Kind RecordKind
	ID   SubjectID
}

// Observation preserves optional display provenance without creating a
// conversation lifecycle entity.
type Observation struct {
	ObservedAtMillis int64
	HarnessSessionID string
	Branch           string
	Worktree         string
}

// JournalCategory is the closed vocabulary for project journal timeline labels.
// A label never creates or changes a first-class decision, spark, finding, or wrap;
// those families change only through their own typed facts.
type JournalCategory string

const (
	JournalNote     JournalCategory = "note"
	JournalSkill    JournalCategory = "skill"
	JournalCommit   JournalCategory = "commit"
	JournalDecision JournalCategory = "decision"
	JournalDiscover JournalCategory = "discover"
	JournalBlock    JournalCategory = "block"
	JournalUnblock  JournalCategory = "unblock"
	JournalSpark    JournalCategory = "spark"
	JournalTodo     JournalCategory = "todo"
	JournalFinding  JournalCategory = "finding"
	JournalWrap     JournalCategory = "wrap"
)

// CheckpointItemKind identifies one ordered exploration checkpoint item.
type CheckpointItemKind string

const (
	CheckpointCandidate  CheckpointItemKind = "candidate"
	CheckpointEvidence   CheckpointItemKind = "evidence"
	CheckpointConstraint CheckpointItemKind = "constraint"
)

// CheckpointItem is one ordered item in an exploration checkpoint.
type CheckpointItem struct {
	Kind CheckpointItemKind
	Text string
}

// VerificationOutcome is the closed result vocabulary for private evidence.
type VerificationOutcome string

const (
	VerificationPassed        VerificationOutcome = "passed"
	VerificationFailed        VerificationOutcome = "failed"
	VerificationIndeterminate VerificationOutcome = "indeterminate"
)

// JournalContent is the complete replacement content of a journal entry.
type JournalContent struct {
	Category JournalCategory
	Scope    string
	Text     string
}

// IdeaContent is the complete replacement content of an idea.
type IdeaContent struct {
	Label string
	Text  string
}

// FindingContent is the complete replacement content of a finding.
type FindingContent struct {
	Scope          string
	Summary        string
	Detail         string
	Recommendation string
}

// ProjectRegistrationPayload is the semantic content of project.registered.
type ProjectRegistrationPayload struct {
	Observation Observation
	Label       string
}

// ProjectLabelRevisionPayload is the semantic content of project.label-revised.
type ProjectLabelRevisionPayload struct {
	Observation Observation
	Revises     FactID
	Label       string
}

// JournalRecordedPayload is the semantic content of journal.recorded.
type JournalRecordedPayload struct {
	Observation Observation
	Content     JournalContent
}

// JournalCorrectionPayload is the semantic content of journal.correction-recorded.
type JournalCorrectionPayload struct {
	Observation Observation
	Corrects    FactID
	Content     JournalContent
}

// WrapRecordedPayload is the semantic content of wrap.recorded.
type WrapRecordedPayload struct {
	Observation Observation
	Focus       *SubjectRef
	Scope       string
	Synthesis   string
}

// SparkCapturedPayload is the semantic content of spark.captured.
type SparkCapturedPayload struct {
	Observation Observation
	Scope       string
	Text        string
}

// SparkDismissedPayload is the semantic content of spark.dismissed.
type SparkDismissedPayload struct {
	Observation Observation
	Predecessor FactID
	Reason      string
}

// SparkPromotionPayload is the semantic content of spark.promoted-to-idea.
type SparkPromotionPayload struct {
	Observation Observation
	Predecessor FactID
	IdeaID      SubjectID
}

// IdeaCreatedPayload is the semantic content of idea.created.
type IdeaCreatedPayload struct {
	Observation Observation
	Content     IdeaContent
}

// IdeaRevisionPayload is the semantic content of idea.revised.
type IdeaRevisionPayload struct {
	Observation Observation
	Revises     FactID
	Content     IdeaContent
}

// IdeaResolutionPayload is the semantic content of idea.resolved.
type IdeaResolutionPayload struct {
	Observation Observation
	Predecessor FactID
	Resolution  string
}

// IdeaArchivePayload is the semantic content of idea.archived.
type IdeaArchivePayload struct {
	Observation Observation
	Predecessor FactID
	Reason      string
}

// IdeaPromotionPayload is the semantic content of idea.promoted-to-external-reference.
type IdeaPromotionPayload struct {
	Observation Observation
	Predecessor FactID
	ReferenceID SubjectID
}

// DecisionOpenedPayload is the semantic content of decision.opened.
type DecisionOpenedPayload struct {
	Observation Observation
	Scope       string
	Question    string
	Context     string
}

// DecisionResolutionPayload is the semantic content of decision.resolved.
type DecisionResolutionPayload struct {
	Observation Observation
	Predecessor FactID
	Resolution  string
	Rationale   string
}

// DecisionSupersessionPayload is the semantic content of decision.superseded.
type DecisionSupersessionPayload struct {
	Observation Observation
	Predecessor FactID
	SuccessorID SubjectID
	Rationale   string
}

// ExplorationStartedPayload is the semantic content of exploration.started.
type ExplorationStartedPayload struct {
	Observation Observation
	Label       string
	Purpose     string
}

// CheckpointRecordedPayload is the semantic content of checkpoint.recorded.
type CheckpointRecordedPayload struct {
	Observation        Observation
	ExplorationID      SubjectID
	CurrentFraming     string
	Conclusions        string
	UnresolvedQuestion string
	NextAction         string
	Items              []CheckpointItem
}

// FindingRecordedPayload is the semantic content of finding.recorded.
type FindingRecordedPayload struct {
	Observation Observation
	Content     FindingContent
}

// FindingCorrectionPayload is the semantic content of finding.corrected.
type FindingCorrectionPayload struct {
	Observation Observation
	Corrects    FactID
	Content     FindingContent
}

// FindingRetractionPayload is the semantic content of finding.retracted.
type FindingRetractionPayload struct {
	Observation Observation
	Predecessor FactID
	Reason      string
}

// HandoffRecordedPayload is the semantic content of handoff.recorded.
type HandoffRecordedPayload struct {
	Observation       Observation
	Focus             *SubjectRef
	Purpose           string
	Situation         string
	NextActions       string
	QuestionsAndRisks string
	SuggestedSkills   []string
}

// ScratchpadOpenedPayload is the semantic content of scratchpad.opened.
type ScratchpadOpenedPayload struct {
	Observation Observation
	Focus       *SubjectRef
	Label       string
}

// ScratchpadParticipantPayload is the semantic content of scratchpad.participant-introduced.
type ScratchpadParticipantPayload struct {
	Observation   Observation
	ParticipantID SubjectID
	Name          string
	Focus         *SubjectRef
}

// ScratchpadMessagePayload is the semantic content of scratchpad.message-recorded.
type ScratchpadMessagePayload struct {
	Observation   Observation
	ParticipantID SubjectID
	Text          string
}

// ScratchpadClaimPayload is the semantic content of scratchpad.claim-recorded.
type ScratchpadClaimPayload struct {
	Observation     Observation
	ClaimID         SubjectID
	ParticipantID   SubjectID
	Resource        string
	ExpiresAtMillis int64
}

// ScratchpadClaimReleasePayload is the semantic content of scratchpad.claim-released.
type ScratchpadClaimReleasePayload struct {
	Observation Observation
	ClaimID     SubjectID
	ReleasedBy  SubjectID
	Reason      string
}

// ScratchpadClosePayload is the semantic content of scratchpad.closed.
type ScratchpadClosePayload struct {
	Observation Observation
	ClosedBy    SubjectID
	Reason      string
}

// ExternalReferenceRegistrationPayload is the semantic content of external-reference.registered.
type ExternalReferenceRegistrationPayload struct {
	Observation Observation
	Locator     string
}

// ExternalReferenceAttachmentPayload is the semantic content of external-reference.attached.
type ExternalReferenceAttachmentPayload struct {
	Observation Observation
	Target      SubjectRef
	Predecessor FactID
}

// ExternalReferenceDetachmentPayload is the semantic content of external-reference.detached.
type ExternalReferenceDetachmentPayload struct {
	Observation Observation
	Target      SubjectRef
	Predecessor FactID
	Reason      string
}

// VerificationEvidencePayload is the semantic content of verification-evidence.recorded.
type VerificationEvidencePayload struct {
	Observation Observation
	Target      SubjectRef
	Check       string
	Method      string
	Outcome     VerificationOutcome
	Detail      string
}

// AppendReceipt reports one inserted or exactly replayed fact after commit.
type AppendReceipt struct {
	FactID              FactID
	ProjectID           ProjectID
	Subject             SubjectRef
	Kind                FactKind
	EnvironmentID       EnvironmentID
	EnvironmentSequence int64
	Clock               HybridTime
	Replayed            bool
}

// ProblemCode classifies continuity failures without package-level state.
type ProblemCode string

const (
	ProblemInvalid                  ProblemCode = "invalid"
	ProblemFactConflict             ProblemCode = "fact-conflict"
	ProblemProjectNotRegistered     ProblemCode = "project-not-registered"
	ProblemProjectAlreadyRegistered ProblemCode = "project-already-registered"
	ProblemSubjectNotRegistered     ProblemCode = "subject-not-registered"
	ProblemSubjectAlreadyRegistered ProblemCode = "subject-already-registered"
	ProblemReferenceNotFound        ProblemCode = "reference-not-found"
	ProblemReferenceMismatch        ProblemCode = "reference-mismatch"
	ProblemPreconditionFailed       ProblemCode = "precondition-failed"
	ProblemClockExhausted           ProblemCode = "clock-exhausted"
	ProblemCorruptFact              ProblemCode = "corrupt-fact"
	ProblemStoreClosed              ProblemCode = "store-closed"
	ProblemStoreUnavailable         ProblemCode = "store-unavailable"
	ProblemCommitUnknown            ProblemCode = "commit-unknown"
)

// Problem provides stable machine classification and bounded human detail.
type Problem struct {
	Code   ProblemCode
	Field  string
	Detail string
}

func (problem *Problem) Error() string {
	if problem == nil {
		return "continuity problem"
	}
	if problem.Field == "" {
		return fmt.Sprintf("continuity %s: %s", problem.Code, problem.Detail)
	}
	return fmt.Sprintf("continuity %s at %s: %s", problem.Code, problem.Field, problem.Detail)
}
