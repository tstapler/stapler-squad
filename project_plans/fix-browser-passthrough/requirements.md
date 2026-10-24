# Fix Browser Passthrough — Requirements

## Problem Statement

The browser passthrough feature (Browser tab in session detail view) is broken on both
macOS and Linux. The symptom: the Browser tab is permanently disabled, and hovering over
it shows a stale tooltip — "Browser passthrough requires Linux with Xvfb, x11vnc, and
xdotool" — even though the implementation was migrated from VNC/noVNC to CDP
(Chrome DevTools Protocol) screencasting (see ADR-016).

The root cause is a front-end signal mismatch: `SessionDetailView.tsx` derives the
`isBrowserAvailable` flag from `session.vncState.status` (the legacy VNC state), while
the actual streaming subsystem is driven by `session.cdpState.status` (the CDP state).
On macOS the VNC manager always returns `VNC_STATUS_UNAVAILABLE` (no Xvfb), so the tab
is permanently disabled even though Chrome + CDP work natively on macOS. On Linux the
tab only activates when xdotool detects a Chrome window and x11vnc is restarted — a
condition entirely unrelated to whether the CDP stream is flowing.

## Functional Requirements

### FR-1: Frontend reads CDPState, not VNCState, for browser tab availability
- `SessionDetailView` SHALL derive `isBrowserAvailable` from `session.cdpState.status`
  (`CDP_STATUS_STREAMING`, `CDP_STATUS_WAITING`, or `CDP_STATUS_NO_BROWSER`) instead of
  from `session.vncState.status`.
- The Browser tab SHALL be enabled (not disabled) whenever `cdpState.status !== CDP_STATUS_UNAVAILABLE && cdpState.status !== CDP_STATUS_UNSPECIFIED`.
- The Browser tab SHALL be disabled/hidden when `cdpState.status === CDP_STATUS_UNAVAILABLE` (Chrome not installed) or `CDP_STATUS_UNSPECIFIED` (CDP not initialized).

### FR-2: BrowserTab component reads CDPState for streaming readiness
- `BrowserTab` SHALL receive `cdpState` (type `CDPState | undefined`) instead of
  `vncState` (type `VNCState | undefined`).
- `isReady` SHALL be `cdpState?.status === CDPStatus.CDP_STATUS_STREAMING`.
- `isWaiting` SHALL be true when `cdpState` is `CDP_STATUS_WAITING` or
  `CDP_STATUS_NO_BROWSER`.
- `isUnavailable` SHALL be true when `cdpState` is absent, `CDP_STATUS_UNAVAILABLE`, or
  `CDP_STATUS_UNSPECIFIED`.
- The `hasBeenReadyRef` keep-alive pattern SHALL remain (CDP streaming is also transient).

### FR-3: Corrected disabled-tab tooltip message
- The tooltip text for a disabled Browser tab SHALL read:
  "Browser passthrough requires Chrome or Chromium installed on the host"
  (not the legacy "requires Linux with Xvfb, x11vnc, and xdotool").
- This copy SHALL be accurate for both Linux (where Chrome plus optional Xvfb is used)
  and macOS (where Chrome runs on the native display, no Xvfb).

### FR-4: CDPViewer placeholder messages updated for CDP context
- The "starting" placeholder in `BrowserTab` when `CDP_STATUS_WAITING` SHALL read
  "Connecting to browser..." (not "Starting virtual display...").
- The "waiting" placeholder when `CDP_STATUS_NO_BROWSER` SHALL read
  "No browser detected — run Chrome or Chromium in this session".

### FR-5: macOS support (no regression)
- On macOS, when Chrome is installed and `cdpState.status === CDP_STATUS_STREAMING`, the
  Browser tab SHALL be enabled and the CDPViewer SHALL connect and render frames.
- The VNC manager is still a noop on macOS (no Xvfb) — the fix does NOT change the
  VNC subsystem; it only changes which proto field the frontend reads.

### FR-6: Linux support parity
- On Linux, the Browser tab availability SHALL be driven by CDP state only.
- The VNC subsystem (Xvfb + x11vnc + xdotool) MAY still run for the virtual display
  and the `vnc_state` field in the proto may still be populated — but the frontend
  SHALL NOT use `vnc_state` to determine whether the Browser tab is enabled or whether
  the CDPViewer is in a "ready" state.
- If in a future sprint the VNC subsystem's browser-window-detection signal is wanted
  by the UI (e.g. to auto-switch to the Browser tab), it can be added as an additive
  enhancement without regressions.

### FR-7: No change to backend or proto
- No Go code, proto schema, or generated proto bindings need to change for this fix.
- `session.cdpState` is already populated in `instance_adapter.go` and present on the
  proto `Session` message (field 52 `cdp_state`), and `make build` regenerates the TS
  bindings. Only the React component layer needs to be updated.

## Non-Functional Requirements

### NFR-1: No existing tests broken
- All existing `BrowserTab.test.tsx` and `SessionDetailView.test.tsx` tests SHALL pass
  after updating the prop type from `vncState` to `cdpState`.
- Test fixtures SHALL be updated to pass `CDPState` objects instead of `VNCState` objects.

### NFR-2: Tooltip copy accessible on mobile
- The `showDisabledTabHint` touch-tap path in SessionDetailView SHALL display the
  updated tooltip copy ("Chrome or Chromium") so mobile users receive the correct
  dependency guidance.

## Out of Scope

- Adding the CDP inspect feature (already implemented, unchanged).
- VNC subsystem changes (Xvfb, x11vnc, xdotool management unchanged).
- Quality toggle UI (already implemented via CDPViewer, unchanged).
- macOS native display passthrough via any mechanism other than CDP (future).
- Audio passthrough.
- ADR-016 authoring (that ADR is already referenced in requirements.md; the ADR document itself is outside scope of this fix).

## Success Criteria

1. On macOS with Chrome installed: the Browser tab is enabled when a session is active
   and becomes fully interactive (CDPViewer renders frames) once Chrome starts.
2. On Linux with Chrome installed: the Browser tab activates when `cdpState.status`
   reaches `CDP_STATUS_STREAMING` — independent of whether xdotool has detected a window
   or x11vnc is in `-id` mode.
3. Hovering over a disabled Browser tab shows "Browser passthrough requires Chrome or
   Chromium installed on the host", not the old Xvfb message.
4. The "Connecting to browser..." placeholder appears while `CDP_STATUS_WAITING`.
5. Existing unit tests pass with updated prop types; new tests cover the CDP-gated
   availability path.
6. No backend Go code, proto schema, or generated files are modified.
