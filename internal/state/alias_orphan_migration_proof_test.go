package state

import (
	"context"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/levifig/loaf/internal/project"
)

// A same-titled row that predates the damaging import is not a twin. The proof
// has to name the surviving alias holder as the 2026-06-24 re-import, not merely
// find the date somewhere in the pair.
func TestAliasOrphanContentIdentityRequiresTheHolderToBeTheReimport(t *testing.T) {
	ctx := context.Background()
	root, stateHome, projectID, _ := seedAliasOrphanFixtureBase(t)

	seedTask(t, stateHome, root, projectID, "task:incumbentlive00000001", "Untitled", "todo", "2024-01-05T00:00:00Z", true, "TASK-500")
	unrelated := "task:unrelatedlive0000000001"
	seedTask(t, stateHome, root, projectID, unrelated, "Untitled", "todo", "2026-06-24T13:03:00Z", false, "")

	preview, err := PreviewAliasOrphanMigration(ctx, root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("PreviewAliasOrphanMigration() error = %v", err)
	}
	got := aliasOrphanClassification(t, preview, unrelated)
	if got.Proof != aliasOrphanProofUnproven || got.Disposition != "" {
		t.Fatalf("classification = %#v, want unproven with no disposition", got)
	}
	if preview.Totals.Retire != 0 {
		t.Fatalf("preview retire = %d, want 0", preview.Totals.Retire)
	}
}

// Equal titles are not equal content: rows whose stored bodies differ stay
// unproven, and rows that agree on both keep the fallback proof.
func TestAliasOrphanContentIdentityComparesBodies(t *testing.T) {
	ctx := context.Background()
	root, stateHome, projectID, _ := seedAliasOrphanFixtureBase(t)

	twinID := "spec:bodytwin000000000001"
	orphanID := "spec:bodyorphan00000000001"
	seedSpec(t, stateHome, root, projectID, twinID, "Same Title", "active", "2026-06-24T13:03:00Z", true, "SPEC-BODY")
	seedSpec(t, stateHome, root, projectID, orphanID, "Same Title", "active", "2026-06-13T10:00:00Z", false, "")
	seedArtifactBody(t, stateHome, root, projectID, "spec", twinID, "twin body", "hash-twin")
	seedArtifactBody(t, stateHome, root, projectID, "spec", orphanID, "orphan body", "hash-orphan")

	preview, err := PreviewAliasOrphanMigration(ctx, root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("PreviewAliasOrphanMigration() error = %v", err)
	}
	if got := aliasOrphanClassification(t, preview, orphanID); got.Proof != aliasOrphanProofUnproven {
		t.Fatalf("differing bodies classification = %#v, want unproven", got)
	}

	mustExecOpen(t, stateHome, root, `UPDATE artifact_bodies SET content = 'twin body', content_hash = 'hash-twin' WHERE project_id = ? AND entity_id = ?`, projectID, orphanID)
	agreed, err := PreviewAliasOrphanMigration(ctx, root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("second PreviewAliasOrphanMigration() error = %v", err)
	}
	got := aliasOrphanClassification(t, agreed, orphanID)
	if got.Proof != aliasOrphanProofContentIdentity || got.TwinID != twinID {
		t.Fatalf("matching bodies classification = %#v, want content-identity against %s", got, twinID)
	}
}

// A project that moved after the damaging import still has a recomputable
// legacy ID — from project_paths, not only from its current path.
func TestAliasOrphanDerivationUsesHistoricalProjectPaths(t *testing.T) {
	ctx := context.Background()
	root, stateHome, projectID, _ := seedAliasOrphanFixtureBase(t)

	oldPath := "/previous/home/of/this/project"
	mustExecOpen(t, stateHome, root, `
INSERT INTO project_paths (id, project_id, path, is_current, first_seen_at, last_seen_at, created_at, updated_at)
VALUES (?, ?, ?, 0, ?, ?, ?, ?)
`, "projpath:historical00000001", projectID, oldPath, "2026-06-13T10:00:00Z", "2026-06-13T10:00:00Z", "2026-06-13T10:00:00Z", "2026-06-13T10:00:00Z")

	legacyID := hex.EncodeToString(sha256Sum(oldPath))
	alias := "TASK-MOVED"
	twinID := stableMigrationID("task", projectID, alias)
	orphanID := stableMigrationID("task", legacyID, alias)
	seedTask(t, stateHome, root, projectID, twinID, "Moved Project Task", "todo", "2026-06-24T13:03:00Z", true, alias)
	seedTask(t, stateHome, root, projectID, orphanID, "Moved Project Task", "todo", "2026-06-13T10:00:00Z", false, "")

	preview, err := PreviewAliasOrphanMigration(ctx, root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("PreviewAliasOrphanMigration() error = %v", err)
	}
	got := aliasOrphanClassification(t, preview, orphanID)
	if got.Proof != aliasOrphanProofDerivation {
		t.Fatalf("classification = %#v, want derivation from the historical path", got)
	}
	if got.LegacyPath != oldPath || got.LegacyProjectID != legacyID {
		t.Fatalf("classification legacy salt = %q/%q, want %q/%q", got.LegacyPath, got.LegacyProjectID, oldPath, legacyID)
	}
}

