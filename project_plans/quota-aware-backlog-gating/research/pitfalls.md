# Research: Pitfalls & Risks — quota-aware-backlog-gating

Scope: known failure modes for building automatic pause/throttle logic off an
inferred (not authoritative) quota signal, and how this codebase's existing
patterns/incidents should shape the design. All file:line references were
read directly from the working tree at HEAD (`ed0fda703`) on 2026-08-10.

## 1. Inferred-signal pitfalls: false positives, false negatives, flapping

**No authoritative quota API exists today.** Confirmed by re-reading the
requirements doc's own Feasibility Risks section
(`project_plans/quota-aware-backlog-gating/requirements.md:69-71`) and by
inspecting the two existing signal sources in this repo — neither is an
account-wide quota poll:

- `session/detection/ratelimit/manager.go` — per-session, **reactive**
  terminal-output pattern detection (`Detector`/`Scheduler`/`RecoveryHandler`).
  It only fires *after* a session has already hit a rate limit
  (`manager.go:195` `handleDetection`), and its state machine
  (`StateNone → StateFailed/StateRecovered → StateNone`, see
  `manager.go:240-267`) is scoped to a single session — there is no
  account-wide aggregation today. Promoting this to "any active session
  recently rate-limited → pause backlog" (the fallback increment in the
  requirements doc) inherits every false-negative in the underlying pattern
  matcher: if the detector's regex/pattern misses a wording variant, the
  account-wide signal never fires and backlog keeps burning quota — a classic
  false negative from trusting message-scraping as ground truth.
- `server/services/capacity_monitor.go` — polls **per-provider/per-model**
  API rate-limit headers (requests/tokens remaining) and context-window
  usage, not the account-wide 5-hour/weekly Claude Code session quota. It is
  a different resource axis entirely (explicitly out of scope per the
  requirements doc), but its `checkThresholds` (`capacity_monitor.go:228-248`)
  and `handleTransitionTrigger` (`capacity_monitor.go:250-313`) are the
  closest existing template for how this repo already does percentage-based
  threshold gating + rate-limited notification — reuse the *shape*, not the
  data source.

**False positive risk**: pausing backlog when quota is actually fine. Given
inference is the only available signal, expect a nonzero false-positive rate
purely from pattern-matching against terminal text that resembles but isn't a
real quota-exhaustion message. Because the requirements doc's Risk Control
section (`requirements.md:76-77`) explicitly rules this the "safe" failure
direction (recoverable via the manual toggle), lean toward that direction if
forced to choose — but a false-positive-heavy design that pauses/resumes
often erodes trust and produces exactly the flapping problem below.

**False negative risk**: not pausing before hitting the limit. Since the
signal is reactive (a session already got rate-limited) rather than
predictive, by construction there is no way to gate *before* the first
real hit — the fallback increment can only prevent the *second* session from
also getting hit, not the first. This should be stated explicitly as an
accepted limitation, not silently implied to be prevention.

**Flapping**: rapid pause/resume cycling. This repo does not have a prior
incident of *this specific* flapping shape (quota-driven feature-flag
flapping), but it has two directly analogous, documented incidents worth
mining for the general pattern — both are about a fast-changing signal racing
a slower reconciliation/read path and producing visible state oscillation:

- `ed0fda703` (`fix(backlog): stop plan-approval UI flicker on stuck items and
  item detail`, #386, merged 2026-08-10 — the commit immediately preceding
  this research) fixed exactly this class of bug: two independent 60s polls
  (`StuckNavBadge`/`StuckItemsSection`) reading possibly-stale `reason` data
  produced visible flicker, fixed by (a) gating the affordance on a more
  precise field instead of trusting a possibly-stale value alone, and (b) a
  `loadSeqRef` + `updatedAt` guard so an out-of-order/stale RPC response
  can't stomp a fresher one. **Direct lesson for this feature**: if the
  quota-headroom evaluator and `BacklogController.IsEnabled()` read/write
  from different reconcile loops or event handlers without a sequence/staleness
  guard, an out-of-order write (e.g. a slow "quota recovered" check completing
  after a fast subsequent "quota critical" check) can silently re-enable
  backlog into a still-bad quota window.
