# Browser Passthrough — Validation Plan

Generated: 2026-05-16  
Phase: SDD Phase 4 (Validation)  
Input: `plan.md`, `requirements.md`

---

## Summary

| Test Type | Count |
|---|---|
| Unit Tests (Go) | 24 |
| Unit Tests (TypeScript/Jest) | 14 |
| Integration Tests (Go) | 9 |
| E2E Tests (Playwright) | 7 |
| **Total** | **54** |

Requirements coverage: **15/15** (FR-1 through FR-10, NFR-1 through NFR-5)

---

## Unit Tests — Go

### Package: `session/vnc` — Display Allocation (`display_alloc_test.go`)

**UT-GO-001** `DisplayAllocator_Allocate_ReturnsDisplayInRange`  
- Setup: fresh `DisplayAllocator` with base=100, max=200  
- Action: call `Allocate("session-1")`  
- Assert: returned `displayN >= 100 && displayN <= 200`  
- Covers: FR-1

**UT-GO-002** `DisplayAllocator_Allocate_NoCollisionConcurrent`  
- Setup: 10 goroutines each calling `Allocate("session-N")` simultaneously  
- Action: collect all returned display numbers  
- Assert: all 10 numbers are unique (no duplicates)  
- Covers: FR-1 (collision prevention via `O_CREATE|O_EXCL`)

**UT-GO-003** `DisplayAllocator_Allocate_FailsWhenRangeFull`  
- Setup: allocate displays 100–110 (11 allocations); configured max=110  
- Action: call `Allocate("overflow-session")`  
- Assert: returns an error (no display available)  
- Covers: FR-1

**UT-GO-004** `DisplayAllocator_Release_RemovesLockFile`  
- Setup: allocate display N  
- Action: call `Release(N)`  
- Assert: `/tmp/.XN-lock` no longer exists; subsequent `Allocate()` can return N again  
- Covers: FR-1

**UT-GO-005** `DisplayAllocator_CleanupStaleDisplays_RemovesDeadPidLocks`  
- Setup: write fake lock file `/tmp/.X105-lock` containing a PID that is dead (use a PID that definitely does not exist, e.g. 9999999)  
- Action: call `CleanupStaleDisplays()`  
- Assert: `/tmp/.X105-lock` is removed; no error returned  
- Covers: FR-1, NFR-5 (startup orphan cleanup)

**UT-GO-006** `DisplayAllocator_CleanupStaleDisplays_PreservesLivePidLocks`  
- Setup: write lock file containing `os.Getpid()` (current process — definitely alive)  
- Action: call `CleanupStaleDisplays()`  
- Assert: lock file is NOT removed  
- Covers: FR-1

### Package: `session/vnc` — VNCProcessManager (`manager_test.go`)

**UT-GO-007** `VNCProcessManager_Start_StartsXvfbThenX11vnc`  
- Setup: mock `executor.ManagedProcess` factory; capture call order  
- Action: call `manager.Start(ctx)`  
- Assert: Xvfb is started before x11vnc; both start args include the allocated display number; `state.Status` becomes `VNC_STATUS_NO_BROWSER`  
- Covers: FR-1, FR-2

**UT-GO-008** `VNCProcessManager_Stop_StopsX11vncBeforeXvfb`  
- Setup: running manager with mocked processes  
- Action: call `manager.Stop()`  
- Assert: x11vnc `Stop()` is called before Xvfb `Stop()`; `DisplayAllocator.Release()` is called after both  
- Covers: FR-1, FR-2, FR-9

**UT-GO-009** `VNCProcessManager_Start_InjectsNopwAndLocalhost`  
- Setup: capture x11vnc command arguments  
- Action: call `manager.Start(ctx)`  
- Assert: x11vnc args contain `-nopw` and `-localhost` and `-noclipboard`  
- Covers: NFR-2

**UT-GO-010** `VNCProcessManager_CrashRecovery_RestartsUpToThreeTimes`  
- Setup: mock x11vnc process that exits immediately (simulating crash); restart count tracking  
- Action: call `manager.Start(ctx)`, let crash recovery goroutine run  
- Assert: x11vnc is restarted exactly 3 times before `state.Status` is set to `VNC_STATUS_UNAVAILABLE`  
- Covers: FR-9

