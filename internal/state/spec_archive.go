package state

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/levifig/loaf/internal/project"
)

// SpecArchiveResult describes a state-backed spec archive mutation.
type SpecArchiveResult struct {
	ContractVersion    int               `json:"contract_version,omitempty"`
	DatabaseScope      string            `json:"database_scope,omitempty"`
	DatabasePath       string            `json:"database_path,omitempty"`
	ProjectID          string            `json:"project_id,omitempty"`
	ProjectName        string            `json:"project_name,omitempty"`
	ProjectCurrentPath string            `json:"project_current_path,omitempty"`
	Archived           []SpecArchiveItem `json:"archived"`
	Skipped            []SpecArchiveItem `json:"skipped"`
}

// SpecArchiveItem describes one requested spec archive outcome.
type SpecArchiveItem struct {
	Spec     *TraceEntity `json:"spec,omitempty"`
	Ref      string       `json:"ref,omitempty"`
	Previous string       `json:"previous_status,omitempty"`
	Status   string       `json:"status,omitempty"`
	Reason   string       `json:"reason,omitempty"`
	EventID  string       `json:"event_id,omitempty"`
}

// ArchiveSpecs archives complete specs in initialized SQLite state.
func ArchiveSpecs(ctx context.Context, root project.Root, resolver PathResolver, refs []string) (SpecArchiveResult, error) {
	store, err := openProjectStoreMutateExisting(ctx, root, resolver)
	if err != nil {
		return SpecArchiveResult{}, err
	}
	defer store.Close()
	return store.ArchiveSpecs(ctx, root, refs)
}

// ArchiveSpecs archives complete specs in an open store.
func (s *Store) ArchiveSpecs(ctx context.Context, root project.Root, refs []string) (SpecArchiveResult, error) {
	if len(refs) == 0 {
		return SpecArchiveResult{}, fmt.Errorf("spec archive requires at least one spec")
	}
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return SpecArchiveResult{}, err
	}
	identity, err := s.projectIdentity(ctx, projectID)
	if err != nil {
		return SpecArchiveResult{}, err
	}
	result := SpecArchiveResult{
		ContractVersion:    StateJSONContractVersion,
		DatabaseScope:      identity.DatabaseScope,
		DatabasePath:       identity.DatabasePath,
		ProjectID:          identity.ID,
		ProjectName:        identity.FriendlyName,
		ProjectCurrentPath: identity.CurrentPath,
		Archived:           []SpecArchiveItem{},
		Skipped:            []SpecArchiveItem{},
	}
	for _, ref := range refs {
		item, archived, err := s.archiveSpec(ctx, projectID, ref)
		if err != nil {
			return SpecArchiveResult{}, err
		}
		if archived {
			result.Archived = append(result.Archived, item)
		} else {
			result.Skipped = append(result.Skipped, item)
		}
	}
	return result, nil
}

func (s *Store) archiveSpec(ctx context.Context, projectID string, ref string) (SpecArchiveItem, bool, error) {
	spec, err := s.resolveTraceEntity(ctx, projectID, ref)
	if err != nil {
		return SpecArchiveItem{Ref: ref, Reason: err.Error()}, false, nil
	}
	if spec.Kind != "spec" {
		return SpecArchiveItem{Spec: &spec, Ref: ref, Reason: fmt.Sprintf("%q resolves to %s, not spec", ref, spec.Kind)}, false, nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return SpecArchiveItem{}, false, fmt.Errorf("begin spec archive transaction: %w", err)
	}
	defer tx.Rollback()

	var previousStatus string
	err = tx.QueryRowContext(ctx, `SELECT status FROM specs WHERE project_id = ? AND id = ?`, projectID, spec.ID).Scan(&previousStatus)
	if errors.Is(err, sql.ErrNoRows) {
		return SpecArchiveItem{Spec: &spec, Ref: ref, Reason: fmt.Sprintf("spec %q not found in SQLite state", ref)}, false, nil
	}
	if err != nil {
		return SpecArchiveItem{}, false, fmt.Errorf("read spec status: %w", err)
	}

	if previousStatus == "archived" {
		return SpecArchiveItem{Spec: &spec, Ref: ref, Previous: previousStatus, Status: previousStatus, Reason: "already archived"}, false, nil
	}
	if previousStatus != "complete" {
		return SpecArchiveItem{Spec: &spec, Ref: ref, Previous: previousStatus, Status: previousStatus, Reason: fmt.Sprintf("status is %s, must be complete", previousStatus)}, false, nil
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := tx.ExecContext(ctx, `UPDATE specs SET status = ?, updated_at = ? WHERE project_id = ? AND id = ?`, "archived", now, projectID, spec.ID); err != nil {
		return SpecArchiveItem{}, false, fmt.Errorf("update spec status: %w", err)
	}

	eventID := stableMigrationID("event", projectID, "spec", spec.ID, "status", previousStatus, "archived")
	_, err = tx.ExecContext(ctx, `
INSERT INTO events (id, project_id, entity_kind, entity_id, event_type, from_status, to_status, note, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO NOTHING
`, eventID, projectID, "spec", spec.ID, "status_changed", previousStatus, "archived", "recorded by spec archive", now, now)
	if err != nil {
		return SpecArchiveItem{}, false, fmt.Errorf("record spec archive event: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return SpecArchiveItem{}, false, fmt.Errorf("commit spec archive transaction: %w", err)
	}

	spec.Status = "archived"
	return SpecArchiveItem{Spec: &spec, Ref: ref, Previous: previousStatus, Status: "archived", EventID: eventID}, true, nil
}
