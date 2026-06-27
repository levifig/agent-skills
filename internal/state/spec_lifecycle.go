package state

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/levifig/loaf/internal/project"
)

var (
	specAliasPattern = regexp.MustCompile(`^SPEC-(\d+)$`)
	specSlugCleaner  = regexp.MustCompile(`[^a-z0-9]+`)
	specFilePattern  = regexp.MustCompile(`(?i)^SPEC-(\d+)`)
)

// SpecCreateOptions describes a SQLite-backed spec creation request.
type SpecCreateOptions struct {
	Slug    string
	ID      string
	Title   string
	Source  string
	Branch  string
	Related []string
	Body    string
	SetBody bool
}

// SpecCreateResult describes a created SQLite-backed spec.
type SpecCreateResult struct {
	ContractVersion    int         `json:"contract_version,omitempty"`
	DatabaseScope      string      `json:"database_scope,omitempty"`
	DatabasePath       string      `json:"database_path,omitempty"`
	ProjectID          string      `json:"project_id,omitempty"`
	ProjectName        string      `json:"project_name,omitempty"`
	ProjectCurrentPath string        `json:"project_current_path,omitempty"`
	Spec               TraceEntity   `json:"spec"`
	Source             string        `json:"source"`
	Branch             string        `json:"branch,omitempty"`
	Related            []TraceEntity `json:"related,omitempty"`
	EventID            string        `json:"event_id"`
}

// CreateSpec creates a draft spec in initialized SQLite state.
func CreateSpec(ctx context.Context, root project.Root, resolver PathResolver, options SpecCreateOptions) (SpecCreateResult, error) {
	store, err := openInitializedStore(root, resolver)
	if err != nil {
		return SpecCreateResult{}, err
	}
	defer store.Close()
	return store.CreateSpec(ctx, root, options)
}

// CreateSpec creates a draft spec in an open store.
func (s *Store) CreateSpec(ctx context.Context, root project.Root, options SpecCreateOptions) (SpecCreateResult, error) {
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return SpecCreateResult{}, err
	}
	identity, err := s.projectIdentity(ctx, projectID)
	if err != nil {
		return SpecCreateResult{}, err
	}
	slug := normalizeSpecSlug(options.Slug)
	if slug == "" {
		return SpecCreateResult{}, fmt.Errorf("spec create requires a slug")
	}
	title := strings.TrimSpace(options.Title)
	if title == "" {
		title = specTitleFromSlug(slug)
	}
	source := strings.TrimSpace(options.Source)
	if source == "" {
		source = "ad-hoc"
	}
	branch := strings.TrimSpace(options.Branch)

	// Resolve related specs before opening the write transaction. The store uses a
	// single SQLite connection, so resolution (which reads via s.db) must complete
	// before BeginTx holds that connection. Related specs must already exist.
	related := []TraceEntity{}
	seenRelated := map[string]bool{}
	for _, ref := range options.Related {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			continue
		}
		resolved, err := s.resolveTraceEntity(ctx, projectID, ref)
		if err != nil {
			return SpecCreateResult{}, fmt.Errorf("resolve related spec %q: %w", ref, err)
		}
		if resolved.Kind != "spec" {
			return SpecCreateResult{}, fmt.Errorf("related %q resolves to %s, not spec", ref, resolved.Kind)
		}
		if seenRelated[resolved.ID] {
			continue
		}
		seenRelated[resolved.ID] = true
		related = append(related, resolved)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return SpecCreateResult{}, fmt.Errorf("begin spec create transaction: %w", err)
	}
	defer tx.Rollback()

	alias, err := s.resolveSpecAlias(ctx, tx, root, projectID, options.ID)
	if err != nil {
		return SpecCreateResult{}, err
	}
	specID := stableMigrationID("spec", projectID, alias)
	now := time.Now().UTC().Format(time.RFC3339)

	// Provision a native source row so `spec finalize` renders the durable
	// document to .agents/specs/<alias>-<slug>.md. This stores only the path;
	// no file is written until finalize runs.
	relPath := filepath.ToSlash(filepath.Join(".agents", "specs", alias+"-"+slug+".md"))
	sourceID := stableMigrationID("source", projectID, relPath)
	if _, err := tx.ExecContext(ctx, `
INSERT INTO sources (id, project_id, source_kind, path, hash, imported_at, created_at, updated_at)
VALUES (?, ?, 'native', ?, NULL, NULL, ?, ?)
ON CONFLICT(id) DO UPDATE SET
  source_kind = excluded.source_kind,
  path = excluded.path,
  updated_at = excluded.updated_at
`, sourceID, projectID, relPath, now, now); err != nil {
		return SpecCreateResult{}, fmt.Errorf("provision spec source %s: %w", relPath, err)
	}

	if _, err := tx.ExecContext(ctx, `
INSERT INTO specs (id, project_id, title, status, branch, source, body_source_id, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
`, specID, projectID, title, LifecycleStatusDraft, nullableText(branch), source, sourceID, now, now); err != nil {
		return SpecCreateResult{}, fmt.Errorf("insert spec %s: %w", alias, err)
	}
	if err := insertAlias(ctx, tx, projectID, "spec", specID, "spec", alias, now); err != nil {
		return SpecCreateResult{}, err
	}

	for _, target := range related {
		relationshipID := stableMigrationID("relationship", projectID, "spec", specID, "related_to", "spec", target.ID)
		if _, err := tx.ExecContext(ctx, `
INSERT INTO relationships (id, project_id, from_entity_kind, from_entity_id, to_entity_kind, to_entity_id, relationship_type, reason, origin, created_at, updated_at)
VALUES (?, ?, 'spec', ?, 'spec', ?, 'related_to', 'recorded by spec new', 'command', ?, ?)
ON CONFLICT(id) DO NOTHING
`, relationshipID, projectID, specID, target.ID, now, now); err != nil {
			return SpecCreateResult{}, fmt.Errorf("record related spec relationship %s: %w", firstNonEmpty(target.Alias, target.ID), err)
		}
	}

	eventID := stableMigrationID("event", projectID, "spec", specID, "created", LifecycleStatusDraft)
	if _, err := tx.ExecContext(ctx, `
INSERT INTO events (id, project_id, entity_kind, entity_id, event_type, from_status, to_status, note, created_at, updated_at)
VALUES (?, ?, 'spec', ?, 'status_changed', NULL, ?, ?, ?, ?)
`, eventID, projectID, specID, LifecycleStatusDraft, specCreateEventNote(source), now, now); err != nil {
		return SpecCreateResult{}, fmt.Errorf("record spec create event: %w", err)
	}
	if options.SetBody {
		if _, err := upsertArtifactBodyTx(ctx, tx, projectID, "spec", specID, ArtifactBodyKindMarkdown, options.Body, sourceID, now); err != nil {
			return SpecCreateResult{}, err
		}
	}

	if err := tx.Commit(); err != nil {
		return SpecCreateResult{}, fmt.Errorf("commit spec create transaction: %w", err)
	}

	return SpecCreateResult{
		ContractVersion:    StateJSONContractVersion,
		DatabaseScope:      identity.DatabaseScope,
		DatabasePath:       identity.DatabasePath,
		ProjectID:          identity.ID,
		ProjectName:        identity.FriendlyName,
		ProjectCurrentPath: identity.CurrentPath,
		Spec:               TraceEntity{Kind: "spec", ID: specID, Alias: alias, Title: title, Status: LifecycleStatusDraft},
		Source:             source,
		Branch:             branch,
		Related:            related,
		EventID:            eventID,
	}, nil
}

