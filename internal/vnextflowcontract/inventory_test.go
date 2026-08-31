package vnextflowcontract

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"testing"
	"testing/fstest"
)

func TestTrackerSkillInventoryIsClosed(t *testing.T) {
	t.Parallel()

	content := os.DirFS("../../vnext/content")
	assertNoContractFindings(t, validateClosedContentInventory(content))
	manifest := flowManifest{}
	if err := decodeStrictJSON(content, flowManifestPath, &manifest); err != nil {
		t.Fatalf("decode flow manifest: %v", err)
	}
	wantSkills := []skillDeclaration{
		{Name: "loaf-reference", Path: "skills/loaf-reference/SKILL.md", Kind: "reference"},
		{Name: "project-management", Path: "skills/project-management/SKILL.md", Kind: "reference"},
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

}

type contentInventoryEntry struct {
	Path      string
	Directory bool
}

func validateClosedContentInventory(content fs.FS) []finding {
	wantEntries := canonicalContentInventory()
	wantEntries = append(wantEntries, discoveredProviderInventory(content)...)
	wantByPath := make(map[string]contentInventoryEntry, len(wantEntries))
	for _, entry := range wantEntries {
		wantByPath[entry.Path] = entry
	}

	seen := make(map[string]struct{}, len(wantEntries))
	var findings []finding
	err := fs.WalkDir(content, ".", func(filePath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			findings = append(findings, finding{"content.inventory", filePath, walkErr.Error()})
			return nil
		}
		seen[filePath] = struct{}{}
		if entry.Type()&fs.ModeSymlink != 0 {
			findings = append(findings, finding{"content.symlink", filePath, "symlink entries are forbidden in portable Flow content"})
		}
		expected, exists := wantByPath[filePath]
		if !exists {
			findings = append(findings, finding{"content.inventory", filePath, "path is not in the external content allowlist"})
			return nil
		}
		if entry.IsDir() != expected.Directory {
			wantKind := "file"
			if expected.Directory {
				wantKind = "directory"
			}
			findings = append(findings, finding{"content.inventory", filePath, fmt.Sprintf("entry kind differs from external allowlist; want %s", wantKind)})
		}
		return nil
	})
	if err != nil {
		findings = append(findings, finding{"content.inventory", ".", err.Error()})
	}
	for _, entry := range wantEntries {
		if _, exists := seen[entry.Path]; !exists {
			findings = append(findings, finding{"content.inventory", entry.Path, "path from the external content allowlist is missing"})
		}
	}
	sortFindings(findings)
	return findings
}

func discoveredProviderInventory(content fs.FS) []contentInventoryEntry {
	entries, err := fs.ReadDir(content, "skills")
	if err != nil {
		return nil
	}
	var inventory []contentInventoryEntry
	for _, entry := range entries {
		if !entry.IsDir() || !providerManifestExists(content, entry.Name()) {
			continue
		}
		providerRoot := "skills/" + entry.Name()
		inventory = append(inventory,
			contentInventoryEntry{Path: providerRoot, Directory: true},
			contentInventoryEntry{Path: providerRoot + "/SKILL.md"},
			contentInventoryEntry{Path: providerRoot + "/capabilities.json"},
		)
		references, err := fs.ReadDir(content, providerRoot+"/references")
		if err != nil {
			continue
		}
		inventory = append(inventory, contentInventoryEntry{Path: providerRoot + "/references", Directory: true})
		for _, reference := range references {
			if !reference.IsDir() && path.Ext(reference.Name()) == ".md" {
				inventory = append(inventory, contentInventoryEntry{Path: providerRoot + "/references/" + reference.Name()})
			}
		}
	}
	return inventory
}

func providerManifestExists(content fs.FS, name string) bool {
	info, err := fs.Stat(content, "skills/"+name+"/capabilities.json")
	return err == nil && info.Mode().IsRegular()
}

