# Research: Feature Landscape — quota-aware-backlog-gating

## 1. Notification precedent: `CapacityMonitor` alert pattern

`server/services/capacity_monitor.go`'s `handleTransitionTrigger` (lines 250–313) is the
exact precedent requirement 3 should mirror:

1. Rate-limits its own alerting per key (there: per session title, 5 min cooldown —
   `lastWarningTime` map, checked/updated under `m.mu`) so a threshold that stays crossed
   doesn't spam a notification every poll tick.
2. Logs a structured `log.Warn`/`log.Info` line for the transition (`"CapacityMonitor:
   trigger warning"`) *before* publishing the user-visible event — satisfies the
   Observability Requirements section's ask for structured logs on every gate transition.
3. Publishes via `m.eventBus.Publish(events.NewNotificationEvent(sessionUUID, title,
   dedupeKey, notificationType, priority, title, message, metadataMap))`. The
   `map[string]string` metadata tail (`"type": "capacity_alert"`, `"reason": reason, ...`)
   is how the frontend can distinguish notification subtypes without parsing the message
   string.
4. Optionally performs an automatic side effect (`sessionSwitcher.UpdateSessionProgram`)
   gated by a separate `TransitionMode` config value (`manual`/`auto`/`notify`) — i.e. this
   codebase already has the "notify vs. auto-act" mode split baked into `config.TransitionMode`
   (`config/types.go:273-282`). Worth considering whether quota gating reuses this enum or
   defines its own — the requirements already mandate auto pause/resume, so `notify`-only
   mode may not apply, but the three-way split is a reusable idiom if a "warn only, don't
   pause yet" pre-threshold tier is wanted.

A second, more directly on-point precedent: `server/services/backlog_notifier.go`'s
`EventBusNotifier.Notify` — already exists specifically as the backlog package's
notification adapter (`session.Notifier` interface, wired via
`BacklogLifecycleListener.SetNotifier` / `BacklogService.SetEventBus`). It threads `itemID`
as the event's `sessionID` field specifically so the frontend's coalescing key
(`sessionID:notificationType`, see `server/notifications/subscriber.go`) doesn't collide
across different backlog items — a bug that was fixed once already (see the comment in that
file). A quota-pause notification has no natural "item ID" (it's account-wide, not
per-item), so a **fixed sentinel key** (e.g. `"quota-gate"`) should be used consistently for
both the pause and resume events so they coalesce/replace each other correctly instead of
each getting a random UUID key that never collides with its own history.

`config.CapacityConfig` (`config/types.go:291-330`) is the concrete precedent for
"configurable thresholds, not hardcoded" per Constraints: a `TransitionMode` enum, two float
percentages (`*WarnPct`/`*AutoPct`), a poll interval, and a `CapacityConfigOrDefault()`
method that backfills zero-valued fields with defaults. The new quota config should follow
this exact shape (a new `config.QuotaGateConfig` struct with its own `*OrDefault()`,
registered in `config.go`'s `Config` struct and `LoadConfig`/`SaveConfig` defaulting paths
at lines 339/463/916 — same three touch points `CapacityConfig` uses).

## 2. Foreground vs. background/backlog session distinction

Two independent, already-existing signals — neither one is currently combined into a single
"is this a human-driven foreground session" predicate, so the plan needs to define that
combination itself, not invent a third mechanism:

- **`session.CategoryBacklog = "Backlog"`** (`session/backlog.go:84-88`) — set via
  `inst.SetCategory(session.CategoryBacklog)` at every backlog session creation site
  (`server/services/backlog_service_triage.go:806,2866`, `server/services/session_service.go:869`).
  This is a field on the live `*session.Instance` (`Category string`), readable via
  `inst.Snapshot()` exactly like `CapacityMonitor.evaluateInstance` already does
  (`session/capacity_monitor.go:157-165` reads `snap.Program`; `snap.Category` is the same
  shape). Any session whose `Category != CategoryBacklog` and `Status == session.Active` is,
  by the existing convention, a human-driven session.
- **`session.SessionRoleWork / SessionRoleReview / SessionRoleTriage`** and
  `IsTmuxBackedSessionRole` (`session/backlog.go:48-75`) — this is a *different* axis: it's a
  per-`ItemSession` DB-row field (`session/storage_backlog.go`, `session/ent/itemsession.go`)
  describing a backlog item's session's *role within the backlog pipeline* (work/triage/review),
  used for archive/cleanup sweeps (killing tmux panes when an item goes terminal — see the
  2026-07-29 OOM postmortem cited in `session/backlog.go:58-64`). It is not itself a
  foreground/background predicate and has no notion of "human" sessions at all — every value
  it takes is backlog-spawned. **Do not reuse `IsTmuxBackedSessionRole` for requirement 2's
  foreground detection** — it answers "should this backlog session's tmux pane be cleaned up",
  not "is a human currently driving a session". The requirements doc's Rabbit Holes section
  flags exactly this ambiguity by name-checking `IsTmuxBackedSessionRole`; research confirms
  it is the wrong tool for this job.
