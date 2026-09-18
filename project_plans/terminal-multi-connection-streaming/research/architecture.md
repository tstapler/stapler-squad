# Research: Architecture — terminal-multi-connection-streaming

Scope: architecture pattern selection, integration points, migration/dark-launch failure
modes, prior-art check, and an EventStorming table for the redesign described in
`project_plans/terminal-multi-connection-streaming/requirements.md`.

## 1. Current code, read directly

`server/services/connectrpc_websocket.go`:

- `ConnectRPCWebSocketHandler` (line 194) holds `snapshotCache *xsync.Map[string, sessionSnapshot]`
  and, since commit `420584566`, `activeControlModeStreams *xsync.Map[string, controlModeStreamGeneration]`
  + `controlModeStreamCounter atomic.Int64` (lines 205–226).
- `recordControlModeStreamStart` (lines 248–267) is **purely observational**: it stamps a
  monotonic generation number per `streamViaControlMode` invocation, keyed by tmux session
  name, and logs a WARN if a second generation starts before the first's `done()` runs. It
  does not block, queue, or coordinate anything — confirmed by reading the function body:
  the `Store` always wins regardless of whether a prior entry existed.
- `streamViaControlMode` (line 633 in this read, i.e. requirements.md's cited ~569–1050 range)
  per invocation: parses the handshake for `CurrentPaneRequest` dimensions (line 664), calls
  `instance.ResizePTY` twice — a `cols-1` nudge then the real target (lines 878–884) — waits
  for quiescence via a **local, per-invocation** `quiescenceCh` (line 892), then captures the
  pane via `getOrRefreshSnapshot`/`waitForPaneContent` (line 910). A second concurrent
  invocation runs this same sequence independently, racing the first's resize/capture. This
  is the confirmed root cause named in the requirements doc.
- Two goroutines are spawned per connection: an output-forwarding goroutine (line 741) that
  drains `updateChan` and coalesces up to 32 frames per WebSocket write, and a resize-debounce
  goroutine (line 1005) that drops resize requests within 50ms and suppresses forwarding
  (`resizeSettling`) during a reflow.

## 2. Load-bearing prior art already in the codebase — the hub pattern half-exists

This is the most important finding and changes the shape of Phase 3 planning: **the hub
pattern requirements.md asks for is already implemented, twice, at a lower layer than the
WebSocket handler — just not extended to cover resize/capture.**

### 2a. `session/tmux/control_mode.go` — tmux control-mode is already single-owner + fan-out

- `TmuxSession.StartControlMode()` (line 69) is refcounted: `controlModeStartMu` serializes
  0→1 transitions, and if `controlModeCmd != nil` it just increments `controlModeRefCount`
  and returns (lines 75–82). **Only one `tmux -C attach-session` subprocess is ever spawned
  per `TmuxSession`, no matter how many callers call `StartControlMode`.** `StopControlMode`
  (line 144) mirrors this: it decrements the refcount and only tears down the process when it
  hits zero.
- `SubscribeToControlModeUpdates` (line 733) and `broadcastControlModeUpdate` (line 702)
  already implement N-subscriber fan-out over a `map[string]chan []byte`
  (`controlModeSubscribers`, declared in `session/tmux/tmux.go:150`) guarded by
  `controlModeSubMu`. A slow subscriber gets a bounded grace period
  (`controlModeSlowSubscriberGrace`) before being closed and evicted rather than silently
  dropping bytes — the doc comment at `control_mode.go:691-701` explicitly says this mirrors
  `NativeProcessManager.fanOut`'s close-and-remove pattern (`session/native_process_manager.go:443`).

**This directly answers requirements.md's Feasibility Risk #1** ("does tmux -C support
multiple simultaneous attach-equivalents, or does the hub need to be tmux's ONLY client and
rebroadcast to N logical subscribers itself"): tmux -C does **not** get multiple attach
equivalents today, by construction — the refcounted `Start/StopControlMode` guarantees tmux
sees exactly one control-mode client, and `broadcastControlModeUpdate` already rebroadcasts
its raw output to N subscriber channels. No further verification against tmux itself is
needed; the existing code has already made this design decision and shipped it. What's
**not** covered by this existing layer is resize and capture-pane — those are still invoked
independently by every `streamViaControlMode` caller with no equivalent refcounting/ownership,
which is exactly the gap this project targets.

### 2b. `session/external_streamer.go` — the ssq-mux path is even closer to the target shape

`ExternalStreamer` (line 24) is structurally almost the hub this project wants to build,
already in production for the ssq-mux flow:

- One `net.Conn` per socket, connection lifecycle owned by the streamer (`connect`, `readLoop`,
  `reconnect`) — a single actor-like owner, not per-viewer.
- `AddConsumer(consumer OutputConsumer, catchUp bool) string` / `RemoveConsumer(key string)`
  (lines 165–190) — the registry requirements.md asks for ("understand multiple connections"),
  already keyed by a UUID token, already reference-counted via `ConsumerCount()` (line 193).
- A ring buffer (`newRingBuffer`, line 55) gives late-joining consumers scrollback catch-up
  (`GetSnapshot`/`GetRecentOutput`, lines 265–305) instead of every consumer independently
  polling/capturing.
- `ExternalStreamerManager.GetOrCreate(socketPath)` (line 528) is a per-socket singleton
  factory — i.e. already "one hub per session" at the manager level.

**What it does *not* solve** (so the same open question exists here as in the tmux path):
`SendResize(cols, rows uint16)` (line 250) just writes a resize message straight to the
socket with no negotiation — if two consumers call `SendResize` with different dimensions,
last-write-wins, unmediated, same class of race as `streamViaControlMode`'s per-connection
`ResizePTY`. The resize-negotiation open question in requirements.md is unresolved in both
existing paths, not just the one being redesigned.

**Implication for planning**: rather than designing a hub abstraction from scratch and then
retrofitting ssq-mux into it (requirements.md's Rabbit Hole), the more direct path is to
generalize the *existing* `ExternalStreamer`-shaped pattern (owner + registry + ring buffer +
broadcast) into the shared hub, and have both the tmux control-mode owner and the ssq-mux
owner become instances of it. `ExternalStreamer`'s `AddConsumer`/`RemoveConsumer` interface is
close enough to a subscriber-registration API that it's a strong shape reference for the new
transport interface — Phase 3 should evaluate literally extracting/renaming this type rather
than writing a parallel one.

## 3. Architectural pattern recommendation

**Actor model / single-owner-goroutine hub, with the transport as a narrow interface the hub
broadcasts to.** Concretely:

- One goroutine (or a small owned goroutine group: reader + sender, mirroring
  `TmuxSession`'s existing `runCMSender`/`readControlModeOutput` split) owns all mutable state
  for one tmux session's stream: the resize target, the quiescence state, the cached snapshot,
  and the subscriber registry. All external requests (attach, detach, resize-request, input)
  become messages/method calls into this owner, never direct field mutation from N caller
  goroutines. This is exactly Go's actor idiom ("no shared-state locking; goroutine + channels
  own the state") and is already half-implemented per §2 above — `TmuxSession`'s
  `controlModeStartMu`-guarded refcounting is a mutex-based approximation of ownership, not a
  true actor, and that's precisely why resize/capture (which sit *outside* that guarded
  section, in the WebSocket handler) aren't covered by it.
- This repo's own `.claude/docs/concurrency-patterns.md` and the `golang-concurrency` /
  `golang-design-patterns` skills are the house style reference for choosing here: prefer
  channels/actor ownership over broader mutexes for "one writer, many readers of evolving
  state" shapes (this is exactly that shape — one hub decides "the current pane content and
  size," N subscribers only ever read from it). `docs/adr/011-prefer-lock-free-concurrency.md`
  sets repo-wide precedent for preferring atomics/CAS/copy-on-write over mutexes for simple
  state — the hub design should keep the *subscriber registry* itself lock-free-friendly
  (an `xsync.Map[string, subscriber]`, matching `snapshotCache`'s existing use of
  `xsync.Map` at `connectrpc_websocket.go:203`, or the mutex-guarded map `TmuxSession` already
  uses) while the resize/capture decision logic itself is the one thing that must be
  serialized through a single owner — not lock-free, because it's a sequential decision
  ("what size, what content, whose request wins"), not a simple counter/flag.
- **Reject a pure pub/sub broker** (e.g. a generic in-process message bus) as the top-level
  pattern: it would solve fan-out but not the resize/capture mutual-exclusion problem, which
  is the actual root cause. A broker is a reasonable *implementation detail* inside the hub
  for the broadcast step (and `broadcastControlModeUpdate` already is one), but the top-level
  architectural decision is "single owner," not "many publishers, one topic."

## 4. Integration points

| System | Current role | What changes |
|---|---|---|
| `session.Instance` (`session/instance.go:122`) | Owns the `TmuxBackend`/`processManager`; `streamViaControlMode` calls its `SessionStreamer`-interface delegation methods (`StartControlMode`, `SubscribeControlModeUpdates`, `ResizePTY`, `SetWindowSize`) directly, once per connection. | The hub becomes the single caller of these methods per session; `Instance` itself is unchanged (it already only exposes ref-counted `Start/StopControlMode` per §2a) — the hub sits **between** `ConnectRPCWebSocketHandler` and `Instance`, replacing "N handler invocations each call Instance directly" with "N handler invocations attach to one hub, hub calls Instance." |
| `session/tmux/control_mode.go`'s refcounted Start/Stop + `controlModeSubscribers` fan-out | Already single-tmux-client + N-subscriber broadcast for **output only** (§2a). | No change needed to this layer for output; the hub's resize/capture logic sits logically above it. The hub's own subscriber registry can either wrap this one (double bookkeeping) or replace it (hub takes over `controlModeSubscribers`' job) — Phase 3 must decide which, since the WebSocket handler's `activeControlModeStreams` (a *third*, informational-only registry) already overlaps this same concern; **the observability requirement to consolidate to one connection-count source of truth (§ requirements.md Observability Requirements) means at most one of these three registries should survive.** |
| `session/external_streamer.go` + `ExternalStreamerManager` (ssq-mux) | Independently hub-shaped already (§2b) but on a separate code path from the browser one, with its own resize race (`SendResize`). | Candidate to become the second transport under the same interface, or — per requirements.md's Rabbit Hole — to have its existing owner/registry/ring-buffer generalized *into* the shared hub type rather than kept as a parallel implementation. Either way, its `SendResize` race must be fixed by the same resize-negotiation mechanism the tmux path gets — don't fix one path's race and leave the other's, or the observability/success-metric bar ("race eliminated... verified by stress test") is only half met. |
| `scrollback.ScrollbackManager` | Already read directly by `streamViaControlMode` (line 959: `GetRecentLines`, `GetStats`) to send an initial `ScrollbackResponse` per connection — this is naturally idempotent (read-only, sequence-numbered) and does **not** need hub mediation; each new subscriber can keep calling it directly on attach. |
| `session/external_discovery.go` | Discovers external (ssq-mux) sessions and hands `resolveSession` an `*Instance`-compatible object; unaffected by the hub redesign except insofar as external sessions' hub instances need the same lifecycle (created on first attach, torn down on last detach) as managed sessions'. |

## 5. Data flow / consistency requirements

- **Hub restart while N subscribers attached** (e.g. the underlying `tmux -C` subprocess
  dies and needs relaunching): the existing `TmuxSession.controlModeExited` flag +
  pre-closed-channel behavior on `SubscribeToControlModeUpdates` (line 742) is the existing
  precedent for "tell new subscribers the stream already ended" — the hub needs the
  equivalent for *existing* subscribers on an unexpected mid-stream death: broadcast a
  sentinel/close, then each transport implementation decides whether to auto-reconnect
  (mirroring `ExternalStreamer.reconnect`, `external_streamer.go:472`, which already retries
  with backoff) or surface an error upstream (mirroring the browser client's own reconnect
  logic in `docs/adr/ADR-023-client-reconnect-browser-lifecycle.md`, which already handles
  full-stream teardown/reconnect with jittered backoff at the WebSocket layer). In-flight
  output at the moment of death has no stronger guarantee available than today: control-mode
  output already isn't transactionally tied to scrollback persistence, so "some bytes lost at
  the exact moment of subprocess death, recovered via the next capture-pane snapshot on
  restart" is the existing failure mode and is acceptable to keep, not stronger than the
  current per-connection code offers today.
- **Connection registry consistency**: requirements.md itself frames scale as "a handful of
  concurrent connections per session, single-operator instance" (Non-functional
  Requirements). Given that, **eventual/best-effort consistency is sufficient** for the
  registry — it exists for observability and connection *counting*, not for correctness of
  the resize/capture critical section itself (that section's correctness comes from having
  exactly one owner goroutine, a structural guarantee, not from the registry being
  linearizable). Requiring linearizability here would be over-engineering relative to the
  stated NFR ("this is about correctness and architectural flexibility... not high-throughput
  multi-tenant scale").

## 6. Migration / dark-launch failure modes (Complexity 4 focus)

The constraint is explicit: **no maintenance window, feature-flag dark-launch, live
single-instance production system, rollback must not require a code revert.** Failure modes
specific to *cutting over*, not to the steady-state design:

1. **Two owners of one tmux session during the flag-flip window.** If the flag is read
   per-connection (as `STAPLER_SQUAD_USE_CONTROL_MODE` is today, `os.Getenv` called fresh
   inside `streamViaControlMode`/its caller at lines 571 and 601 — re-read on every call, not
   cached), a config reload or a rolling per-request toggle could route connection A to the
   old per-connection path and connection B (same session) to the new hub path
   *simultaneously*. The old path's `ResizePTY`/`waitForPaneContent` and the new hub's owned
   resize/capture would then race exactly like today's bug, except now between two
   *architecturally different* code paths, which is harder to detect with a single
   generation counter. **Mitigation the plan must specify**: either (a) the flag is
   session-scoped and sticky for the session's lifetime (first connection to a session decides
   which path *all* connections to that session use, stored on `Instance` or a small
   per-session flag cache, not re-read per connection), or (b) the two paths are made mutually
   exclusive via the *same* refcounted ownership primitive already in `TmuxSession` — e.g. the
   new hub's creation acquires the same `controlModeStartMu`-class lock the old path's
   `StartControlMode` uses, so at most one "owner" (old-style connection or new-style hub)
   exists at a time regardless of which flag value spawned it. Option (b) is safer because it
   doesn't depend on every call site consistently reading a cached flag value.