// Sparks are minted from (path, line), never from an alias, so alias-salt
// recomputation can never reach them. Their own source ID carries the salt, and
// the manifest labels the resulting proof for what it is: half salt, half content.
func TestAliasOrphanSparkEarnsSourceDerivationProof(t *testing.T) {
	ctx := context.Background()
	root, stateHome, projectID, path := seedAliasOrphanFixtureBase(t)
	legacyID := hex.EncodeToString(sha256Sum(path))
	relPath := ".agents/sessions/20260613-session.md"

	legacySourceID := stableMigrationID("source", legacyID, relPath)
	currentSourceID := stableMigrationID("source", projectID, relPath)
	seedSource(t, stateHome, root, projectID, legacySourceID, relPath)
	seedSource(t, stateHome, root, projectID, currentSourceID, relPath)

	orphanID := stableMigrationID("spark", legacyID, relPath, "12")
	twinID := stableMigrationID("spark", projectID, relPath, "12")
	seedSpark(t, stateHome, root, projectID, twinID, "dedupe the state tables one day", currentSourceID, "2026-06-24T13:03:00Z", "SPARK-dedupe")
	seedSpark(t, stateHome, root, projectID, orphanID, "dedupe the state tables one day", legacySourceID, "2026-06-13T10:00:00Z", "")

	preview, err := PreviewAliasOrphanMigration(ctx, root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("PreviewAliasOrphanMigration() error = %v", err)
	}
	got := aliasOrphanClassification(t, preview, orphanID)
	if got.Proof != aliasOrphanProofSourceDerivation || got.TwinID != twinID {
		t.Fatalf("spark classification = %#v, want source-derivation against %s", got, twinID)
	}
}

// Two pre-rekey sparks with identical text from one file against a single alias
// holder is a merge, not a twin proof. Both rows stay unproven and untouched.
func TestAliasOrphanSourceSaltRefusesManyOrphansToOneHolder(t *testing.T) {
	ctx := context.Background()
	root, stateHome, projectID, path := seedAliasOrphanFixtureBase(t)
	legacyID := hex.EncodeToString(sha256Sum(path))
	relPath := ".agents/sessions/20260613-merge.md"

	legacySourceID := stableMigrationID("source", legacyID, relPath)
	currentSourceID := stableMigrationID("source", projectID, relPath)
	seedSource(t, stateHome, root, projectID, legacySourceID, relPath)
	seedSource(t, stateHome, root, projectID, currentSourceID, relPath)

	text := "dedupe the state tables one day"
	holderID := stableMigrationID("spark", projectID, relPath, "12")
	firstOrphan := stableMigrationID("spark", legacyID, relPath, "12")
	secondOrphan := stableMigrationID("spark", legacyID, relPath, "31")
	seedSpark(t, stateHome, root, projectID, holderID, text, currentSourceID, "2026-06-24T13:03:00Z", "SPARK-dedupe")
	seedSpark(t, stateHome, root, projectID, firstOrphan, text, legacySourceID, "2026-06-13T10:00:00Z", "")
	seedSpark(t, stateHome, root, projectID, secondOrphan, text, legacySourceID, "2026-06-13T10:05:00Z", "")

	preview, err := PreviewAliasOrphanMigration(ctx, root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("PreviewAliasOrphanMigration() error = %v", err)
	}
	for _, orphanID := range []string{firstOrphan, secondOrphan} {
		got := aliasOrphanClassification(t, preview, orphanID)
		if got.Proof != aliasOrphanProofUnproven || got.Disposition != "" {
			t.Fatalf("classification for %s = %#v, want unproven with no disposition", orphanID, got)
		}
	}

	if _, err := ApplyAliasOrphanMigration(ctx, root, PathResolver{StateHome: stateHome}, AliasOrphanApplyOptions{}); err != nil {
		t.Fatalf("ApplyAliasOrphanMigration() error = %v", err)
	}
	for _, orphanID := range []string{firstOrphan, secondOrphan} {
		if !entityExists(t, stateHome, root, "sparks", orphanID) {
			t.Fatalf("unproven spark %s was retired against a shared holder", orphanID)
		}
	}
}

