---
change: intent-exploration-foundation
created: 2026-07-17
branch: intent-exploration-foundation
lineage: change-model-hard-cut
predecessor: journal-reliability-foundation
---

# Intent and Exploration Foundation — Portable Context Without Session Lifecycle

## Lineage

- **Lineage:** `change-model-hard-cut`.
- **Position:** Immediate successor to `journal-reliability-foundation` and first materialized node of the 2026-07-17 workflow amendment.
- **Current accepted order:** `journal-reliability-foundation` → `intent-exploration-foundation` → `change-native-execution-migration` → `linear-native-coordination` → `spec-conversion-and-guidance-sweep`.
- **Activation evidence:** The predecessor is retained in current `main`; its journal, origin, lineage, and target-delivery foundations have landed; the workflow redesign was explored and approved before this Change was materialized.
- **Release gate:** The root lineage retains `release-after: spec-conversion-and-guidance-sweep`; this Change cannot enable a stable or normal-semver release before that terminal node lands.
- **Workspace rule:** This branch/worktree is the one active Change workspace for this lineage. Explorations and Intents remain project-scoped SQLite records and never allocate branches or worktrees themselves.

### Immediate successor packet — `change-native-execution-migration`

- **Purpose:** Promote a bounded Intent into a Git-canonical Change and mechanize the first durable write through one Change-dedicated branch/worktree, `change.md`, `plan.md`, an affected-artifact manifest, and an early draft PR.
- **Activation gate:** This Change is merged; additive migrations and the legacy-deferral bridge pass against current production-shaped fixtures; one Exploration is resumed through a second harness/context using only its portable checkpoint; one Intent is deferred and resumed without lifecycle drift; `/triage` and `/explore` dogfood produces no direct provider calls.
- **Inherited decisions:** `change.md` owns the Product Contract; `plan.md` owns named implementation units, dependencies, acceptance criteria, and verification; durable project artifacts remain Git-canonical; CLI primitives perform deterministic workspace and state operations while humans and Skills decide when to invoke them.
- **Required outputs:** Intent-to-Change promotion; branch/worktree allocation before the first durable write; Change-local materialization of relevant Exploration research/reports; draft-PR linkage; Change-native `/shape`, `/plan`, `/implement`, and `/ship`; path-based affected-artifact reconciliation; per-project migration of remaining legacy execution state.
- **No-gos:** No tracker synchronization, no direct MCP/provider logic in Skills, no numbered Task replacement, no shadow copies of canonical durable artifacts, no broad guidance hard cut, and no automatic semantic promotion of Intents.
- **Implementation evidence (2026-07-19):** The foundation shipped as schema 12 with composite same-project foreign keys, so cross-project references fail before commit. Shipped command surfaces for this packet to build on: `loaf intent create|defer|resume|resolve|show|list`, `loaf exploration create|checkpoint|list|context|conversation add`, `loaf conversation create|show|list|handle add|observe`, `loaf intake list`, `loaf state migrate deferrals --dry-run|--apply`, `loaf doctor --json`, and `loaf install --upgrade --dry-run --json`. Operation keys bind to exactly one aggregate — cross-binding retries are rejected explicitly rather than silently reusing another aggregate's first write — and the `journal defer` adapter materializes legacy projections from the stored canonical packet with an all-or-nothing version 0→1 advance. Journal continuity carries two layers this packet may consume directly: `deferred-intent` (canonical deferred Intents as active truth with legacy dedup through `intent_operations`) and `exploration-checkpoints` (latest portable checkpoint per Exploration with exact resume commands). Promotion should link the created Change to its Intent through the relationship registry's future `materialized-as` extension, which remains deliberately unregistered here.

### Following successor packet — `linear-native-coordination`

- **Purpose:** Add Linear as the native collaborative authority for published Intent, Change, and meaningful plan-unit workflow state while preserving Git as authority for durable artifacts and SQLite as the local operational/provenance layer.
- **Activation gate:** `change-native-execution-migration` is merged and its local-only workflow is dogfooded end to end; the provider-neutral Change and plan primitives are stable enough that the Linear adapter does not define their semantics.
- **Inherited decisions:** One primary issue per Change; meaningful named plan units become subissues; inbound issues may be adopted as Intent; tracker IDs are mappings, not intrinsic local identity; Skills request provider-neutral deterministic operations and never branch on `integrations.linear.enabled`.
- **Required outputs:** Publish/adopt/reconcile primitives, revision/content hashes, an outbox with idempotent retries, explicit conflict detection, issue/PR/Change links, subissue dependencies and acceptance checklists, and agent-from-Linear bootstrap into the Git-canonical Change.
- **No-gos:** No tracker-owned durable artifact bodies, no silent last-writer-wins conflict resolution, no provider payload archive, no Linear requirement for local-only projects, and no provider-specific workflow prose duplicated across Skills.

