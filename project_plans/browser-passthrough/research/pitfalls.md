# Browser Passthrough — Pitfalls Research

## 1. VNC Security Pitfalls

### 1.1 VNC Password Is Limited to 8 Characters (RFB Protocol Hard Limit)
The RFB 3.x protocol truncates passwords to 8 bytes in its DES-based challenge-response auth (VNC Authentication type 2). Any password longer than 8 characters silently works as if truncated — the extra characters are ignored. This means a 32-character random token is effectively reduced to 8 characters of entropy (~48 bits at best, often less due to DES key schedule weaknesses).

**Mitigation**: Do not rely on x11vnc's `-passwd` flag for security. Instead:
- Launch x11vnc with `-nopw` (no VNC-level auth) and rely entirely on the authenticated Go WebSocket proxy as the auth layer (see NFR-2).
- Alternatively, use `-rfbauth <htpasswd-file>` combined with a uniquely generated 8-char random token — but document the 8-char ceiling explicitly.
- The **recommended design**: x11vnc binds only to 127.0.0.1 (per NFR-2), VNC port is never reachable without going through the Go proxy, and the proxy validates the stapler-squad session cookie before upgrading the WebSocket. VNC-level auth becomes defense-in-depth only.

### 1.2 VNC Port Collision Between Sessions
If two sessions are assigned the same localhost VNC port (e.g., both try `:5900 + session_index` using a naive index), x11vnc on the second session will silently fail to bind and exit. The Go layer may not detect this if it only checks process liveness after a short sleep.

**Mitigation**:
- Allocate VNC ports by asking the OS: open a `net.Listen("tcp", "127.0.0.1:0")` listener, record the port, close the listener, then pass that port to x11vnc via `-rfbport <port>`. There is a TOCTOU race but it is very narrow on localhost.
- Alternatively, reserve a port range in config (e.g., 5900–5999) and track allocations in a Go map keyed by session UUID, with a port-release step in session teardown.
- Check x11vnc's stdout/stderr for "bind: Address already in use" in the first 2 seconds; treat this as a hard startup error and retry with a different port.

### 1.3 x11vnc Left Running After Session Cleanup Failure
If the Go session teardown goroutine panics, returns early on error, or is killed mid-flight, x11vnc and Xvfb can be left as orphans. On the next stapler-squad startup they will not be associated with any session and will hold onto their display numbers and ports indefinitely.

**Mitigation** (borrowing from the existing `ManagedProcess` model):
- Use `StartProcess()` from `executor/managed_process.go` (with `Setpgid: true`) to launch x11vnc. This places it in its own process group. On session close, call `ManagedProcess.Stop()` which sends SIGTERM then SIGKILL to the entire process group.
- On stapler-squad startup, scan `/tmp/.X<N>-lock` files and reconcile against the session store. Any lock file whose owner PID does not match a live session should trigger cleanup (kill the PID, remove the lock file, then start fresh).
- Register the Xvfb PID in the session's persisted state so that crash recovery after a Go restart can find and kill it.

---

## 2. Xvfb Pitfalls

### 2.1 Zombie Xvfb Processes / Stale `.X-lock` Files
Xvfb creates `/tmp/.X<N>-lock` and `/tmp/.X11-unix/X<N>` on startup. If Xvfb is killed with SIGKILL (or crashes) without cleanup, the lock file and socket remain. Any subsequent attempt to start a new Xvfb on the same display number fails immediately with "Server is already active for display :<N>".

**Mitigation**:
- Use display numbers in a reserved range (e.g., `:100 + session_slot`), not `:0` or `:1`, to avoid colliding with host displays.
- On session startup, check whether `/tmp/.X<N>-lock` exists. If the PID in the lock file is dead, remove both the lock file and the socket, then proceed.
- Implement a `CleanupStaleDisplay(n int)` helper that wraps this logic, called before each Xvfb launch.
- Use `Xvfb :<N> -nolisten tcp` (disables TCP listening, UNIX socket only) to reduce the attack surface.