// Retiring a report used to abort the whole migration at COMMIT: findings hold
// a NOT NULL foreign key that no polymorphic sweep reaches.
func TestAliasOrphanRetiresReportFindingsAndVerdicts(t *testing.T) {
	ctx := context.Background()
	root, stateHome, projectID, path := seedAliasOrphanFixtureBase(t)
	legacyID := hex.EncodeToString(sha256Sum(path))
	alias := "report-findings"
	twinID := stableMigrationID("report", projectID, alias)
	orphanID := stableMigrationID("report", legacyID, alias)

	seedReport(t, stateHome, root, projectID, twinID, "Findings Report", "2026-06-24T13:03:00Z", alias)
	seedReport(t, stateHome, root, projectID, orphanID, "Findings Report", "2026-06-13T10:00:00Z", "")
	mustExecOpen(t, stateHome, root, `
INSERT INTO findings (id, project_id, report_id, title, status, severity, confidence, created_at, updated_at)
VALUES (?, ?, ?, 'Orphan finding', 'open', 'high', 'confirmed', ?, ?)
`, "finding:orphan00000000001", projectID, orphanID, "2026-06-13T10:00:00Z", "2026-06-13T10:00:00Z")
	mustExecOpen(t, stateHome, root, `
INSERT INTO verdicts (id, project_id, finding_id, outcome, rationale, created_at, updated_at)
VALUES (?, ?, ?, 'confirmed', 'fixture', ?, ?)
`, "verdict:orphan00000000001", projectID, "finding:orphan00000000001", "2026-06-13T10:00:00Z", "2026-06-13T10:00:00Z")

	applied, err := ApplyAliasOrphanMigration(ctx, root, PathResolver{StateHome: stateHome}, AliasOrphanApplyOptions{})
	if err != nil {
		t.Fatalf("ApplyAliasOrphanMigration() error = %v", err)
	}
	if entityExists(t, stateHome, root, "reports", orphanID) {
		t.Fatal("orphan report survived apply")
	}
	if entityExists(t, stateHome, root, "findings", "finding:orphan00000000001") {
		t.Fatal("finding for retired report survived apply")
	}
	if entityExists(t, stateHome, root, "verdicts", "verdict:orphan00000000001") {
		t.Fatal("verdict for retired finding survived apply")
	}

	if _, err := RollbackAliasOrphanMigration(ctx, root, PathResolver{StateHome: stateHome}, applied.RollbackManifestPath); err != nil {
		t.Fatalf("RollbackAliasOrphanMigration() error = %v", err)
	}
	if !entityExists(t, stateHome, root, "findings", "finding:orphan00000000001") {
		t.Fatal("finding not restored by rollback")
	}
	if !entityExists(t, stateHome, root, "verdicts", "verdict:orphan00000000001") {
		t.Fatal("verdict not restored by rollback")
	}
}

