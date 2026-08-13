# ADR-002: Keyboard Shortcut for "Open in Creation Mode"

**Date**: 2026-04-21
**Status**: Accepted

## Context

The requirements specify a shortcut to open the omnibar directly in creation mode (bypassing discovery). The existing `Cmd+K` shortcut opens the omnibar in discovery mode. The user preference is to stay in the `Cmd+K` family — consistent with VS Code, Linear, and other tools that use `Cmd+K` as their command palette trigger.

`Cmd+N` was the original candidate but is captured by browsers as "New Window" before React can intercept it. `Cmd+Shift+N` is "New Incognito Window" — also captured. `Alt+N` breaks the `Cmd+K` convention.

## Decision

**Use `Cmd+Shift+K` as the shortcut for "open omnibar in creation mode."**

| Shortcut | Action |
|---|---|
| `Cmd+K` | Open omnibar (existing — unchanged) |
| `Cmd+Shift+K` | Open omnibar directly in creation mode |

`Cmd+Shift+K` is not captured by Chrome, Firefox, or Safari on macOS, Windows, or Linux in the main browser window. It stays in the same key family as `Cmd+K`, making it easy to remember.

Secondary paths (always available, no shortcut needed):
- `new/` prefix in the search input forces creation mode automatically
- Clicking the "Create" side of the mode badge switches modes

## Implementation

```typescript
// OmnibarContext.tsx — global keydown listener (alongside existing Cmd+K handler)
if (e.metaKey && e.shiftKey && e.key === "K") {
  e.preventDefault();
  e.stopPropagation();
  openInCreationMode();
  return;
}
```

The listener is attached at document level (same as the existing `Cmd+K` listener).

## Rationale

- `Cmd+K` is already established as the omnibar trigger — users know it
- `Cmd+Shift+K` extends the pattern naturally (Shift = "enhanced" or "create" variant)
- Stays consistent with the app's existing keyboard convention
- No browser conflict on macOS/Windows/Linux
- Mode badge tooltip shows: `Cmd+K` for Jump, `Cmd+Shift+K` for Create

## Consequences

- Both shortcuts use the same `K` key — easy to discover by accident and easy to learn.
- `Cmd+Shift+K` on some platforms may conflict with IDE shortcuts (e.g., JetBrains uses it for comment/uncomment). Since stapler-squad runs in the browser, IDE shortcuts only fire when the IDE has focus — no actual conflict.
- The mode badge tooltip makes the shortcut discoverable without documentation.
