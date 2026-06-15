package state

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/levifig/loaf/internal/project"
)

// BundleList is the state-backed bundle-list read model.
type BundleList struct {
	ContractVersion    int                   `json:"contract_version,omitempty"`
	DatabaseScope      string                `json:"database_scope,omitempty"`
	DatabasePath       string                `json:"database_path,omitempty"`
	ProjectID          string                `json:"project_id,omitempty"`
	ProjectName        string                `json:"project_name,omitempty"`
	ProjectCurrentPath string                `json:"project_current_path,omitempty"`
	Version            int                   `json:"version"`
	Bundles            map[string]BundleItem `json:"bundles"`
}

// BundleItem is one bundle row returned by the state-backed bundle list.
type BundleItem struct {
	Title           string   `json:"title"`
	TagQuery        []string `json:"tag_query,omitempty"`
	ExplicitCount   int      `json:"explicit_count"`
	TagMatchedCount int      `json:"tag_matched_count"`
	MemberCount     int      `json:"member_count"`
	CreatedAt       string   `json:"created_at"`
	UpdatedAt       string   `json:"updated_at"`
}

// BundleShowResult describes a bundle and its resolved member set.
type BundleShowResult struct {
	ContractVersion    int           `json:"contract_version,omitempty"`
	DatabaseScope      string        `json:"database_scope,omitempty"`
	DatabasePath       string        `json:"database_path,omitempty"`
	ProjectID          string        `json:"project_id,omitempty"`
	ProjectName        string        `json:"project_name,omitempty"`
	ProjectCurrentPath string        `json:"project_current_path,omitempty"`
	Slug               string        `json:"slug"`
	Title              string        `json:"title"`
	TagQuery           []string      `json:"tag_query,omitempty"`
	Members            []TraceEntity `json:"members"`
	Explicit           []TraceEntity `json:"explicit,omitempty"`
	TagMatched         []TraceEntity `json:"tag_matched,omitempty"`
}

// BundleMutationResult describes create/add/remove bundle mutations.
type BundleMutationResult struct {
	ContractVersion    int          `json:"contract_version,omitempty"`
	DatabaseScope      string       `json:"database_scope,omitempty"`
	DatabasePath       string       `json:"database_path,omitempty"`
	ProjectID          string       `json:"project_id,omitempty"`
	ProjectName        string       `json:"project_name,omitempty"`
	ProjectCurrentPath string       `json:"project_current_path,omitempty"`
	Slug               string       `json:"slug"`
	Title              string       `json:"title,omitempty"`
	Tags               []string     `json:"tags,omitempty"`
	Entity             *TraceEntity `json:"entity,omitempty"`
}

// BundleCreateOptions describes bundle creation.
type BundleCreateOptions struct {
	Slug  string
	Title string
	Tags  []string
}

// BundleUpdateOptions describes bundle metadata updates.
type BundleUpdateOptions struct {
	Slug     string
	Title    string
	SetTitle bool
	Tags     []string
	SetTags  bool
}

// CreateBundle creates or updates a bundle in initialized SQLite state.
func CreateBundle(ctx context.Context, root project.Root, resolver PathResolver, options BundleCreateOptions) (BundleMutationResult, error) {
	store, err := openInitializedStore(root, resolver)
	if err != nil {
		return BundleMutationResult{}, err
	}
	defer store.Close()
	return store.CreateBundle(ctx, root, options)
}

// ListBundles returns bundle rows from initialized SQLite state.
func ListBundles(ctx context.Context, root project.Root, resolver PathResolver) (BundleList, error) {
	store, err := openInitializedStore(root, resolver)
	if err != nil {
		return BundleList{}, err
	}
	defer store.Close()
	return store.ListBundles(ctx, root)
}

// ShowBundle returns a bundle and its full related set.
func ShowBundle(ctx context.Context, root project.Root, resolver PathResolver, slug string) (BundleShowResult, error) {
	store, err := openInitializedStore(root, resolver)
	if err != nil {
		return BundleShowResult{}, err
	}
	defer store.Close()
	return store.ShowBundle(ctx, root, slug)
}

// UpdateBundle updates an existing bundle in initialized SQLite state.
func UpdateBundle(ctx context.Context, root project.Root, resolver PathResolver, options BundleUpdateOptions) (BundleMutationResult, error) {
	store, err := openInitializedStore(root, resolver)
	if err != nil {
		return BundleMutationResult{}, err
	}
	defer store.Close()
	return store.UpdateBundle(ctx, root, options)
}

