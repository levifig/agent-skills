# Intent and Exploration Foundation Plan

## Contract

This plan moves Loaf from scattered capture records and journal/Spark deferrals to the bounded product contract in [change.md](change.md). The Change defines what must become true; this file owns implementation order, dependencies, review boundaries, rollback, and verification. Unit slugs are stable local handles and potential commit/review boundaries, not numbered Tasks or mutable workflow entities.

## Starting State

- Schema 11 has Ideas, Sparks, Brainstorms, generic aliases/relationships/bodies, journal origins, and `journal_deferrals`, but no Intent, Exploration, logical conversation, immutable checkpoint, or portable context aggregate.
- `journal defer` atomically writes a journal decision and open Spark keyed by an operation ID. Its four-field packet and retry behavior are useful implementation evidence, but the pair is not the target authority.
- `relationships` accepts polymorphic endpoints but entity-kind support is distributed across fixed switch/list sites; endpoint and relationship semantics can drift.
- Journal rows may carry one opaque `harness_session_id` and normalized origin fields, but Loaf cannot group multiple handles/logs under a logical conversation or attach many conversations to one Exploration.
- `/triage`, Idea, Brainstorm, Shape, Implement, orchestration, and Linear guidance still encode overlapping legacy workflow ownership. This Change updates only the intake/exploration/capture slice needed to consume the new state.
- The global journal search index is known to diverge from canonical rows. Implementation and migration tests use isolated absolute `LOAF_DB` paths; this Change must not repair real global state implicitly.

## Implementation Invariants

- New state is additive, project-scoped, and transactionally verified. Explicit project-scoped data conversion requires a project-specific dry-run manifest plus a verified whole-database backup because the existing recovery operation restores the global database as one unit.
- Intent disposition and Exploration continuity are derived from immutable facts with a transactionally allocated per-aggregate sequence and uniqueness constraint; wall-clock time never decides between concurrent appends, and no mutable lifecycle status or global-current pointer is introduced.
- Every retry-safe compound write uses a caller-supplied/generated operation key, returns the first write after response loss, and reports semantic digest drift.
- Portable checkpoint content remains useful after every conversation handle, log locator, branch, worktree, and source artifact disappears. A valid portable checkpoint requires purpose/current framing, conclusions or constraints, unresolved question or decision, and recommended next action.
- Conversation handles and logs are bounded provenance records, not transcript storage and not proof that context is portable. Availability is ephemeral probe output or an immutable timestamped/locality-scoped observation, never a mutable handle property.
- Entity kinds and relationship types are centrally registered; both endpoints must exist in the same project. Git Change path references and materialization edges begin in `change-native-execution-migration`, where their validation and authority can be defined with the target workspace model.
- Skills choose semantic operations. CLI/state code validates and performs them deterministically. Hooks may supply observable provenance but never semantic classification.
- Shared project intent comes from `.agents/loaf.json`; package provenance, installed harness targets, ownership manifests, SQLite readiness, and checkout alignment are observed local facts. The hidden `loaf-reference` Skill may interpret both, but it consumes CLI diagnostics/actions instead of duplicating config parsing, target detection, or mutation logic.
- Legacy compatibility cannot create state invisible to the new model after the cutover unit lands.

## Units of Work

| Unit | Delivers | Depends on |
|------|----------|------------|
| `establish-relational-foundation` | Additive schema, registries, immutable fact model, upgrade/parity fixtures | — |
| `make-intent-operations-atomic` | Intent reads/writes, append-only dispositions, rich deferral, legacy command bridge | `establish-relational-foundation` |
| `make-exploration-portable` | Logical conversations, handles/log refs, checkpoints/items, portable context | `establish-relational-foundation` |
| `connect-intake-and-trace` | Validated relationships, trace/export/delete/status integration, local intake projection | `make-intent-operations-atomic`, `make-exploration-portable` |
| `make-loaf-maintenance-config-aware` | Hidden maintenance protocol plus non-mutating doctor/install-upgrade plans covering the new release surface | `connect-intake-and-trace` |
| `converge-explore-and-triage` | `/explore`, Triage/capture vocabulary, deterministic CLI use, generated/routing convergence | `make-loaf-maintenance-config-aware` |
| `migrate-and-dogfood-intent-model` | Manifest-driven legacy conversion, failure recovery, cross-context dogfood, final docs/effect reconciliation | all preceding units |

### `establish-relational-foundation`

**Deliverable:** One additive schema migration and one centralized entity/relationship contract that every downstream surface can use without lifecycle abstractions.

