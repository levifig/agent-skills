package state

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/levifig/loaf/internal/project"
)

const (
	JournalContextContractVersion = 2

	JournalContextLayerSynthesis   = "project_synthesis"
	JournalContextLayerCheckpoint  = "latest_checkpoint"
	JournalContextLayerLineage     = "active_lineage"
	JournalContextLayerBlockers    = "unresolved_blockers"
	JournalContextLayerDeferrals   = "deferred_intent"
	JournalContextLayerCheckpoints = "exploration_checkpoints"
	JournalContextLayerBranch      = "branch_recency"
	JournalContextLayerTasks       = "transitional_tasks"

	defaultJournalContextLineageLimit     = 10
	defaultJournalContextBlockerLimit     = 10
	defaultJournalContextDeferralLimit    = 10
	defaultJournalContextCheckpointsLimit = 5
	defaultJournalContextBranchLimit      = 10
	defaultJournalContextTaskLimit        = 10
	journalContextSynthesisLimit          = 1
)

// JournalContextOptions describes a continuity digest request. Limits are
// independent; zero selects the per-layer default. CursorLayer is optional but,
// when supplied, prevents a cursor generated for one layer being used as
// another layer's expansion token.
type JournalContextOptions struct {
	Branch           string
	LineageKeys      []string
	LineageLimit     int
	BlockerLimit     int
	DeferralLimit    int
	CheckpointsLimit int
	BranchLimit      int
	TaskLimit        int
	Cursor           string
	CursorLayer      string
}

// JournalContext is the contract-v2 active-truth read model. Every layer is
// computed from one read transaction and is never persisted.
type JournalContext struct {
	ContractVersion    int    `json:"contract_version"`
	DatabaseScope      string `json:"database_scope"`
	DatabasePath       string `json:"database_path"`
	ProjectID          string `json:"project_id"`
	ProjectName        string `json:"project_name"`
	ProjectCurrentPath string `json:"project_current_path"`
	Branch             string `json:"branch,omitempty"`
	JournalWatermark   int64  `json:"journal_watermark"`

	ProjectSynthesis       JournalContextJournalLayer     `json:"project_synthesis"`
	LatestCheckpoint       JournalContextCheckpointLayer  `json:"latest_checkpoint"`
	ActiveLineage          JournalContextJournalLayer     `json:"active_lineage"`
	UnresolvedBlockers     JournalContextBlockerLayer     `json:"unresolved_blockers"`
	DeferredIntent         JournalContextDeferralLayer    `json:"deferred_intent"`
	ExplorationCheckpoints JournalContextCheckpointsLayer `json:"exploration_checkpoints"`
	BranchRecency          JournalContextJournalLayer     `json:"branch_recency"`
	TransitionalTasks      JournalContextTaskLayer        `json:"transitional_tasks"`
	Diagnostics            []JournalContextDiagnostic     `json:"diagnostics"`

	// Deprecated compatibility fields. The v2 CLI should consume the named
	// layers above; these keep existing callers source-compatible during U6.
	LatestWrap    *JournalEntryRecord  `json:"-"`
	BranchEntries []JournalEntryRecord `json:"-"`
	OpenTasks     []JournalContextTask `json:"-"`
}

type JournalContextJournalLayer struct {
	Available      bool                 `json:"source_available"`
	AvailableCount int                  `json:"available_count"`
	ShownCount     int                  `json:"shown_count"`
	Truncated      bool                 `json:"truncated"`
	Cursor         string               `json:"cursor,omitempty"`
	ExpandCommand  string               `json:"expand_command"`
	Items          []JournalEntryRecord `json:"items"`
}

type JournalContextCheckpointLayer struct {
	Available      bool                           `json:"source_available"`
	AvailableCount int                            `json:"available_count"`
	ShownCount     int                            `json:"shown_count"`
	Truncated      bool                           `json:"truncated"`
	Cursor         string                         `json:"cursor,omitempty"`
	ExpandCommand  string                         `json:"expand_command"`
	Items          []JournalContextCheckpointItem `json:"items"`
}

type JournalContextCheckpointItem struct {
	Entry            JournalEntryRecord `json:"entry"`
	Scope            string             `json:"scope"`
	ProjectSynthesis bool               `json:"project_synthesis"`
	Label            string             `json:"label"`
}

type JournalContextBlockerLayer struct {
	Available      bool                        `json:"source_available"`
	AvailableCount int                         `json:"available_count"`
	ShownCount     int                         `json:"shown_count"`
	Truncated      bool                        `json:"truncated"`
	Cursor         string                      `json:"cursor,omitempty"`
	ExpandCommand  string                      `json:"expand_command"`
	Items          []JournalContextBlockerItem `json:"items"`
}

type JournalContextBlockerItem struct {
	Key               string             `json:"key"`
	Block             JournalEntryRecord `json:"block"`
	PreviousUnblockID string             `json:"previous_unblock_id,omitempty"`
}

type JournalContextDeferralLayer struct {
	Available      bool                         `json:"source_available"`
	AvailableCount int                          `json:"available_count"`
	ShownCount     int                          `json:"shown_count"`
	Truncated      bool                         `json:"truncated"`
	Cursor         string                       `json:"cursor,omitempty"`
	ExpandCommand  string                       `json:"expand_command"`
	Items          []JournalContextDeferralItem `json:"items"`
}

type JournalContextDeferralItem struct {
	OperationKey string                        `json:"operation_key"`
	Decision     JournalEntryRecord            `json:"decision"`
	Spark        JournalContextDeferredSpark   `json:"spark"`
	Origin       *JournalContextOriginSummary  `json:"origin,omitempty"`
	Intent       *JournalContextDeferredIntent `json:"intent,omitempty"`
}

