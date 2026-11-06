# Toast placement: mobile vs desktop on the session page

Question: where should the in-app toast stack sit on mobile vs desktop, given the session page is a terminal whose input line is at the bottom? Operator's prior: bottom; open to top-on-mobile + bottom-on-desktop if evidence supports it.

Labels: SOURCED (URL read or returned by search this session) vs PRIOR-ART (from memory, not re-verified). Thin spots are named in section 6.

## 1. Recommendation (summary)

| State | Placement | Cards |
|---|---|---|
| Mobile portrait, keyboard closed | Top dock, directly under the pane tab row (below memory banner) | max 1 compact toast + "+N" chip |
| Mobile portrait, keyboard open | Same top anchor, 1-line chip only | 0 cards |
| Mobile landscape | Top dock under tab row, chip or 1 compact toast, max-height <= 40% of `--viewport-height` | 0-1 |
| Desktop (>= 900px) | Bottom-right, 360-400px wide, above the status/footer area | max 3 + "+N more" |

Bottom is the right default for desktop and for mobile pages with no composer. It is wrong for the mobile session page because the composer (xterm input line, page-keys row, keyboard) occupies exactly the bottom region, and toasts here arrive unprompted (not after a user action).

## 2. Platform guidance

- Material snackbar: sits "toward the bottom of the screen", temporary, non-blocking. SOURCED (search result quoting https://material.io/components/snackbars; the m3.material.io page failed to fetch via read_website). The MD3 bottom-app-bar rule says snackbars move up above the bottom app bar. SOURCED (search summary, not opened at primary). Reasoning stated by MD is "don't interrupt / don't cover key controls" (the Firefox draft says never cover FAB/nav/bottom bar: https://acorn.firefox.com/latest/mobile/components/snackbar/android-pNOUdFi9). So MD's own rule is "bottom, but never over controls". The terminal input is a control.
- Adobe Spectrum: toast defaults to bottom center on desktop and mobile; mobile toasts go above bottom navigation. SOURCED (https://spectrum.adobe.com/page/toast, via search).
- Sainsbury's: pick one position and keep it consistent. SOURCED (https://design-systems.sainsburys.co.uk/components/toast/).
- Android heads-up notification: high-priority banner at the top of the screen overlaying the app for a few seconds. SOURCED (https://help.moengage.com/hc/en-us/articles/34998473340948; AOSP automotive variant https://source.android.com/docs/automotive/hmi/notifications/hun is not the phone OS). Operator's screenshot 2 shows it covering y~175-400 of 2000, i.e. above the tab row (tab row at y~520-570).
- iOS banners at the top, swipe up to dismiss. SOURCED (secondary only: https://jaredsinclair.com/2014/06/24/nitpicking-ios-notification-bann.html). Apple HIG wording on placement was not found. Windows toast bottom-right, macOS banner top-right: PRIOR-ART.
- Thumb zone: bottom easiest, top hardest. SOURCED but weak research base (https://parachutedesign.ca/blog/thumb-zone-ux/, https://mockflow.com/glossary/Thumb-reachability). A toast is feedback that rarely needs a tap, so reach matters less than occlusion (search synthesis; no controlled study of toast position found).

## 3. Libraries

- Sonner: default `bottom-right`, `visibleToasts` 3, offset 32px desktop / `mobileOffset` 16px under 600px; positions include top-center. SOURCED (https://sonner.emilkowal.ski/toaster). Its CSS goes full-width at `max-width: 600px` and anchors with `bottom: var(--mobile-offset-bottom)` on `position: fixed`, with no keyboard or visualViewport handling. SOURCED (https://raw.githubusercontent.com/emilkowalski/sonner/main/src/styles.css).
- react-hot-toast: `top-center` default, `reverseOrder` option. SOURCED (https://react-hot-toast.com/docs/toaster).
- react-toastify: playground default `top-right`, `limit` option, `newestOnTop` "play nice with bottom toast". SOURCED (https://fkhadra.github.io/react-toastify/introduction/).
- Mantine: six positions, `limit` + queue, `priority`, `layout="stacked"` (tap to expand on touch). SOURCED (https://mantine.dev/x/notifications/). Priority + limit mirrors our pinned-vs-routine model.
- Radix Toast: viewport is a fixed area you place; F8 hotkey; says actions must be safe to ignore, and responses that must be obtained should be an AlertDialog styled as a toast. SOURCED (https://www.radix-ui.com/primitives/docs/components/toast). Relevant to pinned approval toasts: they are decisions, so they belong in a persistent surface (tray) with the toast only as a pointer.
- No library read here handles the soft keyboard for fixed bottom toasts. Chakra/MUI Snackbar: PRIOR-ART only (MUI bottom-left desktop, bottom-center mobile; no keyboard handling). Unverified.

## 4. Products with a bottom composer

- Wikimedia mobile editing: moving toasts from bottom to top while editing because bottom toasts block "next suggestion", obscure buttons, and "the keyboard appears over the Toast". SOURCED (https://phabricator.wikimedia.org/T426191, proposed, "below the edit toolbar"). Closest real-world match to this app: same problem, same fix as ux.md D1.
- Wikimedia, earlier: the iOS keyboard covers bottom toasts, and `vh`/`dvh` ignore the keyboard while visualViewport updates are not reliable enough to position them; they accepted the risk because toasts followed submit actions. SOURCED (search synthesis of a Wikimedia Phabricator task; task id not captured). Our toasts are not tied to submit actions, so that tradeoff does not carry over.
- VS Code: toasts bottom-right; bell in the bottom-right status bar opens the notification center; Do Not Disturb exists there. SOURCED (secondary: https://harman.helpjuice.com/en_US/visual-studio-code-notifications). Desktop precedent for bottom-right plus a persistent tray handle.
- Slack/Discord mobile in-app banners at top; GitHub mobile, Linear, Termius/Blink/JuiceSSH: PRIOR-ART, not verified. I did not find how mobile terminals avoid the issue.

## 5. Tradeoffs and options

Facts from this repo:
- `ViewportProvider.tsx` computes `--keyboard-height` and `--viewport-height` from visualViewport and flags `isVirtualKeyboardOpen` at > 100px (`web-app/src/components/providers/ViewportProvider.tsx:35-62`). Verified by reading.
- `NotificationToast.css.ts` anchors mobile toasts at `bottom: nav + --mobile-pane-tab-strip-height + 12px + safe-area` and never references `--keyboard-height` (`web-app/src/components/ui/NotificationToast.css.ts:30-34`). That bottom is computed against the layout viewport. Whether it lands over the input line in the keyboard-closed screenshot is consistent with screenshot 1 (cards cover input and page-keys), so this is the current failure.
- Screenshot 2: with the keyboard open the terminal is ~35% of the screen and the page-keys row is already clipped, so "above the page-keys row" has no stable anchor.

| Option | Covers input? | One-handed dismiss | OS heads-up / URL bar | Cost |
|---|---|---|---|---|
| A. Bottom stack (current, plan 3.7 as written) | Yes in screenshot 1; keyboard hides it or it covers input when open | Best | None | Low, but already failing |
| B. Bottom dock above page-keys row / keyboard | No when closed; with keyboard open anchor is fragile (clipped row, `--keyboard-height` timing on iOS) | Good | None | Medium: needs measured row height var, keyboard rAF lag |
| C. Top dock under tab row, 1 card + chip (primary) | No: sits over read-only output | Poor reach; mitigated by swipe + chip tap at 44px, and toasts are optional-to-act | Heads-up is above the tab row in screenshot 2 (ends y~400 vs tab row y~520); a taller expanded heads-up could overlap briefly. URL bar is outside the page viewport in a browser tab; pull-to-refresh only at scroll top and the dock is not the scroll surface | Low-medium: position from tab-row bottom (new CSS var, parallel to `--mobile-pane-tab-strip-height`), safe-area-inset-top only matters in PWA/fullscreen |
| D. Edge chip only on mobile, no cards | No | Good if chip at the edge | None | Low, but pinned decisions (approve/deny) lose one-tap action |

Pinned vs routine: routine items already do not toast (ux.md section 3). Pinned decisions are the only ones needing a card; keep exactly one on mobile portrait, rest in the chip count.

Per state:
- Portrait, keyboard closed: C. Fallback: B, only if a device test shows the top card hides something users need (e.g. first output lines).
- Portrait, keyboard open: chip only at the top anchor (matches ux.md D1/TD-3). Do not render cards; the visible terminal is ~35% of screen.
- Landscape: viewport height is the scarce axis; chip or 1 compact card, capped at 40% of `--viewport-height`. Top-right avoids the input line.
- Desktop: bottom-right matches Sonner/VS Code/Fluent convention; keyboard is not a factor; cap 3. If it covers terminal output on a short pane, shift offset above the pane footer or fall back to top-right.

Where this agrees with the operator: bottom is the correct default almost everywhere and desktop stays bottom-right. Where it departs: only on the mobile session page, where Wikimedia's evidence (the one directly matching case) and our own screenshots point to top.

## 6. Evidence gaps and device checks

Thin evidence:
- No controlled study of toast position vs keyboard occlusion; the Wikimedia ticket is a proposal, not a measured result.
- Primary pages for MD3 placement and Apple HIG banner placement were not opened (fetch failed or search-only).
- Slack/Discord/GitHub mobile, Termius/Blink/JuiceSSH, MUI/Chakra behavior is PRIOR-ART only.
- Whether Android's expanded heads-up overlaps the dock was not tested.

Verify on a real device (Android Chrome and iOS Safari):
1. Keyboard closed: top card does not hide the first terminal lines the user is reading; chip and close targets >= 44px reachable one-handed (thumb stretch on a 6.7" phone).
2. Trigger an OS heads-up while a toast is showing: confirm no unreachable overlap and that the toast is still dismissable afterwards.
3. Keyboard open and a toast arrives: no blur on the xterm textarea, chip stays above the keyboard, `--keyboard-height` has no stale value after dismiss (iOS visualViewport scroll/resize ordering).
4. Pull-to-refresh and Chrome URL-bar collapse: the dock position does not jump when the dynamic toolbar hides/shows (use `position: fixed` against the tab-row var, test in browser tab and installed PWA).
5. Display cutout / landscape: `safe-area-inset-left/right` in landscape; `safe-area-inset-top` in PWA.
6. Swipe-to-dismiss on the top card does not trigger terminal gestures or pull-to-refresh (plan 3.8 TC-4).

Plan impact (not edited here): Story 3.7's bottom-offset acceptance criterion applies to desktop and non-session mobile pages only; ux.md D1 already specifies the top dock for session pages.
