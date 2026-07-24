package state

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/levifig/loaf/internal/project"
)

// MarkdownMigrationResult is the structured result for a markdown migration
// apply or dry-run (simulate / inventory).
type MarkdownMigrationResult struct {
	MarkdownMigrationPlan
	ImportReport         *ImportReport `json:"import_report,omitempty"`
	DatabaseScope        string        `json:"database_scope"`
	ImportScope          string        `json:"import_scope"`
	DatabasePath         string        `json:"database_path"`
	ProjectID            string        `json:"project_id"`
	ProjectName          string        `json:"project_name"`
	ProjectCurrentPath   string        `json:"project_current_path"`
	Action               string        `json:"action"`
	Mode                 string        `json:"mode,omitempty"`
	Applied              bool          `json:"applied"`
	BackupPath           string        `json:"backup_path,omitempty"`
	RollbackManifestPath string        `json:"rollback_manifest_path,omitempty"`
	AgentsBackupPath     string        `json:"agents_backup_path,omitempty"`
	RemovedSourceFiles   []string      `json:"removed_source_files,omitempty"`
}

const (
	MarkdownMigrationActionApply    = "apply"
	MarkdownMigrationActionResume   = "resume"
	MarkdownMigrationActionSimulate = "simulate"

	MarkdownMigrationModeSimulation = "simulation"
	MarkdownMigrationModeInventory  = "inventory"
)

type taskIndexEntry struct {
	Title     string   `json:"title"`
	Spec      string   `json:"spec"`
	Status    string   `json:"status"`
	Priority  string   `json:"priority"`
	DependsOn []string `json:"depends_on"`
	File      string   `json:"file"`
}

type specIndexEntry struct {
	Title  string `json:"title"`
	Status string `json:"status"`
}

const frontmatterListSeparator = "\x1f"

type markdownImporter struct {
	tx           *sql.Tx
	root         project.Root
	projectID    string
	now          string
	taskIndex    map[string]taskIndexEntry
	specIndex    map[string]specIndexEntry
	sparkAliases map[string]string
	// report accumulates in-transaction provenance/status outcomes.
	// Shared pointer so value-receiver methods can mutate the same report.
	report *ImportReport
}

// markdownApplyOperations contains per-invocation seams used by deterministic
// tests. Public callers pass nil; no mutable process-global hooks are involved.
type markdownApplyOperations struct {
	// afterImportCommitted runs after ImportMarkdown commits successfully.
	// Errors degrade to plan.Warnings so Applied stays true.
	afterImportCommitted func(root project.Root) error
}

// ApplyMarkdownMigration initializes SQLite state and imports structurally
// knowable .agents artifacts without mutating source Markdown files.
// Inventory is computed before the import transaction opens; import_report
// comes from the committed import. Apply does not call PreviewMarkdownMigration.
// Post-commit file-access failures never fail an already-committed import —
// they degrade to plan warnings.
func ApplyMarkdownMigration(ctx context.Context, root project.Root, resolver PathResolver) (MarkdownMigrationResult, error) {
	return applyMarkdownMigration(ctx, root, resolver, nil)
}

func applyMarkdownMigration(ctx context.Context, root project.Root, resolver PathResolver, ops *markdownApplyOperations) (MarkdownMigrationResult, error) {
	status, err := Initialize(ctx, root, resolver)
	if err != nil {
		return MarkdownMigrationResult{}, err
	}

	// Inventory before opening the import transaction so a successful commit
	// cannot be reported as failure by a later .agents walk error.
	plan, err := inventoryMarkdownMigration(root)
	if err != nil {
		return MarkdownMigrationResult{}, err
	}

	store, err := OpenStore(status.DatabasePath)
	if err != nil {
		return MarkdownMigrationResult{}, err
	}
	defer store.Close()

	report, err := store.ImportMarkdown(ctx, root)
	if err != nil {
		return MarkdownMigrationResult{}, err
	}

	if ops != nil && ops.afterImportCommitted != nil {
		if afterErr := ops.afterImportCommitted(root); afterErr != nil {
			plan.Warnings = append(plan.Warnings, fmt.Sprintf("post-import inventory access failed: %v", afterErr))
		}
	}

	return MarkdownMigrationResult{
		MarkdownMigrationPlan: plan,
		ImportReport:          &report,
		DatabaseScope:         "global",
		ImportScope:           "project",
		DatabasePath:          status.DatabasePath,
		ProjectID:             status.ProjectID,
		ProjectName:           status.ProjectName,
		ProjectCurrentPath:    status.ProjectCurrentPath,
		Action:                MarkdownMigrationActionApply,
		Applied:               true,
	}, nil
}

// ImportMarkdown imports .agents artifacts into an initialized state database
// and returns the in-transaction import report.
func (s *Store) ImportMarkdown(ctx context.Context, root project.Root) (ImportReport, error) {
	return s.importMarkdown(ctx, root)
}

// importMarkdown runs the markdown import transaction and returns ImportReport.
func (s *Store) importMarkdown(ctx context.Context, root project.Root) (ImportReport, error) {
	report := emptyImportReport()
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return report, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return report, fmt.Errorf("begin markdown import transaction: %w", err)
	}
	defer tx.Rollback()

	importer := markdownImporter{
		tx:           tx,
		root:         root,
		projectID:    projectID,
		now:          time.Now().UTC().Format(time.RFC3339),
		taskIndex:    loadTaskIndex(root.Path()),
		specIndex:    loadSpecIndex(root.Path()),
		sparkAliases: map[string]string{},
		report:       &report,
	}
	if err := importer.importAll(ctx); err != nil {
		return emptyImportReport(), err
	}
	if _, err := rebuildAndVerifyJournalSearch(ctx, tx); err != nil {
		return emptyImportReport(), fmt.Errorf("rebuild journal search after markdown import: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return emptyImportReport(), fmt.Errorf("commit markdown import: %w", err)
	}
	return report, nil
}

