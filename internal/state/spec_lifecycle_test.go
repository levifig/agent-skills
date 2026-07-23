package state

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/levifig/loaf/internal/project"
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

func TestEditSpecBodyUpdatesBodyAndRecordsEvent(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	created, err := CreateSpec(context.Background(), root, PathResolver{StateHome: stateHome}, SpecCreateOptions{
		Slug:    "edit-target",
		Title:   "Edit Target",
		Body:    "Original body.",
		SetBody: true,
	})
	if err != nil {
		t.Fatalf("CreateSpec() error = %v", err)
	}

	edited, err := EditSpecBody(context.Background(), root, PathResolver{StateHome: stateHome}, SpecEditOptions{
		Ref:  "SPEC-001",
		Body: "Edited body.",
	})
	if err != nil {
		t.Fatalf("EditSpecBody() error = %v", err)
	}
	if edited.Spec.Alias != "SPEC-001" || edited.Imported || edited.EventID == "" || edited.ContentHash == "" {
		t.Fatalf("edited = %#v, want non-imported edit with event id and content hash", edited)
	}

	show, err := ShowSpec(context.Background(), root, PathResolver{StateHome: stateHome}, "SPEC-001")
	if err != nil {
		t.Fatalf("ShowSpec() error = %v", err)
	}
	if show.Spec.Body != "Edited body." || !show.Spec.HasBody {
		t.Fatalf("Spec = %#v, want edited body with HasBody", show.Spec)
	}
	assertBodyEventCount(t, root, stateHome, "spec", created.Spec.ID, "body_edited", 1)
	assertBodyEventCount(t, root, stateHome, "spec", created.Spec.ID, "body_imported", 0)
}

func TestEditSpecBodyStripsFullDurableRenderInput(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	if _, err := CreateSpec(context.Background(), root, PathResolver{StateHome: stateHome}, SpecCreateOptions{
		Slug:    "render-strip",
		Title:   "Render Strip",
		Body:    "Original body.",
		SetBody: true,
	}); err != nil {
		t.Fatalf("CreateSpec() error = %v", err)
	}

	// The render-drift remediation names the committed render itself as the
	// --body-file path, so the edit input is a complete durable render.
	render, err := RenderDurableDocument(DurableRenderDocument{
		Kind: "spec",
		Fields: []DurableRenderField{
			{Key: "id", Value: "SPEC-001"},
			{Key: "status", Value: "draft"},
			{Key: "title", Value: "Render Strip"},
		},
		Body: "Inner prose from the committed render.",
	})
	if err != nil {
		t.Fatalf("RenderDurableDocument() error = %v", err)
	}

	if _, err := EditSpecBody(context.Background(), root, PathResolver{StateHome: stateHome}, SpecEditOptions{
		Ref:  "SPEC-001",
		Body: render,
	}); err != nil {
		t.Fatalf("EditSpecBody() error = %v", err)
	}

	show, err := ShowSpec(context.Background(), root, PathResolver{StateHome: stateHome}, "SPEC-001")
	if err != nil {
		t.Fatalf("ShowSpec() error = %v", err)
	}
	if show.Spec.Body != "Inner prose from the committed render." {
		t.Fatalf("Body = %q, want inner render body only", show.Spec.Body)
	}
	if strings.HasPrefix(show.Spec.Body, "---") || strings.Contains(show.Spec.Body, "loaf:render") {
		t.Fatalf("Body = %q, want no frontmatter and no render stamp", show.Spec.Body)
	}
}

