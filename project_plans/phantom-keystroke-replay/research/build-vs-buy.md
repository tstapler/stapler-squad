# Research: Build vs. Buy — client-side reconnect hardening

Agent 6 (Build vs. Buy). Scope: the four remaining client-side deliverables —
epoch/generation guard in `useTerminalStream.ts`, drop-on-close fix in
`MessageQueue.ts`, a drop-and-signal UI badge, and Go/Jest regression tests.

## Method

- Read `web-app/package.json` dependencies/devDependencies in full.
- Read `web-app/src/lib/terminal/MessageQueue.ts` and
  `web-app/src/lib/hooks/useTerminalStream.ts` in full.
- Grepped the web-app tree for existing `epoch`/`generation` counters and
  `AbortController`-based reconnect guards to find reusable in-repo idioms
  (`usePathCompletions.ts`, `useWatchBacklogItems.ts`).
- Inspected the installed `@connectrpc/connect` / `@connectrpc/connect-web`
  packages (found in a sibling worktree's `node_modules`, since this
  worktree's `node_modules` isn't installed) for reconnect/idempotency
  primitives.
- Grepped for other bespoke async-iterable queues in the codebase
  (`watch-ws-transport.ts`'s `fromWebSocket`) to check for a reusable
  implementation or established local idiom.
- Checked for existing `aria-live` badge/indicator components as precedent
  for the new drop-and-signal UI element.

---

## Option 1: Third-party reconnect/state library (`use-websocket`, `SWR`, etc.)

**What it would replace:** the epoch/generation guard in
`useTerminalStream.ts`'s `connect()`.

### Pros
- `use-websocket` (react-use-websocket) and similar libraries bundle
  reconnect backoff, connection-state tracking, and some message-queueing
  out of the box — in principle less hand-rolled logic to maintain.

### Cons
- **Not a dependency today.** `web-app/package.json` has no `use-websocket`,
  `swr`, `react-query`/`@tanstack/query`, or any generic "connection
  manager" library. Adding one is a new dependency, not reuse of an existing
  one.
- None of these libraries model a **bidirectional ConnectRPC stream backed
  by an `AsyncIterable` producer** (`MessageQueue` passed into
  `client.streamTerminal(queue)`). They're built around plain `WebSocket`
  send/receive, not around supplying a client-managed async-iterable request
  stream to a typed RPC client. Adopting one would mean either forking the
  transport layer (`websocket-transport.ts`, already custom-built on
  `it-ws` + hand-rolled envelope framing) or running two parallel connection
  layers — one for the library's `WebSocket` lifecycle, one for the actual
  ConnectRPC stream. Neither is a net simplification.
- `useTerminalStream.ts` already has substantial custom logic that a
  generic library doesn't know about: the terminal state machine
  (`TerminalState`), flow control (`useTerminalFlowControl`), resize
  quiescence, scrollback fetch/replay, shell-status routing, and the
  existing `BackoffState` reconnect backoff (`lib/utils/backoff.ts`,
  already hand-rolled and working). A generic reconnect library would only
  ever cover a thin slice of this and still leave the epoch-guard problem
  unsolved for the other 90% of the hook.
- The actual code needed — a `useRef<number>` incremented once per
  `connect()` call, compared after the awaited work resolves — is ~4 lines,
  already implemented once in this exact codebase
  (`usePathCompletions.ts:107,122,152,168`).

### Verdict: **Not recommended.**
No existing dependency fits the ConnectRPC-stream-as-async-iterable shape,
adopting one would add a parallel connection-management layer instead of
removing complexity, and the in-repo generation-counter idiom already
solves this exact problem in ~4 lines with a proven pattern.

---

## Option 2: ConnectRPC's own client/transport (`@connectrpc/connect`,
`@connectrpc/connect-web`) — are reconnect/idempotency primitives being
under-used?

### Pros
- Already a project dependency (`^2.1.1` for both packages), so "using more
  of it" would be free in dependency-footprint terms.
- ConnectRPC protocol does define an `idempotency_level` `MethodOptions` for
  **unary** RPCs (`NO_SIDE_EFFECTS` — used to allow HTTP GET instead of
  POST for read-only calls).

