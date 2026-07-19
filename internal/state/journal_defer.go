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
	journalDeferFieldMaxLength = 4096
	journalDeferOperationMax   = 200
	journalDeferScopePrefixLen = 16
)

// JournalDeferOptions describes a self-sufficient deferred-intent packet.
type JournalDeferOptions struct {
	Intent      string
	Why         string
	Boundary    string
	Trigger     string
	OperationID string
	Origin      *JournalOriginInput
}

// JournalDeferResult describes the reciprocal journal decision and spark
// created by one operation key. A retry returns the original pair with Created
// false and reports whether its packet digest matched the first write. Since
// the Intent model became canonical this command is a compatibility adapter:
// IntentID/IntentAlias identify the canonical Intent behind the operation key,
// and the decision/spark pair is a labeled legacy projection.
type JournalDeferResult struct {
	ContractVersion    int                `json:"contract_version,omitempty"`
	DatabaseScope      string             `json:"database_scope,omitempty"`
	DatabasePath       string             `json:"database_path,omitempty"`
	ProjectID          string             `json:"project_id,omitempty"`
	ProjectName        string             `json:"project_name,omitempty"`
	ProjectCurrentPath string             `json:"project_current_path,omitempty"`
	OperationID        string             `json:"operation_id"`
	Created            bool               `json:"created"`
	Decision           JournalEntryRecord `json:"decision"`
	Spark              SparkDetail        `json:"spark"`
	InputDigest        string             `json:"input_digest"`
	StoredDigest       string             `json:"stored_digest"`
	InputDigestMatches bool               `json:"input_digest_matches"`
	IntentID           string             `json:"intent_id,omitempty"`
	IntentAlias        string             `json:"intent_alias,omitempty"`
	Origin             *JournalOrigin     `json:"origin,omitempty"`
}

// JournalDeferValidationError identifies malformed semantic packet input.
type JournalDeferValidationError struct {
	Field string
	Err   error
}

func (e *JournalDeferValidationError) Error() string {
	if e == nil {
		return "journal defer validation failed"
	}
	return fmt.Sprintf("journal defer validation failed for %s: %v", e.Field, e.Err)
}

func (e *JournalDeferValidationError) Unwrap() error { return e.Err }

// JournalDeferTransactionError identifies the transactional stage that failed.
type JournalDeferTransactionError struct {
	Stage string
	Err   error
}

func (e *JournalDeferTransactionError) Error() string {
	if e == nil {
		return "journal defer transaction failed"
	}
	return fmt.Sprintf("journal defer transaction failed at %s: %v", e.Stage, e.Err)
}

func (e *JournalDeferTransactionError) Unwrap() error { return e.Err }

// DeferJournal writes a deferred-intent packet to initialized SQLite state.
func DeferJournal(ctx context.Context, root project.Root, resolver PathResolver, options JournalDeferOptions) (JournalDeferResult, error) {
	store, err := openProjectStoreMutateExisting(ctx, root, resolver)
	if err != nil {
		return JournalDeferResult{}, err
	}
	defer store.Close()
	return store.DeferJournal(ctx, root, options)
}

// DeferJournal writes one journal decision and one open spark in a single
// serializable transaction. The project must already be registered.
func (s *Store) DeferJournal(ctx context.Context, root project.Root, options JournalDeferOptions) (JournalDeferResult, error) {
	return s.deferJournalWithHooks(ctx, root, options, nil)
}

type journalDeferHooks struct {
	afterDecision        func(*sql.Tx) error
	afterFTS             func(*sql.Tx) error
	afterSpark           func(*sql.Tx) error
	afterAliasEvent      func(*sql.Tx) error
	afterOrigin          func(*sql.Tx) error
	afterDeferral        func(*sql.Tx) error
	afterCanonicalIntent func(*sql.Tx) error
	beforeCommit         func(*sql.Tx) error
}

