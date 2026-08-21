# Research: Build vs. Buy — Foreground Fast Reconnect

Companion to `research/stack.md` (which already covers the implementation
hook-point and confirms no timeout mechanism exists today). This doc answers
the four build-vs-buy questions specifically.

## 1. Existing OSS library for "WebSocket reconnect with foreground/background-aware timeout policy"?

**No fit. Recommend against adopting one.**

- `web-app/package.json` dependencies/devDependencies (checked in full):
  zero hits for `reconnecting-websocket`, `p-retry`, `p-timeout`,
  `websocket-reconnector`, `rxjs`, or any retry/backoff/timeout package.
  The only relevant existing pieces are hand-rolled:
  `web-app/src/lib/utils/backoff.ts` (`BackoffState`, full-jitter delay,
  close-code classification).
- More fundamentally, the thing being reconnected here is **not a raw
  browser `WebSocket`** that a library like `reconnecting-websocket` could
  wrap. `useTerminalStream.ts` calls
  `clientRef.current.streamTerminal(messageQueueRef.current, { signal })`,
  a ConnectRPC bidi-streaming call running over a custom transport
  (`web-app/src/lib/transport/websocket-transport.ts`, built on `it-ws`,
  see `research/stack.md`). A generic WebSocket-reconnect library has
  nothing to attach to — the reconnect loop already lives at the
  ConnectRPC-call level, driven by an `async for await` loop over the
  stream, not by `ws.onclose`.
- `p-timeout`/`p-retry` are single-`Promise` wrappers. They don't fit either:
  the thing needing a timeout is "time to first message from a long-lived
  stream," not a call that resolves once. `research/stack.md` already found
  the idiomatic hook point is a `setTimeout` that calls
  `abortControllerRef.current.abort()` if `firstMessage` is still `true`,
  reusing the `AbortController` already threaded through `connect()` — a
  generic promise-timeout wrapper would just be a worse-fitting layer on
  top of infrastructure that's already there.
- The foreground/background *policy* piece (fast timeout for first N
  attempts, reset counter on transition) is inherently product-specific
  and stateful across the visibility lifecycle — no library encodes this
  concept generically; it is exactly the shape of herdr-web's own
  hand-rolled `terminalReconnectPolicy.ts` (see Q4).

**Verdict: build, don't buy.** Adding a dependency here would add an
adapter layer around `AbortController`/`setTimeout` (primitives already in
use twice in this file — see `research/stack.md`'s disconnect-timeout
precedent at `useTerminalStream.ts:393-397`) for no reduction in code and a
worse fit to the existing ConnectRPC-stream reconnect loop.

## 2. SaaS / managed API?

Not applicable, as expected. This is a purely client-side policy governing
when the browser tab retries its own ConnectRPC stream to `stapler-squad`'s
own backend — there is no third-party reconnect-as-a-service to delegate
to, and nothing here talks to an external network boundary that a managed
API could intermediate. Confirmed and moving on per the research brief.

## 3. Hand-written `withTimeout()` vs. an existing timeout-race utility already in the codebase

Searched `web-app/src/lib` and `web-app/src/components` for `Promise.race`,
`AbortSignal.timeout`, and `withTimeout`/`raceWithTimeout`-style helpers.

- **One existing `Promise.race` usage**, `web-app/src/app/config/ConfigPageContent.tsx:205-213`:
  ```ts
  const response = await Promise.race([
    clientRef.current.listClaudeConfigs({}),
    new Promise<never>((_, reject) =>
      setTimeout(() => reject(new Error("Timed out waiting for the server to respond.")), CONFIG_LOAD_TIMEOUT_MS)
    ),
  ]);
  ```
  This is a **single-call** timeout (load a config list once), not a
  long-lived stream, and it demonstrates exactly the kind of subtle bug the
  research question warns about: the `setTimeout` timer is never captured
  or `clearTimeout`'d after the race settles. If `listClaudeConfigs`
  resolves first, the timer is still live and will fire its reject
  callback later into a promise nobody is listening to — harmless here
  (V8/Node/browser GC the orphaned promise, and `reject` on an
  already-external no-op has no observable effect) but it is a real, live
  example in this codebase of the "cleanup is easy to get subtly wrong"
  pattern the research question anticipated.
- **Zero occurrences** of `AbortSignal.timeout` anywhere in `web-app/src`.
- **No shared `withTimeout()`/`raceWithTimeout()` utility exists** — there
  is nothing in `web-app/src/lib` to reuse.

**Recommendation: do not add a generic `Promise.race`-based `withTimeout()`
helper, and do not copy the `ConfigPageContent.tsx` pattern.** For this
specific use case, `research/stack.md` already identified a better-fitting,
leak-free mechanism that requires no new abstraction: reuse the
`AbortController` already created per `connect()` attempt
(`useTerminalStream.ts:184`) and schedule a plain `setTimeout` right after
it that calls `abortControllerRef.current?.abort()` if `firstMessage` is
still `true`, clearing that timer inside the existing
`if (firstMessage) { ... }` block (`:221-227`). This is safer than a
`Promise.race`/`withTimeout()` wrapper for three concrete reasons:
1. It has exactly one cleanup path (clear-on-first-message) instead of two
   (clear-on-resolve *and* clear-on-reject) — fewer branches to forget.
