package state

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/levifig/loaf/internal/project"
)

const (
	explorationTitleMaxBytes = 200
	checkpointFieldMaxBytes  = 4096
	checkpointItemMaxBytes   = 4096
)

// ExplorationValidationError identifies malformed exploration input with a
// stable field name; oversize core input is rejected, never truncated.
type ExplorationValidationError struct {
	Field string
	Err   error
}

func (e *ExplorationValidationError) Error() string {
	if e == nil {
		return "exploration validation failed"
	}
	return fmt.Sprintf("exploration validation failed for %s: %v", e.Field, e.Err)
}

func (e *ExplorationValidationError) Unwrap() error { return e.Err }

// ExplorationTransactionError identifies the transactional stage that failed.
type ExplorationTransactionError struct {
	Stage string
	Err   error
}

func (e *ExplorationTransactionError) Error() string {
	if e == nil {
		return "exploration transaction failed"
	}
	return fmt.Sprintf("exploration transaction failed at %s: %v", e.Stage, e.Err)
}

func (e *ExplorationTransactionError) Unwrap() error { return e.Err }

// ExplorationCreateOptions describes a new Exploration identity.
type ExplorationCreateOptions struct {
	Title   string
	Sources []string
}

// CheckpointItemInput is one ordered typed optional detail entry.
type CheckpointItemInput struct {
	Type    string
	Content string
}

// ExplorationCheckpointOptions appends one immutable portable checkpoint.
type ExplorationCheckpointOptions struct {
	ExplorationRef string
	Purpose        string
	Conclusions    string
	Unresolved     string
	NextAction     string
	Items          []CheckpointItemInput
	OperationID    string
}

// CheckpointItemDetail is one stored ordered item.
type CheckpointItemDetail struct {
	Type     string `json:"type"`
	Position int    `json:"position"`
	Content  string `json:"content"`
}

// CheckpointDetail is one immutable checkpoint read model.
type CheckpointDetail struct {
	ID            string `json:"id"`
	Seq           int    `json:"seq"`
	Purpose       string `json:"purpose"`
	Conclusions   string `json:"conclusions"`
	Unresolved    string `json:"unresolved"`
	NextAction    string `json:"next_action"`
	ContentDigest string `json:"content_digest"`
	CreatedAt     string `json:"created_at"`
}

// ExplorationMutationResult is the shared envelope for exploration writes.
type ExplorationMutationResult struct {
	ContractVersion    int               `json:"contract_version"`
	DatabaseScope      string            `json:"database_scope,omitempty"`
	DatabasePath       string            `json:"database_path,omitempty"`
	ProjectID          string            `json:"project_id,omitempty"`
	ProjectName        string            `json:"project_name,omitempty"`
	ProjectCurrentPath string            `json:"project_current_path,omitempty"`
	OperationID        string            `json:"operation_id,omitempty"`
	Created            bool              `json:"created"`
	InputDigest        string            `json:"input_digest,omitempty"`
	StoredDigest       string            `json:"stored_digest,omitempty"`
	InputDigestMatches bool              `json:"input_digest_matches"`
	Exploration        ExplorationDetail `json:"exploration"`
	Checkpoint         *CheckpointDetail `json:"checkpoint,omitempty"`
	ItemCount          int               `json:"item_count,omitempty"`
}

// ExplorationDetail is the compact exploration identity read model.
type ExplorationDetail struct {
	ID                     string `json:"id"`
	Alias                  string `json:"alias,omitempty"`
	Title                  string `json:"title"`
	CheckpointCount        int    `json:"checkpoint_count"`
	LatestCheckpointSeq    int    `json:"latest_checkpoint_seq,omitempty"`
	PortableContextPresent bool   `json:"portable_context_present"`
	CreatedAt              string `json:"created_at"`
}

// ExplorationListResult lists explorations deterministically.
type ExplorationListResult struct {
	ContractVersion    int                 `json:"contract_version"`
	DatabaseScope      string              `json:"database_scope,omitempty"`
	DatabasePath       string              `json:"database_path,omitempty"`
	ProjectID          string              `json:"project_id,omitempty"`
	ProjectName        string              `json:"project_name,omitempty"`
	ProjectCurrentPath string              `json:"project_current_path,omitempty"`
	Explorations       []ExplorationDetail `json:"explorations"`
}