func (m markdownImporter) importAll(ctx context.Context) error {
	agentsPath := filepath.Join(m.root.Path(), ".agents")
	if info, err := os.Stat(agentsPath); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return fmt.Errorf("inspect .agents: %w", err)
	} else if !info.IsDir() {
		return fmt.Errorf(".agents is not a directory: %s", agentsPath)
	}

	if err := m.importJSONSource(ctx, filepath.Join(agentsPath, "TASKS.json")); err != nil {
		return err
	}
	if err := m.importSpecs(ctx, agentsPath); err != nil {
		return err
	}
	if err := m.importTasks(ctx, agentsPath); err != nil {
		return err
	}
	if err := m.importSimpleMarkdown(ctx, agentsPath, "ideas", "idea", "ideas"); err != nil {
		return err
	}
	if err := m.importSimpleMarkdown(ctx, agentsPath, "drafts", "brainstorm", "brainstorms"); err != nil {
		return err
	}
	if err := m.importShapingDrafts(ctx, agentsPath); err != nil {
		return err
	}
	if err := m.importSessionJournals(ctx, agentsPath); err != nil {
		return err
	}
	if err := m.importReports(ctx, agentsPath); err != nil {
		return err
	}
	return nil
}

func (m markdownImporter) importSpecs(ctx context.Context, agentsPath string) error {
	files, err := filepath.Glob(filepath.Join(agentsPath, "specs", "*.md"))
	if err != nil {
		return fmt.Errorf("find specs: %w", err)
	}
	for _, path := range files {
		artifact, err := readMarkdownArtifact(m.root.Path(), path)
		if err != nil {
			return err
		}
		alias := firstNonEmpty(artifact.Frontmatter["id"], specAliasFromPath(path), artifact.Stem)
		id := stableMigrationID("spec", m.projectID, alias)
		meta := m.specIndex[alias]
		sourceID, err := m.upsertSource(ctx, artifact, "markdown")
		if err != nil {
			return err
		}
		title := firstNonEmpty(meta.Title, artifact.Frontmatter["title"], artifact.Heading, alias)
		status, writeStatus, err := m.resolveImportStatus(
			ctx, "specs", LifecycleEntitySpec, id,
			firstNonEmpty(meta.Status, artifact.Frontmatter["status"]),
			"unknown", true,
		)
		if err != nil {
			return err
		}
		if err := m.upsertSpec(ctx, id, title, status, writeStatus, sourceID); err != nil {
			return err
		}
		if err := m.upsertArtifactBody(ctx, "spec", id, sourceID, artifact); err != nil {
			return err
		}
		if err := m.upsertAlias(ctx, "spec", id, "spec", alias); err != nil {
			return err
		}
		if err := m.deleteImportedRelationships(ctx, "spec", id); err != nil {
			return err
		}
		if err := m.importArtifactRelationships(ctx, "spec", id, artifact); err != nil {
			return err
		}
	}
	return nil
}

func (m markdownImporter) importTasks(ctx context.Context, agentsPath string) error {
	files, err := filepath.Glob(filepath.Join(agentsPath, "tasks", "*.md"))
	if err != nil {
		return fmt.Errorf("find tasks: %w", err)
	}
	for _, path := range files {
		artifact, err := readMarkdownArtifact(m.root.Path(), path)
		if err != nil {
			return err
		}
		alias := firstNonEmpty(artifact.Frontmatter["id"], taskAliasFromPath(path), artifact.Stem)
		meta := m.taskIndex[alias]
		sourceID, err := m.upsertSource(ctx, artifact, "markdown")
		if err != nil {
			return err
		}

		specAlias := firstNonEmpty(meta.Spec, artifact.Frontmatter["spec"])
		var specID any
		if specAlias != "" {
			resolvedSpecID, err := m.ensureSpecPlaceholder(ctx, specAlias)
			if err != nil {
				return err
			}
			specID = resolvedSpecID
		}

		id := stableMigrationID("task", m.projectID, alias)
		title := firstNonEmpty(meta.Title, artifact.Frontmatter["title"], artifact.Heading, alias)
		status, writeStatus, err := m.resolveImportStatus(
			ctx, "tasks", LifecycleEntityTask, id,
			firstNonEmpty(meta.Status, artifact.Frontmatter["status"]),
			"unknown", true,
		)
		if err != nil {
			return err
		}
		priority := firstNonEmpty(meta.Priority, artifact.Frontmatter["priority"])
		if err := m.upsertTask(ctx, id, specID, title, status, writeStatus, priority, sourceID); err != nil {
			return err
		}
		if err := m.upsertArtifactBody(ctx, "task", id, sourceID, artifact); err != nil {
			return err
		}
		if err := m.upsertAlias(ctx, "task", id, "task", alias); err != nil {
			return err
		}
		if err := m.deleteImportedRelationships(ctx, "task", id); err != nil {
			return err
		}
		if specAlias != "" {
			toID := stableMigrationID("spec", m.projectID, specAlias)
			if err := m.upsertRelationship(ctx, "task", id, "spec", toID, "implements", "imported from task metadata"); err != nil {
				return err
			}
		}
		for _, dependency := range taskDependencies(meta, artifact.Frontmatter["depends_on"]) {
			toID := stableMigrationID("task", m.projectID, dependency)
			if err := m.upsertAlias(ctx, "task", toID, "task", dependency); err != nil {
				return err
			}
			if err := m.upsertRelationship(ctx, "task", id, "task", toID, "blocked_by", "imported from task depends_on"); err != nil {
				return err
			}
		}
		if err := m.importArtifactRelationships(ctx, "task", id, artifact); err != nil {
			return err
		}
	}
	return nil
}

