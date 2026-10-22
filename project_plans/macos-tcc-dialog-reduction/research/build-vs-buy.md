# Build-vs-Buy: macOS Window Focus Without Apple Events TCC Dialogs

**Context:** `server/services/utility_service.go:358` runs  
`osascript -e 'tell application id "X" to activate'`  
which triggers `kTCCServiceAppleEvents` consent per target bundle ID. Every unique app the user focuses via stapler-squad generates its own dialog; the Apple Music one reads "access Apple Music, your music and video activity, and your media library" — alarming for a developer tool.

**Goal of this document:** Evaluate replacement mechanisms for the `activate` verb that do not trigger per-app `kTCCServiceAppleEvents` dialogs.

---

## Option 1: `/usr/bin/open -b <bundle_id>` (shell-out) — RECOMMENDED

### What it does

`open -b com.example.App` invokes the macOS **Launch Services** API (`LSOpenApplication`), not
Apple Events. Launch Services brings a running application's frontmost window to the front, or
launches the app if it is not running — exactly the same observable outcome as `tell application
id X to activate`, but through a different kernel path.

### TCC impact

**None.** `kTCCServiceAppleEvents` is only triggered when one process sends Apple Events to
another. Launch Services is a separate framework that does not go through the Apple Events daemon.
`open` has been a public system utility since macOS 10.0; it carries its own entitlements and
never generates a per-caller TCC prompt.

### Implementation

```go
// focusWindowOpen brings app to front via Launch Services. No Apple Events TCC.
// bundleID must be non-empty; returns an error if the app is not found or the
// open binary fails.
func focusWindowOpen(ctx context.Context, bundleID string) error {
    cmd := safeexec.CommandContext(ctx, "/usr/bin/open", "-b", bundleID)
    out, err := cmd.CombinedOutput()
    if err != nil {
        return fmt.Errorf("open -b %s: %w (output: %s)", bundleID, err, strings.TrimSpace(string(out)))
    }
    return nil
}
```

Fallback by app name is not directly possible with `open -b` (bundle ID only), but the existing
`req.Msg.AppName` path can fall back to the current `osascript` approach — or simply be
unsupported when no bundle ID is supplied, since the Apple-Music dialog only fires via bundle ID.

### Edge cases

| Scenario | Behaviour |
|---|---|
| App not running | `open -b` launches the app (same as current `tell application id X to activate`) |
| App sandboxed / no LSOpen entitlement | `open -b` still works — `open` carries its own Launch Services rights |
| Sandboxed app that refuses to activate | `open -b` returns exit 1; stapler-squad logs and returns `Success: false` |
| App name only (no bundle ID) | Fall back to existing osascript path or return unsupported; see below |

### Pros

- Zero TCC dialogs for Apple Events — eliminates the root cause
- Two-line change at the call site in `FocusWindow`; existing logic unchanged
- No CGO, no new dependencies, no build flag changes
- `open` ships with every macOS version; no version gating needed
- Observable behaviour is identical to the current implementation for the common case

### Cons

- Cannot target a specific window within an app (same limitation as the current implementation)
- App-name–only fallback disappears; the existing `req.Msg.AppName` code path would need the
  caller to also supply a bundle ID, or a separate lookup step (e.g. `lsappinfo` or CGO), or
  simply remain on `osascript` for that sub-path (which is rarely exercised today)

### Verdict: **Recommended** — primary replacement

---

## Option 2: CGO `NSRunningApplication` shim — VIABLE

### What it does

`NSRunningApplication` (Cocoa/AppKit) exposes `activateWithOptions:` to bring a running app to
front without sending any Apple Events. This is the same call that Spotlight and the Dock use
internally.

### TCC impact

**None.** `NSRunningApplication.activateWithOptions:` uses Mach ports and window-server IPC
directly; it does not involve the Apple Events daemon or `kTCCServiceAppleEvents`.

### Implementation sketch

```c
// window_focus_darwin.m
#include <Cocoa/Cocoa.h>

// Returns 0 on success, -1 if no running instance found.
int activateApp(const char *bundleID) {
    @autoreleasepool {
        NSString *bid = [NSString stringWithUTF8String:bundleID];
        NSArray<NSRunningApplication *> *apps =
            [NSRunningApplication runningApplicationsWithBundleIdentifier:bid];
        if (apps.count == 0) return -1;
        BOOL ok = [[apps firstObject]
            activateWithOptions:NSApplicationActivateIgnoringOtherApps];
        return ok ? 0 : -2;
    }
}
```

