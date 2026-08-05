---
source: harness-neutral-skills discovery smoke, run against installed harnesses on the dogfooding machine
date: 2026-08-05
harnesses: Amp (CLI), Cursor (cursor-agent), OpenCode 1.18.7
---

# Discovery Smoke: Deduplication and Frontmatter Tolerance

## Contents
- Questions
- Method
- Results
- The shadowing finding
- Incidental findings

Answers the two questions TASK-006 left open: whether harnesses deduplicate a skill discoverable through several search paths, and whether Cursor and Amp tolerate the OpenCode-owned frontmatter keys (`subtask`, `user-invocable`) that reach them through the single canonical copy.

## Questions

1. When one skill name is discoverable through more than one search path, does the harness list it once or twice — and which copy wins?
2. Do harnesses that do not own the `subtask` / `user-invocable` frontmatter keys still load a skill that carries them?

## Method

Temporary probe skills with unique body markers ("magic words"), planted and removed in the same run, environment verified restored afterwards (40 store entries before and after; no probe remnants).

- **Amp**: `amp skill list` / `amp skill info` — no model call needed. An isolated-`HOME` variant confirmed root behaviour; a deliberate name collision (`architecture`) planted in a second readable root tested dedup directly.
- **Cursor**: `cursor-agent --mode ask` (read-only), asked targeted questions about specific probe names and body markers rather than enumeration. Read-path verified first: `council-session` and `resume-session` exist only in `~/.cursor/skills` and were reported VISIBLE.
- **OpenCode**: `opencode run` with a collision planted across `~/.config/opencode/skills` (created for the test, deleted after) and `~/.agents/skills`, distinct markers per copy.

## Results

| Harness | Reads `~/.agents/skills` | Dedupes colliding names | Winning root | Tolerates `subtask`/`user-invocable` |
|---|---|---|---|---|
| Amp | yes | yes — collision listed once | `~/.agents/skills` | yes — probe listed, `skill info` resolves, no warning |
| Cursor | yes (also `~/.cursor/skills`) | yes — one entry | `~/.agents/skills` (probe marker MANGO-4482 returned, not the native copy's) | yes — probe body loaded (PINEAPPLE-7731) |
| OpenCode | yes (also `~/.config/opencode/skills`) | yes — one entry | **its own native dir** (marker KIWI-9903 returned from `~/.config/opencode/skills`) | n/a — OpenCode owns these keys |

Supporting Amp evidence: `architecture` was discoverable in ten locations (the canonical store plus nine cached Claude-plugin versions of Loaf, alpha.6–alpha.19); `amp skill list` showed it exactly once, resolved to the canonical store. Amp does **not** read `~/.cursor/skills` — the four skills existing only there were absent from its listing, so cross-root dedup had to be proven with a collision between roots Amp actually reads.

## The shadowing finding

OpenCode resolves collisions the opposite way from Cursor and Amp: **its native directory shadows the canonical store**. A stale skill left in `~/.config/opencode/skills` silently overrides the canonical copy for OpenCode alone, while every other harness reads the new one — no error, no duplicate listing. This is the measured justification for ADR-018's relocation and for migration transferring claims rather than leaving both copies standing: migration conservatism was the mitigation while this was unknown; cleanup of native dirs is the cure now that it is measured.

## Incidental findings

- Every `opencode run` emitted `[loaf] OpenCode system.transform hook session-start-loaf failed (exit 1); context delivery continued` — the installed (pre-Change) plugin hook fails in the field. Tracked separately.
- Codex warns that skill descriptions are shortened to fit a **2% skills context budget** at the current 40-skill store size — input for the skills audit.
- Nine stale Loaf plugin cache versions under `~/.claude/plugins/cache/levifig-loaf/` each carry a full discoverable skill tree; Amp dedupes them away, but they are housekeeping debt.