### 2.2 Display Number Exhaustion
With many concurrent sessions each consuming a display number, the range can be exhausted. Linux supports up to display `:255` by default (though higher numbers work in practice up to `32767`).

**Mitigation**:
- Maintain a pool of display numbers in a Go channel or sync.Map. Return numbers to the pool on session teardown.
- Use a range large enough for anticipated concurrency (e.g., `:100`–`:200` for up to 100 sessions).
- Emit a warning metric/log when the pool drops below 10% capacity.

### 2.3 Memory Growth in Long-Running Xvfb Instances
Xvfb accumulates pixmap memory for each window/widget that is created and never destroyed within the session (e.g., a long-running Chrome with many tabs opened and closed). On GPU-less software rendering, there is no hardware memory reclaim. Over hours, RSS can grow well past the 50 MB idle budget.

**Mitigation**:
- Configure Xvfb with the minimum framebuffer depth needed: `-screen 0 1280x800x24` (24-bit is sufficient; 32-bit wastes memory).
- Monitor Xvfb RSS in the health-check goroutine; if it exceeds a configurable threshold (default 200 MB), log a warning and optionally restart x11vnc (which survives Xvfb restart if Xvfb is restarted with the same display number and screen geometry — but this is complex; log-only is safer for v1).
- Document that sessions running browsers for >8 hours should expect elevated Xvfb memory.

### 2.4 Xvfb Not Available in Docker / Minimal Containers
Many Docker base images (Alpine, distroless, slim Debian) do not include Xvfb. The `xvfb-run` wrapper script is also absent.

**Mitigation**:
- Check for Xvfb availability at session-start time using `exec.LookPath("Xvfb")`. If absent, set a flag that suppresses the Browser tab for that session (NFR-4 graceful degradation).
- Document required packages: `xvfb x11vnc xdotool` on Debian/Ubuntu; `xorg-server-Xvfb tigervnc-standalone-server xdotool` on Alpine/Arch.
- Provide a Dockerfile snippet in the README for users running stapler-squad in containers.

---

## 3. xdotool / Window Tracking Pitfalls

### 3.1 Chrome Opens Multiple Windows / Which Window to Track
When Chrome starts it may open multiple windows: the main browser window plus a crash-recovery dialog, an update notification, or a detached DevTools panel. `xdotool search --name "Google Chrome"` returns all matching windows. Picking the wrong one crops to the wrong area.

**Mitigation**:
- Use `xdotool search --class "google-chrome" --sync --maxdepth 2` (class-based search is more specific than title-based).
- If multiple windows match, prefer the window with the largest area (get geometry with `xdotool getwindowgeometry`) — the main browser window is almost always the largest.
- Poll every 500ms for the first 10 seconds after Chrome launch before giving up (handles the race condition where Chrome takes several seconds to show the first window).

### 3.2 Race Between Browser Launch and First xdotool Poll
After the agent runs `google-chrome`, xdotool may run before Chrome's window appears on the display. `xdotool search` returns an empty set, and the Browser tab shows the placeholder. A naive implementation may then stop polling.

**Mitigation**:
- Poll with exponential backoff: 100ms, 200ms, 400ms … up to 5s intervals, for up to 30s total.
- Use `xdotool search --sync` if available (blocks until at least one window matches, with a timeout).
- If the window ID was previously tracked and is now absent, switch back to the placeholder but continue polling (user may have closed and reopened the browser).

### 3.3 Window ID Changes After Chrome Restart or Update
Chrome's window ID (X11 Window resource ID) changes every time Chrome restarts. A cached window ID becomes stale. Passing a stale window ID to x11vnc's `-id` flag causes x11vnc to exit immediately with an error.

**Mitigation**:
- Do not pass the window ID to x11vnc at startup time. Instead, use x11vnc's full-display mode (`-display :<N>`) and crop in the noVNC/proxy layer using xdotool-provided geometry updated dynamically.
- Alternatively, implement a window-tracker goroutine that re-polls xdotool every 2s and updates the clip geometry by sending a SIGHUP or dynamic reconfiguration command to x11vnc if the window ID changes.

