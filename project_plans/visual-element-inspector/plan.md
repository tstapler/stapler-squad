# Browser Element Inspector — Implementation Plan

## Architecture Summary

The feature adds an Inspect mode to the existing CDP Browser Passthrough tab. When active, clicking the canvas:
1. Sends the scaled (x, y) coordinates to a new Go HTTP endpoint.
2. The Go handler opens a short-lived CDP WebSocket to Chrome (independent of the screencast connection) and runs DOM/CSS/screenshot queries.
3. Returns structured result JSON to the frontend.
4. The frontend shows a result panel with HTML snippet, CSS, and cropped screenshot.
5. "Send to Agent" injects formatted context into the session terminal using the existing terminal-input path.

Hover highlighting in v1 uses a client-side canvas overlay (no additional protocol changes needed).

---

## Phase 1: Go Backend — CDP Inspect Helper

### Task 1.1: `session/cdp/inspect.go` — Short-Lived Inspect Connection

Create `InspectAtPoint(ctx context.Context, port int, x, y int) (*InspectResult, error)`:

```
InspectResult {
    OuterHTML        string         // truncated to 8 KB
    ComputedCSS      []CSSProperty  // filtered to ≤ 50 layout/visual props
    ScreenshotBase64 string         // base64-encoded JPEG, omitted if clip fails
    BoundingBox      BoundingBox    // {x, y, width, height} in device pixels
}
```

Implementation steps:
1. `GET http://127.0.0.1:{port}/json/version` → parse `webSocketDebuggerUrl`.
2. Dial CDP WebSocket with a 3-second context deadline.
3. Enable domains: `DOM.enable`, `CSS.enable` (send, no response required).
4. Send `DOM.getNodeForLocation({x, y, includeUserAgentShadowDOM: false})` with unique `cmdID`; read messages until response ID matches.
5. Extract `nodeId` from response. If `nodeId` == 0, return `ErrNoElementAtPoint`.
6. Send `DOM.getBoxModel({nodeId})`; parse `model.content` quad → bounding box.
7. Send `DOM.getOuterHTML({nodeId})`; truncate to 8 KB.
8. Send `CSS.getComputedStyleForNode({nodeId})`; filter to layout/visual set; truncate to 50 entries.
9. Send `Page.captureScreenshot({format:"jpeg", quality:80, clip:{x,y,width,height,scale:1}})` if bounding box is non-degenerate.
10. Close WebSocket. Return result.

Error handling: each send+read step has a 1-second sub-deadline. If any step fails, return a partial result with a non-nil error describing which step failed (the handler logs but returns 200 with whatever partial data exists, or 503 if nodeId lookup failed entirely).

### Task 1.2: Unit Tests — `session/cdp/inspect_test.go`

- Mock CDP WebSocket server using `net/http/httptest` + `gorilla/websocket`.
- Test: normal element found → all fields populated.
- Test: nodeId == 0 → `ErrNoElementAtPoint`.
- Test: zero-area bounding box → screenshot omitted.
- Test: `DOM.getOuterHTML` returns > 8 KB → result is truncated with marker.
- Test: context cancelled mid-flight → returns error without panic.

---

## Phase 2: Go Backend — HTTP Handler

### Task 2.1: `server/services/inspect_element_handler.go`

```go
// POST /api/sessions/{id}/inspect-element
// Body: {"x": int, "y": int}
type InspectElementHandler struct { finder InstanceFinder }

func (h *InspectElementHandler) ServeHTTP(w http.ResponseWriter, r *http.Request)
```

Flow:
1. Parse `sessionID` from path.
2. `finder.FindInstance(sessionID)` → if nil, 404.
3. `inst.CDPManager().Port()` → if 0, 503 `{"error": "CDP not connected"}`.
4. `cdp.InspectAtPoint(r.Context(), port, x, y)` with 3-second deadline.
5. If `ErrNoElementAtPoint`, 200 + `{"html": "", "notFound": true}`.
6. Otherwise, 200 + JSON-encoded `InspectResult`.

