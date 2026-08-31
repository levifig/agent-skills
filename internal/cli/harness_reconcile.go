package cli

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	harnessReconcileReceiptFile = ".loaf-reconcile-receipt.json"
	harnessReconcileLockFile    = ".loaf-reconcile.lock"
)

var managedContentLockWait = hookFileLockWait

func managedContentLockDir(home string) string {
	return filepath.Join(home, ".agents")
}

type harnessReconcileOptions struct {
	target string
	json   bool
	help   bool
}

type harnessReconcileReceipt struct {
	ContractVersion int                      `json:"contract_version"`
	Command         string                   `json:"command"`
	Target          string                   `json:"target"`
	FromVersion     string                   `json:"from_version,omitempty"`
	ToVersion       string                   `json:"to_version"`
	Outcome         string                   `json:"outcome"`
	Checked         []string                 `json:"checked"`
	UpdatedTargets  []string                 `json:"updated_targets"`
	Cohort          []harnessReconcileTarget `json:"cohort"`
	Message         string                   `json:"message,omitempty"`
	RecordedAt      string                   `json:"recorded_at,omitempty"`
	RestartRequired bool                     `json:"restart_required"`
}

type harnessReconcileTarget struct {
	Target      string `json:"target"`
	FromVersion string `json:"from_version,omitempty"`
	ToVersion   string `json:"to_version"`
	Outcome     string `json:"outcome"`
}

func (r Runner) runHarness(args []string, out io.Writer, runtimeRoot string) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		writeHarnessHelp(out)
		return nil
	}
	if args[0] != "reconcile" {
		return fmt.Errorf("unknown harness subcommand %q", args[0])
	}
	options, err := parseHarnessReconcileArgs(args[1:])
	if err != nil {
		return err
	}
	if options.help {
		writeHarnessReconcileHelp(out)
		return nil
	}
	receipt, err := r.reconcileManagedHarness(options.target, runtimeRoot)
	if err != nil {
		return err
	}
	if options.json {
		return writeJSON(out, receipt)
	}
	fmt.Fprintf(out, "Loaf harness content: %s (%s → %s)\n", receipt.Outcome, emptyVersion(receipt.FromVersion), receipt.ToVersion)
	if receipt.Message != "" {
		fmt.Fprintln(out, receipt.Message)
	}
	return nil
}

func parseHarnessReconcileArgs(args []string) (harnessReconcileOptions, error) {
	var options harnessReconcileOptions
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--target":
			if i+1 >= len(args) {
				return options, fmt.Errorf("--target requires a value")
			}
			i++
			options.target = args[i]
		case "--json":
			options.json = true
		case "--help", "-h":
			options.help = true
		default:
			return options, fmt.Errorf("unknown harness reconcile option %q", args[i])
		}
	}
	if !options.help && options.target == "" {
		return options, fmt.Errorf("--target is required")
	}
	if options.target != "" && !isValidInstallTarget(options.target) {
		return options, fmt.Errorf("unknown harness target %q", options.target)
	}
	return options, nil
}

func writeHarnessHelp(out io.Writer) {
	fmt.Fprintln(out, "Usage: loaf harness reconcile --target <target> [--json]")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Reconcile Loaf-owned harness content to the running CLI distribution.")
}

func writeHarnessReconcileHelp(out io.Writer) {
	writeHarnessHelp(out)
}

func emptyVersion(value string) string {
	if value == "" {
		return "unknown"
	}
	return value
}

