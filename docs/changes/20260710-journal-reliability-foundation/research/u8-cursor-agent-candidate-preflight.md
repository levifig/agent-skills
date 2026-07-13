# Cursor Agent U8 candidate preflight

This receipt records an unavailable preflight on `darwin-arm64`, not installed model-visible support. The installed Cursor Agent identity is `2026.05.09-0afadcc`; `cursor-agent --help` does not expose `--no-session-persistence`, so no model-visible prompt is invoked that could persist a session outside the disposable project. The separate `cursor-agent status --format json` check was not retained because it includes account-state output; no account data is part of the evidence.

Rerun from the repository root:

```bash
node cli/scripts/u8-cursor-smoke.mjs
```

The script records the exact candidate `dist/cursor/hooks.json` and native binary SHA-256 values, the sanitized preflight facts, and the explicit blocker in [u8-cursor-agent-candidate-preflight.json](u8-cursor-agent-candidate-preflight.json). No global Cursor configuration, install, session store, or Loaf database is modified. The Cursor Agent `new-composer` capability therefore remains `candidate`; IDE `3.11.19` remains a separate `candidate` identity.