### Cons
- Inspected `connect/dist/cjs/protocol-connect/transport.js` and
  `connect-web/dist/cjs/connect-transport.js` directly: the only
  `idempotency`-related code in either package is that unary
  `MethodOptions_IdempotencyLevel` check for GET-vs-POST request framing.
  There is **no** reconnect primitive, no de-duplication/replay-guard for
  streaming RPCs, and no built-in queue-lifecycle management anywhere in
  either package. `@connectrpc/connect-web`'s `createWebsocketBasedTransport`
  (used here via a project-custom wrapper, not the stock connect-web
  WebSocket transport) is a thin protocol/framing layer — it has no opinion
  about what happens to a half-sent request stream when the caller tears
  down and reconnects. That responsibility is explicitly left to the
  application, which is exactly why this project already hand-rolls
  `BackoffState`, `MessageQueue`, and the various `AbortController` +
  ref-guard patterns seen throughout `web-app/src/lib/hooks/`.
- The library's stated design boundary (transport/protocol only, no session
  semantics) means there is nothing "under-used" here to reach for — the
  gap isn't a missed feature, it's out of scope for what a ConnectRPC
  transport is.

### Verdict: **Not viable / not applicable.**
Confirmed by reading the installed package source: no reconnect or
streaming-idempotency primitive exists in `@connectrpc/connect` or
`@connectrpc/connect-web` to be under-using. The epoch-guard and
drop-on-close logic have to live in application code regardless of which
transport library is used.

---

## Option 3: Replace bespoke `MessageQueue.ts` with a queue library
(e.g. a generic async-iterable/backpressure queue package)

### Pros
- A well-tested library could offload edge cases (backpressure, multiple
  consumers, ordering guarantees) if the queue's responsibilities ever grow
  beyond "single producer, single `for await` consumer, FIFO, drop-on-close."

### Cons
- `MessageQueue.ts` is ~60 lines total and has exactly one job: bridge
  `push()` calls to a single `Symbol.asyncIterator` consumer for
  `client.streamTerminal(queue)`. The actual bug is a **one-line
  root-cause**: `close()` never clears `this.queue`, so the iterator's loop
  condition (`while (!this.closed || this.queue.length > 0)`) keeps
  draining and yielding already-buffered messages after `close()` is
  called. The fix is `this.queue = []` (or equivalent) inside `close()` —
  not a structural rewrite.
- The codebase already has a second, independent bespoke async-iterable
  queue with the identical array+resolve-callback shape:
  `watch-ws-transport.ts`'s `fromWebSocket()` (queue array, `notify`
  callback, `push()` helper). This confirms "small hand-rolled
  producer/consumer queue via array + resolver" is the established local
  idiom for bridging callback-driven data into `for await`, not an
  outlier that should be replaced with a dependency. (Note:
  `fromWebSocket` handles the *inbound* direction and has no drop-on-close
  concern of its own — it's not directly reusable for this fix, but it is
  evidence of the idiom.)
- No queue library is currently a dependency (checked `package.json` fully:
  no `p-queue`, no `async`, no generic FIFO/channel package). Adding one for
  a single-line fix is disproportionate — new dependency, new supply-chain
  surface, new API to wrap around ConnectRPC's `AsyncIterable` expectation,
  for a fix that's a one-line mutation plus a couple of Jest tests.
- Patching in place keeps the fix minimal and reviewable as a diff tied
  directly to the root cause described in `requirements.md`'s "Remaining
  confirmed gap" section.

