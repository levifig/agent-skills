---
name: explore
description: >-
  Conducts divergent inquiry as a durable Exploration: portable checkpoints, conversation provenance, and Intent capture that survive compaction and harness changes. Use when the direction is genuinely undecided ("explore this", "we don't know which approach yet") or when resuming a named Exploration. Produces Exploration records, portable checkpoints, and tracked or deferred Intents. Not for evidence gathering on a known question (use research), continuing implementation or delivery work (use implement), processing the intake queue (use triage), shaping a bounded Change (use shape), or quick capture (use idea).
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
- Techniques
- Related Skills

## Critical Rules

- Log invocation first: `loaf journal log "skill(explore): <topic or exploration ref>"`
- You choose what an Exploration means and when to checkpoint; the CLI validates and performs the operation you request. Never expect the CLI to classify or decide for you.
- Checkpoint before the context window gets hostile: every checkpoint must carry all four portable fields — purpose, conclusions, unresolved, next action — each self-sufficient without this conversation.
- A conversation handle or log path is provenance, never context. Presence of handles does not make an Exploration resumable; only a portable checkpoint does.
- Capture crystallized directions as Intent (`loaf intent create`), deferred bodies with `--disposition deferred`; never leave a substantial direction only in prose.
- Never create Git artifacts, branches, worktrees, or Changes from Explore; when a direction is ready for bounded delivery, hand it to `/shape`.
- Never store transcripts, prompts, or tool output in checkpoints or items; curate semantic context instead.

## Verification

- The Exploration exists with `portable_context_present: true` after the first checkpoint (`loaf exploration list`).
- `loaf exploration context <ref> --json` returns the four-field core whole, and a fresh reader could identify the next action from it alone.
- Crystallized directions exist as Intents with derived dispositions (`loaf intent list`).
- Conversation provenance, when recorded, carries harness and locality facts without any transcript content.

## Quick Reference

| Operation | Command |
|-----------|---------|
| Start an inquiry | `loaf exploration create --title <title> [--from <intent-or-source>]...` |
| Checkpoint | `loaf exploration checkpoint <ref> --purpose <p> --conclusions <c> --unresolved <u> --next <n> [--item candidate:<text>]... [--operation-id <key>]` |
| Resume elsewhere | `loaf exploration context <ref> --json` |
| Track a direction | `loaf intent create --title <t> --body <b> --from <source>...` |
| Defer a direction | `loaf intent create --title <t> --body <b> --disposition deferred --why <w> --boundary <bd> --trigger <tr> --operation-id <key> [--from <source>]` |
| Record provenance | `loaf conversation create --title <label>` then `loaf conversation handle add <id> --harness <h> --handle <opaque-id> [--locality <scope>] [--log-ref <path>]` |
| Associate conversation | `loaf exploration conversation add <exploration> <conversation-id>` |

## Process

1. **Orient.** If the input names an existing Exploration, run `loaf exploration context <ref>` and continue from its recommended next action. Otherwise check `loaf exploration list` before creating a duplicate inquiry.
2. **Create or resume.** New inquiries get `loaf exploration create` with `--from` links to the Intents, journal entries, reports, or findings that motivated them.
3. **Diverge.** Expand the option space before judging it. Use the brainstorm stance (below), research, scouting, prototypes, or spikes as the question demands.
4. **Capture as you go.** Incidental thoughts become sparks; explicit propositions become ideas; deliberately tracked directions become Intents with their sources linked.
5. **Checkpoint.** At every meaningful plateau — and always before ending a session — append a checkpoint with the four portable fields and optional `candidate:`/`evidence:` items.
6. **Record provenance when useful.** Machine-local conversation handles and log locators help forensic navigation later; add them explicitly, and never infer identity from the current session.

## Checkpoint Discipline

The four fields are the portable contract; each is capped at 4096 UTF-8 bytes and oversize input is rejected, never truncated:

- **purpose** — the current framing of the inquiry, restated so a stranger understands what is being explored and why.
- **conclusions** — constraints and conclusions established so far, including rejected options and why they fell.
- **unresolved** — the open question or decision the inquiry currently turns on.
- **next** — the recommended next action, concrete enough for a fresh agent to execute without this conversation.

Larger detail belongs in ordered `--item candidate:` and `--item evidence:` entries or in related reports, not crammed into the core fields.

## Resumption

A new conversation, harness, or machine resumes with `loaf exploration context <ref> --json`: the portable core returns whole, and each optional layer (items, intents, evidence, conversations) reports counts, truncation, and its exact expansion command. Source handles appear with their last observed availability; treat unavailable ones as lost without ceremony — the checkpoint is the context. If `portable_context_present` is false, the Exploration was never checkpointed: rebuild understanding from linked sources, then write the missing checkpoint first.

Before continuing, inspect the linked Intents in the context. If an Intent this inquiry was developing has since been resolved, do not silently reopen it: acknowledge the resolution, and if the checkpoint's next action still matters, create a successor Intent, record why in its body, and relate the lineage with `loaf link create --from <new-intent-ref> --to <resolved-intent-ref> --type derived-from`. Continued evidence gathering that serves no unresolved Intent should say so in its next checkpoint.

## Deferring

An Exploration is never deferred, paused, or closed — it has no lifecycle to transition. "Defer this exploration" means two concrete acts: checkpoint the current state honestly, then defer the direction it was developing as an Intent — `loaf intent defer` on the linked Intent, or `loaf intent create --disposition deferred` for a new one followed by `loaf link create --from <exploration-ref> --to <intent-ref> --type explores`. The deferred Intent carries the revisit trigger; the Exploration simply waits, resumable from its checkpoint whenever the Intent is resumed.

## Techniques

Brainstorm's full divergent stance lives inside Explore: generate options before judging, connect to VISION/STRATEGY context, document discarded options, set boundaries on exploration time. Scout, research, prototype, and spike remain subordinate techniques invoked from whatever stage needs them — none of them owns lifecycle.

## Related Skills

- **triage** — processes the intake queue and chooses dispositions, including "explore this"
- **shape** — narrows one direction into a bounded Change; the exit door from Explore
- **research** — evidence gathering for a known question, usable inside an Exploration
- **idea** — quick capture without inquiry
