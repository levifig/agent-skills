package state

import (
	"context"
	"fmt"
	"os"

	"github.com/levifig/loaf/internal/project"
)

var specStatusOrder = []string{"implementing", "approved", "drafting", "complete", "archived"}

// SpecList is the state-backed spec-list read model.
type SpecList struct {
	Version int                 `json:"version"`
	Specs   map[string]SpecItem `json:"specs"`
}

// SpecItem is a spec entry returned by the state-backed spec list.
type SpecItem struct {
	Title      string         `json:"title"`
	Status     string         `json:"status"`
	Tasks      SpecTaskCounts `json:"tasks"`
	SourcePath string         `json:"source_path,omitempty"`
}

// SpecTaskCounts summarizes tasks associated with a spec.
type SpecTaskCounts struct {
	Todo       int `json:"todo"`
	InProgress int `json:"in_progress"`
	Done       int `json:"done"`
}

// ListSpecs returns imported specs from initialized SQLite state.
func ListSpecs(ctx context.Context, root project.Root, resolver PathResolver) (SpecList, error) {
	databasePath, err := resolver.DatabasePath(root)
	if err != nil {
		return SpecList{}, err
	}
	if _, err := os.Stat(databasePath); os.IsNotExist(err) {
		return SpecList{}, fmt.Errorf("SQLite state database is not initialized; run `loaf state migrate markdown --apply` first")
	} else if err != nil {
		return SpecList{}, fmt.Errorf("inspect state database: %w", err)
	}
	store, err := OpenStore(databasePath)
	if err != nil {
		return SpecList{}, err
	}
	defer store.Close()
	return store.ListSpecs(ctx, root)
}

// ListSpecs returns imported specs from an open store.
func (s *Store) ListSpecs(ctx context.Context, root project.Root) (SpecList, error) {
	projectID := ProjectID(root)
	rows, err := s.db.QueryContext(ctx, `
SELECT
  spec_alias.alias,
  specs.title,
  specs.status,
  COALESCE(sources.path, '')
FROM specs
JOIN aliases spec_alias
  ON spec_alias.project_id = specs.project_id
 AND spec_alias.entity_kind = 'spec'
 AND spec_alias.entity_id = specs.id
 AND spec_alias.namespace = 'spec'
LEFT JOIN sources ON sources.id = specs.body_source_id
WHERE specs.project_id = ?
ORDER BY spec_alias.alias
`, projectID)
	if err != nil {
		return SpecList{}, fmt.Errorf("query specs: %w", err)
	}

	specList := SpecList{Version: 1, Specs: map[string]SpecItem{}}
	for rows.Next() {
		var alias, title, status, sourcePath string
		if err := rows.Scan(&alias, &title, &status, &sourcePath); err != nil {
			rows.Close()
			return SpecList{}, fmt.Errorf("scan spec: %w", err)
		}
		specList.Specs[alias] = SpecItem{
			Title:      title,
			Status:     status,
			SourcePath: sourcePath,
		}
	}
	if err := rows.Close(); err != nil {
		return SpecList{}, fmt.Errorf("close specs: %w", err)
	}
	if err := rows.Err(); err != nil {
		return SpecList{}, fmt.Errorf("iterate specs: %w", err)
	}

	for alias, spec := range specList.Specs {
		counts, err := s.specTaskCounts(ctx, projectID, alias)
		if err != nil {
			return SpecList{}, err
		}
		spec.Tasks = counts
		specList.Specs[alias] = spec
	}
	return specList, nil
}

func (s *Store) specTaskCounts(ctx context.Context, projectID string, alias string) (SpecTaskCounts, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT tasks.status, COUNT(*)
FROM tasks
JOIN aliases spec_alias
  ON spec_alias.project_id = tasks.project_id
 AND spec_alias.entity_kind = 'spec'
 AND spec_alias.entity_id = tasks.spec_id
 AND spec_alias.namespace = 'spec'
WHERE tasks.project_id = ?
  AND spec_alias.alias = ?
GROUP BY tasks.status
`, projectID, alias)
	if err != nil {
		return SpecTaskCounts{}, fmt.Errorf("query spec task counts: %w", err)
	}
	defer rows.Close()

	var counts SpecTaskCounts
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return SpecTaskCounts{}, fmt.Errorf("scan spec task count: %w", err)
		}
		switch status {
		case "done", "archived":
			counts.Done += count
		case "in_progress":
			counts.InProgress += count
		default:
			counts.Todo += count
		}
	}
	if err := rows.Err(); err != nil {
		return SpecTaskCounts{}, fmt.Errorf("iterate spec task counts: %w", err)
	}
	return counts, nil
}

// SpecStatusOrder returns the display order for known spec statuses.
func SpecStatusOrder() []string {
	return append([]string{}, specStatusOrder...)
}
