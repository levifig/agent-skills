package cli

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/levifig/loaf/internal/state"
)

// testHookPriorVersion is the released generation this migration absorbs, as
// the canary's files carry it.
const testHookPriorVersion = "0.2.20"

var errHookCrashInjection = errors.New("injected crash before projection")

// A fresh install has no prior install to absorb from: every catalog hook
// projects enabled, the file is created, and the marker closes the migration
// window so a later hand-deletion is never mistaken for intent.
func TestHookReconcileFreshInstallEnablesEverything(t *testing.T) {
	fixture := newCursorHookFixture(t)

	actions := fixture.apply(t)

	for _, action := range actions {
		if action.action != hookActionAdd {
			t.Fatalf("fresh install took %s on %s, want adds only", action.action, action.id())
		}
	}
	entries := fixture.hookIDsByEvent(t)
	if len(entries["sessionStart"]) != 1 || entries["sessionStart"][0] != "session-start-loaf" {
		t.Fatalf("sessionStart = %v, want the shipped session hook", entries["sessionStart"])
	}
	if _, marked, err := fixture.store.GetHookAbsorptionMarker(t.Context(), "cursor"); err != nil || !marked {
		t.Fatalf("absorption marker = %v, %v, want the fresh install to close the migration window", marked, err)
	}
	if rows, err := fixture.store.ListHookEnablements(t.Context(), "cursor"); err != nil || len(rows) != 0 {
		t.Fatalf("enablement rows = %#v, %v, want nothing recorded on a fresh install", rows, err)
	}
}

// The canary's Codex file: the operator emptied it before this release, so the
// hook the prior version shipped is absorbed as disabled instead of re-added,
// and the third-party group is untouched.
func TestHookReconcileAbsorbsTheDeletedCodexHook(t *testing.T) {
	fixture := newCodexHookFixture(t)
	fixture.writeHooks(t, string(testHookFixture(t, "codex-hooks-live.json")))
	fixture.writeInstalledManifest(t, testHookPriorVersion)

	actions := fixture.apply(t)

	if len(actions) != 1 || actions[0].action != hookActionAbsorb || actions[0].hookID != "session-start-loaf" {
		t.Fatalf("actions = %#v, want one absorption of session-start-loaf", actions)
	}
	row, found, err := fixture.store.GetHookEnablement(t.Context(), "codex", "SessionStart", "session-start-loaf")
	if err != nil || !found {
		t.Fatalf("GetHookEnablement = %v, %v, want an absorbed record", found, err)
	}
	if row.Enablement != state.HookEnablementDisabled || row.AbsorbedAt == nil {
		t.Fatalf("absorbed record = %#v, want disabled with immutable absorption provenance", row)
	}
	// The file was never touched: absorption is a record, not an edit.
	fixture.assertHooksUnchanged(t, string(testHookFixture(t, "codex-hooks-live.json")))
	fixture.assertForeignSurvives(t, "SessionStart", "bash '/Users/canary/.config/codex/herdr-agent-state.sh' session")
}

// A second run of the same upgrade decides nothing: convergence is a fixed
// point, and the quiet upgrade is the normal one.
func TestHookReconcileIsIdempotent(t *testing.T) {
	fixture := newCodexHookFixture(t)
	fixture.writeHooks(t, string(testHookFixture(t, "codex-hooks-live.json")))
	fixture.writeInstalledManifest(t, testHookPriorVersion)

	fixture.apply(t)
	before := fixture.readHooks(t)
	actions := fixture.apply(t)

	if len(actions) != 0 {
		t.Fatalf("second reconcile actions = %#v, want none", actions)
	}
	if after := fixture.readHooks(t); after != before {
		t.Fatalf("second reconcile rewrote the file:\n%s\n%s", before, after)
	}
}

// Every entry Loaf does not own survives a reconcile value-identical and in the
// order it was written — the 32-entry legacy generation and the third-party
// herdr hook alike.
func TestHookReconcilePreservesEveryForeignCursorEntry(t *testing.T) {
	fixture := newCursorHookFixture(t)
	live := string(testHookFixture(t, "cursor-hooks-live.json"))
	fixture.writeHooks(t, live)
	fixture.writeInstalledManifest(t, testHookPriorVersion)

	fixture.apply(t)

	before := testHookDocumentSnapshot(t, []byte(live), fixture.recognition(t))
	after := testHookDocumentSnapshot(t, []byte(fixture.readHooks(t)), fixture.recognition(t))
	if total := testHookCountForeign(before); total != 33 {
		t.Fatalf("live Cursor fixture has %d foreign entries, want 33", total)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("the preserved half of the document changed across the reconcile:\nbefore %#v\nafter  %#v", before, after)
	}
}

// The same guarantee stated against the Codex file, whose top-level fields and
// third-party group must survive the absorption that touches neither.
func TestHookReconcilePreservesTheCodexDocumentAroundAnAbsorption(t *testing.T) {
	fixture := newCodexHookFixture(t)
	live := `{"description":"canary hooks","hooks":{"SessionStart":[{"hooks":[{"command":"bash '/Users/canary/.config/codex/herdr-agent-state.sh' session","timeout":10,"type":"command"}]}],"Stop":[]}}`
	fixture.writeHooks(t, live)
	fixture.writeInstalledManifest(t, testHookPriorVersion)

	before := testHookDocumentSnapshot(t, []byte(live), fixture.recognition(t))
	fixture.apply(t)
	after := testHookDocumentSnapshot(t, []byte(fixture.readHooks(t)), fixture.recognition(t))

	if !reflect.DeepEqual(before, after) {
		t.Fatalf("the preserved half of the document changed across the reconcile:\nbefore %#v\nafter  %#v", before, after)
	}
}

// Codex handler shapes Loaf has no model of at all. The whole-file merge used
// to parse these against its own schema and refuse the ones it could not
// classify; reconciliation carries them as the raw values they are, so a prompt
// handler, an agent handler, a command handler with current-schema fields Loaf
// never writes, and the degenerate groups all survive value-identical and in
// position — through a reconcile that does write, so the guarantee is about
// what was republished rather than about a file nobody touched.
func TestHookReconcilePreservesForeignCodexHandlerShapes(t *testing.T) {
	fixture := newCodexHookFixture(t)
	live := `{"description":"canary hooks","hooks":{"SessionStart":[` +
		`{},` +
		`{"matcher":null},` +
		`{"matcher":"resume","hooks":[{"type":"prompt"}]},` +
		`{"matcher":"clear","hooks":[{"type":"agent"}]},` +
		`{"matcher":"compact","hooks":[{"type":"command","command":"user hook","command_windows":"powershell user hook","timeout":0,"async":true,"statusMessage":"checking"}]}` +
		`],"Stop":[]}}`
	fixture.writeHooks(t, live)
	before := testHookDocumentSnapshot(t, []byte(live), fixture.recognition(t))

	actions := fixture.apply(t)

	if !testHookHasAnyAction(actions, hookActionAdd) {
		t.Fatalf("actions = %s, want the reconcile to have written the file", describeHookActions(actions))
	}
	after := testHookDocumentSnapshot(t, []byte(fixture.readHooks(t)), fixture.recognition(t))
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("a foreign handler shape did not survive the reconcile:\nbefore %#v\nafter  %#v", before, after)
	}
	for _, shape := range []string{`"type":"prompt"`, `"type":"agent"`, `"command_windows":"powershell user hook"`, `"statusMessage":"checking"`} {
		if !strings.Contains(strings.ReplaceAll(fixture.readHooks(t), " ", ""), strings.ReplaceAll(shape, " ", "")) {
			t.Fatalf("hooks file lost %s:\n%s", shape, fixture.readHooks(t))
		}
	}
}

// The goldens above prove the post-verify comparison stays silent when nothing
// moved, which is only half of what it is for. These are the mutations it has
// to name — and the middle two are the reason it compares values in order
// rather than counting: a swap and a removal paired with an insertion both
// leave the count exactly right.
func TestHookForeignDriftIsDetectedByValueAndOrder(t *testing.T) {
	entries := []string{"SessionStart {\"a\":1}", "SessionStart {\"b\":2}", "Stop {\"c\":3}"}
	for _, testCase := range []struct {
		name  string
		after []string
		want  string
	}{
		{name: "nothing moved", after: entries, want: ""},
		{name: "value rewritten", after: []string{entries[0], "SessionStart {\"b\":99}", entries[2]}, want: "became"},
		{name: "order swapped", after: []string{entries[1], entries[0], entries[2]}, want: "became"},
		{name: "one replaced by another", after: []string{entries[0], entries[2], "Stop {\"d\":4}"}, want: "became"},
		{name: "entry gone", after: entries[:2], want: "is gone"},
		{name: "entry appeared", after: append(append([]string{}, entries...), "Stop {\"e\":5}"), want: "appeared"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			difference := describeHookForeignDrift(entries, testCase.after)
			if testCase.want == "" {
				if difference != "" {
					t.Fatalf("describeHookForeignDrift() = %q, want no reported drift", difference)
				}
				return
			}
			if !strings.Contains(difference, testCase.want) {
				t.Fatalf("describeHookForeignDrift() = %q, want it to report %q", difference, testCase.want)
			}
		})
	}
}

// The live Cursor file is already converged for its Loaf entries, so a
// reconcile of it decides nothing at all.
func TestHookReconcileLeavesTheConvergedCursorFileAlone(t *testing.T) {
	fixture := newCursorHookFixture(t)
	live := string(testHookFixture(t, "cursor-hooks-live.json"))
	fixture.writeHooks(t, live)
	fixture.writeInstalledManifest(t, testHookPriorVersion)

	actions := fixture.apply(t)

	if len(actions) != 0 {
		t.Fatalf("actions = %s, want a converged file to decide nothing", describeHookActions(actions))
	}
}

// Decision 3: a Loaf entry someone weakened converges back to the shipped shape
// instead of refusing the file. There is no drift verdict left to reach.
func TestHookReconcileConvergesAWeakenedEntryInsteadOfRefusing(t *testing.T) {
	fixture := newCursorHookFixture(t)
	fixture.apply(t)
	fixture.mutateEntry(t, "preToolUse", "check-"+"sec"+"rets", func(entry map[string]any) {
		entry["command"] = "loaf check --hook check-" + "sec" + "rets --advisory"
		delete(entry, "failClosed")
	})

	actions := fixture.apply(t)

	if len(actions) != 1 || actions[0].action != hookActionUpdate || actions[0].hookID != "check-"+"sec"+"rets" {
		t.Fatalf("actions = %#v, want one update of the weakened entry", actions)
	}
	if strings.Contains(fixture.readHooks(t), "check-"+"sec"+"rets --advisory") {
		t.Fatalf("weakened enforcement survived the reconcile:\n%s", fixture.readHooks(t))
	}
}

// A hand-deleted entry after absorption is not a disable gesture: the file is a
// projection of the records, so the next reconcile puts it back.
func TestHookReconcileReaddsAHandDeletedEntryAfterAbsorption(t *testing.T) {
	fixture := newCodexHookFixture(t)
	fixture.apply(t)
	fixture.writeHooks(t, `{"hooks":{"SessionStart":[]}}`)

	actions := fixture.apply(t)

	if len(actions) != 1 || actions[0].action != hookActionAdd || actions[0].hookID != "session-start-loaf" {
		t.Fatalf("actions = %#v, want the deleted entry re-added", actions)
	}
}

// A disabled record removes the entry and keeps it out, which is what makes the
// verb surface a projection rather than surgery.
func TestHookReconcileRemovesADisabledHookAndKeepsItOut(t *testing.T) {
	fixture := newCodexHookFixture(t)
	fixture.apply(t)
	if _, err := fixture.store.SetHookEnablement(t.Context(), "codex", "SessionStart", "session-start-loaf", false); err != nil {
		t.Fatalf("SetHookEnablement error = %v", err)
	}

	actions := fixture.apply(t)
	if len(actions) != 1 || actions[0].action != hookActionRemove || actions[0].detail != "disabled" {
		t.Fatalf("actions = %#v, want the disabled entry removed", actions)
	}
	if actions := fixture.apply(t); len(actions) != 0 {
		t.Fatalf("actions = %#v, want a disabled hook to stay out", actions)
	}
}

