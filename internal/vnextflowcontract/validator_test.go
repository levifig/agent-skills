package vnextflowcontract

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"regexp"
	"sort"
	"strings"
)

const (
	flowManifestPath = "flow-contract.json"
	flowSchema       = "loaf-flow/v1"
	trackerContract  = "project-management/v1"
)

var (
	fieldMarkerPattern  = regexp.MustCompile(`<!-- loaf:field ([a-z][a-z0-9_]*) -->`)
	placeholderPattern  = regexp.MustCompile(`\{\{([a-z][a-z0-9_]*)\}\}`)
	markdownLinkPattern = regexp.MustCompile(`\[[^]]+\]\(([^)]+)\)`)
	skillNamePattern    = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
)

type finding struct {
	Rule   string
	Path   string
	Detail string
}

func (item finding) String() string {
	return fmt.Sprintf("%s: %s: %s", item.Rule, item.Path, item.Detail)
}

type flowManifest struct {
	Schema          string             `json:"schema"`
	TrackerContract string             `json:"tracker_contract"`
	Authority       authorityContract  `json:"authority"`
	LocalWorkRecord *bool              `json:"local_work_record"`
	TrackerProxy    *bool              `json:"tracker_proxy"`
	TrackerSync     *bool              `json:"tracker_sync"`
	Skills          []skillDeclaration `json:"skills"`
	Ceremonies      []ceremonyContract `json:"ceremonies"`
	Templates       []templateContract `json:"templates"`
	Agents          []agentDeclaration `json:"agents"`
}

type authorityContract struct {
	SharedWork         string `json:"shared_work"`
	ServiceConnections string `json:"service_connections"`
	FlowCeremonies     string `json:"flow_ceremonies"`
	Code               string `json:"code"`
}

type skillDeclaration struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Kind string `json:"kind"`
}

type templateContract struct {
	ID             string   `json:"id"`
	Path           string   `json:"path"`
	RequiredFields []string `json:"required_fields"`
}

type ceremonyContract struct {
	Name              string   `json:"name"`
	Skill             string   `json:"skill"`
	Input             string   `json:"input"`
	Output            string   `json:"output"`
	Template          string   `json:"template"`
	TrackerOperations []string `json:"tracker_operations"`
}

type agentDeclaration struct {
	Name         string `json:"name"`
	Path         string `json:"path"`
	ContractPath string `json:"contract_path"`
}

type frontMatter struct {
	Name        string
	Description string
}

func validateFlowContract(content fs.FS) []finding {
	manifest := flowManifest{}
	if err := decodeStrictJSON(content, flowManifestPath, &manifest); err != nil {
		return []finding{{Rule: "flow.manifest", Path: flowManifestPath, Detail: err.Error()}}
	}

	var findings []finding
	findings = append(findings, validateManifestIdentity(manifest)...)
	findings = append(findings, validateSkillDeclarations(content, manifest.Skills)...)
	findings = append(findings, validateCeremonyDeclarations(manifest)...)
	findings = append(findings, validateTemplateContracts(content, manifest.Templates)...)
	findings = append(findings, validateAgentDeclarations(content, manifest.Agents)...)
	findings = append(findings, validateMarkdownLinks(content)...)
	findings = append(findings, validateForbiddenSurfaces(content)...)
	sortFindings(findings)
	return findings
}

