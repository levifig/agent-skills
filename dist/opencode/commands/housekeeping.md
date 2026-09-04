---
description: >-
  Reviews temporary reports, private continuity artifacts, and Git workspace
  hygiene without creating another work system. Use when the user asks to clean
  up, tidy `.agents/`, or decide what to retain. Produces artifact-by-artifact
  recommendations and performs only explicitly approved dispositions.
subtask: false
version: 0.5.0
---

# Housekeeping

Remove noise without erasing useful knowledge or inventing lifecycle state. The tracker remains canonical for shared work; the project journal remains append-only continuity.

## Contents

- Critical Rules
- Verification
- Quick Reference
- Report Review
- Topics

## Critical Rules

- As the first action, run `loaf journal log "skill(housekeeping): <scope or trigger>"` against the current private local journal. If the write fails, report the failure and continue only when cleanup can safely proceed.
- Review every report individually. Never sample, infer one report's value from another, or recommend a directory-wide action without reading each file.
- Inspect the report's contents, references, consumers, Git state, and whether its durable conclusions already live in the tracker, journal, an ADR, or project documentation.
- Present a concrete recommendation and rationale for every report before mutation.
- Require explicit user approval for every deletion or move. A batch approval is valid only when it unambiguously names every artifact and disposition.
- Preserve the project journal; it is append-only and never a cleanup target.
- Read shared work state from the configured native tracker through `project-management/v1`. Never infer canonical workflow state from reports, local rows, branches, or worktrees.
- Use reversible, path-specific operations where possible. Never overwrite a destination or remove an occupied or dirty worktree.
- After approved actions, verify the exact source and destination paths and report partial failures honestly.

## Verification

- Every discovered report was read and received an individual recommendation.
- No report was deleted or moved without explicit user approval.
- Durable knowledge was extracted before an approved delete when the recommendation required it.
- A report moved to `docs/reports/` has perennial value as a whole, not merely one reusable conclusion.
- The journal and canonical tracker were not treated as cleanup mirrors.
- Final output lists what remained, what changed, verification, and unresolved risk.

## Quick Reference

| Artifact | Canonical meaning | Housekeeping boundary |
|----------|-------------------|-----------------------|
| `.agents/reports/*.md` | Temporary skill output | Review individually and recommend one disposition |
| `docs/reports/*.md` | Deliberately promoted perennial report | Normal documentation maintenance; do not demote automatically |
| Project journal | Append-only private continuity | Read for context; never delete or archive |
| Native tracker work | Canonical shared work | Read or mutate through the selected provider |
| Git worktrees | Repository-native execution spaces | Inspect branch, dirtiness, reachability, and occupancy before proposing removal |

## Report Review

For every file matching `.agents/reports/*.md`, recommend exactly one of these dispositions:

1. **Leave it in place.** It still has an active near-term consumer or unresolved purpose.
2. **Extract durable conclusions, then delete.** Some knowledge matters, but the report as a whole does not. Name the canonical destination for each conclusion and perform extraction before deletion.
3. **Delete it.** Its purpose is complete and no unique evidence or conclusion remains useful.
4. **Move the report to `docs/reports/`.** The report itself has perennial value, such as an audit, durable benchmark, incident analysis, or evidence record readers will revisit.

Reports have no universal status, identifier, database row, or archive directory. Age alone does not decide value. Housekeeping proposes; the user approves; only then may the agent apply the exact disposition.

## Topics

| Topic | Reference | Use When |
|-------|-----------|----------|
| Report authority | [Authority model](../loaf-reference/references/authority-model.md#temporary-reports) | Confirming why reports have no CLI, state, sync, or shared schema |
| Tracker operations | [Project management](../project-management/SKILL.md) | Reading or changing canonical shared work during housekeeping |
