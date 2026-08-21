# Implementation Plan: System Service Auto-Start

**Source:** `project_plans/system-service-autostart/`
**Status:** Complete (Stories 1-4 implemented)
**Date:** 2026-04-12

---

## Epic Overview

### User Value

Users can install stapler-squad as a system service so it automatically starts on login without manual intervention. On Linux the app registers as a systemd user service; on macOS it registers as a LaunchAgent. Installation and removal are handled by a single `make install-service` target (or equivalent `ssq-hooks install service` subcommand) that detects the OS, writes the correct service file, and prints next-step instructions.

### Success Metrics

- `make install-service` succeeds on Linux (systemd user) and macOS (LaunchAgent) with zero manual file editing
- `make uninstall-service` cleanly removes the service file and disables auto-start
- After enabling, the service starts automatically on next login without any shell intervention
- Stdout/stderr from the process land in `~/.stapler-squad/logs/service.log`
- Binary path resolves correctly whether the user installed to `~/.local/bin/stapler-squad` or built in-place
- `make install-service` is idempotent: running it a second time overwrites the file without error

### Scope

**In scope:** systemd user service unit, macOS LaunchAgent plist, `scripts/install-service.sh` shell script, `make install-service` / `make uninstall-service` Makefile targets, optional `ssq-hooks install service` subcommand (adds service target alongside existing `gemini` and `open-code` targets), inline ADRs.

**Out of scope:** System-wide (root) service installation, Windows service (Task Scheduler), Docker/container deployment, service monitoring dashboards, auto-update on install, distribution packages (`.deb`, `.rpm`, Homebrew formula).

### Constraints

- Binary path must be resolved at install time from `which stapler-squad` or the current build output, not hard-coded
- No root/sudo required for either platform (systemd `--user` and `~/Library/LaunchAgents` are both user-scope)
- The script must be POSIX sh compatible (same as `install-mux.sh`)
- tmux must already be running or the service must set `TERM` and `PATH` correctly for tmux to start child sessions
- The LaunchAgent plist must not set `RunAtLoad` to `false` by default — the primary goal is auto-start
- Existing `make install` target (`go install .`) is unrelated and must not be modified

### Worktree Setup

```bash
git worktree add ../stapler-squad-service main -b feat/system-service-autostart
```

---

## Architecture Decisions

### ADR-001: Shell Script vs Go Subcommand for Service Installation

**Context:** Service installation requires writing files to OS-specific paths, running OS-specific CLI commands (`systemctl`, `launchctl`), and printing human-readable instructions. Two implementation strategies are available: (a) a POSIX shell script called from the Makefile, and (b) a Go subcommand added to `ssq-hooks install`.

**Decision:** Implement as a POSIX shell script (`scripts/install-service.sh`) with a Makefile wrapper (`make install-service`). Optionally expose the same logic as `ssq-hooks install service` by having the Go handler shell out to or replicate the script's logic.

**Rationale:**
- The existing `scripts/install-mux.sh` establishes a shell script pattern with helper functions (`log_info`, `log_success`, `log_warning`, `log_error`) that is well understood and easy to extend.
- Service file templating (embedding a binary path and log path) is trivial in shell with `printf` or heredoc; it does not justify a Go dependency.
- The shell script can be run directly without building the Go binary first, which matters during fresh installs where only the binary is copied from a release.
- `ssq-hooks install service` is additive: it can call `scripts/install-service.sh` or replicate its logic as a thin wrapper for users who prefer the Go CLI.

**Consequences:** The Makefile and shell script are the canonical install path. The `ssq-hooks` subcommand is optional and must stay in sync manually if logic changes.

---

### ADR-002: Service File Templates — Embedded vs Runtime-Generated

**Context:** The systemd unit and LaunchAgent plist must contain the resolved binary path, log directory path, and working directory. These values are user-specific and cannot be committed as static files.