func (r Runner) reconcileManagedHarness(target string, runtimeRoot string) (harnessReconcileReceipt, error) {
	loafRoot, err := r.resolveInstalledDistributionRoot()
	if err != nil {
		return harnessReconcileReceipt{}, err
	}
	version := packageVersion(loafRoot)
	// Resolve the same global-or-project install layout used by install and
	// upgrade. In project-bound cloud environments this updates only Loaf-owned
	// harness artifacts and skills; it never writes project config or human
	// instructions.
	configDirs, home := resolveInstallLayout(runtimeRoot)
	tools := detectInstallToolsForProject(runtimeRoot)
	configDir := configDirs[target]
	targetOption := targetInstallOptions{
		Target: target, DistDir: filepath.Join(loafRoot, "dist", target), ConfigDir: configDir,
		Upgrade: true, Version: version, HomeDir: home, CodexHome: resolveInstallCodexHome(configDir),
		ProjectRoot: runtimeRoot, SkipSkillsSync: true,
	}
	cohortOptions := reconcileSkillsSourceOptions(targetOption, loafRoot, home, runtimeRoot, tools)
	receipt := harnessReconcileReceipt{
		ContractVersion: 1,
		Command:         "harness reconcile",
		Target:          target,
		FromVersion:     readHarnessVersionMarker(configDir),
		ToVersion:       version,
		Checked:         []string{"managed-skills", "target-adapters", "version-markers", "install-records"},
		UpdatedTargets:  []string{},
		Cohort:          []harnessReconcileTarget{},
	}
	staleOptions, cohort, cohortState, err := classifyHarnessReconcileCohort(cohortOptions, version)
	receipt.Cohort = cohort
	if err != nil {
		return harnessReconcileReceipt{}, err
	}
	switch cohortState {
	case harnessDriftCurrent:
		receipt.Outcome = "current"
		receipt.Message = "Managed harness content already matches the running Loaf CLI across the installed shared-skills cohort."
		return receipt, nil
	case harnessDriftBinaryStale:
		receipt.Outcome = "binary-stale"
		receipt.Message = "Installed content is newer than this CLI; refusing to downgrade it. Upgrade the Loaf CLI, then retry."
		return receipt, nil
	case harnessDriftUnknown:
		receipt.Outcome = "unknown"
		receipt.Message = "Installed content version is missing or unreadable; refusing to claim or overwrite it. Run loaf doctor."
		return receipt, nil
	}

	lock, err := acquireHarnessReconcileLock(managedContentLockDir(home), managedContentLockWait)
	if err != nil {
		return harnessReconcileReceipt{}, err
	}
	defer lock.release()

	// A second process may have converged the cohort while this process waited.
	receipt.FromVersion = readHarnessVersionMarker(configDir)
	staleOptions, receipt.Cohort, cohortState, err = classifyHarnessReconcileCohort(cohortOptions, version)
	if err != nil {
		return harnessReconcileReceipt{}, err
	}
	if cohortState == harnessDriftCurrent {
		receipt.Outcome = "current"
		receipt.Message = "Another Loaf process already reconciled the installed shared-skills cohort."
		return receipt, nil
	}
	if cohortState != harnessDriftContentStale {
		return harnessReconcileReceipt{}, fmt.Errorf("harness content changed while waiting for the reconcile lock; run `loaf doctor`")
	}

	for _, option := range staleOptions {
		if !dirExistsForInstall(option.DistDir) || !fileExistsForInstall(filepath.Join(option.DistDir, targetBuildManifestFile)) {
			return harnessReconcileReceipt{}, fmt.Errorf("running Loaf distribution has no verified %s target content; reinstall the CLI", option.Target)
		}
		if !fileExistsForInstall(filepath.Join(option.ConfigDir, targetInstallManifestFile)) {
			return harnessReconcileReceipt{}, fmt.Errorf("installed %s content has no Loaf ownership manifest; refusing automatic overwrite; run `loaf upgrade --to %s`", option.Target, option.Target)
		}
	}

	hookState, releaseHookState := r.hookStateForApply(runtimeRoot)
	defer releaseHookState()
	for i := range cohortOptions {
		cohortOptions[i].HookState = hookState
	}
	skillsErr := syncCanonicalManagedSkills(cohortOptions)
	var skillConflicts *skillSyncConflictsError
	if skillsErr != nil && !errors.As(skillsErr, &skillConflicts) {
		return harnessReconcileReceipt{}, skillsErr
	}
	if skillConflicts != nil {
		// The skills sync can update non-conflicting skills before reporting the
		// owned paths it preserved. Keep the adapter and marker stale so the next
		// startup retries after those conflicts are resolved.
		return harnessReconcileReceipt{}, skillConflicts
	}
	for _, option := range staleOptions {
		option.HookState = hookState
		if err := installTargetDistribution(option); err != nil {
			return harnessReconcileReceipt{}, err
		}
		receipt.UpdatedTargets = append(receipt.UpdatedTargets, option.Target)
	}

	receipt.Outcome = "updated"
	receipt.RestartRequired = true
	receipt.Message = "Reconciled Loaf-owned content for the installed shared-skills harness cohort from the already-installed CLI distribution; no project configuration, human-authored instructions, connector, authentication material, or tracker state was changed. A harness may have loaded previous bytes already, so start new sessions in the updated targets."
	receipt.RecordedAt = time.Now().UTC().Format(time.RFC3339)
	if err := writeHarnessReconcileReceipt(configDir, receipt); err != nil {
		return harnessReconcileReceipt{}, err
	}
	return receipt, nil
}

