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

### Native Stateful Runtime (ADR-014)

ADR-014 records the decision to use Go for Loaf's stateful core. Native Go is the shipped public runtime; TypeScript command registrations, the fallback bundle, and the TypeScript test harness are no longer part of the active CLI surface.

The public command has one native runtime:

```
loaf                     # Native Go command surface
└── command families       # Stateful operations, build/install, checks, and project workflows
```

Historical decision records describe how the runtime moved, but the active `cli/` tree contains only JavaScript launcher, build, verification, smoke, and evaluation scripts. It does not contain TypeScript command source or tests.

General workflow skills call `loaf` for Loaf-owned state and route user-scoped external collaboration through dedicated provider skills; hooks enforce through `loaf`, and users see one deterministic Loaf command surface. ADR and SPEC identifiers cited in this document serve only as decision and work provenance.

### Operational State Identity

Loaf stores operational state in one global SQLite database at `$XDG_DATA_HOME/loaf/loaf.sqlite`, partitioned by project ID. New project IDs are generated and stored in SQLite; they are not derived from checkout path or friendly name. The `projects` row carries the friendly display name and current path, while `project_paths` records path mappings so a checkout can move without changing identity. Legacy path-hash IDs remain only as an adoption key for migrated pre-stable-identity data.

Entity identity follows the same discipline one level down (ADR-028). Derived entity IDs are mint-once opaque keys: computed at first creation, never recomputed for resolution. The aliases table — with the schema's only content-meaningful unique constraint, `UNIQUE (project_id, namespace, alias)` — is the identity registry, and the markdown importer resolves through it before deriving an ID for anything. Unaliased kinds resolve by natural key: journal entries by (entry type, scope, message) for markdown-origin rows, sources by project and path. Sparks whose message normalizes to an empty slug receive a deterministic content-hash alias so no row is born unreachable.

The standing invariant is alias parity: for every project and every aliased entity table, raw row counts equal alias-reachable counts, with zero dead aliases. `loaf state doctor` checks it on demand (read-only, error severity without invalidating the database, naming `loaf state migrate alias-orphans` as the repair). The June-24 identity fork — a project rekey silently invalidating every derived ID, repaired by the state-dedupe Change — is the incident this invariant exists to catch on day one instead of week six.

### Recovery Tiers and Restore Safety

Recovery has three named tiers: `local_rollback` snapshots remain in the same data home for local corruption rollback, project-scoped replay is the ordinary rollback mechanism for later migrations, and `external_disaster_copy` is an operator-selected non-temporary external destination for a point-in-time copy. An explicit destination is resolved through symlinks and rejected when it is absolute-but-volatile; the path check does not prove that the destination is physically remote or durable, so `device_loss_protected` remains false. Backup and verification results include SQLite validity, journal retrieval readiness, search parity, project evidence, checksum, and the latest canonical journal watermark.

`loaf state backup restore <backup> --to <absolute-empty-database-path>` is an isolated disposable rehearsal. It creates an exact copy at an empty target, verifies integrity, foreign keys, schema, projects, canonical journal rows, derived search parity, and the watermark, and leaves the live database untouched. There is no automated live activation, no universal mutation lease honored by every writer, and no claim that a concurrent restore is safe.

Live activation is therefore a quiesced operator procedure: stop or terminate every harness, Loaf process, background writer, and process that might retain an open database connection; verify the backup and isolated rehearsal; retain a preserve-current backup; while quiesced move the old main database and any matching `-wal` and `-shm` sidecars together into durable quarantine; install the verified copy with mode `0600`; start current Loaf; run `loaf state doctor`, `loaf state status`, and a known journal retrieval check; and, on failure, quiesce again and activate the preserve-current copy. Sidecars from different database files must never be mixed.

### Repair Migrations

