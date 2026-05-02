---
name: refactor-deepen
description: >-
  Surfaces refactoring opportunities through a deepening lens — modules that
  hide complexity behind narrow interfaces. Not for renames, extractions, or
  generic restructuring (use `/loaf:implement`). Use when looking for structural
  improvements, or when t...
user-invocable: true
argument-hint: '[module or area]'
version: 2.0.0-dev.38
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

### Linear-Native Mode: Fail Fast on PLAN Writes

Before writing a PLAN file, check the project's Linear-native flag:

```bash
node -e 'const c=JSON.parse(require("fs").readFileSync(".agents/loaf.json","utf-8"));process.exit(c.integrations?.linear?.enabled?1:0)' 2>/dev/null
```

If exit code is `1` (Linear-native enabled), abort the skill with the exact
message:

> Linear-native plan storage pending artifact-taxonomy spec — local mode only for now.

Do **not** write the PLAN file, do **not** invoke `loaf kb glossary upsert`,
and do **not** call Codex review. Partial state across local PLAN + remote
glossary is the explicit failure mode SPEC-034 forbids (see line 81 No-Gos).
The glossary `upsert` would also fail fast on its own; this rule prevents
the skill from getting that far so no orphan PLAN file lands on disk.

### Termination

If Linear-native is **not** enabled, the skill terminates by writing a PLAN
file using [templates/plan.md](templates/plan.md) at
`.agents/plans/<YYYYMMDD-HHMMSS>-<slug>.md`, with this exact closing message
(substitute the actual filename you wrote):

> Plan saved to `.agents/plans/<filename>.md`. Workflow handoff is pending
> the SPEC/PLAN/TASKS artifact taxonomy spec — for now, decide manually.

Do **not** recommend `/loaf:breakdown` or `/loaf:implement` as the next step. The
handoff design is downstream of a deferred taxonomy spec.

### Codex Review (Opt-In, Plugin-Gated)

After the PLAN file is written, check whether the `codex` plugin is
available in the current harness — i.e. whether the `codex:codex-rescue`
agent (or the `codex:rescue` skill) is exposed in this session's available
skills/agents list. Loaf's CLI-level tool detection (`cli/lib/detect/`)
discovers external CLIs like the Codex binary; it does **not** report
Claude-Code-plugin presence inside the running session. The skill-level
ambient-availability check is the right contract here — what we care about
is "can I route a review *right now*," not "is the Codex CLI installed
somewhere on this machine." If a future Loaf surface adds plugin-level
detection, this rule can switch to it.

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
  `ARCHITECTURE.md` were read before the candidate list was presented
- Every candidate module name was checked via `loaf kb glossary check` before
  being proposed
- Output uses the eight source terms verbatim — zero occurrences of
  "boundary," "service," "component," or "layer" in their place
- INTERFACE-DESIGN phase spawned exactly 3 sub-agents with identical briefs
- A `.agents/plans/<YYYYMMDD-HHMMSS>-*.md` file was written with the minimal
  shape filled out (skipped iff Linear-native mode aborted the skill before
  this step)
- If Linear-native mode is enabled, the skill exited fast with the
  artifact-taxonomy error and wrote nothing to disk
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
`/loaf:architecture` and exploratory flows.

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
   list`), `docs/decisions/ADR-*.md`, and `ARCHITECTURE.md`. Read
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
6. **Check Linear-native mode** via the inline node-one-liner above. If
   enabled, abort with the verbatim Linear-native error and skip steps 6–9.
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