**Implementation:**

- Add `0012_intents_and_explorations.sql` and register it after schema 11 without changing prior SQL/checksums or the explicit schema-10 journal-first ceremony.
- Represent stable Intent/Exploration identity separately from immutable content/history. The physical layout may reuse generic body/search primitives only if it preserves immutable revisions/checkpoints and does not pass through `artifact_entities.go` status semantics.
- Store append-only Intent snapshots/dispositions and immutable deferral payloads. Enforce one project-scoped operation key per compound mutation and checksums for self-sufficient content.
- Add the canonical `intent_operations` contract with `(project_id, operation_key)` as its unique identity, `intent_id` and `stored_digest` as required canonical results, nullable `journal_entry_id`/`spark_id` historical projection references, `projection_version`, and `created_at`. Projection version 0 requires both legacy IDs to be null; version 1 requires both to be non-null. `intent create`, `intent defer`, `journal defer`, and legacy conversion must consult and populate this same mapping inside their canonical transaction.
- Allocate immutable per-Intent and per-Exploration append sequences transactionally with unique aggregate/sequence constraints. Reads derive “latest” from the greatest committed sequence, not timestamp or lexical ID.
- Store Exploration checkpoints with four required portable fields and ordered typed checkpoint items. Trim each core field and cap it at 4096 UTF-8 bytes; reject overflow with a stable field-specific error and no partial write. The database must distinguish a structurally valid portable checkpoint from an early Exploration with only optional source references. Larger detail belongs in bounded checkpoint items or related evidence/report references, or in a later checkpoint; no read or write path truncates the core.
- Normalize logical conversations, harness/machine-local handles, bounded log references, Exploration↔conversation membership, and journal↔conversation-handle association. Do not persist raw transcript/tool/provider bodies.
- Centralize entity-kind and relationship-type registration used by alias resolution, link creation, trace details, export, deletion, mapping audits, state status/doctor, and test fixtures. Retain existing supported legacy kinds through explicit registration rather than a broad compatibility wildcard.
- Store source availability only as ephemeral probe results or immutable observations carrying observed time, observer/locality, and result; do not update handle/log rows when reachability changes.
- Add constraints/indexes for project scope, sequence allocation, stable ordering, idempotency, duplicate prevention, and common context/intake queries.

**Acceptance:**

- Upgrade fixtures from schema 9, explicit schema 10, and schema 11 converge to schema 12 with prior checksums intact.
- Schema inspection finds no mutable Intent/Exploration status/current pointer.
- Invalid type combinations, duplicate operation keys, dangling endpoints, and cross-project joins fail before commit.
- Concurrent conflicting appends receive distinct committed sequences; equal timestamps cannot produce two “latest” records.
- Search/index parity can be checked/rebuilt from canonical new facts independently of the known global journal-search divergence.

**Rollback:** Before merge, revert the migration and code together against disposable fixtures. After intentional dogfood, use the verified pre-migration backup or forward repair; never edit migration 12 after release.

### `make-intent-operations-atomic`

**Deliverable:** A deterministic `loaf intent` resource surface and one canonical compound deferral operation.

**Implementation:**

- Implement create/list/show around stable identity, latest immutable snapshot, derived latest disposition, aliases, sources, and relationships. `intent create` defaults to `tracked`; a new self-sufficient deferred direction uses `intent create --disposition deferred --why ... --boundary ... --trigger ... --operation-id ...`.
- Implement `intent defer <intent-ref> --why ... --boundary ... --trigger ... --operation-id ...` only for an existing Intent. Both new-deferred creation and existing-Intent deferral use one serializable canonical operation that consults/writes `intent_operations` and writes or returns the snapshot, deferred disposition, immutable four-field payload, operation/digests, aliases, and source links.
- Implement `resume` by appending a new tracked disposition linked to the deferral it supersedes; implement `resolve` by appending a reasoned terminal disposition. Neither command updates a status column.
- Keep automatic output factual: IDs, created/reused, derived disposition, input/stored digests, sources, and exact read commands. Do not generate a workflow recommendation.
- Convert `journal defer` into a compatibility adapter over the same canonical Intent operation so it cannot create a new journal/Spark-only record. The adapter and canonical command consult/write `intent_operations` in the same transaction; only the adapter must create the legacy journal/Spark factual projections and record their IDs/projection version. If a canonical command won the key first, the adapter locks/reads that mapping, creates both missing projections from the stored canonical packet rather than the retry body, advances projection version 0→1 atomically, and returns the established decision/Spark IDs plus input/stored digest match. If the adapter won first, later canonical calls reuse the mapped Intent without changing those projections. Preserve the installed JSON/human contract as far as possible and label legacy projections.
- Add safe-command classification only for exact read-only/basic leaves that meet the existing fail-closed policy; body-taking mutations remain operator-gated.