**Decision:** Generate both files at install time inside `scripts/install-service.sh` using shell heredocs. Do not commit static template files. The script resolves the binary path via `which stapler-squad` with a fallback to the local `./stapler-squad` build artifact.

**Binary path resolution order:**
1. `STAPLER_SQUAD_BIN` environment variable (explicit override)
2. `which stapler-squad` (installed to PATH)
3. `$(pwd)/stapler-squad` (local build artifact)

**Rationale:** Static template files require a separate installation step to substitute variables. Shell heredocs produce the final file in one step and are self-documenting. The resolution order matches the precedence pattern used in `install-mux.sh` (`INSTALL_DIR` env var override).

**Consequences:** The generated file paths are correct at install time but not portable across machines or user renames. This is acceptable — service files are machine-local configuration, not version-controlled artifacts.

---

### ADR-003: systemd User Session vs System Session

**Context:** systemd offers two scopes: system services (running as root, active at boot before login) and user services (running as the logged-in user, activated at login). The application binds to `localhost:8543`, manages AI sessions under the user's home directory, and requires the user's tmux environment.

**Decision:** Use systemd user services (`~/.config/systemd/user/`) with `WantedBy=default.target`. Enable with `systemctl --user enable --now stapler-squad`. Linger (`loginctl enable-linger`) is documented as optional for users who want the service to survive logout.

**Rationale:** System services require root and cannot access user home directories, tmux user sockets, or user-installed tooling (claude, aider) without complex environment configuration. User services run as the correct user, inherit the user's `$HOME`, and require zero sudo.

**Consequences:** The service does not start at boot before first login. If the user logs out, the service stops unless linger is enabled. This is the correct trade-off for a developer tool.

---

## Story 1: Core Shell Script — Detect, Write, and Enable

**Goal:** A single shell script detects the OS, resolves the binary path, writes the appropriate service file to the correct OS-specific location, and prints instructions for enabling and starting the service.

**Acceptance Criteria (INVEST validated):**
- Script exits 0 on Linux (systemd user present) and macOS (LaunchAgent path exists)
- Script exits 1 with a descriptive error message when neither platform is detected
- Script writes the correct file to `~/.config/systemd/user/stapler-squad.service` on Linux
- Script writes the correct file to `~/Library/LaunchAgents/com.stapler-squad.plist` on macOS
- Script prints instructions for enabling and starting the service after writing the file
- Script is idempotent: re-running overwrites the file without error
- `STAPLER_SQUAD_BIN` environment variable overrides binary path detection

**Independent:** Does not require Go changes or Makefile changes (can be tested standalone).
**Negotiable:** Print-only mode (`--dry-run`) deferred to Story 3.
**Valuable:** Delivers the core user-facing feature end-to-end.
**Estimable:** 2-4 hours; follows `install-mux.sh` pattern exactly.
**Small:** Single file, under 250 lines.
**Testable:** Run on both platforms; check file content and exit code.

### Task 1.1: Create `scripts/install-service.sh` with OS detection

**File:** `scripts/install-service.sh`

Create the script skeleton with the helper functions from `install-mux.sh` and add OS detection:

```sh
#!/bin/sh
# install-service.sh — Install stapler-squad as a system service
# Supports: systemd user services (Linux), LaunchAgent (macOS)
set -e

# Colors
RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; BLUE='\033[0;34m'; NC='\033[0m'

log_info()    { printf "${BLUE}==>${NC} %s\n" "$1"; }
log_success() { printf "${GREEN}✓${NC} %s\n" "$1"; }
log_warning() { printf "${YELLOW}!${NC} %s\n" "$1"; }
log_error()   { printf "${RED}✗${NC} %s\n" "$1"; }

detect_os() {
    case "$(uname -s)" in
        Linux*)  echo "linux" ;;
        Darwin*) echo "macos" ;;
        *)       echo "unsupported" ;;
    esac
}
```

