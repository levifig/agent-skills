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

const (
	SessionStartActionCreated       = "created"
	SessionStartActionAlreadyActive = "already-active"
	SessionStartActionResumed       = "resumed"
	SessionStartActionRotated       = "rotated"
)

// SessionStartOptions describes a SQLite-backed session start request.
type SessionStartOptions struct {
	Branch           string
	HarnessSessionID string
}

// SessionStartResult describes the active session after `loaf session start`.
type SessionStartResult struct {
	ContractVersion     int          `json:"contract_version,omitempty"`
	DatabaseScope       string       `json:"database_scope,omitempty"`
	DatabasePath        string       `json:"database_path,omitempty"`
	ProjectID           string       `json:"project_id,omitempty"`
	ProjectName         string       `json:"project_name,omitempty"`
	ProjectCurrentPath  string       `json:"project_current_path,omitempty"`
	Version             int          `json:"version"`
	Action              string       `json:"action"`
	Session             TraceEntity  `json:"session"`
	HarnessSessionID    string       `json:"harness_session_id,omitempty"`
	JournalEntryIDs     []string     `json:"journal_entry_ids,omitempty"`
	PreviousSession     *TraceEntity `json:"previous_session,omitempty"`
	PreviousJournalIDs  []string     `json:"previous_journal_entry_ids,omitempty"`
	PreviousSessionNote string       `json:"previous_session_note,omitempty"`
}

type sessionRow struct {
	ID               string
	Alias            string
	Branch           string
	Status           string
	HarnessSessionID string
}

// StartSession creates or resumes a session in initialized SQLite state.
func StartSession(ctx context.Context, root project.Root, resolver PathResolver, options SessionStartOptions) (SessionStartResult, error) {
	store, err := openInitializedStore(root, resolver)
	if err != nil {
		return SessionStartResult{}, err
	}
	defer store.Close()
	return store.StartSession(ctx, root, options)
}