func (s *Store) deferJournalWithHooks(ctx context.Context, root project.Root, options JournalDeferOptions, hooks *journalDeferHooks) (JournalDeferResult, error) {
	normalized, packet, inputDigest, err := normalizeJournalDeferOptions(options)
	if err != nil {
		return JournalDeferResult{}, err
	}
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return JournalDeferResult{}, err
	}
	identity, err := s.projectIdentity(ctx, projectID)
	if err != nil {
		return JournalDeferResult{}, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return JournalDeferResult{}, &JournalDeferTransactionError{Stage: "begin", Err: err}
	}
	defer tx.Rollback()

	var existingJournalID, existingSparkID, storedDigest string
	err = tx.QueryRowContext(ctx, `
SELECT journal_entry_id, spark_id, stored_digest
FROM journal_deferrals
WHERE project_id = ? AND operation_key = ?
`, projectID, normalized.OperationID).Scan(&existingJournalID, &existingSparkID, &storedDigest)
	switch {
	case err == nil:
		result, loadErr := loadExistingJournalDeferralTx(ctx, tx, identity, normalized.OperationID, inputDigest, existingJournalID, existingSparkID, storedDigest)
		if loadErr != nil {
			return JournalDeferResult{}, &JournalDeferTransactionError{Stage: "load existing deferral", Err: loadErr}
		}
		if err := tx.Commit(); err != nil {
			return JournalDeferResult{}, &JournalDeferTransactionError{Stage: "commit retry", Err: err}
		}
		return result, nil
	case !errors.Is(err, sql.ErrNoRows):
		return JournalDeferResult{}, &JournalDeferTransactionError{Stage: "lookup operation key", Err: err}
	}

	// Canonical-first recovery: a canonical intent command may have won this
	// operation key without legacy projections (projection version 0). The
	// adapter then materializes both projections from the STORED canonical
	// packet — never from the retry body — and advances the mapping to
	// version 1 in this same transaction.
	var mappedIntentID, mappedDigest string
	var mappedVersion int
	err = tx.QueryRowContext(ctx, `
SELECT intent_id, stored_digest, projection_version
FROM intent_operations
WHERE project_id = ? AND operation_key = ?
`, projectID, normalized.OperationID).Scan(&mappedIntentID, &mappedDigest, &mappedVersion)
	switch {
	case err == nil:
		result, backfillErr := s.backfillJournalDeferProjectionsTx(ctx, tx, identity, normalized, inputDigest, mappedIntentID, mappedDigest, mappedVersion, hooks)
		if backfillErr != nil {
			return JournalDeferResult{}, backfillErr
		}
		if err := tx.Commit(); err != nil {
			return JournalDeferResult{}, &JournalDeferTransactionError{Stage: "commit backfill", Err: err}
		}
		return result, nil
	case !errors.Is(err, sql.ErrNoRows):
		return JournalDeferResult{}, &JournalDeferTransactionError{Stage: "lookup canonical operation", Err: err}
	}

	operationDigest := journalDeferOperationDigest(projectID, normalized.OperationID)
	decisionID := stableMigrationID("journal-defer-decision", projectID, normalized.OperationID)
	sparkID := stableMigrationID("journal-defer-spark", projectID, normalized.OperationID)
	alias := "SPARK-DEFER-" + operationDigest[:journalDeferScopePrefixLen]
	scope := "defer/" + operationDigest[:journalDeferScopePrefixLen]
	nowTime := time.Now().UTC()
	now := nowTime.Format(time.RFC3339Nano)
	decisionMessage := packet + "\nSpark: " + sparkID
	sparkText := packet + "\nDecision: " + decisionID

	if _, err := tx.ExecContext(ctx, `
INSERT INTO journal_entries (
  id, project_id, entry_type, scope, message,
  observed_branch, observed_worktree, harness_session_id,
  spec_id, task_id, created_at, updated_at
) VALUES (?, ?, 'decision', ?, ?, NULL, NULL, NULL, NULL, NULL, ?, ?)
`, decisionID, projectID, scope, decisionMessage, now, now); err != nil {
		return JournalDeferResult{}, &JournalDeferTransactionError{Stage: "decision", Err: err}
	}
	if err := runJournalDeferHook(hooks, "after decision", func(h *journalDeferHooks) func(*sql.Tx) error { return h.afterDecision }, tx); err != nil {
		return JournalDeferResult{}, err
	}
	if err := insertJournalSearchTx(ctx, tx, projectID, decisionID, "", "decision", scope, decisionMessage); err != nil {
		return JournalDeferResult{}, &JournalDeferTransactionError{Stage: "fts", Err: err}
	}
	if err := runJournalDeferHook(hooks, "after FTS", func(h *journalDeferHooks) func(*sql.Tx) error { return h.afterFTS }, tx); err != nil {
		return JournalDeferResult{}, err
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO sparks (id, project_id, scope, status, text, source_id, created_at, updated_at)
VALUES (?, ?, ?, 'open', ?, NULL, ?, ?)
`, sparkID, projectID, scope, sparkText, now, now); err != nil {
		return JournalDeferResult{}, &JournalDeferTransactionError{Stage: "spark", Err: err}
	}
	if err := runJournalDeferHook(hooks, "after spark", func(h *journalDeferHooks) func(*sql.Tx) error { return h.afterSpark }, tx); err != nil {
		return JournalDeferResult{}, err
	}
	if err := insertAlias(ctx, tx, projectID, "spark", sparkID, "spark", alias, now); err != nil {
		return JournalDeferResult{}, &JournalDeferTransactionError{Stage: "alias", Err: err}
	}
	eventID := stableMigrationID("event", projectID, "spark", sparkID, "created", "open")
	if _, err := tx.ExecContext(ctx, `
INSERT INTO events (id, project_id, entity_kind, entity_id, event_type, from_status, to_status, note, created_at, updated_at)
VALUES (?, ?, 'spark', ?, 'status_changed', NULL, 'open', 'recorded by journal defer', ?, ?)
`, eventID, projectID, sparkID, now, now); err != nil {
		return JournalDeferResult{}, &JournalDeferTransactionError{Stage: "event", Err: err}
	}
	if err := runJournalDeferHook(hooks, "after alias/event", func(h *journalDeferHooks) func(*sql.Tx) error { return h.afterAliasEvent }, tx); err != nil {
		return JournalDeferResult{}, err
	}
	if normalized.Origin != nil {
		if err := insertJournalOriginTx(ctx, tx, decisionID, *normalized.Origin); err != nil {
			return JournalDeferResult{}, &JournalDeferTransactionError{Stage: "origin", Err: err}
		}
	}
	if err := runJournalDeferHook(hooks, "after origin", func(h *journalDeferHooks) func(*sql.Tx) error { return h.afterOrigin }, tx); err != nil {
		return JournalDeferResult{}, err
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO journal_deferrals (project_id, operation_key, journal_entry_id, spark_id, stored_digest, created_at)
VALUES (?, ?, ?, ?, ?, ?)
`, projectID, normalized.OperationID, decisionID, sparkID, inputDigest, now); err != nil {
		return JournalDeferResult{}, &JournalDeferTransactionError{Stage: "deferral", Err: err}
	}
	if err := runJournalDeferHook(hooks, "after deferral", func(h *journalDeferHooks) func(*sql.Tx) error { return h.afterDeferral }, tx); err != nil {
		return JournalDeferResult{}, err
	}
	// The adapter can no longer create a legacy-only deferral: every write
	// also records the canonical Intent, its immutable deferral payload, its
	// deferred disposition, and the shared operation mapping at projection
	// version 1 in this same transaction.
	intentID, intentAlias, err := s.insertCanonicalDeferredIntentTx(ctx, tx, projectID, normalized, inputDigest, decisionID, sparkID, nowTime)
	if err != nil {
		return JournalDeferResult{}, err
	}
	if err := runJournalDeferHook(hooks, "after canonical intent", func(h *journalDeferHooks) func(*sql.Tx) error { return h.afterCanonicalIntent }, tx); err != nil {
		return JournalDeferResult{}, err
	}
	if err := verifyJournalDeferralTx(ctx, tx, projectID, normalized.OperationID, decisionID, sparkID); err != nil {
		return JournalDeferResult{}, &JournalDeferTransactionError{Stage: "verify", Err: err}
	}
	if err := verifyIntentOperationProjectionTx(ctx, tx, projectID, normalized.OperationID, intentID, decisionID, sparkID); err != nil {
		return JournalDeferResult{}, &JournalDeferTransactionError{Stage: "verify", Err: err}
	}
	if err := runJournalDeferHook(hooks, "before commit", func(h *journalDeferHooks) func(*sql.Tx) error { return h.beforeCommit }, tx); err != nil {
		return JournalDeferResult{}, err
	}
	origin, err := loadJournalOrigin(ctx, tx, projectID, decisionID)
	if err != nil {
		return JournalDeferResult{}, &JournalDeferTransactionError{Stage: "read origin", Err: err}
	}
	result := buildJournalDeferResult(identity, normalized.OperationID, true, inputDigest, inputDigest, true, decisionID, projectID, scope, decisionMessage, sparkID, alias, sparkText, now, origin)
	result.IntentID = intentID
	result.IntentAlias = intentAlias
	if err := tx.Commit(); err != nil {
		return JournalDeferResult{}, &JournalDeferTransactionError{Stage: "commit", Err: err}
	}
	return result, nil
}

