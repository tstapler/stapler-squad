#!/bin/sh
#
# install-service.sh — Install stapler-squad as a system service
#
# Supports:
#   Linux  — systemd user service (~/.config/systemd/user/)
#   macOS  — LaunchAgent (~/Library/LaunchAgents/)
#
# Usage:
#   ./scripts/install-service.sh              # install (profiling enabled on :6060 by default)
#   ./scripts/install-service.sh --no-profile # install without profiling
#   ./scripts/install-service.sh --profile-port 6061  # install with custom profiling port
#   ./scripts/install-service.sh --uninstall  # remove
#
# Environment:
#   STAPLER_SQUAD_BIN   Override binary path (default: auto-detected)
#   PROFILE_PORT        Override profiling port (default: 6060)
#

set -e

# ── Colors ────────────────────────────────────────────────────────────────────
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

log_info()    { printf "${BLUE}==>${NC} %s\n" "$1"; }
log_success() { printf "${GREEN}✓${NC} %s\n" "$1"; }
log_warning() { printf "${YELLOW}!${NC} %s\n" "$1"; }
log_error()   { printf "${RED}✗${NC} %s\n" "$1" >&2; }

# ── OS Detection ──────────────────────────────────────────────────────────────
detect_os() {
    case "$(uname -s)" in
        Linux*)  echo "linux" ;;
        Darwin*) echo "macos" ;;
        *)       echo "unsupported" ;;
    esac
}

# ── Binary Path Resolution ────────────────────────────────────────────────────
# Priority: STAPLER_SQUAD_BIN env var > which > local build artifact
resolve_binary() {
    if [ -n "${STAPLER_SQUAD_BIN:-}" ]; then
        if [ ! -x "$STAPLER_SQUAD_BIN" ]; then
            log_error "STAPLER_SQUAD_BIN='$STAPLER_SQUAD_BIN' is not executable"
            exit 1
        fi
        echo "$STAPLER_SQUAD_BIN"
        return
    fi

    if command -v stapler-squad >/dev/null 2>&1; then
        command -v stapler-squad
        return
    fi

    local_bin="$(pwd)/stapler-squad"
    if [ -x "$local_bin" ]; then
        echo "$local_bin"
        return
    fi

    log_error "Cannot find stapler-squad binary."
    log_info  "Options:"
    log_info  "  1. Run 'make build' then re-run this script from the project root"
    log_info  "  2. Run 'make install' to install to GOPATH/bin, then re-run"
    log_info  "  3. Set STAPLER_SQUAD_BIN=/path/to/binary and re-run"
    exit 1
}

# ── Linux / systemd user service ──────────────────────────────────────────────
install_linux() {
    bin_path="$1"
    service_dir="$HOME/.config/systemd/user"
    service_file="$service_dir/stapler-squad.service"
    log_dir="$HOME/.stapler-squad/logs"

    # Verify systemd --user is available before writing any files
    if ! systemctl --user is-system-running >/dev/null 2>&1 && \
       ! systemctl --user status >/dev/null 2>&1; then
        log_error "systemd user session is not available."
        log_info  "On WSL or minimal containers, try adding stapler-squad to ~/.profile instead:"
        log_info  "  echo '$bin_path &' >> ~/.profile"
        exit 1
    fi

    log_info "Creating systemd user service..."
    mkdir -p "$service_dir"
    mkdir -p "$log_dir"

    cat > "$service_file" << EOF
[Unit]
Description=Stapler Squad — AI Agent Session Manager
Documentation=https://github.com/tstapler/stapler-squad
After=network.target

[Service]
Type=simple
ExecStart=$bin_path --remote-access$extra_flags
WorkingDirectory=$HOME
Restart=on-failure
RestartSec=5s
KillMode=process
StandardOutput=append:$log_dir/service.log
StandardError=append:$log_dir/service.log
Environment=HOME=$HOME
Environment=PATH=$PATH

[Install]
WantedBy=default.target
EOF

    log_success "Service file written to: $service_file"
    echo ""

    # Reload systemd and restart (or start) the service automatically.
    log_info "Reloading systemd and restarting service..."
    systemctl --user daemon-reload
    systemctl --user enable stapler-squad
    if systemctl --user is-active --quiet stapler-squad; then
        systemctl --user restart stapler-squad
        log_success "Service restarted."
    else
        systemctl --user start stapler-squad
        log_success "Service started."
    fi

    echo ""
    log_info "Check status:"
    echo "    systemctl --user status stapler-squad"
    echo ""
    log_info "View logs:"
    echo "    tail -f $log_dir/service.log"
    echo ""
    log_info "Optional — keep service running after logout (one-time setup):"
    echo "    loginctl enable-linger \$USER"
}

