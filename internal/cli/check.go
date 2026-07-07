package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/levifig/loaf/internal/state"
)

type checkOptions struct {
	hook       string
	jsonOutput bool
	advisory   bool
}

type checkHookContext struct {
	Tool struct {
		Name  string         `json:"name"`
		Input checkHookInput `json:"input"`
	} `json:"tool"`
	ToolName        string         `json:"tool_name"`
	ToolInput       checkHookInput `json:"tool_input"`
	Input           checkHookInput `json:"input"`
	AgentType       string         `json:"agent_type"`
	ValidationLevel string         `json:"validation_level"`
}

type checkHookInput struct {
	Command   string `json:"command"`
	FilePath  string `json:"file_path"`
	Content   string `json:"content"`
	OldString string `json:"old_string"`
	NewString string `json:"new_string"`
}

type checkResult struct {
	Passed   bool     `json:"-"`
	Blocked  bool     `json:"-"`
	Warnings []string `json:"-"`
	Errors   []string `json:"-"`
	Findings []string `json:"-"`
}

type checkJSONOutput struct {
	Hook     string   `json:"hook"`
	Passed   bool     `json:"passed"`
	Blocked  bool     `json:"blocked"`
	Advisory bool     `json:"advisory,omitempty"`
	ExitCode int      `json:"exitCode"`
	Warnings []string `json:"warnings"`
	Errors   []string `json:"errors"`
	Findings []string `json:"findings,omitempty"`
}

type secretPattern struct {
	name string
	re   *regexp.Regexp
}

var nativeCheckSecretPatterns = []secretPattern{
	{name: "AWS Access Key ID", re: regexp.MustCompile(`AKIA[0-9A-Z]{16}`)},
	{name: "AWS Secret Key", re: regexp.MustCompile(`(?i)aws_secret_access_key\s*=\s*["']?[A-Za-z0-9/+=]{40}["']?`)},
	{name: "OpenAI API Key", re: regexp.MustCompile(`sk-[a-zA-Z0-9]{20,}`)},
	{name: "Stripe Live Key", re: regexp.MustCompile(`sk_live_[a-zA-Z0-9]{10,}`)},
	{name: "Stripe Test Key", re: regexp.MustCompile(`sk_test_[a-zA-Z0-9]{10,}`)},
	{name: "Private Key", re: regexp.MustCompile(`-----BEGIN (?:RSA |EC |DSA |OPENSSH )?PRIVATE KEY-----`)},
	{name: "Database Connection", re: regexp.MustCompile(`(postgres|mysql|mongodb)://[^:]+:[^@]+@`)},
	{name: "Password Assignment", re: regexp.MustCompile(`(?i)password\s*=\s*["'][^"']{8,}["']`)},
	{name: "Secret Assignment", re: regexp.MustCompile(`(?i)secret\s*=\s*["'][^"']{8,}["']`)},
	{name: "API Key Assignment", re: regexp.MustCompile(`(?i)api_key\s*=\s*["'][^"']{16,}["']`)},
	{name: "JWT Token", re: regexp.MustCompile(`eyJ[A-Za-z0-9-_]+\.eyJ[A-Za-z0-9-_]+\.[A-Za-z0-9-_.+/=]*`)},
	{name: "GitHub Token", re: regexp.MustCompile(`gh[pousr]_[A-Za-z0-9_]{36}`)},
}

var validCheckHooks = map[string]bool{
	"artifact-body-write":  true,
	"check-secrets":        true,
	"ephemeral-provenance": true,
	"github-account":       true,
	"render-drift":         true,
	"validate-push":        true,
	"workflow-pre-pr":      true,
	"validate-commit":      true,
	"security-audit":       true,
}

func (r Runner) runCheck(args []string, out io.Writer, runtimeRoot string) error {
	if isHelpArg(args) {
		writeCheckHelp(out)
		return nil
	}
	options, err := parseCheckArgs(args)
	if err != nil {
		return err
	}
	if options.hook == "" {
		return fmt.Errorf("--hook <id> is required")
	}
	if !validCheckHooks[options.hook] {
		return fmt.Errorf("Unknown hook: %s", options.hook)
	}
	context := r.readCheckContext()
	var result checkResult
	switch options.hook {
	case "artifact-body-write":
		result = runNativeArtifactBodyWriteGuard(context)
	case "check-secrets":
		result = runNativeCheckSecrets(context)
	case "ephemeral-provenance":
		result = runNativeEphemeralProvenance(context, runtimeRoot)
	case "github-account":
		result = runNativeGitHubAccount(context, runtimeRoot)
	case "render-drift":
		result = runNativeRenderDrift(context, runtimeRoot)
	case "validate-commit":
		result = runNativeValidateCommit(context, runtimeRoot)
	case "security-audit":
		result = runNativeSecurityAudit(context, runtimeRoot)
	case "workflow-pre-pr":
		result = runNativeWorkflowPrePR(context, runtimeRoot)
	case "validate-push":
		result = runNativeValidatePush(context, runtimeRoot)
	default:
		return fmt.Errorf("Unknown hook: %s", options.hook)
	}
	if options.jsonOutput {
		if err := writeCheckJSON(out, options.hook, result, options.advisory); err != nil {
			return err
		}
	} else {
		writeCheckText(out, firstWriter(r.Stderr, os.Stderr), options.hook, result, options.advisory)
	}
	if result.Blocked && !options.advisory {
		return ExitError{Code: 2}
	}
	return nil
}

func writeCheckHelp(out io.Writer) {
	writeUsageHelp(out, "loaf check --hook <id> [--advisory] [--json]", "Run one registered hook check.", "--hook      Hook id: artifact-body-write, check-secrets, ephemeral-provenance, github-account, render-drift, validate-commit, security-audit, workflow-pre-pr, validate-push", "--advisory  Surface findings without blocking: always exit 0, even when the check fails", "--json      Output hook result, pass/block status, exit code, warnings, errors, and findings as JSON")
}

func parseCheckArgs(args []string) (checkOptions, error) {
	var options checkOptions
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--hook":
			value, err := consumeFlagValue(args, &i, "--hook")
			if err != nil {
				return checkOptions{}, err
			}
			options.hook = value
		case "--json":
			options.jsonOutput = true
		case "--advisory":
			options.advisory = true
		default:
			return checkOptions{}, fmt.Errorf("unknown check option %q", args[i])
		}
	}
	return options, nil
}

func (r Runner) readCheckContext() checkHookContext {
	reader := firstReader(r.Stdin, os.Stdin)
	if file, ok := reader.(*os.File); ok {
		if info, err := file.Stat(); err == nil && info.Mode()&os.ModeCharDevice != 0 {
			return checkHookContext{}
		}
	}
	body, err := io.ReadAll(reader)
	if err != nil || len(strings.TrimSpace(string(body))) == 0 {
		return checkHookContext{}
	}
	var context checkHookContext
	if err := json.Unmarshal(body, &context); err != nil {
		return checkHookContext{}
	}
	return context
}

