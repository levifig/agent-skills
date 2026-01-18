#!/bin/bash
# Hook: Validate Kubernetes manifests (INFORMATIONAL)
# PreToolUse hook - reads JSON from stdin per Claude Code hooks API
#
# Triggers on Bash commands applying k8s manifests
# Outputs validation warnings (non-blocking)
#
# Exit codes:
#   0 - Always (informational only)

# Read and parse stdin JSON
INPUT=$(cat)
COMMAND=$(echo "$INPUT" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('tool_input',{}).get('command',''))" 2>/dev/null || echo "")

# Only check kubectl/helm commands
if ! echo "$COMMAND" | grep -qE "(kubectl apply|kubectl create|helm install|helm upgrade)" 2>/dev/null; then
  exit 0
fi

# Extract file path from -f flag
MANIFEST_FILE=$(echo "$COMMAND" | grep -oE '\-f\s+[^ ]+' | sed 's/-f\s*//' || true)

if [[ -z "$MANIFEST_FILE" ]] || [[ ! -f "$MANIFEST_FILE" ]]; then
  exit 0
fi

echo "# K8s Manifest Check: $MANIFEST_FILE"
echo ""

# Check for common issues
WARNINGS=""

# Check for missing resource limits
if grep -qE "kind:\s*(Deployment|StatefulSet|DaemonSet)" "$MANIFEST_FILE" 2>/dev/null; then
  if ! grep -q "resources:" "$MANIFEST_FILE" 2>/dev/null; then
    WARNINGS="${WARNINGS}- Missing resource limits/requests (CPU, memory)\n"
  fi
fi

# Check for missing liveness/readiness probes
if grep -qE "kind:\s*(Deployment|StatefulSet)" "$MANIFEST_FILE" 2>/dev/null; then
  if ! grep -q "livenessProbe:" "$MANIFEST_FILE" 2>/dev/null; then
    WARNINGS="${WARNINGS}- Missing livenessProbe\n"
  fi
  if ! grep -q "readinessProbe:" "$MANIFEST_FILE" 2>/dev/null; then
    WARNINGS="${WARNINGS}- Missing readinessProbe\n"
  fi
fi

# Check for 'latest' tag
if grep -qE "image:.*:latest" "$MANIFEST_FILE" 2>/dev/null; then
  WARNINGS="${WARNINGS}- Using ':latest' tag (prefer specific version tags)\n"
fi

# Check for privileged containers
if grep -qE "privileged:\s*true" "$MANIFEST_FILE" 2>/dev/null; then
  WARNINGS="${WARNINGS}- Container running as privileged (security concern)\n"
fi

# Check for missing namespace
if ! grep -q "namespace:" "$MANIFEST_FILE" 2>/dev/null; then
  WARNINGS="${WARNINGS}- No namespace specified (will use 'default')\n"
fi

if [[ -n "$WARNINGS" ]]; then
  echo "Suggestions for $MANIFEST_FILE:"
  echo -e "$WARNINGS"
  echo ""
  echo "Run 'kubectl apply --dry-run=client -f $MANIFEST_FILE' to validate syntax"
fi

# Run dry-run if kubectl available
if command -v kubectl &> /dev/null; then
  DRY_RUN=$(kubectl apply --dry-run=client -f "$MANIFEST_FILE" 2>&1 || true)
  if echo "$DRY_RUN" | grep -qi "error" 2>/dev/null; then
    echo "Validation errors:"
    echo "$DRY_RUN"
  fi
fi

exit 0
