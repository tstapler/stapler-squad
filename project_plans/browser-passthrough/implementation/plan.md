# Browser Passthrough — Implementation Plan

## Architecture Summary

The browser passthrough feature integrates a live, interactive view of a per-session virtual display directly into the stapler-squad web UI. On Linux, each session owns a dedicated Xvfb virtual framebuffer (display `:100+N`, allocated via the standard X11 lock-file protocol) and a paired x11vnc server bound exclusively to `127.0.0.1`. The agent process inherits `DISPLAY=:<N>` via the existing tmux `-e` injection path, so any GUI application it launches automatically appears on the virtual display. A polling goroutine (`xdotool`) detects when a browser window appears on the display and triggers x11vnc to focus on that window via `-id <windowid>`; this eliminates the need for clip-geometry tracking.

The Go server exposes a single raw WebSocket endpoint (`/api/sessions/{id}/vnc`) that tunnels bytes bidirectionally between the browser and the per-session x11vnc TCP port. No VNC protocol knowledge is required in Go — the proxy is a dumb byte relay (~60 lines) modeled on the existing `terminal_websocket.go` pattern, with a shared `context.Context` ensuring both goroutines tear down cleanly when either side closes. Authentication is handled entirely by the existing auth middleware that wraps all `/api/` routes; x11vnc itself binds to localhost only with no VNC-level password (`-nopw`), making the Go proxy the sole access gate.

The React frontend adds a `"browser"` tab to `SessionDetailTab` and mounts a new `BrowserTab` component that uses `@novnc/novnc` (imported as a dynamic/SSR-disabled ES module, matching the `TerminalOutput` pattern exactly). The tab renders conditionally based on `VNCState.status` pushed through the existing `WatchSessions` stream. On non-Linux hosts or when dependencies are absent, `VNCState.status` is `VNC_STATUS_UNAVAILABLE` and the browser tab is hidden entirely — all other stapler-squad functionality is unaffected.

## Technology Decisions

| Decision | Choice | Rationale |
|---|---|---|
| Virtual display | Xvfb (`Xvfb :<N> -screen 0 1280x800x24 -nolisten tcp`) | Industry standard; ~15 MB RSS at idle; lock-file allocation protocol prevents collisions |
| VNC server | x11vnc with `-nopw -localhost -id <windowid>` | Independently restartable from Xvfb; `-id` mode eliminates clip-geometry tracking; crash recovery (FR-9) leaves the browser running in Xvfb untouched |
| VNC auth | `-nopw -localhost` only | RFB 3.x DES auth has an 8-char ceiling making it inadequate; proxy-only auth (the Go middleware) is the correct gate per NFR-2 |
| WebSocket proxy | Raw TCP tunnel via `gorilla/websocket` + `net.Dial` | Already in go.mod; ~60 LoC; no VNC protocol knowledge needed; identical to websockify's approach |
| noVNC integration | `@novnc/novnc` npm package, direct `RFB` import via `next/dynamic` SSR-disabled | Matches `TerminalOutput`/xterm.js pattern exactly; no extra dependency layer |
| Display allocation | X11 lock-file protocol (`O_CREATE|O_EXCL` on `/tmp/.X<N>-lock`), range 100–200 | This is the standard X11 protocol; Xvfb uses it natively; stale-lock cleanup on startup |
| VNC port allocation | `net.Listen("tcp", "127.0.0.1:0")` (OS-assigned) | Eliminates fixed-port collisions (pitfall §1.2); port stored in VNCProcessManager |
| Process management | `executor.StartProcess` (existing `ManagedProcess`) with `Setpgid: true` | `Stop()` delivers SIGKILL to the process group; prevents orphans on crash (pitfall §6.3) |
| Window tracking | xdotool polling at 500ms (no-window) / 2s (window stable) | Already installed; `--pid` flag gives reliable per-session targeting |
| macOS v1 | Graceful degradation — Browser tab hidden | ARD/Screen Sharing viable for v2; no macOS-specific code paths added |

