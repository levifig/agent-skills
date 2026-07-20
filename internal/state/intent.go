package state

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/levifig/loaf/internal/project"
)

const (
	intentTitleMaxBytes     = 200
	intentFieldMaxBytes     = 4096
	intentOperationMaxBytes = 200
)

// IntentValidationError identifies malformed intent input.
type IntentValidationError struct {
	Field string
	Err   error
}

func (e *IntentValidationError) Error() string {
	if e == nil {
		return "intent validation failed"
	}
	return fmt.Sprintf("intent validation failed for %s: %v", e.Field, e.Err)
}

func (e *IntentValidationError) Unwrap() error { return e.Err }

// IntentTransactionError identifies the transactional stage that failed.
type IntentTransactionError struct {
	Stage string
	Err   error
}

func (e *IntentTransactionError) Error() string {
	if e == nil {
		return "intent transaction failed"
	}
	return fmt.Sprintf("intent transaction failed at %s: %v", e.Stage, e.Err)
}

func (e *IntentTransactionError) Unwrap() error { return e.Err }

// IntentCreateOptions describes a new tracked or deferred Intent.
type IntentCreateOptions struct {
	Title       string
	Body        string
	Disposition string
	Reason      string
	Why         string
	Boundary    string
	Trigger     string
	OperationID string
	Sources     []string
}

// IntentDeferOptions defers an existing Intent with an immutable payload.
type IntentDeferOptions struct {
	IntentRef   string
	Why         string
	Boundary    string
	Trigger     string
	OperationID string
}

// IntentDispositionOptions appends a reasoned disposition (resume/resolve).
type IntentDispositionOptions struct {
	IntentRef string
	Reason    string
}

// IntentDeferralDetail is the immutable self-sufficient deferral payload.
type IntentDeferralDetail struct {
	ID             string `json:"id"`
	OperationKey   string `json:"operation_key"`
	Body           string `json:"body"`
	Why            string `json:"why"`
	Boundary       string `json:"boundary"`
	RevisitTrigger string `json:"revisit_trigger"`
	StoredDigest   string `json:"stored_digest"`
	CreatedAt      string `json:"created_at"`
}

// IntentDetail is the derived read model for one Intent.
type IntentDetail struct {
	ID                string                `json:"id"`
	Alias             string                `json:"alias,omitempty"`
	Title             string                `json:"title"`
	Body              string                `json:"body"`
	SnapshotSeq       int                   `json:"snapshot_seq"`
	ContentDigest     string                `json:"content_digest"`
	Disposition       string                `json:"disposition"`
	DispositionSeq    int                   `json:"disposition_seq"`
	DispositionReason string                `json:"disposition_reason,omitempty"`
	Deferral          *IntentDeferralDetail `json:"deferral,omitempty"`
	Sources           []TraceRelationship   `json:"sources"`
	CreatedAt         string                `json:"created_at"`
}

// IntentMutationResult is the shared envelope for intent writes.
type IntentMutationResult struct {
	ContractVersion    int          `json:"contract_version"`
	DatabaseScope      string       `json:"database_scope,omitempty"`
	DatabasePath       string       `json:"database_path,omitempty"`
	ProjectID          string       `json:"project_id,omitempty"`
	ProjectName        string       `json:"project_name,omitempty"`
	ProjectCurrentPath string       `json:"project_current_path,omitempty"`
	OperationID        string       `json:"operation_id,omitempty"`
	Created            bool         `json:"created"`
	InputDigest        string       `json:"input_digest,omitempty"`
	StoredDigest       string       `json:"stored_digest,omitempty"`
	InputDigestMatches bool         `json:"input_digest_matches"`
	Intent             IntentDetail `json:"intent"`
}

// IntentListItem is one row of the deterministic intent list projection.
type IntentListItem struct {
	ID          string `json:"id"`
	Alias       string `json:"alias,omitempty"`
	Title       string `json:"title"`
	Disposition string `json:"disposition"`
	CreatedAt   string `json:"created_at"`
}

// IntentListResult is the intent list read model.
type IntentListResult struct {
	ContractVersion    int              `json:"contract_version"`
	DatabaseScope      string           `json:"database_scope,omitempty"`
	DatabasePath       string           `json:"database_path,omitempty"`
	ProjectID          string           `json:"project_id,omitempty"`
	ProjectName        string           `json:"project_name,omitempty"`
	ProjectCurrentPath string           `json:"project_current_path,omitempty"`
	Disposition        string           `json:"disposition,omitempty"`
	Intents            []IntentListItem `json:"intents"`
}

// IntentShowResult is the intent show read model.
type IntentShowResult struct {
	ContractVersion    int          `json:"contract_version"`
	DatabaseScope      string       `json:"database_scope,omitempty"`
	DatabasePath       string       `json:"database_path,omitempty"`
	ProjectID          string       `json:"project_id,omitempty"`
	ProjectName        string       `json:"project_name,omitempty"`
	ProjectCurrentPath string       `json:"project_current_path,omitempty"`
	Query              string       `json:"query"`
	Intent             IntentDetail `json:"intent"`
}

// intentWriteHooks injects failures between transactional stages in tests.
type intentWriteHooks struct {
	afterIntent       func(*sql.Tx) error
	afterSnapshot     func(*sql.Tx) error
	afterDisposition  func(*sql.Tx) error
	afterDeferral     func(*sql.Tx) error
	afterOperation    func(*sql.Tx) error
	afterRelationship func(*sql.Tx) error
	beforeCommit      func(*sql.Tx) error
}

