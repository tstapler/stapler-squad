# Research: Pitfalls for terminal-multi-connection-streaming

Grounds the Phase 3 hub/transport design against this repo's own prior incidents rather than
generic broadcast-hub theory. Every claim below cites a file/line or a prior project-plan doc.

## 1. Go broadcast-hub pitfalls, and how this repo already hits them

### 1a. Sequential fan-out under a shared lock blocks the fast path on the slowest subscriber

`session/mux/multiplexer.go:706-720`'s `broadcastToClients` is the closest existing analog to
the hub this project builds, and it already has the "slow subscriber blocks everyone" bug the
requirements ask the new design to avoid:

```go
func (m *Multiplexer) broadcastToClients(msg *Message) {
	m.clientsMu.RLock()
	defer m.clientsMu.RUnlock()
	encoded, err := EncodeMessage(msg)
	...
	for conn := range m.clients {
		_ = conn.SetWriteDeadline(time.Now().Add(100 * time.Millisecond))
		_, _ = conn.Write(encoded)
	}
}
```

This is called synchronously from `forwardPTYOutput` (`multiplexer.go:576-603`), the goroutine
that also reads the PTY. Each client write gets its own 100ms deadline, but the loop is
sequential and unbuffered per-client — N slow/stalled clients (a suspended IDE, a laptop that
went to sleep with the socket still open) serialize to up to `N * 100ms` of added latency on
*every* PTY read, degrading every other subscriber including fast ones. **Design against this
directly**: the new hub must fan out to each transport via its own bounded outbound channel/
buffer (see §1b), never a shared-lock loop of blocking `Write` calls on the hub's own goroutine.

### 1b. Unbounded/blocking subscriber channels vs. bounded-with-drop — this repo's own precedent

`server/services/connectrpc_websocket.go:1000-1004`'s `resizeCh` is this repo's existing
"coalesce, don't queue" pattern for a single-slot channel:

```go
resizeCh := make(chan resizeReq, 1)
...
select {
case resizeCh <- req:
default:
	// Worker is busy; drain stale value and replace with latest.
	select {
	case <-resizeCh:
	default:
	}
	resizeCh <- req
}
```

(non-blocking send with a "drain-then-replace" fallback — never blocks the caller, never grows
unbounded, and only ever holds the *latest* value.) The hub's per-subscriber output path should
follow the same shape for terminal-output frames: a bounded channel per transport with an
explicit policy for what happens when it's full — either drop-oldest-and-replace-with-latest
(safe for a "one screen redraw supersedes the previous" full snapshot, unsafe for streamed PTY
deltas that must not be dropped), or detach the subscriber and force a resync. **A plain
unbuffered `chan []byte` per subscriber written to inside the hub's own goroutine reproduces
§1a's blocking problem in a different shape** — if any one subscriber's channel has no reader
keeping up, the hub goroutine itself blocks on that subscriber's send and stalls broadcast to
every other subscriber. The fix is either (a) give every subscriber its own writer goroutine
(hub only ever does a non-blocking or bounded send into a per-subscriber queue) or (b) a
select-with-default fan-out that never blocks the hub goroutine, with metrics/logging when a
subscriber is dropped for falling behind.

### 1c. Goroutine leak risk: subscriber channel never drained after transport disconnects

