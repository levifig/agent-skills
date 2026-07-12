package state

import (
	"context"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

const (
	JournalOriginEnvelopeVersion = 1

	JournalOriginMechanismManual    = "manual"
	JournalOriginMechanismSkill     = "skill"
	JournalOriginMechanismHook      = "hook"
	JournalOriginMechanismMigration = "migration"
	JournalOriginMechanismUnknown   = "unknown"
)

const (
	journalOriginMechanismMaxLength = 64
	journalOriginStringMaxLength    = 1024
	journalOriginPathMaxLength      = 4096
	journalOriginDigestLength       = 32
)

// JournalOriginInput is optional provenance supplied when a journal entry is
// captured. Entry identity, project scope, and creation time are always taken
// from the canonical journal row rather than from this input.
type JournalOriginInput struct {
	EnvelopeVersion        int    `json:"envelope_version"`
	CaptureMechanism       string `json:"capture_mechanism"`
	ObservedHarness        string `json:"observed_harness,omitempty"`
	ObservedHarnessVersion string `json:"observed_harness_version,omitempty"`
	HarnessSessionID       string `json:"harness_session_id,omitempty"`
	AgentID                string `json:"agent_id,omitempty"`
	SourceEvent            string `json:"source_event,omitempty"`
	Branch                 string `json:"branch,omitempty"`
	Worktree               string `json:"worktree,omitempty"`
	Head                   string `json:"head,omitempty"`
	ChangePath             string `json:"change_path,omitempty"`
	ChangeSHA256           string `json:"change_sha256,omitempty"`
	Dirty                  *bool  `json:"dirty,omitempty"`
	Reconstructable        *bool  `json:"reconstructable,omitempty"`
	DurableResultKind      string `json:"durable_result_kind,omitempty"`
	DurableResultID        string `json:"durable_result_id,omitempty"`
}

// JournalOrigin is the normalized provenance envelope associated with one
// journal entry. Supported is false when the stored envelope version is newer
// than this binary; all stored fields remain visible without inference.
type JournalOrigin struct {
	JournalEntryID         string `json:"journal_entry_id"`
	ProjectID              string `json:"project_id"`
	EnvelopeVersion        int    `json:"envelope_version"`
	CaptureMechanism       string `json:"capture_mechanism"`
	ObservedHarness        string `json:"observed_harness,omitempty"`
	ObservedHarnessVersion string `json:"observed_harness_version,omitempty"`
	HarnessSessionID       string `json:"harness_session_id,omitempty"`
	AgentID                string `json:"agent_id,omitempty"`
	SourceEvent            string `json:"source_event,omitempty"`
	Branch                 string `json:"branch,omitempty"`
	Worktree               string `json:"worktree,omitempty"`
	Head                   string `json:"head,omitempty"`
	ChangePath             string `json:"change_path,omitempty"`
	ChangeSHA256           string `json:"change_sha256,omitempty"`
	Dirty                  *bool  `json:"dirty"`
	Reconstructable        *bool  `json:"reconstructable"`
	DurableResultKind      string `json:"durable_result_kind,omitempty"`
	DurableResultID        string `json:"durable_result_id,omitempty"`
	CreatedAt              string `json:"created_at"`
	Supported              bool   `json:"supported"`
}

func normalizeJournalOriginInput(input JournalOriginInput) (JournalOriginInput, error) {
	if input.EnvelopeVersion < 1 {
		return JournalOriginInput{}, fmt.Errorf("journal origin envelope_version must be at least 1")
	}
	mechanism, err := normalizeOriginString(input.CaptureMechanism, "capture_mechanism", journalOriginMechanismMaxLength, false)
	if err != nil {
		return JournalOriginInput{}, err
	}
	input.CaptureMechanism = mechanism
	for _, field := range []struct {
		name  string
		value *string
	}{
		{"observed_harness", &input.ObservedHarness},
		{"observed_harness_version", &input.ObservedHarnessVersion},
		{"harness_session_id", &input.HarnessSessionID},
		{"agent_id", &input.AgentID},
		{"source_event", &input.SourceEvent},
		{"branch", &input.Branch},
		{"worktree", &input.Worktree},
		{"head", &input.Head},
		{"durable_result_kind", &input.DurableResultKind},
		{"durable_result_id", &input.DurableResultID},
	} {
		value, err := normalizeOriginString(*field.value, field.name, journalOriginStringMaxLength, true)
		if err != nil {
			return JournalOriginInput{}, err
		}
		*field.value = value
	}
	changePath, err := normalizeOriginString(input.ChangePath, "change_path", journalOriginPathMaxLength, true)
	if err != nil {
		return JournalOriginInput{}, err
	}
	if changePath != "" {
		if filepath.IsAbs(changePath) || strings.Contains(changePath, "\\") {
			return JournalOriginInput{}, fmt.Errorf("journal origin change_path must be repository-relative")
		}
		for _, segment := range strings.Split(changePath, "/") {
			if segment == ".." {
				return JournalOriginInput{}, fmt.Errorf("journal origin change_path must not contain traversal")
			}
		}
	}
	input.ChangePath = changePath
	digest := strings.TrimSpace(input.ChangeSHA256)
	if digest != "" {
		if len(digest) != journalOriginDigestLength*2 {
			return JournalOriginInput{}, fmt.Errorf("journal origin change_sha256 must be a 64-character hexadecimal digest")
		}
		decoded, err := hex.DecodeString(digest)
		if err != nil || len(decoded) != journalOriginDigestLength {
			return JournalOriginInput{}, fmt.Errorf("journal origin change_sha256 must be a 64-character hexadecimal digest")
		}
		input.ChangeSHA256 = strings.ToLower(digest)
	} else {
		input.ChangeSHA256 = ""
	}
	hasChangePath := input.ChangePath != ""
	hasChangeDigest := input.ChangeSHA256 != ""
	if hasChangePath != hasChangeDigest {
		return JournalOriginInput{}, fmt.Errorf("journal origin change_path and change_sha256 must be provided together")
	}
	hasChangeEvidence := hasChangePath || hasChangeDigest || input.Dirty != nil || input.Reconstructable != nil
	if hasChangeEvidence {
		if !hasChangePath || !hasChangeDigest {
			return JournalOriginInput{}, fmt.Errorf("journal origin change evidence requires change_path and change_sha256")
		}
		if input.Dirty == nil || input.Reconstructable == nil {
			return JournalOriginInput{}, fmt.Errorf("journal origin change evidence requires dirty and reconstructable")
		}
		if *input.Dirty && *input.Reconstructable {
			return JournalOriginInput{}, fmt.Errorf("journal origin dirty and reconstructable cannot both be true")
		}
		if *input.Reconstructable {
			if *input.Dirty {
				return JournalOriginInput{}, fmt.Errorf("journal origin reconstructable requires dirty=false")
			}
			if input.Head == "" {
				return JournalOriginInput{}, fmt.Errorf("journal origin reconstructable requires head")
			}
		}
	}
	if (input.DurableResultKind == "") != (input.DurableResultID == "") {
		return JournalOriginInput{}, fmt.Errorf("journal origin durable_result_kind and durable_result_id must be provided together")
	}
	return input, nil
}

func normalizeOriginString(value string, field string, maxLength int, optional bool) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		if optional {
			return "", nil
		}
		return "", fmt.Errorf("journal origin %s cannot be empty", field)
	}
	if len(value) > maxLength {
		return "", fmt.Errorf("journal origin %s exceeds %d characters", field, maxLength)
	}
	if strings.IndexByte(value, 0) >= 0 {
		return "", fmt.Errorf("journal origin %s contains NUL", field)
	}
	return value, nil
}

