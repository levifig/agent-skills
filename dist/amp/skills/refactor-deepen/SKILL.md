---
name: refactor-deepen
description: >-
  Surfaces refactoring opportunities through a deepening lens — modules that
  hide complexity behind narrow interfaces. Not for renames, extractions, or
  generic restructuring (use `/implement`). Use when looking for structural
  improvements, or when the user asks "is this module too shallow?" or "where
  should we deepen this code?" Produces either a read-only report or a PLAN file
  with candidates, dependency categories, and proposed deepened modules.
version: 2.0.0-pre.20260625190923
---

# Refactor-Deepen

Identify shallow modules and propose deepenings that hide complexity behind
narrow, stable interfaces. Vocabulary discipline is load-bearing: drift to
"boundary," "service," "component," or "layer" defeats the entire point.

## Contents
- Critical Rules
- Verification
- Quick Reference
- Process
- Topics
- Related Skills

**Input:** $ARGUMENTS (module path, area, or "scan the repo")

---

## Critical Rules

### First Action: Self-Log

Before anything else, log invocation to the session journal:

```bash
loaf session log "skill(refactor-deepen): <module or area>"
```

### Vocabulary Discipline

The source taxonomy is **eight terms**. Use them verbatim. Do not substitute
"boundary," "service," "component," or "layer" — those words are imprecise
and erode the lens.

| Term | One-line cue |
|------|--------------|
| **Module** | A unit of code with a name, a public surface, and hidden internals |
| **Interface** | The narrow public surface a module exposes |
| **Implementation** | What lives behind the interface |
| **Depth** | Ratio of hidden complexity to interface surface (more hidden = deeper) |
| **Seam** | A point where the call graph can be cleanly cut and substituted |
| **Adapter** | The translation layer between an internal interface and an external concern |
| **Leverage** | How much downstream code is simplified per unit of interface |
| **Locality** | Whether related concerns live near each other or are scattered |

Full semantics live in [references/language.md](references/language.md). Read
that file on every invocation before naming anything.

### Glossary Integration

The skill *pressure-tests* glossary terms — it consumes existing entries and
adds new ones when a deepening clearly names a structural module.

- **Before naming a candidate module**, check the glossary:
  ```bash
  loaf kb glossary check <term>
  ```
  - Canonical hit → use the existing term verbatim.
  - Avoided-alias hit → use the canonical replacement, surface the swap to the user.
  - Unknown → proceed; if the term proves load-bearing, add it (next rule).
- **When a deepening clearly names a structural module**, add it canonically:
  ```bash
  loaf kb glossary upsert "<term>" \
    --definition "<one-line definition>" \
    --avoid "<alias-1>,<alias-2>"
  ```
  Use `upsert` (high commitment), not `propose`. A named deepening is not exploratory.
- **Linear-native mode fails fast** on `upsert`. If the user is in
  Linear-native mode, surface the error verbatim and continue without the
  glossary write — do not synthesize partial state.

### Grilling Protocol

The interview phase imports the shared
[templates/grilling.md](templates/grilling.md) template — relentless
interview, walk the decision tree, recommend per question, prefer exploration
when the codebase can answer. Do not re-derive the protocol; follow it.

### INTERFACE-DESIGN Phase: 3 Unprimed Sub-Agents

When the grilling loop reaches interface design for a candidate, spawn
**exactly 3 sub-agents with identical briefs**. Do not prime them with
opposing constraints (no "Agent 1 minimal, Agent 2 flexible"). Variety must
emerge from sampling, not manufactured opposition. If the three designs
converge accidentally, the fallback is a 4th agent or a rerun with different
seeds — *not* introducing priming. See
[references/interface-design.md](references/interface-design.md).

### Report-Only Mode

Use report-only mode when the user asks for a broad scan, full report,
repository-wide review, or anything intended to feed a later brief/task
workflow rather than immediately persist a PLAN. Report-only mode is also the
fallback when Linear-native mode blocks local PLAN/glossary writes.

Report-only mode still performs the investigation: read context, survey
candidates, check candidate names through `loaf kb glossary check`, classify
dependencies, and present findings. It must not write `.agents/plans/*`, must
not invoke `loaf kb glossary upsert`, and must not offer Codex review because
there is no PLAN artifact to review.

Report-only output should be structured so it can become a brief later:

- Scope and evidence read
- Candidate modules, ordered by expected leverage
- For each candidate: current interface, hidden implementation complexity,
  dependency category, locality problem, proposed deepened module, tests that
  should survive, and rejected alternatives
