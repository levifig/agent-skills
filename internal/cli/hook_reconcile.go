package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"time"

	"github.com/levifig/loaf/internal/state"
)

// hook_reconcile.go converges Loaf's own entries inside a hooks file that Loaf
// does not own. Every entry the recognition predicate does not claim is
// invisible: it is never classified, never rewritten, and its presence or
// absence changes nothing. What Loaf's entries should look like comes from the
// built catalog; whether they should be there at all comes from the user-scoped
// enablement records. The file is the projection of those two, never an
// authority over either — which is why no divergence between the file and any
// recorded fingerprint can refuse anything here.

// The action vocabulary the plan surface speaks. These name what happens to one
// identity, not to the file.
const (
	hookActionAdd    = "add"
	hookActionUpdate = "update"
	hookActionRemove = "remove"
	hookActionAbsorb = "absorb"
)

// hookAction is one per-entry decision. Entries that pair to no current
// identity carry their file position instead of a hook id, because a retired
// generation has no name this version still knows.
type hookAction struct {
	action string
	event  string
	hookID string
	index  int
	detail string
}

func (a hookAction) id() string {
	if a.hookID != "" {
		return "hook:" + a.event + "/" + a.hookID
	}
	return fmt.Sprintf("hook:%s[%d]", a.event, a.index)
}

// hookReconcileOperations injects failures at the two points Decision 10 names
// as windows: between the record commit and the file projection, and between
// staging the new bytes and comparing the destination.
type hookReconcileOperations struct {
	afterRecords func() error
	beforeRename func() error
}

type hookReconciler struct {
	target  string
	path    string
	catalog hookCatalog
	state   hookStateResolver
	version string
	// priorManifest records that an installed manifest existed at all, and
	// priorVersion what it named. The pair is what bounds absorption: only a
	// version this catalog enumerates has a known cohort, and only the absence
	// of a manifest licenses falling back to the full frozen one.
	priorManifest bool
	priorVersion  string
	// priorProjection records that the installed manifest carried a
	// hook-projection row — the manifest-side half of prior-install detection.
	priorProjection bool
	managedPaths    []string
	homeDir         string
	goos            string
	operations      *hookReconcileOperations
	lockWait        time.Duration

	resolveExecutable  func() (string, error)
	executable         string
	executableErr      error
	executableResolved bool

	// lock is held from the first state read through file publication; recorded
	// carries the actions the record half already took, so the report a caller
	// finally sees names them alongside the file half's.
	lock     *hookFileLock
	recorded []hookAction
}

// hookProjection is one computed convergence: the actions to report, the
// records to write, and the bytes to publish. It is display and intent, never a
// script — apply recomputes it from live state inside the lock.
//
// foreign holds every entry Loaf does not own, canonicalized and in file order,
// so the post-verify comparison is about values and positions rather than a
// count that two compensating mistakes could satisfy.
type hookProjection struct {
	actions       []hookAction
	absorbed      []state.HookEnablementRef
	writeMarker   bool
	markerVersion string
	file          hookFile
	body          []byte
	foreign       []string
}

