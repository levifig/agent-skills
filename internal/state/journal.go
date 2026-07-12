package state

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/levifig/loaf/internal/project"
)

// JournalLogOptions describes a journal entry write request. Entries are
// project-scoped events tagged with an opaque harness_session_id correlation
// column; there is no session entity to open, close, or link.
type JournalLogOptions struct {
	Entry            string
	ObservedBranch   string
	ObservedWorktree string
	HarnessSessionID string
	Origin           *JournalOriginInput
}

// JournalLogResult is returned after a state-backed journal entry write.
type JournalLogResult struct {
	ContractVersion    int    `json:"contract_version,omitempty"`
	DatabaseScope      string `json:"database_scope,omitempty"`
	DatabasePath       string `json:"database_path,omitempty"`
	ProjectID          string `json:"project_id,omitempty"`
	ProjectName        string `json:"project_name,omitempty"`
	ProjectCurrentPath string `json:"project_current_path,omitempty"`
	ID                 string `json:"id"`
	EntryType          string `json:"entry_type"`
	Scope              string `json:"scope,omitempty"`
	Message            string `json:"message"`
	ObservedBranch     string `json:"observed_branch,omitempty"`
	ObservedWorktree   string `json:"observed_worktree,omitempty"`
	HarnessSessionID   string `json:"harness_session_id,omitempty"`
}

const JournalUnmatchedUnblockCode = "journal-unmatched-unblock"

// JournalUnmatchedUnblockError reports an unblock write whose exact key is not
// currently open. LatestSourceID and LatestSourceType are empty when the key
// has no historical block/unblock outcome.
type JournalUnmatchedUnblockError struct {
	Code             string
	Key              string
	LatestSourceID   string
	LatestSourceType string
}

func (e *JournalUnmatchedUnblockError) Error() string {
	if e.LatestSourceID == "" {
		return fmt.Sprintf("cannot unblock journal key %q: no matching block exists", e.Key)
	}
	return fmt.Sprintf("cannot unblock journal key %q: latest outcome is %s from %s", e.Key, e.LatestSourceType, e.LatestSourceID)
}

type journalLogHooks struct {
	afterCanonical func(*sql.Tx) error
	afterSearch    func(*sql.Tx) error
	afterOrigin    func(*sql.Tx) error
}

// queryContext is the minimal database surface needed to inspect SQLite
// metadata. Both *sql.Tx and *Store implement QueryContext, which lets the
// journal helpers operate against a transaction or a read-only store.
type queryContext interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

// QueryContext exposes the store's read query surface for helpers that need to
// work with either an open store or a transaction.
func (s *Store) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return s.db.QueryContext(ctx, query, args...)
}

// QueryRowContext exposes the store's single-row query surface for parity
// helpers that need to work with either an open store or a transaction.
func (s *Store) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	return s.db.QueryRowContext(ctx, query, args...)
}

// LogJournal writes a journal entry into initialized SQLite state.
func LogJournal(ctx context.Context, root project.Root, resolver PathResolver, options JournalLogOptions) (JournalLogResult, error) {
	store, err := openProjectStoreMutateExisting(ctx, root, resolver)
	if err != nil {
		return JournalLogResult{}, err
	}
	defer store.Close()
	return store.LogJournal(ctx, root, options)
}

// LogJournal writes a project-scoped journal entry into an open store. The entry
// carries an optional opaque harness_session_id correlation tag; nothing is
// opened, closed, or transitioned.
func (s *Store) LogJournal(ctx context.Context, root project.Root, options JournalLogOptions) (JournalLogResult, error) {
	return s.logJournalWithHooks(ctx, root, options, nil)
}

func (s *Store) logJournalWithHooks(ctx context.Context, root project.Root, options JournalLogOptions, hooks *journalLogHooks) (JournalLogResult, error) {
	entryType, scope, message, err := parseJournalEntry(options.Entry)
	if err != nil {
		return JournalLogResult{}, err
	}
	if (entryType == "block" || entryType == "unblock") && scope == "" {
		return JournalLogResult{}, fmt.Errorf("journal %s entries require a nonempty key in %s(<key>)", entryType, entryType)
	}
	var origin JournalOriginInput
	if options.Origin != nil {
		origin, err = normalizeJournalOriginInput(*options.Origin)
		if err != nil {
			return JournalLogResult{}, err
		}
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
	id := stableMigrationID("journal", projectID, now, entryType, scope, message)
	// SQLite maps serializable transactions to BEGIN IMMEDIATE. Taking the write
	// lock before inspecting the latest exact key outcome makes validation and
	// insertion one atomic operation across independent stores and processes.
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return JournalLogResult{}, fmt.Errorf("begin journal transaction: %w", err)
	}
	defer tx.Rollback()
	if entryType == "unblock" {
		if err := validateJournalUnblockTx(ctx, tx, projectID, scope); err != nil {
			return JournalLogResult{}, err
		}
	}
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
  spec_id,
  task_id,
  created_at,
  updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, NULL, NULL, ?, ?)
