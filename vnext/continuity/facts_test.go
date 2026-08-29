package continuity

import (
	"reflect"
	"testing"
)

func TestContinuityFactCatalogWireValuesArePinned(t *testing.T) {
	t.Parallel()

	want := []string{
		"project.registered|project-identity",
		"project.label-revised|project-identity",
		"journal.recorded|journal-entry",
		"journal.correction-recorded|journal-entry",
		"wrap.recorded|wrap",
		"spark.captured|spark",
		"spark.dismissed|spark",
		"spark.promoted-to-idea|spark",
		"idea.created|idea",
		"idea.revised|idea",
		"idea.resolved|idea",
		"idea.archived|idea",
		"idea.promoted-to-external-reference|idea",
		"decision.opened|decision",
		"decision.resolved|decision",
		"decision.superseded|decision",
		"exploration.started|exploration",
		"checkpoint.recorded|checkpoint",
		"finding.recorded|finding",
		"finding.corrected|finding",
		"finding.retracted|finding",
		"handoff.recorded|handoff",
		"scratchpad.opened|scratchpad",
		"scratchpad.participant-introduced|scratchpad",
		"scratchpad.message-recorded|scratchpad",
		"scratchpad.claim-recorded|scratchpad",
		"scratchpad.claim-released|scratchpad",
		"scratchpad.closed|scratchpad",
		"external-reference.registered|external-reference",
		"external-reference.attached|external-reference",
		"external-reference.detached|external-reference",
		"verification-evidence.recorded|verification-evidence",
	}

	got := make([]string, 0, len(want))
	for _, definition := range FactCatalog() {
		got = append(got, string(definition.Kind)+"|"+string(definition.Record))
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("fact catalog wire values = %v, want %v", got, want)
	}
}

func TestContinuityFactCatalogReturnsFreshValues(t *testing.T) {
	t.Parallel()

	first := FactCatalog()
	first[0].Kind = "mutated"
	first = append(first, FactDefinition{Kind: "invented", Record: RecordDerivedContext})

	second := FactCatalog()
	if second[0].Kind != FactProjectRegistered {
		t.Fatalf("caller mutation changed catalog: first kind = %q", second[0].Kind)
	}
	if len(second) != 32 {
		t.Fatalf("caller append changed catalog length: got %d, want 32", len(second))
	}
}

func TestContinuityFactDefinitionLookupIsClosed(t *testing.T) {
	t.Parallel()

	for _, want := range FactCatalog() {
		got, ok := DefinitionFor(want.Kind)
		if !ok {
			t.Errorf("DefinitionFor(%q) was not found", want.Kind)
			continue
		}
		if got != want {
			t.Errorf("DefinitionFor(%q) = %#v, want %#v", want.Kind, got, want)
		}
	}

	got, ok := DefinitionFor("unknown")
	if ok {
		t.Fatalf("DefinitionFor(unknown) = %#v, true; want zero value, false", got)
	}
	if got != (FactDefinition{}) {
		t.Fatalf("DefinitionFor(unknown) = %#v, want zero value", got)
	}
}

func TestContinuityFactCatalogIsUniqueAndNeverPersistsDerivedContext(t *testing.T) {
	t.Parallel()

	seen := make(map[FactKind]struct{})
	for _, definition := range FactCatalog() {
		if definition.Kind == "" || definition.Record == "" {
			t.Errorf("empty fact definition: %#v", definition)
		}
		if _, exists := seen[definition.Kind]; exists {
			t.Errorf("duplicate fact kind %q", definition.Kind)
		}
		seen[definition.Kind] = struct{}{}
		if definition.Record == RecordDerivedContext {
			t.Errorf("derived context has persisted fact %q", definition.Kind)
		}
		if _, ok := SemanticsFor(definition.Record); !ok {
			t.Errorf("fact %q maps to unknown record kind %q", definition.Kind, definition.Record)
		}
	}
}
