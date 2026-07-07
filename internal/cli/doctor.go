package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type doctorStatus string

const (
	doctorPass doctorStatus = "pass"
	doctorWarn doctorStatus = "warn"
	doctorFail doctorStatus = "fail"
	doctorSkip doctorStatus = "skip"
)

type doctorResult struct {
	Status  doctorStatus
	Message string
	Detail  string
	Fixable bool
}

type doctorFixResult struct {
	Fixed   bool
	Message string
}

type doctorContext struct {
	projectRoot string
}

type doctorCheck struct {
	Name        string
	Description string
	Run         func(doctorContext) doctorResult
	Fix         func(doctorContext, doctorResult) doctorFixResult
}

type doctorOptions struct {
	fix     bool
	verbose bool
	help    bool
}

type doctorReport struct {
	Passes       int
	Warnings     int
	Failures     int
	Skips        int
	FixesApplied int
	FixesFailed  int
}

func (r Runner) runDoctor(args []string, out io.Writer, runtimeRoot string) error {
	options, err := parseLoafDoctorArgs(args)
	if err != nil {
		return err
	}
	if options.help {
		writeDoctorHelp(out)
		return nil
	}
	report := runDoctorChecks(out, doctorContext{projectRoot: runtimeRoot}, options, packageVersion(runtimeRoot))
	if report.Failures > 0 {
		return ExitError{Code: 1}
	}
	return nil
}

func parseLoafDoctorArgs(args []string) (doctorOptions, error) {
	var options doctorOptions
	for _, arg := range args {
		switch arg {
		case "--fix":
			options.fix = true
		case "--verbose":
			options.verbose = true
		case "--help", "-h":
			options.help = true
		default:
			return options, fmt.Errorf("unknown doctor option %q", arg)
		}
	}
	return options, nil
}

func writeDoctorHelp(out io.Writer) {
	fmt.Fprint(out, "Usage: loaf doctor [--fix] [--verbose]\n\n")
	fmt.Fprint(out, "Diagnose Loaf project alignment (symlinks, stale files, version drift)\n\n")
	fmt.Fprint(out, "Options:\n")
	fmt.Fprint(out, "  --fix       Apply safe auto-fixes for failing checks\n")
	fmt.Fprint(out, "  --verbose   Print each check name even when passing\n")
	fmt.Fprint(out, "  -h, --help  Show help\n")
}

func runDoctorChecks(out io.Writer, ctx doctorContext, options doctorOptions, cliVersion string) doctorReport {
	report := doctorReport{}
	fmt.Fprintf(out, "\n%s\n\n", ansiBold("loaf doctor"))

	for _, check := range doctorChecks(cliVersion) {
		result := safeRunDoctorCheck(check, ctx)
		printDoctorCheckLine(out, check, result, options)

		if options.fix && result.Status == doctorFail && result.Fixable && check.Fix != nil {
			fix := safeFixDoctorCheck(check, ctx, result)
			printDoctorFixLine(out, fix)
			if fix.Fixed {
				report.FixesApplied++
				recheck := safeRunDoctorCheck(check, ctx)
				tallyDoctorStatus(recheck.Status, &report)
				continue
			}
			report.FixesFailed++
		}

		tallyDoctorStatus(result.Status, &report)
	}

	printDoctorSummary(out, report)
	return report
}

func doctorChecks(cliVersion string) []doctorCheck {
	return []doctorCheck{
		checkCanonicalAgentsFile(),
		checkAgentsSymlink(),
		checkClaudeSymlink(),
		checkStaleCursorMdc(),
		checkFencedVersion(cliVersion),
		checkDuplicateFencedSections(),
		checkGHShim(),
	}
}

