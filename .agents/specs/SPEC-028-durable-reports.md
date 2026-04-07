---
id: SPEC-028
title: Durable Reports — reports/ as concluded artifact store
status: implementing
created: 2026-04-07
branch: feat/durable-reports
---

# SPEC-028: Durable Reports

## Problem

Research output currently lands in `.agents/drafts/` as disposable working documents. Promotion to `.agents/reports/` requires a manual copy-and-reformat step using a separate template. This means research findings are treated as throwaway artifacts by default, and the promotion path has enough friction that it rarely happens.

Meanwhile, `.agents/reports/` already exists and is used by background agents and orchestration, but the research skill — the primary producer of investigative output — doesn't write there directly.

The result: valuable research findings live in `drafts/` alongside brainstorm scratch and are eventually cleaned up by housekeeping without being preserved.

## Solution Direction

Make `reports/` the default output location for all concluded investigative work. Research writes directly to `reports/` with a lifecycle status (`draft → final → archived`). A single unified template replaces the current separate findings and report templates. Full CLI support (`loaf report`) and a lifecycle policy for ephemeral drafts round out the feature.

## Scope

### In

- **Research skill** writes directly to `.agents/reports/` instead of `.agents/drafts/`
- **Single report template** with status lifecycle replaces findings + report templates
- **Multi-type support**: `research`, `audit`, `analysis`, `council` (template already has this)
- **Council skill** output goes to `reports/` (type: council)
- **Background agent** output already goes to `reports/` — no change needed
- **Housekeeping** already handles `reports/` lifecycle (done this session)
- **Remove findings template** — no longer needed
- **Update research skill** — Topic Investigation mode writes to `reports/`, not `drafts/`
- **`loaf report` CLI** — `list`, `create`, `finalize`, `archive` (mirrors `loaf spec`/`loaf task` pattern)
- **Drafts lifecycle policy** — housekeeping flags state assessments for cleanup when linked session is archived

### Out

- **Brainstorms** stay in `drafts/` (ephemeral working docs with spark extraction)
- **State assessments** stay in `drafts/` (point-in-time snapshots, go stale fast)
- **Brainstorm → spark extraction pipeline** changes

## Changes

### 1. Unified report template

Merge findings template and report template into one. Keep in `content/skills/research/templates/report.md`.

```yaml
---
title: "Report: [Topic]"
type: research | audit | analysis | council
created: YYYY-MM-DDTHH:MM:SSZ
status: draft | final | archived
source: SPEC-XXX | TASK-XXX | ad-hoc
tags: []
---
```

Key sections: Summary, Key Findings (with confidence), Methodology, Detailed Analysis, Recommendations, Sources, Open Questions.

Status lifecycle:
- `draft` — work in progress, findings incomplete
- `final` — conclusions validated, recommendations made
- `archived` — processed, linked session archived, moved to `archive/`

### 2. Research skill updates

- **Topic Investigation** output path changes from `.agents/drafts/` to `.agents/reports/`
- Remove the "promote to reports/" step — research writes there from the start
- Remove findings template reference from Topics table
- Update SKILL.md Topic Investigation section (line ~107-111)
- Status starts as `draft`, skill sets to `final` when research concludes

### 3. Template cleanup

- Delete `content/skills/research/templates/findings.md`
- Update `content/skills/research/templates/report.md` with unified template
- Remove "Promotion from Draft" section from report template (no longer needed)
- Keep `content/skills/research/templates/state-assessment.md` (stays in drafts/)

### 4. Housekeeping template update

- Update `content/skills/housekeeping/templates/report.md` to match new frontmatter
- Already has lifecycle awareness (done this session)

### 5. `loaf report` CLI

New command: `cli/commands/report.ts`. Follows the `loaf spec` / `loaf task` pattern (Commander.js, ANSI output).

**Subcommands:**

| Command | Description |
|---------|-------------|
| `loaf report list` | List reports grouped by status, filterable by `--type` and `--status` |
| `loaf report create <slug>` | Scaffold new report from template with pre-filled frontmatter (`--type`, `--source`) |
| `loaf report finalize <file>` | Transition `status: draft → final`, set `finalized_at` timestamp |
| `loaf report archive <file>` | Move to `archive/`, set `archived_at` and `archived_by`, validate linked session is archived |

Register in `cli/index.ts` alongside existing commands.

### 6. Drafts lifecycle policy (housekeeping)

Add lifecycle rules for state assessments in `drafts/`:

- **Trigger:** Linked session is archived (not age-based)
- **Action:** Housekeeping flags state assessments whose linked session is archived for cleanup
- **Matching:** State assessments have `type: state-assessment` in frontmatter; linked session is inferred from the session file that was active when the assessment was created (or from a `session:` frontmatter field if present)

Update `content/skills/housekeeping/SKILL.md`:
- Add drafts lifecycle policy to Critical Rules
- Add state-assessment cleanup to Artifact Lifecycle table (trigger: linked session archived)

### 7. Cross-references

Update references to drafts/ in:
- `content/skills/research/SKILL.md` — Topic Investigation output path
- `content/skills/research/templates/findings.md` — delete
- `content/skills/research/templates/report.md` — remove promotion section

Leave alone (out of scope):
- Brainstorm references to drafts/
- Idea origin tracking referencing drafts/
- Triage spark scanning of drafts/brainstorm files

## Rabbit Holes

- **Don't redesign brainstorm pipeline** — brainstorms stay in drafts/ with spark extraction
- **Don't add new report types** — the template supports arbitrary types; don't gate-keep
- **Don't over-engineer CLI** — keep report commands simple; no index file (REPORTS.json) unless needed later

## Test Conditions

### Skill & Template
- [ ] Research skill Topic Investigation writes to `.agents/reports/`, not `.agents/drafts/`
- [ ] Report template has unified frontmatter with status lifecycle
- [ ] Findings template is deleted
- [ ] State assessments still write to `.agents/drafts/`
- [ ] Brainstorm output unchanged (still `.agents/drafts/`)

### CLI
- [ ] `loaf report list` shows reports grouped by status
- [ ] `loaf report list --type research` filters correctly
- [ ] `loaf report create my-topic --type research` scaffolds report with correct frontmatter
- [ ] `loaf report finalize <file>` sets `status: final` and `finalized_at`
- [ ] `loaf report archive <file>` moves to `archive/`, validates linked session
- [ ] Error handling: archive fails if linked session not yet archived

### Housekeeping
- [ ] Housekeeping flags state assessments in `drafts/` when linked session is archived
- [ ] Housekeeping does NOT flag brainstorm drafts (no linked-session cleanup for brainstorms)
- [ ] Housekeeping can archive reports with the new frontmatter schema

### Build
- [ ] `loaf build` succeeds across all targets
- [ ] `npm run typecheck` passes
- [ ] All tests pass
- [ ] Built output for all targets reflects the updated skill

## Strategic Alignment

This aligns with the knowledge management direction — making investigative output durable and searchable rather than ephemeral. It's a step toward reports becoming a first-class artifact type alongside specs and sessions.
