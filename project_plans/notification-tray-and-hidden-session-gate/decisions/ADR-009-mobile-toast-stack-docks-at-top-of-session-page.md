# ADR-009: Mobile Toast Stack Docks at the Top of the Session Page; Desktop Stays Bottom-Right

**Status**: Accepted (operator decision, 2026-10-07); revised in plan repair iteration 3: the device checks gate enabling `notification_tray_v2`, not the PR 3 merge
**Date**: 2026-10-07
**Project**: notification-tray-and-hidden-session-gate
**Refines**: ADR-007 decision 3 (cap and chip). Supersedes the bottom-offset criterion of plan Story 3.7 for mobile session pages.

## Context

The two operator screenshots show bottom-anchored toasts covering the terminal input line and the page-keys row (keyboard closed), and the page-keys row already clipped with the keyboard open (`design/ux.md` section 0). Toasts here arrive unprompted, so they are not tied to a submit action. `NotificationToast.css.ts:33` anchors mobile toasts at `bottom: nav + --mobile-pane-tab-strip-height + 12px + safe-area` and never references `--keyboard-height`; `ViewportProvider.tsx:35-62` computes `--keyboard-height` and `--viewport-height` from `visualViewport` and flags `isVirtualKeyboardOpen` at more than 100px.

Evidence in `research/toast-placement.md`: bottom is the right default almost everywhere (section 2: Material, Spectrum; section 3: Sonner, VS Code) but Material's own rule is "bottom, never over controls", and the terminal input is a control (section 2). The closest real-world match is the Wikimedia mobile editing proposal to move toasts from bottom to top because bottom toasts block the composer and "the keyboard appears over the Toast" (section 4, Wikimedia T426191, a proposal, not a measured result). No library read there handles the soft keyboard for fixed bottom toasts (section 3).

## Decision

| State | Placement | Cards |
|---|---|---|
| Mobile portrait, keyboard closed | Top dock directly under the pane tab row, below the memory banner | 1 compact toast plus a "+N" chip |
| Mobile portrait, keyboard open | Same top anchor | 1-line chip only, 0 cards |
| Mobile landscape | Top dock under the tab row | chip or 1 compact card, max-height about 40% of `--viewport-height` |
| Desktop (>= 900px) | Bottom-right, 360px wide (stays above the status/footer area) | max 3 plus "+N more" |
| Mobile page with no terminal (no session page) | Bottom offset of plan Story 3.7 (nav + safe area), unchanged | cap as portrait |

1. New CSS custom property `--mobile-stack-top-offset`: the y coordinate (px) of the bottom edge of the session tab row. It is **measured, not computed**: the session layout publishes `getBoundingClientRect().bottom` of the tab row through a `ResizeObserver`, beside the existing `--mobile-pane-tab-strip-height` publisher (`web-app/src/components/pane/PaneSplitRenderer.tsx:442-449`). Measuring absorbs the variable-height, dismissible memory/fork-pressure banners (`ForkPressureStatusBanner` and `TmuxVersionMismatchBanner` are mounted in `web-app/src/app/layout.tsx:68-69`) without the stack knowing about them. The CSS is `top: max(var(--mobile-stack-top-offset, 0px), env(safe-area-inset-top, 0px)) + 8px`, so a missing publisher or a PWA/fullscreen cutout still lands the stack on screen (INFERRED, device check 5).
2. The bottom-offset criterion in Story 3.7 now applies to desktop and to mobile pages with no terminal only.
3. **Fallback** (not the plan): option B of `research/toast-placement.md` section 5, a bottom dock above the page-keys row, is used only if device check 1 shows that the top card hides terminal lines users need. Switching is a CSS and variant change; the data model, partition and chip logic do not change.
4. Pinned decisions keep exactly one card on mobile portrait; the rest count in the chip (`research/toast-placement.md` section 5, "Pinned vs routine"). This agrees with Radix's guidance that a response that must be obtained belongs in a persistent surface, with the toast as a pointer (section 3).

## Alternatives Considered

