package syncserver

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/ncruces/go-sqlite3/driver"
)

const sqliteDriverName = "sqlite3"

const schemaSQL = `
CREATE TABLE IF NOT EXISTS sync_accounts (
  access_key_id TEXT PRIMARY KEY,
  secret_hash BLOB NOT NULL,
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS connection_tokens (
  token_id TEXT PRIMARY KEY,
  account_access_key_id TEXT NOT NULL REFERENCES sync_accounts(access_key_id),
  name TEXT NOT NULL,
  secret_hash BLOB NOT NULL,
  project_id TEXT NOT NULL,
  revoked_at TEXT,
  last_seen_at TEXT,
  created_at TEXT NOT NULL,
  UNIQUE(account_access_key_id, name)
);

CREATE TABLE IF NOT EXISTS project_arrival_cursors (
  project_id TEXT PRIMARY KEY,
  next_seq INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS fact_blobs (
  project_id TEXT NOT NULL,
  fact_id TEXT NOT NULL,
  arrival_seq INTEGER NOT NULL,
  blob BLOB NOT NULL,
  created_at TEXT NOT NULL,
  PRIMARY KEY (project_id, fact_id)
);

CREATE INDEX IF NOT EXISTS idx_fact_blobs_arrival ON fact_blobs(project_id, arrival_seq);

CREATE TABLE IF NOT EXISTS scratchpad_messages (
  project_id TEXT NOT NULL,
  channel TEXT NOT NULL,
  seq INTEGER NOT NULL,
  payload BLOB NOT NULL,
  created_at TEXT NOT NULL,
  PRIMARY KEY (project_id, channel, seq)
);
`

type Store struct {
	db *sql.DB
}

type Account struct {
	AccessKeyID string    `json:"access_key_id"`
	CreatedAt   time.Time `json:"created_at"`
}

