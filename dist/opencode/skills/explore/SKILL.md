---
name: explore
description: >-
  Conducts divergent inquiry as a durable Exploration with portable checkpoints,
  conversation provenance, and backlog-issue dispositions that survive
  compaction and harness changes. Agent technique — not a user entry point:
  route "explore this" and similar user asks to pitch; use this technique from
  inside pitch or other agent work when the direction is genuinely undecided, or
  when resuming a named Exploration. Produces Exploration records, portable
  checkpoints, and backlog issues for crystallized directions; Exploration
  machinery and the four-field checkpoint contract stay intact. Not for evidence
  gathering on a known question (use research), continuing implementation (use
  implement), processing the intake queue (use triage), shaping a bounded issue
  (use shape), problem discovery (use pitch), or quick capture (use idea).
version: 0.2.21
---

# Explore

Divergent inquiry with durable continuity. An Exploration is a relational identity over immutable checkpoints and provenance — it has no status, no owner, and no lifecycle to maintain. Resuming means reading portable context and appending new facts, never toggling state.

**Input:** $ARGUMENTS

---

## Contents
- Critical Rules
- Verification
- Quick Reference
- Process
- Checkpoint Discipline
- Resumption
- Parking a direction
- Techniques
- Related Skills

## Critical Rules

- Log invocation first: `loaf journal log "skill(explore): <topic or exploration ref>"`
- You choose what an Exploration means and when to checkpoint; the CLI validates and performs the operation you request. Never expect the CLI to classify or decide for you.
- Checkpoint before the context window gets hostile: every checkpoint must carry all four portable fields — purpose, conclusions, unresolved, next action — each self-sufficient without this conversation.
- A conversation handle or log path is provenance, never context. Presence of handles does not make an Exploration resumable; only a portable checkpoint does.
- Capture crystallized directions as backlog issues (`loaf issue new "<title>" --status backlog`); park remaining unsharp questions on that issue with `--fog`. Never leave a substantial direction only in prose.
- Never create Git artifacts, branches, or worktrees from Explore; when a direction is ready for problem discovery hand it to pitch, and when it is ready for bounded delivery hand it to shape (issue preparation).
- Never store transcripts, prompts, or tool output in checkpoints or items; curate semantic context instead.
- Not a user slash front door — human "explore this" / "where do I start" routes to pitch.

## Verification

- The Exploration exists with `portable_context_present: true` after the first checkpoint (`loaf exploration list`).
- `loaf exploration context <ref> --json` returns the four-field core whole, and a fresh reader could identify the next action from it alone.
- Crystallized directions exist as backlog issues (`loaf issue list --status backlog`); issue aliases named in the checkpoint match those rows.
- Conversation provenance, when recorded, carries harness and locality facts without any transcript content.

## Quick Reference

| Operation | Command |
|-----------|---------|
| Start an inquiry | `loaf exploration create --title <title> [--from <source>]...` |
| Checkpoint | `loaf exploration checkpoint <ref> --purpose <p> --conclusions <c> --unresolved <u> --next <n> [--item candidate:<text>]... [--operation-id <key>]` |
| Resume elsewhere | `loaf exploration context <ref> --json` |
| File a direction | `loaf issue new "<title>" --status backlog [--parent <ref>] [--kind delivery\|decision] [--fog <text>] [--body <text>]` |
| Optional bucket | `loaf issue bucket <ref> now\|next\|later\|none` |
| Record provenance | `loaf conversation create --title <label>` then `loaf conversation handle add <id> --harness <h> --handle <opaque-id> [--locality <scope>] [--log-ref <path>]` |
| Associate conversation | `loaf exploration conversation add <exploration> <conversation-id>` |

`--from` on create accepts journal entries, handoffs, reports, and findings. It does not accept issue, spark, or idea refs — name those in the checkpoint and in the issue body instead. Buckets are labels only and are never read as a constraint. `fog` is writeable only at create.

## Process

1. **Orient.** If the input names an existing Exploration, run `loaf exploration context <ref>` and continue from its recommended next action. Otherwise check `loaf exploration list` before creating a duplicate inquiry.
2. **Create or resume.** New inquiries get `loaf exploration create` with `--from` links to the journal entries, reports, findings, or handoffs that motivated them.
3. **Diverge.** Expand the option space before judging it. Use the brainstorm stance (below), research, scouting, prototypes, or spikes as the question demands.
4. **Capture as you go.** Incidental thoughts become sparks (`loaf spark capture --scope <scope> --text <text>`); explicit propositions become ideas (`loaf idea capture --title "..."`); directions worth keeping become backlog issues. Resolve the capture against the issue so the direction appears once: `loaf spark resolve <ref> --by <issue-ref>` or `loaf idea resolve <ref> --by <issue-ref>`.
5. **Checkpoint.** At every meaningful plateau — and always before ending a session — append a checkpoint with the four portable fields and optional `candidate:`/`evidence:` items. Name any filed issue aliases in conclusions or next.
6. **Record provenance when useful.** Machine-local conversation handles and log locators help forensic navigation later; add them explicitly, and never infer identity from the current session.

## Checkpoint Discipline

The four fields are the portable contract; each is capped at 4096 UTF-8 bytes and oversize input is rejected, never truncated:

- **purpose** — the current framing of the inquiry, restated so a stranger understands what is being explored and why.
- **conclusions** — constraints and conclusions established so far, including rejected options and why they fell.
- **unresolved** — the open question or decision the inquiry currently turns on.
- **next** — the recommended next action, concrete enough for a fresh agent to execute without this conversation.

Larger detail belongs in ordered `--item candidate:` and `--item evidence:` entries or in related reports, not crammed into the core fields. When filing an issue, copy still-unsharp questions into `--fog`; they will not be editable on the issue after create.

## Resumption

A new conversation, harness, or machine resumes with `loaf exploration context <ref> --json`: the portable core returns whole, and each optional layer reports counts, truncation, and its exact expansion command. Source handles appear with their last observed availability; treat unavailable ones as lost without ceremony — the checkpoint is the context. If `portable_context_present` is false, the Exploration was never checkpointed: rebuild understanding from linked sources, then write the missing checkpoint first.

Before continuing, inspect issue aliases named in the checkpoint. If an issue this inquiry was developing is now done, cancelled, or duplicate, do not silently reopen it: acknowledge the resolution, and if the checkpoint's next action still matters, file a successor backlog issue and record why in its body. Continued evidence gathering that serves no open issue should say so in its next checkpoint.

## Parking a direction

An Exploration is never paused or closed — it has no lifecycle to transition. When the user wants to park or set aside the inquiry, do two concrete acts: checkpoint the current state honestly, then file the direction it was developing as a backlog issue (`loaf issue new "<title>" --status backlog`, optional `--parent`, `--fog` for remaining unsharp questions, optional `loaf issue bucket <ref> later`). The issue is the revisit surface; the Exploration simply waits, resumable from its checkpoint.

## Techniques

Brainstorm's full divergent stance lives inside Explore: generate options before judging, connect to VISION/STRATEGY context, document discarded options, set boundaries on exploration time. Scout, research, prototype, and spike remain subordinate techniques invoked from whatever stage needs them — none of them owns lifecycle.

## Related Skills

- **pitch** — human problem-discovery front door; route user entry here; reach for explore from inside pitch when still undecided
- **triage** — processes the intake queue and may disposition items toward a backlog issue, pitch, shape, or agent-side explore
- **shape** — prepares a well-understood direction as a bounded issue
- **research** — evidence gathering for a known question, usable inside an Exploration
- **idea** — quick capture without inquiry
