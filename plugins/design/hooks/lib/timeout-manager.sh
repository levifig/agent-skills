#!/bin/bash
# Timeout Management Utilities
# Track and manage execution timeouts

# Global timeout tracking
_TIMEOUT_START_TIME=""
_TIMEOUT_LIMIT=""

start_timeout_tracker() {
    # Begin timing with specified limit
    # Args: $1 - timeout in seconds

    local timeout_seconds="${1:-300}"
    _TIMEOUT_START_TIME=$(date +%s)
    _TIMEOUT_LIMIT="${timeout_seconds}"
}

get_elapsed_time() {
    # Get elapsed time since timeout tracker started
    # Returns: Elapsed seconds

    if [[ -z "${_TIMEOUT_START_TIME}" ]]; then
        echo "0"
        return
    fi

    local current_time
    current_time=$(date +%s)
    local elapsed=$((current_time - _TIMEOUT_START_TIME))
    echo "${elapsed}"
}

get_remaining_time() {
    # Get remaining time before timeout
    # Returns: Remaining seconds

    if [[ -z "${_TIMEOUT_LIMIT}" ]] || [[ -z "${_TIMEOUT_START_TIME}" ]]; then
        echo "0"
        return
    fi

    local elapsed
    elapsed=$(get_elapsed_time)
    local remaining=$((_TIMEOUT_LIMIT - elapsed))

    if [[ ${remaining} -lt 0 ]]; then
        echo "0"
    else
        echo "${remaining}"
    fi
}

check_remaining_time() {
    # Check if sufficient time remains (at least 30 seconds)
    # Returns: 0 if time remains, 1 if approaching timeout

    local remaining
    remaining=$(get_remaining_time)

    if [[ ${remaining} -gt 30 ]]; then
        return 0
    else
        return 1
    fi
}

graceful_timeout() {
    # Handle timeout gracefully with message
    # Args: $1 - operation name

    local operation="${1:-operation}"
    local elapsed
    elapsed=$(get_elapsed_time)

    echo ""
    echo "⏱️  Timeout: ${operation} exceeded ${_TIMEOUT_LIMIT}s limit (${elapsed}s elapsed)"
    echo "   Stopping gracefully to preserve remaining time budget."
}

format_duration() {
    # Format duration in human-readable format
    # Args: $1 - duration in seconds
    # Returns: Formatted string (e.g., "2m 30s")

    local seconds="$1"
    local minutes=$((seconds / 60))
    local remaining_seconds=$((seconds % 60))

    if [[ ${minutes} -gt 0 ]]; then
        echo "${minutes}m ${remaining_seconds}s"
    else
        echo "${seconds}s"
    fi
}

# Export functions
export -f start_timeout_tracker
export -f get_elapsed_time
export -f get_remaining_time
export -f check_remaining_time
export -f graceful_timeout
export -f format_duration
