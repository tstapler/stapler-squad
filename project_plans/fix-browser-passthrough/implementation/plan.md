# Fix Browser Passthrough — Implementation Plan

## Summary

Two frontend files need targeted changes to fix the broken browser passthrough feature on
both macOS and Linux. The backend already correctly populates `session.cdpState` over the
WatchSessions stream; the front-end was never updated to read it. This plan consists of
three small, sequential tasks — each independently verifiable.

No backend code, proto schema, or generated Go/TS bindings need to change.
`make build` must be run first to ensure the TS bindings include `CDPState`/`CDPStatus`.

---

## Pre-flight

Run `make build` in the worktree to ensure `web-app/src/gen/session/v1/types_pb.ts` is
up to date and contains `CDPState` and `CDPStatus`. Do not proceed without this check.

---

## Task 1: Update `SessionDetailView.tsx`

**File:** `web-app/src/components/sessions/SessionDetailView.tsx`

### 1a. Update import

Add `CDPStatus` to the existing import line (or add a new import):
```tsx
import { CDPStatus } from "@/gen/session/v1/types_pb";
```

The existing `import { VNCStatus } from "@/gen/session/v1/types_pb"` can be removed if
no other part of SessionDetailView uses `VNCStatus`. Grep first to confirm.

### 1b. Replace vncState-based availability signal (lines ~603–605)

**Remove:**
```tsx
const vncState = session.vncState;
const vncStatus = vncState?.status ?? VNCStatus.VNC_STATUS_UNSPECIFIED;
const isBrowserAvailable = vncStatus !== VNCStatus.VNC_STATUS_UNAVAILABLE && vncStatus !== VNCStatus.VNC_STATUS_UNSPECIFIED;
```

**Add:**
```tsx
const cdpState = session.cdpState;
const cdpStatus = cdpState?.status ?? CDPStatus.CDP_STATUS_UNSPECIFIED;
const isBrowserAvailable = cdpStatus !== CDPStatus.CDP_STATUS_UNAVAILABLE && cdpStatus !== CDPStatus.CDP_STATUS_UNSPECIFIED;
```

### 1c. Update tooltip text (line ~922)

**Remove:**
```tsx
? "Browser passthrough requires Linux with Xvfb, x11vnc, and xdotool"
```

**Add:**
```tsx
? "Browser passthrough requires Chrome or Chromium installed on the host"
```

### 1d. Pass cdpState to BrowserTab (line ~1138–1142)

**Remove:**
```tsx
vncState={vncState}
```

**Add:**
```tsx
cdpState={cdpState}
```

---

## Task 2: Update `BrowserTab.tsx`

**File:** `web-app/src/components/sessions/BrowserTab.tsx`

### 2a. Update imports

Remove the `VNCState` and `VNCStatus` imports. Add `CDPState` and `CDPStatus`:
```tsx
import { CDPState, CDPStatus } from '@/gen/session/v1/types_pb';
```

Remove the existing re-exports if unused by callers (verify first):
```tsx
// Remove these if no other file imports from BrowserTab:
export type { VNCState };
export { VNCStatus };
```

### 2b. Rename prop type

**Remove:**
```tsx
interface BrowserTabProps {
  ...
  vncState: VNCState | undefined;
}
```

**Add:**
```tsx
interface BrowserTabProps {
  ...
  cdpState: CDPState | undefined;
}
```

### 2c. Update status derivation logic

**Remove:**
```tsx
export function BrowserTab({ sessionId, baseUrl, isVisible, vncState }: BrowserTabProps) {
  const status = vncState?.status ?? VNCStatus.VNC_STATUS_UNSPECIFIED;
  const isUnavailable = status === VNCStatus.VNC_STATUS_UNAVAILABLE || status === VNCStatus.VNC_STATUS_UNSPECIFIED;
  const isWaiting = !isUnavailable && (
    status === VNCStatus.VNC_STATUS_STARTING ||
    status === VNCStatus.VNC_STATUS_NO_BROWSER ||
    !vncState?.browserWindowDetected
  );
  const isReady = status === VNCStatus.VNC_STATUS_READY && vncState?.browserWindowDetected === true;
```