func runNativeCheckSecrets(context checkHookContext) checkResult {
	result := checkResult{Passed: true, Warnings: []string{}, Errors: []string{}, Findings: []string{}}
	filePath := checkContextFilePath(context)
	contentToScan := checkContextContent(context)
	if checkContextToolName(context) == "Bash" && checkContextCommand(context) != "" {
		contentToScan += "\n" + checkContextCommand(context)
	}
	if filePath == "" && strings.TrimSpace(contentToScan) == "" {
		return result
	}
	for _, pattern := range nativeCheckSecretPatterns {
		for _, match := range pattern.re.FindAllString(contentToScan, -1) {
			preview := match
			if len(preview) > 40 {
				preview = preview[:40]
			}
			result.Findings = append(result.Findings, fmt.Sprintf("%s: %s...", pattern.name, preview))
		}
	}
	if len(result.Findings) > 0 {
		result.Passed = false
		result.Blocked = true
		result.Errors = append(result.Errors, fmt.Sprintf("Potential secrets detected in %s", firstNonEmpty(filePath, "input")))
	}
	return result
}

type artifactBodyWriteTarget struct {
	path    string
	kind    string
	command string
	content string
}

var artifactBodyPathDirs = map[string]string{
	"brainstorms": "brainstorm",
	"councils":    "council",
	"drafts":      "draft",
	"handoffs":    "handoff",
	"ideas":       "idea",
	"plans":       "plan",
	"reports":     "report",
	"specs":       "spec",
	"tasks":       "task",
}

var artifactBodyWriteBashPathRE = regexp.MustCompile(`(?:^|[\s"'=])(\.agents/(?:brainstorms|councils|drafts|handoffs|ideas|plans|reports|specs|tasks)(?:/archive)?/[^\s"'` + "`" + `;|&<>]+\.md)`)

func runNativeArtifactBodyWriteGuard(context checkHookContext) checkResult {
	result := checkResult{Passed: true, Warnings: []string{}, Errors: []string{}, Findings: []string{}}
	for _, target := range artifactBodyWriteTargets(context) {
		if artifactBodyWriteAllowed(target.path, target.content) {
			continue
		}
		result.Passed = false
		result.Blocked = true
		result.Errors = append(result.Errors, fmt.Sprintf("Direct artifact body write blocked for %s", target.path))
		result.Errors = append(result.Errors, fmt.Sprintf("Use `%s` so the SQLite artifact body is registered.", target.command))
		result.Findings = append(result.Findings, fmt.Sprintf("%s is a body-capable %s artifact path", target.path, target.kind))
	}
	return result
}

var gitPushCommandRE = regexp.MustCompile(`^git\s+push(?:\s|$)`)

func shouldRunPushScopedCheck(context checkHookContext) bool {
	tool := checkContextToolName(context)
	command := strings.TrimSpace(checkContextCommand(context))
	if tool == "" && command == "" {
		return true
	}
	if tool != "Bash" {
		return false
	}
	return gitPushCommandRE.MatchString(command)
}

func runNativeRenderDrift(context checkHookContext, cwd string) checkResult {
	result := checkResult{Passed: true, Warnings: []string{}, Errors: []string{}, Findings: []string{}}
	if !shouldRunPushScopedCheck(context) {
		return result
	}
	root := firstNonEmpty(strings.TrimSpace(cwd), ".")
	for _, dir := range []string{filepath.Join(root, ".agents", "specs"), filepath.Join(root, ".agents", "reports")} {
		entries, err := os.ReadDir(dir)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			result.Passed = false
			result.Blocked = true
			result.Errors = append(result.Errors, fmt.Sprintf("Read render directory %s: %v", dir, err))
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
				continue
			}
			path := filepath.Join(dir, entry.Name())
			content, err := os.ReadFile(path)
			if err != nil {
				result.Passed = false
				result.Blocked = true
				result.Errors = append(result.Errors, fmt.Sprintf("Read durable render %s: %v", renderDriftRelativePath(root, path), err))
				continue
			}
			if !hasFinalDurableRenderStamp(string(content)) {
				continue
			}
			rerendered, err := state.ReRenderDurableRender(string(content))
			if err != nil {
				result.Passed = false
				result.Blocked = true
				result.Errors = append(result.Errors, fmt.Sprintf("Durable render %s is invalid: %v", renderDriftRelativePath(root, path), err))
				result.Findings = append(result.Findings, "Edit via `loaf <entity> edit`, then run `loaf <entity> finalize`.")
				continue
			}
			if rerendered != string(content) {
				result.Passed = false
				result.Blocked = true
				result.Errors = append(result.Errors, fmt.Sprintf("Durable render drift detected in %s", renderDriftRelativePath(root, path)))
				result.Findings = append(result.Findings, "Committed render is not byte-identical to its deterministic self-render.")
				result.Findings = append(result.Findings, "Edit via `loaf <entity> edit`, then run `loaf <entity> finalize`.")
			}
		}
	}
	return result
}

var ephemeralProvenanceRefRE = regexp.MustCompile(`\.agents/(?:tasks|ideas|sparks|sessions|brainstorms|drafts)/(?:archive/)?[A-Za-z0-9][^\s)'"]*|\.agents/TASKS\.json`)

func runNativeEphemeralProvenance(context checkHookContext, cwd string) checkResult {
	result := checkResult{Passed: true, Warnings: []string{}, Errors: []string{}, Findings: []string{}}
	if !shouldRunPushScopedCheck(context) {
		return result
	}
	root := firstNonEmpty(strings.TrimSpace(cwd), ".")
	tracked, err := trackedEphemeralMarkdownPaths(root)
	if err != nil {
		result.Passed = false
		result.Blocked = true
		result.Errors = append(result.Errors, fmt.Sprintf("Check tracked ephemeral files: %v", err))
		return result
	}
	if len(tracked) > 0 {
		result.Passed = false
		result.Blocked = true
		result.Errors = append(result.Errors, "Tracked ephemeral Markdown is not allowed after SQLite cutover")
		result.Findings = append(result.Findings, tracked...)
		return result
	}

	specsDir := filepath.Join(root, ".agents", "specs")
	entries, err := os.ReadDir(specsDir)
	if os.IsNotExist(err) {
		return result
	}
	if err != nil {
		result.Passed = false
		result.Blocked = true
		result.Errors = append(result.Errors, fmt.Sprintf("Read active specs: %v", err))
		return result
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") || strings.HasPrefix(entry.Name(), "SPEC-045-") {
			continue
		}
		path := filepath.Join(specsDir, entry.Name())
		content, err := os.ReadFile(path)
		if err != nil {
			result.Passed = false
			result.Blocked = true
			result.Errors = append(result.Errors, fmt.Sprintf("Read active spec %s: %v", renderDriftRelativePath(root, path), err))
			continue
		}
		for lineNumber, line := range strings.Split(string(content), "\n") {
			if ephemeralProvenanceRefRE.MatchString(line) {
				result.Passed = false
				result.Blocked = true
				result.Findings = append(result.Findings, fmt.Sprintf("%s:%d: %s", renderDriftRelativePath(root, path), lineNumber+1, strings.TrimSpace(line)))
			}
		}
	}
	if len(result.Findings) > 0 {
		result.Errors = append(result.Errors, "Dangling active-spec provenance points at ephemeral Markdown slated for SQLite cutover")
	}
	return result
}