func (m markdownImporter) importSimpleMarkdown(ctx context.Context, agentsPath string, subdir string, kind string, table string) error {
	pattern := "*.md"
	if kind == "brainstorm" {
		pattern = "*brainstorm*.md"
	}
	files, err := filepath.Glob(filepath.Join(agentsPath, subdir, pattern))
	if err != nil {
		return fmt.Errorf("find %s: %w", table, err)
	}
	for _, path := range files {
		artifact, err := readMarkdownArtifact(m.root.Path(), path)
		if err != nil {
			return err
		}
		alias := firstNonEmpty(artifact.Frontmatter["id"], artifact.Stem)
		id := stableMigrationID(kind, m.projectID, alias)
		sourceID, err := m.upsertSource(ctx, artifact, "markdown")
		if err != nil {
			return err
		}
		title := firstNonEmpty(artifact.Frontmatter["title"], artifact.Heading, alias)
		status, writeStatus, err := m.resolveImportStatus(
			ctx, table, kind, id, artifact.Frontmatter["status"], LifecycleStatusOpen, true,
		)
		if err != nil {
			return err
		}
		if err := m.upsertSimpleEntity(ctx, table, id, title, status, writeStatus, sourceID); err != nil {
			return err
		}
		if err := m.upsertArtifactBody(ctx, kind, id, sourceID, artifact); err != nil {
			return err
		}
		if err := m.upsertAlias(ctx, kind, id, kind, alias); err != nil {
			return err
		}
		if err := m.deleteImportedRelationships(ctx, kind, id); err != nil {
			return err
		}
		if err := m.importArtifactRelationships(ctx, kind, id, artifact); err != nil {
			return err
		}
	}
	return nil
}

func (m markdownImporter) importShapingDrafts(ctx context.Context, agentsPath string) error {
	files, err := filepath.Glob(filepath.Join(agentsPath, "drafts", "*.md"))
	if err != nil {
		return fmt.Errorf("find shaping drafts: %w", err)
	}
	for _, path := range files {
		artifact, err := readMarkdownArtifact(m.root.Path(), path)
		if err != nil {
			return err
		}
		if !isShapingDraftArtifact(artifact.RelPath, artifact.Frontmatter) {
			continue
		}
		alias := firstNonEmpty(artifact.Frontmatter["id"], artifact.Stem)
		id := stableMigrationID("shaping_draft", m.projectID, alias)
		sourceID, err := m.upsertSource(ctx, artifact, "markdown")
		if err != nil {
			return err
		}
		title := firstNonEmpty(artifact.Frontmatter["title"], artifact.Heading, alias)
		status, writeStatus, err := m.resolveImportStatus(
			ctx, "shaping_drafts", "shaping_draft", id, artifact.Frontmatter["status"], "draft", false,
		)
		if err != nil {
			return err
		}
		if err := m.upsertSimpleEntity(ctx, "shaping_drafts", id, title, status, writeStatus, sourceID); err != nil {
			return err
		}
		if err := m.upsertArtifactBody(ctx, "shaping_draft", id, sourceID, artifact); err != nil {
			return err
		}
		if err := m.upsertAlias(ctx, "shaping_draft", id, "shaping_draft", alias); err != nil {
			return err
		}
		if err := m.deleteImportedRelationships(ctx, "shaping_draft", id); err != nil {
			return err
		}
		if err := m.importArtifactRelationships(ctx, "shaping_draft", id, artifact); err != nil {
			return err
		}
	}
	return nil
}

// importSessionJournals imports legacy .agents/sessions/*.md files as
// project-scoped journal entries. Under the journal-first model (SPEC-056) the
// session entity no longer exists: each session file's journal lines become
// project-scoped journal_entries tagged with the file's harness session id, and
// its spark lines become sparks. No session row, body, or alias is created.
func (m markdownImporter) importSessionJournals(ctx context.Context, agentsPath string) error {
	files, err := filepath.Glob(filepath.Join(agentsPath, "sessions", "*.md"))
	if err != nil {
		return fmt.Errorf("find sessions: %w", err)
	}
	archivedFiles, err := filepath.Glob(filepath.Join(agentsPath, "sessions", "archive", "*.md"))
	if err != nil {
		return fmt.Errorf("find archived sessions: %w", err)
	}
	files = append(files, archivedFiles...)
	for _, path := range files {
		artifact, err := readMarkdownArtifact(m.root.Path(), path)
		if err != nil {
			return err
		}
		sourceID, err := m.upsertSource(ctx, artifact, "markdown")
		if err != nil {
			return err
		}
		observedBranch := firstNonEmpty(artifact.Frontmatter["branch"])
		observedWorktree := firstNonEmpty(artifact.Frontmatter["worktree"], artifact.Frontmatter["observed_worktree"])
		harnessSessionID := firstNonEmpty(artifact.Frontmatter["claude_session_id"], artifact.Frontmatter["harness_session_id"], artifact.Frontmatter["session_id"])
		if err := m.importSessionJournal(ctx, artifact, observedBranch, observedWorktree, harnessSessionID, sourceID); err != nil {
			return err
		}
	}
	return nil
}

func (m markdownImporter) importReports(ctx context.Context, agentsPath string) error {
	files, err := filepath.Glob(filepath.Join(agentsPath, "reports", "*.md"))
	if err != nil {
		return fmt.Errorf("find reports: %w", err)
	}
	archivedFiles, err := filepath.Glob(filepath.Join(agentsPath, "reports", "archive", "*.md"))
	if err != nil {
		return fmt.Errorf("find archived reports: %w", err)
	}
	files = append(files, archivedFiles...)
	for _, path := range files {
		artifact, err := readMarkdownArtifact(m.root.Path(), path)
		if err != nil {
			return err
		}
		alias := firstNonEmpty(artifact.Frontmatter["id"], artifact.Stem)
		id := stableMigrationID("report", m.projectID, alias)
		sourceID, err := m.upsertSource(ctx, artifact, "markdown")
		if err != nil {
			return err
		}
		title := firstNonEmpty(artifact.Frontmatter["title"], artifact.Heading, alias)
		status, writeStatus, err := m.resolveImportStatus(
			ctx, "reports", LifecycleEntityReport, id, reportSourceStatus(artifact), "unknown", true,
		)
		if err != nil {
			return err
		}
		reportKind := firstNonEmpty(artifact.Frontmatter["type"], artifact.Frontmatter["report_kind"], artifact.Frontmatter["kind"], "markdown")
		if err := m.upsertReport(ctx, id, reportKind, title, status, writeStatus, sourceID); err != nil {
			return err
		}
		if err := m.upsertArtifactBody(ctx, "report", id, sourceID, artifact); err != nil {
			return err
		}
		if err := m.upsertAlias(ctx, "report", id, "report", alias); err != nil {
			return err
		}
		if err := m.deleteImportedRelationships(ctx, "report", id); err != nil {
			return err
		}
		if err := m.importArtifactRelationships(ctx, "report", id, artifact); err != nil {
			return err
		}
	}
	return nil
}

