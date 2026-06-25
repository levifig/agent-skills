# ADR-017: Global Agents Install Convention

## Status

Accepted

## Decision Date

2026-06-25

## Context

Loaf installs skills for several first-class harnesses. Before SPEC-052, install destinations were
split across harness config directories and the emerging global `~/.agents/skills/` convention.
That left two problems:

- The same Loaf skill body could be copied into multiple harness-specific homes.
- Moving a harness to the global convention could leave stale per-harness skill directories behind.

This ADR covers user-level installation only. It does not change project-local `.agents/` state or
ADR-013's rule that project-local agentic state resolves through the main worktree.

## Decision

Loaf installs user-level skills for Codex, Cursor, OpenCode, and Amp to:

```text
~/.agents/skills/
```

Claude Code remains the exception because Loaf installs it as a plugin distribution, not as direct
user-level skill folders.

Installers must use the shared destination resolver instead of per-target path literals. Shared
skill-home writes must preserve non-Loaf entries and track Loaf-managed skill names with a local
manifest rather than clearing the whole directory.

`loaf install --upgrade --yes` relocates old Loaf-owned OpenCode and Amp skill homes through the
SPEC-053 deprecation manifest:

- OpenCode: `$XDG_CONFIG_HOME/opencode/skills` -> `~/.agents/skills`
- Amp: `~/.config/agents/skills` -> `~/.agents/skills`

Relocation is allowed only when a Loaf owner marker exists for the matching harness install.

## Capability Table

| Harness | User-level skill path | Evidence |
|---|---:|---|
| Codex | `~/.agents/skills` | OpenAI Codex docs list `$HOME/.agents/skills` as the user skill scope: <https://developers.openai.com/codex/skills> |
| Cursor | `~/.agents/skills` | Cursor docs list `~/.agents/skills/` as a user-level skill directory: <https://cursor.com/docs/skills> |
| OpenCode | `~/.agents/skills` | OpenCode docs list global agent-compatible skills at `~/.agents/skills/<name>/SKILL.md`: <https://opencode.ai/docs/skills/> |
| Amp | `~/.agents/skills` | Amp's manual lists `~/.agents/skills/` as a user-wide skill path, after `~/.config/agents/skills/` in precedence: <https://ampcode.com/manual> |
| Claude Code | Plugin exception | Loaf's Claude Code target is distributed as `plugins/loaf/`, not direct user-level skills. |

## Consequences

- A single global skill copy can serve Codex, Cursor, OpenCode, and Amp.
- OpenCode and Amp get one-time cleanup for old per-harness skill homes when users opt into
  destructive upgrade cleanup.
- Shared skill-home cleanup is constrained to names Loaf previously managed, so unrelated user or
  vendor skills are not removed.
- Install detection records the target, config directory, and skill destination separately so future
  relocations do not depend on the skill path doubling as the install marker.

## Non-Goals

- This ADR does not define vendor skill installation, recommended skills, or curated skill packs.
- This ADR does not move project-local `.agents/skills/`.
- This ADR does not change Amp plugin installation under `.amp/plugins/`.
