#!/usr/bin/env bash
# Cursor Cloud Agent start script (LOAF-78).
# Harness install runs in the install/build phase (loaf-cloud-install.sh) so
# hooks/skills exist before the agent works. Re-export LOAF_PROJECT_ENV so
# session loaf upgrade/config/hooks keep the project-local layout — Cursor Cloud
# does not carry install-time shell exports into agent sessions.
# Attach pre-warm (`loaf attach` or equivalent) is deferred until LOAF-67.
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"
export PATH="$ROOT/bin:$PATH"
export LOAF_PROJECT_ENV=1
# no-op until LOAF-67 attach pre-warm lands
exit 0
