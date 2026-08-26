package state

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/levifig/loaf/internal/project"
)

func (s *Store) searchFileBackedReports(ctx context.Context, root project.Root, projectID string, allProjects bool, queryTokens []string) ([]SearchHit, error) {
	if len(queryTokens) == 0 {
		return nil, nil
	}
	if allProjects {
		return nil, nil
	}
	reportsDir := filepath.Join(root.Path(), ".agents", "reports")
	patterns := []string{
		filepath.Join(reportsDir, ".work", "*.md"),
		filepath.Join(reportsDir, "*.md"),
	}
	var hits []SearchHit
	seen := map[string]bool{}
	for _, pattern := range patterns {
		files, err := filepath.Glob(pattern)
		if err != nil {
			return nil, fmt.Errorf("find file-backed reports: %w", err)
		}
		for _, path := range files {
			contentBytes, err := os.ReadFile(path)
			if err != nil {
				return nil, fmt.Errorf("read report %s: %w", path, err)
			}
			content := string(contentBytes)
			lowered := strings.ToLower(content)
			matched := false
			for _, token := range queryTokens {
				if strings.Contains(lowered, strings.ToLower(token)) {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
			item, alias, err := readFileBackedReportItem(root.Path(), path)
			if err != nil {
				return nil, err
			}
			if seen[alias] {
				continue
			}
			seen[alias] = true
			snippet := fileBackedReportSnippet(content, queryTokens)
			hits = append(hits, SearchHit{
				Tier:       "tier1",
				Source:     "artifact_body",
				ProjectID:  projectID,
				EntityKind: "report",
				EntityID:   alias,
				BodyKind:   ArtifactBodyKindMarkdown,
				Locator:    "report/" + alias + "#" + ArtifactBodyKindMarkdown,
				Snippet:    redactSearchSnippet(snippet),
				Rank:       -1,
			})
			_ = item
		}
	}
	return hits, nil
}

func fileBackedReportSnippet(content string, tokens []string) string {
	body := markdownContentWithoutFrontmatterForSearch(content)
	for _, line := range strings.Split(body, "\n") {
		lowered := strings.ToLower(line)
		for _, token := range tokens {
			if strings.Contains(lowered, strings.ToLower(token)) {
				return strings.TrimSpace(line)
			}
		}
	}
	trimmed := strings.TrimSpace(body)
	if len(trimmed) > 120 {
		return trimmed[:120] + "..."
	}
	return trimmed
}

func markdownContentWithoutFrontmatterForSearch(content string) string {
	trimmed := strings.TrimSpace(content)
	if !strings.HasPrefix(trimmed, "---\n") {
		return trimmed
	}
	parts := strings.SplitN(trimmed, "\n---\n", 2)
	if len(parts) != 2 {
		return trimmed
	}
	return strings.TrimSpace(parts[1])
}
