package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	goruntime "runtime"
	"strconv"
	"strings"
	"time"
)

// ghShimMinVersion is the lowest gh release this CLI will shim against. gh
// v2.40.0 (2023-12-07, PR cli/cli#8425 "Support multiple accounts on a
// single host") is the release that introduced `--user` on `gh auth token`,
// `gh auth switch`, and `gh auth status` — the named-account resolution the
// shim's token step depends on. Older gh binaries don't have the flag at
// all, so enable refuses rather than falling back to "whichever account is
// active" (a wrong-identity success is worse than a visible refusal).
const ghShimMinVersion = "2.40.0"

var ghVersionRE = regexp.MustCompile(`gh version (\d+)\.(\d+)\.(\d+)`)

func (r Runner) runShim(args []string, out io.Writer) error {
	if len(args) == 0 || isHelpArg(args) {
		writeShimHelp(out)
		return nil
	}
	switch args[0] {
	case "enable":
		return r.runShimEnable(args[1:], out)
	case "disable":
		return r.runShimDisable(args[1:], out)
	case "status":
		return r.runShimStatus(args[1:], out)
	default:
		return unknownSubcommandError("shim", args[0])
	}
}

func writeShimHelp(out io.Writer) {
	writeCommandGroupHelp(out, "loaf shim <subcommand> [options]",
		"Manage the opt-in per-invocation gh identity shim (see docs/changes/20260707-per-invocation-gh-identity).",
		[]subcommandHelpItem{
			{Name: "enable gh", Summary: "Symlink gh through loaf and record the real gh path (asks for consent)"},
			{Name: "disable gh", Summary: "Remove the symlink and forget the recorded gh path"},
			{Name: "status", Summary: "Report shim health: absent, healthy, broken-symlink, path-shadowed, real-gh-missing"},
		})
}

// shimTargetArgs parses `<name> [--yes] [--json] [--help]` — the shared shape
// of shim enable/disable/status. Not every subcommand uses every flag; each
// caller only reads the ones it needs.
type shimTargetArgs struct {
	name       string
	yes        bool
	jsonOutput bool
	help       bool
}

func parseShimTargetArgs(args []string) (shimTargetArgs, error) {
	var parsed shimTargetArgs
	for _, arg := range args {
		switch arg {
		case "--yes", "-y":
			parsed.yes = true
		case "--json":
			parsed.jsonOutput = true
		case "--help", "-h":
			parsed.help = true
		default:
			if strings.HasPrefix(arg, "-") {
				return shimTargetArgs{}, fmt.Errorf("unknown option %q", arg)
			}
			if parsed.name != "" {
				return shimTargetArgs{}, fmt.Errorf("unexpected argument %q", arg)
			}
			parsed.name = arg
		}
	}
	return parsed, nil
}

func requireGHShimTarget(name string) error {
	if name == "" {
		return fmt.Errorf(`missing shim name; only "gh" is supported`)
	}
	if name != "gh" {
		return fmt.Errorf(`unsupported shim %q; only "gh" is supported (no general shim framework — see change.md)`, name)
	}
	return nil
}

// --- enable ---

func (r Runner) runShimEnable(args []string, out io.Writer) error {
	parsed, err := parseShimTargetArgs(args)
	if err != nil {
		return err
	}
	if parsed.help {
		writeUsageHelp(out, "loaf shim enable gh [--yes]", "Symlink gh through loaf and record the real gh path. Prints the exact mutations and asks for confirmation.", "--yes  Confirm non-interactively (required when stdin isn't a terminal)")
		return nil
	}
	if err := requireGHShimTarget(parsed.name); err != nil {
		return err
	}
	if goruntime.GOOS == "windows" {
		return fmt.Errorf("gh shim is not supported on Windows; rely on the github-account hook (tier 1) instead")
	}
	return r.enableGHShim(out, parsed.yes)
}

