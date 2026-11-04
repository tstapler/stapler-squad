# Fix Browser Passthrough — Pitfalls

## P1: Generated proto bindings not committed

`web-app/src/gen/` is gitignored. `CDPStatus` and `CDPState` types exist in the proto
but the TS bindings are only present after `make build`. The implementation must not
assume the types are available in the worktree; `make build` must be run first and the
types verified before writing the import.

**Mitigation:** Run `make build` in the worktree before editing; confirm
`web-app/src/gen/session/v1/types_pb.ts` contains `CDPStatus` and `CDPState`.

## P2: VNCState export used by callers outside BrowserTab

`BrowserTab.tsx` currently exports `VNCState` and `VNCStatus`. If any other component
imports these from `BrowserTab` (rather than directly from the proto gen), removing the
exports will break them.

**Mitigation:** Before removing the exports, grep for `from.*BrowserTab` to confirm
no other file imports those re-exports. If found, update those imports to source from
`@/gen/session/v1/types_pb` directly.

## P3: BrowserTab.test.tsx fixtures

The existing BrowserTab Jest tests pass `vncState` objects as props. Changing the prop
name to `cdpState` without updating the tests will produce TypeScript type errors and
Jest failures.

**Mitigation:** Update all test fixtures to use `CDPState` objects with `CDPStatus`
enum values. The mapping is 1:1:
- `VNC_STATUS_UNSPECIFIED` → `CDP_STATUS_UNSPECIFIED`
- `VNC_STATUS_NO_BROWSER` → `CDP_STATUS_NO_BROWSER`
- `VNC_STATUS_STARTING` → `CDP_STATUS_WAITING`
- `VNC_STATUS_READY` (+ browserWindowDetected=true) → `CDP_STATUS_STREAMING`
- `VNC_STATUS_UNAVAILABLE` → `CDP_STATUS_UNAVAILABLE`

## P4: buildWsUrl still references `/cdp-stream` endpoint

The current `buildWsUrl` function in `BrowserTab.tsx` correctly builds
`/api/sessions/${sessionId}/cdp-stream`. This endpoint is unchanged — it is the CDP
WebSocket relay, not a VNC proxy. No change needed here.

## P5: SessionDetailView passes `vncState` to BrowserTab in multiple places

Search the file for all occurrences of `vncState` passed as a prop — there may be more
than one mount point if the component has multiple render paths (modal vs. inline panel).
Confirm only one BrowserTab mount exists; if multiple, update all.

## P6: `isWaiting` logic simplification may hide intermediate states

The original `isWaiting` was: `status === VNC_STATUS_STARTING || status === VNC_STATUS_NO_BROWSER || !vncState?.browserWindowDetected`.
The simplified CDP version `isWaiting = !isUnavailable && status !== CDP_STATUS_STREAMING`
means `CDP_STATUS_WAITING` and `CDP_STATUS_NO_BROWSER` both show the waiting placeholder.
This is correct — the CDPViewer should not attempt to connect until `CDP_STATUS_STREAMING`.

The risk is that `CDP_STATUS_WAITING` (actively polling for Chrome) and
`CDP_STATUS_NO_BROWSER` (port allocated, Chrome not yet launched) look identical to the
user. Differentiate with distinct placeholder text:
- `CDP_STATUS_WAITING` → "Connecting to browser..."
- `CDP_STATUS_NO_BROWSER` → "No browser detected — run Chrome or Chromium in this session"

## P7: `cdpState.port` not exposed to frontend but not needed

`CDPState` proto has only `CDPStatus status = 1`; it does not expose the port number.
The CDPViewer connects to `/api/sessions/{id}/cdp-stream` (the Go relay), not directly
to the Chrome CDP port — so no port field is needed on the frontend.

## Live finding (2026-10-25): backend never delivers frames — second root cause

Reproduced on Linux with `/usr/bin/google-chrome-stable` (Chrome 146) against an isolated instance
(`PORT=62871`, `browser-passthrough` flag on, session `cdp-live`):

- `GetSession` → `cdpState: CDP_STATUS_STREAMING` once Chrome ran with `--remote-debugging-port=$CDP_PORT`.
- Frontend (this fix): Browser tab enabled, `CDPViewer` mounted, `cdp-stream` WebSocket opens — but **0 frames**
  arrive in 7 s and the canvas stays 300x150/blank.
- Cause: `session/cdp/manager.go` `runScreencast` dials the **browser-level** endpoint
  (`/devtools/browser/<id>` from `/json/version`) and sends `Page.enable` / `Page.startScreencast` with no
  `sessionId`. Chrome answers `{"error":{"code":-32601,"message":"'Page.enable' wasn't found"}}` for both
  (probe: `bun run cdpprobe.ts` against port 35541). `setState(CDPStatusStreaming)` runs regardless of those errors.
- Consequence: `STREAMING` does not mean frames are flowing, so AC5/AC6 ("CDPViewer rendering frames") cannot be
  met by a frontend-only change, and AC9 ("no Go backend code modified") conflicts with them.
- Test-setup note: the in-session shell rc reordered `PATH`, hiding the `cdp-bins` wrapper dir, so Chrome had to be
  launched with `--remote-debugging-port=$CDP_PORT` explicitly.
