---
source: follow-up probe run after the discovery smoke, same rig and cleanup discipline
date: 2026-08-05
harnesses: Amp (CLI), Cursor (cursor-agent), OpenCode 1.18.7, Codex 0.146.0
---

# Symlink-Following Smoke

## Contents
- Question
- Method
- Results
- Design consequences

Input to the next Change in the arc (skills-store redesign): whether harnesses load a skill whose body exists **only** behind a symlink in `~/.agents/skills`.

## Question

If `~/.agents/skills/<name>` is a symlink into a Loaf-owned backing store outside every harness search path, does each harness still discover and load the skill — for both absolute and relative link targets?

## Method

Two probe skills materialized only under `~/.local/share/loaf/skills-probe/internal/` (a path no harness searches), exposed as `~/.agents/skills/zz-symlink-abs` (absolute target) and `zz-symlink-rel` (relative target, `../../.local/share/...`), each with a unique body marker. Zero real directories matching the probe names existed in any search path. All probes and the backing store removed afterwards; store verified back to 40 entries with no dangling links.

## Results

| Harness | Absolute link | Relative link | Path reported |
|---|---|---|---|
| Amp | listed + described | listed + described | the **link** (`~/.agents/skills/...`) |
| Cursor | body loaded (PAPAYA-1177) | body loaded (LYCHEE-8823) | — |
| OpenCode | body loaded via `skill` tool | body loaded | the **link** |
| Codex | body loaded (after permitting file reads — Codex loads skill bodies by reading files) | body loaded | the **resolved target** (`~/.local/share/loaf/...`) |

All four non-Claude harnesses follow both link types. The backing-store design is buildable.

## Design consequences

- **Path-reporting split.** Amp and OpenCode name the link; Codex names the resolved store path. Any Loaf reporting, ownership, or error surface must handle both forms and never assume one.
- **Mid-session flips are the one unmeasured claim.** Codex resolves links at load time, so replacing a version directory during a running session could break that session; the store design's GC needs a one-version grace window, and shaping should verify flip behaviour directly.
- Prior art on this machine: `npx skills` already plants symlinks from `~/.claude/skills` into `~/.agents/skills` — including one dangling link (`find-skills`), which Amp silently skips. Link repair belongs in reconciliation, not in an error path.