func (r Runner) enableGHShim(out io.Writer, yes bool) error {
	// path-shadowed counts as "already enabled" too: right after a real
	// enable, the *current* shell session hasn't picked up the new PATH line
	// yet (that only takes effect on restart/source), so a second `enable`
	// in the same session would otherwise see path-shadowed, not healthy,
	// and needlessly re-run the whole consent flow.
	if diag, err := diagnoseGHShim(); err == nil && (diag.State == ghShimHealthy || diag.State == ghShimPathShadowed) {
		fmt.Fprintln(out, "gh shim is already enabled.")
		if diag.State == ghShimPathShadowed {
			fmt.Fprintf(out, "  %s\n", diag.Detail)
		}
		fmt.Fprintln(out, "Run `loaf shim status` for details, or `loaf shim disable gh` first to reconfigure.")
		return nil
	}

	symlinkPath, err := shimSymlinkPath()
	if err != nil {
		return err
	}
	shimDir := filepath.Dir(symlinkPath)

	realGH, err := findRealGHOnPATH(shimDir)
	if err != nil {
		return fmt.Errorf("gh not found on PATH; install the GitHub CLI first (https://cli.github.com)")
	}
	if resolved, everr := filepath.EvalSymlinks(realGH); everr == nil {
		realGH = resolved
	}

	version, verr := ghVersion(realGH)
	if verr != nil {
		return fmt.Errorf("could not determine gh version at %s: %w", realGH, verr)
	}
	if !ghVersionAtLeast(version, ghShimMinVersion) {
		return fmt.Errorf("gh %s at %s is older than the minimum %s required for named-account token resolution (`gh auth token --user`); upgrade gh and re-run `loaf shim enable gh`", version, realGH, ghShimMinVersion)
	}

	selfPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("could not determine the loaf binary path: %w", err)
	}
	if resolved, everr := filepath.EvalSymlinks(selfPath); everr == nil {
		selfPath = resolved
	}

	configPath, err := shimUserConfigPath()
	if err != nil {
		return err
	}
	profilePath, profileLine, hasProfile := shimProfileTarget(shimDir)

	printShimConsentPrompt(out, symlinkPath, selfPath, configPath, realGH, profilePath, profileLine, hasProfile)

	if !yes {
		if !readerIsTerminal(r.Stdin) {
			return fmt.Errorf("refusing to enable the gh shim non-interactively without --yes; review the mutations above and re-run with --yes")
		}
		reader := bufio.NewReader(firstReader(r.Stdin, os.Stdin))
		confirmed, err := askInstallYesNo(reader, out, "Proceed? [y/N] ", false)
		if err != nil {
			return err
		}
		if !confirmed {
			fmt.Fprintln(out, "Aborted; nothing changed.")
			return nil
		}
	}

	if err := os.MkdirAll(shimDir, 0o755); err != nil {
		return fmt.Errorf("could not create %s: %w", shimDir, err)
	}
	if pathExistsForDoctor(symlinkPath) {
		if err := os.Remove(symlinkPath); err != nil {
			return fmt.Errorf("could not replace existing %s: %w", symlinkPath, err)
		}
	}
	if err := os.Symlink(selfPath, symlinkPath); err != nil {
		return fmt.Errorf("could not create symlink: %w", err)
	}

	enabledAt := time.Now().UTC().Format(time.RFC3339)
	if err := writeShimUserConfig(configPath, realGH, enabledAt); err != nil {
		return err
	}

	fmt.Fprintf(out, "\n%s Symlink created: %s -> %s\n", ansiGreen("✓"), symlinkPath, selfPath)
	fmt.Fprintf(out, "%s Recorded in %s\n", ansiGreen("✓"), configPath)

	if hasProfile {
		offerProfileAppend(r, out, profilePath, profileLine, yes)
	} else {
		fmt.Fprintf(out, "\n%s Could not detect a shell profile to update; add this line yourself:\n    %s\n", ansiYellow("!"), profileLine)
	}

	fmt.Fprintln(out)
	fmt.Fprintln(out, "Restart your shell (or `source` your profile) for the new PATH to take effect.")
	fmt.Fprintln(out, "Check status any time with `loaf shim status` or `loaf doctor`.")
	return nil
}