func classifyHarnessReconcileCohort(options []targetInstallOptions, version string) ([]targetInstallOptions, []harnessReconcileTarget, harnessDriftState, error) {
	var stale []targetInstallOptions
	cohort := make([]harnessReconcileTarget, 0, len(options))
	state := harnessDriftCurrent
	for _, option := range options {
		from := readHarnessVersionMarker(option.ConfigDir)
		reading := classifyHarnessDrift(from, version)
		cohort = append(cohort, harnessReconcileTarget{Target: option.Target, FromVersion: from, ToVersion: version, Outcome: string(reading)})
		switch reading {
		case harnessDriftContentStale:
			stale = append(stale, option)
			state = harnessDriftContentStale
		case harnessDriftBinaryStale:
			return nil, cohort, harnessDriftBinaryStale, nil
		case harnessDriftUnknown:
			return nil, cohort, harnessDriftUnknown, nil
		}
	}
	return stale, cohort, state, nil
}

// reconcileSkillsSourceOptions preserves the canonical shared-skills shape
// selected by install/upgrade. In particular, OpenCode owns frontmatter keys
// that Amp does not; reconciling Amp alone must not strip those keys while the
// OpenCode adapter remains stamped current.
func reconcileSkillsSourceOptions(targetOption targetInstallOptions, loafRoot string, home string, projectRoot string, tools []detectedInstallTool) []targetInstallOptions {
	options := []targetInstallOptions{targetOption}
	seen := map[string]bool{targetOption.Target: true}
	for _, tool := range tools {
		if !tool.installed || seen[tool.key] || !isValidInstallTarget(tool.key) {
			continue
		}
		distDir := filepath.Join(loafRoot, "dist", tool.key)
		if !dirExistsForInstall(filepath.Join(distDir, "skills")) {
			continue
		}
		seen[tool.key] = true
		options = append(options, targetInstallOptions{
			Target:         tool.key,
			DistDir:        distDir,
			ConfigDir:      tool.configDir,
			Version:        targetOption.Version,
			HomeDir:        home,
			CodexHome:      resolveInstallCodexHome(tool.configDir),
			ProjectRoot:    projectRoot,
			SkipSkillsSync: true,
			HookState:      targetOption.HookState,
		})
	}
	return options
}

type harnessReconcileLock struct {
	path string
	file *os.File
}

// acquireHarnessReconcileLock serializes the whole managed-content cohort.
// Unix ownership lives in the kernel and survives an old metadata timestamp;
// a crashed holder releases automatically without any pathname reclamation.
// Platforms without that primitive use an exclusive-create lock and never
// guess staleness, preserving exclusivity at the cost of manual crash cleanup.
func acquireHarnessReconcileLock(configDir string, wait time.Duration) (*harnessReconcileLock, error) {
	if wait <= 0 {
		wait = hookFileLockWait
	}
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return nil, err
	}
	path := filepath.Join(configDir, harnessReconcileLockFile)
	tokenBytes := make([]byte, 16)
	if _, err := rand.Read(tokenBytes); err != nil {
		return nil, fmt.Errorf("create harness reconcile lock token: %w", err)
	}
	token := fmt.Sprintf("%x", tokenBytes)
	deadline := time.Now().Add(wait)
	for {
		file, acquired, err := tryAcquireHarnessMutationFile(path)
		if err != nil {
			return nil, fmt.Errorf("take harness reconcile lock %s: %w", path, err)
		}
		if acquired {
			lock := &harnessReconcileLock{path: path, file: file}
			if err := file.Truncate(0); err != nil {
				return nil, fmt.Errorf("reset harness reconcile lock %s: %w", path, firstHookLockError(err, lock.release()))
			}
			if _, err := file.Seek(0, io.SeekStart); err != nil {
				return nil, fmt.Errorf("seek harness reconcile lock %s: %w", path, firstHookLockError(err, lock.release()))
			}
			_, writeErr := fmt.Fprintf(file, "token %s\npid %d\nsince %s\n", token, os.Getpid(), time.Now().UTC().Format(time.RFC3339))
			syncErr := file.Sync()
			if writeErr != nil || syncErr != nil {
				return nil, fmt.Errorf("record harness reconcile lock %s: %w", path, firstHookLockError(writeErr, syncErr, lock.release()))
			}
			return lock, nil
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("another Loaf process is updating managed harness content in %s (%s); %s", configDir, strings.TrimSpace(describeHookFileLockHolder(path)), harnessMutationLockContentionAdvice(path))
		}
		time.Sleep(hookFileLockPoll)
	}
}

func (lock *harnessReconcileLock) release() error {
	if lock == nil {
		return nil
	}
	return releaseHarnessMutationFile(lock.path, lock.file)
}

func writeHarnessReconcileReceipt(configDir string, receipt harnessReconcileReceipt) error {
	body, err := json.MarshalIndent(receipt, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	return writeFileAtomically(filepath.Join(configDir, harnessReconcileReceiptFile), body, 0o644)
}
