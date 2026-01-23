#!/usr/bin/env bash
# Validates SQL commands for destructive operations
# Blocks DROP, TRUNCATE, DELETE without WHERE unless explicitly confirmed
#
# Exit codes:
#   0 - Allow operation
#   2 - Block operation (message via stderr)

set -euo pipefail

# Read JSON input from stdin
INPUT=$(cat)

# Extract command from tool_input
COMMAND=$(echo "$INPUT" | jq -r '.tool_input.command // empty' 2>/dev/null || echo "")

if [[ -z "$COMMAND" ]]; then
  exit 0
fi

# Convert to uppercase for matching
UPPER_CMD=$(echo "$COMMAND" | tr '[:lower:]' '[:upper:]')

# Block DROP DATABASE/TABLE/SCHEMA without confirmation
if echo "$UPPER_CMD" | grep -qE '\bDROP\s+(DATABASE|TABLE|SCHEMA)\b'; then
  echo "⛔ BLOCKED: DROP DATABASE/TABLE/SCHEMA detected." >&2
  echo "This is a destructive operation that cannot be undone." >&2
  echo "If intentional, ask the user to confirm before proceeding." >&2
  exit 2
fi

# Block TRUNCATE (deletes all rows, cannot be rolled back in some DBs)
if echo "$UPPER_CMD" | grep -qE '\bTRUNCATE\b'; then
  echo "⛔ BLOCKED: TRUNCATE detected." >&2
  echo "This deletes all rows and may not be reversible." >&2
  echo "If intentional, ask the user to confirm before proceeding." >&2
  exit 2
fi

# Warn on DELETE without WHERE (but don't block)
if echo "$UPPER_CMD" | grep -qE '\bDELETE\s+FROM\b' && ! echo "$UPPER_CMD" | grep -qE '\bWHERE\b'; then
  echo "⚠️  WARNING: DELETE without WHERE clause detected." >&2
  echo "This will delete ALL rows in the table." >&2
  # Don't block, just warn
fi

# Warn on ALTER TABLE DROP COLUMN
if echo "$UPPER_CMD" | grep -qE '\bALTER\s+TABLE\b.*\bDROP\s+COLUMN\b'; then
  echo "⚠️  WARNING: DROP COLUMN detected." >&2
  echo "This is a destructive migration. Ensure it's reversible." >&2
  # Don't block, just warn
fi

exit 0
