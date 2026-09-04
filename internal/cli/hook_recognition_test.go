package cli

import (
	"encoding/json"
	"sort"
	"strings"
	"testing"
)

const testHookCanaryHome = "/Users/canary"

// Every entry 0.2.20 shipped into the canary's live Cursor file is recognized
// as Loaf's by construction — not one of them through the `loaf-managed`
// marker, which the predicate never reads.
func TestHookRecognitionClaimsEveryShippedCursorEntry(t *testing.T) {
	recognition := testHookRecognition(t, "cursor", testRepoHookCatalog(t, "cursor"))
	owned, foreign := testPartitionHookFixture(t, recognition, "cursor-hooks-live.json")

	if len(owned) != 17 {
		t.Fatalf("recognized %d live Cursor entries, want the 17 shipped entries: %v", len(owned), owned)
	}
	if len(foreign) != 33 {
		t.Fatalf("left %d live Cursor entries unclaimed, want 33 (32 legacy-generation plus one herdr)", len(foreign))
	}
}

// The 2026-03-25 generation and the third-party herdr hook are outside the
// closed recognition set forever, including the five legacy entries that
// functionally duplicate shipped enforcement hooks.
func TestHookRecognitionLeavesLegacyGenerationAndHerdrUnclaimed(t *testing.T) {
	recognition := testHookRecognition(t, "cursor", testRepoHookCatalog(t, "cursor"))
	_, foreign := testPartitionHookFixture(t, recognition, "cursor-hooks-live.json")

	for _, command := range []string{
		"bash $HOME/.cursor/hooks/pre-tool/foundations-check-secrets.sh",
		"bash $HOME/.cursor/hooks/pre-tool/foundations-security-audit.sh",
		"bash $HOME/.cursor/hooks/pre-tool/foundations-validate-push.sh",
		"python3 $HOME/.cursor/hooks/pre-tool/orchestration-validate-commit.py",
		"bash $HOME/.cursor/hooks/post-tool/orchestration-generate-task-board.sh",
		"bash '/Users/canary/.cursor/herdr-agent-state.sh' session",
	} {
		if !testContainsHookCommand(foreign, command) {
			t.Fatalf("command %q was claimed; it must stay foreign", command)
		}
	}
}

func TestHookRecognitionLeavesCodexHerdrGroupUnclaimed(t *testing.T) {
	recognition := testHookRecognition(t, "codex", testRepoHookCatalog(t, "codex"))
	owned, foreign := testPartitionHookFixture(t, recognition, "codex-hooks-live.json")

	if len(owned) != 0 || len(foreign) != 1 {
		t.Fatalf("live Codex file recognized %d owned and %d foreign entries, want 0 and 1", len(owned), len(foreign))
	}
}

// Ownership is by construction. Stripping the marker changes nothing, and
// adding it to a foreign entry claims nothing.
func TestHookRecognitionIgnoresTheLoafManagedMarker(t *testing.T) {
	recognition := testHookRecognition(t, "cursor", testRepoHookCatalog(t, "cursor"))
	events := testHookEventEntries(t, testHookFixture(t, "cursor-hooks-live.json"))

	owned := 0
	for _, entries := range events {
		for _, entry := range entries {
			delete(entry, loafHookMarker)
			ownership, err := recognition.ownsEntry(entry)
			if err != nil {
				t.Fatalf("ownsEntry error = %v", err)
			}
			if ownership.owned {
				owned++
			}
		}
	}
	if owned != 17 {
		t.Fatalf("recognized %d entries with the marker stripped, want 17", owned)
	}

	imposter := map[string]any{
		"command":       "bash '/Users/canary/.cursor/herdr-agent-state.sh' session",
		loafHookMarker:  true,
		"timeout":       json.Number("30"),
		"loaf-managed2": false,
	}
	ownership, err := recognition.ownsEntry(imposter)
	if err != nil {
		t.Fatalf("ownsEntry error = %v", err)
	}
	if ownership.owned {
		t.Fatal("a foreign entry wearing the loaf-managed marker was claimed")
	}
}