# ── macOS / LaunchAgent ───────────────────────────────────────────────────────
install_macos() {
    bin_path="$1"
    plist_dir="$HOME/Library/LaunchAgents"
    plist_file="$plist_dir/com.stapler-squad.plist"
    log_dir="$HOME/.stapler-squad/logs"

    log_info "Creating macOS LaunchAgent..."
    mkdir -p "$plist_dir"
    mkdir -p "$log_dir"

    # Build a PATH that preserves the user's shell PATH first (so custom tools,
    # go/bin, nvm, rbenv, etc. take precedence), then appends both Homebrew
    # prefixes (Apple Silicon + Intel) as a fallback so tools like tmux, git,
    # and claude are found even if not already on the shell PATH.
    plist_path="$PATH:/opt/homebrew/bin:/opt/homebrew/sbin:/usr/local/bin:/usr/local/sbin:/usr/bin:/bin:/usr/sbin:/sbin"

    # Build XML <string> entries for any extra flags (e.g. --profile --profile-port 6060).
    # We rely on the EnvironmentVariables PATH key above, so no shell wrapper is needed.
    extra_args_xml=""
    for arg in $extra_flags; do
        extra_args_xml="$extra_args_xml
        <string>$arg</string>"
    done

    cat > "$plist_file" << EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN"
    "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.stapler-squad</string>

    <key>Program</key>
    <string>$bin_path</string>

    <key>ProgramArguments</key>
    <array>
        <string>$bin_path</string>
        <string>--remote-access</string>$extra_args_xml
    </array>

    <key>RunAtLoad</key>
    <true/>

    <key>KeepAlive</key>
    <dict>
        <key>SuccessfulExit</key>
        <false/>
    </dict>

    <key>WorkingDirectory</key>
    <string>$HOME</string>

    <key>EnvironmentVariables</key>
    <dict>
        <key>HOME</key>
        <string>$HOME</string>
        <key>PATH</key>
        <string>$plist_path</string>
    </dict>

    <key>StandardOutPath</key>
    <string>$log_dir/service.log</string>

    <key>StandardErrorPath</key>
    <string>$log_dir/service.log</string>

    <key>ThrottleInterval</key>
    <integer>5</integer>
</dict>
</plist>
EOF

    log_success "LaunchAgent plist written to: $plist_file"
    echo ""

    # Stop the existing service before loading the updated plist.
    # Use 'launchctl bootout' (blocking — waits for the process to exit) so the
    # old process is fully gone before the new one starts.  This prevents the two
    # processes from racing over tmux sessions.  Fall back to 'launchctl unload'
    # on older macOS that lacks bootout support.
    log_info "Stopping existing service (if running)..."
    if ! launchctl bootout "gui/$(id -u)/com.stapler-squad" 2>/dev/null; then
        launchctl unload "$plist_file" 2>/dev/null || true
    fi

    # Brief grace period for the process to finish writing its final state.
    sleep 0.5

    log_info "Starting updated service..."
    if launchctl bootstrap "gui/$(id -u)" "$plist_file" 2>/dev/null; then
        log_success "Service started via launchctl bootstrap."
    else
        # Fallback for macOS 12 and earlier
        launchctl load -w "$plist_file"
        log_success "Service loaded via launchctl load."
    fi

    echo ""
    log_info "Check status:"
    echo "    launchctl list | grep stapler-squad"
    echo ""
    log_info "View logs:"
    echo "    tail -f $log_dir/service.log"

    # ── Full Disk Access reminder ─────────────────────────────────────────────
    # stapler-squad creates sessions in arbitrary directories (~/Documents,
    # ~/Developer, etc.).  Without Full Disk Access, macOS pops a TCC consent
    # dialog on every startup for each protected directory it touches.
    # Granting Full Disk Access suppresses those dialogs permanently.
    echo ""
    log_info "macOS Privacy — Full Disk Access"
    echo "    stapler-squad needs Full Disk Access to create sessions in any"
    echo "    directory without macOS prompting for consent each time."
    echo ""
    echo "    To grant it:"
    echo "      1. Open: System Settings → Privacy & Security → Full Disk Access"
    echo "      2. Click '+' and add: $bin_path"
    echo "      3. Restart the service: launchctl kickstart -k gui/\$(id -u)/com.stapler-squad"
    echo ""
    echo "    Opening Privacy & Security now..."
    open "x-apple.systempreferences:com.apple.preference.security?Privacy_AllFiles" 2>/dev/null || true
}

