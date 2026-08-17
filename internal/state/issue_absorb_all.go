package state

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/levifig/loaf/internal/project"
)

const (
	AbsorbActionAbsorb  = "absorb"
	AbsorbActionDismiss = "dismiss"
	AbsorbActionSkip    = "skip"
	AbsorbActionRefuse  = "refuse"

	LeftoverOpenWorkCode                = "leftover-open-work"
	LeftoverHistoryWorkCode             = "leftover-history-work"
	LeftoverHistoryFrozenCode           = "leftover-history-frozen"
	LeftoverAbsorbCheckUnreadableCode   = "leftover-absorb-check-unreadable"
	LeftoverAbsorbPreviewCommand        = "loaf issue absorb --all --dry-run"
	LeftoverAbsorbHistoryPreviewCommand = "loaf issue absorb --all --history --dry-run"
	LeftoverAbsorbApplyCommand          = "loaf issue absorb --all"
	LeftoverAbsorbHistoryApplyCommand   = "loaf issue absorb --all --history"
)

// AbsorbHistoryIndependentError is the --history gate when the issue table
// already has independently created issues. History stays frozen until 0.5.0.
type AbsorbHistoryIndependentError struct {
	Count int
}

func (e *AbsorbHistoryIndependentError) Error() string {
	return fmt.Sprintf("issue absorb --history refuses because this project already has %d issue(s) that were not minted by absorb; history stays frozen until 0.5.0 (LOAF-47)", e.Count)
}

// LeftoverAbsorbReport is the doctor inventory of leftover SQLite work.
type LeftoverAbsorbReport struct {
	OpenAbsorb        int
	OpenRefuse        int
	HistoryAbsorb     int
	HistoryRefuse     int
	HistoryFrozen     bool
	IndependentIssues int
	FrozenHistory     int
}

func (r LeftoverAbsorbReport) OpenActionable() int {
	return r.OpenAbsorb + r.OpenRefuse
}

func (r LeftoverAbsorbReport) HistoryActionable() int {
	return r.HistoryAbsorb + r.HistoryRefuse
}

// LeftoverAbsorbUnavailable reports that leftover inventory cannot run yet
// because SQLite is missing or the checkout is not registered.
func LeftoverAbsorbUnavailable(err error) bool {
	if err == nil {
		return false
	}
	var unregistered *UnregisteredProjectIdentityError
	if errors.As(err, &unregistered) {
		return true
	}
	return strings.Contains(err.Error(), "SQLite state database is not initialized")
}

// ReportLeftoverAbsorb inventories leftover SQLite work without writing.
func ReportLeftoverAbsorb(ctx context.Context, root project.Root, resolver PathResolver) (LeftoverAbsorbReport, error) {
	store, err := openProjectStoreReadExisting(ctx, root, resolver)
	if err != nil {
		return LeftoverAbsorbReport{}, err
	}
	defer store.Close()
	return store.leftoverAbsorbInventory(ctx, root)
}

// AbsorbAllOptions describes the project-wide leftover projector.
type AbsorbAllOptions struct {
	History bool
	Dismiss bool
	DryRun  bool
	// Aliases maps a leftover source id to a pre-minted issue alias (Linear).
	Aliases map[string]string
}

// AbsorbProjectionItem is one leftover row the projector classified.
type AbsorbProjectionItem struct {
	Action      string       `json:"action"`
	Reason      string       `json:"reason,omitempty"`
	IssueStatus string       `json:"issue_status,omitempty"`
	Source      AbsorbSource `json:"source"`
	Issue       *Issue       `json:"issue,omitempty"`
}

// AbsorbProjectionResult is the plan or apply envelope for loaf issue absorb --all.
type AbsorbProjectionResult struct {
	ContractVersion    int                    `json:"contract_version,omitempty"`
	DatabaseScope      string                 `json:"database_scope,omitempty"`
	DatabasePath       string                 `json:"database_path,omitempty"`
	ProjectID          string                 `json:"project_id,omitempty"`
	ProjectName        string                 `json:"project_name,omitempty"`
	ProjectCurrentPath string                 `json:"project_current_path,omitempty"`
	All                bool                   `json:"all"`
	History            bool                   `json:"history"`
	Dismiss            bool                   `json:"dismiss"`
	DryRun             bool                   `json:"dry_run"`
	Absorbed           int                    `json:"absorbed"`
	Dismissed          int                    `json:"dismissed"`
	Skipped            int                    `json:"skipped"`
	Refused            int                    `json:"refused"`
	Items              []AbsorbProjectionItem `json:"items"`
}

