package state

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/levifig/loaf/internal/project"
)

// DocsIndexOptions controls a docs index scan.
type DocsIndexOptions struct {
	Rebuild      bool
	WorktreePath string
}

// DocsIndexResult describes a completed docs index scan.
type DocsIndexResult struct {
	ContractVersion    int            `json:"contract_version,omitempty"`
	DatabaseScope      string         `json:"database_scope,omitempty"`
	DatabasePath       string         `json:"database_path,omitempty"`
	ProjectID          string         `json:"project_id,omitempty"`
	ProjectName        string         `json:"project_name,omitempty"`
	ProjectCurrentPath string         `json:"project_current_path,omitempty"`
	IndexedWorktree    string         `json:"indexed_worktree"`
	IndexedRef         string         `json:"indexed_ref,omitempty"`
	Rebuild            bool           `json:"rebuild"`
	Scanned            int            `json:"scanned"`
	Indexed            int            `json:"indexed"`
	Removed            int            `json:"removed"`
	Docs               []DocsIndexDoc `json:"docs"`
}

// DocsIndexDoc is one indexed docs/ Markdown file.
type DocsIndexDoc struct {
	ID          string `json:"id"`
	Path        string `json:"path"`
	ContentHash string `json:"content_hash"`
}

type docsIndexCandidate struct {
	relativePath string
	content      string
	contentHash  string
}

// IndexDocs scans docs/**/*.md from the invoking project root into SQLite.
func IndexDocs(ctx context.Context, root project.Root, resolver PathResolver, options DocsIndexOptions) (DocsIndexResult, error) {
	store, err := openInitializedStore(root, resolver)
	if err != nil {
		return DocsIndexResult{}, err
	}
	defer store.Close()
	return store.IndexDocs(ctx, root, options)
}

