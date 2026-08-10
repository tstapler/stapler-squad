# Implementation Plan: quota-aware-backlog-gating

**Feature**: Automatically pause/resume backlog automation (`BacklogController`) based on an
inferred account-wide Claude Code quota-headroom signal, plus a soft foreground-session throttle,
with visible pause/resume notifications and a Settings UI status line.
**Date**: 2026-08-10
**Status**: Ready for implementation
**ADRs**: [ADR-001: Quota-Headroom Signal Source — Combined Percentage-Heuristic + Reactive-Override](../decisions/ADR-001-quota-signal-source.md)

---

## Step 0.5 — Creative Pass (Alternatives Explored)

Three high-level designs were compared before committing (full detail in ADR-001):

- **A. Pure percentage heuristic** (bucket `session/tokens.TokenStore` into a rolling 5h window,
  compare to a configured assumed budget). *Strength*: proactive — can warn/pause before any
  session actually gets rate-limited, and directly satisfies the requirements doc's literal ask
  for a configurable percentage threshold. *Weakness*: the "budget" it's a percentage of is not
  published by Anthropic and must be guessed/configured by the user — a wrong guess produces
  persistent false positives or false negatives with no ground truth to self-correct against.
- **B. Pure reactive aggregation** (promote `session/detection/ratelimit`'s per-session detection
  events to an account-wide "any session recently rate-limited" binary signal — requirements.md's
  own "Fallback Increment"). *Strength*: zero new heuristic risk, ground truth not inference,
  lowest implementation risk — the choice three of six research files converge on independently.
  *Weakness*: reactive by construction — cannot prevent the *first* hit in a quota window, only
  compounding hits after one is already observed.
- **C. Combination** — A as a configurable, hysteresis-gated soft/proactive tier; B as an
  unconditional, hysteresis-free hard override that always wins. *Strength*: A's miscalibration
  risk degrades gracefully to B's ground-truth safety net (never "no protection"); B's
  detection-gap risk (a missed regex wording variant) is still covered by A's independent signal.
  *Weakness*: two signal computations to build, test, and explain in one UI string instead of one.

**Chosen: C.** See ADR-001 for full rationale. The rejected alternatives are also recorded in the
Pattern Decisions table below where they map onto specific component choices.

---

## Domain Glossary

