package state

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/levifig/loaf/internal/project"
)

// JournalRecentOptions filters the project journal timeline.
type JournalRecentOptions struct {
	// Branch restricts the timeline to entries observed on a single branch.
	Branch string
	// SinceLastWrap trims the timeline to entries logged after the most recent
	// wrap entry (project-scoped, or branch-scoped when Branch is set).
	SinceLastWrap bool
	// Limit caps the number of entries returned (most recent first). A value of
	// zero applies the default limit.
	Limit int
}

// JournalRecent is the state-backed project journal timeline read model.
type JournalRecent struct {
	ContractVersion    int                  `json:"contract_version,omitempty"`
	DatabaseScope      string               `json:"database_scope,omitempty"`
	DatabasePath       string               `json:"database_path,omitempty"`
	ProjectID          string               `json:"project_id,omitempty"`
	ProjectName        string               `json:"project_name,omitempty"`
	ProjectCurrentPath string               `json:"project_current_path,omitempty"`
	Branch             string               `json:"branch,omitempty"`
	SinceLastWrap      bool                 `json:"since_last_wrap"`
	Entries            []JournalEntryRecord `json:"entries"`
}

// JournalEntryRecord is one journal entry in a project-scoped timeline.
type JournalEntryRecord struct {
	ID               string `json:"id"`
	EntryType        string `json:"entry_type"`
	Scope            string `json:"scope,omitempty"`
	Message          string `json:"message"`
	ObservedBranch   string `json:"observed_branch,omitempty"`
	ObservedWorktree string `json:"observed_worktree,omitempty"`
	HarnessSessionID string `json:"harness_session_id,omitempty"`
	CreatedAt        string `json:"created_at"`
}

const defaultJournalRecentLimit = 20

// RecentJournal returns the project journal timeline from initialized SQLite state.
func RecentJournal(ctx context.Context, root project.Root, resolver PathResolver, options JournalRecentOptions) (JournalRecent, error) {
	store, err := openProjectStoreReadExisting(ctx, root, resolver)
	if err != nil {
		return JournalRecent{}, err
	}
	defer store.Close()
	return store.RecentJournal(ctx, root, options)
}

// LatestJournalEntryForScope returns the newest entry for one exact type/scope
// pair. It is intentionally a narrow read so derived projections do not infer
// lineage intent by parsing prose or requiring callers to scan a bounded feed.
func LatestJournalEntryForScope(ctx context.Context, root project.Root, resolver PathResolver, entryType, scope string) (JournalEntryRecord, bool, bool, error) {
	databasePath, err := resolver.DatabasePath(root)
	if err != nil {
		return JournalEntryRecord{}, false, false, err
	}
	if _, err := os.Stat(databasePath); errors.Is(err, os.ErrNotExist) {
		return JournalEntryRecord{}, false, false, nil
	} else if err != nil {
		return JournalEntryRecord{}, false, false, fmt.Errorf("inspect state database: %w", err)
	}
	store, err := OpenStoreReadOnly(databasePath)
	if err != nil {
		return JournalEntryRecord{}, false, false, err
	}
	defer store.Close()
	identity, err := store.LookupProjectIdentityForRoot(ctx, root)
	if errors.Is(err, sql.ErrNoRows) {
		return JournalEntryRecord{}, false, false, nil
	}
	if err != nil {
		return JournalEntryRecord{}, false, false, err
	}
	var entry JournalEntryRecord
	err = store.db.QueryRowContext(ctx, `
SELECT id, entry_type, COALESCE(scope, ''), message, COALESCE(observed_branch, ''), COALESCE(observed_worktree, ''), COALESCE(harness_session_id, ''), created_at
FROM journal_entries
WHERE project_id = ? AND entry_type = ? AND scope = ?
ORDER BY created_at DESC, rowid DESC
LIMIT 1`, identity.ID, entryType, scope).Scan(&entry.ID, &entry.EntryType, &entry.Scope, &entry.Message, &entry.ObservedBranch, &entry.ObservedWorktree, &entry.HarnessSessionID, &entry.CreatedAt)
	if err == sql.ErrNoRows {
		return JournalEntryRecord{}, false, true, nil
	}
	if err != nil {
		return JournalEntryRecord{}, false, true, err
	}
	return entry, true, true, nil
}

