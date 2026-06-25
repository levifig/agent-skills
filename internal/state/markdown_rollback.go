package state

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/levifig/loaf/internal/project"
)

// MarkdownRollbackManifest records the repository-external files needed to
// restore .agents Markdown after a destructive cutover step.
type MarkdownRollbackManifest struct {
	ContractVersion  int                         `json:"contract_version"`
	CreatedAt        string                      `json:"created_at"`
	ProjectPath      string                      `json:"project_path"`
	AgentsPath       string                      `json:"agents_path"`
	StateBackupPath  string                      `json:"state_backup_path"`
	AgentsBackupPath string                      `json:"agents_backup_path"`
	Files            []MarkdownRollbackFileEntry `json:"files"`
}

// MarkdownRollbackFileEntry describes one backed-up .agents file.
type MarkdownRollbackFileEntry struct {
	Path       string `json:"path"`
	BackupPath string `json:"backup_path"`
	Bytes      int64  `json:"bytes"`
	SHA256     string `json:"sha256"`
	Mode       uint32 `json:"mode"`
}

// MarkdownRollbackBackupResult describes a created rollback manifest.
type MarkdownRollbackBackupResult struct {
	ContractVersion      int    `json:"contract_version"`
	StateBackupPath      string `json:"state_backup_path"`
	RollbackManifestPath string `json:"rollback_manifest_path"`
	AgentsBackupPath     string `json:"agents_backup_path"`
	FileCount            int    `json:"file_count"`
	CreatedAt            string `json:"created_at"`
}

// MarkdownRollbackResult describes restoration from a rollback manifest.
type MarkdownRollbackResult struct {
	ContractVersion      int      `json:"contract_version"`
	Action               string   `json:"action"`
	ProjectPath          string   `json:"project_path"`
	RollbackManifestPath string   `json:"rollback_manifest_path"`
	StateBackupPath      string   `json:"state_backup_path"`
	RestoredFiles        []string `json:"restored_files"`
	Restored             bool     `json:"restored"`
}

const MarkdownMigrationActionRollback = "rollback"

// CreateMarkdownRollbackBackup snapshots .agents into the state backup
// directory and writes a manifest usable by RollbackMarkdownMigration.
func CreateMarkdownRollbackBackup(ctx context.Context, root project.Root, stateBackupPath string) (MarkdownRollbackBackupResult, error) {
	select {
	case <-ctx.Done():
		return MarkdownRollbackBackupResult{}, ctx.Err()
	default:
	}

	agentsPath := filepath.Join(root.Path(), ".agents")
	info, err := os.Stat(agentsPath)
	if os.IsNotExist(err) {
		return MarkdownRollbackBackupResult{}, fmt.Errorf(".agents directory not found")
	}
	if err != nil {
		return MarkdownRollbackBackupResult{}, fmt.Errorf("inspect .agents for rollback backup: %w", err)
	}
	if !info.IsDir() {
		return MarkdownRollbackBackupResult{}, fmt.Errorf(".agents is not a directory: %s", agentsPath)
	}
	if stateBackupPath == "" {
		return MarkdownRollbackBackupResult{}, fmt.Errorf("state backup path is required for markdown rollback backup")
	}

	base := strings.TrimSuffix(filepath.Base(stateBackupPath), filepath.Ext(stateBackupPath))
	backupRoot := filepath.Join(filepath.Dir(stateBackupPath), base+"-markdown")
	agentsBackupPath := filepath.Join(backupRoot, "agents")
	if isWithinRoot(backupRoot, root.Path()) {
		return MarkdownRollbackBackupResult{}, fmt.Errorf("rollback backup directory must be outside project root")
	}
	if err := os.MkdirAll(agentsBackupPath, 0o700); err != nil {
		return MarkdownRollbackBackupResult{}, fmt.Errorf("create markdown rollback backup directory: %w", err)
	}

	manifest := MarkdownRollbackManifest{
		ContractVersion:  StateJSONContractVersion,
		CreatedAt:        time.Now().UTC().Format(time.RFC3339Nano),
		ProjectPath:      root.Path(),
		AgentsPath:       agentsPath,
		StateBackupPath:  stateBackupPath,
		AgentsBackupPath: agentsBackupPath,
		Files:            []MarkdownRollbackFileEntry{},
	}
	err = filepath.WalkDir(agentsPath, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(root.Path(), path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		backupPath := filepath.Join(agentsBackupPath, filepath.FromSlash(strings.TrimPrefix(rel, ".agents/")))
		if err := copyFile(path, backupPath, info.Mode().Perm()); err != nil {
			return fmt.Errorf("backup markdown source %s: %w", rel, err)
		}
		sum, err := fileSHA256(backupPath)
		if err != nil {
			return fmt.Errorf("checksum markdown rollback backup %s: %w", rel, err)
		}
		manifest.Files = append(manifest.Files, MarkdownRollbackFileEntry{
			Path:       rel,
			BackupPath: backupPath,
			Bytes:      info.Size(),
			SHA256:     sum,
			Mode:       uint32(info.Mode().Perm()),
		})
		return nil
	})
	if err != nil {
		return MarkdownRollbackBackupResult{}, fmt.Errorf("create markdown rollback backup: %w", err)
	}
	sort.Slice(manifest.Files, func(i, j int) bool { return manifest.Files[i].Path < manifest.Files[j].Path })

	manifestPath := filepath.Join(backupRoot, "manifest.json")
	content, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return MarkdownRollbackBackupResult{}, fmt.Errorf("encode markdown rollback manifest: %w", err)
	}
	content = append(content, '\n')
	if err := os.WriteFile(manifestPath, content, 0o600); err != nil {
		return MarkdownRollbackBackupResult{}, fmt.Errorf("write markdown rollback manifest: %w", err)
	}

	return MarkdownRollbackBackupResult{
		ContractVersion:      StateJSONContractVersion,
		StateBackupPath:      stateBackupPath,
		RollbackManifestPath: manifestPath,
		AgentsBackupPath:     agentsBackupPath,
		FileCount:            len(manifest.Files),
		CreatedAt:            manifest.CreatedAt,
	}, nil
}

