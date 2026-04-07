---
topics:
  - build-system
  - targets
  - distribution
covers:
  - cli/lib/build/**/*.ts
  - config/targets.yaml
  - config/hooks.yaml
consumers:
  - implementer
  - reviewer
last_reviewed: '2026-04-07'
---

# Build System

Loaf compiles skills, agents, and hooks from a single source tree into multiple target-specific formats.

## Key Rules

- **Single source, multiple outputs.** Content is authored in `content/`. The build system transforms it per-target.
- **Two-phase build.** Skills first compile to a shared intermediate (`dist/skills/`), then each target reads from the intermediate.
- **Targets are additive.** Each target gets what it supports — Claude Code gets everything, Codex gets skills only.
- **Sidecars carry target-specific fields.** SKILL.md has standard fields only. `.claude-code.yaml`, `.opencode.yaml`, etc. carry extensions. Build merges them.
- **Shared templates distribute at build time.** `content/templates/` files (`session.md`, `adr.md`) are copied to specified skills via `shared-templates` in `targets.yaml`. Other templates (e.g., `soul.md`) are distributed by install/session commands, not the build.
- **Command substitution.** `{{IMPLEMENT_CMD}}`, `{{ORCHESTRATE_CMD}}` placeholders in skill content are replaced per-target (e.g., `/implement` for Claude Code, OpenCode command name for OpenCode).

## Build Flow

`loaf build` (or `npm run build`) → CLI at `cli/commands/build.ts` → loads `hooks.yaml` + `targets.yaml` → builds shared skills intermediate to `dist/skills/` → calls each target's transformer in `cli/lib/build/targets/{target}.ts` → output to `plugins/` or `dist/{target}/`.

## Targets

| Target | Output | Agents | Skills | Hooks | Runtime Plugin |
|--------|--------|:------:|:------:|:-----:|:--------------:|
| claude-code | `plugins/loaf/` | Yes | Yes | Yes | No (bundled binary) |
| cursor | `dist/cursor/` | Yes | Yes | Yes | No |
| opencode | `dist/opencode/` | Yes | Yes | Yes | Yes (`hooks.ts`) |
| codex | `dist/codex/` | No | Yes | Yes | No |
| gemini | `dist/gemini/` | No | Yes | No | No |
| amp | `dist/amp/` | No | Yes | No | Yes (`loaf.js`) |

### Notes

- **Claude Code** bundles a self-contained `loaf` binary in `plugins/loaf/bin/loaf` for hook execution.
- **OpenCode and Amp** generate runtime plugins (`hooks.ts` / `loaf.js`) that implement enforcement hooks via subprocess calls to `loaf check`.
- **Codex** generates `.codex/hooks.json` for Bash-matching enforcement hooks.
- **MCP servers** are not bundled. `loaf install` detects and recommends MCPs at install time; integration state stored in `.agents/loaf.json`.

## Fenced Sections

`loaf install` writes fenced Loaf sections into project instruction files (CLAUDE.md, .cursorrules, AGENTS.md, etc.) using markers to identify managed content. This is separate from the build — it runs during installation to inject project-level configuration.

## Dev Tooling

| Script | Command | Purpose |
|--------|---------|---------|
| `cli/scripts/smoke-test.js` | `npm run test:smoke` | Validates built hook artifacts across all 6 targets (structure, `if` conditions, `failClosed` flags). Run after build changes. |
| `cli/scripts/eval-skill-routing.mjs` | `npm run eval:routing` | Tests whether Claude routes prompts to correct skills. Requires `ANTHROPIC_API_KEY`. Use `--model` for cheaper runs, `--skill` to test one skill. |

**Smoke test** is a build output integration test — should eventually be converted to a vitest suite. **Routing eval** is a non-deterministic quality tool for tuning skill descriptions; test cases need updating when skills are added/removed/renamed.

## Cross-References

- [skill-architecture.md](skill-architecture.md) — how skills are structured
- [hook-system.md](hook-system.md) — how hooks are registered and distributed