// Duplicates are Loaf's by construction: the first survives converged and the
// extras go, with no refusal and no fork. A legacy entry that pairs to no
// current identity is a retired generation and goes the same way.
func TestHookReconcileRemovesDuplicateAndRetiredOwnedEntries(t *testing.T) {
	fixture := newCursorHookFixture(t)
	fixture.apply(t)
	converged := fixture.decodeHooks(t)
	events := converged["hooks"].(map[string]any)
	entries := events["preToolUse"].([]any)
	events["preToolUse"] = append(entries, entries[0], map[string]any{"command": "loaf session start"})
	fixture.writeHooksValue(t, converged)

	actions := fixture.apply(t)

	if len(actions) != 2 {
		t.Fatalf("actions = %s, want the duplicate and the retired generation removed", describeHookActions(actions))
	}
	for _, action := range actions {
		if action.action != hookActionRemove {
			t.Fatalf("actions = %s, want removals only", describeHookActions(actions))
		}
	}
	if got, want := len(fixture.decodeHooks(t)["hooks"].(map[string]any)["preToolUse"].([]any)), len(entries); got != want {
		t.Fatalf("preToolUse has %d entries after the reconcile, want the converged %d", got, want)
	}
}

// The migration matrix. Each case is one shape of "what was here before", and
// absorption fires exactly once, for the generation that shipped it.
func TestHookReconcileMigrationMatrix(t *testing.T) {
	t.Run("no-manifest legacy upgrade absorbs the deleted cohort hook", func(t *testing.T) {
		fixture := newCursorHookFixture(t)
		fixture.writeHooks(t, testHookCursorFileWithout(t, "session-start-loaf"))

		actions := fixture.apply(t)

		if !testHookHasAction(actions, hookActionAbsorb, "session-start-loaf") {
			t.Fatalf("actions = %s, want the deleted 0.2.20 hook absorbed without a manifest", describeHookActions(actions))
		}
		marker, _, err := fixture.store.GetHookAbsorptionMarker(t.Context(), "cursor")
		if err != nil {
			t.Fatalf("GetHookAbsorptionMarker error = %v", err)
		}
		if marker.AbsorbedFromVersion != "unknown" {
			t.Fatalf("marker version = %q, want the unidentifiable prior install named as such", marker.AbsorbedFromVersion)
		}
	})

	t.Run("normal upgrade absorbs from the recorded prior version", func(t *testing.T) {
		fixture := newCursorHookFixture(t)
		fixture.writeHooks(t, testHookCursorFileWithout(t, "session-start-loaf"))
		fixture.writeInstalledManifest(t, testHookPriorVersion)

		actions := fixture.apply(t)

		if !testHookHasAction(actions, hookActionAbsorb, "session-start-loaf") {
			t.Fatalf("actions = %s, want the deleted hook absorbed", describeHookActions(actions))
		}
		marker, _, err := fixture.store.GetHookAbsorptionMarker(t.Context(), "cursor")
		if err != nil {
			t.Fatalf("GetHookAbsorptionMarker error = %v", err)
		}
		if marker.AbsorbedFromVersion != testHookPriorVersion {
			t.Fatalf("marker version = %q, want %q", marker.AbsorbedFromVersion, testHookPriorVersion)
		}
	})

	t.Run("repeat upgrade absorbs nothing", func(t *testing.T) {
		fixture := newCursorHookFixture(t)
		fixture.writeHooks(t, testHookCursorFileWithout(t, "session-start-loaf"))
		fixture.writeInstalledManifest(t, testHookPriorVersion)
		fixture.apply(t)

		// The operator deletes another entry between runs. After absorption a
		// deletion says nothing, so it comes back.
		fixture.deleteEntry(t, "preToolUse", "check-"+"sec"+"rets")
		actions := fixture.apply(t)

		if testHookHasAnyAction(actions, hookActionAbsorb) {
			t.Fatalf("actions = %s, want absorption to have run once", describeHookActions(actions))
		}
		if !testHookHasAction(actions, hookActionAdd, "check-"+"sec"+"rets") {
			t.Fatalf("actions = %s, want the hand-deleted entry re-added", describeHookActions(actions))
		}
	})

	t.Run("reinstall after the marker exists absorbs nothing", func(t *testing.T) {
		fixture := newCursorHookFixture(t)
		fixture.apply(t)
		if err := os.Remove(fixture.hooks); err != nil {
			t.Fatalf("Remove(hooks) error = %v", err)
		}

		actions := fixture.apply(t)

		if testHookHasAnyAction(actions, hookActionAbsorb) {
			t.Fatalf("actions = %s, want a reinstall to project rather than absorb", describeHookActions(actions))
		}
		if !testHookHasAnyAction(actions, hookActionAdd) {
			t.Fatalf("actions = %s, want the catalog reprojected", describeHookActions(actions))
		}
	})

	t.Run("downgrade then re-upgrade absorbs nothing", func(t *testing.T) {
		fixture := newCursorHookFixture(t)
		fixture.writeInstalledManifest(t, testHookPriorVersion)
		fixture.apply(t)
		// A downgrade rewrites the installed manifest to the older version and
		// takes its entries with it; the marker outlives both.
		fixture.writeInstalledManifest(t, "0.2.19")
		fixture.writeHooks(t, testHookCursorFileWithout(t, "session-start-loaf"))

		actions := fixture.apply(t)

		if testHookHasAnyAction(actions, hookActionAbsorb) {
			t.Fatalf("actions = %s, want the durable marker to hold across a downgrade", describeHookActions(actions))
		}
	})

	t.Run("a manifest naming an unenumerated version absorbs nothing", func(t *testing.T) {
		fixture := newCursorHookFixture(t)
		fixture.writeHooks(t, testHookCursorFileWithout(t, "session-start-loaf"))
		// The install is identified and its cohort is not recorded, so nothing
		// its file lacks is evidence of anything.
		fixture.writeInstalledManifest(t, "0.1.4")

		actions := fixture.apply(t)

		if testHookHasAnyAction(actions, hookActionAbsorb) {
			t.Fatalf("actions = %s, want no absorption from an unenumerated cohort", describeHookActions(actions))
		}
		if !testHookHasAction(actions, hookActionAdd, "session-start-loaf") {
			t.Fatalf("actions = %s, want the missing hook projected as enabled", describeHookActions(actions))
		}
		marker, marked, err := fixture.store.GetHookAbsorptionMarker(t.Context(), "cursor")
		if err != nil || !marked {
			t.Fatalf("marker = %v, %v, want the migration window closed anyway", marked, err)
		}
		if marker.AbsorbedFromVersion != "0.1.4" {
			t.Fatalf("marker version = %q, want the identified prior version recorded", marker.AbsorbedFromVersion)
		}
	})

	// The canary is alpha.19: its Codex manifest still says so because the drift
	// refusal this Change removes is what stopped the manifest from being
	// rewritten, and that release is 0.2.19 under the old spelling — the same
	// generation 0.2.20 shipped. Reading it as an unknown version is what made
	// the first dry-run offer to re-add the hook the operator deleted.
	//
	// Every enumerated release is exercised, and the list below is written out
	// rather than read from hookCatalogPreResetVersions on purpose: a test that
	// iterated the production slice would lose a case exactly when a literal
	// went missing from it, which is the regression this is here to catch.
	t.Run("a manifest naming a pre-reset release absorbs its cohort", func(t *testing.T) {
		enumerated := []string{
			"2.0.0-alpha.14",
			"2.0.0-alpha.15",
			"2.0.0-alpha.16",
			"2.0.0-alpha.17",
			"2.0.0-alpha.18",
			"2.0.0-alpha.19",
		}
		// Drift in the other direction — a version added to the catalog with no
		// case here — would otherwise go unexercised.
		if len(enumerated) != len(hookCatalogPreResetVersions) {
			t.Fatalf("catalog enumerates %d pre-reset releases, this table covers %d; they have to move together", len(hookCatalogPreResetVersions), len(enumerated))
		}
		for _, version := range enumerated {
			t.Run(version, func(t *testing.T) {
				fixture := newCodexHookFixture(t)
				fixture.writeHooks(t, string(testHookFixture(t, "codex-hooks-live.json")))
				fixture.writeInstalledManifest(t, version)

				if cohort := hookAbsorptionCohort(fixture.catalog(t), version, true); len(cohort) == 0 {
					t.Fatalf("cohort for %s is empty, want the enumerated generation", version)
				}

				actions := fixture.apply(t)

				if !testHookHasAction(actions, hookActionAbsorb, "session-start-loaf") {
					t.Fatalf("actions = %s, want the deleted hook absorbed from the pre-reset cohort", describeHookActions(actions))
				}
				if testHookHasAction(actions, hookActionAdd, "session-start-loaf") {
					t.Fatalf("actions = %s, want no re-add of the hook the operator deleted", describeHookActions(actions))
				}
				row, found, err := fixture.store.GetHookEnablement(t.Context(), "codex", "SessionStart", "session-start-loaf")
				if err != nil || !found || row.Enablement != state.HookEnablementDisabled {
					t.Fatalf("record = %#v, %v, %v, want the disable intent recorded", row, found, err)
				}
				marker, marked, err := fixture.store.GetHookAbsorptionMarker(t.Context(), "codex")
				if err != nil || !marked || marker.AbsorbedFromVersion != version {
					t.Fatalf("marker = %#v, %v, %v, want %s recorded as provenance", marker, marked, err, version)
				}
			})
		}
	})

	// The enumeration stops where the evidence does. alpha.13 shipped 16 Cursor
	// entries, so a hook it never had is not something its operator deleted.
	t.Run("a pre-reset release below the enumerated floor absorbs nothing", func(t *testing.T) {
		fixture := newCodexHookFixture(t)
		fixture.writeHooks(t, string(testHookFixture(t, "codex-hooks-live.json")))
		fixture.writeInstalledManifest(t, "2.0.0-alpha.13")

		actions := fixture.apply(t)

		if testHookHasAnyAction(actions, hookActionAbsorb) {
			t.Fatalf("actions = %s, want no absorption below the enumerated floor", describeHookActions(actions))
		}
		if !testHookHasAction(actions, hookActionAdd, "session-start-loaf") {
			t.Fatalf("actions = %s, want the hook projected as enabled instead", describeHookActions(actions))
		}
	})

	// The family is an enumeration, not a loosening: a version nobody recorded
	// still absorbs nothing, whichever spelling it uses.
	t.Run("an unenumerated future version still absorbs nothing", func(t *testing.T) {
		for _, version := range []string{"0.2.25", "2.0.0-alpha.20", "2.0.0-dev.49", "2.0.0-pre.20260614235428"} {
			t.Run(version, func(t *testing.T) {
				fixture := newCodexHookFixture(t)
				fixture.writeHooks(t, string(testHookFixture(t, "codex-hooks-live.json")))
				fixture.writeInstalledManifest(t, version)

				actions := fixture.apply(t)

				if testHookHasAnyAction(actions, hookActionAbsorb) {
					t.Fatalf("actions = %s, want no absorption for an unenumerated version", describeHookActions(actions))
				}
			})
		}
	})

	t.Run("only the prior cohort absorbs", func(t *testing.T) {
		fixture := newCursorHookFixture(t)
		// One hook the prior version shipped and one introduced after it, both
		// absent. Absence of a hook that was never installed says nothing.
		fixture.writeHooks(t, testHookCursorFileWithout(t, "session-start-loaf", "artifact-names"))
		fixture.writeInstalledManifest(t, testHookPriorVersion)
		reconciler := fixture.reconciler(t)
		reconciler.catalog = testHookCatalogWithCohort(t, fixture.catalog(t), "artifact-names")
		actions, err := reconciler.apply(t.Context())
		if err != nil {
			t.Fatalf("apply error = %v", err)
		}

		if !testHookHasAction(actions, hookActionAbsorb, "session-start-loaf") {
			t.Fatalf("actions = %s, want the prior cohort's hook absorbed", describeHookActions(actions))
		}
		if testHookHasAction(actions, hookActionAbsorb, "artifact-names") {
			t.Fatalf("actions = %s, want a hook outside the prior cohort projected, not absorbed", describeHookActions(actions))
		}
		if !testHookHasAction(actions, hookActionAdd, "artifact-names") {
			t.Fatalf("actions = %s, want the newly introduced hook added enabled", describeHookActions(actions))
		}
	})
}

