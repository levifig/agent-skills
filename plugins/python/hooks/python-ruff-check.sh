#!/bin/bash
# Python plugin hook: Run ruff linter on modified Python files
# Type: PostToolUse (informational, exit 0)
# Trigger: After Edit/Write on *.py files

set -euo pipefail

# Get the file path from environment (PostToolUse hook)
FILE_PATH="${CLAUDE_FILE_PATH:-}"

# Only check Python files
if [[ ! "$FILE_PATH" =~ \.py$ ]]; then
    exit 0
fi

# Check if file exists
if [[ ! -f "$FILE_PATH" ]]; then
    exit 0
fi

# Check if ruff is available
if ! command -v ruff &> /dev/null; then
    # No linter available, skip silently
    exit 0
fi

# Run ruff check on the specific file
OUTPUT=$(ruff check "$FILE_PATH" 2>&1)
EXIT_CODE=$?

if [[ $EXIT_CODE -ne 0 ]]; then
    echo "⚠️  Linting issues in $FILE_PATH:"
    echo "$OUTPUT" | head -20
    echo ""
    echo "Run 'ruff check --fix $FILE_PATH' to auto-fix issues."
fi

# Check formatting
FORMAT_OUTPUT=$(ruff format --check "$FILE_PATH" 2>&1)
FORMAT_EXIT=$?

if [[ $FORMAT_EXIT -ne 0 ]]; then
    echo "⚠️  Formatting issues in $FILE_PATH:"
    echo "Run 'ruff format $FILE_PATH' to fix formatting."
fi

# Always exit 0 (informational only)
exit 0
