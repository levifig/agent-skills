package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/levifig/loaf/internal/state"
)

// hooks_test.go drives the verb the way an operator does: against an installed
// distribution on a fixture machine, with no source checkout in reach. What it
// asserts is the contract the shape states — a listing that agrees with both the
// records and the file, a toggle that edits one entry on a converged target and
// names everything else it touched on one that had drifted, and errors that say
// which command would fix them.

// TestRunnerHooksListReportsEnablementAndFileStateForEveryCatalogHook covers all
// four combinations of the two independent facts a row carries. Enablement and
// projection disagreeing is a pending reconcile, not a conflict, and the listing
// has to be able to say so.
func TestRunnerHooksListReportsEnablementAndFileStateForEveryCatalogHook(t *testing.T) {
	root, home := setupHooksFixture(t, hooksFixtureCursorSources()...)
	path := hooksFixtureFilePath(home)

	// validate-commit is enabled and present: untouched since the install
	// projected it. The other three are recorded first and then hand-edited,
	// because every verb call converges the whole file — which is the point.
	runInstallFixture(t, root, "hooks", "disable", "render-drift", "--target", "cursor")
	runInstallFixture(t, root, "hooks", "disable", "validate-push", "--target", "cursor")
	restoreHooksFixtureEntry(t, path, "beforeShellExecution", map[string]any{
		"command":      "loaf check --hook validate-push",
		"matcher":      "Bash",
		"loaf-managed": true,
	})
	deleteHooksFixtureEntry(t, path, "beforeSubmitPrompt", "loaf check --hook artifact-names")

	rows := parseHooksListRows(t, runInstallCapture(t, root, "hooks", "list"))

	for _, want := range []hooksListRow{
		{hookID: "validate-commit", event: "beforeShellExecution", enablement: "enabled", projection: "present"},
		{hookID: "validate-push", event: "beforeShellExecution", enablement: "disabled", projection: "present"},
		{hookID: "render-drift", event: "beforeShellExecution", enablement: "disabled", projection: "absent"},
		{hookID: "artifact-names", event: "beforeSubmitPrompt", enablement: "enabled", projection: "absent"},
	} {
		got, listed := rows[want.hookID]
		if !listed {
			t.Fatalf("hooks list rows = %#v, want a row for %s", rows, want.hookID)
		}
		if got.event != want.event || got.enablement != want.enablement || got.projection != want.projection {
			t.Fatalf("%s row = %#v, want %#v", want.hookID, got, want)
		}
	}
	if note := rows["artifact-names"].note; !strings.Contains(note, "reconcile pending") {
		t.Fatalf("enabled-but-missing note = %q, want it to name the pending reconcile", note)
	}
	if note := rows["validate-commit"].note; note != "" {
		t.Fatalf("in-sync note = %q, want nothing said about a converged hook", note)
	}
}

// The listing enumerates the catalog, so a record for a hook id this version no
// longer ships stays in the database as Decision 5 requires and out of the
// operator's view, which only ever describes what this version can project.
func TestRunnerHooksListOmitsTombstonedRecords(t *testing.T) {
	root, _ := setupHooksFixture(t, hooksFixtureCursorSources()...)
	withHooksFixtureStore(t, func(store *state.Store) {
		if _, err := store.SetHookEnablement(context.Background(), "cursor", "beforeShellExecution", "retired-generation-hook", false); err != nil {
			t.Fatalf("SetHookEnablement error = %v", err)
		}
	})

	output := runInstallCapture(t, root, "hooks", "list", "--target", "cursor")

	if strings.Contains(output, "retired-generation-hook") {
		t.Fatalf("hooks list = %q, want no row for a hook id this version does not ship", output)
	}
	if rows := parseHooksListRows(t, output); len(rows) != len(hooksFixtureCursorSources()) {
		t.Fatalf("hooks list rows = %#v, want exactly the catalog", rows)
	}
}

