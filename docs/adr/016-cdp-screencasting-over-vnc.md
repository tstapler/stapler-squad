# ADR-016: CDP Screencasting as Primary Browser Passthrough Path

## Status
Accepted

## Context

ADR-013 selected Xvfb + x11vnc as the browser-passthrough stack and ADR-015 selected noVNC as the client. Together these provide a cross-browser, X11-agnostic streaming path — but they impose a hard Linux dependency: `xorg-server-xvfb`, `x11vnc`, and `xdotool` must be present, and the entire stack is unavailable on macOS.

After implementation, three friction points emerged:

1. **macOS gap.** The `VNCProcessManager.IsSupported()` gate returns false on macOS (ADR-013, §macOS). Developers on macOS cannot use the browser-passthrough feature at all, which slows iteration and forces feature development onto Linux-only environments.

2. **Binary dependency surface.** Installing and pinning x11vnc across Arch, Debian/Ubuntu, and macOS (where it is not available) adds operational overhead. CI must install these packages, and the Linux-specific code path diverges from the macOS path in ways that are hard to test uniformly.

3. **noVNC bundle weight.** ADR-015 acknowledged ~150 KB gzipped added to the webpack bundle. For a single-feature tab this is acceptable, but when a lighter alternative exists it is worth taking.

### Chrome DevTools Protocol (CDP) Screencasting

Chrome (and Chromium-family browsers) expose a WebSocket-based DevTools Protocol on a localhost port. The `Page.startScreencast` command delivers a continuous stream of JPEG-encoded screen frames over the DevTools WebSocket. Input events (mouse, keyboard) are injected via `Input.dispatchMouseEvent` and `Input.dispatchKeyEvent`.

This provides browser streaming without any X11 tooling:

| Concern | VNC path (ADR-013/015) | CDP path (this ADR) |
|---|---|---|
| Linux support | Yes (Xvfb + x11vnc) | Yes (Chrome + CDP) |
| macOS support | No (Xvfb unavailable) | Yes (Chrome runs natively; DISPLAY pre-set by caller) |
| Required OS packages | xorg-server-xvfb, x11vnc, xdotool | None (Chrome installed by user) |
| Client library | noVNC (~150 KB gzipped) | Vanilla WebSocket + `<canvas>` drawImage |
| Browser coverage | Any X11 app | Chrome/Chromium only |
| Frame encoding | RFB (multiple encodings) | JPEG via `Page.startScreencast` |
| Input injection | RFB client events → x11vnc | CDP `Input.dispatch*` commands |

### macOS Support Model

On macOS, Chrome runs in a normal windowed environment; no virtual display is needed. The session's Chrome wrapper script (written by `CDPStreamManager.Allocate()`) launches Chrome with `--remote-debugging-port=<N>` and `--app=about:blank`. The caller is responsible for ensuring `DISPLAY` is set appropriately on Linux; on macOS no `DISPLAY` is needed.

The VNC structs (`session/vnc/`) remain in place for display number management on Linux (Xvfb slot allocation) and for future headless-browser scenarios that may require a virtual display independently of CDP.

### Scope

This ADR covers only the screencasting path (frame delivery and input injection). It does not change:
- Port allocation strategy (see ADR-017)
- VNC proxy handler (`/api/sessions/{id}/vnc`) — still registered and available
- Xvfb display management code (`session/vnc/display_alloc.go`)
- VNC authentication model (ADR-014)

## Decision

Use **Chrome DevTools Protocol `Page.startScreencast`** as the primary browser passthrough path, delivered via a Go WebSocket handler at `/api/sessions/{id}/cdp-stream`.

- `CDPStreamManager` (`session/cdp/`) manages the Chrome subprocess, DevTools connection, and frame buffer. It exposes `LatestFrame() []byte` (most-recent JPEG) and `DispatchInput(msg []byte) error`.
- Chrome is launched with `--remote-debugging-port=<N> --headless=new --app=about:blank` via a per-session wrapper script written to a temp directory. The wrapper prepends to PATH so that other tools in the session (e.g., the agent's browser automation) transparently pick up the CDP-enabled Chrome.
- The Go WebSocket handler (`CDPStreamHandler.HandleWebSocket`) runs a frame-sender goroutine at ~15 fps and an input-receiver goroutine; both share a cancellable context so either side closing tears down the pair cleanly.
- On macOS no Xvfb is started; on Linux Xvfb display allocation continues as before via `session/vnc/display_alloc.go`.
- The feature degrades gracefully when Chrome is absent: `CheckDependencies()` returns `Available: false` and the Browser tab is hidden.

## Consequences

### Positive

- Browser passthrough works on macOS without any additional packages.
- No x11vnc or xdotool binaries required on any platform.
- The client is a plain `<canvas>` element receiving binary WebSocket frames — no noVNC bundle needed for the CDP path.
- CDP port is bound to `127.0.0.1` only; Go proxy is the sole access gate (same security model as ADR-014).
- Frame delivery latency is lower than the VNC path on the same host because there is no X11 protocol bridge between the capture point and the server.

### Negative / Constraints

- Chrome-only. Any non-Chromium browser running in the session cannot be streamed via CDP. The VNC path remains available for those use cases but requires Linux.
- JPEG-only encoding. CDP `Page.startScreencast` does not offer lossless or delta-encoded frames. For text-heavy content, JPEG artefacts may be visible at lower quality settings.
- Chrome must be installed and in PATH (or configured via `ChromePath` in config). The feature is unavailable if Chrome is absent — this is the same graceful-degradation behaviour as the VNC path on macOS.

## References

- Supersedes (for screencasting): ADR-013 (VNC stack), ADR-015 (noVNC embedding)
- Port allocation strategy: ADR-017
- VNC auth model (unchanged): ADR-014
- CDP `Page.startScreencast`: https://chromedevtools.github.io/devtools-protocol/tot/Page/#method-startScreencast
- CDP `Input.dispatchMouseEvent`: https://chromedevtools.github.io/devtools-protocol/tot/Input/#method-dispatchMouseEvent
- Implementation: `session/cdp/manager.go`, `server/services/cdp_stream_handler.go`
