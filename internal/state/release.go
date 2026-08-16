package state

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/levifig/loaf/internal/project"
)

const (
	ReleaseMemberKindIssue   = "issue"
	ReleaseMemberKindRelease = "release"
)

// ReleaseValidationError identifies malformed release input.
type ReleaseValidationError struct {
	Field string
	Err   error
}

func (e *ReleaseValidationError) Error() string {
	if e == nil {
		return "release validation failed"
	}
	return fmt.Sprintf("release validation failed for %s: %v", e.Field, e.Err)
}

func (e *ReleaseValidationError) Unwrap() error { return e.Err }

// Release is one recorded retroactive release.
type Release struct {
	ID           string          `json:"id"`
	Version      string          `json:"version"`
	Tag          string          `json:"tag"`
	TaggedCommit string          `json:"tagged_commit"`
	Notes        string          `json:"notes,omitempty"`
	Members      []ReleaseMember `json:"members,omitempty"`
	CreatedAt    string          `json:"created_at"`
	UpdatedAt    string          `json:"updated_at"`
}

// ReleaseMember is one recorded fact about what landed in a release.
type ReleaseMember struct {
	ID         string `json:"id"`
	Kind       string `json:"member_kind"`
	MemberID   string `json:"member_id"`
	RecordedAt string `json:"recorded_at"`
}

// RecordReleaseOptions describes a retroactive release to persist.
type RecordReleaseOptions struct {
	Version      string
	Tag          string
	TaggedCommit string
	Notes        string
	IssueIDs     []string
	IncludedIDs  []string
}

// ReleaseResult is one release plus project identity for CLI JSON.
type ReleaseResult struct {
	ContractVersion    int     `json:"contract_version,omitempty"`
	DatabaseScope      string  `json:"database_scope,omitempty"`
	DatabasePath       string  `json:"database_path,omitempty"`
	ProjectID          string  `json:"project_id,omitempty"`
	ProjectName        string  `json:"project_name,omitempty"`
	ProjectCurrentPath string  `json:"project_current_path,omitempty"`
	Release            Release `json:"release"`
}

// JournalEntryRecord list of commit() journal rows used by release attribution.
func ListCommitJournalEntries(ctx context.Context, root project.Root, resolver PathResolver) ([]JournalEntryRecord, error) {
	store, err := openProjectStoreReadExisting(ctx, root, resolver)
	if err != nil {
		return nil, err
	}
	defer store.Close()
	return store.ListCommitJournalEntries(ctx, root)
}

