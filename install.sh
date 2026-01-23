#!/usr/bin/env bash
#
# Loaf - Levi's Opinionated Agentic Framework Installer
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/levifig/loaf/main/install.sh | bash
#   ./install.sh [--update] [--target <target>] [--all]
#
set -euo pipefail

# Configuration
REPO_URL="https://github.com/levifig/loaf.git"
INSTALL_DIR="${HOME}/.local/share/loaf"
VERSION="1.0.0"

# Colors (256-color)
BOLD='\033[1m'
DIM='\033[2m'
RESET='\033[0m'
RED='\033[38;5;196m'
GREEN='\033[38;5;82m'
YELLOW='\033[38;5;220m'
BLUE='\033[38;5;39m'
MAGENTA='\033[38;5;135m'
CYAN='\033[38;5;51m'
PURPLE='\033[38;5;99m'
GRAY='\033[38;5;245m'
WHITE='\033[38;5;255m'
ORANGE='\033[38;5;208m'

# ─────────────────────────────────────────────────────────────────────────────
# UI Components
# ─────────────────────────────────────────────────────────────────────────────

print_header() {
    clear
    echo ""
    echo -e "   \033[38;5;208m█░░\033[0m \033[38;5;214m█▀█\033[0m \033[38;5;220m▄▀█\033[0m \033[38;5;226m█▀▀\033[0m"
    echo -e "   \033[38;5;208m█▄▄\033[0m \033[38;5;214m█▄█\033[0m \033[38;5;220m█▀█\033[0m \033[38;5;226m█▀░\033[0m"
    echo ""
    echo -e "   ${GRAY}Levi's Opinionated Agentic Framework${RESET}  ${GRAY}v${VERSION}${RESET}"
    echo -e "   ${GRAY}\"Why have just a slice when you can get the whole loaf?\"${RESET}"
    echo ""
    echo -e "${GRAY}   ──────────────────────────────────────────────────${RESET}"
    echo ""
}

print_dev_banner() {
    echo -e "  ${ORANGE}${BOLD}◆ DEVELOPMENT MODE${RESET}"
    echo ""
    print_info "Running from local clone - will build from source"
    echo ""
}

print_step() {
    echo -e "  ${PURPLE}${BOLD}[$1]${RESET} $2"
}

print_success() {
    echo -e "  ${GREEN}✓${RESET} $1"
}

print_error() {
    echo -e "  ${RED}✗${RESET} $1" >&2
}

print_info() {
    echo -e "    ${GRAY}$1${RESET}"
}

print_warn() {
    echo -e "  ${YELLOW}⚡${RESET} $1"
}

spinner() {
    local msg="$1"
    shift
    echo -en "    ${GRAY}$msg...${RESET} "
    if "$@" > /dev/null 2>&1; then
        echo -e "${GREEN}done${RESET}"
    else
        echo -e "${RED}failed${RESET}"
        return 1
    fi
}

# ─────────────────────────────────────────────────────────────────────────────
# Detection
# ─────────────────────────────────────────────────────────────────────────────

declare -a TOOL_KEYS=()
declare -a TOOL_NAMES=()
declare -a TOOL_INSTALLED=()

# Config directory resolution
# Use XDG only if: directory exists AND tool's config env var is set
declare -A TOOL_CONFIG_DIRS=()

# Get config directory for a tool
# Usage: get_config_dir "cursor" "CURSOR_CONFIG_DIR" -> returns path
get_config_dir() {
    local tool="$1"
    local config_env_var="$2"
    local xdg_dir="${XDG_CONFIG_HOME:-${HOME}/.config}/${tool}"
    local dotfile_dir="${HOME}/.${tool}"

    # Use XDG only if BOTH: directory exists AND config env var is set
    if [[ -d "${xdg_dir}" ]] && [[ -n "${!config_env_var:-}" ]]; then
        echo "${xdg_dir}"
    else
        # Default to dotfile directory
        echo "${dotfile_dir}"
    fi
}

# Check if running from a local development repo
IS_DEV_MODE=false
LOCAL_REPO_PATH=""

detect_dev_mode() {
    local script_dir
    script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

    # Check if script is in a git repo with our expected structure
    if [[ -d "${script_dir}/.git" ]] && [[ -f "${script_dir}/package.json" ]] && [[ -d "${script_dir}/src/skills" ]]; then
        # Not running from the install dir itself
        if [[ "${script_dir}" != "${INSTALL_DIR}" ]]; then
            IS_DEV_MODE=true
            LOCAL_REPO_PATH="${script_dir}"
        fi
    fi
}

