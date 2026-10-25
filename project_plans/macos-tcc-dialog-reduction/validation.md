# Validation: macOS TCC Dialog Reduction

Adversarial review of the plan against the requirements and research findings.

---

## Finding 1 — `open -b` unconditionally launches: pre-flight check is load-bearing, not optional

**Risk level:** HIGH

The plan states the pre-flight CGO running check is part of Phase 1.2. The risk register acknowledges `open -b` launches apps if not running. But the plan's Phase 1.2 does not specify what happens when the CGO check is unavailable (nocgo build, older macOS, or CI). The nocgo fallback uses `lsappinfo` — but `lsappinfo` is a private Apple utility with no documented API stability guarantee. If `lsappinfo` is ever removed or changes output format, the nocgo path silently skips the running check and calls `open -b` unconditionally, launching arbitrary installed apps on the user's machine when they click Focus Window on a stale notification.

**Recommendation:** The nocgo path should not fall through to `open -b` if the running check fails. Instead: return `Success=false, Message="App may not be running; cannot confirm"`. This is safer than a spurious app launch. The running check must be mandatory, not best-effort.

---

## Finding 2 — `appName`-only path still calls `osascript`

**Risk level:** HIGH

The plan explicitly leaves the `appName`-only code path (no bundle ID in the request) on `osascript`. The requirements doc's AC 1 states: "After the fix, no TCC dialogs from Apple Events appear when the user clicks Focus Window." AC 1 is written as unconditional. If any production notification populates `source_bundle` correctly but `source_app` only (no bundle ID), the `appName` path fires and the TCC dialog still appears.

**Assessment:** The `appName` path is exercised today (research found it as a fallback). Leaving it on `osascript` is an incomplete fix.

**Recommendation:** Either (a) remove the `appName`-only path entirely (return `Success=false, Message="Bundle ID required for Focus Window"`) and update the frontend to ensure it always sends a bundle ID; or (b) implement a `lsappinfo` / `NSWorkspace` bundle-ID lookup by app name so the `appName` path can also use `open -b`. Option (a) is simpler and safe — research showed `source_bundle` is always set alongside `source_app` in the notification hook.

---

## Finding 3 — Bundle ID blocklist is fragile; allowlist is safer

**Risk level:** MEDIUM

The plan proposes a blocklist of known-bad `com.apple.*` apps. The pitfalls research explicitly warns: "A blocklist can't be exhaustive." New Apple apps could be added to the OS that are similarly alarming. A blocklist requires ongoing maintenance.

**Alternative that was recommended in build-vs-buy.md but not fully adopted:** Allowlist by known-safe domain patterns (`com.microsoft.*`, `dev.zed.*`, `dev.warp.*`, `com.jetbrains.*`, `com.panic.*`, `com.sublimetext.*`, `com.barebones.*`, `com.apple.Terminal`, `com.apple.dt.Xcode`, `com.apple.Safari`, `com.apple.finder`, `com.googlecode.iterm2`, `net.kovidgoyal.kitty`, `com.github.wez.wezterm`, `org.gnu.Emacs`) with an explicit hard-coded exact-match list for opaque Electron IDs.

**Assessment:** The plan is inconsistent with the recommendation from `build-vs-buy.md` (section 6) which recommended "allowlist by known-safe domains." The current plan uses a blocklist, which is weaker.

**Recommendation:** Implement a hybrid: allowlist of known-good domain patterns (handles the common case) + a separate exact-match set for opaque IDs. Unknown bundle IDs not matching the allowlist return `Success=false` with a logged message. This is more maintainable than a blocklist and provides defense-in-depth against novel system apps.

---

## Finding 4 — Codesign status detection is fragile

**Risk level:** MEDIUM

Phase 2.1 proposes detecting codesign status by running `codesign -dv <self>` and checking for the string `"StaplerSquadDev"`. This breaks if:
- The user uses a different cert name (a legitimate scenario: enterprise installs, custom setups).
- The `codesign` command is slow or absent in the test environment.
- The string changes in a future macOS version of codesign output.

**Better approach:** Check whether the binary has a valid `CFBundleIdentifier` embedded (readable via `infoplist_darwin.go`'s already-embedded plist) and whether the `com.stapler-squad` bundle ID resolves in the running process's `NSBundle.mainBundle`. This works without shelling out to `codesign`. Alternatively: check `SecCodeCheckValidity` via CGO (available in Security.framework, no external process).

**Recommendation:** Don't parse `codesign -dv` output. Instead, expose a Go constant or function `macos.IsCodesigned() bool` that returns true iff the embedded bundle ID is `com.stapler-squad` (verifiable at process start from the plist). This is already built — `infoplist_darwin.go` embeds it.

---

## Finding 5 — Phase ordering puts UX before the backend is testable

**Risk level:** LOW

The plan phases 3 and 4 (frontend) depend on Phase 2 (codesign status RPC). But Phase 2 depends on Phase 1 completing (to know whether Apple Events are still in play). The sequencing diagram shows Phase 2 can run in parallel with Phase 1, which is correct. However, the frontend (Phase 3) should not be merged before Phase 1 is in production — otherwise the dimmed button is displayed to users but the backend still uses `osascript`, meaning the button state is accurate but the fix is not live.

**Recommendation:** Gate the Phase 3 frontend changes behind a feature flag (`focus-window-v2`) so the UX can ship after Phase 1 is verified in production. Remove the flag when both Phase 1 and Phase 2 are stable.

---

## Finding 6 — No rollback plan for the `open -b` change

**Risk level:** LOW

If `open -b` causes regressions (e.g. unexpected app launches, wrong space switching behavior), there is no fast rollback path. The plan does not mention a feature flag or a server-side config toggle.

**Recommendation:** Wrap the activator selection in a server config option (`focus_window_use_launch_services`, default `true`) that falls back to the existing `osascript` path. This enables per-instance rollback without a binary redeploy. Remove the config once the new path has been stable for 30+ days.

---

## Finding 7 — Missing test for the CGO running-check path on macOS 14 Sonoma

**Risk level:** LOW

The pitfalls research explicitly noted that `NSRunningApplication.activateWithOptions:` is deprecated on macOS 14 and may silently fail from background agents. The plan uses `open -b` (not `activateWithOptions:`) which avoids this, but the pre-flight CGO running check using `runningApplicationsWithBundleIdentifier:` must also be verified on macOS 14.

The plan's testing strategy mentions a manual darwin test but does not specify the macOS version. Given this is a macOS 14 regression, the manual test should be run on macOS 14 (Sonoma) specifically.

**Recommendation:** Add an explicit "Test on macOS 14" requirement to the manual verification checklist in Phase 1.2.

---

## Summary Table

| Finding | Risk | Required? |
|---------|------|-----------|
| 1. nocgo running-check fallback must not silently skip | HIGH | Must fix before shipping |
| 2. appName-only path still uses osascript | HIGH | Must fix or explicitly accept scope limit |
| 3. Blocklist weaker than allowlist | MEDIUM | Should upgrade to hybrid allowlist |
| 4. Codesign detection via codesign subprocess fragile | MEDIUM | Should use embedded plist / constant |
| 5. Frontend should be feature-flagged until backend stable | LOW | Recommended |
| 6. No rollback path for open -b | LOW | Recommended |
| 7. macOS 14 testing not specified | LOW | Add to checklist |

Findings 1 and 2 are blocking — the plan as written does not fully satisfy AC 1 (no TCC dialogs) if the `appName` path remains on `osascript`, and the nocgo running-check fallback has a silent failure mode that can cause spurious app launches.
