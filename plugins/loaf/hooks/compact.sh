#!/usr/bin/env bash

set -euo pipefail

PROJECT_DIR="${CLAUDE_PROJECT_DIR:-.}"

if ! command -v loaf >/dev/null 2>&1; then
  exit 0
fi

cd "$PROJECT_DIR"

# Log compaction marker
loaf session log 'compact(session): context compaction triggered' >/dev/null 2>&1 || true

# Warn if ## Current State is still placeholder
branch=$(git branch --show-current 2>/dev/null || echo "unknown")
agents_dir=""
for d in .agents agents; do
  if [ -d "$d/sessions" ]; then
    agents_dir="$d"
    break
  fi
done

if [ -n "$agents_dir" ]; then
  session_file=$(grep -rl "branch: $branch" "$agents_dir/sessions/"*.md 2>/dev/null | head -1)
  if [ -n "$session_file" ]; then
    if grep -q "No state summary yet" "$session_file" 2>/dev/null; then
      echo "WARNING: ## Current State has not been written — still contains placeholder."
      echo "Write a state summary NOW before compaction loses your context."
    elif ! grep -q "## Current State (.*)" "$session_file" 2>/dev/null; then
      echo "WARNING: ## Current State has no timestamp — may be stale from a previous compaction."
      echo "Update it NOW with current state before compaction."
    fi
  fi
fi