- Suggested follow-up: "Use this report to draft a brief, then decide whether
  to break that brief down into tasks."

### Whole-Repo Inventory

For broad scans, inventory source and project-owned docs first. Exclude
generated, vendored, cached, virtualenv, and build output trees unless the user
explicitly asks to inspect them.

Default excludes:

- `node_modules/`, `.next/`, `.venv/`, `venv/`, `__pycache__/`
- `dist/`, `dist-cli/`, `plugins/`, `build/`, `coverage/`, `.turbo/`
- generated lock/vendor output and other tool caches

Use `rg --files` with `-g` excludes or targeted `find` commands rather than a
blind recursive listing. If an excluded tree appears relevant, name it as an
assumption and ask before pulling it into context.

### Linear-Native Mode: Disable Local Writes

Before writing a PLAN file or mutating the glossary, check the project's
Linear-native flag:

```bash
node -e 'const c=JSON.parse(require("fs").readFileSync(".agents/loaf.json","utf-8"));process.exit(c.integrations?.linear?.enabled?1:0)' 2>/dev/null
```

If exit code is `1` (Linear-native enabled), continue in report-only mode and
surface the exact storage constraint once:

> Linear-native plan storage pending artifact-taxonomy spec — continuing with a read-only report.

Do **not** write the PLAN file, do **not** invoke `loaf kb glossary upsert`,
and do **not** call Codex review. Partial state across local PLAN + remote
glossary is the explicit failure mode SPEC-034 forbids (see line 81 No-Gos).
The report is allowed because it is read-only and can feed a later brief.

### Termination

If report-only mode is active, terminate with the report in chat and this
closing message:

> Report complete. Use this report to draft a brief, then decide whether to
> break that brief down into tasks.

If report-only mode is **not** active and Linear-native is **not** enabled, the
skill terminates by writing a PLAN file using [templates/plan.md](templates/plan.md) at
`.agents/plans/<YYYYMMDD-HHMMSS>-<slug>.md`, with this exact closing message
(substitute the actual filename you wrote):

> Plan saved to `.agents/plans/<filename>.md`. Workflow handoff is pending
> the SPEC/PLAN/TASKS artifact taxonomy spec — for now, decide manually.

Do **not** recommend `/breakdown` or `/implement` as the next step. The
handoff design is downstream of a deferred taxonomy spec.

### Codex Review (Opt-In, Plugin-Gated)

After the PLAN file is written, check whether the `codex` plugin is
available in the current harness — i.e. whether the `codex:codex-rescue`
agent (or the `codex:rescue` skill) is exposed in this session's available
skills/agents list. Loaf's install-time tool detection discovers external CLIs
like the Codex binary; it does **not** report Claude-Code-plugin presence
inside the running session. The skill-level ambient-availability check is the
right contract here — what we care about is "can I route a review *right now*,"
not "is the Codex CLI installed somewhere on this machine." If a future Loaf
surface adds plugin-level detection, this rule can switch to it.

- **Always** make this offer plugin-gated and opt-in. Ask once, verbatim:
  > Want a Codex review of this deepening before commit?
  Wait for an affirmative reply before invoking anything.
