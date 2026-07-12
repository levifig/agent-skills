package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
)

const (
	codexJournalRuleRelativePath         = "loaf.rules"
	codexJournalRuleTemplateRelativePath = "loaf.rules.tmpl"
	codexJournalGuidanceRelativePath     = "AGENTS.md"
	codexJournalRuleManifest             = ".loaf-managed-rules.json"
	codexJournalExecutablePlaceholder    = "{{LOAF_EXECUTABLE}}"
	codexJournalGuidanceStart            = "<!-- loaf:managed:codex-basic-commands:start -->"
	codexJournalGuidanceEnd              = "<!-- loaf:managed:codex-basic-commands:end -->"
	codexLegacyGuidanceStart             = "<!-- loaf:managed:codex-auto-journal:start -->"
	codexLegacyGuidanceEnd               = "<!-- loaf:managed:codex-auto-journal:end -->"
	codexBasicRulesPlaceholder           = "{{LOAF_BASIC_RULES}}"
)

type codexManagedRuleManifest struct {
	Version int                    `json:"version"`
	Files   []codexManagedRuleFile `json:"files"`
}

type codexManagedRuleFile struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type codexRuleInstallOperations struct {
	lookPath       func(string) (string, error)
	forbiddenRoots []string
}

func installCodexJournalRule(options targetInstallOptions, codexHome string) error {
	return installCodexJournalRuleWithOperations(options, codexHome, nil)
}