**ADR Candidates** (three decisions warrant formal ADRs):
- ADR-010: VNC-proxy-only auth vs. defense-in-depth RFB password (security boundary)
- ADR-011: Xvfb+x11vnc (two-process) vs. Xvnc (one-process) — crash-recovery trade-off
- ADR-012: OS-assigned VNC port vs. fixed `5900+displayN` mapping — TOCTOU vs. predictability

---

## Epics

### Epic 1: VNC Process Manager (Go backend)

**Goal:** Implement `session/vnc` package with full Xvfb + x11vnc lifecycle management, display-number allocation, port management, crash recovery, and startup dependency detection.

**Stories:**

- Story 1.1: VNC package skeleton and types
  - Task 1.1.1: Create `session/vnc/types.go` — define `VNCConfig` (display resolution, display base, restart limit) and `VNCState` (Go-native, distinct from proto) structs.
  - Task 1.1.2: Create `session/vnc/display_alloc.go` — implement `DisplayAllocator` with `Allocate(sessionID string) (int, error)` and `Release(n int)` using `O_CREATE|O_EXCL` on `/tmp/.X<N>-lock` (range 100–200); `CleanupStaleDisplays()` called at startup to reclaim lock files whose PID is no longer live.
  - Task 1.1.3: Create `session/vnc/manager.go` — define `VNCProcessManager` interface and `vncProcessManager` struct; fields: `xvfb *executor.ManagedProcess`, `x11vnc *executor.ManagedProcess`, `displayN int`, `vncPort int`, `state VNCState`, `mu sync.RWMutex`.

- Story 1.2: Xvfb and x11vnc process lifecycle
  - Task 1.2.1: Implement `vncProcessManager.startXvfb(ctx context.Context)` in `session/vnc/manager.go` — allocate display number via `DisplayAllocator`, call `executor.StartProcess(ctx, "Xvfb", []string{":<N>", "-screen", "0", "1280x800x24", "-nolisten", "tcp"}, ...)` with `ManagedProcess`'s `WithStdoutWriter`/`WithStderrWriter` piping to session log.
  - Task 1.2.2: Implement `vncProcessManager.startX11vnc(ctx context.Context, windowID string)` in `session/vnc/manager.go` — obtain OS-assigned port via `net.Listen("tcp", "127.0.0.1:0")` + `close + record port`, call `executor.StartProcess(ctx, "x11vnc", []string{"-display", ":<N>", "-rfbport", "<port>", "-localhost", "-nopw", "-noclipboard", "-id", windowID, "-shared"}, ...)`. If `windowID` is empty, omit `-id` flag (full-display mode until browser is detected).
  - Task 1.2.3: Implement `vncProcessManager.Start(ctx context.Context)` — call `startXvfb`, then `startX11vnc("")`, update `state.Status` to `VNC_STATUS_NO_BROWSER`, launch window tracker goroutine (see Epic 4).
  - Task 1.2.4: Implement `vncProcessManager.Stop()` in `session/vnc/manager.go` — call `x11vnc.Stop()` then `xvfb.Stop()` (order matters), call `DisplayAllocator.Release(displayN)`.

- Story 1.3: Crash recovery
  - Task 1.3.1: Implement x11vnc crash-recovery goroutine in `session/vnc/manager.go` — launched after `startX11vnc`; monitors `x11vnc.Wait()` return; on unexpected exit, applies exponential backoff (100ms, 200ms, 400ms … cap 30s) for up to 3 restart attempts per the `trackRestartRate` pattern from `session/instance_tmux.go`; after 3 consecutive failures sets `state.Status = VNC_STATUS_UNAVAILABLE` and emits a state-change event.
  - Task 1.3.2: Add `vncProcessManager.IsAlive() bool` and `vncProcessManager.Port() int` accessors used by the WebSocket proxy (Epic 2).

