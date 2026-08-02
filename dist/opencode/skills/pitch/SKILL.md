---
name: pitch
description: >-
  Runs the human problem-discovery ceremony at change or project scale: grills
  problem, who has it, current alternatives, value proposition, and constraints,
  then authors a brief (change brief.md via loaf change init --brief, or project
  docs/BRIEF.md with source: pitch). Use when the user invokes /pitch, starts
  work on a raw concept, or triage dispositions an item as pitch. Produces an
  authored problem-space brief and a shape-now or park offer — never shape.md,
  tasks, or PRs. Not for solution shaping (use shape), queue processing (use
  triage), quick capture (use idea), or open-ended divergent inquiry (use
  explore as an agent technique when pitch reveals the direction is undecided).
version: 2.0.0-alpha.18
---

# Pitch

Human problem-discovery ceremony. Authors a brief at the matching scale so `/shape` starts from a framed problem and bootstrap can consume a pitched project BRIEF.

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

1. **Agents never initiate a pitch.** This ceremony is human-invoked only. On OpenCode the sidecar sets `disable-model-invocation: true`; on every target this rule binds behaviorally. Agent legwork *inside* a human-opened pitch (competitive scans, file writes the skill directs) is fine — opening one is not.
2. **Log invocation first** — `loaf journal log "skill(pitch): <idea, problem, or intake item>"` before interviewing.
3. **Problem-space only** — grill what, who, and why-valuable. Approach, architecture, decomposition, and verification design belong to `/shape`. A brief that reads like a pseudo-shape is a failure; rewrite before landing.
4. **One question at a time, recommendation-first** — via `prompt the user in chat` (or one inline question per message). Never a multi-field form. Order by impact on the brief. Full mechanics: [references/interview-guide.md](references/interview-guide.md).
5. **Never write `shape.md`, seed `tasks/`, push, or open PRs** — pitch prepares commits and hands off; push and PR stay human. Never auto-run `/shape` or `/bootstrap`.
6. **Landing is validated, then committed once** — every capture landing runs explicit-path `loaf change check <folder> --json` (zero violations, expected captured state) and a direct read-back of that folder's `change.json` confirming intended `target_release` presence or absence, then one docs-only commit per capture. Never batch captures into one commit.
7. **Slug identity is local** — propose a slug that names the concept, never another work unit (no `spec-042`, no task ids). Provenance lives in frontmatter and the change folder.
8. **Log the outcome** — `loaf journal log "decision(pitch): <slug or project> — <shape-now|park-targeted|park-untargeted|handed-to-bootstrap>"`.

---

## Verification

- Change scale: `docs/changes/YYYYMMDD-slug/` holds `change.json` + authored `brief.md`; `loaf change check <folder> --json` reports zero violations and captured state; `change.json` read-back matches the intended target binding
- Project scale: `docs/BRIEF.md` exists with `source: pitch` and the shared problem-space skeleton
- Cold-read: problem, who, alternative, and value nameable in one pass; zero solution-space content
- No `shape.md` or `tasks/` written by this skill; no push; no PR
- Journal shows skill invocation and outcome entries

---

## Quick Reference

### Scale detection

| Signal | Scale | Output |
|--------|-------|--------|
| Existing project (git history, source, or Loaf state) + a concept | **Change** | `loaf change init <slug> --brief` → authored `brief.md` |
| Empty or minimal directory / greenfield intent | **Project** | `docs/BRIEF.md` with `source: pitch` → hand to `/bootstrap` |

Detect and confirm briefly; let the human correct. When both could apply (repo exists but they want a new product pitch), ask once.

### Landing matrix (Decision 11)

| Intent | Branch | Commit | Target |
|--------|--------|--------|--------|
| **Shape now** | Create the slug branch (`git switch -c <slug>`), stay there | Hand to `/shape` for in-place promotion — do not park-commit first | Stamp `target_release` when known |
| **Park targeted** | Default branch | One docs-only commit on default (promise-carrier exception) | `target_release` present and confirmed by read-back |
| **Park untargeted** | Slug branch **or** remain intake (Intent/spark) | Docs-only commit on the slug branch if becoming a Change; else no Change folder | No `target_release`; untargeted captures never land on main |

Pitch prepares the commit; never pushes; never opens PRs.

### Pre-landing guard (every capture)

```bash
loaf change check <folder> --json   # zero violations; state is captured
# then read <folder>/change.json and confirm target_release presence/absence matches intent
```