func printShimConsentPrompt(out io.Writer, symlinkPath string, targetLoafBinary string, configPath string, realGH string, profilePath string, profileLine string, hasProfile bool) {
	fmt.Fprintln(out)
	fmt.Fprintln(out, ansiBold("loaf shim enable gh")+" will make these changes on this machine:")
	fmt.Fprintln(out)
	fmt.Fprintf(out, "  1. Create symlink:\n       %s -> %s\n\n", symlinkPath, targetLoafBinary)
	fmt.Fprintf(out, "  2. Record in %s:\n       shims.gh.real_path = %q\n       shims.gh.enabled_at = <now, UTC>\n\n", configPath, realGH)
	if hasProfile {
		fmt.Fprintf(out, "  3. Offer to add this line to your shell profile (%s):\n       %s\n\n", profilePath, profileLine)
	} else {
		fmt.Fprintf(out, "  3. Print this PATH line for you to add manually (no shell profile detected):\n       %s\n\n", profileLine)
	}
	fmt.Fprintln(out, "From then on, `gh` resolves to this symlink first. Inside a Loaf project with")
	fmt.Fprintln(out, "integrations.github.account configured, it fetches a per-invocation token for")
	fmt.Fprintln(out, "that named account from gh's own keychain and execs the real gh with it. Every-")
	fmt.Fprintln(out, "where else it execs the real gh untouched. gh's own config and active-account")
	fmt.Fprintln(out, "pointer (~/.config/gh/hosts.yml) are never written by the shim — only read.")
	fmt.Fprintln(out)
}

func offerProfileAppend(r Runner, out io.Writer, profilePath string, profileLine string, yes bool) {
	if yes {
		fmt.Fprintf(out, "%s Non-interactive; add this line to %s yourself:\n    %s\n", ansiYellow("!"), profilePath, profileLine)
		return
	}
	if !readerIsTerminal(r.Stdin) {
		fmt.Fprintf(out, "%s Add this line to %s yourself:\n    %s\n", ansiYellow("!"), profilePath, profileLine)
		return
	}
	reader := bufio.NewReader(firstReader(r.Stdin, os.Stdin))
	confirmed, err := askInstallYesNo(reader, out, fmt.Sprintf("Append PATH line to %s? [y/N] ", profilePath), false)
	if err != nil || !confirmed {
		fmt.Fprintf(out, "%s Skipped. Add this line yourself when ready:\n    %s\n", ansiYellow("!"), profileLine)
		return
	}
	if err := appendShimProfileLine(profilePath, profileLine); err != nil {
		fmt.Fprintf(out, "%s Could not update %s: %v\n    Add this line yourself:\n    %s\n", ansiRed("x"), profilePath, err, profileLine)
		return
	}
	fmt.Fprintf(out, "%s Appended to %s\n", ansiGreen("✓"), profilePath)
}

func appendShimProfileLine(profilePath string, profileLine string) error {
	if err := os.MkdirAll(filepath.Dir(profilePath), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(profilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = fmt.Fprintf(f, "\n# added by `loaf shim enable gh`\n%s\n", profileLine)
	return err
}

func shimProfileTarget(shimDir string) (profilePath string, profileLine string, ok bool) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Sprintf(`export PATH="%s:$PATH"`, shimDir), false
	}
	shell := os.Getenv("SHELL")
	switch {
	case strings.Contains(shell, "fish"):
		return filepath.Join(home, ".config", "fish", "config.fish"), fmt.Sprintf("set -gx PATH %s $PATH", shimDir), true
	case strings.Contains(shell, "zsh"):
		return filepath.Join(home, ".zshrc"), fmt.Sprintf(`export PATH="%s:$PATH"`, shimDir), true
	case strings.Contains(shell, "bash"):
		return filepath.Join(home, ".bash_profile"), fmt.Sprintf(`export PATH="%s:$PATH"`, shimDir), true
	default:
		return "", fmt.Sprintf(`export PATH="%s:$PATH"`, shimDir), false
	}
}

// --- disable ---