// The four path-backed entries are claimed by exact manifest destination, not
// by living under a Loaf-managed directory.
func TestHookRecognitionClaimsPathBackedEntriesByExactDestination(t *testing.T) {
	recognition := testHookRecognition(t, "cursor", testRepoHookCatalog(t, "cursor"))
	for _, command := range []string{
		"bash $HOME/.cursor/hooks/post-tool/kb-staleness-nudge.sh",
		`cat "$HOME/.cursor/hooks/instructions/pre-merge.md"`,
		`cat "$HOME/.cursor/hooks/instructions/pre-push.md"`,
		`cat "$HOME/.cursor/hooks/instructions/post-merge.md"`,
	} {
		ownership, err := recognition.ownsEntry(map[string]any{"command": command})
		if err != nil {
			t.Fatalf("ownsEntry(%q) error = %v", command, err)
		}
		if !ownership.owned || ownership.reason != hookOwnershipManagedPath {
			t.Fatalf("ownsEntry(%q) = %#v, want ownership through the recorded destination", command, ownership)
		}
	}
}

func TestHookRecognitionAcceptsEveryHomeSpellingOfADestination(t *testing.T) {
	recognition := testHookRecognition(t, "cursor", testRepoHookCatalog(t, "cursor"))
	for _, command := range []string{
		"bash $HOME/.cursor/hooks/post-tool/kb-staleness-nudge.sh",
		"bash ${HOME}/.cursor/hooks/post-tool/kb-staleness-nudge.sh",
		"bash ~/.cursor/hooks/post-tool/kb-staleness-nudge.sh",
		"bash /Users/canary/.cursor/hooks/post-tool/kb-staleness-nudge.sh",
		"bash '/Users/canary/.cursor/hooks/post-tool/kb-staleness-nudge.sh'",
		"bash /Users/canary/.cursor/hooks/../hooks/post-tool/kb-staleness-nudge.sh",
	} {
		ownership, err := recognition.ownsEntry(map[string]any{"command": command})
		if err != nil {
			t.Fatalf("ownsEntry(%q) error = %v", command, err)
		}
		if !ownership.owned {
			t.Fatalf("ownsEntry(%q) left the entry unclaimed", command)
		}
	}
}

// Quoting is part of what a command means. A single-quoted `$HOME` is a
// literal filename in every shell that runs these hooks, so expanding it here
// would claim a foreign entry that never pointed at a Loaf path at all — and
// `~` does not expand inside double quotes either.
func TestHookRecognitionDoesNotExpandQuotedHomePaths(t *testing.T) {
	recognition := testHookRecognition(t, "cursor", testRepoHookCatalog(t, "cursor"))
	for _, command := range []string{
		`cat '$HOME/.cursor/hooks/instructions/pre-merge.md'`,
		`bash '$HOME/.cursor/hooks/post-tool/kb-staleness-nudge.sh'`,
		`bash '${HOME}/.cursor/hooks/post-tool/kb-staleness-nudge.sh'`,
		`bash '~/.cursor/hooks/post-tool/kb-staleness-nudge.sh'`,
		`bash "~/.cursor/hooks/post-tool/kb-staleness-nudge.sh"`,
	} {
		ownership, err := recognition.ownsEntry(map[string]any{"command": command})
		if err != nil {
			t.Fatalf("ownsEntry(%q) error = %v", command, err)
		}
		if ownership.owned {
			t.Fatalf("ownsEntry(%q) = %#v, want a literal path left foreign", command, ownership)
		}
	}

	// The spellings the shell really does expand stay claimed.
	for _, command := range []string{
		`cat "$HOME/.cursor/hooks/instructions/pre-merge.md"`,
		`bash $HOME/.cursor/hooks/post-tool/kb-staleness-nudge.sh`,
		`bash ~/.cursor/hooks/post-tool/kb-staleness-nudge.sh`,
	} {
		ownership, err := recognition.ownsEntry(map[string]any{"command": command})
		if err != nil {
			t.Fatalf("ownsEntry(%q) error = %v", command, err)
		}
		if !ownership.owned {
			t.Fatalf("ownsEntry(%q) left an expanded Loaf destination unclaimed", command)
		}
	}
}

