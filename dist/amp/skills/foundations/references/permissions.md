# Permissions Configuration

## Contents
- Overview
- Permission Commands
- Recommended Allowlists
- Agent-Specific Recommendations
- Sandbox Configuration
- Permission Patterns
- Security Considerations
- MCP Tool Permissions
- Troubleshooting
- Best Practices

Permission patterns for autonomous operation and interactive workflows.

## Overview

Claude Code uses permission prompts to protect against unintended actions. Configure permissions to reduce interruptions while maintaining appropriate safety.

## Permission Commands

| Command | Purpose |
|---------|---------|
| `/permissions` | View and modify permission allowlists |
| `/allowed-tools` | Show currently allowed tools |
| `/sandbox` | Configure sandboxed execution environment |

## Recommended Allowlists

### Development Work

For standard development sessions:

```
# Read operations (safe by default)
Read, Glob, Grep, WebFetch

# Write operations (allow for active development)
Write, Edit, NotebookEdit

# Execution (allow with caution)
Bash(npm run *), Bash(pytest *), Bash(make *)
Bash(git status), Bash(git diff), Bash(git log *)
```

### CI/Autonomous Mode

For unattended execution in CI pipelines:

```
# Strict read-only
Read, Glob, Grep

# Build commands only
Bash(npm run build), Bash(npm run test)
Bash(pytest *), Bash(mypy *)

# No write permissions in CI
# Write, Edit - DISABLED
```

### Code Review

For review-focused sessions:

```
# Read everything
Read, Glob, Grep

# Analysis tools
Bash(git diff *), Bash(git log *)
Bash(npm run lint), Bash(pytest --collect-only)

# No modifications
# Write, Edit - DISABLED
```

## Agent-Specific Recommendations

### Orchestrator

```
# Coordination only - no implementation
Read, Glob, Grep
TodoWrite, TodoRead
Linear MCP tools (if configured)
Bash(date *), Bash(git status)
```

### Backend Developer

```
# Full development permissions
Read, Write, Edit, Glob, Grep
Bash(pytest *), Bash(mypy *), Bash(ruff *)
Bash(pip *), Bash(poetry *)
Bash(git add *), Bash(git commit *)
```

### Frontend Developer

```
# Frontend toolchain
Read, Write, Edit, Glob, Grep
Bash(npm *), Bash(pnpm *), Bash(yarn *)
Bash(tsc *), Bash(eslint *)
Bash(git add *), Bash(git commit *)
```

### DBA Agent

```
# Schema analysis, no direct execution
Read, Glob, Grep
Bash(psql --help), Bash(pg_dump --help)
# Direct database commands require explicit approval
```

### DevOps Agent

```
# Infrastructure management
Read, Write, Edit, Glob, Grep
Bash(docker *), Bash(kubectl get *)
Bash(terraform plan *), Bash(terraform validate *)
# Apply operations require explicit approval
```

## Sandbox Configuration

Use `/sandbox` for isolated execution environments:

### When to Use Sandbox

- Testing untrusted code
- Running unfamiliar build systems
- Executing user-provided scripts
- CI pipeline verification

### Sandbox Restrictions

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

# Use scoped permissions:
Bash(npm run test), Bash(npm run lint)
Bash(pytest tests/)
```

### Time-Limited Pattern

For sensitive operations:

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

MCPs are not bundled with Loaf — users configure them independently.
Run `loaf install` to see recommendations.

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

1. Review `/permissions` output
2. Add specific allowlist entries
3. Use patterns (`Bash(npm *)`) not wildcards (`Bash(*)`)

### Permission Denied Errors

1. Check `/allowed-tools`
2. Verify command matches allowlist exactly
3. Consider if permission should be granted

### Sandbox Escapes

1. Review sandbox configuration
2. Ensure network isolation is complete
3. Verify filesystem restrictions

## Network Access Posture

Combining skills with network access creates a risk path for data exfiltration. Skills can instruct agents to fetch or send data over the network, and broad network permissions amplify this risk.

### Default Posture

- **No network tools unless explicitly required** - Don't include `WebFetch`, `Bash(curl *)`, or `Bash(wget *)` in default agent allowlists
- **Scope to specific domains** - When network access is needed, use `Bash(curl https://api.example.com/*)` not `Bash(curl *)`
- **Never combine broad network + broad file read** in the same agent session without explicit approval
- **Prefer MCP tools for external APIs** - MCP servers handle auth at the transport layer, keeping credentials out of prompts

### Authenticated API Calls

When agents need to call authenticated APIs:

```
# Good: environment variable reference (agent never sees the secret)
Bash(curl -H "Authorization: Bearer $API_TOKEN" https://api.example.com/*)

# Bad: literal credential in prompt
"Use API key sk-abc123 to call the API"
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
6. **Isolate network access** - grant only to agents that need it, scope to specific domains
