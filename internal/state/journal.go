package state

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/levifig/loaf/internal/project"
)

// JournalLogOptions describes a journal entry write request.
type JournalLogOptions struct {
	Entry            string
	SessionRef       string
	ObservedBranch   string
	ObservedWorktree string
	HarnessSessionID string
	LinkSession      bool
	IfSessionActive  bool
}

// JournalLogResult is returned after a state-backed journal entry write.
type JournalLogResult struct {
	ContractVersion    int          `json:"contract_version,omitempty"`
	DatabaseScope      string       `json:"database_scope,omitempty"`
	DatabasePath       string       `json:"database_path,omitempty"`
	ProjectID          string       `json:"project_id,omitempty"`
	ProjectName        string       `json:"project_name,omitempty"`
	ProjectCurrentPath string       `json:"project_current_path,omitempty"`
	ID                 string       `json:"id"`
	EntryType          string       `json:"entry_type"`
	Scope              string       `json:"scope,omitempty"`
	Message            string       `json:"message"`
	ObservedBranch     string       `json:"observed_branch,omitempty"`
	ObservedWorktree   string       `json:"observed_worktree,omitempty"`
	HarnessSessionID   string       `json:"harness_session_id,omitempty"`
	Session            *TraceEntity `json:"session,omitempty"`
	NoopReason         string       `json:"noop_reason,omitempty"`
}

// LogJournal writes a journal entry into initialized SQLite state.
func LogJournal(ctx context.Context, root project.Root, resolver PathResolver, options JournalLogOptions) (JournalLogResult, error) {
	databasePath, err := resolver.DatabasePath(root)
	if err != nil {
		return JournalLogResult{}, err
	}
	if _, err := os.Stat(databasePath); os.IsNotExist(err) {
		return JournalLogResult{}, fmt.Errorf("SQLite state database is not initialized; run `loaf state init` first")
	} else if err != nil {
		return JournalLogResult{}, fmt.Errorf("inspect state database: %w", err)
	}
	store, err := OpenStore(databasePath)
	if err != nil {
		return JournalLogResult{}, err
	}
	defer store.Close()
	return store.LogJournal(ctx, root, options)
}

// LogJournal writes a journal entry into an open store.
func (s *Store) LogJournal(ctx context.Context, root project.Root, options JournalLogOptions) (JournalLogResult, error) {
	entryType, scope, message, err := parseJournalEntry(options.Entry)
	if err != nil {
		return JournalLogResult{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return JournalLogResult{}, err
	}
	identity, err := s.projectIdentity(ctx, projectID)
	if err != nil {
		return JournalLogResult{}, err
	}
	var session sessionRow
	if options.LinkSession {
		sessionRef := normalizeJournalSessionRef(options.SessionRef)
		if sessionRef != "" {
			entity, err := s.resolveTraceEntity(ctx, projectID, sessionRef)
			if err != nil {
				return JournalLogResult{}, err
			}
			if entity.Kind != "session" {
				return JournalLogResult{}, fmt.Errorf("%q resolves to %s, not session", options.SessionRef, entity.Kind)
			}
			session = sessionRow{ID: entity.ID, Alias: entity.Alias, Status: entity.Status}
		}
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return JournalLogResult{}, fmt.Errorf("begin journal transaction: %w", err)
		}
		defer tx.Rollback()

		harnessSessionID := strings.TrimSpace(options.HarnessSessionID)
		if session.ID == "" && harnessSessionID != "" {
			session, err = findSessionByHarnessID(ctx, tx, projectID, harnessSessionID)
			if err != nil {
				return JournalLogResult{}, err
			}
		}
		if session.ID == "" && strings.TrimSpace(options.ObservedBranch) != "" {
			session, err = findActiveSessionByBranch(ctx, tx, projectID, strings.TrimSpace(options.ObservedBranch))
			if err != nil {
				return JournalLogResult{}, err
			}
		}
		if session.ID == "" {
			if options.IfSessionActive {
				return JournalLogResult{
					ContractVersion:    StateJSONContractVersion,
					DatabaseScope:      identity.DatabaseScope,
					DatabasePath:       identity.DatabasePath,
					ProjectID:          identity.ID,
					ProjectName:        identity.FriendlyName,
					ProjectCurrentPath: identity.CurrentPath,
					EntryType:          entryType,
					Scope:              scope,
					Message:            message,
					ObservedBranch:     options.ObservedBranch,
					ObservedWorktree:   options.ObservedWorktree,
					HarnessSessionID:   options.HarnessSessionID,
					NoopReason:         "no active session found",
				}, nil
			}
			return JournalLogResult{}, fmt.Errorf("no active session found")
		}
		if session.Status == "stopped" {
			resumeID, err := insertSessionJournalEntry(ctx, tx, projectID, session.ID, "session", "resume", resumeMessage(harnessSessionID), now)
			if err != nil {
				return JournalLogResult{}, err
			}
			if err := updateSessionActive(ctx, tx, projectID, session.ID, firstNonEmpty(options.ObservedBranch, session.Branch), harnessSessionID, now); err != nil {
				return JournalLogResult{}, err
			}
			session.Status = "active"
			_ = resumeID
		}
		id := stableMigrationID("journal", projectID, session.ID, now, entryType, scope, message)
		_, err = tx.ExecContext(ctx, `
INSERT INTO journal_entries (
  id,
  project_id,
  entry_type,
  scope,
  message,
  observed_branch,
  observed_worktree,
  harness_session_id,
  session_id,
  spec_id,
  task_id,
  created_at,
  updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, NULL, ?, ?)
		`, id, projectID, entryType, emptyToNil(scope), message, emptyToNil(options.ObservedBranch), emptyToNil(options.ObservedWorktree), emptyToNil(options.HarnessSessionID), session.ID, now, now)
		if err != nil {
			return JournalLogResult{}, fmt.Errorf("insert journal entry: %w", err)
		}
		if err := insertJournalSearchTx(ctx, tx, projectID, id, session.ID, entryType, scope, message); err != nil {
			return JournalLogResult{}, err
		}
		if err := tx.Commit(); err != nil {
			return JournalLogResult{}, fmt.Errorf("commit journal transaction: %w", err)
		}
		return JournalLogResult{
			ContractVersion:    StateJSONContractVersion,
			DatabaseScope:      identity.DatabaseScope,
			DatabasePath:       identity.DatabasePath,
			ProjectID:          identity.ID,
			ProjectName:        identity.FriendlyName,
			ProjectCurrentPath: identity.CurrentPath,
			ID:                 id,
			EntryType:          entryType,
			Scope:              scope,
			Message:            message,
			ObservedBranch:     options.ObservedBranch,
			ObservedWorktree:   options.ObservedWorktree,
			HarnessSessionID:   options.HarnessSessionID,
			Session:            &TraceEntity{Kind: "session", ID: session.ID, Alias: session.Alias, Status: session.Status},
		}, nil
	}
	id := stableMigrationID("journal", projectID, now, entryType, scope, message)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return JournalLogResult{}, fmt.Errorf("begin journal transaction: %w", err)
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `
INSERT INTO journal_entries (
  id,
  project_id,
  entry_type,
  scope,
  message,
  observed_branch,
  observed_worktree,
  harness_session_id,
  session_id,
  spec_id,
  task_id,
  created_at,
  updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, NULL, NULL, NULL, ?, ?)
`, id, projectID, entryType, emptyToNil(scope), message, emptyToNil(options.ObservedBranch), emptyToNil(options.ObservedWorktree), emptyToNil(options.HarnessSessionID), now, now)
	if err != nil {
		return JournalLogResult{}, fmt.Errorf("insert journal entry: %w", err)
	}
	if err := insertJournalSearchTx(ctx, tx, projectID, id, "", entryType, scope, message); err != nil {
		return JournalLogResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return JournalLogResult{}, fmt.Errorf("commit journal transaction: %w", err)
	}
	return JournalLogResult{
		ContractVersion:    StateJSONContractVersion,
		DatabaseScope:      identity.DatabaseScope,
		DatabasePath:       identity.DatabasePath,
		ProjectID:          identity.ID,
		ProjectName:        identity.FriendlyName,
		ProjectCurrentPath: identity.CurrentPath,
		ID:                 id,
		EntryType:          entryType,
		Scope:              scope,
		Message:            message,
		ObservedBranch:     options.ObservedBranch,
		ObservedWorktree:   options.ObservedWorktree,
		HarnessSessionID:   options.HarnessSessionID,
	}, nil
}

