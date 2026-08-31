package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	"github.com/levifig/loaf/internal/project"
	"github.com/levifig/loaf/internal/state"
	"github.com/levifig/loaf/vnext/continuity"
	continuitysqlite "github.com/levifig/loaf/vnext/continuity/sqlite"
	"github.com/levifig/loaf/vnext/migration/archive"
)

const vnextRehearsalCLIContractVersion = 1

type migrateVNextRehearsalOptions struct {
	backupPath string
	targetRoot string
	jsonOutput bool
}

type migrateVNextRehearsalSourceEvidence struct {
	BackupPath    string `json:"backup_path"`
	SHA256        string `json:"sha256"`
	Bytes         int64  `json:"bytes"`
	SchemaVersion int    `json:"schema_version"`
	Verified      bool   `json:"verified"`
}

type migrateVNextRehearsalDestinationEvidence struct {
	Root          string                   `json:"root"`
	Database      string                   `json:"database"`
	EnvironmentID continuity.EnvironmentID `json:"environment_id"`
}

type migrateVNextRehearsalArchiveEvidence struct {
	Format        string                     `json:"format"`
	Version       int                        `json:"version"`
	ContentSHA256 string                     `json:"content_sha256"`
	RecordCount   int                        `json:"record_count"`
	Families      archive.FamilyManifest     `json:"families"`
	Expected      archive.ProjectionManifest `json:"expected_projection"`
	Actual        archive.ProjectionManifest `json:"actual_projection"`
}

type migrateVNextRehearsalResult struct {
	ContractVersion     int                                      `json:"contract_version"`
	Command             string                                   `json:"command"`
	Mode                string                                   `json:"mode"`
	ProjectID           string                                   `json:"project_id"`
	ProjectRoot         string                                   `json:"project_root"`
	Source              migrateVNextRehearsalSourceEvidence      `json:"source"`
	Destination         migrateVNextRehearsalDestinationEvidence `json:"destination"`
	Archive             migrateVNextRehearsalArchiveEvidence     `json:"archive"`
	ProjectionVerified  bool                                     `json:"projection_verified"`
	Disposable          bool                                     `json:"disposable"`
	ActivationPerformed bool                                     `json:"activation_performed"`
	CutoverReady        bool                                     `json:"cutover_ready"`
	Succeeded           bool                                     `json:"succeeded"`
	ErrorCode           string                                   `json:"error_code,omitempty"`
	Error               string                                   `json:"error,omitempty"`
}

type migrateVNextRehearsalOperations struct {
	newEnvironmentID func() (continuity.EnvironmentID, error)
	open             func(string, continuity.EnvironmentID) (*continuitysqlite.Store, error)
	run              func(context.Context, state.VNextRehearsalExportOptions, *continuitysqlite.Store) (state.VNextRehearsalResult, error)
	close            func(*continuitysqlite.Store) error
}

func writeMigrateVNextRehearsalHelp(out io.Writer) {
	fmt.Fprintln(out, "Usage: loaf migrate vnext-rehearsal --backup <ABSOLUTE-SQLITE> --to <ABSOLUTE-NEW-DIRECTORY> [--json]")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Import verified legacy continuity into a new isolated vNext database without activation or cutover.")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Options:")
	fmt.Fprintln(out, "  --backup <path>  Absolute, clean path to a verified standalone legacy SQLite backup")
	fmt.Fprintln(out, "  --to <directory> Absolute, clean path to a brand-new isolated destination directory")
	fmt.Fprintln(out, "  --json           Output structured rehearsal evidence as JSON")
	fmt.Fprintln(out, "  -h, --help       Show help")
}

