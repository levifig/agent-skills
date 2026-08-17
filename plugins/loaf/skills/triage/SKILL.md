---
name: triage
description: >-
  Processes the local intake queue from loaf intake list: unresolved sparks,
  ideas, and brainstorms. Use when the user asks "triage", "process my backlog",
  or wants dispositions chosen across intake items. Produces explicit
  dispositions: discard, retain as spark/idea, file as backlog issue, resume
  exploration, resolve, hand to pitch, or hand to shape (issue preparation). Not
  for reading a single known item (use loaf issue show, loaf spark show, loaf
  idea show, or journal directly), capturing new ideas (use idea), problem
  discovery (use pitch), or bounding one chosen direction (use shape).
user-invocable: true
version: 0.3.0
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
- Leftover kinds
- Guardrails
- Related Skills

## Critical Rules

- Log invocation first: `loaf journal log "skill(triage): <trigger or scope>"`
- Read the queue with `loaf intake list --json`; it projects every unresolved logical item exactly once with its provenance and exact read command.
- Present everything before acting — the user decides each disposition; never auto-promote, auto-discard, or auto-convert.
- The CLI never classifies: you and the user interpret each item; commands perform the chosen operation deterministically.
- Capture, issue, and Exploration are different claims: a spark or idea is retained material, a backlog issue is deliberately tracked work, an Exploration is an inquiry. Do not conflate them to save a step.
- One pass through the queue — don't loop or re-present items.
- **Two doors into issue work:** items needing problem discovery hand to pitch; well-understood directions hand to shape (issue preparation). Worth keeping but not ready for either door files as a backlog issue (`loaf issue new "<title>" --status backlog`, optional `--parent`, optional `loaf issue bucket`). Triage never runs `loaf issue start`, never opens PRs, and never invents Git artifacts.

## Verification

- Every presented item has a recorded disposition or an explicit "leave for next triage".
- Filed directions exist as backlog issues (`loaf issue list --status backlog`) and no longer appear in `loaf intake list` once their captures are resolved or archived.
- Discards are resolved or archived through their own commands and no longer appear in `loaf intake list`.
- No Linear or tracker operation was attempted; publication is a later concern outside triage.

## Quick Reference

| Item kind | Comes from | Typical dispositions |
|-----------|-----------|----------------------|
| spark | `loaf spark capture --scope <scope> --text <text>` | discard, retain, promote to idea, file as backlog issue, resume exploration, resolve, hand to pitch, hand to shape |
| idea | `loaf idea capture --title "<title>"` | archive, retain, file as backlog issue, resume exploration, resolve, hand to pitch, hand to shape |
| brainstorm | `loaf brainstorm capture` | archive, retain, promote to idea, file as backlog issue, resume exploration, hand to pitch, hand to shape |

## Process

1. **Scan.** Run `loaf intake list --json`. Summarize counts by kind, then list each item with its title, disposition or status, and read command.
2. **Read on demand.** Use each item's `read_command` verbatim when the user wants detail before deciding. If a read command fails, record the exact command and error in the summary as `unreadable`, make no semantic disposition for that item, continue the pass, and offer a factual diagnostic step (`loaf state doctor --json`) afterward. Never persist unreadable as a status.
3. **Decide per item.** Present the applicable dispositions and perform exactly the chosen one.
4. **Summarize.** Report what was discarded, retained, filed as backlog issues, resumed as explorations, resolved, or handed to pitch or shape, and journal notable decisions.

## Dispositions

- **Discard** — ideas and brainstorms: `loaf idea archive <ref> --reason <r>` or `loaf brainstorm archive <ref> --reason <r>`. A spark is resolved against the entity that addressed it (`loaf spark resolve <ref> --by <entity> --reason <r>`); a pure dead-end spark currently has no deterministic discard operation — leave it retained, journal the judgment, and never invent a resolving entity.
- **Retain as spark/idea** — do nothing to leave the capture open, or promote into the other capture primitive: capture the idea first (`loaf idea capture --title "..."`), then `loaf spark promote <spark> --to-idea <idea>` or `loaf brainstorm promote <brainstorm> --to-idea <idea>`. Open captures resurface next triage.
- **File as backlog issue** — two steps so the direction appears once. Create the issue, then close the capture against it:

  ```bash
  loaf issue new "<title>" --status backlog [--parent <ref>] [--kind delivery|decision] [--fog <text>] [--body <text>]
  loaf issue bucket <issue-ref> now|next|later    # optional; labels only, never a constraint
  loaf spark resolve <capture-ref> --by <issue-ref>
  # or: loaf idea resolve <capture-ref> --by <issue-ref>
  # brainstorms: loaf brainstorm archive <ref> --reason "filed as <issue-ref>"
  ```

  Use `--kind decision` when filing a sharp question. Copy still-unsharp questions into `--fog` (create-time only). `--parent` nests under an existing issue; omit it for a different problem.
- **Resume exploration** — agent technique for genuinely undecided directions (Explorations and checkpoints); not a user slash entry. Prefer pitch when the human needs problem discovery first, then reach for explore from inside that work if still undecided. Resume with `loaf exploration context <ref>` when a named Exploration already exists.
- **Resolve** — the capture is already represented elsewhere. `loaf spark resolve <ref> --by <entity> --reason <r>` or `loaf idea resolve <ref> --by <entity>`. History is never rewritten.
- **Hand to pitch** — items needing problem discovery hand to pitch. Resolve the capture against the issue once one exists (`loaf spark resolve` / `loaf idea resolve --by <issue-ref>` / archive the brainstorm with that issue as the reason).
- **Hand to shape** — when a direction is already well-understood and ready for bounded delivery, hand it to shape for issue preparation. Triage never writes definition-of-done criteria, never runs `loaf issue check`, and never creates branches or worktrees.

## Leftover kinds

`loaf intake list` may still include `intent` and `legacy_deferral` items. Do not create new `intent` rows. Treat leftover directions like any other capture: file a backlog issue if worth keeping, or leave them for a later pass. Do not offer conversion commands that recreate the old tracked/deferred row.

## Guardrails

1. **User decides every disposition** — present, don't decide.
2. **Batch presentation, individual decisions** — show the full queue, then process one item at a time.
3. **Log everything** — no silent discards, promotions, or conversions.
4. **Filed is not forgotten** — backlog issues remain on `loaf issue list` and may appear on `loaf issue frontier` until their status changes. Buckets are labels only.

## Related Skills

- **idea** — capture a new idea (fast, minimal friction)
- **pitch** — problem-discovery ceremony for items that need framing before shape
- **explore** — agent technique for divergent inquiry with portable checkpoints
- **shape** — develop a well-understood direction into a bounded issue
- **housekeeping** — flags stale artifacts; does not choose dispositions
