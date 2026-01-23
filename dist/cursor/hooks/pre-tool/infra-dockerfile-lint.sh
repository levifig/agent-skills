#!/bin/bash
# Infrastructure plugin hook: Check Dockerfiles for common issues
# Type: PreToolUse (informational, exit 0)
# Trigger: Before Edit/Write on Dockerfile*

# Get the file path from environment
FILE_PATH="${CLAUDE_FILE_PATH:-}"

# Only check Dockerfiles
if [[ ! "$FILE_PATH" =~ Dockerfile ]]; then
    exit 0
fi

# Get the content being written (if available)
CONTENT="${CLAUDE_FILE_CONTENT:-}"

# If no content, try to read the file
if [[ -z "$CONTENT" && -f "$FILE_PATH" ]]; then
    CONTENT=$(cat "$FILE_PATH")
fi

WARNINGS=""

# Check for root user (should use non-root)
if ! echo "$CONTENT" | grep -qE "^USER\s+[^r]|useradd|adduser"; then
    if echo "$CONTENT" | grep -qE "^FROM"; then
        WARNINGS="${WARNINGS}⚠️  No non-root USER specified - consider adding a non-root user\n"
    fi
fi

# Check for latest tag
if echo "$CONTENT" | grep -qE "^FROM\s+\S+:latest|^FROM\s+[^:]+\s*$"; then
    WARNINGS="${WARNINGS}⚠️  Using :latest or untagged base image - pin to specific version\n"
fi

# Check for HEALTHCHECK
if ! echo "$CONTENT" | grep -q "HEALTHCHECK"; then
    WARNINGS="${WARNINGS}ℹ️  No HEALTHCHECK instruction - consider adding health check\n"
fi

# Check for secrets in ENV
if echo "$CONTENT" | grep -qiE "^ENV\s+.*(PASSWORD|SECRET|KEY|TOKEN)"; then
    WARNINGS="${WARNINGS}⚠️  Secrets in ENV instruction - use runtime secrets instead\n"
fi

# Check for ADD vs COPY
if echo "$CONTENT" | grep -qE "^ADD\s+" && ! echo "$CONTENT" | grep -qE "^ADD\s+https?://"; then
    WARNINGS="${WARNINGS}ℹ️  Using ADD for local files - prefer COPY unless extracting archives\n"
fi

if [[ -n "$WARNINGS" ]]; then
    echo "🐳 Dockerfile Check: $FILE_PATH"
    echo ""
    echo -e "$WARNINGS"
fi

exit 0
