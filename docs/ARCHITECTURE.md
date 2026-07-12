# Loaf Architecture

## Current Architecture (v2.0)

```
cmd/loaf/                       # Go CLI entry point
internal/cli/                   # Native command dispatcher, command families, build helpers
cli/                            # Portable launcher plus JS build/verifier/smoke/eval scripts
├── runtime/                    # Node launcher wrapper
└── scripts/                    # JS build, verification, smoke, and evaluation scripts

content/                        # Distributable content (separated from tooling)
├── skills/{name}/SKILL.md      # Domain knowledge (Agent Skills standard)
├── agents/{name}.md            # Functional profiles (tool boundaries + behavioral contracts)
├── hooks/                      # Enforcement + instruction hook scripts
└── templates/                  # Shared templates (distributed at build time)

config/
├── hooks.yaml                  # Hook definitions (enforcement, instruction, SessionStart digest)
└── targets.yaml                # Target defaults + shared-templates mapping

Output:
├── bin/loaf                    # Portable launcher
├── bin/native/{platform}/loaf   # Native Go runtime
├── plugins/loaf/               # Claude Code plugin (hooks, skills, agents, binary)
└── dist/{target}/              # Other targets (cursor, opencode, codex, amp)
```

### Build Flow

```
cmd/loaf + internal/cli -> go build -> bin/native/{platform}/loaf
content/ + config/ -> loaf build -> dist/ + plugins/
```

Each target transformer reads content (skills/agents/hooks) and config, then produces target-specific output. Skills get sidecar files merged. Hooks get registered in plugin manifests. Shared templates get distributed to specified skills.

The public runtime and CLI reference generation are native Go. Remaining non-Go files under `cli/` are JavaScript launcher/build/smoke/evaluation scripts, not TypeScript command implementations or tests.

### Stateful Runtime Migration (ADR-014)

ADR-014 accepts Go as the runtime direction for Loaf's stateful core. Native Go is now the shipped public runtime; TypeScript command registrations, the shipped fallback bundle, and the local TypeScript test harness have been removed from the active CLI surface.

The transition shape is a Go front controller for the public `loaf` command:

```
loaf                     # Go front controller
└── native Go commands    # stateful/runtime-heavy behavior and migrated public commands
```

The former TypeScript bridge prevented a big-bang rewrite. Public commands have moved to Go for the stateful runtime, storage layer, and lower-dependency distribution shape. Historical ADRs still describe the transition, but the active `cli/` tree no longer contains TypeScript source or test files.

This changes the construction technique, not the product contract: skills still call `loaf`, hooks still enforce through `loaf`, and users still see one command surface. The implementation boundary behind that command is allowed to migrate command-by-command.

### Operational State Identity

Loaf stores operational state in one global SQLite database at `$XDG_DATA_HOME/loaf/loaf.sqlite`, partitioned by project ID. New project IDs are generated and stored in SQLite; they are not derived from checkout path or friendly name. The `projects` row carries the friendly display name and current path, while `project_paths` records path mappings so a checkout can move without changing identity. Legacy path-hash IDs remain only as an adoption key for migrated pre-stable-identity data.

### Recovery Tiers and Restore Safety

Recovery has three named tiers: `local_rollback` snapshots remain in the same data home for local corruption rollback, project-scoped replay is the ordinary rollback mechanism for later migrations, and `external_disaster_copy` is an operator-selected non-temporary external destination for a point-in-time copy. An explicit destination is resolved through symlinks and rejected when it is absolute-but-volatile; the path check does not prove that the destination is physically remote or durable, so `device_loss_protected` remains false. Backup and verification results include SQLite validity, journal retrieval readiness, search parity, project evidence, checksum, and the latest canonical journal watermark.

