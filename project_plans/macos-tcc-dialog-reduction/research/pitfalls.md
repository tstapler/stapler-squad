# Pitfalls: macOS TCC Dialog Reduction

Research for replacing `osascript "tell application id X to activate"` with
`NSRunningApplication.activateWithOptions:` (CGO) or `open -b`. Context: this
project already uses CGO for macOS syscalls (`session/procinfo/openfiles_darwin.go`),
so adding more CGO is an established pattern.

---

## 1. NSRunningApplication.activateWithOptions: — TCC and Accessibility

**Does it require TCC?** No — `NSRunningApplication.activate(options:)` does not
trigger a `kTCCServiceAppleEvents` prompt and does not appear in System Settings →
Privacy & Security → Automation. It is a public AppKit API that operates at the
window-server level. No Accessibility TCC grant is required.

**The real TCC risk:** If your CGO code also calls `AXUIElement*` (Accessibility)
APIs — e.g. to confirm the window is frontmost — that triggers
`kTCCServiceAccessibility`. The implementation must stay with `NSRunningApplication`
only and not mix in any `AX*` calls.

**Pitfall — `ignoringOtherApps` silently ignored on macOS 14+ (Sonoma):**
Apple deprecated `NSApplicationActivateIgnoringOtherApps` in macOS 14 (WWDC23
session 10054). On macOS 14+, `activate(options: [.activateIgnoringOtherApps])`
may be silently ignored for apps that are on a different Space. The "winning"
pattern for macOS 14 is:

```objc
[[NSRunningApplication runningApplicationsWithBundleIdentifier:@"com.example.App"]
    .firstObject activateWithOptions:NSApplicationActivateIgnoringOtherApps];
```

works on 13 and below. On 14+, it returns `YES` from the call but the window may
not come forward. The fix is to use the new `NSApp.activate(ignoringOtherApps:)`
pattern OR fall back to `open -b` for cross-Space cases (see §2). Detecting this
silently-stale return is hard — `activateWithOptions:` returns `BOOL YES` even when
the activation was rejected.

**Pitfall — calling app must not be in background on macOS 14:**
On macOS 14+, `NSRunningApplication.activate(options:)` called from a background
agent (no `LSUIElement=true` + no visible windows, or an `LSBackgroundOnly` agent)
may silently no-op. `stapler-squad` runs as a LaunchAgent without a UI window, which
puts it in the "background agent" category. The workaround is to set
`NSApplicationActivationPolicy` to `.regular` in the embedded plist at build time
— but this causes the Dock icon to appear, which is also undesirable. The pragmatic
path: fall back to `open -b` (see §2) for cross-Space and macOS-14-background-agent
cases; keep the NSRunningApplication path for the common same-Space case.

**Pitfall — calling from a goroutine (not main thread):**
AppKit requires all UI operations on the main thread. CGO calling AppKit from a
goroutine (which Go maps to an OS thread, not necessarily the main thread) will
cause a crash or silent failure. The CGO wrapper must dispatch onto the main thread
via `dispatch_async(dispatch_get_main_thread(), ...)`. This pattern exists in the
macOS Swift/ObjC community but adds complexity to the CGO layer.

---

## 2. `open -b bundle_id` Behavior

**Does it launch if not running?** Yes, unconditionally. `open -b com.example.App`
with no path argument launches the app if it is not already running. There is no
"only activate, don't launch" flag for `open`. To avoid spurious launches, the
caller must check `NSRunningApplication.runningApplicationsWithBundleIdentifier:`
first and only call `open -b` when the app is actually running. A CGO check before
shelling out is the cleanest pattern.

**Different Space:** `open` switches the user to the Space where the app's window is
— this is the expected behavior and works even in cases where
`NSRunningApplication.activate` fails silently on macOS 14+. However, the Space
switch is jarring if the user is in full-screen mode or has many Spaces. There is no
flag to suppress the Space switch.

**No TCC required:** `open` uses Launch Services and does not trigger any
`kTCCService*` prompt. It is the safest fallback for the cross-Space and
macOS-14-background-agent cases.

**Timing:** `open` is fire-and-forget; it exits before the app is in the foreground.
The existing code already treats activation as best-effort (`log.Warn` on failure,
returns success to the caller), so this is acceptable.

**Bundle ID not found:** `open -b com.nonexistent.App` exits 1 with
`"LSOpenURLsWithRole() failed with error -10814"`. This must be caught and logged,
not surfaced as a user-visible error — same as the current AppleScript path's
`"Application isn't running"` handling (`utility_service.go:374–380`).

---

## 3. CGO Cross-Compilation Pitfalls (darwin arm64 vs amd64)

**The project already uses CGO on darwin:** `session/procinfo/openfiles_darwin.go`
sets the pattern — `//go:build darwin && cgo`, `import "C"`, C headers in the
preamble comment. The new window-focus file would follow identical structure.

**Cross-compilation to the other arch is unsupported without a cross-compiler:**
`GOARCH=amd64 go build` on an arm64 machine with CGO requires either Rosetta 2 or
`CC=x86_64-apple-darwin-clang`. The project does not cross-compile today — the
Makefile's `build-embedded` target has no `GOARCH` override. Adding CGO for
NSRunningApplication keeps this constraint unchanged; it does not introduce a new
limitation.

