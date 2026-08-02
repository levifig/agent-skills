package cli

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

var defaultBuildTargets = []string{"claude-code", "opencode", "cursor", "codex", "amp"}

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
	loafRoot, err := resolveSourceCheckoutRoot(r.WorkingDir, runtimeRoot)
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
	start := time.Now()
	fmt.Fprintf(out, "\n%s\n\n", ansiBold("loaf build"))

	sharedStart := time.Now()
	fmt.Fprintf(out, "  %s shared skills intermediate...", ansiCyan("building"))
	if err := buildNativeSharedSkillsIntermediate(root); err != nil {
		fmt.Fprintf(out, "\r  %s shared skills intermediate\n", ansiRed("✗"))
		return err
	}
	fmt.Fprintf(out, "\r  %s shared skills intermediate %s\n", ansiGreen("✓"), ansiGray("("+elapsedSeconds(sharedStart)+")"))

	targetStart := time.Now()
	fmt.Fprintf(out, "  %s %s...", ansiCyan("building"), targetName)
	warnings, err := buildNativeTargetOnly(root, targetName)
	if err != nil {
		fmt.Fprintf(out, "\r  %s %s\n", ansiRed("✗"), targetName)
		return err
	}
	fmt.Fprintf(out, "\r  %s %s %s\n", ansiGreen("✓"), targetName, ansiGray("("+elapsedSeconds(targetStart)+")"))
	for _, warning := range warnings {
		fmt.Fprintf(out, "    %s %s\n", ansiYellow("warn"), warning)
	}
	fmt.Fprintln(out)
	fmt.Fprintf(out, "%s %s\n", ansiGreen("Build complete"), ansiGray("("+elapsedSeconds(start)+")"))
	return nil
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
		warnings, err := buildNativeTargetOnly(root, targetName)
		if err != nil {
			fmt.Fprintf(out, "\r  %s %s\n", ansiRed("✗"), targetName)
			fmt.Fprintf(out, "    %s\n", ansiRed(err.Error()))
			failed = true
			continue
		}
		fmt.Fprintf(out, "\r  %s %s %s\n", ansiGreen("✓"), targetName, ansiGray("("+elapsedSeconds(targetStart)+")"))
		for _, warning := range warnings {
			fmt.Fprintf(out, "    %s %s\n", ansiYellow("warn"), warning)
		}
	}
	fmt.Fprintln(out)
	if failed {
		return fmt.Errorf("%s %s", ansiRed("Build failed"), ansiGray("("+elapsedSeconds(start)+")"))
	}
	fmt.Fprintf(out, "%s %s\n", ansiGreen("Build complete"), ansiGray("("+elapsedSeconds(start)+")"))
	return nil
}

func buildNativeTargetOnly(root string, targetName string) ([]string, error) {
	var err error
	switch targetName {
	case "claude-code":
		err = buildNativeClaudeCodeTarget(root)
	case "opencode":
		err = buildNativeOpenCodeTarget(root)
	case "cursor":
		err = buildNativeCursorTarget(root)
	case "codex":
		err = buildNativeCodexTarget(root)
	case "amp":
		err = buildNativeAmpTarget(root)
	default:
		return nil, fmt.Errorf("native build target %s is not implemented", targetName)
	}
	if err != nil {
		return nil, err
	}
	if err := writeNativeBuildTargetManifest(root, targetName); err != nil {
		return nil, err
	}
	return validateNativeBuildArtifacts(root, targetName)
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

func validateNativeBuildArtifacts(root string, targetName string) ([]string, error) {
	outputDir := nativeBuildTargetOutputDir(root, targetName)
	var jsFiles []string
	var tsFiles []string
	if err := filepath.WalkDir(outputDir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if entry.Name() == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		switch ext {
		case ".js", ".mjs", ".cjs":
			jsFiles = append(jsFiles, path)
		case ".ts", ".mts", ".cts":
			tsFiles = append(tsFiles, path)
		}
		return nil
	}); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	for _, path := range jsFiles {
		if err := runNativeBuildArtifactCheck("node", []string{"--check", path}); err != nil {
			return nil, fmt.Errorf("JavaScript validation failed for %s: %w", nativeBuildRelativePath(root, path), err)
		}
	}
	if err := validateNativeBuildUnresolvedPlaceholders(root, targetName); err != nil {
		return nil, err
	}
	if len(tsFiles) == 0 {
		return nil, nil
	}
	files := nativeBuildRelativePaths(root, tsFiles)
	if !nativeBuildShouldValidateTypeScript() {
		return []string{"TypeScript validation skipped outside CI; set LOAF_VALIDATE_TYPESCRIPT=1 to check " + strings.Join(files, ", ")}, nil
	}
	if _, err := exec.LookPath("tsc"); err != nil {
		if nativeBuildIsCI() {
			return nil, fmt.Errorf("TypeScript validation requires tsc in CI for %s", strings.Join(files, ", "))
		}
		message := "TypeScript validation skipped; tsc not found for " + strings.Join(files, ", ")
		return []string{message}, nil
	}
	ambientTypes, cleanup, err := writeNativeBuildTypeScriptAmbientTypes()
	if err != nil {
		return nil, err
	}
	defer cleanup()

	args := []string{"--noEmit", "--allowJs", "false", "--skipLibCheck", "--module", "ES2022", "--moduleResolution", "Bundler", "--target", "ES2022", ambientTypes}
	args = append(args, tsFiles...)
	if err := runNativeBuildArtifactCheck("tsc", args); err != nil {
		return nil, fmt.Errorf("TypeScript validation failed: %w", err)
	}
	return nil, nil
}

