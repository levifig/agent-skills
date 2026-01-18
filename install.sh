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

# Colors
BOLD='\033[1m'
DIM='\033[2m'
ITALIC='\033[3m'
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
MAGENTA='\033[0;35m'
CYAN='\033[0;36m'
WHITE='\033[0;37m'
NC='\033[0m'

# Check if gum is available
HAS_GUM=false
if command -v gum &> /dev/null; then
    HAS_GUM=true
fi

# ─────────────────────────────────────────────────────────────────────────────
# UI Components
# ─────────────────────────────────────────────────────────────────────────────

print_header() {
    clear
    if [[ "$HAS_GUM" == true ]]; then
        gum style \
            --border rounded \
            --border-foreground 99 \
            --padding "1 3" \
            --margin "1 0" \
            --align center \
            "$(gum style --foreground 99 --bold 'Universal Agent Skills')" \
            "$(gum style --foreground 240 "v${VERSION}")"
    else
        echo ""
        echo -e "${MAGENTA}╭─────────────────────────────────────╮${NC}"
        echo -e "${MAGENTA}│${NC}                                     ${MAGENTA}│${NC}"
        echo -e "${MAGENTA}│${NC}   ${BOLD}${CYAN}Universal Agent Skills${NC}            ${MAGENTA}│${NC}"
        echo -e "${MAGENTA}│${NC}   ${DIM}v${VERSION}${NC}                              ${MAGENTA}│${NC}"
        echo -e "${MAGENTA}│${NC}                                     ${MAGENTA}│${NC}"
        echo -e "${MAGENTA}╰─────────────────────────────────────╯${NC}"
        echo ""
    fi
}

print_step() {
    local step="$1"
    local desc="$2"
    if [[ "$HAS_GUM" == true ]]; then
        gum style --foreground 99 --bold "[$step]" --margin "0 1"
        gum style --foreground 255 "$desc"
    else
        echo -e "${MAGENTA}${BOLD}[$step]${NC} $desc"
    fi
}

print_success() {
    if [[ "$HAS_GUM" == true ]]; then
        gum style --foreground 82 "✓ $1"
    else
        echo -e "${GREEN}✓${NC} $1"
    fi
}

print_error() {
    if [[ "$HAS_GUM" == true ]]; then
        gum style --foreground 196 "✗ $1"
    else
        echo -e "${RED}✗${NC} $1" >&2
    fi
}

print_info() {
    if [[ "$HAS_GUM" == true ]]; then
        gum style --foreground 244 "  $1"
    else
        echo -e "${DIM}  $1${NC}"
    fi
}

spinner() {
    local msg="$1"
    shift
    if [[ "$HAS_GUM" == true ]]; then
        gum spin --spinner dot --title "$msg" -- "$@"
    else
        echo -en "${DIM}$msg...${NC} "
        if "$@" > /dev/null 2>&1; then
            echo -e "${GREEN}done${NC}"
        else
            echo -e "${RED}failed${NC}"
            return 1
        fi
    fi
}

# ─────────────────────────────────────────────────────────────────────────────
# Detection
# ─────────────────────────────────────────────────────────────────────────────

declare -A DETECTED_TOOLS

detect_tools() {
    DETECTED_TOOLS=()

    # Claude Code
    if command -v claude &> /dev/null; then
        DETECTED_TOOLS["claude-code"]="Claude Code (CLI detected)"
    fi

    # OpenCode
    if [[ -d "${HOME}/.config/opencode" ]]; then
        DETECTED_TOOLS["opencode"]="OpenCode (config found)"
    fi

    # Cursor
    if [[ -d "${HOME}/.cursor" ]] || [[ -d "/Applications/Cursor.app" ]]; then
        DETECTED_TOOLS["cursor"]="Cursor (app detected)"
    fi

    # Copilot - always available
    DETECTED_TOOLS["copilot"]="GitHub Copilot (project-level)"
}

# ─────────────────────────────────────────────────────────────────────────────
# Requirements
# ─────────────────────────────────────────────────────────────────────────────

