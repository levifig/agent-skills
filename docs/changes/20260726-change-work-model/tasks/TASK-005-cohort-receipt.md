---
change: change-work-model
id: TASK-005
title: Cohort receipt
blocked-by:
  - TASK-003
relates-to:
  - TASK-004
---

# TASK-005 — Cohort receipt

## Objective

Add `loaf change verify`: run declared criteria commands, write a committed receipt (criteria digest, verified commit, per-criterion command/exit/output digest). Gate requires a current receipt for every cohort member; re-run criteria when a later commit touches any path other than the receipt itself. New-layout-only.

## Scope boundaries

**In:** `loaf change verify`; receipt format/serialization (choose against `target_capability_contract.go` precedent); gate receipt requirement + stale re-run; criteria-digest expiry.

**Out:** Skill sweep (`TASK-006`). Activates receipt requirement left dormant by TASK-004.

## Context pointers

- Decisions 13/14; V5; capability receipt precedent in `internal/cli/target_capability_contract.go`.

## Acquisition

```bash
loaf journal log "skill(implement): TASK-005 — cohort receipt"
```

## Steps

- [ ] Parse executable criteria command form from `shape.md`; run exactly those commands.
- [ ] Write receipt with criteria digest, verified commit, per-criterion evidence.
- [ ] Gate requires current receipt for cohort members; receipt's own commit never stales; other later paths force re-run.
- [ ] Criteria edit expires; `plan.md` edit does not; retarget after verify triggers re-run path.
- [ ] Legacy members directed to convert first.

## Verification

- V5 fixtures green; receipt enforcement active on finalization paths.
