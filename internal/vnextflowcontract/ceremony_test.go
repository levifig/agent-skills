package vnextflowcontract

import (
	"fmt"
	"reflect"
	"testing"
)

func TestFlowCeremonyContracts(t *testing.T) {
	t.Parallel()

	manifest := canonicalCeremonyManifest()
	assertNoContractFindings(t, validateCeremonyDeclarations(manifest))
}

func TestFlowCeremonyContractRejectsMissingWorkflowAndUnknownOperation(t *testing.T) {
	t.Parallel()

	manifest := flowManifest{
		Skills: []skillDeclaration{
			{Name: "pitch", Kind: "workflow"},
			{Name: "shape", Kind: "workflow"},
		},
		Templates: []templateContract{{ID: "problem-narrative"}},
		Ceremonies: []ceremonyContract{
			{Name: "pitch", Skill: "pitch", Input: "human-context", Output: "problem-narrative", Template: "problem-narrative", TrackerOperations: []string{"local.issue.create"}},
		},
	}
	findings := validateCeremonyDeclarations(manifest)
	assertContractFinding(t, findings, "ceremony.inventory", "workflow skill \"shape\"")
	assertContractFinding(t, findings, "ceremony.operation", "local.issue.create")
}

func TestFlowCeremonyContractRejectsAuthorityDrift(t *testing.T) {
	t.Parallel()

	mutations := []struct {
		name   string
		mutate func([]ceremonyContract)
	}{
		{name: "pitch gains create", mutate: func(ceremonies []ceremonyContract) {
			ceremonies[0].TrackerOperations = append(ceremonies[0].TrackerOperations, "work.create")
		}},
		{name: "pitch gains transition", mutate: func(ceremonies []ceremonyContract) {
			ceremonies[0].TrackerOperations = append(ceremonies[0].TrackerOperations, "status.transition")
		}},
		{name: "input drift", mutate: func(ceremonies []ceremonyContract) { ceremonies[2].Input = "local-work-copy" }},
		{name: "output drift", mutate: func(ceremonies []ceremonyContract) { ceremonies[4].Output = "approval-comment" }},
		{name: "template drift", mutate: func(ceremonies []ceremonyContract) { ceremonies[5].Template = "work-contract" }},
		{name: "orchestration gains transition", mutate: func(ceremonies []ceremonyContract) {
			ceremonies[6].TrackerOperations = append(ceremonies[6].TrackerOperations, "status.transition")
		}},
		{name: "operation order drift", mutate: func(ceremonies []ceremonyContract) {
			ceremonies[1].TrackerOperations[2], ceremonies[1].TrackerOperations[3] = ceremonies[1].TrackerOperations[3], ceremonies[1].TrackerOperations[2]
		}},
	}

	for _, mutation := range mutations {
		mutation := mutation
		t.Run(mutation.name, func(t *testing.T) {
			t.Parallel()

			manifest := canonicalCeremonyManifest()
			mutation.mutate(manifest.Ceremonies)
			assertContractFinding(t, validateCeremonyDeclarations(manifest), "ceremony.contract", "external canonical contract")
		})
	}
}

