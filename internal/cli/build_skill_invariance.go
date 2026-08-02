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
// (plus the stamped version) may differ — and only when the built value matches
// that authorization.

// nativeBuildSidecarOwnedFrontmatterKeys are frontmatter keys targets may set
// via SKILL.<target>.yaml sidecars. Presence of the key alone is not enough:
// the built value must match the sidecar (see normalizeNativeBuildSkillFileForInvariance).
var nativeBuildSidecarOwnedFrontmatterKeys = map[string]bool{
	"subtask":                  true, // SKILL.opencode.yaml
	"user-invocable":           true, // SKILL.claude-code.yaml (and some opencode sidecars)
	"argument-hint":            true, // SKILL.claude-code.yaml
	"allowed-tools":            true, // SKILL.claude-code.yaml
	"context":                  true, // SKILL.claude-code.yaml
	"model":                    true, // SKILL.claude-code.yaml
	"disable-model-invocation": true, // SKILL.claude-code.yaml
}

func nativeBuildSkillTreeDir(root string, targetName string) string {
	return filepath.Join(nativeBuildTargetOutputDir(root, targetName), "skills")
}

func nativeBuildSkillContentSrcDir(root string) string {
	return filepath.Join(root, "content", "skills")
}

func nativeBuildSkillSidecarPath(root string, skill string, targetName string) string {
	return filepath.Join(nativeBuildSkillContentSrcDir(root), skill, "SKILL."+targetName+".yaml")
}

func nativeBuildSkillSidecarAuthorizedValues(root string, skill string, targetName string) (map[string]string, error) {
	path := nativeBuildSkillSidecarPath(root, skill, targetName)
	body, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]string{}, nil
		}
		return nil, err
	}
	values := map[string]string{}
	for _, field := range parseNativeBuildSimpleYAMLScalarFields(string(body)) {
		if nativeBuildSidecarOwnedFrontmatterKeys[field.key] {
			values[field.key] = field.value
		}
	}
	return values, nil
}

// normalizeNativeBuildSkillFileForInvariance returns the comparable form of a
// built skill file. SKILL.md frontmatter is compared as raw text after removing
// only keys whose built values match that target's authorized sidecar (and the
// stamped version, after the caller has asserted version equality). Nested
// YAML under kept keys is preserved — the comparison must not silently drop it.
func normalizeNativeBuildSkillFileForInvariance(rel string, body string, authorized map[string]string) (normalized string, version string, err error) {
	if filepath.Base(rel) != "SKILL.md" {
		return body, "", nil
	}
	frontmatter, content := splitNativeBuildFrontmatter(body)
	if frontmatter == "" {
		return body, "", nil
	}
	kept, version, err := stripAuthorizedNativeBuildFrontmatter(frontmatter, authorized)
	if err != nil {
		return "", "", fmt.Errorf("%s: %w", rel, err)
	}
	if strings.TrimSpace(kept) == "" {
		return content, version, nil
	}
	return "---\n" + kept + "---\n" + content, version, nil
}

// stripAuthorizedNativeBuildFrontmatter walks raw frontmatter lines, removing
// top-level keys that are authorized to differ and keeping everything else —
// including indented nested structure under kept keys.
func stripAuthorizedNativeBuildFrontmatter(frontmatter string, authorized map[string]string) (kept string, version string, err error) {
	lines := strings.Split(frontmatter, "\n")
	var out []string
	for i := 0; i < len(lines); {
		line := lines[i]
		if line == "" || strings.HasPrefix(strings.TrimSpace(line), "#") || strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") || !strings.Contains(line, ":") {
			out = append(out, line)
			i++
			continue
		}
		key, rawValue, _ := strings.Cut(strings.TrimSpace(line), ":")
		key = strings.TrimSpace(key)
		block, next := collectNativeBuildFrontmatterKeyBlock(lines, i)
		i = next

		if key == "version" {
			version = unquoteNativeBuildYAML(strings.TrimSpace(rawValue))
			if version == ">-" || version == ">" || version == "|" || version == "|-" {
				version = strings.TrimSpace(strings.Join(block[1:], "\n"))
			}
			continue
		}
		if nativeBuildSidecarOwnedFrontmatterKeys[key] {
			auth, ok := authorized[key]
			if !ok {
				return "", "", fmt.Errorf("sidecar-owned frontmatter key %q present without a matching SKILL.<target>.yaml entry", key)
			}
			builtValue := nativeBuildFrontmatterBlockScalarValue(block)
			if builtValue != auth {
				return "", "", fmt.Errorf("sidecar-owned frontmatter key %q value %q does not match authorized sidecar value %q", key, builtValue, auth)
			}
			continue
		}
		out = append(out, block...)
	}
	return strings.Join(out, "\n"), version, nil
}

