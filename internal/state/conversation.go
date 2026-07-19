package state

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/levifig/loaf/internal/project"
)

const (
	conversationTitleMaxBytes      = 200
	conversationFieldMaxBytes      = 1024
	explorationContextDefaultLimit = 10
	explorationContextMaxLimit     = 100
)

// ConversationCreateOptions describes a new logical conversation.
type ConversationCreateOptions struct {
	Title       string
	OperationID string
}

// ConversationHandleAddOptions attaches one machine-local handle, with an
// optional bounded log reference, to a logical conversation.
type ConversationHandleAddOptions struct {
	ConversationRef string
	Harness         string
	Handle          string
	Locality        string
	LogRef          string
	Hash            string
	Range           string
}

// ConversationObserveOptions appends one immutable availability observation.
type ConversationObserveOptions struct {
	SubjectKind string
	SubjectID   string
	Available   bool
	Observer    string
	Locality    string
	Note        string
}

// ConversationHandleDetail is one machine-local handle read model.
type ConversationHandleDetail struct {
	ID       string                         `json:"id"`
	Harness  string                         `json:"harness"`
	Handle   string                         `json:"handle"`
	Locality string                         `json:"locality,omitempty"`
	LogRefs  []ConversationLogRefDetail     `json:"log_refs,omitempty"`
	Latest   *SourceAvailabilityObservation `json:"latest_observation,omitempty"`
}

// ConversationLogRefDetail is one bounded log locator read model.
type ConversationLogRefDetail struct {
	ID      string                         `json:"id"`
	Locator string                         `json:"locator"`
	Hash    string                         `json:"hash,omitempty"`
	Range   string                         `json:"range,omitempty"`
	Latest  *SourceAvailabilityObservation `json:"latest_observation,omitempty"`
}

// SourceAvailabilityObservation is one immutable availability fact.
type SourceAvailabilityObservation struct {
	ObservedAt string `json:"observed_at"`
	Observer   string `json:"observer,omitempty"`
	Locality   string `json:"locality,omitempty"`
	Available  bool   `json:"available"`
	Note       string `json:"note,omitempty"`
}

// ConversationDetail is the logical conversation read model.
type ConversationDetail struct {
	ID        string                     `json:"id"`
	Title     string                     `json:"title"`
	Handles   []ConversationHandleDetail `json:"handles"`
	CreatedAt string                     `json:"created_at"`
}

// ConversationMutationResult is the shared write envelope.
type ConversationMutationResult struct {
	ContractVersion    int                `json:"contract_version"`
	DatabaseScope      string             `json:"database_scope,omitempty"`
	DatabasePath       string             `json:"database_path,omitempty"`
	ProjectID          string             `json:"project_id,omitempty"`
	ProjectName        string             `json:"project_name,omitempty"`
	ProjectCurrentPath string             `json:"project_current_path,omitempty"`
	OperationID        string             `json:"operation_id,omitempty"`
	Created            bool               `json:"created"`
	Conversation       ConversationDetail `json:"conversation"`
	HandleID           string             `json:"handle_id,omitempty"`
	LogRefID           string             `json:"log_ref_id,omitempty"`
	ExplorationID      string             `json:"exploration_id,omitempty"`
	ObservationID      string             `json:"observation_id,omitempty"`
}

func validateConversationField(field, value string, maxBytes int, required bool) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		if required {
			return "", fmt.Errorf("conversation %s must be nonempty", field)
		}
		return "", nil
	}
	if len(trimmed) > maxBytes {
		return "", fmt.Errorf("conversation %s exceeds %d bytes", field, maxBytes)
	}
	return trimmed, nil
}

// CreateConversation writes one logical conversation identity.
func CreateConversation(ctx context.Context, root project.Root, resolver PathResolver, options ConversationCreateOptions) (ConversationMutationResult, error) {
	store, err := openProjectStoreMutateExisting(ctx, root, resolver)
	if err != nil {
		return ConversationMutationResult{}, err
	}
	defer store.Close()
	return store.CreateConversation(ctx, root, options)
}