// A listing is a read. It may not rewrite the file it describes, and it may not
// acquire state in order to report that no state exists.
func TestRunnerHooksListWritesNothing(t *testing.T) {
	root, home := setupHooksFixture(t, hooksFixtureCursorSources()...)
	path := hooksFixtureFilePath(home)
	before := readFileBytes(t, path)
	database := os.Getenv("LOAF_DB")
	if err := os.Remove(database); err != nil {
		t.Fatalf("Remove(%s) error = %v", database, err)
	}

	rows := parseHooksListRows(t, runInstallCapture(t, root, "hooks", "list", "--target", "cursor"))

	if !bytes.Equal(readFileBytes(t, path), before) {
		t.Fatalf("hooks list rewrote %s", path)
	}
	if _, err := os.Stat(database); !os.IsNotExist(err) {
		t.Fatalf("Stat(%s) = %v, want the listing to leave a stateless host stateless", database, err)
	}
	for _, row := range rows {
		if row.enablement != "enabled" || row.projection != "present" {
			t.Fatalf("row = %#v, want absence of records to read as enabled", row)
		}
	}
}

// A harness that is not installed is simply not part of the answer: it is never
// an error, and it never costs the installed ones their listing.
func TestRunnerHooksListSkipsUninstalledTargets(t *testing.T) {
	root, _ := setupHooksFixture(t, hooksFixtureCursorSources()...)

	output := stripANSI(runInstallCapture(t, root, "hooks", "list"))

	if !strings.Contains(output, "Cursor") {
		t.Fatalf("hooks list = %q, want the installed target listed", output)
	}
	if strings.Contains(output, "Codex") {
		t.Fatalf("hooks list = %q, want an uninstalled target left out", output)
	}
}

// H3, at the level the verb can prove on a fixture: a round trip on a converged
// target edits exactly one entry in each direction. Everything else — foreign
// entries, the other Loaf entries, the top-level fields — survives with its
// exact JSON value and its position among the rest.
func TestRunnerHooksToggleRoundTripEditsExactlyOneEntry(t *testing.T) {
	root, home := setupHooksFixture(t, hooksFixtureCursorSources()...)
	path := hooksFixtureFilePath(home)
	seedHooksFixtureForeignEntries(t, path)
	converged := string(readFileBytes(t, path))
	entry := "beforeShellExecution " + hooksFixtureEntryJSON("loaf check --hook validate-push")

	output := stripANSI(runInstallCapture(t, root, "hooks", "disable", "validate-push", "--target", "cursor"))
	disabled := string(readFileBytes(t, path))

	if !strings.Contains(output, "disabled — entry removed from ~/.cursor/hooks.json") {
		t.Fatalf("hooks disable = %q, want the removal named against the file", output)
	}
	assertHooksDocumentDelta(t, converged, disabled, nil, []string{entry})

	output = stripANSI(runInstallCapture(t, root, "hooks", "enable", "validate-push", "--target", "cursor"))
	reenabled := string(readFileBytes(t, path))

	if !strings.Contains(output, "enabled — entry restored to ~/.cursor/hooks.json") {
		t.Fatalf("hooks enable = %q, want the restoration named against the file", output)
	}
	assertHooksDocumentDelta(t, disabled, reenabled, []string{entry}, nil)
}

// The verb runs the whole reconciler, so a target carrying drift the operator
// did not ask about has that drift converged too — and the output says so
// rather than reporting single-entry surgery it did not perform.
func TestRunnerHooksToggleNamesEveryActionWhenTheTargetHasDrift(t *testing.T) {
	root, home := setupHooksFixture(t, hooksFixtureCursorSources()...)
	path := hooksFixtureFilePath(home)
	weakenHooksFixtureEntry(t, path, "beforeShellExecution", "loaf check --hook validate-commit")
	restoreHooksFixtureEntry(t, path, "beforeShellExecution", map[string]any{
		"command":      "loaf check --hook validate-commit",
		"matcher":      "Bash",
		"loaf-managed": true,
	})

	output := stripANSI(runInstallCapture(t, root, "hooks", "disable", "validate-push", "--target", "cursor"))

	for _, want := range []string{
		"disabled — entry removed from ~/.cursor/hooks.json",
		"Also in this run:",
		"update hook:beforeShellExecution/validate-commit",
		"remove hook:beforeShellExecution/validate-commit",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("hooks disable = %q, want it to name %q", output, want)
		}
	}
	if strings.Contains(string(readFileBytes(t, path)), "--advisory") {
		t.Fatalf("weakened enforcement survived the verb's reconcile:\n%s", readFileBytes(t, path))
	}
}