func writeNativeBuildTypeScriptAmbientTypes() (string, func(), error) {
	dir, err := os.MkdirTemp("", "loaf-build-ts-*")
	if err != nil {
		return "", func() {}, err
	}
	cleanup := func() {
		_ = os.RemoveAll(dir)
	}
	path := filepath.Join(dir, "generated-artifact-env.d.ts")
	if err := os.WriteFile(path, []byte(nativeBuildTypeScriptAmbientTypes()), 0o644); err != nil {
		cleanup()
		return "", func() {}, err
	}
	return path, cleanup, nil
}

func nativeBuildTypeScriptAmbientTypes() string {
	return `declare const process: {
  env: Record<string, string | undefined>;
  cwd(): string;
};

declare const console: {
  error(...args: unknown[]): void;
  log(...args: unknown[]): void;
  warn(...args: unknown[]): void;
};

declare module 'child_process' {
  export interface ExecFileOptions {
    cwd?: string;
    env?: Record<string, string | undefined>;
    encoding?: string;
    timeout?: number;
  }

  export interface WritableStreamLike {
    write(data: string): void;
    end(): void;
  }

  export interface ReadableStreamLike {
    on(event: 'data', listener: (data: string) => void): void;
  }

  export interface ChildProcessLike {
    stdin?: WritableStreamLike;
    stdout?: ReadableStreamLike;
    stderr?: ReadableStreamLike;
    on(event: 'close', listener: (code: number | null) => void): void;
    on(event: 'error', listener: (err: Error) => void): void;
  }

  export function execFile(file: string, args: string[], options?: ExecFileOptions): ChildProcessLike;
}

declare module 'util' {
  export function promisify(fn: (...args: any[]) => any): (...args: any[]) => Promise<any>;
}

declare module 'path' {
  export function dirname(path: string): string;
  export function join(...paths: string[]): string;
}

declare module 'url' {
  export function fileURLToPath(url: string | { href: string }): string;
}

declare module '@ampcode/plugin' {
  export interface ToolCallEvent {
    toolUseID: string;
    tool: string;
    input: Record<string, unknown>;
    thread: { id: string };
  }

  export interface ToolResultEvent extends ToolCallEvent {
    status: 'done' | 'error' | 'cancelled';
    error?: string;
    output?: unknown;
  }

  export interface ShellCommand {
    command: string;
    dir?: string;
  }

  export type ToolCallResult =
    | { action: 'allow' }
    | { action: 'reject-and-continue'; message: string };

  export interface PluginAPI {
    helpers: {
      shellCommandFromToolCall(event: ToolCallEvent): ShellCommand | null;
    };
    on(event: 'tool.call', handler: (event: ToolCallEvent) => ToolCallResult | Promise<ToolCallResult>): void;
    on(event: 'tool.result', handler: (event: ToolResultEvent) => void | Promise<void>): void;
  }
}
`
}

// nativeBuildInstallTimePlaceholders are {{TOKEN}} forms that generated non-skill
// artifacts may still carry at build time because install resolves them once the
// trusted absolute Loaf executable is known. Any other {{TOKEN}} in those
// artifacts is a stray placeholder and fails the build.
var nativeBuildInstallTimePlaceholders = map[string]bool{
	"{{LOAF_EXECUTABLE}}":  true,
	"{{LOAF_BASIC_RULES}}": true,
}

type nativeBuildUnresolvedPlaceholderFinding struct {
	path  string
	line  int
	token string
}

