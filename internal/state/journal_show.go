package state

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/levifig/loaf/internal/project"
)

// JournalShow is the state-backed single journal-entry read model.
type JournalShow struct {
	ContractVersion    int                `json:"contract_version,omitempty"`
	DatabaseScope      string             `json:"database_scope,omitempty"`
	DatabasePath       string             `json:"database_path,omitempty"`
	ProjectID          string             `json:"project_id,omitempty"`
	ProjectName        string             `json:"project_name,omitempty"`
	ProjectCurrentPath string             `json:"project_current_path,omitempty"`
	Query              string             `json:"query"`
	Entry              JournalEntryRecord `json:"entry"`
	Origin             *JournalOrigin     `json:"origin,omitempty"`
}

// ShowJournal returns one journal entry from initialized SQLite state.
func ShowJournal(ctx context.Context, root project.Root, resolver PathResolver, ref string) (JournalShow, error) {
	store, err := openProjectStoreReadExisting(ctx, root, resolver)
	if err != nil {
		return JournalShow{}, err
	}
	defer store.Close()
	return store.ShowJournal(ctx, root, ref)
}

// ShowJournal returns one journal entry from an open store, addressed by its id.
func (s *Store) ShowJournal(ctx context.Context, root project.Root, ref string) (JournalShow, error) {
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return JournalShow{}, err
	}
	identity, err := s.projectIdentity(ctx, projectID)
	if err != nil {
		return JournalShow{}, err
	}
	id := strings.TrimSpace(ref)
	if id == "" {
		return JournalShow{}, fmt.Errorf("journal show requires an entry id")
	}

	var entry JournalEntryRecord
	err = s.db.QueryRowContext(ctx, `
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
WHERE project_id = ? AND id = ?
`, projectID, id).Scan(
		&entry.ID,
		&entry.EntryType,
		&entry.Scope,
		&entry.Message,
		&entry.ObservedBranch,
		&entry.ObservedWorktree,
		&entry.HarnessSessionID,
		&entry.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return JournalShow{}, fmt.Errorf("journal entry %q not found in SQLite state", id)
	}
	if err != nil {
		return JournalShow{}, fmt.Errorf("read journal entry %s: %w", id, err)
	}
	origin, err := loadJournalOrigin(ctx, s, projectID, entry.ID)
	if err != nil {
		return JournalShow{}, err
	}

	return JournalShow{
		ContractVersion:    StateJSONContractVersion,
		DatabaseScope:      identity.DatabaseScope,
		DatabasePath:       identity.DatabasePath,
		ProjectID:          identity.ID,
		ProjectName:        identity.FriendlyName,
		ProjectCurrentPath: identity.CurrentPath,
		Query:              ref,
		Entry:              entry,
		Origin:             origin,
	}, nil
}
