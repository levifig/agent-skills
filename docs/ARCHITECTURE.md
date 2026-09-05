# Loaf Architecture

_Last updated: 2026-08-31_

## vNext Kernel (Pre-Cutover)

vNext is an unshipped, isolated Go production boundary. It shares the repository and Go module so it can be tested during construction, but it does not share runtime packages, command dispatch, state access, or generated artifacts with the shipped line.

```
vnext/
├── cmd/loaf/                 # Unshipped entry point; not the canonical installed binary
├── continuity/               # Closed continuity domain and isolated SQLite adapter
├── content/
│   ├── flow-contract.json    # Machine-checked authority and portable Flow inventory
│   ├── skills/               # Common Flow, provider contract, and Linear mapping
│   ├── templates/            # Problem narrative, work contract, tracker update
│   └── agents/               # Optional least-authority project manager profile
└── internal/
    ├── command/              # Bootstrap command dispatch
    └── kernel/               # Schema identity and ownership contract
```

The schema identity starts on a new line at `vnext/1`. ADR-030 chooses SQLite-backed typed facts for private continuity; that is not a claim the kernel already opens a database. The kernel does not inspect legacy tables or translate schema 25. Continuity persistence arrives in its own implementation phase, and one-time archive migration remains a later slice. Continuity lives at `vnext/continuity` so an external oracle can import it; it is not placed under `vnext/internal/`.

### Ownership Matrix

Every responsibility has one canonical authority:

| Authority | Canonical responsibilities |
|-----------|----------------------------|
| Loaf | Flow ceremonies, skills, templates, profiles, project identity, private continuity, derived context, and private sync |
| Tracker | Work identity and definition, definition of done, workflow state, hierarchy, assignment, and collaboration |
| Git | Code and deliberately promoted artifacts |
| Harness | Execution, model selection, tool boundaries, service connections, and service credentials |

Loaf may give an agent deterministic instructions and templates for tracker work, but the agent uses its harness-native connection. vNext does not store a shadow issue, hold provider credentials, proxy tracker calls, or reconcile a local work record with the tracker.

Scratchpad is deferred from the current vNext boundary. A later conversation-oriented surface may revisit ephemeral agent collaboration, but current Flow, continuity, migration, and sync do not depend on or advertise it.

### Private Continuity Persistence

vNext continuity is a personal, operator-owned store (ADR-030), distinct from the legacy Go/SQLite runtime (ADR-014) and the shipped fact-envelope sync contract (ADR-029). Typed append-only facts are canonical. Projections, including derived conversation context, are pure read-time folds in this slice; they are not stored tables that can drift from facts. Total order is hybrid-logical-clock wall and logical counters, then environment ID, then fact ID. The domain exposes a closed typed API: named kinds and semantics, not a runtime registry, generic metadata map, or tracker/provider/credential surface.

SQLite is the persistence engine now. A schema-and-API placeholder with no concrete adapter loses because it cannot prove durability, concurrent writes, or deterministic snapshot reads and would defer the central storage choice into sync and migration. A stdlib append-only file store loses because it cannot offer indexed project-scoped retrieval, transactional append, or crash-safe readers across environments without reinventing SQLite. Scratchpad is absent from the current catalog and API. Exact pre-cutover rows remain physically intact but are ignored by snapshots and sync export.

### Portable Tracker-Native Flow

The vNext content layer carries one authored set of Agent Skills bytes for pitch, triage, shape, implement, ship, release, and orchestration. `flow-contract.json` binds each ceremony to its input, output, shared template, and closed tracker-operation set; an external canonical table prevents that manifest from expanding its own authority. Three marker-bearing templates define the portable problem narrative, ephemeral definition packet, and evidence update. The definition packet routes problem, definition of done, out-of-scope, verification, and risk semantics once to native fields. The title is a native work field, and hierarchy and dependencies remain native relationship fields; none is copied into a locally stored monolithic body. The shared target builder now promotes these checked vNext skills into every supported harness from one common intermediate, without per-harness prose rewriting.

