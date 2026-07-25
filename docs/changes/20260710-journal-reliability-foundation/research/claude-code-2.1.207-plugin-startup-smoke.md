# Claude Code 2.1.207 plugin startup smoke

- Date: 2026-07-13
- Surface: Claude Code 2.1.207 on `darwin-arm64`
- Mode: explicit candidate plugin mode using `claude --plugin-dir <candidate-plugin>`, strict empty MCP configuration, disabled tools, `--no-session-persistence`, and stream JSON with hook events included.
- Isolation: disposable Git repository and absolute disposable `LOAF_DB`; no global plugin installation, global Loaf database, or persisted Claude session was used.
- Candidate: freshly built `plugins/loaf` Claude target.
- Result: exit 0 with no stderr. The `SessionStart:startup` hook returned valid Claude-native JSON with `hookSpecificOutput.hookEventName=SessionStart` and an `additionalContext` containing the random marker written only to the isolated journal. The model-visible response returned that marker exactly.
- Reusable command: `node cli/scripts/smoke-claude-code-startup.mjs --client claude --expected-version 2.1.207 --receipt docs/changes/20260710-journal-reliability-foundation/research/claude-code-2.1.207-plugin-startup-smoke.json` (builds the candidate, creates disposable Git and absolute `LOAF_DB` state, runs the exact smoke, validates the native hook response and assistant marker, cleans disposable state, and atomically publishes the receipt only after success).
- Structured receipt: [claude-code-2.1.207-plugin-startup-smoke.json](claude-code-2.1.207-plugin-startup-smoke.json)
- Evidence level: candidate model-visible smoke for Claude Code 2.1.207 in the explicit candidate-plugin mode. The receipt proves startup only; resume, clear, and compact remain candidate modes. This does not prove the globally installed Loaf plugin.