func installCodexJournalRuleWithOperations(options targetInstallOptions, codexHome string, operations *codexRuleInstallOperations) error {
	rulesDir := filepath.Join(codexHome, "rules")
	ruleDest := filepath.Join(rulesDir, codexJournalRuleRelativePath)
	manifestPath := filepath.Join(rulesDir, codexJournalRuleManifest)
	guidanceDest := filepath.Join(codexHome, codexJournalGuidanceRelativePath)
	templatePath := filepath.Join(options.DistDir, ".codex", "rules", codexJournalRuleTemplateRelativePath)

	manifest, err := readCodexManagedRuleManifest(manifestPath)
	if err != nil {
		return err
	}
	ownedRuleSHA, ownedRule := manifest.ownedDigest(codexJournalRuleRelativePath)
	ownedGuidanceSHA, ownedGuidance := manifest.ownedDigest(codexJournalGuidanceRelativePath)
	legacyCapability, err := detectLegacyCodexJournalCapability(ruleDest, guidanceDest)
	if err != nil {
		return err
	}
	if options.Upgrade && !options.CodexBasicCommands && legacyCapability {
		if ownedRule || ownedGuidance {
			return retireCodexJournalCapability(ruleDest, guidanceDest, manifestPath, manifest, ownedRule, ownedRuleSHA, ownedGuidanceSHA, ownedGuidance)
		}
		return fmt.Errorf("legacy Codex journal-only capability requires explicit --codex-basic-commands or recorded Loaf ownership before upgrade")
	}

	templateBody, templateErr := os.ReadFile(templatePath)
	templateExists := templateErr == nil
	if templateErr != nil && !os.IsNotExist(templateErr) {
		return fmt.Errorf("read generated Codex journal rule template: %w", templateErr)
	}
	// A stale owned install can be retired without resolving the current
	// executable. This is important when PATH has been removed or rebound.
	if !templateExists {
		if options.CodexBasicCommands {
			return fmt.Errorf("generated Codex journal rule template is missing at %s", templatePath)
		}
		if options.Upgrade && (ownedRule || ownedGuidance) {
			return retireCodexJournalCapability(ruleDest, guidanceDest, manifestPath, manifest, ownedRule, ownedRuleSHA, ownedGuidanceSHA, ownedGuidance)
		}
		return nil
	}

	needsConvergence := options.CodexBasicCommands || (options.Upgrade && (ownedRule || ownedGuidance))
	if !needsConvergence {
		return nil
	}
	executable, err := trustedCodexJournalExecutable(options.ProjectRoot, operations)
	if err != nil {
		return err
	}
	renderedRule, err := renderCodexJournalRule(string(templateBody), executable)
	if err != nil {
		return err
	}
	guidanceBlock := generateCodexJournalGuidance(executable)
	newRuleSHA := sha256Bytes([]byte(renderedRule))
	newGuidanceSHA := sha256Bytes([]byte(guidanceBlock))

	currentRule, ruleExists, err := readOptionalInstallFile(ruleDest, "installed Codex journal rule")
	if err != nil {
		return err
	}
	if ruleExists {
		currentSHA := sha256Bytes(currentRule)
		switch {
		case ownedRule && currentSHA == ownedRuleSHA:
			// Expected old managed content; converge below.
		case ownedRule && currentSHA == newRuleSHA:
			// An interrupted rule write can be adopted and recorded below.
		case !ownedRule && options.CodexBasicCommands && currentSHA == newRuleSHA:
			// An interrupted first install can be adopted and recorded below.
		case !ownedRule && options.CodexBasicCommands:
			return fmt.Errorf("refusing to overwrite unowned Codex rule %s", ruleDest)
		case ownedRule:
			return fmt.Errorf("refusing to overwrite modified Loaf-owned Codex rule %s", ruleDest)
		default:
			return nil
		}
	}

	guidanceContent, guidanceExists, err := readOptionalInstallFile(guidanceDest, "Codex journal guidance")
	if err != nil {
		return err
	}
	guidanceText := string(guidanceContent)
	if err := validateCodexJournalGuidanceStructure(guidanceText); err != nil {
		return fmt.Errorf("inspect Codex journal guidance: %w", err)
	}
	guidanceRange, hasGuidance := findCodexJournalGuidance(guidanceText)
	currentGuidance := ""
	if hasGuidance {
		currentGuidance = guidanceText[guidanceRange.start:guidanceRange.end]
	}
	switch {
	case ownedGuidance && hasGuidance && sha256Bytes([]byte(currentGuidance)) == ownedGuidanceSHA:
		// Expected old managed content; replace below.
	case ownedGuidance && hasGuidance && sha256Bytes([]byte(currentGuidance)) == newGuidanceSHA:
		// An interrupted guidance write can be adopted and recorded below.
	case !ownedGuidance && hasGuidance && currentGuidance == guidanceBlock:
		// An interrupted first install can be adopted and recorded below.
	case ownedGuidance:
		return fmt.Errorf("refusing to overwrite modified Loaf-owned Codex guidance block in %s", guidanceDest)
	case hasGuidance:
		return fmt.Errorf("refusing to overwrite unowned Codex guidance block in %s", guidanceDest)
	}

	updatedGuidance := guidanceText
	guidanceChanged := false
	switch {
	case hasGuidance:
		if currentGuidance != guidanceBlock {
			updatedGuidance = replaceCodexJournalGuidance(guidanceText, guidanceRange, guidanceBlock)
			guidanceChanged = true
		}
	case guidanceExists:
		updatedGuidance = appendCodexJournalGuidance(guidanceText, guidanceBlock)
		guidanceChanged = true
	default:
		updatedGuidance = guidanceBlock
		guidanceChanged = true
	}

	// Publish model guidance before enabling the outside-sandbox rule. An
	// interrupted first install may leave harmless guidance, never a hidden
	// active capability.
	if guidanceChanged {
		if err := writeFileAtomically(guidanceDest, []byte(updatedGuidance), 0o644); err != nil {
			return fmt.Errorf("install Codex journal guidance: %w", err)
		}
	}
	if !ruleExists || string(currentRule) != renderedRule {
		if err := writeFileAtomically(ruleDest, []byte(renderedRule), 0o644); err != nil {
			return fmt.Errorf("install Codex journal rule: %w", err)
		}
	}
	manifest.set(codexJournalRuleRelativePath, newRuleSHA)
	manifest.set(codexJournalGuidanceRelativePath, newGuidanceSHA)
	if err := writeCodexManagedRuleManifest(manifestPath, manifest); err != nil {
		return fmt.Errorf("record Codex journal rule ownership: %w", err)
	}
	return nil
}

func detectLegacyCodexJournalCapability(rulePath string, guidancePath string) (bool, error) {
	guidance, guidanceExists, err := readOptionalInstallFile(guidancePath, "Codex journal guidance")
	if err != nil {
		return false, err
	}
	if guidanceExists {
		guidanceText := string(guidance)
		if strings.Contains(guidanceText, codexJournalGuidanceStart) || strings.Contains(guidanceText, codexJournalGuidanceEnd) {
			return false, nil
		}
		if strings.Contains(guidanceText, codexLegacyGuidanceStart) || strings.Contains(guidanceText, codexLegacyGuidanceEnd) {
			return true, nil
		}
	}
	rule, ruleExists, err := readOptionalInstallFile(rulePath, "installed Codex journal rule")
	if err != nil {
		return false, err
	}
	if ruleExists && strings.Count(string(rule), "prefix_rule(") == 1 && strings.Contains(string(rule), `"journal", "log", "--execpolicy-safe"`) {
		return true, nil
	}
	return false, nil
}