// AddBundleMember adds an explicit bundle member.
func AddBundleMember(ctx context.Context, root project.Root, resolver PathResolver, slug string, ref string) (BundleMutationResult, error) {
	store, err := openInitializedStore(root, resolver)
	if err != nil {
		return BundleMutationResult{}, err
	}
	defer store.Close()
	return store.AddBundleMember(ctx, root, slug, ref)
}

// RemoveBundleMember removes an explicit bundle member.
func RemoveBundleMember(ctx context.Context, root project.Root, resolver PathResolver, slug string, ref string) (BundleMutationResult, error) {
	store, err := openInitializedStore(root, resolver)
	if err != nil {
		return BundleMutationResult{}, err
	}
	defer store.Close()
	return store.RemoveBundleMember(ctx, root, slug, ref)
}

// CreateBundle creates or updates a bundle in an open store.
func (s *Store) CreateBundle(ctx context.Context, root project.Root, options BundleCreateOptions) (BundleMutationResult, error) {
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return BundleMutationResult{}, err
	}
	identity, err := s.projectIdentity(ctx, projectID)
	if err != nil {
		return BundleMutationResult{}, err
	}
	slug, err := normalizeBundleSlug(options.Slug)
	if err != nil {
		return BundleMutationResult{}, err
	}
	tags, err := normalizeBundleTags(options.Tags)
	if err != nil {
		return BundleMutationResult{}, err
	}
	title := strings.TrimSpace(options.Title)
	if title == "" {
		title = slug
	}
	now := time.Now().UTC().Format(time.RFC3339)
	id := stableMigrationID("bundle", projectID, slug)
	_, err = s.db.ExecContext(ctx, `
INSERT INTO bundles (id, project_id, slug, title, tag_query, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(project_id, slug) DO UPDATE SET
  title = excluded.title,
  tag_query = excluded.tag_query,
  updated_at = excluded.updated_at
`, id, projectID, slug, title, strings.Join(tags, ","), now, now)
	if err != nil {
		return BundleMutationResult{}, fmt.Errorf("upsert bundle %s: %w", slug, err)
	}
	return BundleMutationResult{
		ContractVersion:    StateJSONContractVersion,
		DatabaseScope:      identity.DatabaseScope,
		DatabasePath:       identity.DatabasePath,
		ProjectID:          identity.ID,
		ProjectName:        identity.FriendlyName,
		ProjectCurrentPath: identity.CurrentPath,
		Slug:               slug,
		Title:              title,
		Tags:               tags,
	}, nil
}

// ListBundles returns all bundles in an open store.
func (s *Store) ListBundles(ctx context.Context, root project.Root) (BundleList, error) {
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return BundleList{}, err
	}
	identity, err := s.projectIdentity(ctx, projectID)
	if err != nil {
		return BundleList{}, err
	}
	rows, err := s.db.QueryContext(ctx, `
	SELECT id, slug, title, COALESCE(tag_query, ''), created_at, updated_at
FROM bundles
WHERE project_id = ?
ORDER BY slug
`, projectID)
	if err != nil {
		return BundleList{}, fmt.Errorf("query bundles: %w", err)
	}

	type bundleRow struct {
		id        string
		slug      string
		title     string
		tagQuery  string
		createdAt string
		updatedAt string
	}
	var bundleRows []bundleRow
	for rows.Next() {
		var row bundleRow
		if err := rows.Scan(&row.id, &row.slug, &row.title, &row.tagQuery, &row.createdAt, &row.updatedAt); err != nil {
			rows.Close()
			return BundleList{}, fmt.Errorf("scan bundle: %w", err)
		}
		bundleRows = append(bundleRows, row)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return BundleList{}, fmt.Errorf("iterate bundles: %w", err)
	}
	if err := rows.Close(); err != nil {
		return BundleList{}, fmt.Errorf("close bundle rows: %w", err)
	}

	list := BundleList{
		ContractVersion:    StateJSONContractVersion,
		DatabaseScope:      identity.DatabaseScope,
		DatabasePath:       identity.DatabasePath,
		ProjectID:          identity.ID,
		ProjectName:        identity.FriendlyName,
		ProjectCurrentPath: identity.CurrentPath,
		Version:            1,
		Bundles:            map[string]BundleItem{},
	}
	for _, row := range bundleRows {
		tags, err := normalizeBundleTags(splitCommaList(row.tagQuery))
		if err != nil {
			return BundleList{}, err
		}
		explicit, err := s.bundleExplicitMembers(ctx, projectID, row.id)
		if err != nil {
			return BundleList{}, err
		}
		tagMatched, err := s.bundleTagMembers(ctx, projectID, tags)
		if err != nil {
			return BundleList{}, err
		}
		list.Bundles[row.slug] = BundleItem{
			Title:           row.title,
			TagQuery:        tags,
			ExplicitCount:   len(explicit),
			TagMatchedCount: len(tagMatched),
			MemberCount:     len(mergeTraceEntities(explicit, tagMatched)),
			CreatedAt:       row.createdAt,
			UpdatedAt:       row.updatedAt,
		}
	}
	return list, nil
}