// journalDeferIntentTitle derives a bounded single-line canonical title from
// the legacy Intent field, cutting on a rune boundary.
func journalDeferIntentTitle(intentText string) string {
	title := strings.TrimSpace(intentText)
	if len(title) <= intentTitleMaxBytes {
		return title
	}
	cut := intentTitleMaxBytes
	for cut > 0 && (title[cut]&0xC0) == 0x80 {
		cut--
	}
	return strings.TrimSpace(title[:cut])
}

// insertCanonicalDeferredIntentTx writes the canonical Intent rows for one
// adapter-first journal defer: identity, first snapshot, immutable deferral
// payload, deferred disposition, intent alias, and the shared operation
// mapping at projection version 1 carrying the legacy projection IDs.
func (s *Store) insertCanonicalDeferredIntentTx(ctx context.Context, tx *sql.Tx, projectID string, normalized JournalDeferOptions, inputDigest, decisionID, sparkID string, nowTime time.Time) (string, string, error) {
	timestamp := nowTime.Format(time.RFC3339Nano)
	title := journalDeferIntentTitle(normalized.Intent)
	intentAlias, err := s.nextIntentAlias(ctx, tx, projectID, title, nowTime)
	if err != nil {
		return "", "", &JournalDeferTransactionError{Stage: "canonical alias", Err: err}
	}
	intentID := stableMigrationID("intent", projectID, intentAlias)
	if _, err := tx.ExecContext(ctx, `
INSERT INTO intents (id, project_id, created_at) VALUES (?, ?, ?)
`, intentID, projectID, timestamp); err != nil {
		return "", "", &JournalDeferTransactionError{Stage: "canonical intent", Err: err}
	}
	snapshotID := stableMigrationID("intent-snapshot", projectID, intentID, "1")
	if _, err := tx.ExecContext(ctx, `
INSERT INTO intent_snapshots (id, project_id, intent_id, seq, title, body, content_digest, created_at)
VALUES (?, ?, ?, 1, ?, ?, ?, ?)
`, snapshotID, projectID, intentID, title, normalized.Intent, intentDigest(title+"\x00"+normalized.Intent), timestamp); err != nil {
		return "", "", &JournalDeferTransactionError{Stage: "canonical snapshot", Err: err}
	}
	deferralID := stableMigrationID("intent-deferral", projectID, normalized.OperationID)
	if _, err := tx.ExecContext(ctx, `
INSERT INTO intent_deferrals (id, project_id, intent_id, operation_key, body, why, boundary, revisit_trigger, stored_digest, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`, deferralID, projectID, intentID, normalized.OperationID, normalized.Intent, normalized.Why, normalized.Boundary, normalized.Trigger, inputDigest, timestamp); err != nil {
		return "", "", &JournalDeferTransactionError{Stage: "canonical deferral", Err: err}
	}
	dispositionID := stableMigrationID("intent-disposition", projectID, intentID, "1")
	if _, err := tx.ExecContext(ctx, `
INSERT INTO intent_dispositions (id, project_id, intent_id, seq, disposition, reason, deferral_id, created_at)
VALUES (?, ?, ?, 1, 'deferred', NULL, ?, ?)
`, dispositionID, projectID, intentID, deferralID, timestamp); err != nil {
		return "", "", &JournalDeferTransactionError{Stage: "canonical disposition", Err: err}
	}
	if err := insertAlias(ctx, tx, projectID, "intent", intentID, "intent", intentAlias, timestamp); err != nil {
		return "", "", &JournalDeferTransactionError{Stage: "canonical intent alias", Err: err}
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO intent_operations (project_id, operation_key, intent_id, stored_digest, journal_entry_id, spark_id, projection_version, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, 1, ?, ?)
`, projectID, normalized.OperationID, intentID, inputDigest, decisionID, sparkID, timestamp, timestamp); err != nil {
		return "", "", &JournalDeferTransactionError{Stage: "canonical operation", Err: err}
	}
	return intentID, intentAlias, nil
}

// verifyIntentOperationProjectionTx confirms the shared mapping row carries
// exactly this intent and legacy projection pair at version 1.
func verifyIntentOperationProjectionTx(ctx context.Context, tx *sql.Tx, projectID, operationID, intentID, decisionID, sparkID string) error {
	var count int
	if err := tx.QueryRowContext(ctx, `
SELECT COUNT(*) FROM intent_operations
WHERE project_id = ? AND operation_key = ? AND intent_id = ?
  AND journal_entry_id = ? AND spark_id = ? AND projection_version = 1
`, projectID, operationID, intentID, decisionID, sparkID).Scan(&count); err != nil {
		return fmt.Errorf("verify canonical operation mapping: %w", err)
	}
	if count != 1 {
		return fmt.Errorf("verify canonical operation mapping: expected one version-1 row, got %d", count)
	}
	return nil
}

// backfillJournalDeferProjectionsTx materializes the legacy decision/spark
// pair for an operation key that a canonical intent command won first. Content
// comes from the stored canonical deferral packet, never the retry input.
func (s *Store) backfillJournalDeferProjectionsTx(ctx context.Context, tx *sql.Tx, identity ProjectIdentity, normalized JournalDeferOptions, inputDigest, intentID, storedDigest string, mappedVersion int, hooks *journalDeferHooks) (JournalDeferResult, error) {
	projectID := identity.ID
	if mappedVersion == 1 {
		// Both projections already exist (for example through conversion);
		// return the established pair without writing anything.
		var journalID, sparkID string
		if err := tx.QueryRowContext(ctx, `
SELECT journal_entry_id, spark_id FROM intent_operations
WHERE project_id = ? AND operation_key = ?
`, projectID, normalized.OperationID).Scan(&journalID, &sparkID); err != nil {
			return JournalDeferResult{}, &JournalDeferTransactionError{Stage: "load projected operation", Err: err}
		}
		result, err := loadExistingJournalDeferralTx(ctx, tx, identity, normalized.OperationID, inputDigest, journalID, sparkID, storedDigest)
		if err != nil {
			return JournalDeferResult{}, &JournalDeferTransactionError{Stage: "load projected operation", Err: err}
		}
		return result, nil
	}

	var body, why, boundary, trigger string
	err := tx.QueryRowContext(ctx, `
SELECT body, why, boundary, revisit_trigger FROM intent_deferrals
WHERE project_id = ? AND operation_key = ?
`, projectID, normalized.OperationID).Scan(&body, &why, &boundary, &trigger)
	if errors.Is(err, sql.ErrNoRows) {
		return JournalDeferResult{}, &JournalDeferValidationError{Field: "operation_id", Err: fmt.Errorf("operation key %q is already bound to intent %s without a deferral (its disposition is not deferred); use a distinct operation key or defer that intent explicitly", normalized.OperationID, intentID)}
	}
	if err != nil {
		return JournalDeferResult{}, &JournalDeferTransactionError{Stage: "read canonical packet", Err: err}
	}
	storedPacket := intentDeferralPacket(body, why, boundary, trigger)

	operationDigest := journalDeferOperationDigest(projectID, normalized.OperationID)
	decisionID := stableMigrationID("journal-defer-decision", projectID, normalized.OperationID)
	sparkID := stableMigrationID("journal-defer-spark", projectID, normalized.OperationID)
	alias := "SPARK-DEFER-" + operationDigest[:journalDeferScopePrefixLen]
	scope := "defer/" + operationDigest[:journalDeferScopePrefixLen]
	now := time.Now().UTC().Format(time.RFC3339Nano)
	decisionMessage := storedPacket + "\nSpark: " + sparkID
	sparkText := storedPacket + "\nDecision: " + decisionID

	if _, err := tx.ExecContext(ctx, `
INSERT INTO journal_entries (
  id, project_id, entry_type, scope, message,
  observed_branch, observed_worktree, harness_session_id,
  spec_id, task_id, created_at, updated_at
) VALUES (?, ?, 'decision', ?, ?, NULL, NULL, NULL, NULL, NULL, ?, ?)
`, decisionID, projectID, scope, decisionMessage, now, now); err != nil {
		return JournalDeferResult{}, &JournalDeferTransactionError{Stage: "decision", Err: err}
	}
	if err := runJournalDeferHook(hooks, "after decision", func(h *journalDeferHooks) func(*sql.Tx) error { return h.afterDecision }, tx); err != nil {
		return JournalDeferResult{}, err
	}
	if err := insertJournalSearchTx(ctx, tx, projectID, decisionID, "", "decision", scope, decisionMessage); err != nil {
		return JournalDeferResult{}, &JournalDeferTransactionError{Stage: "fts", Err: err}
	}
	if err := runJournalDeferHook(hooks, "after FTS", func(h *journalDeferHooks) func(*sql.Tx) error { return h.afterFTS }, tx); err != nil {
		return JournalDeferResult{}, err
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO sparks (id, project_id, scope, status, text, source_id, created_at, updated_at)
VALUES (?, ?, ?, 'open', ?, NULL, ?, ?)
`, sparkID, projectID, scope, sparkText, now, now); err != nil {
		return JournalDeferResult{}, &JournalDeferTransactionError{Stage: "spark", Err: err}
	}
	if err := runJournalDeferHook(hooks, "after spark", func(h *journalDeferHooks) func(*sql.Tx) error { return h.afterSpark }, tx); err != nil {
		return JournalDeferResult{}, err
	}
	if err := insertAlias(ctx, tx, projectID, "spark", sparkID, "spark", alias, now); err != nil {
		return JournalDeferResult{}, &JournalDeferTransactionError{Stage: "alias", Err: err}
	}
	eventID := stableMigrationID("event", projectID, "spark", sparkID, "created", "open")
	if _, err := tx.ExecContext(ctx, `
INSERT INTO events (id, project_id, entity_kind, entity_id, event_type, from_status, to_status, note, created_at, updated_at)
VALUES (?, ?, 'spark', ?, 'status_changed', NULL, 'open', 'recorded by journal defer', ?, ?)
`, eventID, projectID, sparkID, now, now); err != nil {
		return JournalDeferResult{}, &JournalDeferTransactionError{Stage: "event", Err: err}
	}
	if err := runJournalDeferHook(hooks, "after alias/event", func(h *journalDeferHooks) func(*sql.Tx) error { return h.afterAliasEvent }, tx); err != nil {
		return JournalDeferResult{}, err
	}
	if normalized.Origin != nil {
		if err := insertJournalOriginTx(ctx, tx, decisionID, *normalized.Origin); err != nil {
			return JournalDeferResult{}, &JournalDeferTransactionError{Stage: "origin", Err: err}
		}
	}
	if err := runJournalDeferHook(hooks, "after origin", func(h *journalDeferHooks) func(*sql.Tx) error { return h.afterOrigin }, tx); err != nil {
		return JournalDeferResult{}, err
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO journal_deferrals (project_id, operation_key, journal_entry_id, spark_id, stored_digest, created_at)
VALUES (?, ?, ?, ?, ?, ?)
`, projectID, normalized.OperationID, decisionID, sparkID, storedDigest, now); err != nil {
		return JournalDeferResult{}, &JournalDeferTransactionError{Stage: "deferral", Err: err}
	}
	if err := runJournalDeferHook(hooks, "after deferral", func(h *journalDeferHooks) func(*sql.Tx) error { return h.afterDeferral }, tx); err != nil {
		return JournalDeferResult{}, err
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE intent_operations
SET journal_entry_id = ?, spark_id = ?, projection_version = 1, updated_at = ?
WHERE project_id = ? AND operation_key = ? AND projection_version = 0
`, decisionID, sparkID, now, projectID, normalized.OperationID); err != nil {
		return JournalDeferResult{}, &JournalDeferTransactionError{Stage: "advance projection", Err: err}
	}
	if err := runJournalDeferHook(hooks, "after canonical intent", func(h *journalDeferHooks) func(*sql.Tx) error { return h.afterCanonicalIntent }, tx); err != nil {
		return JournalDeferResult{}, err
	}
	if err := verifyJournalDeferralTx(ctx, tx, projectID, normalized.OperationID, decisionID, sparkID); err != nil {
		return JournalDeferResult{}, &JournalDeferTransactionError{Stage: "verify", Err: err}
	}
	if err := verifyIntentOperationProjectionTx(ctx, tx, projectID, normalized.OperationID, intentID, decisionID, sparkID); err != nil {
		return JournalDeferResult{}, &JournalDeferTransactionError{Stage: "verify", Err: err}
	}
	if err := runJournalDeferHook(hooks, "before commit", func(h *journalDeferHooks) func(*sql.Tx) error { return h.beforeCommit }, tx); err != nil {
		return JournalDeferResult{}, err
	}
	origin, err := loadJournalOrigin(ctx, tx, projectID, decisionID)
	if err != nil {
		return JournalDeferResult{}, &JournalDeferTransactionError{Stage: "read origin", Err: err}
	}
	result := buildJournalDeferResult(identity, normalized.OperationID, false, inputDigest, storedDigest, inputDigest == storedDigest, decisionID, projectID, scope, decisionMessage, sparkID, alias, sparkText, now, origin)
	result.IntentID = intentID
	if alias, aliasErr := intentAliasTx(ctx, tx, projectID, intentID); aliasErr == nil {
		result.IntentAlias = alias
	}
	return result, nil
}