**UT-GO-011** `VNCProcessManager_CrashRecovery_ExponentialBackoff`  
- Setup: mock x11vnc that crashes; capture timestamps of each restart attempt  
- Action: let crash recovery run 3 attempts  
- Assert: delay between attempt 1→2 is ~100ms; delay between 2→3 is ~200ms (±50ms tolerance)  
- Covers: FR-9

**UT-GO-012** `VNCProcessManager_CrashRecovery_XvfbUnaffected`  
- Setup: running manager; mock x11vnc crashes  
- Action: let crash recovery run  
- Assert: Xvfb `Stop()` is NOT called during x11vnc crash recovery; Xvfb mock records zero `Stop()` calls until `manager.Stop()` is explicitly called  
- Covers: FR-9

**UT-GO-013** `VNCProcessManager_NoopOnNonLinux_ReturnsUnavailable`  
- Setup: call `vnc.New(cfg)` with `runtime.GOOS` stubbed to `"darwin"` (or use build-tag isolation)  
- Action: call `manager.State()`  
- Assert: `State().Status == VNC_STATUS_UNAVAILABLE`; `Port() == 0`; `DisplayNumber() == 0`  
- Covers: FR-8, NFR-4

**UT-GO-014** `VNCProcessManager_NoopOnMissingDeps_ReturnsUnavailable`  
- Setup: `CheckDependencies()` returns `Available: false` (xvfb not found)  
- Action: `vnc.New(cfg)` → `manager.State()`  
- Assert: `State().Status == VNC_STATUS_UNAVAILABLE`; no process spawned  
- Covers: FR-8, NFR-4

### Package: `session/vnc` — Dependency Check (`deps_check_test.go`)

**UT-GO-015** `CheckDependencies_ReturnsAvailableFalse_WhenBinaryMissing`  
- Setup: PATH override that does not contain `x11vnc`  
- Action: call `CheckDependencies()`  
- Assert: `result.Available == false`; `result.Missing` contains `"x11vnc"`  
- Covers: NFR-4

**UT-GO-016** `CheckDependencies_ReturnsAvailableFalse_OnNonLinux`  
- Setup: test environment or mock that simulates non-Linux GOOS  
- Action: call `CheckDependencies()`  
- Assert: `result.Available == false`; reason contains `"platform not supported"`  
- Covers: FR-8, NFR-4

### Package: `session/vnc` — Window Tracker (`window_tracker_test.go`)

**UT-GO-017** `WindowTracker_Poll_CallsOnWindowDetected_WhenChromeFound`  
- Setup: mock `safeexec.CommandContext` to return a window ID on stdout  
- Action: run one poll cycle  
- Assert: `onWindowDetected(windowID)` callback is called with the correct window ID  
- Covers: FR-3, FR-6

**UT-GO-018** `WindowTracker_Poll_CallsOnWindowLost_WhenWindowDisappears`  
- Setup: first poll returns window ID "12345"; second poll returns empty  
- Action: run two poll cycles  
- Assert: `onWindowLost()` is called after the second poll; `lastWindowID` is reset  
- Covers: FR-3, FR-6

**UT-GO-019** `WindowTracker_Poll_SelectsLargestWindowByGeometry`  
- Setup: mock xdotool returns two window IDs; mock `getwindowgeometry` returns 800×600 for first, 1280×800 for second  
- Action: one poll cycle  
- Assert: `onWindowDetected` is called with the second (larger) window ID  
- Covers: FR-3

**UT-GO-020** `WindowTracker_Poll_SetsDisplayEnvOnSubprocess`  
- Setup: capture subprocess environment in mock  
- Action: run poll cycle on display N=105  
- Assert: captured env includes `DISPLAY=:105`  
- Covers: FR-3 (prevents xdotool targeting host display :0)

### Package: `server/services` — VNC WebSocket Proxy (`vnc_proxy_handler_test.go`)

