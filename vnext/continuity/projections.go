package continuity

// FactStamp identifies one persisted fact. Projection order compares wall time,
// logical time, environment ID, and fact ID in that order using byte-exact IDs.
// EnvironmentSequence is provenance and never participates in that comparison.
type FactStamp struct {
	Clock               HybridTime
	EnvironmentID       EnvironmentID
	EnvironmentSequence int64
	FactID              FactID
}

// RecordVersion identifies the canonical root and semantic head of one record.
// Root is the least eligible mint in the canonical causal closure. Head is the
// winner after family-specific class dominance, so it need not be the greatest
// stamp when a terminal class dominates a later nonterminal fact.
type RecordVersion struct {
	ProjectID ProjectID
	Subject   SubjectRef
	Root      FactStamp
	Head      FactStamp
}

// SnapshotRequest selects deterministic time-relative projection behavior.
// AtMillis evaluates scratchpad claim expiry only. It never filters facts by
// hybrid time, and zero means the Unix epoch rather than the current clock.
type SnapshotRequest struct {
	AtMillis int64
}

// Validate checks a snapshot request without consulting a clock.
func (request SnapshotRequest) Validate() error {
	if request.AtMillis < 0 {
		return invalid("at_millis", "must be zero or positive")
	}
	return nil
}

// ContextRequest selects exact typed and textual context without inference.
// Focus must identify an existing same-project continuity subject. Empty Scope
// and Branch are absent selectors. Nonempty values match exact bytes. Focus
// precedes Scope, which precedes Branch, and overlaps are removed by subject
// before caps. FocusedJournal contains only an exact journal focus;
// ProjectJournal ranks Scope, then Branch, then the project remainder. Scope
// applies only to effective journal entries, active sparks, decisions, and
// effective findings. Branch applies only to the winning journal content
// observation. AtMillis has the snapshot meaning even though scratchpad content
// is excluded from context.
type ContextRequest struct {
	Focus    *SubjectRef
	Scope    string
	Branch   string
	AtMillis int64
}

// Validate checks a context request without consulting persisted state.
func (request ContextRequest) Validate() error {
	if request.AtMillis < 0 {
		return invalid("at_millis", "must be zero or positive")
	}
	if request.Focus != nil {
		if err := request.Focus.ID.Validate(); err != nil {
			return refield(err, "focus.id")
		}
		switch request.Focus.Kind {
		case RecordProjectIdentity,
			RecordJournalEntry,
			RecordWrap,
			RecordSpark,
			RecordIdea,
			RecordDecision,
			RecordExploration,
			RecordCheckpoint,
			RecordFinding,
			RecordHandoff,
			RecordExternalReference:
		default:
			return invalid("focus.kind", "is not available to derived context")
		}
	}
	if err := validateOptionalShort("scope", request.Scope); err != nil {
		return err
	}
	return validateOptionalText("branch", request.Branch, maximumLocatorBytes, false)
}

// IdeaDisposition is the closed current-state vocabulary for an idea.
type IdeaDisposition string

const (
	IdeaActive   IdeaDisposition = "active"
	IdeaResolved IdeaDisposition = "resolved"
	IdeaArchived IdeaDisposition = "archived"
	IdeaPromoted IdeaDisposition = "promoted"
)

// DecisionState is the closed current-state vocabulary for a decision.
type DecisionState string

const (
	DecisionOpen       DecisionState = "open"
	DecisionResolved   DecisionState = "resolved"
	DecisionSuperseded DecisionState = "superseded"
)

// FindingState is the closed current-state vocabulary for a finding.
type FindingState string

const (
	FindingCurrent   FindingState = "current"
	FindingRetracted FindingState = "retracted"
)

// ScratchpadState is the closed visibility state of a scratchpad.
type ScratchpadState string

const (
	ScratchpadOpen   ScratchpadState = "open"
	ScratchpadClosed ScratchpadState = "closed"
)

// ProjectIdentity is the current label and provenance of a project identity.
type ProjectIdentity struct {
	Record                RecordVersion
	Label                 string
	RegisteredObservation Observation
	HeadObservation       Observation
}

// JournalEntry is one effective entry anchored at its original timeline position.
type JournalEntry struct {
	Record              RecordVersion
	Content             JournalContent
	RecordedObservation Observation
	HeadObservation     Observation
}

// Wrap is one immutable synthesis record.
type Wrap struct {
	Record          RecordVersion
	Focus           *SubjectRef
	Scope           string
	Synthesis       string
	HeadObservation Observation
}

// Spark is one active captured thought.
type Spark struct {
	Record          RecordVersion
	Scope           string
	Text            string
	HeadObservation Observation
}

// Idea is one current idea view, including any terminal disposition.
type Idea struct {
	Record              RecordVersion
	Content             IdeaContent
	ContentStamp        FactStamp
	Disposition         IdeaDisposition
	Resolution          string
	ArchiveReason       string
	ExternalReferenceID SubjectID
	ContentObservation  Observation
	HeadObservation     Observation
}