func intentAliasTx(ctx context.Context, tx *sql.Tx, projectID, intentID string) (string, error) {
	var alias string
	err := tx.QueryRowContext(ctx, `
SELECT alias FROM aliases WHERE project_id = ? AND entity_kind = 'intent' AND entity_id = ?
ORDER BY namespace, alias LIMIT 1
`, projectID, intentID).Scan(&alias)
	return alias, err
}

func journalDeferOperationDigest(projectID, operationID string) string {
	digest := sha256.Sum256([]byte(projectID + "\x00" + operationID))
	return hex.EncodeToString(digest[:])
}

func runJournalDeferHook(hooks *journalDeferHooks, stage string, callback func(*journalDeferHooks) func(*sql.Tx) error, tx *sql.Tx) error {
	if hooks == nil {
		return nil
	}
	hook := callback(hooks)
	if hook == nil {
		return nil
	}
	if err := hook(tx); err != nil {
		return &JournalDeferTransactionError{Stage: stage, Err: err}
	}
	return nil
}

func normalizeJournalDeferOptions(options JournalDeferOptions) (JournalDeferOptions, string, string, error) {
	normalized := options
	for name, value := range map[string]*string{
		"intent":   &normalized.Intent,
		"why":      &normalized.Why,
		"boundary": &normalized.Boundary,
		"trigger":  &normalized.Trigger,
	} {
		trimmed := strings.TrimSpace(*value)
		if trimmed == "" {
			return JournalDeferOptions{}, "", "", &JournalDeferValidationError{Field: name, Err: fmt.Errorf("must be nonempty")}
		}
		if len(trimmed) > journalDeferFieldMaxLength {
			return JournalDeferOptions{}, "", "", &JournalDeferValidationError{Field: name, Err: fmt.Errorf("exceeds %d characters", journalDeferFieldMaxLength)}
		}
		for _, r := range trimmed {
			if unicode.IsControl(r) {
				return JournalDeferOptions{}, "", "", &JournalDeferValidationError{Field: name, Err: fmt.Errorf("contains control characters")}
			}
		}
		*value = trimmed
	}
	operationID := strings.TrimSpace(normalized.OperationID)
	if operationID == "" {
		return JournalDeferOptions{}, "", "", &JournalDeferValidationError{Field: "operation_id", Err: fmt.Errorf("must be nonempty")}
	}
	if len(operationID) > journalDeferOperationMax {
		return JournalDeferOptions{}, "", "", &JournalDeferValidationError{Field: "operation_id", Err: fmt.Errorf("exceeds %d characters", journalDeferOperationMax)}
	}
	for _, r := range operationID {
		if unicode.IsControl(r) {
			return JournalDeferOptions{}, "", "", &JournalDeferValidationError{Field: "operation_id", Err: fmt.Errorf("contains control characters")}
		}
	}
	normalized.OperationID = operationID
	if normalized.Origin != nil {
		origin, err := normalizeJournalOriginInput(*normalized.Origin)
		if err != nil {
			return JournalDeferOptions{}, "", "", &JournalDeferValidationError{Field: "origin", Err: err}
		}
		normalized.Origin = &origin
	}
	packet := fmt.Sprintf("Intent: %s\nWhy: %s\nBoundary: %s\nTrigger: %s", normalized.Intent, normalized.Why, normalized.Boundary, normalized.Trigger)
	digest := sha256.Sum256([]byte(packet))
	return normalized, packet, hex.EncodeToString(digest[:]), nil
}

