# Browser Passthrough — Technology Stack Research

## 1. Virtual Display Server: Xvfb vs. Alternatives

### Xvfb (Recommended)

Xvfb (X Virtual Framebuffer) is the correct choice for this feature on Linux. It is a headless X server that renders to shared memory instead of a physical display, and it is the industry-standard approach used by CI systems, Kasm Workspaces, Guacamole lab environments, and every other headless browser automation stack.

**Package (Arch Linux)**: `xorg-server-xvfb` v21.1.22-1 — available in the standard repos.

**Typical resource usage at 1280x800x24:**
- RSS at idle (no client connected): ~12–18 MB per instance
- The framebuffer itself consumes `width × height × depth/8` bytes in shared memory: 1280 × 800 × 3 = ~3 MB of SHM per instance
- CPU: near-zero at idle; spikes when the browser is rendering and x11vnc is polling

**Display number allocation**: Xvfb creates `/tmp/.X<N>-lock` (containing its PID) and `/tmp/.X11-unix/X<N>` (a Unix domain socket) on startup. The lock file format is a 10-character decimal PID string. Allocating a display number safely:

1. Scan `/tmp/.X<N>-lock` for N in `[100, 200)` to find occupied slots.
2. For each candidate N, attempt `os.OpenFile("/tmp/.X<N>-lock", O_CREATE|O_EXCL, 0644)` — if it succeeds, write the stapler-squad PID and proceed; if it fails with `EEXIST`, try N+1.
3. Remove the lock file when the session ends (Xvfb does this automatically on clean exit).
4. On stapler-squad startup, any lock file whose PID is not a live process should be cleaned up and the display slot reclaimed.

The project already has `gofrs/flock` for advisory file locking, which can supplement the `O_EXCL` approach for Go-level serialization of concurrent session starts.

**Current host display**: The development machine uses `:0` and `:1`. Slot 100+ is safely clear.

### Alternatives Considered

**Xephyr**: A nested X server that renders inside another X window. Requires an existing display to run. Not suitable for headless server use — eliminated.

**Xvnc (TigerVNC)**: TigerVNC provides `Xvnc`, which combines the X server and VNC server into one process. This eliminates the need to wire x11vnc to Xvfb separately. See §2 for detailed comparison.

**Virtual**: The `xf86-video-dummy` driver with a full Xorg instance. More complex setup, no meaningful advantage over Xvfb for our use case — eliminated.

---

## 2. VNC Server: x11vnc vs. TigerVNC's Xvnc

### Option A: Xvfb + x11vnc (Two Processes)

**x11vnc** (v0.9.17-1 on Arch) is a VNC server that attaches to a running X display. It reads the framebuffer over the X11 protocol and serves it via RFB.

Key flags for this feature:
- `-display :<N>` — attach to the Xvfb display
- `-rfbport <port>` — bind VNC on a specific localhost port
- `-localhost` — refuse connections from non-loopback addresses
- `-nopw` — no VNC-level password (rely on Go proxy auth; see pitfalls.md §1.1)
- `-id <windowid>` — restrict capture to a single X window ID (eliminates need for `-clip` geometry tracking)
- `-rfbauth <passfile>` — alternative: htpasswd-format auth file (8-char limit applies to RFB 3.x)
- `-xrandr` — handle display resize events
- `-shared` — allow multiple simultaneous viewers (useful for debugging)

**Window isolation with `-id`**: `x11vnc -id <windowid>` captures only the pixels of that window, automatically adapting to window resizes. This is simpler than computing clip geometry — use xdotool to find the window ID, then (re-)start x11vnc with `-id`. See §5 for xdotool details.

**Pros**: Mature, widely used, many fine-grained options. Separating Xvfb and x11vnc allows restarting x11vnc without losing the display (the browser stays running in Xvfb even while x11vnc restarts).
**Cons**: Two processes to manage per session; requires the X11 protocol bridge overhead between Xvfb and x11vnc.

### Option B: TigerVNC's Xvnc (One Process)

**Xvnc** is TigerVNC's combined X server + VNC server in a single binary. It starts a virtual X display and simultaneously serves it over RFB — no separate Xvfb or x11vnc needed.

**Package (Arch Linux)**: `tigervnc` v1.16.2-1. TigerVNC provides: `Xvnc`, `vncserver` (wrapper script), `vncviewer`, `x0vncserver` (attach to existing display).

**Comparison**:

| Concern | Xvfb + x11vnc | Xvnc alone |
|---|---|---|
| Process count per session | 2 | 1 |
| Restart x11vnc without losing browser | Yes | No (restarting Xvnc kills the X session) |
| Window isolation (`-id` equivalent) | x11vnc `-id <winid>` | No native equivalent — needs clip geometry |
| Encoding support | Good (Hextile, ZRLE, Tight) | Excellent (Tight, ZRLE, H.264 in KasmVNC fork) |
| RFB 8-char password limit | Yes | Yes |
| Built-in WebSocket (no proxy needed) | No | No (standard Xvnc does not) |
| Crash recovery granularity | Restart x11vnc only | Must restart entire display |

**Recommendation: Xvfb + x11vnc.**

The critical advantage is that x11vnc can be restarted independently of Xvfb. FR-9 requires up to 3 restart attempts on x11vnc crash — with Xvnc, a crash kills the entire virtual display and the browser session inside it, making recovery far more disruptive. The `-id <windowid>` window isolation is also easier to implement with x11vnc.

TigerVNC's `x0vncserver` (attach to an existing display, like x11vnc) is a viable alternative to x11vnc specifically, but x11vnc is better documented for the `-id` window capture mode and is already available on Arch.

### libvncserver / Direct Framebuffer Capture

Rolling a custom VNC server in Go using libvncserver or pure-Go framebuffer capture is a significant engineering effort with no advantages over the proven x11vnc path. Eliminated.

---

## 3. noVNC — Embedding the HTML5 Client

### Current Version and Distribution

**npm package**: `@novnc/novnc` v1.7.0 (latest as of 2026-05).
**Tarball size**: ~155 KB (66 files).
**License**: MPL-2.0.

The package exports a single ES module entry point: `./core/rfb.js`. It does not ship an HTML file — the integration page must be written by the consuming application.

### Package Contents (Key Files)

```
core/rfb.js          # Main RFB client class — the only import needed
core/websock.js      # WebSocket abstraction (sets binaryType = "arraybuffer")
core/decoders/       # Tight, ZRLE, Hextile, H.264, JPEG decoders
core/input/          # Keyboard + mouse normalization
vendor/pako/         # Bundled zlib (no external CDN dependency)
```

### WebSocket Protocol

noVNC uses standard binary WebSocket frames (`binaryType = "arraybuffer"`). The Go proxy does not need to be VNC-aware — it only needs to relay binary WebSocket messages to/from the raw TCP VNC socket. The `wsProtocols` option on `new RFB()` defaults to `[]` (no subprotocol negotiation required). The Go server should not set a `Sec-WebSocket-Protocol` response header unless the frontend passes one.

### Embedding Options

**Option A: npm package imported via bundler (Recommended)**

Add `@novnc/novnc` to `web-app/package.json` as a direct dependency. Import `RFB` in the React component:

```typescript
import RFB from '@novnc/novnc/core/rfb.js';
```

The bundler (Next.js/Webpack) includes the required files in the JavaScript bundle. No vendoring needed. The `vendor/pako/` files are auto-included via the import graph. This is the cleanest integration and keeps noVNC updated via normal dependency management.

**Option B: Vendored static files served by Go embed**

Extract the noVNC `core/` and `vendor/` directories into `server/static/novnc/` and serve them via `go:embed`. This adds ~150 KB to the binary and requires manual updates but makes the feature available even if the npm build fails. Suitable as a fallback or if the React SPA approach is abandoned.

**Recommendation**: Option A (npm package). The project already uses Next.js bundling for all other frontend assets.

### RFB Object Integration Pattern

