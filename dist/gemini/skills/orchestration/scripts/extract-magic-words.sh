#!/bin/bash
# Extract Linear magic words from commit messages
# Usage: extract-magic-words.sh [git-range]
#        extract-magic-words.sh HEAD~5..HEAD
#        extract-magic-words.sh  # defaults to last 10 commits

set -e

RANGE="${1:-HEAD~10..HEAD}"

echo "Linear Issue References in Commits"
echo "==================================="
echo "Range: $RANGE"
echo ""

# Magic word patterns
CLOSING_PATTERN="(Closes|Fixes|Resolves)\s+[A-Z]+-[0-9]+"
NON_CLOSING_PATTERN="(Refs|Part of)\s+[A-Z]+-[0-9]+"

# Get commit messages
commits=$(git log --pretty=format:"%h %s%n%b---" "$RANGE" 2>/dev/null || echo "")

if [[ -z "$commits" ]]; then
    echo "No commits found in range: $RANGE"
    exit 0
fi

echo "Closing references (auto-close on merge):"
echo "------------------------------------------"
closing=$(echo "$commits" | grep -oE "$CLOSING_PATTERN" | sort | uniq)
if [[ -n "$closing" ]]; then
    echo "$closing" | while read -r ref; do
        echo "  $ref"
    done
else
    echo "  None found"
fi

echo ""
echo "Non-closing references (link only):"
echo "------------------------------------"
non_closing=$(echo "$commits" | grep -oE "$NON_CLOSING_PATTERN" | sort | uniq)
if [[ -n "$non_closing" ]]; then
    echo "$non_closing" | while read -r ref; do
        echo "  $ref"
    done
else
    echo "  None found"
fi

echo ""
echo "All referenced issues:"
echo "----------------------"
all_issues=$(echo "$commits" | grep -oE "[A-Z]+-[0-9]+" | sort | uniq)
if [[ -n "$all_issues" ]]; then
    echo "$all_issues" | while read -r issue; do
        echo "  $issue"
    done
else
    echo "  None found"
fi