func collectNativeBuildFrontmatterKeyBlock(lines []string, start int) (block []string, next int) {
	block = []string{lines[start]}
	i := start + 1
	for i < len(lines) {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			// Blank lines inside a folded/literal block belong to the key; a
			// blank followed by another top-level key ends the block.
			if i+1 < len(lines) && !strings.HasPrefix(lines[i+1], " ") && !strings.HasPrefix(lines[i+1], "\t") && strings.TrimSpace(lines[i+1]) != "" {
				break
			}
			block = append(block, line)
			i++
			continue
		}
		if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			block = append(block, line)
			i++
			continue
		}
		break
	}
	return block, i
}

func nativeBuildFrontmatterBlockScalarValue(block []string) string {
	if len(block) == 0 {
		return ""
	}
	_, rawValue, _ := strings.Cut(strings.TrimSpace(block[0]), ":")
	rawValue = strings.TrimSpace(rawValue)
	if rawValue == ">-" || rawValue == ">" || rawValue == "|" || rawValue == "|-" {
		var nested []string
		for _, line := range block[1:] {
			nested = append(nested, strings.TrimPrefix(strings.TrimPrefix(line, "  "), "\t"))
		}
		if strings.HasPrefix(rawValue, ">") {
			return foldNativeBuildYAMLBlockValue(nested)
		}
		return strings.Join(nested, "\n")
	}
	return unquoteNativeBuildYAML(rawValue)
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
// differs across targets. baseline is the first target in targets. Sidecar-owned
// keys are stripped only when their built values match that target's sidecar;
// stamped versions must be identical across every target.
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

	type normalizedFile struct {
		body    string
		version string
	}
	baselineContent := map[string]normalizedFile{}
	var stampedVersion string
	for _, rel := range baselinePaths {
		body, err := os.ReadFile(filepath.Join(baselineDir, filepath.FromSlash(rel)))
		if err != nil {
			return err
		}
		authorized, err := nativeBuildSkillSidecarAuthorizedValues(root, strings.Split(filepath.ToSlash(rel), "/")[0], baseline)
		if err != nil {
			return err
		}
		got, version, err := normalizeNativeBuildSkillFileForInvariance(rel, string(body), authorized)
		if err != nil {
			return fmt.Errorf("%s: %w", baseline, err)
		}
		baselineContent[rel] = normalizedFile{body: got, version: version}
		if filepath.Base(rel) == "SKILL.md" {
			if stampedVersion == "" {
				stampedVersion = version
			} else if version != stampedVersion {
				return fmt.Errorf("%s stamps inconsistent versions %q and %q", baseline, stampedVersion, version)
			}
		}
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
			authorized, err := nativeBuildSkillSidecarAuthorizedValues(root, strings.Split(filepath.ToSlash(rel), "/")[0], target)
			if err != nil {
				return err
			}
			got, version, err := normalizeNativeBuildSkillFileForInvariance(rel, string(body), authorized)
			if err != nil {
				return fmt.Errorf("%s: %w", target, err)
			}
			want, ok := baselineContent[rel]
			if !ok {
				findings = append(findings, fmt.Sprintf("%s has extra skill path %s (absent from %s)", target, rel, baseline))
				continue
			}
			if filepath.Base(rel) == "SKILL.md" {
				if version != stampedVersion {
					findings = append(findings, fmt.Sprintf("%s stamps version %q at skills/%s, want %q (same as %s)", target, version, rel, stampedVersion, baseline))
				}
			}
			if got != want.body {
				findings = append(findings, fmt.Sprintf("%s differs from %s at skills/%s", target, baseline, rel))
			}
		}
		for _, rel := range baselinePaths {
			if !targetSet[rel] {
				findings = append(findings, fmt.Sprintf("%s missing skill path %s (present in %s)", target, rel, baseline))
			}
		}
	}
	if stampedVersion == "" {
		findings = append(findings, "no stamped version found on any SKILL.md")
	}
	if len(findings) == 0 {
		return nil
	}
	sort.Strings(findings)
	return fmt.Errorf("skill tree is not target-invariant:\n  %s", strings.Join(findings, "\n  "))
}

