---
topics:
  - build-system
  - targets
  - distribution
covers:
  - internal/cli/build*.go
  - config/targets.yaml
  - config/hooks.yaml
consumers:
  - implementer
  - reviewer
last_reviewed: '2026-07-14'
---

# Build System

Loaf compiles skills, agents, and hooks from a single source tree into multiple target-specific formats.

## Key Rules

- **Single source, multiple outputs.** Content is authored in `content/`. The build system transforms it per-target.
- **Shared intermediate.** Skills compile to `dist/skills/`, then each target reads that normalized content.
- **Targets are additive.** Each target gets what it supports — Claude Code gets everything, while Codex gets skills plus its policy template and current-schema SessionStart context hook.
- **Sidecars carry target-specific fields.** SKILL.md has standard fields only. `.claude-code.yaml`, `.opencode.yaml`, etc. carry extensions. Build merges them.
- **Shared templates distribute at build time.** `content/templates/` files (`session.md`, `adr.md`) are copied to specified skills via `shared-templates` in `targets.yaml`.
- **Command substitution.** `{{IMPLEMENT_CMD}}`, `{{ORCHESTRATE_CMD}}` placeholders in skill content are replaced per-target (e.g., `/implement` for Claude Code, OpenCode command name for OpenCode).

## Build Flow

`loaf build` (or `npm run build`) -> Go dispatcher in `internal/cli/build.go` -> loads `hooks.yaml` + `targets.yaml` -> builds shared skills intermediate to `dist/skills/` -> calls each native target builder in `internal/cli/build_{target}.go` -> output to `plugins/` or `dist/{target}/`.

## Targets

| Target | Output | Agents | Skills | Hooks | Runtime Plugin |
|--------|--------|:------:|:------:|:-----:|:--------------:|
| claude-code | `plugins/loaf/` | Yes | Yes | Yes | `plugin.json` + `hooks.json` |
| cursor | `dist/cursor/` | Yes | Yes | Yes | No |
| opencode | `dist/opencode/` | Yes | Yes | Yes | Yes (`hooks.ts`) |
| codex | `dist/codex/` | No | Yes | SessionStart context | No |
| amp | `dist/amp/` | No | Yes | No | Yes (`.amp/plugins/loaf.ts`) |

### Notes

- **Claude Code** bundles a self-contained `loaf` binary in `plugins/loaf/bin/loaf` for hook execution. Hooks are registered in `hooks/hooks.json` because `plugin.json` silently drops non-matcher session events.
- **OpenCode and Amp** generate runtime plugins (`hooks.ts` / `.amp/plugins/loaf.ts`) that implement enforcement hooks via subprocess calls to `loaf check`.
- **Codex** generates a current-schema `.codex/hooks.json` SessionStart matcher group because Codex `0.144.1` rejects Loaf's legacy flat hook projection; the command and `commandWindows` placeholders are rendered to trusted absolute paths at install. POSIX installs retain the exact two-field command shape and omit the Windows variant; Windows installs render both fields to the same `cmd.exe /C` outer-wrapped command. Isolated `CODEX_HOME` startup on `darwin-arm64` is model-visible smoke-proven; global-home installation, resume, clear, compact, Windows runtime behavior, and completion remain separately unproven. The separately opted-in basic command policy renders one absolute-executable prefix per explicitly classified leaf, while body/file-consuming leaves and path-taking `change check` remain operator-gated. Other harness adapters are not implied.
- **MCP servers** are not bundled. `loaf install` detects and recommends MCPs at install time; integration state stored in `.agents/loaf.json`.

### Hook Registration (Claude Code)

Claude Code has a split registration model: `plugin.json` handles the plugin manifest (skills, agents, metadata) while `hooks/hooks.json` handles all hook registrations. This split exists because `plugin.json` silently drops session events (SessionStart, PreCompact, PostCompact, TaskCompleted, etc.) that lack a `matcher` field. All hooks — enforcement, instruction, journal, and conversation — are registered in `hooks/hooks.json` for reliable dispatch.

## Fenced Sections

`loaf install` writes fenced Loaf sections into project instruction files (CLAUDE.md, .cursorrules, AGENTS.md, etc.) using markers to identify managed content. This is separate from the build — it runs during installation to inject project-level configuration.

`loaf config check` is the focused health gate for project Loaf configuration. It validates `.agents/loaf.json` and compares installed Loaf-managed target hook registrations against the current distribution. Use `loaf config check --fix` to add missing safe project-config defaults and refresh stale installed target artifacts when a release adds hooks such as `github-account`.

## Dev Tooling

| Script | Command | Purpose |
|--------|---------|---------|
| `cli/scripts/smoke-test.js` | `npm run test:smoke` | Validates built hook artifacts across supported targets (structure, `if` conditions, `failClosed` flags). Run after build changes. |
| `cli/scripts/eval-skill-routing.mjs` | `npm run eval:routing` | Tests whether Claude routes prompts to correct skills. Requires `ANTHROPIC_API_KEY`. Use `--model` for cheaper runs, `--skill` to test one skill. |

**Smoke test** is a build output integration test for script-level artifact assertions that are not yet worth moving into native Go tests. **Routing eval** is a non-deterministic quality tool for tuning skill descriptions; test cases need updating when skills are added/removed/renamed.

## Cross-References

- [skill-architecture.md](skill-architecture.md) — how skills are structured
- [hook-system.md](hook-system.md) — how hooks are registered and distributed