**Framework linker flags are required:**
```go
// #cgo LDFLAGS: -framework AppKit -framework Foundation
// #include <AppKit/AppKit.h>
```
Forgetting `-framework AppKit` causes `ld: framework not found AppKit` at link time
(not compile time). The `otool -L` assertion in the Makefile's `build-embedded` step
does not check for framework linkage, so this failure mode can be silent during
development if `go test` is run without the CGO build tag.

**`//go:build darwin && cgo` vs `//go:build darwin`:**
A file without the `cgo` build constraint will be compiled when `CGO_ENABLED=0`,
causing a compile failure on the `import "C"` line. The nocgo fallback file
(`openfiles_darwin_nocgo.go`) pattern in `session/procinfo/` must be replicated:
create `window_darwin_nocgo.go` with `//go:build darwin && !cgo` that returns
`ErrNotSupported` or similar.

**`otool` plist-embed assertion:**
The Makefile's `otool -l` check (line 188–190) guards against silent
`CGO_ENABLED=0` builds. If a CI step sets `CGO_ENABLED=0` (e.g. `make lint` at
line 866 explicitly does), the window-focus CGO code will not compile there — this
is expected and acceptable, since `make lint` runs `golangci-lint` with
`CGO_ENABLED=0` for speed. The `make test` and `make build` paths set
`CGO_ENABLED=1` (Makefile line 14).

---

## 4. TCC Database: Denied Prompts

**A deny is permanent for that app+target pair:**
If a user clicks "Don't Allow" on a TCC dialog, macOS records `auth_value=2`
(DENY) in `~/Library/Application Support/com.apple.TCC/TCC.db`. On macOS 13+
(Ventura), the OS does NOT re-show the dialog for subsequent requests — it silently
denies. The only recovery paths are:

- User manually enables in System Settings → Privacy & Security → Automation
- `tccutil reset AppleEvents com.stapler-squad` (requires confirmation or sudo)
- `sudo tccutil reset AppleEvents` (resets ALL apps — dangerous)

**The NSRunningApplication path avoids this entirely** because it does not register
under `kTCCServiceAppleEvents` at all. The osascript replacement removes the denial
risk completely.

