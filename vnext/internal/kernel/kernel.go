// Package kernel defines the dependency-free identity and ownership contract
// shared by every vNext command.
package kernel

const (
	productName    = "loaf"
	generationName = "vnext"
	schemaLine     = "vnext"
	schemaVersion  = 1
)

// Authority names the system that is canonical for a responsibility.
type Authority string

const (
	// AuthorityLoaf owns private continuity and the workflow definition.
	AuthorityLoaf Authority = "loaf"
	// AuthorityTracker owns shared work and collaboration state.
	AuthorityTracker Authority = "tracker"
	// AuthorityGit owns code and deliberately promoted artifacts.
	AuthorityGit Authority = "git"
	// AuthorityHarness owns execution and external-service connections.
	AuthorityHarness Authority = "harness"
)

// Ownership binds responsibilities to their one canonical authority.
type Ownership struct {
	Authority        Authority
	Responsibilities []string
}

var ownershipMatrix = []Ownership{
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

// SchemaIdentity names an independent schema line and its current version.
type SchemaIdentity struct {
	Line    string
	Version int
}

// Identity describes the vNext kernel without consulting legacy runtime state.
type Identity struct {
	Product    string
	Generation string
	Schema     SchemaIdentity
}

// CurrentIdentity returns the immutable identity of this kernel build.
func CurrentIdentity() Identity {
	return Identity{
		Product:    productName,
		Generation: generationName,
		Schema: SchemaIdentity{
			Line:    schemaLine,
			Version: schemaVersion,
		},
	}
}

// OwnershipMatrix returns a defensive copy of the vNext ownership contract.
func OwnershipMatrix() []Ownership {
	matrix := make([]Ownership, len(ownershipMatrix))
	for index, ownership := range ownershipMatrix {
		matrix[index] = Ownership{
			Authority:        ownership.Authority,
			Responsibilities: append([]string(nil), ownership.Responsibilities...),
		}
	}
	return matrix
}