func checkGHShim() doctorCheck {
	return doctorCheck{
		Name:        "gh-shim",
		Description: "gh shim (if enabled) is symlinked, resolvable first on PATH, and points at a live real gh",
		Run: func(_ doctorContext) doctorResult {
			diag, err := diagnoseGHShim()
			if err != nil {
				return doctorResult{Status: doctorFail, Message: fmt.Sprintf("could not diagnose gh shim: %v", err)}
			}
			switch diag.State {
			case ghShimAbsent:
				return doctorResult{Status: doctorSkip, Message: "gh shim not enabled"}
			case ghShimHealthy:
				return doctorResult{Status: doctorPass, Message: diag.Detail}
			case ghShimPathShadowed:
				return doctorResult{Status: doctorWarn, Message: "gh shim configured but not first on PATH", Detail: diag.Detail}
			case ghShimBrokenSymlink:
				return doctorResult{Status: doctorFail, Message: "gh shim symlink is broken", Detail: diag.Detail, Fixable: true}
			case ghShimRealGHMissing:
				return doctorResult{Status: doctorFail, Message: "gh shim's recorded real gh is missing", Detail: diag.Detail}
			default:
				return doctorResult{Status: doctorFail, Message: fmt.Sprintf("unknown gh shim state %q", diag.State)}
			}
		},
		Fix: func(_ doctorContext, _ doctorResult) doctorFixResult {
			return fixGHShimSymlink()
		},
	}
}

// fixGHShimSymlink repairs a broken symlink from already-recorded, already-
// consented config. It never (re)writes shims.gh itself — that would be a
// fresh enable, which needs its own consent prompt, not a --fix side effect.
func fixGHShimSymlink() doctorFixResult {
	diag, err := diagnoseGHShim()
	if err != nil {
		return doctorFixResult{Fixed: false, Message: fmt.Sprintf("could not re-diagnose: %v", err)}
	}
	if diag.State != ghShimBrokenSymlink {
		return doctorFixResult{Fixed: false, Message: "state no longer matches - re-run doctor"}
	}
	if diag.RealGHPath == "" {
		return doctorFixResult{Fixed: false, Message: "no recorded gh shim configuration to repair from; run `loaf shim enable gh`"}
	}
	self, err := os.Executable()
	if err != nil {
		return doctorFixResult{Fixed: false, Message: fmt.Sprintf("could not determine loaf binary path: %v", err)}
	}
	if resolved, everr := filepath.EvalSymlinks(self); everr == nil {
		self = resolved
	}
	if err := os.MkdirAll(filepath.Dir(diag.SymlinkPath), 0o755); err != nil {
		return doctorFixResult{Fixed: false, Message: fmt.Sprintf("could not prepare shim directory: %v", err)}
	}
	if pathExistsForDoctor(diag.SymlinkPath) {
		if err := os.Remove(diag.SymlinkPath); err != nil {
			return doctorFixResult{Fixed: false, Message: fmt.Sprintf("could not remove stale symlink: %v", err)}
		}
	}
	if err := os.Symlink(self, diag.SymlinkPath); err != nil {
		return doctorFixResult{Fixed: false, Message: fmt.Sprintf("symlink failed: %v", err)}
	}
	return doctorFixResult{Fixed: true, Message: fmt.Sprintf("Recreated %s -> %s", diag.SymlinkPath, self)}
}

