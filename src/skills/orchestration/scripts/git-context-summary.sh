#!/bin/bash
# Generate git context summary for sessions
# Usage: git-context-summary.sh [--brief]
#
# Output includes:
#   - Current branch name
#   - Uncommitted changes count
#   - Recent commits (last 3-5)
#   - Summary of changes from main/master (if on feature branch)
#
# Options:
#   --brief   Output only branch and uncommitted changes (for hooks)

set -e

BRIEF_MODE=false
if [[ "$1" == "--brief" ]]; then
    BRIEF_MODE=true
fi

# Check if we're in a git repository
if ! git rev-parse --is-inside-work-tree &>/dev/null; then
    echo "Not a git repository"
    exit 0
fi

# Get current branch
CURRENT_BRANCH=$(git branch --show-current 2>/dev/null || echo "")
if [[ -z "$CURRENT_BRANCH" ]]; then
    # Detached HEAD state
    CURRENT_BRANCH="(detached HEAD at $(git rev-parse --short HEAD 2>/dev/null || echo 'unknown'))"
fi

# Count uncommitted changes (staged + unstaged + untracked)
UNCOMMITTED_COUNT=$(git status --porcelain 2>/dev/null | wc -l | tr -d ' ')

# Brief mode: just branch and changes
if [[ "$BRIEF_MODE" == "true" ]]; then
    echo "## Git Context"
    echo "- **Branch**: $CURRENT_BRANCH"
    echo "- **Uncommitted changes**: $UNCOMMITTED_COUNT file(s)"
    exit 0
fi

# Full output
echo "## Git Context"
echo ""
echo "**Branch**: \`$CURRENT_BRANCH\`"
echo ""
echo "**Uncommitted changes**: $UNCOMMITTED_COUNT file(s)"

# Show brief status if there are changes
if [[ "$UNCOMMITTED_COUNT" -gt 0 ]]; then
    echo ""
    echo "**Changed files**:"
    # Show first 10 files max
    git status --porcelain 2>/dev/null | head -10 | while read -r line; do
        STATUS="${line:0:2}"
        FILE="${line:3}"
        case "$STATUS" in
            "M "| " M"|"MM") echo "  - Modified: \`$FILE\`" ;;
            "A ") echo "  - Added: \`$FILE\`" ;;
            "D "| " D") echo "  - Deleted: \`$FILE\`" ;;
            "R ") echo "  - Renamed: \`$FILE\`" ;;
            "??") echo "  - Untracked: \`$FILE\`" ;;
            *) echo "  - Changed: \`$FILE\`" ;;
        esac
    done
    TOTAL=$(git status --porcelain 2>/dev/null | wc -l | tr -d ' ')
    if [[ "$TOTAL" -gt 10 ]]; then
        echo "  - ... and $((TOTAL - 10)) more"
    fi
fi

# Recent commits (last 5)
echo ""
echo "**Recent commits**:"
git log --oneline -5 2>/dev/null | while read -r line; do
    echo "  - \`$line\`"
done

# Detect main branch (main or master)
MAIN_BRANCH=""
if git show-ref --verify --quiet refs/heads/main 2>/dev/null; then
    MAIN_BRANCH="main"
elif git show-ref --verify --quiet refs/heads/master 2>/dev/null; then
    MAIN_BRANCH="master"
fi

# Summary of changes from main (if on feature branch)
if [[ -n "$MAIN_BRANCH" && "$CURRENT_BRANCH" != "$MAIN_BRANCH" && "$CURRENT_BRANCH" != "(detached"* ]]; then
    # Count commits ahead of main
    COMMITS_AHEAD=$(git rev-list --count "$MAIN_BRANCH..$CURRENT_BRANCH" 2>/dev/null || echo "0")
    COMMITS_BEHIND=$(git rev-list --count "$CURRENT_BRANCH..$MAIN_BRANCH" 2>/dev/null || echo "0")

    echo ""
    echo "**Relative to \`$MAIN_BRANCH\`**:"
    echo "  - $COMMITS_AHEAD commit(s) ahead"
    echo "  - $COMMITS_BEHIND commit(s) behind"

    # Files changed from main
    if [[ "$COMMITS_AHEAD" -gt 0 ]]; then
        FILES_CHANGED=$(git diff --name-only "$MAIN_BRANCH...$CURRENT_BRANCH" 2>/dev/null | wc -l | tr -d ' ')
        if [[ "$FILES_CHANGED" -gt 0 ]]; then
            echo "  - $FILES_CHANGED file(s) changed from \`$MAIN_BRANCH\`"
        fi
    fi
fi
