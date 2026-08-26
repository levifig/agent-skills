package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/levifig/loaf/internal/project"
)

// hooks.go is the operator's surface over hook enablement: which hooks this
// version ships for an installed target, whether the target's shared hooks file
// currently carries each one, and the two acts that change it.
//
// Enablement lives in the user-scoped records and the file is their projection,
// so deleting an entry by hand says nothing — the next reconcile restores it.
// `loaf hooks disable` is the gesture that means it, and because it is recorded,
// every later upgrade honours it.
//
// Both writers run the full reconciler rather than editing one entry. There is
// one convergence path for this file and the verb is one more caller of it,
// which is why enable and disable report drift they were not asked about
// instead of pretending single-entry surgery.
//
// Everything here resolves through the installed distribution's built catalog
// and the detected harnesses: no source checkout is involved, and no identity is
// invented locally.

func (r Runner) runHooks(args []string, out io.Writer, runtimeRoot string) error {
	if len(args) == 0 {
		writeHooksHelp(out)
		return nil
	}
	switch args[0] {
	case "--help", "-h", "help":
		writeHooksHelp(out)
		return nil
	case "list":
		return r.runHooksList(args[1:], out, runtimeRoot)
	case "enable":
		return r.runHooksToggle(args[1:], out, runtimeRoot, true)
	case "disable":
		return r.runHooksToggle(args[1:], out, runtimeRoot, false)
	default:
		return fmt.Errorf("unknown hooks subcommand %q (valid subcommands: list, enable, disable)", args[0])
	}
}

type hooksListOptions struct {
	target string
	help   bool
}

type hooksToggleOptions struct {
	hookID string
	target string
	help   bool
}

func parseHooksListArgs(args []string) (hooksListOptions, error) {
	var options hooksListOptions
	for i := 0; i < len(args); i++ {
		switch arg := args[i]; arg {
		case "--target":
			if i+1 >= len(args) {
				return hooksListOptions{}, fmt.Errorf("--target requires a value")
			}
			i++
			options.target = args[i]
		case "--help", "-h":
			options.help = true
		default:
			return hooksListOptions{}, fmt.Errorf("unknown hooks list option %q", arg)
		}
	}
	return options, nil
}

func parseHooksToggleArgs(verb string, args []string) (hooksToggleOptions, error) {
	var options hooksToggleOptions
	for i := 0; i < len(args); i++ {
		switch arg := args[i]; arg {
		case "--target":
			if i+1 >= len(args) {
				return hooksToggleOptions{}, fmt.Errorf("--target requires a value")
			}
			i++
			options.target = args[i]
		case "--help", "-h":
			options.help = true
		default:
			if strings.HasPrefix(arg, "-") {
				return hooksToggleOptions{}, fmt.Errorf("unknown hooks %s option %q", verb, arg)
			}
			if options.hookID != "" {
				return hooksToggleOptions{}, fmt.Errorf("hooks %s takes one hook id, got %q and %q", verb, options.hookID, arg)
			}
			options.hookID = arg
		}
	}
	return options, nil
}

func (r Runner) runHooksList(args []string, out io.Writer, runtimeRoot string) error {
	options, err := parseHooksListArgs(args)
	if err != nil {
		return err
	}
	if options.help {
		writeHooksListHelp(out)
		return nil
	}
	projectRoot, err := project.ResolveRoot(runtimeRoot)
	if err != nil {
		return err
	}
	// A listing reads and never records. The plan resolver neither creates nor
	// bootstraps a database, so a host that has never run Loaf reports every
	// hook enabled instead of acquiring state in order to say so.
	hookState, releaseHookState := r.hookStateForPlan(projectRoot.Path())
	defer releaseHookState()
	surfaces, err := r.hooksTargetSurfaces(projectRoot.Path(), hookState, options.target)
	if err != nil {
		return err
	}
	ctx := context.Background()
	listings := make([]hookListing, 0, len(surfaces))
	for _, surface := range surfaces {
		listing := hookListing{target: surface.target, path: surface.path}
		listing.rows, listing.err = readHookListing(ctx, surface)
		listings = append(listings, listing)
	}
	writeHookListings(out, listings)
	// One unreadable target does not cost the others their listing, but it does
	// cost the run its exit code: what was printed is not the whole answer.
	for _, listing := range listings {
		if listing.err != nil {
			return ExitError{Code: 1}
		}
	}
	return nil
}

