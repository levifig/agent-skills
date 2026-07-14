---
change: journal-reliability-foundation
created: 2026-07-10
branch: journal-reliability-foundation
lineage: change-model-hard-cut
release-after: spec-conversion-and-guidance-sweep
---

<!-- Frontmatter must open the file at byte one — parsers depend on it. No status-like frontmatter (readiness/status/state): readiness is derived — a draft PR is shaping; `loaf change check` derives structural executability from the sections below. -->

# Journal Reliability Foundation — No Silent Loss Across Harnesses

## Lineage

- **Lineage:** `change-model-hard-cut`.
- **Position:** Root prerequisite; no predecessor.
- **Accepted order:** `journal-reliability-foundation` → `change-native-execution-migration` → `spec-conversion-and-guidance-sweep`.
- **Release gate:** A stable or normal-semver release whose ancestry contains any materialized node of `change-model-hard-cut` is refused until `spec-conversion-and-guidance-sweep` is also in that ancestry. Alpha/prerelease dogfood may advance an already-prerelease version only through an explicit `loaf release --bump prerelease` (or post-merge finalization of that prerelease). Releases cut from ancestry predating the first merged node are unaffected; `release-after` is immutable dependency metadata, not lifecycle state.
- **Activation rule:** One active Change branch/worktree at a time. The immediate successor remains a packet until this Change lands and its dogfood evidence is reconciled.
- **Canonical lineage seed:** `journal:00f302e40678b0f13b61f28c`, which supersedes the prose-keyed `journal:8255bef3bf2f5cd68a57fd6f` without rewriting history.
- **Parked terminal draft:** Commit `59fbcdcf` is valuable review evidence but not a lineage authority. This Change carries the durable successor packets; after explicit user approval, the parked branch should be pushed as a remote ref without opening a PR or worktree.

### Immediate successor packet — `change-native-execution-migration`

- **Purpose:** Add the Change-native execution and per-project migration path while the legacy public surface still exists, so the destructive hard cut consumes proven additive behavior rather than inventing it during removal.
- **Activation gate:** This Change is merged; its Verification Contract is freshly rerun against current main; project identity, journal retrieval, deferral capture, continuity projections, structural lineage checks, and every claimed target adapter pass real dogfood; deviations are reconciled into this packet before materialization.
- **Inherited decisions:** Changes remain git-canonical; no persistent task replacement is introduced; `implement` consumes executable Change slugs/paths; `ship` consumes the Verification Contract and durable-output obligations without auto-promoting ordinary decisions into ADRs; migration is deterministic in the CLI and judgment-guided through `bootstrap`; global snapshots are disaster recovery while rollback of one project's migration is project-scoped replay.
- **Required outputs:** `loaf migrate change-model` dry-run/manifest/apply/replay; additive Change-native `implement` and `ship`; Loaf's explicit legacy-work disposition inventory; the atomic journal-deferral primitive wired into `shape`, `ship`, and `triage`; no removal of public spec/task surfaces yet.
- **No-gos:** No automatic semantic disposition, no interactive CLI wizard, no new workflow entity, no mutation of other projects, no stable or normal-semver release, and no guidance sweep that makes the still-present legacy recovery path undiscoverable.
- **Fog to re-open:** Exact Loaf legacy dispositions, project-scoped replay collision semantics, additive coexistence hazards found by journal dogfood, and any target adapter that remained fallback-only.
- **Sources:** This Change's proven outputs; the bounded packet in parked commit `59fbcdcf`; journal decisions `journal:e76678700d531bdc14eb9836`, `journal:8a409ff660cb7b0a6f436eb5`, and `journal:8b85530c4ef3f60bea492292`.

### Terminal successor stub — `spec-conversion-and-guidance-sweep`

- **Purpose:** Hard-remove public spec/task/breakdown workflow, quarantine remaining legacy tables as migration-only input, converge guidance/build/install surfaces, and dogfood Loaf's own migration.
- **Activation gate:** Both predecessors are merged and unreleased; the parked draft is rebased, receives `predecessor: change-native-execution-migration`, replaces assumptions with landed evidence, and passes fresh critique plus `--require-executable`.
- **Inherited decisions:** Hard cut with no compatibility release; old projects migrate when next opened; quarantine is temporary internal input; historical evidence may retain legacy vocabulary; terminal stable release occurs only after live and generated guidance converge. Alpha/prerelease dogfood remains allowed through the explicit prerelease release path during the freeze. ADRs remain rare, narrow records for deep and durable architectural choices: prospective reasoning defaults to the Change/journal, greenfield speculation and single-Change implementation choices do not become ADRs, and an applicable existing ADR is evidence plus an explicit supersession obligation rather than a veto on an intentional refactor.
- **Required guidance convergence:** Align `shape`, `architecture`, `ship`, the Change template, and durable-document guidance around one high ADR threshold: a focused architectural choice, credible alternatives, long-lived/cross-Change consequence, rationale worth preserving, and low expectation of near-term churn. Shaping checks ADR applicability/status, may explicitly challenge one, and carries any proposed creation/deprecation/supersession as a reviewable output of the Change rather than blocking forward motion.
- **No-gos:** No public read-only compatibility commands, no global deletion while any project retains rows, no broad skill-quality audit, no transcript ingestion, no implementation from the stale parked snapshot without revalidation, no ADR-per-decision habit, no speculative greenfield ADRs, no treating an accepted ADR as immutable law, and no silently contradicting an applicable ADR instead of superseding or deprecating it explicitly.
- **Durable source:** The compact contract here plus accepted lineage journal entries. Commit `59fbcdcf` is a detailed working draft to reconcile, not the only copy of intent.

## Problem

The project journal is Loaf's portability spine: it should preserve enough semantic project depth that switching sessions, branches, worktrees, checkouts, or harnesses does not make the user start from a shallower or false understanding. The current journal has a sound append-only direction and meaningful initialized-project concurrency coverage, but it can silently fail the more important promise that an entry remains associated with the project, findable through every supported retrieval path, honestly summarized, and delivered to the next harness.

Adversarial scenarios found multiple silent-success failures. Moving a checkout without first running `loaf project move` causes a conceptual read to create a second project ID and show an empty journal while doctor reports healthy state. Removing one derived FTS row leaves the canonical entry intact but makes search return no result; doctor and backup verification still certify the divergent database. Context labels the newest wrap from any branch as project synthesis, returns only ten current-branch entries without saying more were omitted, and does not surface the accepted cross-branch lineage decision. The parked terminal Change passes `--require-executable` even though its predecessors do not exist because lineage metadata is currently decorative.

Capture and delivery are equally uneven. Manual entries commonly have no harness session ID despite guidance claiming automatic correlation. Missing-state automatic logging intentionally exits silently. Git and PR hooks infer completion from command text without requiring successful results or durable IDs. Before U8, the OpenCode plugin discarded stdout and the Amp `session.start` projection ran unrelated hooks without model context; U8 replaces those claims with an installed OpenCode request proof and an explicit Amp fallback. Before U8, Loaf installed no Codex lifecycle hooks even though the installed Codex runtime exposed stable SessionStart/UserPromptSubmit/Stop hooks. Equal event names are not equal outcomes.

Recovery language is also too broad. `VACUUM INTO` produces a consistent verified snapshot, but backups live beside the database by default, verification does not prove journal-search parity, Loaf has no global database restore command, and raw live file replacement is unsafe while another harness may retain an open connection. Process-kill atomicity is not the same claim as OS or power-loss durability, and the SQLite connection does not pin a synchronous mode.

The accepted hard-cut lineage cannot remove persistent task/spec execution until the journal earns trust across identity, storage, retrieval, semantic capture, continuity synthesis, recovery, lineage, installation, and real target delivery. Otherwise Loaf would replace imperfect explicit state with project memory that can be lost while reporting success.

## Hypothesis

If Loaf makes canonical-versus-derived state explicit, refuses unknown project identity instead of inventing it, detects and repairs retrieval divergence, pins and scopes durability claims, proves isolated recovery, stores self-sufficient semantic nuggets with honest optional provenance, gives active truth precedence over recency, enforces Change lineage, and delivers context through runtime-proven target-specific adapters or explicit fallbacks, then the journal can serve as trustworthy shared project memory. The additive execution/migration successor can then remove its dependency on legacy task/spec continuity without creating an invisible regression.

## Scope

**In**

