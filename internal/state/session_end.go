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
	SessionEndActionStopped       = "stopped"
	SessionEndActionDone          = "done"
	SessionEndActionCleared       = "cleared"
	SessionEndActionNoop          = "noop"
	SessionEndActionAlreadyClosed = "already-closed"
)

// SessionEndOptions describes a SQLite-backed session end request.
type SessionEndOptions struct {
	Branch           string
	HarnessSessionID string
	IfActive         bool
	Wrap             bool
	Clear            bool
}

// SessionEndResult describes the affected session after `loaf session end`.
type SessionEndResult struct {
	Version          int         `json:"version"`
	Action           string      `json:"action"`
	Session          TraceEntity `json:"session,omitempty"`
	HarnessSessionID string      `json:"harness_session_id,omitempty"`
	JournalEntryIDs  []string    `json:"journal_entry_ids,omitempty"`
	NoopReason       string      `json:"noop_reason,omitempty"`
}

// EndSession ends, wraps, or clears a session in initialized SQLite state.
func EndSession(ctx context.Context, root project.Root, resolver PathResolver, options SessionEndOptions) (SessionEndResult, error) {
	store, err := openInitializedStore(root, resolver)
	if err != nil {
		return SessionEndResult{}, err
	}
	defer store.Close()
	return store.EndSession(ctx, root, options)
}

// EndSession ends, wraps, or clears a session in an open store.
func (s *Store) EndSession(ctx context.Context, root project.Root, options SessionEndOptions) (SessionEndResult, error) {
	projectID := ProjectID(root)
	branch := strings.TrimSpace(options.Branch)
	harnessSessionID := strings.TrimSpace(options.HarnessSessionID)
	if harnessSessionID == "" && branch == "" {
		return SessionEndResult{}, fmt.Errorf("session end requires a git branch or harness session id")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return SessionEndResult{}, fmt.Errorf("begin session end transaction: %w", err)
	}
	defer tx.Rollback()

	var target sessionRow
	if harnessSessionID != "" {
		target, err = findSessionByHarnessID(ctx, tx, projectID, harnessSessionID)
		if err != nil {
			return SessionEndResult{}, err
		}
	}
	if target.ID == "" && branch != "" {
		target, err = findActiveSessionByBranch(ctx, tx, projectID, branch)
		if err != nil {
			return SessionEndResult{}, err
		}
	}
	if target.ID == "" {
		if options.IfActive {
			return SessionEndResult{Version: 1, Action: SessionEndActionNoop, NoopReason: "no active session found"}, nil
		}
		return SessionEndResult{}, fmt.Errorf("no active session found")
	}
	if target.Status != "active" {
		if options.IfActive {
			return SessionEndResult{
				Version:          1,
				Action:           SessionEndActionNoop,
				Session:          TraceEntity{Kind: "session", ID: target.ID, Alias: target.Alias, Status: target.Status},
				HarnessSessionID: target.HarnessSessionID,
				NoopReason:       fmt.Sprintf("session is %s", target.Status),
			}, nil
		}
		return SessionEndResult{
			Version:          1,
			Action:           SessionEndActionAlreadyClosed,
			Session:          TraceEntity{Kind: "session", ID: target.ID, Alias: target.Alias, Status: target.Status},
			HarnessSessionID: target.HarnessSessionID,
			NoopReason:       fmt.Sprintf("session is %s", target.Status),
		}, nil
	}

	action := SessionEndActionStopped
	status := "stopped"
	journalEntryIDs := []string{}
	switch {
	case options.Clear:
		action = SessionEndActionCleared
		status = "active"
		id, err := insertSessionJournalEntry(ctx, tx, projectID, target.ID, "session", "clear", "=== CONTEXT CLEARED ===", now)
		if err != nil {
			return SessionEndResult{}, err
		}
		journalEntryIDs = append(journalEntryIDs, id)
	case options.Wrap:
		action = SessionEndActionDone
		status = "done"
		id, err := insertSessionJournalEntry(ctx, tx, projectID, target.ID, "session", "wrap", "session ended", now)
		if err != nil {
			return SessionEndResult{}, err
		}
		journalEntryIDs = append(journalEntryIDs, id)
	default:
		endID, err := insertSessionJournalEntry(ctx, tx, projectID, target.ID, "session", "end", "session ended", now)
		if err != nil {
			return SessionEndResult{}, err
		}
		stopID, err := insertSessionJournalEntry(ctx, tx, projectID, target.ID, "session", "stop", "=== SESSION STOPPED ===", now)
		if err != nil {
			return SessionEndResult{}, err
		}
		journalEntryIDs = append(journalEntryIDs, endID, stopID)
	}
	if status != target.Status {
		if err := updateSessionStatusTransition(ctx, tx, projectID, target.ID, target.Status, status, "recorded by session end", now); err != nil {
			return SessionEndResult{}, err
		}
	} else if _, err := tx.ExecContext(ctx, `UPDATE sessions SET updated_at = ? WHERE project_id = ? AND id = ?`, now, projectID, target.ID); err != nil {
		return SessionEndResult{}, fmt.Errorf("touch session after end: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return SessionEndResult{}, fmt.Errorf("commit session end transaction: %w", err)
	}
	return SessionEndResult{
		Version:          1,
		Action:           action,
		Session:          TraceEntity{Kind: "session", ID: target.ID, Alias: target.Alias, Status: status},
		HarnessSessionID: firstNonEmpty(harnessSessionID, target.HarnessSessionID),
		JournalEntryIDs:  journalEntryIDs,
	}, nil
}

func updateSessionStatusTransition(ctx context.Context, tx *sql.Tx, projectID string, sessionID string, fromStatus string, toStatus string, note string, now string) error {
	_, err := tx.ExecContext(ctx, `
UPDATE sessions
SET status = ?, updated_at = ?
WHERE project_id = ? AND id = ?
`, toStatus, now, projectID, sessionID)
	if err != nil {
		return fmt.Errorf("update session status: %w", err)
	}
	return insertSessionStatusEvent(ctx, tx, projectID, sessionID, fromStatus, toStatus, note, now)
}
