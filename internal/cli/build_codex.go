package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

var codexEnforcementHooks = map[string]bool{
	"artifact-body-write":     true,
	"check-" + "sec" + "rets": true,
	"render-drift":            true,
	"validate-push":           true,
	"validate-commit":         true,
	"workflow-pre-pr":         true,
	"security-audit":          true,
}

type nativeBuildYAMLField struct {
	key   string
	value string
}

type nativeCodexHooksJSON struct {
	Version int                  `json:"version"`
	Hooks   nativeCodexHookTypes `json:"hooks"`
}

type nativeCodexHookTypes struct {
	PreToolUse []nativeCodexPreToolHookJSON `json:"PreToolUse"`
}

type nativeCodexPreToolHookJSON struct {
	LoafManaged bool   `json:"loaf-managed"`
	Matcher     string `json:"matcher"`
	Command     string `json:"command"`
	Timeout     int    `json:"timeout"`
	FailClosed  bool   `json:"failClosed"`
	Blocking    bool   `json:"blocking"`
	If          string `json:"if,omitempty"`
	Description string `json:"description,omitempty"`
}

func runNativeBuildCodex(root string, out io.Writer) error {
	start := time.Now()
	fmt.Fprintf(out, "\n%s\n\n", ansiBold("loaf build"))

	sharedStart := time.Now()
	fmt.Fprintf(out, "  %s shared skills intermediate...", ansiCyan("building"))
	if err := buildNativeSharedSkillsIntermediate(root); err != nil {
		fmt.Fprintf(out, "\r  %s shared skills intermediate\n", ansiRed("✗"))
		return err
	}
	fmt.Fprintf(out, "\r  %s shared skills intermediate %s\n", ansiGreen("✓"), ansiGray("("+elapsedSeconds(sharedStart)+")"))

	targetStart := time.Now()
	fmt.Fprintf(out, "  %s codex...", ansiCyan("building"))
	if err := buildNativeCodexTarget(root); err != nil {
		fmt.Fprintf(out, "\r  %s codex\n", ansiRed("✗"))
		return err
	}
	fmt.Fprintf(out, "\r  %s codex %s\n", ansiGreen("✓"), ansiGray("("+elapsedSeconds(targetStart)+")"))
	fmt.Fprintln(out)
	fmt.Fprintf(out, "%s %s\n", ansiGreen("Build complete"), ansiGray("("+elapsedSeconds(start)+")"))
	return nil
}

func runNativeBuildSkillOnlyTarget(root string, out io.Writer, targetName string) error {
	start := time.Now()
	fmt.Fprintf(out, "\n%s\n\n", ansiBold("loaf build"))

	sharedStart := time.Now()
	fmt.Fprintf(out, "  %s shared skills intermediate...", ansiCyan("building"))
	if err := buildNativeSharedSkillsIntermediate(root); err != nil {
		fmt.Fprintf(out, "\r  %s shared skills intermediate\n", ansiRed("✗"))
		return err
	}
	fmt.Fprintf(out, "\r  %s shared skills intermediate %s\n", ansiGreen("✓"), ansiGray("("+elapsedSeconds(sharedStart)+")"))

	targetStart := time.Now()
	fmt.Fprintf(out, "  %s %s...", ansiCyan("building"), targetName)
	if err := buildNativeSkillOnlyTarget(root, targetName); err != nil {
		fmt.Fprintf(out, "\r  %s %s\n", ansiRed("✗"), targetName)
		return err
	}
	fmt.Fprintf(out, "\r  %s %s %s\n", ansiGreen("✓"), targetName, ansiGray("("+elapsedSeconds(targetStart)+")"))
	fmt.Fprintln(out)
	fmt.Fprintf(out, "%s %s\n", ansiGreen("Build complete"), ansiGray("("+elapsedSeconds(start)+")"))
	return nil
}

func elapsedSeconds(start time.Time) string {
	return fmt.Sprintf("%.2fs", time.Since(start).Seconds())
}

func buildNativeSharedSkillsIntermediate(root string) error {
	targetsConfig, err := readNativeBuildTargetsConfig(root)
	if err != nil {
		return err
	}
	src := filepath.Join(root, "content")
	dest := filepath.Join(root, "dist", "skills")
	if err := os.RemoveAll(dest); err != nil {
		return err
	}
	return copyNativeBuildSkills(nativeBuildSkillCopyOptions{
		srcDir:        src,
		destDir:       dest,
		targetName:    "shared",
		targetsConfig: targetsConfig,
		transformMd:   substituteNativeBuildCommands,
	})
}