func readOptionalInstallFile(path string, description string) ([]byte, bool, error) {
	body, err := os.ReadFile(path)
	if err == nil {
		return body, true, nil
	}
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	return nil, false, fmt.Errorf("read %s: %w", description, err)
}

func retireCodexJournalCapability(ruleDest string, guidanceDest string, manifestPath string, manifest codexManagedRuleManifest, ownedRule bool, ownedRuleSHA string, ownedGuidanceSHA string, ownedGuidance bool) error {
	if ownedRule {
		currentRule, ruleExists, err := readOptionalInstallFile(ruleDest, "installed Codex journal rule")
		if err != nil {
			return err
		}
		if ruleExists && sha256Bytes(currentRule) != ownedRuleSHA {
			return fmt.Errorf("refusing to remove modified Loaf-owned Codex rule %s", ruleDest)
		}
		// Retire the exact owned rule before inspecting guidance. If guidance is
		// malformed or locally modified, the capability is still inactive.
		if ruleExists {
			if err := os.Remove(ruleDest); err != nil {
				return fmt.Errorf("remove stale Codex journal rule: %w", err)
			}
		}
		manifest.remove(codexJournalRuleRelativePath)
		if err := writeCodexManagedRuleManifest(manifestPath, manifest); err != nil {
			return fmt.Errorf("record retired Codex journal rule ownership: %w", err)
		}
	}
	guidanceContent, _, err := readOptionalInstallFile(guidanceDest, "Codex journal guidance")
	if err != nil {
		return err
	}
	guidanceText := string(guidanceContent)
	if err := validateCodexJournalGuidanceStructure(guidanceText); err != nil {
		return fmt.Errorf("inspect Codex journal guidance: %w", err)
	}
	guidanceRange, hasGuidance := findCodexJournalGuidance(guidanceText)
	if hasGuidance && !ownedGuidance {
		return fmt.Errorf("refusing to remove Codex journal guidance without recorded ownership in %s", guidanceDest)
	}
	if ownedGuidance {
		if !hasGuidance || sha256Bytes([]byte(guidanceText[guidanceRange.start:guidanceRange.end])) != ownedGuidanceSHA {
			return fmt.Errorf("refusing to remove modified Loaf-owned Codex guidance block in %s", guidanceDest)
		}
	}
	if hasGuidance && ownedGuidance {
		updated := removeCodexJournalGuidance(guidanceText, guidanceRange)
		if err := writeFileAtomically(guidanceDest, []byte(updated), 0o644); err != nil {
			return fmt.Errorf("remove stale Codex journal guidance: %w", err)
		}
	}
	manifest.remove(codexJournalRuleRelativePath)
	manifest.remove(codexJournalGuidanceRelativePath)
	return writeCodexManagedRuleManifest(manifestPath, manifest)
}

func renderCodexJournalRule(template string, executable string) (string, error) {
	if !filepath.IsAbs(executable) {
		return "", fmt.Errorf("cannot render Codex journal rule with non-absolute Loaf executable %q", executable)
	}
	rendered := template
	if strings.Contains(rendered, codexBasicRulesPlaceholder) {
		rendered = strings.ReplaceAll(rendered, codexBasicRulesPlaceholder, renderCodexBasicRules(executable))
	}
	if strings.Contains(rendered, codexJournalExecutablePlaceholder) {
		rendered = strings.ReplaceAll(rendered, codexJournalExecutablePlaceholder, strconv.Quote(executable))
	}
	if rendered == template {
		return "", fmt.Errorf("Codex journal rule template is missing %s or %s", codexBasicRulesPlaceholder, codexJournalExecutablePlaceholder)
	}
	if strings.Contains(rendered, codexBasicRulesPlaceholder) || strings.Contains(rendered, codexJournalExecutablePlaceholder) {
		return "", fmt.Errorf("Codex journal rule template contains an unresolved placeholder")
	}
	return rendered, nil
}

func renderCodexBasicRules(executable string) string {
	var rendered strings.Builder
	for _, prefix := range BasicCommandAuthorityPrefixes() {
		rendered.WriteString("prefix_rule(\n    pattern = [")
		rendered.WriteString(strconv.Quote(executable))
		for _, token := range prefix {
			rendered.WriteString(", ")
			rendered.WriteString(strconv.Quote(token))
		}
		rendered.WriteString("],\n    decision = \"allow\",\n)\n")
	}
	return rendered.String()
}

