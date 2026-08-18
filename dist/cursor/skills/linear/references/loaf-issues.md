# Loaf Issue Coordination

Loaf issue execution has stricter ownership boundaries than general Linear work. Use this reference whenever a Linear record is or may become a Loaf work unit.

## Contents

- Authority Model
- Adapter Prerequisite
- Creation and Adoption
- Synchronization Commands
- Field Ownership and Reconciliation
- Status Boundaries
- Descriptions and Comments
- Team and Workspace Routing
- Git References
- Critical Rules

## Authority Model

Read `.agents/loaf.json` before coordinating a Linear-backed issue:

```json
{
  "issue": {
    "authority": "linear",
    "prefix": "ENG"
  },
  "integrations": {
    "linear": { "enabled": true }
  }
}
```

`integrations.linear.enabled` records whether the project configured the integration. It does not elect the issue backend. `issue.authority` is the execution identity contract, and `issue.prefix` is the Linear team key when authority is `linear`.

When Linear is the authority:

| Owner | Fields and behavior |
|-------|---------------------|
| Linear | Identifier, title, tracker workflow state, assignment |
| Loaf | Shaping body, definition-of-done criteria, claims, started worktree, local event history |

The Loaf issue remains the work unit. Linear MCP is an overlay for reads, comments, and unrelated collaboration; it must not silently drive Loaf status.

Elect the backend explicitly:

```text
loaf issue identity --authority linear --prefix ENG
```

Do not infer Linear authority from the presence of an MCP server or from `integrations.linear.enabled`.

## Adapter Prerequisite

Linear MCP authentication and Loaf CLI adapter authentication are independent. Generic Linear MCP workflows use the selected provider tool's OAuth or session, while `loaf issue new` under Linear authority and `loaf issue pull`, `loaf issue push`, and `loaf issue reconcile` use `LinearClientFromEnv`; the environment running `loaf` must expose `LINEAR_API_KEY` before those adapter commands run.

A working MCP connection does not satisfy this prerequisite. If `LINEAR_API_KEY` is absent, surface the CLI's exact `LINEAR_API_KEY is not set` error and stop. Do not fall back to a local identifier, bypass the adapter with MCP writes, or claim that MCP authentication covers the CLI.

## Creation and Adoption

Create Loaf work through `loaf issue new`. Under Linear authority, Linear mints the identifier and the Linear key becomes the local alias; the local counter is not advanced. If Linear is offline, `loaf issue new` refuses rather than minting a local fallback. Capture the direction with `loaf spark` or `loaf idea` until identity can be minted correctly.

Do not create a would-be Loaf work unit directly through Linear MCP and leave it unbound. Adopt an existing Linear issue, including a Linear issue created before a local bind failure, with:

```text
loaf issue pull <linear-key>
loaf issue pull <linear-key> --tree
```

`--tree` adopts descendants and preserves parent edges. A single-issue pull adopts only that issue.

Parent/child decomposition belongs to `loaf issue promote` or `loaf issue new --parent`. Do not create legacy `loaf task` or `loaf spec` records or model decomposition as a label-only Linear rollup.

## Synchronization Commands

```text
loaf issue pull <linear-key> [--tree] [--json]
loaf issue push <ref> [--json]
loaf issue reconcile [<ref>] [--take-local|--take-tracker] [--json]
```

| Command | Writes? | Contract |
|---------|---------|----------|
| `loaf issue pull` | Yes, local | Adopts an existing Linear issue. The Linear key becomes the alias; `--tree` also adopts descendants and parent edges |
| `loaf issue push` | Yes, Linear | Writes `loaf issue render` as the Linear description. Writes status only when the local status event is newer than the tracker. Never renames the Linear issue |
| `loaf issue reconcile` | Comparison plus defined resolutions | Compares mapped records. Title drift updates local because the tracker wins. Description drift is report-only. Status drift remains unresolved until a take flag is supplied |

Run reconciliation without a ref to compare every mapped issue. `--take-local` writes local status to Linear. `--take-tracker` writes the Linear status to Loaf through the local events path. The flags are mutually exclusive; reconciliation never applies silent last-writer-wins.

