#!/usr/bin/env bash
set -euo pipefail

GOW_BIN="/usr/local/bin/gow"
GOW_CONF_DIR="/etc/gow"
GOW_DNS_DIR="/etc/gow/dns"
GOW_WEB_ROOT="/var/www"
GOW_BACKUP_DIR="/var/backups/gow"
GOW_LOG_DIR="/var/log/lsws"
GOW_CRON_DIR="/etc/cron.d"
RELEASE_URL="https://github.com/aprakasa/gow/releases/latest/download/gow"
INSTALL_LOG="/var/log/gow-install.log"

# Colors
if [ -t 1 ] && [ "${TERM:-}" != "dumb" ]; then
    C_GREEN="\033[0;32m"
    C_RED="\033[0;31m"
    C_CYAN="\033[0;36m"
    C_YELLOW="\033[1;33m"
    C_RESET="\033[0m"
else
    C_GREEN="" C_RED="" C_CYAN="" C_YELLOW="" C_RESET=""
fi

ERRORS=0
FORCE=0
MODE=install

# --- Utility functions ---

_info()  { printf "${C_CYAN}[INFO]${C_RESET}  %s\n" "$*"; }
_warn()  { printf "${C_YELLOW}[WARN]${C_RESET}  %s\n" "$*"; }
_error() { printf "${C_RED}[ERROR]${C_RESET} %s\n" "$*" >&2; }

_run() {
    local label="$1"; shift
    printf "  %-50s " "$label..."
    if "$@" >>"$INSTALL_LOG" 2>&1; then
        printf '%b\n' "${C_GREEN}[OK]${C_RESET}"
    else
        printf '%b\n' "${C_RED}[KO]${C_RESET}"
        ERRORS=$((ERRORS + 1))
    fi
}

_banner() {
    if [ "$MODE" = "purge" ]; then
        printf "\n  gow uninstaller\n  https://github.com/aprakasa/gow\n\n"
    else
        printf "\n  gow installer\n  https://github.com/aprakasa/gow\n\n"
    fi
}

_cleanup() {
    if [ "$ERRORS" -gt 0 ]; then
        _error "Completed with $ERRORS error(s). See $INSTALL_LOG"
    fi
}

# --- Checks ---

_check_root() {
    if [ "$EUID" -ne 0 ]; then
        _error "This script must be run as root."
        exit 100
    fi
}

_check_os() {
    if [ "$FORCE" -eq 1 ]; then
        _warn "Skipping OS check (--force)"
        return 0
    fi
    if [ ! -f /etc/os-release ]; then
        _error "Cannot detect OS (/etc/os-release missing). Use --force to bypass."
        exit 100
    fi
    # shellcheck disable=SC1091
    local id
    # shellcheck disable=SC1091
    id=$(. /etc/os-release 2>/dev/null && echo "$ID")
    case "$id" in
        ubuntu|debian) ;;
        *)
            _error "Unsupported OS: $id. Only Ubuntu and Debian are supported."
            _error "Re-run with --force to bypass this check."
            exit 100
            ;;
    esac
}

_detect_existing() {
    if [ ! -x "$GOW_BIN" ]; then
        return 0
    fi
    local version
    version=$("$GOW_BIN" --version 2>/dev/null || echo "unknown")
    _info "Existing install found: $version"
    printf "  Upgrade to latest? [Y/n] "
    read -r answer
    case "$answer" in
        n*|N*) _info "Keeping current version."; exit 0 ;;
    esac
}

# --- Install ---

_download_binary() {
    local tmp="/tmp/gow.$$"

    _info "Downloading gow binary..."
    if command -v curl >/dev/null 2>&1; then
        if ! curl -fSL -o "$tmp" "$RELEASE_URL" 2>>"$INSTALL_LOG"; then
            _error "Download failed. Check your internet connection and GitHub releases."
            rm -f "$tmp"
            exit 1
        fi
    elif command -v wget >/dev/null 2>&1; then
        if ! wget -qO "$tmp" "$RELEASE_URL" 2>>"$INSTALL_LOG"; then
            _error "Download failed. Check your internet connection and GitHub releases."
            rm -f "$tmp"
            exit 1
        fi
    else
        _error "Neither curl nor wget found. Install one and re-run."
        exit 1
    fi

    if [ ! -s "$tmp" ]; then
        _error "Downloaded file is empty."
        rm -f "$tmp"
        exit 1
    fi

    if command -v file >/dev/null 2>&1; then
        if ! file "$tmp" | grep -q "ELF.*executable\|ELF.*shared object"; then
            _error "Downloaded file is not a valid binary. Expected ELF executable."
            _error "You may be on an unsupported architecture (only amd64 is supported)."
            rm -f "$tmp"
            exit 1
        fi
    else
        _warn "'file' not found, skipping binary validation"
    fi

    mv "$tmp" "$GOW_BIN"
    chmod 0755 "$GOW_BIN"
}

