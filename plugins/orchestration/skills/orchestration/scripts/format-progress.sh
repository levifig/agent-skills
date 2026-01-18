#!/bin/bash
# Format progress update for Linear
# Usage: format-progress.sh [completed-items...] -- [pending-items...]
#        format-progress.sh -f <file>
#
# Examples:
#   format-progress.sh "Database schema" "API endpoints" -- "Integration tests"
#   format-progress.sh -f progress.txt

set -e

# Check for file input
if [[ "$1" == "-f" ]]; then
    if [[ -z "$2" ]]; then
        echo "Usage: format-progress.sh -f <file>"
        exit 1
    fi
    cat "$2"
    exit 0
fi

# Parse completed and pending items
completed=()
pending=()
parsing_pending=false

for arg in "$@"; do
    if [[ "$arg" == "--" ]]; then
        parsing_pending=true
        continue
    fi

    if $parsing_pending; then
        pending+=("$arg")
    else
        completed+=("$arg")
    fi
done

# Generate output
echo "## Progress"

for item in "${completed[@]}"; do
    echo "- [x] $item"
done

for item in "${pending[@]}"; do
    echo "- [ ] $item"
done

echo ""
echo "## Blockers"
echo "None currently."

# Show tips
echo ""
echo "---"
echo "Tips:"
echo "  - Copy the above to Linear issue comment"
echo "  - Remove 'Tips' section before pasting"
echo "  - Don't include file paths or agent references"
