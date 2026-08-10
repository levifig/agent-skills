package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestHookCatalogIsEmittedForCursorAndCodexBuilds(t *testing.T) {
	root := setupBuildCommandLoafRoot(t)
	seedNativeCodexBuildFixture(t, root)
	seedNativeCursorBuildFixture(t, root)
	var stdout bytes.Buffer

	for _, target := range []string{"cursor", "codex"} {
		if err := (Runner{Stdout: &stdout, WorkingDir: root}).Run([]string{"build", "--target", target}); err != nil {
			t.Fatalf("build --target %s error = %v\n%s", target, err, stdout.String())
		}
	}

	cursor, err := readHookCatalog(filepath.Join(root, "dist", "cursor"))
	if err != nil {
		t.Fatalf("readHookCatalog(cursor) error = %v", err)
	}
	if cursor.Target != "cursor" || cursor.PackageVersion != "9.8.7-test.1" || cursor.Version != hookCatalogVersion {
		t.Fatalf("cursor catalog metadata = %#v", cursor)
	}
	events := map[string]string{}
	for _, entry := range cursor.Entries {
		events[entry.HookID] = entry.Event
	}
	for hookID, want := range map[string]string{
		"check-secrets":       "preToolUse",
		"workflow-pre-merge":  "preToolUse",
		"generate-task-board": "postToolUse",
		"kb-staleness-nudge":  "postToolUse",
		"session-start-loaf":  "sessionStart",
	} {
		if events[hookID] != want {
			t.Fatalf("cursor catalog event for %s = %q, want %q", hookID, events[hookID], want)
		}
	}

	codex, err := readHookCatalog(filepath.Join(root, "dist", "codex"))
	if err != nil {
		t.Fatalf("readHookCatalog(codex) error = %v", err)
	}
	if len(codex.Entries) != 1 || codex.Entries[0].Event != "SessionStart" || codex.Entries[0].HookID != "session-start-loaf" {
		t.Fatalf("codex catalog entries = %#v, want the single SessionStart identity", codex.Entries)
	}
	if !strings.Contains(string(codex.Entries[0].Template), codexJournalExecutablePlaceholder) {
		t.Fatalf("codex catalog template = %s, want the install-time executable placeholder", codex.Entries[0].Template)
	}
}

