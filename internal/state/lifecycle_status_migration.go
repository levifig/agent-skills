package state

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/levifig/loaf/internal/project"
)

const (
	LifecycleStatusMigrationActionDryRun   = "dry-run"
	LifecycleStatusMigrationActionApply    = "apply"
	LifecycleStatusMigrationActionRollback = "rollback"

	lifecycleStatusMigrationEventType = "status_normalized"
	lifecycleStatusMigrationNote      = "SPEC-049 lifecycle status normalization"
)

type LifecycleStatusMigrationResult struct {
	ContractVersion          int      `json:"contract_version"`
	DatabaseScope            string   `json:"database_scope"`
	DatabasePath             string   `json:"database_path"`
	ProjectID                string   `json:"project_id"`
	ProjectName              string   `json:"project_name,omitempty"`
	ProjectCurrentPath       string   `json:"project_current_path,omitempty"`
	Action                   string   `json:"action"`
	Applied                  bool     `json:"applied"`
	CopyRun                  bool     `json:"copy_run"`
	BackupPath               string   `json:"backup_path,omitempty"`
	RollbackManifestPath     string   `json:"rollback_manifest_path,omitempty"`
	EntitiesScanned          int      `json:"entities_scanned"`
	EntitiesRewritten        int      `json:"entities_rewritten"`
	EventsScanned            int      `json:"events_scanned"`
	EventsRewritten          int      `json:"events_rewritten"`
	NormalizationEvents      int      `json:"normalization_events"`
	LegacyStatusesRemaining  int      `json:"legacy_statuses_remaining"`
	RollbackEntitiesRestored int      `json:"rollback_entities_restored,omitempty"`
	RollbackEventsRestored   int      `json:"rollback_events_restored,omitempty"`
	Warnings                 []string `json:"warnings,omitempty"`
}

type LifecycleStatusRollbackManifest struct {
	ContractVersion      int                                   `json:"contract_version"`
	Migration            string                                `json:"migration"`
	CreatedAt            string                                `json:"created_at"`
	DatabaseScope        string                                `json:"database_scope"`
	DatabasePath         string                                `json:"database_path"`
	ProjectID            string                                `json:"project_id"`
	ProjectName          string                                `json:"project_name,omitempty"`
	ProjectCurrentPath   string                                `json:"project_current_path,omitempty"`
	EntityChanges        []LifecycleStatusEntityRollback       `json:"entity_changes"`
	EventChanges         []LifecycleStatusEventRollback        `json:"event_changes"`
	NormalizationEventID []string                              `json:"normalization_event_ids"`
	Counts               LifecycleStatusRollbackManifestCounts `json:"counts"`
	Metadata             map[string]string                     `json:"metadata,omitempty"`
}

type LifecycleStatusEntityRollback struct {
	Table                string `json:"table"`
	Kind                 string `json:"kind"`
	ID                   string `json:"id"`
	PreviousStatus       string `json:"previous_status"`
	CanonicalStatus      string `json:"canonical_status"`
	NormalizationEventID string `json:"normalization_event_id"`
}

type LifecycleStatusEventRollback struct {
	ID                string `json:"id"`
	Kind              string `json:"kind"`
	EntityID          string `json:"entity_id"`
	PreviousToStatus  string `json:"previous_to_status"`
	CanonicalToStatus string `json:"canonical_to_status"`
}

type LifecycleStatusRollbackManifestCounts struct {
	EntitiesRewritten   int `json:"entities_rewritten"`
	EventsRewritten     int `json:"events_rewritten"`
	NormalizationEvents int `json:"normalization_events"`
}

type lifecycleStatusTable struct {
	kind  string
	table string
}

