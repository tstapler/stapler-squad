# Architecture Research: monitor-waiting-indicator

## Prior art already in the repo — build on this, don't re-derive

`project_plans/session-status-display/research/architecture.md` maps the full
existing `DetectedStatus` pipeline end-to-end (tmux capture → `StatusDetector`
regex match → `ClaudeController` cache → status-change callback →
`toProtoSubStatus()` → proto `SubStatus` enum → `WatchSessions` stream →
Redux → `SessionRow` guard → `SubStatusChip`). That doc's "Key Files Summary"
table is accurate and current; cite it rather than re-walking the pipeline.

No `project_plans/session-status-display`/`review-queue-state-detection`
research mentions this specific "N shell, N monitor still running" footer
format — it's new. But the repo already ships a **second, orthogonal**
detector for exactly this shape of problem: rate-limit detection
(`session/detection/ratelimit/`). That's the closer precedent for this
feature than the `DetectedStatus` regex-priority pipeline, for reasons below.

## Two existing patterns, and which one this feature should follow

### Pattern A: `DetectedStatus` (session/detection/detector.go)

A single enum (`StatusReady`, `StatusProcessing`, `StatusNeedsApproval`, …)
computed by `Detect()`/`DetectWithContextFromLines()` scanning a tail slice of
tmux `capture-pane` output, priority-ordered, **first match wins**. It answers
"what is the single current status of this session" — mutually exclusive
states. Adding a new state here means slotting a new regex into the priority
list and it competes for precedence with every existing pattern.

### Pattern B: `ratelimit` package (session/detection/ratelimit/)

A **separate, independently-tracked signal** that coexists with
`DetectedStatus` rather than competing inside it:

- `ratelimit.Detector.ProcessOutput()` (`session/detection/ratelimit/detector.go:219`)
  — regex-matches provider-specific rate-limit wording, transitions a small
  state enum (`StateNone/StateWaiting/StateRecovering/StateRecovered/StateFailed`,
  `detector.go:37`).
- `ratelimit.Manager` (`session/detection/ratelimit/manager.go:107`) wraps the
  detector with cooldown/reset-time parsing and an event bus.
- `ratelimit.PTYConsumer` (`session/detection/ratelimit/integration.go:96`)
  polls a `BufferReader` in a loop (`pollLoop`, `integration.go:152`) and
  calls `manager.ProcessOutput(data)` (`integration.go:164,169`) — **this
  `BufferReader` is the PTY/scrollback buffer itself**, not something
  control-mode-specific or polling-mode-specific, so the detector runs
  identically regardless of `STAPLER_SQUAD_USE_CONTROL_MODE`.
- Wired into `ClaudeController` as `cc.rateLimitHandler
  atomic.Pointer[ratelimit.PTYConsumer]` (`session/claude_controller.go:147`),
  constructed once per controller instance (`claude_controller.go:242-243`)
  and exposed via `cc.GetRateLimitState()` (`claude_controller.go:1218`).
- Read on the `Instance` via `GetRateLimitState() int`
  (`session/instance_controller.go:333`).
- Surfaced in the proto **independently** of `DetectedStatus`/`SubStatus`
  precedence: `server/adapters/instance_adapter.go:203`
  (`protoSession.RateLimitState = rateLimitStateToProto(...)`), plus a
  precedence check at `instance_adapter.go:344` where
  `ratelimit.StateWaiting` overrides `SubStatus` to `SUB_STATUS_RATE_LIMITED`
  inside `toProtoSubStatus()`. So the frontend gets *both* a dedicated
  `RateLimitState` proto field *and* a folded-in `SubStatus` override for the
  common badge slot.
- **Not** part of `InstanceSnapshot` (`session/instance_snapshot.go` has no
  `RateLimitState` field — grepped, only `RateLimitAutoResume *bool` appears,
  an unrelated config flag). It's read live off the `ClaudeController`
  (`cc.rateLimitHandler.Load()`), which is itself already safe for concurrent
  reads (atomic pointer load), not through the `i.mu`-guarded raw-field path
  `instance-lock-free-reads.md` warns about.

### Recommendation: follow Pattern B (ratelimit), not Pattern A

The shell/monitor footer is the same shape of problem as rate-limit
detection: a transient, independently-clearing signal parsed from a specific
wording pattern, not one of the mutually-exclusive lifecycle states
`DetectedStatus` already enumerates. Concretely:

1. New package `session/detection/monitorwait/` (or similar) mirroring
   `ratelimit`'s `Detector`/`Manager`/`PTYConsumer` structure — one file for
   the regex + parse (`N shell, N monitor(s)? (still running)?`), one for the
   the atomic-pointer state.
2. Wire a `cc.monitorWaitHandler atomic.Pointer[monitorwait.PTYConsumer]`
   into `ClaudeController` next to `rateLimitHandler`
   (`session/claude_controller.go:147,242-243,381,453`), same construction
   and teardown lifecycle.
3. Expose `cc.GetMonitorWaitCounts()` / `Instance.GetMonitorWaitCounts()`
   mirroring `GetRateLimitState()`'s two-layer read
   (`claude_controller.go:1218`, `instance_controller.go:333`).