### Terminal successor packet — `spec-conversion-and-guidance-sweep`

- **Purpose:** Hard-remove public Spec/Task/render/breakdown authority, migrate Loaf's own remaining legacy records, supersede conflicting storage/workflow guidance, and converge live source, generated targets, installed surfaces, and release documentation around the completed workflow.
- **Activation gate:** Every preceding node is merged and freshly verified; the parked commit `59fbcdcf` is reconciled as historical working evidence rather than applied as current truth; the terminal Change materializes with `predecessor: linear-native-coordination`.
- **Inherited decisions:** Git owns Changes, plans, specs, ADRs, knowledge, reports, and code; SQLite owns operational state, journal, Intents, Explorations, checkpoints, provenance, relationships, handoffs/wraps, mappings, and derived indexes; Linear owns published collaborative work state when configured; `.agents/` remains tracked shared configuration plus explicitly disposable scratch, not a durable artifact home.
- **Required outputs:** Public command and Skill hard cut, deterministic migration/quarantine, ADR supersession where the reversal test warrants it, current-guidance grep gates, generated-target convergence, installed-surface smokes, and the stable-release gate.
- **No-gos:** No indefinite dual authority, no compatibility release that advertises retired commands, no rewriting historical Changes/ADRs to hide the old model, and no broad unrelated Skill-quality rewrite.

## Problem

Loaf can capture a Spark, retain an Idea, store a Brainstorm, append journal entries, and atomically preserve a deferred nugget as a journal-decision/Spark pair, but it cannot represent the body of inquiry that connects them. The workflow and artifact redesign behind this Change exists across one long Codex conversation, multiple compactions, scattered journal entries, prior research, and local repository evidence. There is no first-class Exploration to resume, no first-class Intent that means “this is tracked work worth returning to,” and no relational model connecting those concepts to their source conversations, checkpoints, handoffs, wraps, reports, Changes, or other Loaf entities.

The absence is already causing vocabulary and ownership drift. Spark, Idea, pre-Change tracker issue, deferred work, and Change are forced to impersonate one another. `journal defer` retains only a four-field packet plus a Spark projection; a substantial exploration cannot be preserved as structured context. A scalar `harness_session_id` can correlate some journal entries but cannot express that one Exploration spans multiple conversations, session handles, harnesses, log fragments, or successor Changes. Machine-local IDs may aid forensic navigation but cannot make a fresh checkout or another machine self-sufficient.

Trying to solve that with mutable `active`, `paused`, or `complete` records would repeat the Session model's failure: concurrent agents and harnesses cannot reliably maintain shared lifecycle flags. Putting authored artifacts back into SQLite would repeat the render/spec authority problem. Letting Skills choose storage, call Linear directly, or hand-roll worktree operations would repeat the current cross-harness inconsistency. The missing foundation is relational, append-only operational state plus small deterministic CLI primitives—not another workflow engine.

## Hypothesis

If Loaf introduces project-scoped Intent and Exploration identities, immutable portable checkpoints, normalized many-to-many conversation/log provenance, append-only Intent dispositions, validated typed relationships, and deterministic provider-neutral CLI operations, then an idea can become tracked, an exploration can survive compaction and harness changes, and substantial deferred work can be resumed without recreating a session lifecycle or requiring its original local logs. `/triage` and `/explore` can then become thinner and more predictable, while later Changes can materialize Git workspaces and Linear coordination without reinventing capture, provenance, or deferral.

## Scope

**In**

