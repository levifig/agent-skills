package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/levifig/loaf/internal/project"
)

// MarkdownMigrationPlan is the read-only preview for importing .agents files.
type MarkdownMigrationPlan struct {
	ContractVersion int                         `json:"contract_version"`
	AgentsPath      string                      `json:"agents_path"`
	Specs           int                         `json:"specs"`
	Tasks           int                         `json:"tasks"`
	Ideas           int                         `json:"ideas"`
	Sparks          int                         `json:"sparks"`
	Brainstorms     int                         `json:"brainstorms"`
	ShapingDrafts   int                         `json:"shaping_drafts"`
	Sessions        int                         `json:"sessions"`
	Reports         int                         `json:"reports"`
	Relationships   int                         `json:"relationships"`
	SkippedFiles    []string                    `json:"skipped_files"`
	UnimportedFiles []MarkdownMigrationFileNote `json:"unimported_files"`
	IgnoredFiles    []MarkdownMigrationFileNote `json:"ignored_files"`
	Warnings        []string                    `json:"warnings"`
}

// MarkdownMigrationFileNote explains why a .agents file is absent from the
// SQLite import plan.
type MarkdownMigrationFileNote struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

// MarkdownMigrationPreviewResult is the CLI-facing dry-run envelope for a
// Markdown import preview. It does not imply initialized SQLite state.
type MarkdownMigrationPreviewResult struct {
	MarkdownMigrationPlan
	DatabaseScope      string `json:"database_scope"`
	ImportScope        string `json:"import_scope"`
	DatabasePath       string `json:"database_path"`
	ProjectName        string `json:"project_name"`
	ProjectCurrentPath string `json:"project_current_path"`
	Applied            bool   `json:"applied"`
}

// NewMarkdownMigrationPreviewResult adds global DB and project context to a
// read-only Markdown migration preview without creating SQLite state.
func NewMarkdownMigrationPreviewResult(plan MarkdownMigrationPlan, root project.Root, databasePath string) MarkdownMigrationPreviewResult {
	return MarkdownMigrationPreviewResult{
		MarkdownMigrationPlan: plan,
		DatabaseScope:         "global",
		ImportScope:           "project",
		DatabasePath:          databasePath,
		ProjectName:           filepath.Base(root.Path()),
		ProjectCurrentPath:    root.Path(),
		Applied:               false,
	}
}

// PreviewMarkdownMigration inspects .agents without mutating files or SQLite state.
func PreviewMarkdownMigration(root project.Root) (MarkdownMigrationPlan, error) {
	agentsPath := filepath.Join(root.Path(), ".agents")
	plan := MarkdownMigrationPlan{
		ContractVersion: StateJSONContractVersion,
		AgentsPath:      agentsPath,
		SkippedFiles:    []string{},
		UnimportedFiles: []MarkdownMigrationFileNote{},
		IgnoredFiles:    []MarkdownMigrationFileNote{},
		Warnings:        []string{},
	}

	info, err := os.Stat(agentsPath)
	if os.IsNotExist(err) {
		plan.Warnings = append(plan.Warnings, ".agents directory not found")
		return plan, nil
	}
	if err != nil {
		return plan, fmt.Errorf("inspect .agents: %w", err)
	}
	if !info.IsDir() {
		return plan, fmt.Errorf(".agents is not a directory: %s", agentsPath)
	}

	countKnownMarkdownFiles(agentsPath, "specs", &plan.Specs)
	countKnownMarkdownFiles(agentsPath, "tasks", &plan.Tasks)
	countKnownMarkdownFiles(agentsPath, "ideas", &plan.Ideas)
	countKnownMarkdownFilesRecursive(agentsPath, "sessions", &plan.Sessions)
	countKnownMarkdownFilesRecursive(agentsPath, "reports", &plan.Reports)
	countBrainstorms(agentsPath, &plan.Brainstorms)
	countShapingDrafts(agentsPath, &plan.ShapingDrafts)
	countSparks(agentsPath, &plan.Sparks)
	countRelationships(agentsPath, &plan.Relationships, &plan.Warnings)

	disposition, err := classifySkippedFiles(agentsPath)
	if err != nil {
		return plan, err
	}
	plan.SkippedFiles = disposition.skippedPaths()
	plan.UnimportedFiles = disposition.Unimported
	plan.IgnoredFiles = disposition.Ignored
	return plan, nil
}

