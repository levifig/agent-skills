# Journal Reliability Adversarial Scenarios

## Verdict

The journal-first direction survives review, but the current implementation and the parked hard-cut contract do not yet justify the words reliable, holistic, automatic, or recoverable. Ordinary initialized-project concurrency is a relative strength. The dominant failures are silent identity forks, silent retrieval divergence, false completeness in continuity output, false success in automatic events, unenforced Change lineage, and generated target integrations that execute continuity commands without delivering their output to the model. All isolated `/tmp` and `LOAF_DB` probes were disposable falsification fixtures; the durable evidence is this repository artifact plus journal decisions, not the temporary databases or worktrees.

The selected response is to keep the accepted three-Change serial lineage and make `journal-reliability-foundation` a strict prerequisite with one governing rule: Loaf must not silently claim that project memory was captured, found, delivered, complete, or recoverable when it was not. The root Change carries self-sufficient successor packets so the parked terminal branch is useful evidence but never the sole authority.

## Scenario Matrix

| Scenario | Result | Severity | Selected correction |
|----------|--------|----------|---------------------|
| Move an initialized checkout, then run `journal recent` from the new path without `project move` | A second project ID is created, the old journal becomes invisible, and doctor reports the empty identity healthy | P1 verified | Reads and ordinary writes resolve only registered project paths; only explicit initialization creates identity; an unknown path fails with `loaf project move --from <old-path>` guidance; first registration becomes one transaction |
| Delete a `journal_search` row while leaving its canonical `journal_entries` row | Search returns zero, doctor stays green, and backup verification certifies the divergent snapshot | P1 verified | Declare `journal_entries` canonical; add bidirectional identity/content parity diagnostics, guarded index rebuild, post-bulk-operation rebuild, and retrieval-readiness to backup verification |
| Hold a write transaction while independent processes log entries | Existing focused Go tests pass and no entries are dropped | Positive verified | Preserve WAL plus busy timeout and the current transactional base/index write; add rather than replace the existing contention coverage |
| Kill journal writers around commit boundaries | Isolated probes retained base/index parity and SQLite integrity | Positive verified with bounded claim | Keep process-kill atomicity tests; separate process termination from OS crash and power-loss claims; pin `synchronous=FULL` before claiming committed-write durability beyond process death |
| Back up while another process writes | `VACUUM INTO` produces a valid prefix snapshot | Positive verified | Preserve consistent snapshot creation; report the covered journal watermark so the recovery point is explicit |
| Verify a backup, then try to restore through Loaf | Verification succeeds but no global database restore command exists; documented replacement is unsafe while harnesses may still hold connections | P1 risk with missing mechanism verified | Distinguish local rollback snapshot, non-temporary external disaster copy, and project-scoped replay; reject known volatile backup destinations; add isolated restore-and-verify rehearsal whose output is explicitly disposable; do not claim concurrent live global restore |
| Log a project decision on branch A, then start on noisy branch B | Context shows the latest unrelated wrap, only ten branch-B entries, no lineage decision, and no omission signal | P1 verified | Only `wrap(project)` is project synthesis; until one exists, show the latest scoped wrap as a separately labeled non-project checkpoint; give lineage, unresolved blockers, deferrals, and active Changes dedicated latest-per-key projections; expose counts, truncation, cursor, and expansion command for every bounded layer |
| Materialize a self-cycle, two-node cycle, missing predecessor, or two children in one lineage | `loaf change check --require-executable` accepts the invalid graph | P0/P1 verified | Validate globally unique immutable slugs, one predecessor for non-roots, lineage agreement, self-reference, cycles, missing prerequisites, and multiple materialized children; make unsatisfied predecessors an executability gap |
| Run the parked terminal Change through `--require-executable` before either prerequisite exists | It passes even though its own contract says it must land last | P0 verified | Add the predecessor to the parked Change when refreshed and make predecessor ancestry a required structural precondition; carry the dependency packet in the root Change immediately; rerun predecessor verification on current main before successor activation because plan ancestry is not completion |
| Merge the first lineage node, then attempt an unrelated release before the terminal node | An unscoped `release-after` rule either freezes every release forever or is ignored outside the Change branch | P1 contract gap | Freeze releases only when their ancestry contains any node of the lineage but not its named terminal; ancestry predating the first node remains unaffected; this lineage accepts the freeze without a hotfix override |
| Delete or rename the branch after a Change is retained | Bare `loaf change check` cannot discover the Change because branch matching is the only current resolver | P2 verified | Add `loaf change list --lineage <key> --json`; treat retained Change slug as identity and branch as provenance after merge |
| Start OpenCode with the generated plugin and later compact | The plugin runs context commands, discards stdout, and listens on the wrong compaction injection surface | P0 source-verified | Use an OpenCode-specific model-context adapter and compaction transformation; test injected content, not generated event strings |
| Start Amp with the generated plugin | The handler runs every session hook at startup and discards stdout | P0 source-verified | Use an Amp-specific agent-start context message; reserve session-start for initialization with no model-visible result |
| Start Codex 0.144.1 after Loaf installation | Native hooks are stable and local config accepts SessionStart/UserPromptSubmit/Stop, but Loaf installs only PreToolUse hooks | P0 capability gap verified; output injection not yet runtime-proven | Spike native SessionStart output injection; generate it if proven, otherwise install an explicit managed-instruction fallback; version the capability record by tested harness version |
| Log manually or through adapters without a Claude-shaped payload | `harness_session_id` is null or target identity is discarded; subagent suppression is Claude-specific | P1 verified | Add a versioned Loaf hook envelope and namespaced optional harness/session provenance; missing identity remains explicit and valid |
| A failed `git commit` or `gh pr create` reaches PostToolUse | Command text alone can create `commit(unknown)` or an unverifiable PR event | P1 source-verified | Automatic completion events require a target payload with proven success/result fidelity plus resolved durable identity; use commit SHA or PR number as an idempotency key; targets without the signal emit no automatic completion event |
| Write `block(login-flow)` and later typo `unblock(auth)` | A latest-per-key projection preserves a false active blocker indefinitely while appearing authoritative | P1 contract gap | Validate new unblock keys against an exact open block; interactive mismatch fails visibly, adapter mismatch emits a visible nonblocking diagnostic, and context exposes the unresolved-block expansion command |
| Defer work while `change.md` is dirty, then lose the worktree | HEAD identifies the parent snapshot, not the uncommitted discussion; a digest proves mismatch but cannot reconstruct content | P1 verified model flaw | Make the deferred nugget self-sufficient; record Change path, HEAD, digest, and dirty flag; advertise reconstructability only when committed content matches the digest; never auto-commit |
| Retry a compound deferral after commit succeeded but the client missed the response | Timestamp-derived journal IDs create a duplicate; a fresh agent may reword the retry | P2 source-verified, P1 for compound capture | `journal defer` receives or creates an operation key before mutation and derives the journal/spark pair from it under one uniqueness constraint and transaction; first write wins and reworded retries return the original pair with digest-mismatch telemetry rather than erroring |
| Change managed instructions without bumping the package version | Installer can skip the changed body because the marker version is equal | P1 source-verified | Fingerprint managed content and compare source/build/install digests; version equality alone is not convergence |
| Lose the local repository or delete the parked terminal branch | Commit `59fbcdcf` and the full terminal plan disappear because they have no remote ref; the journal holds only a summary | P1 operational risk | Root Change carries bounded successor packets; after explicit approval, persist the parked branch as a remote ref without opening a premature PR |