func loadExistingJournalDeferralTx(ctx context.Context, tx *sql.Tx, identity ProjectIdentity, operationID, inputDigest, journalID, sparkID, storedDigest string) (JournalDeferResult, error) {
	decision, err := loadDeferredDecisionTx(ctx, tx, identity.ID, journalID)
	if err != nil {
		return JournalDeferResult{}, err
	}
	alias, err := loadDeferredSparkAliasTx(ctx, tx, identity.ID, sparkID)
	if err != nil {
		return JournalDeferResult{}, err
	}
	spark, err := loadDeferredSparkTx(ctx, tx, identity.ID, sparkID, alias)
	if err != nil {
		return JournalDeferResult{}, err
	}
	origin, err := loadJournalOrigin(ctx, tx, identity.ID, journalID)
	if err != nil {
		return JournalDeferResult{}, err
	}
	result := JournalDeferResult{
		ContractVersion:    StateJSONContractVersion,
		DatabaseScope:      identity.DatabaseScope,
		DatabasePath:       identity.DatabasePath,
		ProjectID:          identity.ID,
		ProjectName:        identity.FriendlyName,
		ProjectCurrentPath: identity.CurrentPath,
		OperationID:        operationID,
		Created:            false,
		Decision:           decision,
		Spark:              spark,
		InputDigest:        inputDigest,
		StoredDigest:       storedDigest,
		InputDigestMatches: inputDigest == storedDigest,
		Origin:             origin,
	}
	// Attach the canonical mapping when the operation key is already bound to
	// an Intent; legacy-only rows awaiting conversion carry no intent fields.
	var mappedIntentID string
	err = tx.QueryRowContext(ctx, `
SELECT intent_id FROM intent_operations WHERE project_id = ? AND operation_key = ?
`, identity.ID, operationID).Scan(&mappedIntentID)
	switch {
	case err == nil:
		result.IntentID = mappedIntentID
		if alias, aliasErr := intentAliasTx(ctx, tx, identity.ID, mappedIntentID); aliasErr == nil {
			result.IntentAlias = alias
		}
	case !errors.Is(err, sql.ErrNoRows):
		return JournalDeferResult{}, fmt.Errorf("read canonical mapping for %s: %w", operationID, err)
	}
	return result, nil
}