func validateManifestIdentity(manifest flowManifest) []finding {
	var findings []finding
	if manifest.Schema != flowSchema {
		findings = append(findings, finding{"flow.schema", flowManifestPath, fmt.Sprintf("schema = %q, want %q", manifest.Schema, flowSchema)})
	}
	if manifest.TrackerContract != trackerContract {
		findings = append(findings, finding{"flow.tracker-contract", flowManifestPath, fmt.Sprintf("tracker_contract = %q, want %q", manifest.TrackerContract, trackerContract)})
	}
	wantAuthority := authorityContract{
		SharedWork:         "tracker",
		ServiceConnections: "harness",
		FlowCeremonies:     "loaf",
		Code:               "git",
	}
	if manifest.Authority != wantAuthority {
		findings = append(findings, finding{"flow.authority", flowManifestPath, fmt.Sprintf("authority = %+v, want %+v", manifest.Authority, wantAuthority)})
	}
	if manifest.LocalWorkRecord == nil {
		findings = append(findings, finding{"flow.local-authority", flowManifestPath, "local_work_record must be explicit"})
	} else if *manifest.LocalWorkRecord {
		findings = append(findings, finding{"flow.local-authority", flowManifestPath, "local_work_record must be false"})
	}
	if manifest.TrackerProxy == nil {
		findings = append(findings, finding{"flow.tracker-proxy", flowManifestPath, "tracker_proxy must be explicit"})
	} else if *manifest.TrackerProxy {
		findings = append(findings, finding{"flow.tracker-proxy", flowManifestPath, "tracker_proxy must be false"})
	}
	if manifest.TrackerSync == nil {
		findings = append(findings, finding{"flow.tracker-sync", flowManifestPath, "tracker_sync must be explicit"})
	} else if *manifest.TrackerSync {
		findings = append(findings, finding{"flow.tracker-sync", flowManifestPath, "tracker_sync must be false"})
	}
	if manifest.Skills == nil {
		findings = append(findings, finding{"flow.inventory", flowManifestPath, "skills must be an explicit array"})
	}
	if manifest.Templates == nil {
		findings = append(findings, finding{"flow.inventory", flowManifestPath, "templates must be an explicit array"})
	}
	if manifest.Ceremonies == nil {
		findings = append(findings, finding{"flow.inventory", flowManifestPath, "ceremonies must be an explicit array"})
	}
	if manifest.Agents == nil {
		findings = append(findings, finding{"flow.inventory", flowManifestPath, "agents must be an explicit array"})
	}
	return findings
}

func validateSkillDeclarations(content fs.FS, declarations []skillDeclaration) []finding {
	var findings []finding
	declared := make(map[string]struct{}, len(declarations))
	for _, declaration := range declarations {
		findings = append(findings, validateSkillDirectory(content, declaration)...)
		if !skillNamePattern.MatchString(declaration.Name) || len(declaration.Name) > 64 {
			findings = append(findings, finding{"skill.name", declaration.Path, fmt.Sprintf("invalid skill name %q", declaration.Name)})
		}
		if _, exists := declared[declaration.Name]; exists {
			findings = append(findings, finding{"skill.inventory", declaration.Path, fmt.Sprintf("duplicate declaration for %q", declaration.Name)})
		}
		declared[declaration.Name] = struct{}{}

		wantPath := path.Join("skills", declaration.Name, "SKILL.md")
		if declaration.Path != wantPath {
			findings = append(findings, finding{"skill.path", declaration.Path, fmt.Sprintf("path must be %q", wantPath)})
		}
		if declaration.Kind != "reference" && declaration.Kind != "workflow" && declaration.Kind != "provider" {
			findings = append(findings, finding{"skill.kind", declaration.Path, fmt.Sprintf("unknown kind %q", declaration.Kind)})
		}

		body, err := fs.ReadFile(content, declaration.Path)
		if err != nil {
			findings = append(findings, finding{"skill.missing", declaration.Path, err.Error()})
			continue
		}
		matter, err := parseFrontMatter(string(body))
		if err != nil {
			findings = append(findings, finding{"skill.frontmatter", declaration.Path, err.Error()})
			continue
		}
		if matter.Name != declaration.Name {
			findings = append(findings, finding{"skill.frontmatter", declaration.Path, fmt.Sprintf("name = %q, want %q", matter.Name, declaration.Name)})
		}
		if len(matter.Description) == 0 || len(matter.Description) > 1024 {
			findings = append(findings, finding{"skill.description", declaration.Path, "description must contain 1-1024 characters"})
		}
		if strings.HasPrefix(matter.Description, "Use ") || !strings.Contains(matter.Description, "Use when") {
			findings = append(findings, finding{"skill.description", declaration.Path, "description must start with an action verb and include a Use when trigger"})
		}
		if declaration.Kind == "workflow" && !strings.Contains(matter.Description, "Produces") {
			findings = append(findings, finding{"skill.description", declaration.Path, "workflow description must state what it Produces"})
		}
		findings = append(findings, validateSkillSections(declaration.Path, string(body))...)
		findings = append(findings, validateReferenceInventory(content, declaration, string(body))...)
	}

	entries, err := fs.ReadDir(content, "skills")
	if err != nil {
		findings = append(findings, finding{"skill.inventory", "skills", err.Error()})
		return findings
	}
	actual := make(map[string]struct{})
	for _, entry := range entries {
		if !entry.IsDir() {
			findings = append(findings, finding{"skill.inventory", path.Join("skills", entry.Name()), "skill inventory may contain directories only"})
			continue
		}
		actual[entry.Name()] = struct{}{}
	}
	for name := range declared {
		if _, exists := actual[name]; !exists {
			findings = append(findings, finding{"skill.inventory", path.Join("skills", name), "declared skill directory is missing"})
		}
	}
	for name := range actual {
		if _, exists := declared[name]; !exists {
			findings = append(findings, finding{"skill.inventory", path.Join("skills", name), "skill is not declared in flow-contract.json"})
		}
	}
	return findings
}