- Define a falsifiable reliability vocabulary that separates atomicity, process termination, OS/power-loss policy, canonical persistence, derived retrieval, context completeness, capture, delivery, installation, local rollback, project-scoped replay, and external disaster recovery.
- Stop conceptual reads and ordinary journal writes from silently creating project identities. Make fresh registration explicit and atomic, preserve `loaf project move` as the relocation ceremony, and diagnose the exact old-path recovery action.
- Declare `journal_entries` canonical and `journal_search` derived; add bidirectional identity/content parity diagnostics, guarded dry-run/apply repair, post-bulk-operation rebuild, and retrieval-readiness checks in backup verification.
- Preserve WAL, foreign keys, busy timeout, one-connection writer serialization, and transactional journal/index writes; explicitly set and assert `synchronous=FULL`; retain process-kill and concurrent-writer falsification tests without overstating power-loss proof.
- Distinguish same-data-home rollback snapshots from explicit external disaster copies; support an absolute `--to` destination outside known OS-temporary roots, report the exact covered journal watermark, and provide an isolated restore-and-verify rehearsal that never replaces the live database.
- Deliberately do not add automated restore-to-live. Recovery activation remains a documented quiesced operator procedure; project-scoped replay in the next Change is the safe normal rollback path.
- Introduce a versioned, minimal journal-origin envelope: capture mechanism, observed harness/version when known, optional harness session/agent identity, branch, worktree, HEAD, current Change path/digest, dirty/reconstructable flags, source event, and durable result identity where applicable. Do not store raw prompts or tool output.
- Add idempotent atomic deferred-intent capture: one operation key creates or returns the same first-written self-sufficient journal decision plus spark projection, or creates neither. Retry wording drift is telemetry, not a duplicate-write error. The nugget remains useful when every origin pointer disappears.
- Replace the current digest with explicit project synthesis, a clearly labeled latest-scoped-checkpoint fallback when no `wrap(project)` exists, dedicated active-truth projections, bounded branch recency, omission metadata, stable cursors, and exact expansion commands. Active truth includes latest-per-key lineage decisions, unresolved blockers, deferred sparks, and active Changes; transitional open tasks may remain until the terminal cut but cannot be the continuity backbone.
- Validate new `unblock(<key>)` writes against an exact currently open `block(<key>)`; an unmatched key fails visibly for interactive writes and produces a visible nonblocking diagnostic for adapter-driven writes instead of silently leaving false active truth.
- Standardize lineage decisions as `decision(lineage/<key>): ...`; add a derived `loaf change list --lineage`; validate globally unique immutable Change slugs, non-root predecessors, lineage agreement, missing dependencies, self-reference, cycles, and multiple materialized children; make unmet ancestry and `release-after` constraints block execution/release without adding mutable status.
- Replace nominal cross-target hook projection with versioned target capability records and target-specific capture/delivery adapters. Prove model-visible behavior for supported installed modes or install an explicit CLI/managed-instruction fallback and label it non-automatic.
- Make routine Loaf state-plane work in Codex `Approve for me`/Auto mode without `danger-full-access` or a general Loaf data-directory writable root. Install an explicitly opted-in, Loaf-owned user-layer execpolicy policy generated from a central fail-closed basic/operator command classification; pin every allowed basic leaf to the trusted executable, preserve unrelated rules, and refuse ownership conflicts.
- Make automatic git/PR events result-aware and idempotent: failed or indeterminate commands emit no completion event; successful events resolve commit SHA or PR number before logging.
- Fingerprint managed instruction bodies and generated hook capability, compare source/build/install content rather than package version alone, and make doctor report artifact/install divergence plus the last explicitly run delivery smoke without implying universal observation.
- Run cross-harness dogfood that writes through one supported mechanism, resumes through another, retrieves the exact entry and origin, expands every truncated layer, exercises dirty deferral, and records bypass/fallback modes honestly.
- Reconcile README, ARCHITECTURE, journal/orchestration/shape/triage guidance, CLI reference, schemas, generated artifacts, and installed surfaces with behavior proven by this Change.

**Out** (deferred, not rejected)

- Change-native `implement`/`ship`, per-project legacy migration, Loaf's legacy-work dispositions, and integration of deferral capture into the final workflow — owned by `change-native-execution-migration` after this foundation lands.
- Removal of public `loaf spec`, `loaf task`, breakdown, legacy writers, generated guidance, and physical legacy schemas — owned by `spec-conversion-and-guidance-sweep` and the later zero-row schema cleanup.
- Migrating GridSight, mvault, dots, or any other project. This Change may use isolated copies/fixtures but mutates only Loaf's code, generated artifacts, isolated installs, and explicitly approved local dogfood configuration.
- Raw transcript ingestion, prompt mirroring, harness-log parsing, or guaranteed navigation into conversation JSONL. A later `journal-source-navigation` Change may use origin pointers if repeated recovery needs justify it.
- Automated cloud-provider backup, continuous replication, zero-RPO disaster recovery, cross-device synchronization, or automatic backup pruning/retention. This Change provides explicit external destinations, watermarks, and verified restore rehearsal.
- Safe concurrent in-place replacement of the global database. Global disaster activation remains a quiesced operator procedure; normal migration rollback remains project-scoped replay.
- A broad skill usefulness/routing audit, new skill router, or invocation-control redesign.
- Parallel/forking program management, tracker-native unit claiming, or a new lineage/program/session entity.

**Cut** (explicitly rejected)

- Silent fallback from an unknown checkout path to a fresh project ID or an empty continuity view.
- Returning false-empty search, false-complete context, a false successful commit/PR event, or a verified backup that has not passed journal retrieval checks.
- Calling a same-volume snapshot disaster recovery, calling a managed-instruction fallback automatic, or calling an adapter supported without installed-runtime evidence.
- Treating arbitrary `wrap(*)` recency as project synthesis.
- Requiring every journal entry to have a harness session ID or fabricating one when the runtime cannot supply it.
- Auto-committing dirty Change files, copying Change bodies into SQLite, or treating a content digest as reconstructable source.
- A generic session-hook abstraction that maps event names without mapping output, errors, identity, subagent semantics, and model-context delivery.
- A mutable lineage table, standalone program manifest, status flag, or persistent replacement task entity.
- Raw command output, prompts, transcripts, secrets, or provider-specific payload bodies in the origin envelope.
- Broad Codex filesystem grants for routine journal capture, a bare `loaf`/`loaf journal` execpolicy prefix, silent installation of an outside-sandbox command rule, or reliance on Auto-review as if it expanded sandbox access.

## Observable Workflow

### Continuity on a supported installed harness

```text
conversation starts
  → target adapter supplies normalized event envelope
  → loaf journal context computes:
      project synthesis: latest wrap(project), if any
      latest checkpoint fallback: newest scoped wrap, explicitly not project synthesis, only when synthesis is absent
      active truth: latest lineage decisions, unresolved blockers, deferred sparks, active Changes
      current branch: bounded recent entries
      transitional legacy: bounded open tasks, while they still exist
  → every layer states available/shown/truncated and its expansion command
  → adapter injects the digest into model-visible context
  → runtime smoke records evidence for this harness version and mode
```

If native injection is unavailable or disabled by a harness bypass mode, installed guidance says exactly when and how to run `loaf journal context`; Loaf labels that mechanism explicit/model-mediated, never automatic.

### Moved or unknown checkout

```text
$ loaf journal recent
error: project identity is not registered for <new-path>
       if this checkout moved, run:
       loaf project move --from <old-path> --to <new-path> --dry-run
       otherwise initialize it explicitly with:
       loaf state init
```

The read writes no rows. Applying `project move` preserves the original project ID and journal; simultaneous first initialization converges on one project and one path mapping.

### Search divergence and repair

```text
$ loaf journal search "needle"
error: journal search index diverges from canonical entries
       run `loaf state repair journal-search --dry-run`

$ loaf state repair journal-search --dry-run --json
{ "canonical_rows": 42, "index_rows": 41, "missing": 1, "extra": 0, "changed": 0, "applied": false }

$ loaf state repair journal-search --apply --json
{ "backup_verified": true, "rebuilt": 42, "parity_verified": true, "applied": true }
```

Repair is backup-first and deterministic. Retry is idempotent. Bulk import/merge paths rebuild and verify before reporting success.

### Backup and recovery rehearsal

```text
$ loaf state backup
local rollback snapshot: <data-home>/backups/...
covered through: journal:<id> at <timestamp>
device-loss protection: false

$ loaf state backup --to /absolute/external/path
external disaster copy: /absolute/external/path/...
covered through: journal:<id> at <timestamp>
device-loss protection: operator-selected non-temporary external destination

$ loaf state backup restore <backup> --to /absolute/empty/rehearsal/loaf.sqlite --json
{ "live_database_mutated": false, "integrity": "ok", "journal_retrieval_ready": true, "watermark_present": true }
```

The command proves that the artifact restores exactly into isolation. The rehearsal destination may be disposable and is labeled as such; it is never the backup or durable recovery artifact. Activating a global disaster copy is documented as a quiesced operator action and never presented as concurrent project rollback.

### Deferred intent during dirty shaping

```text
$ loaf journal defer "Preserve target-specific context injection; do not revive a generic hook projector" --change journal-reliability-foundation --operation-id <opaque-key>
created decision + spark
origin: branch, worktree, HEAD, Change path, content digest, dirty=true
snapshot reconstructable: false
```

Repeating the operation key returns the same first-written pair even if a retry rewords the nugget; output reports whether the retry digest matched so drift is visible without defeating response-loss recovery. The nugget contains the why, boundary, and trigger needed for later triage; origin enables investigation but is not required for comprehension.

### Lineage discovery and execution gate

```text
$ loaf change list --lineage change-model-hard-cut --json
root: journal-reliability-foundation
next packet: change-native-execution-migration
parked materialized node: spec-conversion-and-guidance-sweep
gap: predecessor change-native-execution-migration is not materialized
release after: spec-conversion-and-guidance-sweep

$ loaf change check --require-executable docs/changes/20260710-spec-conversion-and-guidance-sweep
error: predecessor change-native-execution-migration is missing from this lineage ancestry
```

Retained Change slug is identity; branch is provenance. The graph is derived from git-visible Change files and structured journal decisions.

## Rabbit Holes and No-Gos