## Evidence Anchors

- Project identity creation on read: `internal/state/tag.go:321-335`, `internal/state/project_identity.go:119-169`, and `internal/state/journal_recent.go:51`.
- Canonical/index transaction and search-only reads: `internal/state/journal.go:80-110`, `internal/state/search.go:166-173`, `internal/state/status.go:474-631`, and `internal/state/backup.go:129-165`.
- Missing FTS rebuild after storage-home merge: `internal/state/storage_home_migration.go:21-25` and `internal/state/storage_home_migration.go:238`.
- Current context shape and arbitrary latest wrap: `internal/state/journal_context.go:13-169`.
- Current journal origin fields and timestamp-derived identity: `internal/state/journal.go:16-79`.
- Missing-state hook logging is intentionally silent: `internal/cli/journal_hooks_test.go:260-267`.
- Automatic git/PR derivation ignores command outcome: `internal/cli/journal_hooks.go:57-110`.
- OpenCode discards hook output: `internal/cli/build_opencode.go:230-242`.
- Amp runs and discards projected session hooks: `internal/cli/build_amp.go:331-365` and `internal/cli/build_amp.go:439`.
- Codex builder emits only PreToolUse enforcement hooks: `internal/cli/build_codex.go:633-678`; the installed Codex CLI is `0.144.1`, reports stable hooks, and accepts SessionStart/UserPromptSubmit/Stop in `~/.config/codex/hooks.json`.
- Same-version managed body skip: `internal/cli/install_fenced.go:50`.
- Change checker does not validate lineage: `internal/cli/change.go:421-455`.
- Local backup placement and verification: `internal/state/backup.go:68-165`.
- Parked terminal contract and lineage protocol: commit `59fbcdcf`, `docs/changes/20260710-spec-conversion-and-guidance-sweep/change.md`.