// A Windows entry spells its paths with backslashes, and cmd.exe does not treat
// them as escapes. Both halves have to hold for the destination to be
// recognized: the separators survive tokenization, and expansion happens after
// they are normalized.
func TestHookRecognitionClaimsWindowsSpelledDestinations(t *testing.T) {
	recognition := testHookRecognition(t, "cursor", testRepoHookCatalog(t, "cursor"))
	recognition.goos = "windows"
	recognition.homeDir = `C:\Users\canary`
	var manifest targetAdapterManifest
	if err := json.Unmarshal(testHookFixture(t, "cursor-target-manifest-0.2.20.json"), &manifest); err != nil {
		t.Fatalf("decode cursor manifest fixture error = %v", err)
	}
	recognition.managedPaths = hookManagedDestinations(`C:\Users\canary\.cursor`, manifest)

	for _, command := range []string{
		`bash $HOME\.cursor\hooks\post-tool\kb-staleness-nudge.sh`,
		`bash ~\.cursor\hooks\post-tool\kb-staleness-nudge.sh`,
		`bash C:\Users\canary\.cursor\hooks\post-tool\kb-staleness-nudge.sh`,
		`cat "$HOME\.cursor\hooks\instructions\pre-merge.md"`,
	} {
		ownership, err := recognition.ownsEntry(map[string]any{"command": command})
		if err != nil {
			t.Fatalf("ownsEntry(%q) error = %v", command, err)
		}
		if !ownership.owned || ownership.reason != hookOwnershipManagedPath {
			t.Fatalf("ownsEntry(%q) = %#v, want the Windows spelling recognized", command, ownership)
		}
	}

	single := `bash '$HOME\.cursor\hooks\post-tool\kb-staleness-nudge.sh'`
	ownership, err := recognition.ownsEntry(map[string]any{"command": single})
	if err != nil {
		t.Fatalf("ownsEntry(%q) error = %v", single, err)
	}
	if ownership.owned {
		t.Fatalf("ownsEntry(%q) = %#v, want a literal path left foreign on Windows too", single, ownership)
	}
}

// No directory containment: a sibling under the managed hooks directory that
// the manifest never recorded is not Loaf's. Claiming the directory would
// swallow the whole March generation.
func TestHookRecognitionRefusesDirectoryContainment(t *testing.T) {
	recognition := testHookRecognition(t, "cursor", testRepoHookCatalog(t, "cursor"))
	for _, command := range []string{
		"bash $HOME/.cursor/hooks/post-tool/kb-staleness-nudge.sh.bak",
		"bash $HOME/.cursor/hooks/post-tool/something-else.sh",
		"bash $HOME/.cursor/hooks/instructions/pre-merge.md.orig",
		"bash $HOME/.cursor/hooks/pre-tool/foundations-check-secrets.sh",
	} {
		ownership, err := recognition.ownsEntry(map[string]any{"command": command})
		if err != nil {
			t.Fatalf("ownsEntry(%q) error = %v", command, err)
		}
		if ownership.owned && ownership.reason == hookOwnershipManagedPath {
			t.Fatalf("ownsEntry(%q) claimed a path the manifest never recorded", command)
		}
	}
}

// An absolute executable path is Loaf's only when it is a trusted path: the
// currently resolved executable or one previously recorded for the target.
func TestHookRecognitionRequiresATrustedAbsoluteExecutable(t *testing.T) {
	catalog := testRepoHookCatalog(t, "cursor")
	untrusted := testHookRecognition(t, "cursor", catalog)
	entry := map[string]any{"command": "'/opt/imposter/loaf' check --hook check-secrets"}

	ownership, err := untrusted.ownsEntry(entry)
	if err != nil {
		t.Fatalf("ownsEntry error = %v", err)
	}
	if ownership.owned {
		t.Fatal("an untrusted absolute executable was claimed as Loaf's")
	}

	trusted := untrusted
	trusted.trustedPaths = []string{"/opt/imposter/loaf"}
	ownership, err = trusted.ownsEntry(entry)
	if err != nil {
		t.Fatalf("ownsEntry error = %v", err)
	}
	if !ownership.owned || ownership.hookID != "check-secrets" {
		t.Fatalf("ownsEntry with the path recorded = %#v, want check-secrets", ownership)
	}
}

