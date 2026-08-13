# ADR-001: Session-change mid-batch drops the pending buffer (abort), it does not flush-and-relabel

**Status**: Accepted
**Date**: 2026-08-06
**Context**: `project_plans/terminal-input-batching`

## Context

`useTerminalFlowControl.ts`'s existing >512-byte chunk sender (`sendChunk`, lines
168-172) already handles "session changes while a multi-tick send is in flight" by
capturing `sessionIdAtStart = sessionId` once and aborting (silently dropping the
remaining chunks) if the live `sessionId` no longer matches at fire-time:

```ts
const sessionIdAtStart = sessionId;
...
const sendChunk = () => {
  if (sessionId !== sessionIdAtStart) return; // session changed; abort
  ...
};
```

The new input-batching buffer (`pendingBytesRef`) introduces the same shape of
problem one level up: bytes can sit buffered across multiple `sendInput()` calls and
a `setTimeout`-scheduled `flush()`, so a session change could in principle occur
between "bytes buffered" and "flush fires."

`project_plans/terminal-input-batching/research/features.md` (§3, edge case 1)
explicitly flagged this as an *open* design question and leaned toward the opposite
answer from what this ADR adopts: "recommend flush-on-session-change for symmetry
with flush-on-unmount, since both are 'the buffer's destination is about to become
invalid.'" That recommendation was made without noticing a correctness hazard: `flush()`
calls `sendBytes(merged)`, and `sendBytes` reads `sessionId` **fresh at call time**
(that's how `sessionIdAtStart` has always worked). If `flush()` fires after a session
change and is allowed to proceed, the bytes the user typed for session A would be sent
tagged with session B's `sessionId` — silent mislabeling, not just silent dropping.

Note: in production this scenario is not reachable today. `TerminalOutput` instances
are `key`-ed by session/pool id (`SessionDetailView.tsx:697-701`), so a mounted
`useTerminalFlowControl` instance's `sessionId` prop never changes — a session switch
unmounts the old keyed instance (triggering the flush-on-unmount path, ADR-unrelated)
and mounts a fresh one. This decision is defense-in-depth for the unit tests that do
call `rerender()` with a new `sessionId` on the same hook instance
(`useTerminalFlowControl.test.ts`, `TerminalOutput.reconnect.test.tsx`) and for any
future caller that stops keying by session id.

## Decision

The batch buffer captures `pendingSessionIdRef.current = sessionId` when a new batch
starts (the first byte pushed onto an empty `pendingBytesRef`). At flush time, if the
live `sessionId` no longer matches `pendingSessionIdRef.current`, `flush()` **drops**
the buffered bytes (clears `pendingBytesRef`, `pendingByteLengthRef`,
`pendingSessionIdRef`) and returns without calling `sendBytes` — mirroring
`sendChunk`'s existing abort, not inventing a new "flush under the new session"
behavior.

This is a deliberate reversal of `features.md`'s "recommend flush-on-session-change"
leaning, once the mislabeling risk is accounted for.

## Consequences

- **Positive**: no risk of a keystroke buffered for session A ever being sent to the
  server tagged as session B — matches the existing chunker's proven, tested
  precedent exactly, no new failure mode introduced.
- **Negative**: bytes typed just before an in-place session switch (test/rerender
  scenario only, per the production-unreachability note above) are silently lost,
  same as the existing chunker's pre-existing behavior for its own in-flight sends.
  This is consistent with, not worse than, today's code.
- This is **distinct from unmount**, where `flush()` is still expected to succeed:
  unmount does not change `sessionId` out from under the buffer first, so
  `sessionId === pendingSessionIdRef.current` still holds at cleanup time and the
  flush sends normally (see Acceptance Criterion 5, Story 1.4.1 in `plan.md`).

## Alternatives Considered

1. **Flush-on-session-change** (features.md's original lean): flush the pending
   buffer synchronously the moment `sessionId` is detected to differ, before the
   batch's own timer would have fired. Rejected: requires detecting the change
   proactively (an effect watching `sessionId`), adds a second flush trigger
   distinct from "timer fires" and "byte threshold crossed," and still needs the
   same abort-vs-relabel decision at the moment of that synchronous flush — it
   doesn't remove the core mislabeling question, it just moves it earlier.
2. **Relabel and send under the new session** (do nothing — let `flush()`'s natural
   fresh-`sessionId` read proceed): rejected outright — this is the mislabeling bug
   described above, not a real option.