func buildNativeCodexTarget(root string) error {
	version, err := nativeBuildPackageVersion(root)
	if err != nil {
		return err
	}
	targetsConfig, err := readNativeBuildTargetsConfig(root)
	if err != nil {
		return err
	}
	dist := filepath.Join(root, "dist", "codex")
	if err := os.RemoveAll(dist); err != nil {
		return err
	}
	if err := copyNativeBuildSkills(nativeBuildSkillCopyOptions{
		srcDir:        filepath.Join(root, "dist"),
		destDir:       filepath.Join(dist, "skills"),
		targetName:    "codex",
		version:       version,
		targetsConfig: targetsConfig,
		transformMd:   func(content string) string { return substituteNativeBuildHarnessLanguage(content, "codex") },
	}); err != nil {
		return err
	}
	return generateNativeCodexHooksJSON(root, dist)
}

func buildNativeSkillOnlyTarget(root string, targetName string) error {
	version, err := nativeBuildPackageVersion(root)
	if err != nil {
		return err
	}
	targetsConfig, err := readNativeBuildTargetsConfig(root)
	if err != nil {
		return err
	}
	dist := filepath.Join(root, "dist", targetName)
	if err := os.RemoveAll(dist); err != nil {
		return err
	}
	return copyNativeBuildSkills(nativeBuildSkillCopyOptions{
		srcDir:        filepath.Join(root, "dist"),
		destDir:       filepath.Join(dist, "skills"),
		targetName:    targetName,
		version:       version,
		targetsConfig: targetsConfig,
		transformMd:   func(content string) string { return substituteNativeBuildHarnessLanguage(content, targetName) },
	})
}

type nativeBuildSkillCopyOptions struct {
	srcDir        string
	destDir       string
	sidecarSrcDir string
	targetName    string
	version       string
	targetsConfig nativeBuildTargetsConfig
	transformMd   func(string) string
}

type nativeBuildTargetsConfig struct {
	sharedTemplates map[string][]string
}

func copyNativeBuildSkills(options nativeBuildSkillCopyOptions) error {
	skillsDir := filepath.Join(options.srcDir, "skills")
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		skill := entry.Name()
		skillSrc := filepath.Join(skillsDir, skill)
		skillDest := filepath.Join(options.destDir, skill)
		if err := os.MkdirAll(skillDest, 0o755); err != nil {
			return err
		}
		if err := writeNativeBuildSkillMarkdown(skillSrc, skillDest, options); err != nil {
			return err
		}
		for _, subdir := range []string{"references", "templates"} {
			if err := copyNativeBuildDir(filepath.Join(skillSrc, subdir), filepath.Join(skillDest, subdir), options.transformMd, true); err != nil {
				return err
			}
		}
		if err := copyNativeBuildDir(filepath.Join(skillSrc, "scripts"), filepath.Join(skillDest, "scripts"), nil, false); err != nil {
			return err
		}
		if err := copyNativeSharedTemplates(skill, skillDest, options.srcDir, options.targetsConfig, options.transformMd); err != nil {
			return err
		}
	}
	return nil
}

func writeNativeBuildSkillMarkdown(skillSrc string, skillDest string, options nativeBuildSkillCopyOptions) error {
	path := filepath.Join(skillSrc, "SKILL.md")
	body, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	frontmatter, content := splitNativeBuildFrontmatter(string(body))
	fields := parseNativeBuildYAMLFields(frontmatter)
	sidecarSrc := skillSrc
	if options.sidecarSrcDir != "" {
		sidecarSrc = filepath.Join(options.sidecarSrcDir, "skills", filepath.Base(skillSrc))
	}
	fields = mergeNativeBuildTargetSidecar(fields, filepath.Join(sidecarSrc, "SKILL."+options.targetName+".yaml"))
	if options.targetName == "claude-code" {
		description := firstNativeBuildYAMLField(fields, "description")
		descriptionRunes := []rune(description)
		if len(descriptionRunes) > 250 {
			fields = setNativeBuildYAMLField(fields, "description", string(descriptionRunes[:247])+"...")
		}
	}
	if options.version != "" {
		fields = setNativeBuildYAMLField(fields, "version", options.version)
	}
	output := "---\n" + renderNativeBuildYAMLFields(fields) + "---\n" + content
	output = options.transformMd(output)
	return os.WriteFile(filepath.Join(skillDest, "SKILL.md"), []byte(output), 0o644)
}