func trackedEphemeralMarkdownPaths(root string) ([]string, error) {
	args := []string{"-C", root, "ls-files", "--", ".agents/TASKS.json", ".agents/tasks", ".agents/ideas", ".agents/sparks", ".agents/sessions", ".agents/brainstorms", ".agents/drafts"}
	output, err := exec.Command("git", args...).Output()
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		paths = append(paths, filepath.ToSlash(line))
	}
	return paths, nil
}

func hasFinalDurableRenderStamp(content string) bool {
	lines := strings.Split(strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(content, "\r\n", "\n"), "\r", "\n")), "\n")
	if len(lines) == 0 {
		return false
	}
	return strings.HasPrefix(strings.TrimSpace(lines[len(lines)-1]), "<!-- loaf:render ")
}

func renderDriftRelativePath(root string, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}

func artifactBodyWriteTargets(context checkHookContext) []artifactBodyWriteTarget {
	tool := checkContextToolName(context)
	if tool == "Bash" {
		return artifactBodyWriteBashTargets(checkContextCommand(context))
	}
	if tool != "Edit" && tool != "Write" && tool != "MultiEdit" {
		return nil
	}
	path := checkContextFilePath(context)
	kind, ok := artifactBodyPathKind(path)
	if !ok {
		return nil
	}
	path = artifactBodyRelativePath(path)
	return []artifactBodyWriteTarget{{
		path:    path,
		kind:    kind,
		command: artifactBodyWriteCommand(kind, path),
		content: checkContextContent(context),
	}}
}

func artifactBodyWriteBashTargets(command string) []artifactBodyWriteTarget {
	if strings.TrimSpace(command) == "" || bashCommandUsesLoafCLI(command) {
		return nil
	}
	var targets []artifactBodyWriteTarget
	seen := map[string]bool{}
	for _, match := range artifactBodyWriteBashPathRE.FindAllStringSubmatch(command, -1) {
		if len(match) != 2 || seen[match[1]] || !bashCommandWritesPath(command, match[1]) {
			continue
		}
		seen[match[1]] = true
		kind, ok := artifactBodyPathKind(match[1])
		if !ok {
			continue
		}
		targets = append(targets, artifactBodyWriteTarget{
			path:    artifactBodyRelativePath(match[1]),
			kind:    kind,
			command: artifactBodyWriteCommand(kind, match[1]),
			content: command,
		})
	}
	return targets
}

func artifactBodyPathKind(path string) (string, bool) {
	path = artifactBodyRelativePath(path)
	if !strings.HasSuffix(path, ".md") || strings.Contains(path, "/templates/") {
		return "", false
	}
	parts := strings.Split(path, "/")
	if len(parts) < 3 || parts[0] != ".agents" {
		return "", false
	}
	kind, ok := artifactBodyPathDirs[parts[1]]
	return kind, ok
}

func normalizeHookPath(path string) string {
	path = strings.TrimSpace(path)
	path = strings.Trim(path, `"'`)
	path = filepath.ToSlash(filepath.Clean(path))
	if path == "." {
		return ""
	}
	return path
}

func artifactBodyRelativePath(path string) string {
	path = normalizeHookPath(path)
	if strings.HasPrefix(path, "./") {
		path = strings.TrimPrefix(path, "./")
	}
	if strings.HasPrefix(path, "/") {
		if idx := strings.Index(path, "/.agents/"); idx >= 0 {
			return strings.TrimPrefix(path[idx+1:], "/")
		}
	}
	return path
}

func artifactBodyWriteAllowed(path string, content string) bool {
	path = artifactBodyRelativePath(path)
	if path == "" || !strings.HasPrefix(path, ".agents/") {
		return true
	}
	if artifactBodyWriteIsGeneratedRender(content) {
		return true
	}
	return artifactBodyWriteIsMetadataOnly(content)
}

func artifactBodyWriteIsGeneratedRender(content string) bool {
	lower := strings.ToLower(content)
	for _, marker := range []string{
		"<!-- loaf:render ",
		"<!-- loaf:rendered",
		"loaf_renderer_version:",
		"renderer_contract_version:",
		"renderer-contract-version:",
	} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func artifactBodyWriteIsMetadataOnly(content string) bool {
	content = strings.TrimSpace(content)
	if content == "" {
		return true
	}
	if strings.HasPrefix(content, "---") {
		_, body := splitMarkdownDocument(content)
		return strings.TrimSpace(body) == ""
	}
	hasMetadataKey := regexp.MustCompile(`(?m)^\s*[A-Za-z0-9_-]+:\s*`).MatchString(content)
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line == "---" {
			continue
		}
		if strings.HasPrefix(line, "- ") {
			if hasMetadataKey {
				continue
			}
			return false
		}
		if !regexp.MustCompile(`^[A-Za-z0-9_-]+:\s*.*$`).MatchString(line) {
			return false
		}
	}
	return true
}

func bashCommandUsesLoafCLI(command string) bool {
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return false
	}
	base := filepath.Base(fields[0])
	return base == "loaf" || (len(fields) > 1 && (fields[0] == "bin/loaf" || fields[0] == "./bin/loaf"))
}

func bashCommandWritesPath(command string, path string) bool {
	if strings.Contains(command, "> "+path) || strings.Contains(command, ">"+path) || strings.Contains(command, "tee "+path) || strings.Contains(command, "tee -a "+path) {
		return true
	}
	for _, writeVerb := range []string{"sed -i", "perl -pi", "python -c", "python3 -c", "node -e"} {
		if strings.Contains(command, writeVerb) && strings.Contains(command, path) {
			return true
		}
	}
	return false
}

func artifactBodyWriteCommand(kind string, path string) string {
	ref := artifactBodyRefFromPath(path)
	switch kind {
	case "brainstorm", "draft":
		return "loaf brainstorm capture --title <title> --body-file <path>"
	case "council":
		return "loaf council new --title <title> --body-file <path>"
	case "handoff":
		return "loaf handoff new --title <title> --body-file <path>"
	case "idea":
		return "loaf idea capture --title <title> --body-file <path>"
	case "plan":
		return "loaf plan new --title <title> --body-file <path>"
	case "report":
		return "loaf report create <slug> --body-file <path>"
	case "spec":
		return "loaf spec show " + firstNonEmpty(ref, "<ref>") + " --json"
	case "task":
		return "loaf task update " + firstNonEmpty(ref, "<ref>") + " --status <status>"
	default:
		return "loaf <entity> <verb> --body-file <path>"
	}
}

func artifactBodyRefFromPath(path string) string {
	stem := strings.TrimSuffix(filepath.Base(normalizeHookPath(path)), ".md")
	if matches := regexp.MustCompile(`^(SPEC-\d+|TASK-\d+)`).FindStringSubmatch(stem); len(matches) == 2 {
		return matches[1]
	}
	return stem
}

var conventionalCommitRE = regexp.MustCompile(`^(feat|fix|docs|style|refactor|perf|test|chore|ci|build|revert)!?: .+`)
var commitMessageFlagRE = regexp.MustCompile(`(?s)-m(?:\s+|=)(?:"([^"]+)"|'([^']+)'|([^\s"']+))`)
var commitMessageHeredocStartRE = regexp.MustCompile(`<<'?([A-Za-z0-9_]+)'?\s*\n`)
var releaseCommitSubjectRE = regexp.MustCompile(`^chore: release v\d+\.\d+\.\d+(?:-[a-zA-Z0-9.-]+)?(?:\+[a-zA-Z0-9.-]+)?(?:\s+\(#\d+\))?$`)

var aiAttributionPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?im)^\s*co-authored-by:\s*[^\n]*\b(?:claude|gpt|copilot|chatgpt|gemini|anthropic|openai)\b`),
	regexp.MustCompile(`(?i)\b(?:generated|created|authored|written|produced)\s+(?:by|with|using)\s+[^\n]*\b(?:claude|gpt|copilot|chatgpt|gemini|anthropic|openai)\b`),
	regexp.MustCompile(`(?i)🤖[^\n]*\b(?:claude|gpt|copilot|chatgpt|gemini|anthropic|openai)\b`),
}

var exemptBuildSubjectPatterns = []*regexp.Regexp{
	releaseCommitSubjectRE,
	regexp.MustCompile(`^chore(?:\(.+\))?: build\b`),
	regexp.MustCompile(`^chore(?:\(.+\))?: deps\b`),
	regexp.MustCompile(`^chore(?:\(.+\))?: lockfile\b`),
}

var buildOutputPathPrefixes = []string{"plugins/", "dist/", ".claude-plugin/"}
var rootLockfiles = map[string]bool{
	"package-lock.json": true,
	"yarn.lock":         true,
	"pnpm-lock.yaml":    true,
	"bun.lock":          true,
	"bun.lockb":         true,
}

func runNativeValidateCommit(context checkHookContext, cwd string) checkResult {
	result := checkResult{Passed: true, Warnings: []string{}, Errors: []string{}, Findings: []string{}}
	command := checkContextCommand(context)
	if checkContextToolName(context) != "Bash" || !strings.Contains(command, "git commit") {
		return result
	}
	if strings.Contains(command, "--amend") && !strings.Contains(command, "-m") {
		return result
	}
	if strings.Contains(command, "--no-edit") || strings.Contains(command, "git merge") {
		return result
	}
	if regexp.MustCompile(`(?:^|\s)(?:-F|--file)\s`).MatchString(command) {
		return result
	}
	message := extractCommitMessage(command)
	if message == "" {
		return result
	}
	if !conventionalCommitRE.MatchString(message) {
		result.Passed = false
		result.Blocked = true
		result.Errors = append(result.Errors,
			"Commit message does not follow Conventional Commits format",
			"Expected format: <type>: <description> (scoped commits not allowed)",
			"Valid types: feat, fix, docs, style, refactor, perf, test, chore, ci, build, revert",
			fmt.Sprintf("Your message: %q", message),
		)
	}
	for _, pattern := range aiAttributionPatterns {
		if pattern.MatchString(message) {
			result.Passed = false
			result.Blocked = true
			result.Errors = append(result.Errors, "Commit message contains AI attribution (trailer, attribution verb, or bot footer). Remove the attribution before committing.")
			break
		}
	}
	subjectLine := strings.Split(message, "\n")[0]
	if len(subjectLine) > 72 {
		result.Warnings = append(result.Warnings, fmt.Sprintf("Subject line is %d characters (recommended: <=72)", len(subjectLine)))
	}
	if !result.Blocked {
		leaked := detectBundledArtifactLeak(cwd, subjectLine)
		if len(leaked) > 0 {
			result.Passed = false
			result.Blocked = true
			result.Errors = append(result.Errors, buildLeakBlockMessage(leaked)...)
		}
	}
	return result
}

type dangerousCommandPattern struct {
	name     string
	re       *regexp.Regexp
	critical bool
}

var dangerousCommandPatterns = []dangerousCommandPattern{
	{name: "Dangerous rm -rf", re: regexp.MustCompile(`rm\s+-rf?\s+/|rm\s+-rf?\s+\*+`), critical: true},
	{name: "chmod 777 (world-writable)", re: regexp.MustCompile(`chmod\s+.*777`), critical: true},
	{name: "eval of untrusted input", re: regexp.MustCompile("eval\\s*\\$|eval\\s*`"), critical: true},
	{name: "Unsafe curl to bash", re: regexp.MustCompile(`curl.*\|\s*(ba)?sh`), critical: true},
	{name: "wget to shell", re: regexp.MustCompile(`wget.*\|\s*(ba)?sh`), critical: true},
	{name: "sudo without validation", re: regexp.MustCompile(`sudo\s+(rm|dd|mkfs|fdisk|format)`), critical: false},
	{name: "Hardcoded sudo password", re: regexp.MustCompile(`echo\s+['"].*['"]\s*\|\s*sudo`), critical: true},
	{name: "Unsafe find exec", re: regexp.MustCompile(`find\s+.*-exec\s+(rm|mv|cp)\s*\{\}`), critical: false},
	{name: "SQL injection risk in command", re: regexp.MustCompile(`mysql.*-e\s*.*['"]\s*\$`), critical: false},
}

var trivialCommandRE = regexp.MustCompile(`^\s*(ls|echo|pwd|whoami|date|cat|head|tail|wc|true|false)\b`)

func runNativeSecurityAudit(context checkHookContext, cwd string) checkResult {
	result := checkResult{Passed: true, Warnings: []string{}, Errors: []string{}, Findings: []string{}}
	if checkContextToolName(context) != "Bash" {
		return result
	}
	command := checkContextCommand(context)
	var criticalFindings []string
	var warningFindings []string
	for _, pattern := range dangerousCommandPatterns {
		if pattern.re.MatchString(command) {
			finding := pattern.name + ": matched pattern in command"
			if pattern.critical {
				criticalFindings = append(criticalFindings, finding)
			} else {
				warningFindings = append(warningFindings, finding)
			}
		}
	}
	if len(criticalFindings) > 0 {
		result.Passed = false
		result.Blocked = true
		result.Errors = append(result.Errors, "Critical security issues detected in command")
		result.Findings = append(result.Findings, criticalFindings...)
		result.Findings = append(result.Findings, warningFindings...)
	} else if len(warningFindings) > 0 {
		result.Warnings = append(result.Warnings, warningFindings...)
	}

	if shouldRunSecurityScanners(context) && !trivialCommandRE.MatchString(command) {
		scanner := runSecurityScanners(cwd)
		if !scanner.anyAvailable {
			result.Warnings = append(result.Warnings, "No vulnerability scanners found (trivy, semgrep, npm audit). Install for deeper security coverage.")
		}
		if len(scanner.criticalFindings) > 0 && !result.Blocked {
			result.Passed = false
			result.Blocked = true
			result.Errors = append(result.Errors, "Critical vulnerabilities detected by security scanners")
			result.Findings = append(result.Findings, scanner.criticalFindings...)
			result.Findings = append(result.Findings, scanner.warningFindings...)
		} else {
			for _, warning := range scanner.warningFindings {
				if !stringSliceContains(result.Warnings, warning) {
					result.Warnings = append(result.Warnings, warning)
				}
			}
		}
	}
	return result
}

var unreleasedHeadingRE = regexp.MustCompile(`(?m)^##\s*\[Unreleased\]`)
var changelogHeadingRE = regexp.MustCompile(`(?m)^##\s`)
var unreleasedStubRE = regexp.MustCompile(`^\s*[-*]\s*_No unreleased changes(?: yet| since v[^.]+(?:\.[^.]+){0,2}(?:-[^.]+)?)\._\s*$`)
var prTitleFlagRE = regexp.MustCompile(`--title(?:\s+|=)(?:"([^"]+)"|'([^']+)'|([^\s"']+))`)
var conventionalPRTitleRE = regexp.MustCompile(`(?i)^(feat|fix|docs|style|refactor|perf|test|chore|ci|build|revert)(\(.+\))?!?: .+`)