// A previously recorded install path keeps recognizing entries written before
// Loaf moved, so relocation never orphans a live entry.
func TestHookRecognitionAcceptsAPreviouslyRecordedInstallPath(t *testing.T) {
	recognition := testHookRecognition(t, "cursor", testRepoHookCatalog(t, "cursor"))
	recognition.trustedPaths = []string{"/opt/homebrew/bin/loaf", "/Users/canary/.local/share/loaf/bin/loaf"}

	ownership, err := recognition.ownsEntry(map[string]any{"command": "'/Users/canary/.local/share/loaf/bin/loaf' task refresh"})
	if err != nil {
		t.Fatalf("ownsEntry error = %v", err)
	}
	if !ownership.owned || ownership.hookID != "" || ownership.reason != hookOwnershipLegacy {
		t.Fatalf("ownsEntry = %#v, want the previously recorded retired path recognized", ownership)
	}
}

// A loaf-invoking command carrying no catalog identity is the operator's own
// hook and stays theirs.
func TestHookRecognitionLeavesForeignLoafCommandsUnclaimed(t *testing.T) {
	recognition := testHookRecognition(t, "cursor", testRepoHookCatalog(t, "cursor"))
	for _, command := range []string{
		"loaf journal log 'decision(scope): my own hook'",
		"loaf check --hook check-secrets-disabled",
		"loaf change check",
	} {
		ownership, err := recognition.ownsEntry(map[string]any{"command": command})
		if err != nil {
			t.Fatalf("ownsEntry(%q) error = %v", command, err)
		}
		if ownership.owned {
			t.Fatalf("ownsEntry(%q) = %#v, want the operator's own hook left alone", command, ownership)
		}
	}
}

func TestHookRecognitionReportsCommandsCarryingTwoIdentities(t *testing.T) {
	recognition := testHookRecognition(t, "cursor", testRepoHookCatalog(t, "cursor"))
	_, err := recognition.ownsEntry(map[string]any{"command": "loaf check --hook check-secrets --hook validate-commit"})
	if err == nil || !strings.Contains(err.Error(), "more than one Loaf hook") {
		t.Fatalf("ownsEntry error = %v, want an integrity error naming both identities", err)
	}
}

// Prompt-prefix recognition is bounded to prompt-type entries: a command that
// merely quotes a legacy prompt is never claimed by it.
func TestHookRecognitionAppliesLegacyPromptPrefixesToPromptEntriesOnly(t *testing.T) {
	recognition := testHookRecognition(t, "cursor", testRepoHookCatalog(t, "cursor"))

	prompt := map[string]any{"prompt": "KNOWLEDGE BASE: covered files changed."}
	ownership, err := recognition.ownsEntry(prompt)
	if err != nil {
		t.Fatalf("ownsEntry(prompt) error = %v", err)
	}
	if !ownership.owned || ownership.reason != hookOwnershipLegacy {
		t.Fatalf("ownsEntry(prompt) = %#v, want the frozen legacy allowlist", ownership)
	}

	command := map[string]any{"command": "echo 'KNOWLEDGE BASE: covered files changed.'"}
	ownership, err = recognition.ownsEntry(command)
	if err != nil {
		t.Fatalf("ownsEntry(command) error = %v", err)
	}
	if ownership.owned {
		t.Fatal("a command entry was claimed by a legacy prompt prefix")
	}
}