_verify_binary() {
    if ! "$GOW_BIN" --help >/dev/null 2>&1; then
        _error "Binary verification failed. The binary may not be compatible with this system."
        exit 1
    fi
    local version
    version=$("$GOW_BIN" --version 2>/dev/null || echo "unknown")
    _info "Installed: gow $version"
}

_create_dirs() {
    _run "Creating $GOW_CONF_DIR" mkdir -p "$GOW_CONF_DIR"
    _run "Creating $GOW_DNS_DIR" mkdir -p "$GOW_DNS_DIR"
    chmod 0700 "$GOW_DNS_DIR"

    if [ -d "$GOW_WEB_ROOT" ] && [ "$(ls -A "$GOW_WEB_ROOT" 2>/dev/null)" ]; then
        _info "$GOW_WEB_ROOT already exists and is non-empty, preserving"
    else
        _run "Creating $GOW_WEB_ROOT" mkdir -p "$GOW_WEB_ROOT"
    fi

    _run "Creating $GOW_BACKUP_DIR" mkdir -p "$GOW_BACKUP_DIR"
    _run "Creating $GOW_LOG_DIR" mkdir -p "$GOW_LOG_DIR"
}

_install_flow() {
    _banner
    _check_root
    _check_os

    # Set up logging (after root check so we can write to /var/log)
    mkdir -p /var/log
    touch "$INSTALL_LOG"
    exec > >(tee -a "$INSTALL_LOG") 2>&1

    _detect_existing
    _download_binary
    _verify_binary
    _create_dirs

    printf "\n"
    if [ "$ERRORS" -gt 0 ]; then
        _error "Install completed with errors. See $INSTALL_LOG"
    else
        printf '%b\n' "  ${C_GREEN}gow installed successfully.${C_RESET}\n"
        printf "  Next steps:\n"
        printf "    sudo gow stack install\n"
        printf "    sudo gow site create example.com --type wp --tune blog --php 83\n\n"
    fi
}

# --- Purge ---

_purge_flow() {
    _banner
    _check_root

    mkdir -p /var/log
    touch "$INSTALL_LOG"
    exec > >(tee -a "$INSTALL_LOG") 2>&1

    printf "  This will remove the gow binary, configuration, cron jobs, and backups.\n"
    printf "  Site data in /var/www will NOT be removed.\n"
    printf "  Continue? [y/N] "
    read -r answer
    case "$answer" in
        y*|Y*) ;;
        *) _info "Aborted."; exit 0 ;;
    esac

    _run "Removing binary" rm -f "$GOW_BIN"
    _run "Removing cron files" rm -f "$GOW_CRON_DIR"/gow-* "$GOW_CRON_DIR"/gow-backups
    _run "Removing config" rm -rf "$GOW_CONF_DIR"
    _run "Removing backups" rm -rf "$GOW_BACKUP_DIR"
    _run "Removing logrotate config" rm -f /etc/logrotate.d/gow

    printf "\n"
    _warn "Site data in /var/www was NOT removed."
    _warn "Log files in /var/log/lsws were NOT removed."
    _warn "The OpenLiteSpeed stack was NOT removed. Run 'gow stack purge' first if needed."
    printf '%b\n' "\n  ${C_GREEN}gow has been uninstalled.${C_RESET}\n"
}

# --- Args & main ---

_parse_args() {
    for arg in "$@"; do
        case "$arg" in
            --purge|--uninstall) MODE=purge ;;
            --force)             FORCE=1 ;;
            --help|-h)
                printf "Usage: %s [--purge] [--force] [--help]\n\n" "$0"
                printf "  --purge   Uninstall gow (binary, config, cron, backups)\n"
                printf "  --force   Bypass OS distro check\n"
                printf "  --help    Show this help\n"
                exit 0
                ;;
            *)
                _error "Unknown argument: $arg"
                printf "Run '%s --help' for usage.\n" "$0"
                exit 1
                ;;
        esac
    done
}

main() {
    _parse_args "$@"
    trap _cleanup EXIT
    if [ "$MODE" = "purge" ]; then
        _purge_flow
    else
        _install_flow
    fi
}

main "$@"
