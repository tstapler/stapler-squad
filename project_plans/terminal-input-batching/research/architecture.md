# Architecture Research: Terminal Input Batching

Agent 3 (Architecture), SDD Phase 2. Scope: integration design for coalescing
`sendInput()` calls in `web-app/src/lib/hooks/useTerminalFlowControl.ts` without
disturbing the existing >512-byte paste chunker in the same function.

## Prior research check

`Glob project_plans/*/research/architecture.md` (127 files) grepped for
`useTerminalFlowControl` and `useTerminalStream` — **none found**. The closest
adjacent docs are `project_plans/terminal-jank/research/architecture.md`,
`project_plans/terminal-robustness/research/architecture.md`, and
`project_plans/terminal-resize-fit-loop/research/architecture.md`, but none
touch `sendInput`/chunking. This is fresh ground; no full hotspot-analysis run
(out of scope per task instructions).

## 1. Current architecture of `useTerminalFlowControl.ts`

`web-app/src/lib/hooks/useTerminalFlowControl.ts:34-40` — hook signature:

```ts
export function useTerminalFlowControl({
  sessionId,
  getTerminal,
  pushMessageRef,
  isConnectedRef,
  onError,
}: UseTerminalFlowControlOptions): UseTerminalFlowControlResult
```

- **`sessionId: string`** — plain value prop, not a ref. Re-passed every
  render from the parent (`useTerminalStream`), which itself gets it as a
  plain prop from `TerminalOutput`.
- **`getTerminal: () => Terminal | null`** — getter, evaluated at call time,
  not used by `sendInput`.
- **`pushMessageRef: React.MutableRefObject<((msg: TerminalData) => void) | null>`**
  — the single message-emission seam. `useTerminalStream.ts:134-142` populates
  it in a `useEffect` that wraps `messageQueueRef.current?.push(msg)`; the ref
  indirection exists specifically so `sendInput`'s `useCallback` doesn't close
  over a stale `messageQueue`.
- **`isConnectedRef: React.MutableRefObject<boolean>`** — kept in sync with
  `isConnected` state via a separate effect (`useTerminalStream.ts:126-128`).
- **`onError?: (error: Error) => void`** — surfaced through the hook's own
  `handleError` helper (line 68-71), itself wrapped again by
  `useTerminalStream`'s `handleError` (sets `error` state + calls the same
  `onError` prop). Errors from any new batching code should route through the
  existing local `handleError`, not a new channel.

**Refs owned by the hook** (lines 42-49): `isResyncingRef`,
`waitingForPaneResponseRef`, `lastResyncTimeRef`, `lastResizeTimeRef`,
`lastSentDimsRef`, `pendingResizeTimerRef`, `paneRequestTimerRef`,
`dimensionSyncRef`. All are `useRef`, so they persist for the life of the
*hook-call-site* (the enclosing component instance), not per-render.

**Existing unmount cleanup** (lines 54-65): a single `useEffect(() => () => {...}, [])`
that clears `pendingResizeTimerRef` and `paneRequestTimerRef` on unmount, with
an explicit comment about not letting timers fire against a torn-down
connection.

**`sendInput` today** (lines 142-195):
1. Bails if `!pushMessageRef.current || !isConnectedRef.current`.
2. UTF-8 encodes `input` to `inputBytes`.
3. If `inputBytes.length <= PASTE_CHUNK_SIZE` (512): one immediate
   `pushMessage(TerminalData{ data: { case: "input", value: TerminalInput } })`
   call, synchronous, no timers.
4. Else: captures `sessionIdAtStart = sessionId`, then recursively
   `setTimeout`-schedules `sendChunk()` every `CHUNK_DELAY_MS` (10ms),
   slicing `PASTE_CHUNK_SIZE`-byte chunks off `inputBytes`, aborting if
   `sessionId !== sessionIdAtStart` (stale closure comparison — see §4) or if
   `pushMessageRef.current`/`isConnectedRef.current` become falsy mid-flight.

## 2. Composition in `useTerminalStream.ts`

