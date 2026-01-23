#!/bin/bash
#
# List available session decision memories from Serena.
#
# Usage: list-session-decisions.sh
#
# This script lists Serena memories matching the pattern session-*-decisions.md
# and displays session name and metadata.
#
# Note: This is a helper script. In practice, agents should use Serena MCP
# directly via mcp__serena__list_memories() for programmatic access.

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo -e "${BLUE}Session Decision Memories${NC}"
echo "========================="
echo ""

# Check if .serena directory exists
if [[ ! -d ".serena" ]]; then
    echo -e "${YELLOW}No .serena directory found in current project.${NC}"
    echo "Serena memories are stored per-project. Make sure you're in a Serena-enabled project."
    exit 0
fi

# Check for memories directory
MEMORIES_DIR=".serena/memories"
if [[ ! -d "$MEMORIES_DIR" ]]; then
    echo -e "${YELLOW}No memories directory found.${NC}"
    echo "No session decisions have been extracted yet."
    echo ""
    echo "To extract decisions from a session:"
    echo "  python3 extract-decisions.py <session-file>"
    echo "  Then use Serena MCP to write the memory."
    exit 0
fi

# Find session decision memories
MEMORIES=$(find "$MEMORIES_DIR" -name "session-*-decisions.md" 2>/dev/null | sort -r)

if [[ -z "$MEMORIES" ]]; then
    echo -e "${YELLOW}No session decision memories found.${NC}"
    echo ""
    echo "Session decision memories are created when sessions are archived."
    echo "Pattern: session-<slug>-decisions.md"
    exit 0
fi

# Display memories
count=0
echo -e "${GREEN}Available Decision Memories:${NC}"
echo ""

for memory in $MEMORIES; do
    count=$((count + 1))
    filename=$(basename "$memory")

    # Extract slug from filename
    slug=$(echo "$filename" | sed 's/^session-//' | sed 's/-decisions\.md$//')

    # Try to read session info from memory content
    if [[ -f "$memory" ]]; then
        session_name=$(grep -m1 "^\- \*\*Session\*\*:" "$memory" 2>/dev/null | sed 's/.*: //' || echo "Unknown")
        archived=$(grep -m1 "^\- \*\*Archived\*\*:" "$memory" 2>/dev/null | sed 's/.*: //' || echo "Unknown")
        linear=$(grep -m1 "^\- \*\*Linear Issue\*\*:" "$memory" 2>/dev/null | sed 's/.*: //' || echo "N/A")
        decision_count=$(grep -c "^### Decision" "$memory" 2>/dev/null || echo "0")

        printf "  %2d. %s\n" "$count" "$slug"
        printf "      Session:  %s\n" "$session_name"
        printf "      Archived: %s\n" "$archived"
        printf "      Linear:   %s\n" "$linear"
        printf "      Decisions: %s\n" "$decision_count"
        echo ""
    else
        printf "  %2d. %s (unreadable)\n" "$count" "$filename"
    fi
done

echo "------------------------"
echo "Total: $count decision memories"
echo ""
echo -e "${BLUE}Usage:${NC}"
echo "  /reference-session <search-term>    # Import decisions to current session"
echo "  mcp__serena__read_memory(name=...)  # Read memory directly via MCP"
