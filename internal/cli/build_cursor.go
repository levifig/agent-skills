package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type nativeBuildYAMLValue struct {
	kind    string
	scalar  string
	items   []string
	entries []nativeBuildYAMLFieldValue
}

type nativeBuildYAMLFieldValue struct {
	key   string
	value nativeBuildYAMLValue
}

type nativeCursorHooksJSON struct {
	Version int                    `json:"version"`
	Hooks   nativeCursorHookGroups `json:"hooks"`
}

type nativeCursorHookGroups struct {
	PreToolUse         []nativeCursorHookJSON `json:"preToolUse,omitempty"`
	PostToolUse        []nativeCursorHookJSON `json:"postToolUse,omitempty"`
	SessionStart       []nativeCursorHookJSON `json:"sessionStart,omitempty"`
	SessionEnd         []nativeCursorHookJSON `json:"sessionEnd,omitempty"`
	BeforeSubmitPrompt []nativeCursorHookJSON `json:"beforeSubmitPrompt,omitempty"`
	Stop               []nativeCursorHookJSON `json:"stop,omitempty"`
	PreCompact         []nativeCursorHookJSON `json:"preCompact,omitempty"`
}

type nativeCursorHookJSON struct {
	LoafManaged bool   `json:"loaf-managed"`
	Timeout     int    `json:"timeout"`
	Matcher     string `json:"matcher,omitempty"`
	FailClosed  bool   `json:"failClosed,omitempty"`
	Command     string `json:"command,omitempty"`
	Prompt      string `json:"prompt,omitempty"`
	If          string `json:"if,omitempty"`
}

func runNativeBuildCursor(root string, out io.Writer) error {
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
	fmt.Fprintf(out, "  %s cursor...", ansiCyan("building"))
	if err := buildNativeCursorTarget(root); err != nil {
		fmt.Fprintf(out, "\r  %s cursor\n", ansiRed("✗"))
		return err
	}
	fmt.Fprintf(out, "\r  %s cursor %s\n", ansiGreen("✓"), ansiGray("("+elapsedSeconds(targetStart)+")"))
	fmt.Fprintln(out)
	fmt.Fprintf(out, "%s %s\n", ansiGreen("Build complete"), ansiGray("("+elapsedSeconds(start)+")"))
	return nil
}

func buildNativeCursorTarget(root string) error {
	version, err := nativeBuildPackageVersion(root)
	if err != nil {
		return err
	}
	targetsConfig, err := readNativeBuildTargetsConfig(root)
	if err != nil {
		return err
	}
	dist := filepath.Join(root, "dist", "cursor")
	if err := os.RemoveAll(dist); err != nil {
		return err
	}
	if err := copyNativeBuildSkills(nativeBuildSkillCopyOptions{
		srcDir:        filepath.Join(root, "dist"),
		destDir:       filepath.Join(dist, "skills"),
		targetName:    "cursor",
		version:       version,
		targetsConfig: targetsConfig,
		transformMd:   substituteNativeBuildCursorCommands,
	}); err != nil {
		return err
	}
	srcDir := filepath.Join(root, "content")
	if err := copyNativeBuildAgents(srcDir, filepath.Join(dist, "agents"), "cursor", version, []nativeBuildYAMLFieldValue{
		{key: "model", value: nativeBuildStringValue("inherit")},
		{key: "is_background", value: nativeBuildBoolValue(true)},
	}, false); err != nil {
		return err
	}
	if err := copyNativeCursorHooks(filepath.Join(srcDir, "hooks"), filepath.Join(dist, "hooks")); err != nil {
		return err
	}
	return generateNativeCursorHooksJSON(filepath.Join(root, "config", "hooks.yaml"), dist)
}

func substituteNativeBuildCursorCommands(content string) string {
	replacer := strings.NewReplacer(
		"{{IMPLEMENT_CMD}}", "/implement",
		"{{RESUME_CMD}}", "/resume",
		"{{ORCHESTRATE_CMD}}", "/implement",
	)
	return replacer.Replace(content)
}

