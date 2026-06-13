package state

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/levifig/loaf/internal/project"
)

const (
	SessionArchiveActionArchived        = "archived"
	SessionArchiveActionAlreadyArchived = "already-archived"
)

// SessionArchiveOptions describes a SQLite-backed session archive request.
type SessionArchiveOptions struct {
	Branch           string
	HarnessSessionID string
}

// SessionArchiveResult describes the affected session after `loaf session archive`.
type SessionArchiveResult struct {
	Version          int         `json:"version"`
	Action           string      `json:"action"`
	Session          TraceEntity `json:"session"`
	HarnessSessionID string      `json:"harness_session_id,omitempty"`
}

// ArchiveSession marks a session archived in initialized SQLite state.
func ArchiveSession(ctx context.Context, root project.Root, resolver PathResolver, options SessionArchiveOptions) (SessionArchiveResult, error) {
	store, err := openInitializedStore(root, resolver)
	if err != nil {
		return SessionArchiveResult{}, err
	}
	defer store.Close()
	return store.ArchiveSession(ctx, root, options)
}

// ArchiveSession marks a session archived in an open store.
func (s *Store) ArchiveSession(ctx context.Context, root project.Root, options SessionArchiveOptions) (SessionArchiveResult, error) {
	projectID := ProjectID(root)
	branch := strings.TrimSpace(options.Branch)
	harnessSessionID := strings.TrimSpace(options.HarnessSessionID)
	if branch == "" && harnessSessionID == "" {
		return SessionArchiveResult{}, fmt.Errorf("session archive requires a git branch or harness session id")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return SessionArchiveResult{}, fmt.Errorf("begin session archive transaction: %w", err)
	}
	defer tx.Rollback()

	var target sessionRow
	if harnessSessionID != "" {
		target, err = findSessionByHarnessID(ctx, tx, projectID, harnessSessionID)
		if err != nil {
			return SessionArchiveResult{}, err
		}
	}
	if target.ID == "" && branch != "" {
		target, err = findArchivableSessionByBranch(ctx, tx, projectID, branch)
		if err != nil {
			return SessionArchiveResult{}, err
		}
	}
	if target.ID == "" {
		return SessionArchiveResult{}, fmt.Errorf("no active session found")
	}
	if target.Status == "archived" {
		return SessionArchiveResult{
			Version:          1,
			Action:           SessionArchiveActionAlreadyArchived,
			Session:          TraceEntity{Kind: "session", ID: target.ID, Alias: target.Alias, Status: target.Status},
			HarnessSessionID: target.HarnessSessionID,
		}, nil
	}
	if err := updateSessionStatusTransition(ctx, tx, projectID, target.ID, target.Status, "archived", "recorded by session archive", now); err != nil {
		return SessionArchiveResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return SessionArchiveResult{}, fmt.Errorf("commit session archive transaction: %w", err)
	}
	return SessionArchiveResult{
		Version:          1,
		Action:           SessionArchiveActionArchived,
		Session:          TraceEntity{Kind: "session", ID: target.ID, Alias: target.Alias, Status: "archived"},
		HarnessSessionID: firstNonEmpty(harnessSessionID, target.HarnessSessionID),
	}, nil
}

func findArchivableSessionByBranch(ctx context.Context, tx *sql.Tx, projectID string, branch string) (sessionRow, error) {
	return scanOptionalSession(ctx, tx, `
SELECT sessions.id, COALESCE(session_alias.alias, ''), COALESCE(sessions.branch, ''), sessions.status, COALESCE(sessions.harness_session_id, '')
FROM sessions
LEFT JOIN aliases session_alias
  ON session_alias.project_id = sessions.project_id
 AND session_alias.entity_kind = 'session'
 AND session_alias.entity_id = sessions.id
 AND session_alias.namespace = 'session'
WHERE sessions.project_id = ? AND sessions.branch = ? AND sessions.status != 'archived'
ORDER BY sessions.updated_at DESC
LIMIT 1
`, projectID, branch)
}