// CreateConversation writes one logical conversation on an open store.
func (s *Store) CreateConversation(ctx context.Context, root project.Root, options ConversationCreateOptions) (ConversationMutationResult, error) {
	title, err := validateConversationField("title", options.Title, conversationTitleMaxBytes, true)
	if err != nil {
		return ConversationMutationResult{}, err
	}
	operationID, err := validateConversationField("operation id", options.OperationID, intentOperationMaxBytes, false)
	if err != nil {
		return ConversationMutationResult{}, err
	}
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return ConversationMutationResult{}, err
	}
	identity, err := s.projectIdentity(ctx, projectID)
	if err != nil {
		return ConversationMutationResult{}, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return ConversationMutationResult{}, fmt.Errorf("begin conversation create: %w", err)
	}
	defer tx.Rollback()

	if operationID != "" {
		var existingID string
		err := tx.QueryRowContext(ctx, `
SELECT id FROM logical_conversations WHERE project_id = ? AND operation_key = ?
`, projectID, operationID).Scan(&existingID)
		switch {
		case err == nil:
			detail, loadErr := loadConversationDetailTx(ctx, tx, projectID, existingID)
			if loadErr != nil {
				return ConversationMutationResult{}, loadErr
			}
			if err := tx.Commit(); err != nil {
				return ConversationMutationResult{}, fmt.Errorf("commit conversation retry: %w", err)
			}
			return conversationMutationResult(identity, operationID, false, detail), nil
		case !errors.Is(err, sql.ErrNoRows):
			return ConversationMutationResult{}, fmt.Errorf("lookup conversation operation key: %w", err)
		}
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	conversationID := ""
	if operationID != "" {
		conversationID = stableMigrationID("logical-conversation", projectID, operationID)
	} else {
		conversationID = stableMigrationID("logical-conversation", projectID, title, now)
	}
	operationValue := sql.NullString{}
	if operationID != "" {
		operationValue = sql.NullString{String: operationID, Valid: true}
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO logical_conversations (id, project_id, title, operation_key, created_at)
VALUES (?, ?, ?, ?, ?)
`, conversationID, projectID, title, operationValue, now); err != nil {
		return ConversationMutationResult{}, fmt.Errorf("insert logical conversation: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return ConversationMutationResult{}, fmt.Errorf("commit conversation create: %w", err)
	}
	return conversationMutationResult(identity, operationID, true, ConversationDetail{
		ID:        conversationID,
		Title:     title,
		Handles:   []ConversationHandleDetail{},
		CreatedAt: now,
	}), nil
}

// AddConversationHandle attaches a machine-local handle and optional log ref.
func AddConversationHandle(ctx context.Context, root project.Root, resolver PathResolver, options ConversationHandleAddOptions) (ConversationMutationResult, error) {
	store, err := openProjectStoreMutateExisting(ctx, root, resolver)
	if err != nil {
		return ConversationMutationResult{}, err
	}
	defer store.Close()
	return store.AddConversationHandle(ctx, root, options)
}

// AddConversationHandle attaches a handle on an open store. Re-adding an
// identical handle is idempotent and returns the existing row.
func (s *Store) AddConversationHandle(ctx context.Context, root project.Root, options ConversationHandleAddOptions) (ConversationMutationResult, error) {
	harness, err := validateConversationField("harness", options.Harness, conversationFieldMaxBytes, true)
	if err != nil {
		return ConversationMutationResult{}, err
	}
	handle, err := validateConversationField("handle", options.Handle, conversationFieldMaxBytes, true)
	if err != nil {
		return ConversationMutationResult{}, err
	}
	locality, err := validateConversationField("locality", options.Locality, conversationFieldMaxBytes, false)
	if err != nil {
		return ConversationMutationResult{}, err
	}
	logRef, err := validateConversationField("log ref", options.LogRef, conversationFieldMaxBytes, false)
	if err != nil {
		return ConversationMutationResult{}, err
	}
	rangeSpec, err := validateConversationField("range", options.Range, conversationFieldMaxBytes, false)
	if err != nil {
		return ConversationMutationResult{}, err
	}
	hash := strings.TrimSpace(options.Hash)
	if hash != "" && (len(hash) != 64 || strings.ContainsFunc(hash, func(r rune) bool {
		return !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F'))
	})) {
		return ConversationMutationResult{}, fmt.Errorf("conversation log hash must be 64 hex characters")
	}
	if (hash != "" || rangeSpec != "") && logRef == "" {
		return ConversationMutationResult{}, fmt.Errorf("conversation --hash and --range require --log-ref")
	}

	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return ConversationMutationResult{}, err
	}
	identity, err := s.projectIdentity(ctx, projectID)
	if err != nil {
		return ConversationMutationResult{}, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return ConversationMutationResult{}, fmt.Errorf("begin handle add: %w", err)
	}
	defer tx.Rollback()

	conversationID, err := resolveConversationRefTx(ctx, tx, projectID, options.ConversationRef)
	if err != nil {
		return ConversationMutationResult{}, err
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	created := false
	var handleID string
	err = tx.QueryRowContext(ctx, `
SELECT id FROM conversation_handles
WHERE conversation_id = ? AND harness = ? AND handle = ? AND COALESCE(locality, '') = ?
`, conversationID, harness, handle, locality).Scan(&handleID)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		handleID = stableMigrationID("conversation-handle", projectID, conversationID, harness, handle, locality)
		localityValue := sql.NullString{}
		if locality != "" {
			localityValue = sql.NullString{String: locality, Valid: true}
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO conversation_handles (id, project_id, conversation_id, harness, handle, locality, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
`, handleID, projectID, conversationID, harness, handle, localityValue, now); err != nil {
			return ConversationMutationResult{}, fmt.Errorf("insert conversation handle: %w", err)
		}
		created = true
	case err != nil:
		return ConversationMutationResult{}, fmt.Errorf("probe conversation handle: %w", err)
	}

	logRefID := ""
	if logRef != "" {
		err = tx.QueryRowContext(ctx, `
SELECT id FROM conversation_log_refs
WHERE handle_id = ? AND locator = ? AND COALESCE(range_spec, '') = ?
`, handleID, logRef, rangeSpec).Scan(&logRefID)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			logRefID = stableMigrationID("conversation-log-ref", projectID, handleID, logRef, rangeSpec)
			hashValue := sql.NullString{}
			if hash != "" {
				hashValue = sql.NullString{String: strings.ToLower(hash), Valid: true}
			}
			rangeValue := sql.NullString{}
			if rangeSpec != "" {
				rangeValue = sql.NullString{String: rangeSpec, Valid: true}
			}
			if _, err := tx.ExecContext(ctx, `
INSERT INTO conversation_log_refs (id, project_id, handle_id, locator, content_hash, range_spec, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
`, logRefID, projectID, handleID, logRef, hashValue, rangeValue, now); err != nil {
				return ConversationMutationResult{}, fmt.Errorf("insert conversation log ref: %w", err)
			}
			created = true
		case err != nil:
			return ConversationMutationResult{}, fmt.Errorf("probe conversation log ref: %w", err)
		}
	}

	detail, err := loadConversationDetailTx(ctx, tx, projectID, conversationID)
	if err != nil {
		return ConversationMutationResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return ConversationMutationResult{}, fmt.Errorf("commit handle add: %w", err)
	}
	result := conversationMutationResult(identity, "", created, detail)
	result.HandleID = handleID
	result.LogRefID = logRefID
	return result, nil
}