var lifecycleStatusTables = []lifecycleStatusTable{
	{kind: LifecycleEntitySpec, table: "specs"},
	{kind: LifecycleEntityTask, table: "tasks"},
	{kind: LifecycleEntityIdea, table: "ideas"},
	{kind: LifecycleEntitySpark, table: "sparks"},
	{kind: LifecycleEntityBrainstorm, table: "brainstorms"},
	{kind: LifecycleEntityReport, table: "reports"},
	{kind: LifecycleEntityPlan, table: "plans"},
	{kind: LifecycleEntityHandoff, table: "handoffs"},
	{kind: LifecycleEntityCouncil, table: "councils"},
}

// PreviewLifecycleStatusMigration runs lifecycle status normalization against a temporary copy.
func PreviewLifecycleStatusMigration(ctx context.Context, root project.Root, resolver PathResolver) (LifecycleStatusMigrationResult, error) {
	status, err := requireLifecycleStatusMigrationStatus(root, resolver)
	if err != nil {
		return LifecycleStatusMigrationResult{}, err
	}
	source, err := OpenStoreReadOnly(status.DatabasePath)
	if err != nil {
		return LifecycleStatusMigrationResult{}, err
	}
	defer source.Close()

	tempDir, err := os.MkdirTemp("", "loaf-lifecycle-status-migration-*")
	if err != nil {
		return LifecycleStatusMigrationResult{}, fmt.Errorf("create lifecycle status migration temp dir: %w", err)
	}
	defer os.RemoveAll(tempDir)
	copyPath := filepath.Join(tempDir, "state.sqlite")
	if err := copySQLiteDatabase(ctx, source, copyPath, 0o600); err != nil {
		return LifecycleStatusMigrationResult{}, err
	}
	copyStore, err := OpenStore(copyPath)
	if err != nil {
		return LifecycleStatusMigrationResult{}, err
	}
	defer copyStore.Close()

	result, manifest, err := planLifecycleStatusMigration(ctx, copyStore, status.ProjectID, lifecycleStatusMigrationBaseResult(status, LifecycleStatusMigrationActionDryRun))
	if err != nil {
		return LifecycleStatusMigrationResult{}, err
	}
	if err := applyLifecycleStatusMigrationManifest(ctx, copyStore, status.ProjectID, manifest); err != nil {
		return LifecycleStatusMigrationResult{}, err
	}
	remaining, err := countLegacyLifecycleStatuses(ctx, copyStore, status.ProjectID)
	if err != nil {
		return LifecycleStatusMigrationResult{}, err
	}
	result.CopyRun = true
	result.LegacyStatusesRemaining = remaining
	return result, nil
}

// ApplyLifecycleStatusMigration normalizes legacy lifecycle statuses in the live database.
func ApplyLifecycleStatusMigration(ctx context.Context, root project.Root, resolver PathResolver) (LifecycleStatusMigrationResult, error) {
	status, err := requireLifecycleStatusMigrationStatus(root, resolver)
	if err != nil {
		return LifecycleStatusMigrationResult{}, err
	}
	backup, err := Backup(ctx, root, resolver)
	if err != nil {
		return LifecycleStatusMigrationResult{}, err
	}
	store, err := openInitializedStore(root, resolver)
	if err != nil {
		return LifecycleStatusMigrationResult{}, err
	}
	defer store.Close()

	result, manifest, err := planLifecycleStatusMigration(ctx, store, status.ProjectID, lifecycleStatusMigrationBaseResult(status, LifecycleStatusMigrationActionApply))
	if err != nil {
		return LifecycleStatusMigrationResult{}, err
	}
	result.BackupPath = backup.BackupPath
	result.Applied = true
	if len(manifest.EntityChanges) > 0 || len(manifest.EventChanges) > 0 || len(manifest.NormalizationEventID) > 0 {
		manifestPath, err := writeLifecycleStatusRollbackManifest(manifest, filepath.Dir(backup.BackupPath), time.Now().UTC())
		if err != nil {
			return LifecycleStatusMigrationResult{}, err
		}
		result.RollbackManifestPath = manifestPath
	}
	if err := applyLifecycleStatusMigrationManifest(ctx, store, status.ProjectID, manifest); err != nil {
		return LifecycleStatusMigrationResult{}, err
	}
	remaining, err := countLegacyLifecycleStatuses(ctx, store, status.ProjectID)
	if err != nil {
		return LifecycleStatusMigrationResult{}, err
	}
	result.LegacyStatusesRemaining = remaining
	return result, nil
}