func (r Runner) runHooksToggle(args []string, out io.Writer, runtimeRoot string, enable bool) error {
	verb := hooksToggleVerb(enable)
	options, err := parseHooksToggleArgs(verb, args)
	if err != nil {
		return err
	}
	if options.help {
		writeHooksToggleHelp(out, verb)
		return nil
	}
	if options.hookID == "" {
		return fmt.Errorf("loaf hooks %s requires a hook id; run `loaf hooks list` to see the hooks this version ships", verb)
	}
	if options.target == "" {
		return fmt.Errorf("loaf hooks %s requires --target: enablement is recorded per target (%s)", verb, strings.Join(reconciledHookTargets(), ", "))
	}
	projectRoot, err := project.ResolveRoot(runtimeRoot)
	if err != nil {
		return err
	}
	hookState, releaseHookState := r.hookStateForApply(projectRoot.Path())
	defer releaseHookState()
	surface, err := r.hooksTargetSurface(projectRoot.Path(), hookState, options.target)
	if err != nil {
		return err
	}
	reconciler, err := hooksReconcilerFor(surface.options)
	if err != nil {
		return err
	}
	entries, err := hooksCatalogEntriesFor(reconciler.catalog, options.hookID)
	if err != nil {
		return err
	}
	actions, err := toggleHookEnablement(context.Background(), reconciler, surface, entries, enable)
	if err != nil {
		return err
	}
	writeHookToggleResult(out, surface, entries, enable, actions)
	return nil
}

// toggleHookEnablement is Decision 10's ordering for a verb write, and the
// reason it composes the reconciler instead of editing one entry: take the
// target's lock, make the record durable inside it, then run the whole
// convergence, which recomputes from the records it just read and publishes the
// file before the lock is dropped.
//
// begin holds the lock and lands the absorption half; the enablement record goes
// in behind it, so a first-ever reconcile that absorbs this very hook is
// overridden by what the operator just asked for rather than racing it.
func toggleHookEnablement(ctx context.Context, reconciler *hookReconciler, surface hooksTargetSurface, entries []hookCatalogEntry, enable bool) ([]hookAction, error) {
	if err := reconciler.begin(ctx); err != nil {
		return nil, err
	}
	defer releaseHookReconcile(reconciler)
	store, err := surface.options.HookState()
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if _, err := store.SetHookEnablement(ctx, surface.target, entry.Event, entry.HookID, enable); err != nil {
			return nil, err
		}
	}
	return reconciler.complete(ctx)
}

// hooksCatalogEntriesFor resolves a hook id against the target's built catalog,
// which is the identity authority: an id the catalog does not carry is not a
// hook this version ships for this target, whatever another target ships or an
// older release called it.
func hooksCatalogEntriesFor(catalog hookCatalog, hookID string) ([]hookCatalogEntry, error) {
	var matched []hookCatalogEntry
	for _, entry := range catalog.Entries {
		if entry.HookID == hookID {
			matched = append(matched, entry)
		}
	}
	if len(matched) == 0 {
		return nil, fmt.Errorf("%s ships no hook %q in this version; it ships: %s", installDisplayName(catalog.Target), hookID, strings.Join(hooksCatalogIDs(catalog), ", "))
	}
	return matched, nil
}

func hooksCatalogIDs(catalog hookCatalog) []string {
	seen := map[string]bool{}
	var ids []string
	for _, entry := range catalog.Entries {
		if seen[entry.HookID] {
			continue
		}
		seen[entry.HookID] = true
		ids = append(ids, entry.HookID)
	}
	sort.Strings(ids)
	return ids
}