// absorbed_at is immutable provenance, not state: toggling the hook it belongs
// to is exactly what the record exists to allow, and it must not erase how the
// record came to be.
func TestRunnerHooksToggleKeepsAbsorptionProvenance(t *testing.T) {
	root, _ := setupHooksFixtureWithState(t, func(store *state.Store) {
		if _, err := store.AbsorbAndMarkHooks(context.Background(), "cursor", "0.2.20", []state.HookEnablementRef{
			{Target: "cursor", Event: "beforeShellExecution", HookID: "validate-push"},
		}); err != nil {
			t.Fatalf("AbsorbAndMarkHooks error = %v", err)
		}
	}, hooksFixtureCursorSources()...)
	absorbed := hooksFixtureAbsorbedAt(t, "beforeShellExecution", "validate-push")

	rows := parseHooksListRows(t, runInstallCapture(t, root, "hooks", "list", "--target", "cursor"))
	if row := rows["validate-push"]; row.enablement != "disabled" || row.projection != "absent" {
		t.Fatalf("absorbed row = %#v, want the disable-intent honoured by the install that followed it", row)
	}
	if want := "(absorbed " + time.Now().UTC().Format("2006-01-02") + ")"; !strings.Contains(rows["validate-push"].note, want) {
		t.Fatalf("absorbed note = %q, want %q", rows["validate-push"].note, want)
	}

	runInstallFixture(t, root, "hooks", "enable", "validate-push", "--target", "cursor")
	runInstallFixture(t, root, "hooks", "disable", "validate-push", "--target", "cursor")

	if got := hooksFixtureAbsorbedAt(t, "beforeShellExecution", "validate-push"); got != absorbed {
		t.Fatalf("absorbed_at = %q after a round trip, want the immutable %q", got, absorbed)
	}
	rows = parseHooksListRows(t, runInstallCapture(t, root, "hooks", "list", "--target", "cursor"))
	if !strings.Contains(rows["validate-push"].note, "(absorbed ") {
		t.Fatalf("absorbed note = %q after a round trip, want the provenance still reported", rows["validate-push"].note)
	}
}

func TestRunnerHooksUnknownHookIDNamesWhatTheTargetShips(t *testing.T) {
	root, _ := setupHooksFixture(t, hooksFixtureCursorSources()...)

	err := runHooksExpectingError(t, root, "hooks", "disable", "session-start-loaf", "--target", "cursor")

	for _, want := range []string{"ships no hook \"session-start-loaf\"", "validate-commit", "validate-push"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %v, want it to contain %q", err, want)
		}
	}
}

func TestRunnerHooksTargetErrorsAreActionable(t *testing.T) {
	root, _ := setupHooksFixture(t, hooksFixtureCursorSources()...)

	for _, testCase := range []struct {
		name string
		args []string
		want string
	}{
		{name: "uninstalled", args: []string{"hooks", "disable", "validate-push", "--target", "codex"}, want: "run `loaf install --to codex` first"},
		{name: "no hooks file", args: []string{"hooks", "list", "--target", "amp"}, want: "Amp keeps no hooks file Loaf reconciles per entry; use --target cursor or --target codex"},
		// Claude Code is a harness Loaf deploys to whose hooks ride the plugin
		// rather than a shared JSON file. It is absent from the `install --to`
		// target list, which is exactly why naming it here must not read as a
		// name Loaf has never heard of.
		{name: "no hooks file outside the install target list", args: []string{"hooks", "list", "--target", "claude-code"}, want: "Claude Code keeps no hooks file Loaf reconciles per entry; use --target cursor or --target codex"},
		{name: "unknown", args: []string{"hooks", "list", "--target", "vim"}, want: "unknown target \"vim\" (valid targets: cursor, codex)"},
		{name: "missing target", args: []string{"hooks", "disable", "validate-push"}, want: "requires --target"},
		{name: "missing hook id", args: []string{"hooks", "disable", "--target", "cursor"}, want: "requires a hook id"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			err := runHooksExpectingError(t, root, testCase.args...)
			if !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("error = %v, want it to contain %q", err, testCase.want)
			}
		})
	}
}

