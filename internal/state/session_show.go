package state

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/levifig/loaf/internal/project"
)

// SessionShow is the state-backed single-session read model.
type SessionShow struct {
	ContractVersion    int           `json:"contract_version,omitempty"`
	DatabaseScope      string        `json:"database_scope,omitempty"`
	DatabasePath       string        `json:"database_path,omitempty"`
	ProjectID          string        `json:"project_id,omitempty"`
	ProjectName        string        `json:"project_name,omitempty"`
	ProjectCurrentPath string        `json:"project_current_path,omitempty"`
	Query              string        `json:"query"`
	Session            SessionDetail `json:"session"`
}

// SessionDetail contains operational session metadata plus journal context.
type SessionDetail struct {
	ID               string                `json:"id"`
	Alias            string                `json:"alias,omitempty"`
	Branch           string                `json:"branch,omitempty"`
	Status           string                `json:"status"`
	HarnessSessionID string                `json:"harness_session_id,omitempty"`
	Sources          []TraceSource         `json:"sources"`
	StateSnapshot    *SessionStateSnapshot `json:"state_snapshot,omitempty"`
	JournalEntries   []SessionJournalEntry `json:"journal_entries"`
	Relationships    []TraceRelationship   `json:"relationships"`
	CreatedAt        string                `json:"created_at"`
	UpdatedAt        string                `json:"updated_at"`
}

// SessionJournalEntry is a compact journal row shown with a session.
type SessionJournalEntry struct {
	ID        string `json:"id"`
	EntryType string `json:"entry_type"`
	Scope     string `json:"scope,omitempty"`
	Message   string `json:"message"`
	CreatedAt string `json:"created_at"`
}

// ShowSession returns one session from initialized SQLite state.
func ShowSession(ctx context.Context, root project.Root, resolver PathResolver, ref string) (SessionShow, error) {
	store, err := openInitializedStore(root, resolver)
	if err != nil {
		return SessionShow{}, err
	}
	defer store.Close()
	return store.ShowSession(ctx, root, ref)
}

// ShowSession returns one session from an open store.
func (s *Store) ShowSession(ctx context.Context, root project.Root, ref string) (SessionShow, error) {
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return SessionShow{}, err
	}
	identity, err := s.projectIdentity(ctx, projectID)
	if err != nil {
		return SessionShow{}, err
	}
	entity, err := s.resolveTraceEntity(ctx, projectID, ref)
	if err != nil {
		return SessionShow{}, err
	}
	if entity.Kind != "session" {
		return SessionShow{}, fmt.Errorf("session show target %q resolved to %s, not session", ref, entity.Kind)
	}

	session, err := s.sessionDetail(ctx, projectID, entity)
	if err != nil {
		return SessionShow{}, err
	}
	return SessionShow{
		ContractVersion:    StateJSONContractVersion,
		DatabaseScope:      identity.DatabaseScope,
		DatabasePath:       identity.DatabasePath,
		ProjectID:          identity.ID,
		ProjectName:        identity.FriendlyName,
		ProjectCurrentPath: identity.CurrentPath,
		Query:              ref,
		Session:            session,
	}, nil
}

func (s *Store) sessionDetail(ctx context.Context, projectID string, entity TraceEntity) (SessionDetail, error) {
	var status, createdAt, updatedAt string
	var branch, harnessSessionID, sourcePath, sourceHash sql.NullString
	err := s.db.QueryRowContext(ctx, `
SELECT
  COALESCE(sessions.branch, ''),
  sessions.status,
  COALESCE(sessions.harness_session_id, ''),
  sessions.created_at,
  sessions.updated_at,
  sources.path,
  sources.hash
FROM sessions
LEFT JOIN sources ON sources.id = sessions.body_source_id
WHERE sessions.project_id = ? AND sessions.id = ?
`, projectID, entity.ID).Scan(&branch, &status, &harnessSessionID, &createdAt, &updatedAt, &sourcePath, &sourceHash)
	if errors.Is(err, sql.ErrNoRows) {
		return SessionDetail{}, fmt.Errorf("session %q not found in SQLite state", firstNonEmpty(entity.Alias, entity.ID))
	}
	if err != nil {
		return SessionDetail{}, fmt.Errorf("read session %s: %w", entity.ID, err)
	}
	status = LifecycleStatusForDisplay(LifecycleEntitySession, status)

	alias := firstNonEmpty(entity.Alias)
	if alias == "" {
		if found, err := s.entityAlias(ctx, projectID, "session", entity.ID); err == nil {
			alias = found
		}
	}

	sources := []TraceSource{}
	if sourcePath.Valid && sourcePath.String != "" {
		sources = append(sources, TraceSource{Path: filepath.ToSlash(sourcePath.String), Hash: sourceHash.String})
	}

	journalEntries, err := s.sessionJournalEntries(ctx, projectID, entity.ID)
	if err != nil {
		return SessionDetail{}, err
	}
	stateSnapshot, err := s.latestSessionStateSnapshot(ctx, projectID, entity.ID)
	if err != nil {
		return SessionDetail{}, err
	}
	relationships, err := s.traceRelationships(ctx, projectID, TraceEntity{
		Kind:   "session",
		ID:     entity.ID,
		Alias:  alias,
		Status: status,
	})
	if err != nil {
		return SessionDetail{}, err
	}

	return SessionDetail{
		ID:               entity.ID,
		Alias:            alias,
		Branch:           branch.String,
		Status:           status,
		HarnessSessionID: harnessSessionID.String,
		Sources:          sources,
		StateSnapshot:    stateSnapshot,
		JournalEntries:   journalEntries,
		Relationships:    relationships,
		CreatedAt:        createdAt,
		UpdatedAt:        updatedAt,
	}, nil
}

func (s *Store) sessionJournalEntries(ctx context.Context, projectID string, sessionID string) ([]SessionJournalEntry, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, entry_type, COALESCE(scope, ''), message, created_at
FROM journal_entries
WHERE project_id = ? AND session_id = ?
ORDER BY created_at, rowid
`, projectID, sessionID)
	if err != nil {
		return nil, fmt.Errorf("query session journal entries: %w", err)
	}
	defer rows.Close()

	entries := []SessionJournalEntry{}
	for rows.Next() {
		var entry SessionJournalEntry
		if err := rows.Scan(&entry.ID, &entry.EntryType, &entry.Scope, &entry.Message, &entry.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan session journal entry: %w", err)
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate session journal entries: %w", err)
	}
	return entries, nil
}
