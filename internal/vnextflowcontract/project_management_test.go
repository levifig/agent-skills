package vnextflowcontract

import (
	"fmt"
	"io/fs"
	"os"
	"strings"
	"testing"
	"testing/fstest"
)

const (
	projectManagementContractPath = "skills/project-management/contract.json"
	linearCapabilitiesPath        = "skills/linear/capabilities.json"
	projectManagerContractPath    = "agents/project-manager.contract.json"
)

type projectManagementContract struct {
	ID               string                   `json:"id"`
	Authority        trackerAuthorityContract `json:"authority"`
	Routing          trackerRoutingContract   `json:"routing"`
	MutationPolicy   trackerMutationPolicy    `json:"mutation_policy"`
	Operations       []trackerOperation       `json:"operations"`
	Outcomes         []string                 `json:"outcomes"`
	Fidelities       []string                 `json:"fidelities"`
	ResultFields     []string                 `json:"result_fields"`
	SemanticChannels []string                 `json:"semantic_channels"`
	CommentPolicy    trackerCommentPolicy     `json:"comment_policy"`
}

type trackerAuthorityContract struct {
	SharedWork         string `json:"shared_work"`
	ServiceConnections string `json:"service_connections"`
	LocalWorkRecord    string `json:"local_work_record"`
	TrackerProxy       string `json:"tracker_proxy"`
	TrackerSync        string `json:"tracker_sync"`
}

type trackerRoutingContract struct {
	ConnectionDiscovery string `json:"connection_discovery"`
	DestinationScope    string `json:"destination_scope"`
	ProviderSelection   string `json:"provider_selection"`
}

type trackerMutationPolicy struct {
	ReadBeforeWrite       string `json:"read_before_write"`
	AuthoritativeReadback string `json:"authoritative_readback"`
	AmbiguousCreateRetry  string `json:"ambiguous_create_retry"`
	AmbiguousCommentRetry string `json:"ambiguous_comment_retry"`
	PartialOutcomes       string `json:"partial_outcomes"`
	IndeterminateOutcomes string `json:"indeterminate_outcomes"`
}

type trackerOperation struct {
	ID             string `json:"id"`
	Mode           string `json:"mode"`
	Precondition   string `json:"precondition"`
	Verification   string `json:"verification"`
	AmbiguousRetry string `json:"ambiguous_retry"`
}

type trackerCommentPolicy struct {
	Purpose                   string `json:"purpose"`
	SubstituteForDefinition   string `json:"substitute_for_definition"`
	SubstituteForRelationship string `json:"substitute_for_relationship"`
	SubstituteForStatus       string `json:"substitute_for_status"`
}

type providerCapabilities struct {
	Schema                     string                     `json:"schema"`
	Provider                   string                     `json:"provider"`
	Contract                   string                     `json:"contract"`
	Connection                 string                     `json:"connection"`
	RuntimeCapabilityDiscovery string                     `json:"runtime_capability_discovery"`
	Operations                 []providerOperationMapping `json:"operations"`
}

type providerOperationMapping struct {
	ID              string   `json:"id"`
	NativeSemantic  string   `json:"native_semantic"`
	Availability    string   `json:"availability"`
	MaximumFidelity string   `json:"maximum_fidelity"`
	Requires        []string `json:"requires"`
}

type projectManagerProfileContract struct {
	ID                    string   `json:"id"`
	TrackerContract       string   `json:"tracker_contract"`
	Execution             string   `json:"execution"`
	Equivalence           string   `json:"equivalence"`
	ProviderSelection     string   `json:"provider_selection"`
	AllowedCapabilities   []string `json:"allowed_capabilities"`
	ForbiddenCapabilities []string `json:"forbidden_capabilities"`
	Fallback              string   `json:"fallback"`
}

func TestTrackerSkillProjectManagementContract(t *testing.T) {
	t.Parallel()

	content := os.DirFS("../../vnext/content")
	assertNoContractFindings(t, validateProjectManagementContract(content))
}