- Bottom dock above page-keys row (option B): rejected as primary; the anchor is fragile because the page-keys row is clipped when the keyboard is open and `--keyboard-height` lags on iOS (section 5 table).
- Edge chip only on mobile, no cards (option D): rejected; pinned approve/deny would lose its one-tap action.
- Keep plan 3.7 as written (option A): rejected; it is the current failure in the screenshots.

## Consequences

- Poor one-handed reach for the card. Mitigated by 44px targets, swipe plus button, and the fact that toasts are optional to act on (section 5).
- An OS heads-up (Android) renders above the tab row in screenshot 2 (ends y~400, tab row y~520) but a taller expanded heads-up could overlap briefly (device check 2).
- Story 3.7 gains Tasks 3.7c (publisher for the new var), 3.7d (this checklist), 3.7f (desktop overlap guard) and 3.7g (manual contrast check MC-1). Story 3.3/3.7 tests assert the top-dock bounding boxes (`design/ux.md` TD-2).

## Device-verification task list (plan Task 3.7d; Android Chrome and iOS Safari, browser tab and installed PWA)

Source: `research/toast-placement.md` section 6. **Who and when (adversarial N8)**: agents have no phones, so the checks cannot block the PR 3 merge. PR 3 merges behind the default-off `notification_tray_v2` flag; the operator runs DV-1..DV-6 on his own Android and iOS devices and records pass/fail and a screenshot per item in the PR 3 thread **before turning the flag on**. DV-1 failing switches to the documented bottom-dock fallback (a CSS/variant change) before the flag is enabled.

- [ ] **DV-1** Keyboard closed: the top card does not hide the first terminal lines the user is reading; chip and close targets (>= 44px) are reachable one-handed on a 6.7" phone. (Failing this triggers the fallback in Decision item 3.)
- [ ] **DV-2** Trigger an OS heads-up while a toast is showing: no unreachable overlap, and the toast is still dismissable afterwards.
- [ ] **DV-3** Keyboard open and a toast arrives: no blur on the xterm textarea, the chip stays above the keyboard, `--keyboard-height` has no stale value after dismiss (iOS `visualViewport` scroll/resize ordering).
- [ ] **DV-4** Pull-to-refresh and Chrome URL-bar collapse: the dock does not jump when the dynamic toolbar hides or shows (`position: fixed` against the tab-row var).
- [ ] **DV-5** Display cutout and landscape: `safe-area-inset-left/right` in landscape and `safe-area-inset-top` in the installed PWA; with the memory banner shown, dismissed, and wrapped to two lines, the stack always sits under the tab row.
- [ ] **DV-6** Swipe-to-dismiss on the top card triggers neither terminal gestures nor pull-to-refresh (plan Story 3.8, `design/ux.md` TC-4).

**Triad iteration 1 notes on the checklist** (no new DV numbers; DV-1..DV-6 keep their meaning):
- The stack **overlays** (it is `position: fixed`, no layout space): it covers the first terminal lines and the "Connected / Redraw / Hist" status row, and on Diff, VCS, Files, Logs and Info it covers the top of that tab. DV-1 also records whether the covered status row is needed while a toast is up and whether the pinned card's 8s auto-collapse to a chip (`design/ux.md` TD-14) is long enough.
- DV-3 is the real-device leg of TD-3, TK-1, TS-2 and RP-9: Playwright proves only the keyboard CSS-var path, so those criteria rest on DV-1..DV-6.
- Open bugs `docs/bugs/open/BUG-109-mobile-touch-drag-scroll-one-directional.md` and `BUG-110-mobile-terminal-blank-after-keyboard-open.md` (Android touch scroll, blank terminal after the keyboard opens; tracked in `project_plans/scrolling`) can look like toast or sheet defects. Record whether each is fixed with every DV result and triage a failure against them first.
- The manual contrast check of the toast, chip and handle over the xterm canvas (MC-1, plan Task 3.7g) is separate from DV-1..DV-6 because Axe cannot compute that composite.

## Evidence gaps carried forward

No controlled study of toast position against keyboard occlusion exists; the Wikimedia ticket is a proposal. The Material 3 and Apple HIG primary pages were not opened (`research/toast-placement.md` section 6). DV-1 to DV-6 are the verification that replaces them.