`loaf state backup restore <backup> --to <absolute-empty-database-path>` is an isolated disposable rehearsal. It creates an exact copy at an empty target, verifies integrity, foreign keys, schema, projects, canonical journal rows, derived search parity, and the watermark, and leaves the live database untouched. There is no automated live activation, no universal mutation lease honored by every writer, and no claim that a concurrent restore is safe.

Live activation is therefore a quiesced operator procedure: stop or terminate every harness, Loaf process, background writer, and process that might retain an open database connection; verify the backup and isolated rehearsal; retain a preserve-current backup; while quiesced move the old main database and any matching `-wal` and `-shm` sidecars together into durable quarantine; install the verified copy with mode `0600`; start current Loaf; run `loaf state doctor`, `loaf state status`, and a known journal retrieval check; and, on failure, quiesce again and activate the preserve-current copy. Sidecars from different database files must never be mixed.

### Targets

| Target | Output | Agents | Skills | Hooks | Runtime Plugin |
|--------|--------|:------:|:------:|:-----:|:--------------:|
| claude-code | plugins/loaf/ | Yes | Yes | Yes | plugin.json |
| cursor | dist/cursor/ | Yes | Yes | Yes | hooks.json |
| opencode | dist/opencode/ | Yes | Yes | Yes | hooks.ts |
| codex | dist/codex/ | No | Yes | Yes | hooks.json |
| amp | dist/amp/ | No | Yes | No | .amp/plugins/loaf.ts |

### Amp Plugin API Constraints

Amp's plugin API is intentionally minimal. Plugin handlers are dispatched via `handleRequest()` for exactly four event names:

- `tool.call` — before a tool is invoked
- `tool.result` — after a tool returns
- `agent.start` — when an agent begins a turn
- `agent.end` — when an agent finishes a turn

There is no session-lifecycle dispatch. Amp's binary internally emits `emitEvent("session.start", ...)` for telemetry purposes, but this is not exposed to plugins. Features that require a SessionStart hook (the journal continuity digest, SOUL.md self-healing) or PreCompact flushes are not viable on Amp without upstream support. Loaf's Amp target is scoped to tool events only; the SessionStart digest that other targets ship is intentionally absent here.

