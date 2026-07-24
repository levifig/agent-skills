package state

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/levifig/loaf/internal/project"
)

func TestOriginMatchesMigrationFingerprint(t *testing.T) {
	journal := journalEntryScanRow{
		HarnessSessionID: sql.NullString{String: "hsid", Valid: true},
		ObservedBranch:   sql.NullString{String: "feat/x", Valid: true},
		ObservedWorktree: sql.NullString{},
		CreatedAt:        "2026-07-10T10:00:00Z",
	}
	base := journalOriginScanRow{
		CaptureMechanism: JournalOriginMechanismUnknown,
		EnvelopeVersion:  1,
		HarnessSessionID: sql.NullString{String: "hsid", Valid: true},
		Branch:           sql.NullString{String: "feat/x", Valid: true},
		CreatedAt:        "2026-07-10T10:00:00Z",
	}
	if !originMatchesMigrationFingerprint(base, journal) {
		t.Fatal("clean 0011 fingerprint should match")
	}

	dirty := base
	dirty.Dirty = sql.NullInt64{Int64: 1, Valid: true}
	if originMatchesMigrationFingerprint(dirty, journal) {
		t.Fatal("dirty=1 must not match")
	}

	v2 := base
	v2.EnvelopeVersion = 2
	if originMatchesMigrationFingerprint(v2, journal) {
		t.Fatal("envelope v2 must not match")
	}

	mismatch := base
	mismatch.Branch = sql.NullString{String: "other", Valid: true}
	if originMatchesMigrationFingerprint(mismatch, journal) {
		t.Fatal("branch copy mismatch must not match")
	}

	manual := base
	manual.CaptureMechanism = JournalOriginMechanismManual
	if decideOriginImportDisposition(&manual, &journal) != originImportSkip {
		t.Fatal("manual origin must skip")
	}
	if decideOriginImportDisposition(&base, &journal) != originImportReclaim {
		t.Fatal("fingerprint unknown must reclaim")
	}
	migration := base
	migration.CaptureMechanism = JournalOriginMechanismMigration
	if decideOriginImportDisposition(&migration, &journal) != originImportRefreshMigration {
		t.Fatal("migration origin must refresh")
	}
	if decideOriginImportDisposition(nil, nil) != originImportInsert {
		t.Fatal("missing origin must insert")
	}
}

func TestDecideImportStatusMatrix(t *testing.T) {
	unknown := "unknown"
	archived := "archived"
	inProgress := "in_progress"

	// insert no-opinion idea → open
	d := decideImportStatus(LifecycleEntityIdea, "idea-1", nil, "unknown", LifecycleStatusOpen, true)
	if d.Status != LifecycleStatusOpen || !d.Write || d.Warning != "" {
		t.Fatalf("idea no-opinion insert = %#v", d)
	}

	// insert no-opinion task → unknown
	d = decideImportStatus(LifecycleEntityTask, "task-1", nil, "", "unknown", true)
	if d.Status != "unknown" || !d.Write {
		t.Fatalf("task no-opinion insert = %#v", d)
	}

	// normalized fills stored unknown
	d = decideImportStatus(LifecycleEntityIdea, "idea-1", &unknown, "resolved", LifecycleStatusOpen, true)
	if d.Status != LifecycleStatusDone || !d.Write || d.Divergence != nil {
		t.Fatalf("fill unknown idea = %#v", d)
	}

	// normalized over real keeps + diverges
	d = decideImportStatus(LifecycleEntitySpec, "spec-1", &inProgress, "complete", "unknown", true)
	if d.Write || d.Status != inProgress || d.Divergence == nil || d.Divergence.IncomingStatus != "done" {
		t.Fatalf("real status keep+diverge = %#v", d)
	}

	// legacy complete vs done is not a divergence
	complete := "complete"
	d = decideImportStatus(LifecycleEntitySpec, "spec-1", &complete, "done", "unknown", true)
	if d.Write || d.Divergence != nil {
		t.Fatalf("canonical equality should not diverge: %#v", d)
	}

	// OOV never fills unknown
	d = decideImportStatus(LifecycleEntityTask, "task-1", &unknown, "accepted", "unknown", true)
	if d.Write || d.Status != "unknown" || d.Warning == "" || d.Divergence != nil {
		t.Fatalf("OOV over unknown = %#v", d)
	}

	// no-opinion over real keeps, no divergence
	d = decideImportStatus(LifecycleEntitySpec, "spec-1", &archived, "", "unknown", true)
	if d.Write || d.Status != archived || d.Divergence != nil || d.Warning != "" {
		t.Fatalf("no-opinion over archived = %#v", d)
	}

	// shaping draft insert-only, raw explicit, no vocabulary warning
	d = decideImportStatus("shaping_draft", "draft-1", nil, "grilling", "draft", false)
	if d.Status != "grilling" || !d.Write || d.Warning != "" {
		t.Fatalf("shaping insert raw = %#v", d)
	}
	storedDraft := "draft"
	d = decideImportStatus("shaping_draft", "draft-1", &storedDraft, "grilling", "draft", false)
	if d.Write || d.Status != "draft" || d.Divergence == nil {
		t.Fatalf("shaping existing insert-only diverge = %#v", d)
	}
}