- `.claude/rules/service-restart-orphan-process.md` documents a *structural*
  hypothesis (not fully confirmed) that independent per-session/per-instance
  callbacks firing without coordination produced sessions going `Stopped`
  "one at a time" rather than atomically — the same shape of risk as N
  independent quota-check goroutines each deciding to flip
  `BacklogController` independently.

**Hysteresis is not optional** — the requirements doc already flags this
(`requirements.md:49,62`) and `capacity_monitor.go` already contains a
directly reusable precedent: `handleTransitionTrigger` rate-limits
re-triggering to once per 5 minutes per session via `lastWarningTime`
(`capacity_monitor.go:254-265`). The quota gate needs the equivalent for
*both* directions (pause and resume), not just a single crossing check —
e.g. require headroom to stay above `threshold + margin` for N consecutive
reconcile ticks before resuming, and below `threshold` for N ticks before
pausing, mirroring a Schmitt-trigger band rather than one comparison.

## 2. Race conditions: concurrent mutation of `BacklogController` state

`session/feature_controller.go` already establishes the locking pattern to
follow — `BacklogController` (`feature_controller.go:13-23`) guards
`syncLoop`/`syncCancel` with a plain `sync.Mutex`, and both `Enable`
(`feature_controller.go:43-65`) and `Disable` (`feature_controller.go:69-86`)
are explicitly documented as **idempotent** and **safe to call concurrently**
(doc comment at `feature_controller.go:12`). `IsEnabled()`
(`feature_controller.go:89-91`) reads `c.listener.enabled.Load()` — an
`atomic.Bool` on the listener, not the mutex — so reads are lock-free and
always see the latest written value.

**What this pattern does *not* give you for free**: idempotency alone
doesn't prevent a **last-writer-wins race between the quota auto-gate and
the existing manual toggle**. If a human manually re-enables backlog via the
Settings UI at the same moment the quota evaluator decides headroom is still
critical and calls `Disable()`, whichever call lands last wins with no
signal to the other caller that it was overridden. Concretely:

- There is currently **no "reason" or "source" tracked alongside enabled
  state** — `IsEnabled()` returns a single bool with no provenance. A design
  that adds an auto-gate must decide explicitly how it composes with the
  manual toggle (e.g. store a `pausedByQuota bool` sentinel distinct from the
  user's manual preference, so quota-resume doesn't stomp a user's deliberate
  manual-disable, and a user's manual-enable can either override or be
  rejected while quota is critical — this must be a design decision, not
  left implicit).

- The project's own documented golang double-checked-locking pitfall
  (`.claude/rules/go-double-checked-locking.md`) is directly relevant if the
  quota evaluator caches headroom state: "always return the locally-computed
  value, not the cache slot" after a lock/re-check — i.e. if two reconcile
  goroutines race to update a shared headroom cache, the goroutine that
  loses the write race must not report the *winner's* value as if it were
  its own observation, since that can make a stale decision look
  self-consistent when it wasn't.

**Recommendation for this feature's own controller**: follow
`BacklogController`'s existing shape exactly — one `sync.Mutex` (or reuse the
existing one via a method added to `BacklogController` itself) guarding a
small explicit state struct (`{enabled bool; pausedByQuota bool;
lastTransition time.Time; consecutiveTicksAboveThreshold int;
consecutiveTicksBelowThreshold int}`), with a single reconcile loop (not
multiple independent goroutines) as the only writer, exactly mirroring the
existing single-`SyncLoop`-per-controller model at
`feature_controller.go:56-63`.

## 3. Service-restart / in-memory-state risk

Confirmed by reading the wiring in `server/dependencies.go:1000-1015`:
`BacklogController`'s enabled state is initialized **purely from the
persisted config flag** (`cfg.GetFeatureFlag("backlog")`) on every process
start — there is no reference to any quota-headroom state at startup. This
is the same shape of problem `.claude/rules/service-restart-orphan-process.md`
and `.claude/rules/tmux-keep-server-on-restart.md` document for other
subsystems (tmux server + session state getting silently reset by a
restart), and this dev instance restarts multiple times per day per those
same docs' own account of `make install-service` usage.