// IndexDocs scans docs/**/*.md from the invoking project root into SQLite using an open store.
func (s *Store) IndexDocs(ctx context.Context, root project.Root, options DocsIndexOptions) (DocsIndexResult, error) {
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return DocsIndexResult{}, err
	}
	identity, err := s.projectIdentity(ctx, projectID)
	if err != nil {
		return DocsIndexResult{}, err
	}
	indexedWorktree, err := docsIndexWorktreePath(root, options.WorktreePath)
	if err != nil {
		return DocsIndexResult{}, err
	}
	candidates, err := scanDocsIndexCandidates(indexedWorktree)
	if err != nil {
		return DocsIndexResult{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	indexedRef := observedDocsGitRef(indexedWorktree)
	indexedWorktree = filepath.ToSlash(indexedWorktree)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return DocsIndexResult{}, fmt.Errorf("begin docs index transaction: %w", err)
	}
	defer tx.Rollback()

	removed := 0
	if options.Rebuild {
		count, err := deleteDocsIndexRows(ctx, tx, projectID, indexedWorktree, nil)
		if err != nil {
			return DocsIndexResult{}, err
		}
		removed += count
	}

	seen := make(map[string]bool, len(candidates))
	docs := make([]DocsIndexDoc, 0, len(candidates))
	for _, candidate := range candidates {
		seen[candidate.relativePath] = true
		docID := stableMigrationID("doc-index", projectID, indexedWorktree, candidate.relativePath)
		rowid, err := upsertDocsIndexCandidate(ctx, tx, projectID, docID, indexedWorktree, indexedRef, now, candidate)
		if err != nil {
			return DocsIndexResult{}, err
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO docs_search(rowid, project_id, id, path, content)
VALUES (?, ?, ?, ?, ?)
`, rowid, projectID, docID, candidate.relativePath, candidate.content); err != nil {
			return DocsIndexResult{}, fmt.Errorf("insert docs search row %s: %w", candidate.relativePath, err)
		}
		docs = append(docs, DocsIndexDoc{ID: docID, Path: candidate.relativePath, ContentHash: candidate.contentHash})
	}
	if !options.Rebuild {
		count, err := deleteDocsIndexRows(ctx, tx, projectID, indexedWorktree, seen)
		if err != nil {
			return DocsIndexResult{}, err
		}
		removed += count
	}

	if err := tx.Commit(); err != nil {
		return DocsIndexResult{}, fmt.Errorf("commit docs index transaction: %w", err)
	}

	return DocsIndexResult{
		ContractVersion:    StateJSONContractVersion,
		DatabaseScope:      identity.DatabaseScope,
		DatabasePath:       identity.DatabasePath,
		ProjectID:          identity.ID,
		ProjectName:        identity.FriendlyName,
		ProjectCurrentPath: identity.CurrentPath,
		IndexedWorktree:    indexedWorktree,
		IndexedRef:         indexedRef,
		Rebuild:            options.Rebuild,
		Scanned:            len(candidates),
		Indexed:            len(candidates),
		Removed:            removed,
		Docs:               docs,
	}, nil
}

func docsIndexWorktreePath(root project.Root, override string) (string, error) {
	path := strings.TrimSpace(override)
	if path == "" {
		path = root.Path()
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve docs index worktree path: %w", err)
	}
	return filepath.Clean(abs), nil
}

func scanDocsIndexCandidates(rootPath string) ([]docsIndexCandidate, error) {
	docsRoot := filepath.Join(rootPath, "docs")
	if _, err := os.Stat(docsRoot); os.IsNotExist(err) {
		return nil, nil
	} else if err != nil {
		return nil, fmt.Errorf("stat docs directory: %w", err)
	}

	var candidates []docsIndexCandidate
	err := filepath.WalkDir(docsRoot, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if !strings.EqualFold(filepath.Ext(path), ".md") {
			return nil
		}
		rel, err := filepath.Rel(rootPath, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if skipDocsIndexPath(rel) {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read docs file %s: %w", rel, err)
		}
		if !utf8.Valid(body) {
			return fmt.Errorf("docs file %s must be UTF-8 text", rel)
		}
		hash := sha256.Sum256(body)
		candidates = append(candidates, docsIndexCandidate{
			relativePath: rel,
			content:      string(body),
			contentHash:  hex.EncodeToString(hash[:]),
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan docs directory: %w", err)
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].relativePath < candidates[j].relativePath
	})
	return candidates, nil
}

func skipDocsIndexPath(relativePath string) bool {
	clean := filepath.ToSlash(relativePath)
	base := strings.ToLower(filepath.Base(clean))
	return base == "readme.md" && strings.HasPrefix(clean, "docs/decisions/")
}

func upsertDocsIndexCandidate(ctx context.Context, tx *sql.Tx, projectID string, docID string, indexedWorktree string, indexedRef string, now string, candidate docsIndexCandidate) (int64, error) {
	var rowid int64
	var existingID string
	var existingPath string
	var existingContent string
	err := tx.QueryRowContext(ctx, `
SELECT rowid, id, path, content
FROM docs_index
WHERE project_id = ? AND indexed_worktree = ? AND path = ?
`, projectID, indexedWorktree, candidate.relativePath).Scan(&rowid, &existingID, &existingPath, &existingContent)
	switch {
	case err == nil:
		if err := deleteDocsSearchRow(ctx, tx, docsSearchRow{
			RowID:     rowid,
			ProjectID: projectID,
			DocID:     existingID,
			Path:      existingPath,
			Content:   existingContent,
		}); err != nil {
			return 0, err
		}
		if _, err := tx.ExecContext(ctx, `
UPDATE docs_index
SET content = ?, content_hash = ?, indexed_ref = ?, indexed_at = ?, updated_at = ?
WHERE project_id = ? AND indexed_worktree = ? AND path = ?
`, candidate.content, candidate.contentHash, emptyToNil(indexedRef), now, now, projectID, indexedWorktree, candidate.relativePath); err != nil {
			return 0, fmt.Errorf("update docs index row %s: %w", candidate.relativePath, err)
		}
		return rowid, nil
	case !errors.Is(err, sql.ErrNoRows):
		return 0, fmt.Errorf("read docs index row %s: %w", candidate.relativePath, err)
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO docs_index (id, project_id, path, content, content_hash, indexed_ref, indexed_worktree, indexed_at, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`, docID, projectID, candidate.relativePath, candidate.content, candidate.contentHash, emptyToNil(indexedRef), indexedWorktree, now, now, now); err != nil {
		return 0, fmt.Errorf("insert docs index row %s: %w", candidate.relativePath, err)
	}
	if err := tx.QueryRowContext(ctx, `SELECT rowid FROM docs_index WHERE id = ?`, docID).Scan(&rowid); err != nil {
		return 0, fmt.Errorf("read inserted docs index rowid %s: %w", candidate.relativePath, err)
	}
	return rowid, nil
}

type docsSearchRow struct {
	RowID     int64
	ProjectID string
	DocID     string
	Path      string
	Content   string
}

func deleteDocsIndexRows(ctx context.Context, tx *sql.Tx, projectID string, indexedWorktree string, keep map[string]bool) (int, error) {
	rows, err := tx.QueryContext(ctx, `
SELECT rowid, id, path, content
FROM docs_index
WHERE project_id = ? AND indexed_worktree = ?
`, projectID, indexedWorktree)
	if err != nil {
		return 0, fmt.Errorf("query docs index rows for deletion: %w", err)
	}
	var deleteRows []docsSearchRow
	for rows.Next() {
		var row docsSearchRow
		row.ProjectID = projectID
		if err := rows.Scan(&row.RowID, &row.DocID, &row.Path, &row.Content); err != nil {
			rows.Close()
			return 0, fmt.Errorf("scan docs index deletion row: %w", err)
		}
		if keep == nil || !keep[row.Path] {
			deleteRows = append(deleteRows, row)
		}
	}
	if err := rows.Close(); err != nil {
		return 0, fmt.Errorf("close docs index deletion rows: %w", err)
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterate docs index deletion rows: %w", err)
	}
	for _, row := range deleteRows {
		if err := deleteDocsSearchRow(ctx, tx, row); err != nil {
			return 0, fmt.Errorf("delete docs search row %s: %w", row.Path, err)
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM docs_index WHERE rowid = ?`, row.RowID); err != nil {
			return 0, fmt.Errorf("delete docs index row %s: %w", row.Path, err)
		}
	}
	return len(deleteRows), nil
}

func deleteDocsSearchRow(ctx context.Context, tx *sql.Tx, row docsSearchRow) error {
	if _, err := tx.ExecContext(ctx, `
INSERT INTO docs_search(docs_search, rowid, project_id, id, path, content)
VALUES ('delete', ?, ?, ?, ?, ?)
`, row.RowID, row.ProjectID, row.DocID, row.Path, row.Content); err != nil {
		return fmt.Errorf("delete docs search row %d: %w", row.RowID, err)
	}
	return nil
}

func observedDocsGitRef(rootPath string) string {
	branch := gitCommandOutput(rootPath, "rev-parse", "--abbrev-ref", "HEAD")
	if branch != "" && branch != "HEAD" {
		return branch
	}
	return gitCommandOutput(rootPath, "rev-parse", "HEAD")
}

func gitCommandOutput(dir string, args ...string) string {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