func checkAgentsSymlink() doctorCheck {
	return doctorCheck{
		Name:        "agents-symlink",
		Description: "./AGENTS.md is a symlink to .agents/AGENTS.md",
		Run: func(ctx doctorContext) doctorResult {
			linkPath := filepath.Join(ctx.projectRoot, "AGENTS.md")
			canonical := filepath.Join(ctx.projectRoot, ".agents", "AGENTS.md")

			if !doctorFileExists(canonical) {
				if pathExistsForDoctor(linkPath) && !isSymlinkForDoctor(linkPath) {
					return doctorResult{
						Status:  doctorFail,
						Message: "Legacy layout - ./AGENTS.md exists as a real file, canonical .agents/AGENTS.md missing",
						Detail:  fmt.Sprintf("%s has content but .agents/AGENTS.md doesn't exist yet. Run `loaf doctor --fix` to migrate content into canonical, back up as ./AGENTS.md.bak, and replace with a symlink.", linkPath),
						Fixable: true,
					}
				}
				return doctorResult{Status: doctorSkip, Message: "No .agents/AGENTS.md to link to"}
			}
			if !pathExistsForDoctor(linkPath) {
				return doctorResult{
					Status:  doctorFail,
					Message: "./AGENTS.md is missing",
					Detail:  fmt.Sprintf("Expected symlink at %s -> .agents/AGENTS.md", linkPath),
					Fixable: true,
				}
			}
			if !isSymlinkForDoctor(linkPath) {
				return doctorResult{
					Status:  doctorFail,
					Message: "./AGENTS.md exists but is not a symlink",
					Detail:  "Duplicate content risks drift from .agents/AGENTS.md. Run `loaf doctor --fix` to merge its content into canonical, back up as ./AGENTS.md.bak, and replace with a symlink.",
					Fixable: true,
				}
			}
			if !symlinkPointsToForDoctor(linkPath, canonical) {
				actual := resolveSymlinkForDoctor(linkPath)
				if actual == "" {
					actual = "<unreadable>"
				}
				return doctorResult{
					Status:  doctorFail,
					Message: "./AGENTS.md points to the wrong target",
					Detail:  fmt.Sprintf("Got: %s\nWant: %s", actual, canonical),
					Fixable: true,
				}
			}
			return doctorResult{Status: doctorPass, Message: "./AGENTS.md -> .agents/AGENTS.md"}
		},
		Fix: func(ctx doctorContext, _ doctorResult) doctorFixResult {
			linkPath := filepath.Join(ctx.projectRoot, "AGENTS.md")
			canonical := filepath.Join(ctx.projectRoot, ".agents", "AGENTS.md")
			if !doctorFileExists(canonical) {
				if err := os.MkdirAll(filepath.Dir(canonical), 0o755); err != nil {
					return doctorFixResult{Fixed: false, Message: fmt.Sprintf("Could not create .agents/AGENTS.md: %v", err)}
				}
				if err := os.WriteFile(canonical, []byte{}, 0o644); err != nil {
					return doctorFixResult{Fixed: false, Message: fmt.Sprintf("Could not create .agents/AGENTS.md: %v", err)}
				}
			}
			if pathExistsForDoctor(linkPath) && !isSymlinkForDoctor(linkPath) {
				return migrateRealFileToDoctorSymlink(linkPath, canonical, ctx.projectRoot)
			}
			if isSymlinkForDoctor(linkPath) {
				if err := os.Remove(linkPath); err != nil {
					return doctorFixResult{Fixed: false, Message: fmt.Sprintf("Symlink failed: %v", err)}
				}
			}
			return createDoctorSymlink(linkPath, canonical, "Created ./AGENTS.md")
		},
	}
}