func parseMigrateVNextRehearsalArgs(args []string) (migrateVNextRehearsalOptions, error) {
	options := migrateVNextRehearsalOptions{}
	for index := 0; index < len(args); index++ {
		switch args[index] {
		case "--backup", "--to":
			flag := args[index]
			if index+1 >= len(args) || strings.HasPrefix(args[index+1], "-") {
				return options, fmt.Errorf("%s requires a value", flag)
			}
			index++
			value := args[index]
			if flag == "--backup" {
				if options.backupPath != "" {
					return options, fmt.Errorf("--backup may be specified only once")
				}
				options.backupPath = value
			} else {
				if options.targetRoot != "" {
					return options, fmt.Errorf("--to may be specified only once")
				}
				options.targetRoot = value
			}
		case "--json":
			if options.jsonOutput {
				return options, fmt.Errorf("--json may be specified only once")
			}
			options.jsonOutput = true
		default:
			return options, fmt.Errorf("unknown migrate vnext-rehearsal option %q", args[index])
		}
	}
	if options.backupPath == "" {
		return options, fmt.Errorf("--backup is required")
	}
	if options.targetRoot == "" {
		return options, fmt.Errorf("--to is required")
	}
	if err := requireAbsoluteCleanPath("backup", options.backupPath); err != nil {
		return options, err
	}
	if err := requireAbsoluteCleanPath("destination", options.targetRoot); err != nil {
		return options, err
	}
	if options.targetRoot == string(filepath.Separator) {
		return options, fmt.Errorf("destination must not be the filesystem root")
	}
	return options, nil
}

func requireAbsoluteCleanPath(name, path string) error {
	if !filepath.IsAbs(path) {
		return fmt.Errorf("%s path must be absolute", name)
	}
	if path != filepath.Clean(path) {
		return fmt.Errorf("%s path must be clean", name)
	}
	return nil
}

func (r Runner) runMigrateVNextRehearsal(args []string, out io.Writer, runtime state.Runtime) error {
	return r.runMigrateVNextRehearsalWithOperations(args, out, runtime, migrateVNextRehearsalOperations{})
}

func (r Runner) runMigrateVNextRehearsalWithOperations(args []string, out io.Writer, stateRuntime state.Runtime, operations migrateVNextRehearsalOperations) error {
	options, err := parseMigrateVNextRehearsalArgs(args)
	if err != nil {
		return err
	}
	ctx := context.Background()
	root, err := project.ResolveRoot(stateRuntime.RootPath())
	if err != nil {
		return fmt.Errorf("resolve authoritative project root: %w", err)
	}
	resolver := state.PathResolver{StateHome: r.StateHome}
	identity, liveDatabasePath, err := readVNextRehearsalAuthority(ctx, root, resolver)
	if err != nil {
		return err
	}
	if err := validateVNextRehearsalBackupPath(options.backupPath, liveDatabasePath); err != nil {
		return err
	}
	pinnedParent, err := validateVNextRehearsalDestination(options.targetRoot, root.Path(), liveDatabasePath)
	if err != nil {
		return err
	}
	verification, err := state.VerifyBackup(ctx, options.backupPath)
	if err != nil {
		return fmt.Errorf("verify rehearsal backup: %w", err)
	}
	if !verification.Verified || !verification.SQLiteValid || !verification.RecoveryReady {
		return fmt.Errorf("rehearsal backup is not verified and recovery-ready")
	}
	projectKnown := false
	for _, verifiedProject := range verification.Projects {
		if verifiedProject.ID == identity.ID {
			projectKnown = true
			break
		}
	}
	if !projectKnown {
		return fmt.Errorf("authoritative project %q is absent from the verified backup", identity.ID)
	}
	newEnvironmentID := operations.newEnvironmentID
	if newEnvironmentID == nil {
		newEnvironmentID = continuity.NewEnvironmentID
	}
	environmentID, err := newEnvironmentID()
	if err != nil {
		return fmt.Errorf("mint rehearsal environment identity: %w", err)
	}
	if err := os.Mkdir(options.targetRoot, 0o700); err != nil {
		return fmt.Errorf("reserve new rehearsal destination: %w", err)
	}
	evidence := newMigrateVNextRehearsalEvidence(identity.ID, root.Path(), options.targetRoot, verification, environmentID)
	reservedRoot, err := validateReservedVNextRehearsalDestination(options.targetRoot, root.Path(), liveDatabasePath, pinnedParent)
	if err != nil {
		return writeMigrateVNextRehearsalFailure(out, options.jsonOutput, evidence, "reservation_validation_failed", nil, false)
	}
	openDestination := operations.open
	if openDestination == nil {
		openDestination = continuitysqlite.Open
	}
	destination, err := openDestination(options.targetRoot, environmentID)
	if err != nil {
		return writeMigrateVNextRehearsalFailure(out, options.jsonOutput, evidence, "destination_open_failed", nil, false)
	}
	closeDestination := operations.close
	if closeDestination == nil {
		closeDestination = func(store *continuitysqlite.Store) error { return store.Close() }
	}
	if err := validateOpenedVNextRehearsalDestination(options.targetRoot, root.Path(), liveDatabasePath, pinnedParent, reservedRoot); err != nil {
		_ = closeDestination(destination)
		return writeMigrateVNextRehearsalFailure(out, options.jsonOutput, evidence, "destination_validation_failed", nil, false)
	}
	runRehearsal := operations.run
	if runRehearsal == nil {
		runRehearsal = state.RunVNextRehearsal
	}
	rehearsal, runErr := runRehearsal(ctx, state.VNextRehearsalExportOptions{
		Backup:    backupResultFromVerification(verification, liveDatabasePath),
		ProjectID: identity.ID,
		Root:      root,
		Resolver:  resolver,
	}, destination)
	closeErr := closeDestination(destination)
	if runErr != nil {
		return writeMigrateVNextRehearsalFailure(out, options.jsonOutput, evidence, "rehearsal_failed", &rehearsal, false)
	}
	if closeErr != nil {
		return writeMigrateVNextRehearsalFailure(out, options.jsonOutput, evidence, "destination_close_failed", &rehearsal, true)
	}
	sealed, err := archive.Parse(rehearsal.Archive)
	if err != nil {
		return writeMigrateVNextRehearsalFailure(out, options.jsonOutput, evidence, "archive_evidence_failed", &rehearsal, true)
	}
	result := evidence
	result.Archive = migrateVNextRehearsalArchiveEvidence{
		Format: sealed.Format, Version: sealed.Version, ContentSHA256: sealed.ContentSHA256,
		RecordCount: len(sealed.Content.Records), Families: sealed.Content.Families,
		Expected: rehearsal.Report.Expected, Actual: rehearsal.Report.Actual,
	}
	result.ProjectionVerified = rehearsal.Report.Expected == rehearsal.Report.Actual
	result.Succeeded = true
	if options.jsonOutput {
		return writeJSON(out, result)
	}
	writeMigrateVNextRehearsalHuman(out, result)
	return nil
}