// newHookReconciler builds the reconciler for a target that keeps its hooks in
// a shared JSON file. Targets without one, and distributions old enough to
// predate the target adapter manifest — and therefore the catalog the whole
// identity model rests on — return no reconciler and keep their own paths.
//
// Callers build it before syncing the adapter manifest and apply it after:
// prior-install detection reads the manifest the previous release wrote, and
// the sync is what replaces that with this release's.
func newHookReconciler(options targetInstallOptions) (*hookReconciler, error) {
	path := targetHookFilePath(options)
	if path == "" {
		return nil, nil
	}
	manifestPath := filepath.Join(options.DistDir, targetBuildManifestFile)
	if !fileExistsForInstall(manifestPath) {
		return nil, staleHookDistributionError(options.Target, options.DistDir, "it carries no target adapter manifest")
	}
	catalog, err := readHookCatalog(options.DistDir)
	if err != nil {
		return nil, err
	}
	if catalog.Target != options.Target {
		return nil, fmt.Errorf("hook catalog target %q does not match install target %q", catalog.Target, options.Target)
	}
	installed, hasInstalled, err := readInstalledHookManifest(options)
	if err != nil {
		return nil, err
	}
	desired, err := readTargetAdapterManifest(manifestPath)
	if err != nil {
		return nil, err
	}
	// The catalog and the manifest are emitted by one build. If they disagree
	// about which release this is, one of them was left behind, and neither the
	// desired entries nor the cohort bound can be trusted to describe what is
	// about to be installed.
	if catalog.PackageVersion != desired.PackageVersion {
		return nil, staleHookDistributionError(options.Target, options.DistDir,
			fmt.Sprintf("its hook catalog is built from %s while its target manifest is built from %s", catalog.PackageVersion, desired.PackageVersion))
	}
	// Recognition claims a path-backed entry only through an exact recorded
	// destination. Both manifests contribute: the installed one names what the
	// entries in the file today were written against, and the desired one names
	// what this version is about to write, so a hook whose script is new to this
	// release is recognizable the moment it is added.
	root := hookArtifactRoot(options)
	managed := append(hookManagedDestinations(root, installed), hookManagedDestinations(root, desired)...)

	reconciler := &hookReconciler{
		target:          options.Target,
		path:            path,
		catalog:         catalog,
		state:           options.HookState,
		version:         options.Version,
		priorManifest:   hasInstalled,
		priorVersion:    installed.PackageVersion,
		priorProjection: manifestCarriesHookProjection(installed),
		managedPaths:    managed,
		homeDir:         installHomeDir(options),
		goos:            runtime.GOOS,
		operations:      options.HookOps,
		resolveExecutable: func() (string, error) {
			return trustedCodexJournalExecutable(options.ProjectRoot, options.CodexRuleOperations)
		},
	}
	return reconciler, nil
}

// staleHookDistributionError is what a target gets instead of the whole-file
// merge that used to stand behind it. That merge trusted a marker, wrote the
// hooks file before the scripts it referenced, and refused files it could not
// classify — all of which this Change removes, so reaching it from a build
// output that predates the catalog would reintroduce every one of them.
func staleHookDistributionError(target string, distDir string, because string) error {
	return fmt.Errorf("cannot reconcile %s hooks: the build output at %s is stale because %s; rebuild with `loaf build` or install a current release", target, distDir, because)
}

// targetHookFilePath is the one place that knows where a target keeps the file
// its entries live in. An empty result means the target has no such file.
func targetHookFilePath(options targetInstallOptions) string {
	switch options.Target {
	case "cursor", "codex":
		return filepath.Join(hookArtifactRoot(options), "hooks.json")
	default:
		return ""
	}
}

func hookArtifactRoot(options targetInstallOptions) string {
	if options.Target != "codex" {
		return options.ConfigDir
	}
	if options.CodexHome != "" {
		return options.CodexHome
	}
	return filepath.Join(installHomeDir(options), ".codex")
}

// targetReconcilesHookEntries reports whether this target's hooks file is
// converged per entry. The whole-file artifact machinery must leave those
// destinations alone: reading, publishing, removing, or judging one would
// reintroduce exactly the file-level verdict this replaces.
func targetReconcilesHookEntries(options targetInstallOptions) bool {
	return targetHookFilePath(options) != ""
}

func readInstalledHookManifest(options targetInstallOptions) (targetAdapterManifest, bool, error) {
	path := filepath.Join(options.ConfigDir, targetInstallManifestFile)
	if _, err := os.Lstat(path); err != nil {
		if os.IsNotExist(err) {
			return targetAdapterManifest{}, false, nil
		}
		return targetAdapterManifest{}, false, err
	}
	manifest, err := readTargetAdapterManifest(path)
	if err != nil {
		return targetAdapterManifest{}, false, err
	}
	return manifest, true, nil
}

func manifestCarriesHookProjection(manifest targetAdapterManifest) bool {
	for _, artifact := range manifest.Artifacts {
		if artifact.Kind == "hook-projection" {
			return true
		}
	}
	return false
}