None of the existing fan-out code in this repo (`multiplexer.go`'s `clients` map,
`external_streamer.go`'s `consumers` map) uses `xsync.Map` for the subscriber registry itself —
both use a plain `map` + `sync.RWMutex`/`sync.Mutex` (`multiplexer.go:65-66`,
`external_streamer.go` `consumersMu`), with explicit `delete()` in a `defer` on disconnect
(`multiplexer.go:490-496`, `external_streamer.go:186-190` `RemoveConsumer`). This is a clean
precedent to follow for the hub's own subscriber registry — but the goroutine-leak failure mode
to design against is specifically: if a per-subscriber writer goroutine (per §1b's option a)
blocks on a channel send and the corresponding "remove from registry + close channel" path is
never reached because the read side of that same goroutine already exited (e.g. transport's
`Close()` errored and was swallowed), the writer goroutine leaks forever, pinned by a channel
nobody will ever read again. **Concretely test this** with `goleak` (already a project
dependency — see `session/actor_test.go`, `session/pty_discovery_test.go`,
`server/server_test.go` for existing usage) around the hub's attach/detach lifecycle, not just
around happy-path streaming.

### 1d. Deadlock risk: sends inside a lock

`activeControlModeStreams` (an `xsync.Map`, `server/services/connectrpc_websocket.go:216`) is
this repo's existing lock-free registry pattern for per-session state — its `Compute` method
(used at `connectrpc_websocket.go:260-265`) is a good model for the hub's subscriber-count /
generation bookkeeping (atomic compare-and-swap-style updates, no manual `Lock()`/`Unlock()`).
The pitfall to design against explicitly: never perform a potentially-blocking operation (a
channel send to a subscriber, a network write, waiting on quiescence) while holding any lock —
including implicitly inside an `xsync.Map.Compute` callback, which runs under that map's
internal per-bucket lock. `xsync.Map` is a good fit for pure bookkeeping (registry
membership, counters) but the wrong place to also perform the fan-out send.

## 2. Wire-protocol batching pitfalls

### 2a. A batching boundary must never land mid-ANSI-escape-sequence

This exact failure class is why `web-app/src/lib/terminal/EscapeSequenceParser.ts` exists at
all — `project_plans/terminal-redraw-corruption/research/pitfalls.md:71-76` confirms (by
reading the current file and `git log -p`) that its *only* remaining job today is holding back
an incomplete trailing escape sequence at a chunk boundary so it can be prepended to the next
chunk (`findPartialEscapeAtEnd`/`isCompleteEscapeSequence`), and that this invariant is
currently correctly maintained. **Any new server-side batching/coalescing window that
concatenates or truncates raw PTY bytes must preserve this same invariant on the batching side
too** — if the hub buffers N discrete output events into one wire frame per tick, it must never
split within a single event's bytes (each underlying PTY read is already delivered as one
atomic unit today; batching must concatenate whole events, not slice into their byte buffers).
If future work ever needs to split a *single* oversized event across frames, it must reimplement
the same partial-escape-holdback logic `EscapeSequenceParser.ts` already has client-side — don't
assume the client only ever needs to handle chunking, not truncation-mid-escape.

### 2b. Content-sniffing coalescers can silently drop frames with different erase footprints

`project_plans/terminal-redraw-corruption/research/pitfalls.md:42-65` (§4a, "high confidence")
root-caused a *real, shipped* corruption bug in `RedrawThrottler`
(`web-app/src/lib/terminal/TerminalStreamManager.ts:42-92`): it classifies chunks as "full
redraw" via a regex and unconditionally overwrites `pendingRedraw` with the newest match,
discarding an earlier redraw outright if a second one lands within its 33ms window. When two
consecutive Ink redraws erase different numbers of rows (e.g. a 5-line status block that shrinks
to 1 line on the next frame), dropping the first frame leaves stale glyphs from rows the second
frame never touches. The doc's own pitfalls-for-the-planner list (lines 80-87) is directly
reusable here — in particular: **any coalescing/throttling layer must not silently discard a
pending frame whose erase footprint differs from the frame replacing it**; either flush before
replacing whenever footprints differ, merge by applying each frame's erase set, or don't
content-sniff at all and rely on time/count-based batching of *opaque* byte ranges (concatenate,
don't decide what's redundant). The new server-side batching design should default to the
opaque/concatenate strategy — it carries none of this risk — and treat any semantic
frame-dropping ("this redraw supersedes that one") as a separate, much riskier optimization that
needs its own dedicated regression test asserting erase-coverage is monotonic across coalesced
output (the fix that doc recommends at line 87, apparently not yet implemented — verify at
implementation time whether it landed).

### 2c. Batching must not fight the existing quiescence-detection contract

`waitForQuiescence` (`server/services/connectrpc_websocket.go:271-292`) already exists to answer
"has the TUI finished redrawing" — it's fed by a `quiescenceCh` that PTY-output events publish to
(see the resize-worker's use of it at `connectrpc_websocket.go:1064`) and is the mechanism that
lets the resize handler know when it's safe to `capture-pane` a stable screen (see
`resizeSettling` gating at `connectrpc_websocket.go:1024-1029, 1108-1112`, which explicitly
suppresses live forwarding during a reflow so a partial-redraw frame can never race the
authoritative post-resize snapshot). **A new hub-level batching/coalescing tick must not become
a second, uncoordinated notion of "settled."** Concretely: if the hub's batching window flushes
on its own fixed cadence independent of `quiescenceCh`, it can flush a batch mid-reflow (the
same race `resizeSettling` was added to close, just at a different layer), or it can *delay* the
resize-worker's quiescence signal from reaching subscribers because it's sitting in a
not-yet-flushed batch — silently reintroducing latency into the one path (`ResizeQuiescence`
signaling, R1.4) that exists specifically to keep the client's "is resizing" UI state accurate.
The batching design should treat `ResizeQuiescence` frames (and the authoritative post-resize
snapshot) as flush-immediately/bypass-batching messages, never subject to the same coalescing
window as ordinary output deltas — the existing resize-worker already treats this class of
message as special (dedicated marshal + `WriteMessage` calls outside the batching-eligible
path); the hub redesign must preserve that same "control/quiescence signals skip the queue"
property, not implicitly flatten all `TerminalData` variants into one uniform batched stream.

### 2d. Reordering: batching must preserve emission order across event types

Because a single tmux session today interleaves at least three message kinds on one WebSocket —
live output deltas, `ResizeQuiescence` signals, and scrollback/snapshot responses — the batching
layer must define and preserve a total order per subscriber (e.g. a monotonic per-hub sequence
number stamped at hub-broadcast time, not at per-subscriber-flush time) so that two subscribers
attached to the same hub see events in the same relative order even if their individual flush
cadences differ, and so a reconnecting subscriber can request a bounded backfill (this repo
already tracks `OldestSequence`/`NewestSequence` for scrollback chunks —
`connectrpc_websocket.go:967-989` — the hub's batching sequence numbers should be the same
concept extended to the live stream, not a separate scheme).

## 3. Feature-flag dark-launch pitfalls specific to a stateful, per-connection-lifetime system

### 3a. Flag flip mid-connection: don't tear down or fork a live connection's ownership model

`STAPLER_SQUAD_USE_CONTROL_MODE` (referenced in this repo's root `CLAUDE.md`) is checked once,
at stream-start time, in `streamViaControlMode` vs. `streamViaTmuxCapturePane` — an env-var flag
that's effectively fixed for the process lifetime (changing it requires a restart, which itself
force-disconnects every session, per this project's own Baseline section (requirements.md:17)
documenting exactly that failure mode from a real `make install-service` restart). The new
hub/transport flag must decide, explicitly, whether it follows this same "fixed at process
start, changed only via restart" model, or is dynamically re-checked per new connection while
existing connections keep running under whichever path they started on. **The dangerous middle
ground to design against**: a flag that's re-read per-connection but where the hub itself is a
process-lifetime singleton keyed by tmux session name — if connection A creates the *old* direct
per-connection path for session S, and the flag flips before connection B attaches to the same
session S, B's *new* hub-owned path and A's *old* direct path can both resize/capture S
concurrently. This is the exact race this project exists to eliminate, reintroduced at the
boundary between old and new code during rollout. The rollout plan should make the flag
per-session-hub-instantiation-time, not per-connection: once a session has a hub (new path) or a
direct owner (old path), every subsequent connection to that same session must join the same
model until the session's hub/owner is fully torn down (last connection closes) — never let two
different ownership models coexist for one live tmux session, only for two *different* sessions
during the transition window.

### 3b. Connection-registry observability during old/new coexistence

The Observability Requirements (requirements.md:87-93) call for per-session connection count and
"is this the old or new path" visible in logs. During dark-launch, some sessions will be on the
old path (each connection independently in `activeControlModeStreams`, no shared registry) and
others on the new path (hub-owned registry). **A dashboard/log query built against only the new
registry will undercount total live connections during the entire rollout window** — the old
path's per-connection generation tracking (`recordControlModeStreamStart`,
`connectrpc_websocket.go:248-267`) and the new path's hub subscriber registry are two disjoint
data sources that must both be queried (or explicitly unified into one logging namespace) until
the old path is fully retired, or an operator debugging a live incident mid-rollout will get a
falsely-low connection count from checking only one of the two.