func copyNativeBuildAgents(srcDir string, destDir string, targetName string, version string, defaults []nativeBuildYAMLFieldValue, sidecarRequired bool) error {
	agentsDir := filepath.Join(srcDir, "agents")
	entries, err := os.ReadDir(agentsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return err
	}
	var files []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".md") {
			files = append(files, entry.Name())
		}
	}
	sort.Strings(files)
	for _, file := range files {
		srcPath := filepath.Join(agentsDir, file)
		body, err := os.ReadFile(srcPath)
		if err != nil {
			return err
		}
		frontmatter, content := splitNativeBuildFrontmatter(string(body))
		sourceFields := parseNativeBuildYAMLFieldValues(frontmatter)
		agentName := strings.TrimSuffix(file, ".md")
		fields := append([]nativeBuildYAMLFieldValue{}, defaults...)
		fields = setNativeBuildYAMLFieldValue(fields, "name", nativeBuildStringValue(firstNativeBuildFieldString(sourceFields, "name", agentName)))
		fields = setNativeBuildYAMLFieldValue(fields, "description", nativeBuildStringValue(firstNativeBuildFieldString(sourceFields, "description", agentName+" agent for specialized tasks")))
		sidecarPath := strings.TrimSuffix(srcPath, ".md") + "." + targetName + ".yaml"
		sidecarFields, err := readNativeBuildAgentSidecar(sidecarPath, sidecarRequired)
		if err != nil {
			return err
		}
		for _, field := range sidecarFields {
			fields = setNativeBuildYAMLFieldValue(fields, field.key, field.value)
		}
		content = strings.TrimSpace(content) + "\n\n---\nversion: " + version + "\n"
		output := "---\n" + renderNativeBuildYAMLFieldValues(fields) + "---\n" + content
		if err := os.WriteFile(filepath.Join(destDir, file), []byte(output), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func readNativeBuildAgentSidecar(path string, required bool) ([]nativeBuildYAMLFieldValue, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) && !required {
			return nil, nil
		}
		return nil, err
	}
	return parseNativeBuildYAMLFieldValues(string(body)), nil
}

func parseNativeBuildYAMLFieldValues(frontmatter string) []nativeBuildYAMLFieldValue {
	var fields []nativeBuildYAMLFieldValue
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
			fields = setNativeBuildYAMLFieldValue(fields, key, nativeBuildStringValue(value))
			continue
		}
		if value == "" {
			if i+1 < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[i+1]), "- ") {
				var items []string
				for i+1 < len(lines) && strings.HasPrefix(lines[i+1], "  ") {
					next := strings.TrimSpace(lines[i+1])
					if !strings.HasPrefix(next, "- ") {
						break
					}
					i++
					items = append(items, unquoteNativeBuildYAML(strings.TrimSpace(strings.TrimPrefix(next, "- "))))
				}
				fields = setNativeBuildYAMLFieldValue(fields, key, nativeBuildYAMLValue{kind: "list", items: items})
				continue
			}
			var children []nativeBuildYAMLFieldValue
			for i+1 < len(lines) && strings.HasPrefix(lines[i+1], "  ") && !strings.HasPrefix(lines[i+1], "    ") {
				i++
				childLine := strings.TrimSpace(lines[i])
				childKey, childValue, ok := strings.Cut(childLine, ":")
				if !ok {
					continue
				}
				children = append(children, nativeBuildYAMLFieldValue{
					key:   strings.TrimSpace(childKey),
					value: nativeBuildScalarYAMLValue(strings.TrimSpace(childValue)),
				})
			}
			fields = setNativeBuildYAMLFieldValue(fields, key, nativeBuildYAMLValue{kind: "map", entries: children})
			continue
		}
		fields = setNativeBuildYAMLFieldValue(fields, key, nativeBuildScalarYAMLValue(value))
	}
	return fields
}

