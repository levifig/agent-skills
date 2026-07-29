---
id: ADR-023
title: "Execution provenance and cohort receipts — git as the witness, verify as the only runner"
status: Accepted
date: 2026-07-28
supersedes: null
superseded_by: ADR-024  # freshness section only; see ADR-024
---

# ADR-023: Execution provenance and cohort receipts

## Context

ADR-022's release cohorts need an evidence model: what proves a change was executed and verified, readable on any machine that can clone the repo. SQLite facts were rejected because the gate must hold on CI and other machines; slug-grep heuristics were rejected as trivially satisfiable. The receipt design superseded itself four times before settling — a `plan.md` binding (expired on approach churn, wrong surface), a criteria-digest-only form (would have survived a revert of the verified work), a legacy-prose digest patch (digesting prose does not make it executable), and shaped auto-re-run-at-preflight semantics, superseded at implementation review round 2 when the shipped block-with-reason behavior proved the better design. Two implementation incidents shaped the final rules: a receipt written by a failing `loaf change verify` satisfied the gate end-to-end (a fixture cut stable 1.0.0 against `exit_code: 1, ok: false`), and the committed binary went three code commits stale while the suite stayed green because tests compile source and the capability receipts pin the committed binary's hash.

## Decision

**Two provenance grades, git-derived.** Path grade (display): a change is executing when a commit in HEAD ancestry modifies its `tasks/` files and at least one path outside `docs/changes/` entirely, so cross-change task edits and folder-local scaffolding never count. Flip grade (gating): the commit's diff must carry a true `- [ ]`→`- [x]` transition — removed-unchecked and added-checked lines in the same hunk with the same normalized label, outside code fences with fence state tracked. Reverse flips, added boxes in either state, whitespace and title edits, and fenced-block flips are non-events, each with a negative fixture. The accepted failure direction is false negatives: work that does not register blocks stable until a one-commit remedy; a false positive would open a release on nothing.

**The receipt is a committed cache of verify.** `loaf change verify` parses executable criteria from `shape.md`'s Verification Contract (V-entries declaring `Command:` and `Expect:`, inline or sub-bullet form), runs exactly those commands from the repository root, and writes `receipts/verify.json`: schema version, change slug, verified commit, criteria digest, target release, cwd, and per-criterion command, exit code, output digest, and ok. H-tier prose criteria are review material, never gate input.

**Success is required, not just execution.** A receipt recording any `ok: false` criterion blocks with `receipt records failing criteria (<IDs>)`. Verify still writes the receipt on failure — the evidence stays on disk — and exits non-zero; the gate is the arbiter.

**Freshness (superseded by ADR-024).** The commit-walk freshness rule below is historical. Receipt validity now binds to a masked root-tree content digest — see ADR-024. Retained for provenance of the original design: any later commit touching a path other than the receipt file itself staled the receipt; the receipt's own commit never did; touch-then-revert still forced re-verification.

**The gate is a pure reader; verify is the only runner.** A stale or failing receipt blocks with a plain reason naming the mechanical remedy (`run: loaf change verify <folder>, then commit the receipt`) rather than re-running criteria at preflight: running declared commands from preflight would make `--dry-run` a lie and would leave its freshest result uncommitted. Evidence is always committed before it is read.

**Scope.** Receipts are new-layout-only and required only for changes declaring a `target_release` — declaring a target is opting into verification. The gate is accident-proofing, not tamper-proofing: a repo owner can edit the gate's source, so hand-authored receipts and trivialized criteria are out of threat model, caught by human review of the cohort member's `shape.md` diff.

## Consequences

Release preflight is deterministic from the repository alone — clone, run, same verdict — and the verification evidence is a reviewable PR artifact rather than a machine-local fact. The B1 incident is under contract (V5 requires failing receipts to block naming the failed IDs) with `RejectsFailingReceipt` and `V5PlanMdEditStalesNotExpires` among the fixtures.

Costs accepted: any post-verify commit imposes a re-verify before stable finalization — `INTENT-20260727-exempt-plan-md-from-receipt-freshness-re-run` stays open as a UX refinement, weighed against every freshness exemption being a future argument. The installed-smoke capability receipts pin the committed binary hash, so every code commit demands a rebuild and re-record; the canary fired four times during this change's implementation, and `INTENT-20260719-decouple-installed-smoke-evidence-from-binary-rebuilds` tracks the standing fix. Delegation discipline matters to the evidence trail: task packets must be committed unchecked before execution, or the delivering diff carries added-checked lines and no transition.

Provenance: `docs/changes/20260726-change-work-model/` shape.md Decisions 2, 13–15 and the Provenance precision contract; review boards rounds 1–2 (B1 reproduction transcript on round 1, block-with-reason adjudication R2-2 on round 2); commits `799019f4`, `6fd0464a`, `9584a307`; journal `finding(release-gate)` and `decision(review)` 2026-07-27→28.
