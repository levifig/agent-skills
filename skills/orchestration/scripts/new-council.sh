#!/bin/bash
# Generate a new council file with correct format
# Usage: new-council.sh <topic> <session-filename> <participant1> <participant2> ...
# Example: new-council.sh database-schema-choice 20251204-143000-auth-system backend-dev dba devops security testing-qa

set -e

TOPIC="${1:?Usage: new-council.sh <topic> <session-filename> <participant1> <participant2> ...}"
SESSION="${2:?Session filename required}"
shift 2
PARTICIPANTS=("$@")

# Validate minimum participants (5) and odd count
if [[ ${#PARTICIPANTS[@]} -lt 5 ]]; then
    echo "Error: Council requires at least 5 participants (got ${#PARTICIPANTS[@]})" >&2
    exit 1
fi

if [[ $((${#PARTICIPANTS[@]} % 2)) -eq 0 ]]; then
    echo "Error: Council requires an odd number of participants (got ${#PARTICIPANTS[@]})" >&2
    exit 1
fi

# Generate timestamps
TIMESTAMP=$(date -u +"%Y%m%d-%H%M%S")
ISO_TIMESTAMP=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

# Validate topic format (kebab-case)
if [[ ! "$TOPIC" =~ ^[a-z0-9-]+$ ]]; then
    echo "Error: Topic must be kebab-case (lowercase letters, numbers, hyphens)" >&2
    exit 1
fi

# Validate session file exists
SESSION_PATH=".agents/sessions/${SESSION}.md"
if [[ ! -f "$SESSION_PATH" ]]; then
    echo "Error: Session file not found: $SESSION_PATH" >&2
    exit 1
fi

# Build filename
FILENAME="${TIMESTAMP}-${TOPIC}.md"
FILEPATH=".agents/councils/${FILENAME}"

# Check if file already exists
if [[ -f "$FILEPATH" ]]; then
    echo "Error: Council file already exists: $FILEPATH" >&2
    exit 1
fi

# Build participants YAML
PARTICIPANTS_YAML=""
for p in "${PARTICIPANTS[@]}"; do
    PARTICIPANTS_YAML="${PARTICIPANTS_YAML}    - ${p}
"
done

# Generate council file
cat > "$FILEPATH" << EOF
---
council:
  topic: "${TOPIC//-/ }"
  timestamp: "${ISO_TIMESTAMP}"
  status: pending
  session: "${SESSION}"
  participants:
${PARTICIPANTS_YAML}  decision: ""
---

# Council: ${TOPIC//-/ }

## Context

Why this council was convened.
What problem or decision needed multiple perspectives.

## Options Considered

### Option A: [Name]
Brief description and trade-offs.

### Option B: [Name]
Brief description and trade-offs.

## Agent Perspectives

$(for p in "${PARTICIPANTS[@]}"; do
echo "### ${p}

**Position**: [Option X]

**Pros**:
-

**Cons**:
-

**Concerns**:
-

"
done)

## Synthesis

**Consensus Points**:
-

**Key Disagreements**:
-

**Recommendation**: [Option X]

## Decision

**Chosen**: [Pending user approval]

## Rationale

[To be filled after deliberation]

## Implementation Notes

[To be filled after decision]
EOF

echo "Created: $FILEPATH"
echo "Participants: ${PARTICIPANTS[*]}"
