package cli

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

const (
	harnessDriftBinaryFixtureVersion = "2.0.0-alpha.17"
	harnessDriftStaleFixtureVersion  = "2.0.0-alpha.16"
	harnessDriftNewerFixtureVersion  = "2.0.0-alpha.18"
)

// harnessDriftHome isolates HOME and every config-dir override so the drift
// surfaces read fixture markers instead of the developer's real harnesses.
func harnessDriftHome(t *testing.T) string {
	t.Helper()
	home := realpath(t, t.TempDir())
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("CODEX_HOME", filepath.Join(home, ".codex"))
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(home, ".claude"))
	return home
}

// harnessDriftInstalledHarness makes one config dir look like a real Loaf
// install: a skills payload marks it installed, and the marker (when supplied)
// records which version that payload came from.
func harnessDriftInstalledHarness(t *testing.T, configDir string, marker string) {
	t.Helper()
	mkdirAll(t, filepath.Join(configDir, "skills", "foundations"))
	if marker != "" {
		writeInstallFile(t, filepath.Join(configDir, loafInstallMarkerFile), marker+"\n")
	}
}

func harnessDriftDistribution(t *testing.T, version string) string {
	t.Helper()
	root := realpath(t, t.TempDir())
	writeFile(t, filepath.Join(root, "package.json"), `{"name":"loaf","version":"`+version+`"}`+"\n")
	return root
}

func TestHarnessContentDriftDoctorReportsEachMarkerState(t *testing.T) {
	for _, tc := range []struct {
		name       string
		marker     string
		wantStatus doctorStatus
		wantDetail string
		reject     string
	}{
		{name: "stale_marker_names_upgrade", marker: harnessDriftStaleFixtureVersion, wantStatus: doctorWarn, wantDetail: "Cursor content is 2.0.0-alpha.16"},
		{name: "current_marker_passes", marker: harnessDriftBinaryFixtureVersion, wantStatus: doctorPass},
		{name: "missing_marker_reports_unknown", marker: "", wantStatus: doctorWarn, wantDetail: "content version is unknown"},
		{name: "unparseable_marker_reports_unknown", marker: "not-a-version", wantStatus: doctorWarn, wantDetail: "is unreadable"},
		{name: "newer_marker_names_both_directions", marker: harnessDriftNewerFixtureVersion, wantStatus: doctorWarn, wantDetail: "brew upgrade loaf", reject: "the binary is the stale side"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := harnessDriftHome(t)
			harnessDriftInstalledHarness(t, filepath.Join(home, ".cursor"), tc.marker)

			result := checkHarnessContentDrift().Run(doctorContext{projectRoot: home, cliVersion: harnessDriftBinaryFixtureVersion})

			if result.Status != tc.wantStatus {
				t.Fatalf("harness-content-drift result = %#v, want status %q", result, tc.wantStatus)
			}
			if tc.wantDetail != "" && !strings.Contains(result.Detail, tc.wantDetail) {
				t.Fatalf("harness-content-drift detail = %q, want containing %q", result.Detail, tc.wantDetail)
			}
			if tc.reject != "" && (strings.Contains(result.Message, tc.reject) || strings.Contains(result.Detail, tc.reject)) {
				t.Fatalf("harness-content-drift result = %#v, must not contain %q", result, tc.reject)
			}
			if result.Fixable {
				t.Fatal("harness-content-drift must stay report-only")
			}
		})
	}
}

func TestHarnessContentDriftDoctorSkipsWithoutSubject(t *testing.T) {
	t.Run("no_installed_harness", func(t *testing.T) {
		home := harnessDriftHome(t)
		result := checkHarnessContentDrift().Run(doctorContext{projectRoot: home, cliVersion: harnessDriftBinaryFixtureVersion})
		if result.Status != doctorSkip {
			t.Fatalf("harness-content-drift result = %#v, want skip with no installed harness", result)
		}
	})

	t.Run("unresolvable_binary_version", func(t *testing.T) {
		home := harnessDriftHome(t)
		harnessDriftInstalledHarness(t, filepath.Join(home, ".cursor"), harnessDriftStaleFixtureVersion)
		result := checkHarnessContentDrift().Run(doctorContext{projectRoot: home})
		if result.Status != doctorSkip {
			t.Fatalf("harness-content-drift result = %#v, want skip without a binary version to compare", result)
		}
	})
}

