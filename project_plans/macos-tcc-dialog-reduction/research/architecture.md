# Architecture Research: macOS TCC Dialog Reduction

## 1. FocusWindow Handler Architecture

**File:** `server/services/utility_service.go:311–395`

`FocusWindow` is a plain method on `UtilityService` with no injected backend dependency — the activation strategy is hardcoded inline:

```
RPC arrives → validateLocalhostOriginForFocus → detectPlatform()
  → if darwin: build osascript string → safeexec.CommandContext("osascript", "-e", script)
  → if !darwin: return Success=false
```

The critical lines (350–355):
```go
if bundleID != "" {
    script = fmt.Sprintf(`tell application id "%s" to activate`, bundleID)
} else {
    script = fmt.Sprintf(`tell application "%s" to activate`, appName)
}
cmd := safeexec.CommandContext(ctx, "osascript", "-e", script)
```

`tell application id "<bundle>" to activate` is an **Apple Events send** — it triggers TCC consent for every distinct target bundle ID, because each app is a separate Apple Events recipient. This is the root cause of the per-app TCC dialog storm.

The handler has **no interface seam** — the activation mechanism cannot be swapped without modifying `utility_service.go` directly.

### Dependencies on UtilityService

`UtilityService` currently holds only `approvalStore` and `reviewQueuePoller`. No platform-activation backend is injected. Error handling in the existing code is string-based (checks `outputStr` for "not allowed", "permission", etc.) rather than structured.

---

## 2. Abstraction Seams Available

No activation seam exists today. The cleanest insertion point is a new interface injected into `UtilityService`:

```go
// WindowActivator activates an application window on the host platform.
// The darwin implementation may use NSRunningApplication (CGO) or `open -b`.
type WindowActivator interface {
    Activate(ctx context.Context, bundleID, appName string) error
}
```

This fits the existing constructor pattern — `NewUtilityService` already takes `approvalStore` as a dependency, so a second parameter `activator WindowActivator` is the natural extension. A `NewNoopWindowActivator()` covers Linux/CI without conditional compilation at the service layer.

The existing test in `window_debug_test.go` tests `FocusWindow` indirectly via the RPC and does not mock the activation backend, which means it will need updating once a seam is added. The non-darwin path test (`TestFocusWindow_NonDarwinPlatform`) already passes `Success=false` gracefully, so the noop activator pattern matches existing test expectations.

---

## 3. Darwin-Specific CGO vs Platform-Agnostic Path

**Current CGO usage:** `infoplist_darwin.go` (build tag `darwin && cgo`) uses `import "C"` to embed `Info.plist` via CGO linker flags — it is a plist-embedding mechanism, not a window-activation mechanism. No CGO code currently performs window activation.

**Recommended structure for the replacement:**

| File | Build tag | Purpose |
|---|---|---|
| `macos/activate_darwin.go` | `//go:build darwin` | NSRunningApplication path via `open -b` subprocess (no CGO needed for this approach) |
| `macos/activate_darwin_cgo.go` | `//go:build darwin && cgo` | Optional CGO path using `objc/runtime.h` + NSRunningApplication if pure-subprocess `open -b` has latency issues |
| `macos/activate_other.go` | `//go:build !darwin` | Noop activator for Linux/Windows |

**`open -b <bundle-id>` vs `NSRunningApplication`:**
- `open -b com.example.App` uses Launch Services and does **not** send Apple Events to the target app — it tells `launchd`/`lsd` to bring the app forward, which is a different IPC channel that does not trigger per-app Apple Events TCC consent.
- `NSRunningApplication.activateWithOptions:` (CGO/Objective-C) also avoids Apple Events; it calls directly into AppKit via `NSWorkspace`.
- Either alternative avoids the per-app TCC dialog. `open -b` is simpler (no CGO) and testable with `safeexec`.

**Recommendation:** Start with `open -b` (no CGO dependency) for the darwin activator. A CGO path can be added later if activation latency proves unacceptable.

---

## 4. source_bundle Sanitization: Where Should It Live?

**Current data flow:**

```
session/hook → NotificationEvent.metadata["source_bundle"] → SSE stream
  → useSessionNotifications.ts:161 reads event.metadata?.["source_bundle"]
  → focusWindow(sourceBundleId, sourceApp) → FocusWindow RPC → osascript
```

The `source_bundle` value is written into notification metadata by the hook handler (see `server/services/ssq_hook_handler_stamp_test.go`), flows through the event bus untouched, and arrives at the client as a raw string. The FocusWindow RPC then passes it directly to `osascript`.