// RollbackLifecycleStatusMigration restores statuses recorded in a lifecycle status rollback manifest.
func RollbackLifecycleStatusMigration(ctx context.Context, root project.Root, resolver PathResolver, manifestPath string) (LifecycleStatusMigrationResult, error) {
	if manifestPath == "" {
		return LifecycleStatusMigrationResult{}, fmt.Errorf("lifecycle status rollback requires a manifest path")
	}
	status, err := requireLifecycleStatusMigrationStatus(root, resolver)
	if err != nil {
		return LifecycleStatusMigrationResult{}, err
	}
	manifest, err := readLifecycleStatusRollbackManifest(manifestPath)
	if err != nil {
		return LifecycleStatusMigrationResult{}, err
	}
	if manifest.ProjectID != status.ProjectID {
		return LifecycleStatusMigrationResult{}, fmt.Errorf("rollback manifest project %s does not match current project %s", manifest.ProjectID, status.ProjectID)
	}
	backup, err := Backup(ctx, root, resolver)
	if err != nil {
		return LifecycleStatusMigrationResult{}, err
	}
	store, err := openInitializedStore(root, resolver)
	if err != nil {
		return LifecycleStatusMigrationResult{}, err
	}
	defer store.Close()

	result := lifecycleStatusMigrationBaseResult(status, LifecycleStatusMigrationActionRollback)
	result.Applied = true
	result.BackupPath = backup.BackupPath
	result.RollbackManifestPath = manifestPath
	if err := rollbackLifecycleStatusMigrationManifest(ctx, store, manifest, &result); err != nil {
		return LifecycleStatusMigrationResult{}, err
	}
	remaining, err := countLegacyLifecycleStatuses(ctx, store, status.ProjectID)
	if err != nil {
		return LifecycleStatusMigrationResult{}, err
	}
	result.LegacyStatusesRemaining = remaining
	return result, nil
}

func lifecycleStatusMigrationBaseResult(status Status, action string) LifecycleStatusMigrationResult {
	return LifecycleStatusMigrationResult{
		ContractVersion:    StateJSONContractVersion,
		DatabaseScope:      "global",
		DatabasePath:       status.DatabasePath,
		ProjectID:          status.ProjectID,
		ProjectName:        status.ProjectName,
		ProjectCurrentPath: status.ProjectCurrentPath,
		Action:             action,
	}
}

func requireLifecycleStatusMigrationStatus(root project.Root, resolver PathResolver) (Status, error) {
	status, err := Inspect(root, resolver)
	if err != nil {
		return Status{}, err
	}
	switch status.Mode {
	case ModeSQLiteReady:
		if status.ProjectID == "" {
			return Status{}, fmt.Errorf("SQLite state project identity is missing; run `loaf state status`")
		}
		return status, nil
	case ModeMarkdownOnly:
		return Status{}, fmt.Errorf("SQLite state database is not initialized; run `loaf state migrate markdown --apply` first")
	case ModeInvalid:
		return Status{}, fmt.Errorf("state database is invalid; run `loaf state doctor`")
	default:
		return Status{}, fmt.Errorf("state database is not ready; run `loaf state status`")
	}
}