- Do not equate trust with volume. The journal is curated semantic project memory, not an exhaustive activity feed; automation may only log proven durable outcomes.
- Do not solve moved checkout identity by guessing from repository name or remote URL. Explicit `project move` is safer than accidental cross-project fusion; a stable repo marker can be shaped later only if repeated relocation friction justifies it.
- Do not make every search perform an unbounded full-content comparison if a cheaper identity guard plus doctor/rebuild can prevent false success. The implementation must benchmark the invariant and preserve exact doctor verification.
- Do not turn active-truth projections into mutable status. Blockers resolve because the latest paired `block`/`unblock` key says so; lineage advances because retained git ancestry and the latest structured decision say so.
- Do not inject unlimited context. Dedicated projections are bounded; completeness is represented by counts, truncation, cursor, and expansion commands.
- Do not copy provider payloads into the database. Adapters normalize a small versioned envelope and discard raw prompt/tool bodies after deriving the allowed fields.
- Do not let target documentation outrun installed-runtime proof. Source docs justify a spike; only a real smoke justifies a supported capability claim.
- Do not introduce a third-party dependency for file locking, backups, hook normalization, or content hashing without reopening shaping and asking first.
- Do not expand isolated restore rehearsal into live concurrent restore. That requires a global mutation lease honored by every writer/version and is not necessary for the hard-cut migration's project-scoped replay.
- Do not split this foundation merely to create smaller labels. Identity, persistence, retrieval, capture, continuity, lineage, delivery, and install parity form one release-blocking guarantee; Implementation Units are review/commit boundaries. If a target adapter requires a new dependency or an unsupported runtime change, reopen this decision before implementation continues.

## Decisions

Provenance: Decisions 1–5 originate in the accepted 2026-07-10 journal-first and hard-cut conversations. Decisions 6–20 were selected after isolated adversarial scenarios, three independent read-only reviews, current source inspection, and installed Codex capability inspection; they remain reviewable through this Change and the final Claude critique.

1. **Journal reliability is a release gate, not best-effort polish.** The legacy execution model cannot be removed until every guarantee claimed by supported targets has direct evidence or an explicit fallback.
2. **The serial three-Change lineage remains.** One active branch/worktree avoids contradictory plans; the root carries successor packets so deferred nodes survive without premature worktrees or PRs.
3. **No stable release occurs between lineage nodes.** Once release ancestry contains the first materialized lineage node, release preflight freezes stable and normal-semver publication until that ancestry also contains the named terminal node; explicit `--bump prerelease` may advance an already-prerelease version for alpha/prerelease dogfood, and post-merge finalization may complete that prerelease. Ancestry predating the lineage remains unaffected, and `release-after` is immutable dependency metadata, not mutable lineage status.
4. **No persistent task/session/program entity replaces retired state.** Durable truth remains journal + retained Changes + git; CLI views are derived.
5. **Raw transcripts remain outside Loaf state.** Semantic nuggets and bounded origin pointers provide portability without mirroring sensitive/noisy harness logs.
6. **No silent success is the governing reliability rule.** Identity, capture, retrieval, context, automation, backup, restore, target delivery, and install checks must refuse or qualify uncertain results.
7. **`journal_entries` is canonical; search and context are derived.** Derived structures require parity diagnostics, deterministic repair, and explicit completeness metadata.
8. **Project identity creation is explicit.** Reads never create identity; ordinary writes require a registered path; fresh initialization and relocation are separate atomic ceremonies.
9. **Committed-write durability uses `synchronous=FULL`.** Journal volume is low and semantic value is high; measured latency is evidence to report, not a reason to inherit an unspecified default. Process-kill tests and SQLite policy support different claims and are documented separately.
10. **Recovery has three named tiers.** Same-data-home snapshots support local corruption rollback; explicit external copies support device/data-home loss to a reported watermark; later migration rollback is project-scoped replay. Concurrent live global restore is not promised.
11. **The nugget is self-sufficient; origin is evidence.** Dirty or absent source context never invalidates the semantic entry. Reconstructability is true only when committed content at HEAD matches the captured Change digest.
12. **Compound deferral is idempotent and atomic.** A caller-supplied or pre-generated operation key uniquely identifies the first-written journal/spark pair and survives post-commit response loss even when a fresh agent rewords the retry; semantic-digest mismatch is returned as telemetry, not a second write or default error.
13. **Active truth outranks recency.** Lineage, unresolved blockers, deferrals, and active Changes have dedicated latest-per-key slots; current-branch recency cannot evict them.
14. **Only `wrap(project)` is project synthesis.** Until one exists, the newest scoped wrap is shown in a separate `latest checkpoint` fallback labeled with its scope and explicitly marked non-project synthesis, preventing a first-run continuity regression without promoting it as holistic truth.
15. **Harness provenance is namespaced and optional.** Adapters record harness/version and session/agent IDs when provided; manual CLI entries remain valid without them, and documentation states the difference.
16. **Cross-target reliability uses adapters, not event-name projection.** Every target translates normalized events, output, warnings, and context through its real runtime API; unproven or bypassed modes use explicit fallback language.
17. **Automatic completion events require proven outcomes and proven capture fidelity.** Commit SHA and PR number are durable identities/idempotency keys; failed, no-op, or indeterminate commands do not create completion history. A target whose payload does not expose a trustworthy success/result signal gets no automatic completion capture.
18. **Lineage is structurally enforceable without becoming an entity.** Exact-scope journal decisions preserve evolving intent; retained Change frontmatter/ancestry defines materialized order; CLI checks/list/release preflight derive and validate the graph. Structural ancestry does not prove implementation completion, so successor activation separately reruns the predecessor Verification Contract against current main.
19. **Codex basic command policy is explicit and fail-closed.** The central registry classifies explicit basic leaf prefixes and defaults every unclassified or operator command to gated; an opt-in adds one managed user-layer execpolicy prefix per basic leaf for the canonical absolute Loaf executable. `journal log --execpolicy-safe` is the only basic journal leaf, while ordinary journal logging, body/file-consuming creation or import leaves, and path-taking `change check` remain operator-gated. Codex documents `allow` rules as exact command prefixes that run outside the sandbox without prompting, which is narrower than making the global Loaf data directory writable to every sandboxed process. Safe journal mode requires the registered current project, rejects `LOAF_DB` and caller-supplied worktree/branch/session overrides, and retains manual, `--from-hook`, `--detect-linear`, and JSON behavior. The installer pins the trusted executable in rendered prefixes and digest-owned `CODEX_HOME/AGENTS.md` guidance, never allows bare Loaf namespaces, never overwrites unowned or locally modified managed content, retires legacy journal-only ownership before broadening, and reports when executable resolution or installed ownership cannot be trusted. Other harness adapters are absent; the central registry enables future adapters without claiming them now.
20. **Codex lifecycle delivery uses a narrow current-schema adapter.** Codex `0.144.1` rejects Loaf's legacy top-level `version` and flat hook records, so the Codex build emits a current-schema `SessionStart` matcher group with both `command` and `commandWindows` placeholders. Install renders a trusted absolute executable: POSIX installs keep the exact two-field command shape and omit `commandWindows`, while Windows installs render both fields to the same canonical `cmd.exe /C` outer-wrapped command. U8 proves model-visible startup only on `darwin-arm64` in an isolated `CODEX_HOME`; `failClosed`, `blocking`, command conditions, global-home installation, resume, clear, compact, Windows runtime behavior, and completion remain separately unproven or unsupported. Install retires only recognized legacy flat Loaf records, preserves every recognized current-schema matcher group and top-level description, and refuses malformed, unknown, or unsupported user content under Loaf's conservative merge policy.

## Planning Contract

### Reliability invariant matrix

| Surface | Canonical fact | Required refusal/qualification | Proof |
|---------|----------------|--------------------------------|-------|
| Project identity | Registered `project_paths` mapping to one durable project ID | Unknown path must not create or return an empty project | Move/unknown-path fixtures and concurrent first-init process test |
| Journal persistence | Committed `journal_entries` row | No success before base + derived write transaction commits | Transaction, contention, and SIGKILL tests |
| Search | Canonical entry plus parity-valid derived FTS row | Divergence blocks search success and backup retrieval-ready claim | Delete/change/extra-row fixtures, repair, and exact parity |
| Context | Explicit project synthesis plus dedicated active projections and bounded recency | Every bounded layer discloses omissions and expansion | Multi-branch noise fixtures and JSON contract tests |
| Capture | Self-sufficient semantic nugget | Missing provenance is explicit; dirty source is never called reconstructable | Manual/hook/dirty-worktree/idempotency fixtures |
| Automation | Proven successful durable result | Failed/unknown result emits no completion event | Git/PR falsification matrix |
| Backup | Consistent snapshot with retrieval parity and watermark | Same-volume copy reports device-loss protection false | Concurrent snapshot, corruption, and verify tests |
| Restore | Exact isolated restored database | Live database remains untouched; global activation requires quiescence | Restore rehearsal byte/table/watermark comparison |
| Harness delivery | Model-visible context or explicit fallback for a tested version/mode | Generated-but-unseen output is unsupported | Target adapter unit tests and installed runtime smokes |
| Installation | Content-addressed source/build/install equivalence | Equal version with unequal body is drift | Isolated-home and real-install digest checks |
| Lineage | Structured journal intent plus retained Change graph | Invalid/missing ancestry blocks execution/release | Cycle/fork/missing/predecessor/release fixtures |

### Project identity boundary

Refactor store opening into explicit intents: inspect/read-existing, mutate-existing, and initialize/register. `journal recent`, `journal search`, `journal context`, doctor, and ordinary journal writes resolve a registered project path and never call a helper that can insert a project. `state init` owns first creation; `project move` owns path relocation. First registration performs path lookup, project creation, current-path selection, and conflict re-read in one transaction so simultaneous harnesses converge on one project ID.

Unknown-path diagnostics compare registered paths only to present actionable candidates; they do not automatically attach based on repository name, Git remote, or basename. JSON errors carry a stable code, current path, and suggested commands. Existing aliases and project IDs remain unchanged.

### Canonical journal and repairable projections

Journal writes keep the base row and FTS row in one transaction. `state doctor` compares both directions and validates identity plus indexed content, not only counts. `journal search` runs a bounded parity guard sufficient to prevent known divergence from returning a trusted result; any detected mismatch returns a repair-required error instead of a false empty result. `state repair journal-search --dry-run` reports canonical/index/missing/extra/changed counts; `--apply` creates and verifies a backup, rebuilds entirely from canonical entries inside a transaction, and proves exact parity before success.