### 3.4 DISPLAY-less xdotool Invocations
xdotool requires `DISPLAY` to be set to the same virtual display as the application. Invoking xdotool without `DISPLAY=:<N>` falls back to the host's `$DISPLAY` (e.g., `:0`), finding completely different windows or failing entirely.

**Mitigation**:
- Always pass `DISPLAY=:<N>` explicitly in the environment when invoking xdotool (use `WithProcessEnv("DISPLAY", ":"+strconv.Itoa(displayNum))` via the `executor` package).
- Verify in CI/integration tests that xdotool is invoked with the correct DISPLAY by inspecting the command environment.

---

## 4. noVNC / WebSocket Proxy Pitfalls

### 4.1 Go Goroutine Leak if Proxy Is Not Cancelled When WebSocket Closes
The VNC proxy will run two `io.Copy` goroutines (WebSocket→TCP and TCP→WebSocket). If the WebSocket closes and only one direction's goroutine detects the error, the other goroutine may block on a read from the still-open TCP connection to x11vnc, leaking it indefinitely.

This is a well-known pattern in Go proxy code. The existing `connectrpc_websocket.go` uses `doneChan` + `select` idioms to coordinate goroutine teardown (lines ~675–984). The VNC proxy must follow the same pattern.

**Mitigation**:
- Use a shared `context.Context` (derived from the HTTP request context, which is cancelled when the WebSocket closes) for both directions.
- When either direction returns an error, cancel the context and close both the WebSocket conn and the TCP connection. The other goroutine will unblock immediately.
- Use `sync.WaitGroup` to wait for both goroutines before returning from the handler.

**Recommended pattern** (matching existing codebase conventions):
```go
ctx, cancel := context.WithCancel(r.Context())
defer cancel()
tcpConn, err := net.Dial("tcp", vncAddr)
// ...
var wg sync.WaitGroup
wg.Add(2)
go func() { defer wg.Done(); defer cancel(); io.Copy(tcpConn, wsReader) }()
go func() { defer wg.Done(); defer cancel(); io.Copy(wsWriter, tcpConn) }()
wg.Wait()
```