- Add first-class project-scoped Intent and Exploration identities to SQLite through a strictly additive migration. Neither entity has a mutable lifecycle status.
- Represent an Intent's current disposition as a derived view over append-only events. The initial vocabulary is `tracked`, `deferred`, and `resolved`; resuming appends a new `tracked` disposition linked to the deferral it supersedes. Every append receives a transactionally allocated immutable per-Intent sequence with a uniqueness constraint, so concurrent commits have one total order independent of wall-clock timestamps.
- Preserve every deferral as an immutable self-sufficient snapshot containing the retained proposition/body, why it matters, the boundary that excluded it, the future trigger or review condition, the operation key, and source relationships.
- Add one canonical `intent_operations` mapping shared by `intent create`, `intent defer`, the transitional `journal defer` adapter, and legacy conversion. Each project-scoped operation key resolves to one Intent and stored digest, with optional historical journal/Spark projection IDs and a projection version, so retries and compatibility paths cannot mint parallel canonical records. A canonical-first write records projection version 0 with null legacy IDs; a later `journal defer` deterministically materializes both legacy projections from the stored canonical packet and advances the mapping to version 1 in one transaction.
- Represent Exploration continuity through immutable checkpoints with four required portable fields—purpose/current framing, conclusions or constraints so far, unresolved question or decision, and recommended next action—plus optional typed items such as candidates/evidence and explicit ordering. Each core field is trimmed, non-empty, and capped at 4096 UTF-8 bytes; oversize input fails with a stable field-specific error and is never truncated. Every append receives a transactionally allocated immutable per-Exploration sequence. An early Exploration may exist without a portable checkpoint and must report that honestly; a new conversation continues it by adding a checkpoint or source relationship, not by toggling lifecycle state.
- Normalize logical conversations separately from machine/harness-local handles and log references so one conversation may carry multiple opaque IDs and one Exploration may span many conversations and harnesses. Store locators, hashes, and bounded ranges—not raw transcript bodies. Availability is either probed ephemerally or appended as a timestamped/locality-scoped observation; it is never a mutable property of the handle/log reference.
- Relate the new entities through a closed entity-kind and relationship-type registry with project and endpoint validation. This Change's required matrix is bounded to Intent, Exploration, checkpoint, logical conversation, Spark, Idea, Brainstorm, journal entry (including wrap), handoff, report, and finding; Git Change references and `materialized-as` edges begin in `change-native-execution-migration` when the target exists and its authority is defined.
- Add focused deterministic `loaf intent`, `loaf exploration`, `loaf conversation`, and `loaf intake` primitives to create, inspect, list, relate, defer, resume, resolve, checkpoint, attach provenance, and project portable Exploration context. Commands execute an explicitly requested operation and report facts; they do not decide which concept applies.
- Bound every collection in `exploration context` independently. The portable checkpoint core is always returned whole; optional items, sources, relationships, and evidence report available/shown counts, truncation, a stable next cursor, and the exact layer-expansion command.
- Add a deterministic local intake projection spanning open Sparks, Ideas, Brainstorms, unresolved Intents, and legacy deferrals without semantically promoting any item.
- Replace deferred-Spark-only continuity with bounded derived Intent/Exploration projections: unresolved deferred Intent is active truth, and recent portable Exploration checkpoints are discoverable with counts and exact expansion/context commands rather than silently flooding startup context.
- Make new Intent deferral atomic and retry-safe. Keep `journal defer` as a transitional compatibility adapter that cannot create a new legacy-only deferral after the new model exists.
- Provide a project-scoped dry-run/manifest/apply conversion for existing `journal_deferrals`, protected by a verified whole-database backup and a project-specific manifest; link each converted Intent to its historical decision and Spark, preserve legacy rows, report unparseable records, and make retries idempotent.
- Introduce the user-facing `/explore` workflow, preserve the full Brainstorm stance as an internal technique, and narrowly converge `/triage`, Idea, and Spark guidance on the new concepts. Generated target output and routing evaluations are part of the same vertical change.
- Evolve the non-user-invocable `loaf-reference` Skill into the config-aware maintenance operator for the in-scope rollout. It consumes project intent through `loaf config check --json`, combines that with observed package, installed-target, SQLite, and checkout facts, discovers exact syntax from live CLI help, asks only for missing project-owned choices, sequences approved deterministic operations, and verifies convergence instead of hardcoding a version-specific recipe.
- Add the non-mutating machine surfaces that maintenance requires for this Change: JSON project-alignment diagnosis from `loaf doctor` and dry-run/JSON planning for `loaf install --upgrade`. The plan reports selected installed targets, owned changes, preserved conflicts, deprecation actions, and project-file effects without mutating either the checkout or harness homes; apply remains an explicit existing install operation.
- Document the proven authority boundary in current architecture/schema guidance after implementation: operational Intent/Exploration state in SQLite, authored durable artifacts in Git, and no tracker authority until the later coordination Change.

**Out** (deferred, not rejected)

- Intent-to-Change promotion, branch/worktree creation, `change.md`/`plan.md` scaffolding, draft PR creation, artifact-effect reconciliation, and Change-native implementation/ship behavior — owned by `change-native-execution-migration`.
- Linear or GitHub issue adoption/publication, remote IDs and revisions, polling, synchronization, outbox/retry/conflict behavior, tracker-backed intake, issue completion policy, and agent-from-tracker bootstrap — owned by `linear-native-coordination`.
- `/plan`, `/implement`, `/ship`, `/release`, `/reflect`, and `/housekeeping` convergence; public `/breakdown`, Spec, Task, and render removal; broad README/strategy/guidance cleanup — owned by `spec-conversion-and-guidance-sweep` unless an earlier successor must touch a narrow consumer for correctness.
- A top-level `loaf upgrade` coordinator, package-manager mutation/self-update, fleet-wide or all-project upgrade sweeps, and a user-invocable `/maintain` Skill. Dogfood the hidden config-aware `loaf-reference` protocol and deterministic plan surfaces first; a later Change may add a thin public ceremony if repeated use justifies it.
- Cross-worktree activity scanning, automatic sibling-worktree notices, work claiming, portfolio/program views, or a mutable worktree registry.
- Rewriting handoff or wrap semantics. They remain SQLite/journal snapshots and may be related to an Exploration through the generic relationship surface.
- Copying, ingesting, indexing, or synchronizing raw prompts, transcripts, tool output, provider payloads, or arbitrary external documents.
- Cross-device state synchronization or any guarantee that a machine-local conversation/log locator remains navigable elsewhere.

