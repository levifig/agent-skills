package state

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/levifig/loaf/internal/project"
)

// DurableRenderOptions describes a scratch durable render request.
type DurableRenderOptions struct {
	Kind   string
	Ref    string
	Branch string
}

// DurableRenderResult describes a rendered scratch durable document.
type DurableRenderResult struct {
	ContractVersion    int    `json:"contract_version,omitempty"`
	DatabaseScope      string `json:"database_scope,omitempty"`
	DatabasePath       string `json:"database_path,omitempty"`
	ProjectID          string `json:"project_id,omitempty"`
	ProjectName        string `json:"project_name,omitempty"`
	ProjectCurrentPath string `json:"project_current_path,omitempty"`
	Kind               string `json:"kind"`
	Ref                string `json:"ref"`
	Title              string `json:"title,omitempty"`
	Branch             string `json:"branch"`
	Path               string `json:"path"`
	ContentHash        string `json:"content_hash"`
	Contract           string `json:"contract"`
}

// DurableFinalizeOptions describes a tracked durable render write request.
type DurableFinalizeOptions struct {
	Kind string
	Ref  string
}

// DurableFinalizeResult describes a deterministic render written to git.
type DurableFinalizeResult struct {
	ContractVersion    int    `json:"contract_version,omitempty"`
	DatabaseScope      string `json:"database_scope,omitempty"`
	DatabasePath       string `json:"database_path,omitempty"`
	ProjectID          string `json:"project_id,omitempty"`
	ProjectName        string `json:"project_name,omitempty"`
	ProjectCurrentPath string `json:"project_current_path,omitempty"`
	Kind               string `json:"kind"`
	Ref                string `json:"ref"`
	Title              string `json:"title,omitempty"`
	Path               string `json:"path"`
	RelativePath       string `json:"relative_path"`
	ContentHash        string `json:"content_hash"`
	Contract           string `json:"contract"`
}

// RenderDurableArtifact writes a deterministic scratch render under XDG cache.
func RenderDurableArtifact(ctx context.Context, root project.Root, resolver PathResolver, options DurableRenderOptions) (DurableRenderResult, error) {
	store, err := openInitializedStore(root, resolver)
	if err != nil {
		return DurableRenderResult{}, err
	}
	defer store.Close()
	return store.RenderDurableArtifact(ctx, root, resolver, options)
}

// RenderDurableArtifact writes a deterministic scratch render using an open store.
func (s *Store) RenderDurableArtifact(ctx context.Context, root project.Root, resolver PathResolver, options DurableRenderOptions) (DurableRenderResult, error) {
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return DurableRenderResult{}, err
	}
	identity, err := s.projectIdentity(ctx, projectID)
	if err != nil {
		return DurableRenderResult{}, err
	}
	kind := strings.TrimSpace(options.Kind)
	ref := strings.TrimSpace(options.Ref)
	if ref == "" {
		return DurableRenderResult{}, fmt.Errorf("durable render requires a ref")
	}

	doc, detail, err := s.durableRenderDocument(ctx, root, kind, ref)
	if err != nil {
		return DurableRenderResult{}, err
	}
	ref = firstNonEmpty(detail.Ref, ref)

	content, err := RenderDurableDocument(doc)
	if err != nil {
		return DurableRenderResult{}, err
	}
	branch := firstNonEmpty(strings.TrimSpace(options.Branch), ObservedGitBranch(root.Path()), "detached")
	path, err := durableRenderCacheFile(root, resolver, identity.ID, branch, kind, ref)
	if err != nil {
		return DurableRenderResult{}, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return DurableRenderResult{}, fmt.Errorf("create durable render cache directory: %w", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return DurableRenderResult{}, fmt.Errorf("write durable render %s: %w", path, err)
	}

	return DurableRenderResult{
		ContractVersion:    StateJSONContractVersion,
		DatabaseScope:      identity.DatabaseScope,
		DatabasePath:       identity.DatabasePath,
		ProjectID:          identity.ID,
		ProjectName:        identity.FriendlyName,
		ProjectCurrentPath: identity.CurrentPath,
		Kind:               kind,
		Ref:                ref,
		Title:              detail.Title,
		Branch:             branch,
		Path:               path,
		ContentHash:        artifactBodyHash(content),
		Contract:           DurableRenderContract,
	}, nil
}

// FinalizeDurableArtifact writes a deterministic durable render to its tracked git location.
func FinalizeDurableArtifact(ctx context.Context, root project.Root, resolver PathResolver, options DurableFinalizeOptions) (DurableFinalizeResult, error) {
	store, err := openInitializedStore(root, resolver)
	if err != nil {
		return DurableFinalizeResult{}, err
	}
	defer store.Close()
	return store.FinalizeDurableArtifact(ctx, root, options)
}

