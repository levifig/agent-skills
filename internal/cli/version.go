package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	stdlibRuntime "runtime"
	"strings"
	"time"
)

type builtTarget struct {
	name string
	path string
}

var versionTargetOutputs = []builtTarget{
	{name: "claude-code", path: "plugins/loaf/"},
	{name: "cursor", path: "dist/cursor/"},
	{name: "opencode", path: "dist/opencode/"},
	{name: "codex", path: "dist/codex/"},
}

func (r Runner) runVersion(out io.Writer) error {
	root, rootErr := r.resolveInstalledDistributionRoot()
	if rootErr != nil {
		root = ""
	}
	fmt.Fprintf(out, "\n%s %s%s\n", ansiBold("loaf"), r.reportedVersion(root), r.versionSuffix())
	fmt.Fprintf(out, "%s %s\n", ansiGray("go"), strings.TrimPrefix(runtimeVersion(), "go"))

	// Targets and Content describe the installed distribution. Without
	// resolvable executable provenance there is no distribution to describe —
	// an empty root would make the lookups below relative to the working
	// directory, counting whatever checkout the caller happens to stand in —
	// so the degraded output ends at identity plus the resolver's guidance.
	if rootErr != nil {
		fmt.Fprintf(out, "\n%s\n\n", ansiGray(rootErr.Error()))
		return nil
	}

	targets := builtTargets(root)
	if len(targets) > 0 {
		fmt.Fprintf(out, "\n%s\n", ansiBold("Targets:"))
		maxName := 0
		for _, target := range targets {
			if len(target.name) > maxName {
				maxName = len(target.name)
			}
		}
		for _, target := range targets {
			fmt.Fprintf(out, "  %-*s%s\n", maxName+2, target.name, ansiGray(target.path))
		}
	}

	fmt.Fprintf(out, "\n%s\n", ansiBold("Content:"))
	fmt.Fprintf(out, "  Skills:  %d\n", countSkillDirs(root))
	fmt.Fprintf(out, "  Agents:  %d\n", countAgentFiles(root))
	fmt.Fprintf(out, "  Hooks:   %d\n\n", countHookEntries(root))
	return nil
}

// devVersionPatchFloor is the smallest patch number that can only be a Unix
// timestamp. The patch slot counts releases within a minor and will never
// approach a billion, so a patch at or above this floor identifies a dev build
// and nothing else.
const devVersionPatchFloor = 1_000_000_000

// isDevVersion is the shared dev-identity predicate. One rule — a patch of
// timestamp magnitude — serves every surface that has to tell a dev build from
// a release: the version report mints these, the drift classifier reads them
// off install markers, and the release pipeline refuses to run its ceremony for
// one. There is no flag, suffix, or second source to keep in step.
func isDevVersion(version string) bool {
	parsed, ok := parseUpgradeSemver(version)
	if !ok {
		return false
	}
	return parsed.patch >= devVersionPatchFloor
}

// devVersion mints a dev build's identity: the distribution's major and minor
// with the moment the binary was linked in the patch slot. A timestamp patch is
// valid SemVer and sorts above every release in the minor, which is the truth
// about a machine running its own build — a prerelease suffix would sort below
// the latest release instead, and nag that machine to "upgrade" forever.
//
// The stamp is not linked into the binary. The committed native binaries are
// asserted byte-for-byte reproducible (cli/scripts/verify-go-artifacts.mjs), so
// a build-varying ldflag would fail that assertion on every build; the linker's
// own output timestamp carries it instead (see cmd/loaf/main.go).
//
// A clock that has not been set would mint a patch below the floor — a version
// no surface could tell from a release — so that falls back to the release
// version rather than claim to be a build it cannot name. So does a binary with
// no resolvable distribution: its version is unknown, and dressing the unknown
// sentinel in a timestamp would claim an identity it does not have.
func devVersion(releaseVersion string, buildTime time.Time) string {
	parsed, ok := parseUpgradeSemver(releaseVersion)
	if !ok || buildTime.IsZero() || releaseVersion == packageVersionUnknown {
		return releaseVersion
	}
	stamp := buildTime.Unix()
	if stamp < devVersionPatchFloor {
		return releaseVersion
	}
	return fmt.Sprintf("%d.%d.%d", parsed.major, parsed.minor, stamp)
}

// reportedVersion is the version this binary answers with: a release build
// reports the distribution's, a dev build reports its own identity. Only the
// version report reads it. Everything describing installed *content* — install
// markers, doctor's CLI version, change receipts — stays on packageVersion,
// because content always carries the release version whichever binary deployed
// it.
func (r Runner) reportedVersion(root string) string {
	return devVersion(packageVersion(root), r.DevBuildTime)
}