func newMigrateVNextRehearsalEvidence(projectID, projectRoot, targetRoot string, verification state.BackupVerificationResult, environmentID continuity.EnvironmentID) migrateVNextRehearsalResult {
	return migrateVNextRehearsalResult{
		ContractVersion: vnextRehearsalCLIContractVersion,
		Command:         "migrate vnext-rehearsal",
		Mode:            "isolated-rehearsal",
		ProjectID:       projectID,
		ProjectRoot:     projectRoot,
		Source: migrateVNextRehearsalSourceEvidence{
			BackupPath: verification.BackupPath, SHA256: verification.SHA256, Bytes: verification.Bytes,
			SchemaVersion: verification.SchemaVersion, Verified: verification.Verified,
		},
		Destination: migrateVNextRehearsalDestinationEvidence{
			Root: targetRoot, Database: filepath.Join(targetRoot, "vnext", "continuity.sqlite"), EnvironmentID: environmentID,
		},
		Disposable: true, ActivationPerformed: false, CutoverReady: false,
	}
}

func writeMigrateVNextRehearsalFailure(out io.Writer, jsonOutput bool, result migrateVNextRehearsalResult, stageCode string, rehearsal *state.VNextRehearsalResult, projectionKnown bool) error {
	if rehearsal != nil {
		if sealed, err := archive.Parse(rehearsal.Archive); err == nil {
			result.Archive = migrateVNextRehearsalArchiveEvidence{
				Format: sealed.Format, Version: sealed.Version, ContentSHA256: sealed.ContentSHA256,
				RecordCount: len(sealed.Content.Records), Families: sealed.Content.Families,
				Expected: sealed.Content.Expected,
			}
		}
		if projectionKnown {
			result.Archive.Format = rehearsal.Report.Format
			result.Archive.Version = rehearsal.Report.Version
			result.Archive.ContentSHA256 = rehearsal.Report.ContentSHA256
			result.Archive.RecordCount = rehearsal.Report.RecordCount
			result.Archive.Expected = rehearsal.Report.Expected
			result.Archive.Actual = rehearsal.Report.Actual
			result.ProjectionVerified = rehearsal.Report.Expected == rehearsal.Report.Actual
		}
	}
	result.ErrorCode = stageCode
	result.Error = migrateVNextRehearsalFailureMessage(stageCode)
	if !jsonOutput {
		return fmt.Errorf("%s; destination preserved for inspection at %s", result.Error, result.Destination.Root)
	}
	if err := writeJSON(out, result); err != nil {
		return errors.Join(ExitError{Code: 1}, err)
	}
	return ExitError{Code: 1}
}

