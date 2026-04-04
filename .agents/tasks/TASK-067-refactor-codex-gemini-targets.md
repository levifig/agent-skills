---
id: TASK-067
title: Refactor Codex + Gemini targets (byte-identical parity)
spec: SPEC-020
status: todo
priority: P0
created: '2026-04-04T16:41:22.294Z'
updated: '2026-04-04T16:41:22.294Z'
---

# TASK-067: Refactor Codex + Gemini targets (byte-identical parity)

Rewrite `codex.ts` and `gemini.ts` to read from the shared intermediate at `dist/skills/`.

## Scope

Both targets are structurally identical (~89 lines each): sidecar merge + version injection only. The per-target step becomes:
1. Copy skill directories from `dist/skills/` to target output
2. Load `SKILL.{target}.yaml` sidecar
3. Merge sidecar fields into frontmatter
4. Inject version

Remove inline `substituteCommands()`, skill discovery loop, and shared template calls from each target — the intermediate already handled these.

## Constraints

- **Byte-identical output** required — diff `dist/codex/skills/` and `dist/gemini/skills/` before and after
- Save pre-refactor output for comparison before modifying targets
- Watch for gray-matter serialization quirks (trailing newlines, key ordering)

## Verification

- [ ] Pre-refactor output saved for comparison
- [ ] `loaf build --target codex` output is byte-identical before/after
- [ ] `loaf build --target gemini` output is byte-identical before/after
- [ ] `npm run typecheck` and `npm run test` pass