type ConnectionToken struct {
	TokenID    string     `json:"token_id"`
	Name       string     `json:"name"`
	ProjectID  string     `json:"project_id"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
	LastSeenAt *time.Time `json:"last_seen_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

type FactBlob struct {
	ProjectID  string    `json:"project_id"`
	FactID     string    `json:"fact_id"`
	ArrivalSeq int64     `json:"arrival_seq"`
	Blob       []byte    `json:"blob"`
	CreatedAt  time.Time `json:"created_at"`
}

type PushInput struct {
	FactID string
	Blob   []byte
}

type PushResult struct {
	FactID     string
	ArrivalSeq int64
	Duplicate  bool
}

func OpenStore(path string) (*Store, error) {
	db, err := sql.Open(sqliteDriverName, sqliteDSN(path))
	if err != nil {
		return nil, fmt.Errorf("open sync server database: %w", err)
	}
	db.SetMaxOpenConns(1)
	store := &Store{db: db}
	if err := store.initSchema(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) initSchema() error {
	if _, err := s.db.Exec(schemaSQL); err != nil {
		return fmt.Errorf("init sync server schema: %w", err)
	}
	return nil
}

func hashSecret(secret string) []byte {
	sum := sha256.Sum256([]byte(secret))
	return sum[:]
}

func verifySecret(secret string, stored []byte) bool {
	got := hashSecret(secret)
	return subtleCompare(got, stored)
}

func subtleCompare(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	var v byte
	for i := range a {
		v |= a[i] ^ b[i]
	}
	return v == 0
}

func randomID(prefix string) (string, error) {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return prefix + hex.EncodeToString(buf[:]), nil
}

func nowUTC() time.Time {
	return time.Now().UTC().Truncate(time.Microsecond)
}

func formatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

func parseTime(raw string) (time.Time, error) {
	return time.Parse(time.RFC3339Nano, raw)
}

func normalizeProjectID(projectID string) (string, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return "", errors.New("project id is empty")
	}
	return projectID, nil
}

func normalizeFactID(factID string) (string, error) {
	factID = strings.TrimSpace(factID)
	if factID == "" {
		return "", errors.New("fact id is empty")
	}
	return factID, nil
}

func (s *Store) HasKeyMaterial(ctx context.Context) (bool, error) {
	return s.SchemaHasKeyMaterial(ctx)
}

func (s *Store) CreateAccountWithKey(ctx context.Context, accessKeyIDHint string) (Account, string, error) {
	accessSecret, err := randomID("sec_")
	if err != nil {
		return Account{}, "", fmt.Errorf("create account secret: %w", err)
	}
	accessKeyID := strings.TrimSpace(accessKeyIDHint)
	if accessKeyID == "" {
		accessKeyID, err = randomID("ak_")
		if err != nil {
			return Account{}, "", fmt.Errorf("create account access key: %w", err)
		}
	}
	createdAt := nowUTC()
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO sync_accounts(access_key_id, secret_hash, created_at) VALUES (?, ?, ?)`,
		accessKeyID, hashSecret(accessSecret), formatTime(createdAt),
	)
	if err != nil {
		return Account{}, "", fmt.Errorf("insert sync account: %w", err)
	}
	return Account{AccessKeyID: accessKeyID, CreatedAt: createdAt}, accessSecret, nil
}

func (s *Store) AuthenticateBearerToken(ctx context.Context, rawToken, projectID string) error {
	projectID, err := normalizeProjectID(projectID)
	if err != nil {
		return err
	}
	rawToken = strings.TrimSpace(rawToken)
	if rawToken == "" {
		return errors.New("invalid connection token")
	}
	var tokenID string
	var storedHash []byte
	var scopedProject string
	var revokedAt sql.NullString
	err = s.db.QueryRowContext(ctx, `
SELECT token_id, secret_hash, project_id, revoked_at
FROM connection_tokens
WHERE secret_hash = ?`, hashSecret(rawToken)).Scan(&tokenID, &storedHash, &scopedProject, &revokedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errors.New("invalid connection token")
		}
		return fmt.Errorf("lookup connection token: %w", err)
	}
	if revokedAt.Valid {
		return errors.New("connection token revoked")
	}
	if scopedProject != projectID {
		return errProjectScopeMismatch
	}
	_, err = s.db.ExecContext(ctx,
		`UPDATE connection_tokens SET last_seen_at = ? WHERE token_id = ?`,
		formatTime(nowUTC()), tokenID,
	)
	if err != nil {
		return fmt.Errorf("update connection token last seen: %w", err)
	}
	return nil
}

var errProjectScopeMismatch = errors.New("connection token project scope mismatch")

func (s *Store) CreateAccount(ctx context.Context, accessSecret string) (Account, string, error) {
	accessKeyID, err := randomID("ak_")
	if err != nil {
		return Account{}, "", fmt.Errorf("create account access key: %w", err)
	}
	createdAt := nowUTC()
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO sync_accounts(access_key_id, secret_hash, created_at) VALUES (?, ?, ?)`,
		accessKeyID, hashSecret(accessSecret), formatTime(createdAt),
	)
	if err != nil {
		return Account{}, "", fmt.Errorf("insert sync account: %w", err)
	}
	return Account{AccessKeyID: accessKeyID, CreatedAt: createdAt}, accessKeyID, nil
}

func (s *Store) MintConnectionToken(ctx context.Context, accessKeyID, accessSecret, name, projectID string) (ConnectionToken, string, error) {
	accessKeyID = strings.TrimSpace(accessKeyID)
	name = strings.TrimSpace(name)
	projectID, err := normalizeProjectID(projectID)
	if err != nil {
		return ConnectionToken{}, "", err
	}
	if name == "" {
		return ConnectionToken{}, "", errors.New("connection token name is empty")
	}
	if !s.verifyAccount(ctx, accessKeyID, accessSecret) {
		return ConnectionToken{}, "", errors.New("invalid account credentials")
	}
	tokenID, err := randomID("tok_")
	if err != nil {
		return ConnectionToken{}, "", fmt.Errorf("create connection token id: %w", err)
	}
	tokenSecret, err := randomID("sec_")
	if err != nil {
		return ConnectionToken{}, "", fmt.Errorf("create connection token secret: %w", err)
	}
	createdAt := nowUTC()
	_, err = s.db.ExecContext(ctx, `
INSERT INTO connection_tokens(token_id, account_access_key_id, name, secret_hash, project_id, created_at)
VALUES (?, ?, ?, ?, ?, ?)`,
		tokenID, accessKeyID, name, hashSecret(tokenSecret), projectID, formatTime(createdAt),
	)
	if err != nil {
		return ConnectionToken{}, "", fmt.Errorf("insert connection token: %w", err)
	}
	return ConnectionToken{
		TokenID:   tokenID,
		Name:      name,
		ProjectID: projectID,
		CreatedAt: createdAt,
	}, tokenSecret, nil
}

