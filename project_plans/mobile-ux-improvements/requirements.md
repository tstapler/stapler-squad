# Requirements: Mobile UX Improvements

Status: Draft | Phase: 1 - Ideation complete
Created: 2026-04-07

## Problem Statement

Claude Squad's web UI is used from iPhone (iOS Safari) for managing and interacting with AI agent sessions. Several specific pain points make the experience frustrating enough to block real use:

1. **Terminal toolbar overflow** — The toolbar in `TerminalOutput` has 7+ buttons (Debug, Record, streaming mode selector w/ 140px min-width, Resize, Clear, Bottom, Copy) that overflow and clip on phone-width screens. There is no horizontal scroll — buttons are simply unreachable.
2. **Mobile supplemental keyboard is always-visible, not toggleable** — The mobile arrow/modifier key overlay (Esc, Tab, Ctrl+C, Ctrl+D, ←↑↓→) is shown permanently on `≤768px` via a media query. There is no toggle button to show/hide it.
3. **Virtual keyboard covers content** — On iOS Safari, `100vh` does not shrink when the virtual keyboard appears. The terminal or input field is hidden behind the keyboard with no scroll/reposition to compensate. The app does not use `dvh`, `visualViewport`, or `env(safe-area-inset-*)`.
4. **No safe-area handling** — No `env(safe-area-inset-bottom)` padding, so content is hidden behind the iPhone home indicator.

The goal is to fix these issues and establish patterns so future features are mobile-friendly by default.

## Success Criteria

- Terminal toolbar is fully accessible on mobile (all buttons reachable via horizontal scroll or overflow menu)
- Mobile supplemental keyboard has a visible toggle button; state persists across sessions in localStorage
- When the iOS virtual keyboard opens, the active content/input scrolls into view and is not obscured
- Safe-area insets applied so no content is cut off by notch or home indicator
- New mobile-aware CSS patterns documented/enforced so future components inherit good defaults

## Scope

### Must Have (MoSCoW)
- Horizontal scroll (`overflow-x: auto`) on the terminal toolbar's `.actions` div
- Toggle button for `.mobileKeyboard` in `TerminalOutput.tsx` (show/hide, state in localStorage)
- Keyboard-aware layout: use `dvh` or `visualViewport` API to resize modal/terminal when virtual keyboard opens
- `env(safe-area-inset-*)` padding in `globals.css` and key layout components

### Should Have
- Hide developer-only toolbar items (Debug, Record, streaming mode selector) on mobile by default, accessible behind a "More" overflow menu or developer-mode toggle
- Visual polish: mobile key buttons sized to `min-height: 44px` (already partially done)

### Out of Scope
- TUI (terminal UI) / desktop parity — desktop experience is already good
- Push notifications, PWA manifest, offline support
- Android-specific testing (iOS Safari is the primary target)
- Full mobile redesign / new layout
- Performance optimization (separate concern)

## Constraints

Tech stack: React (Next.js), CSS Modules, xterm.js for terminal
Platform target: iOS Safari (iPhone) — primary; iOS Chrome secondary
Timeline: Solo dev, fixes desired quickly — incremental PRs acceptable
Dependencies: xterm.js viewport handling, Next.js viewport metadata

## Context

### Existing Work
- Mobile keyboard overlay already exists and works (`TerminalOutput.tsx:652-666`, `TerminalOutput.module.css:225-293`)
- SessionDetail tabs already have `overflow-x: auto` on mobile (good pattern to replicate for toolbar)
- Header already has mobile hamburger menu pattern (good reference)
- `globals.css` already defines `--min-touch-target: 44px` CSS var (unused in toolbar)
- Modal is already a bottom sheet on mobile (`globals.css:229-240`)
- Viewport meta set: `initialScale: 1, maximumScale: 5` (layout.tsx)
- No `env(safe-area-inset-*)` usage anywhere in the codebase currently

### Code Locations for Changes
- `web-app/src/components/sessions/TerminalOutput.tsx` — toolbar + mobile keyboard component
- `web-app/src/components/sessions/TerminalOutput.module.css` — toolbar + mobile keyboard styles
- `web-app/src/app/page.module.css` — modal height calculations (100vh issue)
- `web-app/src/app/globals.css` — safe-area insets, root CSS vars
- `web-app/src/app/layout.tsx` — viewport meta

### Stakeholders
Solo developer/owner using the app daily from iPhone.

## Research Dimensions Needed

- [ ] Stack — iOS Safari viewport/keyboard behavior: `dvh`, `visualViewport` API, `env(safe-area-inset-*)`, 100vh behavior
- [ ] Features — existing patterns in the codebase for mobile-friendly toolbars/overflow menus
- [ ] Architecture — where to add keyboard-aware layout state (hook, context, or CSS-only approach)
- [ ] Pitfalls — iOS Safari known bugs: keyboard resize events, `dvh` support, `overscroll-behavior`, xterm.js + virtual keyboard