### Verdict: **Recommended: patch in place, do not replace.**
The bug is a missing `queue = []` in `close()`; the class is already small,
single-purpose, and matches an established in-repo idiom
(`watch-ws-transport.ts`'s `fromWebSocket`). Swapping in a library would add
a dependency and an adapter layer to fix one line.

---

## Option 4: Reuse/fork an existing internal epoch/generation or
`AbortController`-based reconnect guard

### Pros
- `usePathCompletions.ts` already implements the exact epoch/generation
  pattern needed: a `useRef(0)` counter (`generationRef`), incremented once
  per operation (`const generation = ++generationRef.current`), and checked
  after each `await` boundary (`if (generation !== generationRef.current)
  return;`) at two points (success and error paths) — see
  `usePathCompletions.ts:107, 122, 152, 168`. This is a direct, proven
  precedent for guarding `useTerminalStream.ts`'s `connect()` against
  overlapping invocations (the exact race described in
  `requirements.md`: "rapid/triple reconnect can start a second stream loop
  before the first one's cleanup finishes").
- `useWatchBacklogItems.ts` (and `useReviewQueue.ts`, which it mirrors) show
  a second, complementary in-repo idiom: one `AbortController` scoped per
  connect-effect, with `signal.aborted` checks gating post-await logic and
  retry scheduling. `useTerminalStream.ts` already uses this same
  `AbortController`-per-`connect()` shape (`abortControllerRef.current = new
  AbortController()`), so that half of the guard is already present — what's
  missing is specifically the epoch counter to stop a second `connect()`
  call from starting a second stream loop before the first one's `finally`
  block (and its reconnect-scheduling) has run, since `isConnectingRef` /
  `isConnectedRef` alone are set/cleared at points that don't fully close
  the overlap window described in the requirements doc.
- Both reference implementations live in `web-app/src/lib/hooks/`, so no
  cross-boundary import/refactor is needed — the pattern can be copied
  directly into `useTerminalStream.ts` at the top of `connect()` and checked
  at the post-`await` points inside the message-processing IIFE and its
  `finally` block.

### Verdict: **Recommended: reuse the existing pattern, don't invent a new one.**
`usePathCompletions.ts`'s generation-ref idiom is the direct precedent named
in `requirements.md` itself ("No existing generation-counter idiom is
applied here, unlike `usePathCompletions.ts` which already uses one for a
similar race") and should be copied in with the same three touchpoints:
increment on entry, ref-array declared once, checked after every `await`
boundary that can leave a stale closure still running.

---

## Drop-and-signal UI badge — build vs. buy

Not one of the four numbered questions but part of the same scope; folded in
here since the verdict is one-sided.

- No `InputDropBadge` (or equivalent) exists yet in
  `web-app/src/components/sessions/`.
- Strong in-repo precedent for the shape this should take:
  `aria-live` regions already appear in `ConnectionIndicator.tsx` (both the
  `layout/` and `backlog/` variants), `TriageLoadingIndicator.tsx`, and
  others. Per this repo's CSS rules (`.claude/rules/css-architecture.md`),
  a new component must use vanilla-extract (`.css.ts`) colocated with the
  component, and must avoid inline `style={{ flexDirection: ... }}`-style
  layout overrides.
- This is inherently bespoke UI copy/markup tied to this app's specific
  drop-signal semantics ("input was dropped because the connection was
  superseded") — there is no third-party library question here at all; it's
  a small new component following the existing `ConnectionIndicator.tsx`
  pattern (assertive `aria-live="assertive"` region + visible badge).

### Verdict: **Recommended: build, following the existing `ConnectionIndicator.tsx` pattern.**

---

## Overall recommendation

Every piece of this remaining scope is small, has a direct in-repo
precedent, and does not fit any dependency already in `web-app/package.json`
or any primitive exposed by `@connectrpc/connect`/`@connectrpc/connect-web`.
Concretely:

| Deliverable | Verdict | Basis |
|---|---|---|
| Epoch/generation guard in `useTerminalStream.ts` | **Build — reuse `usePathCompletions.ts`'s generation-ref pattern** | Direct precedent, ~4 lines, same file type/directory |
| Drop-on-close fix in `MessageQueue.ts` | **Build — patch in place** | One-line root cause (`close()` doesn't clear `queue`); matches `watch-ws-transport.ts`'s bespoke-queue idiom |
| Drop-and-signal UI badge | **Build — follow `ConnectionIndicator.tsx`** | No existing component; strong aria-live precedent already in the codebase |
| Go/Jest regression tests | **Build** | Test-only work, not a library question |

No option evaluated (third-party reconnect library, ConnectRPC built-ins,
generic queue library) offers real leverage over what's already in the
codebase — in every case, adopting a library would mean bridging it onto
ConnectRPC's `AsyncIterable`-stream shape (which none of the candidates
model natively) while the in-repo alternative is either already proven
(`usePathCompletions.ts`'s generation ref) or a trivial, well-scoped patch
(`MessageQueue.ts`'s one-line fix). **Recommendation: build all four pieces
bespoke, reusing the two identified in-repo patterns rather than writing
either from scratch or reaching for a new dependency.**