func (s *Store) verifyAccount(ctx context.Context, accessKeyID, accessSecret string) bool {
	var stored []byte
	err := s.db.QueryRowContext(ctx,
		`SELECT secret_hash FROM sync_accounts WHERE access_key_id = ?`,
		strings.TrimSpace(accessKeyID),
	).Scan(&stored)
	if err != nil {
		return false
	}
	return verifySecret(accessSecret, stored)
}

func (s *Store) AuthenticateConnectionToken(ctx context.Context, tokenID, tokenSecret, projectID string) error {
	projectID, err := normalizeProjectID(projectID)
	if err != nil {
		return err
	}
	var storedHash []byte
	var scopedProject string
	var revokedAt sql.NullString
	err = s.db.QueryRowContext(ctx, `
SELECT secret_hash, project_id, revoked_at
FROM connection_tokens
WHERE token_id = ?`, strings.TrimSpace(tokenID)).Scan(&storedHash, &scopedProject, &revokedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errors.New("invalid connection token")
		}
		return fmt.Errorf("lookup connection token: %w", err)
	}
	if revokedAt.Valid {
		return errors.New("connection token revoked")
	}
	if scopedProject != projectID {
		return errors.New("connection token project scope mismatch")
	}
	if !verifySecret(tokenSecret, storedHash) {
		return errors.New("invalid connection token")
	}
	_, err = s.db.ExecContext(ctx,
		`UPDATE connection_tokens SET last_seen_at = ? WHERE token_id = ?`,
		formatTime(nowUTC()), strings.TrimSpace(tokenID),
	)
	if err != nil {
		return fmt.Errorf("update connection token last seen: %w", err)
	}
	return nil
}

var (
	errInvalidAccountCredentials = errors.New("invalid account credentials")
	errAdminProjectForbidden     = errors.New("admin is not authorized for this project")
)

func (s *Store) AuthenticateAdmin(ctx context.Context, accessKeyID, accessSecret string) error {
	if !s.verifyAccount(ctx, accessKeyID, accessSecret) {
		return errInvalidAccountCredentials
	}
	return nil
}

func (s *Store) AuthenticateAdminForProject(ctx context.Context, accessKeyID, accessSecret, projectID string) error {
	if err := s.AuthenticateAdmin(ctx, accessKeyID, accessSecret); err != nil {
		return err
	}
	projectID, err := normalizeProjectID(projectID)
	if err != nil {
		return err
	}
	var n int
	err = s.db.QueryRowContext(ctx, `
SELECT COUNT(*) FROM connection_tokens
WHERE account_access_key_id = ? AND project_id = ?`,
		strings.TrimSpace(accessKeyID), projectID,
	).Scan(&n)
	if err != nil {
		return fmt.Errorf("lookup admin project binding: %w", err)
	}
	if n == 0 {
		return errAdminProjectForbidden
	}
	return nil
}