func (r Runner) runShimDisable(args []string, out io.Writer) error {
	parsed, err := parseShimTargetArgs(args)
	if err != nil {
		return err
	}
	if parsed.help {
		writeUsageHelp(out, "loaf shim disable gh", "Remove the gh shim symlink and forget the recorded real gh path.")
		return nil
	}
	if err := requireGHShimTarget(parsed.name); err != nil {
		return err
	}

	symlinkPath, err := shimSymlinkPath()
	if err != nil {
		return err
	}
	configPath, err := shimUserConfigPath()
	if err != nil {
		return err
	}

	removedSymlink := false
	if pathExistsForDoctor(symlinkPath) {
		if err := os.Remove(symlinkPath); err != nil {
			return fmt.Errorf("could not remove %s: %w", symlinkPath, err)
		}
		removedSymlink = true
	}
	removedConfig, err := removeShimUserConfigEntry(configPath)
	if err != nil {
		return err
	}

	if !removedSymlink && !removedConfig {
		fmt.Fprintln(out, "gh shim is not enabled; nothing to do.")
		return nil
	}
	if removedSymlink {
		fmt.Fprintf(out, "%s Removed symlink %s\n", ansiGreen("✓"), symlinkPath)
	}
	if removedConfig {
		fmt.Fprintf(out, "%s Removed shims.gh from %s\n", ansiGreen("✓"), configPath)
	}
	fmt.Fprintln(out)
	fmt.Fprintln(out, "The PATH line in your shell profile (if you added one) is left in place — it's")
	fmt.Fprintln(out, "harmless once the shims directory is empty, but you can remove it manually.")
	return nil
}

// --- status ---

func (r Runner) runShimStatus(args []string, out io.Writer) error {
	parsed, err := parseShimTargetArgs(args)
	if err != nil {
		return err
	}
	if parsed.help {
		writeUsageHelp(out, "loaf shim status [--json]", "Report gh shim health.", "--json  Output the diagnosis as JSON")
		return nil
	}
	diag, err := diagnoseGHShim()
	if err != nil {
		return err
	}
	if parsed.jsonOutput {
		if err := writeJSON(out, diag); err != nil {
			return err
		}
	} else {
		writeShimStatusText(out, diag)
	}
	if diag.State == ghShimBrokenSymlink || diag.State == ghShimRealGHMissing {
		return ExitError{Code: 1}
	}
	return nil
}

func writeShimStatusText(out io.Writer, diag ghShimDiagnosis) {
	glyphs := map[ghShimState]string{
		ghShimHealthy:       ansiGreen("✓"),
		ghShimAbsent:        ansiGray("-"),
		ghShimPathShadowed:  ansiYellow("⚠"),
		ghShimBrokenSymlink: ansiRed("✗"),
		ghShimRealGHMissing: ansiRed("✗"),
	}
	fmt.Fprintf(out, "\n%s gh shim: %s\n", glyphs[diag.State], diag.State)
	fmt.Fprintf(out, "  %s\n", diag.Detail)
	if diag.RealGHPath != "" {
		fmt.Fprintf(out, "  real gh: %s\n", diag.RealGHPath)
	}
	if diag.EnabledAt != "" {
		fmt.Fprintf(out, "  enabled: %s\n", diag.EnabledAt)
	}
	fmt.Fprintln(out)
}

// --- diagnosis (shared by `loaf shim status` and the `loaf doctor` check) ---

type ghShimState string

const (
	ghShimAbsent        ghShimState = "absent"
	ghShimHealthy       ghShimState = "healthy"
	ghShimBrokenSymlink ghShimState = "broken-symlink"
	ghShimPathShadowed  ghShimState = "path-shadowed"
	ghShimRealGHMissing ghShimState = "real-gh-missing"
)

type ghShimDiagnosis struct {
	State       ghShimState `json:"state"`
	SymlinkPath string      `json:"symlink_path"`
	ConfigPath  string      `json:"config_path"`
	RealGHPath  string      `json:"real_gh_path,omitempty"`
	EnabledAt   string      `json:"enabled_at,omitempty"`
	Detail      string      `json:"detail"`
}

