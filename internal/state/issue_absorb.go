package state

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/levifig/loaf/internal/project"
)

const (
	AbsorbDispositionAbsorbed   = "absorbed"
	AbsorbDispositionSuperseded = "superseded"

	intentAbsorbReasonPrefix = "absorbed into "
	intentSupersedeReason    = "superseded by loaf issue absorb --dismiss"
	absorbProvenancePrefix   = "Absorbed from "
)

// AbsorbOptions describes loaf issue absorb.
type AbsorbOptions struct {
	Ref     string
	Dismiss bool
	// Alias, when set, is stored as the minted issue alias without advancing
	// issue_identity.next_number. Linear mint uses this so the tracker key
	// becomes the alias. Ignored when Dismiss is true.
	Alias string
	// History allows done and archived leftover tasks. Single-ref absorb
	// never sets this; the project-wide projector does when --history is on.
	History bool
	// IssueStatus, when set, is applied in the same transaction as mint.
	// Empty or triage leaves the create default. Cancelled archives the issue.
	IssueStatus string
}

// AbsorbSource is the leftover task or intent being absorbed or dismissed.
type AbsorbSource struct {
	Kind              string        `json:"kind"`
	ID                string        `json:"id"`
	Alias             string        `json:"alias,omitempty"`
	Title             string        `json:"title"`
	Status            string        `json:"status,omitempty"`
	Priority          string        `json:"priority,omitempty"`
	Disposition       string        `json:"disposition,omitempty"`
	DispositionReason string        `json:"disposition_reason,omitempty"`
	Body              string        `json:"body,omitempty"`
	Sources           []TraceSource `json:"sources,omitempty"`
	DisplayRef        string        `json:"display_ref"`
}

// AbsorbResult is the mutation envelope for loaf issue absorb.
type AbsorbResult struct {
	ContractVersion    int          `json:"contract_version,omitempty"`
	DatabaseScope      string       `json:"database_scope,omitempty"`
	DatabasePath       string       `json:"database_path,omitempty"`
	ProjectID          string       `json:"project_id,omitempty"`
	ProjectName        string       `json:"project_name,omitempty"`
	ProjectCurrentPath string       `json:"project_current_path,omitempty"`
	Dismiss            bool         `json:"dismiss"`
	Disposition        string       `json:"disposition"`
	Source             AbsorbSource `json:"source"`
	Issue              *Issue       `json:"issue,omitempty"`
}

// IsChangeLocalTaskPath reports whether ref or an imported source path sits
// under docs/changes/**/tasks/. Those files are the Change corpus, not leftover
// SQLite task rows, and absorb refuses them.
func IsChangeLocalTaskPath(raw string) bool {
	p := filepath.ToSlash(strings.TrimSpace(raw))
	if p == "" {
		return false
	}
	p = strings.TrimPrefix(p, "./")
	const marker = "docs/changes/"
	idx := strings.Index(p, marker)
	if idx < 0 {
		return false
	}
	rest := p[idx+len(marker):]
	return rest == "tasks" || strings.HasPrefix(rest, "tasks/") || strings.HasSuffix(rest, "/tasks") || strings.Contains(rest, "/tasks/")
}

// FormatAbsorbProvenance writes the issue body section naming the source
// TASK-* or INTENT-* alias/id. It does not invent a relationship type.
func FormatAbsorbProvenance(source AbsorbSource) string {
	display := firstNonEmpty(source.Alias, source.ID)
	var b strings.Builder
	fmt.Fprintf(&b, "%s%s (%s:%s).", absorbProvenancePrefix, display, source.Kind, source.ID)
	if source.Title != "" {
		fmt.Fprintf(&b, "\n\n- Title: %s", source.Title)
	}
	if source.Status != "" {
		fmt.Fprintf(&b, "\n- Status: %s", source.Status)
	}
	if source.Priority != "" {
		fmt.Fprintf(&b, "\n- Priority: %s", source.Priority)
	}
	if source.Disposition != "" {
		fmt.Fprintf(&b, "\n- Disposition: %s", source.Disposition)
	}
	if strings.TrimSpace(source.Body) != "" {
		fmt.Fprintf(&b, "\n\n%s", source.Body)
	}
	return b.String()
}

