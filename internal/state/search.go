package state

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/levifig/loaf/internal/project"
)

// SearchOptions describes a Tier-1 SQLite search request.
type SearchOptions struct {
	Query       string
	AllProjects bool
	Limit       int
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

// SearchHit is one Tier-1 FTS result.
type SearchHit struct {
	Tier           string  `json:"tier"`
	Source         string  `json:"source"`
	ProjectID      string  `json:"project_id"`
	EntityKind     string  `json:"entity_kind,omitempty"`
	EntityID       string  `json:"entity_id,omitempty"`
	BodyKind       string  `json:"body_kind,omitempty"`
	JournalEntryID string  `json:"journal_entry_id,omitempty"`
	SessionID      string  `json:"session_id,omitempty"`
	EntryType      string  `json:"entry_type,omitempty"`
	Scope          string  `json:"scope,omitempty"`
	Snippet        string  `json:"snippet"`
	Rank           float64 `json:"rank"`
}

var searchTokenRE = regexp.MustCompile(`[[:alnum:]_]+`)

// Search queries Tier-1 SQLite-resident artifact bodies and journal entries.
func Search(ctx context.Context, root project.Root, resolver PathResolver, options SearchOptions) (SearchResult, error) {
	store, err := openInitializedStore(root, resolver)
	if err != nil {
		return SearchResult{}, err
	}
	defer store.Close()
	return store.Search(ctx, root, options)
}

// Search queries Tier-1 SQLite-resident artifact bodies and journal entries using an open store.
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
	limit := options.Limit
	if limit <= 0 {
		limit = 20
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
		return hits[i].JournalEntryID < hits[j].JournalEntryID
	})
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
		hit.Snippet = redactSearchSnippet(hit.Snippet)
		hits = append(hits, hit)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate artifact search hits: %w", err)
	}
	return hits, nil
}

func (s *Store) searchJournalEntries(ctx context.Context, projectID string, allProjects bool, ftsQuery string) ([]SearchHit, error) {
	query := `
SELECT project_id, journal_entry_id, session_id, entry_type, scope, snippet(journal_search, 5, '', '', '...', 12), bm25(journal_search)
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
		var sessionID sql.NullString
		var scope sql.NullString
		hit.Tier = "tier1"
		hit.Source = "journal_entry"
		if err := rows.Scan(&hit.ProjectID, &hit.JournalEntryID, &sessionID, &hit.EntryType, &scope, &hit.Snippet, &hit.Rank); err != nil {
			return nil, fmt.Errorf("scan journal search hit: %w", err)
		}
		hit.SessionID = sessionID.String
		hit.Scope = scope.String
		hit.Snippet = redactSearchSnippet(hit.Snippet)
		hits = append(hits, hit)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate journal search hits: %w", err)
	}
	return hits, nil
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