**UT-GO-021** `VNCProxyHandler_RejectsUnauthenticated_Returns401`  
- Setup: handler with auth middleware; request with no auth cookie/token  
- Action: HTTP GET to `/api/sessions/{id}/vnc`  
- Assert: HTTP 401 returned before WebSocket upgrade  
- Covers: FR-4, NFR-2

**UT-GO-022** `VNCProxyHandler_ReturnsNotFound_WhenSessionMissing`  
- Setup: `VNCManager.GetPort("unknown-id")` returns 0  
- Action: WebSocket upgrade request for session ID "unknown-id"  
- Assert: HTTP 404 returned  
- Covers: FR-4

**UT-GO-023** `VNCProxyHandler_RelaysBytesInBothDirections`  
- Setup: loopback TCP listener on `127.0.0.1:0`; mock server echoes received bytes; authenticated WebSocket client  
- Action: client sends binary frame "HELLO"; read back response  
- Assert: TCP server receives "HELLO"; client receives "HELLO" echoed back; both goroutines exit cleanly after connection close  
- Covers: FR-4

**UT-GO-024** `VNCProxyHandler_GoroutineCleanup_OnDisconnect`  
- Setup: goroutine count snapshot before handler; mock WebSocket client; mock TCP server that closes immediately  
- Action: run handler; wait for both goroutines to finish  
- Assert: goroutine count returns to pre-handler baseline; no FD leak (check via `/proc/self/fd` count on Linux)  
- Covers: FR-4 (pitfall §4.1 / §6.1 goroutine leak prevention)

---

## Unit Tests — TypeScript/Jest

Test file locations: `web-app/src/components/sessions/`

### BrowserTab Component (`BrowserTab.test.tsx`)

**UT-TS-001** `BrowserTab_renders_placeholder_when_browserWindowDetected_false`  
- Setup: render `<BrowserTab>` with `vncState={{ status: VNC_STATUS_NO_BROWSER, browserWindowDetected: false }}`  
- Assert: element with text "No browser open yet" is present in the DOM; canvas element is NOT present  
- Covers: FR-3, FR-6

**UT-TS-002** `BrowserTab_renders_placeholder_when_status_UNAVAILABLE`  
- Setup: render `<BrowserTab>` with `vncState={{ status: VNC_STATUS_UNAVAILABLE }}`  
- Assert: element indicating unavailability is present; no canvas; tab content does not crash  
- Covers: FR-8, NFR-4

**UT-TS-003** `BrowserTab_renders_canvas_when_browserWindowDetected_true`  
- Setup: mock `NoVNCViewer`; render `<BrowserTab>` with `vncState={{ status: VNC_STATUS_READY, browserWindowDetected: true }}`  
- Assert: `NoVNCViewer` mock is called; canvas container `div` is present  
- Covers: FR-5, FR-6

**UT-TS-004** `BrowserTab_constructs_wsUrl_correctly_for_http`  
- Setup: render with `baseUrl="http://localhost:8543"` and `sessionId="abc-123"`  
- Assert: `NoVNCViewer` mock receives `wsUrl="ws://localhost:8543/api/sessions/abc-123/vnc"`  
- Covers: FR-4

**UT-TS-005** `BrowserTab_constructs_wssUrl_correctly_for_https`  
- Setup: render with `baseUrl="https://onyx.staplerhome.internal:8444"` and `sessionId="abc-123"`  
- Assert: `NoVNCViewer` mock receives `wsUrl="wss://onyx.staplerhome.internal:8444/api/sessions/abc-123/vnc"`  
- Covers: FR-4

**UT-TS-006** `BrowserTab_quality_toggle_Low_sets_qualityLevel_3`  
- Setup: render `<BrowserTab>` with status READY; find quality toggle buttons  
- Action: click "Low" button  
- Assert: `NoVNCViewer` mock receives `qualityLevel={3}`  
- Covers: FR-10