func nativeBuildScalarYAMLValue(value string) nativeBuildYAMLValue {
	switch value {
	case "true":
		return nativeBuildBoolValue(true)
	case "false":
		return nativeBuildBoolValue(false)
	default:
		return nativeBuildStringValue(unquoteNativeBuildYAML(value))
	}
}

func nativeBuildStringValue(value string) nativeBuildYAMLValue {
	return nativeBuildYAMLValue{kind: "string", scalar: value}
}

func nativeBuildBoolValue(value bool) nativeBuildYAMLValue {
	if value {
		return nativeBuildYAMLValue{kind: "bool", scalar: "true"}
	}
	return nativeBuildYAMLValue{kind: "bool", scalar: "false"}
}

func setNativeBuildYAMLFieldValue(fields []nativeBuildYAMLFieldValue, key string, value nativeBuildYAMLValue) []nativeBuildYAMLFieldValue {
	for i, field := range fields {
		if field.key == key {
			fields[i].value = value
			return fields
		}
	}
	return append(fields, nativeBuildYAMLFieldValue{key: key, value: value})
}

func firstNativeBuildFieldString(fields []nativeBuildYAMLFieldValue, key string, fallback string) string {
	for _, field := range fields {
		if field.key == key && field.value.kind == "string" && field.value.scalar != "" {
			return field.value.scalar
		}
	}
	return fallback
}

func renderNativeBuildYAMLFieldValues(fields []nativeBuildYAMLFieldValue) string {
	var out strings.Builder
	for _, field := range fields {
		writeNativeBuildYAMLFieldValue(&out, field, "")
	}
	return out.String()
}

func writeNativeBuildYAMLFieldValue(out *strings.Builder, field nativeBuildYAMLFieldValue, indent string) {
	out.WriteString(indent)
	out.WriteString(field.key)
	switch field.value.kind {
	case "map":
		out.WriteString(":\n")
		for _, child := range field.value.entries {
			writeNativeBuildYAMLFieldValue(out, child, indent+"  ")
		}
	case "list":
		out.WriteString(":\n")
		for _, item := range field.value.items {
			out.WriteString(indent)
			out.WriteString("  - ")
			out.WriteString(quoteNativeBuildYAMLScalar(item))
			out.WriteByte('\n')
		}
	case "bool":
		out.WriteString(": ")
		out.WriteString(field.value.scalar)
		out.WriteByte('\n')
	default:
		value := field.value.scalar
		if shouldFoldNativeBuildYAMLValue(value) {
			out.WriteString(": >-\n")
			for _, line := range wrapNativeBuildYAMLText(value, 78) {
				out.WriteString(indent)
				out.WriteString("  ")
				out.WriteString(line)
				out.WriteByte('\n')
			}
			return
		}
		out.WriteString(": ")
		out.WriteString(quoteNativeBuildYAMLScalar(value))
		out.WriteByte('\n')
	}
}

func copyNativeCursorHooks(src string, dest string) error {
	for _, subdir := range []string{"session", "post-tool", "lib", "instructions"} {
		if err := copyNativeCursorHookFiles(filepath.Join(src, subdir), filepath.Join(dest, subdir)); err != nil {
			return err
		}
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if err := copyNativeBuildFile(filepath.Join(src, entry.Name()), filepath.Join(dest, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

func copyNativeCursorHookFiles(src string, dest string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	overrides := map[string]bool{}
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".cursor.sh") {
			overrides[strings.TrimSuffix(entry.Name(), ".cursor.sh")+".sh"] = true
		}
	}
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		if entry.IsDir() {
			if err := copyNativeCursorHookFiles(srcPath, filepath.Join(dest, entry.Name())); err != nil {
				return err
			}
			continue
		}
		destName := entry.Name()
		if strings.HasSuffix(entry.Name(), ".cursor.sh") {
			destName = strings.TrimSuffix(entry.Name(), ".cursor.sh") + ".sh"
		} else if overrides[entry.Name()] {
			continue
		}
		if err := copyNativeBuildFile(srcPath, filepath.Join(dest, destName)); err != nil {
			return err
		}
	}
	return nil
}

func copyNativeBuildFile(src string, dest string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return nil
	}
	body, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dest, body, info.Mode().Perm())
}

