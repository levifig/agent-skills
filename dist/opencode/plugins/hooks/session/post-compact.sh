#!/usr/bin/env bash

set -euo pipefail

PROJECT_DIR="${CLAUDE_PROJECT_DIR:-.}"

if ! command -v loaf >/dev/null 2>&1; then
  exit 0
fi

cd "$PROJECT_DIR"

# Find active session file for current branch
branch=$(git branch --show-current 2>/dev/null || echo "unknown")
agents_dir=""
for d in .agents agents; do
  if [ -d "$d/sessions" ]; then
    agents_dir="$d"
    break
  fi
done

if [ -z "$agents_dir" ]; then
  exit 0
fi

session_file=$(grep -rl "branch: $branch" "$agents_dir/sessions/"*.md 2>/dev/null | head -1)
if [ -z "$session_file" ]; then
  exit 0
fi

# Extract ## Current State section (everything between ## Current State and the next ##)
current_state=$(sed -n '/^## Current State/,/^## /{/^## Current State/d;/^## /d;p;}' "$session_file" | sed '/^$/N;/^\n$/d')

echo "=== POST-COMPACTION RESUMPTION ==="
echo ""
echo "Session file: $session_file"
echo ""

if [ -n "$current_state" ] && ! echo "$current_state" | grep -q "No state summary yet"; then
  echo "## Current State (from session file)"
  echo ""
  echo "$current_state"
else
  echo "WARNING: No state summary was written before compaction."
  echo "Read the session file manually for context."
fi

echo ""
echo "Read the session file's ## Journal section for the full decision/discovery trail."
echo "Resume work from where the summary left off — do not ask 'where were we?'"