### 4.2 noVNC Clipboard Sync Leaks Host Clipboard Data
noVNC's ServerCutText extension syncs the VNC server's clipboard (which is the host's X11 clipboard) to the browser's clipboard API, and vice versa. This means a user copy-pasting in the browser can leak clipboard content to other noVNC sessions or expose host clipboard data to the browser.

**Mitigation (per requirements: clipboard sync is out of scope for v1)**:
- Disable clipboard sync in the noVNC client initialization: `rfb.clipViewport = false` and do not send `clipboardPasteFrom` events.
- On the x11vnc side, pass `-noclipboard` to x11vnc to suppress X11 clipboard synchronization entirely.

### 4.3 TLS / WSS Handling for the Proxy Endpoint
noVNC requires that if the page is served over HTTPS, the WebSocket must use WSS. The Go proxy for VNC must be accessible as WSS (not WS) when the main stapler-squad UI is served over TLS.

**Mitigation**:
- The VNC WebSocket proxy endpoint lives under the same Go HTTP server as the rest of the UI. If TLS is configured on that server (which it is for remote/LAN access), the endpoint is automatically WSS. No separate handling is needed.
- The internal TCP connection from the Go proxy to x11vnc remains plain TCP on localhost — there is no need for TLS there.

### 4.4 Input Event Encoding Differences Between noVNC Versions
Different versions of noVNC encode keyboard events differently (key codes vs. key symbols, modifier handling). If noVNC is vendored at one version and later updated, keyboard input may break silently.

**Mitigation**:
- Vendor noVNC at a pinned version (e.g., a git submodule or specific npm package version).
- Document the pinned version in `package.json` or the Go embed directive.
- Add a basic E2E test that sends a key event and verifies it appears in the virtual display (via xdotool or screenshot comparison).

---

## 5. macOS Pitfalls

### 5.1 Screen Recording Permission Cannot Be Granted Programmatically
On macOS 10.15+, accessing screen content (for VNC server use) requires the `com.apple.screencaptured` TCC entitlement to be granted interactively by the user in System Preferences → Privacy & Security → Screen Recording. The application cannot grant this itself.

**Mitigation**:
- Detect the TCC permission at startup (use `CGWindowListCopyWindowInfo` in a canary call; if it returns empty results for on-screen windows, the permission is likely missing).
- Show a clear one-time setup prompt in the UI: "Screen Recording permission required for Browser Passthrough. Please grant it in System Preferences."
- Do not attempt VNC server startup on macOS until the permission is confirmed.
- Use `open "x-apple.systempreferences:com.apple.preference.security?Privacy_ScreenCapture"` to open the correct pane directly.

### 5.2 macOS ARD/VNC Uses a Different Auth Flow
macOS's built-in Screen Sharing uses Apple Remote Desktop protocol on top of RFB. The ARD auth handshake (auth type 30) is incompatible with standard x11vnc `-passwd`. Additionally, `screensharingd` uses system user credentials by default, not a custom VNC password.

**Mitigation**:
- For v1 macOS support, use a third-party VNC server that supports standard RFB auth, such as `librfb`-based solutions or a Go-native RFB server library (e.g., `github.com/nicowillis/go-vnc`).
- Alternatively, deploy a secondary lightweight VNC server (e.g., TigerVNC's `x0vncserver` equivalent for macOS) rather than relying on System Preferences Screen Sharing.
- Document macOS support as "experimental" in v1.

### 5.3 `screensharingd` May Not Run on Headless Servers
On macOS servers without a logged-in GUI session (common in CI/cloud), `screensharingd` may not be loaded by launchd. Remote management VNC also requires the AppleVNCServer bundle to be active.

**Mitigation**:
- On macOS, check that the session's display is a real display (not a virtual one) before attempting VNC. The concept of "Xvfb display" does not apply.
- For headless macOS, `screencapture -l <windowID>` (CGWindowListCreateImage) can be used for screenshot-based polling instead of a live VNC stream. This is a significant architectural difference from Linux and should be called out as a v2 concern.

---

## 6. Resource Leak Pitfalls

### 6.1 Go Goroutine Leak — Proxy Not Cancelled on WebSocket Close
See §4.1. This is the single highest-priority resource leak risk.

### 6.2 OS File Descriptor Leak — VNC TCP Connection Not Closed
If the `net.Conn` to x11vnc is not closed when the WebSocket terminates, the file descriptor remains open. With many sessions and many reconnects, FD limits can be exhausted. Go's garbage collector will eventually close the connection when the `net.Conn` is GC'd, but this is non-deterministic and not timely.

**Mitigation**:
- Always `defer tcpConn.Close()` immediately after a successful `net.Dial` in the proxy handler.
- Use `context`-aware dial: `(&net.Dialer{}).DialContext(ctx, "tcp", vncAddr)` so that context cancellation closes the connection.

### 6.3 Process Group Management — Xvfb / x11vnc Must Die with Go Process
If stapler-squad is killed with SIGKILL (no graceful shutdown), Xvfb and x11vnc processes it spawned survive as orphans. On the next startup they may hold stale display numbers and ports.

**Mitigation**:
- Use `ManagedProcess` from `executor/managed_process.go` (which already sets `Setpgid: true` and sends SIGKILL to the process group in `Stop()`). **Do not use raw `exec.Command` for Xvfb or x11vnc.**
- On Linux, the existing `SetSubreaper()` + `StartZombieReaper()` infrastructure in `session/tmux/` provides a fallback reaper for orphaned grandchildren. Xvfb and x11vnc spawned by `ManagedProcess` with `Setpgid: true` will be in their own process groups, so the subreaper will collect their zombies.
- Persist VNC process PIDs to the session state (same pattern as `TmuxSessionName` stored in `ExternalMetadata`). On startup, kill any PIDs from the previous run that are still alive.
- Note: `Pdeathsig` (Linux-only, `PR_SET_PDEATHSIG`) would kill child processes when the parent dies, but it does not work when the parent is killed with SIGKILL (the kernel delivers pdeathsig only when the parent exits, not when it is SIGKILLed). Process groups with an explicit `Stop()` path is the correct approach.

---

## 7. Lessons from the Existing Codebase

### 7.1 Use `ManagedProcess` — Not Raw `exec.Cmd`
The `executor` package (`executor/managed_process.go`) provides a lifecycle-managed subprocess with:
- `Setpgid: true` by default — child is in its own process group
- `Stop()` that sends SIGTERM to the process group, waits `gracePeriod` (5s), then SIGKILLs the group
- A finalizer that SIGKILLs on GC if `Stop()` was never called
- `IsAlive()` for health checks without re-forking

Xvfb and x11vnc MUST use `StartProcess()`, not raw `exec.Command`. This directly satisfies FR-9 (crash recovery) and NFR-4 (cleanup on session close).

### 7.2 The `TmuxProcessManager` Pattern — Adapt for VNC
The existing `TmuxProcessManager` struct (`session/tmux_process_manager.go`) provides the template for a `VNCProcessManager`:
- Owns the Xvfb `*ManagedProcess` and the x11vnc `*ManagedProcess`
- Exposes `HasDisplay() bool`, `IsAlive() bool`, `Close() error` methods
- Is nil-safe (guard every method with a `if pm.xvfb == nil { return }` check)
- Is mockable via an interface for unit tests (same pattern as `TmuxManager` interface)

### 7.3 Crash-Loop Detection Already Exists — Reuse It
`instance_tmux.go` has `trackRestartRate()` (lines 163–188) that detects >5 restarts in 5 minutes. The x11vnc restart logic (FR-9: "up to 3 attempts") should reuse or adapt the same pattern rather than implementing ad-hoc retry logic.

### 7.4 Context Cancellation for Goroutine Teardown — Match Existing Patterns
`connectrpc_websocket.go` (lines 675–984) coordinates goroutine teardown via a `doneChan` and a `context.WithCancel` derived from the request context. The VNC WebSocket proxy handler MUST follow the same idiom to avoid the goroutine leak described in §4.1.

### 7.5 Circuit Breaker — Prevent Hot Loops on Persistent Failures
The tmux layer (`session/tmux/server_registry.go`) uses circuit breakers to prevent hot-looping when the tmux server is down. If x11vnc persistently fails to start (e.g., Xvfb died and its lock file is stale), a naive restart loop will spin at 100% CPU. The VNC manager should use either:
- An exponential backoff (100ms, 200ms, 400ms … cap at 30s)
- Or the existing `executor` circuit breaker if it is exposed

---

## Priority Summary

**Highest-priority pitfalls to design against:**

1. **Process group orphaning on shutdown/crash** — Xvfb and x11vnc must be launched via `ManagedProcess` (with `Setpgid: true`) so that `Stop()` delivers SIGKILL to the entire process group. Raw `exec.Cmd` leaves zombies holding stale display locks and VNC ports across restarts. On startup, reconcile persisted PIDs against live processes and kill any orphans. This is the single most operationally damaging failure mode.

2. **Go goroutine / FD leak in the WebSocket-to-VNC proxy** — the proxy bidirectional copy requires a shared `context.Context` that is cancelled when either direction errors, plus `defer tcpConn.Close()` immediately after dial. Without this, each browser tab reconnect leaks two goroutines and two file descriptors permanently, exhausting OS resources after ~thousands of reconnects. Match the `doneChan`/`cancel` pattern in `connectrpc_websocket.go`.

3. **VNC security via proxy-only authentication (not x11vnc password)** — the 8-character RFB password limit makes x11vnc's native auth inadequate. The correct design is `x11vnc -nopw` bound to `127.0.0.1` only, with all access brokered through the Go WebSocket proxy that validates the stapler-squad session cookie. Any VNC port exposure to non-localhost (even with a "password") is a security regression.