func TestEditSpecBodyKeepsNonRenderFrontmatterVerbatim(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	if _, err := CreateSpec(context.Background(), root, PathResolver{StateHome: stateHome}, SpecCreateOptions{
		Slug:    "verbatim-frontmatter",
		Title:   "Verbatim Frontmatter",
		Body:    "Original body.",
		SetBody: true,
	}); err != nil {
		t.Fatalf("CreateSpec() error = %v", err)
	}

	// Starts with "---" but has no render stamp, so it is not a durable render:
	// only a complete render is stripped; everything else is stored verbatim.
	body := "---\nlooks: like frontmatter\n---\n\nProse that intentionally opens with a divider."
	if _, err := EditSpecBody(context.Background(), root, PathResolver{StateHome: stateHome}, SpecEditOptions{
		Ref:  "SPEC-001",
		Body: body,
	}); err != nil {
		t.Fatalf("EditSpecBody() error = %v", err)
	}

	show, err := ShowSpec(context.Background(), root, PathResolver{StateHome: stateHome}, "SPEC-001")
	if err != nil {
		t.Fatalf("ShowSpec() error = %v", err)
	}
	if show.Spec.Body != body {
		t.Fatalf("Body = %q, want verbatim input %q", show.Spec.Body, body)
	}
}

func TestEditSpecBodyImportsLegacySourceOnFirstEdit(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	// A spec with a provisioned source path but no SQLite body row: the legacy
	// file is the only holder of the prose until the first edit imports it.
	created, err := CreateSpec(context.Background(), root, PathResolver{StateHome: stateHome}, SpecCreateOptions{
		Slug:  "legacy-import",
		Title: "Legacy Import",
	})
	if err != nil {
		t.Fatalf("CreateSpec() error = %v", err)
	}
	writeAgentsFile(t, root.Path(), "specs/SPEC-001-legacy-import.md", "---\nid: SPEC-001\nstatus: draft\ntitle: Legacy Import\n---\n\nLegacy prose body.\n\n<!-- loaf:render kind=spec contract=durable-doc-v1 -->\n")

	edited, err := EditSpecBody(context.Background(), root, PathResolver{StateHome: stateHome}, SpecEditOptions{
		Ref:  "SPEC-001",
		Body: "Edited after import.",
	})
	if err != nil {
		t.Fatalf("EditSpecBody() error = %v", err)
	}
	if !edited.Imported {
		t.Fatalf("edited = %#v, want Imported true on first edit of legacy source", edited)
	}
	assertBodyEventCount(t, root, stateHome, "spec", created.Spec.ID, "body_imported", 1)
	assertBodyEventCount(t, root, stateHome, "spec", created.Spec.ID, "body_edited", 1)

	show, err := ShowSpec(context.Background(), root, PathResolver{StateHome: stateHome}, "SPEC-001")
	if err != nil {
		t.Fatalf("ShowSpec() error = %v", err)
	}
	if show.Spec.Body != "Edited after import." || !show.Spec.HasBody {
		t.Fatalf("Spec = %#v, want edited body after import", show.Spec)
	}
}

func TestEditSpecBodyRefusesWhenLegacySourceDiverges(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	if _, err := CreateSpec(context.Background(), root, PathResolver{StateHome: stateHome}, SpecCreateOptions{
		Slug:    "diverge-check",
		Title:   "Diverge Check",
		Body:    "Original body.",
		SetBody: true,
	}); err != nil {
		t.Fatalf("CreateSpec() error = %v", err)
	}
	fileContent := "---\nid: SPEC-001\nstatus: draft\ntitle: Diverge Check\n---\n\nHand-edited divergent prose.\n\n<!-- loaf:render kind=spec contract=durable-doc-v1 -->\n"
	writeAgentsFile(t, root.Path(), "specs/SPEC-001-diverge-check.md", fileContent)

	_, err := EditSpecBody(context.Background(), root, PathResolver{StateHome: stateHome}, SpecEditOptions{
		Ref:  "SPEC-001",
		Body: "Replacement body.",
	})
	if err == nil {
		t.Fatal("EditSpecBody() error = nil, want divergence refusal")
	}
	for _, want := range []string{"no longer matches the SQLite body", "loaf spec finalize", "--force"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("EditSpecBody() error = %q, want containing %q", err, want)
		}
	}

	show, err := ShowSpec(context.Background(), root, PathResolver{StateHome: stateHome}, "SPEC-001")
	if err != nil {
		t.Fatalf("ShowSpec() error = %v", err)
	}
	if show.Spec.Body != "Original body." {
		t.Fatalf("Body = %q, want unchanged SQLite body after refusal", show.Spec.Body)
	}
	assertSpecSourceFileBytes(t, root, "SPEC-001-diverge-check.md", fileContent)
}