// The catalog template is the desired entry: whatever the build wrote into
// hooks.json must be byte-for-byte what reconciliation converges toward.
func TestHookCatalogTemplatesMatchTheBuiltHooksFile(t *testing.T) {
	root := setupBuildCommandLoafRoot(t)
	seedNativeCodexBuildFixture(t, root)
	seedNativeCursorBuildFixture(t, root)
	var stdout bytes.Buffer

	if err := (Runner{Stdout: &stdout, WorkingDir: root}).Run([]string{"build", "--target", "cursor"}); err != nil {
		t.Fatalf("build --target cursor error = %v\n%s", err, stdout.String())
	}

	catalog, err := readHookCatalog(filepath.Join(root, "dist", "cursor"))
	if err != nil {
		t.Fatalf("readHookCatalog error = %v", err)
	}
	body, err := os.ReadFile(filepath.Join(root, "dist", "cursor", "hooks.json"))
	if err != nil {
		t.Fatalf("ReadFile(hooks.json) error = %v", err)
	}
	built := testHookEventEntries(t, body)
	total := 0
	for _, entries := range built {
		total += len(entries)
	}
	if total != len(catalog.Entries) {
		t.Fatalf("hooks.json has %d entries, catalog has %d", total, len(catalog.Entries))
	}
	for _, entry := range catalog.Entries {
		template, err := decodeHookJSONValue(entry.Template)
		if err != nil {
			t.Fatalf("decode template for %s error = %v", entry.HookID, err)
		}
		found := false
		for _, candidate := range built[entry.Event] {
			value, err := canonicalHookValue(candidate)
			if err != nil {
				t.Fatalf("canonicalize built entry error = %v", err)
			}
			if reflect.DeepEqual(template, value) {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("catalog template for %s/%s has no matching entry in hooks.json", entry.Event, entry.HookID)
		}
	}
}

// The cohort is a frozen enumeration, not a re-derivation of today's config.
// It is pinned against captured 0.2.20 build output: every Loaf entry that
// release shipped pairs to a cohort ID, and the cohort names nothing else.
func TestHookCatalogCohortPinsWhatZeroTwoTwentyShipped(t *testing.T) {
	for _, testCase := range []struct {
		target  string
		fixture string
		want    int
	}{
		{target: "cursor", fixture: "cursor-hooks-0.2.20.json", want: 17},
		{target: "codex", fixture: "codex-hooks-0.2.20.json", want: 1},
	} {
		t.Run(testCase.target, func(t *testing.T) {
			catalog := testRepoHookCatalog(t, testCase.target)
			cohort, ok := catalog.cohortHookIDs("0.2.20")
			if !ok {
				t.Fatalf("%s catalog has no 0.2.20 cohort", testCase.target)
			}
			if len(cohort) != testCase.want {
				t.Fatalf("%s 0.2.20 cohort has %d hook ids, want %d", testCase.target, len(cohort), testCase.want)
			}
			recognition := testHookRecognition(t, testCase.target, catalog)
			shipped := []string{}
			for event, entries := range testHookEventEntries(t, testHookFixture(t, testCase.fixture)) {
				outcome, err := pairHookEventEntries(recognition, event, entries)
				if err != nil {
					t.Fatalf("pairHookEventEntries(%s) error = %v", event, err)
				}
				if len(outcome.foreign) != 0 || len(outcome.retired) != 0 || len(outcome.duplicates) != 0 {
					t.Fatalf("%s %s: shipped 0.2.20 output must pair entirely, got %#v", testCase.target, event, outcome)
				}
				for _, pairing := range outcome.paired {
					shipped = append(shipped, pairing.hookID)
				}
			}
			sort.Strings(shipped)
			want := append([]string{}, cohort...)
			sort.Strings(want)
			if !reflect.DeepEqual(shipped, want) {
				t.Fatalf("%s 0.2.20 shipped %v, cohort records %v", testCase.target, shipped, want)
			}
		})
	}
}

func TestHookCatalogRejectsStemsThatOverlapAnotherIdentity(t *testing.T) {
	_, err := newHookCatalog("cursor", "test", []hookCatalogSource{
		{event: "preToolUse", hookID: "check", typeName: "command", command: "loaf check --hook check", template: map[string]any{"command": "loaf check --hook check"}},
		{event: "preToolUse", hookID: "wrapper", typeName: "command", command: "loaf run --hook check --wrap", template: map[string]any{"command": "loaf run --hook check --wrap"}},
	})
	if err == nil || !strings.Contains(err.Error(), "also matches") {
		t.Fatalf("newHookCatalog error = %v, want an overlapping-stem refusal", err)
	}
}

func TestHookCatalogRejectsStemsThatOverlapAnotherSignature(t *testing.T) {
	_, err := newHookCatalog("cursor", "test", []hookCatalogSource{
		{event: "preToolUse", hookID: "narrow", typeName: "command", command: "loaf task refresh", template: map[string]any{"command": "loaf task refresh"}},
		{event: "postToolUse", hookID: "wide", typeName: "command", command: "loaf task refresh --all", template: map[string]any{"command": "loaf task refresh --all"}},
	})
	if err == nil || !strings.Contains(err.Error(), "also matches") {
		t.Fatalf("newHookCatalog error = %v, want an overlapping-signature refusal", err)
	}
}

func TestHookCatalogRejectsDuplicateIdentity(t *testing.T) {
	_, err := newHookCatalog("cursor", "test", []hookCatalogSource{
		{event: "preToolUse", hookID: "check-secrets", typeName: "command", command: "loaf check --hook check-secrets", template: map[string]any{"command": "loaf check --hook check-secrets"}},
		{event: "preToolUse", hookID: "check-secrets", typeName: "command", command: "loaf check --hook check-secrets --advisory", template: map[string]any{"command": "x"}},
	})
	if err == nil || !strings.Contains(err.Error(), "twice") {
		t.Fatalf("newHookCatalog error = %v, want a duplicate-identity refusal", err)
	}
}

// An empty catalog is not a catalog with nothing to say — it is a claim that
// this version ships no hooks, which would read every installed Loaf entry as a
// retired generation and remove all of them while projecting nothing back.
func TestHookCatalogRejectsAnEmptyCatalog(t *testing.T) {
	if _, err := newHookCatalog("cursor", "test", nil); err == nil || !strings.Contains(err.Error(), "no entries") {
		t.Fatalf("newHookCatalog(nil) error = %v, want an empty-catalog refusal", err)
	}

	empty := t.TempDir()
	writeFile(t, filepath.Join(empty, hookCatalogFile), `{"version":1,"target":"cursor","package_version":"test","entries":[],"cohorts":[]}`)
	if _, err := readHookCatalog(empty); err == nil || !strings.Contains(err.Error(), "no entries") {
		t.Fatalf("readHookCatalog(empty) error = %v, want an empty-catalog refusal", err)
	}
}

// The template is what reconciliation publishes. Anything that is not an entry
// the target can carry would reach the file before post-verify could notice.
func TestHookCatalogRejectsTemplatesThatAreNotEntries(t *testing.T) {
	for name, template := range map[string]any{
		"null":   nil,
		"array":  []any{map[string]any{"command": "loaf task refresh"}},
		"scalar": "loaf task refresh",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := newHookCatalog("cursor", "test", []hookCatalogSource{{
				event: "postToolUse", hookID: "generate-task-board", typeName: "command",
				command: "loaf task refresh", template: template,
			}})
			if err == nil || !strings.Contains(err.Error(), "not an entry object") {
				t.Fatalf("newHookCatalog error = %v, want a non-object template refusal", err)
			}
		})
	}

	for name, template := range map[string]string{
		"two handlers":   `{"matcher":"startup","hooks":[{"type":"command","command":"loaf journal context --from-hook --codex-hook"},{"type":"command","command":"other"}]}`,
		"no handler":     `{"matcher":"startup","hooks":[]}`,
		"prompt handler": `{"matcher":"startup","hooks":[{"type":"prompt"}]}`,
	} {
		t.Run("codex "+name, func(t *testing.T) {
			dist := t.TempDir()
			writeFile(t, filepath.Join(dist, hookCatalogFile), `{"version":1,"target":"codex","package_version":"test","entries":[{"event":"SessionStart","hook_id":"session-start-loaf","type":"command","template":`+template+`,"signatures":[["loaf","journal","context","--from-hook","--codex-hook"]]}],"cohorts":[]}`)
			if _, err := readHookCatalog(dist); err == nil || !strings.Contains(err.Error(), "codex hook catalog entry") {
				t.Fatalf("readHookCatalog error = %v, want a matcher-group refusal", err)
			}
		})
	}
}

