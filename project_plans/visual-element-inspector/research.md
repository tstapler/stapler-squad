# Browser Element Inspector — Research

## 1. Existing Infrastructure Audit

### 1.1 CDP Manager (`session/cdp/manager.go`)

The live `cdpStreamManager` holds one persistent CDP WebSocket connection to Chrome, used exclusively for:
- `Page.enable` + `Page.startScreencast` on connect
- `Page.screencastFrameAck` responses
- `Input.dispatch*` commands forwarded from `DispatchInput()`

The `readLoop` currently ignores **all CDP response messages** (those with an `id` field) and only processes the `Page.screencastFrame` event. The command `sendCommand()` is fire-and-forget — it writes a CDP command but never reads the result.

**Consequence**: Adding request/response CDP commands (e.g. `DOM.getNodeForLocation`) to the existing connection would require adding a pending-command map keyed by cmdID plus a response fanout path in `readLoop`. This is non-trivial and risks interfering with the high-frequency screencast frame flow.

**Resolution**: Open a **separate, short-lived CDP WebSocket connection** per inspect request. Chrome allows multiple simultaneous CDP clients on the same browser. This keeps the screencast connection untouched and makes each inspect call stateless and independently cancellable.

### 1.2 CDP WebSocket Discovery

Chrome exposes: `GET http://127.0.0.1:{port}/json/version` → `{"webSocketDebuggerUrl": "ws://…"}`. The existing `pollForChrome()` already does this. A new helper `openCDPConn(ctx, port) (*websocket.Conn, error)` can reuse the same HTTP call pattern from the existing `cdpStreamManager`.

### 1.3 Existing Input Path (`server/services/terminal_access.go`)

`paneWriter.SendInput(ctx, PaneInput{Data: []byte("..."), ...})` → `tmux send-keys` or control-mode. This is already wired and used by `connectrpc_websocket.go` for terminal input. The inspect feature can call this directly from the new HTTP handler (it needs an `InstanceFinder` to locate the instance and obtain its tmux socket + session name).

### 1.4 Frontend Canvas Coordinate Scaling

`CDPViewer.tsx`'s `getCanvasCoords(e)` already maps CSS-pixel mouse events to Chrome device pixels:
```ts
const scaleX = canvas.width / rect.width;
const scaleY = canvas.height / rect.height;
```
The inspect click handler in the frontend can use the same function and send already-scaled coordinates to the backend, so the backend does not need to know the canvas/viewport sizes.

### 1.5 BrowserTab Structure (`web-app/src/components/sessions/BrowserTab.tsx`)

`BrowserTab` currently renders:
- A status/placeholder overlay (isUnavailable, isWaiting states)
- `CDPViewer` (canvas-based frame renderer)

A toolbar area is not yet present. Adding one above or overlaid on `CDPViewer` is straightforward; CSS architecture (`vanilla-extract`) uses `BrowserTab.css.ts`.

## 2. CDP Domain Requirements

The following CDP commands are needed for the inspect operation:

| Command | Domain | Purpose |
|---|---|---|
| `DOM.enable` | DOM | Enables the DOM domain (required before other DOM calls) |
| `CSS.enable` | CSS | Enables the CSS domain |
| `Overlay.enable` | Overlay | Enables native element highlighting |
| `DOM.getNodeForLocation` | DOM | Returns the deepest node at (x, y) coordinates |
| `DOM.getOuterHTML` | DOM | Returns element's full outer HTML |
| `CSS.getComputedStyleForNode` | CSS | Returns computed CSS properties |
| `DOM.getBoxModel` | DOM | Returns bounding box / content/padding/border/margin quads |
| `Page.captureScreenshot` | Page | Takes a JPEG screenshot with optional clip rectangle |
| `Overlay.highlightNode` | Overlay | Draws DevTools highlight on a node |
| `Overlay.hideHighlight` | Overlay | Removes the highlight |