// The integrity preconditions. Every one of them preserves the file exactly as
// written and says which file and why.
func TestHookReconcileIntegrityPreconditionsPreserveTheFile(t *testing.T) {
	for _, testCase := range []struct {
		name string
		body string
		want string
	}{
		{name: "malformed JSON", body: `{"hooks":`, want: "parse hooks file"},
		{name: "non-object top level", body: `["hooks"]`, want: "parse hooks file"},
		{name: "null top level", body: `null`, want: "parse hooks file"},
		{name: "hooks is not an object", body: `{"hooks":[]}`, want: `"hooks" must be an object`},
		{name: "event is not an array", body: `{"hooks":{"SessionStart":{}}}`, want: "must be an array"},
		{name: "event is null", body: `{"hooks":{"SessionStart":null}}`, want: "must be an array"},
		{name: "entry is not an object", body: `{"hooks":{"SessionStart":["hook"]}}`, want: "must be an object"},
		{name: "duplicate keys", body: `{"hooks":{},"hooks":{}}`, want: "parse hooks file"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			fixture := newCodexHookFixture(t)
			fixture.writeHooks(t, testCase.body)

			_, err := fixture.reconciler(t).apply(t.Context())
			if err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("apply error = %v, want a refusal naming %q", err, testCase.want)
			}
			if !strings.Contains(err.Error(), "preserving it as written") {
				t.Fatalf("apply error = %v, want the preservation promise stated", err)
			}
			fixture.assertHooksUnchanged(t, testCase.body)
		})
	}
}

func TestHookReconcileRefusesASymlinkedHooksFile(t *testing.T) {
	fixture := newCodexHookFixture(t)
	target := filepath.Join(fixture.root, "elsewhere.json")
	writeInstallFile(t, target, `{"hooks":{}}`)
	if err := os.MkdirAll(filepath.Dir(fixture.hooks), 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}
	if err := os.Symlink(target, fixture.hooks); err != nil {
		t.Fatalf("Symlink error = %v", err)
	}

	_, err := fixture.reconciler(t).apply(t.Context())
	if err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("apply error = %v, want a non-regular-destination refusal", err)
	}
	assertInstallFile(t, target, `{"hooks":{}}`)
}

func TestHookReconcileRefusesAnUnreadableHooksFile(t *testing.T) {
	fixture := newCodexHookFixture(t)
	fixture.writeHooks(t, `{"hooks":{}}`)
	if err := os.Chmod(fixture.hooks, 0o000); err != nil {
		t.Fatalf("Chmod error = %v", err)
	}
	t.Cleanup(func() { os.Chmod(fixture.hooks, 0o644) })

	_, err := fixture.reconciler(t).apply(t.Context())
	if err == nil || !strings.Contains(err.Error(), "read hooks file") {
		t.Fatalf("apply error = %v, want the read failure reported", err)
	}
}

// A writer that does not honour the lock is caught by the pre-rename
// comparison: the publication aborts and the third party's bytes stand.
func TestHookReconcileAbortsOnConcurrentModification(t *testing.T) {
	fixture := newCodexHookFixture(t)
	fixture.writeHooks(t, `{"hooks":{"SessionStart":[]}}`)
	third := `{"hooks":{"SessionStart":[{"matcher":"startup","hooks":[{"type":"command","command":"third party"}]}]}}`

	reconciler := fixture.reconciler(t)
	reconciler.operations = &hookReconcileOperations{beforeRename: func() error {
		writeInstallFile(t, fixture.hooks, third)
		return nil
	}}
	_, err := reconciler.apply(t.Context())

	if err == nil || !strings.Contains(err.Error(), "changed while Loaf was reconciling it") {
		t.Fatalf("apply error = %v, want a concurrent-modification abort", err)
	}
	fixture.assertHooksUnchanged(t, third)
}

// Decision 10's retry-safe window: records commit, the process dies before the
// file is written, and the next reconcile converges the file to the records.
func TestHookReconcileConvergesAfterACrashBetweenRecordsAndProjection(t *testing.T) {
	fixture := newCodexHookFixture(t)
	fixture.writeHooks(t, string(testHookFixture(t, "codex-hooks-live.json")))
	fixture.writeInstalledManifest(t, testHookPriorVersion)

	reconciler := fixture.reconciler(t)
	reconciler.operations = &hookReconcileOperations{afterRecords: func() error {
		return errHookCrashInjection
	}}
	if _, err := reconciler.apply(t.Context()); err == nil {
		t.Fatal("apply error = nil, want the injected crash")
	}
	if _, marked, err := fixture.store.GetHookAbsorptionMarker(t.Context(), "codex"); err != nil || !marked {
		t.Fatalf("marker = %v, %v, want the records committed before the crash", marked, err)
	}

	actions := fixture.apply(t)
	if len(actions) != 0 {
		t.Fatalf("actions = %s, want the retried reconcile to find the records already absorbed", describeHookActions(actions))
	}
	fixture.assertForeignSurvives(t, "SessionStart", "bash '/Users/canary/.config/codex/herdr-agent-state.sh' session")
}

// The same window with entries to write rather than records to absorb: the
// file is one reconcile behind the records, and the retry closes the gap
// without the crashed run having left anything half-written.
func TestHookReconcileRetriesAfterACrashBeforeTheFileIsWritten(t *testing.T) {
	fixture := newCursorHookFixture(t)
	reconciler := fixture.reconciler(t)
	reconciler.operations = &hookReconcileOperations{afterRecords: func() error {
		return errHookCrashInjection
	}}
	if _, err := reconciler.apply(t.Context()); !errors.Is(err, errHookCrashInjection) {
		t.Fatalf("apply error = %v, want the injected crash", err)
	}
	if _, err := os.Stat(fixture.hooks); !os.IsNotExist(err) {
		t.Fatalf("hooks file stat = %v, want nothing written before the crash", err)
	}

	actions := fixture.apply(t)
	if !testHookHasAnyAction(actions, hookActionAdd) {
		t.Fatalf("actions = %s, want the retry to project the catalog", describeHookActions(actions))
	}
	if actions := fixture.apply(t); len(actions) != 0 {
		t.Fatalf("actions = %s, want the retried run to have converged", describeHookActions(actions))
	}
}

// One writer at a time. The verb and an upgrade are the same code path, so this
// is the interleaving the shape names, exercised with the reconciler itself.
func TestHookReconcileSerializesConcurrentWriters(t *testing.T) {
	fixture := newCursorHookFixture(t)
	lock, err := acquireHookFileLock(fixture.hooks, hookFileLockWait)
	if err != nil {
		t.Fatalf("acquireHookFileLock error = %v", err)
	}

	reconciler := fixture.reconciler(t)
	reconciler.lockWait = 20 * time.Millisecond
	_, err = reconciler.apply(t.Context())
	if err == nil || !strings.Contains(err.Error(), "another Loaf process is reconciling") {
		t.Fatalf("apply error = %v, want an actionable contention failure", err)
	}
	if _, statErr := os.Stat(fixture.hooks); !os.IsNotExist(statErr) {
		t.Fatalf("hooks file stat = %v, want the contended run to write nothing", statErr)
	}

	if err := lock.release(); err != nil {
		t.Fatalf("release error = %v", err)
	}
	if actions := fixture.apply(t); len(actions) == 0 {
		t.Fatal("apply after release took no actions, want the reconcile to proceed")
	}
}

// Serialization, not just refusal: a writer that arrives while another holds
// the lock waits for it and then does its work against the file the first one
// left behind.
func TestHookReconcileWaitsForTheLockAndThenProceeds(t *testing.T) {
	fixture := newCodexHookFixture(t)
	lock, err := acquireHookFileLock(fixture.hooks, hookFileLockWait)
	if err != nil {
		t.Fatalf("acquireHookFileLock error = %v", err)
	}

	released := make(chan struct{})
	go func() {
		time.Sleep(30 * time.Millisecond)
		close(released)
		if err := lock.release(); err != nil {
			panic(err)
		}
	}()

	actions, err := fixture.reconciler(t).apply(t.Context())
	<-released
	if err != nil {
		t.Fatalf("apply error = %v, want the waiting writer to proceed", err)
	}
	if !testHookHasAnyAction(actions, hookActionAdd) {
		t.Fatalf("actions = %s, want the waiting writer to project the catalog", describeHookActions(actions))
	}
}