// journal_deferrals.spark_id and intent_operations.spark_id are NOT NULL with no
// foreign key: retiring a spark without touching them leaves silent dangling
// provenance that nothing detects.
func TestAliasOrphanRepointsSparkProvenanceAtTheTwin(t *testing.T) {
	ctx := context.Background()
	root, stateHome, projectID, path := seedAliasOrphanFixtureBase(t)
	legacyID := hex.EncodeToString(sha256Sum(path))
	relPath := ".agents/sessions/20260613-deferral.md"

	legacySourceID := stableMigrationID("source", legacyID, relPath)
	currentSourceID := stableMigrationID("source", projectID, relPath)
	seedSource(t, stateHome, root, projectID, legacySourceID, relPath)
	seedSource(t, stateHome, root, projectID, currentSourceID, relPath)

	orphanID := stableMigrationID("spark", legacyID, relPath, "4")
	twinID := stableMigrationID("spark", projectID, relPath, "4")
	seedSpark(t, stateHome, root, projectID, twinID, "defer this thought", currentSourceID, "2026-06-24T13:03:00Z", "SPARK-defer")
	seedSpark(t, stateHome, root, projectID, orphanID, "defer this thought", legacySourceID, "2026-06-13T10:00:00Z", "")

	mustExecOpen(t, stateHome, root, `
INSERT INTO journal_entries (id, project_id, entry_type, scope, message, created_at, updated_at)
VALUES (?, ?, 'spark', 'scope', 'defer this thought', ?, ?)
`, "journal:deferral0000000001", projectID, "2026-06-13T10:00:00Z", "2026-06-13T10:00:00Z")
	mustExecOpen(t, stateHome, root, `
INSERT INTO journal_deferrals (project_id, operation_key, journal_entry_id, spark_id, stored_digest, created_at)
VALUES (?, 'op-defer-1', ?, ?, ?, ?)
`, projectID, "journal:deferral0000000001", orphanID, strings.Repeat("a", 64), "2026-06-13T10:00:00Z")

	applied, err := ApplyAliasOrphanMigration(ctx, root, PathResolver{StateHome: stateHome}, AliasOrphanApplyOptions{})
	if err != nil {
		t.Fatalf("ApplyAliasOrphanMigration() error = %v", err)
	}
	if entityExists(t, stateHome, root, "sparks", orphanID) {
		t.Fatal("orphan spark survived apply")
	}
	if got := deferralSparkID(t, stateHome, root, projectID, "op-defer-1"); got != twinID {
		t.Fatalf("journal_deferrals.spark_id = %q, want the surviving twin %q", got, twinID)
	}

	if _, err := RollbackAliasOrphanMigration(ctx, root, PathResolver{StateHome: stateHome}, applied.RollbackManifestPath); err != nil {
		t.Fatalf("RollbackAliasOrphanMigration() error = %v", err)
	}
	if got := deferralSparkID(t, stateHome, root, projectID, "op-defer-1"); got != orphanID {
		t.Fatalf("journal_deferrals.spark_id after rollback = %q, want %q", got, orphanID)
	}
}

// --realias onto a claimed alias would manufacture a brand-new alias-orphan of
// exactly the class this migration exists to repair.
func TestAliasOrphanRealiasRefusesAClaimedAlias(t *testing.T) {
	ctx := context.Background()
	root, stateHome, projectID, _ := seedAliasOrphanFixtureBase(t)

	incumbent := "task:incumbent000000000001"
	thief := "task:orphanthief00000000001"
	seedTask(t, stateHome, root, projectID, incumbent, "Incumbent", "todo", "2026-05-01T00:00:00Z", true, "TASK-777")
	seedTask(t, stateHome, root, projectID, thief, "Orphan Thief", "todo", "2026-05-01T00:00:00Z", false, "")

	_, err := ApplyAliasOrphanMigration(ctx, root, PathResolver{StateHome: stateHome}, AliasOrphanApplyOptions{
		Realias: map[string]string{thief: "TASK-777"},
		Flags:   []string{"--realias " + thief + "=TASK-777"},
	})
	if err == nil {
		t.Fatal("ApplyAliasOrphanMigration() accepted a realias onto a claimed alias")
	}
	if !strings.Contains(err.Error(), incumbent) {
		t.Fatalf("error = %v, want it to name the incumbent %s", err, incumbent)
	}
	if !aliasPointsTo(t, stateHome, root, projectID, "task", "TASK-777", incumbent) {
		t.Fatal("TASK-777 no longer names the incumbent")
	}
	if !entityExists(t, stateHome, root, "tasks", thief) {
		t.Fatal("the refused orphan was mutated")
	}
}

