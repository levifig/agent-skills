---
id: SPEC-012
title: Cleanup Refinement — differentiated processes + CLI
source: direct
created: '2026-03-16T23:50:30.000Z'
status: complete
---

# SPEC-012: Cleanup Refinement

## Problem Statement

The `/cleanup` skill already defines differentiated handling rules for every artifact type (sessions, tasks, specs, plans, drafts, councils, reports) — but those rules only execute when an agent reads the skill and follows its instructions. There's no CLI surface for humans to run cleanup directly, no way to automate or test the rules, and no guardrails against accidental data loss. The gap is CLI automation of existing policy, not defining new policy.

## Strategic Alignment

- **Vision:** Supports the "knowledge management" pillar by ensuring agent artifacts don't accumulate as noise.
- **Personas:** Framework user gets a `loaf cleanup` command for project hygiene. Agents get clearer rules for artifact lifecycle.
- **Architecture:** Builds on SPEC-010's task management CLI pattern (CLI mediates operations).

## Solution Direction

### Part 1: CLI Implementation of Existing Cleanup Rules

The cleanup skill already defines these rules. The CLI implements them programmatically:

| Artifact | State | Action | Preservation |
|----------|-------|--------|-------------|
| **Sessions** | Completed | Archive (move to archive/) | Full — set `archived_at`, `archived_by` |
| **Sessions** | Stale (>7 days inactive) | Flag for review | Never auto-delete |
| **Sessions** | Cancelled/abandoned | Archive with `status: cancelled` | Preserved with reason |
| **Tasks** | Done | Archive via `loaf task archive` (per-task or `--spec`) | Full — uses existing archive helpers |
| **Tasks** | Orphaned (no spec match) | Flag for review | Never auto-delete |
| **Specs** | Complete | Archive via `loaf spec archive` | Full — uses existing archive helpers |
| **Specs** | Stale drafting | Flag for review | Never auto-delete |
| **Plans** | Session archived | Delete | Ephemeral — decisions in session |
| **Plans** | Orphaned (no linked session) | Delete | Ephemeral — no value without session |
| **Drafts** | Promoted to spec | Archive or delete (user choice) | Ask first |
| **Drafts** | Stale (>30 days) | Flag for review | Never auto-delete |
| **Councils** | Session summary captured | Archive | Full preservation |
| **Reports** | Processed + session archived | Archive | Full preservation |

**Core principle:** Archive, don't delete — except for truly ephemeral artifacts (plans) whose value lives entirely in session files and git history.

### V1 Artifact Contract

| Directory | Status | Notes |
|-----------|--------|-------|
| `sessions/` | **Required** | Scaffolded by `loaf init` |
| `specs/` | **Required** | Scaffolded by `loaf init` |
| `tasks/` | **Required** | Scaffolded by `loaf init` |
| `ideas/` | **Required** | Scaffolded by `loaf init`. Not scanned — ideas have no lifecycle state to clean up |
| `plans/` | Optional | Created by workflow on demand |
| `drafts/` | Optional | Created by workflow on demand |
| `councils/` | Optional | Created by workflow on demand |
| `reports/` | Optional | Created by workflow on demand |

- Missing optional directories → skip silently, no empty dir creation
- Missing required directories → warn and continue (partial init or old checkout)
- Unknown directories inside `.agents/` → ignore
- Each archive target gets an `archive/` subdirectory created on first use

### Part 2: `loaf cleanup` CLI Command

A new CLI command that automates scanning and reporting:

```
loaf cleanup              # Scan all artifacts, show summary + recommendations
loaf cleanup --dry-run    # Same but don't prompt for actions
loaf cleanup --sessions   # Only review sessions
loaf cleanup --specs      # Only review specs
loaf cleanup --plans      # Only review plans
loaf cleanup --drafts     # Only review drafts
```

The command:
1. Scans `.agents/` directories for each artifact type (per V1 contract above)
2. Applies the existing cleanup rules
3. Shows a formatted summary table (like `loaf task list`)
4. For each actionable item, prompts: Archive / Delete / Keep / Skip
5. Performs the chosen action (move, delete, update status)
6. Suggests `/crystallize` for sessions with extractable learnings

#### CLI Interaction Contract

