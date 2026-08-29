package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/levifig/loaf/vnext/sync/relay"
)

const sqliteDriverName = "sqlite3"

type Store struct {
	mu         sync.RWMutex
	db         *sql.DB
	verifier   relay.Verifier
	generation relay.RelayGeneration
	now        func() time.Time
	closed     bool
}

type databasePathIdentity struct {
	directory os.FileInfo
	database  os.FileInfo
}

func Open(databasePath string, verifier relay.Verifier) (*Store, error) {
	if verifier == nil {
		return nil, fmt.Errorf("%w: verifier is required", relay.ErrInvalidArgument)
	}
	identity, err := prepareDatabasePath(databasePath)
	if err != nil {
		return nil, err
	}
	if err := secureSQLiteSidecars(databasePath); err != nil {
		return nil, err
	}
	db, err := sql.Open(sqliteDriverName, databaseDSN(databasePath))
	if err != nil {
		return nil, fmt.Errorf("open relay database: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	closeOnError := func(openErr error) (*Store, error) {
		if closeErr := db.Close(); closeErr != nil {
			return nil, fmt.Errorf("%w (close relay database: %v)", openErr, closeErr)
		}
		return nil, openErr
	}
	if err := db.PingContext(context.Background()); err != nil {
		return closeOnError(fmt.Errorf("ping relay database: %w", err))
	}
	after, err := inspectPrivateDatabaseFile(databasePath)
	if err != nil {
		return closeOnError(err)
	}
	if !os.SameFile(identity.database, after) {
		return closeOnError(fmt.Errorf("relay database changed while opening"))
	}
	if err := initializeSchemaIfEmpty(db); err != nil {
		return closeOnError(err)
	}
	if err := validateSchema(db); err != nil {
		return closeOnError(err)
	}
	generation, err := readRelayGeneration(db)
	if err != nil {
		return closeOnError(err)
	}
	if err := verifySQLiteFiles(databasePath); err != nil {
		return closeOnError(err)
	}
	if err := verifyDatabasePathIdentity(databasePath, identity); err != nil {
		return closeOnError(err)
	}
	return &Store{
		db:         db,
		verifier:   verifier,
		generation: generation,
		now:        time.Now,
	}, nil
}

func (store *Store) RelayGeneration() relay.RelayGeneration {
	if store == nil {
		return relay.RelayGeneration{}
	}
	return store.generation
}

func (store *Store) Close() error {
	if store == nil {
		return nil
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.closed || store.db == nil {
		return nil
	}
	store.closed = true
	if err := store.db.Close(); err != nil {
		return fmt.Errorf("close relay database: %w", err)
	}
	return nil
}

func prepareDatabasePath(path string) (databasePathIdentity, error) {
	if path == "" || !filepath.IsAbs(path) || filepath.Clean(path) != path || filepath.Dir(path) == path {
		return databasePathIdentity{}, fmt.Errorf("%w: relay database path must be absolute, clean, and non-root", relay.ErrInvalidArgument)
	}
	parent := filepath.Dir(path)
	if err := validateDatabaseDirectoryPath(parent); err != nil {
		return databasePathIdentity{}, err
	}
	parentInfo, err := os.Lstat(parent)
	if err != nil {
		return databasePathIdentity{}, fmt.Errorf("inspect relay database directory: %w", err)
	}
	if parentInfo.Mode()&os.ModeSymlink != 0 || !parentInfo.IsDir() {
		return databasePathIdentity{}, fmt.Errorf("relay database directory must be a real directory")
	}
	if err := validatePrivateDirectoryMode(parentInfo); err != nil {
		return databasePathIdentity{}, err
	}
	_, err = os.Lstat(path)
	if err == nil {
		databaseInfo, err := inspectPrivateDatabaseFile(path)
		return databasePathIdentity{directory: parentInfo, database: databaseInfo}, err
	}
	if !errors.Is(err, os.ErrNotExist) {
		return databasePathIdentity{}, fmt.Errorf("inspect relay database: %w", err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			databaseInfo, inspectErr := inspectPrivateDatabaseFile(path)
			return databasePathIdentity{directory: parentInfo, database: databaseInfo}, inspectErr
		}
		return databasePathIdentity{}, fmt.Errorf("create relay database: %w", err)
	}
	if err := file.Close(); err != nil {
		return databasePathIdentity{}, fmt.Errorf("close new relay database: %w", err)
	}
	databaseInfo, err := inspectPrivateDatabaseFile(path)
	return databasePathIdentity{directory: parentInfo, database: databaseInfo}, err
}

func verifyDatabasePathIdentity(path string, want databasePathIdentity) error {
	if err := validateDatabaseDirectoryPath(filepath.Dir(path)); err != nil {
		return err
	}
	directory, err := os.Lstat(filepath.Dir(path))
	if err != nil {
		return fmt.Errorf("reinspect relay database directory: %w", err)
	}
	database, err := inspectPrivateDatabaseFile(path)
	if err != nil {
		return err
	}
	if !os.SameFile(want.directory, directory) || !os.SameFile(want.database, database) {
		return fmt.Errorf("relay database path identity changed while opening")
	}
	return nil
}

func inspectPrivateDatabaseFile(path string) (os.FileInfo, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("inspect relay database: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("relay database must be a real file")
	}
	if err := validatePrivateFileMode(path, info); err != nil {
		return nil, err
	}
	return info, nil
}

func secureSQLiteSidecars(databasePath string) error {
	for _, suffix := range []string{"-wal", "-shm", "-journal"} {
		path := databasePath + suffix
		before, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return fmt.Errorf("inspect relay SQLite sidecar: %w", err)
		}
		if before.Mode()&os.ModeSymlink != 0 || !before.Mode().IsRegular() {
			return fmt.Errorf("relay SQLite sidecar %s must be a real file", filepath.Base(path))
		}
		secured, err := securePrivateFileMode(path, before)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return fmt.Errorf("secure relay SQLite sidecar %s: %w", filepath.Base(path), err)
		}
		if secured.Mode()&os.ModeSymlink != 0 || !secured.Mode().IsRegular() || !os.SameFile(before, secured) {
			return fmt.Errorf("relay SQLite sidecar %s changed while securing permissions", filepath.Base(path))
		}
	}
	return nil
}

