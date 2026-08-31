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
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/levifig/loaf/internal/project"
	"github.com/levifig/loaf/vnext/continuity"
	"github.com/levifig/loaf/vnext/migration/archive"
)

const (
	vnextRehearsalIDDomainV1                 = "loaf-vnext-rehearsal-v1"
	vnextRehearsalMaxRecordsV1               = 100_000
	vnextRehearsalMaxAggregatePayloadBytesV1 = 32 << 20
)

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
	Payloads map[string]JournalFactPayload
	Roots    map[string]vnextRehearsalFactRootV1
	FactRows int
}

type vnextRehearsalProjectionV1 struct {
	Payload  JournalFactPayload
	SpecLink bool
	TaskLink bool
}

type vnextRehearsalHandoffFoldV1 struct {
	Facts    map[string]CoreEventPayload
	Roots    map[string]vnextRehearsalFactRootV1
	FactRows int
}

type vnextRehearsalFactRootV1 struct {
	Family    string
	SubjectID string
	HLC       HLC
	EnvID     string
	FactID    string
}

type vnextRehearsalArchiveBudgetV1 struct {
	RecordCount  int
	PayloadBytes int
}

type vnextRehearsalHandoffProjectionV1 struct {
	ID               string
	Alias            string
	Title            string
	Body             string
	HarnessSessionID string
	CreatedAt        string
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
	archiveBudget := vnextRehearsalArchiveBudgetV1ForJournal(project, projection)
	handoffProjection, err := readVNextRehearsalHandoffProjectionV1(ctx, tx, options.ProjectID, archiveBudget)
	if err != nil {
		return nil, err
	}
	handoffFold, err := readVNextRehearsalHandoffFactsV1(ctx, tx, options.ProjectID, archiveBudget)
	if err != nil {
		return nil, err
	}
	if err := validateVNextRehearsalHandoffParityV1(handoffFold, handoffProjection); err != nil {
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
		verification, project, fold, projection, handoffFold, handoffProjection,
		originRows, droppedSpecLinks, droppedTaskLinks,
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
SELECT id, payload, hlc, env_id
FROM facts
WHERE project_id = ? AND kind = ?
ORDER BY hlc COLLATE BINARY ASC, env_id COLLATE BINARY ASC, id COLLATE BINARY ASC`, projectID, FactKindJournal)
	if err != nil {
		return vnextRehearsalFoldV1{}, fmt.Errorf("export vNext rehearsal archive: read journal facts: %w", err)
	}
	defer rows.Close()
	fold := vnextRehearsalFoldV1{
		Payloads: map[string]JournalFactPayload{},
		Roots:    map[string]vnextRehearsalFactRootV1{},
	}
	heads := map[string]vnextRehearsalFactRootV1{}
	for rows.Next() {
		var factID, payloadRaw, rawHLC, envID string
		if err := rows.Scan(&factID, &payloadRaw, &rawHLC, &envID); err != nil {
			return vnextRehearsalFoldV1{}, fmt.Errorf("export vNext rehearsal archive: scan journal fact: %w", err)
		}
		stamp, err := vnextRehearsalFactRootV1FromRaw(FactKindJournal, "", rawHLC, envID, factID)
		if err != nil {
			return vnextRehearsalFoldV1{}, fmt.Errorf("export vNext rehearsal archive: journal fact %q: %w", factID, err)
		}
		payload, err := decodeVNextRehearsalJournalFactPayloadV1(payloadRaw)
		if err != nil {
			return vnextRehearsalFoldV1{}, fmt.Errorf("export vNext rehearsal archive: %w", err)
		}
		entryID := journalEntryIDFromFact(factID, payload)
		root, found := fold.Roots[entryID]
		if !found || compareFactOrder(stamp.HLC, stamp.EnvID, stamp.FactID, root.HLC, root.EnvID, root.FactID) < 0 {
			stamp.SubjectID = entryID
			fold.Roots[entryID] = stamp
		}
		head, found := heads[entryID]
		if !found || compareFactOrder(head.HLC, head.EnvID, head.FactID, stamp.HLC, stamp.EnvID, stamp.FactID) < 0 {
			fold.Payloads[entryID] = payload
			heads[entryID] = stamp
		}
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

func vnextRehearsalArchiveBudgetV1ForJournal(
	project vnextRehearsalProjectV1,
	projection map[string]vnextRehearsalProjectionV1,
) vnextRehearsalArchiveBudgetV1 {
	budget := vnextRehearsalArchiveBudgetV1{
		RecordCount:  1 + len(projection),
		PayloadBytes: 2 * len(project.Label),
	}
	for entryID, projected := range projection {
		payload := projected.Payload
		budget.PayloadBytes += len(entryID) + len(payload.HarnessSessionID) + len(payload.ObservedBranch) +
			len(payload.ObservedWorktree) + len(payload.Scope) + len(payload.Message)
	}
	return budget
}

func readVNextRehearsalHandoffFactsV1(
	ctx context.Context,
	tx *sql.Tx,
	projectID string,
	budget vnextRehearsalArchiveBudgetV1,
) (vnextRehearsalHandoffFoldV1, error) {
	var factRows int
	var selectedBytes int64
	if err := tx.QueryRowContext(ctx, `
SELECT COUNT(*), COALESCE(SUM(
    length(CAST(id AS BLOB)) +
    length(CAST(payload AS BLOB)) +
    length(CAST(hlc AS BLOB)) +
    length(CAST(env_id AS BLOB))
), 0)
FROM facts
WHERE project_id = ? AND kind = ?`, projectID, FactKindHandoffRecorded).Scan(&factRows, &selectedBytes); err != nil {
		return vnextRehearsalHandoffFoldV1{}, fmt.Errorf("export vNext rehearsal archive: inspect handoff fact bounds: %w", err)
	}
	if err := validateVNextRehearsalHandoffFactBoundsV1(factRows, selectedBytes, budget); err != nil {
		return vnextRehearsalHandoffFoldV1{}, err
	}
	rows, err := tx.QueryContext(ctx, `
SELECT id, payload, hlc, env_id
FROM facts
WHERE project_id = ? AND kind = ?
ORDER BY hlc COLLATE BINARY ASC, env_id COLLATE BINARY ASC, id COLLATE BINARY ASC`, projectID, FactKindHandoffRecorded)
	if err != nil {
		return vnextRehearsalHandoffFoldV1{}, fmt.Errorf("export vNext rehearsal archive: read handoff facts: %w", err)
	}
	defer rows.Close()
	fold := vnextRehearsalHandoffFoldV1{
		Facts: make(map[string]CoreEventPayload, factRows),
		Roots: make(map[string]vnextRehearsalFactRootV1, factRows),
	}
	for rows.Next() {
		var factID, payloadRaw, rawHLC, envID string
		if err := rows.Scan(&factID, &payloadRaw, &rawHLC, &envID); err != nil {
			return vnextRehearsalHandoffFoldV1{}, fmt.Errorf("export vNext rehearsal archive: scan handoff fact: %w", err)
		}
		stamp, err := vnextRehearsalFactRootV1FromRaw(FactKindHandoffRecorded, "", rawHLC, envID, factID)
		if err != nil {
			return vnextRehearsalHandoffFoldV1{}, fmt.Errorf("export vNext rehearsal archive: handoff fact %q: %w", factID, err)
		}
		payload, err := decodeVNextRehearsalHandoffFactPayloadV1(payloadRaw)
		if err != nil {
			return vnextRehearsalHandoffFoldV1{}, fmt.Errorf("export vNext rehearsal archive: handoff fact %q: %w", factID, err)
		}
		if _, duplicate := fold.Facts[payload.SubjectID]; duplicate {
			return vnextRehearsalHandoffFoldV1{}, fmt.Errorf("export vNext rehearsal archive: handoff %q has more than one legacy fact", payload.SubjectID)
		}
		fold.Facts[payload.SubjectID] = payload
		stamp.SubjectID = payload.SubjectID
		fold.Roots[payload.SubjectID] = stamp
		fold.FactRows++
	}
	if err := rows.Err(); err != nil {
		return vnextRehearsalHandoffFoldV1{}, fmt.Errorf("export vNext rehearsal archive: iterate handoff facts: %w", err)
	}
	return fold, nil
}

func validateVNextRehearsalHandoffFactBoundsV1(
	factRows int,
	selectedBytes int64,
	budget vnextRehearsalArchiveBudgetV1,
) error {
	if budget.RecordCount+factRows > vnextRehearsalMaxRecordsV1 {
		return fmt.Errorf("export vNext rehearsal archive: handoff facts would exceed archive record limit %d", vnextRehearsalMaxRecordsV1)
	}
	if budget.PayloadBytes > vnextRehearsalMaxAggregatePayloadBytesV1 {
		return fmt.Errorf("export vNext rehearsal archive: journal projection already exceeds archive aggregate payload limit %d", vnextRehearsalMaxAggregatePayloadBytesV1)
	}
	if selectedBytes > int64(vnextRehearsalMaxAggregatePayloadBytesV1-budget.PayloadBytes) {
		return fmt.Errorf("export vNext rehearsal archive: handoff fact selected bytes would exceed archive aggregate payload limit %d", vnextRehearsalMaxAggregatePayloadBytesV1)
	}
	return nil
}

func vnextRehearsalFactRootV1FromRaw(family, subjectID, rawHLC, envID, factID string) (vnextRehearsalFactRootV1, error) {
	hlc, err := parseHLC(rawHLC)
	if err != nil {
		return vnextRehearsalFactRootV1{}, err
	}
	if hlc.String() != rawHLC {
		return vnextRehearsalFactRootV1{}, fmt.Errorf("noncanonical hlc %q", rawHLC)
	}
	return vnextRehearsalFactRootV1{Family: family, SubjectID: subjectID, HLC: hlc, EnvID: envID, FactID: factID}, nil
}

func decodeVNextRehearsalHandoffFactPayloadV1(raw string) (CoreEventPayload, error) {
	if !utf8.ValidString(raw) {
		return CoreEventPayload{}, fmt.Errorf("decode strict handoff fact payload: invalid UTF-8")
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	opening, err := decoder.Token()
	if err != nil {
		return CoreEventPayload{}, fmt.Errorf("decode strict handoff fact payload: %w", err)
	}
	if delimiter, ok := opening.(json.Delim); !ok || delimiter != '{' {
		return CoreEventPayload{}, fmt.Errorf("decode strict handoff fact payload: expected JSON object")
	}
	fields := map[string]bool{
		"subject_kind": true, "subject_id": true, "alias": true, "status": true,
		"title": true, "body": true, "note": true, "harness_session_id": true,
		"task_id": true, "created_at": true, "updated_at": true,
		"text": false, "scope": false, "from_status": false, "to_status": false,
		"related_kind": false, "related_id": false, "version": false, "tag": false,
		"tagged_commit": false, "notes": false, "members": false, "backend": false,
		"entity_kind": false, "entity_id": false, "external_kind": false, "external_id": false,
		"external_url": false, "sync_status": false, "provider": false, "provider_ref": false,
		"mapping_kind": false, "mapping_value": false, "receipt_kind": false, "receipt_value": false,
		"branch": false, "worktree": false,
	}
	values := map[string]string{}
	for decoder.More() {
		nameToken, err := decoder.Token()
		if err != nil {
			return CoreEventPayload{}, fmt.Errorf("decode strict handoff fact payload member: %w", err)
		}
		name, ok := nameToken.(string)
		if !ok {
			return CoreEventPayload{}, fmt.Errorf("decode strict handoff fact payload: member name is not a string")
		}
		if _, duplicate := values[name]; duplicate {
			return CoreEventPayload{}, fmt.Errorf("decode strict handoff fact payload: duplicate member %q", name)
		}
		supported, known := fields[name]
		if !known {
			return CoreEventPayload{}, fmt.Errorf("decode strict handoff fact payload: unknown member %q", name)
		}
		valueToken, err := decoder.Token()
		if err != nil {
			return CoreEventPayload{}, fmt.Errorf("decode strict handoff fact payload member %q: %w", name, err)
		}
		value, ok := valueToken.(string)
		if !ok {
			return CoreEventPayload{}, fmt.Errorf("decode strict handoff fact payload member %q: value must be a string", name)
		}
		if !supported {
			if value != "" {
				return CoreEventPayload{}, fmt.Errorf("decode strict handoff fact payload: unsupported member %q is nonempty", name)
			}
		}
		values[name] = value
	}
	closing, err := decoder.Token()
	if err != nil {
		return CoreEventPayload{}, fmt.Errorf("decode strict handoff fact payload closing object: %w", err)
	}
	if delimiter, ok := closing.(json.Delim); !ok || delimiter != '}' {
		return CoreEventPayload{}, fmt.Errorf("decode strict handoff fact payload: expected closing object")
	}
	for _, required := range []string{"subject_kind", "subject_id", "status", "title", "body", "created_at", "updated_at"} {
		if _, found := values[required]; !found {
			return CoreEventPayload{}, fmt.Errorf("decode strict handoff fact payload: required member %q is missing", required)
		}
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return CoreEventPayload{}, fmt.Errorf("decode strict handoff fact payload: multiple JSON values")
		}
		return CoreEventPayload{}, fmt.Errorf("decode strict handoff fact payload trailing data: %w", err)
	}
	return CoreEventPayload{
		SubjectKind: values["subject_kind"], SubjectID: values["subject_id"], Alias: values["alias"],
		Status: values["status"], Title: values["title"], Body: values["body"], Note: values["note"],
		HarnessSessionID: values["harness_session_id"], TaskID: values["task_id"],
		CreatedAt: values["created_at"], UpdatedAt: values["updated_at"],
	}, nil
}

func auditVNextRehearsalHandoffProjectionShapeV1(ctx context.Context, tx *sql.Tx, projectID string) error {
	var handoffID string
	if err := tx.QueryRowContext(ctx, `
SELECT h.id
FROM handoffs AS h
LEFT JOIN aliases AS a
  ON a.project_id = h.project_id AND a.entity_kind = 'handoff' AND a.entity_id = h.id
WHERE h.project_id = ?
GROUP BY h.id
HAVING COUNT(a.id) <> 1
    OR SUM(CASE WHEN a.namespace = 'handoff' THEN 1 ELSE 0 END) <> 1
LIMIT 1`, projectID).Scan(&handoffID); err == nil {
		return fmt.Errorf("export vNext rehearsal archive: handoff %q alias cardinality or namespace diverges", handoffID)
	} else if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("export vNext rehearsal archive: audit handoff aliases: %w", err)
	}

	if err := tx.QueryRowContext(ctx, `
SELECT h.id
FROM handoffs AS h
LEFT JOIN artifact_bodies AS b
  ON b.project_id = h.project_id AND b.entity_kind = 'handoff' AND b.entity_id = h.id
WHERE h.project_id = ?
GROUP BY h.id
HAVING COUNT(b.id) <> 1
LIMIT 1`, projectID).Scan(&handoffID); err == nil {
		return fmt.Errorf("export vNext rehearsal archive: handoff %q body cardinality diverges", handoffID)
	} else if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("export vNext rehearsal archive: audit handoff bodies: %w", err)
	}

	if err := tx.QueryRowContext(ctx, `
WITH unsupported(entity_id) AS (
    SELECT from_entity_id FROM relationships
    WHERE project_id = ? AND from_entity_kind = 'handoff'
    UNION ALL
    SELECT to_entity_id FROM relationships
    WHERE project_id = ? AND to_entity_kind = 'handoff'
    UNION ALL
    SELECT entity_id FROM entity_tags
    WHERE project_id = ? AND entity_kind = 'handoff'
    UNION ALL
    SELECT entity_id FROM bundle_members
    WHERE project_id = ? AND entity_kind = 'handoff'
    UNION ALL
    SELECT entity_id FROM backend_mappings
    WHERE project_id = ? AND entity_kind = 'handoff'
    UNION ALL
    SELECT entity_id FROM events
    WHERE project_id = ? AND entity_kind = 'handoff'
)
SELECT h.id
FROM unsupported AS u
JOIN handoffs AS h ON h.project_id = ? AND h.id = u.entity_id
LIMIT 1`, projectID, projectID, projectID, projectID, projectID, projectID, projectID).Scan(&handoffID); err == nil {
		return fmt.Errorf("export vNext rehearsal archive: handoff %q has unsupported relationship or projection metadata", handoffID)
	} else if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("export vNext rehearsal archive: audit handoff relationship metadata: %w", err)
	}
	return nil
}

func readVNextRehearsalHandoffProjectionV1(
	ctx context.Context,
	tx *sql.Tx,
	projectID string,
	budget vnextRehearsalArchiveBudgetV1,
) (map[string]vnextRehearsalHandoffProjectionV1, error) {
	var handoffRows int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM handoffs WHERE project_id = ?`, projectID).Scan(&handoffRows); err != nil {
		return nil, fmt.Errorf("export vNext rehearsal archive: inspect handoff projection bounds: %w", err)
	}
	if budget.RecordCount+handoffRows > vnextRehearsalMaxRecordsV1 {
		return nil, fmt.Errorf("export vNext rehearsal archive: handoff projection rows would exceed archive record limit %d", vnextRehearsalMaxRecordsV1)
	}
	if budget.PayloadBytes > vnextRehearsalMaxAggregatePayloadBytesV1 {
		return nil, fmt.Errorf("export vNext rehearsal archive: journal projection already exceeds archive aggregate payload limit %d", vnextRehearsalMaxAggregatePayloadBytesV1)
	}
	correlationColumn, err := handoffCorrelationColumn(ctx, tx)
	if err != nil {
		return nil, fmt.Errorf("export vNext rehearsal archive: inspect handoff correlation: %w", err)
	}
	correlationExpression := "COALESCE(h.harness_session_id, '')"
	if correlationColumn == "session_id" {
		correlationExpression = "COALESCE((SELECT s.harness_session_id FROM sessions AS s WHERE s.id = h.session_id), '')"
	}
	if err := auditVNextRehearsalHandoffProjectionShapeV1(ctx, tx, projectID); err != nil {
		return nil, err
	}
	var selectedBytes int64
	if err := tx.QueryRowContext(ctx, fmt.Sprintf(`
SELECT COALESCE(SUM(
    length(CAST(h.id AS BLOB)) +
    length(CAST(%s AS BLOB)) +
    length(CAST(COALESCE(h.task_id, '') AS BLOB)) +
    length(CAST(h.title AS BLOB)) +
    length(CAST(h.status AS BLOB)) +
    length(CAST(COALESCE(h.body_source_id, '') AS BLOB)) +
    length(CAST(h.created_at AS BLOB)) +
    length(CAST(h.updated_at AS BLOB)) +
    length(CAST(a.alias AS BLOB)) +
    length(CAST(a.created_at AS BLOB)) +
    length(CAST(a.updated_at AS BLOB)) +
    length(CAST(b.body_kind AS BLOB)) +
    length(CAST(b.content AS BLOB)) +
    length(CAST(b.content_hash AS BLOB)) +
    length(CAST(COALESCE(b.source_id, '') AS BLOB)) +
    length(CAST(b.created_at AS BLOB)) +
    length(CAST(b.updated_at AS BLOB))
), 0)
FROM handoffs AS h
JOIN aliases AS a
  ON a.project_id = h.project_id AND a.entity_kind = 'handoff' AND a.entity_id = h.id
JOIN artifact_bodies AS b
  ON b.project_id = h.project_id AND b.entity_kind = 'handoff' AND b.entity_id = h.id
WHERE h.project_id = ?`, correlationExpression), projectID).Scan(&selectedBytes); err != nil {
		return nil, fmt.Errorf("export vNext rehearsal archive: inspect handoff projection byte bounds: %w", err)
	}
	if selectedBytes > int64(vnextRehearsalMaxAggregatePayloadBytesV1-budget.PayloadBytes) {
		return nil, fmt.Errorf("export vNext rehearsal archive: handoff projection selected bytes would exceed archive aggregate payload limit %d", vnextRehearsalMaxAggregatePayloadBytesV1)
	}
	rows, err := tx.QueryContext(ctx, fmt.Sprintf(`
SELECT h.id, %s, COALESCE(h.task_id, ''), h.title, h.status,
       COALESCE(h.body_source_id, ''), h.created_at, h.updated_at,
       a.alias, a.created_at, a.updated_at,
       b.body_kind, b.content, b.content_hash, COALESCE(b.source_id, ''), b.created_at, b.updated_at
FROM handoffs AS h
JOIN aliases AS a
  ON a.project_id = h.project_id AND a.entity_kind = 'handoff' AND a.entity_id = h.id
JOIN artifact_bodies AS b
  ON b.project_id = h.project_id AND b.entity_kind = 'handoff' AND b.entity_id = h.id
WHERE h.project_id = ?`, correlationExpression), projectID)
	if err != nil {
		return nil, fmt.Errorf("export vNext rehearsal archive: read handoff projection: %w", err)
	}
	defer rows.Close()
	projection := make(map[string]vnextRehearsalHandoffProjectionV1, handoffRows)
	aggregatePayloadBytes := budget.PayloadBytes
	for rows.Next() {
		var handoff vnextRehearsalHandoffProjectionV1
		var taskID, status, bodySourceID, updatedAt string
		var aliasCreatedAt, aliasUpdatedAt string
		var bodyKind, contentHash, bodySource, bodyCreatedAt, bodyUpdatedAt string
		if err := rows.Scan(
			&handoff.ID, &handoff.HarnessSessionID, &taskID, &handoff.Title, &status,
			&bodySourceID, &handoff.CreatedAt, &updatedAt,
			&handoff.Alias, &aliasCreatedAt, &aliasUpdatedAt,
			&bodyKind, &handoff.Body, &contentHash, &bodySource, &bodyCreatedAt, &bodyUpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("export vNext rehearsal archive: scan handoff projection: %w", err)
		}
		for _, field := range []struct {
			name  string
			value string
		}{
			{"id", handoff.ID}, {"harness_session_id", handoff.HarnessSessionID}, {"task_id", taskID},
			{"title", handoff.Title}, {"status", status}, {"body_source_id", bodySourceID},
			{"created_at", handoff.CreatedAt}, {"updated_at", updatedAt}, {"alias", handoff.Alias},
			{"alias created_at", aliasCreatedAt}, {"alias updated_at", aliasUpdatedAt}, {"body_kind", bodyKind},
			{"body", handoff.Body}, {"content_hash", contentHash}, {"body source_id", bodySource},
			{"body created_at", bodyCreatedAt}, {"body updated_at", bodyUpdatedAt},
		} {
			if !utf8.ValidString(field.value) {
				return nil, fmt.Errorf("export vNext rehearsal archive: handoff %q projection %s contains invalid UTF-8", handoff.ID, field.name)
			}
		}
		if status != LifecycleStatusDraft || taskID != "" || bodySourceID != "" || handoff.CreatedAt != updatedAt {
			return nil, fmt.Errorf("export vNext rehearsal archive: handoff %q has unsupported lifecycle or source metadata", handoff.ID)
		}
		if _, err := parseExactVNextRehearsalTimeV1(handoff.CreatedAt, "handoff created_at"); err != nil {
			return nil, fmt.Errorf("export vNext rehearsal archive: handoff %q: %w", handoff.ID, err)
		}
		if handoff.Alias == "" ||
			aliasCreatedAt != handoff.CreatedAt || aliasUpdatedAt != handoff.CreatedAt {
			return nil, fmt.Errorf("export vNext rehearsal archive: handoff %q alias diverges from projection", handoff.ID)
		}
		if bodyKind != ArtifactBodyKindMarkdown || bodySource != "" ||
			bodyCreatedAt != handoff.CreatedAt || bodyUpdatedAt != handoff.CreatedAt ||
			contentHash != artifactBodyHash(handoff.Body) {
			return nil, fmt.Errorf("export vNext rehearsal archive: handoff %q body metadata or SHA-256 diverges", handoff.ID)
		}
		aggregatePayloadBytes += len(handoff.Alias) + len(handoff.HarnessSessionID) + len(handoff.Title) + len(handoff.Body)
		if aggregatePayloadBytes > vnextRehearsalMaxAggregatePayloadBytesV1 {
			return nil, fmt.Errorf("export vNext rehearsal archive: handoff aggregate payload exceeds %d bytes", vnextRehearsalMaxAggregatePayloadBytesV1)
		}
		projection[handoff.ID] = handoff
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("export vNext rehearsal archive: iterate handoff projection: %w", err)
	}
	return projection, nil
}

