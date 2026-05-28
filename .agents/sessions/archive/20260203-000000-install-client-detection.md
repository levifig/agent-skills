---
session:
  title: "install.sh client detection"
  status: archived
  created: "2026-02-03T00:00:00Z"
  archived_at: "2026-03-27T23:06:02Z"
  archived_by: cleanup
---

# Session: install.sh client detection

**Date:** 2026-02-03
**Owner:** orchestrating PM

## User request
- install.sh only detects Claude Code; other installed clients not detected.

## Notes
- User on macOS. Missing detection for Cursor, VS Code, Zed, and other clients (expected: Codex, Gemini, Cursor).
- Install methods: multiple.
- User evidence:
  - install.sh only detects Claude Code.
  - `command -v` outputs:
    - cursor: /Users/levifig/.local/bin/cursor
    - codex: /opt/homebrew/bin/codex
    - gemini: /Users/levifig/.local/share/mise/installs/node/25...
    - zed: /opt/homebrew/bin/zed
  - Apps installed in /Applications (confirmed).
- Exploration summary:
  - Detection logic in install.sh `detect_tools()`.
  - Cursor detected by `command -v cursor` OR `~/.cursor` dir.
  - Codex detected by `command -v codex` OR `CODEX_HOME` dir (default `~/.codex`).
  - Gemini always added; checks `~/.gemini/skills/python` symlink for Loaf install.
  - VS Code/Zed not in detection list.
- Pending: user install.sh output and exact install locations.

## Plan references
- .agents/plans/20260204-140000-install-detection-macos.md (approved)
