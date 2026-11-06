# ADR-002: Cross-tab write conflicts on the shared windows blob use a revision-guarded read-check-write (Optimistic Offline Lock)

**Date**: 2026-09-23
**Status**: Accepted

## Context

`requirements.md`'s Rabbit Holes section (updated after per-tab binding was added)
calls cross-tab concurrent writes "load-bearing rather than edge-case": two tabs
viewing *different* windows will each debounce-save the *entire* shared `windows`
array whenever either one edits any window (a rename, a pane split, a session
assignment). `research/pitfalls.md` confirms today's `usePaneReducer.ts` has no
cross-tab awareness at all (`savePaneLayout` writes blind) and recommends, as the
appetite-appropriate option: "read-check-write... if its revision counter has moved
past what this tab last loaded, log a client-side warning and skip the write rather
than clobbering."

The alternatives considered:
- **True merge/reconciliation** (diff two windows arrays, merge field-by-field) — out
  of scope for the 3–6 week appetite per `research/pitfalls.md`; requires designing a
  conflict resolution policy for arbitrary pane-tree edits, which is disproportionate
  to a client-side, single-user, no-oncall-alert feature.
- **Pessimistic lock** (Web Locks API, `navigator.locks.request`) — serializes writes
  across tabs but doesn't answer "whose edit wins," adds a browser-API dependency this
  codebase doesn't otherwise use, and doesn't fit the "no feature flag, mitigate the
  specific risk instead" posture in requirements.md's Risk Control.
- **Last-write-wins with no guard** (today's behavior) — silently loses whichever
  tab wrote first; requirements.md explicitly calls this out as newly load-bearing
  and unacceptable to leave as-is.

## Decision

This is [PoEAA's Optimistic Offline Lock](https://martinfowler.com/eaaCatalog/optimisticOfflineLock.html)
pattern, applied to a single localStorage key instead of a database row:

- `PersistedWindowLayoutV2` carries a monotonic `revision: number`.
- Before every debounced save, `useWindowManager` re-reads the current stored value
  and compares its `revision` to the `revision` this tab last observed
  (`lastKnownRevisionRef`).
  - If the stored revision is still equal to what this tab last observed: proceed,
    write `revision + 1`, update `lastKnownRevisionRef`.
  - If the stored revision has moved ahead (another tab wrote since this tab's last
    read): log a client-side warning (`console.error`, matching the existing
    try/catch logging convention in `usePaneLayout.ts`), **skip this write**, and
    immediately adopt the newer stored value into this tab's in-memory state
    (dispatch `RESTORE_WINDOWS`) so the tab doesn't stay stuck unable to save forever.
- A `window.addEventListener("storage", ...)` listener provides the same
  adopt-newer-value behavior for a tab that isn't actively mid-save when another tab
  writes — this is the "live-update mechanism" requirements.md's Rabbit Holes section
  asks for, so a backgrounded tab picks up a rename made elsewhere without a manual
  reload.

## Consequences

- A tab whose save is skipped loses that specific in-flight edit — this is a known,
  logged limitation (per pitfalls.md: "converts silent data loss into a detectable,
  logged event"), not silent data loss. True reconciliation is explicitly deferred,
  matching the Alternatives Considered / Out of Scope framing in requirements.md.
- `lastFocusedWindowId` (see ADR-001) deliberately does **not** go through this guard
  — it's a single scalar with no partial-tree content to lose, so last-write-wins on
  it is harmless by construction, and gating it through the revision check would
  force every window-switch (not just content edits) to touch the shared blob,
  reintroducing exactly the "switching in one tab shouldn't disturb another tab's
  save cadence" problem ADR-001 avoids.
- The `storage` event never fires in the tab that performed the write (per the Web
  Storage API spec), so there is no self-triggered reconciliation loop.