// LookupAbsorbSource resolves and validates a leftover task or intent without writing.
func LookupAbsorbSource(ctx context.Context, root project.Root, resolver PathResolver, ref string) (AbsorbSource, error) {
	store, err := openProjectStoreReadExisting(ctx, root, resolver)
	if err != nil {
		return AbsorbSource{}, err
	}
	defer store.Close()
	projectID, err := store.projectID(ctx, root)
	if err != nil {
		return AbsorbSource{}, err
	}
	return store.lookupAbsorbSource(ctx, root, projectID, ref, false)
}

// Absorb mints (unless dismissed) an issue from a leftover task or intent and
// archives the source. It never registers TASK-* / INTENT-* as a live issue alias.
func Absorb(ctx context.Context, root project.Root, resolver PathResolver, options AbsorbOptions) (AbsorbResult, error) {
	store, err := openProjectStoreMutateExisting(ctx, root, resolver)
	if err != nil {
		return AbsorbResult{}, err
	}
	defer store.Close()
	return store.Absorb(ctx, root, options)
}

// Absorb is the store-level absorb mutation.
func (s *Store) Absorb(ctx context.Context, root project.Root, options AbsorbOptions) (AbsorbResult, error) {
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return AbsorbResult{}, err
	}
	identity, err := s.projectIdentity(ctx, projectID)
	if err != nil {
		return AbsorbResult{}, err
	}
	source, err := s.lookupAbsorbSource(ctx, root, projectID, options.Ref, options.History)
	if err != nil {
		return AbsorbResult{}, err
	}
	if err := validateAbsorbIssueStatus(options.IssueStatus); err != nil {
		return AbsorbResult{}, err
	}

	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return AbsorbResult{}, &IssueTransactionError{Stage: "begin absorb", Err: err}
	}
	defer tx.Rollback()

	now := time.Now().UTC().Format(time.RFC3339Nano)
	disposition := AbsorbDispositionAbsorbed
	if options.Dismiss {
		disposition = AbsorbDispositionSuperseded
	}

	var issue *Issue
	if !options.Dismiss {
		created, err := createIssueTx(ctx, tx, projectID, IssueCreateOptions{
			Title: source.Title,
			Body:  FormatAbsorbProvenance(source),
			Alias: strings.TrimSpace(options.Alias),
		}, now)
		if err != nil {
			return AbsorbResult{}, err
		}
		created, err = applyAbsorbIssueStatusTx(ctx, tx, projectID, created, options.IssueStatus, now)
		if err != nil {
			return AbsorbResult{}, err
		}
		issue = &created
	}

	note := absorbArchiveNote(disposition, issue)
	if err := archiveAbsorbSourceTx(ctx, tx, projectID, source, note, now, options.History); err != nil {
		return AbsorbResult{}, err
	}

	if err := tx.Commit(); err != nil {
		return AbsorbResult{}, &IssueTransactionError{Stage: "commit absorb", Err: err}
	}

	if source.Kind == "task" {
		source.Status = LifecycleStatusArchived
	} else {
		source.Disposition = "resolved"
	}
	return AbsorbResult{
		ContractVersion:    StateJSONContractVersion,
		DatabaseScope:      identity.DatabaseScope,
		DatabasePath:       identity.DatabasePath,
		ProjectID:          identity.ID,
		ProjectName:        identity.FriendlyName,
		ProjectCurrentPath: identity.CurrentPath,
		Dismiss:            options.Dismiss,
		Disposition:        disposition,
		Source:             source,
		Issue:              issue,
	}, nil
}

func (s *Store) lookupAbsorbSource(ctx context.Context, root project.Root, projectID, ref string, history bool) (AbsorbSource, error) {
	trimmed := strings.TrimSpace(ref)
	if trimmed == "" {
		return AbsorbSource{}, fmt.Errorf("issue absorb requires a task or intent ref")
	}
	if IsChangeLocalTaskPath(trimmed) {
		return AbsorbSource{}, fmt.Errorf("issue absorb refuses change-local task files under docs/changes/**/tasks/")
	}
	entity, err := s.resolveTraceEntity(ctx, projectID, trimmed)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return AbsorbSource{}, fmt.Errorf("issue absorb target %q not found in SQLite state", trimmed)
		}
		return AbsorbSource{}, err
	}
	switch entity.Kind {
	case "task":
		return s.absorbSourceFromTask(ctx, root, projectID, entity, history)
	case "intent":
		return s.absorbSourceFromIntent(ctx, projectID, entity)
	default:
		return AbsorbSource{}, fmt.Errorf("issue absorb requires a task or intent ref; %q resolves to %s", trimmed, entity.Kind)
	}
}