// skillMarkdownTransformIsIdentity reports whether transform leaves every sample
// unchanged. A nil transform is a contract violation — callers must pass the
// real transform under test, never nil as a vacuous pass.
func skillMarkdownTransformIsIdentity(transform func(string) string, samples []string) error {
	if transform == nil {
		return fmt.Errorf("skill markdown transform is nil; pass the real transform to prove identity")
	}
	for _, sample := range samples {
		if got := transform(sample); got != sample {
			return fmt.Errorf("transform rewrote markdown:\n  in:  %q\n  out: %q", sample, got)
		}
	}
	return nil
}

// harnessProseSubstitutionProbeSamples are strings the retired second-stage
// replacer would have mutated. If any build path rewrites these, the neutral
// build contract is broken. Samples intentionally contain the *pre-substitution*
// forms so a reintroduced replacer would emit the banned post-substitution strings.
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

// harnessProseSubstitutionBannedProducts are strings a reintroduced second-stage
// replacer would emit from harnessProseSubstitutionProbeSamples. Presence of any
// of these in built probe content means substitution returned.
func harnessProseSubstitutionBannedProducts() []string {
	return []string{
		"Codex uses permission",
		"OpenCode uses permission",
		"Cursor uses permission",
		"Amp uses permission",
		"update_plan, update_plan",
		"request_user_input",
		"native task/todo surface when available, native task/todo surface when available",
		"task list or chat checklist, task list or chat checklist",
		"Amp thread checklist, Amp thread checklist",
	}
}

// labeledHarnessSectionBody returns the body under a markdown heading until the
// next ## / ### ATX heading of the same or higher level (fewer #), or EOF. Child
// ### sections stay inside a ## parent. Only space-terminated ATX headings at
// level 2+ count — fenced comments like "# Coordination only" are not headings.
func labeledHarnessSectionBody(doc string, heading string) (string, bool) {
	marker := strings.TrimSpace(heading)
	if !strings.HasPrefix(marker, "#") {
		marker = "### " + marker
	}
	level := 0
	for level < len(marker) && marker[level] == '#' {
		level++
	}
	index := strings.Index(doc, marker)
	if index < 0 {
		return "", false
	}
	if index > 0 && doc[index-1] != '\n' {
		return "", false
	}
	rest := doc[index+len(marker):]
	if len(rest) > 0 && rest[0] == '\n' {
		rest = rest[1:]
	}
	end := len(rest)
	offset := 0
	for offset < len(rest) {
		i := strings.Index(rest[offset:], "\n#")
		if i < 0 {
			break
		}
		abs := offset + i
		hashes := 0
		for abs+1+hashes < len(rest) && rest[abs+1+hashes] == '#' {
			hashes++
		}
		// Require ## or ### (or more) followed by a space — not "# comment" fences.
		next := abs + 1 + hashes
		if hashes >= 2 && next < len(rest) && rest[next] == ' ' && hashes <= level {
			end = abs
			break
		}
		offset = abs + 1
	}
	return rest[:end], true
}
