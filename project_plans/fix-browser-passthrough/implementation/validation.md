# Fix Browser Passthrough — Validation Plan

## Unit Tests — TypeScript/Jest

### BrowserTab Component (`BrowserTab.test.tsx`)

**UT-TS-001** `BrowserTab_shows_unavailable_when_cdpStatus_UNAVAILABLE`
- Setup: render `<BrowserTab cdpState={{ status: CDP_STATUS_UNAVAILABLE }} ...>`
- Assert: "Browser passthrough unavailable" text present; CDPViewer not mounted
- Covers: FR-1, FR-2

**UT-TS-002** `BrowserTab_shows_waiting_when_cdpStatus_NO_BROWSER`
- Setup: render with `cdpState={{ status: CDP_STATUS_NO_BROWSER }}`
- Assert: "No browser detected" placeholder visible; CDPViewer not mounted
- Covers: FR-2, FR-4

**UT-TS-003** `BrowserTab_shows_connecting_when_cdpStatus_WAITING`
- Setup: render with `cdpState={{ status: CDP_STATUS_WAITING }}`
- Assert: "Connecting to browser..." placeholder visible; CDPViewer not mounted
- Covers: FR-2, FR-4

**UT-TS-004** `BrowserTab_mounts_viewer_when_cdpStatus_STREAMING`
- Setup: mock CDPViewer; render with `cdpState={{ status: CDP_STATUS_STREAMING }}`
- Assert: CDPViewer mock is called; `isReady=true` path reached
- Covers: FR-2

**UT-TS-005** `BrowserTab_viewer_stays_mounted_across_tab_switches`
- Setup: mount with STREAMING; flip `isVisible` false then true
- Assert: CDPViewer not remounted (hasBeenReadyRef pattern preserved)
- Covers: FR-2 (keep-alive)

**UT-TS-006** `BrowserTab_does_not_mount_viewer_before_STREAMING`
- Setup: render with WAITING, then update to NO_BROWSER — never STREAMING
- Assert: CDPViewer mock is never called
- Covers: FR-2

### SessionDetailView Browser Tab Availability (`SessionDetailView.test.tsx`)

**UT-TS-007** `SessionDetailView_browser_tab_disabled_when_cdpStatus_UNAVAILABLE`
- Setup: render session with `cdpState.status = CDP_STATUS_UNAVAILABLE`
- Assert: Browser tab button has `aria-disabled="true"` or `disabled` attribute
- Covers: FR-1

**UT-TS-008** `SessionDetailView_browser_tab_enabled_when_cdpStatus_STREAMING`
- Setup: render session with `cdpState.status = CDP_STATUS_STREAMING`
- Assert: Browser tab button is enabled (no disabled attribute)
- Covers: FR-1

**UT-TS-009** `SessionDetailView_browser_tab_enabled_when_cdpStatus_NO_BROWSER`
- Setup: render session with `cdpState.status = CDP_STATUS_NO_BROWSER`
- Assert: Browser tab button is enabled (CDP is available even before Chrome launched)
- Covers: FR-1

**UT-TS-010** `SessionDetailView_disabled_tab_tooltip_references_chrome`
- Setup: render with `cdpState.status = CDP_STATUS_UNAVAILABLE`; inspect the disabled tab button
- Assert: button `title` or `aria-label` contains "Chrome or Chromium"
- Assert: does NOT contain "Xvfb" or "x11vnc" or "xdotool"
- Covers: FR-3

**UT-TS-011** `SessionDetailView_does_not_use_vncState_for_availability`
- Setup: render session with `vncState.status = VNC_STATUS_UNAVAILABLE` AND `cdpState.status = CDP_STATUS_STREAMING`
- Assert: Browser tab is enabled (cdpState wins)
- Covers: FR-5 (macOS case: VNC unavailable but CDP streaming)

## Manual Verification

**MV-001** macOS with Chrome installed
- Start server on macOS host where Chrome is installed
- Create a session; verify Browser tab is enabled (not greyed)
- In session terminal, run `google-chrome --no-sandbox &` or equivalent
- Verify Browser tab activates (CDPViewer renders frames)
- Covers: FR-5

**MV-002** Linux with Chrome, without Xvfb/x11vnc
- On a Linux machine with Chrome but without `xvfb` or `x11vnc` in PATH
- Create session; verify Browser tab is enabled
- Launch Chrome; verify CDPViewer streams
- Covers: FR-6

**MV-003** Host without Chrome
- Create session on host where no Chrome/Chromium binary is found
- Verify Browser tab is disabled
- Hover over it; verify tooltip reads "Chrome or Chromium" (not Xvfb)
- Covers: FR-3, FR-1

**MV-004** Mobile (touch) — iOS Safari at `onyx.staplerhome.internal:8444`
- Navigate to session detail view
- Tap disabled Browser tab
- Verify toast message shows "Chrome or Chromium" copy (not the old Xvfb copy)
- Covers: NFR-2

## Requirements Coverage

| Requirement | Tests |
|---|---|
| FR-1: cdpState drives tab availability | UT-TS-007, UT-TS-008, UT-TS-009, UT-TS-011 |
| FR-2: BrowserTab reads CDPState | UT-TS-001–006 |
| FR-3: Correct tooltip text | UT-TS-010, MV-003 |
| FR-4: Placeholder text updated | UT-TS-002, UT-TS-003 |
| FR-5: macOS works | UT-TS-011, MV-001 |
| FR-6: Linux parity | MV-002 |
| FR-7: No backend changes | Build passes, no Go test regressions |
| NFR-1: No tests broken | `make test` / `bunx jest` pass |
| NFR-2: Mobile tooltip | MV-004 |