func validateSkillDirectory(content fs.FS, declaration skillDeclaration) []finding {
	directory := path.Join("skills", declaration.Name)
	entries, err := fs.ReadDir(content, directory)
	if err != nil {
		return nil
	}
	allowedFiles := map[string]map[string]struct{}{
		"project-management": {"SKILL.md": {}, "contract.json": {}},
		"linear":             {"SKILL.md": {}, "capabilities.json": {}},
	}
	allowed := allowedFiles[declaration.Name]
	if allowed == nil {
		allowed = map[string]struct{}{"SKILL.md": {}}
	}
	var findings []finding
	for _, entry := range entries {
		entryPath := path.Join(directory, entry.Name())
		if entry.IsDir() {
			if entry.Name() != "references" {
				findings = append(findings, finding{"skill.surface", entryPath, "only the references directory is allowed beside SKILL.md"})
			}
			continue
		}
		if _, exists := allowed[entry.Name()]; !exists {
			findings = append(findings, finding{"skill.surface", entryPath, "undeclared skill sidecar or configuration file"})
		}
	}
	return findings
}

func validateSkillSections(filePath, body string) []finding {
	sections := []string{"## Contents", "## Critical Rules", "## Verification", "## Quick Reference", "## Topics"}
	last := -1
	var findings []finding
	for _, section := range sections {
		position := strings.Index(body, section)
		if position < 0 {
			findings = append(findings, finding{"skill.structure", filePath, fmt.Sprintf("missing %q", section)})
			continue
		}
		if position < last {
			findings = append(findings, finding{"skill.structure", filePath, fmt.Sprintf("%q is out of standard order", section)})
		}
		last = position
	}
	return findings
}

func validateReferenceInventory(content fs.FS, declaration skillDeclaration, skillBody string) []finding {
	referencesPath := path.Join("skills", declaration.Name, "references")
	entries, err := fs.ReadDir(content, referencesPath)
	if err != nil {
		if errorsIsNotExist(err) {
			return nil
		}
		return []finding{{"skill.references", referencesPath, err.Error()}}
	}

	linked := make(map[string]struct{})
	for _, match := range markdownLinkPattern.FindAllStringSubmatch(skillBody, -1) {
		target := strings.SplitN(match[1], "#", 2)[0]
		resolved := path.Clean(path.Join(path.Dir(declaration.Path), target))
		linked[resolved] = struct{}{}
	}
	var findings []finding
	for _, entry := range entries {
		entryPath := path.Join(referencesPath, entry.Name())
		if entry.IsDir() || path.Ext(entry.Name()) != ".md" {
			findings = append(findings, finding{"skill.references", entryPath, "references must be one level of Markdown files"})
			continue
		}
		if _, exists := linked[entryPath]; !exists {
			findings = append(findings, finding{"skill.references", entryPath, "reference is not linked from SKILL.md"})
		}
	}
	return findings
}

func validateTemplateContracts(content fs.FS, contracts []templateContract) []finding {
	want := []templateContract{
		{ID: "problem-narrative", Path: "templates/problem-narrative.md", RequiredFields: []string{"problem", "affected_people", "current_reality", "desired_outcome", "constraints", "unknowns", "handoff"}},
		{ID: "work-contract", Path: "templates/work-contract.md", RequiredFields: []string{"title", "problem", "definition_of_done", "out_of_scope", "relationships", "verification", "risks"}},
		{ID: "tracker-update", Path: "templates/tracker-update.md", RequiredFields: []string{"native_ref", "summary", "evidence", "status_intent", "blockers", "next_step"}},
	}
	var findings []finding
	if len(contracts) != len(want) {
		findings = append(findings, finding{"template.inventory", flowManifestPath, fmt.Sprintf("templates has %d entries, want %d", len(contracts), len(want))})
	}
	for index, expected := range want {
		if index >= len(contracts) {
			continue
		}
		actual := contracts[index]
		if actual.ID != expected.ID || actual.Path != expected.Path || !equalStrings(actual.RequiredFields, expected.RequiredFields) {
			findings = append(findings, finding{"template.contract", actual.Path, fmt.Sprintf("contract = %+v, want %+v", actual, expected)})
		}
		body, err := fs.ReadFile(content, actual.Path)
		if err != nil {
			findings = append(findings, finding{"template.missing", actual.Path, err.Error()})
			continue
		}
		findings = append(findings, validateTemplateFields(actual, string(body))...)
	}
	declaredPaths := make(map[string]struct{}, len(contracts))
	for _, contract := range contracts {
		declaredPaths[contract.Path] = struct{}{}
	}
	entries, err := fs.ReadDir(content, "templates")
	if err != nil {
		return append(findings, finding{"template.inventory", "templates", err.Error()})
	}
	for _, entry := range entries {
		entryPath := path.Join("templates", entry.Name())
		if entry.IsDir() || path.Ext(entry.Name()) != ".md" {
			findings = append(findings, finding{"template.inventory", entryPath, "template inventory may contain Markdown files only"})
			continue
		}
		if _, exists := declaredPaths[entryPath]; !exists {
			findings = append(findings, finding{"template.inventory", entryPath, "template is not declared in flow-contract.json"})
		}
	}
	return findings
}