// openState reaches the enablement authority. A caller that supplied no
// resolver gets an error rather than an empty view: projecting defaults because
// the records could not be read is exactly how a disabled hook comes back.
func (r *hookReconciler) openState() (*state.Store, error) {
	if r.state == nil {
		return nil, fmt.Errorf("cannot reconcile %s: hook enablement state is unavailable", r.path)
	}
	return r.state()
}

// plan computes the per-entry actions without writing anything — no records, no
// file, no lock. What it reports is what an apply run would decide against the
// state that exists right now, which is all a plan can honestly promise.
func (r *hookReconciler) plan(ctx context.Context) ([]hookAction, error) {
	store, err := r.openState()
	if err != nil {
		return nil, err
	}
	records, err := loadHookRecords(ctx, store, r.target)
	if err != nil {
		return nil, err
	}
	projection, err := r.computeProjection(records)
	if err != nil {
		return nil, err
	}
	return projection.actions, nil
}

// apply converges the target in one call: the record half, then the file half.
// Install and upgrade split the two around their artifact work; everything else
// wants them adjacent.
func (r *hookReconciler) apply(ctx context.Context) ([]hookAction, error) {
	if err := r.begin(ctx); err != nil {
		return nil, err
	}
	return r.complete(ctx)
}

// begin takes the per-target lock and makes the record half durable. The lock
// comes first — before any state is opened or read — because Decision 10's
// guarantee is over the whole read-compute-record-project sequence, and a read
// that happens outside it is a read another writer can invalidate.
//
// Records land here rather than alongside the file projection because the
// caller may replace the installed manifest in between. That manifest is where
// the prior version's identity lives, and absorption is bounded by it: if it
// were overwritten while the marker was still unwritten, a retry would read a
// fresh install and project as enabled the very hook the operator deleted.
func (r *hookReconciler) begin(ctx context.Context) error {
	lock, err := acquireHookFileLock(r.path, r.lockWait)
	if err != nil {
		return err
	}
	r.lock = lock
	if err := r.recordAbsorption(ctx); err != nil {
		if releaseErr := r.release(); releaseErr != nil {
			return fmt.Errorf("%w; %v", err, releaseErr)
		}
		return err
	}
	return nil
}

func (r *hookReconciler) recordAbsorption(ctx context.Context) error {
	store, err := r.openState()
	if err != nil {
		return err
	}
	if store == nil {
		return fmt.Errorf("cannot reconcile %s: hook enablement state is unavailable", r.path)
	}
	if err := r.recordTrustedExecutable(ctx, store); err != nil {
		return err
	}
	records, err := loadHookRecords(ctx, store, r.target)
	if err != nil {
		return err
	}
	projection, err := r.computeProjection(records)
	if err != nil {
		return err
	}
	if projection.writeMarker {
		if _, err := store.AbsorbAndMarkHooks(ctx, r.target, projection.markerVersion, projection.absorbed); err != nil {
			return err
		}
	}
	r.recorded = hookActionsOfKind(projection.actions, hookActionAbsorb)
	// From here until the file is projected the file is at most one reconcile
	// behind the records. That is the retry-safe window: nothing is lost, and
	// the next reconcile converges.
	if r.operations != nil && r.operations.afterRecords != nil {
		return r.operations.afterRecords()
	}
	return nil
}

// complete projects the file and releases the lock. It recomputes from live
// state rather than replaying what begin decided, so a disable that committed
// while the caller was doing its other work is honoured instead of overwritten.
func (r *hookReconciler) complete(ctx context.Context) (actions []hookAction, err error) {
	defer func() {
		if releaseErr := r.release(); releaseErr != nil && err == nil {
			err = releaseErr
		}
	}()
	store, err := r.openState()
	if err != nil {
		return nil, err
	}
	if store == nil {
		return nil, fmt.Errorf("cannot reconcile %s: hook enablement state is unavailable", r.path)
	}
	records, err := loadHookRecords(ctx, store, r.target)
	if err != nil {
		return nil, err
	}
	projection, err := r.computeProjection(records)
	if err != nil {
		return nil, err
	}
	if projection.body != nil {
		if err := publishHookFile(projection.file, projection.body, r.operations); err != nil {
			return nil, err
		}
		if err := r.verify(ctx, store, projection); err != nil {
			return nil, err
		}
	}
	return append(append([]hookAction{}, r.recorded...), projection.actions...), nil
}