**Non-developer bundle IDs** (e.g. `com.apple.Preview`, `com.apple.finder`) that should never be sent Apple Events must be blocked somewhere. The correct layer is the **RPC handler** (`FocusWindow` in `utility_service.go`), because:
1. The client is untrusted (it's a browser tab, not server-owned code).
2. The hook handler cannot know the full set of disallowed IDs at event-creation time.
3. A server-side allowlist or regex guard at the RPC layer blocks misuse regardless of what the client sends.

An allowlist of recognized developer/editor bundle IDs (VS Code, Cursor, Zed, iTerm2, Terminal, Warp, etc.) is the right disposition. Unknown bundle IDs should return `Success=false` with a clear message rather than being passed through.

**Alternatively**, if using `open -b` (which is lower-risk than Apple Events), the sanitization risk is reduced — but an allowlist is still good practice to prevent the server from launching or foregrounding arbitrary installed apps on the user's machine.

---

## 5. Disposition for utility_service.go

**Recommended: Refactor-first / Isolate via seam**

The handler is 85 lines including comments, platform detection, error handling, and the activation call. The right move is:

1. Extract the activation logic into a `WindowActivator` interface (2–3 lines of interface definition).
2. Provide a `darwinWindowActivator` (using `open -b`) under `macos/` with a build tag.
3. Inject via `NewUtilityService(approvalStore *ApprovalStore, activator WindowActivator)`.
4. Keep all the existing RPC scaffolding (localhost check, platform guard, argument validation, error response shape) in `utility_service.go` — it is not the problem.

This is a **seam insertion**, not a full rewrite. The service file shrinks by ~20 lines (the osascript block becomes one `activator.Activate(ctx, bundleID, appName)` call), and the darwin-specific CGO/subprocess code moves to its own file where it can carry a build tag and be tested in isolation.

Extending as-is (`extend-as-is`) is wrong: it would add a CGO block inline in a file that has no build tag and is compiled on all platforms.

---

## 6. Codesign + TCC Grant Flow End-to-End

**Current flow (as verified by reading Makefile and plist files):**

```
make setup-codesign
  └─ scripts/setup-codesign.sh  → creates self-signed cert "StaplerSquadDev" in Keychain

make install-service
  └─ _codesign-binary
       └─ codesign --force --sign "StaplerSquadDev"
                   --entitlements entitlements.plist
                   stapler-squad
```

`entitlements.plist` contains `com.apple.security.automation.apple-events = true`.
`macos/Info.plist` contains:
- `CFBundleIdentifier = com.stapler-squad`
- `NSAppleEventsUsageDescription` (the user-facing dialog string)

`infoplist_darwin.go` (build tag `darwin && cgo`) uses CGO to embed `Info.plist` into the `__TEXT __info_plist` Mach-O section so that TCC can read the bundle ID from the binary itself (not a side-car `.app` bundle).

**When `open -b` replaces `osascript`:**
- The `com.apple.security.automation.apple-events` entitlement is still needed if *any* Apple Events path is used. If `osascript` is fully removed, this entitlement becomes unnecessary.
- The `NSAppleEventsUsageDescription` in Info.plist can be removed or updated.
- `tcc-reset` (`sudo tccutil reset All com.stapler-squad`) resets all TCC rows for the bundle ID — useful for testing that the new path does not generate new TCC prompts.
- The `CFBundleIdentifier` and `codesign --sign "StaplerSquadDev"` flow is unchanged and still needed for TCC grant persistence (stable identity across binary rebuilds).

**The per-app TCC prompt root cause:** `tell application id X to activate` is an Apple Event **send** to app `X`. macOS TCC tracks the pair `(sender bundle ID, target bundle ID)`. Stapler-squad's bundle ID is the sender; each different target app (VS Code, iTerm2, Chrome, …) requires a separate TCC row and thus a separate consent dialog. Using `open -b` avoids the Apple Events send entirely — no per-target TCC row, no per-app dialog.

---

## Summary

- **FocusWindow** (`utility_service.go:311–395`) has no abstraction seam; the `osascript` Apple Events call is hardcoded inline. The seam to insert is a `WindowActivator` interface injected into `UtilityService`.
- **Replacement strategy:** `open -b <bundle-id>` subprocess (no CGO, pure `darwin` build tag) avoids Apple Events and thus eliminates per-app TCC dialogs. A `macos/activate_darwin.go` file carries the implementation; a `macos/activate_other.go` noop covers non-darwin.
- **Bundle ID sanitization** belongs at the RPC layer (server-side allowlist in `FocusWindow`) — not at notification event creation or client side — because the browser client is untrusted and the hook handler cannot enumerate all unsafe IDs at event-creation time.