func TestTrackerSkillContractRejectsProviderOverclaim(t *testing.T) {
	t.Parallel()

	fixture := validProjectManagementFixture()
	capabilities := fixture[linearCapabilitiesPath]
	capabilities.Data = []byte(strings.Replace(string(capabilities.Data), `"availability": "runtime"`, `"availability": "universal"`, 1))
	fixture[linearCapabilitiesPath] = capabilities

	findings := validateProjectManagementContract(fixture)
	assertContractFinding(t, findings, "provider.capability", "availability")
}

func TestTrackerSkillContractRejectsExpandedProjectManagerAuthority(t *testing.T) {
	t.Parallel()

	fixture := validProjectManagementFixture()
	profile := fixture[projectManagerContractPath]
	profile.Data = []byte(strings.Replace(string(profile.Data), `["harness-connector"]`, `["harness-connector", "shell"]`, 1))
	fixture[projectManagerContractPath] = profile

	findings := validateProjectManagementContract(fixture)
	assertContractFinding(t, findings, "profile.authority", "allowed_capabilities")
}

func validateProjectManagementContract(content fs.FS) []finding {
	var findings []finding
	contract := projectManagementContract{}
	if err := decodeStrictJSON(content, projectManagementContractPath, &contract); err != nil {
		return []finding{{"tracker.contract", projectManagementContractPath, err.Error()}}
	}
	findings = append(findings, validateTrackerContract(contract)...)

	provider := providerCapabilities{}
	if err := decodeStrictJSON(content, linearCapabilitiesPath, &provider); err != nil {
		findings = append(findings, finding{"provider.contract", linearCapabilitiesPath, err.Error()})
	} else {
		findings = append(findings, validateProviderCapabilities(contract, provider)...)
	}

	profile := projectManagerProfileContract{}
	if err := decodeStrictJSON(content, projectManagerContractPath, &profile); err != nil {
		findings = append(findings, finding{"profile.contract", projectManagerContractPath, err.Error()})
	} else {
		findings = append(findings, validateProjectManagerProfile(profile)...)
	}

	for _, entrypoint := range []string{
		"skills/project-management/SKILL.md",
		"skills/linear/SKILL.md",
		"agents/project-manager.md",
	} {
		body, err := fs.ReadFile(content, entrypoint)
		if err != nil {
			findings = append(findings, finding{"tracker.entrypoint", entrypoint, err.Error()})
			continue
		}
		if !strings.Contains(string(body), trackerContract) {
			findings = append(findings, finding{"tracker.entrypoint", entrypoint, fmt.Sprintf("must name %s", trackerContract)})
		}
	}
	sortFindings(findings)
	return findings
}