func validateCheckpointCoreField(field, value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", &ExplorationValidationError{Field: field, Err: fmt.Errorf("must be nonempty")}
	}
	if len(trimmed) > checkpointFieldMaxBytes {
		return "", &ExplorationValidationError{Field: field, Err: fmt.Errorf("exceeds %d UTF-8 bytes; store detail in checkpoint items or related evidence instead of truncating", checkpointFieldMaxBytes)}
	}
	for _, r := range trimmed {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return "", &ExplorationValidationError{Field: field, Err: fmt.Errorf("contains control characters")}
		}
	}
	return trimmed, nil
}

// CreateExploration writes a new Exploration identity with optional source
// relationships. There is no lifecycle status to initialize.
func CreateExploration(ctx context.Context, root project.Root, resolver PathResolver, options ExplorationCreateOptions) (ExplorationMutationResult, error) {
	store, err := openProjectStoreMutateExisting(ctx, root, resolver)
	if err != nil {
		return ExplorationMutationResult{}, err
	}
	defer store.Close()
	return store.CreateExploration(ctx, root, options)
}

// CreateExploration writes a new Exploration identity on an open store.
func (s *Store) CreateExploration(ctx context.Context, root project.Root, options ExplorationCreateOptions) (ExplorationMutationResult, error) {
	title := strings.TrimSpace(options.Title)
	if title == "" {
		return ExplorationMutationResult{}, &ExplorationValidationError{Field: "title", Err: fmt.Errorf("must be nonempty")}
	}
	if len(title) > explorationTitleMaxBytes {
		return ExplorationMutationResult{}, &ExplorationValidationError{Field: "title", Err: fmt.Errorf("exceeds %d bytes", explorationTitleMaxBytes)}
	}
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return ExplorationMutationResult{}, err
	}
	identity, err := s.projectIdentity(ctx, projectID)
	if err != nil {
		return ExplorationMutationResult{}, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return ExplorationMutationResult{}, &ExplorationTransactionError{Stage: "begin", Err: err}
	}
	defer tx.Rollback()

	now := time.Now().UTC()
	timestamp := now.Format(time.RFC3339Nano)
	alias, err := s.nextExplorationAlias(ctx, tx, projectID, title, now)
	if err != nil {
		return ExplorationMutationResult{}, &ExplorationTransactionError{Stage: "alias allocation", Err: err}
	}
	explorationID := stableMigrationID("exploration", projectID, alias)
	if _, err := tx.ExecContext(ctx, `
INSERT INTO explorations (id, project_id, title, created_at) VALUES (?, ?, ?, ?)
`, explorationID, projectID, title, timestamp); err != nil {
		return ExplorationMutationResult{}, &ExplorationTransactionError{Stage: "exploration", Err: err}
	}
	if err := insertAlias(ctx, tx, projectID, "exploration", explorationID, "exploration", alias, timestamp); err != nil {
		return ExplorationMutationResult{}, &ExplorationTransactionError{Stage: "alias", Err: err}
	}
	if err := writeExplorationSourceRelationshipsTx(ctx, tx, projectID, explorationID, options.Sources, timestamp); err != nil {
		return ExplorationMutationResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return ExplorationMutationResult{}, &ExplorationTransactionError{Stage: "commit", Err: err}
	}
	return ExplorationMutationResult{
		ContractVersion:    StateJSONContractVersion,
		DatabaseScope:      identity.DatabaseScope,
		DatabasePath:       identity.DatabasePath,
		ProjectID:          identity.ID,
		ProjectName:        identity.FriendlyName,
		ProjectCurrentPath: identity.CurrentPath,
		Created:            true,
		InputDigestMatches: true,
		Exploration: ExplorationDetail{
			ID:        explorationID,
			Alias:     alias,
			Title:     title,
			CreatedAt: timestamp,
		},
	}, nil
}

// explorationWriteHooks injects failures between checkpoint stages in tests.
type explorationWriteHooks struct {
	afterCheckpoint func(*sql.Tx) error
	afterItems      func(*sql.Tx) error
	beforeCommit    func(*sql.Tx) error
}

// AppendExplorationCheckpoint appends one immutable portable checkpoint.
func AppendExplorationCheckpoint(ctx context.Context, root project.Root, resolver PathResolver, options ExplorationCheckpointOptions) (ExplorationMutationResult, error) {
	store, err := openProjectStoreMutateExisting(ctx, root, resolver)
	if err != nil {
		return ExplorationMutationResult{}, err
	}
	defer store.Close()
	return store.AppendExplorationCheckpoint(ctx, root, options)
}