// JournalContextDeferredIntent is the canonical Intent behind a deferred item.
// When present it is active truth: the item surfaces regardless of journal
// recency, and the legacy Decision/Spark fields carry mapped v1 projections
// only when those historical rows exist.
type JournalContextDeferredIntent struct {
	ID             string `json:"id"`
	Alias          string `json:"alias,omitempty"`
	Title          string `json:"title"`
	Disposition    string `json:"disposition"`
	Body           string `json:"body"`
	Why            string `json:"why"`
	Boundary       string `json:"boundary"`
	RevisitTrigger string `json:"revisit_trigger"`
}

type JournalContextDeferredSpark struct {
	ID        string `json:"id"`
	Alias     string `json:"alias,omitempty"`
	Text      string `json:"text"`
	Scope     string `json:"scope,omitempty"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type JournalContextOriginSummary struct {
	CaptureMechanism string `json:"capture_mechanism"`
	Branch           string `json:"branch,omitempty"`
	Worktree         string `json:"worktree,omitempty"`
	Head             string `json:"head,omitempty"`
	ChangePath       string `json:"change_path,omitempty"`
	Dirty            *bool  `json:"dirty"`
	Reconstructable  *bool  `json:"reconstructable"`
	Supported        bool   `json:"supported"`
}

// JournalContextCheckpointsLayer is the separately bounded, discoverable layer
// of recent portable Exploration checkpoints. It never exposes a
// current-Exploration pointer: every item carries its own read command, and an
// Exploration with no checkpoint (a handle-only inquiry) is absent because a
// handle never implies portable context.
type JournalContextCheckpointsLayer struct {
	Available      bool                                      `json:"source_available"`
	AvailableCount int                                       `json:"available_count"`
	ShownCount     int                                       `json:"shown_count"`
	Truncated      bool                                      `json:"truncated"`
	Cursor         string                                    `json:"cursor,omitempty"`
	ExpandCommand  string                                    `json:"expand_command"`
	Items          []JournalContextExplorationCheckpointItem `json:"items"`
}

// JournalContextExplorationCheckpointItem is the latest portable checkpoint of
// one Exploration.
type JournalContextExplorationCheckpointItem struct {
	ExplorationID  string `json:"exploration_id"`
	Alias          string `json:"alias,omitempty"`
	Title          string `json:"title"`
	Seq            int    `json:"seq"`
	NextAction     string `json:"next_action"`
	CreatedAt      string `json:"created_at"`
	ContextCommand string `json:"context_command"`
}

type JournalContextTaskLayer struct {
	Available      bool                 `json:"source_available"`
	AvailableCount int                  `json:"available_count"`
	ShownCount     int                  `json:"shown_count"`
	Truncated      bool                 `json:"truncated"`
	Cursor         string               `json:"cursor,omitempty"`
	ExpandCommand  string               `json:"expand_command"`
	Items          []JournalContextTask `json:"items"`
}

// JournalContextTask is an open transitional task surfaced in continuity.
type JournalContextTask struct {
	Ref      string `json:"ref"`
	Title    string `json:"title"`
	Status   string `json:"status"`
	Priority string `json:"priority,omitempty"`
	Spec     string `json:"spec,omitempty"`
}

type JournalContextDiagnostic struct {
	Code    string `json:"code"`
	Key     string `json:"key,omitempty"`
	EntryID string `json:"entry_id,omitempty"`
	Message string `json:"message"`
}

type journalContextJournalCandidate struct {
	Entry JournalEntryRecord
	RowID int64
}

type journalContextBlockerCandidate struct {
	Item      JournalContextBlockerItem
	CreatedAt string
	RowID     int64
}

type journalContextDeferralCandidate struct {
	Item      JournalContextDeferralItem
	CreatedAt string
	RowID     int64
	// Canonical marks a candidate derived from a canonical Intent. Canonical
	// candidates are active truth: they precede legacy-only items and never
	// depend on the journal watermark for inclusion.
	Canonical bool
}

type journalContextCheckpointCandidate struct {
	Item JournalContextExplorationCheckpointItem
}

// JournalContextForRoot computes the continuity digest from registered state.
func JournalContextForRoot(ctx context.Context, root project.Root, resolver PathResolver, options JournalContextOptions) (JournalContext, error) {
	store, err := openProjectStoreReadExisting(ctx, root, resolver)
	if err != nil {
		return JournalContext{}, err
	}
	defer store.Close()
	return store.JournalContext(ctx, root, options)
}

// JournalContext computes all contract-v2 layers in one SQLite read snapshot.
func (s *Store) JournalContext(ctx context.Context, root project.Root, options JournalContextOptions) (JournalContext, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return JournalContext{}, fmt.Errorf("begin journal context snapshot: %w", err)
	}
	defer tx.Rollback()

	// This is deliberately the first query: it establishes the SQLite snapshot
	// and the immutable upper bound used by every journal projection.
	var currentWatermark int64
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(rowid), 0) FROM journal_entries`).Scan(&currentWatermark); err != nil {
		return JournalContext{}, fmt.Errorf("capture journal context watermark: %w", err)
	}
	identity, err := journalContextProjectIdentity(ctx, tx, root, s.path)
	if errors.Is(err, sql.ErrNoRows) {
		return JournalContext{}, s.unregisteredProjectIdentityError(ctx, root.Path())
	}
	if err != nil {
		return JournalContext{}, err
	}

	branch := strings.TrimSpace(options.Branch)
	lineageKeys := canonicalJournalContextLineageKeys(options.LineageKeys)
	lineageFingerprint := journalContextFingerprint(lineageKeys)
	cursor, err := decodeAndValidateJournalContextCursor(options.Cursor, options.CursorLayer, identity.ID, branch)
	if err != nil {
		return JournalContext{}, err
	}
	if cursor != nil {
		expectedLimit := journalContextLimitForLayer(options, cursor.Layer)
		if cursor.Limit != expectedLimit {
			return JournalContext{}, newJournalContextCursorInvalid("cursor belongs to a different layer limit")
		}
		if cursor.Layer == JournalContextLayerLineage && cursor.Fingerprint != lineageFingerprint {
			return JournalContext{}, newJournalContextCursorStale(branch, cursor.Layer, expectedLimit)
		}
	}
	watermark := currentWatermark
	if cursor != nil {
		if cursor.Watermark > currentWatermark {
			return JournalContext{}, newJournalContextCursorInvalid("cursor watermark is newer than the database snapshot")
		}
		watermark = cursor.Watermark
	}

	synthesisCandidates, err := queryJournalContextEntries(ctx, tx, identity.ID, watermark, "entry_type = 'wrap' AND scope = 'project'", nil)
	if err != nil {
		return JournalContext{}, err
	}
	if len(synthesisCandidates) > journalContextSynthesisLimit {
		synthesisCandidates = synthesisCandidates[:journalContextSynthesisLimit]
	}
	checkpointCandidates := []journalContextJournalCandidate{}
	if len(synthesisCandidates) == 0 {
		checkpointCandidates, err = queryJournalContextEntries(ctx, tx, identity.ID, watermark, "entry_type = 'wrap' AND COALESCE(scope, '') <> 'project'", nil)
		if err != nil {
			return JournalContext{}, err
		}
		if len(checkpointCandidates) > journalContextSynthesisLimit {
			checkpointCandidates = checkpointCandidates[:journalContextSynthesisLimit]
		}
	}

	lineageCandidates, err := queryLineageCandidates(ctx, tx, identity.ID, watermark, lineageKeys)
	if err != nil {
		return JournalContext{}, err
	}
	blockerCandidates, diagnostics, err := queryBlockerCandidates(ctx, tx, identity.ID, watermark)
	if err != nil {
		return JournalContext{}, err
	}
	deferralCandidates, err := queryDeferralCandidates(ctx, tx, identity.ID, watermark)
	if err != nil {
		return JournalContext{}, err
	}
	// Deferrals have a journal endpoint but are mutable through their spark and
	// deferral rows, and canonical Intents are active truth independent of the
	// watermark. Fingerprint the complete current candidate set, including
	// deferrals created after a continuation cursor's journal watermark.
	currentDeferralCandidates := deferralCandidates
	if currentWatermark != watermark {
		currentDeferralCandidates, err = queryDeferralCandidates(ctx, tx, identity.ID, currentWatermark)
		if err != nil {
			return JournalContext{}, err
		}
	}
	checkpointItemCandidates, err := queryExplorationCheckpointCandidates(ctx, tx, identity.ID)
	if err != nil {
		return JournalContext{}, err
	}

	activeIDs := make(map[string]struct{}, len(synthesisCandidates)+len(checkpointCandidates)+len(lineageCandidates)+len(blockerCandidates)+len(deferralCandidates))
	for _, candidates := range [][]journalContextJournalCandidate{synthesisCandidates, checkpointCandidates, lineageCandidates} {
		for _, candidate := range candidates {
			activeIDs[candidate.Entry.ID] = struct{}{}
		}
	}
	for _, candidate := range blockerCandidates {
		activeIDs[candidate.Item.Block.ID] = struct{}{}
	}
	for _, candidate := range deferralCandidates {
		if candidate.Item.Decision.ID != "" {
			activeIDs[candidate.Item.Decision.ID] = struct{}{}
		}
	}

	branchCandidates := []journalContextJournalCandidate{}
	if branch != "" {
		branchCandidates, err = queryJournalContextEntries(ctx, tx, identity.ID, watermark, "observed_branch = ?", []any{branch})
		if err != nil {
			return JournalContext{}, err
		}
		filtered := branchCandidates[:0]
		for _, candidate := range branchCandidates {
			if _, duplicate := activeIDs[candidate.Entry.ID]; !duplicate {
				filtered = append(filtered, candidate)
			}
		}
		branchCandidates = filtered
	}
	taskCandidates, err := queryJournalContextTasks(ctx, tx, identity.ID)
	if err != nil {
		return JournalContext{}, err
	}

	deferralFingerprint := journalContextFingerprintDeferrals(currentDeferralCandidates)
	taskFingerprint := journalContextFingerprintTasks(taskCandidates)
	if cursor != nil {
		cursorLimit := journalContextLimitForLayer(options, cursor.Layer)
		switch cursor.Layer {
		case JournalContextLayerDeferrals:
			if cursor.Fingerprint != deferralFingerprint {
				return JournalContext{}, newJournalContextCursorStale(branch, cursor.Layer, cursorLimit)
			}
		case JournalContextLayerTasks:
			if cursor.Fingerprint != taskFingerprint {
				return JournalContext{}, newJournalContextCursorStale(branch, cursor.Layer, cursorLimit)
			}
		}
	}

	synthesisLayer := journalContextJournalPage(synthesisCandidates, journalContextSynthesisLimit, nil, JournalContextLayerSynthesis, identity.ID, branch, watermark, "")
	checkpointLayer := journalContextCheckpointPage(checkpointCandidates, branch)
	lineageLayer, err := journalContextJournalPageChecked(lineageCandidates, contextLimit(options.LineageLimit, defaultJournalContextLineageLimit), cursorForLayer(cursor, JournalContextLayerLineage), JournalContextLayerLineage, identity.ID, branch, watermark, lineageFingerprint)
	if err != nil {
		return JournalContext{}, err
	}
	blockerLayer, err := journalContextBlockerPage(blockerCandidates, contextLimit(options.BlockerLimit, defaultJournalContextBlockerLimit), cursorForLayer(cursor, JournalContextLayerBlockers), identity.ID, branch, watermark)
	if err != nil {
		return JournalContext{}, err
	}
	deferralLayer, err := journalContextDeferralPage(deferralCandidates, contextLimit(options.DeferralLimit, defaultJournalContextDeferralLimit), cursorForLayer(cursor, JournalContextLayerDeferrals), identity.ID, branch, watermark, deferralFingerprint)
	if err != nil {
		return JournalContext{}, err
	}
	checkpointsLayer := journalContextCheckpointsPage(checkpointItemCandidates, contextLimit(options.CheckpointsLimit, defaultJournalContextCheckpointsLimit), branch)
	branchLayer, err := journalContextJournalPageChecked(branchCandidates, contextLimit(options.BranchLimit, defaultJournalContextBranchLimit), cursorForLayer(cursor, JournalContextLayerBranch), JournalContextLayerBranch, identity.ID, branch, watermark, "")
	if err != nil {
		return JournalContext{}, err
	}
	if branch == "" {
		branchLayer.Available = false
	}
	taskLayer, err := journalContextTaskPage(taskCandidates, contextLimit(options.TaskLimit, defaultJournalContextTaskLimit), cursorForLayer(cursor, JournalContextLayerTasks), identity.ID, branch, watermark, taskFingerprint)
	if err != nil {
		return JournalContext{}, err
	}

	result := JournalContext{
		ContractVersion:        JournalContextContractVersion,
		DatabaseScope:          identity.DatabaseScope,
		DatabasePath:           identity.DatabasePath,
		ProjectID:              identity.ID,
		ProjectName:            identity.FriendlyName,
		ProjectCurrentPath:     identity.CurrentPath,
		Branch:                 branch,
		JournalWatermark:       watermark,
		ProjectSynthesis:       synthesisLayer,
		LatestCheckpoint:       checkpointLayer,
		ActiveLineage:          lineageLayer,
		UnresolvedBlockers:     blockerLayer,
		DeferredIntent:         deferralLayer,
		ExplorationCheckpoints: checkpointsLayer,
		BranchRecency:          branchLayer,
		TransitionalTasks:      taskLayer,
		Diagnostics:            diagnostics,
		BranchEntries:          append([]JournalEntryRecord(nil), branchLayer.Items...),
		OpenTasks:              append([]JournalContextTask(nil), taskLayer.Items...),
	}
	if len(synthesisLayer.Items) > 0 {
		entry := synthesisLayer.Items[0]
		result.LatestWrap = &entry
	}
	if err := tx.Commit(); err != nil {
		return JournalContext{}, fmt.Errorf("finish journal context snapshot: %w", err)
	}
	return result, nil
}