**Add:**
```tsx
export function BrowserTab({ sessionId, baseUrl, isVisible, cdpState }: BrowserTabProps) {
  const status = cdpState?.status ?? CDPStatus.CDP_STATUS_UNSPECIFIED;
  const isUnavailable = status === CDPStatus.CDP_STATUS_UNAVAILABLE || status === CDPStatus.CDP_STATUS_UNSPECIFIED;
  const isWaiting = !isUnavailable && status !== CDPStatus.CDP_STATUS_STREAMING;
  const isReady = status === CDPStatus.CDP_STATUS_STREAMING;
```

### 2d. Update placeholder strings in JSX

Find the waiting state placeholder branch (currently using `VNCStatus.VNC_STATUS_STARTING`):

**Remove:**
```tsx
{status === VNCStatus.VNC_STATUS_STARTING ? (
  <span>Starting virtual display...</span>
) : (
  <>
    <span>No browser open yet</span>
    <span style={{ fontSize: '0.85em', opacity: 0.7 }}>
      Run a browser in this session to see it here
    </span>
  </>
)}
```

**Add:**
```tsx
{status === CDPStatus.CDP_STATUS_WAITING ? (
  <span>Connecting to browser...</span>
) : (
  <>
    <span>No browser detected</span>
    <span style={{ fontSize: '0.85em', opacity: 0.7 }}>
      Run Chrome or Chromium in this session to see it here
    </span>
  </>
)}
```

---

## Task 3: Update tests

**File:** `web-app/src/components/sessions/BrowserTab.test.tsx`

For every test that passes a `vncState` prop to `<BrowserTab>`, replace it with `cdpState`.
Map the old VNCStatus values to CDPStatus using the table in `research/stack.md`.

Key test-fixture replacements:
- `vncState={{ status: VNCStatus.VNC_STATUS_UNAVAILABLE }}` → `cdpState={{ status: CDPStatus.CDP_STATUS_UNAVAILABLE }}`
- `vncState={{ status: VNCStatus.VNC_STATUS_NO_BROWSER, browserWindowDetected: false }}` → `cdpState={{ status: CDPStatus.CDP_STATUS_NO_BROWSER }}`
- `vncState={{ status: VNCStatus.VNC_STATUS_READY, browserWindowDetected: true }}` → `cdpState={{ status: CDPStatus.CDP_STATUS_STREAMING }}`

**File:** `web-app/src/components/sessions/SessionDetailView.test.tsx` (if it tests
browser-tab visibility using `vncState`): update to use `session.cdpState`.

### New tests to add (in BrowserTab.test.tsx)

1. `BrowserTab_renders_placeholder_when_cdpStatus_NO_BROWSER` — assert "No browser detected" visible, CDPViewer not mounted
2. `BrowserTab_renders_placeholder_when_cdpStatus_WAITING` — assert "Connecting to browser..." visible
3. `BrowserTab_renders_cdp_viewer_when_cdpStatus_STREAMING` — assert CDPViewer mounted, `isReady=true`
4. `BrowserTab_disabled_when_cdpStatus_UNAVAILABLE` — assert unavailable message shown

---

## Acceptance Checklist

- [ ] `make build` completes without error (generates TS bindings)
- [ ] `cd web-app && bunx jest --testPathPatterns="BrowserTab|SessionDetailView" --no-coverage` — all pass
- [ ] `make lint` — zero TypeScript/ESLint warnings or errors in changed files
- [ ] Manual: macOS with Chrome — Browser tab enabled, CDPViewer renders after starting Chrome in a session
- [ ] Manual: Linux with Chrome — Browser tab enables when cdpState goes STREAMING (not waiting for xdotool)
- [ ] Manual: host without Chrome — Browser tab shows correct tooltip text "Chrome or Chromium"
- [ ] Manual: mobile (touch) — tapping disabled Browser tab shows correct toast copy