Data surgery on the live database rides one sanctioned pattern, proven across three instances (`lifecycle-statuses`, `alias-orphans`, `journal-duplicates`): preview on a temporary copy → mandatory backup → fsynced JSON rollback manifest (file and parent directory, before COMMIT) → apply in one transaction → post-apply verification → `--rollback <manifest>` restoring every deleted row. Registered under `loaf state migrate`; a second apply is provably a no-op.

Four rules the third instance made explicit:

- **Classification iterates to a fixed point** inside the shared classifier, because proof predicates can be sensitive to what the run itself retires. Preview and apply must report the same result set; a repair that reports failure on a correct first apply is a defect (reproduced on a production copy during review, pre-merge).
- **Unproven rows refuse by default.** The migration never guesses; explicit per-row operator dispositions (`--retire`, `--realias`) are accepted in preview so the exact apply invocation is rehearsable, recorded verbatim in the manifest, and conflicting dispositions are a parse error.
- **Reference residue is swept from one shared enumeration.** The polymorphic entity-reference tables (events, relationships, entity_tags, bundle_members, backend_mappings, exports, artifact_bodies with its FTS mirror, aliases) are enumerated once and consumed by every retirement path, so repairs cannot drift from the schema or from each other.
- **FTS mirrors are derived data.** Rollback re-derives index state from restored content rows rather than restoring captured index bytes, and delete paths tolerate a desynced mirror instead of aborting (an unindexed FTS5 external-content delete raises SQLITE_CORRUPT — the tolerance probe exists because a pre-existing desync once made a repair unrunnable).

The operational gate is rehearsal on a disposable production copy: `LOAF_DB` and `XDG_DATA_HOME` redirected to a sandbox, first apply must exit 0, second must no-op, and the acceptance queries must hold before the same invocation touches the real database. The state-dedupe ceremony (2026-08-09, receipts in the Change folder) ran exactly as rehearsed, including catching a generated-flags bug in preview that never reached apply.

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

`agent.end` is turn-end, not session-end, so Loaf does not map session lifecycle behavior onto it.

### Prompt Overlay Consolidation (ADR-020, superseding ADR-010)

The managed fenced section is written once to the standards-native root `AGENTS.md`. `.agents/` remains Loaf's project state and configuration directory; Claude Code retains its native compatibility path as a symlink to the root file.

```
./AGENTS.md                              # Canonical real file (source of truth, committed)
.claude/CLAUDE.md        → symlink →      ../AGENTS.md
.agents/                                  # Loaf state and configuration; no AGENTS.md
```

**Write path (`loaf install`):** the native Go installer maps AGENTS.md-native targets directly to root `AGENTS.md`, resolves destinations via `realpath`, and groups writes by canonical path. Claude Code writes through `.claude/CLAUDE.md`, which resolves to the same root file. Before fenced-section writes, install creates or preserves the root real file, migrates the retired `.agents/AGENTS.md` layout, and enforces the Claude compatibility symlink.

| State | Action |
|-------|--------|
| root file absent | Create it, or move legacy `.agents/AGENTS.md` into place |
| root file is the old symlink to `.agents/AGENTS.md` | Replace it with the legacy file as a real root canonical |
| both root and legacy real files exist | Ask before preserving root as canonical, merging legacy user content, and retiring the legacy file to a collision-safe `.bak` path; `--yes` approves and noninteractive mode skips |
| Claude path missing/correct/wrong/real | Create/no-op/relink, or merge and back up before replacing with `../AGENTS.md` |

Fresh installs pre-create an empty root canonical so the Claude symlink is never dangling. `--yes` and non-TTY detection retain the existing consent behavior for replacing user-defined noncanonical symlinks or real compatibility files and for reconciling conflicting real root and legacy instruction files.

**Config health (`loaf config check`):** the native CLI validates `.agents/loaf.json` and installed Loaf-managed hook config separately. `--fix` creates missing safe project-config defaults and refreshes stale installed target artifacts through the same target installers as `loaf install`, so new hooks such as `github-account` can be propagated without hand-editing target config files.