// A stale distribution is the one failure a target can carry into the listing
// without the others deserving to lose theirs: it is reported in place, and the
// run still exits non-zero because what was printed is not the whole answer.
func TestRunnerHooksListReportsAStaleDistributionAndExitsNonZero(t *testing.T) {
	root, _ := setupHooksFixture(t, hooksFixtureCursorSources()...)
	if err := os.Remove(filepath.Join(root, "dist", "cursor", hookCatalogFile)); err != nil {
		t.Fatalf("Remove(catalog) error = %v", err)
	}

	output := runUpgradeExpectingExitError(t, root, "hooks", "list", "--target", "cursor")

	if !strings.Contains(stripANSI(output), "read hook catalog") {
		t.Fatalf("hooks list = %q, want the unreadable catalog reported in place", output)
	}
}

// The round-trip oracle is only as good as what it flattens, and an event
// section carrying no entries contributes no entry lines at all. This pins the
// section markers that make dropping one visible: without them the two
// documents below flatten identically and the exact-one-entry assertion would
// pass a reconcile that silently removed a section the file declared.
func TestHooksDocumentLinesRecordEmptyEventSections(t *testing.T) {
	withSection := `{"version":1,"hooks":{"preToolUse":[{"command":"loaf check"}],"afterFileEdit":[]}}`
	withoutSection := `{"version":1,"hooks":{"preToolUse":[{"command":"loaf check"}]}}`

	lines := hooksDocumentLines(t, withSection)
	dropped := hooksDocumentLines(t, withoutSection)

	if !containsString(lines, "event afterFileEdit") {
		t.Fatalf("document lines = %#v, want the empty section recorded", lines)
	}
	if equalStringSlices(lines, dropped) {
		t.Fatalf("dropping an empty section left the flattening unchanged: %#v", lines)
	}
}

func TestRunnerHooksHelpNamesEverySubcommand(t *testing.T) {
	root, _ := setupHooksFixture(t, hooksFixtureCursorSources()...)

	for _, testCase := range []struct {
		args  []string
		wants []string
	}{
		{args: []string{"hooks"}, wants: []string{"loaf hooks <subcommand>", "list", "enable", "disable"}},
		{args: []string{"hooks", "--help"}, wants: []string{"loaf hooks <subcommand>"}},
		{args: []string{"hooks", "list", "--help"}, wants: []string{"Usage: loaf hooks list", "--target <target>"}},
		{args: []string{"hooks", "enable", "--help"}, wants: []string{"Usage: loaf hooks enable <hook-id> --target <target>"}},
		{args: []string{"hooks", "disable", "--help"}, wants: []string{"Usage: loaf hooks disable <hook-id> --target <target>"}},
		{args: []string{"--help"}, wants: []string{"hooks         Inspect and set Loaf hook enablement per target"}},
	} {
		output := stripANSI(runInstallCapture(t, root, testCase.args...))
		for _, want := range testCase.wants {
			if !strings.Contains(output, want) {
				t.Fatalf("%v = %q, want it to contain %q", testCase.args, output, want)
			}
		}
	}
	if err := runHooksExpectingError(t, root, "hooks", "toggle"); !strings.Contains(err.Error(), "unknown hooks subcommand") {
		t.Fatalf("error = %v, want an unknown-subcommand rejection", err)
	}
}