**Acceptance:**

- Failure injection after each write stage proves all-or-nothing behavior.
- Concurrent identical/reworded retries under one operation key converge on the first write and expose digest mismatch without duplication/default failure.
- Canonical-first→adapter, adapter-first→canonical, and concurrent mixed-entry-point fixtures converge on one Intent and at most one reciprocal journal/Spark pair. Version 0→1 projection backfill is all-or-nothing under failure injection, uses stored canonical content on digest mismatch, and every successful adapter response contains the established legacy IDs.
- Resume preserves the immutable deferred payload and derives `tracked`; resolve preserves all prior history.
- `intent create --disposition deferred`, `intent defer`, `journal defer`, and legacy conversion cannot create two canonical Intents for the same project/operation key; retrying through a different entry point returns the mapped first write and reports digest equality/mismatch.
- Commands reject unknown projects, invalid sources, cross-project endpoints, control characters, unbounded input, and ambiguous aliases with stable JSON errors.

**Rollback:** The compatibility adapter can be reverted before merge while additive rows remain harmless in isolated fixtures. Real dogfood is backup-first and rolls forward or invokes the existing quiesced whole-database restore procedure; conversion scope is project-local, but restore scope is not. Never delete rows ad hoc.

### `make-exploration-portable`

**Deliverable:** A deterministic `loaf exploration` resource surface whose context remains resumable without local session/log access.

**Implementation:**

- Implement create/list/show with stable Exploration identity and relationships; no open/active/paused/completed lifecycle.
- Implement immutable checkpoint append with required purpose/current framing, conclusions or constraints, unresolved question or decision, and recommended next action; trim and cap each at 4096 UTF-8 bytes, rejecting overflow without truncation or partial writes. Add optional individually bounded ordered candidates/evidence, source relationships, operation key, digest, and transactionally allocated Exploration sequence.
- Implement the focused deterministic conversation resource: `loaf conversation create`, `loaf conversation show/list`, `loaf conversation handle add`, and `loaf exploration conversation add`. A logical conversation may have multiple harness-local handles; an Exploration may span many logical conversations; no command infers equivalence from current session, branch, worktree, recency, or provider.
- Record handles/log locators with harness, locality/namespace, observation time, and optional hash/range. Reachability checks return ephemeral results or append separate timestamped observer/locality-scoped observations; do not infer equivalence from matching branch, worktree, timestamp, or opaque ID.
- Implement `exploration context` as a stable projection over latest checkpoint, ordered items, linked Intent/entities, related journal/wrap/handoff evidence, source conversations/handles/logs, and next question. Return the four-field portable core whole; bound each optional layer independently and report available/shown counts, truncation, a stable next cursor, and an exact `--layer <name> --cursor <cursor> --limit <n>` expansion command. Report `portable_context_present` separately from source-reference presence/availability.
- Make checkpoint writes concurrency-safe and deterministic without selecting a globally current Exploration.

**Acceptance:**

- Context remains complete and stable after every handle/log locator is marked unavailable or removed from the fixture machine.
- A handle-only Exploration reports `portable_context_present=false` and never claims resumability.
- Every optional context layer stays within configured bounds, paginates without omission/duplication under stable state, and returns deterministic cursors/counts; the portable core is never truncated.
- Multiple conversations/harnesses append checkpoints and source handles without overwriting or ordering ambiguity.
- Concurrent checkpoint appends receive distinct committed sequences, and context deterministically selects the greatest sequence even when timestamps are identical.
- A checkpoint missing any of the four portable fields is retained only as non-portable evidence or rejected according to the command contract; it never makes `portable_context_present=true`. With all source locators unavailable, the context fixture preserves the expected `recommended_next_action` byte-for-byte. Oversized core fields produce the documented stable error and leave no checkpoint/item rows.
- Unknown harness fields remain optional/absent; no ID or locator is fabricated.
- Raw transcripts, prompts, tool output, and provider payload bodies are absent from schema/API fixtures.

**Rollback:** Checkpoints and source records are additive. Before merge, revert against isolated fixtures; after dogfood, preserve facts and roll forward unless the existing quiesced whole-database restore procedure is explicitly selected.

