package state

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const (
	FactKindSparkCaptured        = "spark.captured"
	FactKindSparkPromoted        = "spark.promoted"
	FactKindSparkArchived        = "spark.archived"
	FactKindIdeaCreated          = "idea.created"
	FactKindIdeaPromoted         = "idea.promoted"
	FactKindIdeaResolved         = "idea.resolved"
	FactKindIdeaArchived         = "idea.archived"
	FactKindHandoffRecorded      = "handoff.recorded"
	FactKindRefRegistered        = "ref.registered"
	FactKindWorktreeBound        = "worktree.bound"
	FactKindWorktreeUnbound      = "worktree.unbound"
	FactKindVerificationRecorded = "verification.recorded"
	FactKindReleaseRecorded      = "release.recorded"

	PermanenceLedger   = "ledger"
	PermanenceNotebook = "notebook"

	mutableCoreEventFactMigrationVersion = 25
)

func init() {
	RegisterFactKind(FactKindSparkCaptured, PermanenceNotebook)
	RegisterFactKind(FactKindSparkPromoted, PermanenceNotebook)
	RegisterFactKind(FactKindSparkArchived, PermanenceNotebook)
	RegisterFactKind(FactKindIdeaCreated, PermanenceNotebook)
	RegisterFactKind(FactKindIdeaPromoted, PermanenceNotebook)
	RegisterFactKind(FactKindIdeaResolved, PermanenceNotebook)
	RegisterFactKind(FactKindIdeaArchived, PermanenceNotebook)
	RegisterFactKind(FactKindHandoffRecorded, PermanenceLedger)
	RegisterFactKind(FactKindRefRegistered, PermanenceLedger)
	RegisterFactKind(FactKindWorktreeBound, PermanenceLedger)
	RegisterFactKind(FactKindWorktreeUnbound, PermanenceLedger)
	RegisterFactKind(FactKindVerificationRecorded, PermanenceLedger)
	RegisterFactKind(FactKindReleaseRecorded, PermanenceLedger)
}

// CoreEventPayload is the latest-wins body for mutable-core event facts.
// Display timestamps live here and never participate in fold order.
type CoreEventPayload struct {
	SubjectKind      string              `json:"subject_kind"`
	SubjectID        string              `json:"subject_id"`
	Alias            string              `json:"alias,omitempty"`
	Status           string              `json:"status,omitempty"`
	Title            string              `json:"title,omitempty"`
	Text             string              `json:"text,omitempty"`
	Scope            string              `json:"scope,omitempty"`
	Body             string              `json:"body,omitempty"`
	Note             string              `json:"note,omitempty"`
	FromStatus       string              `json:"from_status,omitempty"`
	ToStatus         string              `json:"to_status,omitempty"`
	RelatedKind      string              `json:"related_kind,omitempty"`
	RelatedID        string              `json:"related_id,omitempty"`
	HarnessSessionID string              `json:"harness_session_id,omitempty"`
	TaskID           string              `json:"task_id,omitempty"`
	Version          string              `json:"version,omitempty"`
	Tag              string              `json:"tag,omitempty"`
	TaggedCommit     string              `json:"tagged_commit,omitempty"`
	Notes            string              `json:"notes,omitempty"`
	Members          []ReleaseMemberFact `json:"members,omitempty"`
	Backend          string              `json:"backend,omitempty"`
	EntityKind       string              `json:"entity_kind,omitempty"`
	EntityID         string              `json:"entity_id,omitempty"`
	ExternalKind     string              `json:"external_kind,omitempty"`
	ExternalID       string              `json:"external_id,omitempty"`
	ExternalURL      string              `json:"external_url,omitempty"`
	SyncStatus       string              `json:"sync_status,omitempty"`
	Provider         string              `json:"provider,omitempty"`
	ProviderRef      string              `json:"provider_ref,omitempty"`
	MappingKind      string              `json:"mapping_kind,omitempty"`
	MappingValue     string              `json:"mapping_value,omitempty"`
	ReceiptKind      string              `json:"receipt_kind,omitempty"`
	ReceiptValue     string              `json:"receipt_value,omitempty"`
	Branch           string              `json:"branch,omitempty"`
	Worktree         string              `json:"worktree,omitempty"`
	CreatedAt        string              `json:"created_at,omitempty"`
	UpdatedAt        string              `json:"updated_at,omitempty"`
}

// ReleaseMemberFact is the grow-only member snapshot carried on release.recorded.
type ReleaseMemberFact struct {
	Kind     string `json:"member_kind"`
	MemberID string `json:"member_id"`
}

func encodeCoreEventPayload(payload CoreEventPayload) (string, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode core event payload: %w", err)
	}
	return string(raw), nil
}

func decodeCoreEventPayload(raw string) (CoreEventPayload, error) {
	var payload CoreEventPayload
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return CoreEventPayload{}, fmt.Errorf("decode core event payload: %w", err)
	}
	return payload, nil
}

func appendCoreEventFactTx(ctx context.Context, tx *sql.Tx, projectID, kind, id string, payload CoreEventPayload, now time.Time, envID string) (FactEnvelope, error) {
	encoded, err := encodeCoreEventPayload(payload)
	if err != nil {
		return FactEnvelope{}, err
	}
	return appendFactTx(ctx, tx, AppendFactInput{
		ProjectID: projectID,
		Kind:      kind,
		Payload:   encoded,
		EnvID:     envID,
		ID:        id,
		Now:       now,
	})
}

func parseCoreEventTime(value string) time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Now().UTC()
	}
	if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return parsed.UTC()
	}
	if parsed, err := time.Parse(time.RFC3339, value); err == nil {
		return parsed.UTC()
	}
	return time.Now().UTC()
}

var mutableCoreFactKinds = []string{
	FactKindSparkCaptured,
	FactKindSparkPromoted,
	FactKindSparkArchived,
	FactKindIdeaCreated,
	FactKindIdeaPromoted,
	FactKindIdeaResolved,
	FactKindIdeaArchived,
	FactKindHandoffRecorded,
	FactKindRefRegistered,
	FactKindWorktreeBound,
	FactKindWorktreeUnbound,
	FactKindVerificationRecorded,
	FactKindReleaseRecorded,
}

func sparkFactKinds() []string {
	return []string{FactKindSparkCaptured, FactKindSparkPromoted, FactKindSparkArchived}
}

func ideaFactKinds() []string {
	return []string{FactKindIdeaCreated, FactKindIdeaPromoted, FactKindIdeaResolved, FactKindIdeaArchived}
}

func worktreeFactKinds() []string {
	return []string{FactKindWorktreeBound, FactKindWorktreeUnbound}
}