detect_tools() {
    TOOL_KEYS=()
    TOOL_NAMES=()
    TOOL_INSTALLED=()
    TOOL_CONFIG_DIRS=()

    # Claude Code - detected separately (just needs marketplace add)
    HAS_CLAUDE_CODE=false
    if command -v claude &> /dev/null; then
        HAS_CLAUDE_CODE=true
    fi

    # OpenCode (always uses XDG)
    local opencode_config="${XDG_CONFIG_HOME:-${HOME}/.config}/opencode"
    if [[ -d "${opencode_config}" ]]; then
        TOOL_KEYS+=("opencode")
        TOOL_NAMES+=("OpenCode")
        TOOL_CONFIG_DIRS[opencode]="${opencode_config}"
        if [[ -d "${opencode_config}/skill/python" ]]; then
            TOOL_INSTALLED+=("yes")
        else
            TOOL_INSTALLED+=("no")
        fi
    fi

    # Codex (XDG if dir exists AND CODEX_HOME set, else dotfile)
    local codex_config
    codex_config=$(get_config_dir "codex" "CODEX_HOME")
    if command -v codex &> /dev/null || [[ -d "${codex_config}" ]] || [[ -d "${HOME}/.codex" ]]; then
        TOOL_KEYS+=("codex")
        TOOL_NAMES+=("Codex")
        TOOL_CONFIG_DIRS[codex]="${codex_config}"
        if [[ -L "${codex_config}/skills/python" ]] || [[ -L "${HOME}/.codex/skills/python" ]]; then
            TOOL_INSTALLED+=("yes")
        else
            TOOL_INSTALLED+=("no")
        fi
    fi

    # Copilot (XDG if dir exists AND COPILOT_CONFIG_DIR set, else dotfile)
    local copilot_config
    copilot_config=$(get_config_dir "copilot" "COPILOT_CONFIG_DIR")
    TOOL_KEYS+=("copilot")
    TOOL_NAMES+=("GitHub Copilot")
    TOOL_CONFIG_DIRS[copilot]="${copilot_config}"
    if [[ -L "${copilot_config}/skills/python" ]] || [[ -L "${HOME}/.copilot/skills/python" ]]; then
        TOOL_INSTALLED+=("yes")
    else
        TOOL_INSTALLED+=("no")
    fi

    # Gemini - always available (project-based)
    TOOL_KEYS+=("gemini")
    TOOL_NAMES+=("Gemini")
    TOOL_INSTALLED+=("no")
}

check_existing_installation() {
    [[ -d "${INSTALL_DIR}" ]] && [[ -f "${INSTALL_DIR}/.version" || -d "${INSTALL_DIR}/opencode" || -d "${INSTALL_DIR}/standard" || -d "${INSTALL_DIR}/copilot" || -d "${INSTALL_DIR}/gemini" ]]
}

# ─────────────────────────────────────────────────────────────────────────────
# Requirements
# ─────────────────────────────────────────────────────────────────────────────

