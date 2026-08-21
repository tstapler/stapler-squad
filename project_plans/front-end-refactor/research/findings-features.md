# Findings: Features

**Date**: 2026-04-16
**Scope**: Mobile UX patterns for terminal, diff viewer, review queue, and foldable-screen layout
**Input**: `project_plans/front-end-refactor/requirements.md`, existing codebase audit, prior `mobile-ux-improvements` research

---

## Summary

Four feature areas need mobile-first redesign for the Pixel 9 Pro Fold (Android, Chromium-based). The key findings:

1. **xterm.js mobile touch**: Android Chrome handles keyboard input better than iOS Safari, but focus management, pinch-to-zoom prevention, and scroll conflict all require explicit work. No official mobile addon exists; the workarounds are well-understood.
2. **CodeMirror 6 vs Monaco on mobile**: CodeMirror 6 is the clear winner for mobile read-mostly diff viewing. Monaco's sandboxed worker architecture causes input lag and bundle weight on mobile; CodeMirror is built for the browser and has no native dependency on DOM measurements that break on small viewports.
3. **Foldable CSS media features**: The CSS `@media (horizontal-viewport-segments: 2)` API and `env(viewport-segment-*)` are available in Chromium 100+ (Android) but have limited real-world adoption and poor developer tooling. The Pixel 9 Pro Fold runs Chrome/Android WebView and has known quirks. The pragmatic path is breakpoint-driven layout switching with a foldable enhancement layer, not primary reliance on fold APIs.
4. **Review queue UX**: For time-sensitive approve/deny workflows on mobile, bottom-sheet with large tap targets consistently outperforms modal and inline-card patterns. Swipe gestures add discoverability risk on mobile web (no haptics, no native swipe affordance). The current `ReviewQueuePanel` uses small icon-only buttons (`✓` / `✗`) that are below the 44dp minimum — this is the highest-priority UX fix.

---

## Options Surveyed

### 1. xterm.js Mobile Touch