`project-management/v1` is the common semantic boundary. It discovers only connections already exposed by the harness, scopes one exact destination, reads before writing, and verifies mutations through authoritative native readback. The closed operation vocabulary keeps work, definition, hierarchy, dependency, status, and comment semantics distinct. Results always carry the operation, destination, native reference, observed state, verification evidence, one truthful outcome (`confirmed`, `unchanged`, `partial`, `failed`, or `indeterminate`), and one fidelity (`exact`, `advisory`, `manual`, or `unsupported`). Ambiguous creates and comment appends are never retried without first reading native state.

Linear is the first provider module, not a provider baked into core Flow. Each provider lives at `skills/<provider>/` with a harness-neutral `SKILL.md` and a strict `loaf-provider-capabilities/v1` manifest bound by directory slug to `project-management/v1`. The build and validator discover these modules dynamically, so GitHub, GitLab, Gitea, or a community provider can be added without changing the Flow manifest, ceremony skills, or build code. A module declares the maximum fidelity and phased `before`, `execute`, and `after` requirements for every common operation; unsupported operations remain explicit rather than being emulated through comments or local state. Linear's mapping remains externally pinned exactly. Availability is always runtime-discovered from the selected harness connection, so a module cannot claim that every harness exposes every operation or that semantic capability identifiers are universal tool names. `flow-contract.json` makes main-agent execution through the selected provider skill the only current route. A dedicated provider profile is deferred until target packaging and connector-only enforcement are proven across supported harnesses.

The root-module `internal/vnextflowcontract` package is test-only and outside `vnext/`. Its standard-library validator accepts an `fs.FS`, rejects duplicate JSON object keys recursively before strict typed decoding, and compares core content plus strictly shaped provider directories to an external recursive allowlist while rejecting symlinks. It checks exact ceremonies, generic provider-module invariants, the exact Linear mapping, frontmatter, one-level linked references, template markers, authority, and profile derivation through deterministic negative fixtures. A synthetic additional provider proves both validation and target packaging without edits to core Flow. A bounded phrase corpus guards known local-authority and provider-transport regressions in allowed Markdown and JSON, but it is defense in depth rather than a claim of arbitrary prose understanding. `go test ./internal/vnextflowcontract ./vnext/... -count=1 -run 'Flow|Template|TrackerSkill|ProviderModule'` is the canonical Flow gate.

### Bootstrap Command Ceremonies

| Command | Purpose |
|---------|---------|
| `loaf version` | Report the `loaf`, `vnext`, and `vnext/1` kernel identity without consulting legacy state. |
| `loaf ownership` | Print the ownership matrix in stable authority order. |

No state, tracker, sync, migration, build, or install commands exist in the vNext boundary yet. A command family is introduced only by the slice that owns its behavior and proof.

### Hard Legacy Dependency Boundary

The bootstrap source closure may import only the Go standard library and packages below the actual vNext import root. The root-module `internal/vnextboundary` package owns the gate outside the tree it checks; packages below `vnext/` neither contain nor import the checker. The gate derives the module root and module path from the repository's regular, non-symlinked root `go.mod`, rejects nested `go.mod` files and every symlink entry below `vnext/`, and parses every `.go` file present there — tests and files excluded by current build tags, GOOS, or GOARCH included. Module-local imports outside the derived vNext root and all third-party imports fail, with one finite reviewed exception for continuity persistence (ADR-030): `database/sql` is admitted only in the exact package `vnext/continuity/sqlite`, and a blank import of `github.com/ncruces/go-sqlite3/driver` is admitted only in the exact file `vnext/continuity/sqlite/driver.go`. Path or prefix spoofing, non-blank driver imports, and every other ncruces or third-party import remain forbidden. The kernel and remaining bootstrap tree have no third-party allowlist.

The same source policy rejects direct `database/sql` outside that package, plus `os/exec`, `syscall`, `plugin`, and cgo `C` imports, compiler linkname directives, dot imports of `os`, and references to `os.StartProcess`. It also rejects the Go toolchain's recognized native and object build inputs: `.c`, `.cc`, `.cpp`, `.cxx`, `.m`, `.h`, `.hh`, `.hpp`, `.hxx`, `.f`, `.F`, `.for`, `.f90`, `.s`, `.S`, `.sx`, `.swig`, `.swigcxx`, and `.syso`. Ordinary non-build documents remain permitted. This proves that the checked tree contains no legacy-module import, does not admit third-party code outside the exact SQLite driver file, does not directly name those escape capabilities, and cannot add code through those recognized native inputs. It does not claim semantic proof against every possible dynamic mechanism or ordinary standard-library I/O; those remain visible code subject to review.