`DOM.getNodeForLocation`, `DOM.getBoxModel`, `CSS.getComputedStyleForNode`, and `Page.captureScreenshot` are all **request/response** commands — they require reading the CDP response. These must run on the short-lived inspect connection.

`Overlay.highlightNode` / `Overlay.hideHighlight` are **fire-and-forget** (from the perspective of the client — Chrome sends no response). However, they must be sent on the **main screencast connection** (or a persistent overlay connection) because the highlight must remain visible across frames from the screencast. Sending them on the short-lived inspect connection would cause the highlight to disappear as soon as that connection closes.

**Resolution for hover highlighting**: Send overlay commands through `cdpStreamManager.DispatchInput()` — which already exists and forwards arbitrary CDP JSON to Chrome. The frontend mousemove handler in Inspect mode sends `Overlay.enable` + `Overlay.highlightNode` messages as text WebSocket frames via the existing `cdp-stream` WebSocket.

## 3. Go Backend Design: Short-Lived CDP Connections

### 3.1 New `cdpSession` helper (in `session/cdp/` package)

A new file `session/cdp/inspect.go` provides:
```go
// InspectAtPoint connects to Chrome's CDP, runs the inspect queries for the
// element at (x, y), and returns the result. The connection is closed before returning.
func InspectAtPoint(ctx context.Context, port int, x, y int) (*InspectResult, error)

type InspectResult struct {
    OuterHTML        string            `json:"html"`
    ComputedCSS      []CSSProperty     `json:"computedCss"`
    ScreenshotBase64 string            `json:"screenshotBase64"`
    BoundingBox      BoundingBox       `json:"boundingBox"`
}
```

This helper:
1. Dials `ws://127.0.0.1:{port}/json/version` → gets `webSocketDebuggerUrl`
2. Dials the debugger URL → CDP WebSocket
3. Enables DOM + CSS domains
4. Calls `DOM.getNodeForLocation` with a pending-response pattern (send command, read messages until response ID matches)
5. Calls `DOM.getOuterHTML`, `CSS.getComputedStyleForNode`, `DOM.getBoxModel`, `Page.captureScreenshot` in sequence
6. Returns result; closes connection

The pending-response pattern in this helper is simple (synchronous, single-threaded) because there's no screencast loop competing for the same connection.

### 3.2 New HTTP Handler (`server/services/inspect_element_handler.go`)

```go
// POST /api/sessions/{id}/inspect-element
// Body: {"x": int, "y": int}
// Response: InspectResult JSON
func (h *InspectElementHandler) ServeHTTP(w http.ResponseWriter, r *http.Request)
```

Registered in `server.go` alongside the existing `cdp-stream` handler.

### 3.3 Sending to Terminal

An optional `POST /api/sessions/{id}/send-inspect-result` endpoint (or extending the inspect endpoint with a `"sendToAgent": true` flag) calls `paneWriter.SendInput` with the formatted text. Alternatively, the frontend can use the existing `StreamTerminal` RPC directly (it already handles `TerminalInput` messages). Using the frontend path is simpler and avoids a new RPC.

## 4. Frontend Design

### 4.1 Inspect Mode State in CDPViewer / BrowserTab

Add `inspectMode: boolean` prop and `onInspectResult` callback to `CDPViewer`. When `inspectMode=true`:
- `onMouseMove` sends `Overlay.highlightNode` via the existing `sendJson` path
- `onMouseClick` sends coordinates to `POST /api/sessions/{id}/inspect-element` (fetch call)
- Normal mouse events are suppressed

`BrowserTab` holds `inspectMode` state and the last `InspectResult`. It renders the toolbar toggle and the result panel.

### 4.2 Hover Highlighting via Existing WebSocket

