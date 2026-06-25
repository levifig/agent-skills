package main

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

func TestSkillContentHygieneStaleReferences(t *testing.T) {
	root := repoRoot(t)
	cases := []struct {
		rel       string
		forbidden []string
		required  []string
	}{
		{
			rel:       "content/skills/database-design/SKILL.md",
			forbidden: []string{"`infrastructure`"},
			required:  []string{"`infrastructure-management`"},
		},
		{
			rel:       "content/skills/power-systems-modeling/SKILL.md",
			forbidden: []string{"`database-patterns`"},
			required:  []string{"`database-design`"},
		},
		{
			rel: "content/skills/foundations/references/code-style.md",
			forbidden: []string{
				"`python` skill",
				"`typescript` skill",
				"`rails` skill",
			},
			required: []string{
				"`python-development` skill",
				"`typescript-development` skill",
				"`ruby-development` skill",
			},
		},
		{
			rel:       "content/skills/knowledge-base/SKILL.md",
			forbidden: []string{"CLAUDE.md"},
			required:  []string{".agents/AGENTS.md"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.rel, func(t *testing.T) {
			body := readTextFile(t, filepath.Join(root, filepath.FromSlash(tc.rel)))
			for _, forbidden := range tc.forbidden {
				if strings.Contains(body, forbidden) {
					t.Fatalf("%s still contains stale reference %q", tc.rel, forbidden)
				}
			}
			for _, required := range tc.required {
				if !strings.Contains(body, required) {
					t.Fatalf("%s missing replacement reference %q", tc.rel, required)
				}
			}
		})
	}
}

func TestSkillContentHygieneMarkdownStructure(t *testing.T) {
	root := repoRoot(t)
	skillsRoot := filepath.Join(root, "content", "skills")
	var failures []string
	err := filepath.WalkDir(skillsRoot, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || entry.Name() != "SKILL.md" {
			return nil
		}
		body := readTextFile(t, path)
		lines := strings.Split(body, "\n")
		if len(lines) > 100 && !strings.Contains(body, "\n## Contents\n") {
			failures = append(failures, relToRoot(t, root, path)+": missing ## Contents for "+strconv.Itoa(len(lines))+" lines")
		}
		if fences := strings.Count(body, "```"); fences%2 != 0 {
			failures = append(failures, relToRoot(t, root, path)+": unbalanced fenced code blocks")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WalkDir(%s) error = %v", skillsRoot, err)
	}
	sort.Strings(failures)
	if len(failures) > 0 {
		t.Fatalf("skill markdown structure hygiene failures:\n%s", strings.Join(failures, "\n"))
	}
}

func TestDocumentationStandardsDoesNotDuplicateADRTemplate(t *testing.T) {
	root := repoRoot(t)
	rel := filepath.FromSlash("content/skills/documentation-standards/references/documentation.md")
	body := readTextFile(t, filepath.Join(root, rel))
	for _, forbidden := range []string{"### ADR Template", "# ADR-XXX: Title"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("%s still publishes duplicate ADR authority %q", filepath.ToSlash(rel), forbidden)
		}
	}
	for _, required := range []string{"architecture", "templates/adr.md"} {
		if !strings.Contains(body, required) {
			t.Fatalf("%s missing ADR authority pointer %q", filepath.ToSlash(rel), required)
		}
	}
}

func TestSkillHelperExecutableContracts(t *testing.T) {
	root := repoRoot(t)

	validatorRel := filepath.FromSlash("content/skills/infrastructure-management/scripts/validate-k8s-manifest.py")
	validator := readTextFile(t, filepath.Join(root, validatorRel))
	for _, forbidden := range []string{"import yaml", "yaml.safe_load"} {
		if strings.Contains(validator, forbidden) {
			t.Fatalf("%s still depends on undeclared PyYAML via %q", filepath.ToSlash(validatorRel), forbidden)
		}
	}

	powerSkillDir := filepath.Join(root, "content", "skills", "power-systems-modeling")
	sidecar := readTextFile(t, filepath.Join(powerSkillDir, "SKILL.claude-code.yaml"))
	if hasShellScripts(t, filepath.Join(powerSkillDir, "scripts")) && !allowedToolsCanRunShellScripts(sidecar) {
		t.Fatalf("power-systems-modeling ships .sh helpers but sidecar allowed-tools cannot run shell scripts")
	}
}

func TestOrchestrationScriptSurfaceClassifiesEveryHelper(t *testing.T) {
	root := repoRoot(t)
	scriptsDir := filepath.Join(root, "content", "skills", "orchestration", "scripts")
	surfaceRel := filepath.FromSlash("content/skills/orchestration/references/script-surface.md")
	surface := readTextFile(t, filepath.Join(root, surfaceRel))

	var missing []string
	err := filepath.WalkDir(scriptsDir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		name := entry.Name()
		if !strings.Contains(surface, name) {
			missing = append(missing, name)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WalkDir(%s) error = %v", scriptsDir, err)
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Fatalf("%s does not classify orchestration helpers: %s", filepath.ToSlash(surfaceRel), strings.Join(missing, ", "))
	}
}

func TestOrchestrationDuplicateAuthorityReferencesRetired(t *testing.T) {
	root := repoRoot(t)
	orchestration := readTextFile(t, filepath.Join(root, "content", "skills", "orchestration", "SKILL.md"))
	for _, retired := range []string{
		"references/councils.md",
		"references/planning.md",
		"references/specs.md",
		"references/product-development.md",
	} {
		if strings.Contains(orchestration, retired) {
			t.Fatalf("orchestration router still links retired duplicate authority %q", retired)
		}
		if _, err := os.Stat(filepath.Join(root, "content", "skills", "orchestration", retired)); !os.IsNotExist(err) {
			t.Fatalf("%s stat = %v, want retired duplicate authority file missing", retired, err)
		}
	}
	for _, owner := range []string{
		"../council/SKILL.md",
		"../shape/SKILL.md",
		"../breakdown/SKILL.md",
	} {
		if !strings.Contains(orchestration, owner) {
			t.Fatalf("orchestration router missing owning skill link %q", owner)
		}
	}
}

func TestCliReferenceCatalogsSessionFamily(t *testing.T) {
	root := repoRoot(t)
	rel := filepath.FromSlash("content/skills/cli-reference/SKILL.md")
	body := readTextFile(t, filepath.Join(root, rel))
	for _, command := range []string{
		"loaf session start",
		"loaf session log",
		"loaf session end",
		"loaf session list",
		"loaf session show",
		"loaf session archive",
	} {
		if !strings.Contains(body, command) {
			t.Fatalf("%s missing session command %q", filepath.ToSlash(rel), command)
		}
	}
}

func hasShellScripts(t *testing.T, dir string) bool {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir(%s) error = %v", dir, err)
	}
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".sh") {
			return true
		}
	}
	return false
}

func allowedToolsCanRunShellScripts(sidecar string) bool {
	for _, line := range strings.Split(sidecar, "\n") {
		if !strings.HasPrefix(strings.TrimSpace(line), "allowed-tools:") {
			continue
		}
		tools := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "allowed-tools:"))
		for _, rawTool := range strings.Split(tools, ",") {
			tool := strings.TrimSpace(rawTool)
			if tool == "Bash" || strings.Contains(tool, ".sh") || strings.Contains(tool, "bash") {
				return true
			}
		}
	}
	return false
}

func relToRoot(t *testing.T, root string, path string) string {
	t.Helper()
	rel, err := filepath.Rel(root, path)
	if err != nil {
		t.Fatalf("Rel(%s, %s) error = %v", root, path, err)
	}
	return filepath.ToSlash(rel)
}