`go test ./internal/vnextboundary ./vnext/... -count=1 -run 'Kernel|DependencyBoundary'` is the canonical gate. Network-free fixtures cover inactive source, alternate module identities, root-anchor symlinks, nested-module self-disable, native inputs, third-party imports, and forbidden capabilities. The current implementation remains useful as behavioral specification, failure corpus, and source for a versioned one-time export, but no production code is copied across the boundary.

Everything below this section documents the current shipped legacy line. It remains operational until the reviewed cutover, but its ownership model, package graph, schema, and command breadth are not vNext precedent.

## Legacy Runtime Architecture (Current Shipped v2.0 Line)

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

> **Revision 2026-08-26 (LOAF-90):** The XDG SQLite file is a local replica of the personal substrate, not the complete destination store. See Personal Memory Substrate below.

Loaf stores operational state in one global SQLite database at `$XDG_DATA_HOME/loaf/loaf.sqlite`, partitioned by project ID. New project IDs are generated and stored in SQLite; they are not derived from checkout path or friendly name. The `projects` row carries the friendly display name and current path, while `project_paths` records path mappings so a checkout can move without changing identity. Legacy path-hash IDs remain only as an adoption key for migrated pre-stable-identity data.

Entity identity follows the same discipline one level down (ADR-028). Derived entity IDs are mint-once opaque keys: computed at first creation, never recomputed for resolution. The aliases table — with the schema's only content-meaningful unique constraint, `UNIQUE (project_id, namespace, alias)` — is the identity registry, and the markdown importer resolves through it before deriving an ID for anything. Unaliased kinds resolve by natural key: journal entries by (entry type, scope, message) for markdown-origin rows, sources by project and path. Sparks whose message normalizes to an empty slug receive a deterministic content-hash alias so no row is born unreachable.

The standing invariant is alias parity: for every project and every aliased entity table, raw row counts equal alias-reachable counts, with zero dead aliases. `loaf state doctor` checks it on demand (read-only, error severity without invalidating the database, naming `loaf state migrate alias-orphans` as the repair). The June-24 identity fork — a project rekey silently invalidating every derived ID, repaired by the state-dedupe Change — is the incident this invariant exists to catch on day one instead of week six.

### Personal Memory Substrate

> **Revision 2026-08-26 (LOAF-90):** New section. Supersedes: implicit single-machine SQLite as sufficient for Loaf-flow work.

One operator's substrate is **identical across every environment** they operate. Local SQLite on a trusted machine is a full replica (local-first). A self-hostable sync relay (`loaf serve`, LOAF-75) converges facts between environments; the server holds opaque ciphertext blobs and auth tokens only — never keys, never plaintext semantics. The client sync engine (LOAF-76) queues, pulls by cursor, and detects env-seq gaps with loud warnings. Environments that cannot attach hard-refuse substrate-touching commands (LOAF-67); there is no detached override in v1. Never-attached local use remains valid local-first; fail-loud attach applies when joining the sync fleet.

**Synced minimal core:** project identity and attachment evidence, ref mappings and work contracts, ledger facts (journal, sparks, wraps, handoffs, decisions), verification-run records, releases. **Never synced:** FTS indexes, hook trust, conversation handles, vocabulary fossils (local archive).

The fleet agrees on the **fact envelope** (versioned cleartext contract + E2E payload), not the SQLite schema. Schema evolves per machine; facts replay into projections via latest-event-wins folds ordered by `(hlc, env_id, id)` (LOAF-71, ADR-029).

**Credentials (two-mode):** admin credentials (master key + account access key/secret) live on trusted machines only; client credentials bundle endpoint + project-scoped token + derived project key in per-project harness secrets. Repo `.agents/loaf.conf` carries project ID and tracker binding — never the sync endpoint.

**Permanence classes:** ledger (permanent), notebook (durable retrieval), scratchpad (effort-scoped ephemeral). Promoted exports (PR bodies, rendered artifacts, tracker rows) are never re-imported as authority.

**Ship status:** LOAF-63–67, 71–72, 75–76, 87–89 shipped on main. Writers append through the LOAF-71 chokepoint; the `events` table is local archive (migration 0025). LOAF-62 parent closeout is independent review and ship.


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