- **The right building block**: `CapacityMonitor.evaluate()`'s own pattern —
  `m.poller.GetInstances()` (interface `InstancePoller`, `server/services/capacity_monitor.go:36-38`)
  then filter `inst.Snapshot().Status == session.Active`. Add a `Category != CategoryBacklog`
  filter to the same loop shape and that *is* "foreground/human session active" — no new
  primitive needed, just a composition of two fields that already exist on `Instance`.

## 3. Edge cases

### Flapping (quota recovers then drops again rapidly)
No existing hysteresis/hold-off primitive exists in this codebase for a threshold-crossing
gate (`grep -i hysteresis|debounce|flap` across `*.go` turns up only unrelated debounce
timers — file-watcher coalescing in `daemon/daemon.go`, tmux `%output` coalescing in
`session/external_tmux_streamer.go` — none is a threshold-hysteresis pattern). The closest
analog is `SessionHealthChecker.recoveryDebounced` (`session/health.go:136-152`): a
per-key consecutive-failure counter that only fires recovery once N consecutive checks are
bad, then resets. That "N consecutive polls, not one" pattern is a good template for both
directions of the quota gate (N consecutive low-quota polls before pausing, N consecutive
recovered polls *above* threshold-plus-hysteresis-margin before resuming) — this is exactly
what the requirements doc already asks for ("recovers above the threshold (plus hysteresis)")
but no code precedent implements the "plus a percentage margin" half; only the "N consecutive"
half exists as precedent. The design should combine both: a margin-based threshold *and* a
consecutive-poll count, since either alone is flap-prone (a margin alone still flaps if quota
oscillates by more than the margin; a poll-count alone still flaps if it sits exactly on the
line).

### Manual override vs. auto-gate: who wins?
This is a real collision, confirmed by reading `server/services/feature_flag_service.go`'s
`UpdateFeatureFlag` (lines 118-201) against `BacklogController` (`session/feature_controller.go`):

- The **manual/UI path** (`UpdateFeatureFlag` RPC) does two things atomically: persists
  `cfg.SetFeatureFlag("backlog", enabled)` to disk, *and* calls `ctrl.Enable(ctx)` /
  `ctrl.Disable()` on the wired `FeatureController`. If the controller call fails, it rolls
  back the disk write so disk and live state can never diverge for *that* path.
  `GetFeatureFlags` prefers the live controller state (`ctrl.IsEnabled()`) over the disk
  value whenever a controller is wired (line 104-107) — so the UI always shows truth, not
  stale disk state.
