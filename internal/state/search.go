package state

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/levifig/loaf/internal/project"
)

// SearchOptions describes a SQLite search request.
type SearchOptions struct {
	Query        string
	AllProjects  bool
	Limit        int
	WorktreePath string
}

// SearchResult is the state-backed search response.
type SearchResult struct {
	ContractVersion    int         `json:"contract_version,omitempty"`
	DatabaseScope      string      `json:"database_scope,omitempty"`
	DatabasePath       string      `json:"database_path,omitempty"`
	ProjectID          string      `json:"project_id,omitempty"`
	ProjectName        string      `json:"project_name,omitempty"`
	ProjectCurrentPath string      `json:"project_current_path,omitempty"`
	Query              string      `json:"query"`
	AllProjects        bool        `json:"all_projects"`
	Results            []SearchHit `json:"results"`
}

// SearchHit is one FTS result.
type SearchHit struct {
	Tier               string  `json:"tier"`
	Source             string  `json:"source"`
	ProjectID          string  `json:"project_id"`
	ProjectName        string  `json:"project_name,omitempty"`
	ProjectCurrentPath string  `json:"project_current_path,omitempty"`
	Locator            string  `json:"locator,omitempty"`
	EntityKind         string  `json:"entity_kind,omitempty"`
	EntityID           string  `json:"entity_id,omitempty"`
	BodyKind           string  `json:"body_kind,omitempty"`
	Path               string  `json:"path,omitempty"`
	LineStart          int     `json:"line_start,omitempty"`
	IndexedWorktree    string  `json:"indexed_worktree,omitempty"`
	JournalEntryID     string  `json:"journal_entry_id,omitempty"`
	HarnessSessionID   string  `json:"harness_session_id,omitempty"`
	EntryType          string  `json:"entry_type,omitempty"`
	Scope              string  `json:"scope,omitempty"`
	Snippet            string  `json:"snippet"`
	Rank               float64 `json:"rank"`
}

var searchTokenRE = regexp.MustCompile(`[[:alnum:]_]+`)

// Search queries SQLite-resident artifact bodies, journal entries, and indexed docs.
func Search(ctx context.Context, root project.Root, resolver PathResolver, options SearchOptions) (SearchResult, error) {
	store, err := openInitializedStore(root, resolver)
	if err != nil {
		return SearchResult{}, err
	}
	defer store.Close()
	return store.Search(ctx, root, options)
}

// Search queries SQLite-resident artifact bodies, journal entries, and indexed docs using an open store.
func (s *Store) Search(ctx context.Context, root project.Root, options SearchOptions) (SearchResult, error) {
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return SearchResult{}, err
	}
	identity, err := s.projectIdentity(ctx, projectID)
	if err != nil {
		return SearchResult{}, err
	}
	ftsQuery, err := searchFTSQuery(options.Query)
	if err != nil {
		return SearchResult{}, err
	}
	queryTokens := strings.Fields(ftsQuery)
	limit := options.Limit
	if limit <= 0 {
		limit = 20
	}
	indexedWorktree, err := docsIndexWorktreePath(root, options.WorktreePath)
	if err != nil {
		return SearchResult{}, err
	}
	indexedWorktree = filepath.ToSlash(indexedWorktree)
	if err := s.ensureDocsIndexFresh(ctx, root, projectID, indexedWorktree); err != nil {
		return SearchResult{}, err
	}

	hits, err := s.searchArtifactBodies(ctx, projectID, options.AllProjects, ftsQuery)
	if err != nil {
		return SearchResult{}, err
	}
	journalHits, err := s.searchJournalEntries(ctx, projectID, options.AllProjects, ftsQuery)
	if err != nil {
		return SearchResult{}, err
	}
	hits = append(hits, journalHits...)
	docHits, err := s.searchDocsIndex(ctx, projectID, indexedWorktree, options.AllProjects, ftsQuery, queryTokens)
	if err != nil {
		return SearchResult{}, err
	}
	hits = append(hits, docHits...)
	sortSearchHits(hits)
	if len(hits) > limit {
		hits = hits[:limit]
	}

	return SearchResult{
		ContractVersion:    StateJSONContractVersion,
		DatabaseScope:      identity.DatabaseScope,
		DatabasePath:       identity.DatabasePath,
		ProjectID:          identity.ID,
		ProjectName:        identity.FriendlyName,
		ProjectCurrentPath: identity.CurrentPath,
		Query:              options.Query,
		AllProjects:        options.AllProjects,
		Results:            hits,
	}, nil
}