### 3c. Rollback must not orphan hub state

The Risk Control section (requirements.md:94-98) specifies flipping the flag back as the
rollback path, with no code revert needed. For this to actually work for a *live* incident
(flip the flag while sessions are already running under the new hub), the rollback needs an
explicit answer to: what happens to a hub's already-attached subscribers when the flag flips?
Two safe options: (a) the flag only gates *new* hub creation — existing hubs and their attached
connections finish out their lifecycle under whichever model they started with (mirrors §3a's
per-session-instantiation-time model, extended to also cover flag-flip-while-running), or (b)
flipping the flag triggers a deliberate, logged forced-reconnect of every session (equivalent to
today's restart-triggered mass-disconnect, but intentional and controlled) so every connection
re-attaches uniformly under the new state. Silently trying to migrate a *live* hub's existing
subscribers to the old path in place is the highest-risk option and should be explicitly
rejected in the Phase 3 design rather than left undecided.

## 4. ssq-mux unification pitfalls (folding a Unix-socket transport into the same interface)

### 4a. Trust boundary mismatch: filesystem permission vs. application-level auth

The browser WebSocket path has an explicit comment naming its trust model:
`server/services/connectrpc_websocket.go:80` — *"Remote HTTPS access is secured by the auth
middleware; the origin check here only [...]"* / line 96, *"Allow any HTTPS origin — auth is
enforced by the middleware layer."* Authentication is enforced by a layer the transport itself
doesn't implement. ssq-mux's entire trust model is the opposite: `session/mux/multiplexer.go:199-203`
creates the Unix socket then immediately `os.Chmod(m.socketPath, 0600)` — filesystem ownership
*is* the auth boundary, with zero application-level authentication inside the mux protocol
itself (`handleClient`, `multiplexer.go:489-573`, accepts `MessageTypeInput`/`MessageTypeResize`
from any connected peer with no credential exchange). **If both become "just transports"
implementing the same interface, the interface itself must not silently assume the WebSocket
transport's implicit assumption ("auth already happened upstream") applies universally** — a
generic `Transport.Send`/`Attach` interface with no `Authenticate()`/trust-level concept baked in
risks a future refactor accidentally exposing ssq-mux's input/resize handling through a code path
that assumes middleware-level auth already ran (e.g. if `session/external_discovery.go`'s
auto-discovery ever became reachable over something other than the local Unix socket). Phase 3
should decide explicitly whether the transport interface carries a trust-level marker/capability
per implementation, or whether trust enforcement stays entirely external to the interface (each
transport's constructor is responsible for confirming its own boundary, and the interface itself
makes no claims) — leaving it unstated is the actual risk.

### 4b. Resize authority: ssq-mux already has its own unmediated multi-writer race

`multiplexer.go:549-553` lets *any* connected client send `MessageTypeResize`, which calls
`m.SetWindowSize` directly with zero coordination among ssq-mux's own multiple clients — this is
the *exact* race this project is being built to fix for the browser path, already present
independently in ssq-mux's existing code. Folding ssq-mux into the same hub eliminates this for
free *if* ssq-mux's resize messages are routed through the hub's single resize-authority instead
of calling `SetWindowSize` directly from `handleClient` — but that requires touching
`multiplexer.go`'s client-handling loop, which the Rabbit Holes section (requirements.md:71)
already flags as a possible "own multi-week redesign." Concretely scope what changes: routing
ssq-mux's resize/input through the hub means `Multiplexer` can no longer own `SetWindowSize`
calls directly from `handleClient` — it must forward resize *requests* to whatever now owns
authority (the hub), which is a real behavior change to ssq-mux's existing multi-IDE-terminal
use case, not just an additive wrapper.

### 4c. Lifecycle mismatch: process-scoped vs. session-scoped

ssq-mux's `Multiplexer` is a per-*process* thing (`NewMultiplexer` spawns and owns the actual
`exec.Cmd`/PTY, `Shutdown()` at `multiplexer.go:398` tears down the whole tmux session) — it's
the tmux session's original creator/owner. The browser WebSocket path's hub is being designed as
a per-tmux-*session* stream owner that outlives any individual connection but does not itself
own the tmux process's lifecycle (tmux sessions in this repo are typically long-lived, created
independently of any particular connection). If ssq-mux's `Multiplexer` becomes "just a
transport" attached to the same session hub, its `Shutdown()`/process-exit semantics (which
currently tear down the tmux session itself) must not be conflated with a transport merely
*disconnecting* (which should just detach from the hub, leaving the session alive for the
browser or other transports still attached) — these are two different lifecycle events that
ssq-mux's current code treats as one because today it's the only owner. Phase 3 needs to decide
whether ssq-mux-as-transport keeps its process-exit-kills-session behavior (documented,
intentional) or whether unification requires decoupling "the external terminal process exited"
from "the underlying tmux session should die."