func runNativeWorkflowPrePR(hookContext checkHookContext, cwd string) checkResult {
	result := checkResult{Passed: true, Warnings: []string{}, Errors: []string{}, Findings: []string{}}
	command := checkContextCommand(hookContext)
	if checkContextToolName(hookContext) != "Bash" || !strings.Contains(command, "gh pr create") {
		return result
	}

	changelog, err := os.ReadFile(filepath.Join(cwd, "CHANGELOG.md"))
	if err != nil {
		result.Passed = false
		result.Blocked = true
		if os.IsNotExist(err) {
			result.Errors = append(result.Errors, "CHANGELOG.md not found")
		} else {
			result.Errors = append(result.Errors, "Could not read CHANGELOG.md")
		}
	} else {
		section, ok := unreleasedSection(string(changelog))
		if !ok {
			result.Passed = false
			result.Blocked = true
			result.Errors = append(result.Errors, "CHANGELOG.md missing [Unreleased] section")
		} else if !unreleasedSectionHasEntries(section) && !isWorkflowReleaseEscape(cwd, string(changelog)) {
			result.Passed = false
			result.Blocked = true
			result.Errors = append(result.Errors,
				"CHANGELOG.md [Unreleased] section is empty (no entries found)",
				"Add changelog entries before creating a PR",
			)
		}
	}

	result.Warnings = append(result.Warnings, baseBranchAbsorptionWarnings(cwd)...)
	validatePRCreateCommand(command, &result)
	return result
}

func runNativeGitHubAccount(hookContext checkHookContext, cwd string) checkResult {
	return runNativeGitHubAccountWithRunner(hookContext, cwd, activeGitHubAccount, switchGitHubAccount)
}

func runNativeGitHubAccountWithRunner(
	hookContext checkHookContext,
	cwd string,
	status func(string) githubAccountCommandResult,
	switcher func(string, string) githubAccountCommandResult,
) checkResult {
	result := checkResult{Passed: true, Warnings: []string{}, Errors: []string{}, Findings: []string{}}
	command := checkContextCommand(hookContext)
	if checkContextToolName(hookContext) != "Bash" || !shellCommandUsesGitHubCLI(command) {
		return result
	}
	// Identity administration (`gh auth ...`) is the user's domain: pass it
	// through untouched — no status probe, no switch — so convergence never
	// misdirects the command onto the configured account.
	if shellCommandOnlyManagesGitHubAuth(command) {
		return result
	}
	expected, err := configuredGitHubAccount(cwd)
	if err != nil {
		result.Passed = false
		result.Blocked = true
		result.Errors = append(result.Errors, err.Error())
		return result
	}
	if expected == "" {
		return result
	}
	resolution := resolveGitHubAccount(cwd, expected, status, switcher)
	switch resolution.outcome {
	case githubAccountOutcomeMatch:
		return result
	case githubAccountOutcomeSwitched:
		// Converge instead of dead-ending, but surface the mutation as a
		// pass-with-warnings finding so the switch is never silent.
		result.Warnings = append(result.Warnings, githubAccountSwitchNotice(resolution))
		return result
	case githubAccountOutcomeUnavailable:
		result.Passed = false
		result.Blocked = true
		result.Errors = append(result.Errors, resolution.reason)
		return result
	default:
		result.Passed = false
		result.Blocked = true
		result.Errors = append(result.Errors, githubAccountSwitchFailureMessages(resolution)...)
		result.Findings = append(result.Findings, "gh could not switch to the configured account, so gh would run with the wrong project identity")
		return result
	}
}

func runNativeValidatePush(hookContext checkHookContext, cwd string) checkResult {
	result := checkResult{Passed: true, Warnings: []string{}, Errors: []string{}, Findings: []string{}}
	command := checkContextCommand(hookContext)
	if checkContextToolName(hookContext) != "Bash" || !gitPushCommandRE.MatchString(strings.TrimSpace(command)) {
		return result
	}

	checkDirectMainSourcePush(cwd, &result)

	packageVersion, hasPackageJSON, hasBuildScript := headPackageInfo(cwd)

	// Version-bump and CHANGELOG checks are release readiness: they only apply to
	// release-flow pushes (default branch or tags). Feature-branch pushes happen
	// between releases, where the version legitimately still matches the last tag.
	if pushIsReleaseFlow(command, cwd) {
		lastTag := lastGitTag(cwd)
		isReleaseCommit := false
		if lastTag != "" {
			isReleaseCommit = tagPointsAtHEAD(cwd, lastTag)
		}
		if !isReleaseCommit && releaseCommitSubjectRE.MatchString(headSubject(cwd)) {
			isReleaseCommit = true
		}

		if lastTag != "" && hasPackageJSON && !isReleaseCommit {
			if tagVersion, ok := tagPackageVersion(cwd, lastTag); ok && packageVersion != "" && packageVersion == tagVersion {
				result.Errors = append(result.Errors, fmt.Sprintf("Version not bumped since %s (still %s)", lastTag, packageVersion))
			}
		}

		if lastTag != "" && !isReleaseCommit {
			if !headHasFile(cwd, "CHANGELOG.md") {
				result.Errors = append(result.Errors, "CHANGELOG.md not found in HEAD (required for tagged releases)")
			} else if !fileChangedSinceTag(cwd, lastTag, "CHANGELOG.md") {
				result.Errors = append(result.Errors, fmt.Sprintf("CHANGELOG.md not updated since %s", lastTag))
			}
		}
	}

	if hasBuildScript && !buildSucceeds(cwd) {
		result.Errors = append(result.Errors, "Build failed - fix build errors before pushing")
	}

	if len(result.Errors) > 0 {
		result.Passed = false
		result.Blocked = true
	}
	return result
}

var pushTagRefspecRE = regexp.MustCompile(`(?:^|\s)\+?(?:refs/tags/\S+|\S+:refs/tags/\S+|v\d+\.\d+\.\d+[^\s:]*)(?:\s|$)`)

// pushIsReleaseFlow reports whether a push participates in the release flow:
// pushing tags or pushing the default branch.
func pushIsReleaseFlow(command string, cwd string) bool {
	if strings.Contains(command, "--tags") || strings.Contains(command, "--follow-tags") || pushTagRefspecRE.MatchString(command) {
		return true
	}
	currentBranch, err := commandOutput(cwd, "git", "branch", "--show-current")
	if err != nil {
		return false
	}
	currentBranch = strings.TrimSpace(currentBranch)
	return currentBranch != "" && currentBranch == gitDefaultBranch(cwd)
}

func gitDefaultBranch(cwd string) string {
	if output, err := commandOutput(cwd, "git", "symbolic-ref", "refs/remotes/origin/HEAD"); err == nil {
		ref := strings.TrimSpace(output)
		if strings.HasPrefix(ref, "refs/remotes/origin/") {
			return strings.TrimPrefix(ref, "refs/remotes/origin/")
		}
	}
	return "main"
}

