# Feature Research: terminal-resync-reliability (Agent 2 — Features)

## 1. Prior art in this codebase

The resync feature under review already borrows from an existing pattern in the
same file, `web-app/src/lib/hooks/useTerminalFlowControl.ts`: `resize()`
(lines 197-291) already does throttle + defer-trailing-edge + a post-resize
`CurrentPaneRequest` follow-up (100ms after resize). `requestFullResync()`
(lines 80-135) reuses the identical `CurrentPaneRequest` wire message but adds
an `urgent` bypass for the 2s throttle. **There is no correlation ID on either
path today** — both rely on the same imprecise "next `output` message means
resync done" heuristic that `useVisibilityResync.ts`'s
`notifyResyncOutputReceived()` (lines 200-219) implements, per its own comment
at lines 46-51: "same imprecise heuristic the pre-existing resize->resync path
already relies on." This means **fixing the correlation ID only in the
visibility-resync path while leaving `resize()`'s follow-up `CurrentPaneRequest`
uncorrelated reintroduces the exact ambient-output-masks-completion bug for
resize-driven resyncs** — Scope Item 2 needs to cover both call sites or
explicitly document why it doesn't.

`isVisible`/`foreground` is already plumbed as a prop through
`TerminalOutput.tsx` (line 68 declaration, line 535 `foreground: isVisible`
passed into `useTerminalStream`) and is *already load-bearing* for reconnect
backoff there — `useTerminalStream.ts` lines 125-203 use a
`foregroundRef`/`foregroundConnectAttemptRef` pair to reset backoff attempts
on a background→foreground transition and pick timeout duration via
`connectTimeoutMs(foregroundAtSchedule, attempt)`. It is **not**, however,
threaded into `useVisibilityResync.ts` at all — that hook takes no
`isVisible`/`foreground` param and reacts to the raw document-level
`visibilitychange`/`focus` listeners it attaches itself (lines 176-183). So
Scope Item 1 isn't wiring up a new concept; it's connecting a signal that
already flows one hop away (`SessionDetailView.tsx` → `TerminalOutput` prop →
`useTerminalStream`) into a sibling hook that currently ignores it. The
existing `foregroundRef` pattern (a ref updated in a `useEffect`, read inside
event-driven callbacks to avoid stale closures) is the template to copy rather
than reinvent — `useVisibilityResync.ts` already uses the identical
ref-mirroring idiom for `isConnectedRef`/`terminalStateRef`/etc. (lines 61-92).

`SessionDetailView.tsx` sets `isVisible` from three different scoping rules
depending on layout mode: `poolPath === session.externalMetadata?.muxSocketPath`
(line 741, pooled-terminal grid), `poolId === session.id` (line 760), and
`activeTabId === shellKey` (line 868, shell tabs) plus `activeTab === "browser"`
(line 846). This confirms the requirement's framing that "visible" is not a
single boolean flag globally — it's *per-terminal*, computed against
whichever container/tab is currently selected, and a **split view can make
more than one of these true simultaneously** (the pooled-terminal grid case is
explicitly a multi-visible layout, not tabs). Any resync-scoping design must
handle "N ≥ 1 simultaneously visible" as the normal case for split-view users,
not just the tab-switch single-terminal case that dominates in most other
tools' mental model.

The exec-gate (`session/tmux/exec_gate.go`) is a **cross-process file-lock
gate** (`flock`, lines 41-52), keyed by a hash of the tmux server socket path
(`gateDir(serverSocket)`), not literally a single global "default" string —
the requirement's framing ("virtually all sessions map to the same `default`
gate key") is an observed consequence of most sessions sharing one tmux server
socket in typical deployments, not a hardcoded key. This matters for the
"new gate key / fast lane" open question: a resync-specific fast lane most
naturally means a *second* gate directory alongside the existing one (same
`serverSocket`-keyed derivation, different suffix), not a new global constant
— consistent with how the file already isolates gates per socket.