Every bulk operation that copies/imports/deletes journal rows either maintains FTS in the same transaction or rebuilds and verifies it before returning. Backup verification reports `sqlite_valid` and `journal_retrieval_ready` separately. A backup may be structurally valid but is not migration/recovery-ready until both are true.

### Durability and recovery tiers

Both read-write connection paths pin WAL, foreign keys, busy timeout, and `synchronous=FULL`; tests query the pragmas through Loaf's own connection because busy timeout and foreign keys are connection-local. Existing max-open-connection serialization remains unless measured evidence shows a correctness issue. Verification distinguishes ordinary concurrent writers, abrupt process termination, and SQLite's configured OS/power-loss policy; no test result is generalized beyond its tier.

`state backup` keeps the current local default but labels it `local_rollback` and reports `device_loss_protected: false`. `--to <absolute-directory>` creates an operator-selected `external_disaster_copy`; it resolves symlinks and rejects destinations under `os.TempDir()` plus platform-known volatile roots such as `/tmp`, `/private/tmp`, and `/var/tmp`. Passing that path check does not prove the destination is physically remote or durable, so Loaf reports the selected destination and never calls it off-device without evidence. Every backup records schema, integrity, foreign-key result, retrieval parity, checksum, project identities, and the latest included canonical journal ID/timestamp watermark.

`state backup restore <backup> --to <absolute-empty-database-path>` copies/restores into isolation, opens it read-only, verifies all invariants, proves the watermark exists, and never mutates the live database. The destination is an explicitly disposable rehearsal output and cannot become the sole recovery copy; durable evidence of the rehearsal is the verification result recorded in the journal/repository, not the restored temp file. Documentation gives a quiesced activation procedure with preserve-current-first steps. The later migration Change must use project-scoped replay rather than global snapshot replacement for normal rollback.

### Origin envelope and deferred-intent transaction

Add a versioned minimal origin representation associated with journal entries. The logical fields are capture mechanism (`manual`, `skill`, `hook`, `migration`), observed harness and version, optional harness session and agent IDs, source event, branch, worktree, HEAD, Change path, Change content digest, dirty flag, reconstructable flag, and durable result identity. The implementation may use columns or a normalized child table, but must keep indexed retrieval simple, preserve unknown future fields safely, and avoid raw payload storage.

Each target adapter produces one normalized event envelope before calling journal/context code: contract version, harness/version, event, session/agent identity when known, worktree, success state, command family, durable result identity, and a bounded diagnostic code. The CLI derives allowed journal fields then discards raw input. Unknown fields degrade without fabrication.

`loaf journal defer <nugget> --operation-id <key> [--change <slug|path>]` validates a self-sufficient packet containing intent, why it matters, the boundary that deferred it, and a future trigger/next question. The operation key is unique per project. One transaction writes the decision and spark projection with reciprocal stable IDs; first write wins, and every retry returns that pair. The response reports `input_digest_matches` plus the stored digest so rewording or accidental key reuse is visible, but response-loss recovery never creates a duplicate or fails by default. When a Change is dirty, the origin records the digest and `reconstructable=false`; no source pointer relaxes the nugget requirement.

### Active-truth continuity model

Replace `LatestWrap`, unqualified `BranchEntries`, and unbounded-looking task output with named layers. The JSON contract for every layer includes `available_count`, `shown_count`, `truncated`, `cursor` when more exists, and `expand_command`. Human output says `showing N of M`; an empty layer says whether none exist or the source is unavailable.

The layers and precedence are:

1. **Project synthesis:** latest `wrap(project)` only; when absent, render a separate latest-scoped-checkpoint fallback with scope and an explicit `not project synthesis` label.
2. **Active lineage:** latest exact-scope `decision(lineage/<key>)` for each lineage relevant to an active/retained Change.
3. **Unresolved blockers:** latest paired `block(<key>)`/`unblock(<key>)` outcomes, with source IDs and an expansion command; new unmatched unblocks fail or diagnose visibly rather than pretending to resolve another key.
4. **Deferred intent:** unresolved spark projections created by journal deferral, with source Change/origin summary.
5. **Active Changes:** git-derived Change identity, branch provenance, predecessor/release gaps, and current-branch match.
6. **Current branch recency:** bounded chronological entries deduplicated against higher layers by journal ID.
7. **Transitional legacy tasks:** bounded and explicitly marked temporary until the terminal cut removes them.

No layer competes with another for one shared limit. Retrieval commands accept stable cursors or explicit limits so a user/agent can expand without reconstructing query semantics. Context remains ephemeral; it is never written back as another journal entry.

### Enforceable Change lineage without a new entity

Materialized Change identity is the globally unique slug, not the dated folder or current branch. `loaf change init` rejects any slug already present under `docs/changes/*-<slug>`. A root may omit `predecessor`; every non-root materialized Change in a lineage must name exactly one predecessor. `change check` validates self-reference, global uniqueness, predecessor existence when materialized, same-lineage membership, cycles, and at most one materialized child. Alternative/forked work uses a new lineage key citing the same source decision rather than overloading a serial chain.

Normal `change check` may report a not-yet-materialized predecessor as a shaping gap, but `--require-executable` fails until the predecessor Change is present in the current git ancestry and structurally valid. This is a materialization/structure guarantee, not proof that predecessor behavior still works. Before a successor is materialized, its activation gate reruns the predecessor Verification Contract and dogfood against current main so a revert or partial landing cannot hide behind the retained plan file. `loaf change list --lineage <key> --json` scans retained Changes plus latest exact-scope lineage decisions, shows packets/gaps without persisting them, and remains usable after branch deletion/merge. `/release` refuses stable and normal-semver publication when ancestry contains any node of a lineage but not that lineage's named `release-after` terminal; an explicit `--bump prerelease` may advance an already-prerelease version for dogfood, and post-merge finalization may complete that prerelease; ancestry before the first node is unaffected.

Append-only lineage decisions supersede earlier entries through exact scope and recency. Abandoning an unmerged node requires a newer `decision(lineage/<key>)` that explains the replacement; retained historical Changes are never silently renamed to repair a graph.

### Target capability and adapter plan

| Target | Current evidence | Completion success/result signal | Planned mechanism | Fallback |
|--------|------------------|----------------------------------|-------------------|----------|
| Claude Code | Exact CLI 2.1.207 startup is model-visible in the retained plugin-dir smoke; resume, clear, and compact remain candidate fixture modes | Unknown until real PostToolUse payload fixtures prove exit/result semantics | Native SessionStart delivery; startup is the only exact-version model-visible proof and compact continuity is `SessionStart:compact`, not `PostCompact` | Managed instruction to run `loaf journal context`; explicit CLI; no automatic completion when unproven |
| Cursor | Native hook projection exists and is closest to Claude semantics; real installed smoke still required | Unknown until target payload/runtime smoke | Target payload adapter plus native context delivery where model-visible; completion capture only with proven signal | Managed instruction plus explicit CLI; no automatic completion when unproven |
| Codex | Installed `codex-cli 0.144.1` reports stable hooks, accepts SessionStart/UserPromptSubmit/Stop, and parses user-layer execpolicy rules whose exact `allow` prefixes run outside the workspace sandbox without prompting; the isolated `CODEX_HOME` startup smoke on `darwin-arm64` observes exact native `SessionStart` `additionalContext` and the model returns its random marker exactly; global home, resume, clear, compact, and Windows runtime remain candidate | Unknown; SessionStart capability does not imply PostToolUse result fidelity | Install the current-schema SessionStart adapter with a trusted absolute executable rendered at install, retain hardened journal guidance, and classify compaction and completion separately | Managed skill/instruction to run `loaf journal context` plus explicit CLI; global-home, Windows, and unproven lifecycle modes remain gated; body/file-consuming and path-taking leaves remain gated; no automatic completion when unproven; fail visibly when the managed policy is absent, conflicted, or bypassed |
| OpenCode | CLI 1.17.18 isolated-XDG request smoke proves the exact marker is model-visible through `experimental.chat.system.transform` `output.system`; resume and compact remain candidates, and startup has no distinct native signal | Bash exit status is source-proven; operation success and durable identity remain unproven, so completion is disabled | Use per-request `experimental.chat.system.transform` `output.system` and `experimental.session.compacting` `output.context`; root lookup without `parentID` fails closed while child/background classification uses `parentID` | Managed instruction plus explicit CLI for bypasses; no automatic completion; bypass missing/disabled plugin, attached/remote server without candidate plugin, missing state, untested version |
| Amp | `tool.call`/`tool.result` are available, but no root identity is proven; `agent.start` is only a foreground-turn candidate because root-only identity cannot be proven; startup/resume/compact have no distinct native signals | Generic done/error/cancelled is source-proven, but durable operation success, identity, and child suppression are unproven, so completion is disabled | No automatic context; keep the `agent.start` foreground-turn candidate only as classified evidence and disable policies where foreground children are indistinguishable; candidate plugin paths are `XDG_CONFIG_HOME/amp/plugins` or `HOME/.config/amp/plugins` | Honest managed instruction plus explicit `loaf journal context` and CLI fallback; bypass missing/disabled plugin, legacy-only `~/.amp` installation, missing state, or untested version; legacy-path retirement belongs to U9; no automatic completion |

Capability records name tested harness version, installed mode, adapter, model-visible effect, success/result signal fidelity, provenance fields, bypass modes, and smoke command/evidence. Context delivery and automatic completion are independent capabilities: a target may support one and not the other. A source API claim is a spike input; a capability becomes supported only after installed runtime proof. `--bare`, safe/pure modes, disabled customizations, missing Loaf state, and subagent/background modes are separately classified.