func (m markdownImporter) importJSONSource(ctx context.Context, path string) error {
	artifact, err := readSourceArtifact(m.root.Path(), path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	_, err = m.upsertSource(ctx, artifact, "json")
	return err
}

func (m markdownImporter) importSessionJournal(ctx context.Context, artifact sourceArtifact, observedBranch string, observedWorktree string, harnessSessionID string, sourceID string) error {
	re := regexp.MustCompile(`^\[(?P<time>[^\]]+)\]\s+(?P<type>[a-zA-Z0-9_-]+)\((?P<scope>[^)]*)\):\s*(?P<message>.*)$`)
	for lineNumber, line := range strings.Split(string(artifact.Content), "\n") {
		matches := re.FindStringSubmatch(line)
		if matches == nil {
			continue
		}
		entryType := matches[2]
		scope := matches[3]
		message := matches[4]
		// Skip lifecycle-noise markers: they are the exact rows the journal-first
		// migration purges, so importing them would just re-seed noise.
		if entryType == "session" {
			continue
		}
		entryID := stableMigrationID("journal", m.projectID, artifact.RelPath, fmt.Sprint(lineNumber+1))
		imported, err := m.upsertJournalEntry(ctx, entryID, entryType, scope, message, observedBranch, observedWorktree, harnessSessionID)
		if err != nil {
			return err
		}
		if !imported {
			continue
		}
		if entryType == "spark" {
			sparkID := stableMigrationID("spark", m.projectID, artifact.RelPath, fmt.Sprint(lineNumber+1))
			if err := m.upsertSpark(ctx, sparkID, scope, message, sourceID); err != nil {
				return err
			}
			slug := sparkSlugFromMessage(message)
			if slug != "" {
				alias := "SPARK-" + slug
				if err := m.upsertAlias(ctx, "spark", sparkID, "spark", alias); err != nil {
					return err
				}
				m.sparkAliases[slug] = sparkID
			}
			if err := m.deleteImportedRelationships(ctx, "spark", sparkID); err != nil {
				return err
			}
			if target, ok := m.capturedIdeaTarget(message); ok {
				if err := m.upsertAlias(ctx, target.kind, target.id, target.kind, target.alias); err != nil {
					return err
				}
				if err := m.upsertRelationship(ctx, "spark", sparkID, target.kind, target.id, "promoted_to", "imported from spark journal entry"); err != nil {
					return err
				}
			}
		}
		if entryType == "resolve" && scope == "spark" {
			if err := m.importSparkResolution(ctx, message); err != nil {
				return err
			}
		}
	}
	return nil
}

type relationshipTarget struct {
	kind  string
	id    string
	alias string
}

func (m markdownImporter) importArtifactRelationships(ctx context.Context, fromKind string, fromID string, artifact sourceArtifact) error {
	for _, field := range relationshipFrontmatterFields() {
		for _, value := range splitFrontmatterList(artifact.Frontmatter[field]) {
			target, ok := m.resolveRelationshipTarget(value)
			if !ok {
				continue
			}
			if err := m.upsertAlias(ctx, target.kind, target.id, target.kind, target.alias); err != nil {
				return err
			}
			reason := fmt.Sprintf("imported from %s frontmatter", field)
			if err := m.upsertRelationship(ctx, fromKind, fromID, target.kind, target.id, field, reason); err != nil {
				return err
			}
		}
	}
	return nil
}

func (m markdownImporter) importSparkResolution(ctx context.Context, message string) error {
	slug, rest, ok := strings.Cut(message, " -> ")
	if !ok {
		return nil
	}
	slug = normalizeSparkSlug(slug)
	sparkID := m.sparkAliases[slug]
	if sparkID == "" {
		return nil
	}
	targetText := strings.TrimSpace(rest)
	targetText = strings.TrimPrefix(targetText, "promoted to ")
	targetText = strings.TrimPrefix(targetText, "resolved by ")
	target, ok := m.resolveRelationshipTarget(targetText)
	if !ok {
		return nil
	}
	if err := m.upsertAlias(ctx, target.kind, target.id, target.kind, target.alias); err != nil {
		return err
	}
	relationshipType := "promoted_to"
	if !strings.HasPrefix(strings.TrimSpace(rest), "promoted to ") {
		relationshipType = "resolved_by"
	}
	return m.upsertRelationship(ctx, "spark", sparkID, target.kind, target.id, relationshipType, "imported from resolve(spark) journal entry")
}

func (m markdownImporter) resolveRelationshipTarget(value string) (relationshipTarget, bool) {
	if alias, ok := m.resolveDraftRelationshipTarget(value); ok {
		return relationshipTarget{
			kind:  "shaping_draft",
			id:    stableMigrationID("shaping_draft", m.projectID, alias),
			alias: alias,
		}, true
	}
	alias, kind, ok := relationshipAliasAndKind(value)
	if !ok {
		return relationshipTarget{}, false
	}
	return relationshipTarget{
		kind:  kind,
		id:    stableMigrationID(kind, m.projectID, alias),
		alias: alias,
	}, true
}

func (m markdownImporter) resolveDraftRelationshipTarget(value string) (string, bool) {
	trimmed := strings.Trim(strings.TrimSpace(value), `"'`)
	normalizedPath := strings.TrimPrefix(filepath.ToSlash(trimmed), "./")
	if !strings.HasPrefix(normalizedPath, ".agents/drafts/") || !strings.HasSuffix(normalizedPath, ".md") || isBrainstormPath(normalizedPath) {
		return "", false
	}
	path := filepath.Join(m.root.Path(), filepath.FromSlash(normalizedPath))
	artifact, err := readMarkdownArtifact(m.root.Path(), path)
	if err != nil {
		return "", false
	}
	if !isShapingDraftArtifact(artifact.RelPath, artifact.Frontmatter) {
		return "", false
	}
	return firstNonEmpty(artifact.Frontmatter["id"], artifact.Stem), true
}

func (m markdownImporter) ensureSpecPlaceholder(ctx context.Context, alias string) (string, error) {
	id := stableMigrationID("spec", m.projectID, alias)
	_, err := m.tx.ExecContext(ctx, `
INSERT INTO specs (id, project_id, title, status, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO NOTHING
`, id, m.projectID, alias, "unknown", m.now, m.now)
	if err != nil {
		return "", fmt.Errorf("ensure spec placeholder %s: %w", alias, err)
	}
	if err := m.upsertAlias(ctx, "spec", id, "spec", alias); err != nil {
		return "", err
	}
	return id, nil
}

func (m markdownImporter) upsertSource(ctx context.Context, artifact sourceArtifact, sourceKind string) (string, error) {
	id := stableMigrationID("source", m.projectID, artifact.RelPath)
	_, err := m.tx.ExecContext(ctx, `
INSERT INTO sources (id, project_id, source_kind, path, hash, imported_at, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
  source_kind = excluded.source_kind,
  path = excluded.path,
  hash = excluded.hash,
  imported_at = excluded.imported_at,
  updated_at = excluded.updated_at
`, id, m.projectID, sourceKind, artifact.RelPath, artifact.Hash, m.now, m.now, m.now)
	if err != nil {
		return "", fmt.Errorf("upsert source %s: %w", artifact.RelPath, err)
	}
	return id, nil
}

func (m markdownImporter) upsertArtifactBody(ctx context.Context, entityKind string, entityID string, sourceID string, artifact sourceArtifact) error {
	_, err := upsertArtifactBodyTx(ctx, m.tx, m.projectID, entityKind, entityID, ArtifactBodyKindMarkdown, markdownArtifactBodyContent(artifact.Content), sourceID, m.now)
	if err != nil {
		return fmt.Errorf("upsert %s artifact body %s: %w", entityKind, entityID, err)
	}
	return nil
}

func (m markdownImporter) upsertSpec(ctx context.Context, id string, title string, status string, writeStatus bool, sourceID any) error {
	_, err := m.tx.ExecContext(ctx, `
INSERT INTO specs (id, project_id, title, status, body_source_id, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
  title = excluded.title,
  status = CASE WHEN ? THEN excluded.status ELSE specs.status END,
  body_source_id = COALESCE(excluded.body_source_id, specs.body_source_id),
  updated_at = excluded.updated_at
`, id, m.projectID, title, status, sourceID, m.now, m.now, writeStatus)
	if err != nil {
		return fmt.Errorf("upsert spec %s: %w", id, err)
	}
	return nil
}

func (m markdownImporter) upsertTask(ctx context.Context, id string, specID any, title string, status string, writeStatus bool, priority string, sourceID string) error {
	_, err := m.tx.ExecContext(ctx, `
INSERT INTO tasks (id, project_id, spec_id, title, status, priority, body_source_id, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
  spec_id = excluded.spec_id,
  title = excluded.title,
  status = CASE WHEN ? THEN excluded.status ELSE tasks.status END,
  priority = excluded.priority,
  body_source_id = excluded.body_source_id,
  updated_at = excluded.updated_at
`, id, m.projectID, specID, title, status, emptyToNil(priority), sourceID, m.now, m.now, writeStatus)
	if err != nil {
		return fmt.Errorf("upsert task %s: %w", id, err)
	}
	return nil
}

func (m markdownImporter) upsertSimpleEntity(ctx context.Context, table string, id string, title string, status string, writeStatus bool, sourceID string) error {
	_, err := m.tx.ExecContext(ctx, fmt.Sprintf(`
INSERT INTO %s (id, project_id, title, status, body_source_id, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
  title = excluded.title,
  status = CASE WHEN ? THEN excluded.status ELSE %s.status END,
  body_source_id = excluded.body_source_id,
  updated_at = excluded.updated_at
`, table, table), id, m.projectID, title, status, sourceID, m.now, m.now, writeStatus)
	if err != nil {
		return fmt.Errorf("upsert %s %s: %w", table, id, err)
	}
	return nil
}

func (m markdownImporter) upsertReport(ctx context.Context, id string, reportKind string, title string, status string, writeStatus bool, sourceID string) error {
	_, err := m.tx.ExecContext(ctx, `
INSERT INTO reports (id, project_id, report_kind, title, status, body_source_id, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
  report_kind = excluded.report_kind,
  title = excluded.title,
  status = CASE WHEN ? THEN excluded.status ELSE reports.status END,
  body_source_id = excluded.body_source_id,
  updated_at = excluded.updated_at
`, id, m.projectID, reportKind, title, status, sourceID, m.now, m.now, writeStatus)
	if err != nil {
		return fmt.Errorf("upsert report %s: %w", id, err)
	}
	return nil
}

func (m markdownImporter) upsertSpark(ctx context.Context, id string, scope string, text string, sourceID string) error {
	_, err := m.tx.ExecContext(ctx, `
INSERT INTO sparks (id, project_id, scope, status, text, source_id, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
  scope = excluded.scope,
  text = excluded.text,
  source_id = excluded.source_id,
  updated_at = excluded.updated_at
`, id, m.projectID, emptyToNil(scope), "open", text, sourceID, m.now, m.now)
	if err != nil {
		return fmt.Errorf("upsert spark %s: %w", id, err)
	}
	return nil
}

// upsertJournalEntry writes a journal line and its migration origin unless the
// existing origin fails the reclaim fingerprint, in which case the whole entry
// is skipped untouched. Returns false when skipped.
func (m markdownImporter) upsertJournalEntry(ctx context.Context, id string, entryType string, scope string, message string, observedBranch string, observedWorktree string, harnessSessionID string) (bool, error) {
	origin, journal, err := m.loadOriginJournalPair(ctx, id)
	if err != nil {
		return false, err
	}
	action := decideOriginImportDisposition(origin, journal)
	if action == originImportSkip {
		mechanism := JournalOriginMechanismUnknown
		if origin != nil {
			mechanism = origin.CaptureMechanism
		}
		if m.report != nil {
			m.report.SkippedEntries = append(m.report.SkippedEntries, ImportSkippedEntry{
				JournalEntryID:   id,
				CaptureMechanism: mechanism,
			})
		}
		return false, nil
	}

	_, err = m.tx.ExecContext(ctx, `
INSERT INTO journal_entries (id, project_id, entry_type, scope, message, observed_branch, observed_worktree, harness_session_id, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
  entry_type = excluded.entry_type,
  scope = excluded.scope,
  message = excluded.message,
  observed_branch = excluded.observed_branch,
  observed_worktree = excluded.observed_worktree,
  harness_session_id = excluded.harness_session_id,
  updated_at = excluded.updated_at
`, id, m.projectID, entryType, emptyToNil(scope), message, emptyToNil(observedBranch), emptyToNil(observedWorktree), emptyToNil(harnessSessionID), m.now, m.now)
	if err != nil {
		return false, fmt.Errorf("upsert journal entry %s: %w", id, err)
	}
	if err := m.writeMigrationJournalOrigin(ctx, id, observedBranch, observedWorktree, harnessSessionID, origin != nil); err != nil {
		return false, err
	}
	if action == originImportReclaim && m.report != nil {
		m.report.ReclaimedOrigins++
	}
	return true, nil
}

func (m markdownImporter) loadOriginJournalPair(ctx context.Context, journalEntryID string) (*journalOriginScanRow, *journalEntryScanRow, error) {
	var journal journalEntryScanRow
	err := m.tx.QueryRowContext(ctx, `
SELECT harness_session_id, observed_branch, observed_worktree, created_at
FROM journal_entries
WHERE project_id = ? AND id = ?
`, m.projectID, journalEntryID).Scan(
		&journal.HarnessSessionID, &journal.ObservedBranch, &journal.ObservedWorktree, &journal.CreatedAt,
	)
	var journalPtr *journalEntryScanRow
	switch {
	case err == nil:
		journalPtr = &journal
	case errors.Is(err, sql.ErrNoRows):
		journalPtr = nil
	default:
		return nil, nil, fmt.Errorf("read journal entry %s for origin decision: %w", journalEntryID, err)
	}

	var origin journalOriginScanRow
	err = m.tx.QueryRowContext(ctx, `
SELECT
  capture_mechanism, envelope_version,
  observed_harness, observed_harness_version, harness_session_id, agent_id,
  source_event, branch, worktree, head, change_path, change_sha256,
  dirty, reconstructable, durable_result_kind, durable_result_id, created_at
FROM journal_origins
WHERE project_id = ? AND journal_entry_id = ?
`, m.projectID, journalEntryID).Scan(
		&origin.CaptureMechanism, &origin.EnvelopeVersion,
		&origin.ObservedHarness, &origin.ObservedHarnessVersion, &origin.HarnessSessionID, &origin.AgentID,
		&origin.SourceEvent, &origin.Branch, &origin.Worktree, &origin.Head, &origin.ChangePath, &origin.ChangeSHA256,
		&origin.Dirty, &origin.Reconstructable, &origin.DurableResultKind, &origin.DurableResultID, &origin.CreatedAt,
	)
	switch {
	case err == nil:
		return &origin, journalPtr, nil
	case errors.Is(err, sql.ErrNoRows):
		return nil, journalPtr, nil
	default:
		return nil, nil, fmt.Errorf("read journal origin %s for decision: %w", journalEntryID, err)
	}
}

func (m markdownImporter) writeMigrationJournalOrigin(ctx context.Context, journalEntryID string, branch string, worktree string, harnessSessionID string, exists bool) error {
	var createdAt string
	if err := m.tx.QueryRowContext(ctx, `SELECT created_at FROM journal_entries WHERE project_id = ? AND id = ?`, m.projectID, journalEntryID).Scan(&createdAt); err != nil {
		return fmt.Errorf("read imported journal entry %s for origin: %w", journalEntryID, err)
	}
	if exists {
		if _, err := m.tx.ExecContext(ctx, `
UPDATE journal_origins
SET envelope_version = 1,
    capture_mechanism = ?,
    observed_harness = NULL,
    observed_harness_version = NULL,
    harness_session_id = ?,
    agent_id = NULL,
    source_event = 'markdown_import',
    branch = ?,
    worktree = ?,
    head = NULL,
    change_path = NULL,
    change_sha256 = NULL,
    dirty = NULL,
    reconstructable = NULL,
    durable_result_kind = NULL,
    durable_result_id = NULL,
    created_at = ?
WHERE project_id = ? AND journal_entry_id = ?
`, JournalOriginMechanismMigration, emptyToNil(harnessSessionID), emptyToNil(branch), emptyToNil(worktree), createdAt, m.projectID, journalEntryID); err != nil {
			return fmt.Errorf("update imported journal origin %s: %w", journalEntryID, err)
		}
		return nil
	}
	if _, err := m.tx.ExecContext(ctx, `
INSERT INTO journal_origins (
  journal_entry_id, project_id, envelope_version, capture_mechanism,
  observed_harness, observed_harness_version, harness_session_id, agent_id,
  source_event, branch, worktree, head, change_path, change_sha256,
  dirty, reconstructable, durable_result_kind, durable_result_id, created_at
) VALUES (?, ?, 1, ?, NULL, NULL, ?, NULL, 'markdown_import', ?, ?, NULL, NULL, NULL, NULL, NULL, NULL, NULL, ?)
`, journalEntryID, m.projectID, JournalOriginMechanismMigration, emptyToNil(harnessSessionID), emptyToNil(branch), emptyToNil(worktree), createdAt); err != nil {
		return fmt.Errorf("insert imported journal origin %s: %w", journalEntryID, err)
	}
	return nil
}

func (m markdownImporter) upsertRelationship(ctx context.Context, fromKind string, fromID string, toKind string, toID string, relationshipType string, reason string) error {
	// loaf link derives the same deterministic id for a given tuple (link.go),
	// so a manual edge collides here; the conflict WHERE keeps the importer
	// from claiming rows it did not create (Decision 7 ownership).
	id := stableMigrationID("relationship", m.projectID, fromKind, fromID, relationshipType, toKind, toID)
	_, err := m.tx.ExecContext(ctx, `
INSERT INTO relationships (id, project_id, from_entity_kind, from_entity_id, to_entity_kind, to_entity_id, relationship_type, reason, origin, source_id, source_field, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
  reason = excluded.reason,
  origin = excluded.origin,
  source_id = excluded.source_id,
  source_field = excluded.source_field,
  updated_at = excluded.updated_at
WHERE relationships.origin = 'imported'
`, id, m.projectID, fromKind, fromID, toKind, toID, relationshipType, reason, "imported", nil, relationshipType, m.now, m.now)
	if err != nil {
		return fmt.Errorf("upsert relationship %s: %w", id, err)
	}
	return nil
}

// deleteImportedRelationships removes only origin='imported' edges for the
// from-entity. Manual relationships whose reason happens to start with
// "imported from" are owned by the user and must survive refresh (Decision 7).
// Artifacts and per-spark refresh share this helper — do not fork the predicate.
func (m markdownImporter) deleteImportedRelationships(ctx context.Context, fromKind string, fromID string) error {
	_, err := m.tx.ExecContext(ctx, `
DELETE FROM relationships
WHERE project_id = ?
  AND from_entity_kind = ?
  AND from_entity_id = ?
  AND origin = 'imported'
`, m.projectID, fromKind, fromID)
	if err != nil {
		return fmt.Errorf("delete imported relationships for %s %s: %w", fromKind, fromID, err)
	}
	return nil
}

func (m markdownImporter) upsertAlias(ctx context.Context, entityKind string, entityID string, namespace string, alias string) error {
	id := stableMigrationID("alias", m.projectID, namespace, alias)
	_, err := m.tx.ExecContext(ctx, `
INSERT INTO aliases (id, project_id, entity_kind, entity_id, namespace, alias, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(project_id, namespace, alias) DO UPDATE SET
  entity_kind = excluded.entity_kind,
  entity_id = excluded.entity_id,
  updated_at = excluded.updated_at
`, id, m.projectID, entityKind, entityID, namespace, alias, m.now, m.now)
	if err != nil {
		return fmt.Errorf("upsert alias %s:%s: %w", namespace, alias, err)
	}
	return nil
}

type sourceArtifact struct {
	Path        string
	RelPath     string
	Stem        string
	Content     []byte
	Hash        string
	Frontmatter map[string]string
	Heading     string
}

func readMarkdownArtifact(rootPath string, path string) (sourceArtifact, error) {
	artifact, err := readSourceArtifact(rootPath, path)
	if err != nil {
		return sourceArtifact{}, err
	}
	artifact.Frontmatter = parseFrontmatterMap(artifact.Content)
	artifact.Heading = firstHeading(artifact.Content)
	return artifact, nil
}

func readSourceArtifact(rootPath string, path string) (sourceArtifact, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return sourceArtifact{}, fmt.Errorf("read source %s: %w", path, err)
	}
	rel, err := filepath.Rel(rootPath, path)
	if err != nil {
		return sourceArtifact{}, fmt.Errorf("rel source %s: %w", path, err)
	}
	sum := sha256.Sum256(content)
	return sourceArtifact{
		Path:    path,
		RelPath: filepath.ToSlash(rel),
		Stem:    strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)),
		Content: content,
		Hash:    hex.EncodeToString(sum[:]),
	}, nil
}