The server already has a working example of "skip the expensive path when
nothing actually changed": lines 1702-1711 of
`server/services/connectrpc_websocket.go` skip the SIGWINCH nudge entirely
when `actualCols/actualRows` (read fresh via `target.GetPaneDimensions()`)
already match the requested target — explicitly to avoid the readline
double-echo artifact from an unconditional nudge on every reconnect. The
mid-connection resync path this project targets (lines 1966-2019, handling
`GetCurrentPaneRequest()` in the input loop) does **not** have this
short-circuit — it always re-resizes and does the 3x-refresh-signal +
250ms-sleep dance (~450ms+) whenever `currentCols != targetCols ||
currentRows != targetRows` (line 1984), with no way to distinguish "genuinely
resized" from "client's last-known dimensions are stale because the tab was
backgrounded." The handshake path's server-side dimension read
(`target.GetPaneDimensions()`) is itself the fix pattern to reuse: if the
server already knows the pane's real current dimensions, and those match what
tmux is already showing, a stale client-reported target that differs from the
*true* server pane can be reconciled without a resize round-trip at all —
the client's request would need to become "does this match?" rather than
"resize to this," or the server needs a way to know the client was
backgrounded (a `wasBackgrounded` flag on `CurrentPaneRequest`, since nothing
in the current proto distinguishes a resize-driven resync from a
visibility-driven one).

## 2. Industry precedent for multi-pane reconnect/resync