// AddExplorationConversation records exploration membership explicitly.
func AddExplorationConversation(ctx context.Context, root project.Root, resolver PathResolver, explorationRef, conversationRef string) (ConversationMutationResult, error) {
	store, err := openProjectStoreMutateExisting(ctx, root, resolver)
	if err != nil {
		return ConversationMutationResult{}, err
	}
	defer store.Close()
	return store.AddExplorationConversation(ctx, root, explorationRef, conversationRef)
}

// AddExplorationConversation records membership on an open store.
func (s *Store) AddExplorationConversation(ctx context.Context, root project.Root, explorationRef, conversationRef string) (ConversationMutationResult, error) {
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return ConversationMutationResult{}, err
	}
	identity, err := s.projectIdentity(ctx, projectID)
	if err != nil {
		return ConversationMutationResult{}, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return ConversationMutationResult{}, fmt.Errorf("begin exploration conversation add: %w", err)
	}
	defer tx.Rollback()

	explorationID, _, err := resolveExplorationRefTx(ctx, tx, projectID, explorationRef)
	if err != nil {
		return ConversationMutationResult{}, err
	}
	conversationID, err := resolveConversationRefTx(ctx, tx, projectID, conversationRef)
	if err != nil {
		return ConversationMutationResult{}, err
	}
	if err := validateRelationshipAgainstRegistry("exploration", "has-conversation", "logical_conversation"); err != nil {
		return ConversationMutationResult{}, err
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	created := false
	membershipID := stableMigrationID("exploration-conversation", projectID, explorationID, conversationID)
	var existing string
	err = tx.QueryRowContext(ctx, `
SELECT id FROM exploration_conversations WHERE exploration_id = ? AND conversation_id = ?
`, explorationID, conversationID).Scan(&existing)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		if _, err := tx.ExecContext(ctx, `
INSERT INTO exploration_conversations (id, project_id, exploration_id, conversation_id, created_at)
VALUES (?, ?, ?, ?, ?)
`, membershipID, projectID, explorationID, conversationID, now); err != nil {
			return ConversationMutationResult{}, fmt.Errorf("insert exploration conversation: %w", err)
		}
		created = true
	case err != nil:
		return ConversationMutationResult{}, fmt.Errorf("probe exploration conversation: %w", err)
	}

	detail, err := loadConversationDetailTx(ctx, tx, projectID, conversationID)
	if err != nil {
		return ConversationMutationResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return ConversationMutationResult{}, fmt.Errorf("commit exploration conversation add: %w", err)
	}
	result := conversationMutationResult(identity, "", created, detail)
	result.ExplorationID = explorationID
	return result, nil
}

// ObserveConversationSource appends one immutable availability observation for
// a conversation handle or log ref; it never mutates the observed row.
func ObserveConversationSource(ctx context.Context, root project.Root, resolver PathResolver, options ConversationObserveOptions) (ConversationMutationResult, error) {
	store, err := openProjectStoreMutateExisting(ctx, root, resolver)
	if err != nil {
		return ConversationMutationResult{}, err
	}
	defer store.Close()
	return store.ObserveConversationSource(ctx, root, options)
}