// ShowBundle returns a bundle and its full related set from an open store.
func (s *Store) ShowBundle(ctx context.Context, root project.Root, slugValue string) (BundleShowResult, error) {
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return BundleShowResult{}, err
	}
	identity, err := s.projectIdentity(ctx, projectID)
	if err != nil {
		return BundleShowResult{}, err
	}
	bundleID, slug, title, tags, err := s.resolveBundle(ctx, projectID, slugValue)
	if err != nil {
		return BundleShowResult{}, err
	}
	tagMatched, err := s.bundleTagMembers(ctx, projectID, tags)
	if err != nil {
		return BundleShowResult{}, err
	}
	explicit, err := s.bundleExplicitMembers(ctx, projectID, bundleID)
	if err != nil {
		return BundleShowResult{}, err
	}
	members := mergeTraceEntities(tagMatched, explicit)
	return BundleShowResult{
		ContractVersion:    StateJSONContractVersion,
		DatabaseScope:      identity.DatabaseScope,
		DatabasePath:       identity.DatabasePath,
		ProjectID:          identity.ID,
		ProjectName:        identity.FriendlyName,
		ProjectCurrentPath: identity.CurrentPath,
		Slug:               slug,
		Title:              title,
		TagQuery:           tags,
		Members:            members,
		Explicit:           explicit,
		TagMatched:         tagMatched,
	}, nil
}

// UpdateBundle updates an existing bundle in an open store.
func (s *Store) UpdateBundle(ctx context.Context, root project.Root, options BundleUpdateOptions) (BundleMutationResult, error) {
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return BundleMutationResult{}, err
	}
	identity, err := s.projectIdentity(ctx, projectID)
	if err != nil {
		return BundleMutationResult{}, err
	}
	bundleID, slug, currentTitle, currentTags, err := s.resolveBundle(ctx, projectID, options.Slug)
	if err != nil {
		return BundleMutationResult{}, err
	}
	if !options.SetTitle && !options.SetTags {
		return BundleMutationResult{}, fmt.Errorf("bundle update requires --title, --tag, or --clear-tags")
	}

	title := currentTitle
	if options.SetTitle {
		title = strings.TrimSpace(options.Title)
		if title == "" {
			return BundleMutationResult{}, fmt.Errorf("bundle title cannot be empty")
		}
	}
	tags := currentTags
	if options.SetTags {
		tags, err = normalizeBundleTags(options.Tags)
		if err != nil {
			return BundleMutationResult{}, err
		}
	}

	now := time.Now().UTC().Format(time.RFC3339)
	_, err = s.db.ExecContext(ctx, `
UPDATE bundles
SET title = ?, tag_query = ?, updated_at = ?
WHERE project_id = ? AND id = ?
`, title, strings.Join(tags, ","), now, projectID, bundleID)
	if err != nil {
		return BundleMutationResult{}, fmt.Errorf("update bundle %s: %w", slug, err)
	}
	return BundleMutationResult{
		ContractVersion:    StateJSONContractVersion,
		DatabaseScope:      identity.DatabaseScope,
		DatabasePath:       identity.DatabasePath,
		ProjectID:          identity.ID,
		ProjectName:        identity.FriendlyName,
		ProjectCurrentPath: identity.CurrentPath,
		Slug:               slug,
		Title:              title,
		Tags:               tags,
	}, nil
}