2. It reuses a signal the stream already honors for cancellation (passed
   into `streamTerminal(..., { signal })` at `:211`), so "timeout" and
   "user-initiated disconnect" funnel through the identical abort path
   instead of introducing a second, parallel cancellation mechanism.
3. It naturally falls into the existing `catch (err)` → backoff → reconnect
   pipeline (`:313-353`) when the abort fires, rather than requiring new
   error-classification logic to distinguish "timed out" from "stream
   errored" the way a `Promise.race` rejection would.

A generic `withTimeout()` helper would be justified only if a *second*,
structurally different call site needed the same race-a-promise-against-a-
timer shape. That doesn't exist in this codebase today (the one existing
usage is a single non-streaming RPC call, not a stream), so introducing one
now would be speculative — extend the `AbortController` pattern already
proven at this exact call site instead.

## 4. Fork/adapt herdr-web's `terminalReconnectPolicy.ts` — extend `BackoffState` or new sibling module?

herdr-web's file is referenced by name/constants only in the requirements
doc (`TERMINAL_FOREGROUND_CONNECT_TIMEOUT_MS=1200`,
`TERMINAL_CONNECT_TIMEOUT_MS=3500`, `TERMINAL_FOREGROUND_FAST_ATTEMPTS=2`);
it is not vendored into this repo (no `herdr` matches anywhere in the
tree), and it isn't reachable to inspect its actual license/implementation
from here — treat "adapt its exact constants/shape" as license-unverified
if herdr-web is a separate proprietary/private repo. The constants
themselves (three named numeric thresholds) are not copyrightable
expression in any meaningful sense — they're the acceptance criteria's own
numbers (see requirements.md AC2: "~1200-1500ms" / "~3500ms" / "first 2
attempts"), so there's nothing to "port" beyond re-deriving the same
values this task's own AC already specifies. **Recommend: do not import or
transcribe herdr-web source. Re-implement the policy shape locally against
this codebase's own `BackoffState`, using the AC's numbers directly.**

On **where** the new logic should live — extend `BackoffState` vs. a new
sibling module — recommend **extending `backoff.ts`**, not creating a new
file, for three reasons specific to this codebase:

1. **Single call site, single ref.** `terminalBackoffRef` is already the
   one shared piece of reconnect state per stream
   (`useTerminalStream.ts:108`), and AC3 ("foreground false→true resets
   backoff attempt counter") is naturally `terminalBackoffRef.current.reset()`
   — the exact same method `BackoffState` already exposes and that
   `connect()` already calls three times (`research/stack.md`, section
   "Backoff/reconnect state to extend"). A sibling module would need its
   own ref, its own reset call, and its own wiring into the same three
   call sites `BackoffState.reset()` already touches — pure duplication.
2. **The connect-timeout is a function of attempt number, exactly like the
   backoff delay already is.** `jitteredDelay(baseMs, capMs, attempt)` is a
   pure function of `attempt` in the same file; a
   `connectTimeoutMs(foreground, attempt)` pure function (or a method on
   `BackoffState` reading its own `.attempt`) is the same shape — inter-
   attempt delay and per-attempt duration cap are two properties of the
   same "attempt N" state, not two independent concerns.
3. **`backoff.ts` is already the file for exactly this class of pure,
   dependency-free policy helpers** (see `isRetriableCloseCode`,
   `getWsCloseCode` alongside `BackoffState`/`jitteredDelay`) — it has no
   React or ConnectRPC imports beyond `ConnectError` for close-code
   parsing, so it stays trivially unit-testable in isolation, matching AC6.
   A new sibling module would fragment one small, already-cohesive unit of
   policy logic across two files for no isolation benefit.

**Concrete shape recommended:** add a `foreground` flag/setter and a
`connectTimeoutMs()` accessor to `BackoffState` (or a small exported pure
function `connectTimeoutMs(foreground: boolean, attempt: number): number`
next to `jitteredDelay`, called with `terminalBackoffRef.current.attempt`)
using the AC2 constants:
```ts
const FOREGROUND_CONNECT_TIMEOUT_MS = 1200; // or up to 1500 per AC2's "~1200-1500ms"
const BACKGROUND_CONNECT_TIMEOUT_MS = 3500;
const FOREGROUND_FAST_ATTEMPTS = 2;
```
— named and scoped identically to herdr-web's constants (same values, same
intent) since those values are simply what AC2 specifies, not anything
that needs to be sourced from herdr-web's file directly.

## Summary recommendation

| Question | Answer |
|---|---|
| 1. OSS reconnect library | No — nothing fits a ConnectRPC-stream-over-custom-transport reconnect loop; extend hand-rolled `BackoffState`. |
| 2. SaaS/managed API | Not applicable — pure client-side policy against our own backend. |
| 3. Bespoke `withTimeout()` vs. reuse | Neither a new generic helper nor the existing `ConfigPageContent.tsx` `Promise.race` pattern (which already leaks an uncleared timer) — reuse the `AbortController` + `firstMessage` guard already wired through `connect()`. |
| 4. Fork herdr-web vs. extend `backoff.ts` | Extend `backoff.ts` in place; don't create a sibling module and don't transcribe herdr-web source — re-derive the same constants from this task's own AC2. |
