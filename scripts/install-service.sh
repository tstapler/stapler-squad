#!/bin/sh
# shellcheck shell=bash
#
# This targets bash specifically (it uses 'local', a bash extension not in
# POSIX sh) despite the #!/bin/sh shebang: on every platform this script
# actually runs on (macOS, and Linux with a systemd user session), /bin/sh
# resolves to bash or another shell with 'local' support, so the shebang
# stays POSIX-portable-looking while the directive above tells shellcheck to
# check it as bash rather than flag every 'local' as non-POSIX (SC3043).
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
#   STAPLER_SQUAD_BIN          Override binary path (default: auto-detected)
#   PROFILE_PORT               Override profiling port (default: 6060)
#   STAPLER_SQUAD_HEALTH_TIMEOUT  Seconds to wait for /health after (re)start (default: 300)
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

# ── Log Rotation ──────────────────────────────────────────────────────────────
# launchd/systemd just append() to StandardOutPath/StandardError forever — neither
# rotates it. A crash-restart loop (e.g. ThrottleInterval-bounded respawns) can grow
# service.log to millions of lines in minutes. Rotate on every install/restart so a
# bad run doesn't silently fill the disk between installs.
# Threshold: 20 MiB.
LOG_ROTATE_MAX_BYTES=20971520

rotate_log_if_large() {
    log_file="$1"
    [ -f "$log_file" ] || return 0

    size=$(wc -c < "$log_file" 2>/dev/null | tr -d ' ')
    [ -n "$size" ] || return 0

    if [ "$size" -gt "$LOG_ROTATE_MAX_BYTES" ]; then
        mv -f "$log_file" "$log_file.old"
        : > "$log_file"
        log_info "Rotated oversized log ($((size / 1048576)) MiB): $log_file -> $log_file.old"
    fi
}

# Remove duplicate ':'-separated PATH entries, keeping first occurrence order.
# The unit/plist below bakes in "$PATH:<fallbacks>" verbatim from the invoking
# shell; re-running this script from a shell whose own PATH already carries
# duplicates (e.g. nested tool/plugin PATH prepends) writes those duplicates
# into the persisted service file, and each subsequent install compounds it
# further since the new shell inherits the bloated PATH. Once large enough,
# every spawned tmux session re-embeds PATH via `-e PATH=...`, and the total
# `tmux new-session` command line exceeds tmux's message-size limit — every
# session/tmux spawn then fails with "command too long" (exit status 1).
dedup_path() {
    printf '%s' "$1" | awk -v RS=':' '{ if (!seen[$0]++) { if (out != "") out = out ":" $0; else out = $0 } } END { printf "%s", out }'
}

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

    # Verify systemd --user is available before writing any files.
    # Use timeout to avoid hanging indefinitely if D-Bus is unresponsive.
    if ! timeout 5 systemctl --user is-system-running >/dev/null 2>&1 && \
       ! timeout 5 systemctl --user status >/dev/null 2>&1; then
        log_error "systemd user session is not available."
        log_info  "On WSL or minimal containers, try adding stapler-squad to ~/.profile instead:"
        log_info  "  echo '$bin_path &' >> ~/.profile"
        exit 1
    fi

    log_info "Creating systemd user service..."
    mkdir -p "$service_dir"
    mkdir -p "$log_dir"
    rotate_log_if_large "$log_dir/service.log"

    # Build a PATH that preserves the current shell's PATH first (so custom
    # tools, nvm/asdf shims, etc. resolve identically to an interactive shell)
    # but appends standard fallback locations, mirroring install_macos's
    # LaunchAgent PATH below. Without this, the unit bakes in a raw PATH
    # snapshot from install time with no fallback: if claude/tmux/git later
    # move (nvm/asdf reinstall, a fresh `pip install --user`/npm global
    # install to ~/.local/bin) without a subsequent `make install-service`,
    # the headless LLM pool's exec.LookPath("claude") silently fails and
    # backlog triage no-ops with only a log warning (see server/dependencies.go).
    # Deduplicated (see dedup_path) so repeated installs don't compound PATH growth.
    service_path=$(dedup_path "$PATH:$HOME/.local/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin")

    # Cgroup memory bound: this service's cgroup covers the Go binary AND every
    # process it forks (tmux server + all spawned Claude agent sessions), since
    # children inherit their parent's cgroup at fork time regardless of tmux
    # daemonizing/detaching. Capping it keeps a runaway burst of concurrent agents
    # (the 2026-07-12 OOM incident: 57/61GB used, swap exhausted) from taking down
    # the whole box — the kernel's cgroup-aware OOM killer instead picks a victim
    # from within this budget, leaving unrelated system processes alone.
    # MemoryHigh (soft: throttle/reclaim, no kill) at 60% and MemoryMax (hard kill
    # boundary) at 80% of total RAM, both computed from this machine's actual
    # /proc/meminfo rather than a hardcoded value so the same script is safe on a
    # small VM or a large workstation alike. Skipped entirely if detection fails.
    mem_total_kb=$(awk '/^MemTotal:/{print $2}' /proc/meminfo 2>/dev/null || true)
    memory_limit_lines=""
    if [ -n "$mem_total_kb" ]; then
        mem_high_mb=$((mem_total_kb * 60 / 100 / 1024))
        mem_max_mb=$((mem_total_kb * 80 / 100 / 1024))
        memory_limit_lines="MemoryHigh=${mem_high_mb}M
