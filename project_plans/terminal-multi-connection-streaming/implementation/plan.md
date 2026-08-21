# Implementation Plan: terminal-multi-connection-streaming

**Feature**: Single-owner per-session stream hub + transport abstraction + batched wire protocol, replacing per-connection resize/capture ownership in the tmux control-mode streaming path.
**Date**: 2026-08-20
**Status**: Ready for implementation
**ADRs**: ADR-001 (single-owner stream hub), ADR-002 (resize negotiation model), ADR-003 (per-session sticky flag scoping), ADR-004 (ssq-mux output-only transport, resize unification deferred)

---

## Step 0.5 — Alternatives Explored (CREATIVE pass)

Three distinct high-level approaches were considered before committing:

1. **Per-session mutex around the existing resize+capture critical section** (the alternative requirements.md says was already recommended pre-SDD). Strength: smallest possible diff, fixes the race with one lock. Weakness: doesn't touch transport coupling or give a home for a transport-agnostic interface — the requester's explicit "stream wherever" goal goes unmet, and a future consumer still has to be special-cased into `streamViaControlMode`.
2. **Full multi-sink platform**: build the hub, the transport interface, *and* every named future sink (audit, webhook, SSE, cross-host) in one pass. Strength: interface gets stress-tested against maximum variety immediately. Weakness: designs the interface against sinks that don't exist yet and blows the "foundational, phased" appetite the requester explicitly set.
3. **Single-owner actor-model hub + narrow Transport interface, foundational scope only** (chosen). Strength: matches Go's idiom for "one writer, many readers of evolving state," and — critically — this shape is already half-built twice in this repo (`TmuxSession`'s refcounted control-mode ownership, `ExternalStreamer`'s consumer registry), so this is generalization of proven code, not a blank-page design. Weakness: still a rewrite of the most failure-visible code path in the product, on a live single-instance deployment with no staging environment — mitigated by the dark-launch flag and sticky per-session ownership lock (Phase 3).

Approach 3 is adopted. Approaches 1 and 2 are recorded in the Pattern Decisions table below as rejected alternatives at the component level where they recur (e.g., approach 1 recurs as "distributed lock" rejected for the hub itself; approach 2 recurs as "build all sinks now" rejected for scope).

---

## Domain Glossary
*(Ubiquitous language — every domain term that appears as a type, method, or variable name. Use these exact names in code, tests, and comments.)*

| Term | Definition | Notes |
|------|-----------|-------|
| `StreamHub` | The single-owner runtime object that performs resize + quiescence-wait + capture-pane + broadcast exactly once per tmux session, regardless of subscriber count. One instance per tmux session name, created lazily on first attach. | Actor-model owner; new type, package `session/streamhub` |
| `HubRegistry` | The process-wide `xsync.Map[string, *StreamHub]` keyed by tmux session name; hands out the one `StreamHub` for a name via a `Compute`-based get-or-create. | Generalizes `activeControlModeStreams`'s existing `xsync.Map` shape |
| `Subscriber` | The hub-side handle for one attached `Transport`: holds a bounded outbound channel, a `SubscriberCapability`, its last `ResizeVote`, and a `SubscriberID`. | One per attached connection/transport instance |
| `SubscriberID` | Newtype wrapping a UUID string identifying one `Subscriber`; never a bare `string` parameter alongside other string IDs. | Per `primitive-obsession-checklist.md` |
| `Transport` | The narrow interface (`Send([]byte) error`, `Close() error`) a consumer type implements to attach to a `StreamHub`. Defined in the hub's consuming package, scoped to only the methods the hub calls. **Error contract**: a non-nil error from `Send` is treated identically to the slow-subscriber-eviction path (Story 1.2.1) — log, evict, close, remove from registry. `Send` failing is just another way for a subscriber to become unable to receive; it is never hub-fatal. | Per `interface-pollution-checklist.md` — three implementations this pass: `WebSocketTransport`, `MemoryTransport`, `MuxTransport` |
| `SessionController` | The narrow interface (`SetWindowSize(cols, rows int) error`, `ResizePTY(cols, rows int) error`, `CapturePaneContent() (string, error)`, `StopControlMode() error`, `SubscribeControlModeUpdates() (string, <-chan []byte)`, `UnsubscribeControlModeUpdates(id string)`) that `StreamHub` depends on instead of the concrete `*session.Instance` type. Declared in `session/streamhub`, scoped to exactly the `Instance` methods (`session/instance_tmux.go:587,600,722,727,733,738,752`) the hub calls. `*session.Instance` satisfies it structurally — `session/streamhub` never imports package `session`. | Per `interface-pollution-checklist.md` ("define the interface where it's consumed") and Dependency Inversion; the concrete cycle-avoidance mechanism for ADR-001/Story 1.3.2 — see Pattern Decisions row below |
| `SubscriberCapability` | A struct `{CanResize bool, CanWrite bool}` attached to a `Subscriber` at attach time; gates whether it gets a `ResizeVote` and whether its input is forwarded to the pane. | Prevents a future read-only sink from ever shrinking the shared pane |
| `TerminalSize` | A validated value object `{Cols, Rows int}` constructed only via `NewTerminalSize(cols, rows int) (TerminalSize, error)`, which rejects non-positive dimensions. The single shared representation for a pane size — used by `ResizeVote`, `NegotiatedSize`, and `RequestResize`'s parameter; never a bare `int, int` pair or an independently-inlined `{Cols, Rows int}` struct. | Per `primitive-obsession-checklist.md` — replaces what were three independent inline `{Cols, Rows int}` shapes |
| `ResizeVote` | A `{SubscriberID, TerminalSize}` tuple submitted by a capability-eligible `Subscriber`. | Never-resized subscribers default their vote to the current `NegotiatedSize` (itself a `TerminalSize`), not a hardcoded 80×24 (GoTTY-bug avoidance, `research/features.md` §2) |
| `NegotiatedSize` | The `StreamHub`'s current resolved `TerminalSize` — component-wise minimum (`Cols`, `Rows` each independently) across all live `ResizeVote`s from `CanResize` subscribers. | Smallest-common-size model (ADR-002) |
| `HubLifecycleState` | Sum type (Go: typed int with exhaustive `switch`): `HubStarting` / `HubActive` / `HubDraining` / `HubTornDown`. | Every state-transition method must exhaustively handle all four |
| `StreamPath` | Sum type: `PathLegacyPerConnection` / `PathHubOwned`. Resolved exactly once per tmux session at first-attach time and stored sticky for that session's lifetime. | Prevents the two-owner flag-flip race (`research/architecture.md` §6.1) |
| `StreamOwnershipLock` | The per-tmux-session mutex (generalizing `TmuxSession.controlModeStartMu`) that both the legacy per-connection path and hub creation must acquire before deciding/joining a `StreamPath`. Owned by package `session/streamhub` (type `StreamOwnershipLock` plus a package-level `xsync.Map[string, *StreamOwnershipLock]` keyed by tmux session name, exposed via `streamhub.AcquireOwnershipLock(sessionName string) *StreamOwnershipLock`). Package `session`'s legacy `StartControlMode` call site imports `session/streamhub` and calls this function directly — a one-way dependency (`session` → `session/streamhub`) that does not cycle, because `session/streamhub` depends only on `SessionController` and never imports package `session`. | Structural mitigation, not a convention; see Pattern Decisions row below for the full cycle-avoidance rationale |
| `BatchWindow` | The `StreamHub`'s single shared accumulation buffer + timer for coalescing `OutputReceived` events before a flush. Opportunistic-with-ceiling: flush at the next subscriber-write opportunity or `MaxBatchWindow` (20ms), whichever is first. | One hub-owned timer, never per-subscriber (ADR-002 decoupled-sampler precedent) |
| `HubSequenceNumber` | Monotonic `uint64` stamped on every broadcast unit (batched frame or bypass message) at hub-broadcast time. | Gives every `Subscriber` the same total order regardless of individual flush cadence |
| `CatchUpSnapshot` | The cached capture-pane content (or `ExternalStreamer` ring-buffer content) sent to a newly-attached `Subscriber` instead of re-running capture-pane. | Avoids the redundant-capture root cause named in requirements.md's Baseline |
| `OverlapInvariant` | The post-redesign hard-invariant check (successor to the `420584566` WARN) asserting no two owners (legacy connection or hub) ever hold resize/capture authority for one tmux session concurrently. | Always emits `slog.Error` with full context — never `panic` in the running binary, since this codebase has no dev/prod build-mode distinction and a panic in a live single-operator daily-driver process is worse than the bug it would catch; every `-race` test that could trigger it (e.g. Story 1.4.2's flag-flip race test, Story 3.3.3's storm test) explicitly fails via `t.Fatal` if it fires during test execution, making it a hard test-time invariant without new production build-mode machinery |