func TestEditSpecBodyForceOverridesDivergence(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	if _, err := CreateSpec(context.Background(), root, PathResolver{StateHome: stateHome}, SpecCreateOptions{
		Slug:    "force-override",
		Title:   "Force Override",
		Body:    "Original body.",
		SetBody: true,
	}); err != nil {
		t.Fatalf("CreateSpec() error = %v", err)
	}
	fileContent := "---\nid: SPEC-001\nstatus: draft\ntitle: Force Override\n---\n\nHand-edited divergent prose.\n\n<!-- loaf:render kind=spec contract=durable-doc-v1 -->\n"
	writeAgentsFile(t, root.Path(), "specs/SPEC-001-force-override.md", fileContent)

	edited, err := EditSpecBody(context.Background(), root, PathResolver{StateHome: stateHome}, SpecEditOptions{
		Ref:   "SPEC-001",
		Body:  "Forced replacement.",
		Force: true,
	})
	if err != nil {
		t.Fatalf("EditSpecBody(force) error = %v", err)
	}
	if edited.EventID == "" {
		t.Fatalf("edited = %#v, want recorded edit event", edited)
	}

	show, err := ShowSpec(context.Background(), root, PathResolver{StateHome: stateHome}, "SPEC-001")
	if err != nil {
		t.Fatalf("ShowSpec() error = %v", err)
	}
	if show.Spec.Body != "Forced replacement." {
		t.Fatalf("Body = %q, want forced replacement body", show.Spec.Body)
	}
	// Force proceeds in SQLite only; the stale file is untouched until finalize.
	assertSpecSourceFileBytes(t, root, "SPEC-001-force-override.md", fileContent)
}

func TestEditSpecBodyRefusesStaleRenderWithoutFinalize(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	if _, err := CreateSpec(context.Background(), root, PathResolver{StateHome: stateHome}, SpecCreateOptions{
		Slug:    "stale-render",
		Title:   "Stale Render",
		Body:    "Body A.",
		SetBody: true,
	}); err != nil {
		t.Fatalf("CreateSpec() error = %v", err)
	}

	if _, err := EditSpecBody(context.Background(), root, PathResolver{StateHome: stateHome}, SpecEditOptions{Ref: "SPEC-001", Body: "Body B."}); err != nil {
		t.Fatalf("EditSpecBody(B) error = %v", err)
	}
	if _, err := FinalizeDurableArtifact(context.Background(), root, PathResolver{StateHome: stateHome}, DurableFinalizeOptions{Kind: "spec", Ref: "SPEC-001"}); err != nil {
		t.Fatalf("FinalizeDurableArtifact() error = %v", err)
	}
	if _, err := EditSpecBody(context.Background(), root, PathResolver{StateHome: stateHome}, SpecEditOptions{Ref: "SPEC-001", Body: "Body C."}); err != nil {
		t.Fatalf("EditSpecBody(C) error = %v", err)
	}

	// Accepted limitation: without a finalize after the previous edit the render
	// on disk is stale, so the next edit reads as a divergence and refuses.
	_, err := EditSpecBody(context.Background(), root, PathResolver{StateHome: stateHome}, SpecEditOptions{Ref: "SPEC-001", Body: "Body D."})
	if err == nil || !strings.Contains(err.Error(), "no longer matches the SQLite body") {
		t.Fatalf("EditSpecBody(D) error = %v, want stale-render refusal", err)
	}

	if _, err := EditSpecBody(context.Background(), root, PathResolver{StateHome: stateHome}, SpecEditOptions{Ref: "SPEC-001", Body: "Body D.", Force: true}); err != nil {
		t.Fatalf("EditSpecBody(D, force) error = %v", err)
	}
	show, err := ShowSpec(context.Background(), root, PathResolver{StateHome: stateHome}, "SPEC-001")
	if err != nil {
		t.Fatalf("ShowSpec() error = %v", err)
	}
	if show.Spec.Body != "Body D." {
		t.Fatalf("Body = %q, want Body D. after forced edit", show.Spec.Body)
	}
}