### `connect-intake-and-trace`

**Deliverable:** Intent and Exploration behave as first-class relational state rather than isolated command silos.

**Initial relationship matrix:**

| From | Relationship | To | Required now |
|------|--------------|----|--------------|
| Spark, Idea, Brainstorm, journal entry | `source-of` | Intent | Preserve capture/provenance without automatic promotion |
| Intent | `derived-from`, `split-from`, `supersedes`, `duplicates` | Intent | Track candidate lineage without a program entity |
| Exploration | `explores`, `informs` | Intent | One inquiry may develop many tracked directions |
| Exploration | `has-conversation` | logical conversation | Associate explicitly without inferring provider identity |
| Exploration | `uses-source` | journal entry, handoff, report, finding | Connect portable context to bounded evidence |
| checkpoint | `belongs-to` | Exploration | Prefer a hard foreign key; expose as a trace edge if useful |
| report, finding, journal entry | `evidence-for` | Exploration or Intent | Retain conclusions without copying bodies |

Existing legacy relationships remain readable. Git Change paths, `materialized-as`, blocking/dependency semantics, Spec, Task, Plan, Council, Run, Verdict, and arbitrary cross-kind pairings are not required new-write combinations in this Change; future Changes extend the registry deliberately.

**Implementation:**

- Extend generic aliases/link/trace/show resolution through the centralized registry, including new checkpoint/conversation kinds where exposing them is useful.
- Implement the closed matrix above and preserve reasons; reject unsupported endpoint/type combinations rather than accepting a Cartesian product. Deferral linkage uses its dedicated foreign-key/event contract and may expose a derived trace edge without adding a free-form relationship row.
- Integrate new facts into export/import or backup/restore coverage, project deletion, state status/doctor, mapping-invariant scans, search/parity checks, and trace JSON.
- Add `loaf intake list` as a deterministic project-local projection of unresolved Sparks, Ideas, Brainstorms, unresolved Intents, deferred Intents, and unmigrated legacy deferrals. Deduplicate by `intent_operations` plus canonical relationships/migration markers without making a semantic decision.
- Update the layered journal continuity projection so unresolved deferred Intents receive active-truth precedence and recent portable Exploration checkpoints appear as a separately bounded/discoverable layer with counts, truncation, and exact context/expansion commands. Deduplicate converted legacy Spark projections.
- Return exact next read commands and source relationships; never rank, promote, publish, or choose a disposition in CLI code.

**Acceptance:**

- Link/trace round trips work for every pairing in the initial matrix and fail for unsupported, invalid, dangling, duplicate, or cross-project edges.
- Export/restore and project deletion preserve/remove every new row exactly; state health catches orphaned or parity-divergent records.
- Intake shows one logical item before and after legacy conversion and remains byte-deterministic for equal state.
- Context shows unresolved deferred Intent regardless of newer noise, exposes bounded checkpoint discovery without a current-Exploration pointer, and never equates handle presence with portable context.
- An inbound external tracker concept is absent from the data model/output except as a future generic mapping capability; no provider-specific fields appear.

**Rollback:** All projections are derived. Reverting a read surface must not mutate canonical facts; schema-backed write rollback follows the prior units' backup/forward-repair contract.

### `make-loaf-maintenance-config-aware`

**Deliverable:** One hidden config-aware Loaf maintenance protocol backed by deterministic, non-mutating plans for the project-alignment and installed-target surfaces changed by this release.

**Implementation:**