```go
// window_focus_darwin.go
//go:build darwin && cgo

package windowfocus

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
#include "window_focus_darwin.m"
*/
import "C"
import "fmt"

func ActivateByBundleID(bundleID string) error {
    cid := C.CString(bundleID)
    defer C.free(unsafe.Pointer(cid))
    rc := C.activateApp(cid)
    switch rc {
    case 0:
        return nil
    case -1:
        return fmt.Errorf("no running instance of %s found", bundleID)
    default:
        return fmt.Errorf("activateWithOptions failed for %s", bundleID)
    }
}
```

**Build requirement:** `CGO_ENABLED=1` — already unconditionally set in the Makefile. The
`-framework Cocoa` flag is safe alongside the existing `CGO_LDFLAGS` that embed `Info.plist`.
A `//go:build darwin && cgo` guard keeps the stub (returning `errors.New("unsupported")`) for
non-Darwin or `CGO_ENABLED=0` builds, matching the pattern in `session/procinfo/openfiles_darwin.go`.

### Why `open -b` is still preferred

CGO adds Obj-C compilation to the build, which works today but:
1. Requires linking AppKit / Cocoa — not currently in the link graph, adds ~1 MB to binary size.
2. Cross-compilation (Darwin → Linux) becomes impossible for this package without `-tags nocgo`.
3. The `activateWithOptions:` call must run on a thread that has a `NSRunLoop` running or in a
   dedicated AppKit thread — in a pure Go web server this is a footgun (AppKit is not
   thread-safe). `open -b` delegates all of this to the OS-managed `open` process.
4. Return code `-2` ("activateWithOptions returned NO") is underdocumented; in practice it means
   the app is running but in background or minimised and cannot come to front without user
   interaction (e.g. another fullscreen app is in the way). `open -b` handles this gracefully.

### Verdict: **Viable** — use as fallback if `open -b` proves insufficient for a specific use case (e.g. needing to target a specific window within an app), but don't start here.

---

## Option 3: AXUIElement (Accessibility API) — NOT RECOMMENDED

### What it does

The Accessibility API (`AXUIElement`) lets a process inspect and control other apps' UI
hierarchies. It can raise windows (`kAXRaiseAction`) and set `kAXFocusedApplicationAttribute`.

### TCC impact

**Replaces one class of dialog with another.** AXUIElement requires `kTCCServiceAccessibility` —
a single system-level grant, not per-app. However:

- The Accessibility dialog is not less alarming: "stapler-squad would like to control this
  computer using accessibility features."
- It must be granted in System Settings > Privacy > Accessibility — the same friction level.
- Adding Accessibility to the binary's `Info.plist` entitlement set widens the attack surface
  and will draw attention in future App Store review if the app ever ships there.

### Additional complexity

AXUIElement requires `CGO_ENABLED=1` + `#cgo LDFLAGS: -framework ApplicationServices`, plus
careful handling of `kAXErrorCannotComplete` (sandboxed apps silently refuse Accessibility
actions). The implementation is meaningfully more complex than Option 2 for the same observable
result: bringing an app to front.

### Verdict: **Not recommended** — trades per-app Apple Events dialogs for a single but more
alarming Accessibility dialog, while adding CGO complexity. `open -b` achieves the same goal
with no TCC at all.

---

## Option 4: Alternative AppleScript verbs — NOT RECOMMENDED

### The question

Is there an AppleScript verb that brings an app to front *without* triggering
`kTCCServiceAppleEvents`?

### Answer: No

Every `tell application X … end tell` block in AppleScript sends Apple Events to the named
target process, regardless of the verb used. The TCC check happens at the point the Apple Events
daemon routes the event to the target — not at the verb level. Examples:

| Script | Still triggers kTCCServiceAppleEvents? |
|---|---|
| `tell application id X to activate` | Yes |
| `tell application id X to quit` | Yes |
| `tell application "System Events" to set frontmost of process "X" to true` | Yes — for System Events itself, a single grant, but still Apple Events |
| `do shell script "open -b X"` | Yes for the `do shell script` Apple Event to osascript runner — plus Launch Services for the actual open call |

The `System Events` approach (`tell application "System Events" to set frontmost of process "X"
to true`) deserves special note: it does consolidate all Apple Events into a *single* TCC grant
for System Events rather than one per target app. This is an improvement over the current
approach, but not the same as zero TCC dialogs. It also has higher latency (two Apple Events
hops) and `System Events` can be temporarily unavailable during login/logout.

### Verdict: **Not recommended** — no AppleScript verb avoids `kTCCServiceAppleEvents`
entirely. `open -b` is cleaner.

---

## Option 5: Third-party Go libraries — NOT RECOMMENDED

### Libraries evaluated

| Library | Approach | TCC impact | Status |
|---|---|---|---|
| `github.com/go-vgo/robotgo` | CGO + Accessibility API | kTCCServiceAccessibility | Active; heavy (xdotool/X11 code included even on Darwin); same TCC problem as Option 3 |
| `github.com/progrium/macdriver` | CGO + full Obj-C bridge | None if using NSRunningApplication | Active but API unstable; entire Cocoa framework surface; heavyweight for a two-line use case |
| `github.com/keybase/go-keychain` / others | Unrelated | — | — |