| Term | Definition | Notes |
|------|-----------|-------|
| `QuotaGate` | The server-layer type owning quota-headroom state, hysteresis, provenance, and the periodic `Reconcile` decision that drives `BacklogController.Enable`/`Disable`. Constructed with the real `*events.EventBus` directly (same pattern as `CapacityMonitor`), not the narrower `session.Notifier` interface — see the Notifications row of Pattern Decisions. | New: `server/services/quota_gate.go` |
| `QuotaConfig` | Config struct holding all tunable thresholds for `QuotaGate`, mirroring `config.CapacityConfig`'s shape. | New: `config/types.go` |
| `QuotaConfigOrDefault` | Defaulting method on `QuotaConfig`, mirrors `CapacityConfigOrDefault`. | New: `config/types.go` |
| `HeadroomEstimate` | Value object: `{WindowStart, WindowEnd time.Time; TokensUsed, AssumedBudget int64; PctRemaining float64; Valid bool}` — the soft signal's output. `Valid=false` when `AssumedWindowTokenBudget<=0` (uncalibrated). | New: `server/services/quota_headroom.go` |
| `computeHeadroom` | Pure function: `[]*tokens.ParseResult, budget int64, isLoading bool, now time.Time -> HeadroomEstimate`. Buckets `TurnStats` into the trailing 5h window. `isLoading==true` (from `tokens.TokenStoreReader.IsLoading()`, already part of the narrow interface `QuotaGate` depends on — `session/tokens/types.go:57-63`) forces `Valid=false`, same no-op path as an uncalibrated budget, so a cold-cache read (boot, or any tick during a restart-adjacent background walk) is honest about not having data instead of silently reading "healthy." | New: `server/services/quota_headroom.go` |
| `RateLimitAggregate` | Struct tracking `{LastEventAt time.Time; RecentEventCount int}` — the hard signal's rolling state, fed by every session's rate-limit detection. | New: `server/services/quota_gate.go` |
| `hasRecentRateLimitEvent` | Predicate: true if `RateLimitAggregate.LastEventAt` is within `RateLimitWindowMinutes` of now. | New: `server/services/quota_gate.go` |
| `gateState` | Internal, mutex-guarded struct: `{pausedByQuota bool; lastSetEnabled *bool; manualOverrideAt time.Time; consecutiveBelow, consecutiveAbove int; lastPauseNotifyAt, lastResumeNotifyAt time.Time}`. Single writer: `QuotaGate.Reconcile`. | New: `server/services/quota_gate.go` |
| `pausedByQuota` | Provenance sentinel: true only when `QuotaGate` itself most recently called `Disable()`. Gates auto-resume — `Enable()` is only ever called by `QuotaGate` when this is true. | Field on `gateState` |
| `lastSetEnabled` | The enabled-state `QuotaGate` last wrote to `BacklogController`. Compared against `IsEnabled()` each tick to detect an external (manual) write since the last tick — the provenance-detection mechanism. | Field on `gateState` |
| `manualOverrideAt` | Timestamp of the most recently detected external (manual) enable/disable, used to bypass the notification cooldown for the very next auto-transition and to word that notification differently. | Field on `gateState` |
| `Reconcile` | `QuotaGate`'s single per-tick method: evaluates both signals, applies hysteresis, updates provenance, calls `Enable`/`Disable`, fires notifications. Called from exactly one place: the existing 60s reconcile ticker (plus once synchronously at boot). | Method on `QuotaGate` |
| `foregroundSessionActive` | Predicate: `snap.Category != session.CategoryBacklog && snap.Status == session.Active` for at least one instance, where `snap := inst.Snapshot()` — never a direct field read, since this runs on the reconcile-ticker goroutine (cross-goroutine read; see `session/instance.go:387-389`'s documented contract and `capacity_monitor.go:149`'s precedent). Reused, not invented — per `architecture.md` §5. | New helper in `server/services/quota_gate.go` |
| `foregroundThrottleUntil` | Sliding-window `time.Time` on `QuotaGate`, pushed forward by `ForegroundThrottleDelaySeconds` every tick a foreground session is observed active. | Field on `QuotaGate` |
| `ShouldThrottleForeground` | `QuotaGate` method: `time.Now().Before(foregroundThrottleUntil)`. Consulted (not `Disable`/`Enable`) by the composed `SyncFeatureEnabledCheck`. | Method on `QuotaGate` |
| `quota_gate_paused` / `quota_gate_resumed` | Distinct `metadata["type"]` notification values, mirroring `capacity_alert`'s pattern but kept separate so the UI can distinguish this feature's events. | String constants, `server/services/quota_gate.go` |
| `"backlog-quota-gate"` | Stable synthetic notifier key (passed as `itemID` to `session.Notifier.Notify`) so repeated pause/resume events coalesce instead of spamming, mirroring `backlog_notifier.go`'s per-item key discipline. | Constant, `server/services/quota_gate.go` |
| `status_detail` | New optional string field on the `FeatureFlag` proto message, rendered as a second line under the existing Settings → Feature Flags "backlog" row. | `proto/session/v1/session.proto` |
| `StatusDetail` | `QuotaGate` method returning the human-readable current-state string (or `""`) consumed by `GetFeatureFlags`. | Method on `QuotaGate` |

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| `QuotaGate` overall structure | Transaction-Script-style single `Reconcile` method over one small state struct | PoEAA (Fowler) | Domain Model with separate rich `HeadroomSignal`/`RateLimitSignal` objects each owning transition behavior | `architecture.md` §6 explicitly frames this as one linear automated policy, not a multi-actor domain — a Domain Model here is unjustified complexity for a single read-evaluate-act loop |
| Combining the soft + hard signals | Plain function composition inside `Reconcile` (two evaluator calls, explicit `if hard { ... } else if soft { ... }`) | Type-driven design / YAGNI | `SignalSource` interface with pluggable/registered implementations | Exactly two fixed, compile-time-known sources exist and neither is expected to be swapped at runtime — an interface here is the "speculative interface" smell named in `.claude/rules/interface-pollution-checklist.md` item 1 |
| Hard-pause / soft-pause enforcement point | Reuse existing `services.FeatureController` interface via `BacklogController.Enable`/`Disable` | Existing repo convention (already the pattern the Settings UI toggle uses) | New `QuotaFeatureController` wrapper type implementing its own enable/disable | Constraints explicitly forbid "a second, parallel gate"; `architecture.md` §1 confirms `BacklogController` is already the generic, concurrency-safe, programmatically-settable switch — no new toggle mechanism needed |
| Foreground-throttle enforcement point | Decorator-style composition of the existing `SyncFeatureEnabledCheck func() bool` passed to `BacklogService.SetSyncFeatureEnabledCheck` | GoF Decorator (functional form) | New `BacklogController.SetForegroundThrottle(bool)` method, or a second `Disable()` reason | Keeps the one existing consulted checker as the sole enforcement seam; avoids widening `BacklogController`'s public surface and avoids conflating "quota critical" (heavy: stops `SyncLoop`, sets `pausedByQuota`) with "human is actively working" (light: just skip this tick's dispatch) |
| Manual-vs-quota provenance | Single `pausedByQuota bool` + "compare current state against last self-write" detection, not two independent booleans | Type-driven design (illegal states unrepresentable) | Two unlinked booleans (`manuallyDisabled`, `quotaDisabled`) | Two independent booleans admit an impossible/ambiguous combination (both true, or both false while actually disabled); deriving "did someone else change this since I last wrote it" from the single existing source of truth (`BacklogController.IsEnabled()`) can't drift out of sync with reality the way duplicated state can |
| Hysteresis | Schmitt-trigger: asymmetric N-consecutive-ticks (pause=2, resume=3) *plus* a percentage margin (`ResumeMarginPct`) | Precedent: `session/health.go:136-152`'s `recoveryDebounced` | Single-crossing threshold check | `pitfalls.md` §1 explicitly documents that a margin alone still flaps if usage oscillates by more than the margin, and a tick-count alone still flaps sitting exactly on the line — the combination is the named mitigation for both failure modes |
| Config shape | Typed `QuotaConfig` value object + `QuotaConfigOrDefault()`, registered in `Config`/`LoadConfig`/`SaveConfig` | PoEAA Value Object; existing repo convention (`CapacityConfig`) | Ad hoc `map[string]interface{}` config, or adding fields directly onto `CapacityConfig` | Repo convention already establishes this exact shape; folding into `CapacityConfig` would conflate two genuinely different resource axes (per-session token/context budget vs. account-wide session quota — `requirements.md`'s own explicit distinction) |
| Rate-limit hard-signal wiring | Extend the existing fan-in point (`SessionService.wireRateLimitCallbacks`/`onRateLimitDetected`, `server/services/session_service.go:4099-4155`) | Observer (GoF) — already-established pub/sub | New standalone listener independently subscribing to every session's private `ratelimit.Manager.EventBus` | `architecture.md` §3 confirms the existing per-instance fan-in is already account-wide in reach; subscribing to N per-session buses independently duplicates existing wiring and reintroduces the "N independent goroutines writing shared state" race class `pitfalls.md` §2 warns against |
| Notifications | Publish directly via the injected `*events.EventBus` (`events.NewNotificationEvent(...)`), mirroring `CapacityMonitor`'s own wiring, with a fixed sentinel key `"backlog-quota-gate"` as the event's `sessionID` | Adapter (GoF) — existing `events.NewNotificationEvent` call shape, `server/services/capacity_monitor.go:282-312` | (a) `session.Notifier`/`EventBusNotifier` adapter (`server/services/backlog_notifier.go`); (b) a new notification event type/pipeline | (a) rejected: `session.Notifier.Notify(itemID, title, message, notificationType, priority)` has no metadata parameter, but a distinct `metadata["type"]` (`quota_gate_paused`/`quota_gate_resumed`) is required for the frontend to distinguish this feature's events from `capacity_alert` and per-session rate-limit notifications — `CapacityMonitor`'s direct-`EventBus` pattern supports this and `QuotaGate` is architecturally its sibling anyway (§4 of `architecture.md`). (b) rejected: `events.NewNotificationEvent` already supports everything needed; the stable-key coalescing discipline (`"backlog-quota-gate"` as `sessionID`) is copied directly from `backlog_notifier.go`'s own documented itemID-collision fix without needing that file's adapter type itself |

---

## Migration Plan

*(Omitted — no schema or persisted-data changes. Per `architecture.md` §4 and the item's own
Package Placement directive, all `QuotaGate` state is in-memory-only; `requirements.md`'s Risk
Control section explicitly frames both failure directions as low-risk/recoverable via the existing
manual toggle, which is what licenses skipping persistence and the `ent`
`--feature sql/upsert` workflow entirely.)*

## Observability Plan

- **Logs**: structured `log.Info`/`log.Warn` on every gate transition — `"QuotaGate: pausing
  backlog"` / `"QuotaGate: resuming backlog"` / `"QuotaGate: foreground throttle active"` /
  `"QuotaGate: foreground throttle cleared"` — each with `reason`, the relevant `HeadroomEstimate`
  fields, and `RateLimitAggregate.LastEventAt` when applicable. Matches the item's own
  Observability Requirements section (log line required for "backlog paused for quota," "backlog
  resumed," and "background session throttled" — the last is log-only, no notification, see
  Non-Goals).
- **Metrics**: none new — out of scope per requirements.md ("no new metrics/alerting
  infrastructure required beyond" structured logs + notifications).
- **Alerts**: the pause/resume notifications *are* the alerting surface (existing
  `NotificationToast`/`NotificationPanel` pipeline) — no separate alerting system.

## Risk Control

- **Feature flag**: `config.QuotaConfig.Enabled` (default `false`) gates the entire gate —
  when `false`, `QuotaGate.Reconcile` is a no-op (never calls `Enable`/`Disable`, never notifies),
  leaving today's purely-manual `BacklogController` toggle behavior completely unchanged. Ship
  code with this default off; flip to `true` in `config.json` only after the smoke test (Phase 4)
  passes on this single-user instance.
- **Rollback procedure**: set `Quota.Enabled=false` in `config.json` (or omit the field —
  zero-value default) and restart, or use the existing manual Settings → Feature Flags toggle at
  any time regardless of `QuotaGate`'s state — per `architecture.md` §1, both paths are idempotent
  callers of the same `BacklogController.Enable`/`Disable`, so there is no new rollback machinery
  to build.
- **Staged rollout**: (1) ship with `Quota.Enabled=false` and `AssumedWindowTokenBudget=0` — pure
  no-op, existing behavior untouched; (2) flip `Quota.Enabled=true` with
  `AssumedWindowTokenBudget` still `0` — this activates only the hard reactive-override half
  (ADR-001's "Fallback Increment" mode), the lowest-risk configuration; (3) once real usage
  patterns are observed, set a real `AssumedWindowTokenBudget` to activate the proactive
  percentage tier. Each stage is a config-only change, no redeploy required beyond stage 1.

## Unresolved Questions

- **`AssumedWindowTokenBudget` calibration value.** No default is safe to guess (Anthropic
  publishes no cap). Shipping with `0` (soft-signal disabled) sidesteps this at ship time, but the
  user will need to pick a real number from observed usage before requirement 2's full proactive
  behavior is live — this is an explicit post-ship follow-up, not a blocker.
- **Do Hidden (headless review) sessions count as "foreground"?** `foregroundSessionActive`
  filters on `Category != CategoryBacklog && Status == Active`. Headless review sessions spawned
  by backlog itself should already have `Category == CategoryBacklog` (excluding them correctly),
  but this hasn't been independently confirmed for every backlog-spawned session type (triage,
  one-shot PR-fix, etc.) — verify with a quick grep/test during Story 2.3.1 rather than assuming.
- **Default `QuotaConfig.Enabled = false` is a product decision made on the user's behalf** in
  this plan (see Risk Control) — flag for confirmation before merge; flipping the default to
  `true` is a one-line change if the user prefers it live immediately.

## Dependency Visualization

```
Phase 1 — Foundation
  Epic 1.1 QuotaConfig ─────────────────┐
  Epic 1.2 HeadroomEstimate (soft) ─────┼──► Phase 2 Epic 2.1 QuotaGate core
  Epic 1.3 RateLimitAggregate (hard) ───┘         │      ▲
       │  Story 1.3.2 (Task 1.3.2b's call site)   │      │
       └────────────────────────────────────────────────┘
          needs Task 2.1.1c's locking wrapper to exist
          first — cross-phase dependency, both land in
          the same PR; not a strict phase-1-before-phase-2
          execution order for this one task pair.
                                                   ▼
Phase 2 — Gate Logic & Provenance
  Epic 2.1 QuotaGate core (Reconcile, hysteresis, provenance,
            Task 2.1.1c locking recordRateLimitEvent wrapper)
       │
       ├──► Epic 2.2 Wiring: boot sequence + 60s ticker
       │
       └──► Epic 2.3 Foreground throttle (independent of 2.2's internals,
                       but needs QuotaGate to exist)
                                                   │
                                                   ▼
Phase 3 — Notifications & UI Surface
  Epic 3.1 Pause/resume notifications  (needs 2.1's Reconcile transitions)
  Epic 3.2 status_detail proto + UI    (needs 2.1's StatusDetail(), independent of 3.1)
                                                   │
                                                   ▼
Phase 4 — Verification
  Epic 4.1 Non-goal safeguard test (needs 2.1, 2.2)
  Epic 4.2 Live smoke test (needs everything above)
```

---

## Phase 1: Foundation — Config & Signal Computation

### Epic 1.1: `QuotaConfig`
**Goal**: A typed, defaulted config struct mirroring `CapacityConfig`'s shape, wired into
`config.Config`'s load/save paths, giving every threshold in this feature a configurable,
non-hardcoded home.

#### Story 1.1.1: Add `config.QuotaConfig` and its defaulting method
**As a** self-hosted operator, **I want** every quota-gate threshold to be configurable via
`config.json`, **so that** I can tune pause/resume behavior without a code change or redeploy.
**Acceptance Criteria**:
- A zero-value `QuotaConfig{}` becomes a fully-defaulted, safe struct after
  `QuotaConfigOrDefault()`.
  - *Given* `cfg := config.QuotaConfig{}`, *When* `cfg.QuotaConfigOrDefault()` is called, *Then*
    the result has `Enabled == false`, `PauseBelowHeadroomPct == 20.0`, `ResumeMarginPct == 15.0`,
    `ConsecutiveTicksToPause == 2`, `ConsecutiveTicksToResume == 3`,
    `AssumedWindowTokenBudget == 0`, `RateLimitWindowMinutes == 30`,
    `ManualOverrideGraceMinutes == 10`, `ForegroundThrottleDelaySeconds == 300`.
- Explicit non-zero values are preserved, not overwritten by defaults.
  - *Given* `cfg := config.QuotaConfig{PauseBelowHeadroomPct: 35.0}`, *When*
    `cfg.QuotaConfigOrDefault()` is called, *Then* the result's `PauseBelowHeadroomPct == 35.0`
    (unchanged) while every other field is defaulted as above.
**Files**: `config/types.go`, `config/config.go`, `config/types_test.go` (new or extended)

##### Task 1.1.1a: Define `QuotaConfig` struct (~3 min)
- Add `type QuotaConfig struct` to `config/types.go`, adjacent to `CapacityConfig`, with fields:
  `Enabled bool`, `PauseBelowHeadroomPct float64`, `ResumeMarginPct float64`,
  `ConsecutiveTicksToPause int`, `ConsecutiveTicksToResume int`,
  `AssumedWindowTokenBudget int64`, `RateLimitWindowMinutes int`,
  `ManualOverrideGraceMinutes int`, `ForegroundThrottleDelaySeconds int` — each with a doc comment
  stating its default and, for `AssumedWindowTokenBudget`, an explicit note that `0` means
  "soft/percentage signal disabled — Anthropic publishes no real budget, this must be
  operator-supplied."
- Files: `config/types.go`

##### Task 1.1.1b: Add `QuotaConfigOrDefault()` method (~3 min)
- Mirror `CapacityConfigOrDefault`'s zero-value-backfill pattern exactly (value receiver, copy-out,
  `if out.Field <= 0 { out.Field = default }` per field; `Enabled` stays `false` by default per
  Risk Control — do not default it to `true`).
- Files: `config/types.go`

##### Task 1.1.1c: Wire `Quota` field into `Config` + `LoadConfig`/`SaveConfig` defaulting (~4 min)
- Add `Quota QuotaConfig \`json:"quota,omitempty"\`` to the `Config` struct next to the existing
  `Capacity CapacityConfig` field (`config/config.go:338-339`).
- Add `cfg.Quota = QuotaConfig{}.QuotaConfigOrDefault()` next to the existing
  `cfg.Capacity = CapacityConfig{}.CapacityConfigOrDefault()` call (`config/config.go:463`, the
  "new config file" default path).
- Add `cfg.Quota = cfg.Quota.QuotaConfigOrDefault()` next to
  `cfg.Capacity = cfg.Capacity.CapacityConfigOrDefault()` (`config/config.go:916`, the
  "loaded existing config, backfill zero fields" path).
- Files: `config/config.go`

##### Task 1.1.1d: Unit test `QuotaConfigOrDefault` (~3 min)
- Table-driven test covering: zero-value input → all defaults; partially-set input → only unset
  fields defaulted (both Given-When-Then examples above as cases).
- Files: `config/types_test.go`

---

### Epic 1.2: `HeadroomEstimate` (soft/proactive signal)
**Goal**: A pure, independently testable computation turning `TokenStore.GetAll()` into a
5-hour-rolling-window percentage-headroom estimate — the ADR-001 "soft signal."

#### Story 1.2.1: `computeHeadroom` over `TokenStore` data
**As** `QuotaGate`, **I want** a pure function that turns raw token-usage data into a
headroom percentage, **so that** the hysteresis/threshold logic in `Reconcile` never has to know
about `ParseResult`/`TurnStats` internals.
**Acceptance Criteria**:
- Tokens outside the trailing 5h window are excluded from the sum.
  - *Given* two `*tokens.ParseResult` entries, one with a `TurnStats{Timestamp: now.Add(-6*time.Hour),
    Input: 1000}` and one with `TurnStats{Timestamp: now.Add(-1*time.Hour), Input: 500}`, *When*
    `computeHeadroom(results, 1000, false, now)` is called, *Then* `TokensUsed == 500` (only the
    in-window turn counted) and `PctRemaining == 50.0`.
- `AssumedWindowTokenBudget <= 0` produces an explicitly invalid, inert estimate.
  - *Given* any non-empty `results` slice, *When* `computeHeadroom(results, 0, false, now)` is
    called, *Then* `Valid == false` and `PctRemaining == 100.0` (never triggers a pause downstream).
- `isLoading == true` forces an invalid estimate regardless of budget or data, so a cold/partially-
  populated `TokenStore` cache (boot, or mid-restart background walk) never reads as artificially
  healthy.
  - *Given* a non-empty `results` slice and `assumedBudget > 0` (a fully calibrated config), *When*
    `computeHeadroom(results, assumedBudget, true, now)` is called (i.e. `isLoading == true`),
    *Then* `Valid == false` and `PctRemaining == 100.0` — identical no-op shape to the uncalibrated-
    budget case, even though the budget itself is valid.
**Files**: `server/services/quota_headroom.go` (new), `server/services/quota_headroom_test.go` (new)

##### Task 1.2.1a: Define `HeadroomEstimate` + `computeHeadroom` (~5 min)
- New file `server/services/quota_headroom.go`. Define `HeadroomEstimate` struct per Domain
  Glossary. Implement `computeHeadroom(results []*tokens.ParseResult, assumedBudget int64,
  isLoading bool, now time.Time) HeadroomEstimate`: if `isLoading`, return
  `HeadroomEstimate{Valid: false, PctRemaining: 100.0}` immediately — do not bucket or sum anything,
  since the cache may be only partially populated by the background walk
  (`session/tokens/store.go:65-76`'s `TokenStore.Start` launches its initial walk asynchronously and
  returns immediately, so `GetAll()` can return an incomplete result set for some time after
  `Start`). Otherwise: window = `[now.Add(-5*time.Hour), now]`; sum
  `Input+Output+CacheCreation+CacheRead` across every `TurnStats` in every result's
  `TurnTimeline` whose `Timestamp` falls in the window; `PctRemaining = 100 * (1 -
  float64(used)/float64(assumedBudget))`, clamped to `[0, 100]`; `Valid = assumedBudget > 0`.
- Files: `server/services/quota_headroom.go`

##### Task 1.2.1b: Unit tests for `computeHeadroom` (~5 min)
- Cases: empty `results`; all turns outside window; mixed in/out-of-window turns (both GWT
  examples above); `assumedBudget <= 0` sentinel; usage exceeding budget clamps `PctRemaining` to
  `0`, not negative; `isLoading == true` with an otherwise-fully-calibrated, non-empty `results` +
  positive `assumedBudget` still yields `Valid == false` (the new GWT case above) — this is the
  case most likely to regress silently since it's the one place a "healthy-looking" input still
  must produce an invalid estimate.
- Files: `server/services/quota_headroom_test.go`

---

### Epic 1.3: `RateLimitAggregate` (hard/reactive override signal)
**Goal**: Promote the existing per-session reactive rate-limit detection to an account-wide
rolling aggregate, per ADR-001's hard-override half, reusing the existing fan-in point rather than
subscribing to per-session event buses independently.

#### Story 1.3.1: `RateLimitAggregate` type
**As** `QuotaGate`, **I want** a small rolling-window record of "did any session hit a rate limit
recently," **so that** the hard-override check in `Reconcile` is O(1) and needs no per-tick
re-scan of session state.
**Acceptance Criteria**:
- A recorded event is "recent" only within the configured window.
  - *Given* `agg := RateLimitAggregate{}` and `agg.recordRateLimitEvent(now.Add(-10*time.Minute))`,
    *When* `agg.hasRecentRateLimitEvent(now, 30*time.Minute)` is called, *Then* it returns `true`.
  - *Given* the same `agg`, *When* `agg.hasRecentRateLimitEvent(now, 5*time.Minute)` is called
    (a narrower window than the 10-minute-old event), *Then* it returns `false`.
**Files**: `server/services/quota_gate.go` (new), `server/services/quota_gate_test.go` (new)

##### Task 1.3.1a: Define `RateLimitAggregate` (~4 min)
- New file `server/services/quota_gate.go` (this becomes the primary file for the whole feature —
  subsequent epics add to it). Define `RateLimitAggregate struct { LastEventAt time.Time;
  RecentEventCount int }` with methods `recordRateLimitEvent(at time.Time)` and
  `hasRecentRateLimitEvent(now time.Time, window time.Duration) bool`. No locking here — this type
  is always accessed under `QuotaGate.mu`, via the locking wrapper method `(*QuotaGate)
  recordRateLimitEvent` defined in Task 2.1.1c, never called directly by anything outside this
  file. Note in a comment that `rateLimits RateLimitAggregate` is a *named* field on `QuotaGate`
  (not embedded), so Go does not promote this method to `*QuotaGate` — callers outside
  `quota_gate.go` must go through the Task 2.1.1c wrapper, they cannot call
  `quotaGate.rateLimits.recordRateLimitEvent(...)` or an auto-promoted `quotaGate.recordRateLimitEvent(...)`
  without it.
- Files: `server/services/quota_gate.go`

##### Task 1.3.1b: Unit tests for `RateLimitAggregate` (~4 min)
- Both Given-When-Then cases above, plus: no event recorded yet → `hasRecentRateLimitEvent` always
  `false`.
- Files: `server/services/quota_gate_test.go`

#### Story 1.3.2: Wire rate-limit detection into the aggregator
**As** `QuotaGate`, **I want** every session's existing rate-limit detection to feed my aggregate
automatically, **so that** no new PTY-output scanning or per-session subscription is built.

**Cross-epic dependency**: Task 1.3.2b's call site (`s.quotaGate.recordRateLimitEvent(...)`) targets
the locking wrapper method defined in Task 2.1.1c, which is part of Epic 2.1 (Phase 2) even though
this story is in Epic 1.3 (Phase 1). Both epics land in the same PR (this is a phase-numbering
convenience, not a strict execution order), so implement/land Task 2.1.1c before or alongside Task
1.3.2b — do not implement Task 1.3.2b calling a not-yet-defined method. See the Dependency
Visualization diagram for the explicit cross-phase arrow.
**Acceptance Criteria**:
- A rate-limit detection on any tracked session updates the shared aggregate.
  - *Given* a `*SessionService` with `quotaGate` wired via `SetQuotaGate`, *When*
    `onRateLimitDetected` fires for any `*session.Instance` (as it already does today via
    `wireRateLimitCallbacks`), *Then* the locking wrapper `quotaGate.recordRateLimitEvent(time.Now())`
    (Task 2.1.1c — not `RateLimitAggregate`'s own unlocked method) is called in addition to the
    existing notification publish — the existing per-session notification behavior is unchanged.
**Files**: `server/services/session_service.go`, `server/dependencies.go`

##### Task 1.3.2a: Add `SessionService.quotaGate` field + `SetQuotaGate` setter (~3 min)
- Add `quotaGate *QuotaGate` field to `SessionService` struct and a `SetQuotaGate(g *QuotaGate)`
  setter, mirroring the existing setter-injection pattern already used for
  `SetSyncFeatureEnabledCheck` and similar late-wired dependencies (avoids a constructor-ordering
  problem, since `QuotaGate` needs `backlogCtrl`, which doesn't exist yet inside
  `NewSessionService`).
- Files: `server/services/session_service.go`

##### Task 1.3.2b: Call the locking `recordRateLimitEvent` wrapper from `onRateLimitDetected` (~4 min)
- In `onRateLimitDetected` (`server/services/session_service.go:~4131`), after the existing
  notification-publish logic and nil-guard, add: `if s.quotaGate != nil {
  s.quotaGate.recordRateLimitEvent(time.Now()) }`. This calls the mutex-guarded `(*QuotaGate)
  recordRateLimitEvent` wrapper defined in Task 2.1.1c — **never** call
  `s.quotaGate.rateLimits.recordRateLimitEvent(...)` directly, which would bypass the lock and race
  with every other session's concurrent `onRateLimitDetected` invocation plus `Reconcile`'s own
  reads. Do not gate this call on `inst.Hidden` (unlike the notification above it) — a rate limit
  hit by a hidden/headless session still consumes real account quota and must count toward the
  hard override.
- Files: `server/services/session_service.go`

##### Task 1.3.2c: Wire `sessionService.SetQuotaGate(quotaGate)` at construction time (~2 min)
- In `server/dependencies.go`, immediately after `quotaGate` is constructed (Epic 2.2), call
  `sessionService.SetQuotaGate(quotaGate)`.
- Files: `server/dependencies.go`

---

## Phase 2: Gate Logic & Provenance

### Epic 2.1: `QuotaGate` core — `Reconcile`, hysteresis, provenance
**Goal**: The single decision-making loop: read both signals, apply hysteresis, detect manual
overrides, drive `BacklogController`, without ever becoming a second independent writer racing the
manual toggle.

#### Story 2.1.1: `QuotaGate` struct + constructor
**As a** maintainer, **I want** `QuotaGate`'s dependencies expressed as narrow, consumer-defined
interfaces, **so that** it's unit-testable without a real `TokenStore`/`BacklogController`.
**Acceptance Criteria**:
- `NewQuotaGate` accepts a config accessor, `tokens.TokenStoreReader`, `InstancePoller`, a
  `services.FeatureController`, and a real `*events.EventBus`, and returns a usable, zero-state
  `*QuotaGate`.
  - *Given* fakes/test doubles for the first four dependencies and a real (in-process, no
    subscribers) `*events.EventBus` for the fifth, *When* `NewQuotaGate(cfgFn, fakeStore,
    fakePoller, fakeCtrl, eventBus)` is called, *Then* the returned `*QuotaGate` has `gateState{}`'s
    zero values (`pausedByQuota == false`, `lastSetEnabled == nil`) and calling `IsPausedByQuota()`
    returns `false`.
**Files**: `server/services/quota_gate.go`

##### Task 2.1.1a: Define `QuotaGate` struct + `NewQuotaGate` (~5 min)
- Fields: `mu sync.Mutex`, `cfgFn func() config.QuotaConfig`, `tokenStore tokens.TokenStoreReader`,
  `poller InstancePoller` (reuse the interface already defined in `capacity_monitor.go` —
  same package, no duplicate), `backlogCtrl FeatureController` (reuse the existing interface, same
  package), `eventBus *events.EventBus` (direct injection, same pattern `CapacityMonitor` uses —
  **not** `session.Notifier`, since `Notify`'s signature has no metadata parameter and this
  feature needs a distinct `metadata["type"]` per transition; see Pattern Decisions), `rateLimits
  RateLimitAggregate`, `state gateState`, `foregroundThrottleUntil time.Time`. Constructor takes
  all five injected dependencies as parameters:
  `NewQuotaGate(cfgFn func() config.QuotaConfig, tokenStore tokens.TokenStoreReader, poller
  InstancePoller, backlogCtrl FeatureController, eventBus *events.EventBus) *QuotaGate`.
- Files: `server/services/quota_gate.go`

##### Task 2.1.1b: Define `gateState` struct (~3 min)
- Per Domain Glossary: `pausedByQuota bool`, `lastSetEnabled *bool`, `manualOverrideAt time.Time`,
  `consecutiveBelow, consecutiveAbove int`, `lastPauseNotifyAt, lastResumeNotifyAt time.Time`.
- Files: `server/services/quota_gate.go`

##### Task 2.1.1c: Define the locking `(*QuotaGate) recordRateLimitEvent` wrapper (~2 min)
- `rateLimits RateLimitAggregate` (Task 2.1.1a) is a *named*, non-embedded field on `QuotaGate`, so
  Go does not promote `RateLimitAggregate.recordRateLimitEvent` to `*QuotaGate` — and that method is
  explicitly unlocked (Task 1.3.1a), while it's invoked from `SessionService.onRateLimitDetected`
  (Task 1.3.2b), a callback that fires independently per session and can run concurrently across
  sessions on separate goroutines. Add:
  ```go
  func (g *QuotaGate) recordRateLimitEvent(at time.Time) {
      g.mu.Lock()
      defer g.mu.Unlock()
      g.rateLimits.recordRateLimitEvent(at)
  }
  ```
  This is the *only* sanctioned way to feed the hard signal from outside `quota_gate.go` — Task
  1.3.2b's call site must target this method, not `RateLimitAggregate`'s own unlocked method.
- Files: `server/services/quota_gate.go`

#### Story 2.1.2: `Reconcile` — signal evaluation, hysteresis, provenance
**As** the system, **I want** one method that reads both signals and decides on a single,
mutex-serialized enable/disable action per tick, **so that** the manual toggle and the auto-gate
never race each other into an inconsistent state.
**Acceptance Criteria**:
- The hard signal overrides immediately, with no hysteresis delay.
  - *Given* a `QuotaGate` with `backlogCtrl.IsEnabled() == true` and
    `rateLimits.recordRateLimitEvent(now.Add(-1*time.Minute))` already called, *When*
    `Reconcile(ctx)` runs a single time, *Then* `backlogCtrl.Disable()` is called exactly once and
    `state.pausedByQuota == true`.
- The soft signal requires `ConsecutiveTicksToPause` consecutive below-threshold ticks before
  acting.
  - *Given* `QuotaConfig.ConsecutiveTicksToPause == 2`, `PauseBelowHeadroomPct == 20.0`, no recent
    rate-limit event, and `computeHeadroom` returning `PctRemaining == 10.0` (below threshold) on
    every call, *When* `Reconcile(ctx)` is called once, *Then* `backlogCtrl.Disable()` is **not**
    yet called (`state.consecutiveBelow == 1`); *When* `Reconcile(ctx)` is called a second
    consecutive time with the same low headroom, *Then* `backlogCtrl.Disable()` **is** called
    (`state.consecutiveBelow` resets to `0`, `state.pausedByQuota == true`).
- Resume requires headroom above `PauseBelowHeadroomPct + ResumeMarginPct` for
  `ConsecutiveTicksToResume` ticks.
  - *Given* `QuotaGate` currently paused-by-quota (`state.pausedByQuota == true`,
    `backlogCtrl.IsEnabled() == false`), `PauseBelowHeadroomPct == 20.0`, `ResumeMarginPct ==
    15.0` (resume threshold 35.0), `ConsecutiveTicksToResume == 3`, and `computeHeadroom`
    returning `PctRemaining == 40.0` on each call, *When* `Reconcile(ctx)` is called three
    consecutive times, *Then* `backlogCtrl.Enable()` is called exactly once, on the third call.
- The hard signal blocks resume even on ticks after the one that triggered the pause (not just the
  triggering tick itself) — this was a BLOCKER in adversarial review: a guard of
  `hard && backlogCtrl.IsEnabled()` only disables on the *first* tick after detection (when
  `IsEnabled()` is still `true`); on the very next tick `IsEnabled()` is already `false`, so that
  guard evaluates `false` and control falls through to the soft-threshold branch, which can then
  call `Enable()` while the hard/reactive signal is still within its window — contradicting
  ADR-001's "hard signal always wins, unconditional override" design.
  - *Given* `pausedByQuota == true`, the hard signal still active (a rate-limit event within
    `RateLimitWindowMinutes`), and `computeHeadroom` returning healthy headroom (above
    `PauseBelowHeadroomPct + ResumeMarginPct`) for `ConsecutiveTicksToResume` consecutive ticks,
    *When* `Reconcile` runs those ticks, *Then* `Enable()` is never called while `hard` remains
    `true` — the soft-resume branch must additionally require `!hard`, not just
    `!backlogCtrl.IsEnabled() && pausedByQuota`.
**Files**: `server/services/quota_gate.go`

##### Task 2.1.2a: Implement hard-override check (~5 min)
- In `Reconcile(ctx context.Context)`: acquire `mu`; compute
  `hard := g.rateLimits.hasRecentRateLimitEvent(time.Now(), time.Duration(cfg.RateLimitWindowMinutes)*time.Minute)`.
  **`hard` is computed once per tick and consulted by both this task's disable branch and Task
  2.1.2b's resume-eligibility check below — it is not scoped or reset just to this task.** If
  `hard && backlogCtrl.IsEnabled()`, disable immediately (bypasses the consecutive-tick counters
  entirely — reset both counters to `0` too, since the hard override supersedes whatever the soft
  signal was tracking) and return early after notifying (Epic 3.1 wires the actual notify call;
  this task can call a not-yet-implemented `g.notifyPaused(...)` stub or leave a `// TODO(3.1)`
  if sequencing tasks strictly — prefer implementing the full call here since `quota_gate.go` is
  one file being built incrementally across epics in the same PR). If `hard` is `true` but
  `backlogCtrl.IsEnabled()` is already `false` (already paused, from this tick or an earlier one),
  do **not** return early here — fall through to Task 2.1.2b, whose resume branch must itself check
  `!hard` (see below) so the already-paused-and-still-hard case is correctly kept paused rather
  than silently skipped.
- Files: `server/services/quota_gate.go`

##### Task 2.1.2b: Implement soft-threshold consecutive-tick logic (~5 min)
- Compute `estimate := computeHeadroom(g.tokenStore.GetAll(), cfg.AssumedWindowTokenBudget,
  g.tokenStore.IsLoading(), time.Now())` (the `IsLoading()` argument closes the boot-time /
  restart-adjacent gap flagged in adversarial review — see the Domain Glossary's `computeHeadroom`
  entry: a cold or partially-populated cache now yields `Valid == false` instead of reading as
  artificially healthy). Skip the pause branch entirely if `!estimate.Valid` (uncalibrated or
  loading — per ADR-001, treat as "no soft signal," do not touch the counters). If
  `backlogCtrl.IsEnabled()` and `estimate.Valid` and `estimate.PctRemaining <
  cfg.PauseBelowHeadroomPct`: increment `consecutiveBelow`, reset `consecutiveAbove`; if
  `consecutiveBelow >= cfg.ConsecutiveTicksToPause`, disable and reset `consecutiveBelow`.
  **Resume branch — must gate on `!hard` unconditionally (the BLOCKER fix; `hard` is the same
  value Task 2.1.2a computed this tick, not re-derived):** if `!hard && !backlogCtrl.IsEnabled() &&
  state.pausedByQuota && estimate.Valid && estimate.PctRemaining >=
  cfg.PauseBelowHeadroomPct+cfg.ResumeMarginPct`: increment `consecutiveAbove`, reset
  `consecutiveBelow`; if `consecutiveAbove >= cfg.ConsecutiveTicksToResume`, enable and reset
  `consecutiveAbove`. If `hard` is `true` while `!backlogCtrl.IsEnabled() && state.pausedByQuota`
  (still paused, hard signal still active), do not increment `consecutiveAbove` at all this tick —
  treat it the same as a below-threshold tick for hysteresis purposes, i.e. reset both counters to
  `0` (a hard-active tick must not let a partial resume streak survive into the tick where `hard`
  finally clears, since that would let a stale streak fire `Enable()` on the very next good tick
  instead of requiring a fresh `ConsecutiveTicksToResume` run). Any other tick that doesn't match
  either direction also resets both counters to `0` (a single good/bad tick shouldn't half-count
  toward a much later run).
- Files: `server/services/quota_gate.go`

##### Task 2.1.2c: Implement provenance detection (~5 min)
- At the top of `Reconcile`, before evaluating signals: read `current :=
  g.backlogCtrl.IsEnabled()`. If `g.state.lastSetEnabled != nil && *g.state.lastSetEnabled !=
  current`: an external actor changed state since the last tick — set
  `g.state.manualOverrideAt = time.Now()`; if `current == true`, clear `g.state.pausedByQuota =
  false` (a manual re-enable always clears quota-provenance, per the design decision in
  requirements' tension item — the *next* threshold re-evaluation, not this detection step, decides
  whether to re-pause). Log this transition explicitly (`log.Info("QuotaGate: detected external
  change to backlog enabled state", "enabled", current)`).
- Files: `server/services/quota_gate.go`

##### Task 2.1.2d: Implement auto-resume guard + `lastSetEnabled` bookkeeping (~3 min)
- Every place `Reconcile` calls `backlogCtrl.Enable()` or `.Disable()`, immediately set
  `g.state.lastSetEnabled = &v` (the value just written) so the next tick's provenance check
  has an accurate baseline. `Enable()` is only ever reached from the resume branch in 2.1.2b, which
  is itself already gated on `!hard && state.pausedByQuota == true` — no separate guard needed
  beyond those two existing conditions, but add a defensive comment stating both invariants
  explicitly: auto-resume must never fire when `pausedByQuota == false` (backlog is off because a
  human turned it off), **and** must never fire while `hard == true` (a rate-limit event is still
  within its window — the fix for the hard/soft interaction BLOCKER in Task 2.1.2b).
- Files: `server/services/quota_gate.go`

#### Story 2.1.3: `Reconcile` unit tests
**As a** maintainer, **I want** the hysteresis and provenance logic covered by fast, dependency-
free unit tests, **so that** future edits to `Reconcile` can't silently reintroduce flapping or a
manual-override stomp.
**Acceptance Criteria**:
- All four Given-When-Then examples from Story 2.1.2 pass as table-driven test cases (hard-override
  disable, soft-threshold consecutive-tick pause, soft-threshold consecutive-tick resume, and the
  hard-blocks-resume interaction case).
- A manual disable is never auto-resumed.
  - *Given* `backlogCtrl.IsEnabled() == false` with `state.pausedByQuota == false` (simulating a
    manual disable — never touched by `QuotaGate`), *When* `Reconcile(ctx)` is called with
    `computeHeadroom` returning healthy headroom on every call, for any number of ticks, *Then*
    `backlogCtrl.Enable()` is never called.
- A manual re-enable while quota is still low re-pauses, bypassing the notification cooldown.
  - *Given* `QuotaGate` currently paused-by-quota, then a fake external actor flips
    `backlogCtrl`'s state to enabled (simulating the manual UI path) without going through
    `QuotaGate`, and headroom is still below `PauseBelowHeadroomPct`, *When* `Reconcile(ctx)` is
    called enough times to reach `ConsecutiveTicksToPause`, *Then* `notifyPaused` is called with a
    message distinguishing "re-paused after a manual override" from a fresh pause, and the call
    happens even if it's within `lastPauseNotifyAt`'s normal 5-minute cooldown window (because
    `time.Since(manualOverrideAt) < ManualOverrideGraceMinutes`).
- The hard signal blocks resume across multiple ticks, not just the triggering tick (BLOCKER
  regression guard — see Story 2.1.2's fourth GWT case).
  - *Given* `pausedByQuota == true`, the hard signal still active every tick (a rate-limit event
    within `RateLimitWindowMinutes` on every call to `hasRecentRateLimitEvent`), and `computeHeadroom`
    returning healthy headroom (above `PauseBelowHeadroomPct + ResumeMarginPct`) for
    `ConsecutiveTicksToResume` consecutive ticks, *When* `Reconcile(ctx)` is called that many times,
    *Then* `backlogCtrl.Enable()` is never called across any of those ticks; *When* the hard signal
    subsequently clears and healthy headroom persists for a fresh `ConsecutiveTicksToResume` ticks,
    *Then* `backlogCtrl.Enable()` is finally called.
**Files**: `server/services/quota_gate_test.go`

##### Task 2.1.3a: Table-driven tests for hard-override + soft-threshold pause/resume (~5 min)
- Cover all four GWT examples from Story 2.1.2 using fakes for `InstancePoller`,
  `FeatureController`, `tokens.TokenStoreReader`, `session.Notifier`.
- Files: `server/services/quota_gate_test.go`

##### Task 2.1.3b: Table-driven tests for provenance + manual-override behavior (~5 min)
- Cover both Story 2.1.3 manual-override GWT cases above (manual disable never auto-resumed;
  manual re-enable while still low re-pauses and bypasses cooldown).
- Files: `server/services/quota_gate_test.go`

##### Task 2.1.3c: Regression test — hard signal blocks resume across multiple ticks (~4 min)
- Implement the "hard signal blocks resume across multiple ticks" GWT case above. This is the
  direct regression guard for the adversarial-review BLOCKER on Task 2.1.2a/2.1.2b's hard/soft
  interaction: assert specifically that `Enable()` is not called on *any* tick while `hard` remains
  `true`, not just that it's eventually called once `hard` clears — a test that only checks the
  final state after the hard signal clears would pass even with the old, buggy
  `hard && backlogCtrl.IsEnabled()` guard, since the bug only manifests as a premature `Enable()`
  call on an intermediate tick while `hard` is still `true`.
- Files: `server/services/quota_gate_test.go`

---

### Epic 2.2: Wiring — boot sequence + reconcile ticker
**Goal**: `QuotaGate` participates in the exact same startup and periodic-reconcile cadence every
other backlog-adjacent mechanism in this codebase already uses — no new ticker, no boot-time gap
where a genuinely-critical quota state is trusted-away by a stale persisted flag.

#### Story 2.2.1: Move only the minimal `TokenStore` construction ahead of the backlog boot-enable block
**As** `QuotaGate`, **I want** `TokenStore` to exist before `BacklogController`'s boot-time
`Enable()` decision, **so that** the very first post-restart reconcile can use real (JSONL-file-
backed, restart-durable) headroom data instead of skipping the soft check entirely.

**Scope correction (was a Blocker in adversarial review)**: the real `if homeDir, homeDirErr :=
os.UserHomeDir(); homeDirErr == nil { ... }` block spans `server/dependencies.go:1152-1225`
(verified by brace-matching the file directly), not `:1150-1170` as an earlier draft of this task
stated. That block includes `backlogSvc.SetTokenStore(tokenStore, pricing)` (line 1174),
`sessionService.SetTokenStoreReader(tokenStore)` (line 1173), the `InsightsService` construction,
and the entire ArtifactExtractor wiring (lines 1180-1222) — all of which depend on `backlogSvc`
(declared at line 1017) and `insightsSvc`/`historyLinker` state that either doesn't exist yet, or
would be moved to reference something declared later, at the proposed new insertion point
(immediately before `syncRegistry := session.NewDefaultRegistry()` at line 1001, which is *before*
`backlogSvc`'s own declaration at line 1017). Moving the *whole* block as originally specced
references `backlogSvc` before its declaration — a compile error. **Only the two lines `QuotaGate`
actually needs early — `tokenStore := tokens.NewTokenStore(historyDir)` and
`tokenStore.Start(context.Background())`, plus the `historyDir` value and the
`historyLinker.RegisterFileCallback(tokenStore.OnHistoryFileChanged)` call that must run before
`Start` — move.** `backlogSvc.SetTokenStore(...)`, `sessionService.SetTokenStoreReader(...)`, the
`InsightsService` construction, and the ArtifactExtractor wiring all stay at their current later
position in the file (lines 1172-1222), now reading `tokenStore`/`historyDir` from the
already-constructed outer-scope variables instead of declaring them inline.
**Acceptance Criteria**:
- `tokens.NewTokenStore(...)` and `tokenStore.Start(...)` are relocated and assigned to variables
  usable by the code that builds `backlogCtrl`/`quotaGate`, before that code runs; every other
  statement in the original `homeDir` block (pricing, `InsightsService`, `backlogSvc.SetTokenStore`,
  ArtifactExtractor) stays where it is today and continues to compile by referencing the
  now-outer-scope `tokenStore`/`historyDir`.
  - *Given* `server/dependencies.go` after this change, *When* reading the function top-to-bottom,
    *Then* the `tokenStore := tokens.NewTokenStore(historyDir)` and `tokenStore.Start(...)` lines
    appear strictly before the `backlogCtrl := session.NewBacklogController(...)` line (today both
    are ~150 lines *after* `backlogCtrl`'s boot-enable block, inside the 1152-1225 block), while
    `backlogSvc.SetTokenStore(tokenStore, pricing)` and the ArtifactExtractor wiring remain at their
    current position, unmoved.
**Files**: `server/dependencies.go`

##### Task 2.2.1a: Move only `tokenStore` construction + `Start` out of the `homeDir` block (~6 min)
- The full `if homeDir, homeDirErr := os.UserHomeDir(); homeDirErr == nil { ... }` block is at
  `server/dependencies.go:1152-1225` (corrected from an earlier, inaccurate `:1150-1170` citation —
  verify against current line numbers before editing, since other work may have shifted them
  slightly). From inside that block, extract only:
  ```go
  historyDir := filepath.Join(homeDir, ".claude", "projects")
  tokenStore := tokens.NewTokenStore(historyDir)
  historyLinker.RegisterFileCallback(tokenStore.OnHistoryFileChanged)
  tokenStore.Start(context.Background())
  ```
  and relocate exactly these four lines (still guarded by the same `if homeDir, homeDirErr :=
  os.UserHomeDir(); homeDirErr == nil` check, restructured as needed so `tokenStore`/`historyDir`
  end up in a scope visible both to this new early position and to the original `homeDir` block
  later in the function — e.g. hoist `tokenStore`/`historyDir`/`homeDir` to named outer-scope
  variables declared just before `syncRegistry := session.NewDefaultRegistry()` at line 1001, with
  the later block's `if homeDir, homeDirErr := os.UserHomeDir(); homeDirErr == nil` check reduced to
  operating on the already-resolved `homeDir`/`tokenStore` instead of re-deriving them) to
  immediately before that `syncRegistry`/`backlogCtrl := session.NewBacklogController(...)` block.
  **Leave everything else** — pricing table loading, `associator`, `insightsSvc =
  services.NewInsightsService(...)`, `sessionService.SetTokenStoreReader(tokenStore)`,
  `backlogSvc.SetTokenStore(tokenStore, pricing)`, the `sessionSummaryGenerator.SetTokenStore` call,
  and the entire ArtifactExtractor wiring — at their current position (now reading the
  already-constructed `tokenStore`/`historyDir` rather than declaring them).
- Files: `server/dependencies.go`

##### Task 2.2.1b: Verify no ordering regressions from the move (~4 min)
- Grep every use of `tokenStore`/`historyDir`/`homeDir`/`insightsSvc`/`pricing`/`associator`
  between the block's old and new positions; confirm nothing between the new (earlier) position and
  the old (later) position — including `backlogSvc.SetTokenStore` and the ArtifactExtractor wiring,
  which stay in place per Task 2.2.1a — was relying on `tokenStore`/`historyDir` being locally
  scoped rather than hoisted. Run `make build` to catch any compile-order issue immediately; this
  is the primary verification step since a mis-scoped extraction here is a compile error, not a
  silent regression.
- Files: `server/dependencies.go`

#### Story 2.2.2: Construct `QuotaGate`, replace naked boot `Enable()` with a synchronous `Reconcile`
**As** the system, **I want** the very first quota check to happen before backlog is allowed to
start on a fresh boot, **so that** a restart during a genuinely-critical quota window doesn't
silently resume backlog for up to 60 seconds.
**Acceptance Criteria**:
- Boot no longer unconditionally trusts the persisted flag.
  - *Given* `cfg.GetFeatureFlag("backlog") == true` and a hard-override condition already true at
    process start (a rate-limit event recorded — realistically this can't happen pre-boot since
    `rateLimits` starts empty every process start, but the *code path* must still route through
    `Reconcile` rather than a naked `Enable()` call, so the soft/hard checks are exercised
    uniformly with every other tick, not specially bypassed), *When* the server boots, *Then*
    `backlogCtrl.Enable()` is only reached via `quotaGate.Reconcile(ctx)`'s own decision logic, not
    a separate unconditional call.
**Files**: `server/dependencies.go`

##### Task 2.2.2a: Construct `quotaGate` and call boot `Reconcile` (~5 min)
- Immediately after `backlogCtrl := session.NewBacklogController(...)`, construct:
  `quotaGate := services.NewQuotaGate(func() config.QuotaConfig { return cfg.Quota.QuotaConfigOrDefault() },
  tokenStore, sessionService, backlogCtrl, eventBus)` (passing the same `eventBus` already used
  throughout this function, e.g. by `backlogSvc.SetEventBus(eventBus)`). Replace the
  existing
  ```go
  if cfg.GetFeatureFlag("backlog") {
      if err := backlogCtrl.Enable(context.Background()); err != nil { ... } else { ... }
  } else { log.Info(...) }
  ```
  block with: if `cfg.GetFeatureFlag("backlog")`, call `backlogCtrl.Enable(context.Background())`
  exactly as today (this is still the one true source of "did the user want this on" — unchanged
  from today's behavior), **then** call `quotaGate.Reconcile(context.Background())` once,
  synchronously, right after — so if quota state is already bad, it's disabled again within the
  same boot sequence rather than waiting up to 60s for the first ticker fire.
- Files: `server/dependencies.go`

##### Task 2.2.2b: Wire `sessionService.SetQuotaGate(quotaGate)` (~2 min)
- Immediately after `quotaGate` construction (same location as Task 2.2.2a), add
  `sessionService.SetQuotaGate(quotaGate)` (completes Task 1.3.2c's dependency).
- Files: `server/dependencies.go`

#### Story 2.2.3: Hook `Reconcile` into the existing 60s reconcile ticker
**As** the system, **I want** `QuotaGate.Reconcile` to run on the same cadence as
`ReconcileStuck`, **so that** the "disabled within one reconcile-ticker interval" success metric
is met without a second, independently-drifting ticker.
**Acceptance Criteria**:
- The 60s ticker calls both `ReconcileStuck` and `quotaGate.Reconcile`, each independently
  panic-recovered.
  - *Given* the reconcile ticker fires, *When* `quotaGate.Reconcile(ctx)` panics (e.g. a nil-
    pointer bug), *Then* `backlogLifecycleListener.ReconcileStuck(ctx)` still runs on this and
    every subsequent tick (the panic doesn't kill the shared ticker goroutine).
**Files**: `server/dependencies.go`

##### Task 2.2.3a: Move the ticker's `go func(){...}()` launch after `quotaGate` construction (~4 min)
- The existing 60s ticker (`server/dependencies.go:984-998`) currently starts before
  `backlogCtrl`/`quotaGate` exist in source order, so it cannot close over `quotaGate` today.
  Relocate the entire `go func() { ticker := time.NewTicker(60 * time.Second) ... }()` block to
  immediately after Task 2.2.2b's `SetQuotaGate` call, still referencing
  `backlogLifecycleListener` (already constructed earlier, unaffected by this move) exactly as
  before.
- Files: `server/dependencies.go`

##### Task 2.2.3b: Add the `quotaGate.Reconcile` call with its own `recover()` (~4 min)
- Inside the ticker's `for range ticker.C` loop, add a second `func() { defer func() { if r :=
  recover(); r != nil { log.Error("quota gate reconcile ticker recovered from panic", "recover",
  r) } }(); quotaGate.Reconcile(ctx) }()` call, sibling to (not nested inside) the existing
  `ReconcileStuck` closure, so a panic in one never prevents the other from running this tick or
  future ticks.
- Files: `server/dependencies.go`

---

### Epic 2.3: Foreground-session throttle (requirement 2)
**Goal**: New backlog dispatch is delayed while a human is actively driving a non-backlog session,
without ever calling `Disable()` (a full stop is disproportionate to "someone has a terminal
open") — enforced through the existing `SyncFeatureEnabledCheck` seam, composed rather than
replaced.

#### Story 2.3.1: `foregroundSessionActive` predicate + sliding throttle window
**As** `QuotaGate`, **I want** to detect "a human is actively working" using the existing
`Category`/`Status` fields on `Instance`, **so that** requirement 2 needs no new session role or
primitive.
**Acceptance Criteria**:
- Any active, non-backlog session counts as foreground.
  - *Given* `poller.GetInstances()` returns one `*session.Instance` with `Category == ""` (a
    normal Omnibar-created session, no backlog role) and `Status == session.Active`, *When*
    `foregroundSessionActive(instances)` is called, *Then* it returns `true`.
- Backlog-owned sessions never count as foreground, no matter how many are active.
  - *Given* `poller.GetInstances()` returns three `*session.Instance`s, all with `Category ==
    session.CategoryBacklog` and `Status == session.Active`, *When*
    `foregroundSessionActive(instances)` is called, *Then* it returns `false`.
- The throttle window slides forward while foreground activity continues, and expires after
  `ForegroundThrottleDelaySeconds` of no foreground activity.
  - *Given* `QuotaConfig.ForegroundThrottleDelaySeconds == 300` and `Reconcile` observes a
    foreground session active on tick N, *When* `ShouldThrottleForeground()` is checked
    immediately after tick N, *Then* it returns `true`; *When* no foreground session is observed
    on ticks N+1 through N+5 (5 more minutes of ticks at 60s cadence) and `ShouldThrottleForeground()`
    is checked after tick N+5, *Then* it returns `false`.
**Files**: `server/services/quota_gate.go`, `server/services/quota_gate_test.go`

##### Task 2.3.1a: Implement `foregroundSessionActive` (~4 min)
- `func foregroundSessionActive(instances []*session.Instance) bool`: iterate, and for each
  instance call `snap := inst.Snapshot()`, returning `true` on the first instance where
  `snap.Category != session.CategoryBacklog && snap.Status == session.Active`. **Do not** read
  `inst.Status`/`inst.Category` directly off `*session.Instance` — `Reconcile` runs on the 60s
  reconcile-ticker goroutine, a different goroutine than whatever mutates a given session's
  `Instance` (tmux control-mode callbacks, RPC handlers, the session driver), so a direct field
  read here is a genuine cross-goroutine data race. `session/instance.go:387-389` documents this
  contract explicitly ("Use ... Snapshot() for reads"), and `capacity_monitor.go:149` — this
  component's own cited precedent — already gets this right (`inst.Snapshot().Status !=
  session.Active`). Add a one-line comment on `foregroundSessionActive` recording this rationale so
  a future edit doesn't "simplify" it back to a direct field read.
- Files: `server/services/quota_gate.go`

##### Task 2.3.1b: Implement sliding `foregroundThrottleUntil` + `ShouldThrottleForeground` (~4 min)
- In `Reconcile` (or a small helper called from it), if `foregroundSessionActive(g.poller.GetInstances())`,
  set `g.foregroundThrottleUntil = time.Now().Add(time.Duration(cfg.ForegroundThrottleDelaySeconds)
  * time.Second)` under `g.mu`. Add `func (g *QuotaGate) ShouldThrottleForeground() bool` reading
  `time.Now().Before(g.foregroundThrottleUntil)` under `g.mu`. Log a transition line (once, not
  every tick — compare against previous state) when throttle becomes active/clears, per the
  Observability Plan.
- Files: `server/services/quota_gate.go`

##### Task 2.3.1c: Verify Hidden/headless backlog session types are correctly excluded (~3 min)
- Per the Unresolved Questions entry: grep every backlog session-spawning call site
  (`server/services/backlog_service_triage.go:806,2866`, `server/services/session_service.go:869`)
  to confirm every one sets `Category = session.CategoryBacklog` before or immediately after
  creation, including headless/Hidden review sessions — add a short comment in
  `foregroundSessionActive`'s doc comment recording the confirmation (or, if a gap is found, file
  it as a fast-follow rather than silently expanding this task's scope).
- Files: `server/services/quota_gate.go` (doc comment only; no source changes expected)

#### Story 2.3.2: Compose `SyncFeatureEnabledCheck` instead of adding a second gate
**As** the system, **I want** the foreground throttle to be consulted at the exact same seam
that already gates both the periodic sync loop and manual "Trigger Sync," **so that** no new
enforcement point is introduced.
**Acceptance Criteria**:
- Backlog dispatch is blocked while throttled, even though `BacklogController.IsEnabled()` is
  still `true`.
  - *Given* `backlogCtrl.IsEnabled() == true` and `quotaGate.ShouldThrottleForeground() == true`,
    *When* the composed checker function passed to `backlogSvc.SetSyncFeatureEnabledCheck` is
    invoked, *Then* it returns `false`.
  - *Given* the same state but `quotaGate.ShouldThrottleForeground() == false`, *When* the same
    checker is invoked, *Then* it returns `true` (identical to today's `backlogCtrl.IsEnabled`
    behavior).
**Files**: `server/dependencies.go`, `server/services/quota_gate_test.go`

##### Task 2.3.2a: Replace `SetSyncFeatureEnabledCheck(backlogCtrl.IsEnabled)` with a composed closure (~3 min)
- In `server/dependencies.go`, change
  `backlogSvc.SetSyncFeatureEnabledCheck(backlogCtrl.IsEnabled)` to
  `backlogSvc.SetSyncFeatureEnabledCheck(func() bool { return backlogCtrl.IsEnabled() &&
  !quotaGate.ShouldThrottleForeground() })`.
- Files: `server/dependencies.go`

##### Task 2.3.2b: Unit test the composed-checker semantics (~4 min)
- Since the composed closure itself lives in `dependencies.go` (not easily unit-testable in
  isolation), instead unit test `QuotaGate.ShouldThrottleForeground()`'s sliding-window behavior
  directly (the Story 2.3.1 GWT example), which is the only non-trivial half of the composition —
  the `&&` itself needs no test.
- Files: `server/services/quota_gate_test.go`

---

## Phase 3: Notifications & UI Surface

### Epic 3.1: Pause/resume notifications
**Goal**: Both transition directions post a visible, non-silent notification with the observed
value and reason, per the item's Constraints and `ux.md`'s message-format recommendation — closing
the gap `pitfalls.md` §4 identifies (only the *pause* half has an existing copyable precedent).

#### Story 3.1.1: Pause notification
**As** the operator, **I want** to see why backlog was paused and what value triggered it, **so
that** I can judge whether the pause is reasonable given the headroom is only ever an estimate.
**Acceptance Criteria**:
- The notification names the reason and the observed value, never just "paused."
  - *Given* a soft-threshold pause with `HeadroomEstimate{PctRemaining: 15.0}` and
    `QuotaConfig.PauseBelowHeadroomPct == 20.0`, *When* `notifyPaused` is called, *Then* the
    published message contains both `"15"` and `"20"` (the observed and threshold values), the
    `metadata["type"] == "quota_gate_paused"`, and the notifier is called with `itemID ==
    "backlog-quota-gate"`.
- Repeated pause ticks while already paused don't spam a notification every 60s.
  - *Given* `QuotaGate` already paused-by-quota with `state.lastPauseNotifyAt` set 2 minutes ago
    (inside the 5-minute cooldown) and not within `ManualOverrideGraceMinutes` of a manual
    override, *When* `Reconcile` runs another tick that would otherwise re-trigger the pause path
    (e.g. the hard signal is still active), *Then* `notifyPaused` is **not** called again this
    tick.
**Files**: `server/services/quota_gate.go`, `server/services/quota_gate_test.go`

##### Task 3.1.1a: Implement `notifyPaused` (~5 min)
- `func (g *QuotaGate) notifyPaused(reason string, estimate HeadroomEstimate)`:
  cooldown check (5 min, matching `capacity_monitor.go`'s precedent) **unless**
  `time.Since(g.state.manualOverrideAt) < time.Duration(cfg.ManualOverrideGraceMinutes)*time.Minute`
  (bypass, per `ux.md` §3's explicit "don't suppress the re-pause after a manual override"
  recommendation — and use a distinguishing message in that case, e.g. "Backlog was manually
  re-enabled at %s but quota is still critical — pausing again."). Build message per
  `ux.md`'s format: e.g. `"Backlog paused: session-quota headroom below threshold (%.0f%%
  remaining, assumed budget; threshold %.0f%%). Resumes automatically once headroom recovers."` (or
  the hard-override variant: `"Backlog paused: a session hit the usage limit within the last %d
  minutes. Resumes automatically once no recent rate-limit events are observed."`). Publish via
  `g.eventBus.Publish(events.NewNotificationEvent("backlog-quota-gate", "", uuid.New().String(),
  int32(8), int32(3), "Backlog Automation Paused", msg, map[string]string{"type":
  "quota_gate_paused", "reason": reason}))` (`8` = `NOTIFICATION_TYPE_WARNING`, `3` =
  `NOTIFICATION_PRIORITY_HIGH`, per `proto/session/v1/types.proto:795,814` — match
  `session_service.go`'s existing convention of a raw int with a trailing enum-name comment rather
  than importing `sessionv1` into this file) — mirroring `capacity_monitor.go:282-312`'s call shape,
  with `"backlog-quota-gate"` as the stable sentinel `sessionID` (per Pattern Decisions'
  Notifications row) so repeated pause events while still paused coalesce in the persisted
  notification history instead of each getting an unrelated key.
- Files: `server/services/quota_gate.go`

##### Task 3.1.1b: Unit tests for `notifyPaused` cooldown + message content (~4 min)
- Both Story 3.1.1 GWT cases, using a fake `*events.EventBus` (or a capturing publish func) in
  place of the real bus.
- Files: `server/services/quota_gate_test.go`

#### Story 3.1.2: Resume notification
**As the operator**, **I want** an explicit "backlog resumed" notification, **so that** I don't
have to infer resumption from the absence of further pause notifications.
**Acceptance Criteria**:
- The resume notification never promises a specific ETA.
  - *Given* a resume transition with `HeadroomEstimate{PctRemaining: 42.0}`, *When*
    `notifyResumed` is called, *Then* the message contains `"42"` and the phrase "automatically"
    but contains no time-of-day, countdown, or "in N minutes" phrasing, and
    `metadata["type"] == "quota_gate_resumed"`.
**Files**: `server/services/quota_gate.go`, `server/services/quota_gate_test.go`

##### Task 3.1.2a: Implement `notifyResumed` (~4 min)
- Mirror `notifyPaused`'s structure and `events.NewNotificationEvent` call shape (same
  cooldown/bypass logic against `state.lastResumeNotifyAt`), message e.g. `"Backlog automation
  resumed: session-quota headroom recovered to ~%.0f%% (threshold %.0f%%). Re-evaluated every
  reconcile cycle."`, `metadata["type"] = "quota_gate_resumed"`, `int32(12)` (`NOTIFICATION_TYPE_STATUS_CHANGE`,
  `proto/session/v1/types.proto:801`) and `int32(2)` (`NOTIFICATION_PRIORITY_MEDIUM`, `:813`) — per
  `ux.md`'s notification-type mapping, lighter-weight than the WARNING/HIGH used for pause.
- Files: `server/services/quota_gate.go`

##### Task 3.1.2b: Unit test for `notifyResumed` message content (~3 min)
- The Story 3.1.2 GWT case.
- Files: `server/services/quota_gate_test.go`

#### Story 3.1.3: Verify the event bus wiring reaches the real notification pipeline
**As** the system, **I want** confirmation that `QuotaGate`'s notifications land in the same
`NotificationToast`/`NotificationPanel` pipeline every other feature uses, **so that** pause/resume
events aren't silently dropped by a wiring mistake.
**Acceptance Criteria**:
- `NewQuotaGate`'s constructor call in `dependencies.go` passes the real `eventBus` (already done
  in Task 2.2.2a — this story is a verification checkpoint, not new wiring).
  - *Given* `server/dependencies.go` after Task 2.2.2a, *When* `NewQuotaGate(...)` is called,
    *Then* its last argument is the same `eventBus` variable `backlogSvc.SetEventBus(eventBus)`
    already uses (not a newly constructed one).
**Files**: `server/dependencies.go` (read-only verification)

##### Task 3.1.3a: Confirm `NewQuotaGate(...)` receives the shared `eventBus` (~2 min)
- Read the call site added in Task 2.2.2a and confirm the argument identity matches the GWT above;
  no code change expected unless Task 2.2.2a was implemented differently than specified.
- Files: `server/dependencies.go`

---

### Epic 3.2: `status_detail` — Settings UI surface
**Goal**: A persistent (non-toast, doesn't time out) explanation of *why* backlog is currently
off/throttled, per `ux.md`'s recommendation — one new optional proto field, rendered as one new
text line on the existing row. No new page.

#### Story 3.2.1: Add `status_detail` to the `FeatureFlag` proto message
**As a** frontend developer, **I want** an optional field the backend can populate per-flag,
**so that** the existing generic feature-flag row can show feature-specific status without a new
RPC.
**Acceptance Criteria**:
- The field is optional and additive — existing flags with no controller-provided detail keep
  working unchanged.
  - *Given* the regenerated `sessionv1.FeatureFlag` Go/TS types, *When* a `FeatureFlag` is
    constructed with `StatusDetail` unset, *Then* it serializes as an empty string (proto3
    default), and existing `GetFeatureFlags` callers for flags with no wired controller (e.g.
    `"browser-passthrough"`) are unaffected.
**Files**: `proto/session/v1/session.proto`, generated bindings

##### Task 3.2.1a: Add the field to the proto message (~2 min)
- Add `string status_detail = 4; // Optional human-readable status line (e.g. why a
  controller-backed flag is currently off). Empty when not applicable.` to the `FeatureFlag`
  message (`proto/session/v1/session.proto:2166-2173`).
- Files: `proto/session/v1/session.proto`

##### Task 3.2.1b: Regenerate bindings (~3 min)
- Run `make proto-gen`. Commit the regenerated `session/gen/session/v1/*.go` and
  `web-app/src/gen/session/v1/*_pb.ts` files alongside the proto change (per
  `instinct_alias_session.md`: `web-app/src/gen` is tracked despite `.gitignore`).
- Files: `session/gen/session/v1/session_pb.go` (or equivalent generated path), `web-app/src/gen/session/v1/session_pb.ts`

#### Story 3.2.2: Populate `status_detail` server-side
**As a** user glancing at Settings → Feature Flags, **I want** the "backlog" row's status line to
reflect `QuotaGate`'s current reasoning, **so that** I don't have to check logs to understand why
it's off.
**Acceptance Criteria**:
- `GetFeatureFlags` returns a non-empty `status_detail` only for "backlog," and only when
  `QuotaGate` has something to say.
  - *Given* `QuotaGate.pausedByQuota == true` with the most recent `HeadroomEstimate{PctRemaining:
    15.0}`, *When* `GetFeatureFlags` is called, *Then* the `"backlog"` `FeatureFlag`'s
    `StatusDetail` is non-empty and mentions the pause reason; every other flag's `StatusDetail`
    remains `""`.
  - *Given* `QuotaGate.pausedByQuota == false` and `backlogCtrl.IsEnabled() == true` (healthy,
    unthrottled), *When* `GetFeatureFlags` is called, *Then* the `"backlog"` flag's `StatusDetail`
    is `""` (no noise when everything is normal).
**Files**: `server/services/feature_flag_service.go`, `server/services/quota_gate.go`, `server/dependencies.go`

##### Task 3.2.2a: Implement `QuotaGate.StatusDetail()` (~4 min)
- `func (g *QuotaGate) StatusDetail() string`: under `g.mu`, if `state.pausedByQuota`, return a
  short reason string (reuse the same wording style as the pause notification, minus the "resumes
  automatically" boilerplate — keep it to one line); if `ShouldThrottleForeground()`, return
  "Throttled — foreground session active, dispatch resumes automatically once idle."; otherwise
  return `""`.
- Files: `server/services/quota_gate.go`

##### Task 3.2.2b: Extend `FeatureFlagService` to consult a status-detail provider (~5 min)
- Add `statusDetailProviders map[string]func() string` to `FeatureFlagService` (mirrors
  `featureControllers`'s shape), a `SetStatusDetailProvider(name string, fn func() string)` setter,
  and in `GetFeatureFlags`'s loop, after resolving `enabled`, set
  `flags[i].StatusDetail = f.statusDetailProviders[kf.name]()` if a provider is registered for
  `kf.name`, else `""`.
- Files: `server/services/feature_flag_service.go`

##### Task 3.2.2c: Wire the provider in `dependencies.go` (~2 min)
- After `quotaGate` construction, call
  `featureFlagSvc.SetStatusDetailProvider("backlog", quotaGate.StatusDetail)` (wherever
  `featureFlagSvc.SetFeatureController("backlog", backlogCtrl)` is already called, per
  `ux.md`'s reference to `server/dependencies.go:1131`).
- Files: `server/dependencies.go`

#### Story 3.2.3: Render `status_detail` in the frontend
**As a** user, **I want** to see the status reason directly on the Settings → Feature Flags page,
**so that** the information persists past the notification toast's ~12s timeout.
**Acceptance Criteria**:
- The second line only renders when `statusDetail` is non-empty.
  - *Given* the "backlog" flag's `statusDetail == "Paused: session-quota headroom below
    threshold..."`, *When* `FeaturesPage` renders, *Then* a second text line with that content
    appears under the existing description line for the "backlog" row.
  - *Given* any flag with `statusDetail == ""` (every flag today, and "backlog" itself when
    healthy), *When* `FeaturesPage` renders, *Then* no extra line/empty paragraph is rendered for
    that row (no layout shift from an empty element).
**Files**: `web-app/src/lib/contexts/FeatureFlagsContext.tsx`, `web-app/src/app/settings/features/page.tsx`

##### Task 3.2.3a: Add `statusDetail` to `FeatureFlagMeta` + fetch mapping (~3 min)
- Add `statusDetail: string` to the `FeatureFlagMeta` interface
  (`web-app/src/lib/contexts/FeatureFlagsContext.tsx:10-13`) and
  `list.push({ name: f.name, enabled: f.enabled, description: f.description, statusDetail:
  f.statusDetail })` at the existing mapping site (`:50`).
- Files: `web-app/src/lib/contexts/FeatureFlagsContext.tsx`

##### Task 3.2.3b: Render the conditional second line (~3 min)
- In `FeaturesPage` (`web-app/src/app/settings/features/page.tsx`), destructure `statusDetail`
  from the mapped item, and render `{statusDetail && <div className={flagDescription}>{statusDetail}</div>}`
  immediately below the existing `{description && ...}` block (reuse the existing
  `flagDescription` style per `ux.md`'s explicit "no new component" recommendation).
- Files: `web-app/src/app/settings/features/page.tsx`

##### Task 3.2.3c: Test the conditional render (~4 min)
- Extend or create the Jest test covering `FeaturesPage` (check for an existing
  `page.test.tsx`/`__tests__` first; add one row-level test if none exists) asserting both GWT
  cases from Story 3.2.3.
- Files: `web-app/src/app/settings/features/page.test.tsx` (new or extended)

#### Story 3.2.4: Feature registry entry
**As the** repo's registry convention, **I want** the modified `GetFeatureFlags` RPC's entry
updated, **so that** `make registry-diff` doesn't flag an unreviewed drift.
**Acceptance Criteria**:
- The existing per-feature file reflects the change.
  - *Given* `docs/registry/features/backend/feature-flag/get.json` before this change
    (`lastModified: "2026-06-21T..."`), *When* `make registry-generate` is run after this feature's
    code lands, *Then* `lastModified` updates to the current date and `make registry-diff` reports
    no unexplained net-new coverage gap.
**Files**: `docs/registry/features/backend/feature-flag/get.json`

##### Task 3.2.4a: Run `make registry-generate` and verify (~3 min)
- Run `make registry-generate`; inspect the diff on
  `docs/registry/features/backend/feature-flag/get.json` (and confirm no other per-feature file
  unexpectedly changed); commit the updated file.
- Files: `docs/registry/features/backend/feature-flag/get.json`

---

## Phase 4: Verification

### Epic 4.1: Non-goal safeguard test
**Goal**: Lock in, with a test, the explicit non-goal that quota-driven pause never touches
in-flight backlog sessions — only new dispatch.

#### Story 4.1.1: In-flight sessions are never killed by a quota pause
**As a** user with an in-flight backlog session when quota drops, **I want** that session to keep
running to completion, **so that** partial work already in progress isn't wasted by a forced kill.
**Non-Goal (explicit)**: this feature never stops, kills, or interrupts an already-running
`*session.Instance`. `BacklogController.Disable()` only stops the `SyncLoop` (new-work dispatch);
`QuotaGate` never calls anything on a live `*session.Instance` directly. This must not be
accidentally implemented later without a fresh product decision.
**Acceptance Criteria**:
- `Reconcile`'s pause path calls only `backlogCtrl.Disable()`, never anything session-instance-
  scoped.
  - *Given* a fake `FeatureController` and a fake `InstancePoller` returning one active,
    non-backlog-owned `*session.Instance`, *When* `Reconcile(ctx)` triggers a pause (via the hard
    signal), *Then* the fake `FeatureController.Disable()` is called exactly once, and the fake
    `InstancePoller`/any session-stop hook is never invoked (asserted via a call-count spy that
    would fail the test if any session-stop method were called).
**Files**: `server/services/quota_gate_test.go`

##### Task 4.1.1a: Add the non-goal safeguard test (~4 min)
- Implement the GWT above using a spy `FeatureController` fake that also embeds a "session stop"
  call-recorder unrelated to `QuotaGate`'s actual dependencies, to make the absence of any such
  call explicit and future-proof against an accidental regression that reaches into session
  control.
- Files: `server/services/quota_gate_test.go`

---

### Epic 4.2: Live smoke test
**Goal**: Satisfy `requirements.md`'s stated ship-time verification bar — since no authoritative
quota API exists to test against, verification is "the chosen detection/inference source correctly
reflects observed rate-limit events... and toggling inferred headroom below/above threshold
measurably pauses/resumes `BacklogController.IsEnabled()`."

#### Story 4.2.1: Manual verification on a second, isolated instance
**As the** implementer, **I want** to observe a real pause→resume cycle before flipping
`Quota.Enabled=true` on the live deployed instance, **so that** the first real activation isn't
also the first real test.
**Acceptance Criteria**:
- A synthetic hard-override event visibly pauses backlog, and clearing it (past the window)
  visibly resumes it, within one 60s tick each way.
  - *Given* a manually-built second instance (per `CLAUDE.md`'s documented pattern: `go build -o
    /tmp/ssq-manual-test .`, `PORT=8999 STAPLER_SQUAD_INSTANCE=quota-gate-manual-test
    /tmp/ssq-manual-test --tmux-keep-server`) with `Quota.Enabled=true` and the "backlog" flag on,
    *When* a rate-limit detection is triggered against a real or synthetic session in that
    instance, *Then* within ~60s the Settings → Feature Flags page shows "Backlog: Off" with a
    populated status-detail line, and a toast/notification appears; *When* `RateLimitWindowMinutes`
    subsequently elapses with no further events, *Then* within ~60s of that elapsing, backlog
    shows "Backlog: On" again with a resume notification.
**Files**: none (manual verification — no new source files)

##### Task 4.2.1a: Run the manual smoke test (~5 min, verification not code)
- Follow `CLAUDE.md`'s "Manual/interactive testing without touching the live deployed instance"
  pattern exactly (never `make install-service` for this). Trigger the hard-override path directly
  (simplest: call `recordRateLimitEvent` via a temporary debug hook, or genuinely exhaust a
  session's rate limit if time allows) rather than waiting on real quota exhaustion.
- Files: none

##### Task 4.2.1b: Record the smoke-test result in the PR description (~2 min)
- Per `.claude/CLAUDE.md`'s "run it, don't read it" evidence discipline — state pass/fail and any
  deviation from the expected GWT directly in the PR body, not a new doc file.
- Files: none (PR description only)