// hooksTargetSurface is one installed target's reconciled hooks file: the
// install options a reconciler is built from, and where the file lives.
type hooksTargetSurface struct {
	target  string
	options targetInstallOptions
	path    string
}

func (r Runner) hooksTargetSurface(projectRoot string, hookState hookStateResolver, target string) (hooksTargetSurface, error) {
	loafRoot, err := r.resolveInstalledDistributionRoot()
	if err != nil {
		return hooksTargetSurface{}, err
	}
	tools := detectInstallTools()
	if err := requireOperableHooksTarget(target, installedUpgradeTargets(tools)); err != nil {
		return hooksTargetSurface{}, err
	}
	return newHooksTargetSurface(loafRoot, tools, projectRoot, hookState, target), nil
}

func (r Runner) hooksTargetSurfaces(projectRoot string, hookState hookStateResolver, only string) ([]hooksTargetSurface, error) {
	loafRoot, err := r.resolveInstalledDistributionRoot()
	if err != nil {
		return nil, err
	}
	tools := detectInstallTools()
	installed := installedUpgradeTargets(tools)
	if only != "" {
		if err := requireOperableHooksTarget(only, installed); err != nil {
			return nil, err
		}
		return []hooksTargetSurface{newHooksTargetSurface(loafRoot, tools, projectRoot, hookState, only)}, nil
	}
	// Unfiltered, an uninstalled harness is simply not part of the answer, and
	// one that keeps no shared hooks file has nothing this verb can say about
	// it. Neither is an error: the listing describes what is here.
	var surfaces []hooksTargetSurface
	for _, target := range installed {
		if !targetKeepsReconciledHookFile(target) {
			continue
		}
		surfaces = append(surfaces, newHooksTargetSurface(loafRoot, tools, projectRoot, hookState, target))
	}
	return surfaces, nil
}

func newHooksTargetSurface(loafRoot string, tools []detectedInstallTool, projectRoot string, hookState hookStateResolver, target string) hooksTargetSurface {
	configDir := installLayoutConfigDirs(projectRoot)[target]
	if tool, ok := installToolsByKey(tools)[target]; ok && tool.configDir != "" {
		configDir = tool.configDir
	}
	options := targetInstallOptions{
		Target:      target,
		DistDir:     filepath.Join(loafRoot, "dist", target),
		ConfigDir:   configDir,
		Version:     packageVersion(loafRoot),
		HomeDir:     installLayoutHome(projectRoot),
		CodexHome:   os.Getenv("CODEX_HOME"),
		ProjectRoot: projectRoot,
		HookState:   hookState,
	}
	return hooksTargetSurface{target: target, options: options, path: targetHookFilePath(options)}
}

// requireOperableHooksTarget rejects the three ways a named target can have no
// hooks to operate, each with the command that would change it.
func requireOperableHooksTarget(target string, installed []string) error {
	if !isKnownHooksHarness(target) {
		return fmt.Errorf("unknown target %q (valid targets: %s)", target, strings.Join(reconciledHookTargets(), ", "))
	}
	if !targetKeepsReconciledHookFile(target) {
		return fmt.Errorf("%s keeps no hooks file Loaf reconciles per entry; use --target %s", installDisplayName(target), strings.Join(reconciledHookTargets(), " or --target "))
	}
	if !containsString(installed, target) {
		return fmt.Errorf("%s is not installed here, so it has no hooks to operate; run `loaf install --to %s` first", installDisplayName(target), target)
	}
	return nil
}

// isKnownHooksHarness reports whether target names a harness Loaf deploys to at
// all. It reads the display-name map rather than installValidTargets, which is
// narrower — that list is what `loaf install --to` accepts, and Claude Code is
// not on it because its content rides the plugin. Claude Code is still a real
// harness with real hooks, so naming it here earns the explanation of where
// those hooks live, not a claim that Loaf has never heard of it.
func isKnownHooksHarness(target string) bool {
	_, known := installDisplayNames[target]
	return known
}

