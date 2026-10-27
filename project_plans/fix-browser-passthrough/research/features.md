# Fix Browser Passthrough — Features Research

## Existing Feature State

The browser passthrough feature is already fully implemented in the backend (CDP manager,
WebSocket relay, proto). The frontend implementation was migrated from VNC/noVNC to CDP,
but the availability signal was not updated from `vncState` to `cdpState`. This is a
pure wiring bug in the React component layer.

### What is already working correctly

- `session/cdp/manager.go`: CDPStreamManager lifecycle (allocate → start → poll → stream → stop)
- `session/instance_cdp.go`: wires CDP into session lifecycle; injects `CDP_PORT` and
  updated `PATH` into the tmux session environment
- `/api/sessions/{id}/cdp-stream` WebSocket endpoint: relays CDP screencast frames from
  Chrome to the browser UI
- `web-app/src/components/sessions/CDPViewer.tsx`: canvas-based JPEG frame renderer with
  input forwarding (mouse, keyboard), inspect mode, reconnect
- `proto/session/v1/types.proto`: both `VNCState` (field 51) and `CDPState` (field 52)
  are on `Session`; `CDPStatus` enum is complete
- `server/adapters/instance_adapter.go`: populates `protoSession.CdpState` on every
  session event

### What is broken (this PR's scope)

- `SessionDetailView.tsx:603-605`: reads `session.vncState` for `isBrowserAvailable`
  → should read `session.cdpState`
- `SessionDetailView.tsx:922`: tooltip says "requires Linux with Xvfb, x11vnc, and xdotool"
  → should say "requires Chrome or Chromium installed on the host"
- `BrowserTab.tsx:14-19,75-83`: accepts `vncState` prop and uses `VNCStatus` enum
  → should accept `cdpState` prop and use `CDPStatus` enum
- `BrowserTab.tsx:203`: renders `VNCStatus.VNC_STATUS_STARTING` branch with
  "Starting virtual display..." text
  → should use `CDP_STATUS_WAITING` with "Connecting to browser..."

## Impact Radius

Only 2 production source files need substantive changes:
- `web-app/src/components/sessions/SessionDetailView.tsx`
- `web-app/src/components/sessions/BrowserTab.tsx`

Test files updated accordingly:
- `web-app/src/components/sessions/BrowserTab.test.tsx`
- Any `SessionDetailView` tests that assert browser tab visibility state

No backend, proto, infra, or doc changes required.