func (r *AbsorbProjectionResult) recount() {
	r.Absorbed = 0
	r.Dismissed = 0
	r.Skipped = 0
	r.Refused = 0
	for _, item := range r.Items {
		switch item.Action {
		case AbsorbActionAbsorb:
			r.Absorbed++
		case AbsorbActionDismiss:
			r.Dismissed++
		case AbsorbActionSkip:
			r.Skipped++
		case AbsorbActionRefuse:
			r.Refused++
		}
	}
}

// PlanAbsorbAll classifies leftover SQLite work without writing.
func PlanAbsorbAll(ctx context.Context, root project.Root, resolver PathResolver, options AbsorbAllOptions) (AbsorbProjectionResult, error) {
	store, err := openProjectStoreReadExisting(ctx, root, resolver)
	if err != nil {
		return AbsorbProjectionResult{}, err
	}
	defer store.Close()
	return store.planAbsorbAll(ctx, root, options)
}

// AbsorbAll applies the leftover projector, or returns the dry-run plan.
func AbsorbAll(ctx context.Context, root project.Root, resolver PathResolver, options AbsorbAllOptions) (AbsorbProjectionResult, error) {
	plan, err := PlanAbsorbAll(ctx, root, resolver, options)
	if err != nil {
		return AbsorbProjectionResult{}, err
	}
	if options.DryRun {
		plan.DryRun = true
		return plan, nil
	}
	for i, item := range plan.Items {
		if item.Action != AbsorbActionAbsorb && item.Action != AbsorbActionDismiss {
			continue
		}
		result, err := Absorb(ctx, root, resolver, AbsorbOptions{
			Ref:         firstNonEmpty(item.Source.Alias, item.Source.ID),
			Dismiss:     item.Action == AbsorbActionDismiss,
			History:     options.History,
			IssueStatus: item.IssueStatus,
			Alias:       options.Aliases[item.Source.ID],
		})
		if err != nil {
			return plan, err
		}
		plan.Items[i].Source = result.Source
		plan.Items[i].Issue = result.Issue
	}
	return plan, nil
}

func (s *Store) planAbsorbAll(ctx context.Context, root project.Root, options AbsorbAllOptions) (AbsorbProjectionResult, error) {
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return AbsorbProjectionResult{}, err
	}
	identity, err := s.projectIdentity(ctx, projectID)
	if err != nil {
		return AbsorbProjectionResult{}, err
	}
	if options.History {
		independent, err := s.countIndependentAbsorbIssues(ctx, projectID)
		if err != nil {
			return AbsorbProjectionResult{}, err
		}
		if independent > 0 {
			return AbsorbProjectionResult{}, &AbsorbHistoryIndependentError{Count: independent}
		}
	}
	represented, err := s.absorbProvenanceIndex(ctx, projectID)
	if err != nil {
		return AbsorbProjectionResult{}, err
	}
	items := []AbsorbProjectionItem{}
	taskItems, err := s.planAbsorbTasks(ctx, root, projectID, options, represented)
	if err != nil {
		return AbsorbProjectionResult{}, err
	}
	items = append(items, taskItems...)
	intentItems, err := s.planAbsorbIntents(ctx, projectID, options, represented)
	if err != nil {
		return AbsorbProjectionResult{}, err
	}
	items = append(items, intentItems...)
	result := AbsorbProjectionResult{
		ContractVersion:    StateJSONContractVersion,
		DatabaseScope:      identity.DatabaseScope,
		DatabasePath:       identity.DatabasePath,
		ProjectID:          identity.ID,
		ProjectName:        identity.FriendlyName,
		ProjectCurrentPath: identity.CurrentPath,
		All:                true,
		History:            options.History,
		Dismiss:            options.Dismiss,
		Items:              items,
	}
	result.recount()
	return result, nil
}