func generateCodexJournalGuidance(executable string) string {
	return "\n" + strings.Join([]string{
		codexJournalGuidanceStart,
		"<!-- Maintained by loaf install/upgrade - do not edit manually -->",
		"## Loaf Codex basic command policy",
		"",
		"When Codex Auto mode records a durable decision, use this exact command; do not substitute a bare `loaf` command or a shell/environment wrapper:",
		"",
		"`" + journalContextShellQuote(executable) + " journal log --execpolicy-safe \"type(scope): description\"`",
		"",
		"The rule permits only explicitly classified basic Loaf command leaves, including routine state-plane operations and approved readers. It does not authorize unclassified or operator commands, a bare Loaf namespace, or a general Loaf data-directory writable root. Other harness adapters are not implied by this Codex policy.",
		codexJournalGuidanceEnd,
	}, "\n") + "\n"
}

type codexJournalGuidanceRange struct{ start, end int }

func findCodexJournalGuidance(content string) (codexJournalGuidanceRange, bool) {
	startMarker := codexJournalGuidanceStart
	endMarker := codexJournalGuidanceEnd
	markerStart := strings.Index(content, startMarker)
	if markerStart < 0 {
		startMarker = codexLegacyGuidanceStart
		endMarker = codexLegacyGuidanceEnd
		markerStart = strings.Index(content, startMarker)
	}
	if markerStart < 0 {
		return codexJournalGuidanceRange{}, false
	}
	start := markerStart
	if start > 0 && content[start-1] == '\n' {
		start--
	}
	endStart := strings.Index(content[markerStart+len(startMarker):], endMarker)
	if endStart < 0 {
		return codexJournalGuidanceRange{}, false
	}
	end := markerStart + len(startMarker) + endStart + len(endMarker)
	if end < len(content) && content[end] == '\n' {
		end++
	}
	return codexJournalGuidanceRange{start: start, end: end}, true
}

func validateCodexJournalGuidanceStructure(content string) error {
	starts := strings.Count(content, codexJournalGuidanceStart) + strings.Count(content, codexLegacyGuidanceStart)
	ends := strings.Count(content, codexJournalGuidanceEnd) + strings.Count(content, codexLegacyGuidanceEnd)
	if starts == 0 && ends == 0 {
		return nil
	}
	if starts != 1 || ends != 1 {
		return fmt.Errorf("expected exactly one complete Loaf-managed block, found %d starts and %d ends", starts, ends)
	}
	startMarker := codexJournalGuidanceStart
	endMarker := codexJournalGuidanceEnd
	start := strings.Index(content, startMarker)
	if start < 0 {
		startMarker = codexLegacyGuidanceStart
		endMarker = codexLegacyGuidanceEnd
		start = strings.Index(content, startMarker)
	}
	end := strings.Index(content, endMarker)
	if start < 0 || end < start+len(startMarker) {
		return fmt.Errorf("Loaf-managed guidance end marker precedes its start marker")
	}
	return nil
}

func appendCodexJournalGuidance(content string, block string) string {
	return content + block
}

func replaceCodexJournalGuidance(content string, r codexJournalGuidanceRange, block string) string {
	return content[:r.start] + block + content[r.end:]
}

func removeCodexJournalGuidance(content string, r codexJournalGuidanceRange) string {
	return content[:r.start] + content[r.end:]
}

func currentSHAEquals(body []byte, want string) bool { return sha256Bytes(body) == want }

