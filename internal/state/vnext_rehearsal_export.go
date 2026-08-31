package state

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/levifig/loaf/internal/project"
	"github.com/levifig/loaf/vnext/continuity"
	"github.com/levifig/loaf/vnext/migration/archive"
)

const vnextRehearsalIDDomainV1 = "loaf-vnext-rehearsal-v1"

// VNextRehearsalExportOptions selects one project from one explicit verified
// legacy backup and binds its live-database provenance to an authoritative
// project root and resolver.
type VNextRehearsalExportOptions struct {
	Backup    BackupResult
	ProjectID string
	Root      project.Root
	Resolver  PathResolver
}

type vnextRehearsalExportOperationsV1 struct {
	afterVerification     func(string) error
	afterReadSnapshot     func(string) error
	removePrivateSnapshot func(string) error
}

type vnextRehearsalProjectV1 struct {
	ID        string
	Label     string
	CreatedAt string
}

type vnextRehearsalFoldV1 struct {
	Order    []string
	Payloads map[string]JournalFactPayload
	FactRows int
}

type vnextRehearsalProjectionV1 struct {
	Payload  JournalFactPayload
	SpecLink bool
	TaskLink bool
}

// ExportVNextRehearsalArchive exports project identity, effective journal
// entries, and deliberate wraps from one verified read-only backup. It returns
// bytes only: no archive file, destination store, activation, or live-state
// mutation is created by this function.
func ExportVNextRehearsalArchive(ctx context.Context, options VNextRehearsalExportOptions) ([]byte, error) {
	return exportVNextRehearsalArchiveV1(ctx, options, vnextRehearsalExportOperationsV1{})
}

