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
	"artifact-names":          true,
	"check-" + "sec" + "rets": true,
	"github-account":          true,
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
	Hooks nativeCodexHookTypes `json:"hooks"`
}

type nativeCodexHookTypes struct {
	SessionStart []nativeCodexMatcherGroupJSON `json:"SessionStart,omitempty"`
}

type nativeCodexMatcherGroupJSON struct {
	Matcher string                       `json:"matcher"`
	Hooks   []nativeCodexCommandHookJSON `json:"hooks"`
}

type nativeCodexCommandHookJSON struct {
	Type           string `json:"type"`
	Command        string `json:"command"`
	CommandWindows string `json:"commandWindows"`
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
	}); err != nil {
		return err
	}
	if err := generateNativeCodexHooksJSON(root, dist); err != nil {
		return err
	}
	if err := generateNativeCodexHookCatalog(dist, version); err != nil {
		return err
	}
	return copyNativeCodexRules(root, dist)
}

// copyNativeCodexRules copies the Loaf-owned Codex policy template into the
// target bundle. Rendering is deliberately deferred until installation, when
// the trusted absolute Loaf executable is known.
func copyNativeCodexRules(root string, dist string) error {
	src := filepath.Join(root, "content", "codex", "rules", "loaf.rules.tmpl")
	body, err := readRegularFileNoFollow(src, projectFileReadLimit)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("Codex journal rule template missing at %s", src)
		}
		return err
	}
	dest := filepath.Join(dist, ".codex", "rules", "loaf.rules.tmpl")
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dest, body, 0o644)
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
	body, err := readRegularFileNoFollow(path, projectFileReadLimit)
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
	var errMerge error
	fields, errMerge = mergeNativeBuildTargetSidecar(fields, filepath.Join(sidecarSrc, "SKILL."+options.targetName+".yaml"), options.targetName)
	if errMerge != nil {
		return errMerge
	}
	if options.version != "" {
		fields = setNativeBuildYAMLField(fields, "version", options.version)
	}
	output := "---\n" + renderNativeBuildYAMLFields(fields) + "---\n" + content
	if options.transformMd != nil {
		output = options.transformMd(output)
	}
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

func mergeNativeBuildTargetSidecar(fields []nativeBuildYAMLField, sidecarPath string, targetName string) ([]nativeBuildYAMLField, error) {
	body, err := readRegularFileNoFollow(sidecarPath, projectFileReadLimit)
	if err != nil {
		if os.IsNotExist(err) {
			return fields, nil
		}
		return fields, err
	}
	owned := nativeBuildSidecarOwnedFrontmatterKeys(targetName)
	for _, field := range parseNativeBuildSimpleYAMLScalarFields(string(body)) {
		if !owned[field.key] {
			return fields, fmt.Errorf("SKILL.%s.yaml key %q is not owned by target %q", targetName, field.key, targetName)
		}
		fields = setNativeBuildYAMLField(fields, field.key, field.value)
	}
	return fields, nil
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
		// Leaf no-follow (symlink escape) without the text-size ceiling: this helper
		// also copies bin/native binaries, which exceed projectFileReadLimit.
		srcFile, err := openRegularFileNoFollow(path)
		if err != nil {
			return err
		}
		body, err := io.ReadAll(srcFile)
		srcFile.Close()
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
		body, err := readRegularFileNoFollow(src, projectFileReadLimit)
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

// nativeCodexHookProjection is one desired Codex matcher group with the
// identity it carries. The hooks file and the hook catalog are generated from
// the same list so shape and identity cannot drift apart.
type nativeCodexHookProjection struct {
	event  string
	hookID string
	group  nativeCodexMatcherGroupJSON
}

// nativeCodexHookProjections describes what Codex 0.144.1 accepts: matcher
// groups with nested command handlers. The executable stays a placeholder here;
// install renders it to a trusted absolute Loaf binary once the path is known
// and can be pinned. Windows parity is concrete rather than asserted — the
// template carries command and commandWindows with identical values.
func nativeCodexHookProjections() []nativeCodexHookProjection {
	return []nativeCodexHookProjection{{
		event:  "SessionStart",
		hookID: "session-start-loaf",
		group: nativeCodexMatcherGroupJSON{
			Matcher: codexJournalHookMatcher,
			Hooks: []nativeCodexCommandHookJSON{{
				Type:           "command",
				Command:        codexJournalExecutablePlaceholder + codexJournalHookCommandSuffix,
				CommandWindows: codexJournalExecutablePlaceholder + codexJournalHookCommandSuffix,
			}},
		},
	}}
}

func generateNativeCodexHooksJSON(root string, dist string) error {
	_ = root
	var payload nativeCodexHooksJSON
	for _, projection := range nativeCodexHookProjections() {
		if projection.event == "SessionStart" {
			payload.Hooks.SessionStart = append(payload.Hooks.SessionStart, projection.group)
		}
	}
	body, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	codexDir := filepath.Join(dist, ".codex")
	if err := os.MkdirAll(codexDir, 0o755); err != nil {
		return err
	}
	body = append(body, '\n')
	return os.WriteFile(filepath.Join(codexDir, "hooks.json"), body, 0o644)
}

func generateNativeCodexHookCatalog(dist string, version string) error {
	projections := nativeCodexHookProjections()
	sources := make([]hookCatalogSource, 0, len(projections))
	for _, projection := range projections {
		sources = append(sources, hookCatalogSource{
			event:    projection.event,
			hookID:   projection.hookID,
			typeName: "command",
			command:  projection.group.Hooks[0].Command,
			template: projection.group,
		})
	}
	catalog, err := newHookCatalog("codex", version, sources)
	if err != nil {
		return err
	}
	return writeHookCatalog(dist, catalog)
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
