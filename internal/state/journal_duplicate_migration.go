package state

import (
	"context"
	"database/sql"
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

// Journal-duplicates repair migration.
//
// Schema references to journal_entries.id (verified against internal/state/migrations/*.sql):
//
//   - journal_search.journal_entry_id  (0006_journal_search.sql; rebuilt in 0010_journal_first.sql)
//     FTS5 derived index; not a foreign key. rowid mirrors journal_entries.rowid at insert time.
//   - journal_origins.journal_entry_id (0011_journal_origins_and_deferrals.sql) — PRIMARY KEY, deliberately not an FK
//   - journal_deferrals.journal_entry_id (0011) — NOT NULL UNIQUE, deliberately not an FK
//   - intent_operations.journal_entry_id (0012_intents_and_explorations.sql) — optional projection ref;
//     CHECK ties projection_version=1 to non-NULL journal_entry_id + spark_id
//   - journal_conversation_handles.journal_entry_id (0012) — NOT NULL, UNIQUE (journal_entry_id, handle_id), not an FK
//
// FTS strategy: targeted DELETE FROM journal_search WHERE journal_entry_id = ? inside the same
// apply transaction as the journal_entries deletion. A full RepairJournalSearch rebuild would work
// but is heavier and spans a second ceremony; targeted deletes keep apply/rollback symmetric —
// rollback restores the journal_entries row then rebuilds its FTS row via insertJournalSearchTx
// (the live write path), avoiding FTS rowid drift after re-insert.
//
// Window constants (june13OriginalImportWindow*, june24ReimportWindow*) and inTimestampWindow
// are shared with the alias-orphans migration — do not redefine them here.

const (
	JournalDuplicateMigrationActionDryRun   = "dry-run"
	JournalDuplicateMigrationActionApply    = "apply"
	JournalDuplicateMigrationActionRollback = "rollback"

	journalDuplicateMigrationName = "journal-duplicates"

	journalDuplicateProofPair     = "pair"
	journalDuplicateProofUnproven = "unproven"

	journalDuplicateDispositionRetire = "retire"
)

// JournalDuplicateMigrationResult is the preview/apply/rollback outcome for the
// journal-duplicates repair migration. Classification spans every project.
type JournalDuplicateMigrationResult struct {
	ContractVersion      int                              `json:"contract_version"`
	DatabaseScope        string                           `json:"database_scope"`
	DatabasePath         string                           `json:"database_path"`
	ProjectID            string                           `json:"project_id,omitempty"`
	ProjectName          string                           `json:"project_name,omitempty"`
	ProjectCurrentPath   string                           `json:"project_current_path,omitempty"`
	Action               string                           `json:"action"`
	Applied              bool                             `json:"applied"`
	CopyRun              bool                             `json:"copy_run"`
	BackupPath           string                           `json:"backup_path,omitempty"`
	RollbackManifestPath string                           `json:"rollback_manifest_path,omitempty"`
	Projects             []JournalDuplicateProjectSummary `json:"projects"`
	Totals               JournalDuplicateCounts           `json:"totals"`
	Dispositions         []JournalDuplicateDisposition    `json:"dispositions,omitempty"`
	OperatorFlags        []string                         `json:"operator_flags,omitempty"`
	Warnings             []string                         `json:"warnings,omitempty"`
	RowsRestored         int                              `json:"rows_restored,omitempty"`
}

// JournalDuplicateProjectSummary reports classification for one project.
type JournalDuplicateProjectSummary struct {
	ProjectID          string                        `json:"project_id"`
	ProjectName        string                        `json:"project_name,omitempty"`
	ProjectCurrentPath string                        `json:"project_current_path,omitempty"`
	Counts             JournalDuplicateCounts        `json:"counts"`
	Classifications    []JournalDuplicateRowClassify `json:"classifications,omitempty"`
	Dispositions       []JournalDuplicateDisposition `json:"dispositions,omitempty"`
}

// JournalDuplicateCounts aggregates window and action counts.
type JournalDuplicateCounts struct {
	June13Rows        int `json:"june13_rows"`
	June24Rows        int `json:"june24_rows"`
	Pairs             int `json:"pairs"`
	Retire            int `json:"retire"`
	Unproven          int `json:"unproven"`
	NamedDispositions int `json:"named_dispositions,omitempty"`
	OperatorRetire    int `json:"operator_retire,omitempty"`
	EntriesRetired    int `json:"entries_retired,omitempty"`
}

// JournalDuplicateRowClassify is one journal entry's classification under the
// two-window natural-key pairing rules.
type JournalDuplicateRowClassify struct {
	ProjectID   string `json:"project_id"`
	EntryID     string `json:"entry_id"`
	EntryType   string `json:"entry_type"`
	Scope       string `json:"scope,omitempty"`
	Message     string `json:"message,omitempty"`
	CreatedAt   string `json:"created_at,omitempty"`
	Window      string `json:"window,omitempty"` // "june13" or "june24"
	Proof       string `json:"proof"`
	TwinID      string `json:"twin_id,omitempty"`
	Disposition string `json:"disposition,omitempty"`
}

// JournalDuplicateDisposition is a planned action against a specific journal row.
type JournalDuplicateDisposition struct {
	ProjectID string `json:"project_id"`
	EntryID   string `json:"entry_id"`
	Action    string `json:"action"`
	Proof     string `json:"proof,omitempty"`
	TwinID    string `json:"twin_id,omitempty"`
	Flag      string `json:"flag,omitempty"`
	Note      string `json:"note,omitempty"`
}

// JournalDuplicateApplyOptions carries explicit per-row operator dispositions.
// --realias is not supported for this migration (journal rows carry no aliases).
type JournalDuplicateApplyOptions struct {
	Retire []string // journal entry IDs to force-retire when classified unproven
	Flags  []string // verbatim flag strings for the manifest
}

// JournalDuplicateRollbackManifest preserves every deleted/changed row for rollback.
type JournalDuplicateRollbackManifest struct {
	ContractVersion      int                           `json:"contract_version"`
	Migration            string                        `json:"migration"`
	CreatedAt            string                        `json:"created_at"`
	DatabaseScope        string                        `json:"database_scope"`
	DatabasePath         string                        `json:"database_path"`
	OperatorFlags        []string                      `json:"operator_flags,omitempty"`
	OperatorDispositions []JournalDuplicateDisposition `json:"operator_dispositions,omitempty"`
	Retirements          []JournalDuplicateDisposition `json:"retirements,omitempty"`
	DeletedRows          []JournalDuplicateDeletedRow  `json:"deleted_rows"`
	Unlinks              []JournalDuplicateUnlink      `json:"unlinks,omitempty"`
	Counts               JournalDuplicateCounts        `json:"counts"`
	Metadata             map[string]string             `json:"metadata,omitempty"`
}

// JournalDuplicateDeletedRow is one full row snapshot for rollback restore.
type JournalDuplicateDeletedRow struct {
	Table   string            `json:"table"`
	Columns []string          `json:"columns"`
	Values  []any             `json:"values"`
	Order   int               `json:"order"`
	Meta    map[string]string `json:"meta,omitempty"`
}

// JournalDuplicateUnlink records a soft-reference rewrite for rollback.
// NewID is empty when the row was deleted instead of repointed.
type JournalDuplicateUnlink struct {
	Table      string `json:"table"`
	ProjectID  string `json:"project_id"`
	Column     string `json:"column"`
	KeyColumn  string `json:"key_column,omitempty"`
	RowID      string `json:"row_id"`
	PreviousID string `json:"previous_id"`
	NewID      string `json:"new_id,omitempty"`
}

type journalDuplicateWindowRow struct {
	id        string
	projectID string
	entryType string
	scope     string
	message   string
	createdAt string
	window    string
}

// PreviewJournalDuplicateMigration classifies and simulates journal-duplicate repair on a copy.
func PreviewJournalDuplicateMigration(ctx context.Context, root project.Root, resolver PathResolver, options JournalDuplicateApplyOptions) (JournalDuplicateMigrationResult, error) {
	status, err := requireJournalDuplicateMigrationStatus(root, resolver)
	if err != nil {
		return JournalDuplicateMigrationResult{}, err
	}
	source, err := OpenStoreReadOnly(status.DatabasePath)
	if err != nil {
		return JournalDuplicateMigrationResult{}, err
	}
	defer source.Close()

	tempDir, err := os.MkdirTemp("", "loaf-journal-duplicate-migration-*")
	if err != nil {
		return JournalDuplicateMigrationResult{}, fmt.Errorf("create journal-duplicate migration temp dir: %w", err)
	}
	defer os.RemoveAll(tempDir)
	copyPath := filepath.Join(tempDir, "state.sqlite")
	if err := copySQLiteDatabase(ctx, source, copyPath, 0o600); err != nil {
		return JournalDuplicateMigrationResult{}, err
	}
	copyStore, err := OpenStore(copyPath)
	if err != nil {
		return JournalDuplicateMigrationResult{}, err
	}
	defer copyStore.Close()

	result, manifest, err := planJournalDuplicateMigration(ctx, copyStore, journalDuplicateMigrationBaseResult(status, JournalDuplicateMigrationActionDryRun), options, journalDuplicateExecutedDispositions{})
	if err != nil {
		return JournalDuplicateMigrationResult{}, err
	}
	result.OperatorFlags = append([]string{}, options.Flags...)
	if err := applyJournalDuplicateMigrationManifest(ctx, copyStore, &manifest, nil); err != nil {
		return JournalDuplicateMigrationResult{}, fmt.Errorf("simulate journal-duplicate migration: %w", err)
	}
	result.CopyRun = true
	return result, nil
}

// ApplyJournalDuplicateMigration backs up, writes a rollback manifest, and retires June-13 journal twins.
func ApplyJournalDuplicateMigration(ctx context.Context, root project.Root, resolver PathResolver, options JournalDuplicateApplyOptions) (JournalDuplicateMigrationResult, error) {
	status, err := requireJournalDuplicateMigrationStatus(root, resolver)
	if err != nil {
		return JournalDuplicateMigrationResult{}, err
	}
	backup, err := Backup(ctx, root, resolver)
	if err != nil {
		return JournalDuplicateMigrationResult{}, err
	}
	store, err := openInitializedStore(root, resolver)
	if err != nil {
		return JournalDuplicateMigrationResult{}, err
	}
	defer store.Close()

	executed, err := readJournalDuplicateExecutedDispositions(filepath.Dir(backup.BackupPath))
	if err != nil {
		return JournalDuplicateMigrationResult{}, err
	}
	result, manifest, err := planJournalDuplicateMigration(ctx, store, journalDuplicateMigrationBaseResult(status, JournalDuplicateMigrationActionApply), options, executed)
	if err != nil {
		return JournalDuplicateMigrationResult{}, err
	}
	result.BackupPath = backup.BackupPath
	result.OperatorFlags = append([]string{}, options.Flags...)
	if len(result.Warnings) > 0 {
		return result, fmt.Errorf("journal-duplicate dispositions matched no rows: %s", strings.Join(result.Warnings, "; "))
	}
	result.Applied = true

	manifestPath := ""
	if err := applyJournalDuplicateMigrationManifest(ctx, store, &manifest, func(final JournalDuplicateRollbackManifest) error {
		if !journalDuplicateManifestHasWork(final) {
			return nil
		}
		path, err := writeJournalDuplicateRollbackManifest(final, filepath.Dir(backup.BackupPath), time.Now().UTC())
		if err != nil {
			return err
		}
		manifestPath = path
		return nil
	}); err != nil {
		if manifestPath != "" {
			os.Remove(manifestPath)
		}
		result.Applied = false
		return result, err
	}
	result.RollbackManifestPath = manifestPath
	result.Totals.EntriesRetired = manifest.Counts.EntriesRetired

	verify, _, err := planJournalDuplicateMigration(ctx, store, journalDuplicateMigrationBaseResult(status, JournalDuplicateMigrationActionApply), JournalDuplicateApplyOptions{}, executed)
	if err != nil {
		return result, fmt.Errorf("post-apply verification: %w (backup %s, rollback manifest %s)", err, result.BackupPath, result.RollbackManifestPath)
	}
	if verify.Totals.Retire > 0 {
		return result, fmt.Errorf("post-apply verification failed: %d retire-class journal duplicates remain (backup %s, rollback manifest %s)", verify.Totals.Retire, result.BackupPath, result.RollbackManifestPath)
	}
	return result, nil
}

// RollbackJournalDuplicateMigration restores rows recorded in a journal-duplicate rollback manifest.
func RollbackJournalDuplicateMigration(ctx context.Context, root project.Root, resolver PathResolver, manifestPath string) (JournalDuplicateMigrationResult, error) {
	if manifestPath == "" {
		return JournalDuplicateMigrationResult{}, fmt.Errorf("journal-duplicate rollback requires a manifest path")
	}
	status, err := requireJournalDuplicateMigrationStatus(root, resolver)
	if err != nil {
		return JournalDuplicateMigrationResult{}, err
	}
	manifest, err := readJournalDuplicateRollbackManifest(manifestPath)
	if err != nil {
		return JournalDuplicateMigrationResult{}, err
	}
	backup, err := Backup(ctx, root, resolver)
	if err != nil {
		return JournalDuplicateMigrationResult{}, err
	}
	store, err := openInitializedStore(root, resolver)
	if err != nil {
		return JournalDuplicateMigrationResult{}, err
	}
	defer store.Close()

	result := journalDuplicateMigrationBaseResult(status, JournalDuplicateMigrationActionRollback)
	result.Applied = true
	result.BackupPath = backup.BackupPath
	result.RollbackManifestPath = manifestPath
	result.OperatorFlags = append([]string{}, manifest.OperatorFlags...)
	if err := rollbackJournalDuplicateMigrationManifest(ctx, store, manifest, &result); err != nil {
		return JournalDuplicateMigrationResult{}, err
	}
	return result, nil
}

func journalDuplicateMigrationBaseResult(status Status, action string) JournalDuplicateMigrationResult {
	return JournalDuplicateMigrationResult{
		ContractVersion:    StateJSONContractVersion,
		DatabaseScope:      "global",
		DatabasePath:       status.DatabasePath,
		ProjectID:          status.ProjectID,
		ProjectName:        status.ProjectName,
		ProjectCurrentPath: status.ProjectCurrentPath,
		Action:             action,
		Projects:           []JournalDuplicateProjectSummary{},
	}
}

func requireJournalDuplicateMigrationStatus(root project.Root, resolver PathResolver) (Status, error) {
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

func planJournalDuplicateMigration(ctx context.Context, store *Store, result JournalDuplicateMigrationResult, options JournalDuplicateApplyOptions, executed journalDuplicateExecutedDispositions) (JournalDuplicateMigrationResult, JournalDuplicateRollbackManifest, error) {
	manifest := JournalDuplicateRollbackManifest{
		ContractVersion: StateJSONContractVersion,
		Migration:       journalDuplicateMigrationName,
		CreatedAt:       time.Now().UTC().Format(time.RFC3339Nano),
		DatabaseScope:   result.DatabaseScope,
		DatabasePath:    result.DatabasePath,
		OperatorFlags:   append([]string{}, options.Flags...),
		DeletedRows:     []JournalDuplicateDeletedRow{},
		Metadata: map[string]string{
			"june13_window_start": june13OriginalImportWindowStart,
			"june13_window_end":   june13OriginalImportWindowEnd,
			"june24_window_start": june24ReimportWindowStart,
			"june24_window_end":   june24ReimportWindowEnd,
		},
	}

	retireSet := journalDuplicateOperatorRetireSet(options)
	for _, id := range sortedKeys(retireSet) {
		manifest.OperatorDispositions = append(manifest.OperatorDispositions, JournalDuplicateDisposition{
			EntryID: id,
			Action:  journalDuplicateDispositionRetire,
			Flag:    "--retire " + id,
		})
	}

	projects, err := store.ListProjects(ctx)
	if err != nil {
		return result, manifest, err
	}

	matched := map[string]struct{}{}
	for _, project := range projects.Projects {
		summary, err := classifyJournalDuplicatesForProject(ctx, store.db, project, retireSet)
		if err != nil {
			return result, manifest, err
		}
		result.Projects = append(result.Projects, summary)
		result.Totals.June13Rows += summary.Counts.June13Rows
		result.Totals.June24Rows += summary.Counts.June24Rows
		result.Totals.Pairs += summary.Counts.Pairs
		result.Totals.Retire += summary.Counts.Retire
		result.Totals.Unproven += summary.Counts.Unproven
		result.Totals.NamedDispositions += summary.Counts.NamedDispositions
		result.Totals.OperatorRetire += summary.Counts.OperatorRetire
		result.Dispositions = append(result.Dispositions, summary.Dispositions...)
		for _, c := range summary.Classifications {
			matched[c.EntryID] = struct{}{}
		}
	}
	warnings, err := journalDuplicateUnmatchedDispositionWarnings(ctx, store.db, retireSet, matched, executed)
	if err != nil {
		return result, manifest, err
	}
	result.Warnings = append(result.Warnings, warnings...)

	manifest.Counts = JournalDuplicateCounts{
		June13Rows:        result.Totals.June13Rows,
		June24Rows:        result.Totals.June24Rows,
		Pairs:             result.Totals.Pairs,
		Retire:            result.Totals.Retire,
		Unproven:          result.Totals.Unproven,
		NamedDispositions: result.Totals.NamedDispositions,
		OperatorRetire:    result.Totals.OperatorRetire,
	}
	return result, manifest, nil
}

func journalDuplicateOperatorRetireSet(options JournalDuplicateApplyOptions) map[string]struct{} {
	retireSet := map[string]struct{}{}
	for _, id := range options.Retire {
		if id = strings.TrimSpace(id); id != "" {
			retireSet[id] = struct{}{}
		}
	}
	return retireSet
}

func journalDuplicateUnmatchedDispositionWarnings(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, retireSet map[string]struct{}, matched map[string]struct{}, executed journalDuplicateExecutedDispositions) ([]string, error) {
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
		warnings = append(warnings, fmt.Sprintf("--retire %s matched no journal-duplicate row", id))
	}
	return warnings, nil
}

type journalDuplicateExecutedDispositions struct {
	retired map[string]struct{}
}

func readJournalDuplicateExecutedDispositions(dir string) (journalDuplicateExecutedDispositions, error) {
	executed := journalDuplicateExecutedDispositions{retired: map[string]struct{}{}}
	if dir == "" {
		return executed, nil
	}
	paths, err := filepath.Glob(filepath.Join(dir, "journal-duplicate-rollback-*.json"))
	if err != nil {
		return executed, fmt.Errorf("list journal-duplicate rollback manifests: %w", err)
	}
	for _, path := range paths {
		payload, err := os.ReadFile(path)
		if err != nil {
			return executed, fmt.Errorf("read journal-duplicate rollback manifest %s: %w", path, err)
		}
		var manifest struct {
			Retirements []JournalDuplicateDisposition `json:"retirements"`
		}
		if err := json.Unmarshal(payload, &manifest); err != nil {
			continue
		}
		for _, retirement := range manifest.Retirements {
			executed.retired[retirement.EntryID] = struct{}{}
		}
	}
	return executed, nil
}

func (e journalDuplicateExecutedDispositions) retirementCarriedOut(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, entryID string) (bool, error) {
	if _, ok := e.retired[entryID]; !ok {
		return false, nil
	}
	var found int
	if err := q.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM journal_entries WHERE id = ?)`, entryID).Scan(&found); err != nil {
		return false, fmt.Errorf("look for journal entry %s: %w", entryID, err)
	}
	return found == 0, nil
}

func classifyJournalDuplicatesForProject(ctx context.Context, q interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}, project ProjectIdentity, retireSet map[string]struct{}) (JournalDuplicateProjectSummary, error) {
	summary := JournalDuplicateProjectSummary{
		ProjectID:          project.ID,
		ProjectName:        project.FriendlyName,
		ProjectCurrentPath: project.CurrentPath,
		Classifications:    []JournalDuplicateRowClassify{},
		Dispositions:       []JournalDuplicateDisposition{},
	}

	rows, err := q.QueryContext(ctx, `
SELECT id, project_id, entry_type, COALESCE(scope, ''), message, created_at
FROM journal_entries
WHERE project_id = ?
ORDER BY created_at, id
`, project.ID)
	if err != nil {
		return summary, fmt.Errorf("list journal entries for project %s: %w", project.ID, err)
	}
	defer rows.Close()

	june13ByKey := map[string][]journalDuplicateWindowRow{}
	june24ByKey := map[string][]journalDuplicateWindowRow{}
	for rows.Next() {
		var row journalDuplicateWindowRow
		if err := rows.Scan(&row.id, &row.projectID, &row.entryType, &row.scope, &row.message, &row.createdAt); err != nil {
			return summary, fmt.Errorf("scan journal entry: %w", err)
		}
		switch {
		case inTimestampWindow(row.createdAt, june13OriginalImportWindowStart, june13OriginalImportWindowEnd):
			row.window = "june13"
			summary.Counts.June13Rows++
			key := journalDuplicateNaturalKey(row.entryType, row.scope, row.message)
			june13ByKey[key] = append(june13ByKey[key], row)
		case inTimestampWindow(row.createdAt, june24ReimportWindowStart, june24ReimportWindowEnd):
			row.window = "june24"
			summary.Counts.June24Rows++
			key := journalDuplicateNaturalKey(row.entryType, row.scope, row.message)
			june24ByKey[key] = append(june24ByKey[key], row)
		}
	}
	if err := rows.Err(); err != nil {
		return summary, fmt.Errorf("list journal entries for project %s: %w", project.ID, err)
	}

	keys := map[string]struct{}{}
	for key := range june13ByKey {
		if _, ok := june24ByKey[key]; ok {
			keys[key] = struct{}{}
		}
	}
	for key := range keys {
		left := june13ByKey[key]
		right := june24ByKey[key]
		if len(left) == 1 && len(right) == 1 {
			summary.Counts.Pairs++
			retire := left[0]
			twin := right[0]
			classify := JournalDuplicateRowClassify{
				ProjectID:   project.ID,
				EntryID:     retire.id,
				EntryType:   retire.entryType,
				Scope:       retire.scope,
				Message:     retire.message,
				CreatedAt:   retire.createdAt,
				Window:      retire.window,
				Proof:       journalDuplicateProofPair,
				TwinID:      twin.id,
				Disposition: journalDuplicateDispositionRetire,
			}
			summary.Classifications = append(summary.Classifications, classify)
			summary.Counts.Retire++
			disp := JournalDuplicateDisposition{
				ProjectID: project.ID,
				EntryID:   retire.id,
				Action:    journalDuplicateDispositionRetire,
				Proof:     journalDuplicateProofPair,
				TwinID:    twin.id,
			}
			summary.Dispositions = append(summary.Dispositions, disp)
			continue
		}
		// Multi-candidate on either side: refuse by default; --retire may force specific rows.
		ambiguous := append(append([]journalDuplicateWindowRow{}, left...), right...)
		sort.Slice(ambiguous, func(i, j int) bool {
			if ambiguous[i].createdAt != ambiguous[j].createdAt {
				return ambiguous[i].createdAt < ambiguous[j].createdAt
			}
			return ambiguous[i].id < ambiguous[j].id
		})
		for _, row := range ambiguous {
			classify := JournalDuplicateRowClassify{
				ProjectID: project.ID,
				EntryID:   row.id,
				EntryType: row.entryType,
				Scope:     row.scope,
				Message:   row.message,
				CreatedAt: row.createdAt,
				Window:    row.window,
				Proof:     journalDuplicateProofUnproven,
			}
			summary.Counts.Unproven++
			if _, force := retireSet[row.id]; force {
				classify.Disposition = journalDuplicateDispositionRetire
				summary.Counts.Retire++
				summary.Counts.OperatorRetire++
				summary.Counts.NamedDispositions++
				summary.Dispositions = append(summary.Dispositions, JournalDuplicateDisposition{
					ProjectID: project.ID,
					EntryID:   row.id,
					Action:    journalDuplicateDispositionRetire,
					Proof:     journalDuplicateProofUnproven,
					Flag:      "--retire " + row.id,
					Note:      "operator force-retire of unproven multi-candidate match",
				})
			}
			summary.Classifications = append(summary.Classifications, classify)
		}
	}

	sort.Slice(summary.Classifications, func(i, j int) bool {
		return summary.Classifications[i].EntryID < summary.Classifications[j].EntryID
	})
	sort.Slice(summary.Dispositions, func(i, j int) bool {
		return summary.Dispositions[i].EntryID < summary.Dispositions[j].EntryID
	})
	return summary, nil
}

func journalDuplicateNaturalKey(entryType, scope, message string) string {
	return entryType + "\x00" + scope + "\x00" + message
}

func journalDuplicateManifestHasWork(manifest JournalDuplicateRollbackManifest) bool {
	return len(manifest.DeletedRows) > 0 || len(manifest.Unlinks) > 0 || len(manifest.Retirements) > 0
}

func applyJournalDuplicateMigrationManifest(ctx context.Context, store *Store, manifest *JournalDuplicateRollbackManifest, beforeCommit func(JournalDuplicateRollbackManifest) error) error {
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin journal-duplicate migration: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `PRAGMA defer_foreign_keys = ON`); err != nil {
		return fmt.Errorf("defer foreign keys: %w", err)
	}

	order := 0
	projects, err := listProjectsTx(ctx, tx, store.path)
	if err != nil {
		return err
	}

	retireSet := map[string]struct{}{}
	for _, d := range manifest.OperatorDispositions {
		if d.Action == journalDuplicateDispositionRetire {
			retireSet[d.EntryID] = struct{}{}
		}
	}

	// Fixed-point: retiring one multi-candidate row can leave a clean 1:1 pair
	// that classification would auto-retire on a subsequent pass. Keep going
	// until a pass plans no further retirements (operator --retire is only
	// needed on the first pass; later passes use empty dispositions).
	for pass := 0; pass < 32; pass++ {
		passRetire := retireSet
		if pass > 0 {
			passRetire = map[string]struct{}{}
		}
		retiredThisPass := 0
		for _, project := range projects {
			summary, err := classifyJournalDuplicatesForProject(ctx, tx, project, passRetire)
			if err != nil {
				return err
			}
			for _, c := range summary.Classifications {
				if c.Disposition != journalDuplicateDispositionRetire {
					continue
				}
				if err := retireJournalDuplicateTx(ctx, tx, project.ID, c, manifest, &order); err != nil {
					return err
				}
				manifest.Retirements = append(manifest.Retirements, JournalDuplicateDisposition{
					ProjectID: project.ID,
					EntryID:   c.EntryID,
					Action:    journalDuplicateDispositionRetire,
					Proof:     c.Proof,
					TwinID:    c.TwinID,
				})
				manifest.Counts.EntriesRetired++
				retiredThisPass++
			}
		}
		if retiredThisPass == 0 {
			break
		}
	}

	if beforeCommit != nil {
		if err := beforeCommit(*manifest); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit journal-duplicate migration: %w", err)
	}
	return nil
}

func retireJournalDuplicateTx(ctx context.Context, tx *sql.Tx, projectID string, classify JournalDuplicateRowClassify, manifest *JournalDuplicateRollbackManifest, order *int) error {
	entryID := classify.EntryID
	twinID := classify.TwinID

	// FTS: targeted delete keeps the index consistent without a full rebuild.
	if err := captureAndDeleteJournalDuplicateTx(ctx, tx, "journal_search", `WHERE journal_entry_id = ?`, []any{entryID}, manifest, order); err != nil {
		return err
	}
	if err := captureAndDeleteJournalDuplicateTx(ctx, tx, "journal_origins", `WHERE journal_entry_id = ?`, []any{entryID}, manifest, order); err != nil {
		return err
	}

	// Soft refs that cannot be NULLed (NOT NULL columns / CHECK constraints).
	// Repoint to the surviving June-24 twin when possible; otherwise delete.
	if err := repointOrDeleteJournalSoftRefTx(ctx, tx, projectID, journalDuplicateSoftRef{
		table: "journal_deferrals", column: "journal_entry_id", keyColumn: "operation_key", uniqueColumn: true,
	}, entryID, twinID, manifest, order); err != nil {
		return err
	}
	if err := repointOrDeleteJournalSoftRefTx(ctx, tx, projectID, journalDuplicateSoftRef{
		table: "intent_operations", column: "journal_entry_id", keyColumn: "operation_key", uniqueColumn: false,
	}, entryID, twinID, manifest, order); err != nil {
		return err
	}
	if err := repointOrDeleteJournalConversationHandlesTx(ctx, tx, projectID, entryID, twinID, manifest, order); err != nil {
		return err
	}

	if err := captureAndDeleteJournalDuplicateTx(ctx, tx, "journal_entries", `WHERE project_id = ? AND id = ?`, []any{projectID, entryID}, manifest, order); err != nil {
		return err
	}
	return nil
}

type journalDuplicateSoftRef struct {
	table        string
	column       string
	keyColumn    string
	uniqueColumn bool
}

func repointOrDeleteJournalSoftRefTx(ctx context.Context, tx *sql.Tx, projectID string, ref journalDuplicateSoftRef, entryID, twinID string, manifest *JournalDuplicateRollbackManifest, order *int) error {
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

	rows, err := tx.QueryContext(ctx, fmt.Sprintf(`SELECT %s FROM %s WHERE project_id = ? AND %s = ? ORDER BY %s`, quotedKey, quotedTable, quotedColumn, quotedKey), projectID, entryID)
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
		if repoint && ref.uniqueColumn {
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
			manifest.Unlinks = append(manifest.Unlinks, JournalDuplicateUnlink{
				Table:      ref.table,
				ProjectID:  projectID,
				Column:     ref.column,
				KeyColumn:  ref.keyColumn,
				RowID:      key,
				PreviousID: entryID,
				NewID:      twinID,
			})
			continue
		}
		if err := captureAndDeleteJournalDuplicateTx(ctx, tx, ref.table, fmt.Sprintf(`WHERE project_id = ? AND %s = ?`, quotedKey), []any{projectID, key}, manifest, order); err != nil {
			return err
		}
	}
	return nil
}

func repointOrDeleteJournalConversationHandlesTx(ctx context.Context, tx *sql.Tx, projectID, entryID, twinID string, manifest *JournalDuplicateRollbackManifest, order *int) error {
	exists, err := sqliteTableExistsQ(ctx, tx, "journal_conversation_handles")
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}
	rows, err := tx.QueryContext(ctx, `
SELECT id, handle_id FROM journal_conversation_handles
WHERE project_id = ? AND journal_entry_id = ?
ORDER BY id
`, projectID, entryID)
	if err != nil {
		return fmt.Errorf("list journal_conversation_handles references: %w", err)
	}
	type handleRef struct {
		id       string
		handleID string
	}
	var refs []handleRef
	for rows.Next() {
		var ref handleRef
		if err := rows.Scan(&ref.id, &ref.handleID); err != nil {
			rows.Close()
			return fmt.Errorf("scan journal_conversation_handles reference: %w", err)
		}
		refs = append(refs, ref)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("list journal_conversation_handles references: %w", err)
	}
	rows.Close()

	for _, ref := range refs {
		repoint := twinID != ""
		if repoint {
			var taken int
			if err := tx.QueryRowContext(ctx, `
SELECT EXISTS(SELECT 1 FROM journal_conversation_handles WHERE journal_entry_id = ? AND handle_id = ?)
`, twinID, ref.handleID).Scan(&taken); err != nil {
				return fmt.Errorf("check journal_conversation_handles availability: %w", err)
			}
			repoint = taken == 0
		}
		if repoint {
			if _, err := tx.ExecContext(ctx, `
UPDATE journal_conversation_handles SET journal_entry_id = ? WHERE project_id = ? AND id = ?
`, twinID, projectID, ref.id); err != nil {
				return fmt.Errorf("repoint journal_conversation_handles: %w", err)
			}
			manifest.Unlinks = append(manifest.Unlinks, JournalDuplicateUnlink{
				Table:      "journal_conversation_handles",
				ProjectID:  projectID,
				Column:     "journal_entry_id",
				KeyColumn:  "id",
				RowID:      ref.id,
				PreviousID: entryID,
				NewID:      twinID,
			})
			continue
		}
		if err := captureAndDeleteJournalDuplicateTx(ctx, tx, "journal_conversation_handles", `WHERE project_id = ? AND id = ?`, []any{projectID, ref.id}, manifest, order); err != nil {
			return err
		}
	}
	return nil
}

func captureAndDeleteJournalDuplicateTx(ctx context.Context, tx *sql.Tx, table string, where string, args []any, manifest *JournalDuplicateRollbackManifest, order *int) error {
	exists, err := sqliteTableExistsQ(ctx, tx, table)
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}
	quoted := quoteSQLiteIdentifier(table)
	if err := captureJournalDuplicateRowsTx(ctx, tx, table, fmt.Sprintf(`SELECT * FROM %s %s`, quoted, where), args, manifest, order, nil); err != nil {
		return err
	}
	if _, err := execCountTx(ctx, tx, fmt.Sprintf(`DELETE FROM %s %s`, quoted, where), args...); err != nil {
		return fmt.Errorf("delete %s rows: %w", table, err)
	}
	return nil
}

func captureJournalDuplicateRowsTx(ctx context.Context, tx *sql.Tx, table string, query string, args []any, manifest *JournalDuplicateRollbackManifest, order *int, meta map[string]string) error {
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
		for col := range row {
			columns = append(columns, col)
		}
		sort.Strings(columns)
		values := make([]any, len(columns))
		for i, col := range columns {
			values[i] = row[col]
		}
		*order++
		entry := JournalDuplicateDeletedRow{
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

func rollbackJournalDuplicateMigrationManifest(ctx context.Context, store *Store, manifest JournalDuplicateRollbackManifest, result *JournalDuplicateMigrationResult) error {
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin journal-duplicate rollback: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `PRAGMA defer_foreign_keys = ON`); err != nil {
		return fmt.Errorf("defer foreign keys: %w", err)
	}

	// Restore deleted rows in reverse capture order.
	rows := append([]JournalDuplicateDeletedRow{}, manifest.DeletedRows...)
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].Order > rows[j].Order })
	for _, row := range rows {
		if row.Table == "journal_search" {
			// FTS is rebuilt from restored journal_entries below so rowids stay aligned
			// with the live insert path.
			continue
		}
		if err := insertJournalDuplicateDeletedRowTx(ctx, tx, row); err != nil {
			return err
		}
		result.RowsRestored++
		if row.Table == "journal_entries" {
			if err := restoreJournalSearchForDeletedRowTx(ctx, tx, row); err != nil {
				return err
			}
		}
	}

	// Undo repoints after row restores so PreviousID targets exist.
	for _, unlink := range manifest.Unlinks {
		keyColumn := firstNonEmpty(unlink.KeyColumn, "id")
		if _, err := tx.ExecContext(ctx, fmt.Sprintf(
			`UPDATE %s SET %s = ? WHERE project_id = ? AND %s = ?`,
			quoteSQLiteIdentifier(unlink.Table),
			quoteSQLiteIdentifier(unlink.Column),
			quoteSQLiteIdentifier(keyColumn),
		), unlink.PreviousID, unlink.ProjectID, unlink.RowID); err != nil {
			return fmt.Errorf("restore unlink %s.%s: %w", unlink.Table, unlink.Column, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit journal-duplicate rollback: %w", err)
	}
	return nil
}

func insertJournalDuplicateDeletedRowTx(ctx context.Context, tx *sql.Tx, row JournalDuplicateDeletedRow) error {
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

func restoreJournalSearchForDeletedRowTx(ctx context.Context, tx *sql.Tx, row JournalDuplicateDeletedRow) error {
	projectID := journalDuplicateRowValueString(row, "project_id")
	entryID := journalDuplicateRowValueString(row, "id")
	entryType := journalDuplicateRowValueString(row, "entry_type")
	scope := journalDuplicateRowValueString(row, "scope")
	message := journalDuplicateRowValueString(row, "message")
	harness := journalDuplicateRowValueString(row, "harness_session_id")
	if projectID == "" || entryID == "" {
		return nil
	}
	// Drop any stale FTS row then re-insert via the live write path.
	if _, err := tx.ExecContext(ctx, `DELETE FROM journal_search WHERE journal_entry_id = ?`, entryID); err != nil {
		return fmt.Errorf("clear journal_search before restore for %s: %w", entryID, err)
	}
	return insertJournalSearchTx(ctx, tx, projectID, entryID, harness, entryType, scope, message)
}

func journalDuplicateRowValueString(row JournalDuplicateDeletedRow, column string) string {
	for i, col := range row.Columns {
		if col != column {
			continue
		}
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
	return ""
}

func writeJournalDuplicateRollbackManifest(manifest JournalDuplicateRollbackManifest, dir string, now time.Time) (string, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create journal-duplicate rollback manifest directory: %w", err)
	}
	for i := 0; i < 100; i++ {
		suffix := ""
		if i > 0 {
			suffix = fmt.Sprintf("-%02d", i)
		}
		path := filepath.Join(dir, fmt.Sprintf("journal-duplicate-rollback-%s%s.json", now.Format("20060102T150405Z"), suffix))
		if _, err := os.Stat(path); err == nil {
			continue
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("stat journal-duplicate rollback manifest: %w", err)
		}
		payload, err := json.MarshalIndent(manifest, "", "  ")
		if err != nil {
			return "", fmt.Errorf("encode journal-duplicate rollback manifest: %w", err)
		}
		payload = append(payload, '\n')
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			if errors.Is(err, os.ErrExist) {
				continue
			}
			return "", fmt.Errorf("create journal-duplicate rollback manifest: %w", err)
		}
		if _, err := file.Write(payload); err != nil {
			_ = file.Close()
			_ = os.Remove(path)
			return "", fmt.Errorf("write journal-duplicate rollback manifest: %w", err)
		}
		if err := file.Sync(); err != nil {
			_ = file.Close()
			_ = os.Remove(path)
			return "", fmt.Errorf("sync journal-duplicate rollback manifest: %w", err)
		}
		if err := file.Close(); err != nil {
			_ = os.Remove(path)
			return "", fmt.Errorf("close journal-duplicate rollback manifest: %w", err)
		}
		if err := syncDirectory(dir); err != nil {
			_ = os.Remove(path)
			return "", fmt.Errorf("sync journal-duplicate rollback manifest directory: %w", err)
		}
		return path, nil
	}
	return "", fmt.Errorf("create journal-duplicate rollback manifest: exhausted timestamp suffixes")
}

func readJournalDuplicateRollbackManifest(path string) (JournalDuplicateRollbackManifest, error) {
	payload, err := os.ReadFile(path)
	if err != nil {
		return JournalDuplicateRollbackManifest{}, fmt.Errorf("read journal-duplicate rollback manifest: %w", err)
	}
	var manifest JournalDuplicateRollbackManifest
	if err := json.Unmarshal(payload, &manifest); err != nil {
		return JournalDuplicateRollbackManifest{}, fmt.Errorf("decode journal-duplicate rollback manifest: %w", err)
	}
	if manifest.Migration != "" && manifest.Migration != journalDuplicateMigrationName {
		return JournalDuplicateRollbackManifest{}, fmt.Errorf("rollback manifest migration %q is not %s", manifest.Migration, journalDuplicateMigrationName)
	}
	return manifest, nil
}