func loadDeferredDecisionTx(ctx context.Context, tx *sql.Tx, projectID, journalID string) (JournalEntryRecord, error) {
	var decision JournalEntryRecord
	err := tx.QueryRowContext(ctx, `
SELECT id, entry_type, COALESCE(scope, ''), message,
  COALESCE(observed_branch, ''), COALESCE(observed_worktree, ''), COALESCE(harness_session_id, ''), created_at
FROM journal_entries
WHERE project_id = ? AND id = ?
`, projectID, journalID).Scan(&decision.ID, &decision.EntryType, &decision.Scope, &decision.Message, &decision.ObservedBranch, &decision.ObservedWorktree, &decision.HarnessSessionID, &decision.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return JournalEntryRecord{}, fmt.Errorf("deferred decision %q not found", journalID)
	}
	if err != nil {
		return JournalEntryRecord{}, fmt.Errorf("read deferred decision %s: %w", journalID, err)
	}
	return decision, nil
}

func loadDeferredSparkAliasTx(ctx context.Context, tx *sql.Tx, projectID, sparkID string) (string, error) {
	var alias string
	err := tx.QueryRowContext(ctx, `SELECT alias FROM aliases WHERE project_id = ? AND entity_kind = 'spark' AND entity_id = ? AND namespace = 'spark' ORDER BY alias LIMIT 1`, projectID, sparkID).Scan(&alias)
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("deferred spark %q alias not found", sparkID)
	}
	if err != nil {
		return "", fmt.Errorf("read deferred spark alias %s: %w", sparkID, err)
	}
	return alias, nil
}

