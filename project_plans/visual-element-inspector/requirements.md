# Browser Element Inspector — Requirements

## Problem Statement

Browser Passthrough streams the live browser via CDP screencasting (`cdp_stream_handler.go`), giving users a read/write view of the agent's browser session. The current flow is one-directional for *context*: users can watch and control Chrome, but there is no way to point at a specific element on screen and send its markup to the agent. Describing a layout bug in prose is lossy; a user spotting a misaligned button mid-review should be able to click it and inject exact, grounded HTML/CSS context into the agent's prompt in one action.

Orca implements this as "Design Mode": click a UI element in the embedded Chromium window and its HTML/CSS plus a cropped screenshot are sent directly into the agent's prompt. This feature brings equivalent capability to Stapler Squad's Browser Passthrough tab.

## Functional Requirements

### FR-1: Inspect Mode Toggle
- The Browser tab toolbar SHALL include an "Inspect" toggle button (keyboard shortcut: `I`).
- While Inspect mode is **active**: hovering over the canvas draws a highlight box around the element under the cursor; normal mouse-input dispatch to Chrome is suppressed.
- While Inspect mode is **inactive**: the canvas behaves exactly as it does today (full mouse/keyboard pass-through to Chrome via CDP).

### FR-2: Element Highlight on Hover
- In Inspect mode, mousemove events SHALL send `Overlay.highlightNode` (or equivalent) CDP commands to Chrome so Chrome draws a native DevTools highlight over the hovered element.
- The highlight SHALL update in real time as the cursor moves across the canvas.
- Falling back to a client-side canvas overlay rectangle is acceptable if Chrome's `Overlay` domain is not enabled in the current target.

### FR-3: Inspect Click — Capture Payload
- Clicking on the canvas in Inspect mode SHALL issue the following CDP queries to Chrome using the (x, y) canvas coordinates already scaled to Chrome device pixels:
  1. `DOM.getNodeForLocation({x, y, includeUserAgentShadowDOM: false})` → nodeId
  2. `DOM.getOuterHTML({nodeId})` → outerHTML (truncated to ≤ 8 KB)
  3. `CSS.getComputedStyleForNode({nodeId})` → computed CSS properties (filtered to layout/visual properties, ≤ 50 entries)
  4. `Page.captureScreenshot({format: "jpeg", quality: 80, clip: {<nodeId bounding box>}})` → base64-encoded cropped JPEG
- The Go server SHALL perform all four calls over a **short-lived CDP WebSocket connection** opened for the duration of the request, independent of the main screencast connection in `cdp/manager.go`.
- Coordinate scaling (CSS pixels → Chrome device pixels) already in `CDPViewer.tsx` (`getCanvasCoords`) SHALL be applied before sending.

### FR-4: Backend Inspect Endpoint
- A new HTTP endpoint `POST /api/sessions/{id}/inspect-element` SHALL accept:
  ```json
  { "x": int, "y": int }
  ```
  (coordinates in Chrome device pixels, already scaled by the frontend).
- It SHALL return:
  ```json
  {
    "html":             "...",
    "computedCss":      [{"name": "...", "value": "..."}, ...],
    "screenshotBase64": "...",
    "boundingBox":      {"x": float, "y": float, "width": float, "height": float}
  }
  ```
- The endpoint SHALL be authenticated by the existing session-auth middleware.
- If Chrome's CDP is not connected (CDPStatus ≠ Streaming), the endpoint SHALL return HTTP 503.
- The response SHALL be returned within 3 seconds; individual CDP sub-calls SHALL each have a 1-second context deadline.

### FR-5: Inspect Result Panel
- The Browser tab SHALL display an inspect result panel (slide-in drawer or right-side pane) when a capture result is available.
- The panel SHALL show:
  - A cropped screenshot of the element (from `screenshotBase64`).
  - The `outerHTML` snippet in a syntax-highlighted code block (scrollable, truncated with "…" if > 8 KB).
  - Key computed CSS properties (display, position, width, height, color, background-color, font-family, font-size, border, margin, padding) in a compact list.