func journalContextProjectIdentity(ctx context.Context, tx *sql.Tx, root project.Root, databasePath string) (ProjectIdentity, error) {
	identity := ProjectIdentity{ContractVersion: StateJSONContractVersion, DatabaseScope: "global", DatabasePath: databasePath}
	var friendlyName, currentPath, lastSeenAt sql.NullString
	err := tx.QueryRowContext(ctx, `
SELECT p.id, p.friendly_name, COALESCE(cp.path, p.current_path), p.last_seen_at
FROM project_paths requested
JOIN projects p ON p.id = requested.project_id
LEFT JOIN project_paths cp ON cp.project_id = p.id AND cp.is_current = 1
WHERE requested.path = ?
`, root.Path()).Scan(&identity.ID, &friendlyName, &currentPath, &lastSeenAt)
	if err != nil {
		return ProjectIdentity{}, err
	}
	identity.FriendlyName = friendlyName.String
	identity.CurrentPath = currentPath.String
	identity.LastSeenAt = lastSeenAt.String
	return identity, nil
}

func queryJournalContextEntries(ctx context.Context, tx *sql.Tx, projectID string, watermark int64, predicate string, predicateArgs []any) ([]journalContextJournalCandidate, error) {
	query := `
SELECT id, entry_type, COALESCE(scope, ''), message, COALESCE(observed_branch, ''),
  COALESCE(observed_worktree, ''), COALESCE(harness_session_id, ''), created_at, rowid
FROM journal_entries
WHERE project_id = ? AND rowid <= ? AND (` + predicate + `)
ORDER BY created_at DESC, rowid DESC`
	args := []any{projectID, watermark}
	args = append(args, predicateArgs...)
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query journal context entries: %w", err)
	}
	defer rows.Close()
	candidates := []journalContextJournalCandidate{}
	for rows.Next() {
		var candidate journalContextJournalCandidate
		if err := rows.Scan(&candidate.Entry.ID, &candidate.Entry.EntryType, &candidate.Entry.Scope, &candidate.Entry.Message, &candidate.Entry.ObservedBranch, &candidate.Entry.ObservedWorktree, &candidate.Entry.HarnessSessionID, &candidate.Entry.CreatedAt, &candidate.RowID); err != nil {
			return nil, fmt.Errorf("scan journal context entry: %w", err)
		}
		candidates = append(candidates, candidate)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate journal context entries: %w", err)
	}
	return candidates, nil
}

