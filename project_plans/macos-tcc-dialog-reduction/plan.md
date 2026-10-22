# Implementation Plan: macOS TCC Dialog Reduction

## Executive Summary

Replace `osascript "tell application id X to activate"` (Apple Events, triggers per-app TCC consent) with `/usr/bin/open -b <bundleID>` (Launch Services, zero TCC). Add a pre-flight running check via a small CGO shim, a server-side blocklist for non-developer bundle IDs, extract a `WindowActivator` interface, remove the now-unnecessary `apple-events` entitlement, and surface codesign status in the Settings UI.

---

## Phase 1 — Backend: Replace FocusWindow activation mechanism

**Scope:** `server/services/utility_service.go`, new `macos/` package

### 1.1 Extract `WindowActivator` interface

In `server/services/utility_service.go`, define:

```go
// WindowActivator brings a running application to the foreground on the host platform.
// The darwin implementation uses Launch Services (open -b); non-darwin is a noop.
type WindowActivator interface {
    Activate(ctx context.Context, bundleID, appName string) error
}
```

Add `activator WindowActivator` as the second parameter to `NewUtilityService`. Update the call in `server/dependencies.go` (or wherever the service is wired) to pass the platform-appropriate activator.

Replace the `osascript` block (lines 350–358 of `utility_service.go`) with a single call:

```go
if err := s.activator.Activate(ctx, bundleID, appName); err != nil {
    // existing error-classification logic stays here
}
```

### 1.2 darwin activator: pre-flight running check + `open -b`

New file: `macos/window_activator_darwin.go` (build tag `//go:build darwin`)

Two-step logic:
1. **Running check** — use `NSRunningApplication.runningApplicationsWithBundleIdentifier:` via CGO (or `lsappinfo` subprocess as a nocgo fallback) to check whether the app is running before calling `open -b`. If not running, return a structured `ErrAppNotRunning` so the RPC can return the existing human-readable message without launching a stray instance.
2. **Activate** — call `/usr/bin/open -b <bundleID>`. This uses Launch Services and does not send any Apple Events, eliminating the per-app TCC dialog entirely.

New file: `macos/window_activator_darwin_cgo.go` (build tag `//go:build darwin && cgo`)
- CGO preamble imports `<Cocoa/Cocoa.h>`.
- `runningApplicationsWithBundleIdentifier:` returns count 0 → not running.
- Must dispatch to main thread if called from a goroutine — use `dispatch_async(dispatch_get_main_thread(), ...)`. OR, since we only need a read (not activate via AppKit), `runningApplicationsWithBundleIdentifier:` is safe off main thread per Apple docs.

New file: `macos/window_activator_darwin_nocgo.go` (build tag `//go:build darwin && !cgo`)
- Subprocess fallback: `lsappinfo find bundleid <id>` to check running status; then `open -b`.
- Pattern mirrors `session/procinfo/openfiles_darwin_nocgo.go`.

New file: `macos/window_activator_other.go` (build tag `//go:build !darwin`)
- Noop activator: returns `errors.New("window activation not supported on this platform")`.

### 1.3 Bundle ID sanitization

In `utility_service.go`'s `FocusWindow` RPC handler, before calling `s.activator.Activate`, add a bundle ID gate:

```go
if bundleID != "" && isBlockedBundleID(bundleID) {
    return connect.NewResponse(&sessionv1.FocusWindowResponse{
        Success: false,
        Message: fmt.Sprintf("FocusWindow is not supported for %q (non-developer app)", bundleID),
    }), nil
}
```

`isBlockedBundleID` (package-private, in `macos/bundle_id_filter.go` or inline):
- Exact blocklist: `com.apple.Music`, `com.apple.TV`, `com.apple.Photos`, `com.apple.iTunesU`, `com.apple.podcasts`.
- **Not** a prefix allowlist for `com.apple.*` — that would block `com.apple.Terminal` and `com.apple.dt.Xcode`.
- Log the blocked bundle ID at `INFO` level so real-world data can inform future list growth.

### 1.4 Update error handling

Remove the Apple Events / Automation TCC error strings (`"not allowed"`, `"permission"`, `"accessibility"`, `"System Events"`) from the `osascript` error path. Keep `"Application isn't running"` / `"Can't get application"` pattern matching — re-express it via the structured `ErrAppNotRunning` from the activator.

Add a new error message for the blocked-bundle case (return immediately, no activator call).

### 1.5 Entitlement cleanup (defer until osascript fully removed)

Once the `appName`-only path (no bundle ID) is also ported off `osascript` or removed, remove:
- `com.apple.security.automation.apple-events = true` from `entitlements.plist`.
- `NSAppleEventsUsageDescription` from `macos/Info.plist` (or update it to reflect the new usage if any Apple Events path remains).
- Update `docs/how-to/macos-codesigning.md` to remove the `tccutil reset AppleEvents` diagnostic step.

If the `appName`-only path is kept on `osascript` temporarily, document this and the entitlement stays.

---

## Phase 2 — Backend: Codesign status API

### 2.1 Expose codesign status to the UI

Add a field `codesign_status` (enum: `UNKNOWN`, `SIGNED`, `UNSIGNED`) to the existing `GetSystemStatus` RPC response (or a new lightweight `GetMacosSetupStatus` RPC if the former is too broad).

Implementation in `server/services/` (darwin build tag):
```go
func detectCodesignStatus() CodesignStatus {
    out, err := exec.Command("codesign", "-dv", os.Args[0]).CombinedOutput()
    if err != nil { return CodesignStatusUnsigned }
    if strings.Contains(string(out), "StaplerSquadDev") { return CodesignStatusSigned }
    return CodesignStatusSigned // any valid signing is acceptable
}
```

