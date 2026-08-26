#!/usr/bin/env bash
# Cursor Cloud Agent start script (LOAF-78).
# Harness install runs in the install/build phase (loaf-cloud-install.sh) so
# hooks/skills exist before the agent works. Re-export LOAF_PROJECT_ENV so
# session loaf upgrade/config/hooks keep the project-local layout — Cursor Cloud
# does not carry install-time shell exports into agent sessions.
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"
export PATH="$ROOT/bin:$PATH"
export LOAF_PROJECT_ENV=1
if [[ -n "${LOAF_CLIENT_TOKEN:-}" ]]; then
  loaf attach
fi