// AppendExplorationCheckpoint appends one immutable checkpoint on an open store.
func (s *Store) AppendExplorationCheckpoint(ctx context.Context, root project.Root, options ExplorationCheckpointOptions) (ExplorationMutationResult, error) {
	return s.appendExplorationCheckpointWithHooks(ctx, root, options, nil)
}

func (s *Store) appendExplorationCheckpointWithHooks(ctx context.Context, root project.Root, options ExplorationCheckpointOptions, hooks *explorationWriteHooks) (ExplorationMutationResult, error) {
	purpose, err := validateCheckpointCoreField("purpose", options.Purpose)
	if err != nil {
		return ExplorationMutationResult{}, err
	}
	conclusions, err := validateCheckpointCoreField("conclusions", options.Conclusions)
	if err != nil {
		return ExplorationMutationResult{}, err
	}
	unresolved, err := validateCheckpointCoreField("unresolved", options.Unresolved)
	if err != nil {
		return ExplorationMutationResult{}, err
	}
	nextAction, err := validateCheckpointCoreField("next_action", options.NextAction)
	if err != nil {
		return ExplorationMutationResult{}, err
	}
	operationID := strings.TrimSpace(options.OperationID)
	if len(operationID) > intentOperationMaxBytes {
		return ExplorationMutationResult{}, &ExplorationValidationError{Field: "operation_id", Err: fmt.Errorf("exceeds %d bytes", intentOperationMaxBytes)}
	}
	items := make([]CheckpointItemDetail, 0, len(options.Items))
	for index, item := range options.Items {
		itemType := strings.TrimSpace(item.Type)
		if err := validateCheckpointItemType(itemType); err != nil {
			return ExplorationMutationResult{}, &ExplorationValidationError{Field: fmt.Sprintf("item[%d].type", index), Err: err}
		}
		content := strings.TrimSpace(item.Content)
		if content == "" {
			return ExplorationMutationResult{}, &ExplorationValidationError{Field: fmt.Sprintf("item[%d].content", index), Err: fmt.Errorf("must be nonempty")}
		}
		if len(content) > checkpointItemMaxBytes {
			return ExplorationMutationResult{}, &ExplorationValidationError{Field: fmt.Sprintf("item[%d].content", index), Err: fmt.Errorf("exceeds %d UTF-8 bytes", checkpointItemMaxBytes)}
		}
		items = append(items, CheckpointItemDetail{Type: itemType, Position: index + 1, Content: content})
	}

	// The idempotency digest covers the complete requested checkpoint: the
	// four-field core plus every ordered typed item, so a retry that differs
	// only in items reports a digest mismatch instead of a silent match.
	digestParts := []string{purpose, conclusions, unresolved, nextAction}
	for _, item := range items {
		digestParts = append(digestParts, item.Type+":"+item.Content)
	}
	inputDigest := intentDigest(strings.Join(digestParts, "\x00"))

	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return ExplorationMutationResult{}, err
	}
	identity, err := s.projectIdentity(ctx, projectID)
	if err != nil {
		return ExplorationMutationResult{}, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return ExplorationMutationResult{}, &ExplorationTransactionError{Stage: "begin", Err: err}
	}
	defer tx.Rollback()

	explorationID, alias, err := resolveExplorationRefTx(ctx, tx, projectID, options.ExplorationRef)
	if err != nil {
		return ExplorationMutationResult{}, err
	}

	if operationID != "" {
		var existingID, existingExploration string
		var existingSeq int
		var existingDigest string
		err := tx.QueryRowContext(ctx, `
SELECT id, exploration_id, seq, content_digest FROM exploration_checkpoints
WHERE project_id = ? AND operation_key = ?
`, projectID, operationID).Scan(&existingID, &existingExploration, &existingSeq, &existingDigest)
		switch {
		case err == nil:
			if existingExploration != explorationID {
				return ExplorationMutationResult{}, &ExplorationValidationError{Field: "operation_id", Err: fmt.Errorf("operation key %q is already bound to a checkpoint on exploration %s and cannot append to %s; use a distinct operation key", operationID, existingExploration, explorationID)}
			}
			checkpoint, loadErr := loadCheckpointTx(ctx, tx, projectID, existingID)
			if loadErr != nil {
				return ExplorationMutationResult{}, &ExplorationTransactionError{Stage: "load established checkpoint", Err: loadErr}
			}
			detail, loadErr := loadExplorationDetailTx(ctx, tx, projectID, explorationID)
			if loadErr != nil {
				return ExplorationMutationResult{}, &ExplorationTransactionError{Stage: "load exploration", Err: loadErr}
			}
			if err := tx.Commit(); err != nil {
				return ExplorationMutationResult{}, &ExplorationTransactionError{Stage: "commit retry", Err: err}
			}
			return ExplorationMutationResult{
				ContractVersion:    StateJSONContractVersion,
				DatabaseScope:      identity.DatabaseScope,
				DatabasePath:       identity.DatabasePath,
				ProjectID:          identity.ID,
				ProjectName:        identity.FriendlyName,
				ProjectCurrentPath: identity.CurrentPath,
				OperationID:        operationID,
				Created:            false,
				InputDigest:        inputDigest,
				StoredDigest:       existingDigest,
				InputDigestMatches: inputDigest == existingDigest,
				Exploration:        detail,
				Checkpoint:         &checkpoint,
			}, nil
		case !errors.Is(err, sql.ErrNoRows):
			return ExplorationMutationResult{}, &ExplorationTransactionError{Stage: "lookup operation key", Err: err}
		}
	}

	seq, err := nextAggregateSeq(ctx, tx, "exploration_checkpoints", "exploration_id", explorationID)
	if err != nil {
		return ExplorationMutationResult{}, &ExplorationTransactionError{Stage: "sequence", Err: err}
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	checkpointID := stableMigrationID("exploration-checkpoint", projectID, explorationID, fmt.Sprintf("%d", seq))
	operationValue := sql.NullString{}
	if operationID != "" {
		operationValue = sql.NullString{String: operationID, Valid: true}
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO exploration_checkpoints (id, project_id, exploration_id, seq, purpose, conclusions, unresolved, next_action, operation_key, content_digest, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`, checkpointID, projectID, explorationID, seq, purpose, conclusions, unresolved, nextAction, operationValue, inputDigest, now); err != nil {
		return ExplorationMutationResult{}, &ExplorationTransactionError{Stage: "checkpoint", Err: err}
	}
	if hooks != nil && hooks.afterCheckpoint != nil {
		if err := hooks.afterCheckpoint(tx); err != nil {
			return ExplorationMutationResult{}, &ExplorationTransactionError{Stage: "after checkpoint", Err: err}
		}
	}
	for _, item := range items {
		itemID := stableMigrationID("exploration-checkpoint-item", projectID, checkpointID, fmt.Sprintf("%d", item.Position))
		if _, err := tx.ExecContext(ctx, `
INSERT INTO exploration_checkpoint_items (id, project_id, checkpoint_id, item_type, position, content, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
`, itemID, projectID, checkpointID, item.Type, item.Position, item.Content, now); err != nil {
			return ExplorationMutationResult{}, &ExplorationTransactionError{Stage: "items", Err: err}
		}
	}
	if hooks != nil && hooks.afterItems != nil {
		if err := hooks.afterItems(tx); err != nil {
			return ExplorationMutationResult{}, &ExplorationTransactionError{Stage: "after items", Err: err}
		}
	}
	if hooks != nil && hooks.beforeCommit != nil {
		if err := hooks.beforeCommit(tx); err != nil {
			return ExplorationMutationResult{}, &ExplorationTransactionError{Stage: "before commit", Err: err}
		}
	}
	detail, err := loadExplorationDetailTx(ctx, tx, projectID, explorationID)
	if err != nil {
		return ExplorationMutationResult{}, &ExplorationTransactionError{Stage: "load exploration", Err: err}
	}
	if err := tx.Commit(); err != nil {
		return ExplorationMutationResult{}, &ExplorationTransactionError{Stage: "commit", Err: err}
	}
	_ = alias
	return ExplorationMutationResult{
		ContractVersion:    StateJSONContractVersion,
		DatabaseScope:      identity.DatabaseScope,
		DatabasePath:       identity.DatabasePath,
		ProjectID:          identity.ID,
		ProjectName:        identity.FriendlyName,
		ProjectCurrentPath: identity.CurrentPath,
		OperationID:        operationID,
		Created:            true,
		InputDigest:        inputDigest,
		StoredDigest:       inputDigest,
		InputDigestMatches: true,
		Exploration:        detail,
		Checkpoint: &CheckpointDetail{
			ID:            checkpointID,
			Seq:           seq,
			Purpose:       purpose,
			Conclusions:   conclusions,
			Unresolved:    unresolved,
			NextAction:    nextAction,
			ContentDigest: inputDigest,
			CreatedAt:     now,
		},
		ItemCount: len(items),
	}, nil
}

// ListExplorations returns the deterministic exploration list projection.
func ListExplorations(ctx context.Context, root project.Root, resolver PathResolver) (ExplorationListResult, error) {
	store, err := openProjectStoreReadExisting(ctx, root, resolver)
	if err != nil {
		return ExplorationListResult{}, err
	}
	defer store.Close()
	projectID, err := store.projectID(ctx, root)
	if err != nil {
		return ExplorationListResult{}, err
	}
	identity, err := store.projectIdentity(ctx, projectID)
	if err != nil {
		return ExplorationListResult{}, err
	}
	rows, err := store.db.QueryContext(ctx, `
SELECT e.id,
  COALESCE((SELECT alias FROM aliases WHERE project_id = e.project_id AND entity_kind = 'exploration' AND entity_id = e.id ORDER BY namespace, alias LIMIT 1), ''),
  e.title,
  (SELECT COUNT(*) FROM exploration_checkpoints WHERE exploration_id = e.id),
  COALESCE((SELECT MAX(seq) FROM exploration_checkpoints WHERE exploration_id = e.id), 0),
  e.created_at
FROM explorations AS e
WHERE e.project_id = ?
ORDER BY e.created_at, e.id
`, projectID)
	if err != nil {
		return ExplorationListResult{}, fmt.Errorf("list explorations: %w", err)
	}
	defer rows.Close()
	items := []ExplorationDetail{}
	for rows.Next() {
		var detail ExplorationDetail
		if err := rows.Scan(&detail.ID, &detail.Alias, &detail.Title, &detail.CheckpointCount, &detail.LatestCheckpointSeq, &detail.CreatedAt); err != nil {
			return ExplorationListResult{}, fmt.Errorf("scan exploration: %w", err)
		}
		detail.PortableContextPresent = detail.CheckpointCount > 0
		items = append(items, detail)
	}
	if err := rows.Err(); err != nil {
		return ExplorationListResult{}, fmt.Errorf("iterate explorations: %w", err)
	}
	return ExplorationListResult{
		ContractVersion:    StateJSONContractVersion,
		DatabaseScope:      identity.DatabaseScope,
		DatabasePath:       identity.DatabasePath,
		ProjectID:          identity.ID,
		ProjectName:        identity.FriendlyName,
		ProjectCurrentPath: identity.CurrentPath,
		Explorations:       items,
	}, nil
}

func (s *Store) nextExplorationAlias(ctx context.Context, tx *sql.Tx, projectID string, title string, now time.Time) (string, error) {
	slug := normalizeSparkSlug(title)
	if slug == "" {
		slug = "exploration"
	}
	prefix := "EXPL-" + now.UTC().Format("20060102") + "-" + slug
	for next := 1; ; next++ {
		alias := prefix
		if next > 1 {
			alias = fmt.Sprintf("%s-%d", prefix, next)
		}
		var existing string
		err := tx.QueryRowContext(ctx, `SELECT id FROM aliases WHERE project_id = ? AND namespace = 'exploration' AND alias = ?`, projectID, alias).Scan(&existing)
		if errors.Is(err, sql.ErrNoRows) {
			return alias, nil
		}
		if err != nil {
			return "", fmt.Errorf("probe exploration alias %s: %w", alias, err)
		}
	}
}

func resolveExplorationRefTx(ctx context.Context, tx *sql.Tx, projectID string, ref string) (string, string, error) {
	trimmed := strings.TrimSpace(ref)
	if trimmed == "" {
		return "", "", &ExplorationValidationError{Field: "exploration", Err: fmt.Errorf("must be nonempty")}
	}
	var kind, id, alias string
	err := tx.QueryRowContext(ctx, `
SELECT entity_kind, entity_id, alias FROM aliases
WHERE project_id = ? AND alias = ?
ORDER BY namespace LIMIT 1
`, projectID, trimmed).Scan(&kind, &id, &alias)
	switch {
	case err == nil:
		if kind != "exploration" {
			return "", "", fmt.Errorf("%q resolves to %s, not an exploration", trimmed, kind)
		}
		return id, alias, nil
	case !errors.Is(err, sql.ErrNoRows):
		return "", "", fmt.Errorf("resolve exploration %q: %w", trimmed, err)
	}
	var existing string
	err = tx.QueryRowContext(ctx, `SELECT id FROM explorations WHERE project_id = ? AND id = ?`, projectID, trimmed).Scan(&existing)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", fmt.Errorf("exploration %q not found in SQLite state", trimmed)
	}
	if err != nil {
		return "", "", fmt.Errorf("resolve exploration %q: %w", trimmed, err)
	}
	return existing, "", nil
}

func loadExplorationDetailTx(ctx context.Context, tx *sql.Tx, projectID, explorationID string) (ExplorationDetail, error) {
	detail := ExplorationDetail{ID: explorationID}
	err := tx.QueryRowContext(ctx, `
SELECT title, created_at,
  (SELECT COUNT(*) FROM exploration_checkpoints WHERE exploration_id = explorations.id),
  COALESCE((SELECT MAX(seq) FROM exploration_checkpoints WHERE exploration_id = explorations.id), 0)
FROM explorations WHERE project_id = ? AND id = ?
`, projectID, explorationID).Scan(&detail.Title, &detail.CreatedAt, &detail.CheckpointCount, &detail.LatestCheckpointSeq)
	if errors.Is(err, sql.ErrNoRows) {
		return ExplorationDetail{}, fmt.Errorf("exploration %s not found", explorationID)
	}
	if err != nil {
		return ExplorationDetail{}, err
	}
	detail.PortableContextPresent = detail.CheckpointCount > 0
	if err := tx.QueryRowContext(ctx, `
SELECT alias FROM aliases WHERE project_id = ? AND entity_kind = 'exploration' AND entity_id = ?
ORDER BY namespace, alias LIMIT 1
`, projectID, explorationID).Scan(&detail.Alias); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return ExplorationDetail{}, err
	}
	return detail, nil
}

func loadCheckpointTx(ctx context.Context, tx *sql.Tx, projectID, checkpointID string) (CheckpointDetail, error) {
	var checkpoint CheckpointDetail
	err := tx.QueryRowContext(ctx, `
SELECT id, seq, purpose, conclusions, unresolved, next_action, content_digest, created_at
FROM exploration_checkpoints WHERE project_id = ? AND id = ?
`, projectID, checkpointID).Scan(&checkpoint.ID, &checkpoint.Seq, &checkpoint.Purpose, &checkpoint.Conclusions, &checkpoint.Unresolved, &checkpoint.NextAction, &checkpoint.ContentDigest, &checkpoint.CreatedAt)
	if err != nil {
		return CheckpointDetail{}, err
	}
	return checkpoint, nil
}

// writeExplorationSourceRelationshipsTx maps each source by its kind through
// the closed matrix: intents get explores edges; journal entries, handoffs,
// reports, and findings get uses-source edges. Anything else is rejected.
func writeExplorationSourceRelationshipsTx(ctx context.Context, tx *sql.Tx, projectID, explorationID string, refs []string, now string) error {
	seen := map[string]bool{}
	for _, ref := range refs {
		trimmed := strings.TrimSpace(ref)
		if trimmed == "" {
			continue
		}
		entity, err := resolveSourceEntityTx(ctx, tx, projectID, trimmed)
		if err != nil {
			return &ExplorationTransactionError{Stage: "resolve source", Err: err}
		}
		relationshipType := ""
		switch entity.Kind {
		case "intent":
			relationshipType = "explores"
		case "journal_entry", "handoff", "report", "finding":
			relationshipType = "uses-source"
		default:
			return &ExplorationValidationError{Field: "from", Err: fmt.Errorf("exploration cannot use %s %q as a source; supported sources are intents, journal entries, handoffs, reports, and findings", entity.Kind, trimmed)}
		}
		if err := validateRelationshipAgainstRegistry("exploration", relationshipType, entity.Kind); err != nil {
			return &ExplorationValidationError{Field: "from", Err: err}
		}
		key := entity.Kind + "\x00" + entity.ID
		if seen[key] {
			return &ExplorationValidationError{Field: "from", Err: fmt.Errorf("source %q is duplicated", trimmed)}
		}
		seen[key] = true
		relationshipID := stableMigrationID("relationship", projectID, "exploration", explorationID, relationshipType, entity.Kind, entity.ID)
		if _, err := tx.ExecContext(ctx, `
INSERT INTO relationships (id, project_id, from_entity_kind, from_entity_id, to_entity_kind, to_entity_id, relationship_type, reason, origin, created_at, updated_at)
VALUES (?, ?, 'exploration', ?, ?, ?, ?, 'recorded by exploration create', 'exploration-create', ?, ?)
`, relationshipID, projectID, explorationID, entity.Kind, entity.ID, relationshipType, now, now); err != nil {
			return &ExplorationTransactionError{Stage: "relationship", Err: err}
		}
	}
	return nil
}