func checkClaudeSymlink() doctorCheck {
	return doctorCheck{
		Name:        "claude-symlink",
		Description: ".claude/CLAUDE.md is a symlink to .agents/AGENTS.md",
		Run: func(ctx doctorContext) doctorResult {
			linkPath := filepath.Join(ctx.projectRoot, ".claude", "CLAUDE.md")
			canonical := filepath.Join(ctx.projectRoot, ".agents", "AGENTS.md")

			if !doctorFileExists(canonical) {
				if pathExistsForDoctor(linkPath) && !isSymlinkForDoctor(linkPath) {
					return doctorResult{
						Status:  doctorFail,
						Message: "Legacy layout - .claude/CLAUDE.md exists as a real file, canonical .agents/AGENTS.md missing",
						Detail:  fmt.Sprintf("%s has content but .agents/AGENTS.md doesn't exist yet. Run `loaf doctor --fix` to migrate content into canonical, back up as .claude/CLAUDE.md.bak, and replace with a symlink.", linkPath),
						Fixable: true,
					}
				}
				return doctorResult{Status: doctorSkip, Message: "No .agents/AGENTS.md to link to"}
			}
			if !pathExistsForDoctor(linkPath) {
				return doctorResult{
					Status:  doctorFail,
					Message: ".claude/CLAUDE.md is missing",
					Detail:  fmt.Sprintf("Expected symlink at %s -> .agents/AGENTS.md", linkPath),
					Fixable: true,
				}
			}
			if !isSymlinkForDoctor(linkPath) {
				return doctorResult{
					Status:  doctorFail,
					Message: ".claude/CLAUDE.md exists but is not a symlink",
					Detail:  "Duplicate content risks drift from .agents/AGENTS.md. Run `loaf doctor --fix` to merge its content into canonical, back up as .claude/CLAUDE.md.bak, and replace with a symlink.",
					Fixable: true,
				}
			}
			if !symlinkPointsToForDoctor(linkPath, canonical) {
				actual := resolveSymlinkForDoctor(linkPath)
				if actual == "" {
					actual = "<unreadable>"
				}
				return doctorResult{
					Status:  doctorFail,
					Message: ".claude/CLAUDE.md points to the wrong target",
					Detail:  fmt.Sprintf("Got: %s\nWant: %s", actual, canonical),
					Fixable: true,
				}
			}
			return doctorResult{Status: doctorPass, Message: ".claude/CLAUDE.md -> .agents/AGENTS.md"}
		},
		Fix: func(ctx doctorContext, _ doctorResult) doctorFixResult {
			linkPath := filepath.Join(ctx.projectRoot, ".claude", "CLAUDE.md")
			canonical := filepath.Join(ctx.projectRoot, ".agents", "AGENTS.md")
			if !doctorFileExists(canonical) {
				if err := os.MkdirAll(filepath.Dir(canonical), 0o755); err != nil {
					return doctorFixResult{Fixed: false, Message: fmt.Sprintf("Could not create .agents/AGENTS.md: %v", err)}
				}
				if err := os.WriteFile(canonical, []byte{}, 0o644); err != nil {
					return doctorFixResult{Fixed: false, Message: fmt.Sprintf("Could not create .agents/AGENTS.md: %v", err)}
				}
			}
			if pathExistsForDoctor(linkPath) && !isSymlinkForDoctor(linkPath) {
				return migrateRealFileToDoctorSymlink(linkPath, canonical, ctx.projectRoot)
			}
			if isSymlinkForDoctor(linkPath) {
				if err := os.Remove(linkPath); err != nil {
					return doctorFixResult{Fixed: false, Message: fmt.Sprintf("Symlink failed: %v", err)}
				}
			}
			return createDoctorSymlink(linkPath, canonical, "Created .claude/CLAUDE.md")
		},
	}
}

