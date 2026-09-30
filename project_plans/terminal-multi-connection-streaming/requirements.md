# Requirements: terminal-multi-connection-streaming

**Date**: 2026-08-20
**Type**: architecture redesign (core streaming layer)
**Complexity**: 4 — high-stakes / cross-cutting

## Problem Statement

`server/services/connectrpc_websocket.go`'s `streamViaControlMode` gives each WebSocket connection independent, unmediated ownership of a session's tmux control-mode process: every connection resizes the shared tmux pane, waits for quiescence, and captures its own initial snapshot. When two connections exist for the same session at once — a browser tab reconnecting after a server restart, a second tab/device, or an external viewer (ssq-mux) attached alongside the web UI — nothing coordinates their resize/capture operations. A connection can capture a snapshot mid-resize by a *different* connection, producing garbled, overlapping terminal output (confirmed live, 2026-08-20, immediately after a `make install-service` restart force-disconnected and reconnected every open terminal at once).

Separately, the transport is hard-wired: `connectWebSocketStream` wraps `*websocket.Conn` (gorilla/websocket) directly, and today's opportunistic coalescing (`streamViaControlMode`, `server/services/connectrpc_websocket.go:800-821` — up to 32 immediately-available frames drained into a single write, capped for worst-case latency) operates independently per connection: each of a session's N subscribers runs its own coalescing loop against the same underlying tmux output, so a burst is coalesced-and-sent N separate times instead of once. Adding a new consumer type (an audit/recording sink, a different transport, a test harness) today means threading a new implementation through code that also owns resize/capture/broadcast logic, because those concerns aren't separated.

This item redesigns the architecture so that (a) a session's tmux stream has exactly one owner, eliminating the resize/capture race structurally rather than by convention, (b) arbitrary consumer types can attach to that owned stream through a small transport interface instead of being special-cased, and (c) the wire protocol can batch/coalesce updates across a session's subscribers instead of coalescing independently, once per connection.

## Baseline