- `web-app/src/lib/hooks/useTerminalStream.ts:144-150` — single call site:
  ```ts
  const flowControl = useTerminalFlowControl({
    sessionId, getTerminal: getTerminal ?? (() => null),
    pushMessageRef, isConnectedRef, onError,
  });
  ```
- `useTerminalStream.ts:473` — `sendInput: flowControl.sendInput` is
  re-exported verbatim into `TerminalStreamResult`; `useTerminalStream` adds
  no wrapping logic around `sendInput` itself (contrast with `resize`, which
  is also passed through untouched — throttling lives entirely inside
  `useTerminalFlowControl`).
- Caller: `TerminalOutput.tsx:456` destructures `sendInput` from
  `useTerminalStream(...)` and wires it to xterm.js's `onData` handler
  (`TerminalOutput.tsx:568`), one call per xterm-reported input event.

**Component-instance lifetime / `sessionId` stability** (relevant to §4):
`TerminalOutput` is rendered per-session with `key={poolId}` /
`key={session.id}` inside a keep-alive pool
(`SessionDetailView.tsx:697-701`, `775-784` — `pooledSessionIds.map(poolId => <div key={poolId}><TerminalOutput sessionId={poolId} .../></div>)`).
Because `key` equals `sessionId`, **a mounted `TerminalOutput`/`useTerminalStream`/
`useTerminalFlowControl` instance never sees its `sessionId` prop change** —
switching sessions unmounts the old keyed instance and mounts a new one under
a different key, it does not re-render the same instance with a new
`sessionId`. All hook refs (including any new batch-buffer refs) are
therefore scoped 1:1 to a single session's lifetime by construction; the
`sessionIdAtStart !== sessionId` guard in the existing chunker is effectively
dead code under the current render tree (kept only as defense-in-depth,
confirmed also exercised directly by unit tests that call `rerender()` with a
new `sessionId` prop on the *same* instance — see
`TerminalOutput.reconnect.test.tsx` and `useTerminalFlowControl.test.ts`,
which do change `sessionId` across `rerender`/re-invocation without
unmounting, so the guard is real and tested, just not reachable via the
current production `SessionDetailView` render tree).

## 3. Integration point design

**Where new state lives**: entirely inside `useTerminalFlowControl`'s own
closure, as new `useRef`s sitting alongside the existing ones (lines 42-49) —
no new hook, no new file. Proposed additions:

```ts
const pendingBytesRef = useRef<Uint8Array[]>([]);   // accumulated chunks awaiting flush
const pendingByteLengthRef = useRef(0);              // running total, avoids re-summing on every push
const flushTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
```