func checkDirectMainSourcePush(cwd string, result *checkResult) {
	currentBranch, err := commandOutput(cwd, "git", "branch", "--show-current")
	if err != nil {
		return
	}
	currentBranch = strings.TrimSpace(currentBranch)
	if currentBranch == "" || currentBranch != gitDefaultBranch(cwd) {
		return
	}
	changedFilesOutput, err := commandOutput(cwd, "git", "diff", "origin/"+currentBranch+"..HEAD", "--name-only")
	if err != nil {
		return
	}
	files := nonEmptyLines(changedFilesOutput)
	if len(files) == 0 {
		return
	}
	var disallowed []string
	for _, file := range files {
		if !strings.HasPrefix(file, ".agents/") && !strings.HasPrefix(file, "docs/") {
			disallowed = append(disallowed, file)
		}
	}
	if len(disallowed) > 0 {
		result.Passed = false
		result.Blocked = true
		sample := disallowed
		if len(sample) > 5 {
			sample = sample[:5]
		}
		message := fmt.Sprintf("Direct push to %s only allowed for .agents/ and docs/. Use a feature branch + PR for: %s", currentBranch, strings.Join(sample, ", "))
		if len(disallowed) > 5 {
			message += fmt.Sprintf(" (and %d more)", len(disallowed)-5)
		}
		result.Errors = append(result.Errors, message)
	}
}

func headPackageInfo(cwd string) (version string, exists bool, hasBuildScript bool) {
	body, err := commandOutput(cwd, "git", "show", "HEAD:package.json")
	if err != nil {
		return "", false, false
	}
	var pkg struct {
		Version string            `json:"version"`
		Scripts map[string]string `json:"scripts"`
	}
	if err := json.Unmarshal([]byte(body), &pkg); err != nil {
		return "", true, false
	}
	_, hasBuild := pkg.Scripts["build"]
	return pkg.Version, true, hasBuild
}

func tagPackageVersion(cwd string, tag string) (string, bool) {
	body, err := commandOutput(cwd, "git", "show", tag+":package.json")
	if err != nil {
		return "", false
	}
	var pkg struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal([]byte(body), &pkg); err != nil {
		return "", false
	}
	return pkg.Version, pkg.Version != ""
}

func lastGitTag(cwd string) string {
	output, err := commandOutput(cwd, "git", "describe", "--tags", "--abbrev=0")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(output)
}

func tagPointsAtHEAD(cwd string, tag string) bool {
	tagSHA, err := commandOutput(cwd, "git", "rev-list", "-1", tag)
	if err != nil {
		return false
	}
	headSHA, err := commandOutput(cwd, "git", "rev-parse", "HEAD")
	if err != nil {
		return false
	}
	return strings.TrimSpace(tagSHA) == strings.TrimSpace(headSHA)
}

func headHasFile(cwd string, path string) bool {
	_, err := commandOutput(cwd, "git", "show", "HEAD:"+path)
	return err == nil
}

func fileChangedSinceTag(cwd string, tag string, path string) bool {
	output, err := commandOutput(cwd, "git", "diff", tag, "HEAD", "--name-only", "--", path)
	return err == nil && strings.TrimSpace(output) != ""
}

func buildSucceeds(cwd string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "npm", "run", "build")
	cmd.Dir = cwd
	return cmd.Run() == nil
}

func nonEmptyLines(output string) []string {
	var lines []string
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func unreleasedSection(changelog string) (string, bool) {
	match := unreleasedHeadingRE.FindStringIndex(changelog)
	if match == nil {
		return "", false
	}
	bodyStart := match[1]
	rest := changelog[bodyStart:]
	next := changelogHeadingRE.FindStringIndex(rest)
	if next == nil {
		return rest, true
	}
	return rest[:next[0]], true
}

func unreleasedSectionHasEntries(section string) bool {
	for _, line := range strings.Split(section, "\n") {
		if unreleasedStubRE.MatchString(line) {
			continue
		}
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") {
			return true
		}
	}
	return false
}

func isWorkflowReleaseEscape(cwd string, changelog string) bool {
	if headHasTag(cwd) || releaseCommitSubjectRE.MatchString(headSubject(cwd)) {
		return true
	}
	return isReleaseOnlyPR(cwd, changelog)
}

func headHasTag(cwd string) bool {
	output, err := commandOutput(cwd, "git", "tag", "--points-at", "HEAD")
	return err == nil && strings.TrimSpace(output) != ""
}

func headSubject(cwd string) string {
	output, err := commandOutput(cwd, "git", "log", "-1", "--pretty=%s")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(output)
}

func baseBranchAbsorptionWarnings(cwd string) []string {
	branch, err := commandOutput(cwd, "git", "branch", "--show-current")
	if err != nil {
		return nil
	}
	branch = strings.TrimSpace(branch)
	baseRef, err := commandOutput(cwd, "gh", "repo", "view", "--json", "defaultBranchRef", "-q", ".defaultBranchRef.name")
	if err != nil {
		return nil
	}
	baseRef = strings.TrimSpace(baseRef)
	if branch == "" || baseRef == "" || branch == baseRef {
		return nil
	}
	unpushed, err := commandOutput(cwd, "git", "rev-list", "--count", "origin/"+baseRef+".."+baseRef)
	if err != nil {
		return nil
	}
	count := strings.TrimSpace(unpushed)
	if count == "" || count == "0" {
		return nil
	}
	return []string{
		fmt.Sprintf("%s has %s unpushed commit(s) that will be absorbed into this PR's squash merge", baseRef, count),
		fmt.Sprintf("Fix: git checkout %s && git push && git checkout %s - then create the PR", baseRef, branch),
	}
}

func validatePRCreateCommand(command string, result *checkResult) {
	hasFillFlag := strings.Contains(command, "--fill")
	hasBodyFileFlag := strings.Contains(command, "--body-file")
	hasAnyExplicitFlag := regexp.MustCompile(`--(title|body|fill|body-file)`).MatchString(command)
	if !hasAnyExplicitFlag || hasFillFlag || hasBodyFileFlag {
		return
	}

	titleMatch := prTitleFlagRE.FindStringSubmatch(command)
	if len(titleMatch) != 4 {
		result.Passed = false
		result.Blocked = true
		result.Errors = append(result.Errors,
			"Missing --title flag - PR title is required",
			`Example: gh pr create --title "feat: add new feature"`,
		)
	} else {
		title := strings.TrimSpace(firstNonEmpty(titleMatch[1], titleMatch[2], titleMatch[3]))
		if len(title) < 10 {
			result.Passed = false
			result.Blocked = true
			result.Errors = append(result.Errors, fmt.Sprintf("PR title is too short (%d chars) - minimum 10 characters", len(title)))
		}
		if !conventionalPRTitleRE.MatchString(title) {
			result.Warnings = append(result.Warnings, "PR title doesn't follow Conventional Commits format (e.g., 'feat: add feature')")
		}
	}

	if !strings.Contains(command, "--body") {
		result.Passed = false
		result.Blocked = true
		result.Errors = append(result.Errors,
			"Missing --body flag - PR description is required",
			`Example: gh pr create --title "..." --body "Description of changes"`,
		)
	}
}

type detectedVersionFile struct {
	path    string
	version string
}