- Story 1.4: Config and dependency detection
  - Task 1.4.1: Add `BrowserPassthrough BrowserPassthroughConfig` field to `Config` struct in `config/config.go` — struct fields: `Enabled bool`, `DisplayBase int`, `Resolution string`, `DisplayRangeMax int`. `Enabled` defaults to `true` on Linux when deps are present.
  - Task 1.4.2: Create `session/vnc/deps_check.go` — implement `CheckDependencies() DepsResult` using `exec.LookPath("Xvfb")`, `exec.LookPath("x11vnc")`, `exec.LookPath("xdotool")`; return a struct with `Available bool` and missing-binary list; called once at startup and logged.
  - Task 1.4.3: In `session/vnc/manager.go` `New(cfg VNCConfig) VNCProcessManager` — if deps check fails or `runtime.GOOS != "linux"`, return a `noopVNCManager` that satisfies the interface with all methods being no-ops and `state.Status = VNC_STATUS_UNAVAILABLE`.

- Story 1.5: Startup orphan cleanup
  - Task 1.5.1: Add `VNCProcessManager.ReconcileOrphans(sessions []StoredSession)` in `session/vnc/manager.go` — on startup, scan `/tmp/.X100-lock` through `/tmp/.X200-lock`; for each stale lock (PID dead or not matching a live session), kill the PID if alive, remove lock + socket, log reclamation. Called from `BuildDependencies()` in `server/dependencies.go` before sessions load.

---

### Epic 2: VNC WebSocket Proxy (Go backend)

**Goal:** Implement the `/api/sessions/{id}/vnc` HTTP endpoint that upgrades to WebSocket and tunnels raw bytes to the per-session x11vnc TCP port, behind the existing auth middleware.

**Stories:**

- Story 2.1: Proxy handler
  - Task 2.1.1: Create `server/services/vnc_proxy_handler.go` — define `VNCProxyHandler` struct with a `VNCManager` (interface); implement `HandleWebSocket(w http.ResponseWriter, r *http.Request)`:
    1. Extract `sessionID` via `r.PathValue("id")`.
    2. Look up `vncPort` from `VNCManager.GetPort(sessionID)` — return 404 if not found or port is 0 (VNC unavailable).
    3. Upgrade to WebSocket using `gorilla/websocket.Upgrader` with `isAllowedOrigin` from `connectrpc_websocket.go`.
    4. Dial `net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", vncPort), 5*time.Second)`.
    5. Launch two goroutines (VNC→WS, WS→VNC) sharing a `context.WithCancel(r.Context())`; each goroutine calls `defer cancel()` on return so the other goroutine is cancelled immediately on any error or close.
    6. `defer tcpConn.Close()` immediately after dial success.
    7. `wg.Wait()` before handler returns.
  - Task 2.1.2: Add `wsReader` and `wsWriter` thin adapter types in `server/services/vnc_proxy_handler.go` — `wsReader.Read()` calls `wsConn.ReadMessage()` and copies bytes into buffer; `wsWriter.Write()` calls `wsConn.WriteMessage(websocket.BinaryMessage, p)`. These make `io.Copy` composable.

- Story 2.2: Route registration
  - Task 2.2.1: Add `VNCManager VNCProcessManager` to `ServerDependencies` struct in `server/dependencies.go` — populated during `BuildDependencies()` after calling `session/vnc.New(cfg)` and `ReconcileOrphans`.
  - Task 2.2.2: In `wireDepsIntoServer()` in `server/server.go` — register `srv.mux.HandleFunc("/api/sessions/{id}/vnc", vncHandler.HandleWebSocket)` after the existing WebSocket handlers; auth middleware covers it automatically since all `/api/` routes are wrapped.

- Story 2.3: Unit tests
  - Task 2.3.1: Create `server/services/vnc_proxy_handler_test.go` — test that an unauthenticated request returns 401 (middleware test); test that a request for an unknown session returns 404; test bidirectional byte relay using a mock TCP server (loopback listener).

---

### Epic 3: Proto + Session State

**Goal:** Add `VNCState` to the proto `Session` message, wire it through `WatchSessions` events and `GetSession` responses, and regenerate all bindings.

