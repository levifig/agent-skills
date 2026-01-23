#!/bin/bash
# JSON Parser Utilities
# Extract fields from Claude Code hook JSON input

parse_file_path() {
    # Extract file_path from hook JSON input
    # Args: $1 - JSON string
    # Returns: File path or empty string

    local json="$1"

    # Try to parse JSON with jq if available
    if command -v jq >/dev/null 2>&1; then
        echo "${json}" | jq -r '.file_path // empty' 2>/dev/null || echo ""
    else
        # Fallback to grep/sed
        echo "${json}" | grep -o '"file_path"[[:space:]]*:[[:space:]]*"[^"]*"' | sed 's/.*"\([^"]*\)".*/\1/' || echo ""
    fi
}

parse_tool_name() {
    # Extract tool_name from hook JSON input
    # Args: $1 - JSON string
    # Returns: Tool name or empty string

    local json="$1"

    if command -v jq >/dev/null 2>&1; then
        echo "${json}" | jq -r '.tool_name // empty' 2>/dev/null || echo ""
    else
        echo "${json}" | grep -o '"tool_name"[[:space:]]*:[[:space:]]*"[^"]*"' | sed 's/.*"\([^"]*\)".*/\1/' || echo ""
    fi
}

parse_agent_type() {
    # Extract agent_type from hook JSON input (if present)
    # Args: $1 - JSON string
    # Returns: Agent type or empty string

    local json="$1"

    if command -v jq >/dev/null 2>&1; then
        echo "${json}" | jq -r '.agent_type // empty' 2>/dev/null || echo ""
    else
        echo "${json}" | grep -o '"agent_type"[[:space:]]*:[[:space:]]*"[^"]*"' | sed 's/.*"\([^"]*\)".*/\1/' || echo ""
    fi
}

parse_parameters() {
    # Extract parameters object from hook JSON input
    # Args: $1 - JSON string
    # Returns: Parameters JSON object or empty string

    local json="$1"

    if command -v jq >/dev/null 2>&1; then
        echo "${json}" | jq -r '.parameters // empty' 2>/dev/null || echo ""
    else
        echo ""
    fi
}

# Export functions
export -f parse_file_path
export -f parse_tool_name
export -f parse_agent_type
export -f parse_parameters