---

## Revision 2 (2026-10-25): live finding — scope conflict AC5/AC6 vs AC9

Tasks 1–3 above are implemented (commit `034e28ca3`). Live verification (see `research/pitfalls.md`, "Live finding")
showed the frontend fix makes the tab enable and `CDPViewer` mount, but **no frames reach the canvas** because
`session/cdp/manager.go` `runScreencast` sends `Page.enable`/`Page.startScreencast` on Chrome's browser-level
WebSocket, which rejects them (`'Page.enable' wasn't found`), while still publishing `CDP_STATUS_STREAMING`.

### Decision required (owner): AC9 vs AC5/AC6 cannot both hold

| Option | Scope | AC5/AC6 | AC9 |
|---|---|---|---|
| A. Ship frontend-only now; file backend follow-up | 4 FE files (done) | stay FAIL (unverified/unmet) | holds |
| B. Also fix backend in this PR | + `session/cdp/manager.go` (+ test) | can pass after live proof | **violated; AC9 must be waived** |

Recommendation: A for this PR (AC9 is explicit), and a follow-up backlog item for B.

### Task 4 (follow-up, NOT in this PR unless owner waives AC9): attach to a page target
1. After `Chrome detected`, send `Target.getTargets` (or `GET /json/list`), pick the first `type=="page"` target.
2. Either dial that target's `webSocketDebuggerUrl` (Page.* works there) or `Target.attachToTarget{flatten:true}` and
   include `sessionId` on every command and filter events by it.
3. Set `CDPStatusStreaming` only after `Page.startScreencast` returns a non-error result (today it is set unconditionally).
4. Re-attach on target close / navigation to a new page target.
5. Test: fake CDP server asserting Page.* arrives on the page endpoint and state stays `Waiting` on an error reply;
   live proof: `bun run cdpprobe.ts`-style probe plus a non-blank canvas in the Browser tab.

### Added risks
- R-A: `STREAMING` is a false positive today; the frontend now trusts it (isReady) — spinner-less blank viewer until Task 4.
- R-B: `PATH` wrapper (`cdp-bins`) can be defeated by shell rc files reordering `PATH`; Chrome then runs without the debug port and state sits at `NO_BROWSER`.

### Revision 2 disposition (2026-10-25)
- Owner decision (A vs B) is not available in this unattended run; Option A is taken because AC9 is explicit. B is filed as backlog item `36a72546-6b68-46dc-bf4c-9612a94978a4`.
- Pre-mortem P1 mitigated within AC9: `CDPViewer` gained `onFrame`; `BrowserTab` shows "Waiting for first frame…" and, after 10 s, "No frames received from the browser" (3 new Jest tests; confirmed live on a fresh session).
- Adversarial correction accepted: only AC5 needs the backend; AC6 (tab activates on STREAMING) is met by the frontend and was confirmed live. AC7/AC8 were not in the table.

## Readiness gate (sdd:4-validate step 5), 2026-10-25
| # | Criterion | Result |
|---|---|---|
| 1 | Every requirement has a test case in validation.md | PASS (validation.md predates Rev 2; first-frame overlay tests added in `BrowserTab.test.tsx` but not yet listed there) |
| 2 | No TODO/TBD in plan | PASS |
| 3 | ADRs on disk | N/A (none referenced) |
| 4 | No BLOCKER in adversarial-review.md | PASS (CONCERNS, 0 blockers) |
| 5 | No BLOCKER in architecture-review.md | N/A (not dispatched; plan touches frontend files only) |
| 6 | Migration plan for schema changes | N/A |
| 7 | No open P1 in pre-mortem.md | PASS after mitigation above |
| 8 | No cycle/layering problems | PASS (no new packages) |
Verdict: CONCERNS (criterion 1 note; Option B owner decision pending). Triad review not run (complexity field absent in requirements.md; treated as 2).