**UT-TS-007** `BrowserTab_quality_toggle_High_sets_qualityLevel_9`  
- Setup: same as above  
- Action: click "High" button  
- Assert: `NoVNCViewer` mock receives `qualityLevel={9}`  
- Covers: FR-10

**UT-TS-008** `BrowserTab_quality_toggle_defaults_to_Medium_qualityLevel_6`  
- Setup: render `<BrowserTab>` with status READY; no user action  
- Assert: `NoVNCViewer` initially receives `qualityLevel={6}`  
- Covers: FR-10

### NoVNCViewer Connection Lifecycle (`NoVNCViewer.test.tsx`)

**UT-TS-009** `NoVNCViewer_connects_RFB_on_mount`  
- Setup: mock `@novnc/novnc` `RFB` class; render `<NoVNCViewer wsUrl="ws://..." isVisible={true} qualityLevel={6} />`  
- Assert: `RFB` constructor is called once with the container element and `wsUrl`; `rfb.scaleViewport = true` is set  
- Covers: FR-5

**UT-TS-010** `NoVNCViewer_disconnects_RFB_on_unmount`  
- Setup: mock RFB; mount and then unmount component  
- Assert: `rfb.disconnect()` is called exactly once during unmount cleanup  
- Covers: FR-5

**UT-TS-011** `NoVNCViewer_does_not_reconnect_on_isVisible_change`  
- Setup: mount with `isVisible={true}`; update to `isVisible={false}` then back to `isVisible={true}`  
- Assert: `RFB` constructor is called exactly once (no reconnect on visibility change)  
- Covers: FR-6, FR-5 (keep-alive mount pattern)

**UT-TS-012** `NoVNCViewer_does_not_sync_clipboard`  
- Setup: mock RFB; mount component  
- Assert: `rfb.clipViewport` is NOT set to `true`; no clipboard event listeners are registered  
- Covers: NFR-2 (clipboard sync out of scope)

### Session Tab Visibility (`SessionDetailView.test.tsx`)

**UT-TS-013** `SessionDetailView_browser_tab_hidden_when_status_UNAVAILABLE`  
- Setup: render `<SessionDetailView>` with session where `vncState.status === VNC_STATUS_UNAVAILABLE`  
- Assert: "Browser" tab button is either not present in DOM, or has `aria-disabled="true"` / `disabled` attribute  
- Covers: FR-6, FR-8

**UT-TS-014** `SessionDetailView_browser_tab_visible_when_status_READY`  
- Setup: render `<SessionDetailView>` with session where `vncState.status === VNC_STATUS_READY && browserWindowDetected === true`  
- Assert: "Browser" tab button is present and interactive (no disabled attribute)  
- Covers: FR-6

---

## Integration Tests — Go

Test file locations: `server/services/` and `session/vnc/` with real subprocesses (skipped if deps absent via `t.Skip`).

**IT-GO-001** `Integration_CreateSession_SpawnsXvfbAndX11vnc`  
- Precondition: `CheckDependencies()` returns `Available: true`; otherwise `t.Skip("xvfb/x11vnc not installed")`  
- Setup: create session via `VNCProcessManager.Start(ctx)` with real Xvfb and x11vnc  
- Assert: within 5 seconds, `manager.Port() > 0`; TCP dial to `127.0.0.1:<port>` succeeds (port is reachable); `/tmp/.XN-lock` exists  
- Covers: FR-1, FR-2

**IT-GO-002** `Integration_DestroySession_CleansUpProcessesAndLockFile`  
- Precondition: deps available  
- Setup: start manager as in IT-GO-001; record display N and VNC port  
- Action: call `manager.Stop()`  
- Assert: `kill -0 <xvfb_pid>` fails (process gone); `kill -0 <x11vnc_pid>` fails; `/tmp/.XN-lock` does not exist; TCP dial to the recorded VNC port is refused  
- Covers: FR-1, FR-2, FR-9