func runIntentWriteHook(hooks *intentWriteHooks, stage string, pick func(*intentWriteHooks) func(*sql.Tx) error, tx *sql.Tx) error {
	if hooks == nil {
		return nil
	}
	hook := pick(hooks)
	if hook == nil {
		return nil
	}
	if err := hook(tx); err != nil {
		return &IntentTransactionError{Stage: stage, Err: err}
	}
	return nil
}

// intentDeferralPacket composes the canonical deferral packet. The format is
// deliberately identical to the legacy journal defer packet so digests remain
// comparable across every entry point that shares intent_operations.
func intentDeferralPacket(body, why, boundary, trigger string) string {
	return fmt.Sprintf("Intent: %s\nWhy: %s\nBoundary: %s\nTrigger: %s", body, why, boundary, trigger)
}

func intentDigest(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

func validateIntentField(field, value string, maxBytes int, allowMultiline bool) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", &IntentValidationError{Field: field, Err: fmt.Errorf("must be nonempty")}
	}
	if len(trimmed) > maxBytes {
		return "", &IntentValidationError{Field: field, Err: fmt.Errorf("exceeds %d bytes", maxBytes)}
	}
	for _, r := range trimmed {
		if !unicode.IsControl(r) {
			continue
		}
		if allowMultiline && (r == '\n' || r == '\t') {
			continue
		}
		return "", &IntentValidationError{Field: field, Err: fmt.Errorf("contains control characters")}
	}
	return trimmed, nil
}

func validateIntentOperationID(value string, required bool) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		if required {
			return "", &IntentValidationError{Field: "operation_id", Err: fmt.Errorf("must be nonempty")}
		}
		return "", nil
	}
	return validateIntentField("operation_id", trimmed, intentOperationMaxBytes, false)
}

// CreateIntent writes one Intent snapshot plus its initial disposition.
func CreateIntent(ctx context.Context, root project.Root, resolver PathResolver, options IntentCreateOptions) (IntentMutationResult, error) {
	store, err := openProjectStoreMutateExisting(ctx, root, resolver)
	if err != nil {
		return IntentMutationResult{}, err
	}
	defer store.Close()
	return store.CreateIntent(ctx, root, options)
}

// CreateIntent writes one Intent snapshot plus its initial disposition in one
// serializable transaction on an open store.
func (s *Store) CreateIntent(ctx context.Context, root project.Root, options IntentCreateOptions) (IntentMutationResult, error) {
	return s.createIntentWithHooks(ctx, root, options, nil)
}