func validateTrackerContract(contract projectManagementContract) []finding {
	wantAuthority := trackerAuthorityContract{
		SharedWork:         "tracker",
		ServiceConnections: "harness",
		LocalWorkRecord:    "forbidden",
		TrackerProxy:       "forbidden",
		TrackerSync:        "forbidden",
	}
	wantRouting := trackerRoutingContract{
		ConnectionDiscovery: "already-exposed-only",
		DestinationScope:    "required",
		ProviderSelection:   "runtime-capability-checked",
	}
	wantMutation := trackerMutationPolicy{
		ReadBeforeWrite:       "required",
		AuthoritativeReadback: "required",
		AmbiguousCreateRetry:  "forbidden",
		AmbiguousCommentRetry: "forbidden",
		PartialOutcomes:       "preserve",
		IndeterminateOutcomes: "preserve",
	}
	wantOperations := canonicalOperations()
	wantOutcomes := []string{"confirmed", "unchanged", "partial", "failed", "indeterminate"}
	wantFidelities := []string{"exact", "advisory", "manual", "unsupported"}
	wantResultFields := []string{"operation", "destination", "native_ref", "outcome", "fidelity", "observed_state", "verification_evidence"}
	wantChannels := []string{"definition", "hierarchy", "dependency", "status", "comment"}
	wantComment := trackerCommentPolicy{
		Purpose:                   "evidence-and-collaboration",
		SubstituteForDefinition:   "forbidden",
		SubstituteForRelationship: "forbidden",
		SubstituteForStatus:       "forbidden",
	}

	var findings []finding
	if contract.ID != trackerContract {
		findings = append(findings, finding{"tracker.identity", projectManagementContractPath, fmt.Sprintf("id = %q, want %q", contract.ID, trackerContract)})
	}
	if contract.Authority != wantAuthority {
		findings = append(findings, finding{"tracker.authority", projectManagementContractPath, fmt.Sprintf("authority = %+v, want %+v", contract.Authority, wantAuthority)})
	}
	if contract.Routing != wantRouting {
		findings = append(findings, finding{"tracker.routing", projectManagementContractPath, fmt.Sprintf("routing = %+v, want %+v", contract.Routing, wantRouting)})
	}
	if contract.MutationPolicy != wantMutation {
		findings = append(findings, finding{"tracker.mutation", projectManagementContractPath, fmt.Sprintf("mutation_policy = %+v, want %+v", contract.MutationPolicy, wantMutation)})
	}
	if !equalOperations(contract.Operations, wantOperations) {
		findings = append(findings, finding{"tracker.operations", projectManagementContractPath, "operations must match the closed project-management/v1 vocabulary and semantics"})
	}
	if !equalStrings(contract.Outcomes, wantOutcomes) {
		findings = append(findings, finding{"tracker.outcomes", projectManagementContractPath, fmt.Sprintf("outcomes = %v, want %v", contract.Outcomes, wantOutcomes)})
	}
	if !equalStrings(contract.Fidelities, wantFidelities) {
		findings = append(findings, finding{"tracker.fidelities", projectManagementContractPath, fmt.Sprintf("fidelities = %v, want %v", contract.Fidelities, wantFidelities)})
	}
	if !equalStrings(contract.ResultFields, wantResultFields) {
		findings = append(findings, finding{"tracker.result", projectManagementContractPath, fmt.Sprintf("result_fields = %v, want %v", contract.ResultFields, wantResultFields)})
	}
	if !equalStrings(contract.SemanticChannels, wantChannels) {
		findings = append(findings, finding{"tracker.semantics", projectManagementContractPath, fmt.Sprintf("semantic_channels = %v, want %v", contract.SemanticChannels, wantChannels)})
	}
	if contract.CommentPolicy != wantComment {
		findings = append(findings, finding{"tracker.comments", projectManagementContractPath, fmt.Sprintf("comment_policy = %+v, want %+v", contract.CommentPolicy, wantComment)})
	}
	return findings
}

func validateProviderCapabilities(contract projectManagementContract, provider providerCapabilities) []finding {
	var findings []finding
	if provider.Schema != "loaf-provider-capabilities/v1" || provider.Provider != "linear" || provider.Contract != trackerContract {
		findings = append(findings, finding{"provider.identity", linearCapabilitiesPath, "provider identity must bind Linear to project-management/v1"})
	}
	if provider.Connection != "harness-native" || provider.RuntimeCapabilityDiscovery != "required" {
		findings = append(findings, finding{"provider.connection", linearCapabilitiesPath, "connection must remain harness-native with runtime capability discovery"})
	}
	if len(provider.Operations) != len(contract.Operations) {
		findings = append(findings, finding{"provider.operations", linearCapabilitiesPath, fmt.Sprintf("operations has %d entries, want %d", len(provider.Operations), len(contract.Operations))})
	}
	fidelities := make(map[string]struct{}, len(contract.Fidelities))
	for _, fidelity := range contract.Fidelities {
		fidelities[fidelity] = struct{}{}
	}
	for index, mapping := range provider.Operations {
		if index >= len(contract.Operations) || mapping.ID != contract.Operations[index].ID {
			findings = append(findings, finding{"provider.operations", linearCapabilitiesPath, fmt.Sprintf("operation %d does not match the common contract order", index)})
		}
		if mapping.NativeSemantic == "" || len(mapping.Requires) == 0 {
			findings = append(findings, finding{"provider.mapping", linearCapabilitiesPath, fmt.Sprintf("operation %q needs a native semantic and capability requirements", mapping.ID)})
		}
		if mapping.Availability != "runtime" {
			findings = append(findings, finding{"provider.capability", linearCapabilitiesPath, fmt.Sprintf("operation %q availability = %q, want runtime", mapping.ID, mapping.Availability)})
		}
		if _, exists := fidelities[mapping.MaximumFidelity]; !exists || mapping.MaximumFidelity == "unsupported" {
			findings = append(findings, finding{"provider.fidelity", linearCapabilitiesPath, fmt.Sprintf("operation %q has invalid maximum_fidelity %q", mapping.ID, mapping.MaximumFidelity)})
		}
	}
	return findings
}