func validateVNextRehearsalHandoffParityV1(
	fold vnextRehearsalHandoffFoldV1,
	projection map[string]vnextRehearsalHandoffProjectionV1,
) error {
	if len(fold.Facts) != len(projection) || fold.FactRows != len(projection) {
		return fmt.Errorf("export vNext rehearsal archive: selected project handoff fact/projection row counts diverge")
	}
	for handoffID, payload := range fold.Facts {
		projected, found := projection[handoffID]
		if !found || payload.SubjectKind != "handoff" || payload.SubjectID != projected.ID ||
			(payload.Alias != "" && payload.Alias != projected.Alias) || payload.Status != LifecycleStatusDraft || payload.Title != projected.Title ||
			payload.Body != projected.Body || payload.HarnessSessionID != projected.HarnessSessionID || payload.TaskID != "" ||
			payload.CreatedAt != projected.CreatedAt || payload.UpdatedAt != projected.CreatedAt ||
			(payload.Note != "" && payload.Note != "recorded by handoff new") {
			return fmt.Errorf("export vNext rehearsal archive: handoff %q fact/projection/body/alias content diverges", handoffID)
		}
	}
	return nil
}

func buildVNextRehearsalContentV1(
	verification BackupVerificationResult,
	project vnextRehearsalProjectV1,
	fold vnextRehearsalFoldV1,
	projection map[string]vnextRehearsalProjectionV1,
	handoffFold vnextRehearsalHandoffFoldV1,
	handoffProjection map[string]vnextRehearsalHandoffProjectionV1,
	originRows int,
	droppedSpecLinks int,
	droppedTaskLinks int,
) (archive.Content, error) {
	projectObservedAt, err := parseVNextRehearsalTimeV1(project.CreatedAt, "project created_at")
	if err != nil {
		return archive.Content{}, err
	}
	projectID := continuity.ProjectID(project.ID)
	if len(fold.Roots)+len(handoffFold.Roots)+1 > vnextRehearsalMaxRecordsV1 {
		return archive.Content{}, fmt.Errorf("export vNext rehearsal archive: effective record count exceeds %d", vnextRehearsalMaxRecordsV1)
	}
	roots := make([]vnextRehearsalFactRootV1, 0, len(fold.Roots)+len(handoffFold.Roots))
	for _, root := range fold.Roots {
		roots = append(roots, root)
	}
	for _, root := range handoffFold.Roots {
		roots = append(roots, root)
	}
	sort.Slice(roots, func(left, right int) bool {
		return compareFactOrder(
			roots[left].HLC, roots[left].EnvID, roots[left].FactID,
			roots[right].HLC, roots[right].EnvID, roots[right].FactID,
		) < 0
	})
	records := make([]archive.Record, 0, len(roots)+1)
	records = append(records, archive.Record{
		Kind:        archive.RecordProject,
		FactID:      continuity.FactID(vnextRehearsalIDV1("fact_migration_", "project-fact", project.ID)),
		SubjectID:   continuity.SubjectID(projectID),
		Observation: archive.Observation{ObservedAtMillis: projectObservedAt},
		Project:     &archive.ProjectRecord{Label: project.Label},
	})
	for _, root := range roots {
		if root.Family == FactKindHandoffRecorded {
			handoffID := root.SubjectID
			projected := handoffProjection[handoffID]
			observedAt, err := parseExactVNextRehearsalTimeV1(projected.CreatedAt, "handoff created_at")
			if err != nil {
				return archive.Content{}, fmt.Errorf("export vNext rehearsal archive: handoff %q: %w", handoffID, err)
			}
			records = append(records, archive.Record{
				Kind:     archive.RecordHandoff,
				SourceID: projected.Alias,
				FactID: continuity.FactID(vnextRehearsalIDV1(
					"fact_migration_", "handoff-fact", project.ID, handoffID,
				)),
				SubjectID: continuity.SubjectID(vnextRehearsalIDV1(
					"handoff_migration_", "handoff-subject", project.ID, handoffID,
				)),
				Observation: archive.Observation{
					ObservedAtMillis: observedAt,
					HarnessSessionID: projected.HarnessSessionID,
				},
				Handoff: &archive.HandoffRecord{
					Purpose: projected.Title, Situation: projected.Body, SuggestedSkills: []string{},
				},
			})
			continue
		}
		entryID := root.SubjectID
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
	handoffMapping := ""
	if len(handoffProjection) > 0 {
		handoffMapping = archive.HandoffMappingUnparsedLegacyV1
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
			HandoffRows:           len(handoffProjection),
			HandoffMapping:        handoffMapping,
		},
		Project:  archive.ProjectMapping{LegacyProjectID: project.ID, ProjectID: projectID, Label: project.Label},
		Families: archive.FamilyManifest{Project: true, Journal: true, Wrap: true, Handoffs: len(handoffProjection) > 0},
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

func parseExactVNextRehearsalTimeV1(value string, field string) (int64, error) {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return 0, fmt.Errorf("%s is not RFC3339: %w", field, err)
	}
	if !time.UnixMilli(parsed.UnixMilli()).Equal(parsed) {
		return 0, fmt.Errorf("%s is not exactly representable in milliseconds", field)
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
