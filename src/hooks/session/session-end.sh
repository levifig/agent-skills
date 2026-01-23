#!/bin/bash
# Hook: Comprehensive session end checklist (INFORMATIONAL)
# SessionEnd hook - agent-aware checklists before session closes
#
# Triggers when Claude Code session ends
# Displays agent-specific completion checklists

# Use CLAUDE_PROJECT_DIR if available, otherwise current directory
SESSIONS_DIR="${CLAUDE_PROJECT_DIR:-.}/.agents/sessions"

# Detect agent type from environment
AGENT_TYPE="${AGENT_TYPE:-unknown}"

# Count session files (excluding archive subdirectory)
SESSION_COUNT=0
if [ -d "$SESSIONS_DIR" ]; then
  SESSION_COUNT=$(find "$SESSIONS_DIR" -maxdepth 1 -name "*.md" -type f 2>/dev/null | wc -l | tr -d ' ')
fi

if [ "$SESSION_COUNT" -eq 0 ]; then
  # No sessions, no checklist needed
  exit 0
fi

echo "# Session End Checklist"
echo ""
echo "**$SESSION_COUNT** active session file(s) found."
echo ""

# Agent-specific checklists
case "$AGENT_TYPE" in
  pm-orchestrator|product)
    # PM agents: Full completion checklist
    echo "## PM Orchestration Checklist"
    echo ""
    echo "- [ ] All spawned agents completed their tasks"
    echo "- [ ] Session outcomes logged in session file"
    echo "- [ ] Linear issues synced with actual progress"
    echo "- [ ] Decisions captured (ADRs if architectural)"
    echo "- [ ] Session file \`## Current State\` is handoff-ready"
    echo "- [ ] **Code review completed** (run \`pr-review-toolkit:code-reviewer\` if significant changes)"
    echo "- [ ] Completed sessions deleted (knowledge extracted)"
    echo "- [ ] Active sessions have clear next steps"
    echo ""
    echo "### Code Review Reminder"
    echo ""
    echo "If this session involved significant code changes, consider running:"
    echo "\`\`\`"
    echo "Task(subagent_type=\"pr-review-toolkit:code-reviewer\", prompt=\"Review recent changes for this session\")"
    echo "\`\`\`"
    ;;

  backend-dev|frontend-dev|rails-dev|dba|devops)
    # Implementation agents: Outcome reminder
    echo "## Implementation Outcome Checklist"
    echo ""
    echo "- [ ] Tests pass for implemented changes"
    echo "- [ ] Code follows project style conventions"
    echo "- [ ] Session file updated with outcomes (if applicable)"
    echo "- [ ] Changes aligned with session requirements"
    echo "- [ ] Documentation updated (if significant changes)"
    ;;

  code-reviewer|security|testing-qa)
    # Quality agents: Review completion
    echo "## Quality Review Checklist"
    echo ""
    echo "- [ ] Review findings documented"
    echo "- [ ] Critical issues flagged appropriately"
    echo "- [ ] Session file updated with review outcomes"
    echo "- [ ] Recommendations captured for PM coordination"
    ;;

  design)
    # Design agent: Design deliverables
    echo "## Design Deliverables Checklist"
    echo ""
    echo "- [ ] Design decisions documented"
    echo "- [ ] UI/UX patterns consistent with project standards"
    echo "- [ ] Session file updated with design outcomes"
    echo "- [ ] Implementation guidance clear for developers"
    ;;

  *)
    # Generic checklist
    echo "## General Checklist"
    echo ""
    echo "- [ ] Work outcomes captured appropriately"
    echo "- [ ] Session files current and handoff-ready"
    echo "- [ ] No loose ends or undocumented decisions"
    ;;
esac

echo ""
echo "---"
echo ""
echo "**Active Sessions**: $SESSION_COUNT"
echo "**Tip**: Session files persist across conversations. Keep them current or delete when complete!"

exit 0