func parseFrontmatterMap(content []byte) map[string]string {
	values := map[string]string{}
	frontmatter := markdownFrontmatter(content)
	if frontmatter == "" {
		return values
	}
	var currentListKey string
	for _, line := range strings.Split(frontmatter, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if currentListKey != "" && strings.HasPrefix(trimmed, "-") {
			item := strings.TrimSpace(strings.TrimPrefix(trimmed, "-"))
			if item != "" {
				if values[currentListKey] != "" {
					values[currentListKey] += frontmatterListSeparator
				}
				values[currentListKey] += strings.Trim(item, `"'`)
			}
			continue
		}
		currentListKey = ""
		key, value, ok := strings.Cut(trimmed, ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if value == "" {
			currentListKey = key
			continue
		}
		values[key] = strings.Trim(value, `"'`)
	}
	return values
}

func firstHeading(content []byte) string {
	for _, line := range strings.Split(string(content), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "# ") {
			return strings.TrimSpace(strings.TrimPrefix(trimmed, "# "))
		}
	}
	return ""
}

// reportSourceStatus returns the raw incoming report status before classification.
// Archive-directory placement is a real directory-derived source status, not no-opinion.
func reportSourceStatus(artifact sourceArtifact) string {
	if strings.HasPrefix(artifact.RelPath, ".agents/reports/archive/") {
		return LifecycleStatusArchived
	}
	return artifact.Frontmatter["status"]
}

