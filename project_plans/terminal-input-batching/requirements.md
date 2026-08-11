# Requirements: Terminal Input Batching

## Source

Backlog item `762417d4-d848-4168-a231-1df67485909b`, migrated from
[TylerStaplerAtFanatics/stapler-squad#173](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/173).
No interactive ideation interview — this is an autonomous SDD triage pass; requirements
below are derived directly from the item description and this repo's existing terminal
input code.

**Complexity**: 2 (feature-level change — new buffering behavior in an existing hot-path
hook plus a config/settings surface; single-file core change, no new services, RPCs, or
proto/schema changes).

## Problem

`web-app/src/components/sessions/TerminalOutput.tsx` wires xterm.js's `onData` handler
straight to `sendInput()` (`TerminalOutput.tsx:568`), which in turn
(`web-app/src/lib/hooks/useTerminalFlowControl.ts:142`) pushes one `TerminalData` /
`case: "input"` protobuf message onto the WebSocket message queue per call. xterm.js
fires `onData` once per keystroke and once per paste event — but on paste, the *browser*
already delivers the full pasted string as a single `onData` call, so the "many small
WS messages" problem described in the source issue is really about **fast individual
keystrokes** (e.g. holding a key, or an IME/host terminal that decomposes input),
not paste specifically. That distinction matters for scoping the fix below.

The existing `sendInput` already has a *chunking* path (`PASTE_CHUNK_SIZE = 512` bytes,
`CHUNK_DELAY_MS = 10`ms) for the opposite problem — splitting one large input into
multiple sends so tmux's PTY write buffer isn't overrun. Batching (coalescing many
small sends into fewer) and chunking (splitting one large send into many) are
complementary, not conflicting, and must compose: a batch flush that itself exceeds
`PASTE_CHUNK_SIZE` should still chunk on the way out.

## Goal

Reduce WebSocket message count for high-frequency small keystroke bursts (e.g. held-key
repeat, fast typing, non-Latin IME composition) by coalescing them into a single message
per short time window, while preserving the current low-latency, chunked path for
paste-sized input and never introducing perceptible input lag at the default setting.

## Non-Goals

- Changing the paste chunking behavior (`PASTE_CHUNK_SIZE` / `CHUNK_DELAY_MS`) — out of
  scope; batching sits in front of it, not instead of it.
- Server-side / tmux-side batching — this is purely a client `sendInput` transport
  change.
- New session creation modes, new RPCs, or proto changes — no touchpoints from
  `.claude/rules/session-creation-registry.md` apply.

## Proposed Behavior (from the backlog item, adapted to this repo's code)

1. Add a configurable `inputBatchDelayMs` (default `0` = off, matching herdr-web's
   default-off stance) to the terminal input path.
2. Buffer accumulated input bytes; flush on a timer OR immediately once the buffered
   byte count reaches an early-flush threshold (herdr-web uses 32 bytes) — whichever
   comes first.
3. Expose the delay as a user setting in the existing terminal config UI
   (`web-app/src/lib/config/terminalConfig.ts` / its settings panel), following the same
   `TerminalConfig` + `loadTerminalConfig`/`saveTerminalConfig` pattern already used for
   `fontSize`, `cursorBlink`, etc.
4. At `inputBatchDelayMs <= 0`, behavior must be byte-for-byte identical to today (each
   `sendInput` call flushes immediately) — this is the default, so no regression risk for
   users who don't opt in.

## Acceptance Criteria

1. `useTerminalFlowControl` (or a small helper it uses) exposes an `inputBatchDelayMs`
   option; at `0` (default), every `sendInput` call sends its own `TerminalData` message
   immediately, with no behavior change from the current implementation.
2. When `inputBatchDelayMs > 0`, consecutive small `sendInput` calls within the delay
   window are coalesced into a single `TerminalData`/`input` message instead of one
   message per call.
3. Buffered-but-unflushed input is flushed immediately once it reaches a fixed
   byte threshold (early-flush), so paste-like bursts are not held back by the timer.
4. The early-flush and paste-chunking paths compose correctly: a flushed batch larger
   than `PASTE_CHUNK_SIZE` still gets split into ≤512-byte chunks with the existing
   `CHUNK_DELAY_MS` inter-chunk delay — no PTY buffer overrun regression.
5. Pending batched input is flushed on disconnect/unmount (no silently dropped
   keystrokes when a session closes mid-batch).
6. `inputBatchDelayMs` is persisted via the existing `TerminalConfig` /
   `loadTerminalConfig`/`saveTerminalConfig` mechanism and is user-editable from the
   terminal settings UI, with the same options herdr-web exposes (0/32/64/128/256ms) or
   a documented reason for a different option set.
7. Ordering is preserved: batched bytes arrive at the PTY in the same order the user
   typed them (no reordering across the buffer/flush boundary).
8. Existing `useTerminalStream`/`useTerminalFlowControl`/`TerminalOutput` tests continue
   to pass, and new tests cover: default-off passthrough, coalescing under the delay,
   early-flush at the byte threshold, and flush-on-unmount.

## Constraints / Context

- Frontend-only change: `web-app/src/lib/hooks/useTerminalFlowControl.ts` (message
  send path), `web-app/src/lib/config/terminalConfig.ts` (new setting), and whatever
  settings-panel component currently renders `TerminalConfig` fields.
- This is a `perf` item — priority driven by actual measured impact, not assumption.
  Typical interactive typing (a few keystrokes/sec) is nowhere near WebSocket message
  overhead limits; the benefit is real but narrow (rapid key-repeat, fast pasting from
  a source that fires many small `onData` events, low-end clients). No user-reported
  performance complaint is attached to this item — it originates from studying another
  project's implementation, not from an observed stapler-squad problem.
- No proto/backend changes are implied — `TerminalData`/`TerminalInput` schema is
  unchanged; only how many times the client calls `pushMessage` changes.