func diagnoseGHShim() (ghShimDiagnosis, error) {
	symlinkPath, err := shimSymlinkPath()
	if err != nil {
		return ghShimDiagnosis{}, err
	}
	configPath, err := shimUserConfigPath()
	if err != nil {
		return ghShimDiagnosis{}, err
	}

	diag := ghShimDiagnosis{SymlinkPath: symlinkPath, ConfigPath: configPath}

	cfg, hasEntry, err := readShimUserConfig()
	if err != nil {
		return ghShimDiagnosis{}, err
	}
	hasEntry = hasEntry && cfg.Shims.GH != nil
	symlinkExists := pathExistsForDoctor(symlinkPath)

	if !hasEntry && !symlinkExists {
		diag.State = ghShimAbsent
		diag.Detail = "gh shim is not enabled; run `loaf shim enable gh` to turn it on"
		return diag, nil
	}
	if hasEntry {
		diag.RealGHPath = cfg.Shims.GH.RealPath
		diag.EnabledAt = cfg.Shims.GH.EnabledAt
	}

	if !symlinkExists {
		diag.State = ghShimBrokenSymlink
		diag.Detail = fmt.Sprintf("config records a gh shim but %s is missing; run `loaf shim enable gh` again", symlinkPath)
		return diag, nil
	}
	if !isSymlinkForDoctor(symlinkPath) {
		diag.State = ghShimBrokenSymlink
		diag.Detail = fmt.Sprintf("%s exists but is not a symlink", symlinkPath)
		return diag, nil
	}
	target := resolveSymlinkForDoctor(symlinkPath)
	if target == "" || !pathExistsForDoctor(target) {
		diag.State = ghShimBrokenSymlink
		diag.Detail = fmt.Sprintf("%s points at a target that no longer exists", symlinkPath)
		return diag, nil
	}
	if !hasEntry {
		diag.State = ghShimBrokenSymlink
		diag.Detail = fmt.Sprintf("%s exists but no shims.gh entry is recorded in %s; run `loaf shim enable gh` again", symlinkPath, configPath)
		return diag, nil
	}
	if !pathIsExecutableFile(cfg.Shims.GH.RealPath) {
		diag.State = ghShimRealGHMissing
		diag.Detail = fmt.Sprintf("recorded real gh %q no longer exists; run `loaf shim enable gh` again", cfg.Shims.GH.RealPath)
		return diag, nil
	}

	resolved, perr := findRealGHOnPATH("")
	if perr != nil || !isSameExecutable(resolved, target) {
		diag.State = ghShimPathShadowed
		diag.Detail = fmt.Sprintf("PATH does not resolve gh to the shim yet; put %s ahead of the real gh's directory on PATH", filepath.Dir(symlinkPath))
		return diag, nil
	}

	diag.State = ghShimHealthy
	diag.Detail = fmt.Sprintf("gh shim healthy: %s -> %s -> %s", symlinkPath, target, cfg.Shims.GH.RealPath)
	return diag, nil
}

// --- gh version gate ---

func ghVersion(realGH string) (string, error) {
	output, err := exec.Command(realGH, "--version").Output()
	if err != nil {
		return "", err
	}
	match := ghVersionRE.FindStringSubmatch(string(output))
	if match == nil {
		return "", fmt.Errorf("could not parse gh --version output")
	}
	return fmt.Sprintf("%s.%s.%s", match[1], match[2], match[3]), nil
}

func ghVersionAtLeast(version string, minVersion string) bool {
	v, verr := parseGHVersionTriplet(version)
	m, merr := parseGHVersionTriplet(minVersion)
	if verr != nil || merr != nil {
		return false
	}
	for i := 0; i < 3; i++ {
		if v[i] != m[i] {
			return v[i] > m[i]
		}
	}
	return true
}

func parseGHVersionTriplet(version string) ([3]int, error) {
	parts := strings.Split(version, ".")
	if len(parts) != 3 {
		return [3]int{}, fmt.Errorf("invalid version %q", version)
	}
	var out [3]int
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return [3]int{}, err
		}
		out[i] = n
	}
	return out, nil
}

// --- XDG paths ---

