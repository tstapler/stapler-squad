# Browser Passthrough — Architecture Research

## 1. Existing Session Lifecycle

### How Sessions Are Created, Tracked, and Destroyed

The core entity is `session.Instance` (`session/instance.go`). Key lifecycle methods:

- **Construction**: `NewInstance(InstanceOptions)` — validates, path-expands, populates struct fields. Does not touch tmux.
- **Start**: `Instance.start(firstTimeSetup bool, …)` — the critical path:
  1. `initTmuxSession()` creates a `tmux.TmuxSession` object and calls `SetSession()` on `TmuxProcessManager`.
  2. Calls `i.tmuxManager.Start(startPath)` which runs:
     ```
     tmux new-session -d -s <name> -e CLAUDECODE= -c <workDir> <program>
     ```
     Note the `-e KEY=VALUE` syntax: tmux supports injecting environment variables into the child session this way.
  3. Attaches a PTY via `RestoreWithWorkDir`, then calls `StartController`.
  4. On success, fires `EventStarted` to all `LifecycleListener` subscribers.
- **Destroy**: `Instance.Destroy()` — calls `StopController()`, `KillSession()` (kills tmux session), `CleanupWorktree()` (removes git worktree).
- **Pause/Resume**: detaches from tmux or removes worktree while keeping branch; resumes by recreating worktree + restarting tmux.

All session state is persisted through `session.Storage` (Ent-backed SQLite). Live in-memory instances are held by `ReviewQueuePoller` and accessed by the server without touching storage.

### Where VNC Server Lifecycle Should Hook In

The cleanest hook points are:

1. **After `tmuxManager.Start()`** in `Instance.start()` — once the tmux session is alive and the working directory is established, Xvfb and x11vnc can be launched.
2. **Before/inside `Instance.Destroy()`** — before `KillSession()`, kill the x11vnc and Xvfb processes.
3. **On restart** — the `onExitCallback` fires `EventExited` → a new call to `start()` is made; VNC processes need to be killed and restarted there too.

The recommended structure is a new `VNCProcessManager` struct (analogous to `TmuxProcessManager`) that owns Xvfb and x11vnc process handles, and is called from the same lifecycle entry points.

### `session/instance.go` Summary

`Instance` is a large struct (~280 fields) that owns:
- `tmuxManager TmuxProcessManager` — wraps a `*tmux.TmuxSession`
- `gitManager GitWorktreeManager` — wraps git worktree operations
- `controllerManager ControllerManager` — owns `ClaudeController` and `InstanceStatusManager`
- A set of mutex-protected callback fields (`onStatusChange`, `onRateLimitDetected`, `onRateLimitRecovery`)
- `LifecycleListener` list — receives `EventStarted` / `EventExited` notifications

`SessionType` is a string enum: `"directory"`, `"new_worktree"`, `"existing_worktree"`, `"new_project"`.

---

## 2. Go Server Structure

### WebSocket Endpoints

The server uses `github.com/gorilla/websocket` throughout. Currently registered WebSocket/streaming paths:

| Path | Handler | Purpose |
|---|---|---|
| `/api` + StreamTerminalProcedure | `ConnectRPCWebSocketHandler.HandleWebSocket` | Bidirectional terminal I/O (PTY ↔ browser) |
| `/api` + WatchSessionsProcedure | `StreamingWSBridge` | Server-streaming session events |
| `/api` + WatchReviewQueueProcedure | `StreamingWSBridge` | Server-streaming queue events |
| `/api/external/approvals` | `ExternalWebSocketHandler` | External session approval flow |

The `ConnectRPCWebSocketHandler` (`server/services/connectrpc_websocket.go`) is the primary pattern to follow: it uses `websocket.Upgrader` with `isAllowedOrigin` for origin enforcement, and authenticates via the auth middleware layer.

### Where `/api/sessions/{id}/vnc` Should Live

This endpoint is a raw TCP tunnel (VNC bytes in both directions), not a ConnectRPC call. It should be a standalone handler registered directly on `srv.mux`:

```go
// In wireDepsIntoServer():
vncHandler := services.NewVNCProxyHandler(deps.VNCManager)
srv.mux.HandleFunc("/api/sessions/{id}/vnc", vncHandler.HandleWebSocket)
```