// release drops the lock. It is idempotent so a caller can defer it against an
// early failure and still let complete release at the right moment.
func (r *hookReconciler) release() error {
	if r == nil || r.lock == nil {
		return nil
	}
	lock := r.lock
	r.lock = nil
	return lock.release()
}

func hookActionsOfKind(actions []hookAction, kind string) []hookAction {
	var matched []hookAction
	for _, action := range actions {
		if action.action == kind {
			matched = append(matched, action)
		}
	}
	return matched
}

// verify re-reads the published file and runs recognition over it again. A
// converged target computes no further actions and still carries every foreign
// entry it started with; nothing here consults a digest, because no digest of
// this file means anything.
func (r *hookReconciler) verify(ctx context.Context, store *state.Store, published hookProjection) error {
	records, err := loadHookRecords(ctx, store, r.target)
	if err != nil {
		return err
	}
	projection, err := r.computeProjection(records)
	if err != nil {
		return fmt.Errorf("verify reconciled %s: %w", r.path, err)
	}
	if projection.body != nil {
		return fmt.Errorf("reconciled %s did not converge: %s", r.path, describeHookActions(projection.actions))
	}
	if difference := describeHookForeignDrift(published.foreign, projection.foreign); difference != "" {
		return fmt.Errorf("reconciled %s changed an entry Loaf does not own: %s", r.path, difference)
	}
	return nil
}

// describeHookForeignDrift names the first foreign entry whose value or
// position moved, or "" when every one of them survived exactly. Comparing
// values in order rather than counting them is the point: a removal paired with
// an insertion, or two entries swapping places, both keep a count honest.
func describeHookForeignDrift(before []string, after []string) string {
	for index := range before {
		if index >= len(after) {
			return fmt.Sprintf("%s is gone", before[index])
		}
		if before[index] != after[index] {
			return fmt.Sprintf("%s became %s", before[index], after[index])
		}
	}
	if len(after) > len(before) {
		return fmt.Sprintf("%s appeared", after[len(before)])
	}
	return ""
}

func (r *hookReconciler) computeProjection(records hookRecords) (hookProjection, error) {
	file, err := readHookFile(r.path)
	if err != nil {
		return hookProjection{}, err
	}
	file.seed(r.target)

	recognition := r.recognition(records)
	events := r.reconciledEvents(file)
	outcomes := make(map[string]hookPairingOutcome, len(events))
	projection := hookProjection{file: file}
	owned := 0
	for _, event := range events {
		entries, err := file.eventEntries(event)
		if err != nil {
			return hookProjection{}, err
		}
		outcome, err := pairHookEventEntries(recognition, event, entries)
		if err != nil {
			return hookProjection{}, err
		}
		outcomes[event] = outcome
		owned += len(entries) - len(outcome.foreign)
		for _, index := range outcome.foreign {
			canonical, err := canonicalHookEntry(file.entries[event][index])
			if err != nil {
				return hookProjection{}, fmt.Errorf("read %s entry %d in %s: %w", event, index, r.path, err)
			}
			projection.foreign = append(projection.foreign, event+" "+canonical)
		}
	}

	disabled := r.planAbsorption(records, outcomes, owned, &projection)
	changed := false
	for _, event := range events {
		eventChanged, err := r.projectEvent(event, outcomes[event], records, disabled, &projection)
		if err != nil {
			return hookProjection{}, err
		}
		changed = changed || eventChanged
	}
	if !changed {
		return projection, nil
	}
	body, err := projection.file.marshal()
	if err != nil {
		return hookProjection{}, err
	}
	projection.body = body
	return projection, nil
}