func isReleaseOnlyPR(cwd string, changelog string) bool {
	currentBranch, err := commandOutput(cwd, "git", "branch", "--show-current")
	if err != nil || strings.TrimSpace(currentBranch) == "" {
		return false
	}
	base := resolveReleaseBase(cwd, strings.TrimSpace(currentBranch))
	if base == "" {
		return false
	}
	versionFiles := detectWorkflowVersionFiles(cwd)
	if len(versionFiles) == 0 {
		return false
	}
	diff, err := commandOutput(cwd, "git", "diff", base+"..HEAD", "--name-only")
	if err != nil {
		return false
	}
	changed := map[string]bool{}
	for _, line := range strings.Split(diff, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			changed[line] = true
		}
	}
	if len(changed) == 0 || !changed["CHANGELOG.md"] {
		return false
	}
	allowlist := map[string]bool{"CHANGELOG.md": true}
	for _, file := range versionFiles {
		allowlist[filepath.ToSlash(file.path)] = true
	}
	for path := range changed {
		if !allowlist[path] {
			return false
		}
	}
	hasVersionChange := false
	for _, file := range versionFiles {
		if changed[filepath.ToSlash(file.path)] {
			hasVersionChange = true
			break
		}
	}
	if !hasVersionChange {
		return false
	}
	candidate := versionFiles[0].version
	if candidate == "" {
		return false
	}
	for _, file := range versionFiles {
		if file.version != candidate {
			return false
		}
	}
	return changelogVersionSectionHasEntries(changelog, candidate)
}

func resolveReleaseBase(cwd string, currentBranch string) string {
	if output, err := commandOutput(cwd, "gh", "pr", "view", currentBranch, "--json", "baseRefName,state", "-q", `select(.state == "OPEN") | .baseRefName`); err == nil && strings.TrimSpace(output) != "" {
		return strings.TrimSpace(output)
	}
	if output, err := commandOutput(cwd, "git", "config", "--get", "loaf.release.base"); err == nil && strings.TrimSpace(output) != "" {
		return strings.TrimSpace(output)
	}
	if output, err := commandOutput(cwd, "gh", "repo", "view", "--json", "defaultBranchRef", "-q", ".defaultBranchRef.name"); err == nil && strings.TrimSpace(output) != "" {
		return strings.TrimSpace(output)
	}
	if output, err := commandOutput(cwd, "git", "symbolic-ref", "refs/remotes/origin/HEAD"); err == nil {
		ref := strings.TrimSpace(output)
		if strings.HasPrefix(ref, "refs/remotes/origin/") {
			return strings.TrimPrefix(ref, "refs/remotes/origin/")
		}
	}
	return ""
}

func detectWorkflowVersionFiles(cwd string) []detectedVersionFile {
	var paths []string
	if isFile(filepath.Join(cwd, "package.json")) {
		paths = append(paths, "package.json")
	}
	paths = append(paths, configuredWorkflowVersionFiles(cwd)...)
	seen := map[string]bool{}
	var files []detectedVersionFile
	for _, path := range paths {
		path = filepath.ToSlash(path)
		if seen[path] {
			continue
		}
		seen[path] = true
		version := readWorkflowVersionFile(cwd, path)
		if version != "" {
			files = append(files, detectedVersionFile{path: path, version: version})
		}
	}
	return files
}

func configuredWorkflowVersionFiles(cwd string) []string {
	body, err := os.ReadFile(filepath.Join(cwd, ".agents", "loaf.json"))
	if err != nil {
		return nil
	}
	var config struct {
		Release struct {
			VersionFiles []string `json:"versionFiles"`
		} `json:"release"`
	}
	if err := json.Unmarshal(body, &config); err != nil {
		return nil
	}
	return config.Release.VersionFiles
}

func readWorkflowVersionFile(cwd string, path string) string {
	body, err := os.ReadFile(filepath.Join(cwd, filepath.FromSlash(path)))
	if err != nil {
		return ""
	}
	if strings.HasSuffix(path, ".json") {
		var data struct {
			Version string `json:"version"`
		}
		if err := json.Unmarshal(body, &data); err == nil {
			return data.Version
		}
	}
	if matches := regexp.MustCompile(`(?m)^\s*version\s*=\s*["']([^"']+)["']`).FindStringSubmatch(string(body)); len(matches) == 2 {
		return matches[1]
	}
	return ""
}

func changelogVersionSectionHasEntries(changelog string, version string) bool {
	heading := regexp.MustCompile(`(?m)^##\s*\[` + regexp.QuoteMeta(version) + `\].*$`)
	match := heading.FindStringIndex(changelog)
	if match == nil {
		return false
	}
	rest := changelog[match[1]:]
	next := changelogHeadingRE.FindStringIndex(rest)
	section := rest
	if next != nil {
		section = rest[:next[0]]
	}
	for _, line := range strings.Split(section, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") {
			return true
		}
	}
	return false
}

func commandOutput(cwd string, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = cwd
	output, err := cmd.Output()
	return string(output), err
}

func shouldRunSecurityScanners(context checkHookContext) bool {
	validationLevel := firstNonEmpty(context.ValidationLevel, os.Getenv("VALIDATION_LEVEL"))
	agentType := firstNonEmpty(context.AgentType, os.Getenv("AGENT_TYPE"))
	return validationLevel == "thorough" || agentType == "reviewer" || agentType == "implementer"
}

type securityScannerResult struct {
	anyAvailable     bool
	criticalFindings []string
	warningFindings  []string
}

func runSecurityScanners(cwd string) securityScannerResult {
	var result securityScannerResult
	if _, err := exec.LookPath("trivy"); err == nil {
		result.anyAvailable = true
		result.merge(parseTrivyOutput(runScanner(cwd, "trivy", "fs", "--severity", "CRITICAL,HIGH", "--format", "json", "--quiet", ".")))
	}
	if _, err := exec.LookPath("semgrep"); err == nil {
		result.anyAvailable = true
		result.merge(parseSemgrepOutput(runScanner(cwd, "semgrep", "--config", "auto", "--json", "--quiet", "--severity", "ERROR", ".")))
	}
	if isFile(filepath.Join(cwd, "package.json")) {
		result.anyAvailable = true
		result.merge(parseNpmAuditOutput(runScannerWithShell(cwd, "npm audit --json 2>/dev/null || true")))
	}
	return result
}

func isFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func (r *securityScannerResult) merge(other securityScannerResult) {
	r.criticalFindings = append(r.criticalFindings, other.criticalFindings...)
	r.warningFindings = append(r.warningFindings, other.warningFindings...)
}

func runScanner(cwd string, name string, args ...string) []byte {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = cwd
	output, err := cmd.Output()
	if err != nil {
		return nil
	}
	return output
}

func runScannerWithShell(cwd string, command string) []byte {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Dir = cwd
	output, err := cmd.Output()
	if err != nil {
		return nil
	}
	return output
}