// RecentJournal returns the project journal timeline from an open store.
func (s *Store) RecentJournal(ctx context.Context, root project.Root, options JournalRecentOptions) (JournalRecent, error) {
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return JournalRecent{}, err
	}
	identity, err := s.projectIdentity(ctx, projectID)
	if err != nil {
		return JournalRecent{}, err
	}
	branch := strings.TrimSpace(options.Branch)
	limit := options.Limit
	if limit <= 0 {
		limit = defaultJournalRecentLimit
	}

	sinceCreatedAt := ""
	sinceRowID := int64(0)
	if options.SinceLastWrap {
		sinceCreatedAt, sinceRowID, err = s.latestWrapCutoff(ctx, projectID, branch)
		if err != nil {
			return JournalRecent{}, err
		}
	}

	entries, err := s.journalTimeline(ctx, projectID, branch, sinceCreatedAt, sinceRowID, limit)
	if err != nil {
		return JournalRecent{}, err
	}

	return JournalRecent{
		ContractVersion:    StateJSONContractVersion,
		DatabaseScope:      identity.DatabaseScope,
		DatabasePath:       identity.DatabasePath,
		ProjectID:          identity.ID,
		ProjectName:        identity.FriendlyName,
		ProjectCurrentPath: identity.CurrentPath,
		Branch:             branch,
		SinceLastWrap:      options.SinceLastWrap,
		Entries:            entries,
	}, nil
}

// latestWrapCutoff returns the (created_at, rowid) of the most recent wrap entry
// so a timeline can be trimmed to entries logged after it. When Branch is set the
// search is branch-scoped; otherwise it is project-scoped. Returns empty values
// when no wrap exists (the timeline then degrades to the full window).
func (s *Store) latestWrapCutoff(ctx context.Context, projectID string, branch string) (string, int64, error) {
	query := `
SELECT created_at, rowid
FROM journal_entries
WHERE project_id = ? AND entry_type = 'wrap'`
	args := []any{projectID}
	if branch != "" {
		query += ` AND observed_branch = ?`
		args = append(args, branch)
	}
	query += `
ORDER BY created_at DESC, rowid DESC
LIMIT 1`
	var createdAt string
	var rowID int64
	err := s.db.QueryRowContext(ctx, query, args...).Scan(&createdAt, &rowID)
	if err == sql.ErrNoRows {
		return "", 0, nil
	}
	if err != nil {
		return "", 0, fmt.Errorf("query latest wrap entry: %w", err)
	}
	return createdAt, rowID, nil
}

func (s *Store) journalTimeline(ctx context.Context, projectID string, branch string, sinceCreatedAt string, sinceRowID int64, limit int) ([]JournalEntryRecord, error) {
	query := `
SELECT
  id,
  entry_type,
  COALESCE(scope, ''),
  message,
  COALESCE(observed_branch, ''),
  COALESCE(observed_worktree, ''),
  COALESCE(harness_session_id, ''),
  created_at
FROM journal_entries
WHERE project_id = ?`
	args := []any{projectID}
	if branch != "" {
		query += ` AND observed_branch = ?`
		args = append(args, branch)
	}
	if sinceCreatedAt != "" {
		// Strictly newer than the wrap: later timestamp, or same timestamp with a
		// higher rowid (insertion order tiebreak for entries sharing a timestamp).
		query += ` AND (created_at > ? OR (created_at = ? AND rowid > ?))`
		args = append(args, sinceCreatedAt, sinceCreatedAt, sinceRowID)
	}
	query += `
ORDER BY created_at DESC, rowid DESC
LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query journal timeline: %w", err)
	}
	defer rows.Close()

	entries := []JournalEntryRecord{}
	for rows.Next() {
		var entry JournalEntryRecord
		if err := rows.Scan(
			&entry.ID,
			&entry.EntryType,
			&entry.Scope,
			&entry.Message,
			&entry.ObservedBranch,
			&entry.ObservedWorktree,
			&entry.HarnessSessionID,
			&entry.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan journal timeline entry: %w", err)
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate journal timeline entries: %w", err)
	}
	return entries, nil
}