func queryLineageCandidates(ctx context.Context, tx *sql.Tx, projectID string, watermark int64, scopes []string) ([]journalContextJournalCandidate, error) {
	if len(scopes) == 0 {
		return []journalContextJournalCandidate{}, nil
	}
	all, err := queryJournalContextEntries(ctx, tx, projectID, watermark, "entry_type = 'decision' AND scope LIKE 'lineage/%'", nil)
	if err != nil {
		return nil, err
	}
	wanted := make(map[string]struct{}, len(scopes))
	for _, scope := range scopes {
		wanted[scope] = struct{}{}
	}
	seen := map[string]struct{}{}
	latest := []journalContextJournalCandidate{}
	for _, candidate := range all {
		if _, ok := wanted[candidate.Entry.Scope]; !ok {
			continue
		}
		if _, ok := seen[candidate.Entry.Scope]; ok {
			continue
		}
		seen[candidate.Entry.Scope] = struct{}{}
		latest = append(latest, candidate)
	}
	return latest, nil
}

func queryBlockerCandidates(ctx context.Context, tx *sql.Tx, projectID string, watermark int64) ([]journalContextBlockerCandidate, []JournalContextDiagnostic, error) {
	all, err := queryJournalContextEntries(ctx, tx, projectID, watermark, "entry_type IN ('block', 'unblock') AND COALESCE(scope, '') <> ''", nil)
	if err != nil {
		return nil, nil, err
	}
	byKey := map[string][]journalContextJournalCandidate{}
	for _, candidate := range all {
		byKey[candidate.Entry.Scope] = append(byKey[candidate.Entry.Scope], candidate)
	}
	candidates := []journalContextBlockerCandidate{}
	diagnostics := []JournalContextDiagnostic{}
	for key, entries := range byKey {
		outstanding := false
		for i := len(entries) - 1; i >= 0; i-- {
			entry := entries[i]
			switch entry.Entry.EntryType {
			case "block":
				outstanding = true
			case "unblock":
				if !outstanding {
					diagnostics = append(diagnostics, JournalContextDiagnostic{Code: "journal-context-unmatched-unblock", Key: key, EntryID: entry.Entry.ID, Message: "unblock has no unresolved block for the exact scope key"})
					continue
				}
				outstanding = false
			}
		}
		if !outstanding {
			continue
		}
		latest := entries[0]
		item := JournalContextBlockerItem{Key: key, Block: latest.Entry}
		for _, prior := range entries[1:] {
			if prior.Entry.EntryType == "unblock" {
				item.PreviousUnblockID = prior.Entry.ID
				break
			}
		}
		candidates = append(candidates, journalContextBlockerCandidate{Item: item, CreatedAt: latest.Entry.CreatedAt, RowID: latest.RowID})
	}
	sort.Slice(candidates, func(i, j int) bool {
		return journalContextNewer(candidates[i].CreatedAt, candidates[i].RowID, candidates[j].CreatedAt, candidates[j].RowID)
	})
	sort.Slice(diagnostics, func(i, j int) bool { return diagnostics[i].Key < diagnostics[j].Key })
	return candidates, diagnostics, nil
}