## 5. What to design against explicitly, given no staging environment

This is a rewrite of `server/services/connectrpc_websocket.go`, the single most failure-visible
path in the product (requirements.md:85 states this directly), on the operator's own live daily
driver, with no staging environment. Combined with §§1-4 above, the concrete design constraints
this implies:

1. **The dark-launch flag must default OFF and be per-session-hub-lifetime, not per-request**
   (§3a) — a mid-flight flip must never let two ownership models race the same session.
2. **The old per-connection race-detector WARN (`420584566`,
   `server/services/connectrpc_websocket.go:240-267`) should stay live and unmodified until the
   new path is the default**, not repurposed or removed early — it is the only existing
   after-the-fact signal for exactly the failure this project fixes, and Requirements
   (requirements.md:91) already calls for it to become a hard invariant only "once the new hub
   design is in place," implying it must survive the whole dark-launch window unchanged as a
   safety net for the old path.
3. **Every new hub/transport code path needs unit coverage using the in-memory transport before
   it ever runs against a real session** (the explicit Testability NFR, requirements.md:50) —
   given there's no staging environment, the in-memory transport is the only rehearsal
   environment this system will get before hitting production; treat gaps in that test's
   coverage of §§1-4's specific failure modes (slow-subscriber, goroutine leak, quiescence
   interaction, resize-authority negotiation) as high-severity findings during Phase 4 validation
   (test-coverage-to-requirements mapping) rather than generic "add more tests" feedback.
