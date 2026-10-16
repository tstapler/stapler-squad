# Requirements: terminal-resync-reliability

**Date**: 2026-08-13
**Type**: bug fix / reliability hardening (cross-cutting: client + server + wire protocol)
**Complexity**: 4 — high-stakes / cross-cutting

## Problem Statement

The visibility/focus resync feature (shipped in `96990ce12`, PR #184) fixes tab-backgrounding
corruption but introduces a "resync storm" under concurrent terminals. `SessionDetailView.tsx`
keeps every terminal for a session mounted and connected simultaneously (for keep-alive), and
every mounted `TerminalOutput` independently listens to the same document-level
`visibilitychange`/`focus` events. One tab-focus event fires a `requestFullResync(true)` burst
across every mounted terminal at once — not just the one the user is looking at.

Each resync request goes through a mid-connection `CurrentPaneRequest` handler
(`connectrpc_websocket.go`) that takes an expensive ~450ms+ slow path when client-reported
terminal dimensions (plausibly stale for backgrounded xterm.js instances) don't match the
server-side tmux pane. All resyncs also contend for the same shared 8-slot tmux exec-gate
(`session/tmux/exec_gate.go`, `defaultTmuxExecGateSlots = 8`), since virtually all sessions map
to the same `"default"` gate key.

The resync's completion signal has no correlation ID — `notifyResyncOutputReceived()` fires on
ANY `output`-type message, not specifically the resync's own response. The actively-focused
terminal gets lucky: continuous ambient output (keystroke echo, shell activity) satisfies this
heuristic quickly regardless of queue position. Background/idle terminals have no such traffic,
so under a multi-terminal burst they're the ones most likely to sit queued past the 4-second
`RESYNC_STALL_TIMEOUT_MS` watchdog and get force-disconnected/reconnected — which matches the
user's own observation: "it's usually not the one I'm typing in that runs into trouble."

## Baseline

Today, opening/focusing a browser tab with several concurrent terminal sessions frequently
triggers the `[resync] ... stall watchdog fired after 4000ms, forcing disconnect+reconnect`
console warning on one or more background terminals, causing a visible disconnect/reconnect
cycle on tabs the user wasn't even interacting with. There is no visibility into how often this
happens, how large a "burst" is, or how close to the 8-slot exec-gate capacity a given burst
gets — the only signal is the client-side console warning and the user's own experience of
tabs "restarting."

## Users / Consumers

Any user running multiple concurrent Claude Code / Aider sessions in the stapler-squad web UI
(the primary and, per the project's own usage pattern, most common way it's used) — i.e.
essentially all active users of the product, not a narrow subset.

## Success Metrics

- Stall-watchdog fires (disconnect+reconnect forced by `RESYNC_STALL_TIMEOUT_MS`) drop to
  near-zero for backgrounded/non-focused terminals during a multi-terminal focus event, measured
  against the baseline rate observed today.
- A single tab-focus event with N mounted terminals no longer produces N simultaneous
  full-resync wire requests — resyncs for non-visible terminals are scoped out, staggered, or
  batched so the exec-gate and server slow path are not hit by a synchronized burst.
- The server's dimension-mismatch slow path (~450ms+ of hardcoded sleeps) is no longer
  triggered by resync requests from terminals whose reported dimensions are stale because they
  were backgrounded (not because they were genuinely resized).
- Resync-related wire traffic (request count and/or bytes) for a multi-terminal focus event is
  measurably reduced via batching/compression, compared to the current one-request-per-terminal
  baseline.

**Scope note (added during implementation planning, `implementation/plan.md` Task 6.1.1.5, cross-ref
`implementation/pre-mortem.md` P2 #3)**: the first two metrics above are measured **per session**
(across the terminals mounted within one `SessionDetailView`), not across a user's full set of
concurrently open sessions. A synchronized resync burst *across* several simultaneously-open
sessions (all sharing one tab-focus instant, and — if they share a tmux socket — the same exec-gate
fast lane) is a real but narrower and rarer condition than the primary single-session,
multi-terminal burst this project targets, and is explicitly out of scope for this Large-appetite
effort. It is tracked as its own follow-up, not silently absorbed into "near-zero."

## Appetite

Large (3–6 weeks)

*(Scope must fit the appetite. If it doesn't fit, cut scope — do not move the deadline.)*

## Constraints

- Must not regress or modify (byte-for-byte) the three pre-existing full-resync/refit triggers
  documented as out-of-scope in the original `terminal-visibility-resync/requirements.md`:
  mount-time fit, the manual Reconnect button, and ResizeObserver-driven fit.
- Must not remove or weaken the underlying guarantee the original feature shipped to fix
  (backgrounded-tab xterm.js buffer divergence/corruption) — this is a reliability hardening of
  that feature, not a rollback of it.
- This is a live, shipped feature carrying real user traffic — changes must ship behind a
  feature flag (see Risk Control) rather than as a direct behavioral change.

## Non-functional Requirements

- **Performance SLO**: not formally specified prior to this project; this project's own success
  metrics (stall-watchdog fire rate, resync request count/bytes per focus event) serve as the
  de facto targets — see Observability Requirements for how they'll be measured.
- **Scalability**: must hold up under the project's own typical concurrency pattern — many
  simultaneously mounted terminals per session, many sessions per user.
- **Security classification**: internal (self-hosted dev tool, no regulated data in scope).
- **Data residency**: not applicable.

## Scope

### In Scope

All five fix categories identified in the architectural review, all selected by the user as
in-scope (multi-select, all four proposed options plus one user-added item):

1. **Scope resync to visibility** — only fire `requestFullResync` for terminals that are
   actually visible/foregrounded, instead of every mounted instance reacting to the same global
   `visibilitychange`/`focus` event. Requires wiring the existing `isVisible`/`foreground` prop
   (currently cosmetic-only per the architectural review) into the resync trigger path in
   `useVisibilityResync.ts`.
2. **Add a correlation ID to resync** — tag each `CurrentPaneRequest` resync with an ID that its
   corresponding server response echoes back, so `notifyResyncOutputReceived()` can clear the
   *specific* pending resync it's tied to rather than any arbitrary `output` message satisfying
   it. Removes the "ambient output masks the real completion signal" failure mode.
3. **Server-side capacity fixes** — investigate and address both: (a) skipping or
   short-circuiting the expensive dimension-mismatch slow path in
   `connectrpc_websocket.go`/`streamViaTmuxCapture` when the mismatch is attributable to a
   backgrounded client's stale reported dimensions rather than a genuine resize; and (b)
   exec-gate capacity — raising `defaultTmuxExecGateSlots` and/or adding a fast lane so resync
   bursts don't queue behind unrelated tmux work on the shared `"default"` gate key.
4. **Stagger/prioritize resync bursts** — when multiple terminals do need to resync together
   (e.g. several are simultaneously visible in a split view), spread their requests over time
   or prioritize by visibility/recency instead of firing all of them synchronously on the same
   event tick.
5. **Batch updates / reduce round trips and improve wire compression** (user-added, not
   originally part of the architectural review) — redesign the resync wire protocol so that,
   where multiple terminals must resync together, their requests/responses are batched into
   fewer round trips, and/or the payload benefits from better compression, rather than each
   terminal making an independent full-duplex request. Given the Large appetite selected, this
   is a real protocol change, not a stretch goal — but the exact mechanism (coalescing multiple
   `CurrentPaneRequest`s into one message, frame-level compression, or both) is left for Phase 2
   research to resolve; see Rabbit Holes.

All five ship behind the feature flag described in Risk Control — none change default runtime
behavior until explicitly enabled.

### Out of Scope

- The three pre-existing full-resync triggers named in Constraints above (left byte-for-byte
  unmodified, per the original feature's own out-of-scope boundary).
- The existing `NEXT_PUBLIC_RECONNECT_V2`-gated visibility/online reconnect-when-disconnected
  listener in `useTerminalStream.ts` — left as-is.
- General terminal output streaming performance/compression unrelated to the resync path (e.g.
  normal tmux control-mode output frames during active, non-resync use) — batching/compression
  in scope here is specifically for the resync burst path, not a wholesale streaming-protocol
  rewrite.
- Fixing `useBrowserLogStream.ts`'s console-patching multi-mount issue (noted in that file's own
  comment) — a real but unrelated multi-mount concern, not part of the resync-storm problem.
- Any change to the underlying tmux control-mode protocol itself (only the resync/`CurrentPaneRequest`
  path is in scope).

## Rabbit Holes

- **Batching/compression mechanism is unspecified.** "Batch updates... better compression over
  the wire" was added by the user without a specific design in mind. This could mean anything
  from a small protocol tweak (one `BatchedCurrentPaneRequest` message covering N pane IDs) to a
  much larger change (frame-level compression middleware for the whole ConnectRPC stream).
  Phase 2 research must scope this down to something concretely buildable within the Large
  appetite, and Phase 3 planning must treat it as its own epic with an explicit go/no-go rather
  than assuming it falls out naturally from the other four fixes.
- **Correlation ID plumbing touches two independent state machines.** The architectural review
  found `useTerminalFlowControl` and `useVisibilityResync` maintain separate,
  redundantly-overlapping pending-resync state (`isResyncingRef`/`waitingForPaneResponseRef` vs.
  `pendingResyncCompletionRef`/`bannerShownRef`). Adding a correlation ID could either thread
  through both as-is or become an opportunity (out of scope unless it falls out naturally) to
  collapse them into one state machine — resist scope creep here.
- **Server-side dimension-mismatch detection for "backgrounded vs. genuinely resized" is a new
  heuristic that doesn't exist today.** The server currently has no way to know a client's
  reported dimensions are stale specifically *because* the tab was backgrounded (as opposed to a
  real resize it should honor) — building this signal may require new client-to-server
  information, not just a server-side code path change.
- **Exec-gate capacity increase is a blast-radius question, not just a config change.** Slots
  are shared across all tmux subprocess calls on the `"default"` key, not just resyncs — raising
  the slot count changes contention behavior for every kind of tmux operation, not only the
  resync path this project is scoped to fix.

## Alternatives Considered

- **Do nothing / accept the stall-and-reconnect behavior** — rejected: reconnect cycles are
  visibly disruptive and the user explicitly asked for an architectural fix, not a workaround.
- **Simplest possible fix (visibility-scoping only)** — considered as the Medium-appetite option
  during scoping; rejected once Large appetite was chosen, in favor of addressing the full set of
  contributing causes (client trigger fan-out, missing correlation, server slow path, exec-gate
  contention, and wire efficiency) rather than only the most visible one.

## Feasibility Risks

- The two overlapping resync state machines (`useTerminalFlowControl` vs `useVisibilityResync`)
  make correlation-ID plumbing more error-prone than a single-state-machine design would be —
  see Rabbit Holes.
- Raising exec-gate slot capacity is shared infra used by all tmux operations, not just resync —
  changes here carry contention/regression risk beyond this project's own scope (see Rabbit
  Holes) and need their own before/after load characterization, not just a config bump.
- Batching/compression protocol design (see Rabbit Holes) is the least-scoped item and has the
  highest risk of expanding past the Large appetite if not tightly bounded in Phase 2/3.
- This is a change to a live, currently-shipped, traffic-bearing feature — any regression risks
  reintroducing the original tab-backgrounding corruption bug the feature was built to fix, not
  just a fresh bug.

## Observability Requirements

*(complexity ≥ 3)*

- Emit a counter/log for stall-watchdog fires (`RESYNC_STALL_TIMEOUT_MS` triggering
  disconnect+reconnect), tagged by whether the terminal was visible/foregrounded at the time —
  this is the primary before/after signal for the Success Metrics above.
- Emit a counter/log for resync burst size (number of simultaneous `CurrentPaneRequest`s
  triggered by a single visibility/focus event) to validate that visibility-scoping and
  staggering actually reduce fan-out.
- Emit a metric/log for exec-gate wait time or queue depth on the `"default"` key, to
  characterize contention before and after the capacity fix.
- Emit a metric/log for how often the server's dimension-mismatch slow path is taken during a
  resync, to validate the skip-slow-path fix.
- If batching/compression ships, emit request-count and/or byte-count metrics for resync traffic
  to demonstrate the round-trip/wire-size reduction claimed in Success Metrics.

## Risk Control

*(complexity ≥ 3)*

Feature flag. All five fix categories ship behind a single config flag (e.g. in
`config/types.go`, following the existing `TmuxExecGateConfig` pattern) so the new
resync/batching behavior can be disabled instantly without a redeploy if it misbehaves in
production, falling back to today's shipped behavior. Exact flag name(s) and whether one flag
covers all five fixes or each fix gets its own toggle is left for Phase 3 planning.

## Open Questions

- Should the feature flag be one flag covering all five fixes, or a separate flag per fix (so a
  regression in one, e.g. batching, doesn't force disabling all five)? Leaning toward
  per-fix flags given the Rabbit Holes above, but this is a Phase 3 planning decision.
- What's the concrete design for batching/compression (see Rabbit Holes) — resolve in Phase 2
  research.
- Does raising exec-gate capacity need to be scoped only to resync traffic (a new, separate gate
  key/fast lane) rather than raising the shared `"default"` slot count globally? Needs research
  into blast radius on other tmux operations.
