---
name: triage
description: >-
  Processes the local intake queue from loaf intake list: unresolved sparks,
  ideas, brainstorms, tracked and deferred Intents, and unmigrated legacy
  deferrals. Use when the user asks "triage", "process my backlog", or wants
  dispositions chosen across intake items. Produces explicit dispositions:
  discard, retain, track as Intent, defer, resume, resolve, explore, or hand to
  shape. Not for reading a single known item (use loaf intent show or journal
  directly), capturing new ideas (use idea), divergent inquiry (use explore), or
  bounding one chosen direction (use shape).
user-invocable: true
version: 2.0.0-alpha.11
---

# Triage

Process the intake queue. Triage is the public funnel where captured material meets judgment: you present facts, the user chooses each disposition, and the CLI performs exactly what was chosen.

**Input:** $ARGUMENTS

---

## Contents
- Critical Rules
- Verification
- Quick Reference
- Process
- Dispositions
- Legacy Deferrals
- Guardrails
- Related Skills

## Critical Rules

- Log invocation first: `loaf journal log "skill(triage): <trigger or scope>"`
- Read the queue with `loaf intake list --json`; it projects every unresolved logical item exactly once with its provenance and exact read command.
- Present everything before acting — the user decides each disposition; never auto-promote, auto-discard, or auto-convert.
- The CLI never classifies: you and the user interpret each item; commands perform the chosen operation deterministically.
- Capture, Intent, and Exploration are different claims: a spark or idea is retained material, a tracked Intent is deliberately tracked work, a deferral is an Intent disposition with an immutable payload, an Exploration is an inquiry. Do not conflate them to save a step.
- One pass through the queue — don't loop or re-present items.

## Verification

- Every presented item has a recorded disposition or an explicit "leave for next triage".
- Tracked and deferred choices exist as Intents with the expected derived disposition (`loaf intent list`).
- Discards are resolved or archived through their own commands and no longer appear in `loaf intake list`.
- No Linear or tracker operation was attempted; publication is a later concern outside this Change.

## Quick Reference

| Item kind | Comes from | Typical dispositions |
|-----------|-----------|----------------------|
| spark | `loaf spark capture` moments | discard, promote to idea, track as Intent |
| idea | `/idea` capture | archive, explore, track as Intent, hand to `/shape` |
| brainstorm | archived divergent sessions | archive, explore, promote |
| intent (tracked) | `loaf intent create` | keep tracking, defer, resolve, explore, hand to `/shape` |
| intent (deferred) | `loaf intent defer` or adapter | resume, resolve, leave deferred |
| legacy_deferral | pre-conversion `journal defer` | read, then optionally convert (see Legacy Deferrals) |

## Process

1. **Scan.** Run `loaf intake list --json`. Summarize counts by kind, then list each item with its title, disposition or status, and read command.
2. **Read on demand.** Use each item's `read_command` verbatim when the user wants detail before deciding. If a read command fails, record the exact command and error in the summary as `unreadable`, make no semantic disposition for that item, continue the pass, and offer a factual diagnostic step (`loaf state doctor --json`) afterward. Never persist unreadable as a status.
3. **Decide per item.** Present the applicable dispositions and perform exactly the chosen one.
4. **Summarize.** Report what was discarded, retained, tracked, deferred, resumed, resolved, or handed onward, and journal notable decisions.

## Dispositions

- **Discard** — ideas and brainstorms: `loaf idea archive <ref> --reason <r>` or `loaf brainstorm archive <ref> --reason <r>`. A spark is resolved against the entity that addressed it (`loaf spark resolve <ref> --by <entity> --reason <r>`); a pure dead-end spark currently has no deterministic discard operation — leave it retained, journal the judgment, and never invent a resolving entity.
- **Retain as capture** — do nothing; open captures resurface next triage.
- **Track as Intent** — two steps: create the Intent with the capture as its source, then close the capture against it so the direction appears once. `loaf intent create --title <t> --body <self-sufficient body> --from <capture-ref>`, then `loaf spark resolve <capture-ref> --by <intent-ref>` or `loaf idea resolve <capture-ref> --by <intent-ref>` (brainstorms: `loaf brainstorm archive <ref> --reason "tracked as <intent-ref>"`).
- **Defer** — an existing Intent: `loaf intent defer <ref> --why <w> --boundary <b> --trigger <t> --operation-id <key>`; a new deferred direction needs the full skeleton: `loaf intent create --title <t> --body <b> --disposition deferred --why <w> --boundary <bd> --trigger <tr> --operation-id <key> [--from <source-ref>]`.
- **Resume** — `loaf intent resume <ref> --reason <why now>`; appends a tracked disposition linked to the deferral it supersedes.
- **Resolve** — `loaf intent resolve <ref> --reason <outcome>`; history is never rewritten.
- **Explore** — hand the item to `/explore`, linking it with `--from` when the Exploration is created.
- **Shape** — when a direction is ready for bounded delivery, hand it to `/shape`; triage never creates Changes, branches, or worktrees.

## Legacy Deferrals

Items of kind `legacy_deferral` are pre-conversion `journal defer` records. They stay visible and readable until the explicit, backup-first conversion is run; nothing disappears while migration is pending. When the user wants them converged, offer `loaf state migrate deferrals --dry-run` to preview the project-specific manifest and `--apply` only with explicit consent — apply verifies a whole-database backup first and preserves every legacy row.

## Guardrails

1. **User decides every disposition** — present, don't decide.
2. **Batch presentation, individual decisions** — show the full queue, then process one item at a time.
3. **Log everything** — no silent discards, promotions, or conversions.
4. **Deferred is not forgotten** — deferred Intents remain active truth in `loaf journal context` until resumed or resolved.

## Related Skills

- **idea** — capture a new idea (fast, minimal friction)
- **explore** — divergent inquiry with portable checkpoints
- **shape** — develop a chosen direction into a bounded Change
- **housekeeping** — flags stale artifacts; does not choose dispositions
