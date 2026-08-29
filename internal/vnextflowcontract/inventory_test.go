package vnextflowcontract

import (
	"os"
	"testing"
)

func TestTrackerSkillInventoryIsClosed(t *testing.T) {
	t.Parallel()

	manifest := flowManifest{}
	if err := decodeStrictJSON(os.DirFS("../../vnext/content"), flowManifestPath, &manifest); err != nil {
		t.Fatalf("decode flow manifest: %v", err)
	}
	wantSkills := []skillDeclaration{
		{Name: "loaf-reference", Path: "skills/loaf-reference/SKILL.md", Kind: "reference"},
		{Name: "project-management", Path: "skills/project-management/SKILL.md", Kind: "reference"},
		{Name: "linear", Path: "skills/linear/SKILL.md", Kind: "provider"},
		{Name: "pitch", Path: "skills/pitch/SKILL.md", Kind: "workflow"},
		{Name: "triage", Path: "skills/triage/SKILL.md", Kind: "workflow"},
		{Name: "shape", Path: "skills/shape/SKILL.md", Kind: "workflow"},
		{Name: "implement", Path: "skills/implement/SKILL.md", Kind: "workflow"},
		{Name: "ship", Path: "skills/ship/SKILL.md", Kind: "workflow"},
		{Name: "release", Path: "skills/release/SKILL.md", Kind: "workflow"},
		{Name: "orchestration", Path: "skills/orchestration/SKILL.md", Kind: "workflow"},
	}
	if !equalSkillDeclarations(manifest.Skills, wantSkills) {
		t.Fatalf("skills = %+v, want closed inventory %+v", manifest.Skills, wantSkills)
	}

	wantCeremonies := []string{"pitch", "triage", "shape", "implement", "ship", "release", "orchestration"}
	actualCeremonies := make([]string, 0, len(manifest.Ceremonies))
	for _, ceremony := range manifest.Ceremonies {
		actualCeremonies = append(actualCeremonies, ceremony.Name)
	}
	if !equalStrings(actualCeremonies, wantCeremonies) {
		t.Fatalf("ceremonies = %v, want %v", actualCeremonies, wantCeremonies)
	}

	wantAgents := []agentDeclaration{{
		Name:         "project-manager",
		Path:         "agents/project-manager.md",
		ContractPath: "agents/project-manager.contract.json",
	}}
	if !equalAgentDeclarations(manifest.Agents, wantAgents) {
		t.Fatalf("agents = %+v, want %+v", manifest.Agents, wantAgents)
	}
}

func equalSkillDeclarations(left, right []skillDeclaration) bool {
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

func equalAgentDeclarations(left, right []agentDeclaration) bool {
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
