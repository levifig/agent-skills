#!/bin/bash
# Validate commit message format
# Usage: check-commit-msg.sh <commit-message-file>
#        echo "message" | check-commit-msg.sh -
#
# Expected format: <type>: <description>
# Types: feat, fix, docs, test, refactor, chore

set -e

# Read commit message
if [[ "$1" == "-" ]]; then
    MSG=$(cat)
elif [[ -n "$1" ]]; then
    MSG=$(cat "$1")
else
    echo "Usage: check-commit-msg.sh <commit-message-file>"
    echo "       echo 'message' | check-commit-msg.sh -"
    exit 1
fi

# Get first line (subject)
SUBJECT=$(echo "$MSG" | head -1)

echo "Checking commit message: $SUBJECT"
echo "================================================"

errors=0

# Valid types
VALID_TYPES="feat|fix|docs|test|refactor|chore|ci|build|perf"

# Check format: <type>: <description>
if ! echo "$SUBJECT" | grep -qE "^($VALID_TYPES): .+"; then
    echo "✗ Format must be '<type>: <description>'"
    echo "  Valid types: feat, fix, docs, test, refactor, chore, ci, build, perf"
    errors=$((errors + 1))
fi

# Check for scoped commits (not allowed)
if echo "$SUBJECT" | grep -qE "^[a-z]+\([^)]+\):"; then
    echo "✗ Scoped commits not allowed (e.g., 'feat(auth):' should be 'feat:')"
    errors=$((errors + 1))
fi

# Check length (50 chars recommended, 72 max)
if [[ ${#SUBJECT} -gt 72 ]]; then
    echo "✗ Subject line too long (${#SUBJECT} chars, max 72)"
    errors=$((errors + 1))
elif [[ ${#SUBJECT} -gt 50 ]]; then
    echo "⚠ Subject line over 50 chars (${#SUBJECT}), consider shortening"
fi

# Check for imperative mood indicators
BAD_STARTS="Added|Fixed|Updated|Removed|Changed|Deleted|Created"
if echo "$SUBJECT" | grep -qE ": ($BAD_STARTS) "; then
    echo "⚠ Use imperative mood ('Add feature' not 'Added feature')"
fi

# Check for agent attribution (not allowed)
if echo "$MSG" | grep -qiE "claude|gpt|copilot|ai assistant"; then
    echo "✗ No agent attribution in commit messages"
    errors=$((errors + 1))
fi

# Check for file lists in message (not recommended)
if echo "$MSG" | grep -qE "^\s*[-*]\s+\`?[a-zA-Z0-9_/]+\.(py|ts|js|md|yaml|json)\`?"; then
    echo "⚠ Avoid listing files in commit message (the diff shows this)"
fi

# Check for Linear magic words in body
if echo "$MSG" | grep -qE "(Closes|Fixes|Resolves|Refs|Part of) [A-Z]+-[0-9]+"; then
    echo "✓ Linear integration: $(echo "$MSG" | grep -oE "(Closes|Fixes|Resolves|Refs|Part of) [A-Z]+-[0-9]+")"
fi

echo ""
if [[ $errors -eq 0 ]]; then
    echo "✓ Commit message is valid"
    exit 0
else
    echo "✗ Found $errors error(s)"
    exit 1
fi