// Decision is one current decision view.
type Decision struct {
	Record                RecordVersion
	Scope                 string
	Question              string
	Context               string
	State                 DecisionState
	Resolution            string
	ResolutionRationale   string
	ResolutionStamp       FactStamp
	SuccessorID           SubjectID
	SupersessionRationale string
	OpenedObservation     Observation
	ResolutionObservation Observation
	HeadObservation       Observation
}

// Exploration is one immutable exploration identity.
type Exploration struct {
	Record          RecordVersion
	Label           string
	Purpose         string
	HeadObservation Observation
}

// Checkpoint is the current checkpoint for one exploration.
type Checkpoint struct {
	Record             RecordVersion
	ExplorationID      SubjectID
	CurrentFraming     string
	Conclusions        string
	UnresolvedQuestion string
	NextAction         string
	Items              []CheckpointItem
	HeadObservation    Observation
}

// Finding is one effective finding with an optional retraction marker.
type Finding struct {
	Record             RecordVersion
	Content            FindingContent
	ContentStamp       FactStamp
	State              FindingState
	RetractionReason   string
	ContentObservation Observation
	HeadObservation    Observation
}

// Handoff is one immutable context-transfer record.
type Handoff struct {
	Record            RecordVersion
	Focus             *SubjectRef
	Purpose           string
	Situation         string
	NextActions       string
	QuestionsAndRisks string
	SuggestedSkills   []string
	HeadObservation   Observation
}

// ScratchpadParticipant is one canonical participant introduction.
type ScratchpadParticipant struct {
	Stamp         FactStamp
	ParticipantID SubjectID
	Name          string
	Focus         *SubjectRef
	Observation   Observation
}

// ScratchpadMessage is one ordered visible message.
type ScratchpadMessage struct {
	Stamp         FactStamp
	ParticipantID SubjectID
	Text          string
	Observation   Observation
}

// ScratchpadClaim is one visible advisory claim after renewal folding.
type ScratchpadClaim struct {
	ClaimID         SubjectID
	ParticipantID   SubjectID
	Resource        string
	ExpiresAtMillis int64
	Root            FactStamp
	Head            FactStamp
	HeadObservation Observation
}

// Scratchpad is the current visible coordination state of one scratchpad.
type Scratchpad struct {
	Record            RecordVersion
	Focus             *SubjectRef
	Label             string
	State             ScratchpadState
	ClosedBy          SubjectID
	CloseReason       string
	OpenedObservation Observation
	HeadObservation   Observation
	Participants      []ScratchpadParticipant
	Messages          []ScratchpadMessage
	Claims            []ScratchpadClaim
}

// ExternalReferenceAttachment is one active exact-target edge.
type ExternalReferenceAttachment struct {
	Target      SubjectRef
	Stamp       FactStamp
	Observation Observation
}

// ExternalReference is one opaque locator and its current active edges.
type ExternalReference struct {
	Record                RecordVersion
	Locator               string
	RegisteredObservation Observation
	HeadObservation       Observation
	Attachments           []ExternalReferenceAttachment
}

// VerificationEvidence is one immutable verification record.
type VerificationEvidence struct {
	Record          RecordVersion
	Target          SubjectRef
	Check           string
	Method          string
	Outcome         VerificationOutcome
	Detail          string
	HeadObservation Observation
}

// ProjectIdentityProjection contains the one current project identity.
type ProjectIdentityProjection struct {
	Identity ProjectIdentity
}

// EffectiveJournalProjection orders entries by recording root, newest first.
type EffectiveJournalProjection struct {
	Entries []JournalEntry
}

// LatestWrapsProjection contains one winner per exact optional focus, ordered
// by winning stamp newest first. A nil focus is structurally project-level;
// Scope is descriptive content and never a grouping or ownership key.
type LatestWrapsProjection struct {
	Wraps []Wrap
}

// ActiveSparksProjection orders active sparks by capture root, newest first.
type ActiveSparksProjection struct {
	Sparks []Spark
}

// CurrentIdeasProjection orders current ideas by semantic head, newest first.
type CurrentIdeasProjection struct {
	Ideas []Idea
}

// CurrentDecisionsProjection orders decisions by semantic head, newest first.
type CurrentDecisionsProjection struct {
	Decisions []Decision
}

// ExplorationsProjection orders canonical exploration roots newest first.
type ExplorationsProjection struct {
	Explorations []Exploration
}

// LatestCheckpointsProjection contains one winner per exploration and orders
// winners by their root stamps, newest first.
type LatestCheckpointsProjection struct {
	Checkpoints []Checkpoint
}

// CurrentFindingsProjection orders findings by semantic head, newest first.
type CurrentFindingsProjection struct {
	Findings []Finding
}

// LatestHandoffsProjection contains one winner per exact optional focus and
// orders winners by root stamp, newest first.
type LatestHandoffsProjection struct {
	Handoffs []Handoff
}

