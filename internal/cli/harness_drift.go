package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Harness identities used by drift surfacing. The install targets reuse their
// install keys so the marker lookup stays on the same table install stamps
// with; Claude Code has no install key because its content ships through the
// plugin marketplace, so it only ever appears as a SessionStart dispatch
// variant and resolves to its own config home.
const (
	harnessDriftClaudeCode = "claude-code"
	harnessDriftCursor     = "cursor"
	harnessDriftCodex      = "codex"
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
	if harness == harnessDriftClaudeCode {
		return claudeCodeConfigHome()
	}
	return defaultInstallConfigDirs()[harness]
}

func claudeCodeConfigHome() string {
	if configured := strings.TrimRight(os.Getenv("CLAUDE_CONFIG_DIR"), string(filepath.Separator)); configured != "" {
		return configured
	}
	home := installHome()
	if home == "" {
		return ""
	}
	return filepath.Join(home, ".claude")
}

func readHarnessVersionMarker(configDir string) string {
	if configDir == "" {
		return ""
	}
	body, err := os.ReadFile(filepath.Join(configDir, loafInstallMarkerFile))
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

// compareHarnessDriftVersions orders two semver strings including prerelease
// precedence (1.0.0-alpha sorts before 1.0.0). ok is false when either side
// does not parse, which the callers treat as an unknown state rather than a
// comparison result.
func compareHarnessDriftVersions(left string, right string) (int, bool) {
	leftVersion, leftOK := parseHarnessDriftVersion(left)
	rightVersion, rightOK := parseHarnessDriftVersion(right)
	if !leftOK || !rightOK {
		return 0, false
	}
	if leftVersion.major != rightVersion.major {
		return compareHarnessDriftInts(leftVersion.major, rightVersion.major), true
	}
	if leftVersion.minor != rightVersion.minor {
		return compareHarnessDriftInts(leftVersion.minor, rightVersion.minor), true
	}
	if leftVersion.patch != rightVersion.patch {
		return compareHarnessDriftInts(leftVersion.patch, rightVersion.patch), true
	}
	return compareHarnessDriftPrerelease(leftVersion.prerelease, rightVersion.prerelease), true
}

func parseHarnessDriftVersion(value string) (releaseSemver, bool) {
	trimmed := strings.TrimPrefix(strings.TrimSpace(value), "v")
	if core, _, found := strings.Cut(trimmed, "+"); found {
		trimmed = core
	}
	if trimmed == "" {
		return releaseSemver{}, false
	}
	return parseReleaseSemver(trimmed)
}

// compareHarnessDriftPrerelease applies semver §11.4: a version without a
// prerelease outranks one with it, identifiers compare field by field with
// numeric fields ordered numerically and below alphanumeric ones, and a longer
// identifier list wins when every shared field is equal.
func compareHarnessDriftPrerelease(left string, right string) int {
	if left == right {
		return 0
	}
	if left == "" {
		return 1
	}
	if right == "" {
		return -1
	}
	leftFields := strings.Split(left, ".")
	rightFields := strings.Split(right, ".")
	for index := 0; index < len(leftFields) && index < len(rightFields); index++ {
		leftField, rightField := leftFields[index], rightFields[index]
		leftNumber, leftNumeric := harnessDriftNumericIdentifier(leftField)
		rightNumber, rightNumeric := harnessDriftNumericIdentifier(rightField)
		switch {
		case leftNumeric && rightNumeric:
			if leftNumber != rightNumber {
				return compareHarnessDriftInts(leftNumber, rightNumber)
			}
		case leftNumeric:
			return -1
		case rightNumeric:
			return 1
		default:
			if leftField != rightField {
				return compareHarnessDriftStrings(leftField, rightField)
			}
		}
	}
	return compareHarnessDriftInts(len(leftFields), len(rightFields))
}

func harnessDriftNumericIdentifier(field string) (int, bool) {
	if field == "" {
		return 0, false
	}
	value := 0
	for _, char := range field {
		if char < '0' || char > '9' {
			return 0, false
		}
		value = value*10 + int(char-'0')
	}
	return value, true
}

func compareHarnessDriftInts(left int, right int) int {
	switch {
	case left < right:
		return -1
	case left > right:
		return 1
	default:
		return 0
	}
}

func compareHarnessDriftStrings(left string, right string) int {
	switch {
	case left < right:
		return -1
	case left > right:
		return 1
	default:
		return 0
	}
}
