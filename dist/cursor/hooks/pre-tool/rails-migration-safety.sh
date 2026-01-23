#!/bin/bash
# Rails plugin hook: Check migrations for potentially dangerous operations
# Type: PreToolUse (blocking, exit 2 to block)
# Trigger: Before Edit/Write on db/migrate/*.rb files

# Get the file path from environment
FILE_PATH="${CLAUDE_FILE_PATH:-}"

# Only check migration files
if [[ ! "$FILE_PATH" =~ db/migrate/.*\.rb$ ]]; then
    exit 0
fi

# Get the content being written (if available)
CONTENT="${CLAUDE_FILE_CONTENT:-}"

# If no content, try to read the file
if [[ -z "$CONTENT" && -f "$FILE_PATH" ]]; then
    CONTENT=$(cat "$FILE_PATH")
fi

# Check for dangerous operations
WARNINGS=""

# Removing columns without safety
if echo "$CONTENT" | grep -qE "remove_column|remove_columns"; then
    if ! echo "$CONTENT" | grep -q "safety_assured"; then
        WARNINGS="${WARNINGS}⚠️  remove_column detected - ensure ignored in model first\n"
    fi
fi

# Renaming columns (can break running code)
if echo "$CONTENT" | grep -qE "rename_column|rename_table"; then
    WARNINGS="${WARNINGS}⚠️  rename detected - consider add+copy+remove pattern for zero-downtime\n"
fi

# Adding NOT NULL without default
if echo "$CONTENT" | grep -qE "null:\s*false" && ! echo "$CONTENT" | grep -qE "default:"; then
    WARNINGS="${WARNINGS}⚠️  NOT NULL constraint without default - may fail on existing rows\n"
fi

# Adding index without algorithm: :concurrently
if echo "$CONTENT" | grep -qE "add_index" && ! echo "$CONTENT" | grep -q "algorithm: :concurrently"; then
    WARNINGS="${WARNINGS}ℹ️  Consider 'algorithm: :concurrently' for large tables\n"
fi

# Changing column type
if echo "$CONTENT" | grep -qE "change_column(?!_null|_default|_comment)"; then
    WARNINGS="${WARNINGS}⚠️  change_column detected - may lock table and lose data\n"
fi

if [[ -n "$WARNINGS" ]]; then
    echo "🔍 Migration Safety Check: $FILE_PATH"
    echo ""
    echo -e "$WARNINGS"
    echo ""
    echo "See: https://github.com/ankane/strong_migrations"
    # Exit 0 to warn but not block (change to exit 2 to block)
    exit 0
fi

exit 0