// ListCommitJournalEntries returns commit() journal rows from an open store.
func (s *Store) ListCommitJournalEntries(ctx context.Context, root project.Root) ([]JournalEntryRecord, error) {
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT id, entry_type, COALESCE(scope, ''), message, COALESCE(observed_branch, ''),
       COALESCE(observed_worktree, ''), COALESCE(harness_session_id, ''), created_at
FROM journal_entries
WHERE project_id = ? AND entry_type = 'commit'
ORDER BY created_at, id
`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list commit journal entries: %w", err)
	}
	defer rows.Close()
	entries := []JournalEntryRecord{}
	for rows.Next() {
		var entry JournalEntryRecord
		if err := rows.Scan(&entry.ID, &entry.EntryType, &entry.Scope, &entry.Message, &entry.ObservedBranch, &entry.ObservedWorktree, &entry.HarnessSessionID, &entry.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan commit journal entry: %w", err)
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate commit journal entries: %w", err)
	}
	return entries, nil
}

// RecordRelease persists a release and its members as one transaction.
func RecordRelease(ctx context.Context, root project.Root, resolver PathResolver, options RecordReleaseOptions) (Release, error) {
	store, err := openProjectStoreMutateExisting(ctx, root, resolver)
	if err != nil {
		return Release{}, err
	}
	defer store.Close()
	return store.RecordRelease(ctx, root, options)
}

// RecordRelease persists a release on an open store.
func (s *Store) RecordRelease(ctx context.Context, root project.Root, options RecordReleaseOptions) (Release, error) {
	version := strings.TrimSpace(options.Version)
	if version == "" {
		return Release{}, &ReleaseValidationError{Field: "version", Err: fmt.Errorf("must be nonempty")}
	}
	tag := strings.TrimSpace(options.Tag)
	if tag == "" {
		return Release{}, &ReleaseValidationError{Field: "tag", Err: fmt.Errorf("must be nonempty")}
	}
	taggedCommit := strings.TrimSpace(options.TaggedCommit)
	if taggedCommit == "" {
		return Release{}, &ReleaseValidationError{Field: "tagged_commit", Err: fmt.Errorf("must be nonempty")}
	}

	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return Release{}, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return Release{}, fmt.Errorf("begin record release: %w", err)
	}
	defer tx.Rollback()

	existing, err := lookupReleaseByVersionOrTagTx(ctx, tx, projectID, version, tag)
	switch {
	case err == nil:
		if existing.Version != version || existing.Tag != tag || existing.TaggedCommit != taggedCommit {
			return Release{}, &ReleaseValidationError{
				Field: "release",
				Err:   fmt.Errorf("already recorded as version %s tag %s commit %s", existing.Version, existing.Tag, existing.TaggedCommit),
			}
		}
		if diffs := releaseRecordedContentDiffs(existing, options); len(diffs) > 0 {
			return Release{}, &ReleaseValidationError{
				Field: "release",
				Err:   fmt.Errorf("already recorded with divergent %s", strings.Join(diffs, ", ")),
			}
		}
		return existing, nil
	case !errors.Is(err, sql.ErrNoRows):
		return Release{}, err
	}

	releaseID, err := newOpaqueStateID("rel")
	if err != nil {
		return Release{}, fmt.Errorf("mint release id: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, `
INSERT INTO releases (id, project_id, version, tag, tagged_commit, notes, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
`, releaseID, projectID, version, tag, taggedCommit, options.Notes, now, now); err != nil {
		return Release{}, fmt.Errorf("insert release: %w", err)
	}

	seen := map[string]bool{}
	for _, issueID := range options.IssueIDs {
		issueID = strings.TrimSpace(issueID)
		if issueID == "" {
			continue
		}
		key := ReleaseMemberKindIssue + "\x00" + issueID
		if seen[key] {
			continue
		}
		seen[key] = true
		if err := insertReleaseMemberTx(ctx, tx, projectID, releaseID, ReleaseMemberKindIssue, issueID, now); err != nil {
			return Release{}, err
		}
	}
	for _, includedID := range options.IncludedIDs {
		includedID = strings.TrimSpace(includedID)
		if includedID == "" {
			continue
		}
		if includedID == releaseID {
			return Release{}, &ReleaseValidationError{Field: "includes", Err: fmt.Errorf("a release cannot include itself")}
		}
		key := ReleaseMemberKindRelease + "\x00" + includedID
		if seen[key] {
			continue
		}
		seen[key] = true
		if err := insertReleaseMemberTx(ctx, tx, projectID, releaseID, ReleaseMemberKindRelease, includedID, now); err != nil {
			return Release{}, err
		}
	}

	detail, err := loadReleaseTx(ctx, tx, projectID, releaseID)
	if err != nil {
		return Release{}, err
	}
	if err := tx.Commit(); err != nil {
		return Release{}, fmt.Errorf("commit record release: %w", err)
	}
	return detail, nil
}

// GetRelease returns one release by opaque id, version, or tag.
func GetRelease(ctx context.Context, root project.Root, resolver PathResolver, ref string) (Release, error) {
	store, err := openProjectStoreReadExisting(ctx, root, resolver)
	if err != nil {
		return Release{}, err
	}
	defer store.Close()
	return store.GetRelease(ctx, root, ref)
}

// GetRelease returns one release from an open store.
func (s *Store) GetRelease(ctx context.Context, root project.Root, ref string) (Release, error) {
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return Release{}, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return Release{}, fmt.Errorf("begin get release: %w", err)
	}
	defer tx.Rollback()
	releaseID, err := resolveReleaseRefTx(ctx, tx, projectID, ref)
	if err != nil {
		return Release{}, err
	}
	return loadReleaseTx(ctx, tx, projectID, releaseID)
}

// ListReleases returns every recorded release for the project.
func ListReleases(ctx context.Context, root project.Root, resolver PathResolver) ([]Release, error) {
	store, err := openProjectStoreReadExisting(ctx, root, resolver)
	if err != nil {
		return nil, err
	}
	defer store.Close()
	return store.ListReleases(ctx, root)
}

// ListReleases returns recorded releases from an open store.
func (s *Store) ListReleases(ctx context.Context, root project.Root) ([]Release, error) {
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return nil, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("begin list releases: %w", err)
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `
SELECT id FROM releases WHERE project_id = ? ORDER BY created_at, id
`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list releases: %w", err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan release id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate releases: %w", err)
	}
	releases := make([]Release, 0, len(ids))
	for _, id := range ids {
		detail, err := loadReleaseTx(ctx, tx, projectID, id)
		if err != nil {
			return nil, err
		}
		releases = append(releases, detail)
	}
	return releases, nil
}

func releaseRecordedContentDiffs(existing Release, options RecordReleaseOptions) []string {
	var diffs []string
	if !sameStringSet(memberIDsOfKind(existing.Members, ReleaseMemberKindIssue), trimmedNonEmptySet(options.IssueIDs)) {
		diffs = append(diffs, "issue members")
	}
	if !sameStringSet(memberIDsOfKind(existing.Members, ReleaseMemberKindRelease), trimmedNonEmptySet(options.IncludedIDs)) {
		diffs = append(diffs, "included members")
	}
	if existing.Notes != options.Notes {
		diffs = append(diffs, "notes")
	}
	return diffs
}

func memberIDsOfKind(members []ReleaseMember, kind string) map[string]struct{} {
	out := make(map[string]struct{})
	for _, member := range members {
		if member.Kind == kind && member.MemberID != "" {
			out[member.MemberID] = struct{}{}
		}
	}
	return out
}

func trimmedNonEmptySet(ids []string) map[string]struct{} {
	out := make(map[string]struct{})
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		out[id] = struct{}{}
	}
	return out
}

func sameStringSet(a, b map[string]struct{}) bool {
	if len(a) != len(b) {
		return false
	}
	for key := range a {
		if _, ok := b[key]; !ok {
			return false
		}
	}
	return true
}

func insertReleaseMemberTx(ctx context.Context, tx *sql.Tx, projectID, releaseID, kind, memberID, now string) error {
	if kind != ReleaseMemberKindIssue && kind != ReleaseMemberKindRelease {
		return &ReleaseValidationError{Field: "member_kind", Err: fmt.Errorf("must be issue or release")}
	}
	if err := validateReleaseMemberExistsTx(ctx, tx, projectID, kind, memberID); err != nil {
		return err
	}
	memberRowID, err := newOpaqueStateID("rlm")
	if err != nil {
		return fmt.Errorf("mint release member id: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO release_members (id, project_id, release_id, member_kind, member_id, recorded_at)
VALUES (?, ?, ?, ?, ?, ?)
`, memberRowID, projectID, releaseID, kind, memberID, now); err != nil {
		return fmt.Errorf("insert release member: %w", err)
	}
	return nil
}

