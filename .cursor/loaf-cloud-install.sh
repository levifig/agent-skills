#!/usr/bin/env bash
# Cursor Cloud Agent install script (LOAF-78).
# Builds the Loaf CLI for this host, then installs harness surfaces into the
# project environment so hooks/skills exist before the agent starts.
# Project secrets (LOAF-67, when attach ships): set LOAF_CLIENT_TOKEN from
# `loaf auth link --project` in the Cursor Cloud project environment; optional
# LOAF_SYNC_URL for sync endpoint.
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

os="$(uname -s | tr '[:upper:]' '[:lower:]')"
arch="$(uname -m)"
case "$arch" in
  x86_64) arch=x64 ;;
  aarch64|arm64) arch=arm64 ;;
esac
NATIVE="$ROOT/bin/native/${os}-${arch}/loaf"

# bin/loaf is only the Node launcher; require the native binary for this host
# before skipping the build (cloud agents are typically linux-*, not darwin).
if [[ ! -x "$ROOT/bin/loaf" || ! -x "$NATIVE" ]]; then
  npm ci
  npm run build:go
fi

export PATH="$ROOT/bin:$PATH"
export LOAF_PROJECT_ENV=1
loaf install --to cursor --yes