// targetKeepsReconciledHookFile asks the path resolver rather than restating
// its switch, so a target that grows a reconciled hooks file later is operable
// through this verb the moment it does.
func targetKeepsReconciledHookFile(target string) bool {
	return targetHookFilePath(targetInstallOptions{Target: target, ConfigDir: ".", HomeDir: "."}) != ""
}

func reconciledHookTargets() []string {
	var targets []string
	for _, target := range installValidTargets {
		if targetKeepsReconciledHookFile(target) {
			targets = append(targets, target)
		}
	}
	return targets
}

// hooksReconcilerFor builds the target's reconciler, naming the two ways an
// installed distribution can fail to carry one before the reconciler's own
// staleness message has to.
func hooksReconcilerFor(options targetInstallOptions) (*hookReconciler, error) {
	if !dirExistsForInstall(options.DistDir) {
		return nil, fmt.Errorf("no build output found for %s at %s; run `loaf build` or reinstall Loaf", installDisplayName(options.Target), options.DistDir)
	}
	reconciler, err := newHookReconciler(options)
	if err != nil {
		return nil, err
	}
	if reconciler == nil {
		return nil, fmt.Errorf("%s keeps no hooks file Loaf reconciles per entry", installDisplayName(options.Target))
	}
	return reconciler, nil
}

// hookListingRow is one catalog identity as the operator sees it: what it is,
// what the records say, and what the file carries. The last two are independent
// — a disagreement is a pending reconcile, never a conflict.
type hookListingRow struct {
	event      string
	hookID     string
	enabled    bool
	projected  bool
	absorbedAt string
}

type hookListing struct {
	target string
	path   string
	rows   []hookListingRow
	err    error
}

// readHookListing reports every hook the current catalog ships for one target.
// It iterates the catalog rather than the records, which is what keeps a
// tombstoned row — an enablement record for a hook id this version retired —
// out of the listing while leaving it in the database where Decision 5 wants it.
func readHookListing(ctx context.Context, surface hooksTargetSurface) ([]hookListingRow, error) {
	reconciler, err := hooksReconcilerFor(surface.options)
	if err != nil {
		return nil, err
	}
	store, err := surface.options.HookState()
	if err != nil {
		return nil, err
	}
	records, err := loadHookRecords(ctx, store, surface.target)
	if err != nil {
		return nil, err
	}
	projected, err := projectedHookIdentities(reconciler, records)
	if err != nil {
		return nil, err
	}
	rows := make([]hookListingRow, 0, len(reconciler.catalog.Entries))
	for _, entry := range reconciler.catalog.Entries {
		key := hookRecordKey(entry.Event, entry.HookID)
		row := hookListingRow{
			event:     entry.Event,
			hookID:    entry.HookID,
			enabled:   records.enabled(entry.Event, entry.HookID),
			projected: projected[key],
		}
		if record, recorded := records.enablement[key]; recorded && record.AbsorbedAt != nil {
			row.absorbedAt = *record.AbsorbedAt
		}
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].event != rows[j].event {
			return rows[i].event < rows[j].event
		}
		return rows[i].hookID < rows[j].hookID
	})
	return rows, nil
}

// projectedHookIdentities answers which catalog identities the live file
// currently carries. It runs the same recognition and pairing a reconcile does:
// a listing that judged the file by its own rules would be a second opinion
// about ownership, and there is only one.
func projectedHookIdentities(reconciler *hookReconciler, records hookRecords) (map[string]bool, error) {
	file, err := readHookFile(reconciler.path)
	if err != nil {
		return nil, err
	}
	recognition := reconciler.recognition(records)
	projected := map[string]bool{}
	for _, event := range reconciler.reconciledEvents(file) {
		entries, err := file.eventEntries(event)
		if err != nil {
			return nil, err
		}
		outcome, err := pairHookEventEntries(recognition, event, entries)
		if err != nil {
			return nil, err
		}
		for _, pairing := range outcome.paired {
			projected[hookRecordKey(event, pairing.hookID)] = true
		}
	}
	return projected, nil
}

