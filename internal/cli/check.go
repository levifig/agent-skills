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
)

type checkOptions struct {
	hook       string
	jsonOutput bool
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
	"check-secrets":   true,
	"validate-push":   true,
	"workflow-pre-pr": true,
	"validate-commit": true,
	"security-audit":  true,
}

func (r Runner) runCheck(args []string, out io.Writer, runtimeRoot string) error {
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
	case "check-secrets":
		result = runNativeCheckSecrets(context)
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
		if err := writeCheckJSON(out, options.hook, result); err != nil {
			return err
		}
	} else {
		writeCheckText(out, firstWriter(r.Stderr, os.Stderr), options.hook, result)
	}
	if result.Blocked {
		return ExitError{Code: 2}
	}
	return nil
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

func runNativeValidatePush(hookContext checkHookContext, cwd string) checkResult {
	result := checkResult{Passed: true, Warnings: []string{}, Errors: []string{}, Findings: []string{}}
	command := checkContextCommand(hookContext)
	if checkContextToolName(hookContext) != "Bash" || !regexp.MustCompile(`^git\s+push`).MatchString(command) {
		return result
	}

	if pushIsAllowedOperationalMainPush(cwd, &result) {
		return result
	}

	packageVersion, hasPackageJSON, hasBuildScript := headPackageInfo(cwd)
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

	if hasBuildScript && !buildSucceeds(cwd) {
		result.Errors = append(result.Errors, "Build failed - fix build errors before pushing")
	}

	if len(result.Errors) > 0 {
		result.Passed = false
		result.Blocked = true
	}
	return result
}

func pushIsAllowedOperationalMainPush(cwd string, result *checkResult) bool {
	currentBranch, err := commandOutput(cwd, "git", "branch", "--show-current")
	if err != nil {
		return false
	}
	currentBranch = strings.TrimSpace(currentBranch)
	defaultBranch := "main"
	if output, err := commandOutput(cwd, "git", "symbolic-ref", "refs/remotes/origin/HEAD"); err == nil {
		ref := strings.TrimSpace(output)
		if strings.HasPrefix(ref, "refs/remotes/origin/") {
			defaultBranch = strings.TrimPrefix(ref, "refs/remotes/origin/")
		}
	}
	if currentBranch == "" || currentBranch != defaultBranch {
		return false
	}
	changedFilesOutput, err := commandOutput(cwd, "git", "diff", "origin/"+currentBranch+"..HEAD", "--name-only")
	if err != nil {
		return false
	}
	files := nonEmptyLines(changedFilesOutput)
	if len(files) == 0 {
		return true
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
		return true
	}
	return true
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

func writeCheckJSON(out io.Writer, hook string, result checkResult) error {
	exitCode := 0
	if result.Blocked {
		exitCode = 2
	}
	return writeJSON(out, checkJSONOutput{
		Hook:     hook,
		Passed:   result.Passed && !result.Blocked,
		Blocked:  result.Blocked,
		ExitCode: exitCode,
		Warnings: result.Warnings,
		Errors:   result.Errors,
		Findings: result.Findings,
	})
}

func writeCheckText(out io.Writer, errOut io.Writer, hook string, result checkResult) {
	if result.Blocked {
		fmt.Fprintf(errOut, "\n%s %s: blocked\n", ansiRed("x"), ansiBold(hook))
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