check_requirements() {
    local missing=()

    # Git always required
    command -v git &> /dev/null || missing+=("git")

    # Node/npm only required for local dev (building)
    if [[ "$IS_DEV_MODE" == true ]]; then
        command -v node &> /dev/null || missing+=("node")
        command -v npm &> /dev/null || missing+=("npm")
    fi

    if [[ ${#missing[@]} -gt 0 ]]; then
        print_error "Missing required tools: ${missing[*]}"
        echo ""
        print_info "Please install them before continuing."
        exit 1
    fi

    # Check node version only if node is required
    if [[ "$IS_DEV_MODE" == true ]]; then
        local node_version
        node_version=$(node -v | sed 's/v//' | cut -d. -f1)
        if [[ "$node_version" -lt 18 ]]; then
            print_error "Node.js 18+ required (found: $(node -v))"
            exit 1
        fi
        print_success "All requirements met (git, node 18+, npm)"
    else
        print_success "All requirements met (git)"
    fi
}

# ─────────────────────────────────────────────────────────────────────────────
# Multi-select UI
# ─────────────────────────────────────────────────────────────────────────────

select_targets_interactive() {
    SELECTED_TARGETS=()

    if [[ ${#TOOL_KEYS[@]} -eq 0 ]]; then
        print_info "No other tools detected"
        return
    fi

    local count=${#TOOL_KEYS[@]}

    echo ""
    echo -e "  ${BOLD}Select targets to install:${RESET}"
    echo ""

    # First pass: show all targets faded (pending)
    for ((i = 0; i < count; i++)); do
        local label="${TOOL_NAMES[$i]}"
        local status=""
        if [[ "${TOOL_INSTALLED[$i]}" == "yes" ]]; then
            status=" ${YELLOW}(installed)${RESET}"
        fi
        echo -e "    ${GRAY}○ ${label}${status}${RESET}"
    done

    # Move cursor back up to first target
    echo -en "\033[${count}A"

    # Second pass: prompt for each target
    for ((i = 0; i < count; i++)); do
        local label="${TOOL_NAMES[$i]}"
        local status=""
        if [[ "${TOOL_INSTALLED[$i]}" == "yes" ]]; then
            status=" ${YELLOW}(installed)${RESET}"
        fi

        # Highlight current line with prompt
        echo -en "\033[2K"
        echo -en "    ${WHITE}▶${RESET} ${BOLD}${label}${RESET}${status}  ${GRAY}[y/N]${RESET} "
        read -r response

        # Rewrite line with result
        echo -en "\033[A\033[2K"
        if [[ "$response" =~ ^[Yy] ]]; then
            SELECTED_TARGETS+=("${TOOL_KEYS[$i]}")
            echo -e "    ${GREEN}✓${RESET} ${label}${status}"
        else
            echo -e "    ${GRAY}○ ${label}${status}${RESET}"
        fi
    done

    echo ""
}

# ─────────────────────────────────────────────────────────────────────────────
# Installation Steps
# ─────────────────────────────────────────────────────────────────────────────

# Build in local dev repo before syncing
build_local() {
    local repo_dir="$1"
    cd "${repo_dir}"

    if [[ ! -d "node_modules" ]] || [[ ! -f "node_modules/.package-lock.json" ]]; then
        spinner "Installing dependencies" npm install --silent
    fi

    spinner "Building distributions" npm run build --silent
    cd - > /dev/null
}

clone_or_update() {
    if [[ "$IS_DEV_MODE" == true ]]; then
        # Running from local development repo
        print_info "Building from: ${LOCAL_REPO_PATH}"

        # Build in the local repo first
        build_local "${LOCAL_REPO_PATH}"

        # Sync only dist/ to cache (not plugins/ - those stay at root for Claude Code)
        rm -rf "${INSTALL_DIR}"
        mkdir -p "${INSTALL_DIR}"
        if command -v rsync &> /dev/null; then
            spinner "Syncing dist to cache" rsync -a "${LOCAL_REPO_PATH}/dist/" "${INSTALL_DIR}/"
        else
            spinner "Copying dist to cache" cp -r "${LOCAL_REPO_PATH}/dist/"* "${INSTALL_DIR}/"
        fi
    else
        # Remote install - clone to temp, copy dist/ to cache
        local temp_dir
        temp_dir=$(mktemp -d)

        if [[ -d "${INSTALL_DIR}" ]]; then
            # Check if update needed by comparing with remote
            spinner "Checking for updates" git clone --depth 1 "${REPO_URL}" "${temp_dir}"

            local remote_hash
            remote_hash=$(git -C "${temp_dir}" rev-parse HEAD)

            if [[ -f "${INSTALL_DIR}/.version" ]] && [[ "$(cat "${INSTALL_DIR}/.version")" == "$remote_hash" ]]; then
                print_info "Already up to date"
                rm -rf "${temp_dir}"
                return 0
            fi
        else
            spinner "Cloning repository" git clone --depth 1 "${REPO_URL}" "${temp_dir}"
        fi

        # Copy only dist/ to cache (for OpenCode, Cursor, Copilot, Codex)
        rm -rf "${INSTALL_DIR}"
        mkdir -p "${INSTALL_DIR}"
        cp -r "${temp_dir}/dist/"* "${INSTALL_DIR}/"

        # Store version hash for future update checks
        git -C "${temp_dir}" rev-parse HEAD > "${INSTALL_DIR}/.version"

        rm -rf "${temp_dir}"
    fi
}

show_claude_code_dev_instructions() {
    echo -e "  ${GREEN}✓${RESET} ${BOLD}Claude Code${RESET} detected"
    echo ""
    print_info "For development, test your local marketplace:"
    echo ""
    echo -e "    ${WHITE}/plugin marketplace add ${LOCAL_REPO_PATH}${RESET}"
    echo ""
    print_info "This uses plugins/ built at repo root."
    print_info "For production, users will use:"
    echo ""
    echo -e "    ${GRAY}/plugin marketplace add levifig/loaf${RESET}"
    echo ""
}

show_claude_code_instructions() {
    echo -e "  ${GREEN}✓${RESET} ${BOLD}Claude Code${RESET} detected"
    echo ""
    print_info "Add the marketplace in Claude Code:"
    echo ""
    echo -e "    ${WHITE}/plugin marketplace add levifig/loaf${RESET}"
    echo ""
    print_info "Then browse and install plugins via ${WHITE}/plugin${RESET}"
    echo ""
}

install_opencode() {
    local dist="${INSTALL_DIR}/opencode"
    local config="${TOOL_CONFIG_DIRS[opencode]}"

    mkdir -p "${config}"/{skill,agent,command,plugin}

    if command -v rsync &> /dev/null; then
        rsync -a --delete "${dist}/skill/" "${config}/skill/"
        rsync -a --delete "${dist}/agent/" "${config}/agent/"
        rsync -a --delete "${dist}/command/" "${config}/command/"
        rsync -a --delete "${dist}/plugin/" "${config}/plugin/"
    else
        cp -r "${dist}/skill/"* "${config}/skill/" 2>/dev/null || true
        cp -r "${dist}/agent/"* "${config}/agent/" 2>/dev/null || true
        cp -r "${dist}/command/"* "${config}/command/" 2>/dev/null || true
        cp -r "${dist}/plugin/"* "${config}/plugin/" 2>/dev/null || true
    fi

    print_success "OpenCode installed to ${config}"
}

# Helper: symlink individual skills from standard to target directory
symlink_standard_skills() {
    local target_dir="$1"
    local source_dir="${INSTALL_DIR}/standard/skills"

    mkdir -p "${target_dir}"

    # Create symlink for each skill
    for skill in "${source_dir}"/*/; do
        local skill_name
        skill_name=$(basename "${skill}")
        local target_link="${target_dir}/${skill_name}"

        # Remove existing symlink or directory
        if [[ -L "${target_link}" ]] || [[ -d "${target_link}" ]]; then
            rm -rf "${target_link}"
        fi

        ln -s "${skill}" "${target_link}"
    done
}

install_codex() {
    local config="${TOOL_CONFIG_DIRS[codex]}/skills"
    symlink_standard_skills "${config}"
    print_success "Codex skills symlinked to ${config}"
}

install_copilot() {
    local config="${TOOL_CONFIG_DIRS[copilot]}/skills"
    symlink_standard_skills "${config}"
    print_success "Copilot skills symlinked to ${config}"
}

install_gemini() {
    local dist="${INSTALL_DIR}/standard/skills"
    print_success "Gemini skills ready (uses standard format)"
    print_info "In your project directory, symlink each skill:"
    echo ""
    echo -e "    ${WHITE}mkdir -p .gemini/skills${RESET}"
    echo -e "    ${WHITE}for skill in ${dist}/*/; do${RESET}"
    echo -e "    ${WHITE}  ln -s \"\$skill\" .gemini/skills/\$(basename \"\$skill\")${RESET}"
    echo -e "    ${WHITE}done${RESET}"
    echo ""
}

prompt_update() {
    echo ""
    print_warn "Existing installation detected"
    echo ""
    echo -en "  ${WHITE}Update to latest version? [Y/n]${RESET} "
    read -r response
    if [[ "$response" =~ ^[Nn] ]]; then
        return 1
    fi
    return 0
}

show_dev_completion() {
    echo ""
    echo -e "${GRAY}  ──────────────────────────────────────────────────${RESET}"
    echo ""
    echo -e "  ${GREEN}${BOLD}✓ Development build complete!${RESET}"
    echo ""
    echo -e "  ${BOLD}What was built:${RESET}"
    print_info "• Claude Code: plugins/ at repo root"
    print_info "• Others: dist/ synced to ~/.local/share/loaf/"
    echo ""
    echo -e "  ${BOLD}Test your changes:${RESET}"
    print_info "Claude Code:"
    echo -e "    ${WHITE}/plugin marketplace add ${LOCAL_REPO_PATH}${RESET}"
    echo ""
    print_info "Rebuild after changes:"
    echo -e "    ${WHITE}npm run build${RESET}"
    echo ""
}

show_completion() {
    echo ""
    echo -e "${GRAY}  ──────────────────────────────────────────────────${RESET}"
    echo ""
    echo -e "  ${GREEN}${BOLD}✓ Installation complete!${RESET}"
    echo ""
    print_info "Update: curl -fsSL https://raw.githubusercontent.com/levifig/loaf/main/install.sh | bash"
    echo ""
}

# ─────────────────────────────────────────────────────────────────────────────
# Main
# ─────────────────────────────────────────────────────────────────────────────

main() {
    local specific_target=""
    local install_all=false
    local force_fresh=false

    while [[ $# -gt 0 ]]; do
        case $1 in
            --update) shift ;;
            --fresh) force_fresh=true; shift ;;
            --target) specific_target="$2"; shift 2 ;;
            --all) install_all=true; shift ;;
            --help|-h)
                echo "Usage: install.sh [options]"
                echo ""
                echo "Options:"
                echo "  --update    Update existing installation"
                echo "  --fresh     Force fresh install (remove existing)"
                echo "  --target X  Install specific target only"
                echo "  --all       Install all detected targets"
                echo "  --help      Show this help"
                echo ""
                echo "Development:"
                echo "  Run from a cloned repo to build from source."
                echo "  Requires: git, node 18+, npm"
                exit 0
                ;;
            *) shift ;;
        esac
    done

    # Detect development mode early
    detect_dev_mode

    print_header

    # Show dev banner if applicable
    if [[ "$IS_DEV_MODE" == true ]]; then
        print_dev_banner
    fi

    # Step 1: Check requirements
    print_step "1" "Checking requirements"
    check_requirements
    echo ""

    # Step 2: Detect tools
    print_step "2" "Detecting AI tools"
    detect_tools
    echo ""

    # Show Claude Code instructions (dev vs production)
    if [[ "$HAS_CLAUDE_CODE" == true ]]; then
        if [[ "$IS_DEV_MODE" == true ]]; then
            show_claude_code_dev_instructions
        else
            show_claude_code_instructions
        fi
    fi

    # Step 3: Select other targets
    print_step "3" "Other targets"

    if [[ -n "$specific_target" ]]; then
        local valid_target=false
        for target in "${TOOL_KEYS[@]}"; do
            if [[ "$target" == "$specific_target" ]]; then
                valid_target=true
                break
            fi
        done
        if [[ "$valid_target" == false ]]; then
            print_error "Unknown target: ${specific_target}"
            exit 1
        fi
        SELECTED_TARGETS=("$specific_target")
        print_info "Target: $specific_target"
        echo ""
    elif [[ "$install_all" == true ]]; then
        SELECTED_TARGETS=("${TOOL_KEYS[@]}")
        print_info "Installing all detected targets"
        echo ""
    else
        select_targets_interactive
    fi

    # Check for existing installation
    if check_existing_installation; then
        if [[ "$force_fresh" == true ]]; then
            print_info "Removing existing installation..."
            rm -rf "${INSTALL_DIR}"
        else
            if ! prompt_update; then
                print_info "Keeping existing installation."
                return 0
            fi
        fi
        echo ""
    fi

    # Step 4: Fetch and build
    if [[ "$IS_DEV_MODE" == true ]]; then
        print_step "4" "Building from source"
    else
        print_step "4" "Fetching loaf"
    fi
    clone_or_update
    echo ""

    # Step 5: Install selected targets
    if [[ ${#SELECTED_TARGETS[@]} -gt 0 ]]; then
        print_step "5" "Installing"
        echo ""
        for target in "${SELECTED_TARGETS[@]}"; do
            case $target in
                opencode) install_opencode ;;
                codex) install_codex ;;
                copilot) install_copilot ;;
                gemini) install_gemini ;;
            esac
        done
    fi

    # Done - show appropriate completion message
    if [[ "$IS_DEV_MODE" == true ]]; then
        show_dev_completion
    else
        show_completion
    fi
}

main "$@"