func TestHookRecognitionClaimsCodexTemplateAndResolvedForms(t *testing.T) {
	catalog := testRepoHookCatalog(t, "codex")
	unix := testHookRecognition(t, "codex", catalog)
	unix.trustedPaths = []string{"/Users/canary/.local/bin/loaf"}

	template := testCodexGroup(codexJournalExecutablePlaceholder+codexJournalHookCommandSuffix, codexJournalExecutablePlaceholder+codexJournalHookCommandSuffix)
	ownership, err := unix.ownsEntry(template)
	if err != nil {
		t.Fatalf("ownsEntry(template) error = %v", err)
	}
	if !ownership.owned || ownership.hookID != "session-start-loaf" {
		t.Fatalf("ownsEntry(template) = %#v, want session-start-loaf", ownership)
	}

	resolved := testCodexGroup("'/Users/canary/.local/bin/loaf'"+codexJournalHookCommandSuffix, "")
	ownership, err = unix.ownsEntry(resolved)
	if err != nil {
		t.Fatalf("ownsEntry(resolved) error = %v", err)
	}
	if !ownership.owned || ownership.hookID != "session-start-loaf" {
		t.Fatalf("ownsEntry(resolved) = %#v, want session-start-loaf", ownership)
	}
}

// Windows parity is concrete: the template carries identical command and
// commandWindows values, and the installed cmd.exe form resolves to the same
// identity through the Windows quoting rules.
func TestHookRecognitionClaimsCodexWindowsCommandParity(t *testing.T) {
	catalog := testRepoHookCatalog(t, "codex")
	template, err := decodeHookJSONValue(catalog.Entries[0].Template)
	if err != nil {
		t.Fatalf("decode template error = %v", err)
	}
	handler := template.(map[string]any)["hooks"].([]any)[0].(map[string]any)
	if handler["command"] != handler["commandWindows"] {
		t.Fatalf("codex template command = %v, commandWindows = %v, want identical values", handler["command"], handler["commandWindows"])
	}

	windows := testHookRecognition(t, "codex", catalog)
	windows.goos = "windows"
	windows.homeDir = `C:\Users\canary`
	windows.trustedPaths = []string{`C:\Users\canary\AppData\Local\loaf\loaf.exe`}
	command, err := codexWindowsJournalContextCommand(`C:\Users\canary\AppData\Local\loaf\loaf.exe`)
	if err != nil {
		t.Fatalf("codexWindowsJournalContextCommand error = %v", err)
	}

	ownership, err := windows.ownsEntry(testCodexGroup(command, command))
	if err != nil {
		t.Fatalf("ownsEntry(windows) error = %v", err)
	}
	if !ownership.owned || ownership.hookID != "session-start-loaf" {
		t.Fatalf("ownsEntry(windows) = %#v, want session-start-loaf", ownership)
	}

	untrusted := windows
	untrusted.trustedPaths = nil
	ownership, err = untrusted.ownsEntry(testCodexGroup(command, command))
	if err != nil {
		t.Fatalf("ownsEntry(untrusted windows) error = %v", err)
	}
	if !ownership.owned || ownership.reason != hookOwnershipLegacy {
		t.Fatalf("ownsEntry(untrusted windows) = %#v, want the frozen 0.2.20 shape to still recognize it", ownership)
	}
}

// A matcher group the operator extended with a second handler is their
// construction, not Loaf's; reconciliation never edits it.
func TestHookRecognitionLeavesMultiHandlerCodexGroupsUnclaimed(t *testing.T) {
	recognition := testHookRecognition(t, "codex", testRepoHookCatalog(t, "codex"))
	recognition.trustedPaths = []string{"/Users/canary/.local/bin/loaf"}

	group := testCodexGroup("'/Users/canary/.local/bin/loaf'"+codexJournalHookCommandSuffix, "")
	handlers := group["hooks"].([]any)
	group["hooks"] = append(handlers, map[string]any{"type": "command", "command": "bash /Users/canary/.config/codex/herdr-agent-state.sh session"})

	ownership, err := recognition.ownsEntry(group)
	if err != nil {
		t.Fatalf("ownsEntry error = %v", err)
	}
	if ownership.owned {
		t.Fatalf("ownsEntry = %#v, want a hand-extended group left alone", ownership)
	}
}

