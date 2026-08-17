package state

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/levifig/loaf/internal/project"
)

// IssuePrefixAlignItem is one project rewritten (or previewed) by --align.
type IssuePrefixAlignItem struct {
	ProjectID   string `json:"project_id"`
	ProjectName string `json:"project_name,omitempty"`
	FromPrefix  string `json:"from_prefix"`
	ToPrefix    string `json:"to_prefix"`
	Aliases     int    `json:"aliases"`
	Notes       int    `json:"notes"`
}

// IssuePrefixAlignResult is the plan or apply envelope for loaf issue identity --align.
type IssuePrefixAlignResult struct {
	ContractVersion    int                    `json:"contract_version,omitempty"`
	DatabaseScope      string                 `json:"database_scope,omitempty"`
	DatabasePath       string                 `json:"database_path,omitempty"`
	ProjectID          string                 `json:"project_id,omitempty"`
	ProjectName        string                 `json:"project_name,omitempty"`
	ProjectCurrentPath string                 `json:"project_current_path,omitempty"`
	All                bool                   `json:"all"`
	DryRun             bool                   `json:"dry_run"`
	Rewritten          int                    `json:"rewritten"`
	Authority          string                 `json:"authority,omitempty"`
	Prefix             string                 `json:"prefix,omitempty"`
	ConfigWritten      bool                   `json:"config_written,omitempty"`
	TeamKeyWritten     bool                   `json:"team_key_written,omitempty"`
	Items              []IssuePrefixAlignItem `json:"items"`
}

// DeriveIssuePrefix turns a project name or path into a local issue prefix.
// Empty means the slug cannot form a valid prefix; callers fall back to LOAF.
func DeriveIssuePrefix(name, path string) string {
	for _, raw := range []string{name, filepath.Base(strings.TrimSpace(path))} {
		if prefix := issuePrefixFromSlug(raw); prefix != "" {
			return prefix
		}
	}
	return ""
}

func issuePrefixFromSlug(raw string) string {
	var b strings.Builder
	for i := 0; i < len(raw); i++ {
		c := raw[i]
		switch {
		case c >= 'a' && c <= 'z':
			b.WriteByte(c - 'a' + 'A')
		case c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
			b.WriteByte(c)
		}
	}
	prefix := b.String()
	if err := validateIssuePrefix(prefix); err != nil {
		return ""
	}
	return prefix
}

func derivedIssuePrefixTx(ctx context.Context, tx *sql.Tx, projectID string) (string, error) {
	prefix, _, err := resolveIssueIdentityDefaultsTx(ctx, tx, projectID)
	return prefix, err
}

// IssuePrefixLeak reports a materialized LOAF prefix on a project whose slug
// derives to something else.
func IssuePrefixLeak(ctx context.Context, root project.Root, resolver PathResolver) (from, to string, leaked bool, err error) {
	store, err := openProjectStoreReadExisting(ctx, root, resolver)
	if err != nil {
		return "", "", false, err
	}
	defer store.Close()
	projectID, err := store.projectID(ctx, root)
	if err != nil {
		return "", "", false, err
	}
	return store.issuePrefixLeak(ctx, projectID)
}

func (s *Store) issuePrefixLeak(ctx context.Context, projectID string) (from, to string, leaked bool, err error) {
	identity, err := s.LookupIssueIdentityForProject(ctx, projectID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", false, nil
	}
	if err != nil {
		return "", "", false, err
	}
	if identity.Prefix != DefaultIssuePrefix {
		return "", "", false, nil
	}
	proj, err := s.projectIdentity(ctx, projectID)
	if err != nil {
		return "", "", false, err
	}
	if cfg, cfgErr := LoadIssueProjectConfig(proj.CurrentPath); cfgErr == nil && cfg.Prefix != "" {
		return "", "", false, nil
	}
	derived := DeriveIssuePrefix(proj.FriendlyName, proj.CurrentPath)
	if derived == "" || derived == DefaultIssuePrefix {
		return "", "", false, nil
	}
	return identity.Prefix, derived, true, nil
}

// LookupIssueIdentityForProject returns the stored identity row for a project id.
func (s *Store) LookupIssueIdentityForProject(ctx context.Context, projectID string) (IssueIdentity, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return IssueIdentity{}, err
	}
	defer tx.Rollback()
	return loadIssueIdentityTx(ctx, tx, projectID)
}