func verifySQLiteFiles(databasePath string) error {
	if _, err := inspectPrivateDatabaseFile(databasePath); err != nil {
		return err
	}
	return secureSQLiteSidecars(databasePath)
}

func validateContext(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("%w: context is nil", relay.ErrInvalidArgument)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}

func databaseDSN(path string) string {
	values := url.Values{}
	values.Set("mode", "rwc")
	values.Set("_txlock", "immediate")
	values.Add("_pragma", "busy_timeout(5000)")
	values.Add("_pragma", "foreign_keys(on)")
	values.Add("_pragma", "journal_mode(wal)")
	values.Add("_pragma", "synchronous(full)")
	values.Add("_pragma", "trusted_schema(off)")
	urlPath := filepath.ToSlash(path)
	if filepath.VolumeName(path) != "" && !strings.HasPrefix(urlPath, "/") {
		urlPath = "/" + urlPath
	}
	return (&url.URL{Scheme: "file", Path: urlPath, RawQuery: values.Encode()}).String()
}

func (store *Store) readLock() error {
	store.mu.RLock()
	if store.closed || store.db == nil {
		store.mu.RUnlock()
		return relay.ErrClosed
	}
	return nil
}

func (store *Store) readUnlock() {
	store.mu.RUnlock()
}

func transact(ctx context.Context, db *sql.DB, action func(*sql.Tx) error) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin relay transaction: %w", err)
	}
	if err := action(tx); err != nil {
		if rollbackErr := tx.Rollback(); rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
			return errors.Join(err, fmt.Errorf("rollback relay transaction: %w", rollbackErr))
		}
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit relay transaction: %w", err)
	}
	return nil
}

type channelRecord struct {
	authority            relay.ChannelAuthority
	ownerTokenHash       relay.TokenHash
	membershipGeneration uint32
	createdAtMillis      int64
}

