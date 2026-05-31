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
	AgentsPath    string   `json:"agents_path"`
	Specs         int      `json:"specs"`
	Tasks         int      `json:"tasks"`
	Ideas         int      `json:"ideas"`
	Sparks        int      `json:"sparks"`
	Brainstorms   int      `json:"brainstorms"`
	ShapingDrafts int      `json:"shaping_drafts"`
	Sessions      int      `json:"sessions"`
	Reports       int      `json:"reports"`
	Relationships int      `json:"relationships"`
	SkippedFiles  []string `json:"skipped_files"`
	Warnings      []string `json:"warnings"`
}

// PreviewMarkdownMigration inspects .agents without mutating files or SQLite state.
func PreviewMarkdownMigration(root project.Root) (MarkdownMigrationPlan, error) {
	agentsPath := filepath.Join(root.Path(), ".agents")
	plan := MarkdownMigrationPlan{
		AgentsPath:   agentsPath,
		SkippedFiles: []string{},
		Warnings:     []string{},
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

	skipped, err := skippedFiles(agentsPath)
	if err != nil {
		return plan, err
	}
	plan.SkippedFiles = skipped
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

func skippedFiles(agentsPath string) ([]string, error) {
	skipped := []string{}
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
		skipped = append(skipped, filepath.ToSlash(filepath.Join(".agents", rel)))
		return nil
	})
	return skipped, err
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