- **Always** route an accepted review through the `codex:codex-rescue`
  agent (or the `codex:rescue` skill if that's what the harness exposes),
  passing the PLAN file path and a pointer to
  [references/deepening.md](references/deepening.md) for vocabulary context.
- **Never** mention Codex if the plugin is absent — proceed straight to the
  termination message below.
- **Never** invoke Codex review by default, on a non-affirmative reply, or
  on a "skip" — fall through to termination.

---

## Verification

- Session journal contains `skill(refactor-deepen):` entry as the first action
- `docs/knowledge/glossary.md`, `docs/decisions/ADR-*.md`, and
  `docs/ARCHITECTURE.md` were read before the candidate list was presented
- Every candidate module name was checked via `loaf kb glossary check` before
  being proposed
- Output uses the eight source terms verbatim — zero occurrences of
  "boundary," "service," "component," or "layer" in their place
- INTERFACE-DESIGN phase spawned exactly 3 sub-agents with identical briefs
- Whole-repo scans excluded generated/vendor/cache/build trees unless the user
  explicitly asked to inspect them
- Report-only mode produced a complete read-only report and wrote nothing to
  `.agents/plans/*`
- A `.agents/plans/<YYYYMMDD-HHMMSS>-*.md` file was written with the minimal
  shape filled out (skipped iff report-only mode was active)
- If Linear-native mode is enabled, the skill continued in report-only mode and
  wrote nothing to disk
- Codex review offer fires only when the `codex` plugin is detected, is
  worded verbatim, and runs only on an affirmative reply — never by default
- Closing message matches the termination template verbatim

---

## Quick Reference

### PLAN Filename

```bash
slug="<lowercase-hyphenated-title>"
date -u +%Y%m%d-%H%M%S | xargs -I{} echo ".agents/plans/{}-${slug}.md"
```

PLAN files use the same temporal-record naming as sessions, ideas, drafts,
and councils — write-once snapshots, not sequentially-numbered contracts.
Re-deepening the same module later writes a new file rather than updating
the existing one. The filename is the identity; PLAN frontmatter does not
carry an `id` field.

Create `.agents/plans/` lazily — `mkdir -p` immediately before the first
plan write, never upfront. The sequential-ID allocation race is gone;
same-second filename collisions remain theoretically possible at scripted
speed but are unlikely at human pace.

### Glossary CLI Cheat Sheet

| Verb | When |
|------|------|
| `loaf kb glossary check <term>` | Before naming any candidate module |
| `loaf kb glossary upsert <term> --definition <d> --avoid <list>` | When a deepening names a structural module |
| `loaf kb glossary list` | At interview start (see grilling protocol) |

`propose` and `stabilize` are not used by this skill — they belong to
`/architecture` and exploratory flows.

### When to Deepen vs. Leave Alone

| Signal | Deepen? |
|--------|---------|
| Interface surface ≈ implementation surface | Yes — module is shallow |
| Callers reach across the interface to internals | Yes — interface is leaky |
| One module concern is scattered across many files | Yes — locality is poor |
| Interface is narrow and implementation is large | No — already deep |
| Module is a thin pass-through to a true-external dependency | No — adapter, by design |

---

## Process

1. **Read context.** Open `docs/knowledge/glossary.md` (via `loaf kb glossary
   list`), `docs/decisions/ADR-*.md`, and `docs/ARCHITECTURE.md`. Read
   [references/language.md](references/language.md) and
   [references/deepening.md](references/deepening.md).
2. **Survey candidates.** Walk the target module/area. Produce a numbered list
   of shallow-module candidates with one-line rationales using the eight terms.
3. **Pick a candidate** (with the user) and enter the grilling loop following
   [templates/grilling.md](templates/grilling.md). Surface canonical terms
   inline; challenge drift on contact.
4. **Classify dependencies.** For each candidate, classify dependencies by
   category (in-process, local-substitutable, ports-and-adapters, true-external)
   per [references/deepening.md](references/deepening.md).
5. **Design the interface** by spawning 3 unprimed sub-agents per the rules in
   [references/interface-design.md](references/interface-design.md). Present
   all three designs to the user; do not pre-rank.
6. **Check report-only and Linear-native mode.** For broad reports, or when the
   inline Linear-native check exits `1`, continue in report-only mode and skip
   steps 7-9.
7. **Write the PLAN** to `.agents/plans/<YYYYMMDD-HHMMSS>-<slug>.md` using
   [templates/plan.md](templates/plan.md). The filename timestamp must match
   the frontmatter `created` field. Required sections: candidate, dependency
   category, proposed deepened module, what survives in tests, rejected
   alternatives.
8. **Mutate the glossary** (`loaf kb glossary upsert`) for any structural
   module the PLAN names canonically.
9. **Offer Codex review** only if the `codex` plugin is detected (i.e.
   `codex:codex-rescue` agent or `codex:rescue` skill is available). Ask
   verbatim and wait for an affirmative reply; on accept, route through
   `codex:codex-rescue` with the PLAN file path. On decline, skip, or
   plugin absence, fall through silently.
10. **Terminate** with the canonical message.

---

## Topics

| Topic | Reference | Use When |
|-------|-----------|----------|
| Vocabulary | [references/language.md](references/language.md) | Naming any module, interface, or seam — read first, every invocation |
| Deepening Patterns | [references/deepening.md](references/deepening.md) | Classifying dependencies and applying seam discipline |
| Interface Design | [references/interface-design.md](references/interface-design.md) | Running the parallel 3-agent INTERFACE-DESIGN phase |
| Grilling Protocol | [templates/grilling.md](templates/grilling.md) | Interview discipline during the candidate-selection loop |
| PLAN Template | [templates/plan.md](templates/plan.md) | Writing the terminating PLAN artifact |

---

## Related Skills

- **architecture** — Stabilizes glossary terms during ADR interviews; consumes the same `grilling.md` template
- **knowledge-base** — Owns the underlying KB and glossary CLI verbs
- **implement** — Where generic refactors (rename, extract, restructure) belong