**Verification:** `sh scripts/install-service.sh` on Linux prints "linux", macOS prints "macos", other prints "unsupported" and exits 1.

### Task 1.2: Implement binary path resolution

**File:** `scripts/install-service.sh`

Add the `resolve_binary` function after `detect_os`:

```sh
resolve_binary() {
    # Priority: explicit env var > which > local build
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
    log_info "Options:"
    log_info "  1. Run 'make build' then re-run this script from the project root"
    log_info "  2. Run 'make install' to install to GOPATH/bin, then re-run"
    log_info "  3. Set STAPLER_SQUAD_BIN=/path/to/binary and re-run"
    exit 1
}
```

**Verification:** With binary in PATH, prints correct path. With `STAPLER_SQUAD_BIN=/custom/path` pointing to a non-existent file, exits 1 with error.

### Task 1.3: Write systemd user service file for Linux

**File:** `scripts/install-service.sh`

Add `install_linux` function:

```sh
install_linux() {
    bin_path="$1"
    service_dir="$HOME/.config/systemd/user"
    service_file="$service_dir/stapler-squad.service"
    log_dir="$HOME/.stapler-squad/logs"

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
ExecStart=$bin_path
WorkingDirectory=$HOME
Restart=on-failure
RestartSec=5s
StandardOutput=append:$log_dir/service.log
StandardError=append:$log_dir/service.log
Environment=HOME=$HOME
Environment=PATH=$PATH

[Install]
WantedBy=default.target
EOF

    log_success "Service file written to: $service_file"
    echo ""
    log_info "Enable and start now:"
    echo "    systemctl --user daemon-reload"
    echo "    systemctl --user enable --now stapler-squad"
    echo ""
    log_info "Check status:"
    echo "    systemctl --user status stapler-squad"
    echo ""
    log_info "Optional — persist service across logout (requires sudo once):"
    echo "    loginctl enable-linger \$USER"
    echo ""
}
```

**Verification:** On Linux, the generated `.service` file validates with `systemd-analyze verify ~/.config/systemd/user/stapler-squad.service`.

### Task 1.4: Write LaunchAgent plist for macOS

**File:** `scripts/install-service.sh`

Add `install_macos` function:

```sh
install_macos() {
    bin_path="$1"
    plist_dir="$HOME/Library/LaunchAgents"
    plist_file="$plist_dir/com.stapler-squad.plist"
    log_dir="$HOME/.stapler-squad/logs"

    log_info "Creating macOS LaunchAgent..."

    mkdir -p "$plist_dir"
    mkdir -p "$log_dir"

    cat > "$plist_file" << EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN"
    "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.stapler-squad</string>

    <key>ProgramArguments</key>
    <array>
        <string>$bin_path</string>
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
        <string>$PATH</string>
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
    log_info "Load and start now:"
    echo "    launchctl load -w $plist_file"
    echo ""
    log_info "Check status:"
    echo "    launchctl list | grep stapler-squad"
    echo ""
    log_info "On macOS 13+ (Ventura and later), you may also use:"
    echo "    launchctl bootstrap gui/\$(id -u) $plist_file"
    echo ""
}
```

**Verification:** On macOS, `plutil -lint ~/Library/LaunchAgents/com.stapler-squad.plist` exits 0. The plist `Label` matches the file name prefix.

### Task 1.5: Wire `main` function with OS dispatch and `--uninstall` flag

**File:** `scripts/install-service.sh`

```sh
UNINSTALL=0
for arg in "$@"; do
    case "$arg" in
        --uninstall) UNINSTALL=1 ;;
    esac
done

main() {
    os=$(detect_os)

    if [ "$os" = "unsupported" ]; then
        log_error "Unsupported platform: $(uname -s)"
        log_info "Supported platforms: Linux (systemd), macOS (LaunchAgent)"
        exit 1
    fi

    if [ "$UNINSTALL" = "1" ]; then
        uninstall_service "$os"
        exit 0
    fi

    bin_path=$(resolve_binary)
    log_info "Using binary: $bin_path"

    case "$os" in
        linux) install_linux "$bin_path" ;;
        macos) install_macos "$bin_path" ;;
    esac
}

main "$@"
```

