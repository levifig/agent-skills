/**
 * loaf check command
 *
 * Run enforcement hook checks for quality gates and security.
 * Reads hook context via stdin (JSON from harness) and returns appropriate exit codes.
 *
 * Exit codes:
 *   0 = pass (including warnings)
 *   1 = internal error only
 *   2 = block (check failed)
 *
 * Usage:
 *   echo '{"tool":{"name":"Bash"},"input":{"command":"git push"}}' | loaf check --hook validate-push
 *   loaf check --hook check-secrets --json < context.json
 */

import { Command } from "commander";
import { readFileSync, existsSync } from "fs";
import { join, dirname } from "path";
import { fileURLToPath } from "url";
import { execSync } from "child_process";

const __dirname = dirname(fileURLToPath(import.meta.url));

// ─────────────────────────────────────────────────────────────────────────────
// Types
// ─────────────────────────────────────────────────────────────────────────────

interface HookContext {
  tool?: {
    name?: string;
    input?: {
      command?: string;
      file_path?: string;
      content?: string;
      new_string?: string;
      [key: string]: unknown;
    };
  };
  tool_name?: string;
  tool_input?: {
    command?: string;
    file_path?: string;
    content?: string;
    new_string?: string;
    [key: string]: unknown;
  };
  input?: {
    command?: string;
    file_path?: string;
    content?: string;
    new_string?: string;
    [key: string]: unknown;
  };
  agent_type?: string;
  validation_level?: string;
}

interface CheckResult {
  passed: boolean;
  blocked: boolean;
  warnings: string[];
  errors: string[];
  findings?: string[];
}

interface JsonOutput {
  hook: string;
  passed: boolean;
  blocked: boolean;
  exitCode: number;
  warnings: string[];
  errors: string[];
  findings?: string[];
}

// ─────────────────────────────────────────────────────────────────────────────
// ANSI color helpers
// ─────────────────────────────────────────────────────────────────────────────

const bold = (s: string) => `\x1b[1m${s}\x1b[0m`;
const green = (s: string) => `\x1b[32m${s}\x1b[0m`;
const red = (s: string) => `\x1b[31m${s}\x1b[0m`;
const yellow = (s: string) => `\x1b[33m${s}\x1b[0m`;
const gray = (s: string) => `\x1b[90m${s}\x1b[0m`;

// ─────────────────────────────────────────────────────────────────────────────
// Utility Functions
// ─────────────────────────────────────────────────────────────────────────────

export async function readStdin(): Promise<string> {
  return new Promise((resolve, reject) => {
    let data = "";
    process.stdin.setEncoding("utf8");
    
    process.stdin.on("data", (chunk) => {
      data += chunk;
    });
    
    process.stdin.on("end", () => {
      resolve(data);
    });
    
    process.stdin.on("error", (err) => {
      reject(err);
    });
    
    // Handle case where stdin is already closed or not available
    if (process.stdin.isTTY) {
      resolve("");
    }
  });
}

function parseContext(stdinData: string): HookContext {
  if (!stdinData.trim()) {
    return {};
  }
  
  return JSON.parse(stdinData) as HookContext;
}

function getToolName(context: HookContext): string {
  return context.tool?.name || context.tool_name || "";
}

function getCommand(context: HookContext): string {
  // Support both flat (tool_input) and nested (tool.input) formats for cross-harness compatibility
  return context.tool_input?.command || 
         context.tool?.input?.command || 
         context.input?.command || 
         "";
}

function getFilePath(context: HookContext): string {
  // Support both flat (tool_input) and nested (tool.input) formats for cross-harness compatibility
  return context.tool_input?.file_path || 
         context.tool?.input?.file_path || 
         context.input?.file_path || 
         "";
}

function getContent(context: HookContext): string {
  // Support both flat (tool_input) and nested (tool.input) formats for cross-harness compatibility
  return context.tool_input?.content || 
         context.tool?.input?.content || 
         context.tool_input?.new_string || 
         context.tool?.input?.new_string || 
         context.input?.content || 
         context.input?.new_string || 
         "";
}

function outputJson(result: CheckResult, hookId: string): void {
  const output: JsonOutput = {
    hook: hookId,
    passed: result.passed && !result.blocked,
    blocked: result.blocked,
    exitCode: result.blocked ? 2 : 0,
    warnings: result.warnings,
    errors: result.errors,
    findings: result.findings,
  };
  console.log(JSON.stringify(output, null, 2));
}