### Dead State Retirement and Document Demotion

> **Revision 2026-08-26 (LOAF-90):** Supersedes: reports/councils/shaping_drafts as SQLite authority; findings/verdicts/runs as live schema.

LOAF-79 (migration 0018) deletes zero-row findings/verdicts/runs schema. LOAF-80 (migration 0021) demotes document-layer rows to files; CLI serves file-backed reports/councils/drafts. LOAF-81 classifies every project-scoped table (sync, local-archive, machine-local, gone) and prunes fossil relationship edges. Handoffs remain in the synced core as ledger facts.

LOAF-72 (migration 0025) writes sparks, ideas, handoffs, refs, worktree bindings, verification receipts, and releases as grow-only event facts through the LOAF-71 append chokepoint. Projections fold latest-event-wins; existing rows migrate by verified replay (event facts, then birth-fact on fold mismatch). The `events` table stays as local archive.

### Legacy Scratchpad Compatibility

> **Revision 2026-08-31:** Scratchpad is outside the current vNext product boundary. The following shipped-line history is retained only as migration evidence.

The shipped runtime introduced `loaf scratchpad append|read|list|claim|release`, server fanout, logical close, and relay prune. vNext does not carry those capabilities forward. Its internal decoder recognizes exact pre-cutover rows and tombstones only to preserve data and fail safely; it creates, projects, exports, synchronizes, and prunes none of them.


### Targets

| Target | Output | Agents | Skills | Hooks | Runtime Plugin |
|--------|--------|:------:|:------:|:-----:|:--------------:|
| claude-code | plugins/loaf/ | Yes | Yes | Yes | plugin.json |
| cursor | dist/cursor/ | Yes | Yes | Yes | hooks.json |
| opencode | dist/opencode/ | Yes | Yes | Yes | hooks.ts |
| codex | dist/codex/ | No | Yes | Yes | hooks.json |
| amp | dist/amp/ | Modes | Yes | No | .amp/plugins/loaf.ts, .amp/plugins/loaf-modes.ts |

### Amp Plugin API Constraints

Amp's plugin API is intentionally minimal. Plugin handlers are dispatched via `handleRequest()` for exactly four event names:

- `tool.call` — before a tool is invoked
- `tool.result` — after a tool returns
- `agent.start` — when an agent begins a turn
- `agent.end` — when an agent finishes a turn

There is no session-lifecycle dispatch. Amp's binary internally emits `emitEvent("session.start", ...)` for telemetry purposes, but this is not exposed to plugins. Features that require a true SessionStart hook (the journal continuity digest, SOUL.md self-healing) or PreCompact flushes are not viable on Amp without upstream support. Loaf does use `agent.start` for one bounded global action: reconcile Loaf-owned installed skills and the Amp adapter to the already-installed CLI distribution. The handler never downloads a CLI, installs a connector, touches project files, or blocks the turn on failure. If bytes change after Amp has loaded them, the durable receipt and diagnostic require a new session; Loaf does not claim hot reload. Agent/mode support is provided by a second managed plugin, `.amp/plugins/loaf-modes.ts`, which registers `Loaf Medium`, `Loaf Ultra`, and the pinned delegation tools. The orchestrator and oracle agents are `openai/gpt-6-astra` at medium/xhigh and high; review stays on `openai/gpt-5.6-luna` at max; the implementation agent is `xai/grok-4.6` with Fast and no explicit reasoning effort.

`agent.end` is turn-end, not session-end, so Loaf does not map session lifecycle behavior onto it.

### Managed Harness Content Adoption

`loaf harness reconcile --target <target>` names the initiating harness, not the atomic update scope. Under one shared managed-content lock, Loaf selects the same canonical skills source as install/upgrade and reconciles the complete installed shared-skills cohort whose verified distributions are available: shared skills once, then every stale target adapter, marker, and install record. It refuses missing ownership evidence, unreadable versions, downgrades, and distributions without a verified target manifest. On supported Unix release systems, lock ownership is kernel-managed: an old timestamp never authorizes pathname replacement, and a crashed process releases ownership automatically while leaving reusable diagnostic metadata. Windows uses a conservative exclusive-create fallback with no automatic stale reclamation; persistent crash residue requires explicit operator cleanup after confirming no Loaf process remains. Successful changes write `.loaf-reconcile-receipt.json` with cohort versions, checked surfaces, updated targets, and `restart_required`.

