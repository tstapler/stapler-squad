# Requirements: Jump to Session — Modal Mode on Mobile

## Source
Backlog item `7d579e1e-96cf-4790-8365-272a6982399b`
> "The modal version of Jump to Work Session sucks. It should probably just open the session in a modal."

Screenshot shows mobile view (`onyx.staplerhome.internal:8444`): the "Jump to Work Session" panel expands **inline** inside the backlog item detail page, constrained to `min(70vh, 420px)`. With the virtual keyboard open, the terminal is too cramped to be usable.

## Problem Statement
`JumpToSessionPanel` expands the `SessionDetail` component (full terminal + tabs) inline in the backlog item detail page. On desktop this is fine — the panel fills `min(60vh, 520px)` of page height. On mobile (`< 600px`) the page already has significant stacked content, and the inline panel gets squeezed to `min(70vh, 420px)` while competing with the virtual keyboard and the page scroll context, making terminal interaction impractical.

An existing `SessionPeekModal` component already renders `SessionDetail` in a `position: fixed` overlay occupying 94vw × 94vh of viewport. This modal form factor is far better for mobile terminal interaction: it is unaffected by page scroll, it anchors to the visual viewport, and it gives the terminal the maximum available space.

## Goals
1. On **mobile** (`isMobile` from `useViewport()`, i.e. viewport width < 600 px), clicking "Jump to `{role}` session" opens `SessionPeekModal` instead of expanding an inline panel.
2. On **desktop/foldable** (≥ 600 px), the existing inline-expand behavior is preserved unchanged.
3. The modal on mobile shows the session title and a close button (already `SessionPeekModal`'s header).
4. The role-selector caret menu continues to work on mobile so the operator can pick a different session role before the modal opens.
5. Analytics event `backlog_jump_to_session` fires on modal open (same as the current inline-expand path).
6. `SessionPeekModal.css.ts` is updated so the modal uses full-screen (`inset: 0`, no border-radius) on viewports < 600 px, improving the mobile experience for all callers.

## Out of Scope
- Changing the desktop inline-expand behavior.
- Adding new session roles or filters.
- Changing the `SessionPeekModal` semantics for its other caller (the session-list row peek).
