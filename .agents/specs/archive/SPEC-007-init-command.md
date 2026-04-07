---
id: SPEC-007
title: loaf init — project bootstrapping
source: direct
created: '2026-01-24T22:47:04.000Z'
status: complete
---

# SPEC-007: loaf init

## Problem Statement

Loaf provides powerful agentic workflows, but there's no guided entry point. Users must manually create `.agents/`, know which files to create, understand what Loaf offers, and bootstrap strategic documents. Without `loaf init`, adoption requires implicit knowledge and manual setup.

## Strategic Alignment

| Question | Assessment |
|----------|------------|
| Advances vision? | Yes — lowers barrier to Loaf adoption |
| Fits CLI architecture? | Yes — `loaf init` as a new CLI command in `cli/commands/init.ts` |
| Depends on? | SPEC-008 (CLI skeleton) — done |

## Solution Direction

`loaf init` as a CLI command that scans, reports, scaffolds, and recommends:

1. **Scan** — detect language, frameworks, existing patterns
2. **Report** — show what exists, what's missing, what's recommended
3. **Scaffold** — create `.agents/` structure, strategic docs, changelog
4. **Recommend** — suggest skills based on detected stack

The command is **idempotent** — safe to run multiple times. Re-running reports status and fills gaps.

### Implementation

New file: `cli/commands/init.ts` registered in `cli/index.ts`.

## Scope

### In Scope

**Directory structure (auto-create):**
```
.agents/
├── AGENTS.md          # Canonical agent instructions (template)
├── sessions/
├── ideas/
├── specs/
└── tasks/

docs/
├── knowledge/         # Project knowledge base
├── decisions/         # ADRs
├── VISION.md          # Template with sections to fill
├── STRATEGY.md        # Template with sections to fill
└── ARCHITECTURE.md    # Template with sections to fill
```

**Root files (auto-create if missing):**
- `CHANGELOG.md` — Keep-a-Changelog format with `[Unreleased]` section

**Symlinks (ask before creating):**
- `.claude/CLAUDE.md` → `.agents/AGENTS.md` (Claude Code reads this)
- `./AGENTS.md` → `.agents/AGENTS.md` (human-facing alias)

**Project config (auto-create if missing):**
- `.agents/loaf.json` — project-level Loaf config

**Project detection:**
- Language detection (Python, TypeScript, Ruby, Go, etc.)
- Framework detection (FastAPI, Next.js, Rails, etc.)
- Existing documentation patterns

**Skill recommendations:**
- Map detected stack to relevant Loaf skill groups
- Report which skills would be activated

**Brownfield support:**
- Detect existing docs
- Suggest consolidation without auto-migrating
- Never overwrite existing files

### Out of Scope

- Auto-migration of existing files
- Git operations
- Installing dependencies or tools
- Creating project-specific AGENTS.md content (just template)
- CI/CD configuration
- Knowledge management setup (SPEC-009)

### Rabbit Holes

- **Deep framework detection** — keep simple, don't parse every config file
- **Smart template generation** — templates should be generic; `/loaf:strategy` fills in details
- **Dependency analysis** — just detect top-level indicators, don't parse lock files

### No-Gos

- Don't overwrite existing files — ever
- Don't auto-migrate brownfield docs — only suggest
- Don't create symlinks without asking
- Don't require user input for basic setup — questions only for ambiguous cases

## Test Conditions

- [ ] `loaf init` creates `.agents/` with all subdirectories
- [ ] Creates `.agents/AGENTS.md` with useful template
- [ ] Creates `docs/knowledge/`, `docs/decisions/`
- [ ] Creates `docs/VISION.md`, `docs/STRATEGY.md`, `docs/ARCHITECTURE.md` if missing
- [ ] Creates `CHANGELOG.md` in root if missing
- [ ] Creates `.agents/loaf.json` with default config
- [ ] Does NOT overwrite any existing files
- [ ] Asks before creating symlinks
- [ ] Creates `.claude/CLAUDE.md` → `.agents/AGENTS.md` when approved
- [ ] Creates `./AGENTS.md` → `.agents/AGENTS.md` when approved
- [ ] Detects Python/TypeScript/Ruby/Go projects
- [ ] Recommends appropriate skills based on detection
- [ ] Reports existing patterns in brownfield projects
- [ ] Idempotent: re-running shows status, fills gaps only

## Circuit Breaker

At 50%: Ship with full file creation but basic language detection only (skip framework detection and skill recommendations).
At 75%: Ship what works. Detailed detection can be enhanced incrementally.