func TestHookCommandTokensRespectsQuoting(t *testing.T) {
	for _, testCase := range []struct {
		command string
		want    []string
		ok      bool
	}{
		{command: `cat "$HOME/.cursor/hooks/instructions/pre-merge.md"`, want: []string{"cat", "$HOME/.cursor/hooks/instructions/pre-merge.md"}, ok: true},
		{command: `bash '/Users/canary/my hooks/run.sh' session`, want: []string{"bash", "/Users/canary/my hooks/run.sh", "session"}, ok: true},
		{command: `loaf journal log 'decision(scope): x'`, want: []string{"loaf", "journal", "log", "decision(scope): x"}, ok: true},
		{command: `loaf   task    refresh`, want: []string{"loaf", "task", "refresh"}, ok: true},
		{command: `bash "/Users/canary/it\"s/run.sh"`, want: []string{"bash", `/Users/canary/it"s/run.sh`}, ok: true},
		{command: `bash 'unterminated`, ok: false},
	} {
		tokens, ok := hookCommandTokens(testCase.command)
		if ok != testCase.ok {
			t.Fatalf("hookCommandTokens(%q) ok = %v, want %v", testCase.command, ok, testCase.ok)
		}
		if ok && !hookTokensEqual(tokens, testCase.want) {
			t.Fatalf("hookCommandTokens(%q) = %q, want %q", testCase.command, tokens, testCase.want)
		}
	}
}

func TestContainsHookTokenSequenceMatchesWholeTokensOnly(t *testing.T) {
	haystack := []string{"check", "--hook", "check-secrets-disabled"}
	if containsHookTokenSequence(haystack, []string{"--hook", "check-secrets"}) {
		t.Fatal("a longer token was matched as a prefix; identity must compare whole tokens")
	}
	if !containsHookTokenSequence(haystack, []string{"--hook", "check-secrets-disabled"}) {
		t.Fatal("an exact token run did not match")
	}
}

func testHookRecognition(t *testing.T, target string, catalog hookCatalog) hookRecognition {
	t.Helper()
	recognition := hookRecognition{
		target:  target,
		catalog: catalog,
		homeDir: testHookCanaryHome,
		goos:    "darwin",
	}
	if target == "cursor" {
		var manifest targetAdapterManifest
		if err := json.Unmarshal(testHookFixture(t, "cursor-target-manifest-0.2.20.json"), &manifest); err != nil {
			t.Fatalf("decode cursor manifest fixture error = %v", err)
		}
		recognition.managedPaths = hookManagedDestinations(testHookCanaryHome+"/.cursor", manifest)
	}
	return recognition
}

func testPartitionHookFixture(t *testing.T, recognition hookRecognition, fixture string) (owned []map[string]any, foreign []map[string]any) {
	t.Helper()
	events := testHookEventEntries(t, testHookFixture(t, fixture))
	names := make([]string, 0, len(events))
	for event := range events {
		names = append(names, event)
	}
	sort.Strings(names)
	for _, event := range names {
		for _, entry := range events[event] {
			ownership, err := recognition.ownsEntry(entry)
			if err != nil {
				t.Fatalf("ownsEntry(%s) error = %v", event, err)
			}
			if ownership.owned {
				owned = append(owned, entry)
			} else {
				foreign = append(foreign, entry)
			}
		}
	}
	return owned, foreign
}

func testContainsHookCommand(entries []map[string]any, command string) bool {
	for _, entry := range entries {
		if value, ok := entry["command"].(string); ok && value == command {
			return true
		}
	}
	return false
}

func testCodexGroup(command string, windowsCommand string) map[string]any {
	handler := map[string]any{"type": "command", "command": command}
	if windowsCommand != "" {
		handler["commandWindows"] = windowsCommand
	}
	return map[string]any{
		"matcher": codexJournalHookMatcher,
		"hooks":   []any{handler},
	}
}
