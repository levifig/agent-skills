package state

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
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

// PreviewMarkdownMigration inspects .agents without mutating files or SQLite state.
// Doctor and inventory-only dry-run use this cheap path; simulation uses
// SimulateMarkdownMigration instead.
func PreviewMarkdownMigration(root project.Root) (MarkdownMigrationPlan, error) {
	return inventoryMarkdownMigration(root)
}

const markdownInventoryOnlyWarning = "inventory-only: conflict detection requires an initialized SQLite project; no import_report"

// SimulateMarkdownMigration runs dry-run dispatch: when the database exists and
// the project is registered, it snapshots the live DB and runs the full apply
// pipeline against the snapshot; otherwise it returns an honestly labeled
// inventory-only plan with no import_report. Simulation performs no Inspect
// gate — behind-schema databases surface the same typed RequireCurrentSchema
// error as apply.
func SimulateMarkdownMigration(ctx context.Context, root project.Root, resolver PathResolver) (MarkdownMigrationResult, error) {
	livePath, err := resolver.DatabasePath(root)
	if err != nil {
		return MarkdownMigrationResult{}, err
	}

	info, err := os.Stat(livePath)
	if os.IsNotExist(err) {
		return inventoryMarkdownMigrationResult(root, livePath)
	}
	if err != nil {
		return MarkdownMigrationResult{}, fmt.Errorf("stat state database: %w", err)
	}
	if info.IsDir() {
		return MarkdownMigrationResult{}, fmt.Errorf("state database path is a directory: %s", livePath)
	}

	registered, err := markdownMigrationProjectRegistered(ctx, livePath, root)
	if err != nil {
		return MarkdownMigrationResult{}, err
	}
	if !registered {
		return inventoryMarkdownMigrationResult(root, livePath)
	}

	snapshotPath, cleanup, err := createMarkdownSimulationSnapshot(ctx, livePath)
	if err != nil {
		return MarkdownMigrationResult{}, err
	}
	defer cleanup()

	result, err := ApplyMarkdownMigration(ctx, root, PathResolver{DatabaseFile: snapshotPath})
	if err != nil {
		return MarkdownMigrationResult{}, err
	}
	result.Applied = false
	result.Action = MarkdownMigrationActionSimulate
	result.Mode = MarkdownMigrationModeSimulation
	result.DatabasePath = livePath
	return result, nil
}

func markdownMigrationProjectRegistered(ctx context.Context, databasePath string, root project.Root) (bool, error) {
	store, err := OpenStoreReadOnly(databasePath)
	if err != nil {
		return false, fmt.Errorf("open state database for simulation dispatch: %w", err)
	}
	defer store.Close()

	_, err = store.LookupProjectIdentityForRoot(ctx, root)
	if err == nil {
		return true, nil
	}
	var unregistered *UnregisteredProjectIdentityError
	if errors.As(err, &unregistered) || errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	// Pre-0003 databases lack project_paths. Treat as unregistered so dry-run
	// falls back to inventory — no Inspect gate (Decision 3 / Decision 8).
	if isMissingProjectIdentitySchema(err) {
		return false, nil
	}
	return false, err
}

func inventoryMarkdownMigrationResult(root project.Root, databasePath string) (MarkdownMigrationResult, error) {
	plan, err := inventoryMarkdownMigration(root)
	if err != nil {
		return MarkdownMigrationResult{}, err
	}
	hasWarning := false
	for _, warning := range plan.Warnings {
		if warning == markdownInventoryOnlyWarning {
			hasWarning = true
			break
		}
	}
	if !hasWarning {
		plan.Warnings = append(plan.Warnings, markdownInventoryOnlyWarning)
	}
	return MarkdownMigrationResult{
		MarkdownMigrationPlan: plan,
		DatabaseScope:         "global",
		ImportScope:           "project",
		DatabasePath:          databasePath,
		ProjectName:           filepath.Base(root.Path()),
		ProjectCurrentPath:    root.Path(),
		Action:                MarkdownMigrationActionSimulate,
		Mode:                  MarkdownMigrationModeInventory,
		Applied:               false,
	}, nil
}