func (s *Store) createIntentWithHooks(ctx context.Context, root project.Root, options IntentCreateOptions, hooks *intentWriteHooks) (IntentMutationResult, error) {
	title, err := validateIntentField("title", options.Title, intentTitleMaxBytes, false)
	if err != nil {
		return IntentMutationResult{}, err
	}
	body, err := validateIntentField("body", options.Body, intentFieldMaxBytes, true)
	if err != nil {
		return IntentMutationResult{}, err
	}
	disposition := strings.TrimSpace(options.Disposition)
	if disposition == "" {
		disposition = "tracked"
	}
	if disposition != "tracked" && disposition != "deferred" {
		return IntentMutationResult{}, &IntentValidationError{Field: "disposition", Err: fmt.Errorf("must be tracked or deferred; resolution happens through intent resolve")}
	}
	var why, boundary, trigger string
	if disposition == "deferred" {
		if why, err = validateIntentField("why", options.Why, intentFieldMaxBytes, false); err != nil {
			return IntentMutationResult{}, err
		}
		if boundary, err = validateIntentField("boundary", options.Boundary, intentFieldMaxBytes, false); err != nil {
			return IntentMutationResult{}, err
		}
		if trigger, err = validateIntentField("trigger", options.Trigger, intentFieldMaxBytes, false); err != nil {
			return IntentMutationResult{}, err
		}
	}
	operationID, err := validateIntentOperationID(options.OperationID, disposition == "deferred")
	if err != nil {
		return IntentMutationResult{}, err
	}
	reason := ""
	if strings.TrimSpace(options.Reason) != "" {
		if reason, err = validateIntentField("reason", options.Reason, intentFieldMaxBytes, false); err != nil {
			return IntentMutationResult{}, err
		}
	}

	var inputDigest string
	if disposition == "deferred" {
		inputDigest = intentDigest(intentDeferralPacket(body, why, boundary, trigger))
	} else {
		inputDigest = intentDigest("intent-create\x00" + title + "\x00" + body)
	}

	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return IntentMutationResult{}, err
	}
	identity, err := s.projectIdentity(ctx, projectID)
	if err != nil {
		return IntentMutationResult{}, err
	}

	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return IntentMutationResult{}, &IntentTransactionError{Stage: "begin", Err: err}
	}
	defer tx.Rollback()

	if operationID != "" {
		established, found, loadErr := loadEstablishedIntentOperationTx(ctx, tx, identity, operationID, inputDigest)
		if loadErr != nil {
			return IntentMutationResult{}, loadErr
		}
		if found {
			if err := tx.Commit(); err != nil {
				return IntentMutationResult{}, &IntentTransactionError{Stage: "commit retry", Err: err}
			}
			return established, nil
		}
		if err := rejectLegacyBoundOperationKeyTx(ctx, tx, projectID, operationID); err != nil {
			return IntentMutationResult{}, err
		}
	}

	now := time.Now().UTC()
	timestamp := now.Format(time.RFC3339Nano)
	alias, err := s.nextIntentAlias(ctx, tx, projectID, title, now)
	if err != nil {
		return IntentMutationResult{}, &IntentTransactionError{Stage: "alias allocation", Err: err}
	}
	intentID := stableMigrationID("intent", projectID, alias)

	if _, err := tx.ExecContext(ctx, `
INSERT INTO intents (id, project_id, created_at) VALUES (?, ?, ?)
`, intentID, projectID, timestamp); err != nil {
		return IntentMutationResult{}, &IntentTransactionError{Stage: "intent", Err: err}
	}
	if err := runIntentWriteHook(hooks, "after intent", func(h *intentWriteHooks) func(*sql.Tx) error { return h.afterIntent }, tx); err != nil {
		return IntentMutationResult{}, err
	}

	snapshotDigest := intentDigest(title + "\x00" + body)
	snapshotID := stableMigrationID("intent-snapshot", projectID, intentID, "1")
	if _, err := tx.ExecContext(ctx, `
INSERT INTO intent_snapshots (id, project_id, intent_id, seq, title, body, content_digest, created_at)
VALUES (?, ?, ?, 1, ?, ?, ?, ?)
`, snapshotID, projectID, intentID, title, body, snapshotDigest, timestamp); err != nil {
		return IntentMutationResult{}, &IntentTransactionError{Stage: "snapshot", Err: err}
	}
	if err := runIntentWriteHook(hooks, "after snapshot", func(h *intentWriteHooks) func(*sql.Tx) error { return h.afterSnapshot }, tx); err != nil {
		return IntentMutationResult{}, err
	}

	var deferral *IntentDeferralDetail
	if disposition == "deferred" {
		deferral = &IntentDeferralDetail{
			ID:             stableMigrationID("intent-deferral", projectID, operationID),
			OperationKey:   operationID,
			Body:           body,
			Why:            why,
			Boundary:       boundary,
			RevisitTrigger: trigger,
			StoredDigest:   inputDigest,
			CreatedAt:      timestamp,
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO intent_deferrals (id, project_id, intent_id, operation_key, body, why, boundary, revisit_trigger, stored_digest, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`, deferral.ID, projectID, intentID, operationID, body, why, boundary, trigger, inputDigest, timestamp); err != nil {
			return IntentMutationResult{}, &IntentTransactionError{Stage: "deferral", Err: err}
		}
		if err := runIntentWriteHook(hooks, "after deferral", func(h *intentWriteHooks) func(*sql.Tx) error { return h.afterDeferral }, tx); err != nil {
			return IntentMutationResult{}, err
		}
	}

	dispositionID := stableMigrationID("intent-disposition", projectID, intentID, "1")
	deferralID := sql.NullString{}
	if deferral != nil {
		deferralID = sql.NullString{String: deferral.ID, Valid: true}
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO intent_dispositions (id, project_id, intent_id, seq, disposition, reason, deferral_id, created_at)
VALUES (?, ?, ?, 1, ?, ?, ?, ?)
`, dispositionID, projectID, intentID, disposition, emptyToNil(reason), deferralID, timestamp); err != nil {
		return IntentMutationResult{}, &IntentTransactionError{Stage: "disposition", Err: err}
	}
	if err := runIntentWriteHook(hooks, "after disposition", func(h *intentWriteHooks) func(*sql.Tx) error { return h.afterDisposition }, tx); err != nil {
		return IntentMutationResult{}, err
	}

	if err := insertAlias(ctx, tx, projectID, "intent", intentID, "intent", alias, timestamp); err != nil {
		return IntentMutationResult{}, &IntentTransactionError{Stage: "alias", Err: err}
	}

	sources, err := writeIntentSourceRelationshipsTx(ctx, tx, projectID, intentID, options.Sources, timestamp)
	if err != nil {
		return IntentMutationResult{}, err
	}
	if err := runIntentWriteHook(hooks, "after relationships", func(h *intentWriteHooks) func(*sql.Tx) error { return h.afterRelationship }, tx); err != nil {
		return IntentMutationResult{}, err
	}

	if operationID != "" {
		if _, err := tx.ExecContext(ctx, `
INSERT INTO intent_operations (project_id, operation_key, intent_id, stored_digest, journal_entry_id, spark_id, projection_version, created_at, updated_at)
VALUES (?, ?, ?, ?, NULL, NULL, 0, ?, ?)
`, projectID, operationID, intentID, inputDigest, timestamp, timestamp); err != nil {
			return IntentMutationResult{}, &IntentTransactionError{Stage: "operation mapping", Err: err}
		}
		if err := runIntentWriteHook(hooks, "after operation", func(h *intentWriteHooks) func(*sql.Tx) error { return h.afterOperation }, tx); err != nil {
			return IntentMutationResult{}, err
		}
	}

	if err := runIntentWriteHook(hooks, "before commit", func(h *intentWriteHooks) func(*sql.Tx) error { return h.beforeCommit }, tx); err != nil {
		return IntentMutationResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return IntentMutationResult{}, &IntentTransactionError{Stage: "commit", Err: err}
	}

	return IntentMutationResult{
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
		Intent: IntentDetail{
			ID:                intentID,
			Alias:             alias,
			Title:             title,
			Body:              body,
			SnapshotSeq:       1,
			ContentDigest:     snapshotDigest,
			Disposition:       disposition,
			DispositionSeq:    1,
			DispositionReason: reason,
			Deferral:          deferral,
			Sources:           sources,
			CreatedAt:         timestamp,
		},
	}, nil
}

// DeferIntent appends an immutable deferral to an existing Intent.
func DeferIntent(ctx context.Context, root project.Root, resolver PathResolver, options IntentDeferOptions) (IntentMutationResult, error) {
	store, err := openProjectStoreMutateExisting(ctx, root, resolver)
	if err != nil {
		return IntentMutationResult{}, err
	}
	defer store.Close()
	return store.DeferIntent(ctx, root, options)
}

// DeferIntent appends an immutable deferral to an existing Intent in one
// serializable transaction on an open store.
func (s *Store) DeferIntent(ctx context.Context, root project.Root, options IntentDeferOptions) (IntentMutationResult, error) {
	return s.deferIntentWithHooks(ctx, root, options, nil)
}

func (s *Store) deferIntentWithHooks(ctx context.Context, root project.Root, options IntentDeferOptions, hooks *intentWriteHooks) (IntentMutationResult, error) {
	why, err := validateIntentField("why", options.Why, intentFieldMaxBytes, false)
	if err != nil {
		return IntentMutationResult{}, err
	}
	boundary, err := validateIntentField("boundary", options.Boundary, intentFieldMaxBytes, false)
	if err != nil {
		return IntentMutationResult{}, err
	}
	trigger, err := validateIntentField("trigger", options.Trigger, intentFieldMaxBytes, false)
	if err != nil {
		return IntentMutationResult{}, err
	}
	operationID, err := validateIntentOperationID(options.OperationID, true)
	if err != nil {
		return IntentMutationResult{}, err
	}

	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return IntentMutationResult{}, err
	}
	identity, err := s.projectIdentity(ctx, projectID)
	if err != nil {
		return IntentMutationResult{}, err
	}

	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return IntentMutationResult{}, &IntentTransactionError{Stage: "begin", Err: err}
	}
	defer tx.Rollback()

	intentID, _, err := resolveIntentRefTx(ctx, tx, projectID, options.IntentRef)
	if err != nil {
		return IntentMutationResult{}, err
	}
	snapshot, err := latestIntentSnapshotTx(ctx, tx, projectID, intentID)
	if err != nil {
		return IntentMutationResult{}, &IntentTransactionError{Stage: "read snapshot", Err: err}
	}
	inputDigest := intentDigest(intentDeferralPacket(snapshot.Body, why, boundary, trigger))

	established, found, loadErr := loadEstablishedIntentOperationTx(ctx, tx, identity, operationID, inputDigest)
	if loadErr != nil {
		return IntentMutationResult{}, loadErr
	}
	if found {
		if established.Intent.ID != intentID {
			return IntentMutationResult{}, &IntentValidationError{Field: "operation_id", Err: fmt.Errorf("operation key %q is already bound to intent %s and cannot defer %s; use a distinct operation key", operationID, established.Intent.ID, intentID)}
		}
		if err := tx.Commit(); err != nil {
			return IntentMutationResult{}, &IntentTransactionError{Stage: "commit retry", Err: err}
		}
		return established, nil
	}
	if err := rejectLegacyBoundOperationKeyTx(ctx, tx, projectID, operationID); err != nil {
		return IntentMutationResult{}, err
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	deferral := IntentDeferralDetail{
		ID:             stableMigrationID("intent-deferral", projectID, operationID),
		OperationKey:   operationID,
		Body:           snapshot.Body,
		Why:            why,
		Boundary:       boundary,
		RevisitTrigger: trigger,
		StoredDigest:   inputDigest,
		CreatedAt:      now,
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO intent_deferrals (id, project_id, intent_id, operation_key, body, why, boundary, revisit_trigger, stored_digest, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`, deferral.ID, projectID, intentID, operationID, deferral.Body, why, boundary, trigger, inputDigest, now); err != nil {
		return IntentMutationResult{}, &IntentTransactionError{Stage: "deferral", Err: err}
	}
	if err := runIntentWriteHook(hooks, "after deferral", func(h *intentWriteHooks) func(*sql.Tx) error { return h.afterDeferral }, tx); err != nil {
		return IntentMutationResult{}, err
	}

	seq, err := nextAggregateSeq(ctx, tx, "intent_dispositions", "intent_id", intentID)
	if err != nil {
		return IntentMutationResult{}, &IntentTransactionError{Stage: "sequence", Err: err}
	}
	dispositionID := stableMigrationID("intent-disposition", projectID, intentID, fmt.Sprintf("%d", seq))
	if _, err := tx.ExecContext(ctx, `
INSERT INTO intent_dispositions (id, project_id, intent_id, seq, disposition, reason, deferral_id, created_at)
VALUES (?, ?, ?, ?, 'deferred', NULL, ?, ?)
`, dispositionID, projectID, intentID, seq, deferral.ID, now); err != nil {
		return IntentMutationResult{}, &IntentTransactionError{Stage: "disposition", Err: err}
	}
	if err := runIntentWriteHook(hooks, "after disposition", func(h *intentWriteHooks) func(*sql.Tx) error { return h.afterDisposition }, tx); err != nil {
		return IntentMutationResult{}, err
	}

	if _, err := tx.ExecContext(ctx, `
INSERT INTO intent_operations (project_id, operation_key, intent_id, stored_digest, journal_entry_id, spark_id, projection_version, created_at, updated_at)
VALUES (?, ?, ?, ?, NULL, NULL, 0, ?, ?)
`, projectID, operationID, intentID, inputDigest, now, now); err != nil {
		return IntentMutationResult{}, &IntentTransactionError{Stage: "operation mapping", Err: err}
	}
	if err := runIntentWriteHook(hooks, "after operation", func(h *intentWriteHooks) func(*sql.Tx) error { return h.afterOperation }, tx); err != nil {
		return IntentMutationResult{}, err
	}
	if err := runIntentWriteHook(hooks, "before commit", func(h *intentWriteHooks) func(*sql.Tx) error { return h.beforeCommit }, tx); err != nil {
		return IntentMutationResult{}, err
	}

	detail, err := loadIntentDetailTx(ctx, tx, projectID, intentID)
	if err != nil {
		return IntentMutationResult{}, &IntentTransactionError{Stage: "read result", Err: err}
	}
	if err := tx.Commit(); err != nil {
		return IntentMutationResult{}, &IntentTransactionError{Stage: "commit", Err: err}
	}
	return IntentMutationResult{
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
		Intent:             detail,
	}, nil
}

// ResumeIntent appends a tracked disposition superseding the current deferral.
func ResumeIntent(ctx context.Context, root project.Root, resolver PathResolver, options IntentDispositionOptions) (IntentMutationResult, error) {
	store, err := openProjectStoreMutateExisting(ctx, root, resolver)
	if err != nil {
		return IntentMutationResult{}, err
	}
	defer store.Close()
	return store.appendIntentDisposition(ctx, root, options, "tracked")
}

// ResolveIntent appends a reasoned terminal disposition.
func ResolveIntent(ctx context.Context, root project.Root, resolver PathResolver, options IntentDispositionOptions) (IntentMutationResult, error) {
	store, err := openProjectStoreMutateExisting(ctx, root, resolver)
	if err != nil {
		return IntentMutationResult{}, err
	}
	defer store.Close()
	return store.appendIntentDisposition(ctx, root, options, "resolved")
}

func (s *Store) appendIntentDisposition(ctx context.Context, root project.Root, options IntentDispositionOptions, disposition string) (IntentMutationResult, error) {
	reason, err := validateIntentField("reason", options.Reason, intentFieldMaxBytes, false)
	if err != nil {
		return IntentMutationResult{}, err
	}
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return IntentMutationResult{}, err
	}
	identity, err := s.projectIdentity(ctx, projectID)
	if err != nil {
		return IntentMutationResult{}, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return IntentMutationResult{}, &IntentTransactionError{Stage: "begin", Err: err}
	}
	defer tx.Rollback()

	intentID, _, err := resolveIntentRefTx(ctx, tx, projectID, options.IntentRef)
	if err != nil {
		return IntentMutationResult{}, err
	}
	current, err := latestIntentDispositionTx(ctx, tx, projectID, intentID)
	if err != nil {
		return IntentMutationResult{}, &IntentTransactionError{Stage: "read disposition", Err: err}
	}

	supersedes := sql.NullString{}
	if disposition == "tracked" {
		if current.Disposition != "deferred" || !current.DeferralID.Valid {
			return IntentMutationResult{}, fmt.Errorf("intent %s is %s, not deferred; resume supersedes an existing deferral", options.IntentRef, current.Disposition)
		}
		supersedes = current.DeferralID
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	seq, err := nextAggregateSeq(ctx, tx, "intent_dispositions", "intent_id", intentID)
	if err != nil {
		return IntentMutationResult{}, &IntentTransactionError{Stage: "sequence", Err: err}
	}
	dispositionID := stableMigrationID("intent-disposition", projectID, intentID, fmt.Sprintf("%d", seq))
	if _, err := tx.ExecContext(ctx, `
INSERT INTO intent_dispositions (id, project_id, intent_id, seq, disposition, reason, deferral_id, supersedes_deferral_id, created_at)
VALUES (?, ?, ?, ?, ?, ?, NULL, ?, ?)
`, dispositionID, projectID, intentID, seq, disposition, reason, supersedes, now); err != nil {
		return IntentMutationResult{}, &IntentTransactionError{Stage: "disposition", Err: err}
	}

	detail, err := loadIntentDetailTx(ctx, tx, projectID, intentID)
	if err != nil {
		return IntentMutationResult{}, &IntentTransactionError{Stage: "read result", Err: err}
	}
	if err := tx.Commit(); err != nil {
		return IntentMutationResult{}, &IntentTransactionError{Stage: "commit", Err: err}
	}
	return IntentMutationResult{
		ContractVersion:    StateJSONContractVersion,
		DatabaseScope:      identity.DatabaseScope,
		DatabasePath:       identity.DatabasePath,
		ProjectID:          identity.ID,
		ProjectName:        identity.FriendlyName,
		ProjectCurrentPath: identity.CurrentPath,
		Created:            true,
		InputDigestMatches: true,
		Intent:             detail,
	}, nil
}

// ShowIntent returns the derived read model for one Intent.
func ShowIntent(ctx context.Context, root project.Root, resolver PathResolver, ref string) (IntentShowResult, error) {
	store, err := openProjectStoreReadExisting(ctx, root, resolver)
	if err != nil {
		return IntentShowResult{}, err
	}
	defer store.Close()
	projectID, err := store.projectID(ctx, root)
	if err != nil {
		return IntentShowResult{}, err
	}
	identity, err := store.projectIdentity(ctx, projectID)
	if err != nil {
		return IntentShowResult{}, err
	}
	tx, err := store.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return IntentShowResult{}, fmt.Errorf("begin intent show: %w", err)
	}
	defer tx.Rollback()
	intentID, _, err := resolveIntentRefTx(ctx, tx, projectID, ref)
	if err != nil {
		return IntentShowResult{}, err
	}
	detail, err := loadIntentDetailTx(ctx, tx, projectID, intentID)
	if err != nil {
		return IntentShowResult{}, err
	}
	return IntentShowResult{
		ContractVersion:    StateJSONContractVersion,
		DatabaseScope:      identity.DatabaseScope,
		DatabasePath:       identity.DatabasePath,
		ProjectID:          identity.ID,
		ProjectName:        identity.FriendlyName,
		ProjectCurrentPath: identity.CurrentPath,
		Query:              ref,
		Intent:             detail,
	}, nil
}

// ListIntents returns the deterministic intent list projection.
func ListIntents(ctx context.Context, root project.Root, resolver PathResolver, dispositionFilter string) (IntentListResult, error) {
	store, err := openProjectStoreReadExisting(ctx, root, resolver)
	if err != nil {
		return IntentListResult{}, err
	}
	defer store.Close()
	projectID, err := store.projectID(ctx, root)
	if err != nil {
		return IntentListResult{}, err
	}
	identity, err := store.projectIdentity(ctx, projectID)
	if err != nil {
		return IntentListResult{}, err
	}
	filter := strings.TrimSpace(dispositionFilter)
	if filter != "" && filter != "tracked" && filter != "deferred" && filter != "resolved" {
		return IntentListResult{}, &IntentValidationError{Field: "disposition", Err: fmt.Errorf("must be tracked, deferred, or resolved")}
	}
	rows, err := store.db.QueryContext(ctx, `
SELECT i.id,
  COALESCE((SELECT alias FROM aliases WHERE project_id = i.project_id AND entity_kind = 'intent' AND entity_id = i.id ORDER BY namespace, alias LIMIT 1), ''),
  COALESCE((SELECT title FROM intent_snapshots WHERE intent_id = i.id ORDER BY seq DESC LIMIT 1), ''),
  COALESCE((SELECT disposition FROM intent_dispositions WHERE intent_id = i.id ORDER BY seq DESC LIMIT 1), ''),
  i.created_at
FROM intents AS i
WHERE i.project_id = ?
ORDER BY i.created_at, i.id
`, projectID)
	if err != nil {
		return IntentListResult{}, fmt.Errorf("list intents: %w", err)
	}
	defer rows.Close()
	items := []IntentListItem{}
	for rows.Next() {
		var item IntentListItem
		if err := rows.Scan(&item.ID, &item.Alias, &item.Title, &item.Disposition, &item.CreatedAt); err != nil {
			return IntentListResult{}, fmt.Errorf("scan intent row: %w", err)
		}
		if filter != "" && item.Disposition != filter {
			continue
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return IntentListResult{}, fmt.Errorf("iterate intents: %w", err)
	}
	return IntentListResult{
		ContractVersion:    StateJSONContractVersion,
		DatabaseScope:      identity.DatabaseScope,
		DatabasePath:       identity.DatabasePath,
		ProjectID:          identity.ID,
		ProjectName:        identity.FriendlyName,
		ProjectCurrentPath: identity.CurrentPath,
		Disposition:        filter,
		Intents:            items,
	}, nil
}

func (s *Store) nextIntentAlias(ctx context.Context, tx *sql.Tx, projectID string, title string, now time.Time) (string, error) {
	slug := normalizeSparkSlug(title)
	if slug == "" {
		slug = "intent"
	}
	prefix := "INTENT-" + now.UTC().Format("20060102") + "-" + slug
	for next := 1; ; next++ {
		alias := prefix
		if next > 1 {
			alias = fmt.Sprintf("%s-%d", prefix, next)
		}
		var existing string
		err := tx.QueryRowContext(ctx, `SELECT id FROM aliases WHERE project_id = ? AND namespace = 'intent' AND alias = ?`, projectID, alias).Scan(&existing)
		if errors.Is(err, sql.ErrNoRows) {
			return alias, nil
		}
		if err != nil {
			return "", fmt.Errorf("probe intent alias %s: %w", alias, err)
		}
	}
}

// resolveIntentRefTx resolves an alias or internal ID to an intent row.
func resolveIntentRefTx(ctx context.Context, tx *sql.Tx, projectID string, ref string) (string, string, error) {
	trimmed := strings.TrimSpace(ref)
	if trimmed == "" {
		return "", "", &IntentValidationError{Field: "intent", Err: fmt.Errorf("must be nonempty")}
	}
	var kind, id, alias string
	err := tx.QueryRowContext(ctx, `
SELECT entity_kind, entity_id, alias FROM aliases
WHERE project_id = ? AND alias = ?
ORDER BY namespace LIMIT 1
`, projectID, trimmed).Scan(&kind, &id, &alias)
	switch {
	case err == nil:
		if kind != "intent" {
			return "", "", fmt.Errorf("%q resolves to %s, not an intent", trimmed, kind)
		}
		return id, alias, nil
	case !errors.Is(err, sql.ErrNoRows):
		return "", "", fmt.Errorf("resolve intent %q: %w", trimmed, err)
	}
	var existing string
	err = tx.QueryRowContext(ctx, `SELECT id FROM intents WHERE project_id = ? AND id = ?`, projectID, trimmed).Scan(&existing)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", fmt.Errorf("intent %q not found in SQLite state", trimmed)
	}
	if err != nil {
		return "", "", fmt.Errorf("resolve intent %q: %w", trimmed, err)
	}
	return existing, "", nil
}

type intentSnapshotRow struct {
	Seq           int
	Title         string
	Body          string
	ContentDigest string
}

func latestIntentSnapshotTx(ctx context.Context, tx *sql.Tx, projectID, intentID string) (intentSnapshotRow, error) {
	var snapshot intentSnapshotRow
	err := tx.QueryRowContext(ctx, `
SELECT seq, title, body, content_digest FROM intent_snapshots
WHERE project_id = ? AND intent_id = ?
ORDER BY seq DESC LIMIT 1
`, projectID, intentID).Scan(&snapshot.Seq, &snapshot.Title, &snapshot.Body, &snapshot.ContentDigest)
	if errors.Is(err, sql.ErrNoRows) {
		return intentSnapshotRow{}, fmt.Errorf("intent %s has no snapshot", intentID)
	}
	if err != nil {
		return intentSnapshotRow{}, err
	}
	return snapshot, nil
}

type intentDispositionRow struct {
	Seq         int
	Disposition string
	Reason      sql.NullString
	DeferralID  sql.NullString
}

func latestIntentDispositionTx(ctx context.Context, tx *sql.Tx, projectID, intentID string) (intentDispositionRow, error) {
	var row intentDispositionRow
	err := tx.QueryRowContext(ctx, `
SELECT seq, disposition, reason, deferral_id FROM intent_dispositions
WHERE project_id = ? AND intent_id = ?
ORDER BY seq DESC LIMIT 1
`, projectID, intentID).Scan(&row.Seq, &row.Disposition, &row.Reason, &row.DeferralID)
	if errors.Is(err, sql.ErrNoRows) {
		return intentDispositionRow{}, fmt.Errorf("intent %s has no disposition", intentID)
	}
	if err != nil {
		return intentDispositionRow{}, err
	}
	return row, nil
}

func loadIntentDeferralTx(ctx context.Context, tx *sql.Tx, projectID, deferralID string) (*IntentDeferralDetail, error) {
	deferral := &IntentDeferralDetail{}
	err := tx.QueryRowContext(ctx, `
SELECT id, operation_key, body, why, boundary, revisit_trigger, stored_digest, created_at
FROM intent_deferrals WHERE project_id = ? AND id = ?
`, projectID, deferralID).Scan(&deferral.ID, &deferral.OperationKey, &deferral.Body, &deferral.Why, &deferral.Boundary, &deferral.RevisitTrigger, &deferral.StoredDigest, &deferral.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("intent deferral %s not found", deferralID)
	}
	if err != nil {
		return nil, err
	}
	return deferral, nil
}

// loadIntentDetailTx builds the derived read model inside a transaction.
func loadIntentDetailTx(ctx context.Context, tx *sql.Tx, projectID, intentID string) (IntentDetail, error) {
	var createdAt string
	err := tx.QueryRowContext(ctx, `SELECT created_at FROM intents WHERE project_id = ? AND id = ?`, projectID, intentID).Scan(&createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return IntentDetail{}, fmt.Errorf("intent %s not found", intentID)
	}
	if err != nil {
		return IntentDetail{}, err
	}
	snapshot, err := latestIntentSnapshotTx(ctx, tx, projectID, intentID)
	if err != nil {
		return IntentDetail{}, err
	}
	disposition, err := latestIntentDispositionTx(ctx, tx, projectID, intentID)
	if err != nil {
		return IntentDetail{}, err
	}
	var deferral *IntentDeferralDetail
	if disposition.Disposition == "deferred" && disposition.DeferralID.Valid {
		deferral, err = loadIntentDeferralTx(ctx, tx, projectID, disposition.DeferralID.String)
		if err != nil {
			return IntentDetail{}, err
		}
	}
	var alias string
	if err := tx.QueryRowContext(ctx, `
SELECT alias FROM aliases WHERE project_id = ? AND entity_kind = 'intent' AND entity_id = ?
ORDER BY namespace, alias LIMIT 1
`, projectID, intentID).Scan(&alias); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return IntentDetail{}, err
	}
	sources, err := loadIntentSourcesTx(ctx, tx, projectID, intentID)
	if err != nil {
		return IntentDetail{}, err
	}
	return IntentDetail{
		ID:                intentID,
		Alias:             alias,
		Title:             snapshot.Title,
		Body:              snapshot.Body,
		SnapshotSeq:       snapshot.Seq,
		ContentDigest:     snapshot.ContentDigest,
		Disposition:       disposition.Disposition,
		DispositionSeq:    disposition.Seq,
		DispositionReason: disposition.Reason.String,
		Deferral:          deferral,
		Sources:           sources,
		CreatedAt:         createdAt,
	}, nil
}

func loadIntentSourcesTx(ctx context.Context, tx *sql.Tx, projectID, intentID string) ([]TraceRelationship, error) {
	rows, err := tx.QueryContext(ctx, `
SELECT relationship_type, from_entity_kind, from_entity_id, COALESCE(reason, '')
FROM relationships
WHERE project_id = ? AND to_entity_kind = 'intent' AND to_entity_id = ? AND relationship_type = 'source-of'
ORDER BY from_entity_kind, from_entity_id
`, projectID, intentID)
	if err != nil {
		return nil, fmt.Errorf("read intent sources: %w", err)
	}
	defer rows.Close()
	sources := []TraceRelationship{}
	for rows.Next() {
		var relationshipType, kind, id, reason string
		if err := rows.Scan(&relationshipType, &kind, &id, &reason); err != nil {
			return nil, fmt.Errorf("scan intent source: %w", err)
		}
		sources = append(sources, TraceRelationship{
			Direction: "inbound",
			Type:      relationshipType,
			Entity:    TraceEntity{Kind: kind, ID: id},
			Reason:    reason,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate intent sources: %w", err)
	}
	return sources, nil
}

// writeIntentSourceRelationshipsTx validates each source against the closed
// relationship matrix and records the source-of edges.
func writeIntentSourceRelationshipsTx(ctx context.Context, tx *sql.Tx, projectID, intentID string, refs []string, now string) ([]TraceRelationship, error) {
	sources := []TraceRelationship{}
	seen := map[string]bool{}
	for _, ref := range refs {
		trimmed := strings.TrimSpace(ref)
		if trimmed == "" {
			continue
		}
		entity, err := resolveSourceEntityTx(ctx, tx, projectID, trimmed)
		if err != nil {
			return nil, &IntentTransactionError{Stage: "resolve source", Err: err}
		}
		if err := validateRelationshipAgainstRegistry(entity.Kind, "source-of", "intent"); err != nil {
			return nil, &IntentValidationError{Field: "from", Err: err}
		}
		key := entity.Kind + "\x00" + entity.ID
		if seen[key] {
			return nil, &IntentValidationError{Field: "from", Err: fmt.Errorf("source %q is duplicated", trimmed)}
		}
		seen[key] = true
		relationshipID := stableMigrationID("relationship", projectID, entity.Kind, entity.ID, "source-of", "intent", intentID)
		if _, err := tx.ExecContext(ctx, `
INSERT INTO relationships (id, project_id, from_entity_kind, from_entity_id, to_entity_kind, to_entity_id, relationship_type, reason, origin, created_at, updated_at)
VALUES (?, ?, ?, ?, 'intent', ?, 'source-of', 'recorded by intent create', ?, ?, ?)
`, relationshipID, projectID, entity.Kind, entity.ID, intentID, relationshipOriginCommand, now, now); err != nil {
			return nil, &IntentTransactionError{Stage: "relationship", Err: err}
		}
		sources = append(sources, TraceRelationship{Direction: "inbound", Type: "source-of", Entity: entity, Reason: "recorded by intent create"})
	}
	return sources, nil
}

// resolveSourceEntityTx resolves a source ref by alias then internal ID within
// the write transaction so validation and write observe one snapshot.
func resolveSourceEntityTx(ctx context.Context, tx *sql.Tx, projectID, ref string) (TraceEntity, error) {
	var kind, id, alias string
	err := tx.QueryRowContext(ctx, `
SELECT entity_kind, entity_id, alias FROM aliases
WHERE project_id = ? AND alias = ?
ORDER BY namespace LIMIT 1
`, projectID, ref).Scan(&kind, &id, &alias)
	switch {
	case err == nil:
		return TraceEntity{Kind: kind, ID: id, Alias: alias}, nil
	case !errors.Is(err, sql.ErrNoRows):
		return TraceEntity{}, fmt.Errorf("resolve source %q: %w", ref, err)
	}
	for _, kind := range internalIDResolvableKinds() {
		table := traceTable(kind)
		var id string
		err := tx.QueryRowContext(ctx, fmt.Sprintf(`SELECT id FROM %s WHERE project_id = ? AND id = ?`, table), projectID, ref).Scan(&id)
		if err == nil {
			return TraceEntity{Kind: kind, ID: id}, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return TraceEntity{}, fmt.Errorf("resolve source %q: %w", ref, err)
		}
	}
	return TraceEntity{}, fmt.Errorf("source %q not found in SQLite state", ref)
}

// rejectLegacyBoundOperationKeyTx refuses to mint a new canonical mapping for
// an operation key that a pre-conversion legacy deferral already owns. Without
// this guard a tracked create could capture the key, hide the legacy deferral
// from continuity, and permanently block its conversion.
func rejectLegacyBoundOperationKeyTx(ctx context.Context, tx *sql.Tx, projectID, operationID string) error {
	var journalID string
	err := tx.QueryRowContext(ctx, `
SELECT journal_entry_id FROM journal_deferrals WHERE project_id = ? AND operation_key = ?
`, projectID, operationID).Scan(&journalID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return &IntentTransactionError{Stage: "lookup legacy operation key", Err: err}
	}
	return &IntentValidationError{Field: "operation_id", Err: fmt.Errorf("operation key %q belongs to a pre-conversion legacy deferral (decision %s); convert it with `loaf state migrate deferrals` or choose a different key", operationID, journalID)}
}

// loadEstablishedIntentOperationTx returns the first-written result for an
// operation key when the mapping already exists.
func loadEstablishedIntentOperationTx(ctx context.Context, tx *sql.Tx, identity ProjectIdentity, operationID, inputDigest string) (IntentMutationResult, bool, error) {
	var intentID, storedDigest string
	err := tx.QueryRowContext(ctx, `
SELECT intent_id, stored_digest FROM intent_operations
WHERE project_id = ? AND operation_key = ?
`, identity.ID, operationID).Scan(&intentID, &storedDigest)
	if errors.Is(err, sql.ErrNoRows) {
		return IntentMutationResult{}, false, nil
	}
	if err != nil {
		return IntentMutationResult{}, false, &IntentTransactionError{Stage: "lookup operation key", Err: err}
	}
	detail, err := loadIntentDetailTx(ctx, tx, identity.ID, intentID)
	if err != nil {
		return IntentMutationResult{}, false, &IntentTransactionError{Stage: "load established intent", Err: err}
	}
	return IntentMutationResult{
		ContractVersion:    StateJSONContractVersion,
		DatabaseScope:      identity.DatabaseScope,
		DatabasePath:       identity.DatabasePath,
		ProjectID:          identity.ID,
		ProjectName:        identity.FriendlyName,
		ProjectCurrentPath: identity.CurrentPath,
		OperationID:        operationID,
		Created:            false,
		InputDigest:        inputDigest,
		StoredDigest:       storedDigest,
		InputDigestMatches: inputDigest == storedDigest,
		Intent:             detail,
	}, true, nil
}