// versionSuffix annotates the version line. A release build carries its commit
// and date; a dev build says what it is, because a ten-digit patch only reads
// as a timestamp once you know to read it as one.
func (r Runner) versionSuffix() string {
	if !r.DevBuildTime.IsZero() {
		return " (dev build)"
	}
	return buildInfoSuffix(r.BuildCommit, r.BuildDate)
}

// buildInfoSuffix renders optional build metadata for the version line. The
// semver identifier is build-independent, so commit/date are appended as a
// parenthetical suffix only when supplied (release builds). It returns an empty
// string when neither is set, preserving the clean `loaf <version>` output.
func buildInfoSuffix(commit, date string) string {
	commit = strings.TrimSpace(commit)
	date = strings.TrimSpace(date)
	var parts []string
	if date != "" {
		parts = append(parts, "built "+date)
	}
	if commit != "" {
		parts = append(parts, "git "+commit)
	}
	if len(parts) == 0 {
		return ""
	}
	return " (" + strings.Join(parts, " · ") + ")"
}

func findLoafPackageRoot(path string, seen map[string]bool) (string, bool) {
	if path == "" {
		return "", false
	}
	clean, err := filepath.Abs(path)
	if err != nil {
		return "", false
	}
	for {
		if !seen[clean] {
			seen[clean] = true
			if packageName(clean) == "loaf" {
				return clean, true
			}
		}
		parent := filepath.Dir(clean)
		if parent == clean {
			return "", false
		}
		clean = parent
	}
}

func packageName(root string) string {
	var pkg struct {
		Name string `json:"name"`
	}
	if err := readPackageJSON(root, &pkg); err != nil {
		return ""
	}
	return pkg.Name
}

// packageVersionUnknown is what packageVersion answers when there is no
// readable distribution package.json: a version that says so rather than one
// that guesses.
const packageVersionUnknown = "0.0.0"

func packageVersion(root string) string {
	var pkg struct {
		Version string `json:"version"`
	}
	if err := readPackageJSON(root, &pkg); err != nil || pkg.Version == "" {
		return packageVersionUnknown
	}
	return pkg.Version
}

// readPackageJSON reads a package.json that may belong to anyone: the Loaf
// distribution, the project being verified, or an ancestor directory the root
// search walked through on its way up. The last of those is why it reads
// through the descriptor-hardened open — the search visits directories chosen
// by where the operator happened to be standing.
func readPackageJSON(root string, target any) error {
	if root == "" {
		return fmt.Errorf("missing root")
	}
	body, err := readRegularFile(filepath.Join(root, "package.json"), projectFileReadLimit)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, target)
}

func builtTargets(root string) []builtTarget {
	var targets []builtTarget
	for _, target := range versionTargetOutputs {
		if isDir(filepath.Join(root, filepath.FromSlash(target.path))) {
			targets = append(targets, target)
		}
	}
	return targets
}

func countSkillDirs(root string) int {
	entries, err := os.ReadDir(filepath.Join(root, "content", "skills"))
	if err != nil {
		return 0
	}
	count := 0
	for _, entry := range entries {
		if entry.IsDir() {
			count++
		}
	}
	return count
}

func countAgentFiles(root string) int {
	entries, err := os.ReadDir(filepath.Join(root, "content", "agents"))
	if err != nil {
		return 0
	}
	count := 0
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".md") {
			count++
		}
	}
	return count
}

func countHookEntries(root string) int {
	body, err := os.ReadFile(filepath.Join(root, "config", "hooks.yaml"))
	if err != nil {
		return 0
	}
	allowedSections := map[string]bool{
		"pre-tool":  true,
		"post-tool": true,
		"session":   true,
	}
	inHooks := false
	activeSection := ""
	count := 0
	for _, line := range strings.Split(string(body), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if !strings.HasPrefix(line, " ") && strings.HasSuffix(trimmed, ":") {
			inHooks = trimmed == "hooks:"
			activeSection = ""
			continue
		}
		if !inHooks {
			continue
		}
		if strings.HasPrefix(line, "  ") && !strings.HasPrefix(line, "    ") && strings.HasSuffix(trimmed, ":") {
			section := strings.TrimSuffix(trimmed, ":")
			if allowedSections[section] {
				activeSection = section
			} else {
				activeSection = ""
			}
			continue
		}
		if activeSection != "" && strings.HasPrefix(strings.TrimLeft(line, " "), "- ") {
			count++
		}
	}
	return count
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func ansiBold(value string) string {
	return "\x1b[1m" + value + "\x1b[0m"
}

func ansiGray(value string) string {
	return "\x1b[90m" + value + "\x1b[0m"
}

func runtimeVersion() string {
	return strings.TrimPrefix(stdlibRuntime.Version(), "go")
}