**Glossary term count: 17.**

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| `StreamHub` | Single-owner actor (goroutine + channels own all mutable resize/capture state) | Go concurrency idiom, `golang-concurrency` skill | Distributed lock around the existing per-connection critical section (Approach 1 above) | Fixes the race but not the transport-coupling/"stream wherever" goal; kept as an internal *component* (`StreamOwnershipLock`) inside the chosen design instead |
| `HubRegistry` | Lock-free concurrent map (`xsync.Map`) with `Compute`-based get-or-create | Already used identically for `activeControlModeStreams`/`snapshotCache` in this file | `sync.RWMutex` + plain `map` | Avoids RWMutex contention between register/unregister and broadcast (`research/stack.md` §3); repo precedent already uses `xsync.Map` one file over |
| `Transport` | Narrow consumer-defined interface (`Send`/`Close`) | `interface-pollution-checklist.md` | Generic pub/sub broker (embedded NATS) | Wrong scale — single-process fan-out of "a handful of connections," not a distributed broker problem (`research/build-vs-buy.md` §1a) |
| `SessionController` (hub's dependency on `*session.Instance`) | Dependency Inversion: `session/streamhub` defines a local, narrow `SessionController` interface (`SetWindowSize`/`ResizePTY`/`CapturePaneContent`/`StopControlMode`/`Subscribe`-`UnsubscribeControlModeUpdates`) and depends only on it; `*session.Instance` satisfies it structurally, with no import of package `session` from `session/streamhub`. `StreamOwnershipLock` is likewise owned by `session/streamhub` (package-level `xsync.Map[string, *StreamOwnershipLock]`), and package `session`'s legacy `StartControlMode` call site imports `session/streamhub` (one-way) to acquire the same lock instance per session name | `interface-pollution-checklist.md` ("define the interface where it's consumed"); `golang-development` DIP guidance | (a) `session/streamhub` imports package `session` directly for the concrete `*Instance` type; (b) `StreamOwnershipLock` living in package `session` with `session/streamhub` importing it | Both rejected alternatives create a two-way package dependency: `session/streamhub` → `session` (for `Instance`) and `session` → `session/streamhub` (for the shared lock or hub registry) is a Go import cycle, a compile failure, not a style issue. Defining `SessionController` in `session/streamhub` and having `*session.Instance` satisfy it structurally removes the only reason `session/streamhub` would need to import `session`, making `session` → `session/streamhub` a safe one-way dependency; this also gives Stories 1.2.2/1.3.2's planned "fake Instance" test doubles a concrete contract to implement |
| Subscriber fan-out | Per-subscriber writer goroutine draining a bounded channel; non-blocking send from the hub | GoF Observer (adapted); repo precedent `NativeProcessManager.fanOut` | Sequential shared-lock loop over all subscribers on the hub's own goroutine (`Multiplexer.broadcastToClients`'s existing shape) | A slow/stalled subscriber serializes up to `N × writeDeadline` onto every other subscriber (`research/pitfalls.md` §1a) — already a live bug in `session/mux/multiplexer.go:706-720` |
| Session→hub resolution | State/Strategy (`StreamPath` sum type, resolved once, sticky) | Type-driven design | Env var re-read per connection (today's `STAPLER_SQUAD_USE_CONTROL_MODE` pattern) | Re-reading per connection lets two ownership models race the same session mid-flip (`research/pitfalls.md` §3a) |
| Resize authority | Smallest-common-size negotiation over capability-gated `ResizeVote`s | tmux's own default (`research/features.md` §1) | Authoritative-single-subscriber (VS Code Live Share model) | No trust asymmetry exists here (single operator, no untrusted guests) — smallest-common avoids one subscriber silently clipping another's content; VS Code's capability-gating idea is adopted separately for read-only sinks |
| Output batching | Single hub-owned opportunistic-with-ceiling timer, adapting the existing `coalesceBufPool` buffer-pooling pattern (`server/services/connectrpc_websocket.go:790-845`) for accumulation and reusing `pkg/ansi`'s existing escape-sequence-safe filtering for any byte-level handling, rather than designing either from scratch | `research/build-vs-buy.md` §3b (explicit recommendation to lift and generalize `coalesceBufPool`/`pkg/ansi` rather than redesign batching from a blank page); `terminal-resize-fit-loop` ADR-002 (decoupled fixed-cadence sampler) for the timer shape | Per-subscriber independent flush tickers; designing a new buffer-pooling/escape-handling scheme from scratch | Independent timers drift out of phase over a long session (`research/features.md` §2); `coalesceBufPool` and `pkg/ansi` are already-shipped, escape-sequence-safe, and avoid per-flush allocation — reimplementing that logic from scratch would ignore this project's own build-vs-buy research and risk reintroducing bugs `pkg/ansi` already handles |
| Batching *scope* (this project vs. prior art) | Cross-subscriber output-frame coalescing, a new and distinct axis from `terminal-resync-reliability`'s ADR-006 | This project's own read of `terminal-resync-reliability/decisions/ADR-006-batching-scoped-to-same-connection-go-no-go.md` | Treat ADR-006 as already having settled this project's batching question | ADR-006 scoped batching to coalescing multiple `CurrentPaneRequest`s *within one connection* (a request-coalescing decision, explicitly deferring cross-connection batching's go/no-go pending compression benchmarks) — a different axis from this project's cross-*subscriber* output-broadcast coalescing. ADR-006 does not block this project's batching design, but its go/no-go-with-explicit-decision-point discipline is followed here too (see Pattern Decisions row above and ADR entries) rather than silently treating batching as an obviously-fine default |
| Batching content policy | Concatenate opaque byte ranges, never content-sniff | `terminal-redraw-corruption` pitfalls §4a (RedrawThrottler bug) | Semantic "this redraw supersedes that one" frame-dropping | A content-sniffing coalescer already caused a real shipped corruption bug in `RedrawThrottler`; opaque concatenation carries none of that risk |
| `SubscriberID`, session identifiers | Newtypes with validating constructors | `type-driven-design`, `primitive-obsession-checklist.md` | Bare `string` parameters | Prevents a `SubscriberID`/tmux-session-name swap at a call site from silently compiling |
| `TerminalSize` (`RequestResize`, `ResizeVote`, `NegotiatedSize`) | Single validated value object (`NewTerminalSize(cols, rows int) (TerminalSize, error)`, rejects non-positive dimensions), shared across all three call sites | `type-driven-design`, `primitive-obsession-checklist.md` | Bare `cols, rows int` parameters on `RequestResize`, plus independently-inlined `{Cols, Rows int}` structs on `ResizeVote` and `NegotiatedSize` | `RequestResize(id, cols, rows int)` lets a caller silently transpose `cols`/`rows` (compiles, corrupts the pane); the three independent inline shapes also duplicate the same concept three times with no shared validation |
| `HubLifecycleState`, `StreamPath` | Sum types with exhaustive `switch` | `type-driven-design` | Boolean flags (`isActive`, `isLegacy`) | Boolean pairs admit invalid combinations (`isActive=false, isLegacy=false` meaning nothing); sum types make illegal states unrepresentable |
| `StreamHub` internals vs. `ExternalStreamer` | Build `StreamHub` fresh (Phase 1), generalizing the *pattern* `ExternalStreamer` already proves (owner + registry + ring-buffer catch-up + broadcast), rather than merging `ExternalStreamer`'s actual code into `StreamHub` | `research/architecture.md` §2b (recommends evaluating extraction first) | Literally extract/rename `ExternalStreamer` into the shared hub type, then have ssq-mux and tmux control-mode both become instances of it | `ExternalStreamer` is coupled to a raw `net.Conn`/socket-reconnect model (`connect`/`reconnect`, `session/external_streamer.go`) that has nothing to do with tmux control-mode's resize/quiescence/capture-pane pipeline (Epic 1.3) — merging them would force the tmux-control-mode path to carry ssq-mux's connection-lifecycle concerns (and vice versa) for no shared benefit. Building fresh and adapting `ExternalStreamer` in as a `Transport` (below) gets the "zero changes to hub logic" Success Metric bar directly, which literal extraction would not have made easier |
| ssq-mux integration | Adapter (GoF) wrapping the existing `ExternalStreamer` as a `Transport`, output-only this pass | GoF Adapter; `research/architecture.md` §2b's extraction recommendation | Full rewrite of `Multiplexer.handleClient`'s resize authority to route through the hub | Named in Rabbit Holes/pitfalls §4b as a possible multi-week redesign on its own; deferred as its own follow-on phase (ADR-004) rather than blocking this pass's two other required transports |
| Wire framing | Keep hand-rolled ConnectRPC-envelope-over-`gorilla/websocket` | `research/stack.md` §6 | Real `connect.BidiStream` | Needs HTTP/2 (TLS or h2c), a deployment change out of appetite for this pass; documented as a deferred follow-on transport |
| Scope breadth | Foundational hub + 3 transports only | Appetite section, requirements.md | Build all named future sinks now (Approach 2 above) | Designing the interface against sinks that don't exist yet risks getting the interface wrong; explicitly deferred per Appetite |

---

## Migration Plan

**N/A — no schema or persisted-data changes.** This is a runtime/in-memory architecture change (new Go types, a new goroutine-owned hub per session, a feature flag). No SQL migration, no `session/ent/schema` change, no on-disk format change to `config.json`/`sessions.json`. The operationally-equivalent concern — cutting a live, stateful system over to a new ownership model with no maintenance window — is covered under **Risk Control** and **Phase 3** below (sticky per-session `StreamPath` resolution, `StreamOwnershipLock`, dark-launch flag), which serves the same "safe transition" purpose a schema migration plan would serve for data.

## Observability Plan

- **Logs** (structured, `slog`), one line per event, at:
  - `StreamHub` creation/teardown (`HubLifecycleState` transition), including `tmux_session`, `resolved_path` (`StreamPath`), and subscriber count at teardown.
  - Subscriber attach/detach: `subscriber_id`, `transport_type`, `capability` (`CanResize`/`CanWrite`), resulting subscriber count.
  - Resize-negotiation decisions: incoming `ResizeVote`s, resulting `NegotiatedSize`, whether it changed from the prior value.
  - `BatchWindow` flush: frames coalesced, byte count, flush trigger (`opportunistic` vs `ceiling`).
  - `OverlapInvariant` violation (should never fire post-cutover): `tmux_session`, both claimed owners' identifying info.
  - `StreamPath` resolution per session (which path, and why — flag value + any per-session override).
- **Metrics** (one entry per new operation, Atlas/OTel per this repo's existing `connectrpc.com/otelconnect` wiring):
  - `streamhub_active_hubs` (gauge) — count of live `StreamHub`s.
  - `streamhub_subscribers_per_hub` (histogram) — fan-out width, replaces ad hoc counting across three legacy registries.
  - `streamhub_resize_negotiations_total` (counter, tagged `changed=true/false`).
  - `streamhub_batch_flush_frames_coalesced` (histogram) — records how many underlying output events are folded into a single hub-side coalesce/flush; validates the "duplicated per-connection coalescing eliminated" success metric against the concrete target set in Story 2.1.1: under N≥3 concurrent subscribers receiving the same 50-event burst within one `MaxBatchWindow` (20ms) tick, the hub's accumulation/coalesce step must execute exactly once for the burst (an Nx reduction versus today's per-connection design, where each of N subscribers runs its own coalesce loop against the same burst) — this metric is about eliminating redundant coalescing computation, not reducing wire-message count.
  - `streamhub_slow_subscriber_drops_total` (counter) — backpressure visibility (`research/pitfalls.md` §2 backpressure edge case).
  - `streamhub_overlap_invariant_violations_total` (counter) — must stay 0 after cutover; this is the load-bearing regression signal.
- **Alerts**: this is a single-operator personal instance with no external paging system (confirmed — no PagerDuty/Atlas-alert integration in this repo's docs). `streamhub_overlap_invariant_violations_total > 0` is surfaced as an `slog.Error`-level log line in the existing log stream the operator already greps (`~/.stapler-squad/logs/stapler-squad.log`) — this is the alert channel for this project, not a new paging integration.

## Risk Control

- **Feature flag**: `STAPLER_SQUAD_USE_STREAM_HUB` (default `false`). Unlike `STAPLER_SQUAD_USE_CONTROL_MODE` (read fresh per call), this flag is read **once per tmux session, at first-attach time**, and the resulting `StreamPath` is cached sticky for that session's lifetime (Phase 3, Story 3.1.1) — never re-evaluated per connection.
- **Per-session override**: an operator can force `PathHubOwned` for one disposable/test session without flipping the global default (Phase 3, Story 3.3.1) — the realistic "canary" shape for a single-operator instance (`research/architecture.md` §6.3).
- **Rollback procedure**: flip `STAPLER_SQUAD_USE_STREAM_HUB` back to `false` — affects only *new* sessions' `StreamPath` resolution (existing hubs finish their lifecycle under whichever path they started with, per `StreamOwnershipLock`'s sticky-resolution guarantee). A full code revert via PR remains the second-line fallback.
- **Rollback rehearsal (mandatory, one-time, mechanically enforced — not a checklist line)**: Phase 3 Story 3.3.2 requires actually exercising a flip-on → use briefly → flip-off → confirm-clean-reconnect cycle against a real, disposable session before the flag is ever turned on for a session the operator depends on — per `research/pitfalls.md` §5 point 4, the first exercise of a rollback path should not be a real incident. Per pre-mortem P1 #4, this is enforced in code, not left as an optional process step: completing the rehearsal (Task 3.3.2c) persists a `rollback_rehearsal_completed_at` timestamp to the existing per-session/global config store (`config/config.go`, alongside the per-session override added in Task 3.3.1b); the flag-resolution code that would set `STAPLER_SQUAD_USE_STREAM_HUB`'s *global default* to `true` (Task 3.3.1d) refuses to do so — returning a startup error, not silently falling back — if `rollback_rehearsal_completed_at` is unset. The per-session override (canary) path is unaffected by this gate; only flipping the *global default* is blocked.
- **Staged rollout**: per-session override (canary) → operator's own daily-driver sessions under the global default, opt-in → global default flips to `true` only after both the rollback rehearsal gate above and the trial period below are satisfied → the real multi-session reconnect-storm test (Story 3.3.3) is green.
- **`420584566` WARN stays live and unmodified** until the hub path is the default (`research/pitfalls.md` §5 point 2) — it remains the only after-the-fact signal for the legacy path's race for the entire dark-launch window.
- **Trial period before old-path removal (traffic-weighted, not vacuous)**: 14 consecutive calendar days of the operator's own daily-driver use with the hub path as the *effective* path for all new sessions, **and** at least 20 distinct tmux sessions (or 50 cumulative session-hours, whichever threshold is reached first — chosen against this single-operator instance's own daily-driver usage pattern, not an arbitrary round number) actually served via `PathHubOwned` during that window, with zero `streamhub_overlap_invariant_violations_total` and zero `420584566` WARNs logged across that real traffic, **and** the rollback rehearsal gate (above) already satisfied. The prior "14 days, zero WARNs" bar alone was near-vacuous — it would be trivially satisfied by 14 days in which the hub path received no real traffic at all; the added session-count/session-hours floor requires the zero-violations result to have actually been tested against real usage. Only after this bar is met does old-path removal become its own separate, later change — this plan does not include deleting `streamViaControlMode`'s legacy branch.

## Unresolved Questions

