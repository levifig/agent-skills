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

// LinkMutationOptions describes a relationship write or removal.
type LinkMutationOptions struct {
	From   string
	To     string
	Type   string
	Reason string
}

// LinkMutationResult describes a relationship mutation.
type LinkMutationResult struct {
	ContractVersion    int         `json:"contract_version,omitempty"`
	DatabaseScope      string      `json:"database_scope,omitempty"`
	DatabasePath       string      `json:"database_path,omitempty"`
	ProjectID          string      `json:"project_id,omitempty"`
	ProjectName        string      `json:"project_name,omitempty"`
	ProjectCurrentPath string      `json:"project_current_path,omitempty"`
	RelationshipID     string      `json:"relationship_id"`
	Type               string      `json:"type"`
	Reason             string      `json:"reason,omitempty"`
	From               TraceEntity `json:"from"`
	To                 TraceEntity `json:"to"`
}

// LinkListResult describes immediate relationships for one entity.
type LinkListResult struct {
	ContractVersion    int                 `json:"contract_version,omitempty"`
	DatabaseScope      string              `json:"database_scope,omitempty"`
	DatabasePath       string              `json:"database_path,omitempty"`
	ProjectID          string              `json:"project_id,omitempty"`
	ProjectName        string              `json:"project_name,omitempty"`
	ProjectCurrentPath string              `json:"project_current_path,omitempty"`
	Query              string              `json:"query"`
	Entity             TraceEntity         `json:"entity"`
	Relationships      []TraceRelationship `json:"relationships"`
}

// CreateLink writes an explicit relationship in initialized SQLite state.
func CreateLink(ctx context.Context, root project.Root, resolver PathResolver, options LinkMutationOptions) (LinkMutationResult, error) {
	store, err := openInitializedStore(root, resolver)
	if err != nil {
		return LinkMutationResult{}, err
	}
	defer store.Close()
	return store.CreateLink(ctx, root, options)
}

// ListLinks returns immediate relationships for one entity.
func ListLinks(ctx context.Context, root project.Root, resolver PathResolver, ref string) (LinkListResult, error) {
	store, err := openInitializedStore(root, resolver)
	if err != nil {
		return LinkListResult{}, err
	}
	defer store.Close()
	return store.ListLinks(ctx, root, ref)
}

// RemoveLink removes one explicit relationship from initialized SQLite state.
func RemoveLink(ctx context.Context, root project.Root, resolver PathResolver, options LinkMutationOptions) (LinkMutationResult, error) {
	store, err := openInitializedStore(root, resolver)
	if err != nil {
		return LinkMutationResult{}, err
	}
	defer store.Close()
	return store.RemoveLink(ctx, root, options)
}

// CreateLink writes an explicit relationship in an open store.
func (s *Store) CreateLink(ctx context.Context, root project.Root, options LinkMutationOptions) (LinkMutationResult, error) {
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return LinkMutationResult{}, err
	}
	identity, err := s.projectIdentity(ctx, projectID)
	if err != nil {
		return LinkMutationResult{}, err
	}
	from, to, relationshipType, reason, err := s.resolveLinkOptions(ctx, projectID, options)
	if err != nil {
		return LinkMutationResult{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	relationshipID := stableMigrationID("relationship", projectID, from.Kind, from.ID, relationshipType, to.Kind, to.ID)
	_, err = s.db.ExecContext(ctx, `
INSERT INTO relationships (id, project_id, from_entity_kind, from_entity_id, to_entity_kind, to_entity_id, relationship_type, reason, origin, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
  reason = excluded.reason,
  origin = excluded.origin,
  updated_at = excluded.updated_at
`, relationshipID, projectID, from.Kind, from.ID, to.Kind, to.ID, relationshipType, reason, "manual", now, now)
	if err != nil {
		return LinkMutationResult{}, fmt.Errorf("create link: %w", err)
	}
	return LinkMutationResult{
		ContractVersion:    StateJSONContractVersion,
		DatabaseScope:      identity.DatabaseScope,
		DatabasePath:       identity.DatabasePath,
		ProjectID:          identity.ID,
		ProjectName:        identity.FriendlyName,
		ProjectCurrentPath: identity.CurrentPath,
		RelationshipID:     relationshipID,
		Type:               relationshipType,
		Reason:             reason,
		From:               from,
		To:                 to,
	}, nil
}

// ListLinks returns immediate relationships for one entity from an open store.
func (s *Store) ListLinks(ctx context.Context, root project.Root, ref string) (LinkListResult, error) {
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return LinkListResult{}, err
	}
	identity, err := s.projectIdentity(ctx, projectID)
	if err != nil {
		return LinkListResult{}, err
	}
	entity, err := s.resolveTraceEntity(ctx, projectID, ref)
	if err != nil {
		return LinkListResult{}, err
	}
	relationships, err := s.traceRelationships(ctx, projectID, entity)
	if err != nil {
		return LinkListResult{}, err
	}
	return LinkListResult{
		ContractVersion:    StateJSONContractVersion,
		DatabaseScope:      identity.DatabaseScope,
		DatabasePath:       identity.DatabasePath,
		ProjectID:          identity.ID,
		ProjectName:        identity.FriendlyName,
		ProjectCurrentPath: identity.CurrentPath,
		Query:              ref,
		Entity:             entity,
		Relationships:      relationships,
	}, nil
}

