---
id: SPEC-008
title: Loaf CLI
created: '2026-03-14T19:48:00.000Z'
status: complete
requirement: Loaf should be a proper CLI tool with clean content/tooling separation
---

# SPEC-008: Loaf CLI

## Problem Statement

Loaf has grown into a multi-target build system with skills, agents, hooks, and templates — but its only entry point is `npm run build`, a raw Node.js script with no flags, no UX, and no extensibility. As Loaf evolves from a build-time framework into an agent-agnostic CLI (ADR-005), it needs:

1. **A proper CLI entry point** — `loaf` command with subcommands, help, version, flags
2. **Content/tooling separation** — skills, agents, hooks, and templates (content) are intermixed with build logic (tooling) under `src/`. These are fundamentally different concerns.
3. **Better distribution** — `install.sh` duplicates logic that should live in the CLI. Five targets get near-identical copies of skills. The output structure can be cleaner.
4. **TypeScript codebase** — the build system is vanilla JS. The CLI and all tooling should be TypeScript.

This is a clean break. No backward compatibility with `npm run build` or the current directory structure. Content (skills, agents, hooks) carries forward; the tooling around it is rebuilt.

## Strategic Alignment

| Question | Assessment |
|----------|------------|
| Advances vision? | Yes — "agent-agnostic CLI" is Pillar 1. Enables Pillars 2 (knowledge) and 3 (autonomous execution). |
| Serves target personas? | Yes — agents call `loaf build` / `loaf install` via Bash on any harness. Humans get proper CLI. |
| Fits technical constraints? | Yes — Node.js/TypeScript, same runtime. Commander.js for routing, tsup for bundling. |
| Conflicts with existing work? | SPEC-007 (init) depends on CLI skeleton. SPEC-009/010 extend it with subcommands. No conflicts. |

## Solution Direction

### CLI Tool

Create a `loaf` CLI using Commander.js (argument parsing) + tsup (esbuild bundling) + TypeScript. The `package.json` `bin` field provides the `loaf` command. Development via `npm link`.

Two commands ship with this spec:
- `loaf build` — build skills/agents/hooks for targets
- `loaf install` — detect and install to target tools (replaces install.sh)

### Source Reorganization

Separate content (what gets distributed) from tooling (what does the distributing):

```
cli/                        # CLI tool (TypeScript)
├── index.ts                # Entry point (Commander setup)
├── commands/
│   ├── build.ts            # loaf build
│   └── install.ts          # loaf install (absorbs install.sh)
└── lib/
    ├── build/              # Build system (refactored from build/*.js)
    │   ├── orchestrator.ts # Was build.js
    │   ├── targets/        # Target transformers
    │   └── lib/            # Build utilities (version, sidecar, etc.)
    ├── install/            # Install logic (from install.sh)
    └── detect/             # Tool detection (from install.sh)

content/                    # Distributable content
├── skills/                 # Was src/skills/
├── agents/                 # Was src/agents/
├── hooks/                  # Was src/hooks/
└── templates/              # Was src/templates/

config/                     # Build & tool configuration
├── hooks.yaml              # Was src/config/hooks.yaml
└── targets.yaml            # Was src/config/targets.yaml

dist/                       # Build output (generated)
└── {target}/
```

**Key interface:** CLI reads from `content/` and `config/`, builds to `dist/`. This boundary makes content extractable to a separate repo later (decision deferred — see Notes).

### Output Improvements

- Reduce duplication across targets where possible
- Simpler, flatter dist/ structure within target constraints
- Clear separation of what each target gets (some only need skills)
- Build output should be clean and minimal

### Install Replacement

`loaf install` absorbs install.sh's logic:
- Tool detection (Claude Code, Cursor, OpenCode, Codex, Gemini)
- `loaf install --to <target>` for specific targets
- `loaf install --to all` for auto-detected targets
- Interactive target selection when no flag given
- Colored output, status reporting

install.sh becomes a thin bootstrap: curl-pipe installs the npm package, then calls `loaf install`.

## Scope

### In Scope

**CLI skeleton:**
- `cli/` directory with TypeScript source
- Commander.js setup with global `--help`, `--version`
- `loaf` bin entry point via package.json
- tsup build config (single-file bundle)
- tsc --noEmit for type checking

**`loaf build` command:**
- `loaf build` (all targets)
- `loaf build --target <name>` (specific target)
- Colored output, timing info, error formatting
- Same build logic, better interface

**`loaf install` command:**
- Full replacement of install.sh logic
- Tool detection for all 5 targets
- `--to <target>`, `--to all`, interactive mode
- `--upgrade` for unattended updates
- Dev mode detection (running from repo)

