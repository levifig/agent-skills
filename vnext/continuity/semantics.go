// Package continuity defines Loaf's closed private-continuity domain.
package continuity

// RecordKind identifies one continuity record family.
type RecordKind string

const (
	RecordProjectIdentity      RecordKind = "project-identity"
	RecordJournalEntry         RecordKind = "journal-entry"
	RecordWrap                 RecordKind = "wrap"
	RecordSpark                RecordKind = "spark"
	RecordIdea                 RecordKind = "idea"
	RecordDecision             RecordKind = "decision"
	RecordExploration          RecordKind = "exploration"
	RecordCheckpoint           RecordKind = "checkpoint"
	RecordFinding              RecordKind = "finding"
	RecordHandoff              RecordKind = "handoff"
	RecordScratchpad           RecordKind = "scratchpad"
	RecordExternalReference    RecordKind = "external-reference"
	RecordVerificationEvidence RecordKind = "verification-evidence"
	RecordDerivedContext       RecordKind = "derived-context"
)

// Permanence distinguishes the ledger, notebook, scratchpad, and computed
// continuity families.
type Permanence string

const (
	PermanenceLedger     Permanence = "ledger"
	PermanenceNotebook   Permanence = "notebook"
	PermanenceScratchpad Permanence = "scratchpad"
	PermanenceDerived    Permanence = "derived"
)

// Mutation states how later facts may affect a record family.
type Mutation string

const (
	MutationMintOnce                               Mutation = "mint-once"
	MutationMintOnceWithLabelRevisions             Mutation = "mint-once-with-label-revisions"
	MutationAppendOnly                             Mutation = "append-only"
	MutationAppendWithCorrections                  Mutation = "append-with-corrections"
	MutationAppendWithDisposition                  Mutation = "append-with-disposition"
	MutationAppendWithRevisionDispositionPromotion Mutation = "append-with-revision-disposition-promotion"
	MutationAppendWithResolutionSupersession       Mutation = "append-with-resolution-supersession"
	MutationAppendWithCorrectionRetraction         Mutation = "append-with-correction-retraction"
	MutationAppendWithAttachmentChanges            Mutation = "append-with-attachment-changes"
	MutationAdvisoryFacts                          Mutation = "advisory-facts"
	MutationReadTimeFold                           Mutation = "read-time-fold"
)

// Retention states when physical records may cease to be retained.
type Retention string

const (
	RetentionForever            Retention = "forever"
	RetentionUntilSyncSafePoint Retention = "until-sync-safe-point"
	RetentionRecomputed         Retention = "recomputed"
)

// ReferenceBehavior states how a record participates in opaque references.
type ReferenceBehavior string

const (
	ReferencesMayAttachOpaque    ReferenceBehavior = "may-attach-opaque"
	ReferencesEphemeralOpaqueIDs ReferenceBehavior = "ephemeral-opaque-ids"
	ReferencesOpaqueLeaf         ReferenceBehavior = "opaque-leaf"
	ReferencesEvidenceLeaf       ReferenceBehavior = "evidence-leaf"
	ReferencesIncludeAttached    ReferenceBehavior = "include-attached"
)

// Projection identifies the read-time fold for a record family.
type Projection string

const (
	ProjectionCurrentIdentity      Projection = "current-identity"
	ProjectionEffectiveTimeline    Projection = "effective-timeline"
	ProjectionLatestWrap           Projection = "latest-wrap"
	ProjectionActiveSpark          Projection = "active-spark"
	ProjectionCurrentIdea          Projection = "current-idea"
	ProjectionCurrentDecision      Projection = "current-decision"
	ProjectionExplorationIdentity  Projection = "exploration-identity"
	ProjectionLatestCheckpoint     Projection = "latest-checkpoint"
	ProjectionCurrentFinding       Projection = "current-finding"
	ProjectionLatestHandoff        Projection = "latest-handoff"
	ProjectionCoordinationState    Projection = "coordination-state"
	ProjectionReferenceAttachments Projection = "reference-attachments"
	ProjectionEvidenceLedger       Projection = "evidence-ledger"
	ProjectionContextDigest        Projection = "context-digest"
)

// ContextInclusion identifies a record family's fixed context layer.
type ContextInclusion string

const (
	ContextProject                 ContextInclusion = "project"
	ContextFocusThenProjectJournal ContextInclusion = "focus-then-project-journal"
	ContextFocusAndProjectWrap     ContextInclusion = "focus-and-project-wrap"
	ContextFocusActive             ContextInclusion = "focus-active"
	ContextFocusDecision           ContextInclusion = "focus-decision"
	ContextExcluded                ContextInclusion = "excluded"
	ContextLatestCheckpoint        ContextInclusion = "latest-checkpoint"
	ContextFocusFinding            ContextInclusion = "focus-finding"
	ContextLatestFocusHandoff      ContextInclusion = "latest-focus-handoff"
	ContextAttachedOnly            ContextInclusion = "attached-only"
	ContextOutput                  ContextInclusion = "output"
)