Make the script executable:

```bash
chmod +x scripts/install-service.sh
```

**Verification:** Running `scripts/install-service.sh` on each platform writes the correct file and exits 0.

### Story 1 Integration Checkpoint

- [ ] Script detects Linux, macOS, and exits 1 on unsupported platforms
- [ ] Binary resolution tries env var, `which`, local build in order
- [ ] `~/.config/systemd/user/stapler-squad.service` written on Linux with correct `ExecStart` path
- [ ] `~/Library/LaunchAgents/com.stapler-squad.plist` written on macOS with correct `ProgramArguments`
- [ ] Both service files reference `~/.stapler-squad/logs/service.log`
- [ ] Script is idempotent (re-running overwrites without error)
- [ ] Instructions printed to stdout are copy-paste-ready

---

## Story 2: Uninstall Support

**Goal:** A single `--uninstall` flag (or `make uninstall-service`) removes the service file and disables auto-start on both platforms.

**Acceptance Criteria:**
- `scripts/install-service.sh --uninstall` removes the service file and prints disable/stop commands
- `make uninstall-service` is a thin wrapper around `scripts/install-service.sh --uninstall`
- Uninstall exits 0 even if the service file does not exist (idempotent)
- Uninstall does not delete `~/.stapler-squad/logs/` or any user data

### Task 2.1: Implement `uninstall_service` function in the script

**File:** `scripts/install-service.sh`

Add `uninstall_service` function before `main`:

```sh
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
                systemctl --user daemon-reload
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
}
```

**Verification:** After install + uninstall, the service file is absent. Running uninstall a second time exits 0 with a warning.

### Story 2 Integration Checkpoint

- [ ] `--uninstall` flag recognized and dispatched correctly
- [ ] Service file removed on both platforms
- [ ] `systemctl --user daemon-reload` called after removing on Linux
- [ ] `launchctl unload` called before removing plist on macOS
- [ ] No user data deleted
- [ ] Idempotent (no error when file already absent)

---

## Story 3: Makefile Integration

**Goal:** Expose `make install-service` and `make uninstall-service` targets so the feature integrates with the existing development workflow documented in `CLAUDE.md`.

**Acceptance Criteria:**
- `make install-service` calls `scripts/install-service.sh`
- `make uninstall-service` calls `scripts/install-service.sh --uninstall`
- Both targets appear in `make help` output
- `make install-service` depends on `build` so the binary exists before path resolution
- Targets appear in the `.PHONY` declaration

### Task 3.1: Add targets to Makefile

**File:** `Makefile`

Add to the `.PHONY` line:

```makefile
.PHONY: ... install-service uninstall-service
```

Add the targets after the existing `install-mux` target (around line 133):

```makefile
install-service: build ## Install stapler-squad as a system service (systemd on Linux, LaunchAgent on macOS)
	@./scripts/install-service.sh

uninstall-service: ## Remove the system service and disable auto-start
	@./scripts/install-service.sh --uninstall
```

**Verification:** `make help | grep service` shows both targets with descriptions.

### Task 3.2: Add `STAPLER_SQUAD_BIN` passthrough in Makefile

**File:** `Makefile`

Allow users to override the binary path when the build output is not in the current directory:

```makefile
install-service: build ## Install stapler-squad as a system service (systemd on Linux, LaunchAgent on macOS)
	@STAPLER_SQUAD_BIN="$(CURDIR)/stapler-squad" ./scripts/install-service.sh
```

Setting `STAPLER_SQUAD_BIN` to the Makefile's build output path ensures the service always points at the freshly-built binary, not a stale PATH entry from a previous install. Users who want to point at a different binary can override with `make install-service STAPLER_SQUAD_BIN=/custom/path`.

