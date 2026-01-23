#!/usr/bin/env bash
# Validates infrastructure commands for destructive operations
# Blocks dangerous kubectl, terraform, docker commands unless explicitly confirmed
#
# Exit codes:
#   0 - Allow operation
#   2 - Block operation (message via stderr)

set -euo pipefail

# Read JSON input from stdin
INPUT=$(cat)

# Extract command from tool_input
COMMAND=$(echo "$INPUT" | jq -r '.tool_input.command // empty' 2>/dev/null || echo "")

if [[ -z "$COMMAND" ]]; then
  exit 0
fi

# --- Kubernetes ---

# Block kubectl delete namespace/all
if echo "$COMMAND" | grep -qE 'kubectl\s+delete\s+(namespace|ns)\b'; then
  echo "⛔ BLOCKED: kubectl delete namespace detected." >&2
  echo "This will delete the entire namespace and all resources within it." >&2
  echo "If intentional, ask the user to confirm before proceeding." >&2
  exit 2
fi

if echo "$COMMAND" | grep -qE 'kubectl\s+delete\s+.*--all\b'; then
  echo "⛔ BLOCKED: kubectl delete --all detected." >&2
  echo "This will delete all resources of that type." >&2
  echo "If intentional, ask the user to confirm before proceeding." >&2
  exit 2
fi

# Warn on kubectl delete (but allow specific resource deletion)
if echo "$COMMAND" | grep -qE 'kubectl\s+delete\b'; then
  echo "⚠️  WARNING: kubectl delete detected. Verify the target resource." >&2
fi

# --- Terraform ---

# Block terraform destroy without confirmation
if echo "$COMMAND" | grep -qE 'terraform\s+destroy\b' && ! echo "$COMMAND" | grep -qE '\-auto-approve'; then
  echo "⚠️  WARNING: terraform destroy detected." >&2
  echo "This will destroy infrastructure. Terraform will prompt for confirmation." >&2
fi

if echo "$COMMAND" | grep -qE 'terraform\s+destroy\b.*-auto-approve'; then
  echo "⛔ BLOCKED: terraform destroy -auto-approve detected." >&2
  echo "Auto-approved destroy is dangerous. Remove -auto-approve flag." >&2
  echo "If intentional, ask the user to confirm before proceeding." >&2
  exit 2
fi

# --- Docker ---

# Block docker system prune -a
if echo "$COMMAND" | grep -qE 'docker\s+system\s+prune\s+(-a|--all)'; then
  echo "⛔ BLOCKED: docker system prune -a detected." >&2
  echo "This removes all unused images, not just dangling ones." >&2
  echo "If intentional, ask the user to confirm before proceeding." >&2
  exit 2
fi

# Warn on docker rm/rmi with force
if echo "$COMMAND" | grep -qE 'docker\s+(rm|rmi)\s+.*(-f|--force)'; then
  echo "⚠️  WARNING: Forced docker removal detected. Verify targets." >&2
fi

# --- Helm ---

# Block helm uninstall without confirmation
if echo "$COMMAND" | grep -qE 'helm\s+uninstall\b'; then
  echo "⚠️  WARNING: helm uninstall detected." >&2
  echo "This will remove the release and its resources." >&2
fi

# --- General ---

# Block rm -rf on system paths
if echo "$COMMAND" | grep -qE 'rm\s+(-rf|-fr)\s+(/|/usr|/etc|/var|/home)\b'; then
  echo "⛔ BLOCKED: Dangerous rm -rf on system path detected." >&2
  echo "This could damage the system." >&2
  exit 2
fi

exit 0
