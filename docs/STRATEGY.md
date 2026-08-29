# Strategy

_Last updated: 2026-08-29_

Loaf is an opinionated agentic framework for AI coding assistants. It ships portable skills, bounded agent profiles, target-native adapters, enforcement hooks, and a native CLI. This document states current strategy; detailed history and evidence remain in [Changes](changes/), [decision records](decisions/), [reports](reports/), and git history.

## Who This Serves

**Solo developers** need an agent workflow that reduces context-switching overhead, preserves work across conversations, and enforces quality without becoming more cumbersome than direct tool use.

**Teams** need consistent agent behavior across developers and harnesses, with auditable decisions, trustworthy quality checks, and installation behavior that does not require everyone to understand Loaf internals.

## vNext Reset

Loaf vNext is being built as an isolated production boundary under `vnext/`. The current implementation remains operational until cutover, but it is input evidence rather than a reusable runtime layer: vNext may learn from its behavior and consume a versioned one-time export, but cannot import its command, state, issue, work-contract, crypto, sync, or scratchpad packages.

The ownership contract is deliberately small and exclusive:

| Authority | Canonical responsibilities |
|-----------|----------------------------|
| Loaf | Flow ceremonies, skills, templates, profiles, project identity, private continuity, derived context, and private sync |
| Tracker | Work identity and definition, definition of done, workflow state, hierarchy, assignment, and collaboration |
| Git | Code and deliberately promoted artifacts |
| Harness | Execution, model selection, tool boundaries, service connections, and service credentials |

The vNext schema line starts independently at `vnext/1`. Its bootstrap command surface contains only `version` and `ownership`; it has no state, tracker, sync, migration, build, or install command yet. New command families are added only with the slice that owns and proves their behavior.

## Proven Principles

**Skills are the portable knowledge layer.** Shared authoring should remain the default, while target-specific adapters translate that knowledge into the strongest trustworthy native surface each harness exposes.

**The CLI is the protocol layer for Loaf-owned mechanics.** Skills describe judgment and workflow, the CLI performs deterministic continuity, identity, filesystem, and diagnostic operations, and hooks enforce or inject narrowly scoped behavior. Tracker operations use the harness's configured native connection; Loaf does not put a provider adapter between the agent and the tracker.

**Continuity belongs to the project journal, not a session lifecycle.** The journal remains the private timeline and resumption spine; vNext's operator-owned substrate also preserves wraps, sparks, ideas, decisions, explorations, findings, handoffs, scratchpad coordination, and derived context. Tracker work remains outside it. Scratchpad coordination is private, effort-scoped, and ephemeral: it never owns work, definition of done, workflow, hierarchy, assignment, or team collaboration state. Context is derived at read time, and a wrap is an optional synthesis checkpoint rather than a transition.

**Managed installation requires ownership evidence.** Loaf should change installed content only when it can identify what it owns and verify the expected digest. Capability claims must be tied to exact client versions and installed runtime evidence rather than inferred from build output.

ADR-020 preserves that single-overlay result while making root `AGENTS.md` the canonical real file. This improves standards-native discoverability and removes the unnecessary root indirection; `.agents/` remains reserved for Loaf state and configuration, while `.claude/CLAUDE.md` remains a compatibility symlink.

**Continuity must survive everything -- the session entity didn't need to.** Context compaction, `/clear`, tool restarts, and cross-conversation handoffs all create new harness session IDs pointing at the same stream of work. Any architecture that assumes 1 session = 1 conversation fails in practice.

**Automation must fail within its evidence boundary.** Automatic completion remains disabled unless a target supplies trustworthy success evidence and a durable event identity. When a harness cannot distinguish the relevant traffic or lifecycle event reliably, an explicit fallback is preferable to a false guarantee.

**The tracker is the protocol layer for shared work.** General workflow skills tell the agent what outcome and template to apply, then the agent uses its harness-native tracker tools. Provider-specific skills or a narrowly instructed project-management profile may encode interaction details, but Loaf neither configures credentials nor stores provider mappings, retry queues, reconciliation state, or mirrored issue fields.

**Diagnosis and repair must share the same state taxonomy.** Sharing repair helpers is not enough; the detection branches in a diagnostic tool must consult the same classification logic as the repair path, or they will drift apart.

An early `loaf doctor` silently skipped legacy-layout states -- a real `./AGENTS.md` or `.claude/CLAUDE.md` with no canonical overlay -- even though the helper that repairs those states was already written and invocable from the fix path. The two halves had diverged: the repair path knew about legacy layouts, the detection path did not. External review caught it, and the fix rebuilt detection to surface legacy states as fixable `fail` and taught the fix paths to create canonical lazily before delegating to the shared helper. For both personas this means `loaf doctor --fix` can be trusted to heal every state the diagnostic surfaces as fixable, because the same state definitions drive both halves of the state machine.