// planAbsorption is the run-once migration. It fires only when this target has
// no marker and a prior install is detected, and it considers only the hook ids
// the prior version actually shipped: a hook introduced after that version was
// never there to be deleted, so its absence says nothing. The marker is written
// either way, so a first reconcile that absorbs nothing still closes the window
// in which a later hand-deletion could be read as intent.
func (r *hookReconciler) planAbsorption(records hookRecords, outcomes map[string]hookPairingOutcome, owned int, projection *hookProjection) map[string]bool {
	disabled := map[string]bool{}
	if records.absorbed {
		return disabled
	}
	priorInstall := r.priorProjection || owned > 0
	projection.writeMarker = true
	projection.markerVersion = r.absorptionVersion(priorInstall)
	if !priorInstall {
		return disabled
	}
	cohort := hookAbsorptionCohort(r.catalog, r.priorVersion, r.priorManifest)
	for _, entry := range r.catalog.Entries {
		if !cohort[entry.HookID] || records.recorded(entry.Event, entry.HookID) {
			continue
		}
		if hookPairedInEvent(outcomes[entry.Event], entry.HookID) {
			continue
		}
		key := hookRecordKey(entry.Event, entry.HookID)
		disabled[key] = true
		projection.absorbed = append(projection.absorbed, state.HookEnablementRef{Target: r.target, Event: entry.Event, HookID: entry.HookID})
		projection.actions = append(projection.actions, hookAction{
			action: hookActionAbsorb,
			event:  entry.Event,
			hookID: entry.HookID,
			detail: "absent before this upgrade; recorded as disabled",
		})
	}
	return disabled
}

// projectEvent converges one event section. Entries keep their positions, so
// the relative order of everything Loaf does not own is exactly what it was;
// entries this version adds go to the end of the section.
func (r *hookReconciler) projectEvent(event string, outcome hookPairingOutcome, records hookRecords, disabled map[string]bool, projection *hookProjection) (bool, error) {
	removed := map[int]bool{}
	replaced := map[int]json.RawMessage{}
	var added []json.RawMessage

	for _, duplicate := range outcome.duplicates {
		removed[duplicate.index] = true
		projection.actions = append(projection.actions, hookAction{
			action: hookActionRemove,
			event:  event,
			hookID: duplicate.hookID,
			index:  duplicate.index,
			detail: "second entry for the same hook",
		})
	}
	for _, index := range outcome.retired {
		removed[index] = true
		projection.actions = append(projection.actions, hookAction{
			action: hookActionRemove,
			event:  event,
			index:  index,
			detail: "entry from a retired Loaf generation",
		})
	}

	paired := make(map[string]hookEntryPairing, len(outcome.paired))
	for _, pairing := range outcome.paired {
		paired[pairing.hookID] = pairing
	}
	for _, entry := range r.catalog.entriesForEvent(event) {
		pairing, present := paired[entry.HookID]
		enabled := records.enabled(entry.Event, entry.HookID) && !disabled[hookRecordKey(entry.Event, entry.HookID)]
		switch {
		case !enabled && present:
			removed[pairing.index] = true
			projection.actions = append(projection.actions, hookAction{
				action: hookActionRemove,
				event:  event,
				hookID: entry.HookID,
				index:  pairing.index,
				detail: "disabled",
			})
		case !enabled:
			continue
		case !present:
			desired, err := r.desiredEntry(entry)
			if err != nil {
				return false, err
			}
			added = append(added, desired)
			projection.actions = append(projection.actions, hookAction{action: hookActionAdd, event: event, hookID: entry.HookID})
		default:
			desired, err := r.desiredEntry(entry)
			if err != nil {
				return false, err
			}
			same, err := hookEntriesEqual(projection.file.entries[event][pairing.index], desired)
			if err != nil {
				return false, err
			}
			if same {
				continue
			}
			replaced[pairing.index] = desired
			projection.actions = append(projection.actions, hookAction{
				action: hookActionUpdate,
				event:  event,
				hookID: entry.HookID,
				index:  pairing.index,
				detail: "entry differs from the shipped hook",
			})
		}
	}

	if len(removed) == 0 && len(replaced) == 0 && len(added) == 0 {
		return false, nil
	}
	existing := projection.file.entries[event]
	kept := make([]json.RawMessage, 0, len(existing)+len(added))
	for index, entry := range existing {
		if removed[index] {
			continue
		}
		if replacement, ok := replaced[index]; ok {
			kept = append(kept, replacement)
			continue
		}
		kept = append(kept, entry)
	}
	projection.file.setEventEntries(event, append(kept, added...))
	return true, nil
}