// AlignLeakedIssuePrefix rewrites a leaked LOAF prefix on the current project.
func AlignLeakedIssuePrefix(ctx context.Context, root project.Root, resolver PathResolver, dryRun bool) (IssuePrefixAlignResult, error) {
	return alignLeakedIssuePrefixes(ctx, root, resolver, false, dryRun)
}

// AlignLeakedIssuePrefixes rewrites leaked LOAF prefixes. All walks every
// project in the global database.
func AlignLeakedIssuePrefixes(ctx context.Context, root project.Root, resolver PathResolver, all, dryRun bool) (IssuePrefixAlignResult, error) {
	return alignLeakedIssuePrefixes(ctx, root, resolver, all, dryRun)
}

func alignLeakedIssuePrefixes(ctx context.Context, root project.Root, resolver PathResolver, all, dryRun bool) (IssuePrefixAlignResult, error) {
	store, err := openProjectStoreMutateExisting(ctx, root, resolver)
	if err != nil {
		return IssuePrefixAlignResult{}, err
	}
	defer store.Close()
	projectID, err := store.projectID(ctx, root)
	if err != nil {
		return IssuePrefixAlignResult{}, err
	}
	identity, err := store.projectIdentity(ctx, projectID)
	if err != nil {
		return IssuePrefixAlignResult{}, err
	}
	tx, err := store.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return IssuePrefixAlignResult{}, &IssueTransactionError{Stage: "begin prefix align", Err: err}
	}
	defer tx.Rollback()

	now := time.Now().UTC().Format(time.RFC3339Nano)
	ids := []string{projectID}
	if all {
		ids, err = listIssueIdentityProjectIDsTx(ctx, tx)
		if err != nil {
			return IssuePrefixAlignResult{}, err
		}
	}
	items := []IssuePrefixAlignItem{}
	for _, id := range ids {
		item, err := alignLeakedIssuePrefixTx(ctx, tx, id, now, dryRun)
		if err != nil {
			return IssuePrefixAlignResult{}, err
		}
		if item.ToPrefix == "" {
			continue
		}
		items = append(items, item)
	}
	if !dryRun {
		if err := tx.Commit(); err != nil {
			return IssuePrefixAlignResult{}, &IssueTransactionError{Stage: "commit prefix align", Err: err}
		}
		if err := persistAlignedIssueConfigs(ctx, store, items); err != nil {
			return IssuePrefixAlignResult{}, err
		}
	}
	return IssuePrefixAlignResult{
		ContractVersion:    StateJSONContractVersion,
		DatabaseScope:      identity.DatabaseScope,
		DatabasePath:       identity.DatabasePath,
		ProjectID:          identity.ID,
		ProjectName:        identity.FriendlyName,
		ProjectCurrentPath: identity.CurrentPath,
		All:                all,
		DryRun:             dryRun,
		Rewritten:          len(items),
		Items:              items,
		ConfigWritten:      !dryRun && len(items) > 0,
	}, nil
}