**Verification:** `make install-service` installs the binary from the current build directory, not whatever `which stapler-squad` returns.

### Story 3 Integration Checkpoint

- [ ] `make install-service` builds then installs the service using the local binary
- [ ] `make uninstall-service` removes the service
- [ ] Both targets in `.PHONY`
- [ ] Both appear in `make help` output with descriptions
- [ ] `STAPLER_SQUAD_BIN` can be overridden on the command line

---

## Story 4: `ssq-hooks install service` Subcommand (Optional)

**Goal:** Users who prefer the Go CLI can run `ssq-hooks install service` instead of `make install-service`. This adds `service` as a target alongside the existing `gemini` and `open-code` targets in `cmd/ssq-hooks/main.go`.

**Acceptance Criteria:**
- `ssq-hooks install service` performs the same install as the shell script on both platforms
- `ssq-hooks install service --uninstall` removes the service
- `ssq-hooks install` usage message lists `service` as a valid target
- Implementation shares binary path resolution logic with the shell script via a common Go function

**Dependent on:** Story 1 (the service file content is defined there). The Go subcommand generates identical file content.

### Task 4.1: Add `installService` function to `cmd/ssq-hooks/main.go`

**File:** `cmd/ssq-hooks/main.go`

Add a new case to `handleInstall`:

```go
case "service":
    installService()
```

Add the `installService` function. It uses `runtime.GOOS` for OS detection, `os.Executable()` with a fallback to `exec.LookPath("stapler-squad")` for binary resolution, and `os.WriteFile` for file creation.

Key implementation notes:
- Use `os.MkdirAll` to create parent directories
- Use `os.UserHomeDir()` for `$HOME` resolution (consistent with existing code in the file)
- Use `os.Getenv("PATH")` to embed the current PATH in the service file
- Print the same instructions as the shell script after writing
- Accept `--uninstall` flag via `flag.FlagSet`

**Files affected:**
- `cmd/ssq-hooks/main.go`

### Task 4.2: Update `printUsage` and `handleInstall` usage message

**File:** `cmd/ssq-hooks/main.go`

Update the install subcommand usage:

```go
func handleInstall() {
    if len(os.Args) < 3 {
        fmt.Fprintln(os.Stderr, "Usage: ssq-hooks install <target>")
        fmt.Fprintln(os.Stderr, "Targets: gemini, open-code, service")
        os.Exit(1)
    }
    // ...
    case "service":
        installService()
    // ...
}
```

**Verification:** `ssq-hooks install` with no target prints updated usage including `service`.

### Story 4 Integration Checkpoint

- [ ] `ssq-hooks install service` writes the correct service file on both platforms
- [ ] `ssq-hooks install service --uninstall` removes the service file
- [ ] Usage message lists `service` as a valid target
- [ ] `go build ./cmd/ssq-hooks` succeeds with no lint errors
- [ ] File content is identical to what the shell script generates

---

## Known Issues — Proactive Bug Identification

### BUG-001: PATH in Service Environment Missing User Tooling [SEVERITY: High]

**Description:** Both systemd user services and LaunchAgents inherit a minimal PATH at login time — typically `/usr/bin:/bin:/usr/sbin:/sbin`. tmux, claude, aider, and user-installed Go binaries live in paths like `~/.local/bin`, `/usr/local/bin`, or `$(go env GOPATH)/bin` that are NOT in the login PATH. The stapler-squad process will start but will fail to find `tmux` when creating sessions, resulting in session creation errors with no visible explanation in the web UI.

**Mitigation:**
- Embed `$PATH` from the install-time environment in the service file (both ADR-002 and tasks 1.3 and 1.4 specify this)
- Document that users must re-run `make install-service` if they later install tools to new PATH locations
- Add a startup warning log in Go when `which tmux` fails at application start

