package state

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/levifig/loaf/internal/project"
)

// ListFileBackedReports reads reports from .agents/reports on disk.
func ListFileBackedReports(_ context.Context, root project.Root, options ReportListOptions) (ReportList, error) {
	reports := ReportList{Version: 1, Reports: map[string]ReportItem{}}
	agentsDir := filepath.Join(root.Path(), ".agents", "reports")
	patterns := []string{
		filepath.Join(agentsDir, "*.md"),
		filepath.Join(agentsDir, "archive", "*.md"),
	}
	for _, pattern := range patterns {
		files, err := filepath.Glob(pattern)
		if err != nil {
			return ReportList{}, fmt.Errorf("find file-backed reports: %w", err)
		}
		sort.Strings(files)
		for _, path := range files {
			item, alias, err := readFileBackedReportItem(root.Path(), path)
			if err != nil {
				return ReportList{}, err
			}
			if !reportMatchesFileBackedFilters(item, options) {
				continue
			}
			reports.Reports[alias] = item
		}
	}
	return reports, nil
}

func readFileBackedReportItem(rootPath, path string) (ReportItem, string, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return ReportItem{}, "", fmt.Errorf("read report %s: %w", path, err)
	}
	frontmatter := parseFrontmatterMap(body)
	stem := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	alias := firstNonEmptyString(frontmatter["id"], stem)
	status := firstNonEmptyString(frontmatter["status"], "draft")
	if strings.Contains(path, string(filepath.Separator)+"archive"+string(filepath.Separator)) {
		status = "archived"
	}
	status = LifecycleStatusForDisplay(LifecycleEntityReport, status)
	title := firstNonEmptyString(frontmatter["title"], alias)
	kind := firstNonEmptyString(frontmatter["type"], frontmatter["report_kind"], frontmatter["kind"], "markdown")
	rel, _ := filepath.Rel(rootPath, path)
	if rel == "." {
		rel = path
	}
	return ReportItem{
		Title:      title,
		Kind:       kind,
		Status:     status,
		SourcePath: filepath.ToSlash(rel),
	}, alias, nil
}

func reportMatchesFileBackedFilters(item ReportItem, options ReportListOptions) bool {
	if options.Type != "" && item.Kind != options.Type {
		return false
	}
	if options.Status != "" && item.Status != options.Status {
		return false
	}
	return true
}

func fileBackedHousekeepingSection(rootPath, relativeDir string, cleanupStatuses ...string) (HousekeepingSection, error) {
	cleanup := map[string]bool{}
	for _, status := range cleanupStatuses {
		cleanup[status] = true
	}
	dir := filepath.Join(rootPath, ".agents", relativeDir)
	section := HousekeepingSection{ByStatus: map[string]int{}}
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
			return nil
		}
		if relativeDir == "drafts" {
			body, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(rootPath, path)
			if rel == "." {
				rel = path
			}
			if !isShapingDraftArtifact(filepath.ToSlash(rel), parseFrontmatterMap(body)) {
				return nil
			}
		}
		status, err := fileBackedArtifactStatus(path)
		if err != nil {
			return err
		}
		if strings.Contains(path, string(filepath.Separator)+"archive"+string(filepath.Separator)) {
			status = "archived"
		}
		if status == "" {
			status = "unknown"
		}
		section.Total++
		section.ByStatus[status]++
		if cleanup[status] {
			section.CleanupCandidate++
		}
		return nil
	})
	if err != nil {
		if os.IsNotExist(err) {
			return section, nil
		}
		return HousekeepingSection{}, fmt.Errorf("scan file-backed housekeeping %s: %w", relativeDir, err)
	}
	return section, nil
}

func fileBackedArtifactStatus(path string) (string, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	frontmatter := parseFrontmatterMap(body)
	return strings.TrimSpace(frontmatter["status"]), nil
}

func fileBackedRecentReports(root project.Root) ([]releaseReadinessReport, error) {
	agentsDir := filepath.Join(root.Path(), ".agents", "reports")
	var files []string
	for _, pattern := range []string{filepath.Join(agentsDir, "*.md"), filepath.Join(agentsDir, "archive", "*.md")} {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			return nil, err
		}
		files = append(files, matches...)
	}
	sort.Slice(files, func(i, j int) bool {
		ai, _ := os.Stat(files[i])
		aj, _ := os.Stat(files[j])
		if ai == nil || aj == nil {
			return files[i] > files[j]
		}
		return ai.ModTime().After(aj.ModTime())
	})
	if len(files) > 5 {
		files = files[:5]
	}
	reports := make([]releaseReadinessReport, 0, len(files))
	for _, path := range files {
		item, _, err := readFileBackedReportItem(root.Path(), path)
		if err != nil {
			return nil, err
		}
		reports = append(reports, releaseReadinessReport{Title: item.Title, Kind: item.Kind, Status: item.Status})
	}
	return reports, nil
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func loadFileBackedReportDetail(root project.Root, ref string) (ReportDetail, error) {
	path, item, alias, err := findFileBackedReport(root.Path(), ref)
	if err != nil {
		return ReportDetail{}, err
	}
	bodyBytes, err := os.ReadFile(path)
	if err != nil {
		return ReportDetail{}, fmt.Errorf("read report %s: %w", path, err)
	}
	body := strings.TrimSpace(stripMarkdownFrontmatter(string(bodyBytes)))
	sourcePath := item.SourcePath
	if strings.Contains(sourcePath, "/.work/") {
		sourcePath = filepath.ToSlash(filepath.Join(".agents", "reports", alias+".md"))
	}
	return ReportDetail{
		ID:      alias,
		Alias:   alias,
		Title:   item.Title,
		Kind:    item.Kind,
		Status:  item.Status,
		Sources: []TraceSource{{Path: sourcePath}},
		Body:    body,
		HasBody: body != "",
	}, nil
}

