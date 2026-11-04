# Fix Browser Passthrough — Architecture Research

## 1. Current Signal Flow (broken)

```
Go backend                    Proto / WatchSessions            Frontend
──────────────────────────────────────────────────────────────────────
session/cdp/manager.go        Session.cdp_state (field 52)     SessionDetailView.tsx
  CDPStatusStreaming  ──────► CDPState{ status: STREAMING }     reads session.cdpState? ❌ NO
                                                                reads session.vncState  ✓ (wrong)
                                                                  → isBrowserAvailable

session/vnc/manager.go        Session.vnc_state (field 51)
  VNC_STATUS_UNAVAILABLE ───► VNCState{ status: UNAVAILABLE }  → isBrowserAvailable = false ← BUG
  (always on macOS)                                             → Browser tab disabled
```

**Both `vnc_state` AND `cdp_state` are populated in `instance_adapter.go`
(`server/adapters/instance_adapter.go:229-242`). But the frontend only reads
`session.vncState` for the tab availability decision.**

## 2. What Needs to Change in the Frontend Only

### 2a. `SessionDetailView.tsx` — two changes

**Line 603–605 (current)**:
```tsx
const vncState = session.vncState;
const vncStatus = vncState?.status ?? VNCStatus.VNC_STATUS_UNSPECIFIED;
const isBrowserAvailable = vncStatus !== VNCStatus.VNC_STATUS_UNAVAILABLE && vncStatus !== VNCStatus.VNC_STATUS_UNSPECIFIED;
```

**Fixed**:
```tsx
const cdpState = session.cdpState;
const cdpStatus = cdpState?.status ?? CDPStatus.CDP_STATUS_UNSPECIFIED;
const isBrowserAvailable = cdpStatus !== CDPStatus.CDP_STATUS_UNAVAILABLE && cdpStatus !== CDPStatus.CDP_STATUS_UNSPECIFIED;
```

Also add the import: `import { CDPStatus } from "@/gen/session/v1/types_pb";`

**Line 614 (tabs array)**:
```tsx
{ id: "browser", label: "Browser", icon: Globe, disabled: !isBrowserAvailable },
```
No change needed here — `isBrowserAvailable` is now derived from CDP state.

**Line 922 (tooltip)**:
```tsx
? "Browser passthrough requires Linux with Xvfb, x11vnc, and xdotool"
```
→
```tsx
? "Browser passthrough requires Chrome or Chromium installed on the host"
```

**Line 1138–1142 (BrowserTab mount)**:
```tsx
<BrowserTab
  sessionId={session.id}
  baseUrl={baseUrl}
  isVisible={activeTab === "browser"}
  vncState={vncState}  // ← change to cdpState
/>
```
→
```tsx
<BrowserTab
  sessionId={session.id}
  baseUrl={baseUrl}
  isVisible={activeTab === "browser"}
  cdpState={cdpState}
/>
```

### 2b. `BrowserTab.tsx` — props + state logic

**Current prop type**:
```tsx
interface BrowserTabProps {
  sessionId: string;
  baseUrl: string;
  isVisible: boolean;
  vncState: VNCState | undefined;
}
```

**Fixed**:
```tsx
import { CDPState, CDPStatus } from '@/gen/session/v1/types_pb';

interface BrowserTabProps {
  sessionId: string;
  baseUrl: string;
  isVisible: boolean;
  cdpState: CDPState | undefined;
}
```

**Current status derivation**:
```tsx
const status = vncState?.status ?? VNCStatus.VNC_STATUS_UNSPECIFIED;
const isUnavailable = status === VNCStatus.VNC_STATUS_UNAVAILABLE || status === VNCStatus.VNC_STATUS_UNSPECIFIED;
const isWaiting = !isUnavailable && (
  status === VNCStatus.VNC_STATUS_STARTING || status === VNCStatus.VNC_STATUS_NO_BROWSER || !vncState?.browserWindowDetected
);
const isReady = status === VNCStatus.VNC_STATUS_READY && vncState?.browserWindowDetected === true;
```

**Fixed**:
```tsx
const status = cdpState?.status ?? CDPStatus.CDP_STATUS_UNSPECIFIED;
const isUnavailable = status === CDPStatus.CDP_STATUS_UNAVAILABLE || status === CDPStatus.CDP_STATUS_UNSPECIFIED;
const isWaiting = !isUnavailable && status !== CDPStatus.CDP_STATUS_STREAMING;
const isReady = status === CDPStatus.CDP_STATUS_STREAMING;
```

**Placeholder text update (in JSX)**:
- `VNCStatus.VNC_STATUS_STARTING` → `CDPStatus.CDP_STATUS_WAITING`, text: "Connecting to browser..."
- No-browser placeholder text: "No browser detected — run Chrome or Chromium in this session"

## 3. Backend: No Changes Required

`server/adapters/instance_adapter.go` already populates both states:
- Lines 229–237: maps VNC manager state → `protoSession.VncState`
- Lines 240–244: maps CDP manager state → `protoSession.CdpState`

Both are already sent over WatchSessions events. The fix is purely in how the
frontend *reads* these fields.

## 4. Proto: No Changes Required

`proto/session/v1/types.proto` already has:
- `CDPStatus` enum: `CDP_STATUS_UNSPECIFIED=0`, `CDP_STATUS_WAITING=1`, `CDP_STATUS_STREAMING=2`, `CDP_STATUS_NO_BROWSER=3`, `CDP_STATUS_UNAVAILABLE=4`
- `CDPState { CDPStatus status = 1; }` on `Session` as field 52 `cdp_state`

After `make build`, the generated TS bindings at `web-app/src/gen/session/v1/types_pb.ts`
will include `CDPState` and `CDPStatus` — import them the same way `VNCStatus` is imported.

## 5. Affected Files (frontend only)

| File | Change |
|---|---|
| `web-app/src/components/sessions/SessionDetailView.tsx` | Read `cdpState` from session; update `isBrowserAvailable` logic; update tooltip text; pass `cdpState` to BrowserTab |
| `web-app/src/components/sessions/BrowserTab.tsx` | Rename prop `vncState → cdpState`; import CDPStatus; update status derivation logic; update placeholder strings |
| `web-app/src/components/sessions/BrowserTab.test.tsx` | Update fixtures/mocks from VNCState to CDPState |
| `web-app/src/components/sessions/SessionDetailView.test.tsx` | Update browser-tab-availability tests to use cdpState |

## 6. VNCState Export Cleanup (Optional)

`BrowserTab.tsx` currently exports `VNCState` and `VNCStatus`:
```tsx
export type { VNCState };
export { VNCStatus };
```

These exports are not referenced by any other component (confirmed by grepping
`from.*BrowserTab`). They can be removed as part of the cleanup, or left in place if
they're used by tests.
