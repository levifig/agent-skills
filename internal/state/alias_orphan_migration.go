package state

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/levifig/loaf/internal/project"
)

const (
	AliasOrphanMigrationActionDryRun   = "dry-run"
	AliasOrphanMigrationActionApply    = "apply"
	AliasOrphanMigrationActionRollback = "rollback"

	aliasOrphanMigrationName = "alias-orphans"

	aliasOrphanProofDerivation = "derivation"
	// aliasOrphanProofSourceDerivation recomputes the orphan's own source row
	// under a legacy salt and then binds it to a holder by content. The salt half
	// is derivation; the binding half is content identity, so the manifest labels
	// it as its own class instead of borrowing the stronger name.
	aliasOrphanProofSourceDerivation = "source-derivation"
	aliasOrphanProofContentIdentity  = "content-identity"
	aliasOrphanProofUnproven         = "unproven"

	aliasOrphanDispositionRetire       = "retire"
	aliasOrphanDispositionRealias      = "realias"
	aliasOrphanDispositionArchiveMoot  = "archive-as-moot"
	aliasOrphanDispositionDeleteDangle = "delete-dangling-alias"

	// brokenEvidenceReportID is the named per-row disposition for the bodyless
	// report whose evidence is unrecoverable (SPEC-047 already shipped the
	// simplification it guarded).
	brokenEvidenceReportID = "report:7644bb23d2664de93b6cb6a5"

	aliasOrphanArchiveMootEventType = "status_normalized"
	aliasOrphanArchiveMootNote      = "evidence unrecoverable; archived as moot — SPEC-047 shipped the simplification this report guarded against deepening"

	// The 2026-06-24 re-import wrote its rows in one tight instant cluster; rows created later that
	// same day are real work, not re-import twins, so membership is a window and not a date prefix.
	june24ReimportWindowStart = "2026-06-24T13:03:00Z"
	june24ReimportWindowEnd   = "2026-06-24T13:04:00Z"

	// The 2026-06-13 original import wrote at one instant; bodyless content-identity twins use this
	// window so two empty fingerprints are not treated as equal evidence outside the import event.
	june13OriginalImportWindowStart = "2026-06-13T01:39:00Z"
	june13OriginalImportWindowEnd   = "2026-06-13T01:46:00Z"
)

// AliasOrphanMigrationResult is the preview/apply/rollback outcome for the
// alias-orphan repair migration. Classification spans every project in the
// global database.
type AliasOrphanMigrationResult struct {
	ContractVersion      int                         `json:"contract_version"`
	DatabaseScope        string                      `json:"database_scope"`
	DatabasePath         string                      `json:"database_path"`
	ProjectID            string                      `json:"project_id,omitempty"`
	ProjectName          string                      `json:"project_name,omitempty"`
	ProjectCurrentPath   string                      `json:"project_current_path,omitempty"`
	Action               string                      `json:"action"`
	Applied              bool                        `json:"applied"`
	CopyRun              bool                        `json:"copy_run"`
	BackupPath           string                      `json:"backup_path,omitempty"`
	RollbackManifestPath string                      `json:"rollback_manifest_path,omitempty"`
	Projects             []AliasOrphanProjectSummary `json:"projects"`
	Totals               AliasOrphanCounts           `json:"totals"`
	Dispositions         []AliasOrphanDisposition    `json:"dispositions,omitempty"`
	OperatorFlags        []string                    `json:"operator_flags,omitempty"`
	Warnings             []string                    `json:"warnings,omitempty"`
	RowsRestored         int                         `json:"rows_restored,omitempty"`
}

// AliasOrphanProjectSummary reports classification for one project.
type AliasOrphanProjectSummary struct {
	ProjectID          string                    `json:"project_id"`
	ProjectName        string                    `json:"project_name,omitempty"`
	ProjectCurrentPath string                    `json:"project_current_path,omitempty"`
	LegacyProjectID    string                    `json:"legacy_project_id,omitempty"`
	LegacyProjectIDs   []string                  `json:"legacy_project_ids,omitempty"`
	LegacyPaths        []string                  `json:"legacy_paths,omitempty"`
	Tables             []AliasOrphanTableSummary `json:"tables"`
	Counts             AliasOrphanCounts         `json:"counts"`
	Dispositions       []AliasOrphanDisposition  `json:"dispositions,omitempty"`
}

// AliasOrphanTableSummary reports per-entity-table classification counts.
// Retire counts the rows this run would retire — proven twins plus any row the
// operator named with --retire. Unproven counts rows no proof reached, whatever
// disposition the operator then supplied for them, so the two overlap by design.
type AliasOrphanTableSummary struct {
	Kind             string                   `json:"kind"`
	Table            string                   `json:"table"`
	Orphans          int                      `json:"orphans"`
	Retire           int                      `json:"retire"`
	Unproven         int                      `json:"unproven"`
	DanglingAliases  int                      `json:"dangling_aliases"`
	OrphanedSources  int                      `json:"orphaned_sources,omitempty"`
	Classifications  []AliasOrphanRowClassify `json:"classifications,omitempty"`
	DanglingAliasIDs []string                 `json:"dangling_alias_ids,omitempty"`
}

// AliasOrphanCounts aggregates migration action counts.
type AliasOrphanCounts struct {
	Orphans           int `json:"orphans"`
	Retire            int `json:"retire"`
	Unproven          int `json:"unproven"`
	DanglingAliases   int `json:"dangling_aliases"`
	OrphanedSources   int `json:"orphaned_sources"`
	NamedDispositions int `json:"named_dispositions,omitempty"`
	OperatorRetire    int `json:"operator_retire,omitempty"`
	OperatorRealias   int `json:"operator_realias,omitempty"`
	EntitiesRetired   int `json:"entities_retired,omitempty"`
	AliasesDeleted    int `json:"aliases_deleted,omitempty"`
	SourcesDeleted    int `json:"sources_deleted,omitempty"`
	StatusesChanged   int `json:"statuses_changed,omitempty"`
	AliasesInserted   int `json:"aliases_inserted,omitempty"`
}

// AliasOrphanRowClassify is one orphan entity's classification.
type AliasOrphanRowClassify struct {
	ProjectID       string `json:"project_id"`
	Kind            string `json:"kind"`
	Table           string `json:"table"`
	EntityID        string `json:"entity_id"`
	Title           string `json:"title,omitempty"`
	Proof           string `json:"proof"`
	TwinID          string `json:"twin_id,omitempty"`
	TwinAlias       string `json:"twin_alias,omitempty"`
	LegacyProjectID string `json:"legacy_project_id,omitempty"`
	LegacyPath      string `json:"legacy_path,omitempty"`
	Disposition     string `json:"disposition,omitempty"`
}

// AliasOrphanDisposition is a planned action against a specific row.
type AliasOrphanDisposition struct {
	ProjectID       string `json:"project_id"`
	Kind            string `json:"kind,omitempty"`
	EntityID        string `json:"entity_id"`
	Action          string `json:"action"`
	Alias           string `json:"alias,omitempty"`
	Proof           string `json:"proof,omitempty"`
	TwinID          string `json:"twin_id,omitempty"`
	TwinAlias       string `json:"twin_alias,omitempty"`
	LegacyProjectID string `json:"legacy_project_id,omitempty"`
	LegacyPath      string `json:"legacy_path,omitempty"`
	Note            string `json:"note,omitempty"`
	Flag            string `json:"flag,omitempty"`
}

// AliasOrphanApplyOptions carries explicit per-row operator dispositions for apply.
type AliasOrphanApplyOptions struct {
	Retire  []string          // entity IDs
	Realias map[string]string // entity ID → alias
	Flags   []string          // verbatim flag strings for the manifest
}

// AliasOrphanRollbackManifest preserves every deleted/changed row for rollback.
type AliasOrphanRollbackManifest struct {
	ContractVersion      int                       `json:"contract_version"`
	Migration            string                    `json:"migration"`
	CreatedAt            string                    `json:"created_at"`
	DatabaseScope        string                    `json:"database_scope"`
	DatabasePath         string                    `json:"database_path"`
	OperatorFlags        []string                  `json:"operator_flags,omitempty"`
	OperatorDispositions []AliasOrphanDisposition  `json:"operator_dispositions,omitempty"`
	Retirements          []AliasOrphanDisposition  `json:"retirements,omitempty"`
	DeletedRows          []AliasOrphanDeletedRow   `json:"deleted_rows"`
	StatusChanges        []AliasOrphanStatusChange `json:"status_changes,omitempty"`
	AliasInserts         []AliasOrphanAliasInsert  `json:"alias_inserts,omitempty"`
	Unlinks              []AliasOrphanUnlink       `json:"unlinks,omitempty"`
	Counts               AliasOrphanCounts         `json:"counts"`
	Metadata             map[string]string         `json:"metadata,omitempty"`
}

// AliasOrphanDeletedRow is one full row snapshot for rollback restore.
type AliasOrphanDeletedRow struct {
	Table   string            `json:"table"`
	Columns []string          `json:"columns"`
	Values  []any             `json:"values"`
	Order   int               `json:"order"`
	Meta    map[string]string `json:"meta,omitempty"`
}

// AliasOrphanStatusChange records a status rewrite for rollback.
type AliasOrphanStatusChange struct {
	ProjectID         string `json:"project_id"`
	Table             string `json:"table"`
	Kind              string `json:"kind"`
	EntityID          string `json:"entity_id"`
	PreviousStatus    string `json:"previous_status"`
	PreviousUpdatedAt string `json:"previous_updated_at,omitempty"`
	NewStatus         string `json:"new_status"`
	EventID           string `json:"event_id"`
	EventNote         string `json:"event_note"`
}

// AliasOrphanAliasInsert records an alias created by --realias for rollback.
type AliasOrphanAliasInsert struct {
	ProjectID  string `json:"project_id"`
	AliasID    string `json:"alias_id"`
	EntityKind string `json:"entity_kind"`
	EntityID   string `json:"entity_id"`
	Namespace  string `json:"namespace"`
	Alias      string `json:"alias"`
}

// AliasOrphanUnlink records a reference rewrite for rollback restore. NewID is
// empty when the column was nulled and carries the twin's ID when the reference
// was repointed at the surviving row.
type AliasOrphanUnlink struct {
	Table      string `json:"table"`
	ProjectID  string `json:"project_id"`
	Column     string `json:"column"`
	KeyColumn  string `json:"key_column,omitempty"`
	RowID      string `json:"row_id"`
	PreviousID string `json:"previous_id"`
	NewID      string `json:"new_id,omitempty"`
}

type aliasOrphanEntityTable struct {
	kind         string
	table        string
	titleColumn  string
	sourceColumn string
	namespace    string
}

var aliasOrphanEntityTables = []aliasOrphanEntityTable{
	{kind: "task", table: "tasks", titleColumn: "title", sourceColumn: "body_source_id", namespace: "task"},
	{kind: "spec", table: "specs", titleColumn: "title", sourceColumn: "body_source_id", namespace: "spec"},
	{kind: "report", table: "reports", titleColumn: "title", sourceColumn: "body_source_id", namespace: "report"},
	{kind: "idea", table: "ideas", titleColumn: "title", sourceColumn: "body_source_id", namespace: "idea"},
	{kind: "spark", table: "sparks", titleColumn: "text", sourceColumn: "source_id", namespace: "spark"},
	{kind: "brainstorm", table: "brainstorms", titleColumn: "title", sourceColumn: "body_source_id", namespace: "brainstorm"},
	{kind: "shaping_draft", table: "shaping_drafts", titleColumn: "title", sourceColumn: "body_source_id", namespace: "shaping_draft"},
}

