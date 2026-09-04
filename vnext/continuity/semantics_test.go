package continuity

import (
	"reflect"
	"strings"
	"testing"
)

func TestContinuityCatalogIsExact(t *testing.T) {
	t.Parallel()

	want := []RecordSemantics{
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
		{Kind: RecordExternalReference, Permanence: PermanenceLedger, Mutation: MutationAppendWithAttachmentChanges, Retention: RetentionForever, References: ReferencesOpaqueLeaf, Projection: ProjectionReferenceAttachments, Context: ContextAttachedOnly},
		{Kind: RecordVerificationEvidence, Permanence: PermanenceLedger, Mutation: MutationAppendOnly, Retention: RetentionForever, References: ReferencesEvidenceLeaf, Projection: ProjectionEvidenceLedger, Context: ContextAttachedOnly},
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
	if len(second) != 13 {
		t.Fatalf("caller append changed catalog length: got %d, want 13", len(second))
	}
}

func TestContinuityCatalogWireValuesArePinned(t *testing.T) {
	t.Parallel()

	want := []string{
		"project-identity|ledger|mint-once-with-label-revisions|forever|may-attach-opaque|current-identity|project",
		"journal-entry|ledger|append-with-corrections|forever|may-attach-opaque|effective-timeline|focus-then-project-journal",
		"wrap|ledger|append-only|forever|may-attach-opaque|latest-wrap|focus-and-project-wrap",
		"spark|notebook|append-with-disposition|forever|may-attach-opaque|active-spark|focus-active",
		"idea|notebook|append-with-revision-disposition-promotion|forever|may-attach-opaque|current-idea|focus-active",
		"decision|ledger|append-with-resolution-supersession|forever|may-attach-opaque|current-decision|focus-decision",
		"exploration|notebook|mint-once|forever|may-attach-opaque|exploration-identity|excluded",
		"checkpoint|notebook|append-only|forever|may-attach-opaque|latest-checkpoint|latest-checkpoint",
		"finding|ledger|append-with-correction-retraction|forever|may-attach-opaque|current-finding|focus-finding",
		"handoff|ledger|append-only|forever|may-attach-opaque|latest-handoff|latest-focus-handoff",
		"external-reference|ledger|append-with-attachment-changes|forever|opaque-leaf|reference-attachments|attached-only",
		"verification-evidence|ledger|append-only|forever|evidence-leaf|evidence-ledger|attached-only",
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