`{id}` is a Go 1.22+ pattern-based path variable (the project already uses Go 1.25). The handler extracts the session ID via `r.PathValue("id")`, looks up the per-session VNC port from the `VNCManager`, upgrades to WebSocket, and proxies raw bytes to `127.0.0.1:<port>`.

### Auth Middleware

`server/middleware/auth.go` is applied as a handler wrapper in `server.go`'s `Start()` method. The middleware chain processes all `/api/…` paths. Since the new `/api/sessions/{id}/vnc` handler is registered on `srv.mux`, it is automatically covered by the existing auth middleware. No additional auth work is needed at the handler level — the cookie/Bearer token validation happens before the request reaches the handler.

The `isAllowedOrigin` function in `connectrpc_websocket.go` is the WebSocket-specific CORS guard; the new VNC proxy handler should use the same function.

---

## 3. Display Number Allocation

### Recommended Scheme

Use a **port-range + file-lock scheme** with a global atomic counter:

```
displayBase = 100  (configurable)
displayN    = displayBase + sessionIndex  (or use a free-slot scanner)
VNC port    = 5900 + displayN
```

**Allocation algorithm** (to avoid conflicts with concurrent session creation):

1. On `VNCManager` init, scan `/tmp/.X<N>-lock` files for `N` in `[100, 200)`.
2. Keep a `sync.Map[int]string` of `displayN → sessionID` for tracking live allocations.
3. On session start: iterate from `displayBase` upward; for each candidate `N`, attempt to create `/tmp/.X<N>-lock` atomically using `os.OpenFile(O_CREATE|O_EXCL)`. If successful, `N` is ours; release the lock file when the session ends. This is exactly how the X11 locking protocol works.
4. On server restart, the lock files from a crashed run remain. Use a grace-period scan: if a lock file exists but no process matches the PID inside it, reclaim the slot.

**Lock file format** (standard X11):
```
/tmp/.X<N>-lock  →  contains PID of Xvfb as a 10-char decimal string
```

**Port mapping**: display `:N` → VNC port `5900+N` → Go proxy binds on no external port (all on `127.0.0.1`).

**Alternative**: simpler is a per-session random port from a reserved range (e.g. 15900–16100) rather than fixed `5900+N`, but this loses the useful `display → port` predictability. The X11-lock approach is preferred because it is the standard protocol that Xvfb itself uses.

---

## 4. VNC WebSocket Proxy Design

### Raw TCP Tunnel vs. RFB-Aware Proxy

**Recommendation: raw TCP tunnel.**

The RFB (Remote Framebuffer) protocol used by VNC operates over a raw TCP byte stream. noVNC already implements the full client-side RFB stack in JavaScript. A raw tunnel (WebSocket binary frames ↔ `net.Dial` TCP) requires ~40 lines of Go and has zero codec overhead.

RFB-aware proxying would be required only if:
- The proxy needed to transcode encodings (e.g., force ZRLE → Tight for bandwidth), or
- The proxy needed to intercept credentials for token-based auth at the Go layer.

Since x11vnc will be configured with a per-session random password passed as a shared secret, the proxy can simply forward the authentication handshake transparently. The Go layer only needs to validate the stapler-squad session cookie before upgrading — not the VNC password.

### Proxy Implementation Pattern

```go
// In VNCProxyHandler.HandleWebSocket:
conn, _ := wsUpgrader.Upgrade(w, r, nil)
tcpConn, _ := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", vncPort), 5*time.Second)

var wg sync.WaitGroup
wg.Add(2)
go func() { defer wg.Done(); io.Copy(tcpWriter{conn}, tcpConn) }()  // VNC → WS
go func() { defer wg.Done(); io.Copy(tcpConn, wsReader{conn}) }()   // WS → VNC
wg.Wait()
```