// RecordSemantics is the immutable semantic tuple for one record family.
type RecordSemantics struct {
	Kind       RecordKind
	Permanence Permanence
	Mutation   Mutation
	Retention  Retention
	References ReferenceBehavior
	Projection Projection
	Context    ContextInclusion
}

// Catalog returns the closed continuity catalog in canonical order.
func Catalog() []RecordSemantics {
	return []RecordSemantics{
		{Kind: RecordProjectIdentity, Permanence: PermanenceLedger, Mutation: MutationMintOnceWithLabelRevisions, Retention: RetentionForever, References: ReferencesMayAttachOpaque, Projection: ProjectionCurrentIdentity, Context: ContextProject},
		{Kind: RecordJournalEntry, Permanence: PermanenceLedger, Mutation: MutationAppendWithCorrections, Retention: RetentionForever, References: ReferencesMayAttachOpaque, Projection: ProjectionEffectiveTimeline, Context: ContextFocusThenProjectJournal},
		{Kind: RecordWrap, Permanence: PermanenceLedger, Mutation: MutationAppendOnly, Retention: RetentionForever, References: ReferencesMayAttachOpaque, Projection: ProjectionLatestWrap, Context: ContextFocusAndProjectWrap},
		{Kind: RecordSpark, Permanence: PermanenceNotebook, Mutation: MutationAppendWithDisposition, Retention: RetentionForever, References: ReferencesMayAttachOpaque, Projection: ProjectionActiveSpark, Context: ContextFocusActive},
		{Kind: RecordIdea, Permanence: PermanenceNotebook, Mutation: MutationAppendWithRevisionDispositionPromotion, Retention: RetentionForever, References: ReferencesMayAttachOpaque, Projection: ProjectionCurrentIdea, Context: ContextFocusActive},
		{Kind: RecordDecision, Permanence: PermanenceLedger, Mutation: MutationAppendWithResolutionSupersession, Retention: RetentionForever, References: ReferencesMayAttachOpaque, Projection: ProjectionCurrentDecision, Context: ContextFocusDecision},
		{Kind: RecordExploration, Permanence: PermanenceNotebook, Mutation: MutationMintOnce, Retention: RetentionForever, References: ReferencesMayAttachOpaque, Projection: ProjectionExplorationIdentity, Context: ContextExcluded},
		{Kind: RecordCheckpoint, Permanence: PermanenceNotebook, Mutation: MutationAppendOnly, Retention: RetentionForever, References: ReferencesMayAttachOpaque, Projection: ProjectionLatestCheckpoint, Context: ContextLatestCheckpoint},
		{Kind: RecordFinding, Permanence: PermanenceLedger, Mutation: MutationAppendWithCorrectionRetraction, Retention: RetentionForever, References: ReferencesMayAttachOpaque, Projection: ProjectionCurrentFinding, Context: ContextFocusFinding},
		{Kind: RecordHandoff, Permanence: PermanenceLedger, Mutation: MutationAppendOnly, Retention: RetentionForever, References: ReferencesMayAttachOpaque, Projection: ProjectionLatestHandoff, Context: ContextLatestFocusHandoff},
		{Kind: RecordScratchpad, Permanence: PermanenceScratchpad, Mutation: MutationAdvisoryFacts, Retention: RetentionUntilSyncSafePoint, References: ReferencesEphemeralOpaqueIDs, Projection: ProjectionCoordinationState, Context: ContextExcluded},
		{Kind: RecordExternalReference, Permanence: PermanenceLedger, Mutation: MutationAppendWithAttachmentChanges, Retention: RetentionForever, References: ReferencesOpaqueLeaf, Projection: ProjectionReferenceAttachments, Context: ContextAttachedOnly},
		{Kind: RecordVerificationEvidence, Permanence: PermanenceLedger, Mutation: MutationAppendOnly, Retention: RetentionForever, References: ReferencesEvidenceLeaf, Projection: ProjectionEvidenceLedger, Context: ContextAttachedOnly},
		{Kind: RecordDerivedContext, Permanence: PermanenceDerived, Mutation: MutationReadTimeFold, Retention: RetentionRecomputed, References: ReferencesIncludeAttached, Projection: ProjectionContextDigest, Context: ContextOutput},
	}
}

// SemanticsFor returns the semantic tuple for kind.
func SemanticsFor(kind RecordKind) (RecordSemantics, bool) {
	for _, semantics := range Catalog() {
		if semantics.Kind == kind {
			return semantics, true
		}
	}
	return RecordSemantics{}, false
}
