---
session:
  title: "install.sh upgrade flag"
  status: archived
  created: "2026-02-04T00:00:00Z"
  archived_at: "2026-03-27T23:06:02Z"
  archived_by: cleanup
---

# Session: install.sh upgrade flag

**Date:** 2026-02-04
**Owner:** orchestrating PM

## User request
- `./install.sh --upgrade` does not work; upgrade path needs investigation.

## Notes
- User ran `LOAF_DEBUG=1 ./install.sh --upgrade` and got:
  - Debug shows Cursor/Codex/Gemini detected via cli.
  - Output: "No installed targets to upgrade".
  - Script still builds and syncs dist to cache.
- User reports same behavior without `--upgrade` (no targets shown/marked for upgrade).
- Likely issue: upgrade mode selects only targets marked installed; skill-based marker checks are brittle.
- Decision: use a stable marker file in each target config root for install detection.
- New issue: Codex install message shows double slash when CODEX_HOME ends with trailing slash.

## Plan references
- .agents/plans/20260204-143000-install-upgrade-fix.md (approved)