- **Prompts:** Reuse `askYesNo()` / `askChoice()` from `release.ts` (extract to shared `cli/lib/prompts.ts` if needed)
- **Per-item confirmation:** Each actionable artifact is confirmed individually (not batch). Matches `loaf release` UX pattern.
- **Delete previews:** Show first 3 lines of frontmatter before any destructive action.
- **`--dry-run`:** Print recommendations table, exit 0, no prompts.
- **Non-TTY (piped):** Behave like `--dry-run` — report only, no prompts. Detect via `process.stdout.isTTY`.
- **75% circuit-breaker mode** (scanning + dry-run only) is functionally identical to `--dry-run` shipping as the only mode.

### Part 3: Skill Update

Update the `/cleanup` skill to reference the CLI command. The skill becomes guidance for agents on _when_ to invoke `loaf cleanup`, while the CLI does the actual work. The differentiated rules stay in the skill as the authoritative source; the CLI is the execution engine.

## Scope

### In Scope
- `loaf cleanup` CLI command with scanning, reporting, and interactive actions
- `--dry-run` and artifact-type filters (`--sessions`, `--specs`, `--plans`, `--drafts`)
- Non-TTY detection (pipe-safe output)
- Archive operations (move + set metadata) for sessions, specs, councils, reports
- Delete operations for ephemeral artifacts (plans)
- Reuse existing `archiveTasks()` / `archiveSpecs()` from `migrate.ts`
- Extract prompt helpers to shared `cli/lib/prompts.ts` if needed
- Suggest `/crystallize` for sessions with extractable learnings
- Update `/cleanup` skill to reference CLI
- Register command in `cli/index.ts`

### Out of Scope
- Automatic cleanup (always interactive or dry-run)
- Linear integration (existing, not changing)
- Knowledge staleness detection (SPEC-009)
- The `/crystallize` skill itself (SPEC-011)
- Spec `completed_at` data model changes (follow-up spec or fold into SPEC-014)
- Branch → PR suggestions (already in implement skill's AFTER flow)

### Rabbit Holes
- Building a TUI for cleanup — stick with sequential prompts like `loaf release`
- Smart staleness detection beyond file dates — keep it simple (mtime, frontmatter dates)
- Recursive archive scanning for cleanup — `.agents/` subdirectories are one level deep

### No-Gos
- Don't auto-delete anything without user confirmation
- Don't archive sessions without checking for extractable learnings first
- Cleanup **runtime actions** (archive, delete, move) only operate on `.agents/` contents — CLI registration, skill updates, and test files are normal implementation work
- Don't require Linear for any functionality
- Don't create empty directories for optional artifact types

## Risks

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Users accidentally delete drafts with unique content | Low | High | Always show content preview before delete; default to "Keep" |
| Cleanup removes plans that active sessions reference | Low | Medium | Check session status before deleting linked plans |
| Edge cases in frontmatter parsing | Medium | Low | Reuse SPEC-010's gray-matter parser; graceful fallbacks |

## Open Questions

- [x] Appetite → Medium (3-5 days)
- [x] CLI or skill-only → CLI command + skill update

## Test Conditions

- [ ] `loaf cleanup` scans required + present optional directories and shows formatted summary
- [ ] `loaf cleanup --dry-run` shows recommendations table without prompting for actions
- [ ] `loaf cleanup --sessions` filters to sessions only (same for `--specs`, `--plans`, `--drafts`)
- [ ] Non-TTY invocation (piped) behaves like `--dry-run`
- [ ] Missing optional directories (plans, drafts, councils, reports) are skipped silently
- [ ] Completed sessions are offered for archive with `archived_at` and `archived_by` metadata
- [ ] Stale sessions (>7 days) are flagged but not auto-actioned
- [ ] Done tasks are offered for archive via existing `archiveTasks()` helper
- [ ] Orphaned plans (no linked session) are offered for deletion
- [ ] Plans linked to archived sessions are offered for deletion
- [ ] Drafts >30 days old are flagged for review
- [ ] Delete operations show frontmatter preview and require confirmation
- [ ] Archive operations move files to `archive/` subdirectory (created on first use)
- [ ] Suggests `/crystallize` for sessions with key decisions or lessons

## Circuit Breaker

At 50%: Drop artifact-type filters (`--sessions`, `--specs`, etc.). Ship `loaf cleanup` with full scan only.

At 75%: Drop interactive actions. Ship scanning + dry-run reporting only. User manually archives/deletes based on the report.