func splitNativeBuildFrontmatter(content string) (string, string) {
	if !strings.HasPrefix(content, "---\n") {
		return "", content
	}
	rest := strings.TrimPrefix(content, "---\n")
	index := strings.Index(rest, "\n---")
	if index < 0 {
		return "", content
	}
	frontmatter := rest[:index]
	body := rest[index+len("\n---"):]
	if strings.HasPrefix(body, "\n") {
		body = body[1:]
	}
	return frontmatter, body
}

func parseNativeBuildYAMLFields(frontmatter string) []nativeBuildYAMLField {
	var fields []nativeBuildYAMLField
	lines := strings.Split(frontmatter, "\n")
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(line, " ") || !strings.Contains(trimmed, ":") {
			continue
		}
		key, value, _ := strings.Cut(trimmed, ":")
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if value == ">-" || value == ">" || value == "|" || value == "|-" {
			var block []string
			for i+1 < len(lines) && (strings.HasPrefix(lines[i+1], " ") || strings.TrimSpace(lines[i+1]) == "") {
				i++
				block = append(block, strings.TrimPrefix(lines[i], "  "))
			}
			if strings.HasPrefix(value, ">") {
				value = foldNativeBuildYAMLBlockValue(block)
			} else {
				value = strings.Join(block, "\n")
			}
		} else {
			value = unquoteNativeBuildYAML(value)
		}
		fields = setNativeBuildYAMLField(fields, key, value)
	}
	return fields
}

func foldNativeBuildYAMLBlockValue(lines []string) string {
	var paragraphs []string
	var current []string
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			if len(current) > 0 {
				paragraphs = append(paragraphs, strings.Join(current, " "))
				current = nil
			}
			paragraphs = append(paragraphs, "")
			continue
		}
		current = append(current, strings.TrimSpace(line))
	}
	if len(current) > 0 {
		paragraphs = append(paragraphs, strings.Join(current, " "))
	}
	return strings.Join(paragraphs, "\n")
}

func mergeNativeBuildTargetSidecar(fields []nativeBuildYAMLField, sidecarPath string) []nativeBuildYAMLField {
	body, err := os.ReadFile(sidecarPath)
	if err != nil {
		return fields
	}
	for _, field := range parseNativeBuildSimpleYAMLScalarFields(string(body)) {
		fields = setNativeBuildYAMLField(fields, field.key, field.value)
	}
	return fields
}

func setNativeBuildYAMLField(fields []nativeBuildYAMLField, key string, value string) []nativeBuildYAMLField {
	for i, field := range fields {
		if field.key == key {
			fields[i].value = value
			return fields
		}
	}
	return append(fields, nativeBuildYAMLField{key: key, value: value})
}

func firstNativeBuildYAMLField(fields []nativeBuildYAMLField, key string) string {
	for _, field := range fields {
		if field.key == key {
			return field.value
		}
	}
	return ""
}

func renderNativeBuildYAMLFields(fields []nativeBuildYAMLField) string {
	var out strings.Builder
	for _, field := range fields {
		if shouldFoldNativeBuildYAMLValue(field.value) {
			out.WriteString(field.key)
			out.WriteString(": >-\n")
			for _, line := range wrapNativeBuildYAMLText(field.value, 78) {
				out.WriteString("  ")
				out.WriteString(line)
				out.WriteByte('\n')
			}
			continue
		}
		out.WriteString(field.key)
		out.WriteString(": ")
		out.WriteString(quoteNativeBuildYAMLScalar(field.value))
		out.WriteByte('\n')
	}
	return out.String()
}

func shouldFoldNativeBuildYAMLValue(value string) bool {
	return strings.Contains(value, "\n") || len(value) > 80
}

func wrapNativeBuildYAMLText(value string, width int) []string {
	var wrapped []string
	paragraphs := strings.Split(value, "\n")
	for index, paragraph := range paragraphs {
		words := strings.Fields(paragraph)
		if len(words) == 0 {
			wrapped = append(wrapped, "")
		} else {
			line := words[0]
			for _, word := range words[1:] {
				if len(line)+1+len(word) > width {
					wrapped = append(wrapped, line)
					line = word
				} else {
					line += " " + word
				}
			}
			wrapped = append(wrapped, line)
		}
		if index < len(paragraphs)-1 {
			wrapped = append(wrapped, "")
		}
	}
	return wrapped
}

func quoteNativeBuildYAMLScalar(value string) string {
	if value == "" {
		return `""`
	}
	if strings.ContainsAny(value, ":#[]{}\n\t,") || strings.HasPrefix(value, " ") || strings.HasSuffix(value, " ") {
		if !strings.Contains(value, "'") {
			return "'" + value + "'"
		}
		return strconv.Quote(value)
	}
	return value
}

