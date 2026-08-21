# Research: Stack (Terminal Input Batching)

## 1. Current `sendInput` implementation

`web-app/src/lib/hooks/useTerminalFlowControl.ts:142-195` — full text read directly.

- Line 139-140: `PASTE_CHUNK_SIZE = 512` bytes, `CHUNK_DELAY_MS = 10`ms — the existing
  chunking constants declared as plain `const`s inside the hook body (not module-level,
  not part of `UseTerminalFlowControlOptions`).
- `sendInput(input: string)` (142): encodes to `Uint8Array` via `new TextEncoder().encode(input)`
  on every call (a fresh `TextEncoder` instance each call — no shared/module-level instance).
- If `inputBytes.length <= PASTE_CHUNK_SIZE`, sends one `TerminalData`/`case:"input"` message
  immediately via `pushMessage`.
- Else, chunks via a local recursive `sendChunk` closure using `setTimeout(sendChunk, CHUNK_DELAY_MS)`,
  capturing `sessionIdAtStart = sessionId` at call time so a session change mid-paste silently
  aborts remaining chunks (line 172: `if (sessionId !== sessionIdAtStart) return;`). **This exact
  abort-on-session-change pattern is the direct precedent for AC5 ("session-change mid-batch
  aborts pending buffer for old session") in the requirements** — the new batch buffer should
  follow the same capture-at-schedule-time + compare-at-fire-time shape, not a new mechanism.
- Guard at top of `sendInput` (143): `if (!pushMessageRef.current || !isConnectedRef.current) return;`
  — same guard reused inside `sendChunk` per-chunk (171) so a mid-chunk disconnect also aborts.
- `useCallback` dependency arrays throughout consistently include `[sessionId, pushMessage,
  pushMessageRef, isConnectedRef, handleError]` — any new batching state/refs introduced should
  follow this same pattern if they need to be read inside `sendInput`'s callback.

## 2. Test file conventions

`web-app/src/lib/hooks/__tests__/useTerminalFlowControl.test.ts` — full text read.

- **Framework**: Jest (`jest.mock`, `jest.fn`, `jest.useFakeTimers`) + React Testing Library
  (`renderHook`, `act` from `@testing-library/react`).
- `beforeEach`: `jest.useFakeTimers()` + `console.log`/`console.warn` spies silenced.
  `afterEach`: `jest.restoreAllMocks()` + `jest.useRealTimers()`. New batching tests should
  follow this same setup/teardown so `setTimeout`-based flush timers are advanced via
  `act(() => { jest.advanceTimersByTime(ms); })` rather than real waits.
- `@bufbuild/protobuf`'s `create()` is mocked to `(_schema, init) => init` (line 12-14) — so
  assertions read plain-object shape (`msg.data.case`, `msg.data.value.cols`, etc.), not real
  protobuf instances.
- `@/gen/session/v1/events_pb` is mocked with plain classes matching each schema (lines 17-37).
  `TerminalInput`'s mock class only carries `data` (line 28) — a batched-input test can assert
  `msg.data.value.data` is the concatenated `Uint8Array`.
- `createTestOptions()` helper (42-63) builds `pushMessageRef`/`isConnectedRef`/`getTerminal`
  mocks — reused, not reimplemented, per test. Any new batching option (e.g. `inputBatchDelayMs`)
  should be threaded through this helper's `overrides` param, matching how `onError` etc. are
  already overridable.
- Existing `sendInput` tests (77-103) are minimal: one happy-path assert on `pushMessageFn`
  call count + `msg.sessionId`/`msg.data.case`, and one disconnected-no-op case. No existing
  test exercises the chunking path (`PASTE_CHUNK_SIZE` branch) at all — there is no fake-timer
  precedent specifically for `sendInput`'s `setTimeout(sendChunk, ...)` path to copy verbatim,
  but the `resize()` tests (105-294) extensively exercise `jest.advanceTimersByTime` +
  `act()` for `setTimeout`-driven deferred/throttled sends and are the closest pattern to
  mirror for the new batch-flush-timer tests.

## 3. Debounce-with-early-flush idiom — no new library needed

- **Existing convention in this exact file**: `pendingResizeTimerRef` (line 47) and
  `paneRequestTimerRef` (line 48) are both typed `useRef<ReturnType<typeof setTimeout> | null>(null)`,
  confirming `ReturnType<typeof setTimeout>` (not `NodeJS.Timeout` or `number`) is the
  established timer-ref typing convention in this file — used for both immediate-timer and
  deferred/throttled-timer refs (`pendingResizeTimerRef` doubles as both, see lines 209-212,
  283-287). A new `batchFlushTimerRef` should use the identical type.
- Timers are cleared on unmount via a `useEffect` cleanup (54-65) that nulls out both refs
  after `clearTimeout`. A new batch-flush timer needs the same unmount-cleanup treatment to
  satisfy AC5 ("flush on unmount/disconnect... no dropped keystrokes") — but note the cleanup
  effect as written only *cancels*, it doesn't *flush*; the new behavior needs an explicit
  flush call before/instead of a bare `clearTimeout` so buffered-but-unsent bytes aren't lost
  on unmount (existing resize-timer cleanup has no such requirement since a cancelled resize
  is fine to drop — batched keystrokes are not).
- **No debounce/throttle/buffering library is installed anywhere in `web-app/package.json`**
  (checked both `dependencies` and `devDependencies` — no `lodash`, `lodash.debounce`,
  `use-debounce`, `throttle-debounce`, `rxjs`, or similar). Combined with this repo's stated
  aversion to unnecessary dependencies (`.claude/rules/interface-pollution-checklist.md`'s
  broader "don't over-abstract" stance, and the fact every existing throttle/debounce-shaped
  behavior in this exact file — resize throttle, resync throttle, paste chunking — is hand-rolled
  with raw `useRef` + `setTimeout`, not a library), the idiomatic implementation here is a
  hand-rolled buffer using the same `useRef`/`useCallback`/`setTimeout` shape already used
  three times in this file. No new dependency is justified for what is a ~15-line buffer/flush
  function.
- Suggested shape (not prescriptive — Phase 3 planning owns final design): a
  `pendingBatchRef = useRef<Uint8Array[]>([])` (array-of-chunks, concatenated only at flush
  time — see §4) plus `batchFlushTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)`,
  a `flushBatch()` `useCallback` that concatenates and calls the existing single-send logic
  (factored out of the current `sendInput` body), and `sendInput` gaining an early branch: if
  `inputBatchDelayMs > 0`, push encoded bytes onto `pendingBatchRef`, flush immediately if the
  summed length crosses 32 bytes, otherwise (re)start `batchFlushTimerRef` if not already
  running (do not reset an in-flight timer on every keystroke — herdr-web's model and AC2's
  "within the delay window" phrasing both imply a fixed window from first buffered byte, not
  a sliding debounce that could starve under continuous typing).

## 4. Uint8Array concatenation — feasible, one existing precedent, no shared helper

- `TextEncoder`/`Uint8Array` are already used in this exact function (line 145-146) and are
  standard browser/Node globs — no polyfill or environment concern; the existing test file
  runs under `jest-environment-jsdom` (see `web-app/package.json` devDependency
  `jest-environment-jsdom: ^30.2.0`) and jsdom + modern Node both expose global `TextEncoder`,
  so the current tests already implicitly exercise this without issue.
- **No shared/reusable Uint8Array-concat helper exists anywhere in `web-app/src/lib`** (searched
  `src/lib` and `src/components` for `concat` and `Uint8Array` — zero hits for a named
  utility). The one concatenation precedent in the codebase is inlined, not extracted:
  `web-app/src/lib/transport/websocket-transport.ts:375-380`:
  ```ts
  function append(chunk: Uint8Array): void {
    const n = new Uint8Array(buffer.length + chunk.length);
    n.set(buffer);
    n.set(chunk, buffer.length);
    buffer = n;
  }
  ```
  This is the closest in-repo precedent for the batch-flush concat step, but it re-allocates
  and copies the *entire* accumulated buffer on every single chunk append (O(n²) for many
  small appends) — acceptable there because it's low-frequency (WS frame reassembly), but a
  bad model to copy directly for a per-keystroke hot path. The better approach for the
  batching buffer specifically: keep `pendingBatchRef.current` as an **array of `Uint8Array`
  chunks** (`Uint8Array[]`), pushing one small array per `sendInput` call (no copy), and only
  do a single `Uint8Array.set`-based concatenation pass at flush time (summing lengths once,
  allocating once, `.set()`-ing each chunk at its offset) — same total data movement as today
  (one encode per call) but avoids repeated reallocation between keystrokes. No new
  dependency needed; this is a ~10-line local helper, not import-worthy as a shared utility
  given it has exactly one call site.

## 5. Versions (from `web-app/package.json`)

| Tool | Version |
|---|---|
| React | `^19.0.0` |
| React DOM | `^19.0.0` |
| Next.js | `15.3.2` |
| TypeScript | `^5.9.3` |
| Jest | `^30.2.0` |
| jest-environment-jsdom | `^30.2.0` |
| ts-jest | `^29.4.11` |
| @testing-library/react | `^16.3.0` |
| @testing-library/jest-dom | `^6.9.1` |
| @bufbuild/protobuf | `^2.11.0` |
| @xterm/xterm | `^6.0.0` |
| Package manager | `pnpm@10.27.0` |

Testing framework is **Jest**, not Vitest — confirmed by `"test": "jest"` in `scripts` and the
`jest`/`jest-environment-jsdom`/`ts-jest` devDependencies; no `vitest` dependency present.

## Summary for Phase 3 planning

- No new npm dependency needed or justified — hand-roll the batch buffer with the same
  `useRef`/`useCallback`/`setTimeout` idiom already used 3x in this file (resize throttle,
  resync throttle, paste chunker), typed `ReturnType<typeof setTimeout>` per existing
  convention (lines 47-48).
- Reuse the session-change-abort pattern from the existing chunker (`sessionIdAtStart` capture
  + compare-at-fire-time) for AC5, rather than inventing a new abort mechanism.
- Store pending batch bytes as `Uint8Array[]` (array of chunks), concatenate once at flush
  time via a small local helper modeled on but not copy-pasted from
  `websocket-transport.ts:375-380` (that function's repeated-reallocation-per-chunk shape is
  the wrong model for a per-keystroke hot path).
- Unmount cleanup must flush pending bytes, not just `clearTimeout` — the existing
  `pendingResizeTimerRef`/`paneRequestTimerRef` cleanup effect (lines 54-65) only cancels,
  which is correct for resize but insufficient for AC5's no-dropped-keystrokes requirement.
- Test with Jest fake timers + RTL `renderHook`/`act`, extending `createTestOptions()` with an
  `inputBatchDelayMs` override; mirror the `resize()` describe block's fake-timer patterns
  (`jest.advanceTimersByTime` inside `act`) since `sendInput` currently has no fake-timer test
  precedent of its own to copy.
