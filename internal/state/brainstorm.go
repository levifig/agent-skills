package state

import (
	"context"
	"fmt"

	"github.com/levifig/loaf/internal/project"
)

// BrainstormList is the state-backed brainstorm-list read model.
type BrainstormList struct {
	Version     int                       `json:"version"`
	Brainstorms map[string]BrainstormItem `json:"brainstorms"`
}

// BrainstormItem is a brainstorm entry returned by the state-backed brainstorm list.
type BrainstormItem struct {
	Title      string `json:"title"`
	Status     string `json:"status"`
	SourcePath string `json:"source_path,omitempty"`
}

// BrainstormListOptions filter the state-backed brainstorm list.
type BrainstormListOptions struct {
	All    bool
	Status string
}

// ListBrainstorms returns imported brainstorms from initialized SQLite state.
func ListBrainstorms(ctx context.Context, root project.Root, resolver PathResolver, options BrainstormListOptions) (BrainstormList, error) {
	store, err := openInitializedStore(root, resolver)
	if err != nil {
		return BrainstormList{}, err
	}
	defer store.Close()
	return store.ListBrainstorms(ctx, root, options)
}

// ListBrainstorms returns imported brainstorms from an open store.
func (s *Store) ListBrainstorms(ctx context.Context, root project.Root, options BrainstormListOptions) (BrainstormList, error) {
	projectID := ProjectID(root)
	rows, err := s.db.QueryContext(ctx, `
SELECT
  brainstorm_alias.alias,
  brainstorms.title,
  brainstorms.status,
  COALESCE(sources.path, '')
FROM brainstorms
JOIN aliases brainstorm_alias
  ON brainstorm_alias.project_id = brainstorms.project_id
 AND brainstorm_alias.entity_kind = 'brainstorm'
 AND brainstorm_alias.entity_id = brainstorms.id
 AND brainstorm_alias.namespace = 'brainstorm'
LEFT JOIN sources ON sources.id = brainstorms.body_source_id
WHERE brainstorms.project_id = ?
ORDER BY brainstorm_alias.alias
`, projectID)
	if err != nil {
		return BrainstormList{}, fmt.Errorf("query brainstorms: %w", err)
	}

	brainstorms := BrainstormList{Version: 1, Brainstorms: map[string]BrainstormItem{}}
	for rows.Next() {
		var alias, title, status, sourcePath string
		if err := rows.Scan(&alias, &title, &status, &sourcePath); err != nil {
			rows.Close()
			return BrainstormList{}, fmt.Errorf("scan brainstorm: %w", err)
		}
		if !includeBrainstormStatus(status, options) {
			continue
		}
		brainstorms.Brainstorms[alias] = BrainstormItem{
			Title:      title,
			Status:     status,
			SourcePath: sourcePath,
		}
	}
	if err := rows.Close(); err != nil {
		return BrainstormList{}, fmt.Errorf("close brainstorms: %w", err)
	}
	if err := rows.Err(); err != nil {
		return BrainstormList{}, fmt.Errorf("iterate brainstorms: %w", err)
	}
	return brainstorms, nil
}

func includeBrainstormStatus(status string, options BrainstormListOptions) bool {
	if options.Status != "" && status != options.Status {
		return false
	}
	if options.Status != "" {
		return true
	}
	if !options.All && (status == "resolved" || status == "archived") {
		return false
	}
	return true
}