// FinalizeDurableArtifact writes a deterministic durable render using an open store.
func (s *Store) FinalizeDurableArtifact(ctx context.Context, root project.Root, options DurableFinalizeOptions) (DurableFinalizeResult, error) {
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return DurableFinalizeResult{}, err
	}
	identity, err := s.projectIdentity(ctx, projectID)
	if err != nil {
		return DurableFinalizeResult{}, err
	}
	kind := strings.TrimSpace(options.Kind)
	ref := strings.TrimSpace(options.Ref)
	if ref == "" {
		return DurableFinalizeResult{}, fmt.Errorf("durable finalize requires a ref")
	}
	doc, detail, err := s.durableRenderDocument(ctx, root, kind, ref)
	if err != nil {
		return DurableFinalizeResult{}, err
	}
	content, err := RenderDurableDocument(doc)
	if err != nil {
		return DurableFinalizeResult{}, err
	}
	path, rel, err := durableRenderGitFile(root, kind, firstNonEmpty(detail.Ref, ref), detail.Sources)
	if err != nil {
		return DurableFinalizeResult{}, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return DurableFinalizeResult{}, fmt.Errorf("create durable render target directory: %w", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return DurableFinalizeResult{}, fmt.Errorf("write durable render %s: %w", rel, err)
	}
	return DurableFinalizeResult{
		ContractVersion:    StateJSONContractVersion,
		DatabaseScope:      identity.DatabaseScope,
		DatabasePath:       identity.DatabasePath,
		ProjectID:          identity.ID,
		ProjectName:        identity.FriendlyName,
		ProjectCurrentPath: identity.CurrentPath,
		Kind:               kind,
		Ref:                firstNonEmpty(detail.Ref, ref),
		Title:              detail.Title,
		Path:               path,
		RelativePath:       rel,
		ContentHash:        artifactBodyHash(content),
		Contract:           DurableRenderContract,
	}, nil
}

type durableRenderDetail struct {
	Ref     string
	Title   string
	Sources []TraceSource
}

func (s *Store) durableRenderDocument(ctx context.Context, root project.Root, kind string, ref string) (DurableRenderDocument, durableRenderDetail, error) {
	switch kind {
	case "spec":
		result, err := s.ShowSpec(ctx, root, ref)
		if err != nil {
			return DurableRenderDocument{}, durableRenderDetail{}, err
		}
		return DurableSpecRenderDocument(result.Spec), durableRenderDetail{
			Ref:     firstNonEmpty(result.Spec.Alias, ref),
			Title:   result.Spec.Title,
			Sources: result.Spec.Sources,
		}, nil
	case "report":
		result, err := s.ShowReport(ctx, root, ref)
		if err != nil {
			return DurableRenderDocument{}, durableRenderDetail{}, err
		}
		return DurableReportRenderDocument(result.Report), durableRenderDetail{
			Ref:     firstNonEmpty(result.Report.Alias, ref),
			Title:   result.Report.Title,
			Sources: result.Report.Sources,
		}, nil
	default:
		return DurableRenderDocument{}, durableRenderDetail{}, fmt.Errorf("durable render kind %q is not supported", kind)
	}
}

func durableRenderCacheFile(root project.Root, resolver PathResolver, projectID string, branch string, kind string, ref string) (string, error) {
	return resolver.RenderCachePath(
		root,
		safeDurableRenderSegment(projectID),
		safeDurableRenderSegment(branch),
		safeDurableRenderSegment(kind)+"-"+safeDurableRenderSegment(ref)+".md",
	)
}

func durableRenderGitFile(root project.Root, kind string, ref string, sources []TraceSource) (string, string, error) {
	rel := ""
	for _, source := range sources {
		if strings.TrimSpace(source.Path) != "" {
			rel = filepath.ToSlash(strings.TrimSpace(source.Path))
			break
		}
	}
	if rel == "" {
		switch kind {
		case "spec":
			rel = filepath.ToSlash(filepath.Join(".agents", "specs", safeDurableRenderFilename(ref)+".md"))
		case "report":
			rel = filepath.ToSlash(filepath.Join(".agents", "reports", safeDurableRenderFilename(ref)+".md"))
		default:
			return "", "", fmt.Errorf("durable render kind %q is not supported", kind)
		}
	}
	if strings.HasPrefix(rel, "/") || strings.Contains(rel, "\x00") {
		return "", "", fmt.Errorf("durable render target %q must be a relative path", rel)
	}
	path := filepath.Join(root.Path(), filepath.FromSlash(rel))
	cleanRoot := filepath.Clean(root.Path())
	cleanPath := filepath.Clean(path)
	if !isWithinRoot(cleanPath, cleanRoot) {
		return "", "", fmt.Errorf("durable render target %q escapes project root", rel)
	}
	return cleanPath, filepath.ToSlash(rel), nil
}

func safeDurableRenderFilename(value string) string {
	value = strings.TrimSpace(value)
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	cleaned := strings.Trim(b.String(), "-")
	if cleaned == "" {
		return "render"
	}
	return cleaned
}

func safeDurableRenderSegment(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	cleaned := strings.Trim(b.String(), "-")
	if cleaned == "" {
		return "unknown"
	}
	return cleaned
}
