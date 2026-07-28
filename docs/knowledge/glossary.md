---
type: glossary
topics:
  - glossary
last_reviewed: '2026-07-28'
---
## Canonical Terms

### Skill

A domain-knowledge unit following the Agent Skills standard. Loaf's universal knowledge layer — distributed across all build targets without target-specific code.

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

### Task Packet

A tasks/TASK-NNN-slug.md file: a self-sufficient delegation brief with objective, scope boundaries, context pointers, checkbox steps, and verification. Numbered locally to its owning change; a task is a commit, not a PR. Committed unchecked before execution so checkbox flips in delivering commits are the evidence.

_Avoid_: ticket, issue, task entity

### Release Cohort

All changes declaring the same target_release. The cohort is the arc — derived, never declared as a graph. Cutting that version stable requires every member executed at flip grade and receipt-verified; a retarget is a reviewable diff, surfaced and never blocked.

_Avoid_: arc, milestone, train

### Flip Grade

The gating tier of execution provenance: a commit whose diff carries a true unchecked→checked checkbox transition (same hunk, same normalized label, outside code fences) in a change's tasks/ plus a path outside docs/changes/. Path grade (task-file edit + outside path, no transition required) feeds display states only.

_Avoid_: checkbox count, completion percentage

### Verification Receipt

receipts/verify.json — the committed cache of loaf change verify: criteria digest, verified commit, cwd, and per-criterion command, exit, output digest, and ok. Required current and all-passing for cohort members at stable finalization; the gate never runs criteria itself (ADR-023).

_Avoid_: credential, attestation, proof-of-work

### Derived State Ladder

The display states computed from artifacts and history — captured → shaped → executable → executing → complete, plus verified for cohort members. Shared by list, show, and check; nothing stores a status anywhere.

_Avoid_: workflow status, lifecycle stage

## Candidates


## Relationships


## Flagged ambiguities