func TestHarnessContentDriftDoctorCountsEveryInstalledHarness(t *testing.T) {
	home := harnessDriftHome(t)
	harnessDriftInstalledHarness(t, filepath.Join(home, ".cursor"), harnessDriftStaleFixtureVersion)
	harnessDriftInstalledHarness(t, filepath.Join(home, ".codex"), harnessDriftBinaryFixtureVersion)
	harnessDriftInstalledHarness(t, filepath.Join(home, ".config", "opencode"), "")

	result := checkHarnessContentDrift().Run(doctorContext{projectRoot: home, cliVersion: harnessDriftBinaryFixtureVersion})

	if result.Status != doctorWarn {
		t.Fatalf("harness-content-drift result = %#v, want warn", result)
	}
	if !strings.Contains(result.Message, "2 of 3 installed harnesses") {
		t.Fatalf("harness-content-drift message = %q, want a 2-of-3 tally", result.Message)
	}
	if strings.Contains(result.Detail, "Codex") {
		t.Fatalf("harness-content-drift detail = %q, must not report the current harness", result.Detail)
	}
	for _, want := range []string{"Cursor content is 2.0.0-alpha.16", "OpenCode content version is unknown"} {
		if !strings.Contains(result.Detail, want) {
			t.Fatalf("harness-content-drift detail = %q, want containing %q", result.Detail, want)
		}
	}
}

// TestHarnessDriftBinaryVersionIgnoresTheDevStamp pins the split between the
// two versions a dev build carries. `loaf --version` reports the build's own
// timestamp identity; drift compares markers against the distribution's release
// version, because that is the content a dev binary installs and the number it
// stamps into the marker. Comparing a marker against the build clock instead
// would report drift on every harness of every dev machine.
func TestHarnessDriftBinaryVersionIgnoresTheDevStamp(t *testing.T) {
	distRoot := markSourceCheckout(t, harnessDriftDistribution(t, "0.2.20"))
	runner := Runner{Executable: distributionFixtureExecutable(distRoot), DevBuildTime: devBuildFixtureTime}

	if got := runner.reportedVersion(distRoot); got != "0.2.1754593012" {
		t.Fatalf("reportedVersion = %q, want the dev identity", got)
	}
	binaryVersion := harnessDriftBinaryVersion(runner)
	if binaryVersion != "0.2.20" {
		t.Fatalf("harnessDriftBinaryVersion = %q, want the distribution's release version", binaryVersion)
	}
	if got := classifyHarnessDrift("0.2.20", binaryVersion); got != harnessDriftCurrent {
		t.Fatalf("classifyHarnessDrift(release marker, dev binary) = %q, want %q", got, harnessDriftCurrent)
	}
}

// TestHarnessDriftReadsDevMarkersAsContentDrift covers the other side: content
// installed by a dev build carries a timestamp marker that outranks every
// published version by construction, so it can never mean the binary is behind.
func TestHarnessDriftReadsDevMarkersAsContentDrift(t *testing.T) {
	const devMarker = "0.2.1754593012"

	if got := classifyHarnessDrift(devMarker, "0.2.20"); got != harnessDriftContentStale {
		t.Fatalf("classifyHarnessDrift(%q, %q) = %q, want %q", devMarker, "0.2.20", got, harnessDriftContentStale)
	}

	home := harnessDriftHome(t)
	harnessDriftInstalledHarness(t, filepath.Join(home, ".cursor"), devMarker)
	result := checkHarnessContentDrift().Run(doctorContext{projectRoot: home, cliVersion: "0.2.20"})

	if result.Status != doctorWarn {
		t.Fatalf("harness-content-drift result = %#v, want warn", result)
	}
	if !strings.Contains(result.Detail, "Cursor content is "+devMarker) || !strings.Contains(result.Detail, "run `loaf upgrade`") {
		t.Fatalf("harness-content-drift detail = %q, want the dev marker reported as content to refresh", result.Detail)
	}
	if strings.Contains(result.Detail, "brew upgrade loaf") {
		t.Fatalf("harness-content-drift detail = %q, must not blame the binary for a build-clock marker", result.Detail)
	}
}

