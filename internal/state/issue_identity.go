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
	IssueAuthorityLocal  = "local"
	IssueAuthorityLinear = "linear"
	IssueAuthorityGitHub = "github"

	DefaultIssueAuthority = IssueAuthorityLocal
	// DefaultIssuePrefix is the fallback when a project slug cannot form a
	// valid prefix. A project named "loaf" also derives LOAF. Other projects
	// must not materialize this default.
	DefaultIssuePrefix = "LOAF"

	IssuePrefixLeakCode        = "issue-prefix-default-leak"
	IssuePrefixAlignCommand    = "loaf issue identity --align --dry-run"
	IssuePrefixAlignAllCommand = "loaf issue identity --align --all --dry-run"
)

// IssueIdentity is the per-project authority and local-number counter.
type IssueIdentity struct {
	Authority  string `json:"authority"`
	Prefix     string `json:"prefix"`
	NextNumber int    `json:"next_number"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`
}

// IssueIdentityOptions configures a project's issue authority.
// Prefix applies to local minting; tracker authorities mint no alias.
// NextNumber is never set by callers — the stored counter only advances.
type IssueIdentityOptions struct {
	Authority string
	Prefix    string
}

// LookupIssueIdentity returns the stored identity row without inserting a
// default. ok is false when the project has no identity row yet.
func LookupIssueIdentity(ctx context.Context, root project.Root, resolver PathResolver) (IssueIdentity, bool, error) {
	store, err := openProjectStoreReadExisting(ctx, root, resolver)
	if err != nil {
		return IssueIdentity{}, false, err
	}
	defer store.Close()
	return store.LookupIssueIdentity(ctx, root)
}

// LookupIssueIdentity returns the stored identity row on an open store
// without inserting a default.
func (s *Store) LookupIssueIdentity(ctx context.Context, root project.Root) (IssueIdentity, bool, error) {
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return IssueIdentity{}, false, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return IssueIdentity{}, false, &IssueTransactionError{Stage: "begin", Err: err}
	}
	defer tx.Rollback()
	identity, err := loadIssueIdentityTx(ctx, tx, projectID)
	if errors.Is(err, sql.ErrNoRows) {
		return IssueIdentity{}, false, nil
	}
	if err != nil {
		return IssueIdentity{}, false, err
	}
	return identity, true, nil
}

// GetIssueIdentity returns the project's authority row, materializing a
// local identity from the project slug when none has been written yet.
func GetIssueIdentity(ctx context.Context, root project.Root, resolver PathResolver) (IssueIdentity, error) {
	store, err := openProjectStoreMutateExisting(ctx, root, resolver)
	if err != nil {
		return IssueIdentity{}, err
	}
	defer store.Close()
	return store.GetIssueIdentity(ctx, root)
}

// GetIssueIdentity returns the project's authority row on an open store.
func (s *Store) GetIssueIdentity(ctx context.Context, root project.Root) (IssueIdentity, error) {
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return IssueIdentity{}, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return IssueIdentity{}, &IssueTransactionError{Stage: "begin", Err: err}
	}
	defer tx.Rollback()
	identity, err := ensureIssueIdentityTx(ctx, tx, projectID, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return IssueIdentity{}, err
	}
	if err := tx.Commit(); err != nil {
		return IssueIdentity{}, &IssueTransactionError{Stage: "commit", Err: err}
	}
	return identity, nil
}

// SetIssueIdentity writes the project's authority. Switching away from local
// does not rewind next_number; minted numbers stay reserved.
func SetIssueIdentity(ctx context.Context, root project.Root, resolver PathResolver, options IssueIdentityOptions) (IssueIdentity, error) {
	store, err := openProjectStoreMutateExisting(ctx, root, resolver)
	if err != nil {
		return IssueIdentity{}, err
	}
	defer store.Close()
	return store.SetIssueIdentity(ctx, root, options)
}