// ObserveConversationSource appends an observation on an open store.
func (s *Store) ObserveConversationSource(ctx context.Context, root project.Root, options ConversationObserveOptions) (ConversationMutationResult, error) {
	subjectKind := strings.TrimSpace(options.SubjectKind)
	if subjectKind != "conversation_handle" && subjectKind != "conversation_log_ref" {
		return ConversationMutationResult{}, fmt.Errorf("observation subject kind must be conversation_handle or conversation_log_ref")
	}
	subjectID := strings.TrimSpace(options.SubjectID)
	if subjectID == "" {
		return ConversationMutationResult{}, fmt.Errorf("observation subject id must be nonempty")
	}
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return ConversationMutationResult{}, err
	}
	identity, err := s.projectIdentity(ctx, projectID)
	if err != nil {
		return ConversationMutationResult{}, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return ConversationMutationResult{}, fmt.Errorf("begin observation: %w", err)
	}
	defer tx.Rollback()

	subjectTable := "conversation_handles"
	if subjectKind == "conversation_log_ref" {
		subjectTable = "conversation_log_refs"
	}
	var conversationID string
	query := `SELECT conversation_id FROM conversation_handles WHERE project_id = ? AND id = ?`
	if subjectKind == "conversation_log_ref" {
		query = `SELECT h.conversation_id FROM conversation_log_refs AS r JOIN conversation_handles AS h ON h.id = r.handle_id WHERE r.project_id = ? AND r.id = ?`
	}
	if err := tx.QueryRowContext(ctx, query, projectID, subjectID).Scan(&conversationID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ConversationMutationResult{}, fmt.Errorf("%s %q not found in this project", subjectKind, subjectID)
		}
		return ConversationMutationResult{}, fmt.Errorf("resolve observation subject in %s: %w", subjectTable, err)
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	observationID := stableMigrationID("source-availability", projectID, subjectKind, subjectID, now)
	available := 0
	if options.Available {
		available = 1
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO source_availability_observations (id, project_id, subject_kind, subject_id, observed_at, observer, locality, available, note, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`, observationID, projectID, subjectKind, subjectID, now, emptyToNil(strings.TrimSpace(options.Observer)), emptyToNil(strings.TrimSpace(options.Locality)), available, emptyToNil(strings.TrimSpace(options.Note)), now); err != nil {
		return ConversationMutationResult{}, fmt.Errorf("insert availability observation: %w", err)
	}
	detail, err := loadConversationDetailTx(ctx, tx, projectID, conversationID)
	if err != nil {
		return ConversationMutationResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return ConversationMutationResult{}, fmt.Errorf("commit observation: %w", err)
	}
	result := conversationMutationResult(identity, "", true, detail)
	result.ObservationID = observationID
	return result, nil
}

func conversationMutationResult(identity ProjectIdentity, operationID string, created bool, detail ConversationDetail) ConversationMutationResult {
	return ConversationMutationResult{
		ContractVersion:    StateJSONContractVersion,
		DatabaseScope:      identity.DatabaseScope,
		DatabasePath:       identity.DatabasePath,
		ProjectID:          identity.ID,
		ProjectName:        identity.FriendlyName,
		ProjectCurrentPath: identity.CurrentPath,
		OperationID:        operationID,
		Created:            created,
		Conversation:       detail,
	}
}

func resolveConversationRefTx(ctx context.Context, tx *sql.Tx, projectID, ref string) (string, error) {
	trimmed := strings.TrimSpace(ref)
	if trimmed == "" {
		return "", fmt.Errorf("conversation reference must be nonempty")
	}
	var existing string
	err := tx.QueryRowContext(ctx, `SELECT id FROM logical_conversations WHERE project_id = ? AND id = ?`, projectID, trimmed).Scan(&existing)
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("logical conversation %q not found in SQLite state", trimmed)
	}
	if err != nil {
		return "", fmt.Errorf("resolve conversation %q: %w", trimmed, err)
	}
	return existing, nil
}

func loadConversationDetailTx(ctx context.Context, tx *sql.Tx, projectID, conversationID string) (ConversationDetail, error) {
	detail := ConversationDetail{ID: conversationID, Handles: []ConversationHandleDetail{}}
	err := tx.QueryRowContext(ctx, `
SELECT title, created_at FROM logical_conversations WHERE project_id = ? AND id = ?
`, projectID, conversationID).Scan(&detail.Title, &detail.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return ConversationDetail{}, fmt.Errorf("logical conversation %s not found", conversationID)
	}
	if err != nil {
		return ConversationDetail{}, err
	}
	rows, err := tx.QueryContext(ctx, `
SELECT id, harness, handle, COALESCE(locality, '')
FROM conversation_handles WHERE conversation_id = ?
ORDER BY harness, handle, COALESCE(locality, '')
`, conversationID)
	if err != nil {
		return ConversationDetail{}, fmt.Errorf("read conversation handles: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var handle ConversationHandleDetail
		if err := rows.Scan(&handle.ID, &handle.Harness, &handle.Handle, &handle.Locality); err != nil {
			return ConversationDetail{}, fmt.Errorf("scan conversation handle: %w", err)
		}
		detail.Handles = append(detail.Handles, handle)
	}
	if err := rows.Err(); err != nil {
		return ConversationDetail{}, err
	}
	for index := range detail.Handles {
		handle := &detail.Handles[index]
		if latest, err := latestObservationTx(ctx, tx, projectID, "conversation_handle", handle.ID); err != nil {
			return ConversationDetail{}, err
		} else if latest != nil {
			handle.Latest = latest
		}
		logRows, err := tx.QueryContext(ctx, `
SELECT id, locator, COALESCE(content_hash, ''), COALESCE(range_spec, '')
FROM conversation_log_refs WHERE handle_id = ?
ORDER BY locator, COALESCE(range_spec, '')
`, handle.ID)
		if err != nil {
			return ConversationDetail{}, fmt.Errorf("read conversation log refs: %w", err)
		}
		for logRows.Next() {
			var logRef ConversationLogRefDetail
			if err := logRows.Scan(&logRef.ID, &logRef.Locator, &logRef.Hash, &logRef.Range); err != nil {
				logRows.Close()
				return ConversationDetail{}, fmt.Errorf("scan conversation log ref: %w", err)
			}
			handle.LogRefs = append(handle.LogRefs, logRef)
		}
		if err := logRows.Close(); err != nil {
			return ConversationDetail{}, err
		}
		for logIndex := range handle.LogRefs {
			logRef := &handle.LogRefs[logIndex]
			if latest, err := latestObservationTx(ctx, tx, projectID, "conversation_log_ref", logRef.ID); err != nil {
				return ConversationDetail{}, err
			} else if latest != nil {
				logRef.Latest = latest
			}
		}
	}
	return detail, nil
}

func latestObservationTx(ctx context.Context, tx *sql.Tx, projectID, subjectKind, subjectID string) (*SourceAvailabilityObservation, error) {
	observation := &SourceAvailabilityObservation{}
	var available int
	var observer, locality, note sql.NullString
	err := tx.QueryRowContext(ctx, `
SELECT observed_at, observer, locality, available, note
FROM source_availability_observations
WHERE project_id = ? AND subject_kind = ? AND subject_id = ?
ORDER BY observed_at DESC, id DESC LIMIT 1
`, projectID, subjectKind, subjectID).Scan(&observation.ObservedAt, &observer, &locality, &available, &note)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read latest observation: %w", err)
	}
	observation.Observer = observer.String
	observation.Locality = locality.String
	observation.Available = available == 1
	observation.Note = note.String
	return observation, nil
}

// --- Exploration context projection ---

// ExplorationContextLayer is one bounded, cursor-expandable optional layer.
type ExplorationContextLayer struct {
	Available     int             `json:"available_count"`
	Shown         int             `json:"shown_count"`
	Truncated     bool            `json:"truncated"`
	Cursor        string          `json:"cursor,omitempty"`
	ExpandCommand string          `json:"expand_command,omitempty"`
	Items         json.RawMessage `json:"items"`
}

// ExplorationContextResult is the portable context projection: the four-field
// core is always returned whole, every optional layer is bounded.
type ExplorationContextResult struct {
	ContractVersion        int                                `json:"contract_version"`
	DatabaseScope          string                             `json:"database_scope,omitempty"`
	DatabasePath           string                             `json:"database_path,omitempty"`
	ProjectID              string                             `json:"project_id,omitempty"`
	ProjectName            string                             `json:"project_name,omitempty"`
	ProjectCurrentPath     string                             `json:"project_current_path,omitempty"`
	Query                  string                             `json:"query"`
	Exploration            ExplorationDetail                  `json:"exploration"`
	PortableContextPresent bool                               `json:"portable_context_present"`
	Checkpoint             *CheckpointDetail                  `json:"checkpoint,omitempty"`
	Layers                 map[string]ExplorationContextLayer `json:"layers"`
}

// ExplorationContextOptions selects the projection scope.
type ExplorationContextOptions struct {
	ExplorationRef string
	Layer          string
	Cursor         string
	Limit          int
}

type explorationContextCursor struct {
	Version       int    `json:"v"`
	Layer         string `json:"layer"`
	ExplorationID string `json:"exploration"`
	Offset        int    `json:"offset"`
}

var explorationContextLayers = []string{"items", "intents", "evidence", "conversations"}

func validExplorationContextLayer(layer string) bool {
	for _, known := range explorationContextLayers {
		if known == layer {
			return true
		}
	}
	return false
}

func encodeExplorationContextCursor(cursor explorationContextCursor) string {
	payload, _ := json.Marshal(cursor)
	return base64.RawURLEncoding.EncodeToString(payload)
}

func decodeExplorationContextCursor(raw, layer, explorationID string) (explorationContextCursor, error) {
	payload, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return explorationContextCursor{}, fmt.Errorf("exploration context cursor is malformed")
	}
	var cursor explorationContextCursor
	if err := json.Unmarshal(payload, &cursor); err != nil {
		return explorationContextCursor{}, fmt.Errorf("exploration context cursor is malformed")
	}
	if cursor.Version != 1 || cursor.Layer != layer || cursor.ExplorationID != explorationID || cursor.Offset < 0 {
		return explorationContextCursor{}, fmt.Errorf("exploration context cursor does not match this layer and exploration; rerun without --cursor")
	}
	return cursor, nil
}

// ExplorationContext builds the portable projection for one Exploration.
func ExplorationContext(ctx context.Context, root project.Root, resolver PathResolver, options ExplorationContextOptions) (ExplorationContextResult, error) {
	store, err := openProjectStoreReadExisting(ctx, root, resolver)
	if err != nil {
		return ExplorationContextResult{}, err
	}
	defer store.Close()
	return store.ExplorationContext(ctx, root, options)
}

// ExplorationContext builds the projection on an open store.
func (s *Store) ExplorationContext(ctx context.Context, root project.Root, options ExplorationContextOptions) (ExplorationContextResult, error) {
	if options.Layer != "" && !validExplorationContextLayer(options.Layer) {
		return ExplorationContextResult{}, fmt.Errorf("unknown exploration context layer %q; layers are %s", options.Layer, strings.Join(explorationContextLayers, ", "))
	}
	if options.Cursor != "" && options.Layer == "" {
		return ExplorationContextResult{}, fmt.Errorf("--cursor requires --layer")
	}
	limit := options.Limit
	if limit == 0 {
		limit = explorationContextDefaultLimit
	}
	if limit < 1 || limit > explorationContextMaxLimit {
		return ExplorationContextResult{}, fmt.Errorf("exploration context limit must be between 1 and %d", explorationContextMaxLimit)
	}
	if options.Limit != 0 && options.Layer == "" {
		return ExplorationContextResult{}, fmt.Errorf("--limit requires --layer")
	}

	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return ExplorationContextResult{}, err
	}
	identity, err := s.projectIdentity(ctx, projectID)
	if err != nil {
		return ExplorationContextResult{}, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return ExplorationContextResult{}, fmt.Errorf("begin exploration context: %w", err)
	}
	defer tx.Rollback()

	explorationID, _, err := resolveExplorationRefTx(ctx, tx, projectID, options.ExplorationRef)
	if err != nil {
		return ExplorationContextResult{}, err
	}
	detail, err := loadExplorationDetailTx(ctx, tx, projectID, explorationID)
	if err != nil {
		return ExplorationContextResult{}, err
	}
	result := ExplorationContextResult{
		ContractVersion:        StateJSONContractVersion,
		DatabaseScope:          identity.DatabaseScope,
		DatabasePath:           identity.DatabasePath,
		ProjectID:              identity.ID,
		ProjectName:            identity.FriendlyName,
		ProjectCurrentPath:     identity.CurrentPath,
		Query:                  options.ExplorationRef,
		Exploration:            detail,
		PortableContextPresent: detail.PortableContextPresent,
		Layers:                 map[string]ExplorationContextLayer{},
	}
	var latestCheckpointID string
	if detail.LatestCheckpointSeq > 0 {
		if err := tx.QueryRowContext(ctx, `
SELECT id FROM exploration_checkpoints WHERE exploration_id = ? AND seq = ?
`, explorationID, detail.LatestCheckpointSeq).Scan(&latestCheckpointID); err != nil {
			return ExplorationContextResult{}, fmt.Errorf("read latest checkpoint: %w", err)
		}
		checkpoint, err := loadCheckpointTx(ctx, tx, projectID, latestCheckpointID)
		if err != nil {
			return ExplorationContextResult{}, fmt.Errorf("load latest checkpoint: %w", err)
		}
		result.Checkpoint = &checkpoint
	}

	layers := explorationContextLayers
	if options.Layer != "" {
		layers = []string{options.Layer}
	}
	offset := 0
	if options.Cursor != "" {
		cursor, err := decodeExplorationContextCursor(options.Cursor, options.Layer, explorationID)
		if err != nil {
			return ExplorationContextResult{}, err
		}
		offset = cursor.Offset
	}
	ref := detail.Alias
	if ref == "" {
		ref = explorationID
	}
	for _, layer := range layers {
		built, err := s.buildExplorationContextLayer(ctx, tx, projectID, explorationID, latestCheckpointID, layer, offset, limit, ref)
		if err != nil {
			return ExplorationContextResult{}, err
		}
		result.Layers[layer] = built
	}
	return result, nil
}

func (s *Store) buildExplorationContextLayer(ctx context.Context, tx *sql.Tx, projectID, explorationID, latestCheckpointID, layer string, offset, limit int, ref string) (ExplorationContextLayer, error) {
	var available int
	var items any
	switch layer {
	case "items":
		if latestCheckpointID == "" {
			items = []CheckpointItemDetail{}
			break
		}
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM exploration_checkpoint_items WHERE checkpoint_id = ?`, latestCheckpointID).Scan(&available); err != nil {
			return ExplorationContextLayer{}, err
		}
		rows, err := tx.QueryContext(ctx, `
SELECT item_type, position, content FROM exploration_checkpoint_items
WHERE checkpoint_id = ? ORDER BY position LIMIT ? OFFSET ?
`, latestCheckpointID, limit, offset)
		if err != nil {
			return ExplorationContextLayer{}, err
		}
		list := []CheckpointItemDetail{}
		for rows.Next() {
			var item CheckpointItemDetail
			if err := rows.Scan(&item.Type, &item.Position, &item.Content); err != nil {
				rows.Close()
				return ExplorationContextLayer{}, err
			}
			list = append(list, item)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return ExplorationContextLayer{}, err
		}
		items = list
	case "intents":
		if err := tx.QueryRowContext(ctx, `
SELECT COUNT(*) FROM relationships
WHERE project_id = ? AND from_entity_kind = 'exploration' AND from_entity_id = ?
  AND relationship_type IN ('explores', 'informs') AND to_entity_kind = 'intent'
`, projectID, explorationID).Scan(&available); err != nil {
			return ExplorationContextLayer{}, err
		}
		rows, err := tx.QueryContext(ctx, `
SELECT r.relationship_type, r.to_entity_id,
  COALESCE((SELECT alias FROM aliases WHERE project_id = r.project_id AND entity_kind = 'intent' AND entity_id = r.to_entity_id ORDER BY namespace, alias LIMIT 1), ''),
  COALESCE((SELECT title FROM intent_snapshots WHERE intent_id = r.to_entity_id ORDER BY seq DESC LIMIT 1), ''),
  COALESCE((SELECT disposition FROM intent_dispositions WHERE intent_id = r.to_entity_id ORDER BY seq DESC LIMIT 1), '')
FROM relationships AS r
WHERE r.project_id = ? AND r.from_entity_kind = 'exploration' AND r.from_entity_id = ?
  AND r.relationship_type IN ('explores', 'informs') AND r.to_entity_kind = 'intent'
ORDER BY r.relationship_type, r.to_entity_id LIMIT ? OFFSET ?
`, projectID, explorationID, limit, offset)
		if err != nil {
			return ExplorationContextLayer{}, err
		}
		type linkedIntent struct {
			Relationship string `json:"relationship"`
			ID           string `json:"id"`
			Alias        string `json:"alias,omitempty"`
			Title        string `json:"title"`
			Disposition  string `json:"disposition"`
		}
		list := []linkedIntent{}
		for rows.Next() {
			var item linkedIntent
			if err := rows.Scan(&item.Relationship, &item.ID, &item.Alias, &item.Title, &item.Disposition); err != nil {
				rows.Close()
				return ExplorationContextLayer{}, err
			}
			list = append(list, item)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return ExplorationContextLayer{}, err
		}
		items = list
	case "evidence":
		if err := tx.QueryRowContext(ctx, `
SELECT COUNT(*) FROM relationships
WHERE project_id = ? AND (
  (from_entity_kind = 'exploration' AND from_entity_id = ? AND relationship_type = 'uses-source')
  OR (to_entity_kind = 'exploration' AND to_entity_id = ? AND relationship_type = 'evidence-for')
)
`, projectID, explorationID, explorationID).Scan(&available); err != nil {
			return ExplorationContextLayer{}, err
		}
		rows, err := tx.QueryContext(ctx, `
SELECT relationship_type,
  CASE WHEN from_entity_kind = 'exploration' THEN to_entity_kind ELSE from_entity_kind END,
  CASE WHEN from_entity_kind = 'exploration' THEN to_entity_id ELSE from_entity_id END
FROM relationships
WHERE project_id = ? AND (
  (from_entity_kind = 'exploration' AND from_entity_id = ? AND relationship_type = 'uses-source')
  OR (to_entity_kind = 'exploration' AND to_entity_id = ? AND relationship_type = 'evidence-for')
)
ORDER BY relationship_type, 2, 3 LIMIT ? OFFSET ?
`, projectID, explorationID, explorationID, limit, offset)
		if err != nil {
			return ExplorationContextLayer{}, err
		}
		type evidenceItem struct {
			Relationship string `json:"relationship"`
			Kind         string `json:"kind"`
			ID           string `json:"id"`
			Title        string `json:"title,omitempty"`
		}
		list := []evidenceItem{}
		for rows.Next() {
			var item evidenceItem
			if err := rows.Scan(&item.Relationship, &item.Kind, &item.ID); err != nil {
				rows.Close()
				return ExplorationContextLayer{}, err
			}
			list = append(list, item)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return ExplorationContextLayer{}, err
		}
		for index := range list {
			if entity, err := s.entityDetails(ctx, projectID, list[index].Kind, list[index].ID); err == nil {
				list[index].Title = entity.Title
			}
		}
		items = list
	case "conversations":
		if err := tx.QueryRowContext(ctx, `
SELECT COUNT(*) FROM exploration_conversations WHERE exploration_id = ?
`, explorationID).Scan(&available); err != nil {
			return ExplorationContextLayer{}, err
		}
		rows, err := tx.QueryContext(ctx, `
SELECT conversation_id FROM exploration_conversations
WHERE exploration_id = ? ORDER BY conversation_id LIMIT ? OFFSET ?
`, explorationID, limit, offset)
		if err != nil {
			return ExplorationContextLayer{}, err
		}
		conversationIDs := []string{}
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return ExplorationContextLayer{}, err
			}
			conversationIDs = append(conversationIDs, id)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return ExplorationContextLayer{}, err
		}
		list := []ConversationDetail{}
		for _, id := range conversationIDs {
			detail, err := loadConversationDetailTx(ctx, tx, projectID, id)
			if err != nil {
				return ExplorationContextLayer{}, err
			}
			list = append(list, detail)
		}
		items = list
	default:
		return ExplorationContextLayer{}, fmt.Errorf("unknown exploration context layer %q", layer)
	}

	encoded, err := json.Marshal(items)
	if err != nil {
		return ExplorationContextLayer{}, fmt.Errorf("encode %s layer: %w", layer, err)
	}
	shown := countJSONArray(encoded)
	built := ExplorationContextLayer{
		Available: available,
		Shown:     shown,
		Truncated: offset+shown < available,
		Items:     encoded,
	}
	if built.Truncated {
		cursor := encodeExplorationContextCursor(explorationContextCursor{Version: 1, Layer: layer, ExplorationID: explorationID, Offset: offset + shown})
		built.Cursor = cursor
		built.ExpandCommand = fmt.Sprintf("loaf exploration context %s --layer %s --cursor %s --limit %d", ref, layer, cursor, limit)
	}
	return built, nil
}