// TestHarnessDriftAdviceSurvivesTheVersionSchemeReset covers the transit this
// repo performed: markers stamped by 2.0.0-alpha.19 sit above the 0.2.20 binary
// that replaced them, so the marker is higher while the binary is the newer
// side. The state stays binary-stale — nothing in a marker can prove otherwise
// — but the advice must name `loaf upgrade`, the command that actually resolves
// it, instead of sending the reader to upgrade a binary that is already current.
func TestHarnessDriftAdviceSurvivesTheVersionSchemeReset(t *testing.T) {
	const marker = "2.0.0-alpha.19"
	const binaryVersion = "0.2.20"

	if got := classifyHarnessDrift(marker, binaryVersion); got != harnessDriftBinaryStale {
		t.Fatalf("classifyHarnessDrift(%q, %q) = %q, want %q", marker, binaryVersion, got, harnessDriftBinaryStale)
	}

	home := harnessDriftHome(t)
	harnessDriftInstalledHarness(t, filepath.Join(home, ".cursor"), marker)
	result := checkHarnessContentDrift().Run(doctorContext{projectRoot: home, cliVersion: binaryVersion})

	if result.Status != doctorWarn {
		t.Fatalf("harness-content-drift result = %#v, want warn", result)
	}
	for _, want := range []string{"Cursor content is " + marker, "ahead of the binary's " + binaryVersion, "brew upgrade loaf", "run `loaf upgrade`"} {
		if !strings.Contains(result.Detail, want) {
			t.Fatalf("harness-content-drift detail = %q, want containing %q", result.Detail, want)
		}
	}
	if strings.Contains(result.Detail, "the binary is the stale side") {
		t.Fatalf("harness-content-drift detail = %q, must not assert a direction the marker cannot prove", result.Detail)
	}
}

func TestHarnessContentDriftCheckIsRegisteredAndReportOnly(t *testing.T) {
	for _, check := range doctorChecks() {
		if check.Name != "harness-content-drift" {
			continue
		}
		if check.Fix != nil || check.Repair != "" || check.RepairID != "" {
			t.Fatalf("harness-content-drift check = %#v, want no repair surface", check)
		}
		return
	}
	t.Fatal("doctorChecks() does not register harness-content-drift")
}

// TestCompareHarnessDriftVersionsSeparatesUnparseableFromEqual guards the one
// thing the wrapper adds over the shared semver comparison: an unparseable side
// reports "no comparison" rather than zero. Ordering itself belongs to
// TestCompareUpgradeSemverFollowsPrereleasePrecedence; what is pinned here is
// that a garbage marker never arrives at the caller looking like a tie.
func TestCompareHarnessDriftVersionsSeparatesUnparseableFromEqual(t *testing.T) {
	for _, tc := range []struct {
		left  string
		right string
		want  int
		fails bool
	}{
		{left: "2.0.0-alpha.16", right: "2.0.0-alpha.17", want: -1},
		{left: "2.0.0-alpha.18", right: "2.0.0-alpha.17", want: 1},
		{left: "v2.0.0", right: "2.0.0+build.5", want: 0},
		{left: "2.0.0-alpha.17", right: "2.0.0-alpha.17", want: 0},
		{left: "not-a-version", right: "2.0.0", fails: true},
		{left: "2.0.0", right: "", fails: true},
		{left: "2.0", right: "2.0.0", fails: true},
	} {
		t.Run(tc.left+"_vs_"+tc.right, func(t *testing.T) {
			got, ok := compareHarnessDriftVersions(tc.left, tc.right)
			if tc.fails {
				if ok {
					t.Fatalf("compareHarnessDriftVersions(%q, %q) = %d, true; want unparseable", tc.left, tc.right, got)
				}
				return
			}
			if !ok || got != tc.want {
				t.Fatalf("compareHarnessDriftVersions(%q, %q) = %d, %t; want %d, true", tc.left, tc.right, got, ok, tc.want)
			}
		})
	}
}