func countKnownMarkdownFiles(agentsPath string, subdir string, count *int) {
	files, err := filepath.Glob(filepath.Join(agentsPath, subdir, "*.md"))
	if err != nil {
		return
	}
	*count = len(files)
}

func countKnownMarkdownFilesRecursive(agentsPath string, subdir string, count *int) {
	countKnownMarkdownFiles(agentsPath, subdir, count)
	files, err := filepath.Glob(filepath.Join(agentsPath, subdir, "archive", "*.md"))
	if err != nil {
		return
	}
	*count += len(files)
}

func countBrainstorms(agentsPath string, count *int) {
	files, err := filepath.Glob(filepath.Join(agentsPath, "drafts", "*brainstorm*.md"))
	if err != nil {
		return
	}
	*count = len(files)
}

func countShapingDrafts(agentsPath string, count *int) {
	files, err := filepath.Glob(filepath.Join(agentsPath, "drafts", "*.md"))
	if err != nil {
		return
	}
	for _, path := range files {
		content, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		rel, err := filepath.Rel(agentsPath, path)
		if err != nil {
			continue
		}
		rel = filepath.ToSlash(filepath.Join(".agents", rel))
		if isShapingDraftArtifact(rel, parseFrontmatterMap(content)) {
			*count++
		}
	}
}

func countSparks(agentsPath string, count *int) {
	re := regexp.MustCompile(`(?m)\bspark\([^)]+\):`)
	sessionFiles, err := filepath.Glob(filepath.Join(agentsPath, "sessions", "*.md"))
	if err != nil {
		return
	}
	for _, path := range sessionFiles {
		content, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		*count += len(re.FindAll(content, -1))
	}
}

func countRelationships(agentsPath string, count *int, warnings *[]string) {
	indexPath := filepath.Join(agentsPath, "TASKS.json")
	content, err := os.ReadFile(indexPath)
	if err != nil {
		*count += countTaskDependencyLines(agentsPath)
		*count += countArtifactFrontmatterRelationships(agentsPath)
		return
	}

	var index struct {
		Tasks map[string]struct {
			Spec      string   `json:"spec"`
			DependsOn []string `json:"depends_on"`
		} `json:"tasks"`
	}
	if err := json.Unmarshal(content, &index); err != nil {
		*warnings = append(*warnings, fmt.Sprintf("could not parse .agents/TASKS.json: %v", err))
		*count += countTaskDependencyLines(agentsPath)
		*count += countArtifactFrontmatterRelationships(agentsPath)
		return
	}
	for _, task := range index.Tasks {
		if task.Spec != "" {
			*count++
		}
		*count += len(task.DependsOn)
	}
	*count += countArtifactFrontmatterRelationships(agentsPath)
}

func countTaskDependencyLines(agentsPath string) int {
	files, err := filepath.Glob(filepath.Join(agentsPath, "tasks", "*.md"))
	if err != nil {
		return 0
	}
	total := 0
	for _, path := range files {
		content, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		total += countTaskFrontmatterRelationships(content)
	}
	return total
}

func countArtifactFrontmatterRelationships(agentsPath string) int {
	total := 0
	_ = filepath.WalkDir(agentsPath, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		if !isKnownAgentsFile(agentsPath, path) {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		frontmatter := parseFrontmatterMap(content)
		for _, field := range relationshipFrontmatterFields() {
			total += len(splitFrontmatterList(frontmatter[field]))
		}
		return nil
	})
	return total
}

func countTaskFrontmatterRelationships(content []byte) int {
	frontmatter := markdownFrontmatter(content)
	if frontmatter == "" {
		return 0
	}

	total := 0
	inDependsOn := false
	taskID := regexp.MustCompile(`\bTASK-\d+\b`)
	for _, line := range strings.Split(frontmatter, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "spec:") && strings.TrimSpace(strings.TrimPrefix(trimmed, "spec:")) != "" {
			total++
			inDependsOn = false
			continue
		}
		if strings.HasPrefix(trimmed, "depends_on:") {
			inDependsOn = true
			total += len(taskID.FindAllString(strings.TrimPrefix(trimmed, "depends_on:"), -1))
			continue
		}
		if inDependsOn {
			if strings.HasPrefix(trimmed, "-") {
				total += len(taskID.FindAllString(trimmed, -1))
				continue
			}
			if trimmed != "" && !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
				inDependsOn = false
			}
		}
	}
	return total
}

