package continuity

import (
	"reflect"
	"strings"
	"testing"
)

func TestContinuityCatalogIsExact(t *testing.T) {
	t.Parallel()

	want := []RecordSemantics{
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

	got := Catalog()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Catalog() = %#v, want %#v", got, want)
	}
	if !reflect.DeepEqual(Catalog(), want) {
		t.Fatal("Catalog() changed between reads")
	}
}

func TestContinuityCatalogReturnsFreshValues(t *testing.T) {
	t.Parallel()

	first := Catalog()
	first[0].Kind = "mutated"
	first = append(first, RecordSemantics{Kind: "invented"})

	second := Catalog()
	if second[0].Kind != RecordProjectIdentity {
		t.Fatalf("caller mutation changed catalog: first kind = %q", second[0].Kind)
	}
	if len(second) != 14 {
		t.Fatalf("caller append changed catalog length: got %d, want 14", len(second))
	}
}

func TestContinuityCatalogWireValuesArePinned(t *testing.T) {
	t.Parallel()

	want := []string{
		"project-identity|durable|mint-once|forever|own-identity|current-identity|project",
		"journal-entry|durable|append-only|forever|may-attach-opaque|timeline|recent-journal",
		"wrap|durable|append-only|forever|may-attach-opaque|latest-wrap|wrap",
		"spark|durable|append-with-disposition|forever|may-attach-opaque|active-spark|active",
		"idea|durable|append-with-revision-disposition-promotion|forever|may-attach-opaque|current-idea|active",
		"decision|durable|append-with-supersession|forever|may-attach-opaque|current-decision|decision",
		"exploration|durable|mint-once|forever|may-attach-opaque|exploration-identity|excluded",
		"checkpoint|durable|append-only|forever|may-attach-opaque|latest-checkpoint|checkpoint",
		"finding|durable|append-with-correction-retraction|forever|may-attach-opaque|current-finding|finding",
		"handoff|durable|append-only|forever|may-attach-opaque|latest-handoff|latest-handoff",
		"scratchpad|ephemeral|advisory-facts|until-sync-safe-point|none|coordination-state|excluded",
		"external-reference|durable|append-only|forever|opaque-leaf|attached-leaf|attached-only",
		"verification-evidence|durable|append-only|forever|evidence-leaf|evidence-ledger|attached-only",
		"derived-context|derived|read-time-fold|recomputed|include-attached|context-digest|output",
	}

	got := make([]string, 0, len(want))
	for _, semantics := range Catalog() {
		got = append(got, strings.Join([]string{
			string(semantics.Kind),
			string(semantics.Permanence),
			string(semantics.Mutation),
			string(semantics.Retention),
			string(semantics.References),
			string(semantics.Projection),
			string(semantics.Context),
		}, "|"))
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("catalog wire values = %v, want %v", got, want)
	}
}

func TestContinuitySemanticsLookupIsClosed(t *testing.T) {
	t.Parallel()

	for _, want := range Catalog() {
		got, ok := SemanticsFor(want.Kind)
		if !ok {
			t.Errorf("SemanticsFor(%q) was not found", want.Kind)
			continue
		}
		if got != want {
			t.Errorf("SemanticsFor(%q) = %#v, want %#v", want.Kind, got, want)
		}
	}

	got, ok := SemanticsFor("unknown")
	if ok {
		t.Fatalf("SemanticsFor(unknown) = %#v, true; want zero value, false", got)
	}
	if got != (RecordSemantics{}) {
		t.Fatalf("SemanticsFor(unknown) = %#v, want zero value", got)
	}
}

func TestContinuityCatalogValuesAreUniqueAndNonEmpty(t *testing.T) {
	t.Parallel()

	seenKinds := make(map[RecordKind]struct{})
	for _, semantics := range Catalog() {
		values := []string{
			string(semantics.Kind),
			string(semantics.Permanence),
			string(semantics.Mutation),
			string(semantics.Retention),
			string(semantics.References),
			string(semantics.Projection),
			string(semantics.Context),
		}
		for field, value := range values {
			if value == "" {
				t.Errorf("Catalog()[%q] field %d is empty", semantics.Kind, field)
			}
		}
		if _, exists := seenKinds[semantics.Kind]; exists {
			t.Errorf("duplicate record kind %q", semantics.Kind)
		}
		seenKinds[semantics.Kind] = struct{}{}
	}
}
