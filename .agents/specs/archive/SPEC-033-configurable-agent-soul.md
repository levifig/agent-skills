---
id: SPEC-033
title: Configurable Agent Soul (Lore Decoupling)
source: >-
  direct — conversation about decoupling Warden lore from agent profile
  mechanics
created: '2026-04-28T15:30:00.000Z'
status: complete
---

# SPEC-033: Configurable Agent Soul (Lore Decoupling)

## Problem Statement

Loaf's agent profiles (Smith/Implementer, Sentinel/Reviewer, Ranger/Researcher, Librarian) currently embed the Warden/Fellowship lore directly in their system prompts. The four profile files at `content/agents/*.md` open with character/race framing ("You are a Smith — a Dwarf who forges...", "You are a Sentinel — an Elf who watches..."), and reference each other through lore terms ("that is Smith work", "the Warden's role").

This couples three things that should be independent:

1. **Functional contract** — what tools the profile has, what it does, what it doesn't.
2. **Personality** — the character voice (Wizard, Dwarf, Elf, etc.), instance naming convention, council vocabulary.
3. **The active SOUL.md** — the orchestrator's identity for the main session.

A user installing Loaf in a corporate, professional, or no-fantasy context inherits Tolkien-flavored lore even when only the workflow opinions matter. There is no opt-out short of forking the framework.

The fix is mechanical, not philosophical: profiles describe the role; SOUL.md describes the identity. Each agent reads the active SOUL.md at spawn for character and vocabulary. Swapping the SOUL.md swaps the personality without rebuilding profiles or skills.

## Strategic Alignment

- **Vision:** Reinforces "Bounded Autonomy" by sharpening the profile/skill split — profiles become *mechanics-only*, skills handle *knowledge*, soul handles *identity*. Three orthogonal layers.
- **Personas:** The team-lead persona benefits most — corporate/multi-developer contexts where the lore feels out of place. The solo-developer persona keeps Warden if they want it (zero-config preservation for existing repos).
- **Architecture:** Extends the existing pattern where SessionStart restores SOUL.md from a canonical template. This spec generalizes "canonical" to "configured soul from catalog." Composes cleanly with SPEC-022 (native agent profile routing) — that spec makes Claude *use* profiles; this spec makes profiles *neutral about identity*.
- **Strategic tension addressed:** *Convention vs. flexibility* — Loaf's pipeline opinions stay strict; the lore convention becomes optional.

## Solution Direction

Three coordinated changes:

1. **Souls catalog.** Ship `content/souls/{fellowship,none}/SOUL.md` with Loaf.
   - `fellowship` — current Warden/Fellowship lore, preserved verbatim. Provides character names (Warden, Smith, Sentinel, Ranger, Librarian), races, instance naming conventions, council vocabulary.
   - `none` — minimal SOUL.md describing the orchestrator role and its team purely functionally: *orchestrator*, *implementer*, *reviewer*, *researcher*, *librarian*. Same role definitions and orchestration principles as `fellowship`, but with function-based labels — no names, no races, no metaphor. Sets the foundation for a future https://soul.md-aligned standard.

2. **Configurable soul, runtime-resolved.** `.agents/loaf.json` gains a `soul: <name>` field. `loaf install` writes the active soul's content to `.agents/SOUL.md` (a real file, not a symlink — copy/replace mechanics). User edits to the local file are preserved; Loaf detects divergence and refuses to overwrite without confirmation.

3. **Profile neutralization + spawn-time SOUL.md read.** Rewrite the four profile prompts (`content/agents/{implementer,reviewer,researcher,librarian}.md`) to:
   - Drop all Smith/Sentinel/Ranger/Dwarf/Elf/Human/Ent/Wizard/Warden/Fellowship vocabulary.
   - State the functional contract directly ("You are an implementer. Full write access...").
   - Include a Critical Rule: *"Your first action MUST be to Read `.agents/SOUL.md` and internalize the character described there as your identity."*

   Same shape as the existing "skills self-log to journal first" rule — proven pattern.

CLI surface: `loaf soul list / current / show <name> / use <name>`.

## Scope