Bare `loaf change check` resolves by branch and can miss a capture landing elsewhere — always pass the explicit folder path.

### Defined terms

- **Brief** — the pitch output (problem-space). Superseded by `shape.md` when shaping starts; may accrete parked problem-space sentences until then; freezes when `shape.md` exists.
- **Accretion** — adding problem-space concepts to a parked brief is legal; solution prose is not.
- **Shape now** — slug branch + hand to `/shape`, which promotes the capture in place via ordinary `loaf change init <slug>` (no `--brief`).

---

## Process

### Step 1: Log and parse input

```bash
loaf journal log "skill(pitch): <idea, problem, or intake item>"
```

Parse `$ARGUMENTS`: free text, an intake ref the human already chose, or empty (ask what to pitch). Read the named intake item when provided (`loaf intake list` / the item's read command). Do not invent a pitch from the queue without human selection.

### Step 2: Detect scale

Apply the Quick Reference table. Confirm: "I'll treat this as a **change-scale** pitch on this repo" or "…as a **project-scale** pitch for a new BRIEF." Adjust if corrected.

### Step 3: Problem-discovery interview

Run the interview per [references/interview-guide.md](references/interview-guide.md):

- Pin a one-or-two-line **destination** before dimension grilling (fixes brief scope; project scale feeds VISION success criteria, change scale sharpens the eventual Hypothesis)
- Dimensions: problem, who has it, current alternatives / competitive landscape, value proposition, constraints (plus sequencing and open questions when needed)
- Depth: scenario stress-testing, challenge stance, glossary-term hygiene; open questions must pass the specifiability test and carry HITL/AFK tags when precise
- Applicability judgment: skip formal competitive analysis and deep personas when the pitch kind does not warrant them (bug fixes, internal chores)
- One question at a time, recommendation-first, ordered by brief impact
- Stop on exit criteria or when answers stop changing the framing

If the direction is genuinely undecided mid-interview, offer the **explore** technique (agent-side; Explorations and checkpoints) rather than forcing a false brief. Pitch remains the human front door; explore is not re-opened as a slash alternative by this skill.

### Step 4: Evidence delegation (when warranted)

When competitive or landscape facts would change the brief and are not already known:

1. Spawn a **researcher** subtask agent with a bounded question (competitors, substitutes, prior art — not solution design).
2. Land evidence:
   - **Change scale:** files under the change folder's `research/` (create the folder with the change); link from Sources and Research Links
   - **Project scale:** inline source links in `docs/BRIEF.md` Sources and Research Links (no change `research/` yet)
3. Resume the interview or brief draft with recommendations informed by the scan.

Never fabricate competitive claims. If a scan is skipped, say so in Sources ("no external scan; alternative is internal workaround X").

### Step 5a: Change-scale ceremony

1. **Propose a slug** — lowercase, digits, single hyphens; names the concept locally. Confirm with the human.
2. **Initialize capture:**

   ```bash
   loaf change init <slug> --brief
   ```

   Creates `docs/changes/YYYYMMDD-<slug>/` with `change.json` + `brief.md` scaffold only.
3. **Author `brief.md`** against the shared problem-space skeleton (shape's brief template / the scaffold just written): Problem Statement, Who Has It, Current Alternatives, Value Proposition, Constraints, Sequencing and Relationships, Sources and Research Links, Open Questions. Problem-space sentences only.
4. **Stamp `target_release` when known** — edit `change.json` with canonical `MAJOR.MINOR.PATCH` (no `v`, no prerelease). Omit the field when untargeted. Confirm with the human before stamping.
5. **Accretion note** — tell the human: parked problem-space concepts may accrete into this brief until shaping starts; once `shape.md` exists the brief freezes.
6. **Cold-read** the brief (interview guide test); revise with the human until it passes.
7. **Offer landing** (recommendation-first):

   | Offer | When to recommend |
   |-------|-------------------|
   | **Shape now** | Framing is solid and they want to bound implementation next |
   | **Park targeted** | Bound to a release cohort but not shaping yet — docs-only on default branch |
   | **Park untargeted** | Worth capturing off-main, or not ready as a Change (stay intake) |

8. **Execute the chosen landing:**

   - **Shape now:** `git switch -c <slug>` (from default unless already on a working branch the human prefers), ensure pre-landing guard would pass if they later park, hand to `/shape` with the folder path — shape promotes in place. Do not open a PR.
   - **Park targeted:** on the **default branch**, run pre-landing guard on the explicit folder, confirm `target_release` present in `change.json`, then one docs-only commit of the change folder (and any `research/` under it).
   - **Park untargeted as Change:** `git switch -c <slug>`, pre-landing guard, confirm `target_release` **absent**, one docs-only commit on the slug branch.
   - **Park as intake:** do not leave a half-written change folder; prefer Intent/spark retention and delete or never create the capture if the human backs out.

9. **Commit message** (when parking): conventional, e.g. `docs(change): capture <slug> brief` — one commit per capture.

10. **Closing ceremony (required — never trail off).** After the landing is executed (or intake retained), announce completion with a full closing block:

    - **Recap the brief** — section-by-section gist (Problem, Who, Alternatives, Value, Constraints, Sequencing, Open Questions). Name the change folder path (`docs/changes/YYYYMMDD-<slug>/`) and what it holds (`change.json` + `brief.md`, plus any `research/`).
    - **Restate the landing actually taken** and what it means next:
      - **Shape now** — you are on the slug branch; run `/shape` next to promote the capture in place and bound implementation. No park-commit was made.
      - **Park targeted** — the capture is a docs-only commit on the default branch with `target_release` stamped; it sits as a promise carrier for that cohort until `/shape` is invoked later.
      - **Park untargeted** — the capture lives on the slug branch (or remains intake) without `target_release`; it is off-main until retargeted or shaped. If intake-only, name the Intent/spark and that no change folder was left half-written.
    - **Announce completion** in plain language: "Pitch is complete." Do not end on a dangling offer or an unfinished sentence.

### Step 5b: Project-scale ceremony

1. **Author `docs/BRIEF.md`** using bootstrap's brief skeleton with frontmatter:

   ```yaml
   ---
   source: pitch
   created: <ISO-8601>
   archived: true
   ---
   ```

   Same problem-space sections as change scale, at project altitude (Sequencing describes the initial arc as prose).
2. **Cold-read** and revise with the human.
3. Optionally commit `docs/BRIEF.md` if the human wants it durable before bootstrap; still no push unless they ask outside this skill's duties — pitch itself never pushes.
4. **Closing ceremony (required — never trail off).** Announce completion with a full closing block — do not hand off in a half-sentence:

    - **Recap what was authored** — section-by-section gist of the BRIEF (Problem Statement, Who Has It, Current Alternatives, Value Proposition, Constraints, Sequencing and Relationships, Sources and Research Links, Open Questions). One or two sentences per section is enough; the human should hear what landed without reopening the file.
    - **Artifact path** — name `docs/BRIEF.md` explicitly, including that frontmatter carries `source: pitch`.
    - **Explicit handoff** — state, verbatim in spirit: next, run `/bootstrap`: it reads this BRIEF as discovery-done (`source: pitch`), interviews only on gaps, populates the operating documents (VISION, STRATEGY, ARCHITECTURE, AGENTS), and closes by proposing your initial arc of captured changes. Do not auto-run `/bootstrap`.
    - **Announce completion** in plain language: "Pitch is complete." The ceremony ends with a period, never a trail-off.

### Step 6: Log the outcome

```bash
loaf journal log "decision(pitch): <slug or project> — <shape-now|park-targeted|park-untargeted|handed-to-bootstrap>"
```

The journal line is mechanical; the human-facing close is the closing ceremony in Step 5a/5b. Never log-and-stop without that recap and next-step restatement.

---

## Related Skills

- **shape** — solution-space narrowing from an existing brief (or full narrowing when no brief); promotes capture folders in place
- **bootstrap** — consumes `docs/BRIEF.md` (`source: pitch` → gap interview) and series-preps captured changes
- **triage** — queue dispositions; may hand an item to pitch when problem discovery is needed
- **explore** — agent-side technique when pitch finds the direction still undecided
- **idea** — quick capture without ceremony; not a substitute for pitch
- **research** — patterns the researcher subtask agent follows for landscape scans

## Topics

| Topic | Reference | Use When |
|-------|-----------|----------|
| Interview guide | [references/interview-guide.md](references/interview-guide.md) | Running or adapting the problem-discovery grill |

## Artifact Naming

Name every artifact for what it is, never for the work unit that produced it. The change folder already records provenance. Put source in frontmatter, not the filename. See the `foundations` skill; `loaf check --hook artifact-names` enforces it at commit.