func validateProjectManagerProfile(profile projectManagerProfileContract) []finding {
	want := projectManagerProfileContract{
		ID:                  "project-manager/v1",
		TrackerContract:     trackerContract,
		Execution:           "optional",
		Equivalence:         "same-contract",
		ProviderSelection:   "selected-provider-skill",
		AllowedCapabilities: []string{"harness-connector"},
		ForbiddenCapabilities: []string{
			"shell",
			"filesystem-write",
			"git",
			"loaf-cli",
			"loaf-state",
			"credentials",
			"configuration",
			"installation",
			"authentication",
			"implementation",
			"research",
			"shaping",
			"prioritization",
			"orchestration",
			"spawn",
		},
		Fallback: "main-agent-same-contract",
	}
	var findings []finding
	if profile.ID != want.ID || profile.TrackerContract != want.TrackerContract || profile.Execution != want.Execution || profile.Equivalence != want.Equivalence || profile.ProviderSelection != want.ProviderSelection || profile.Fallback != want.Fallback {
		findings = append(findings, finding{"profile.contract", projectManagerContractPath, fmt.Sprintf("profile identity or execution differs from %+v", want)})
	}
	if !equalStrings(profile.AllowedCapabilities, want.AllowedCapabilities) {
		findings = append(findings, finding{"profile.authority", projectManagerContractPath, fmt.Sprintf("allowed_capabilities = %v, want %v", profile.AllowedCapabilities, want.AllowedCapabilities)})
	}
	if !equalStrings(profile.ForbiddenCapabilities, want.ForbiddenCapabilities) {
		findings = append(findings, finding{"profile.authority", projectManagerContractPath, "forbidden_capabilities must preserve the least-authority boundary"})
	}
	return findings
}

func canonicalOperations() []trackerOperation {
	return []trackerOperation{
		{ID: "connection.discover", Mode: "discover", Precondition: "none", Verification: "observed-connection", AmbiguousRetry: "not-applicable"},
		{ID: "capability.discover", Mode: "discover", Precondition: "selected-connection", Verification: "observed-capabilities", AmbiguousRetry: "not-applicable"},
		{ID: "work.read", Mode: "read", Precondition: "scoped-destination", Verification: "authoritative-read", AmbiguousRetry: "not-applicable"},
		{ID: "work.create", Mode: "write", Precondition: "read-current", Verification: "authoritative-readback", AmbiguousRetry: "never"},
		{ID: "work.update", Mode: "write", Precondition: "read-current", Verification: "authoritative-readback", AmbiguousRetry: "re-read-first"},
		{ID: "definition.write", Mode: "write", Precondition: "read-current", Verification: "authoritative-readback", AmbiguousRetry: "re-read-first"},
		{ID: "hierarchy.read", Mode: "read", Precondition: "scoped-destination", Verification: "authoritative-read", AmbiguousRetry: "not-applicable"},
		{ID: "hierarchy.change", Mode: "write", Precondition: "read-current", Verification: "authoritative-readback", AmbiguousRetry: "re-read-first"},
		{ID: "dependency.read", Mode: "read", Precondition: "scoped-destination", Verification: "authoritative-read", AmbiguousRetry: "not-applicable"},
		{ID: "dependency.change", Mode: "write", Precondition: "read-current", Verification: "authoritative-readback", AmbiguousRetry: "re-read-first"},
		{ID: "status.read", Mode: "read", Precondition: "scoped-destination", Verification: "authoritative-read", AmbiguousRetry: "not-applicable"},
		{ID: "status.transition", Mode: "write", Precondition: "read-current", Verification: "authoritative-readback", AmbiguousRetry: "re-read-first"},
		{ID: "comment.list", Mode: "read", Precondition: "scoped-destination", Verification: "authoritative-read", AmbiguousRetry: "not-applicable"},
		{ID: "comment.append", Mode: "write", Precondition: "read-current", Verification: "authoritative-readback", AmbiguousRetry: "never"},
	}
}

