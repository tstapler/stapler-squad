# Browser Passthrough — Feature Research

## 1. Apache Guacamole — VNC Proxy Architecture

### How It Works

Guacamole uses a three-tier architecture:

```
Browser (guacamole-common-js) ←─WebSocket/HTTP tunnel─→ guacd (C daemon) ←─RFB/RDP/SSH─→ VNC server
```

- **guacd** is the core: a native C daemon that speaks raw VNC/RFB, RDP, and SSH on the backend, and a custom "Guacamole protocol" (text-based, instruction-oriented) to the web layer. It loads protocol support as plugins.
- **guacamole-common-js** is the browser client library. It provides a `Guacamole.Client` that renders to an HTML5 canvas and a `Guacamole.Tunnel` abstraction (HTTP long-poll or WebSocket). It also supplies mouse/keyboard abstraction objects that translate browser events into Guacamole protocol keydown/keyup and mouse-event instructions.
- **Input forwarding**: The JS library exposes `Guacamole.Keyboard` and `Guacamole.Mouse` which normalize browser input events and call `client.sendKeyEvent()` / `client.sendMouseState()`. The custom protocol passes these through guacd, which re-encodes them as RFB key/pointer events to the VNC server.
- **No RFB in the browser**: Guacamole terminates RFB entirely in guacd. The browser never sees raw RFB bytes — it speaks the higher-level Guacamole protocol. This contrasts with noVNC, which speaks RFB directly in JavaScript.

### What We Can Borrow

- **Proxy-terminates-protocol pattern**: stapler-squad does not need to expose raw RFB to the browser if it adopts the Guacamole model. However, the full guacd pipeline is heavy for our use case. A simpler dumb-tunnel (websockify-style) is sufficient because noVNC handles RFB in JS.
- **Input abstraction**: guacamole-common-js's mouse/keyboard normalization is a useful reference, but noVNC already provides equivalent built-in input handling.
- **HTTP tunnel fallback**: Guacamole's fallback from WebSocket to HTTP long-poll shows that maintaining a tunnel abstraction with both transports keeps the feature working in restrictive proxy environments. Worth noting for NFR-1 (latency) — WebSocket should be strongly preferred.

### References

