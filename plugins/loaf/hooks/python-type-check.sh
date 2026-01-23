#!/bin/bash
# Python plugin hook: Run mypy type checking on modified Python files
# Type: PreToolUse (informational, exit 0)
# Trigger: Before Edit/Write on *.py files

set -euo pipefail

# Read and parse stdin JSON
INPUT=$(cat)
TOOL_NAME=$(echo "$INPUT" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('tool_name',''))" 2>/dev/null || echo "")
FILE_PATH=$(echo "$INPUT" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('tool_input',{}).get('file_path',''))" 2>/dev/null || echo "")

# Only check Python files
if [[ ! "$FILE_PATH" =~ \.py$ ]]; then
    exit 0
fi

# Skip test files and migrations
if [[ "$FILE_PATH" =~ (test_|_test\.py|/tests/|/migrations/) ]]; then
    exit 0
fi

# Check if file exists
if [[ ! -f "$FILE_PATH" ]]; then
    exit 0
fi

# Check if mypy is available
if ! command -v mypy &> /dev/null; then
    # No type checker available, skip silently
    exit 0
fi

# Run mypy on the specific file
OUTPUT=$(mypy --show-error-codes --no-error-summary "$FILE_PATH" 2>&1)
EXIT_CODE=$?

if [[ $EXIT_CODE -ne 0 ]]; then
    echo "⚠️  Type check issues in $FILE_PATH:"
    echo "$OUTPUT" | head -20
    echo ""
    echo "Run 'mypy $FILE_PATH' to see all issues."
    echo "Add '# type: ignore[error-code]' to suppress specific errors."
fi

# Always exit 0 (informational only)
exit 0