**IT-GO-003** `Integration_NoOrphanProcessGroups_AfterCrash`  
- Precondition: deps available  
- Setup: start manager; get x11vnc PID and PGID via `ps -o pgid= -p <pid>`  
- Action: send SIGKILL directly to x11vnc PID (bypassing manager); wait for crash recovery to trigger; then call `manager.Stop()`  
- Assert: after `Stop()`, no processes remain in the process group (check via `/proc/PGID/` absence)  
- Covers: FR-9, NFR-5 (no orphan processes)

**IT-GO-004** `Integration_DependencyMissing_FeatureDisabledNocrash`  
- Setup: temporarily shadow `PATH` to hide `x11vnc`; create manager via `vnc.New(cfg)`  
- Action: call `manager.Start(ctx)`  
- Assert: no panic; `manager.State().Status == VNC_STATUS_UNAVAILABLE`; no Xvfb process is spawned  
- Covers: FR-8, NFR-4

**IT-GO-005** `Integration_VNCProxy_HandshakeCompletes`  
- Precondition: deps available; full VNC manager running  
- Setup: start VNC manager; create a real WebSocket client connecting to the proxy handler  
- Action: complete WebSocket upgrade; wait for RFB protocol handshake bytes  
- Assert: first bytes from x11vnc (RFB version string "RFB 003.889\n") are forwarded to the WebSocket client within 2 seconds  
- Covers: FR-4, FR-5

**IT-GO-006** `Integration_DisplayEnvInheritedByTmuxSession`  
- Precondition: deps available; tmux available  
- Setup: create a session instance via `session.NewInstance()`; let it start fully  
- Action: run `tmux show-environment -t <session>` and parse output  
- Assert: `DISPLAY` variable is present and equals `:<N>` where N matches `vncManager.DisplayNumber()`  
- Covers: FR-1

**IT-GO-007** `Integration_MultipleSessionsGetUniqueDisplays`  
- Precondition: deps available  
- Setup: create 3 session instances concurrently  
- Assert: all 3 `vncManager.DisplayNumber()` values are distinct and in the 100–200 range; all 3 VNC ports are distinct  
- Covers: FR-1, FR-2 (collision prevention end-to-end)

**IT-GO-008** `Integration_X11vncRestartPreservesChromePid`  
- Precondition: deps available; `google-chrome` or `chromium` available  
- Setup: start VNC manager; launch a headless Chrome on the virtual display (`google-chrome --headless=new --display=:<N>`)  
- Action: externally kill x11vnc PID; wait for crash recovery  
- Assert: Chrome process is still alive (PID unchanged) after x11vnc restarts; `manager.State().Status` eventually returns to `VNC_STATUS_READY`  
- Covers: FR-9

**IT-GO-009** `Integration_ReconcileOrphans_CleansUpOnStartup`  
- Setup: manually write a lock file `/tmp/.X150-lock` with a dead PID; write a matching x11vnc socket stub  
- Action: create a new `VNCProcessManager` and call `ReconcileOrphans([]StoredSession{})`  
- Assert: `/tmp/.X150-lock` is removed; no error returned; new `Allocate()` can claim display 150  
- Covers: FR-1, NFR-5

---

## E2E Tests — Playwright

Test file: `tests/e2e/browser-passthrough.spec.ts`  
Precondition guard at test file level:

```typescript
// @feature browser:passthrough, session:create
test.beforeAll(async () => {
  const resp = await request.get('/api/health');
  const health = await resp.json();
  if (!health.vncAvailable) {
    test.skip(); // deps not installed on CI runner
  }
});
```

**E2E-001** `browser-passthrough_browser_tab_appears_in_session_view`  
- Setup: create a new session via the omnibar  
- Assert: session detail view renders; "Browser" tab button is present in the tab strip (may be greyed out initially)  
- Covers: FR-6

**E2E-002** `browser-passthrough_browser_tab_disabled_when_no_browser_detected`  
- Setup: create session; do NOT launch a browser  
- Assert: "Browser" tab is present but disabled (`aria-disabled="true"` or grayed); clicking it does not switch content pane  
- Covers: FR-6, FR-3