// SetIssueIdentity writes the project's authority on an open store.
func (s *Store) SetIssueIdentity(ctx context.Context, root project.Root, options IssueIdentityOptions) (IssueIdentity, error) {
	authority, prefix, err := normalizeIssueIdentity(options)
	if err != nil {
		return IssueIdentity{}, err
	}
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return IssueIdentity{}, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return IssueIdentity{}, &IssueTransactionError{Stage: "begin", Err: err}
	}
	defer tx.Rollback()

	now := time.Now().UTC().Format(time.RFC3339Nano)
	if prefix == "" {
		derived, deriveErr := derivedIssuePrefixTx(ctx, tx, projectID)
		if deriveErr != nil {
			return IssueIdentity{}, deriveErr
		}
		prefix = derived
	}
	_, err = loadIssueIdentityTx(ctx, tx, projectID)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		id, idErr := newOpaqueStateID("iid")
		if idErr != nil {
			return IssueIdentity{}, &IssueTransactionError{Stage: "id", Err: idErr}
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO issue_identity (id, project_id, authority, prefix, next_number, created_at, updated_at)
VALUES (?, ?, ?, ?, 1, ?, ?)
`, id, projectID, authority, prefix, now, now); err != nil {
			return IssueIdentity{}, &IssueTransactionError{Stage: "identity", Err: err}
		}
	case err != nil:
		return IssueIdentity{}, err
	default:
		if _, err := tx.ExecContext(ctx, `
UPDATE issue_identity SET authority = ?, prefix = ?, updated_at = ? WHERE project_id = ?
`, authority, prefix, now, projectID); err != nil {
			return IssueIdentity{}, &IssueTransactionError{Stage: "identity", Err: err}
		}
	}

	identity, err := loadIssueIdentityTx(ctx, tx, projectID)
	if err != nil {
		return IssueIdentity{}, &IssueTransactionError{Stage: "read result", Err: err}
	}
	if err := tx.Commit(); err != nil {
		return IssueIdentity{}, &IssueTransactionError{Stage: "commit", Err: err}
	}
	return identity, nil
}

func normalizeIssueAuthorityValue(authority string) (string, error) {
	authority = strings.ToLower(strings.TrimSpace(authority))
	if authority == "" {
		return "", &IssueValidationError{Field: "authority", Err: fmt.Errorf("must be local, linear, or github")}
	}
	if authority != IssueAuthorityLocal && authority != IssueAuthorityLinear && authority != IssueAuthorityGitHub {
		return "", &IssueValidationError{Field: "authority", Err: fmt.Errorf("must be local, linear, or github")}
	}
	return authority, nil
}

func normalizeIssueIdentity(options IssueIdentityOptions) (string, string, error) {
	authority := strings.TrimSpace(options.Authority)
	if authority == "" {
		authority = DefaultIssueAuthority
	} else {
		normalized, err := normalizeIssueAuthorityValue(authority)
		if err != nil {
			return "", "", err
		}
		authority = normalized
	}
	prefix := strings.TrimSpace(options.Prefix)
	if prefix != "" {
		normalized, err := normalizeStoredIssuePrefix(prefix)
		if err != nil {
			return "", "", err
		}
		prefix = normalized
	}
	return authority, prefix, nil
}

func normalizeStoredIssuePrefix(prefix string) (string, error) {
	prefix = strings.ToUpper(strings.TrimSpace(prefix))
	if err := validateIssuePrefix(prefix); err != nil {
		return "", err
	}
	return prefix, nil
}

func validateIssuePrefix(prefix string) error {
	if prefix == "" {
		return &IssueValidationError{Field: "prefix", Err: fmt.Errorf("must be nonempty")}
	}
	for i := 0; i < len(prefix); i++ {
		c := prefix[i]
		if i == 0 && !isASCIILetter(c) {
			return &IssueValidationError{Field: "prefix", Err: fmt.Errorf("must start with a letter")}
		}
		if !isASCIILetter(c) && !isASCIIDigit(c) {
			return &IssueValidationError{Field: "prefix", Err: fmt.Errorf("must be alphanumeric")}
		}
	}
	return nil
}

func isASCIILetter(c byte) bool {
	return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')
}

func isASCIIDigit(c byte) bool {
	return c >= '0' && c <= '9'
}

func ensureIssueIdentityTx(ctx context.Context, tx *sql.Tx, projectID, now string) (IssueIdentity, error) {
	identity, err := loadIssueIdentityTx(ctx, tx, projectID)
	if err == nil {
		return identity, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return IssueIdentity{}, err
	}
	id, err := newOpaqueStateID("iid")
	if err != nil {
		return IssueIdentity{}, &IssueTransactionError{Stage: "id", Err: err}
	}
	prefix, authority, err := resolveIssueIdentityDefaultsTx(ctx, tx, projectID)
	if err != nil {
		return IssueIdentity{}, err
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO issue_identity (id, project_id, authority, prefix, next_number, created_at, updated_at)
VALUES (?, ?, ?, ?, 1, ?, ?)
`, id, projectID, authority, prefix, now, now); err != nil {
		return IssueIdentity{}, &IssueTransactionError{Stage: "identity", Err: err}
	}
	return loadIssueIdentityTx(ctx, tx, projectID)
}

func loadIssueIdentityTx(ctx context.Context, tx *sql.Tx, projectID string) (IssueIdentity, error) {
	var identity IssueIdentity
	err := tx.QueryRowContext(ctx, `
SELECT authority, prefix, next_number, created_at, updated_at
FROM issue_identity WHERE project_id = ?
`, projectID).Scan(&identity.Authority, &identity.Prefix, &identity.NextNumber, &identity.CreatedAt, &identity.UpdatedAt)
	if err != nil {
		return IssueIdentity{}, err
	}
	return identity, nil
}

// mintLocalIssueAliasTx consumes next_number when authority is local.
// Tracker authorities return an empty alias and leave the counter untouched.
func mintLocalIssueAliasTx(ctx context.Context, tx *sql.Tx, projectID, now string) (string, error) {
	identity, err := ensureIssueIdentityTx(ctx, tx, projectID, now)
	if err != nil {
		return "", err
	}
	if identity.Authority != IssueAuthorityLocal {
		return "", nil
	}
	alias := fmt.Sprintf("%s-%d", identity.Prefix, identity.NextNumber)
	if _, err := tx.ExecContext(ctx, `
UPDATE issue_identity SET next_number = next_number + 1, updated_at = ? WHERE project_id = ?
`, now, projectID); err != nil {
		return "", &IssueTransactionError{Stage: "mint", Err: err}
	}
	return alias, nil
}