func (s *Store) searchArtifactBodies(ctx context.Context, projectID string, allProjects bool, ftsQuery string) ([]SearchHit, error) {
	query := `
SELECT project_id, entity_kind, entity_id, body_kind, snippet(artifact_search, 4, '', '', '...', 12), bm25(artifact_search)
FROM artifact_search
WHERE artifact_search MATCH ?`
	args := []any{ftsQuery}
	if !allProjects {
		query += ` AND project_id = ?`
		args = append(args, projectID)
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("search artifact bodies: %w", err)
	}
	defer rows.Close()
	var hits []SearchHit
	for rows.Next() {
		var hit SearchHit
		hit.Tier = "tier1"
		hit.Source = "artifact_body"
		if err := rows.Scan(&hit.ProjectID, &hit.EntityKind, &hit.EntityID, &hit.BodyKind, &hit.Snippet, &hit.Rank); err != nil {
			return nil, fmt.Errorf("scan artifact search hit: %w", err)
		}
		hit.Locator = hit.EntityKind + "/" + hit.EntityID
		if hit.BodyKind != "" {
			hit.Locator += "#" + hit.BodyKind
		}
		hit.Snippet = redactSearchSnippet(hit.Snippet)
		hits = append(hits, hit)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate artifact search hits: %w", err)
	}
	return hits, nil
}

func (s *Store) searchJournalEntries(ctx context.Context, projectID string, allProjects bool, ftsQuery string) ([]SearchHit, error) {
	// The FTS correlation column (harness_session_id post-migration, session_id
	// on the pre-migration schema) is UNINDEXED passthrough metadata; selecting
	// the indexed message columns keeps this query schema-shape agnostic.
	query := `
SELECT project_id, journal_entry_id, entry_type, scope, snippet(journal_search, 5, '', '', '...', 12), bm25(journal_search)
FROM journal_search
WHERE journal_search MATCH ?`
	args := []any{ftsQuery}
	if !allProjects {
		query += ` AND project_id = ?`
		args = append(args, projectID)
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("search journal entries: %w", err)
	}
	defer rows.Close()
	var hits []SearchHit
	for rows.Next() {
		var hit SearchHit
		var scope sql.NullString
		hit.Tier = "tier1"
		hit.Source = "journal_entry"
		if err := rows.Scan(&hit.ProjectID, &hit.JournalEntryID, &hit.EntryType, &scope, &hit.Snippet, &hit.Rank); err != nil {
			return nil, fmt.Errorf("scan journal search hit: %w", err)
		}
		hit.Scope = scope.String
		hit.Locator = hit.JournalEntryID
		if hit.EntryType != "" {
			hit.Locator = hit.EntryType + ":" + hit.Locator
		}
		hit.Snippet = redactSearchSnippet(hit.Snippet)
		hits = append(hits, hit)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate journal search hits: %w", err)
	}
	return hits, nil
}

func (s *Store) searchDocsIndex(ctx context.Context, projectID string, indexedWorktree string, allProjects bool, ftsQuery string, queryTokens []string) ([]SearchHit, error) {
	query := `
SELECT
  docs_index.project_id,
  COALESCE(NULLIF(projects.friendly_name, ''), docs_index.project_id),
  COALESCE(current_path.path, projects.current_path, ''),
  docs_index.path,
  docs_index.indexed_worktree,
  docs_index.content,
  snippet(docs_search, 3, '', '', '...', 12),
  bm25(docs_search)
FROM docs_search
JOIN docs_index ON docs_index.rowid = docs_search.rowid
LEFT JOIN projects ON projects.id = docs_index.project_id
LEFT JOIN project_paths AS current_path
  ON current_path.project_id = docs_index.project_id
 AND current_path.is_current = 1
WHERE docs_search MATCH ?`
	args := []any{ftsQuery}
	if !allProjects {
		query += ` AND docs_index.project_id = ? AND docs_index.indexed_worktree = ?`
		args = append(args, projectID, indexedWorktree)
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("search docs index: %w", err)
	}
	defer rows.Close()
	var hits []SearchHit
	for rows.Next() {
		var hit SearchHit
		var content string
		hit.Tier = "tier2"
		hit.Source = "docs_index"
		if err := rows.Scan(&hit.ProjectID, &hit.ProjectName, &hit.ProjectCurrentPath, &hit.Path, &hit.IndexedWorktree, &content, &hit.Snippet, &hit.Rank); err != nil {
			return nil, fmt.Errorf("scan docs search hit: %w", err)
		}
		hit.LineStart = firstMatchingLine(content, queryTokens)
		hit.Locator = hit.Path
		if hit.LineStart > 0 {
			hit.Locator = fmt.Sprintf("%s:%d", hit.Path, hit.LineStart)
		}
		hit.Snippet = redactSearchSnippet(hit.Snippet)
		hits = append(hits, hit)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate docs search hits: %w", err)
	}
	return hits, nil
}

