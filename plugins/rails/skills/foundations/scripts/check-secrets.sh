#!/usr/bin/env bash
# Scan for hardcoded secrets in source files
# Usage: check-secrets.sh [directory]

set -euo pipefail

DIR="${1:-.}"

echo "Scanning for secrets in: $DIR"
echo "================================"

ERRORS=0

# Patterns that indicate hardcoded secrets
PATTERNS=(
    'password\s*=\s*["\x27][^"\x27]+'
    'secret\s*=\s*["\x27][^"\x27]+'
    'api_key\s*=\s*["\x27][^"\x27]+'
    'apikey\s*=\s*["\x27][^"\x27]+'
    'token\s*=\s*["\x27][^"\x27]+'
    'private_key\s*=\s*["\x27][^"\x27]+'
    'AWS_ACCESS_KEY_ID\s*=\s*["\x27]?[A-Z0-9]{20}'
    'AWS_SECRET_ACCESS_KEY\s*=\s*["\x27]?[A-Za-z0-9/+=]{40}'
    'GITHUB_TOKEN\s*=\s*["\x27]?gh[ps]_[A-Za-z0-9]{36}'
)

# Files to exclude
EXCLUDE_PATTERNS=(
    '*.example*'
    '*.md'
    '*.rst'
    '*test*'
    '*mock*'
    '*fixture*'
    '.git'
    'node_modules'
    '__pycache__'
    '.venv'
    'venv'
)

# Build exclude arguments
EXCLUDE_ARGS=""
for pattern in "${EXCLUDE_PATTERNS[@]}"; do
    EXCLUDE_ARGS="$EXCLUDE_ARGS --exclude=$pattern"
done

for pattern in "${PATTERNS[@]}"; do
    # shellcheck disable=SC2086
    MATCHES=$(grep -rniE "$pattern" "$DIR" $EXCLUDE_ARGS 2>/dev/null || true)
    if [[ -n "$MATCHES" ]]; then
        echo "⚠️  Potential secret found (pattern: $pattern):"
        echo "$MATCHES" | head -5
        if [[ $(echo "$MATCHES" | wc -l) -gt 5 ]]; then
            echo "   ... and more"
        fi
        echo ""
        ((ERRORS++)) || true
    fi
done

# Check for .env files that shouldn't be committed
ENV_FILES=$(find "$DIR" -name ".env" -o -name ".env.local" -o -name ".env.*.local" 2>/dev/null | grep -v node_modules || true)
if [[ -n "$ENV_FILES" ]]; then
    echo "⚠️  .env files found (should be gitignored):"
    echo "$ENV_FILES"
    echo ""
    ((ERRORS++)) || true
fi

# Check for private keys
KEY_FILES=$(find "$DIR" -name "*.pem" -o -name "*.key" -o -name "id_rsa*" 2>/dev/null | grep -v node_modules || true)
if [[ -n "$KEY_FILES" ]]; then
    echo "⚠️  Private key files found:"
    echo "$KEY_FILES"
    echo ""
    ((ERRORS++)) || true
fi

echo "================================"
if [[ $ERRORS -gt 0 ]]; then
    echo "Found $ERRORS potential issue(s)"
    echo "Review each finding - some may be false positives"
    exit 1
else
    echo "No obvious secrets found"
    exit 0
fi