### Task 2.2: Register Route in `server/server.go`

```go
inspectEl := services.NewInspectElementHandler(deps.ReviewQueuePoller)
srv.mux.HandleFunc("POST /api/sessions/{id}/inspect-element", inspectEl.ServeHTTP)
```

### Task 2.3: Handler Tests — `server/services/inspect_element_handler_test.go`

- CDP unavailable → 503.
- Session not found → 404.
- Bad JSON body → 400.
- Normal case → 200 with populated fields.

---

## Phase 3: Frontend — CDPViewer Inspect Mode

### Task 3.1: `CDPViewer.tsx` — Add `inspectMode` Prop and Overlay Canvas

New props:
```ts
interface CDPViewerProps {
  ...
  inspectMode?: boolean;
  onInspectClick?: (x: number, y: number) => void;
}
```

Changes:
- Add an overlay `<canvas>` (absolute-positioned, pointer-events: none except in Inspect mode) for hover highlight drawing.
- In Inspect mode, `onMouseMove`:
  - Draw a light blue rectangle on the overlay canvas at the hover position (simple bounding-box approximation in v1; no CDP Overlay domain call needed).
  - Do NOT send `Input.dispatchMouseEvent` to Chrome.
- In Inspect mode, `onMouseDown`:
  - Call `onInspectClick(x, y)` with scaled coordinates.
  - Suppress CDP mouse event dispatch.
- In normal mode, clear the overlay canvas and restore default behaviour.

### Task 3.2: `BrowserTab.tsx` — Inspect Toolbar and Result Panel

New state:
```ts
const [inspectMode, setInspectMode] = useState(false);
const [inspectResult, setInspectResult] = useState<InspectResult | null>(null);
const [inspecting, setInspecting] = useState(false); // loading
```

New UI:
- Toolbar bar above the canvas: "Inspect" toggle button (icon: `<svg ...>` cursor/magnifier), disabled when not `isReady`. Keyboard shortcut label `I`.
- On `onInspectClick(x, y)`:
  1. Set `inspecting = true`.
  2. `fetch(`/api/sessions/${sessionId}/inspect-element`, { method: 'POST', body: JSON.stringify({x, y}) })`.
  3. Set `inspectResult = await res.json()`.
  4. Set `inspecting = false`.
- Result panel (drawer right side or bottom panel):
  - Shown when `inspectResult !== null`.
  - Element screenshot (small `<img>` from base64 JPEG).
  - HTML snippet in `<pre><code>` (syntax highlighted via existing libs if available, otherwise plain monospace).
  - Compact CSS table (filtered properties).
  - "Send to Agent" button.
  - "✕ Close" button.
- "Send to Agent":
  - Formats text: `[Browser Inspector] Element at (x, y):\nHTML: ...\nCSS: ...`
  - Sends via `StreamTerminal` RPC using the existing terminal WebSocket (not a new RPC; re-use the same `useTerminalStream` hook or equivalent already in the session view).
  - Does NOT send a trailing `\n` — leaves user to confirm.

### Task 3.3: `BrowserTab.css.ts` — New Style Tokens

- `.toolbar` — flex row, 28px height, border-bottom, semi-transparent background.
- `.inspectButton` — icon button with active/disabled variants.
- `.resultPanel` — absolute or side-drawer, 300px wide, scrollable, z-index above canvas.
- `.resultScreenshot` — max-width 100%, border-radius.
- `.resultCode` — monospace, font-size 0.75rem, overflow-x scroll.
- `.cssTable` — compact two-column table.

---

## Phase 4: Integration and Tests

### Task 4.1: Frontend Tests

- `CDPViewer.test.tsx`: Inspect mode prop suppresses CDP mouse dispatch; `onInspectClick` fires on canvas click.
- `BrowserTab.test.tsx`: Inspect button disabled when not ready; fetch called on inspect click; result panel appears; "Send to Agent" button present; Close button hides panel.