// TestClassifyHarnessDriftKeepsUnparseableApartFromCurrent pins the same
// distinction at the classifier that consumes it: doctor's unknown-state report
// and SessionStart's silence both depend on an unreadable marker landing on
// harnessDriftUnknown, not on harnessDriftCurrent.
func TestClassifyHarnessDriftKeepsUnparseableApartFromCurrent(t *testing.T) {
	for _, tc := range []struct {
		name   string
		marker string
		want   harnessDriftState
	}{
		{name: "equal", marker: harnessDriftBinaryFixtureVersion, want: harnessDriftCurrent},
		{name: "older", marker: harnessDriftStaleFixtureVersion, want: harnessDriftContentStale},
		{name: "newer", marker: harnessDriftNewerFixtureVersion, want: harnessDriftBinaryStale},
		{name: "unparseable", marker: "not-a-version", want: harnessDriftUnknown},
		{name: "missing", marker: "", want: harnessDriftUnknown},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := classifyHarnessDrift(tc.marker, harnessDriftBinaryFixtureVersion); got != tc.want {
				t.Fatalf("classifyHarnessDrift(%q, %q) = %q, want %q", tc.marker, harnessDriftBinaryFixtureVersion, got, tc.want)
			}
		})
	}
}

// TestHarnessDriftReadsNonCanonicalMarkersAsUnknown is the drift half of the
// strict parse. A marker whose precedence nobody agreed on is an unknown
// harness state — doctor says so and SessionStart stays quiet — rather than a
// version guessed at and then compared.
func TestHarnessDriftReadsNonCanonicalMarkersAsUnknown(t *testing.T) {
	for _, testCase := range upgradeNonCanonicalVersions {
		t.Run(testCase.name, func(t *testing.T) {
			if got := classifyHarnessDrift(testCase.value, harnessDriftBinaryFixtureVersion); got != harnessDriftUnknown {
				t.Fatalf("classifyHarnessDrift(%q, %q) = %q, want %q", testCase.value, harnessDriftBinaryFixtureVersion, got, harnessDriftUnknown)
			}
			if _, ok := compareHarnessDriftVersions(testCase.value, harnessDriftBinaryFixtureVersion); ok {
				t.Fatalf("compareHarnessDriftVersions(%q, ...) reported a comparison, want unparseable", testCase.value)
			}
		})
	}
}

// harnessDriftSessionStartVariant is one target-specific --from-hook dispatch
// form: its own invocation, its own native payload shape, and the config dir
// its marker lives in.
type harnessDriftSessionStartVariant struct {
	name      string
	args      []string
	configDir func(home string) string
	input     func(workingDir string) string
	extract   func(t *testing.T, output string) string
}

// harnessDriftSessionStartVariants lists the variants whose harness Loaf
// delivers content to, and which therefore have a marker to compare. Claude
// Code is not among them by design — see
// TestClaudeCodeSessionStartIsSilentOnDriftByDesign.
func harnessDriftSessionStartVariants() []harnessDriftSessionStartVariant {
	return []harnessDriftSessionStartVariant{
		{
			name:      "cursor",
			args:      []string{"journal", "context", "--from-hook", "--cursor-hook"},
			configDir: func(home string) string { return filepath.Join(home, ".cursor") },
			input: func(string) string {
				return `{"hook_event_name":"sessionStart","is_background_agent":false,"cursor_version":"3.11.19"}`
			},
			extract: func(t *testing.T, output string) string {
				t.Helper()
				var payload cursorSessionStartOutput
				if err := json.Unmarshal([]byte(output), &payload); err != nil {
					t.Fatalf("cursor output = %q: %v", output, err)
				}
				return payload.AdditionalContext
			},
		},
		{
			name:      "codex",
			args:      []string{"journal", "context", "--from-hook", "--codex-hook"},
			configDir: func(home string) string { return filepath.Join(home, ".codex") },
			input: func(workingDir string) string {
				return `{"cwd":"` + workingDir + `","hook_event_name":"SessionStart","model":"gpt-5.6","permission_mode":"default","session_id":"s1","source":"startup","transcript_path":null}`
			},
			extract: func(t *testing.T, output string) string {
				t.Helper()
				var payload codexSessionStartOutput
				if err := json.Unmarshal([]byte(output), &payload); err != nil {
					t.Fatalf("codex output = %q: %v", output, err)
				}
				if payload.HookSpecificOutput == nil {
					t.Fatalf("codex output = %q, want hookSpecificOutput", output)
				}
				return payload.HookSpecificOutput.AdditionalContext
			},
		},
		{
			name:      "opencode",
			args:      []string{"journal", "context", "--from-hook", "--opencode-hook"},
			configDir: func(home string) string { return filepath.Join(home, ".config", "opencode") },
			input: func(string) string {
				return openCodeSessionStartPayload
			},
			// OpenCode consumes plain text, so the digest is the output itself.
			extract: func(t *testing.T, output string) string {
				t.Helper()
				return output
			},
		},
	}
}

