package state

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/levifig/loaf/internal/project"
)

// TagList is the state-backed tag-list read model.
type TagList struct {
	ContractVersion    int                `json:"contract_version,omitempty"`
	DatabaseScope      string             `json:"database_scope,omitempty"`
	DatabasePath       string             `json:"database_path,omitempty"`
	ProjectID          string             `json:"project_id,omitempty"`
	ProjectName        string             `json:"project_name,omitempty"`
	ProjectCurrentPath string             `json:"project_current_path,omitempty"`
	Version            int                `json:"version"`
	Tags               map[string]TagItem `json:"tags"`
}

// TagItem is a tag entry returned by the state-backed tag list.
type TagItem struct {
	Count int `json:"count"`
}

// TagShowResult describes a tag and its classified rows.
type TagShowResult struct {
	ContractVersion    int           `json:"contract_version,omitempty"`
	DatabaseScope      string        `json:"database_scope,omitempty"`
	DatabasePath       string        `json:"database_path,omitempty"`
	ProjectID          string        `json:"project_id,omitempty"`
	ProjectName        string        `json:"project_name,omitempty"`
	ProjectCurrentPath string        `json:"project_current_path,omitempty"`
	Name               string        `json:"name"`
	Members            []TraceEntity `json:"members"`
}

// TagMutationResult describes an add/remove tag mutation.
type TagMutationResult struct {
	ContractVersion    int         `json:"contract_version,omitempty"`
	DatabaseScope      string      `json:"database_scope,omitempty"`
	DatabasePath       string      `json:"database_path,omitempty"`
	ProjectID          string      `json:"project_id,omitempty"`
	ProjectName        string      `json:"project_name,omitempty"`
	ProjectCurrentPath string      `json:"project_current_path,omitempty"`
	Name               string      `json:"name"`
	Entity             TraceEntity `json:"entity"`
}

// ListTags returns tags from initialized SQLite state.
func ListTags(ctx context.Context, root project.Root, resolver PathResolver) (TagList, error) {
	store, err := openInitializedStore(root, resolver)
	if err != nil {
		return TagList{}, err
	}
	defer store.Close()
	return store.ListTags(ctx, root)
}

// ShowTag returns members for one tag from initialized SQLite state.
func ShowTag(ctx context.Context, root project.Root, resolver PathResolver, name string) (TagShowResult, error) {
	store, err := openInitializedStore(root, resolver)
	if err != nil {
		return TagShowResult{}, err
	}
	defer store.Close()
	return store.ShowTag(ctx, root, name)
}

// AddTag adds a tag membership in initialized SQLite state.
func AddTag(ctx context.Context, root project.Root, resolver PathResolver, ref string, name string) (TagMutationResult, error) {
	store, err := openInitializedStore(root, resolver)
	if err != nil {
		return TagMutationResult{}, err
	}
	defer store.Close()
	return store.AddTag(ctx, root, ref, name)
}

// RemoveTag removes a tag membership in initialized SQLite state.
func RemoveTag(ctx context.Context, root project.Root, resolver PathResolver, ref string, name string) (TagMutationResult, error) {
	store, err := openInitializedStore(root, resolver)
	if err != nil {
		return TagMutationResult{}, err
	}
	defer store.Close()
	return store.RemoveTag(ctx, root, ref, name)
}

// ListTags returns tags from an open store.
func (s *Store) ListTags(ctx context.Context, root project.Root) (TagList, error) {
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return TagList{}, err
	}
	identity, err := s.projectIdentity(ctx, projectID)
	if err != nil {
		return TagList{}, err
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT tags.name, COUNT(entity_tags.id)
FROM tags
LEFT JOIN entity_tags
  ON entity_tags.project_id = tags.project_id
 AND entity_tags.tag_id = tags.id
WHERE tags.project_id = ?
GROUP BY tags.name
ORDER BY tags.name
`, projectID)
	if err != nil {
		return TagList{}, fmt.Errorf("query tags: %w", err)
	}
	defer rows.Close()

	result := TagList{
		ContractVersion:    StateJSONContractVersion,
		DatabaseScope:      identity.DatabaseScope,
		DatabasePath:       identity.DatabasePath,
		ProjectID:          identity.ID,
		ProjectName:        identity.FriendlyName,
		ProjectCurrentPath: identity.CurrentPath,
		Version:            1,
		Tags:               map[string]TagItem{},
	}
	for rows.Next() {
		var name string
		var count int
		if err := rows.Scan(&name, &count); err != nil {
			return TagList{}, fmt.Errorf("scan tag: %w", err)
		}
		result.Tags[name] = TagItem{Count: count}
	}
	if err := rows.Err(); err != nil {
		return TagList{}, fmt.Errorf("iterate tags: %w", err)
	}
	return result, nil
}

// ShowTag returns members for one tag from an open store.
func (s *Store) ShowTag(ctx context.Context, root project.Root, name string) (TagShowResult, error) {
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return TagShowResult{}, err
	}
	identity, err := s.projectIdentity(ctx, projectID)
	if err != nil {
		return TagShowResult{}, err
	}
	tagName, err := normalizeTagName(name)
	if err != nil {
		return TagShowResult{}, err
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT entity_tags.entity_kind, entity_tags.entity_id
FROM entity_tags
JOIN tags
  ON tags.project_id = entity_tags.project_id
 AND tags.id = entity_tags.tag_id
WHERE entity_tags.project_id = ? AND tags.name = ?
ORDER BY entity_tags.entity_kind, entity_tags.entity_id
`, projectID, tagName)
	if err != nil {
		return TagShowResult{}, fmt.Errorf("query tag members: %w", err)
	}

	type rawMember struct {
		kind string
		id   string
	}
	var raw []rawMember
	for rows.Next() {
		var kind, id string
		if err := rows.Scan(&kind, &id); err != nil {
			return TagShowResult{}, fmt.Errorf("scan tag member: %w", err)
		}
		raw = append(raw, rawMember{kind: kind, id: id})
	}
	if err := rows.Close(); err != nil {
		return TagShowResult{}, fmt.Errorf("close tag members: %w", err)
	}
	if err := rows.Err(); err != nil {
		return TagShowResult{}, fmt.Errorf("iterate tag members: %w", err)
	}

	var members []TraceEntity
	for _, member := range raw {
		entity, err := s.entityDetails(ctx, projectID, member.kind, member.id)
		if err != nil {
			return TagShowResult{}, err
		}
		if alias, err := s.entityAlias(ctx, projectID, member.kind, member.id); err == nil {
			entity.Alias = alias
		}
		members = append(members, entity)
	}
	sort.SliceStable(members, func(i, j int) bool {
		left := members[i].Kind + "\x00" + firstNonEmpty(members[i].Alias, members[i].ID)
		right := members[j].Kind + "\x00" + firstNonEmpty(members[j].Alias, members[j].ID)
		return left < right
	})
	return TagShowResult{
		ContractVersion:    StateJSONContractVersion,
		DatabaseScope:      identity.DatabaseScope,
		DatabasePath:       identity.DatabasePath,
		ProjectID:          identity.ID,
		ProjectName:        identity.FriendlyName,
		ProjectCurrentPath: identity.CurrentPath,
		Name:               tagName,
		Members:            members,
	}, nil
}