Cache the result for the process lifetime (codesign status doesn't change while running).

---

## Phase 3 — Frontend: Focus Window button states

**Scope:** `web-app/src/components/ui/NotificationToast.tsx`, `NotificationToast.css.ts`, `web-app/src/lib/hooks/useSessionNotifications.ts`

### 3.1 Read codesign status

Fetch `codesign_status` from the new/updated RPC on app load (or hook into the existing system-status polling). Store in React context or a Zustand slice.

### 3.2 Button state: not codesigned

When `codesign_status !== SIGNED` (or unknown):
- Render Focus Window button dimmed (muted border, 50% opacity).
- On click, show inline toast card instead of calling the RPC:
  ```
  🔒 Focus Window needs one-time setup
  Run: make setup-codesign    [Copy]
  Then restart the service.   [Learn more →]
  ```
- This eliminates the alarming dialog for new installs.

### 3.3 Inline error feedback on RPC failure

When `FocusWindowResponse.success === false`:
- Show the `message` field in an inline toast expansion (currently silently dropped).
- For the blocked-bundle case: "This app can't be used as a Focus Window target."
- For app-not-running: existing "App is not running" message — already handled, just needs to surface.

### 3.4 Success feedback

On `success === true`: dismiss the notification toast after 800 ms (currently it stays indefinitely with no feedback).

---

## Phase 4 — Frontend: Settings page macOS section

**Scope:** `web-app/src/app/settings/` (or wherever `PushNotificationSettings.tsx` lives)

### 4.1 New "macOS Permissions" section

Following the `PushNotificationSettings.tsx` pattern (status badge + instructions):

| Status | Display |
|--------|---------|
| Signed | ✅ Code signing configured |
| Unsigned / unknown | ⚠ Setup needed — run `make setup-codesign` |

Include:
- Plain-English explanation: "Code signing lets stapler-squad switch your editor window without triggering unexpected macOS permission dialogs."
- `[Copy command]` button for `make setup-codesign`.
- `[Learn more →]` link to `docs/how-to/macos-codesigning.md`.

### 4.2 Route meta

Add a row to `web-app/src/app/route-meta.ts` if a new page is created. If added to an existing settings page as a section, no route-meta change needed.

---

## Phase 5 — Docs update

- Update `docs/how-to/macos-codesigning.md`:
  - Note that TCC dialogs for Apple Events are eliminated by the new `open -b` path.
  - Remove the `tccutil reset AppleEvents` step (no longer needed) if entitlement removed.
  - Add "What happens if I haven't run `make setup-codesign`?" FAQ entry.
- Update `docs/reference/state-isolation.md` if any new config state is introduced.

---

## Sequencing and dependencies

```
Phase 1.1 (interface seam) → Phase 1.2 (darwin activator) → Phase 1.3 (blocklist)
                          ↓
                    Phase 1.4 (error handling)
                          ↓
                    Phase 1.5 (entitlement cleanup — after osascript fully removed)

Phase 2.1 (codesign API) → Phase 3.1 (read status) → Phase 3.2 (button state)
                                                    → Phase 3.3 (error feedback)
                                                    → Phase 3.4 (success feedback)
                                                             ↓
                                                       Phase 4.1 (settings section)

Phase 5 (docs) — last, after Phases 1–4 are complete
```

Phases 1 and 2 can be built in parallel by separate engineers. Phase 3 depends on Phase 2 (needs the codesign status RPC to exist). Phase 4 can be parallelized with Phase 3.

---

## Testing strategy

### Unit tests (Go)
- `TestWindowActivator_BlockedBundleID` — isBlockedBundleID returns true for `com.apple.Music`, `com.apple.TV`; returns false for `com.apple.Terminal`, `com.microsoft.VSCode`.
- `TestWindowActivator_AppNotRunning` — mock activator returns `ErrAppNotRunning`; FocusWindow RPC returns `Success=false` with expected message.
- `TestWindowActivator_Darwin` — integration test gated `//go:build darwin`; verifies `open -b com.apple.Terminal` succeeds when Terminal is running.

### Unit tests (React)
- `NotificationToast` snapshot: dimmed button when `codesignStatus = UNSIGNED`.
- Inline error card renders when `success = false`.
- Inline setup card shown on click when unsigned (RPC not called).

### Manual verification (darwin)
1. Before: `tccutil reset AppleEvents com.stapler-squad` to clear grants.
2. After installing with `make install-service`, trigger Focus Window on a notification.
3. Confirm: no TCC dialog appears.
4. Confirm: editor window comes to front.
5. Confirm: `sqlite3 ~/Library/Application\ Support/com.apple.TCC/TCC.db "SELECT client, auth_value FROM access WHERE service='kTCCServiceAppleEvents';"` shows no new row for `com.stapler-squad`.

---

## Risk register

| Risk | Likelihood | Mitigation |
|------|-----------|------------|
| `open -b` launches app if not running | High (known) | Pre-flight running check via CGO `NSRunningApplication` |
| CGO main-thread requirement for `NSRunningApplication` read | Medium | Only use `runningApplicationsWithBundleIdentifier:` (safe off-thread); no AppKit UI calls |
| `activateIgnoringOtherApps` deprecated on macOS 14 (Sonoma) | Not applicable | We're using `open -b`, not AppKit activate |
| Electron apps (Cursor, Windsurf) have opaque bundle IDs that change | Medium | Start with exact known IDs; log unrecognized IDs for future list growth |
| `appName`-only path still uses `osascript` | Low | Document clearly; the common path (bundle ID present) is fixed |
| nocgo fallback subprocess (`lsappinfo`) not available on older macOS | Low | `lsappinfo` ships in macOS 10.9+; acceptable floor |