// queryDeferralCandidates builds the deferred-intent candidate set. Canonical
// Intents whose latest disposition is 'deferred' come first as active truth
// (never bounded by the journal watermark), followed by pre-conversion legacy
// journal_deferrals rows that have no intent_operations mapping. A converted
// legacy row is represented by its canonical Intent item, not a second entry.
func queryDeferralCandidates(ctx context.Context, tx *sql.Tx, projectID string, watermark int64) ([]journalContextDeferralCandidate, error) {
	canonical, err := queryCanonicalDeferralCandidates(ctx, tx, projectID)
	if err != nil {
		return nil, err
	}
	legacy, err := queryLegacyDeferralCandidates(ctx, tx, projectID, watermark)
	if err != nil {
		return nil, err
	}
	return append(canonical, legacy...), nil
}

// queryCanonicalDeferralCandidates returns every canonical Intent whose latest
// disposition is 'deferred'. When the operation key that produced the current
// deferral carries a version-1 projection, the mapped decision/spark/origin
// rows enrich the item; version-0 (or unmapped) Intents leave those legacy
// fields zero-valued and rely on the intent sub-object alone.
func queryCanonicalDeferralCandidates(ctx context.Context, tx *sql.Tx, projectID string) ([]journalContextDeferralCandidate, error) {
	rows, err := tx.QueryContext(ctx, `
SELECT i.id, i.created_at, COALESCE(ia.alias, ''),
  COALESCE(sn.title, ''), d.disposition,
  df.operation_key, df.body, df.why, df.boundary, df.revisit_trigger,
  COALESCE(op.journal_entry_id, ''), COALESCE(op.spark_id, ''),
  COALESCE(j.id, ''), COALESCE(j.entry_type, ''), COALESCE(j.scope, ''), COALESCE(j.message, ''),
  COALESCE(j.observed_branch, ''), COALESCE(j.observed_worktree, ''), COALESCE(j.harness_session_id, ''),
  COALESCE(j.created_at, ''), COALESCE(j.rowid, 0),
  COALESCE(s.id, ''), COALESCE(sa.alias, ''), COALESCE(s.text, ''), COALESCE(s.scope, ''),
  COALESCE(s.status, ''), COALESCE(s.created_at, ''), COALESCE(s.updated_at, ''),
  o.envelope_version, o.capture_mechanism, o.branch, o.worktree, o.head, o.change_path, o.dirty, o.reconstructable
FROM intents i
JOIN intent_dispositions d ON d.project_id = i.project_id AND d.intent_id = i.id
  AND d.seq = (SELECT MAX(seq) FROM intent_dispositions WHERE intent_id = i.id)
JOIN intent_deferrals df ON df.project_id = i.project_id AND df.id = d.deferral_id
LEFT JOIN intent_snapshots sn ON sn.project_id = i.project_id AND sn.intent_id = i.id
  AND sn.seq = (SELECT MAX(seq) FROM intent_snapshots WHERE intent_id = i.id)
LEFT JOIN (
  SELECT project_id, entity_id, MIN(alias) AS alias
  FROM aliases
  WHERE entity_kind = 'intent' AND namespace = 'intent'
  GROUP BY project_id, entity_id
) ia ON ia.project_id = i.project_id AND ia.entity_id = i.id
LEFT JOIN intent_operations op ON op.project_id = i.project_id AND op.operation_key = df.operation_key
LEFT JOIN journal_entries j ON j.project_id = i.project_id AND j.id = op.journal_entry_id
LEFT JOIN sparks s ON s.project_id = i.project_id AND s.id = op.spark_id
LEFT JOIN (
  SELECT project_id, entity_id, MIN(alias) AS alias
  FROM aliases
  WHERE entity_kind = 'spark' AND namespace = 'spark'
  GROUP BY project_id, entity_id
) sa ON sa.project_id = s.project_id AND sa.entity_id = s.id
LEFT JOIN journal_origins o ON o.project_id = i.project_id AND o.journal_entry_id = op.journal_entry_id
WHERE i.project_id = ? AND d.disposition = 'deferred'
ORDER BY i.created_at DESC, i.id DESC
`, projectID)
	if err != nil {
		return nil, fmt.Errorf("query journal context canonical deferrals: %w", err)
	}
	defer rows.Close()
	candidates := []journalContextDeferralCandidate{}
	for rows.Next() {
		var candidate journalContextDeferralCandidate
		var intent JournalContextDeferredIntent
		var journalID, sparkID string
		var envelopeVersion sql.NullInt64
		var mechanism, branch, worktree, head, changePath sql.NullString
		var dirty, reconstructable sql.NullBool
		if err := rows.Scan(
			&intent.ID, &candidate.CreatedAt, &intent.Alias,
			&intent.Title, &intent.Disposition,
			&candidate.Item.OperationKey, &intent.Body, &intent.Why, &intent.Boundary, &intent.RevisitTrigger,
			&journalID, &sparkID,
			&candidate.Item.Decision.ID, &candidate.Item.Decision.EntryType, &candidate.Item.Decision.Scope, &candidate.Item.Decision.Message,
			&candidate.Item.Decision.ObservedBranch, &candidate.Item.Decision.ObservedWorktree, &candidate.Item.Decision.HarnessSessionID,
			&candidate.Item.Decision.CreatedAt, &candidate.RowID,
			&candidate.Item.Spark.ID, &candidate.Item.Spark.Alias, &candidate.Item.Spark.Text, &candidate.Item.Spark.Scope,
			&candidate.Item.Spark.Status, &candidate.Item.Spark.CreatedAt, &candidate.Item.Spark.UpdatedAt,
			&envelopeVersion, &mechanism, &branch, &worktree, &head, &changePath, &dirty, &reconstructable,
		); err != nil {
			return nil, fmt.Errorf("scan journal context canonical deferral: %w", err)
		}
		candidate.Canonical = true
		candidate.Item.Intent = &intent
		// Without a version-1 projection there is no legacy decision/spark to
		// surface; never fabricate one from the canonical rows.
		if journalID == "" || sparkID == "" {
			candidate.Item.Decision = JournalEntryRecord{}
			candidate.Item.Spark = JournalContextDeferredSpark{}
		} else if mechanism.Valid {
			candidate.Item.Origin = buildJournalContextOrigin(envelopeVersion, mechanism, branch, worktree, head, changePath, dirty, reconstructable)
		}
		candidates = append(candidates, candidate)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate journal context canonical deferrals: %w", err)
	}
	return candidates, nil
}