func listIssueIdentityProjectIDsTx(ctx context.Context, tx *sql.Tx) ([]string, error) {
	rows, err := tx.QueryContext(ctx, `SELECT project_id FROM issue_identity ORDER BY project_id`)
	if err != nil {
		return nil, fmt.Errorf("list issue identity projects: %w", err)
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan issue identity project: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// DefineIssueIdentityOptions sets prefix and/or authority and persists loaf.json.
type DefineIssueIdentityOptions struct {
	Prefix    string
	Authority string
	DryRun    bool
}

// DefineIssuePrefix sets the issue prefix, rewrites local aliases, and writes loaf.json.
func DefineIssuePrefix(ctx context.Context, root project.Root, resolver PathResolver, prefix string, dryRun bool) (IssuePrefixAlignResult, error) {
	return DefineIssueIdentity(ctx, root, resolver, DefineIssueIdentityOptions{Prefix: prefix, DryRun: dryRun})
}

// DefineIssueIdentity sets prefix and/or authority. Local prefix changes rewrite
// aliases. Tracker authorities record the prefix (Linear team key) without rewrite.
func DefineIssueIdentity(ctx context.Context, root project.Root, resolver PathResolver, options DefineIssueIdentityOptions) (IssuePrefixAlignResult, error) {
	store, err := openProjectStoreMutateExisting(ctx, root, resolver)
	if err != nil {
		return IssuePrefixAlignResult{}, err
	}
	defer store.Close()
	projectID, err := store.projectID(ctx, root)
	if err != nil {
		return IssuePrefixAlignResult{}, err
	}
	identity, err := store.projectIdentity(ctx, projectID)
	if err != nil {
		return IssuePrefixAlignResult{}, err
	}
	tx, err := store.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return IssuePrefixAlignResult{}, &IssueTransactionError{Stage: "begin prefix define", Err: err}
	}
	defer tx.Rollback()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	current, err := ensureIssueIdentityTx(ctx, tx, projectID, now)
	if err != nil {
		return IssuePrefixAlignResult{}, err
	}
	toPrefix := current.Prefix
	if strings.TrimSpace(options.Prefix) != "" {
		toPrefix, err = normalizeStoredIssuePrefix(options.Prefix)
		if err != nil {
			return IssuePrefixAlignResult{}, err
		}
	}
	toAuthority := current.Authority
	if strings.TrimSpace(options.Authority) != "" {
		toAuthority, err = normalizeIssueAuthorityValue(options.Authority)
		if err != nil {
			return IssuePrefixAlignResult{}, err
		}
	}
	result := IssuePrefixAlignResult{
		ContractVersion:    StateJSONContractVersion,
		DatabaseScope:      identity.DatabaseScope,
		DatabasePath:       identity.DatabasePath,
		ProjectID:          identity.ID,
		ProjectName:        identity.FriendlyName,
		ProjectCurrentPath: identity.CurrentPath,
		DryRun:             options.DryRun,
		Authority:          toAuthority,
		Prefix:             toPrefix,
	}
	if toAuthority != IssueAuthorityLocal {
		if current.Prefix != toPrefix || current.Authority != toAuthority {
			result.Items = []IssuePrefixAlignItem{{
				ProjectID:   projectID,
				ProjectName: identity.FriendlyName,
				FromPrefix:  current.Prefix,
				ToPrefix:    toPrefix,
			}}
			result.Rewritten = 1
		}
		if !options.DryRun {
			if _, err := tx.ExecContext(ctx, `
UPDATE issue_identity SET authority = ?, prefix = ?, updated_at = ? WHERE project_id = ?
`, toAuthority, toPrefix, now, projectID); err != nil {
				return IssuePrefixAlignResult{}, &IssueTransactionError{Stage: "identity", Err: err}
			}
			if err := tx.Commit(); err != nil {
				return IssuePrefixAlignResult{}, &IssueTransactionError{Stage: "commit prefix define", Err: err}
			}
			if toAuthority == IssueAuthorityLinear && toPrefix != "" {
				if err := store.upsertBackendMapping(ctx, root, backendMapping{
					EntityKind:   "project",
					EntityID:     projectID,
					ExternalKind: linearExternalKindTeam,
					ExternalID:   toPrefix,
					SyncStatus:   linearSyncLinked,
				}); err != nil {
					return IssuePrefixAlignResult{}, err
				}
				result.TeamKeyWritten = true
			}
			if err := persistIssueProjectConfig(root.Path(), toAuthority, toPrefix); err != nil {
				return result, fmt.Errorf("identity applied; persist .agents/loaf.json failed: %w", err)
			}
			result.ConfigWritten = true
		}
		return result, nil
	}
	if current.Authority == IssueAuthorityLocal {
		item, err := rewriteIssuePrefixTx(ctx, tx, projectID, current.Prefix, toPrefix, now, options.DryRun)
		if err != nil {
			return IssuePrefixAlignResult{}, err
		}
		if item.ToPrefix != "" {
			result.Items = []IssuePrefixAlignItem{item}
			result.Rewritten = 1
		}
	} else if current.Prefix != toPrefix || current.Authority != toAuthority {
		result.Items = []IssuePrefixAlignItem{{
			ProjectID:   projectID,
			ProjectName: identity.FriendlyName,
			FromPrefix:  current.Prefix,
			ToPrefix:    toPrefix,
		}}
		result.Rewritten = 1
	}
	if !options.DryRun && (current.Authority != toAuthority || (current.Authority != IssueAuthorityLocal && current.Prefix != toPrefix)) {
		if _, err := tx.ExecContext(ctx, `
UPDATE issue_identity SET authority = ?, prefix = ?, updated_at = ? WHERE project_id = ?
`, toAuthority, toPrefix, now, projectID); err != nil {
			return IssuePrefixAlignResult{}, &IssueTransactionError{Stage: "identity", Err: err}
		}
	}
	if !options.DryRun {
		if err := tx.Commit(); err != nil {
			return IssuePrefixAlignResult{}, &IssueTransactionError{Stage: "commit prefix define", Err: err}
		}
		if err := persistIssueProjectConfig(root.Path(), toAuthority, toPrefix); err != nil {
			return result, fmt.Errorf("prefix applied; persist .agents/loaf.json failed: %w", err)
		}
		result.ConfigWritten = true
	}
	return result, nil
}

func alignLeakedIssuePrefixTx(ctx context.Context, tx *sql.Tx, projectID, now string, dryRun bool) (IssuePrefixAlignItem, error) {
	identity, err := loadIssueIdentityTx(ctx, tx, projectID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return IssuePrefixAlignItem{}, nil
		}
		return IssuePrefixAlignItem{}, err
	}
	if identity.Prefix != DefaultIssuePrefix {
		return IssuePrefixAlignItem{}, nil
	}
	var name, path string
	if err := tx.QueryRowContext(ctx, `
SELECT COALESCE(friendly_name, ''), COALESCE(current_path, '')
FROM projects WHERE id = ?
`, projectID).Scan(&name, &path); err != nil {
		return IssuePrefixAlignItem{}, fmt.Errorf("read project slug for prefix align: %w", err)
	}
	if cfg, err := LoadIssueProjectConfig(path); err == nil && cfg.Prefix != "" {
		return IssuePrefixAlignItem{}, nil
	}
	derived := DeriveIssuePrefix(name, path)
	if derived == "" || derived == DefaultIssuePrefix {
		return IssuePrefixAlignItem{}, nil
	}
	return rewriteIssuePrefixTx(ctx, tx, projectID, identity.Prefix, derived, now, dryRun)
}