func countJSONArray(encoded []byte) int {
	var raw []json.RawMessage
	if err := json.Unmarshal(encoded, &raw); err != nil {
		return 0
	}
	return len(raw)
}

// ConversationListResult lists logical conversations deterministically.
type ConversationListResult struct {
	ContractVersion    int                  `json:"contract_version"`
	DatabaseScope      string               `json:"database_scope,omitempty"`
	DatabasePath       string               `json:"database_path,omitempty"`
	ProjectID          string               `json:"project_id,omitempty"`
	ProjectName        string               `json:"project_name,omitempty"`
	ProjectCurrentPath string               `json:"project_current_path,omitempty"`
	Conversations      []ConversationDetail `json:"conversations"`
}

// ConversationShowResult is the single-conversation read model.
type ConversationShowResult struct {
	ContractVersion    int                `json:"contract_version"`
	DatabaseScope      string             `json:"database_scope,omitempty"`
	DatabasePath       string             `json:"database_path,omitempty"`
	ProjectID          string             `json:"project_id,omitempty"`
	ProjectName        string             `json:"project_name,omitempty"`
	ProjectCurrentPath string             `json:"project_current_path,omitempty"`
	Query              string             `json:"query"`
	Conversation       ConversationDetail `json:"conversation"`
}

// ShowConversation returns one logical conversation with handles and logs.
func ShowConversation(ctx context.Context, root project.Root, resolver PathResolver, ref string) (ConversationShowResult, error) {
	store, err := openProjectStoreReadExisting(ctx, root, resolver)
	if err != nil {
		return ConversationShowResult{}, err
	}
	defer store.Close()
	projectID, err := store.projectID(ctx, root)
	if err != nil {
		return ConversationShowResult{}, err
	}
	identity, err := store.projectIdentity(ctx, projectID)
	if err != nil {
		return ConversationShowResult{}, err
	}
	tx, err := store.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return ConversationShowResult{}, fmt.Errorf("begin conversation show: %w", err)
	}
	defer tx.Rollback()
	conversationID, err := resolveConversationRefTx(ctx, tx, projectID, ref)
	if err != nil {
		return ConversationShowResult{}, err
	}
	detail, err := loadConversationDetailTx(ctx, tx, projectID, conversationID)
	if err != nil {
		return ConversationShowResult{}, err
	}
	return ConversationShowResult{
		ContractVersion:    StateJSONContractVersion,
		DatabaseScope:      identity.DatabaseScope,
		DatabasePath:       identity.DatabasePath,
		ProjectID:          identity.ID,
		ProjectName:        identity.FriendlyName,
		ProjectCurrentPath: identity.CurrentPath,
		Query:              ref,
		Conversation:       detail,
	}, nil
}