func planLifecycleStatusMigration(ctx context.Context, store *Store, projectID string, result LifecycleStatusMigrationResult) (LifecycleStatusMigrationResult, LifecycleStatusRollbackManifest, error) {
	manifest := LifecycleStatusRollbackManifest{
		ContractVersion:    StateJSONContractVersion,
		Migration:          "lifecycle-statuses",
		CreatedAt:          time.Now().UTC().Format(time.RFC3339Nano),
		DatabaseScope:      result.DatabaseScope,
		DatabasePath:       result.DatabasePath,
		ProjectID:          result.ProjectID,
		ProjectName:        result.ProjectName,
		ProjectCurrentPath: result.ProjectCurrentPath,
		Metadata: map[string]string{
			"event_type": lifecycleStatusMigrationEventType,
			"note":       lifecycleStatusMigrationNote,
		},
	}
	for _, table := range lifecycleStatusTables {
		exists, err := sqliteTableExists(ctx, store.db, table.table)
		if err != nil {
			return result, manifest, err
		}
		if !exists {
			result.Warnings = append(result.Warnings, fmt.Sprintf("table %s is missing; skipped %s statuses", table.table, table.kind))
			continue
		}
		rows, err := store.db.QueryContext(ctx, fmt.Sprintf(`SELECT id, status FROM %s WHERE project_id = ? ORDER BY id`, quoteSQLiteIdentifier(table.table)), projectID)
		if err != nil {
			return result, manifest, fmt.Errorf("scan %s lifecycle statuses: %w", table.table, err)
		}
		for rows.Next() {
			var id string
			var stored string
			if err := rows.Scan(&id, &stored); err != nil {
				rows.Close()
				return result, manifest, fmt.Errorf("scan %s lifecycle status row: %w", table.table, err)
			}
			result.EntitiesScanned++
			canonical, ok := CanonicalLifecycleStatus(table.kind, stored)
			if !ok {
				result.Warnings = append(result.Warnings, fmt.Sprintf("%s %s has unsupported status %q", table.kind, id, stored))
				continue
			}
			if canonical == stored {
				continue
			}
			eventID := stableMigrationID("event", projectID, table.kind, id, lifecycleStatusMigrationEventType, stored, canonical)
			manifest.EntityChanges = append(manifest.EntityChanges, LifecycleStatusEntityRollback{
				Table:                table.table,
				Kind:                 table.kind,
				ID:                   id,
				PreviousStatus:       stored,
				CanonicalStatus:      canonical,
				NormalizationEventID: eventID,
			})
			manifest.NormalizationEventID = append(manifest.NormalizationEventID, eventID)
			result.EntitiesRewritten++
			result.NormalizationEvents++
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return result, manifest, fmt.Errorf("scan %s lifecycle statuses: %w", table.table, err)
		}
		rows.Close()
	}

	rows, err := store.db.QueryContext(ctx, `
SELECT id, entity_kind, entity_id, to_status
FROM events
WHERE project_id = ? AND to_status IS NOT NULL
ORDER BY id
`, projectID)
	if err != nil {
		return result, manifest, fmt.Errorf("scan lifecycle event statuses: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var kind string
		var entityID string
		var toStatus string
		if err := rows.Scan(&id, &kind, &entityID, &toStatus); err != nil {
			return result, manifest, fmt.Errorf("scan lifecycle event status row: %w", err)
		}
		if !isLifecycleStatusEntityKind(kind) {
			continue
		}
		result.EventsScanned++
		canonical, ok := CanonicalLifecycleStatus(kind, toStatus)
		if !ok {
			result.Warnings = append(result.Warnings, fmt.Sprintf("event %s for %s %s has unsupported to_status %q", id, kind, entityID, toStatus))
			continue
		}
		if canonical == toStatus {
			continue
		}
		manifest.EventChanges = append(manifest.EventChanges, LifecycleStatusEventRollback{
			ID:                id,
			Kind:              kind,
			EntityID:          entityID,
			PreviousToStatus:  toStatus,
			CanonicalToStatus: canonical,
		})
		result.EventsRewritten++
	}
	if err := rows.Err(); err != nil {
		return result, manifest, fmt.Errorf("scan lifecycle event statuses: %w", err)
	}
	manifest.Counts = LifecycleStatusRollbackManifestCounts{
		EntitiesRewritten:   result.EntitiesRewritten,
		EventsRewritten:     result.EventsRewritten,
		NormalizationEvents: result.NormalizationEvents,
	}
	return result, manifest, nil
}

func applyLifecycleStatusMigrationManifest(ctx context.Context, store *Store, projectID string, manifest LifecycleStatusRollbackManifest) error {
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin lifecycle status migration: %w", err)
	}
	defer tx.Rollback()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, change := range manifest.EntityChanges {
		if _, err := tx.ExecContext(ctx, fmt.Sprintf(`UPDATE %s SET status = ?, updated_at = ? WHERE project_id = ? AND id = ?`, quoteSQLiteIdentifier(change.Table)), change.CanonicalStatus, now, projectID, change.ID); err != nil {
			return fmt.Errorf("normalize %s %s status: %w", change.Kind, change.ID, err)
		}
		if _, err := tx.ExecContext(ctx, `
INSERT OR IGNORE INTO events (id, project_id, entity_kind, entity_id, event_type, from_status, to_status, note, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`, change.NormalizationEventID, projectID, change.Kind, change.ID, lifecycleStatusMigrationEventType, change.PreviousStatus, change.CanonicalStatus, lifecycleStatusMigrationNote, now, now); err != nil {
			return fmt.Errorf("record %s %s lifecycle normalization event: %w", change.Kind, change.ID, err)
		}
	}
	for _, change := range manifest.EventChanges {
		if _, err := tx.ExecContext(ctx, `UPDATE events SET to_status = ?, updated_at = ? WHERE project_id = ? AND id = ?`, change.CanonicalToStatus, now, projectID, change.ID); err != nil {
			return fmt.Errorf("normalize event %s to_status: %w", change.ID, err)
		}
	}
	return tx.Commit()
}

