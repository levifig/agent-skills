package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	stdlibRuntime "runtime"
	"strings"
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
	fmt.Fprintf(out, "\n%s %s%s\n", ansiBold("loaf"), r.reportedVersion(root), r.versionSuffix(root))
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

// legacyDevVersionPatchFloor preserves recognition of the timestamp identities
// minted before dev builds became commit-addressed. Installed harness markers
// can outlive the binary that wrote them, so removing this rule would misread
// existing dev content as a newer release.
const legacyDevVersionPatchFloor = 1_000_000_000

// isDevVersion recognizes the current +g<short-sha> convention (optionally
// followed by .dirty) and the legacy timestamp-in-patch convention. Build metadata does not affect SemVer
// precedence, but it remains an explicit and machine-readable channel marker.
func isDevVersion(version string) bool {
	parsed, ok := parseUpgradeSemver(version)
	if !ok {
		return false
	}
	if parsed.patch >= legacyDevVersionPatchFloor {
		return true
	}
	value := normalizeUpgradeVersion(version)
	_, build, found := strings.Cut(value, "+")
	if !found {
		return false
	}
	for _, identifier := range strings.Split(build, ".") {
		if isDevCommitIdentifier(identifier) {
			return true
		}
	}
	return false
}

func isDevCommitIdentifier(identifier string) bool {
	if len(identifier) < 8 || len(identifier) > 41 || identifier[0] != 'g' {
		return false
	}
	for _, char := range identifier[1:] {
		if char < '0' || char > '9' {
			if char < 'a' || char > 'f' {
				return false
			}
		}
	}
	return true
}

// devVersion appends the commit a local build was compiled from as SemVer
// build metadata, followed by `.dirty` when the working tree carried
// uncommitted changes at compile time. This keeps the package version and its
// precedence intact while making the exact source commit immediately
// comparable with git. Both facts come from the VCS stamp `go build
// -buildvcs=true` writes into the binary (see cmd/loaf/main.go), so the
// identity describes the compiled bytes rather than whatever HEAD said when a
// separate provenance file was written.
func devVersion(releaseVersion string, commit string, modified bool) string {
	if _, ok := parseUpgradeSemver(releaseVersion); !ok || releaseVersion == packageVersionUnknown {
		return releaseVersion
	}
	commit = strings.ToLower(strings.TrimSpace(commit))
	if !isDevCommitIdentifier("g" + commit) {
		return releaseVersion
	}
	commit = commit[:7]
	separator := "+"
	if strings.Contains(releaseVersion, "+") {
		separator = "."
	}
	identity := "g" + commit
	if modified {
		identity += ".dirty"
	}
	return releaseVersion + separator + identity
}

// reportedVersion is the version this binary answers with: a release build
// reports the distribution's, a dev build reports its own identity. Only the
// version report reads it. Everything describing installed *content* — install
// markers, doctor's CLI version, change receipts — stays on packageVersion,
// because content always carries the release version whichever binary deployed
// it.
func (r Runner) reportedVersion(root string) string {
	if !r.isDevBuild(root) {
		return packageVersion(root)
	}
	return devVersion(packageVersion(root), r.DevBuildCommit, r.DevBuildModified)
}

// isDevBuild takes two facts rather than one. Absent release metadata says no
// release pipeline built this binary; a resolved distribution that is the
// source checkout says it is running out of the tree that did.
//
// Absence alone would be wrong, because Loaf ships binaries that never pass
// through the release workflow: `npx github:levifig/loaf` compiles one at
// install time, and any prebuilt binary copied into a distribution layout
// carries no metadata. Reading that absence as proof would tell a user on
// 0.2.20 they were running a dev build. (The Claude Code plugin ships no
// binary at all since 2026-09-06; its shim runs whichever loaf is installed.)
func (r Runner) isDevBuild(root string) bool {
	return isSourceCheckout(root) && strings.TrimSpace(r.BuildCommit) == "" && strings.TrimSpace(r.BuildDate) == ""
}

// isSourceCheckout reports whether a resolved distribution root is the Loaf
// checkout that builds this binary. Every shipped distribution — release
// archive, Homebrew keg, npm package — is content plus a
// prebuilt binary; only a checkout carries the Go module beside them. An
// unresolved root is never a checkout: probing it would read go.mod relative to
// wherever the caller happened to be standing.
func isSourceCheckout(root string) bool {
	if root == "" {
		return false
	}
	_, err := os.Stat(filepath.Join(root, "go.mod"))
	return err == nil
}

// versionSuffix annotates the version line. Release builds carry their commit
// and date; source-checkout builds name their channel explicitly.
func (r Runner) versionSuffix(root string) string {
	if r.isDevBuild(root) {
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