// reportStatus is retained for callers that want the post-default insert value
// without going through the importer (tests / inventory helpers).
func reportStatus(artifact sourceArtifact) string {
	return classifyImportLifecycleStatus(LifecycleEntityReport, "", reportSourceStatus(artifact), "unknown").Status
}

func loadTaskIndex(rootPath string) map[string]taskIndexEntry {
	index := map[string]taskIndexEntry{}
	content, err := os.ReadFile(filepath.Join(rootPath, ".agents", "TASKS.json"))
	if err != nil {
		return index
	}
	var parsed struct {
		Tasks map[string]taskIndexEntry `json:"tasks"`
	}
	if err := json.Unmarshal(content, &parsed); err != nil {
		return index
	}
	return parsed.Tasks
}

func loadSpecIndex(rootPath string) map[string]specIndexEntry {
	index := map[string]specIndexEntry{}
	content, err := os.ReadFile(filepath.Join(rootPath, ".agents", "TASKS.json"))
	if err != nil {
		return index
	}
	var parsed struct {
		Specs map[string]specIndexEntry `json:"specs"`
	}
	if err := json.Unmarshal(content, &parsed); err != nil {
		return index
	}
	return parsed.Specs
}

func taskDependencies(meta taskIndexEntry, frontmatterDependsOn string) []string {
	if len(meta.DependsOn) > 0 {
		return meta.DependsOn
	}
	return splitFrontmatterList(frontmatterDependsOn)
}

