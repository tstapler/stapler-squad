# ADR-006: Tray Is a Non-Modal, Layout-Neutral Overlay; Radix Dialog Only If a Spike Passes

**Status**: Proposed
**Date**: 2026-10-07
**Project**: notification-tray-and-hidden-session-gate

## Context

`NotificationPanel` (`web-app/src/components/ui/NotificationPanel.tsx`) is a
sibling of `<main>` in `web-app/src/app/layout.tsx:61-77`, `position: fixed`
with a transform slide, but renders `role="dialog" aria-modal="true"` with a
click-catcher backdrop (lines 145-153) and hard-coded z-index 9998/9999 in
`NotificationPanel.css.ts`. A modal dialog steals xterm focus and inerts the
terminal. Any layout change to the terminal container triggers
`ResizeObserver` -> `fit()` -> a PTY resize vote (`XtermTerminal.tsx`, #728/#731).
`useTerminalGestures.ts` registers non-passive document-level touch listeners.

## Decision

1. Evolve `NotificationPanel`; do not add a second tray component.
2. Non-modal on every breakpoint where the terminal is still visible: no
   backdrop dim, no `aria-modal`, no focus trap, no `body` overflow lock, only
   `transform`/`position: fixed`. Variants: desktop right-edge overlay with an
   edge handle; mobile portrait bottom sheet (peek/expanded) opened by a chip
   above the bottom nav and top-anchored full-height while the soft keyboard is
   open; mobile landscape right panel ~50vw. The expanded mobile sheet may be
   modal (terminal is occluded).
3. Focus primitive: `@radix-ui/react-dialog` with `modal={false}` **only if**
   Spike 1.2 shows it neither steals xterm focus nor mutates terminal layout.
   Otherwise hand-roll `role="complementary"` with explicit
   capture/restore of `document.activeElement` (restore to xterm's textarea).
4. Tray hotkey is bound in the capture phase and never reaches xterm.
5. Edge-drag-to-open is not offered on mobile (collides with the OS back
   gesture and `useWindowSwipe`'s 30px edge zone); open is by tap. Drag-to-close
   on the open sheet is allowed.
6. z-index uses `zIndex` tokens (`web-app/src/styles/theme-contract.css.ts`),
   replacing the 9998/9999 literals.

## Alternatives Considered

- Docked push-aside panel: rejected; resizes the terminal.
- vaul / react-modal-sheet: rejected (build-vs-buy.md; new dependency).
- Modal Radix dialog everywhere: rejected; steals terminal focus.

## Consequences

- Playwright asserts terminal `cols`/`rows` and DOM node identity unchanged
  across open/close (pattern at `tests/e2e/terminal-stress/tmux-roundtrip.spec.ts`).
- Axe must pass with the new roles; badge contrast and icon-only handle labels
  are part of the acceptance criteria.
