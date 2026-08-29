package kernel

import "testing"

func TestKernelIdentityStartsIndependentSchemaLine(t *testing.T) {
	t.Parallel()

	identity := CurrentIdentity()
	if identity.Product != "loaf" {
		t.Fatalf("Product = %q, want loaf", identity.Product)
	}
	if identity.Generation != "vnext" {
		t.Fatalf("Generation = %q, want vnext", identity.Generation)
	}
	if identity.Schema.Line != "vnext" {
		t.Fatalf("Schema.Line = %q, want vnext", identity.Schema.Line)
	}
	if identity.Schema.Version != 1 {
		t.Fatalf("Schema.Version = %d, want 1", identity.Schema.Version)
	}
}

func TestKernelOwnershipMatrixHasOneAuthorityPerResponsibility(t *testing.T) {
	t.Parallel()

	want := []Ownership{
		{Authority: AuthorityLoaf, Responsibilities: []string{
			"flow-ceremonies",
			"skills",
			"templates",
			"profiles",
			"project-identity",
			"private-continuity",
			"derived-context",
			"private-sync",
		}},
		{Authority: AuthorityTracker, Responsibilities: []string{
			"work-identity",
			"work-definition",
			"definition-of-done",
			"workflow-state",
			"hierarchy",
			"assignment",
			"collaboration",
		}},
		{Authority: AuthorityGit, Responsibilities: []string{
			"code",
			"promoted-artifacts",
		}},
		{Authority: AuthorityHarness, Responsibilities: []string{
			"execution",
			"model-selection",
			"tool-boundaries",
			"service-connections",
			"service-credentials",
		}},
	}

	got := OwnershipMatrix()
	if len(got) != len(want) {
		t.Fatalf("OwnershipMatrix() returned %d authorities, want %d", len(got), len(want))
	}

	seen := make(map[string]Authority)
	for index, expected := range want {
		actual := got[index]
		if actual.Authority != expected.Authority {
			t.Errorf("OwnershipMatrix()[%d].Authority = %q, want %q", index, actual.Authority, expected.Authority)
		}
		if len(actual.Responsibilities) != len(expected.Responsibilities) {
			t.Fatalf("OwnershipMatrix()[%d] has %d responsibilities, want %d", index, len(actual.Responsibilities), len(expected.Responsibilities))
		}
		for responsibilityIndex, responsibility := range expected.Responsibilities {
			if actual.Responsibilities[responsibilityIndex] != responsibility {
				t.Errorf("OwnershipMatrix()[%d].Responsibilities[%d] = %q, want %q", index, responsibilityIndex, actual.Responsibilities[responsibilityIndex], responsibility)
			}
			if authority, exists := seen[responsibility]; exists {
				t.Errorf("responsibility %q belongs to both %q and %q", responsibility, authority, actual.Authority)
			}
			seen[responsibility] = actual.Authority
		}
	}
}

func TestKernelOwnershipMatrixReturnsIndependentValues(t *testing.T) {
	t.Parallel()

	first := OwnershipMatrix()
	first[0].Authority = AuthorityTracker
	first[0].Responsibilities[0] = "mutated"

	second := OwnershipMatrix()
	if second[0].Authority != AuthorityLoaf {
		t.Errorf("OwnershipMatrix()[0].Authority = %q after caller mutation, want %q", second[0].Authority, AuthorityLoaf)
	}
	if second[0].Responsibilities[0] != "flow-ceremonies" {
		t.Errorf("OwnershipMatrix()[0].Responsibilities[0] = %q after caller mutation, want flow-ceremonies", second[0].Responsibilities[0])
	}
}