# ── Health Check + Auto-rollback ──────────────────────────────────────────────
# Polls localhost:8543/health for up to 15s. On failure, restores the .prev
# binary (if it exists) and restarts the service automatically.
health_check_and_rollback() {
    bin_path="$1"
    prev_bin="${bin_path}.prev"
    max_wait=15
    elapsed=0
    url="http://localhost:8543/health"
    if ! command -v curl >/dev/null 2>&1; then
        echo "curl not found, skipping health check"
        return 0
    fi
    printf "==> Waiting for service to be healthy"
    while [ "$elapsed" -lt "$max_wait" ]; do
        if curl -sf "$url" >/dev/null 2>&1; then
            printf "\n"
            log_success "Service is healthy"
            return 0
        fi
        printf "."
        sleep 1
        elapsed=$((elapsed + 1))
    done
    printf "\n"
    log_error "Service did not respond within ${max_wait}s."

    if [ ! -f "$prev_bin" ]; then
        log_warning "No previous build found at $prev_bin — cannot auto-rollback."
        log_info "Check logs: tail -f ~/.stapler-squad/logs/service.log"
        return 1
    fi

    log_info "Auto-rolling back to previous build..."
    cp -f "$prev_bin" "$bin_path"
    log_success "Binary restored from $prev_bin"

    os=$(detect_os)
    case "$os" in
        linux)
            systemctl --user restart stapler-squad
            log_success "Service restarted with previous build."
            ;;
        macos)
            launchctl kickstart -k "gui/$(id -u)/com.stapler-squad" 2>/dev/null || \
                launchctl stop "gui/$(id -u)/com.stapler-squad" 2>/dev/null || true
            log_success "Service restarted with previous build."
            ;;
    esac
    log_info "Check logs: tail -f ~/.stapler-squad/logs/service.log"
    return 1
}


# ── Uninstall ─────────────────────────────────────────────────────────────────
uninstall_service() {
    os="$1"
    case "$os" in
        linux)
            service_file="$HOME/.config/systemd/user/stapler-squad.service"
            log_info "Stopping and disabling systemd user service..."
            systemctl --user stop stapler-squad 2>/dev/null || true
            systemctl --user disable stapler-squad 2>/dev/null || true
            if [ -f "$service_file" ]; then
                rm -f "$service_file"
                systemctl --user daemon-reload 2>/dev/null || true
                log_success "Removed: $service_file"
            else
                log_warning "Service file not found (already removed?): $service_file"
            fi
            ;;
        macos)
            plist_file="$HOME/Library/LaunchAgents/com.stapler-squad.plist"
            log_info "Unloading macOS LaunchAgent..."
            launchctl unload "$plist_file" 2>/dev/null || true
            if [ -f "$plist_file" ]; then
                rm -f "$plist_file"
                log_success "Removed: $plist_file"
            else
                log_warning "Plist not found (already removed?): $plist_file"
            fi
            ;;
    esac
    log_info "stapler-squad will no longer start automatically on login."
    log_info "Your data in ~/.stapler-squad/ has not been touched."
}

# ── Main ──────────────────────────────────────────────────────────────────────
UNINSTALL=0
ENABLE_PROFILE=1
PROFILE_PORT="${PROFILE_PORT:-6060}"

while [ $# -gt 0 ]; do
    case "$1" in
        --uninstall)      UNINSTALL=1 ;;
        --no-profile)     ENABLE_PROFILE=0 ;;
        --profile-port)   shift; PROFILE_PORT="$1" ;;
        --profile-port=*) PROFILE_PORT="${1#*=}" ;;
    esac
    shift
done

main() {
    os=$(detect_os)

    if [ "$os" = "unsupported" ]; then
        log_error "Unsupported platform: $(uname -s)"
        log_info  "Supported platforms: Linux (systemd user), macOS (LaunchAgent)"
        exit 1
    fi

    if [ "$UNINSTALL" = "1" ]; then
        uninstall_service "$os"
        exit 0
    fi

    # Build extra flags to append to the binary invocation
    extra_flags=""
    if [ "$ENABLE_PROFILE" = "1" ]; then
        extra_flags=" --profile --profile-port $PROFILE_PORT"
        log_info "Profiling enabled on port $PROFILE_PORT (pass --no-profile to disable)"
    fi

    bin_path=$(resolve_binary)
    log_info "Using binary: $bin_path"

    case "$os" in
        linux) install_linux "$bin_path" ;;
        macos) install_macos "$bin_path" ;;
    esac

    health_check_and_rollback "$bin_path"
}

main "$@"