func loadDeferredSparkTx(ctx context.Context, tx *sql.Tx, projectID, sparkID, alias string) (SparkDetail, error) {
	var spark SparkDetail
	err := tx.QueryRowContext(ctx, `
SELECT id, text, COALESCE(scope, ''), status, created_at, updated_at
FROM sparks
WHERE project_id = ? AND id = ?
`, projectID, sparkID).Scan(&spark.ID, &spark.Text, &spark.Scope, &spark.Status, &spark.CreatedAt, &spark.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return SparkDetail{}, fmt.Errorf("deferred spark %q not found", sparkID)
	}
	if err != nil {
		return SparkDetail{}, fmt.Errorf("read deferred spark %s: %w", sparkID, err)
	}
	spark.Status = LifecycleStatusForDisplay(LifecycleEntitySpark, spark.Status)
	spark.Alias = alias
	spark.Sources = []TraceSource{}
	spark.Relationships = []TraceRelationship{}
	return spark, nil
}

func verifyJournalDeferralTx(ctx context.Context, tx *sql.Tx, projectID, operationID, journalID, sparkID string) error {
	var count int
	if err := tx.QueryRowContext(ctx, `
SELECT COUNT(*) FROM journal_deferrals AS d
JOIN journal_entries AS j ON j.project_id = d.project_id AND j.id = d.journal_entry_id
JOIN sparks AS s ON s.project_id = d.project_id AND s.id = d.spark_id
WHERE d.project_id = ? AND d.operation_key = ? AND d.journal_entry_id = ? AND d.spark_id = ?
`, projectID, operationID, journalID, sparkID).Scan(&count); err != nil {
		return fmt.Errorf("verify deferred endpoints: %w", err)
	}
	if count != 1 {
		return fmt.Errorf("verify deferred endpoints: expected one reciprocal row, got %d", count)
	}
	return nil
}