Two concrete failure directions if quota-headroom state is kept in-memory
only (e.g. inside the new gate's controller, not persisted):

- **Wrongly resumes**: quota is genuinely still critical, but a restart
  discards the in-memory "paused for quota" state and falls back to whatever
  `cfg.GetFeatureFlag("backlog")` says on disk. If quota-driven pause does
  *not* also flip the persisted flag, a restart mid-pause silently resumes
  backlog into a still-critical quota window — the exact regression the
  Success Metrics section is trying to prevent, arriving through the back
  door of a routine restart rather than a design bug.
- **Wrongly stays paused**: if quota-driven pause *does* persist the flag to
  disk (to avoid the above), then a restart after quota has already
  recovered will boot with backlog still disabled and no in-memory record of
  *why* it was disabled or when it should re-check — it will sit paused
  until the next reconcile tick evaluates fresh headroom, which is probably
  fine here (safe direction) but only if the reconcile loop is guaranteed to
  run again after restart without requiring a manual toggle.

**Recommendation**: treat "paused for quota" as a *transient* signal that is
always re-derived fresh on every reconcile tick (including the first one
after a restart), never trusted from a stale on-disk value. If persisting a
"paused for quota, last known reason X" flag for UI-visibility purposes, the
reconcile loop must re-evaluate real headroom on startup before trusting
that persisted reason, rather than treating "was paused" as authoritative
state to restore as-is.

## 4. Silent-notification pitfall — does `capacity_monitor.go` actually satisfy the standing rule?

The user's memory (`feedback_document_ai_decisions_in_edge_cases`) and the
requirements doc's Constraints section (`requirements.md:36`) both require
mirroring `capacity_monitor.go`'s alert pattern: "pause/resume actions must
post a visible notification, never act silently."

Reading `capacity_monitor.go:282-312` (`handleTransitionTrigger`) closely:
it **does** publish a visible notification via
`m.eventBus.Publish(events.NewNotificationEvent(...))` with a real title
("Capacity Alert"), message, and priority — this part is a good template to
copy directly.

**Where it falls short as a template for a pause/resume pair specifically**:
`capacity_monitor.go` only has *one* transition direction instrumented —
a one-shot "warning fired" event, rate-limited to once per 5 minutes per
session (`capacity_monitor.go:258-265`). There is no corresponding "capacity
recovered" / "warning cleared" notification anywhere in the file — grepping
the file confirms no second `eventBus.Publish` call exists outside
`handleTransitionTrigger`. So while the *pause*-shaped half of the pattern
(`capacity_monitor.go`'s trigger) is a faithful template, **the *resume*-shaped
half has no existing precedent in this codebase to copy** — the quota gate's
resume notification will need to be designed from scratch (new event type or
reused `NewNotificationEvent` call with different metadata/title on the
"headroom recovered above threshold + hysteresis" transition), not assumed
already-solved by pointing at `capacity_monitor.go`.

Additionally: `capacity_monitor.go`'s notification embeds a machine-readable
`type: "capacity_alert"` field in its `map[string]string` metadata
(`capacity_monitor.go:296-301`) — the quota gate should use an equally
distinct `type` (e.g. `"quota_gate_paused"` / `"quota_gate_resumed"`) so the
frontend/notification list can distinguish and correlate the pair, rather
than reusing `capacity_monitor`'s literal `type` value and getting
conflated with unrelated per-session capacity warnings in the UI.

## Summary of concrete design obligations this research surfaces

1. Do not claim prevention of the *first* rate-limit hit — the reactive
   signal can only prevent compounding hits after the first one is observed.
2. Implement a Schmitt-trigger-style hysteresis (N consecutive ticks each
   direction), not a single-crossing check — precedent:
   `capacity_monitor.go`'s 5-minute per-session re-trigger cooldown.
3. Give `BacklogController` (or a wrapping quota-gate controller) an explicit
   "why disabled" provenance (`pausedByQuota` vs. manual) so the auto-gate
   and the manual toggle don't silently stomp each other — no such
   provenance exists today; `IsEnabled()` is a single bool.
4. Follow the single-mutex, single-writer-loop pattern already established
   by `BacklogController.Enable`/`Disable` — don't introduce a second
   independent goroutine race that can write `enabled` state out of order
   (see `ed0fda703`'s stale-response guard as the precedent for why this
   matters even for read paths).
5. Never trust a persisted "paused for quota" flag as authoritative across a
   restart — always re-derive headroom fresh on the first post-restart
   reconcile tick, matching this dev instance's documented multiple-per-day
   restart cadence.
6. Build both the pause *and* the resume notification explicitly — only the
   pause half has a copyable precedent in `capacity_monitor.go` today.