func checkCanonicalAgentsFile() doctorCheck {
	return doctorCheck{
		Name:        "canonical-agents-file",
		Description: ".agents/AGENTS.md exists when referenced by symlinks or when legacy real files are present",
		Run: func(ctx doctorContext) doctorResult {
			canonical := filepath.Join(ctx.projectRoot, ".agents", "AGENTS.md")
			agentsLink := filepath.Join(ctx.projectRoot, "AGENTS.md")
			claudeLink := filepath.Join(ctx.projectRoot, ".claude", "CLAUDE.md")
			canonicalExists := doctorFileExists(canonical)
			referenced := (isSymlinkForDoctor(agentsLink) && symlinkPointsToForDoctor(agentsLink, canonical)) ||
				(isSymlinkForDoctor(claudeLink) && symlinkPointsToForDoctor(claudeLink, canonical))

			if !referenced {
				if !canonicalExists {
					agentsIsReal := pathExistsForDoctor(agentsLink) && !isSymlinkForDoctor(agentsLink)
					claudeIsReal := pathExistsForDoctor(claudeLink) && !isSymlinkForDoctor(claudeLink)
					if agentsIsReal || claudeIsReal {
						var found []string
						if agentsIsReal {
							found = append(found, "./AGENTS.md")
						}
						if claudeIsReal {
							found = append(found, ".claude/CLAUDE.md")
						}
						return doctorResult{
							Status:  doctorFail,
							Message: "Legacy layout - real files exist but .agents/AGENTS.md is not set up",
							Detail:  fmt.Sprintf("Found real files: %s. Run `loaf doctor --fix` to create .agents/AGENTS.md and migrate content into it.", strings.Join(found, ", ")),
							Fixable: true,
						}
					}
				}
				return doctorResult{Status: doctorSkip, Message: "No symlinks reference .agents/AGENTS.md"}
			}
			if !canonicalExists {
				return doctorResult{
					Status:  doctorFail,
					Message: ".agents/AGENTS.md is missing but referenced by symlinks",
					Detail:  fmt.Sprintf("Dangling symlinks point at %s", canonical),
				}
			}
			return doctorResult{Status: doctorPass, Message: ".agents/AGENTS.md is present"}
		},
		Fix: func(ctx doctorContext, _ doctorResult) doctorFixResult {
			canonical := filepath.Join(ctx.projectRoot, ".agents", "AGENTS.md")
			agentsLink := filepath.Join(ctx.projectRoot, "AGENTS.md")
			claudeLink := filepath.Join(ctx.projectRoot, ".claude", "CLAUDE.md")
			if doctorFileExists(canonical) {
				return doctorFixResult{Fixed: false, Message: "State no longer matches - re-run doctor"}
			}
			agentsIsReal := pathExistsForDoctor(agentsLink) && !isSymlinkForDoctor(agentsLink)
			claudeIsReal := pathExistsForDoctor(claudeLink) && !isSymlinkForDoctor(claudeLink)
			if !agentsIsReal && !claudeIsReal {
				return doctorFixResult{Fixed: false, Message: "No legacy real files to migrate"}
			}
			if err := os.MkdirAll(filepath.Dir(canonical), 0o755); err != nil {
				return doctorFixResult{Fixed: false, Message: fmt.Sprintf("Could not create .agents/AGENTS.md: %v", err)}
			}
			if err := os.WriteFile(canonical, []byte{}, 0o644); err != nil {
				return doctorFixResult{Fixed: false, Message: fmt.Sprintf("Could not create .agents/AGENTS.md: %v", err)}
			}
			var messages []string
			if agentsIsReal {
				messages = append(messages, migrateRealFileToDoctorSymlink(agentsLink, canonical, ctx.projectRoot).Message)
			}
			if claudeIsReal {
				messages = append(messages, migrateRealFileToDoctorSymlink(claudeLink, canonical, ctx.projectRoot).Message)
			}
			return doctorFixResult{Fixed: true, Message: "Created .agents/AGENTS.md and migrated: " + strings.Join(messages, "; ")}
		},
	}
}

func checkStaleCursorMdc() doctorCheck {
	return doctorCheck{
		Name:        "stale-cursor-mdc",
		Description: "No stale .cursor/rules/loaf.mdc left over from legacy installs",
		Run: func(ctx doctorContext) doctorResult {
			stale := filepath.Join(ctx.projectRoot, ".cursor", "rules", "loaf.mdc")
			if !pathExistsForDoctor(stale) {
				return doctorResult{Status: doctorPass, Message: "No stale .cursor/rules/loaf.mdc"}
			}
			return doctorResult{
				Status:  doctorFail,
				Message: "Stale .cursor/rules/loaf.mdc should be removed",
				Detail:  "Cursor now uses .agents/AGENTS.md via the consolidated prompt overlay.",
				Fixable: true,
			}
		},
		Fix: func(ctx doctorContext, _ doctorResult) doctorFixResult {
			stale := filepath.Join(ctx.projectRoot, ".cursor", "rules", "loaf.mdc")
			if !pathExistsForDoctor(stale) {
				return doctorFixResult{Fixed: false, Message: "Nothing to remove"}
			}
			if err := os.Remove(stale); err != nil {
				return doctorFixResult{Fixed: false, Message: fmt.Sprintf("Delete failed: %v", err)}
			}
			return doctorFixResult{Fixed: true, Message: "Removed .cursor/rules/loaf.mdc"}
		},
	}
}

