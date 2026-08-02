package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Skill tree invariance is the cross-target build contract that replaced
// per-harness prose substitution: one authored body is copied to every target
// without rewriting. Only frontmatter fields a target sidecar legitimately owns
// (plus the stamped version) may differ.

// nativeBuildSidecarOwnedFrontmatterKeys are frontmatter keys targets may set
// via SKILL.<target>.yaml sidecars or other target-owned packaging. Bodies and
// all other frontmatter must be byte-identical across targets.
var nativeBuildSidecarOwnedFrontmatterKeys = map[string]bool{
	"version":         true, // stamped by the build for every target
	"subtask":         true, // SKILL.opencode.yaml
	"user-invocable":  true, // SKILL.claude-code.yaml
	"argument-hint":   true, // SKILL.claude-code.yaml
	"allowed-tools":   true, // SKILL.claude-code.yaml
	"context":         true, // SKILL.claude-code.yaml
	"model":           true, // SKILL.claude-code.yaml
	"disable-model-invocation": true, // SKILL.claude-code.yaml
}

func nativeBuildSkillTreeDir(root string, targetName string) string {
	return filepath.Join(nativeBuildTargetOutputDir(root, targetName), "skills")
}

// normalizeNativeBuildSkillFileForInvariance returns the comparable form of a
// built skill file: for SKILL.md, frontmatter with sidecar-owned keys removed;
// for every other file, the raw bytes as a string.
func normalizeNativeBuildSkillFileForInvariance(path string, body string) string {
	if filepath.Base(path) != "SKILL.md" {
		return body
	}
	frontmatter, content := splitNativeBuildFrontmatter(body)
	if frontmatter == "" {
		return body
	}
	fields := parseNativeBuildYAMLFields(frontmatter)
	var kept []nativeBuildYAMLField
	for _, field := range fields {
		if nativeBuildSidecarOwnedFrontmatterKeys[field.key] {
			continue
		}
		kept = append(kept, field)
	}
	if len(kept) == 0 {
		return content
	}
	return "---\n" + renderNativeBuildYAMLFields(kept) + "---\n" + content
}

func listNativeBuildSkillRelativePaths(skillsDir string) ([]string, error) {
	var paths []string
	err := filepath.WalkDir(skillsDir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(skillsDir, path)
		if err != nil {
			return err
		}
		paths = append(paths, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	return paths, nil
}

// compareNativeBuildSkillTrees reports every path whose comparable content
// differs across targets. baseline is the first target in targets.
func compareNativeBuildSkillTrees(root string, targets []string) error {
	if len(targets) < 2 {
		return fmt.Errorf("need at least two targets to compare skill trees")
	}
	baseline := targets[0]
	baselineDir := nativeBuildSkillTreeDir(root, baseline)
	baselinePaths, err := listNativeBuildSkillRelativePaths(baselineDir)
	if err != nil {
		return fmt.Errorf("%s skill tree: %w", baseline, err)
	}
	baselineContent := map[string]string{}
	for _, rel := range baselinePaths {
		body, err := os.ReadFile(filepath.Join(baselineDir, filepath.FromSlash(rel)))
		if err != nil {
			return err
		}
		baselineContent[rel] = normalizeNativeBuildSkillFileForInvariance(rel, string(body))
	}

	var findings []string
	for _, target := range targets[1:] {
		targetDir := nativeBuildSkillTreeDir(root, target)
		targetPaths, err := listNativeBuildSkillRelativePaths(targetDir)
		if err != nil {
			return fmt.Errorf("%s skill tree: %w", target, err)
		}
		targetSet := map[string]bool{}
		for _, rel := range targetPaths {
			targetSet[rel] = true
			body, err := os.ReadFile(filepath.Join(targetDir, filepath.FromSlash(rel)))
			if err != nil {
				return err
			}
			got := normalizeNativeBuildSkillFileForInvariance(rel, string(body))
			want, ok := baselineContent[rel]
			if !ok {
				findings = append(findings, fmt.Sprintf("%s has extra skill path %s (absent from %s)", target, rel, baseline))
				continue
			}
			if got != want {
				findings = append(findings, fmt.Sprintf("%s differs from %s at skills/%s", target, baseline, rel))
			}
		}
		for _, rel := range baselinePaths {
			if !targetSet[rel] {
				findings = append(findings, fmt.Sprintf("%s missing skill path %s (present in %s)", target, rel, baseline))
			}
		}
	}
	if len(findings) == 0 {
		return nil
	}
	sort.Strings(findings)
	return fmt.Errorf("skill tree is not target-invariant:\n  %s", strings.Join(findings, "\n  "))
}

// skillMarkdownHasHarnessProseSubstitution is true when content differs from
// itself under any historical harness-language rewrite. Used only by tests to
// prove no build path still rewrites authored prose.
func skillMarkdownTransformIsIdentity(transform func(string) string, samples []string) error {
	if transform == nil {
		return nil
	}
	for _, sample := range samples {
		if got := transform(sample); got != sample {
			return fmt.Errorf("transform rewrote markdown:\n  in:  %q\n  out: %q", sample, got)
		}
	}
	return nil
}

// harnessProseSubstitutionProbeSamples are strings the retired second-stage
// replacer would have mutated. If any build transform rewrites these, the
// neutral build contract is broken.
func harnessProseSubstitutionProbeSamples() []string {
	return []string{
		"Claude Code uses permission prompts.",
		"Edit CLAUDE.md when needed.",
		"Create `.claude/CLAUDE.md -> ../AGENTS.md`.",
		"Use AskUserQuestionTool for interviews.",
		"Use AskUserQuestion when clarifying.",
		"Track work with TodoWrite and TodoRead.",
		"Use TodoWrite/TodoRead together.",
		"Invoke /loaf:implement for the task.",
		"Spawn via Task tool with Task(subagent_type=implementer).",
		"Subagents and subagent development use subagents.",
		"{{IMPLEMENT_CMD}} --continue",
		"{{HARNESS_NAME}} / {{INTERVIEW_TOOL}} / {{SUBAGENT_MECHANISM}} / {{TODO_TOOL}} / {{AGENTS_FILE}}",
	}
}