**Stories:**

- Story 3.1: Proto schema additions
  - Task 3.1.1: Add `VNCState` message to `proto/session/v1/types.proto` (after field 50 `WorkingState`) — fields: `Status status = 1` (enum: UNSPECIFIED=0, STARTING=1, READY=2, NO_BROWSER=3, UNAVAILABLE=4), `int32 display_number = 2`, `string vnc_password = 3` (intentionally empty in list/watch events, only populated in GetSession), `bool browser_window_detected = 4`.
  - Task 3.1.2: Add `VNCState vnc_state = 51;` field to `message Session` in `proto/session/v1/types.proto`.
  - Task 3.1.3: Run `make generate-proto` to regenerate `gen/proto/go/session/v1/*.go` and `web-app/src/gen/session/v1/*_pb.ts`.

- Story 3.2: Backend wiring
  - Task 3.2.1: In `server/services/session_service.go` — add `sessionToProto` mapping for `vnc_state`: read `inst.VNCManager.State()` and map Go `VNCState` fields to proto `VNCState`; omit `vnc_password` in list/watch contexts (set empty string), include it only in `GetSession`.
  - Task 3.2.2: In `session/instance.go` — add `vncManager VNCProcessManager` field (after `tmuxManager` and `gitManager`); initialize in `NewInstance()` via `vnc.New(cfg)`; call `vncManager.Start(ctx)` at the end of `Instance.start()` (after `tmuxManager.Start()`); call `vncManager.Stop()` at the start of `Instance.Destroy()` (before `KillSession()`).
  - Task 3.2.3: In `session/instance.go` — inject `DISPLAY=:<N>` into the tmux environment by appending `-e`, `fmt.Sprintf("DISPLAY=:%d", vncManager.DisplayNumber())` to the tmux `new-session` command args in `initTmuxSession()`. Guard with `if vncManager.DisplayNumber() > 0`.
  - Task 3.2.4: Publish `VNCState` changes on the existing `EventBus` — in `session/vnc/manager.go`, when `state` changes (status, browser_window_detected), call a registered `OnStateChange(VNCState)` callback; wire this callback in `Instance.start()` to emit a `SessionUpdated` event so `WatchSessions` clients receive the update without polling.

- Story 3.3: State persistence
  - Task 3.3.1: In `session/vnc/manager.go`, after `startXvfb` succeeds, persist `displayN` and the Xvfb PID into `Instance.ExternalMetadata` (or a new `VNCMetadata` struct stored alongside it) via a callback; this allows orphan cleanup on restart (Epic 1, Story 1.5) to find and kill PIDs from the previous run.

---

### Epic 4: Browser Window Detection

**Goal:** Implement the xdotool polling goroutine that detects a Chrome window on the session's virtual display, updates `VNCState.browser_window_detected`, and restarts x11vnc in `-id` mode when detected.

**Stories:**

- Story 4.1: xdotool polling goroutine
  - Task 4.1.1: Create `session/vnc/window_tracker.go` — define `WindowTracker` struct with `displayN int`, `onWindowDetected func(windowID string)`, `onWindowLost func()`; implement `Start(ctx context.Context)` goroutine that polls at 500ms (no window) or 2s (window stable).
  - Task 4.1.2: Implement `WindowTracker.poll(ctx context.Context)` in `session/vnc/window_tracker.go` — execute via `executor/safeexec.CommandContext(ctx, "xdotool", "search", "--onlyvisible", "--classname", "google-chrome")` with `DISPLAY=:<N>` in subprocess env and a 1s timeout; parse stdout for window IDs; if multiple results, select largest by geometry using `xdotool getwindowgeometry <id>`.
  - Task 4.1.3: Implement multi-window selection heuristic in `session/vnc/window_tracker.go` — if multiple windows returned by xdotool, call `xdotool getwindowgeometry` on each; select the window with maximum `width * height` as the main browser window.