func checkFencedVersion(cliVersion string) doctorCheck {
	return doctorCheck{
		Name:        "fenced-version",
		Description: "Fenced section version matches installed loaf version",
		Run: func(ctx doctorContext) doctorResult {
			canonical := filepath.Join(ctx.projectRoot, ".agents", "AGENTS.md")
			if !doctorFileExists(canonical) {
				return doctorResult{Status: doctorSkip, Message: "No .agents/AGENTS.md to inspect"}
			}
			fencedVersion, ok := getDoctorFencedVersion(canonical)
			if !ok {
				return doctorResult{
					Status:  doctorWarn,
					Message: "No loaf:managed fenced section found in .agents/AGENTS.md",
					Detail:  "Run `loaf install` to add the framework section.",
				}
			}
			if fencedVersion != cliVersion {
				return doctorResult{
					Status:  doctorWarn,
					Message: fmt.Sprintf("Fenced section version drift: %s (installed: %s)", fencedVersion, cliVersion),
					Detail:  "Run `loaf install --upgrade` to refresh the fenced section.",
				}
			}
			return doctorResult{Status: doctorPass, Message: fmt.Sprintf("Fenced section is v%s (matches installed loaf)", fencedVersion)}
		},
	}
}

func checkDuplicateFencedSections() doctorCheck {
	return doctorCheck{
		Name:        "duplicate-fenced-sections",
		Description: "No duplicate fenced sections across real files",
		Run: func(ctx doctorContext) doctorResult {
			claudePath := filepath.Join(ctx.projectRoot, ".claude", "CLAUDE.md")
			agentsPath := filepath.Join(ctx.projectRoot, ".agents", "AGENTS.md")
			claudeIsReal := pathExistsForDoctor(claudePath) && !isSymlinkForDoctor(claudePath)
			agentsIsReal := pathExistsForDoctor(agentsPath) && !isSymlinkForDoctor(agentsPath)
			if !claudeIsReal || !agentsIsReal {
				return doctorResult{Status: doctorSkip, Message: "No duplication risk - at least one side is absent or a symlink"}
			}
			if hasDoctorFencedSection(claudePath) && hasDoctorFencedSection(agentsPath) {
				return doctorResult{
					Status:  doctorFail,
					Message: "Duplicate fenced sections in both .claude/CLAUDE.md and .agents/AGENTS.md",
					Detail:  "These will drift on upgrade. Run `loaf doctor --fix` to merge .claude/CLAUDE.md content into .agents/AGENTS.md, back it up, and replace it with a symlink. No content is deleted.",
					Fixable: true,
				}
			}
			return doctorResult{Status: doctorPass, Message: "No duplicate fenced sections across real files"}
		},
		Fix: func(ctx doctorContext, _ doctorResult) doctorFixResult {
			claudePath := filepath.Join(ctx.projectRoot, ".claude", "CLAUDE.md")
			canonical := filepath.Join(ctx.projectRoot, ".agents", "AGENTS.md")
			if !doctorFileExists(canonical) || !pathExistsForDoctor(claudePath) || isSymlinkForDoctor(claudePath) {
				return doctorFixResult{Fixed: false, Message: "State no longer matches - re-run doctor"}
			}
			return migrateRealFileToDoctorSymlink(claudePath, canonical, ctx.projectRoot)
		},
	}
}

func safeRunDoctorCheck(check doctorCheck, ctx doctorContext) (result doctorResult) {
	defer func() {
		if recovered := recover(); recovered != nil {
			result = doctorResult{Status: doctorFail, Message: fmt.Sprintf("Check threw an error: %v", recovered)}
		}
	}()
	return check.Run(ctx)
}

func safeFixDoctorCheck(check doctorCheck, ctx doctorContext, last doctorResult) (result doctorFixResult) {
	defer func() {
		if recovered := recover(); recovered != nil {
			result = doctorFixResult{Fixed: false, Message: fmt.Sprintf("Fix threw an error: %v", recovered)}
		}
	}()
	return check.Fix(ctx, last)
}

