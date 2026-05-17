# Browser Passthrough — Requirements

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

### FR-2: VNC Server per Session (Linux)
- An `x11vnc` instance SHALL be launched per session, attached to its Xvfb display.
- x11vnc SHALL listen on a per-session localhost port (not exposed publicly).
- x11vnc SHALL be password-protected or use token auth; access is brokered only through the stapler-squad server.
- x11vnc SHALL be cleaned up with the session.

### FR-3: Window Isolation — Browser Window Only
- The VNC viewport SHALL be cropped/clipped to show only the browser window, not the full virtual desktop.
- Implementation: use `xdotool` to find the browser window ID and pass geometry to x11vnc via `-clip` or equivalent, OR use `xdotool getactivewindow` polling to track focus.
- If no browser window is detected, the view SHALL show a placeholder ("No browser open yet").

### FR-4: WebSocket Proxy
- Stapler-squad's Go server SHALL proxy VNC WebSocket connections from the browser UI to the per-session x11vnc port.
- The proxy endpoint SHALL be authenticated via the existing stapler-squad session token/cookie.
- VNC traffic SHALL NOT be directly exposed on any port outside localhost.

### FR-5: noVNC Embedded Client
- The web UI SHALL embed the noVNC JavaScript client (vendored or served from the Go binary).
- The "Browser" tab in the session view SHALL render an HTML5 canvas powered by noVNC.
- Full mouse (click, scroll, drag) and keyboard input SHALL be forwarded to the host.

### FR-6: Browser Tab in Session UI
- A "Browser" tab SHALL appear alongside the existing "Terminal" tab in the session detail view.
- The tab SHALL be hidden (or greyed out) when no browser process is detected on the session's virtual display.
- When the browser window is detected, the tab becomes active and the noVNC canvas connects automatically.

### FR-7: Browser Launch Helper
- Stapler-squad SHALL provide a helper CLI or tmux shortcut that agents/users can invoke to open Chrome/Chromium on the session's virtual display.
- The agent can also open the browser independently; the passthrough detects it regardless.

### FR-8: macOS Support
- On macOS hosts, the Xvfb/x11vnc approach is not available.
- macOS sessions SHALL use the macOS built-in VNC server (Screen Sharing / ARD protocol) or a 3rd-party VNC server (e.g., `librfb`-based).
- The same noVNC WebSocket proxy path SHALL be reused; only the server-side VNC source differs.
- Window isolation on macOS: use Quartz Window Services (`CGWindowListCreateImage`) or `screencapture -l <windowid>` to crop to the browser window.

### FR-9: Lifecycle Integration
- VNC server and virtual display lifecycle SHALL be tied to the Go session manager (same goroutine/process group that manages tmux).
- Crash recovery: if x11vnc dies unexpectedly, stapler-squad SHALL restart it (up to 3 attempts) before marking the browser tab as unavailable.

### FR-10: Bandwidth / Quality Controls
- The noVNC connection SHALL default to a reasonable quality preset (ZRLE or Tight encoding at medium quality).
- A quality toggle in the browser tab UI (Low / Medium / High) SHALL allow the user to trade bandwidth for fidelity.

## Non-Functional Requirements

### NFR-1: Latency
- End-to-end input latency (keypress → screen update) SHALL be ≤ 200 ms on a LAN connection.
- Screen refresh rate SHALL support at least 10 fps at medium quality.

### NFR-2: Security
- VNC ports SHALL only be bound to `127.0.0.1`; the Go proxy is the only access path.
- Each VNC session SHALL use a unique random password or token, rotated per session start.
- The Go proxy SHALL reject WebSocket upgrade requests that do not carry a valid session auth token.

### NFR-3: Resource Usage
- Each Xvfb + x11vnc pair SHALL consume ≤ 50 MB RSS at idle (no active VNC client).
- Xvfb display resolution SHALL default to 1280×800; configurable via config.json.

### NFR-4: Dependencies
- New system-level dependencies (Xvfb, x11vnc, xdotool) SHALL be documented in the README.
- The feature SHALL degrade gracefully (browser tab hidden) if dependencies are not installed.

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

1. A user opens a session in stapler-squad, the agent runs `google-chrome --display=:NN`, and a "Browser" tab appears automatically in the UI.
2. Clicking the "Browser" tab shows a live, interactive view of Chrome — user can navigate, click, type.
3. The browser tab shows only the Chrome window, not the full virtual desktop.
4. Closing the session cleans up Xvfb and x11vnc processes.
5. On a machine without Xvfb, the feature is absent but the rest of stapler-squad works normally.
6. The proxy connection is authenticated — an unauthenticated request cannot access the VNC stream.