// AddTag adds a tag membership in an open store.
func (s *Store) AddTag(ctx context.Context, root project.Root, ref string, name string) (TagMutationResult, error) {
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return TagMutationResult{}, err
	}
	identity, err := s.projectIdentity(ctx, projectID)
	if err != nil {
		return TagMutationResult{}, err
	}
	tagName, err := normalizeTagName(name)
	if err != nil {
		return TagMutationResult{}, err
	}
	entity, err := s.resolveTraceEntity(ctx, projectID, ref)
	if err != nil {
		return TagMutationResult{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	tagID := stableMigrationID("tag", projectID, tagName)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return TagMutationResult{}, fmt.Errorf("begin tag transaction: %w", err)
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx, `
	INSERT INTO tags (id, project_id, name, created_at, updated_at)
	VALUES (?, ?, ?, ?, ?)
	ON CONFLICT(project_id, name) DO UPDATE SET updated_at = excluded.updated_at
	`, tagID, projectID, tagName, now, now)
	if err != nil {
		return TagMutationResult{}, fmt.Errorf("upsert tag %s: %w", tagName, err)
	}
	memberID := stableMigrationID("entity_tag", projectID, tagName, entity.Kind, entity.ID)
	_, err = tx.ExecContext(ctx, `
	INSERT INTO entity_tags (id, project_id, tag_id, entity_kind, entity_id, created_at, updated_at)
	VALUES (?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(project_id, tag_id, entity_kind, entity_id) DO UPDATE SET updated_at = excluded.updated_at
	`, memberID, projectID, tagID, entity.Kind, entity.ID, now, now)
	if err != nil {
		return TagMutationResult{}, fmt.Errorf("add tag membership: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return TagMutationResult{}, fmt.Errorf("commit tag transaction: %w", err)
	}
	return TagMutationResult{
		ContractVersion:    StateJSONContractVersion,
		DatabaseScope:      identity.DatabaseScope,
		DatabasePath:       identity.DatabasePath,
		ProjectID:          identity.ID,
		ProjectName:        identity.FriendlyName,
		ProjectCurrentPath: identity.CurrentPath,
		Name:               tagName,
		Entity:             entity,
	}, nil
}

// RemoveTag removes a tag membership in an open store.
func (s *Store) RemoveTag(ctx context.Context, root project.Root, ref string, name string) (TagMutationResult, error) {
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return TagMutationResult{}, err
	}
	identity, err := s.projectIdentity(ctx, projectID)
	if err != nil {
		return TagMutationResult{}, err
	}
	tagName, err := normalizeTagName(name)
	if err != nil {
		return TagMutationResult{}, err
	}
	entity, err := s.resolveTraceEntity(ctx, projectID, ref)
	if err != nil {
		return TagMutationResult{}, err
	}
	tagID := stableMigrationID("tag", projectID, tagName)
	result, err := s.db.ExecContext(ctx, `
DELETE FROM entity_tags
WHERE project_id = ? AND tag_id = ? AND entity_kind = ? AND entity_id = ?
`, projectID, tagID, entity.Kind, entity.ID)
	if err != nil {
		return TagMutationResult{}, fmt.Errorf("remove tag membership: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return TagMutationResult{}, fmt.Errorf("read removed tag membership count: %w", err)
	}
	if rows == 0 {
		return TagMutationResult{}, fmt.Errorf("tag %q is not attached to %s %q", tagName, entity.Kind, firstNonEmpty(entity.Alias, entity.ID))
	}
	return TagMutationResult{
		ContractVersion:    StateJSONContractVersion,
		DatabaseScope:      identity.DatabaseScope,
		DatabasePath:       identity.DatabasePath,
		ProjectID:          identity.ID,
		ProjectName:        identity.FriendlyName,
		ProjectCurrentPath: identity.CurrentPath,
		Name:               tagName,
		Entity:             entity,
	}, nil
}

func normalizeTagName(name string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(name))
	if normalized == "" {
		return "", fmt.Errorf("tag name cannot be empty")
	}
	if strings.ContainsAny(normalized, " \t\r\n") {
		return "", fmt.Errorf("tag name cannot contain whitespace")
	}
	return normalized, nil
}