// Two identities that answer to the same command are ambiguous by construction:
// which one a mutated entry pairs to would come down to catalog order.
func TestHookCatalogRejectsASignatureClaimedByTwoIdentities(t *testing.T) {
	_, err := newHookCatalog("cursor", "test", []hookCatalogSource{
		{event: "preToolUse", hookID: "first", typeName: "command", command: "loaf check --hook shared", template: map[string]any{"command": "loaf check --hook shared"}},
		{event: "postToolUse", hookID: "second", typeName: "command", command: "loaf check --hook shared", template: map[string]any{"command": "loaf check --hook shared", "matcher": "Bash"}},
	})
	if err == nil || !strings.Contains(err.Error(), "belongs to both") {
		t.Fatalf("newHookCatalog error = %v, want a duplicate-signature refusal", err)
	}
}

func TestHookCatalogRejectsInconsistentCohorts(t *testing.T) {
	dist := t.TempDir()
	entry := `{"event":"postToolUse","hook_id":"generate-task-board","type":"command","template":{"command":"loaf task refresh"},"signatures":[["loaf","task","refresh"]]}`
	for name, cohorts := range map[string]string{
		"duplicate version": `[{"version":"0.2.20","hook_ids":["a"]},{"version":"0.2.20","hook_ids":["b"]}]`,
		"empty version":     `[{"version":"","hook_ids":["a"]}]`,
		"no hook ids":       `[{"version":"0.2.20","hook_ids":[]}]`,
		"duplicate hook id": `[{"version":"0.2.20","hook_ids":["a","a"]}]`,
	} {
		t.Run(name, func(t *testing.T) {
			writeFile(t, filepath.Join(dist, hookCatalogFile), `{"version":1,"target":"cursor","package_version":"test","entries":[`+entry+`],"cohorts":`+cohorts+`}`)
			if _, err := readHookCatalog(dist); err == nil || !strings.Contains(err.Error(), "cohort") {
				t.Fatalf("readHookCatalog error = %v, want a cohort refusal", err)
			}
		})
	}
}

// A catalog that cannot be read is an error, never an empty identity authority:
// an empty catalog would silently make every installed Loaf entry foreign.
func TestReadHookCatalogFailsClosed(t *testing.T) {
	missing := t.TempDir()
	if _, err := readHookCatalog(missing); err == nil {
		t.Fatal("readHookCatalog(missing) error = nil, want failure")
	}

	malformed := t.TempDir()
	writeFile(t, filepath.Join(malformed, hookCatalogFile), "{ not json")
	if _, err := readHookCatalog(malformed); err == nil {
		t.Fatal("readHookCatalog(malformed) error = nil, want failure")
	}

	future := t.TempDir()
	writeFile(t, filepath.Join(future, hookCatalogFile), `{"version":99,"target":"cursor","entries":[],"cohorts":[]}`)
	if _, err := readHookCatalog(future); err == nil || !strings.Contains(err.Error(), "unsupported version") {
		t.Fatalf("readHookCatalog(future) error = %v, want an unsupported-version refusal", err)
	}
}

func testRepoHookCatalog(t *testing.T, target string) hookCatalog {
	t.Helper()
	dist := t.TempDir()
	switch target {
	case "cursor":
		if err := generateNativeCursorHookCatalog(filepath.Join(testRepositoryRoot(t), "config", "hooks.yaml"), dist, "test"); err != nil {
			t.Fatalf("generateNativeCursorHookCatalog error = %v", err)
		}
	case "codex":
		if err := generateNativeCodexHookCatalog(dist, "test"); err != nil {
			t.Fatalf("generateNativeCodexHookCatalog error = %v", err)
		}
	default:
		t.Fatalf("no hook catalog builder for target %q", target)
	}
	catalog, err := readHookCatalog(dist)
	if err != nil {
		t.Fatalf("readHookCatalog(%s) error = %v", target, err)
	}
	return catalog
}

func testHookFixture(t *testing.T, name string) []byte {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(testRepositoryRoot(t), "internal", "cli", "testdata", "hooks", name))
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", name, err)
	}
	return body
}

func testHookEventEntries(t *testing.T, body []byte) map[string][]map[string]any {
	t.Helper()
	var file struct {
		Hooks map[string][]map[string]any `json:"hooks"`
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := decoder.Decode(&file); err != nil {
		t.Fatalf("decode hooks file error = %v", err)
	}
	return file.Hooks
}