func TestEditSpecBodyGuardsHandEditedRenderWithoutSourcePath(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	created, err := CreateSpec(context.Background(), root, PathResolver{StateHome: stateHome}, SpecCreateOptions{
		Slug:    "no-source-guard",
		Title:   "No Source Guard",
		Body:    "Original spec body.",
		SetBody: true,
	})
	if err != nil {
		t.Fatalf("CreateSpec() error = %v", err)
	}
	// A spec without a body source row (the migrated-spec shape) finalizes to
	// the alias-derived render path, which the edit guard must inspect.
	store := openTestStore(t, root, stateHome)
	if _, err := store.db.ExecContext(context.Background(), `UPDATE specs SET body_source_id = NULL WHERE project_id = ? AND id = ?`, projectIDForTest(t, store, root), created.Spec.ID); err != nil {
		store.Close()
		t.Fatalf("clear spec body_source_id error = %v", err)
	}
	store.Close()

	render, err := FinalizeDurableArtifact(context.Background(), root, PathResolver{StateHome: stateHome}, DurableFinalizeOptions{Kind: "spec", Ref: created.Spec.Alias})
	if err != nil {
		t.Fatalf("FinalizeDurableArtifact() error = %v", err)
	}
	if render.RelativePath != ".agents/specs/SPEC-001.md" {
		t.Fatalf("RelativePath = %q, want alias-derived render path", render.RelativePath)
	}

	content, err := os.ReadFile(render.Path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", render.Path, err)
	}
	handEdited := strings.Replace(string(content), "Original spec body.", "Hand-edited render prose.", 1)
	if handEdited == string(content) {
		t.Fatalf("render content missing stored body:\n%s", content)
	}
	if err := os.WriteFile(render.Path, []byte(handEdited), 0o600); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", render.Path, err)
	}

	_, err = EditSpecBody(context.Background(), root, PathResolver{StateHome: stateHome}, SpecEditOptions{
		Ref:  created.Spec.Alias,
		Body: "Replacement body.",
	})
	if err == nil {
		t.Fatal("EditSpecBody(hand-edited render) error = nil, want divergence refusal")
	}
	for _, want := range []string{"no longer matches the SQLite body", "loaf spec finalize", "--force"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("EditSpecBody() error = %q, want containing %q", err, want)
		}
	}
	show, err := ShowSpec(context.Background(), root, PathResolver{StateHome: stateHome}, created.Spec.Alias)
	if err != nil {
		t.Fatalf("ShowSpec() error = %v", err)
	}
	if show.Spec.Body != "Original spec body." {
		t.Fatalf("Body = %q, want unchanged SQLite body after refusal", show.Spec.Body)
	}

	if _, err := EditSpecBody(context.Background(), root, PathResolver{StateHome: stateHome}, SpecEditOptions{
		Ref:   created.Spec.Alias,
		Body:  "Forced replacement.",
		Force: true,
	}); err != nil {
		t.Fatalf("EditSpecBody(force) error = %v", err)
	}
	show, err = ShowSpec(context.Background(), root, PathResolver{StateHome: stateHome}, created.Spec.Alias)
	if err != nil {
		t.Fatalf("ShowSpec() error = %v", err)
	}
	if show.Spec.Body != "Forced replacement." {
		t.Fatalf("Body = %q, want forced replacement body", show.Spec.Body)
	}
}