Today: one `streamViaControlMode` invocation per WebSocket connection, each independently driving tmux resize + capture-pane for its session. Each connection already opportunistically coalesces its own outbound frames (up to 32 immediately-available frames per write, `server/services/connectrpc_websocket.go:800-821`) — batching within a connection is not new — but that coalescing is per-connection: with N subscribers on one session, the same burst of underlying tmux output is drained, coalesced, and sent N independent times (once per connection's own read of `updateChan`), and each connection also independently resizes/captures. No registry of how many connections are attached to a session. No abstraction between "drive this tmux session" and "speak WebSocket to this browser tab" — they're the same code path. A same-session external viewer (ssq-mux) uses an entirely separate code path with no coordination with the browser-facing one. A 2026-08-20 commit (`420584566`) added observability (a per-session generation counter that logs a WARN when a second stream starts before the first finishes) but does not prevent the race — it only proves it's happening.

## Users / Consumers

- The stapler-squad web UI (`web-app/`), specifically `XtermTerminal.tsx` / `TerminalOutput.tsx` / `useTerminalStream.ts` — the primary and highest-volume consumer.
- External IDE terminals via `ssq-mux` (`.claude/docs/pty-multiplexing.md`) — a second, architecturally distinct consumer today.
- Future/aspirational consumers named directly by the requester: additional browser tabs/devices on the same session, a recording/audit sink, a different transport (ConnectRPC bidi stream, plain socket for cross-host streaming (depends on the Workspace Host Registry designed in a *different* project's ADR, `project_plans/backlog-deep-linking/decisions/ADR-002-gossip-based-host-registry.md` — this project has no ADR of its own by that name or number; see the Non-functional Requirements/Security note and Scope's Out of Scope section for the same citation), SSE for read-only viewers), and a test harness that needs to exercise the hub/broadcast logic without a real WebSocket or tmux process.

## Success Metrics

- **Race eliminated, not just detected**: with N ≥ 2 concurrent connections to the same session (including a rapid reconnect), the `420584566` overlap-detection WARN never fires under the new architecture, because at most one component ever resizes/captures a given tmux session at a time — verified by a stress test that opens/closes many concurrent connections against one session.
- **New consumer types don't touch core logic**: adding the in-memory test transport and formalizing ssq-mux as a subscriber both happen by implementing one transport interface, with zero changes to the resize/capture/broadcast (hub) code — verified by the diff for each.
- **Testability**: the hub/broadcast logic has unit test coverage that runs with no real tmux process and no real WebSocket server, using the in-memory transport.
- **Duplicated per-connection work eliminated under concurrency**: with multiple subscribers on one session receiving the same burst of output, the underlying tmux output is coalesced and captured once at the hub, not independently by each of N connections — measured as a reduction in redundant hub-side coalescing/capture work versus today's N-independent-coalescing baseline (each of N subscribers separately draining and coalescing the same burst), not a naive "batched vs. unbatched" comparison, since per-connection coalescing already exists today (exact target set during planning once the batching design is chosen; see Story 2.1.1).
- **No regression**: existing single-connection browser sessions behave identically from the user's perspective (latency, redraw correctness) — verified via the existing Playwright terminal e2e coverage plus `terminal-jank`/`terminal-resize-fit-loop`'s prior test suites.
- **Operator-observed outcome**: zero self-reported incidents of garbled/overlapping terminal output during the 14-day trial period (Risk Control) once the hub path is the effective default for a session — the user-facing bar this project ultimately exists to clear, distinct from the engineering-process proxies above.

## Appetite

Extra-large / phased, no hard deadline. This SDD pass plans and ships the **foundational** layer: one owning "session stream hub" per tmux session, a transport interface with at least three real implementations (browser WebSocket, ssq-mux Unix socket, in-memory test transport), and the batched wire protocol design implemented for at least the browser WebSocket path. Additional sink types (audit/recording, webhook, file, SSE, cross-host socket) are explicitly follow-on work built against the same interface, not required in this pass.
*(Scope must fit the appetite. If new gaps emerge during planning that don't fit, cut them to follow-on work — do not silently expand this pass or move the goalposts.)*

## Constraints

- No hard boundary was placed on what existing code this may touch (explicitly confirmed) — the browser wire protocol, the ssq-mux flow, and the capture-pane polling fallback are all open to being redesigned or reconciled into the new model if that's the right architecture, not preserved for their own sake.
- Rollout: feature-flag dark-launch. The new hub/transport path ships behind a flag (mirroring the existing `STAPLER_SQUAD_USE_CONTROL_MODE` pattern) alongside the current per-connection path; the old path is only removed after the new one is proven in production.
- This is a live, currently-deployed production system (the operator's own daily-driver instance) — planning must account for a real rollback path, not just "revert the commit."

## Non-functional Requirements

- **Performance SLO**: not formally specified; qualitatively, redraw latency and input responsiveness must not regress versus today's control-mode path (this repo already has prior, well-documented work — `project_plans/terminal-jank`, `project_plans/terminal-resize-fit-loop` — establishing what "acceptable" looks like; the new design must not reintroduce those classes of jank).
- **Scalability**: expected concurrency is small (single-operator instance, a handful of concurrent connections per session at most) — this is about correctness and architectural flexibility under realistic concurrency, not high-throughput multi-tenant scale.
- **Security classification**: internal/personal-use tool; no new external network exposure implied by this item (new transports like a cross-host socket are explicitly follow-on and would carry their own security review, per the precedent of TOFU/Ed25519 review for anything that leaves localhost established in `project_plans/backlog-deep-linking/decisions/ADR-002-gossip-based-host-registry.md` — that project's own ADR-002, not this project's; this project has no ADR of that name).
- **Data residency**: not applicable.
- **Testability (elevated to a first-class NFR per explicit request)**: the hub/broadcast core must be unit-testable without a real tmux process or real network transport.

## Scope

### In Scope
- A single "stream owner" per tmux session (a hub) that performs resize + quiescence-wait + capture-pane exactly once per resize event, regardless of how many connections are attached.
- A transport abstraction that the hub broadcasts to, replacing direct `*websocket.Conn` coupling in the hot path.
- At least three transport implementations: browser WebSocket (replacing today's direct-coupled path), ssq-mux Unix socket (folding today's separate external-session path into the same model, if planning confirms this is the right integration point), and an in-memory transport for tests.
- A per-session connection registry (how many/which connections are attached right now) — this is also what the requester meant by "understand multiple connections."
- A wire-protocol batching/coalescing design (e.g. per-tick frame coalescing across a session's subscribers) implemented for at least the browser WebSocket transport.
- Removal or retirement plan for the `420584566` observability-only race detector once the new architecture makes the race structurally impossible (or its continued role as a regression safety net, if planning decides to keep it).
- Feature flag to dark-launch the new path alongside the old one.

### Out of Scope
- Building out additional concrete sink types beyond the three transports above (audit/recording sink, webhook, file, SSE, cross-host socket) — these are follow-on work against the new interface.
- Exotic/speculative transports (WebAssembly-based clients, Chrome Isolated Web Apps' Direct Sockets API) as *implementations* — these are a **research question** for Phase 2 (see Rabbit Holes), not a commitment to build.
- Any change to `streamViaTmuxCapturePane` (the capture-pane polling fallback for unmanaged sessions / control-mode-disabled deployments) beyond what's needed for architectural consistency — it may be left as-is if unifying it isn't cheap.
- Cross-host session streaming (a session on one machine streamed to a viewer on another) — this is a natural future consumer of the new transport interface but depends on the Workspace Host Registry work designed in `project_plans/backlog-deep-linking/decisions/ADR-002-gossip-based-host-registry.md` (a different project's ADR, not one belonging to this project — see the Non-functional Requirements/Security note above) and is not being built now.

## Rabbit Holes

- **ssq-mux unification**: folding the external-session Unix-socket flow into the same hub/transport model may be straightforward or may uncover protocol-level mismatches (ssq-mux's PTY multiplexing assumptions vs. tmux control-mode's) that turn a "third transport implementation" into its own multi-week redesign. Phase 3 planning must explicitly scope this or defer it.
- **Exotic browser transports**: the requester raised WebAssembly-based clients and Chrome's Isolated Web Apps Direct Sockets API (`https://developer.chrome.com/docs/iwa/direct-sockets`) as directions worth researching. IWA Direct Sockets requires an Isolated Web App package (not a normal browser tab) and is a narrow, non-standard platform capability — Phase 2 research must determine feasibility/relevance before this goes anywhere near a design decision, and it is entirely plausible the answer is "not applicable to a normal browser tab today, revisit if requirements change."
- **Batching/coalescing correctness**: naive frame coalescing can reorder or merge output in ways that break escape-sequence integrity (a batching boundary landing mid-escape-sequence) — this repo's own `terminal-jank` work already dealt with adjacent issues (ED3 filtering, quiescence detection). The batching design must be reviewed against that prior work, not designed in a vacuum.
- **tmux as the single source of truth**: tmux control-mode itself has no native concept of "multiple resize authorities" — the hub design needs to decide whether resize dimensions are negotiated (e.g., smallest-common-size across subscribers, like real tmux client attachment) or whether one subscriber is authoritative. This is a real design decision, not a mechanical refactor.

## Alternatives Considered

- **Keep per-connection ownership, add a distributed lock**: the alternative already recommended in the immediately-preceding conversation (a per-session mutex around just the resize+capture critical section). Rejected as the sole solution because it only fixes the race — it doesn't address the transport-coupling or "stream wherever" goals the requester explicitly asked for. It may still be a useful building block *inside* the new hub design (Phase 3 to decide).
- **Full multi-sink platform in one pass**: considered and explicitly scoped down (see Appetite) to the foundational hub + 3 transports, with additional sinks as follow-on, because a single pass building N sink types against an unproven interface risks designing the interface wrong for sinks that don't exist yet.
- **Extra-large, phased scope over a minimal fix**: the "stream wherever" architectural goal (arbitrary future consumer types attaching through one transport interface) is a deliberate architecture-runway investment made at the requester's explicit direction, not a validated multi-consumer requirement today — only one non-browser consumer (ssq-mux) exists right now, and the additional sinks named in Users/Consumers (recording/audit, cross-host socket, SSE) are stated future preferences, not demand evidenced by current usage. A future reader should not mistake this pass's extra-large/phased appetite for having been extensively user-validated; it reflects a requester-directed tradeoff of near-term scope for long-term flexibility.

## Interim Mitigation (recommended, out of scope for this plan's stories)

The live corruption bug (Problem Statement) will remain unmitigated in production for the duration of this plan's phased rollout (Phases 1-3 are weeks of work before `PathHubOwned` is even the default for one session). Rejecting the per-session mutex as this project's *sole* solution (Alternatives Considered, above) should not be read as rejecting it as an *immediate stopgap*: a narrow, separate, fast-follow PR — a per-session mutex around just the resize+capture critical section in the CURRENT `streamViaControlMode` code (`server/services/connectrpc_websocket.go`) — is recommended to ship first or in parallel with this project, to reduce the live bug's frequency while the full hub build proceeds through its phased rollout. This stopgap is explicitly **out of scope for this plan's Stories** — it is a separate, smaller change with its own PR — but it is a recommended immediate action, not a silently-accepted gap.

## Feasibility Risks

- tmux control-mode's actual behavior under multiple simultaneous attach-equivalents (this repo uses `-C` control mode, not literal multi-client `tmux attach`, so this needs direct verification rather than assuming tmux's own multi-client semantics apply) is not yet confirmed — Phase 2 research must verify this before the hub design assumes any particular resize-negotiation model.
- ssq-mux's existing protocol/assumptions may not cleanly map onto a "transport implementation of the same interface" — see Rabbit Holes.
- This is a rewrite of the most failure-visible part of the product (if the terminal doesn't render, the whole tool is unusable) on a live, single-instance deployment with no staging environment described in this repo's docs — the feature-flag dark-launch requirement exists specifically to de-risk this.

## Observability Requirements

- Per-session connection count (how many transports are currently attached to a session's hub) — replaces/extends the ad-hoc generation counter from `420584566`.
- Structured logging at hub creation, subscriber attach/detach, resize-negotiation decisions, and batching-window flush events — sufficient to reconstruct "what did the hub do and who was attached" after the fact, the same standard the `420584566` WARN was trying to meet after-the-fact.
- The `420584566` overlap-detection WARN's underlying signal (two authorities resizing one session concurrently) must become either structurally impossible (preferred) or converted into a hard invariant check (e.g. panic/error in dev, alert in prod) rather than a passive log line, once the new hub design is in place.
- Feature-flag state (old path vs. new path) must be visible in logs per session, to support the dark-launch rollout.

## Risk Control

- Feature flag (name TBD in planning, following the `STAPLER_SQUAD_USE_CONTROL_MODE` naming convention) gates the new hub/transport path; default remains the current per-connection path until the new path is verified in production use on the operator's own instance.
- Rollback: flip the flag back (no code revert needed) if the new path misbehaves; a full code revert remains available as a second-line fallback per this repo's standard git workflow.
- Old per-connection path is only removed in a later, separate change once the new path has run as the default for a defined trial period (exact duration TBD in planning).

## Open Questions

- What resize-negotiation model should the hub use when multiple subscribers request different dimensions (authoritative-subscriber, smallest-common-size, or something else)? → Phase 2/3.
- Is folding ssq-mux into the same transport interface worth doing in this pass, or should it stay a separate path with the new interface reserved for browser + test + future sinks? → Phase 3 planning, informed by Phase 2 research into ssq-mux's actual protocol.
- What's the right batching/coalescing window and unit (fixed tick interval like the existing terminal-resize-fit-loop's decoupled sampler, vs. adaptive/backpressure-driven)? → Phase 2 research + Phase 3 design.
- Are Chrome IWA Direct Sockets or WASM-based terminal clients relevant to this product at all, given it's a normal web app today? → Phase 2 research; expected answer is "not now," but must be confirmed rather than assumed.
- Exact trial-period duration and success bar before removing the old per-connection path → Phase 3 planning.
