package vnextflowcontract

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"reflect"
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
	ID              string                        `json:"id"`
	NativeSemantic  string                        `json:"native_semantic"`
	Availability    string                        `json:"availability"`
	MaximumFidelity string                        `json:"maximum_fidelity"`
	Requires        providerOperationRequirements `json:"requires"`
}

type providerOperationRequirements struct {
	Before  []string `json:"before"`
	Execute []string `json:"execute"`
	After   []string `json:"after"`
}

type projectManagerProfileContract struct {
	ID             string                       `json:"id"`
	Execution      string                       `json:"execution"`
	BehaviorSource projectManagerBehaviorSource `json:"behavior_source"`
	Authority      projectManagerAuthority      `json:"authority"`
	Fallback       string                       `json:"fallback"`
}

type projectManagerBehaviorSource struct {
	ContractID    string `json:"contract_id"`
	ContractPath  string `json:"contract_path"`
	SkillPath     string `json:"skill_path"`
	ProviderRoute string `json:"provider_route"`
}

type projectManagerAuthority struct {
	Connector             string `json:"connector"`
	NonConnectorAuthority string `json:"non_connector_authority"`
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
	assertContractFinding(t, findings, "provider.mapping", "exact Linear mapping")
}

func TestTrackerSkillContractRejectsExpandedProjectManagerAuthority(t *testing.T) {
	t.Parallel()

	fixture := validProjectManagementFixture()
	profile := fixture[projectManagerContractPath]
	profile.Data = []byte(strings.Replace(string(profile.Data), `"non_connector_authority": "none"`, `"non_connector_authority": "shell"`, 1))
	fixture[projectManagerContractPath] = profile

	findings := validateProjectManagementContract(fixture)
	assertContractFinding(t, findings, "profile.authority", "non-connector")
}

func TestTrackerSkillLinearMappingRejectsExactContractDrift(t *testing.T) {
	t.Parallel()

	mutations := []struct {
		name   string
		mutate func(*providerOperationMapping)
	}{
		{name: "native semantic", mutate: func(mapping *providerOperationMapping) { mapping.NativeSemantic += ".broader" }},
		{name: "availability", mutate: func(mapping *providerOperationMapping) { mapping.Availability = "universal" }},
		{name: "maximum fidelity", mutate: func(mapping *providerOperationMapping) { mapping.MaximumFidelity = "advisory" }},
		{name: "before requirement", mutate: func(mapping *providerOperationMapping) {
			mapping.Requires.Before = append(mapping.Requires.Before, "issue.unproven")
		}},
		{name: "execute requirement", mutate: func(mapping *providerOperationMapping) {
			mapping.Requires.Execute = append(mapping.Requires.Execute, "issue.unproven")
		}},
		{name: "after requirement", mutate: func(mapping *providerOperationMapping) {
			mapping.Requires.After = append(mapping.Requires.After, "issue.unproven")
		}},
	}

	for operationIndex := range canonicalProviderOperations() {
		for _, mutation := range mutations {
			operationIndex := operationIndex
			mutation := mutation
			t.Run(fmt.Sprintf("%02d/%s", operationIndex, mutation.name), func(t *testing.T) {
				t.Parallel()

				contract := projectManagementContract{Operations: canonicalOperations()}
				provider := providerCapabilities{
					Schema:                     "loaf-provider-capabilities/v1",
					Provider:                   "linear",
					Contract:                   trackerContract,
					Connection:                 "harness-native",
					RuntimeCapabilityDiscovery: "required",
					Operations:                 canonicalProviderOperations(),
				}
				mutation.mutate(&provider.Operations[operationIndex])
				assertContractFinding(t, validateProviderCapabilities(contract, provider), "provider.mapping", "exact Linear mapping")
			})
		}
	}
}

