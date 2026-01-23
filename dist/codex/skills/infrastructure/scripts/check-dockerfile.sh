#!/usr/bin/env bash
# Check Dockerfile for best practices
# Usage: check-dockerfile.sh <dockerfile>

set -euo pipefail

DOCKERFILE="${1:-Dockerfile}"

if [[ ! -f "$DOCKERFILE" ]]; then
    echo "Error: Dockerfile not found: $DOCKERFILE"
    exit 1
fi

ERRORS=0

echo "Checking: $DOCKERFILE"
echo "================================"

# Check for multi-stage build
if ! grep -q "^FROM.*AS" "$DOCKERFILE"; then
    echo "⚠️  No multi-stage build detected"
    ((ERRORS++)) || true
else
    echo "✓ Multi-stage build detected"
fi

# Check for non-root user
if ! grep -q "^USER" "$DOCKERFILE"; then
    echo "⚠️  No USER directive found (running as root)"
    ((ERRORS++)) || true
else
    echo "✓ USER directive found"
fi

# Check for HEALTHCHECK
if ! grep -q "^HEALTHCHECK" "$DOCKERFILE"; then
    echo "⚠️  No HEALTHCHECK defined"
    ((ERRORS++)) || true
else
    echo "✓ HEALTHCHECK defined"
fi

# Check for latest tag
if grep -q "FROM.*:latest" "$DOCKERFILE"; then
    echo "⚠️  Using :latest tag (specify version)"
    ((ERRORS++)) || true
else
    echo "✓ No :latest tags"
fi

# Check for apt cleanup
if grep -q "apt-get install" "$DOCKERFILE"; then
    if ! grep -q "rm -rf /var/lib/apt/lists" "$DOCKERFILE"; then
        echo "⚠️  apt-get used without cleanup"
        ((ERRORS++)) || true
    else
        echo "✓ apt lists cleaned up"
    fi
fi

# Check for PYTHONUNBUFFERED
if grep -q "python" "$DOCKERFILE"; then
    if ! grep -q "PYTHONUNBUFFERED" "$DOCKERFILE"; then
        echo "⚠️  Python detected but PYTHONUNBUFFERED not set"
        ((ERRORS++)) || true
    else
        echo "✓ PYTHONUNBUFFERED set"
    fi
fi

# Check for secrets
if grep -Eq "(PASSWORD|SECRET|KEY|TOKEN)=" "$DOCKERFILE"; then
    echo "❌ Potential secrets in Dockerfile!"
    ((ERRORS++)) || true
else
    echo "✓ No obvious secrets"
fi

echo "================================"
if [[ $ERRORS -gt 0 ]]; then
    echo "Found $ERRORS issue(s)"
    exit 1
else
    echo "All checks passed!"
    exit 0
fi