func harnessDriftSessionStartVariantsByName() map[string]harnessDriftSessionStartVariant {
	variants := map[string]harnessDriftSessionStartVariant{}
	for _, variant := range harnessDriftSessionStartVariants() {
		variants[variant.name] = variant
	}
	return variants
}

func runHarnessDriftSessionStart(t *testing.T, variant harnessDriftSessionStartVariant, workingDir string, stateHome string, distRoot string, input string) string {
	t.Helper()
	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		WorkingDir: workingDir,
		StateHome:  stateHome,
		Stdin:      strings.NewReader(input),
		Executable: distributionFixtureExecutable(distRoot),
	}.Run(variant.args)
	if err != nil {
		t.Fatalf("%v error = %v\n%s", variant.args, err, stdout.String())
	}
	return stdout.String()
}

func TestSessionStartCarriesDriftNudgeInEveryHarnessVariant(t *testing.T) {
	wantNudge := "Loaf content in this harness is " + harnessDriftStaleFixtureVersion + "; binary is " + harnessDriftBinaryFixtureVersion + " — run loaf upgrade"
	for _, variant := range harnessDriftSessionStartVariants() {
		for _, tc := range []struct {
			name      string
			marker    string
			wantNudge bool
		}{
			{name: "stale_marker_nudges", marker: harnessDriftStaleFixtureVersion, wantNudge: true},
			{name: "current_marker_silent", marker: harnessDriftBinaryFixtureVersion},
			{name: "missing_marker_silent", marker: ""},
			{name: "unparseable_marker_silent", marker: "nonsense"},
			{name: "newer_marker_silent", marker: harnessDriftNewerFixtureVersion},
		} {
			t.Run(variant.name+"/"+tc.name, func(t *testing.T) {
				home := harnessDriftHome(t)
				harnessDriftInstalledHarness(t, variant.configDir(home), tc.marker)
				distRoot := harnessDriftDistribution(t, harnessDriftBinaryFixtureVersion)
				workingDir, stateHome := setupJournalHookRunner(t)
				if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"journal", "log", "wrap(drift): continuity marker"}); err != nil {
					t.Fatal(err)
				}

				output := runHarnessDriftSessionStart(t, variant, workingDir, stateHome, distRoot, variant.input(workingDir))
				digest := variant.extract(t, output)

				if !strings.Contains(digest, "wrap(drift): continuity marker") {
					t.Fatalf("digest = %q, want the continuity digest itself", digest)
				}
				occurrences := strings.Count(digest, wantNudge)
				if tc.wantNudge && occurrences != 1 {
					t.Fatalf("digest = %q, want exactly one %q (got %d)", digest, wantNudge, occurrences)
				}
				if !tc.wantNudge && occurrences != 0 {
					t.Fatalf("digest = %q, want no drift nudge for marker %q", digest, tc.marker)
				}
				if !tc.wantNudge && strings.Contains(digest, "run loaf upgrade") {
					t.Fatalf("digest = %q, want session start silent for marker %q", digest, tc.marker)
				}
			})
		}
	}
}

func TestSessionStartNudgeReadsOnlyTheInvokingHarnessMarker(t *testing.T) {
	home := harnessDriftHome(t)
	harnessDriftInstalledHarness(t, filepath.Join(home, ".cursor"), harnessDriftStaleFixtureVersion)
	harnessDriftInstalledHarness(t, filepath.Join(home, ".codex"), harnessDriftBinaryFixtureVersion)
	distRoot := harnessDriftDistribution(t, harnessDriftBinaryFixtureVersion)
	workingDir, stateHome := setupJournalHookRunner(t)

	variants := harnessDriftSessionStartVariantsByName()

	codex := variants["codex"]
	codexDigest := codex.extract(t, runHarnessDriftSessionStart(t, codex, workingDir, stateHome, distRoot, codex.input(workingDir)))
	if strings.Contains(codexDigest, "run loaf upgrade") {
		t.Fatalf("codex digest = %q, want silence: Codex content is current even though Cursor drifted", codexDigest)
	}

	cursor := variants["cursor"]
	cursorDigest := cursor.extract(t, runHarnessDriftSessionStart(t, cursor, workingDir, stateHome, distRoot, cursor.input(workingDir)))
	if !strings.Contains(cursorDigest, "Loaf content in this harness is "+harnessDriftStaleFixtureVersion) {
		t.Fatalf("cursor digest = %q, want the drift nudge for its own stale marker", cursorDigest)
	}
}