func canonicalContentInventory() []contentInventoryEntry {
	return []contentInventoryEntry{
		{Path: ".", Directory: true},
		{Path: "agents", Directory: true},
		{Path: "agents/project-manager.contract.json"},
		{Path: "agents/project-manager.md"},
		{Path: "flow-contract.json"},
		{Path: "skills", Directory: true},
		{Path: "skills/implement", Directory: true},
		{Path: "skills/implement/SKILL.md"},
		{Path: "skills/loaf-reference", Directory: true},
		{Path: "skills/loaf-reference/SKILL.md"},
		{Path: "skills/loaf-reference/references", Directory: true},
		{Path: "skills/loaf-reference/references/authority-model.md"},
		{Path: "skills/loaf-reference/references/flow-semantics.md"},
		{Path: "skills/orchestration", Directory: true},
		{Path: "skills/orchestration/SKILL.md"},
		{Path: "skills/orchestration/references", Directory: true},
		{Path: "skills/orchestration/references/delegation.md"},
		{Path: "skills/pitch", Directory: true},
		{Path: "skills/pitch/SKILL.md"},
		{Path: "skills/pitch/references", Directory: true},
		{Path: "skills/pitch/references/interview-guide.md"},
		{Path: "skills/project-management", Directory: true},
		{Path: "skills/project-management/SKILL.md"},
		{Path: "skills/project-management/contract.json"},
		{Path: "skills/project-management/references", Directory: true},
		{Path: "skills/project-management/references/provider-modules.md"},
		{Path: "skills/project-management/references/record-contract.md"},
		{Path: "skills/release", Directory: true},
		{Path: "skills/release/SKILL.md"},
		{Path: "skills/shape", Directory: true},
		{Path: "skills/shape/SKILL.md"},
		{Path: "skills/shape/references", Directory: true},
		{Path: "skills/shape/references/decomposition.md"},
		{Path: "skills/ship", Directory: true},
		{Path: "skills/ship/SKILL.md"},
		{Path: "skills/triage", Directory: true},
		{Path: "skills/triage/SKILL.md"},
		{Path: "templates", Directory: true},
		{Path: "templates/problem-narrative.md"},
		{Path: "templates/tracker-update.md"},
		{Path: "templates/work-contract.md"},
	}
}

func TestFlowContentInventoryRejectsEveryUndeclaredSurface(t *testing.T) {
	t.Parallel()

	fixtures := []struct {
		name string
		path string
	}{
		{name: "orphan agent json", path: "agents/orphan.json"},
		{name: "orphan agent yaml", path: "agents/orphan.yaml"},
		{name: "unknown provider tree", path: "skills/jira/SKILL.md"},
		{name: "root script", path: "bootstrap.sh"},
		{name: "root hook", path: "pre-tool-hook"},
		{name: "root config", path: "config.toml"},
		{name: "root yaml", path: "settings.yaml"},
		{name: "root source", path: "client.go"},
		{name: "root text", path: "notes.txt"},
		{name: "extra profile", path: "agents/reviewer.md"},
		{name: "extra profile contract", path: "agents/reviewer.contract.json"},
		{name: "target sidecar", path: "skills/linear/SKILL.codex.yaml"},
		{name: "target surface", path: "targets/codex.json"},
		{name: "build surface", path: "build/manifest.json"},
		{name: "install surface", path: "install/connector.sh"},
	}

	for _, fixtureCase := range fixtures {
		fixtureCase := fixtureCase
		t.Run(fixtureCase.name, func(t *testing.T) {
			t.Parallel()

			fixture := validContentInventoryFixture()
			fixture[fixtureCase.path] = &fstest.MapFile{Data: []byte("undeclared\n")}
			assertContractFinding(t, validateClosedContentInventory(fixture), "content.inventory", "not in the external content allowlist")
		})
	}
}

func TestFlowContentInventoryRejectsSymlinks(t *testing.T) {
	t.Parallel()

	fixture := validContentInventoryFixture()
	fixture["templates/work-contract.md"] = &fstest.MapFile{
		Data: []byte(validWorkContractTemplate),
		Mode: fs.ModeSymlink | 0o777,
	}
	assertContractFinding(t, validateClosedContentInventory(fixture), "content.symlink", "symlink")
}

func validContentInventoryFixture() fstest.MapFS {
	fixture := fstest.MapFS{}
	for _, entry := range canonicalContentInventory() {
		if !entry.Directory {
			fixture[entry.Path] = &fstest.MapFile{Data: []byte{}}
		}
	}
	return fixture
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