func (s *Store) absorbSourceFromTask(ctx context.Context, root project.Root, projectID string, entity TraceEntity, history bool) (AbsorbSource, error) {
	source, err := s.loadAbsorbTaskSource(ctx, root, projectID, entity)
	if err != nil {
		return AbsorbSource{}, err
	}
	if absorbSourceIsChangeLocal(source) {
		return AbsorbSource{}, fmt.Errorf("issue absorb refuses change-local task files under docs/changes/**/tasks/")
	}
	if err := refuseAbsorbTaskStatus(source.DisplayRef, source.Status, history); err != nil {
		return AbsorbSource{}, err
	}
	if history {
		note, err := s.latestTaskAbsorbNote(ctx, projectID, source.ID)
		if err != nil {
			return AbsorbSource{}, err
		}
		if intentAbsorbAlreadyTerminal(note) {
			return AbsorbSource{}, fmt.Errorf("issue absorb source %s is already %s", source.DisplayRef, intentAbsorbDispositionLabel(note))
		}
	}
	return source, nil
}

func (s *Store) loadAbsorbTaskSource(ctx context.Context, root project.Root, projectID string, entity TraceEntity) (AbsorbSource, error) {
	detail, err := s.taskDetail(ctx, root, projectID, entity)
	if err != nil {
		return AbsorbSource{}, err
	}
	display := firstNonEmpty(detail.Alias, detail.ID)
	return AbsorbSource{
		Kind:       "task",
		ID:         detail.ID,
		Alias:      detail.Alias,
		Title:      detail.Title,
		Status:     detail.Status,
		Priority:   detail.Priority,
		Body:       detail.Body,
		Sources:    detail.Sources,
		DisplayRef: display,
	}, nil
}

func (s *Store) absorbSourceFromIntent(ctx context.Context, projectID string, entity TraceEntity) (AbsorbSource, error) {
	source, err := s.loadAbsorbIntentSource(ctx, projectID, entity.ID)
	if err != nil {
		return AbsorbSource{}, err
	}
	if intentAbsorbAlreadyTerminal(source.DispositionReason) {
		return AbsorbSource{}, fmt.Errorf("issue absorb source %s is already %s", source.DisplayRef, intentAbsorbDispositionLabel(source.DispositionReason))
	}
	return source, nil
}

func (s *Store) loadAbsorbIntentSource(ctx context.Context, projectID, intentID string) (AbsorbSource, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return AbsorbSource{}, fmt.Errorf("begin intent absorb lookup: %w", err)
	}
	defer tx.Rollback()
	return loadAbsorbIntentSourceTx(ctx, tx, projectID, intentID)
}

func loadAbsorbIntentSourceTx(ctx context.Context, tx *sql.Tx, projectID, intentID string) (AbsorbSource, error) {
	detail, err := loadIntentDetailTx(ctx, tx, projectID, intentID)
	if err != nil {
		return AbsorbSource{}, err
	}
	display := firstNonEmpty(detail.Alias, detail.ID)
	return AbsorbSource{
		Kind:              "intent",
		ID:                detail.ID,
		Alias:             detail.Alias,
		Title:             detail.Title,
		Disposition:       detail.Disposition,
		DispositionReason: detail.DispositionReason,
		Body:              detail.Body,
		DisplayRef:        display,
	}, nil
}

func archiveAbsorbSourceTx(ctx context.Context, tx *sql.Tx, projectID string, source AbsorbSource, note, now string, history bool) error {
	switch source.Kind {
	case "task":
		return archiveAbsorbTaskTx(ctx, tx, projectID, source, note, now, history)
	case "intent":
		return archiveAbsorbIntentTx(ctx, tx, projectID, source, note, now)
	default:
		return fmt.Errorf("issue absorb requires a task or intent ref; got %s", source.Kind)
	}
}