// inventoryMarkdownMigration builds the file-inventory plan shared by preview
// and apply. Apply must not call PreviewMarkdownMigration; it calls this helper
// before opening the import transaction.
func inventoryMarkdownMigration(root project.Root) (MarkdownMigrationPlan, error) {
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

	countKnownMarkdownFiles(agentsPath, "specs", &plan.Specs, &plan.Warnings)
	countKnownMarkdownFiles(agentsPath, "tasks", &plan.Tasks, &plan.Warnings)
	countKnownMarkdownFiles(agentsPath, "ideas", &plan.Ideas, &plan.Warnings)
	countKnownMarkdownFilesRecursive(agentsPath, "sessions", &plan.Sessions, &plan.Warnings)
	countKnownMarkdownFilesRecursive(agentsPath, "reports", &plan.Reports, &plan.Warnings)
	countBrainstorms(agentsPath, &plan.Brainstorms, &plan.Warnings)
	countShapingDrafts(agentsPath, &plan.ShapingDrafts, &plan.Warnings)
	countSparks(agentsPath, &plan.Sparks, &plan.Warnings)
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

func countKnownMarkdownFiles(agentsPath string, subdir string, count *int, warnings *[]string) {
	files, err := filepath.Glob(filepath.Join(agentsPath, subdir, "*.md"))
	if err != nil {
		*warnings = append(*warnings, fmt.Sprintf("could not list .agents/%s Markdown files: %v", subdir, err))
		return
	}
	*count = len(files)
}

func countKnownMarkdownFilesRecursive(agentsPath string, subdir string, count *int, warnings *[]string) {
	countKnownMarkdownFiles(agentsPath, subdir, count, warnings)
	files, err := filepath.Glob(filepath.Join(agentsPath, subdir, "archive", "*.md"))
	if err != nil {
		*warnings = append(*warnings, fmt.Sprintf("could not list .agents/%s/archive Markdown files: %v", subdir, err))
		return
	}
	*count += len(files)
}

func countBrainstorms(agentsPath string, count *int, warnings *[]string) {
	files, err := filepath.Glob(filepath.Join(agentsPath, "drafts", "*brainstorm*.md"))
	if err != nil {
		*warnings = append(*warnings, fmt.Sprintf("could not list brainstorm drafts: %v", err))
		return
	}
	*count = len(files)
}

func countShapingDrafts(agentsPath string, count *int, warnings *[]string) {
	files, err := filepath.Glob(filepath.Join(agentsPath, "drafts", "*.md"))
	if err != nil {
		*warnings = append(*warnings, fmt.Sprintf("could not list shaping drafts: %v", err))
		return
	}
	for _, path := range files {
		content, err := os.ReadFile(path)
		if err != nil {
			*warnings = append(*warnings, fmt.Sprintf("could not read %s for shaping-draft count: %v", inventoryAgentsRel(agentsPath, path), err))
			continue
		}
		rel, err := filepath.Rel(agentsPath, path)
		if err != nil {
			*warnings = append(*warnings, fmt.Sprintf("could not relativize shaping draft path %s: %v", path, err))
			continue
		}
		rel = filepath.ToSlash(filepath.Join(".agents", rel))
		if isShapingDraftArtifact(rel, parseFrontmatterMap(content)) {
			*count++
		}
	}
}

func countSparks(agentsPath string, count *int, warnings *[]string) {
	re := regexp.MustCompile(`(?m)\bspark\([^)]+\):`)
	sessionFiles, err := filepath.Glob(filepath.Join(agentsPath, "sessions", "*.md"))
	if err != nil {
		*warnings = append(*warnings, fmt.Sprintf("could not list session files for spark count: %v", err))
		return
	}
	archiveFiles, err := filepath.Glob(filepath.Join(agentsPath, "sessions", "archive", "*.md"))
	if err != nil {
		*warnings = append(*warnings, fmt.Sprintf("could not list archived session files for spark count: %v", err))
		archiveFiles = nil
	}
	for _, path := range append(sessionFiles, archiveFiles...) {
		content, err := os.ReadFile(path)
		if err != nil {
			*warnings = append(*warnings, fmt.Sprintf("could not read %s for spark count: %v", inventoryAgentsRel(agentsPath, path), err))
			continue
		}
		*count += len(re.FindAll(content, -1))
	}
}

func inventoryAgentsRel(agentsPath, path string) string {
	rel, err := filepath.Rel(agentsPath, path)
	if err != nil {
		return path
	}
	return filepath.ToSlash(filepath.Join(".agents", rel))
}

func countRelationships(agentsPath string, count *int, warnings *[]string) {
	indexPath := filepath.Join(agentsPath, "TASKS.json")
	content, err := os.ReadFile(indexPath)
	if err != nil {
		*count += countTaskDependencyLines(agentsPath, warnings)
		*count += countArtifactFrontmatterRelationships(agentsPath, warnings)
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
		*count += countTaskDependencyLines(agentsPath, warnings)
		*count += countArtifactFrontmatterRelationships(agentsPath, warnings)
		return
	}
	for _, task := range index.Tasks {
		if task.Spec != "" {
			*count++
		}
		*count += len(task.DependsOn)
	}
	*count += countArtifactFrontmatterRelationships(agentsPath, warnings)
}

func countTaskDependencyLines(agentsPath string, warnings *[]string) int {
	files, err := filepath.Glob(filepath.Join(agentsPath, "tasks", "*.md"))
	if err != nil {
		*warnings = append(*warnings, fmt.Sprintf("could not list .agents/tasks Markdown files for relationship count: %v", err))
		return 0
	}
	total := 0
	for _, path := range files {
		content, err := os.ReadFile(path)
		if err != nil {
			*warnings = append(*warnings, fmt.Sprintf("could not read %s for relationship count: %v", inventoryAgentsRel(agentsPath, path), err))
			continue
		}
		total += countTaskFrontmatterRelationships(content)
	}
	return total
}

func countArtifactFrontmatterRelationships(agentsPath string, warnings *[]string) int {
	total := 0
	walkErr := filepath.WalkDir(agentsPath, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			*warnings = append(*warnings, fmt.Sprintf("could not walk %s for relationship count: %v", inventoryAgentsRel(agentsPath, path), err))
			return nil
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		if !isKnownAgentsFile(agentsPath, path) {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			*warnings = append(*warnings, fmt.Sprintf("could not read %s for relationship count: %v", inventoryAgentsRel(agentsPath, path), err))
			return nil
		}
		frontmatter := parseFrontmatterMap(content)
		for _, field := range relationshipFrontmatterFields() {
			total += len(splitFrontmatterList(frontmatter[field]))
		}
		return nil
	})
	if walkErr != nil {
		*warnings = append(*warnings, fmt.Sprintf("could not walk .agents for relationship count: %v", walkErr))
	}
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
