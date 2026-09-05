#!/bin/sh
# Loaf Claude Code plugin shim.
#
# The marketplace plugin ships content and hooks only; the loaf CLI is
# installed separately (Homebrew, a release archive, or a checkout build).
# Hooks invoke this file as "${CLAUDE_PLUGIN_ROOT}/bin/loaf", and it execs the
# installed CLI: $LOAF_BIN when set, then loaf on PATH, then the usual install
# locations. When nothing resolves it exits 1, which Claude Code reports as a
# non-blocking hook error, so a missing CLI is visible without locking tools.
resolve_loaf() {
  if [ -n "${LOAF_BIN:-}" ] && [ -x "${LOAF_BIN}" ]; then
    printf '%s\n' "${LOAF_BIN}"
    return 0
  fi
  if found=$(command -v loaf 2>/dev/null) && [ -n "${found}" ]; then
    printf '%s\n' "${found}"
    return 0
  fi
  for candidate in "${HOME:-}/.local/bin/loaf" /opt/homebrew/bin/loaf /usr/local/bin/loaf "${HOME:-}/.local/bin/loaf.exe"; do
    if [ -x "${candidate}" ]; then
      printf '%s\n' "${candidate}"
      return 0
    fi
  done
  return 1
}

if loaf_bin=$(resolve_loaf); then
  exec "${loaf_bin}" "$@"
fi
printf '%s\n' "loaf: the Loaf plugin needs the loaf CLI. Install it (brew install levifig/tap/loaf) or set LOAF_BIN to a loaf binary." >&2
exit 1