- Story 4.2: x11vnc restart on window detection
  - Task 4.2.1: In `session/vnc/manager.go`, implement `restartX11vncWithWindow(ctx context.Context, windowID string)` — call `x11vnc.Stop()`, then `startX11vnc(ctx, windowID)` with the new `-id <windowID>` arg; update `state.Status = VNC_STATUS_READY` and `state.browser_window_detected = true`; emit state-change event.
  - Task 4.2.2: Wire `WindowTracker.onWindowDetected` → `restartX11vncWithWindow` and `WindowTracker.onWindowLost` → set `state.browser_window_detected = false`, `state.Status = VNC_STATUS_NO_BROWSER`, emit state-change event (do NOT stop x11vnc — it continues serving the full virtual display as a blank placeholder).

- Story 4.3: Window ID staleness handling
  - Task 4.3.1: In `session/vnc/window_tracker.go`, track `lastWindowID string`; on each poll, if `lastWindowID != "" && new result is empty`, call `onWindowLost()` and reset `lastWindowID`; if result changes (window closed and new window opened), call `onWindowDetected(newID)` with the new ID so x11vnc is restarted with the fresh window ID (prevents pitfall §3.3 — stale window IDs).

---

### Epic 5: Frontend Browser Tab

**Goal:** Add a "Browser" tab to the session detail view, implement the noVNC RFB viewer component, conditionally show/hide based on `VNCState`, and integrate quality controls.

**Stories:**

- Story 5.1: Tab type extension
  - Task 5.1.1: Update `web-app/src/lib/pane/paneTypes.ts` — add `"browser"` to the `SessionDetailTab` type union.
  - Task 5.1.2: Update `web-app/src/components/pane/PaneHeader.tsx` — add `browser: "Browser"` to `TAB_LABELS`, `browser: "Browser"` to `TAB_FULL_LABELS`, and `"browser"` to `ALL_TABS` array.
  - Task 5.1.3: Update `web-app/src/components/sessions/SessionDetailView.tsx` — add `{ id: "browser", label: "Browser", icon: Globe }` (or `Monitor`) to the `tabs` array; conditionally disable/hide the tab entry when `session.vncState?.status` is absent, `UNSPECIFIED`, or `UNAVAILABLE` (grey out with `disabled` prop, not fully removed — so the user knows the feature exists but is unavailable).

- Story 5.2: noVNC RFB component
  - Task 5.2.1: Add `@novnc/novnc` to `web-app/package.json` dependencies (run `cd web-app && npm install @novnc/novnc`).
  - Task 5.2.2: Create `web-app/src/components/sessions/NoVNCViewer.tsx` — inner component (not dynamically imported itself); accepts `{ containerRef: React.RefObject<HTMLDivElement>, wsUrl: string, isVisible: boolean, qualityLevel: number }`; implements `useEffect` that creates `new RFB(containerRef.current, wsUrl)` on mount, sets `rfb.scaleViewport = true`, `rfb.resizeSession = false`, `rfb.clipViewport = false` (no clipboard), `rfb.qualityLevel = qualityLevel`, `rfb.compressionLevel = 6`; cleans up `rfb.disconnect()` on unmount; pauses/resumes rendering updates via `rfb.focus()`/`rfb.blur()` on `isVisible` changes.
  - Task 5.2.3: Create `web-app/src/components/sessions/BrowserTab.tsx` — top-level component accepting `{ sessionId: string, baseUrl: string, isVisible: boolean, vncState: VNCState | undefined }`; uses `next/dynamic` SSR-disabled to import `NoVNCViewer`; constructs WebSocket URL as `ws://${baseUrl}/api/sessions/${sessionId}/vnc` (or `wss://` when `baseUrl` starts with `https`); renders placeholder ("No browser open yet") when `vncState?.browserWindowDetected === false` or `status === NO_BROWSER`; renders error state when `status === UNAVAILABLE`.
  - Task 5.2.4: Create `web-app/src/components/sessions/BrowserTab.css.ts` — vanilla-extract styles for the canvas container, placeholder overlay, and quality control strip; import tokens from `vars` in `theme.css.ts`; define `browserTabContainer`, `placeholderOverlay`, `qualityControls` style classes.