// Windows parity is concrete rather than asserted: the entry the reconciler
// writes there carries command and commandWindows with the same cmd.exe form,
// and recognizing it back is what makes the second run a no-op.
func TestHookReconcileProjectsWindowsCommandParity(t *testing.T) {
	fixture := newCodexHookFixture(t)
	executable := `C:\Users\canary\AppData\Local\loaf\loaf.exe`
	windows := func() *hookReconciler {
		reconciler := fixture.reconciler(t)
		reconciler.goos = "windows"
		reconciler.homeDir = `C:\Users\canary`
		reconciler.resolveExecutable = func() (string, error) { return executable, nil }
		return reconciler
	}

	if _, err := windows().apply(t.Context()); err != nil {
		t.Fatalf("apply error = %v", err)
	}

	file, err := readHookFile(fixture.hooks)
	if err != nil {
		t.Fatalf("readHookFile error = %v", err)
	}
	entries, err := file.eventEntries("SessionStart")
	if err != nil {
		t.Fatalf("eventEntries error = %v", err)
	}
	handler := entries[0]["hooks"].([]any)[0].(map[string]any)
	want, err := codexWindowsJournalContextCommand(executable)
	if err != nil {
		t.Fatalf("codexWindowsJournalContextCommand error = %v", err)
	}
	if handler["command"] != want || handler["commandWindows"] != want {
		t.Fatalf("handler = %#v, want command and commandWindows both %q", handler, want)
	}

	actions, err := windows().apply(t.Context())
	if err != nil {
		t.Fatalf("second apply error = %v", err)
	}
	if len(actions) != 0 {
		t.Fatalf("actions = %s, want the Windows projection recognized as converged", describeHookActions(actions))
	}

	// Loaf moves. The entry written against the old path is still recognized —
	// the recorded trusted path says so — and converges to the new one in place
	// rather than being orphaned beside a second group.
	rotated := `C:\Program Files\loaf\loaf.exe`
	moved := windows()
	moved.resolveExecutable = func() (string, error) { return rotated, nil }
	actions, err = moved.apply(t.Context())
	if err != nil {
		t.Fatalf("rotated apply error = %v", err)
	}
	if len(actions) != 1 || actions[0].action != hookActionUpdate {
		t.Fatalf("actions = %s, want the moved executable converged in place", describeHookActions(actions))
	}
	file, err = readHookFile(fixture.hooks)
	if err != nil {
		t.Fatalf("readHookFile error = %v", err)
	}
	entries, err = file.eventEntries("SessionStart")
	if err != nil {
		t.Fatalf("eventEntries error = %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("entries = %#v, want the rotation to converge one entry rather than add another", entries)
	}
	handler = entries[0]["hooks"].([]any)[0].(map[string]any)
	want, err = codexWindowsJournalContextCommand(rotated)
	if err != nil {
		t.Fatalf("codexWindowsJournalContextCommand error = %v", err)
	}
	if handler["command"] != want || handler["commandWindows"] != want {
		t.Fatalf("handler = %#v, want both command fields rotated to %q", handler, want)
	}
}

// The race the shape names: an upgrade that reads enabled must not write the
// stale add if a disable commits first. Apply recomputes inside the lock, so
// the record the operator just wrote wins.
func TestHookReconcileRecomputesActionsInsideTheLock(t *testing.T) {
	fixture := newCodexHookFixture(t)
	fixture.apply(t)
	stale := fixture.reconciler(t)

	// A disable lands between the plan and the apply, exactly as the verb would
	// write it while an upgrade was starting.
	if _, err := fixture.store.SetHookEnablement(t.Context(), "codex", "SessionStart", "session-start-loaf", false); err != nil {
		t.Fatalf("SetHookEnablement error = %v", err)
	}
	actions, err := stale.apply(t.Context())
	if err != nil {
		t.Fatalf("apply error = %v", err)
	}
	if len(actions) != 1 || actions[0].action != hookActionRemove {
		t.Fatalf("actions = %s, want the fresh disable honoured", describeHookActions(actions))
	}
}

// A recorded install path keeps recognizing the entries written before Loaf
// moved, and records never leak across targets.
func TestHookReconcileRecordsTrustedExecutablePathsPerTarget(t *testing.T) {
	fixture := newCursorHookFixture(t)
	reconciler := fixture.reconciler(t)
	reconciler.resolveExecutable = func() (string, error) { return "/opt/homebrew/bin/loaf", nil }
	if _, err := reconciler.apply(t.Context()); err != nil {
		t.Fatalf("first apply error = %v", err)
	}

	relocated := fixture.reconciler(t)
	relocated.resolveExecutable = func() (string, error) { return "/usr/local/bin/loaf", nil }
	if _, err := relocated.apply(t.Context()); err != nil {
		t.Fatalf("relocated apply error = %v", err)
	}

	paths, err := fixture.store.ListHookTrustedPaths(t.Context(), "cursor")
	if err != nil {
		t.Fatalf("ListHookTrustedPaths error = %v", err)
	}
	if len(paths) != 2 {
		t.Fatalf("recorded paths = %#v, want the relocation recorded and the previous path kept", paths)
	}
	if !paths[0].IsCurrent || paths[0].Path != "/usr/local/bin/loaf" {
		t.Fatalf("current path = %#v, want the relocated executable", paths[0])
	}
	if codex, err := fixture.store.ListHookTrustedPaths(t.Context(), "codex"); err != nil || len(codex) != 0 {
		t.Fatalf("codex paths = %#v, %v, want records isolated per target", codex, err)
	}

	// An entry quoting the previous path is still recognized as Loaf's.
	records, err := loadHookRecords(t.Context(), fixture.store, "cursor")
	if err != nil {
		t.Fatalf("loadHookRecords error = %v", err)
	}
	ownership, err := relocated.recognition(records).ownsEntry(map[string]any{"command": "'/opt/homebrew/bin/loaf' task refresh"})
	if err != nil {
		t.Fatalf("ownsEntry error = %v", err)
	}
	if !ownership.owned || ownership.hookID != "generate-task-board" {
		t.Fatalf("ownsEntry = %#v, want the previously recorded path still recognized", ownership)
	}
}

// A distribution whose catalog and manifest disagree about which release they
// are is stale on one side or the other, and neither the desired entries nor
// the cohort bound can be trusted. It fails closed before anything is written.
func TestHookReconcileRefusesAStaleDistribution(t *testing.T) {
	t.Run("catalog and manifest disagree", func(t *testing.T) {
		fixture := newCursorHookFixture(t)
		if err := generateNativeCursorHookCatalog(filepath.Join(testRepositoryRoot(t), "config", "hooks.yaml"), fixture.dist, "0.2.20"); err != nil {
			t.Fatalf("generateNativeCursorHookCatalog error = %v", err)
		}

		_, err := newHookReconciler(fixture.options)
		if err == nil || !strings.Contains(err.Error(), "stale") {
			t.Fatalf("newHookReconciler error = %v, want a stale-distribution refusal", err)
		}
		if _, statErr := os.Stat(fixture.hooks); !os.IsNotExist(statErr) {
			t.Fatalf("hooks file stat = %v, want nothing written", statErr)
		}
	})

	// The other direction of the same disagreement. Which half was left behind
	// is not knowable from inside, so both spellings refuse rather than one of
	// them guessing that the newer half is authoritative.
	t.Run("manifest older than catalog", func(t *testing.T) {
		fixture := newCursorHookFixture(t)
		writeInstallFile(t, filepath.Join(fixture.dist, targetBuildManifestFile), testHookTargetManifestBody(t, "cursor", "0.2.20", fixture.dist))

		_, err := newHookReconciler(fixture.options)
		if err == nil || !strings.Contains(err.Error(), "stale") {
			t.Fatalf("newHookReconciler error = %v, want a stale-distribution refusal", err)
		}
		if !strings.Contains(err.Error(), "9.8.7-test.1") || !strings.Contains(err.Error(), "0.2.20") {
			t.Fatalf("newHookReconciler error = %v, want both versions named", err)
		}
		if _, statErr := os.Stat(fixture.hooks); !os.IsNotExist(statErr) {
			t.Fatalf("hooks file stat = %v, want nothing written", statErr)
		}
	})

	t.Run("no target manifest at all", func(t *testing.T) {
		fixture := newCursorHookFixture(t)
		if err := os.Remove(filepath.Join(fixture.dist, targetBuildManifestFile)); err != nil {
			t.Fatalf("Remove(manifest) error = %v", err)
		}

		_, err := newHookReconciler(fixture.options)
		if err == nil || !strings.Contains(err.Error(), "stale") {
			t.Fatalf("newHookReconciler error = %v, want a stale-distribution refusal", err)
		}
	})
}

// Decision 10's lock lifetime is over the whole sequence, so the lock is taken
// before any state is read. A run that never gets the lock never opens state.
func TestHookReconcileTakesTheLockBeforeReadingState(t *testing.T) {
	fixture := newCursorHookFixture(t)
	lock, err := acquireHookFileLock(fixture.hooks, hookFileLockWait)
	if err != nil {
		t.Fatalf("acquireHookFileLock error = %v", err)
	}
	defer lock.release()

	opened := 0
	reconciler := fixture.reconciler(t)
	reconciler.lockWait = 20 * time.Millisecond
	reconciler.state = func() (*state.Store, error) {
		opened++
		return fixture.store, nil
	}

	if _, err := reconciler.apply(t.Context()); err == nil || !strings.Contains(err.Error(), "another Loaf process") {
		t.Fatalf("apply error = %v, want a contention failure", err)
	}
	if opened != 0 {
		t.Fatalf("state opened %d times before the lock was held, want 0", opened)
	}
}

// The regression this Change exists to prevent, stated as an ordering test: the
// records commit before the caller replaces the installed manifest, so a run
// that dies in between still knows the hook was deliberately absent.
func TestHookReconcileRecordsAbsorptionBeforeTheManifestIsReplaced(t *testing.T) {
	fixture := newCodexHookFixture(t)
	live := string(testHookFixture(t, "codex-hooks-live.json"))
	fixture.writeHooks(t, live)
	fixture.writeInstalledManifest(t, testHookPriorVersion)

	reconciler := fixture.reconciler(t)
	if err := reconciler.begin(t.Context()); err != nil {
		t.Fatalf("begin error = %v", err)
	}
	// The adapter sync replaces the prior release's manifest with this one's,
	// and the run dies before the file half ever happens.
	fixture.writeInstalledManifest(t, "9.8.7-test.1")
	releaseHookReconcile(reconciler)

	actions := fixture.apply(t)

	if len(actions) != 0 {
		t.Fatalf("actions = %s, want the retry to find the absorption already recorded", describeHookActions(actions))
	}
	row, found, err := fixture.store.GetHookEnablement(t.Context(), "codex", "SessionStart", "session-start-loaf")
	if err != nil || !found || row.Enablement != state.HookEnablementDisabled {
		t.Fatalf("record = %#v, %v, %v, want the disable intact after the manifest moved on", row, found, err)
	}
	fixture.assertHooksUnchanged(t, live)
}

// State that cannot be read is not state that says "enabled". A reconcile whose
// authority is unavailable fails closed with the file untouched, because the
// alternative is silently restoring a hook the operator disabled.
func TestHookReconcileFailsClosedWhenEnablementStateIsUnavailable(t *testing.T) {
	fixture := newCursorHookFixture(t)
	unavailable := errors.New("state database is invalid: schema version 41 does not match expected version 42")

	for name, resolver := range map[string]hookStateResolver{
		"unreadable": func() (*state.Store, error) { return nil, unavailable },
		"absent":     func() (*state.Store, error) { return nil, nil },
		"unset":      nil,
	} {
		t.Run(name, func(t *testing.T) {
			reconciler := fixture.reconciler(t)
			reconciler.state = resolver

			if _, err := reconciler.apply(t.Context()); err == nil {
				t.Fatal("apply error = nil, want a fail-closed refusal")
			}
			if _, err := os.Stat(fixture.hooks); !os.IsNotExist(err) {
				t.Fatalf("hooks file stat = %v, want nothing written", err)
			}
		})
	}
}

// A plan is allowed to run on a host that has never recorded anything: with no
// database there is nothing to read, every hook reads enabled, and the plan
// still refuses to create one.
func TestHookReconcilePlanToleratesAnAbsentStateDatabase(t *testing.T) {
	fixture := newCursorHookFixture(t)
	reconciler := fixture.reconciler(t)
	reconciler.state = func() (*state.Store, error) { return nil, nil }

	actions, err := reconciler.plan(t.Context())
	if err != nil {
		t.Fatalf("plan error = %v", err)
	}
	if !testHookHasAnyAction(actions, hookActionAdd) {
		t.Fatalf("actions = %s, want the catalog planned as enabled", describeHookActions(actions))
	}
}

// "Nothing recorded yet" and "this is not Loaf's database" fail an ordinary
// schema read identically, and only the first of them means every hook is
// enabled. Reading a foreign database as empty would report the operator's
// disabled hooks as enabled and call that a plan, so the readable-but-foreign
// case fails closed with the underlying reason — the same answer apply gives.
func TestHookReconcilePlanDistinguishesEmptyStateFromUnreadableState(t *testing.T) {
	t.Run("uninitialized plans as empty", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "loaf.sqlite")
		writeFile(t, path, "")

		store, err := openHookStateStoreReadOnly(path)
		if err != nil {
			t.Fatalf("openHookStateStoreReadOnly error = %v, want an uninitialized database to read as empty", err)
		}
		if store != nil {
			store.Close()
			t.Fatal("openHookStateStoreReadOnly returned a store for an uninitialized database, want no records")
		}
	})

	t.Run("readable non-Loaf database fails closed", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "loaf.sqlite")
		testHookWriteForeignDatabase(t, path)

		store, err := openHookStateStoreReadOnly(path)
		if store != nil {
			store.Close()
		}
		if err == nil {
			t.Fatal("openHookStateStoreReadOnly error = nil, want a foreign database to fail the plan closed")
		}
		if !strings.Contains(err.Error(), "cannot be read") || !strings.Contains(err.Error(), path) {
			t.Fatalf("error = %v, want it to name the unreadable state and its path", err)
		}
	})

	// The plan's verdict has to be the one apply would reach, or a dry run
	// promises work the real run refuses.
	t.Run("apply refuses the same database", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "loaf.sqlite")
		testHookWriteForeignDatabase(t, path)

		store, err := state.OpenStore(path)
		if err != nil {
			t.Fatalf("OpenStore error = %v", err)
		}
		defer store.Close()
		bootstrapped, err := store.BootstrapIfEmpty(t.Context())
		if err != nil {
			t.Fatalf("BootstrapIfEmpty error = %v", err)
		}
		if bootstrapped {
			t.Fatal("BootstrapIfEmpty bootstrapped a database carrying somebody else's tables")
		}
		if err := store.RequireCurrentSchema(t.Context()); err == nil {
			t.Fatal("RequireCurrentSchema error = nil, want apply to refuse the foreign database too")
		}
	})
}

