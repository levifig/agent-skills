package state

import (
	"context"
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
	ObservedBranch   string
	ObservedWorktree string
	HarnessSessionID string
}

// JournalLogResult is returned after a state-backed journal entry write.
type JournalLogResult struct {
	ID               string `json:"id"`
	EntryType        string `json:"entry_type"`
	Scope            string `json:"scope,omitempty"`
	Message          string `json:"message"`
	ObservedBranch   string `json:"observed_branch,omitempty"`
	ObservedWorktree string `json:"observed_worktree,omitempty"`
	HarnessSessionID string `json:"harness_session_id,omitempty"`
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

// LogJournal writes a journal entry into an open store. Unresolved session,
// spec, and task context intentionally remain null for this first write path.
func (s *Store) LogJournal(ctx context.Context, root project.Root, options JournalLogOptions) (JournalLogResult, error) {
	entryType, scope, message, err := parseJournalEntry(options.Entry)
	if err != nil {
		return JournalLogResult{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	projectID := ProjectID(root)
	id := stableMigrationID("journal", projectID, now, entryType, scope, message)
	_, err = s.db.ExecContext(ctx, `
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
	return JournalLogResult{
		ID:               id,
		EntryType:        entryType,
		Scope:            scope,
		Message:          message,
		ObservedBranch:   options.ObservedBranch,
		ObservedWorktree: options.ObservedWorktree,
		HarnessSessionID: options.HarnessSessionID,
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