func TestTrackerSkillProjectManagerUsesSharedBehaviorSource(t *testing.T) {
	t.Parallel()

	mutations := []struct {
		name        string
		oldValue    string
		newValue    string
		wantRule    string
		wantDetails string
	}{
		{name: "different contract", oldValue: `"contract_path": "skills/project-management/contract.json"`, newValue: `"contract_path": "agents/independent.json"`, wantRule: "profile.behavior", wantDetails: "behavior_source"},
		{name: "provider bypass", oldValue: `"provider_route": "selected-provider-skill"`, newValue: `"provider_route": "direct-connector"`, wantRule: "profile.behavior", wantDetails: "behavior_source"},
		{name: "broader authority", oldValue: `"non_connector_authority": "none"`, newValue: `"non_connector_authority": "implementation"`, wantRule: "profile.authority", wantDetails: "non-connector"},
	}

	for _, mutation := range mutations {
		mutation := mutation
		t.Run(mutation.name, func(t *testing.T) {
			t.Parallel()

			fixture := validProjectManagementFixture()
			profile := fixture[projectManagerContractPath]
			profile.Data = []byte(strings.Replace(string(profile.Data), mutation.oldValue, mutation.newValue, 1))
			fixture[projectManagerContractPath] = profile
			assertContractFinding(t, validateProjectManagementContract(fixture), mutation.wantRule, mutation.wantDetails)
		})
	}
}

func TestTrackerSkillSharedMutationSafetyCannotDrift(t *testing.T) {
	t.Parallel()

	mutations := []struct {
		name     string
		oldValue string
		newValue string
	}{
		{name: "read before write", oldValue: `"read_before_write": "required"`, newValue: `"read_before_write": "optional"`},
		{name: "authoritative readback", oldValue: `"authoritative_readback": "required"`, newValue: `"authoritative_readback": "optional"`},
		{name: "blind create retry", oldValue: `"ambiguous_create_retry": "forbidden"`, newValue: `"ambiguous_create_retry": "allowed"`},
		{name: "blind comment retry", oldValue: `"ambiguous_comment_retry": "forbidden"`, newValue: `"ambiguous_comment_retry": "allowed"`},
	}

	for _, mutation := range mutations {
		mutation := mutation
		t.Run(mutation.name, func(t *testing.T) {
			t.Parallel()

			fixture := validProjectManagementFixture()
			contract := fixture[projectManagementContractPath]
			contract.Data = []byte(strings.Replace(string(contract.Data), mutation.oldValue, mutation.newValue, 1))
			fixture[projectManagementContractPath] = contract
			assertContractFinding(t, validateProjectManagementContract(fixture), "tracker.mutation", "mutation_policy")
		})
	}
}

func TestTrackerSkillProjectManagerCannotOwnIndependentOperations(t *testing.T) {
	t.Parallel()

	for _, field := range []string{"operations", "capabilities", "retry_policy"} {
		field := field
		t.Run(field, func(t *testing.T) {
			t.Parallel()

			fixture := validProjectManagementFixture()
			profile := fixture[projectManagerContractPath]
			profile.Data = []byte(strings.Replace(string(profile.Data), "\n}", fmt.Sprintf(",\n  %q: []\n}", field), 1))
			fixture[projectManagerContractPath] = profile
			assertContractFinding(t, validateProjectManagementContract(fixture), "profile.contract", "unknown field")
		})
	}
}

