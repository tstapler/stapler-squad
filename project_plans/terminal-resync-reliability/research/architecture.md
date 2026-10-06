# Architecture research: terminal-resync-reliability

Agent 3 (Architecture), SDD Phase 2. Research question: what architectural
patterns apply (event-fan-out scoping, request correlation, admission
control, wire batching), what are the integration points with existing
systems, what are the data-flow/consistency requirements, and — as its own
category per this project's Complexity 4 calibration — what fails during a
live, flag-gated rollout of changes to a traffic-bearing feature.

Builds directly on two existing docs; **read those first**, this doc does
not re-derive their findings:

- `project_plans/terminal-visibility-resync/research/architecture.md` —
  state-ownership table, listener placement (`TerminalOutput.tsx`, not
  `useTerminalStream.ts`), the no-correlation-ID resync-completion heuristic
  and why it's sound today (single ordered `for await` stream, no
  reordering).
- `project_plans/terminal-input-batching/research/architecture.md` —
  `SessionDetailView.tsx` keep-alive pool keying
  (`SessionDetailView.tsx:697-701,775-784`), confirming every pooled
  session's `TerminalOutput` stays mounted and independently wired; the
  batching-design pattern (`pendingBytesRef`/`flushTimerRef`/extracted
  `sendBytes` helper) as an analog, not a template, for item 5.

