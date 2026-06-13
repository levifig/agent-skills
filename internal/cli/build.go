package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var defaultBuildTargets = []string{"claude-code", "opencode", "cursor", "codex", "gemini", "amp"}

type buildOptions struct {
	target string
	help   bool
}

func (r Runner) runBuild(args []string, out io.Writer, runtimeRoot string) error {
	options, err := parseBuildArgs(args)
	if err != nil {
		return err
	}
	if options.help {
		writeBuildHelp(out)
		return nil
	}
	loafRoot, err := resolveLoafPackageRoot(r.WorkingDir, runtimeRoot)
	if err != nil {
		return err
	}
	return runBuildContent(loafRoot, options, out)
}

func parseBuildArgs(args []string) (buildOptions, error) {
	var options buildOptions
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--help" || arg == "-h":
			options.help = true
		case arg == "-t" || arg == "--target":
			if i+1 >= len(args) {
				return buildOptions{}, fmt.Errorf("%s requires a value", arg)
			}
			i++
			options.target = args[i]
		case strings.HasPrefix(arg, "--target="):
			options.target = strings.TrimPrefix(arg, "--target=")
			if options.target == "" {
				return buildOptions{}, fmt.Errorf("--target requires a value")
			}
		default:
			return buildOptions{}, fmt.Errorf("unknown build option %q", arg)
		}
	}
	return options, nil
}

func writeBuildHelp(out io.Writer) {
	fmt.Fprintln(out, strings.Join([]string{
		"Usage: loaf build [options]",
		"",
		"Build skill distributions for agent harnesses.",
		"",
		"Options:",
		"  -t, --target <name>  Build a specific target only",
		"  -h, --help           Show help",
	}, "\n"))
}

func runBuildContent(loafRoot string, options buildOptions, out io.Writer) error {
	if options.target == "" {
		return runNativeBuildAllTargets(loafRoot, out)
	}
	targets, err := nativeBuildTargetNames(loafRoot)
	if err != nil {
		return err
	}
	if !containsBuildTarget(targets, options.target) {
		return fmt.Errorf("error: Unknown target %s\nValid targets: %s", options.target, strings.Join(targets, ", "))
	}
	return runNativeBuildTarget(loafRoot, out, options.target)
}

func runNativeBuildTarget(root string, out io.Writer, targetName string) error {
	switch targetName {
	case "claude-code":
		return runNativeBuildClaudeCode(root, out)
	case "opencode":
		return runNativeBuildOpenCode(root, out)
	case "cursor":
		return runNativeBuildCursor(root, out)
	case "codex":
		return runNativeBuildCodex(root, out)
	case "gemini":
		return runNativeBuildSkillOnlyTarget(root, out, "gemini")
	case "amp":
		return runNativeBuildAmp(root, out)
	default:
		return fmt.Errorf("native build target %s is not implemented", targetName)
	}
}

func runNativeBuildAllTargets(root string, out io.Writer) error {
	start := time.Now()
	fmt.Fprintf(out, "\n%s\n\n", ansiBold("loaf build"))

	sharedStart := time.Now()
	fmt.Fprintf(out, "  %s shared skills intermediate...", ansiCyan("building"))
	if err := buildNativeSharedSkillsIntermediate(root); err != nil {
		fmt.Fprintf(out, "\r  %s shared skills intermediate\n", ansiRed("✗"))
		return err
	}
	fmt.Fprintf(out, "\r  %s shared skills intermediate %s\n", ansiGreen("✓"), ansiGray("("+elapsedSeconds(sharedStart)+")"))

	failed := false
	for _, targetName := range defaultBuildTargets {
		targetStart := time.Now()
		fmt.Fprintf(out, "  %s %s...", ansiCyan("building"), targetName)
		if err := buildNativeTargetOnly(root, targetName); err != nil {
			fmt.Fprintf(out, "\r  %s %s\n", ansiRed("✗"), targetName)
			fmt.Fprintf(out, "    %s\n", ansiRed(err.Error()))
			failed = true
			continue
		}
		fmt.Fprintf(out, "\r  %s %s %s\n", ansiGreen("✓"), targetName, ansiGray("("+elapsedSeconds(targetStart)+")"))
	}
	fmt.Fprintln(out)
	if failed {
		return fmt.Errorf("%s %s", ansiRed("Build failed"), ansiGray("("+elapsedSeconds(start)+")"))
	}
	fmt.Fprintf(out, "%s %s\n", ansiGreen("Build complete"), ansiGray("("+elapsedSeconds(start)+")"))
	return nil
}

func buildNativeTargetOnly(root string, targetName string) error {
	switch targetName {
	case "claude-code":
		return buildNativeClaudeCodeTarget(root)
	case "opencode":
		return buildNativeOpenCodeTarget(root)
	case "cursor":
		return buildNativeCursorTarget(root)
	case "codex":
		return buildNativeCodexTarget(root)
	case "gemini":
		return buildNativeSkillOnlyTarget(root, "gemini")
	case "amp":
		return buildNativeAmpTarget(root)
	default:
		return fmt.Errorf("native build target %s is not implemented", targetName)
	}
}

func nativeBuildTargetNames(loafRoot string) ([]string, error) {
	path := filepath.Join(loafRoot, "config", "targets.yaml")
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return append([]string{}, defaultBuildTargets...), nil
		}
		return nil, err
	}
	defer file.Close()

	var targets []string
	inTargets := false
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if !strings.HasPrefix(line, " ") && strings.HasSuffix(trimmed, ":") {
			inTargets = trimmed == "targets:"
			continue
		}
		if !inTargets {
			continue
		}
		if strings.HasPrefix(line, "  ") && !strings.HasPrefix(line, "    ") && strings.HasSuffix(trimmed, ":") {
			name := strings.TrimSuffix(trimmed, ":")
			if name != "" {
				targets = append(targets, name)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(targets) == 0 {
		return append([]string{}, defaultBuildTargets...), nil
	}
	return targets, nil
}

func containsBuildTarget(targets []string, target string) bool {
	for _, candidate := range targets {
		if candidate == target {
			return true
		}
	}
	return false
}
