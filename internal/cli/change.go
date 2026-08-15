package cli

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// changeFolderRE bounds a Change folder name: YYYYMMDD-slug.
var changeFolderRE = regexp.MustCompile(`^(\d{8})-([a-z0-9]+(?:-[a-z0-9]+)*)$`)

// changeNode is the git-canonical portion of a materialized Change. It is
// deliberately derived from retained files; no lineage state is persisted.
// Layout is "new" when change.json is present, else "legacy" (change.md).
type changeNode struct {
	Slug          string   `json:"slug"`
	Branch        string   `json:"branch,omitempty"`
	Created       string   `json:"created,omitempty"`
	Lineage       string   `json:"lineage,omitempty"`
	Predecessor   string   `json:"predecessor,omitempty"`
	ReleaseAfter  string   `json:"releaseAfter,omitempty"`
	TargetRelease string   `json:"targetRelease,omitempty"`
	Layout        string   `json:"layout"`
	Folder        string   `json:"folder"`
	ChangeFile    string   `json:"-"`
	ContractFile  string   `json:"-"`
	Content       string   `json:"-"`
	MetaContent   string   `json:"-"`
	ParseFindings []string `json:"-"`
	CapturedOnly  bool     `json:"-"`
}

type changeFrontmatterField struct {
	Key   string
	Value string
}

type changeFrontmatterParse struct {
	Fields    []changeFrontmatterField
	AtByteOne bool
	Findings  []string
}

func loadChangeNodes(rootPath string) ([]changeNode, error) {
	folders, err := listChangeFolderNames(rootPath)
	if err != nil {
		return nil, err
	}
	nodes := make([]changeNode, 0, len(folders))
	for _, name := range folders {
		folderAbs := filepath.Join(rootPath, "docs", "changes", name)
		node, err := assembleChangeNodeFromFolder(rootPath, folderAbs)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, node)
	}
	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].Folder == nodes[j].Folder {
			return nodes[i].ChangeFile < nodes[j].ChangeFile
		}
		return nodes[i].Folder < nodes[j].Folder
	})
	return nodes, nil
}

// changeFrontmatterFields parses the leading YAML frontmatter into ordered
// key/value fields. The second return reports whether frontmatter opens the
// file at byte one — parsers depend on it, so this is checkable on its own.
func changeFrontmatterFields(content string) ([]changeFrontmatterField, bool) {
	parsed := parseChangeFrontmatter(content)
	return parsed.Fields, parsed.AtByteOne
}

func parseChangeFrontmatter(content string) changeFrontmatterParse {
	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	if !strings.HasPrefix(normalized, "---\n") {
		return changeFrontmatterParse{Fields: []changeFrontmatterField{}, Findings: []string{}}
	}
	result := changeFrontmatterParse{Fields: []changeFrontmatterField{}, AtByteOne: true, Findings: []string{}}
	lines := strings.Split(normalized, "\n")
	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			end = i
			break
		}
	}
	if end < 0 {
		result.Findings = append(result.Findings, "frontmatter is not closed with ---")
		return result
	}
	for index, line := range lines[1:end] {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		key, value, ok := strings.Cut(trimmed, ":")
		if !ok {
			result.Findings = append(result.Findings, fmt.Sprintf("malformed frontmatter line %d: expected key: value", index+2))
			continue
		}
		key = strings.TrimSpace(key)
		if key == "" {
			result.Findings = append(result.Findings, fmt.Sprintf("malformed frontmatter line %d: key cannot be empty", index+2))
			continue
		}
		result.Fields = append(result.Fields, changeFrontmatterField{
			Key:   key,
			Value: cleanChangeScalar(strings.TrimSpace(value)),
		})
	}
	return result
}

func changeFieldValue(fields []changeFrontmatterField, key string) string {
	for _, field := range fields {
		if strings.EqualFold(field.Key, key) {
			return field.Value
		}
	}
	return ""
}

func cleanChangeScalar(value string) string {
	if len(value) >= 2 {
		if (value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'') {
			return value[1 : len(value)-1]
		}
	}
	return value
}

func prefixChangeFinding(changePath, finding string) string {
	if changePath == "" {
		return finding
	}
	return filepath.ToSlash(changePath) + ": " + finding
}

func relFromRoot(root string, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}

func countChangeFields(fields []changeFrontmatterField, key string) int {
	count := 0
	for _, field := range fields {
		if strings.EqualFold(field.Key, key) {
			count++
		}
	}
	return count
}