func rollbackLifecycleStatusMigrationManifest(ctx context.Context, store *Store, manifest LifecycleStatusRollbackManifest, result *LifecycleStatusMigrationResult) error {
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin lifecycle status rollback: %w", err)
	}
	defer tx.Rollback()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, change := range manifest.EntityChanges {
		if !isLifecycleStatusEntityKind(change.Kind) {
			return fmt.Errorf("rollback manifest contains non-lifecycle kind %q", change.Kind)
		}
		if _, err := tx.ExecContext(ctx, fmt.Sprintf(`UPDATE %s SET status = ?, updated_at = ? WHERE project_id = ? AND id = ?`, quoteSQLiteIdentifier(change.Table)), change.PreviousStatus, now, manifest.ProjectID, change.ID); err != nil {
			return fmt.Errorf("restore %s %s status: %w", change.Kind, change.ID, err)
		}
		result.RollbackEntitiesRestored++
	}
	for _, change := range manifest.EventChanges {
		if _, err := tx.ExecContext(ctx, `UPDATE events SET to_status = ?, updated_at = ? WHERE project_id = ? AND id = ?`, change.PreviousToStatus, now, manifest.ProjectID, change.ID); err != nil {
			return fmt.Errorf("restore event %s to_status: %w", change.ID, err)
		}
		result.RollbackEventsRestored++
	}
	for _, eventID := range manifest.NormalizationEventID {
		if _, err := tx.ExecContext(ctx, `DELETE FROM events WHERE project_id = ? AND id = ? AND event_type = ?`, manifest.ProjectID, eventID, lifecycleStatusMigrationEventType); err != nil {
			return fmt.Errorf("remove normalization event %s: %w", eventID, err)
		}
	}
	result.EntitiesRewritten = len(manifest.EntityChanges)
	result.EventsRewritten = len(manifest.EventChanges)
	result.NormalizationEvents = len(manifest.NormalizationEventID)
	return tx.Commit()
}

