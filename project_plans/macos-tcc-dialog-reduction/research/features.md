# Feature Landscape: FocusWindow & TCC Dialog Reduction

## 1. FocusWindow callers and bundle IDs

**Implementation location:** `server/services/utility_service.go:313` — `UtilityService.FocusWindow`.

**Caller chain:**
- `server/services/session_service_delegates.go:146` — `SessionService.FocusWindow` is a thin delegate that calls `us.utilitySvc.FocusWindow(ctx, req)`. This is the ConnectRPC handler exposed on the session service.
- The RPC is defined in `proto/session/v1/session.proto:115` as `rpc FocusWindow(FocusWindowRequest) returns (FocusWindowResponse)`.
- The front-end calls it via `focusWindow(bundleId?, appName?)` in `web-app/src/lib/hooks/useSessionNotifications.ts:48`, inside `onFocusWindow` callbacks attached to notification toasts.

**What bundle IDs get passed:**

Bundle IDs are not hardcoded anywhere in the Go server. They flow entirely through the notification metadata pipeline:

1. A tmux session (or any caller) sends a `SendNotification` RPC with arbitrary `metadata` key-value pairs.
2. The `ssq-notify` shell script (`scripts/ssq-notify`) accepts `--metadata key=value` flags; any caller can include `source_bundle=<any-value>`.
3. `useSessionNotifications.ts:161` reads `event.metadata?.["source_bundle"]` and passes it as `bundleId` to `focusWindow()`.
4. The frontend then calls `FocusWindow` RPC with that bundle ID.

There is **no server-side validation or allowlist** for bundle IDs before they reach `osascript`. A notification with `metadata["source_bundle"] = "com.apple.Music"` will cause:
```
osascript -e 'tell application id "com.apple.Music" to activate'
```
…which triggers a per-app TCC prompt for Apple Events access to Music.app.

The story fields in `NotificationToast.stories.tsx` mention "Claude Code" and "Visual Studio Code" as `sourceApp` values, indicating the intended use case is developer tools, not system apps.

## 2. How `source_bundle` is populated

`source_bundle` is a **free-form, client-supplied metadata key** with no server-side restrictions. There is no field in `FocusWindowRequest` or `NotificationEvent` proto messages; instead it travels as an opaque `map<string, string>` in `metadata`.

**Production path:**
- `pkg/events/notification_metadata.go` defines the set of *known* metadata keys (`session_scoped`, `item_id`, `auto_remediating`, `risk_level`, `delivery_class`, `ssq_notify_schema`, `ssq_untrusted_type`). `source_bundle` is **not in this list** — it is an informal convention, not a formally defined field.
- `server/services/notification_service.go:61` — `requestMetadata()` strips only the `ssq_untrusted_type` key if a client tries to forge it; all other metadata keys (including `source_bundle`) pass through unchanged.
- An external script can inject any bundle ID: `ssq-notify -s "session" -t "title" --metadata "source_bundle=com.apple.Music"`.

**Why Music.app (or any system app) can end up in `source_bundle`:**
- Any poorly written notification hook or third-party script can pass an unvetted bundle ID.
- There is no schema or validation at the ingestion point.
- The frontend will happily call `FocusWindow("com.apple.Music")` → `osascript` → TCC dialog for Apple Events on Music.app.

## 3. Existing allowlist/denylist patterns for bundle IDs

**None exist.** A search for `allowlist`, `denylist`, `isValidBundle`, `sanitize.*bundle`, and `com.apple` in `server/` found no bundle-ID gating logic whatsoever. The only allowlist-adjacent code in the server relates to:
- `server/mcp/caller_identity_gate.go` — role-based tool access (not bundle IDs)
- `server/services/file_service.go` — file extension allowlist for text detection
- `server/services/capacity_monitor.go` — narrow allowlist for capacity-monitor outcomes

There is no precedent in the codebase for validating or filtering bundle IDs.

## 4. Edge cases

**App not running:**
- `utility_service.go:374-380`: The existing error handler checks `outputStr` for `"Application isn't running"` or `"Can't get application"` and returns a user-readable message like `"Application 'com.foo.bar' is not running"`. The RPC returns `Success: false` without an error code.

**App on different Space/display:**
- Not handled at all. `tell application id X to activate` will switch Spaces if the app is on a different Space, but if the app has no open windows, the behavior is OS-defined (may do nothing). No special handling exists.