// --- helpers -------------------------------------------------------------

type hooksListRow struct {
	hookID     string
	event      string
	enablement string
	projection string
	note       string
}

// setupHooksFixture builds an installed Cursor whose distribution carries the
// given catalog, which is what the verb resolves identity through. Nothing here
// is a source checkout: the fixture is a machine with Loaf installed on it.
func setupHooksFixture(t *testing.T, sources ...hookCatalogSource) (string, string) {
	t.Helper()
	return setupHooksFixtureWithState(t, nil, sources...)
}

// setupHooksFixtureWithState seeds the user-scoped records before the install
// runs, which is the only way to reach the states a prior generation leaves
// behind — an absorbed record predates every install this fixture can perform.
func setupHooksFixtureWithState(t *testing.T, seed func(*state.Store), sources ...hookCatalogSource) (string, string) {
	t.Helper()
	root, home := setupInstallCommandFixture(t)
	t.Setenv("LOAF_DB", filepath.Join(t.TempDir(), "loaf.sqlite"))
	if seed != nil {
		withHooksFixtureStore(t, seed)
	}
	writeInstallFile(t, filepath.Join(root, "dist", "cursor", "skills", "foundations", "SKILL.md"), "# Foundations\n")
	installTestHookDistribution(t, root, "cursor", sources...)
	runInstallFixture(t, root, "install", "--to", "cursor", "--yes")
	return root, home
}

// hooksFixtureCursorSources is a catalog wide enough to hold all four listing
// states at once, across two events so the listing has something to group.
func hooksFixtureCursorSources() []hookCatalogSource {
	return []hookCatalogSource{
		hooksFixtureCursorSource("beforeShellExecution", "validate-commit"),
		hooksFixtureCursorSource("beforeShellExecution", "validate-push"),
		hooksFixtureCursorSource("beforeShellExecution", "render-drift"),
		hooksFixtureCursorSource("beforeSubmitPrompt", "artifact-names"),
	}
}

func hooksFixtureCursorSource(event string, hookID string) hookCatalogSource {
	command := "loaf check --hook " + hookID
	return hookCatalogSource{
		event:    event,
		hookID:   hookID,
		typeName: "command",
		command:  command,
		template: map[string]any{"command": command, "matcher": "Bash", "loaf-managed": true},
	}
}

func hooksFixtureEntryJSON(command string) string {
	return `{"command":"` + command + `","loaf-managed":true,"matcher":"Bash"}`
}

func hooksFixtureFilePath(home string) string {
	return filepath.Join(home, ".cursor", "hooks.json")
}

func withHooksFixtureStore(t *testing.T, use func(*state.Store)) {
	t.Helper()
	store, err := state.OpenStore(os.Getenv("LOAF_DB"))
	if err != nil {
		t.Fatalf("OpenStore error = %v", err)
	}
	defer store.Close()
	if err := store.ApplyMigrations(context.Background()); err != nil {
		t.Fatalf("ApplyMigrations error = %v", err)
	}
	use(store)
}

func hooksFixtureAbsorbedAt(t *testing.T, event string, hookID string) string {
	t.Helper()
	var absorbed string
	withHooksFixtureStore(t, func(store *state.Store) {
		row, found, err := store.GetHookEnablement(context.Background(), "cursor", event, hookID)
		if err != nil || !found {
			t.Fatalf("GetHookEnablement = %v, %v, want the seeded record", found, err)
		}
		if row.AbsorbedAt == nil {
			t.Fatalf("record = %#v, want absorption provenance", row)
		}
		absorbed = *row.AbsorbedAt
	})
	return absorbed
}

func runHooksExpectingError(t *testing.T, root string, args ...string) error {
	t.Helper()
	var stdout bytes.Buffer
	err := Runner{Stdout: &stdout, WorkingDir: root, Executable: distributionFixtureExecutable(root)}.Run(args)
	if err == nil {
		t.Fatalf("%v error = nil, want a rejection\n%s", args, stdout.String())
	}
	return err
}

