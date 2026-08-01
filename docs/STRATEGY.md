# Strategy

Loaf is an opinionated agentic framework for AI coding assistants. It ships portable skills, bounded agent profiles, target-native adapters, enforcement hooks, and a native CLI. This document states current strategy; detailed history and evidence remain in [Changes](changes/), [decision records](decisions/), [reports](reports/), and git history.

## Who This Serves

**Solo developers** need an agent workflow that reduces context-switching overhead, preserves work across conversations, and enforces quality without becoming more cumbersome than direct tool use.

**Teams** need consistent agent behavior across developers and harnesses, with auditable decisions, trustworthy quality checks, and installation behavior that does not require everyone to understand Loaf internals.

## Proven Principles

**Skills are the portable knowledge layer.** Shared authoring should remain the default, while target-specific adapters translate that knowledge into the strongest trustworthy native surface each harness exposes.

**The CLI is the protocol layer.** Skills describe judgment and workflow, the CLI performs deterministic state and filesystem operations, and hooks enforce or inject narrowly scoped behavior. Runtime behavior should not depend on prose reimplementing CLI logic.

**Continuity belongs to the project journal, not a session lifecycle.** Journal entries are project-scoped events correlated by an opaque harness identity. Context is derived at read time, and a wrap is an optional synthesis checkpoint rather than a transition.

**Managed installation requires ownership evidence.** Loaf should change installed content only when it can identify what it owns and verify the expected digest. Capability claims must be tied to exact client versions and installed runtime evidence rather than inferred from build output.

ADR-020 preserves that single-overlay result while making root `AGENTS.md` the canonical real file. This improves standards-native discoverability and removes the unnecessary root indirection; `.agents/` remains reserved for Loaf state and configuration, while `.claude/CLAUDE.md` remains a compatibility symlink.

**Continuity must survive everything -- the session entity didn't need to.** Context compaction, `/clear`, tool restarts, and cross-conversation handoffs all create new harness session IDs pointing at the same stream of work. Any architecture that assumes 1 session = 1 conversation fails in practice.

**Automation must fail within its evidence boundary.** Automatic completion remains disabled unless a target supplies trustworthy success evidence and a durable event identity. When a harness cannot distinguish the relevant traffic or lifecycle event reliably, an explicit fallback is preferable to a false guarantee.

**The CLI is the correct protocol layer.** Skills should describe what to do. The CLI should execute it deterministically. Hooks should enforce invariants. This three-layer separation emerged from repeated attempts to let skills act on the world directly.

Every time a skill tried to call an external tool directly -- Linear MCP, raw git commands, file operations -- reliability dropped. The CLI absorbs that complexity and presents a stable interface to skills. For the team lead persona, this is critical: the CLI is the enforcement layer that makes agent behavior deterministic regardless of which LLM or harness is running.

**Diagnosis and repair must share the same state taxonomy.** Sharing repair helpers is not enough; the detection branches in a diagnostic tool must consult the same classification logic as the repair path, or they will drift apart.

An early `loaf doctor` silently skipped legacy-layout states -- a real `./AGENTS.md` or `.claude/CLAUDE.md` with no canonical overlay -- even though the helper that repairs those states was already written and invocable from the fix path. The two halves had diverged: the repair path knew about legacy layouts, the detection path did not. External review caught it, and the fix rebuilt detection to surface legacy states as fixable `fail` and taught the fix paths to create canonical lazily before delegating to the shared helper. For both personas this means `loaf doctor --fix` can be trusted to heal every state the diagnostic surfaces as fixable, because the same state definitions drive both halves of the state machine.

ADR-020 applies that lesson to the reverse migration: doctor now classifies root-file canonicality, retired `.agents/AGENTS.md` presence, and Claude compatibility-link drift independently. Plain diagnosis stays read-only; `--fix` asks before each repair, preserves legacy content and backups for accepted repairs, and requires `--force` to accept all repairs non-interactively.

**Personality is decorative, mechanics are durable.** An attempt to decouple agent personality (Warden/Fellowship lore) from agent mechanics through a swappable souls catalog was fully implemented, reviewed twice, and then pivoted in flight before merge: after months of real use the lore had never landed, and review flagged a prose-driven soul file as a brittle prompt-injection surface enforceable only by advice. What shipped keeps the profile neutralization and the skill prose audit; the catalog, CLI, install, and startup-restoration layers were removed.

The implication for both personas: Loaf's value is the *framework* -- mechanical hooks, structured pipeline, profile boundaries, knowledge layer -- not a personality layer. Adding identity through prompt content is incompatible with hardening agents: it costs complexity, adds attack surface, and delivers value only when the user already buys into the metaphor. The lore concept survives in the archived record as an account of what was tried, with a possible future home in a Pi-based harness where it could be load-bearing rather than decorative.