type environmentRecord struct {
	authority         relay.EnvironmentAuthority
	tokenHash         relay.TokenHash
	currentMembership uint32
	retiredAtMillis   sql.NullInt64
}

func authenticateOwner(ctx context.Context, tx *sql.Tx, authorization relay.OwnerAuthorization) (channelRecord, error) {
	if err := authorization.Validate(); err != nil {
		return channelRecord{}, err
	}
	var record channelRecord
	var channelID, generation, adminPublicKey, storedHash []byte
	err := tx.QueryRowContext(ctx, `
SELECT c.channel_id, i.relay_generation, c.admin_public_key, c.owner_token_hash,
       c.membership_generation, c.created_at_millis
FROM relay_channels AS c
CROSS JOIN relay_instance AS i
WHERE channel_id = ? AND owner_token_id = ?`,
		authorization.ChannelID[:], authorization.TokenID[:],
	).Scan(
		&channelID,
		&generation,
		&adminPublicKey,
		&storedHash,
		&record.membershipGeneration,
		&record.createdAtMillis,
	)
	if errors.Is(err, sql.ErrNoRows) {
		verifyDummySecret(authorization.TokenSecret)
		return channelRecord{}, relay.ErrUnauthenticated
	}
	if err != nil {
		return channelRecord{}, fmt.Errorf("read owner authorization: %w", err)
	}
	if !scanFixed(record.authority.ChannelID[:], channelID) || !scanFixed(record.authority.RelayGeneration[:], generation) ||
		!scanFixed(record.authority.AdminPublicKey[:], adminPublicKey) || !scanFixed(record.ownerTokenHash[:], storedHash) {
		return channelRecord{}, fmt.Errorf("relay owner authorization record is corrupt")
	}
	if !relay.VerifyTokenSecret(record.ownerTokenHash, authorization.TokenSecret) {
		return channelRecord{}, relay.ErrUnauthenticated
	}
	if record.authority.RelayGeneration != authorization.RelayGeneration {
		return channelRecord{}, relay.ErrGenerationMismatch
	}
	return record, nil
}

func authenticateEnvironment(ctx context.Context, tx *sql.Tx, authorization relay.EnvironmentAuthorization) (environmentRecord, error) {
	if err := authorization.Validate(); err != nil {
		return environmentRecord{}, err
	}
	var record environmentRecord
	var channelID, generation, adminPublicKey, certificateID, certificateBytes, storedHash []byte
	var mode string
	err := tx.QueryRowContext(ctx, `
SELECT c.channel_id, i.relay_generation, c.admin_public_key, c.membership_generation,
       e.environment_id, e.certificate_id, e.certificate_bytes, e.mode, e.expires_at_millis,
       e.token_expires_at_millis, e.membership_generation, e.token_hash, e.retired_at_millis
FROM relay_channels AS c
CROSS JOIN relay_instance AS i
JOIN relay_environments AS e ON e.channel_id = c.channel_id
WHERE c.channel_id = ? AND e.environment_id = ? AND e.token_id = ?`,
		authorization.ChannelID[:], string(authorization.EnvironmentID), authorization.TokenID[:],
	).Scan(
		&channelID,
		&generation,
		&adminPublicKey,
		&record.currentMembership,
		&record.authority.EnvironmentID,
		&certificateID,
		&certificateBytes,
		&mode,
		&record.authority.ExpiresAtMillis,
		&record.authority.RelayTokenExpiresAtMillis,
		&record.authority.MembershipGeneration,
		&storedHash,
		&record.retiredAtMillis,
	)
	if errors.Is(err, sql.ErrNoRows) {
		verifyDummySecret(authorization.TokenSecret)
		return environmentRecord{}, relay.ErrUnauthenticated
	}
	if err != nil {
		return environmentRecord{}, fmt.Errorf("read environment authorization: %w", err)
	}
	if !scanFixed(record.authority.ChannelID[:], channelID) || !scanFixed(record.authority.RelayGeneration[:], generation) ||
		!scanFixed(record.authority.AdminPublicKey[:], adminPublicKey) || !scanFixed(record.authority.CertificateID[:], certificateID) ||
		!scanFixed(record.tokenHash[:], storedHash) {
		return environmentRecord{}, fmt.Errorf("relay environment authorization record is corrupt")
	}
	record.authority.CertificateBytes = append([]byte(nil), certificateBytes...)
	record.authority.Mode = relay.EnvironmentMode(mode)
	if !relay.VerifyTokenSecret(record.tokenHash, authorization.TokenSecret) {
		return environmentRecord{}, relay.ErrUnauthenticated
	}
	if record.authority.RelayGeneration != authorization.RelayGeneration {
		return environmentRecord{}, relay.ErrGenerationMismatch
	}
	if record.authority.CertificateID != authorization.CertificateID {
		return environmentRecord{}, relay.ErrUnauthenticated
	}
	return record, nil
}