**Cut** (explicitly rejected)

- `intent.status`, `exploration.status`, `active_exploration`, pause/complete lifecycle transitions, or a globally current conversation/session/worktree.
- Treating “resume Exploration” as a mutable lifecycle operation; resumption means reading portable context and appending new evidence/checkpoints.
- Automatically converting every Spark, Idea, Brainstorm, issue, or journal entry into an Intent.
- Storing Linear/GitHub fields or backend-selection flags on Intent/Exploration rows before the tracker Change defines synchronization authority.
- Arbitrary polymorphic relationship strings without a closed registry and endpoint validation.
- A second canonical body store for Changes, plans, specs, ADRs, knowledge, reports, or code.
- Direct MCP/provider calls, storage-mode branches, Git/worktree shell recipes, or semantic classification logic inside Skills.
- CLI heuristics that decide whether input is a Spark, Idea, Intent, Exploration, deferral, or Change. The caller supplies the judgment; the CLI validates and performs it.
- Durable authored artifacts under `.agents/`, other than the existing tracked shared Loaf configuration; disposable scratch remains explicitly non-authoritative.

## Observable Workflow

### Track a considered direction

```text
conversation or triage judgment: this is worth tracking
  → loaf intent create --title <title> --body <self-sufficient body> --from <source>...
  → CLI validates source endpoints, writes one Intent snapshot plus tracked disposition, and returns stable local identity
  → no branch, worktree, Change folder, tracker issue, or journal prose is invented
```

### Explore across conversations and harnesses

```text
$ loaf exploration create --title "Workflow and artifact model" --from <intent-or-source>...
$ loaf exploration checkpoint <exploration> --purpose <current framing> --conclusions <constraints or conclusions> --unresolved <question or decision> --next <recommended action> [--item candidate:<...> --item evidence:<...>]
$ loaf conversation create --title <label> --operation-id <key>
$ loaf conversation handle add <conversation> --harness codex --handle <opaque-local-id> [--locality <machine-or-namespace> --log-ref <locator> --hash <digest> --range <range>]
$ loaf exploration conversation add <exploration> <conversation>

another conversation or harness later:
$ loaf exploration context <exploration> --json
  → portable checkpoint present: true
  → complete portable core plus bounded summaries of ordered items, linked Intents/entities, journal/wrap/handoff evidence, conversations, and source locators
  → every optional layer reports available/shown counts, truncation, a stable next cursor, and the exact `--layer <name> --cursor <cursor> --limit <n>` expansion command
  → source handles and log locators shown separately with locality and ephemeral/latest observed availability; their presence never implies portable context
```

### Defer substantial work at any stage

```text
human or Skill decides this coherent body belongs later
  → existing Intent: loaf intent defer <intent-ref> --why <why> --boundary <what excluded it> --trigger <when to revisit> --operation-id <key>
  → new deferred Intent: loaf intent create --title <title> --body <self-sufficient body> --disposition deferred --why <why> --boundary <what excluded it> --trigger <when to revisit> --operation-id <key> --from <source>...
  → one serializable transaction consults/writes the shared operation mapping and writes or returns exactly one Intent snapshot, deferred disposition, immutable deferral payload, and validated source relationships
  → response-loss retry with the same key returns the first write and reports digest equality/mismatch

later:
  → loaf intent resume <intent> --reason <why now>
  → CLI appends a tracked disposition linked to the prior deferral; history is never overwritten
```

### Triage local intake

```text
$ loaf intake list --json
  → deterministic projection of unresolved Sparks, Ideas, Brainstorms, Intents, and legacy deferrals
  → provenance, current derived disposition, duplicate/source relationships, and exact follow-up read commands

/triage chooses: discard, retain as capture, track as Intent, research, explore, resume, resolve, or later publish
```

## Rabbit Holes and No-Gos

- Do not design the eventual Linear state machine in this foundation. Absence of provider fields is deliberate; later mappings bind stable local identity to remote authority.
- Do not turn an Exploration into a notebook/document platform. Portable checkpoints are curated semantic context, not a mirror of every message or artifact.
- Do not make “relates to everything” mean “accepts any string.” Centralized kind/type registration and endpoint validation are required to prevent relationship entropy.
- Do not create mutable convenience fields that can disagree with append-only dispositions or checkpoints. Derived reads may cache only with source hashes/watermarks and self-invalidation.
- Do not let compatibility with `journal defer` keep the journal-decision/Spark pair authoritative. The bridge is temporary, explicit, and covered by the terminal removal gate.
- Do not broaden this Change into Change materialization merely because this Change itself is dogfooding `change.md` plus `plan.md`; that belongs to the next successor.
- Do not rename every workflow Skill now. `/explore` is deliberately provisional; dogfood behavior before the terminal naming/routing sweep.
- Do not repair the known global journal-search divergence as an incidental side effect. New migration/tests use isolated state; repair of real state remains a separately consented operator action.

