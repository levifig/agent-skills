---
description: >-
  Runs the human problem-discovery ceremony: grills problem, who has it, current
  alternatives, value proposition, and constraints, then hands a sharpened
  problem narrative to shape or authors project docs/BRIEF.md. Use when the user
  invokes pitch, starts work on a raw concept, or triage dispositions a spark or
  idea as pitch. Produces a problem-space narrative and a shape-now or park
  offer — never a bounded issue, criteria, or PRs. Not for quick capture (use
  idea), solution bounding (use shape), queue processing (use triage), or
  open-ended divergent inquiry (use explore as an agent technique when pitch
  reveals the direction is undecided).
version: 0.5.0
---

# Pitch

Human problem-discovery ceremony. Narrows sparks and ideas into a framed problem so shape can mint an issue, and bootstrap can consume a pitched project BRIEF.

## Contents
- Critical Rules
- Verification
- Quick Reference
- Process
- Related Skills
- Topics

**Input:** $ARGUMENTS

---

## Critical Rules

1. **Agents never initiate a pitch.** This ceremony is human-invoked only. On Claude Code the sidecar sets `disable-model-invocation: true`; on every target this rule binds behaviorally. Agent legwork *inside* a human-opened pitch (competitive scans, file writes the skill directs) is fine — opening one is not.
2. **Log invocation first** — `loaf journal log "skill(pitch): <idea, problem, spark, or intake item>"` before interviewing.
3. **Problem-space only** — grill what, who, and why-valuable. Approach, architecture, decomposition, and verification design belong to shape. A narrative that reads like a pseudo-shape is a failure; rewrite before landing.
4. **Seek the first rideable journey** — identify the smallest valuable end-to-end change for a real operator, without designing it. Sequencing names useful outcomes, not layers such as database, API, then UI. Shape later makes the [increment contract](../foundations/references/rideable-increments.md) concrete.
5. **One question at a time, recommendation-first** — using your harness's structured question tool if it has one (otherwise one inline question per message). Never a multi-field form. Order by impact on the narrative. Full mechanics: [references/interview-guide.md](references/interview-guide.md).
6. **Never bound, never ship** — do not add definition-of-done criteria, do not write an out-of-scope statement, do not run `loaf issue check` or `loaf issue promote`, do not push, do not open PRs. Never auto-run shape or bootstrap.
7. **Shape mints on the happy path** — same-session shape-now hands the authored narrative; shape runs `loaf issue new` with that body. Pitch writes an issue body only when parking an unshaped row or when `$ARGUMENTS` already names an issue (`loaf issue edit` replaces the body).
8. **Titles name the concept** — propose a working title, never another work unit's alias. Provenance lives in the issue row, the spark/idea resolution, and frontmatter on `docs/BRIEF.md`.
9. **Log the outcome** — `loaf journal log "decision(pitch): <title, ref, or project> — <shape-now|park-issue|park-idea|handed-to-bootstrap>"`.

---

## Verification

- Issue-scale: a problem narrative exists against the shared skeleton; it was handed to shape, written into an existing issue body, or minted as an unshaped triage row with that body and no criteria
- Project-scale: `docs/BRIEF.md` exists with `source: pitch` and the shared problem-space skeleton
- Cold-read: problem, who, alternative, and value nameable in one pass; zero solution-space content; no out-of-scope statement and no criteria added by this skill
- The first useful operator journey is nameable, and sequencing is expressed as outcomes rather than component layers
- Named sparks were promoted to an idea when pitching them; ideas and sparks were resolved against the issue only after a row exists
- No push; no PR; shape and bootstrap were not auto-run
- Journal shows skill invocation and outcome entries

---

## Quick Reference

| Harness | Invoke skill |
|---------|----------------|
| Claude Code (plugin) | `/loaf:pitch` |
| OpenCode, Cursor, Codex, Amp | `/pitch` |

### Scale detection

| Signal | Scale | Output |
|--------|-------|--------|
| Existing project (git history, source, or Loaf state) + a concept | **Issue** | Problem narrative → shape (`loaf issue new --body`) or an unshaped triage row |
| Empty or minimal directory / greenfield product pitch | **Project** | `docs/BRIEF.md` with `source: pitch` → hand to bootstrap |

Detect and confirm briefly; let the human correct. When both could apply (repo exists but they want a new product pitch), ask once.

### Landing offers

| Offer | When to recommend | What pitch does |
|-------|-------------------|-----------------|
| **Shape now** | Framing is solid; they want to bound next | Hand the narrative; do not mint; do not auto-run shape |
| **Park as issue** | Framed, durable, not bounding yet | `loaf issue new "<title>" --body -` with the narrative only; status stays `triage` |
| **Park as idea** | Too thin to keep as a row, or might discard | `loaf idea capture --title "..."`; journal the gist |
| **Hand to bootstrap** | Project-scale BRIEF authored | Point at bootstrap; do not auto-run it |