func requireActiveEnvironment(record environmentRecord, nowMillis int64) error {
	if record.retiredAtMillis.Valid {
		return relay.ErrRetired
	}
	if record.authority.ExpiresAtMillis != 0 && nowMillis >= record.authority.ExpiresAtMillis {
		return relay.ErrExpired
	}
	if record.authority.RelayTokenExpiresAtMillis != 0 && nowMillis >= record.authority.RelayTokenExpiresAtMillis {
		return relay.ErrExpired
	}
	return nil
}

func verifyDummySecret(secret relay.RelayTokenSecret) {
	var dummy relay.TokenHash
	relay.VerifyTokenSecret(dummy, secret)
}

func scanFixed(destination, source []byte) bool {
	if len(destination) != len(source) {
		return false
	}
	copy(destination, source)
	return true
}

func channelHead(ctx context.Context, tx *sql.Tx, channelID relay.ChannelID) (int64, error) {
	var head int64
	if err := tx.QueryRowContext(ctx, `
SELECT COALESCE(MAX(arrival_sequence), 0)
FROM relay_arrivals
WHERE channel_id = ?`, channelID[:]).Scan(&head); err != nil {
		return 0, fmt.Errorf("read relay head: %w", err)
	}
	return head, nil
}

func producerHead(ctx context.Context, tx *sql.Tx, channelID relay.ChannelID, environmentID relay.EnvironmentID) (int64, error) {
	var head int64
	if err := tx.QueryRowContext(ctx, `
SELECT COALESCE(MAX(environment_sequence), 0)
FROM relay_arrivals
WHERE channel_id = ? AND environment_id = ?`, channelID[:], string(environmentID)).Scan(&head); err != nil {
		return 0, fmt.Errorf("read relay producer head: %w", err)
	}
	return head, nil
}

func producerFence(ctx context.Context, tx *sql.Tx, channelID relay.ChannelID, environmentID relay.EnvironmentID) (int64, relay.Digest, error) {
	head, err := producerHead(ctx, tx, channelID, environmentID)
	if err != nil || head == 0 {
		return head, relay.Digest{}, err
	}
	var encodedDigest []byte
	if err := tx.QueryRowContext(ctx, `
SELECT envelope_digest
FROM relay_arrivals
WHERE channel_id = ? AND environment_id = ? AND environment_sequence = ?`,
		channelID[:], string(environmentID), head,
	).Scan(&encodedDigest); err != nil {
		return 0, relay.Digest{}, fmt.Errorf("read relay producer fence: %w", err)
	}
	var digest relay.Digest
	if !scanFixed(digest[:], encodedDigest) {
		return 0, relay.Digest{}, fmt.Errorf("relay producer fence contains an invalid digest")
	}
	return head, digest, nil
}

func pruneHead(ctx context.Context, tx *sql.Tx, channelID relay.ChannelID) (int64, error) {
	var head int64
	if err := tx.QueryRowContext(ctx, `
SELECT COALESCE(MAX(prune_sequence), 0)
FROM relay_prune_certificates
WHERE channel_id = ?`, channelID[:]).Scan(&head); err != nil {
		return 0, fmt.Errorf("read relay prune head: %w", err)
	}
	return head, nil
}

func millisTime(value int64) time.Time {
	return time.UnixMilli(value).UTC()
}