- [Guacamole Architecture v1.6.0](https://guacamole.apache.org/doc/gug/guacamole-architecture.html)
- [guacamole-common-js v1.6.0](https://guacamole.apache.org/doc/gug/guacamole-common-js.html)

---

## 2. Kasm Workspaces — Per-Session Display Isolation and KasmVNC

### Architecture

Kasm runs each workspace in an isolated **Docker container**. The container starts with:
- An Xvfb virtual display (or equivalent)
- A **KasmVNC** server (a fork of TigerVNC with web-native enhancements)
- A websockify bridge bundled inside the container image

The agent connects to the user's browser directly via KasmVNC's built-in WebSocket support (no separate websockify process needed for recent KasmVNC versions).

### Per-Session Display Isolation

- Each container gets its own `:N` X display — no sharing between users.
- The display number is assigned at container launch time and passed as `DISPLAY=:N` to all processes in the container.
- This is exactly the model stapler-squad should follow: one Xvfb display per session, assigned at session creation.

### Encoding and Quality

- KasmVNC v1.4+ uses **WebP** encoding and smart (adaptive) encoding, yielding ~30% better compression than standard RFB encodings.
- Supports **near-60fps** streaming with lossless mode for LAN connections.
- For our use case (x11vnc instead of KasmVNC), the relevant encodings are **Tight** (good compression over LAN/WAN) and **ZRLE** (lower CPU, good LAN). These are the two encodings the requirements specify as defaults (FR-10).

### What We Can Borrow

- **One virtual display per session lifecycle unit**: Kasm's container = stapler-squad's tmux session. The display is born with the session and destroyed with it — the same lifecycle integration model required by FR-9.
- **Single proxy port per session**: Kasm exposes one WebSocket endpoint per container. For stapler-squad: one per-session localhost TCP port (x11vnc) proxied through the Go server (FR-4).
- **Placeholder UX when no display is active**: Kasm shows a "workspace is loading" view before the VNC connection is ready. Analogous to the "No browser open yet" placeholder required by FR-3.

### References

- [Kasm System Architecture](https://docs.kasm.com/docs/guide/system_architecture/index.html)
- [KasmVNC GitHub](https://github.com/kasmtech/KasmVNC)

---

## 3. noVNC — Client Library

### Current State (2025–2026)

- **Latest stable**: v1.5.0; a v1.7.0-beta was tagged November 2025.
- **npm package**: `@novnc/novnc` — published as ES modules.
- **Bundle size**: The core library (`core/rfb.js` + dependencies) is approximately 150–200 KB unminified; tree-shaken for production it is smaller. The full application bundle (with UI chrome) is larger but unnecessary when embedding.
- **Rendering**: Pure HTML5 `<canvas>` via `createImageData`. No WebGL required.

### Embedding in React

Three viable patterns, in order of simplicity:

**Pattern A — `react-vnc` npm wrapper** (`roerohan/react-vnc`):
```tsx
import { VncScreen } from 'react-vnc';
<VncScreen url="wss://..." />
```
Wraps noVNC's `RFB` class in a React component with lifecycle management. Simplest approach. Downside: an extra dependency layer that may lag behind noVNC releases.

**Pattern B — Direct `@novnc/novnc` with `dynamic()` import**:
```tsx
// BrowserTab.tsx
const noVNCViewer = dynamic(() => import('./NoVNCViewer'), { ssr: false });
```
Instantiate `RFB` from `@novnc/novnc/core/rfb.js` directly inside a `useEffect`, attaching it to a `<div ref>`. This is how the existing `TerminalOutput` component handles xterm.js — SSR-disabled dynamic import, imperative API behind a ref. **Recommended pattern** given stapler-squad's existing conventions.

**Pattern C — Vendored static files + `<iframe>`**:
Serve noVNC's `vnc.html` as a static asset from the Go binary and embed it in an iframe. Simplest backend-wise but worst React integration — cross-frame messaging needed for quality controls, tab visibility events, etc.

### Input Event Support

noVNC's `RFB` class handles:
- Mouse: `mousemove`, `mousedown`, `mouseup`, `wheel` → RFB PointerEvent messages
- Keyboard: `keydown`, `keyup` → RFB KeyEvent (XKB keysyms)
- Touch: touch → pointer event translation (mobile support)
- Clipboard: bidirectional clipboard via RFB ClientCutText / ServerCutText (marked out of scope for v1 in requirements)

### Key Integration Points

- `new RFB(container, wsUrl, { credentials: { password } })` — creates the connection
- `rfb.addEventListener('connect', ...)` — fires when VNC negotiation completes
- `rfb.addEventListener('disconnect', ...)` — fires on clean or error disconnect
- `rfb.scaleViewport = true` — auto-scales canvas to container size
- `rfb.resizeSession = true` — sends RFB SetDesktopSize to resize the Xvfb display
- Quality: `rfb.qualityLevel` (0–9) and `rfb.compressionLevel` (0–9)

### References

- [noVNC GitHub](https://github.com/novnc/noVNC)
- [noVNC Embedding Docs](https://novnc.com/noVNC/docs/EMBEDDING.html)
- [@novnc/novnc npm](https://www.npmjs.com/package/@novnc/novnc)
- [react-vnc GitHub](https://github.com/roerohan/react-vnc)

---

## 4. websockify — WebSocket-to-TCP Bridge

### What It Is

websockify (Python, by the noVNC project) is a **dumb byte-level tunnel**: it accepts a WebSocket connection, opens a TCP connection to a target host:port, and forwards bytes in both directions. It does **not parse or modify RFB protocol data** — it is protocol-agnostic. The RFB handshake and all framing is handled by noVNC (client-side) and the VNC server (server-side).

The original implementation is **Python-only** (primary), with community ports in C, Node.js, Ruby, and Clojure.

### Go Alternatives

Multiple Go implementations exist:

| Project | Notes |
|---|---|
| `github.com/msquee/go-websockify` | Pure Go, improved connection handling, Linux/Windows/macOS |
| `github.com/miladj/websockify-go` | Simple port |
| `github.com/ruzhila/websockify-go` | Pure Go, SSL support |
| `github.com/evangwt/go-vncproxy` | Includes token handler for multi-VNC routing |
| `github.com/amitbet/vncproxy` | Full RFB proxy (parses protocol, can record/replay FBS files) |

### Recommendation for stapler-squad

**Do not use an external websockify process.** The Go server already owns a WebSocket upgrader (`gorilla/websocket` in `terminal_websocket.go`). Adding a VNC proxy endpoint follows the same pattern: upgrade HTTP to WebSocket, dial the per-session x11vnc TCP port on localhost, and `io.Copy` in both directions with goroutines — exactly what `terminal_websocket.go` does for PTY output.

The `go-vncproxy` token handler model is directly applicable: the proxy endpoint receives a `session_id` query parameter, looks up the corresponding x11vnc port, and establishes the tunnel. Authentication is handled by the existing stapler-squad session auth middleware (NFR-2).

A minimal implementation is ~50 lines of Go using `gorilla/websocket` + `net.Dial`.

### References

- [websockify GitHub](https://github.com/novnc/websockify)
- [go-vncproxy GitHub](https://github.com/evangwt/go-vncproxy)
- [go-websockify (msquee)](https://github.com/msquee/go-websockify)

---

## 5. Chrome DevTools Protocol (CDP) Screencasting — Comparison

### What CDP Offers

`Page.startScreencast` streams captured frames as base64-encoded PNG/JPEG via the `Page.screencastFrame` DevTools event. Parameters: format, quality (0–100), max width/height, and `everyNthFrame`.

### Limitations vs. VNC

| Dimension | CDP Screencasting | VNC (x11vnc + noVNC) |
|---|---|---|
| **Scope** | Chrome/Chromium only | Any X11 app on the display |
| **Input forwarding** | `Input.dispatchKeyEvent`, `Input.dispatchMouseEvent` via CDP | Native RFB (full keyboard + pointer) |
| **Frame rate** | Event-driven; `everyNthFrame` workaround; rarely exceeds 24 fps; known scaling issues | x11vnc can deliver 30 fps+ via Tight/ZRLE |
| **Encoding efficiency** | PNG/JPEG per frame (no delta encoding) | RFB delta encodings (ZRLE, Tight, Hextile) transmit only changed regions |
| **Latency** | Higher due to Chrome's internal frame capture pipeline | Lower — screen buffer read is direct |
| **Browser coupling** | Chrome must be launched with `--remote-debugging-port`; requires CDP client in Go | x11vnc works with any X11 display |
| **Security** | CDP port exposes full browser control | x11vnc password-protected; stapler-squad proxies |
| **Architecture fit** | Would require a CDP Go client (e.g., `chromedp`) per session | Simple TCP tunnel reusing existing WS infrastructure |

**Decision**: The requirements document (FR-5) correctly selects VNC + noVNC. CDP screencasting would lock the feature to Chrome, add a CDP management layer per session, and deliver inferior frame rate and efficiency compared to RFB delta encoding.

### References

- [CDP Page domain](https://chromedevtools.github.io/devtools-protocol/tot/Page/)
- [CDP FPS limitations issue](https://github.com/ChromeDevTools/devtools-protocol/issues/63)

---

## 6. Existing Stapler-Squad UI Patterns

### Tab System

`SessionDetail.tsx` defines `SessionDetailTab` as a string union:
```ts
export type SessionDetailTab = "terminal" | "diff" | "vcs" | "logs" | "info" | "files";
```

Adding `"browser"` to this union is the first change needed. The tab is registered in two places:

1. **`SessionDetailView.tsx`** — the `tabs` array that drives the tab strip UI (line ~176):
   ```ts
   const tabs: { id: SessionDetailTab; label: string; icon: LucideIcon }[] = [
     { id: "terminal", label: "Terminal", icon: Terminal },
     // ...
   ];
   ```
   Add `{ id: "browser", label: "Browser", icon: Globe }` (or `Monitor` from lucide-react).

2. **`PaneHeader.tsx`** — `TAB_LABELS`, `TAB_FULL_LABELS`, and `ALL_TABS` arrays. Update all three.

3. **`paneTypes.ts`** — `SessionDetailTab` type definition and any pane action types that carry it.

### Terminal Tab Mount Pattern (Critical for Browser Tab)

The terminal tab is **kept mounted** even when hidden (`display: none`), to preserve xterm.js state across tab switches. The "pooled session" pattern (up to 8 sessions kept alive simultaneously) is implemented via absolute-positioned divs with `visibility` toggling:

```tsx
{pooledSessionIds.map(poolId => (
  <div
    key={poolId}
    style={{
      ...POOL_PANE_BASE, // position:absolute, fill parent
      visibility: poolId === session.id ? "visible" : "hidden",
      pointerEvents: poolId === session.id ? "auto" : "none",
    }}
  >
    <TerminalOutput sessionId={poolId} baseUrl={...} isVisible={poolId === session.id} />
  </div>
))}
```

**The Browser tab must follow the same pattern** for the noVNC RFB connection. An RFB connection should not be torn down and re-established every time the user switches between Terminal and Browser tabs. Use the same `visibility`/`pointerEvents` approach with a noVNC component that accepts an `isVisible` prop (to pause/resume rendering updates when hidden).

### Dynamic Import (SSR Disabled)

`TerminalOutput` uses `next/dynamic` with `ssr: false`:
```tsx
const TerminalOutput = dynamic(
  () => import("./TerminalOutput").then(mod => mod.TerminalOutput),
  { ssr: false, loading: () => <div>Loading terminal...</div> }
);
```

The `BrowserTab` component must do the same — noVNC manipulates the DOM directly and requires a browser environment.

### WebSocket Proxy Pattern (Go)

`terminal_websocket.go` shows the established pattern for Go WebSocket proxying:
- `gorilla/websocket` upgrader
- Extract `session_id` from query params
- Look up session in storage
- Bidirectional copy via goroutines using a shared `done` channel
- Authentication via existing middleware (not duplicated in the handler itself)

A VNC proxy handler follows the exact same skeleton, replacing PTY read/write with `net.Dial("tcp", vncPort)` + `io.Copy`.

### CSS Architecture

New components use vanilla-extract `.css.ts` files (ADR-009). The `BrowserTab` component should have a `BrowserTab.css.ts` file importing tokens from `vars` in `theme.css.ts`.

### Feature Registry

Per `.claude/rules/feature-registry.md`, a new `browser-passthrough` entry must be added to:
- `docs/registry/backend-features.json` — for new RPC methods (e.g., `browser:status`, `browser:proxy`)
- `docs/registry/frontend-features.json` — for the `BrowserTab` component

---

## Summary of Key Design Decisions

### WebSocket Proxy

**Implement in Go directly** (no external websockify process). Use `gorilla/websocket` + `net.Dial` in a new `vnc_websocket.go` handler, mirroring `terminal_websocket.go`. Token/session routing via `session_id` query param; auth via existing middleware. ~50–80 lines.

### noVNC Integration

**Direct `@novnc/novnc` import** (Pattern B above), not `react-vnc` or iframe. Use `next/dynamic` with `ssr: false`, imperative `RFB` API inside a `useEffect`, attached to a `<div ref>`. Match the `TerminalOutput` component pattern exactly. Wrap in a `BrowserTab.tsx` component that accepts `sessionId`, `wsBaseUrl`, and `isVisible` props.

### Tab Addition

Extend `SessionDetailTab` union with `"browser"`. Add the tab entry conditionally: grey/disabled when no browser process is detected (`FR-6`). Use a polling or WebSocket-push endpoint (`/api/browser/status?session_id=...`) to determine detection state.

### Lifecycle Model

Follow Kasm's "one display per session unit" model. Xvfb + x11vnc processes are launched in `session.Instance.Start()` (or a new `BrowserManager` sub-component), tracked with PIDs, and killed in `Stop()`. Crash recovery (FR-9): up to 3 restarts via a goroutine with exponential backoff, analogous to the tmux session recovery logic.