// TestSessionStartDriftNudgeStaysSilentForSubagents pins the ordering every
// variant depends on: suppression short-circuits before the nudge, so a
// subagent inherits silence even when its harness content is stale.
func TestSessionStartDriftNudgeStaysSilentForSubagents(t *testing.T) {
	for _, tc := range []struct {
		name    string
		subdirs []string
		input   string
	}{
		{
			name:    "cursor",
			subdirs: []string{".cursor"},
			input:   `{"hook_event_name":"sessionStart","is_background_agent":true,"cursor_version":"3.11.19"}`,
		},
		{
			name:    "opencode",
			subdirs: []string{".config", "opencode"},
			input:   `{"target":"opencode","session_id":"ses_1","lifecycle_event":"system.transform","agent_id":"child-1"}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := harnessDriftHome(t)
			harnessDriftInstalledHarness(t, filepath.Join(append([]string{home}, tc.subdirs...)...), harnessDriftStaleFixtureVersion)
			distRoot := harnessDriftDistribution(t, harnessDriftBinaryFixtureVersion)
			workingDir, stateHome := setupJournalHookRunner(t)

			variants := harnessDriftSessionStartVariantsByName()
			output := runHarnessDriftSessionStart(t, variants[tc.name], workingDir, stateHome, distRoot, tc.input)

			if output != "" {
				t.Fatalf("subagent output = %q, want silent exit even with a stale marker", output)
			}
		})
	}
}

// TestClaudeCodeSessionStartIsSilentOnDriftByDesign pins the deliberate hole in
// the drift surface. Claude Code content ships on the plugin-marketplace
// channel: nothing Loaf runs stamps a marker into its config home, and
// `loaf upgrade` could not refresh that content if it did — so a fabricated
// marker must produce neither a session-start nudge nor a doctor finding,
// rather than advice that cannot work.
func TestClaudeCodeSessionStartIsSilentOnDriftByDesign(t *testing.T) {
	home := harnessDriftHome(t)
	harnessDriftInstalledHarness(t, filepath.Join(home, ".claude"), harnessDriftStaleFixtureVersion)
	distRoot := harnessDriftDistribution(t, harnessDriftBinaryFixtureVersion)
	workingDir, stateHome := setupJournalHookRunner(t)
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"journal", "log", "wrap(drift): continuity marker"}); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	if err := (Runner{
		Stdout:     &stdout,
		WorkingDir: workingDir,
		StateHome:  stateHome,
		Stdin:      strings.NewReader(`{"hook_event_name":"SessionStart","source":"startup","session_id":"s1"}`),
		Executable: distributionFixtureExecutable(distRoot),
	}).Run([]string{"journal", "context", "--from-hook", "--claude-code"}); err != nil {
		t.Fatalf("claude session start error = %v\n%s", err, stdout.String())
	}
	var payload claudeSessionStartOutput
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("claude output = %q: %v", stdout.String(), err)
	}
	if payload.HookSpecificOutput == nil {
		t.Fatalf("claude output = %q, want hookSpecificOutput", stdout.String())
	}
	digest := payload.HookSpecificOutput.AdditionalContext
	if !strings.Contains(digest, "wrap(drift): continuity marker") {
		t.Fatalf("digest = %q, want the continuity digest itself", digest)
	}
	if strings.Contains(digest, "run loaf upgrade") || strings.Contains(digest, "Loaf content in this harness") {
		t.Fatalf("digest = %q, want no drift advice for the marketplace channel", digest)
	}

	if result := checkHarnessContentDrift().Run(doctorContext{projectRoot: home, cliVersion: harnessDriftBinaryFixtureVersion}); result.Status != doctorSkip {
		t.Fatalf("harness-content-drift result = %#v, want doctor to skip a home with only Claude Code content", result)
	}
}