func sha256Bytes(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func (m codexManagedRuleManifest) ownedDigest(path string) (string, bool) {
	for _, file := range m.Files {
		if file.Path == path {
			return file.SHA256, true
		}
	}
	return "", false
}

func (m *codexManagedRuleManifest) set(path string, digest string) {
	for i := range m.Files {
		if m.Files[i].Path == path {
			m.Files[i].SHA256 = digest
			return
		}
	}
	m.Files = append(m.Files, codexManagedRuleFile{Path: path, SHA256: digest})
	sort.Slice(m.Files, func(i, j int) bool { return m.Files[i].Path < m.Files[j].Path })
}

func (m *codexManagedRuleManifest) remove(path string) {
	files := m.Files[:0]
	for _, file := range m.Files {
		if file.Path != path {
			files = append(files, file)
		}
	}
	m.Files = files
}

func readCodexManagedRuleManifest(path string) (codexManagedRuleManifest, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return codexManagedRuleManifest{Version: 1}, nil
		}
		return codexManagedRuleManifest{}, fmt.Errorf("read Codex rule ownership manifest: %w", err)
	}
	var manifest codexManagedRuleManifest
	if err := json.Unmarshal(body, &manifest); err != nil {
		return codexManagedRuleManifest{}, fmt.Errorf("decode Codex rule ownership manifest: %w", err)
	}
	if manifest.Version != 1 {
		return codexManagedRuleManifest{}, fmt.Errorf("unsupported Codex rule ownership manifest version %d", manifest.Version)
	}
	seen := make(map[string]bool, len(manifest.Files))
	for _, file := range manifest.Files {
		if file.Path == "" || filepath.Base(file.Path) != file.Path || filepath.IsAbs(file.Path) || file.Path != filepath.Clean(file.Path) || file.SHA256 == "" {
			return codexManagedRuleManifest{}, fmt.Errorf("invalid Codex rule ownership manifest path or digest %q", file.Path)
		}
		if seen[file.Path] {
			return codexManagedRuleManifest{}, fmt.Errorf("duplicate Codex rule ownership manifest path %q", file.Path)
		}
		seen[file.Path] = true
		if _, err := hex.DecodeString(file.SHA256); err != nil || len(file.SHA256) != sha256.Size*2 {
			return codexManagedRuleManifest{}, fmt.Errorf("invalid Codex rule ownership digest for %q", file.Path)
		}
	}
	return manifest, nil
}

func writeCodexManagedRuleManifest(path string, manifest codexManagedRuleManifest) error {
	if manifest.Version == 0 {
		manifest.Version = 1
	}
	sort.Slice(manifest.Files, func(i, j int) bool { return manifest.Files[i].Path < manifest.Files[j].Path })
	body, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return writeFileAtomically(path, body, 0o644)
}

func writeFileAtomically(path string, body []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".loaf-install-*")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Chmod(mode); err != nil {
		temp.Close()
		return err
	}
	if _, err := temp.Write(body); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(tempPath, path)
}

func validateCodexJournalExecutable(projectRoot string) error {
	_, err := trustedCodexJournalExecutable(projectRoot, nil)
	return err
}

func trustedCodexJournalExecutable(projectRoot string, operations *codexRuleInstallOperations) (string, error) {
	if projectRoot == "" {
		projectRoot, _ = os.Getwd()
	}
	lookPath := exec.LookPath
	forbiddenRoots := codexJournalForbiddenExecutableRoots(projectRoot)
	if operations != nil {
		if operations.lookPath != nil {
			lookPath = operations.lookPath
		}
		if operations.forbiddenRoots != nil {
			forbiddenRoots = operations.forbiddenRoots
		}
	}
	path, err := lookPath("loaf")
	if err != nil {
		return "", fmt.Errorf("cannot trust Codex basic command policy: loaf executable is not on PATH: %w", err)
	}
	canonical, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", fmt.Errorf("cannot trust Codex basic command policy: resolve loaf executable %s: %w", path, err)
	}
	canonical, err = filepath.Abs(canonical)
	if err != nil {
		return "", fmt.Errorf("cannot trust Codex basic command policy: canonicalize loaf executable: %w", err)
	}
	if strings.ContainsAny(canonical, "`\r\n") {
		return "", fmt.Errorf("cannot trust Codex basic command policy: executable path contains unsupported guidance characters")
	}
	for _, forbidden := range forbiddenRoots {
		if forbidden == "" {
			continue
		}
		forbiddenPath, resolveErr := filepath.EvalSymlinks(forbidden)
		if resolveErr != nil {
			forbiddenPath = forbidden
		}
		forbiddenPath, resolveErr = filepath.Abs(forbiddenPath)
		if resolveErr != nil {
			continue
		}
		if pathWithinInstall(forbiddenPath, canonical) {
			return "", fmt.Errorf("cannot trust Codex basic command policy: loaf executable %s is inside forbidden path %s", canonical, forbiddenPath)
		}
	}
	return canonical, nil
}

func codexJournalForbiddenExecutableRoots(projectRoot string) []string {
	roots := []string{projectRoot, os.TempDir(), "/tmp", "/private/tmp", "/var/tmp"}
	if runtime.GOOS == "darwin" {
		roots = append(roots, "/var/folders", "/private/var/folders")
	}
	if runtime.GOOS == "linux" {
		roots = append(roots, "/dev/shm", "/run/user", "/run/lock", "/run/shm")
		if runtimeDir := os.Getenv("XDG_RUNTIME_DIR"); filepath.IsAbs(runtimeDir) {
			roots = append(roots, runtimeDir)
		}
	}
	return roots
}

func pathWithinInstall(parent string, child string) bool {
	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}