## Field Ownership and Reconciliation

- **Identity:** Linear mints the key under Linear authority. Never substitute a temporary Loaf alias.
- **Title:** Linear wins. `loaf issue push` never renames the Linear issue; reconciliation applies tracker title drift locally.
- **Description:** Loaf owns shaping. `loaf issue push` publishes the exact `loaf issue render` output. Reconciliation reports description drift but never imports the tracker description over local shaping.
- **Status:** Resolve only through Loaf status events and the adapter commands. A direct MCP status edit does not authorize a corresponding local status change.
- **Assignment:** Linear owns assignment. Read the current team and user before changing it through the selected Linear MCP.
- **Comments and collaboration metadata:** MCP writes are allowed when the user requested them and they do not compete with an owned field.

## Status Boundaries

Loaf status changes use `loaf issue status`, with worktree claims handled by `loaf issue start` and `loaf issue stop`. Linear workflow names may vary, so the adapter maps their state types to Loaf statuses and may use configured overrides.

| Work event | Loaf action | Linear coordination |
|------------|-------------|---------------------|
| Work begins | `loaf issue start <ref>` sets `active` and creates the started worktree | Publish through `loaf issue push` or reconcile; do not set local state from MCP |
| Work is externally blocked | Keep the issue `active`; log `block(scope)` | Add a self-contained blocker comment if useful |
| Implementation reaches review | Keep the Loaf lifecycle authoritative until ship | A tracker workflow change may be represented, but reconcile explicitly |
| Work lands | Ship sets `done`, then `loaf issue stop <ref>` removes the claim | Push or reconcile the completed state |
| Tracker and Loaf disagree | Do not guess | Run `loaf issue reconcile`; choose `--take-local` or `--take-tracker` deliberately |

Never mark a Loaf issue `done` merely because Linear says Done. Never move Linear merely because local work appears complete without using the adapter boundary.

## Descriptions and Comments

The Linear issue description for a mapped Loaf issue is `loaf issue push` output: the exact `loaf issue render`, not a competing hand-authored summary.

Comments use the Linear skill's progress format:

```markdown
## Progress

- [x] Completed outcome
- [ ] Pending outcome

## Blockers

None currently.
```

Keep comments succinct, outcome-focused, and understandable without local context. Do not include absolute file paths, agent or session details, council artifacts, internal journal prose, or invented estimates.

For a blocker:

```markdown
## Blockers

### Blocker title

**Impact**: What cannot proceed
**Needed**: What resolves the blocker
**ETA**: Known date or TBD
```

## Team and Workspace Routing

Select the configured Linear MCP using the parent skill's server-selection workflow. Verify the workspace and team before creating, assigning, or moving work. When multiple servers could reach different workspaces, do not infer the destination from a team key alone.

For a new issue, read the available teams and compare them with the work's domain. Suggest the best-supported team, name any plausible alternative, and ask when the choice changes ownership or remains ambiguous. Once the Loaf issue authority and team prefix are established, create through `loaf issue new` so identity delegation remains atomic.

## Git References

When the repository's Linear integration recognizes commit magic words, put them in the commit body rather than the subject and use only the issue identifier:

```text
feat: add token rotation

Closes ENG-123
```

Use `Fixes` for bugs, `Closes` for other completed work, and `Refs` or `Part of` for non-closing references. Do not duplicate the issue title after the identifier or claim closure for partial work.

Use branches created by `loaf issue start`; do not replace Loaf's started-worktree contract with a hand-authored Linear branch name.

## Critical Rules

- The Loaf issue is the work unit; Linear is not a second execution ledger.
- Identity, title, description, and status follow the ownership table above.
- Create through `loaf issue new`; adopt pre-existing tracker work through `loaf issue pull`.
- Publish shaping through `loaf issue push`; reconcile drift through `loaf issue reconcile`.
- Never drive Loaf status from a generic MCP mutation.
- Never paste a hand-authored description over `loaf issue render`.
- Keep comments self-contained, succinct, and free of local-only process details.
