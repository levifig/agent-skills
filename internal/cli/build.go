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
	"strconv"
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
	var textFiles []string
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
			textFiles = append(textFiles, path)
		case ".md", ".json", ".yaml", ".yml", ".toml":
			textFiles = append(textFiles, path)
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
	if err := validateNativeBuildHarnessLanguage(root, targetName, textFiles); err != nil {
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

type nativeBuildHarnessLanguageFinding struct {
	path   string
	line   int
	reason string
}

func validateNativeBuildHarnessLanguage(root string, targetName string, paths []string) error {
	var findings []nativeBuildHarnessLanguageFinding
	for _, path := range paths {
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		relative := nativeBuildRelativePath(root, path)
		for lineNumber, line := range strings.Split(string(body), "\n") {
			if hasNativeBuildUnresolvedToken(line) && !nativeBuildAllowedCodexExecutablePlaceholder(targetName, relative, line) {
				findings = append(findings, nativeBuildHarnessLanguageFinding{path: relative, line: lineNumber + 1, reason: "unresolved harness token"})
			}
			if targetName == "claude-code" {
				continue
			}
			for _, forbidden := range nativeBuildNonClaudeForbiddenTerms() {
				if strings.Contains(line, forbidden) && !nativeBuildHarnessLanguageAllowed(targetName, relative, forbidden) {
					findings = append(findings, nativeBuildHarnessLanguageFinding{path: relative, line: lineNumber + 1, reason: "non-Claude output contains " + forbidden})
				}
			}
		}
	}
	if len(findings) == 0 {
		return nil
	}
	var out strings.Builder
	out.WriteString("harness language lint failed:")
	for _, finding := range findings {
		out.WriteString("\n")
		out.WriteString(finding.path)
		out.WriteString(":")
		out.WriteString(strconv.Itoa(finding.line))
		out.WriteString(": ")
		out.WriteString(finding.reason)
	}
	return errors.New(out.String())
}

func hasNativeBuildUnresolvedToken(line string) bool {
	index := strings.Index(line, "{{")
	for index >= 0 {
		if index == 0 || line[index-1] != '$' {
			return true
		}
		next := strings.Index(line[index+2:], "{{")
		if next < 0 {
			return false
		}
		index += next + 2
	}
	return false
}

func nativeBuildAllowedCodexExecutablePlaceholder(targetName string, relative string, line string) bool {
	if targetName != "codex" || filepath.ToSlash(relative) != "dist/codex/.codex/hooks.json" {
		return false
	}
	switch strings.TrimSuffix(strings.TrimSpace(line), ",") {
	case `"command": "{{LOAF_EXECUTABLE}} journal context --from-hook --codex-hook"`,
		`"commandWindows": "{{LOAF_EXECUTABLE}} journal context --from-hook --codex-hook"`:
		return true
	default:
		return false
	}
}

func nativeBuildNonClaudeForbiddenTerms() []string {
	return []string{
		"CLAUDE.md",
		"AskUserQuestionTool",
		"AskUserQuestion",
		"TodoWrite",
		"/loaf:",
		"Task tool",
		"subagent_type",
		"Subagents",
		"subagents",
		"Subagent",
		"subagent",
	}
}

func nativeBuildHarnessLanguageAllowed(targetName string, relativePath string, term string) bool {
	if targetName == "opencode" && term == "subagent" && strings.HasPrefix(relativePath, "dist/opencode/agents/") {
		return true
	}
	return false
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