func findFileBackedReport(rootPath, ref string) (string, ReportItem, string, error) {
	agentsDir := filepath.Join(rootPath, ".agents", "reports")
	patterns := []string{
		filepath.Join(agentsDir, ".work", "*.md"),
		filepath.Join(agentsDir, "*.md"),
		filepath.Join(agentsDir, "archive", "*.md"),
	}
	var matches []struct {
		path  string
		item  ReportItem
		alias string
	}
	for _, pattern := range patterns {
		files, err := filepath.Glob(pattern)
		if err != nil {
			return "", ReportItem{}, "", fmt.Errorf("find reports: %w", err)
		}
		sort.Strings(files)
		for _, path := range files {
			item, alias, err := readFileBackedReportItem(rootPath, path)
			if err != nil {
				return "", ReportItem{}, "", err
			}
			stem := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
			if ref == alias || ref == item.SourcePath || ref == stem || strings.Contains(stem, ref) || strings.Contains(alias, ref) {
				matches = append(matches, struct {
					path  string
					item  ReportItem
					alias string
				}{path, item, alias})
			}
		}
	}
	switch len(matches) {
	case 0:
		return "", ReportItem{}, "", fmt.Errorf("report %q not found", ref)
	case 1:
		return matches[0].path, matches[0].item, matches[0].alias, nil
	default:
		return "", ReportItem{}, "", fmt.Errorf("ambiguous report %q", ref)
	}
}

func stripMarkdownFrontmatter(content string) string {
	trimmed := strings.TrimSpace(content)
	if !strings.HasPrefix(trimmed, "---") {
		return content
	}
	rest := trimmed[3:]
	if strings.HasPrefix(rest, "\n") {
		rest = rest[1:]
	} else if strings.HasPrefix(rest, "\r\n") {
		rest = rest[2:]
	}
	idx := strings.Index(rest, "\n---")
	if idx < 0 {
		return content
	}
	body := rest[idx+4:]
	if strings.HasPrefix(body, "\n") {
		body = body[1:]
	} else if strings.HasPrefix(body, "\r\n") {
		body = body[2:]
	}
	return body
}

func (s *Store) enrichFileBackedTraceEntityFromProject(ctx context.Context, projectID string, entity *TraceEntity) {
	if entity == nil {
		return
	}
	identity, err := s.projectIdentity(ctx, projectID)
	if err != nil || identity.CurrentPath == "" {
		return
	}
	enrichFileBackedTraceEntity(identity.CurrentPath, entity)
}

func enrichFileBackedTraceEntity(rootPath string, entity *TraceEntity) {
	if entity == nil {
		return
	}
	ref := strings.TrimSpace(entity.Alias)
	if ref == "" {
		return
	}
	switch entity.Kind {
	case "report":
		_, item, _, err := findFileBackedReport(rootPath, ref)
		if err != nil {
			return
		}
		if entity.Title == "" {
			entity.Title = item.Title
		}
		if entity.Status == "" {
			entity.Status = item.Status
		}
	case "council":
		path := filepath.Join(rootPath, ".agents", "councils", ref+".md")
		body, err := os.ReadFile(path)
		if err != nil {
			return
		}
		frontmatter := parseFrontmatterMap(body)
		if entity.Title == "" {
			entity.Title = firstNonEmptyString(frontmatter["title"], ref)
		}
		if entity.Status == "" {
			entity.Status = strings.TrimSpace(frontmatter["status"])
		}
	case "shaping_draft":
		dir := filepath.Join(rootPath, ".agents", "drafts")
		_ = filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
			if err != nil || entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
				return err
			}
			body, readErr := os.ReadFile(path)
			if readErr != nil {
				return nil
			}
			rel, _ := filepath.Rel(rootPath, path)
			frontmatter := parseFrontmatterMap(body)
			if !isShapingDraftArtifact(filepath.ToSlash(rel), frontmatter) {
				return nil
			}
			stem := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
			id := firstNonEmptyString(frontmatter["id"], stem)
			if id != ref && stem != ref {
				return nil
			}
			if entity.Title == "" {
				entity.Title = firstNonEmptyString(frontmatter["title"], stem)
			}
			if entity.Status == "" {
				entity.Status = strings.TrimSpace(frontmatter["status"])
			}
			return fs.SkipAll
		})
	}
}