To get the nodeId at hover-time without a round-trip, the frontend can:
1. On hover, send `{"method": "DOM.getNodeForLocation", "params": {"x": x, "y": y}}` via `sendJson` — BUT this requires the server to echo back the response. The current CDPViewer only sends binary frames from server to client; text messages from server are currently ignored (`ws.addEventListener('message', ...)` skips non-ArrayBuffer).
2. Alternatively (simpler for v1): skip hover highlight via CDP nodeId, instead **draw a client-side canvas overlay rectangle** on hover. This requires no protocol changes: on mousemove in Inspect mode, do a simple visual rectangle draw on a second canvas layer overlaid on the main frame canvas.

**Recommended for v1**: Client-side canvas hover highlight (no protocol change needed). CDP `Overlay.highlightNode` can be a v2 enhancement if desired.

### 4.3 Keyboard Shortcut: `I` to Toggle Inspect

The canvas's `onKeyDown` handler (already in `CDPViewer`) can be extended: when `Inspect mode` is requested from outside (prop), or when a `toggleInspect` callback is provided, pressing `I` while the canvas is focused toggles the mode.

## 5. Risks and Open Questions

| Risk | Likelihood | Mitigation |
|---|---|---|
| Chrome's `DOM.getNodeForLocation` returns null for non-interactive areas (e.g. `<html>`, `<body>`) | Medium | Fall back to parent node traversal or show "no inspectable element" |
| `CSS.getComputedStyleForNode` returns 400+ properties | High | Filter to a known set (≤ 50 layout/visual properties) |
| `Page.captureScreenshot` with clip fails for zero-size elements | Medium | Check bounding box before calling; omit screenshot if degenerate |
| Multiple inspect requests can race (user clicks fast) | Low | Cancel previous request on new click via `AbortController` |
| `DOM.getOuterHTML` is enormous for large elements | High | Truncate to 8 KB with a clear marker |
| Overlay commands sent via DispatchInput may be rejected if Chrome's Overlay domain isn't enabled | Medium | Enable in a fire-and-forget `DOM.enable`+`Overlay.enable` pair on first Inspect mode activation |
| Short-lived inspect connection interferes with screencast (Chrome CDP has per-target session limits) | Low | Each WS dial to `/json/version`'s `webSocketDebuggerUrl` is a new independent session |

## 6. Precedent: Orca Design Mode

Orca runs a real Chromium instance embedded in Electron. Its "Design Mode" uses CDP directly (same pattern) to:
1. Inject a `mousemove` listener into the page via `Runtime.evaluate` that calls back via `Runtime.consoleAPICalled` (a more complex approach we don't need).
2. On click, call `DOM.getNodeForLocation` + `DOM.getOuterHTML` + `CSS.getComputedStyleForNode`.
3. Render HTML in a side panel, attach to the agent's context window.

Our approach is simpler: we have a canvas rendering a JPEG stream, so we already know the pixel coordinates without injecting scripts. We don't need `Runtime.evaluate` at all.

## 7. Files to Create or Modify

| File | Change |
|---|---|
| `session/cdp/inspect.go` | NEW: `InspectAtPoint` helper + result types |
| `session/cdp/inspect_test.go` | NEW: unit tests with mock CDP WS server |
| `server/services/inspect_element_handler.go` | NEW: HTTP handler for `POST /api/sessions/{id}/inspect-element` |
| `server/services/inspect_element_handler_test.go` | NEW: handler unit tests |
| `server/server.go` | MODIFY: register new route |
| `web-app/src/components/sessions/CDPViewer.tsx` | MODIFY: add inspectMode prop, overlay canvas, inspect click |
| `web-app/src/components/sessions/BrowserTab.tsx` | MODIFY: add toolbar toggle, inspect result panel state |
| `web-app/src/components/sessions/BrowserTab.css.ts` | MODIFY: toolbar + result panel styles |
| `web-app/src/components/sessions/__tests__/CDPViewer.test.tsx` | MODIFY: add Inspect mode tests |
| `web-app/src/components/sessions/__tests__/BrowserTab.test.tsx` | MODIFY: add toolbar toggle tests |
| `docs/features.md` | MODIFY: document Browser Element Inspector |
| `README.md` | MODIFY: mention Inspect mode in Browser Passthrough section |