**Files Affected:**
- `scripts/install-service.sh` (PATH embedding in heredoc)
- `cmd/ssq-hooks/main.go` (same for Go implementation)
- Potentially `main.go` (startup tmux check)

**Prevention:** Integration test: after service install, verify `systemctl --user status` shows `Active: active (running)` and the web UI `/health` endpoint returns 200 within 10 seconds of login.

---

### BUG-002: Binary Path Becomes Stale After `go install` or Rebuild [SEVERITY: Medium]

**Description:** The service file records the resolved binary path at install time. If the user later runs `go install .` and the binary moves to a different location (e.g., from `$(pwd)/stapler-squad` to `$(go env GOPATH)/bin/stapler-squad`), the service continues pointing to the old path. On Linux, if the old path no longer exists, the service will fail to start with `code=exited, status=203/EXEC`.

**Mitigation:**
- `make install-service` (Story 3 Task 3.2) always pins to `$(CURDIR)/stapler-squad`, making the path explicit and predictable
- Document in printed instructions: "If you move the binary, re-run `make install-service`"
- Add `RestartSec=5s` (Linux) and `ThrottleInterval` (macOS) to prevent rapid crash loops

**Files Affected:**
- `scripts/install-service.sh`
- `Makefile`

**Prevention:** After uninstall + reinstall from a new binary location, verify service starts from the correct binary via `systemctl --user show stapler-squad --property=ExecStart`.

---

### BUG-003: macOS LaunchAgent Fails on Upgrade to Ventura+ [SEVERITY: Medium]

**Description:** macOS 13 Ventura deprecated `launchctl load`/`launchctl unload` in favor of `launchctl bootstrap`/`launchctl bootout`. The old commands still work but emit deprecation warnings and may be removed in a future macOS release. Users on older macOS (10.15–12.x) do not have `launchctl bootstrap`.

**Mitigation:**
- Print both the old and new commands in the post-install instructions (Task 1.4 already does this)
- The plist format itself is identical across macOS versions — only the load command differs
- Do not attempt to `launchctl load` automatically from the script; let the user choose the correct command for their macOS version

**Files Affected:**
- `scripts/install-service.sh` (printed instructions in `install_macos`)

**Prevention:** Test on macOS 12 (Monterey) and macOS 14 (Sonoma) if available. If only one macOS version is available, test with both `launchctl load -w` and `launchctl bootstrap gui/$(id -u)`.

---

### BUG-004: systemd User Session Not Available in WSL or Minimal Linux Containers [SEVERITY: Low]

**Description:** `systemctl --user` requires `systemd` as PID 1 and the user session D-Bus socket to be active. On WSL1, systemd is not available. On WSL2, it may be available but requires explicit configuration. On minimal Docker/container environments, systemd is typically absent. The script will print confusing `systemctl: command not found` or `Failed to connect to bus` errors.

**Mitigation:**
- Before writing the service file on Linux, check that `systemctl --user status` exits without a "Failed to connect" error
- If systemd is not available, print a clear error message and suggest alternatives (`~/.profile`, `crontab @reboot`)

**Files Affected:**
- `scripts/install-service.sh` (`install_linux` function — add pre-check)

**Prevention:** Add a `check_systemd_user` helper that runs `systemctl --user is-system-running 2>/dev/null` and checks the exit code before writing the service file.

---

### BUG-005: Concurrent Service Instance if User Also Runs Manually [SEVERITY: Low]

**Description:** If the user runs `./stapler-squad` manually while the service is already running, two instances bind to port 8543. The second instance fails with `address already in use`, but the user sees a cryptic error with no indication that the service is already running.

**Mitigation:**
- The application already logs to `~/.stapler-squad/logs/stapler-squad.log` — the port bind error will be visible there
- Document in printed instructions: "If the server is already running manually, stop it first with `pkill -f stapler-squad`"
- This is a user-education issue, not a code bug; no code change required for MVP