MemoryMax=${mem_max_mb}M"
    else
        log_warning "Could not read /proc/meminfo; skipping MemoryHigh/MemoryMax cgroup limits"
    fi

    cat > "$service_file" << EOF
[Unit]
Description=Stapler Squad — AI Agent Session Manager
Documentation=https://github.com/tstapler/stapler-squad
After=network.target
# Tolerate a burst of OOM-kill/restart cycles during sustained memory pressure
# without systemd permanently giving up (default is 5 restarts / 10s — a 5s
# RestartSec can blow through that in one bad episode, leaving the service
# down until a manual 'systemctl reset-failed'). It still gives up eventually
# if genuinely crash-looping forever.
StartLimitIntervalSec=600
StartLimitBurst=10

[Service]
Type=simple
ExecStart=$bin_path --remote-access --tmux-keep-server$extra_flags
WorkingDirectory=$HOME
Restart=on-failure
RestartSec=5s
KillMode=process
# Mild protective bias for the coordinator process itself (also inherited by
# spawned children — a coarse, honestly-scoped tradeoff; per-session cgroup
# delegation would be needed to bias only the coordinator, which is out of
# scope here). In practice the kernel's OOM badness score is dominated by RSS,
# and Claude agent subprocesses are the memory-heavy ones, so this mostly just
# nudges ties in the coordinator's favor.
OOMScoreAdjust=-500
$memory_limit_lines
StandardOutput=append:$log_dir/service.log
StandardError=append:$log_dir/service.log
Environment="HOME=$HOME"
Environment="PATH=$service_path"

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