- Story 5.3: Tab mount pattern (keep-alive)
  - Task 5.3.1: In `web-app/src/components/sessions/SessionDetailView.tsx` — render `BrowserTab` inside the same `visibility`/`pointerEvents` pooling pattern used for `TerminalOutput`; mount it once (not conditionally) so the noVNC RFB connection persists across tab switches; pass `isVisible={activeTab === "browser"}` to `BrowserTab`.

- Story 5.4: Quality controls UI
  - Task 5.4.1: Add a quality toggle to `web-app/src/components/sessions/BrowserTab.tsx` — three-button strip (Low / Medium / High) rendered inside the browser tab header area; maps to `rfb.qualityLevel` values `{ Low: 3, Medium: 6, High: 9 }`; `qualityLevel` state is local to `BrowserTab` (not persisted server-side for v1).

- Story 5.5: VNCState in frontend session type
  - Task 5.5.1: Verify that the auto-generated TypeScript bindings in `web-app/src/gen/session/v1/types_pb.ts` include `vncState` on `Session` after `make generate-proto` (Epic 3); no manual type additions needed if proto generation is correct.
  - Task 5.5.2: Update `web-app/src/lib/hooks/useSessionService.ts` (or wherever sessions are fetched/watched) — ensure `vncState` from `WatchSessions` events is propagated to the session state accessible by `SessionDetailView`.

---

### Epic 6: Dependency Detection + Graceful Degradation

**Goal:** Check for required binaries at startup, expose a health endpoint, hide the Browser tab on unsupported hosts, and document installation requirements.

**Stories:**

- Story 6.1: Startup detection and logging
  - Task 6.1.1: Call `session/vnc.CheckDependencies()` in `BuildDependencies()` in `server/dependencies.go`; log a `WARN` with the list of missing binaries; store the `DepsResult` on `ServerDependencies` so it is accessible without re-running `LookPath`.
  - Task 6.1.2: In `session/vnc/deps_check.go`, also verify that `runtime.GOOS == "linux"` — on non-Linux platforms, `Available` is always `false` and the reason is `"platform not supported"`.

- Story 6.2: Noop manager on unsupported hosts
  - Task 6.2.1: `session/vnc/manager.go` `New()` returns `noopVNCManager` when deps are unavailable; `noopVNCManager` implements the `VNCProcessManager` interface with all methods being no-ops; `State()` returns `VNCState{Status: VNC_STATUS_UNAVAILABLE}`; `DisplayNumber()` returns `0`; `Port()` returns `0`.
  - Task 6.2.2: In `server/services/session_service.go` `sessionToProto()` — when `inst.vncManager` returns `VNC_STATUS_UNAVAILABLE`, set `vnc_state.status = VNC_STATUS_UNAVAILABLE` and omit all other fields. The frontend Browser tab hides entirely on this state.

- Story 6.3: Feature registry update
  - Task 6.3.1: Add entry to `docs/registry/backend-features.json` for `browser:proxy` (the VNC WebSocket proxy endpoint); `tested: false` initially; update to `true` after proxy handler tests are written.
  - Task 6.3.2: Add entry to `docs/registry/frontend-features.json` for the `BrowserTab` component; `tested: false` initially.

- Story 6.4: Browser launch helper
  - Task 6.4.1: Add a `launch-browser` helper script at `scripts/launch-browser.sh` — accepts `DISPLAY` as env var or first arg; exec `google-chrome --no-sandbox --disable-dev-shm-usage --display=$DISPLAY` (falling back to `chromium`); meant to be called by agents or users from within the tmux session.
  - Task 6.4.2: Update `README.md` — add "Browser Passthrough" section documenting required packages (`xorg-server-xvfb`, `x11vnc`, `xdotool`, `google-chrome` or `chromium`), package names for Arch and Debian/Ubuntu, and the graceful-degradation behavior when packages are absent.

---