4. **A same-day rollback path must be exercised, not just asserted** — given the operator is
   also the only user and there's no staging, "flip the flag back" (requirements.md:96) should
   itself be validated against a live session at least once during rollout (e.g. flip on, use it
   briefly, flip off, confirm the session survives and reconnects cleanly), because the first
   time a rollback path is actually exercised is otherwise during a real incident.
5. **Prefer additive observability over behavior change in the same commit as the hub cutover** —
   land the connection registry, structured attach/detach/resize-negotiation logging, and the
   `420584566` WARN's continued operation *before or alongside* the first session that runs on
   the new path in production, not after, so that if the new path misbehaves on the very first
   real session, there's already a log trail to diagnose it rather than needing a second
   deploy to add instrumentation after the fact.

## Sources

- `project_plans/terminal-multi-connection-streaming/requirements.md` (this project's own spec)
- `project_plans/terminal-jank/research/pitfalls.md` (Pitfalls 4-6: rapid-switch races,
  alternate-screen/cursor loss, capture-pane resize race — background context on why
  control-mode + capture-pane is already fragile)
- `project_plans/terminal-redraw-corruption/research/pitfalls.md` (§4a `RedrawThrottler` frame-
  coalescing drop, §5 `EscapeSequenceParser.ts` partial-escape-holdback invariant, §6 pitfalls
  list for planners — directly reused in §§2a-2b above)
- `project_plans/terminal-resize-fit-loop/research/pitfalls.md` (headers reviewed; shipped work
  on resize-observer/fit feedback loops, adjacent but client-side — no direct reuse found beyond
  confirming this repo's existing resize-jank vocabulary)
- `server/services/connectrpc_websocket.go` (activeControlModeStreams/generation tracking lines
  190-267; resizeCh coalescing lines 1000-1158; waitForQuiescence lines 271-292; resizeSettling
  gating lines 1024-1112; auth-boundary comments lines 80, 96)
- `session/mux/multiplexer.go` (clients registry lines 36-66; handleClient resize handling lines
  549-553; broadcastToClients lines 706-720; socket permission lines 199-203)
- `session/external_streamer.go` (consumers registry + AddConsumer/RemoveConsumer lines 160-197)
- `.claude/docs/pty-multiplexing.md` (ssq-mux architecture overview)
- `docs/registry` / root `CLAUDE.md` (`STAPLER_SQUAD_USE_CONTROL_MODE` flag precedent)