// shimUserConfigPath is the user-level (never repo-level) config that
// records shim consent: $XDG_CONFIG_HOME/loaf/config.json, falling back to
// ~/.config/loaf/config.json. This is deliberately separate from the
// project's .agents/loaf.json — the trust boundary in change.md's Decision 1
// keeps policy (which account) in the repo and mechanism (whether a shim
// enforces it) on the machine.
func shimUserConfigPath() (string, error) {
	if v := os.Getenv("XDG_CONFIG_HOME"); v != "" && filepath.IsAbs(v) {
		return filepath.Join(v, "loaf", "config.json"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve config home: %w", err)
	}
	return filepath.Join(home, ".config", "loaf", "config.json"), nil
}

func shimDataHome() (string, error) {
	if v := os.Getenv("XDG_DATA_HOME"); v != "" && filepath.IsAbs(v) {
		return v, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve data home: %w", err)
	}
	return filepath.Join(home, ".local", "share"), nil
}

func shimSymlinkPath() (string, error) {
	dataHome, err := shimDataHome()
	if err != nil {
		return "", err
	}
	return filepath.Join(dataHome, "loaf", "shims", "gh"), nil
}

// shimSymlinkDir returns the shim symlink's parent directory, or "" if it
// can't be resolved (e.g. HOME unset) — callers treat "" as "exclude
// nothing" rather than failing, since this is only used to keep a PATH walk
// from re-selecting the shim.
func shimSymlinkDir() (string, error) {
	path, err := shimSymlinkPath()
	if err != nil {
		return "", err
	}
	return filepath.Dir(path), nil
}

// --- user config read/write ---

type shimGHRecord struct {
	RealPath  string `json:"real_path"`
	EnabledAt string `json:"enabled_at"`
}

type shimShimsSection struct {
	GH *shimGHRecord `json:"gh,omitempty"`
}

type shimConfigSnapshot struct {
	Shims shimShimsSection `json:"shims"`
}

// readShimUserConfig returns the shims.gh record, if any. The bool result is
// true when the config file exists and parses, regardless of whether a gh
// entry is present.
func readShimUserConfig() (shimConfigSnapshot, bool, error) {
	path, err := shimUserConfigPath()
	if err != nil {
		return shimConfigSnapshot{}, false, err
	}
	root, existed, err := readShimUserConfigRaw(path)
	if err != nil {
		return shimConfigSnapshot{}, false, err
	}
	if !existed {
		return shimConfigSnapshot{}, false, nil
	}
	shimsRaw, _ := root["shims"].(map[string]any)
	ghRaw, _ := shimsRaw["gh"].(map[string]any)
	if ghRaw == nil {
		return shimConfigSnapshot{}, true, nil
	}
	return shimConfigSnapshot{Shims: shimShimsSection{GH: &shimGHRecord{
		RealPath:  stringFromAny(ghRaw["real_path"]),
		EnabledAt: stringFromAny(ghRaw["enabled_at"]),
	}}}, true, nil
}

func writeShimUserConfig(path string, realPath string, enabledAt string) error {
	root, _, err := readShimUserConfigRaw(path)
	if err != nil {
		return err
	}
	shims, ok := root["shims"].(map[string]any)
	if !ok {
		shims = map[string]any{}
		root["shims"] = shims
	}
	shims["gh"] = map[string]any{
		"real_path":  realPath,
		"enabled_at": enabledAt,
	}
	return writeShimUserConfigRaw(path, root)
}

// removeShimUserConfigEntry deletes shims.gh, dropping the now-empty shims
// object too. It reports whether anything was actually removed so `disable`
// can tell "was enabled" from "nothing to do" apart.
func removeShimUserConfigEntry(path string) (bool, error) {
	root, existed, err := readShimUserConfigRaw(path)
	if err != nil {
		return false, err
	}
	if !existed {
		return false, nil
	}
	shims, ok := root["shims"].(map[string]any)
	if !ok {
		return false, nil
	}
	if _, has := shims["gh"]; !has {
		return false, nil
	}
	delete(shims, "gh")
	if len(shims) == 0 {
		delete(root, "shims")
	}
	if err := writeShimUserConfigRaw(path, root); err != nil {
		return false, err
	}
	return true, nil
}

func readShimUserConfigRaw(path string) (map[string]any, bool, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]any{}, false, nil
		}
		return nil, false, fmt.Errorf("could not read %s: %w", path, err)
	}
	var root map[string]any
	if err := json.Unmarshal(body, &root); err != nil {
		return nil, false, fmt.Errorf("could not parse %s: %w", path, err)
	}
	if root == nil {
		root = map[string]any{}
	}
	return root, true, nil
}

func writeShimUserConfigRaw(path string, root map[string]any) error {
	body, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, append(body, '\n'), 0o600)
}

func stringFromAny(v any) string {
	s, _ := v.(string)
	return s
}