func insertJournalOriginTx(ctx context.Context, tx *sql.Tx, journalEntryID string, input JournalOriginInput) error {
	var projectID, createdAt string
	if err := tx.QueryRowContext(ctx, `SELECT project_id, created_at FROM journal_entries WHERE id = ?`, journalEntryID).Scan(&projectID, &createdAt); err != nil {
		return fmt.Errorf("read canonical journal row for origin %s: %w", journalEntryID, err)
	}
	_, err := tx.ExecContext(ctx, `
INSERT INTO journal_origins (
  journal_entry_id, project_id, envelope_version, capture_mechanism,
  observed_harness, observed_harness_version, harness_session_id, agent_id,
  source_event, branch, worktree, head, change_path, change_sha256,
  dirty, reconstructable, durable_result_kind, durable_result_id, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`, journalEntryID, projectID, input.EnvelopeVersion, input.CaptureMechanism,
		emptyToNil(input.ObservedHarness), emptyToNil(input.ObservedHarnessVersion), emptyToNil(input.HarnessSessionID), emptyToNil(input.AgentID),
		emptyToNil(input.SourceEvent), emptyToNil(input.Branch), emptyToNil(input.Worktree), emptyToNil(input.Head), emptyToNil(input.ChangePath), emptyToNil(input.ChangeSHA256),
		optionalBoolValue(input.Dirty), optionalBoolValue(input.Reconstructable), emptyToNil(input.DurableResultKind), emptyToNil(input.DurableResultID), createdAt)
	if err != nil {
		return fmt.Errorf("insert journal origin %s: %w", journalEntryID, err)
	}
	return nil
}