### Result-aware automatic events

The normalized hook envelope carries success/exit/result metadata only when the target can prove it. Git commit capture verifies command success, resolves the post-command SHA, distinguishes new commit/amend/no-op/failure, and uses SHA as the operation key. PR create/merge capture requires success and resolves the actual PR number/URL/state through command output or a bounded query; unknown identifiers are diagnostics, not completed events. A target with absent or unknown success/result fidelity emits no automatic completion entry. Repeated delivery returns the existing event. Missing initialized state stays non-blocking but emits a structured warning that the adapter must route to a visible channel.

Subagents do not emit project journal churn by default, but suppression is based on each target's normalized agent identity rather than Claude-only `agent_id`. A parent/orchestrator that consumes material subagent findings logs the semantic conclusion at handback; raw subagent traces remain outside the journal.

### Content-addressed install and doctor convergence

Managed fences include a content fingerprint independent of package version. Build manifests record target capability/adapters and hashes for managed instructions/hooks/plugins. Codex's optional Auto-journal rule has its own per-file ownership and digest manifest under `CODEX_HOME`; a root version marker never authorizes overwriting user rules. Install compares desired and installed fingerprints even when the version marker matches, removes stale managed projections, and reports user-owned conflicts without overwriting them.

Doctor compares source-to-build when run in the Loaf repository and build-to-install for detected targets. It reports capability mismatch, stale managed body, missing adapter, unsupported installed version, and absent/expired smoke evidence as separate diagnostics. Smoke evidence is explicitly invoked and timestamped local evidence, not passive proof that every session worked.

### Sequencing and rollback

Implement the lineage execution guard first so the already-materialized terminal Change stops passing executability before other work begins. Land identity and canonical/index guarantees before enriching journal schema. Add backup/recovery before any repair or origin migration can mutate global state. Build semantic capture/context before target delivery so every adapter injects the final bounded contract. Add content-addressed install before real cross-target dogfood so tested artifacts are provably current.

Every schema migration is backup-first, idempotent, and tested from the current production schema plus pre-journal-first fixtures. No implementation probe uses the global database unless it is an intentional read-only inspection or explicitly approved dogfood; automated and adversarial tests use an absolute temporary `LOAF_DB` whose database and worktree fixtures are disposable and never carry the only copy of evidence or intent. Results worth retaining are written into the repository Change/research artifact or project journal before cleanup. Rollback before merge is code/schema-test reversal; recovery after a dogfood migration uses the verified snapshot or a targeted forward repair, never ad hoc deletion.

## Implementation Units

- **U1 — Lineage execution guard and derived view.** Add global slug identity, predecessor/lineage/release validation, `change list --lineage`, exact-scope decision parsing, invalid-graph fixtures, and a parked-terminal regression proving it is non-executable until refreshed over both predecessors.
- **U2 — Explicit project identity continuity.** Split read/mutate/init store intents, remove implicit project creation from journal reads/writes, make first registration transactional and race-safe, preserve project move, and add actionable human/JSON diagnostics plus moved-checkout/concurrent-init tests.
- **U3 — Canonical journal/index integrity and durability.** Pin `synchronous=FULL`, preserve WAL/busy-timeout/foreign-key invariants, add bidirectional FTS parity, guarded search refusal, backup-first journal-search repair, bulk-operation rebuild hooks, and contention/SIGKILL/tiered durability tests.
- **U4 — Recovery tiers and rehearsal.** Add backup classification, absolute external destination, journal watermark, retrieval-ready verification, isolated restore command, exact restored-state comparison, and quiesced activation documentation without claiming concurrent live restore.
- **U5 — Honest origin envelope and atomic deferral.** Migrate/store normalized origin, add per-project operation-key uniqueness, implement atomic journal/spark defer with semantic retry checks, capture dirty/reconstructable Change evidence, and keep manual provenance optional.
- **U6 — Active-truth context query.** Implement explicit `wrap(project)`, labeled latest-scoped-checkpoint fallback, lineage/blocker/deferral/Change projections, unmatched-unblock validation, per-layer counts/cursors/expansion, deduplication, bounded transitional tasks, and noisy multi-branch tests that prove active facts cannot be evicted.
- **U6A — Least-privilege Codex basic-command capability.** Classify explicit Loaf leaves as basic or operator with unknown leaves failing closed; keep mixed read/write leaves operator-only; retain hardened `journal log --execpolicy-safe`; render one exact absolute-executable Codex allow rule per basic leaf plus matching global guidance; install only through explicit consent with per-body ownership and conflict-safe upgrades/retirement; replace the rejected legacy Codex hook projection with an explicit current-schema surface for U8's target adapter; and prove permitted/non-permitted command classification without widening writable roots.
- **U7 — Normalized hook envelope and result-aware automation.** Normalize target events, success and identity; record success/result-signal fidelity per adapter; prevent false git/PR completion entries or disable automatic completion when unproven; make warnings visible/non-blocking; implement per-target subagent semantics and parent handback guidance.
- **U8 — Target-specific delivery adapters.** Prove Claude/Cursor behavior, implement the Codex SessionStart adapter with isolated startup smoke evidence or retain explicit fallback, correct OpenCode request/compaction delivery, classify Amp's foreground-turn candidate while retaining an honest explicit-context fallback, and record versioned supported/bypass capability evidence.
- **U9 — Content-addressed install, documentation, and cross-harness dogfood.** Add managed fingerprints/manifests/doctor convergence, rebuild every target, test isolated upgrades and the real installed surfaces with approval, run the write/defer/resume/expand/recovery matrix, reconcile durable docs/spec deltas, and feed deviations into the immediate successor packet.

## Implementation Evidence