func buildJournalDeferResult(identity ProjectIdentity, operationID string, created bool, inputDigest, storedDigest string, matches bool, decisionID, projectID, scope, decisionMessage, sparkID, alias, sparkText, now string, origin *JournalOrigin) JournalDeferResult {
	return JournalDeferResult{
		ContractVersion:    StateJSONContractVersion,
		DatabaseScope:      identity.DatabaseScope,
		DatabasePath:       identity.DatabasePath,
		ProjectID:          identity.ID,
		ProjectName:        identity.FriendlyName,
		ProjectCurrentPath: identity.CurrentPath,
		OperationID:        operationID,
		Created:            created,
		Decision: JournalEntryRecord{
			ID:        decisionID,
			EntryType: "decision",
			Scope:     scope,
			Message:   decisionMessage,
			CreatedAt: now,
		},
		Spark: SparkDetail{
			ID:            sparkID,
			Alias:         alias,
			Text:          sparkText,
			Scope:         scope,
			Status:        "open",
			Sources:       []TraceSource{},
			Relationships: []TraceRelationship{},
			CreatedAt:     now,
			UpdatedAt:     now,
		},
		InputDigest:        inputDigest,
		StoredDigest:       storedDigest,
		InputDigestMatches: matches,
		Origin:             origin,
	}
}