**Files Affected:**
- Printed instructions in `scripts/install-service.sh`

**Prevention:** Consider adding a `--pid-file` flag to the application in a future iteration so systemd/launchctl can detect stale instances.

---

### BUG-006: Service Process No Longer Inherits Shell-RC-Exported Secrets (ANTHROPIC_API_KEY, GITHUB_TOKEN) [SEVERITY: Medium]

**Status:** Deferred — not implemented this session

**Description:** The launchd plist and systemd unit generated by `install_macos()`/`install_linux()` used to source the user's `~/.zshrc` at service start, which caused non-deterministic startup (interactive-shell hazards: prompts, `stty` calls, slow plugin init, hangs without a TTY). That sourcing was removed in commit `baca1c7c2`, replacing it with a direct `Program`/`ProgramArguments` (macOS) / `ExecStart` (Linux) invocation of the binary, with `PATH`/`HOME` supplied explicitly via `EnvironmentVariables`/`Environment=`. This was the correct fix for the startup bug, but `.zshrc` sourcing was incidentally the only path by which shell-rc-exported secrets reached the service process — removing it also silently dropped that inheritance. Spawned `claude`/`aider` sessions inside the service's tmux panes now inherit only the minimal `HOME`/`PATH` baked into the plist/unit. Impact is conditional, not universal: `GITHUB_TOKEN` is mostly de-risked by the `gh auth token` fallback in `github/http_client.go:31-58`; `ANTHROPIC_API_KEY` has no equivalent fallback and only affects users who authenticate via a raw API key exported in shell rc (not `claude login` OAuth users).

**Possible Future Mitigations (not implemented):**
- `EnvironmentFile=` (systemd `[Service]` directive) pointing at a `~/.stapler-squad/env` dotenv file — native, no shell interpreter invoked, re-read at every service restart.
- Install-time inlining of the same `~/.stapler-squad/env` file's contents into the macOS plist's `EnvironmentVariables` dict — the same generation-time pattern already used for `PATH`/`HOME` today, since launchd has no file-include directive analogous to `EnvironmentFile=`.
- A `/bin/sh` wrapper that sources an env file at service start was considered and explicitly rejected: invoked via `ProgramArguments`/`ExecStart`, it reintroduces a shell process at service-start time — the same category of thing this bug's fix removed — even though POSIX `.` of a flat `KEY=VALUE` file has none of `.zshrc`'s interactive hazards.
- Either mitigation requires `0600` permissions on the env file (and the plist, once it can contain secrets) as a prerequisite — not automatic today.

**Files Affected:**
- `scripts/install-service.sh` (`install_macos()`, `install_linux()` — no fix implemented)
- `config/config.go`, `server/services/session_service.go` (current env read sites, unaffected by this gap but relevant to a future fix)

**Prevention:** Once a fix ships, add a regression test verifying a service-spawned session's environment matches the contents of a `~/.stapler-squad/env` file.

---

## Dependency Visualization

```
Task 1.1 (script skeleton + OS detection)
  |
  +-- Task 1.2 (binary resolution)
  |     |
  |     +-- Task 1.3 (Linux service file)  --+
  |     |                                     |
  |     +-- Task 1.4 (macOS plist)         --+-- Task 1.5 (main + --uninstall flag)
  |                                                       |
  +-- Story 2 (uninstall_service function)  <------------+
  |     |
  |     +-- Task 2.1 (uninstall logic)
  |
  +-- Story 3 (Makefile targets)  <-- depends on Story 1 complete
  |     |
  |     +-- Task 3.1 (add targets)
  |     +-- Task 3.2 (STAPLER_SQUAD_BIN passthrough)
  |
  +-- Story 4 (ssq-hooks subcommand)  <-- optional, depends on Story 1 content
        |
        +-- Task 4.1 (installService Go function)
        +-- Task 4.2 (usage message update)
```