- **Committed checkpoints:** U1 lineage execution guard: `b7f4d556`; U2 explicit project identity: `2d7c352e`; U3 journal/index integrity and durability: `31993521`; U4 recovery tiers and rehearsal: `803366f4`; U5 honest origin and atomic deferral: `ce0af7d0`; U6 active project truth: `3308d98f`.
- **U5 implemented surfaces:** schema migration 11 introduces normalized journal origins and deferrals; explicit backup-first schema upgrade replaces implicit mutation on normal opens; validation-only diagnostic opens preserve inspectability of schema-current divergent project state; provenance integrity contributes to recovery readiness; `journal defer` atomically writes its decision/spark pair with operation-key retry convergence; local Change evidence and manual local Git origins are captured without fabricating unavailable fields; import, storage merge, export, project delete, and recovery preserve the new provenance/deferral records.
- **U6 implemented surfaces:** `journal context` now returns a contract-v2 active-truth snapshot with exact `wrap(project)` synthesis, labeled scoped fallback, retained-lineage decisions, unresolved blockers, open deferrals with origin, Git-derived active Changes, deduplicated branch recency, and transitional tasks. Every layer exposes source availability, counts, truncation, cursor, and an exact expansion command; cursors bind project, layer, branch/filter, snapshot, and limit. Exact unmatched-unblock validation is atomic and adapter-driven failures remain visible and nonblocking.
- **U6 adversarial evidence:** noisy multi-branch fixtures prove active truth is not evicted; exact lineage matching excludes prefix collisions and retains both HEAD and dirty-worktree lineage keys; empty retained-key sets leave unrelated lineage decisions in recency; task and linked-spec alias fanout cannot duplicate cursor items; nested-directory invocation resolves Change discovery from the registered project root; active-Change limit changes and stale snapshots reject cursor replay. Focused concurrency and cursor suites passed at count 10 and under `-race` count 3, and the final Sol review returned `APPROVE` with no findings.
- **U6A implemented surfaces:** `journal log --execpolicy-safe` is the only basic journal leaf; ordinary `journal log`, body/file-consuming creation/import leaves, and path-taking `change check` remain operator-gated. The command limits outside-sandbox writes to the registered current project and rejects `LOAF_DB` plus caller-supplied worktree, branch, and harness identity. `loaf install --to codex --codex-basic-commands` renders one canonical absolute-executable prefix per centrally classified basic leaf plus matching digest-owned `CODEX_HOME/AGENTS.md` guidance, preserves unrelated rules/guidance, retires legacy journal-only guidance safely, and uses rule-first, independently owned upgrade/retirement. U8 now adds a current-schema SessionStart context adapter; Codex enforcement semantics and other harness adapters remain separately classified.
- **U6A adversarial evidence:** installed Codex `0.144.1` classified the current canonical basic prefixes as allowed while bare/alternate executables, ordinary journal logging, body/file-consuming and arbitrary-Change-path leaves, unknown export/report kinds, other unclassified/operator leaves, wrappers, and environment redirection remained unmatched. The integrated suite and focused current-policy tests passed, including repeated installed-execpolicy classification, conflict/ownership handling, and legacy journal-only retirement without consent broadening. An earlier, now superseded U6A isolated `codex exec` with Luna, `workspace-write`, the same hardened journal prefix, and the then-generated empty hooks file wrote and retrieved `journal:b07996f688054ca5ddf0f503`; removing its rule permission-blocked the same write. That historical dogfood proves the hardened writer's outside-sandbox behavior, not every newly classified basic leaf; U9 still owns fresh full-policy installed dogfood. The first Sol review found ordinary-log bypass, arbitrary body-file reads, family-prefix inheritance, legacy-consent broadening, and stale documentation; all were remediated, and two focused follow-up reviews returned `APPROVE` with no findings.
- **U7 implemented surfaces:** `internal/cli/journal_hook_envelope.go` normalizes a version-1 envelope with explicit outcome classes (`unknown`, `failed`, `no-op`, `created`, `amended`, `merged`, `completed`) and family-compatible durable identities for git commits, PR creation/merge, and task completion; trusted capture requires explicit success, the compatible outcome, and the durable key, while real target capture remains unsupported and the automatic git/PR/task registrations are removed pending U8.
- **U7 hook and replay evidence:** hook origins retain normalized harness/version/session/agent/event/worktree values, recognized target namespaces isolate identity fields, lifecycle hooks suppress normalized subagents, safe `--execpolicy-safe --from-hook` paths preflight registered project identity, and transactional replay returns one canonical entry for repeated delivery with distinct `pull-request:create`, `pull-request:merge`, and `task-completion` keys. Malformed payloads and missing initialized state produce structured nonblocking diagnostics; the Claude Code, Cursor, Codex, OpenCode, and Amp fixtures each persisted one namespaced origin and reloaded it through the state API without raw or unknown-field fabrication.
- **U7 verification and review:** focused `go test ./internal/cli -run 'TestJournalHook|TestNormalizeJournalHook|TestJournalLogExecpolicySafe' -count=1`, full `go test ./internal/cli ./internal/state -count=1`, and durable replay `go test ./internal/state -run TestLogJournalConcurrentDurableResultReplay -count=10 -race` passed; `npm run typecheck`, `npm run build`, and `git diff --check` passed. Sol/high final review returned `APPROVE` after two blocker-remediation passes, with no U7 findings remaining.
- **U8 evidence:** The version-3 target-capability registry records exact platform-scoped identities, per-mode triggers, retained source evidence, and explicit candidate/unsupported boundaries across the build targets. Claude Code 2.1.207 startup, Codex CLI 0.144.1 startup on `darwin-arm64` in an isolated `CODEX_HOME`, and OpenCode CLI 1.17.18 request delivery in isolated XDG now have retained model-visible smoke proof with strict receipts and current artifact hashes. OpenCode request is supported through `experimental.chat.system.transform` `output.system`; startup has no distinct native signal, resume is a transform candidate after reopen, compact is a compaction candidate, and completion remains disabled because operation success and durable identity are unproven. Cursor IDE 3.11.19 and cursor-agent 2026.05.09-0afadcc remain separate exact-version candidate records handled by one fixed `cursor-session-start-v1` adapter; focused tests cover complete active-truth delivery, native background suppression, and generated-hook parity, but both remain candidate because the safe cursor-agent preflight found no `--no-session-persistence` boundary and the IDE was not independently smoked. Amp has no automatic context claim: `agent.start` is a foreground-turn candidate whose exact reason is that root-only identity cannot be proven, while startup/resume/compact and completion are unsupported with the managed explicit-context fallback; candidate plugin paths are `XDG_CONFIG_HOME/amp/plugins` or `HOME/.config/amp/plugins`, and legacy `~/.amp` retirement belongs to U9. Pi remains deferred and is not a build target. U8 may close with this honest Amp fallback; any newly discovered target mode remains unsupported until classified.
- **U8 adapter and adversarial evidence:** OpenCode gates model-visible request/compaction delivery and the `detect-linear-magic` capture hook on a fail-closed root-session lookup; enforcement and unrelated post-tool hooks remain active. Amp preserves documented `tool.call`/`tool.result` enforcement behavior but omits only `detect-linear-magic` because its foreground events cannot distinguish root work from child work; automatic context also stays disabled. Shared workflow guidance now makes startup, compaction, and resumption claims conditional on the exact target-mode capability and directs candidate/unsupported modes to explicit `loaf journal context`. Strict receipt validators bind Claude and Codex native paths to the recorded platform and target root; matching binary bytes and hashes at platform-swapped or cross-target paths are rejected. Registry tests separately reject stale versions, malformed receipts, missing artifacts, hash drift, unsupported bypasses, and candidate evidence presented as support.
- **U8 verification and review:** `LOAF_VALIDATE_TYPESCRIPT=1 npm run build` passed all targets and synchronized generated Go artifacts. Fresh retained smokes passed for Claude startup, Codex startup, and OpenCode request delivery; the OpenCode receipt additionally proves plugin load, root-session lookup, no supplied auth, and disposable-state cleanup. The strict capability suite, 19 Node smoke-parser tests, `npm run test`, `npm run typecheck`, executable Change check, render-drift, ephemeral-provenance, check-secrets, validate-commit, security-audit, and `git diff --check` passed. The parity regression test was falsified by the former unconditional Amp-hook expectation and now requires the unsupported capture hook to remain absent while preserving every enforcement hook. Initial final reviews found the omitted Amp missing/disabled-plugin bypass and substitution-damaged generated guidance; after correcting both and sweeping the U8-touched orchestration sources, final Sol/high and Claude Opus verification returned `APPROVE` with no blocking findings. Claude separately noted pre-existing plural-substitution defects in unrelated housekeeping, wrap, and foundations guidance; U9's documentation reconciliation retains that boundary.
- **U9 content-ownership primitive:** managed fences use version-independent body fingerprints with legacy migration, tamper refusal, atomic user-prose preservation, and strict fence parsing; the shared skill manifest v2 records deterministic per-tree digests with v1 migration, stale/missing convergence, foreign/tamper/race refusal, and rollback, covered by mode, symlink, strict-manifest, and fence-parser tests. Rebuilding changed the candidate native binary hash, so the strict capability suite failed closed on stale receipts; fresh isolated Claude startup, Codex startup, and OpenCode request smokes passed, refreshed retained hashes, and integrated tests passed afterward.
- **U9 target-adapter manifest slice:** every native build target now emits one deterministic strict manifest whose adapter identities come from the validated version-3 capability evidence and whose managed-instruction digest is version-independent. Concrete hook/plugin artifacts bind content plus permission mode, while merged user hook projections retain their existing mode; malformed metadata, source or installed content/mode drift, unsafe paths, symlinks, duplicate keys, and trailing JSON fail closed. OpenCode, Cursor, Codex, and Amp installs persist strict ownership state, converge same-version changes, migrate recognized legacy surfaces, retire unchanged stale artifacts, preserve foreign content, reject unowned or modified destinations, revalidate each destination immediately before mutation, and roll back only actual mutations in reverse order with restoration failures surfaced alongside the primary error and current recovery state. Claude Code receives the build manifest but remains intentionally outside CLI install ownership because the CLI has no Claude installer. Focused manifest/install tests, the full internal CLI suite, `go vet ./...`, formatting, and diff checks passed; this slice does not complete V12 or U9.
- **U9 installed-ownership doctor slice:** `loaf doctor` now runs one read-only, non-fixable aggregate ownership check for recorded or detected OpenCode, Cursor, Codex, and Amp installs. It reads each recorded configuration root as authoritative, requires the strict installed manifest, skips the managed-instruction pseudo-artifact, and validates every declared hook, plugin, or merged projection through safe destination resolution plus exact projection/content digest and concrete permission-mode ownership; malformed or symlinked manifests, unsafe or symlinked artifact paths, missing artifacts, foreign Loaf-projection drift, content drift, and mode drift fail closed. Missing records for detected legacy installs, missing manifests, malformed records or manifests, wrong-target manifests, and record/manifest/trusted executable-distribution version disagreement are reported deterministically alongside other target and artifact defects. The check has no fix function, and `doctor --fix` leaves target bytes and permission modes unchanged. If the executable distribution root cannot be proven, doctor retains legacy fence behavior but makes no current-version ownership claim from an unrelated project package. Focused doctor tests, formatting, and diff checks passed; this slice does not implement source-to-build comparison, capability or smoke diagnostics, repair, or V12 completion.
- **Search-parity incident evidence:** a real global divergence belonging entirely to another project reproduced Claude Code's nonblocking SessionStart failure because schema validation incorrectly treated the derived FTS index as a whole-database gate. A two-project regression falsified the fix under the old gate, then proved current-project context, recent, and log remain available while global parity stays non-ready and search still returns the typed divergence refusal. The rebuilt Claude plugin's exact SessionStart command exits zero with no stderr against that unchanged database; no global repair or unrelated-project mutation was performed.
- **Direct evidence passed during implementation:** independent two-process `journal defer` convergence passed at count 10; schema-upgrade rollback and stale-source refusal tests passed; state, CLI, and command-surface suites passed in the integrated workspace. Atomic staged storage copy SIGKILL and concurrent-publisher coverage passed at count 10 and under `-race` count 3. Final Sol review was clean.
- **Project-move race evidence:** a deterministic independent-store claimant race passed at focused count 10 and under `-race` count 3; disabling transactional rejection falsified the guard. The protected path retained one owner/current mapping and both projects' journals remained preserved.
- **Final verification gates:** `LOAF_VALIDATE_TYPESCRIPT=1 npm run build` passed all targets with Go artifact synchronization; `npm run test` and typecheck passed. Executable Change check passed with only the expected unmaterialized `release-after` warning. Render-drift, ephemeral-provenance, changed-text secret, validate-commit, and security checks passed. Isolated public smokes proved manual origin capture, atomic defer/search, current schema, backup plus verify recovery/provenance readiness, and contract-v2 synthesis/lineage/blocker/Change projection without touching the global database.
- **Remaining boundary:** U7 normalized envelopes, outcome-aware trusted seams, namespaced origins, lifecycle suppression, safe-hook preflight, and replay evidence are implemented. U8 now has strict installed model-visible proof for Claude startup, Codex startup, and OpenCode request delivery; OpenCode completion is disabled, Cursor remains candidate, and Amp remains an explicitly unsupported automatic-context fallback with a foreground-turn candidate trigger. The U9 content-ownership primitive, target-adapter manifest slice, and read-only installed-ownership doctor slice are implemented: deterministic strict build manifests cover Claude Code, OpenCode, Cursor, Codex, and Amp, while strict CLI install ownership and recorded-install doctor checks cover OpenCode, Cursor, Codex, and Amp because Claude Code intentionally has no CLI installer. U9 still owns source-to-build comparison, capability mismatch and missing-adapter diagnostics, smoke freshness and absence diagnostics, broader documentation reconciliation, legacy `~/.amp` cleanup, isolated builds/upgrades, and approved real installed cross-harness dogfood; V12, U9, and the root Change remain open.

