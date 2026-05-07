# Mobile UX Follow-up Tasks

Tracking items from the UX review of the mobile keyboard toolbar and pane layout. Issues discovered during the `stapler-squad-mobile-elements-cut-off` branch session.

---

## Done

These were fixed in the current session.

- **Keyboard toggle buried two taps deep** — Moved the keyboard (⌨️) icon outside the `toolbarExpanded` gate so it is always visible regardless of toolbar state.

- **^D adjacent to ^C** — Reordered Row 3 to: `^C ^Z ^L ^R ^W ^U ^D`, separating the two destructive keys.

- **Tab touch targets ~28px** — Added `minHeight: 44px` to tab elements to meet WCAG 2.5.5 AA touch target size.

- **Sticky Ctrl/Alt not cleared by Row 3** — Each Row 3 key handler now calls `setCtrlActive(false)` and `setAltActive(false)` after sending its key.

- **Status text hidden on mobile (WCAG 1.4.1)** — Restored status text at `0.7rem` so it is visible on small viewports.

- **^C color contrast / visual salience** — Updated to `#ff6060` with a `boxShadow` ring for better contrast and distinctiveness.

- **Key font size 0.78rem** — Bumped to `0.8rem`.

---

## High Priority

### H1: Pane header hidden on mobile with no fallback for split-pane users

**Problem:** `PaneHeader` is hidden at the 768px breakpoint (Nielsen heuristic #3: User Control). When a user has configured a multi-pane layout on desktop and then opens the app on mobile, there is no way to switch panes, close panes, or change the active terminal tab. The user is stuck in whatever pane was last active.

**Files:**
- `web-app/src/components/pane/PaneHeader.tsx`
- `web-app/src/styles/pane/paneHeader.css.ts`
- `web-app/src/components/pane/MobilePaneTabStrip.tsx` (create or populate)

**Recommendation:** Choose one of two approaches:
1. Force single-pane mode when the viewport is below 768px (collapse splits on resize, restore on widen). Simpler, but loses multi-pane context.
2. Add a bottom-sheet or swipe-accessible pane switcher (`MobilePaneTabStrip`) that renders only on mobile and surfaces pane labels + close controls. Preserves layout intent.

---

### H2: SessionDetail header flexbox conflict on mobile

**Problem:** `headerActions` has `width: "100%"` in the mobile `flexWrap: wrap` media query (from the fullscreen override) but `flexWrap: "nowrap"` in the compressed header style. For long session names these rules interact and can produce broken layouts where buttons wrap unexpectedly or overflow their container.

**File:**
- `web-app/src/components/sessions/SessionDetail.css.ts`

**Recommendation:** Audit `fullscreenMobileHeaderActions` against the new mobile compressed header styles. Decide which wins at each breakpoint and make the two declarations mutually exclusive (e.g., scope `fullscreenMobileHeaderActions` to a `.fullscreen` parent class so it does not apply in normal mobile view).

---

## Medium Priority

### M1: Secondary toolbar actions duplicated in JSX

**Problem:** Lines ~1101–1141 (`secondaryGroup`) and ~1159–1199 (`mobileOverflowRow`) in `TerminalOutput.tsx` render the same five buttons: Mouse, Copy, Bottom, Resize, Clear. When one copy is updated (label, icon, handler, disabled state) the other is silently left stale. This has already caused drift and will do so again.

**File:**
- `web-app/src/components/sessions/TerminalOutput.tsx`

**Recommendation:** Extract the five button definitions into a shared `SECONDARY_ACTIONS` array (or a `<SecondaryToolbarButtons>` sub-component). Both `secondaryGroup` and `mobileOverflowRow` map over the same source. Eliminates the divergence risk entirely.

---

### M2: Horizontal toolbar overflow has no scroll indicator

**Problem:** `toolbarActions` and `actions` use `overflowX: auto` with hidden scrollbars, so content that extends off-screen is invisible and unreachable for users who do not know to swipe. There is no fade, gradient, or chevron to signal that more items exist.

**File:**
- `web-app/src/components/sessions/TerminalOutput.css.ts`

**Recommendation:** Add a right-edge `::after` gradient fade (`background: linear-gradient(to right, transparent, var(--terminal-background))`) that appears when the container is scrollable. Alternatively, ensure the "More ▾" button is always visible outside the scroll region so users can always reach overflow content regardless of scroll position.

---

### M3: Toolbar toggle icon is ambiguous

**Problem:** The ▼/▲ chevron toggle conveys direction but not meaning. Users unfamiliar with the toolbar may not know it opens additional tools.

**File:**
- `web-app/src/components/sessions/TerminalOutput.tsx`

**Recommendation:** Either add a "Tools" text label alongside the chevron, or replace ▼/▲ with "⋯" (ellipsis) which has an established convention for "more actions". Whichever is chosen, add an `aria-label` like `"Toggle toolbar"` for screen reader users.

---

## Low Priority / Polish

### L1: BottomNav sheet hardcodes pixel offset

**Problem:** `moreSheet` in `BottomNav.css.ts` uses `bottom: "64px"` instead of a token. If the bottom nav height ever changes, the sheet position must be updated manually in a separate file.

**File:**
- `web-app/src/components/layout/BottomNav.css.ts`

**Recommendation:** vanilla-extract `.css.ts` files cannot use `var()` strings directly at build time. Bridge via `globalStyle` or expose the height as a CSS custom property from the `BottomNav` component (`style={{ "--bottom-nav-height": "64px" } as React.CSSProperties}`) and reference `var(--bottom-nav-height, 64px)` in the sheet's `bottom` value via a `style` prop or dynamic style injection. Alternatively, define the `64` value as a shared constant imported by both files.

---

### L2: Keyboard visibility localStorage key is not scoped per session

**Problem:** The key `stapler-squad-mobile-keyboard-visible` is global across all sessions. A user who prefers the keyboard visible for a terminal session but hidden for a review session cannot have both preferences simultaneously.

**File:**
- `web-app/src/components/sessions/TerminalOutput.tsx`

**Recommendation:** Namespace the key by session ID: `stapler-squad-mobile-keyboard-visible-${sessionId}`. The `sessionId` is already in scope in `TerminalOutput`. This is a low-risk one-line change with meaningful UX improvement for multi-session workflows.

---

### L3: Undocumented breakpoint gap at 769–900px

**Problem:** `paneHeader` hides at 768px but `cockpitRoot` subtracts bottom nav height at 900px. The 769–900px range is in a liminal state: no pane header, but also no bottom nav height compensation. This may be intentional (tablets in portrait orientation have different layout goals) but it is not documented.

**Files:**
- `web-app/src/styles/pane/paneHeader.css.ts`
- `web-app/src/styles/layout.css.ts`

**Recommendation:** Add a comment block at each breakpoint explaining the design intent and the relationship between the two thresholds. If the gap is not intentional, align both to the same breakpoint (768px or 900px, whichever is correct for the layout model).