func validateProjectManagementContract(content fs.FS) []finding {
	var findings []finding
	manifest := flowManifest{}
	if err := decodeStrictJSON(content, flowManifestPath, &manifest); err != nil {
		return []finding{{"flow.manifest", flowManifestPath, err.Error()}}
	}
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
		findings = append(findings, validateProjectManagerExecution(manifest, profile)...)
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

func validateProjectManagerExecution(manifest flowManifest, profile projectManagerProfileContract) []finding {
	wantExecution := canonicalExecutionContract()
	var findings []finding
	if manifest.Execution != wantExecution {
		findings = append(findings, finding{"profile.execution", flowManifestPath, "main-agent primary execution and optional profile fallback must match the canonical route"})
	}
	if profile.BehaviorSource.ContractID != manifest.TrackerContract ||
		profile.BehaviorSource.ContractPath != manifest.Execution.Primary.BehaviorContract ||
		profile.BehaviorSource.SkillPath != manifest.Execution.Primary.BehaviorSkill ||
		profile.BehaviorSource.ProviderRoute != manifest.Execution.Primary.ProviderRoute {
		findings = append(findings, finding{"profile.execution", projectManagerContractPath, "optional profile behavior must derive from the primary main-agent contract, skill, and provider route"})
	}
	if profile.ID != manifest.Execution.OptionalProfile.ID ||
		manifest.Execution.OptionalProfile.ContractPath != projectManagerContractPath ||
		manifest.Execution.OptionalProfile.Fallback != "primary" ||
		profile.Fallback != "main-agent-same-contract" {
		findings = append(findings, finding{"profile.execution", projectManagerContractPath, "optional profile must fall back to primary main-agent execution of the same contract"})
	}
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
	wantMappings := canonicalProviderOperations()
	if len(provider.Operations) != len(wantMappings) || len(provider.Operations) != len(contract.Operations) {
		findings = append(findings, finding{"provider.operations", linearCapabilitiesPath, fmt.Sprintf("operations has %d entries, want %d", len(provider.Operations), len(wantMappings))})
	}
	for index, mapping := range provider.Operations {
		if index >= len(contract.Operations) || mapping.ID != contract.Operations[index].ID {
			findings = append(findings, finding{"provider.operations", linearCapabilitiesPath, fmt.Sprintf("operation %d does not match the common contract order", index)})
		}
		if index >= len(wantMappings) || !reflect.DeepEqual(mapping, wantMappings[index]) {
			findings = append(findings, finding{"provider.mapping", linearCapabilitiesPath, fmt.Sprintf("operation %q must match the exact Linear mapping and phased capability requirements", mapping.ID)})
		}
	}
	return findings
}

func validateProjectManagerProfile(profile projectManagerProfileContract) []finding {
	want := projectManagerProfileContract{
		ID:        "project-manager/v1",
		Execution: "optional",
		BehaviorSource: projectManagerBehaviorSource{
			ContractID:    trackerContract,
			ContractPath:  projectManagementContractPath,
			SkillPath:     "skills/project-management/SKILL.md",
			ProviderRoute: "selected-provider-skill",
		},
		Authority: projectManagerAuthority{
			Connector:             "selected-provider-skill-only",
			NonConnectorAuthority: "none",
		},
		Fallback: "main-agent-same-contract",
	}
	var findings []finding
	if profile.ID != want.ID || profile.Execution != want.Execution || profile.Fallback != want.Fallback {
		findings = append(findings, finding{"profile.contract", projectManagerContractPath, fmt.Sprintf("profile identity or execution differs from %+v", want)})
	}
	if profile.BehaviorSource != want.BehaviorSource {
		findings = append(findings, finding{"profile.behavior", projectManagerContractPath, fmt.Sprintf("behavior_source = %+v, want the shared source %+v", profile.BehaviorSource, want.BehaviorSource)})
	}
	if profile.Authority != want.Authority {
		findings = append(findings, finding{"profile.authority", projectManagerContractPath, "authority must remain selected-provider-skill-only with no non-connector authority"})
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

func canonicalProviderOperations() []providerOperationMapping {
	return []providerOperationMapping{
		{ID: "connection.discover", NativeSemantic: "harness.exposed-linear-connection", Availability: "runtime", MaximumFidelity: "exact", Requires: providerOperationRequirements{Before: []string{}, Execute: []string{"connection.list"}, After: []string{}}},
		{ID: "capability.discover", NativeSemantic: "harness.linear-capability-description", Availability: "runtime", MaximumFidelity: "exact", Requires: providerOperationRequirements{Before: []string{"connection.select"}, Execute: []string{"connection.describe"}, After: []string{}}},
		{ID: "work.read", NativeSemantic: "linear.issue", Availability: "runtime", MaximumFidelity: "exact", Requires: providerOperationRequirements{Before: []string{"destination.scope"}, Execute: []string{"issue.read"}, After: []string{}}},
		{ID: "work.create", NativeSemantic: "linear.issue", Availability: "runtime", MaximumFidelity: "exact", Requires: providerOperationRequirements{Before: []string{"issue.search", "issue.read"}, Execute: []string{"issue.create"}, After: []string{"issue.read"}}},
		{ID: "work.update", NativeSemantic: "linear.issue-fields", Availability: "runtime", MaximumFidelity: "exact", Requires: providerOperationRequirements{Before: []string{"issue.read"}, Execute: []string{"issue.update"}, After: []string{"issue.read"}}},
		{ID: "definition.write", NativeSemantic: "linear.issue-description", Availability: "runtime", MaximumFidelity: "exact", Requires: providerOperationRequirements{Before: []string{"issue.read"}, Execute: []string{"issue.description.write"}, After: []string{"issue.read"}}},
		{ID: "hierarchy.read", NativeSemantic: "linear.issue-parent-and-children", Availability: "runtime", MaximumFidelity: "exact", Requires: providerOperationRequirements{Before: []string{"destination.scope"}, Execute: []string{"issue.parent.read", "issue.children.read"}, After: []string{}}},
		{ID: "hierarchy.change", NativeSemantic: "linear.issue-parent-and-children", Availability: "runtime", MaximumFidelity: "exact", Requires: providerOperationRequirements{Before: []string{"issue.parent.read", "issue.children.read"}, Execute: []string{"issue.parent.write"}, After: []string{"issue.parent.read", "issue.children.read"}}},
		{ID: "dependency.read", NativeSemantic: "linear.issue-relations", Availability: "runtime", MaximumFidelity: "exact", Requires: providerOperationRequirements{Before: []string{"destination.scope"}, Execute: []string{"issue.relation.read"}, After: []string{}}},
		{ID: "dependency.change", NativeSemantic: "linear.issue-relations", Availability: "runtime", MaximumFidelity: "exact", Requires: providerOperationRequirements{Before: []string{"issue.relation.read"}, Execute: []string{"issue.relation.write"}, After: []string{"issue.relation.read"}}},
		{ID: "status.read", NativeSemantic: "linear.issue-state", Availability: "runtime", MaximumFidelity: "exact", Requires: providerOperationRequirements{Before: []string{"destination.scope"}, Execute: []string{"issue.state.read", "workflow.state.list"}, After: []string{}}},
		{ID: "status.transition", NativeSemantic: "linear.issue-state", Availability: "runtime", MaximumFidelity: "exact", Requires: providerOperationRequirements{Before: []string{"issue.state.read", "workflow.state.list"}, Execute: []string{"issue.state.write"}, After: []string{"issue.state.read"}}},
		{ID: "comment.list", NativeSemantic: "linear.issue-comments", Availability: "runtime", MaximumFidelity: "exact", Requires: providerOperationRequirements{Before: []string{"issue.read"}, Execute: []string{"issue.comment.read"}, After: []string{}}},
		{ID: "comment.append", NativeSemantic: "linear.issue-comment", Availability: "runtime", MaximumFidelity: "exact", Requires: providerOperationRequirements{Before: []string{"issue.read", "issue.comment.read"}, Execute: []string{"issue.comment.write"}, After: []string{"issue.read", "issue.comment.read"}}},
	}
}

func validProjectManagementFixture() fstest.MapFS {
	return fstest.MapFS{
		flowManifestPath:              &fstest.MapFile{Data: []byte(validFlowManifest)},
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
	operations := canonicalProviderOperations()
	lines := make([]string, 0, len(operations))
	for _, operation := range operations {
		before, _ := json.Marshal(operation.Requires.Before)
		execute, _ := json.Marshal(operation.Requires.Execute)
		after, _ := json.Marshal(operation.Requires.After)
		lines = append(lines, fmt.Sprintf(`    {"id": %q, "native_semantic": %q, "availability": %q, "maximum_fidelity": %q, "requires": {"before": %s, "execute": %s, "after": %s}}`,
			operation.ID,
			operation.NativeSemantic,
			operation.Availability,
			operation.MaximumFidelity,
			before,
			execute,
			after,
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
  "execution": "optional",
  "behavior_source": {
    "contract_id": "project-management/v1",
    "contract_path": "skills/project-management/contract.json",
    "skill_path": "skills/project-management/SKILL.md",
    "provider_route": "selected-provider-skill"
  },
  "authority": {
    "connector": "selected-provider-skill-only",
    "non_connector_authority": "none"
  },
  "fallback": "main-agent-same-contract"
}`