// queryLegacyDeferralCandidates returns pre-conversion legacy journal_deferrals
// rows (open spark) that have no intent_operations mapping. A row with a
// mapping is deduplicated: its canonical Intent item represents it instead.
func queryLegacyDeferralCandidates(ctx context.Context, tx *sql.Tx, projectID string, watermark int64) ([]journalContextDeferralCandidate, error) {
	rows, err := tx.QueryContext(ctx, `
SELECT d.operation_key,
  j.id, j.entry_type, COALESCE(j.scope, ''), j.message, COALESCE(j.observed_branch, ''),
  COALESCE(j.observed_worktree, ''), COALESCE(j.harness_session_id, ''), j.created_at, j.rowid,
  s.id, COALESCE(a.alias, ''), s.text, COALESCE(s.scope, ''), s.status, s.created_at, s.updated_at,
  o.envelope_version, o.capture_mechanism, o.branch, o.worktree, o.head, o.change_path, o.dirty, o.reconstructable
FROM journal_deferrals d
JOIN journal_entries j ON j.project_id = d.project_id AND j.id = d.journal_entry_id AND j.rowid <= ?
JOIN sparks s ON s.project_id = d.project_id AND s.id = d.spark_id AND s.status = 'open'
LEFT JOIN (
  SELECT project_id, entity_id, MIN(alias) AS alias
  FROM aliases
  WHERE entity_kind = 'spark' AND namespace = 'spark'
  GROUP BY project_id, entity_id
) a ON a.project_id = s.project_id AND a.entity_id = s.id
LEFT JOIN journal_origins o ON o.project_id = d.project_id AND o.journal_entry_id = j.id
WHERE d.project_id = ?
  AND NOT EXISTS (
    SELECT 1 FROM intent_operations op
    WHERE op.project_id = d.project_id AND op.operation_key = d.operation_key
  )
ORDER BY j.created_at DESC, j.rowid DESC, d.operation_key
`, watermark, projectID)
	if err != nil {
		return nil, fmt.Errorf("query journal context legacy deferrals: %w", err)
	}
	defer rows.Close()
	candidates := []journalContextDeferralCandidate{}
	for rows.Next() {
		var candidate journalContextDeferralCandidate
		var envelopeVersion sql.NullInt64
		var mechanism, branch, worktree, head, changePath sql.NullString
		var dirty, reconstructable sql.NullBool
		if err := rows.Scan(
			&candidate.Item.OperationKey,
			&candidate.Item.Decision.ID, &candidate.Item.Decision.EntryType, &candidate.Item.Decision.Scope, &candidate.Item.Decision.Message,
			&candidate.Item.Decision.ObservedBranch, &candidate.Item.Decision.ObservedWorktree, &candidate.Item.Decision.HarnessSessionID,
			&candidate.Item.Decision.CreatedAt, &candidate.RowID,
			&candidate.Item.Spark.ID, &candidate.Item.Spark.Alias, &candidate.Item.Spark.Text, &candidate.Item.Spark.Scope,
			&candidate.Item.Spark.Status, &candidate.Item.Spark.CreatedAt, &candidate.Item.Spark.UpdatedAt,
			&envelopeVersion, &mechanism, &branch, &worktree, &head, &changePath, &dirty, &reconstructable,
		); err != nil {
			return nil, fmt.Errorf("scan journal context legacy deferral: %w", err)
		}
		candidate.CreatedAt = candidate.Item.Decision.CreatedAt
		if mechanism.Valid {
			candidate.Item.Origin = buildJournalContextOrigin(envelopeVersion, mechanism, branch, worktree, head, changePath, dirty, reconstructable)
		}
		candidates = append(candidates, candidate)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate journal context legacy deferrals: %w", err)
	}
	return candidates, nil
}