## Decisions

Provenance: Decisions 1–12 were accepted through the 2026-07-15–17 workflow exploration, explicit user adjudication, current source/state inspection, and prior comparative research. Decision 13 and the ordering/portability/availability refinements were accepted through three independent read-only convergence critiques of the shaped packet. Decision 14 was accepted while dogfooding the alpha.8 upgrade/install path and revisiting the existing `loaf-reference` maintenance boundary. The current Change records the accepted delivery-scoped ledger; no new ADR is justified before dogfood proves which constraints remain cross-Change and load-bearing.

1. **Intent is a first-class backend-neutral concept, distinct from Spark, Idea, Exploration, issue, and Change.** Spark captures an interruption; Idea retains a proposition; Intent means work is deliberately tracked; Exploration develops understanding; an issue may later publish/adopt Intent; Change is bounded delivery.
2. **Deferred is a disposition of Intent, not another entity or workflow stage.** The stage that discovered it is provenance; the retained body, rationale, boundary, and trigger are the portable contract.
3. **Exploration is a relational inquiry aggregate, not a session.** It spans conversations, harnesses, Intents, and evidence through immutable checkpoints and relations, with no active/paused/done status; later materialization may relate it to a Change without changing this model.
4. **Portable context and local provenance are separate claims.** A checkpoint must be sufficient to resume elsewhere; a harness session ID or log locator is optional machine-local evidence and never satisfies portability by itself.
5. **SQLite uses append-only, totally ordered facts to avoid staleness.** Current Intent disposition and latest Exploration context are derived from transactionally sequenced immutable records rather than timestamps; no agent must maintain shared lifecycle flags after every conversation.
6. **The CLI is a deterministic mechanism layer.** It validates, persists, queries, links, migrates, checks, and serves hooks; humans and Skills interpret input, make recommendations, choose the operation, and author prose.
7. **`/triage` is the public intake funnel and `/explore` is the divergent workflow.** Idea and Spark remain capture primitives reachable from natural conversation; Brainstorm's full stance survives internally inside Explore; `scout`, prototype, and spike remain subordinate techniques rather than competing lifecycle owners.
8. **The foundation is local and provider-neutral.** Local tracked Intent is canonical in SQLite until a later tracker Change publishes/adopts it; no Skill contains a Linear-enabled branch or direct provider call.
9. **Relationships are broad but typed and validated.** New entity kinds integrate with one centralized registry used by create/link/trace/export/delete/doctor surfaces; free-form dangling graph edges are rejected.
10. **Legacy deferrals converge without destructive rewriting.** Existing journal decisions and Sparks remain historical evidence; deterministic conversion adds canonical Intent links and migration markers, and the old command becomes a bridge until the terminal cut.
11. **A Change is self-sufficient in a fresh checkout, not detached from the repository.** This Change materializes the relevant exploration synthesis under `research/`; raw local conversation handles remain provenance, while future Changes continue editing canonical durable artifacts in place and record their affected paths.
12. **The hard-cut lineage expands to five nodes.** The original three-node decision was correct for the earlier model; the approved workflow redesign adds this foundation and Linear-native coordination so execution and final convergence do not invent those contracts during migration/removal.
13. **The initial relationship matrix is deliberately bounded.** The registry supports future extension, but this Change proves only the edges required to connect capture, Intent, Exploration, portable checkpoints/conversations, journal/wrap/handoff evidence, and reports/findings. Git Change origin/materialization belongs to the next Change; unsupported legacy pairings remain explicit future extensions rather than an accidental whole-state-plane refactor.
14. **Loaf maintenance is a hidden config-aware Skill over deterministic observed-state plans.** `.agents/loaf.json` records shared project intent, not machine-specific harness installation state. The CLI diagnoses and plans from config plus actual local state; `loaf-reference` interprets those facts, asks for missing choices, invokes approved operations, and proves convergence. This Change strengthens that existing hidden Skill rather than adding `/maintain`, and it does not let Loaf replace the package manager that owns the executable.

## Planning Contract

The authoritative implementation route is [plan.md](plan.md). This compatibility section exists because the current `loaf change check` still expects planning inside `change.md`; this Change deliberately dogfoods the accepted separation where `change.md` owns the Product Contract and `plan.md` owns the transition from current to desired state.

## Implementation Units

