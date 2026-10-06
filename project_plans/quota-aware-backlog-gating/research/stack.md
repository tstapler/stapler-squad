# Research: Stack & Prior Art — quota-aware-backlog-gating

## 1. How rate-limit detection currently works (`session/detection/ratelimit/`)

Reactive, per-session, terminal-output regex matching — not a quota signal source, but the
event-emission pattern to imitate.

- **`detector.go`** — `Detector.ProcessOutput` runs a fixed set of `regexp.Regexp` patterns
  (`defaultRateLimitPatterns`, e.g. `"Usage limit reached"`, `"quota exceeded"`) against
  stripped-ANSI terminal output, then a `continuePatterns` set (e.g. `"Access resets at"`) to
  confirm and extract `resetTime` (`parseResetTime` handles both timezone-qualified and
  duration-relative formats). Only fires when `currentState` is `StateNone`/`StateWaiting` and
  outside a `cooldown` (default 30s) — [`session/detection/ratelimit/detector.go:183-210`](/home/tstapler/Programming/stapler-squad/session/detection/ratelimit/detector.go).
- **`manager.go`** — wraps `Detector` + `Scheduler` + `RecoveryHandler`, exposes an in-process
  pub/sub `EventBus` (`eventDetected`/`eventRecoveryStart`/`eventRecoveryDone`/`eventRecoveryFail`)
  plus two external callback hooks (`SetDetectionCallback`, `SetRecoveryCallback`) that
  `server/services/session_service.go` wires to `eventBus.Publish(events.NewNotificationEvent(...))`
  — this is the exact notification-plumbing pattern the new feature should reuse
  ([`session/detection/ratelimit/manager.go:333-347`](/home/tstapler/Programming/stapler-squad/session/detection/ratelimit/manager.go), consumer at
  [`server/services/session_service.go:4131-4174`](/home/tstapler/Programming/stapler-squad/server/services/session_service.go)).
- **`integration.go`** — `PTYConsumer` polls a session's scrollback buffer every 500ms (plus an
  edge-triggered `NotifyOutput()` channel) and feeds it to the manager. Entirely per-session; no
  shared/account-wide state exists anywhere in this package.

**Conclusion**: this package cannot be extended into an account-wide signal — it only sees text
that has already scrolled past in one session's pane, after the limit was already hit. It is a
reactive backstop for one active session, not a source of remaining-headroom data.

## 2. Existing polling/threshold pattern to mirror (`server/services/capacity_monitor.go` + `provider_limits.go`)

This is the config/threshold/notification shape the new quota gate should structurally copy.

- **`config.CapacityConfig`** (`config/types.go:291-334`) — `TransitionMode` (`manual`/`auto`/`notify`
  enum, `config/types.go:273-283`), `*WarnPct`/`*AutoPct` float thresholds, `PollIntervalSeconds`,
  and a `CapacityConfigOrDefault()` zero-value-defaulting method. **This is the config shape to
  copy** for a new `config.QuotaConfig` (e.g. `WarnPct`/`PauseBacklogPct`/`PollIntervalSeconds`).
- **`CapacityMonitor.Start(ctx)`** runs a `time.Ticker`-driven poll loop identical to what a quota
  monitor needs — poll, evaluate thresholds, and on crossing, `eventBus.Publish(events.NewNotificationEvent(...))`
  plus an optional auto-action (here, `sessionSwitcher.UpdateSessionProgram`; for this feature,
  `BacklogController.Disable()`/`.Enable()`).
- **`checkThresholds`** (`server/services/capacity_monitor.go:228-248`) returns a reason string
  once a %-based or absolute threshold is crossed — same shape needed for a quota-headroom check.
- **Important distinction already called out in requirements**: `CapacityMonitor` is per-session
  token/context-window budget (`ProviderLimits.ContextTokensUsed/Max`, request/token rate-limit
  headers), not the shared account-wide 5-hour/weekly Claude Code usage quota. `ProviderLimits`
  (`server/services/provider_limits.go:64-99`) has no field for "account quota remaining" — this
  confirms the requirements doc's framing that quota-headroom is a genuinely new signal, not an
  existing field to read.

## 3. Where "Claude Code session quota remaining" actually comes from — key finding

**There is no official CLI flag or API for this.** Findings, most to least authoritative:

- Anthropic has an **open GitHub issue** requesting exactly this capability — confirms it does
  not exist as of research date: [anthropics/claude-code#40395 "Add CLI command to check usage and
  remaining quota"](https://github.com/anthropics/claude-code/issues/40395).
- **`/usage`** is an *interactive slash command inside the Claude Code REPL* — it prints a
  point-in-time snapshot of plan usage broken down by skills/subagents/plugins/MCP servers. It is
  not scriptable/non-interactively invocable from outside a live session, so it cannot be polled
  by a background Go process the way `CapacityMonitor` polls an HTTP API.
- **`claude.ai/settings/usage`** — account-level usage bars in the web UI. No documented public
  API backing it as of this research.
- **`ccusage`** ([github.com/ryoppippi/ccusage](https://github.com/ryoppippi/ccusage), npm package
  `ccusage`, run via `npx ccusage`) — a third-party TypeScript CLI that parses the **same local
  JSONL transcript files Claude Code already writes** (`~/.claude/projects/**/*.jsonl`) to
  reconstruct token usage, with a dedicated **`ccusage blocks`** report that buckets usage into
  Claude's **rolling 5-hour billing windows** and reports an "active block" burn rate + JSON
  output (`ccusage blocks --json`) — see [ccusage blocks-reports guide](https://github.com/ryoppippi/ccusage/blob/main/docs/guide/blocks-reports.md)
  and [ccusage JSON output docs](https://ccusage.com/guide/json-output). This is a **read-model
  over the same raw source stapler-squad already parses** (see §4) — it requires no API key
  because it's pure local-file introspection, not a live quota API.

**Bottom line: there is no live API/CLI signal for "quota remaining."** The only viable signal is
inference from the local JSONL transcript logs Claude Code writes for every session — exactly
what `ccusage blocks` does, and exactly what stapler-squad's own `session/tokens` package already
parses for a different purpose (per-session cost/context tracking).

## 4. `session/tokens` — the raw data source is *already wired into this repo*

This is the most important finding for scoping the implementation: **stapler-squad does not need
a new detection/polling source at all.** It already has a live-updating, account-wide index of
every Claude Code session's token usage, built for `InsightsService`, that can be re-aggregated
into a "5-hour rolling window" view identical to `ccusage blocks`.

- **`session/tokens/store.go`** — `TokenStore` (constructed at
  [`server/dependencies.go:1152-1154`](/home/tstapler/Programming/stapler-squad/server/dependencies.go) as
  `tokens.NewTokenStore(filepath.Join(homeDir, ".claude", "projects"))`) walks and parses **every**
  JSONL transcript file under `~/.claude/projects/` on startup (4-worker pool, `parser.go`'s
  `bufio.Scanner` with a 10MB line buffer), keeps the cache fresh via `fsnotify` on file changes,
  and exposes:
  - `GetAll() []*ParseResult` — every session's parsed result, not just stapler-squad-tracked ones
    (this is the account-wide scope the requirements doc needs — a human's foreground `claude`
    session run outside stapler-squad still writes to the same `~/.claude/projects/` tree and
    would be visible here).
  - `Subscribe() <-chan struct{}` — push notification on any cache update, so a quota monitor
    could recompute on-change instead of only on a poll tick.
  - `GetByUUID(uuid string) *ParseResult` — already used by `CapacityMonitor.evaluateInstance`
    for per-session context tracking.
- **`session/tokens/types.go`** — `ParseResult.TurnTimeline []TurnStats`, each `TurnStats` carries
  `Timestamp time.Time` + `Input`/`Output`/`CacheCreation`/`CacheRead int64`. This per-turn
  timestamped token data is exactly what a 5-hour rolling-window aggregator needs: bucket every
  `TurnStats` across every `ParseResult` in `GetAll()` by `Timestamp`, sum tokens falling inside
  `[now-5h, now]`, and compare against a configured budget — the same computation `ccusage
  blocks` does, but reusable natively in Go with no subprocess/npx dependency.
- **`TokenStoreReader`** (`session/tokens/types.go:57-63`) is already the narrow consumer-defined
  interface (`GetAll`, `GetByUUID`, `IsLoading`, `Subscribe`, `Unsubscribe`) `CapacityMonitor`
  depends on — a new quota monitor should depend on this same interface rather than the concrete
  `*TokenStore`, consistent with this repo's interface-pollution convention (interfaces defined
  in the consumer package).

**Caveat / open gap**: `ParseResult` has no explicit "5-hour session quota" boundary field —
Claude's actual billing-window start/reset time (visible only via `/usage` or the web UI) is not
recoverable purely from token counts; the JSONL data only gives *usage volume over time*, not the
*plan's configured cap* or *exact window anchor*. `ccusage blocks` handles this by inferring
window boundaries from the first message timestamp in each observed cluster of activity (gaps
imply a new window) rather than reading a true API-provided reset time — the same heuristic
would need to be replicated here, and the actual token/message *cap* per plan tier is not
programmatically discoverable at all (Anthropic does not publish it) — only usage *volume* is
observable, so any "headroom remaining" number is necessarily a heuristic/relative estimate, not
an authoritative quota fraction. This should be treated as an explicit assumption to validate
with the user before implementation, not a solved problem.

## 5. `BacklogController.IsEnabled()` mechanism (`session/feature_controller.go`)

Simple in-memory bool, no persistence, no DB row:

- `BacklogController` wraps a `*BacklogLifecycleListener` (whose `enabled` field is an
  `atomic.Bool`, hence lock-free `IsEnabled()` at
  [`session/feature_controller.go:89-91`](/home/tstapler/Programming/stapler-squad/session/feature_controller.go)) plus a
  `*SyncLoop` that `Enable`/`Disable` start/stop via a `context.CancelFunc`.
- It implements the narrow `services.FeatureController` interface
  (`Enable(ctx) error` / `Disable() error` / `IsEnabled() bool` —
  [`server/services/session_service.go:68-72`](/home/tstapler/Programming/stapler-squad/server/services/session_service.go)), registered
  into `FeatureFlagService.featureControllers["backlog"]` via `SetFeatureController`
  (`server/services/feature_flag_service.go:83`).
- Wired at construction: `server/dependencies.go:1282` exposes `backlogCtrl.IsEnabled` as
  `RuntimeDeps.BacklogEnabledCheck func() bool`, consumed at `server/dependencies.go:1039`
  (`backlogSvc.SetSyncFeatureEnabledCheck(backlogCtrl.IsEnabled)` — gates manual "Trigger Sync"
  the same way it gates the periodic loop).
- **No persistence**: toggling is purely in-process/in-memory (an `atomic.Bool` + a cancelable
  goroutine) — there is no config file write or DB row backing "enabled." A quota-driven gate
  would call `backlogCtrl.Disable()`/`.Enable()` directly (or go through `FeatureFlagService` if
  flag-flip auditability/UI surfacing is wanted) rather than needing new persistence machinery.

## 6. Dependencies — no new third-party library required

- **No new Go dependency needed for the quota signal itself** — `session/tokens.TokenStore` (already
  imported via `github.com/fsnotify/fsnotify v1.9.0`, already in `go.mod`) supplies the raw data;
  the 5-hour-window bucketing logic is pure computation over `[]*ParseResult`, implementable
  without any new package.
  - **Explicitly do not shell out to `npx ccusage`** — it would add a Node.js runtime dependency
    to a Go binary for logic this repo can already compute natively from data it already parses;
    also inconsistent with `.claude/rules/prefer-go-git-over-subshells.md`'s general preference
    against subprocess dependencies when a native path exists.
- **Foreground/human-session detection** (scope item "Background-session throttle when a
  foreground/human session is active") has no existing signal in the codebase found by this
  research pass — `session.Instance`/`InstancePoller` (used by `CapacityMonitor`) expose
  `Status`/`Program`/`Snapshot()` per tracked session, but "foreground" here likely means a human
  actively typing in *any* Claude Code session, tracked or not (same account-wide scope problem as
  §3/§4). This needs its own research/design pass in `research/architecture.md` or the planning
  phase — flagging as an open question rather than guessing a mechanism.
- **Config**: extend `config.Config` with a new `Quota QuotaConfig` field following the exact
  `CapacityConfig` shape (`config/types.go:291-334`) — `WarnPct`, a backlog-pause threshold pct,
  `PollIntervalSeconds` (or event-driven via `TokenStore.Subscribe()` instead of polling, since the
  data source already pushes on change), and a `TransitionMode`-style enum if manual/auto/notify
  variants are wanted (reuse `config.TransitionMode` directly rather than defining a parallel type).
- **Notifications**: reuse `events.NewNotificationEvent` (`server/events/forward.go:51`) exactly as
  both `CapacityMonitor.handleTransitionTrigger` and `SessionService.onRateLimitDetected` do —
  no new notification mechanism needed.

## Summary of what to build vs. reuse

| Component | Verdict |
|---|---|
| Quota data source | **Reuse** `session/tokens.TokenStore.GetAll()` / `Subscribe()` — no new detection/polling source, no new dependency |
| 5-hour window bucketing | **New** — pure Go computation over `ParseResult.TurnTimeline`, heuristic window-boundary inference (same limitation `ccusage` has) |
| Config shape | **Copy** `config.CapacityConfig` pattern → new `config.QuotaConfig` |
| Threshold/poll loop | **Copy** `CapacityMonitor.Start`/`poll`/`evaluate` shape, or event-driven off `TokenStore.Subscribe()` |
| Gating mechanism | **Reuse** `BacklogController.Enable()`/`Disable()` directly, already a `services.FeatureController` |
| Notifications | **Reuse** `events.NewNotificationEvent` exactly as `capacity_monitor.go`/`session_service.go` do |
| Foreground/human-session detection | **Open question** — no existing signal found; needs its own research/design |
| Authoritative quota cap (vs. usage volume) | **Not obtainable** — no API/CLI exposes the plan's actual cap; only usage volume is observable, so "headroom %" is necessarily a heuristic/relative estimate |
