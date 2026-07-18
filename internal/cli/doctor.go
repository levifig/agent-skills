package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
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
	RepairID    string
	Repair      string
	Run         func(doctorContext) doctorResult
	Fix         func(doctorContext, doctorResult) doctorFixResult
}

type doctorOptions struct {
	fix     bool
	force   bool
	verbose bool
	help    bool
}

type doctorReport struct {
	Passes        int
	Warnings      int
	Failures      int
	Skips         int
	FixesApplied  int
	FixesFailed   int
	FixesDeclined int
	FixesSkipped  int
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
	report := runDoctorChecks(out, doctorContext{projectRoot: runtimeRoot}, options, packageVersion(runtimeRoot), r.Stdin)
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
		case "--force":
			options.force = true
		case "--verbose":
			options.verbose = true
		case "--help", "-h":
			options.help = true
		default:
			return options, fmt.Errorf("unknown doctor option %q", arg)
		}
	}
	if options.force && !options.fix {
		return options, fmt.Errorf("--force requires --fix (usage: loaf doctor --fix --force)")
	}
	return options, nil
}

func writeDoctorHelp(out io.Writer) {
	fmt.Fprint(out, "Usage: loaf doctor [--fix [--force]] [--verbose]\n\n")
	fmt.Fprint(out, "Diagnose Loaf project alignment (symlinks, stale files, version drift)\n\n")
	fmt.Fprint(out, "Options:\n")
	fmt.Fprint(out, "  --fix       Offer each safe repair and prompt y/N before applying it\n")
	fmt.Fprint(out, "  --force     With --fix, apply every offered repair without prompting\n")
	fmt.Fprint(out, "  --verbose   Print each check name even when passing\n")
	fmt.Fprint(out, "  -h, --help  Show help\n")
}

func runDoctorChecks(out io.Writer, ctx doctorContext, options doctorOptions, cliVersion string, input io.Reader) doctorReport {
	report := doctorReport{}
	interactive := doctorInputIsInteractive(input)
	reader := bufio.NewReader(firstReader(input, os.Stdin))
	handledRepairs := map[string]string{}
	fmt.Fprintf(out, "\n%s\n\n", ansiBold("loaf doctor"))

	for _, check := range doctorChecks(cliVersion) {
		result := safeRunDoctorCheck(check, ctx)
		printDoctorCheckLine(out, check, result, options)

		if options.fix && result.Status == doctorFail && result.Fixable && check.Fix != nil {
			repairID := check.RepairID
			if repairID == "" {
				repairID = check.Name
			}
			if priorCheck, handled := handledRepairs[repairID]; handled {
				printDoctorRepairSkipped(out, check, "already handled by "+priorCheck+"; no second attempt is allowed in the same run")
				tallyDoctorStatus(result.Status, &report)
				continue
			}
			handledRepairs[repairID] = check.Name
			accepted := options.force
			if !options.force {
				if !interactive {
					printDoctorRepairSkipped(out, check, "non-interactive input; rerun with `loaf doctor --fix --force` to apply this repair")
					report.FixesSkipped++
					tallyDoctorStatus(result.Status, &report)
					continue
				}
				var promptErr error
				accepted, promptErr = promptDoctorRepair(reader, out, check)
				if promptErr != nil {
					printDoctorRepairSkipped(out, check, "could not read confirmation: "+promptErr.Error())
					report.FixesDeclined++
					tallyDoctorStatus(result.Status, &report)
					continue
				}
				if !accepted {
					printDoctorRepairSkipped(out, check, "declined; no changes made")
					report.FixesDeclined++
					tallyDoctorStatus(result.Status, &report)
					continue
				}
			}
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

func doctorInputIsInteractive(input io.Reader) bool {
	if input != nil {
		if _, ok := input.(*os.File); !ok {
			return true
		}
	}
	return readerIsTerminal(input)
}

func promptDoctorRepair(reader *bufio.Reader, out io.Writer, check doctorCheck) (bool, error) {
	repair := check.Repair
	if repair == "" {
		repair = check.Description
	}
	fmt.Fprintf(out, "    %s %s: %s? [y/N] ", ansiCyan("?"), ansiBold(check.Name), repair)
	answer, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return false, err
	}
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(answer)), "y"), nil
}