// RemoveMarkdownMigrationSources removes only ephemeral Markdown files that are
// covered by a rollback manifest. Durable renders remain tracked Markdown.
func RemoveMarkdownMigrationSources(root project.Root, manifestPath string) ([]string, error) {
	manifest, err := readMarkdownRollbackManifest(manifestPath)
	if err != nil {
		return nil, err
	}
	if err := validateMarkdownRollbackManifestRoot(root, manifest); err != nil {
		return nil, err
	}
	removed := []string{}
	for _, file := range manifest.Files {
		if !isEphemeralMarkdownMigrationSource(file.Path) {
			continue
		}
		path, err := rollbackProjectPath(root, file.Path)
		if err != nil {
			return nil, err
		}
		if strings.HasPrefix(file.Path, ".agents/drafts/") && !isKnownAgentsFile(filepath.Join(root.Path(), ".agents"), path) {
			continue
		}
		sum, err := fileSHA256(file.BackupPath)
		if err != nil {
			return nil, fmt.Errorf("checksum rollback source before removal %s: %w", file.Path, err)
		}
		if sum != file.SHA256 {
			return nil, fmt.Errorf("rollback source checksum mismatch before removal for %s", file.Path)
		}
		if err := os.Remove(path); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("remove markdown migration source %s: %w", file.Path, err)
		}
		removed = append(removed, file.Path)
	}
	sort.Strings(removed)
	return removed, nil
}