```typescript
const rfb = new RFB(canvasElement, `ws://${host}/api/sessions/${sessionId}/vnc`, {
  credentials: { password: vncPassword },  // optional — only if VNC auth is enabled
});
rfb.scaleViewport = true;
rfb.resizeSession = false;  // let Xvfb hold fixed resolution
```

The `canvasElement` must be an `<div>` container (not a `<canvas>` directly); RFB creates its own canvas inside it.

---

## 4. Go WebSocket VNC Proxy

### Library Choice

**`github.com/gorilla/websocket` v1.5.3** is already in `go.mod`. No new dependency is needed.

The other option, `golang.org/x/net/websocket` (also in go.mod via indirect dependency), uses the older `http.Handler` API and is considered deprecated for new code. Gorilla is the correct choice and is already used throughout the codebase (`terminal_websocket.go`, `connectrpc_websocket.go`, `ws_stream_bridge.go`).

### Raw TCP Tunnel vs. VNC-Aware Proxy

**Recommendation: raw TCP tunnel.**

noVNC implements the full RFB client stack in JavaScript. The Go proxy only needs to relay bytes bidirectionally between the WebSocket connection and the TCP socket to x11vnc. No VNC protocol knowledge is required in Go.

This is identical to what `websockify` (noVNC's official reference proxy, written in Python) does. The proxy is ~40 lines of Go:

```go
func (h *VNCProxyHandler) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
    sessionID := r.PathValue("id")
    vncPort, err := h.vncManager.GetPort(sessionID)
    // ... auth check, 404 if not found ...
    
    wsConn, _ := wsUpgrader.Upgrade(w, r, nil)
    defer wsConn.Close()
    
    tcpConn, _ := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", vncPort), 5*time.Second)
    defer tcpConn.Close()
    
    var wg sync.WaitGroup
    wg.Add(2)
    // VNC TCP → WebSocket
    go func() {
        defer wg.Done()
        buf := make([]byte, 65536)
        for {
            n, err := tcpConn.Read(buf)
            if n > 0 { wsConn.WriteMessage(websocket.BinaryMessage, buf[:n]) }
            if err != nil { return }
        }
    }()
    // WebSocket → VNC TCP
    go func() {
        defer wg.Done()
        for {
            _, msg, err := wsConn.ReadMessage()
            if err != nil { return }
            tcpConn.Write(msg)
        }
    }()
    wg.Wait()
}
```

### Authentication

The VNC proxy endpoint (`/api/sessions/{id}/vnc`) should be registered behind the existing auth middleware chain — no extra auth work needed (see `architecture.md §2`). VNC-level passwords are defense-in-depth only, and x11vnc should be launched with `-nopw` + `-localhost`, with the Go proxy as the sole access gate (see `pitfalls.md §1.1`).

### Upgrader Configuration

Reuse or extend the existing `websocket.Upgrader` in `terminal_websocket.go`. The VNC proxy should use `isAllowedOrigin` from `connectrpc_websocket.go` for CORS enforcement. Buffer sizes can be larger than the terminal WebSocket (64 KB read/write buffers) since VNC sends bigger frames.

---

## 5. xdotool — Window Tracking

### Installed Version

`xdotool` v4.20251130.1 is already installed on the development host (`/usr/bin/xdotool`).

### Finding the Browser Window ID

```bash
DISPLAY=:<N> xdotool search --onlyvisible --classname "google-chrome" --pid <browser_pid>
```

The `--pid` flag is the most reliable filter when the browser PID is known (which it is — it's the child process of the tmux session). The `--classname google-chrome` filter adds resilience if the PID is unavailable.

**Output**: one or more window IDs (decimal), one per line. For Chrome, multiple windows may appear (main window + devtools + sub-windows); filter for the largest (by geometry) to find the main window.

To get geometry:
```bash
DISPLAY=:<N> xdotool getwindowgeometry <winid>
```

Output: `Window <winid>\n  Position: X,Y\n  Geometry: WxH`

### Tracking Strategy

**Recommended approach: use x11vnc `-id <windowid>`** rather than `-clip` geometry. x11vnc's `-id` mode:
- Captures only that window's pixels
- Automatically adjusts when the window is resized
- Shows the window without the surrounding virtual desktop

When the window ID changes (e.g., user closes and re-opens Chrome), the polling goroutine detects no result, marks `VNCState.browser_window_detected = false`, kills and restarts x11vnc with the new window ID once detected.

**Polling intervals**:
- 500ms when `browser_window_detected == false` (fast detection)
- 2s when `browser_window_detected == true` and geometry is stable (reduce syscall load)

**DISPLAY= prefix**: all `xdotool` calls for a session must set `DISPLAY=:<N>` in the subprocess env to target the correct virtual display.

### Alternatives

**`wmctrl`** (`/usr/bin/wmctrl` is installed): `wmctrl -l -p -G -x` lists all windows with PID, geometry, and class. Provides a single call for both discovery and geometry. Less commonly used than xdotool but a viable fallback. xdotool's `--sync` flag (wait until a match is found) is useful for initial detection; wmctrl has no equivalent.

**`xprop`**: reads X11 window properties. More low-level than xdotool; no practical advantage here.

**Events vs. polling**: The X11 event model (via `XSelectInput`/`XNextEvent`) would eliminate polling but requires a persistent X connection and significant Go CGo bindings or a C helper process. Not worth the complexity for this feature. Polling at 500ms is acceptable (10ms max detection lag at 500ms polling is well within the 200ms latency NFR-1 target, since detection lag only affects the tab appearance, not VNC frame latency).

---

## 6. macOS VNC

### Recommended Approach: Built-in Screen Sharing (ARD/VNC)

macOS includes a built-in VNC server that can be enabled programmatically via `launchctl`:

```bash
sudo launchctl load -w /System/Library/LaunchDaemons/com.apple.screensharing.plist
```

Or via `defaults write`:
```bash
sudo defaults write /var/db/launchd.db/com.apple.launchd/overrides.plist \
    com.apple.screensharing -dict Disabled -bool false
