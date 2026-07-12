package state

import (
	"context"

	"github.com/levifig/loaf/internal/project"
)

// SearchJournal runs a journal-only full-text search against initialized SQLite
// state. Unlike the global Search, it queries only the journal_search FTS table
// joined to journal_entries: it never touches the docs index, so a journal read
// cannot refresh docs state or fail on unrelated docs scanning (SPEC-056 M1).
func SearchJournal(ctx context.Context, root project.Root, resolver PathResolver, options SearchOptions) (SearchResult, error) {
	store, err := openProjectStoreReadExistingForJournalSearch(ctx, root, resolver)
	if err != nil {
		return SearchResult{}, err
	}
	defer store.Close()
	return store.SearchJournal(ctx, root, options)
}

// SearchJournal runs a journal-only full-text search using an open store. The
// result shape matches the journal hits `loaf journal search` already emits: a
// SearchResult whose Results carries only journal_entry hits, ranked by bm25.
func (s *Store) SearchJournal(ctx context.Context, root project.Root, options SearchOptions) (SearchResult, error) {
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return SearchResult{}, err
	}
	identity, err := s.projectIdentity(ctx, projectID)
	if err != nil {
		return SearchResult{}, err
	}
	if err := s.requireJournalSearchReady(ctx); err != nil {
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

	hits, err := s.searchJournalEntries(ctx, projectID, options.AllProjects, ftsQuery)
	if err != nil {
		return SearchResult{}, err
	}
	sortSearchHits(hits)
	if len(hits) > limit {
		hits = hits[:limit]
	}
	if hits == nil {
		hits = []SearchHit{}
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