// A mistyped disposition names nothing; applying the rest of the run silently
// would leave the operator believing a row was handled.
func TestAliasOrphanApplyRejectsDispositionsThatMatchNothing(t *testing.T) {
	ctx := context.Background()
	root, stateHome, projectID, _ := seedAliasOrphanFixtureBase(t)
	seedTask(t, stateHome, root, projectID, "task:realorphan0000000001", "Real Orphan", "todo", "2026-05-01T00:00:00Z", false, "")

	preview, err := PreviewAliasOrphanMigration(ctx, root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("PreviewAliasOrphanMigration() error = %v", err)
	}
	if len(preview.Warnings) != 0 {
		t.Fatalf("preview warnings = %v, want none", preview.Warnings)
	}

	refused, err := ApplyAliasOrphanMigration(ctx, root, PathResolver{StateHome: stateHome}, AliasOrphanApplyOptions{
		Retire: []string{"task:doesnotexist00000001"},
		Flags:  []string{"--retire task:doesnotexist00000001"},
	})
	if err == nil {
		t.Fatal("ApplyAliasOrphanMigration() silently ignored a disposition that matched nothing")
	}
	if !strings.Contains(err.Error(), "task:doesnotexist00000001") {
		t.Fatalf("error = %v, want it to name the unmatched id", err)
	}
	// The refusal happens after the backup, so the result has to name it — an
	// error that hides the artifact it just created is an artifact nobody cleans up.
	if refused.Applied || refused.BackupPath == "" {
		t.Fatalf("refused result = %#v, want applied=false with the backup path", refused)
	}
	if !entityExists(t, stateHome, root, "tasks", "task:realorphan0000000001") {
		t.Fatal("apply mutated rows despite the rejected disposition")
	}
}

// An explicit --realias outranks the automatic classification: a row the
// operator named for preservation is preserved, whatever the proof says.
func TestAliasOrphanRealiasOutranksAProvenRetirement(t *testing.T) {
	ctx := context.Background()
	root, stateHome, projectID, path := seedAliasOrphanFixtureBase(t)
	legacyID := hex.EncodeToString(sha256Sum(path))
	alias := "TASK-777"
	twinID := stableMigrationID("task", projectID, alias)
	orphanID := stableMigrationID("task", legacyID, alias)
	seedTask(t, stateHome, root, projectID, twinID, "Proven Twin", "todo", "2026-06-24T13:03:00Z", true, alias)
	seedTask(t, stateHome, root, projectID, orphanID, "Proven Twin", "todo", "2026-06-13T10:00:00Z", false, "")

	applied, err := ApplyAliasOrphanMigration(ctx, root, PathResolver{StateHome: stateHome}, AliasOrphanApplyOptions{
		Realias: map[string]string{orphanID: "TASK-KEEPME"},
		Flags:   []string{"--realias " + orphanID + "=TASK-KEEPME"},
	})
	if err != nil {
		t.Fatalf("ApplyAliasOrphanMigration() error = %v", err)
	}
	if !entityExists(t, stateHome, root, "tasks", orphanID) {
		t.Fatal("--realias on a proven orphan deleted the row the operator asked to keep")
	}
	if !aliasPointsTo(t, stateHome, root, projectID, "task", "TASK-KEEPME", orphanID) {
		t.Fatal("--realias did not attach the requested alias")
	}
	if applied.Totals.EntitiesRetired != 0 {
		t.Fatalf("entities retired = %d, want 0", applied.Totals.EntitiesRetired)
	}
	for _, disposition := range applied.Dispositions {
		if disposition.EntityID == orphanID && disposition.Action != aliasOrphanDispositionRealias {
			t.Fatalf("manifest disposition for %s = %q, want realias", orphanID, disposition.Action)
		}
	}
}

