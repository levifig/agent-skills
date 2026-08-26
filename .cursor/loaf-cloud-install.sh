#!/usr/bin/env bash
# Cursor Cloud Agent install script (LOAF-78).
# Builds the Loaf CLI from this checkout. Project secrets (LOAF-67, when attach
# ships): set LOAF_CLIENT_TOKEN from `loaf auth link --project` in the Cursor
# Cloud project environment; optional LOAF_SYNC_URL for sync endpoint.
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"
if [[ ! -x "$ROOT/bin/loaf" ]]; then
  npm ci
  npm run build:go
fi