func copyNativeBuildDir(src string, dest string, transform func(string) string, transformMarkdown bool) error {
	info, err := os.Stat(src)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if !info.IsDir() {
		return nil
	}
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dest, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		mode := info.Mode().Perm()
		if transformMarkdown && strings.HasSuffix(d.Name(), ".md") && transform != nil {
			body = []byte(transform(string(body)))
		}
		return os.WriteFile(target, body, mode)
	})
}

func copyNativeSharedTemplates(skill string, skillDest string, srcDir string, config nativeBuildTargetsConfig, transform func(string) string) error {
	for template, skills := range config.sharedTemplates {
		if !containsBuildTarget(skills, skill) {
			continue
		}
		src := filepath.Join(srcDir, "templates", template)
		dest := filepath.Join(skillDest, "templates", template)
		if pathExistsNative(dest) || !pathExistsNative(src) {
			continue
		}
		body, err := os.ReadFile(src)
		if err != nil {
			return err
		}
		if strings.HasSuffix(template, ".md") && transform != nil {
			body = []byte(transform(string(body)))
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(dest, body, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func substituteNativeBuildCommands(content string) string {
	return substituteNativeBuildHarnessLanguage(content, "claude-code")
}

type nativeBuildHarnessLanguage struct {
	harnessName       string
	interviewTool     string
	subagentMechanism string
	todoTool          string
	agentsFile        string
}

var nativeBuildHarnessLanguages = map[string]nativeBuildHarnessLanguage{
	"claude-code": {
		harnessName:       "Claude Code",
		interviewTool:     "AskUserQuestionTool",
		subagentMechanism: "Task subagents",
		todoTool:          "TodoWrite",
		agentsFile:        "CLAUDE.md",
	},
	"codex": {
		harnessName:       "Codex",
		interviewTool:     "request_user_input",
		subagentMechanism: "separate Codex thread or explicit multi-agent tool when available",
		todoTool:          "update_plan",
		agentsFile:        "AGENTS.md",
	},
	"cursor": {
		harnessName:       "Cursor",
		interviewTool:     "built-in chat clarification",
		subagentMechanism: "background agent",
		todoTool:          "task list or chat checklist",
		agentsFile:        "AGENTS.md",
	},
	"opencode": {
		harnessName:       "OpenCode",
		interviewTool:     "prompt the user in chat",
		subagentMechanism: "subtask agent",
		todoTool:          "native task/todo surface when available",
		agentsFile:        "AGENTS.md",
	},
	"amp": {
		harnessName:       "Amp",
		interviewTool:     "Amp UI input",
		subagentMechanism: "Amp check/agent mode or new thread",
		todoTool:          "Amp thread checklist",
		agentsFile:        "AGENTS.md",
	},
}

func substituteNativeBuildHarnessLanguage(content string, targetName string) string {
	language, ok := nativeBuildHarnessLanguages[targetName]
	if !ok {
		language = nativeBuildHarnessLanguages["claude-code"]
	}
	content = strings.NewReplacer(
		"{{IMPLEMENT_CMD}}", "/implement",
		"{{RESUME_CMD}}", "/implement",
		"{{ORCHESTRATE_CMD}}", "/implement",
		"{{HARNESS_NAME}}", language.harnessName,
		"{{INTERVIEW_TOOL}}", language.interviewTool,
		"{{SUBAGENT_MECHANISM}}", language.subagentMechanism,
		"{{TODO_TOOL}}", language.todoTool,
		"{{AGENTS_FILE}}", language.agentsFile,
	).Replace(content)
	if targetName == "claude-code" {
		return content
	}
	return strings.NewReplacer(
		".claude/CLAUDE.md -> .agents/AGENTS.md", ".agents/AGENTS.md",
		".claude/CLAUDE.md", ".agents/AGENTS.md",
		"Claude Code", language.harnessName,
		"CLAUDE.md", language.agentsFile,
		"AskUserQuestionTool", language.interviewTool,
		"AskUserQuestion", language.interviewTool,
		"TodoWrite/TodoRead", language.todoTool,
		"TodoWrite", language.todoTool,
		"TodoRead", language.todoTool,
		"/loaf:", "/",
		"Task subagents", language.subagentMechanism,
		"Task tool", language.subagentMechanism,
		"Task(subagent_type=", "Agent(agent_type=",
		"subagent_type", "agent_type",
		"Subagents", language.subagentMechanism,
		"subagents", language.subagentMechanism,
		"Subagent", language.subagentMechanism,
		"subagent", language.subagentMechanism,
	).Replace(content)
}

func readNativeBuildTargetsConfig(root string) (nativeBuildTargetsConfig, error) {
	body, err := os.ReadFile(filepath.Join(root, "config", "targets.yaml"))
	if err != nil {
		if os.IsNotExist(err) {
			return nativeBuildTargetsConfig{sharedTemplates: map[string][]string{}}, nil
		}
		return nativeBuildTargetsConfig{}, err
	}
	return nativeBuildTargetsConfig{sharedTemplates: parseNativeBuildSharedTemplates(string(body))}, nil
}

func parseNativeBuildSharedTemplates(content string) map[string][]string {
	templates := map[string][]string{}
	inShared := false
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if !strings.HasPrefix(line, " ") && strings.HasSuffix(trimmed, ":") {
			inShared = trimmed == "shared-templates:"
			continue
		}
		if !inShared || !strings.HasPrefix(line, "  ") || strings.HasPrefix(line, "    ") {
			continue
		}
		key, value, ok := strings.Cut(trimmed, ":")
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		if strings.HasPrefix(value, "[") && strings.HasSuffix(value, "]") {
			value = strings.TrimSuffix(strings.TrimPrefix(value, "["), "]")
			for _, part := range strings.Split(value, ",") {
				name := strings.TrimSpace(part)
				if name != "" {
					templates[key] = append(templates[key], name)
				}
			}
		}
	}
	return templates
}

func nativeBuildPackageVersion(root string) (string, error) {
	body, err := os.ReadFile(filepath.Join(root, "package.json"))
	if err != nil {
		return "", err
	}
	var pkg struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(body, &pkg); err != nil {
		return "", err
	}
	if pkg.Version == "" {
		return "", fmt.Errorf("package.json missing version")
	}
	return pkg.Version, nil
}

func generateNativeCodexHooksJSON(root string, dist string) error {
	hooks, err := readNativeBuildHooks(filepath.Join(root, "config", "hooks.yaml"))
	if err != nil {
		return err
	}
	var preTool []nativeCodexPreToolHookJSON
	for _, hook := range hooks {
		if hook.section != "pre-tool" || !codexEnforcementHooks[hook.id] || !strings.Contains(hook.matcher, "Bash") {
			continue
		}
		timeout := hook.timeout
		if timeout == 0 {
			timeout = 30000
		}
		entry := nativeCodexPreToolHookJSON{
			LoafManaged: true,
			Matcher:     "Bash",
			Command:     "loaf check --hook " + hook.id,
			Timeout:     timeout / 1000,
			FailClosed:  hook.failClosed,
			Blocking:    hook.blocking,
			If:          hook.ifCondition,
		}
		if hook.description != "" {
			entry.Description = hook.description
		}
		preTool = append(preTool, entry)
	}
	if len(preTool) == 0 {
		return nil
	}
	payload := nativeCodexHooksJSON{
		Version: 1,
		Hooks: nativeCodexHookTypes{
			PreToolUse: preTool,
		},
	}
	body, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	codexDir := filepath.Join(dist, ".codex")
	if err := os.MkdirAll(codexDir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(codexDir, "hooks.json"), body, 0o644)
}

func parseNativeBuildSimpleYAMLScalars(content string) map[string]string {
	values := map[string]string{}
	for _, field := range parseNativeBuildSimpleYAMLScalarFields(content) {
		values[field.key] = field.value
	}
	return values
}

func parseNativeBuildSimpleYAMLScalarFields(content string) []nativeBuildYAMLField {
	var fields []nativeBuildYAMLField
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || !strings.Contains(trimmed, ":") {
			continue
		}
		key, value, _ := strings.Cut(trimmed, ":")
		fields = setNativeBuildYAMLField(fields, strings.TrimSpace(key), unquoteNativeBuildYAML(strings.TrimSpace(value)))
	}
	return fields
}

func unquoteNativeBuildYAML(value string) string {
	if len(value) >= 2 {
		if (strings.HasPrefix(value, `"`) && strings.HasSuffix(value, `"`)) || (strings.HasPrefix(value, `'`) && strings.HasSuffix(value, `'`)) {
			if unquoted, err := strconv.Unquote(value); err == nil {
				return unquoted
			}
			return strings.Trim(value, `"'`)
		}
	}
	return value
}

func sortedNativeBuildMapKeys[T any](values map[string]T) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