// aliasOrphanDeadAliasPredicate distinguishes a dead alias from a forward
// reference. An alias with no entity row is damage only when nothing in the
// project still names that entity: the importer legitimately registers the alias
// of a referenced-but-unimported artifact (a `depends_on` naming a task with no
// file) so the reference renders as `TASK-000` instead of an opaque ID, and it
// deletes the edge when the reference leaves the markdown. What survives that
// deletion — an alias nothing points at, its row long gone — is the wreckage
// this migration collects. Placeholders are formatted with the entity table.
const aliasOrphanDeadAliasPredicate = `
  AND NOT EXISTS (
    SELECT 1 FROM %s AS e
    WHERE e.project_id = a.project_id AND e.id = a.entity_id
  )
  AND NOT EXISTS (
    SELECT 1 FROM relationships AS r
    WHERE r.project_id = a.project_id
      AND ((r.from_entity_kind = a.entity_kind AND r.from_entity_id = a.entity_id)
        OR (r.to_entity_kind = a.entity_kind AND r.to_entity_id = a.entity_id))
  )`

const aliasOrphanDanglingAliasQuery = `
SELECT a.id
FROM aliases AS a
WHERE a.project_id = ?
  AND a.entity_kind = ?
  AND a.namespace = ?` + aliasOrphanDeadAliasPredicate + `
ORDER BY a.id
`

// aliasOrphanTableForKind returns the entity-table descriptor this migration
// classifies for a kind.
func aliasOrphanTableForKind(kind string) (aliasOrphanEntityTable, bool) {
	for _, table := range aliasOrphanEntityTables {
		if table.kind == kind {
			return table, true
		}
	}
	return aliasOrphanEntityTable{}, false
}

// PreviewAliasOrphanMigration classifies alias-orphans against a temporary copy.
// Options may include operator --retire / --realias dispositions so the ceremony
// invocation can be rehearsed before --apply.
func PreviewAliasOrphanMigration(ctx context.Context, root project.Root, resolver PathResolver, options AliasOrphanApplyOptions) (AliasOrphanMigrationResult, error) {
	status, err := requireAliasOrphanMigrationStatus(root, resolver)
	if err != nil {
		return AliasOrphanMigrationResult{}, err
	}
	source, err := OpenStoreReadOnly(status.DatabasePath)
	if err != nil {
		return AliasOrphanMigrationResult{}, err
	}
	defer source.Close()

	tempDir, err := os.MkdirTemp("", "loaf-alias-orphan-migration-*")
	if err != nil {
		return AliasOrphanMigrationResult{}, fmt.Errorf("create alias-orphan migration temp dir: %w", err)
	}
	defer os.RemoveAll(tempDir)
	copyPath := filepath.Join(tempDir, "state.sqlite")
	if err := copySQLiteDatabase(ctx, source, copyPath, 0o600); err != nil {
		return AliasOrphanMigrationResult{}, err
	}
	copyStore, err := OpenStore(copyPath)
	if err != nil {
		return AliasOrphanMigrationResult{}, err
	}
	defer copyStore.Close()

	result, manifest, err := planAliasOrphanMigration(ctx, copyStore, aliasOrphanMigrationBaseResult(status, AliasOrphanMigrationActionDryRun), options, aliasOrphanExecutedDispositions{})
	if err != nil {
		return AliasOrphanMigrationResult{}, err
	}
	result.OperatorFlags = append([]string{}, options.Flags...)
	// The copy is disposable, so the repair runs against it for real. That is
	// the only way the preview can report the source rows the retire set will
	// strand — the blast radius the go/no-go decision reads.
	if err := applyAliasOrphanMigrationManifest(ctx, copyStore, &manifest, nil); err != nil {
		return AliasOrphanMigrationResult{}, fmt.Errorf("simulate alias-orphan migration: %w", err)
	}
	applyAliasOrphanSimulationProjection(&result, manifest)
	result.CopyRun = true
	return result, nil
}

// applyAliasOrphanSimulationProjection folds the simulated run's collateral back
// into the plan, per table and in total: the source rows the retire set strands,
// and the aliases the retirements themselves kill. Neither is visible to
// classification — both only exist once the repair has run — so the preview
// reads them off the simulation instead, and the operator sees the same blast
// radius apply will produce.
func applyAliasOrphanSimulationProjection(result *AliasOrphanMigrationResult, manifest AliasOrphanRollbackManifest) {
	sourcesByTable := map[string]int{}
	aliasesByTable := map[string][]string{}
	for _, row := range manifest.DeletedRows {
		projectID := rowValueString(row, "project_id")
		switch row.Table {
		case "sources":
			sourcesByTable[projectID+"\x00"+row.Meta["entity_kind"]]++
		case "aliases":
			key := projectID + "\x00" + rowValueString(row, "entity_kind")
			aliasesByTable[key] = append(aliasesByTable[key], rowValueString(row, "id"))
		}
	}
	result.Totals.OrphanedSources = manifest.Counts.OrphanedSources
	result.Totals.SourcesDeleted = manifest.Counts.SourcesDeleted
	result.Totals.DanglingAliases = 0
	result.Dispositions = nil
	for i := range result.Projects {
		project := &result.Projects[i]
		project.Counts.DanglingAliases = 0
		for j := range project.Tables {
			table := &project.Tables[j]
			key := project.ProjectID + "\x00" + table.Kind
			table.OrphanedSources = sourcesByTable[key]
			project.Counts.OrphanedSources += table.OrphanedSources

			known := map[string]struct{}{}
			for _, aliasID := range table.DanglingAliasIDs {
				known[aliasID] = struct{}{}
			}
			for _, aliasID := range aliasesByTable[key] {
				if _, seen := known[aliasID]; seen {
					continue
				}
				known[aliasID] = struct{}{}
				table.DanglingAliasIDs = append(table.DanglingAliasIDs, aliasID)
				project.Dispositions = append(project.Dispositions, AliasOrphanDisposition{
					ProjectID: project.ProjectID,
					Kind:      table.Kind,
					EntityID:  aliasID,
					Action:    aliasOrphanDispositionDeleteDangle,
				})
			}
			sort.Strings(table.DanglingAliasIDs)
			table.DanglingAliases = len(table.DanglingAliasIDs)
			project.Counts.DanglingAliases += table.DanglingAliases
		}
		result.Totals.DanglingAliases += project.Counts.DanglingAliases
		result.Dispositions = append(result.Dispositions, project.Dispositions...)
	}
}

// ApplyAliasOrphanMigration backs up, writes a rollback manifest, and repairs alias-orphans.
func ApplyAliasOrphanMigration(ctx context.Context, root project.Root, resolver PathResolver, options AliasOrphanApplyOptions) (AliasOrphanMigrationResult, error) {
	status, err := requireAliasOrphanMigrationStatus(root, resolver)
	if err != nil {
		return AliasOrphanMigrationResult{}, err
	}
	backup, err := Backup(ctx, root, resolver)
	if err != nil {
		return AliasOrphanMigrationResult{}, err
	}
	store, err := openInitializedStore(root, resolver)
	if err != nil {
		return AliasOrphanMigrationResult{}, err
	}
	defer store.Close()

	executed, err := readAliasOrphanExecutedDispositions(filepath.Dir(backup.BackupPath))
	if err != nil {
		return AliasOrphanMigrationResult{}, err
	}
	result, manifest, err := planAliasOrphanMigration(ctx, store, aliasOrphanMigrationBaseResult(status, AliasOrphanMigrationActionApply), options, executed)
	if err != nil {
		return AliasOrphanMigrationResult{}, err
	}
	result.BackupPath = backup.BackupPath
	result.OperatorFlags = append([]string{}, options.Flags...)
	// A disposition that names nothing and was never carried out is a typo, and
	// silently applying the rest would leave the operator believing a row was
	// handled.
	if len(result.Warnings) > 0 {
		return result, fmt.Errorf("alias-orphan dispositions matched no rows: %s", strings.Join(result.Warnings, "; "))
	}
	result.Applied = true

	// The row snapshots are captured inside the transaction and the rollback
	// manifest is written to disk before COMMIT, so no deletion is ever visible
	// without its restore record: backup → manifest → apply.
	manifestPath := ""
	if err := applyAliasOrphanMigrationManifest(ctx, store, &manifest, func(final AliasOrphanRollbackManifest) error {
		if !aliasOrphanManifestHasWork(final) {
			return nil
		}
		path, err := writeAliasOrphanRollbackManifest(final, filepath.Dir(backup.BackupPath), time.Now().UTC())
		if err != nil {
			return err
		}
		manifestPath = path
		return nil
	}); err != nil {
		if manifestPath != "" {
			os.Remove(manifestPath)
		}
		// Nothing committed: the backup is the only artifact this run left.
		result.Applied = false
		return result, err
	}
	result.RollbackManifestPath = manifestPath
	result.Totals.EntitiesRetired = manifest.Counts.EntitiesRetired
	result.Totals.AliasesDeleted = manifest.Counts.AliasesDeleted
	result.Totals.SourcesDeleted = manifest.Counts.SourcesDeleted
	result.Totals.OrphanedSources = manifest.Counts.OrphanedSources
	result.Totals.StatusesChanged = manifest.Counts.StatusesChanged
	result.Totals.AliasesInserted = manifest.Counts.AliasesInserted

	// The repair is committed from here on, so every failure has to hand back the
	// backup and the rollback manifest — they are the operator's way out.
	verify, _, err := planAliasOrphanMigration(ctx, store, aliasOrphanMigrationBaseResult(status, AliasOrphanMigrationActionApply), AliasOrphanApplyOptions{}, executed)
	if err != nil {
		return result, fmt.Errorf("post-apply verification: %w (backup %s, rollback manifest %s)", err, result.BackupPath, result.RollbackManifestPath)
	}
	if verify.Totals.Retire > 0 || verify.Totals.DanglingAliases > 0 {
		return result, fmt.Errorf("post-apply verification failed: %d retire-class orphans and %d dangling aliases remain (backup %s, rollback manifest %s)", verify.Totals.Retire, verify.Totals.DanglingAliases, result.BackupPath, result.RollbackManifestPath)
	}
	return result, nil
}

// RollbackAliasOrphanMigration restores rows recorded in an alias-orphan rollback manifest.
func RollbackAliasOrphanMigration(ctx context.Context, root project.Root, resolver PathResolver, manifestPath string) (AliasOrphanMigrationResult, error) {
	if manifestPath == "" {
		return AliasOrphanMigrationResult{}, fmt.Errorf("alias-orphan rollback requires a manifest path")
	}
	status, err := requireAliasOrphanMigrationStatus(root, resolver)
	if err != nil {
		return AliasOrphanMigrationResult{}, err
	}
	manifest, err := readAliasOrphanRollbackManifest(manifestPath)
	if err != nil {
		return AliasOrphanMigrationResult{}, err
	}
	backup, err := Backup(ctx, root, resolver)
	if err != nil {
		return AliasOrphanMigrationResult{}, err
	}
	store, err := openInitializedStore(root, resolver)
	if err != nil {
		return AliasOrphanMigrationResult{}, err
	}
	defer store.Close()

	result := aliasOrphanMigrationBaseResult(status, AliasOrphanMigrationActionRollback)
	result.Applied = true
	result.BackupPath = backup.BackupPath
	result.RollbackManifestPath = manifestPath
	result.OperatorFlags = append([]string{}, manifest.OperatorFlags...)
	if err := rollbackAliasOrphanMigrationManifest(ctx, store, manifest, &result); err != nil {
		return AliasOrphanMigrationResult{}, err
	}
	return result, nil
}