// ObservedGitBranch returns the current branch name for context capture. It
// returns an empty string outside Git or in detached HEAD.
func ObservedGitBranch(worktree string) string {
	cmd := exec.Command("git", "branch", "--show-current")
	cmd.Dir = worktree
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

func parseJournalEntry(entry string) (string, string, string, error) {
	trimmed := strings.TrimSpace(entry)
	if trimmed == "" {
		return "", "", "", fmt.Errorf("session log entry cannot be empty")
	}
	re := regexp.MustCompile(`^([A-Za-z0-9_-]+)(?:\(([^)]*)\))?:\s*(.+)$`)
	matches := re.FindStringSubmatch(trimmed)
	if matches == nil {
		return "", "", "", fmt.Errorf("session log entry must look like type(scope): message")
	}
	return matches[1], strings.TrimSpace(matches[2]), strings.TrimSpace(matches[3]), nil
}

func normalizeJournalSessionRef(ref string) string {
	ref = strings.TrimSpace(strings.ReplaceAll(ref, "\\", "/"))
	ref = strings.TrimPrefix(ref, "./")
	for _, prefix := range []string{".agents/sessions/archive/", ".agents/sessions/", "sessions/archive/", "sessions/", "archive/"} {
		ref = strings.TrimPrefix(ref, prefix)
	}
	if strings.Contains(ref, "/") && strings.HasSuffix(ref, ".md") {
		ref = ref[strings.LastIndex(ref, "/")+1:]
	}
	return strings.TrimSuffix(ref, ".md")
}

func insertJournalSearchTx(ctx context.Context, tx *sql.Tx, projectID string, journalEntryID string, sessionID string, entryType string, scope string, message string) error {
	var rowID int64
	if err := tx.QueryRowContext(ctx, `
SELECT rowid FROM journal_entries
WHERE project_id = ? AND id = ?
`, projectID, journalEntryID).Scan(&rowID); err != nil {
		return fmt.Errorf("read journal entry rowid %s: %w", journalEntryID, err)
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO journal_search(rowid, project_id, journal_entry_id, session_id, entry_type, scope, message)
VALUES (?, ?, ?, ?, ?, ?, ?)
`, rowID, projectID, journalEntryID, firstNonEmpty(sessionID, ""), entryType, firstNonEmpty(scope, ""), message); err != nil {
		return fmt.Errorf("insert journal search row %s: %w", journalEntryID, err)
	}
	return nil
}