func buildJournalContextOrigin(envelopeVersion sql.NullInt64, mechanism, branch, worktree, head, changePath sql.NullString, dirty, reconstructable sql.NullBool) *JournalContextOriginSummary {
	origin := &JournalContextOriginSummary{CaptureMechanism: mechanism.String, Branch: branch.String, Worktree: worktree.String, Head: head.String, ChangePath: changePath.String, Supported: envelopeVersion.Valid && envelopeVersion.Int64 <= JournalOriginEnvelopeVersion}
	if dirty.Valid {
		value := dirty.Bool
		origin.Dirty = &value
	}
	if reconstructable.Valid {
		value := reconstructable.Bool
		origin.Reconstructable = &value
	}
	return origin
}

// queryExplorationCheckpointCandidates returns the latest checkpoint of every
// Exploration that has at least one, newest checkpoint first. An Exploration
// with no checkpoint is absent: a conversation handle never implies portable
// context, so it must not surface here.
func queryExplorationCheckpointCandidates(ctx context.Context, tx *sql.Tx, projectID string) ([]journalContextCheckpointCandidate, error) {
	rows, err := tx.QueryContext(ctx, `
SELECT e.id, COALESCE(ea.alias, ''), e.title, c.seq, c.next_action, c.created_at
FROM explorations e
JOIN exploration_checkpoints c ON c.project_id = e.project_id AND c.exploration_id = e.id
  AND c.seq = (SELECT MAX(seq) FROM exploration_checkpoints WHERE exploration_id = e.id)
LEFT JOIN (
  SELECT project_id, entity_id, MIN(alias) AS alias
  FROM aliases
  WHERE entity_kind = 'exploration' AND namespace = 'exploration'
  GROUP BY project_id, entity_id
) ea ON ea.project_id = e.project_id AND ea.entity_id = e.id
WHERE e.project_id = ?
ORDER BY c.created_at DESC, e.id
`, projectID)
	if err != nil {
		return nil, fmt.Errorf("query journal context exploration checkpoints: %w", err)
	}
	defer rows.Close()
	candidates := []journalContextCheckpointCandidate{}
	for rows.Next() {
		var candidate journalContextCheckpointCandidate
		if err := rows.Scan(&candidate.Item.ExplorationID, &candidate.Item.Alias, &candidate.Item.Title, &candidate.Item.Seq, &candidate.Item.NextAction, &candidate.Item.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan journal context exploration checkpoint: %w", err)
		}
		reference := candidate.Item.Alias
		if reference == "" {
			reference = candidate.Item.ExplorationID
		}
		candidate.Item.ContextCommand = "loaf exploration context " + reference
		candidates = append(candidates, candidate)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate journal context exploration checkpoints: %w", err)
	}
	return candidates, nil
}

func queryJournalContextTasks(ctx context.Context, tx *sql.Tx, projectID string) ([]JournalContextTask, error) {
	rows, err := tx.QueryContext(ctx, `
SELECT a.alias, t.title, t.status, COALESCE(t.priority, ''), COALESCE(sa.alias, '')
FROM tasks t
JOIN (
  SELECT project_id, entity_id, MIN(alias) AS alias
  FROM aliases
  WHERE entity_kind = 'task' AND namespace = 'task'
  GROUP BY project_id, entity_id
) a ON a.project_id = t.project_id AND a.entity_id = t.id
LEFT JOIN (
  SELECT project_id, entity_id, MIN(alias) AS alias
  FROM aliases
  WHERE entity_kind = 'spec' AND namespace = 'spec'
  GROUP BY project_id, entity_id
) sa ON sa.project_id = t.project_id AND sa.entity_id = t.spec_id
WHERE t.project_id = ? AND t.status NOT IN ('done', 'archived')
ORDER BY a.alias
`, projectID)
	if err != nil {
		return nil, fmt.Errorf("query journal context tasks: %w", err)
	}
	defer rows.Close()
	tasks := []JournalContextTask{}
	for rows.Next() {
		var task JournalContextTask
		if err := rows.Scan(&task.Ref, &task.Title, &task.Status, &task.Priority, &task.Spec); err != nil {
			return nil, fmt.Errorf("scan journal context task: %w", err)
		}
		task.Status = LifecycleStatusForDisplay(LifecycleEntityTask, task.Status)
		tasks = append(tasks, task)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate journal context tasks: %w", err)
	}
	return tasks, nil
}