func validateTemplateFields(contract templateContract, body string) []finding {
	markers := captureValues(fieldMarkerPattern, body)
	placeholders := captureValues(placeholderPattern, body)
	var findings []finding
	findings = append(findings, compareFieldInventory("template.marker", contract.Path, markers, contract.RequiredFields)...)
	findings = append(findings, compareFieldInventory("template.placeholder", contract.Path, placeholders, contract.RequiredFields)...)
	return findings
}

func compareFieldInventory(rule, filePath string, actual, expected []string) []finding {
	counts := make(map[string]int)
	for _, value := range actual {
		counts[value]++
	}
	var findings []finding
	for _, field := range expected {
		if counts[field] == 0 {
			findings = append(findings, finding{rule, filePath, fmt.Sprintf("missing field %q", field)})
		}
		if counts[field] > 1 {
			findings = append(findings, finding{rule, filePath, fmt.Sprintf("field %q appears %d times", field, counts[field])})
		}
		delete(counts, field)
	}
	for field := range counts {
		findings = append(findings, finding{rule, filePath, fmt.Sprintf("orphan field %q", field)})
	}
	return findings
}

func validateAgentDeclarations(content fs.FS, declarations []agentDeclaration) []finding {
	declared := make(map[string]struct{}, len(declarations))
	var findings []finding
	for _, declaration := range declarations {
		if !skillNamePattern.MatchString(declaration.Name) {
			findings = append(findings, finding{"agent.name", declaration.Path, fmt.Sprintf("invalid agent name %q", declaration.Name)})
		}
		if _, exists := declared[declaration.Name]; exists {
			findings = append(findings, finding{"agent.inventory", declaration.Path, fmt.Sprintf("duplicate declaration for %q", declaration.Name)})
		}
		declared[declaration.Name] = struct{}{}
		if declaration.Path != path.Join("agents", declaration.Name+".md") {
			findings = append(findings, finding{"agent.path", declaration.Path, "agent path must match its name"})
		}
		if _, err := fs.ReadFile(content, declaration.Path); err != nil {
			findings = append(findings, finding{"agent.missing", declaration.Path, err.Error()})
		}
		if declaration.ContractPath == "" {
			findings = append(findings, finding{"agent.contract", declaration.Path, "contract_path is required"})
		} else if _, err := fs.ReadFile(content, declaration.ContractPath); err != nil {
			findings = append(findings, finding{"agent.contract", declaration.ContractPath, err.Error()})
		}
	}

	entries, err := fs.ReadDir(content, "agents")
	if err != nil {
		if errorsIsNotExist(err) && len(declarations) == 0 {
			return findings
		}
		return append(findings, finding{"agent.inventory", "agents", err.Error()})
	}
	actual := make(map[string]struct{})
	for _, entry := range entries {
		if entry.IsDir() || path.Ext(entry.Name()) != ".md" {
			continue
		}
		actual[strings.TrimSuffix(entry.Name(), ".md")] = struct{}{}
	}
	for name := range actual {
		if _, exists := declared[name]; !exists {
			findings = append(findings, finding{"agent.inventory", path.Join("agents", name+".md"), "agent is not declared in flow-contract.json"})
		}
	}
	return findings
}