function outputText(result: CheckResult, hookId: string): void {
  if (result.blocked) {
    console.error(`\n${red("✗")} ${bold(hookId)}: blocked`);
    for (const error of result.errors) {
      console.error(`   ${red("•")} ${error}`);
    }
    if (result.findings && result.findings.length > 0) {
      console.error(`\n   ${bold("Findings:")}`);
      for (const finding of result.findings) {
        console.error(`   ${gray("-")} ${finding}`);
      }
    }
  } else if (result.warnings.length > 0) {
    console.log(`\n${yellow("⚠")} ${bold(hookId)}: passed with warnings`);
    for (const warning of result.warnings) {
      console.log(`   WARN: ${warning}`);
    }
  } else {
    console.log(`${green("✓")} ${bold(hookId)}: passed`);
  }
}

// ─────────────────────────────────────────────────────────────────────────────
// Check: check-secrets
// Scan for hardcoded secrets, API keys, credentials
// ─────────────────────────────────────────────────────────────────────────────

async function checkSecrets(context: HookContext): Promise<CheckResult> {
  const result: CheckResult = {
    passed: true,
    blocked: false,
    warnings: [],
    errors: [],
    findings: [],
  };

  const filePath = getFilePath(context);
  const content = getContent(context);
  const command = getCommand(context);
  const toolName = getToolName(context);

  // Build content to scan from file content and/or command
  let contentToScan = content || "";

  // If this is a Bash command, also scan the command text for secrets
  if (toolName === "Bash" && command) {
    contentToScan += "\n" + command;
  }

  // Skip if nothing to scan
  if (!filePath && !contentToScan.trim()) {
    return result;
  }

  // All files are scanned for secrets - no exemptions
  // Previous exemptions for .md, .txt, lock files removed because:
  // - Lock files: may contain hashed secrets that should still be flagged
  // - Documentation files: common place for real credentials to leak
  // - Example/template files: often contain real-looking credentials that get copy-pasted
  
  // Secret patterns to detect
  const secretPatterns: Array<{ name: string; regex: RegExp }> = [
    { name: "AWS Access Key ID", regex: /AKIA[0-9A-Z]{16}/g },
    { name: "AWS Secret Key", regex: /aws_secret_access_key\s*=\s*["']?[A-Za-z0-9/+=]{40}["']?/ig },
    { name: "OpenAI API Key", regex: /sk-[a-zA-Z0-9]{20,}/g },
    { name: "Stripe Live Key", regex: /sk_live_[a-zA-Z0-9]{10,}/g },
    { name: "Stripe Test Key", regex: /sk_test_[a-zA-Z0-9]{10,}/g },
    { name: "Private Key", regex: /-----BEGIN (?:RSA |EC |DSA |OPENSSH )?PRIVATE KEY-----/g },
    { name: "Database Connection", regex: /(postgres|mysql|mongodb):\/\/[^:]+:[^@]+@/g },
    { name: "Password Assignment", regex: /password\s*=\s*["'][^"']{8,}["']/ig },
    { name: "Secret Assignment", regex: /secret\s*=\s*["'][^"']{8,}["']/ig },
    { name: "API Key Assignment", regex: /api_key\s*=\s*["'][^"']{16,}["']/ig },
    { name: "JWT Token", regex: /eyJ[A-Za-z0-9-_]+\.eyJ[A-Za-z0-9-_]+\.[A-Za-z0-9-_.+/=]*/g },
    { name: "GitHub Token", regex: /gh[pousr]_[A-Za-z0-9_]{36}/g },
  ];

  const targetContent = contentToScan;
  const foundSecrets: string[] = [];

  for (const { name, regex } of secretPatterns) {
    const matches = targetContent.match(regex);
    if (matches) {
      for (const match of matches) {
        const matchPreview = match.substring(0, 40);
        foundSecrets.push(`${name}: ${matchPreview}...`);
      }
    }
  }

  if (foundSecrets.length > 0) {
    result.passed = false;
    result.blocked = true;
    result.errors.push(`Potential secrets detected in ${filePath || "input"}`);
    result.findings = foundSecrets;
  }

  return result;
}

// ─────────────────────────────────────────────────────────────────────────────
// Check: validate-push
// Verify version bump, CHANGELOG entry, and successful build
// ─────────────────────────────────────────────────────────────────────────────

async function validatePush(context: HookContext): Promise<CheckResult> {
  const result: CheckResult = {
    passed: true,
    blocked: false,
    warnings: [],
    errors: [],
  };

  const toolName = getToolName(context);
  const command = getCommand(context);

  // Only process Bash tool with git push
  if (toolName !== "Bash" || !command.match(/^git\s+push/)) {
    return result;
  }

  const errors: string[] = [];

  // Detect project type - check from HEAD (committed), not disk (may have uncommitted changes)
  let hasPackageJson = false;
  let hasBuildScript = false;
  
  try {
    execSync("git show HEAD:package.json 2>/dev/null", { stdio: "pipe" });
    hasPackageJson = true;
    
    const pkgContent = execSync("git show HEAD:package.json 2>/dev/null", { encoding: "utf-8" });
    const pkg = JSON.parse(pkgContent);
    hasBuildScript = !!pkg.scripts?.build;
  } catch {
    // No package.json in HEAD
  }

  // Check 1: Version bump since last tag
  try {
    const lastTag = execSync("git describe --tags --abbrev=0 2>/dev/null", {
      encoding: "utf-8",
      stdio: ["pipe", "pipe", "ignore"],
    }).trim();

    if (lastTag && hasPackageJson) {
      try {
        // Read version from HEAD (committed), not disk (may have uncommitted changes)
        const headPkgContent = execSync(
          "git show HEAD:package.json 2>/dev/null",
          { encoding: "utf-8", stdio: ["pipe", "pipe", "ignore"] }
        );
        const headPkg = JSON.parse(headPkgContent);
        const currentVersion = headPkg.version;
        
        const tagPkgContent = execSync(
          `git show ${lastTag}:package.json 2>/dev/null`,
          { encoding: "utf-8", stdio: ["pipe", "pipe", "ignore"] }
        );
        const tagPkg = JSON.parse(tagPkgContent);
        const tagVersion = tagPkg.version;

        if (currentVersion && currentVersion === tagVersion) {
          errors.push(`Version not bumped since ${lastTag} (still ${currentVersion})`);
        }
      } catch {
        // ignore errors reading tag version
      }
    }
  } catch {
    // No tags yet, skip version check
  }

  // Check 2: CHANGELOG exists and is updated since last tag
  try {
    const lastTag = execSync("git describe --tags --abbrev=0 2>/dev/null", {
      encoding: "utf-8",
      stdio: ["pipe", "pipe", "ignore"],
    }).trim();

    if (lastTag) {
      // Check from HEAD (committed), not disk (may have uncommitted changes)
      let hasChangelogInHead = false;
      try {
        execSync("git show HEAD:CHANGELOG.md 2>/dev/null", { stdio: "pipe" });
        hasChangelogInHead = true;
      } catch {
        // No CHANGELOG.md in HEAD
      }
      
      if (!hasChangelogInHead) {
        errors.push("CHANGELOG.md not found in HEAD (required for tagged releases)");
      } else {
          try {
            // Check only committed changes between lastTag and HEAD (not unstaged edits)
            const changedFiles = execSync(
              `git diff ${lastTag} HEAD --name-only -- CHANGELOG.md 2>/dev/null`,
              { encoding: "utf-8", stdio: ["pipe", "pipe", "ignore"] }
            ).trim();

            if (!changedFiles) {
              errors.push(`CHANGELOG.md not updated since ${lastTag}`);
            }
          } catch {
            // ignore
          }
        }
      }
  } catch {
    // No tags yet, skip changelog check
  }

  // Check 3: Build validation (SPEC-020 compliance)
  // Validates that the build succeeds before pushing
  if (hasBuildScript) {
    try {
      // Must be < hook timeout (60s in hooks.yaml)
      execSync("npm run build", {
        cwd: process.cwd(),
        encoding: "utf-8",
        stdio: ["pipe", "pipe", "pipe"],
        timeout: 45000,
      });
    } catch {
      errors.push("Build failed - fix build errors before pushing");
    }
  }

  if (errors.length > 0) {
    result.passed = false;
    result.blocked = true;
    result.errors.push(...errors);
  }

  return result;
}

// ─────────────────────────────────────────────────────────────────────────────
// Check: workflow-pre-pr
// Enforce PR title format and CHANGELOG entry
// ─────────────────────────────────────────────────────────────────────────────

async function workflowPrePr(context: HookContext): Promise<CheckResult> {
  const result: CheckResult = {
    passed: true,
    blocked: false,
    warnings: [],
    errors: [],
  };

  const toolName = getToolName(context);
  const command = getCommand(context);

  // Only match gh pr create
  if (toolName !== "Bash" || !command.includes("gh pr create")) {
    return result;
  }

  // Check 1: CHANGELOG.md has actual entries under [Unreleased]
  if (existsSync("CHANGELOG.md")) {
    try {
      const changelog = readFileSync("CHANGELOG.md", "utf-8");
      
      // Find the Unreleased section
      const unreleasedMatch = changelog.match(/##\s*\[Unreleased\]([\s\S]*?)(?=\n##\s|$)/);
      
      if (unreleasedMatch) {
        const unreleasedSection = unreleasedMatch[1];
        // Check for list items (lines starting with - or *)
        const hasEntries = /^[-*]\s/m.test(unreleasedSection);
        
        if (!hasEntries) {
          result.passed = false;
          result.blocked = true;
          result.errors.push("CHANGELOG.md [Unreleased] section is empty (no entries found)");
          result.errors.push("Add changelog entries before creating a PR");
        }
      } else {
        result.passed = false;
        result.blocked = true;
        result.errors.push("CHANGELOG.md missing [Unreleased] section");
      }
    } catch {
      result.passed = false;
      result.blocked = true;
      result.errors.push("Could not read CHANGELOG.md");
    }
  } else {
    result.passed = false;
    result.blocked = true;
    result.errors.push("CHANGELOG.md not found");
  }

  // Check 2: Base branch has unpushed commits (would be absorbed into PR)
  try {
    const branch = execSync("git branch --show-current", { encoding: "utf-8" }).trim();
    const baseRef = execSync("gh repo view --json defaultBranchRef -q .defaultBranchRef.name", { encoding: "utf-8" }).trim();
    if (branch !== baseRef) {
      const unpushed = execSync(`git rev-list --count origin/${baseRef}..${baseRef}`, { encoding: "utf-8" }).trim();
      const count = parseInt(unpushed, 10);
      if (count > 0) {
        result.warnings.push(
          `${baseRef} has ${count} unpushed commit(s) that will be absorbed into this PR's squash merge`,
          `Fix: git checkout ${baseRef} && git push && git checkout ${branch} — then create the PR`
        );
      }
    }
  } catch {
    // Non-critical — skip if git/gh commands fail
  }

  // Check 3: PR title and body requirements
  // Allow --fill or --body-file as alternatives to --title/--body
  // Interactive mode (no flags at all) is also allowed
  const hasFillFlag = /--fill/.test(command);
  const hasBodyFileFlag = /--body-file/.test(command);
  const hasAnyExplicitFlag = /--(title|body|fill|body-file)/.test(command);
  
  // If using --fill or --body-file, skip strict checks
  // If NO flags at all, also skip (interactive mode)
  // But if ANY explicit flag is present, require complete --title/--body
  if (hasAnyExplicitFlag && !hasFillFlag && !hasBodyFileFlag) {
    // Must have --title flag with non-empty value
    const titleMatch = command.match(/--title(?:\s+|=)(?:"([^"]+)"|'([^']+)'|([^\s"']+))/);
    if (!titleMatch) {
      result.passed = false;
      result.blocked = true;
      result.errors.push("Missing --title flag - PR title is required");
      result.errors.push("Example: gh pr create --title \"feat: add new feature\"");
    } else {
      const title = (titleMatch[1] || titleMatch[2] || titleMatch[3]).trim();
      
      // Title should follow conventional format: type(scope): description
      // or at minimum be descriptive (not just "fix" or "update")
      if (title.length < 10) {
        result.passed = false;
        result.blocked = true;
        result.errors.push(`PR title is too short (${title.length} chars) - minimum 10 characters`);
      }
      
      // Check for conventional commit format (optional but recommended)
      const conventionalPattern = /^(feat|fix|docs|style|refactor|perf|test|chore|ci|build|revert)(\(.+\))?!?: .+/i;
      if (!conventionalPattern.test(title)) {
        result.warnings.push("PR title doesn't follow Conventional Commits format (e.g., 'feat: add feature')");
      }
    }
    
    // Must have --body flag (can be empty but flag should be present)
    const hasBodyFlag = /--body/.test(command);
    if (!hasBodyFlag) {
      result.passed = false;
      result.blocked = true;
      result.errors.push("Missing --body flag - PR description is required");
      result.errors.push("Example: gh pr create --title \"...\" --body \"Description of changes\"");
    }
  }

  return result;
}

// ─────────────────────────────────────────────────────────────────────────────
// Check: validate-commit
// Validate Conventional Commits conventions
// ─────────────────────────────────────────────────────────────────────────────

async function validateCommit(context: HookContext): Promise<CheckResult> {
  const result: CheckResult = {
    passed: true,
    blocked: false,
    warnings: [],
    errors: [],
  };

  const toolName = getToolName(context);
  const command = getCommand(context);

  // Only check Bash commands that are git commits
  if (toolName !== "Bash" || !command.includes("git commit")) {
    return result;
  }

  // Skip if --amend without -m (uses existing message)
  if (command.includes("--amend") && !command.includes("-m")) {
    return result;
  }

  // Skip merge commits
  if (command.includes("--no-edit") || command.includes("git merge")) {
    return result;
  }

  // Extract commit message from command
  let message: string | null = null;

  // Match -m "message" or -m 'message' or -m message
  const msgMatch = command.match(/-m(?:\s+|=)(?:"([^"]+)"|'([^']+)'|([^\s"']+))/);
  
  if (msgMatch) {
    message = msgMatch[1] || msgMatch[2] || msgMatch[3];
  } else {
    // Match HEREDOC: -m "$(cat <<'EOF' ... EOF )"
    const heredocMatch = command.match(/<<'?EOF'?\s*\n(.+?)\n\s*EOF/s);
    if (heredocMatch) {
      message = heredocMatch[1].trim();
    }
  }

  if (!message) {
    // Can't extract message (might be interactive), allow
    return result;
  }

  // Validate Conventional Commits format (scoped commits not allowed)
  // Format: <type>[!]: <description>
  const conventionalCommitRegex = /^(feat|fix|docs|style|refactor|perf|test|chore|ci|build|revert)!?: .+/;

  if (!conventionalCommitRegex.test(message)) {
    result.passed = false;
    result.blocked = true;
    result.errors.push("Commit message does not follow Conventional Commits format");
    result.errors.push("Expected format: <type>: <description> (scoped commits not allowed)");
    result.errors.push("Valid types: feat, fix, docs, style, refactor, perf, test, chore, ci, build, revert");
    result.errors.push(`Your message: "${message}"`);
  }

  // Check for AI attribution in commit body
  const aiAttributionPattern = /\b(claude|gpt|copilot|chatgpt|gemini|anthropic)\b/i;
  if (aiAttributionPattern.test(message)) {
    result.passed = false;
    result.blocked = true;
    result.errors.push("Commit message contains AI attribution. Remove tool references from commit messages.");
  }

  // Check message length (should be under 72 chars for the subject)
  const subjectLine = message.split("\n")[0];
  if (subjectLine.length > 72) {
    result.warnings.push(`Subject line is ${subjectLine.length} characters (recommended: ≤72)`);
  }

  return result;
}

// ─────────────────────────────────────────────────────────────────────────────
// Check: security-audit
// Detect dangerous Bash command patterns
// ─────────────────────────────────────────────────────────────────────────────

async function securityAudit(context: HookContext): Promise<CheckResult> {
  const result: CheckResult = {
    passed: true,
    blocked: false,
    warnings: [],
    errors: [],
    findings: [],
  };

  const toolName = getToolName(context);
  const command = getCommand(context);

  // Only run on Bash tool
  if (toolName !== "Bash") {
    return result;
  }

  // Dangerous patterns to detect
  const dangerousPatterns: Array<{ name: string; regex: RegExp; critical: boolean }> = [
    { 
      name: "Dangerous rm -rf", 
      regex: /rm\s+-rf?\s+\/|rm\s+-rf?\s+\*+/, 
      critical: true 
    },
    { 
      name: "chmod 777 (world-writable)", 
      regex: /chmod\s+.*777/, 
      critical: true 
    },
    { 
      name: "eval of untrusted input", 
      regex: /eval\s*\$|eval\s*\`/, 
      critical: true 
    },
    { 
      name: "Unsafe curl to bash", 
      regex: /curl.*\|\s*(ba)?sh/, 
      critical: true 
    },
    { 
      name: "wget to shell", 
      regex: /wget.*\|\s*(ba)?sh/, 
      critical: true 
    },
    { 
      name: "sudo without validation", 
      regex: /sudo\s+(rm|dd|mkfs|fdisk|format)/, 
      critical: false 
    },
    { 
      name: "Hardcoded sudo password", 
      regex: /echo\s+['"].*['"]\s*\|\s*sudo/, 
      critical: true 
    },
    { 
      name: "Unsafe find exec", 
      regex: /find\s+.*-exec\s+(rm|mv|cp)\s*\{\}/, 
      critical: false 
    },
    { 
      name: "SQL injection risk in command", 
      regex: /mysql.*-e\s*.*['"]\s*\$/, 
      critical: false 
    },
  ];

  const targetCommand = command || "";
  const criticalFindings: string[] = [];
  const warningFindings: string[] = [];

  for (const { name, regex, critical } of dangerousPatterns) {
    if (regex.test(targetCommand)) {
      const finding = `${name}: matched pattern in command`;
      if (critical) {
        criticalFindings.push(finding);
      } else {
        warningFindings.push(finding);
      }
    }
  }

  if (criticalFindings.length > 0) {
    result.passed = false;
    result.blocked = true;
    result.errors.push("Critical security issues detected in command");
    result.findings = [...criticalFindings, ...warningFindings];
  } else if (warningFindings.length > 0) {
    result.warnings.push(...warningFindings);
  }

  // ─── Vulnerability Scanner Execution ─────────────────────────────────
  // Additive to pattern checks above. Runs real scanners against the
  // project filesystem, matching the coverage the old shell script
  // (foundations-security-audit.sh) provided via Trivy, Semgrep, npm audit.
  //
  // Gated: only runs when VALIDATION_LEVEL=thorough or AGENT_TYPE is
  // reviewer/implementer, to avoid expensive scans on every Bash command.
  // ─────────────────────────────────────────────────────────────────────

  const validationLevel = context.validation_level || process.env.VALIDATION_LEVEL || "";
  const agentType = context.agent_type || process.env.AGENT_TYPE || "";
  const shouldRunScanners =
    validationLevel === "thorough" ||
    agentType === "reviewer" ||
    agentType === "implementer";

  // Skip scanners for trivial commands (ls, echo, pwd, etc.)
  const isTrivialCommand = /^\s*(ls|echo|pwd|whoami|date|cat|head|tail|wc|true|false)\b/.test(targetCommand);

  if (shouldRunScanners && !isTrivialCommand) {
    const scannerTimeout = 30000; // 30s per tool
    let anyScannerAvailable = false;

    // Trivy — filesystem vulnerability scan
    try {
      execSync("command -v trivy", { stdio: "pipe" });
      anyScannerAvailable = true;
      try {
        const trivyOutput = execSync(
          "trivy fs --severity CRITICAL,HIGH --format json --quiet .",
          { encoding: "utf-8", stdio: ["pipe", "pipe", "pipe"], timeout: scannerTimeout }
        );
        const trivyResults = JSON.parse(trivyOutput);
        if (trivyResults?.Results) {
          for (const r of trivyResults.Results) {
            for (const v of r.Vulnerabilities || []) {
              const finding = `trivy: ${v.VulnerabilityID} (${v.Severity}) in ${v.PkgName}`;
              if (v.Severity === "CRITICAL") {
                criticalFindings.push(finding);
              } else {
                warningFindings.push(finding);
              }
            }
          }
        }
      } catch {
        // Trivy execution failed — non-fatal, continue
      }
    } catch {
      // Trivy not installed
    }

    // Semgrep — static analysis
    try {
      execSync("command -v semgrep", { stdio: "pipe" });
      anyScannerAvailable = true;
      try {
        const semgrepOutput = execSync(
          "semgrep --config auto --json --quiet --severity ERROR .",
          { encoding: "utf-8", stdio: ["pipe", "pipe", "pipe"], timeout: scannerTimeout }
        );
        const semgrepResults = JSON.parse(semgrepOutput);
        for (const finding of semgrepResults?.results || []) {
          const severity = finding.extra?.severity || "WARNING";
          const desc = `semgrep: ${finding.check_id} in ${finding.path}:${finding.start?.line}`;
          if (severity === "ERROR") {
            criticalFindings.push(desc);
          } else {
            warningFindings.push(desc);
          }
        }
      } catch {
        // Semgrep execution failed — non-fatal, continue
      }
    } catch {
      // Semgrep not installed
    }

    // npm audit — dependency vulnerabilities (only if package.json exists)
    if (existsSync("package.json")) {
      try {
        anyScannerAvailable = true;
        const auditOutput = execSync(
          "npm audit --json 2>/dev/null || true",
          { encoding: "utf-8", stdio: ["pipe", "pipe", "pipe"], timeout: scannerTimeout }
        );
        const auditResults = JSON.parse(auditOutput);
        const vulnerabilities = auditResults?.vulnerabilities || {};
        for (const [pkg, info] of Object.entries(vulnerabilities)) {
          const vuln = info as { severity?: string };
          const severity = vuln.severity || "unknown";
          const desc = `npm-audit: ${severity} vulnerability in ${pkg}`;
          if (severity === "critical") {
            criticalFindings.push(desc);
          } else if (severity === "high") {
            warningFindings.push(desc);
          }
        }
      } catch {
        // npm audit failed — non-fatal, continue
      }
    }

    if (!anyScannerAvailable) {
      result.warnings.push(
        "No vulnerability scanners found (trivy, semgrep, npm audit). Install for deeper security coverage."
      );
    }

    // Re-evaluate findings after scanner results
    if (criticalFindings.length > 0 && !result.blocked) {
      result.passed = false;
      result.blocked = true;
      result.errors.push("Critical vulnerabilities detected by security scanners");
      result.findings = [...(result.findings || []), ...criticalFindings, ...warningFindings];
    } else if (warningFindings.length > 0) {
      // Merge scanner warnings with any existing warnings
      for (const w of warningFindings) {
        if (!result.warnings.includes(w)) {
          result.warnings.push(w);
        }
      }
    }
  }

  return result;
}

// ─────────────────────────────────────────────────────────────────────────────
// Command Registration
// ─────────────────────────────────────────────────────────────────────────────

export function registerCheckCommand(program: Command): void {
  program
    .command("check")
    .description("Run enforcement hook checks")
    .option("--hook <id>", "Hook ID to run (check-secrets, validate-push, workflow-pre-pr, validate-commit, security-audit)")
    .option("--json", "Output JSON format")
    .action(async (options: { hook?: string; json?: boolean }) => {
      // Validate hook ID
      const validHooks = [
        "check-secrets",
        "validate-push", 
        "workflow-pre-pr",
        "validate-commit",
        "security-audit",
      ];

      if (!options.hook) {
        console.error(`${red("error:")} --hook <id> is required`);
        console.error(`${gray("Valid hooks:")} ${validHooks.join(", ")}`);
        process.exit(1);
      }

      if (!validHooks.includes(options.hook)) {
        console.error(`${red("error:")} Unknown hook: ${bold(options.hook)}`);
        console.error(`${gray("Valid hooks:")} ${validHooks.join(", ")}`);
        process.exit(1);
      }

      // Read stdin — if unavailable or unparseable, treat as empty context (pass)
      let context: HookContext;
      try {
        const stdinData = await readStdin();
        context = parseContext(stdinData);
      } catch {
        context = {};
      }

      // Run appropriate check
      let result: CheckResult;
      try {
        switch (options.hook) {
          case "check-secrets":
            result = await checkSecrets(context);
            break;
          case "validate-push":
            result = await validatePush(context);
            break;
          case "workflow-pre-pr":
            result = await workflowPrePr(context);
            break;
          case "validate-commit":
            result = await validateCommit(context);
            break;
          case "security-audit":
            result = await securityAudit(context);
            break;
          default:
            // This should never happen due to validation above
            console.error(`${red("error:")} Unknown hook: ${options.hook}`);
            process.exit(1);
        }
      } catch (err) {
        console.error(`${red("error:")} Check failed with exception: ${err}`);
        process.exit(1);
      }

      // Output results
      if (options.json) {
        outputJson(result, options.hook);
      } else {
        outputText(result, options.hook);
      }

      // Exit with appropriate code
      // 0 = pass (including warnings), 2 = block, 1 = internal error only
      if (result.blocked) {
        process.exit(2);
      } else {
        process.exit(0);
      }
    });
}
