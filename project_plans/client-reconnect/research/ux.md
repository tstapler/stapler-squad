# UX Research: client-reconnect

**Date**: 2026-06-23  
**Feature**: Soft reconnect for session-watch + terminal streams  
**Complexity**: 3

---

## 1. Comparable UX Patterns from Production Apps

### How leading apps handle disconnection/reconnection

**Figma** — shows a yellow "You are offline" banner across the top, but keeps the canvas fully interactive with local state. It does not count down or offer a button; it just polls silently and removes the banner when connectivity returns. Key lesson: do not block work; stay ambient.

**Linear** — shows a small pill near the top-right ("Offline") that turns amber. There is no countdown; the app transparently reconciles state once the stream reconnects. Clicking the pill does nothing; Linear treats reconnection as automatic and below the user's attention level.

**Slack** — on mobile shows a "Connecting…" banner with an animated pulsing dot. On desktop it places a subtle yellow header bar. There is no countdown; Slack treats reconnect as a system concern, not a user task. If it takes more than ~15 s the bar changes text to "Trying to connect." The pattern is: invisible when working, ambient when degraded, escalate text only on prolonged failure.

**Vercel (dashboard)** — real-time deployment logs use a "Reconnecting…" inline spinner inside the log viewport on disconnect. The page never reloads. If reconnect fails permanently after ~6 attempts, a "Reload" button appears inline. The primary action is transparent reconnect; the manual action is a last resort.

**GitHub (Actions live logs)** — identical philosophy to Vercel: the log stream reconnects silently. A subtle "Connection lost, retrying…" line appears in the log stream itself as plain text, rather than a modal or toast.

**VS Code Server / code-server** — shows a bottom-status-bar message "Server Disconnected" with two buttons: "Retry" and "Reload Window". This is the closest comparable to our terminal use case. Key differentiator: "Reload Window" destroys in-progress work; it is offered only as a last resort and is visually de-emphasised relative to "Retry".

### Distilled principles from production apps

1. **Reconnect should be automatic and silent** — only surface state to the user when reconnect takes long enough to be noticeable (threshold: 3–5 s).
2. **Never reload the page automatically** — page reload destroys React state, session list filters, selected session, scroll position. Even VS Code offers "Reload" as a manual escape hatch, not an automatic action.
3. **Countdown timers are anxiety-inducing** — Figma, Linear, and Slack all avoid countdown text. The countdown pattern is associated with forced-log-out warnings and CAPTCHA timeouts; it tells users "something bad is happening on a schedule." A spinner communicates "working on it" without inducing time pressure.
4. **Attempt counts are useful in tooltips, not in primary copy** — surfacing "attempt 3/5" tells a technical user that the system is trying, without making a non-technical user feel the system is failing.

---

## 2. Mental Model: What Users Expect from Each State Label

### "Connected" / "Live"
User expectation: everything is working. Indicator should be quiet — a small green dot is sufficient. The text "Live" is appropriate because it implies real-time data, matching the mental model of a streaming session list.

### "Stale"
User expectation: ambiguous. Most users do not know what "stale" means in a data-sync context. Research consistently shows that ambiguous state labels cause users to either ignore the indicator (assuming it will self-resolve) or take drastic action (reload). "Stale" is an internal implementation concept, not a user concept.

**Recommendation**: Rename to "Catching up" or collapse into the "Reconnecting" state. The distinction between stale (stream closed gracefully, data frozen) and disconnected (stream errored) is meaningful to engineers but invisible and irrelevant to users. If the data is not live, the app is in one state from the user's perspective: "not fully connected."

### "Offline" / "Disconnected"
User expectation: "my work is at risk." This is the highest-alarm state. Users expect the app to be actively trying to fix this. The label "Offline" carries the connotation of no internet connection, which is a different failure mode than "server stream dropped." Consider "Reconnecting…" as the primary label while the reconnect loop is running, reserving "Offline" for when reconnects have been exhausted.

### Recommendation: 2 visible states instead of 3

| Internal state | User-visible label | Colour | Behaviour |
|---|---|---|---|
| `connected` | "Live" | green | Indicator silent; no tooltip needed |
| `stale` or `disconnected` while attempting | "Reconnecting…" (with spinner) | amber | Ambient indicator; tooltip shows attempt count |
| `disconnected` + attempts exhausted | "Reconnecting failed" | red | Shows "Retry" button inline; still no reload |

This collapse eliminates the "Stale" vs. "Offline" confusion without losing fidelity.

---

## 3. Terminal Reconnect UX

### What to show during reconnect

The terminal already has a partial implementation: the toolbar status row shows "Disconnected • Reconnecting (attempt N/5)…" and a "🔄 Reconnect" button appears after 5 s. This is a solid foundation. Problems with the current approach:

- The status text is small and inside the toolbar, below the terminal pane. Users watching output may not notice.
- After 5 failed attempts the text says "Terminal unavailable" but there is no inline call-to-action near the terminal content area.

**Recommendation: thin reconnecting banner overlaid on the terminal content**

BrowserTab already implements this pattern correctly in `reconnectingBanner` (absolute-positioned pill, warning colours, "Reconnecting… [Reconnect]" button). This is the right design; apply the same overlay approach to TerminalOutput.

Specifics:
- Show the banner only after the first reconnect attempt fails (i.e., after ~2 s), not immediately on disconnect.
- Banner text: "Reconnecting terminal…" with a spinner. No countdown.
- If auto-reconnect fails 5 times: change to "Connection lost — [Retry]" with a primary-styled button.
- Do not clear and replay the terminal buffer on reconnect — preserve existing scroll history. Only append a dim separator line `--- reconnected ---` after successful reconnect, similar to how tmux marks window activity.

### Should the terminal output be cleared on reconnect?

No. Users need the terminal history to understand what was happening when connection was lost. Clearing on reconnect is disorienting and loses context. The terminal stream manager already supports replaying from a sequence number (`after_seq`), so the reconnect can be seamless from the user's perspective.

---

## 4. ConnectionIndicator: Affordance and Interaction Design

### Current state

The `ConnectionIndicator` component is a `<button>` with `window.location.reload()` on click. It is small (8px dot + label hidden on narrow viewports), in the header, and calls `reload()` which destroys React state. The ARIA label says "Click to reload" which is destructive.

### Recommended affordance

Keep it as a `<button>` (not a chip or toast) because it is persistent — it lives in the header throughout the session. Toasts are ephemeral and for one-time notifications; the connection state is persistent context.

Interaction design:
- **Connected**: button disabled (`cursor: default`), no affordance. Green dot + "Live".
- **Reconnecting**: button active, cursor: pointer. Click calls `triggerSoftReconnect()` (a prop callback that re-invokes `watchSessions()`), not `window.location.reload()`. Label: "Reconnecting…" with a CSS spinner (not the dot). Tooltip shows attempt count: "Reconnecting… attempt 2".
- **Reconnect exhausted**: button active, red. Label: "Reconnecting failed". Tooltip: "Click to retry". Click still calls `triggerSoftReconnect()`, not reload.
- **Hard reload**: never triggered automatically. Only available as an escape hatch in the tooltip or a secondary button, and always labelled "Reload page (clears state)" so the user understands the cost.

### Button vs. chip vs. toast

| Option | Pro | Con |
|---|---|---|
| Persistent button in header (current) | Always visible; non-intrusive | Easy to miss on narrow screens |
| Toast notification | High visibility | Dismissible, disappears — user may not notice reconnect failed |
| Banner across top of page | Impossible to miss | Obtrusive for a transient state |
| Inline in session list | Close to the affected content | Only relevant to session list, not terminal |

**Decision**: Keep the persistent button in the header, but add a toast or inline banner for the terminal that is visible to users focused on the terminal pane.

---

## 5. Accessibility

### Current ARIA issues in ConnectionIndicator

```tsx
<button
  aria-label={STATE_ARIA[connectionState]}
  aria-live="polite"   // WRONG: aria-live on a button does not work as intended
  ...
/>
```

`aria-live` on a button element does not cause screen readers to announce state changes. `aria-live` must be on a container element that holds the dynamic text, and the container must be present in the DOM before the text changes (a newly-injected element with `aria-live` will not fire an announcement).

### Correct ARIA pattern

```tsx
// Persistent live region — rendered at all times, even when empty
<div aria-live="polite" aria-atomic="true" className={srOnly}>
  {connectionState !== "connected" ? STATE_ARIA[connectionState] : ""}
</div>

// The visible button
<button
  aria-label={STATE_ARIA[connectionState]}
  aria-disabled={connectionState === "connected"}
  ...
/>
```

Screen readers will announce changes via the live region. `aria-atomic="true"` ensures the whole message is read, not just the changed fragment. Use `aria-live="polite"` (not "assertive") — assertive interrupts mid-sentence which is inappropriate for a connection status update.

### Live region text cadence

- On transition to "Reconnecting…": announce once immediately.
- While retrying: do not repeat the announcement on every retry — that would be extremely noisy (potentially every 2–30 s). Only announce again when the state changes to failed or recovered.
- On recovery to "connected": announce "Connection restored" once.

### Focus management

When the "Reconnect" or "Retry" button is clicked, focus should remain on the button. Do not move focus to the terminal or session list — the user is signalling intent to fix the connection, not to resume work.

### Keyboard interaction

The current `onKeyDown` handler for Enter and Space is correct. Ensure the button is reachable via Tab when in an actionable state.

---

## 6. Error vs. Reconnecting vs. Stale: Should These Collapse?

