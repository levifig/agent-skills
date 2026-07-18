package cli

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type setupOptions struct {
	path string
	help bool
}

func (r Runner) runSetup(args []string, out io.Writer, runtimeRoot string) error {
	options, err := parseSetupArgs(args)
	if err != nil {
		return err
	}
	if options.help {
		writeSetupHelp(out)
		return nil
	}

	targetRoot := runtimeRoot
	if options.path != "" {
		targetRoot, err = prepareSetupTarget(runtimeRoot, options.path)
		if err != nil {
			return err
		}
	}

	loafRoot, err := resolveSourceCheckoutRoot(r.WorkingDir, runtimeRoot)
	if err != nil {
		return err
	}

	fmt.Fprintln(out)
	fmt.Fprintln(out, ansiBold("loaf setup"))
	fmt.Fprintln(out)
	if options.path != "" {
		fmt.Fprintf(out, "  %s project %s\n\n", ansiGreen("✓"), ansiGray(targetRoot))
	}

	fmt.Fprintf(out, "  %s Scaffolding project structure...\n", ansiCyan("init"))
	setupRunner := r
	setupRunner.WorkingDir = targetRoot
	if err := setupRunner.runInit([]string{"--no-symlinks"}, out, targetRoot); err != nil {
		return err
	}

	fmt.Fprintf(out, "  %s Building Loaf distributions...\n", ansiCyan("build"))
	if err := runSetupBuild(loafRoot, out); err != nil {
		return err
	}
	fmt.Fprintln(out)

	fmt.Fprintf(out, "  %s Distributing to detected tools...\n", ansiCyan("install"))
	if err := setupRunner.runInstall([]string{"--to", "all", "--yes"}, out, targetRoot); err != nil {
		return err
	}

	fmt.Fprintf(out, "  %s Setup complete. Run %s in Claude Code to set up your project.\n\n", ansiGreen("✓"), ansiBold("/bootstrap"))
	return nil
}

func parseSetupArgs(args []string) (setupOptions, error) {
	var options setupOptions
	for _, arg := range args {
		switch {
		case arg == "--help" || arg == "-h":
			options.help = true
		case strings.HasPrefix(arg, "-"):
			return setupOptions{}, fmt.Errorf("unknown setup option %q", arg)
		case options.path == "":
			options.path = arg
		default:
			return setupOptions{}, fmt.Errorf("setup accepts at most one path")
		}
	}
	return options, nil
}

func writeSetupHelp(out io.Writer) {
	fmt.Fprintln(out, strings.Join([]string{
		"Usage: loaf setup [path]",
		"",
		"One-step bootstrap: init + build + install.",
		"",
		"Arguments:",
		"  path        Directory to set up (created if it does not exist)",
		"",
		"Options:",
		"  -h, --help  Show help",
	}, "\n"))
}

func prepareSetupTarget(baseRoot string, pathArg string) (string, error) {
	targetRoot := pathArg
	if !filepath.IsAbs(targetRoot) {
		targetRoot = filepath.Join(baseRoot, pathArg)
	}
	targetRoot = filepath.Clean(targetRoot)
	if stat, err := os.Stat(targetRoot); err == nil {
		if !stat.IsDir() {
			return "", fmt.Errorf("path exists but is not a directory: %s", targetRoot)
		}
		return filepath.EvalSymlinks(targetRoot)
	} else if !os.IsNotExist(err) {
		return "", err
	}
	if err := os.MkdirAll(targetRoot, 0o755); err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(targetRoot)
}

func runSetupBuild(loafRoot string, out io.Writer) error {
	cmd := exec.Command("npm", "run", "build")
	cmd.Dir = loafRoot
	cmd.Stdout = out
	cmd.Stderr = out
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("setup build failed: %w", err)
	}
	return nil
}
