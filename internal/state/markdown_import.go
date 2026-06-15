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

// MarkdownMigrationResult is the structured result for a markdown migration apply.
type MarkdownMigrationResult struct {
	MarkdownMigrationPlan
	DatabaseScope      string `json:"database_scope"`
	ImportScope        string `json:"import_scope"`
	DatabasePath       string `json:"database_path"`
	ProjectID          string `json:"project_id"`
	ProjectName        string `json:"project_name"`
	ProjectCurrentPath string `json:"project_current_path"`
	Action             string `json:"action"`
	Applied            bool   `json:"applied"`
}

const (
	MarkdownMigrationActionApply  = "apply"
	MarkdownMigrationActionResume = "resume"
)

type taskIndexEntry struct {
	Title     string   `json:"title"`
	Spec      string   `json:"spec"`
	Status    string   `json:"status"`
	Priority  string   `json:"priority"`
	DependsOn []string `json:"depends_on"`
	File      string   `json:"file"`
}

const frontmatterListSeparator = "\x1f"

type markdownImporter struct {
	tx           *sql.Tx
	root         project.Root
	projectID    string
	now          string
	taskIndex    map[string]taskIndexEntry
	sparkAliases map[string]string
}

// ApplyMarkdownMigration initializes SQLite state and imports structurally
// knowable .agents artifacts without mutating source Markdown files.
func ApplyMarkdownMigration(ctx context.Context, root project.Root, resolver PathResolver) (MarkdownMigrationResult, error) {
	plan, err := PreviewMarkdownMigration(root)
	if err != nil {
		return MarkdownMigrationResult{}, err
	}
	status, err := Initialize(ctx, root, resolver)
	if err != nil {
		return MarkdownMigrationResult{}, err
	}

	store, err := OpenStore(status.DatabasePath)
	if err != nil {
		return MarkdownMigrationResult{}, err
	}
	defer store.Close()

	if err := store.ImportMarkdown(ctx, root); err != nil {
		return MarkdownMigrationResult{}, err
	}
	return MarkdownMigrationResult{
		MarkdownMigrationPlan: plan,
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

// ImportMarkdown imports .agents artifacts into an initialized state database.
func (s *Store) ImportMarkdown(ctx context.Context, root project.Root) error {
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin markdown import transaction: %w", err)
	}
	defer tx.Rollback()

	importer := markdownImporter{
		tx:           tx,
		root:         root,
		projectID:    projectID,
		now:          time.Now().UTC().Format(time.RFC3339),
		taskIndex:    loadTaskIndex(root.Path()),
		sparkAliases: map[string]string{},
	}
	if err := importer.importAll(ctx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit markdown import: %w", err)
	}
	return nil
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
	if err := m.importSessions(ctx, agentsPath); err != nil {
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
		sourceID, err := m.upsertSource(ctx, artifact, "markdown")
		if err != nil {
			return err
		}
		title := firstNonEmpty(artifact.Frontmatter["title"], artifact.Heading, alias)
		status := firstNonEmpty(artifact.Frontmatter["status"], "unknown")
		if err := m.upsertSpec(ctx, id, title, status, sourceID); err != nil {
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
		status := firstNonEmpty(meta.Status, artifact.Frontmatter["status"], "unknown")
		priority := firstNonEmpty(meta.Priority, artifact.Frontmatter["priority"])
		if err := m.upsertTask(ctx, id, specID, title, status, priority, sourceID); err != nil {
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
		status := firstNonEmpty(artifact.Frontmatter["status"], "open")
		if err := m.upsertSimpleEntity(ctx, table, id, title, status, sourceID); err != nil {
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
		status := firstNonEmpty(artifact.Frontmatter["status"], "draft")
		if err := m.upsertSimpleEntity(ctx, "shaping_drafts", id, title, status, sourceID); err != nil {
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

func (m markdownImporter) importSessions(ctx context.Context, agentsPath string) error {
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
		alias := firstNonEmpty(artifact.Frontmatter["id"], artifact.Stem)
		id := stableMigrationID("session", m.projectID, alias)
		sourceID, err := m.upsertSource(ctx, artifact, "markdown")
		if err != nil {
			return err
		}
		status := sessionStatus(artifact)
		branch := firstNonEmpty(artifact.Frontmatter["branch"])
		harnessSessionID := firstNonEmpty(artifact.Frontmatter["claude_session_id"], artifact.Frontmatter["harness_session_id"], artifact.Frontmatter["session_id"])
		if err := m.upsertSession(ctx, id, branch, status, harnessSessionID, sourceID); err != nil {
			return err
		}
		if err := m.upsertAlias(ctx, "session", id, "session", alias); err != nil {
			return err
		}
		if err := m.importSessionJournal(ctx, artifact, id, sourceID); err != nil {
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
		status := reportStatus(artifact)
		reportKind := firstNonEmpty(artifact.Frontmatter["type"], artifact.Frontmatter["report_kind"], artifact.Frontmatter["kind"], "markdown")
		if err := m.upsertReport(ctx, id, reportKind, title, status, sourceID); err != nil {
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

func (m markdownImporter) importSessionJournal(ctx context.Context, artifact sourceArtifact, sessionID string, sourceID string) error {
	re := regexp.MustCompile(`^\[(?P<time>[^\]]+)\]\s+(?P<type>[a-zA-Z0-9_-]+)\((?P<scope>[^)]*)\):\s*(?P<message>.*)$`)
	for lineNumber, line := range strings.Split(string(artifact.Content), "\n") {
		matches := re.FindStringSubmatch(line)
		if matches == nil {
			continue
		}
		entryType := matches[2]
		scope := matches[3]
		message := matches[4]
		entryID := stableMigrationID("journal", m.projectID, artifact.RelPath, fmt.Sprint(lineNumber+1))
		if err := m.upsertJournalEntry(ctx, entryID, entryType, scope, message, sessionID); err != nil {
			return err
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

func (m markdownImporter) upsertSpec(ctx context.Context, id string, title string, status string, sourceID any) error {
	_, err := m.tx.ExecContext(ctx, `
INSERT INTO specs (id, project_id, title, status, body_source_id, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
  title = excluded.title,
  status = excluded.status,
  body_source_id = COALESCE(excluded.body_source_id, specs.body_source_id),
  updated_at = excluded.updated_at
`, id, m.projectID, title, status, sourceID, m.now, m.now)
	if err != nil {
		return fmt.Errorf("upsert spec %s: %w", id, err)
	}
	return nil
}

func (m markdownImporter) upsertTask(ctx context.Context, id string, specID any, title string, status string, priority string, sourceID string) error {
	_, err := m.tx.ExecContext(ctx, `
INSERT INTO tasks (id, project_id, spec_id, title, status, priority, body_source_id, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
  spec_id = excluded.spec_id,
  title = excluded.title,
  status = excluded.status,
  priority = excluded.priority,
  body_source_id = excluded.body_source_id,
  updated_at = excluded.updated_at
`, id, m.projectID, specID, title, status, emptyToNil(priority), sourceID, m.now, m.now)
	if err != nil {
		return fmt.Errorf("upsert task %s: %w", id, err)
	}
	return nil
}

func (m markdownImporter) upsertSimpleEntity(ctx context.Context, table string, id string, title string, status string, sourceID string) error {
	_, err := m.tx.ExecContext(ctx, fmt.Sprintf(`
INSERT INTO %s (id, project_id, title, status, body_source_id, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
  title = excluded.title,
  status = excluded.status,
  body_source_id = excluded.body_source_id,
  updated_at = excluded.updated_at
`, table), id, m.projectID, title, status, sourceID, m.now, m.now)
	if err != nil {
		return fmt.Errorf("upsert %s %s: %w", table, id, err)
	}
	return nil
}

func (m markdownImporter) upsertSession(ctx context.Context, id string, branch string, status string, harnessSessionID string, sourceID string) error {
	_, err := m.tx.ExecContext(ctx, `
INSERT INTO sessions (id, project_id, harness_session_id, branch, status, body_source_id, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
  harness_session_id = excluded.harness_session_id,
  branch = excluded.branch,
  status = excluded.status,
  body_source_id = excluded.body_source_id,
  updated_at = excluded.updated_at
`, id, m.projectID, emptyToNil(harnessSessionID), emptyToNil(branch), status, sourceID, m.now, m.now)
	if err != nil {
		return fmt.Errorf("upsert session %s: %w", id, err)
	}
	return nil
}

func (m markdownImporter) upsertReport(ctx context.Context, id string, reportKind string, title string, status string, sourceID string) error {
	_, err := m.tx.ExecContext(ctx, `
INSERT INTO reports (id, project_id, report_kind, title, status, body_source_id, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
  report_kind = excluded.report_kind,
  title = excluded.title,
  status = excluded.status,
  body_source_id = excluded.body_source_id,
  updated_at = excluded.updated_at
`, id, m.projectID, reportKind, title, status, sourceID, m.now, m.now)
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

func (m markdownImporter) upsertJournalEntry(ctx context.Context, id string, entryType string, scope string, message string, sessionID string) error {
	_, err := m.tx.ExecContext(ctx, `
INSERT INTO journal_entries (id, project_id, entry_type, scope, message, session_id, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
  entry_type = excluded.entry_type,
  scope = excluded.scope,
  message = excluded.message,
  session_id = excluded.session_id,
  updated_at = excluded.updated_at
`, id, m.projectID, entryType, emptyToNil(scope), message, sessionID, m.now, m.now)
	if err != nil {
		return fmt.Errorf("upsert journal entry %s: %w", id, err)
	}
	return nil
}

func (m markdownImporter) upsertRelationship(ctx context.Context, fromKind string, fromID string, toKind string, toID string, relationshipType string, reason string) error {
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
`, id, m.projectID, fromKind, fromID, toKind, toID, relationshipType, reason, "imported", nil, relationshipType, m.now, m.now)
	if err != nil {
		return fmt.Errorf("upsert relationship %s: %w", id, err)
	}
	return nil
}

func (m markdownImporter) deleteImportedRelationships(ctx context.Context, fromKind string, fromID string) error {
	_, err := m.tx.ExecContext(ctx, `
DELETE FROM relationships
WHERE project_id = ?
  AND from_entity_kind = ?
  AND from_entity_id = ?
  AND (origin = 'imported' OR reason LIKE 'imported from %')
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
ON CONFLICT(id) DO UPDATE SET
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

func archivedStatus(artifact sourceArtifact) string {
	if strings.HasPrefix(artifact.RelPath, ".agents/sessions/archive/") {
		return "archived"
	}
	return ""
}

func sessionStatus(artifact sourceArtifact) string {
	if status := archivedStatus(artifact); status != "" {
		return status
	}
	return firstNonEmpty(artifact.Frontmatter["status"], "unknown")
}

func reportStatus(artifact sourceArtifact) string {
	if strings.HasPrefix(artifact.RelPath, ".agents/reports/archive/") {
		return "archived"
	}
	return firstNonEmpty(artifact.Frontmatter["status"], "unknown")
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
