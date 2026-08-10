package cli

import (
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestHookPairingMatchesTheCurrentDesiredTemplate(t *testing.T) {
	recognition := testHookRecognition(t, "cursor", testRepoHookCatalog(t, "cursor"))
	entry := map[string]any{
		loafHookMarker: true,
		"timeout":      30,
		"matcher":      "Edit|Write|Bash",
		"failClosed":   true,
		"command":      "loaf check --hook check-secrets",
	}

	outcome, err := pairHookEventEntries(recognition, "preToolUse", []map[string]any{entry})
	if err != nil {
		t.Fatalf("pairHookEventEntries error = %v", err)
	}
	if len(outcome.paired) != 1 || outcome.paired[0].hookID != "check-secrets" || outcome.paired[0].pass != hookPairingTemplate {
		t.Fatalf("pairing = %#v, want check-secrets through the template pass", outcome)
	}
}

// The pre-dispatch-flag sessionStart command an earlier release shipped still
// pairs to its hook ID instead of reading as a retired generation.
func TestHookPairingMatchesAHistoricalSignature(t *testing.T) {
	recognition := testHookRecognition(t, "cursor", testRepoHookCatalog(t, "cursor"))
	entry := map[string]any{loafHookMarker: true, "timeout": 60, "command": "loaf journal context"}

	outcome, err := pairHookEventEntries(recognition, "sessionStart", []map[string]any{entry})
	if err != nil {
		t.Fatalf("pairHookEventEntries error = %v", err)
	}
	if len(outcome.paired) != 1 || outcome.paired[0].hookID != "session-start-loaf" || outcome.paired[0].pass != hookPairingSignature {
		t.Fatalf("pairing = %#v, want session-start-loaf through the signature pass", outcome)
	}
}

// Decision 3's security property: enforcement weakened by hand pairs to its
// hook ID so the next reconcile converges it, rather than orphaning as foreign
// and surviving the upgrade.
func TestHookPairingConvergesWeakenedEnforcement(t *testing.T) {
	recognition := testHookRecognition(t, "cursor", testRepoHookCatalog(t, "cursor"))
	entry := map[string]any{
		loafHookMarker: true,
		"timeout":      30,
		"matcher":      "Edit|Write|Bash",
		"command":      "loaf check --hook check-secrets --advisory",
	}

	outcome, err := pairHookEventEntries(recognition, "preToolUse", []map[string]any{entry})
	if err != nil {
		t.Fatalf("pairHookEventEntries error = %v", err)
	}
	if len(outcome.paired) != 1 || outcome.paired[0].hookID != "check-secrets" || outcome.paired[0].pass != hookPairingStem {
		t.Fatalf("pairing = %#v, want check-secrets through the stem pass", outcome)
	}
	if len(outcome.foreign) != 0 {
		t.Fatalf("pairing left %d foreign entries, want the weakened entry claimed", len(outcome.foreign))
	}
}

// The boundary the stem rule exists to hold: a longer hook id is a different
// hook, and substring resemblance never claims it.
func TestHookPairingLeavesTheDisabledLookalikeForeign(t *testing.T) {
	recognition := testHookRecognition(t, "cursor", testRepoHookCatalog(t, "cursor"))
	entry := map[string]any{"matcher": "Bash", "command": "loaf check --hook check-secrets-disabled"}

	outcome, err := pairHookEventEntries(recognition, "preToolUse", []map[string]any{entry})
	if err != nil {
		t.Fatalf("pairHookEventEntries error = %v", err)
	}
	if len(outcome.paired) != 0 || len(outcome.foreign) != 1 {
		t.Fatalf("pairing = %#v, want the lookalike left foreign", outcome)
	}
}

// Duplicates are Loaf's by construction, so the first survives to be converged
// and the extras are removable — no refusal, no fork.
func TestHookPairingConvergesDuplicateOwnedEntries(t *testing.T) {
	recognition := testHookRecognition(t, "cursor", testRepoHookCatalog(t, "cursor"))
	entries := []map[string]any{
		{loafHookMarker: true, "timeout": 30, "matcher": "Edit|Write|Bash", "failClosed": true, "command": "loaf check --hook check-secrets"},
		{"matcher": "Bash", "command": "loaf check --hook check-secrets --advisory"},
		{"matcher": "Edit|Write", "command": "loaf task refresh"},
	}

	outcome, err := pairHookEventEntries(recognition, "preToolUse", entries)
	if err != nil {
		t.Fatalf("pairHookEventEntries error = %v", err)
	}
	if len(outcome.paired) != 1 || outcome.paired[0].index != 0 || outcome.paired[0].hookID != "check-secrets" {
		t.Fatalf("paired = %#v, want the first entry surviving", outcome.paired)
	}
	if len(outcome.duplicates) != 1 || outcome.duplicates[0].index != 1 || outcome.duplicates[0].hookID != "check-secrets" {
		t.Fatalf("duplicates = %#v, want the second entry removable", outcome.duplicates)
	}
	// `loaf task refresh` belongs to postToolUse: under preToolUse it pairs to
	// no identity this section ships and reads as a retired generation.
	if !reflect.DeepEqual(outcome.retired, []int{2}) {
		t.Fatalf("retired = %#v, want the misplaced entry removable", outcome.retired)
	}
}

func TestHookPairingRemovesARetiredGeneration(t *testing.T) {
	recognition := testHookRecognition(t, "cursor", testRepoHookCatalog(t, "cursor"))
	entries := []map[string]any{
		{loafHookMarker: true, "matcher": "Bash", "if": "Bash(git commit:*)", "command": "loaf journal log --from-hook"},
		{"command": "loaf session start"},
		{"command": "bash $HOME/.cursor/hooks/session/session-start.sh"},
	}

	outcome, err := pairHookEventEntries(recognition, "preToolUse", entries)
	if err != nil {
		t.Fatalf("pairHookEventEntries error = %v", err)
	}
	if !reflect.DeepEqual(outcome.retired, []int{0, 1, 2}) {
		t.Fatalf("retired = %#v, want every legacy-allowlist entry removable", outcome.retired)
	}
	if len(outcome.foreign) != 0 || len(outcome.paired) != 0 {
		t.Fatalf("pairing = %#v, want the retired generation owned but unpaired", outcome)
	}
}

// Prompt-type hooks carry no command to tokenize, so they pair on the exact
// prompt the catalog declares.
func TestHookPairingMatchesPromptEntriesByExactPrompt(t *testing.T) {
	catalog, err := newHookCatalog("cursor", "test", []hookCatalogSource{{
		event:    "preToolUse",
		hookID:   "journal-nudge",
		typeName: "prompt",
		prompt:   "SESSION JOURNAL NUDGE: log decisions before committing.",
		template: map[string]any{"prompt": "SESSION JOURNAL NUDGE: log decisions before committing.", "matcher": "Bash"},
	}})
	if err != nil {
		t.Fatalf("newHookCatalog error = %v", err)
	}
	recognition := testHookRecognition(t, "cursor", catalog)
	entries := []map[string]any{
		{"prompt": "SESSION JOURNAL NUDGE: log decisions before committing.", "matcher": "Bash", "timeout": 5},
		{"prompt": "A third-party nudge nobody else owns."},
	}

	outcome, err := pairHookEventEntries(recognition, "preToolUse", entries)
	if err != nil {
		t.Fatalf("pairHookEventEntries error = %v", err)
	}
	if len(outcome.paired) != 1 || outcome.paired[0].hookID != "journal-nudge" {
		t.Fatalf("paired = %#v, want the prompt entry paired", outcome.paired)
	}
	if !reflect.DeepEqual(outcome.foreign, []int{1}) {
		t.Fatalf("foreign = %#v, want the third-party prompt untouched", outcome.foreign)
	}
}

func TestHookPairingReportsCommandsCarryingTwoIdentities(t *testing.T) {
	recognition := testHookRecognition(t, "cursor", testRepoHookCatalog(t, "cursor"))
	entries := []map[string]any{{"matcher": "Bash", "command": "loaf check --hook check-secrets --hook validate-commit"}}

	_, err := pairHookEventEntries(recognition, "preToolUse", entries)
	if err == nil || !strings.Contains(err.Error(), "more than one Loaf hook") {
		t.Fatalf("pairHookEventEntries error = %v, want an integrity error", err)
	}
}

// The whole live file: the 17 shipped entries pair to their identities through
// the template pass, and all 33 foreign entries stay outside pairing entirely.
func TestHookPairingOverTheLiveCursorFile(t *testing.T) {
	recognition := testHookRecognition(t, "cursor", testRepoHookCatalog(t, "cursor"))
	events := testHookEventEntries(t, testHookFixture(t, "cursor-hooks-live.json"))

	paired := map[string][]string{}
	foreign := 0
	for event, entries := range events {
		outcome, err := pairHookEventEntries(recognition, event, entries)
		if err != nil {
			t.Fatalf("pairHookEventEntries(%s) error = %v", event, err)
		}
		if len(outcome.retired) != 0 || len(outcome.duplicates) != 0 {
			t.Fatalf("%s: retired = %#v, duplicates = %#v, want none", event, outcome.retired, outcome.duplicates)
		}
		foreign += len(outcome.foreign)
		for _, pairing := range outcome.paired {
			if pairing.pass != hookPairingTemplate {
				t.Fatalf("%s/%s paired through %q, want the in-sync template pass", event, pairing.hookID, pairing.pass)
			}
			paired[event] = append(paired[event], pairing.hookID)
		}
	}
	if foreign != 33 {
		t.Fatalf("pairing saw %d foreign entries, want 33", foreign)
	}
	want := map[string][]string{
		"preToolUse": {
			"artifact-body-write",
			"artifact-names",
			"check-secrets",
			"detect-linear-magic",
			"ephemeral-provenance",
			"github-account",
			"render-drift",
			"security-audit",
			"validate-commit",
			"validate-push",
			"workflow-pre-merge",
			"workflow-pre-pr",
			"workflow-pre-push",
		},
		"postToolUse":  {"generate-task-board", "kb-staleness-nudge", "workflow-post-merge"},
		"sessionStart": {"session-start-loaf"},
	}
	for event := range paired {
		sort.Strings(paired[event])
	}
	if !reflect.DeepEqual(paired, want) {
		t.Fatalf("paired = %#v, want %#v", paired, want)
	}
}

// The canary's Codex file: the operator deleted the Loaf entry, so nothing
// pairs, and the third-party group is never touched.
func TestHookPairingOverTheLiveCodexFile(t *testing.T) {
	recognition := testHookRecognition(t, "codex", testRepoHookCatalog(t, "codex"))
	events := testHookEventEntries(t, testHookFixture(t, "codex-hooks-live.json"))

	outcome, err := pairHookEventEntries(recognition, "SessionStart", events["SessionStart"])
	if err != nil {
		t.Fatalf("pairHookEventEntries error = %v", err)
	}
	if len(outcome.paired) != 0 || len(outcome.retired) != 0 || !reflect.DeepEqual(outcome.foreign, []int{0}) {
		t.Fatalf("pairing = %#v, want the herdr group foreign and nothing paired", outcome)
	}
}