func archiveAbsorbTaskTx(ctx context.Context, tx *sql.Tx, projectID string, source AbsorbSource, note, now string, history bool) error {
	var previous string
	err := tx.QueryRowContext(ctx, `SELECT status FROM tasks WHERE project_id = ? AND id = ?`, projectID, source.ID).Scan(&previous)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("issue absorb source %s is not found in SQLite state", source.DisplayRef)
	}
	if err != nil {
		return fmt.Errorf("read task status: %w", err)
	}
	if err := refuseAbsorbTaskStatus(source.DisplayRef, previous, history); err != nil {
		return err
	}
	existingNote, err := latestTaskAbsorbNoteTx(ctx, tx, projectID, source.ID)
	if err != nil {
		return err
	}
	if intentAbsorbAlreadyTerminal(existingNote) {
		return fmt.Errorf("issue absorb source %s is already %s", source.DisplayRef, intentAbsorbDispositionLabel(existingNote))
	}
	if !LifecycleStatusMatches(LifecycleEntityTask, previous, LifecycleStatusArchived) {
		if _, err := tx.ExecContext(ctx, `UPDATE tasks SET status = ?, updated_at = ? WHERE project_id = ? AND id = ?`, LifecycleStatusArchived, now, projectID, source.ID); err != nil {
			return fmt.Errorf("archive absorbed task: %w", err)
		}
	}
	eventID := stableMigrationID("event", projectID, "task", source.ID, "status", previous, LifecycleStatusArchived, note)
	_, err = tx.ExecContext(ctx, `
INSERT INTO events (id, project_id, entity_kind, entity_id, event_type, from_status, to_status, note, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO NOTHING
`, eventID, projectID, "task", source.ID, "status_changed", previous, LifecycleStatusArchived, note, now, now)
	if err != nil {
		return fmt.Errorf("record absorbed task event: %w", err)
	}
	return nil
}

func archiveAbsorbIntentTx(ctx context.Context, tx *sql.Tx, projectID string, source AbsorbSource, note, now string) error {
	current, err := latestIntentDispositionTx(ctx, tx, projectID, source.ID)
	if err != nil {
		return err
	}
	if intentAbsorbAlreadyTerminal(current.Reason.String) {
		return fmt.Errorf("issue absorb source %s is already %s", source.DisplayRef, intentAbsorbDispositionLabel(current.Reason.String))
	}
	seq, err := nextAggregateSeq(ctx, tx, "intent_dispositions", "intent_id", source.ID)
	if err != nil {
		return &IntentTransactionError{Stage: "sequence", Err: err}
	}
	dispositionID := stableMigrationID("intent-disposition", projectID, source.ID, fmt.Sprintf("%d", seq), note)
	if _, err := tx.ExecContext(ctx, `
INSERT INTO intent_dispositions (id, project_id, intent_id, seq, disposition, reason, deferral_id, supersedes_deferral_id, created_at)
VALUES (?, ?, ?, ?, ?, ?, NULL, NULL, ?)
`, dispositionID, projectID, source.ID, seq, "resolved", note, now); err != nil {
		return &IntentTransactionError{Stage: "disposition", Err: err}
	}
	return nil
}

func absorbArchiveNote(disposition string, issue *Issue) string {
	if disposition == AbsorbDispositionSuperseded {
		return intentSupersedeReason
	}
	if issue == nil {
		return strings.TrimSpace(intentAbsorbReasonPrefix)
	}
	target := firstNonEmpty(issue.Alias, issue.ID)
	return fmt.Sprintf("%s%s (issue:%s)", intentAbsorbReasonPrefix, target, issue.ID)
}

func leftoverOpenTaskStatus(status string) bool {
	for _, open := range []string{
		LifecycleStatusTodo,
		LifecycleStatusInProgress,
		LifecycleStatusBlocked,
		LifecycleStatusReview,
	} {
		if LifecycleStatusMatches(LifecycleEntityTask, status, open) {
			return true
		}
	}
	return false
}