func printDoctorCheckLine(out io.Writer, check doctorCheck, result doctorResult, options doctorOptions) {
	glyph := map[doctorStatus]string{
		doctorPass: ansiGreen("✓"),
		doctorWarn: ansiYellow("⚠"),
		doctorFail: ansiRed("✗"),
		doctorSkip: ansiGray("-"),
	}[result.Status]
	showName := result.Status != doctorPass || options.verbose
	label := result.Message
	if showName {
		label = fmt.Sprintf("%s - %s", ansiBold(check.Name), result.Message)
	}
	fmt.Fprintf(out, "  %s %s\n", glyph, label)
	if result.Detail != "" && (result.Status == doctorFail || result.Status == doctorWarn) {
		for _, line := range strings.Split(result.Detail, "\n") {
			fmt.Fprintf(out, "    %s\n", ansiGray(line))
		}
	}
}

func printDoctorFixLine(out io.Writer, fix doctorFixResult) {
	glyph := ansiYellow("→")
	if fix.Fixed {
		glyph = ansiGreen("→")
	}
	fmt.Fprintf(out, "    %s %s\n", glyph, fix.Message)
}

func printDoctorSummary(out io.Writer, report doctorReport) {
	parts := []string{ansiGreen(fmt.Sprintf("%d passed", report.Passes))}
	if report.Warnings > 0 {
		parts = append(parts, ansiYellow(fmt.Sprintf("%d warning", report.Warnings)))
	}
	if report.Failures > 0 {
		parts = append(parts, ansiRed(fmt.Sprintf("%d failed", report.Failures)))
	}
	if report.Skips > 0 {
		parts = append(parts, ansiGray(fmt.Sprintf("%d skipped", report.Skips)))
	}
	prefix := ansiGreen("✓")
	if report.Failures > 0 {
		prefix = ansiRed("✗")
	} else if report.Warnings > 0 {
		prefix = ansiYellow("⚠")
	}
	fmt.Fprintf(out, "\n  %s %s\n", prefix, strings.Join(parts, ansiGray(" · ")))
	if report.FixesApplied > 0 || report.FixesFailed > 0 {
		var fixParts []string
		if report.FixesApplied > 0 {
			fixParts = append(fixParts, ansiGreen(fmt.Sprintf("%d fixed", report.FixesApplied)))
		}
		if report.FixesFailed > 0 {
			fixParts = append(fixParts, ansiRed(fmt.Sprintf("%d could not be fixed", report.FixesFailed)))
		}
		fmt.Fprintf(out, "  %s %s\n", ansiCyan("→"), strings.Join(fixParts, ansiGray(" · ")))
	}
	fmt.Fprintln(out)
}

func tallyDoctorStatus(status doctorStatus, report *doctorReport) {
	switch status {
	case doctorPass:
		report.Passes++
	case doctorWarn:
		report.Warnings++
	case doctorFail:
		report.Failures++
	case doctorSkip:
		report.Skips++
	}
}

func isSymlinkForDoctor(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.Mode()&os.ModeSymlink != 0
}