// ListConversations returns the deterministic conversation projection.
func ListConversations(ctx context.Context, root project.Root, resolver PathResolver) (ConversationListResult, error) {
	store, err := openProjectStoreReadExisting(ctx, root, resolver)
	if err != nil {
		return ConversationListResult{}, err
	}
	defer store.Close()
	projectID, err := store.projectID(ctx, root)
	if err != nil {
		return ConversationListResult{}, err
	}
	identity, err := store.projectIdentity(ctx, projectID)
	if err != nil {
		return ConversationListResult{}, err
	}
	tx, err := store.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return ConversationListResult{}, fmt.Errorf("begin conversation list: %w", err)
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `
SELECT id FROM logical_conversations WHERE project_id = ? ORDER BY created_at, id
`, projectID)
	if err != nil {
		return ConversationListResult{}, fmt.Errorf("list conversations: %w", err)
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return ConversationListResult{}, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return ConversationListResult{}, err
	}
	conversations := []ConversationDetail{}
	for _, id := range ids {
		detail, err := loadConversationDetailTx(ctx, tx, projectID, id)
		if err != nil {
			return ConversationListResult{}, err
		}
		conversations = append(conversations, detail)
	}
	return ConversationListResult{
		ContractVersion:    StateJSONContractVersion,
		DatabaseScope:      identity.DatabaseScope,
		DatabasePath:       identity.DatabasePath,
		ProjectID:          identity.ID,
		ProjectName:        identity.FriendlyName,
		ProjectCurrentPath: identity.CurrentPath,
		Conversations:      conversations,
	}, nil
}
