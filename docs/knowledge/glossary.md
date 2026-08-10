---
type: glossary
topics:
  - glossary
last_reviewed: 2026-08-10
---
## Canonical Terms

### Skill

A domain-knowledge unit following the Agent Skills standard. Loaf's universal knowledge layer — distributed across all build targets without target-specific code. Skills under `content/skills/` are auto-discovered at build time; `config/hooks.yaml` registers hook instances with an owning skill, never the skill roster itself.

_Avoid_: module, knowledge file, doc

### Target

A build output destination: claude-code, opencode, cursor, codex, amp. Each target has its own native builder in `internal/cli/build_{target}.go`.

_Avoid_: platform, backend, tool

### Sidecar

A target-specific YAML file that extends a SKILL.md with target-only fields (e.g., SKILL.claude-code.yaml for user-invocable, argument-hint). Merged into the build output for that target only.

_Avoid_: extension, override, plugin file

### Shared Template

A markdown template at content/templates/ that is distributed to multiple skills at build time per the shared-templates registration in config/targets.yaml. Examples: session.md, adr.md, grilling.md.

_Avoid_: common template, global template

### Change

The bounded-work unit: a folder under docs/changes/YYYYMMDD-slug/ holding change.json (identity), role-named narrative (shape.md required; brief/plan/design optional), tasks/, research/, and reports/. The slug is the identity; one change rides one PR (ADR-022).

_Avoid_: spec (reserved for .agents/specs/ records), plan (reserved for plan.md), CR

### Brief

The problem-space document for a Change (`brief.md`) or project (`docs/BRIEF.md`). Authored by `/pitch` (or seeded at capture); may accrete parked problem-space concepts until shaping; freezes when `shape.md` exists; superseded by the contract; never mechanically load-bearing. Shared skeleton: problem, who has it, current alternatives, value proposition, constraints, sequencing, sources, open questions (ADR-025).

_Avoid_: pitch.md, pseudo-shape, solution design

### Pitch

The human-invoked problem-discovery ceremony (`/pitch`). Grills the problem register and authors a brief at change or project scale; never writes `shape.md`, never seeds `tasks/`, never auto-runs shape or bootstrap. Agents never initiate a pitch (ADR-025).

_Avoid_: explore-as-front-door, brainstorm-as-front-door, automatic pitching

### Loaf Flow

The ceremony pipeline: pitch → shape → implement → ship → release, at change scale and project scale. Pitch owns problem-space; shape owns solution-space. Explore and brainstorm sit as agent-side techniques inside the flow, not as user slash entry points. Operating account: [loaf-flow.md](loaf-flow.md) and [work-model.md](work-model.md).

_Avoid_: mandatory ceremony for every tiny fix (pitch is optional at change scale)

### Task Packet

A tasks/TASK-NNN-slug.md file: a self-sufficient delegation brief with objective, scope boundaries, context pointers, checkbox steps, and verification. Numbered locally to its owning change; a task is a commit, not a PR. Committed unchecked before execution so checkbox flips in delivering commits are the evidence.

_Avoid_: ticket, issue, task entity

### Capture Promotion

`loaf change init <slug>` completing a capture-only folder in place: brief.md and change.json values preserved verbatim, shape.md and seeded tasks/ instantiated. Duplicate rejection remains for fully materialized folders (ADR-025, Decision 12 of the pitch-entrypoint change).

_Avoid_: skill-side template copy, new CLI verb for promote

### Release Cohort

All changes declaring the same target_release. The cohort is the **target-version bucket** — derived, never declared as a graph. Cutting that version stable requires every member executed at flip grade and receipt-verified; a retarget is a reviewable diff, surfaced and never blocked.

_Avoid_: milestone, train, roadmap planner

### Flip Grade

The gating tier of execution provenance: a commit whose diff carries a true unchecked→checked checkbox transition (same hunk, same normalized label, outside code fences) in a change's tasks/ plus a path outside docs/changes/. Path grade (task-file edit + outside path, no transition required) feeds display states only.

_Avoid_: checkbox count, completion percentage

### Verification Receipt

receipts/verify.json — the committed cache of loaf change verify: criteria digest, verified commit, cwd, and per-criterion command, exit, output digest, and ok. Required current and all-passing for cohort members at stable finalization; the gate never runs criteria itself (ADR-023).

_Avoid_: credential, attestation, proof-of-work

### Derived State Ladder

The display states computed from artifacts and history — captured → shaped → executable → executing → complete, plus verified for cohort members. Shared by list, show, and check; nothing stores a status anywhere. Brief-only folders derive captured regardless of brief richness.

_Avoid_: workflow status, lifecycle stage, pitched-state machinery

### Arc

A release cohort viewed as narrative: the linked Changes that complete together into one X release. Synonym of Release Cohort (ADR-022: the cohort is the arc); a standalone unpinned Change is an arc of one.

## Candidates


## Relationships


## Flagged ambiguities

