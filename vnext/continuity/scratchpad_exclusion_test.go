package continuity

import (
	"reflect"
	"strings"
	"testing"
)

func TestCurrentContinuitySurfaceExcludesScratchpad(t *testing.T) {
	t.Parallel()

	for _, definition := range FactCatalog() {
		if strings.Contains(string(definition.Kind), "scratchpad") || strings.Contains(string(definition.Record), "scratchpad") {
			t.Fatalf("FactCatalog exposes deferred scratchpad fact %#v", definition)
		}
	}
	for _, semantics := range Catalog() {
		if strings.Contains(string(semantics.Kind), "scratchpad") || strings.Contains(string(semantics.Permanence), "scratchpad") {
			t.Fatalf("Catalog exposes deferred scratchpad semantics %#v", semantics)
		}
	}
	if _, ok := reflect.TypeOf(Snapshot{}).FieldByName("Scratchpads"); ok {
		t.Fatal("Snapshot exposes deferred Scratchpads projection")
	}
}
