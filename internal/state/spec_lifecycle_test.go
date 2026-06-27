package state

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreateSpecStoresBodyAndAllocatesAlias(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	created, err := CreateSpec(context.Background(), root, PathResolver{StateHome: stateHome}, SpecCreateOptions{
		Slug:    "token-rotation",
		Title:   "Token Rotation",
		Body:    "# Token Rotation\n\nRotate tokens nightly.",
		SetBody: true,
	})
	if err != nil {
		t.Fatalf("CreateSpec() error = %v", err)
	}
	if created.Spec.Alias != "SPEC-001" || created.Spec.Title != "Token Rotation" || created.Spec.Status != "draft" || created.EventID == "" {
		t.Fatalf("created = %#v, want draft SPEC-001 metadata", created)
	}

	show, err := ShowSpec(context.Background(), root, PathResolver{StateHome: stateHome}, "SPEC-001")
	if err != nil {
		t.Fatalf("ShowSpec() error = %v", err)
	}
	if show.Spec.Body != "# Token Rotation\n\nRotate tokens nightly." {
		t.Fatalf("Body = %q, want byte-exact spec body", show.Spec.Body)
	}
	if !show.Spec.HasBody {
		t.Fatalf("HasBody = false, want true")
	}
	if show.Spec.Title != "Token Rotation" || show.Spec.Status != "draft" || show.Spec.Alias != "SPEC-001" {
		t.Fatalf("Spec = %#v, want created spec metadata", show.Spec)
	}

	specs, err := ListSpecs(context.Background(), root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("ListSpecs() error = %v", err)
	}
	if _, ok := specs.Specs["SPEC-001"]; !ok {
		t.Fatalf("ListSpecs() = %#v, want SPEC-001 present", specs.Specs)
	}
}