## Acceptance Criteria

1. **FR-1 (Per-session virtual display):** Creating a session on Linux allocates a unique Xvfb display (`:100+N`); the tmux session environment includes `DISPLAY=:<N>`; destroying the session kills Xvfb and removes the X11 lock file.
2. **FR-2 (VNC server per session):** x11vnc starts automatically with the session, binds to a localhost-only OS-assigned port; x11vnc is killed when the session is destroyed.
3. **FR-3 (Window isolation):** When Chrome is detected via xdotool, x11vnc is restarted with `-id <windowID>` so only the Chrome window pixels are served; when no window is detected, the frontend shows "No browser open yet."
4. **FR-4 (WebSocket proxy):** `GET /api/sessions/{id}/vnc` (WebSocket upgrade) proxies bytes to the per-session x11vnc port; an unauthenticated request returns 401 before upgrade; a request for a different session's ID cannot access another session's display.
5. **FR-5 (noVNC client):** The "Browser" tab renders an HTML5 canvas via `@novnc/novnc`; mouse clicks, scrolls, drags, and keyboard input are forwarded to the host.
6. **FR-6 (Browser tab visibility):** The "Browser" tab is greyed/disabled when `vnc_state.status` is `UNAVAILABLE` or `NO_BROWSER`; it becomes active and the RFB connection opens automatically when `browser_window_detected` transitions to `true` via `WatchSessions`.
7. **FR-7 (Browser launch helper):** `scripts/launch-browser.sh` launches Chrome on the session's `$DISPLAY`; agents can also open the browser independently and it is detected within 500ms.
8. **FR-8 (macOS graceful degradation):** On macOS hosts, `CheckDependencies()` returns `Available: false`; `VNCState.status = VNC_STATUS_UNAVAILABLE`; the Browser tab is hidden; no macOS-specific code paths execute.
9. **FR-9 (Lifecycle integration):** x11vnc crash triggers up to 3 restart attempts with exponential backoff before marking `VNC_STATUS_UNAVAILABLE`; Xvfb is unaffected by x11vnc restarts (Chrome keeps running).
10. **FR-10 (Bandwidth/quality):** noVNC defaults to `qualityLevel = 6` (medium); the Low/Medium/High toggle in the Browser tab UI changes `rfb.qualityLevel` at runtime without reconnecting.

**NFR compliance:**
- NFR-1: Raw byte tunnel adds <1ms proxy overhead; noVNC's ZRLE/Tight delta encoding supports ≥10 fps at medium quality over LAN.
- NFR-2: x11vnc bound to `127.0.0.1` only; Go proxy validates session cookie before upgrade; per-session port isolation (OS-assigned) prevents cross-session access.
- NFR-3: Xvfb at 1280×800×24 consumes ~15 MB RSS idle; x11vnc adds ~10 MB — total ~25 MB idle, within the 50 MB budget (NFR-3).
- NFR-4: Binary detection at startup; noop manager + hidden tab when deps absent; README documents packages.
- NFR-5: `DISPLAY=` injection is conditional (`vncManager.DisplayNumber() > 0`); existing session creation on macOS or without Xvfb is unaffected.

---

## Implementation Notes

### Process Management (Critical)
- All Xvfb and x11vnc invocations MUST use `executor.StartProcess` — never raw `exec.Command`. `ManagedProcess` with `Setpgid: true` ensures `Stop()` delivers SIGKILL to the entire process group, preventing orphan processes on crash (pitfall §6.3 / §1.3).
- On shutdown, VNC cleanup (Epic 1 Story 1.2) must run before tmux cleanup. The shutdown hook in `wireDepsIntoServer` triggers `historyLinker.Instances()` capture; add VNC manager stop to each instance's `Destroy()` path, which is already called before tmux kill.

