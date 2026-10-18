# Browser Element Inspector — Validation

## Validation Pass: Plan vs. Requirements Cross-Check

### AC-1: Inspect toggle visible only when CDP is streaming
- **Plan coverage**: Task 3.2 specifies `disabled when not isReady`; BrowserTab already tracks CDP streaming state via `vncState` / `isReady`.
- **Gap**: The toggle needs to check specifically for `CDPStatusStreaming`, not just `isReady` (which checks `VNC_STATUS_READY && browserWindowDetected`). The VNC status and CDP status may diverge. **Recommendation**: confirm that `isReady = true` in BrowserTab already implies CDP is in `Streaming` state — if not, add a `cdpStreaming` prop.
- **Verdict**: PLAUSIBLE. Likely safe for v1 since `VNC_STATUS_READY && browserWindowDetected` already implies CDP is streaming (the browser window is only "detected" when the CDPStreamManager transitions to Streaming).

### AC-2: Inspect mode suppresses normal mouse-click; hover draws highlight
- **Plan coverage**: Task 3.1 covers suppression of CDP mouse events when `inspectMode=true`; overlay canvas draws a rectangle on hover.
- **Gap**: The overlay canvas is "approximate" — it draws a fixed-size rectangle around the cursor position, not the actual element bounding box. The result panel's bounding box will disagree with what the hover showed.
- **Verdict**: CONFIRMED acceptable for v1. The requirements explicitly note that accurate bounding box is only available after a click. The hover rectangle is a UX cue, not an accurate selector.

### AC-3: Click sends to endpoint and opens result panel
- **Plan coverage**: Task 3.2's `onInspectClick` handler calls `fetch(POST /api/sessions/{id}/inspect-element)`. Task 2.1 defines the handler.
- **Gap**: The fetch URL in the frontend is constructed as `/api/sessions/${sessionId}/inspect-element`. The existing `buildWsUrl` in BrowserTab converts the `baseUrl` prop to a WebSocket URL; the REST base URL path needs to be constructed separately (it's different from the WS URL). Confirm that `baseUrl` can be used to derive the REST URL, or add a separate `apiBaseUrl` prop.
- **Verdict**: CONFIRMED risk. `baseUrl` currently ends in `/api` (see `BrowserTab.tsx` JSDoc). So `${baseUrl}/sessions/${sessionId}/inspect-element` is correct. No new prop needed.

### AC-4: Result panel shows screenshot, outerHTML, key CSS
- **Plan coverage**: Task 3.2 describes the result panel with all three sections. Task 1.1 returns all three fields from the Go backend.
- **Gap**: If `screenshotBase64` is empty (zero-area element, or screenshot failed), the panel must show a placeholder. This is specified in NFR-4 but not explicitly called out in Task 3.2.
- **Action**: Add to Task 3.2: show a "Screenshot not available" placeholder when `screenshotBase64` is empty.

### AC-5: "Send to Agent" injects context into terminal
- **Plan coverage**: Task 3.2 describes formatting and sending via `StreamTerminal` RPC.
- **Gap**: The full `screenshotBase64` JPEG is specified as attached "as base64 JPEG in context" with "exact format TBD based on the agent program." The plan says "formatted text block" but does not commit to the image attachment format. For Claude Code, the terminal does not natively accept inline base64 images — it accepts text only.
- **Resolution**: In v1, do not inject the base64 image into the terminal. Instead: inject the text block (HTML + CSS); the cropped screenshot remains visible only in the result panel (a side-channel reference the user can see while talking to the agent). Log the limitation as a known v1 constraint.
- **Impact**: Removes the base64 image injection from AC-5's scope. Requirements need a small clarification (see below).

### AC-6: Escape closes panel and deactivates Inspect mode
- **Plan coverage**: Task 3.2 mentions "✕ Close" button. Escape key handling is NOT explicitly coded in the plan.
- **Action**: Add to Task 3.2: `useEffect` listening for `keydown` event on `document` (or the canvas) to call `setInspectMode(false)` + `setInspectResult(null)` on `Escape`.

### AC-7: 503 when CDP not connected
- **Plan coverage**: Task 2.1 checks `Port() == 0` and returns 503. Also covers the case where the manager is a `noopCDPManager`.
- **Gap**: The check is `Port() == 0`. But `Port()` returns 0 for both "CDPStatusUnavailable" and "CDPStatusNoBrowser" (browser not yet launched). The 503 is appropriate for both. ✓

### AC-8: No regression to existing CDPViewer / VNC
- **Plan coverage**: CDPViewer changes are additive (new `inspectMode` prop defaults to `false`); all existing behaviour preserved when prop is absent. VNC proxy handler is untouched. HTTP route is new, not replacing anything.
- **Gap**: The overlay canvas added to CDPViewer must not intercept pointer events in normal mode (non-inspect). `pointer-events: none` on the overlay canvas is required. Confirmed in Task 3.1 design.

### AC-9: Documentation
- **Plan coverage**: Task 4.3 updates both `docs/features.md` and `README.md`.
- **Gap**: `docs/features.md` may not exist yet. Check before assuming. If absent, create a minimal stub with just the inspector entry.
- **Action**: Add to Task 4.3: check if `docs/features.md` exists; create if missing.

---

## Additional Risks Surfaced in Validation

### V-1: CDPStreamManager interface extension not required
The plan does NOT add any new methods to the `CDPStreamManager` interface. The inspect handler only uses `Port()` (already in the interface) to get the CDP port number and then opens an independent connection. This is correct — no interface pollution.

### V-2: `noopCDPManager` is automatically handled
`noopCDPManager.Port()` returns 0 → handler returns 503. No special case needed.

### V-3: Race between inspect fetch and session teardown
If the session is stopped while an inspect request is in flight, `r.Context()` is cancelled (the request context is tied to the HTTP server), and the CDP sub-calls respect context cancellation. The handler returns an error, the frontend shows "Inspection failed." No goroutine leak.

### V-4: `DOM.getNodeForLocation` requires enabling DOM domain first
The plan notes `DOM.enable` must be sent before `DOM.getNodeForLocation`.

However, `DOM.enable` is an event subscription; Chrome will start sending `DOM.setChildNodes` events (which the short-lived inspect connection will receive and must discard). The response-reading loop in `InspectAtPoint` must handle unexpected event messages gracefully (read until the expected `cmdID` is found, ignoring all other messages).

### V-5: Privacy — outerHTML may contain user data
The inspect payload (outerHTML, computed CSS, screenshot) reflects actual page content. This data is already visible to the session user in the browser (they're watching the live stream). Injecting it into the terminal is controlled by a button press. No privacy regression beyond what the user already sees.

---

## Updated Requirements Clarifications

1. **AC-5 (Send to Agent)**: In v1, only the text block (HTML + CSS) is injected into the terminal. The screenshot is visible in the result panel only — not injected. The base64 image attachment format is deferred to v2.
2. **AC-6 (Escape key)**: Must close the result panel AND deactivate Inspect mode.
3. **AC-4 supplemental**: When `screenshotBase64` is empty, show "Screenshot unavailable" placeholder in the result panel.
4. **FR-3 supplemental**: If `DOM.getNodeForLocation` returns `nodeId == 0`, the endpoint returns 200 with `{"notFound": true}` and the panel shows "No element found at this position."

---

## Go/No-Go Verdict

**GO**: The plan is implementable against the existing codebase with no architectural changes. All critical paths are covered. The v1 scope is well-bounded (no interface changes, no new RPCs, no new proto). Three open items require minor plan additions (Escape handler, screenshot placeholder, `docs/features.md` existence check) — all low risk.