### Task 4.2: E2E Test Spike (`tests/e2e/browser-inspector.spec.ts`)

Lightweight spec (marked `@feature browser:inspect`):
- Open a session with a running CDP Chrome stub (existing test server infrastructure).
- Click "Inspect" toggle; hover over canvas; verify `data-testid="inspect-result-panel"` absent before click.
- Click canvas; mock the inspect endpoint (MSW or test server); verify result panel appears.
- Click "Send to Agent"; verify terminal input contains `[Browser Inspector]`.
- Press `Escape`; verify panel hidden.

### Task 4.3: Documentation

- `docs/features.md`: Add entry "Browser Element Inspector" with prerequisites (Chrome/CDP required), how to use, and what is captured.
- `README.md`: Update Browser Passthrough section to mention Inspect mode.

---

## Adversarial Review Notes

**Reviewer A** raised:
- *"Hover highlighting on a JPEG canvas is lossy — you can't accurately highlight an element without its actual bounding box."* — **Accepted**. v1's hover highlight is approximate (simple rectangle under cursor). The exact bounding box is returned only after a click. This is acceptable and clearly documented as "approximate" in the UI.

**Reviewer B** raised:
- *"Opening a new CDP WebSocket per inspect click will create latency and may saturate Chrome's connection limit under rapid repeated clicking."*  
  — **Mitigated**: Use `AbortController` in the frontend to cancel the in-flight fetch when a new click happens. The handler respects context cancellation (`r.Context()`), which cancels the CDP sub-calls. Chrome's limit is generous (dozens of simultaneous debugger sessions); one-at-a-time via cancellation is safe.

**Reviewer C** raised:
- *"CSS.getComputedStyleForNode returns hundreds of properties — the 50-property cap is arbitrary and may miss critical ones."*  
  — **Resolution**: The cap is on the *total* properties returned; the filter list (display, position, width, height, color, background-color, font-size, font-family, border, margin, padding — 11 named properties) always appear first and are never dropped. The remaining 39 slots are filled with any other properties that appear in the response, preserving full context for unusual cases.

---

## Dependency Map

```
Phase 1 (Go: cdp/inspect.go) ──────────────────────────────┐
Phase 2 (Go: HTTP handler) ←── depends on Phase 1           │
Phase 3 (Frontend) ←── depends on Phase 2 endpoint path     │
Phase 4 (Tests + Docs) ←── depends on Phase 1 + 2 + 3      │
```

Phases 1 and 3 (frontend CDPViewer changes) can proceed in parallel; Phase 2 depends on Phase 1; Phase 4 depends on all prior phases.

## Estimated Cost (agent-wave units)

| Phase | Waves | Notes |
|---|---|---|
| Phase 1: cdp/inspect.go | 1 | Single file + tests |
| Phase 2: HTTP handler | 1 | Small handler + server.go wiring |
| Phase 3: Frontend | 2 | CDPViewer changes + BrowserTab panel |
| Phase 4: Tests + Docs | 1 | E2E spec + doc edits |
| **Total** | **5 waves** | |

---

## Implementation Deviations (recorded during Phase 5)

- Short-lived connection targets the first `page` entry from `/json/list`, not `/json/version`'s browser endpoint (the browser endpoint does not serve DOM/CSS).
- `DOM.getNodeForLocation` yields a `backendNodeId`; the code calls `DOM.getDocument` then `DOM.pushNodesByBackendIdsToFrontend` to obtain a usable `nodeId`.
- "Send to Agent" uses the existing `WriteToSession` RPC with `pressEnter=false` rather than the terminal stream hook (which lives inside `TerminalOutput`).
- `InspectAtPoint` takes a `cdp.Point` instead of two ints. Handler returns 502 for unexpected CDP failures, 400 for a bad body.
