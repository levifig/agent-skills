package cli

import (
	"context"
	"fmt"
	"os"
	"sort"
	"path/filepath"
	"strings"
	"time"

	"github.com/levifig/loaf/internal/project"
	"github.com/levifig/loaf/internal/state"
)

func markdownCouncilNew(ctx context.Context, root project.Root, resolver state.PathResolver, options state.ArtifactEntityCreateOptions) (state.ArtifactEntityCreateResult, error) {
	title := strings.TrimSpace(options.Title)
	if title == "" {
		return state.ArtifactEntityCreateResult{}, fmt.Errorf("council new requires --title")
	}
	if strings.TrimSpace(options.Body) == "" {
		return state.ArtifactEntityCreateResult{}, fmt.Errorf("council new requires body content")
	}
	status, err := state.Inspect(root, resolver)
	if err != nil {
		return state.ArtifactEntityCreateResult{}, err
	}
	if status.Mode == state.ModeInvalid {
		return state.ArtifactEntityCreateResult{}, fmt.Errorf("state database is invalid; run `loaf state doctor`")
	}
	alias := councilAliasFromTitle(title, time.Now().UTC())
	councilsDir := filepath.Join(root.Path(), ".agents", "councils")
	if err := os.MkdirAll(councilsDir, 0o755); err != nil {
		return state.ArtifactEntityCreateResult{}, fmt.Errorf("create councils directory: %w", err)
	}
	path := filepath.Join(councilsDir, alias+".md")
	if _, err := os.Stat(path); err == nil {
		return state.ArtifactEntityCreateResult{}, fmt.Errorf("council file %s already exists", alias)
	} else if err != nil && !os.IsNotExist(err) {
		return state.ArtifactEntityCreateResult{}, fmt.Errorf("inspect council file %s: %w", alias, err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	frontmatter := map[string]frontmatterField{
		"id":      markdownReportFrontmatterScalar(alias),
		"title":   markdownReportFrontmatterScalar(title),
		"status":  markdownReportFrontmatterScalar("draft"),
		"created": markdownReportFrontmatterScalar(now),
	}
	if err := os.WriteFile(path, []byte(renderMarkdownReport(frontmatter, options.Body)), 0o600); err != nil {
		return state.ArtifactEntityCreateResult{}, fmt.Errorf("write council %s: %w", alias, err)
	}
	if status.Mode == state.ModeSQLiteReady {
		if err := state.RegisterFileBackedEntityAlias(ctx, root, resolver, "council", alias, title); err != nil {
			return state.ArtifactEntityCreateResult{}, err
		}
	}
	return state.ArtifactEntityCreateResult{
		ContractVersion:    state.StateJSONContractVersion,
		DatabaseScope:      status.DatabaseScope,
		DatabasePath:       status.DatabasePath,
		ProjectID:          status.ProjectID,
		ProjectName:        status.ProjectName,
		ProjectCurrentPath: status.ProjectCurrentPath,
		Entity:             state.TraceEntity{Kind: "council", ID: alias, Alias: alias, Title: title, Status: "draft"},
	}, nil
}

func markdownCouncilShow(rootPath, ref string) (state.ArtifactEntityShow, error) {
	path, err := resolveMarkdownCouncilFile(rootPath, ref)
	if err != nil {
		return state.ArtifactEntityShow{}, err
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return state.ArtifactEntityShow{}, fmt.Errorf("read council %s: %w", ref, err)
	}
	frontmatter, _ := parseKnowledgeFrontmatter(content)
	alias := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	if id := firstFieldValue(frontmatter["id"]); id != "" {
		alias = id
	}
	title := firstNonEmpty(firstFieldValue(frontmatter["title"]), alias)
	status := state.LifecycleStatusForDisplay(state.LifecycleEntityCouncil, firstNonEmpty(firstFieldValue(frontmatter["status"]), "draft"))
	body := strings.TrimSpace(markdownContentWithoutFrontmatter(string(content)))
	rel, _ := filepath.Rel(rootPath, path)
	if rel == "." {
		rel = path
	}
	return state.ArtifactEntityShow{
		Query: ref,
		Entity: state.ArtifactEntityDetail{
			ID:      alias,
			Kind:    "council",
			Alias:   alias,
			Title:   title,
			Status:  status,
			Sources: []state.TraceSource{{Path: filepath.ToSlash(rel)}},
			Body:    body,
		},
	}, nil
}

func councilAliasFromTitle(title string, now time.Time) string {
	slug := sanitizeMarkdownReportPathSegment(strings.ToLower(strings.TrimSpace(title)))
	if slug == "" {
		slug = "council"
	}
	return strings.ToUpper("council") + "-" + now.Format("20060102") + "-" + slug
}

func resolveMarkdownCouncilFile(rootPath, ref string) (string, error) {
	councilsDir := filepath.Join(rootPath, ".agents", "councils")
	direct := filepath.Join(councilsDir, ref+".md")
	if _, err := os.Stat(direct); err == nil {
		return direct, nil
	}
	files, err := filepath.Glob(filepath.Join(councilsDir, "*.md"))
	if err != nil {
		return "", fmt.Errorf("find councils: %w", err)
	}
	for _, path := range files {
		if strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)) == ref || strings.Contains(filepath.Base(path), ref) {
			return path, nil
		}
	}
	return "", fmt.Errorf("council %q not found", ref)
}

func markdownCouncilList(rootPath string, options state.ArtifactEntityListOptions) (state.ArtifactEntityList, error) {
	result := state.ArtifactEntityList{
		Kind:     "council",
		Entities: map[string]state.ArtifactEntityItem{},
	}
	files, err := filepath.Glob(filepath.Join(rootPath, ".agents", "councils", "*.md"))
	if err != nil {
		return state.ArtifactEntityList{}, fmt.Errorf("find councils: %w", err)
	}
	archived, err := filepath.Glob(filepath.Join(rootPath, ".agents", "councils", "archive", "*.md"))
	if err != nil {
		return state.ArtifactEntityList{}, fmt.Errorf("find archived councils: %w", err)
	}
	files = append(files, archived...)
	sort.Strings(files)
	for _, path := range files {
		content, err := os.ReadFile(path)
		if err != nil {
			return state.ArtifactEntityList{}, fmt.Errorf("read council %s: %w", path, err)
		}
		frontmatter, _ := parseKnowledgeFrontmatter(content)
		alias := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		if id := firstFieldValue(frontmatter["id"]); id != "" {
			alias = id
		}
		title := firstNonEmpty(firstFieldValue(frontmatter["title"]), alias)
		status := firstNonEmpty(firstFieldValue(frontmatter["status"]), "draft")
		if strings.Contains(path, string(filepath.Separator)+"archive"+string(filepath.Separator)) {
			status = "archived"
		}
		if !state.LifecycleStatusFilterMatches("council", status, options.Status) {
			continue
		}
		if !options.All && state.LifecycleStatusMatches("council", status, state.LifecycleStatusArchived) {
			continue
		}
		result.Entities[alias] = state.ArtifactEntityItem{
			Title:  title,
			Status: state.LifecycleStatusForDisplay(state.LifecycleEntityCouncil, status),
		}
	}
	return result, nil
}