Both docs describe the *client-side* shape of the feature as it existed at
their research time. Since then `useVisibilityResync.ts` has actually
shipped (it's a real file in the tree today, not a proposal) — this doc
reads its current implementation directly rather than re-deriving it from
the predecessor's proposal, and spends most of its new-ground effort on the
**server side**, which neither prior doc touched in depth: `exec_gate.go`,
`connectrpc_websocket.go`'s two independent streaming paths, and the
feature-flag precedents in `config/` and `server/services/`.

## 0. Headline finding not covered by prior research: two streaming paths, only one answers a mid-stream resync at all

This changes the root-cause story requirements.md tells, and it matters for
scoping item 2 and item 3(a), so it's called out before anything else.

`StreamTerminal` (`server/services/connectrpc_websocket.go:544-564`) routes
every **managed** session (i.e. essentially all Claude Code/Aider sessions —
the primary product surface per requirements.md's own Users section) to
`streamViaControlMode` by default:

```go
useControlMode := os.Getenv("STAPLER_SQUAD_USE_CONTROL_MODE")
if (useControlMode == "" || useControlMode == "true") && instance.Snapshot().IsManaged {
    return h.streamViaControlMode(stream, instance)   // :550, DEFAULT for managed sessions
}
...
return h.streamViaTmuxCapturePane(stream, instance, "")  // :564, fallback / external sessions
```

`streamViaControlMode`'s mid-connection input loop is `runInputReadLoop`
(`:1488-1560`, invoked as a goroutine at `:1021`). Its own comment states the
fact plainly:

```go
// Handle ScrollbackRequest ...
if scrollbackReq := incomingData.GetScrollbackRequest(); scrollbackReq != nil { ... }

// Note: CurrentPaneRequest is now handled in handshake (not in input loop)   // :1557
```

`runInputReadLoop` is called with exactly three callbacks — `onInput`,
`onResize`, `onScrollbackRequest` (`:1488-1495`) — there is no
`onCurrentPaneRequest`. **A `CurrentPaneRequest` sent mid-connection (which
is exactly what `requestFullResync()` sends —
`useTerminalFlowControl.ts:116-131`, `case: "currentPaneRequest"`) is parsed
by `runInputReadLoop`'s envelope/proto unmarshal and then silently
discarded** for any session on the control-mode path — no capture, no
resize, no response, nothing.

Contrast with the resize path in the *same* function: `onResize` feeds a
coalescing goroutine (`:923-1020`) that, after `SetWindowSize` and a
`waitForQuiescence` wait, does an explicit **forced fresh capture and
push** (`:984` onward) — this is why "resize" always produces a real
response and "resync" (a bare `CurrentPaneRequest` with no accompanying
`TerminalResize`) does not, on this path.

`streamViaTmuxCapturePane`'s own inline read loop, by contrast, **does**
handle `CurrentPaneRequest` mid-stream (`:1966-2049`) — this is the handler
requirements.md describes as taking "an expensive ~450ms+ slow path": on a
dimension mismatch (`:1984`) it resizes, sends 3 `RefreshTmuxClient` SIGWINCH
nudges 100ms apart (`:1997-2005`, ≈200ms), then sleeps a fixed 250ms
(`:2011`) before capturing — ≈450-550ms confirmed by reading the actual
sleep durations, not estimated.

**Implication:** the two mid-stream behaviors aren't "one handler, sometimes
slow" — they're "one real (if slow) handler, and one path with no handler at
all." For the default control-mode path, a resync's "success" today is
already governed entirely by luck (does *some* unrelated output arrive
before `RESYNC_STALL_TIMEOUT_MS`), matching the bug report's own framing —
"it's usually not the one I'm typing in" — even more literally than
requirements.md's exec-gate-contention framing suggests: an idle background
terminal on the default streaming path isn't *queued behind* other resync
work, it has **no pending work answering it at all** and is waiting purely
on ambient traffic.

This reframes items 2 and 3(a):

- **Item 2 (correlation ID)** cannot be "add an ID field, echo it in the
  existing handler" for control-mode sessions — there is no existing
  handler on that path to add an ID to. It requires **building** a
  mid-stream `CurrentPaneRequest` handler for `streamViaControlMode`,
  modeled on the resize path's existing "resize (if needed) → wait for
  quiescence → forced capture → push" shape (`:947-1020`), not modifying
  `streamViaTmuxCapturePane`'s handler alone.
- **Item 3(a) (skip the dimension-mismatch slow path for backgrounded
  clients)** as literally scoped only touches `streamViaTmuxCapturePane`
  (`:1966-2049`) and `streamViaControlMode`'s handshake-time nudge
  (`:660-700`) — it does not, by itself, fix the "no response at all" gap on
  the control-mode mid-stream path, which is a correctness gap, not a
  latency one. Phase 3 planning should treat "give control-mode sessions a
  real mid-stream resync handler" as a load-bearing prerequisite of item 2,
  not an optional refinement — without it, correlation IDs have nothing to
  correlate against for the majority of sessions, and success metric #1
  ("stall-watchdog fires drop to near-zero for backgrounded terminals")
  cannot be met for control-mode sessions no matter how well items 1/3/4/5
  are built, since those idle terminals will keep hitting the 4s watchdog
  by construction.

## 1. Scope resync to visibility (item 1)

**Pattern: gate at the trigger, not the transport.** The existing
`isVisible`/`foreground` prop is real, but only reaches one consumer today —
`useTerminalStream.ts` reads it (as `foreground`) purely to pick a
reconnect-timeout value (`useTerminalStream.ts:125-203`,
`connectTimeoutMs(foregroundAtSchedule, ...)` at `:284`). It is **not**
threaded into `useVisibilityResync.ts` at all: that hook's params
(`UseVisibilityResyncParams`, `useVisibilityResync.ts:12-27`) list
`sessionId, isConnected, terminalState, connect, disconnect,
requestFullResync, markResyncComplete, markPaneResponseReceived,
setShowReconnectButton, setShowReconnectBanner` — no visibility flag. So
"cosmetic-only" in requirements.md is precise about the one place that
matters: the resync-storm trigger path never consults it.

Integration point: `TerminalOutput.tsx:535` already computes `foreground:
isVisible` for the `useTerminalStream` call; the same `isVisible` value is
already a component-scope variable at that point, so passing it into the
`useVisibilityResync` call three lines below (`:538-549`) is a same-file,
zero-new-plumbing addition — consistent with the predecessor doc's finding
that `TerminalOutput.tsx` is where this class of decision belongs. Inside
`useVisibilityResync.ts`, the natural gate point is the top of
`handleVisibilityOrFocusResyncInner` (`:108`) — a `document.visibilityState
!== 'visible'` early-return already exists there for the *document's*
visibility; add an `isVisible` (this terminal's own foreground state, not
the document's) check alongside it. **Do not conflate the two**: a mounted
background terminal has `document.visibilityState === 'visible'` (the tab
itself is foregrounded) but its own `isVisible` prop is false (some other
session/tab is the active one in the pool) — the whole bug is that today's
code only checks the former.

**Failure mode to design against:** the visibility→resync scoping change
must not silently break the *other* existing trigger inside the same
function — the disconnected-branch fallback (`:161-167`, `else` branch:
"Don't take the disconnected fallback mid-handshake... connectRef.current();
setShowReconnectButtonRef.current(true)"). That branch already has nothing
to do with resync-storm scoping (a disconnected background terminal
reconnecting is not the problem this project targets), so the visibility
gate must wrap only the `isConnected` branch (`:120-160`), not the whole
`handleVisibilityOrFocusResyncInner` body — gating the function entry would
also suppress the legitimate disconnected-terminal auto-reconnect for
background terminals, which is out of scope to change.

## 2. Correlation ID (item 2)

**Pattern: request/response tagging over an already-ordered channel.** The
predecessor doc's soundness argument for the no-correlation-ID heuristic
(single `for await` loop, no reordering) is *why* a correlation ID is safe
to add incrementally rather than a rewrite: ordering was never the problem,
attribution was. Concretely:

- **Proto surface**: `CurrentPaneRequest` (`proto/session/v1/events.proto:195-214`)
  has no request-ID field today, and neither does `TerminalOutput`
  (`events.proto:124-126`, just `bytes data = 1`) — the response message
  that both ambient streamed output and resync responses arrive as
  (confirmed by the predecessor doc and independently by this research: both
  `streamViaControlMode`'s forced-snapshot push and
  `streamViaTmuxCapturePane`'s `CurrentPaneRequest` handler wrap their
  payload in the identical `TerminalData_Output{Output: &TerminalOutput{Data: ...}}`
  shape). Add an `optional string resync_id` (or `uint64`) to
  `CurrentPaneRequest` (client-generated, e.g. a monotonic counter or
  `crypto.randomUUID()`) and a matching `optional string resync_id` to
  `TerminalOutput`, populated **only** on responses the server knows are
  answering a `CurrentPaneRequest** — never on ambient output. No existing
  request-ID/correlation-ID convention exists elsewhere in
  `proto/session/v1/*.proto` to match against (grepped, no hits) — this is a
  new pattern for the schema, but proto3 `optional` scalar semantics (a
  presence bit, not just a zero-value) mean an absent ID is distinguishable
  from an empty one, so ambient output naturally decodes as "no correlation
  ID," requiring no separate discriminator field.
- **Client-side plumbing**: `useTerminalFlowControl.ts`'s `requestFullResync`
  (`:80-135`) is the single place the request is built
  (`create(CurrentPaneRequestSchema, {...})`, `:116-121`) — generate and
  stash the ID there, alongside the existing `lastResyncTimeRef`/
  `isResyncingRef`/`waitingForPaneResponseRef` refs (`useTerminalFlowControl.ts:42-49`
  per the predecessor's ownership table). `notifyResyncOutputReceived`
  (`useVisibilityResync.ts:200-219`) is the natural place to *check* the ID
  against the pending one before treating an output message as completion —
  but note `notifyResyncOutputReceived` today has no visibility into the
  output payload at all, only that "an output arrived" (it's invoked from
  `TerminalOutput.tsx:466` inside `handleOutput`, decoupled from the actual
  bytes). Wiring the ID through means `handleOutput` must extract
  `output.resyncId` (if the field is now populated) and pass it into
  `notifyResyncOutputReceived(resyncId?)`, which then compares against its
  own pending ID ref before clearing — this is the one new piece of data
  flow this item requires beyond proto/server changes.
- **Two-state-machine risk, addressed directly**: requirements.md's own
  Rabbit Holes section flags `useTerminalFlowControl` vs
  `useVisibilityResync` as "redundantly-overlapping." Concretely,
  `isResyncingRef`/`waitingForPaneResponseRef` (owned by
  `useTerminalFlowControl`) and `pendingResyncCompletionRef` (owned by
  `useVisibilityResync.ts:52`) are **two separate booleans tracking
  overlapping "a resync is in flight" state today**, both cleared by the
  same `notifyResyncOutputReceived`/`markResyncComplete`/
  `markPaneResponseReceived` call sequence (`useVisibilityResync.ts:200-219`,
  `useVisibilityResync.ts:139-140`). Adding a correlation ID is the point at
  which it becomes tempting to also collapse these — resist that per the
  requirements' own explicit guidance; the ID only needs to live on
  whichever side generates the request (`useTerminalFlowControl`) and
  whichever side detects the response (`useVisibilityResync`), passed as a
  value between them the same way `requestFullResync`/`markResyncComplete`/
  `markPaneResponseReceived` already cross that boundary as function
  references (`TerminalOutput.tsx:538-549`).
- **Server-side echo**: for `streamViaTmuxCapturePane`'s existing handler
  (`:1966-2049`), echoing the ID is a pure addition — read
  `currentPaneReq.ResyncId` at `:1967`, set it on the `TerminalOutput` built
  at `:2034` (`fullContent := clearAndHome + content`). For
  `streamViaControlMode`, per §0 above, there is no handler to add the echo
  to yet — building one (modeled on the resize path's `:947-1020` shape:
  optionally resize if dimensions actually differ, wait for quiescence,
  force a fresh capture, push with the echoed ID) is a prerequisite, not an
  extension.

## 3. Server-side capacity fixes (item 3)

**3(a) — skip the slow path for backgrounded-stale dimensions.** Pattern:
**provenance-tagged input** — the server cannot itself distinguish "the
client's reported dimensions are stale because the tab was backgrounded"
from "the client genuinely resized" using only `(targetCols, targetRows)` at
`:1973-1984` or `:660-662`; both look identical on the wire today. This is
exactly the open question requirements.md's own Feasibility Risks section
names ("may require new client-to-server information"). Two designs, in
increasing order of invasiveness:

  1. **Client asserts staleness directly.** Since item 1 already threads
     `isVisible` into the resync trigger, the same boolean is available at
     the point `requestFullResync`/the resize path builds its
     `CurrentPaneRequest`/`TerminalResize` — add a `stale_dimensions: bool`
     (or reuse a `foreground: bool`) field the client sets when it knows its
     own reported cols/rows haven't been reconciled against a real layout
     pass since backgrounding (e.g. no `ResizeObserver` fit has run since
     the last `visibilitychange` to hidden). Server-side, when that flag is
     set, skip straight to using its own last-known-good pane dimensions
     instead of the client's reported ones, bypassing `:1973-1984`'s
     mismatch branch (and hence the ≈450ms nudge sequence) entirely for
     that request. This is the cheapest option architecturally — no new
     round trip, one new proto field — but trusts client self-reporting.
  2. **Server infers staleness from request cadence**, e.g. "a
     `CurrentPaneRequest` arriving within N ms of a `visibilitychange`
     resync trigger is presumptively a background-catch-up, treat dimension
     mismatch as advisory only." This avoids a new client-trust surface but
     requires the server to track per-session recent-resync timestamps it
     doesn't currently keep (nothing in `connectrpc_websocket.go` currently
     timestamps `CurrentPaneRequest` arrivals) — more state, more edge cases
     (what if the client is on a slow network and the "recent" resync
     request arrives late).

  Recommend (1) for Phase 3: smallest diff, and the client already computes
  the relevant fact for item 1's own gating, so it costs nothing extra to
  compute — it's a **derivation** of an existing signal, not new
  client-side logic.

**3(b) — exec-gate capacity.** `session/tmux/exec_gate.go:41-115` is a
straightforward **admission-control / bulkhead** pattern: N `flock`-backed
slot files per gate-key directory (`gateDir`, `:97-115`), acquired via
randomized-order try-lock with backoff (`acquireSlot`, `:117-150`), one
release-once closure per acquisition. `defaultTmuxExecGateSlots = 8`
(`config/types.go:96`), and the key is `"default"` whenever
`serverSocket == ""` (`exec_gate.go:106-109`) — which, per requirements.md,
is virtually every session's case today, meaning **all** tmux subprocess
work (resync captures, resizes, session creation, health polling, anything
else that calls `AcquireExecSlot`/`runGated`) contends for the same 8 slots
process-wide.

Two independent architectural levers, matching requirements.md's own open
question ("does capacity increase need a new gate key... rather than raising
the shared default count"):

  - **Raise `defaultTmuxExecGateSlots` globally** — zero code change beyond
    the constant (or, better, make it configurable via the existing
    `TmuxExecGateConfig.Slots` field, which already exists precisely for
    this: `config/types.go:88-92`, `SlotsOrDefault()` at `:101-106`, already
    wired through `AcquireExecSlot`/`TryAcquireExecSlot`
    (`exec_gate.go:46,63`) with no server restart needed since
    `appconfig.LoadConfig()` is called fresh on every acquisition). This is
    the "blast radius" risk requirements.md names: raising it helps resync
    bursts but also raises the ceiling for every other concurrent tmux
    subprocess class, diluting the single-threaded-tmux-server protection
    the gate exists for (`exec_gate.go:35-40`'s own doc comment).
  - **A separate, resync-scoped gate key** — `gateDir`'s key derivation
    (`:106-109`) already supports this for free: pass a synthetic
    per-purpose string (e.g. `serverSocket + "#resync"`) instead of the bare
    socket name when acquiring a slot specifically for resync-triggered tmux
    calls, and it lands in its own `tmux-exec-gate/<key>/` directory with
    its own independent slot count — architecturally this is **the same
    bulkhead pattern applied at finer granularity**, not a new mechanism.
    This directly answers the "new gate key vs. raise global count" open
    question: a dedicated `#resync` key with its own (likely small, e.g.
    2-4 slot) budget is strictly safer than a global raise, because it
    can't be starved by, nor starve, unrelated tmux work — a resync burst
    maxes out its own small pool and queues internally (which is exactly
    what item 4's staggering wants anyway) without touching the budget
    session-creation or other tmux calls depend on. The cost is that
    `AcquireExecSlot`'s signature (`ctx, serverSocket string`) would need a
    third parameter or a wrapper (`AcquireResyncExecSlot`) to pass the
    distinguishing key — worth flagging against
    `.claude/rules/primitive-obsession-checklist.md`: if this grows to more
    than two purpose-scoped gate variants, a `GateKey` value type
    (`GateKey{Socket, Purpose string}`) is cheap insurance against a future
    same-typed-string mix-up, rather than adding a fourth bare `string`
    parameter to `AcquireExecSlot`.

  Recommend the dedicated-key fast lane for Phase 3: it's additive (existing
  callers' `serverSocket`-only key derivation is untouched), independently
  tunable, and directly serves item 4's staggering goal by giving
  resync-burst work its own queue depth to observe and stagger against
  (see Observability, below) instead of an opaque shared counter.

## 4. Stagger/prioritize resync bursts (item 4)

**Pattern: client-side request shaping in front of a shared resource**,
same family as item 5's batching but simpler — this item reduces *when*
requests are sent, not how many bytes each carries. Two independent
mechanisms, composable:

  - **Staggering**: since every mounted `TerminalOutput` instance in a
    pooled session independently registers its own `visibilitychange`/
    `focus` listener (`useVisibilityResync.ts:176-183`, one listener per
    instance, confirmed architecturally by the input-batching doc's
    keep-alive-pool finding — each pooled instance is a fully independent
    hook tree), a single document-level event firing N times has no
    natural serialization point today. The debounce that already exists
    (`useDebouncedCallback`, `RESYNC_DEBOUNCE_MS = 300`,
    `useVisibilityResync.ts:5,174`) coalesces **repeated** events on *one*
    instance; it does nothing to spread N *simultaneous first* fires
    across N instances, because each instance's debounce timer is
    independent (they all fire at t+300ms together, not sequentially).
    Introducing an explicit stagger requires either (a) a small
    per-instance random or index-based jitter added to the delay before
    `requestFullResync(true)` fires (cheapest, no cross-instance
    coordination, trades determinism for simplicity), or (b) true
    cross-instance sequencing via a shared coordinator. (b) is a
    materially bigger architectural lift — it needs a registry all pooled
    `TerminalOutput`/`useVisibilityResync` instances register into (new
    module-level or context-provided state, crossing the "no new hook, no
    new file" preference the input-batching doc established for
    same-instance state) purely to answer "how many other terminals in
    this pool are also mid-resync right now." Given the Large-but-bounded
    appetite and this project's own Rabbit Hole warning against
    "collapsing the two state machines," (a) — cheap per-instance jitter —
    is the better-scoped default; recommend Phase 3 treat true
    cross-instance coordination as a stretch goal, not baseline scope.
  - **Prioritization by visibility/recency**: this composes naturally with
    item 1's `isVisible` gate — once resync is scoped to visible terminals
    only (item 1), "prioritize the foreground terminal" is largely already
    true by construction (only it fires at all). The remaining
    prioritization surface is *among* the resync-eligible slice when more
    than one terminal in a pool can legitimately be `isVisible` (browser tab
    focus events fire for the whole document, not per-terminal — see item
    1's distinction between document visibility and per-terminal
    `isVisible`/foreground prop). A simple recency signal (e.g. `lastViewed`
    already exists server-side per-instance via `MarkViewed()`, called at
    `connectrpc_websocket.go:596,689,1092` on every stream start) could
    inform which terminal's resync gets first crack at a scarce
    fast-lane exec-gate slot (item 3(b)), but this is genuinely the least
    concretely scoped item per requirements.md's own Rabbit Holes section —
    Phase 3 should decide whether visibility-only gating (item 1) already
    satisfies this item's intent well enough that explicit recency
    prioritization can be deferred.

## 5. Batch updates / wire protocol redesign (item 5)

Requirements.md explicitly defers the mechanism to this phase. Given §0's
finding, the highest-leverage design is not primarily about compressing
*existing* per-terminal resync responses, but about **whether a resync
response can be shared across multiple pooled terminals that are resyncing
the same underlying tmux session** — a scenario the input-batching precedent
doesn't cover (that project batches per-hook-instance *keystroke input*,
which is inherently per-session and unshareable; resync *output* is a read
of shared server-side state and is a genuinely different sharing
opportunity).

  - **Same-session sharing**: `SessionDetailView.tsx`'s pool keys every
    `TerminalOutput` by `poolId`/`session.id`
    (`SessionDetailView.tsx:697-701,775-784`, per the input-batching doc's
    citation) — but a single logical session can still have multiple
    mounted terminal *views* of it in some layouts (e.g. shell tabs
    alongside the main terminal, per `connectrpc_websocket.go:507-524`'s
    shell-vs-main distinction) that are genuinely different tmux
    targets (main PTY vs. sibling shell session) and cannot share a
    resync response — so same-session sharing is a narrower opportunity
    than "share across the whole pool," bounded to true duplicate views of
    the identical tmux pane, which may not be common enough in practice to
    justify the complexity. Flag this as a design question for Phase 3
    rather than assume it's a large win.
  - **Batching at the transport-envelope level**: multiple terminals'
    resync requests, each already reduced to a small `CurrentPaneRequest`
    (this ConnectRPC handler is a single WebSocket connection *per
    session* — `StreamTerminal` is called once per `sessionId`
    (`connectrpc_websocket.go:496-497`), so "batching multiple terminals'
    resyncs into fewer round trips" cannot mean multiplexing requests onto
    one WebSocket connection (there is no shared connection across
    sessions today — confirmed by `resolveSession(sessionID)` being called
    per-`StreamTerminal`-invocation, one per session's own socket). Item 5's
    "fewer round trips" must therefore mean something narrower than
    cross-session multiplexing: either (a) compressing the existing
    per-connection resync payload itself (the pane content bytes — gzip/
    zstd over the WebSocket frame, or a diff against the last-sent snapshot
    instead of a full repaint each time), or (b) reducing round-trips
    *within* a single resync's own protocol exchange (e.g. today's
    nudge-then-verify-then-capture sequence in
    `streamViaTmuxCapturePane`(`:1973-2025`) is itself multiple sequential
    tmux calls before one client-visible response — collapsing that
    server-internal sequence, not the client-server wire exchange, is a
    "fewer round trips" win that doesn't require any new client-visible
    protocol surface at all).
  - Recommend Phase 3 treat (a)/compression as the concretely-buildable
    default (a real wire-bytes reduction, measurable per the success
    metrics, implementable as a header/flag on the existing envelope format
    without new proto messages) and treat cross-terminal request batching
    as the rabbit hole requirements.md warns about — the "one WebSocket per
    session" architecture makes true multi-terminal request batching a
    much bigger lift (a new fan-in layer above `StreamTerminal`) than the
    appetite likely affords, whereas compression and server-internal
    round-trip reduction are additive, connection-shaped changes.

## Data flow and consistency requirements

**Must every visible terminal eventually get a correct, corruption-free
buffer, even under staggering/batching?** Yes, and the mechanism that
already guarantees this — the ANSI-embedded reset
(`ansiSnapshotPrefix`/`clearAndHome`, per the predecessor doc's §4) — is
orthogonal to *when* or *how batched* the request is: whatever triggers a
fresh capture-and-push still wraps it in the same DECSTR+ED2+CUP prefix
before handing it to xterm.js's own VT parser for the actual repaint. None
of items 1-5 touch that payload shape; they only change trigger scoping
(1), attribution (2), server admission (3), timing (4), and encoding (5) of
the *request path* to get there. So the correctness guarantee survives by
construction **as long as every eligible terminal's resync request
eventually reaches a real handler that performs the capture-and-push** —
which is precisely the gap §0 identifies for control-mode sessions today.
Staggering (item 4) makes this *slower* for some terminals (by design) but
must not make it non-terminating: every deferred/staggered resync needs its
own bounded completion path (the existing `RESYNC_STALL_TIMEOUT_MS` watchdog
already provides an upper bound today, and should remain the backstop even
after staggering is introduced — a staggered request that never fires
should still eventually trip the watchdog and fall back to disconnect
+reconnect, not hang silently past 4s indefinitely).

**Consistency under the fast-lane/dedicated-gate design (3b):** a
resync-scoped exec-gate key must not let resync work starve *itself* into
the same stall watchdog it's meant to relieve — if the dedicated gate's slot
count is too small relative to a burst's size, resyncs still queue, just in
a smaller, isolated line instead of behind unrelated tmux work. This is a
tuning question (slot count vs. typical N-mounted-terminals-per-session),
not a correctness one, but worth an explicit metric (queue depth on the
resync-specific key, distinct from the existing `"default"` key) so Phase 3
can right-size it empirically rather than guessing — this maps directly to
requirements.md's own Observability Requirements ("exec-gate wait
time/queue depth on `default` key," which should be split per-key once a
dedicated key exists).

## Migration / live-rollout failure modes (Complexity 4 — evaluated separately)

This is a shipped, traffic-bearing feature (`96990ce12`, PR #184, per
requirements.md's Problem Statement). Each of the four failure classes
below is evaluated against this repo's actual flag mechanics, not
generically.

### Partial rollout (mixed flag state across instances during a rolling deploy)

This repo's feature-flag storage is **per-instance config file state**, not
a centrally-coordinated rollout service: `Config.FeatureFlags` is a
`map[string]bool` persisted to that instance's own `config.json`
(`config/config.go:332-335`, `GetFeatureFlag`/`SetFeatureFlag` at
`:1037-1054`). There is no cross-instance flag-sync mechanism in this
codebase (confirmed: no references to a shared flag store, feature-flag
service, or LaunchDarkly/Unleash-style external client anywhere in
`config/` or `server/`). Practically, "some instances flag-on, some
flag-off during a rolling deploy" is not a distributed-consistency problem
here — it's a **single-user-facing-instance-at-a-time** deployment model
(per `CLAUDE.md`'s own description: `make install-service` restarts *the*
running systemd unit; there is no fleet of stateless replicas behind a load
balancer implied anywhere in the docs). The realistic partial-rollout
scenario is narrower than the "N replicas, mixed versions" case the prompt
raises: it's **one client connection that was established against the
pre-deploy binary, still open when `make install-service` restarts the
service mid-session** — but `CLAUDE.md`'s own WARNING already documents
that a restart "kills the tmux server and every live tmux session with it"
(unless `--tmux-keep-server` is passed), meaning today, a deploy already
force-closes every WebSocket connection outright, closing this window by
accident rather than by design. If a future deploy adopts
`--tmux-keep-server` more broadly (already the documented recommendation),
this stops being true and the mid-session flag-flip scenario below becomes
live — so the two scenarios converge into one design requirement either way.

### Flag flip mid-session (WebSocket open across a flag change)

The load-bearing question: is `GetFeatureFlag` read once (at connection
start, cached for the connection's life) or on every relevant decision
point? Confirmed by reading `NewFeatureFlagInterceptor`'s own doc comment
(`server/interceptors/feature_flag_interceptor.go:16-19`): *"isEnabled is
called on every request so flag changes are reflected immediately without a
server restart"* — this is the established convention (see also
`workspacePeersBlockFor`, `server/services/feature_flag_service.go:36-40`,
which calls `config.LoadConfig().GetFeatureFlag(...)` fresh on every
invocation, not once at startup). **If this project's resync-reliability
flag follows the same convention** (recommended — it's the only precedent
in the codebase), a flag flip mid-connection means:

- `streamViaControlMode`/`streamViaTmuxCapturePane` read the flag fresh on
  the **next** `CurrentPaneRequest` they process, not the one already
  in-flight when the flip happened — so a resync request built under
  old-flag client behavior could arrive at a new-flag-reading server (or
  vice versa) within the same connection. Concretely: client sends a
  `CurrentPaneRequest` **without** a `resync_id` (built before the client
  process picked up a flag flip — client-side flags are typically baked
  into a page load via `NEXT_PUBLIC_*` env vars, per the existing
  `NEXT_PUBLIC_RECONNECT_V2` precedent, which do **not** support live
  flipping without a page reload, unlike the server's per-request
  `GetFeatureFlag`), and a server that has just flipped to "flag on" tries
  to read `currentPaneReq.ResyncId` and gets the empty/absent value — this
  must be handled as "no correlation, fall back to today's next-output
  heuristic," never as an error. **Design requirement**: any correlation-ID
  read on the server must treat an absent ID as a valid, expected state
  (proto3 `optional` gives this for free — an absent field, not a garbage
  default), not a protocol violation.
- The reverse (client sends a `resync_id`, server hasn't picked up the flag
  flip yet and doesn't echo it) must equally degrade gracefully: the
  client's correlation check should be "if I have a pending ID and the
  response carries no ID, don't treat that as a hard mismatch that discards
  a real completion" — falling back to the old undifferentiated-heuristic
  behavior for that one response, not blocking forever waiting for an ID
  that will never come. This is the direct client-side mirror of the
  server-side requirement above.
- Because `NEXT_PUBLIC_*` flags are baked in at build/page-load time (not
  live-reloadable) while server flags via `GetFeatureFlag` are live, **the
  two sides of this feature can genuinely disagree on flag state for the
  lifetime of an open tab**, independent of any rolling-deploy timing —
  this is a normal, expected steady state under this architecture, not just
  a migration-window edge case, and the design must tolerate it
  indefinitely, not just "during the flip."

### Rollback safety (flag flipped back off)

Given `GetFeatureFlag` is read live and per-request (not cached for a
connection's lifetime), rollback is architecturally the same event as a
forward flip, just in the other direction — the graceful-degradation
requirements above are symmetric and cover it, **provided** every new
behavior gated by the flag has a well-defined "flag off" fallback that
exactly reproduces pre-project behavior, not a new third state. Concretely
for each item:

- Item 1 (visibility scoping): off = today's document-visibility-only gate,
  unconditionally. No new state introduced by turning it off.
- Item 2 (correlation ID): off = server never populates `resync_id` on
  responses, client never reads it, falls back to the pure next-output
  heuristic — safe because the field is `optional` and its absence is
  already the graceful-degradation path designed above, not a special
  rollback code path. **This is a specific, useful property of doing the
  correlation-ID rollout as "populate an optional field" rather than
  "replace the heuristic with a hard requirement for a matching ID"** — a
  hard-requires-a-match design would strand any in-flight resync that
  started under flag-on if the flag flipped off mid-wait (client waiting
  for an ID match that a now-flag-off server will never send); the
  soft/optional design instead has such a resync fall through to "next
  output completes it" exactly as today, no strand possible.
- Item 3(b) (dedicated exec-gate key): off = `AcquireExecSlot` reverts to
  the `"default"` key for resync-triggered acquisitions too — since
  `gateDir`'s key derivation is a pure function of the string passed in
  (`exec_gate.go:106-109`), flipping the flag just changes which string
  gets passed at the call site; no state to unwind (lock files for an
  unused key are simply never touched again, not a leak — `flock` files
  are just filesystem markers with no server-side registration to clean
  up).
- Item 4 (staggering) / item 5 (batching/compression): off = fall back to
  synchronous, uncompressed behavior. The one rollback hazard worth naming:
  if item 5 introduces a **new wire encoding** (e.g. a compression flag bit
  on the envelope, or a batched-response message shape), a client built
  under flag-on that has cached/queued a batched-format response must not
  misinterpret it after a server-side rollback mid-connection flips the
  server back to unbatched — same "read per-request, not per-connection"
  discipline applies, and the encoding choice should be self-describing on
  the wire (e.g. a version/format byte on the envelope) rather than
  implied purely by the flag's current value, so a client can correctly
  decode a message that was encoded under a since-flipped server flag
  value. This is the one item where "the flag's current value" and "the
  wire format of a specific already-sent message" can diverge within a
  single connection, and the design must make the message self-describing
  rather than relying on both ends agreeing on live flag state at the same
  instant.

**Net rollback-safety principle for Phase 3 planning:** prefer designs
where "flag off" is definitionally "absence of the new field/behavior" (an
optional proto field left unset, a gate key reverting to a pure string
substitution) over designs where "flag off" requires actively unwinding
state a flag-on period created (e.g. a stateful per-connection negotiation
of "are we in batched mode"). The former survives an uncoordinated,
per-request flag read with zero special-casing; the latter needs its own
tested rollback path per item, multiplying this project's already
identified two-state-machine risk.

## Summary of file/line references

- `server/services/connectrpc_websocket.go:544-564` — path selection:
  control-mode (default, managed sessions) vs. capture-pane-polling
  (external sessions / control-mode disabled).
- `server/services/connectrpc_websocket.go:1488-1560`, esp. `:1557` — control-mode's
  `runInputReadLoop` explicitly does not handle mid-stream
  `CurrentPaneRequest`; `:923-1020` shows the resize path's own
  forced-capture-and-push shape as the template for building one.
- `server/services/connectrpc_websocket.go:1966-2049` — capture-pane-polling's
  real (but ≈450-550ms) mid-stream `CurrentPaneRequest` handler; `:1973-1984`
  dimension-mismatch branch is item 3(a)'s literal target.
- `server/services/connectrpc_websocket.go:660-700` — control-mode's
  handshake-time (connection-open, not mid-stream) nudge +
  `waitForQuiescence(…, 500ms, 200ms)`, architecturally adjacent but a
  different trigger than resync.
- `session/tmux/exec_gate.go:41-115` — `AcquireExecSlot`/`gateDir`/
  `acquireSlot`; `:106-109` shows the `"default"` key collapse this project
  must design a resync-scoped alternative key against.
- `config/types.go:84-106` — `TmuxExecGateConfig{Slots}` /
  `SlotsOrDefault()`, the existing config-driven-capacity precedent
  requirements.md's Risk Control section points at.
- `config/config.go:332-335,1037-1054` — `FeatureFlags map[string]bool`,
  `GetFeatureFlag`/`SetFeatureFlag`, the per-instance JSON-config flag
  store this project's flag(s) should follow.
- `server/services/feature_flag_service.go:15-77` — `knownFeatureFlags`
  registry + per-flag gate-function convention (`workspacePeersBlockFor`
  as the pattern to mirror for a resync-reliability flag).
- `server/interceptors/feature_flag_interceptor.go:16-19` — documents the
  "read live, every request, no restart" contract this doc's rollback
  analysis depends on.
- `web-app/src/components/sessions/useVisibilityResync.ts` (full file) —
  current shipped implementation: `:5-10` timing constants, `:52`
  `pendingResyncCompletionRef`, `:108-168`
  `handleVisibilityOrFocusResyncInner` (item 1's gate point at `:120`),
  `:200-219` `notifyResyncOutputReceived` (item 2's ID-check point).
- `web-app/src/components/sessions/TerminalOutput.tsx:535,538-549` — where
  `isVisible`/`foreground` is already computed and where it needs to also
  reach `useVisibilityResync`.
- `web-app/src/lib/hooks/useTerminalStream.ts:125-203,284` — `foreground`'s
  only current consumer (reconnect-timeout selection), confirming it is
  not wired into the resync trigger.
- `web-app/src/lib/hooks/useTerminalFlowControl.ts:80-135` — `requestFullResync`,
  where a correlation ID would be generated and attached to the outgoing
  `CurrentPaneRequest`.
- `proto/session/v1/events.proto:124-126,193-214` — `TerminalOutput` and
  `CurrentPaneRequest` message shapes; no existing request/correlation-ID
  field or convention anywhere in this proto package.
