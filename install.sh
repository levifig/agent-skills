#!/usr/bin/env bash
# Loaf installer.
#
#   bash -c "$(curl -fsSL https://raw.githubusercontent.com/levifig/loaf/main/install.sh)"
#
# Downloads the release archive for this platform from GitHub Releases,
# verifies it against the release's checksums.txt, unpacks it under
# $LOAF_HOME/releases/<version>, points $LOAF_HOME/current at it, links
# $LOAF_BIN_DIR/loaf to the binary, and then runs `loaf install`, which
# onboards every harness detected on this machine. Re-run the script to move
# to a newer release; it replaces the current link and runs `loaf upgrade`.
#
# Environment:
#   LOAF_VERSION            release to install (default: latest)
#   LOAF_HOME               install root (default: ${XDG_DATA_HOME:-~/.local/share}/loaf)
#   LOAF_BIN_DIR            where the loaf symlink goes (default: ~/.local/bin)
#   LOAF_RELEASE_BASE_URL   release base (default: https://github.com/levifig/loaf/releases)
#   LOAF_SKIP_HARNESS_INSTALL=1   unpack and link only; do not run loaf install/upgrade
#
# Flags:
#   --version <X.Y.Z>   same as LOAF_VERSION
#   --no-install        same as LOAF_SKIP_HARNESS_INSTALL=1
#   --uninstall         remove the releases, the current link, and the loaf symlink
#   -- <args>           passed through to loaf install (for example: -- --to cursor,codex)
set -euo pipefail

LOAF_HOME="${LOAF_HOME:-${XDG_DATA_HOME:-$HOME/.local/share}/loaf}"
LOAF_BIN_DIR="${LOAF_BIN_DIR:-$HOME/.local/bin}"
LOAF_RELEASE_BASE_URL="${LOAF_RELEASE_BASE_URL:-https://github.com/levifig/loaf/releases}"
LOAF_VERSION="${LOAF_VERSION:-}"
skip_harness_install="${LOAF_SKIP_HARNESS_INSTALL:-0}"
uninstall=0
install_args=()

say() { printf '  \033[32m✓\033[0m %s\n' "$*"; }
note() { printf '  \033[90m○\033[0m %s\n' "$*"; }
warn() { printf '  \033[33m⚠\033[0m %s\n' "$*" >&2; }
fail() { printf '  \033[31m✗\033[0m %s\n' "$*" >&2; exit 1; }

while [ $# -gt 0 ]; do
  case "$1" in
    --version) [ $# -ge 2 ] || fail "--version requires a value"; LOAF_VERSION="$2"; shift 2 ;;
    --version=*) LOAF_VERSION="${1#--version=}"; shift ;;
    --no-install) skip_harness_install=1; shift ;;
    --uninstall) uninstall=1; shift ;;
    --) shift; install_args=("$@"); break ;;
    -h|--help) sed -n '2,25p' "$0" 2>/dev/null || true; exit 0 ;;
    *) fail "unknown option $1" ;;
  esac
done

need() { command -v "$1" >/dev/null 2>&1 || fail "$1 is required but not on PATH"; }

sha256_file() {
  if command -v shasum >/dev/null 2>&1; then shasum -a 256 "$1" | awk '{print $1}'
  elif command -v sha256sum >/dev/null 2>&1; then sha256sum "$1" | awk '{print $1}'
  else fail "neither shasum nor sha256sum is available to verify the download"
  fi
}