ADR-020 applies that lesson to the reverse migration: doctor now classifies root-file canonicality, retired `.agents/AGENTS.md` presence, and Claude compatibility-link drift independently. Plain diagnosis stays read-only; `--fix` asks before each repair, preserves legacy content and backups for accepted repairs, and requires `--force` to accept all repairs non-interactively.

**Truthful state is a product invariant.** Two read surfaces disagreeing about how much work exists is worse than an error: a done-task query that returned zero rows while 66 done tasks existed read as "no work," and agents inherited the blindness — the librarian had to bypass the CLI and read SQLite directly to produce an honest report. The June-24 identity fork sat invisible for six weeks because the list surfaces and the housekeeping scanner used different predicates and nothing compared them. The lesson generalizes: wherever two surfaces can drift, a detection diagnostic must exist (doctor's alias parity), and repair tooling must never guess — refuse-by-default with explicit operator dispositions is what made retiring 1,363 rows from the production database an act of judgment rather than faith.

**Review lenses converge, and convergence is the stop signal.** The state-dedupe repair went through eight adversarial passes across two independent models, and each pass surfaced a different defect class: unsafe deletion proofs, then post-commit failure semantics, then cross-component seams, then documentation honesty. Finding *class* trajectory — not finding count — told us when review was done. Two shapes mattered: single-concern review briefs succeeded where a five-mandate mega-brief died, and for destructive tooling the highest-value verdict was a reviewer *reproducing* a defect on a disposable production copy before merge. Review depth is priced by irreversibility: data surgery earned eight passes; the docs commit that followed earned one.

**Personality is decorative, mechanics are durable.** An attempt to decouple agent personality (Warden/Fellowship lore) from agent mechanics through a swappable souls catalog was fully implemented, reviewed twice, and then pivoted in flight before merge: after months of real use the lore had never landed, and review flagged a prose-driven soul file as a brittle prompt-injection surface enforceable only by advice. What shipped keeps the profile neutralization and the skill prose audit; the catalog, CLI, install, and startup-restoration layers were removed.

The implication for both personas: Loaf's value is the *framework* -- mechanical hooks, structured pipeline, profile boundaries, knowledge layer -- not a personality layer. Adding identity through prompt content is incompatible with hardening agents: it costs complexity, adds attack surface, and delivers value only when the user already buys into the metaphor. The lore concept survives in the archived record as an account of what was tried, with a possible future home in a Pi-based harness where it could be load-bearing rather than decorative.

**Delegated implementation holds when verification is structural, not trust-based.** An implementer's report is a claim, not evidence: in the pitch-entrypoint batch (v0.2.17) one delegated agent reported re-running installed smokes when its diff showed only hash re-pins, and another reported a full test pass that had covered one package tree. Both were caught because the orchestrator verified every delivering commit against the contract's own gates — tests, hooks, checkbox state, projection greps, isolated end-to-end smokes — and a cross-task regression surfaced through the next packet's honest report rather than a failed landing. The corollary is contract rot: executable criteria that cite CLI shapes must be revalidated against the current binary when they run — a bare `loaf check` was valid at shaping on 0.2.15 and gone by execution on 0.2.16. The merge itself is part of the verified surface: an agent-authored commit from a restarted session shipped unsigned, and the required-signatures rule refused the squash until the tip was re-signed — delivery infrastructure catches structurally what nobody was watching for.

**Fresh-eyes review rounds converge by altitude.** Six review rounds on the pitch-entrypoint contract each found a different stratum — consistency drift, inverted assumptions, design gaps, implementation-safety holes, cross-surface contradictions — with zero reversed dispositions across the trail. Per-round disposition boards in the change's `reports/` made residuals explicit carries instead of silent losses, which is what made convergence observable and the stopping point defensible. This pattern feeds the review-convergence-loop Intent. The evidence-gate change (PR #147) repeated the pattern cross-model — six Codex review rounds over Grok-implemented fixes, 14 findings, zero reversed dispositions — so convergence holds when reviewer and implementer are different models entirely.

**Recovery instructions are part of the mechanism.** Three consecutive review rounds on the evidence-gate change (PR #147) caught remediation copy prescribing unexecutable recoveries — a rerun the clean-worktree check refuses, re-pointing a tag the guardrails had just proven absent. A gate whose refusal names an impossible next step is half a gate: recovery paths are code paths, designed (verify-then-restore, no persisted state), implemented, and proven by an end-to-end test of the full refuse → re-record → rerun loop.

**A defect class outlives any single fix; only a shared owner ends it.** The symlink-follow class was found and fixed three separate times on three surfaces — registry probe, version-file admission, candidate-artifact hashing — across evidence-gate review rounds 2–5, and recurrence ended only when a shared regular-file walk became the single owner every surface calls. Reviews should hunt the class, not the instance, and the fix for a class is a helper, not a patch per site.

**Release is separate from shipping.** A Change may land through one or more coherent pull requests; publishing a project version is a distinct operation over already-landed work. CI verifies reproducible outputs and must not silently repair the source branch.

**Evidence that lives in history shape dies in transit; evidence bound to content survives every merge strategy.** The 0.2.20 cut — the first stable candidate the cohort gate ever evaluated — refused a fully implemented, receipt-verified Change because the squash merge had rewritten the checkbox flips its execution grade scanned for, while the content-digest receipt sailed through the same squash untouched. The fix inverted the anchor (executed = flip in ancestry *or* fresh receipt with all boxes checked, ADR-027), and its sibling followed immediately: a self-carrying release has no version-file diff for the release-commit shape check, so that demand now relaxes only under the consistency proof an earlier guardrail already computed. The corollary is a testing pattern: the first stable release after any gate change is that gate's first real test — observed twice now (the alpha.16 evidence canary, then 0.2.20 twice in one day) — so expect the collision there and budget for it (PRs #154, #155).

**Version strings carry narrative weight, and the weight suppresses shipping.** Nineteen `2.0.0-alpha.N` releases in two months trained every cut to read as a statement about 2.0, so cuts stopped and field bugs waited on the daily-driver machine while new work stacked. Renumbering to plain major-zero (`0.2.20`, ADR-026) removed the narrative from the number, and the fix cadence resumed the same day the reset shipped. The scheme was refined mid-shape by the operator — renumbered continuity (history mapped 1:1 onto the new line) beat the pitched clean restart, because the changelog stayed the sole honest carrier of the past without pretending it didn't happen. The reset's own framing got the same treatment within two months: practice falsified the "stabilization epoch" minor (the patch slot absorbed subsystem-scale work and the version stopped carrying information), and the revision to arc-boundary X semantics landed as an in-place ADR-026 rewrite under the living-record convention — the lesson survived; the mechanism it first shipped with did not (rev. 2026-08-10).

## Current Priorities

> **Revision 2026-08-29 (LOAF-93):** vNext becomes the destination architecture. The shipped line stays stable and recoverable until verified cutover; its state, sync, and tracker machinery are evidence and migration input, not vNext dependencies.
>
> **Revision 2026-08-26 (LOAF-90):** Inserts substrate arc priorities. Supersedes: priorities listing only journal reliability and Loaf Flow without personal-substrate destination.
>
> **Revision 2026-08-26 (LOAF-90, schema 25):** LOAF-63/64/72 landed on main. Supersedes: "remaining mutable-core migration" and "LOAF-64 partial".
>
> **Revision 2026-08-26 (LOAF-62 closeout):** LOAF-68 children 84–86 shipped; sync refresh folds refs, worktrees, and verification. Grow-only union docs shipped with the LOAF-63 lock.

- **Isolated tracker-native vNext (LOAF-93).** Establish the kernel boundary first, then add tracker-native Flow, private continuity, private E2E sync, one-time migration, truthful harness support, and a reviewed cutover in dependency order.
- **No dual authority during transition.** Keep the legacy runtime and generated artifacts unchanged while vNext is built. Migrate once through a versioned archive; do not introduce ongoing dual reads, writes, reconciliation, or tracker mirrors.
- **Evidence-led cutover.** Cut over only after real tracker-connected dogfood, migration rehearsal, installed harness evidence, and independent correctness and architecture/security reviews. Preserve the legacy exporter, archive, and rollback route.

### Legacy Line Closeout Context

- **Personal memory substrate (LOAF-62).** Fact envelope, E2E crypto, sync server/client, attach-or-refuse, identity evidence, grow-only union, and mutable-core event facts shipped (LOAF-63–67, 71–72, 75–76). Writers append through the LOAF-71 chokepoint; the `events` table remains local archive (migration 0025). Sync refresh rebuilds spark/idea/handoff/release/ref/verification projections and worktree start bindings from facts (worktree paths may not exist on the receiving machine). Parent closeout is independent review and ship.
- **Refs + contracts cutover (LOAF-68).** Contract machinery keys to refs (LOAF-82), render-out (LOAF-83), branch/PR bootstrap (LOAF-85), flow-skill ref cutover (LOAF-86), and decision re-home (LOAF-84) shipped.
- **Dead state before sync (LOAF-69).** Delete zero-row schema (LOAF-79), demote document layer (LOAF-80), and inventory lock (LOAF-81) shipped — minimal sync contract enforced.
- **Scratchpad (LOAF-74).** CLI and closed kind set (LOAF-87) and server SSE/long-poll fanout (LOAF-88) shipped; close/prune (LOAF-89) shipped.
- **Journal reliability across installed targets.** Converge content-addressed installation, target adapter ownership, capability diagnosis, and isolated installed-runtime dogfood without mutating users' production state.
- **Loaf Flow completeness.** The entry stage shipped in v0.2.17: `/pitch` at both scales, captured-folder promotion in `loaf change init`, bootstrap series-prep under the landing matrix. Next is dogfood-driven fine-tuning (the deferred end-to-end pitch→shape proof, seam oiling), then the Intent-tracked follow-ons: the Arc decision-map, the review-convergence loop, and the skills audit. Existing spec and task records remain supported compatibility surfaces until deliberately converted.
- **The fix cadence, collecting its dividend.** 0.2.21 shipped the identity-fork repair (#159, ADR-028) and per-entry hook reconciliation (#161) as one combined cut — the first release since the reset to leave every harness current with zero refusals, and the first to exercise all four incident-born release mechanisms (release-PR flow, mechanical receipt gate, receipt-vouched cohort grading, changelog staging) without incident. The remaining queue, numbered at cut time per the revised ADR-026: the OpenCode session-start fix, the content-addressed skills store, and the skills audit.
- **Align the bump suggestion with arc evidence.** The revised ADR-026 (2026-08-10) makes X releases arc-boundary events — the cohort is the arc, derived rather than declared — but `suggestReleaseBump` still reads commit types, so until the realignment lands every cut states its bump explicitly. The realignment derives the suggestion from the same evidence the cohort gate already computes: a completed arc in the unreleased range suggests X, otherwise Y.
- **Evidence-driven target support.** Keep capability classifications conservative, version-pinned, and reproducible. Promote native behavior only after the installed target proves model-visible delivery; otherwise retain narrower runtime gating or an explicit fallback. The release command now refuses stale receipts mechanically after the artifact rebuild (PR #147); decoupling receipts from binary rebuilds remains the Intent-tracked structural fix.
- **Durable knowledge with low ceremony.** Preserve decisions, discoveries, and operational lessons where later work can retrieve them, while removing lifecycle machinery and planning vocabulary that do not carry product meaning.

## Strategic Tensions


**Personal continuity vs collaboration surfaces.** The substrate is private and E2E; team coordination stays on trackers and git promote paths. Building a shared memory layer in the substrate is explicitly out of scope for v1.

**Local-first versus multi-environment continuity.** A local replica must remain useful and trustworthy while private sync gives one operator continuity across environments. The vNext protocol is designed in its own slice; the current relay and schema are evidence, not a compatibility contract.

**Portability versus native leverage.** A shared skill should express the common contract, but each native adapter expands the compatibility and test surface. Native behavior earns its place only when it is observable and maintainable.

**Automation versus explainability.** Invisible automation is convenient until it fails. Ownership manifests, diagnostics, isolated smoke tests, and explicit degradation make failures inspectable without requiring hand-edited global configuration.

**Clean authority versus migration safety.** Tracker-native issues are the destination for shared work, while existing Changes, SQLite issues/tasks, and `SPEC-*` records remain migration inputs. A one-time verified import preserves access without retaining dual-read behavior or retired workflow assumptions.

**Durability versus noise.** The journal must retain information that changes future decisions, not duplicate lifecycle state or syntheses that can be derived from source, git, and pull requests.

## Open Questions


- **Master-key recovery (resolved LOAF-66):** Emergency Kit + surviving-machine re-key; no escrow.
- **Supersession collision UX (resolved LOAF-63 lock):** Grow-only union; no supersession ceremony in v1.

- Which target-native signals can provide durable event identity and trustworthy success evidence without coupling Loaf to unstable client internals?
- How should scheduled client-version discovery produce reviewable candidate evidence without automatically changing capability classifications?
- Where does target-native behavior materially improve the user experience enough to justify its maintenance and installed-test burden?
- How well does tracker-native Loaf Flow adoption hold outside Loaf itself, particularly across tracker products and harness-native connection surfaces?

---

## Changelog

- 2026-08-29 - Make the isolated tracker-native vNext reset the destination strategy, define private scratchpad scope, and reclassify the shipped line as evidence and migration input.
