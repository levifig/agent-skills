---
name: architecture
description: >-
  Creates Architecture Decision Records for architecturally significant
  decisions — choices that affect the system's structure, key quality
  attributes, dependencies, interfaces, or construction techniques, or that are
  difficult to reverse. Captures ...
user-invocable: true
argument-hint: '[topic or decision]'
version: 2.0.0-dev.39
---

# Architecture

Guides decision-making for **architecturally significant** choices through structured interviews, options analysis, and Architecture Decision Records. ADRs are **rare yet binding** — they record the rationale for choices that shape the system's structure, quality attributes, dependencies, interfaces, or construction techniques, and that the team agrees to honor until explicitly superseded. Most technical decisions do not warrant an ADR; the skill routes those to their proper destination (session log, SPEC, ARCHITECTURE.md, or owning skill) and stops.

Stabilizes canonical vocabulary in `docs/knowledge/glossary.md` when load-bearing terms surface mid-interview — additive to ADR creation, never a gate on it.

## Critical Rules

### The Bar

ADRs are reserved for **architecturally significant** decisions — those affecting:

- **Structure** (system boundaries, modules, layering)
- **Quality attributes** (performance, security, reliability, scalability)
- **Dependencies** (external services, libraries with broad reach, runtime/language commitments)
- **Interfaces** (public APIs, contracts between teams, cross-system protocols)
- **Construction techniques** (build system, deployment model, testing strategy at the system level)

…**or** are **difficult to reverse** in the project's current state (would require coordination beyond a single PR — schema migration, contract change, multi-skill update, external-tool retraining).

The bar is a **disjunction**: either canonical-domain effect, or difficulty of reversal, satisfies it (Microsoft Well-Architected). The Triage Gate below operationalizes the bar more strictly — `(Q1 OR Q2) AND Q3` — to keep ADRs rare and binding.

The bar is constant. **The number of decisions clearing it scales with project maturity.** Early/exploratory projects clear the bar rarely — usually only foundational shape commitments (language/runtime/standard adoption, primary architectural shape). Mature projects clear it more often as cost-of-change rises across the codebase. **When in doubt during early phases, prefer SPEC over ADR** — SPECs evolve, ADRs supersede. In early phases, foundational commitments typically pass via Q2's "Later" prong — the future-cost is the reason to record the rationale while it's fresh, not the current-cost.

**An ADR captures a choice.** At least one credible alternative was considered and rejected. Without alternatives, you have a principle, vision, or aspiration — record those in `ARCHITECTURE.md` or `VISION.md` instead. The presence of an "Alternatives Considered" section in the ADR template is structural, not optional.

### Triage Gate

Before grilling, confirm with the user that the decision passes the gate:

1. **Architectural significance** — does it affect *structure, quality attributes, dependencies, interfaces, or construction techniques*?
2. **Cost of divergence** — if the team casually diverged from this, what's the consequence? Either:
   - **Now:** multi-PR coordination, security regression, contract or interface break
   - **Later:** this is a foundational shape commitment (runtime/language/standard adoption, primary boundary) whose future reversal cost is the reason to record the rationale now
3. **Rationale durability** — when this debate returns in 18 months, would the team need the *why* reconstructed, or is it self-evident from the code?

**Gate logic: `(Q1 OR Q2) AND Q3` → proceed to grilling and ADR.**