**Denying the replacement one-time TCC grant:**
If the plan includes a one-time Accessibility grant (for fallback paths), a deny
there is also permanent until manually reset. The UI must explain the consequence
before the prompt appears ("we need Accessibility to bring your editor window to
front; you can revoke this later in System Settings").

**TCC.db direct reads require Full Disk Access:**
The diagnostic `sqlite3 ~/Library/Application Support/com.apple.TCC/TCC.db`
command in `macos-codesigning.md` requires FDA for the terminal. The programmatic
check `tccutil` does not — use it in tests/debugging rather than sqlite3.

---

## 5. Code Signing: Expiration and Missing Certs

**Self-signed cert expiration:**
The `StaplerSquadDev` cert created by `make setup-codesign` has a validity period
set in `scripts/setup-codesign.sh`. If it expires, subsequent `make install-service`
runs produce a valid binary (codesign signs successfully with the expired cert on
macOS, which does not check self-signed cert expiry for local signing), but the TCC
`csreq` field still matches the cert's SHA-1 anchor hash — so TCC grants survive
expiry. The practical risk is low. However, if the cert is deleted from Keychain
(user mistake or OS upgrade wiped the keychain), `make install-service` fails with
`errSecInternalComponent` or `no identity found`, and the binary ships unsigned.
An unsigned binary gets a `cdhash`-based DR, which breaks TCC persistence.

**CI builds without the cert:**
CI (GitHub Actions) does not have `StaplerSquadDev` in its keychain. The `build`
target in the Makefile does not call `codesign`. The `install-service` target does.
This is already handled — CI uses `make build` + `make test`, not `make install-service`.
A CI build of the NSRunningApplication CGO layer (no codesign) is safe because the
CI binary is never used for TCC grants.

**Lost cert → all existing TCC grants break:**
If a developer deletes the cert and regenerates it, the new cert has a different
SHA-1. All existing TCC grants in TCC.db still reference the old SHA-1 anchor, so
they stop matching. The user sees all permission dialogs again. `make setup-codesign`
is idempotent and refuses to overwrite — so this only happens if the cert is deleted
first. Document this explicitly in the codesigning how-to.

---

## 6. Bundle ID Allowlist: Coverage Gaps

**The allowlist approach can't be exhaustive:**
Common editors and the non-obvious bundle IDs that would need to be covered:

| App | Bundle ID | Pitfall |
|-----|-----------|---------|
| VS Code | `com.microsoft.VSCode` | Also `com.microsoft.VSCodeInsiders` for Insiders builds |
| Zed | `dev.zed.Zed` | `dev.*` domain — not `com.*`, easily missed |
| Warp | `dev.warp.Warp-Stable` | Also `dev.warp.Warp-Development` |
| Cursor | `com.todesktop.230313mzl4w4u92` | Electron app; opaque bundle ID, changes on major update |
| Windsurf | `com.todesktop.241128sscvq29g4` | Same: opaque ToDesktop bundle ID |
| IntelliJ CE | `com.jetbrains.intellij.ce` | Also `com.jetbrains.intellij` for Ultimate |
| Nova | `com.panic.Nova` | |
| Sublime Text | `com.sublimetext.4` | `com.sublimetext.3` still common |
| BBEdit | `com.barebones.bbedit` | |
| Emacs (Mac Port) | `org.gnu.Emacs` | Also `dev.gnu.emacs` for some builds |

**Electron apps (Cursor, Windsurf, Linear) use opaque reverse-domain IDs** like
`com.todesktop.*` that change across major versions. A client sending `source_bundle`
from one of these will look invalid to a domain-pattern allowlist.

**Recommended approach:** allowlist by known-safe domains (`com.apple.*` **except**
the media apps, `com.microsoft.*`, `dev.zed.*`, `dev.warp.*`, `com.jetbrains.*`,
`com.panic.*`, `com.sublimetext.*`, etc.) and a small hard-coded exact-match list
for the opaque ones. Log bundle IDs that pass activation so real-world data can grow
the list.

**`com.apple.*` is not uniformly safe:**
The intent is to block `com.apple.Music`, `com.apple.TV`, `com.apple.Photos` (all
trigger alarming media-library dialogs). Safe Apple apps include `com.apple.Terminal`,
`com.apple.dt.Xcode`, `com.apple.Safari`, `com.apple.finder`. An allowlist that
blanket-allows `com.apple.*` would admit the media apps. The filter must be an
explicit blocklist of known-bad `com.apple.*` bundle IDs, not a prefix allowlist.

**`source_bundle` vs. the bundle ID the user intends to focus:**
The `source_bundle` in notification events captures what was frontmost when the
session was created — not necessarily what the user wants to focus. An editor session
created while Music was in the foreground gets `source_bundle = com.apple.Music`.
The server-side sanitization must reject known-bad bundle IDs even when they appear
in the focus request, returning a clear error rather than attempting activation.

---

## 7. macOS Version Compatibility (macOS 13/14/15)

**macOS 13 Ventura:** `NSRunningApplication.activateWithOptions:` with
`.activateIgnoringOtherApps` works as expected from a background process. No behavior
change from macOS 12.

**macOS 14 Sonoma:** Significant change. Apple's stated intent is that only the
foreground app should be able to steal focus. `activateWithOptions:` called from a
background LaunchAgent may silently succeed (returns `YES`) but the targeted app's
window does not come to front. The affected scenario is exactly stapler-squad's
use case: a LaunchAgent (no UI, `LSUIElement=1` or similar in Info.plist) calling
`activate` on a running editor. Testing on macOS 14 is required before shipping.

Workaround for macOS 14 background-agent case: call `open -b <bundleID>` as fallback
when `NSRunningApplication.activate` returns `YES` but the app is not subsequently
frontmost (check `[[NSRunningApplication
runningApplicationsWithBundleIdentifier:bundleID].firstObject isActive]` 100ms
after activation). This adds latency but is observable and correct.

**macOS 15 Sequoia:** No new TCC categories affecting window activation were
announced in WWDC24. The macOS 14 `activateIgnoringOtherApps` deprecation carries
forward. Apple's Human Interface Guidelines recommend letting users control focus,
so further restrictions are possible in future OS releases. The `open -b` path is
the most forward-compatible option since it uses Launch Services rather than AppKit
internals.

**API availability guard:**
`NSRunningApplication.activate(options:)` is available macOS 10.6+. No version
guard is needed. The deprecated overload `activateWithOptions:` (ObjC) vs the Swift
`activate(options:)` — the CGO layer uses ObjC, so `activateWithOptions:` is correct
and available. The availability warning on `NSApplicationActivateIgnoringOtherApps`
on macOS 14+ will produce a compile warning unless suppressed with
`#pragma clang diagnostic ignored "-Wdeprecated-declarations"`. Add this pragma
in the CGO preamble to keep CI clean; document the deprecation with a comment
pointing at the `open -b` fallback path.

---

## Summary

- **NSRunningApplication avoids TCC dialogs entirely** but silently fails on macOS
  14+ for background-agent callers (LaunchAgents without a foreground window).
  `open -b` is the reliable fallback for cross-Space and macOS-14 cases, but it
  unconditionally launches the app if not running — a pre-flight running-check via
  CGO is required before calling `open`.
- **Bundle ID filtering must block specific bad `com.apple.*` IDs** (Music, TV,
  Photos), not use a prefix allowlist; electron editors like Cursor use opaque
  `com.todesktop.*` IDs that a domain-pattern approach misses; log real-world
  bundle IDs to grow the allowlist over time.
- **CGO requires main-thread dispatch, a nocgo fallback file, and explicit framework
  linker flags**; cross-compilation constraints are unchanged from the existing
  `openfiles_darwin.go` precedent; a denied TCC prompt for any replacement one-time
  permission (e.g. Accessibility) is permanent until the user resets it manually,
  so the UI must explain the permission before the OS dialog appears.
