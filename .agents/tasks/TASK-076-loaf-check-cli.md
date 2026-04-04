---
id: TASK-076
title: Implement loaf check CLI command
spec: SPEC-020
status: todo
priority: P0
created: '2026-04-04T16:41:22.295Z'
updated: '2026-04-04T16:41:22.295Z'
---

# TASK-076: Implement `loaf check` CLI command

New CLI command implementing the 5-6 enforcement hook checks in TypeScript.

## Scope

Create `cli/commands/check.ts` with Commander.js subcommand:

```bash
loaf check --hook <hook-id> [--json] < /dev/stdin
```

**Checks:**
1. `check-secrets` — Scan for hardcoded secrets, API keys, credentials in file content or Bash commands
2. `validate-push` — Verify version bump, CHANGELOG entry, and successful build before `git push`
3. `workflow-pre-pr` — Enforce PR title format and CHANGELOG entry before `gh pr create`
4. `validate-commit` — Validate Conventional Commits conventions
5. `security-audit` — Detect dangerous Bash command patterns (rm -rf /, chmod 777, eval of untrusted input)

**Interface:**
- Reads hook context via stdin (JSON from harness)
- Exit 0 = pass (including warnings), 2 = block, 1 = internal error only
- Stdout: plain text, `WARN:` prefix for warnings
- `--json` flag for structured machine output

**Port logic** from existing shell scripts into TypeScript. The shell scripts are the source of truth for what each check does.

## Constraints

- One invocation per hook — no multi-hook batching
- Harness-agnostic: same binary, same exit codes for all targets
- Exit 1 is NEVER an intentional check result — only internal errors

## Verification

- [ ] `echo '{"tool":{"name":"Bash"}}' | loaf check --hook check-secrets` returns correct exit code
- [ ] All 5 hook IDs work correctly
- [ ] `--json` produces structured JSON
- [ ] Exit codes: 0=pass, 2=block, 1=error only
- [ ] `npm run typecheck` and `npm run test` pass
