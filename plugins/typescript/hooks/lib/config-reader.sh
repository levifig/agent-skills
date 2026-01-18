#!/bin/bash
# Configuration Reader Utilities
# Read hook-specific configuration from project

get_config_file() {
    # Find project configuration file
    # Returns: Path to config file or empty string

    local project_root
    project_root=$(git rev-parse --show-toplevel 2>/dev/null || echo ".")

    local config_paths=(
        "${project_root}/.agents/config.json"
        "${project_root}/.claude/config.json"
        "${project_root}/config.json"
    )

    for config_path in "${config_paths[@]}"; do
        if [[ -f "${config_path}" ]]; then
            echo "${config_path}"
            return 0
        fi
    done

    echo ""
    return 1
}

get_hook_config() {
    # Read hook-specific configuration
    # Args: $1 - hook name (e.g., "secret-detection")
    # Returns: JSON configuration for hook

    local hook_name="$1"
    local config_file
    config_file=$(get_config_file)

    if [[ -z "${config_file}" ]]; then
        echo "{}"
        return
    fi

    if command -v jq >/dev/null 2>&1; then
        jq -r ".hooks.\"${hook_name}\" // {}" "${config_file}" 2>/dev/null || echo "{}"
    else
        echo "{}"
    fi
}

is_hook_enabled() {
    # Check if a specific hook is enabled
    # Args: $1 - hook name
    # Returns: 0 if enabled, 1 if disabled

    local hook_name="$1"
    local config
    config=$(get_hook_config "${hook_name}")

    if [[ "${config}" == "{}" ]]; then
        # Default: enabled
        return 0
    fi

    if command -v jq >/dev/null 2>&1; then
        local enabled
        enabled=$(echo "${config}" | jq -r '.enabled // true')

        if [[ "${enabled}" == "true" ]]; then
            return 0
        else
            return 1
        fi
    else
        # Default: enabled
        return 0
    fi
}

get_timeout_threshold() {
    # Get timeout threshold for a hook
    # Args: $1 - hook name
    # Returns: Timeout in seconds (default: 300)

    local hook_name="$1"
    local config
    config=$(get_hook_config "${hook_name}")

    if command -v jq >/dev/null 2>&1; then
        echo "${config}" | jq -r '.timeout // 300'
    else
        echo "300"
    fi
}

get_validation_level() {
    # Get validation level for a hook
    # Args: $1 - hook name
    # Returns: quick|normal|thorough (default: normal)

    local hook_name="$1"
    local config
    config=$(get_hook_config "${hook_name}")

    if command -v jq >/dev/null 2>&1; then
        echo "${config}" | jq -r '.validation_level // "normal"'
    else
        echo "normal"
    fi
}

get_config_value() {
    # Get a specific configuration value
    # Args: $1 - JSON path (e.g., "project.name")
    # Returns: Configuration value or empty string

    local json_path="$1"
    local config_file
    config_file=$(get_config_file)

    if [[ -z "${config_file}" ]]; then
        echo ""
        return
    fi

    if command -v jq >/dev/null 2>&1; then
        jq -r ".${json_path} // empty" "${config_file}" 2>/dev/null || echo ""
    else
        echo ""
    fi
}

# Export functions
export -f get_config_file
export -f get_hook_config
export -f is_hook_enabled
export -f get_timeout_threshold
export -f get_validation_level
export -f get_config_value