// StartSession creates or resumes a session in an open store.
func (s *Store) StartSession(ctx context.Context, root project.Root, options SessionStartOptions) (SessionStartResult, error) {
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return SessionStartResult{}, err
	}
	identity, err := s.projectIdentity(ctx, projectID)
	if err != nil {
		return SessionStartResult{}, err
	}
	branch := strings.TrimSpace(options.Branch)
	if branch == "" {
		return SessionStartResult{}, fmt.Errorf("session start requires a git branch")
	}
	harnessSessionID := strings.TrimSpace(options.HarnessSessionID)
	now := time.Now().UTC().Format(time.RFC3339Nano)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return SessionStartResult{}, fmt.Errorf("begin session start transaction: %w", err)
	}
	defer tx.Rollback()

	var existing sessionRow
	if harnessSessionID != "" {
		existing, err = findSessionByHarnessID(ctx, tx, projectID, harnessSessionID)
		if err != nil {
			return SessionStartResult{}, err
		}
	}
	if existing.ID == "" {
		existing, err = findActiveSessionByBranch(ctx, tx, projectID, branch)
		if err != nil {
			return SessionStartResult{}, err
		}
	}

	var previous *TraceEntity
	previousJournalIDs := []string{}
	action := SessionStartActionCreated
	if existing.ID != "" && harnessSessionID != "" && existing.HarnessSessionID != "" && existing.HarnessSessionID != harnessSessionID {
		if err := updateSessionStatus(ctx, tx, projectID, existing.ID, "stopped", now); err != nil {
			return SessionStartResult{}, err
		}
		endID, err := insertSessionJournalEntry(ctx, tx, projectID, existing.ID, "session", "end", "closed by new conversation", now)
		if err != nil {
			return SessionStartResult{}, err
		}
		stopID, err := insertSessionJournalEntry(ctx, tx, projectID, existing.ID, "session", "stop", "=== SESSION STOPPED ===", now)
		if err != nil {
			return SessionStartResult{}, err
		}
		previous = &TraceEntity{Kind: "session", ID: existing.ID, Alias: existing.Alias, Status: "stopped"}
		previousJournalIDs = []string{endID, stopID}
		existing = sessionRow{}
		action = SessionStartActionRotated
	}

	var active sessionRow
	journalEntryIDs := []string{}
	if existing.ID != "" {
		active = existing
		action = SessionStartActionAlreadyActive
		if existing.Status != "active" {
			action = SessionStartActionResumed
			resumeID, err := insertSessionJournalEntry(ctx, tx, projectID, existing.ID, "session", "resume", resumeMessage(harnessSessionID), now)
			if err != nil {
				return SessionStartResult{}, err
			}
			journalEntryIDs = append(journalEntryIDs, resumeID)
		}
		if err := updateSessionActive(ctx, tx, projectID, existing.ID, branch, harnessSessionID, now); err != nil {
			return SessionStartResult{}, err
		}
		active.Branch = branch
		active.Status = "active"
		active.HarnessSessionID = firstNonEmpty(harnessSessionID, existing.HarnessSessionID)
	} else {
		alias, err := nextSessionAlias(ctx, tx, projectID, now)
		if err != nil {
			return SessionStartResult{}, err
		}
		sessionID := stableMigrationID("session", projectID, alias)
		if _, err := tx.ExecContext(ctx, `
INSERT INTO sessions (id, project_id, harness_session_id, branch, status, body_source_id, created_at, updated_at)
VALUES (?, ?, ?, ?, 'active', NULL, ?, ?)
`, sessionID, projectID, emptyToNil(harnessSessionID), branch, now, now); err != nil {
			return SessionStartResult{}, fmt.Errorf("insert session %s: %w", alias, err)
		}
		if err := insertAlias(ctx, tx, projectID, "session", sessionID, "session", alias, now); err != nil {
			return SessionStartResult{}, err
		}
		startID, err := insertSessionJournalEntry(ctx, tx, projectID, sessionID, "session", "start", startMessage(harnessSessionID), now)
		if err != nil {
			return SessionStartResult{}, err
		}
		if err := insertSessionStatusEvent(ctx, tx, projectID, sessionID, "", "active", "recorded by session start", now); err != nil {
			return SessionStartResult{}, err
		}
		active = sessionRow{ID: sessionID, Alias: alias, Branch: branch, Status: "active", HarnessSessionID: harnessSessionID}
		journalEntryIDs = append(journalEntryIDs, startID)
	}

	if err := tx.Commit(); err != nil {
		return SessionStartResult{}, fmt.Errorf("commit session start transaction: %w", err)
	}

	return SessionStartResult{
		ContractVersion:     StateJSONContractVersion,
		DatabaseScope:       identity.DatabaseScope,
		DatabasePath:        identity.DatabasePath,
		ProjectID:           identity.ID,
		ProjectName:         identity.FriendlyName,
		ProjectCurrentPath:  identity.CurrentPath,
		Version:             1,
		Action:              action,
		Session:             TraceEntity{Kind: "session", ID: active.ID, Alias: active.Alias, Status: active.Status},
		HarnessSessionID:    active.HarnessSessionID,
		JournalEntryIDs:     journalEntryIDs,
		PreviousSession:     previous,
		PreviousJournalIDs:  previousJournalIDs,
		PreviousSessionNote: previousSessionNote(previous),
	}, nil
}

func findSessionByHarnessID(ctx context.Context, tx *sql.Tx, projectID string, harnessSessionID string) (sessionRow, error) {
	return scanOptionalSession(ctx, tx, `
SELECT sessions.id, COALESCE(session_alias.alias, ''), COALESCE(sessions.branch, ''), sessions.status, COALESCE(sessions.harness_session_id, '')
FROM sessions
LEFT JOIN aliases session_alias
  ON session_alias.project_id = sessions.project_id
 AND session_alias.entity_kind = 'session'
 AND session_alias.entity_id = sessions.id
 AND session_alias.namespace = 'session'
WHERE sessions.project_id = ? AND sessions.harness_session_id = ?
ORDER BY sessions.updated_at DESC
LIMIT 1
`, projectID, harnessSessionID)
}

func findActiveSessionByBranch(ctx context.Context, tx *sql.Tx, projectID string, branch string) (sessionRow, error) {
	return scanOptionalSession(ctx, tx, `
SELECT sessions.id, COALESCE(session_alias.alias, ''), COALESCE(sessions.branch, ''), sessions.status, COALESCE(sessions.harness_session_id, '')
FROM sessions
LEFT JOIN aliases session_alias
  ON session_alias.project_id = sessions.project_id
 AND session_alias.entity_kind = 'session'
 AND session_alias.entity_id = sessions.id
 AND session_alias.namespace = 'session'
WHERE sessions.project_id = ? AND sessions.branch = ? AND sessions.status = 'active'
ORDER BY sessions.updated_at DESC
LIMIT 1
`, projectID, branch)
}

func scanOptionalSession(ctx context.Context, tx *sql.Tx, query string, args ...any) (sessionRow, error) {
	var row sessionRow
	err := tx.QueryRowContext(ctx, query, args...).Scan(&row.ID, &row.Alias, &row.Branch, &row.Status, &row.HarnessSessionID)
	if errors.Is(err, sql.ErrNoRows) {
		return sessionRow{}, nil
	}
	if err != nil {
		return sessionRow{}, fmt.Errorf("query session: %w", err)
	}
	return row, nil
}

