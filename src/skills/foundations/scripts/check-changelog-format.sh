#!/usr/bin/env bash
# Check micro-changelog format in markdown files
# Usage: check-changelog-format.sh <file.md>

set -euo pipefail

FILE="${1:-}"

if [[ -z "$FILE" ]]; then
    echo "Usage: check-changelog-format.sh <file.md>"
    exit 1
fi

if [[ ! -f "$FILE" ]]; then
    echo "Error: File not found: $FILE"
    exit 1
fi

ERRORS=0

echo "Checking: $FILE"
echo "================================"

# Check for Changelog section
if ! grep -q "^## Changelog" "$FILE"; then
    echo "⚠️  No ## Changelog section found"
    ((ERRORS++)) || true
else
    echo "✓ Changelog section found"

    # Extract changelog section
    CHANGELOG=$(sed -n '/^## Changelog/,/^## /p' "$FILE" | head -n -1)

    # Check date format (YYYY-MM-DD)
    BAD_DATES=$(echo "$CHANGELOG" | grep -E "^- " | grep -vE "^- [0-9]{4}-[0-9]{2}-[0-9]{2} -" || true)
    if [[ -n "$BAD_DATES" ]]; then
        echo "⚠️  Entries with incorrect date format:"
        echo "$BAD_DATES"
        ((ERRORS++)) || true
    else
        echo "✓ All dates in YYYY-MM-DD format"
    fi

    # Check for reverse chronological order
    DATES=$(echo "$CHANGELOG" | grep -oE "[0-9]{4}-[0-9]{2}-[0-9]{2}" || true)
    if [[ -n "$DATES" ]]; then
        SORTED=$(echo "$DATES" | sort -r)
        if [[ "$DATES" != "$SORTED" ]]; then
            echo "⚠️  Entries not in reverse chronological order"
            ((ERRORS++)) || true
        else
            echo "✓ Entries in reverse chronological order"
        fi
    fi
fi

# Check for Last Updated header
if grep -qE "^\*\*Last Updated\*\*:" "$FILE"; then
    echo "✓ Last Updated header found"

    # Check if Last Updated matches newest changelog entry
    LAST_UPDATED=$(grep -oE "\*\*Last Updated\*\*:\s*[0-9]{4}-[0-9]{2}-[0-9]{2}" "$FILE" | grep -oE "[0-9]{4}-[0-9]{2}-[0-9]{2}" || true)
    NEWEST_ENTRY=$(grep -E "^- [0-9]{4}-[0-9]{2}-[0-9]{2}" "$FILE" | head -1 | grep -oE "[0-9]{4}-[0-9]{2}-[0-9]{2}" || true)

    if [[ -n "$LAST_UPDATED" && -n "$NEWEST_ENTRY" && "$LAST_UPDATED" != "$NEWEST_ENTRY" ]]; then
        echo "⚠️  Last Updated ($LAST_UPDATED) doesn't match newest changelog entry ($NEWEST_ENTRY)"
        ((ERRORS++)) || true
    fi
else
    echo "ℹ️  No Last Updated header (optional)"
fi

echo "================================"
if [[ $ERRORS -gt 0 ]]; then
    echo "Found $ERRORS issue(s)"
    exit 1
else
    echo "All checks passed!"
    exit 0
fi