sudo launchctl load -w /System/Library/LaunchDaemons/com.apple.screensharing.plist
```

This starts Apple Remote Desktop / Screen Sharing, which speaks standard RFB and listens on port 5900. The same Go WebSocket proxy works unchanged — it just dials `127.0.0.1:5900` instead of a per-session port.

**Security considerations**:
- Enabling Screen Sharing grants VNC access to the full macOS desktop, not just one window
- The stapler-squad Go proxy is the auth gate (no VNC-level port exposure)
- Set a VNC password via System Preferences → Sharing → Screen Sharing → VNC Viewers option
- ARD uses DES-based auth (same 8-char limit as x11vnc); rely on the proxy, not VNC auth

**Window isolation on macOS**: Use `screencapture -l <windowid>` for screenshot-based approaches, or `CGWindowListCreateImage` via a Go CGo helper or Swift subprocess for real-time capture. However, for v1, macOS support is specified as "use the macOS built-in VNC server" (FR-8) — full-desktop VNC with no window cropping is acceptable.

### Alternative: Third-Party VNC Servers on macOS

- **TigerVNC Viewer** does not provide a macOS VNC server
- **RealVNC** and **Chicken** provide VNC servers but add a paid dependency
- **KasmVNC**: Linux-only in its container-based form

**Recommendation**: Use the built-in ARD/Screen Sharing for macOS v1. The full-desktop view is acceptable. Window isolation on macOS is a v2 concern.

### macOS Graceful Degradation

The `VNCProcessManager.IsSupported()` gate checks `runtime.GOOS == "linux"` and binary availability. On macOS, `VNCState.Status = VNC_STATUS_UNAVAILABLE` is set and the Browser tab is hidden — no macOS-specific code paths in v1.

---

## 7. Summary Recommendations

### Recommended Stack

| Component | Choice | Rationale |
|---|---|---|
| Virtual display | **Xvfb** (`xorg-server-xvfb`) | Industry standard; ~15 MB RSS idle; correct lock-file-based display allocation |
| VNC server | **x11vnc** with `-id <windowid>` | Restartable independently of Xvfb; window isolation via `-id` eliminates clip geometry tracking |
| VNC auth | **`-nopw -localhost`** | Go proxy is the auth gate; VNC 8-char password limit makes RFB auth weak by design |
| Browser client | **`@novnc/novnc` v1.7.0** via npm | ES module import via Next.js bundler; no vendoring needed; pako included |
| Go WebSocket proxy | **`gorilla/websocket` v1.5.3** (already in go.mod) | Raw TCP tunnel, ~40 LoC; no new dependencies |
| Window tracker | **xdotool** with `--pid` + `--classname` | Already installed; reliable; poll at 500ms/2s |
| macOS | Graceful degradation (Browser tab hidden) | ARD/Screen Sharing viable for v2 |

### Display/Port Number Scheme

- Display numbers: `100 + sessionIndex`, allocated via X11 lock file protocol (`O_CREATE|O_EXCL` on `/tmp/.X<N>-lock`)
- VNC ports: `net.Listen("tcp", "127.0.0.1:0")` (OS-assigned, avoids TOCTOU) — store in `VNCProcessManager.vncPort`; do NOT use `5900+N` fixed mapping (collision risk per `pitfalls.md §1.2`)

### Dependencies to Document (NFR-4)

Required on Linux hosts:
- `xorg-server-xvfb` (Arch: `xorg-server-xvfb`; Debian/Ubuntu: `xvfb`)
- `x11vnc` (Arch: `x11vnc`; Debian/Ubuntu: `x11vnc`)
- `xdotool` (Arch: `xdotool`; Debian/Ubuntu: `xdotool`)
- `google-chrome` or `chromium` (for the browser itself)

Feature degrades gracefully (Browser tab hidden) if any dependency is absent (checked via `exec.LookPath` at startup).
