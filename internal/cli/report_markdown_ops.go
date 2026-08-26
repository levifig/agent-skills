package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/levifig/loaf/internal/project"
	"github.com/levifig/loaf/internal/state"
)

const reportWorkSubdir = ".work"

func enrichReportEdit(status state.Status, result *state.ReportEditResult) {
	if status.Mode == state.ModeMarkdownOnly {
		return
	}
	result.ContractVersion = state.StateJSONContractVersion
	result.DatabaseScope = status.DatabaseScope
	result.DatabasePath = status.DatabasePath
	result.ProjectID = status.ProjectID
	result.ProjectName = status.ProjectName
	result.ProjectCurrentPath = status.ProjectCurrentPath
}

func reportAliasFromSlug(slug string) string {
	normalized := sanitizeMarkdownReportPathSegment(strings.ToLower(strings.TrimSpace(slug)))
	if normalized == "" {
		return ""
	}
	return "report-" + normalized
}

func reportWorkPath(rootPath, alias string) string {
	return filepath.Join(rootPath, ".agents", "reports", reportWorkSubdir, alias+".md")
}

func markdownReportCreate(rootPath string, options state.ReportCreateOptions) (state.ReportCreateResult, error) {
	slug := strings.TrimSpace(options.Slug)
	if slug == "" {
		return state.ReportCreateResult{}, fmt.Errorf("report create requires a slug")
	}
	kind := strings.TrimSpace(options.Kind)
	if kind == "" {
		kind = "research"
	}
	source := strings.TrimSpace(options.Source)
	if source == "" {
		source = "ad-hoc"
	}
	reportsDir := filepath.Join(rootPath, ".agents", "reports")
	if err := os.MkdirAll(reportsDir, 0o755); err != nil {
		return state.ReportCreateResult{}, fmt.Errorf("create reports directory: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	title := markdownReportTitleFromSlug(slug)

	var path string
	var alias string
	if options.SetBody {
		alias = reportAliasFromSlug(slug)
		if alias == "" {
			return state.ReportCreateResult{}, fmt.Errorf("report create requires a slug")
		}
		workDir := filepath.Join(reportsDir, reportWorkSubdir)
		if err := os.MkdirAll(workDir, 0o755); err != nil {
			return state.ReportCreateResult{}, fmt.Errorf("create report work directory: %w", err)
		}
		path = reportWorkPath(rootPath, alias)
		if _, err := os.Stat(path); err == nil {
			return state.ReportCreateResult{}, fmt.Errorf("report %s already exists", alias)
		} else if err != nil && !os.IsNotExist(err) {
			return state.ReportCreateResult{}, fmt.Errorf("inspect report work file %s: %w", alias, err)
		}
	} else {
		safeType := sanitizeMarkdownReportPathSegment(kind)
		safeSlug := sanitizeMarkdownReportPathSegment(slug)
		if safeSlug == "" {
			return state.ReportCreateResult{}, fmt.Errorf("report create requires a slug")
		}
		filename := fmt.Sprintf("%s-%s-%s.md", time.Now().Format("20060102-150405"), safeType, safeSlug)
		path = filepath.Join(reportsDir, filename)
		if _, err := os.Stat(path); err == nil {
			return state.ReportCreateResult{}, fmt.Errorf("report file %s already exists", filename)
		} else if err != nil && !os.IsNotExist(err) {
			return state.ReportCreateResult{}, fmt.Errorf("inspect report file %s: %w", filename, err)
		}
		alias = strings.TrimSuffix(filename, filepath.Ext(filename))
	}

	body := markdownReportBody(title)
	if options.SetBody {
		body = strings.TrimSpace(options.Body)
		if body == "" {
			body = markdownReportBody(title)
		}
	}
	frontmatter := map[string]frontmatterField{
		"id":      markdownReportFrontmatterScalar(alias),
		"title":   markdownReportFrontmatterScalar(title),
		"type":    markdownReportFrontmatterScalar(kind),
		"created": markdownReportFrontmatterScalar(now),
		"status":  markdownReportFrontmatterScalar("draft"),
		"source":  markdownReportFrontmatterScalar(source),
		"tags":    {Array: true, Set: true},
	}
	if err := os.WriteFile(path, []byte(renderMarkdownReport(frontmatter, body)), 0o600); err != nil {
		return state.ReportCreateResult{}, fmt.Errorf("write report %s: %w", filepath.Base(path), err)
	}
	return state.ReportCreateResult{
		Report: state.TraceEntity{
			Kind:   "report",
			ID:     alias,
			Alias:  alias,
			Title:  title,
			Status: "draft",
		},
		Kind:   kind,
		Source: source,
	}, nil
}

func markdownReportEdit(rootPath, ref, body string, force bool) (state.ReportEditResult, error) {
	_ = force
	path, item, alias, err := findMarkdownReportIncludingWork(rootPath, ref)
	if err != nil {
		return state.ReportEditResult{}, err
	}
	if item.Status == state.LifecycleStatusArchived {
		return state.ReportEditResult{}, fmt.Errorf("report %q is archived and cannot be edited", firstNonEmpty(alias, ref))
	}
	frontmatter, _, _, err := readMarkdownReportDocument(path)
	if err != nil {
		return state.ReportEditResult{}, err
	}
	if err := os.WriteFile(path, []byte(renderMarkdownReport(frontmatter, body)), 0o600); err != nil {
		return state.ReportEditResult{}, fmt.Errorf("write edited report %s: %w", alias, err)
	}
	hash := sha256.Sum256([]byte(body))
	return state.ReportEditResult{
		Report:      state.TraceEntity{Kind: "report", ID: alias, Alias: alias, Title: item.Title, Status: item.Status},
		ContentHash: hex.EncodeToString(hash[:]),
	}, nil
}

func markdownReportFinalize(ctx context.Context, root project.Root, resolver state.PathResolver, ref string) (state.ReportStatusResult, error) {
	_ = ctx
	_ = resolver
	rootPath := root.Path()
	path, item, alias, err := findMarkdownReportIncludingWork(rootPath, ref)
	if err != nil {
		return state.ReportStatusResult{}, err
	}
	frontmatter, body, docAlias, err := readMarkdownReportDocument(path)
	if err != nil {
		return state.ReportStatusResult{}, err
	}
	if docAlias != "" {
		alias = docAlias
	}
	previousRaw := firstNonEmpty(firstFieldValue(frontmatter["status"]), "draft")
	previous := state.LifecycleStatusForDisplay(state.LifecycleEntityReport, previousRaw)
	title := firstNonEmpty(firstFieldValue(frontmatter["title"]), firstMarkdownHeading(body), alias)

	isWorkDraft := strings.Contains(path, string(filepath.Separator)+reportWorkSubdir+string(filepath.Separator))

	if state.LifecycleStatusMatches(state.LifecycleEntityReport, previousRaw, state.LifecycleStatusDone) {
		result := state.ReportStatusResult{
			Report:   state.TraceEntity{Kind: "report", ID: alias, Alias: alias, Title: title, Status: state.LifecycleStatusDone},
			Previous: previous,
			Status:   state.LifecycleStatusDone,
		}
		if isWorkDraft {
			render, err := writeReportDurableRender(rootPath, alias, title, item.Kind, body)
			if err != nil {
				return state.ReportStatusResult{}, err
			}
			result.Render = &render
		}
		return result, nil
	}
	if !state.LifecycleStatusMatches(state.LifecycleEntityReport, previousRaw, state.LifecycleStatusDraft) {
		return state.ReportStatusResult{}, fmt.Errorf("report %q is not draft (status: %s)", ref, previous)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	frontmatter["status"] = markdownReportFrontmatterScalar(state.LifecycleStatusDone)
	frontmatter["finalized_at"] = markdownReportFrontmatterScalar(now)
	if err := os.WriteFile(path, []byte(renderMarkdownReport(frontmatter, body)), 0o600); err != nil {
		return state.ReportStatusResult{}, fmt.Errorf("write finalized report %s: %w", ref, err)
	}
	result := state.ReportStatusResult{
		Report:   state.TraceEntity{Kind: "report", ID: alias, Alias: alias, Title: title, Status: state.LifecycleStatusDone},
		Previous: previous,
		Status:   state.LifecycleStatusDone,
	}
	if isWorkDraft {
		render, err := writeReportDurableRender(rootPath, alias, title, item.Kind, body)
		if err != nil {
			return state.ReportStatusResult{}, err
		}
		result.Render = &render
	}
	return result, nil
}

func writeReportDurableRender(rootPath, alias, title, kind, body string) (state.DurableFinalizeResult, error) {
	doc := state.DurableReportRenderDocument(state.ReportDetail{
		ID:      alias,
		Alias:   alias,
		Title:   title,
		Kind:    kind,
		Status:  state.LifecycleStatusDone,
		Body:    body,
		HasBody: strings.TrimSpace(body) != "",
	})
	content, err := state.RenderDurableDocument(doc)
	if err != nil {
		return state.DurableFinalizeResult{}, err
	}
	rel := filepath.ToSlash(filepath.Join(".agents", "reports", alias+".md"))
	path := filepath.Join(rootPath, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return state.DurableFinalizeResult{}, fmt.Errorf("create durable render directory: %w", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return state.DurableFinalizeResult{}, fmt.Errorf("write durable render %s: %w", rel, err)
	}
	return state.DurableFinalizeResult{
		Kind:         "report",
		Ref:          alias,
		Title:        title,
		Path:         path,
		RelativePath: rel,
		ContentHash:  artifactBodyHash(content),
		Contract:     state.DurableRenderContract,
	}, nil
}

func artifactBodyHash(content string) string {
	hash := sha256.Sum256([]byte(content))
	return hex.EncodeToString(hash[:])
}

func findMarkdownReportIncludingWork(rootPath, ref string) (string, state.ReportItem, string, error) {
	if path, item, alias, err := findMarkdownReportWork(rootPath, ref); err == nil {
		return path, item, alias, nil
	}
	return findMarkdownReport(rootPath, ref)
}

func findMarkdownReportWork(rootPath, ref string) (string, state.ReportItem, string, error) {
	workDir := filepath.Join(rootPath, ".agents", "reports", reportWorkSubdir)
	if strings.TrimSpace(ref) == "" {
		return "", state.ReportItem{}, "", fmt.Errorf("report ref is required")
	}
	candidates := []string{filepath.Join(workDir, ref+".md")}
	if alias := reportAliasFromSlug(ref); alias != "" {
		candidates = append(candidates, filepath.Join(workDir, alias+".md"))
	}
	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			item, alias, err := readMarkdownReport(rootPath, path)
			if err != nil {
				return "", state.ReportItem{}, "", err
			}
			return path, item, alias, nil
		}
	}
	files, err := filepath.Glob(filepath.Join(workDir, "*.md"))
	if err != nil {
		return "", state.ReportItem{}, "", fmt.Errorf("find work reports: %w", err)
	}
	for _, path := range files {
		item, alias, err := readMarkdownReport(rootPath, path)
		if err != nil {
			return "", state.ReportItem{}, "", err
		}
		if ref == alias || ref == item.SourcePath || strings.Contains(filepath.Base(path), ref) {
			return path, item, alias, nil
		}
	}
	return "", state.ReportItem{}, "", fmt.Errorf("report %q not found", ref)
}