func aliasOrphanMigrationBaseResult(status Status, action string) AliasOrphanMigrationResult {
	return AliasOrphanMigrationResult{
		ContractVersion:    StateJSONContractVersion,
		DatabaseScope:      "global",
		DatabasePath:       status.DatabasePath,
		ProjectID:          status.ProjectID,
		ProjectName:        status.ProjectName,
		ProjectCurrentPath: status.ProjectCurrentPath,
		Action:             action,
		Projects:           []AliasOrphanProjectSummary{},
	}
}

func requireAliasOrphanMigrationStatus(root project.Root, resolver PathResolver) (Status, error) {
	status, err := Inspect(root, resolver)
	if err != nil {
		return Status{}, err
	}
	switch status.Mode {
	case ModeSQLiteReady:
		return status, nil
	case ModeMarkdownOnly:
		return Status{}, fmt.Errorf("SQLite state database is not initialized; run `loaf state migrate markdown --apply` first")
	case ModeInvalid:
		return Status{}, fmt.Errorf("state database is invalid; run `loaf state doctor`")
	default:
		return Status{}, fmt.Errorf("state database is not ready; run `loaf state status`")
	}
}

func planAliasOrphanMigration(ctx context.Context, store *Store, result AliasOrphanMigrationResult, options AliasOrphanApplyOptions, executed aliasOrphanExecutedDispositions) (AliasOrphanMigrationResult, AliasOrphanRollbackManifest, error) {
	manifest := AliasOrphanRollbackManifest{
		ContractVersion: StateJSONContractVersion,
		Migration:       aliasOrphanMigrationName,
		CreatedAt:       time.Now().UTC().Format(time.RFC3339Nano),
		DatabaseScope:   result.DatabaseScope,
		DatabasePath:    result.DatabasePath,
		OperatorFlags:   append([]string{}, options.Flags...),
		DeletedRows:     []AliasOrphanDeletedRow{},
		Metadata: map[string]string{
			"broken_evidence_report_id": brokenEvidenceReportID,
		},
	}

	retireSet, realiasSet := aliasOrphanOperatorSets(options)
	for _, id := range sortedKeys(retireSet) {
		manifest.OperatorDispositions = append(manifest.OperatorDispositions, AliasOrphanDisposition{
			EntityID: id,
			Action:   aliasOrphanDispositionRetire,
			Flag:     "--retire " + id,
		})
	}
	for _, id := range sortedKeys(realiasSet) {
		manifest.OperatorDispositions = append(manifest.OperatorDispositions, AliasOrphanDisposition{
			EntityID: id,
			Action:   aliasOrphanDispositionRealias,
			Alias:    realiasSet[id],
			Flag:     "--realias " + id + "=" + realiasSet[id],
		})
	}

	projects, err := store.ListProjects(ctx)
	if err != nil {
		return result, manifest, err
	}

	matched := map[string]struct{}{}
	for _, project := range projects.Projects {
		salts, err := aliasOrphanLegacySalts(ctx, store.db, project)
		if err != nil {
			return result, manifest, err
		}
		summary, err := classifyAliasOrphansForProject(ctx, store.db, project, salts, retireSet, realiasSet)
		if err != nil {
			return result, manifest, err
		}
		result.Projects = append(result.Projects, summary)
		result.Totals.Orphans += summary.Counts.Orphans
		result.Totals.Retire += summary.Counts.Retire
		result.Totals.Unproven += summary.Counts.Unproven
		result.Totals.DanglingAliases += summary.Counts.DanglingAliases
		result.Totals.NamedDispositions += summary.Counts.NamedDispositions
		result.Totals.OperatorRetire += summary.Counts.OperatorRetire
		result.Totals.OperatorRealias += summary.Counts.OperatorRealias
		result.Dispositions = append(result.Dispositions, summary.Dispositions...)
		for _, table := range summary.Tables {
			for _, c := range table.Classifications {
				matched[c.EntityID] = struct{}{}
			}
		}
	}
	warnings, err := aliasOrphanUnmatchedDispositionWarnings(ctx, store.db, retireSet, realiasSet, matched, executed)
	if err != nil {
		return result, manifest, err
	}
	result.Warnings = append(result.Warnings, warnings...)

	manifest.Counts = AliasOrphanCounts{
		Orphans:           result.Totals.Orphans,
		Retire:            result.Totals.Retire,
		Unproven:          result.Totals.Unproven,
		DanglingAliases:   result.Totals.DanglingAliases,
		NamedDispositions: result.Totals.NamedDispositions,
		OperatorRetire:    result.Totals.OperatorRetire,
		OperatorRealias:   result.Totals.OperatorRealias,
	}
	return result, manifest, nil
}

func aliasOrphanOperatorSets(options AliasOrphanApplyOptions) (map[string]struct{}, map[string]string) {
	retireSet := map[string]struct{}{}
	for _, id := range options.Retire {
		if id = strings.TrimSpace(id); id != "" {
			retireSet[id] = struct{}{}
		}
	}
	realiasSet := map[string]string{}
	for id, alias := range options.Realias {
		id = strings.TrimSpace(id)
		alias = strings.TrimSpace(alias)
		if id != "" && alias != "" {
			realiasSet[id] = alias
		}
	}
	return retireSet, realiasSet
}

// aliasOrphanUnmatchedDispositionWarnings names every operator flag that no
// classified orphan answered, so a typo is reported instead of ignored. A flag
// an earlier run already carried out is not a typo: rerunning the exact command
// the ceremony recorded has to be a no-op, so a disposition whose end state is
// already in place is passed over silently.
func aliasOrphanUnmatchedDispositionWarnings(ctx context.Context, q aliasOrphanQuerier, retireSet map[string]struct{}, realiasSet map[string]string, matched map[string]struct{}, executed aliasOrphanExecutedDispositions) ([]string, error) {
	var warnings []string
	for _, id := range sortedKeys(retireSet) {
		if _, ok := matched[id]; ok {
			continue
		}
		done, err := executed.retirementCarriedOut(ctx, q, id)
		if err != nil {
			return nil, err
		}
		if done {
			continue
		}
		warnings = append(warnings, fmt.Sprintf("--retire %s matched no alias-orphan row", id))
	}
	for _, id := range sortedKeys(realiasSet) {
		if _, ok := matched[id]; ok {
			continue
		}
		done, err := aliasOrphanAliasNames(ctx, q, id, realiasSet[id])
		if err != nil {
			return nil, err
		}
		if done {
			continue
		}
		warnings = append(warnings, fmt.Sprintf("--realias %s=%s matched no alias-orphan row", id, realiasSet[id]))
	}
	return warnings, nil
}

// aliasOrphanExecutedDispositions is the set of operator dispositions earlier
// runs of this migration recorded in their rollback manifests. The manifests are
// the only durable evidence that a row was retired on purpose — the row itself
// is gone — so they are what separates a rerun from a typo.
type aliasOrphanExecutedDispositions struct {
	retired map[string]struct{}
}

// readAliasOrphanExecutedDispositions collects the entity IDs earlier alias-orphan
// runs retired, from the rollback manifests kept beside the backups. A directory
// that cannot be read yields an empty set: no evidence is not evidence of a typo
// either way, and the caller still refuses the disposition.
func readAliasOrphanExecutedDispositions(dir string) (aliasOrphanExecutedDispositions, error) {
	executed := aliasOrphanExecutedDispositions{retired: map[string]struct{}{}}
	if dir == "" {
		return executed, nil
	}
	paths, err := filepath.Glob(filepath.Join(dir, "alias-orphan-rollback-*.json"))
	if err != nil {
		return executed, fmt.Errorf("list alias-orphan rollback manifests: %w", err)
	}
	for _, path := range paths {
		payload, err := os.ReadFile(path)
		if err != nil {
			return executed, fmt.Errorf("read alias-orphan rollback manifest %s: %w", path, err)
		}
		var manifest struct {
			Retirements []AliasOrphanDisposition `json:"retirements"`
		}
		if err := json.Unmarshal(payload, &manifest); err != nil {
			// A manifest this migration cannot parse is not proof of anything;
			// leaving it out only makes the disposition check stricter.
			continue
		}
		for _, retirement := range manifest.Retirements {
			executed.retired[retirement.EntityID] = struct{}{}
		}
	}
	return executed, nil
}

// retirementCarriedOut reports whether an earlier run retired this entity and
// the row is still gone.
func (e aliasOrphanExecutedDispositions) retirementCarriedOut(ctx context.Context, q aliasOrphanQuerier, entityID string) (bool, error) {
	if _, ok := e.retired[entityID]; !ok {
		return false, nil
	}
	exists, err := aliasOrphanEntityRowExists(ctx, q, entityID)
	if err != nil {
		return false, err
	}
	return !exists, nil
}

// aliasOrphanEntityRowExists looks for an entity row with this ID in any table
// the migration classifies, in any project.
func aliasOrphanEntityRowExists(ctx context.Context, q aliasOrphanQuerier, entityID string) (bool, error) {
	for _, table := range aliasOrphanEntityTables {
		exists, err := sqliteTableExistsQ(ctx, q, table.table)
		if err != nil {
			return false, err
		}
		if !exists {
			continue
		}
		var found int
		query := fmt.Sprintf(`SELECT EXISTS(SELECT 1 FROM %s WHERE id = ?)`, quoteSQLiteIdentifier(table.table))
		if err := q.QueryRowContext(ctx, query, entityID).Scan(&found); err != nil {
			return false, fmt.Errorf("look for %s row %s: %w", table.table, entityID, err)
		}
		if found == 1 {
			return true, nil
		}
	}
	return false, nil
}