## Chosen Reliability Contract

### Canonical versus derived

`journal_entries` is canonical. Full-text search and continuity projections are derived. Every derived structure must have a machine-checkable parity invariant, a deterministic rebuild, and a visible refusal mode. A valid SQLite file is not automatically a retrieval-ready journal.

### Identity continuity

A read must never create a project. An ordinary journal write must never silently decide that an unknown path is a new project. Fresh-project registration is explicit and atomic; relocation reuses `loaf project move`. This protects the more important guarantee: an entry remains findable from the project the user believes they opened.

### Durability claims

The product distinguishes transaction atomicity, process termination, OS crash, power loss, local corruption rollback, and device/data-home loss. It pins `synchronous=FULL`, retains WAL and busy timeout, and tests process termination. It does not describe external disaster recovery as zero-loss or claim a live global restore is concurrency-safe.

### Semantic capture

Origin metadata is supporting evidence, not a substitute for content. Important decisions and deferrals are self-sufficient nuggets. A source pointer may help recover nearby discussion, but the journal remains useful when the transcript, harness session, worktree, or dirty Change body is gone.

### Continuity synthesis

Active truth outranks recency. Project synthesis is opt-in through `wrap(project)`; until one exists, the newest scoped wrap remains visible in a separate checkpoint slot labeled with its scope and explicitly marked non-project synthesis. Latest-per-key lineage decisions, validated unresolved blockers, deferred sparks, and active Changes receive dedicated bounded projections and cannot be crowded out by recent noise. Every bounded layer states what was omitted and how to expand it.

### Harness delivery

There is no generic session-hook guarantee. Each target has a versioned capture adapter, delivery adapter, success/result-signal classification, supported/bypass modes, and runtime smoke. Context delivery and completion capture are separate capabilities. If a model-visible effect or result-bearing signal is not proven, the target gets an explicit fallback or no automatic completion, and documentation may not call it automatic.

### Lineage

Lineage remains derived from retained Changes plus structured `decision(lineage/<key>)` entries. No lineage table, mutable status, or standalone program manifest is added. Materialized Changes form one enforceable chain; successor packets remain prose until materialized, and the current Change carries enough of the next two nodes to survive loss of the parked working draft.

## Rejected Corrections

- **A new lineage entity or mutable program record.** This duplicates retained Changes and journal decisions and recreates shared lifecycle state.
- **Raw transcript ingestion.** It expands the privacy and portability surface without fixing semantic capture; source pointers remain optional evidence.
- **A generic cross-target hook projector.** Event names are not outcome parity; target adapters must translate output into the runtime's actual model-context mechanism.
- **Auto-committing dirty Changes for provenance.** Provenance must not mutate work or force checkpoints; dirty state is reported honestly.
- **Treating any latest wrap as project truth.** Only explicit project synthesis earns the project-wide slot.
- **Silently falling back from an unknown checkout path to a new project.** The safe behavior is refusal plus an exact relocation or initialization action.
- **Calling same-volume or OS-temporary snapshots disaster recovery.** They are disposable/rollback snapshots; device-loss protection needs an explicit non-temporary external destination and a reported recovery watermark.
- **Promising concurrent in-place global restore.** The foundation proves isolated restore and documents quiesced activation; project-scoped replay remains the normal rollback mechanism for later migration.
- **Replacing the FTS table solely to avoid a repair command.** Bidirectional parity plus deterministic rebuild is the smallest auditable correction; a trigger/external-content redesign can be reconsidered only if the invariant proves too expensive or fragile.

