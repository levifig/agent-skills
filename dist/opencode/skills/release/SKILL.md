---
name: release
description: >-
  Releases a coherent set of already-landed work through repository-native
  publishing surfaces. Use when shipped tracker records and Git history are
  ready for a versioned publication. Produces a verified release outcome without
  a local release ledger.
version: 0.5.0
---

# Release

Release is retroactive: select already-landed work from Git and live native tracker records, publish through the repository's configured surfaces, then record observed evidence on the same native work.

## Contents

- Critical Rules
- Verification
- Quick Reference
- Topics

## Critical Rules

- As the first action, run `loaf journal log "skill(release): <concise intent>"` against the current private local journal. If the write fails, report the failure and continue only when the work can safely proceed; never put invocation bookkeeping in the tracker.
- Read current tags, versions, published releases, repository rules, and candidate native records before proposing a cohort.
- Include only work whose repository landing and ship verdict are observable. Tracker status alone does not prove landing.
- Derive human release notes from the actual landed change and live work definition; do not copy internal execution logs.
- Describe progress by the larger rideable journey now possible, not by horizontal layers completed. If a release contains foundation work only, label it honestly and name the immediate journey intended to consume it; this is release-note truth, not a new refusal gate.
- Ask for or verify explicit authorization immediately before version changes, tags, pushes, or release publication when governing instructions require it.
- Use repository-native Git and hosting capabilities. Do not create a Loaf-owned release record or provider bridge.
- After publication, read the authoritative tag and release state before claiming success.
- Add a [tracker update](templates/tracker-update.md) or supported native release metadata only after publication is confirmed; comments remain evidence, not workflow substitutes.
- Re-read every changed native record and report partial or indeterminate provider outcomes independently from publication success.

## Verification

- The release cohort resolves to exact landed commits and native work references.
- Version, tag, artifact, and release-note state are mutually consistent.
- Required build, package, signing, or publication checks were actually run and read.
- The authoritative hosting surface confirms the published release and artifacts.
- Tracker updates name the observed release identity and were confirmed by readback.
- Release notes state what a real rider can now do, what dogfood proved, and which breadth remains explicitly deferred.

## Quick Reference

| Phase | Evidence |
|-------|----------|
| Suggest | Landed commit range plus live native work |
| Prepare | Version and release notes consistent with repository policy |
| Publish | Explicitly authorized native Git/hosting mutation |
| Verify | Authoritative tag, release, and artifact reads |
| Record | Confirmed native tracker update with release identity |

## Topics

No supporting references. Load repository-specific packaging, signing, and release guidance before publication.
