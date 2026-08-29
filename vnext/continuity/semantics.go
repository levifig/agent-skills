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

// Permanence distinguishes durable, ephemeral, and computed records.
type Permanence string

const (
	PermanenceDurable   Permanence = "durable"
	PermanenceEphemeral Permanence = "ephemeral"
	PermanenceDerived   Permanence = "derived"
)

// Mutation states how later facts may affect a record family.
type Mutation string

const (
	MutationMintOnce                               Mutation = "mint-once"
	MutationAppendOnly                             Mutation = "append-only"
	MutationAppendWithDisposition                  Mutation = "append-with-disposition"
	MutationAppendWithRevisionDispositionPromotion Mutation = "append-with-revision-disposition-promotion"
	MutationAppendWithSupersession                 Mutation = "append-with-supersession"
	MutationAppendWithCorrectionRetraction         Mutation = "append-with-correction-retraction"
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
	ReferencesOwnIdentity     ReferenceBehavior = "own-identity"
	ReferencesMayAttachOpaque ReferenceBehavior = "may-attach-opaque"
	ReferencesNone            ReferenceBehavior = "none"
	ReferencesOpaqueLeaf      ReferenceBehavior = "opaque-leaf"
	ReferencesEvidenceLeaf    ReferenceBehavior = "evidence-leaf"
	ReferencesIncludeAttached ReferenceBehavior = "include-attached"
)

// Projection identifies the read-time fold for a record family.
type Projection string

const (
	ProjectionCurrentIdentity     Projection = "current-identity"
	ProjectionTimeline            Projection = "timeline"
	ProjectionLatestWrap          Projection = "latest-wrap"
	ProjectionActiveSpark         Projection = "active-spark"
	ProjectionCurrentIdea         Projection = "current-idea"
	ProjectionCurrentDecision     Projection = "current-decision"
	ProjectionExplorationIdentity Projection = "exploration-identity"
	ProjectionLatestCheckpoint    Projection = "latest-checkpoint"
	ProjectionCurrentFinding      Projection = "current-finding"
	ProjectionLatestHandoff       Projection = "latest-handoff"
	ProjectionCoordinationState   Projection = "coordination-state"
	ProjectionAttachedLeaf        Projection = "attached-leaf"
	ProjectionEvidenceLedger      Projection = "evidence-ledger"
	ProjectionContextDigest       Projection = "context-digest"
)

// ContextInclusion identifies a record family's fixed context layer.
type ContextInclusion string

const (
	ContextProject       ContextInclusion = "project"
	ContextRecentJournal ContextInclusion = "recent-journal"
	ContextWrap          ContextInclusion = "wrap"
	ContextActive        ContextInclusion = "active"
	ContextDecision      ContextInclusion = "decision"
	ContextExcluded      ContextInclusion = "excluded"
	ContextCheckpoint    ContextInclusion = "checkpoint"
	ContextFinding       ContextInclusion = "finding"
	ContextLatestHandoff ContextInclusion = "latest-handoff"
	ContextAttachedOnly  ContextInclusion = "attached-only"
	ContextOutput        ContextInclusion = "output"
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
		{Kind: RecordProjectIdentity, Permanence: PermanenceDurable, Mutation: MutationMintOnce, Retention: RetentionForever, References: ReferencesOwnIdentity, Projection: ProjectionCurrentIdentity, Context: ContextProject},
		{Kind: RecordJournalEntry, Permanence: PermanenceDurable, Mutation: MutationAppendOnly, Retention: RetentionForever, References: ReferencesMayAttachOpaque, Projection: ProjectionTimeline, Context: ContextRecentJournal},
		{Kind: RecordWrap, Permanence: PermanenceDurable, Mutation: MutationAppendOnly, Retention: RetentionForever, References: ReferencesMayAttachOpaque, Projection: ProjectionLatestWrap, Context: ContextWrap},
		{Kind: RecordSpark, Permanence: PermanenceDurable, Mutation: MutationAppendWithDisposition, Retention: RetentionForever, References: ReferencesMayAttachOpaque, Projection: ProjectionActiveSpark, Context: ContextActive},
		{Kind: RecordIdea, Permanence: PermanenceDurable, Mutation: MutationAppendWithRevisionDispositionPromotion, Retention: RetentionForever, References: ReferencesMayAttachOpaque, Projection: ProjectionCurrentIdea, Context: ContextActive},
		{Kind: RecordDecision, Permanence: PermanenceDurable, Mutation: MutationAppendWithSupersession, Retention: RetentionForever, References: ReferencesMayAttachOpaque, Projection: ProjectionCurrentDecision, Context: ContextDecision},
		{Kind: RecordExploration, Permanence: PermanenceDurable, Mutation: MutationMintOnce, Retention: RetentionForever, References: ReferencesMayAttachOpaque, Projection: ProjectionExplorationIdentity, Context: ContextExcluded},
		{Kind: RecordCheckpoint, Permanence: PermanenceDurable, Mutation: MutationAppendOnly, Retention: RetentionForever, References: ReferencesMayAttachOpaque, Projection: ProjectionLatestCheckpoint, Context: ContextCheckpoint},
		{Kind: RecordFinding, Permanence: PermanenceDurable, Mutation: MutationAppendWithCorrectionRetraction, Retention: RetentionForever, References: ReferencesMayAttachOpaque, Projection: ProjectionCurrentFinding, Context: ContextFinding},
		{Kind: RecordHandoff, Permanence: PermanenceDurable, Mutation: MutationAppendOnly, Retention: RetentionForever, References: ReferencesMayAttachOpaque, Projection: ProjectionLatestHandoff, Context: ContextLatestHandoff},
		{Kind: RecordScratchpad, Permanence: PermanenceEphemeral, Mutation: MutationAdvisoryFacts, Retention: RetentionUntilSyncSafePoint, References: ReferencesNone, Projection: ProjectionCoordinationState, Context: ContextExcluded},
		{Kind: RecordExternalReference, Permanence: PermanenceDurable, Mutation: MutationAppendOnly, Retention: RetentionForever, References: ReferencesOpaqueLeaf, Projection: ProjectionAttachedLeaf, Context: ContextAttachedOnly},
		{Kind: RecordVerificationEvidence, Permanence: PermanenceDurable, Mutation: MutationAppendOnly, Retention: RetentionForever, References: ReferencesEvidenceLeaf, Projection: ProjectionEvidenceLedger, Context: ContextAttachedOnly},
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