func writeHookListings(out io.Writer, listings []hookListing) {
	fmt.Fprintln(out)
	fmt.Fprintln(out, ansiBold("loaf hooks"))
	fmt.Fprintln(out)
	if len(listings) == 0 {
		fmt.Fprintf(out, "  %s\n\n", ansiGray("No installed harness keeps a Loaf-reconciled hooks file. Run loaf install --to cursor or loaf install --to codex."))
		return
	}
	home := installHome()
	for _, listing := range listings {
		fmt.Fprintf(out, "  %s %s\n", ansiBold(installDisplayName(listing.target)), ansiGray(displayHomeRelativePath(listing.path, home)))
		switch {
		case listing.err != nil:
			fmt.Fprintf(out, "    %s %v\n", ansiRed("✗"), listing.err)
		case len(listing.rows) == 0:
			fmt.Fprintf(out, "    %s\n", ansiGray("This version ships no hooks for this target"))
		default:
			writeHookListingRows(out, listing.rows)
		}
		fmt.Fprintln(out)
	}
}

// writeHookListingRows prints enablement and file state as two independent
// columns, because they are two independent facts. Their disagreement is what
// the trailing note names, and it is always resolvable by a reconcile.
func writeHookListingRows(out io.Writer, rows []hookListingRow) {
	hookWidth, eventWidth := 0, 0
	for _, row := range rows {
		if len(row.hookID) > hookWidth {
			hookWidth = len(row.hookID)
		}
		if len(row.event) > eventWidth {
			eventWidth = len(row.event)
		}
	}
	for _, row := range rows {
		line := fmt.Sprintf("    %-*s  %-*s  %-8s  %-7s", hookWidth, row.hookID, eventWidth, row.event, hookEnablementLabel(row.enabled), hookProjectionLabel(row.projected))
		note := hookListingNote(row)
		if note == "" {
			fmt.Fprintln(out, strings.TrimRight(line, " "))
			continue
		}
		fmt.Fprintf(out, "%s  %s\n", line, ansiGray(note))
	}
}

func hookEnablementLabel(enabled bool) string {
	if enabled {
		return "enabled"
	}
	return "disabled"
}

func hookProjectionLabel(projected bool) string {
	if projected {
		return "present"
	}
	return "absent"
}

func hookListingNote(row hookListingRow) string {
	var notes []string
	if row.absorbedAt != "" {
		notes = append(notes, "(absorbed "+hookAbsorptionDate(row.absorbedAt)+")")
	}
	if row.enabled != row.projected {
		notes = append(notes, "reconcile pending")
	}
	return strings.Join(notes, "  ")
}

// hookAbsorptionDate renders immutable absorption provenance at the resolution
// an operator reads it at. The stored timestamp keeps its full precision.
func hookAbsorptionDate(value string) string {
	if parsed, err := time.Parse(time.RFC3339, value); err == nil {
		return parsed.Format("2006-01-02")
	}
	return value
}

// writeHookToggleResult says what the operator asked for and what happened to
// that hook's entry, then names everything else the same reconcile did. On a
// converged target the second part is empty, which is the sketch's one line;
// when the target had drift, the extra lines are the honest difference between
// "one entry changed" and "one entry was asked about".
func writeHookToggleResult(out io.Writer, surface hooksTargetSurface, entries []hookCatalogEntry, enable bool, actions []hookAction) {
	path := displayHomeRelativePath(surface.path, installHome())
	fmt.Fprintln(out)
	spoken := map[int]bool{}
	for _, entry := range entries {
		index := hookFileActionFor(actions, entry, spoken)
		headline := hookToggleHeadline(enable, hookActionKindAt(actions, index), path)
		if len(entries) > 1 {
			headline = entry.Event + ": " + headline
		}
		spoken[index] = true
		fmt.Fprintf(out, "  %s %s\n", ansiGreen("✓"), headline)
	}
	var remaining []hookAction
	for index, action := range actions {
		if !spoken[index] {
			remaining = append(remaining, action)
		}
	}
	if len(remaining) > 0 {
		fmt.Fprintf(out, "  %s\n", ansiGray("Also in this run:"))
		writeHookActionLines(out, remaining)
	}
	fmt.Fprintln(out)
}