**Drift detection (`loaf doctor`):** Checks cover root canonical presence and file type, retirement of legacy `.agents/AGENTS.md`, the Claude symlink target, stale `.cursor/rules/loaf.mdc`, fenced-section version match, and duplicate fenced sections. Plain diagnosis is read-only. `loaf doctor --fix` offers each logical repair once behind a default-no y/N prompt, preserves legacy content and backups for accepted repairs, and rechecks each repair before tallying the final result; checks that converge through the same filesystem action share one repair identity so a decline cannot be bypassed later in the run. Declined and non-interactively skipped repairs remain failures; only a real terminal is interactive, and `loaf doctor --fix --force` explicitly accepts every offered repair without prompting.

This extends the "CLI is the correct protocol layer" principle to filesystem convention enforcement: the CLI owns the on-disk overlay state, not the skills or the user. ADR-010 records the consolidation from per-harness writes to one canonical file.

### Work Records and Optional Linear Coordination

New bounded work is a Loaf issue, with authored shaping and implementation artifacts kept beside the code and operational issue state stored in project-scoped SQLite. Existing specs and task records remain supported compatibility surfaces until their named removal boundaries.

`issue.authority` in `.agents/loaf.json` elects the issue identity contract. `integrations.linear.enabled` records integration availability but does not select authority. Under local authority, Loaf mints the identifier. Under Linear authority, Linear mints the identifier and owns tracker title, workflow state, and assignment, while Loaf owns the shaping body, definition of done, claims, started worktree, and local event history.

The current CLI adapter owns issue creation, adoption, pull, push, and reconciliation. Provider mappings, conflict resolution, and status translation are deterministic CLI/state responsibilities; revision/content hashes, outbox handling, and retries remain planned CLI/state concerns in the active coordination packet. A working MCP connection does not replace that adapter. A dedicated `linear` provider skill may select an already-configured Linear MCP for user-scoped reads, comments, assignments, and other collaboration that does not compete with Loaf-owned fields. It does not configure or authenticate the MCP, and general Loaf workflow skills route such provider work through it instead of duplicating Linear behavior.

### Dependency and Completion Gates

`loaf issue frontier` derives the unblocked, unclaimed pickup set from Loaf's issue relationships and local status events. Implement refuses a blocked successor and claims an issue through `loaf issue start`; parent/child structure does not imply completion or sequencing without an explicit relationship.

Completion remains explicit: ship records `done` through `loaf issue status` after the work lands, then `loaf issue stop` removes the claim. Neither a Linear workflow state nor the completion of the last child silently closes a Loaf parent. Tracker state moves through `loaf issue push` or an explicit `loaf issue reconcile` resolution, not a generic MCP mutation.

### Agent Model: Functional Profiles

Loaf uses **functional profiles** defined by tool access boundaries, not role-based agents defined by domain identity. Skills provide all domain knowledge; profiles provide the tool sandbox.

**The Orchestrator:**

The main conversation is the **orchestrator** — the coordinator that plans and delegates but does not directly implement, review, research, or curate durable artifacts.

**Functional Profiles:**

| Profile | Tool Access | Purpose |
|---------|-------------|---------|
| implementer | Full write | Writes code, tests, config, docs. Speciality via skills at spawn time. |
| reviewer | Read-only | Audits and verifies. Cannot modify what it reviews — independence is structural. |
| researcher | Read + Web | Investigates options, compares approaches, returns structured reports. No write or execute. |
| librarian | Read + Edit (.agents/) | Tends the project journal and durable `.agents/` artifacts, including wrap checkpoints. Does not implement or research. |

Each profile is defined in `content/agents/{implementer,reviewer,researcher,librarian}.md` — a minimal behavioral contract and tool boundary, not domain knowledge. A spawned implementer becomes a backend engineer, DBA, or devops engineer depending entirely on the skills loaded at spawn time.

**System Agent:**

| Agent | Purpose |
|-------|---------|
| background-runner | Async non-blocking tasks (haiku model) |