## Lineage Re-evaluation

The three-node order remains:

1. `journal-reliability-foundation`
2. `change-native-execution-migration`
3. `spec-conversion-and-guidance-sweep`

The protocol changes in four ways:

1. The root Change stores a self-sufficient immediate-successor packet and a compact terminal stub.
2. Structured lineage decisions use exact scope `decision(lineage/change-model-hard-cut)` so continuity can select them without parsing prose.
3. A materialized non-root Change cannot pass structural executability until its predecessor is materialized in the same lineage and present in its git ancestry; cycles, self-reference, missing predecessors, and multiple materialized children are rejected. This does not prove implementation completion, so successor activation freshly reruns the predecessor Verification Contract and dogfood against current main.
4. A parked materialized Change must have a durable ref if it is worth preserving. For this repository, the recommended action is a remote branch without a PR after explicit user approval; it does not create another active worktree or imply implementation has begun.

Release gating is ancestry-scoped: a release containing any lineage node but not the named terminal is refused, while ancestry predating the first node is unaffected. The accepted hard-cut lineage deliberately freezes releases during that window and provides no hotfix override.

## Claude Opus Critique Adjudication

The fresh read-only `claude -p --model opus --effort high` review returned `REVISE`. It independently checked every evidence anchor against current source and found the diagnosis accurate. It agreed that the journal-trust work should remain one Change and that the three-node lineage is defensible, then identified six bounded contract gaps.

| Finding | Adjudication |
|---------|--------------|
| Existing projects have no `wrap(project)`, so a strict new synthesis slot would hide today's latest checkpoint on first run | Accepted: render the newest scoped wrap in a separately labeled non-project checkpoint slot until explicit project synthesis exists |
| Strict semantic-digest equality would reject a legitimate response-loss retry reworded by a fresh agent | Accepted: operation ID is first-write-wins; every retry returns the original pair and reports digest match/mismatch as telemetry |
| `release-after` had no defined ancestry scope | Accepted: freeze begins when release ancestry first contains any lineage node and ends when it contains the terminal; older ancestry is unaffected; no hotfix override for this accepted lineage |
| Predecessor plan-file ancestry was described as implementation completion | Accepted: checker enforces structural materialization only; successor activation reruns the predecessor Verification Contract and dogfood on current main |
| Target delivery matrix omitted whether hook payloads contain trustworthy success/result signals | Accepted: model context and completion capture are independent per-target capabilities; unknown/absent signals mean no automatic completion |
| Free-text `unblock` keys can leave a false active blocker | Accepted: validate exact open keys and fail or diagnose visibly; retain an expansion command for unresolved blockers |

The optional proposal to split U1 into a fourth immediate Change was rejected. U1 is deliberately first, the product remains unreleased, the terminal branch stays manually parked, and another node would add lineage ceremony without improving the end-to-end journal guarantee. Automated restore-to-live was also not added; the contract now states explicitly that isolated rehearsal plus quiesced operator activation is the selected boundary.

A narrow second Opus pass then rechecked only those six findings against the amended Change. It marked F1–F6 `CLOSED`, found no direct contradiction introduced by the amendments, and returned `APPROVE`.

## Independent Review Inputs

- Storage/recovery adversary: runtime-isolated moved-checkout, FTS-divergence, concurrent-backup, SIGKILL, first-registration, and migration/write probes.
- Cross-harness adversary: generated Claude/Cursor/Codex/OpenCode/Amp builders, installed Codex capability, hook payload assumptions, event success semantics, and managed-install behavior.
- Lineage adversary: executable parked terminal, missing/cyclic/forked predecessor fixtures, branch-deletion discovery, release barrier, and successor-packet survivability.
- Claude Opus external critique: source-confirmed diagnosis plus first-run synthesis, retry idempotency, release scope, ancestry semantics, capture fidelity, and blocker-key review.