Amp is the only automatic startup path in this slice. Other harnesses can run the command explicitly or through a future proven native lifecycle hook; their managed instructions must not imply automatic adoption today. An Amp installation older than the `agent.start` hook cannot invoke behavior it does not contain, so the 0.5 cutover requires one explicit `loaf install --to amp` or `loaf upgrade --to amp`. Automatic adoption applies to subsequent managed-content versions. The mechanism never updates the Loaf CLI itself. It resolves the active install layout: global workstations update the user-owned harness cohort, while `LOAF_PROJECT_ENV=1` updates only Loaf-owned project-local harness artifacts and managed skills. It never changes project configuration, human-authored instructions, connectors, credentials, or tracker state.

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

### Tracker-Native Work Records

> **Revision 2026-08-26 (LOAF-90):** Contract machinery keys to provider-qualified refs (LOAF-82 shipped). Internal issue rows retire after render-out (LOAF-83 shipped); decision-kind issues re-home to ledger facts (LOAF-84 shipped); flow skills on refs (LOAF-86 shipped).
>
> **Revision 2026-08-26 (LOAF-62 inventory):** LOAF-84 and LOAF-86 shipped. Supersedes: "LOAF-84 shipped" and "LOAF-86 shipped".

New bounded work is a canonical record in the selected native tracker. The tracker mints identity and owns the problem definition, definition of done, workflow status, hierarchy, assignment, and collaboration. Loaf supplies the pitch → shape → implement → ship → release method, templates, private continuity, derived context, and deterministic local mechanics. The agent follows the selected provider module and uses only a connection already exposed and authenticated by the harness.

Project configuration may preserve provider-namespaced, non-secret routing hints under `integrations.<provider>`, including a generic `mcp_server_name` and provider-owned opaque vocabulary such as Linear workspace or team hints. Core config accepts extensible provider slugs and preserves provider-owned fields; the provider module interprets and validates them. No `issue.authority` or local issue prefix is required for tracker-native Flow, and `issue.authority: linear` would select the retired API-key adapter rather than the harness-native provider module.

The legacy `loaf issue` database and its Linear pull, push, and reconcile commands remain frozen compatibility surfaces for explicit migration work only. They are not installed/default Flow authority. Local-to-tracker synchronization does not exist and must not be reintroduced. A local project can be migrated once by an agent that reads the source, creates native records through the harness connection, verifies definitions and relationships, and records the cutover; Loaf does not retain an ongoing local-to-remote ID map afterward.

### Dependency and Completion Gates

Dependency readiness, hierarchy, assignment, and completion are read from their exact native tracker semantics through the provider module. Implement refuses work whose observed native dependencies remain blocked. Ship records its verdict and evidence in the canonical tracker and transitions status only after native readback confirms the requested state. Comments never substitute for definition, relationships, or workflow state.

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

### Architecture Serves the Next Rideable Increment

Architectural depth is enabling work, not product progress by itself. New storage, migration, security, crypto, protocol, and abstraction work must name the immediate [rideable increment](../content/skills/foundations/references/rideable-increments.md) that exercises it end to end. Foundation work may span atomic commits, but successive strategic milestones must integrate it into a real operator journey. Narrow slices retain the same integrity requirements as broad ones. This principle changes sequencing and review judgment; it creates no additional authority, tracker record, schema, status, or gate.

### State Authority — Tracker Shares, Git Authors, SQLite Remembers

Shared work contracts live in the selected native tracker. Deliberately authored durable project artifacts — plans, research, ADRs, knowledge, reports, code, and generated deliverables — live in Git and are edited in place. Private operational continuity — the journal, Intent, Exploration, checkpoints, conversation provenance, deferrals, and derived indexes — lives in project-scoped SQLite. None mirrors another: SQLite never becomes a hidden Markdown or tracker repository, Git never holds per-conversation operational facts, and tracker definitions are never synchronized into a local work record.