**Multiple windows:**
- Not handled. `activate` focuses the frontmost window of the app; there is no window selection. If the target app has multiple windows spread across displays, the OS picks the frontmost one.

**Permission-related errors:**
- `utility_service.go:366-371`: Checks `outputStr` for `"not allowed"`, `"permission"`, `"accessibility"`, `"System Events"`. On match, returns an informative message directing the user to System Preferences > Accessibility. These are Apple Events / Automation TCC errors, not the Accessibility TCC category.

**Non-macOS:**
- `utility_service.go:338-344`: `detectPlatform() != "darwin"` returns `Success: false, Message: "window activation not supported on platform: %s"` immediately without calling osascript.

## 5. Existing error handling

`FocusWindow` uses `safeexec.CommandContext` (not raw `exec.Command`) for the osascript call. On failure:
- Logs at `WARN` level with bundle, appName, error, and raw output.
- Checks output string for known error patterns (permission, accessibility, app not running).
- Returns a `FocusWindowResponse{Success: false, Message: <human-readable>}` — never a non-nil gRPC error (the RPC always returns 200 OK, encoding the failure in the response body).
- **No retry logic.** A TCC denial is permanent until the user grants permission; the code does not detect "user denied" specifically — it falls through to the generic "failed to activate window" message.

The AppleScript used is:
```
tell application id "<bundle-id>" to activate
```
This requires the `com.apple.security.automation.apple-events` entitlement on the *calling* process (stapler-squad binary) for each *target* application, which is the root cause of per-app TCC dialogs.

## 6. Comparable window-focus features in similar tools

No code in the repo imports or references `NSWorkspace`, `AXUIElement`, `CGWindowList`, `ApplicationServices`, or any Objective-C/Swift window-management API. The codebase has no `.m`, `.swift`, or CGo files.

**Alternative approaches used by other tools (from general knowledge):**

| Tool | Mechanism | TCC impact |
|---|---|---|
| iTerm2 | `NSWorkspace.shared.activate(options:)` from native Swift/ObjC, or `CGSOrderFront` | Per-app Apple Events only when scripting *other* apps; native activation of own windows needs no TCC |
| Alfred | AppleScript `tell application X to activate` for third-party app launches; known to trigger TCC | Same osascript problem |
| Raycast | Uses `NSWorkspace.openApplication(at:configuration:completionHandler:)` for launching, and `NSRunningApplication.activate(options:)` for already-running apps | No TCC prompt for activation of running apps when called from a native process |
| Hammerspoon | `hs.application.find():activate()` wraps `NSRunningApplication.activate()` | No TCC for activation; TCC only for Accessibility if reading window list |

**Key finding:** The TCC prompt is triggered specifically by **Apple Events** (`tell application X to activate` over osascript). The alternative `NSRunningApplication.activate(options: .activateIgnoringOtherApps)` does **not** require per-app TCC; it is in the public AppKit API and callable from a native macOS process without an Apple Events entitlement. However, it requires a native binary (CGo, a small Swift/ObjC helper, or a pre-built helper tool) because it is not exposed to shell scripts.

A pure-shell alternative is `open -a "AppName"` (LSOpenURLsWithRole), which also avoids Apple Events TCC for launching — but it re-launches the app if not running rather than focusing an existing window.

## Summary of key findings

- **Single implementation, two callers:** `UtilityService.FocusWindow` is the only implementation; `SessionService.FocusWindow` delegates to it. The frontend `focusWindow()` hook in `useSessionNotifications.ts` is the only web-side caller, triggered from notification `onFocusWindow` callbacks.
- **No bundle ID validation exists:** `source_bundle` is a free-form metadata key with no allowlist, schema, or sanitization anywhere in the stack — from ingestion in `SendNotification` through the frontend calling `FocusWindow`. Any notification can inject any bundle ID, including `com.apple.Music` or other system apps, causing alarming per-app TCC dialogs.
- **Root cause of TCC dialogs is osascript Apple Events:** The fix requires either (a) replacing `osascript` with a native `NSRunningApplication.activate()` call via a CGo shim or a small helper binary, or (b) restricting `FocusWindow` to a curated allowlist of developer-tool bundle IDs (e.g. `com.apple.*` prefix → reject) so non-developer apps never reach the osascript path.