`, id, projectID, entryType, emptyToNil(scope), message, emptyToNil(options.ObservedBranch), emptyToNil(options.ObservedWorktree), emptyToNil(options.HarnessSessionID), now, now)
	if err != nil {
		return JournalLogResult{}, fmt.Errorf("insert journal entry: %w", err)
	}
	if err := runJournalLogHook(hooks, func(h *journalLogHooks) func(*sql.Tx) error { return h.afterCanonical }, tx); err != nil {
		return JournalLogResult{}, err
	}
	if err := insertJournalSearchTx(ctx, tx, projectID, id, options.HarnessSessionID, entryType, scope, message); err != nil {
		return JournalLogResult{}, err
	}
	if err := runJournalLogHook(hooks, func(h *journalLogHooks) func(*sql.Tx) error { return h.afterSearch }, tx); err != nil {
		return JournalLogResult{}, err
	}
	if options.Origin != nil {
		if err := insertJournalOriginTx(ctx, tx, id, origin); err != nil {
			return JournalLogResult{}, err
		}
	}
	if err := runJournalLogHook(hooks, func(h *journalLogHooks) func(*sql.Tx) error { return h.afterOrigin }, tx); err != nil {
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

func validateJournalUnblockTx(ctx context.Context, tx *sql.Tx, projectID, key string) error {
	var latestID, latestType string
	err := tx.QueryRowContext(ctx, `
SELECT id, entry_type
FROM journal_entries
WHERE project_id = ?
  AND scope = ?
  AND entry_type IN ('block', 'unblock')
ORDER BY created_at DESC, rowid DESC
LIMIT 1
`, projectID, key).Scan(&latestID, &latestType)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("inspect latest journal blocker outcome for key %q: %w", key, err)
	}
	if err == nil && latestType == "block" {
		return nil
	}
	return &JournalUnmatchedUnblockError{
		Code:             JournalUnmatchedUnblockCode,
		Key:              key,
		LatestSourceID:   latestID,
		LatestSourceType: latestType,
	}
}

func runJournalLogHook(hooks *journalLogHooks, selectHook func(*journalLogHooks) func(*sql.Tx) error, tx *sql.Tx) error {
	if hooks == nil {
		return nil
	}
	hook := selectHook(hooks)
	if hook == nil {
		return nil
	}
	return hook(tx)
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
		return "", "", "", fmt.Errorf("journal entry cannot be empty")
	}
	re := regexp.MustCompile(`^([A-Za-z0-9_-]+)(?:\(([^)]*)\))?:\s*(.+)$`)
	matches := re.FindStringSubmatch(trimmed)
	if matches == nil {
		return "", "", "", fmt.Errorf("journal entry must look like type(scope): message")
	}
	return matches[1], strings.TrimSpace(matches[2]), strings.TrimSpace(matches[3]), nil
}

func insertJournalSearchTx(ctx context.Context, tx *sql.Tx, projectID string, journalEntryID string, harnessSessionID string, entryType string, scope string, message string) error {
	var rowID int64
	if err := tx.QueryRowContext(ctx, `
SELECT rowid FROM journal_entries
WHERE project_id = ? AND id = ?
`, projectID, journalEntryID).Scan(&rowID); err != nil {
		return fmt.Errorf("read journal entry rowid %s: %w", journalEntryID, err)
	}
	// The FTS correlation column is journal-first (harness_session_id) after the
	// SPEC-056 migration but session_id on the pre-migration schema. Match the
	// live table so the write works on both shapes.
	correlationColumn, err := journalSearchCorrelationColumn(ctx, tx)
	if err != nil {
		return err
	}
	query := fmt.Sprintf(`
INSERT INTO journal_search(rowid, project_id, journal_entry_id, %s, entry_type, scope, message)
VALUES (?, ?, ?, ?, ?, ?, ?)
`, correlationColumn)
	if _, err := tx.ExecContext(ctx, query, rowID, projectID, journalEntryID, firstNonEmpty(harnessSessionID, ""), entryType, firstNonEmpty(scope, ""), message); err != nil {
		return fmt.Errorf("insert journal search row %s: %w", journalEntryID, err)
	}
	return nil
}

// journalSearchCorrelationColumn returns the name of the journal_search FTS
// correlation column, tolerating both the journal-first schema
// (harness_session_id) and the pre-migration schema (session_id).
func journalSearchCorrelationColumn(ctx context.Context, queryer queryContext) (string, error) {
	rows, err := queryer.QueryContext(ctx, `PRAGMA table_info(journal_search)`)
	if err != nil {
		return "", fmt.Errorf("inspect journal_search columns: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, columnType string
		var notNull int
		var defaultValue any
		var pk int
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &pk); err != nil {
			return "", fmt.Errorf("scan journal_search columns: %w", err)
		}
		if name == "harness_session_id" {
			return "harness_session_id", nil
		}
	}
	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("iterate journal_search columns: %w", err)
	}
	return "session_id", nil
}