- Keep `loaf-reference` non-user-invocable. Expand its first-sentence routing and maintenance reference for natural-language requests to upgrade, diagnose, repair, configure, or bring a Loaf project current; do not add `/maintain` in this Change.
- Define the protocol as diagnose → plan → ask only for project-owned choices → apply approved operations → rerun diagnostics. It begins with `loaf config check --json`, combines returned project choices/actions with `loaf version`, installed-target ownership, `loaf state status/doctor --json`, and checkout alignment, and discovers exact current syntax through `loaf <command> --help` instead of memorizing command flags.
- Add read-only JSON output to `loaf doctor` with the same checks, repair identities, ownership/version facts, and pass/fail result as human output. JSON diagnosis never prompts or repairs; any later mutation still uses the existing explicit `--fix`/`--force` contract.
- Add `--dry-run` and `--json` to `loaf install --upgrade`. Planning uses the same target detection, ownership/content-digest checks, deprecation manifest, MCP/config analysis, and project-file reconciliation as apply, but records intended creates/updates/removals/preservations/conflicts without writing files, manifests, config, or state. The JSON response returns exact applicable follow-up commands and whether explicit consent is required.
- Reuse the existing doctor/install analyzers and ownership decisions for plan/apply parity. If a correct dry run requires replacing target ownership, config authority, or install architecture rather than extracting shared analysis from current behavior, stop and retain that redesign as deferred Intent; do not widen this unit silently.
- Ensure the maintenance plan understands the in-scope Intent/Exploration schema migration and new command/help/reference metadata without automatically applying a database migration. Behind-schema state continues to return the exact backup-first `loaf state migrate schema --apply` action.
- Keep executable acquisition outside Loaf: report the running version and executable provenance available from local facts, never invoke Homebrew/npm/another package manager, and never claim a newer remote version without evidence from the owning package manager.
- Update source help/reference metadata, `loaf-reference`, focused routing/content evaluations, every generated target, and installed-surface fixtures atomically. No hardcoded Loaf binary path may appear in the new maintenance protocol or generated commands.

**Acceptance:**

- Human and JSON doctor fixtures contain the same check identities and outcomes; JSON mode is byte-for-byte non-mutating and never enters prompt/repair code.
- Install-upgrade dry-run is deterministic and byte-for-byte non-mutating across recognized installed targets, absent targets, current/stale owned content, locally modified or unowned content, deprecation actions, project guidance effects, missing explicit cleanup consent, and a managed safe-command block whose retired version-pinned executable no longer exists while current `loaf` resolves on `PATH`. Applying the reported plan through existing commands produces the predicted effects, and rerunning diagnosis reports convergence.
- Routing chooses the hidden `loaf-reference` Skill for maintenance requests while ordinary workflow prompts continue to route to Triage/Explore/Shape/etc.; generated targets expose the same protocol and command contracts.
- `.agents/loaf.json` remains shared project configuration and gains no machine-specific installed-target or package-manager fields. No top-level `loaf upgrade`, package-manager mutation, fleet sweep, public `/maintain`, or duplicate config parser is introduced.

**Rollback:** Doctor/install planning changes, reference guidance, evaluations, and generated output revert as one unit. Rollback cannot delete or rewrite user files because the new planning paths are non-mutating and the existing apply ownership rules remain authoritative.

### `converge-explore-and-triage`

**Deliverable:** A smaller judgment-bearing Skill surface that consumes deterministic primitives consistently across targets.

**Implementation:**

- Add user-facing `/explore` guidance for divergent inquiry, checkpoint discipline, portable resumption, optional research/scout/prototype/spike techniques, and Intent/defer capture.
- Preserve the full Brainstorm questioning/divergence stance as an internal technique referenced by Explore. Hide it from the primary user workflow without deleting its behavioral value.
- Rewrite `/triage` around capture versus tracked Intent versus deferred disposition versus Exploration versus Change. It reads `loaf intake list` and chooses operations; it does not contain backend/provider branches.
- Narrow Idea and Spark to capture primitives triggered naturally during any conversation and routed through Triage. Neither owns shaping/promotion or tracker behavior.
- Consume the config-aware Loaf maintenance protocol from the preceding unit and update orchestration text only where the new primitives must be discoverable and accurately bounded; defer Plan/Implement/Ship/Release/Reflect/Housekeeping convergence.
- Add/adjust sidecars, routing prompts, hook instructions only where required, CLI reference/help generation, and all tracked `dist/`/`plugins/` output in the same unit.

**Acceptance:**

- Routing evaluation distinguishes “here is an idea,” incidental Spark, “explore this,” “resume this Exploration,” “track this,” “defer this body,” research, Shape, and Change materialization prompts.
- Current and generated in-scope Skills contain no direct provider/MCP calls, `integrations.linear.enabled` branches, branch/worktree shell recipes, or persisted lifecycle status instructions.
- `/explore` produces/checkpoints Exploration; `/triage` chooses disposition; neither writes durable Git artifacts or creates a Change.
- Target builds contain the same concepts and deterministic command contracts, with target-specific invocation metadata only.

**Rollback:** Skill/generation changes are atomic with the CLI they describe. Never leave source Skills ahead of generated/installed contracts or vice versa.

### `migrate-and-dogfood-intent-model`

**Deliverable:** Existing deferrals converge safely, real Loaf usage proves cross-context continuity, and durable documentation/effects match shipped behavior.