func validateCeremonyDeclarations(manifest flowManifest) []finding {
	workflowSkills := make(map[string]struct{})
	for _, skill := range manifest.Skills {
		if skill.Kind == "workflow" {
			workflowSkills[skill.Name] = struct{}{}
		}
	}
	templates := make(map[string]struct{})
	for _, template := range manifest.Templates {
		templates[template.ID] = struct{}{}
	}
	operations := make(map[string]struct{})
	for _, operation := range canonicalOperations() {
		operations[operation.ID] = struct{}{}
	}

	declaredSkills := make(map[string]struct{})
	declaredNames := make(map[string]struct{})
	var findings []finding
	for _, ceremony := range manifest.Ceremonies {
		ceremonyPath := fmt.Sprintf("ceremonies[%s]", ceremony.Name)
		if ceremony.Name == "" || ceremony.Skill == "" || ceremony.Input == "" || ceremony.Output == "" {
			findings = append(findings, finding{"ceremony.contract", flowManifestPath, ceremonyPath + " requires name, skill, input, and output"})
		}
		if _, exists := declaredNames[ceremony.Name]; exists {
			findings = append(findings, finding{"ceremony.inventory", flowManifestPath, fmt.Sprintf("duplicate ceremony %q", ceremony.Name)})
		}
		declaredNames[ceremony.Name] = struct{}{}
		if _, exists := workflowSkills[ceremony.Skill]; !exists {
			findings = append(findings, finding{"ceremony.skill", flowManifestPath, fmt.Sprintf("ceremony %q references non-workflow skill %q", ceremony.Name, ceremony.Skill)})
		}
		if ceremony.Name != ceremony.Skill {
			findings = append(findings, finding{"ceremony.skill", flowManifestPath, fmt.Sprintf("ceremony %q must match skill %q", ceremony.Name, ceremony.Skill)})
		}
		declaredSkills[ceremony.Skill] = struct{}{}
		if _, exists := templates[ceremony.Template]; !exists {
			findings = append(findings, finding{"ceremony.template", flowManifestPath, fmt.Sprintf("ceremony %q references unknown template %q", ceremony.Name, ceremony.Template)})
		}
		if len(ceremony.TrackerOperations) == 0 {
			findings = append(findings, finding{"ceremony.operation", flowManifestPath, fmt.Sprintf("ceremony %q must declare tracker operations", ceremony.Name)})
		}
		seenOperations := make(map[string]struct{})
		for _, operation := range ceremony.TrackerOperations {
			if _, exists := operations[operation]; !exists {
				findings = append(findings, finding{"ceremony.operation", flowManifestPath, fmt.Sprintf("ceremony %q uses unknown operation %q", ceremony.Name, operation)})
			}
			if _, exists := seenOperations[operation]; exists {
				findings = append(findings, finding{"ceremony.operation", flowManifestPath, fmt.Sprintf("ceremony %q repeats operation %q", ceremony.Name, operation)})
			}
			seenOperations[operation] = struct{}{}
		}
	}
	for skill := range workflowSkills {
		if _, exists := declaredSkills[skill]; !exists {
			findings = append(findings, finding{"ceremony.inventory", flowManifestPath, fmt.Sprintf("workflow skill %q has no ceremony contract", skill)})
		}
	}
	wantCeremonies := canonicalCeremonies()
	if len(manifest.Ceremonies) != len(wantCeremonies) {
		findings = append(findings, finding{"ceremony.contract", flowManifestPath, fmt.Sprintf("ceremonies has %d entries, want %d from the external canonical contract", len(manifest.Ceremonies), len(wantCeremonies))})
	}
	for index, ceremony := range manifest.Ceremonies {
		if index >= len(wantCeremonies) || !reflect.DeepEqual(ceremony, wantCeremonies[index]) {
			findings = append(findings, finding{"ceremony.contract", flowManifestPath, fmt.Sprintf("ceremony %q differs from the external canonical contract", ceremony.Name)})
		}
	}
	sortFindings(findings)
	return findings
}

func canonicalCeremonyManifest() flowManifest {
	ceremonies := canonicalCeremonies()
	workflowSkills := make([]skillDeclaration, 0, len(ceremonies))
	for _, ceremony := range ceremonies {
		workflowSkills = append(workflowSkills, skillDeclaration{Name: ceremony.Skill, Kind: "workflow"})
	}
	return flowManifest{
		Skills: workflowSkills,
		Templates: []templateContract{
			{ID: "problem-narrative"},
			{ID: "work-contract"},
			{ID: "tracker-update"},
		},
		Ceremonies: ceremonies,
	}
}

func canonicalCeremonies() []ceremonyContract {
	return []ceremonyContract{
		{Name: "pitch", Skill: "pitch", Input: "human-context", Output: "problem-narrative", Template: "problem-narrative", TrackerOperations: []string{"connection.discover", "capability.discover", "work.read", "comment.list"}},
		{Name: "triage", Skill: "triage", Input: "native-work-candidates", Output: "native-dispositions", Template: "tracker-update", TrackerOperations: []string{"connection.discover", "capability.discover", "work.read", "work.update", "status.read", "status.transition", "comment.list", "comment.append"}},
		{Name: "shape", Skill: "shape", Input: "problem-narrative", Output: "work-contract", Template: "work-contract", TrackerOperations: []string{"connection.discover", "capability.discover", "work.read", "work.create", "work.update", "definition.write", "hierarchy.read", "hierarchy.change", "dependency.read", "dependency.change", "status.read"}},
		{Name: "implement", Skill: "implement", Input: "work-contract", Output: "implementation-evidence", Template: "tracker-update", TrackerOperations: []string{"connection.discover", "capability.discover", "work.read", "hierarchy.read", "dependency.read", "status.read", "status.transition", "comment.list", "comment.append"}},
		{Name: "ship", Skill: "ship", Input: "implementation-evidence", Output: "quality-verdict", Template: "tracker-update", TrackerOperations: []string{"connection.discover", "capability.discover", "work.read", "hierarchy.read", "dependency.read", "status.read", "status.transition", "comment.list", "comment.append"}},
		{Name: "release", Skill: "release", Input: "landed-work", Output: "release-outcome", Template: "tracker-update", TrackerOperations: []string{"connection.discover", "capability.discover", "work.read", "work.update", "status.read", "comment.list", "comment.append"}},
		{Name: "orchestration", Skill: "orchestration", Input: "work-contract", Output: "execution-coordination", Template: "tracker-update", TrackerOperations: []string{"connection.discover", "capability.discover", "work.read", "hierarchy.read", "dependency.read", "status.read", "comment.list", "comment.append"}},
	}
}
