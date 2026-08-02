# Permissions Configuration

## Contents
- Overview
- Read-Only Fence Rule
- Permission Commands by Harness
- Recommended Allowlists by Harness
- Orchestrator Allowlists by Harness
- Other Agent Roles (Claude Code Tokens)
- Sandbox Configuration
- Permission Patterns
- Security Considerations
- MCP Tool Permissions
- Troubleshooting
- Network Access Posture
- Best Practices

Permission patterns for autonomous operation and interactive workflows.

## Overview

Coding harnesses use permission prompts to protect against unintended actions. Configure permissions to reduce interruptions while maintaining appropriate safety. Exact command names and allowlist tokens are product-specific — paste only from the labeled section for the harness you are configuring.

## Read-Only Fence Rule

A fence labelled read-only (or "no modifications") must not auto-allow a command that can mutate. Audit every entry against all three hazards below — using the command's real help text and what the project configures, not recollection.

1. **Wildcard write flags.** Claude Code's `Bash(cmd *)` form matches any arguments and cannot exclude a flag. Any command that exposes a write, output-to-file, or exec flag must not be auto-allowed under a wildcard. Worked example: `git log` and `git diff` accept `--output=<file>`, which writes to an arbitrary path (verified on git 2.55: `git log -1 --output=/tmp/probe`). Prefer an exact entry with no wildcard (`Bash(git log)`, `Bash(git diff)`), or leave argument-bearing forms approval-gated. The same class previously caught `docker *`, `docker compose config` (`-o`/`--output`), and `terraform plan` (`-out`).
2. **Project-defined dispatch.** An exact-match entry is not intrinsically read-only when the fixed invocation runs project-controlled code — a package script (`npm run build`), a Makefile target, or a test collector that loads `conftest.py`. The project, not the allowlist, decides what that command does. Do not put such entries in a fence that claims writes are disabled; either keep them approval-gated, or label them plainly as mutation-capable.
3. **Network-command argument scope.** A trailing `*` matches any sequence of characters including spaces, so it spans further arguments (`Bash(git *)` matches `git log --oneline --all`). Therefore `Bash(curl https://api.example.com/*)` also matches `curl https://api.example.com/path -o /tmp/file`, `-T /etc/secret`, and `-X POST`. Do not use Bash URL patterns for network control — deny `curl`/`wget` and prefer domain-scoped `WebFetch(domain:…)` or a validating PreToolUse hook (Claude Code's own guidance).

## Permission Commands by Harness

### Claude Code

| Command | Purpose |
|---------|---------|
| `/permissions` | View and modify permission allowlists |
| `/allowed-tools` | Show currently allowed tools |
| `/sandbox` | Configure sandboxed execution environment |

## Recommended Allowlists by Harness

Paste-ready fences use product tool tokens. Only the labeled product's fence is valid configuration for that product; do not paste Claude Code tokens into a non-Claude allowlist.

### Claude Code

#### Development Work

For standard development sessions:

```
# Read operations (safe by default)
Read, Glob, Grep, WebFetch

# Write operations (allow for active development)
Write, Edit, NotebookEdit

# Execution (allow with caution)
Bash(npm run *), Bash(pytest *), Bash(make *)
Bash(git status), Bash(git diff), Bash(git log)
# Argument-bearing git log/diff stay approval-gated: both accept --output=<file> (see Read-Only Fence Rule).
```

#### CI/Autonomous Mode

For unattended execution in CI pipelines. This is not a read-only fence: package scripts and tool runners mutate the tree (in this repository `npm run build` writes `bin/`, `dist/`, and `plugins/`).

```
# Read tools (Write, Edit stay off)
Read, Glob, Grep

# Mutation-capable project dispatch (exact names — no wildcards). Approval-gate these if the pipeline must stay non-mutating.
Bash(npm run build), Bash(npm run test)
Bash(pytest), Bash(mypy)
# Wildcards stay approval-gated: pytest exposes --junitxml=<path>; mypy exposes --junit-xml, --*-report DIR, and --cache-dir (see Read-Only Fence Rule hazard 1). Exact forms still run project-defined code (hazard 2).
```

#### Code Review

For review-focused sessions:

```
# Read everything
Read, Glob, Grep

# Inspection only (exact forms — no wildcards, no project-script dispatch). Assumes no diff.external or textconv helpers; those execute project-configured code and make even exact git diff/log hazard-2 dispatch.
Bash(git diff), Bash(git log)
# Argument-bearing git diff/log stay approval-gated: both accept --output=<file> (hazard 1).
# Not auto-allowed: Bash(npm run lint), Bash(pytest --collect-only) — exact forms still dispatch into package scripts / conftest.py (hazard 2). Keep approval-gated.

# No modifications
# Write, Edit - DISABLED
```

## Orchestrator Allowlists by Harness

Coordination only — no implementation. Task-tracking tokens differ by product and must never be merged into one fence. Choose exactly one product's fence.

### Claude Code

```
# Coordination only - no implementation
Read, Glob, Grep
TodoWrite, TodoRead
Linear MCP tools (if configured)
Bash(date *), Bash(git status)
```

### Codex

This coordination fence has no Claude-style `Bash(git status)` tool-token entry — Codex does not paste tool tokens into an allowlist fence the way Claude Code does. Do not put `exec_command` in a coordination fence: that token is unrestricted execution.

```
# Coordination only - no implementation
update_plan
Linear MCP tools (if configured)
```

Grant scoped read/search and status commands (rg, cat, ls, date, git status) through Codex execpolicy `prefix_rule` entries in `~/.codex/rules/` (Starlark `prefix_rule(pattern=[...], decision="allow"|"prompt"|"forbidden")`), or through the session's approval/sandbox flow — not by pre-approving an unrestricted execution token in this fence.

Other harnesses: keep the same coordination scope (read/search, journal-friendly status commands, Linear MCP when configured) and include that product's native task or checklist surface only when it exists as a real allowlist entry — never invent a Claude tool name on a non-Claude product.

## Other Agent Roles (Claude Code Tokens)

The fences below are Claude Code allowlist tokens. Paste only into Claude Code configuration — map the same roles to another product's vocabulary when configuring that product.

### Backend Developer (Claude Code)

```
# Claude Code — full development permissions
Read, Write, Edit, Glob, Grep
Bash(pytest *), Bash(mypy *), Bash(ruff *)
Bash(pip *), Bash(poetry *)
Bash(git add *), Bash(git commit *)
```

### Frontend Developer (Claude Code)

```
# Claude Code — frontend toolchain
Read, Write, Edit, Glob, Grep
Bash(npm *), Bash(pnpm *), Bash(yarn *)
Bash(tsc *), Bash(eslint *)
Bash(git add *), Bash(git commit *)
```

### DBA Agent (Claude Code)

```
# Claude Code — schema analysis, no direct execution
Read, Glob, Grep
Bash(psql --help), Bash(pg_dump --help)
# Direct database commands require explicit approval
```

### DevOps Agent (Claude Code)

```
# Claude Code — infrastructure management
Read, Write, Edit, Glob, Grep
Bash(docker ps *), Bash(docker images *), Bash(docker inspect *)
Bash(docker logs *), Bash(docker compose ps *)
Bash(terraform validate *)
# Apply / mutate operations (docker run/exec/rm, kubectl apply, terraform apply) require explicit approval
# Not auto-allowed (write/output flags; Bash(cmd *) cannot exclude them — see Read-Only Fence Rule):
#   docker compose config (-o/--output), terraform plan (-out), kubectl get (--profile-output writes a pprof file).
```

## Sandbox Configuration

### Claude Code

Use `/sandbox` for isolated execution environments:

#### When to Use Sandbox

- Testing untrusted code
- Running unfamiliar build systems
- Executing user-provided scripts
- CI pipeline verification

#### Sandbox Restrictions

```
# Sandboxed execution limits:
- No network access (unless explicitly allowed)
- Read-only filesystem outside working directory
- Limited process spawning
- Resource quotas (memory, CPU, time)
```

## Permission Patterns

### Escalation Pattern

Start restrictive, escalate as needed:

```
1. Begin with read-only permissions
2. Add write permissions for specific files
3. Add execution permissions for specific commands
4. Document why each permission was granted
```

### Scope Pattern

Limit permissions to task scope:

```
# Instead of blanket permissions:
Bash(*)  # Too broad

# Use scoped permissions (still mutation-capable when they dispatch into project scripts — see Read-Only Fence Rule hazard 2):
Bash(npm run test), Bash(npm run lint)
Bash(pytest tests/)
```

### Time-Limited Pattern

For sensitive operations on Claude Code:

```
# Grant temporarily for specific task
/permissions allow Bash(git push) --until session-end

# Or grant per-prompt
/permissions allow Bash(terraform apply) --once
```

## Security Considerations

### Always Require Approval

- `git push` to main/master
- `rm -rf` or recursive deletions
- Database migrations on production
- Deployment commands
- Secret/credential access

### Never Auto-Allow

- Force push operations
- Irreversible deletions
- Production deployments
- Credential management
- System configuration changes

### Audit Trail

When granting permissions:

```markdown
## Permission Grants

| Permission | Reason | Granted At | Granted By |
|------------|--------|------------|------------|
| `Bash(pytest *)` | Running test suite | 2025-01-23 | User |
| `Write(src/*)` | Feature implementation | 2025-01-23 | User |
```

## MCP Tool Permissions (When Configured)

MCPs are not bundled with Loaf — users configure them independently. Run `loaf install` to see recommendations.

### Linear MCP

```
# Safe for orchestration (if user has Linear MCP configured)
list_issues, get_issue, list_comments
create_comment, update_issue
list_initiatives, list_initiative_updates
list_project_milestones, list_project_updates, list_project_labels
```

### Serena MCP

```
# Safe for code intelligence (if user has Serena MCP configured)
find_symbol, get_symbols_overview
search_for_pattern, read_file
```

## Troubleshooting

### Too Many Permission Prompts

1. Review the harness permission surface (on Claude Code: `/permissions`)
2. Add specific allowlist entries
3. Prefer specific or scoped entries (`Bash(git status)`, `Bash(npm run test)`) over blanket wildcards (`Bash(*)`); reserve broad grants like `Bash(npm *)` for an explicitly full-development fence — that pattern admits install, publish, and exec, so it is not a generic prompt-reduction tweak

### Permission Denied Errors

1. Check the currently allowed tools surface (on Claude Code: `/allowed-tools`)
2. Verify command matches allowlist exactly
3. Consider if permission should be granted

### Sandbox Escapes

1. Review sandbox configuration
2. Ensure network isolation is complete
3. Verify filesystem restrictions

## Network Access Posture

Combining skills with network access creates a risk path for data exfiltration. Skills can instruct agents to fetch or send data over the network, and broad network permissions amplify this risk.

### Default Posture

- **No network tools unless explicitly required** — don't include `WebFetch`, `Bash(curl *)`, or `Bash(wget *)` in default agent allowlists
- **Do not constrain network with Bash URL patterns** — a trailing `*` spans arguments (hazard 3), and flag reordering / redirects / variables bypass string matches. Prefer `WebFetch(domain:api.example.com)` for allowed domains, or deny `curl`/`wget` and enforce destinations in a PreToolUse hook
- **Never combine broad network + broad file read** in the same agent session without explicit approval
- **Prefer MCP tools for external APIs** — MCP servers handle auth at the transport layer, keeping credentials out of prompts

### Authenticated API Calls

When agents need to call authenticated APIs:

```
# Good: domain-scoped WebFetch (or an MCP tool that holds the credential)
WebFetch(domain:api.example.com)

# Also good: environment variable reference when a Bash network call is explicitly approved
# (still not a domain boundary — keep curl/wget deny-listed unless the session truly needs them)
# Authorization: Bearer $API_TOKEN

# Bad: literal credential in prompt
"Use API key sk-abc123 to call the API"

# Bad: Bash URL wildcard pretending to be domain-scoped (hazard 3)
# Bash(curl https://api.example.com/*)
```

**Rules:**
- Store credentials in environment variables or MCP server configuration
- Agent prompts should reference `$ENV_VAR` patterns, never literal secrets
- Use MCP tools (Linear, Serena, etc.) which handle auth internally
- Log which external calls agents make for audit purposes

## Best Practices

1. **Start restrictive** - add permissions as needed
2. **Document grants** - track why permissions were added
3. **Scope narrowly** - use specific patterns over broad wildcards
4. **Review regularly** - audit permissions at session boundaries
5. **Separate concerns** - different permission sets for different roles
6. **Isolate network access** — grant only to agents that need it; prefer `WebFetch(domain:…)` over Bash URL wildcards (hazard 3)