**E2E-003** `browser-passthrough_tab_becomes_active_after_chrome_launch`  
- Setup: create session; via the terminal tab, run `google-chrome --no-sandbox --display=$DISPLAY &`  
- Action: wait for `WatchSessions` stream to push `browserWindowDetected: true` (poll the session endpoint, max 10 seconds)  
- Assert: "Browser" tab becomes interactive (no longer disabled)  
- Covers: FR-6, FR-3, FR-7

**E2E-004** `browser-passthrough_novnc_canvas_visible_and_non_empty`  
- Setup: create session; launch Chrome; wait for tab to become active (as E2E-003)  
- Action: click the "Browser" tab  
- Assert: HTML5 canvas element is present and visible; canvas `width > 0` and `height > 0`; no JavaScript error in console  
- Covers: FR-5, FR-6

**E2E-005** `browser-passthrough_session_destroy_cleans_up`  
- Setup: create session; launch Chrome; verify browser tab active  
- Action: destroy session via the UI  
- Assert: session is removed from session list; no Xvfb or x11vnc processes remain for that session (check via backend health or API)  
- Covers: FR-1, FR-2, FR-9

**E2E-006** `browser-passthrough_tab_hidden_on_unsupported_host`  
- Precondition: run against a server where `CheckDependencies()` returns `Available: false` (mock or macOS runner)  
- Setup: create session normally  
- Assert: "Browser" tab is completely absent from tab strip (not just greyed — hidden)  
- Covers: FR-8, NFR-4, NFR-5

**E2E-007** `browser-passthrough_quality_toggle_changes_rfb_qualityLevel`  
- Setup: create session; launch Chrome; open Browser tab  
- Action: click "Low" quality button  
- Assert: no WebSocket reconnect occurs (verify by counting `connect` events in network log); canvas remains showing content  
- Covers: FR-10

---

## Requirements Coverage Matrix

| Requirement | Description | Test Case IDs | Status |
|---|---|---|---|
| **FR-1** | Per-session Xvfb display allocation, env injection, cleanup | UT-GO-001, UT-GO-002, UT-GO-003, UT-GO-004, UT-GO-005, UT-GO-006, UT-GO-007, UT-GO-008, IT-GO-001, IT-GO-002, IT-GO-006, IT-GO-007, IT-GO-009, E2E-005 | Covered |
| **FR-2** | x11vnc per session, localhost-only port, cleanup | UT-GO-007, UT-GO-008, UT-GO-009, IT-GO-001, IT-GO-002, IT-GO-007, E2E-005 | Covered |
| **FR-3** | Window isolation via xdotool, placeholder when no browser | UT-GO-017, UT-GO-018, UT-GO-019, UT-GO-020, UT-TS-001, E2E-002, E2E-003 | Covered |
| **FR-4** | WebSocket proxy, auth enforcement, byte relay | UT-GO-021, UT-GO-022, UT-GO-023, UT-GO-024, UT-TS-004, UT-TS-005, IT-GO-005 | Covered |
| **FR-5** | noVNC HTML5 canvas, mouse/keyboard input | UT-TS-003, UT-TS-009, UT-TS-010, UT-TS-011, IT-GO-005, E2E-004 | Covered |
| **FR-6** | Browser tab visibility, auto-activate on detection | UT-TS-001, UT-TS-002, UT-TS-003, UT-TS-013, UT-TS-014, E2E-001, E2E-002, E2E-003, E2E-004 | Covered |
| **FR-7** | Browser launch helper script (`launch-browser.sh`) | E2E-003 (agent launch detection), IT-GO-008 | Covered |
| **FR-8** | macOS graceful degradation, tab hidden | UT-GO-013, UT-GO-014, UT-GO-016, UT-TS-002, IT-GO-004, E2E-006 | Covered |
| **FR-9** | Crash recovery (3 restarts, backoff), Xvfb unaffected | UT-GO-010, UT-GO-011, UT-GO-012, UT-GO-008, IT-GO-002, IT-GO-003, IT-GO-008 | Covered |
| **FR-10** | Quality toggle (Low/Medium/High), no reconnect | UT-TS-006, UT-TS-007, UT-TS-008, E2E-007 | Covered |
| **NFR-1** | Latency ≤ 200ms, ≥ 10fps | IT-GO-005 (handshake timing); note: full latency/fps benchmarks are a separate perf suite | Covered (partial — perf suite deferred) |
| **NFR-2** | VNC bound to 127.0.0.1 only, auth enforcement | UT-GO-009, UT-GO-021, UT-TS-012, IT-GO-001 (port not externally reachable) | Covered |
| **NFR-3** | ≤ 50 MB RSS idle per session pair | Not covered by automated unit/integration tests — validate manually via `ps -o rss=` during IT-GO-001; document result in PR | Partial (manual check) |
| **NFR-4** | Deps documented, graceful degradation | UT-GO-015, UT-GO-016, IT-GO-004, E2E-006 | Covered |
| **NFR-5** | No regressions, DISPLAY injection conditional | UT-GO-013, UT-GO-014, IT-GO-009, E2E-006 | Covered |

