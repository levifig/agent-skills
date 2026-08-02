---
change: harness-neutral-skills
id: TASK-004
title: Canonical single-flavor install
blocked-by:
  - TASK-003
blocks:
  - TASK-005
---

# TASK-004 — Canonical single-flavor install

## Objective

One copy of each skill lands in `~/.agents/skills`, no target fans out its own flavor, Amp is bridged into the canonical store at the path it actually reads, and installing every target in any order leaves identical bytes on disk.

## Scope boundaries

**In:** `installSkillsDestination` and the four per-target install functions in `internal/cli/install_target.go`, the Amp bridge, and the symlink-versus-copy fallback.

**Out:** Renaming skills — TASK-005. Retiring stale copies from previous layouts — TASK-006. Claude Code, which keeps its plugin channel untouched.

## Context pointers

- Contract: `shape.md` — Decisions 1, 5, 6; Planning Contract / Risks
- Reference implementation: `vercel-labs/skills` `src/installer.ts` (`getCanonicalSkillsDir`, `getAgentBaseDir`) and `src/agents.ts` (`getUniversalAgents`, `getNonUniversalAgents`)
- Codex documents symlink traversal; Amp is the only target whose answer is load-bearing here

## Acquisition

```bash
loaf journal log "skill(implement): TASK-004 — canonical single-flavor install"
```

## Steps

- [ ] Collapse skill installation to a single canonical write, so Codex, Cursor, and OpenCode consume the store directly with nothing copied for them
- [ ] Bridge Amp at `~/.config/agents/skills` with a symlink into the canonical store, falling back to a copy when symlink creation fails — Windows needs the fallback regardless
- [ ] Record the Amp bridge in the install record so ownership and later migration can find it
- [ ] Add `TestCanonicalSkillsDestination` and `TestSequentialInstallLeavesOneFlavor`, the latter installing every target in more than one order
- [ ] Run an installed smoke proving Amp surfaces Loaf skills, and one proving symlink traversal on any target that depends on it; record both as evidence

## Verification

- `go test ./internal/cli/ -run TestCanonicalSkillsDestination` passes
- `go test ./internal/cli/ -run TestSequentialInstallLeavesOneFlavor` passes
- The Amp smoke shows Loaf skills present, closing the open question rather than restating the inference