func TestCreateSpecAutoAllocatesAcrossSQLiteAndFiles(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	first, err := CreateSpec(context.Background(), root, PathResolver{StateHome: stateHome}, SpecCreateOptions{Slug: "one"})
	if err != nil {
		t.Fatalf("CreateSpec(one) error = %v", err)
	}
	if first.Spec.Alias != "SPEC-001" {
		t.Fatalf("first alias = %q, want SPEC-001", first.Spec.Alias)
	}

	// A render file on disk for a higher id must lift the allocation watermark.
	specsDir := filepath.Join(root.Path(), ".agents", "specs")
	if err := os.MkdirAll(specsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(specsDir, "SPEC-014-legacy.md"), []byte("legacy"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	next, err := CreateSpec(context.Background(), root, PathResolver{StateHome: stateHome}, SpecCreateOptions{Slug: "two"})
	if err != nil {
		t.Fatalf("CreateSpec(two) error = %v", err)
	}
	if next.Spec.Alias != "SPEC-015" {
		t.Fatalf("next alias = %q, want SPEC-015 (max of SQLite SPEC-001 and file SPEC-014, +1)", next.Spec.Alias)
	}
}

func TestCreateSpecExplicitIDAndDuplicateRejection(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	created, err := CreateSpec(context.Background(), root, PathResolver{StateHome: stateHome}, SpecCreateOptions{Slug: "explicit", ID: "SPEC-042"})
	if err != nil {
		t.Fatalf("CreateSpec(explicit) error = %v", err)
	}
	if created.Spec.Alias != "SPEC-042" {
		t.Fatalf("alias = %q, want SPEC-042", created.Spec.Alias)
	}

	if _, err := CreateSpec(context.Background(), root, PathResolver{StateHome: stateHome}, SpecCreateOptions{Slug: "dup", ID: "SPEC-042"}); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("CreateSpec(duplicate) error = %v, want already exists", err)
	}

	if _, err := CreateSpec(context.Background(), root, PathResolver{StateHome: stateHome}, SpecCreateOptions{Slug: "bad", ID: "SPEC-XYZ"}); err == nil || !strings.Contains(err.Error(), "invalid spec id") {
		t.Fatalf("CreateSpec(invalid id) error = %v, want invalid spec id", err)
	}
}

func TestCreateSpecStoresBranchSourceAndRelated(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	// Pre-existing specs to relate to.
	if _, err := CreateSpec(context.Background(), root, PathResolver{StateHome: stateHome}, SpecCreateOptions{Slug: "alpha", ID: "SPEC-001"}); err != nil {
		t.Fatalf("CreateSpec(alpha) error = %v", err)
	}
	if _, err := CreateSpec(context.Background(), root, PathResolver{StateHome: stateHome}, SpecCreateOptions{Slug: "beta", ID: "SPEC-002"}); err != nil {
		t.Fatalf("CreateSpec(beta) error = %v", err)
	}

	created, err := CreateSpec(context.Background(), root, PathResolver{StateHome: stateHome}, SpecCreateOptions{
		Slug:    "gamma",
		ID:      "SPEC-003",
		Source:  "SPARK-42",
		Branch:  "feat/gamma",
		Related: []string{"SPEC-001", "SPEC-002"},
	})
	if err != nil {
		t.Fatalf("CreateSpec(gamma) error = %v", err)
	}
	if created.Spec.Alias != "SPEC-003" {
		t.Fatalf("created alias = %q, want SPEC-003", created.Spec.Alias)
	}

	show, err := ShowSpec(context.Background(), root, PathResolver{StateHome: stateHome}, "SPEC-003")
	if err != nil {
		t.Fatalf("ShowSpec(SPEC-003) error = %v", err)
	}
	if show.Spec.Branch != "feat/gamma" {
		t.Fatalf("Branch = %q, want feat/gamma", show.Spec.Branch)
	}
	if show.Spec.Source != "SPARK-42" {
		t.Fatalf("Source = %q, want SPARK-42", show.Spec.Source)
	}
	if !hasRelatedSpec(show.Spec.Related, "SPEC-001") || !hasRelatedSpec(show.Spec.Related, "SPEC-002") {
		t.Fatalf("Related = %#v, want SPEC-001 and SPEC-002 resolved", show.Spec.Related)
	}
	// The related_to relationships must be created and resolvable from the trace graph.
	if !hasStateTraceRelationship(show.Spec.Relationships, "outbound", "related_to", "spec", "SPEC-001") {
		t.Fatalf("Relationships = %#v, want outbound related_to SPEC-001", show.Spec.Relationships)
	}
	if !hasStateTraceRelationship(show.Spec.Relationships, "outbound", "related_to", "spec", "SPEC-002") {
		t.Fatalf("Relationships = %#v, want outbound related_to SPEC-002", show.Spec.Relationships)
	}

	// The relationship is symmetric and resolvable from the related spec.
	related, err := ShowSpec(context.Background(), root, PathResolver{StateHome: stateHome}, "SPEC-001")
	if err != nil {
		t.Fatalf("ShowSpec(SPEC-001) error = %v", err)
	}
	if !hasStateTraceRelationship(related.Spec.Relationships, "inbound", "related_to", "spec", "SPEC-003") {
		t.Fatalf("SPEC-001 Relationships = %#v, want inbound related_to SPEC-003", related.Spec.Relationships)
	}
	if !hasRelatedSpec(related.Spec.Related, "SPEC-003") {
		t.Fatalf("SPEC-001 Related = %#v, want SPEC-003 (symmetric)", related.Spec.Related)
	}
}

func TestCreateSpecWithoutBranchSourceRelatedUsesDefaults(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	if _, err := CreateSpec(context.Background(), root, PathResolver{StateHome: stateHome}, SpecCreateOptions{Slug: "plain"}); err != nil {
		t.Fatalf("CreateSpec(plain) error = %v", err)
	}

	show, err := ShowSpec(context.Background(), root, PathResolver{StateHome: stateHome}, "SPEC-001")
	if err != nil {
		t.Fatalf("ShowSpec() error = %v", err)
	}
	if show.Spec.Branch != "" {
		t.Fatalf("Branch = %q, want empty", show.Spec.Branch)
	}
	if show.Spec.Source != "ad-hoc" {
		t.Fatalf("Source = %q, want ad-hoc default", show.Spec.Source)
	}
	if len(show.Spec.Related) != 0 {
		t.Fatalf("Related = %#v, want empty", show.Spec.Related)
	}
}

func TestCreateSpecRejectsNonSpecRelated(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	_, err := CreateSpec(context.Background(), root, PathResolver{StateHome: stateHome}, SpecCreateOptions{
		Slug:    "needs-missing",
		Related: []string{"SPEC-404"},
	})
	if err == nil || !strings.Contains(err.Error(), "SPEC-404") {
		t.Fatalf("CreateSpec(missing related) error = %v, want unresolved related spec", err)
	}
}

func hasRelatedSpec(related []TraceEntity, alias string) bool {
	for _, entity := range related {
		if entity.Alias == alias {
			return true
		}
	}
	return false
}

func TestCreateSpecFinalizeRendersSlugFilename(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	created, err := CreateSpec(context.Background(), root, PathResolver{StateHome: stateHome}, SpecCreateOptions{
		Slug:    "render-target",
		Title:   "Render Target",
		Body:    "Body for render.",
		SetBody: true,
	})
	if err != nil {
		t.Fatalf("CreateSpec() error = %v", err)
	}

	render, err := FinalizeDurableArtifact(context.Background(), root, PathResolver{StateHome: stateHome}, DurableFinalizeOptions{
		Kind: "spec",
		Ref:  created.Spec.Alias,
	})
	if err != nil {
		t.Fatalf("FinalizeDurableArtifact() error = %v", err)
	}
	wantRel := ".agents/specs/SPEC-001-render-target.md"
	if render.RelativePath != wantRel {
		t.Fatalf("RelativePath = %q, want %q", render.RelativePath, wantRel)
	}
	content, err := os.ReadFile(render.Path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", render.Path, err)
	}
	if !strings.Contains(string(content), "Body for render.") {
		t.Fatalf("rendered content missing body:\n%s", content)
	}
}