link_points_into_loaf_home() {
  # True when $1 is a symlink whose target lives under $LOAF_HOME/releases or is
  # $LOAF_HOME/current: those are ours to replace. Anything else stays.
  [ -L "$1" ] || return 1
  local target
  target="$(readlink "$1")"
  case "$target" in
    "$LOAF_HOME"/releases/*|"$LOAF_HOME"/current|"$LOAF_HOME"/current/*) return 0 ;;
    *) return 1 ;;
  esac
}

printf '\n\033[1mloaf installer\033[0m\n\n'

if [ "$uninstall" = 1 ]; then
  if link_points_into_loaf_home "$LOAF_BIN_DIR/loaf"; then
    rm -f "$LOAF_BIN_DIR/loaf"; say "removed $LOAF_BIN_DIR/loaf"
  elif [ -e "$LOAF_BIN_DIR/loaf" ]; then
    note "left $LOAF_BIN_DIR/loaf alone; it is not a link into $LOAF_HOME"
  fi
  rm -rf "$LOAF_HOME/releases" "$LOAF_HOME/current"
  say "removed $LOAF_HOME/releases and $LOAF_HOME/current"
  note "harness content installed by loaf install is untouched"
  exit 0
fi

need curl
need tar
need mktemp

case "$(uname -s)" in
  Darwin) os=darwin ;;
  Linux) os=linux ;;
  *) fail "unsupported platform $(uname -s); download a release archive from $LOAF_RELEASE_BASE_URL" ;;
esac
case "$(uname -m)" in
  arm64|aarch64) arch=arm64 ;;
  x86_64|amd64) arch=x64 ;;
  *) fail "unsupported architecture $(uname -m)" ;;
esac
target="$os-$arch"

workdir="$(mktemp -d "${TMPDIR:-/tmp}/loaf-install.XXXXXX")"
trap 'rm -rf "$workdir"' EXIT

if [ -z "$LOAF_VERSION" ]; then
  curl -fsSL --retry 3 -o "$workdir/checksums.txt" "$LOAF_RELEASE_BASE_URL/latest/download/checksums.txt" \
    || fail "could not read the latest release at $LOAF_RELEASE_BASE_URL"
  LOAF_VERSION="$(sed -nE "s/^[0-9a-f]{64}  loaf_([^_]+)_${target}\.tar\.gz$/\1/p" "$workdir/checksums.txt" | head -n 1)"
  [ -n "$LOAF_VERSION" ] || fail "the latest release has no archive for $target"
else
  LOAF_VERSION="${LOAF_VERSION#v}"
  curl -fsSL --retry 3 -o "$workdir/checksums.txt" "$LOAF_RELEASE_BASE_URL/download/v$LOAF_VERSION/checksums.txt" \
    || fail "release v$LOAF_VERSION has no checksums.txt at $LOAF_RELEASE_BASE_URL"
fi

archive="loaf_${LOAF_VERSION}_${target}.tar.gz"
expected="$(sed -nE "s/^([0-9a-f]{64})  ${archive}$/\1/p" "$workdir/checksums.txt" | head -n 1)"
[ -n "$expected" ] || fail "checksums.txt for v$LOAF_VERSION lists no $archive"

curl -fsSL --retry 3 -o "$workdir/$archive" "$LOAF_RELEASE_BASE_URL/download/v$LOAF_VERSION/$archive" \
  || fail "could not download $archive"
actual="$(sha256_file "$workdir/$archive")"
[ "$actual" = "$expected" ] || fail "checksum mismatch for $archive (expected $expected, got $actual)"
say "verified $archive"

release_dir="$LOAF_HOME/releases/$LOAF_VERSION"
staging="$workdir/release"
mkdir -p "$staging"
tar -xzf "$workdir/$archive" -C "$staging" --strip-components=1
[ -x "$staging/bin/loaf" ] || fail "$archive does not contain bin/loaf"
mkdir -p "$LOAF_HOME/releases"
rm -rf "$release_dir"
mv "$staging" "$release_dir"
ln -sfn "$release_dir" "$LOAF_HOME/current"
say "installed loaf $LOAF_VERSION to $release_dir"

mkdir -p "$LOAF_BIN_DIR"
if [ -e "$LOAF_BIN_DIR/loaf" ] || [ -L "$LOAF_BIN_DIR/loaf" ]; then
  if link_points_into_loaf_home "$LOAF_BIN_DIR/loaf"; then
    ln -sfn "$LOAF_HOME/current/bin/loaf" "$LOAF_BIN_DIR/loaf"
    say "updated $LOAF_BIN_DIR/loaf -> $LOAF_HOME/current/bin/loaf"
  else
    warn "$LOAF_BIN_DIR/loaf exists and is not managed by this installer; leaving it. Link it yourself: ln -sfn $LOAF_HOME/current/bin/loaf $LOAF_BIN_DIR/loaf"
  fi
else
  ln -s "$LOAF_HOME/current/bin/loaf" "$LOAF_BIN_DIR/loaf"
  say "linked $LOAF_BIN_DIR/loaf -> $LOAF_HOME/current/bin/loaf"
fi

case ":$PATH:" in
  *":$LOAF_BIN_DIR:"*) ;;
  *) warn "$LOAF_BIN_DIR is not on your PATH; add: export PATH=\"$LOAF_BIN_DIR:\$PATH\"" ;;
esac

loaf_bin="$LOAF_HOME/current/bin/loaf"
"$loaf_bin" --version >/dev/null 2>&1 || fail "$loaf_bin did not run; the archive may not match this platform"

if [ "$skip_harness_install" = 1 ]; then
  note "skipped harness install; run: $loaf_bin install"
  exit 0
fi

printf '\n'
if [ -f "$LOAF_HOME/.installed-once" ]; then
  "$loaf_bin" upgrade "${install_args[@]}"
else
  "$loaf_bin" install "${install_args[@]}"
  : > "$LOAF_HOME/.installed-once"
fi