func hookToggleHeadline(enable bool, action string, path string) string {
	if enable {
		switch action {
		case hookActionAdd:
			return "enabled — entry restored to " + path
		case hookActionUpdate:
			return "enabled — entry updated in " + path
		default:
			return "enabled — entry already in " + path
		}
	}
	switch action {
	case hookActionRemove:
		return "disabled — entry removed from " + path
	default:
		return "disabled — no entry in " + path
	}
}

// hookFileActionFor finds the single action a headline can speak for, skipping
// the ones an earlier headline already claimed. It returns -1 when the reconcile
// changed nothing for this identity — the record moved and the file already
// agreed — and claiming exactly one keeps a second action on the same identity,
// such as a duplicate entry the same run removed, in the reported remainder.
func hookFileActionFor(actions []hookAction, entry hookCatalogEntry, spoken map[int]bool) int {
	for index, action := range actions {
		if spoken[index] || action.event != entry.Event || action.hookID != entry.HookID || !isHookFileAction(action.action) {
			continue
		}
		return index
	}
	return -1
}

func hookActionKindAt(actions []hookAction, index int) string {
	if index < 0 {
		return ""
	}
	return actions[index].action
}

func isHookFileAction(action string) bool {
	switch action {
	case hookActionAdd, hookActionUpdate, hookActionRemove:
		return true
	default:
		return false
	}
}

func hooksToggleVerb(enable bool) string {
	if enable {
		return "enable"
	}
	return "disable"
}

// displayHomeRelativePath prints an absolute path under the operator's home the
// way they would type it. Display only: every path this verb acts on stays
// absolute.
func displayHomeRelativePath(path string, home string) string {
	if home == "" || path == "" {
		return path
	}
	prefix := home + string(filepath.Separator)
	if !strings.HasPrefix(path, prefix) {
		return path
	}
	return "~" + string(filepath.Separator) + path[len(prefix):]
}

func writeHooksHelp(out io.Writer) {
	writeCommandGroupHelp(out, "loaf hooks <subcommand> [options]",
		"Inspect and set which Loaf hooks project into an installed harness's hooks file.",
		[]subcommandHelpItem{
			{Name: "list", Summary: "Show every hook this version ships, its enablement, and whether the file carries it"},
			{Name: "enable", Summary: "Record a hook as enabled for one target and reconcile that target's hooks file"},
			{Name: "disable", Summary: "Record a hook as disabled for one target and reconcile that target's hooks file"},
		})
}

func writeHooksListHelp(out io.Writer) {
	writeUsageHelp(out, "loaf hooks list [--target <target>]",
		"Show every hook this version ships for each installed target, with its event, effective enablement, whether the live file currently carries it, and absorption provenance. Retired hook ids are not listed; entries Loaf does not own are never mentioned.",
		"--target <target>  Restrict the listing to one installed target ("+strings.Join(reconciledHookTargets(), ", ")+")")
}

func writeHooksToggleHelp(out io.Writer, verb string) {
	writeUsageHelp(out, "loaf hooks "+verb+" <hook-id> --target <target>",
		"Record <hook-id> as "+verb+"d for one target, then reconcile that target's hooks file and report every action the reconcile took.",
		"--target <target>  Target whose hooks file to reconcile ("+strings.Join(reconciledHookTargets(), ", ")+")")
}