Q1 and Q2 form a disjunction (matches Microsoft's bar — either canonical-domain effect or difficulty-of-reversal qualifies). Q3 is required regardless: even an architecturally significant or hard-to-reverse decision doesn't need an ADR if its rationale is self-evident from the code itself. Failing the gate → route per the table below and stop.

| Decision shape | Destination |
|---|---|
| Passes the gate | **ADR** (`docs/decisions/`) |
| Development pattern, direction, implementation evidence | SPEC via `/loaf:shape` |
| Guiding principle, philosophy, operating model (stance, not architectural choice) | `ARCHITECTURE.md` / `VISION.md` (mutable, `/loaf:reflect`-revisable) |
| Workflow convention, skill-specific lore | Owning skill's `SKILL.md` or references |
| Local choice, single-PR scope, no consequence to divergence | Session-log `decision(scope)` + code comment if needed |

### Skip ADR When

- Decision is a convention or naming preference — no measurable effect on architecture (fails the architectural-significance test)
- Decision is a stance, principle, philosophy, or vision — even if alternatives are named, the choice is being made on philosophical or operational grounds rather than architectural ones (specific quality attributes, dependencies, interfaces, or construction techniques). Record in `ARCHITECTURE.md` or `VISION.md` (strategic), where principles can evolve via `/loaf:reflect`. ADRs are immutable post-acceptance and reserved for architectural choices.
- Decision is workflow lore belonging to a specific skill — document in that skill, not in `docs/decisions/`
- Decision is exploration of alternatives without a chosen direction — that's a SPEC via `/loaf:shape`; the ADR comes after if the chosen direction is architecturally significant
- Decision can be changed in a single PR with no downstream coordination — session-log `decision(scope)` and a code comment if needed
- Rationale is aesthetic ("looks/feels better", "scans nicer", "more consistent visually") — never an ADR

### Lifecycle

ADRs are **append-only post-acceptance**. The original `Decision`, `Context`, `Rationale`, and `Consequences` sections are immutable — don't rewrite history.

Five statuses: `Proposed` | `Accepted` | `Rejected` | `Deprecated` | `Superseded`.

What's permitted post-acceptance:
- **Status transitions:** `Accepted → Deprecated`, `Accepted → Superseded`, `Proposed → Rejected`, `Proposed → Accepted`
- **Frontmatter additions** per the schema below (transition dates, supersession linkage)
- **Append-only `## Deprecated`, `## Rejected`, or `## Superseded` sections** capturing the lifecycle change

**Frontmatter schema:**

```yaml
---
id: ADR-NNN
title: "..."
status: Proposed | Accepted | Rejected | Deprecated | Superseded
date: YYYY-MM-DD            # creation / proposal
accepted_date: YYYY-MM-DD   # optional — only if differs from `date`
rejected_date: YYYY-MM-DD   # required iff status is Rejected
deprecated_date: YYYY-MM-DD # required iff status is Deprecated
supersedes: ADR-NNN         # optional — points back to the ADR this replaces
superseded_by: ADR-NNN      # required iff status is Superseded
---
```

Frontmatter encodes the structured *what* and *when* of a status transition. Context for *why* and *where-it-went* belongs in the body section (`## Deprecated` / `## Rejected`). Don't duplicate `reason` or `migrated_to` as frontmatter fields — they're prose.

**Body-section requirements by status:**

- `## Deprecated` is **required** when status is `Deprecated`. Explains why and points to the new home (if migrated).
- `## Rejected` is **required** when status is `Rejected`. Explains why the proposal was rejected and what was chosen instead (if anything).
- `## Superseded` is **optional** when status is `Superseded`. The `superseded_by:` linkage carries the structural relationship; the new ADR carries the rationale. Add a body section only if the supersession had specific reasons worth preserving on the old record.

A `Rejected` ADR is a record of "the team weighed this option and explicitly chose against it" — useful when the same idea resurfaces. Keep them.

When the team's answer to a decision changes, write a **new ADR that supersedes the old one** — set `supersedes: ADR-NNN` on the new one and `superseded_by: ADR-MMM` on the old one. Both stay in `docs/decisions/`. The old one preserves the historical "_was_ the decision, _no longer_ the decision" record (Nygard).

When a record is **recategorized** (the underlying choice still holds, but the artifact-classification was wrong — e.g., it was actually a principle, convention, or workflow lore), mark it `Deprecated` and explain the migration target in the `## Deprecated` body section. The original record is preserved; the active source is the migrated content. This is distinct from supersession: nothing's been *replaced*; only the *classification* changed.

Supersession is healthy. The bar for *writing* an ADR is high; once written, the bar for *quietly diverging from* it is also high (write a superseding ADR instead).

**Always**
- Run the Triage Gate before grilling. If the gate fails — `(Q1 OR Q2) AND Q3` is not satisfied — route to the destination in the table and stop.
- When a decision is architecturally significant but routes elsewhere (SPEC, ARCHITECTURE.md, owning skill), help the user place it in the right destination. The skill's job is correct routing, not just ADR creation.
- Read existing VISION.md, ARCHITECTURE.md, and ADRs before proposing changes
- Read `docs/knowledge/glossary.md` at interview start (via `loaf kb glossary list`); use canonical terms throughout
- When fuzzy/drifted language surfaces, challenge inline; if a load-bearing term emerges, offer `loaf kb glossary upsert` or `stabilize`
- Follow the shared interview protocol in [templates/grilling.md](templates/grilling.md)
- Present multiple options with pros/cons and "fits when" context
- Wait for explicit user decision before proceeding with documentation
- Log decision to session journal: `loaf session log "decision(architecture): ADR-NNN adopted for X"`

**Never**
- Make architectural decisions without user input
- Contradict existing decisions without explicitly superseding them
- Proceed past the Triage Gate when it fails — i.e., when neither Q1 nor Q2 affirms, or when Q3 fails. ADRs require `(Q1 OR Q2) AND Q3`.
- Create ADRs without user approval, even when the user requests one — if the decision fails the Triage Gate, propose the correct destination and decline the ADR
- Use the word "irreversible" — software decisions can always be reversed via supersession; the operative criterion is "difficult to reverse"
- ADR-ify aesthetic preferences, naming conventions, workflow lore, or guiding principles — those have other homes (see Skip ADR When)
- Rewrite accepted ADRs' Decision/Context/Rationale/Consequences sections — those are immutable. Status transitions, frontmatter additions, and append-only Deprecated/Rejected/Superseded sections are how lifecycle is recorded
- Block ADR creation on glossary state — glossary mutations are additive and opt-in
- Call `loaf kb glossary propose` (reserved for upstream ambiguity-resolving skills)

## Verification

After work completes, verify:
- Triage Gate ran before any grilling; gate passed `(Q1 OR Q2) AND Q3` before proceeding
- Decision passes the bar: architecturally significant (canonical domains) OR difficult to reverse (cost of divergence)
- ADR captures rationale, not exploration (exploration belongs in a SPEC)
- If the decision supersedes a prior ADR, the new ADR has `supersedes:` and the old one has `superseded_by:`
- ADR created using template at [templates/adr.md](templates/adr.md)
- ARCHITECTURE.md updated with new constraints and ADR reference
- ADR number assigned sequentially (ls docs/decisions/ADR-*.md for next number)
- Council convened if decision affects multiple domains
- Glossary read at interview start; any load-bearing term surfaced was offered for `stabilize` or `upsert`

## Quick Reference

### Routing Cheat Sheet

| Decision shape | Destination |
|---|---|
| Passes the gate | **ADR** (`docs/decisions/`) |
| Development pattern, direction, implementation evidence | SPEC via `/loaf:shape` |
| Guiding principle, philosophy, operating model (stance, not architectural choice) | `ARCHITECTURE.md` / `VISION.md` (mutable, `/loaf:reflect`-revisable) |
| Workflow convention, skill-specific lore | Owning skill's `SKILL.md` or references |
| Local choice, single-PR scope, no consequence to divergence | Session-log `decision(scope)` + code comment if needed |

### ADR Numbering

```bash
ls docs/decisions/ADR-*.md 2>/dev/null | \
  grep -oE 'ADR-[0-9]+' | sort -t- -k2 -n | tail -1 | awk -F- '{print $2 + 1}'
```

Start with `ADR-001` if none exist.

### Council Triggers

Convene when: multiple domains affected, conflicting team opinions, high reversal cost, novel problem, or user requests deliberation.

### Evaluation Criteria

For each option: alignment with VISION/ARCHITECTURE, complexity added, reversibility, team capability, maintenance cost.

### Glossary Mutation Policy

This skill *stabilizes* terms — promote a previously-proposed candidate, or write a canonical term directly when one emerges mid-interview.

| Verb | When |
|------|------|
| `loaf kb glossary list` | At interview start (via grilling protocol) |
| `loaf kb glossary check <term>` | When a term's status is in question during the interview |
| `loaf kb glossary stabilize <term>` | A previously-proposed candidate has firmed up into a load-bearing decision |
| `loaf kb glossary upsert <term> --definition <d> --avoid <list>` | A load-bearing term emerges fresh and is canonical from the outset |

`propose` is reserved for upstream skills that resolve ambiguity (e.g., a future `/loaf:shape` evolution) — do not call it here.

## Topics

| Topic | Reference | Use When |
|-------|-----------|----------|
| ADR Template | [templates/adr.md](templates/adr.md) | Creating new architecture decision records |
| Grilling Protocol | [templates/grilling.md](templates/grilling.md) | Running the structured interview, including glossary discipline |
| Council Workflow | `orchestration/references/councils.md` | Multi-agent deliberation for complex decisions |
| Documentation | `documentation-standards/references/documentation.md` | ADR formatting and standards |
| Canonical ADR sources | [https://adr.github.io/](https://adr.github.io/) | Reference for ADR practice; format hub |
| AWS ADR Process | [AWS prescriptive guidance](https://docs.aws.amazon.com/prescriptive-guidance/latest/architectural-decision-records/adr-process.html) | "Architecturally significant" framing, separate-design-from-decision principle |
| Microsoft Well-Architected ADR | [Azure docs](https://learn.microsoft.com/en-us/azure/well-architected/architect-role/architecture-decision-record) | "Difficult to reverse" criterion, append-only log discipline |
| Nygard original | [Documenting Architecture Decisions (2011)](https://cognitect.com/blog/2011/11/15/documenting-architecture-decisions.html) | Foundational article; supersession lifecycle |
| Architecturally Significant Requirements | [Wikipedia](https://en.wikipedia.org/wiki/Architecturally_significant_requirements) | ASR test — measurable effect on architecture |