func parseTrivyOutput(output []byte) securityScannerResult {
	var result securityScannerResult
	if len(output) == 0 {
		return result
	}
	var parsed struct {
		Results []struct {
			Vulnerabilities []struct {
				ID       string `json:"VulnerabilityID"`
				Severity string `json:"Severity"`
				Package  string `json:"PkgName"`
			} `json:"Vulnerabilities"`
		} `json:"Results"`
	}
	if err := json.Unmarshal(output, &parsed); err != nil {
		return result
	}
	for _, scanResult := range parsed.Results {
		for _, vulnerability := range scanResult.Vulnerabilities {
			finding := fmt.Sprintf("trivy: %s (%s) in %s", vulnerability.ID, vulnerability.Severity, vulnerability.Package)
			if vulnerability.Severity == "CRITICAL" {
				result.criticalFindings = append(result.criticalFindings, finding)
			} else {
				result.warningFindings = append(result.warningFindings, finding)
			}
		}
	}
	return result
}

func parseSemgrepOutput(output []byte) securityScannerResult {
	var result securityScannerResult
	if len(output) == 0 {
		return result
	}
	var parsed struct {
		Results []struct {
			CheckID string `json:"check_id"`
			Path    string `json:"path"`
			Start   struct {
				Line int `json:"line"`
			} `json:"start"`
			Extra struct {
				Severity string `json:"severity"`
			} `json:"extra"`
		} `json:"results"`
	}
	if err := json.Unmarshal(output, &parsed); err != nil {
		return result
	}
	for _, finding := range parsed.Results {
		desc := fmt.Sprintf("semgrep: %s in %s:%d", finding.CheckID, finding.Path, finding.Start.Line)
		if finding.Extra.Severity == "ERROR" {
			result.criticalFindings = append(result.criticalFindings, desc)
		} else {
			result.warningFindings = append(result.warningFindings, desc)
		}
	}
	return result
}

func parseNpmAuditOutput(output []byte) securityScannerResult {
	var result securityScannerResult
	if len(output) == 0 {
		return result
	}
	var parsed struct {
		Vulnerabilities map[string]struct {
			Severity string `json:"severity"`
		} `json:"vulnerabilities"`
	}
	if err := json.Unmarshal(output, &parsed); err != nil {
		return result
	}
	for pkg, vulnerability := range parsed.Vulnerabilities {
		desc := fmt.Sprintf("npm-audit: %s vulnerability in %s", firstNonEmpty(vulnerability.Severity, "unknown"), pkg)
		switch vulnerability.Severity {
		case "critical":
			result.criticalFindings = append(result.criticalFindings, desc)
		case "high":
			result.warningFindings = append(result.warningFindings, desc)
		}
	}
	return result
}

func stringSliceContains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func extractCommitMessage(command string) string {
	if matches := commitMessageHeredocStartRE.FindStringSubmatchIndex(command); len(matches) == 4 {
		marker := command[matches[2]:matches[3]]
		bodyStart := matches[1]
		body := command[bodyStart:]
		if end := strings.Index(body, "\n"+marker); end >= 0 {
			return strings.TrimSpace(body[:end])
		}
	}
	matches := commitMessageFlagRE.FindStringSubmatch(command)
	if len(matches) != 4 {
		return ""
	}
	return firstNonEmpty(matches[1], matches[2], matches[3])
}

func detectBundledArtifactLeak(cwd string, subject string) []string {
	subjectLine := strings.TrimSpace(strings.Split(subject, "\n")[0])
	for _, pattern := range exemptBuildSubjectPatterns {
		if pattern.MatchString(subjectLine) {
			return nil
		}
	}
	staged := stagedFiles(cwd)
	var leaked []string
	for _, path := range staged {
		if isBuildOutputPath(path) {
			leaked = append(leaked, path)
		}
	}
	return leaked
}

func stagedFiles(cwd string) []string {
	cmd := exec.Command("git", "diff", "--cached", "--name-only")
	cmd.Dir = cwd
	output, err := cmd.Output()
	if err != nil {
		return nil
	}
	var paths []string
	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			paths = append(paths, line)
		}
	}
	return paths
}

func isBuildOutputPath(path string) bool {
	for _, prefix := range buildOutputPathPrefixes {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return rootLockfiles[path]
}

func buildLeakBlockMessage(paths []string) []string {
	list := make([]string, 0, len(paths))
	for _, path := range paths {
		list = append(list, "  - "+path)
	}
	resetArgs := strings.Join(paths, " ")
	return []string{
		"Commit includes build-output paths, but subject does not indicate a build/release commit:",
		strings.Join(list, "\n"),
		"",
		"Build-output paths leak into feature commits when the dev forgets to split. To fix:",
		"",
		"  1. Unstage the build outputs:",
		"       git reset HEAD " + resetArgs,
		"  2. Commit the source changes alone with the original subject.",
		"  3. Stage and commit the build outputs separately:",
		"       git add " + resetArgs,
		`       git commit -m "chore: build update bundled CLI"`,
		"",
		"If the leak is intentional (e.g., adding a new file under plugins/), bypass with:",
		"  git commit --no-verify",
	}
}

func checkContextToolName(context checkHookContext) string {
	return firstNonEmpty(context.Tool.Name, context.ToolName)
}

func checkContextCommand(context checkHookContext) string {
	return firstNonEmpty(context.ToolInput.Command, context.Tool.Input.Command, context.Input.Command)
}

func checkContextFilePath(context checkHookContext) string {
	return firstNonEmpty(context.ToolInput.FilePath, context.Tool.Input.FilePath, context.Input.FilePath)
}

func checkContextContent(context checkHookContext) string {
	return firstNonEmpty(
		context.ToolInput.Content,
		context.Tool.Input.Content,
		context.ToolInput.NewString,
		context.Tool.Input.NewString,
		context.Input.Content,
		context.Input.NewString,
	)
}

func writeCheckJSON(out io.Writer, hook string, result checkResult, advisory bool) error {
	exitCode := 0
	if result.Blocked && !advisory {
		exitCode = 2
	}
	return writeJSON(out, checkJSONOutput{
		Hook:     hook,
		Passed:   result.Passed && !result.Blocked,
		Blocked:  result.Blocked,
		Advisory: advisory,
		ExitCode: exitCode,
		Warnings: result.Warnings,
		Errors:   result.Errors,
		Findings: result.Findings,
	})
}

func writeCheckText(out io.Writer, errOut io.Writer, hook string, result checkResult, advisory bool) {
	if result.Blocked {
		if advisory {
			fmt.Fprintf(errOut, "\n%s %s: advisory findings (not blocking)\n", ansiYellow("!"), ansiBold(hook))
		} else {
			fmt.Fprintf(errOut, "\n%s %s: blocked\n", ansiRed("x"), ansiBold(hook))
		}
		for _, checkErr := range result.Errors {
			fmt.Fprintf(errOut, "   %s %s\n", ansiRed("-"), checkErr)
		}
		if len(result.Findings) > 0 {
			fmt.Fprintf(errOut, "\n   %s\n", ansiBold("Findings:"))
			for _, finding := range result.Findings {
				fmt.Fprintf(errOut, "   %s %s\n", ansiGray("-"), finding)
			}
		}
		return
	}
	if len(result.Warnings) > 0 {
		fmt.Fprintf(out, "\n%s %s: passed with warnings\n", ansiYellow("!"), ansiBold(hook))
		for _, warning := range result.Warnings {
			fmt.Fprintf(out, "   WARN: %s\n", warning)
		}
		return
	}
	fmt.Fprintf(out, "%s %s: passed\n", ansiGreen("ok"), ansiBold(hook))
}
