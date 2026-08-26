#!/usr/bin/env bash
# Cursor Cloud Agent start script (LOAF-78).
# Deploys harness surfaces via project-environment install. Attach pre-warm
# (`loaf attach` or equivalent) is deferred until LOAF-67 attach ceremony ships.
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"
export PATH="$ROOT/bin:$PATH"
export LOAF_PROJECT_ENV=1
exec loaf install --to cursor --yes