## Verification Contract

Executable (machine-checkable):

- **V1.** `loaf change check --require-executable docs/changes/20260710-journal-reliability-foundation` exits zero before implementation begins and after every contract revision; the same command against the parked terminal Change exits non-zero until `change-native-execution-migration` is materialized and present in ancestry.
- **V2.** Lineage fixtures reject duplicate slugs across dates, self-predecessors, two-node and longer cycles, lineage mismatch, missing predecessor under `--require-executable`, and multiple materialized children; `change list --lineage --json` remains deterministic after branches are renamed/deleted; checker output labels ancestry as structural materialization rather than completion; successor activation reruns predecessor verification against current main; release preflight leaves ancestry predating the first lineage node unaffected, refuses stable and normal-semver ancestry containing any nonterminal node without `release-after`, allows explicit prerelease dogfood only from an already-prerelease version, and accepts terminal ancestry.
- **V3.** A moved-checkout fixture proves `journal recent`, `journal search`, `journal context`, and ordinary `journal log` create zero project/path rows and return the exact move/init actions; `project move` preserves the original project ID and entries; simultaneous independent `state init` processes converge on one project and one current path.
- **V4.** Loaf-connection pragma tests assert WAL, foreign keys, busy timeout ≥5000 ms, and `synchronous=FULL`. Existing concurrent-writer tests pass; independent-process contention drops no entries; SIGKILL fault injection leaves no partial base/index pairs and `quick_check=ok`. Documentation labels these results transaction/process evidence and makes no stronger hardware claim.
- **V5.** FTS fixtures delete, add, and content-modify derived rows. Doctor reports exact divergence; journal search refuses trusted output; backup verification reports `journal_retrieval_ready=false`; dry-run is byte-for-byte non-mutating; apply creates a verified pre-repair backup, rebuilds from canonical rows, proves exact bidirectional parity, and is idempotent. Storage-home migration and all journal bulk paths finish with parity.
- **V6.** Concurrent backup tests produce a valid prefix snapshot whose watermark exists and whose canonical/index rows match. Local default output reports `device_loss_protected=false`; `--to` uses the requested absolute destination but rejects symlink-resolved paths under `os.TempDir()` and platform-known volatile roots. Isolated restore into an empty disposable path leaves the live database byte/table-equivalent to its pre-command state and produces an integrity-valid, foreign-key-valid, retrieval-ready database matching the snapshot's schema, projects, journal rows, FTS parity, and watermark; deleting the rehearsal output does not delete the source backup or retained verification evidence.
- **V7.** Origin migration preserves every existing entry and search result. Manual entries succeed with explicit missing harness/session values; each target fixture stores namespaced known values; unknown fields do not fabricate provenance. Dirty Change tests set `reconstructable=false`; committed matching digest tests set it true; missing worktree/session/transcript never makes the self-sufficient nugget unreadable.
- **V8.** Deferral tests prove one operation creates one journal decision and one spark or neither; byte-identical and same-intent-reworded response-loss retries return the original pair without error or double-write; output reports digest match/mismatch; concurrent retries converge; origin links are reciprocal; a deleted dirty worktree still leaves enough nugget content for triage and shaping.
- **V9.** Context fixtures with at least 30 noisy entries across multiple branches prove only `wrap(project)` occupies project synthesis; when none exists, the latest scoped wrap appears as a labeled non-project checkpoint rather than disappearing; the latest exact lineage decision, unresolved blocker, deferred spark, and active Change appear regardless of newer noise; matched unblocks resolve blockers while unmatched keys fail or diagnose visibly; every bounded layer reports available/shown/truncated/cursor/expand fields; expansion returns the omitted entries without duplicates.
- **V10.** Per-target hook falsification tests first prove whether the payload supplies trustworthy success/result metadata, then cover failed commit, no-op commit, amend, successful commit, repeated delivery, failed PR create, successful PR create, failed merge, successful merge, missing initialized state, subagent payload, and unknown fields. Targets without the required signal produce zero automatic completion entries; targets with it log only proven durable results keyed by SHA/PR identity; warnings are structured and non-blocking.
- **V11.** Target adapter unit tests assert model-visible returned/injected context and warning routing, not merely event registration. Installed-runtime smokes cover every claimed supported mode, including Claude Code, Codex, and OpenCode request delivery; Cursor and Amp remain explicit candidate/fallback records, and each capability record independently names context-delivery and completion-capture support plus the tested version/mechanism. Any unproven/bypass mode emits the explicit fallback and is absent from the corresponding automatic-capability claim.
- **V12.** Managed-install tests change instruction/hook content without changing package version and prove upgrade still converges by fingerprint. Source/build/install manifests match in isolated homes; stale managed artifacts are removed; user-owned content is preserved/reported; doctor distinguishes stale body, missing adapter, unsupported version, and absent smoke evidence.
- **V13.** Cross-harness dogfood writes a decision and a dirty deferral through one supported target, resumes through a different supported target, retrieves both exact IDs and honest origins, sees lineage/blocker/deferral/Change projections, expands truncated layers, verifies backup/restoration in isolation, and records no mutation of unrelated project rows.
- **V14.** `npm run build`, `npm run typecheck`, `npm run test`, `loaf check`, generated-target verification, schema-doc parity, and `git diff --check` pass. Tests and probes use absolute temporary `LOAF_DB` paths only for disposable fixtures; reboot/cleanup loss of those paths cannot remove the sole copy of a decision, scenario result, backup, or recovery artifact. Any intentional real install/global-state dogfood is separately user-approved and followed by source/build/install verification.
- **V15.** On installed Codex `0.144.1`, `codex execpolicy check` classifies only the pinned canonical absolute Loaf executable followed by an explicitly registered safe prefix as allowed outside the sandbox; bare/alternate executables, ordinary journal logging, body/file-consuming and arbitrary-path leaves, unknown family kinds, other unclassified/operator Loaf namespaces, shell wrappers, and caller-wrapped environment overrides remain unmatched. Safe-mode runtime tests accept manual, `--from-hook`, `--detect-linear`, and JSON suffixes while rejecting `LOAF_DB`, explicit worktree/branch/session overrides, conflicting modes, and unknown project identity. Isolated `CODEX_HOME` install tests prove explicit `--codex-basic-commands` opt-in, executable trust/absolute rendering, matching basic-policy guidance, idempotent and interruption-recoverable fingerprint upgrades, unrelated-rule/guidance preservation, legacy journal-only retirement without consent broadening, malformed/unowned/modified conflict refusal, rule-first retirement without PATH, missing-template build failure, and no writable-root modification. The hardened journal prefix retains positive/negative real Auto-mode evidence; U9 reruns fresh full-policy installed dogfood. Unsupported harness adapters remain outside the support claim.

Human review:

- **H1.** A person can move a checkout, lose a worktree, switch branches, and change harnesses without receiving a plausible but empty or stale project story.
- **H2.** Every use of durable, automatic, complete, supported, verified, recoverable, or reconstructable is tied to a named mode and direct evidence; weaker modes are labeled honestly.
- **H3.** The default digest is compact but not deceptive: active lineage, blockers, deferrals, and Changes are visible; omissions and expansion are obvious; no unrelated scoped wrap masquerades as project synthesis.
- **H4.** Deferred work remains understandable from its nugget alone while origin makes deeper investigation possible when source context still exists.
- **H5.** Recovery instructions distinguish local rollback, project-scoped replay, and external disaster recovery, and never invite concurrent live replacement of the global database.
- **H6.** The immediate successor packet is sufficient to initialize `change-native-execution-migration` after this conversation/branch is gone; the terminal stub preserves its hard-cut intent without treating parked commit `59fbcdcf` as current truth.
- **H7.** Enabling Codex's basic command policy is an explicit, understandable one-time trust decision for the centrally classified routine state-plane and read-only Loaf leaves; it does not authorize unclassified/operator commands, make the Loaf database generally writable to arbitrary processes, imply other harness adapters, or imply that Auto-review grants filesystem access.

## Definition of Done

- Unknown or moved checkouts cannot silently create new project identity through journal reads/writes; explicit init/move is race-safe and preserves continuity.
- Canonical journal entries and every derived search row have a diagnosed, repairable, backup-verified parity contract; search cannot return a trusted false empty result when divergence is detected.
- SQLite connection durability is explicit and tested at the claims it supports; ordinary concurrency and abrupt process termination retain journal/index atomicity.
- Backups state their recovery tier and journal watermark; retrieval readiness is verified; isolated restore is exact; live global replacement is not misrepresented as safe.
- Automated restore-to-live is deliberately absent; recovery activation is a documented quiesced operator procedure, while the next Change owns safe project-scoped replay.
- Important manual, skill, hook, migration, git/PR, and deferred-intent entries carry honest normalized origin when available and remain valid without optional harness identity.
- Atomic deferral is idempotent, self-sufficient, connected to spark intake, and survives loss of dirty source context.
- Context gives dedicated precedence to project synthesis, lineage, blockers, deferrals, and active Changes; every bounded layer discloses omissions and expansion.
- Change lineage cannot pass structural executability with missing/cyclic/forked ancestry or stable release during the accepted freeze; explicit prerelease dogfood is limited to an already-prerelease version and explicit prerelease bump; discovery survives branch deletion; successor activation freshly verifies predecessor behavior because structural ancestry is not completion; no lineage entity/status was introduced.
- Automatic git/PR journal events represent proven durable outcomes only.
- Every supported installed target has a runtime-proven model-visible delivery path for its named version/mode or a clearly labeled explicit fallback; generated no-op adapters are gone.
- Codex Auto mode can use one explicitly installed, hardened policy for centrally classified basic Loaf leaves without full access or a general Loaf-state writable root; absent, stale, conflicted, redirected, unclassified, or unsafe-executable states fail visibly.
- Managed instructions/hooks/plugins converge by content fingerprint across source, build, isolated install, and approved real install even without a version bump.
- Cross-harness dogfood passes without mutating unrelated projects; all executable and human review criteria pass; deviations are reconciled into the successor packet before materialization.