### In Scope

- Catalog at `content/souls/{fellowship,none}/SOUL.md` — two souls only for v1.
- `cli/lib/souls/` module + `cli/commands/soul.ts` — `list`, `current`, `show <name>`, `use <name>` subcommands.
- `loaf.json` schema: add `soul: string` (default `none`).
- `loaf install` updates: fresh installs write `none` SOUL.md; existing installs with an existing `.agents/SOUL.md` and no `soul:` field get `soul: fellowship` written to `loaf.json` (legacy default), SOUL.md untouched. `loaf install --interactive` prompts the user to pick a soul before writing; bootstrap surfaces this prompt by calling install.
- Profile neutralization: rewrite `content/agents/{implementer,reviewer,researcher,librarian}.md` — strip lore, add SOUL.md-read directive.
- SessionStart hook update: restore `.agents/SOUL.md` from configured soul (not canonical template) when missing.
- Skill prose audit + cleanup: `content/skills/orchestration/references/councils.md` and any other Smith/Sentinel/Ranger/Warden/Fellowship references in skills must be replaced with profile types (implementer/reviewer/researcher).
- `ARCHITECTURE.md` "Agent Model: Functional Profiles" rewrite — describe the configurable soul, not the Warden specifically; move character description to a soul-catalog reference.

### Out of Scope

- Custom user-defined souls (`.agents/souls/{name}/SOUL.md`) — catalog-only for v1; future spec.
- More than two souls in v1 (e.g., "professional", "studio"). Ship the mechanism; expand the catalog later.
- Auto-migration tool to translate Warden-customized SOUL.md to a different soul.
- Lore name suggestions per profile (handled by SPEC-022 if it ships).
- Live SOUL.md hot-reload mid-session — agents read once at spawn.
- `loaf doctor` checks for soul-state drift.
- Journal-logging soul changes via `loaf soul use` — keep CLI surface lean.

### Rabbit Holes

- **Iterating on the perfect "neutral" tone.** Ship a serviceable v1; refine via PRs. Don't bikeshed the prose.
- **Aligning with the https://soul.md standard.** Note as future direction; v1 just ships valid SOUL.md text.
- **Comprehensive vocab audit across all 31 skills.** Focus the audit on prose subagents read (orchestration councils, profile-related references). Other skill prose can drift and be cleaned up later — it doesn't block agents.
- **Theme expansion.** Don't ship a third theme until two have been used in the wild.

### No-Gos

- Don't break existing Warden installs. Zero-config preservation: an existing `.agents/SOUL.md` is never overwritten on upgrade.
- Don't auto-edit a user-modified `.agents/SOUL.md`. `loaf soul use <name>` warns and requires `--force` (or interactive confirm) when the local file diverges from the catalog source.
- Don't make profiles dependent on SOUL.md. Profile prompts must include enough functional contract to operate even if SOUL.md is missing — agents lose personality, not capability.
- Don't fork Smith/Sentinel/Ranger names into config. The catalog ships character vocabulary; profiles use type names only.

## Risks

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Agents skip the "read SOUL.md first" directive | Med | Low | Same shape as skill self-logging rule (proven pattern). Mark Critical Rule in profile. |
| Skill prose still leaks fellowship terms post-neutralization | High | Low | Targeted audit of orchestration/councils + grep sweep as part of T8. Accept some non-agent-facing prose drifts. |
| ARCHITECTURE.md rewrite churns docs unhelpfully | Low | Med | Keep the rewrite scoped — rename "Warden" references to "the configured orchestrator" + add a soul-catalog subsection. Don't rewrite the agent model itself. |
| Existing repo upgrades break because `loaf.json` lacks `soul:` field | Med | Med | Install logic detects missing field + existing SOUL.md and writes `soul: fellowship` automatically. Tested via T7. |
| `loaf soul use <name>` accidentally trashes user customizations | Low | High | Diff-check before overwrite; require `--force` or confirm. Tested via T5. |

## Resolved Decisions

These were settled during shaping:

- **Soul changes are not journaled.** `loaf soul use <name>` updates `loaf.json` and `SOUL.md`. No journal entry. Keep CLI surface lean.
- **Bootstrap delegates to install.** `loaf install --interactive` is the single soul-selection surface. The bootstrap skill calls install rather than prompting independently.
- **`none` is purely functional, not anonymous.** The `none` SOUL.md describes the orchestrator/implementer/reviewer/researcher/librarian roles by their functions. Same role definitions and orchestration principles as `fellowship`; just no names, races, or metaphor.

## Test Conditions

- [ ] T1: `loaf soul list` outputs `fellowship` and `none` with one-line descriptions.
- [ ] T2: `loaf soul current` reads `loaf.json` and prints the active soul name.
- [ ] T3: `loaf soul show <name>` prints catalog SOUL.md content without modifying `.agents/SOUL.md`.
- [ ] T4: `loaf soul use none` writes `content/souls/none/SOUL.md` to `.agents/SOUL.md` and updates `loaf.json`.
- [ ] T5: `loaf soul use <name>` with diverged local file requires `--force` (or interactive confirmation) before overwriting.
- [ ] T6: `loaf install` on a fresh project writes `.agents/SOUL.md` from `none` (default) and sets `soul: none` in `loaf.json`.
- [ ] T7: `loaf install` on an existing repo with pre-existing `.agents/SOUL.md` and no `soul:` field writes `soul: fellowship` to `loaf.json` and leaves `SOUL.md` untouched.
- [ ] T8: Agent profile prompts contain no occurrences of `Smith|Sentinel|Ranger|Dwarf|Elf|Human|Ent|Wizard|Warden|Fellowship` (grep test).
- [ ] T9: Agent profile prompts contain an explicit "Read `.agents/SOUL.md` as your first action" Critical Rule.
- [ ] T10: SessionStart hook restores `.agents/SOUL.md` from the configured soul when missing.
- [ ] T11: With `soul: fellowship`, a spawned implementer introduces itself with Dwarvish framing; with `soul: none`, the introduction is functional only. (Manual smoke test acceptable.)
- [ ] T12: `loaf build` succeeds for all 6 targets with the neutralized profiles.
- [ ] T13: Skill prose audit (orchestration/councils + grep across content/skills) — no fellowship vocab in agent-facing skill content.
- [ ] T14: `loaf install --interactive` prompts for soul selection on fresh installs and respects the choice.

## Priority Order

1. **Catalog + config + CLI mechanics.** Ship `content/souls/{fellowship,none}/`, the `loaf soul` command, `loaf.json` schema bump, install behavior. Profiles untouched — Warden lore still in profile prompts but souls catalog exists. Go/no-go: T1–T7 pass. Manually verifies the swap mechanism without behavior risk.
2. **Profile neutralization + skill audit.** Rewrite the four profile prompts; sweep orchestration/councils for fellowship vocab; update ARCHITECTURE.md. This is where behavior changes — agents now defer to SOUL.md for identity. Go/no-go: T8–T11, T13 pass.
3. **Bootstrap/install prompt.** `loaf install --interactive` asks which soul; bootstrap surfaces the choice via install. Drop if tracks 1+2 deliver value alone — users can run `loaf soul use <name>` post-install. Go/no-go: T14.

## Pivot Note (Post-Implementation)

This spec was implemented in full on branch `feat/configurable-agent-soul` and reviewed twice (Loaf reviewer + two Codex passes — see PR #40 history). During final review, the user (the only Loaf user to date) acknowledged that the Warden/Fellowship lore never landed for them across months of Loaf use. Combined with Codex's findings on SOUL.md as a prompt-injection surface (advisory-only enforcement, brittle by design), the soul layer was assessed as cost > benefit for the current Skills-centered Hooks-assisted iteration.

PR #40 was pivoted in-flight: the catalog, `loaf soul` CLI, install integration, SessionStart restoration, runtime distribution, and SOUL.md-read directive were removed. The agent profile neutralization and skill-prose audit were kept — those stand independently and improved the project regardless of the soul concept.

The lore concept is preserved here as a record of what was tried, and may return as a first-class element in a future Pi-based harness where it can be load-bearing rather than decorative.
