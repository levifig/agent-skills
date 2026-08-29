package vnextflowcontract

import (
	"fmt"
	"testing"
)

func TestFlowCeremonyContracts(t *testing.T) {
	t.Parallel()

	manifest := flowManifest{
		Skills: []skillDeclaration{
			{Name: "pitch", Kind: "workflow"},
			{Name: "shape", Kind: "workflow"},
		},
		Templates: []templateContract{
			{ID: "problem-narrative"},
			{ID: "work-contract"},
		},
		Ceremonies: []ceremonyContract{
			{Name: "pitch", Skill: "pitch", Input: "human-context", Output: "problem-narrative", Template: "problem-narrative", TrackerOperations: []string{"work.read"}},
			{Name: "shape", Skill: "shape", Input: "problem-narrative", Output: "work-contract", Template: "work-contract", TrackerOperations: []string{"work.read", "work.create", "definition.write"}},
		},
	}
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
	sortFindings(findings)
	return findings
}