func printDoctorRepairSkipped(out io.Writer, check doctorCheck, reason string) {
	repair := check.Repair
	if repair == "" {
		repair = check.Description
	}
	fmt.Fprintf(out, "    %s Repair for %s skipped (%s): %s\n", ansiYellow("→"), check.Name, repair, reason)
}

func doctorChecks(cliVersion string) []doctorCheck {
	return []doctorCheck{
		checkCanonicalAgentsFile(),
		checkLegacyAgentsFile(),
		checkClaudeSymlink(),
		checkStaleCursorMdc(),
		checkFencedVersion(cliVersion),
		checkDuplicateFencedSections(),
	}
}

func checkLegacyAgentsFile() doctorCheck {
	return doctorCheck{
		Name:        "legacy-agents-file",
		Description: ".agents/AGENTS.md is absent after migration to root AGENTS.md",
		Repair:      "Preserve and merge user content, back up .agents/AGENTS.md, and retire the legacy path",
		Run: func(ctx doctorContext) doctorResult {
			legacy := filepath.Join(ctx.projectRoot, ".agents", "AGENTS.md")
			if !pathExistsForDoctor(legacy) {
				return doctorResult{Status: doctorPass, Message: "No legacy .agents/AGENTS.md"}
			}
			return doctorResult{Status: doctorFail, Message: "Legacy .agents/AGENTS.md is still present", Detail: "Root AGENTS.md is now canonical. Run `loaf doctor --fix` to preserve any legacy content and retire the old path.", Fixable: true}
		},
		Fix: func(ctx doctorContext, _ doctorResult) doctorFixResult {
			return retireLegacyDoctorAgentsFile(ctx.projectRoot)
		},
	}
}