// validateNativeBuildUnresolvedPlaceholders rejects unresolved {{TOKEN}} forms in
// generated non-skill artifacts. Skills/, commands/, and agents/ still carry
// retired content tokens until TASK-003, so those trees are skipped. Everything
// else under the target output — including .ts plugins, shell/Python hooks, and
// instruction Markdown — is scanned. The root adapter manifest is excluded by
// exact path (not basename) so a nested copy named .loaf-target-manifest.json
// cannot hide unresolved tokens.
func validateNativeBuildUnresolvedPlaceholders(root string, targetName string) error {
	outputDir := nativeBuildTargetOutputDir(root, targetName)
	manifestPath := filepath.Join(outputDir, targetBuildManifestFile)
	var paths []string
	err := filepath.WalkDir(outputDir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			name := entry.Name()
			// Skills and OpenCode commands still carry retired content tokens until
			// TASK-003. Agents are authored profiles, not the generated Codex/config
			// artifacts this guard targets. bin/ holds shipped native binaries whose
			// bytes are not text and would false-positive on {{ spans.
			if name == "node_modules" || name == "skills" || name == "commands" || name == "agents" || name == "bin" {
				return filepath.SkipDir
			}
			return nil
		}
		paths = append(paths, path)
		return nil
	})
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var findings []nativeBuildUnresolvedPlaceholderFinding
	for _, path := range paths {
		if path == manifestPath {
			continue
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		// Skip opaque binaries that slipped outside bin/ (NUL is never valid in
		// the text artifacts this lint targets).
		if bytes.IndexByte(body, 0) >= 0 {
			continue
		}
		relative := nativeBuildRelativePath(root, path)
		for lineNumber, line := range strings.Split(string(body), "\n") {
			for _, token := range nativeBuildUnresolvedTokensInLine(line) {
				if nativeBuildInstallTimePlaceholders[token] {
					continue
				}
				findings = append(findings, nativeBuildUnresolvedPlaceholderFinding{
					path:  relative,
					line:  lineNumber + 1,
					token: token,
				})
			}
		}
	}
	if len(findings) == 0 {
		return nil
	}
	var out strings.Builder
	out.WriteString("unresolved placeholder lint failed:")
	for _, finding := range findings {
		out.WriteString("\n")
		out.WriteString(finding.path)
		out.WriteString(":")
		out.WriteString(fmt.Sprintf("%d", finding.line))
		out.WriteString(": unresolved harness token ")
		out.WriteString(finding.token)
	}
	return errors.New(out.String())
}

// nativeBuildUnresolvedTokensInLine returns every {{...}} span on the line.
// A leading $ does not exempt a token: GitHub Actions ${{ github.* }} forms live
// only under skipped skill trees, and "${{ARBITRARY}}" must not bypass the guard
// in generated artifacts.
func nativeBuildUnresolvedTokensInLine(line string) []string {
	var tokens []string
	remaining := line
	for {
		start := strings.Index(remaining, "{{")
		if start < 0 {
			return tokens
		}
		end := strings.Index(remaining[start+2:], "}}")
		if end < 0 {
			return tokens
		}
		token := remaining[start : start+2+end+2]
		tokens = append(tokens, token)
		remaining = remaining[start+len(token):]
	}
}

func nativeBuildTargetOutputDir(root string, targetName string) string {
	if targetName == "claude-code" {
		return filepath.Join(root, "plugins", "loaf")
	}
	return filepath.Join(root, "dist", targetName)
}

func runNativeBuildArtifactCheck(name string, args []string) error {
	cmd := exec.Command(name, args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		detail := strings.TrimSpace(strings.TrimSpace(stderr.String()) + "\n" + strings.TrimSpace(stdout.String()))
		if detail == "" {
			return err
		}
		return fmt.Errorf("%w: %s", err, detail)
	}
	return nil
}

func nativeBuildIsCI() bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv("CI")))
	return value != "" && value != "0" && value != "false"
}

func nativeBuildShouldValidateTypeScript() bool {
	if nativeBuildIsCI() {
		return true
	}
	value := strings.ToLower(strings.TrimSpace(os.Getenv("LOAF_VALIDATE_TYPESCRIPT")))
	return value != "" && value != "0" && value != "false"
}

func nativeBuildRelativePaths(root string, paths []string) []string {
	relative := make([]string, 0, len(paths))
	for _, path := range paths {
		relative = append(relative, nativeBuildRelativePath(root, path))
	}
	return relative
}

func nativeBuildRelativePath(root string, path string) string {
	if rel, err := filepath.Rel(root, path); err == nil {
		return filepath.ToSlash(rel)
	}
	return filepath.ToSlash(path)
}
