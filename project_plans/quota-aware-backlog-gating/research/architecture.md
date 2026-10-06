# Architecture Research: quota-aware-backlog-gating

**Prior hotspot/architecture analysis of this area**: none found — treated as a fresh investigation.

## 1. `BacklogController.IsEnabled()` — is it settable programmatically?

`session/feature_controller.go` (`BacklogController`, ~92 lines):

```go
type BacklogController struct {
    mu       sync.Mutex
    listener *BacklogLifecycleListener
    storage  *Storage
    registry *PluginRegistry
    keyFunc  func() ([]byte, error)
    syncLoop   *SyncLoop
    syncCancel context.CancelFunc
}
```

- `IsEnabled()` reads `c.listener.enabled.Load()` — an atomic bool on the wrapped `BacklogLifecycleListener`, not a field on the controller itself.
- **It is already a general-purpose, concurrency-safe, programmatically-settable switch** — not hardwired to any single caller. `Enable(ctx)`/`Disable()` are idempotent, mutex-guarded, and callable from anywhere holding a `*BacklogController` reference. `Enable` additionally starts/stops the `SyncLoop` goroutine; `Disable` stops it. This means flipping backlog off for quota reasons doesn't just gate a check — it *stops the sync loop goroutine entirely*, which is heavier than a pure boolean but is also exactly the "hard pause" semantics requirements.md's fallback increment wants.
- **Every current call site** (`grep -rn "BacklogController"`):
  - `main.go` — likely just type reference/wiring.
  - `server/dependencies.go:1006` constructs it; `:1008` calls `Enable` once at startup gated on `cfg.GetFeatureFlag("backlog")`; `:1039` passes `backlogCtrl.IsEnabled` into `backlogSvc.SetSyncFeatureEnabledCheck`; `:1282` exposes `backlogCtrl.IsEnabled` as `RuntimeDeps.BacklogEnabledCheck`.
  - `server/services/backlog_service.go`, `server/services/backlog_github_forward_sync.go` — consumers of the `IsEnabled` check function (read-only).
  - `session/feature_controller_test.go` — tests.