# ── TCC / Full Disk Access helpers ───────────────────────────────────────────
# Returns 0 if $1 (binary path) has an explicit FDA grant in the TCC database,
# 1 otherwise (not granted, denied, or DB unreadable without FDA itself).
# sqlite3 is pre-installed on macOS; we try both the system DB and the
# per-user DB so the check works regardless of whether the calling terminal
# has FDA.
#
# Non-admin users cannot read either TCC database (authorization denied).
# In that case, if the binary is already installed and cert-signed (not
# ad-hoc), we assume FDA was previously granted — the TCC grant is tied
# to the signing identity (com.stapler-squad + cert), which is stable across
# rebuilds, so re-installs don't need a new grant.
fda_is_granted() {
    local bin_path="$1"
    local result
    local any_db_found=false
    local all_denied=true
    for tcc_db in \
        "/Library/Application Support/com.apple.TCC/TCC.db" \
        "$HOME/Library/Application Support/com.apple.TCC/TCC.db"
    do
        [ -f "$tcc_db" ] || continue
        any_db_found=true
        if [ ! -r "$tcc_db" ]; then
            # DB exists but unreadable — likely non-admin user; note it and skip.
            continue
        fi
        all_denied=false
        # auth_value=2  → kTCCAuthorizationRightAllow (macOS 11+)
        # allowed=1     → legacy boolean schema (macOS 10.x)
        result=$(sqlite3 "$tcc_db" \
            "SELECT COALESCE(auth_value, allowed) FROM access
             WHERE service='kTCCServiceSystemPolicyAllFiles'
               AND client='$bin_path'" 2>/dev/null)
        [ "$result" = "2" ] || [ "$result" = "1" ] && return 0
    done

    # If at least one TCC DB existed but none were readable (non-admin user),
    # fall back to a heuristic: assume FDA is already granted if the binary
    # exists at the install path and is signed with our cert (not ad-hoc).
    # Ad-hoc signatures embed a cdhash that changes every build; cert-signed
    # binaries keep a stable designated requirement, so their TCC grant persists.
    if $any_db_found && $all_denied && [ -f "$bin_path" ]; then
        local dr
        dr=$(codesign -d --requirements - "$bin_path" 2>/dev/null)
        if echo "$dr" | grep -q "certificate root"; then
            return 0
        fi
    fi

    return 1
}

# Polls (up to 10s) until none of the given TCP ports have a LISTENer, so the
# incoming process doesn't race the outgoing one's socket teardown. Ports that
# are empty/unset (e.g. profiling disabled) are skipped. Proceeds with a
# warning on timeout rather than blocking forever — a genuinely stuck old
# process needs a human, not a longer sleep.
wait_for_port_release() {
    max_ticks=20  # 20 * 0.5s = 10s
    tick=0
    while [ "$tick" -lt "$max_ticks" ]; do
        busy=0
        for port in "$@"; do
            [ -n "$port" ] || continue
            if lsof -nP -iTCP:"$port" -sTCP:LISTEN >/dev/null 2>&1; then
                busy=1
                break
            fi
        done
        [ "$busy" = "0" ] && return 0
        sleep 0.5
        tick=$((tick + 1))
    done
    log_warning "Old process still holding a port after $((max_ticks / 2))s — starting anyway."
    return 1
}

# ── macOS launchd start/stop helpers ─────────────────────────────────────────
# Shared by install_macos (fresh install/redeploy) and health_check_and_rollback
# (restarting with the previous binary after a failed deploy) so both paths use
# the exact same, verified start/stop sequence instead of the rollback path
# guessing at a different launchctl call — see macos_start_service's doc
# comment for the incident this fixes.

# Fully unregisters the current job (if any) and waits for its listening ports
# to actually clear, so the next start doesn't race the old process's teardown.
#
# 'launchctl bootout' unregisters the job from launchd but, in practice,
# returns before the old process has actually released its listening sockets
# — the process is still tearing down (flushing tmux/session state) when
# bootout's call returns. A fixed short sleep here would be a race: if the new
# process starts and tries to bind :8543/:8444 before the old one has let go,
# it fails with "bind: address already in use", crashes, and gets stuck in
# launchd's KeepAlive restart loop until the old socket is finally freed —
# which is what caused the health check to time out on prior runs (confirmed
# via ~/.stapler-squad/logs/service.log showing repeated "bind remote server
# on 0.0.0.0:8444: ... address already in use" crash-loop entries). Poll for
# the ports to actually clear instead of guessing a sleep duration. Fall back
# to 'launchctl unload' on older macOS that lacks bootout support.
macos_stop_service() {
    plist_file="$1"
    log_info "Stopping existing service (if running)..."
    if ! launchctl bootout "gui/$(id -u)/com.stapler-squad" 2>/dev/null; then
        launchctl unload "$plist_file" 2>/dev/null || true
    fi
    if [ "$ENABLE_PROFILE" = "1" ]; then
        wait_for_port_release 8543 8444 "$PROFILE_PORT"
    else
        wait_for_port_release 8543 8444
    fi
}

# Registers and starts the job from plist_file via 'launchctl bootstrap',
# falling back to legacy 'load' if bootstrap fails (some macOS versions return
# an I/O error from bootstrap for reasons not fully understood — see git
# history). Returns 0 if either registration succeeded, 1 if both failed.
#
# IMPORTANT: whichever of the two succeeds determines which launchctl "domain"
# the job lives in. A job started via legacy 'load' is not reliably reachable
# via 'launchctl kickstart -k gui/$(id -u)/com.stapler-squad' (the modern
# bootstrap-domain API) — this is what caused a real incident (2026-08-18):
# a deploy fell back to 'load', its health check later timed out, and the
# rollback path's 'kickstart -k ... || stop ... || true' silently failed to
# restart anything (both calls target the bootstrap domain the job was never
# actually in) while still printing "Service restarted with previous build."
# The service was fully down until someone noticed and re-ran 'bootstrap' by
# hand. Always go through this same function to (re)start, on both the
# forward-deploy and rollback paths, so there's only one code path that can
# have this class of bug.
macos_start_service() {
    plist_file="$1"
    log_info "Starting service..."
    if launchctl bootstrap "gui/$(id -u)" "$plist_file" 2>/dev/null; then
        log_success "Service started via launchctl bootstrap."
        return 0
    fi
    if launchctl load "$plist_file" 2>/dev/null; then
        log_success "Service started via launchctl load (bootstrap fallback)."
        return 0
    fi
    return 1
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
    rotate_log_if_large "$log_dir/service.log"

    # Build a PATH that preserves the user's shell PATH first (so custom tools,
    # go/bin, nvm, rbenv, etc. take precedence), then appends both Homebrew
    # prefixes (Apple Silicon + Intel) as a fallback so tools like tmux, git,
    # and claude are found even if not already on the shell PATH.
    # Deduplicated (see dedup_path) so repeated installs don't compound PATH growth.
    plist_path=$(dedup_path "$PATH:/opt/homebrew/bin:/opt/homebrew/sbin:/usr/local/bin:/usr/local/sbin:/usr/bin:/bin:/usr/sbin:/sbin")

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
        <string>--remote-access</string>
        <string>--tmux-keep-server</string>$extra_args_xml
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

    # ── Full Disk Access — gate before starting the service ──────────────────
    # On a fresh install (or after the binary path changes) the binary isn't
    # yet in the FDA list.  macOS will pop a TCC consent dialog the first time
    # it accesses a protected path (Documents, Desktop, iCloud Drive, etc.),
    # stalling startup past the health-check window and causing an apparent
    # crash/segfault.  Check the TCC database first; only prompt if FDA isn't
    # already granted so re-installs stay quiet.
    if ! fda_is_granted "$bin_path"; then
        echo ""
        log_warning "Full Disk Access not detected for this binary"
        echo ""
        echo "    stapler-squad needs Full Disk Access to create sessions in"
        echo "    Documents, Desktop, iCloud Drive, and other protected locations."
        echo ""
        echo "    System Settings → Privacy & Security → Full Disk Access"
        echo "    is opening now.  Add this binary and toggle it ON:"
        echo ""
        echo "      $bin_path"
        echo ""
        open "x-apple.systempreferences:com.apple.preference.security?Privacy_AllFiles" 2>/dev/null || true
        printf "    Waiting 15 s for you to grant access before starting the service"
        i=0
        while [ $i -lt 15 ]; do
            sleep 1
            printf "."
            i=$((i + 1))
        done
        printf "\n\n"
    fi

    # Verify the new binary is properly signed before stopping the running service.
    # This prevents a bad binary from taking the service down with no way back.
    if ! codesign --verify --no-strict "$bin_path" 2>/dev/null; then
        log_error "New binary failed code signature verification: $bin_path"
        log_error "Aborting install — existing service left running."
        exit 1
    fi

    macos_stop_service "$plist_file"

    if ! macos_start_service "$plist_file"; then
        log_error "launchctl bootstrap and load both failed — service may not start on login."
        log_error "Try: launchctl bootstrap gui/$(id -u) $plist_file"
        exit 1
    fi

    echo ""
    log_info "Check status:  launchctl list | grep stapler-squad"
    log_info "View logs:     tail -f $log_dir/service.log"
}

# ── Health Check + Auto-rollback ──────────────────────────────────────────────
# Default 300s (was 120s until 2026-08-18): a real instance with 20+ live tmux
# sessions to reconnect on startup was observed taking longer than 120s to
# answer /health, which made the old default declare a perfectly-healthy-but-
# still-starting deploy "dead" and trigger a needless rollback. Override with
# STAPLER_SQUAD_HEALTH_TIMEOUT for a faster local dev loop on a lightly-loaded
# instance, or an even longer wait on one with many more sessions still.
HEALTH_TIMEOUT="${STAPLER_SQUAD_HEALTH_TIMEOUT:-300}"

# Polls localhost:8543/health for up to max_wait seconds, printing a "." per
# second. Returns 0 as soon as it responds healthy. On macOS, also bails out
# early (before max_wait) if launchctl reports the job has already crashed,
# since a crashed binary won't recover just by waiting longer — crash_hint_bin
# is only used to print the FDA-grant hint in that crash case.
#
# Params are deliberately NOT named bin_path/prev_bin: this runs under /bin/sh
# with no real parameter scoping, and health_check_and_rollback (the caller)
# has its own same-named variables — reusing those names here would silently
# clobber the caller's copy after this function returns.
wait_for_health() {
    max_wait="$1"
    crash_hint_bin="$2"
    elapsed=0
    url="http://localhost:8543/health"
    printf '==> Waiting for service to be healthy (up to %ss)' "$max_wait"
    while [ "$elapsed" -lt "$max_wait" ]; do
        if curl -sf "$url" >/dev/null 2>&1; then
            printf "\n"
            log_success "Service is healthy"
            return 0
        fi

        # On macOS, check if the service has already crashed so we can bail
        # early instead of polling the full timeout for a dead process.
        if [ "$(uname -s)" = "Darwin" ] && [ "$elapsed" -ge 5 ]; then
            # launchctl list output: <pid>  <last_exit_status>  <label>
            # A running service has a numeric PID; a crashed one shows "-".
            svc_line=$(launchctl list 2>/dev/null | grep "com.stapler-squad" | head -1)
            svc_pid=$(echo "$svc_line" | awk '{print $1}')
            svc_exit=$(echo "$svc_line" | awk '{print $2}')
            if [ "$svc_pid" = "-" ] && [ -n "$svc_exit" ] && [ "$svc_exit" != "0" ]; then
                printf "\n"
                log_error "Service crashed at startup (launchctl exit status: $svc_exit)."
                log_info  "Check logs: tail -20 ~/.stapler-squad/logs/service.log"
                if [ "$svc_exit" = "139" ] || [ "$svc_exit" = "11" ]; then
                    log_warning "Exit status $svc_exit suggests a segfault."
                    log_warning "If this is a first install, ensure Full Disk Access is granted:"
                    log_warning "  System Settings → Privacy & Security → Full Disk Access"
                    log_warning "  Add: $crash_hint_bin"
                fi
                return 1
            fi
        fi

        printf "."
        sleep 1
        elapsed=$((elapsed + 1))
    done
    printf "\n"
    log_error "Service did not respond within ${max_wait}s."
    return 1
}

health_check_and_rollback() {
    bin_path="$1"
    prev_bin="${bin_path}.prev"

    if wait_for_health "$HEALTH_TIMEOUT" "$bin_path"; then
        return 0
    fi

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
            # Not '|| true': a genuinely failed restart must not be reported as a
            # success below — see the macos branch's comment for the exact
            # incident this class of bug caused there.
            if ! systemctl --user restart stapler-squad; then
                log_error "Failed to restart systemd service with rolled-back binary."
                log_info "Check logs: tail -f ~/.stapler-squad/logs/service.log"
                return 1
            fi
            ;;
        macos)
            # Go through the exact same stop/start helpers install_macos uses —
            # not 'launchctl kickstart -k ... || stop ... || true' (the prior
            # rollback logic). That assumed the running job was reachable via
            # the modern bootstrap-domain API, which is false whenever the
            # deploy that's being rolled back from itself fell back to legacy
            # 'launchctl load' (see macos_start_service's doc comment) — in
            # that case both kickstart and stop silently no-op, '|| true'
            # swallows the failure, and "Service restarted with previous
            # build." printed unconditionally regardless. That's exactly what
            # happened in a real 2026-08-18 incident: the service was fully
            # down, with no ports listening, print notwithstanding, until a
            # human noticed and ran 'launchctl bootstrap' by hand.
            plist_file="$HOME/Library/LaunchAgents/com.stapler-squad.plist"
            macos_stop_service "$plist_file"
            if ! macos_start_service "$plist_file"; then
                log_error "launchctl bootstrap and load both failed for the rolled-back binary."
                log_error "Try: launchctl bootstrap gui/$(id -u) $plist_file"
                return 1
            fi
            ;;
    esac

    # The restart call succeeding is necessary but not sufficient — verify the
    # rolled-back binary actually becomes healthy too before calling this a
    # successful rollback, rather than trusting launchctl/systemctl's exit
    # code alone (a job can register fine and still crash-loop immediately
    # after, e.g. the same port-race this script already guards against once).
    if wait_for_health "$HEALTH_TIMEOUT" "$prev_bin"; then
        log_success "Rolled back to previous build and confirmed healthy."
    else
        log_error "Rolled-back service did not become healthy either — service may be down."
    fi
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
