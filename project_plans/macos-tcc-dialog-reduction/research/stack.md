# macOS Stack Research: TCC Dialog Reduction

## 1. Existing darwin-specific Go patterns

The codebase uses two distinct patterns for darwin-specific code:

### a) `//go:build darwin && cgo` — CGO with C headers (libproc, plist embedding)

`session/procinfo/openfiles_darwin.go` (build tag `darwin && cgo`) uses classic CGO:
```go
/*
#include <libproc.h>
#include <string.h>
*/
import "C"
```
Calls `C.proc_pidinfo` / `C.proc_pidfdinfo` directly. The `//go:build darwin && !cgo` sibling
(`openfiles_darwin_nocgo.go`) provides a fallback that shells out to `lsof` when CGO is
unavailable. This paired pattern is the established project idiom for optional CGO.

`infoplist_darwin.go` (build tag `darwin && cgo`) is the minimal CGO stub that triggers the
linker to embed the Info.plist:
```go
//go:build darwin && cgo
package main
import "C"
```
The actual embed happens via `CGO_LDFLAGS="-sectcreate __TEXT __info_plist $(CURDIR)/macos/Info.plist"` in the Makefile.

### b) `//go:build darwin` — `purego` / dlopen (no CGO required)

`internal/fsevents/stream_darwin.go` (build tag `darwin`, no cgo tag) uses
[`github.com/ebitengine/purego`](https://github.com/ebitengine/purego) to call CoreFoundation
and CoreServices at runtime via `purego.Dlopen` / `purego.RegisterLibFunc`. No CGO required —
it's pure Go with dlopen at runtime. This is the project's preferred pattern for calling macOS
frameworks when CGO overhead or cross-compilation concerns apply.

Other darwin-only files use only the stdlib (`executor/managed_process_darwin.go` — pure
`syscall` use, no CGO; `server/services/gatestats_liveness_darwin.go`;
`session/tmux/socket_owner_darwin.go`; `session/unfinished/treewatch_darwin.go`).

## 2. Current CGO setup for darwin

- **Go version:** `go 1.26.6` (module `github.com/tstapler/stapler-squad`)
- **CGO_ENABLED:** `1` (set in Makefile, required for go-sqlite3)
- **CGO_CFLAGS:** `-Wno-ignored-qualifiers` (macOS) / `-Wno-discarded-qualifiers -Wno-ignored-qualifiers` (Linux)
- **Info.plist embedding:** `CGO_LDFLAGS="-sectcreate __TEXT __info_plist $(CURDIR)/macos/Info.plist"` on darwin builds
- **Plist content** (`macos/Info.plist`): declares `NSAppleEventsUsageDescription`
- **Entitlements** (`entitlements.plist`): `com.apple.security.automation.apple-events = true`

The build verifies embedding succeeded:
```makefile
CGO_LDFLAGS="-sectcreate __TEXT __info_plist $(CURDIR)/macos/Info.plist" \
    go build -ldflags "$(LDFLAGS)" -o stapler-squad .
```

## 3. Go version and darwin build constraints used

- Go 1.26.6
- Build tags used: `darwin`, `darwin && cgo`, `darwin && !cgo`
- No use of `//go:build darwin && arm64` or specific arch constraints found in the codebase

## 4. Calling NSRunningApplication / NSWorkspace from Go: options and patterns

### Option A: `purego` + Objective-C runtime (preferred, no CGO required)

The existing `stream_darwin.go` pattern uses `purego.Dlopen` on system frameworks.
`NSRunningApplication` and `NSWorkspace` live in `AppKit.framework`. The Objective-C runtime
(`/usr/lib/libobjc.dylib`) exposes `objc_msgSend` which purego can call:

```go
// Load AppKit
appkit, _ := purego.Dlopen("/System/Library/Frameworks/AppKit.framework/AppKit", purego.RTLD_NOW|purego.RTLD_GLOBAL)
objc, _ := purego.Dlopen("/usr/lib/libobjc.dylib", purego.RTLD_NOW|purego.RTLD_GLOBAL)

var objcMsgSend func(id uintptr, sel uintptr, args ...uintptr) uintptr
purego.RegisterLibFunc(&objcMsgSend, objc, "objc_msgSend")
```

`NSRunningApplication.runningApplicationsWithBundleIdentifier:` returns running instances.
`NSRunningApplication.activate(options:)` activates without Apple Events.
`NSWorkspace.shared.frontmostApplication` / `activateFileViewerSelectingURLs:` are also accessible.

This avoids all Apple Events TCC. The TCC category `NSRunningApplication.activate` falls under
**Accessibility** (if using `NSApplicationActivateIgnoringOtherApps`) or may require no TCC
at all for the `activationPolicy = regular` path introduced in macOS 14 (`NSApplicationActivationOptions`).

### Option B: `#cgo` with Objective-C / AppKit headers

Use `//go:build darwin && cgo` + `import "C"` with `#import <AppKit/AppKit.h>` and a C bridge
function. Heavier but more straightforward to write:
```c
// bridge.m (or inline in the cgo comment)
#import <AppKit/AppKit.h>
void activateByBundleID(const char *bundleID) {
    NSString *bid = [NSString stringWithUTF8String:bundleID];
    NSArray *apps = [NSRunningApplication runningApplicationsWithBundleIdentifier:bid];
    [[apps firstObject] activateWithOptions:NSApplicationActivateIgnoringOtherApps];
}
```
Requires `#cgo LDFLAGS: -framework AppKit` or adding `-framework AppKit` to `CGO_LDFLAGS`.

### Option C: `open -b <bundle_id>` shell command

`open -b com.example.app` launches or brings to front the named app. On macOS, `open(1)` uses
`NSWorkspace.shared.open(configuration:)` / Launch Services internally — it does **not** send
Apple Events. It does not require per-app TCC consent. The downside: it cannot target a
*specific window* within the app, and it may launch the app if it is not already running (add
`-g` to avoid raising it to front without bringing it forward; no clean "activate only if
already running" flag). For the FocusWindow use case (bring a running app to the foreground),
`open -b <id>` is a viable fallback that avoids all TCC, but it lacks fine-grained control.

**TCC comparison:**
| Method | TCC category | Per-app prompt? |
|---|---|---|
| `osascript "tell application id X to activate"` | Apple Events (`com.apple.security.automation.apple-events`) | Yes, once per target app |
| `NSRunningApplication.activate(options:)` via AppKit | Accessibility (macOS 12–13) or none (macOS 14+) | No per-app prompt |
| `open -b <bundle_id>` | None | No |

## 5. `open -b <bundle_id>` vs `osascript activate`

- `osascript "tell application id X to activate"` — sends an Apple Event to the target app's
  process. Apple Events are an IPC mechanism; each *target* app must be in the sender's TCC
  Apple Events allowlist. This generates a dialog per unique target app the first time.
- `open -b <bundle_id>` — invokes Launch Services to bring the app forward. Uses no Apple
  Events. Requires no TCC consent at all. Limitation: cannot target a specific window.
- `NSRunningApplication.activate` (AppKit direct call) — calls the app's activation directly via
  the window server without Apple Events. On macOS 14+, `NSApplicationActivationOptions` no
  longer requires Accessibility. On macOS 12–13, `NSApplicationActivateIgnoringOtherApps`
  may require Accessibility TCC (one-time global grant for stapler-squad, not per-target-app).

## 6. All darwin-specific Go files in the codebase

| File | Build tag | Technique |
|---|---|---|
| `infoplist_darwin.go` | `darwin && cgo` | CGO stub for Info.plist linker embed |
| `session/procinfo/openfiles_darwin.go` | `darwin && cgo` | CGO + `libproc.h` direct syscall |
| `session/procinfo/openfiles_darwin_nocgo.go` | `darwin && !cgo` | Fallback shell-out to `lsof` |
| `internal/fsevents/stream_darwin.go` | `darwin` | `purego` / dlopen (CoreFoundation + CoreServices) |
| `internal/fsevents/spike_darwin_test.go` | `darwin` | test |
| `internal/fsevents/stream_darwin_test.go` | `darwin` | test |
| `executor/managed_process_darwin.go` | `darwin` | `syscall.SysProcAttr` platform diff |
| `executor/managed_process_darwin_test.go` | `darwin` | test |
| `server/services/gatestats_liveness_darwin.go` | `darwin` | platform-specific liveness check |
| `session/tmux/socket_owner_darwin.go` | `darwin` | socket ownership check |
| `session/tmux/socket_owner_darwin_test.go` | `darwin` | test |
| `session/unfinished/treewatch_darwin.go` | `darwin` | fsevents integration |

## Key conclusions

1. **Preferred pattern for AppKit calls: `purego`** — `stream_darwin.go` already demonstrates
   loading system frameworks without CGO. The same technique can load AppKit and call
   `NSRunningApplication` / `objc_msgSend` without any new CGO dependency.

2. **CGO with Objective-C bridge** is also fully supported — `openfiles_darwin.go` shows the
   paired `darwin && cgo` / `darwin && !cgo` pattern. Adding `-framework AppKit` to `CGO_LDFLAGS`
   (already set in the Makefile) is the only plumbing change needed.

3. **`open -b` is TCC-free** and can serve as a safe fallback when AppKit activation is
   unavailable, at the cost of losing window-level focus control.
