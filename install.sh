#!/usr/bin/env bash
#
# Universal Agent Skills Installer
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/levifig/agent-skills/main/install.sh | bash
#   ./install.sh [--update] [--target <target>] [--all]
#
set -euo pipefail

# Configuration
REPO_URL="https://github.com/levifig/agent-skills.git"
INSTALL_DIR="${HOME}/.local/share/agent-skills"
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

# ─────────────────────────────────────────────────────────────────────────────
# UI Components
# ─────────────────────────────────────────────────────────────────────────────

print_header() {
    clear
    echo ""
    echo -e "${PURPLE}  _   _       _                          _${RESET}"
    echo -e "${PURPLE} | | | |_ __ (_)_   _____ _ __ ___  __ _| |${RESET}"
    echo -e "${PURPLE} | | | |  _ \\| \\ \\ / / _ \\  __/ __|/ _\` | |${RESET}"
    echo -e "${PURPLE} | |_| | | | | |\\ V /  __/ |  \\__ \\ (_| | |${RESET}"
    echo -e "${PURPLE}  \\___/|_| |_|_| \\_/ \\___|_|  |___/\\__,_|_|${RESET}"
    echo ""
    echo -e "${MAGENTA}    _                    _     ____  _    _ _ _${RESET}"
    echo -e "${MAGENTA}   / \\   __ _  ___ _ __ | |_  / ___|| | _(_) | |___${RESET}"
    echo -e "${MAGENTA}  / _ \\ / _\` |/ _ \\  _ \\| __| \\___ \\| |/ / | | / __|${RESET}"
    echo -e "${MAGENTA} / ___ \\ (_| |  __/ | | | |_   ___) |   <| | | \\__ \\${RESET}"
    echo -e "${MAGENTA}/_/   \\_\\__, |\\___|_| |_|\\__| |____/|_|\\_\\_|_|_|___/${RESET}"
    echo -e "${MAGENTA}        |___/${RESET}"
    echo ""
    echo -e "${GRAY}  v${VERSION}${RESET}"
    echo ""
    echo -e "${GRAY}  ──────────────────────────────────────────────────${RESET}"
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

detect_tools() {
    TOOL_KEYS=()
    TOOL_NAMES=()
    TOOL_INSTALLED=()

    # Claude Code - detected separately (just needs marketplace add)
    HAS_CLAUDE_CODE=false
    if command -v claude &> /dev/null; then
        HAS_CLAUDE_CODE=true
    fi

    # OpenCode
    if [[ -d "${HOME}/.config/opencode" ]]; then
        TOOL_KEYS+=("opencode")
        TOOL_NAMES+=("OpenCode")
        if [[ -d "${HOME}/.config/opencode/skill/python" ]]; then
            TOOL_INSTALLED+=("yes")
        else
            TOOL_INSTALLED+=("no")
        fi
    fi

    # Cursor
    if [[ -d "${HOME}/.cursor" ]] || [[ -d "/Applications/Cursor.app" ]]; then
        TOOL_KEYS+=("cursor")
        TOOL_NAMES+=("Cursor")
        if [[ -f "${HOME}/.cursor/rules/python.mdc" ]]; then
            TOOL_INSTALLED+=("yes")
        else
            TOOL_INSTALLED+=("no")
        fi
    fi

    # Copilot - always available
    TOOL_KEYS+=("copilot")
    TOOL_NAMES+=("GitHub Copilot")
    TOOL_INSTALLED+=("no")
}

check_existing_installation() {
    [[ -d "${INSTALL_DIR}" ]] && [[ -f "${INSTALL_DIR}/.version" || -d "${INSTALL_DIR}/claude-code" ]]
}

# ─────────────────────────────────────────────────────────────────────────────
# Requirements
# ─────────────────────────────────────────────────────────────────────────────

check_requirements() {
    local missing=()
    local is_local_dev=false

    # Check if running from local dev repo (needs node/npm for building)
    local script_dir
    script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
    if [[ -d "${script_dir}/.git" ]] && [[ -f "${script_dir}/package.json" ]]; then
        is_local_dev=true
    fi

    # Git always required
    command -v git &> /dev/null || missing+=("git")

    # Node/npm only required for local dev (building)
    if [[ "$is_local_dev" == true ]]; then
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
    if [[ "$is_local_dev" == true ]]; then
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
        echo -en "    ${WHITE}▶${RESET} ${BOLD}${label}${RESET}${status}  ${GRAY}[Y/n]${RESET} "
        read -r response

        # Rewrite line with result
        echo -en "\033[A\033[2K"
        if [[ -z "$response" ]] || [[ "$response" =~ ^[Yy] ]]; then
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

# Detect if running from a local development repo
detect_local_repo() {
    local script_dir
    script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

    # Check if script is in a git repo with our expected structure
    if [[ -d "${script_dir}/.git" ]] && [[ -f "${script_dir}/package.json" ]] && [[ -d "${script_dir}/skills" ]]; then
        # Not running from the install dir itself
        if [[ "${script_dir}" != "${INSTALL_DIR}" ]]; then
            echo "${script_dir}"
            return 0
        fi
    fi
    return 1
}

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
    local local_repo
    local_repo=$(detect_local_repo) || true

    if [[ -n "$local_repo" ]]; then
        # Running from local development repo
        print_info "Using local development repo: ${local_repo}"

        # Build in the local repo first
        build_local "${local_repo}"

        # Sync only dist/ to cache
        rm -rf "${INSTALL_DIR}"
        mkdir -p "${INSTALL_DIR}"
        if command -v rsync &> /dev/null; then
            spinner "Syncing dist to cache" rsync -a "${local_repo}/dist/" "${INSTALL_DIR}/"
        else
            spinner "Copying dist to cache" cp -r "${local_repo}/dist/"* "${INSTALL_DIR}/"
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

        # Copy only dist/ to cache
        rm -rf "${INSTALL_DIR}"
        mkdir -p "${INSTALL_DIR}"
        cp -r "${temp_dir}/dist/"* "${INSTALL_DIR}/"

        # Store version hash for future update checks
        git -C "${temp_dir}" rev-parse HEAD > "${INSTALL_DIR}/.version"

        rm -rf "${temp_dir}"
    fi
}

show_claude_code_instructions() {
    echo ""
    echo -e "  ${CYAN}${BOLD}Claude Code${RESET}"
    echo ""
    print_info "Add the marketplace in Claude Code:"
    echo ""
    echo -e "    ${WHITE}/plugin marketplace add github:levifig/agent-skills/dist/claude-code${RESET}"
    echo ""
    print_info "Then browse and install plugins via ${WHITE}/plugin${RESET}"
}

install_opencode() {
    local dist="${INSTALL_DIR}/opencode"
    local config="${HOME}/.config/opencode"

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

install_cursor() {
    local dist="${INSTALL_DIR}/cursor"
    print_success "Cursor rules ready"
    print_info "Copy to your project:"
    print_info "  cp -r ${dist}/.cursor/rules <project>/.cursor/"
}

install_copilot() {
    local dist="${INSTALL_DIR}/copilot"
    print_success "Copilot instructions ready"
    print_info "Copy to your repository:"
    print_info "  cp ${dist}/.github/copilot-instructions.md <repo>/.github/"
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
                exit 0
                ;;
            *) shift ;;
        esac
    done

    print_header

    # Step 1: Check requirements
    print_step "1" "Checking requirements"
    check_requirements
    echo ""

    # Step 2: Detect tools
    print_step "2" "Detecting AI tools"
    detect_tools
    echo ""

    # Show Claude Code first if detected
    if [[ "$HAS_CLAUDE_CODE" == true ]]; then
        echo -e "  ${GREEN}✓${RESET} ${BOLD}Claude Code${RESET} detected"
        echo ""
        print_info "Add the marketplace in Claude Code:"
        echo ""
        echo -e "    ${WHITE}/plugin marketplace add github:levifig/agent-skills/dist/claude-code${RESET}"
        echo ""
        print_info "Then browse and install plugins via ${WHITE}/plugin${RESET}"
        echo ""
    fi

    # Step 3: Select other targets
    print_step "3" "Other targets"

    if [[ -n "$specific_target" ]]; then
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
    if check_existing_installation && [[ "$force_fresh" != true ]]; then
        if ! prompt_update; then
            print_info "Removing existing installation..."
            rm -rf "${INSTALL_DIR}"
        fi
        echo ""
    fi

    # Step 4: Fetch and build
    print_step "4" "Fetching agent-skills"
    clone_or_update
    echo ""

    # Step 5: Install selected targets
    if [[ ${#SELECTED_TARGETS[@]} -gt 0 ]]; then
        print_step "5" "Installing"
        echo ""
        for target in "${SELECTED_TARGETS[@]}"; do
            case $target in
                opencode) install_opencode ;;
                cursor) install_cursor ;;
                copilot) install_copilot ;;
            esac
        done
    fi

    # Done
    echo ""
    echo -e "${GRAY}  ──────────────────────────────────────────────────${RESET}"
    echo ""
    echo -e "  ${GREEN}${BOLD}✓ Installation complete!${RESET}"
    echo ""
    print_info "Update: curl -fsSL https://raw.githubusercontent.com/levifig/agent-skills/main/install.sh | bash"
    echo ""
}

main "$@"