func migrateVNextRehearsalFailureMessage(stageCode string) string {
	switch stageCode {
	case "reservation_validation_failed":
		return "reserved destination failed physical safety validation"
	case "destination_open_failed":
		return "isolated destination could not be opened"
	case "destination_validation_failed":
		return "opened destination failed pinned identity or containment validation"
	case "rehearsal_failed":
		return "isolated continuity rehearsal failed"
	case "destination_close_failed":
		return "completed rehearsal destination could not be closed cleanly"
	case "archive_evidence_failed":
		return "completed rehearsal archive evidence could not be validated"
	default:
		return "isolated vNext rehearsal failed after destination reservation"
	}
}

func readVNextRehearsalAuthority(ctx context.Context, root project.Root, resolver state.PathResolver) (state.ProjectIdentity, string, error) {
	databasePath, err := resolver.DatabasePath(root)
	if err != nil {
		return state.ProjectIdentity{}, "", fmt.Errorf("resolve authoritative live database: %w", err)
	}
	store, err := state.OpenStoreReadOnly(databasePath)
	if err != nil {
		return state.ProjectIdentity{}, "", fmt.Errorf("open authoritative live database read-only: %w", err)
	}
	identity, readErr := store.LookupProjectIdentityForRoot(ctx, root)
	closeErr := store.Close()
	if readErr != nil || closeErr != nil {
		return state.ProjectIdentity{}, "", errors.Join(readErr, closeErr)
	}
	return identity, databasePath, nil
}

func backupResultFromVerification(verification state.BackupVerificationResult, liveDatabasePath string) state.BackupResult {
	return state.BackupResult{
		ContractVersion: verification.ContractVersion, DatabaseScope: verification.DatabaseScope,
		DatabasePath: liveDatabasePath, BackupPath: verification.BackupPath, Bytes: verification.Bytes,
		SHA256: verification.SHA256, Verified: verification.Verified, SchemaVersion: verification.SchemaVersion,
		ProjectCount: verification.ProjectCount, IntegrityCheck: verification.IntegrityCheck,
		ForeignKeyCheck: verification.ForeignKeyCheck, JournalRetrievalReady: verification.JournalRetrievalReady,
		JournalSearchParity: verification.JournalSearchParity, JournalProvenanceIntegrity: verification.JournalProvenanceIntegrity,
		SQLiteValid: verification.SQLiteValid, RecoveryReady: verification.RecoveryReady, JournalWatermark: verification.JournalWatermark,
	}
}

func validateVNextRehearsalBackupPath(path, liveDatabasePath string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect rehearsal backup: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return fmt.Errorf("rehearsal backup must be a regular file, not a symlink")
	}
	liveInfo, err := os.Stat(liveDatabasePath)
	if err != nil {
		return fmt.Errorf("inspect authoritative live database: %w", err)
	}
	if os.SameFile(info, liveInfo) {
		return fmt.Errorf("rehearsal source must be a standalone backup, not the live database or an alias")
	}
	return nil
}