No stable, lightweight Go library wraps `NSRunningApplication.activateWithOptions:` without
bundling significant other functionality. The CGO shim in Option 2 is ~25 lines and covers
exactly what's needed; importing a third-party library doesn't improve on that.

### Verdict: **Not recommended** — no existing library hits the sweet spot of narrow scope,
stable API, and no TCC dialogs. Build Option 2 if `open -b` is insufficient.

---

## Option 6: Bundle ID allowlist / blocklist — COMPLEMENTARY, not a replacement

### The question

Can stapler-squad filter out "alarming" bundle IDs (e.g., `com.apple.Music`, `com.apple.TV`)
server-side to prevent the most confusing dialogs?

### Approach

A blocklist of non-developer apps (media, consumer apps) that should never be FocusWindow
targets. If a `FocusWindow` request arrives for a blocked bundle ID, the server logs and returns
`Success: false, Message: "FocusWindow is not supported for this application"`.

This also addresses the requirements doc's AC 5: "source_bundle values that are non-developer
apps … result in a no-op FocusWindow call with a meaningful log message."

### OSS lists of developer-tool bundle IDs

No authoritative maintained OSS list exists specifically for "developer tools". However:

- Apple's own `com.apple.*` namespace covers first-party apps; a simple prefix check (`strings.HasPrefix(bundleID, "com.apple.")`) would block all Apple system apps.
- The `source_bundle` logic (per requirements doc section 4) seems to set bundle IDs from the frontmost app at session-creation time. A curated allowlist of known developer tools is more targeted than a blocklist:

```go
// knownDeveloperBundles is an allowlist of bundle IDs known to be developer tools.
// FocusWindow silently no-ops for apps not in this list when SourceBundle
// filtering is active.
var knownDeveloperBundles = map[string]bool{
    "com.apple.Terminal":                    true,
    "com.googlecode.iterm2":                true,
    "net.kovidgoyal.kitty":                 true,
    "com.github.wez.wezterm":               true,
    "com.microsoft.VSCode":                 true,
    "com.microsoft.VSCodeInsiders":         true,
    "dev.zed.Zed":                          true,
    "com.jetbrains.intellij":               true,
    "com.jetbrains.goland":                 true,
    "com.jetbrains.pycharm":                true,
    "com.jetbrains.webstorm":               true,
    "com.apple.dt.Xcode":                   true,
    "com.sublimetext.4":                    true,
    "com.panic.Nova":                       true,
    "abnerworks.Typora":                    true,
    "com.electron.cursor":                  true,  // Cursor IDE
    "com.todesktop.230313mzl4w4u92":        true,  // Cursor (older)
}
```

This blocklist / allowlist approach is **complementary** — it does not replace the need to
eliminate the `osascript` call, but it ensures that even with the current implementation, the
Apple Music dialog cannot be triggered by a misdirected `source_bundle`.

### Verdict: **Recommended as a complement** to Option 1, not a standalone fix. Implement AC 5
(filter non-developer `source_bundle` values) alongside the `open -b` replacement.

---

## Summary and Recommendation

| Option | Verdict | TCC dialogs eliminated? | CGO required? | Effort |
|---|---|---|---|---|
| 1. `open -b` | **Recommended** | Yes — completely | No | ~10 lines |
| 2. CGO NSRunningApplication | Viable (fallback) | Yes — completely | Yes | ~40 lines + .m file |
| 3. AXUIElement | Not recommended | No (trades for Accessibility) | Yes | High |
| 4. Alternative AppleScript verbs | Not recommended | No | No | — |
| 5. Third-party Go libraries | Not recommended | Depends on lib | Yes | High |
| 6. Bundle ID allowlist | Complementary | Partial (AC 5 only) | No | ~20 lines |

**Recommended implementation plan:**

1. Replace `osascript` in `FocusWindow` with `/usr/bin/open -b <bundleID>` (Option 1).
   - Gate on `bundleID != ""` (the bundle-ID path already exists; the name-only fallback can
     remain on the existing `osascript` path until a bundle ID lookup is implemented, or simply
     be documented as unsupported).
   - Remove the `kTCCServiceAppleEvents`-related error handling in the failure path.

2. Add a `source_bundle` filter (Option 6) to block non-developer apps from being passed as
   FocusWindow targets in the first place, satisfying AC 5.

3. Keep Option 2 (CGO shim) in reserve if a future requirement calls for per-window targeting
   (e.g., focusing a specific terminal window rather than the whole app). Do not implement it
   now; `open -b` is sufficient for the stated acceptance criteria.