- An **auto-gate calling `backlogCtrl.Enable()`/`.Disable()` directly** (the natural
  integration point per the requirements' constraint to reuse `BacklogController.IsEnabled()`)
  bypasses `UpdateFeatureFlag` entirely — it does **not** touch `cfg.FeatureFlags` on disk.
  This has two consequences that need explicit design decisions, not just an implementation
  detail:
  1. **Restart race**: `server/dependencies.go:1007-1015` boot logic only reads
     `cfg.GetFeatureFlag("backlog")` to decide whether to call `backlogCtrl.Enable()` at
     startup — it has no notion of current quota state at boot time. If the process restarts
     while quota is genuinely still low, backlog will boot up enabled and only get
     auto-paused on the *next* quota-check ticker fire (not before). The quota gate should
     run its own check synchronously before/alongside this boot `Enable()` call, not rely on
     the reconcile ticker's first tick.
  2. **Fight-back on manual re-enable while quota is still low**: if the user manually
     flips the toggle on via the UI while quota is still under threshold, `UpdateFeatureFlag`
     will call `ctrl.Enable()` and persist `enabled: true` — succeeding, since
     `BacklogController.Enable` has no quota awareness itself (`session/feature_controller.go:43-65`
     is a pure listener/synloop start, no gating logic lives there). On the *next* quota-gate
     poll tick, if the gate's own logic sees "still below threshold" it will call `Disable()`
     again, silently reverting the user's manual action with no explanation of why it flipped
     back off — this directly violates the "document AI decisions in edge cases" instinct
     (see user's memory: "self-heal/auto-close actions should post a visible comment +
     notify(), not act silently") and the requirement to never act silently. **The design
     needs an explicit decision here**: either (a) a manual re-enable while quota is low sets
     a time-boxed "user override" flag the auto-gate respects for some window (mirrors
     `capacity_monitor.go`'s per-key cooldown pattern), with a notification explaining *why*
     it will still be gated, or (b) manual re-enable always wins until the *next* pause
     transition, with a notification on that next auto-pause explaining "user had manually
     re-enabled this, but quota dropped again." Either way, silently re-disabling a fresh
     manual action without comment is the one option the existing codebase conventions
     explicitly rule out.

### In-flight backlog sessions when quota drops below threshold
Nothing in scope changes existing in-flight sessions. `BacklogController.Disable()`
(`session/feature_controller.go:69-86`) only stops the `SyncLoop` (which is what discovers
and dispatches *new* work) and flips `listener.SetEnabled(false)` — it does not touch any
already-running `*session.Instance`. This matches the Success Metrics wording precisely
("no new backlog work sessions are created while disabled") and the Risk Control section's
framing (recoverable failure directions). Confirm in planning that this is the intended
behavior (let in-flight sessions run to completion rather than killing them), since killing
a mid-task backlog session on quota-drop would itself burn quota (partial work, likely
requiring a redo) rather than conserve it — the requirements doc doesn't explicitly say
"don't kill in-flight sessions" but every adjacent precedent (capacity monitor, rate-limit
manager) is exclusively about *new* dispatch/*this* session's own recovery, never
force-killing a different, currently-running session. Worth stating this explicitly as a
design decision in the plan rather than leaving it implicit.

## 4. Unstated needs

- **Why, not just that.** The requirements' Success Metrics only ask for a notification on
  pause/resume; they don't explicitly ask the notification to state the *reason* (which
  threshold, what the observed value was). `capacity_monitor.go`'s message format
  (`"Capacity Warning for %s: %s (Context: %d/%d)..."`) already sets the bar for this — it
  always includes the metric and its value. A quota-pause notification that just says
  "Backlog automation paused" without the observed headroom value/threshold would be a
  regression from the existing precedent's UX quality and would make the user unable to
  judge whether the pause is reasonable — directly relevant given quota headroom likely has
  to be *inferred* (per the Feasibility Risks section) rather than read from a clean API, so
  the user will want to sanity-check the inference.
- **A manual override / kill-switch that's clearly distinguishable from the auto-gate.**
  Covered above — the existing `BacklogController` toggle already serves as "the" manual
  override per Risk Control, but its current wiring doesn't yet distinguish "off because the
  user turned it off" from "off because quota-gate turned it off," which the UI
  (`GetFeatureFlags`) has no way to surface today (it returns only a boolean `enabled`, no
  reason/source field). Frontend and proto (`sessionv1.FeatureFlag`) may need a `disabledReason`
  or similar field so a user glancing at Settings → Features can tell *why* backlog shows
  disabled, not just that it is. Check `proto/session/v1/session.proto`'s `FeatureFlag`
  message shape in planning — this is a cheap addition if made now vs. a later migration if
  deferred.
- **Historical visibility into past pause events.** Out of Scope explicitly excludes "a new
  UI dashboard for quota headroom," but the existing notification system already has a
  persisted history mechanism (`server/notifications/subscriber.go`'s coalescing-by-key
  persisted record, referenced in `backlog_notifier.go`'s comment) — since notifications are
  already durably stored, no *new* persistence is needed to give the user "look back at when
  backlog was paused and why" — that's just the existing notification history/inbox UI,
  filtered or not, as long as the pause/resume notifications use a stable, greppable
  `metadata["type"]` value (e.g. `"quota_gate_pause"` / `"quota_gate_resume"`, mirroring
  `capacity_alert"`'s pattern) so they're distinguishable from other notification types
  without any new schema. This satisfies the "historical visibility" unstated need for free
  if the metadata convention is followed — worth calling out explicitly in the plan so it's
  not treated as new scope.
- **Distinguishing "quota-gate paused" from "user paused" in the throttle path (requirement
  2) too**, not just the hard-gate (requirement 1). If background-session creation is merely
  throttled/delayed while a foreground session is active, the user likely wants to know that
  new backlog work is queued-but-delayed rather than silently not happening — a difference
  in kind from a hard pause. A single generic "backlog paused" notification type may not be
  expressive enough for both the hard-quota-gate case and the soft foreground-throttle case;
  planning should decide whether these are the same notification type with different
  messages/metadata or genuinely different notification types, since they have different
  user-facing implications (fully off vs. still working, just deprioritized).

## Key file references

| Concern | File |
|---|---|
| Manual toggle enforcement point | `session/feature_controller.go` (`BacklogController`) |
| Notification precedent to mirror | `server/services/capacity_monitor.go:250-313` (`handleTransitionTrigger`) |
| Backlog-specific notification adapter | `server/services/backlog_notifier.go` |
| Configurable-threshold precedent | `config/types.go:273-330` (`CapacityConfig`) |
| Manual-flag persistence + controller wiring | `server/services/feature_flag_service.go` |
| Boot-time enable/disable race | `server/dependencies.go:1000-1015` |
| Reconcile ticker cadence precedent | `server/dependencies.go:981-998` (60s ticker) |
| Foreground/background Instance field | `session/backlog.go:84-88` (`CategoryBacklog`), `session.Instance.Category` |
| Wrong tool for foreground detection (do not reuse) | `session/backlog.go:48-75` (`SessionRoleWork/Review/Triage`, `IsTmuxBackedSessionRole`) |
| N-consecutive-polls debounce precedent | `session/health.go:136-152` (`recoveryDebounced`) |
| Reactive per-session rate-limit signal (fallback source) | `session/detection/ratelimit/manager.go`, `integration.go` |
