package cli

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Harness identities used by drift surfacing. Each one is an install key, so
// the marker lookup stays on the same table install stamps markers with.
// Claude Code is deliberately absent: its content ships through the plugin
// marketplace, nothing here ever writes a marker into its config home, and
// `loaf upgrade` cannot refresh it — so it has no drift to report and its
// SessionStart variant stays silent (see journal_hook_claude.go).
const (
	harnessDriftCursor   = "cursor"
	harnessDriftCodex    = "codex"
	harnessDriftOpenCode = "opencode"
)

// harnessDriftState classifies one harness's stamped content against the
// running binary. Both stale directions are named because they have different
// remediations: stale content is what `loaf upgrade` fixes, while a marker
// newer than the binary means the binary is the side left behind.
type harnessDriftState string

const (
	harnessDriftCurrent      harnessDriftState = "current"
	harnessDriftContentStale harnessDriftState = "content-stale"
	harnessDriftBinaryStale  harnessDriftState = "binary-stale"
	harnessDriftUnknown      harnessDriftState = "unknown"
)

type harnessDriftReading struct {
	target    string
	name      string
	configDir string
	marker    string
	state     harnessDriftState
}

// harnessDriftConfigDir maps one harness identity to the config directory its
// `.loaf-version` marker lives in, through the same table install writes the
// marker with.
func harnessDriftConfigDir(harness string) string {
	return defaultInstallConfigDirs()[harness]
}

func readHarnessVersionMarker(configDir string) string {
	if configDir == "" {
		return ""
	}
	body, err := readRegularFile(filepath.Join(configDir, loafInstallMarkerFile), projectFileReadLimit)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(body))
}

// classifyHarnessDrift implements the marker semantics: equal is current, an
// older marker is stale content, a newer marker makes the binary the stale
// side, and a missing or unparseable marker is an unknown state.
func classifyHarnessDrift(marker string, binaryVersion string) harnessDriftState {
	if marker == "" {
		return harnessDriftUnknown
	}
	comparison, ok := compareHarnessDriftVersions(marker, binaryVersion)
	if !ok {
		return harnessDriftUnknown
	}
	switch {
	case comparison < 0:
		return harnessDriftContentStale
	case comparison > 0:
		return harnessDriftBinaryStale
	default:
		return harnessDriftCurrent
	}
}

func readHarnessDrift(harness string, binaryVersion string) harnessDriftReading {
	configDir := harnessDriftConfigDir(harness)
	marker := readHarnessVersionMarker(configDir)
	return harnessDriftReading{
		target:    harness,
		name:      installDisplayName(harness),
		configDir: configDir,
		marker:    marker,
		state:     classifyHarnessDrift(marker, binaryVersion),
	}
}

// installedHarnessDriftReadings enumerates the harnesses install actually
// stamps markers into, keeping doctor's view and install's view on one table.
// Harnesses that are merely detected but carry no Loaf content are skipped:
// there is nothing to have drifted.
func installedHarnessDriftReadings(binaryVersion string) []harnessDriftReading {
	var readings []harnessDriftReading
	for _, tool := range detectInstallTools() {
		if !tool.installed {
			continue
		}
		marker := readHarnessVersionMarker(tool.configDir)
		readings = append(readings, harnessDriftReading{
			target:    tool.key,
			name:      tool.name,
			configDir: tool.configDir,
			marker:    marker,
			state:     classifyHarnessDrift(marker, binaryVersion),
		})
	}
	return readings
}

// doctorDetailLine renders one reported harness. Current harnesses report
// nothing, so an empty string means "no finding for this harness".
func (reading harnessDriftReading) doctorDetailLine(binaryVersion string) string {
	switch reading.state {
	case harnessDriftContentStale:
		return fmt.Sprintf("%s content is %s (%s) - run `loaf upgrade`", reading.name, reading.marker, reading.configDir)
	case harnessDriftBinaryStale:
		return fmt.Sprintf("%s content is %s, ahead of the binary's %s (%s) - the binary is the stale side; upgrade it (e.g. `brew upgrade loaf`)", reading.name, reading.marker, binaryVersion, reading.configDir)
	case harnessDriftUnknown:
		if reading.marker == "" {
			return fmt.Sprintf("%s content version is unknown - no %s in %s - run `loaf upgrade`", reading.name, loafInstallMarkerFile, reading.configDir)
		}
		return fmt.Sprintf("%s content version %q is unreadable (%s) - run `loaf upgrade`", reading.name, reading.marker, reading.configDir)
	default:
		return ""
	}
}

// harnessDriftNudge renders the single SessionStart line for the invoking
// harness. Only stale content earns a line: equal, missing, unparseable, and
// newer-than-binary markers all stay silent at session start, because session
// start is a nudge toward one command and not a diagnosis surface.
func (r Runner) harnessDriftNudge(harness string) string {
	binaryVersion := harnessDriftBinaryVersion(r)
	if binaryVersion == "" {
		return ""
	}
	reading := readHarnessDrift(harness, binaryVersion)
	if reading.state != harnessDriftContentStale {
		return ""
	}
	return fmt.Sprintf("Loaf content in this harness is %s; binary is %s — run loaf upgrade", reading.marker, binaryVersion)
}

func harnessDriftBinaryVersion(r Runner) string {
	root, err := r.resolveInstalledDistributionRoot()
	if err != nil {
		return ""
	}
	return packageVersion(root)
}

// compareHarnessDriftVersions adapts the shared semver comparison — the one the
// currency advisory reads GitHub release tags with — to the shape drift
// classification needs. The (int, bool) pair exists so a version that does not
// parse stays distinguishable from one that compares equal: the first is an
// unknown harness state, the second is a current harness, and collapsing them
// would report every unreadable marker as up to date.
func compareHarnessDriftVersions(left string, right string) (int, bool) {
	leftVersion, leftOK := parseUpgradeSemver(left)
	rightVersion, rightOK := parseUpgradeSemver(right)
	if !leftOK || !rightOK {
		return 0, false
	}
	return compareUpgradeSemver(leftVersion, rightVersion), true
}