- **VS Code integrated terminal** — persists terminal state via
  `terminal.integrated.enablePersistentSessions` and reconnects to a *server-side*
  pty (via its Remote/pty-host process) rather than re-deriving screen state
  from the client. On window reload, only the terminal panel that's *visible*
  reconnects immediately; background terminal tabs reconnect but are not
  actively redrawn until selected. This is the direct analog to Scope Item 1 —
  VS Code treats "tab is the active one in the terminal view" as a gate on
  redraw work, not just on data delivery. [Terminal Advanced docs](https://code.visualstudio.com/docs/terminal/advanced)
- **GoTTY / ttyd** — neither actually solves N-concurrent-terminal resync; both
  push multi-pane sharing onto tmux/screen itself (`ttyd tmux new -A -s ttyd`)
  and treat the browser side as a single dumb pty relay with a `ResizeTerminal`
  message and a `SetReconnect` server-initiated signal. There's no batching or
  staggering concept because there's fundamentally one pty per connection — this
  is a "smaller surface, no multiplexing problem" precedent, useful mainly as a
  contrast: stapler-squad's problem exists *because* it multiplexes many
  terminals over the app layer while GoTTY/ttyd don't. [ttyd README](https://github.com/tsl0922/ttyd), [GoTTY README](https://github.com/sorenisanerd/gotty)
- **Mosh (State Synchronization Protocol)** — the most relevant conceptual
  precedent for "reduce round trips and improve compression" (Scope Item 5).
  Mosh doesn't resync via "replay what changed since last ack"; both sides hold
  a full screen-state object and the protocol's job is bringing the client to
  the *current* object state as cheaply as possible, explicitly allowed to
  *skip* intermediate states rather than transmit every one. Applied here: a
  resync response is fundamentally a full-snapshot operation already (the
  handshake path proves this — it sends a full `clearAndHome + capture`
  blob), so batching N terminals' resyncs is really "N independent snapshot
  fetches" not "N deltas from possibly-different baselines" — which simplifies
  the batching design (no need to reconcile diverging diff chains across
  panes) but means compression gains come from wire-level tricks (single
  larger message, shared compression dictionary/context across panes) rather
  than from reducing what's fundamentally sent. [Mosh paper](https://mosh.org/mosh-paper-draft.pdf)
- **Eternal Terminal** — reconnects a *byte-stream* TCP session rather than a
  state object; closer to what `useTerminalFlowControl`'s resync already does
  (full capture-pane resend) than to Mosh's approach. No public prior art here
  for multi-pane batching either — ET is single-session per connection like
  ttyd.
- **Zellij** (terminal multiplexer with its own client/server split) — its
  server holds full pane state and clients reattach by requesting a fresh
  render; no public web-multiplexer-specific batching precedent found for the
  "many browser tabs each independently resyncing" scenario this project is
  actually about — the closest true precedent for the *problem shape* (many
  logical channels over one transport, needing correlation IDs to avoid
  cross-talk) is **JSON-RPC 2.0 batch requests over WebSocket**: batch an array
  of requests with client-chosen `id`s, server responds with a corresponding
  array (order not guaranteed — client must match by `id`), which is
  essentially the shape Scope Items 2+5 need (correlate N per-terminal
  `CurrentPaneRequest`s and let 1 batched response satisfy N pending resyncs).
  Note real-world providers cap WS batch size (e.g. 20) — worth setting an
  explicit cap here too rather than batching unboundedly. [JSON-RPC 2.0 spec](https://www.jsonrpc.org/specification), [Batch requests guide](https://www.json-rpc.dev/learn/examples/batch-requests)
- **Reconnect-storm / thundering-herd mitigation** (general WebSocket
  practice) — the standard fix for "many clients act at the same instant" is
  client-side jitter plus server-side rate limiting, which is exactly what
  Scope Item 4 (stagger/prioritize) is reinventing for this narrower case.
  Worth explicitly deciding jitter bounds (e.g. spread N terminals' resyncs
  across a 0-500ms window with per-terminal random offset) rather than a fixed
  stagger interval, since fixed intervals still synchronize badly if two
  browser tabs (two independent JS event loops) both fire focus events at
  once. [WebSocket connection-limits guide](https://websocket.org/guides/connection-limits/)

## 3. Edge cases and failure modes to design for

1. **Terminal becomes visible mid-burst.** If terminal A's resync is already
   queued/staggered and the user switches focus to terminal B before A's
   resync fires, does A still need to resync (it's about to go invisible
   again) or should visibility-scoping cancel A's queued resync and
   fast-track B's? The existing `useVisibilityResync.ts` sessionId-mismatch
   guard (line 118, `if (sessionId !== sessionIdRef.current) return`) is the
   template for "abort a stale scheduled callback," but that guard only
   covers a session *switch*, not a visibility *revocation* of the same
   session mid-debounce — a new ref (`isVisibleRef`) checked at the top of
   `handleVisibilityOrFocusResyncInner` would need the same treatment.
2. **Session closed/unmounted while its resync is in flight.** The existing
   cleanup effect (lines 188-198) already tears down `pendingResyncCompletionRef`,
   timers, and calls `markResyncComplete`/`markPaneResponseReceived` on
   sessionId change — that pattern generalizes to unmount, but a *correlation
   ID* design adds a new failure mode: if the server's async response arrives
   *after* the client has already unmounted and forgotten the correlation ID,
   the response is silently dropped (correct) but the client-side "pending
   resync" map/set must not leak — needs a bounded-size or TTL'd pending-map,
   not an unbounded one keyed by ID.
3. **Correlation ID response arrives after the 4s stall watchdog already
   fired.** This is explicitly called out as the current watchdog's own
   failure mode and gets *worse*, not better, with a naive correlation-ID
   implementation unless handled: today, ANY output message satisfies the
   heuristic, so a late resync response would have been consumed (harmlessly)
   as "ambient output." With per-ID correlation, the watchdog's
   `disconnectRef.current().then(() => connectRef.current())` (line 143)
   tears down the connection — when the late response then arrives on the
   *old* (now-dead) connection or against a *new* connection's stream reader,
   it must be identified as stale (ID no longer in the pending set) and
   discarded rather than being misapplied to whatever resync is pending next.
   This requires either scoping correlation IDs per-connection-instance (e.g.
   include a connection/epoch number) or clearing the entire pending-ID set on
   watchdog-triggered disconnect — the latter is simpler and matches the
   existing "watchdog is the single recovery path" design comment (line 132).
4. **Two terminals become visible simultaneously in a split view.** Per
   `SessionDetailView.tsx`'s pooled-terminal grid (`poolPath ===
   session.externalMetadata?.muxSocketPath`), this isn't a rare race — it's a
   supported, intentional layout. Batching/staggering must treat "all
   currently-visible terminals resync together, batched into one round trip"
   as the *common* case for split-view users, not an edge case to merely
   tolerate. A stagger-only design (spread requests over time, no batching)
   would make split-view resync strictly slower than today for the exact
   users who most need multiple terminals responsive at once — this is a
   real tension between Scope Items 4 and 5 that Phase 3 planning needs to
   resolve explicitly (e.g.: stagger across *invisible* terminals, batch
   across *simultaneously-visible* ones).
5. **Batched request partially fails for one pane but not others.** If pane B
   in a 3-pane batch has since exited or its tmux target is gone
   (`target.GetPaneDimensions()` errors, as already handled per-pane at line
   1978-1981 and 1955-1957 today), the response for A and C must not be
   blocked or invalidated by B's failure — this argues for a response shape
   that's an array of independently-resolved per-ID results (JSON-RPC batch
   shape) rather than one all-or-nothing response, and the client's
   per-terminal `notifyResyncOutputReceived` must be callable per-ID from
   within a single batched wire message, not just once per message received.
6. **Exec-gate queueing during a burst competes with unrelated tmux work.**
   Because the gate is keyed by tmux server socket (not by request type), a
   resync fast lane (Scope Item 3b) that's a *separate* gate keyed the same
   way as the existing one only helps if resync callers actually use the new
   key — any code path that still calls the general
   `AcquireExecSlot`/`TryAcquireExecSlot` for resync-triggered subprocess work
   (the SIGWINCH refresh calls, resize-pane exec, etc. at lines 1944-1952 and
   1997-2005) needs to be identified and migrated, or the fast lane doesn't
   actually bypass the contention it's meant to relieve.
7. **Feature flag off mid-session / flag flip while terminals are mounted.**
   Since `SessionDetailView.tsx` keeps every terminal mounted for the
   session's lifetime, a flag that's read once at mount (typical React
   pattern) versus read live on every `visibilitychange` event will produce
   different behavior for already-open sessions after a flag toggle — worth
   deciding explicitly whether the flag is expected to take effect for
   already-mounted terminals or only future ones, since the existing hooks
   have no live-config-reload plumbing today.
8. **Backgrounded-tab dimension staleness vs. genuine resize, disambiguated
   incorrectly.** If the new "backgrounded, don't take slow path" heuristic
   has false positives (treats a real resize as a stale-background one), the
   pane genuinely won't be resized to the new terminal size — reintroducing
   the wrapping/rendering bugs the original visibility-resync feature (and
   the resize-throttle logic in `useTerminalFlowControl.ts`) were built to
   prevent. This is the single highest-regression-risk edge case given the
   Constraints section's explicit prohibition on weakening the original
   feature's guarantee — the heuristic needs a clear, testable signal (e.g. a
   `wasBackgrounded` bit set by the *client*, which knows definitively whether
   it was hidden, rather than the server inferring it from timing).

## 4. Unstated user needs

- **Visual feedback should track reality, not just elapsed time.** The
  existing 2s reconnecting-banner (`RESYNC_BANNER_DELAY_MS`, Story 2.1.8) fires
  on a fixed timer regardless of *why* the resync is slow. Once resyncs are
  batched/staggered, a background terminal's resync may be legitimately
  delayed by design (staggered on purpose, not stalled) — showing the same
  "reconnecting" banner for an intentionally-deferred, healthy resync as for a
  genuinely stalled one would be a UX regression (alarming users about normal
  behavior). The banner heuristic likely needs to distinguish "queued by
  design" from "taking too long unexpectedly."
- **Graceful degradation when the flag is off must be indistinguishable from
  today's shipped behavior**, not just "no crash." Given this is scoped as a
  reliability hardening of a *live* feature (Constraints section), users
  should see zero behavior change with the flag off — this needs to be an
  explicit acceptance criterion and test, not just an implicit assumption,
  since a flagged code path that changes early-return ordering or event
  listener registration timing can subtly alter behavior even when the new
  logic itself never executes.
- **Users want the terminal they're actually looking at to never be the
  victim**, per the requirement's own framing ("it's usually not the one I'm
  typing in that runs into trouble"). This implies an implicit priority
  ordering users would want made explicit: the visible/focused terminal's
  resync should never be *starved* by a batch/stagger scheme optimizing for
  background terminals — i.e., visibility-based prioritization (Scope Item 4)
  should bias latency improvements toward the foreground terminal first, with
  background terminals accepting more delay, not treat all simultaneously-
  visible-or-about-to-need-resync terminals as equal-priority.
- **Observability consumers (the developer/operator) want burst-size and
  queue-depth signals correlated with user-visible symptoms**, not just raw
  counters — e.g. being able to answer "was terminal X's stall watchdog fire
  caused by exec-gate queueing or by the dimension-mismatch slow path" from
  the observability data (Observability Requirements section lists both
  metrics but doesn't tie them to a shared correlation ID/trace that would let
  someone reconstruct a single incident's causal chain after the fact) — the
  correlation ID introduced for Scope Item 2 is a natural candidate to also
  tag the exec-gate-wait and dimension-mismatch metrics with, turning three
  separate counters into one traceable timeline per resync.
- **No perceptible increase in "time to usable" for a single-terminal user.**
  Batching/staggering optimizations are aimed at the multi-terminal burst
  case; a design that adds any fixed overhead (e.g. a mandatory short debounce
  window to allow batching opportunities) to the common single-terminal-focus
  case would trade a rare annoyance (stall watchdog on background tabs) for a
  constant tax on the majority-case interaction (one person, one focused
  terminal) — worth an explicit non-goal or guard (e.g. only batch/stagger
  when N-visible-or-pending > 1) rather than letting the batching window apply
  universally.