func pathExistsForDoctor(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

func doctorFileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func resolveSymlinkForDoctor(linkPath string) string {
	target, err := os.Readlink(linkPath)
	if err != nil {
		return ""
	}
	if filepath.IsAbs(target) {
		return filepath.Clean(target)
	}
	return filepath.Clean(filepath.Join(filepath.Dir(linkPath), target))
}

func symlinkPointsToForDoctor(linkPath string, expectedAbs string) bool {
	resolved := resolveSymlinkForDoctor(linkPath)
	return resolved != "" && filepath.Clean(resolved) == filepath.Clean(expectedAbs)
}

func createDoctorSymlink(linkPath string, canonical string, prefix string) doctorFixResult {
	if err := os.MkdirAll(filepath.Dir(linkPath), 0o755); err != nil {
		return doctorFixResult{Fixed: false, Message: "Could not prepare parent directory"}
	}
	relTarget, err := filepath.Rel(filepath.Dir(linkPath), canonical)
	if err != nil {
		return doctorFixResult{Fixed: false, Message: fmt.Sprintf("Symlink failed: %v", err)}
	}
	if err := os.Symlink(relTarget, linkPath); err != nil {
		return doctorFixResult{Fixed: false, Message: fmt.Sprintf("Symlink failed: %v", err)}
	}
	return doctorFixResult{Fixed: true, Message: fmt.Sprintf("%s -> %s", prefix, relTarget)}
}

func migrateRealFileToDoctorSymlink(linkPath string, canonical string, projectRoot string) doctorFixResult {
	sourceContent, err := os.ReadFile(linkPath)
	if err != nil {
		return doctorFixResult{Fixed: false, Message: fmt.Sprintf("Migration failed: %v", err)}
	}
	relSource, err := filepath.Rel(projectRoot, linkPath)
	if err != nil {
		relSource = linkPath
	}
	stripped := stripDoctorLoafFence(string(sourceContent))
	merged := false
	if stripped != "" {
		merged, err = mergeDoctorContentIntoCanonical(canonical, stripped, relSource)
		if err != nil {
			return doctorFixResult{Fixed: false, Message: fmt.Sprintf("Migration failed: %v", err)}
		}
	}
	backupPath := linkPath + ".bak"
	if err := os.RemoveAll(backupPath); err != nil {
		return doctorFixResult{Fixed: false, Message: fmt.Sprintf("Migration failed: %v", err)}
	}
	if err := os.Rename(linkPath, backupPath); err != nil {
		return doctorFixResult{Fixed: false, Message: fmt.Sprintf("Migration failed: %v", err)}
	}
	if err := os.MkdirAll(filepath.Dir(linkPath), 0o755); err != nil {
		return doctorFixResult{Fixed: false, Message: "Could not prepare parent directory"}
	}
	relTarget, err := filepath.Rel(filepath.Dir(linkPath), canonical)
	if err != nil {
		return doctorFixResult{Fixed: false, Message: fmt.Sprintf("Symlink failed: %v", err)}
	}
	if err := os.Symlink(relTarget, linkPath); err != nil {
		return doctorFixResult{Fixed: false, Message: fmt.Sprintf("Symlink failed: %v", err)}
	}
	suffix := ""
	if merged {
		suffix = " (merged content into canonical)"
	}
	return doctorFixResult{
		Fixed:   true,
		Message: fmt.Sprintf("Migrated %s -> %s, backup at %s.bak%s", relSource, relTarget, relSource, suffix),
	}
}

func mergeDoctorContentIntoCanonical(canonical string, stripped string, relSource string) (bool, error) {
	if stripped == "" {
		return false, nil
	}
	if !doctorFileExists(canonical) {
		if err := os.MkdirAll(filepath.Dir(canonical), 0o755); err != nil {
			return false, err
		}
		return true, os.WriteFile(canonical, []byte(stripped+"\n"), 0o644)
	}
	existing, err := os.ReadFile(canonical)
	if err != nil {
		return false, err
	}
	trimmedExisting := strings.TrimRight(string(existing), " \t\r\n")
	appended := trimmedExisting + "\n\n## Migrated from " + relSource + "\n\n" + stripped + "\n"
	return true, os.WriteFile(canonical, []byte(appended), 0o644)
}

func stripDoctorLoafFence(content string) string {
	start := strings.Index(content, "<!-- loaf:managed:start")
	if start < 0 {
		return strings.TrimSpace(content)
	}
	endMarker := "<!-- loaf:managed:end -->"
	endRelative := strings.Index(content[start:], endMarker)
	if endRelative < 0 {
		return strings.TrimSpace(content)
	}
	end := start + endRelative + len(endMarker)
	return strings.TrimSpace(content[:start] + content[end:])
}

func hasDoctorFencedSection(path string) bool {
	body, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	text := string(body)
	return strings.Contains(text, "<!-- loaf:managed:start") && strings.Contains(text, "<!-- loaf:managed:end -->")
}

func getDoctorFencedVersion(path string) (string, bool) {
	body, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	match := regexp.MustCompile(`<!-- loaf:managed:start v([^ >]+) -->`).FindStringSubmatch(string(body))
	if len(match) != 2 {
		return "", false
	}
	return match[1], true
}
