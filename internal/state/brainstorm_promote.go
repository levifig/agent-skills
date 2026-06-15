package state

import (
	"context"
	"fmt"
	"time"

	"github.com/levifig/loaf/internal/project"
)

// BrainstormPromoteOptions describes a SQLite-backed brainstorm promotion request.
type BrainstormPromoteOptions struct {
	Brainstorm string
	ToIdea     string
}

// BrainstormPromoteResult describes a state-backed brainstorm promotion mutation.
type BrainstormPromoteResult struct {
	ContractVersion    int         `json:"contract_version,omitempty"`
	DatabaseScope      string      `json:"database_scope,omitempty"`
	DatabasePath       string      `json:"database_path,omitempty"`
	ProjectID          string      `json:"project_id,omitempty"`
	ProjectName        string      `json:"project_name,omitempty"`
	ProjectCurrentPath string      `json:"project_current_path,omitempty"`
	Brainstorm         TraceEntity `json:"brainstorm"`
	Idea               TraceEntity `json:"idea"`
	Relationship       string      `json:"relationship"`
}

// PromoteBrainstorm records that a brainstorm promoted to an idea in initialized SQLite state.
func PromoteBrainstorm(ctx context.Context, root project.Root, resolver PathResolver, options BrainstormPromoteOptions) (BrainstormPromoteResult, error) {
	store, err := openInitializedStore(root, resolver)
	if err != nil {
		return BrainstormPromoteResult{}, err
	}
	defer store.Close()
	return store.PromoteBrainstorm(ctx, root, options)
}

// PromoteBrainstorm records that a brainstorm promoted to an idea in an open store.
func (s *Store) PromoteBrainstorm(ctx context.Context, root project.Root, options BrainstormPromoteOptions) (BrainstormPromoteResult, error) {
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return BrainstormPromoteResult{}, err
	}
	identity, err := s.projectIdentity(ctx, projectID)
	if err != nil {
		return BrainstormPromoteResult{}, err
	}
	brainstorm, err := s.resolveTraceEntity(ctx, projectID, options.Brainstorm)
	if err != nil {
		return BrainstormPromoteResult{}, err
	}
	if brainstorm.Kind != "brainstorm" {
		return BrainstormPromoteResult{}, fmt.Errorf("%q resolves to %s, not brainstorm", options.Brainstorm, brainstorm.Kind)
	}
	idea, err := s.resolveTraceEntity(ctx, projectID, options.ToIdea)
	if err != nil {
		return BrainstormPromoteResult{}, err
	}
	if idea.Kind != "idea" {
		return BrainstormPromoteResult{}, fmt.Errorf("%q resolves to %s, not idea", options.ToIdea, idea.Kind)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	relationshipID := stableMigrationID("relationship", projectID, "brainstorm", brainstorm.ID, "promoted_to", "idea", idea.ID)
	_, err = s.db.ExecContext(ctx, `
INSERT INTO relationships (id, project_id, from_entity_kind, from_entity_id, to_entity_kind, to_entity_id, relationship_type, reason, origin, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'command', ?, ?)
ON CONFLICT(id) DO UPDATE SET
  reason = excluded.reason,
  origin = excluded.origin,
  updated_at = excluded.updated_at
`, relationshipID, projectID, "brainstorm", brainstorm.ID, "idea", idea.ID, "promoted_to", "recorded by brainstorm promote", now, now)
	if err != nil {
		return BrainstormPromoteResult{}, fmt.Errorf("record brainstorm promotion relationship: %w", err)
	}

	return BrainstormPromoteResult{
		ContractVersion:    StateJSONContractVersion,
		DatabaseScope:      identity.DatabaseScope,
		DatabasePath:       identity.DatabasePath,
		ProjectID:          identity.ID,
		ProjectName:        identity.FriendlyName,
		ProjectCurrentPath: identity.CurrentPath,
		Brainstorm:         brainstorm,
		Idea:               idea,
		Relationship:       relationshipID,
	}, nil
}
