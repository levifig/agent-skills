#!/bin/bash
# Hook: Check for secrets/credentials in code (BLOCKING)
# PreToolUse hook - reads JSON from stdin per Claude Code hooks API
#
# Triggers on Edit|Write to source files
# Blocks if potential secrets are detected
#
# Exit codes:
#   0 - Allow (no secrets detected)
#   2 - Block (potential secrets found)

set -euo pipefail

# Read and parse stdin JSON
INPUT=$(cat)
TOOL_NAME=$(echo "$INPUT" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('tool_name',''))" 2>/dev/null || echo "")
FILE_PATH=$(echo "$INPUT" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('tool_input',{}).get('file_path',''))" 2>/dev/null || echo "")
NEW_CONTENT=$(echo "$INPUT" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('tool_input',{}).get('content','') or d.get('tool_input',{}).get('new_string',''))" 2>/dev/null || echo "")

# Skip non-file operations
if [[ -z "$FILE_PATH" ]]; then
  exit 0
fi

# Skip known safe files
SAFE_PATTERNS=(
  "*.md"
  "*.txt"
  "*.lock"
  "package-lock.json"
  "yarn.lock"
  "poetry.lock"
  "*.example"
  "*.template"
  "*.sample"
)

for pattern in "${SAFE_PATTERNS[@]}"; do
  if [[ "$FILE_PATH" == $pattern ]]; then
    exit 0
  fi
done

# Secret patterns to detect (high confidence)
SECRET_PATTERNS=(
  # AWS
  'AKIA[0-9A-Z]{16}'                           # AWS Access Key ID
  'aws_secret_access_key\s*=\s*["\047][A-Za-z0-9/+=]{40}["\047]'
  # API Keys (generic)
  '["\047]sk-[a-zA-Z0-9]{48}["\047]'           # OpenAI API key
  '["\047]sk_live_[a-zA-Z0-9]{24}["\047]'      # Stripe live key
  '["\047]sk_test_[a-zA-Z0-9]{24}["\047]'      # Stripe test key
  # Private keys
  '-----BEGIN (RSA |EC |DSA |OPENSSH )?PRIVATE KEY-----'
  # Database connection strings with passwords
  '(postgres|mysql|mongodb)://[^:]+:[^@]+@'
  # Generic password assignments
  'password\s*=\s*["\047][^"\047]{8,}["\047]'
  'secret\s*=\s*["\047][^"\047]{8,}["\047]'
  'api_key\s*=\s*["\047][^"\047]{16,}["\047]'
  # JWT tokens
  'eyJ[A-Za-z0-9-_]+\.eyJ[A-Za-z0-9-_]+\.[A-Za-z0-9-_.+/=]*'
  # GitHub tokens
  'gh[pousr]_[A-Za-z0-9_]{36}'
)

# Check content for secrets
FOUND_SECRETS=""
for pattern in "${SECRET_PATTERNS[@]}"; do
  if echo "$NEW_CONTENT" | grep -qE "$pattern" 2>/dev/null; then
    MATCH=$(echo "$NEW_CONTENT" | grep -oE "$pattern" 2>/dev/null | head -1 | cut -c1-40)
    FOUND_SECRETS="${FOUND_SECRETS}\n  - Pattern match: ${MATCH}..."
  fi
done

if [[ -n "$FOUND_SECRETS" ]]; then
  cat << EOF >&2
BLOCKED: Potential secrets detected in $FILE_PATH

Detected patterns:$FOUND_SECRETS

Please:
1. Use environment variables instead of hardcoded secrets
2. Reference secrets from a .env file (add .env to .gitignore)
3. Use a secrets manager (AWS Secrets Manager, HashiCorp Vault, etc.)

If this is a false positive (e.g., example/placeholder values), you can:
- Use obviously fake values: "YOUR_API_KEY_HERE"
- Add to a .example or .template file instead
EOF
  exit 2
fi

exit 0