func parseHooksListRows(t *testing.T, output string) map[string]hooksListRow {
	t.Helper()
	rows := map[string]hooksListRow{}
	for _, line := range strings.Split(stripANSI(output), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 4 || (fields[2] != "enabled" && fields[2] != "disabled") {
			continue
		}
		rows[fields[0]] = hooksListRow{
			hookID:     fields[0],
			event:      fields[1],
			enablement: fields[2],
			projection: fields[3],
			note:       strings.Join(fields[4:], " "),
		}
	}
	if len(rows) == 0 {
		t.Fatalf("hooks list output has no rows:\n%s", output)
	}
	return rows
}

func readHooksFixtureFile(t *testing.T, path string) map[string]any {
	t.Helper()
	var value map[string]any
	if err := json.Unmarshal(readFileBytes(t, path), &value); err != nil {
		t.Fatalf("Unmarshal(%s) error = %v", path, err)
	}
	return value
}

func writeHooksFixtureFile(t *testing.T, path string, value map[string]any) {
	t.Helper()
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatalf("Marshal hooks file error = %v", err)
	}
	writeInstallFile(t, path, string(body)+"\n")
}

func hooksFixtureEventEntries(t *testing.T, value map[string]any, event string) []any {
	t.Helper()
	events, ok := value[hookFileEventsField].(map[string]any)
	if !ok {
		t.Fatalf("hooks file = %#v, want an events section", value)
	}
	entries, _ := events[event].([]any)
	return entries
}

func setHooksFixtureEventEntries(t *testing.T, value map[string]any, event string, entries []any) {
	t.Helper()
	events, ok := value[hookFileEventsField].(map[string]any)
	if !ok {
		t.Fatalf("hooks file = %#v, want an events section", value)
	}
	events[event] = entries
}

// restoreHooksFixtureEntry puts a Loaf entry back by hand, which is how a
// disabled-but-present row comes about.
func restoreHooksFixtureEntry(t *testing.T, path string, event string, entry map[string]any) {
	t.Helper()
	value := readHooksFixtureFile(t, path)
	setHooksFixtureEventEntries(t, value, event, append(hooksFixtureEventEntries(t, value, event), entry))
	writeHooksFixtureFile(t, path, value)
}

func deleteHooksFixtureEntry(t *testing.T, path string, event string, command string) {
	t.Helper()
	value := readHooksFixtureFile(t, path)
	// An emptied section keeps its key and stays an array: the file said the
	// event existed, and a null there is a document the reconciler refuses.
	kept := []any{}
	for _, raw := range hooksFixtureEventEntries(t, value, event) {
		if entry, ok := raw.(map[string]any); ok && entry["command"] == command {
			continue
		}
		kept = append(kept, raw)
	}
	setHooksFixtureEventEntries(t, value, event, kept)
	writeHooksFixtureFile(t, path, value)
}

// weakenHooksFixtureEntry is the hand edit Decision 3 converges: a fail-closed
// check turned advisory, which still pairs to its hook id through the stem.
func weakenHooksFixtureEntry(t *testing.T, path string, event string, command string) {
	t.Helper()
	value := readHooksFixtureFile(t, path)
	found := false
	for _, raw := range hooksFixtureEventEntries(t, value, event) {
		entry, ok := raw.(map[string]any)
		if !ok || entry["command"] != command {
			continue
		}
		entry["command"] = command + " --advisory"
		found = true
	}
	if !found {
		t.Fatalf("%s carries no entry commanding %q", path, command)
	}
	writeHooksFixtureFile(t, path, value)
}