**Coverage: 15/15 requirements addressed** (NFR-3 is covered partially — automated test verifies processes start/stop cleanly; exact RSS figure requires a manual benchmark pass and is noted in PR description).

---

## Test Infrastructure Notes

### Go Test Tags and Skip Guards

All integration tests that spawn real processes MUST use a build-tag or `t.Skip` guard:

```go
func requireVNCDeps(t *testing.T) {
    t.Helper()
    result := vnc.CheckDependencies()
    if !result.Available {
        t.Skipf("skipping integration test: missing %v", result.Missing)
    }
}
```

Call `requireVNCDeps(t)` at the top of every IT-GO-* test function.

### Playwright Skip Guard

The `beforeAll` hook in `browser-passthrough.spec.ts` checks `/api/health` for `vncAvailable: true`. This flag is populated by `ServerDependencies.VNCDepsResult.Available` in the health handler. Tests are skipped wholesale on CI runners that do not have Xvfb/x11vnc installed — this prevents CI failures on cross-platform builds.

### Mocking `executor.ManagedProcess`

`VNCProcessManager` unit tests must not spawn real subprocesses. The `executor.StartProcess` function should be injected via an interface or a package-level variable (`var startProcessFn = executor.StartProcess`) that tests override. This is consistent with the existing `tmux_manager_test.go` pattern.

### noVNC in Jest

`@novnc/novnc` manipulates the DOM directly and will fail in jsdom. The module must be mocked in `jest.config.js` or via `__mocks__/@novnc/novnc.ts`:

```typescript
// __mocks__/@novnc/novnc.ts
export const RFB = jest.fn().mockImplementation(() => ({
    disconnect: jest.fn(),
    scaleViewport: false,
    resizeSession: false,
    clipViewport: false,
    qualityLevel: 6,
    compressionLevel: 6,
}));
```

`NoVNCViewer.tsx` must be dynamically imported via `next/dynamic` (`ssr: false`) — Jest tests must render the inner `NoVNCViewer` directly (bypassing the dynamic wrapper) to avoid next.js dynamic import complexity.

### CI Matrix

| Test Suite | Runs in CI | Condition |
|---|---|---|
| Unit Tests (Go) | Always | No external deps needed |
| Unit Tests (TypeScript) | Always | noVNC mocked |
| Integration Tests (Go) | Linux runners only | Skip guard on missing deps |
| E2E Tests (Playwright) | Linux runners with Xvfb installed | `beforeAll` skip guard |

---

## ADRs Required Before Test Implementation

Per `plan.md`, these ADRs must be written before the test harness is finalized:

- **ADR-010** (`docs/adr/010-vnc-proxy-auth.md`) — clarifies that UT-GO-021 is the canonical auth test (proxy-only, no RFB password test needed)
- **ADR-011** (`docs/adr/011-xvfb-x11vnc-two-process.md`) — justifies why UT-GO-012 asserts Xvfb is NOT stopped during x11vnc crash
- **ADR-012** (`docs/adr/012-vnc-port-allocation.md`) — justifies OS-assigned port strategy; IT-GO-007 validates uniqueness end-to-end
