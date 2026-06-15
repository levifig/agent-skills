package state

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/levifig/loaf/internal/project"
)

// ProjectIdentity is the durable database identity for one working project.
// ID is intentionally independent of the current path and friendly name.
type ProjectIdentity struct {
	ContractVersion int    `json:"contract_version"`
	DatabaseScope   string `json:"database_scope"`
	ID              string `json:"id"`
	FriendlyName    string `json:"friendly_name"`
	CurrentPath     string `json:"current_path"`
	LastSeenAt      string `json:"last_seen_at"`
	DatabasePath    string `json:"database_path,omitempty"`
}

// ProjectList is the global project identity index.
type ProjectList struct {
	ContractVersion int               `json:"contract_version"`
	DatabaseScope   string            `json:"database_scope"`
	DatabasePath    string            `json:"database_path"`
	Projects        []ProjectIdentity `json:"projects"`
}

// ProjectMoveResult describes a safe path remapping for a project.
type ProjectMoveResult struct {
	ContractVersion int             `json:"contract_version"`
	DatabaseScope   string          `json:"database_scope"`
	Project         ProjectIdentity `json:"project"`
	FromPath        string          `json:"from_path"`
	ToPath          string          `json:"to_path"`
	Action          string          `json:"action"`
}

// ProjectRenameResult describes a friendly-name change preview.
type ProjectRenameResult struct {
	ContractVersion int             `json:"contract_version"`
	DatabaseScope   string          `json:"database_scope"`
	Project         ProjectIdentity `json:"project"`
	FromName        string          `json:"from_name"`
	ToName          string          `json:"to_name"`
	Action          string          `json:"action"`
}

// ProjectIdentityForRoot returns the DB-backed project identity.
// Writable stores refresh current path metadata; read-only stores only look up
// an existing mapping.
func (s *Store) ProjectIdentityForRoot(ctx context.Context, root project.Root) (ProjectIdentity, error) {
	if s.readOnly {
		return s.LookupProjectIdentityForRoot(ctx, root)
	}
	return s.EnsureProject(ctx, root)
}

// LookupProjectIdentityForRoot reads project identity without refreshing paths.
func (s *Store) LookupProjectIdentityForRoot(ctx context.Context, root project.Root) (ProjectIdentity, error) {
	projectID, err := s.projectIDByPath(ctx, root.Path())
	if err != nil {
		return ProjectIdentity{}, err
	}
	if projectID == "" {
		return ProjectIdentity{}, sql.ErrNoRows
	}
	return s.projectIdentity(ctx, projectID)
}