Pitch never pushes; never opens PRs. There is nothing to commit at issue scale — the row lives in SQLite. Project-scale may commit `docs/BRIEF.md` if the human wants it durable.

### Spark and idea promotion

| Input | Read | Then |
|-------|------|------|
| Spark | `loaf spark show <ref>` | `loaf idea capture --title "..."` then `loaf spark promote <spark> --to-idea <idea>`; grill from the idea |
| Idea | `loaf idea show <ref>` | Grill; after a row exists, `loaf idea resolve <idea> --by <ref>` |
| Existing issue | `loaf issue show <ref>` | Grill; `loaf issue edit <ref> --body -` writes the narrative (replaces the whole body) |
| Free text | — | Grill; shape-now hands text; park captures an idea or mints an unshaped row |

Do not invent a pitch from the queue without human selection. When they name an intake item, read it (`loaf intake list` / the item's read command).

`loaf idea promote --to-spec` is not this path. Resolve ideas against the minted issue.

### Problem-narrative skeleton

Author against these sections, problem-space sentences only. This text is what shape puts in `--body` (or what a park-as-issue row stores):

```markdown
## Problem Statement
## Who Has It
## Current Alternatives
## Value Proposition
## Constraints
## Sequencing and Relationships
## Sources and Research Links
## Open Questions
```

Do not add an out-of-scope statement. Shape bounds; pitch frames.

### Defined terms

- **Problem narrative** — pitch's issue-scale output. Superseded as the working surface once shape mints and bounds the issue; may accrete parked problem-space sentences until then.
- **BRIEF** — project-scale `docs/BRIEF.md`. A project document, not a work container.
- **Accretion** — adding problem-space concepts to a parked narrative is legal; solution prose is not.
- **Shape now** — hand the narrative to shape, which mints via `loaf issue new` and owns bounding.

---

## Process

### Step 1: Log and parse input

```bash
loaf journal log "skill(pitch): <idea, problem, spark, or intake item>"
```

Parse `$ARGUMENTS`: free text, a spark, an idea, an issue ref, an intake ref the human already chose, or empty (ask what to pitch). Read the named item when provided. Do not invent a pitch from the queue without human selection.

### Step 2: Detect scale

Apply the Quick Reference table. Confirm: "I'll treat this as an **issue-scale** pitch on this repo" or "…as a **project-scale** pitch for a new BRIEF." Adjust if corrected.

### Step 3: Promote sparks; read ideas

When the named input is a spark, promote it to an idea before grilling so the capture trail is one idea, not a dangling spark:

```bash
loaf idea capture --title "<working title>"
loaf spark promote <spark> --to-idea <idea>
```

When the named input is already an idea, `loaf idea show` and grill. Leave resolution until an issue row exists.

### Step 4: Problem-discovery interview

Run the interview per [references/interview-guide.md](references/interview-guide.md):

- Pin a one-or-two-line **destination** before dimension grilling (fixes narrative scope; project scale feeds VISION success criteria; issue scale sharpens what good looks like for the row)
- Dimensions: problem, who has it, current alternatives / competitive landscape, value proposition, constraints (plus sequencing and open questions when needed)
- Depth: scenario stress-testing, challenge stance, glossary-term hygiene; open questions must pass the specifiability test and carry HITL/AFK tags when precise
- Applicability judgment: skip formal competitive analysis and deep personas when the pitch kind does not warrant them (bug fixes, internal chores)
- One question at a time, recommendation-first, ordered by narrative impact
- Stop on exit criteria or when answers stop changing the framing

If the direction is genuinely undecided mid-interview, offer the **explore** technique (agent-side; Explorations and checkpoints) rather than forcing a false narrative. Pitch remains the human front door; explore is not re-opened as a slash alternative by this skill.

### Step 5: Evidence delegation (when warranted)

When competitive or landscape facts would change the narrative and are not already known:

1. Spawn a **researcher** subagent with a bounded question (competitors, substitutes, prior art — not solution design).
2. Land evidence:
   - **Issue scale:** source links in the narrative's Sources and Research Links. If a longer scan lands on disk, name it for the landscape, never for the work unit, and cite it from Sources.
   - **Project scale:** inline source links in `docs/BRIEF.md` Sources and Research Links.
3. Resume the interview or draft with recommendations informed by the scan.

Never fabricate competitive claims. If a scan is skipped, say so in Sources ("no external scan; alternative is internal workaround X").

### Step 6a: Issue-scale ceremony

1. **Propose a working title** — names the concept locally. Confirm with the human. This becomes shape's `loaf issue new` title (or the park-as-issue title).
2. **Author the problem narrative** against the skeleton above. Problem-space sentences only.
3. **Accretion note** — tell the human: parked problem-space concepts may accrete until shaping starts; once the issue is minted, the body is the home.
4. **Cold-read** the narrative (interview guide test); revise with the human until it passes.
5. **Offer landing** (recommendation-first) using the Landing offers table.
6. **Execute the chosen landing:**

   - **Shape now:** hand the full narrative and any spark/idea refs. Shape runs `loaf issue new "<title>" --body -` (or `--body-file`) with that text. Do not mint, do not add criteria, do not open a PR. After shape mints, resolve intake: `loaf idea resolve <idea> --by <ref>` (and `loaf spark resolve <spark> --by <ref>` only if the spark was never promoted).
   - **Park as issue:** mint the unshaped row yourself, then resolve intake against it:

     ```bash
     loaf issue new "<title>" --body -
     loaf idea resolve <idea> --by <ref>
     ```

     Paste the narrative on stdin. Do not add criteria. Do not write out-of-scope. Default status is `triage`. Read back with `loaf issue show <ref>`.
   - **Park as idea:** if no idea exists yet, `loaf idea capture --title "<title>"`. Journal the gist (`loaf journal log "discover(pitch): <one-line problem>"`). Do not mint an issue.
   - **Existing issue:** `loaf issue edit <ref> --body -` with the full narrative. Edit replaces the body; do not strip a row that is already bounded — if criteria already exist, hand the narrative to the human and let shape merge.

7. **Closing ceremony (required — never trail off).** After the landing is executed, announce completion with a full closing block:

    - **Recap the narrative** — section-by-section gist (Problem, Who, Alternatives, Value, Constraints, Sequencing, Open Questions). Name where it lives (handed to shape, unshaped issue `<ref>`, idea `<ref>`, or the conversation plus journal gist).
    - **Restate the landing actually taken** and what it means next:
      - **Shape now** — run shape next to mint the issue from this narrative and bound implementation. No row was minted here.
      - **Park as issue** — `<ref>` holds the problem in its body and is unshaped; run shape later on that ref.
      - **Park as idea** — the idea remains open; re-invoke pitch or shape when ready. Name the idea ref.
    - **Announce completion** in plain language: "Pitch is complete." Do not end on a dangling offer or an unfinished sentence.

### Step 6b: Project-scale ceremony

1. **Author `docs/BRIEF.md`** using bootstrap's brief skeleton with frontmatter:

   ```yaml
   ---
   source: pitch
   created: <ISO-8601>
   archived: true
   ---
   ```

   Same problem-space sections as issue scale, at project altitude (Sequencing describes the initial arc as prose).
2. **Cold-read** and revise with the human.
3. Optionally commit `docs/BRIEF.md` if the human wants it durable before bootstrap; still no push unless they ask outside this skill's duties — pitch itself never pushes.
4. **Closing ceremony (required — never trail off).** Announce completion with a full closing block — do not hand off in a half-sentence:

    - **Recap what was authored** — section-by-section gist of the BRIEF (Problem Statement, Who Has It, Current Alternatives, Value Proposition, Constraints, Sequencing and Relationships, Sources and Research Links, Open Questions). One or two sentences per section is enough; the human should hear what landed without reopening the file.
    - **Artifact path** — name `docs/BRIEF.md` explicitly, including that frontmatter carries `source: pitch`.
    - **Explicit handoff** — state, verbatim in spirit: next, run bootstrap: it reads this BRIEF as discovery-done (`source: pitch`), interviews only on gaps, and populates the operating documents (VISION, STRATEGY, ARCHITECTURE, AGENTS). Do not auto-run bootstrap.
    - **Announce completion** in plain language: "Pitch is complete." The ceremony ends with a period, never a trail-off.

### Step 7: Log the outcome

```bash
loaf journal log "decision(pitch): <title, ref, or project> — <shape-now|park-issue|park-idea|handed-to-bootstrap>"
```

The journal line is mechanical; the human-facing close is the closing ceremony in Step 6a/6b. Never log-and-stop without that recap and next-step restatement.

---

## Related Skills

- **shape** — solution-space bounding; mints the issue from the problem narrative (`loaf issue new`) and owns criteria, out-of-scope, and decomposition
- **bootstrap** — consumes `docs/BRIEF.md` (`source: pitch` → gap interview) and populates operating documents
- **triage** — queue dispositions; may hand a spark or idea to pitch when problem discovery is needed
- **explore** — agent-side technique when pitch finds the direction still undecided
- **idea** — quick capture without ceremony; not a substitute for pitch
- **research** — patterns the researcher subagent follows for landscape scans

## Topics

| Topic | Reference | Use When |
|-------|-----------|----------|
| Interview guide | [references/interview-guide.md](references/interview-guide.md) | Running or adapting the problem-discovery grill |
| Rideable increments | [../foundations/references/rideable-increments.md](../foundations/references/rideable-increments.md) | Framing the first useful journey and outcome-led sequencing |

## Artifact Naming

Name every on-disk artifact for what it is, never for the work unit that produced it. The issue row or `docs/BRIEF.md` already records provenance. Put source in frontmatter, not the filename. See the `foundations` skill; `loaf check --hook artifact-names` enforces it at commit.
