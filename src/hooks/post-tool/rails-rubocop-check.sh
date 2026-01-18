#!/bin/bash
# Rails plugin hook: Run StandardRB/RuboCop on modified Ruby files
# Type: PostToolUse (informational, exit 0)
# Trigger: After Edit/Write on *.rb files

# Get the file path from environment
FILE_PATH="${CLAUDE_FILE_PATH:-}"

# Only check Ruby files
if [[ ! "$FILE_PATH" =~ \.rb$ ]]; then
    exit 0
fi

# Check if file exists
if [[ ! -f "$FILE_PATH" ]]; then
    exit 0
fi

# Check if standardrb or rubocop is available
if command -v standardrb &> /dev/null; then
    LINTER="standardrb"
elif command -v rubocop &> /dev/null; then
    LINTER="rubocop"
else
    # No linter available, skip silently
    exit 0
fi

# Run linter on the specific file
OUTPUT=$($LINTER --format simple "$FILE_PATH" 2>&1)
EXIT_CODE=$?

if [[ $EXIT_CODE -ne 0 ]]; then
    echo "⚠️  Style issues in $FILE_PATH:"
    echo "$OUTPUT" | head -20
    echo ""
    echo "Run '$LINTER --fix $FILE_PATH' to auto-fix."
fi

# Always exit 0 (informational only)
exit 0