**Council Composition:**

Councils convene implementers and researchers for deliberation; reviewers join only after, to verify the outcome. The orchestrator runs the council but never votes — the team decides, the orchestrator integrates.

**Skills as Universal Knowledge Layer:**

Skills are the only knowledge mechanism that works across all targets (Claude Code, Cursor, Codex, Amp). Profiles are Claude Code infrastructure — other targets activate knowledge through skills alone. This makes skills the primary investment surface: better skill descriptions and organization improve all targets simultaneously.

## Operating Principles

Principles that shape how Loaf is designed and operated. Unlike ADRs, these are mutable and evolve via `/reflect` as the project learns. They sit above implementation choices but below VISION (which captures product intent and direction).

### State Authority — Git Authors, SQLite Operates

Authored durable artifacts — Changes, plans, research, specs, ADRs, knowledge, reports, code, and generated deliverables — live in Git and are edited in place. Operational, queryable state — the journal, Intent, Exploration, checkpoints, conversation provenance, relationships, deferrals, and derived indexes — lives in project-scoped SQLite. Neither store mirrors the other: SQLite never becomes a hidden Markdown repository, and Git never holds per-conversation operational facts. An elected tracker may own explicitly bounded issue identity and workflow fields, but it never owns either durable store; publication and reconciliation remain explicit.

The intent-exploration-foundation Change proved the operational side as append-only facts rather than mutable lifecycle state. An Intent's disposition (`tracked`, `deferred`, `resolved`) and an Exploration's latest portable checkpoint are derived from transactionally sequenced immutable records — the row with the greatest committed per-aggregate sequence wins, never a timestamp and never a status column. Compound writes are retry-safe through one canonical per-project operation-key mapping (`intent_operations`), which the transitional `journal defer` adapter and legacy conversion share with the native commands, so no entry point can mint a parallel canonical record. Machine-local conversation handles and log locators are optional provenance with observed availability; portable context is exclusively the checkpoint's four required fields, and their presence is reported honestly (`portable_context_present`) rather than inferred from handles.

The judgment boundary follows from the storage boundary: humans and Skills interpret, classify, and choose operations; the CLI validates, proves, and performs Loaf state transitions deterministically. General Loaf workflow skills do not branch on provider configuration, call providers directly, or maintain lifecycle flags. Dedicated provider skills may select an already-configured provider MCP for user-scoped external collaboration, but they neither configure nor authenticate it and never take ownership of Loaf issue identity/state, provider mappings, retries, conflict resolution, or reconciliation. The CLI never decides whether input is a Spark, Idea, Intent, Exploration, or Change.

### Authorship Model — Agents Create, Humans Curate

Agents are the primary authors of Changes, knowledge files, ADRs, and compatible task records. Humans review, approve, and curate — they are not the writing surface. The CLI follows from this: it is for deterministic operations and health checks, while skills guide authorship and judgment.

The principle inverts the traditional "humans write docs, agents consume them" model. Agents are already doing the work and are closest to what's being learned; pulling knowledge creation into the work itself ("maintenance as side effect of work") is cheaper than treating documentation as a separate sprint. Humans are better at judgment — *is this worth recording?* — than at the writing.

The growth loop is concrete: an agent discovers an insight during exploration, implementation, or debugging, proposes the appropriate durable record, and a human accepts, edits, or rejects it. Hooks prompt agents when insights are fresh so useful learning is not deferred until context is gone.

This principle shapes skill design and CLI surface; it is mutable and evolves via `/reflect`.

### Adversarial Review for Substantive Guidance Changes

Substantive changes to skills, guidance docs, or operating principles warrant review beyond the implementer's own check. The Loaf baseline is `loaf:reviewer` (internal-consistency auditor). When available, an adversarial design stress-tester (`codex:rescue` or equivalent) is highly recommended — the two readers catch different defect classes:

- **Internal-consistency review** (`loaf:reviewer`) surfaces stale references, anchor breaks, prose drift, and contradictions between sections.
- **Adversarial design review** (`codex:rescue`, optional) stress-tests the design itself for false positives, false negatives, and self-contradictions; constructs decision examples the rules don't handle cleanly.

Codex is plugin-dependent — it may not be available in all environments. `loaf:reviewer` is the floor; the adversarial pass is recommended when the change is substantive enough that a design defect would compound across many future invocations (skill rewrites, lifecycle codifications, hook-policy changes).

This principle shapes how Loaf evolves substantive guidance. Evidence: the architecture-skill tightening + ADR deprecations (PR #46) shipped through three review-driven refinement rounds, with each reviewer catching defect classes the other missed. PR #122 extended the evidence beyond guidance: after two independent Claude reviews of the intent-exploration foundation, a Codex adversarial pass over the same diff found a state-integrity defect (legacy operation-key capture), a schema constraint gap (cross-intent deferral references), and eleven further findings. Treat the adversarial pass as recommended for foundational state-model and persistence changes as well, not only guidance.

### Recategorization as a General Lifecycle Pattern

Loaf artifacts evolve in two distinct ways:

- **Supersession** — the underlying answer changed; a new artifact replaces the old. The old is preserved as historical record (`status: Superseded`, `superseded_by:` linkage). Used for ADRs whose decisions changed.
- **Recategorization** — the underlying rule still holds, but the artifact's classification was wrong from the outset. The artifact is deprecated in place (`status: Deprecated`, `migrated_to:` reference in the body), and the rule's active source moves to its appropriate home.

Recategorization emerged from PR #46: three ADRs whose conventions/principles still held had been classified as architectural decisions when they were actually a naming convention, an operating principle, and skill-specific workflow lore. Supersession (write a new ADR replacing each) was the wrong tool — there was nothing to replace, only to relabel. Recategorization preserves the historical record without overstating its current authority.

This pattern generalizes beyond ADRs. When any Loaf artifact is later judged to have been classified wrong but its content is still valid, recategorize: deprecate the original, point to the new canonical home, leave the body intact for archeology.

## Change-First Execution Model

New bounded work uses a Change as its primary contract. The Change folder splits role-named narrative (settles at shaping) from task-file state (mutates during execution), so execution evidence is machine-derivable from committed content — checkbox-flip history where the merge strategy preserves it, receipt-vouched content where it does not (ADR-022, ADR-023, ADR-027; operating view in [knowledge/work-model.md](knowledge/work-model.md)). The project journal remains the execution trace and resumption protocol.

```
capture → /shape → Change → /implement (task commits) → review → /reflect → /ship
                        ↓
                 project journal
```

### Work Records

```
docs/changes/YYYYMMDD-slug/           # Change: the bounded-work unit (ADR-022)
├── change.json                       #   identity + optional target_release
├── shape.md                          #   the contract; executable criteria declare Command/Expect
├── brief.md, plan.md, design.md      #   optional roles: pre-shaping ask, technical route
├── tasks/TASK-NNN-slug.md            #   delegation packets; checkboxes flip in delivering commits
├── research/ and reports/            #   shaping inputs; authored snapshot outputs (closed kind registry)
└── receipts/verify.json              #   cohort members: committed cache of loaf change verify
.agents/specs/SPEC-XXX.md             # Existing compatible bounded-work records
SQLite journal_entries                # Project-scoped event record across conversations
```

**Changes** define the problem, scope, decisions, verification contract, and definition of done. `loaf change check` validates both layouts and derives the display ladder (captured → shaped → executable → executing → complete, plus verified for cohort members) — no status fields exist anywhere; every state is computed.

**Releases read cohorts.** A change declaring `target_release` opts into the strong gate: cutting that version stable requires the whole cohort executed and receipt-verified, with all criteria passing. Execution grades as a disjunct — a true `- [ ]`→`- [x]` flip transition in ancestry (outside fences, same hunk and label), **or** a fresh verify receipt vouching for a folder whose every committed box is checked — so the grade holds under every merge strategy, squash included; a receipt cannot exist without the implementation in the tree, which is what keeps the shaping-only attack blocked (ADR-023, ADR-027; PR #154). Release commits may be changelog-only when version files already carry the candidate, the self-carrying shape guardrail 4 proves before guardrail 5 reads the diff (PR #155). In a multi-Change cohort, later members' content stales earlier members' receipts: all cohort receipts re-verify at the final pre-merge tree, terminating because receipt commits are content-free and digest-excluded. The gate is a pure reader of committed evidence — `loaf change verify` is the only surface that runs criteria; stale or failing receipts block with the mechanical remedy named. Prereleases always flow; retargets are reviewable diffs, surfaced and never blocked (ADR-023).

**Releases are arc-boundary events.** Pre-1.0, X bumps when a release ships a completed arc — the cohort *is* the arc (ADR-022), and a standalone executed Change with no pin is an arc of one; every other cut bumps Y. Merging and releasing are separate acts: mid-arc Changes land on main when ready, ride Y cuts unannounced (the changelog is the announcement carrier), and the arc-completing cut is the X release. `target_release` is pinned late — at shaping, never at capture — and retargeted as routine when X advances past it (ADR-026).

**Releases gate on capability evidence.** `loaf release` validates the capability-evidence registry in-process after the artifact rebuild on every mutating path — a post-rebuild refusal in the shared apply executor and a ninth post-merge guardrail. Resume after a refusal is verify-then-restore with no persisted state; post-merge recovery is a single receipt-only repair commit classified against the parent commit's registry; every registry and candidate-artifact read is symlink-hostile through a shared component-wise regular-file walk (PR #147; change record `docs/changes/20260730-release-evidence-gate/`).

**Tasks and specs** remain supported compatibility records. New-work decomposition lives in the change's own `tasks/` packets; SQLite tasks and `.agents/specs/` describe existing work until deliberately converted.

**The journal** captures *what happened* — `journal_entries` rows are project-scoped events (`project_id NOT NULL`), each tagged with an opaque `harness_session_id` that correlates one conversation's entries. Decisions, discoveries, commits, and progress land as structured entries; `loaf journal recent`/`show`/`search` and the `loaf journal context` digest provide handoff-ready context for compaction recovery and cross-conversation resumption. There is no session entity — see [Session Model: Journal-First](#session-model-journal-first).

`.agents/tasks/`, `.agents/ideas/`, `.agents/sparks/`, `.agents/brainstorms/`, `.agents/drafts/`, and `.agents/TASKS.json` are rollback material after the SQLite cutover recorded by `SPEC-045`, not compatibility mirrors. A stale branch that reintroduces them should keep the deletion side and rerun `loaf check --hook ephemeral-provenance`. Legacy `.agents/sessions/` Markdown is also gone: the journal is SQLite-native and never rendered to a hand-authored source file.

### Session Model: Journal-First

The project journal is the **only** session-related structure. There is no session entity — no `sessions` table, no statuses, no lifecycle, no rotation. `journal_entries` are project-scoped events (`project_id NOT NULL`) in the global SQLite database, each carrying an opaque `harness_session_id` column that correlates the entries written by one conversation. Nobody opens, closes, or transitions a session; nothing is ever "unwrapped."

The journal model supersedes the former mutable session lifecycle. **Concurrent conversations on the same project — across branches, worktrees, even harnesses — are safe by construction:** simultaneous writers interleave rows with different `harness_session_id` tags instead of rotating or reconciling shared session state.

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

When review reveals that code on an open branch should not ship as designed, the project favors **forward removal commits over history rewriting**. The final squash preserves a clean mainline diff, while the pull request retains review context and an honest record of the pivot. Avoid force-pushing away citations or shared review history.

## Hook Architecture

Hooks are defined in `config/hooks.yaml` and distributed to target-specific formats at build time. For Claude Code, the canonical hook registration file is `hooks/hooks.json` (inside the plugin output directory). `plugin.json` silently drops non-matcher session events (SessionStart, PreCompact, PostCompact, TaskCompleted) — all hooks should be registered in `hooks.json`.

### Dispatch Types

| Type | Field | Behavior |
|------|-------|----------|
| script | `script:` | Runs a shell script |
| command | `command:` | Runs a CLI command (e.g., `loaf check --hook <id>`) |
| prompt | `prompt:` | Injects text directly to the AI model |

### Hook Type Behavioral Constraints

The target hook APIs impose these behavioral constraints:

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

CWD-relative fixtures are forbidden for subprocess tests because workers may share filesystem state and cwd. A fixture such as `join(process.cwd(), ".test-...")` can race with another subprocess test even when each file passes independently.

`realpath` is required on macOS because the system tmpdir (`/var/folders/...`) is reached through a `/private/var/folders/...` symlink; without realpath, subprocess cwd comparisons can fail.

The active test harness is now Go. `npm test` delegates to `go test ./...`, and `npm run typecheck` compiles all Go packages with `go test ./... -run=^$`.

Migration and state-classification code gets one explicit fixture per supported starting state — every schema version a classifier branches on — never transitive coverage through neighboring versions. The v0.2.10 regression shipped exactly through an accepted "covered transitively" gap: schema-11 databases were unclassifiable and unupgradeable until the same-day 0.2.11 hotfix (#124). A review note of "unproven but transitively covered" on classification code is blocking, not advisory.

## Cross-Cutting Patterns

Patterns that apply across multiple subsystems and emerged from specific post-release followups. Captured here so they inform future work rather than being re-discovered.

### Single-Source Runtime Versioning

The native CLI version must report the package version consistently through the launcher, native runtime, generated targets, and install markers. Go runtime paths read package metadata directly; the obsolete TypeScript version helper was removed after the install and version surfaces moved to native Go.

Any value that must be identical across runtime modes should be injected at build time, not independently resolved by multiple runtime paths. Divergent version discovery creates false positives in every downstream comparison.

One deliberate exception: a dev build's commit identity (`<package-version>+g<short-sha>`, ADR-026) is *not* injected into the native binary, because build-varying bytes would break the reproducibility that `verify:go-artifacts` asserts. `build-go.mjs` compiles requested targets into `bin/native/.staging/` and writes `bin/.loaf-dev-commit` only after every requested target compiles, so a partial rebuild cannot leave a new binary reporting a previous commit. A binary running from a source checkout reads that ignored provenance file at startup, while shipped distributions omit it and continue reporting their release version. After a successful non-release build with Git provenance, the same script retargets `$XDG_DATA_HOME/loaf/current-dev-launcher` and creates `~/.local/bin/loaf` only when that name is absent, as a symlink to the pointer; `LOAF_DEV_LINK=0` disables this behavior, an existing real file, directory, or any other symlink is never overwritten, and activation failure never fails the native build.

### Generated Runtime Plugin Artifacts Parsed From Emitted Output

Files the build emits for downstream runtimes to execute — OpenCode `hooks.ts`, Amp `loaf.js`, and any future per-target runtime plugin — must have tests against the **actual emitted file**, not just the generator's input string.

Source-template assertions cannot prove that escaping remains valid after generation. Native build tests in `internal/cli/build_test.go` therefore read the emitted OpenCode and Amp plugin files and assert the runtime hook bodies and command payloads that downstream runtimes load.

### Visible-Degraded Fallback with Stderr WARN

When strict invariant enforcement would break existing callers but silent fallback risks incorrect behavior, emit a stderr warning that names the missing signal and any silencing flag. The action may proceed for compatibility, but the degraded path must remain visible and regression-testable. The journal no longer uses a branch-fallback session router; entries attach project, branch, and harness context without resolving a mutable session entity.