// ScratchpadsProjection orders scratchpads by opening root, newest first.
// Participants sort by ID, messages by stamp oldest first, and visible claims
// by resource then claim ID.
type ScratchpadsProjection struct {
	Scratchpads []Scratchpad
}

// ExternalReferencesProjection orders registrations newest first. Each value
// contains all active edges ordered by target kind then target ID.
type ExternalReferencesProjection struct {
	References []ExternalReference
}

// VerificationEvidenceProjection orders immutable records by root newest first
// and never collapses equal checks or outcomes.
type VerificationEvidenceProjection struct {
	Evidence []VerificationEvidence
}

// Snapshot is the complete deterministic read-time continuity fold.
type Snapshot struct {
	AtMillis             int64
	Project              ProjectIdentityProjection
	EffectiveJournal     EffectiveJournalProjection
	LatestWraps          LatestWrapsProjection
	ActiveSparks         ActiveSparksProjection
	CurrentIdeas         CurrentIdeasProjection
	CurrentDecisions     CurrentDecisionsProjection
	Explorations         ExplorationsProjection
	LatestCheckpoints    LatestCheckpointsProjection
	CurrentFindings      CurrentFindingsProjection
	LatestHandoffs       LatestHandoffsProjection
	Scratchpads          ScratchpadsProjection
	ExternalReferences   ExternalReferencesProjection
	VerificationEvidence VerificationEvidenceProjection
}

// ContextSelection distinguishes absence from bounded display truncation.
type ContextSelection struct {
	AvailableCount int
	ShownCount     int
	Truncated      bool
}

// ContextJournalLayer is one bounded journal layer. Selection counts describe
// the layer after semantic filtering and deduplication but before and after its
// fixed cap.
type ContextJournalLayer struct {
	Selection ContextSelection
	Entries   []JournalEntry
}

// ContextWrapLayer is the exact-focus wrap followed by the nil-focus project
// wrap, with duplicates removed. Scope never manufactures a focus relation.
type ContextWrapLayer struct {
	Selection ContextSelection
	Wraps     []Wrap
}

// ContextSparkLayer contains selected active sparks.
type ContextSparkLayer struct {
	Selection ContextSelection
	Sparks    []Spark
}

// ContextIdeaLayer contains selected active ideas only.
type ContextIdeaLayer struct {
	Selection ContextSelection
	Ideas     []Idea
}

// ContextDecisionLayer contains selected decisions.
type ContextDecisionLayer struct {
	Selection ContextSelection
	Decisions []Decision
}

// ContextCheckpointLayer contains the one relevant latest checkpoint.
type ContextCheckpointLayer struct {
	Selection   ContextSelection
	Checkpoints []Checkpoint
}

// ContextFindingLayer contains selected findings.
type ContextFindingLayer struct {
	Selection ContextSelection
	Findings  []Finding
}

// ContextHandoffLayer contains the latest exact-focus handoff.
type ContextHandoffLayer struct {
	Selection ContextSelection
	Handoffs  []Handoff
}

// ContextExternalReference contains one selected locator and only active edges
// whose targets are present in the capped primary context. A directly focused
// reference remains visible even when no edge target is selected.
type ContextExternalReference struct {
	ProjectID             ProjectID
	ReferenceID           SubjectID
	Registration          FactStamp
	Locator               string
	RegisteredObservation Observation
	MatchingAttachments   []ExternalReferenceAttachment
}

// ContextExternalReferenceLayer contains selected one-hop opaque references.
// Each reference inherits the best selector rank among its matching targets;
// a directly focused reference forms a strict leading subrank even without a
// matching edge, so the explicit focus cannot be removed by this layer's cap.
type ContextExternalReferenceLayer struct {
	Selection  ContextSelection
	References []ContextExternalReference
}

// ContextVerificationEvidenceLayer contains selected direct evidence ranked by
// its target's selector rank.
type ContextVerificationEvidenceLayer struct {
	Selection ContextSelection
	Evidence  []VerificationEvidence
}

// ContextDigest is the fixed-layer context derived from one project snapshot.
// Layers and one-hop records preserve selector precedence and otherwise retain
// snapshot order within each selector rank. A directly focused external
// reference is the sole exception: it leads inherited focus-rank references.
type ContextDigest struct {
	Project              ProjectIdentityProjection
	Focus                *SubjectRef
	Scope                string
	Branch               string
	AtMillis             int64
	FocusedJournal       ContextJournalLayer
	ProjectJournal       ContextJournalLayer
	Wraps                ContextWrapLayer
	Sparks               ContextSparkLayer
	Ideas                ContextIdeaLayer
	Decisions            ContextDecisionLayer
	Checkpoints          ContextCheckpointLayer
	Findings             ContextFindingLayer
	Handoffs             ContextHandoffLayer
	ExternalReferences   ContextExternalReferenceLayer
	VerificationEvidence ContextVerificationEvidenceLayer
}