func markdownFrontmatter(content []byte) string {
	lines := strings.Split(string(content), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return ""
	}
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			return strings.Join(lines[1:i], "\n")
		}
	}
	return ""
}

type skippedFileDisposition struct {
	Skipped    []string
	Unimported []MarkdownMigrationFileNote
	Ignored    []MarkdownMigrationFileNote
}

func (d skippedFileDisposition) skippedPaths() []string {
	return d.Skipped
}

func classifySkippedFiles(agentsPath string) (skippedFileDisposition, error) {
	disposition := skippedFileDisposition{
		Skipped:    []string{},
		Unimported: []MarkdownMigrationFileNote{},
		Ignored:    []MarkdownMigrationFileNote{},
	}
	err := filepath.WalkDir(agentsPath, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if isKnownAgentsFile(agentsPath, path) {
			return nil
		}
		rel, err := filepath.Rel(agentsPath, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		note := MarkdownMigrationFileNote{
			Path:   filepath.ToSlash(filepath.Join(".agents", rel)),
			Reason: skippedFileReason(rel),
		}
		disposition.Skipped = append(disposition.Skipped, note.Path)
		if isIgnoredMigrationFile(rel) {
			disposition.Ignored = append(disposition.Ignored, note)
			return nil
		}
		disposition.Unimported = append(disposition.Unimported, note)
		return nil
	})
	return disposition, err
}

func isIgnoredMigrationFile(rel string) bool {
	name := filepath.Base(rel)
	if name == ".DS_Store" || name == ".gitkeep" || rel == ".loaf-state" {
		return true
	}
	if strings.HasPrefix(rel, "tmp/") {
		return true
	}
	return false
}

func skippedFileReason(rel string) string {
	name := filepath.Base(rel)
	switch {
	case name == ".DS_Store":
		return "macOS metadata"
	case name == ".gitkeep":
		return "directory placeholder"
	case rel == ".loaf-state":
		return "legacy local state marker"
	case strings.HasPrefix(rel, "tmp/"):
		return "temporary enrichment artifact"
	case strings.HasPrefix(rel, "reports/") && !strings.HasSuffix(rel, ".md"):
		return "unsupported report support file; only Markdown reports are imported"
	case strings.HasPrefix(rel, "councils/") && strings.HasSuffix(rel, ".md"):
		return "unsupported artifact kind: council"
	case strings.HasPrefix(rel, "handoffs/") && strings.HasSuffix(rel, ".md"):
		return "unsupported artifact kind: handoff"
	case strings.HasPrefix(rel, "plans/") && strings.HasSuffix(rel, ".md"):
		return "unsupported artifact kind: plan"
	case strings.HasPrefix(rel, "drafts/") && strings.HasSuffix(rel, ".md"):
		return "draft is not classified as brainstorm or shaping draft"
	case strings.HasPrefix(rel, "skills/"):
		return "project-local skill override is not imported into SQLite state"
	case strings.HasSuffix(rel, ".md"):
		return "unsupported Markdown artifact kind"
	default:
		return "unsupported file type"
	}
}

func isKnownAgentsFile(agentsPath string, path string) bool {
	rel, err := filepath.Rel(agentsPath, path)
	if err != nil {
		return false
	}
	rel = filepath.ToSlash(rel)
	if rel == "TASKS.json" || rel == "AGENTS.md" || rel == "SOUL.md" || rel == "loaf.json" {
		return true
	}
	if strings.HasPrefix(rel, "specs/") && strings.HasSuffix(rel, ".md") {
		return true
	}
	if strings.HasPrefix(rel, "tasks/") && strings.HasSuffix(rel, ".md") {
		return true
	}
	if strings.HasPrefix(rel, "ideas/") && strings.HasSuffix(rel, ".md") {
		return true
	}
	if strings.HasPrefix(rel, "sessions/") && strings.HasSuffix(rel, ".md") {
		return true
	}
	if strings.HasPrefix(rel, "reports/") && strings.HasSuffix(rel, ".md") {
		return true
	}
	if strings.HasPrefix(rel, "drafts/") && strings.Contains(rel, "brainstorm") && strings.HasSuffix(rel, ".md") {
		return true
	}
	if strings.HasPrefix(rel, "drafts/") && strings.HasSuffix(rel, ".md") {
		content, err := os.ReadFile(path)
		if err != nil {
			return false
		}
		return isShapingDraftArtifact(filepath.ToSlash(filepath.Join(".agents", rel)), parseFrontmatterMap(content))
	}
	return false
}