func (s *Store) PushFacts(ctx context.Context, projectID string, inputs []PushInput) ([]PushResult, error) {
	projectID, err := normalizeProjectID(projectID)
	if err != nil {
		return nil, err
	}
	if len(inputs) == 0 {
		return nil, nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin push transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	results := make([]PushResult, 0, len(inputs))
	for _, input := range inputs {
		factID, err := normalizeFactID(input.FactID)
		if err != nil {
			return nil, err
		}
		if len(input.Blob) == 0 {
			return nil, fmt.Errorf("fact %q blob is empty", factID)
		}
		var existingSeq int64
		err = tx.QueryRowContext(ctx,
			`SELECT arrival_seq FROM fact_blobs WHERE project_id = ? AND fact_id = ?`,
			projectID, factID,
		).Scan(&existingSeq)
		if err == nil {
			results = append(results, PushResult{FactID: factID, ArrivalSeq: existingSeq, Duplicate: true})
			continue
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("lookup existing fact blob: %w", err)
		}
		seq, err := reserveArrivalSeq(ctx, tx, projectID)
		if err != nil {
			return nil, err
		}
		createdAt := formatTime(nowUTC())
		if _, err := tx.ExecContext(ctx, `
INSERT INTO fact_blobs(project_id, fact_id, arrival_seq, blob, created_at)
VALUES (?, ?, ?, ?, ?)`,
			projectID, factID, seq, input.Blob, createdAt,
		); err != nil {
			return nil, fmt.Errorf("insert fact blob: %w", err)
		}
		results = append(results, PushResult{FactID: factID, ArrivalSeq: seq, Duplicate: false})
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit push transaction: %w", err)
	}
	return results, nil
}

func reserveArrivalSeq(ctx context.Context, tx *sql.Tx, projectID string) (int64, error) {
	var nextSeq int64
	err := tx.QueryRowContext(ctx,
		`SELECT next_seq FROM project_arrival_cursors WHERE project_id = ?`,
		projectID,
	).Scan(&nextSeq)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		nextSeq = 1
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO project_arrival_cursors(project_id, next_seq) VALUES (?, ?)`,
			projectID, int64(2),
		); err != nil {
			return 0, fmt.Errorf("seed arrival cursor: %w", err)
		}
		return nextSeq, nil
	case err != nil:
		return 0, fmt.Errorf("read arrival cursor: %w", err)
	default:
		if _, err := tx.ExecContext(ctx,
			`UPDATE project_arrival_cursors SET next_seq = ? WHERE project_id = ?`,
			nextSeq+1, projectID,
		); err != nil {
			return 0, fmt.Errorf("advance arrival cursor: %w", err)
		}
		return nextSeq, nil
	}
}

func (s *Store) PullFactsSince(ctx context.Context, projectID string, since int64) ([]FactBlob, int64, error) {
	projectID, err := normalizeProjectID(projectID)
	if err != nil {
		return nil, 0, err
	}
	if since < 0 {
		return nil, 0, errors.New("cursor must be non-negative")
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT fact_id, arrival_seq, blob, created_at
FROM fact_blobs
WHERE project_id = ? AND arrival_seq > ?
ORDER BY arrival_seq ASC`, projectID, since)
	if err != nil {
		return nil, 0, fmt.Errorf("query fact blobs: %w", err)
	}
	defer rows.Close()

	facts := make([]FactBlob, 0)
	var maxCursor int64 = since
	for rows.Next() {
		var fact FactBlob
		var createdAt string
		fact.ProjectID = projectID
		if err := rows.Scan(&fact.FactID, &fact.ArrivalSeq, &fact.Blob, &createdAt); err != nil {
			return nil, 0, fmt.Errorf("scan fact blob: %w", err)
		}
		fact.CreatedAt, err = parseTime(createdAt)
		if err != nil {
			return nil, 0, fmt.Errorf("parse fact created_at: %w", err)
		}
		if fact.ArrivalSeq > maxCursor {
			maxCursor = fact.ArrivalSeq
		}
		facts = append(facts, fact)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate fact blobs: %w", err)
	}
	return facts, maxCursor, nil
}