This was discovered during SPEC-033 review (PR #40). An earlier wiring attempted to map `sessionEnd` to `agent.end` (turn-end) — semantically wrong and now reverted.

### Prompt Overlay Consolidation (ADR-010)

The managed fenced section is written once to a canonical file (`.agents/AGENTS.md`). Per-harness paths are symlinks to it.

```
.agents/AGENTS.md                         # Canonical (source of truth, committed)
.claude/CLAUDE.md        → symlink →      .agents/AGENTS.md
./AGENTS.md              → symlink →      .agents/AGENTS.md  (agents.md spec)
```

**Write path (`loaf install`):** the native Go installer resolves each target's destination via `realpath` and groups writes by canonical path. Five of six targets share `.agents/AGENTS.md`, so they produce a single write. Before writing, the native project-symlink installer runs the 4-state machine per link:

| State | Action |
|-------|--------|
| nothing at linkPath | Create symlink |
| correct symlink | No-op (silent) |
| wrong symlink | Prompt to relink (auto-yes under `--yes`) |
| regular file | Prompt to merge content into canonical under `## Migrated from <path>`, back up as `.bak`, then symlink |

Fresh installs pre-create an empty canonical shell so symlinks are never dangling. `--yes` flag and non-TTY auto-detection allow the flow to run under CI/skills without interactive prompts.

**Config health (`loaf config check`):** the native CLI validates `.agents/loaf.json` and installed Loaf-managed hook config separately. `--fix` creates missing safe project-config defaults and refreshes stale installed target artifacts through the same target installers as `loaf install`, so new hooks such as `github-account` can be propagated without hand-editing target config files.

**Drift detection (`loaf doctor`):** Six checks — canonical presence, per-harness symlink target, stale `.cursor/rules/loaf.mdc`, fenced-section version match, duplicate-resolved writes, and target coverage. `loaf doctor --fix` applies safe repairs non-interactively.

This extends the "CLI is the correct protocol layer" principle to filesystem convention enforcement: the CLI owns the on-disk overlay state, not the skills or the user. When ADR-010 shipped, five harnesses went from "each writes its own file" to "each resolves to the same file" without any skill edits.

### Mode-Aware Skills (Linear-Native Mode, ADR-011)

Skills that orchestrate specs and tasks (`/breakdown`, `/implement`, `/housekeeping`, `/shape`, `/council`) branch on `integrations.linear.enabled` in `.agents/loaf.json`:

- **Local-tasks mode** (default): specs stay in `.agents/specs/`; task, journal, idea, spark, brainstorm, and draft records live in the global SQLite database.
- **Linear-native mode**: specs stay in `.agents/specs/` (canonical, deliberation layer); tasks move to Linear as sub-issues under a `spec`-labeled parent rollup issue (execution layer).

The split reflects an architectural principle from ADR-010's consolidation pattern extended to the spec/task artifact model:

- **Deliberation artifacts belong with code.** Specs, ADRs, councils. Need git history, code-adjacent visibility, travel with the branch, survive the tracker being down or switched.
- **Execution artifacts belong in the tracker.** Tasks, blockers, comments, assignees. Need real-time state, dashboards, blocking graphs, notifications.

In Linear-native mode, the parent Linear issue is a **canonical-elsewhere rollup** — summary + link to the local spec file, not a re-host. Parallels the `.agents/AGENTS.md` → per-harness symlink pattern from ADR-010: the canonical artifact exists in exactly one place; the other surface is a thin pointer.

Skills detect the mode and branch accordingly. No skill edits are required to switch modes — same skill content, different backend. This sets up SPEC-023's backend abstraction as a narrower refactor than originally scoped: the contract is already mode-aware; SPEC-023 just extracts the Linear MCP calls into a shared `tracker` CLI subcommand with pluggable implementations.

### Pre-Flight Dependency Gate

`/implement`'s Linear-native routing enforces `blockedBy` as a **hard pre-flight gate**: before moving a sub-issue to `in_progress` or starting work, every issue in its `blockedBy` field must be in a `completed`-type state. If not, the skill refuses to start — no work begun, no issue moved.

This is different from advisory dependency ordering: the dependency graph becomes a runtime invariant, not a suggestion. The orchestrator cannot implement through open blockers even by accident.

The local-tasks equivalent (dependencies in `TASKS.json`) is still advisory. Pre-flight gating requires a system of record that can be queried reliably for all blockers' current state; Linear provides that, local files do not without a separate polling layer. This asymmetry is intentional: teams that care about strict dependency enforcement get Linear's gate for free; solo developers on local tasks trade enforcement for simplicity.

Parent-issue completion follows from the same contract: when the last sub-issue flips to `completed`, `/implement` auto-closes the parent. A parent with any open sub-issue (including `blocked`) stays open — the spec is not done until its execution tree is.

### Agent Model: Functional Profiles

Loaf uses **functional profiles** defined by tool access boundaries, not role-based agents defined by domain identity. Skills provide all domain knowledge; profiles provide the tool sandbox.

**The Orchestrator:**

The main conversation is the **orchestrator** — the coordinator that plans and delegates but does not directly implement, review, research, or curate durable artifacts.

**4 Functional Profiles:**

| Profile | Tool Access | Purpose |
|---------|-------------|---------|
| implementer | Full write | Writes code, tests, config, docs. Speciality via skills at spawn time. |
| reviewer | Read-only | Audits and verifies. Cannot modify what it reviews — independence is structural. |
| researcher | Read + Web | Investigates options, compares approaches, returns structured reports. No write or execute. |
| librarian | Read + Edit (.agents/) | Tends the project journal and durable `.agents/` artifacts, including wrap checkpoints. Does not implement or research. |

Each profile is defined in `content/agents/{implementer,reviewer,researcher,librarian}.md` — a minimal behavioral contract and tool boundary, not domain knowledge. A spawned implementer becomes a backend engineer, DBA, or devops engineer depending entirely on the skills loaded at spawn time.

**1 System Agent:**

| Agent | Purpose |
|-------|---------|
| background-runner | Async non-blocking tasks (haiku model) |

**Council Composition:**

Councils convene implementers and researchers for deliberation; reviewers join only after, to verify the outcome. The orchestrator runs the council but never votes — the team decides, the orchestrator integrates.

**Skills as Universal Knowledge Layer:**

Skills are the only knowledge mechanism that works across all targets (Claude Code, Cursor, Codex, Amp). Profiles are Claude Code infrastructure — other targets activate knowledge through skills alone. This makes skills the primary investment surface: better skill descriptions and organization improve all targets simultaneously.

## Operating Principles

Principles that shape how Loaf is designed and operated. Unlike ADRs, these are mutable and evolve via `/reflect` as the project learns. They sit above implementation choices but below VISION (which captures product intent and direction).

### Authorship Model — Agents Create, Humans Curate

Agents are the primary authors of knowledge files, ADRs, tasks, and specs. Humans review, approve, and curate — they are not the writing surface. The CLI follows from this: it is for management and health checks (`check`, `validate`, `status`, `review`), not for authoring.

The principle inverts the traditional "humans write docs, agents consume them" model. Agents are already doing the work and are closest to what's being learned; pulling knowledge creation into the work itself ("maintenance as side effect of work") is cheaper than treating documentation as a separate sprint. Humans are better at judgment — *is this worth recording?* — than at the writing.

The growth loop is concrete: agent discovers an insight during brainstorming, development, or debugging → proposes a knowledge file, ADR, task, or spec → human reviews and accepts, edits, or rejects → committed. Quality control depends on human review; agents may write redundant or low-quality material that the curator catches. Hooks (PostToolUse, PreCompact) prompt agents at the moments where insights are freshest, so the proposal isn't deferred until the context is gone.

This principle shapes skill design and CLI surface; it is mutable and evolves via `/reflect`.

### Adversarial Review for Substantive Guidance Changes

Substantive changes to skills, guidance docs, or operating principles warrant review beyond the implementer's own check. The Loaf baseline is `loaf:reviewer` (internal-consistency auditor). When available, an adversarial design stress-tester (`codex:rescue` or equivalent) is highly recommended — the two readers catch different defect classes:

- **Internal-consistency review** (`loaf:reviewer`) surfaces stale references, anchor breaks, prose drift, and contradictions between sections.
- **Adversarial design review** (`codex:rescue`, optional) stress-tests the design itself for false positives, false negatives, and self-contradictions; constructs decision examples the rules don't handle cleanly.

Codex is plugin-dependent — it may not be available in all environments. `loaf:reviewer` is the floor; the adversarial pass is recommended when the change is substantive enough that a design defect would compound across many future invocations (skill rewrites, lifecycle codifications, hook-policy changes).

This principle shapes how Loaf evolves substantive guidance. Evidence: the architecture-skill tightening + ADR deprecations (PR #46) shipped through three review-driven refinement rounds, with each reviewer catching defect classes the other missed.

### Recategorization as a General Lifecycle Pattern

Loaf artifacts evolve in two distinct ways:

- **Supersession** — the underlying answer changed; a new artifact replaces the old. The old is preserved as historical record (`status: Superseded`, `superseded_by:` linkage). Used for ADRs whose decisions changed.
- **Recategorization** — the underlying rule still holds, but the artifact's classification was wrong from the outset. The artifact is deprecated in place (`status: Deprecated`, `migrated_to:` reference in the body), and the rule's active source moves to its appropriate home.

Recategorization emerged from PR #46: three ADRs whose conventions/principles still held had been classified as architectural decisions when they were actually a naming convention, an operating principle, and skill-specific workflow lore. Supersession (write a new ADR replacing each) was the wrong tool — there was nothing to replace, only to relabel. Recategorization preserves the historical record without overstating its current authority.

This pattern generalizes beyond ADRs. When any Loaf artifact is later judged to have been classified wrong but its content is still valid, recategorize: deprecate the original, point to the new canonical home, leave the body intact for archeology.

## Execution Model

The execution model is a three-artifact pipeline. No separate "plan" artifact — the journal serves as both execution trace and resumption protocol.

```
/shape → SPEC file → /breakdown → SQLite tasks → /implement → project journal → Done
```

### Task System

```
.agents/specs/SPEC-XXX.md       # Bounded work definitions (scope, test conditions, priority order)
SQLite tasks                     # Individual work items (criteria, file hints, verification)
SQLite journal_entries           # Project-scoped event record across every conversation
```

**Specs** define *what* to build — problem, solution direction, boundaries, test conditions. Multi-part specs use priority ordering with go/no-go gates between tracks (ship in order, drop from end). Sized by complexity (small/medium/large), not time.

**Tasks** define *what to do* — one concern per task, file hints, verification command, observable done condition. Created by `/breakdown`, worked by `/implement`.

**The journal** captures *what happened* — `journal_entries` rows are project-scoped events (`project_id NOT NULL`), each tagged with an opaque `harness_session_id` that correlates one conversation's entries. Decisions, discoveries, commits, and progress land as structured entries; `loaf journal recent`/`show`/`search` and the `loaf journal context` digest provide handoff-ready context for compaction recovery and cross-conversation resumption. There is no session entity — see [Session Model: Journal-First](#session-model-journal-first).

`.agents/tasks/`, `.agents/ideas/`, `.agents/sparks/`,
`.agents/brainstorms/`, `.agents/drafts/`, and `.agents/TASKS.json` are
rollback material after SPEC-045, not compatibility mirrors. A stale branch that
reintroduces them should keep the deletion side and rerun
`loaf check --hook ephemeral-provenance`. Legacy `.agents/sessions/` markdown is
also gone: the journal is SQLite-native and never rendered to a hand-authored
source file.

### Session Model: Journal-First

The project journal is the **only** session-related structure (SPEC-056). There is no session entity — no `sessions` table, no statuses, no lifecycle, no rotation. `journal_entries` are project-scoped events (`project_id NOT NULL`) in the global SQLite database, each carrying an opaque `harness_session_id` column that correlates the entries written by one conversation. Nobody opens, closes, or transitions a session; nothing is ever "unwrapped."

This supersedes the SPEC-048 `start → log → end --wrap` session lifecycle and its six-state entity: production data showed the lifecycle never maintained itself (stuck `active`/`paused` records, empty snapshots, ~24% lifecycle-noise entries), and its rotation semantics actively fought concurrency. **Concurrent conversations on the same project — across branches, worktrees, even harnesses — are safe by construction:** two conversations logging at once simply interleave rows with different `harness_session_id` tags, which is correct rather than corrupt.

**Logging:** `loaf journal log "type(scope): description"` appends a durable entry; the current branch and harness id are attached automatically. Skills self-log their invocation as their first action; the `session` entry type is gone.

**Wrap is an optional checkpoint, not a transition.** A `wrap` entry is written only when a conversation holds synthesis worth saving — "tried X, abandoned because Y, next is Z" — the connective narrative that evaporates with the context window. Everything else is derivable from raw entries. A conversation that ends abruptly leaves a perfectly valid journal. A wrap claims the writing conversation's own entries (its `harness_session_id`); a manual/untagged wrap falls back to branch scope.

**Continuity is derived, layered, and ephemeral.** At conversation start the SessionStart hook runs `loaf journal context --from-hook`, which emits a digest computed at read time: the latest project-level `wrap` + recent entries scoped to the current branch/worktree + open (`in_progress`/`pending`) tasks. The digest is shown, then discarded — never persisted, because auto-persisting arrival syntheses would re-pollute the journal with derived noise.

**Subagent detection:** Hook JSON from Claude Code includes `agent_id` only for subagents. `loaf journal context --from-hook` checks for this and exits silently, writing nothing — subagents get no digest and create no entries, preventing churn when the Task tool spawns them.

**Compaction resilience:** The journal is external memory that survives context compaction. PreCompact nudges a flush of unrecorded decisions and next actions. PostCompact re-emits the continuity digest. No separate snapshot mechanism, and no Stop/SessionEnd obligation — the SessionEnd hook was removed entirely.

### Journal Entry Sources

The journal receives entries from multiple layered sources:

| Source | Mechanism | When |
|--------|-----------|------|
| Skills | `loaf journal log` in skill Critical Rules | Self-logging on invocation |
| Git events | PostToolUse command hooks (`loaf journal log --from-hook`) | Commits, PRs, merges (automatic) |
| Task events | TaskCompleted hook (`loaf journal log --from-hook`) | Task completed/cancelled (automatic) |
| Compaction | PreCompact command hook | Journal flush nudge before compaction |
| Wrap | `loaf journal log "wrap(scope): …"` | Voluntary end-of-conversation synthesis |

Skills self-log as their first action. Git and task events are captured automatically by hooks. Continuity is read, not written: the SessionStart and PostCompact hooks emit the derived digest rather than logging entries.

**Continuation policy:**

| Scenario | Action |
|----------|--------|
| Same scope, continuing work | Compact (journal survives) |
| Different scope entirely | New conversation (journal persists project-wide) |
| Finished and archived a spec | New conversation |
| Context full mid-task | Auto-compact |
| Quick unrelated question | New conversation |

A new conversation is never a new "session" — it is just a new `harness_session_id` writing into the same project journal. Whether to wrap before switching is a judgment call about whether synthesis is worth saving, not a lifecycle requirement.

### Forward-Only In-Flight Pivots

When a PR is reviewed, approved-with-findings, and the findings reveal that a feature shouldn't ship as designed, the project favors **forward removal commits over history rewriting**.

Three reasons:

1. The squash merge nets the diff regardless — main only sees the final change.
2. Force-push invalidates review thread context (Codex citations, reviewer line refs) and is generally blocked on shared branches.
3. The PR's commit history becomes honest archeology — "we shipped, then reviewed, then pivoted" is more useful than a tidy linear story that hides the deliberation.

SPEC-033 (PR #40, v2.0.0-dev.32) is the canonical example: 13 feature commits + 2 forward-removal commits + 1 cleanup. The squash on main is clean; the PR thread shows the pivot. Future PRs that need to pivot mid-flight should follow the same pattern.

## Hook Architecture

Hooks are defined in `config/hooks.yaml` and distributed to target-specific formats at build time. For Claude Code, the canonical hook registration file is `hooks/hooks.json` (inside the plugin output directory). `plugin.json` silently drops non-matcher session events (SessionStart, PreCompact, PostCompact, TaskCompleted) — all hooks should be registered in `hooks.json`.

### Dispatch Types

| Type | Field | Behavior |
|------|-------|----------|
| script | `script:` | Runs a shell script |
| command | `command:` | Runs a CLI command (e.g., `loaf check --hook <id>`) |
| prompt | `prompt:` | Injects text directly to the AI model |

### Hook Type Behavioral Constraints

Hard-won constraints validated during SPEC-030 implementation:

- **`type: prompt`** — Binary gate. Any non-empty LLM response is treated as rejection (`ok: false`). Cannot express "this looks fine, proceed" — the response itself blocks. Unusable for advisory hooks or hooks requiring LLM judgment. Use only for validation that returns empty on success.
- **`type: agent`** — Read-only tool access (Read, Grep, Glob, WebFetch, WebSearch). No Edit, Write, or Bash. Max 50 turns. Useful for observation, not mutation.
- **`type: command`** — Correct primitive for context injection and side effects. Exit 0 with stdout for context injection. Exit 1 for non-blocking warning. Exit 2 to block the action.
- **Stop-event circularity (general caution)** — A hook that mutates state the hook chain itself monitors can re-trigger that chain. Any hook write must be idempotent or guarded against re-entry. Journal-first removes the specific hazard (there is no Stop/SessionEnd hook writing back to a session record), but the constraint still governs any future stateful hook.
- **PreCompact prompt hooks** — Not supported outside REPL sessions. Use `type: command` for PreCompact context injection.
- **`plugin.json` drops non-matcher events** — Session events (SessionStart, PreCompact, PostCompact, TaskCompleted) must be registered in `hooks/hooks.json`, not `plugin.json`.
- **UserPromptSubmit has no matcher** — Fires on every user message, cannot be filtered by tool name or input.
- **Session events use different JSON shape** — `hook_event_name` field instead of `tool_name`. TaskCompleted passes `task_subject` and `task_description`.
- **Plugin caching** — Cached plugin versions serve stale hook handlers during development. Marketplace remove/re-add is the reliable cache-busting path.
- **CLI-spawned agents need hook isolation** — When the CLI spawns `claude --agent <name> -p`, the child process triggers the SessionStart hook. Set an isolation env var in the child so Loaf's SessionStart digest does not fire in the subprocess. Do NOT use `--bare` — it breaks OAuth for subscription users.
- **`--bare` skips OAuth** — `--bare` mode requires API key auth (`ANTHROPIC_API_KEY`). Subscription users on OAuth cannot use `--bare`. Use env var isolation instead.

### Hook Categories

**Enforcement hooks** — quality gates that block bad actions. Run by `loaf check` through the native Go backend. Exit non-zero to block. `failClosed: true` means failures block the action. `github-account` converges the active GitHub CLI account on `.agents/loaf.json` before `gh` commands run — switching accounts when they differ (passing with a warning so the mutation is visible) and blocking only when the switch cannot be performed; `validate-push` restricts direct pushes to the default branch to `.agents/` and `docs/` files only. Code changes require a feature branch and pull request.

**Instruction hooks** — context injection at tool invocation. Triggered by `matcher` patterns (tool name) and optionally filtered by `if` conditions (tool input). Inject relevant skill instructions or nudges.

**Session event hooks** — tied to events (`SessionStart`, `PreCompact`, `PostCompact`, `TaskCompleted`). SessionStart emits the journal continuity digest (`loaf journal context --from-hook`); PreCompact nudges a journal flush; PostCompact re-emits the digest; TaskCompleted auto-logs completions. There is no SessionEnd or Stop journal obligation.

### Hook JSON Data Model

Claude Code passes JSON to hooks via stdin. Key fields for post-tool hooks:

| Field | Description |
|-------|-------------|
| `session_id` | Current Claude conversation ID |
| `agent_id` | Present only for subagents — the discriminator for session-aware hooks |
| `tool_name` | Name of the tool invoked (e.g., `"Bash"`) |
| `tool_input` | Arguments sent to the tool |
| `tool_response` | Result/output returned by the tool (post-tool only) |
| `cwd` | Working directory |

`loaf journal log --from-hook` uses `tool_input.command` to detect commit/PR/merge patterns and `tool_response` to extract PR numbers from output.

## Knowledge Management

```
docs/knowledge/          # Knowledge files with frontmatter (covers:, topics:, etc.)
docs/decisions/          # ADRs (immutable decision records)
.agents/loaf.json        # Project config (local KB dirs, imports, integration toggles)
```

Knowledge files are managed by `loaf kb` — staleness detection compares file modification time against configurable thresholds. SessionStart surfaces stale file counts. The `/housekeeping` skill flags stale files for review.

## Config

```
.agents/loaf.json               # Project-level (knowledge dirs, integration toggles, settings)
~/.local/share/loaf/            # User-level operational data, including SQLite state
~/.config/loaf/                 # User preferences
```

Integration toggles in `loaf.json` gate runtime features (Linear magic-word detection, MCP recommendations) without rebuilding.

## Test Fixture Hygiene

Any test that spawns a CLI subprocess must use OS-tmp isolation for its fixtures:

```go
workingDir := realpath(t, t.TempDir())
```

CWD-relative fixtures are forbidden for subprocess tests. The old Vitest suite exposed why: workers shared filesystem state and cwd, so `join(process.cwd(), ".test-...")` fixtures could race against other subprocess tests. One such leak in `cli/commands/check.test.ts` silently failed `cli/commands/report.test.ts` for 17+ commits before v2.0.0-dev.28 bisected it.

`realpath` is required on macOS because the system tmpdir (`/var/folders/...`) is reached through a `/private/var/folders/...` symlink; without realpath, subprocess cwd comparisons can fail.

The active test harness is now Go. `npm test` delegates to `go test ./...`, and `npm run typecheck` compiles all Go packages with `go test ./... -run=^$`.

## Cross-Cutting Patterns

Patterns that apply across multiple subsystems and emerged from specific post-release followups. Captured here so they inform future work rather than being re-discovered.

### Single-Source Runtime Versioning

The native CLI version must report the package version consistently through the launcher, native runtime, generated targets, and install markers. Go runtime paths read package metadata directly; the obsolete TypeScript version helper was removed after the install and version surfaces moved to native Go.

Before PR #35 (v2.0.0-dev.30), three separate runtime `package.json` walkers in those same files diverged and all fell back to `"0.0.0"` from the bundled binary — a false-positive factory for every version-comparison check in the CLI. The lesson generalizes: any value that must be identical across runtime modes should be injected at build time, not resolved at runtime.

### Generated Runtime Plugin Artifacts Parsed From Emitted Output

Files the build emits for downstream runtimes to execute — OpenCode `hooks.ts`, Amp `loaf.js`, and any future per-target runtime plugin — must have tests against the **actual emitted file**, not just the generator's input string.

Template-literal escape bugs are invisible at the source-string level: the former TypeScript build helper emitted invalid regex (`/*/g`) into `dist/opencode/plugins/hooks.ts` for multiple versions because the broken code path was unreachable at runtime. The syntactic breakage was dormant until OpenCode's plugin loader tightened its validation and rejected the file on load. Native build tests in `internal/cli/build_test.go` now read the emitted OpenCode and Amp plugin files and assert the runtime hook bodies and command payloads that downstream runtimes load.

### Visible-Degraded Fallback with Stderr WARN

When strict invariant enforcement would break existing callers but silent fallback corrupts data, emit a stderr WARN naming the missing signal and the silencing flag. The action proceeds (preserving compatibility) but the WARN makes the misroute visible in real time and provides a regression-testable surface for the eventual cutover.

SPEC-032 used this pattern for the branch-fallback session router that resolved a target session for `loaf session log`, `archive`, and `end --wrap`. **SPEC-056 superseded that mechanism:** journal-first removed the session entity the router resolved against, so there is nothing to misroute — `loaf journal log` attaches the current branch and `harness_session_id` automatically, and concurrent conversations interleave rows by construction. The WARN it emitted was the regression gate for exactly this cutover; the cutover has now landed. The pattern is retained here as a general design tool, not as a description of live session routing.

The pattern generalizes: any compatibility carve-out that violates a stated invariant should be observable in real time, not invisible. The cost (one extra stderr line for legacy callers) is paid once per invocation; the benefit (every misroute surfaces immediately) is paid forward to whoever next opens an issue saying "my entry didn't land where I expected."