check_requirements() {
    local missing=()

    command -v git &> /dev/null || missing+=("git")
    command -v node &> /dev/null || missing+=("node")
    command -v npm &> /dev/null || missing+=("npm")

    if [[ ${#missing[@]} -gt 0 ]]; then
        print_error "Missing required tools: ${missing[*]}"
        echo ""
        echo "Please install them before continuing."
        exit 1
    fi

    local node_version
    node_version=$(node -v | sed 's/v//' | cut -d. -f1)
    if [[ "$node_version" -lt 18 ]]; then
        print_error "Node.js 18+ required (found: $(node -v))"
        exit 1
    fi
}

# ─────────────────────────────────────────────────────────────────────────────
# Installation Steps
# ─────────────────────────────────────────────────────────────────────────────

clone_or_update() {
    if [[ -d "${INSTALL_DIR}/.git" ]]; then
        spinner "Updating repository" git -C "${INSTALL_DIR}" pull --ff-only
    else
        rm -rf "${INSTALL_DIR}"
        mkdir -p "$(dirname "${INSTALL_DIR}")"
        spinner "Cloning repository" git clone --depth 1 "${REPO_URL}" "${INSTALL_DIR}"
    fi
}

build_targets() {
    local targets=("$@")

    cd "${INSTALL_DIR}"

    if [[ ! -d "node_modules" ]]; then
        spinner "Installing dependencies" npm install --silent
    fi

    for target in "${targets[@]}"; do
        spinner "Building ${target}" npm run "build:${target}" --silent
    done

    cd - > /dev/null
}

install_claude_code() {
    local dist="${INSTALL_DIR}/dist/claude-code"
    print_success "Claude Code plugins ready"
    print_info "Run in Claude Code:"
    print_info "  /plugin marketplace add ${dist}"
    print_info "  /plugin install orchestration@levifig"
}

install_opencode() {
    local dist="${INSTALL_DIR}/dist/opencode"
    local config="${HOME}/.config/opencode"

    mkdir -p "${config}"/{skill,agent,command,plugin}

    cp -r "${dist}/skill/"* "${config}/skill/" 2>/dev/null || true
    cp -r "${dist}/agent/"* "${config}/agent/" 2>/dev/null || true
    cp -r "${dist}/command/"* "${config}/command/" 2>/dev/null || true
    cp -r "${dist}/plugin/"* "${config}/plugin/" 2>/dev/null || true

    print_success "OpenCode installed to ${config}"
}

install_cursor() {
    local dist="${INSTALL_DIR}/dist/cursor"
    print_success "Cursor rules ready"
    print_info "Copy to your project:"
    print_info "  cp -r ${dist}/.cursor/rules <project>/.cursor/"
}

install_copilot() {
    local dist="${INSTALL_DIR}/dist/copilot"
    print_success "Copilot instructions ready"
    print_info "Copy to your repository:"
    print_info "  cp ${dist}/.github/copilot-instructions.md <repo>/.github/"
}

# ─────────────────────────────────────────────────────────────────────────────
# Target Selection
# ─────────────────────────────────────────────────────────────────────────────

select_targets() {
    detect_tools

    if [[ ${#DETECTED_TOOLS[@]} -eq 0 ]]; then
        print_error "No AI tools detected"
        exit 1
    fi

    echo ""
    if [[ "$HAS_GUM" == true ]]; then
        gum style --foreground 255 --bold "Select targets to install:"
        echo ""

        local options=()
        for key in "${!DETECTED_TOOLS[@]}"; do
            options+=("${key}:${DETECTED_TOOLS[$key]}")
        done

        local selected
        selected=$(printf '%s\n' "${options[@]}" | \
            gum choose --no-limit \
                --cursor.foreground 99 \
                --selected.foreground 82 \
                --header.foreground 244 | \
            cut -d: -f1)

        if [[ -z "$selected" ]]; then
            print_error "No targets selected"
            exit 0
        fi

        SELECTED_TARGETS=()
        while IFS= read -r target; do
            [[ -n "$target" ]] && SELECTED_TARGETS+=("$target")
        done <<< "$selected"
    else
        echo -e "${BOLD}Select targets to install:${NC}"
        echo ""

        local i=1
        local keys=()
        for key in "${!DETECTED_TOOLS[@]}"; do
            echo -e "  ${MAGENTA}${i})${NC} ${DETECTED_TOOLS[$key]}"
            keys+=("$key")
            ((i++))
        done

        echo ""
        echo -e "${DIM}Enter numbers (space-separated), 'all', or 'q' to quit:${NC}"
        read -r selection

        if [[ "$selection" == "q" ]]; then
            exit 0
        fi

        SELECTED_TARGETS=()
        if [[ "$selection" == "all" ]]; then
            SELECTED_TARGETS=("${keys[@]}")
        else
            for num in $selection; do
                if [[ "$num" =~ ^[0-9]+$ ]] && [[ "$num" -le "${#keys[@]}" ]] && [[ "$num" -gt 0 ]]; then
                    SELECTED_TARGETS+=("${keys[$((num-1))]}")
                fi
            done
        fi

        if [[ ${#SELECTED_TARGETS[@]} -eq 0 ]]; then
            print_error "No targets selected"
            exit 0
        fi
    fi
}

# ─────────────────────────────────────────────────────────────────────────────
# Main
# ─────────────────────────────────────────────────────────────────────────────

main() {
    local update_mode=false
    local specific_target=""
    local install_all=false

    while [[ $# -gt 0 ]]; do
        case $1 in
            --update) update_mode=true; shift ;;
            --target) specific_target="$2"; shift 2 ;;
            --all) install_all=true; shift ;;
            --help|-h)
                echo "Usage: install.sh [--update] [--target <target>] [--all]"
                exit 0
                ;;
            *) shift ;;
        esac
    done

    print_header

    # Step 1: Check requirements
    print_step "1" "Checking requirements"
    check_requirements
    print_success "All requirements met"
    echo ""

    # Step 2: Clone/update
    print_step "2" "Fetching agent-skills"
    clone_or_update
    echo ""

    # Step 3: Select targets
    print_step "3" "Target selection"
    if [[ -n "$specific_target" ]]; then
        SELECTED_TARGETS=("$specific_target")
        print_info "Target: $specific_target"
    elif [[ "$install_all" == true ]]; then
        detect_tools
        SELECTED_TARGETS=("${!DETECTED_TOOLS[@]}")
        print_info "Installing all detected targets"
    else
        select_targets
    fi
    echo ""

    # Step 4: Build
    print_step "4" "Building distributions"
    build_targets "${SELECTED_TARGETS[@]}"
    echo ""

    # Step 5: Install
    print_step "5" "Installing"
    echo ""
    for target in "${SELECTED_TARGETS[@]}"; do
        case $target in
            claude-code) install_claude_code ;;
            opencode) install_opencode ;;
            cursor) install_cursor ;;
            copilot) install_copilot ;;
        esac
    done

    # Done
    echo ""
    if [[ "$HAS_GUM" == true ]]; then
        gum style \
            --border rounded \
            --border-foreground 82 \
            --padding "0 2" \
            --margin "1 0" \
            "$(gum style --foreground 82 --bold '✓ Installation complete!')"
    else
        echo -e "${GREEN}${BOLD}✓ Installation complete!${NC}"
    fi

    echo ""
    print_info "Update later with: ~/.local/share/agent-skills/install.sh --update"
    echo ""
}

main "$@"