func leftoverHistoryTaskStatus(status string) bool {
	return LifecycleStatusMatches(LifecycleEntityTask, status, LifecycleStatusDone) ||
		LifecycleStatusMatches(LifecycleEntityTask, status, LifecycleStatusArchived)
}

func leftoverNonTerminalIntent(disposition string) bool {
	return strings.TrimSpace(disposition) != "resolved"
}

func refuseAbsorbTaskStatus(display, status string, history bool) error {
	if leftoverOpenTaskStatus(status) {
		return nil
	}
	if history && leftoverHistoryTaskStatus(status) {
		return nil
	}
	shown := LifecycleStatusForDisplay(LifecycleEntityTask, status)
	return fmt.Errorf("issue absorb source %s is not leftover open work (status: %s)", display, shown)
}

func absorbSourceIsChangeLocal(source AbsorbSource) bool {
	if IsChangeLocalTaskPath(source.DisplayRef) {
		return true
	}
	for _, item := range source.Sources {
		if IsChangeLocalTaskPath(item.Path) {
			return true
		}
	}
	return false
}

func validateAbsorbIssueStatus(status string) error {
	switch strings.TrimSpace(status) {
	case "", IssueStatusTriage, IssueStatusBacklog, IssueStatusTodo, IssueStatusDone, IssueStatusCancelled:
		return nil
	default:
		return fmt.Errorf("issue absorb cannot mint status %s", strings.TrimSpace(status))
	}
}

func applyAbsorbIssueStatusTx(ctx context.Context, tx *sql.Tx, projectID string, issue Issue, status, now string) (Issue, error) {
	status = strings.TrimSpace(status)
	if status == "" || status == IssueStatusTriage {
		return issue, nil
	}
	if err := validateAbsorbIssueStatus(status); err != nil {
		return Issue{}, err
	}
	if status == IssueStatusCancelled {
		if _, err := tx.ExecContext(ctx, `
UPDATE issues SET status = ?, archived_at = ?, updated_at = ? WHERE project_id = ? AND id = ?
`, status, now, now, projectID, issue.ID); err != nil {
			return Issue{}, &IssueTransactionError{Stage: "absorb status", Err: err}
		}
	} else if _, err := tx.ExecContext(ctx, `
UPDATE issues SET status = ?, updated_at = ? WHERE project_id = ? AND id = ?
`, status, now, projectID, issue.ID); err != nil {
		return Issue{}, &IssueTransactionError{Stage: "absorb status", Err: err}
	}
	if _, err := insertIssueStatusEventTx(ctx, tx, projectID, issue.ID, IssueStatusTriage, status, "recorded by issue absorb", now); err != nil {
		return Issue{}, err
	}
	return loadIssueTx(ctx, tx, projectID, issue.ID)
}

func (s *Store) latestTaskAbsorbNote(ctx context.Context, projectID, taskID string) (string, error) {
	var note sql.NullString
	err := s.db.QueryRowContext(ctx, `
SELECT note FROM events
WHERE project_id = ? AND entity_kind = 'task' AND entity_id = ? AND event_type = 'status_changed'
ORDER BY created_at DESC, rowid DESC
LIMIT 1
`, projectID, taskID).Scan(&note)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read task absorb note: %w", err)
	}
	return note.String, nil
}

func latestTaskAbsorbNoteTx(ctx context.Context, tx *sql.Tx, projectID, taskID string) (string, error) {
	var note sql.NullString
	err := tx.QueryRowContext(ctx, `
SELECT note FROM events
WHERE project_id = ? AND entity_kind = 'task' AND entity_id = ? AND event_type = 'status_changed'
ORDER BY created_at DESC, rowid DESC
LIMIT 1
`, projectID, taskID).Scan(&note)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read task absorb note: %w", err)
	}
	return note.String, nil
}

func intentAbsorbAlreadyTerminal(reason string) bool {
	reason = strings.TrimSpace(reason)
	return strings.HasPrefix(reason, intentAbsorbReasonPrefix) || strings.HasPrefix(reason, "superseded by loaf issue absorb")
}

func intentAbsorbDispositionLabel(reason string) string {
	if strings.HasPrefix(strings.TrimSpace(reason), "superseded by loaf issue absorb") {
		return AbsorbDispositionSuperseded
	}
	return AbsorbDispositionAbsorbed
}