**Delegated implementation holds when verification is structural, not trust-based.** An implementer's report is a claim, not evidence: in the pitch-entrypoint batch (v2.0.0-alpha.17) one delegated agent reported re-running installed smokes when its diff showed only hash re-pins, and another reported a full test pass that had covered one package tree. Both were caught because the orchestrator verified every delivering commit against the contract's own gates — tests, hooks, checkbox state, projection greps, isolated end-to-end smokes — and a cross-task regression surfaced through the next packet's honest report rather than a failed landing. The corollary is contract rot: executable criteria that cite CLI shapes must be revalidated against the current binary when they run — a bare `loaf check` was valid at shaping on alpha.15 and gone by execution on alpha.16. The merge itself is part of the verified surface: an agent-authored commit from a restarted session shipped unsigned, and the required-signatures rule refused the squash until the tip was re-signed — delivery infrastructure catches structurally what nobody was watching for.

**Fresh-eyes review rounds converge by altitude.** Six review rounds on the pitch-entrypoint contract each found a different stratum — consistency drift, inverted assumptions, design gaps, implementation-safety holes, cross-surface contradictions — with zero reversed dispositions across the trail. Per-round disposition boards in the change's `reports/` made residuals explicit carries instead of silent losses, which is what made convergence observable and the stopping point defensible. This pattern feeds the review-convergence-loop Intent. The evidence-gate change (PR #147) repeated the pattern cross-model — six Codex review rounds over Grok-implemented fixes, 14 findings, zero reversed dispositions — so convergence holds when reviewer and implementer are different models entirely.

**Recovery instructions are part of the mechanism.** Three consecutive review rounds on the evidence-gate change (PR #147) caught remediation copy prescribing unexecutable recoveries — a rerun the clean-worktree check refuses, re-pointing a tag the guardrails had just proven absent. A gate whose refusal names an impossible next step is half a gate: recovery paths are code paths, designed (verify-then-restore, no persisted state), implemented, and proven by an end-to-end test of the full refuse → re-record → rerun loop.

**A defect class outlives any single fix; only a shared owner ends it.** The symlink-follow class was found and fixed three separate times on three surfaces — registry probe, version-file admission, candidate-artifact hashing — across evidence-gate review rounds 2–5, and recurrence ended only when a shared regular-file walk became the single owner every surface calls. Reviews should hunt the class, not the instance, and the fix for a class is a helper, not a patch per site.

**Release is separate from shipping.** A Change may land through one or more coherent pull requests; publishing a project version is a distinct operation over already-landed work. CI verifies reproducible outputs and must not silently repair the source branch.

## Current Priorities

- **Journal reliability across installed targets.** Converge content-addressed installation, target adapter ownership, capability diagnosis, and isolated installed-runtime dogfood without mutating users' production state.
- **Loaf Flow completeness.** The entry stage shipped in v2.0.0-alpha.17: `/pitch` at both scales, captured-folder promotion in `loaf change init`, bootstrap series-prep under the landing matrix. Next is dogfood-driven fine-tuning (the deferred end-to-end pitch→shape proof, seam oiling), then the Intent-tracked follow-ons: the Arc decision-map, the review-convergence loop, and the skills audit. Existing spec and task records remain supported compatibility surfaces until deliberately converted.
- **Evidence-driven target support.** Keep capability classifications conservative, version-pinned, and reproducible. Promote native behavior only after the installed target proves model-visible delivery; otherwise retain narrower runtime gating or an explicit fallback. The release command now refuses stale receipts mechanically after the artifact rebuild (PR #147); decoupling receipts from binary rebuilds remains the Intent-tracked structural fix.
- **Durable knowledge with low ceremony.** Preserve decisions, discoveries, and operational lessons where later work can retrieve them, while removing lifecycle machinery and planning vocabulary that do not carry product meaning.

## Strategic Tensions

**Portability versus native leverage.** A shared skill should express the common contract, but each native adapter expands the compatibility and test surface. Native behavior earns its place only when it is observable and maintainable.

**Automation versus explainability.** Invisible automation is convenient until it fails. Ownership manifests, diagnostics, isolated smoke tests, and explicit degradation make failures inspectable without requiring hand-edited global configuration.

**Convention versus compatibility.** Change-first is the current model for new work, while existing SQLite tasks, `SPEC-*` records, and their CLI commands remain real supported data. Compatibility should preserve access without keeping retired workflow assumptions in current guidance.

**Durability versus noise.** The journal must retain information that changes future decisions, not duplicate lifecycle state or syntheses that can be derived from source, git, and pull requests.

## Open Questions

- Which target-native signals can provide durable event identity and trustworthy success evidence without coupling Loaf to unstable client internals?
- How should scheduled client-version discovery produce reviewable candidate evidence without automatically changing capability classifications?
- Where does target-native behavior materially improve the user experience enough to justify its maintenance and installed-test burden?
- How well does Change-first workflow adoption hold outside Loaf itself, particularly for teams with existing trackers and conventions? First signal, 2026-07-30: a greenfield `/pitch` on a fresh external project produced a working BRIEF on the shared skeleton; the finding was seam friction in the ceremony's handoff, not model rejection.