func TestImportMarkdownReclaimsFingerprintAndSkipsForeignOrigins(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	stateHome := t.TempDir()
	writeAgentsFile(t, root.Path(), "sessions/reclaim.md", `---
branch: feat/reclaim
harness_session_id: hsid-reclaim
---
[2026-07-10 10:00] decision(reclaim): clean fingerprint
[2026-07-10 10:01] decision(dirty): dirty origin
[2026-07-10 10:02] decision(v2): envelope v2
[2026-07-10 10:03] decision(mismatch): copy mismatch
[2026-07-10 10:04] decision(manual): foreign mechanism
`)
	result, err := ApplyMarkdownMigration(ctx, root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("ApplyMarkdownMigration() error = %v", err)
	}
	store, err := OpenStore(result.DatabasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()

	type row struct {
		id      string
		message string
	}
	var rows []row
	q, err := store.db.QueryContext(ctx, `SELECT id, message FROM journal_entries ORDER BY message`)
	if err != nil {
		t.Fatalf("query journals: %v", err)
	}
	defer q.Close()
	for q.Next() {
		var r row
		if err := q.Scan(&r.id, &r.message); err != nil {
			t.Fatalf("scan: %v", err)
		}
		rows = append(rows, r)
	}
	byMsg := map[string]string{}
	for _, r := range rows {
		byMsg[r.message] = r.id
	}

	// Seed 0011-compatible unknown fingerprint on clean row.
	if _, err := store.db.ExecContext(ctx, `
UPDATE journal_origins SET
  capture_mechanism = 'unknown',
  envelope_version = 1,
  observed_harness = NULL,
  observed_harness_version = NULL,
  agent_id = NULL,
  source_event = NULL,
  head = NULL,
  change_path = NULL,
  change_sha256 = NULL,
  dirty = NULL,
  reconstructable = NULL,
  durable_result_kind = NULL,
  durable_result_id = NULL,
  harness_session_id = (SELECT harness_session_id FROM journal_entries WHERE id = ?),
  branch = (SELECT observed_branch FROM journal_entries WHERE id = ?),
  worktree = (SELECT observed_worktree FROM journal_entries WHERE id = ?),
  created_at = (SELECT created_at FROM journal_entries WHERE id = ?)
WHERE journal_entry_id = ?
`, byMsg["clean fingerprint"], byMsg["clean fingerprint"], byMsg["clean fingerprint"], byMsg["clean fingerprint"], byMsg["clean fingerprint"]); err != nil {
		t.Fatalf("seed clean fingerprint: %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `UPDATE journal_origins SET capture_mechanism = 'unknown', dirty = 1, source_event = NULL WHERE journal_entry_id = ?`, byMsg["dirty origin"]); err != nil {
		t.Fatalf("seed dirty: %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `UPDATE journal_origins SET capture_mechanism = 'unknown', envelope_version = 2, source_event = NULL WHERE journal_entry_id = ?`, byMsg["envelope v2"]); err != nil {
		t.Fatalf("seed v2: %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `UPDATE journal_origins SET capture_mechanism = 'unknown', branch = 'other-branch', source_event = NULL WHERE journal_entry_id = ?`, byMsg["copy mismatch"]); err != nil {
		t.Fatalf("seed mismatch: %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `UPDATE journal_origins SET capture_mechanism = 'manual', source_event = 'manual' WHERE journal_entry_id = ?`, byMsg["foreign mechanism"]); err != nil {
		t.Fatalf("seed manual: %v", err)
	}

	// Mutate messages so reclaim/skip is observable.
	for _, msg := range []string{"clean fingerprint", "dirty origin", "envelope v2", "copy mismatch", "foreign mechanism"} {
		if _, err := store.db.ExecContext(ctx, `UPDATE journal_entries SET message = ? WHERE id = ?`, "stale "+msg, byMsg[msg]); err != nil {
			t.Fatalf("stale message: %v", err)
		}
	}

	report, err := store.ImportMarkdown(ctx, root)
	if err != nil {
		t.Fatalf("ImportMarkdown() error = %v", err)
	}
	if report.ReclaimedOrigins != 1 {
		t.Fatalf("ReclaimedOrigins = %d, want 1", report.ReclaimedOrigins)
	}
	if len(report.SkippedEntries) != 4 {
		t.Fatalf("SkippedEntries = %#v, want 4", report.SkippedEntries)
	}

	assertOriginMechanism(t, store, byMsg["clean fingerprint"], JournalOriginMechanismMigration)
	var cleanMsg string
	if err := store.db.QueryRowContext(ctx, `SELECT message FROM journal_entries WHERE id = ?`, byMsg["clean fingerprint"]).Scan(&cleanMsg); err != nil {
		t.Fatalf("read clean: %v", err)
	}
	if cleanMsg != "clean fingerprint" {
		t.Fatalf("reclaimed message = %q, want refreshed", cleanMsg)
	}
	for _, msg := range []string{"dirty origin", "envelope v2", "copy mismatch", "foreign mechanism"} {
		var got string
		if err := store.db.QueryRowContext(ctx, `SELECT message FROM journal_entries WHERE id = ?`, byMsg[msg]).Scan(&got); err != nil {
			t.Fatalf("read %s: %v", msg, err)
		}
		if got != "stale "+msg {
			t.Fatalf("%s message = %q, want untouched stale", msg, got)
		}
	}
	assertOriginMechanism(t, store, byMsg["dirty origin"], JournalOriginMechanismUnknown)
	assertOriginMechanism(t, store, byMsg["envelope v2"], JournalOriginMechanismUnknown)
	assertOriginMechanism(t, store, byMsg["copy mismatch"], JournalOriginMechanismUnknown)
	assertOriginMechanism(t, store, byMsg["foreign mechanism"], "manual")
}

func TestImportMarkdownStatusMatrixExistingRows(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	stateHome := t.TempDir()
	writeAgentsFile(t, root.Path(), "ideas/20260724-fill.md", `---
id: idea-fill
title: Fill Me
status: resolved
---
# Fill Me
`)
	writeAgentsFile(t, root.Path(), "ideas/20260724-keep.md", `---
id: idea-keep
title: Keep Me
status: open
---
# Keep Me
`)
	writeAgentsFile(t, root.Path(), "specs/SPEC-010-archived.md", `---
id: SPEC-010
title: Archived Spec
status: complete
---
# Archived Spec
`)
	writeAgentsFile(t, root.Path(), "TASKS.json", `{
  "specs": {
    "SPEC-010": {"title": "Archived Spec", "status": "complete", "file": "SPEC-010-archived.md"}
  },
  "tasks": {}
}`)

	result, err := ApplyMarkdownMigration(ctx, root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("ApplyMarkdownMigration() error = %v", err)
	}
	store, err := OpenStore(result.DatabasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()

	ideaFill := stableMigrationID("idea", result.ProjectID, "idea-fill")
	ideaKeep := stableMigrationID("idea", result.ProjectID, "idea-keep")
	specID := stableMigrationID("spec", result.ProjectID, "SPEC-010")
	if _, err := store.db.ExecContext(ctx, `UPDATE ideas SET status = 'unknown' WHERE id = ?`, ideaFill); err != nil {
		t.Fatalf("seed unknown idea: %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `UPDATE ideas SET status = 'archived' WHERE id = ?`, ideaKeep); err != nil {
		t.Fatalf("seed archived idea: %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `UPDATE specs SET status = 'archived' WHERE id = ?`, specID); err != nil {
		t.Fatalf("seed archived spec: %v", err)
	}

	second, err := ApplyMarkdownMigration(ctx, root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("second apply error = %v", err)
	}
	assertEntityStatus(t, store, "ideas", result.ProjectID, "idea", "idea-fill", "done")
	assertEntityStatus(t, store, "ideas", result.ProjectID, "idea", "idea-keep", "archived")
	assertEntityStatus(t, store, "specs", result.ProjectID, "spec", "SPEC-010", "archived")
	if second.ImportReport == nil {
		t.Fatal("expected import report")
	}
	// no-opinion/open over archived idea: keep, no divergence (incoming open vs archived diverges!)
	// idea-keep incoming is open (normalized), stored archived → divergence
	foundKeep := false
	foundSpec := false
	for _, d := range second.ImportReport.StatusDivergences {
		if d.EntityID == ideaKeep && d.StoredStatus == "archived" && d.IncomingStatus == "open" {
			foundKeep = true
		}
		if d.EntityID == specID && d.StoredStatus == "archived" && d.IncomingStatus == "done" {
			foundSpec = true
		}
	}
	if !foundKeep || !foundSpec {
		t.Fatalf("divergences = %#v, want idea-keep and SPEC-010", second.ImportReport.StatusDivergences)
	}
}

func TestImportMarkdownSparkPromotedToRefreshesTarget(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	stateHome := t.TempDir()
	writeAgentsFile(t, root.Path(), "ideas/20260724-a.md", `---
id: idea-a
title: A
---
# A
`)
	writeAgentsFile(t, root.Path(), "ideas/20260724-b.md", `---
id: idea-b
title: B
---
# B
`)
	writeAgentsFile(t, root.Path(), "sessions/spark-target.md", `---
---
[2026-07-10 10:00] spark(scope): rename-me captured to .agents/ideas/20260724-a.md
`)

	result, err := ApplyMarkdownMigration(ctx, root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("ApplyMarkdownMigration() error = %v", err)
	}
	store, err := OpenStore(result.DatabasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()

	sparkID := stableMigrationID("spark", result.ProjectID, ".agents/sessions/spark-target.md", "3")
	ideaA := stableMigrationID("idea", result.ProjectID, "20260724-a")
	ideaB := stableMigrationID("idea", result.ProjectID, "20260724-b")
	var toID string
	if err := store.db.QueryRowContext(ctx, `
SELECT to_entity_id FROM relationships
WHERE project_id = ? AND from_entity_kind = 'spark' AND from_entity_id = ? AND relationship_type = 'promoted_to'
`, result.ProjectID, sparkID).Scan(&toID); err != nil {
		t.Fatalf("read initial promoted_to: %v", err)
	}
	if toID != ideaA {
		t.Fatalf("initial target = %q, want %q", toID, ideaA)
	}

	writeAgentsFile(t, root.Path(), "sessions/spark-target.md", `---
---
[2026-07-10 10:00] spark(scope): rename-me captured to .agents/ideas/20260724-b.md
`)
	if _, err := store.ImportMarkdown(ctx, root); err != nil {
		t.Fatalf("reimport error = %v", err)
	}
	var count int
	if err := store.db.QueryRowContext(ctx, `
SELECT COUNT(*) FROM relationships
WHERE project_id = ? AND from_entity_kind = 'spark' AND from_entity_id = ? AND relationship_type = 'promoted_to'
`, result.ProjectID, sparkID).Scan(&count); err != nil {
		t.Fatalf("count relationships: %v", err)
	}
	if count != 1 {
		t.Fatalf("promoted_to count = %d, want 1", count)
	}
	if err := store.db.QueryRowContext(ctx, `
SELECT to_entity_id FROM relationships
WHERE project_id = ? AND from_entity_kind = 'spark' AND from_entity_id = ? AND relationship_type = 'promoted_to'
`, result.ProjectID, sparkID).Scan(&toID); err != nil {
		t.Fatalf("read refreshed promoted_to: %v", err)
	}
	if toID != ideaB {
		t.Fatalf("refreshed target = %q, want %q", toID, ideaB)
	}

	// Competing manual relationship with an "imported from ..." reason must survive
	// origin='imported'-only refresh (Decision 7 ownership).
	manualID := "manual-spark-competing-rel"
	manualReason := "imported from hand curation note"
	manualCreated := "2026-01-01T00:00:00Z"
	if _, err := store.db.ExecContext(ctx, `
INSERT INTO relationships (
  id, project_id, from_entity_kind, from_entity_id, to_entity_kind, to_entity_id,
  relationship_type, reason, origin, created_at, updated_at
) VALUES (?, ?, 'spark', ?, 'idea', ?, 'mentions', ?, 'manual', ?, ?)
`, manualID, result.ProjectID, sparkID, ideaA, manualReason, manualCreated, manualCreated); err != nil {
		t.Fatalf("seed manual competing relationship: %v", err)
	}
	before := readRelationshipRow(t, store, manualID)
	if _, err := store.ImportMarkdown(ctx, root); err != nil {
		t.Fatalf("reimport with manual competitor error = %v", err)
	}
	after := readRelationshipRow(t, store, manualID)
	if !relationshipRowsEqual(before, after) {
		t.Fatalf("manual competing relationship mutated: before=%#v after=%#v", before, after)
	}
}

func TestImportMarkdownArtifactRefreshPreservesManualImportedFromReason(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	stateHome := t.TempDir()
	writeAgentsFile(t, root.Path(), "ideas/20260724-target.md", `---
title: Target
---
# Target
`)
	writeAgentsFile(t, root.Path(), "specs/SPEC-040-rel.md", `---
id: SPEC-040
title: Rel Spec
derived_from: .agents/ideas/20260724-target.md
---
# Rel Spec
`)

	result, err := ApplyMarkdownMigration(ctx, root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("ApplyMarkdownMigration() error = %v", err)
	}
	store, err := OpenStore(result.DatabasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()

	specID := stableMigrationID("spec", result.ProjectID, "SPEC-040")
	ideaID := stableMigrationID("idea", result.ProjectID, "20260724-target")
	manualID := "manual-artifact-competing-rel"
	manualReason := "imported from curated review"
	manualCreated := "2026-02-02T00:00:00Z"
	if _, err := store.db.ExecContext(ctx, `
INSERT INTO relationships (
  id, project_id, from_entity_kind, from_entity_id, to_entity_kind, to_entity_id,
  relationship_type, reason, origin, created_at, updated_at
) VALUES (?, ?, 'spec', ?, 'idea', ?, 'mentions', ?, 'manual', ?, ?)
`, manualID, result.ProjectID, specID, ideaID, manualReason, manualCreated, manualCreated); err != nil {
		t.Fatalf("seed manual competing relationship: %v", err)
	}
	before := readRelationshipRow(t, store, manualID)
	if _, err := store.ImportMarkdown(ctx, root); err != nil {
		t.Fatalf("reimport error = %v", err)
	}
	after := readRelationshipRow(t, store, manualID)
	if !relationshipRowsEqual(before, after) {
		t.Fatalf("manual competing relationship mutated: before=%#v after=%#v", before, after)
	}
	var importedCount int
	if err := store.db.QueryRowContext(ctx, `
SELECT COUNT(*) FROM relationships
WHERE project_id = ? AND from_entity_kind = 'spec' AND from_entity_id = ? AND origin = 'imported'
`, result.ProjectID, specID).Scan(&importedCount); err != nil {
		t.Fatalf("count imported: %v", err)
	}
	if importedCount < 1 {
		t.Fatalf("imported relationships = %d, want at least 1 after refresh", importedCount)
	}
}

func TestImportMarkdownSameTupleManualRelationshipKeepsOwnership(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	stateHome := t.TempDir()
	writeAgentsFile(t, root.Path(), "ideas/20260724-same-tuple.md", `---
title: Same Tuple Target
---
# Same Tuple Target
`)
	writeAgentsFile(t, root.Path(), "specs/SPEC-041-same-tuple.md", `---
id: SPEC-041
title: Same Tuple Spec
---
# Same Tuple Spec
`)

	result, err := ApplyMarkdownMigration(ctx, root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("ApplyMarkdownMigration() error = %v", err)
	}
	store, err := OpenStore(result.DatabasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()

	// loaf link shares stableMigrationID with the importer, so a manual edge
	// over the exact tuple the import derives collides on id, not just on
	// semantics. Seed the manual row first, then make the import derive the
	// same tuple and assert the conflict path leaves ownership untouched.
	specID := stableMigrationID("spec", result.ProjectID, "SPEC-041")
	ideaID := stableMigrationID("idea", result.ProjectID, "20260724-same-tuple")
	sameTupleID := stableMigrationID("relationship", result.ProjectID, "spec", specID, "derived_from", "idea", ideaID)
	manualReason := "linked by operator before reimport"
	manualCreated := "2026-03-03T00:00:00Z"
	if _, err := store.db.ExecContext(ctx, `
INSERT INTO relationships (
  id, project_id, from_entity_kind, from_entity_id, to_entity_kind, to_entity_id,
  relationship_type, reason, origin, created_at, updated_at
) VALUES (?, ?, 'spec', ?, 'idea', ?, 'derived_from', ?, 'manual', ?, ?)
`, sameTupleID, result.ProjectID, specID, ideaID, manualReason, manualCreated, manualCreated); err != nil {
		t.Fatalf("seed same-tuple manual relationship: %v", err)
	}

	writeAgentsFile(t, root.Path(), "specs/SPEC-041-same-tuple.md", `---
id: SPEC-041
title: Same Tuple Spec
derived_from: .agents/ideas/20260724-same-tuple.md
---
# Same Tuple Spec
`)

	before := readRelationshipRow(t, store, sameTupleID)
	if _, err := store.ImportMarkdown(ctx, root); err != nil {
		t.Fatalf("reimport deriving same tuple error = %v", err)
	}
	after := readRelationshipRow(t, store, sameTupleID)
	if !relationshipRowsEqual(before, after) {
		t.Fatalf("same-tuple manual relationship claimed by importer: before=%#v after=%#v", before, after)
	}
	if after["origin"] != "manual" {
		t.Fatalf("origin = %q, want manual", after["origin"])
	}
}

func readRelationshipRow(t *testing.T, store *Store, id string) map[string]string {
	t.Helper()
	var projectID, fromKind, fromID, toKind, toID, relType, createdAt, updatedAt string
	var reason, origin, sourceID, sourceField sql.NullString
	if err := store.db.QueryRowContext(context.Background(), `
SELECT project_id, from_entity_kind, from_entity_id, to_entity_kind, to_entity_id,
       relationship_type, reason, origin, source_id, source_field, created_at, updated_at
FROM relationships WHERE id = ?
`, id).Scan(&projectID, &fromKind, &fromID, &toKind, &toID, &relType, &reason, &origin, &sourceID, &sourceField, &createdAt, &updatedAt); err != nil {
		t.Fatalf("read relationship %s: %v", id, err)
	}
	nullOr := func(v sql.NullString) string {
		if !v.Valid {
			return "<nil>"
		}
		return v.String
	}
	return map[string]string{
		"id":                id,
		"project_id":        projectID,
		"from_entity_kind":  fromKind,
		"from_entity_id":    fromID,
		"to_entity_kind":    toKind,
		"to_entity_id":      toID,
		"relationship_type": relType,
		"reason":            nullOr(reason),
		"origin":            nullOr(origin),
		"source_id":         nullOr(sourceID),
		"source_field":      nullOr(sourceField),
		"created_at":        createdAt,
		"updated_at":        updatedAt,
	}
}

func TestApplyMarkdownMigrationIncludesImportReport(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	writeAgentsFile(t, root.Path(), "ideas/20260724-report.md", "# Idea\n")
	result, err := ApplyMarkdownMigration(context.Background(), root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("ApplyMarkdownMigration() error = %v", err)
	}
	if result.ImportReport == nil {
		t.Fatal("ImportReport is nil")
	}
	if result.ImportReport.SkippedEntries == nil || result.ImportReport.StatusDivergences == nil || result.ImportReport.Warnings == nil {
		t.Fatalf("ImportReport slices must be non-nil: %#v", result.ImportReport)
	}
	if result.Ideas != 1 {
		t.Fatalf("inventory Ideas = %d, want 1", result.Ideas)
	}
}

func assertOriginMechanism(t *testing.T, store *Store, journalID string, want string) {
	t.Helper()
	var got string
	if err := store.db.QueryRowContext(context.Background(), `SELECT capture_mechanism FROM journal_origins WHERE journal_entry_id = ?`, journalID).Scan(&got); err != nil {
		t.Fatalf("read origin %s: %v", journalID, err)
	}
	if got != want {
		t.Fatalf("origin %s mechanism = %q, want %q", journalID, got, want)
	}
}

func TestApplyMarkdownMigrationPostImportWalkFailureKeepsApplied(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	stateHome := t.TempDir()
	writeAgentsFile(t, root.Path(), "ideas/20260724-post.md", "# Idea\n")

	result, err := applyMarkdownMigration(ctx, root, PathResolver{StateHome: stateHome}, &markdownApplyOperations{
		afterImportCommitted: func(projectRoot project.Root) error {
			return errors.New("forced post-import walk failure")
		},
	})
	if err != nil {
		t.Fatalf("applyMarkdownMigration() error = %v", err)
	}
	if !result.Applied {
		t.Fatal("Applied = false, want true after committed import")
	}
	if result.ImportReport == nil {
		t.Fatal("ImportReport is nil")
	}
	found := false
	for _, warning := range result.Warnings {
		if strings.Contains(warning, "forced post-import walk failure") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("Warnings = %#v, want post-import failure warning", result.Warnings)
	}
}

func TestPreviewMarkdownMigrationWarnsOnUnreadableTaskFile(t *testing.T) {
	root := projectRoot(t)
	writeAgentsFile(t, root.Path(), "tasks/TASK-050-unread.md", `---
id: TASK-050
spec: SPEC-050
---
# Task
`)
	taskPath := filepath.Join(root.Path(), ".agents", "tasks", "TASK-050-unread.md")
	// Replace the file with a directory so ReadFile fails portably (chmod 0 is
	// still readable for the owner on macOS).
	if err := os.Remove(taskPath); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	if err := os.Mkdir(taskPath, 0o755); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}

	plan, err := PreviewMarkdownMigration(root)
	if err != nil {
		t.Fatalf("PreviewMarkdownMigration() error = %v", err)
	}
	found := false
	for _, warning := range plan.Warnings {
		if strings.Contains(warning, "TASK-050-unread.md") && strings.Contains(warning, "relationship count") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("Warnings = %#v, want unreadable task relationship-count warning", plan.Warnings)
	}
}

func relationshipRowsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for key, value := range a {
		if b[key] != value {
			return false
		}
	}
	return true
}