// aliasOrphanAliasNames reports whether the alias already names this entity —
// the end state --realias asks for.
func aliasOrphanAliasNames(ctx context.Context, q aliasOrphanQuerier, entityID string, alias string) (bool, error) {
	var found int
	if err := q.QueryRowContext(ctx, `
SELECT EXISTS(SELECT 1 FROM aliases WHERE entity_id = ? AND alias = ?)
`, entityID, alias).Scan(&found); err != nil {
		return false, fmt.Errorf("look for alias %s on %s: %w", alias, entityID, err)
	}
	return found == 1, nil
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// aliasOrphanQuerier is satisfied by both *sql.DB and *sql.Tx so classification
// runs from exactly one implementation on the preview and apply paths.
type aliasOrphanQuerier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// aliasOrphanLegacySalt is one historical project ID the entity IDs of this
// project may have been derived under, with the path that produced it.
type aliasOrphanLegacySalt struct {
	projectID string
	path      string
}

func classifyAliasOrphansForProject(ctx context.Context, q aliasOrphanQuerier, project ProjectIdentity, salts []aliasOrphanLegacySalt, retireSet map[string]struct{}, realiasSet map[string]string) (AliasOrphanProjectSummary, error) {
	summary := AliasOrphanProjectSummary{
		ProjectID:          project.ID,
		ProjectName:        project.FriendlyName,
		ProjectCurrentPath: project.CurrentPath,
		Tables:             []AliasOrphanTableSummary{},
		Dispositions:       []AliasOrphanDisposition{},
	}
	for _, salt := range salts {
		summary.LegacyProjectIDs = append(summary.LegacyProjectIDs, salt.projectID)
		summary.LegacyPaths = append(summary.LegacyPaths, salt.path)
		if salt.path == project.CurrentPath {
			summary.LegacyProjectID = salt.projectID
		}
	}
	if summary.LegacyProjectID == "" && len(salts) > 0 {
		summary.LegacyProjectID = salts[0].projectID
	}

	for _, table := range aliasOrphanEntityTables {
		exists, err := sqliteTableExistsQ(ctx, q, table.table)
		if err != nil {
			return summary, err
		}
		if !exists {
			continue
		}
		tableSummary, err := classifyAliasOrphansForTable(ctx, q, project.ID, salts, table, retireSet, realiasSet)
		if err != nil {
			return summary, err
		}
		summary.Tables = append(summary.Tables, tableSummary)
		summary.Counts.Orphans += tableSummary.Orphans
		summary.Counts.Retire += tableSummary.Retire
		summary.Counts.Unproven += tableSummary.Unproven
		summary.Counts.DanglingAliases += tableSummary.DanglingAliases
		for _, c := range tableSummary.Classifications {
			switch {
			case c.Disposition == aliasOrphanDispositionRetire && c.Proof != aliasOrphanProofUnproven:
				summary.Dispositions = append(summary.Dispositions, aliasOrphanRetireDisposition(c))
			case c.Disposition == aliasOrphanDispositionRetire:
				summary.Counts.OperatorRetire++
				d := aliasOrphanRetireDisposition(c)
				d.Flag = "--retire " + c.EntityID
				summary.Dispositions = append(summary.Dispositions, d)
			case c.Disposition == aliasOrphanDispositionRealias:
				summary.Counts.OperatorRealias++
				summary.Dispositions = append(summary.Dispositions, AliasOrphanDisposition{
					ProjectID: project.ID,
					Kind:      c.Kind,
					EntityID:  c.EntityID,
					Action:    aliasOrphanDispositionRealias,
					Alias:     realiasSet[c.EntityID],
					Proof:     c.Proof,
					Flag:      "--realias " + c.EntityID + "=" + realiasSet[c.EntityID],
				})
			}
		}
		for _, aliasID := range tableSummary.DanglingAliasIDs {
			summary.Dispositions = append(summary.Dispositions, AliasOrphanDisposition{
				ProjectID: project.ID,
				Kind:      table.kind,
				EntityID:  aliasID,
				Action:    aliasOrphanDispositionDeleteDangle,
			})
		}
	}

	named, err := classifyBrokenEvidenceReport(ctx, q, project.ID)
	if err != nil {
		return summary, err
	}
	if named != nil {
		summary.Dispositions = append(summary.Dispositions, *named)
		summary.Counts.NamedDispositions++
	}
	return summary, nil
}

func aliasOrphanRetireDisposition(c AliasOrphanRowClassify) AliasOrphanDisposition {
	return AliasOrphanDisposition{
		ProjectID:       c.ProjectID,
		Kind:            c.Kind,
		EntityID:        c.EntityID,
		Action:          aliasOrphanDispositionRetire,
		Proof:           c.Proof,
		TwinID:          c.TwinID,
		TwinAlias:       c.TwinAlias,
		LegacyProjectID: c.LegacyProjectID,
		LegacyPath:      c.LegacyPath,
		Note:            c.TwinAlias,
	}
}

// aliasOrphanRow is an entity row on either side of a twin proof.
type aliasOrphanRow struct {
	entityID   string
	alias      string
	title      string
	createdAt  string
	sourceID   string
	sourcePath string
}

func classifyAliasOrphansForTable(ctx context.Context, q aliasOrphanQuerier, projectID string, salts []aliasOrphanLegacySalt, table aliasOrphanEntityTable, retireSet map[string]struct{}, realiasSet map[string]string) (AliasOrphanTableSummary, error) {
	summary := AliasOrphanTableSummary{
		Kind:            table.kind,
		Table:           table.table,
		Classifications: []AliasOrphanRowClassify{},
	}

	orphans, err := readAliasOrphanRows(ctx, q, projectID, table, false)
	if err != nil {
		return summary, err
	}
	holders, err := readAliasOrphanRows(ctx, q, projectID, table, true)
	if err != nil {
		return summary, err
	}

	holdersByTitle := map[string][]aliasOrphanRow{}
	// derivedIDs maps a legacy-salt recomputation of an alias holder's ID onto
	// the holder it proves. This is the only recomputation in the codebase and
	// it runs against historical salts, never to resolve a live entity.
	derivedIDs := map[string]aliasOrphanDerivedTwin{}
	for _, h := range holders {
		holdersByTitle[h.title] = append(holdersByTitle[h.title], h)
		for _, salt := range salts {
			derived := stableMigrationID(table.kind, salt.projectID, h.alias)
			if derived == h.entityID {
				continue
			}
			if _, taken := derivedIDs[derived]; taken {
				continue
			}
			derivedIDs[derived] = aliasOrphanDerivedTwin{holder: h, salt: salt}
		}
	}

	// Uniqueness for content-identity and source-derivation is computed over the
	// orphans not yet retiring. Retiring one row can unlock another, so the
	// classification iterates to a fixed point: each pass may promote more rows
	// into the retiring set, then counts recompute over the smaller pool.
	// Operator --retire seeds that set immediately; --realias never does, so a
	// row named for preservation cannot free a competitor's uniqueness.
	retiringForCounts := map[string]struct{}{}
	for id := range retireSet {
		retiringForCounts[id] = struct{}{}
	}
	frozen := map[string]AliasOrphanRowClassify{}

	for {
		orphanTitleCounts := map[string]int{}
		orphanSourceKeyCounts := map[string]int{}
		for _, orphan := range orphans {
			if _, retiring := retiringForCounts[orphan.entityID]; retiring {
				continue
			}
			orphanTitleCounts[orphan.title]++
			orphanSourceKeyCounts[aliasOrphanSourceKey(orphan)]++
		}

		promoted := 0
		pass := map[string]AliasOrphanRowClassify{}
		for _, orphan := range orphans {
			if existing, ok := frozen[orphan.entityID]; ok {
				pass[orphan.entityID] = existing
				continue
			}
			classify, err := classifyOneAliasOrphan(ctx, q, projectID, table, orphan, holders, holdersByTitle, derivedIDs, salts, orphanTitleCounts, orphanSourceKeyCounts, retireSet, realiasSet)
			if err != nil {
				return summary, err
			}
			pass[orphan.entityID] = classify
			if classify.Disposition != aliasOrphanDispositionRetire {
				continue
			}
			// --realias outranks automatic retire; disposition would not be retire.
			if _, already := retiringForCounts[orphan.entityID]; !already {
				promoted++
			}
			frozen[orphan.entityID] = classify
			retiringForCounts[orphan.entityID] = struct{}{}
		}
		if promoted == 0 {
			summary.Orphans = len(orphans)
			for _, orphan := range orphans {
				classify := pass[orphan.entityID]
				if classify.Proof == aliasOrphanProofUnproven {
					summary.Unproven++
				}
				if classify.Disposition == aliasOrphanDispositionRetire {
					summary.Retire++
				}
				summary.Classifications = append(summary.Classifications, classify)
			}
			break
		}
	}

	dangling, err := readDeadAliasIDs(ctx, q, projectID, table)
	if err != nil {
		return summary, err
	}
	summary.DanglingAliasIDs = dangling
	summary.DanglingAliases = len(summary.DanglingAliasIDs)

	return summary, nil
}

func classifyOneAliasOrphan(
	ctx context.Context,
	q aliasOrphanQuerier,
	projectID string,
	table aliasOrphanEntityTable,
	orphan aliasOrphanRow,
	holders []aliasOrphanRow,
	holdersByTitle map[string][]aliasOrphanRow,
	derivedIDs map[string]aliasOrphanDerivedTwin,
	salts []aliasOrphanLegacySalt,
	orphanTitleCounts map[string]int,
	orphanSourceKeyCounts map[string]int,
	retireSet map[string]struct{},
	realiasSet map[string]string,
) (AliasOrphanRowClassify, error) {
	classify := AliasOrphanRowClassify{
		ProjectID: projectID,
		Kind:      table.kind,
		Table:     table.table,
		EntityID:  orphan.entityID,
		Title:     orphan.title,
		Proof:     aliasOrphanProofUnproven,
	}
	if twin, ok := aliasOrphanDerivationTwin(orphan, derivedIDs); ok {
		classify.Proof = aliasOrphanProofDerivation
		classify.TwinID = twin.holder.entityID
		classify.TwinAlias = twin.holder.alias
		classify.LegacyProjectID = twin.salt.projectID
		classify.LegacyPath = twin.salt.path
	} else if twin, salt, ok := aliasOrphanSourceSaltTwin(orphan, holders, salts, orphanSourceKeyCounts); ok {
		classify.Proof = aliasOrphanProofSourceDerivation
		classify.TwinID = twin.entityID
		classify.TwinAlias = twin.alias
		classify.LegacyProjectID = salt.projectID
		classify.LegacyPath = salt.path
	} else {
		twin, ok, err := aliasOrphanContentIdentityTwin(ctx, q, projectID, table, orphan, holdersByTitle, orphanTitleCounts)
		if err != nil {
			return classify, err
		}
		if ok {
			classify.Proof = aliasOrphanProofContentIdentity
			classify.TwinID = twin.entityID
			classify.TwinAlias = twin.alias
		}
	}
	// An explicit operator disposition outranks the automatic one. A proof
	// that the row has a twin is not a licence to delete a row the operator
	// named for preservation.
	_, retireRequested := retireSet[orphan.entityID]
	switch {
	case realiasSet[orphan.entityID] != "":
		classify.Disposition = aliasOrphanDispositionRealias
	case retireRequested:
		classify.Disposition = aliasOrphanDispositionRetire
	case classify.Proof != aliasOrphanProofUnproven:
		classify.Disposition = aliasOrphanDispositionRetire
	}
	return classify, nil
}

// readDeadAliasIDs lists the dead aliases of one entity table: no entity row and
// nothing left in the project naming the entity.
func readDeadAliasIDs(ctx context.Context, q aliasOrphanQuerier, projectID string, table aliasOrphanEntityTable) ([]string, error) {
	rows, err := q.QueryContext(ctx, fmt.Sprintf(aliasOrphanDanglingAliasQuery, quoteSQLiteIdentifier(table.table)), projectID, table.kind, table.namespace)
	if err != nil {
		return nil, fmt.Errorf("scan %s dangling aliases: %w", table.table, err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var aliasID string
		if err := rows.Scan(&aliasID); err != nil {
			return nil, fmt.Errorf("scan %s dangling alias: %w", table.table, err)
		}
		ids = append(ids, aliasID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("scan %s dangling aliases: %w", table.table, err)
	}
	return ids, nil
}

// readAliasOrphanRows returns either the alias-orphaned rows of a table or the
// rows that hold an alias, with their source path attached.
func readAliasOrphanRows(ctx context.Context, q aliasOrphanQuerier, projectID string, table aliasOrphanEntityTable, aliasHolders bool) ([]aliasOrphanRow, error) {
	title := quoteSQLiteIdentifier(table.titleColumn)
	entity := quoteSQLiteIdentifier(table.table)
	source := quoteSQLiteIdentifier(table.sourceColumn)
	var query string
	if aliasHolders {
		query = fmt.Sprintf(`
SELECT a.entity_id, a.alias, e.%s, e.created_at, COALESCE(e.%s, ''), COALESCE(s.path, '')
FROM aliases AS a
JOIN %s AS e ON e.project_id = a.project_id AND e.id = a.entity_id
LEFT JOIN sources AS s ON s.project_id = e.project_id AND s.id = e.%s
WHERE a.project_id = ? AND a.entity_kind = ? AND a.namespace = ?
ORDER BY a.alias
`, title, source, entity, source)
	} else {
		query = fmt.Sprintf(`
SELECT e.id, '', e.%s, e.created_at, COALESCE(e.%s, ''), COALESCE(s.path, '')
FROM %s AS e
LEFT JOIN sources AS s ON s.project_id = e.project_id AND s.id = e.%s
WHERE e.project_id = ?
  AND NOT EXISTS (
    SELECT 1 FROM aliases AS a
    WHERE a.project_id = e.project_id
      AND a.entity_kind = ?
      AND a.entity_id = e.id
      AND a.namespace = ?
  )
ORDER BY e.id
`, title, source, entity, source)
	}
	rows, err := q.QueryContext(ctx, query, projectID, table.kind, table.namespace)
	if err != nil {
		return nil, fmt.Errorf("scan %s rows: %w", table.table, err)
	}
	defer rows.Close()
	var out []aliasOrphanRow
	for rows.Next() {
		var row aliasOrphanRow
		if err := rows.Scan(&row.entityID, &row.alias, &row.title, &row.createdAt, &row.sourceID, &row.sourcePath); err != nil {
			return nil, fmt.Errorf("scan %s row: %w", table.table, err)
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("scan %s rows: %w", table.table, err)
	}
	return out, nil
}

// aliasOrphanDerivedTwin binds a legacy-salt recomputation to the alias holder
// it proves, and the salt that produced the match.
type aliasOrphanDerivedTwin struct {
	holder aliasOrphanRow
	salt   aliasOrphanLegacySalt
}

// aliasOrphanDerivationTwin accepts the salt recomputation as a twin proof only
// when the two rows are the same artifact, not merely the same alias.
//
// Recomputation proves the orphan was minted under a legacy salt *for this
// alias* — it says nothing about whether the row now holding that alias is the
// orphan's duplicate. Alias numbers get reused: delete TASK-042 and the next
// task file to claim the number recomputes to exactly the orphan's ID, and ID
// match alone would then retire a row that is nobody's duplicate. Two guards
// close that, both from data already in hand: the rows must agree on their
// title, and the orphan must predate the row it would retire into — the damage
// this migration repairs always leaves the pre-rekey original as the older of
// the pair. A reuse victim fails one or both and becomes an operator decision
// instead of a silent deletion.
//
// Do not add body-fingerprint equality to this proof. The calibrated production
// case has derivation-proven pairs whose stored bodies genuinely differ: the
// artifacts changed between the June-13 originals and the June-24 re-imports.
// Title equality plus orphan-predates-twin is the guard; a content check would
// refuse every one of those pairs and break the repair.
func aliasOrphanDerivationTwin(orphan aliasOrphanRow, derivedIDs map[string]aliasOrphanDerivedTwin) (aliasOrphanDerivedTwin, bool) {
	twin, ok := derivedIDs[orphan.entityID]
	if !ok {
		return aliasOrphanDerivedTwin{}, false
	}
	if orphan.title != twin.holder.title {
		return aliasOrphanDerivedTwin{}, false
	}
	if orphan.createdAt == "" || orphan.createdAt >= twin.holder.createdAt {
		return aliasOrphanDerivedTwin{}, false
	}
	return twin, true
}

// aliasOrphanSourceKey identifies a row by the content and file that produced
// it, the pair the source-salt proof binds an orphan to its twin with.
func aliasOrphanSourceKey(row aliasOrphanRow) string {
	return row.title + "\x00" + row.sourcePath
}

// aliasOrphanSourceSaltTwin proves twin-ship for rows whose IDs were never
// derived from an alias — sparks are minted from (path, line) — by recomputing
// the row's own source ID under each historical salt. A match proves the orphan
// was minted before the rekey; the surviving twin is then the unique alias
// holder that carries the same content from the same source path. Uniqueness is
// required on both sides: many orphans collapsing onto one holder is a merge,
// not a twin proof, and stays unproven. The holder must also sit in the June-24
// re-import window and the orphan must predate it — the same gates the other
// auto-retiring proofs apply.
func aliasOrphanSourceSaltTwin(orphan aliasOrphanRow, holders []aliasOrphanRow, salts []aliasOrphanLegacySalt, orphanSourceKeyCounts map[string]int) (aliasOrphanRow, aliasOrphanLegacySalt, bool) {
	if orphan.sourceID == "" || orphan.sourcePath == "" || strings.TrimSpace(orphan.title) == "" {
		return aliasOrphanRow{}, aliasOrphanLegacySalt{}, false
	}
	if orphanSourceKeyCounts[aliasOrphanSourceKey(orphan)] != 1 {
		return aliasOrphanRow{}, aliasOrphanLegacySalt{}, false
	}
	var matched aliasOrphanLegacySalt
	found := false
	for _, salt := range salts {
		if stableMigrationID("source", salt.projectID, orphan.sourcePath) == orphan.sourceID {
			matched = salt
			found = true
			break
		}
	}
	if !found {
		return aliasOrphanRow{}, aliasOrphanLegacySalt{}, false
	}
	var twin aliasOrphanRow
	matches := 0
	for _, h := range holders {
		if h.title != orphan.title || h.sourcePath != orphan.sourcePath {
			continue
		}
		twin = h
		matches++
	}
	if matches != 1 {
		return aliasOrphanRow{}, aliasOrphanLegacySalt{}, false
	}
	if !inTimestampWindow(twin.createdAt, june24ReimportWindowStart, june24ReimportWindowEnd) {
		return aliasOrphanRow{}, aliasOrphanLegacySalt{}, false
	}
	if orphan.createdAt == "" || orphan.createdAt >= twin.createdAt {
		return aliasOrphanRow{}, aliasOrphanLegacySalt{}, false
	}
	return twin, matched, true
}

// aliasOrphanContentIdentityTwin is the distinctly-labeled fallback proof. It
// requires the surviving alias holder to be a member of the 2026-06-24
// re-import window, the orphan to predate it, exactly one candidate on each
// side, a non-empty title, and matching body evidence. Bodyless pairs need the
// orphan in the June-13 original-import window; bodyful pairs need equal
// fingerprints. Anything short of that stays unproven.
func aliasOrphanContentIdentityTwin(ctx context.Context, q aliasOrphanQuerier, projectID string, table aliasOrphanEntityTable, orphan aliasOrphanRow, holdersByTitle map[string][]aliasOrphanRow, orphanTitleCounts map[string]int) (aliasOrphanRow, bool, error) {
	if strings.TrimSpace(orphan.title) == "" {
		return aliasOrphanRow{}, false, nil
	}
	if orphanTitleCounts[orphan.title] != 1 {
		return aliasOrphanRow{}, false, nil
	}
	holders := holdersByTitle[orphan.title]
	if len(holders) != 1 {
		return aliasOrphanRow{}, false, nil
	}
	twin := holders[0]
	if !inTimestampWindow(twin.createdAt, june24ReimportWindowStart, june24ReimportWindowEnd) {
		return aliasOrphanRow{}, false, nil
	}
	if orphan.createdAt == "" || orphan.createdAt >= twin.createdAt {
		return aliasOrphanRow{}, false, nil
	}
	orphanBodies, err := aliasOrphanBodyFingerprint(ctx, q, projectID, table.kind, orphan.entityID)
	if err != nil {
		return aliasOrphanRow{}, false, err
	}
	twinBodies, err := aliasOrphanBodyFingerprint(ctx, q, projectID, table.kind, twin.entityID)
	if err != nil {
		return aliasOrphanRow{}, false, err
	}
	orphanBodyless := orphanBodies == ""
	twinBodyless := twinBodies == ""
	switch {
	case orphanBodyless && twinBodyless:
		if !inTimestampWindow(orphan.createdAt, june13OriginalImportWindowStart, june13OriginalImportWindowEnd) {
			return aliasOrphanRow{}, false, nil
		}
	case !orphanBodyless && !twinBodyless:
		if orphanBodies != twinBodies {
			return aliasOrphanRow{}, false, nil
		}
	default:
		return aliasOrphanRow{}, false, nil
	}
	return twin, true, nil
}

// aliasOrphanBodyFingerprint canonicalizes an entity's stored bodies as
// body_kind=content_hash pairs so two rows can be compared for content
// identity. An entity with no bodies fingerprints as the empty string.
func aliasOrphanBodyFingerprint(ctx context.Context, q aliasOrphanQuerier, projectID string, kind string, entityID string) (string, error) {
	rows, err := q.QueryContext(ctx, `
SELECT body_kind, COALESCE(content_hash, '')
FROM artifact_bodies
WHERE project_id = ? AND entity_kind = ? AND entity_id = ?
ORDER BY body_kind
`, projectID, kind, entityID)
	if err != nil {
		return "", fmt.Errorf("read %s body fingerprint: %w", kind, err)
	}
	defer rows.Close()
	var parts []string
	for rows.Next() {
		var bodyKind, hash string
		if err := rows.Scan(&bodyKind, &hash); err != nil {
			return "", fmt.Errorf("scan %s body fingerprint: %w", kind, err)
		}
		parts = append(parts, bodyKind+"="+hash)
	}
	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("read %s body fingerprint: %w", kind, err)
	}
	sort.Strings(parts)
	return strings.Join(parts, "\n"), nil
}

func classifyBrokenEvidenceReport(ctx context.Context, q aliasOrphanQuerier, projectID string) (*AliasOrphanDisposition, error) {
	var status string
	err := q.QueryRowContext(ctx, `
SELECT status FROM reports WHERE project_id = ? AND id = ?
`, projectID, brokenEvidenceReportID).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read broken-evidence report: %w", err)
	}
	if status == LifecycleStatusArchived {
		return nil, nil
	}
	return &AliasOrphanDisposition{
		ProjectID: projectID,
		Kind:      "report",
		EntityID:  brokenEvidenceReportID,
		Action:    aliasOrphanDispositionArchiveMoot,
		Note:      aliasOrphanArchiveMootNote,
	}, nil
}

// inTimestampWindow reports whether timestamp is in the half-open interval
// [start, end). Values that fail RFC3339 parse are not in the window.
func inTimestampWindow(timestamp, start, end string) bool {
	ts, err := time.Parse(time.RFC3339, timestamp)
	if err != nil {
		return false
	}
	startAt, err := time.Parse(time.RFC3339, start)
	if err != nil {
		return false
	}
	endAt, err := time.Parse(time.RFC3339, end)
	if err != nil {
		return false
	}
	return !ts.Before(startAt) && ts.Before(endAt)
}

// aliasOrphanLegacySalts returns every historical project ID this project's
// rows could have been derived under: the current path plus every path the
// project has ever been recorded at.
func aliasOrphanLegacySalts(ctx context.Context, q aliasOrphanQuerier, project ProjectIdentity) ([]aliasOrphanLegacySalt, error) {
	paths := []string{}
	seen := map[string]struct{}{}
	add := func(path string) {
		path = strings.TrimSpace(path)
		if path == "" {
			return
		}
		if _, ok := seen[path]; ok {
			return
		}
		seen[path] = struct{}{}
		paths = append(paths, path)
	}
	add(project.CurrentPath)

	exists, err := sqliteTableExistsQ(ctx, q, "project_paths")
	if err != nil {
		return nil, err
	}
	if exists {
		rows, err := q.QueryContext(ctx, `SELECT path FROM project_paths WHERE project_id = ? ORDER BY is_current DESC, path`, project.ID)
		if err != nil {
			return nil, fmt.Errorf("list historical project paths: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var path string
			if err := rows.Scan(&path); err != nil {
				return nil, fmt.Errorf("scan historical project path: %w", err)
			}
			add(path)
		}
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("list historical project paths: %w", err)
		}
	}

	salts := make([]aliasOrphanLegacySalt, 0, len(paths))
	for _, path := range paths {
		salts = append(salts, aliasOrphanLegacySalt{projectID: legacyProjectIDFromPath(path), path: path})
	}
	return salts, nil
}

func legacyProjectIDFromPath(path string) string {
	sum := sha256.Sum256([]byte(path))
	return hex.EncodeToString(sum[:])
}

func aliasOrphanManifestHasWork(manifest AliasOrphanRollbackManifest) bool {
	return len(manifest.DeletedRows) > 0 ||
		len(manifest.StatusChanges) > 0 ||
		len(manifest.AliasInserts) > 0 ||
		len(manifest.Unlinks) > 0 ||
		manifest.Counts.EntitiesRetired > 0 ||
		manifest.Counts.AliasesDeleted > 0 ||
		manifest.Counts.SourcesDeleted > 0 ||
		manifest.Counts.StatusesChanged > 0 ||
		manifest.Counts.AliasesInserted > 0
}

// applyAliasOrphanMigrationManifest performs the repair in one transaction.
// beforeCommit runs with the fully-populated manifest still inside the
// transaction, so the rollback record is durable on disk before any deletion
// becomes visible; a failure there aborts the repair.
func applyAliasOrphanMigrationManifest(ctx context.Context, store *Store, manifest *AliasOrphanRollbackManifest, beforeCommit func(AliasOrphanRollbackManifest) error) error {
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin alias-orphan migration: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `PRAGMA defer_foreign_keys = ON`); err != nil {
		return fmt.Errorf("defer foreign keys: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	order := 0

	// Re-classify inside the transaction against live state and apply.
	projects, err := listProjectsTx(ctx, tx, store.path)
	if err != nil {
		return err
	}

	retireSet := map[string]struct{}{}
	realiasSet := map[string]string{}
	for _, d := range manifest.OperatorDispositions {
		switch d.Action {
		case aliasOrphanDispositionRetire:
			retireSet[d.EntityID] = struct{}{}
		case aliasOrphanDispositionRealias:
			realiasSet[d.EntityID] = d.Alias
		}
	}

	// Dead aliases go first so a --realias target freed by this same run is
	// available, and so realias never has to distinguish a live claim from a
	// dead one.
	if err := collectDeadAliasesTx(ctx, tx, projects, manifest, &order); err != nil {
		return err
	}

	for _, project := range projects {
		salts, err := aliasOrphanLegacySalts(ctx, tx, project)
		if err != nil {
			return err
		}

		// Named disposition: archive broken-evidence report as moot.
		if err := applyBrokenEvidenceArchiveTx(ctx, tx, project.ID, now, manifest); err != nil {
			return err
		}

		for _, table := range aliasOrphanEntityTables {
			exists, err := sqliteTableExistsQ(ctx, tx, table.table)
			if err != nil {
				return err
			}
			if !exists {
				continue
			}

			summary, err := classifyAliasOrphansForTable(ctx, tx, project.ID, salts, table, retireSet, realiasSet)
			if err != nil {
				return err
			}

			for _, c := range summary.Classifications {
				switch c.Disposition {
				case aliasOrphanDispositionRetire:
					if err := retireEntityWithResidueTx(ctx, tx, project.ID, table, c, manifest, &order); err != nil {
						return err
					}
					manifest.Retirements = append(manifest.Retirements, aliasOrphanRetireDisposition(c))
					manifest.Counts.EntitiesRetired++
				case aliasOrphanDispositionRealias:
					alias := realiasSet[c.EntityID]
					if err := realiasEntityTx(ctx, tx, project.ID, table, c.EntityID, alias, now, manifest); err != nil {
						return err
					}
					manifest.Counts.AliasesInserted++
				}
			}
		}
	}

	// Retiring an orphan deletes the relationship edges that were keeping a
	// forward-declared alias alive, so an alias this run itself kills is
	// invisible to any sweep that ran before the retirement — including a later
	// table's, since the table order is fixed. Collect once more after every
	// retirement, across every project and table. Deleting an alias cannot kill
	// another one, so this second pass is the fixed point and post-apply
	// verification is guaranteed clean on a correct run.
	if err := collectDeadAliasesTx(ctx, tx, projects, manifest, &order); err != nil {
		return err
	}

	if beforeCommit != nil {
		if err := beforeCommit(*manifest); err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit alias-orphan migration: %w", err)
	}
	return nil
}

func applyBrokenEvidenceArchiveTx(ctx context.Context, tx *sql.Tx, projectID string, now string, manifest *AliasOrphanRollbackManifest) error {
	var previous, previousUpdatedAt string
	err := tx.QueryRowContext(ctx, `SELECT status, COALESCE(updated_at, '') FROM reports WHERE project_id = ? AND id = ?`, projectID, brokenEvidenceReportID).Scan(&previous, &previousUpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read broken-evidence report status: %w", err)
	}
	if previous == LifecycleStatusArchived {
		return nil
	}
	if _, err := tx.ExecContext(ctx, `UPDATE reports SET status = ?, updated_at = ? WHERE project_id = ? AND id = ?`, LifecycleStatusArchived, now, projectID, brokenEvidenceReportID); err != nil {
		return fmt.Errorf("archive broken-evidence report: %w", err)
	}
	eventID := stableMigrationID("event", projectID, "report", brokenEvidenceReportID, aliasOrphanArchiveMootEventType, previous, LifecycleStatusArchived, "moot")
	result, err := tx.ExecContext(ctx, `
INSERT OR IGNORE INTO events (id, project_id, entity_kind, entity_id, event_type, from_status, to_status, note, created_at, updated_at)
VALUES (?, ?, 'report', ?, ?, ?, ?, ?, ?, ?)
`, eventID, projectID, brokenEvidenceReportID, aliasOrphanArchiveMootEventType, previous, LifecycleStatusArchived, aliasOrphanArchiveMootNote, now, now)
	if err != nil {
		return fmt.Errorf("record broken-evidence archive event: %w", err)
	}
	recordedEventID := ""
	if rows, rowsErr := result.RowsAffected(); rowsErr == nil && rows > 0 {
		recordedEventID = eventID
	}
	manifest.StatusChanges = append(manifest.StatusChanges, AliasOrphanStatusChange{
		ProjectID:         projectID,
		Table:             "reports",
		Kind:              "report",
		EntityID:          brokenEvidenceReportID,
		PreviousStatus:    previous,
		PreviousUpdatedAt: previousUpdatedAt,
		NewStatus:         LifecycleStatusArchived,
		EventID:           recordedEventID,
		EventNote:         aliasOrphanArchiveMootNote,
	})
	manifest.Counts.StatusesChanged++
	return nil
}

func realiasEntityTx(ctx context.Context, tx *sql.Tx, projectID string, table aliasOrphanEntityTable, entityID string, alias string, now string, manifest *AliasOrphanRollbackManifest) error {
	// Re-pointing a claimed alias at a different row is the exact mechanic that
	// created this damage class. Refuse it: the incumbent keeps its identity and
	// the operator picks a free alias.
	var incumbent string
	err := tx.QueryRowContext(ctx, `
SELECT entity_id FROM aliases WHERE project_id = ? AND namespace = ? AND alias = ?
`, projectID, table.namespace, alias).Scan(&incumbent)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("read alias %s:%s: %w", table.namespace, alias, err)
	}
	if err == nil && incumbent != entityID {
		return fmt.Errorf("realias %s: alias %s:%s already names %s; pick an unclaimed alias or retire the incumbent first", entityID, table.namespace, alias, incumbent)
	}

	aliasID := stableMigrationID("alias", projectID, table.namespace, alias)
	if _, err := tx.ExecContext(ctx, `
INSERT INTO aliases (id, project_id, entity_kind, entity_id, namespace, alias, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(project_id, namespace, alias) DO UPDATE SET
  entity_kind = excluded.entity_kind,
  entity_id = excluded.entity_id,
  updated_at = excluded.updated_at
`, aliasID, projectID, table.kind, entityID, table.namespace, alias, now, now); err != nil {
		return fmt.Errorf("realias %s %s as %s: %w", table.kind, entityID, alias, err)
	}
	manifest.AliasInserts = append(manifest.AliasInserts, AliasOrphanAliasInsert{
		ProjectID:  projectID,
		AliasID:    aliasID,
		EntityKind: table.kind,
		EntityID:   entityID,
		Namespace:  table.namespace,
		Alias:      alias,
	})
	return nil
}

// collectDeadAliasesTx deletes every dead alias in the database, across all
// projects and all entity tables, recording each one in the manifest.
func collectDeadAliasesTx(ctx context.Context, tx *sql.Tx, projects []ProjectIdentity, manifest *AliasOrphanRollbackManifest, order *int) error {
	for _, project := range projects {
		for _, table := range aliasOrphanEntityTables {
			exists, err := sqliteTableExistsQ(ctx, tx, table.table)
			if err != nil {
				return err
			}
			if !exists {
				continue
			}
			aliasIDs, err := readDeadAliasIDs(ctx, tx, project.ID, table)
			if err != nil {
				return err
			}
			for _, aliasID := range aliasIDs {
				if err := deleteDanglingAliasTx(ctx, tx, project.ID, aliasID, manifest, order); err != nil {
					return err
				}
				manifest.Counts.AliasesDeleted++
			}
		}
	}
	return nil
}

func deleteDanglingAliasTx(ctx context.Context, tx *sql.Tx, projectID string, aliasID string, manifest *AliasOrphanRollbackManifest, order *int) error {
	if err := captureRowsTx(ctx, tx, "aliases", `SELECT * FROM aliases WHERE project_id = ? AND id = ?`, []any{projectID, aliasID}, manifest, order, nil); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM aliases WHERE project_id = ? AND id = ?`, projectID, aliasID); err != nil {
		return fmt.Errorf("delete dangling alias %s: %w", aliasID, err)
	}
	return nil
}

// entityReferenceSweep is one polymorphic (entity_kind, entity_id) reference site.
type entityReferenceSweep struct {
	table string
	where string
	args  []any
}

// polymorphicEntityReferenceSweeps returns the six polymorphic tables that can
// cite an entity by (entity_kind, entity_id). where clauses are suitable for
// both captureAndDeleteTx and captureAndDeleteJournalDuplicateTx.
func polymorphicEntityReferenceSweeps(projectID, kind, entityID string) []entityReferenceSweep {
	return []entityReferenceSweep{
		{"events", `WHERE project_id = ? AND entity_kind = ? AND entity_id = ?`, []any{projectID, kind, entityID}},
		{"entity_tags", `WHERE project_id = ? AND entity_kind = ? AND entity_id = ?`, []any{projectID, kind, entityID}},
		{"bundle_members", `WHERE project_id = ? AND entity_kind = ? AND entity_id = ?`, []any{projectID, kind, entityID}},
		{"backend_mappings", `WHERE project_id = ? AND entity_kind = ? AND entity_id = ?`, []any{projectID, kind, entityID}},
		{"exports", `WHERE project_id = ? AND source_entity_kind = ? AND source_entity_id = ?`, []any{projectID, kind, entityID}},
		{"relationships", `WHERE project_id = ? AND ((from_entity_kind = ? AND from_entity_id = ?) OR (to_entity_kind = ? AND to_entity_id = ?))`, []any{projectID, kind, entityID, kind, entityID}},
	}
}

func retireEntityWithResidueTx(ctx context.Context, tx *sql.Tx, projectID string, table aliasOrphanEntityTable, classify AliasOrphanRowClassify, manifest *AliasOrphanRollbackManifest, order *int) error {
	entityID := classify.EntityID
	// Capture and delete artifact bodies (FTS included via delete helper after capture).
	if err := captureRowsTx(ctx, tx, "artifact_bodies", `
SELECT * FROM artifact_bodies WHERE project_id = ? AND entity_kind = ? AND entity_id = ?
`, []any{projectID, table.kind, entityID}, manifest, order, nil); err != nil {
		return err
	}
	if _, _, err := deleteArtifactBodiesForEntityTx(ctx, tx, projectID, table.kind, entityID); err != nil {
		return err
	}

	sweeps := polymorphicEntityReferenceSweeps(projectID, table.kind, entityID)
	for _, op := range sweeps {
		quoted := quoteSQLiteIdentifier(op.table)
		if err := captureRowsTx(ctx, tx, op.table, fmt.Sprintf(`SELECT * FROM %s %s`, quoted, op.where), op.args, manifest, order, nil); err != nil {
			return err
		}
	}
	for _, op := range sweeps {
		quoted := quoteSQLiteIdentifier(op.table)
		if _, err := execCountTx(ctx, tx, fmt.Sprintf(`DELETE FROM %s %s`, quoted, op.where), op.args...); err != nil {
			return fmt.Errorf("delete %s rows for %s %s: %w", op.table, table.kind, entityID, err)
		}
	}

	if err := retireKindSpecificResidueTx(ctx, tx, projectID, table.kind, entityID, classify.TwinID, manifest, order); err != nil {
		return err
	}

	if err := unlinkReferencesToEntityTx(ctx, tx, projectID, table.kind, entityID, manifest); err != nil {
		return err
	}

	// Capture entity source id before deleting the entity row.
	var bodySourceID sql.NullString
	sourceQuery := fmt.Sprintf(`SELECT %s FROM %s WHERE project_id = ? AND id = ?`, quoteSQLiteIdentifier(table.sourceColumn), quoteSQLiteIdentifier(table.table))
	if err := tx.QueryRowContext(ctx, sourceQuery, projectID, entityID).Scan(&bodySourceID); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("read %s source: %w", table.kind, err)
	}

	if err := captureRowsTx(ctx, tx, table.table, fmt.Sprintf(`SELECT * FROM %s WHERE project_id = ? AND id = ?`, quoteSQLiteIdentifier(table.table)), []any{projectID, entityID}, manifest, order, nil); err != nil {
		return err
	}
	if _, err := execCountTx(ctx, tx, fmt.Sprintf(`DELETE FROM %s WHERE project_id = ? AND id = ?`, quoteSQLiteIdentifier(table.table)), projectID, entityID); err != nil {
		return fmt.Errorf("delete %s row %s: %w", table.kind, entityID, err)
	}

	candidateSources := map[string]struct{}{}
	if bodySourceID.Valid && bodySourceID.String != "" {
		candidateSources[bodySourceID.String] = struct{}{}
	}
	// Also consider sources captured from artifact_bodies for this entity.
	for _, row := range manifest.DeletedRows {
		if row.Table != "artifact_bodies" {
			continue
		}
		if sid := rowValueString(row, "source_id"); sid != "" {
			if rowValueString(row, "entity_id") == entityID && rowValueString(row, "entity_kind") == table.kind {
				candidateSources[sid] = struct{}{}
			}
		}
	}
	for sid := range candidateSources {
		referenced, err := sourceStillReferencedTx(ctx, tx, projectID, sid)
		if err != nil {
			return err
		}
		if referenced {
			continue
		}
		if err := captureRowsTx(ctx, tx, "sources", `SELECT * FROM sources WHERE project_id = ? AND id = ?`, []any{projectID, sid}, manifest, order, map[string]string{"entity_kind": table.kind, "entity_id": entityID}); err != nil {
			return err
		}
		count, err := execCountTx(ctx, tx, `DELETE FROM sources WHERE project_id = ? AND id = ?`, projectID, sid)
		if err != nil {
			return fmt.Errorf("delete source %s: %w", sid, err)
		}
		manifest.Counts.SourcesDeleted += count
		manifest.Counts.OrphanedSources += count
	}
	return nil
}

// retireKindSpecificResidueTx clears the references no polymorphic
// (entity_kind, entity_id) sweep can reach: child rows bound by a NOT NULL
// foreign key, and NOT NULL soft references that carry no constraint at all.
// Both would otherwise abort the whole migration at COMMIT or leave the row
// dangling with nothing to detect it.
func retireKindSpecificResidueTx(ctx context.Context, tx *sql.Tx, projectID string, kind string, entityID string, twinID string, manifest *AliasOrphanRollbackManifest, order *int) error {
	switch kind {
	case "spark":
		// spark_id is NOT NULL in both tables and carries no foreign key, so a
		// retirement leaves silent dangling provenance. Repoint at the proven
		// twin where possible; delete the row only when there is nowhere to go.
		for _, ref := range []aliasOrphanSoftRef{
			{table: "journal_deferrals", column: "spark_id", keyColumn: "operation_key", unique: true},
			{table: "intent_operations", column: "spark_id", keyColumn: "operation_key"},
		} {
			if err := repointOrDeleteSoftRefTx(ctx, tx, projectID, ref, entityID, twinID, manifest, order); err != nil {
				return err
			}
		}
	}
	return nil
}

// aliasOrphanSoftRef is a NOT NULL reference to an entity that the schema does
// not enforce. unique marks columns whose value cannot be shared, so a repoint
// has to yield when the twin already holds one.
type aliasOrphanSoftRef struct {
	table     string
	column    string
	keyColumn string
	unique    bool
}

func repointOrDeleteSoftRefTx(ctx context.Context, tx *sql.Tx, projectID string, ref aliasOrphanSoftRef, entityID string, twinID string, manifest *AliasOrphanRollbackManifest, order *int) error {
	exists, err := sqliteTableExistsQ(ctx, tx, ref.table)
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}
	quotedTable := quoteSQLiteIdentifier(ref.table)
	quotedColumn := quoteSQLiteIdentifier(ref.column)
	quotedKey := quoteSQLiteIdentifier(ref.keyColumn)

	rows, err := tx.QueryContext(ctx, fmt.Sprintf(`SELECT %s FROM %s WHERE project_id = ? AND %s = ? ORDER BY %s`, quotedKey, quotedTable, quotedColumn, quotedKey), projectID, entityID)
	if err != nil {
		return fmt.Errorf("list %s.%s references: %w", ref.table, ref.column, err)
	}
	var keys []string
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			rows.Close()
			return fmt.Errorf("scan %s.%s reference: %w", ref.table, ref.column, err)
		}
		keys = append(keys, key)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("list %s.%s references: %w", ref.table, ref.column, err)
	}
	rows.Close()

	for _, key := range keys {
		repoint := twinID != ""
		if repoint && ref.unique {
			var taken int
			if err := tx.QueryRowContext(ctx, fmt.Sprintf(`SELECT EXISTS(SELECT 1 FROM %s WHERE %s = ?)`, quotedTable, quotedColumn), twinID).Scan(&taken); err != nil {
				return fmt.Errorf("check %s.%s availability: %w", ref.table, ref.column, err)
			}
			repoint = taken == 0
		}
		if repoint {
			if _, err := tx.ExecContext(ctx, fmt.Sprintf(`UPDATE %s SET %s = ? WHERE project_id = ? AND %s = ?`, quotedTable, quotedColumn, quotedKey), twinID, projectID, key); err != nil {
				return fmt.Errorf("repoint %s.%s: %w", ref.table, ref.column, err)
			}
			manifest.Unlinks = append(manifest.Unlinks, AliasOrphanUnlink{
				Table:      ref.table,
				ProjectID:  projectID,
				Column:     ref.column,
				KeyColumn:  ref.keyColumn,
				RowID:      key,
				PreviousID: entityID,
				NewID:      twinID,
			})
			continue
		}
		if err := captureAndDeleteTx(ctx, tx, ref.table, fmt.Sprintf(`WHERE project_id = ? AND %s = ?`, quotedKey), []any{projectID, key}, manifest, order); err != nil {
			return err
		}
	}
	return nil
}

// captureAndDeleteTx snapshots every row a predicate selects into the rollback
// manifest and then deletes it.
func captureAndDeleteTx(ctx context.Context, tx *sql.Tx, table string, where string, args []any, manifest *AliasOrphanRollbackManifest, order *int) error {
	quoted := quoteSQLiteIdentifier(table)
	if err := captureRowsTx(ctx, tx, table, fmt.Sprintf(`SELECT * FROM %s %s`, quoted, where), args, manifest, order, nil); err != nil {
		return err
	}
	if _, err := execCountTx(ctx, tx, fmt.Sprintf(`DELETE FROM %s %s`, quoted, where), args...); err != nil {
		return fmt.Errorf("delete %s rows: %w", table, err)
	}
	return nil
}

func unlinkReferencesToEntityTx(ctx context.Context, tx *sql.Tx, projectID string, kind string, entityID string, manifest *AliasOrphanRollbackManifest) error {
	type unlinkSpec struct {
		table  string
		column string
	}
	var unlinkSpecs []unlinkSpec
	switch kind {
	case "spec":
		unlinkSpecs = []unlinkSpec{
			{"tasks", "spec_id"},
			{"journal_entries", "spec_id"},
			{"plans", "spec_id"},
			{"councils", "spec_id"},
		}
	case "task":
		unlinkSpecs = []unlinkSpec{
			{"journal_entries", "task_id"},
			{"handoffs", "task_id"},
		}
	default:
		return nil
	}
	for _, spec := range unlinkSpecs {
		exists, err := sqliteTableExistsQ(ctx, tx, spec.table)
		if err != nil {
			return err
		}
		if !exists {
			continue
		}
		rows, err := tx.QueryContext(ctx, fmt.Sprintf(`SELECT id FROM %s WHERE project_id = ? AND %s = ?`, quoteSQLiteIdentifier(spec.table), quoteSQLiteIdentifier(spec.column)), projectID, entityID)
		if err != nil {
			return fmt.Errorf("list %s.%s unlinks: %w", spec.table, spec.column, err)
		}
		var ids []string
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return err
			}
			ids = append(ids, id)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		rows.Close()
		for _, id := range ids {
			manifest.Unlinks = append(manifest.Unlinks, AliasOrphanUnlink{
				Table:      spec.table,
				ProjectID:  projectID,
				Column:     spec.column,
				KeyColumn:  "id",
				RowID:      id,
				PreviousID: entityID,
			})
		}
		if len(ids) > 0 {
			if _, err := execCountTx(ctx, tx, fmt.Sprintf(`UPDATE %s SET %s = NULL WHERE project_id = ? AND %s = ?`, quoteSQLiteIdentifier(spec.table), quoteSQLiteIdentifier(spec.column), quoteSQLiteIdentifier(spec.column)), projectID, entityID); err != nil {
				return fmt.Errorf("unlink %s.%s: %w", spec.table, spec.column, err)
			}
		}
	}
	return nil
}

func rollbackAliasOrphanMigrationManifest(ctx context.Context, store *Store, manifest AliasOrphanRollbackManifest, result *AliasOrphanMigrationResult) error {
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin alias-orphan rollback: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `PRAGMA defer_foreign_keys = ON`); err != nil {
		return fmt.Errorf("defer foreign keys: %w", err)
	}

	// Undo alias inserts from --realias first.
	for _, insert := range manifest.AliasInserts {
		if _, err := tx.ExecContext(ctx, `DELETE FROM aliases WHERE project_id = ? AND id = ?`, insert.ProjectID, insert.AliasID); err != nil {
			return fmt.Errorf("rollback alias insert %s: %w", insert.AliasID, err)
		}
	}

	// Undo status changes: delete archive events this run inserted and restore previous status.
	for _, change := range manifest.StatusChanges {
		if change.EventID != "" {
			if _, err := tx.ExecContext(ctx, `DELETE FROM events WHERE project_id = ? AND id = ?`, change.ProjectID, change.EventID); err != nil {
				return fmt.Errorf("rollback status event %s: %w", change.EventID, err)
			}
		}
		if change.PreviousUpdatedAt != "" {
			if _, err := tx.ExecContext(ctx, fmt.Sprintf(`UPDATE %s SET status = ?, updated_at = ? WHERE project_id = ? AND id = ?`, quoteSQLiteIdentifier(change.Table)), change.PreviousStatus, change.PreviousUpdatedAt, change.ProjectID, change.EntityID); err != nil {
				return fmt.Errorf("rollback status for %s %s: %w", change.Kind, change.EntityID, err)
			}
			continue
		}
		if _, err := tx.ExecContext(ctx, fmt.Sprintf(`UPDATE %s SET status = ? WHERE project_id = ? AND id = ?`, quoteSQLiteIdentifier(change.Table)), change.PreviousStatus, change.ProjectID, change.EntityID); err != nil {
			return fmt.Errorf("rollback status for %s %s: %w", change.Kind, change.EntityID, err)
		}
	}

	// Restore deleted rows in reverse capture order so parents come before children when needed.
	rows := append([]AliasOrphanDeletedRow{}, manifest.DeletedRows...)
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].Order > rows[j].Order })
	// Prefer restoring entity tables and sources before reference tables? Actually
	// reverse order of deletion: we deleted residue first then entity then sources.
	// Capture order: bodies, polymorphic, entity, sources. Reverse: sources, entity, polymorphic, bodies.
	// Restoring entity before polymorphic is required for FK if not deferred; with defer, any order works at commit.
	// Restore non-archive-event rows; archive events were deleted above via StatusChanges.
	archiveEventIDs := map[string]struct{}{}
	for _, change := range manifest.StatusChanges {
		archiveEventIDs[change.EventID] = struct{}{}
	}
	for _, row := range rows {
		if row.Table == "events" {
			if id := rowValueString(row, "id"); id != "" {
				if _, skip := archiveEventIDs[id]; skip {
					continue
				}
			}
		}
		if err := insertDeletedRowTx(ctx, tx, row); err != nil {
			return err
		}
		if row.Table == "artifact_bodies" {
			if err := restoreArtifactSearchForRowTx(ctx, tx, row); err != nil {
				return err
			}
		}
		result.RowsRestored++
	}

	// Restore nulled and repointed references.
	for _, unlink := range manifest.Unlinks {
		keyColumn := firstNonEmpty(unlink.KeyColumn, "id")
		if _, err := tx.ExecContext(ctx, fmt.Sprintf(`UPDATE %s SET %s = ? WHERE project_id = ? AND %s = ?`, quoteSQLiteIdentifier(unlink.Table), quoteSQLiteIdentifier(unlink.Column), quoteSQLiteIdentifier(keyColumn)), unlink.PreviousID, unlink.ProjectID, unlink.RowID); err != nil {
			return fmt.Errorf("restore unlink %s.%s: %w", unlink.Table, unlink.Column, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit alias-orphan rollback: %w", err)
	}
	return nil
}

func restoreArtifactSearchForRowTx(ctx context.Context, tx *sql.Tx, row AliasOrphanDeletedRow) error {
	return restoreArtifactSearchTx(ctx, tx,
		rowValueString(row, "project_id"),
		rowValueString(row, "entity_kind"),
		rowValueString(row, "entity_id"),
		rowValueString(row, "body_kind"),
		rowValueString(row, "content"),
	)
}

func restoreArtifactSearchTx(ctx context.Context, tx *sql.Tx, projectID, entityKind, entityID, bodyKind, content string) error {
	if projectID == "" || entityKind == "" || entityID == "" {
		return nil
	}
	kind := firstNonEmpty(bodyKind, ArtifactBodyKindMarkdown)
	rowID, err := artifactBodyRowID(ctx, tx, projectID, entityKind, entityID, kind)
	if err != nil {
		return err
	}
	return upsertArtifactSearchTx(ctx, tx, artifactSearchRow{}, false, rowID, projectID, entityKind, entityID, kind, content)
}

func insertDeletedRowTx(ctx context.Context, tx *sql.Tx, row AliasOrphanDeletedRow) error {
	if len(row.Columns) == 0 || len(row.Columns) != len(row.Values) {
		return fmt.Errorf("deleted row for %s has mismatched columns/values", row.Table)
	}
	quoted := make([]string, len(row.Columns))
	placeholders := make([]string, len(row.Columns))
	args := make([]any, len(row.Values))
	for i, col := range row.Columns {
		quoted[i] = quoteSQLiteIdentifier(col)
		placeholders[i] = "?"
		args[i] = normalizeManifestValue(row.Values[i])
	}
	query := fmt.Sprintf(`INSERT OR REPLACE INTO %s (%s) VALUES (%s)`, quoteSQLiteIdentifier(row.Table), strings.Join(quoted, ", "), strings.Join(placeholders, ", "))
	if _, err := tx.ExecContext(ctx, query, args...); err != nil {
		return fmt.Errorf("restore row into %s: %w", row.Table, err)
	}
	return nil
}

func normalizeManifestValue(value any) any {
	switch v := value.(type) {
	case float64:
		if v == float64(int64(v)) {
			return int64(v)
		}
		return v
	case json.Number:
		if i, err := v.Int64(); err == nil {
			return i
		}
		if f, err := v.Float64(); err == nil {
			return f
		}
		return v.String()
	default:
		return v
	}
}

func captureRowsTx(ctx context.Context, tx *sql.Tx, table string, query string, args []any, manifest *AliasOrphanRollbackManifest, order *int, meta map[string]string) error {
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("capture %s rows: %w", table, err)
	}
	defer rows.Close()
	scanned, err := scanRows(rows)
	if err != nil {
		return fmt.Errorf("scan %s rows for capture: %w", table, err)
	}
	for _, row := range scanned {
		columns := make([]string, 0, len(row))
		// Stable column order for deterministic manifests.
		for col := range row {
			columns = append(columns, col)
		}
		sort.Strings(columns)
		values := make([]any, len(columns))
		for i, col := range columns {
			values[i] = row[col]
		}
		*order++
		entry := AliasOrphanDeletedRow{
			Table:   table,
			Columns: columns,
			Values:  values,
			Order:   *order,
		}
		if len(meta) > 0 {
			entry.Meta = meta
		}
		manifest.DeletedRows = append(manifest.DeletedRows, entry)
	}
	return nil
}

func rowValueString(row AliasOrphanDeletedRow, column string) string {
	for i, col := range row.Columns {
		if col == column {
			if i >= len(row.Values) || row.Values[i] == nil {
				return ""
			}
			switch v := row.Values[i].(type) {
			case string:
				return v
			case []byte:
				return string(v)
			default:
				return fmt.Sprint(v)
			}
		}
	}
	return ""
}

func listProjectsTx(ctx context.Context, tx *sql.Tx, databasePath string) ([]ProjectIdentity, error) {
	rows, err := tx.QueryContext(ctx, `
SELECT
  projects.id,
  COALESCE(NULLIF(projects.friendly_name, ''), projects.id),
  COALESCE(current_path.path, projects.current_path, ''),
  COALESCE(projects.last_seen_at, '')
FROM projects
LEFT JOIN project_paths AS current_path
  ON current_path.project_id = projects.id
 AND current_path.is_current = 1
ORDER BY lower(COALESCE(NULLIF(projects.friendly_name, ''), projects.id)), projects.id
`)
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	defer rows.Close()
	var projects []ProjectIdentity
	for rows.Next() {
		identity := ProjectIdentity{ContractVersion: StateJSONContractVersion, DatabaseScope: "global", DatabasePath: databasePath}
		if err := rows.Scan(&identity.ID, &identity.FriendlyName, &identity.CurrentPath, &identity.LastSeenAt); err != nil {
			return nil, fmt.Errorf("scan project identity: %w", err)
		}
		projects = append(projects, identity)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return projects, nil
}

func sqliteTableExistsQ(ctx context.Context, q aliasOrphanQuerier, table string) (bool, error) {
	var name string
	err := q.QueryRowContext(ctx, `SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&name)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return false, fmt.Errorf("inspect table %s: %w", table, err)
}

func writeAliasOrphanRollbackManifest(manifest AliasOrphanRollbackManifest, dir string, now time.Time) (string, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create alias-orphan rollback manifest directory: %w", err)
	}
	for i := 0; i < 100; i++ {
		suffix := ""
		if i > 0 {
			suffix = fmt.Sprintf("-%02d", i)
		}
		path := filepath.Join(dir, fmt.Sprintf("alias-orphan-rollback-%s%s.json", now.Format("20060102T150405Z"), suffix))
		if _, err := os.Stat(path); err == nil {
			continue
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("stat alias-orphan rollback manifest: %w", err)
		}
		payload, err := json.MarshalIndent(manifest, "", "  ")
		if err != nil {
			return "", fmt.Errorf("encode alias-orphan rollback manifest: %w", err)
		}
		payload = append(payload, '\n')
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			if errors.Is(err, os.ErrExist) {
				continue
			}
			return "", fmt.Errorf("create alias-orphan rollback manifest: %w", err)
		}
		if _, err := file.Write(payload); err != nil {
			_ = file.Close()
			_ = os.Remove(path)
			return "", fmt.Errorf("write alias-orphan rollback manifest: %w", err)
		}
		if err := file.Sync(); err != nil {
			_ = file.Close()
			_ = os.Remove(path)
			return "", fmt.Errorf("sync alias-orphan rollback manifest: %w", err)
		}
		if err := file.Close(); err != nil {
			_ = os.Remove(path)
			return "", fmt.Errorf("close alias-orphan rollback manifest: %w", err)
		}
		if err := syncDirectory(dir); err != nil {
			_ = os.Remove(path)
			return "", fmt.Errorf("sync alias-orphan rollback manifest directory: %w", err)
		}
		return path, nil
	}
	return "", fmt.Errorf("create alias-orphan rollback manifest: exhausted timestamp suffixes")
}

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	if err := directory.Sync(); err != nil {
		_ = directory.Close()
		return err
	}
	return directory.Close()
}

func readAliasOrphanRollbackManifest(path string) (AliasOrphanRollbackManifest, error) {
	payload, err := os.ReadFile(path)
	if err != nil {
		return AliasOrphanRollbackManifest{}, fmt.Errorf("read alias-orphan rollback manifest: %w", err)
	}
	var manifest AliasOrphanRollbackManifest
	dec := json.NewDecoder(strings.NewReader(string(payload)))
	dec.UseNumber()
	if err := dec.Decode(&manifest); err != nil {
		return AliasOrphanRollbackManifest{}, fmt.Errorf("decode alias-orphan rollback manifest: %w", err)
	}
	if manifest.Migration != aliasOrphanMigrationName {
		return AliasOrphanRollbackManifest{}, fmt.Errorf("rollback manifest migration %q is not %s", manifest.Migration, aliasOrphanMigrationName)
	}
	return manifest, nil
}