// sortSearchHits applies the canonical FTS ordering: ascending bm25 rank
// (SQLite returns more-relevant rows as smaller/more-negative bm25 scores),
// then a stable tiebreak chain on source and identity fields so results are
// deterministic across the global and journal-only search paths.
func sortSearchHits(hits []SearchHit) {
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].Rank != hits[j].Rank {
			return hits[i].Rank < hits[j].Rank
		}
		if hits[i].Source != hits[j].Source {
			return hits[i].Source < hits[j].Source
		}
		if hits[i].ProjectID != hits[j].ProjectID {
			return hits[i].ProjectID < hits[j].ProjectID
		}
		if hits[i].EntityKind != hits[j].EntityKind {
			return hits[i].EntityKind < hits[j].EntityKind
		}
		if hits[i].EntityID != hits[j].EntityID {
			return hits[i].EntityID < hits[j].EntityID
		}
		if hits[i].Path != hits[j].Path {
			return hits[i].Path < hits[j].Path
		}
		return hits[i].JournalEntryID < hits[j].JournalEntryID
	})
}

func firstMatchingLine(content string, tokens []string) int {
	loweredTokens := make([]string, 0, len(tokens))
	for _, token := range tokens {
		token = strings.ToLower(strings.TrimSpace(token))
		if token != "" {
			loweredTokens = append(loweredTokens, token)
		}
	}
	if len(loweredTokens) == 0 {
		return 0
	}
	for i, line := range strings.Split(content, "\n") {
		loweredLine := strings.ToLower(line)
		for _, token := range loweredTokens {
			if strings.Contains(loweredLine, token) {
				return i + 1
			}
		}
	}
	return 0
}

func (s *Store) ensureDocsIndexFresh(ctx context.Context, root project.Root, projectID string, indexedWorktree string) error {
	candidates, err := scanDocsIndexCandidates(indexedWorktree)
	if err != nil {
		return err
	}
	current := make(map[string]string, len(candidates))
	for _, candidate := range candidates {
		current[candidate.relativePath] = candidate.contentHash
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT path, content_hash
FROM docs_index
WHERE project_id = ? AND indexed_worktree = ?
`, projectID, indexedWorktree)
	if err != nil {
		return fmt.Errorf("query docs index freshness: %w", err)
	}
	stale := false
	indexed := 0
	for rows.Next() {
		var path string
		var hash string
		if err := rows.Scan(&path, &hash); err != nil {
			rows.Close()
			return fmt.Errorf("scan docs index freshness row: %w", err)
		}
		indexed++
		currentHash, ok := current[path]
		if !ok || currentHash != hash {
			stale = true
		}
		delete(current, path)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close docs index freshness rows: %w", err)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate docs index freshness rows: %w", err)
	}
	if indexed == 0 && len(candidates) == 0 {
		return nil
	}
	if stale || len(current) > 0 || indexed != len(candidates) {
		_, err := s.IndexDocs(ctx, root, DocsIndexOptions{WorktreePath: indexedWorktree})
		return err
	}
	return nil
}

func searchFTSQuery(query string) (string, error) {
	tokens := searchTokenRE.FindAllString(strings.TrimSpace(query), -1)
	if len(tokens) == 0 {
		return "", fmt.Errorf("search query must contain at least one searchable token")
	}
	return strings.Join(tokens, " "), nil
}

func redactSearchSnippet(snippet string) string {
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`sk-[A-Za-z0-9_-]{8,}`),
		regexp.MustCompile(`AKIA[0-9A-Z]{16}`),
		regexp.MustCompile(`gh[pousr]_[A-Za-z0-9_]{20,}`),
		regexp.MustCompile(`(?i)(pass` + `word|sec` + `ret|api` + `_` + `key)\s*[:=]\s*["']?[^"'\s]+`),
	}
	redacted := snippet
	for _, pattern := range patterns {
		redacted = pattern.ReplaceAllString(redacted, "[REDACTED_TOKEN]")
	}
	return redacted
}