The intent-exploration-foundation Change proved the operational side as append-only facts rather than mutable lifecycle state. An Intent's disposition (`tracked`, `deferred`, `resolved`) and an Exploration's latest portable checkpoint are derived from transactionally sequenced immutable records — the row with the greatest committed per-aggregate sequence wins, never a timestamp and never a status column. **Revision 2026-08-26 (LOAF-90):** Synced-core projections fold latest-event-wins using HLC total order `(hlc, env_id, id)` (LOAF-71); legacy Intent/Exploration sequence semantics remain (those entities are outside the LOAF-72 mutable-core event-fact set). Compound writes are retry-safe through one canonical per-project operation-key mapping (`intent_operations`), which the transitional `journal defer` adapter and legacy conversion share with the native commands, so no entry point can mint a parallel canonical record. Machine-local conversation handles and log locators are optional provenance with observed availability; portable context is exclusively the checkpoint's four required fields, and their presence is reported honestly (`portable_context_present`) rather than inferred from handles.

The judgment boundary follows from the storage boundary: humans and skills interpret, classify, and choose operations; the CLI validates and performs Loaf-owned local state transitions deterministically. General Flow skills operate the provider-neutral contract. Dedicated provider modules select an already-configured harness connection, interpret provider-owned routing hints, and preserve exact native fidelity; they neither configure nor authenticate connections nor route through a Loaf client. The CLI never calls the tracker or decides whether input is a Spark, Idea, Intent, Exploration, or shared work item.

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

Integration toggles in `loaf.json` gate runtime features such as Linear magic-word detection without rebuilding. `integrations.linear.mcp_server_name` records the project-selected Linear MCP without making Loaf responsible for its connection or credentials.

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

A dev build's commit identity (`<package-version>+g<short-sha>`, ADR-026) follows the same rule. `build-go.mjs` compiles with `-buildvcs=true`, so the toolchain stamps `vcs.revision` and `vcs.modified` into the native binary and `cmd/loaf/main.go` reads them back through `runtime/debug.ReadBuildInfo`; a build from a tree with uncommitted changes reports `.dirty` after the commit. The pinned `go1.26.6` toolchain writes no stamp inside a linked worktree (`go1.27.1` does), so `build-go.mjs` also resolves the commit and `git status --porcelain` state itself and links them as `-X main.devCommit/devModified`; the runtime prefers the stamp and falls back to the linked values. Because the identity is part of the compiled bytes it cannot drift from them, and it also means two builds of identical source from different commits differ, which is why root `bin/` is no longer tracked: the launcher and native binaries are build outputs regenerated by `npm run build:go`, while `plugins/loaf/bin/` stays committed for the Claude Code marketplace until a shim replaces it. `build-go.mjs` still compiles requested targets into `bin/native/.staging/` and publishes them only after every requested target compiles, so a partial rebuild cannot leave a mixed set. After a successful non-release build, the same script retargets `$XDG_DATA_HOME/loaf/current-dev-launcher` and creates `~/.local/bin/loaf` only when that name is absent, as a symlink to the pointer; `LOAF_DEV_LINK=0` disables this behavior, an existing real file, directory, or any other symlink is never overwritten, and activation failure never fails the native build.

### Generated Runtime Plugin Artifacts Parsed From Emitted Output

Files the build emits for downstream runtimes to execute — OpenCode `hooks.ts`, Amp `loaf.js`, and any future per-target runtime plugin — must have tests against the **actual emitted file**, not just the generator's input string.

Source-template assertions cannot prove that escaping remains valid after generation. Native build tests in `internal/cli/build_test.go` therefore read the emitted OpenCode and Amp plugin files and assert the runtime hook bodies and command payloads that downstream runtimes load.

### Visible-Degraded Fallback with Stderr WARN

When strict invariant enforcement would break existing callers but silent fallback risks incorrect behavior, emit a stderr warning that names the missing signal and any silencing flag. The action may proceed for compatibility, but the degraded path must remain visible and regression-testable. The journal no longer uses a branch-fallback session router; entries attach project, branch, and harness context without resolving a mutable session entity.

---

## Changelog

- 2026-08-29 - Add the portable tracker-native vNext Flow contract, shared templates, Linear semantic mapping, optional least-authority project manager, and external validation gate.
- 2026-08-29 - Add the isolated vNext kernel, move its boundary enforcement outside vNext, and cover source imports, native inputs, direct capabilities, schema identity, and bootstrap commands.