func generateNativeCursorHooksJSON(hooksPath string, dist string) error {
	hooks, err := readNativeBuildHooks(hooksPath)
	if err != nil {
		return err
	}
	var payload nativeCursorHooksJSON
	payload.Version = 1
	for _, hook := range hooks {
		switch hook.section {
		case "pre-tool":
			payload.Hooks.PreToolUse = append(payload.Hooks.PreToolUse, nativeCursorHookEntry(hook, 60000, true))
		case "post-tool":
			payload.Hooks.PostToolUse = append(payload.Hooks.PostToolUse, nativeCursorHookEntry(hook, 30000, true))
		case "session":
			entry := nativeCursorHookEntry(hook, 60000, false)
			switch nativeCursorSessionEvent(hook.event) {
			case "sessionStart":
				payload.Hooks.SessionStart = append(payload.Hooks.SessionStart, entry)
			case "sessionEnd":
				payload.Hooks.SessionEnd = append(payload.Hooks.SessionEnd, entry)
			case "beforeSubmitPrompt":
				payload.Hooks.BeforeSubmitPrompt = append(payload.Hooks.BeforeSubmitPrompt, entry)
			case "stop":
				payload.Hooks.Stop = append(payload.Hooks.Stop, entry)
			case "preCompact":
				payload.Hooks.PreCompact = append(payload.Hooks.PreCompact, entry)
			}
		}
	}
	body, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dist, "hooks.json"), body, 0o644)
}

func nativeCursorHookEntry(hook nativeBuildHook, defaultTimeout int, includeMatcher bool) nativeCursorHookJSON {
	entry := nativeCursorHookJSON{
		LoafManaged: true,
		Timeout:     nativeBuildSecondsTimeout(hook.timeout, defaultTimeout),
	}
	if includeMatcher && hook.matcher != "" {
		entry.Matcher = hook.matcher
	}
	if hook.failClosed {
		entry.FailClosed = true
	}
	if hook.typeName == "prompt" {
		entry.Prompt = hook.prompt
	} else {
		entry.Command = nativeCursorHookCommand(hook)
	}
	entry.If = hook.ifCondition
	return entry
}

func nativeBuildSecondsTimeout(value int, fallback int) int {
	if value == 0 {
		value = fallback
	}
	return value / 1000
}

func nativeCursorHookCommand(hook nativeBuildHook) string {
	if hook.instruction != "" {
		return `cat "$HOME/.cursor/hooks/` + hook.instruction + `"`
	}
	if nativeBuildCursorBinaryPathHooks[hook.id] {
		if hook.command == "" {
			return "loaf check --hook " + hook.id
		}
		return hook.command
	}
	if hook.command != "" {
		return hook.command
	}
	script := strings.TrimPrefix(hook.script, "hooks/")
	base := "$HOME/.cursor/hooks"
	switch {
	case strings.HasSuffix(hook.script, ".py"):
		return "python3 " + base + "/" + script
	case strings.HasSuffix(hook.script, ".ts"):
		return "bun run " + base + "/" + script
	default:
		return "bash " + base + "/" + script
	}
}

var nativeBuildCursorBinaryPathHooks = map[string]bool{
	"check-" + "sec" + "rets": true,
	"validate-push":           true,
	"validate-commit":         true,
	"workflow-pre-pr":         true,
	"security-audit":          true,
	"session-start-loaf":      true,
	"session-end-loaf":        true,
	"journal-post-commit":     true,
	"journal-post-pr":         true,
	"journal-post-merge":      true,
}

func nativeCursorSessionEvent(event string) string {
	switch event {
	case "SessionStart":
		return "sessionStart"
	case "SessionEnd":
		return "sessionEnd"
	case "PreCompact":
		return "preCompact"
	case "UserPromptSubmit":
		return "beforeSubmitPrompt"
	case "Stop":
		return "stop"
	default:
		return ""
	}
}

var _ fs.DirEntry