2. **In-flight connections at flip time.** A connection accepted under the old flag value
   mid-stream must not be forcibly cut when the flag changes — the existing
   `activeControlModeStreams` generation/`doneStreaming()` cleanup pattern already models
   "let existing generations finish, only new ones see the new state," which the dark-launch
   plan should reuse: flip the flag for *new* connections only, let already-running
   `streamViaControlMode` invocations (old path) run to natural completion (client disconnect
   or session end), same as how `420584566`'s generation counter already tolerates overlap
   during natural reconnect windows without trying to force anything closed.
3. **Partial-rollout / per-session canary, not just global flag.** Because this is a
   single-operator instance, "canary a subset of sessions" is a more realistic dark-launch
   shape than "canary a subset of traffic": the operator can flip the new path on for one
   *session* (e.g. a disposable test session) while every other live session — including ones
   the operator is actively working in — stays on the old path. The flag design should
   support a per-session override on top of the global default (mirroring `ADR-023`'s
   `NEXT_PUBLIC_RECONNECT_V2` global-flag-with-fallback-guard pattern, but scoped finer given
   there's no separate staging environment here), not only a process-wide env var — a
   process-wide flip on the operator's own daily-driver instance means every session
   (including the one they're actively working in when they restart the service) flips at
   once, which is the exact blast radius `make install-service`'s existing WARNING in
   `CLAUDE.md`/`\.claude/docs/tmux-keep-server-on-restart.md` already flags as dangerous for
   an unrelated reason (tmux server kill) — this project's rollout should not introduce a
   *second*, independent way for one restart/flag-flip to disrupt every open session
   simultaneously.
4. **Rollback must not orphan hub state.** "Flip the flag back" (Risk Control) implies the
   hub for a session must be cleanly tearable-down independent of code deploy — if the hub
   holds the *only* reference to the control-mode subscription and gets abandoned (flag
   flipped back mid-session without closing the hub), the refcounted `StopControlMode` in
   `TmuxSession` never reaches zero and the control-mode subprocess leaks. The hub's shutdown
   path must be reachable both from "last subscriber detached" (steady state) and "flag
   flipped back while I'm still active" (rollback) — these need to be the *same* teardown code
   path, not two.
5. **No staging environment (Feasibility Risks, explicit)**: the dark-launch flag is the only
   safety net; there is no environment to catch a hub bug before it reaches the operator's own
   sessions. This raises the bar for the in-memory test transport (already in scope) to also
   exercise the two failure modes above (flag flip mid-session, hub crash with N subscribers)
   as unit/integration tests *before* any dark-launch toggle happens against the real
   instance — the requirements doc's Testability NFR should explicitly list "flag-flip
   mid-stream" and "hub death with attached subscribers" as required test scenarios, not just
   "runs without a real tmux process."

## 7. Prior art check — status of related project_plans

None of the nine directories checked contain a top-level `architecture.md` file named
exactly that outside of `research/architecture.md` (SDD's standard location) — all nine
*do* have `research/architecture.md`, so "prior art" below cites those plus their ADRs.

| Project | Shipped or planned-only? | Evidence | Overlap with this project |
|---|---|---|---|
| `terminal-jank` | **Shipped** | 3 ADRs (`ADR-001-terminal-instance-pool.md`, `ADR-002-xterm-upgrade-6.0.md`, `ADR-003-cold-start-quiescence.md`) — ADR-003's cold-start quiescence logic is the direct ancestor of `streamViaControlMode`'s `waitForQuiescence` (line 271) cited by name in requirements.md's NFRs. | The new hub must preserve this quiescence-detection behavior, now shared across N subscribers instead of computed per-connection. |
| `terminal-resize-fit-loop` | **Shipped** | `ADR-002-decoupled-sampler-tick-semantics.md` plus a full `implementation/` set (architecture-review, adversarial-review, pre-mortem, validation) — this is the "existing... decoupled sampler" the Open Questions section explicitly asks the batching-window design to be informed by. | Directly relevant to the batching/coalescing window design question (Open Questions #3) — read `ADR-002-decoupled-sampler-tick-semantics.md` in Phase 2/3 before choosing fixed-tick vs adaptive batching. |
| `terminal-redraw-corruption` | **Shipped** | `ADR-002-footprint-aware-redraw-coalescing.md` plus full `implementation/` set. | Coalescing-correctness precedent (Rabbit Holes: "naive frame coalescing can... break escape-sequence integrity") — this ADR's footprint-aware approach is exactly the kind of prior work the batching design must not contradict. |
| `terminal-resync-reliability` | **Shipped** | 6 ADRs including `ADR-005-control-mode-mid-stream-resync-handling.md` and `ADR-006-batching-scoped-to-same-connection-go-no-go.md` — the latter's title directly names the exact question this project must revisit (batching *across* connections/subscribers, not just within one). | **Must read `ADR-006` before finalizing the batching design** — it already made a go/no-go call on same-connection-scoped batching; this project explicitly wants cross-subscriber batching, so the ADR's rejected-alternatives section is likely to name why cross-connection batching wasn't done then, which may still apply. |
| `terminal-visibility-resync` | **Shipped** (full `implementation/` set, no `decisions/`) | `research/architecture.md`, `implementation/{plan,validation,pre-mortem,architecture-review,adversarial-review}.md` present. | Peripheral — visibility-triggered resync, not multi-connection ownership; check for overlap with the hub's "reconnect" handling if a subscriber comes back from background. |
| `new-renderer` | **Shipped** | Full `implementation/` set including a dedicated `architecture-performance-review.md`. | Client-side (xterm.js) rendering, not server-side streaming — low overlap; note only if the renderer makes assumptions about one-frame-per-message that the new batched wire protocol would break. |
| `terminal-robustness` | **Shipped** | 3 ADRs (`ADR-012` scrollback delivery, `ADR-013` iOS text selection, `ADR-014` mobile gesture state machine) + `implementation/{plan,validation}.md`. | `ADR-012-scrollback-delivery-strategy.md` overlaps the `ScrollbackManager` integration point (§4) — confirm the new hub's initial-scrollback-send-per-subscriber approach doesn't contradict this ADR's delivery strategy. |
| `ssh-remote-workspaces` | **Shipped** | 4 ADRs + full `implementation/` set. `ADR-002-commandrunner-in-session-tmux.md` (not "Workspace Host Registry" as requirements.md names it — see note below). | Cross-host framing precedent for "future consumer" (a cross-host socket transport) — but see caveat below. |
| `browser-passthrough` | **Shipped** (`implementation/{plan,validation}.md` present, no `decisions/`) | `research/architecture.md` present. | Check before building the browser WebSocket transport implementation for any passthrough assumptions about connection ownership. |

**Correction / gap found**: requirements.md's Users/Consumers section cites `ADR-002`'s
"Workspace Host Registry" as the basis for future cross-host streaming. **No ADR named or
themed "Workspace Host Registry" exists in this repo** — `docs/adr/` has no ADR-002 at all
(numbering starts at 003 for the low numbers; `ADR-002-commandrunner-in-session-tmux.md` and
`ADR-002-decoupled-sampler-tick-semantics.md` are project-scoped ADR-002s inside
`ssh-remote-workspaces` and `terminal-resize-fit-loop` respectively, neither about a host
registry), and `grep -rl "Workspace Host Registry"` across the repo returns nothing. This is
either a reference to a not-yet-written ADR the requester has in mind, or a misremembered
citation — **Phase 3 planning should flag this back to the requester rather than
silently inventing a Workspace Host Registry design to match the reference**, per this
project's own evidence-and-claims discipline (name the gap, don't smooth over it).

## 8. EventStorming — Events, Commands, Policies

Grammar: **Event** (past tense, something that happened) → **Policy** (reactive rule: "when
event X, do Y") → **Command** (imperative request into the hub/system).

| Domain Event | Triggering Command | Reactive Policy |
|---|---|---|
| `HubRequested` (first connection for a session with no live hub) | `AttachSubscriber(sessionID, transport)` | Policy: *if no hub exists for this tmux session, create one before attaching.* → emits `HubStarted`. |
| `HubStarted` | (internal — hub goroutine spawned) | Policy: *log hub creation with session ID and feature-flag state* (Observability Requirements: "hub creation... sufficient to reconstruct after the fact"). |
| `SubscriberAttached` | `AttachSubscriber` | Policy: *send catch-up content to the new subscriber* — either a fresh capture-pane snapshot (first subscriber) or the ring-buffer/cached-snapshot content (subsequent subscribers, avoiding a redundant capture-pane call per §1's root cause). Policy: *update connection registry count, log attach*. |
| `SubscriberDetached` | `DetachSubscriber(subscriberID)` — client disconnect, error, or explicit close | Policy: *update connection registry count, log detach.* Policy: *if this was the last subscriber, schedule `HubTearDown` after a grace period* (avoid immediately killing control-mode on a reconnect blip — mirrors the existing refcounted `StopControlMode`'s "only the last caller tears down" behavior, §2a). |
| `ResizeRequested` | `RequestResize(subscriberID, cols, rows)` | Policy: *apply the resize-negotiation model (open question — authoritative-subscriber vs. smallest-common-size) to decide the actual target dimensions*; only if the resolved target differs from the hub's current dimensions does it proceed to actually resize. Policy: *suppress broadcast to subscribers during the reflow* (mirrors existing `resizeSettling`, line 736). |
| `ResizeApplied` | (internal, hub calls `Instance.SetWindowSize`/`ResizePTY`) | Policy: *start quiescence detection*; emits `QuiescenceReached` or `QuiescenceTimedOut`. |
| `QuiescenceReached` | (internal) | Policy: *capture pane content once*, cache it, broadcast the authoritative post-resize snapshot to **all** attached subscribers (not just the requester) — this is the structural fix replacing per-connection capture. |
| `QuiescenceTimedOut` | (internal, 500ms deadline per existing `waitForQuiescence`) | Policy: *log a WARN (session may be stalled)* — reuses existing behavior at line 894, now hub-scoped instead of connection-scoped. |
| `BatchWindowElapsed` | (internal timer tick, or backpressure-driven per Open Question #3) | Policy: *flush the accumulated coalesced output buffer to all subscribers' transports as one message each* — this is the batching redesign target (Success Metric: "frame volume drops under concurrency"). |
| `OutputReceived` (raw tmux control-mode bytes) | (internal, from `TmuxSession.broadcastControlModeUpdate` or `ExternalStreamer.broadcast`) | Policy: *append to the current batch window's buffer; if not already pending, start/extend the batch timer.* |
| `HubCrashed` / `TmuxProcessDied` | (internal — control-mode subprocess exit detected) | Policy: *broadcast a stream-ended signal to all attached subscribers* (mirrors `controlModeExited`'s pre-closed-channel behavior for new subscribers, extended to existing ones); Policy: *attempt hub restart* if subscribers remain attached, else tear down cleanly. |
| `HubTornDown` | `TearDownHub(sessionID)` — last subscriber detach grace period elapsed, or session stopped/destroyed | Policy: *call `Instance.StopControlMode()` (refcount-safe teardown), remove from the hub registry, log teardown with final subscriber count for the observability trail.* |
| `FeatureFlagOldPathSelected` / `FeatureFlagNewPathSelected` | `ResolveStreamingPath(sessionID)` — evaluated once per session, not per connection (§6 mitigation) | Policy: *log the resolved path per session* (Observability Requirements: "feature-flag state... visible in logs per session"); Policy: *once resolved for a session, stick to that path for the session's lifetime — do not re-evaluate the env var per connection* (this is the migration-safety policy, not just an observability nicety). |
| `OverlapDetected` (the `420584566` WARN's condition) | — | Policy, post-redesign: *this event should become unreachable under the new hub path* (Success Metric); if it still fires, that indicates the flag-scoping-per-session mitigation (§6.1) failed, i.e. it converts from "passive log line" into the **hard invariant check** requirements.md's Observability Requirements calls for — recommend a `panic` in dev builds / `log.Error` + alert in prod, scoped specifically to "two hubs claim ownership of the same tmux session name," which is now a narrower, more actionable signal than today's per-connection generation counter. |

## Summary of open items this research surfaces for Phase 3 planning

- Decide whether to extract/generalize `ExternalStreamer` into the shared hub type (§2b) or
  build a new type both paths adopt — recommend evaluating the former first, since it already
  matches the target shape closely.
- Decide which of the three existing "how many connections" trackers
  (`activeControlModeStreams`, `TmuxSession.controlModeSubscribers`,
  `ExternalStreamer.consumers`) the new hub's registry subsumes vs. leaves as internal
  plumbing — the Observability Requirements imply exactly one should be the source of truth.
- The per-session (not per-connection, not per-process) feature-flag scoping and the shared
  refcounted-ownership-lock approach (§6.1) are migration-safety requirements, not
  implementation nice-to-haves — they should be written into the Phase 3 plan's acceptance
  criteria explicitly, since a flag that's merely "off by default" doesn't by itself prevent
  the two-owners-during-flip-window failure mode.
- Flag the "ADR-002 Workspace Host Registry" citation gap (§7) back to the requester before
  Phase 3 designs around it.
- Read `terminal-resync-reliability/decisions/ADR-006-batching-scoped-to-same-connection-go-no-go.md`
  and `terminal-resize-fit-loop/decisions/ADR-002-decoupled-sampler-tick-semantics.md` before
  finalizing the cross-subscriber batching design (Open Question #3).
