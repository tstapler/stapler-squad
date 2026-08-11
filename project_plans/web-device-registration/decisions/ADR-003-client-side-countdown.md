# ADR-003: Client-Side TTL Countdown with ttl_seconds

**Status**: Accepted
**Date**: 2026-04-21
**Feature**: Web Device Registration

---

## Context

The `AddDeviceModal` must show the user how much time remains on the invite before it expires. The invite has a 15-minute TTL. Three approaches were considered:

1. **Client-side countdown from `ttl_seconds`**: The generate response includes `"ttl_seconds": 900`. The modal starts a `setInterval` on mount that decrements a local state counter every second. No server involvement after the initial generate call.
2. **Server polling**: The modal periodically calls `GET /auth/invite/status` (or similar) to fetch the current remaining TTL.
3. **Server-Sent Events / WebSocket push**: The server pushes expiry events to the client.

---

## Decision

Use a client-side `setInterval` countdown driven by `ttl_seconds` from the generate response. No polling and no push channel.

Implementation pattern for `AddDeviceModal`:

```tsx
const [secondsLeft, setSecondsLeft] = useState(invite.ttl_seconds);

useEffect(() => {
  if (secondsLeft <= 0) return;
  const id = setInterval(() =>
    setSecondsLeft(s => {
      if (s <= 1) { clearInterval(id); return 0; }
      return s - 1;
    }), 1000);
  return () => clearInterval(id);
}, []);
```

When `secondsLeft === 0`, the modal shows an "Invite expired — generate a new one" message and disables the copy button. The user can click "Generate New Invite" to call the endpoint again.

The generate response also includes `"expires_at"` (ISO 8601 timestamp) as a fallback reference so the client can cross-check against `Date.now()` on mount if the modal is re-opened after a reload.

---

## Rationale

**Accuracy is sufficient.** A `setInterval(1000)` countdown is accurate to within ±1 second per interval. For a 15-minute window the cumulative drift is negligible. The user does not need millisecond precision; they need to know roughly how long they have.

**No additional server load.** Polling would require the server to maintain invite state queryable by ID, add a new authenticated endpoint, and handle the case where the invite no longer exists. For a single-user tool with a 15-minute TTL this overhead is not justified.

**No infrastructure dependency.** Server-Sent Events or WebSockets would require persistent connections. The existing Stapler Squad web layer uses ConnectRPC streaming for session output, but introducing a persistent channel just for a countdown timer in a modal is disproportionate.

**Consistent with existing patterns.** The frontend already uses `useEffect` with `setInterval` for polling and time-based UI (e.g., session status refresh). This pattern is understood and tested.

**`expires_at` provides correctness baseline.** If the page reloads or the modal is reopened, the client can compute `Math.max(0, Math.floor((new Date(invite.expires_at).getTime() - Date.now()) / 1000))` to initialize `secondsLeft` accurately, avoiding the countdown-starting-at-900-when-5-minutes-remain bug.

---

## Consequences

**Accepted costs:**
- If the client's clock is significantly wrong (e.g., system clock skew), the countdown may be inaccurate. Acceptable for a LAN/Tailscale single-user tool.
- The modal will not automatically show an expiry notification if the user leaves the tab and returns. The `expires_at` timestamp re-initialization on mount handles this correctly.

**Not accepted:**
- Polling `GET /auth/invite/status`. Extra endpoint, extra server state, extra request traffic, no UX benefit over a client-side timer.

---

## Alternatives Rejected

**Server polling**: Rejected. Adds an endpoint, a query-by-ID lookup path in `InviteManager`, and network round-trips. The client-side timer achieves the same UX with zero additional infrastructure.

**Server-Sent Events / WebSocket push**: Rejected. Disproportionate infrastructure for a countdown in a modal. The existing ConnectRPC streaming infrastructure is purpose-built for terminal output, not for one-time UI events.