// AddBundleMember adds an explicit member in an open store.
func (s *Store) AddBundleMember(ctx context.Context, root project.Root, slugValue string, ref string) (BundleMutationResult, error) {
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return BundleMutationResult{}, err
	}
	identity, err := s.projectIdentity(ctx, projectID)
	if err != nil {
		return BundleMutationResult{}, err
	}
	bundleID, slug, title, _, err := s.resolveBundle(ctx, projectID, slugValue)
	if err != nil {
		return BundleMutationResult{}, err
	}
	entity, err := s.resolveTraceEntity(ctx, projectID, ref)
	if err != nil {
		return BundleMutationResult{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	memberID := stableMigrationID("bundle_member", projectID, bundleID, entity.Kind, entity.ID)
	_, err = s.db.ExecContext(ctx, `
INSERT INTO bundle_members (id, project_id, bundle_id, entity_kind, entity_id, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(project_id, bundle_id, entity_kind, entity_id) DO UPDATE SET updated_at = excluded.updated_at
`, memberID, projectID, bundleID, entity.Kind, entity.ID, now, now)
	if err != nil {
		return BundleMutationResult{}, fmt.Errorf("add bundle member: %w", err)
	}
	return BundleMutationResult{
		ContractVersion:    StateJSONContractVersion,
		DatabaseScope:      identity.DatabaseScope,
		DatabasePath:       identity.DatabasePath,
		ProjectID:          identity.ID,
		ProjectName:        identity.FriendlyName,
		ProjectCurrentPath: identity.CurrentPath,
		Slug:               slug,
		Title:              title,
		Entity:             &entity,
	}, nil
}

// RemoveBundleMember removes an explicit member in an open store.
func (s *Store) RemoveBundleMember(ctx context.Context, root project.Root, slugValue string, ref string) (BundleMutationResult, error) {
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return BundleMutationResult{}, err
	}
	identity, err := s.projectIdentity(ctx, projectID)
	if err != nil {
		return BundleMutationResult{}, err
	}
	bundleID, slug, title, _, err := s.resolveBundle(ctx, projectID, slugValue)
	if err != nil {
		return BundleMutationResult{}, err
	}
	entity, err := s.resolveTraceEntity(ctx, projectID, ref)
	if err != nil {
		return BundleMutationResult{}, err
	}
	result, err := s.db.ExecContext(ctx, `
DELETE FROM bundle_members
WHERE project_id = ? AND bundle_id = ? AND entity_kind = ? AND entity_id = ?
`, projectID, bundleID, entity.Kind, entity.ID)
	if err != nil {
		return BundleMutationResult{}, fmt.Errorf("remove bundle member: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return BundleMutationResult{}, fmt.Errorf("read removed bundle member count: %w", err)
	}
	if rows == 0 {
		return BundleMutationResult{}, fmt.Errorf("%s %q is not an explicit member of bundle %q", entity.Kind, firstNonEmpty(entity.Alias, entity.ID), slug)
	}
	return BundleMutationResult{
		ContractVersion:    StateJSONContractVersion,
		DatabaseScope:      identity.DatabaseScope,
		DatabasePath:       identity.DatabasePath,
		ProjectID:          identity.ID,
		ProjectName:        identity.FriendlyName,
		ProjectCurrentPath: identity.CurrentPath,
		Slug:               slug,
		Title:              title,
		Entity:             &entity,
	}, nil
}

func (s *Store) resolveBundle(ctx context.Context, projectID string, slugValue string) (string, string, string, []string, error) {
	slug, err := normalizeBundleSlug(slugValue)
	if err != nil {
		return "", "", "", nil, err
	}
	var id, title, tagQuery string
	err = s.db.QueryRowContext(ctx, `SELECT id, title, COALESCE(tag_query, '') FROM bundles WHERE project_id = ? AND slug = ?`, projectID, slug).Scan(&id, &title, &tagQuery)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", "", nil, fmt.Errorf("bundle %q not found in SQLite state", slug)
	}
	if err != nil {
		return "", "", "", nil, fmt.Errorf("read bundle %s: %w", slug, err)
	}
	tags, err := normalizeBundleTags(splitCommaList(tagQuery))
	if err != nil {
		return "", "", "", nil, err
	}
	return id, slug, title, tags, nil
}