`tcpWriter` and `wsReader` are thin adapters that read/write WebSocket binary messages as a stream of bytes. This pattern is identical to what `websockify` (noVNC's reference proxy) does.

### Authentication

- **VNC password**: x11vnc is launched with `-passwd <random32chartoken>`. The token is stored in `VNCProcessManager.vncPassword`. The Go server does NOT need to inspect it — it's forwarded transparently as part of the RFB handshake.
- **stapler-squad auth**: The `/api/sessions/{id}/vnc` endpoint sits behind the standard auth middleware. An unauthenticated WebSocket upgrade request receives a 401 before the upgrade completes.
- **Per-session isolation**: the proxy uses the `sessionID` from the URL path to look up the VNC port; a valid session token does not give access to a different session's display.

### Connection Lifecycle

- **Browser tab closes**: gorilla/websocket detects the close frame; `io.Copy` returns; both goroutines exit; `wg.Wait()` returns; the handler closes the TCP connection to x11vnc. x11vnc enters "waiting for client" state (idle) — it does NOT exit.
- **Reconnect**: a fresh WebSocket upgrade to the same path starts a new TCP dial to the same x11vnc port. x11vnc handles reconnects natively.
- **x11vnc crash**: the TCP dial fails or the `io.Copy` returns with an error. The handler returns an error response. The `VNCProcessManager` crash-recovery goroutine (per FR-9) detects the dead process via `cmd.Wait()`, attempts up to 3 restarts, then marks the session's VNC state as unavailable.

---

## 5. Proto Changes Needed

### New Fields on `Session` Message (`types.proto`)

Add a new embedded message for VNC connection state:

```protobuf
message VNCState {
  enum Status {
    VNC_STATUS_UNSPECIFIED = 0;
    VNC_STATUS_STARTING = 1;
    VNC_STATUS_READY = 2;       // x11vnc is up, browser can connect
    VNC_STATUS_NO_BROWSER = 3;  // running but no browser window detected
    VNC_STATUS_UNAVAILABLE = 4; // dependency missing or crashed > 3 times
  }

  // Current VNC readiness state.
  Status status = 1;

  // Display number (e.g., 100 for :100). 0 when unavailable.
  int32 display_number = 2;

  // Ephemeral VNC password for this session (used by noVNC to connect).
  // Only populated in GetSession response, never in list/watch events.
  string vnc_password = 3;

  // Browser window detected (xdotool found a matching window).
  bool browser_window_detected = 4;

  // Clip geometry for the VNC viewport (x,y,w,h), or empty if full display.
  string clip_geometry = 5;
}
```

On the `Session` message, add:
```protobuf
// VNC browser passthrough state (Linux only; absent on macOS without VNC server).
VNCState vnc_state = 51;
```

### New RPC in `session.proto`

No new RPC methods are strictly required. The VNC WebSocket proxy is a plain HTTP handler, not a ConnectRPC call. However, two optional additions are useful:

1. **`GetVNCToken`** (unary) — returns a short-lived signed token that the frontend can pass as a query param on the WebSocket URL, enabling auth without a cookie (useful for iframes):
   ```protobuf
   rpc GetVNCToken(GetVNCTokenRequest) returns (GetVNCTokenResponse) {}
   ```

2. **`SetVNCQuality`** (unary) — allows the frontend quality toggle to persist the preference server-side:
   ```protobuf
   rpc SetVNCQuality(SetVNCQualityRequest) returns (SetVNCQualityResponse) {}
   ```

Both are deferrable to v2. For v1, `vnc_state` on `Session` plus the plain WebSocket proxy is sufficient.

---

## 6. Window Geometry Tracking

### xdotool Polling for Browser Window

x11vnc's `-clip WxH+X+Y` flag restricts the VNC viewport to a rectangle. To track the Chrome window:

1. A goroutine in `VNCProcessManager` polls xdotool every 500ms:
   ```bash
   xdotool search --onlyvisible --name "Google Chrome" getwindowgeometry --shell %@
   ```
   Output: `WINDOW=12345678\nX=0\nY=0\nWIDTH=1280\nHEIGHT=800\n` for each match.

2. On geometry change, send a `SIGUSR1` to x11vnc (which causes it to re-read its clip geometry from a temp file), OR restart x11vnc with the new `-clip` argument.

   **Preferred**: restart x11vnc with new geometry. x11vnc reconnects in <1s; the client sees a brief disconnect and auto-reconnects. This avoids signal-based IPC complexity.

   **Alternative**: use `-id <windowid>` instead of `-clip`. `x11vnc -id <winid>` captures only that window's pixels without needing clip geometry. This eliminates the geometry tracking problem entirely. On window resize, x11vnc auto-adapts. This is the preferred approach for v1.

3. **No browser detected**: when xdotool finds no matching window, `VNCState.browser_window_detected = false` is set. The frontend shows a placeholder ("No browser open yet"). x11vnc continues running (it shows the full virtual desktop, which is blank).

### Threading / Polling Interval

- Polling goroutine: started with a `context.Context` derived from the session's lifetime; cancelled on `Destroy()`.
- Interval: 500ms when no window detected (fast detection), 2s when a window is stable (reduced syscall load).
- xdotool is a subprocess call (`os/exec`). Use `safeexec` (already used throughout the codebase) with a 1s timeout to prevent blocking.
- Thread safety: geometry updates are communicated to `VNCState` via a mutex-protected field on `VNCProcessManager`; WatchSessions events are published via the existing `EventBus` when state changes.

---

## 7. New Package Structure

```
session/
  vnc/                         # New package
    manager.go                 # VNCProcessManager: start/stop Xvfb + x11vnc, display alloc
    display_alloc.go           # Display number allocation (X11 lock file scheme)
    window_tracker.go          # xdotool polling goroutine
    types.go                   # VNCState, VNCConfig types

server/services/
  vnc_proxy_handler.go         # WebSocket → TCP tunnel handler
  vnc_proxy_handler_test.go
```

`VNCProcessManager` is embedded in `Instance` alongside `TmuxProcessManager`. It is initialized (but not started) in `NewInstance()` when `config.BrowserPassthrough.Enabled == true` on Linux. It is started at the end of `Instance.start()` and stopped in `Instance.Destroy()`.

---

## 8. Environment Injection

The tmux `new-session` command already uses `-e KEY=VALUE` to inject `CLAUDECODE=`. The DISPLAY variable injection follows the same pattern:

```go
// In tmux.go start():
cmd := t.buildTmuxCommand("new-session", "-d", "-s", t.sanitizedName,
    "-e", "CLAUDECODE=",
    "-e", fmt.Sprintf("DISPLAY=:%d", displayN),
    "-c", workDir, programWithHistory)
```

`TmuxSession` needs to accept an optional `ExtraEnv []string` field (or the display number can be passed in the program string via `env DISPLAY=:N <program>`). The `env` prefix approach is already used for `HISTFILE` and requires no struct changes.

---

## 9. macOS Fallback Path

On macOS, the feature degrades gracefully:
- `VNCProcessManager.IsSupported()` checks `runtime.GOOS == "linux"` and that `Xvfb`, `x11vnc`, `xdotool` are in PATH.
- If unsupported, `Instance.vncManager` is nil; no display is allocated; `VNCState.Status = VNC_STATUS_UNAVAILABLE`.
- The frontend hides the Browser tab entirely when `vnc_state` is absent or `status == UNAVAILABLE`.
- No changes to the tmux session creation path on macOS.

---

## 10. Key Constraints and Risks

1. **gorilla/websocket already in go.mod** — no new dependency needed for the TCP proxy.
2. **`gofrs/flock` already in go.mod** — can be used for display-slot advisory locking in addition to the X11 lock file approach.
3. **`safeexec` package** — all subprocess calls (xdotool, Xvfb, x11vnc) must go through `safeexec.CommandContext` with timeout, per project convention.
4. **`connectrpc_websocket.go` is 61K** — avoid adding VNC proxy logic there. Keep it in a separate file.
5. **`-e` flag limit**: tmux passes env vars one at a time; multiple `-e` flags are fine.
6. **x11vnc `-rfbauth <passfile>` vs `-passwd <plaintext>`**: prefer passfile for security; write the random password to a temp file owned by the stapler-squad process with mode 0600.
7. **Shutdown race**: `wireDepsIntoServer` registers a shutdown hook that captures pane state. VNC cleanup should happen before or in parallel with tmux cleanup, not after.