func optionalBoolValue(value *bool) any {
	if value == nil {
		return nil
	}
	if *value {
		return 1
	}
	return 0
}

func loadJournalOrigin(ctx context.Context, queryer queryContext, projectID, journalEntryID string) (*JournalOrigin, error) {
	var tableCount int
	if err := queryer.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'journal_origins'`).Scan(&tableCount); err != nil {
		return nil, fmt.Errorf("inspect journal origins table: %w", err)
	}
	if tableCount == 0 {
		return nil, nil
	}
	row := queryer.QueryRowContext(ctx, `
SELECT
  journal_entry_id, project_id, envelope_version, capture_mechanism,
  observed_harness, observed_harness_version, harness_session_id, agent_id,
  source_event, branch, worktree, head, change_path, change_sha256,
  dirty, reconstructable, durable_result_kind, durable_result_id, created_at
FROM journal_origins
WHERE project_id = ? AND journal_entry_id = ?
`, projectID, journalEntryID)
	origin, err := scanJournalOrigin(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read journal origin %s: %w", journalEntryID, err)
	}
	origin.Supported = origin.EnvelopeVersion == JournalOriginEnvelopeVersion
	return &origin, nil
}

type journalOriginRowScanner interface {
	Scan(dest ...any) error
}

func scanJournalOrigin(row journalOriginRowScanner) (JournalOrigin, error) {
	var (
		origin                 JournalOrigin
		observedHarness        sql.NullString
		observedHarnessVersion sql.NullString
		harnessSessionID       sql.NullString
		agentID                sql.NullString
		sourceEvent            sql.NullString
		branch                 sql.NullString
		worktree               sql.NullString
		head                   sql.NullString
		changePath             sql.NullString
		changeSHA256           sql.NullString
		dirty                  sql.NullInt64
		reconstructable        sql.NullInt64
		durableResultKind      sql.NullString
		durableResultID        sql.NullString
	)
	if err := row.Scan(
		&origin.JournalEntryID, &origin.ProjectID, &origin.EnvelopeVersion, &origin.CaptureMechanism,
		&observedHarness, &observedHarnessVersion, &harnessSessionID, &agentID,
		&sourceEvent, &branch, &worktree, &head, &changePath, &changeSHA256,
		&dirty, &reconstructable, &durableResultKind, &durableResultID, &origin.CreatedAt,
	); err != nil {
		return JournalOrigin{}, err
	}
	origin.ObservedHarness = observedHarness.String
	origin.ObservedHarnessVersion = observedHarnessVersion.String
	origin.HarnessSessionID = harnessSessionID.String
	origin.AgentID = agentID.String
	origin.SourceEvent = sourceEvent.String
	origin.Branch = branch.String
	origin.Worktree = worktree.String
	origin.Head = head.String
	origin.ChangePath = changePath.String
	origin.ChangeSHA256 = changeSHA256.String
	origin.DurableResultKind = durableResultKind.String
	origin.DurableResultID = durableResultID.String
	if dirty.Valid {
		value := dirty.Int64 != 0
		origin.Dirty = &value
	}
	if reconstructable.Valid {
		value := reconstructable.Int64 != 0
		origin.Reconstructable = &value
	}
	return origin, nil
}