// seedHooksFixtureForeignEntries surrounds Loaf's entries with content Loaf
// does not own — foreign entries in both event sections, a top-level field, and
// an empty section for an event this version ships nothing for — so the round
// trip has the full range of things it promises to preserve.
func seedHooksFixtureForeignEntries(t *testing.T, path string) {
	t.Helper()
	value := readHooksFixtureFile(t, path)
	value["description"] = "operator's own hooks"
	for _, event := range []string{"beforeShellExecution", "beforeSubmitPrompt"} {
		entries := hooksFixtureEventEntries(t, value, event)
		foreign := map[string]any{"command": "bash /opt/herdr/agent-state.sh " + event, "matcher": "*"}
		setHooksFixtureEventEntries(t, value, event, append([]any{foreign}, entries...))
	}
	events, ok := value[hookFileEventsField].(map[string]any)
	if !ok {
		t.Fatalf("hooks file = %#v, want an events section", value)
	}
	events["afterFileEdit"] = []any{}
	writeHooksFixtureFile(t, path, value)
}

// hooksDocumentLines flattens everything a reconcile promises to leave alone:
// each top-level field Loaf does not own, each event section the document
// declares, and each entry, in the document's own order, as its exact JSON
// value. Whitespace is normalized because the contract promises value identity
// rather than byte identity — every key, every value, and every position is
// still compared literally.
//
// The section markers are what make an emptied event section visible. Without
// them a section carrying no entries contributes nothing to the flattening, and
// dropping one — a real regression, since the file said the event existed and
// Loaf has no opinion about sections it did not create — would read as no
// difference at all.
func hooksDocumentLines(t *testing.T, body string) []string {
	t.Helper()
	order, fields, err := decodeHookJSONObject([]byte(body))
	if err != nil {
		t.Fatalf("decode hooks document error = %v", err)
	}
	var lines []string
	for _, key := range order {
		if key == hookFileEventsField {
			continue
		}
		canonical, err := canonicalHookEntry(fields[key])
		if err != nil {
			t.Fatalf("canonicalize field %q error = %v", key, err)
		}
		lines = append(lines, "field "+key+" = "+canonical)
	}
	events, sections, err := decodeHookJSONObject(fields[hookFileEventsField])
	if err != nil {
		t.Fatalf("decode hooks section error = %v", err)
	}
	for _, event := range events {
		lines = append(lines, "event "+event)
		var entries []json.RawMessage
		if err := json.Unmarshal(sections[event], &entries); err != nil {
			t.Fatalf("decode %s entries error = %v", event, err)
		}
		for _, raw := range entries {
			canonical, err := canonicalHookEntry(raw)
			if err != nil {
				t.Fatalf("canonicalize %s entry error = %v", event, err)
			}
			lines = append(lines, event+" "+canonical)
		}
	}
	return lines
}

// assertHooksDocumentDelta proves the two documents differ by exactly the named
// entries and nothing else — no other entry, no top-level field, and no event
// section. Anything that survived but moved shows up as one removal plus one
// addition, so this is an assertion about order as much as about content.
func assertHooksDocumentDelta(t *testing.T, before string, after string, wantAdded []string, wantRemoved []string) {
	t.Helper()
	beforeLines := hooksDocumentLines(t, before)
	afterLines := hooksDocumentLines(t, after)
	var added, removed []string
	i, j := 0, 0
	for i < len(beforeLines) && j < len(afterLines) {
		if beforeLines[i] == afterLines[j] {
			i++
			j++
			continue
		}
		if containsString(beforeLines[i:], afterLines[j]) {
			removed = append(removed, beforeLines[i])
			i++
			continue
		}
		added = append(added, afterLines[j])
		j++
	}
	removed = append(removed, beforeLines[i:]...)
	added = append(added, afterLines[j:]...)
	if !equalStringSlices(added, wantAdded) || !equalStringSlices(removed, wantRemoved) {
		t.Fatalf("document delta added %#v and removed %#v, want added %#v and removed %#v\nbefore:\n%s\nafter:\n%s", added, removed, wantAdded, wantRemoved, before, after)
	}
}

func equalStringSlices(got []string, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for index := range got {
		if got[index] != want[index] {
			return false
		}
	}
	return true
}
