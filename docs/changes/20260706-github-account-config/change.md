---
change: github-account-config
created: 2026-07-06
branch: github-account-config
---

<!-- Frontmatter must open the file at byte one. No status-like frontmatter: readiness is derived from the executable sections below. -->

# GitHub Account Config

## Problem

PR #91 received a GitHub interaction from the wrong account. Loaf currently relies on ambient `gh` authentication, so personal and work repos can silently use whichever GitHub CLI account is active globally. That is unsafe for PR comments, releases, branch cleanup, and any future GitHub-backed tracker work.

## Hypothesis

If each repo can declare its expected GitHub account in `.agents/loaf.json`, then Loaf can block `gh` commands and Loaf-owned GitHub release actions before they run with the wrong active account.

## Scope

**In**

- Add `integrations.github.account` as the repo-level configured GitHub login.
- Opt this repo into the contract with `levifig`.
- Add a blocking `github-account` check hook for Bash commands that invoke `gh`.
- Guard `loaf release` GitHub release creation and `loaf release --post-merge` GitHub lookups/actions with the same account check.
- Add `loaf config check` so existing projects can detect stale `.agents/loaf.json` defaults and installed Loaf-managed hook config after new hooks ship.
- Add focused tests for matching and mismatched account behavior.

**Out** (deferred, not rejected)

- Automatically switching `gh` accounts.
- Storing GitHub tokens or secrets.
- GitHub Issues or GitHub Projects tracker integration.
- Rewriting every skill reference to GitHub in this pass.

**Cut** (explicitly rejected)

- Mutating global `gh` state behind the user's back with `gh auth switch`.
- Treating missing `integrations.github.account` as an immediate failure for every existing Loaf project.

## Observable Workflow

Projects can configure:

```json
{
  "integrations": {
    "github": {
      "account": "levifig"
    }
  }
}
```

When an agent or user runs `gh ...` through a Loaf-installed Bash pre-tool hook, Loaf checks `gh auth status --active --hostname github.com --json hosts`. If the active login differs, the command is blocked with a fix command: `gh auth switch --hostname github.com --user <account>`.

`loaf release` performs the same check before creating a GitHub release draft, and `loaf release --post-merge` performs it before any `gh` repo lookup, release lookup, PR lookup, or release creation.

Existing projects can run `loaf config check` to validate `.agents/loaf.json` and installed Loaf-managed hook files. If a release added a hook such as `github-account`, `loaf config check --fix` refreshes stale installed target artifacts through Loaf's normal target installers rather than asking users or agents to hand-edit hook JSON.

## Rabbit Holes and No-Gos

- Do not add secrets to `.agents/loaf.json`; account login is public identity metadata only.
- Do not make Loaf choose credentials automatically. The first hard requirement is preventing accidental wrong-account writes.
- Do not build the full GitHub backend abstraction here.

## Decisions

Provenance: dogfooding failure from PR #91 and user direction to address the wrong-account issue ASAP.

1. **Fail fast instead of switching accounts.** `gh auth switch` changes global GitHub CLI state for the host; Loaf should tell the user exactly what to run rather than mutate identity for other repos or terminals.
2. **Use `.agents/loaf.json` project config.** ADR-007 already establishes `.agents/loaf.json` as the repo-level config surface for integrations.
3. **Missing account config is pass-through.** Existing projects should not break until they declare a GitHub account. Once declared, mismatches are blocking.
4. **Guard both human shell commands and Loaf-owned release commands.** Hooks catch direct `gh ...`; release code must guard itself because it shells out to `gh` internally.
5. **Expose config drift as a first-class CLI check.** Hook changes affect already-installed projects, so Loaf needs a focused command that validates project config and installed managed hook config without requiring a full mental model of target-specific files.

## Planning Contract

### Config

Read `integrations.github.account` from `.agents/loaf.json`. Accept `login` and `username` as fallback field names in the parser to avoid needless future schema churn, but document and emit `account` as the canonical field.

### Hook

Register `github-account` as a blocking pre-tool hook for Bash. The hook should parse the Bash command and only run `gh auth status` when the command invokes `gh`, including `cd repo && gh ...`, `env GH_HOST=github.com gh ...`, and `GH_HOST=github.com gh ...`.

### Release

Call the same configured-account check before any release path performs GitHub writes or lookups:

- `loaf release` before `gh release create`;
- `loaf release --post-merge` before base detection, because base detection may call `gh repo view`.

## Implementation Units

- **U1 - Config and account checker.** Add a shared parser/checker for `.agents/loaf.json` and `gh auth status --json hosts`.
- **U2 - Hook enforcement.** Add `loaf check --hook github-account` and register it in generated hook config.
- **U3 - Release enforcement.** Guard release GitHub interactions against account mismatch.
- **U4 - Repo opt-in.** Configure this repo to require GitHub account `levifig`.
- **U5 - Config health command.** Add `loaf config check [--fix] [--json]` for `.agents/loaf.json` defaults and installed managed hook drift.
- **U6 - Regression coverage.** Test mismatch, match, shell detection, post-merge release abort-before-lookup behavior, config creation, and stale managed-hook refresh.

## Verification Contract

<!-- Executable (machine-checkable): -->

- **V1.** `go test ./internal/cli -run 'TestGitHubAccount|TestShellCommandUsesGitHubCLI|TestReleasePostMergeGuardrailsAbortOnGitHubAccountMismatch'` passes.
- **V2.** `go test ./...` passes.
- **V3.** `loaf change check --require-executable docs/changes/20260706-github-account-config` passes.
- **V4.** `go test ./internal/cli -run 'TestRunnerConfig'` passes.

<!-- Human review: -->

- **H1.** Reviewer confirms the fix prevents wrong-account GitHub interactions without storing tokens or silently switching accounts.

## Definition of Done

- This repo declares its expected GitHub account.
- Any `gh` command through Loaf hooks is blocked when the active account mismatches the configured repo account.
- `loaf release` cannot create or inspect GitHub release state before validating the configured account.
- `loaf config check --fix` can create missing safe config defaults and refresh installed managed hooks that predate `github-account`.
- Tests pin both the account mismatch behavior and the shell forms that must be detected.

## Durable Outputs

No ADR is required for this narrow fix. The durable project-config convention can be documented later when GitHub integration grows beyond account enforcement.

## Open Questions

- Should future `loaf github ...` or tracker commands run with a repo-scoped `GH_CONFIG_DIR` instead of the user's global `gh` config? Deferred until Loaf owns more GitHub operations.
- Should `loaf config check` eventually validate more integration-specific required fields, such as Linear workspace metadata? Deferred until each integration has a clear schema contract.

## Source Inputs

- PR #91 dogfood incident: a GitHub comment was authored with the wrong account.
- User direction in this conversation: repo/project config should associate the correct GitHub account so `gh` and GitHub interactions use the right personal or work identity.