// recognition assembles the closed ownership predicate's inputs. Trusted
// executable paths are every path recorded for this target plus the one
// resolving right now, so an entry written before Loaf moved is still Loaf's.
func (r *hookReconciler) recognition(records hookRecords) hookRecognition {
	trusted := append([]string{}, records.trusted...)
	if executable, err := r.trustedExecutable(); err == nil && executable != "" {
		trusted = appendUniqueHookPath(trusted, executable)
	}
	return hookRecognition{
		target:       r.target,
		catalog:      r.catalog,
		trustedPaths: trusted,
		managedPaths: r.managedPaths,
		homeDir:      r.homeDir,
		goos:         r.goos,
	}
}

// recordTrustedExecutable pins the path this run would write into an entry, so
// the next reconcile recognizes it even after Loaf moves. Resolution failing is
// not fatal here: a target whose entries invoke the bare `loaf` on PATH has
// nothing to pin, and the reconcile that does need the path fails where it
// needs it.
func (r *hookReconciler) recordTrustedExecutable(ctx context.Context, store *state.Store) error {
	executable, err := r.trustedExecutable()
	if err != nil || executable == "" {
		return nil
	}
	_, err = store.RecordHookTrustedPath(ctx, r.target, executable)
	return err
}

func (r *hookReconciler) trustedExecutable() (string, error) {
	if !r.executableResolved {
		r.executableResolved = true
		r.executable, r.executableErr = r.resolveExecutable()
	}
	return r.executable, r.executableErr
}

// desiredEntry is the entry this version wants at an identity. The catalog
// carries the install-time executable placeholder for targets that pin an
// absolute path; everything else is already the desired shape.
func (r *hookReconciler) desiredEntry(entry hookCatalogEntry) (json.RawMessage, error) {
	if !bytes.Contains(entry.Template, []byte(codexJournalExecutablePlaceholder)) {
		return entry.Template, nil
	}
	executable, err := r.trustedExecutable()
	if err != nil {
		return nil, err
	}
	rendered, err := renderCodexHookExecutableForOS(entry.Template, executable, r.goos)
	if err != nil {
		return nil, fmt.Errorf("render %s hook %s: %w", r.target, entry.HookID, err)
	}
	return rendered, nil
}

// reconciledEvents is every section pairing has to look at: the ones the file
// carries, so a Loaf entry left in a section this version no longer uses is
// still found, plus the ones the catalog ships, so a missing section can be
// created.
func (r *hookReconciler) reconciledEvents(file hookFile) []string {
	events := append([]string{}, file.events...)
	seen := make(map[string]bool, len(events))
	for _, event := range events {
		seen[event] = true
	}
	for _, entry := range r.catalog.Entries {
		if !seen[entry.Event] {
			seen[entry.Event] = true
			events = append(events, entry.Event)
		}
	}
	return events
}

// absorptionVersion records what the marker is provenance for. A manifest names
// the version whose cohort bounded the absorption. A prior install without one
// is genuinely unidentifiable and the marker says so rather than naming a
// version nobody observed; with no prior install at all the marker simply dates
// itself to the version that closed the window.
func (r *hookReconciler) absorptionVersion(priorInstall bool) string {
	if version := strings.TrimSpace(r.priorVersion); version != "" {
		return version
	}
	if version := strings.TrimSpace(r.version); version != "" && !priorInstall {
		return version
	}
	return "unknown"
}