func (s *Store) bundleTagMembers(ctx context.Context, projectID string, tags []string) ([]TraceEntity, error) {
	if len(tags) == 0 {
		return []TraceEntity{}, nil
	}
	rawSeen := map[string]struct {
		kind string
		id   string
	}{}
	for _, tagName := range tags {
		rows, err := s.db.QueryContext(ctx, `
SELECT entity_tags.entity_kind, entity_tags.entity_id
FROM entity_tags
JOIN tags
  ON tags.project_id = entity_tags.project_id
 AND tags.id = entity_tags.tag_id
WHERE entity_tags.project_id = ? AND tags.name = ?
`, projectID, tagName)
		if err != nil {
			return nil, fmt.Errorf("query bundle tag members: %w", err)
		}
		for rows.Next() {
			var kind, id string
			if err := rows.Scan(&kind, &id); err != nil {
				rows.Close()
				return nil, fmt.Errorf("scan bundle tag member: %w", err)
			}
			rawSeen[kind+"\x00"+id] = struct {
				kind string
				id   string
			}{kind: kind, id: id}
		}
		if err := rows.Close(); err != nil {
			return nil, fmt.Errorf("close bundle tag members: %w", err)
		}
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("iterate bundle tag members: %w", err)
		}
	}
	members := map[string]TraceEntity{}
	for _, member := range rawSeen {
		entity, err := s.entityDetails(ctx, projectID, member.kind, member.id)
		if err != nil {
			return nil, err
		}
		if alias, err := s.entityAlias(ctx, projectID, member.kind, member.id); err == nil {
			entity.Alias = alias
		}
		members[entity.Kind+"\x00"+entity.ID] = entity
	}
	return sortedTraceEntityValues(members), nil
}

func (s *Store) bundleExplicitMembers(ctx context.Context, projectID string, bundleID string) ([]TraceEntity, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT entity_kind, entity_id
FROM bundle_members
WHERE project_id = ? AND bundle_id = ?
ORDER BY entity_kind, entity_id
`, projectID, bundleID)
	if err != nil {
		return nil, fmt.Errorf("query bundle members: %w", err)
	}
	type rawMember struct {
		kind string
		id   string
	}
	var raw []rawMember
	for rows.Next() {
		var kind, id string
		if err := rows.Scan(&kind, &id); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan bundle member: %w", err)
		}
		raw = append(raw, rawMember{kind: kind, id: id})
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close bundle members: %w", err)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate bundle members: %w", err)
	}
	members := map[string]TraceEntity{}
	for _, member := range raw {
		entity, err := s.entityDetails(ctx, projectID, member.kind, member.id)
		if err != nil {
			return nil, err
		}
		if alias, err := s.entityAlias(ctx, projectID, member.kind, member.id); err == nil {
			entity.Alias = alias
		}
		members[entity.Kind+"\x00"+entity.ID] = entity
	}
	return sortedTraceEntityValues(members), nil
}

func mergeTraceEntities(groups ...[]TraceEntity) []TraceEntity {
	merged := map[string]TraceEntity{}
	for _, group := range groups {
		for _, entity := range group {
			merged[entity.Kind+"\x00"+entity.ID] = entity
		}
	}
	return sortedTraceEntityValues(merged)
}

func sortedTraceEntityValues(values map[string]TraceEntity) []TraceEntity {
	entities := make([]TraceEntity, 0, len(values))
	for _, entity := range values {
		entities = append(entities, entity)
	}
	sort.SliceStable(entities, func(i, j int) bool {
		left := entities[i].Kind + "\x00" + firstNonEmpty(entities[i].Alias, entities[i].ID)
		right := entities[j].Kind + "\x00" + firstNonEmpty(entities[j].Alias, entities[j].ID)
		return left < right
	})
	return entities
}

func normalizeBundleSlug(slug string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(slug))
	if normalized == "" {
		return "", fmt.Errorf("bundle slug cannot be empty")
	}
	if strings.ContainsAny(normalized, " \t\r\n") {
		return "", fmt.Errorf("bundle slug cannot contain whitespace")
	}
	return normalized, nil
}

func normalizeBundleTags(tags []string) ([]string, error) {
	seen := map[string]bool{}
	var normalized []string
	for _, tag := range tags {
		name, err := normalizeTagName(tag)
		if err != nil {
			return nil, err
		}
		if seen[name] {
			continue
		}
		seen[name] = true
		normalized = append(normalized, name)
	}
	sort.Strings(normalized)
	return normalized, nil
}

func splitCommaList(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	var parts []string
	for _, part := range strings.Split(value, ",") {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			parts = append(parts, trimmed)
		}
	}
	return parts
}