**Implementation:**

- Add a project-scoped state conversion command with dry-run/manifest/apply. Dry-run parses every current `journal_deferrals` pair, reports exact intended Intent/relationship/`intent_operations` rows and unparseable records in a project-specific manifest, and performs no writes.
- Apply verifies a whole-database backup, uses deterministic IDs and legacy operation keys, populates the same `intent_operations` mapping used by live commands, links each new deferred Intent to its historical journal decision/Spark/origin, records one migration marker, preserves legacy rows, and is rerunnable.
- Ensure pre-migration intake/context still exposes legacy deferrals so no item disappears while migration is pending.
- Dogfood one Exploration across at least two conversation/harness contexts, with multiple handles/log refs and loss/unavailability of one local source; resume solely from the portable checkpoint.
- Dogfood create/defer/retry/resume/resolve on Loaf state after explicit approval for global-state mutation; retain stable IDs and outcomes in journal/Change research rather than raw logs.
- Update architecture/schema/current knowledge from final code, reconcile the affected-artifact manifest with `git diff`, run the full gate matrix, and refresh immediate successor assumptions with observed behavior.

**Acceptance:**

- Dry-run is byte-for-byte non-mutating; apply is backup-first and leaves no partial conversion under failure injection.
- Every parseable legacy deferral maps exactly once; unparseable records remain visible and actionable without guessed semantics.
- Cross-context dogfood succeeds without access to the original local log and does not mutate unrelated project rows.
- Final documentation describes only proven behavior; historical artifacts remain historical and current guidance has no contradictory authority claims in scope.
- The full Verification Matrix below passes twice: once before approved real dogfood using isolated fixtures and once after final source/generated reconciliation.

**Rollback:** Preserve the verified whole-database pre-apply backup and project-specific conversion manifest. Prefer forward repair for additive rows; restore is a quiesced whole-database operator action and requires explicit approval. Never delete historical journal/Spark records to simulate rollback.

## Sequencing and Parallelism

`establish-relational-foundation` lands first because every later unit depends on one authoritative schema/registry. Intent and Exploration operations may then be implemented in parallel if they do not edit the same registry/trace files without coordination. Intake/trace waits for both. Config-aware maintenance follows the integrated doctor/status/command surface so its plans cover the actual release, and Skill convergence then consumes those stable JSON/human contracts. Migration/dogfood is last because it must exercise the final integrated system. This sequencing is a dependency contract, not mutable project status.

## Verification Matrix

Run focused tests with an absolute temporary `LOAF_DB` during implementation, then run the complete repository gates before review:

```bash
go test ./...
npm run typecheck
npm run test
npm run build
loaf check --hook artifact-body-write
loaf check --hook check-secrets
loaf check --hook render-drift
loaf change check --require-executable docs/changes/20260717-intent-exploration-foundation
git diff --check
```

Run every other registered enforcement hook affected by the final diff explicitly with `loaf check --hook <id>`; the CLI intentionally has no aggregate no-argument `loaf check` operation. Additional falsification suites must cover schema 9/10/11 upgrades, compound-write failure stages, cross-entry-point response-loss retries through `intent_operations`, bounded context pagination, unavailable provenance, cross-project/dangling relationships, export/restore/delete parity, legacy deferral conversion, doctor human/JSON parity, install-upgrade dry-run non-mutation and apply convergence, maintenance routing, and source-versus-generated Skill drift. Any bug fix discovered during implementation receives a regression test that fails when the fix is reverted and passes after restoration.

## Review Checkpoints

- Review schema/registry and command JSON contracts before broad Skill writing; these are the highest-likelihood-of-change surfaces.
- Review the `journal defer` compatibility semantics before real-state dogfood; compatibility must not become indefinite authority.
- Review one rendered `exploration context` from a fixture with unavailable local sources, then give it to a fresh agent without source logs and confirm that agent identifies the intended next action before accepting the human portability claim.
- Review a maintenance dry-run from an old installed-project fixture before apply, then confirm the reported plan matches the actual owned changes, preserved conflicts, required consent, state-migration action, and final converged diagnostics.
- Review source and every generated target together before calling the Skill surface converged.
- Review the final migration manifest and backup evidence before any apply against the global Loaf database.

## Plan Completion

The plan is complete when every unit's acceptance and rollback contract is implemented, the Change Verification Contract passes, dogfood evidence is retained, actual affected paths match `change.md`, and `change-native-execution-migration` can be materialized without reopening the Intent/Exploration authority model.