func validateVNextRehearsalDestination(path, projectRoot, liveDatabasePath string) (os.FileInfo, error) {
	if _, err := os.Lstat(path); err == nil {
		return nil, fmt.Errorf("rehearsal destination already exists")
	} else if !os.IsNotExist(err) && !errors.Is(err, syscall.ENOTDIR) {
		return nil, fmt.Errorf("inspect rehearsal destination: %w", err)
	}
	parent := filepath.Dir(path)
	parentInfo, err := os.Lstat(parent)
	if err != nil {
		return nil, fmt.Errorf("rehearsal destination parent must already exist: %w", err)
	}
	if parentInfo.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("rehearsal destination has a symlink parent: %s", parent)
	}
	if !parentInfo.IsDir() {
		return nil, fmt.Errorf("rehearsal destination has a non-directory parent: %s", parent)
	}
	if err := continuitysqlite.ValidateStateRootLocation(path); err != nil {
		return nil, fmt.Errorf("validate rehearsal destination location: %w", err)
	}
	resolvedProjectRoot, err := filepath.EvalSymlinks(projectRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve authoritative project root: %w", err)
	}
	resolvedLiveDatabase, err := filepath.EvalSymlinks(liveDatabasePath)
	if err != nil {
		return nil, fmt.Errorf("resolve authoritative live database: %w", err)
	}
	resolvedParent, err := filepath.EvalSymlinks(parent)
	if err != nil {
		return nil, fmt.Errorf("resolve rehearsal destination parent: %w", err)
	}
	resolvedDestination := filepath.Join(resolvedParent, filepath.Base(path))
	if pathContains(resolvedProjectRoot, resolvedDestination) {
		return nil, fmt.Errorf("rehearsal destination must be outside the authoritative project root")
	}
	liveTree := filepath.Dir(resolvedLiveDatabase)
	if pathContains(liveTree, resolvedDestination) || pathContains(resolvedDestination, liveTree) {
		return nil, fmt.Errorf("rehearsal destination must not overlap the authoritative live state tree")
	}
	return parentInfo, nil
}

func validateReservedVNextRehearsalDestination(path, projectRoot, liveDatabasePath string, pinnedParent os.FileInfo) (os.FileInfo, error) {
	parentInfo, err := os.Lstat(filepath.Dir(path))
	if err != nil {
		return nil, fmt.Errorf("reinspect rehearsal destination parent: %w", err)
	}
	if pinnedParent == nil || !os.SameFile(pinnedParent, parentInfo) {
		return nil, fmt.Errorf("rehearsal destination parent changed during reservation")
	}
	rootInfo, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("inspect reserved rehearsal destination: %w", err)
	}
	if rootInfo.Mode()&os.ModeSymlink != 0 || !rootInfo.IsDir() {
		return nil, fmt.Errorf("reserved rehearsal destination must be a real directory")
	}
	if runtime.GOOS != "windows" && rootInfo.Mode().Perm() != 0o700 {
		return nil, fmt.Errorf("reserved rehearsal destination must have mode 0700")
	}
	if err := continuitysqlite.ValidateStateRootLocation(path); err != nil {
		return nil, fmt.Errorf("revalidate reserved rehearsal destination location: %w", err)
	}
	resolvedDestination, err := filepath.EvalSymlinks(path)
	if err != nil {
		return nil, fmt.Errorf("resolve reserved rehearsal destination: %w", err)
	}
	resolvedProjectRoot, err := filepath.EvalSymlinks(projectRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve authoritative project root: %w", err)
	}
	resolvedLiveDatabase, err := filepath.EvalSymlinks(liveDatabasePath)
	if err != nil {
		return nil, fmt.Errorf("resolve authoritative live database: %w", err)
	}
	liveTree := filepath.Dir(resolvedLiveDatabase)
	if pathContains(resolvedProjectRoot, resolvedDestination) || pathContains(liveTree, resolvedDestination) || pathContains(resolvedDestination, liveTree) {
		return nil, fmt.Errorf("reserved rehearsal destination crossed an authority containment boundary")
	}
	return rootInfo, nil
}