func checkClaudeSymlink() doctorCheck {
	return doctorCheck{
		Name:        "claude-symlink",
		Description: ".claude/CLAUDE.md is a symlink to root AGENTS.md",
		RepairID:    "claude-compatibility-link",
		Repair:      "Create .claude/CLAUDE.md -> ../AGENTS.md, preserving and backing up any real file",
		Run: func(ctx doctorContext) doctorResult {
			linkPath := filepath.Join(ctx.projectRoot, ".claude", "CLAUDE.md")
			canonical := filepath.Join(ctx.projectRoot, "AGENTS.md")

			if !doctorRegularFileExists(canonical) {
				if doctorIsDirectory(canonical) {
					return doctorResult{Status: doctorSkip, Message: "Root AGENTS.md is not a regular file"}
				}
				if pathExistsForDoctor(linkPath) && !isSymlinkForDoctor(linkPath) {
					return doctorResult{
						Status:  doctorFail,
						Message: "Legacy layout - .claude/CLAUDE.md exists as a real file, canonical AGENTS.md missing",
						Detail:  fmt.Sprintf("%s has content but root AGENTS.md doesn't exist yet. Run `loaf doctor --fix` to migrate content into canonical, back up as .claude/CLAUDE.md.bak, and replace with a symlink.", linkPath),
						Fixable: true,
					}
				}
				return doctorResult{Status: doctorSkip, Message: "No root AGENTS.md to link to"}
			}
			if !pathExistsForDoctor(linkPath) {
				return doctorResult{
					Status:  doctorFail,
					Message: ".claude/CLAUDE.md is missing",
					Detail:  fmt.Sprintf("Expected symlink at %s -> ../AGENTS.md", linkPath),
					Fixable: true,
				}
			}
			if !isSymlinkForDoctor(linkPath) {
				return doctorResult{
					Status:  doctorFail,
					Message: ".claude/CLAUDE.md exists but is not a symlink",
					Detail:  "Duplicate content risks drift from root AGENTS.md. Run `loaf doctor --fix` to merge its content into canonical, back it up as .claude/CLAUDE.md.bak, and replace it with a symlink.",
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
			return doctorResult{Status: doctorPass, Message: ".claude/CLAUDE.md -> ../AGENTS.md"}
		},
		Fix: func(ctx doctorContext, _ doctorResult) doctorFixResult {
			linkPath := filepath.Join(ctx.projectRoot, ".claude", "CLAUDE.md")
			canonical := filepath.Join(ctx.projectRoot, "AGENTS.md")
			if !doctorFileExists(canonical) {
				if err := os.MkdirAll(filepath.Dir(canonical), 0o755); err != nil {
					return doctorFixResult{Fixed: false, Message: fmt.Sprintf("Could not create AGENTS.md: %v", err)}
				}
				if err := os.WriteFile(canonical, []byte{}, 0o644); err != nil {
					return doctorFixResult{Fixed: false, Message: fmt.Sprintf("Could not create AGENTS.md: %v", err)}
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
		Description: "Root AGENTS.md is the canonical real project instruction file",
		Repair:      "Create a canonical real root AGENTS.md while preserving legacy instruction content",
		Run: func(ctx doctorContext) doctorResult {
			canonical := filepath.Join(ctx.projectRoot, "AGENTS.md")
			legacy := filepath.Join(ctx.projectRoot, ".agents", "AGENTS.md")
			claudeLink := filepath.Join(ctx.projectRoot, ".claude", "CLAUDE.md")
			if !pathExistsForDoctor(canonical) {
				if !pathExistsForDoctor(legacy) && !pathExistsForDoctor(claudeLink) {
					return doctorResult{Status: doctorSkip, Message: "No project instruction files to inspect"}
				}
				return doctorResult{Status: doctorFail, Message: "Root AGENTS.md is missing", Detail: "Run `loaf doctor --fix` to create the canonical root file and preserve legacy instruction content.", Fixable: true}
			}
			if doctorIsDirectory(canonical) {
				return doctorResult{Status: doctorFail, Message: "Root AGENTS.md is a directory", Detail: "Expected a canonical regular file. Move or remove the directory manually before running `loaf doctor --fix`.", Fixable: false}
			}
			if isSymlinkForDoctor(canonical) {
				return doctorResult{Status: doctorFail, Message: "Root AGENTS.md is a symlink instead of the canonical real file", Detail: "Run `loaf doctor --fix` to preserve its resolved content, replace it with a real file, and retire the old .agents/AGENTS.md layout when present.", Fixable: true}
			}
			return doctorResult{Status: doctorPass, Message: "Root AGENTS.md is the canonical real file"}
		},
		Fix: func(ctx doctorContext, _ doctorResult) doctorFixResult {
			canonical := filepath.Join(ctx.projectRoot, "AGENTS.md")
			legacy := filepath.Join(ctx.projectRoot, ".agents", "AGENTS.md")
			claudeLink := filepath.Join(ctx.projectRoot, ".claude", "CLAUDE.md")
			if pathExistsForDoctor(canonical) && !isSymlinkForDoctor(canonical) {
				return doctorFixResult{Fixed: false, Message: "State no longer matches - re-run doctor"}
			}
			body := []byte{}
			legacyIsReal := doctorRegularFileExists(legacy) && !isSymlinkForDoctor(legacy)
			if legacyIsReal {
				var err error
				body, err = os.ReadFile(legacy)
				if err != nil {
					return doctorFixResult{Fixed: false, Message: fmt.Sprintf("Migration failed: %v", err)}
				}
			} else if pathExistsForDoctor(canonical) {
				var err error
				body, err = os.ReadFile(canonical)
				if err != nil && !os.IsNotExist(err) {
					return doctorFixResult{Fixed: false, Message: fmt.Sprintf("Migration failed: %v", err)}
				}
			} else if pathExistsForDoctor(claudeLink) && !isSymlinkForDoctor(claudeLink) {
				var err error
				body, err = os.ReadFile(claudeLink)
				if err != nil {
					return doctorFixResult{Fixed: false, Message: fmt.Sprintf("Migration failed: %v", err)}
				}
			}
			canonicalWasSymlink := isSymlinkForDoctor(canonical)
			if pathExistsForDoctor(canonical) {
				if err := os.Remove(canonical); err != nil {
					return doctorFixResult{Fixed: false, Message: fmt.Sprintf("Migration failed: %v", err)}
				}
			}
			legacyBackup := ""
			if legacyIsReal {
				legacyBackup = collisionSafeInstallBackupPath(legacy)
				if err := os.Rename(legacy, legacyBackup); err != nil {
					return doctorFixResult{Fixed: false, Message: fmt.Sprintf("Migration failed: %v", err)}
				}
			}
			if err := os.WriteFile(canonical, body, 0o644); err != nil {
				if legacyBackup != "" {
					_ = os.Rename(legacyBackup, legacy)
					if canonicalWasSymlink {
						_ = os.Symlink(filepath.Join(".agents", "AGENTS.md"), canonical)
					}
				}
				return doctorFixResult{Fixed: false, Message: fmt.Sprintf("Could not create AGENTS.md: %v", err)}
			}
			return doctorFixResult{Fixed: true, Message: "Created canonical root AGENTS.md"}
		},
	}
}

func checkStaleCursorMdc() doctorCheck {
	return doctorCheck{
		Name:        "stale-cursor-mdc",
		Description: "No stale .cursor/rules/loaf.mdc left over from legacy installs",
		Repair:      "Remove stale .cursor/rules/loaf.mdc",
		Run: func(ctx doctorContext) doctorResult {
			stale := filepath.Join(ctx.projectRoot, ".cursor", "rules", "loaf.mdc")
			if !pathExistsForDoctor(stale) {
				return doctorResult{Status: doctorPass, Message: "No stale .cursor/rules/loaf.mdc"}
			}
			return doctorResult{
				Status:  doctorFail,
				Message: "Stale .cursor/rules/loaf.mdc should be removed",
				Detail:  "Cursor now uses root AGENTS.md via the consolidated prompt overlay.",
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
			canonical := filepath.Join(ctx.projectRoot, "AGENTS.md")
			if !doctorRegularFileExists(canonical) {
				return doctorResult{Status: doctorSkip, Message: "No root AGENTS.md to inspect"}
			}
			fencedVersion, ok := getDoctorFencedVersion(canonical)
			if !ok {
				return doctorResult{
					Status:  doctorWarn,
					Message: "No loaf:managed fenced section found in AGENTS.md",
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
		RepairID:    "claude-compatibility-link",
		Repair:      "Merge user content into root AGENTS.md, back up .claude/CLAUDE.md, and replace it with a symlink",
		Run: func(ctx doctorContext) doctorResult {
			claudePath := filepath.Join(ctx.projectRoot, ".claude", "CLAUDE.md")
			agentsPath := filepath.Join(ctx.projectRoot, "AGENTS.md")
			claudeIsReal := pathExistsForDoctor(claudePath) && !isSymlinkForDoctor(claudePath)
			agentsIsReal := pathExistsForDoctor(agentsPath) && !isSymlinkForDoctor(agentsPath)
			if !claudeIsReal || !agentsIsReal {
				return doctorResult{Status: doctorSkip, Message: "No duplication risk - at least one side is absent or a symlink"}
			}
			if hasDoctorFencedSection(claudePath) && hasDoctorFencedSection(agentsPath) {
				return doctorResult{
					Status:  doctorFail,
					Message: "Duplicate fenced sections in both .claude/CLAUDE.md and AGENTS.md",
					Detail:  "These will drift on upgrade. Run `loaf doctor --fix` to merge .claude/CLAUDE.md content into root AGENTS.md, back it up, and replace it with a symlink. No content is deleted.",
					Fixable: true,
				}
			}
			return doctorResult{Status: doctorPass, Message: "No duplicate fenced sections across real files"}
		},
		Fix: func(ctx doctorContext, _ doctorResult) doctorFixResult {
			claudePath := filepath.Join(ctx.projectRoot, ".claude", "CLAUDE.md")
			canonical := filepath.Join(ctx.projectRoot, "AGENTS.md")
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
	if report.FixesApplied > 0 || report.FixesFailed > 0 || report.FixesDeclined > 0 || report.FixesSkipped > 0 {
		var fixParts []string
		if report.FixesApplied > 0 {
			fixParts = append(fixParts, ansiGreen(fmt.Sprintf("%d fixed", report.FixesApplied)))
		}
		if report.FixesFailed > 0 {
			fixParts = append(fixParts, ansiRed(fmt.Sprintf("%d could not be fixed", report.FixesFailed)))
		}
		if report.FixesDeclined > 0 {
			fixParts = append(fixParts, ansiYellow(fmt.Sprintf("%d declined", report.FixesDeclined)))
		}
		if report.FixesSkipped > 0 {
			fixParts = append(fixParts, ansiYellow(fmt.Sprintf("%d repairs skipped", report.FixesSkipped)))
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

func doctorRegularFileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

func doctorIsDirectory(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.IsDir()
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

func retireLegacyDoctorAgentsFile(projectRoot string) doctorFixResult {
	legacy := filepath.Join(projectRoot, ".agents", "AGENTS.md")
	canonical := filepath.Join(projectRoot, "AGENTS.md")
	if !pathExistsForDoctor(legacy) {
		return doctorFixResult{Fixed: false, Message: "State no longer matches - re-run doctor"}
	}
	if isSymlinkForDoctor(legacy) {
		if err := os.Remove(legacy); err != nil {
			return doctorFixResult{Fixed: false, Message: fmt.Sprintf("Migration failed: %v", err)}
		}
		return doctorFixResult{Fixed: true, Message: "Removed legacy .agents/AGENTS.md symlink"}
	}
	if !doctorRegularFileExists(canonical) || isSymlinkForDoctor(canonical) {
		return doctorFixResult{Fixed: false, Message: "Canonical root AGENTS.md is not ready - re-run doctor"}
	}
	body, err := os.ReadFile(legacy)
	if err != nil {
		return doctorFixResult{Fixed: false, Message: fmt.Sprintf("Migration failed: %v", err)}
	}
	stripped := stripDoctorLoafFence(string(body))
	backup := collisionSafeInstallBackupPath(legacy)
	if err := os.Rename(legacy, backup); err != nil {
		return doctorFixResult{Fixed: false, Message: fmt.Sprintf("Migration failed: %v", err)}
	}
	merged, err := mergeLegacyAgentsContentIntoCanonical(canonical, stripped, ".agents/AGENTS.md")
	if err != nil {
		if rollbackErr := os.Rename(backup, legacy); rollbackErr != nil {
			err = fmt.Errorf("%w (rollback failed: %v)", err, rollbackErr)
		}
		return doctorFixResult{Fixed: false, Message: fmt.Sprintf("Migration failed: %v", err)}
	}
	suffix := ""
	if merged {
		suffix = " and merged user content into root AGENTS.md"
	}
	relBackup, relErr := filepath.Rel(projectRoot, backup)
	if relErr != nil {
		relBackup = backup
	}
	return doctorFixResult{Fixed: true, Message: "Backed up legacy .agents/AGENTS.md to " + filepath.ToSlash(relBackup) + suffix}
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
	section, ok := findFencedSectionRange(string(body))
	if !ok || section.malformedHeader || section.version == "" {
		return "", false
	}
	return section.version, true
}