### Goroutine Leak Prevention (Critical)
- The WebSocket proxy (Epic 2 Story 2.1) MUST use a shared `context.WithCancel(r.Context())` with each goroutine calling `defer cancel()`. When either the WebSocket or TCP connection closes, both goroutines must unblock within milliseconds. Failure to do this leaks two goroutines and two FDs per reconnect (pitfall §4.1 / §6.1).
- `defer tcpConn.Close()` must appear immediately after a successful `net.DialTimeout`, not in a goroutine.

### Display Allocation Race Prevention
- `DisplayAllocator.Allocate()` uses `os.OpenFile(path, O_CREATE|O_EXCL, 0644)` — the `EXCL` flag makes the operation atomic on POSIX systems; no mutex needed for the kernel-level allocation step, but Go-level bookkeeping (`sync.Map`) should also be updated inside a lock.
- On startup, `ReconcileOrphans()` must run before any new sessions are started to avoid false-positive "stale lock" removals.

### Window ID Staleness (xdotool)
- Do NOT cache window IDs across x11vnc restarts. When x11vnc is restarted with `-id <newWindowID>`, the old window ID is immediately invalid. The `WindowTracker` must track `lastWindowID` and detect changes (pitfall §3.3).
- Always set `DISPLAY=:<N>` in xdotool subprocess env — without it, xdotool targets the host display `:0` and finds completely different windows (pitfall §3.4).

### noVNC Integration Patterns
- `BrowserTab` must mirror the `TerminalOutput` mount pattern exactly: always mounted (not conditionally rendered), visibility controlled via CSS `visibility: hidden` + `pointer-events: none`, not React conditional rendering. This preserves the RFB TCP connection across tab switches.
- `NoVNCViewer` must be imported via `next/dynamic` with `ssr: false` — noVNC manipulates the DOM directly and requires a browser environment.
- Disable clipboard sync: set `rfb.clipViewport = false` and launch x11vnc with `-noclipboard`. Clipboard sync is out of scope for v1 and creates host data leakage risk (pitfall §4.2).

### Proto Field Numbering
- The next available field number on `Session` is `52` (field 51 is `VNCState vnc_state`; field 50 is `WorkingState working_state`). Reserve field 51.
- `vnc_password` field (field 3 in `VNCState`) must be explicitly cleared (set to `""`) in the `WatchSessions` and `ListSessions` paths — only populate it in `GetSession`. This avoids broadcasting VNC credentials to all connected clients via the event stream.

### Config Defaults
- `BrowserPassthrough.Enabled` should default to `true` when `CheckDependencies().Available == true`; users should not need to opt in manually (NFR-5: opt-in is via the system dependency presence, not a config flag).
- `BrowserPassthrough.Resolution` defaults to `"1280x800x24"`; `DisplayBase` defaults to `100`.

### Testing Strategy
- Go unit tests: `session/vnc/display_alloc_test.go` (concurrent allocation safety), `session/vnc/manager_test.go` (mock `ManagedProcess`, verify start/stop order), `server/services/vnc_proxy_handler_test.go` (mock TCP server, verify byte relay and goroutine cleanup).
- Frontend unit tests (Jest): `BrowserTab.test.tsx` — verify placeholder renders when `browser_window_detected: false`; verify `NoVNCViewer` not mounted when `status === UNAVAILABLE`; verify quality toggle updates `rfb.qualityLevel`.
- E2E test (Playwright): `tests/e2e/browser-passthrough.spec.ts` — start session, verify "Browser" tab disabled initially; run `google-chrome` in tmux, wait for `browser_window_detected: true` event, verify tab becomes active; click tab, verify canvas element present.

### ADRs Required Before Implementation
- Write ADR-010 (`docs/adr/010-vnc-proxy-auth.md`) — document proxy-only auth decision and why RFB password is not relied upon.
- Write ADR-011 (`docs/adr/011-xvfb-x11vnc-two-process.md`) — document two-process vs. Xvnc decision; key rationale is independent x11vnc restartability.
- Write ADR-012 (`docs/adr/012-vnc-port-allocation.md`) — document OS-assigned port vs. fixed `5900+N` scheme; rationale is avoiding TOCTOU collisions and the stale-port problem on crash.
