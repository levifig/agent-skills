#!/bin/bash
# Rails Migration Safety - Deep Validation
# Tier 1: Quick static analysis (existing checks)
# Tier 2: Deep validation with migrate/rollback test
# Exit 2 if rollback fails, 0 otherwise

set -euo pipefail

# Source shared libraries
source "${CLAUDE_PLUGIN_ROOT}/hooks/lib/json-parser.sh"
source "${CLAUDE_PLUGIN_ROOT}/hooks/lib/config-reader.sh"
source "${CLAUDE_PLUGIN_ROOT}/hooks/lib/agent-detector.sh"
source "${CLAUDE_PLUGIN_ROOT}/hooks/lib/timeout-manager.sh"

# Start timeout tracker
start_timeout_tracker 300

# Check if deep validation is enabled
VALIDATION_LEVEL=$(get_validation_level)
DEEP_CHECK_ENABLED=$(get_config_value "hooks.migration-safety.deep_check" || echo "false")

# Read hook input from stdin
HOOK_INPUT=$(cat)
FILE_PATH=$(parse_file_path "${HOOK_INPUT}")
TOOL_NAME=$(parse_tool_name "${HOOK_INPUT}")

# Only run on Write/Edit operations
if [[ "${TOOL_NAME}" != "Write" ]] && [[ "${TOOL_NAME}" != "Edit" ]]; then
    exit 0
fi

# Only check migration files
if [[ ! "${FILE_PATH}" =~ db/migrate/.*\.rb$ ]]; then
    exit 0
fi

echo ""
echo "🛤️  Validating Rails Migration Safety..."
echo ""

# Tier 1: Static Analysis
echo "   → Tier 1: Static Analysis"

ISSUES=()

# Check for dangerous operations
if grep -q "remove_column" "${FILE_PATH}"; then
    ISSUES+=("⚠️  remove_column - Consider using strong_migrations gem")
fi

if grep -q "change_column" "${FILE_PATH}"; then
    ISSUES+=("⚠️  change_column - May cause downtime, use safe_* alternatives")
fi

if grep -q "add_column.*default:" "${FILE_PATH}"; then
    ISSUES+=("⚠️  add_column with default - May lock table on large tables")
fi

if grep -q "add_index.*algorithm:" "${FILE_PATH}"; then
    ISSUES+=("✓ Good: Using algorithm parameter for index creation")
else
    if grep -q "add_index" "${FILE_PATH}"; then
        ISSUES+=("💡 Consider adding 'algorithm: :concurrently' to avoid locks")
    fi
fi

if grep -q "execute.*ALTER TABLE.*ADD CONSTRAINT" "${FILE_PATH}"; then
    ISSUES+=("⚠️  Direct ALTER TABLE - Consider NOT VALID constraints")
fi

# Report Tier 1 findings
if [[ ${#ISSUES[@]} -gt 0 ]]; then
    echo "     Static analysis findings:"
    for issue in "${ISSUES[@]}"; do
        echo "     ${issue}"
    done
fi

# Tier 2: Deep Validation (if enabled and Rails env available)
if [[ "${DEEP_CHECK_ENABLED}" == "true" ]] || [[ "${VALIDATION_LEVEL}" == "thorough" ]]; then
    if ! check_remaining_time 120; then
        echo ""
        echo "   ⏱️  Skipping deep validation (insufficient time)"
        exit 0
    fi

    # Check if Rails is available
    PROJECT_ROOT=$(git rev-parse --show-toplevel 2>/dev/null || echo ".")

    if [[ -f "${PROJECT_ROOT}/bin/rails" ]] || command -v rails >/dev/null 2>&1; then
        echo ""
        echo "   → Tier 2: Deep Validation (migrate + rollback test)"

        cd "${PROJECT_ROOT}"

        # Store current schema version
        BEFORE_VERSION=$(bundle exec rails db:version 2>/dev/null | grep "Current version:" | awk '{print $3}' || echo "0")

        # Try to run migration
        echo "     Testing forward migration..."
        if RAILS_ENV=test bundle exec rails db:migrate:up VERSION="${FILE_PATH##*/}" 2>&1 | tee /tmp/migration-up.log; then
            echo "     ✓ Forward migration succeeded"

            # Try to rollback
            echo "     Testing rollback..."
            if RAILS_ENV=test bundle exec rails db:migrate:down VERSION="${FILE_PATH##*/}" 2>&1 | tee /tmp/migration-down.log; then
                echo "     ✓ Rollback succeeded"
            else
                echo ""
                echo "🚨 MIGRATION ROLLBACK FAILED"
                echo ""
                echo "   Migration can be applied but cannot be rolled back!"
                echo "   This violates reversibility requirements."
                echo ""
                echo "   Review rollback error:"
                tail -20 /tmp/migration-down.log
                echo ""
                exit 2
            fi
        else
            echo "     ⚠️  Forward migration failed (may need dependencies)"
            cat /tmp/migration-up.log
        fi

        # Restore to original version
        if [[ "${BEFORE_VERSION}" != "0" ]]; then
            bundle exec rails db:migrate VERSION="${BEFORE_VERSION}" 2>/dev/null || true
        fi
    else
        echo "     Rails not available - skipping deep validation"
    fi
fi

echo ""
echo "✅ Migration safety checks complete"
echo ""

if [[ ${#ISSUES[@]} -gt 0 ]]; then
    echo "💡 Review best practices: https://github.com/ankane/strong_migrations"
    echo ""
fi

exit 0