func stableMigrationID(parts ...string) string {
	prefix := "row"
	if len(parts) > 0 && parts[0] != "" {
		prefix = parts[0]
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return prefix + ":" + hex.EncodeToString(sum[:])[:24]
}

func specAliasFromPath(path string) string {
	return leadingIDFromPath(path, `SPEC-\d+`)
}

func taskAliasFromPath(path string) string {
	return leadingIDFromPath(path, `TASK-\d+`)
}

func relationshipAliasAndKind(value string) (string, string, bool) {
	trimmed := strings.Trim(strings.TrimSpace(value), `"'`)
	if trimmed == "" {
		return "", "", false
	}
	normalizedPath := strings.TrimPrefix(filepath.ToSlash(trimmed), "./")
	switch {
	case strings.HasPrefix(normalizedPath, ".agents/specs/") && strings.HasSuffix(normalizedPath, ".md"):
		alias := specAliasFromPath(normalizedPath)
		return firstNonEmpty(alias, strings.TrimSuffix(filepath.Base(normalizedPath), filepath.Ext(normalizedPath))), "spec", true
	case strings.HasPrefix(normalizedPath, ".agents/tasks/") && strings.HasSuffix(normalizedPath, ".md"):
		alias := taskAliasFromPath(normalizedPath)
		return firstNonEmpty(alias, strings.TrimSuffix(filepath.Base(normalizedPath), filepath.Ext(normalizedPath))), "task", true
	case strings.HasPrefix(normalizedPath, ".agents/ideas/") && strings.HasSuffix(normalizedPath, ".md"):
		return strings.TrimSuffix(filepath.Base(normalizedPath), filepath.Ext(normalizedPath)), "idea", true
	case strings.HasPrefix(normalizedPath, ".agents/drafts/") && strings.HasSuffix(normalizedPath, ".md"):
		alias := strings.TrimSuffix(filepath.Base(normalizedPath), filepath.Ext(normalizedPath))
		if isBrainstormPath(normalizedPath) {
			return alias, "brainstorm", true
		}
		if isShapingDraftPath(normalizedPath) {
			return alias, "shaping_draft", true
		}
		return "", "", false
	case strings.HasPrefix(normalizedPath, ".agents/reports/") && strings.HasSuffix(normalizedPath, ".md"):
		return strings.TrimSuffix(filepath.Base(normalizedPath), filepath.Ext(normalizedPath)), "report", true
	case regexp.MustCompile(`^SPEC-\d+$`).MatchString(trimmed):
		return trimmed, "spec", true
	case regexp.MustCompile(`^TASK-\d+$`).MatchString(trimmed):
		return trimmed, "task", true
	case strings.HasPrefix(trimmed, "SPARK-"):
		return trimmed, "spark", true
	case strings.HasPrefix(trimmed, "IDEA-"):
		return trimmed, "idea", true
	default:
		return "", "", false
	}
}

func splitFrontmatterList(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" || value == "[]" {
		return nil
	}
	if strings.HasPrefix(value, "[") && strings.HasSuffix(value, "]") {
		value = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(value, "["), "]"))
		if value == "" {
			return nil
		}
	}
	separator := ","
	if strings.Contains(value, frontmatterListSeparator) {
		separator = frontmatterListSeparator
	}
	var values []string
	for _, part := range strings.Split(value, separator) {
		trimmed := strings.Trim(strings.TrimSpace(part), `"'`)
		if trimmed != "" && trimmed != "[]" {
			values = append(values, trimmed)
		}
	}
	return values
}