// hookAbsorptionCohort bounds absorption to the hook ids one released version
// actually shipped, in the three cases that can arise:
//
//   - a manifest naming a version this catalog enumerates: that cohort;
//   - no manifest at all: the last frozen cohort, the generation predating
//     entry-level reconciliation, which is the only thing a pre-manifest
//     install could have been;
//   - a manifest naming a version nobody enumerated: nothing. The install is
//     identified and its cohort is not recorded, so no absence in the file is
//     evidence of anything. Absorbing on a guess here is how a hook the
//     operator never disabled ends up disabled.
func hookAbsorptionCohort(catalog hookCatalog, version string, hasManifest bool) map[string]bool {
	var ids []string
	switch recorded, enumerated := catalog.cohortHookIDs(version); {
	case enumerated:
		ids = recorded
	case !hasManifest && len(catalog.Cohorts) > 0:
		ids = catalog.Cohorts[len(catalog.Cohorts)-1].HookIDs
	}
	cohort := make(map[string]bool, len(ids))
	for _, id := range ids {
		cohort[id] = true
	}
	return cohort
}

func canonicalHookEntry(raw json.RawMessage) (string, error) {
	var compact bytes.Buffer
	if err := json.Compact(&compact, raw); err != nil {
		return "", err
	}
	return compact.String(), nil
}

func hookPairedInEvent(outcome hookPairingOutcome, hookID string) bool {
	for _, pairing := range outcome.paired {
		if pairing.hookID == hookID {
			return true
		}
	}
	return false
}

func hookEntriesEqual(left json.RawMessage, right json.RawMessage) (bool, error) {
	current, err := decodeHookJSONValue(left)
	if err != nil {
		return false, err
	}
	desired, err := decodeHookJSONValue(right)
	if err != nil {
		return false, err
	}
	return reflect.DeepEqual(current, desired), nil
}

func appendUniqueHookPath(paths []string, path string) []string {
	for _, existing := range paths {
		if existing == path {
			return paths
		}
	}
	return append(paths, path)
}

func describeHookActions(actions []hookAction) string {
	if len(actions) == 0 {
		return "no actions"
	}
	described := make([]string, 0, len(actions))
	for _, action := range actions {
		described = append(described, action.action+" "+action.id())
	}
	return strings.Join(described, ", ")
}

// beginHookReconcile is install and upgrade's first hook step. It runs before
// any of the target's artifacts move, because the installed manifest it reads
// is about to be replaced and the absorption bound lives in it.
func beginHookReconcile(reconciler *hookReconciler) error {
	if reconciler == nil {
		return nil
	}
	return reconciler.begin(context.Background())
}

// completeHookReconcile is the last hook step. It runs after the target's other
// artifacts are in place, so an entry is never projected before the script it
// invokes exists.
func completeHookReconcile(options targetInstallOptions, reconciler *hookReconciler) error {
	if reconciler == nil {
		return nil
	}
	actions, err := reconciler.complete(context.Background())
	if err != nil {
		return err
	}
	if options.HookActions != nil && len(actions) > 0 {
		options.HookActions(actions)
	}
	return nil
}

// releaseHookReconcile is the deferred counterpart: whatever fails between the
// two halves, the lock does not outlive the run. Completing already released
// it, so this is a no-op on the successful path.
func releaseHookReconcile(reconciler *hookReconciler) {
	if reconciler != nil {
		_ = reconciler.release()
	}
}

// hookActionPlanDecisions renders the computed actions onto the plan surface.
// A converged target contributes nothing: there is no per-file line to report
// because the file is not the unit of the decision any more.
func hookActionPlanDecisions(reconciler *hookReconciler, actions []hookAction) []artifactPlanDecision {
	decisions := make([]artifactPlanDecision, 0, len(actions))
	for _, action := range actions {
		decisions = append(decisions, artifactPlanDecision{
			ID:          action.id(),
			Kind:        "hook-entry",
			Destination: reconciler.path,
			Action:      action.action,
			Detail:      action.detail,
		})
	}
	return decisions
}