func countLegacyLifecycleStatuses(ctx context.Context, store *Store, projectID string) (int, error) {
	total := 0
	for _, table := range lifecycleStatusTables {
		rows, err := store.db.QueryContext(ctx, fmt.Sprintf(`SELECT status FROM %s WHERE project_id = ?`, quoteSQLiteIdentifier(table.table)), projectID)
		if err != nil {
			return 0, fmt.Errorf("count legacy %s statuses: %w", table.table, err)
		}
		for rows.Next() {
			var stored string
			if err := rows.Scan(&stored); err != nil {
				rows.Close()
				return 0, fmt.Errorf("scan legacy %s status: %w", table.table, err)
			}
			canonical, ok := CanonicalLifecycleStatus(table.kind, stored)
			if ok && canonical != stored {
				total++
			}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return 0, fmt.Errorf("count legacy %s statuses: %w", table.table, err)
		}
		rows.Close()
	}
	rows, err := store.db.QueryContext(ctx, `
SELECT entity_kind, to_status
FROM events
WHERE project_id = ? AND to_status IS NOT NULL
`, projectID)
	if err != nil {
		return 0, fmt.Errorf("count legacy lifecycle event statuses: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var kind string
		var toStatus string
		if err := rows.Scan(&kind, &toStatus); err != nil {
			return 0, fmt.Errorf("scan legacy lifecycle event status: %w", err)
		}
		if !isLifecycleStatusEntityKind(kind) {
			continue
		}
		canonical, ok := CanonicalLifecycleStatus(kind, toStatus)
		if ok && canonical != toStatus {
			total++
		}
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("count legacy lifecycle event statuses: %w", err)
	}
	return total, nil
}

func isLifecycleStatusEntityKind(kind string) bool {
	for _, table := range lifecycleStatusTables {
		if table.kind == kind {
			return true
		}
	}
	return false
}

func sqliteTableExists(ctx context.Context, db *sql.DB, table string) (bool, error) {
	var name string
	err := db.QueryRowContext(ctx, `SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&name)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return false, fmt.Errorf("inspect table %s: %w", table, err)
}

func writeLifecycleStatusRollbackManifest(manifest LifecycleStatusRollbackManifest, dir string, now time.Time) (string, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create lifecycle status rollback manifest directory: %w", err)
	}
	for i := 0; i < 100; i++ {
		suffix := ""
		if i > 0 {
			suffix = fmt.Sprintf("-%02d", i)
		}
		path := filepath.Join(dir, fmt.Sprintf("lifecycle-status-rollback-%s%s.json", now.Format("20060102T150405Z"), suffix))
		if _, err := os.Stat(path); err == nil {
			continue
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("stat lifecycle status rollback manifest: %w", err)
		}
		payload, err := json.MarshalIndent(manifest, "", "  ")
		if err != nil {
			return "", fmt.Errorf("encode lifecycle status rollback manifest: %w", err)
		}
		payload = append(payload, '\n')
		if err := os.WriteFile(path, payload, 0o600); err != nil {
			return "", fmt.Errorf("write lifecycle status rollback manifest: %w", err)
		}
		return path, nil
	}
	return "", fmt.Errorf("create lifecycle status rollback manifest: exhausted timestamp suffixes")
}

func readLifecycleStatusRollbackManifest(path string) (LifecycleStatusRollbackManifest, error) {
	payload, err := os.ReadFile(path)
	if err != nil {
		return LifecycleStatusRollbackManifest{}, fmt.Errorf("read lifecycle status rollback manifest: %w", err)
	}
	var manifest LifecycleStatusRollbackManifest
	if err := json.Unmarshal(payload, &manifest); err != nil {
		return LifecycleStatusRollbackManifest{}, fmt.Errorf("decode lifecycle status rollback manifest: %w", err)
	}
	if manifest.Migration != "lifecycle-statuses" {
		return LifecycleStatusRollbackManifest{}, fmt.Errorf("rollback manifest migration %q is not lifecycle-statuses", manifest.Migration)
	}
	if manifest.ProjectID == "" {
		return LifecycleStatusRollbackManifest{}, fmt.Errorf("rollback manifest is missing project_id")
	}
	return manifest, nil
}