func exportVNextRehearsalArchiveV1(
	ctx context.Context,
	options VNextRehearsalExportOptions,
	operations vnextRehearsalExportOperationsV1,
) (encoded []byte, resultErr error) {
	if ctx == nil {
		return nil, fmt.Errorf("export vNext rehearsal archive: context is nil")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	backupPath := options.Backup.BackupPath
	if backupPath == "" || !filepath.IsAbs(backupPath) {
		return nil, fmt.Errorf("export vNext rehearsal archive: backup path must be absolute")
	}
	authoritativeDatabasePath, authoritativeProjectID, err := resolveVNextRehearsalAuthorityV1(
		ctx, options.Root, options.Resolver,
	)
	if err != nil {
		return nil, fmt.Errorf("export vNext rehearsal archive: resolve authoritative live database: %w", err)
	}
	if authoritativeProjectID != options.ProjectID {
		return nil, fmt.Errorf("export vNext rehearsal archive: selected project does not match the authoritative project root")
	}
	if options.Backup.DatabasePath != authoritativeDatabasePath {
		return nil, fmt.Errorf("export vNext rehearsal archive: supplied backup provenance does not match the authoritative live database")
	}
	if filepath.Clean(backupPath) == filepath.Clean(authoritativeDatabasePath) {
		return nil, fmt.Errorf("export vNext rehearsal archive: source must be a standalone backup, not the live database")
	}
	if strings.TrimSpace(options.ProjectID) == "" {
		return nil, fmt.Errorf("export vNext rehearsal archive: project id is empty")
	}
	backupInfo, err := inspectVNextRehearsalBackupArtifactV1(backupPath)
	if err != nil {
		return nil, err
	}
	if liveInfo, liveErr := os.Stat(authoritativeDatabasePath); liveErr == nil && os.SameFile(liveInfo, backupInfo) {
		return nil, fmt.Errorf("export vNext rehearsal archive: source backup aliases the live database")
	} else if liveErr != nil {
		return nil, fmt.Errorf("export vNext rehearsal archive: inspect live database provenance: %w", liveErr)
	}
	privateSnapshotDirectory, privateSnapshotPath, privateSnapshotBytes, privateSnapshotSHA256, privateSnapshotInfo, err := createVNextRehearsalPrivateSnapshotV1(backupPath, backupInfo)
	if err != nil {
		return nil, err
	}
	removePrivateSnapshot := operations.removePrivateSnapshot
	if removePrivateSnapshot == nil {
		removePrivateSnapshot = os.RemoveAll
	}
	defer func() {
		if cleanupErr := removePrivateSnapshot(privateSnapshotDirectory); cleanupErr != nil {
			encoded = nil
			resultErr = errors.Join(
				resultErr,
				fmt.Errorf("export vNext rehearsal archive: remove private snapshot directory: %w", cleanupErr),
			)
		}
	}()
	store, err := openVNextRehearsalPrivateSnapshotV1(privateSnapshotPath)
	if err != nil {
		return nil, fmt.Errorf("export vNext rehearsal archive: open private snapshot: %w", err)
	}
	storeOpen := true
	defer func() {
		if storeOpen {
			if closeErr := store.Close(); closeErr != nil {
				encoded = nil
				resultErr = errors.Join(
					resultErr,
					fmt.Errorf("export vNext rehearsal archive: close private snapshot after failure: %w", closeErr),
				)
			}
		}
	}()
	privateOpenedInfo, err := inspectVNextRehearsalBackupArtifactV1(privateSnapshotPath)
	if err != nil {
		return nil, err
	}
	privateOpenedSHA256, err := fileSHA256(privateSnapshotPath)
	if err != nil {
		return nil, fmt.Errorf("export vNext rehearsal archive: checksum opened private snapshot: %w", err)
	}
	if !os.SameFile(privateSnapshotInfo, privateOpenedInfo) || privateSnapshotInfo.Size() != privateOpenedInfo.Size() ||
		privateOpenedInfo.Size() != privateSnapshotBytes || privateOpenedSHA256 != privateSnapshotSHA256 {
		return nil, fmt.Errorf("export vNext rehearsal archive: private snapshot changed before verification")
	}
	verification, err := verifyVNextRehearsalPrivateSnapshotV1(
		ctx, privateSnapshotPath, privateSnapshotBytes, privateSnapshotSHA256, store,
	)
	if err != nil {
		return nil, fmt.Errorf("export vNext rehearsal archive: verify backup: %w", err)
	}
	if !verification.Verified || !verification.SQLiteValid || !verification.RecoveryReady ||
		verification.SchemaVersion != CurrentSchemaVersion() {
		return nil, fmt.Errorf("export vNext rehearsal archive: backup is not recovery-ready at the current schema")
	}
	if !options.Backup.Verified || !options.Backup.SQLiteValid || !options.Backup.RecoveryReady ||
		options.Backup.ContractVersion != verification.ContractVersion ||
		options.Backup.DatabaseScope != verification.DatabaseScope ||
		options.Backup.BackupPath != backupPath || options.Backup.SHA256 != verification.SHA256 ||
		options.Backup.Bytes != verification.Bytes || options.Backup.Bytes != privateSnapshotBytes ||
		options.Backup.SHA256 != privateSnapshotSHA256 || options.Backup.SchemaVersion != verification.SchemaVersion ||
		options.Backup.ProjectCount != verification.ProjectCount ||
		options.Backup.IntegrityCheck != verification.IntegrityCheck ||
		options.Backup.ForeignKeyCheck != verification.ForeignKeyCheck ||
		options.Backup.JournalRetrievalReady != verification.JournalRetrievalReady ||
		options.Backup.JournalSearchParity != verification.JournalSearchParity ||
		options.Backup.JournalProvenanceIntegrity != verification.JournalProvenanceIntegrity ||
		options.Backup.JournalWatermark != verification.JournalWatermark {
		return nil, fmt.Errorf("export vNext rehearsal archive: supplied backup result does not match fresh verification")
	}
	projectKnown := false
	for _, project := range verification.Projects {
		if project.ID == options.ProjectID {
			projectKnown = true
			break
		}
	}
	if !projectKnown {
		return nil, fmt.Errorf("export vNext rehearsal archive: project %q is absent from the verified backup", options.ProjectID)
	}
	if operations.afterVerification != nil {
		if err := operations.afterVerification(backupPath); err != nil {
			return nil, fmt.Errorf("export vNext rehearsal archive: after verification: %w", err)
		}
	}
	verifiedInfo, err := inspectVNextRehearsalBackupArtifactV1(backupPath)
	if err != nil {
		return nil, err
	}
	if !os.SameFile(backupInfo, verifiedInfo) || backupInfo.Size() != verifiedInfo.Size() {
		return nil, fmt.Errorf("export vNext rehearsal archive: verified backup changed after verification")
	}

	tx, err := store.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("export vNext rehearsal archive: begin read snapshot: %w", err)
	}
	defer tx.Rollback()
	project, err := readVNextRehearsalProjectV1(ctx, tx, options.ProjectID)
	if err != nil {
		return nil, err
	}
	fold, err := readVNextRehearsalJournalFactsV1(ctx, tx, options.ProjectID)
	if err != nil {
		return nil, err
	}
	projection, droppedSpecLinks, droppedTaskLinks, err := readVNextRehearsalJournalProjectionV1(
		ctx, tx, options.ProjectID,
	)
	if err != nil {
		return nil, err
	}
	if err := validateVNextRehearsalJournalParityV1(fold, projection); err != nil {
		return nil, err
	}
	var originRows int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM journal_origins WHERE project_id = ?`, options.ProjectID).Scan(&originRows); err != nil {
		return nil, fmt.Errorf("export vNext rehearsal archive: count journal origins: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("export vNext rehearsal archive: commit read snapshot: %w", err)
	}
	if err := store.Close(); err != nil {
		return nil, fmt.Errorf("export vNext rehearsal archive: close read snapshot: %w", err)
	}
	storeOpen = false
	privateAfterInfo, err := inspectVNextRehearsalBackupArtifactV1(privateSnapshotPath)
	if err != nil {
		return nil, err
	}
	privateAfterSHA256, err := fileSHA256(privateSnapshotPath)
	if err != nil {
		return nil, fmt.Errorf("export vNext rehearsal archive: checksum private snapshot after read: %w", err)
	}
	if !os.SameFile(privateSnapshotInfo, privateAfterInfo) || privateSnapshotInfo.Size() != privateAfterInfo.Size() ||
		privateAfterInfo.Size() != privateSnapshotBytes || privateAfterSHA256 != privateSnapshotSHA256 {
		return nil, fmt.Errorf("export vNext rehearsal archive: private snapshot changed during export")
	}
	if operations.afterReadSnapshot != nil {
		if err := operations.afterReadSnapshot(backupPath); err != nil {
			return nil, fmt.Errorf("export vNext rehearsal archive: after read snapshot: %w", err)
		}
	}

	afterInfo, err := inspectVNextRehearsalBackupArtifactV1(backupPath)
	if err != nil {
		return nil, err
	}
	afterSHA256, err := fileSHA256(backupPath)
	if err != nil {
		return nil, fmt.Errorf("export vNext rehearsal archive: rechecksum verified backup: %w", err)
	}
	if !os.SameFile(backupInfo, afterInfo) || backupInfo.Size() != afterInfo.Size() ||
		afterInfo.Size() != verification.Bytes || afterSHA256 != verification.SHA256 {
		return nil, fmt.Errorf("export vNext rehearsal archive: verified backup changed during export")
	}

	content, err := buildVNextRehearsalContentV1(
		verification, project, fold, projection, originRows, droppedSpecLinks, droppedTaskLinks,
	)
	if err != nil {
		return nil, err
	}
	sealed, err := archive.Seal(content)
	if err != nil {
		return nil, fmt.Errorf("export vNext rehearsal archive: seal archive: %w", err)
	}
	encoded, err = archive.Marshal(sealed)
	if err != nil {
		return nil, fmt.Errorf("export vNext rehearsal archive: marshal archive: %w", err)
	}
	return encoded, nil
}

func resolveVNextRehearsalAuthorityV1(
	ctx context.Context,
	root project.Root,
	resolver PathResolver,
) (string, string, error) {
	databasePath, err := resolver.DatabasePath(root)
	if err != nil {
		return "", "", err
	}
	if !filepath.IsAbs(databasePath) {
		return "", "", fmt.Errorf("authoritative live database path must be absolute")
	}
	store, err := OpenStoreReadOnly(databasePath)
	if err != nil {
		return "", "", err
	}
	defer store.Close()
	identity, err := store.ProjectIdentityForRoot(ctx, root)
	if err != nil {
		return "", "", err
	}
	return databasePath, identity.ID, nil
}

func inspectVNextRehearsalBackupArtifactV1(backupPath string) (os.FileInfo, error) {
	pathInfo, err := os.Lstat(backupPath)
	if err != nil {
		return nil, fmt.Errorf("export vNext rehearsal archive: inspect standalone backup: %w", err)
	}
	if pathInfo.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("export vNext rehearsal archive: standalone backup must not be a symbolic link")
	}
	if !pathInfo.Mode().IsRegular() {
		return nil, fmt.Errorf("export vNext rehearsal archive: standalone backup must be a regular file")
	}
	for _, suffix := range []string{"-wal", "-shm"} {
		if _, sidecarErr := os.Lstat(backupPath + suffix); sidecarErr == nil {
			return nil, fmt.Errorf("export vNext rehearsal archive: standalone backup has SQLite sidecar %s", suffix)
		} else if !os.IsNotExist(sidecarErr) {
			return nil, fmt.Errorf("export vNext rehearsal archive: inspect backup sidecar %s: %w", suffix, sidecarErr)
		}
	}
	return pathInfo, nil
}

func createVNextRehearsalPrivateSnapshotV1(
	backupPath string,
	expectedInfo os.FileInfo,
) (snapshotDirectory string, snapshotPath string, snapshotBytes int64, snapshotSHA256 string, snapshotInfo os.FileInfo, err error) {
	source, err := os.Open(backupPath)
	if err != nil {
		return "", "", 0, "", nil, fmt.Errorf("export vNext rehearsal archive: open standalone backup: %w", err)
	}
	defer source.Close()
	sourceInfo, err := source.Stat()
	if err != nil {
		return "", "", 0, "", nil, fmt.Errorf("export vNext rehearsal archive: stat opened backup: %w", err)
	}
	if !os.SameFile(expectedInfo, sourceInfo) || expectedInfo.Size() != sourceInfo.Size() {
		return "", "", 0, "", nil, fmt.Errorf("export vNext rehearsal archive: standalone backup changed before snapshot")
	}

	snapshotDirectory, err = os.MkdirTemp("", "loaf-vnext-rehearsal-*")
	if err != nil {
		return "", "", 0, "", nil, fmt.Errorf("export vNext rehearsal archive: create private snapshot directory: %w", err)
	}
	snapshotPath = filepath.Join(snapshotDirectory, "legacy.sqlite")
	snapshot, err := os.OpenFile(snapshotPath, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
	if err != nil {
		createErr := fmt.Errorf("export vNext rehearsal archive: create private snapshot: %w", err)
		if cleanupErr := os.RemoveAll(snapshotDirectory); cleanupErr != nil {
			createErr = errors.Join(
				createErr,
				fmt.Errorf("export vNext rehearsal archive: remove failed private snapshot directory: %w", cleanupErr),
			)
		}
		return "", "", 0, "", nil, createErr
	}
	keepSnapshot := false
	snapshotOpen := true
	defer func() {
		if !keepSnapshot {
			if snapshotOpen {
				if closeErr := snapshot.Close(); closeErr != nil {
					err = errors.Join(
						err,
						fmt.Errorf("export vNext rehearsal archive: close failed private snapshot: %w", closeErr),
					)
				}
			}
			if cleanupErr := os.RemoveAll(snapshotDirectory); cleanupErr != nil {
				err = errors.Join(
					err,
					fmt.Errorf("export vNext rehearsal archive: remove failed private snapshot directory: %w", cleanupErr),
				)
			}
		}
	}()

	hash := sha256.New()
	written, err := io.Copy(io.MultiWriter(snapshot, hash), source)
	if err != nil {
		return "", "", 0, "", nil, fmt.Errorf("export vNext rehearsal archive: copy private snapshot: %w", err)
	}
	if written != sourceInfo.Size() {
		return "", "", 0, "", nil, fmt.Errorf("export vNext rehearsal archive: private snapshot byte count changed during copy")
	}
	if err := snapshot.Sync(); err != nil {
		return "", "", 0, "", nil, fmt.Errorf("export vNext rehearsal archive: sync private snapshot: %w", err)
	}
	snapshotInfo, err = snapshot.Stat()
	if err != nil {
		return "", "", 0, "", nil, fmt.Errorf("export vNext rehearsal archive: stat private snapshot: %w", err)
	}
	closeErr := snapshot.Close()
	snapshotOpen = false
	if closeErr != nil {
		return "", "", 0, "", nil, fmt.Errorf("export vNext rehearsal archive: close private snapshot: %w", closeErr)
	}
	if currentInfo, err := inspectVNextRehearsalBackupArtifactV1(backupPath); err != nil {
		return "", "", 0, "", nil, err
	} else if !os.SameFile(expectedInfo, currentInfo) || expectedInfo.Size() != currentInfo.Size() {
		return "", "", 0, "", nil, fmt.Errorf("export vNext rehearsal archive: standalone backup changed during snapshot")
	}
	keepSnapshot = true
	return snapshotDirectory, snapshotPath, written, hex.EncodeToString(hash.Sum(nil)), snapshotInfo, nil
}

func openVNextRehearsalPrivateSnapshotV1(snapshotPath string) (*Store, error) {
	values := url.Values{}
	values.Add("mode", "ro")
	values.Add("immutable", "1")
	values.Add("_pragma", "busy_timeout(5000)")
	values.Add("_pragma", "foreign_keys(on)")
	dsn := (&url.URL{
		Scheme:   "file",
		Path:     filepath.ToSlash(snapshotPath),
		RawQuery: values.Encode(),
	}).String()
	return openStoreReadOnly(snapshotPath, dsn)
}

func verifyVNextRehearsalPrivateSnapshotV1(
	ctx context.Context,
	snapshotPath string,
	snapshotBytes int64,
	snapshotSHA256 string,
	store *Store,
) (BackupVerificationResult, error) {
	integrityCheck, err := verifySQLiteIntegrity(ctx, store)
	if err != nil {
		return BackupVerificationResult{}, fmt.Errorf("verify private snapshot integrity: %w", err)
	}
	foreignKeyCheck, err := verifyNoForeignKeyViolations(ctx, store)
	if err != nil {
		return BackupVerificationResult{}, fmt.Errorf("verify private snapshot foreign keys: %w", err)
	}
	version, err := store.SchemaVersion(ctx)
	if err != nil {
		return BackupVerificationResult{}, fmt.Errorf("verify private snapshot schema version: %w", err)
	}
	if !acceptableSchemaVersion(version) {
		return BackupVerificationResult{}, fmt.Errorf("verify private snapshot schema version: got %d, want %d", version, CurrentSchemaVersion())
	}
	projects, err := store.ListProjects(ctx)
	if err != nil {
		return BackupVerificationResult{}, fmt.Errorf("verify private snapshot projects: %w", err)
	}
	if len(projects.Projects) == 0 {
		return BackupVerificationResult{}, fmt.Errorf("verify private snapshot project count: empty projects table")
	}
	parity, err := InspectJournalSearchParity(ctx, store)
	if err != nil {
		return BackupVerificationResult{}, fmt.Errorf("verify private snapshot journal search parity: %w", err)
	}
	provenance, err := InspectJournalProvenanceIntegrity(ctx, store)
	if err != nil {
		return BackupVerificationResult{}, fmt.Errorf("verify private snapshot journal provenance: %w", err)
	}
	watermark, err := readJournalWatermark(ctx, store)
	if err != nil {
		return BackupVerificationResult{}, fmt.Errorf("verify private snapshot journal watermark: %w", err)
	}
	return BackupVerificationResult{
		ContractVersion:            StateJSONContractVersion,
		DatabaseScope:              "global",
		BackupPath:                 snapshotPath,
		Bytes:                      snapshotBytes,
		SHA256:                     snapshotSHA256,
		Verified:                   true,
		SchemaVersion:              version,
		ProjectCount:               len(projects.Projects),
		Projects:                   projects.Projects,
		IntegrityCheck:             integrityCheck,
		ForeignKeyCheck:            foreignKeyCheck,
		JournalRetrievalReady:      parity.Ready,
		JournalSearchParity:        parity,
		JournalProvenanceIntegrity: provenance,
		SQLiteValid:                true,
		RecoveryReady:              parity.Ready && provenance.Ready,
		JournalWatermark:           watermark,
	}, nil
}

func readVNextRehearsalProjectV1(ctx context.Context, tx *sql.Tx, projectID string) (vnextRehearsalProjectV1, error) {
	var project vnextRehearsalProjectV1
	err := tx.QueryRowContext(ctx, `
SELECT id, COALESCE(NULLIF(friendly_name, ''), id), created_at
FROM projects
WHERE id = ?`, projectID).Scan(&project.ID, &project.Label, &project.CreatedAt)
	if err != nil {
		return vnextRehearsalProjectV1{}, fmt.Errorf("export vNext rehearsal archive: read project: %w", err)
	}
	return project, nil
}

func readVNextRehearsalJournalFactsV1(ctx context.Context, tx *sql.Tx, projectID string) (vnextRehearsalFoldV1, error) {
	rows, err := tx.QueryContext(ctx, `
SELECT id, payload
FROM facts
WHERE project_id = ? AND kind = ?
ORDER BY hlc COLLATE BINARY ASC, env_id COLLATE BINARY ASC, id COLLATE BINARY ASC`, projectID, FactKindJournal)
	if err != nil {
		return vnextRehearsalFoldV1{}, fmt.Errorf("export vNext rehearsal archive: read journal facts: %w", err)
	}
	defer rows.Close()
	fold := vnextRehearsalFoldV1{Order: []string{}, Payloads: map[string]JournalFactPayload{}}
	seen := map[string]struct{}{}
	for rows.Next() {
		var factID, payloadRaw string
		if err := rows.Scan(&factID, &payloadRaw); err != nil {
			return vnextRehearsalFoldV1{}, fmt.Errorf("export vNext rehearsal archive: scan journal fact: %w", err)
		}
		payload, err := decodeVNextRehearsalJournalFactPayloadV1(payloadRaw)
		if err != nil {
			return vnextRehearsalFoldV1{}, fmt.Errorf("export vNext rehearsal archive: %w", err)
		}
		entryID := journalEntryIDFromFact(factID, payload)
		if _, found := seen[entryID]; !found {
			seen[entryID] = struct{}{}
			fold.Order = append(fold.Order, entryID)
		}
		fold.Payloads[entryID] = payload
		fold.FactRows++
	}
	if err := rows.Err(); err != nil {
		return vnextRehearsalFoldV1{}, fmt.Errorf("export vNext rehearsal archive: iterate journal facts: %w", err)
	}
	return fold, nil
}

func decodeVNextRehearsalJournalFactPayloadV1(raw string) (JournalFactPayload, error) {
	decoder := json.NewDecoder(strings.NewReader(raw))
	opening, err := decoder.Token()
	if err != nil {
		return JournalFactPayload{}, fmt.Errorf("decode strict journal fact payload: %w", err)
	}
	if delimiter, ok := opening.(json.Delim); !ok || delimiter != '{' {
		return JournalFactPayload{}, fmt.Errorf("decode strict journal fact payload: expected JSON object")
	}
	payload := JournalFactPayload{}
	seen := map[string]struct{}{}
	for decoder.More() {
		nameToken, err := decoder.Token()
		if err != nil {
			return JournalFactPayload{}, fmt.Errorf("decode strict journal fact payload member: %w", err)
		}
		name, ok := nameToken.(string)
		if !ok {
			return JournalFactPayload{}, fmt.Errorf("decode strict journal fact payload: member name is not a string")
		}
		if _, duplicate := seen[name]; duplicate {
			return JournalFactPayload{}, fmt.Errorf("decode strict journal fact payload: duplicate member %q", name)
		}
		seen[name] = struct{}{}
		var destination *string
		switch name {
		case "entry_type":
			destination = &payload.EntryType
		case "scope":
			destination = &payload.Scope
		case "message":
			destination = &payload.Message
		case "observed_branch":
			destination = &payload.ObservedBranch
		case "observed_worktree":
			destination = &payload.ObservedWorktree
		case "harness_session_id":
			destination = &payload.HarnessSessionID
		case "created_at":
			destination = &payload.CreatedAt
		case "updated_at":
			destination = &payload.UpdatedAt
		case "entry_id":
			destination = &payload.EntryID
		default:
			return JournalFactPayload{}, fmt.Errorf("decode strict journal fact payload: unknown member %q", name)
		}
		valueToken, err := decoder.Token()
		if err != nil {
			return JournalFactPayload{}, fmt.Errorf("decode strict journal fact payload member %q: %w", name, err)
		}
		value, ok := valueToken.(string)
		if !ok {
			return JournalFactPayload{}, fmt.Errorf("decode strict journal fact payload member %q: value must be a string", name)
		}
		*destination = value
	}
	closing, err := decoder.Token()
	if err != nil {
		return JournalFactPayload{}, fmt.Errorf("decode strict journal fact payload closing object: %w", err)
	}
	if delimiter, ok := closing.(json.Delim); !ok || delimiter != '}' {
		return JournalFactPayload{}, fmt.Errorf("decode strict journal fact payload: expected closing object")
	}
	for _, required := range []string{"entry_type", "message", "created_at", "updated_at"} {
		if _, found := seen[required]; !found {
			return JournalFactPayload{}, fmt.Errorf("decode strict journal fact payload: required member %q is missing", required)
		}
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return JournalFactPayload{}, fmt.Errorf("decode strict journal fact payload: multiple JSON values")
		}
		return JournalFactPayload{}, fmt.Errorf("decode strict journal fact payload trailing data: %w", err)
	}
	return payload, nil
}

func readVNextRehearsalJournalProjectionV1(
	ctx context.Context,
	tx *sql.Tx,
	projectID string,
) (map[string]vnextRehearsalProjectionV1, int, int, error) {
	rows, err := tx.QueryContext(ctx, `
SELECT id, entry_type, COALESCE(scope, ''), message,
       COALESCE(observed_branch, ''), COALESCE(observed_worktree, ''),
       COALESCE(harness_session_id, ''), created_at, updated_at, spec_id, task_id
FROM journal_entries
WHERE project_id = ?
ORDER BY id COLLATE BINARY ASC`, projectID)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("export vNext rehearsal archive: read journal projection: %w", err)
	}
	defer rows.Close()
	projection := map[string]vnextRehearsalProjectionV1{}
	droppedSpecLinks := 0
	droppedTaskLinks := 0
	for rows.Next() {
		var entryID string
		var payload JournalFactPayload
		var specID, taskID sql.NullString
		if err := rows.Scan(
			&entryID, &payload.EntryType, &payload.Scope, &payload.Message,
			&payload.ObservedBranch, &payload.ObservedWorktree, &payload.HarnessSessionID,
			&payload.CreatedAt, &payload.UpdatedAt, &specID, &taskID,
		); err != nil {
			return nil, 0, 0, fmt.Errorf("export vNext rehearsal archive: scan journal projection: %w", err)
		}
		if _, duplicate := projection[entryID]; duplicate {
			return nil, 0, 0, fmt.Errorf("export vNext rehearsal archive: duplicate journal projection id %q", entryID)
		}
		projection[entryID] = vnextRehearsalProjectionV1{Payload: payload, SpecLink: specID.Valid, TaskLink: taskID.Valid}
		if specID.Valid {
			droppedSpecLinks++
		}
		if taskID.Valid {
			droppedTaskLinks++
		}
	}
	if err := rows.Err(); err != nil {
		return nil, 0, 0, fmt.Errorf("export vNext rehearsal archive: iterate journal projection: %w", err)
	}
	return projection, droppedSpecLinks, droppedTaskLinks, nil
}

func validateVNextRehearsalJournalParityV1(
	fold vnextRehearsalFoldV1,
	projection map[string]vnextRehearsalProjectionV1,
) error {
	if len(fold.Payloads) != len(projection) {
		return fmt.Errorf("export vNext rehearsal archive: selected project journal fact/projection row counts diverge")
	}
	for entryID, fact := range fold.Payloads {
		projected, found := projection[entryID]
		if !found || !journalProjectionMatchesPayload(
			projected.Payload.EntryType, projected.Payload.Scope, projected.Payload.Message,
			projected.Payload.ObservedBranch, projected.Payload.ObservedWorktree,
			projected.Payload.HarnessSessionID, projected.Payload.CreatedAt,
			projected.Payload.UpdatedAt, fact,
		) {
			return fmt.Errorf("export vNext rehearsal archive: journal %q fact/projection content diverges", entryID)
		}
	}
	return nil
}

func buildVNextRehearsalContentV1(
	verification BackupVerificationResult,
	project vnextRehearsalProjectV1,
	fold vnextRehearsalFoldV1,
	projection map[string]vnextRehearsalProjectionV1,
	originRows int,
	droppedSpecLinks int,
	droppedTaskLinks int,
) (archive.Content, error) {
	projectObservedAt, err := parseVNextRehearsalTimeV1(project.CreatedAt, "project created_at")
	if err != nil {
		return archive.Content{}, err
	}
	projectID := continuity.ProjectID(project.ID)
	records := make([]archive.Record, 0, len(fold.Order)+1)
	records = append(records, archive.Record{
		Kind:        archive.RecordProject,
		FactID:      continuity.FactID(vnextRehearsalIDV1("fact_migration_", "project-fact", project.ID)),
		SubjectID:   continuity.SubjectID(projectID),
		Observation: archive.Observation{ObservedAtMillis: projectObservedAt},
		Project:     &archive.ProjectRecord{Label: project.Label},
	})
	for _, entryID := range fold.Order {
		projected := projection[entryID].Payload
		observedAt, err := parseVNextRehearsalTimeV1(projected.CreatedAt, "journal created_at")
		if err != nil {
			return archive.Content{}, fmt.Errorf("export vNext rehearsal archive: journal %q: %w", entryID, err)
		}
		observation := archive.Observation{
			ObservedAtMillis: observedAt,
			HarnessSessionID: projected.HarnessSessionID,
			Branch:           projected.ObservedBranch,
			Worktree:         projected.ObservedWorktree,
		}
		if projected.EntryType == string(continuity.JournalWrap) {
			records = append(records, archive.Record{
				Kind: archive.RecordWrap, SourceID: entryID,
				FactID:      continuity.FactID(vnextRehearsalIDV1("fact_migration_", "wrap-fact", project.ID, entryID)),
				SubjectID:   continuity.SubjectID(vnextRehearsalIDV1("wrap_migration_", "wrap-subject", project.ID, entryID)),
				Observation: observation,
				Wrap:        &archive.WrapRecord{Scope: projected.Scope, Synthesis: projected.Message},
			})
			continue
		}
		category, err := vnextRehearsalJournalCategoryV1(projected.EntryType)
		if err != nil {
			return archive.Content{}, fmt.Errorf("export vNext rehearsal archive: journal %q: %w", entryID, err)
		}
		records = append(records, archive.Record{
			Kind: archive.RecordJournal, SourceID: entryID,
			FactID:      continuity.FactID(vnextRehearsalIDV1("fact_migration_", "journal-fact", project.ID, entryID)),
			SubjectID:   continuity.SubjectID(vnextRehearsalIDV1("journal_migration_", "journal-subject", project.ID, entryID)),
			Observation: observation,
			Journal:     &archive.JournalRecord{Category: category, Scope: projected.Scope, Text: projected.Message},
		})
	}
	return archive.Content{
		Source: archive.Source{
			LegacySchemaVersion:   verification.SchemaVersion,
			BackupSHA256:          verification.SHA256,
			BackupBytes:           verification.Bytes,
			JournalFactRows:       fold.FactRows,
			JournalProjectionRows: len(projection),
			CollapsedRevisionRows: fold.FactRows - len(projection),
			JournalOriginRows:     originRows,
			DroppedSpecLinks:      droppedSpecLinks,
			DroppedTaskLinks:      droppedTaskLinks,
		},
		Project:  archive.ProjectMapping{LegacyProjectID: project.ID, ProjectID: projectID, Label: project.Label},
		Families: archive.FamilyManifest{Project: true, Journal: true, Wrap: true},
		Records:  records,
	}, nil
}

func vnextRehearsalJournalCategoryV1(entryType string) (continuity.JournalCategory, error) {
	category := continuity.JournalCategory(entryType)
	switch category {
	case continuity.JournalNote, continuity.JournalSkill, continuity.JournalCommit,
		continuity.JournalDecision, continuity.JournalDiscover, continuity.JournalBlock,
		continuity.JournalUnblock, continuity.JournalSpark, continuity.JournalTodo,
		continuity.JournalFinding:
		return category, nil
	default:
		return "", fmt.Errorf("unsupported legacy journal category %q", entryType)
	}
}

func parseVNextRehearsalTimeV1(value string, field string) (int64, error) {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return 0, fmt.Errorf("%s is not RFC3339: %w", field, err)
	}
	return parsed.UTC().UnixMilli(), nil
}

func vnextRehearsalIDV1(prefix, role string, parts ...string) string {
	hash := sha256.New()
	hash.Write([]byte(vnextRehearsalIDDomainV1))
	writeVNextRehearsalIDPartV1(hash, role)
	for _, part := range parts {
		writeVNextRehearsalIDPartV1(hash, part)
	}
	return prefix + hex.EncodeToString(hash.Sum(nil))
}

type vnextRehearsalHashWriterV1 interface {
	Write([]byte) (int, error)
}

func writeVNextRehearsalIDPartV1(writer vnextRehearsalHashWriterV1, value string) {
	var length [8]byte
	binary.BigEndian.PutUint64(length[:], uint64(len(value)))
	_, _ = writer.Write(length[:])
	_, _ = writer.Write([]byte(value))
}