- [x] **Citation gap — resolved, not a fictional ADR**: requirements.md's Users/Consumers section originally cited "`ADR-002`'s Workspace Host Registry" for future cross-host streaming with no path, which `research/architecture.md` §7 could not locate anywhere in this repo's own `docs/adr/` or `project_plans/terminal-multi-connection-streaming/decisions/`. Follow-up investigation found the real source: `project_plans/backlog-deep-linking/decisions/ADR-002-gossip-based-host-registry.md` — a genuine, accepted ADR, but belonging to a **different** SDD project (`backlog-deep-linking`), not this one. requirements.md's three citations have been corrected to reference that ADR by full path and explicitly note it is not this project's own ADR. **Naming-collision note** (flagged during triad review, non-blocking): this project's *own* `decisions/ADR-002-resize-negotiation-smallest-common-size.md` shares the number "ADR-002" with the unrelated `backlog-deep-linking` ADR being cited — both numbers are correct within their own project's sequence (SDD numbers ADRs per-project, not globally), but a skim reader seeing "ADR-002" in this project's docs should check the full path, not assume it means this project's own resize-negotiation ADR. This plan does not design around cross-host streaming (out of scope for this pass); the citation is now accurate context, not an open gap.
- [ ] ssq-mux's own multi-IDE-terminal resize race (`Multiplexer.handleClient`'s unmediated `SetWindowSize` calls) is explicitly **not** fixed by this pass (ADR-004) — it is a pre-existing, unchanged behavior, not a regression, but it means the browser+ssq-mux combination on one session is not yet fully race-free. Blocks: a future "ssq-mux resize-authority unification" follow-on phase — owner: whoever picks up that phase, informed by `research/pitfalls.md` §4b's scoping.
- [ ] `MaxBatchWindow = 20ms` (Phase 2, Story 2.1.1) is a starting value chosen from qualitative reasoning (below the "feels instantaneous" ~100ms threshold cited in `terminal-resize-fit-loop`'s own UX research), not a live measurement. Blocks: nothing structurally — it's a tunable constant — but should be revisited once the hub path has real dark-launch traffic. Owner: whoever validates Phase 2 against `terminal-jank`/`terminal-resize-fit-loop`'s prior "acceptable jank" bar (Phase 4/validation step of this SDD project).
- [ ] Whether `coder/websocket` ever replaces `gorilla/websocket` for the `WebSocketTransport` implementation is explicitly deferred (`research/stack.md` §4) — not blocking, no story depends on it.
- [ ] Exact wording/placement of the connection-count indicator's tooltip copy for "another connection has this session open at a different size" (Phase 4, Story 4.2.2) is sized in `research/ux.md` but final copy is a small design decision left to implementation — owner: implementer, low-risk, does not block the story's AC.

## Dependency Visualization

```
Phase 1 — Hub Core (in-memory only, no production wiring)
┌─────────────────────────────────────────────────────────────────┐
│ Epic 1.1 Core types & Transport interface                       │
│   Story 1.1.1 ──┐                                                │
│   Story 1.1.2 ──┼─▶ Epic 1.2 Subscriber lifecycle & fan-out      │
│                 │      Story 1.2.1 ──┐                            │
│                 │      Story 1.2.2 ──┼─▶ Epic 1.3 Resize/capture │
│                 │                    │      Story 1.3.1 ──┐      │
│                 │                    │      Story 1.3.2 ──┼─▶ Epic 1.4 │
│                 │                    │                    │  In-memory transport + tests │
│                 │                    │                    │      Story 1.4.1 ──┐          │
│                 │                    │                    │      Story 1.4.2 ──┘ (needs 1.4.1) │
└─────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼ (Phase 1 must be green before Phase 2 wires production traffic)
Phase 2 — Batching & Browser WebSocket transport
┌─────────────────────────────────────────────────────────────────┐
│ Epic 2.1 Batch window & sequencing                               │
│   Story 2.1.1 ──▶ Story 2.1.2 ─┐                                 │
│                                 ▼                                 │
│ Epic 2.2 Browser WebSocket transport cutover                     │
│   Story 2.2.1 ──▶ Story 2.2.2 (behind STAPLER_SQUAD_USE_STREAM_HUB, defaults off) │
└─────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
Phase 3 — Dark-launch flag, ownership lock, registry consolidation, observability
┌─────────────────────────────────────────────────────────────────┐
│ Epic 3.1 StreamPath resolution & StreamOwnershipLock              │
│   Story 3.1.1 ──▶ Story 3.1.2                                    │
│        │                                                          │
│        ▼                                                          │
│ Epic 3.2 Registry consolidation & observability                   │
│   Story 3.2.1 ──▶ Story 3.2.2                                    │
│        │                                                          │
│        ▼                                                          │
│ Epic 3.3 Rollout mechanics & rollback rehearsal                   │
│   Story 3.3.1 ──▶ Story 3.3.2  ─┐                                 │
│                ──▶ Story 3.3.3 ─┴─▶ (both gate before wider rollout, │
│                                      i.e. before global default = true) │
└─────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼ (independent of Phase 3's completion for its own dev work,
                                       but should not ship to production ahead of Phase 3's flag machinery)
Phase 4 — ssq-mux output-only transport & UI connection indicator
┌─────────────────────────────────────────────────────────────────┐
│ Epic 4.1 ssq-mux Transport adapter (output-only)                 │
│   Story 4.1.1 ──▶ Story 4.1.2                                    │
│                                                                    │
│ Epic 4.2 UI connection-count indicator (independent of 4.1)      │
│   Story 4.2.1 ──▶ Story 4.2.2                                    │
└─────────────────────────────────────────────────────────────────┘
```

---

## Phase 1: Hub Core (in-memory only)

**Note on task granularity**: Tasks in this plan are sub-story implementation steps sized for rapid iteration (2-5 min each); Stories are the practical atomic unit for progress tracking and dependency sequencing (each bounded to ~15-60 min, with its own acceptance criteria and file list). Track and sequence work at the Story level — the Task breakdown exists to make each Story's implementation concrete, not to serve as the tracking granularity itself.

### Epic 1.1: Core types & Transport interface
**Goal**: Establish the vocabulary (Domain Glossary types) and the `Transport` interface so every later epic builds against a stable contract.

#### Story 1.1.1: Define `Transport` interface, `SubscriberCapability`, and identifier newtypes
**As a** developer implementing a new consumer of a tmux session's stream, **I want** one small interface to satisfy, **so that** I don't have to touch hub internals.
**Acceptance Criteria**:
- A `Transport` interface with exactly `Send([]byte) error` and `Close() error` exists in `session/streamhub/transport.go`.
  - *Given* a new package `session/streamhub` with no existing types, *When* `transport.go` is added declaring `type Transport interface { Send([]byte) error; Close() error }`, *Then* `go build ./session/streamhub/...` succeeds with zero exported methods beyond those two.
- `SubscriberID` and `SubscriberCapability` are newtypes, not bare primitives.
  - *Given* the constructor `NewSubscriberID() SubscriberID` (wraps `uuid.NewString()`), *When* two `Subscriber`s are created via `NewSubscriberID()`, *Then* their `SubscriberID` values are distinct and neither compiles as interchangeable with a plain `string` parameter elsewhere in the package (verified by `go vet`/compile, not by convention).
**Files**: `session/streamhub/transport.go`, `session/streamhub/types.go`

##### Task 1.1.1a: Create `session/streamhub` package skeleton with `doc.go` (~3 min)
- Create the directory and a `doc.go` describing the hub's single-owner responsibility in one paragraph (per proportionality — no essay).
- Files: `session/streamhub/doc.go`

##### Task 1.1.1b: Define `Transport` interface (~3 min)
- Add `Transport` interface exactly as specified above.
- Files: `session/streamhub/transport.go`

##### Task 1.1.1c: Define `SubscriberID` newtype + constructor (~4 min)
- `type SubscriberID string`, `func NewSubscriberID() SubscriberID`.
- Files: `session/streamhub/types.go`

##### Task 1.1.1d: Define `SubscriberCapability` struct (~3 min)
- `type SubscriberCapability struct { CanResize bool; CanWrite bool }`.
- Files: `session/streamhub/types.go`

#### Story 1.1.2: Define `HubLifecycleState` and `StreamPath` sum types
**As a** developer reading hub state transitions, **I want** exhaustively-handled enums, **so that** an invalid combination can't compile.
**Acceptance Criteria**:
- `HubLifecycleState` has exactly four values and every `switch` over it in the package has a `default: panic("unhandled HubLifecycleState")` case (compile-time exhaustiveness check via `exhaustive` linter, already available per `make analyze`).
  - *Given* `HubLifecycleState` values `HubStarting, HubActive, HubDraining, HubTornDown`, *When* `make lint` runs the `exhaustive` check against `session/streamhub/hub.go`, *Then* it reports zero missing-case findings.
- `StreamPath` has exactly two values.
  - *Given* `StreamPath` values `PathLegacyPerConnection, PathHubOwned`, *When* a new call site adds a third case without updating every switch, *Then* `make lint` fails on the exhaustiveness check.
**Files**: `session/streamhub/types.go`

##### Task 1.1.2a: Define `HubLifecycleState` type + constants (~3 min)
- Files: `session/streamhub/types.go`

##### Task 1.1.2b: Define `StreamPath` type + constants (~3 min)
- Files: `session/streamhub/types.go`

##### Task 1.1.2c: Add `exhaustive` linter check to `.golangci.yml` for `session/streamhub` if not already repo-wide (~4 min)
- Confirm `make lint` already runs `exhaustive`; if scoped/excluded for new packages, add it.
- Files: `.golangci.yml` (only if change needed)

---

### Epic 1.2: Subscriber lifecycle & fan-out
**Goal**: A `StreamHub` can accept/remove `Subscriber`s and fan out bytes to each via a non-blocking, per-subscriber-goroutine path that never lets one slow subscriber stall another (`research/pitfalls.md` §1a/§1b).

#### Story 1.2.1: Attach/detach subscribers with bounded per-subscriber channel + writer goroutine
**As a** `StreamHub`, **I want** to add and remove `Subscriber`s without ever blocking on a slow one, **so that** N-1 fast subscribers aren't punished by 1 slow one.
**Acceptance Criteria**:
- `AttachSubscriber(transport Transport, cap SubscriberCapability) SubscriberID` registers a new `Subscriber`, starts its writer goroutine, and returns its ID.
  - *Given* a `StreamHub` in `HubActive` state with zero subscribers, *When* `AttachSubscriber(memTransport, SubscriberCapability{CanResize: true})` is called, *Then* `hub.SubscriberCount()` returns `1` and a `CatchUpSnapshot` is sent to `memTransport` within the same call (see Story 1.3.2 for snapshot source).
- `DetachSubscriber(id SubscriberID)` removes the subscriber and stops its writer goroutine without leaking it.
  - *Given* a `StreamHub` with one attached `Subscriber` whose `SubscriberID` is `"sub-1"`, *When* `DetachSubscriber("sub-1")` is called, *Then* `hub.SubscriberCount()` returns `0` and a `goleak.Find()` check after the call reports no leaked goroutine for that subscriber's writer.
- A full/slow subscriber channel does not block the hub's broadcast to other subscribers.
  - *Given* two attached subscribers `A` (fast) and `B` (a `MemoryTransport` whose `Send` blocks forever, simulating a stalled writer goroutine), *When* the hub broadcasts 10 output frames within 100ms, *Then* `A` receives all 10 frames within 50ms of broadcast (measured), regardless of `B`'s state, and `B` is evicted (its channel closed, `Subscriber` removed) once it exceeds the bounded-queue-full threshold, not before.
- A `Transport.Send` error is treated identically to the slow-subscriber-eviction path — logged, the subscriber evicted (channel closed, removed from the registry), and the hub continues broadcasting to every other subscriber unaffected. `Transport.Send` returning an error is never hub-fatal; it is just another way a subscriber becomes unable to receive.
  - *Given* a subscriber `C` backed by a `MemoryTransport` configured with `WithErrorSend()` (its `Send` always returns a non-nil error), *When* the hub broadcasts a frame and `C`'s writer goroutine's call to `transport.Send` returns that error, *Then* `C` is detached exactly once (logged once, not once per attempted frame), `hub.SubscriberCount()` no longer includes `C`, and any other attached subscriber still receives the broadcast frame normally.
**Files**: `session/streamhub/hub.go`, `session/streamhub/subscriber.go`, `session/streamhub/subscriber_test.go`

##### Task 1.2.1a: Define `Subscriber` struct (channel, capability, id, last vote) (~4 min)
- Files: `session/streamhub/subscriber.go`

##### Task 1.2.1b: Implement `AttachSubscriber` (registry insert + writer goroutine spawn) (~5 min)
- Files: `session/streamhub/hub.go`

##### Task 1.2.1c: Implement `DetachSubscriber` (registry remove + channel close + goroutine join) (~5 min)
- Files: `session/streamhub/hub.go`

##### Task 1.2.1d: Implement per-subscriber writer goroutine with bounded channel + non-blocking send from hub (~5 min)
- Hub broadcast does `select { case sub.outbound <- frame: default: sub.markSlow() }`; writer goroutine drains `outbound` and calls `transport.Send`.
- Files: `session/streamhub/subscriber.go`

##### Task 1.2.1e: Implement slow-subscriber eviction policy (grace period then close+remove) (~5 min)
- Mirrors `controlModeSlowSubscriberGrace` (`session/tmux/control_mode.go:51`).
- Files: `session/streamhub/subscriber.go`

##### Task 1.2.1f: Write attach/detach/slow-subscriber-eviction unit tests with `goleak` (~5 min)
- Files: `session/streamhub/subscriber_test.go`

##### Task 1.2.1g: Implement `Transport.Send` error handling — a non-nil return detaches the subscriber via the same eviction path as Task 1.2.1e, logged once (~4 min)
- Files: `session/streamhub/subscriber.go`

#### Story 1.2.2: `StreamOwnershipLock` and hub teardown (last-detach grace period)
**As a** `StreamHub`, **I want** to be the sole holder of tmux session ownership and to tear down cleanly, **so that** a reconnect blip doesn't kill control-mode and a flag rollback doesn't orphan a subprocess.
**Acceptance Criteria**:
- Last subscriber detaching schedules teardown after a grace period, not immediately.
  - *Given* a `StreamHub` with exactly one `Subscriber` and `HubTeardownGrace = 5s`, *When* that subscriber is detached, *Then* the hub enters `HubDraining` and only transitions to `HubTornDown` (calling `SessionController.StopControlMode()`) if no new subscriber attaches within 5s.
- A subscriber attaching during the grace period cancels the pending teardown.
  - *Given* a `StreamHub` in `HubDraining` with 2s remaining on its grace timer, *When* `AttachSubscriber` is called, *Then* the hub transitions back to `HubActive` and the pending `SessionController.StopControlMode()` call never fires.
- Teardown is reachable from both "last subscriber detached" and "flag flipped back while active" via the same code path.
  - *Given* a `StreamHub` in `HubActive` with 2 subscribers and the `STAPLER_SQUAD_USE_STREAM_HUB` flag flipped back to `false` mid-session (simulated by calling `hub.ForceTeardown()` directly, the same internal method the grace-period path calls), *When* `ForceTeardown()` runs, *Then* `SessionController.StopControlMode()` is invoked exactly once and `hub.State()` returns `HubTornDown` — the same single teardown path Story 3.1.2 wires the flag-flip trigger to.
**Files**: `session/streamhub/hub.go`, `session/streamhub/lifecycle_test.go`

##### Task 1.2.2a: Implement `HubDraining` grace-period timer + cancel-on-reattach (~5 min)
- Files: `session/streamhub/hub.go`

##### Task 1.2.2b: Implement single `ForceTeardown()` method calling `SessionController.StopControlMode()` (~4 min)
- Files: `session/streamhub/hub.go`

##### Task 1.2.2c: Wire grace-period expiry to call `ForceTeardown()` (single code path, no duplicate teardown logic) (~4 min)
- Files: `session/streamhub/hub.go`

##### Task 1.2.2d: Write teardown/grace-period/reattach-cancels-teardown tests (~5 min)
- Files: `session/streamhub/lifecycle_test.go`

---

### Epic 1.3: Resize negotiation & owned resize/quiescence/capture pipeline
**Goal**: Exactly one `StreamHub` goroutine performs resize + quiescence-wait + capture-pane per resize event, using a smallest-common-size negotiation over capability-gated votes (ADR-002).

#### Story 1.3.1: `ResizeVote` collection and smallest-common-size `NegotiatedSize`
**As a** `StreamHub`, **I want** to reduce N subscriber resize requests to one target size, **so that** no subscriber unilaterally owns the pane's dimensions.
**Acceptance Criteria**:
- `RequestResize(id SubscriberID, size TerminalSize)` records a vote only if that subscriber's capability allows it.
  - *Given* a `Subscriber` with `SubscriberCapability{CanResize: false}` (e.g. a future read-only sink) and `size, _ := NewTerminalSize(200, 50)`, *When* `RequestResize(id, size)` is called for that subscriber, *Then* the hub's `NegotiatedSize` is unchanged and the call is logged as a rejected vote, not applied.
- `NewTerminalSize` rejects non-positive dimensions, so an invalid size can never reach `RequestResize` in the first place.
  - *Given* `NewTerminalSize(0, 24)` and `NewTerminalSize(80, -1)`, *When* each is called, *Then* both return a non-nil error and no `TerminalSize` value, so no call site can construct a `RequestResize` argument from a zero or negative dimension.
- `NegotiatedSize` is the component-wise minimum across all live `CanResize` votes.
  - *Given* two `CanResize` subscribers with votes `TerminalSize{Cols: 120, Rows: 40}` and `TerminalSize{Cols: 80, Rows: 24}`, *When* negotiation runs, *Then* `NegotiatedSize` is `TerminalSize{Cols: 80, Rows: 24}`.
- A never-resized subscriber defaults its vote to the current `NegotiatedSize`, not a hardcoded size.
  - *Given* a hub whose current `NegotiatedSize` is `TerminalSize{Cols: 80, Rows: 24}` and a new `CanResize` subscriber attaches without ever calling `RequestResize`, *When* negotiation next runs (e.g. triggered by a different subscriber's vote), *Then* the new subscriber's implicit vote is treated as `TerminalSize{Cols: 80, Rows: 24}` (its attach-time snapshot of `NegotiatedSize`), not `{80, 24}` as a global constant that would coincidentally match here but is actually derived per-hub — verified by asserting the implicit vote updates if the hub's negotiated size changes before this subscriber ever votes.
**Files**: `session/streamhub/resize.go`, `session/streamhub/types.go`, `session/streamhub/resize_test.go`

##### Task 1.3.1a: Define `TerminalSize` value object + `NewTerminalSize(cols, rows int) (TerminalSize, error)` validating constructor (rejects non-positive dimensions) (~4 min)
- Files: `session/streamhub/types.go`

##### Task 1.3.1b: Define `ResizeVote` struct (`SubscriberID`, `TerminalSize`) + subscriber-side storage (~3 min)
- Files: `session/streamhub/resize.go`

##### Task 1.3.1c: Implement `RequestResize(id SubscriberID, size TerminalSize)` with capability gating (~4 min)
- Files: `session/streamhub/resize.go`

##### Task 1.3.1d: Implement component-wise-minimum negotiation function (pure, unit-testable) (~5 min)
- `func negotiateSize(votes []ResizeVote) NegotiatedSize` — `NegotiatedSize` is itself a `TerminalSize`.
- Files: `session/streamhub/resize.go`

##### Task 1.3.1e: Implement never-resized-subscriber default-to-current-negotiated-size behavior (~4 min)
- Files: `session/streamhub/resize.go`

##### Task 1.3.1f: Write negotiation unit tests (min-across-votes, capability gating, never-resized default, `NewTerminalSize` validation, GoTTY-bug regression) (~5 min)
- Files: `session/streamhub/resize_test.go`

#### Story 1.3.2: Single-owner resize→quiescence→capture pipeline
**As a** `StreamHub`, **I want** to be the only caller of the session's resize/capture surface, **so that** the corruption race from concurrent per-connection resize/capture is structurally impossible.
**Acceptance Criteria**:
- `session/streamhub` depends only on the local `SessionController` interface (`SetWindowSize`/`ResizePTY`/`CapturePaneContent`/`StopControlMode`/`Subscribe`-`UnsubscribeControlModeUpdates`), never the concrete `*session.Instance` type, and never imports package `session`.
  - *Given* `session/streamhub/hub.go` and `session/streamhub/session_controller.go`, *When* `go list -deps ./session/streamhub/...` is run, *Then* `github.com/tstapler/stapler-squad/session` does not appear in the dependency list (verifying no import cycle risk against Story 3.1.2's package `session` → `session/streamhub` dependency).
- Only the hub goroutine calls `SessionController.SetWindowSize`/`ResizePTY`; no other code path may call these for a `PathHubOwned` session.
  - *Given* a `StreamHub` for tmux session `"claude-session-42"` in `PathHubOwned` mode wired to a fake `SessionController`, *When* `NegotiatedSize` changes from `{80,24}` to `{100,30}`, *Then* exactly one call to `controller.SetWindowSize(100, 30)` occurs (asserted via the call-counting fake), regardless of how many subscribers voted for the change.
- Broadcast during a reflow is suppressed until quiescence, mirroring existing `resizeSettling` behavior.
  - *Given* a resize is in progress (hub state `resizing=true`, quiescence not yet reached), *When* raw tmux output arrives, *Then* it is buffered but not broadcast to subscribers until `QuiescenceReached` fires.
- On `QuiescenceReached`, the hub captures the pane exactly once and broadcasts the resulting `CatchUpSnapshot` to **all** attached subscribers, not just the one that triggered the resize.
  - *Given* 3 attached subscribers and a resize triggered by subscriber `B`'s vote, *When* quiescence is reached after the resize, *Then* all 3 subscribers (including `A` and `C`, who did not request the resize) receive the same post-resize capture-pane snapshot, and exactly one `CapturePaneContent` call was made (asserted via a call-counting fake).
- `QuiescenceTimedOut` (500ms, matching existing `waitForQuiescence`) logs a hub-scoped WARN, same as today's connection-scoped one.
  - *Given* a resize where quiescence is never reached within 500ms, *When* the timeout fires, *Then* a `slog.Warn` is logged with the hub's `tmux_session` field (not a per-connection field, since there is no longer a "the connection" that triggered it in a multi-subscriber world).
- A `SessionController.SetWindowSize`/`ResizePTY`/`CapturePaneContent` error while the hub is otherwise alive is treated the same as `HubCrashed`/`TmuxProcessDied` (`research/architecture.md` §8's EventStorming table, also exercised by Story 1.4.2's hub-death test): the hub broadcasts a stream-ended sentinel to all attached subscribers and attempts the same restart-or-clean-teardown handling, rather than leaving pending `ResizeVote`s/`NegotiatedSize` in an undefined state or silently swallowing the error.
  - *Given* a `StreamHub` with 2 attached subscribers and a fake `SessionController` configured so `SetWindowSize` returns an error on the next call, *When* the hub attempts to apply a negotiated resize and that call errors, *Then* both subscribers receive the same stream-ended sentinel frame used by the `TmuxProcessDied` path, the hub logs the error with its `tmux_session` field, and the hub attempts a restart (or clean teardown if restart is not possible) via the same code path Story 1.4.2's hub-crash test exercises — no pending `ResizeVote` or stale `NegotiatedSize` is left referencing a controller that can no longer act on it.
**Files**: `session/streamhub/hub.go`, `session/streamhub/session_controller.go`, `session/streamhub/quiescence.go`, `session/streamhub/hub_test.go`

##### Task 1.3.2a: Define `SessionController` interface in `session/streamhub/session_controller.go`, scoped to exactly `SetWindowSize(cols, rows int) error`, `ResizePTY(cols, rows int) error`, `CapturePaneContent() (string, error)`, `StopControlMode() error`, `SubscribeControlModeUpdates() (string, <-chan []byte)`, `UnsubscribeControlModeUpdates(id string)` — the `*session.Instance` methods at `session/instance_tmux.go:587,600,722,727,733,738,752` (~5 min)
- `session/streamhub` never imports package `session`; `*session.Instance` satisfies `SessionController` structurally. This is the cycle-avoidance mechanism for Story 3.1.2's package-boundary decision (see Pattern Decisions table).
- Files: `session/streamhub/session_controller.go`

##### Task 1.3.2b: Extract `waitForQuiescence`'s logic into a hub-owned, session-agnostic function (~5 min)
- Adapt from `server/services/connectrpc_websocket.go:271-292` (`waitForQuiescence`) into `session/streamhub/quiescence.go`, taking a `SessionController`-scoped update channel instead of a per-connection one.
- Files: `session/streamhub/quiescence.go`

##### Task 1.3.2c: Implement hub's owned resize call (single call site for `SessionController.SetWindowSize`) (~4 min)
- Files: `session/streamhub/hub.go`

##### Task 1.3.2d: Implement reflow broadcast-suppression (`resizeSettling`-equivalent) (~5 min)
- Files: `session/streamhub/hub.go`

##### Task 1.3.2e: Implement post-quiescence single capture + all-subscriber broadcast (~5 min)
- Files: `session/streamhub/hub.go`

##### Task 1.3.2f: Implement `QuiescenceTimedOut` hub-scoped WARN (~3 min)
- Files: `session/streamhub/quiescence.go`

##### Task 1.3.2g: Implement `SessionController` call-error handling (SetWindowSize/ResizePTY/CapturePaneContent error → stream-ended sentinel broadcast + restart-or-teardown, same path as `HubCrashed`) (~5 min)
- Files: `session/streamhub/hub.go`

##### Task 1.3.2h: Write single-owner-call-count + all-subscriber-broadcast + reflow-suppression + `SessionController`-error-handling tests (~5 min)
- Files: `session/streamhub/hub_test.go`

---

### Epic 1.4: In-memory transport & failure-mode test suite
**Goal**: A `MemoryTransport` exists so the hub's core logic is fully unit-tested without a real tmux process or WebSocket — the required Testability NFR — and the specific failure modes named in `research/pitfalls.md` are exercised before any production wiring.

#### Story 1.4.1: `MemoryTransport` implementation
**As a** test author, **I want** an in-process `Transport` implementation, **so that** hub tests never need a real tmux process or network socket.
**Acceptance Criteria**:
- `MemoryTransport` implements `Transport` and exposes received frames for assertions.
  - *Given* `mt := NewMemoryTransport()` attached to a hub via `hub.AttachSubscriber(mt, cap)`, *When* the hub broadcasts a frame `[]byte("output-1")`, *Then* `mt.ReceivedFrames()` returns `[][]byte{[]byte("output-1")}`.
- `MemoryTransport.Send` can be configured to block or error, to simulate a slow/broken subscriber for Story 1.2.1's tests.
  - *Given* `mt := NewMemoryTransport(WithBlockingSend())`, *When* the hub attempts to send, *Then* `mt.Send` blocks until `mt.Unblock()` is called, letting tests deterministically exercise the slow-subscriber eviction path.
**Files**: `session/streamhub/memory_transport.go`, `session/streamhub/memory_transport_test.go`

##### Task 1.4.1a: Implement `MemoryTransport` with `ReceivedFrames()` accessor (~4 min)
- Files: `session/streamhub/memory_transport.go`

##### Task 1.4.1b: Implement `WithBlockingSend`/`WithErrorSend` configurable behaviors (~4 min)
- Files: `session/streamhub/memory_transport.go`

##### Task 1.4.1c: Write `MemoryTransport` self-tests (~3 min)
- Files: `session/streamhub/memory_transport_test.go`

#### Story 1.4.2: Failure-mode test suite (flag-flip, hub crash, backpressure, goroutine leak)
**As a** maintainer with no staging environment, **I want** the four failure modes named in `research/pitfalls.md`/`research/architecture.md` §6 covered by tests using `MemoryTransport`, **so that** these are rehearsed before ever touching a real session.
**Acceptance Criteria**:
- Hub death with attached subscribers broadcasts a stream-ended signal to all of them and attempts restart if subscribers remain (mirrors `controlModeExited`).
  - *Given* a `StreamHub` with 2 `MemoryTransport` subscribers and a fake `SessionController` whose control-mode subprocess is simulated to exit unexpectedly, *When* the exit is detected, *Then* both `MemoryTransport`s receive a stream-ended sentinel frame and the hub logs a restart attempt.
- A `SessionController.SetWindowSize`/`ResizePTY`/`CapturePaneContent` error while the hub is otherwise alive follows the same stream-ended-sentinel + restart-or-teardown path as hub death (Story 1.3.2's AC), not left unhandled.
  - *Given* a `StreamHub` with 2 `MemoryTransport` subscribers and a fake `SessionController` configured so `CapturePaneContent` returns an error on the next call, *When* the hub reaches the post-quiescence capture step and that call errors, *Then* both `MemoryTransport`s receive the same stream-ended sentinel frame as the hub-death case above, and the hub logs a restart attempt with its `tmux_session` field.
- Flag-flip mid-session never produces two owners for one tmux session (this is the structural regression test for the whole project's success metric).
  - *Given* tmux session `"race-test-1"` with `PathHubOwned` already resolved and an active `StreamHub`, *When* a second, concurrent call path attempts to resolve `PathLegacyPerConnection` for the same session name (simulating a flag re-read racing hub creation), *Then* `StreamOwnershipLock.Resolve("race-test-1", ...)` returns the **already-resolved** `PathHubOwned` value to the second caller instead of allowing a second owner, and `OverlapInvariant` is never violated — the test calls `t.Fatal` if it fires (asserted via `-race` and a repeated-trials loop, e.g. 1000 iterations, to catch a rare interleaving).
- Slow-subscriber backpressure evicts without stalling fast subscribers (extends Story 1.2.1's AC with a sustained, not one-shot, backpressure scenario).
  - *Given* one `MemoryTransport` configured with `WithBlockingSend()` never unblocked, and one normal `MemoryTransport`, *When* 100 output frames are broadcast over 1 second, *Then* the normal transport receives all 100 within expected latency and the blocked one is evicted before the 100th frame, with `streamhub_slow_subscriber_drops_total` incremented exactly once for the eviction (not once per dropped frame).
- `Transport.Send` returning an error evicts the subscriber the same way a blocked/slow one does (Story 1.2.1's AC), exercised here with `WithErrorSend` rather than `WithBlockingSend`.
  - *Given* one `MemoryTransport` configured with `WithErrorSend()` and one normal `MemoryTransport`, *When* the hub broadcasts a frame, *Then* the `WithErrorSend()` subscriber is detached after exactly one logged eviction and the normal subscriber still receives the frame.
- No goroutine leak across 1000 attach/detach cycles.
  - *Given* a `StreamHub` in `HubActive` state, *When* 1000 `AttachSubscriber`/`DetachSubscriber` cycles run with `MemoryTransport`, *Then* `goleak.VerifyNone(t)` reports zero leaked goroutines afterward.
- The 1000-iteration flag-flip race test and the 1000-cycle goroutine-leak test are pure in-memory (`MemoryTransport`, fake `SessionController`) and run in the fast unit-test path by default; if their combined wall-clock time becomes significant, they must guard with `if testing.Short() { t.Skip(...) }` (this repo's existing convention for slow tests, e.g. `session/tmux/tmux_test.go:336,470,501`) so `make quick-check`/CI can opt into a shorter run without deleting the coverage.
  - *Given* `go test ./session/streamhub/... -short`, *When* the 1000-iteration and 1000-cycle tests are guarded with `testing.Short()`, *Then* they are skipped under `-short` and still run under the default (non-`-short`) `make test`/`make ci` invocation.
**Files**: `session/streamhub/failure_modes_test.go`

##### Task 1.4.2a: Write hub-crash-with-subscribers test (~5 min)
- Files: `session/streamhub/failure_modes_test.go`

##### Task 1.4.2b: Write flag-flip-mid-session race test (1000-iteration loop, `-race`) (~5 min)
- Files: `session/streamhub/failure_modes_test.go`

##### Task 1.4.2c: Write sustained-backpressure eviction test with drop-metric assertion (~5 min)
- Files: `session/streamhub/failure_modes_test.go`

##### Task 1.4.2d: Write 1000-cycle attach/detach `goleak` test (~4 min)
- Files: `session/streamhub/failure_modes_test.go`

##### Task 1.4.2f: Write `SessionController` call-error (`CapturePaneContent` failure) → stream-ended-sentinel + restart-attempt test (~5 min)
- Files: `session/streamhub/failure_modes_test.go`

##### Task 1.4.2g: Write `Transport.Send`-error (`WithErrorSend`) → single-eviction test, consuming Task 1.4.1b's mode (~4 min)
- Files: `session/streamhub/failure_modes_test.go`

##### Task 1.4.2e: Run `go test ./session/streamhub/... -race -count=1` and confirm all Phase 1 tests green before Phase 2 starts (~3 min)
- Files: none (verification gate)

---

## Phase 2: Batching & Browser WebSocket transport

### Epic 2.1: Batch window & sequencing
**Goal**: Coalesce output across a session's subscribers via one hub-owned opportunistic-with-ceiling timer, without corrupting escape sequences (`research/pitfalls.md` §2a-2d) or delaying control/quiescence signals.

#### Story 2.1.1: Opportunistic-with-ceiling `BatchWindow` flush loop
**As a** `StreamHub`, **I want** one shared accumulation buffer and timer, **so that** N subscribers don't each pay independent flush-timer drift and so raw output batching never regresses today's latency feel.
**Acceptance Criteria**:
- Output events are concatenated as opaque byte ranges — never truncated mid-event, never content-sniffed.
  - *Given* two `OutputReceived` events `[]byte("\x1b[2J")` and `[]byte("hello")` arriving within the same window, *When* the window flushes, *Then* the broadcast frame is their exact concatenation `[]byte("\x1b[2Jhello")` — no byte reordering, no boundary inside either event's own bytes.
- The window flushes at the next subscriber-write opportunity or after `MaxBatchWindow` (20ms) since the first buffered byte, whichever is first.
  - *Given* an empty `BatchWindow` and one buffered event arriving at `t=0`, and no subscriber-write opportunity occurs before `t=20ms`, *When* `t=20ms` elapses, *Then* the window flushes automatically with a flush reason of `ceiling`, not `opportunistic`.
- Exactly one timer exists per hub, not per subscriber.
  - *Given* a `StreamHub` with 5 attached subscribers, *When* the hub is inspected via `runtime` goroutine dump or an internal counter, *Then* exactly one `BatchWindow` timer goroutine/ticker exists for that hub, not 5.
- Batching eliminates the *duplicated per-connection coalescing work* under concurrency, meeting requirements.md's corrected Success Metrics target (set here, concretely, as the metrics section previously deferred). **Corrected baseline**: today, per-connection opportunistic coalescing already exists (`server/services/connectrpc_websocket.go:800-821`, up to 32 immediately-available frames per write) — the real baseline is *not* one-frame-per-event, it's each of N subscribers independently running its own coalesce loop against the same underlying tmux output, i.e. N independent coalescing passes for one burst. The new hub-owned `BatchWindow` performs that accumulation/coalesce step **exactly once per flush, regardless of N**, and then fans the already-coalesced result out to N subscribers via N independent (cheap) delivery writes — this is an **Nx reduction in duplicated coalescing/capture computation**, not a claim that wire-message count drops, since each subscriber still needs its own delivery write per flush under both designs.
  - *Given* a `StreamHub` with 3 attached `MemoryTransport` subscribers (`CanResize: true`) and a simulated 50-event output burst delivered to the hub within one 20ms window, *When* the burst is broadcast, *Then* the hub's accumulation/coalesce function executes **exactly once** for the burst (asserted via a call-counting instrumented `BatchWindow`) — versus 3 independent invocations under today's per-connection design (one coalesce loop per subscriber for the same burst) — a 3x (Nx) reduction in duplicated coalescing work; each subscriber still receives exactly 1 delivery `Send` call for the flushed batch (3 total `Send` calls across the 3 `MemoryTransport`s, i.e. Nx1 delivery — not a wire-message-count reduction claim), and `streamhub_batch_flush_frames_coalesced` records the number of underlying events folded into that single hub-side coalesce (50 for this burst).
**Files**: `session/streamhub/batch.go`, `session/streamhub/batch_test.go`

##### Task 2.1.1a: Define `BatchWindow` struct (buffer, timer, `MaxBatchWindow` const = 20ms), adapting the buffer-pooling pattern from `coalesceBufPool` (`server/services/connectrpc_websocket.go:790-845`) to avoid per-flush allocation rather than designing a new pooling scheme (~4 min)
- Files: `session/streamhub/batch.go`

##### Task 2.1.1b: Implement opaque-concatenate accumulation (append-only byte buffer), routing any escape-sequence-sensitive byte handling through the existing `pkg/ansi` filtering rather than reimplementing it (~4 min)
- Files: `session/streamhub/batch.go`

##### Task 2.1.1c: Implement opportunistic-or-ceiling flush trigger logic (~5 min)
- Files: `session/streamhub/batch.go`

##### Task 2.1.1d: Write concatenation-integrity + single-timer + flush-reason tests (~5 min)
- Files: `session/streamhub/batch_test.go`

##### Task 2.1.1e: Write the N=3-subscribers/50-event-burst call-counting test asserting the hub's coalesce step runs exactly once per flush (Nx reduction in duplicated coalescing work vs. today's per-connection loop), citing `coalesceBufPool`/`pkg/ansi` (`server/services/connectrpc_websocket.go:790-845`) as the adapted source for the accumulation logic under test (~5 min)
- Files: `session/streamhub/batch_test.go`

#### Story 2.1.2: `HubSequenceNumber` + bypass path for control/quiescence messages
**As a** subscriber reconnecting or attached with a different flush cadence, **I want** a stable total order and immediate delivery of control signals, **so that** batching never delays the "am I resizing" UI state (`research/pitfalls.md` §2c) or reorders events across subscribers (§2d).
**Acceptance Criteria**:
- Every broadcast unit (batched frame or bypass message) is stamped with a monotonic `HubSequenceNumber` at broadcast time, not at per-subscriber flush time.
  - *Given* a hub that has broadcast 3 prior units, *When* a 4th unit (batched output frame) is broadcast, *Then* it carries `HubSequenceNumber = 4`, and this holds regardless of which subscriber's flush cadence delivers it first.
- `ResizeQuiescence` signals and the authoritative post-resize `CatchUpSnapshot` bypass the `BatchWindow` entirely.
  - *Given* a `BatchWindow` currently mid-accumulation with 2 buffered output events not yet flushed, *When* a `QuiescenceReached` event occurs, *Then* the quiescence signal is delivered to all subscribers immediately (its own `HubSequenceNumber`, sent before the pending batch's), and the pending batch's own flush happens on its normal schedule afterward — the quiescence signal is never held hostage inside the batch buffer.
**Files**: `session/streamhub/batch.go`, `session/streamhub/sequence.go`, `session/streamhub/batch_test.go`

##### Task 2.1.2a: Implement `HubSequenceNumber` monotonic counter, stamped at broadcast time (~4 min)
- Files: `session/streamhub/sequence.go`

##### Task 2.1.2b: Implement bypass path for `ResizeQuiescence`/`CatchUpSnapshot` message types (~5 min)
- Files: `session/streamhub/batch.go`

##### Task 2.1.2c: Write sequence-ordering-across-subscribers + bypass-not-blocked-by-pending-batch tests (~5 min)
- Files: `session/streamhub/batch_test.go`

---

### Epic 2.2: Browser WebSocket transport cutover
**Goal**: A real `WebSocketTransport` wraps the existing `connectWebSocketStream`/`gorilla/websocket` machinery and becomes the browser path's `Transport`, wired into `streamViaControlMode`'s call site behind the (still-off-by-default) flag.

#### Story 2.2.1: `WebSocketTransport` adapter
**As a** browser connection, **I want** to attach to a `StreamHub` through the `Transport` interface, **so that** the hub's core logic never has to know about `gorilla/websocket`.
**Acceptance Criteria**:
- `WebSocketTransport` wraps `*connectWebSocketStream` and implements `Transport`.
  - *Given* an established `*connectWebSocketStream` for a live browser connection, *When* `NewWebSocketTransport(stream)` is called and then `.Send(envelopeBytes)` is invoked, *Then* the bytes are written via the stream's existing mutex-guarded `WriteMessage` (`server/services/connectrpc_websocket.go:536-540`), reusing that concurrency-safety fix rather than duplicating it.
- `Close()` on the transport cleanly triggers `DetachSubscriber` on its hub without a second, independent teardown path.
  - *Given* a `WebSocketTransport` attached to a hub as subscriber `"sub-7"`, *When* the browser disconnects and the stream's read loop errors, *Then* `transport.Close()` is called exactly once and results in `hub.DetachSubscriber("sub-7")` being called exactly once (no double-detach, no leaked subscriber entry).
**Files**: `server/services/websocket_transport.go`, `server/services/websocket_transport_test.go`

##### Task 2.2.1a: Implement `WebSocketTransport` wrapping `*connectWebSocketStream` (~5 min)
- Files: `server/services/websocket_transport.go`

##### Task 2.2.1b: Implement `Close()` → single `DetachSubscriber` call, no duplicate teardown (~4 min)
- Files: `server/services/websocket_transport.go`

##### Task 2.2.1c: Write `WebSocketTransport` unit tests against a fake stream (~4 min)
- Files: `server/services/websocket_transport_test.go`

#### Story 2.2.2: Wire `streamViaControlMode`'s hub-owned branch behind the flag
**As an** operator dark-launching this feature, **I want** `streamViaControlMode` to route through the hub only when `PathHubOwned` is resolved for a session, **so that** the legacy path is completely unaffected when the flag is off (the current, default state).
**Acceptance Criteria**:
- When `StreamPath` resolves to `PathLegacyPerConnection` (default, flag off), behavior is byte-for-byte identical to today — this story adds a branch, it does not modify the existing branch's logic.
  - *Given* `STAPLER_SQUAD_USE_STREAM_HUB=false` (default) and a new WebSocket connection to session `"legacy-session"`, *When* `streamTerminal` resolves the stream path, *Then* it calls today's unmodified `streamViaControlMode` code path with no `StreamHub` ever created for that session.
- When `StreamPath` resolves to `PathHubOwned`, the connection attaches a `WebSocketTransport` to the session's `StreamHub` (via `HubRegistry`'s get-or-create) instead of running the old per-connection resize/capture logic.
  - *Given* `STAPLER_SQUAD_USE_STREAM_HUB=true` and a new WebSocket connection to session `"hub-session"` with no existing hub, *When* the connection is accepted, *Then* `HubRegistry.GetOrCreate("hub-session")` creates exactly one `StreamHub`, and `AttachSubscriber` is called with a `WebSocketTransport` wrapping this connection — no call to the legacy resize/capture code occurs for this connection.
- The `PathHubOwned` branch has its own verified redraw/latency parity with the legacy path, not just an inferred non-regression on the untouched legacy branch — see Task 2.2.2d's Playwright test.
  - *Given* Task 2.2.2d's e2e test (`tests/e2e/terminal-hub-path.spec.ts`, modeled on `terminal-resize.spec.ts`), *When* it runs against a `PathHubOwned` session, *Then* it passes using the same bounded-wait latency/redraw assertions this repo's existing terminal e2e suite already applies to the legacy path.
**Files**: `server/services/connectrpc_websocket.go`, `tests/e2e/terminal-hub-path.spec.ts`

##### Task 2.2.2a: Add `StreamPath`-branching at the top of `streamTerminal` (no change to the `PathLegacyPerConnection` branch's body) (~5 min)
- Files: `server/services/connectrpc_websocket.go`

##### Task 2.2.2b: Implement the `PathHubOwned` branch: `HubRegistry.GetOrCreate` + `AttachSubscriber(NewWebSocketTransport(stream), ...)` (~5 min)
- Files: `server/services/connectrpc_websocket.go`

##### Task 2.2.2c: Write an integration test exercising both branches against a fake `SessionController` (~5 min)
- Files: `server/services/connectrpc_websocket_test.go`

##### Task 2.2.2d: Write a Playwright e2e test exercising the `PathHubOwned` branch directly (flag on) for redraw/latency parity with the legacy path (~5 min)
- Model this spec on this repo's existing terminal e2e coverage (`tests/e2e/terminal-resize.spec.ts`, `tests/e2e/terminal-flickering.spec.ts`) — same locator/assertion style (`data-testid`/ARIA roles, no `waitForTimeout`, per `.claude/rules/e2e-test-conventions.md`) — but run with `STAPLER_SQUAD_USE_STREAM_HUB=true` (or the per-session override, Story 3.3.1) against a session forced onto `PathHubOwned`, asserting the same redraw-completion/latency assertions those specs already make for the legacy path (e.g. output appears within the same bounded wait, resize settles to one final size without intermediate flicker frames).
- **Acceptance Criterion**: *Given* a session with `PathHubOwned` forced via the per-session override, *When* the existing terminal e2e assertions for output-latency and resize-redraw correctness (modeled on `terminal-resize.spec.ts`) are run against it, *Then* they pass with the same bounded-wait thresholds used for the legacy-path (`PathLegacyPerConnection`) specs — establishing that the new path has its own verified parity, not just an inferred non-regression on the untouched legacy branch.
- Files: `tests/e2e/terminal-hub-path.spec.ts` (new)

---

## Phase 3: Dark-launch flag, ownership lock, registry consolidation, observability

### Epic 3.1: `StreamPath` resolution & `StreamOwnershipLock`
**Goal**: Resolve `StreamPath` exactly once per tmux session, sticky, via a shared lock that both the legacy path and hub creation must acquire — closing the two-owner flip-window race (`research/pitfalls.md` §3a).

#### Story 3.1.1: Sticky per-session `StreamPath` resolution
**As the** system resolving how a new connection should stream a session, **I want** the resolution to happen once per session and stick, **so that** a flag change mid-rollout never splits one session across two owners.
**Acceptance Criteria**:
- The first connection to a session resolves `StreamPath` (reading `STAPLER_SQUAD_USE_STREAM_HUB` and any per-session override); every subsequent connection to the same session reuses that resolution without re-reading the flag.
  - *Given* tmux session `"sticky-test"` with no prior resolution and `STAPLER_SQUAD_USE_STREAM_HUB=true` at the moment the first connection arrives, *When* a second connection arrives 10 seconds later after the flag has been flipped to `false` in the environment, *Then* the second connection still resolves to `PathHubOwned` (the session's sticky value), not `PathLegacyPerConnection`.
- Resolution is per-tmux-session-name, stored alongside (or referenced from) the existing session state, not a global value.
  - *Given* two different tmux sessions `"session-a"` (resolved `PathHubOwned`) and `"session-b"` (resolved `PathLegacyPerConnection`, e.g. because `session-b`'s hub was already torn down and legacy re-resolved it after the flag flipped), *When* both are queried, *Then* each returns its own independently-resolved `StreamPath`.
**Files**: `session/streamhub/ownership.go`, `session/streamhub/ownership_test.go`

##### Task 3.1.1a: Define `StreamOwnershipLock` type (per-tmux-session mutex + resolved `StreamPath` cache), owned by package `session/streamhub`, plus a package-level `xsync.Map[string, *StreamOwnershipLock]` and `func AcquireOwnershipLock(sessionName string) *StreamOwnershipLock` get-or-create accessor — the single, concrete sharing mechanism both the legacy path (package `session`, importing `session/streamhub`) and hub creation (already inside `session/streamhub`) use to look up the one lock instance per tmux session name (~5 min)
- `session/streamhub` still never imports package `session` for this — `StreamOwnershipLock` has no dependency on `SessionController`/`Instance`, only on the session name string.
- Files: `session/streamhub/ownership.go`

##### Task 3.1.1b: Implement `Resolve(sessionName string, flagValue bool) StreamPath` — resolve-once-and-cache (~5 min)
- Files: `session/streamhub/ownership.go`

##### Task 3.1.1c: Write sticky-resolution-survives-later-flag-flip test (~4 min)
- Files: `session/streamhub/ownership_test.go`

#### Story 3.1.2: Shared ownership lock enforcement (legacy path and hub creation both acquire it)
**As the** system, **I want** legacy `StartControlMode` calls and hub creation to share one ownership primitive, **so that** two different ownership models can never both claim one live tmux session.
**Package boundary (concrete, not "either/or")**: `StreamOwnershipLock` is owned by package `session/streamhub` (Task 3.1.1a's `AcquireOwnershipLock(sessionName string) *StreamOwnershipLock`, backed by a package-level `xsync.Map[string, *StreamOwnershipLock]`). `session/instance_tmux.go`'s `Instance.StartControlMode` imports `session/streamhub` and calls `streamhub.AcquireOwnershipLock(i.GetTmuxSessionName())` before acquiring it — a one-way dependency (`session` → `session/streamhub`) that does not cycle, because `session/streamhub` depends only on the `SessionController` interface (Task 1.3.2a) and never imports package `session`. This is the single, concrete mechanism both call sites use — not a choice left to the implementer.
**Acceptance Criteria**:
- `HubRegistry.GetOrCreate` and `Instance.StartControlMode` both call `streamhub.AcquireOwnershipLock(sessionName)` and acquire the same `*StreamOwnershipLock` instance for a given session name, so hub creation and legacy start are mutually exclusive for one session.
  - *Given* tmux session `"exclusive-test"` with its `StreamOwnershipLock` (from `streamhub.AcquireOwnershipLock("exclusive-test")`) currently held by a legacy `StartControlMode` call in progress, *When* `HubRegistry.GetOrCreate("exclusive-test")` is called concurrently, *Then* it blocks until the legacy call releases the lock, then observes the already-resolved `PathLegacyPerConnection` and joins that path (attaches as a legacy-style connection) rather than creating a competing hub.
- This is verified under `-race` with concurrent goroutines racing both entry points.
  - *Given* 100 goroutines simultaneously calling either `Instance.StartControlMode()` or `HubRegistry.GetOrCreate()` for the same session name, *When* run under `go test -race`, *Then* no data race is reported and exactly one control-mode subprocess/hub combination results (never two independent owners).
- `go list -deps ./session/streamhub/...` never lists package `session`, confirming the one-way dependency holds after this story wires `session`'s call site.
  - *Given* the completed Story 3.1.2 diff, *When* `go list -deps ./session/streamhub/...` and `go build ./...` are run, *Then* the dependency list excludes `github.com/tstapler/stapler-squad/session` and the build succeeds (the concrete, executable confirmation that no import cycle was introduced).
**Files**: `session/instance_tmux.go`, `session/streamhub/ownership.go`, `session/streamhub/ownership_test.go`

##### Task 3.1.2a: Wire `Instance.StartControlMode` (`session/instance_tmux.go`) to call `streamhub.AcquireOwnershipLock(i.GetTmuxSessionName())` and acquire it before starting control mode, importing `session/streamhub` from package `session` (one-way; `session/streamhub` does not import back) (~5 min)
- Files: `session/instance_tmux.go`, `session/streamhub/ownership.go`

##### Task 3.1.2b: Implement "legacy call observes hub already resolved → joins legacy path anyway" fallback behavior explicitly (no silent success reinterpreted) (~5 min)
- Files: `session/streamhub/ownership.go`

##### Task 3.1.2c: Write the 100-goroutine concurrent-race test under `-race`, plus a `go list -deps` check confirming no import of package `session` from `session/streamhub` (~5 min)
- Files: `session/streamhub/ownership_test.go`

---

### Epic 3.2: Registry consolidation & observability
**Goal**: Make the hub's subscriber registry the single source of truth for "how many connections are on this session," per requirements.md's Observability Requirements, while keeping the `420584566` WARN alive as the legacy-path safety net.

#### Story 3.2.1: Consolidate connection-count sources; keep the `420584566` WARN live and unmodified
**As an** operator debugging a live session, **I want** one place to check "how many connections are attached," **so that** I'm not cross-referencing three disjoint counters during the dark-launch window.
**Acceptance Criteria**:
- For `PathHubOwned` sessions, `hub.SubscriberCount()` is the sole source of truth; `activeControlModeStreams` is not incremented for hub-owned sessions (it remains legacy-path-only).
  - *Given* a `PathHubOwned` session with 2 attached `WebSocketTransport` subscribers, *When* an operator queries connection count, *Then* the value comes from `hub.SubscriberCount()` (returns `2`), and `activeControlModeStreams.Load(sessionName)` for that session returns `loaded=false` (never touched by the hub path).
- `recordControlModeStreamStart`/`activeControlModeStreams`/the `420584566` WARN are **not modified** by this project and continue to run exactly as today for `PathLegacyPerConnection` sessions.
  - *Given* a `PathLegacyPerConnection` session with two overlapping `streamViaControlMode` invocations (the pre-existing race, unchanged), *When* the second invocation starts before the first's `done()` runs, *Then* the existing WARN log line fires exactly as it does today — a diff of `recordControlModeStreamStart`'s function body against its current form (`connectrpc_websocket.go:248-267`) shows zero changes.
- `TmuxSession.controlModeSubscribers` and `ExternalStreamer.consumers` are demoted to internal, single-entry-per-hub implementation details for `PathHubOwned` sessions (the hub itself is now the one subscriber registered against each), not deleted.
  - *Given* a `PathHubOwned` session, *When* the hub subscribes to the underlying `TmuxSession`'s control-mode output, *Then* `TmuxSession.controlModeSubscribers` has exactly one entry (the hub's own subscription), regardless of how many `Subscriber`s are attached to the hub itself.
**Files**: `session/streamhub/hub.go`, `server/services/connectrpc_websocket.go` (no changes inside `recordControlModeStreamStart` itself — verification only)

##### Task 3.2.1a: Ensure hub-path connections never call `recordControlModeStreamStart`/touch `activeControlModeStreams` (~4 min)
- Files: `server/services/connectrpc_websocket.go`

##### Task 3.2.1b: Diff `recordControlModeStreamStart` against its pre-project form to confirm zero changes (~3 min)
- Files: none (verification task)

##### Task 3.2.1c: Implement hub's single subscription to `TmuxSession.SubscribeToControlModeUpdates` (one call regardless of hub's own subscriber count) (~5 min)
- Files: `session/streamhub/hub.go`

##### Task 3.2.1d: Write a test asserting `controlModeSubscribers` has exactly 1 entry for N hub-attached subscribers (~4 min)
- Files: `session/streamhub/hub_test.go`

#### Story 3.2.2: Structured logging & metrics for hub lifecycle events
**As an** operator, **I want** the observability plan's log lines and metrics actually emitted, **so that** I can reconstruct "what did the hub do and who was attached" after the fact — the same bar `420584566`'s WARN was trying to meet.
**Acceptance Criteria**:
- Hub creation/teardown, subscriber attach/detach, resize-negotiation decisions, and batch-flush events each emit one structured `slog` line with the fields listed in the Observability Plan.
  - *Given* a `StreamHub` created for session `"observed-session"`, *When* a subscriber attaches, requests a resize, and later detaches, *Then* the log stream contains, in order, a `hub_created`, a `subscriber_attached`, a `resize_negotiated`, and a `subscriber_detached` line, each tagged `tmux_session="observed-session"`.
- The four new metrics (`streamhub_active_hubs`, `streamhub_subscribers_per_hub`, `streamhub_resize_negotiations_total`, `streamhub_batch_flush_frames_coalesced`, `streamhub_slow_subscriber_drops_total`, `streamhub_overlap_invariant_violations_total`) are registered and incrementing.
  - *Given* the metrics registry used elsewhere in `server/services` (already wired for other RPCs via `connectrpc.com/otelconnect`), *When* a hub is created and one subscriber attaches, *Then* `streamhub_active_hubs` reads `1` and `streamhub_subscribers_per_hub`'s histogram has one observation of `1`.
**Files**: `session/streamhub/observability.go`, `session/streamhub/observability_test.go`

##### Task 3.2.2a: Add `slog` calls at hub-creation/teardown, attach/detach, resize-negotiation, batch-flush (~5 min)
- Files: `session/streamhub/hub.go`, `session/streamhub/batch.go`, `session/streamhub/resize.go`

##### Task 3.2.2b: Register the 6 new metrics with this repo's existing metrics registry (~5 min)
- Files: `session/streamhub/observability.go`

##### Task 3.2.2c: Write tests asserting log-line presence/ordering and metric increments (~5 min)
- Files: `session/streamhub/observability_test.go`

---

### Epic 3.3: Rollout mechanics & rollback rehearsal
**Goal**: Support a per-session canary on top of the global flag, and exercise the rollback path once for real before any wider rollout.

#### Story 3.3.1: Per-session override + global default flag wiring
**As an** operator, **I want** to force `PathHubOwned` for one disposable session without flipping the global default, **so that** I can canary the feature without touching every live session.
**Acceptance Criteria**:
- A per-session override (e.g. a config field or admin-only control-plane call) forces `PathHubOwned` for one named session regardless of the global `STAPLER_SQUAD_USE_STREAM_HUB` value.
  - *Given* `STAPLER_SQUAD_USE_STREAM_HUB=false` globally and a per-session override set for session `"canary-1"`, *When* the first connection to `"canary-1"` arrives, *Then* it resolves `PathHubOwned` for that session while a simultaneous first connection to a different session `"normal-1"` (no override) resolves `PathLegacyPerConnection`.
- Setting the *global default* to `true` is mechanically gated on a recorded rollback rehearsal (pre-mortem P1 #4) — the per-session override above is unaffected by this gate.
  - *Given* `config.RollbackRehearsalCompletedAt` is unset (zero value) in the config store, *When* startup (or a config-reload path) attempts to resolve the global `STAPLER_SQUAD_USE_STREAM_HUB` default to `true`, *Then* resolution fails with an explicit startup error naming the missing rehearsal (not a silent fallback to `false`), and the per-session override path for a named canary session is unaffected by this failure.
  - *Given* `config.RollbackRehearsalCompletedAt` is set to a valid, non-zero timestamp (written by Story 3.3.2's Task 3.3.2c), *When* the same resolution runs, *Then* the global default is permitted to be `true`.
**Files**: `session/streamhub/ownership.go`, `config/config.go` (or equivalent existing session-scoped config location)

##### Task 3.3.1a: Add per-session override lookup to `StreamOwnershipLock.Resolve` (~5 min)
- Files: `session/streamhub/ownership.go`

##### Task 3.3.1b: Wire override storage into existing per-session config/state (~5 min)
- Files: `config/config.go` (exact location TBD by implementer against current config schema)

##### Task 3.3.1c: Write per-session-override-vs-global-default test (~4 min)
- Files: `session/streamhub/ownership_test.go`

##### Task 3.3.1d: Add `RollbackRehearsalCompletedAt *time.Time` to `config/config.go`'s config struct and a `ResolveGlobalStreamHubDefault(cfg *Config) (bool, error)` (or equivalent) check that returns an error instead of `true` when the field is unset and the raw env/config value requests `true` — wired into whatever reads `STAPLER_SQUAD_USE_STREAM_HUB` for the global default (not the per-session override path) (~5 min)
- Files: `config/config.go`

##### Task 3.3.1e: Write a test asserting global-default resolution fails fast with an explicit error when `RollbackRehearsalCompletedAt` is unset, and succeeds once it's set (~4 min)
- Files: `config/config_test.go`

#### Story 3.3.2: Exercised rollback rehearsal against a disposable session
**As an** operator with no staging environment, **I want** to have actually flipped the flag on and back off against a real (disposable) session before relying on the rollback path during a real incident, **so that** "flip the flag back" is proven, not just asserted.
**Acceptance Criteria**:
- A documented, executed rehearsal: create a disposable session, force `PathHubOwned` via the per-session override, use it briefly, then remove the override (or flip the global flag off) and confirm the session survives and reconnects cleanly under `PathLegacyPerConnection` — for a **new** connection only, since Story 3.1.1 guarantees existing ones don't move mid-flight.
  - *Given* a disposable session `"rollback-rehearsal-1"` running under `PathHubOwned` with one active `WebSocketTransport` subscriber, *When* the per-session override is removed and a **new** connection is made to the same session, *Then* that new connection resolves `PathLegacyPerConnection` for any *future* session with that name only after the current hub has fully torn down (per Story 1.2.2's `ForceTeardown`), and no data corruption or crash is observed in manual verification.
  - This AC is verified by manual execution against the operator's own instance (not just a unit test) — record the outcome (pass/fail, date) as a one-line note in this project's `implementation/validation.md` during Phase 4/validation, per `research/pitfalls.md` §5 point 4.
- On a **passing** rehearsal, `config.RollbackRehearsalCompletedAt` is persisted (per-mortem P1 #4's mechanical gate, Task 3.3.1d) — a passing manual rehearsal that never writes this field does not unblock the global default.
  - *Given* the rehearsal above completes successfully, *When* Task 3.3.2c runs, *Then* it writes the current timestamp to `config.RollbackRehearsalCompletedAt` in the config store (not just prose in `validation.md`), and a subsequent attempt to resolve the global `STAPLER_SQUAD_USE_STREAM_HUB` default to `true` (Task 3.3.1d's check) succeeds where it previously failed.
**Files**: none (operational task) — outcome recorded in `project_plans/terminal-multi-connection-streaming/implementation/validation.md` (created in the SDD validate phase); `config/config.go` (the persisted timestamp)

##### Task 3.3.2a: Create a disposable test session and force `PathHubOwned` via the per-session override (~3 min)
- Files: none (manual operational step)

##### Task 3.3.2b: Use the session briefly, then remove the override and verify clean teardown + legacy reconnect (~5 min)
- Files: none (manual operational step)

##### Task 3.3.2c: Record the rehearsal outcome in `implementation/validation.md` AND, on a pass, persist `config.RollbackRehearsalCompletedAt` (Task 3.3.1d's gate) so the global default can subsequently be set to `true` (~3 min)
- Files: `project_plans/terminal-multi-connection-streaming/implementation/validation.md`, `config/config.go`

#### Story 3.3.3: Real multi-session restart-then-simultaneous-reconnect storm test
**As a** maintainer with no staging environment, **I want** the actual failure scenario that motivated this project — `make install-service` (or a crash) force-disconnecting and reconnecting every open terminal at once — exercised against real tmux control-mode plumbing, not fakes, **so that** the project's structural fix is proven against the real trigger, not only against single-session/fake-backed race tests (pre-mortem P1 #1). Every existing ownership/race test in this plan (Story 1.4.2's 1000-iteration test, Story 3.1.2's 100-goroutine test) uses a single tmux session and `MemoryTransport`/a fake `SessionController` — none exercises N real sessions reconnecting simultaneously.
**Acceptance Criteria**:
- The test spins up multiple real tmux sessions using this repo's existing real-session test pattern — `tmux.NewTmuxSessionWithPrefixAndCleanup` (`session/tmux/tmux.go:784`), the same constructor `session/integration_test.go:823-830` already uses to run two real tmux sessions concurrently — rather than a fake `SessionController`, and skips with `t.Skip("tmux not available")` if `exec.LookPath("tmux")` fails, matching this repo's existing convention (e.g. `session/tmux/tmux_test.go:313`).
  - *Given* `exec.LookPath("tmux")` succeeds, *When* the test starts, *Then* it creates at least 5 real tmux sessions via `tmux.NewTmuxSessionWithPrefixAndCleanup`, each with `PathHubOwned` resolved and its own `StreamHub`/`WebSocketTransport`-equivalent subscriber (or `MemoryTransport` attached to the *real* `SessionController`-satisfying `*session.Instance`, per Task 1.3.2a's structural contract) already attached.
- The test simulates a restart-equivalent mass-disconnect/reconnect across all sessions concurrently, with the hub path active for every session.
  - *Given* the 5 real sessions above each with one attached subscriber, *When* all 5 subscribers are detached and immediately (concurrently, via `sync.WaitGroup`, no serialization) re-attached with a fresh subscriber to simulate every browser tab reconnecting after a `make install-service` restart, *Then* every session's hub completes its resize→quiescence→capture pipeline for the reconnecting subscriber without error.
- `streamhub_overlap_invariant_violations_total` stays at 0 and `OverlapInvariant` never fires across the whole storm.
  - *Given* the concurrent mass-reconnect above run under `go test -race`, *When* the storm completes, *Then* `streamhub_overlap_invariant_violations_total` reads `0` across all 5 sessions combined, and the test asserts zero `OverlapInvariant` `slog.Error` emissions occurred during the run (via `t.Fatal` on the first occurrence, per the Domain Glossary's `OverlapInvariant` entry — this test never expects or recovers a panic, since `OverlapInvariant` no longer panics in the running binary).
- This test is a required, explicit prerequisite gate — it must pass before the global default flag (`STAPLER_SQUAD_USE_STREAM_HUB`) is ever flipped to `true`, listed alongside the rollback rehearsal (Story 3.3.2) in the Risk Control section's rollout staging, not merely included in the general test suite.
  - *Given* the Risk Control section's staged-rollout list, *When* reviewed before flipping the global default, *Then* it names both "rollback rehearsal completed" (Story 3.3.2) and "real multi-session reconnect-storm test green" (this story) as required prerequisites, not just "tests pass."
- This test spins up 5 real tmux sessions and is expected to be slow relative to the pure-unit-test suite; it is marked so it can run in a longer-running CI job separate from the fast unit-test path, using this repo's existing `testing.Short()` skip-guard convention (`session/tmux/tmux_test.go:336,470,501`) rather than being folded silently into `make test`'s default budget.
  - *Given* `go test ./session/streamhub/... -short`, *When* this test's `testing.Short()` guard is in place, *Then* it is skipped under `-short` and runs only in the full (non-`-short`) suite, e.g. the CI job or local invocation that also exercises Story 3.3.2's rehearsal gate before a global-default flip.
**Files**: `session/streamhub/reconnect_storm_integration_test.go`

##### Task 3.3.3a: Write the multi-real-tmux-session setup helper (5 sessions via `tmux.NewTmuxSessionWithPrefixAndCleanup`, `PathHubOwned` forced via per-session override) (~5 min)
- Files: `session/streamhub/reconnect_storm_integration_test.go`

##### Task 3.3.3b: Implement the concurrent mass-detach/re-attach storm simulation (`sync.WaitGroup`, all 5 sessions simultaneously) (~5 min)
- Files: `session/streamhub/reconnect_storm_integration_test.go`

##### Task 3.3.3c: Assert `streamhub_overlap_invariant_violations_total == 0` and `OverlapInvariant` never fires (`t.Fatal` on first occurrence, no panic expected) across the storm, run under `-race` (~4 min)
- Files: `session/streamhub/reconnect_storm_integration_test.go`

##### Task 3.3.3d: Wire this test as an explicit prerequisite check (alongside the rollback rehearsal) before the global default flag can be set to `true` — see Task 3.3.1d in Risk Control (~2 min)
- Files: none (cross-reference to Task 3.3.1d's mechanical gate)

##### Task 3.3.3e: Add a `testing.Short()` skip guard so this slow, real-tmux test is excluded under `-short`, matching `session/tmux/tmux_test.go`'s existing convention (~3 min)
- Files: `session/streamhub/reconnect_storm_integration_test.go`

---

## Phase 4: ssq-mux output-only transport & UI connection indicator

### Epic 4.1: ssq-mux Transport adapter (output-only, resize unification deferred per ADR-004)
**Goal**: Formalize ssq-mux as a `Transport` implementation with zero changes to hub logic (the Success Metric's explicit bar), while deliberately not yet routing ssq-mux's own resize authority through the hub (ADR-004).

#### Story 4.1.1: `MuxTransport` wraps `ExternalStreamer` as a `Transport`
**As a** ssq-mux-attached IDE terminal, **I want** to appear to the hub as just another `Transport`, **so that** the hub's fan-out/registry logic needs zero special-casing for ssq-mux.
**Acceptance Criteria**:
- `MuxTransport` implements `Transport` by delegating `Send` to `ExternalStreamer`'s existing consumer-callback shape (`OutputConsumer`) and `Close` to `ExternalStreamer.RemoveConsumer`.
  - *Given* an existing `*ExternalStreamer` for socket path `/tmp/ssq-mux-1234.sock`, *When* `NewMuxTransport(streamer)` is created and attached to a `StreamHub` via `AttachSubscriber`, *Then* output broadcast by the hub reaches the ssq-mux client(s) through `ExternalStreamer`'s own existing broadcast to its registered consumers — no new byte-delivery code path is introduced in `session/mux/multiplexer.go`.
- Attaching a `MuxTransport` requires zero changes to `session/streamhub/hub.go`'s broadcast or registry logic (the explicit Success Metric).
  - *Given* the diff for this story, *When* reviewed, *Then* it touches only a new file (`session/external_streamer_transport.go`) plus its attach call site — `session/streamhub/hub.go` has zero lines changed.
**Files**: `session/external_streamer_transport.go`, `session/external_streamer_transport_test.go`

##### Task 4.1.1a: Implement `MuxTransport` wrapping `*ExternalStreamer` (~5 min)
- Files: `session/external_streamer_transport.go`

##### Task 4.1.1b: Implement `Close()` → `ExternalStreamer.RemoveConsumer` (~3 min)
- Files: `session/external_streamer_transport.go`

##### Task 4.1.1c: Write `MuxTransport` unit tests against a fake `ExternalStreamer` (~4 min)
- Files: `session/external_streamer_transport_test.go`

##### Task 4.1.1d: Confirm via `git diff` that `session/streamhub/hub.go` has zero changes from this story (~2 min)
- Files: none (verification task)

#### Story 4.1.2: Attach `MuxTransport` with `CanResize: false`
**As the** system, **I want** ssq-mux's attachment to the hub to be output-only, **so that** the hub's resize negotiation never contends with ssq-mux's own independent (and, per ADR-004, still-unmediated-for-now) resize path.
**Acceptance Criteria**:
- `MuxTransport` is always attached with `SubscriberCapability{CanResize: false, CanWrite: false}` for this pass.
  - *Given* a `PathHubOwned` session that also has an active ssq-mux attachment, *When* `MuxTransport` is attached, *Then* `hub.NegotiatedSize` is never influenced by it (verified: forcing `MuxTransport.RequestResize` — if called at all — has no effect, per Story 1.3.1's capability gating), and ssq-mux's own `Multiplexer.handleClient`/`SetWindowSize` code path is completely untouched by this project.
- This is documented as a named, accepted residual gap, not silently absent: the browser+ssq-mux combination on one session is not fully race-free after this pass (ssq-mux's own resize race is pre-existing and unchanged), while the browser+browser and browser+test-transport combinations are fully race-free.
  - *Given* this story's completion, *When* the plan's Unresolved Questions section is checked, *Then* it explicitly names this gap (see Unresolved Questions above) rather than the Success Metrics section being marked fully met for the ssq-mux case.
**Files**: `server/services/connectrpc_websocket.go` (or wherever `ExternalStreamer` sessions are wired to `resolveSession`/hub attach)

##### Task 4.1.2a: Attach `MuxTransport` with `CanResize: false, CanWrite: false` at the ssq-mux session integration point (~4 min)
- Files: `server/services/connectrpc_websocket.go`

##### Task 4.1.2b: Write a test confirming `MuxTransport`'s resize votes (if any) have zero effect on `NegotiatedSize` (~4 min)
- Files: `session/streamhub/resize_test.go`

---

### Epic 4.2: UI connection-count indicator
**Goal**: Ship the one real user-facing surface this project needs — a small, non-alarming connection-count signal — per `research/ux.md`'s sizing (not a presence/avatar system).

#### Story 4.2.1: Expose connection count to the frontend
**As the** web UI, **I want** to know how many connections/transports are attached to the session I'm viewing, **so that** I can show a lightweight indicator instead of leaving garbled output unexplained.
**Acceptance Criteria**:
- The existing terminal-stream RPC/response carries a `connection_count` field (or a lightweight side-channel message) reflecting `hub.SubscriberCount()` for `PathHubOwned` sessions.
  - *Given* a `PathHubOwned` session with 2 attached subscribers, *When* the browser's `useTerminalStream.ts` receives the next relevant message, *Then* it observes `connection_count: 2` in the payload.
- For `PathLegacyPerConnection` sessions (flag off, the default), `connection_count` is omitted or reported as unavailable rather than fabricated from `activeControlModeStreams` (which is a generation counter, not a live count) — this must not silently show a misleading number.
  - *Given* a `PathLegacyPerConnection` session, *When* the frontend receives stream messages, *Then* no `connection_count` field is present (or it is explicitly `null`/absent), and the frontend's indicator component (Story 4.2.2) does not render for that session.
**Files**: `proto/session/v1/session.proto`, `server/services/connectrpc_websocket.go`, `web-app/src/lib/hooks/useTerminalStream.ts`

##### Task 4.2.1a: Add `connection_count` (optional `int32`) to the relevant `TerminalData` oneof message in the proto (~4 min)
- Files: `proto/session/v1/session.proto`

##### Task 4.2.1b: Run `make proto-gen` and populate `connection_count` from `hub.SubscriberCount()` only for `PathHubOwned` sessions (~5 min)
- Files: `server/services/connectrpc_websocket.go`

##### Task 4.2.1c: Parse `connection_count` in `useTerminalStream.ts`, exposing it as `undefined` when absent (~4 min)
- Files: `web-app/src/lib/hooks/useTerminalStream.ts`

##### Task 4.2.1d: Run `make registry-generate` (proto field is RPC-adjacent; confirm feature registry doesn't need a marker update, or add one) (~3 min)
- Files: `docs/registry/features/*.json` (only if `make registry-diff` shows a change)

#### Story 4.2.2: `role="status"`/`aria-live="polite"` connection-count indicator with tooltip explanation
**As an** operator with two tabs open on the same session, **I want** a calm, factual indicator instead of guessing whether garbled output means my agent crashed, **so that** I can distinguish "another connection is attached" from "something is actually broken."
**Acceptance Criteria**:
- A small indicator near the terminal chrome renders only when `connection_count > 1`, using `role="status"` + `aria-live="polite"`, matching `TerminalOutput.tsx`'s existing reconnecting-banner/resizing-overlay convention.
  - *Given* `connection_count` transitions from `1` to `2` for the currently-viewed session, *When* the transition is rendered, *Then* an element with `role="status"` and `aria-label` containing "2 connections active" appears, and a screen reader announces it exactly once (not on initial mount if the count was already `2` at mount time — changes-only announcement).
- The indicator does not use `role="alert"` and does not block interaction — it is informational, per `research/ux.md`'s error-state taxonomy.
  - *Given* the indicator is visible, *When* a screen-reader user is mid-task typing into the terminal, *Then* the indicator's `aria-live="polite"` announcement does not interrupt their current input focus (verified via the existing accessibility test pattern used for `TerminalOutput.tsx`'s resizing overlay).
- Hovering/tapping the indicator reveals a tooltip explaining a resize mismatch, if relevant, rather than a second live region.
  - *Given* `connection_count = 2` and this tab's last `ResizeVote` did not win the `NegotiatedSize` negotiation (i.e., the pane is smaller than this tab requested), *When* the user hovers/taps the indicator, *Then* the expanded tooltip text includes "Another connection has this session open at a different size" — and no second `aria-live` region fires for this fact (it's folded into the existing indicator's expanded state, not announced separately).
**Files**: `web-app/src/components/sessions/TerminalOutput.tsx`, `web-app/src/components/sessions/ConnectionCountIndicator.tsx` (new), `web-app/src/components/sessions/ConnectionCountIndicator.test.tsx` (new)

##### Task 4.2.2a: Create `ConnectionCountIndicator.tsx` with `role="status"`/`aria-live="polite"`, changes-only announcement (~5 min)
- Files: `web-app/src/components/sessions/ConnectionCountIndicator.tsx`

##### Task 4.2.2b: Add hover/tap-revealed tooltip text for the resize-mismatch case (~4 min)
- Files: `web-app/src/components/sessions/ConnectionCountIndicator.tsx`

##### Task 4.2.2c: Mount `ConnectionCountIndicator` in `TerminalOutput.tsx`, gated on `connection_count > 1` (~4 min)
- Files: `web-app/src/components/sessions/TerminalOutput.tsx`

##### Task 4.2.2d: Write component tests: renders only when count > 1, changes-only announcement, tooltip content (~5 min)
- Files: `web-app/src/components/sessions/ConnectionCountIndicator.test.tsx`

---

## Follow-on Work (explicitly out of this pass)

- ssq-mux resize-authority unification (rewiring `Multiplexer.handleClient`'s direct `SetWindowSize` calls through the hub) — ADR-004, Unresolved Questions.
- Additional sink types: audit/recording, webhook, file, SSE read-only viewer, cross-host socket transport (blocked on the not-yet-confirmed "Workspace Host Registry" reference).
- `coder/websocket` migration for `WebSocketTransport`'s underlying library.
- Native `connect.BidiStream` transport (needs an HTTP/2/h2c deployment change).
- Presence/"who's viewing" UI beyond the connection-count indicator (explicitly ruled out as disproportionate per `research/ux.md`).
- Retention/privacy policy for a future audit/recording sink (named gap, not solved here).
- Old `PathLegacyPerConnection` code removal — a separate, later change gated on the 14-day trial period in Risk Control.
