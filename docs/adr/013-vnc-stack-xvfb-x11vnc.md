# ADR-013: VNC Stack — Xvfb + x11vnc with Window-Level Capture

## Status
Accepted

## Context

The browser-passthrough feature (FR-1 through FR-3) requires a per-session virtual display on Linux hosts and a VNC server to export that display to the browser. Three architecturally distinct options were evaluated:

### Option A: Xvfb + x11vnc (Two Processes)

**Xvfb** (`xorg-server-xvfb`) is the industry-standard headless X server, used by every major CI/CD system and headless browser automation stack. It renders to shared memory rather than a physical display. Resource usage at 1280x800x24: ~12–18 MB RSS idle; ~3 MB shared memory for the framebuffer.

**x11vnc** (v0.9.17 on Arch) attaches to a running X display over the X11 protocol and serves it via RFB. It provides a key capability for this feature: `-id <windowid>`, which restricts capture to a single X window. Combined with `xdotool search --pid --classname`, this enables browser-window isolation without computing clip geometry.

The separation of Xvfb from x11vnc is the critical architectural property: x11vnc can be restarted independently of Xvfb. The browser continues running inside the virtual display while x11vnc recovers from a crash.

### Option B: TigerVNC's Xvnc (Single Process)

TigerVNC provides `Xvnc`, which combines the X server and VNC server into a single binary. This eliminates the Xvfb → x11vnc protocol bridge.

| Concern | Xvfb + x11vnc | Xvnc alone |
|---|---|---|
| Process count per session | 2 | 1 |
| Restart VNC without losing browser | Yes | No — Xvnc crash kills the X session |
| Window isolation (`-id` equivalent) | x11vnc `-id <winid>` (native) | No equivalent — requires clip geometry |
| Encoding support | Good (Hextile, ZRLE, Tight) | Excellent (also H.264 in KasmVNC fork) |
| Crash recovery granularity | Restart x11vnc only | Must restart entire display + browser |

Xvnc's single-process design makes it impossible to satisfy FR-9 (crash recovery: restart x11vnc up to 3 times) without also restarting the browser. A VNC crash would force the agent's browser session to terminate — an unacceptable disruption.

TigerVNC's `x0vncserver` (a separate binary that attaches to an existing display, analogous to x11vnc) is a viable alternative to x11vnc specifically, but x11vnc is better documented for `-id` window capture and is already available in the standard Arch repos without pulling in the full `tigervnc` package.

### Option C: Chrome DevTools Protocol (CDP)

CDP provides a WebSocket-based protocol for inspecting and controlling Chromium-family browsers. A CDP-based approach would eliminate Xvfb and x11vnc entirely — the browser streams frames directly.

CDP was eliminated for two reasons:

1. **Chrome-only.** FR-7 notes that only Chrome/Chromium is supported in v1, but the VNC infrastructure is browser-agnostic. A future agent opening Electron, Firefox, or any other X11 application would require parallel infrastructure. The VNC path generalises; CDP does not.
2. **No VNC infrastructure reuse.** The Go proxy, noVNC client, and all lifecycle management built around the VNC path would need to be rebuilt or duplicated for CDP.

### Display Number and Port Allocation

Display numbers are allocated in the range `:100`–`:200` using the standard X11 lock-file protocol (`O_CREATE|O_EXCL` on `/tmp/.X<N>-lock`). This is exactly the protocol Xvfb itself uses and avoids races with concurrent session creation.

VNC ports are allocated by asking the OS for an ephemeral port (`net.Listen("tcp", "127.0.0.1:0")`), recording it, then passing it to x11vnc via `-rfbport`. Fixed `5900+N` mapping is avoided — it is a collision vector when sessions are rapidly created and destroyed (pitfalls.md §1.2).

### macOS

On macOS, Xvfb and x11vnc are not available. The `VNCProcessManager.IsSupported()` gate checks `runtime.GOOS == "linux"` and that `Xvfb`, `x11vnc`, and `xdotool` are in PATH. If unsupported, `VNCState.Status = VNC_STATUS_UNAVAILABLE` is returned and the Browser tab is hidden. macOS support (via the built-in ARD/Screen Sharing or a lightweight VNC server) is deferred to v2.

## Decision

Use **Xvfb + x11vnc**, with browser-window isolation via `x11vnc -id <windowid>`.

- Xvfb is launched per session on display `:<100 + slot>`, allocated via the X11 lock-file protocol.
- x11vnc is launched per session, bound to `127.0.0.1:<OS-assigned-port>`, with `-id <windowid>` to restrict capture to the browser window.
- The window ID is discovered by `xdotool search --pid --classname google-chrome` polling at 500ms (no window) / 2s (window stable).
- Both processes are managed via `ManagedProcess` from the `executor` package (not raw `exec.Cmd`) to ensure `Setpgid: true` and correct SIGKILL on process-group teardown.
- On macOS or missing dependencies, the feature degrades gracefully: Browser tab is hidden.

## Consequences

### Positive

- x11vnc can be restarted independently of Xvfb, satisfying FR-9 (up to 3 crash-recovery attempts) without disrupting the browser session.
- `x11vnc -id <windowid>` provides clean browser-window isolation without requiring clip geometry tracking logic.
- Both packages are in the standard Arch repos (`xorg-server-xvfb`, `x11vnc`) and in Debian/Ubuntu (`xvfb`, `x11vnc`); no custom builds.
- Resource usage per session at idle: ~12–18 MB (Xvfb) + ~5 MB (x11vnc) RSS, well within the 50 MB NFR-3 budget.

### Negative / Constraints

- Two OS processes per session instead of one. Each concurrent session adds two file descriptors (process handles) and two lock-file entries.
- The X11 protocol bridge between Xvfb and x11vnc adds a small CPU overhead when x11vnc is polling the framebuffer. At 10 fps this is negligible.
- Requires three system packages on Linux hosts: `xorg-server-xvfb` (or `xvfb`), `x11vnc`, `xdotool`. Must be documented in the README (NFR-4).
- Window ID staleness: if the browser is closed and re-opened, the window ID changes. The window-tracker goroutine must detect this and restart x11vnc with the new ID (see pitfalls.md §3.3). The 500ms polling interval keeps detection lag within the NFR-1 latency budget.
- macOS support deferred to v2 — the Browser tab will not appear on macOS hosts.

## References

- Requirements: `project_plans/browser-passthrough/requirements.md` (FR-1, FR-2, FR-3, FR-9, NFR-1, NFR-3, NFR-4)
- Stack research: `project_plans/browser-passthrough/research/stack.md` §1–2, §5
- Pitfalls: `project_plans/browser-passthrough/research/pitfalls.md` §1.2, §2.1, §3.3, §6.3, §7.1–7.2
- x11vnc `-id` mode: https://www.karlrunge.com/x11vnc/x11vnc_opts.html
- xdotool: https://github.com/jordansissel/xdotool