**Critical path:** 1.1 → 1.2 → 1.3/1.4 → 1.5 → Story 3

**Parallelizable after Task 1.2:**
- Task 1.3 (Linux) and Task 1.4 (macOS) are independent of each other
- Story 4 (ssq-hooks) can be implemented in parallel with Story 3 (Makefile)

---

## Integration Checkpoints

### After Story 1 (Shell Script)

On Linux:
1. Run `scripts/install-service.sh`
2. Verify `~/.config/systemd/user/stapler-squad.service` exists and contains correct `ExecStart`
3. Run `systemctl --user daemon-reload && systemctl --user enable --now stapler-squad`
4. Verify `systemctl --user status stapler-squad` shows `Active: active (running)`
5. Verify `curl -s http://localhost:8543/` returns a response
6. Verify `~/.stapler-squad/logs/service.log` is being written

On macOS:
1. Run `scripts/install-service.sh`
2. Verify `~/Library/LaunchAgents/com.stapler-squad.plist` exists and passes `plutil -lint`
3. Run `launchctl load -w ~/Library/LaunchAgents/com.stapler-squad.plist`
4. Verify `launchctl list | grep stapler-squad` shows a PID (not `-`)
5. Verify `curl -s http://localhost:8543/` returns a response
6. Verify `~/.stapler-squad/logs/service.log` is being written

### After Story 2 (Uninstall)

1. Run `scripts/install-service.sh --uninstall`
2. Verify the service file is removed
3. Verify the service is stopped and disabled
4. Verify `~/.stapler-squad/logs/` still exists and log files are intact
5. Run uninstall a second time — verify it exits 0 with a warning

### After Story 3 (Makefile)

1. `make help | grep service` shows both targets
2. `make install-service` builds the binary then writes the service file
3. `make uninstall-service` removes the service file
4. `STAPLER_SQUAD_BIN=/custom/path make install-service` uses the override path

### After Story 4 (ssq-hooks, optional)

1. `ssq-hooks install` prints usage listing `service` as a target
2. `ssq-hooks install service` writes the same file as the shell script
3. `ssq-hooks install service --uninstall` removes the file
4. `go build ./cmd/ssq-hooks` succeeds with no lint errors

---

## Context Preparation Guide

### Files to Read Before Implementation

**Must read:**
- `scripts/install-mux.sh` — POSIX sh patterns, helper functions, and installation flow to replicate
- `cmd/ssq-hooks/main.go` — existing `handleInstall` structure for Story 4
- `Makefile` lines 124–133 — `install` and `install-mux` targets to match style

**Reference:**
- `main.go` lines 1–50 — binary flags and package imports for context on binary invocation
- `docs/tasks/mobile-ux-improvements.md` — format reference for this document

### Key Platform Facts for Implementer

**Linux (systemd user):**
- Service files go in `~/.config/systemd/user/`, not `/etc/systemd/system/`
- `WantedBy=default.target` is the correct target for user services (not `multi-user.target`)
- `daemon-reload` must be called after writing or modifying a service file
- `loginctl enable-linger $USER` is required to keep the service running after the user logs out
- `StandardOutput=append:path` (not `file:path`) is needed to append rather than truncate on restart

**macOS (LaunchAgent):**
- Plists go in `~/Library/LaunchAgents/`, not `/Library/LaunchAgents/` (which requires root)
- `RunAtLoad: true` starts the agent immediately when loaded, not just at next login
- `KeepAlive.SuccessfulExit: false` restarts on crash but not on clean exit (user-initiated `Ctrl+C`)
- `ThrottleInterval: 5` prevents rapid restart loops on crash
- `launchctl load -w path` is the pre-Ventura command; `launchctl bootstrap gui/$(id -u) path` is Ventura+
- The `Label` key in the plist must match the filename prefix: `com.stapler-squad` for `com.stapler-squad.plist`
