#!/bin/bash
# Archive Context Before Compaction
# Saves session context and active work before Claude Code compacts the conversation
# Exit 0 (informational only)

set -euo pipefail

# Source shared libraries
source "${CLAUDE_PLUGIN_ROOT}/hooks/lib/config-reader.sh"

# Check if archiving is enabled
if ! is_hook_enabled "archive-context"; then
    exit 0
fi

# Get project root
PROJECT_ROOT=$(git rev-parse --show-toplevel 2>/dev/null || echo ".")

# Create archive directory
ARCHIVE_DIR="${PROJECT_ROOT}/.context-snapshots"
mkdir -p "${ARCHIVE_DIR}"

# Generate timestamp
TIMESTAMP=$(date +"%Y%m%d_%H%M%S")
ARCHIVE_PATH="${ARCHIVE_DIR}/context_${TIMESTAMP}"

# Create archive subdirectory
mkdir -p "${ARCHIVE_PATH}"

echo "📦 Archiving context before compaction..."

# Archive active sessions (if they exist)
SESSIONS_DIR="${PROJECT_ROOT}/.work/sessions"
if [[ -d "${SESSIONS_DIR}" ]]; then
    ACTIVE_SESSIONS=$(find "${SESSIONS_DIR}" -name "*.md" -type f 2>/dev/null | wc -l | tr -d ' ')
    if [[ "${ACTIVE_SESSIONS}" -gt 0 ]]; then
        cp -r "${SESSIONS_DIR}" "${ARCHIVE_PATH}/sessions"
        echo "   ✓ Archived ${ACTIVE_SESSIONS} session file(s)"
    fi
fi

# Archive council decisions (if they exist)
COUNCILS_DIR="${PROJECT_ROOT}/.work/councils"
if [[ -d "${COUNCILS_DIR}" ]]; then
    COUNCIL_FILES=$(find "${COUNCILS_DIR}" -name "*.md" -type f 2>/dev/null | wc -l | tr -d ' ')
    if [[ "${COUNCIL_FILES}" -gt 0 ]]; then
        cp -r "${COUNCILS_DIR}" "${ARCHIVE_PATH}/councils"
        echo "   ✓ Archived ${COUNCIL_FILES} council file(s)"
    fi
fi

# Archive work tracker (if it exists)
WORK_TRACKER="${PROJECT_ROOT}/.work/tracker.md"
if [[ -f "${WORK_TRACKER}" ]]; then
    cp "${WORK_TRACKER}" "${ARCHIVE_PATH}/tracker.md"
    echo "   ✓ Archived work tracker"
fi

# Create metadata file
cat > "${ARCHIVE_PATH}/metadata.json" <<EOF
{
  "archived_at": "$(date -u +"%Y-%m-%dT%H:%M:%SZ")",
  "reason": "pre-compaction",
  "project_root": "${PROJECT_ROOT}"
}
EOF

echo "   ✓ Context archived to: ${ARCHIVE_PATH}"

# Clean old archives (keep last 10)
ARCHIVE_COUNT=$(find "${ARCHIVE_DIR}" -maxdepth 1 -type d -name "context_*" 2>/dev/null | wc -l | tr -d ' ')
if [[ "${ARCHIVE_COUNT}" -gt 10 ]]; then
    ARCHIVES_TO_DELETE=$((ARCHIVE_COUNT - 10))
    find "${ARCHIVE_DIR}" -maxdepth 1 -type d -name "context_*" -print0 | \
        xargs -0 ls -dt | \
        tail -n "${ARCHIVES_TO_DELETE}" | \
        xargs rm -rf
    echo "   ✓ Cleaned ${ARCHIVES_TO_DELETE} old archive(s)"
fi

echo ""
echo "💡 Context preserved. Resume work after compaction by reviewing:"
echo "   ${ARCHIVE_PATH}"

exit 0
