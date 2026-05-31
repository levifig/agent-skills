package state

import (
	"context"
	"fmt"
	"os"

	"github.com/levifig/loaf/internal/project"
)

// SessionList is the state-backed session-list read model.
type SessionList struct {
	Version  int                    `json:"version"`
	Sessions map[string]SessionItem `json:"sessions"`
}

// SessionItem is a session entry returned by the state-backed session list.
type SessionItem struct {
	Branch           string `json:"branch,omitempty"`
	Status           string `json:"status"`
	HarnessSessionID string `json:"harness_session_id,omitempty"`
	SourcePath       string `json:"source_path,omitempty"`
	JournalEntries   int    `json:"journal_entries"`
}

// SessionListOptions filter the state-backed session list.
type SessionListOptions struct {
	All bool
}

// ListSessions returns imported sessions from initialized SQLite state.
func ListSessions(ctx context.Context, root project.Root, resolver PathResolver, options SessionListOptions) (SessionList, error) {
	databasePath, err := resolver.DatabasePath(root)
	if err != nil {
		return SessionList{}, err
	}
	if _, err := os.Stat(databasePath); os.IsNotExist(err) {
		return SessionList{}, fmt.Errorf("SQLite state database is not initialized; run `loaf state migrate markdown --apply` first")
	} else if err != nil {
		return SessionList{}, fmt.Errorf("inspect state database: %w", err)
	}
	store, err := OpenStore(databasePath)
	if err != nil {
		return SessionList{}, err
	}
	defer store.Close()
	return store.ListSessions(ctx, root, options)
}

// ListSessions returns imported sessions from an open store.
func (s *Store) ListSessions(ctx context.Context, root project.Root, options SessionListOptions) (SessionList, error) {
	projectID := ProjectID(root)
	rows, err := s.db.QueryContext(ctx, `
SELECT
  session_alias.alias,
  COALESCE(sessions.branch, ''),
  sessions.status,
  COALESCE(sessions.harness_session_id, ''),
  COALESCE(sources.path, ''),
  COUNT(journal_entries.id)
FROM sessions
JOIN aliases session_alias
  ON session_alias.project_id = sessions.project_id
 AND session_alias.entity_kind = 'session'
 AND session_alias.entity_id = sessions.id
 AND session_alias.namespace = 'session'
LEFT JOIN sources ON sources.id = sessions.body_source_id
LEFT JOIN journal_entries
  ON journal_entries.project_id = sessions.project_id
 AND journal_entries.session_id = sessions.id
WHERE sessions.project_id = ?
GROUP BY session_alias.alias, sessions.branch, sessions.status, sessions.harness_session_id, sources.path
ORDER BY session_alias.alias
`, projectID)
	if err != nil {
		return SessionList{}, fmt.Errorf("query sessions: %w", err)
	}

	sessionList := SessionList{Version: 1, Sessions: map[string]SessionItem{}}
	for rows.Next() {
		var alias, branch, status, harnessSessionID, sourcePath string
		var journalEntries int
		if err := rows.Scan(&alias, &branch, &status, &harnessSessionID, &sourcePath, &journalEntries); err != nil {
			rows.Close()
			return SessionList{}, fmt.Errorf("scan session: %w", err)
		}
		if !options.All && status == "archived" {
			continue
		}
		sessionList.Sessions[alias] = SessionItem{
			Branch:           branch,
			Status:           status,
			HarnessSessionID: harnessSessionID,
			SourcePath:       sourcePath,
			JournalEntries:   journalEntries,
		}
	}
	if err := rows.Close(); err != nil {
		return SessionList{}, fmt.Errorf("close sessions: %w", err)
	}
	if err := rows.Err(); err != nil {
		return SessionList{}, fmt.Errorf("iterate sessions: %w", err)
	}
	return sessionList, nil
}