// ListProjects returns all durable project identities without refreshing any path.
func (s *Store) ListProjects(ctx context.Context) (ProjectList, error) {
	rows, err := s.db.QueryContext(ctx, `
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
		return ProjectList{}, fmt.Errorf("list projects: %w", err)
	}
	defer rows.Close()

	list := ProjectList{
		ContractVersion: StateJSONContractVersion,
		DatabaseScope:   "global",
		DatabasePath:    s.path,
		Projects:        []ProjectIdentity{},
	}
	for rows.Next() {
		identity := ProjectIdentity{ContractVersion: StateJSONContractVersion, DatabaseScope: "global"}
		if err := rows.Scan(&identity.ID, &identity.FriendlyName, &identity.CurrentPath, &identity.LastSeenAt); err != nil {
			return ProjectList{}, fmt.Errorf("scan project identity: %w", err)
		}
		identity.DatabasePath = s.path
		list.Projects = append(list.Projects, identity)
	}
	if err := rows.Err(); err != nil {
		return ProjectList{}, fmt.Errorf("iterate project identities: %w", err)
	}
	return list, nil
}

// EnsureProject records the current path and returns the durable project identity.
func (s *Store) EnsureProject(ctx context.Context, root project.Root) (ProjectIdentity, error) {
	currentPath := root.Path()
	now := time.Now().UTC().Format(time.RFC3339)
	friendlyName := defaultProjectFriendlyName(currentPath)

	projectID, err := s.projectIDByPath(ctx, currentPath)
	if err != nil {
		return ProjectIdentity{}, err
	}
	if projectID == "" {
		legacyID := ProjectID(root)
		if exists, err := s.projectExists(ctx, legacyID); err != nil {
			return ProjectIdentity{}, err
		} else if exists {
			projectID, err = s.rekeyLegacyProject(ctx, legacyID, currentPath, friendlyName, now)
			if err != nil {
				return ProjectIdentity{}, err
			}
		}
	} else if projectID == ProjectID(root) {
		projectID, err = s.rekeyLegacyProject(ctx, projectID, currentPath, friendlyName, now)
		if err != nil {
			return ProjectIdentity{}, err
		}
	}
	if projectID == "" {
		projectID, err = newProjectID()
		if err != nil {
			return ProjectIdentity{}, err
		}
		if _, err := s.db.ExecContext(ctx, `
INSERT INTO projects (id, identity_hash, friendly_name, current_path, last_seen_at, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
`, projectID, projectID, friendlyName, currentPath, now, now, now); err != nil {
			return ProjectIdentity{}, fmt.Errorf("create project identity: %w", err)
		}
	} else {
		if _, err := s.db.ExecContext(ctx, `
UPDATE projects
SET friendly_name = COALESCE(NULLIF(friendly_name, ''), ?),
    current_path = ?,
    last_seen_at = ?,
    updated_at = ?
WHERE id = ?
`, friendlyName, currentPath, now, now, projectID); err != nil {
			return ProjectIdentity{}, fmt.Errorf("refresh project identity: %w", err)
		}
	}

	if err := s.markCurrentProjectPath(ctx, projectID, currentPath, now); err != nil {
		return ProjectIdentity{}, err
	}
	return s.projectIdentity(ctx, projectID)
}

// UpsertProject records the project identity row after migrations are applied.
func (s *Store) UpsertProject(ctx context.Context, root project.Root) error {
	if _, err := s.EnsureProject(ctx, root); err != nil {
		if isMissingProjectIdentitySchema(err) {
			return s.upsertLegacyProject(ctx, root)
		}
		return err
	}
	return nil
}

// RenameProject updates the human-friendly project name without changing identity.
func (s *Store) RenameProject(ctx context.Context, root project.Root, friendlyName string) (ProjectIdentity, error) {
	friendlyName = strings.TrimSpace(friendlyName)
	if friendlyName == "" {
		return ProjectIdentity{}, fmt.Errorf("project name cannot be empty")
	}
	identity, err := s.LookupProjectIdentityForRoot(ctx, root)
	if err != nil {
		return ProjectIdentity{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := s.db.ExecContext(ctx, `
UPDATE projects
SET friendly_name = ?, updated_at = ?
WHERE id = ?
`, friendlyName, now, identity.ID); err != nil {
		return ProjectIdentity{}, fmt.Errorf("rename project: %w", err)
	}
	return s.projectIdentity(ctx, identity.ID)
}

// PreviewRenameProject validates a friendly-name change without mutating project identity rows.
func (s *Store) PreviewRenameProject(ctx context.Context, root project.Root, friendlyName string) (ProjectRenameResult, error) {
	friendlyName = strings.TrimSpace(friendlyName)
	if friendlyName == "" {
		return ProjectRenameResult{}, fmt.Errorf("project name cannot be empty")
	}
	identity, err := s.LookupProjectIdentityForRoot(ctx, root)
	if err != nil {
		return ProjectRenameResult{}, err
	}
	fromName := identity.FriendlyName
	identity.FriendlyName = friendlyName
	return ProjectRenameResult{
		ContractVersion: StateJSONContractVersion,
		DatabaseScope:   identity.DatabaseScope,
		Project:         identity,
		FromName:        fromName,
		ToName:          friendlyName,
		Action:          "dry-run",
	}, nil
}

// MoveProject changes the current path mapping with safeguards against collisions.
func (s *Store) MoveProject(ctx context.Context, root project.Root, fromPath string, toPath string) (ProjectMoveResult, error) {
	return s.moveProject(ctx, root, fromPath, toPath, false)
}

// PreviewMoveProject validates a path remapping without mutating project identity rows.
func (s *Store) PreviewMoveProject(ctx context.Context, root project.Root, fromPath string, toPath string) (ProjectMoveResult, error) {
	return s.moveProject(ctx, root, fromPath, toPath, true)
}

func (s *Store) moveProject(ctx context.Context, root project.Root, fromPath string, toPath string, dryRun bool) (ProjectMoveResult, error) {
	fromPath = filepath.Clean(fromPath)
	toPath = filepath.Clean(toPath)
	if fromPath == "." || toPath == "." || fromPath == "" || toPath == "" {
		return ProjectMoveResult{}, fmt.Errorf("project move requires absolute --from and --to paths")
	}
	if !filepath.IsAbs(fromPath) || !filepath.IsAbs(toPath) {
		return ProjectMoveResult{}, fmt.Errorf("project move requires absolute --from and --to paths")
	}
	if fromPath == toPath {
		return ProjectMoveResult{}, fmt.Errorf("project move requires distinct --from and --to paths")
	}
	if info, err := os.Stat(toPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ProjectMoveResult{}, fmt.Errorf("project move target path does not exist: %s", toPath)
		}
		return ProjectMoveResult{}, fmt.Errorf("inspect project move target path %s: %w", toPath, err)
	} else if !info.IsDir() {
		return ProjectMoveResult{}, fmt.Errorf("project move target path is not a directory: %s", toPath)
	}

	projectID, err := s.projectIDByPath(ctx, fromPath)
	if err != nil {
		return ProjectMoveResult{}, err
	}
	if projectID == "" {
		if fromPath == root.Path() {
			current, err := s.LookupProjectIdentityForRoot(ctx, root)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return ProjectMoveResult{}, err
			}
			if err == nil && current.CurrentPath == fromPath {
				projectID = current.ID
			}
		}
		if projectID == "" {
			return ProjectMoveResult{}, fmt.Errorf("project path %s is not registered; run from the old checkout or initialize it first", fromPath)
		}
	}

	existingProjectID, err := s.projectIDByPath(ctx, toPath)
	if err != nil {
		return ProjectMoveResult{}, err
	}
	if existingProjectID != "" && existingProjectID != projectID {
		return ProjectMoveResult{}, fmt.Errorf("target path %s is already registered to project %s", toPath, existingProjectID)
	}

	if dryRun {
		identity, err := s.projectIdentity(ctx, projectID)
		if err != nil {
			return ProjectMoveResult{}, err
		}
		identity.CurrentPath = toPath
		return ProjectMoveResult{
			ContractVersion: StateJSONContractVersion,
			DatabaseScope:   identity.DatabaseScope,
			Project:         identity,
			FromPath:        fromPath,
			ToPath:          toPath,
			Action:          "dry-run",
		}, nil
	}

	now := time.Now().UTC().Format(time.RFC3339)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ProjectMoveResult{}, fmt.Errorf("begin project move: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `
UPDATE project_paths
SET is_current = 0, updated_at = ?
WHERE project_id = ?
`, now, projectID); err != nil {
		return ProjectMoveResult{}, fmt.Errorf("clear current project paths: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO project_paths (id, project_id, path, is_current, first_seen_at, last_seen_at, created_at, updated_at)
VALUES (?, ?, ?, 1, ?, ?, ?, ?)
ON CONFLICT(path) DO UPDATE SET
  project_id = excluded.project_id,
  is_current = excluded.is_current,
  last_seen_at = excluded.last_seen_at,
  updated_at = excluded.updated_at
`, stableMigrationID("project-path", projectID, toPath), projectID, toPath, now, now, now, now); err != nil {
		return ProjectMoveResult{}, fmt.Errorf("record target project path: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE projects
SET current_path = ?, last_seen_at = ?, updated_at = ?
WHERE id = ?
`, toPath, now, now, projectID); err != nil {
		return ProjectMoveResult{}, fmt.Errorf("update project current path: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return ProjectMoveResult{}, fmt.Errorf("commit project move: %w", err)
	}
	identity, err := s.projectIdentity(ctx, projectID)
	if err != nil {
		return ProjectMoveResult{}, err
	}
	return ProjectMoveResult{
		ContractVersion: StateJSONContractVersion,
		DatabaseScope:   identity.DatabaseScope,
		Project:         identity,
		FromPath:        fromPath,
		ToPath:          toPath,
		Action:          "moved",
	}, nil
}

func (s *Store) projectID(ctx context.Context, root project.Root) (string, error) {
	identity, err := s.ProjectIdentityForRoot(ctx, root)
	if err != nil {
		return "", err
	}
	return identity.ID, nil
}

func (s *Store) projectIdentity(ctx context.Context, projectID string) (ProjectIdentity, error) {
	identity := ProjectIdentity{ContractVersion: StateJSONContractVersion, DatabaseScope: "global"}
	var friendlyName sql.NullString
	var currentPath sql.NullString
	var lastSeenAt sql.NullString
	err := s.db.QueryRowContext(ctx, `
SELECT id, friendly_name, current_path, last_seen_at
FROM projects
WHERE id = ?
`, projectID).Scan(&identity.ID, &friendlyName, &currentPath, &lastSeenAt)
	if err != nil {
		return ProjectIdentity{}, fmt.Errorf("read project identity: %w", err)
	}
	identity.FriendlyName = friendlyName.String
	identity.CurrentPath = currentPath.String
	identity.LastSeenAt = lastSeenAt.String
	identity.DatabasePath = s.path
	return identity, nil
}

func (s *Store) projectIDByPath(ctx context.Context, path string) (string, error) {
	var projectID string
	err := s.db.QueryRowContext(ctx, `SELECT project_id FROM project_paths WHERE path = ?`, path).Scan(&projectID)
	switch {
	case err == nil:
		return projectID, nil
	case errors.Is(err, sql.ErrNoRows):
		return "", nil
	default:
		return "", fmt.Errorf("read project path mapping: %w", err)
	}
}

func (s *Store) projectExists(ctx context.Context, projectID string) (bool, error) {
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM projects WHERE id = ?`, projectID).Scan(&count); err != nil {
		return false, fmt.Errorf("read project row: %w", err)
	}
	return count > 0, nil
}

func (s *Store) markCurrentProjectPath(ctx context.Context, projectID string, path string, now string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin project path refresh: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `
UPDATE project_paths
SET is_current = 0, updated_at = ?
WHERE project_id = ? AND path <> ?
`, now, projectID, path); err != nil {
		return fmt.Errorf("clear stale current paths: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO project_paths (id, project_id, path, is_current, first_seen_at, last_seen_at, created_at, updated_at)
VALUES (?, ?, ?, 1, ?, ?, ?, ?)
ON CONFLICT(path) DO UPDATE SET
  is_current = 1,
  last_seen_at = excluded.last_seen_at,
  updated_at = excluded.updated_at
`, stableMigrationID("project-path", projectID, path), projectID, path, now, now, now, now); err != nil {
		return fmt.Errorf("upsert project path: %w", err)
	}
	return tx.Commit()
}

func (s *Store) rekeyLegacyProject(ctx context.Context, legacyID string, currentPath string, friendlyName string, now string) (string, error) {
	nextID, err := newProjectID()
	if err != nil {
		return "", err
	}
	var createdAt string
	var storedFriendly sql.NullString
	if err := s.db.QueryRowContext(ctx, `SELECT created_at, friendly_name FROM projects WHERE id = ?`, legacyID).Scan(&createdAt, &storedFriendly); err != nil {
		return "", fmt.Errorf("read legacy project before rekey: %w", err)
	}
	if storedFriendly.Valid && strings.TrimSpace(storedFriendly.String) != "" {
		friendlyName = storedFriendly.String
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("begin project rekey: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `
INSERT INTO projects (id, identity_hash, friendly_name, current_path, last_seen_at, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
`, nextID, nextID, friendlyName, currentPath, now, createdAt, now); err != nil {
		return "", fmt.Errorf("insert rekeyed project: %w", err)
	}
	for _, table := range projectScopedRekeyTables() {
		if _, err := tx.ExecContext(ctx, fmt.Sprintf(`
UPDATE %s
SET project_id = ?
WHERE project_id = ?
`, quoteSQLiteIdentifier(table)), nextID, legacyID); err != nil {
			return "", fmt.Errorf("rekey %s project rows: %w", table, err)
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM projects WHERE id = ?`, legacyID); err != nil {
		return "", fmt.Errorf("delete legacy project row: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("commit project rekey: %w", err)
	}
	return nextID, nil
}

func projectScopedRekeyTables() []string {
	tables := append([]string{"project_paths"}, projectScopedMergeTables...)
	return tables
}

func newProjectID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generate project id: %w", err)
	}
	return "proj_" + hex.EncodeToString(raw[:]), nil
}

func defaultProjectFriendlyName(path string) string {
	name := strings.TrimSpace(filepath.Base(path))
	if name == "" || name == "." || name == string(filepath.Separator) {
		return "project"
	}
	return name
}

func (s *Store) upsertLegacyProject(ctx context.Context, root project.Root) error {
	now := time.Now().UTC().Format(time.RFC3339)
	projectID := ProjectID(root)
	_, err := s.db.ExecContext(ctx, `
INSERT INTO projects (id, identity_hash, created_at, updated_at)
VALUES (?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
  identity_hash = excluded.identity_hash,
  updated_at = excluded.updated_at
`, projectID, projectID, now, now)
	if err != nil {
		return fmt.Errorf("upsert legacy project: %w", err)
	}
	return nil
}

func isMissingProjectIdentitySchema(err error) bool {
	message := err.Error()
	return strings.Contains(message, "no such table: project_paths") ||
		strings.Contains(message, "no such column: friendly_name") ||
		strings.Contains(message, "no such column: current_path") ||
		strings.Contains(message, "no such column: last_seen_at")
}