func validateReleaseMemberExistsTx(ctx context.Context, tx *sql.Tx, projectID, kind, memberID string) error {
	var exists string
	var err error
	switch kind {
	case ReleaseMemberKindIssue:
		err = tx.QueryRowContext(ctx, `SELECT id FROM issues WHERE project_id = ? AND id = ?`, projectID, memberID).Scan(&exists)
		if err == sql.ErrNoRows {
			return &ReleaseValidationError{Field: "member_id", Err: fmt.Errorf("issue %s not found in this project", memberID)}
		}
	case ReleaseMemberKindRelease:
		err = tx.QueryRowContext(ctx, `SELECT id FROM releases WHERE project_id = ? AND id = ?`, projectID, memberID).Scan(&exists)
		if err == sql.ErrNoRows {
			return &ReleaseValidationError{Field: "includes", Err: fmt.Errorf("release %s not found in this project", memberID)}
		}
	default:
		return &ReleaseValidationError{Field: "member_kind", Err: fmt.Errorf("must be issue or release")}
	}
	if err != nil {
		return fmt.Errorf("validate release member %s: %w", memberID, err)
	}
	return nil
}

func lookupReleaseByVersionOrTagTx(ctx context.Context, tx *sql.Tx, projectID, version, tag string) (Release, error) {
	rows, err := tx.QueryContext(ctx, `
SELECT id FROM releases
WHERE project_id = ? AND (version = ? OR tag = ?)
ORDER BY created_at, id
`, projectID, version, tag)
	if err != nil {
		return Release{}, fmt.Errorf("lookup release %s/%s: %w", version, tag, err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return Release{}, fmt.Errorf("scan release id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return Release{}, fmt.Errorf("iterate release lookup: %w", err)
	}
	if len(ids) == 0 {
		return Release{}, sql.ErrNoRows
	}
	if len(ids) > 1 {
		return Release{}, &ReleaseValidationError{
			Field: "release",
			Err:   fmt.Errorf("version %s and tag %s match different recorded releases", version, tag),
		}
	}
	return loadReleaseTx(ctx, tx, projectID, ids[0])
}

func resolveReleaseRefTx(ctx context.Context, tx *sql.Tx, projectID, ref string) (string, error) {
	trimmed := strings.TrimSpace(ref)
	if trimmed == "" {
		return "", &ReleaseValidationError{Field: "release", Err: fmt.Errorf("must be nonempty")}
	}
	var id string
	err := tx.QueryRowContext(ctx, `
SELECT id FROM releases
WHERE project_id = ? AND (id = ? OR version = ? OR tag = ?)
ORDER BY CASE WHEN id = ? THEN 0 WHEN version = ? THEN 1 ELSE 2 END, created_at, id
LIMIT 1
`, projectID, trimmed, trimmed, trimmed, trimmed, trimmed).Scan(&id)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("release %q not found in SQLite state", trimmed)
	}
	if err != nil {
		return "", fmt.Errorf("resolve release %q: %w", trimmed, err)
	}
	return id, nil
}

func loadReleaseTx(ctx context.Context, tx *sql.Tx, projectID, releaseID string) (Release, error) {
	var release Release
	var notes sql.NullString
	err := tx.QueryRowContext(ctx, `
SELECT id, version, tag, tagged_commit, notes, created_at, updated_at
FROM releases
WHERE project_id = ? AND id = ?
`, projectID, releaseID).Scan(&release.ID, &release.Version, &release.Tag, &release.TaggedCommit, &notes, &release.CreatedAt, &release.UpdatedAt)
	if err == sql.ErrNoRows {
		return Release{}, fmt.Errorf("release %s not found", releaseID)
	}
	if err != nil {
		return Release{}, fmt.Errorf("load release %s: %w", releaseID, err)
	}
	release.Notes = notes.String
	members, err := loadReleaseMembersTx(ctx, tx, projectID, releaseID)
	if err != nil {
		return Release{}, err
	}
	release.Members = members
	return release, nil
}

func loadReleaseMembersTx(ctx context.Context, tx *sql.Tx, projectID, releaseID string) ([]ReleaseMember, error) {
	rows, err := tx.QueryContext(ctx, `
SELECT id, member_kind, member_id, recorded_at
FROM release_members
WHERE project_id = ? AND release_id = ?
ORDER BY recorded_at, id
`, projectID, releaseID)
	if err != nil {
		return nil, fmt.Errorf("list release members: %w", err)
	}
	defer rows.Close()
	members := []ReleaseMember{}
	for rows.Next() {
		var member ReleaseMember
		if err := rows.Scan(&member.ID, &member.Kind, &member.MemberID, &member.RecordedAt); err != nil {
			return nil, fmt.Errorf("scan release member: %w", err)
		}
		members = append(members, member)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate release members: %w", err)
	}
	return members, nil
}