- **No existing caller ever calls `Enable`/`Disable` again after startup** in the reviewed grep — the only observed mutation is the one-time startup sync from the persisted config flag. The Settings UI toggle (`server/services/feature_flags_test.go` references `SetFeatureController`) presumably calls `Enable`/`Disable` through a `services.FeatureController` interface at `server/services/session_service.go:247` (`SetFeatureController "backlog"` — one call per named flag, after backlog ctrl is built). That confirms a generic `FeatureController` interface already exists in the `services` package for exactly this purpose (interface defined in consumer package `services`, satisfied by `session.BacklogController` — the correct pattern per this repo's interface-pollution convention).

**Conclusion**: no new toggle mechanism is needed. A quota-aware gate is simply another caller of `backlogCtrl.Enable(ctx)` / `backlogCtrl.Disable()`, exactly like the Settings UI toggle is (both are just callers of `services.FeatureController`). The manual toggle and the quota-driven toggle will race harmlessly since both paths are idempotent and mutex-guarded — but see §4 for the one real design question this raises (auto-resume must not silently override a user's manual disable).

## 2. Reconcile ticker integration point — `server/dependencies.go`

Two existing tickers, both started as bare goroutines in `BuildRuntimeDeps`/`BuildCoreDepsWithOptions`:

- **60s backlog reconcile ticker** (`server/dependencies.go:984-998`): calls `backlogLifecycleListener.ReconcileStuck(ctx)` every tick, panic-recovered, comment explicitly frames it as "the only fallback for review-gate respawn, stale-item detection, and PR-pending polling" — i.e. this is the precedent ticker other periodic reconciliation logic already piggybacks on (per requirements.md's explicit reference to the `backlog-stuck-item-visibility` precedent).
- **30min tmux reaper ticker** (`:1253-1259`): unrelated cadence, reaps paused tmux sessions — wrong cadence for a quota check (too slow for the "one reconcile-ticker interval" success metric).

**Where new logic hooks in**: requirements.md's success metric ("disabled within one reconcile-ticker interval") maps directly onto the 60s ticker at line 984. The correct pattern is to **add a step inside the existing 60s ticker's closure** (a new call alongside `backlogLifecycleListener.ReconcileStuck(ctx)`, e.g. `quotaGate.Reconcile(ctx)`), not to start a second competing `time.NewTicker(60 * time.Second)` goroutine. This avoids two independent tickers drifting out of phase and issuing conflicting Enable/Disable calls in the same second, and matches the precedent this repo has already set (see the `backlog-stuck-item-visibility` project's own reuse of this ticker, cited directly in requirements.md's Non-functional Requirements).

Caution: the panic-recover wrapper at `:989-994` only wraps `ReconcileStuck` currently — a new quota-check call added to the same tick must get its own `defer recover()` (or be wrapped by widening the existing recover block) so a panic in quota logic can't silently kill the whole reconcile goroutine (which also owns stale-item detection and PR polling — an unrelated failure mode that would be a bad regression to introduce).

## 3. Rate-limit detection's event mechanism — reusable, not global

`session/detection/ratelimit/manager.go` + `integration.go`:

- `Manager` is **instantiated per session** (`NewManager(sessionID, instance)` inside `NewIntegrationWithAccessor`) — there is no process-wide singleton `Manager`. Each session's `ClaudeController` owns one `Integration` → one `Manager`.
- `Manager` has its own internal `EventBus` (`eventDetected`/`eventRecoveryStart`/`eventRecoveryDone`/`eventRecoveryFail`), but this bus is **scoped to that one Manager instance** — subscribing to it only observes one session's rate-limit lifecycle, not account-wide state. It is not the right subscription point for an account-wide aggregator.
- The actual **cross-session fan-in point already exists**, one layer up: `session/instance.go:418-420` (`Instance.onRateLimitDetected func(sessionID string, resetTime time.Time)`) is wired per-`Instance` via `Instance.SetRateLimitCallbacks(onDetected, onRecovery)` (`session/instance_controller.go:284-332`, `wireRateLimitCallbacks`). At the server layer, `server/services/session_service.go:4099` (`SessionService.wireRateLimitCallbacks`) registers **the same two callbacks on every Instance it creates**, which is exactly the account-wide fan-in a new aggregator needs — `SessionService.onRateLimitDetected` (`:4136`) is already the single choke point every session's rate-limit detection flows through server-side.
- That existing callback currently does exactly one thing: publish a per-session `events.NewNotificationEvent(...)` (WARNING/HIGH) on `s.eventBus` (the server-wide pub/sub — `server/events`, already used throughout `dependencies.go`, e.g. `eventBus.Publish(events.NewSessionUpdatedEvent(...))`).

**Conclusion**: a new account-wide aggregator should **not** re-implement detection or subscribe to each session's private `ratelimit.EventBus`. It should hook into `SessionService.onRateLimitDetected`/`onRateLimitRecovery` (or the layer that wires `SetRateLimitCallbacks`) and additionally record the event into a small shared aggregate struct (e.g. "N sessions rate-limited in the last window", "most recent rate-limit timestamp", "most recent recovery timestamp"). This reuses 100% of the existing detection machinery and requires zero changes to `session/detection/ratelimit/`, matching the Out-of-Scope constraint in requirements.md ("Changing per-session reactive rate-limit handling... is reused as a possible signal source, not replaced").

## 4. Data flow and package placement

```
Per-session detection (existing, unmodified)
  session/detection/ratelimit/{manager,detector}.go
        │  (per-session, via Manager.onDetectionCallback / onRecoveryCallback)
        ▼
Instance-level relay (existing, unmodified)
  session/instance.go: Instance.onRateLimitDetected/onRateLimitRecovery
  session/instance_controller.go: wireRateLimitCallbacks
        │  (per-instance → server layer, already fans in every session)
        ▼
Server-wide aggregator (NEW — server/services or a new small package)
  server/services/session_service.go: wireRateLimitCallbacks/onRateLimitDetected/onRateLimitRecovery
        │  record event into in-memory account-wide state
        ▼
Account-wide quota state (NEW, in-memory)
  e.g. server/services/quota_gate.go — tracks recent rate-limit events across
  all sessions (timestamps, counts), computes "headroom" signal
        │  (read on each 60s reconcile tick — §2)
        ▼
Threshold comparison + hysteresis (NEW)
  same package — compares state against config.QuotaConfig (mirrors
  CapacityConfig's WarnPct/AutoPct pattern), applies hysteresis window
        │  crosses threshold?
        ▼
BacklogController.Enable(ctx) / Disable()  (EXISTING, unmodified — §1)
        │
        ▼
Notification (EXISTING pattern, reused)
  events.NewNotificationEvent(...) published on the existing server eventBus,
  same call shape as SessionService.onRateLimitDetected (server/services/session_service.go:4146)
```

**Package placement rationale**:
- The aggregator and gate logic belong in `server/services/` (a new file, e.g. `quota_gate.go`), sibling to `capacity_monitor.go` — both are server-layer polling/aggregation logic over session state, and `capacity_monitor.go` is the closest existing precedent for "periodic check → threshold compare → alert" (even though its polling loop itself should *not* be reused directly — reuse the 60s reconcile ticker per §2, not `CapacityMonitor.Start`'s own ticker, to avoid a third competing ticker).
- Config lives in `config/types.go`, a new `QuotaConfig` struct mirroring `CapacityConfig`'s shape (`WarnPct`/`AutoPct`-equivalent fields, e.g. a single `HeadroomThresholdPct` plus a `HysteresisPct` or `HysteresisWindow time.Duration`), with a `QuotaConfigOrDefault()` following the same defaulting pattern as `CapacityConfigOrDefault()`.
- `BacklogController` itself needs **no changes** — it's already the generic, reusable toggle identified in §1.

**In-memory vs. persisted**: requirements.md's Risk Control section explicitly frames both failure directions (false pause, false non-pause) as low-risk and recoverable via the existing manual toggle, with "no additional rollback machinery needed." This directly supports **in-memory-only aggregate state** — no new ent schema, no persisted table. A process restart simply resets the rolling window, which is an acceptable cold-start (matches how `capacity_monitor.go`'s per-session state is already in-memory, not persisted). This also sidesteps the `ent-schema-generation.md` `--feature sql/upsert` workflow entirely, keeping the change backend-only and low-blast-radius, consistent with the "Medium (1-2 weeks)" appetite.

**Auto-toggle vs. manual-toggle interaction (the one real design gap)**: because `BacklogController.Enable`/`Disable` is a single shared boolean with no "reason" tag, a quota-driven `Disable()` and a user's manual `Disable()` via Settings are indistinguishable at read time — and a quota-driven auto-`Enable()` on recovery would incorrectly resume backlog if the user had manually disabled it for unrelated reasons. **This needs a design decision in planning**, not left implicit: either (a) track the disable *reason* alongside the boolean (a small enum/flag: `manual` vs `quota`) so auto-resume only re-enables state it itself disabled, or (b) accept the simpler fallback-increment framing from requirements.md (quota-driven pause only, resume requires the same threshold-recovery path — never resume something that started manually-disabled) — this is a planning-phase decision, not an architecture blocker, but the plan must state which one is chosen since the naive "just call Enable/Disable" approach silently breaks the manual override guarantee requirements.md's Risk Control section relies on.

## 5. Requirement 2 (foreground-session throttle) — existing "role" concept

- `session/backlog.go` defines `SessionRoleWork`, `SessionRoleTriage`, `SessionRoleReview` and `IsTmuxBackedSessionRole(role string) bool` — these identify **backlog-spawned** session roles specifically. There is **no existing "foreground" or "human-driven" role constant** — human-driven sessions (created via the Omnibar) simply have no `ItemSession`/backlog `Role` at all (confirmed via `rateLimitLinkedItemID` in `session_service.go:4117-4131`, which treats "no linked backlog item" as the normal/expected case for non-backlog sessions).
- **Implication for requirement 2**: "foreground session active" should be operationalized as "any live session whose `ItemSession` lookup returns not-found / whose Role is empty" (i.e., NOT `SessionRoleWork`/`Review`/`Triage`), rather than inventing a new role. This is consistent with requirements.md's Rabbit Holes note to "check for a `SessionRole`/foreground concept already in the codebase... rather than inventing a new one" — the existing concept is the *absence* of a backlog role, not a positive "foreground" tag. This distinction is a planning-phase detail (exact query mechanism), not an architecture blocker, but the plan should note it explicitly since it's easy to reach for a new enum value instead of the inverse-of-existing-set check.

## 6. Event-Command-Policy table — skipped, with reasoning

Per the task framing, this feature is a single automated policy (one signal source → one threshold comparison → one toggle → one notification), not a multi-actor business process with competing commands/policies from different actors. An EventStorming-style Event-Command-Policy table would mostly restate the linear data flow in §4 with no additional actors, competing events, or reconciliation-of-conflicting-writes complexity beyond the manual/auto toggle interaction already called out in §4 as its own explicit open design point. Skipped as disproportionate to the change's actual shape — see `.claude/CLAUDE.md`'s Proportionality guidance.

## Summary of Integration Points (for planning phase)

| Stage | Location | Status |
|---|---|---|
| Per-session rate-limit detection | `session/detection/ratelimit/manager.go` | existing, unmodified |
| Server-wide fan-in of detection events | `server/services/session_service.go:4099` `wireRateLimitCallbacks` / `:4136` `onRateLimitDetected` | existing, **extend** (add aggregator recording) |
| Account-wide quota state + threshold + hysteresis | new `server/services/quota_gate.go` | new |
| Config | new `QuotaConfig` in `config/types.go`, mirroring `CapacityConfig` | new |
| Periodic re-evaluation | existing 60s ticker, `server/dependencies.go:984-998` | existing, **extend** (add a call, own panic-recover) |
| Enforcement toggle | `session.BacklogController.Enable`/`Disable` via `services.FeatureController` | existing, unmodified — reused as-is |
| Notification | `events.NewNotificationEvent(...)` via server `eventBus`, same shape as `session_service.go:4146` | existing pattern, reused |
| Foreground-session detection (req 2) | inverse of `session.IsTmuxBackedSessionRole`/backlog `Role` presence | existing concept, reused (no new role) |