func nextSessionAlias(ctx context.Context, tx *sql.Tx, projectID string, now string) (string, error) {
	base, err := time.Parse(time.RFC3339Nano, now)
	if err != nil {
		return "", fmt.Errorf("parse session timestamp: %w", err)
	}
	stem := base.UTC().Format("20060102-150405") + "-session"
	for suffix := 0; ; suffix++ {
		alias := stem
		if suffix > 0 {
			alias = fmt.Sprintf("%s-%d", stem, suffix+1)
		}
		var existing string
		err := tx.QueryRowContext(ctx, `SELECT id FROM aliases WHERE project_id = ? AND namespace = 'session' AND alias = ?`, projectID, alias).Scan(&existing)
		if errors.Is(err, sql.ErrNoRows) {
			return alias, nil
		}
		if err != nil {
			return "", fmt.Errorf("check session alias %s: %w", alias, err)
		}
	}
}

func updateSessionActive(ctx context.Context, tx *sql.Tx, projectID string, sessionID string, branch string, harnessSessionID string, now string) error {
	_, err := tx.ExecContext(ctx, `
UPDATE sessions
SET branch = ?,
    harness_session_id = COALESCE(?, harness_session_id),
    status = 'active',
    updated_at = ?
WHERE project_id = ? AND id = ?
`, branch, emptyToNil(harnessSessionID), now, projectID, sessionID)
	if err != nil {
		return fmt.Errorf("update active session: %w", err)
	}
	return insertSessionStatusEvent(ctx, tx, projectID, sessionID, "", "active", "recorded by session start", now)
}

func updateSessionStatus(ctx context.Context, tx *sql.Tx, projectID string, sessionID string, status string, now string) error {
	_, err := tx.ExecContext(ctx, `
UPDATE sessions
SET status = ?, updated_at = ?
WHERE project_id = ? AND id = ?
`, status, now, projectID, sessionID)
	if err != nil {
		return fmt.Errorf("update session status: %w", err)
	}
	return insertSessionStatusEvent(ctx, tx, projectID, sessionID, "", status, "recorded by session start", now)
}

func insertSessionJournalEntry(ctx context.Context, tx *sql.Tx, projectID string, sessionID string, entryType string, scope string, message string, now string) (string, error) {
	id := stableMigrationID("journal", projectID, sessionID, now, entryType, scope, message)
	_, err := tx.ExecContext(ctx, `
INSERT INTO journal_entries (id, project_id, entry_type, scope, message, session_id, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
`, id, projectID, entryType, emptyToNil(scope), message, sessionID, now, now)
	if err != nil {
		return "", fmt.Errorf("insert session journal entry: %w", err)
	}
	if err := insertJournalSearchTx(ctx, tx, projectID, id, sessionID, entryType, scope, message); err != nil {
		return "", err
	}
	return id, nil
}

func insertSessionStatusEvent(ctx context.Context, tx *sql.Tx, projectID string, sessionID string, fromStatus string, toStatus string, note string, now string) error {
	id := stableMigrationID("event", projectID, "session", sessionID, fromStatus, toStatus, now)
	_, err := tx.ExecContext(ctx, `
INSERT INTO events (id, project_id, entity_kind, entity_id, event_type, from_status, to_status, note, created_at, updated_at)
VALUES (?, ?, 'session', ?, 'status_changed', ?, ?, ?, ?, ?)
`, id, projectID, sessionID, emptyToNil(fromStatus), toStatus, note, now, now)
	if err != nil {
		return fmt.Errorf("insert session status event: %w", err)
	}
	return nil
}

func startMessage(harnessSessionID string) string {
	if harnessSessionID == "" {
		return "=== SESSION STARTED ==="
	}
	return fmt.Sprintf("=== SESSION STARTED === (session %s)", shortHarnessSessionID(harnessSessionID))
}

func resumeMessage(harnessSessionID string) string {
	if harnessSessionID == "" {
		return "=== SESSION RESUMED ==="
	}
	return fmt.Sprintf("=== SESSION RESUMED === (session %s)", shortHarnessSessionID(harnessSessionID))
}

func shortHarnessSessionID(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[:8]
}

func previousSessionNote(previous *TraceEntity) string {
	if previous == nil {
		return ""
	}
	return "previous active session stopped because a different harness session started on the same branch"
}