// RemoveLink removes one explicit relationship from an open store.
func (s *Store) RemoveLink(ctx context.Context, root project.Root, options LinkMutationOptions) (LinkMutationResult, error) {
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return LinkMutationResult{}, err
	}
	identity, err := s.projectIdentity(ctx, projectID)
	if err != nil {
		return LinkMutationResult{}, err
	}
	from, to, relationshipType, _, err := s.resolveLinkOptions(ctx, projectID, options)
	if err != nil {
		return LinkMutationResult{}, err
	}
	relationshipID := stableMigrationID("relationship", projectID, from.Kind, from.ID, relationshipType, to.Kind, to.ID)
	var reason sql.NullString
	err = s.db.QueryRowContext(ctx, `SELECT reason FROM relationships WHERE id = ? AND project_id = ?`, relationshipID, projectID).Scan(&reason)
	if errors.Is(err, sql.ErrNoRows) {
		return LinkMutationResult{}, fmt.Errorf("link %s %q -> %s %q with type %q not found", from.Kind, firstNonEmpty(from.Alias, from.ID), to.Kind, firstNonEmpty(to.Alias, to.ID), relationshipType)
	}
	if err != nil {
		return LinkMutationResult{}, fmt.Errorf("read link before remove: %w", err)
	}
	result, err := s.db.ExecContext(ctx, `
DELETE FROM relationships
WHERE id = ? AND project_id = ?
`, relationshipID, projectID)
	if err != nil {
		return LinkMutationResult{}, fmt.Errorf("remove link: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return LinkMutationResult{}, fmt.Errorf("read removed link count: %w", err)
	}
	if rows == 0 {
		return LinkMutationResult{}, fmt.Errorf("link %s %q -> %s %q with type %q not found", from.Kind, firstNonEmpty(from.Alias, from.ID), to.Kind, firstNonEmpty(to.Alias, to.ID), relationshipType)
	}
	return LinkMutationResult{
		ContractVersion:    StateJSONContractVersion,
		DatabaseScope:      identity.DatabaseScope,
		DatabasePath:       identity.DatabasePath,
		ProjectID:          identity.ID,
		ProjectName:        identity.FriendlyName,
		ProjectCurrentPath: identity.CurrentPath,
		RelationshipID:     relationshipID,
		Type:               relationshipType,
		Reason:             reason.String,
		From:               from,
		To:                 to,
	}, nil
}

func (s *Store) resolveLinkOptions(ctx context.Context, projectID string, options LinkMutationOptions) (TraceEntity, TraceEntity, string, string, error) {
	relationshipType, err := normalizeRelationshipType(options.Type)
	if err != nil {
		return TraceEntity{}, TraceEntity{}, "", "", err
	}
	from, err := s.resolveTraceEntity(ctx, projectID, options.From)
	if err != nil {
		return TraceEntity{}, TraceEntity{}, "", "", err
	}
	to, err := s.resolveTraceEntity(ctx, projectID, options.To)
	if err != nil {
		return TraceEntity{}, TraceEntity{}, "", "", err
	}
	reason := strings.TrimSpace(options.Reason)
	if reason == "" {
		reason = "recorded by link create"
	}
	return from, to, relationshipType, reason, nil
}

func normalizeRelationshipType(value string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "" {
		return "", fmt.Errorf("relationship type cannot be empty")
	}
	if strings.ContainsAny(normalized, " \t\r\n") {
		return "", fmt.Errorf("relationship type cannot contain whitespace")
	}
	for _, char := range normalized {
		if (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') || char == '_' || char == '-' {
			continue
		}
		return "", fmt.Errorf("relationship type %q contains unsupported character %q", normalized, char)
	}
	return normalized, nil
}