Named units, dependencies, per-unit acceptance criteria, sequencing, rollback, and verification live in [plan.md](plan.md#units-of-work). Units use stable slugs rather than `U1`/`TASK-*` identifiers and are not separately persisted workflow entities.

## Verification Contract

Executable:

- **Schema convergence.** Migration fixtures beginning at schema 9, explicit journal-first schema 10, and current schema 11 converge to the new additive schema without altering prior migration checksums; interrupted/failed application leaves no partial tables or records.
- **No lifecycle status.** Schema and current-source grep gates reject mutable status/current-session/current-exploration fields for the new entities; derived disposition/context tests rely only on ordered immutable facts.
- **Intent atomicity and ordering.** Create/defer/resume/resolve operations validate required content and project ownership; failure injection at every transactional stage leaves no partial snapshot, disposition, deferral, alias, relationship, compatibility projection, or migration marker. Concurrent conflicting appends allocate unique immutable per-Intent sequences, and derived disposition follows the greatest committed sequence rather than timestamp.
- **Retry convergence.** Concurrent identical and reworded deferral calls with one operation key return one first-written Intent/deferral; `intent create --disposition deferred`, `intent defer`, `journal defer`, and legacy conversion all consult the same canonical operation mapping; output exposes stored/input digests and whether they match without duplicating or rejecting response-loss recovery. Tests cover canonical-first, adapter-first, and concurrent mixed entry points. When the canonical write is first, a later adapter call creates the missing journal/Spark pair from stored canonical content, atomically advances projection version 0→1, returns both IDs, and reports whether the adapter input digest matched.
- **Exploration portability, ordering, and boundedness.** Context fixtures span multiple logical conversations, harness handles, log references, journal entries, handoffs, wraps, and Intents; stable output returns the greatest committed per-Exploration checkpoint sequence even when every local handle/log is unavailable. A checkpoint is portable only when all four required fields are non-empty and within their 4096-byte per-field limits; the fixture asserts that `recommended_next_action` survives byte-for-byte with every source locator unavailable, and oversized core input fails without a partial write. Every optional layer is bounded and cursor-expandable while the intrinsically bounded portable core remains whole.
- **Provenance honesty.** A source handle without a valid portable checkpoint reports `portable_context_present=false`; absent harness/session/log fields remain absent rather than fabricated; availability is an ephemeral probe or immutable timestamped observation rather than a mutable handle field; raw transcript/tool/provider bodies never enter state fixtures.
- **Graph integrity.** Every pairing in the bounded initial relationship matrix participates in link, trace, show, export, project deletion, backup/restore, doctor/status, and search/index parity; invalid kinds/types, unsupported pairings, dangling endpoints, duplicates, and cross-project edges fail visibly.
- **Local intake.** Deterministic intake fixtures project each logical unresolved item once, preserve source/duplicate relations, expose legacy deferrals before conversion, and show the converted Intent exactly once afterward without making a semantic disposition.
- **Continuity projection.** Journal context fixtures show unresolved deferred Intent independently of recency, expose bounded recent portable Exploration checkpoints with counts/truncation/expansion commands, deduplicate legacy Spark projections after conversion, and never treat a local handle as resumable context.
- **Legacy bridge.** Dry-run is byte-for-byte non-mutating; apply verifies a whole-database backup, emits a project-specific complete manifest, populates the shared operation mapping, converts parseable historical deferrals idempotently, reports unparseable packets without guessing, preserves old rows, and prevents all new `journal defer` calls from remaining legacy-only.
- **Skill boundary.** Routing and content gates prove `/triage` chooses dispositions, `/explore` creates/resumes Exploration context, Brainstorm remains an internal technique, Idea/Spark remain captures, and no in-scope current or generated Skill contains direct tracker calls, backend selection, worktree recipes, or CLI-owned semantic judgment.
- **Maintenance planning.** `loaf doctor --json` is fact-equivalent to human diagnosis and never mutates; `loaf install --upgrade --dry-run --json` produces a deterministic complete plan and leaves checkout, harness homes, ownership manifests, config, and SQLite byte-for-byte unchanged. Fixtures cover current and behind-schema state, recognized installed targets, absent targets, owned stale content, locally modified/unowned content, deprecation cleanup with and without explicit consent, and the new Intent/Exploration commands/reference metadata.
- **Maintenance Skill.** Routing/content evaluations prove requests to upgrade, diagnose, repair, configure, or bring a project current select the hidden `loaf-reference` protocol; it reads CLI JSON/actions and live help, does not infer machine-local target intent from `.agents/loaf.json`, does not claim a newer remote version without package-manager evidence, and does not introduce a public `/maintain` or hardcoded binary path.
- **Repository gates.** `go test ./...`, `npm run typecheck`, `npm run test`, `npm run build`, `loaf change check --require-executable docs/changes/20260717-intent-exploration-foundation`, relevant explicit hook checks, generated-target verification, schema-doc parity, `git diff --check`, and focused source/generated gates for the in-scope Skills all pass using isolated absolute `LOAF_DB` paths for disposable tests. The full retired-vocabulary allowlist remains exclusively terminal work.

Human review:

- A fresh agent can read the Git-tracked Change/research packet and understand why Intent and Exploration exist without access to this conversation or the local SQLite database.
- A fresh harness can resume an Exploration from its portable checkpoint while clearly seeing which optional source handles are unavailable locally; a blind fresh-agent exercise identifies the intended next action without consulting the source conversation.
- Triage distinguishes capture, tracked intent, deferred disposition, inquiry, and bounded delivery without forcing a tracker or creating branches prematurely.
- The CLI surface contains only deterministic operations and factual projections; all semantic classification and scope judgment remains in humans/Skills.
- The Change remains bounded before Change materialization and tracker coordination, and every deferred capability has a self-sufficient successor packet.

## Definition of Done

- Intent, append-only dispositions, immutable deferrals, Exploration checkpoints/items, logical conversations, local handles/log references, and validated relationships are implemented as project-scoped SQLite state with documented authority.
- `loaf intent`, `loaf exploration`, `loaf conversation`, and the local intake read surface provide deterministic human/JSON contracts with idempotent writes, bounded reads, and no mutable workflow lifecycle.
- Project continuity surfaces unresolved deferred Intent and discoverable portable Exploration checkpoints without depending on a mutable current-Exploration pointer.
- Existing `journal_deferrals` are discoverable before migration and can be converted through a project-scoped, manifest-driven, rerunnable path protected by a verified whole-database backup without deleting historical evidence.
- `/explore`, `/triage`, Idea, Spark, Brainstorm, config-aware Loaf reference guidance, routing evaluations, generated targets, and installed-surface fixtures agree on one vocabulary and one CLI/Skill boundary; maintenance diagnostics and upgrade plans cover the new commands, migration, and installed artifacts without adding another public Skill.
- Cross-conversation/harness dogfood proves portable context survives loss of local locators; concurrent Intent/deferral dogfood proves append-only state does not go stale.
- All executable and human gates pass; actual durable-artifact effects are reconciled against the final Git diff; successor packets are updated with implementation evidence before Ship.

## Durable Artifact Changes

Expected paths are recorded now and reconciled against the actual Git diff at Ship; `add`, `modify`, and `remove` describe intended effects rather than shadow copies.

- `docs/changes/20260717-intent-exploration-foundation/change.md` — **add** the Product Contract and successor packets.
- `docs/changes/20260717-intent-exploration-foundation/plan.md` — **add** the implementation route, named units, dependencies, and gates.
- `docs/changes/20260717-intent-exploration-foundation/research/workflow-convergence.md` — **add** the portable synthesis of this multi-conversation exploration and current-source audit.
- `docs/changes/20260710-journal-reliability-foundation/change.md` — **modify** only its living successor protocol and forward-looking gates; preserve shipped implementation claims and original decisions as history.
- `internal/state/migrations/0012_intents_and_explorations.sql`, state APIs/tests, schema upgrade/export/delete/status/trace/search surfaces — **add/modify** the relational operational model after implementation proves the physical layout.
- `internal/cli/` command registration, parsers, JSON contracts, help/reference generation, install-upgrade planning, doctor diagnostics, tests, and safe-command classification — **add/modify** deterministic Intent/Exploration/intake/migration and config-aware maintenance primitives.
- `content/skills/{triage,idea,brainstorm,explore,loaf-reference}/` (spark remains a CLI capture primitive carried by idea and triage guidance), including a config-aware maintenance protocol/reference, routing evaluations, shared templates/references, `config/hooks.yaml` only where deterministic provenance capture requires it, and every tracked generated target — **add/modify** the focused workflow and maintenance surface.
- `docs/ARCHITECTURE.md`, `docs/schema/`, and narrowly affected current knowledge — **modify/add** only behavior proven by implementation; broad Spec/Task/render/ADR convergence remains terminal work.
- No canonical spec, ADR, or knowledge body is copied into SQLite or duplicated under this Change. A later reflect pass may distill an ADR only if the implemented constraint passes the reversal test across Changes.

## Open Questions

- [KU] Which existing `artifact_bodies`/search primitives can be reused for Intent snapshots and checkpoints without inheriting `artifact_entities.go` lifecycle assumptions? → `establish-relational-foundation` source spike; preserve immutable-record and parity constraints regardless of physical layout.
- [KU] What exact compatibility projection should `journal defer` return after Intent becomes canonical without breaking installed callers or producing a legacy-only write? → `make-intent-operations-atomic` fixture-led adapter design; terminal removal remains mandatory.
- [KU] How should a logical conversation be explicitly associated with multiple harness handles when no target exposes a trustworthy parent/conversation identity? → `make-exploration-portable` accepts explicit association and absent fields; never infer from branch, worktree, or recency.
- [UK] Does `/explore` remain the clearest public name after real dogfood, or would a unique name such as a wayfinding metaphor route better across harnesses? → retain `/explore` provisionally and evaluate in the terminal routing/naming sweep.

## Critique Gate

- **Did independent critique change the model?** Yes. It rejected mutable Intent/Exploration statuses, made disposition/checkpoint facts append-only, separated logical conversations from local handles, required a closed relationship registry, and tightened the foundation away from Change workspaces and trackers.
- **Is Intent another task entity?** No. It carries tracked direction and disposition, not execution assignment, estimates, dependencies, or implementation status. Plan units remain Git-canonical in a later Change and tracker mappings remain later still.
- **Is Exploration another session entity?** No. It has no active owner, pause, completion, or automatic current pointer. It is an identity over immutable checkpoints and relationships; concurrent conversations append without coordinating lifecycle flags.
- **Can SQLite still go stale?** Derived views can be rebuilt from append-only facts; compatibility writes and data migration are transactional/idempotent; cached projections require source hashes/watermarks. External tracker drift is deliberately absent until the coordination Change defines reconciliation.
- **Is the CLI deciding workflow?** No. It exposes deterministic verbs and validates explicit caller intent. Triage/Explore/humans decide classification, scope, and disposition.
- **Is config-aware maintenance scope creep?** It would be if this Change added package-manager control, fleet orchestration, a public workflow, or new install ownership semantics. The accepted unit is limited to making the new commands, migration, generated Skills, and project guidance diagnosable and safely previewable through existing ownership/apply contracts. If implementation cannot share the existing doctor/install analysis without a broader installer redesign, that redesign becomes deferred Intent rather than expanding this foundation.
- **Is the Change self-sufficient?** Yes. The Product Contract, plan, research synthesis, source IDs, authority boundary, successor packets, and affected paths live in Git. Local handles enrich provenance but are unnecessary to understand or implement the Change.
- **Could this be smaller?** Removing Intent would leave deferment and tracked work conflated with Spark/Idea; removing Exploration would leave the source continuity problem unsolved; adding workspace or tracker work would exceed the hypothesis. The selected boundary is the smallest relational substrate that supports both local continuity and later promotion.

## Source Inputs

- `research/workflow-convergence.md` — portable synthesis of the prior OpenSpec/compound-engineering/Matt Pocock/NotebookLM review, current source/state audit, authority model, Skill simplification, and successor decomposition.
- Codex desktop thread `019f62e6-88d2-7630-96ce-527652fd9a0b` — machine-local source conversation spanning multiple compactions; retained as provenance, not required context.
- `journal:0036cd92bc4412bbe6c0c4c3` and `journal:3ab77620fc36c7100788901b` — accepted five-node lineage amendment and final shaped Intent/Exploration packet.
- `journal:a7f6ccf3c9d2ed8c6b05d9f1` — validation discovery that the current CLI requires explicit `loaf check --hook <id>` leaves rather than the documented bare aggregate command.
- `journal:50205f69f32e2aedc14bf6eb` and `journal:a324913596ab3cd649235344` — accepted first-class Exploration and normalized many-to-many source provenance.
- `journal:7fd96405e8b50b4fdbde57a0`, `journal:8aa7a6e47d958d454fc66b24`, and `journal:28a3a2e2689f8e6e36860499` — Change portability, machine-local handle boundary, and locator/hash/range storage.
- `journal:087d2ce3294618229858f140` — accepted sequence: Exploration continuity and materialization before rich deferral/workflow cleanup.
- `journal:56e7af05c430a63df2047ddb` — backend-neutral Intent with deferred as its disposition.
- `journal:e3c0c830df1fcea2d925d52d` and `journal:b58088cb742a3474c0010b01` — deterministic CLI/judgment-bearing Skill boundary and holistic workflow convergence.
- `journal:612e9b835366823dbd843c23` — Idea/Spark distinction and Triage as the public intake surface.
- `journal:e2ce66d639fbb3c04a7a89b1` and `journal:dcd10ffd88692bdd6b3d0b06` — `change.md`/`plan.md` authority split and direct affected-artifact recording.
- `journal:3274f922f1cc09e24d80cd12` and `journal:fa885c8d627e27e407ea86cb` — later worktree mechanization and Linear/Git authority boundary.
- `docs/changes/20260710-journal-reliability-foundation/change.md` — predecessor reliability contract, lineage authority, journal-origin envelope, atomic deferral evidence, and parked-terminal boundary.
- Current implementation under `internal/state/{migrations/0001_initial.sql,migrations/0011_journal_origins_and_deferrals.sql,journal_defer.go,trace.go,artifact_entities.go,schema.go}`, `internal/cli/{change.go,cli.go,journal.go}`, `content/skills/{triage,idea,brainstorm,shape,implement}/`, `config/hooks.yaml`, and current generated targets.
