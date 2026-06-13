package state

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/levifig/loaf/internal/project"
)

// SessionStateSnapshotOptions describes a state-summary write for resumption.
type SessionStateSnapshotOptions struct {
	SessionRef       string
	Content          string
	ObservedBranch   string
	ObservedWorktree string
}

// SessionStateSnapshot is the latest durable session state summary.
type SessionStateSnapshot struct {
	ID               string `json:"id"`
	Content          string `json:"content"`
	ObservedBranch   string `json:"observed_branch,omitempty"`
	ObservedWorktree string `json:"observed_worktree,omitempty"`
	CreatedAt        string `json:"created_at"`
	UpdatedAt        string `json:"updated_at"`
}

// RecordSessionStateSnapshot upserts the latest state snapshot for a session.
func RecordSessionStateSnapshot(ctx context.Context, root project.Root, resolver PathResolver, options SessionStateSnapshotOptions) (SessionStateSnapshot, error) {
	store, err := openInitializedStore(root, resolver)
	if err != nil {
		return SessionStateSnapshot{}, err
	}
	defer store.Close()
	return store.RecordSessionStateSnapshot(ctx, root, options)
}

// RecordSessionStateSnapshot upserts the latest state snapshot for a session in an open store.
func (s *Store) RecordSessionStateSnapshot(ctx context.Context, root project.Root, options SessionStateSnapshotOptions) (SessionStateSnapshot, error) {
	projectID := ProjectID(root)
	content := strings.TrimSpace(options.Content)
	if content == "" {
		return SessionStateSnapshot{}, fmt.Errorf("session state snapshot content cannot be empty")
	}
	entity, err := s.resolveTraceEntity(ctx, projectID, options.SessionRef)
	if err != nil {
		return SessionStateSnapshot{}, err
	}
	if entity.Kind != "session" {
		return SessionStateSnapshot{}, fmt.Errorf("session state snapshot target %q resolved to %s, not session", options.SessionRef, entity.Kind)
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	id := stableMigrationID("session-state-snapshot", projectID, entity.ID)
	_, err = s.db.ExecContext(ctx, `
INSERT INTO session_state_snapshots (
  id,
  project_id,
  session_id,
  content,
  observed_branch,
  observed_worktree,
  created_at,
  updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(project_id, session_id) DO UPDATE SET
  content = excluded.content,
  observed_branch = excluded.observed_branch,
  observed_worktree = excluded.observed_worktree,
  updated_at = excluded.updated_at
`, id, projectID, entity.ID, content, emptyToNil(options.ObservedBranch), emptyToNil(options.ObservedWorktree), now, now)
	if err != nil {
		return SessionStateSnapshot{}, fmt.Errorf("upsert session state snapshot: %w", err)
	}
	return SessionStateSnapshot{
		ID:               id,
		Content:          content,
		ObservedBranch:   options.ObservedBranch,
		ObservedWorktree: options.ObservedWorktree,
		CreatedAt:        now,
		UpdatedAt:        now,
	}, nil
}

func (s *Store) latestSessionStateSnapshot(ctx context.Context, projectID string, sessionID string) (*SessionStateSnapshot, error) {
	var snapshot SessionStateSnapshot
	err := s.db.QueryRowContext(ctx, `
SELECT
  id,
  content,
  COALESCE(observed_branch, ''),
  COALESCE(observed_worktree, ''),
  created_at,
  updated_at
FROM session_state_snapshots
WHERE project_id = ? AND session_id = ?
LIMIT 1
`, projectID, sessionID).Scan(
		&snapshot.ID,
		&snapshot.Content,
		&snapshot.ObservedBranch,
		&snapshot.ObservedWorktree,
		&snapshot.CreatedAt,
		&snapshot.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read session state snapshot: %w", err)
	}
	return &snapshot, nil
}