// testHookWriteForeignDatabase writes a perfectly readable SQLite file that is
// simply not Loaf's: real tables, no schema ledger.
func testHookWriteForeignDatabase(t *testing.T, path string) {
	t.Helper()
	db, err := sql.Open("sqlite3", "file:"+filepath.ToSlash(path))
	if err != nil {
		t.Fatalf("sql.Open error = %v", err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE somebody_elses_notes (id INTEGER PRIMARY KEY, body TEXT)`); err != nil {
		t.Fatalf("create foreign table error = %v", err)
	}
	if _, err := db.Exec(`INSERT INTO somebody_elses_notes (body) VALUES ('not loaf')`); err != nil {
		t.Fatalf("seed foreign table error = %v", err)
	}
}

// A plan reads state and leaves it byte-for-byte as it found it — no bootstrap,
// no journal-mode change, nothing. The uninitialized case matters most: that is
// the one a writable open would have silently converted. The live-WAL case is
// the one a byte check of the main file alone would miss: a writable open
// against a database another process is holding in WAL could checkpoint it,
// which moves data between the sidecar and the main file without either one
// looking obviously wrong on its own.
func TestHookReconcilePlanLeavesTheStateDatabaseUnchanged(t *testing.T) {
	for _, testCase := range []struct {
		name string
		seed func(t *testing.T, path string)
	}{
		{name: "uninitialized", seed: func(t *testing.T, path string) { writeFile(t, path, "") }},
		{name: "migrated", seed: func(t *testing.T, path string) {
			store, err := state.OpenStore(path)
			if err != nil {
				t.Fatalf("OpenStore error = %v", err)
			}
			if err := store.ApplyMigrations(context.Background()); err != nil {
				t.Fatalf("ApplyMigrations error = %v", err)
			}
			store.Close()
		}},
		// The discriminating case for journal mode. Loaf's writable DSN asks for
		// WAL, and SQLite records that choice in the file header permanently, so
		// a plan that opened this database writably would convert it and hand
		// the operator's database back in a mode they never chose.
		{name: "rollback journal mode", seed: func(t *testing.T, path string) {
			store, err := state.OpenStore(path)
			if err != nil {
				t.Fatalf("OpenStore error = %v", err)
			}
			if err := store.ApplyMigrations(context.Background()); err != nil {
				t.Fatalf("ApplyMigrations error = %v", err)
			}
			store.Close()
			testHookSetJournalMode(t, path, "delete")
			if mode := testHookJournalMode(t, path); mode != "delete" {
				t.Fatalf("seeded journal mode = %q, want delete", mode)
			}
		}},
		{name: "live wal sidecars", seed: func(t *testing.T, path string) {
			// The writer stays open for the whole test, which is what keeps the
			// -wal and -shm files present and populated rather than checkpointed
			// away by a clean close.
			store, err := state.OpenStore(path)
			if err != nil {
				t.Fatalf("OpenStore error = %v", err)
			}
			t.Cleanup(func() { store.Close() })
			if err := store.ApplyMigrations(context.Background()); err != nil {
				t.Fatalf("ApplyMigrations error = %v", err)
			}
			if _, err := store.RecordHookTrustedPath(context.Background(), "cursor", "/Users/canary/.local/bin/loaf"); err != nil {
				t.Fatalf("RecordHookTrustedPath error = %v", err)
			}
			for _, suffix := range []string{"-wal", "-shm"} {
				if _, err := os.Stat(path + suffix); err != nil {
					t.Fatalf("Stat(%s) error = %v, want a populated sidecar before the plan", path+suffix, err)
				}
			}
		}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			fixture := newCursorHookFixture(t)
			path := filepath.Join(t.TempDir(), "loaf.sqlite")
			testCase.seed(t, path)
			before := testHookStateFileSnapshot(t, path)

			reconciler := fixture.reconciler(t)
			reconciler.state = func() (*state.Store, error) { return openHookStateStoreReadOnly(path) }
			actions, err := reconciler.plan(t.Context())
			if err != nil {
				t.Fatalf("plan error = %v", err)
			}
			if !testHookHasAnyAction(actions, hookActionAdd) {
				t.Fatalf("actions = %s, want the catalog planned", describeHookActions(actions))
			}

			after := testHookStateFileSnapshot(t, path)
			for _, suffix := range []string{"", "-wal"} {
				if bytes.Equal(before[suffix], after[suffix]) {
					continue
				}
				// A journal-mode flip rewrites the header without changing the
				// file's length, so say which mode it landed in rather than
				// reporting two identical byte counts.
				detail := fmt.Sprintf("%d bytes before, %d after", len(before[suffix]), len(after[suffix]))
				if suffix == "" {
					detail += fmt.Sprintf("; journal mode is now %q", testHookJournalMode(t, path))
				}
				t.Fatalf("the plan changed %s: %s", path+suffix, detail)
			}
			// The shared-memory index is the one file every reader touches by
			// design — SQLite records a read mark in it to hold back
			// checkpointing while the read runs, so byte stability there is not
			// something any reader can promise. What must hold is that it is
			// still there afterwards: a plan that checkpointed the database out
			// of WAL mode, or opened it writably enough to reset it, would have
			// taken the sidecars with it.
			if _, seeded := before["-shm"]; seeded {
				if _, survived := after["-shm"]; !survived {
					t.Fatalf("the plan removed %s, so it did not leave the database in WAL mode", path+"-shm")
				}
			}
		})
	}
}

// testHookSetJournalMode rewrites the database's persistent journal mode using
// a connection of the test's own, so a case can start from a mode Loaf's own
// DSN would never leave it in.
func testHookSetJournalMode(t *testing.T, path string, mode string) {
	t.Helper()
	db, err := sql.Open("sqlite3", "file:"+filepath.ToSlash(path))
	if err != nil {
		t.Fatalf("sql.Open error = %v", err)
	}
	defer db.Close()
	var applied string
	if err := db.QueryRow(`PRAGMA journal_mode=` + mode).Scan(&applied); err != nil {
		t.Fatalf("set journal_mode=%s error = %v", mode, err)
	}
}

func testHookJournalMode(t *testing.T, path string) string {
	t.Helper()
	db, err := sql.Open("sqlite3", "file:"+filepath.ToSlash(path)+"?mode=ro")
	if err != nil {
		t.Fatalf("sql.Open error = %v", err)
	}
	defer db.Close()
	var mode string
	if err := db.QueryRow(`PRAGMA journal_mode`).Scan(&mode); err != nil {
		t.Fatalf("read journal_mode error = %v", err)
	}
	return mode
}

// testHookStateFileSnapshot captures the database and both WAL sidecars, since
// a checkpoint moves bytes between them and reading only the main file would
// call that no change at all. A missing sidecar is recorded as absent rather
// than as an error: the non-WAL cases have none.
func testHookStateFileSnapshot(t *testing.T, path string) map[string][]byte {
	t.Helper()
	snapshot := map[string][]byte{}
	for _, suffix := range []string{"", "-wal", "-shm"} {
		body, err := os.ReadFile(path + suffix)
		if err != nil {
			if !os.IsNotExist(err) {
				t.Fatalf("ReadFile(%s) error = %v", path+suffix, err)
			}
			continue
		}
		snapshot[suffix] = body
	}
	return snapshot
}

// Install's ordering, stated against the failure it exists for: the sync that
// replaces the prior release's manifest cannot run before the records are
// durable, so a sync failure leaves both the provenance and the disable intact.
func TestInstallTargetKeepsAbsorptionRecordsWhenTheAdapterSyncFails(t *testing.T) {
	fixture := newCodexHookFixture(t)
	live := string(testHookFixture(t, "codex-hooks-live.json"))
	fixture.writeHooks(t, live)
	fixture.writeInstalledManifest(t, testHookPriorVersion)
	manifestPath := filepath.Join(fixture.config, targetInstallManifestFile)
	manifestBefore, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("ReadFile(installed manifest) error = %v", err)
	}

	failing := fixture.options
	failing.TargetAdapterOps = &targetAdapterInstallOperations{beforePublish: func() error { return errHookCrashInjection }}
	if err := installTargetDistribution(failing); !errors.Is(err, errHookCrashInjection) {
		t.Fatalf("install error = %v, want the injected sync failure", err)
	}

	row, found, err := fixture.store.GetHookEnablement(t.Context(), "codex", "SessionStart", "session-start-loaf")
	if err != nil || !found || row.Enablement != state.HookEnablementDisabled {
		t.Fatalf("record = %#v, %v, %v, want the absorption committed before the sync ran", row, found, err)
	}
	assertInstallFile(t, manifestPath, string(manifestBefore))
	fixture.assertHooksUnchanged(t, live)

	// The retry completes, and the hook the operator deleted stays deleted.
	if err := installTargetDistribution(fixture.options); err != nil {
		t.Fatalf("retry install error = %v", err)
	}
	fixture.assertHooksUnchanged(t, live)
}

// The retired whole-file hooks row an older release left in the installed
// manifest is dropped by the next manifest write — and that write is sequenced
// after the absorption marker is durable, never used as the gate for it. The
// gap between the two is a real crash window, so it is injected here: what the
// crashed run leaves behind must be readable by the retry as exactly the same
// prior install, and the absorption must not happen twice.
func TestInstallTargetDropsTheObsoleteHookRowAfterTheMarkerIsDurable(t *testing.T) {
	fixture := newCodexHookFixture(t)
	live := string(testHookFixture(t, "codex-hooks-live.json"))
	fixture.writeHooks(t, live)
	fixture.writeInstalledManifest(t, testHookPriorVersion)
	manifestPath := filepath.Join(fixture.config, targetInstallManifestFile)
	if !strings.Contains(string(readFileBytes(t, manifestPath)), obsoleteHookProjectionKind) {
		t.Fatal("the prior release's manifest carries no obsolete hooks row; this test has no subject")
	}

	failing := fixture.options
	failing.TargetAdapterOps = &targetAdapterInstallOperations{beforePublish: func() error { return errHookCrashInjection }}
	if err := installTargetDistribution(failing); !errors.Is(err, errHookCrashInjection) {
		t.Fatalf("install error = %v, want the injected failure between the marker and the manifest write", err)
	}

	if _, marked, err := fixture.store.GetHookAbsorptionMarker(t.Context(), "codex"); err != nil || !marked {
		t.Fatalf("marker = %v, %v, want it durable before the manifest write ran", marked, err)
	}
	row, found, err := fixture.store.GetHookEnablement(t.Context(), "codex", "SessionStart", "session-start-loaf")
	if err != nil || !found || row.Enablement != state.HookEnablementDisabled || row.AbsorbedAt == nil {
		t.Fatalf("record = %#v, %v, %v, want the absorption committed before the crash", row, found, err)
	}
	absorbedAt := *row.AbsorbedAt
	if !strings.Contains(string(readFileBytes(t, manifestPath)), obsoleteHookProjectionKind) {
		t.Fatal("the crashed run dropped the obsolete row; only a completed manifest write may drop it")
	}

	if err := installTargetDistribution(fixture.options); err != nil {
		t.Fatalf("retry install error = %v", err)
	}

	if body := string(readFileBytes(t, manifestPath)); strings.Contains(body, obsoleteHookProjectionKind) {
		t.Fatalf("installed manifest = %s, want the obsolete row absent after the next write", body)
	}
	rows, err := fixture.store.ListHookEnablements(t.Context(), "codex")
	if err != nil || len(rows) != 1 {
		t.Fatalf("enablement rows = %#v, %v, want exactly the one record the first run absorbed", rows, err)
	}
	if rows[0].AbsorbedAt == nil || *rows[0].AbsorbedAt != absorbedAt {
		t.Fatalf("absorbed_at = %v, want the provenance from the first run (%q) rather than a second absorption", rows[0].AbsorbedAt, absorbedAt)
	}
	fixture.assertHooksUnchanged(t, live)
}

// The tolerance the reader promises, stated where it actually matters: an
// upgrade that meets a retired row it cannot make sense of still absorbs. Each
// defect here is one a later release could plausibly have left behind, and
// under the strict rules every one of them would abort the read — and with it
// the migration — before a single record was written.
func TestInstallTargetAbsorbsThroughADefectiveObsoleteHookRow(t *testing.T) {
	for name, injectDefect := range map[string]func(string) string{
		"unknown field": func(body string) string {
			return strings.Replace(body, `"kind": "hook-projection",`, `"kind": "hook-projection",
      "projection_generation": 7,`, 1)
		},
		"wrong-typed field": func(body string) string {
			return strings.Replace(body, `"kind": "hook-projection",`, `"kind": "hook-projection",
      "mode": "0644",`, 1)
		},
		"duplicate key inside the row": func(body string) string {
			return strings.Replace(body, `"kind": "hook-projection",`, `"kind": "hook-projection",
      "destination": "hooks.json",`, 1)
		},
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newCodexHookFixture(t)
			live := string(testHookFixture(t, "codex-hooks-live.json"))
			fixture.writeHooks(t, live)
			manifestPath := filepath.Join(fixture.config, targetInstallManifestFile)
			defective := injectDefect(testHookTargetManifestBody(t, "codex", testHookPriorVersion, ""))
			if !strings.Contains(defective, obsoleteHookProjectionKind) {
				t.Fatal("the defect injection lost the retired row; this test has no subject")
			}
			writeInstallFile(t, manifestPath, defective)

			if err := installTargetDistribution(fixture.options); err != nil {
				t.Fatalf("install error = %v, want the defective retired row read tolerantly", err)
			}

			// Absorbed: the row was still readable as evidence of a prior install,
			// so the hook the operator deleted is recorded rather than re-added.
			row, found, err := fixture.store.GetHookEnablement(t.Context(), "codex", "SessionStart", "session-start-loaf")
			if err != nil || !found || row.Enablement != state.HookEnablementDisabled || row.AbsorbedAt == nil {
				t.Fatalf("record = %#v, %v, %v, want the absorption the retired row is evidence for", row, found, err)
			}
			if _, marked, err := fixture.store.GetHookAbsorptionMarker(t.Context(), "codex"); err != nil || !marked {
				t.Fatalf("marker = %v, %v, want the migration window closed", marked, err)
			}
			// Dropped: whatever the row carried is gone from the next write.
			if body := string(readFileBytes(t, manifestPath)); strings.Contains(body, obsoleteHookProjectionKind) {
				t.Fatalf("installed manifest = %s, want the defective row absent after the next write", body)
			}
			fixture.assertHooksUnchanged(t, live)
		})
	}
}

// Tolerance stops where identification does. A manifest whose retired row
// cannot be told apart from a live one, or whose bytes carry something the
// stripper would have to repair on the way past, is refused outright — and
// refused before anything is recorded, because absorbing on the strength of a
// row Loaf could not identify is exactly the guess Decision 7 forbids.
func TestInstallTargetRefusesAManifestWhoseRetiredRowCannotBeIdentified(t *testing.T) {
	for name, corrupt := range map[string]func(string) string{
		"duplicate kind field": func(body string) string {
			return strings.Replace(body, `"kind": "hook-projection",`, `"kind": "hook-file",
      "kind": "hook-projection",`, 1)
		},
		"trailing delimiter": func(body string) string {
			return strings.TrimRight(body, "\n") + "}\n"
		},
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newCodexHookFixture(t)
			live := string(testHookFixture(t, "codex-hooks-live.json"))
			fixture.writeHooks(t, live)
			manifestPath := filepath.Join(fixture.config, targetInstallManifestFile)
			corrupted := corrupt(testHookTargetManifestBody(t, "codex", testHookPriorVersion, ""))
			writeInstallFile(t, manifestPath, corrupted)

			if err := installTargetDistribution(fixture.options); err == nil {
				t.Fatalf("install error = nil, want the unidentifiable manifest refused")
			}

			rows, err := fixture.store.ListHookEnablements(t.Context(), "codex")
			if err != nil || len(rows) != 0 {
				t.Fatalf("enablement rows = %#v, %v, want nothing absorbed from a manifest Loaf could not read", rows, err)
			}
			if _, marked, err := fixture.store.GetHookAbsorptionMarker(t.Context(), "codex"); err != nil || marked {
				t.Fatalf("marker = %v, %v, want the migration window still open", marked, err)
			}
			assertInstallFile(t, manifestPath, corrupted)
			fixture.assertHooksUnchanged(t, live)
		})
	}
}

// The other half of install's ordering, and the window the reconciler-level
// tests cannot reach: the records are durable, the adapter sync has already
// replaced the prior release's manifest, and the run dies before the file is
// projected. The prior version's identity is gone from disk at that point, so
// the retry has nothing but the records to go on — which is exactly why they
// are written first. It must converge without absorbing a second time.
func TestInstallTargetConvergesAfterACrashBetweenManifestSyncAndProjection(t *testing.T) {
	fixture := newCursorHookFixture(t)
	// One cohort hook the operator deleted (absorbs as disabled) and one entry
	// weakened by hand (needs a write), so the run has both a record to commit
	// and a projection to die in.
	fixture.writeHooks(t, testHookCursorFileWithout(t, "session-start-loaf"))
	fixture.writeInstalledManifest(t, testHookPriorVersion)
	fixture.mutateEntry(t, "preToolUse", "check-"+"sec"+"rets", func(entry map[string]any) {
		entry["command"] = "loaf check --hook check-" + "sec" + "rets --advisory"
	})
	hooksBefore := fixture.readHooks(t)
	manifestPath := filepath.Join(fixture.config, targetInstallManifestFile)

	failing := fixture.options
	failing.HookOps = &hookReconcileOperations{beforeRename: func() error { return errHookCrashInjection }}
	if err := installTargetDistribution(failing); !errors.Is(err, errHookCrashInjection) {
		t.Fatalf("install error = %v, want the injected projection failure", err)
	}

	// The distinguishing precondition: the sync ran, so the manifest on disk is
	// this release's and no longer names the version absorption was bounded by.
	installed, err := readTargetAdapterManifest(manifestPath)
	if err != nil {
		t.Fatalf("readTargetAdapterManifest error = %v", err)
	}
	if installed.PackageVersion != "9.8.7-test.1" {
		t.Fatalf("installed manifest package_version = %q, want the sync to have replaced it before the crash", installed.PackageVersion)
	}
	row, found, err := fixture.store.GetHookEnablement(t.Context(), "cursor", "sessionStart", "session-start-loaf")
	if err != nil || !found || row.Enablement != state.HookEnablementDisabled || row.AbsorbedAt == nil {
		t.Fatalf("record = %#v, %v, %v, want the absorption durable before the projection died", row, found, err)
	}
	absorbedAt := *row.AbsorbedAt
	fixture.assertHooksUnchanged(t, hooksBefore)

	if err := installTargetDistribution(fixture.options); err != nil {
		t.Fatalf("retry install error = %v", err)
	}

	// Convergence: the weakened entry is back to the shipped shape, the hook the
	// operator deleted stays deleted, and its provenance was not rewritten.
	// The probe names the whole weakened command — two hooks this version ships
	// are advisory by design, so the flag alone appears in a converged file.
	if weakened := "loaf check --hook check-" + "sec" + "rets --advisory"; strings.Contains(fixture.readHooks(t), weakened) {
		t.Fatalf("weakened enforcement survived the retry:\n%s", fixture.readHooks(t))
	}
	if ids := fixture.hookIDsByEvent(t)["sessionStart"]; len(ids) != 0 {
		t.Fatalf("sessionStart = %v, want the disabled hook to stay out of the file", ids)
	}
	rows, err := fixture.store.ListHookEnablements(t.Context(), "cursor")
	if err != nil || len(rows) != 1 {
		t.Fatalf("enablement rows = %#v, %v, want exactly the one absorbed record", rows, err)
	}
	if rows[0].AbsorbedAt == nil || *rows[0].AbsorbedAt != absorbedAt {
		t.Fatalf("absorbed_at = %v, want the immutable provenance from the first run (%q)", rows[0].AbsorbedAt, absorbedAt)
	}

	// And a third run has nothing left to do.
	if actions := fixture.apply(t); len(actions) != 0 {
		t.Fatalf("actions = %s, want the retried install to have converged", describeHookActions(actions))
	}
}

// The whole-file merge is unreachable rather than merely unused: a build output
// that predates the catalog is refused, because every property this Change adds
// is absent from the path behind it.
func TestInstallTargetRefusesAStaleDistribution(t *testing.T) {
	for _, target := range []string{"cursor", "codex"} {
		t.Run(target, func(t *testing.T) {
			fixture := newHookFixture(t, target)
			if err := os.Remove(filepath.Join(fixture.dist, targetBuildManifestFile)); err != nil {
				t.Fatalf("Remove(manifest) error = %v", err)
			}
			existing := `{"hooks":{}}`
			fixture.writeHooks(t, existing)

			err := installTargetDistribution(fixture.options)
			if err == nil || !strings.Contains(err.Error(), "stale") {
				t.Fatalf("install error = %v, want a stale-distribution refusal", err)
			}
			if !strings.Contains(err.Error(), "loaf build") {
				t.Fatalf("install error = %v, want the remedy named", err)
			}
			fixture.assertHooksUnchanged(t, existing)

			decisions, err := planTargetDistribution(fixture.options)
			if err != nil {
				t.Fatalf("planTargetDistribution error = %v", err)
			}
			if !testHookDecisionsContainDetail(decisions, planActionConflict, "stale") {
				t.Fatalf("plan decisions = %#v, want the same refusal promised", decisions)
			}
		})
	}
}

// Refusing a stale distribution is only half a promise if the refusal arrives
// after something has already been taken away. Both installers reach their
// verdict before they touch any surface, so an operator who runs a stale build
// output is left exactly where they started.
func TestInstallTargetRefusesAStaleDistributionBeforeMutatingAnySurface(t *testing.T) {
	for _, target := range []string{"cursor", "codex"} {
		t.Run(target, func(t *testing.T) {
			fixture := newHookFixture(t, target)
			if err := os.Remove(filepath.Join(fixture.dist, targetBuildManifestFile)); err != nil {
				t.Fatalf("Remove(manifest) error = %v", err)
			}
			// Every surface the installers write before they reach the hooks
			// file, seeded with content only this test could have put there.
			writeInstallFile(t, filepath.Join(fixture.dist, "skills", "foundations", "SKILL.md"), "# Distribution\n")
			writeInstallFile(t, filepath.Join(fixture.home, ".agents", "skills", "foundations", "SKILL.md"), "# Installed\n")
			writeInstallFile(t, filepath.Join(fixture.config, "commands", "keep.md"), "# Keep\n")
			writeInstallFile(t, filepath.Join(fixture.config, "agents", "keep.md"), "# Keep\n")

			if err := installTargetDistribution(fixture.options); err == nil || !strings.Contains(err.Error(), "stale") {
				t.Fatalf("install error = %v, want a stale-distribution refusal", err)
			}

			assertInstallFile(t, filepath.Join(fixture.home, ".agents", "skills", "foundations", "SKILL.md"), "# Installed\n")
			assertInstallFile(t, filepath.Join(fixture.config, "commands", "keep.md"), "# Keep\n")
			assertInstallFile(t, filepath.Join(fixture.config, "agents", "keep.md"), "# Keep\n")
			assertInstallPathMissing(t, filepath.Join(fixture.config, targetInstallManifestFile))
			assertInstallPathMissing(t, filepath.Join(fixture.config, loafInstallMarkerFile))
		})
	}
}

// The plan is display: it reports the same per-entry decisions apply would take
// and writes nothing at all.
func TestHookReconcilePlanReportsActionsWithoutWriting(t *testing.T) {
	fixture := newCodexHookFixture(t)
	live := string(testHookFixture(t, "codex-hooks-live.json"))
	fixture.writeHooks(t, live)
	fixture.writeInstalledManifest(t, testHookPriorVersion)

	actions, err := fixture.reconciler(t).plan(t.Context())
	if err != nil {
		t.Fatalf("plan error = %v", err)
	}
	if len(actions) != 1 || actions[0].action != hookActionAbsorb {
		t.Fatalf("plan actions = %s, want the absorption reported", describeHookActions(actions))
	}
	fixture.assertHooksUnchanged(t, live)
	if _, marked, err := fixture.store.GetHookAbsorptionMarker(t.Context(), "codex"); err != nil || marked {
		t.Fatalf("marker after plan = %v, %v, want a plan to record nothing", marked, err)
	}

	decisions := hookActionPlanDecisions(fixture.reconciler(t), actions)
	if len(decisions) != 1 || decisions[0].Action != hookActionAbsorb || decisions[0].Kind != "hook-entry" {
		t.Fatalf("plan decisions = %#v, want one hook-entry absorption", decisions)
	}
	if decisions[0].ID != "hook:SessionStart/session-start-loaf" {
		t.Fatalf("plan decision id = %q, want the identity named", decisions[0].ID)
	}
}

// No file-level verdict survives anywhere on the reconciled path: the plan
// never conflicts over a hooks destination, however much the file diverged.
func TestHookReconcilePlanNeverConflictsOverTheHooksFile(t *testing.T) {
	fixture := newCursorHookFixture(t)
	fixture.apply(t)
	fixture.mutateEntry(t, "sessionStart", "session-start-loaf", func(entry map[string]any) {
		entry["command"] = "loaf journal context"
		delete(entry, loafHookMarker)
	})

	decisions, err := planTargetAdapterArtifacts(fixture.options)
	if err != nil {
		t.Fatalf("planTargetAdapterArtifacts error = %v", err)
	}
	for _, decision := range decisions {
		if decision.Action == planActionConflict {
			t.Fatalf("plan decision = %#v, want no file-level conflict for a diverged hooks file", decision)
		}
		if decision.Kind == obsoleteHookProjectionKind {
			t.Fatalf("plan decision = %#v, want the hooks file planned per entry", decision)
		}
	}
	if !testHookDecisionsContain(decisions, hookActionUpdate, "hook:sessionStart/session-start-loaf") {
		t.Fatalf("plan decisions = %#v, want the diverged entry planned as an update", decisions)
	}
}

// The verbatim plan output for the canary's Codex file, which is what the
// operator reads on the first upgrade to this release.
func TestHookReconcilePlanOutputNamesTheAbsorption(t *testing.T) {
	fixture := newCodexHookFixture(t)
	fixture.writeHooks(t, string(testHookFixture(t, "codex-hooks-live.json")))
	fixture.writeInstalledManifest(t, testHookPriorVersion)

	actions, err := fixture.reconciler(t).plan(t.Context())
	if err != nil {
		t.Fatalf("plan error = %v", err)
	}
	var out strings.Builder
	writeHookActionLines(&out, actions)

	want := "    ○ absorb hook:SessionStart/session-start-loaf — absent before this upgrade; recorded as disabled\n"
	if got := stripANSI(out.String()); got != want {
		t.Fatalf("plan output = %q, want %q", got, want)
	}
}

// hookFixture is one target's install surface: a built distribution carrying
// the real catalog, a config directory, and the user-scoped state the records
// live in.
type hookFixture struct {
	root    string
	dist    string
	config  string
	home    string
	hooks   string
	store   *state.Store
	options targetInstallOptions
}

func newCursorHookFixture(t *testing.T) hookFixture {
	t.Helper()
	fixture := newHookFixture(t, "cursor")
	if err := generateNativeCursorHookCatalog(filepath.Join(testRepositoryRoot(t), "config", "hooks.yaml"), fixture.dist, "9.8.7-test.1"); err != nil {
		t.Fatalf("generateNativeCursorHookCatalog error = %v", err)
	}
	return fixture
}

func newCodexHookFixture(t *testing.T) hookFixture {
	t.Helper()
	fixture := newHookFixture(t, "codex")
	if err := generateNativeCodexHookCatalog(fixture.dist, "9.8.7-test.1"); err != nil {
		t.Fatalf("generateNativeCodexHookCatalog error = %v", err)
	}
	return fixture
}

func newHookFixture(t *testing.T, target string) hookFixture {
	t.Helper()
	root := realpath(t, t.TempDir())
	home := filepath.Join(root, "home")
	// The config directory sits under the fixture's home because that is where
	// a real install puts it, and the `$HOME/...` spellings the live files use
	// only resolve to the recorded destinations when it does.
	config := filepath.Join(home, ".cursor")
	if target != "cursor" {
		config = filepath.Join(root, "reported-config")
	}
	fixture := hookFixture{
		root:   root,
		dist:   filepath.Join(root, "dist", target),
		config: config,
		home:   home,
		store:  testHookStateStore(t),
	}
	if err := os.MkdirAll(fixture.dist, 0o755); err != nil {
		t.Fatalf("MkdirAll(dist) error = %v", err)
	}
	writeTestHookDistManifest(t, fixture.dist, target)
	fixture.options = targetInstallOptions{
		Target:      target,
		DistDir:     fixture.dist,
		ConfigDir:   fixture.config,
		Version:     "9.8.7-test.1",
		HomeDir:     fixture.home,
		ProjectRoot: root,
		HookState:   func() (*state.Store, error) { return fixture.store, nil },
	}
	if target == "codex" {
		fixture.options.CodexHome = filepath.Join(home, ".codex")
	}
	fixture.hooks = targetHookFilePath(fixture.options)
	return fixture
}

// reconciler builds a reconciler with an executable resolution that does not
// depend on what happens to be on PATH where the tests run.
func (f hookFixture) reconciler(t *testing.T) *hookReconciler {
	t.Helper()
	reconciler, err := newHookReconciler(f.options)
	if err != nil {
		t.Fatalf("newHookReconciler error = %v", err)
	}
	if reconciler == nil {
		t.Fatalf("newHookReconciler returned no reconciler for %s", f.options.Target)
	}
	reconciler.resolveExecutable = func() (string, error) { return "/Users/canary/.local/bin/loaf", nil }
	return reconciler
}

func (f hookFixture) apply(t *testing.T) []hookAction {
	t.Helper()
	actions, err := f.reconciler(t).apply(t.Context())
	if err != nil {
		t.Fatalf("apply error = %v", err)
	}
	return actions
}

func (f hookFixture) catalog(t *testing.T) hookCatalog {
	t.Helper()
	catalog, err := readHookCatalog(f.dist)
	if err != nil {
		t.Fatalf("readHookCatalog error = %v", err)
	}
	return catalog
}

func (f hookFixture) recognition(t *testing.T) hookRecognition {
	t.Helper()
	records, err := loadHookRecords(t.Context(), f.store, f.options.Target)
	if err != nil {
		t.Fatalf("loadHookRecords error = %v", err)
	}
	return f.reconciler(t).recognition(records)
}

func (f hookFixture) writeHooks(t *testing.T, body string) {
	t.Helper()
	writeInstallFile(t, f.hooks, body)
}

func (f hookFixture) writeHooksValue(t *testing.T, value map[string]any) {
	t.Helper()
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatalf("marshal hooks error = %v", err)
	}
	f.writeHooks(t, string(body))
}

func (f hookFixture) readHooks(t *testing.T) string {
	t.Helper()
	body, err := os.ReadFile(f.hooks)
	if err != nil {
		t.Fatalf("ReadFile(hooks) error = %v", err)
	}
	return string(body)
}

func (f hookFixture) decodeHooks(t *testing.T) map[string]any {
	t.Helper()
	var value map[string]any
	if err := json.Unmarshal([]byte(f.readHooks(t)), &value); err != nil {
		t.Fatalf("decode hooks error = %v", err)
	}
	return value
}

// writeInstalledManifest stands in for what a previous release left behind: a
// manifest naming the version it shipped, the retired whole-file hooks row that
// release wrote, and the recorded hook-file destinations recognition matches
// against.
func (f hookFixture) writeInstalledManifest(t *testing.T, version string) {
	t.Helper()
	writeTestHookTargetManifest(t, filepath.Join(f.config, targetInstallManifestFile), f.options.Target, version)
}

// writeTestHookTargetManifest writes a real 0.2.20 manifest retargeted to the
// caller's target and version. Using the captured one rather than a synthetic
// stub is what gives the fixtures the hook-file destinations the path-backed
// entries are recognized through.
func writeTestHookTargetManifest(t *testing.T, path string, target string, version string) {
	t.Helper()
	writeInstallFile(t, path, testHookTargetManifestBody(t, target, version, ""))
}

// writeTestHookDistManifest additionally materializes the hook-file sources the
// manifest names, so the distribution is one the ordinary artifact machinery
// can verify alongside the reconciler.
func writeTestHookDistManifest(t *testing.T, dist string, target string) {
	t.Helper()
	writeInstallFile(t, filepath.Join(dist, targetBuildManifestFile), testHookTargetManifestBody(t, target, "9.8.7-test.1", dist))
}

func testHookTargetManifestBody(t *testing.T, target string, version string, sourceRoot string) string {
	t.Helper()
	var manifest map[string]any
	if err := json.Unmarshal(testHookFixture(t, "cursor-target-manifest-0.2.20.json"), &manifest); err != nil {
		t.Fatalf("decode manifest fixture error = %v", err)
	}
	manifest["target"] = target
	manifest["package_version"] = version
	manifest["adapters"] = []string{target + "-session-start-v1"}
	var kept []any
	for _, raw := range manifest["artifacts"].([]any) {
		artifact := raw.(map[string]any)
		if artifact["kind"] != "hook-file" {
			kept = append(kept, artifact)
			continue
		}
		if target != "cursor" {
			continue
		}
		if sourceRoot != "" {
			source := filepath.Join(sourceRoot, filepath.FromSlash(artifact["source_path"].(string)))
			body := "# " + artifact["source_path"].(string) + "\n"
			writeInstallFile(t, source, body)
			info, err := os.Lstat(source)
			if err != nil {
				t.Fatalf("Lstat(%s) error = %v", source, err)
			}
			artifact["sha256"] = sha256Hex(body)
			artifact["mode"] = uint32(info.Mode().Perm())
		}
		kept = append(kept, artifact)
	}
	manifest["artifacts"] = kept
	encoded, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatalf("encode manifest error = %v", err)
	}
	return string(encoded) + "\n"
}

func (f hookFixture) assertHooksUnchanged(t *testing.T, want string) {
	t.Helper()
	if got := f.readHooks(t); got != want {
		t.Fatalf("hooks file changed:\nwant %s\ngot  %s", want, got)
	}
}

func (f hookFixture) assertForeignSurvives(t *testing.T, event string, command string) {
	t.Helper()
	file, err := readHookFile(f.hooks)
	if err != nil {
		t.Fatalf("readHookFile error = %v", err)
	}
	entries, err := file.eventEntries(event)
	if err != nil {
		t.Fatalf("eventEntries error = %v", err)
	}
	for _, entry := range entries {
		if testHookEntryCommand(entry) == command {
			return
		}
	}
	t.Fatalf("%s entries = %#v, want the foreign entry %q preserved", event, entries, command)
}

// mutateEntry rewrites the Loaf entry paired to hookID, standing in for a hand
// edit of the installed file.
func (f hookFixture) mutateEntry(t *testing.T, event string, hookID string, mutate func(map[string]any)) {
	t.Helper()
	f.rewriteEntry(t, event, hookID, mutate, false)
}

func (f hookFixture) deleteEntry(t *testing.T, event string, hookID string) {
	t.Helper()
	f.rewriteEntry(t, event, hookID, nil, true)
}

func (f hookFixture) rewriteEntry(t *testing.T, event string, hookID string, mutate func(map[string]any), remove bool) {
	t.Helper()
	value := f.decodeHooks(t)
	events := value["hooks"].(map[string]any)
	entries := events[event].([]any)
	catalog := f.catalog(t)
	var kept []any
	found := false
	for _, raw := range entries {
		entry := raw.(map[string]any)
		if !found && testHookEntryIsHook(t, catalog, event, hookID, entry) {
			found = true
			if remove {
				continue
			}
			mutate(entry)
		}
		kept = append(kept, entry)
	}
	if !found {
		t.Fatalf("%s has no entry for %s", event, hookID)
	}
	events[event] = kept
	f.writeHooksValue(t, value)
}

func testHookEntryIsHook(t *testing.T, catalog hookCatalog, event string, hookID string, entry map[string]any) bool {
	t.Helper()
	for _, candidate := range catalog.Entries {
		if candidate.Event != event || candidate.HookID != hookID {
			continue
		}
		value, err := canonicalHookValue(entry)
		if err != nil {
			t.Fatalf("canonicalize entry error = %v", err)
		}
		template, err := decodeHookJSONValue(candidate.Template)
		if err != nil {
			t.Fatalf("decode template error = %v", err)
		}
		return reflect.DeepEqual(value, template)
	}
	return false
}

// testHookCursorFileWithout renders what a 0.2.20 install looks like after the
// operator deleted some of its entries by hand.
func testHookCursorFileWithout(t *testing.T, hookIDs ...string) string {
	t.Helper()
	deleted := map[string]bool{}
	for _, hookID := range hookIDs {
		deleted[hookID] = true
	}
	body := testHookFixture(t, "cursor-hooks-live.json")
	var value map[string]any
	if err := json.Unmarshal(body, &value); err != nil {
		t.Fatalf("decode live Cursor fixture error = %v", err)
	}
	catalog := testRepoHookCatalog(t, "cursor")
	events := value["hooks"].(map[string]any)
	for event, raw := range events {
		var kept []any
		for _, candidate := range raw.([]any) {
			entry := candidate.(map[string]any)
			drop := false
			for hookID := range deleted {
				if testHookEntryIsHook(t, catalog, event, hookID, entry) {
					drop = true
					break
				}
			}
			if !drop {
				kept = append(kept, entry)
			}
		}
		events[event] = kept
	}
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatalf("encode Cursor fixture error = %v", err)
	}
	return string(encoded) + "\n"
}

// testHookCatalogWithCohort narrows the frozen cohort so a test can say "this
// hook is the one the prior version shipped, and that one came later".
func testHookCatalogWithCohort(t *testing.T, catalog hookCatalog, introducedAfter ...string) hookCatalog {
	t.Helper()
	excluded := map[string]bool{}
	for _, hookID := range introducedAfter {
		excluded[hookID] = true
	}
	for index, cohort := range catalog.Cohorts {
		var kept []string
		for _, hookID := range cohort.HookIDs {
			if !excluded[hookID] {
				kept = append(kept, hookID)
			}
		}
		catalog.Cohorts[index].HookIDs = kept
	}
	return catalog
}

func (f hookFixture) hookIDsByEvent(t *testing.T) map[string][]string {
	t.Helper()
	file, err := readHookFile(f.hooks)
	if err != nil {
		t.Fatalf("readHookFile error = %v", err)
	}
	recognition := f.recognition(t)
	byEvent := map[string][]string{}
	for _, event := range file.events {
		entries, err := file.eventEntries(event)
		if err != nil {
			t.Fatalf("eventEntries error = %v", err)
		}
		outcome, err := pairHookEventEntries(recognition, event, entries)
		if err != nil {
			t.Fatalf("pairHookEventEntries error = %v", err)
		}
		for _, pairing := range outcome.paired {
			byEvent[event] = append(byEvent[event], pairing.hookID)
		}
	}
	return byEvent
}

// installTestHookCatalog gives a synthetic distribution the identity authority
// a real build emits beside its hooks.json, so an install fixture exercises the
// reconciler rather than tripping over a missing catalog.
func installTestHookCatalog(t *testing.T, dist string, target string, sources []hookCatalogSource) {
	t.Helper()
	catalog, err := newHookCatalog(target, "9.8.7-test.1", sources)
	if err != nil {
		t.Fatalf("newHookCatalog error = %v", err)
	}
	if err := writeHookCatalog(dist, catalog); err != nil {
		t.Fatalf("writeHookCatalog error = %v", err)
	}
}

// installTestHookDistribution gives a synthetic `dist/<target>` the two files
// every real build emits for a target whose hook entries are reconciled: the
// adapter manifest the installer reads, and the catalog identity resolves
// through. A distribution carrying neither is stale and the installer refuses
// it — that refusal has its own tests, and a fixture about skills, project
// files, or MCP prompts should not have to restate it.
// Passing no sources takes the target's default identity.
func installTestHookDistribution(t *testing.T, root string, target string, sources ...hookCatalogSource) {
	t.Helper()
	dist := filepath.Join(root, "dist", target)
	if len(sources) == 0 {
		sources = testHookCatalogSources(target)
	}
	writeTestTargetAdapterManifest(t, dist, target, nil)
	installTestHookCatalog(t, dist, target, sources)
}

// testHookCatalogSources is one identity per target — enough to be a valid
// catalog, small enough that a fixture with no interest in hooks is not paying
// for the whole shipped generation.
func testHookCatalogSources(target string) []hookCatalogSource {
	if target == "codex" {
		return []hookCatalogSource{testCodexHookCatalogSourceOnPath()}
	}
	return []hookCatalogSource{testCursorHookCatalogSource()}
}

// testCodexHookCatalogSourceOnPath spells the Codex identity with the bare
// `loaf` first token — one of the three executable forms recognition accepts.
// The shipped Codex entry carries the install-time placeholder instead, and
// rendering that requires a trusted absolute executable no end-to-end fixture
// can produce: the trust rules refuse every candidate under a temporary root,
// which is where a test's PATH necessarily lives. Tests whose subject is the
// rendered entry use the hook fixtures, which inject the resolution directly.
func testCodexHookCatalogSourceOnPath() hookCatalogSource {
	command := "loaf" + codexJournalHookCommandSuffix
	return hookCatalogSource{
		event:    "SessionStart",
		hookID:   "session-start-loaf",
		typeName: "command",
		command:  command,
		template: map[string]any{
			"matcher": codexJournalHookMatcher,
			"hooks":   []any{map[string]any{"type": "command", "command": command, "commandWindows": command}},
		},
	}
}

// testCursorHookCatalogSource is a Cursor entry in the shape the build emits:
// a bare `loaf` invocation, a matcher, and the human-legible managed marker
// that recognition deliberately does not depend on.
func testCursorHookCatalogSource() hookCatalogSource {
	command := "loaf check --hook validate-commit"
	return hookCatalogSource{
		event:    "beforeShellExecution",
		hookID:   "validate-commit",
		typeName: "command",
		command:  command,
		template: map[string]any{"command": command, "matcher": "Bash", "loaf-managed": true},
	}
}

// testCodexHookCatalogSource is the one identity Codex ships, as the build
// emits it: a matcher group carrying the install-time executable placeholder on
// both the POSIX and Windows commands.
func testCodexHookCatalogSource() hookCatalogSource {
	command := codexJournalExecutablePlaceholder + codexJournalHookCommandSuffix
	return hookCatalogSource{
		event:    "SessionStart",
		hookID:   "session-start-loaf",
		typeName: "command",
		command:  command,
		template: map[string]any{
			"matcher": codexJournalHookMatcher,
			"hooks":   []any{map[string]any{"type": "command", "command": command, "commandWindows": command}},
		},
	}
}

func installTestHookState(t *testing.T) hookStateResolver {
	t.Helper()
	store := testHookStateStore(t)
	return func() (*state.Store, error) { return store, nil }
}

// installTestHookStateAfterMigration is the state of a host that has already
// been through absorption once, which is what a test about ordinary install
// behaviour wants: the migration is a separate subject with its own tests.
func installTestHookStateAfterMigration(t *testing.T, target string) hookStateResolver {
	t.Helper()
	store := testHookStateStore(t)
	if _, err := store.AbsorbAndMarkHooks(context.Background(), target, "9.8.7-test.1", nil); err != nil {
		t.Fatalf("AbsorbAndMarkHooks error = %v", err)
	}
	return func() (*state.Store, error) { return store, nil }
}

func testHookStateStore(t *testing.T) *state.Store {
	t.Helper()
	store, err := state.OpenStore(filepath.Join(t.TempDir(), "loaf.sqlite"))
	if err != nil {
		t.Fatalf("OpenStore error = %v", err)
	}
	if err := store.ApplyMigrations(context.Background()); err != nil {
		t.Fatalf("ApplyMigrations error = %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

// testHookDocumentSnapshot captures everything reconciliation promises to
// leave alone: the top-level fields Loaf does not own with their exact JSON
// values, the event sections in the order the file declares them, and every
// foreign entry's exact value in its position among the other foreign entries.
// Comparing two snapshots is the whole of Decision 9's guarantee, which a map
// of decoded entries could not express — it loses field order, section order,
// and the raw values themselves.
func testHookDocumentSnapshot(t *testing.T, body []byte, recognition hookRecognition) []string {
	t.Helper()
	order, fields, err := decodeHookJSONObject(body)
	if err != nil {
		t.Fatalf("decode hooks document error = %v", err)
	}
	var snapshot []string
	for _, key := range order {
		if key == hookFileEventsField {
			continue
		}
		canonical, err := canonicalHookEntry(fields[key])
		if err != nil {
			t.Fatalf("canonicalize field %q error = %v", key, err)
		}
		snapshot = append(snapshot, "field "+key+" = "+canonical)
	}
	events, sections, err := decodeHookJSONObject(fields[hookFileEventsField])
	if err != nil {
		t.Fatalf("decode hooks section error = %v", err)
	}
	for _, event := range events {
		snapshot = append(snapshot, "event "+event)
		var entries []json.RawMessage
		if err := json.Unmarshal(sections[event], &entries); err != nil {
			t.Fatalf("decode %s entries error = %v", event, err)
		}
		position := 0
		for _, raw := range entries {
			entry, err := decodeHookEntry(raw)
			if err != nil {
				t.Fatalf("decode %s entry error = %v", event, err)
			}
			ownership, err := recognition.ownsEntry(entry)
			if err != nil {
				t.Fatalf("ownsEntry error = %v", err)
			}
			if ownership.owned {
				continue
			}
			canonical, err := canonicalHookEntry(raw)
			if err != nil {
				t.Fatalf("canonicalize %s entry error = %v", event, err)
			}
			snapshot = append(snapshot, fmt.Sprintf("foreign %s#%d %s", event, position, canonical))
			position++
		}
	}
	return snapshot
}

func testHookCountForeign(snapshot []string) int {
	total := 0
	for _, line := range snapshot {
		if strings.HasPrefix(line, "foreign ") {
			total++
		}
	}
	return total
}

func testHookCountEntries(entries map[string][]string) int {
	total := 0
	for _, event := range entries {
		total += len(event)
	}
	return total
}

func testHookEntryCommand(entry map[string]any) string {
	if command, ok := entry["command"].(string); ok {
		return command
	}
	handlers, ok := entry["hooks"].([]any)
	if !ok || len(handlers) == 0 {
		return ""
	}
	handler, ok := handlers[0].(map[string]any)
	if !ok {
		return ""
	}
	command, _ := handler["command"].(string)
	return command
}

func testHookHasAction(actions []hookAction, want string, hookID string) bool {
	for _, action := range actions {
		if action.action == want && action.hookID == hookID {
			return true
		}
	}
	return false
}

func testHookHasAnyAction(actions []hookAction, want string) bool {
	for _, action := range actions {
		if action.action == want {
			return true
		}
	}
	return false
}

func testHookDecisionsContainDetail(decisions []artifactPlanDecision, action string, detail string) bool {
	for _, decision := range decisions {
		if decision.Action == action && strings.Contains(decision.Detail, detail) {
			return true
		}
	}
	return false
}

func testHookDecisionsContain(decisions []artifactPlanDecision, action string, id string) bool {
	for _, decision := range decisions {
		if decision.Action == action && decision.ID == id {
			return true
		}
	}
	return false
}
