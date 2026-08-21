# Browser Passthrough — Requirements

> **Note:** FR-2 through FR-5 and FR-8 were revised after implementation. The original VNC/noVNC design was superseded by CDP screencasting (see ADR-016). The original requirements are preserved in git history.

## Problem Statement

Stapler-squad manages AI agent sessions (Claude Code, Aider, etc.) in isolated tmux sessions.
Agents frequently open browsers to show output, run web servers for the user to preview, or interact with web applications.
Currently, users have no way to see or interact with what the agent has opened in a browser — they would need to VNC or SSH into the host separately.

The goal is to integrate a browser passthrough directly into the stapler-squad UI: a "Browser" tab appears alongside the existing terminal tab for each session, giving the user a live interactive view of the browser running on the host.

## Functional Requirements

### FR-1: Per-Session Virtual Display (Linux)
- Stapler-squad SHALL spawn a dedicated Xvfb virtual display (e.g., `:100 + session_index`) for each session on Linux hosts.
- The session's agent process SHALL inherit `DISPLAY=:<N>` so any GUI app (browser, electron app) it spawns appears on the virtual display.
- The virtual display SHALL be cleaned up (Xvfb process killed) when the session ends.

### FR-2: Chrome Wrapper Scripts + CDP Remote Debugging Port
- Stapler-squad SHALL write per-session Chrome wrapper scripts into `~/.stapler-squad/cdp-bins/<sessionID>/` before the tmux session is created.
- Each wrapper script SHALL intercept any invocation of a known Chrome/Chromium binary name (e.g. `google-chrome`, `chromium`, `chromium-browser`) and exec the real Chrome binary with `--remote-debugging-port=$CDP_PORT` prepended to the argument list.
- The session's `PATH` SHALL be modified (prepend the wrapper directory) and `CDP_PORT` SHALL be set, so any Chrome launch inside the session automatically connects to the allocated debugging port without agent cooperation beyond invoking Chrome normally.
- Wrapper scripts and the `cdp-bins/<sessionID>/` directory SHALL be removed when the session ends (via `CDPStreamManager.Stop()`).
- A `ReconcileOrphans` mechanism SHALL clean up directories left behind by sessions that were deleted without calling `Stop()` (e.g. on server restart).

### FR-3: Window Isolation (native to CDP)
- Window isolation is native to CDP — only the page content of the targeted browser tab is streamed, not the full virtual desktop.
- No `xdotool` or X11 window-clipping is required.
- If no browser has connected to the CDP port yet, the CDPViewer SHALL show a placeholder ("No browser open yet").

### FR-4: CDP WebSocket Relay
- Stapler-squad's Go server SHALL relay CDP `Page.screencastFrame` JPEG frames from Chrome to the browser UI via a dedicated WebSocket endpoint at `/api/sessions/{id}/cdp-stream`.
- The Go server polls Chrome's `/json/version` endpoint (on the allocated localhost CDP port) until Chrome starts, then opens a CDP WebSocket to Chrome and issues `Page.startScreencast` (JPEG format, configurable quality/resolution).
- Each received JPEG frame is forwarded as a binary WebSocket message to all connected browser clients.
- Mouse and keyboard input from browser clients is forwarded back to Chrome as `Input.dispatchMouseEvent` / `Input.dispatchKeyEvent` CDP commands.
- The relay endpoint SHALL be authenticated via the existing stapler-squad session token/cookie.
- Chrome's CDP port SHALL only be bound to `127.0.0.1`; the Go relay is the only access path.

### FR-5: CDPViewer — Canvas-Based JPEG Frame Renderer
- The web UI SHALL include a custom React component (`CDPViewer`) that connects to the CDP WebSocket relay (`/api/sessions/{id}/cdp-stream`) and renders incoming JPEG frames onto an HTML5 `<canvas>` element.
- No third-party VNC client (noVNC or similar) is used; `CDPViewer` is a purpose-built, dependency-free canvas renderer.
- Full mouse (click, scroll, drag) and keyboard input captured on the canvas SHALL be forwarded to the server as CDP `Input.dispatch*` JSON messages over the same WebSocket.
- `CDPViewer` SHALL reconnect automatically (2 s backoff) if the WebSocket is closed.
- Coordinate scaling SHALL be applied so CSS pixel positions are correctly mapped to Chrome device pixel positions based on the canvas intrinsic size.

### FR-6: Browser Tab in Session UI
- A "Browser" tab SHALL appear alongside the existing "Terminal" tab in the session detail view.
- The tab SHALL be hidden (or greyed out) when no browser process has connected to the session's CDP port.
- When Chrome is detected (CDP state transitions to `streaming`), the tab becomes active and the CDPViewer connects automatically.

### FR-7: Browser Launch Helper
- Stapler-squad SHALL provide a helper CLI or tmux shortcut that agents/users can invoke to open Chrome/Chromium on the session's virtual display.
- The agent can also open the browser independently; the passthrough detects it regardless.