func equalOperations(left, right []trackerOperation) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func validProjectManagementFixture() fstest.MapFS {
	return fstest.MapFS{
		projectManagementContractPath: &fstest.MapFile{Data: []byte(validProjectManagementContract)},
		linearCapabilitiesPath:        &fstest.MapFile{Data: []byte(validLinearCapabilities)},
		projectManagerContractPath:    &fstest.MapFile{Data: []byte(validProjectManagerContract)},
		"skills/project-management/SKILL.md": &fstest.MapFile{Data: []byte(
			validSkill("project-management", "Defines project-management/v1 tracker operations. Use when operating canonical work.")),
		},
		"skills/linear/SKILL.md": &fstest.MapFile{Data: []byte(
			validSkill("linear", "Maps project-management/v1 semantics to Linear. Use when a Linear connection is selected.")),
		},
		"agents/project-manager.md": &fstest.MapFile{Data: []byte("# Project Manager\n\nExecutes `project-management/v1`.\n")},
	}
}

func contractOperationsJSON() string {
	operations := canonicalOperations()
	lines := make([]string, 0, len(operations))
	for _, operation := range operations {
		lines = append(lines, fmt.Sprintf(`    {"id": %q, "mode": %q, "precondition": %q, "verification": %q, "ambiguous_retry": %q}`,
			operation.ID,
			operation.Mode,
			operation.Precondition,
			operation.Verification,
			operation.AmbiguousRetry,
		))
	}
	return strings.Join(lines, ",\n")
}

func providerOperationsJSON() string {
	operations := canonicalOperations()
	lines := make([]string, 0, len(operations))
	for _, operation := range operations {
		lines = append(lines, fmt.Sprintf(`    {"id": %q, "native_semantic": %q, "availability": "runtime", "maximum_fidelity": "exact", "requires": ["capability"]}`,
			operation.ID,
			"linear."+strings.ReplaceAll(operation.ID, ".", "-"),
		))
	}
	return strings.Join(lines, ",\n")
}

func init() {
	validProjectManagementContract = fmt.Sprintf(validProjectManagementContractFormat, contractOperationsJSON())
	validLinearCapabilities = fmt.Sprintf(validLinearCapabilitiesFormat, providerOperationsJSON())
}

var (
	validProjectManagementContract string
	validLinearCapabilities        string
)

const validProjectManagementContractFormat = `{
  "id": "project-management/v1",
  "authority": {"shared_work": "tracker", "service_connections": "harness", "local_work_record": "forbidden", "tracker_proxy": "forbidden", "tracker_sync": "forbidden"},
  "routing": {"connection_discovery": "already-exposed-only", "destination_scope": "required", "provider_selection": "runtime-capability-checked"},
  "mutation_policy": {"read_before_write": "required", "authoritative_readback": "required", "ambiguous_create_retry": "forbidden", "ambiguous_comment_retry": "forbidden", "partial_outcomes": "preserve", "indeterminate_outcomes": "preserve"},
  "operations": [
%s
  ],
  "outcomes": ["confirmed", "unchanged", "partial", "failed", "indeterminate"],
  "fidelities": ["exact", "advisory", "manual", "unsupported"],
  "result_fields": ["operation", "destination", "native_ref", "outcome", "fidelity", "observed_state", "verification_evidence"],
  "semantic_channels": ["definition", "hierarchy", "dependency", "status", "comment"],
  "comment_policy": {"purpose": "evidence-and-collaboration", "substitute_for_definition": "forbidden", "substitute_for_relationship": "forbidden", "substitute_for_status": "forbidden"}
}`

const validLinearCapabilitiesFormat = `{
  "schema": "loaf-provider-capabilities/v1",
  "provider": "linear",
  "contract": "project-management/v1",
  "connection": "harness-native",
  "runtime_capability_discovery": "required",
  "operations": [
%s
  ]
}`

const validProjectManagerContract = `{
  "id": "project-manager/v1",
  "tracker_contract": "project-management/v1",
  "execution": "optional",
  "equivalence": "same-contract",
  "provider_selection": "selected-provider-skill",
  "allowed_capabilities": ["harness-connector"],
  "forbidden_capabilities": ["shell", "filesystem-write", "git", "loaf-cli", "loaf-state", "credentials", "configuration", "installation", "authentication", "implementation", "research", "shaping", "prioritization", "orchestration", "spawn"],
  "fallback": "main-agent-same-contract"
}`