func relationshipFrontmatterFields() []string {
	return []string{"promoted_to", "resolved_by", "derived_from", "exported_as", "implements", "supersedes"}
}

func isBrainstormPath(path string) bool {
	return strings.Contains(strings.ToLower(filepath.Base(path)), "brainstorm")
}

func isShapingDraftArtifact(relPath string, frontmatter map[string]string) bool {
	if isBrainstormPath(relPath) {
		return false
	}
	for _, field := range []string{"kind", "type", "artifact"} {
		switch strings.ToLower(strings.TrimSpace(frontmatter[field])) {
		case "shaping_draft", "shaping-draft", "shaping", "shape":
			return true
		}
	}
	return isShapingDraftPath(relPath)
}

func isShapingDraftPath(path string) bool {
	stem := strings.ToLower(strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)))
	for _, part := range regexp.MustCompile(`[^a-z0-9]+`).Split(stem, -1) {
		if part == "shape" || part == "shaping" {
			return true
		}
	}
	return false
}

func sparkSlugFromMessage(message string) string {
	prefix := message
	for _, marker := range []string{" captured to ", " -> ", " promoted to ", " resolved by "} {
		if before, _, ok := strings.Cut(prefix, marker); ok {
			prefix = before
			break
		}
	}
	fields := strings.Fields(prefix)
	if len(fields) > 0 {
		prefix = fields[0]
	}
	return normalizeSparkSlug(prefix)
}

func normalizeSparkSlug(value string) string {
	value = strings.TrimSpace(strings.TrimPrefix(value, "SPARK-"))
	value = strings.Trim(value, `"'`)
	value = strings.ToLower(value)
	re := regexp.MustCompile(`[^a-z0-9_-]+`)
	value = re.ReplaceAllString(value, "-")
	return strings.Trim(value, "-_")
}

func (m markdownImporter) capturedIdeaTarget(message string) (relationshipTarget, bool) {
	_, targetText, ok := strings.Cut(message, " captured to ")
	if !ok {
		return relationshipTarget{}, false
	}
	target, ok := m.resolveRelationshipTarget(targetText)
	if !ok || target.kind != "idea" {
		return relationshipTarget{}, false
	}
	return target, true
}

func leadingIDFromPath(path string, pattern string) string {
	re := regexp.MustCompile(pattern)
	return re.FindString(strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func emptyToNil(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}