4. In `server/adapters/instance_adapter.go`, add a `MonitorWaitCounts` (or
   `ShellCount`/`MonitorCount` int fields) to the proto `Session` message
   alongside `RateLimitState` (`instance_adapter.go:203`), and fold a
   precedence rule into `toProtoSubStatus()` near `instance_adapter.go:344`
   if a `SubStatus` badge slot should also reflect it (acceptance criterion 2
   wants *some* visible badge — cheapest is a new `SubStatus` value, e.g.
   `SUB_STATUS_BACKGROUND_WORK`, following the exact same enum-registration
   pattern the session-status-display doc documents at its
   "Key Files Summary" — proto enum, `toProtoSubStatus()` switch,
   `SubStatusChip.tsx` switch are the three co-located registration points,
   `architecture.md:119`).
5. Frontend: extend `SubStatusChip.tsx`'s switch with the new case (pattern
   at `SubStatusChip.tsx:24-101` in the prior doc), and/or add a small
   "N shell, N monitor" badge component reading the new proto field directly
   if the count itself (not just a boolean chip) should render.

## Why this automatically satisfies "works in both tmux modes" (AC4)

Because `ratelimit.PTYConsumer` (the model to mirror) polls a `BufferReader`
abstraction owned by `ClaudeController`, not tmux `capture-pane` output
directly, and `ClaudeController` is the single object constructed regardless
of `STAPLER_SQUAD_USE_CONTROL_MODE` (control-mode vs. legacy polling only
changes *how* `server/services/connectrpc_websocket.go:916-956` feeds bytes
into the session's buffer, not how `ClaudeController`'s detectors consume
it). Confirm the exact `BufferReader` implementation
(`session/detection/ratelimit/integration.go`'s `BufferReader` interface) is
fed the same way in both modes before implementing — this doc did not trace
that interface's concrete implementations, only confirmed the two
`STAPLER_SQUAD_USE_CONTROL_MODE` branch points
(`connectrpc_websocket.go:924,956`).

## Data flow / consistency (question 3 from the task)

Shell/monitor counts should **not** go into `InstanceSnapshot`
(`session/instance_snapshot.go`) — same reasoning as `RateLimitState`
already being excluded: this is derived, ephemeral, transient regex-detected
state read live off the `ClaudeController`'s own atomic pointer, not a
mutable `Instance` field written under `i.mu.Lock()` by the actor setters in
`session/instance_actor_setters.go`. The `instance-lock-free-reads.md` rule
targets fields that setters mutate under lock; this data never touches that
lock at all, so it's out of scope for the rule, not an exception to it.

Detection should run on **incremental buffer content**, not re-scan full
scrollback each poll — this matches `ratelimit.PTYConsumer.pollLoop()`
(`integration.go:152`), which already reads new bytes since last poll rather
than the whole scrollback. Reuse that same polling/diffing mechanism rather
than inventing a second one; the two detectors (rate-limit, monitor-wait) can
likely share one `PTYConsumer`-style poll loop feeding both managers, or run
as two independent consumers on the same `BufferReader` — decide during
planning based on whether `BufferReader` supports multiple concurrent
readers (needs verification, not confirmed in this research pass).

## Is this Event-Command-Policy or simple? (question 4)

Simple. This is pattern-detection + status-annotation, not multi-actor
business logic — no state-machine semantics beyond the two-state "counts
present / counts cleared" toggle already implied by AC1 and AC3, structurally
identical to `ratelimit`'s `StateNone ⇄ StateWaiting` toggle (minus
`ratelimit`'s extra recovery/scheduler machinery, which this feature doesn't
need — there's no "recovery action" to take for a background shell/monitor,
just a display concern). No `docs/explanation` or `docs/reference` doc in
this repo describes a formal session-lifecycle state machine that this
feature would need to plug into; `review-queue-state-detection`'s
`ADR-001-working-state-enum-vs-bool.md` is the nearest related precedent
worth reading before finalizing whether the new signal is a bool
(`HasOutstandingBackgroundWork bool`) or a richer struct
(`{ShellCount, MonitorCount int}`) — AC1 explicitly wants structured counts,
which argues for the richer struct.

## Existing related project_plans (checked, no direct overlap)

- `project_plans/session-status-display/` — general `DetectedStatus`→UI
  pipeline, cited above; no mention of shell/monitor footer.
- `project_plans/review-queue-state-detection/` — working-state enum vs bool
  ADR relevant to the design-choice question above, but scoped to review-gate
  "is Claude waiting for human input" detection, a different signal.
- `project_plans/stale-session-detection/` — idle/staleness thresholds,
  unrelated signal (session inactivity, not in-flight background work).
- No existing `project_plans/*` directory already covers this footer format.

## Open questions for the planning phase

1. Confirm `BufferReader`'s concrete implementation(s) and whether it's safe
   for two independent consumers (ratelimit + monitor-wait) to poll it
   concurrently, or whether the new detector must share the existing
   `ratelimit.Manager`'s poll loop.
2. Decide bool vs. struct for the new signal per the ADR-001 precedent above.
3. Decide whether the visible indicator is a new `SubStatus` enum value (chip
   slot, mutually exclusive with existing chips) or an additional
   independent badge element (can coexist with e.g. `NEEDS_APPROVAL`) — the
   requirements' "distinguishable from idle/needs-attention" wording doesn't
   force mutual exclusivity, and a session could plausibly be both
   needs-approval *and* have outstanding background shells simultaneously.