// The ceremony records the exact apply command; running it again has to be a
// no-op, not a hard error about flags the first run already carried out.
func TestAliasOrphanSecondApplyWithTheSameDispositionsIsANoOp(t *testing.T) {
	ctx := context.Background()
	root, stateHome, projectID, _ := seedAliasOrphanFixtureBase(t)
	retireID := "task:rerunretire00000000001"
	realiasID := "task:rerunrealias0000000001"
	seedTask(t, stateHome, root, projectID, retireID, "Retire On Rerun", "todo", "2026-05-01T00:00:00Z", false, "")
	seedTask(t, stateHome, root, projectID, realiasID, "Realias On Rerun", "todo", "2026-05-01T00:00:00Z", false, "")

	options := AliasOrphanApplyOptions{
		Retire:  []string{retireID},
		Realias: map[string]string{realiasID: "TASK-RERUN"},
		Flags:   []string{"--retire " + retireID, "--realias " + realiasID + "=TASK-RERUN"},
	}
	if _, err := ApplyAliasOrphanMigration(ctx, root, PathResolver{StateHome: stateHome}, options); err != nil {
		t.Fatalf("first ApplyAliasOrphanMigration() error = %v", err)
	}

	second, err := ApplyAliasOrphanMigration(ctx, root, PathResolver{StateHome: stateHome}, options)
	if err != nil {
		t.Fatalf("second ApplyAliasOrphanMigration() error = %v", err)
	}
	if len(second.Warnings) != 0 {
		t.Fatalf("second apply warnings = %v, want none", second.Warnings)
	}
	if second.Totals.EntitiesRetired != 0 || second.Totals.AliasesInserted != 0 {
		t.Fatalf("second apply totals = %#v, want a no-op", second.Totals)
	}
	if entityExists(t, stateHome, root, "tasks", retireID) {
		t.Fatal("the retired row came back")
	}
	if !aliasPointsTo(t, stateHome, root, projectID, "task", "TASK-RERUN", realiasID) {
		t.Fatal("the realiased row lost its alias")
	}
}

