# Validation: Fix Mobile Bell Dead Spot

## Adversarial Review

### Challenge 1: Does the CSS variable approach actually work in vanilla-extract?

Vanilla-extract CSS is statically extracted at build time; `var(--tray-handle-width, 0px)` is a valid CSS custom property reference in static CSS. Vanilla-extract supports this — the `var()` function is passed as a string literal and emitted verbatim into the generated CSS. No issue.

**Verdict: Valid.**

### Challenge 2: Race condition between TrayHandle mount and content render?

If the TrayHandle mounts after the session list has laid out, users might see a brief flash where the padding is 0 and then jumps to 44px. In practice TrayHandle mounts in the same React tree render that mounts the session list (both are rendered in `RootShell`/`Layout`), so the effect runs before the first paint. The CSS variable fallback is `0px`, which is correct before mount.

**Verdict: Acceptable. No visible flash in practice.**

### Challenge 3: Does the padding affect the BottomNav bell as well?

The BottomNav bell is a standard flex-item in the bottom bar, not `position: fixed` overlapping content. No dead zone from it. The CSS variable fix only targets the right-side `TrayHandle`. BottomNav items are at the bottom, addressed by `--bottom-nav-height` / scroll padding-bottom already in place.

**Verdict: Separate concerns, no overlap.**

### Challenge 4: What about other pages besides session list and backlog?

Other pages that could have overlapping content:
- Notifications page (`/notifications`) — full page, likely full-width. Needs checking.
- Unfinished page — similar.
- Review queue page — similar.

These pages may also need the same padding-right treatment if they have scrollable content that extends to the right edge. The fix should be applied at a higher layout level (the main content scroll container) rather than per-page, if possible.

**Verdict: Recommend also checking `web-app/src/app/globals.css` or the root layout scroll container to apply the fix globally to all page content.**

### Challenge 5: The existing `--tray-handle-inset` variable (60px default) vs proposed `--tray-handle-width` (44px)

`--tray-handle-inset` is used in `NotificationToast.css.ts` with a 60px fallback and is never explicitly set. It may have been a placeholder. The new `--tray-handle-width` variable is more precisely named for layout purposes (actual width = 44px). We should not conflict these two. The plan correctly uses a new variable name.

**Verdict: Use `--tray-handle-width`; don't rename or reuse `--tray-handle-inset`.**

### Challenge 6: Is 44px really the right value?

The TrayHandle CSS shows `width: 44px`. Its `minHeight: 88px` (height only, doesn't affect horizontal dead zone). The 44px is confirmed from the CSS.

**Verdict: 44px is correct.**

## Confirmed Gaps to Address Before Implementation

1. Audit all full-width page-level scroll containers (not just session list and backlog) and apply the same `paddingRight` fix.
2. Consider applying the CSS variable to a top-level content wrapper in the root layout so all pages inherit it, rather than patching per-page.
3. Confirm the existing `--tray-handle-inset` (60px) usage in `NotificationToast.css.ts` is correct and doesn't need to be updated to match the actual 44px value.

## Go/No-Go

**Go.** The root cause is confirmed (position:fixed 44px wide bell overlaps scrollable content at right edge), the fix is straightforward (CSS variable + padding-right), and the risk is low. The fix follows the established `--bottom-nav-height` pattern. Recommended to audit all full-width scroll containers as part of the same PR.
