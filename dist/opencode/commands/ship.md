---
description: >-
  Serves as the sole quality gate for implemented native tracker work. Use when
  a candidate diff needs independent review, criterion verification, and a
  landing decision. Produces an evidence-backed quality verdict and verified
  native transition when authorized.
version: 0.5.0
---

# Ship

Evaluate the candidate against the live [work contract](templates/work-contract.md), repository state, and observed verification evidence. Passing tests are necessary but do not replace criterion-by-criterion review.

## Contents

- Critical Rules
- Verification
- Quick Reference
- Topics

## Critical Rules

- As the first action, run `loaf journal log "skill(ship): <concise intent>"` against the current private local journal. If the write fails, report the failure and continue only when the work can safely proceed; never put invocation bookkeeping in the tracker.
- Re-read the canonical record, relationships, workflow state, and relevant collaboration through [`project-management/v1`](../project-management/SKILL.md).
- Inspect the actual candidate diff and repository instructions. Do not accept an implementer's summary as evidence.
- Review correctness, maintainability, security boundaries, scope, and every completion criterion with fresh eyes; use independent read-only reviewers when available and proportionate.
- Review the rideable increment inside this same quality gate: can the named rider use the result today, does the journey exercise the machinery added, what real dogfood and integrity evidence support the claims, and could less preserve the same safe outcome?
- Return validated findings to implement and rerun affected gates after fixes. Do not waive an unproven criterion as a documentation detail.
- Build review or change-request text from the live native contract and observed diff, not from a synchronized local projection.
- Preserve external-action authority. A quality verdict does not itself authorize commit, push, merge, publication, or destructive action.
- Transition native workflow state only after the corresponding repository event is proven and authorized.
- Re-read the changed native record and any evidence comment; never mark work complete from a mutation request alone.
- Release remains a separate retroactive ceremony over already-landed work.

## Verification

- The reviewed commit or diff identity is exact and the working tree contains no unexplained changes.
- Every completion criterion has direct evidence from code, tests, native state, or an explicit human verdict.
- The complete journey works from its named entry point to observable outcome; no future-only machinery is presented as delivered value.
- Dogfood and safety/integrity evidence match the contract, and learning plus remaining deferrals are reported honestly.
- Required focused, affected, full-suite, format, lint, static-analysis, and build gates were run in proportion to risk.
- All findings have an evidence-backed disposition and fixes were re-reviewed.
- Any native transition matches the observed landed state and was confirmed by readback.

## Quick Reference

| Verdict | Meaning |
|---------|---------|
| Approve | Criteria and quality gates are proven; authorized landing may proceed |
| Request changes | Validated defects or unproven criteria remain |
| Blocked | Required evidence, authority, connection, or repository event is unavailable |
| Landed | Repository event and final native state were both observed |

## Topics

No supporting references. Use the [tracker update](templates/tracker-update.md) for a concise evidence-bearing verdict when collaborators need it.
