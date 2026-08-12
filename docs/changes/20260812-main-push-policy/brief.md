<!-- brief.md is the optional archeological kickstart — the original unshaped ask.
     May accrete parked problem-space concepts while the change is captured; freezes when shape.md exists.
     Superseded by shape.md; never mechanically load-bearing.
     A brief-only folder is legal and non-executable (captured, not shaped). -->

# Main Push Policy — Executable Record-Only Boundary

## Problem Statement

Direct pushes to main bypass the repository's "changes must be made through a pull request" ruleset silently — the operator's account has bypass rights, so nothing distinguishes a sanctioned zero-ceremony capture from a model-changing commit that deserved a review surface. The policy is now clarified (journal decision 2026-08-12: direct-to-main is record-only, allowed solely for capture-only Change folders that gate nothing and change no model or behavior), but it is documentation, and the failure mode it addresses is precisely that the bypass is silent: recent direct-to-main commits included an ADR revision that changed release semantics — exactly the kind of change the clarified policy sends through a PR.

## Who Has It

The operator pushing from any harness or machine, and every agent acting on their behalf — the classification must happen before the push leaves the workstation, uniformly, without depending on which tool constructed the commit.

## Current Alternatives

Discipline plus a bypassable GitHub ruleset. The ruleset cannot express the exception (a capture-only folder diff), so it is either bypassed silently or toggled off entirely — both erase the distinction the policy draws.

## Value Proposition

The exception list becomes executable instead of tribal: a push to main whose range is confined to capture-only Change folders (new folder containing exactly `change.json` + `brief.md`, no `shape.md`) passes; anything else is refused with the PR-flow advisory naming the remedy. Stretching the exception becomes impossible to do unknowingly, and the audit trail shows sanctioned captures as ordinary pushes rather than rule bypasses.

## Constraints

- House pattern: a fail-closed enforcement hook (`loaf check --hook` dispatch) in the mold of the release-flow guardrails born of PR #146 — state-aware, self-carried across harnesses, advisory names the remedy.
- The GitHub ruleset stays as bypassable defense-in-depth behind the Loaf hook, mirroring how `release.yml`'s resolve job mirrors `guardReleaseCeremony` rather than replacing it.
- The git-workflow skill's guidance update (documenting the record-only boundary) rides this Change, not a direct-to-main docs commit — guidance changes are on the PR side of the line this Change enforces.
- Repair ceremonies keep their escape hatch: the ruleset toggle remains the named human path for sanctioned history operations; the hook must not make emergency repair harder.

## Sequencing and Relationships

Standalone; independent of spec-conversion-and-guidance-sweep and linear-native-coordination. Arc-of-one candidate under ADR-026. This folder's own landing commit is the first sanctioned use of the exception it will enforce.

## Sources and Research Links

- Journal decisions 2026-08-11 (one Change per PR, cradle to merge) and 2026-08-12 (record-only boundary test) — the policy this Change makes executable.
- The PR #163 correction (Linear capture cherry-picked to main, sweep branch rebased) — the incident that surfaced the fuzziness.
- `release-flow-guidance` (PR #146) and `self-carried-release-guardrail` — the guardrail pattern being reused.

## Open Questions

- [ ] Where the hook runs: pre-push git hook, `loaf check` pre-push instance, or both — deferrable to shaping
- [ ] Whether amendments to an existing capture-only folder (brief accretion before shaping) also qualify for the exception, or only new-folder creation — blocking for shaping, small
- [ ] Whether the classification should also warn on sanctioned captures (visibility without refusal) — deferrable