func validateMarkdownLinks(content fs.FS) []finding {
	var findings []finding
	_ = fs.WalkDir(content, ".", func(filePath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			findings = append(findings, finding{"content.walk", filePath, walkErr.Error()})
			return nil
		}
		if entry.IsDir() || path.Ext(entry.Name()) != ".md" {
			return nil
		}
		body, err := fs.ReadFile(content, filePath)
		if err != nil {
			findings = append(findings, finding{"content.read", filePath, err.Error()})
			return nil
		}
		for _, match := range markdownLinkPattern.FindAllStringSubmatch(string(body), -1) {
			target := strings.SplitN(strings.TrimSpace(match[1]), "#", 2)[0]
			if target == "" || strings.Contains(target, "://") || strings.HasPrefix(target, "mailto:") {
				continue
			}
			resolved := path.Clean(path.Join(path.Dir(filePath), target))
			if resolved == ".." || strings.HasPrefix(resolved, "../") {
				findings = append(findings, finding{"content.link", filePath, fmt.Sprintf("link escapes content root: %q", target)})
				continue
			}
			if _, err := fs.Stat(content, resolved); err != nil {
				findings = append(findings, finding{"content.link", filePath, fmt.Sprintf("broken link %q: %v", target, err)})
			}
		}
		return nil
	})
	return findings
}

func validateForbiddenSurfaces(content fs.FS) []finding {
	forbidden := []string{
		"loaf issue new",
		"loaf issue render",
		"loaf issue start",
		"loaf issue verify",
		"loaf issue pull",
		"loaf issue push",
		"loaf issue reconcile",
		"linear_api_key",
		"linearclientfromenv",
		".agents/loaf.json",
	}
	var findings []finding
	_ = fs.WalkDir(content, ".", func(filePath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() {
			return nil
		}
		extension := path.Ext(entry.Name())
		if extension != ".md" && extension != ".json" {
			return nil
		}
		body, err := fs.ReadFile(content, filePath)
		if err != nil {
			return nil
		}
		lowerBody := strings.ToLower(string(body))
		for _, phrase := range forbidden {
			if strings.Contains(lowerBody, phrase) {
				findings = append(findings, finding{"flow.forbidden-surface", filePath, fmt.Sprintf("contains forbidden legacy or credential surface %q", phrase)})
			}
		}
		return nil
	})
	return findings
}

func parseFrontMatter(body string) (frontMatter, error) {
	lines := strings.Split(body, "\n")
	if len(lines) < 4 || lines[0] != "---" {
		return frontMatter{}, fmt.Errorf("missing opening frontmatter delimiter")
	}
	values := make(map[string]string)
	closing := -1
	for index := 1; index < len(lines); index++ {
		line := lines[index]
		if line == "---" {
			closing = index
			break
		}
		key, value, found := strings.Cut(line, ":")
		if !found || strings.TrimSpace(key) == "" || strings.TrimSpace(value) == "" {
			return frontMatter{}, fmt.Errorf("frontmatter line %d must be a non-empty key/value pair", index+1)
		}
		key = strings.TrimSpace(key)
		if key != "name" && key != "description" {
			return frontMatter{}, fmt.Errorf("unsupported frontmatter field %q", key)
		}
		if _, exists := values[key]; exists {
			return frontMatter{}, fmt.Errorf("duplicate frontmatter field %q", key)
		}
		values[key] = strings.TrimSpace(value)
	}
	if closing < 0 {
		return frontMatter{}, fmt.Errorf("missing closing frontmatter delimiter")
	}
	if values["name"] == "" || values["description"] == "" {
		return frontMatter{}, fmt.Errorf("name and description are required")
	}
	return frontMatter{Name: values["name"], Description: values["description"]}, nil
}

func decodeStrictJSON(content fs.FS, filePath string, target any) error {
	body, err := fs.ReadFile(content, filePath)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple JSON values")
		}
		return fmt.Errorf("trailing JSON: %w", err)
	}
	return nil
}

func captureValues(pattern *regexp.Regexp, body string) []string {
	matches := pattern.FindAllStringSubmatch(body, -1)
	values := make([]string, 0, len(matches))
	for _, match := range matches {
		values = append(values, match[1])
	}
	return values
}

func errorsIsNotExist(err error) bool {
	return errors.Is(err, fs.ErrNotExist)
}

func equalStrings(left, right []string) bool {
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

func sortFindings(findings []finding) {
	sort.Slice(findings, func(left, right int) bool {
		if findings[left].Rule != findings[right].Rule {
			return findings[left].Rule < findings[right].Rule
		}
		if findings[left].Path != findings[right].Path {
			return findings[left].Path < findings[right].Path
		}
		return findings[left].Detail < findings[right].Detail
	})
}