func (s *Store) DeleteFact(ctx context.Context, projectID, factID string) (bool, error) {
	projectID, err := normalizeProjectID(projectID)
	if err != nil {
		return false, err
	}
	factID, err = normalizeFactID(factID)
	if err != nil {
		return false, err
	}
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM fact_blobs WHERE project_id = ? AND fact_id = ?`,
		projectID, factID,
	)
	if err != nil {
		return false, fmt.Errorf("delete fact blob: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("delete fact rows affected: %w", err)
	}
	return n > 0, nil
}

func (s *Store) RevokeConnectionToken(ctx context.Context, accessKeyID, accessSecret, name string) (bool, error) {
	if !s.verifyAccount(ctx, accessKeyID, accessSecret) {
		return false, errors.New("invalid account credentials")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return false, errors.New("connection token name is empty")
	}
	res, err := s.db.ExecContext(ctx, `
UPDATE connection_tokens
SET revoked_at = ?
WHERE account_access_key_id = ? AND name = ? AND revoked_at IS NULL`,
		formatTime(nowUTC()), strings.TrimSpace(accessKeyID), name,
	)
	if err != nil {
		return false, fmt.Errorf("revoke connection token: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("revoke rows affected: %w", err)
	}
	return n > 0, nil
}

func (s *Store) ListConnectionTokens(ctx context.Context, accessKeyID, accessSecret string) ([]ConnectionToken, error) {
	if !s.verifyAccount(ctx, accessKeyID, accessSecret) {
		return nil, errors.New("invalid account credentials")
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT token_id, name, project_id, revoked_at, last_seen_at, created_at
FROM connection_tokens
WHERE account_access_key_id = ?
ORDER BY created_at ASC`, strings.TrimSpace(accessKeyID))
	if err != nil {
		return nil, fmt.Errorf("list connection tokens: %w", err)
	}
	defer rows.Close()

	tokens := make([]ConnectionToken, 0)
	for rows.Next() {
		var token ConnectionToken
		var revokedAt sql.NullString
		var lastSeenAt sql.NullString
		var createdAt string
		if err := rows.Scan(&token.TokenID, &token.Name, &token.ProjectID, &revokedAt, &lastSeenAt, &createdAt); err != nil {
			return nil, fmt.Errorf("scan connection token: %w", err)
		}
		if revokedAt.Valid {
			t, err := parseTime(revokedAt.String)
			if err != nil {
				return nil, fmt.Errorf("parse revoked_at: %w", err)
			}
			token.RevokedAt = &t
		}
		if lastSeenAt.Valid {
			t, err := parseTime(lastSeenAt.String)
			if err != nil {
				return nil, fmt.Errorf("parse last_seen_at: %w", err)
			}
			token.LastSeenAt = &t
		}
		token.CreatedAt, err = parseTime(createdAt)
		if err != nil {
			return nil, fmt.Errorf("parse created_at: %w", err)
		}
		tokens = append(tokens, token)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate connection tokens: %w", err)
	}
	return tokens, nil
}

func (s *Store) SchemaHasKeyMaterial(ctx context.Context) (bool, error) {
	rows, err := s.db.QueryContext(ctx, `PRAGMA table_info(sync_accounts)`)
	if err != nil {
		return false, fmt.Errorf("inspect sync_accounts schema: %w", err)
	}
	defer rows.Close()
	forbidden := map[string]struct{}{
		"master_key": {}, "project_key": {}, "account_access_secret": {},
	}
	for rows.Next() {
		var cid int
		var name, colType string
		var notNull, pk int
		var defaultValue sql.NullString
		if err := rows.Scan(&cid, &name, &colType, &notNull, &defaultValue, &pk); err != nil {
			return false, fmt.Errorf("scan sync_accounts column: %w", err)
		}
		if _, ok := forbidden[name]; ok {
			return true, nil
		}
	}
	return false, rows.Err()
}

func sqliteDSN(path string) string {
	values := url.Values{}
	values.Add("_pragma", "busy_timeout(5000)")
	values.Add("_pragma", "journal_mode(wal)")
	values.Add("_pragma", "synchronous(full)")
	values.Add("_pragma", "foreign_keys(on)")
	return (&url.URL{
		Scheme:   "file",
		Path:     filepath.ToSlash(path),
		RawQuery: values.Encode(),
	}).String()
}