**Option A: No changes (current state)**
The current `XtermTerminal.tsx` has no touch-specific handling. On Android Chrome the xterm.js hidden textarea approach works for keyboard input (Android does not have iOS's visibility requirement), but:
- Pinch-to-zoom fires on the terminal, scrambling layout
- Scroll events conflict with xterm's built-in scroll handler
- Tapping the terminal sometimes requires two taps to focus

**Option B: touch-action + overscroll-behavior CSS (minimum viable)**
Add to the `.terminal` container:
```css
touch-action: pan-y;             /* allow vertical pan; block horizontal swipe + pinch zoom */
overscroll-behavior: contain;    /* prevent scroll chaining to page */
```
Block zoom on the terminal specifically, leaving zoom available on other app areas. This is a one-line CSS change with zero JS cost.

**Option C: Pointer event interceptor (full touch handling)**
Add a `pointerdown`/`pointermove`/`pointerup` layer on top of the terminal div to:
- Detect single-tap → focus terminal
- Detect two-finger pinch → adjust `terminal.options.fontSize` (font scaling instead of viewport zoom)
- Detect swipe → pass scroll to xterm's internal scroll (prevents conflict with browser scroll)

This mirrors what VSCode's mobile web approach does for terminal interaction. [TRAINING_ONLY — verify VSCode mobile web terminal specifics]

**Option D: Community touch addon**
No official mobile addon exists in the `@xterm` namespace as of xterm.js 6.0. The community has discussed an `@xterm/addon-touch` (GitHub issues #4507, #4892) but it has not shipped as of training data. [TRAINING_ONLY — verify current issue status]

The `@dreamonkey/xterm-js-touch` npm package provides basic touch support but is unmaintained (last publish 2021). [TRAINING_ONLY — verify package current status]

**Recommended**: Option B as immediate fix + Option C for font-scale pinch gesture in Phase 2.

---

### 2. CodeMirror 6 vs Monaco for Mobile Diff Viewer

**Current state**: `DiffViewer.tsx` uses a hand-rolled diff renderer (pure HTML/CSS, no editor library). `FileContentViewer.tsx` uses CodeMirror 6 for syntax highlighting. `@monaco-editor/react ^4.7.0` is in `package.json`.

**Option A: Keep hand-rolled diff renderer (current)**
The current `DiffViewer.tsx` renders unified diffs as custom HTML rows. Already mobile-friendly because it is pure HTML — no canvas, no Web Worker. The weakness is no syntax highlighting.

**Option B: CodeMirror 6 unified diff view via `@codemirror/merge`**
`@codemirror/merge` provides a diff/merge view built on top of CodeMirror 6. Key mobile advantages:
- No Web Worker requirement — computation runs on the main thread or is lazily scheduled
- DOM-based rendering — plays well with system font scaling and accessibility zoom
- Supports `touch-action: pan-y` for conflict-free scroll
- Same library family already in use (`@codemirror/view ^6.41.0`, `@codemirror/state ^6.6.0`)

The `@codemirror/merge` package is approximately 30 KB gzipped. [TRAINING_ONLY — verify current package size]

**Option C: Monaco Editor diff view**
`@monaco-editor/react ^4.7.0` is already installed. Monaco has a built-in `MonacoDiffEditor` component. However on Android Chrome:
- Monaco loads a Web Worker that does not have a clean shutdown on component unmount — memory leak risk during session switching
- The monaco editor container requires explicit pixel-height (not percentage/auto); this creates problems in flex/auto-height layouts
- Monaco's IntelliSense and language server features are wasted for a read-only diff view
- Bundle: `@monaco-editor/react` adds approximately 2.5 MB minified (~400 KB gzipped) [TRAINING_ONLY — verify current bundle size]
- On Android, Monaco's input handling has reported lag on low-end devices [TRAINING_ONLY — verify current status]

**Option D: Shiki static syntax-highlighted diff (server-side)**
Shiki (referenced in `FileContentViewer.tsx` language mapping, and a standard tool in the Next.js ecosystem) renders syntax-highlighted HTML at build or request time. For a read-only diff, Shiki + hand-rolled diff HTML gives the most mobile-optimal output: no JavaScript runtime, no canvas, no Web Worker. The downside is no interactivity (no collapsible hunks, no expand-context).

**Recommended**: Option B (`@codemirror/merge`) for the DiffViewer to add syntax highlighting while retaining mobile-friendly DOM rendering. Option D (Shiki static) if diff view usage is confirmed read-only and interactivity is not required.

---

### 3. Foldable Screen CSS (Pixel 9 Pro Fold)

**The fold APIs**: The CSS `@media (horizontal-viewport-segments: 2)` spec (formerly `spanning`) and `env(viewport-segment-top/right/bottom/left/width/height, 0, 0)` were proposed by Samsung/Microsoft and landed in Chromium 100+ [TRAINING_ONLY — verify exact Chromium version and current specification status]. The Pixel 9 Pro Fold runs Android 14+ with Chrome/Chromium — these APIs should be available.

**Pixel 9 Pro Fold specifics**:
- Inner display: approximately 7.6" OLED, 2208 x 1840, foldable
- Outer display: approximately 6.1" OLED, 2424 x 1080, cover screen
- The fold crease occupies a physical gap of approximately 2mm (software masked to approximately 6–8px)
- When unfolded, Android can present the browser with either a single large viewport or two logical segments depending on app windowing mode [TRAINING_ONLY — verify windowing behavior in Chrome on Pixel 9 Pro Fold]

**Option A: Ignore fold APIs, use breakpoints only**
Use existing `--breakpoint-md: 768px` and `--breakpoint-lg: 1024px` CSS vars. On the outer screen (6.1", portrait approximately 390px wide), the app acts like a standard phone. On the inner screen (7.6" unfolded, landscape approximately 900px wide), the app uses tablet-mode layout. This approach works today without any fold-specific APIs.

Weakness: does not avoid the fold crease. A two-column layout rendered across the crease is physically uncomfortable to read.

**Option B: `@media (horizontal-viewport-segments: 2)` enhancement**
When unfolded with the hinge vertical (book-like), the browser reports two horizontal viewport segments. Use this to avoid placing content over the fold:

```css
@media (horizontal-viewport-segments: 2) {
  .session-layout {
    display: grid;
    grid-template-columns:
      env(viewport-segment-width 0 0)
      env(viewport-segment-width 1 0);
    gap: calc(env(viewport-segment-left 1 0) - env(viewport-segment-right 0 0));
  }
}
```

This creates a natural list (left panel) + detail (right panel) layout that respects the physical crease gap.

**Option C: CSS container queries + fold query**
`@container (horizontal-viewport-segments: 2)` — container query version — is not yet widely supported. [TRAINING_ONLY — verify support status in Chrome Android]. Use media query version only for now.

**Recommended**: Option A as baseline (required for all viewports), Option B as progressive enhancement (crease avoidance). The fold media query is a nice-to-have; the app must work correctly without it.

---

### 4. Review Queue / Approval Workflow UX

**Current state**: `ReviewQueuePanel.tsx` renders items as a vertical list with small icon-only action buttons (`✓` / `✗`). On mobile these are below the 44dp minimum touch target. This is the highest-priority usability issue — users make time-sensitive approve/deny decisions for AI tool calls.

**Option A: Bottom sheet per item**
When a user taps an item in the queue list, slide up a bottom sheet with:
- Large approve/deny buttons spanning the full sheet width (minimum 64px height)
- Item context (tool call details, command preview)
- Swipe-down gesture to dismiss without deciding

Android Material Design 3 recommends bottom sheets for contextual actions on mobile. [TRAINING_ONLY — verify Material Design 3 bottom sheet spec]

Pros: maximum tap area, clear focus on single decision, natural dismissal gesture.
Cons: requires two taps per item (tap item → sheet opens → tap approve); slower for power users reviewing many items quickly.

**Option B: Swipe-to-decide inline card**
Implement swipe-right = approve, swipe-left = deny on each queue item card. On web mobile this requires custom touch gesture handling; there is no native CSS or browser API for this.

Pros: fastest UX for experienced users.
Cons: high implementation complexity; no visual affordance without native haptics; easy to swipe wrong direction under stress; poor accessibility (no keyboard or screen reader equivalent without additional implementation).

**Option C: Large inline buttons (immediate fix)**
Keep the inline item layout but replace icon-only buttons with full-width text buttons:
- "Approve" button: full card width, green background, 56px min-height
- "Deny" button: full card width, red background, 56px min-height
- Show buttons always (not collapsed)

Pros: simplest implementation; no extra interaction layer; excellent keyboard and screen reader support.
Cons: increased visual density; every item shows two large buttons even when not in focus.

**Option D: Sticky action bar for focused item**
Show approve/deny only for the currently-focused item (highlighted in the list). Keyboard navigation (j/k or arrow keys) moves focus. Focused item's approve/deny appear in a sticky bottom action bar. Mirrors how Gmail handles bulk email actions.

Pros: focused workflow, reduces per-item button clutter.
Cons: requires an explicit focus selection step that adds cognitive load, especially under time pressure.

**Recommended**: Option C (large inline buttons) for immediate implementation. The current `✓` / `✗` buttons are clearly broken on mobile and the fix is straightforward CSS. Option A (bottom sheet) is the right direction for a polished experience and should be the Phase 2 target once a shared BottomSheet primitive exists.

---

### 5. Session List + Cards (Touch-Friendly)

**Current state**: `SessionCard.tsx` is dense — multiple rows of metadata, actions hidden behind a toggle button ("Actions ▼"). The 44px touch target minimum is applied to action buttons on mobile but the "Actions" toggle itself is a small text button.

**Option A: Progressive disclosure via card expansion (improve current)**
Keep the current collapsed-by-default actions but make the expansion trigger a full-width 48px tap target. Use a trailing chevron icon. Replace emoji-text buttons with icon+text buttons that meet 44dp.

**Option B: Swipe-to-reveal actions**
Long-press or left-swipe reveals action buttons. Same accessibility concerns as Option B for review queue (swipe-to-decide).

**Option C: Contextual action sheet (long-press)**
Long-press a card opens a bottom sheet with all actions as large tap targets. iOS-style context menu pattern; Android long-press pattern. Pairs with the BottomSheet primitive.

**Recommended**: Option A for immediate work (minimal risk). Option C is better UX and should be planned alongside the BottomSheet primitive for the review queue.

---

### 6. Navigation Shell

**Current state**: `Navigation.tsx` is a top horizontal nav bar. `Header.tsx` implements a hamburger menu on mobile that opens a dropdown list. The current nav items are Sessions and Review Queue (two items).

**Bottom tab bar pattern**: Standard mobile app navigation for 2–5 top-level destinations. Thumb-reachable on both outer and inner screens. Works well for the current 2-item nav. Can expand to 4–5 items if settings/history/logs are promoted.

**Persistent sidebar (tablet/desktop)**: For the inner screen unfolded (approximately 900px wide), a persistent left sidebar with the session list creates a natural two-column master-detail layout. This eliminates the need to navigate back from session detail to session list.

**Recommended**: Add a bottom tab bar at `max-width: 768px`. Keep the top header at `min-width: 769px`. For the inner screen unfolded (treat as tablet at `min-width: 900px`), add a persistent sidebar with two-column layout. The fold media query (Option B above) can further refine this for the exact fold breakpoint.

---

## Trade-off Matrix

| Feature Area | Option | Touch Usability | Foldable Support | Integration Cost | Bundle Impact | Accessibility |
|---|---|---|---|---|---|---|
| xterm.js mobile | B: CSS-only | Medium | None | Very Low | Zero | No change needed |
| xterm.js mobile | C: Pointer interceptor | High | None | Medium | Zero | Needs ARIA update |
| Diff viewer | A: Keep hand-rolled | Medium (no syntax hl) | Medium | Zero | Zero | Excellent (DOM) |
| Diff viewer | B: CodeMirror merge | High | Medium | Low-Medium | +30 KB gz | Good (DOM) |
| Diff viewer | C: Monaco diff | Low (heavy, lag) | Low | Low (already in deps) | +400 KB gz | Medium |
| Diff viewer | D: Shiki static | High (read-only) | Medium | Medium (SSR setup) | Zero | Excellent |
| Foldable CSS | A: Breakpoints only | Medium | Low | Zero | Zero | Good |
| Foldable CSS | B: Segment media query | High | High | Low-Medium | Zero | Good |
| Review queue | C: Large inline buttons | High | Medium | Very Low | Zero | Excellent |
| Review queue | A: Bottom sheet | Very High | High | High (new primitive) | +5 KB gz | Good |
| Session cards | A: Expand toggle fix | Medium | Medium | Low | Zero | Good |
| Session cards | C: Action sheet | High | High | Medium (reuses BottomSheet) | Zero extra | Good |
| Navigation | Bottom tab bar | High | Low | Medium | Zero | Good |
| Navigation | Sidebar + 2-col | Very High | Very High | High | Zero | Good |

---

## Risk and Failure Modes

### R1: xterm.js focus on Android Chrome in split-screen mode
**Risk**: Android Chrome on Pixel 9 Pro Fold supports split-screen and floating window modes. In these modes the xterm.js hidden textarea focus behavior may differ from full-screen Chrome. Tap-to-focus may fail in floating window mode.
**Likelihood**: Medium
**Mitigation**: Include a visible (opacity: 0.01) overlay input element as a focus target. This pattern is already present in `VirtualKeyboard.tsx`.

### R2: Pinch-to-zoom scrambles terminal layout
**Risk**: Without `touch-action: pan-y` on the xterm container, pinch-to-zoom fires on the terminal and causes layout reflow. The terminal re-fits to the wrong dimensions.
**Likelihood**: High (currently unmitigated — no touch-action in `XtermTerminal.tsx`)
**Mitigation**: Add `touch-action: pan-y` to `.terminal` container. This is a one-line CSS fix.

### R3: `@media (horizontal-viewport-segments: 2)` not firing on Pixel 9 Pro Fold
**Risk**: Android's Chrome may not report the fold segments API correctly in all windowing modes (multi-window, taskbar mode in Android 14). [TRAINING_ONLY — verify]
**Likelihood**: Medium
**Mitigation**: Treat fold media query as progressive enhancement only. Breakpoint layout must work without fold APIs.

### R4: `@codemirror/merge` not in current deps
**Risk**: `@codemirror/merge` is absent from `web-app/package.json` (confirmed by inspection). Adding it requires a new install.
**Likelihood**: Certain
**Mitigation**: Trivial to install. `@codemirror/view` already present.

### R5: Bottom sheet component must be built from scratch
**Risk**: No reusable `BottomSheet` component exists in the codebase. Hand-rolling one requires correct handling of focus trap, scroll containment, drag-to-dismiss, and backdrop click.
**Likelihood**: Certain (must be built)
**Mitigation**: Use Radix UI `Dialog` primitive as the accessibility foundation (if Radix is adopted per the stack research). Focus trapping and keyboard dismiss are handled by the primitive.

### R6: Approve/deny fat-finger error on outer screen
**Risk**: On the 6.1" outer screen, if approve and deny buttons are placed adjacent, users will tap the wrong one during time-sensitive decisions.
**Likelihood**: High with current icon-only sizing
**Mitigation**: Separate approve and deny into distinct visual zones with minimum 16px gap. Consider requiring confirmation for deny (approve is lower-risk). Approve button on the right (primary action thumb zone on right-handed devices). [TRAINING_ONLY — verify Claude behavior when a tool call is denied to assess severity of wrong-tap]

### R7: WebGL context loss during fold/unfold
**Risk**: The Pixel 9 Pro Fold fold/unfold event triggers a near-simultaneous orientation change + window resize. This may cause the WebGL context to be lost. The existing `webglAddon.onContextLoss()` handler disposes the WebGL addon but does not re-initialize it — the terminal permanently falls back to canvas renderer.
**Likelihood**: Low-Medium
**Mitigation**: In the `onContextLoss` handler, attempt to re-create the WebGL addon after a short delay (100ms). If re-creation fails, accept the canvas fallback.

---

## Migration and Adoption Cost

| Work item | Estimated effort | Dependencies |
|---|---|---|
| xterm.js CSS touch fixes (Option B) | 0.5 day | None |
| xterm.js pointer interceptor for font scaling | 2 days | None |
| `@codemirror/merge` diff view | 2–3 days | `npm install @codemirror/merge` |
| Foldable CSS breakpoint enhancement | 1 day | None |
| Review queue large buttons (Option C immediate) | 0.5 day | None |
| Bottom sheet primitive component | 2–3 days | Radix UI Dialog/Sheet (if adopted) |
| Review queue bottom-sheet redesign (Phase 2) | 2 days | Bottom sheet primitive |
| Bottom tab bar navigation | 2 days | None |
| Session card contextual action sheet | 2 days | Bottom sheet primitive |
| Consolidate VirtualKeyboard implementations | 1 day | None |

**Critical path**: The BottomSheet primitive unlocks review queue + session card improvements. Build it early. The xterm.js CSS fix and review queue button resize are quick wins that should ship immediately.

---

## Operational Concerns

**Keyboard input latency**: The virtual keyboard on Android adds approximately 50–100ms input latency compared to a hardware keyboard. Keep all input-path callbacks direct — no `setTimeout` on the `onData` path in `XtermTerminal.tsx`.

**Memory pressure on foldable**: The Pixel 9 Pro Fold has 12 GB RAM but Chrome on Android applies aggressive tab/process limits. xterm.js with WebGL addon can use 200–400 MB for large scrollback buffers. The terminal instance pool (ADR-001 from terminal-jank) is required — do not instantiate terminals for off-screen sessions.

**Monaco footprint**: If Monaco is retained for the file viewer, lazy-load it via `next/dynamic`. Verify `next/bundle-analyzer` output after refactor. Monaco should never be in the initial bundle.

**Screen orientation changes during fold/unfold**: The `ResizeObserver` in `XtermTerminal.tsx` handles resize correctly (the double `requestAnimationFrame` pattern is already present). However, folding/unfolding the device triggers a near-simultaneous orientation change + window resize. Test specifically for double-fit race conditions and verify the terminal reaches stable dimensions.

---

## Prior Art and Lessons Learned

**Prior `mobile-ux-improvements` research (in this repo)**
`project_plans/mobile-ux-improvements/research/findings-features.md` (dated 2026-04-07) documents mobile patterns already in the codebase. Key findings carried forward:
- `touch-action: manipulation` and `onPointerDown` + `e.preventDefault()` are used in `VirtualKeyboard.tsx` and `TerminalOutput.tsx`. These are the correct patterns for mobile.
- The `--min-touch-target: 44px` CSS var is defined but hardcoded as `44px` in components. The refactor should switch all hardcoded values to `var(--min-touch-target)`.
- The `ViewportProvider` (ADR-001) correctly handles `visualViewport` resize + scroll events for Android Chrome.

**VSCode web terminal on mobile**: VSCode's web-based terminal uses a custom mobile keyboard overlay (Ctrl+C, Tab, Esc, arrow keys) that floats above the terminal. This is conceptually identical to the existing `VirtualKeyboard.tsx` / `TerminalOutput.tsx` inline keyboard. The key difference: VSCode shows the overlay automatically on mobile viewport detection rather than requiring user toggle. [TRAINING_ONLY — verify VSCode web mobile behavior]

**GitHub Mobile review workflow**: GitHub's mobile web and native app review patterns use a sticky footer bar for primary approval actions. Inline diff comments use bottom sheets. The "sticky footer with primary action" pattern maps directly to the review queue.

**Linear (project management app) mobile**: Linear's mobile web app uses a bottom sheet for all contextual actions on issues. Long-press on a card opens the sheet. This is the correct model for the session card actions.

**Terminal app iPad keyboard accessory view pattern**: Several terminal emulators on iPadOS use a two-row keyboard accessory view (extra keys above the system keyboard) for Esc, Tab, Ctrl+C. This is the same concept as the existing `mobileKeyboard` component in `TerminalOutput.tsx`. The key insight from these apps is that the accessory view should remain visible while the system keyboard is open, positioned just above it. [TRAINING_ONLY — verify Android Chrome equivalent for positioning the accessory bar above the system keyboard]

---

## Open Questions

1. Does `@media (horizontal-viewport-segments: 2)` fire correctly on the Pixel 9 Pro Fold in Chrome when the device is unfolded? The spec implementation may vary by Android version or Chrome version. Requires physical device testing.

2. What happens to the xterm.js WebGL context during fold/unfold? Is the context loss recoverable or does the terminal permanently fall back to canvas renderer?

3. Should `VirtualKeyboard.tsx` replace or supplement the inline `mobileKeyboard` div in `TerminalOutput.tsx`? Two separate implementations exist. The refactor should consolidate to one component with consistent toggle behavior.

4. Is `@codemirror/merge` production-ready for large diffs (100+ files, 10,000+ lines)? Profile with realistic diff sizes before committing to this approach.

5. Does `navigator.virtualKeyboard` (Chrome 94+) work on Chrome for Android on the Pixel 9 Pro Fold? If yes, `navigator.virtualKeyboard.overlaysContent = true` would simplify the terminal keyboard-avoidance logic significantly compared to the current `visualViewport` approach. [TRAINING_ONLY — verify support on Android Chrome]

6. What is the correct outer-screen breakpoint? The current `--breakpoint-md: 768px` falls between the outer screen width (approximately 390px portrait) and the inner screen width (approximately 900px landscape). A `600px` breakpoint may better capture "outer screen = phone mode, inner screen = tablet mode". Needs measurement on device.

---

## Recommendation

**Immediate actions (low effort, high impact)**:
1. Add `touch-action: pan-y` and `overscroll-behavior: contain` to the xterm.js container CSS. Fixes the most visible mobile terminal issue in under an hour.
2. Resize the review queue approve/deny buttons to full-width with `min-height: 56px`. This is the highest-priority safety/UX issue on mobile (time-sensitive decisions with undersized targets).
3. Consolidate the two mobile keyboard overlay implementations (`VirtualKeyboard.tsx` and the `mobileKeyboard` div in `TerminalOutput.tsx`) into a single component controlled by the ADR-004 toggle state.

**Short-term (1–2 weeks)**:
4. Add `@codemirror/merge` for syntax-highlighted diff view in `DiffViewer.tsx`. Low-risk (same library family) and significantly improves the diff review experience on mobile.
5. Add a bottom tab bar for the primary navigation at `max-width: 768px`.
6. Add the fold media query CSS for a session list + session detail two-column layout at approximately `min-width: 900px` (inner screen unfolded).

**Medium-term (design system phase)**:
7. Build a `BottomSheet` primitive using Radix UI. Use it to redesign the review queue and session card actions.
8. Add a pointer-event interceptor on the xterm.js container to implement pinch-to-font-size gesture.

Do **not** use Monaco for the diff view. Keep Monaco installed for the file viewer (where it is justified), but route all diff rendering through the hand-rolled renderer upgraded with `@codemirror/merge`.

---

## Pending Web Searches

The following queries should be run by the parent agent to verify training-only claims and fill gaps:

1. `xterm.js mobile touch Android Chrome 2024 focus keyboard` — verify current state of xterm.js Android support and any new addons or workarounds
2. `xterm.js addon-touch npm site:github.com OR site:npmjs.com 2024 2025` — verify if an official or community touch addon exists
3. `"horizontal-viewport-segments" OR "viewport-segment" CSS media query Chrome Android Pixel Fold support 2024` — verify fold API support status on Pixel 9 Pro Fold
4. `"navigator.virtualKeyboard" Chrome Android support 2024 site:developer.chrome.com OR site:caniuse.com` — verify if Chrome on Android supports the Virtual Keyboard API
5. `"@codemirror/merge" bundle size performance large diff 2024` — verify performance profile and bundle size
6. `Monaco Editor Android Chrome input lag performance mobile 2024` — verify current state of Monaco on Android Chrome
7. `"screen-fold-angle" OR "fold-left" CSS env function deprecated 2024` — verify if original fold env() functions are relevant or superseded by viewport-segments
8. `review queue approve deny mobile UX bottom sheet patterns 2024 site:nngroup.com OR site:smashingmagazine.com OR site:uxdesign.cc` — validate bottom-sheet recommendation with UX research