func TestEditSpecBodyMissingRefFails(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	if _, err := EditSpecBody(context.Background(), root, PathResolver{StateHome: stateHome}, SpecEditOptions{
		Ref:  "SPEC-404",
		Body: "Body for nobody.",
	}); err == nil {
		t.Fatal("EditSpecBody(missing) error = nil, want unresolved ref failure")
	}

	store := openTestStore(t, root, stateHome)
	defer store.Close()
	var bodies, events int
	if err := store.db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM artifact_bodies`).Scan(&bodies); err != nil {
		t.Fatalf("count artifact bodies error = %v", err)
	}
	if err := store.db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM events WHERE event_type IN ('body_imported', 'body_edited')`).Scan(&events); err != nil {
		t.Fatalf("count body events error = %v", err)
	}
	if bodies != 0 || events != 0 {
		t.Fatalf("bodies = %d, body events = %d; want untouched database", bodies, events)
	}
}

func TestEditSpecBodyRoundTripsThroughFinalize(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	cases := []struct {
		name string
		body string
	}{
		{name: "crlf-line-endings", body: "Line one.\r\nLine two.\r\nLine three."},
		{name: "trailing-newlines", body: "Trailing prose.\n\n\n"},
		{name: "bare-separator-mid-body", body: "Intro prose.\n\n---\n\nOutro prose."},
		{name: "unicode", body: "Café ☕ — naïve 日本語 prose."},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			created, err := CreateSpec(context.Background(), root, PathResolver{StateHome: stateHome}, SpecCreateOptions{
				Slug: fmt.Sprintf("round-trip-%d", i+1),
			})
			if err != nil {
				t.Fatalf("CreateSpec() error = %v", err)
			}
			if _, err := EditSpecBody(context.Background(), root, PathResolver{StateHome: stateHome}, SpecEditOptions{
				Ref:  created.Spec.Alias,
				Body: tc.body,
			}); err != nil {
				t.Fatalf("EditSpecBody() error = %v", err)
			}
			render, err := FinalizeDurableArtifact(context.Background(), root, PathResolver{StateHome: stateHome}, DurableFinalizeOptions{
				Kind: "spec",
				Ref:  created.Spec.Alias,
			})
			if err != nil {
				t.Fatalf("FinalizeDurableArtifact() error = %v", err)
			}
			content, err := os.ReadFile(render.Path)
			if err != nil {
				t.Fatalf("ReadFile(%s) error = %v", render.Path, err)
			}
			rerendered, err := ReRenderDurableRender(string(content))
			if err != nil {
				t.Fatalf("ReRenderDurableRender() error = %v", err)
			}
			if rerendered != string(content) {
				t.Fatalf("re-render diverged from finalized file:\nfile:\n%q\nre-render:\n%q", content, rerendered)
			}
			doc, err := ParseDurableRender(string(content))
			if err != nil {
				t.Fatalf("ParseDurableRender() error = %v", err)
			}
			if doc.Body != normalizeDurableBody(tc.body) {
				t.Fatalf("parsed body = %q, want normalized %q", doc.Body, normalizeDurableBody(tc.body))
			}
		})
	}
}

func assertBodyEventCount(t *testing.T, root project.Root, stateHome string, entityKind string, entityID string, eventType string, want int) {
	t.Helper()
	store := openTestStore(t, root, stateHome)
	defer store.Close()

	var got int
	if err := store.db.QueryRowContext(context.Background(), `
SELECT COUNT(*)
FROM events
WHERE project_id = ? AND entity_kind = ? AND entity_id = ? AND event_type = ?
`, projectIDForTest(t, store, root), entityKind, entityID, eventType).Scan(&got); err != nil {
		t.Fatalf("count %s events error = %v", eventType, err)
	}
	if got != want {
		t.Fatalf("%s event count = %d, want %d", eventType, got, want)
	}
}

func assertSpecSourceFileBytes(t *testing.T, root project.Root, filename string, want string) {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(root.Path(), ".agents", "specs", filename))
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", filename, err)
	}
	if string(content) != want {
		t.Fatalf("source file %s changed:\ngot:\n%q\nwant:\n%q", filename, content, want)
	}
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