func rewriteIssuePrefixTx(ctx context.Context, tx *sql.Tx, projectID, fromPrefix, toPrefix, now string, dryRun bool) (IssuePrefixAlignItem, error) {
	if fromPrefix == toPrefix {
		return IssuePrefixAlignItem{}, nil
	}
	var name string
	if err := tx.QueryRowContext(ctx, `
SELECT COALESCE(friendly_name, '') FROM projects WHERE id = ?
`, projectID).Scan(&name); err != nil {
		return IssuePrefixAlignItem{}, fmt.Errorf("read project name for prefix rewrite: %w", err)
	}
	found, err := loadIssueAliasesWithPrefixTx(ctx, tx, projectID, fromPrefix)
	if err != nil {
		return IssuePrefixAlignItem{}, err
	}
	mapping := map[string]string{}
	for old := range found {
		mapping[old] = toPrefix + old[len(fromPrefix):]
	}
	if err := rejectPrefixAliasCollisionsTx(ctx, tx, projectID, mapping); err != nil {
		return IssuePrefixAlignItem{}, err
	}
	notes, err := countPrefixNotesTx(ctx, tx, projectID, mapping)
	if err != nil {
		return IssuePrefixAlignItem{}, err
	}
	item := IssuePrefixAlignItem{
		ProjectID:   projectID,
		ProjectName: name,
		FromPrefix:  fromPrefix,
		ToPrefix:    toPrefix,
		Aliases:     len(mapping),
		Notes:       notes,
	}
	if dryRun {
		return item, nil
	}
	if err := rewriteIssueAliasesTx(ctx, tx, projectID, mapping, now); err != nil {
		return IssuePrefixAlignItem{}, err
	}
	if err := rewritePrefixNotesTx(ctx, tx, projectID, mapping); err != nil {
		return IssuePrefixAlignItem{}, err
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE issue_identity SET prefix = ?, updated_at = ? WHERE project_id = ?
`, toPrefix, now, projectID); err != nil {
		return IssuePrefixAlignItem{}, &IssueTransactionError{Stage: "prefix rewrite identity", Err: err}
	}
	return item, nil
}

func rejectPrefixAliasCollisionsTx(ctx context.Context, tx *sql.Tx, projectID string, mapping map[string]string) error {
	for from, to := range mapping {
		if from == to {
			continue
		}
		var existing string
		err := tx.QueryRowContext(ctx, `
SELECT alias FROM aliases
WHERE project_id = ? AND entity_kind = 'issue' AND namespace = 'issue' AND alias = ?
`, projectID, to).Scan(&existing)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return fmt.Errorf("check prefix alias collision: %w", err)
		}
		return &IssueValidationError{Field: "prefix", Err: fmt.Errorf("alias %s already exists", to)}
	}
	return nil
}

func loadIssueAliasesWithPrefixTx(ctx context.Context, tx *sql.Tx, projectID, prefix string) (map[string]struct{}, error) {
	rows, err := tx.QueryContext(ctx, `
SELECT alias FROM aliases
WHERE project_id = ? AND entity_kind = 'issue' AND namespace = 'issue'
`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list issue aliases for prefix align: %w", err)
	}
	defer rows.Close()
	re := leakedIssueAliasRegexp(prefix)
	found := map[string]struct{}{}
	for rows.Next() {
		var alias string
		if err := rows.Scan(&alias); err != nil {
			return nil, fmt.Errorf("scan issue alias for prefix align: %w", err)
		}
		if re.MatchString(alias) {
			found[alias] = struct{}{}
		}
	}
	return found, rows.Err()
}

func leakedIssueAliasRegexp(prefix string) *regexp.Regexp {
	return regexp.MustCompile(`^` + regexp.QuoteMeta(prefix) + `-[0-9]+$`)
}

func rewriteIssueAliasesTx(ctx context.Context, tx *sql.Tx, projectID string, mapping map[string]string, now string) error {
	for from, to := range mapping {
		if _, err := tx.ExecContext(ctx, `
UPDATE aliases SET alias = ?, updated_at = ?
WHERE project_id = ? AND entity_kind = 'issue' AND namespace = 'issue' AND alias = ?
`, to, now, projectID, from); err != nil {
			return &IssueTransactionError{Stage: "prefix align alias", Err: err}
		}
	}
	return nil
}

func prefixNoteTables() []struct {
	table  string
	column string
} {
	return []struct {
		table  string
		column string
	}{
		{"events", "note"},
		{"intent_dispositions", "reason"},
	}
}

func countPrefixNotesTx(ctx context.Context, tx *sql.Tx, projectID string, mapping map[string]string) (int, error) {
	count := 0
	for _, spec := range prefixNoteTables() {
		rows, err := tx.QueryContext(ctx, `SELECT `+spec.column+` FROM `+spec.table+` WHERE project_id = ?`, projectID)
		if err != nil {
			return 0, fmt.Errorf("list %s for prefix align: %w", spec.table, err)
		}
		for rows.Next() {
			var value sql.NullString
			if err := rows.Scan(&value); err != nil {
				rows.Close()
				return 0, fmt.Errorf("scan %s for prefix align: %w", spec.table, err)
			}
			if rewritePrefixTokens(value.String, mapping) != value.String {
				count++
			}
		}
		err = rows.Err()
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
		if err != nil {
			return 0, err
		}
	}
	return count, nil
}

func rewritePrefixNotesTx(ctx context.Context, tx *sql.Tx, projectID string, mapping map[string]string) error {
	for _, spec := range prefixNoteTables() {
		rows, err := tx.QueryContext(ctx, `SELECT rowid, `+spec.column+` FROM `+spec.table+` WHERE project_id = ?`, projectID)
		if err != nil {
			return fmt.Errorf("list %s for prefix rewrite: %w", spec.table, err)
		}
		type change struct {
			rowid int64
			value string
		}
		changes := []change{}
		for rows.Next() {
			var rowid int64
			var value sql.NullString
			if err := rows.Scan(&rowid, &value); err != nil {
				rows.Close()
				return fmt.Errorf("scan %s for prefix rewrite: %w", spec.table, err)
			}
			next := rewritePrefixTokens(value.String, mapping)
			if next != value.String {
				changes = append(changes, change{rowid: rowid, value: next})
			}
		}
		err = rows.Err()
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
		if err != nil {
			return err
		}
		for _, item := range changes {
			if _, err := tx.ExecContext(ctx, `UPDATE `+spec.table+` SET `+spec.column+` = ? WHERE rowid = ?`, item.value, item.rowid); err != nil {
				return &IssueTransactionError{Stage: "prefix align " + spec.table, Err: err}
			}
		}
	}
	return nil
}

func rewritePrefixTokens(value string, mapping map[string]string) string {
	if value == "" || len(mapping) == 0 {
		return value
	}
	keys := make([]string, 0, len(mapping))
	for from := range mapping {
		keys = append(keys, from)
	}
	sort.Slice(keys, func(i, j int) bool {
		return len(keys[i]) > len(keys[j])
	})
	next := value
	for _, from := range keys {
		next = strings.ReplaceAll(next, from, mapping[from])
	}
	return next
}