- Clicking elsewhere in the canvas (outside Inspect mode) or pressing `Escape` SHALL close the panel and return to normal input mode.

### FR-6: Send to Agent
- The inspect result panel SHALL include a "Send to Agent" button.
- Pressing it SHALL inject a formatted text block into the session's terminal input (using the existing `StreamTerminal` → `TerminalInput` path) without submitting (no trailing newline), leaving the user free to add additional context before pressing Enter.
- The injected text format:
  ```
  [Browser Inspector] Element at ({x}, {y}):
  HTML: `{outerHTML (≤ 500 chars preview)}`
  CSS: display:{v}; position:{v}; width:{v}; height:{v}; color:{v}; background-color:{v}
  Screenshot: [attached as base64 JPEG in context]
  ```
- The full `screenshotBase64` JPEG SHALL be attached separately as a file reference or inline image marker (exact format TBD based on the agent program in use — Claude Code supports inline image markdown; other agents may not).

### FR-7: Inspect Mode Only Available When CDP is Streaming
- The Inspect toggle button SHALL be disabled (greyed out with a tooltip "Browser not connected") when the CDP status is not `streaming`.
- If CDP disconnects while Inspect mode is active, Inspect mode SHALL deactivate automatically.

### FR-8: Works Only via CDP Path (Not VNC)
- The feature is only defined for the CDP path (CDPViewer). The legacy VNC/noVNC path has no DOM access and SHALL remain unmodified.
- The Inspect button SHALL not appear at all when the session's VNC status is `VNC_STATUS_UNAVAILABLE` and there is no CDP connection.

## Non-Functional Requirements

### NFR-1: Latency
- The full inspect round-trip (click → result panel populated) SHALL complete in ≤ 3 seconds on a local host.
- The Go endpoint SHALL use a per-request 3-second context deadline; individual CDP calls SHALL be bounded at 1 second each.

### NFR-2: Non-Interference with Screencast
- The short-lived CDP inspection connection SHALL be a separate WebSocket to Chrome from the persistent screencast connection in `cdp/manager.go`.
- It SHALL NOT interfere with screencast frame delivery or input dispatch.

### NFR-3: Security
- The inspect endpoint falls under the existing `/api/` auth middleware — no new auth surface.
- The CDP port is bound to `127.0.0.1` already; the endpoint does not expose the port externally.

### NFR-4: Graceful Degradation
- If `DOM.getNodeForLocation` returns no node (e.g. click on empty area), the panel SHALL show "No element found at this location" without error.
- If the screenshot clip fails (zero-area bounding box), the panel SHALL omit the screenshot and show a placeholder.

## Out of Scope

- Multi-element selection or selection by CSS selector (v1: single-click only).
- Automatic re-inspection on page navigation.
- Clipboard-only copy without prompt injection.
- Animated highlight / hover history.
- Support for iframes or shadow-DOM elements (noted as a known limitation in v1).

## Acceptance Criteria

- [ ] **AC-1** An "Inspect" toggle is visible in the Browser tab toolbar when CDP is streaming; it is disabled/hidden otherwise.
- [ ] **AC-2** Entering Inspect mode suppresses normal mouse-click input to Chrome; hovering draws a highlight over the element under the cursor.
- [ ] **AC-3** Clicking an element in Inspect mode calls `POST /api/sessions/{id}/inspect-element` and opens the result panel.
- [ ] **AC-4** The result panel displays: cropped element screenshot, outerHTML snippet, and key computed CSS properties.
- [ ] **AC-5** Pressing "Send to Agent" injects the formatted context block into the session's terminal input without submitting it.
- [ ] **AC-6** Pressing `Escape` or clicking outside the canvas closes the result panel and deactivates Inspect mode.
- [ ] **AC-7** The inspect endpoint returns HTTP 503 (with clear message) when CDP is not connected.
- [ ] **AC-8** The feature does not regress any existing CDPViewer, BrowserTab, or VNC proxy behaviour.
- [ ] **AC-9** Documented in `docs/features.md` and the Browser Passthrough section of `README.md`.