### FR-8: macOS Support via $DISPLAY Passthrough
- On macOS hosts, Xvfb is not available and no virtual display is spawned.
- If `$DISPLAY` is already set in the stapler-squad server environment (e.g. the host is running a native desktop session), the VNC manager SHALL reuse the existing display value (`VNCStatusPassthrough`) and skip Xvfb allocation.
- CDP screencasting works natively on macOS since Chrome runs directly on the host without a virtual display — the CDP `Page.startScreencast` stream is OS-agnostic.
- The same CDP WebSocket relay and CDPViewer are used on all platforms; macOS requires no platform-specific streaming code.

### FR-9: Lifecycle Integration
- CDP manager and virtual display (Xvfb) lifecycle SHALL be tied to the Go session manager (same goroutine/process group that manages tmux).
- The CDP manager follows a three-phase startup: (1) `Allocate` — reserves port and writes wrapper scripts before tmux session creation; (2) `Start` — begins polling/screencast goroutine after tmux session is live; (3) `Stop` — cancels goroutines and cleans up on session destroy.
- Crash recovery: if the CDP WebSocket connection to Chrome is lost, the manager SHALL reconnect automatically (polling `/json/version` with backoff) before marking the browser tab as unavailable.

### FR-10: Bandwidth / Quality Controls
- The CDP screencast SHALL default to a reasonable quality preset (JPEG quality and max resolution configurable via `config.json`).
- `ScreencastQuality`, `ScreencastMaxWidth`, `ScreencastMaxHeight`, and `ScreencastMaxFPS` are tunable parameters passed to `Page.startScreencast`.
- A quality toggle in the browser tab UI (Low / Medium / High) SHALL allow the user to trade bandwidth for fidelity.

## Non-Functional Requirements

### NFR-1: Latency
- End-to-end input latency (keypress → screen update) SHALL be ≤ 200 ms on a LAN connection.
- Screen refresh rate SHALL support at least 10 fps at medium quality (JPEG, default `ScreencastMaxFPS`).
- Chrome's `Page.screencastFrame` / `Page.screencastFrameAck` flow is used for back-pressure control; the server sends one ack per frame so Chrome paces delivery to what the relay can consume.

### NFR-2: Security
- Chrome's CDP port SHALL only be bound to `127.0.0.1`; the Go relay is the only external access path.
- Each session's CDP port is allocated dynamically (random free port) and is not shared between sessions.
- The Go relay SHALL reject WebSocket upgrade requests that do not carry a valid session auth token.

### NFR-3: Resource Usage
- Each Xvfb process (Linux) SHALL consume ≤ 30 MB RSS at idle.
- The Go CDP relay goroutines add negligible overhead when no browser is connected (polling only).
- Xvfb display resolution SHALL default to 1280×800; configurable via `config.json`.
- CDP screencast frame size is bounded by `ScreencastMaxWidth` / `ScreencastMaxHeight` (defaults in `CDPConfig`).

### NFR-4: Dependencies
- New system-level dependencies (Xvfb on Linux; Chrome/Chromium on all platforms) SHALL be documented in the README.
- `xdotool` and `x11vnc` are no longer required for the streaming path (only Xvfb for virtual display on Linux).
- The feature SHALL degrade gracefully (browser tab hidden, no-op `CDPStreamManager`) if Chrome is not installed or if `BrowserPassthrough` is disabled in `config.json`.

### NFR-5: No Regressions
- Existing session creation, terminal, and all current UI features SHALL be unaffected.
- Adding DISPLAY= to session env SHALL be opt-in / controlled by config.

## Out of Scope (v1)

- Audio passthrough
- Multi-monitor virtual displays
- Firefox/other browser support (Chrome/Chromium only for browser detection heuristic)
- Agent auto-detection of "should I open a browser now" (agent decides independently)
- Cloud/remote host support (VNC to a remote machine via the internet — future)
- Clipboard sync between host and viewer

## Success Criteria

1. A user opens a session in stapler-squad, the agent runs `google-chrome` (intercepted by the session PATH wrapper which injects `--remote-debugging-port=$CDP_PORT`), and a "Browser" tab appears automatically in the UI.
2. Clicking the "Browser" tab shows a live, interactive view of Chrome via CDPViewer — user can navigate, click, type.
3. The browser tab shows only the Chrome page content, not the full virtual desktop (isolation is native to CDP).
4. Closing the session cleans up Xvfb (Linux), the CDP wrapper scripts (`~/.stapler-squad/cdp-bins/<sessionID>/`), and all associated goroutines.
5. On a machine without Chrome, the feature is absent but the rest of stapler-squad works normally.
6. The CDP relay connection is authenticated — an unauthenticated WebSocket upgrade request cannot access the CDP stream.

## Assumption Register

1. **Chrome must be launched with `--remote-debugging-port=N`.** The browser-passthrough feature auto-injects this flag via wrapper scripts placed in a per-session PATH prefix (`~/.stapler-squad/cdp-bins/<sessionID>/`). Agents that bypass the session PATH (e.g. invoke Chrome via an absolute path, use a non-standard binary name not covered by the wrapper set, or unset `CDP_PORT`) may launch Chrome without the debugging port set and will not be detected by the CDP polling loop. The feature degrades gracefully in this case — the Browser tab remains hidden rather than erroring.
