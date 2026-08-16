package state

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/levifig/loaf/internal/project"
)

var issueBucketTags = []string{
	issueBucketTagPrefix + IssueBucketNow,
	issueBucketTagPrefix + IssueBucketNext,
	issueBucketTagPrefix + IssueBucketLater,
}

// SetIssueBucket stores an advisory Now/Next/Later label on an issue via the
// existing tags tables. Buckets are labels only and must never be read as a
// constraint by any other code path.
func SetIssueBucket(ctx context.Context, root project.Root, resolver PathResolver, ref, bucket string) (IssueResult, error) {
	store, err := openProjectStoreMutateExisting(ctx, root, resolver)
	if err != nil {
		return IssueResult{}, err
	}
	defer store.Close()
	return store.SetIssueBucket(ctx, root, ref, bucket)
}

// SetIssueBucket writes the advisory bucket on an open store.
func (s *Store) SetIssueBucket(ctx context.Context, root project.Root, ref, bucket string) (IssueResult, error) {
	normalized, err := normalizeIssueBucket(bucket)
	if err != nil {
		return IssueResult{}, err
	}
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return IssueResult{}, err
	}
	identity, err := s.projectIdentity(ctx, projectID)
	if err != nil {
		return IssueResult{}, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return IssueResult{}, &IssueTransactionError{Stage: "begin", Err: err}
	}
	defer tx.Rollback()

	issueID, _, err := resolveIssueRefTx(ctx, tx, projectID, ref)
	if err != nil {
		return IssueResult{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if err := clearIssueBucketTagsTx(ctx, tx, projectID, issueID); err != nil {
		return IssueResult{}, err
	}
	if normalized != IssueBucketNone {
		if err := attachIssueBucketTagTx(ctx, tx, projectID, issueID, issueBucketTagPrefix+normalized, now); err != nil {
			return IssueResult{}, err
		}
	}
	result, err := loadIssueResultTx(ctx, tx, identity, issueID)
	if err != nil {
		return IssueResult{}, &IssueTransactionError{Stage: "read result", Err: err}
	}
	if err := tx.Commit(); err != nil {
		return IssueResult{}, &IssueTransactionError{Stage: "commit", Err: err}
	}
	return result, nil
}

func normalizeIssueBucket(value string) (string, error) {
	bucket := strings.ToLower(strings.TrimSpace(value))
	switch bucket {
	case IssueBucketNow, IssueBucketNext, IssueBucketLater, IssueBucketNone:
		return bucket, nil
	default:
		return "", &IssueValidationError{Field: "bucket", Err: fmt.Errorf("must be now, next, later, or none")}
	}
}

// ListIssueBuckets returns the advisory bucket label for every tagged issue.
// Missing keys mean the issue has no bucket. This is a read of labels only.
func ListIssueBuckets(ctx context.Context, root project.Root, resolver PathResolver) (map[string]string, error) {
	store, err := openProjectStoreReadExisting(ctx, root, resolver)
	if err != nil {
		return nil, err
	}
	defer store.Close()
	return store.ListIssueBuckets(ctx, root)
}

// ListIssueBuckets returns advisory buckets from an open store.
func (s *Store) ListIssueBuckets(ctx context.Context, root project.Root) (map[string]string, error) {
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT entity_tags.entity_id, tags.name
FROM entity_tags
JOIN tags
  ON tags.project_id = entity_tags.project_id
 AND tags.id = entity_tags.tag_id
WHERE entity_tags.project_id = ?
  AND entity_tags.entity_kind = ?
  AND tags.name IN (?, ?, ?)
ORDER BY entity_tags.entity_id, tags.name
`, projectID, issueEntityKind, issueBucketTags[0], issueBucketTags[1], issueBucketTags[2])
	if err != nil {
		return nil, fmt.Errorf("list issue buckets: %w", err)
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var issueID, name string
		if err := rows.Scan(&issueID, &name); err != nil {
			return nil, fmt.Errorf("scan issue bucket: %w", err)
		}
		if _, exists := out[issueID]; !exists {
			out[issueID] = strings.TrimPrefix(name, issueBucketTagPrefix)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate issue buckets: %w", err)
	}
	return out, nil
}

func loadIssueBucketTx(ctx context.Context, tx *sql.Tx, projectID, issueID string) (string, error) {
	var name sql.NullString
	err := tx.QueryRowContext(ctx, `
SELECT tags.name
FROM entity_tags
JOIN tags
  ON tags.project_id = entity_tags.project_id
 AND tags.id = entity_tags.tag_id
WHERE entity_tags.project_id = ?
  AND entity_tags.entity_kind = ?
  AND entity_tags.entity_id = ?
  AND tags.name IN (?, ?, ?)
ORDER BY tags.name
LIMIT 1
`, projectID, issueEntityKind, issueID, issueBucketTags[0], issueBucketTags[1], issueBucketTags[2]).Scan(&name)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read issue bucket: %w", err)
	}
	return strings.TrimPrefix(name.String, issueBucketTagPrefix), nil
}

func clearIssueBucketTagsTx(ctx context.Context, tx *sql.Tx, projectID, issueID string) error {
	if _, err := tx.ExecContext(ctx, `
DELETE FROM entity_tags
WHERE project_id = ?
  AND entity_kind = ?
  AND entity_id = ?
  AND tag_id IN (
    SELECT id FROM tags WHERE project_id = ? AND name IN (?, ?, ?)
  )
`, projectID, issueEntityKind, issueID, projectID, issueBucketTags[0], issueBucketTags[1], issueBucketTags[2]); err != nil {
		return &IssueTransactionError{Stage: "clear bucket", Err: err}
	}
	return nil
}

func attachIssueBucketTagTx(ctx context.Context, tx *sql.Tx, projectID, issueID, tagName, now string) error {
	tagID := stableMigrationID("tag", projectID, tagName)
	if _, err := tx.ExecContext(ctx, `
INSERT INTO tags (id, project_id, name, created_at, updated_at)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT(project_id, name) DO UPDATE SET updated_at = excluded.updated_at
`, tagID, projectID, tagName, now, now); err != nil {
		return &IssueTransactionError{Stage: "bucket tag", Err: err}
	}
	if err := tx.QueryRowContext(ctx, `SELECT id FROM tags WHERE project_id = ? AND name = ?`, projectID, tagName).Scan(&tagID); err != nil {
		return &IssueTransactionError{Stage: "bucket tag id", Err: err}
	}
	memberID := stableMigrationID("entity_tag", projectID, tagName, issueEntityKind, issueID)
	if _, err := tx.ExecContext(ctx, `
INSERT INTO entity_tags (id, project_id, tag_id, entity_kind, entity_id, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(project_id, tag_id, entity_kind, entity_id) DO UPDATE SET updated_at = excluded.updated_at
`, memberID, projectID, tagID, issueEntityKind, issueID, now, now); err != nil {
		return &IssueTransactionError{Stage: "bucket membership", Err: err}
	}
	return nil
}