func (s *Store) planAbsorbTasks(ctx context.Context, root project.Root, projectID string, options AbsorbAllOptions, represented map[string]string) ([]AbsorbProjectionItem, error) {
	ids, err := s.listAbsorbTaskIDs(ctx, projectID, options.History)
	if err != nil {
		return nil, err
	}
	items := []AbsorbProjectionItem{}
	for _, id := range ids {
		source, err := s.loadAbsorbTaskSource(ctx, root, projectID, TraceEntity{Kind: "task", ID: id})
		if err != nil {
			return nil, err
		}
		inDefault := leftoverOpenTaskStatus(source.Status)
		inHistory := leftoverHistoryTaskStatus(source.Status)
		if !inDefault && !(options.History && inHistory) {
			continue
		}
		if absorbSourceIsChangeLocal(source) {
			items = append(items, AbsorbProjectionItem{
				Action: AbsorbActionRefuse,
				Reason: "change-local task files under docs/changes/**/tasks/",
				Source: source,
			})
			continue
		}
		note, err := s.latestTaskAbsorbNote(ctx, projectID, source.ID)
		if err != nil {
			return nil, err
		}
		if intentAbsorbAlreadyTerminal(note) || represented[absorbProvenanceKey("task", source.ID)] != "" {
			items = append(items, AbsorbProjectionItem{
				Action: AbsorbActionSkip,
				Reason: "already represented",
				Source: source,
			})
			continue
		}
		items = append(items, classifyAbsorbCandidate(source, options, inHistory))
	}
	return items, nil
}

func (s *Store) planAbsorbIntents(ctx context.Context, projectID string, options AbsorbAllOptions, represented map[string]string) ([]AbsorbProjectionItem, error) {
	ids, err := s.listAbsorbIntentIDs(ctx, projectID, options.History)
	if err != nil {
		return nil, err
	}
	items := []AbsorbProjectionItem{}
	for _, id := range ids {
		source, err := s.loadAbsorbIntentSource(ctx, projectID, id)
		if err != nil {
			return nil, err
		}
		inDefault := leftoverNonTerminalIntent(source.Disposition)
		inHistory := !inDefault
		if !inDefault && !(options.History && inHistory) {
			continue
		}
		if intentAbsorbAlreadyTerminal(source.DispositionReason) || represented[absorbProvenanceKey("intent", source.ID)] != "" {
			items = append(items, AbsorbProjectionItem{
				Action: AbsorbActionSkip,
				Reason: "already represented",
				Source: source,
			})
			continue
		}
		items = append(items, classifyAbsorbCandidate(source, options, inHistory))
	}
	return items, nil
}

func classifyAbsorbCandidate(source AbsorbSource, options AbsorbAllOptions, history bool) AbsorbProjectionItem {
	if options.Dismiss {
		return AbsorbProjectionItem{Action: AbsorbActionDismiss, Source: source}
	}
	status := IssueStatusTriage
	if source.Kind == "intent" && !history {
		status = IssueStatusBacklog
	}
	if history {
		status = IssueStatusDone
		if source.Kind == "task" && LifecycleStatusMatches(LifecycleEntityTask, source.Status, LifecycleStatusArchived) {
			status = IssueStatusCancelled
		}
	}
	return AbsorbProjectionItem{
		Action:      AbsorbActionAbsorb,
		IssueStatus: status,
		Source:      source,
	}
}

func leftoverOpenTaskStatuses() []string {
	return []string{
		LifecycleStatusTodo,
		LifecycleStatusInProgress,
		LifecycleStatusBlocked,
		LifecycleStatusReview,
	}
}

func (s *Store) listAbsorbTaskIDs(ctx context.Context, projectID string, history bool) ([]string, error) {
	query := `
SELECT tasks.id
FROM tasks
WHERE tasks.project_id = ?`
	args := []any{projectID}
	if !history {
		open := leftoverOpenTaskStatuses()
		query += `
  AND tasks.status IN (?, ?, ?, ?)`
		args = append(args, open[0], open[1], open[2], open[3])
	}
	query += `
ORDER BY COALESCE((
  SELECT alias FROM aliases
  WHERE aliases.project_id = tasks.project_id
    AND aliases.entity_kind = 'task'
    AND aliases.entity_id = tasks.id
    AND aliases.namespace = 'task'
  LIMIT 1
), tasks.id), tasks.id
`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list leftover tasks: %w", err)
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan leftover task: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate leftover tasks: %w", err)
	}
	return ids, nil
}