// Preview reports the source rows the retire set will strand — the ceremony's
// go/no-go reads that number before any apply.
func TestAliasOrphanPreviewReportsOrphanedSources(t *testing.T) {
	ctx := context.Background()
	root, stateHome, projectID, path := seedAliasOrphanFixtureBase(t)
	legacyID := hex.EncodeToString(sha256Sum(path))
	alias := "SPEC-SOURCES"
	twinID := stableMigrationID("spec", projectID, alias)
	orphanID := stableMigrationID("spec", legacyID, alias)

	seedSpec(t, stateHome, root, projectID, twinID, "Sourced Spec", "active", "2026-06-24T13:03:00Z", true, alias)
	seedSpec(t, stateHome, root, projectID, orphanID, "Sourced Spec", "active", "2026-06-13T10:00:00Z", false, "")
	seedSpecResidue(t, stateHome, root, projectID, orphanID)

	preview, err := PreviewAliasOrphanMigration(ctx, root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("PreviewAliasOrphanMigration() error = %v", err)
	}
	if preview.Totals.OrphanedSources != 1 {
		t.Fatalf("preview orphaned sources = %d, want 1", preview.Totals.OrphanedSources)
	}
	found := false
	for _, project := range preview.Projects {
		for _, table := range project.Tables {
			if table.Table == "specs" && table.OrphanedSources == 1 {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("specs table did not report its orphaned source: %#v", preview.Projects)
	}
	if !entityExists(t, stateHome, root, "specs", orphanID) {
		t.Fatal("preview simulation leaked onto the live database")
	}
	if !entityExists(t, stateHome, root, "sources", stableMigrationID("source", projectID, "specs/"+orphanID+".md")) {
		t.Fatal("preview simulation deleted a live source row")
	}
}

// Rollback restores the archived report byte-identically, updated_at included.
func TestAliasOrphanRollbackRestoresUpdatedAt(t *testing.T) {
	ctx := context.Background()
	root, stateHome, projectID, _ := seedAliasOrphanFixtureBase(t)
	seedReport(t, stateHome, root, projectID, brokenEvidenceReportID, "Transitional TypeScript Surfaces — Do Not Deepen", "2026-06-20T00:00:00Z", "transitional-surfaces-do-not-deepen")
	mustExecOpen(t, stateHome, root, `UPDATE reports SET status = 'active', updated_at = ? WHERE id = ?`, "2026-06-20T00:00:00Z", brokenEvidenceReportID)

	applied, err := ApplyAliasOrphanMigration(ctx, root, PathResolver{StateHome: stateHome}, AliasOrphanApplyOptions{})
	if err != nil {
		t.Fatalf("ApplyAliasOrphanMigration() error = %v", err)
	}
	if _, err := RollbackAliasOrphanMigration(ctx, root, PathResolver{StateHome: stateHome}, applied.RollbackManifestPath); err != nil {
		t.Fatalf("RollbackAliasOrphanMigration() error = %v", err)
	}
	status, updatedAt := reportStatusAndUpdatedAt(t, stateHome, root, brokenEvidenceReportID)
	if status != "active" || updatedAt != "2026-06-20T00:00:00Z" {
		t.Fatalf("report after rollback = %q/%q, want active/2026-06-20T00:00:00Z", status, updatedAt)
	}
}

// The housekeeping scanner counts shaping drafts and the importer gives them
// aliases, so they can orphan exactly like the other six tables. Detector and
// repair have to reach them or the count-agreement receipt is not a receipt.
func TestAliasOrphanCoversShapingDrafts(t *testing.T) {
	ctx := context.Background()
	root, stateHome, projectID, path := seedAliasOrphanFixtureBase(t)
	legacyID := hex.EncodeToString(sha256Sum(path))
	alias := "shape-token-rotation"
	twinID := stableMigrationID("shaping_draft", projectID, alias)
	orphanID := stableMigrationID("shaping_draft", legacyID, alias)

	seedShapingDraft(t, stateHome, root, projectID, twinID, "Token Rotation Shape", "2026-06-24T13:03:00Z", alias)
	seedShapingDraft(t, stateHome, root, projectID, orphanID, "Token Rotation Shape", "2026-06-13T10:00:00Z", "")

	store := openTestStore(t, root, stateHome)
	parity, err := InspectAliasParity(ctx, store)
	store.Close()
	if err != nil {
		t.Fatalf("InspectAliasParity() error = %v", err)
	}
	drafts := findAliasParityTable(t, parity, projectID, "shaping_drafts")
	if drafts.RawCount != 2 || drafts.AliasReachableCount != 1 || drafts.OrphanDelta != 1 {
		t.Fatalf("shaping_drafts parity = %#v, want raw=2 reachable=1 orphan=1", drafts)
	}

	preview, err := PreviewAliasOrphanMigration(ctx, root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("PreviewAliasOrphanMigration() error = %v", err)
	}
	if got := aliasOrphanClassification(t, preview, orphanID); got.Proof != aliasOrphanProofDerivation || got.TwinID != twinID {
		t.Fatalf("shaping draft classification = %#v, want derivation against %s", got, twinID)
	}

	if _, err := ApplyAliasOrphanMigration(ctx, root, PathResolver{StateHome: stateHome}, AliasOrphanApplyOptions{}); err != nil {
		t.Fatalf("ApplyAliasOrphanMigration() error = %v", err)
	}
	if entityExists(t, stateHome, root, "shaping_drafts", orphanID) {
		t.Fatal("orphan shaping draft survived apply")
	}
	if !entityExists(t, stateHome, root, "shaping_drafts", twinID) {
		t.Fatal("the alias-holding shaping draft was retired")
	}
}

// --- fixture helpers ---

func seedShapingDraft(t *testing.T, stateHome string, root project.Root, projectID, id, title, createdAt, alias string) {
	t.Helper()
	mustExecOpen(t, stateHome, root, `
INSERT INTO shaping_drafts (id, project_id, title, status, body_source_id, created_at, updated_at)
VALUES (?, ?, ?, 'draft', NULL, ?, ?)
`, id, projectID, title, createdAt, createdAt)
	if alias != "" {
		mustExecOpen(t, stateHome, root, `
INSERT INTO aliases (id, project_id, entity_kind, entity_id, namespace, alias, created_at, updated_at)
VALUES (?, ?, 'shaping_draft', ?, 'shaping_draft', ?, ?, ?)
`, stableMigrationID("alias", projectID, "shaping_draft", alias), projectID, id, alias, createdAt, createdAt)
	}
}

func aliasOrphanClassification(t *testing.T, result AliasOrphanMigrationResult, entityID string) AliasOrphanRowClassify {
	t.Helper()
	for _, project := range result.Projects {
		for _, table := range project.Tables {
			for _, c := range table.Classifications {
				if c.EntityID == entityID {
					return c
				}
			}
		}
	}
	t.Fatalf("no classification for %s", entityID)
	return AliasOrphanRowClassify{}
}

func seedArtifactBody(t *testing.T, stateHome string, root project.Root, projectID, kind, entityID, content, hash string) {
	t.Helper()
	bodyID := stableMigrationID("artifact_body", projectID, kind, entityID, "markdown")
	mustExecOpen(t, stateHome, root, `
INSERT INTO artifact_bodies (id, project_id, entity_kind, entity_id, body_kind, content, content_hash, source_id, created_at, updated_at)
VALUES (?, ?, ?, ?, 'markdown', ?, ?, NULL, ?, ?)
`, bodyID, projectID, kind, entityID, content, hash, "2026-06-13T10:00:00Z", "2026-06-13T10:00:00Z")
	store := openTestStore(t, root, stateHome)
	defer store.Close()
	var rowID int64
	if err := store.db.QueryRow(`SELECT rowid FROM artifact_bodies WHERE id = ?`, bodyID).Scan(&rowID); err != nil {
		t.Fatalf("read body rowid: %v", err)
	}
	if _, err := store.db.Exec(`INSERT INTO artifact_search(rowid, project_id, entity_kind, entity_id, body_kind, content) VALUES (?, ?, ?, ?, 'markdown', ?)`, rowID, projectID, kind, entityID, content); err != nil {
		t.Fatalf("insert artifact_search: %v", err)
	}
}

func seedSource(t *testing.T, stateHome string, root project.Root, projectID, id, path string) {
	t.Helper()
	mustExecOpen(t, stateHome, root, `
INSERT INTO sources (id, project_id, source_kind, path, hash, imported_at, created_at, updated_at)
VALUES (?, ?, 'markdown', ?, 'hash', ?, ?, ?)
`, id, projectID, path, "2026-06-13T10:00:00Z", "2026-06-13T10:00:00Z", "2026-06-13T10:00:00Z")
}

func seedSpark(t *testing.T, stateHome string, root project.Root, projectID, id, text, sourceID, createdAt, alias string) {
	t.Helper()
	mustExecOpen(t, stateHome, root, `
INSERT INTO sparks (id, project_id, scope, status, text, source_id, created_at, updated_at)
VALUES (?, ?, 'scope', 'open', ?, ?, ?, ?)
`, id, projectID, text, sourceID, createdAt, createdAt)
	if alias != "" {
		mustExecOpen(t, stateHome, root, `
INSERT INTO aliases (id, project_id, entity_kind, entity_id, namespace, alias, created_at, updated_at)
VALUES (?, ?, 'spark', ?, 'spark', ?, ?, ?)
`, stableMigrationID("alias", projectID, "spark", alias), projectID, id, alias, createdAt, createdAt)
	}
}

func seedReport(t *testing.T, stateHome string, root project.Root, projectID, id, title, createdAt, alias string) {
	t.Helper()
	mustExecOpen(t, stateHome, root, `
INSERT INTO reports (id, project_id, report_kind, title, status, body_source_id, created_at, updated_at)
VALUES (?, ?, 'audit', ?, 'final', NULL, ?, ?)
`, id, projectID, title, createdAt, createdAt)
	if alias != "" {
		mustExecOpen(t, stateHome, root, `
INSERT INTO aliases (id, project_id, entity_kind, entity_id, namespace, alias, created_at, updated_at)
VALUES (?, ?, 'report', ?, 'report', ?, ?, ?)
`, stableMigrationID("alias", projectID, "report", alias), projectID, id, alias, createdAt, createdAt)
	}
}

func deferralSparkID(t *testing.T, stateHome string, root project.Root, projectID, operationKey string) string {
	t.Helper()
	store := openTestStore(t, root, stateHome)
	defer store.Close()
	var sparkID string
	if err := store.db.QueryRow(`SELECT spark_id FROM journal_deferrals WHERE project_id = ? AND operation_key = ?`, projectID, operationKey).Scan(&sparkID); err != nil {
		t.Fatalf("read journal_deferrals.spark_id: %v", err)
	}
	return sparkID
}

func reportStatusAndUpdatedAt(t *testing.T, stateHome string, root project.Root, id string) (string, string) {
	t.Helper()
	store := openTestStore(t, root, stateHome)
	defer store.Close()
	var status, updatedAt string
	if err := store.db.QueryRow(`SELECT status, updated_at FROM reports WHERE id = ?`, id).Scan(&status, &updatedAt); err != nil {
		t.Fatalf("read report: %v", err)
	}
	return status, updatedAt
}