// RollbackMarkdownMigration restores .agents files from a rollback manifest.
func RollbackMarkdownMigration(ctx context.Context, root project.Root, manifestPath string) (MarkdownRollbackResult, error) {
	select {
	case <-ctx.Done():
		return MarkdownRollbackResult{}, ctx.Err()
	default:
	}
	manifest, err := readMarkdownRollbackManifest(manifestPath)
	if err != nil {
		return MarkdownRollbackResult{}, err
	}
	if err := validateMarkdownRollbackManifestRoot(root, manifest); err != nil {
		return MarkdownRollbackResult{}, err
	}

	restored := []string{}
	for _, file := range manifest.Files {
		sum, err := fileSHA256(file.BackupPath)
		if err != nil {
			return MarkdownRollbackResult{}, fmt.Errorf("checksum rollback source %s: %w", file.Path, err)
		}
		if sum != file.SHA256 {
			return MarkdownRollbackResult{}, fmt.Errorf("rollback source checksum mismatch for %s", file.Path)
		}
		path, err := rollbackProjectPath(root, file.Path)
		if err != nil {
			return MarkdownRollbackResult{}, err
		}
		if err := copyFile(file.BackupPath, path, os.FileMode(file.Mode)); err != nil {
			return MarkdownRollbackResult{}, fmt.Errorf("restore markdown source %s: %w", file.Path, err)
		}
		restored = append(restored, file.Path)
	}
	sort.Strings(restored)

	return MarkdownRollbackResult{
		ContractVersion:      StateJSONContractVersion,
		Action:               MarkdownMigrationActionRollback,
		ProjectPath:          root.Path(),
		RollbackManifestPath: manifestPath,
		StateBackupPath:      manifest.StateBackupPath,
		RestoredFiles:        restored,
		Restored:             true,
	}, nil
}

func readMarkdownRollbackManifest(path string) (MarkdownRollbackManifest, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return MarkdownRollbackManifest{}, fmt.Errorf("read markdown rollback manifest: %w", err)
	}
	var manifest MarkdownRollbackManifest
	if err := json.Unmarshal(content, &manifest); err != nil {
		return MarkdownRollbackManifest{}, fmt.Errorf("parse markdown rollback manifest: %w", err)
	}
	if manifest.ContractVersion != StateJSONContractVersion {
		return MarkdownRollbackManifest{}, fmt.Errorf("unsupported markdown rollback manifest contract version %d", manifest.ContractVersion)
	}
	if len(manifest.Files) == 0 {
		return MarkdownRollbackManifest{}, fmt.Errorf("markdown rollback manifest has no files")
	}
	return manifest, nil
}

func validateMarkdownRollbackManifestRoot(root project.Root, manifest MarkdownRollbackManifest) error {
	if manifest.ProjectPath != root.Path() {
		return fmt.Errorf("markdown rollback manifest project path %q does not match current project %q", manifest.ProjectPath, root.Path())
	}
	for _, file := range manifest.Files {
		if _, err := rollbackProjectPath(root, file.Path); err != nil {
			return err
		}
	}
	return nil
}

func rollbackProjectPath(root project.Root, rel string) (string, error) {
	if filepath.IsAbs(rel) {
		return "", fmt.Errorf("rollback path must be relative: %s", rel)
	}
	clean := filepath.Clean(filepath.FromSlash(rel))
	if clean == "." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || clean == ".." {
		return "", fmt.Errorf("rollback path escapes project root: %s", rel)
	}
	if clean != ".agents" && !strings.HasPrefix(filepath.ToSlash(clean), ".agents/") {
		return "", fmt.Errorf("rollback path is outside .agents: %s", rel)
	}
	path := filepath.Join(root.Path(), clean)
	if !isWithinRoot(path, root.Path()) && filepath.Clean(path) != filepath.Clean(root.Path()) {
		return "", fmt.Errorf("rollback path escapes project root: %s", rel)
	}
	return path, nil
}

func isEphemeralMarkdownMigrationSource(rel string) bool {
	rel = filepath.ToSlash(rel)
	if !strings.HasPrefix(rel, ".agents/") || !strings.HasSuffix(rel, ".md") {
		return false
	}
	switch {
	case strings.HasPrefix(rel, ".agents/tasks/"):
		return !strings.Contains(strings.TrimPrefix(rel, ".agents/tasks/"), "/")
	case strings.HasPrefix(rel, ".agents/ideas/"):
		return !strings.Contains(strings.TrimPrefix(rel, ".agents/ideas/"), "/")
	case strings.HasPrefix(rel, ".agents/sessions/"):
		sessionRel := strings.TrimPrefix(rel, ".agents/sessions/")
		return !strings.Contains(sessionRel, "/") || strings.HasPrefix(sessionRel, "archive/")
	case strings.HasPrefix(rel, ".agents/drafts/"):
		return !strings.Contains(strings.TrimPrefix(rel, ".agents/drafts/"), "/")
	default:
		return false
	}
}

func copyFile(src string, dst string, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Chmod(dst, mode)
}