func latestIntentDispositionSQL(alias string) string {
	return `(SELECT disposition FROM intent_dispositions WHERE intent_id = ` + alias + `.id ORDER BY seq DESC LIMIT 1)`
}

func latestIntentDispositionReasonSQL(alias string) string {
	return `(SELECT reason FROM intent_dispositions WHERE intent_id = ` + alias + `.id ORDER BY seq DESC LIMIT 1)`
}

func (s *Store) listAbsorbIntentIDs(ctx context.Context, projectID string, history bool) ([]string, error) {
	query := `SELECT id FROM intents WHERE project_id = ?`
	args := []any{projectID}
	if !history {
		query += ` AND TRIM(COALESCE(` + latestIntentDispositionSQL("intents") + `, '')) <> 'resolved'`
	}
	query += ` ORDER BY created_at, id`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list leftover intents: %w", err)
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan leftover intent: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate leftover intents: %w", err)
	}
	return ids, nil
}

func (s *Store) countIndependentAbsorbIssues(ctx context.Context, projectID string) (int, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT COALESCE(body, '') FROM issues WHERE project_id = ?`, projectID)
	if err != nil {
		return 0, fmt.Errorf("list issues for absorb history gate: %w", err)
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var body string
		if err := rows.Scan(&body); err != nil {
			return 0, fmt.Errorf("scan issue body for absorb history gate: %w", err)
		}
		if !strings.HasPrefix(strings.TrimSpace(body), absorbProvenancePrefix) {
			count++
		}
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterate issues for absorb history gate: %w", err)
	}
	return count, nil
}

func (s *Store) absorbProvenanceIndex(ctx context.Context, projectID string) (map[string]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, COALESCE(body, '') FROM issues WHERE project_id = ?`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list absorb provenance: %w", err)
	}
	defer rows.Close()
	index := map[string]string{}
	for rows.Next() {
		var issueID, body string
		if err := rows.Scan(&issueID, &body); err != nil {
			return nil, fmt.Errorf("scan absorb provenance: %w", err)
		}
		for _, key := range absorbProvenanceKeys(body) {
			if _, exists := index[key]; !exists {
				index[key] = issueID
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate absorb provenance: %w", err)
	}
	return index, nil
}

func absorbProvenanceKey(kind, id string) string {
	return kind + ":" + id
}

func absorbProvenanceKeys(body string) []string {
	keys := []string{}
	for _, kind := range []string{"task", "intent"} {
		prefix := "(" + kind + ":"
		rest := body
		for {
			idx := strings.Index(rest, prefix)
			if idx < 0 {
				break
			}
			rest = rest[idx+len(prefix):]
			end := strings.IndexByte(rest, ')')
			if end < 0 {
				break
			}
			id := strings.TrimSpace(rest[:end])
			if id != "" {
				keys = append(keys, absorbProvenanceKey(kind, id))
			}
			rest = rest[end+1:]
		}
	}
	return keys
}

func leftoverItemIsOpen(item AbsorbProjectionItem) bool {
	if item.Source.Kind == "intent" {
		return leftoverNonTerminalIntent(item.Source.Disposition)
	}
	return leftoverOpenTaskStatus(item.Source.Status)
}

func leftoverItemIsHistory(item AbsorbProjectionItem) bool {
	if item.Source.Kind == "intent" {
		return !leftoverNonTerminalIntent(item.Source.Disposition)
	}
	return leftoverHistoryTaskStatus(item.Source.Status)
}

func tallyAbsorbActions(absorb, refuse *int, item AbsorbProjectionItem) {
	switch item.Action {
	case AbsorbActionAbsorb:
		*absorb++
	case AbsorbActionRefuse:
		*refuse++
	}
}

func (s *Store) leftoverAbsorbInventory(ctx context.Context, root project.Root) (LeftoverAbsorbReport, error) {
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return LeftoverAbsorbReport{}, err
	}
	represented, err := s.absorbProvenanceIndex(ctx, projectID)
	if err != nil {
		return LeftoverAbsorbReport{}, err
	}
	independent, err := s.countIndependentAbsorbIssues(ctx, projectID)
	if err != nil {
		return LeftoverAbsorbReport{}, err
	}
	report := LeftoverAbsorbReport{IndependentIssues: independent}
	if independent > 0 {
		report.HistoryFrozen = true
		openTasks, err := s.planAbsorbTasks(ctx, root, projectID, AbsorbAllOptions{}, represented)
		if err != nil {
			return LeftoverAbsorbReport{}, err
		}
		openIntents, err := s.planAbsorbIntents(ctx, projectID, AbsorbAllOptions{}, represented)
		if err != nil {
			return LeftoverAbsorbReport{}, err
		}
		for _, item := range append(openTasks, openIntents...) {
			tallyAbsorbActions(&report.OpenAbsorb, &report.OpenRefuse, item)
		}
		frozen, err := s.countHistoryLeftoverRows(ctx, projectID, represented)
		if err != nil {
			return LeftoverAbsorbReport{}, err
		}
		report.FrozenHistory = frozen
		return report, nil
	}
	taskItems, err := s.planAbsorbTasks(ctx, root, projectID, AbsorbAllOptions{History: true}, represented)
	if err != nil {
		return LeftoverAbsorbReport{}, err
	}
	intentItems, err := s.planAbsorbIntents(ctx, projectID, AbsorbAllOptions{History: true}, represented)
	if err != nil {
		return LeftoverAbsorbReport{}, err
	}
	for _, item := range append(taskItems, intentItems...) {
		switch {
		case leftoverItemIsOpen(item):
			tallyAbsorbActions(&report.OpenAbsorb, &report.OpenRefuse, item)
		case leftoverItemIsHistory(item):
			tallyAbsorbActions(&report.HistoryAbsorb, &report.HistoryRefuse, item)
		}
	}
	return report, nil
}

