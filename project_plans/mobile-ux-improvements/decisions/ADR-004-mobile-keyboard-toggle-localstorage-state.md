# ADR-004: Mobile Keyboard Toggle — localStorage State

**Status:** Accepted
**Date:** 2026-04-08
**Context:** Mobile UX Improvements — mobile supplemental keyboard visibility toggle

## Context

The mobile supplemental keyboard overlay (Esc, Tab, Ctrl+C, arrows) is currently always-visible on ≤768px via a CSS media query. The requirement is to add a show/hide toggle that persists across sessions.

Three persistence approaches were evaluated:

1. **Session-only state (`useState` only)** — Resets to visible on every page load. Not acceptable for a daily-driver tool.
2. **localStorage** — Persists across browser sessions on the same device. Simple, no server dependency.
3. **Server-side session state** — New RPC endpoint + Go backend storage. Massively over-engineered for a single UI preference. Out of scope.

## Decision

Use **localStorage** with key `stapler-squad-mobile-keyboard-visible`.

- Default: `true` (visible) — backward compatible
- Read on component mount via `useEffect` (avoids SSR hydration mismatch)
- Write on toggle
- Value: `"true"` | `"false"` (string)

```typescript
const [isKeyboardVisible, setIsKeyboardVisible] = useState(true)

useEffect(() => {
  const stored = localStorage.getItem('stapler-squad-mobile-keyboard-visible')
  if (stored !== null) setIsKeyboardVisible(stored === 'true')
}, [])

const toggleKeyboard = () => {
  const next = !isKeyboardVisible
  setIsKeyboardVisible(next)
  localStorage.setItem('stapler-squad-mobile-keyboard-visible', String(next))
}
```

Toggle button follows the hamburger menu pattern: `aria-expanded`, descriptive `aria-label`, `min-height: 44px`.

## Consequences

**Positive:**
- Preference persists across page loads on the same device
- No server changes required
- Default `true` is backward compatible
- Simple to test (mock localStorage)

**Negative:**
- Device-specific — won't sync across devices
- SSR hydration mismatch if server renders keyboard visible but localStorage says hidden
  - Mitigated: read in `useEffect` (client-only); tolerable one-frame flash

**Risks:**
- localStorage unavailable in some private browsing modes — wrap in try/catch:

```typescript
function readKeyboardVisible(): boolean {
  try {
    const val = localStorage.getItem('stapler-squad-mobile-keyboard-visible')
    return val === null ? true : val === 'true'
  } catch {
    return true
  }
}
```

## References

- requirements.md — "Toggle button for .mobileKeyboard in TerminalOutput.tsx (show/hide, state in localStorage)"
- findings-features.md Section 3 — Hamburger menu toggle pattern