### Analysis

The three states exist because they reflect internal machinery:
- `connected` → stream healthy, events flowing
- `stale` → stream closed but no error (e.g. server-side stream end after timeout), no auto-reconnect triggered
- `disconnected` → stream errored, auto-reconnect loop running

From a user's perspective, "stale" and "disconnected" are the same: the app is not showing live data. The only practical difference is whether a reconnect is in progress (which should always be true — the `stale` state should trigger a reconnect automatically, not wait for user action).

**Recommendation**: Eliminate the `stale` state as a distinct user-visible label. When the stream transitions to `stale`, immediately begin reconnecting and show "Reconnecting…" to the user. The `stale` value can remain in the Redux store as an internal signal to the reconnect hook, but should map to the same visual treatment as "Reconnecting."

This simplification has a secondary benefit: the ARIA announcement logic becomes trivial — there are only two announcements, one for degraded and one for restored.

---

## 7. Job-to-Be-Done

The user's actual job when they see "Offline" is: **resume monitoring or interacting with their AI agent session as quickly as possible**.

They are not managing a connection. They do not care whether it is a WebSocket, a ConnectRPC stream, or a polling fallback. The disconnection is an interruption to their primary task.

This JTBD framing has three design implications:

1. **Automatic recovery is non-negotiable.** The app must reconnect without user action. The `window.location.reload()` in the current `ConnectionIndicator` forces the user to actively intervene in a system-level concern — this violates the JTBD.

2. **State continuity is critical.** On reconnect, the user must see exactly what they saw before the drop: same selected session, same scroll position in the terminal, same filter state. Page reload destroys all of this. Soft reconnect preserves it.

3. **Minimize cognitive load during reconnect.** While the connection is recovering, the user should be able to continue reading existing terminal output and reviewing existing session data. The UI should not block or dim the content. The reconnect indicator should live at the periphery (header indicator + toolbar status), not in the center of the viewport.

The one exception: if the terminal stream has been disconnected for long enough that the output is likely stale (e.g. the AI agent made progress the user did not see), a brief "Output may be incomplete — reconnected" message in the terminal scroll area is helpful. This should auto-dismiss after 3 s.

---

## 8. Codebase State Summary

| Component | Location | Current Behaviour |
|---|---|---|
| `ConnectionIndicator` | `web-app/src/components/layout/ConnectionIndicator.tsx` | `window.location.reload()` on click; `aria-live` misplaced on button |
| Terminal reconnect loop | `TerminalOutput.tsx` lines 778–791 | Exponential backoff, 5 max attempts, correct |
| Terminal reconnect UI | `TerminalOutput.tsx` lines 1295–1350 | Status text in toolbar; "🔄 Reconnect" button after 5 s; no overlay |
| BrowserTab reconnect | `BrowserTab.tsx` + `BrowserTab.css.ts` | Good pattern: pill banner overlay with "Reconnecting…" + manual button |
| Session watch reconnect | `useSessionService.ts` lines 763–870 | Exponential backoff 1 s→30 s, sets `connectionState` to "disconnected"/"stale" |
| Connection state type | `sessionsSlice.ts` line 9 | `"connected" \| "stale" \| "disconnected"` |

### Key gap

The `ConnectionIndicator` click handler calls `window.location.reload()` but the `watchSessions` hook in `useSessionService.ts` already implements automatic reconnect with backoff. The soft-reconnect fix for the `ConnectionIndicator` is simply: call `watchSessions()` directly (exposed via context or prop) instead of reloading the page. The hard part is threading that callback through `OmnibarContext` or a dedicated `ConnectionContext` to the `ConnectionIndicator`.

---

## 9. Recommendations Summary

1. **ConnectionIndicator**: replace `window.location.reload()` with a `triggerReconnect()` callback that calls `watchSessions()`. Label becomes "Reconnecting…" with spinner (not "Offline"). Fix `aria-live` placement.

2. **State collapse**: user-visible states should be two: `live` and `reconnecting`. The `stale` Redux value triggers immediate reconnect loop without user intervention.

3. **Terminal overlay**: adopt BrowserTab's pill-banner pattern for TerminalOutput disconnection. Show after 2 s of disconnect, not immediately. Do not clear terminal history on reconnect.

4. **Countdown**: do not show a countdown. Show attempt count only in the tooltip ("Reconnecting… attempt 3"). This provides transparency for technical users without inducing anxiety.

5. **Accessibility**: move `aria-live` to a separate visually-hidden container. Announce only on state change, not on every retry.

6. **Recovery marker**: on successful terminal stream reconnect after a gap, append a dim `--- reconnected ---` line in the terminal buffer. This closes the user's uncertainty about whether they missed output.

7. **Hard reload**: retain as an escape hatch, always labelled to communicate the cost ("Reload page — resets state"). Never trigger automatically.
