#!/bin/bash
# Agent Detection Utilities
# Detect agent type and determine validation depth

get_agent_type() {
    # Get AGENT_TYPE from environment or detect from context
    # Returns: Agent type string (orchestrator, backend-dev, etc.) or "main"

    if [[ -n "${AGENT_TYPE:-}" ]]; then
        echo "${AGENT_TYPE}"
    else
        echo "main"
    fi
}

is_subagent() {
    # Check if running as a subagent (not main Claude Code instance)
    # Returns: 0 if subagent, 1 if main agent

    local agent_type
    agent_type=$(get_agent_type)

    if [[ "${agent_type}" == "main" ]]; then
        return 1
    else
        return 0
    fi
}

should_run_deep_check() {
    # Determine if deep validation should run
    # Deep checks: Type checking, full test suite, etc.
    # Returns: 0 if should run deep checks, 1 otherwise

    local agent_type
    agent_type=$(get_agent_type)

    # Main agent or specific agents run deep checks
    case "${agent_type}" in
        main|backend-dev|frontend-dev|testing-qa)
            return 0
            ;;
        *)
            return 1
            ;;
    esac
}

get_validation_level() {
    # Get validation depth level
    # Returns: quick|normal|thorough

    local agent_type
    agent_type=$(get_agent_type)

    case "${agent_type}" in
        main)
            echo "thorough"
            ;;
        backend-dev|frontend-dev|testing-qa)
            echo "normal"
            ;;
        orchestrator|product)
            echo "quick"
            ;;
        *)
            echo "normal"
            ;;
    esac
}

get_timeout_budget() {
    # Get timeout budget in seconds based on agent type
    # Returns: Timeout in seconds

    local agent_type
    agent_type=$(get_agent_type)

    case "${agent_type}" in
        testing-qa)
            echo "600"  # 10 minutes for testing agent
            ;;
        *)
            echo "300"  # 5 minutes for others
            ;;
    esac
}

# Export functions
export -f get_agent_type
export -f is_subagent
export -f should_run_deep_check
export -f get_validation_level
export -f get_timeout_budget
