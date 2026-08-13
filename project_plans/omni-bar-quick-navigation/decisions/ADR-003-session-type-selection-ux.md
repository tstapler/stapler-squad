# ADR-003: Session Type Selection UX — Arrow Keys, Not Tab Cycling

**Date**: 2026-04-21
**Status**: Accepted

## Context

The requirements specify "Tab within creation mode cycles through session type options (new worktree → directory → existing worktree)." The pitfalls research flagged this as **[CRITICAL]** for accessibility:

- `Tab` is the browser's universal focus-traversal key
- Screen readers expect `Tab` to move focus between focusable elements, not cycle values
- Overriding `Tab` for non-focus-movement violates WCAG 2.1 Level AA (Success Criterion 2.1.2: No Keyboard Trap)
- Users who rely on keyboard navigation would find `Tab` broken or unpredictable

## Decision

**Session type selection uses a radio group widget with arrow key (↑↓) navigation, not Tab cycling.**

The widget renders as a horizontal or vertical group of labeled buttons. Keyboard interaction:
- `Tab` moves focus *to* the session type group (entering from the search input)
- `↑`/`↓` (or `←`/`→`) cycle between "New Worktree", "Directory", "Existing Worktree" while focus is on the group
- `Tab` from the last option in the group moves focus *to* the branch input (exiting)
- `Shift+Tab` moves focus backward

This follows the ARIA "radio group" pattern (`role="radiogroup"` with `role="radio"` children).

## ARIA Pattern

```tsx
<div role="radiogroup" aria-label="Session type" onKeyDown={handleTypeKeyDown}>
  {SESSION_TYPES.map((type) => (
    <button
      key={type.value}
      role="radio"
      aria-checked={sessionType === type.value}
      tabIndex={sessionType === type.value ? 0 : -1}
      onClick={() => setSessionType(type.value)}
    >
      {type.label}
    </button>
  ))}
</div>
```

Arrow key handler: only `↑`/`↓`/`←`/`→` cycle within the group. `Tab` is not intercepted.

## Rationale

- WCAG compliance: radio group pattern is well-established and accessible
- Screen reader users get proper announcements: "New Worktree, radio button, 1 of 3"
- `Tab` retains standard focus-movement semantics throughout the form
- Arrow cycling is expected behavior inside radio groups (per ARIA Authoring Practices Guide)

## Consequences

- Implementation requires a small radio group component or inline keyboard handler — more code than intercepting Tab, but correct and accessible.
- The "Tab to pick session type" phrasing in the requirements doc is incorrect; must be updated to "arrow keys select session type" in all user-facing copy.