// resolveSpecAlias validates an explicit SPEC-NNN id or allocates the next one.
func (s *Store) resolveSpecAlias(ctx context.Context, tx *sql.Tx, root project.Root, projectID string, explicit string) (string, error) {
	used, err := s.usedSpecNumbers(ctx, tx, root, projectID)
	if err != nil {
		return "", err
	}
	explicit = strings.ToUpper(strings.TrimSpace(explicit))
	if explicit != "" {
		match := specAliasPattern.FindStringSubmatch(explicit)
		if len(match) != 2 {
			return "", fmt.Errorf("invalid spec id %q (want SPEC-NNN)", explicit)
		}
		number, err := strconv.Atoi(match[1])
		if err != nil {
			return "", fmt.Errorf("invalid spec id %q (want SPEC-NNN)", explicit)
		}
		if used[number] {
			return "", fmt.Errorf("spec %s already exists", fmt.Sprintf("SPEC-%03d", number))
		}
		return fmt.Sprintf("SPEC-%03d", number), nil
	}
	maxID := 0
	for number := range used {
		if number > maxID {
			maxID = number
		}
	}
	return fmt.Sprintf("SPEC-%03d", maxID+1), nil
}

// usedSpecNumbers collects spec ids in use across SQLite aliases and
// .agents/specs/SPEC-*.md render files.
func (s *Store) usedSpecNumbers(ctx context.Context, tx *sql.Tx, root project.Root, projectID string) (map[int]bool, error) {
	used := map[int]bool{}
	rows, err := tx.QueryContext(ctx, `SELECT alias FROM aliases WHERE project_id = ? AND namespace = 'spec'`, projectID)
	if err != nil {
		return nil, fmt.Errorf("query spec aliases: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var alias string
		if err := rows.Scan(&alias); err != nil {
			return nil, fmt.Errorf("scan spec alias: %w", err)
		}
		if match := specAliasPattern.FindStringSubmatch(alias); len(match) == 2 {
			if number, err := strconv.Atoi(match[1]); err == nil {
				used[number] = true
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate spec aliases: %w", err)
	}
	for number := range specFileNumbers(root) {
		used[number] = true
	}
	return used, nil
}

func specFileNumbers(root project.Root) map[int]bool {
	numbers := map[int]bool{}
	dir := filepath.Join(root.Path(), ".agents", "specs")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return numbers
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		if match := specFilePattern.FindStringSubmatch(entry.Name()); len(match) == 2 {
			if number, err := strconv.Atoi(match[1]); err == nil {
				numbers[number] = true
			}
		}
	}
	return numbers
}

func normalizeSpecSlug(slug string) string {
	normalized := strings.ToLower(strings.TrimSpace(slug))
	normalized = specSlugCleaner.ReplaceAllString(normalized, "-")
	normalized = strings.Trim(normalized, "-")
	return normalized
}

func specTitleFromSlug(slug string) string {
	parts := strings.Split(slug, "-")
	for i, part := range parts {
		if part == "" {
			continue
		}
		parts[i] = strings.ToUpper(part[:1]) + part[1:]
	}
	return strings.Join(parts, " ")
}

func specCreateEventNote(source string) string {
	return fmt.Sprintf("recorded by spec new; source=%s", source)
}

// nullableText returns nil for empty input so optional TEXT columns store SQL NULL
// rather than an empty string, keeping new rows consistent with pre-migration rows.
func nullableText(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}