func (s *Store) countHistoryLeftoverRows(ctx context.Context, projectID string, represented map[string]string) (int, error) {
	// One query per table: MaxOpenConns is 1, so a nested note lookup while
	// these rows are open would deadlock.
	taskRows, err := s.db.QueryContext(ctx, `
SELECT tasks.id, tasks.status, COALESCE((
  SELECT note FROM events
  WHERE events.project_id = tasks.project_id
    AND events.entity_kind = 'task'
    AND events.entity_id = tasks.id
    AND events.event_type = 'status_changed'
  ORDER BY events.created_at DESC, events.rowid DESC
  LIMIT 1
), '')
FROM tasks
WHERE tasks.project_id = ?
`, projectID)
	if err != nil {
		return 0, fmt.Errorf("list leftover history tasks: %w", err)
	}
	count := 0
	for taskRows.Next() {
		var id, status, note string
		if err := taskRows.Scan(&id, &status, &note); err != nil {
			taskRows.Close()
			return 0, fmt.Errorf("scan leftover history task: %w", err)
		}
		if !leftoverHistoryTaskStatus(status) {
			continue
		}
		if represented[absorbProvenanceKey("task", id)] != "" {
			continue
		}
		if intentAbsorbAlreadyTerminal(note) {
			continue
		}
		count++
	}
	err = taskRows.Err()
	if closeErr := taskRows.Close(); closeErr != nil && err == nil {
		err = closeErr
	}
	if err != nil {
		return 0, fmt.Errorf("iterate leftover history tasks: %w", err)
	}

	intentRows, err := s.db.QueryContext(ctx, `SELECT id, COALESCE(`+latestIntentDispositionSQL("intents")+`, ''), COALESCE(`+latestIntentDispositionReasonSQL("intents")+`, '') FROM intents WHERE project_id = ?`, projectID)
	if err != nil {
		return 0, fmt.Errorf("list leftover history intents: %w", err)
	}
	for intentRows.Next() {
		var id, disposition, reason string
		if err := intentRows.Scan(&id, &disposition, &reason); err != nil {
			intentRows.Close()
			return 0, fmt.Errorf("scan leftover history intent: %w", err)
		}
		if leftoverNonTerminalIntent(disposition) {
			continue
		}
		if represented[absorbProvenanceKey("intent", id)] != "" {
			continue
		}
		if intentAbsorbAlreadyTerminal(reason) {
			continue
		}
		count++
	}
	err = intentRows.Err()
	if closeErr := intentRows.Close(); closeErr != nil && err == nil {
		err = closeErr
	}
	if err != nil {
		return 0, fmt.Errorf("iterate leftover history intents: %w", err)
	}
	return count, nil
}