**TypeScript conversion:**
- All build system JS → TypeScript (build.js, targets/*.js, lib/*.js)
- Full `strict: true` TypeScript
- tsup + tsc --noEmit setup

**Source reorganization:**
- `src/skills/` → `content/skills/`
- `src/agents/` → `content/agents/`
- `src/hooks/` → `content/hooks/`
- `src/templates/` → `content/templates/`
- `src/config/` → `config/`
- `build/` → `cli/lib/build/`
- install.sh logic → `cli/lib/install/` + `cli/lib/detect/`

**Output improvements:**
- Cleaner dist/ structure
- Reduce redundant copies where targets allow
- Better separation of concerns per target

**Package setup:**
- package.json: bin, main/exports, publish-ready fields
- tsconfig.json, tsup.config.ts
- npm link works for development
- Clean break: remove old npm build scripts

### Out of Scope

- `loaf init` (SPEC-007 — depends on this skeleton existing)
- `loaf kb` commands (SPEC-009)
- `loaf task` / `loaf spec` commands (SPEC-010)
- `loaf skill add/list/validate` (future)
- `loaf build --watch` (nice-to-have, defer)
- Actually splitting skills into a separate repo (architecture prepared, split deferred)
- npm publish to registry (package.json is publish-ready; publishing is a separate step)
- Hooks ownership decision for future repo split (deferred)

### Rabbit Holes

- **TypeScript strictness** — Use `strict: true` but allow pragmatic escape hatches for dynamic config handling (hooks.yaml parsing, target transformer loading). Don't chase perfect types on the build system's YAML config handling.
- **Output format redesign** — Improve structure but don't invent new formats that break target tool expectations. Claude Code expects `plugins/loaf/`, Cursor expects `~/.cursor/skills/`, etc. Work within those constraints.
- **install.sh feature parity** — Absorb the logic, don't reinvent. The interactive selection, tool detection, and dev mode all work well. Port them faithfully, then improve.
- **Content/CLI boundary** — The separation should be a clean directory boundary, not an abstract plugin system. Don't over-engineer the interface between CLI and content. A simple "read from this directory" is sufficient.
- **Testing** — Don't add a test framework as part of this spec. Verify by diffing output and manual testing. Automated testing comes later.

### No-Gos

- Don't add subcommands beyond `build` and `install` — skeleton must be extensible, not extended
- Don't actually split content into a separate repo — prepare the architecture, defer the split
- Don't build a skill marketplace or registry — `loaf skill add` is future work
- Don't require TypeScript to be globally installed — tsup handles compilation

## Risks

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| TS conversion introduces build regressions | Medium | High | Diff dist/ output before and after conversion. Keep git history for rollback. |
| tsup bundling breaks dynamic imports (target loading) | Medium | Medium | Test Commander + dynamic imports early. May need explicit entry points per target. |
| Source reorg breaks existing dev workflow | Low | Medium | Document new workflow clearly. `npm link` + `loaf build` should feel natural. |
| install.sh port misses edge cases | Medium | Medium | Test on macOS with all targets. install.sh has careful PATH normalization. |
| Scope creep into kb/task/skill commands | Medium | Medium | Circuit breaker. Strict "build + install only" boundary. |
| Content/CLI boundary becomes too abstract | Low | High | Keep it simple: directory path, not a plugin API. |

## Test Conditions

- [ ] `loaf --help` shows version and available commands (build, install)
- [ ] `loaf --version` shows current version from package.json
- [ ] `loaf build` builds all 5 targets successfully
- [ ] `loaf build --target claude-code` builds only Claude Code
- [ ] `loaf build --target nonexistent` shows helpful error with valid targets
- [ ] Build output shows colored status, timing, and target names
- [ ] `loaf install --to cursor` installs to Cursor config directory
- [ ] `loaf install --to all` detects and installs to all available targets
- [ ] `loaf install` (no flags) shows interactive target selection
- [ ] `loaf install --upgrade` updates only already-installed targets
- [ ] Dev mode detected when running from the repo (not from npm global)
- [ ] `npm link` makes `loaf` available globally on the dev machine
- [ ] Source lives in `cli/` (tooling) and `content/` (distributable)
- [ ] Old `build/` and `src/` directories removed
- [ ] Old `npm run build` scripts removed from package.json
- [ ] All TypeScript compiles without errors (`tsc --noEmit`)
- [ ] package.json has `bin`, `exports`, and is publish-ready
- [ ] `loaf` with no subcommand shows help (not an error)
- [ ] dist/ output is functionally equivalent to previous build output
- [ ] install.sh reduced to thin bootstrap (curl → npm install → loaf install)

## Circuit Breaker

**At 30% appetite (~4-5 days):** Ship CLI skeleton + `loaf build` wrapping the existing JS build system (not yet converted to TypeScript). Source reorganization done (content/ vs cli/) but build logic still JS, imported by TypeScript CLI commands. This gives us the `loaf` command and the content separation.

**At 50% appetite (~7-8 days):** Add TypeScript conversion of build files. `loaf build` works fully in TypeScript. `loaf install` still pending — install.sh unchanged.

**At 75% appetite (~11-12 days):** Add `loaf install`. Full TypeScript. Source fully reorganized. install.sh reduced to bootstrap.

**At 100% (~15 days):** Output improvements, dist/ cleanup, polish, documentation updates. Everything in scope delivered.

## Notes

**Content extraction (future):** The content/ directory is designed to be extractable to a separate repo. When that happens, the key questions are: where do hooks go? where does hooks.yaml go? These are deferred. For now, the clean directory boundary is sufficient architecture.

**SPEC-007 dependency:** `loaf init` (SPEC-007) depends on the CLI skeleton from this spec. After SPEC-008 ships, SPEC-007 becomes a new `cli/commands/init.ts` file.

**Related specs:** SPEC-009 (knowledge management) and SPEC-010 (task CLI) both add subcommands to the CLI established here.

See [ADR-005](../../docs/decisions/ADR-005-loaf-cli-evolution.md) for the CLI evolution decision rationale.
See `.agents/drafts/brainstorm-loaf-cli-knowledge-harness.md` for full brainstorm context.