**Unmount cleanup**: yes — extend the *existing* cleanup effect (lines 54-65)
rather than adding a second `useEffect`. It already exists precisely to
cancel in-flight timers on teardown, and `flushTimerRef` is structurally
identical to `pendingResizeTimerRef`/`paneRequestTimerRef` (a `setTimeout`
handle guarding a deferred send). Adding a third `if (flushTimerRef.current) { clearTimeout(...); flushTimerRef.current = null; }`
block keeps all "cancel deferred sends on unmount" logic in one place instead
of scattering a second effect that would race the first for no benefit.
**However**, per Acceptance Criterion 5 ("pending batched input is flushed on
disconnect/unmount, not silently dropped"), plain cancellation is
insufficient here — unlike a resize (safe to drop, next resize supersedes
it) or a pane-request (safe to drop, resync will re-request), buffered
keystrokes are user input the server has never seen. The cleanup callback
must **flush-then-clear**, not clear-only: call the same `flush()` function
used by the timer path before nulling the ref, guarded by
`pushMessageRef.current && isConnectedRef.current` (best-effort — if the
socket is already gone there is nowhere to flush to, and that's an accepted
loss consistent with any other in-flight message at disconnect time, not a
new regression).

## 4. Data flow: batching wraps chunking, not the reverse

Smallest-diff shape: **batching sits strictly in front of the existing
large-input path and hands off through it unchanged.** Do not restructure
`sendChunk`/the immediate-send branch to accept pre-buffered byte arrays —
that would touch tested, working code for no functional gain.

Proposed flow inside `sendInput(input: string)`:

1. Encode `input` to `inputBytes` (unchanged first step).
2. If `inputBatchDelayMs <= 0` (default): **skip batching entirely**, fall
   straight into today's `if (inputBytes.length <= PASTE_CHUNK_SIZE) {...} else {...}`
   body verbatim. This satisfies AC1 (byte-identical at default) by
   construction — the batching branch is not merely a no-op path, it's an
   `if` that bypasses the new code altogether, so there's no risk of a
   behavior change hiding in a "trivial" batch-of-one.
3. If `inputBatchDelayMs > 0`: push `inputBytes` onto `pendingBytesRef.current`,
   add `inputBytes.length` to `pendingByteLengthRef.current`.
   - If `pendingByteLengthRef.current >= EARLY_FLUSH_BYTES` (32, per
     requirements): call `flush()` synchronously now (early-flush path,
     AC3), and clear any pending `flushTimerRef`.
   - Else if no timer is already pending, schedule
     `flushTimerRef.current = setTimeout(flush, inputBatchDelayMs)`. Do
     **not** reset an already-pending timer on every keystroke (that would
     turn this into a debounce and could starve a very fast, continuous
     typist past the delay window indefinitely) — first-byte-in starts the
     clock, matching herdr-web's fixed-window coalescing semantics implied
     by the requirements ("flush on a timer OR ... whichever comes first").
4. `flush()` — the hand-off point into existing chunking:
   ```ts
   const flush = () => {
     if (flushTimerRef.current) { clearTimeout(flushTimerRef.current); flushTimerRef.current = null; }
     if (pendingBytesRef.current.length === 0) return;
     const merged = concatUint8Arrays(pendingBytesRef.current); // single allocation
     pendingBytesRef.current = [];
     pendingByteLengthRef.current = 0;
     sendBytes(merged); // <-- existing lines 148-194, extracted to take bytes not a string
   };
   ```
   This requires exactly one refactor to today's code: extract the body of
   `sendInput` from "UTF-8 encode" onward into a `sendBytes(inputBytes: Uint8Array)`
   helper, so both the direct (batching-off) path and `flush()` can call it.
   `sendBytes` is otherwise a verbatim lift of the current `if (...) {...} else {...}`
   block (immediate send ≤512B, chunked send >512B) — zero logic changes,
   pure extraction. This is what makes AC4 ("a flushed batch larger than
   `PASTE_CHUNK_SIZE` still gets split") fall out for free: `flush()` just
   calls the same function that already does that split, on whatever byte
   length happens to have accumulated.

This also means the 32-byte early-flush threshold and the 512-byte
`PASTE_CHUNK_SIZE` threshold are independent and composable by construction,
not by special-cased interaction: early-flush governs *when* buffered bytes
are handed to `sendBytes`; `PASTE_CHUNK_SIZE` governs *how* `sendBytes` emits
whatever it's given. A flush at exactly 32 bytes takes the immediate-send
branch inside `sendBytes`; a flush that happened to accumulate past 512 bytes
(possible if `inputBatchDelayMs` is large and a paste lands inside the
window — see below) takes the chunked branch automatically.

**Paste interaction, named explicitly**: xterm.js delivers a full paste as
one `onData` call, so one `sendInput(input)` invocation. If batching is on
and that call arrives while a batch is already pending, the paste's bytes get
appended to `pendingBytesRef` like any other input — which is *correct* per
AC4/AC7 (order preserved, still chunks at flush) but means a paste can be
held up to `inputBatchDelayMs` before it starts sending, rather than today's
"chunking starts immediately." Given `EARLY_FLUSH_BYTES = 32` and typical
pastes being far larger than that, the early-flush threshold will fire
almost immediately for any real paste (within the same synchronous
`sendInput` call, since the paste bytes alone exceed 32 bytes) — so in
practice a paste triggers the early-flush branch inline, not the timer
branch, and the added latency is negligible. Worth a unit test explicitly
(paste-sized input triggers early-flush, not timer-wait) since it's the one
place batching's timing behavior could regress today's low-latency paste
path if the early-flush check were implemented wrong (e.g. checked only
*before* appending instead of after).

## 5. Consistency: session-changed guard for the new buffer

**Should an equivalent abort apply to the batch buffer?** Yes, for the same
reason it exists on the chunker, and it costs nothing extra: `flush()` is
already being extracted to call `sendBytes`, and `sendBytes` (the renamed
existing block) already captures `sessionIdAtStart = sessionId` and checks it
per-chunk. Since `flush()` calls `sendBytes` with the *current* `sessionId`
closure value at flush time, the existing guard automatically covers
data that was buffered under an old `sessionId` and flushed after a
(hypothetical) `sessionId` change — no separate check needed in the batch
path itself, as long as `flush()` reads `sessionId` fresh (via closure, which
`useCallback`'s dependency array on `sessionId` already guarantees a new
`sendInput`/`flush` identity for) rather than capturing it once when the
buffer was first created.

**Is `sessionId` itself stable across the hook's lifetime?** In production,
yes by construction — see §2: `TerminalOutput` instances are `key`-ed by
session/pool id, so a mounted `useTerminalFlowControl` instance's `sessionId`
prop never actually changes; a session switch unmounts and remounts under a
new key, running the (extended, flush-then-clear) cleanup effect from §3
rather than a same-instance prop update. The `sessionId !== sessionIdAtStart`
guard — and the new buffer inheriting it for free — is therefore
defense-in-depth for: (a) the direct unit tests that *do* call `rerender()`
with a changed `sessionId` on the same hook instance without unmounting
(`useTerminalFlowControl.test.ts`, `TerminalOutput.reconnect.test.tsx`), and
(b) any future caller that stops keying by session id. Cheap to keep
correct, not safe to drop.

## Event-Command-Policy table

**Skipped.** This is a pure client-side transport optimization inside a
single React hook — no multi-actor workflow, no cross-service commands, no
policy/authorization surface. Confirmed against the requirements' own
Non-Goals ("Server-side / tmux-side batching — out of scope", "No proto/backend
changes are implied") and the Constraints section limiting the change to two
frontend files. Nothing here fits the Event-Command-Policy frame.

## Secondary finding (for Phase 3 awareness, not this task's scope)

Requirement item 3 ("Expose the delay as a user setting in the existing
terminal config UI ... following the same `TerminalConfig` +
`loadTerminalConfig`/`saveTerminalConfig` pattern") assumes a settings UI
already renders `TerminalConfig` fields for editing. Searching
`web-app/src/components/**/*.tsx` for `useTerminalConfig` / `saveTerminalConfig`
/ `loadTerminalConfig` finds **only `XtermTerminal.tsx`**, which *reads*
`loadTerminalConfig()` to apply `cursorBlink`/etc. to the live terminal
instance — no component was found that calls `useTerminalConfig()`'s setter
or `saveTerminalConfig()` to let a user *edit* these values (`fontSize`,
`cursorBlink`, etc. have no discoverable settings form in the current
codebase, only the config/persistence layer and the read side). If true,
Phase 3 planning should confirm whether such a panel exists elsewhere (e.g.
gated behind a feature flag, or named differently) before assuming
`inputBatchDelayMs` can simply be "added alongside the existing fields" —
it may need its own minimal settings affordance rather than an existing one
to extend.

## Files referenced

- `web-app/src/lib/hooks/useTerminalFlowControl.ts` (full read)
- `web-app/src/lib/hooks/useTerminalStream.ts:1-160,440-489`
- `web-app/src/lib/config/terminalConfig.ts` (full read)
- `web-app/src/components/sessions/TerminalOutput.tsx` (grepped)
- `web-app/src/components/sessions/SessionDetailView.tsx:670-785`
- `web-app/src/components/sessions/XtermTerminal.tsx` (grepped)
- `web-app/src/lib/hooks/__tests__/useTerminalFlowControl.test.ts` (grepped for existing test structure)
