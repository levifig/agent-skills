package state

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/levifig/loaf/internal/project"
)

// SpecStatusResult describes a state-backed spec status mutation.
type SpecStatusResult struct {
	ContractVersion    int          `json:"contract_version,omitempty"`
	DatabaseScope      string       `json:"database_scope,omitempty"`
	DatabasePath       string       `json:"database_path,omitempty"`
	ProjectID          string       `json:"project_id,omitempty"`
	ProjectName        string       `json:"project_name,omitempty"`
	ProjectCurrentPath string       `json:"project_current_path,omitempty"`
	Spec               *TraceEntity `json:"spec,omitempty"`
	Ref                string       `json:"ref,omitempty"`
	Previous           string       `json:"previous_status"`
	Status             string       `json:"status"`
	EventID            string       `json:"event_id,omitempty"`
}

// ValidSpecStatus reports whether status is a recognized spec lifecycle status,
// accepting both canonical spellings and legacy aliases.
func ValidSpecStatus(status string) bool {
	_, ok := CanonicalLifecycleStatus(LifecycleEntitySpec, status)
	return ok
}

// SetSpecStatus sets a spec's lifecycle status in initialized SQLite state.
func SetSpecStatus(ctx context.Context, root project.Root, resolver PathResolver, ref string, status string) (SpecStatusResult, error) {
	store, err := openInitializedStore(root, resolver)
	if err != nil {
		return SpecStatusResult{}, err
	}
	defer store.Close()
	return store.SetSpecStatus(ctx, root, ref, status)
}

// SetSpecStatus sets a spec's lifecycle status in an open store.
func (s *Store) SetSpecStatus(ctx context.Context, root project.Root, ref string, status string) (SpecStatusResult, error) {
	canonical, ok := CanonicalLifecycleStatus(LifecycleEntitySpec, status)
	if !ok {
		return SpecStatusResult{}, fmt.Errorf("invalid spec status %q (valid: %s)", status, strings.Join(LifecycleStatusesForEntity(LifecycleEntitySpec), ", "))
	}

	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return SpecStatusResult{}, err
	}
	identity, err := s.projectIdentity(ctx, projectID)
	if err != nil {
		return SpecStatusResult{}, err
	}

	spec, err := s.resolveTraceEntity(ctx, projectID, ref)
	if err != nil {
		return SpecStatusResult{}, err
	}
	if spec.Kind != "spec" {
		return SpecStatusResult{}, fmt.Errorf("%q resolves to %s, not spec", ref, spec.Kind)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return SpecStatusResult{}, fmt.Errorf("begin spec status transaction: %w", err)
	}
	defer tx.Rollback()

	var previousStatus string
	err = tx.QueryRowContext(ctx, `SELECT status FROM specs WHERE project_id = ? AND id = ?`, projectID, spec.ID).Scan(&previousStatus)
	if errors.Is(err, sql.ErrNoRows) {
		return SpecStatusResult{}, fmt.Errorf("spec %q not found in SQLite state", ref)
	}
	if err != nil {
		return SpecStatusResult{}, fmt.Errorf("read spec status: %w", err)
	}
	previousCanonical := LifecycleStatusForDisplay(LifecycleEntitySpec, previousStatus)

	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := tx.ExecContext(ctx, `UPDATE specs SET status = ?, updated_at = ? WHERE project_id = ? AND id = ?`, canonical, now, projectID, spec.ID); err != nil {
		return SpecStatusResult{}, fmt.Errorf("update spec status: %w", err)
	}

	eventID := ""
	if previousCanonical != canonical {
		eventID = stableMigrationID("event", projectID, "spec", spec.ID, "status", previousCanonical, canonical)
		_, err = tx.ExecContext(ctx, `
INSERT INTO events (id, project_id, entity_kind, entity_id, event_type, from_status, to_status, note, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO NOTHING
`, eventID, projectID, "spec", spec.ID, "status_changed", previousCanonical, canonical, "recorded by spec status", now, now)
		if err != nil {
			return SpecStatusResult{}, fmt.Errorf("record spec status event: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return SpecStatusResult{}, fmt.Errorf("commit spec status transaction: %w", err)
	}

	spec.Status = canonical
	return SpecStatusResult{
		ContractVersion:    StateJSONContractVersion,
		DatabaseScope:      identity.DatabaseScope,
		DatabasePath:       identity.DatabasePath,
		ProjectID:          identity.ID,
		ProjectName:        identity.FriendlyName,
		ProjectCurrentPath: identity.CurrentPath,
		Spec:               &spec,
		Ref:                ref,
		Previous:           previousCanonical,
		Status:             canonical,
		EventID:            eventID,
	}, nil
}