func contextLimit(value, fallback int) int {
	if value <= 0 {
		return fallback
	}
	return value
}

func journalContextLimitForLayer(options JournalContextOptions, layer string) int {
	switch layer {
	case JournalContextLayerLineage:
		return contextLimit(options.LineageLimit, defaultJournalContextLineageLimit)
	case JournalContextLayerBlockers:
		return contextLimit(options.BlockerLimit, defaultJournalContextBlockerLimit)
	case JournalContextLayerDeferrals:
		return contextLimit(options.DeferralLimit, defaultJournalContextDeferralLimit)
	case JournalContextLayerBranch:
		return contextLimit(options.BranchLimit, defaultJournalContextBranchLimit)
	case JournalContextLayerTasks:
		return contextLimit(options.TaskLimit, defaultJournalContextTaskLimit)
	default:
		return journalContextSynthesisLimit
	}
}

func canonicalJournalContextLineageKeys(keys []string) []string {
	unique := map[string]struct{}{}
	for _, key := range keys {
		key = strings.TrimPrefix(strings.TrimSpace(key), "lineage/")
		if key != "" {
			unique["lineage/"+key] = struct{}{}
		}
	}
	canonical := make([]string, 0, len(unique))
	for key := range unique {
		canonical = append(canonical, key)
	}
	sort.Strings(canonical)
	return canonical
}

func journalContextNewer(aCreated string, aRow int64, bCreated string, bRow int64) bool {
	return aCreated > bCreated || (aCreated == bCreated && aRow > bRow)
}

func journalContextJournalKey(candidate journalContextJournalCandidate) string {
	return fmt.Sprintf("%s\x00%020d\x00%s", candidate.Entry.CreatedAt, candidate.RowID, candidate.Entry.ID)
}

func journalContextBlockerKey(candidate journalContextBlockerCandidate) string {
	return fmt.Sprintf("%s\x00%020d\x00%s", candidate.CreatedAt, candidate.RowID, candidate.Item.Key)
}

func journalContextDeferralKey(candidate journalContextDeferralCandidate) string {
	return fmt.Sprintf("%s\x00%020d\x00%s", candidate.CreatedAt, candidate.RowID, candidate.Item.OperationKey)
}

func journalContextFingerprintDeferrals(candidates []journalContextDeferralCandidate) string {
	parts := make([]string, 0, len(candidates)*24)
	for _, candidate := range candidates {
		item := candidate.Item
		parts = append(parts,
			journalContextDeferralKey(candidate),
			fmt.Sprintf("canonical:%t", candidate.Canonical),
			item.OperationKey,
			item.Decision.ID,
			item.Decision.EntryType,
			item.Decision.Scope,
			item.Decision.Message,
			item.Decision.ObservedBranch,
			item.Decision.ObservedWorktree,
			item.Decision.HarnessSessionID,
			item.Decision.CreatedAt,
			item.Spark.ID,
			item.Spark.Alias,
			item.Spark.Text,
			item.Spark.Scope,
			item.Spark.Status,
			item.Spark.CreatedAt,
			item.Spark.UpdatedAt,
		)
		if item.Intent == nil {
			parts = append(parts, "intent:nil")
		} else {
			parts = append(parts,
				"intent:present",
				item.Intent.ID,
				item.Intent.Alias,
				item.Intent.Title,
				item.Intent.Disposition,
				item.Intent.Body,
				item.Intent.Why,
				item.Intent.Boundary,
				item.Intent.RevisitTrigger,
			)
		}
		if item.Origin == nil {
			parts = append(parts, "origin:nil")
			continue
		}
		parts = append(parts,
			"origin:present",
			item.Origin.CaptureMechanism,
			item.Origin.Branch,
			item.Origin.Worktree,
			item.Origin.Head,
			item.Origin.ChangePath,
			journalContextBoolPointerFingerprint(item.Origin.Dirty),
			journalContextBoolPointerFingerprint(item.Origin.Reconstructable),
			fmt.Sprintf("supported:%t", item.Origin.Supported),
		)
	}
	return journalContextFingerprint(parts)
}

func journalContextFingerprintTasks(tasks []JournalContextTask) string {
	parts := make([]string, 0, len(tasks))
	for _, task := range tasks {
		parts = append(parts, task.Ref+"\x00"+task.Title+"\x00"+task.Status+"\x00"+task.Priority+"\x00"+task.Spec)
	}
	return journalContextFingerprint(parts)
}

func journalContextFingerprint(parts []string) string {
	var source strings.Builder
	for _, part := range parts {
		fmt.Fprintf(&source, "%d:%s", len(part), part)
	}
	hash := sha256.Sum256([]byte(source.String()))
	return hex.EncodeToString(hash[:])
}

func journalContextBoolPointerFingerprint(value *bool) string {
	if value == nil {
		return "nil"
	}
	return fmt.Sprintf("present:%t", *value)
}