func validateOpenedVNextRehearsalDestination(path, projectRoot, liveDatabasePath string, pinnedParent, pinnedRoot os.FileInfo) error {
	if err := continuitysqlite.ValidateStateRootLocation(path); err != nil {
		return fmt.Errorf("revalidate opened rehearsal destination location: %w", err)
	}
	parentInfo, err := os.Lstat(filepath.Dir(path))
	if err != nil {
		return fmt.Errorf("reinspect opened rehearsal destination parent: %w", err)
	}
	if pinnedParent == nil || !os.SameFile(pinnedParent, parentInfo) {
		return fmt.Errorf("rehearsal destination parent changed while opening")
	}
	rootInfo, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("reinspect opened rehearsal destination: %w", err)
	}
	if pinnedRoot == nil || !os.SameFile(pinnedRoot, rootInfo) {
		return fmt.Errorf("rehearsal destination root changed while opening")
	}
	privateDirectory := filepath.Join(path, "vnext")
	privateInfo, err := os.Lstat(privateDirectory)
	if err != nil {
		return fmt.Errorf("inspect opened continuity directory: %w", err)
	}
	if privateInfo.Mode()&os.ModeSymlink != 0 || !privateInfo.IsDir() {
		return fmt.Errorf("opened continuity directory must be a real directory")
	}
	databasePath := filepath.Join(privateDirectory, "continuity.sqlite")
	databaseInfo, err := os.Lstat(databasePath)
	if err != nil {
		return fmt.Errorf("inspect opened continuity database: %w", err)
	}
	if databaseInfo.Mode()&os.ModeSymlink != 0 || !databaseInfo.Mode().IsRegular() {
		return fmt.Errorf("opened continuity database must be a real file")
	}
	resolvedRoot, err := filepath.EvalSymlinks(path)
	if err != nil {
		return fmt.Errorf("resolve opened rehearsal destination: %w", err)
	}
	resolvedPrivateDirectory, err := filepath.EvalSymlinks(privateDirectory)
	if err != nil {
		return fmt.Errorf("resolve opened continuity directory: %w", err)
	}
	resolvedDatabase, err := filepath.EvalSymlinks(databasePath)
	if err != nil {
		return fmt.Errorf("resolve opened continuity database: %w", err)
	}
	if !pathsEqual(filepath.Dir(resolvedPrivateDirectory), resolvedRoot) || !pathsEqual(filepath.Dir(resolvedDatabase), resolvedPrivateDirectory) {
		return fmt.Errorf("opened continuity database escaped the reserved rehearsal destination")
	}
	resolvedProjectRoot, err := filepath.EvalSymlinks(projectRoot)
	if err != nil {
		return fmt.Errorf("resolve authoritative project root after open: %w", err)
	}
	resolvedLiveDatabase, err := filepath.EvalSymlinks(liveDatabasePath)
	if err != nil {
		return fmt.Errorf("resolve authoritative live database after open: %w", err)
	}
	liveTree := filepath.Dir(resolvedLiveDatabase)
	if pathContains(resolvedProjectRoot, resolvedRoot) || pathContains(liveTree, resolvedRoot) || pathContains(resolvedRoot, liveTree) {
		return fmt.Errorf("opened rehearsal destination crossed an authority containment boundary")
	}
	return nil
}

func pathContains(root, candidate string) bool {
	if runtime.GOOS == "windows" {
		if !strings.EqualFold(filepath.VolumeName(root), filepath.VolumeName(candidate)) {
			return false
		}
		root = strings.ToLower(root)
		candidate = strings.ToLower(candidate)
	}
	relative, err := filepath.Rel(filepath.Clean(root), filepath.Clean(candidate))
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func pathsEqual(left, right string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(filepath.Clean(left), filepath.Clean(right))
	}
	return filepath.Clean(left) == filepath.Clean(right)
}

func writeMigrateVNextRehearsalHuman(out io.Writer, result migrateVNextRehearsalResult) {
	fmt.Fprintln(out, "Verified isolated vNext rehearsal completed; no activation or cutover was performed.")
	fmt.Fprintf(out, "project: %s\n", result.ProjectID)
	fmt.Fprintf(out, "project root: %s\n", result.ProjectRoot)
	fmt.Fprintf(out, "source backup: %s\n", result.Source.BackupPath)
	fmt.Fprintf(out, "source sha256: %s\n", result.Source.SHA256)
	fmt.Fprintf(out, "source bytes: %d\n", result.Source.Bytes)
	fmt.Fprintf(out, "source schema: %d\n", result.Source.SchemaVersion)
	fmt.Fprintf(out, "destination root: %s\n", result.Destination.Root)
	fmt.Fprintf(out, "destination database: %s\n", result.Destination.Database)
	fmt.Fprintf(out, "environment: %s\n", result.Destination.EnvironmentID)
	fmt.Fprintf(out, "archive: %s v%d (%s, %d records)\n", result.Archive.Format, result.Archive.Version, result.Archive.ContentSHA256, result.Archive.RecordCount)
	fmt.Fprintf(out, "projection verified: %t\n", result.ProjectionVerified)
	fmt.Fprintln(out, "disposable: true")
	fmt.Fprintln(out, "activation performed: false")
	fmt.Fprintln(out, "cutover ready: false")
}