## Durable Outputs

- `docs/ARCHITECTURE.md` — canonical/derived journal boundary, identity ceremony, durability tiers, semantic capture, continuity precedence, target adapters, and derived lineage model.
- `docs/schema/` plus migration documentation — origin envelope, operation-key uniqueness, FTS parity/rebuild, and any new indexes/constraints after implementation proves the final shape.
- `docs/specs/` — post-implementation contracts for journal reliability/recovery, context JSON, deferral capture, Change lineage checking, and target capability/install parity.
- `docs/decisions/` — accepted ADR if implementation confirms that explicit project registration, `synchronous=FULL`, derived lineage, and target-specific adapters are architecturally durable choices.
- README and `content/skills/{orchestration,shape,triage,loaf-reference}/` — honest user workflows, commands, fallback modes, and recovery instructions.
- Versioned target capability/build manifest plus runtime smoke fixtures sufficient to prevent generated support claims from drifting away from installed behavior.
- `docs/changes/20260710-journal-reliability-foundation/research/adversarial-scenarios.md` — falsification evidence, rejected alternatives, and lineage re-evaluation retained with the Change.

## Open Questions

- [Resolved in U8] Codex SessionStart command stdout enters model-visible context for startup on exact CLI 0.144.1 under the retained isolated-home smoke; resume, clear, compact, global-home installation, and Windows runtime remain separately unproven and use the explicit fallback.
- [Resolved in U8] OpenCode 1.17.18 request transformation is model-visible under the retained isolated-XDG smoke; resume and compact remain candidates. Amp 0.0.1783873056-g278461 exposes a foreground-turn context surface but cannot prove root-only identity, so automatic context remains disabled with the explicit fallback.
- [KU] What parity guard is cheap enough for every `journal search` while still preventing known false-empty divergence? → U3 benchmark identity/content checks against realistic journal sizes; doctor always performs exact bidirectional validation.
- [KU] Should `state backup --to` accept a directory only or an exact file path, and how should collision naming behave? → U4 CLI consistency spike against existing backup path allocation; deterministic non-overwrite behavior and symlink-resolved volatile-root rejection are mandatory.
- [KU] What minimal quiesced activation instructions are safe across SQLite WAL files and older Loaf processes after an isolated restore succeeds? → U4 source-backed SQLite procedure and destructive rehearsal only inside a disposable data home; no live automation without a universal mutation lease.
- [KU] Can current Change discovery derive active Changes and predecessor ancestry entirely from retained files/git without GitHub availability? → U1 local-first fixture matrix; hosted PR metadata may enrich but never become required authority.
- [UU] Which harness bypass/customization modes or installed-path conventions still evade generated/install detection? → U8/U9 capability inventory and isolated/real install comparison; any newly discovered target mode is unsupported until classified.

## Critique Gate

- **Did adversarial review change the plan?** Yes. It moved priority from richer journaling to silent-identity and silent-retrieval failures, rejected nominal hook parity, added outcome-aware automation, made context completeness explicit, and turned lineage prose into an executability/release contract.
- **Did independent Claude review change the revision?** Yes. The fresh Opus pass confirmed every source diagnosis and kept the one-Change/three-node shape, then exposed six remaining contract holes: first-run wrap regression, reworded response-loss retries, ambiguous release scope, structural-ancestry overclaim, missing capture-fidelity classification, and unmatched blocker keys. All six are incorporated; the optional extra lineage Change is rejected because U1 can land first inside this unreleased foundation without adding another node.
- **Did the amendments close the named findings?** Yes. A narrow second Opus pass checked only F1–F6 across Scope, Decisions, Planning, and Verification, marked every finding `CLOSED`, found no direct contradictions, and returned `APPROVE`.
- **Is the foundation too broad?** It is large because journal trust is an end-to-end property. Shipping only storage, context, or adapters would still let downstream work consume a false guarantee. Units remain independently reviewable and reversible; a new dependency or unsupported runtime requirement reopens the one-Change decision.
- **Is active truth becoming status?** No. Every projection is derived from append-only latest-per-key entries, unresolved spark state already in intake, retained Change files, and git ancestry; no mutable journal/lineage lifecycle is added.
- **Does recovery promise zero loss?** No. Every backup reports a watermark. `FULL` narrows committed-write risk; external copies remain point-in-time and operator-selected; isolated restore proves usability but not concurrent activation.
- **Does origin preservation depend on transcript availability?** No. The nugget is the minimum durable payload; origin is optional evidence and reconstructability is a checked claim.
- **Can one noisy branch hide project-wide intent?** Not under the new precedence contract. Dedicated active projections do not share the recency limit, and every layer exposes omitted counts and expansion.
- **Does the model survive loss of the parked branch?** Essential lineage order, decisions, no-gos, activation gates, outputs, fog, and citations live in this root Change plus structured journal decisions. The remote-ref recommendation protects the detailed draft but is not the sole continuity mechanism.

## Source Inputs

- `research/adversarial-scenarios.md` — runtime/source scenarios, positive evidence, selected corrections, rejected alternatives, and lineage re-evaluation.
- Journal decision `journal:a5a4094d11591a87c29d12db` and discovery `journal:448e39538bbe9e35b67e1e55` — adjudicated the fresh Claude Opus `REVISE` review: accepted six bounded corrections, kept one foundation Change and the three-node lineage, and rejected a new U1-only lineage node.
- Journal discovery `journal:537239e4c916a610fbcb1895` and decision `journal:c550982f58d9f1acfe819e94` — narrow Claude fix verification returned `APPROVE` with F1–F6 closed and no contradictions; shaping is executable but implementation remains unstarted pending user review.
- Journal decision `journal:d98256fa9de6deee30365ca9` — OS temporary directories are disposable test surfaces only; durable backups, recovery artifacts, lineage packets, and review evidence must use repository/git, journal state, or an explicitly durable non-temporary destination.
- Journal decision `journal:415697c16b7b0610fdb56eed` — ADRs are rare, stable, narrowly architectural records; Changes/journal own most prospective decisions, and intentional refactors supersede or deprecate applicable ADRs rather than being vetoed by them.
- Journal decision `journal:d5c01f00a4c4051cea25b1d1` — no-silent-success reliability boundary and canonical/derived/adapter decisions.
- Journal decision `journal:00f302e40678b0f13b61f28c` — structured accepted lineage order, one-worktree/no-release invariant, successor-packet requirement, and parked-draft authority correction.
- Journal discoveries `journal:8415e37b871c53a8783c03c9`, `journal:a8407065f97483c97f2ce4d2`, and `journal:c3c2d1c1deb3cc69a613c8c4` — context/search/recovery, lineage, provenance, and fail-visible capture failures.
- Earlier journal decisions `journal:8255bef3bf2f5cd68a57fd6f`, `journal:156603e71686c8119574484d`, and discovery `journal:d3b5dd930a687f0edfa031c3` — original lineage, journal trust invariant, and target parity gaps.
- Parked terminal Change `spec-conversion-and-guidance-sweep` at commit `59fbcdcf` — detailed working draft for hard-cut migration/execution/guidance, deferred-intent handshake, and original serial activation protocol.
- Current source under `internal/state/{journal.go,journal_context.go,journal_recent.go,project_identity.go,search.go,status.go,backup.go,storage_home_migration.go,store.go}`, `internal/cli/{change.go,journal.go,journal_hooks.go,build_codex.go,build_opencode.go,build_amp.go,install_fenced.go}`, `config/hooks.yaml`, and `docs/ARCHITECTURE.md`.
- Isolated 2026-07-10 moved-checkout, FTS-divergence, context-noise, invalid-lineage, concurrent-write, concurrent-backup, first-init, SIGKILL, and dirty-provenance scenarios; no adversarial database probe used the global Loaf database.
- Historical/superseded U6A Codex evidence: `codex-cli 0.144.1`, stable `hooks` feature, accepted local SessionStart/UserPromptSubmit/Stop configuration, official Rules/Auto-review/Hooks documentation, local `codex execpolicy check` parsing, and isolated positive/negative Auto-mode runs. The runtime rejected Loaf's legacy flat PreToolUse file, accepted the then-generated empty current-schema fallback, wrote and retrieved one journal entry only with the managed exact-prefix rule, and permission-blocked the same write when that rule was absent. U8's current-schema SessionStart adapter and isolated model-visible smoke supersede the empty-hook artifact; this retained record is historical policy evidence only.
- Claude CLI review evidence: the first `claude -p` attempt stalled while starting unrelated configured MCP servers; a second strict empty-MCP read-only attempt produced no verdict within a bounded five-minute wait and was stopped. No approval was inferred from silence; U6A proceeds against official Codex documentation, local policy parsing, executable tests, and an independent Sol review.
- Three independent read-only adversaries covering storage/recovery, cross-harness delivery, and lineage semantics.
- The 2026-07-10 conversation establishing the journal as Loaf's differentiating cornerstone and requesting adversarial scenario review before implementation planning is accepted.
