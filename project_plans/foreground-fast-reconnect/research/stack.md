# Research: Stack & Implementation Patterns — Foreground Fast Reconnect

## Versions in use

- Next.js 15.3.2, React ^19.0.0, TypeScript ^5.9.3 (`web-app/package.json`).
- `@connectrpc/connect` + `@connectrpc/connect-web`, custom WebSocket-based
  streaming transport at `web-app/src/lib/transport/websocket-transport.ts`
  (`createWebsocketBasedTransport`), built on `it-ws/client`.
- Test stack: Jest ^30.2.0 with `ts-jest` (not `@swc/jest` or Babel),
  `jest-environment-jsdom`, `@testing-library/react` ^16.3.0,
  `@testing-library/jest-dom` ^6.9.1, `@testing-library/user-event` ^14.5.2.
  Config: `web-app/jest.config.js` (multi-project; `web-app` project rooted at
  `src/`, `moduleNameMapper` handles `@/` alias and `.css.ts`).
- `useTerminalStream.test.ts` (`web-app/src/lib/hooks/__tests__/`) already
  exercises the `NEXT_PUBLIC_RECONNECT_V2` reconnect path with
  `jest.useFakeTimers()` / `jest.advanceTimersByTime(...)` inside
  `await act(async () => { ... })` — this is the established pattern to
  follow for the new fast-reconnect tests (see lines 382-732 of that file).

## No existing "connect-timeout race" utility — confirmed gap

Searched `web-app/src/lib` for `AbortSignal.timeout`, `raceWithTimeout`,
`withTimeout`, and any `*Timeout*` helper function: **zero matches** outside
test files and the one ConnectRPC-protocol `timeoutMs` plumbing described
below. `web-app/src/lib/utils/backoff.ts` (`BackoffState`, `jitteredDelay`,
`isRetriableCloseCode`, `getWsCloseCode`) is backoff-delay-only, as the
requirements doc states — no attempt-duration cap exists anywhere in this
hook or elsewhere in `web-app/src/lib`. The plan needs to introduce this
mechanism from scratch; there is no library dependency to add for it (no
`p-timeout`, no `abort-controller` polyfill needed — `AbortController` is a
standard browser/jsdom global already used at `useTerminalStream.ts:99,184`).

## Relevant existing pattern 1: `AbortController` already wired through connect()

`useTerminalStream.ts:99` (`abortControllerRef`) and `:184`
(`abortControllerRef.current = new AbortController()`) already create a
fresh `AbortController` per `connect()` call and pass `.signal` into
`clientRef.current.streamTerminal(messageQueueRef.current, { signal: ... })`
(`:209-212`). This is the natural hook point for a connect-timeout: a
`setTimeout` that calls `abortControllerRef.current?.abort()` if the stream
hasn't produced its first message within the timeout window, cleared once
`firstMessage` flips to `false` (`:219-227`). This mirrors the existing
disconnect-timeout pattern already in the file (`useTerminalStream.ts:393-397`,
a bare `setTimeout(...)` that force-aborts if disconnect doesn't resolve in
1000ms) — so a `setTimeout` + `abortControllerRef.current.abort()` pair,
cleared on first message, is idiomatic for this codebase and needs no new
abstraction beyond what's already used twice in this same file.

**Recommended shape** (idiomatic `Promise.race` is not actually the best fit
here): the stream is an `async for await` loop (`:220`), not a single
`Promise` being awaited, so `Promise.race([streamPromise, timeoutPromise])`
doesn't apply directly. The existing-codebase-idiomatic approach is a
`setTimeout` scheduled right after `abortControllerRef.current = new
AbortController()` (`:184`) that calls `.abort()` on the same controller if
`firstMessage` is still `true` when it fires, with the timer cleared inside
the `if (firstMessage) { ... }` block (`:221-227`). Aborting the controller
causes the `for await` loop to throw (it already handles abort via the
`signal` passed to `streamTerminal`), which flows into the existing
`catch (err)` / reconnect-scheduling block at `:313-353` — meaning the
connect-timeout's "abandon and retry" behavior falls out of the *existing*
error → backoff → reconnect pipeline for free, rather than requiring new
retry logic.

## Relevant existing pattern 2 (do NOT reuse directly): ConnectRPC's own `timeoutMs`

`websocket-transport.ts`'s `stream()` method (`:139-260`) already threads a
`timeoutMs` parameter through `runStreamingCall` (`:228-237`) and into the
Connect protocol timeout header (`:245-251`). This is **not** the right
mechanism for the foreground/background connect-timeout distinction:
`runStreamingCall`'s `timeoutMs` caps the entire RPC call duration end-to-end
(uses it to build a deadline header consumed connect-protocol-wide), but a
terminal stream is intentionally long-lived — it must stay open for the
session's lifetime, not just for the first N ms. Passing a short
`timeoutMs` into `streamTerminal(...)` would tear down an already-healthy,
long-running connection at that deadline, not just abandon a slow initial
handshake. The connect-timeout this feature needs is scoped to "time to
first message," which is a hook-level concern (guard around `firstMessage`),
not something `timeoutMs`/the transport layer can express. Confirmed by
reading `websocket-transport.ts:139-260` in full — no separate
"time-to-first-byte vs. total-call" timeout concept exists there either.

## Backoff/reconnect state to extend (not replace)

`terminalBackoffRef` (`BackoffState(1000, 30_000)`, `useTerminalStream.ts:108`)
is the single shared backoff instance for every reconnect, foreground or
background — Acceptance Criterion 3 ("foreground false→true resets backoff
attempt counter") is a `terminalBackoffRef.current.reset()` call, exactly
the same call already made in three existing places (`connect()` at `:166`,
the Story-3.1.3 visibility handler at `:442`, and `handleManualReconnect` at
`:465`) — no new state container needed, just a new call site gated on the
foreground transition.

## Which reconnect path(s) this applies to (Acceptance Criterion 5)

Per the requirements doc's "Current state" section (already verified against
this codebase) and confirmed again here: **only the `NEXT_PUBLIC_RECONNECT_V2`
hook-level path** (`useTerminalStream.ts`'s own reconnect scheduling at
`:331-352`, gated on that flag, and the Story 3.1.3 visibility listener at
`:433-458`, also gated on the same flag). The pre-flag legacy path in
`TerminalOutput.tsx` (`reconnectTimeoutRef`, ~lines 736-778 and 990-992) is a
separate, older reconnect implementation this feature does **not** touch —
`NEXT_PUBLIC_RECONNECT_V2` defaults OFF (`web-app/.env.local.example:1`), so
the fast-reconnect behavior is inert unless that flag is on, same as the
rest of the V2 reconnect logic it's extending.

## Dependencies needed: none

No new npm package is required. `AbortController`, `setTimeout`/`clearTimeout`,
and `Promise` are sufficient for a "cap on one connection attempt's
duration" — the existing `abortControllerRef` + `firstMessage` guard pattern
in `useTerminalStream.ts` is the natural implementation surface, extended
with a foreground-aware timeout value (~1200-1500ms for first 2 foreground
attempts vs. ~3500ms otherwise, per Acceptance Criterion 2) instead of a
hardcoded/absent one.
