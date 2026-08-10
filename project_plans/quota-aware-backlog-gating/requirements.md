# Requirements: quota-aware-backlog-gating

**Date**: 2026-08-10
**Type**: feature addition (backend detection/polling + gating logic + notification)
**Complexity**: 3 — new signal source plus a gate on an existing toggle, no schema-heavy UI

## Problem Statement
Stapler Squad has no proactive signal for the account/session-wide Claude Code usage quota (the 5-hour/weekly-style session limit). Three existing components each solve an adjacent but different problem:
- `session/detection/ratelimit/` (`manager.go`, `integration.go`) — detects rate-limit messages *after* they appear in one session's terminal output, then reactively pauses/resumes that single session. Per-session and reactive, not account-wide or proactive.
- `server/services/capacity_monitor.go` + `provider_limits.go` — polls per-provider/per-model API rate limits (requests/tokens remaining) and suggests model/CLI switches per session (`config.CapacityConfig`, default warn at 75% context / 90% auto-transition). This is about individual sessions' token/context budgets, not the shared account-wide session quota.
- `session/feature_controller.go`'s `BacklogController.IsEnabled()` (wired via `server/dependencies.go`'s `BacklogEnabledCheck`, consumed at `server/dependencies.go:1039` and `:1282`) — backlog automation already has a runtime enable/disable toggle, but nothing today drives it off quota state; it's a static/manual flag.

Result: backlog automation keeps spinning up new work sessions with no awareness of remaining account quota, so sessions get rate-limited mid-task, and a human's foreground session competes equally with backlog's background sessions for the same shared quota.

## Baseline
Today, quota exhaustion is only discovered reactively, per-session, once a session's terminal output already shows a rate-limit message (`session/detection/ratelimit`). There is no polled or inferred account-wide "quota remaining" signal, and `BacklogController.IsEnabled()` cannot be influenced by quota state at all — it is flipped only by whatever currently calls it (manual/static today).

## Users / Consumers
The single user of this self-hosted instance (Tyler), via backlog automation behavior and (per requirement 3) a visible notification when the system pauses/resumes backlog automation on their behalf.

## Success Metrics
- When available session-quota headroom drops below the configured threshold, backlog automation (`BacklogController.IsEnabled()`) is automatically disabled within one reconcile-ticker interval, and no new backlog work sessions are created while disabled.
- When headroom recovers above the threshold (plus hysteresis), backlog automation automatically re-enables, and a visible notification is posted for both the pause and the resume transitions (not a silent flip).
- While a human-driven (non-backlog) session is actively active, new backlog session creation is throttled/delayed rather than competing 1:1 for quota headroom.
- Verification: since Claude Code does not expose a first-class "quota remaining" API today (see Feasibility Risks), the ship-time check is that the chosen detection/inference source correctly reflects observed rate-limit events during a real session in a live smoke test, and that toggling the inferred headroom below/above threshold in that test measurably pauses/resumes `BacklogController.IsEnabled()`.

## Appetite
Medium (1–2 weeks)
*(Scope must fit the appetite. If it doesn't fit, cut scope — do not move the deadline.)*

**Fallback increment**: if a real quota-headroom source proves unavailable (see Feasibility Risks), ship only the hard pause/resume gate driven by the existing reactive `session/detection/ratelimit` signal promoted to account-wide scope (i.e., "any active session recently hit a rate limit" pauses backlog), deferring soft-throttle/priority-demotion (requirement 2) and any predictive/percentage-based threshold to a follow-up.

## Constraints
- Single-developer, self-hosted instance — no multi-tenant considerations.
- Must reuse the existing `BacklogController.IsEnabled()` toggle as the enforcement point (per the item's suggested entry point) rather than introducing a second, parallel gate.
- Must follow `.claude/rules/document-ai-decisions-in-edge-cases`-equivalent guidance already referenced in the item: pause/resume actions must post a visible notification, never act silently (mirrors existing `capacity_monitor.go` alert pattern).
- Existing `docs/registry/` feature-registry rule applies to any new RPC/config surfaced to the frontend (`.claude/rules/feature-registry.md`).

## Non-functional Requirements
- **Performance SLO**: not specified — low-traffic single-user tool; piggybacking on the existing reconcile ticker cadence (`server/dependencies.go`) is an acceptable baseline, matching the precedent set by `backlog-stuck-item-visibility`.
- **Scalability**: not applicable.
- **Security classification**: internal.
- **Data residency**: none.

## Scope
### In Scope
- A new quota-headroom signal source, scope and mechanism to be determined in research (Open Question #1) — options include: aggregating existing per-session `session/detection/ratelimit` events into an account-wide rolling state, or a new polling/detection source if one is discoverable.
- Configurable threshold(s) mirroring `CapacityConfig`'s `*WarnPct`/`*AutoPct` pattern (not hardcoded at 80%).
- Wiring the quota signal to `BacklogController.IsEnabled()` so backlog automation is disabled when headroom drops below threshold and re-enabled once it recovers (with hysteresis to avoid flapping).
- A background-session throttle: when a foreground/human-driven session is active, new backlog session creation is delayed or capped rather than treated as equal priority (requirement 2). Exact mechanism (hard pause vs. soft concurrency-cap demotion) is an open question for planning.
- Visible notifications for both the pause and resume transitions, following the existing `capacity_monitor.go` alert precedent.
- Feature registry entries for any new RPC/config surface per `.claude/rules/feature-registry.md`.

### Out of Scope
- Changing per-session reactive rate-limit handling in `session/detection/ratelimit/` (that mechanism is reused as a possible signal source, not replaced).
- Changing per-provider/per-model API rate-limit monitoring in `capacity_monitor.go`/`provider_limits.go` (different problem: per-session token/context budget, not account-wide session quota).
- Any change to `maxAutoReworkIterations` or other backlog policy knobs unrelated to quota.
- Building a new UI dashboard for quota headroom — visibility here is scoped to notifications, not a persistent browsable view (unless research finds this trivial to fold into existing capacity-alert UI).

## Rabbit Holes
- **No confirmed first-class quota API.** Claude Code's account/session-wide quota (5-hour/weekly-style limit) is not known to be exposed via any documented API today — research must confirm whether it's observable at all, or must be inferred/polled some other way, before design proceeds. Do not assume a clean polling endpoint exists.
- Hysteresis/threshold tuning to avoid flapping (rapid pause/resume cycles) needs explicit design, not just a single crossing check.
- Distinguishing "foreground session active" from "any session active" for requirement 2's throttling needs a precise, existing definition (check for a `SessionRole`/foreground concept already in the codebase — e.g. `IsTmuxBackedSessionRole` referenced in prior OOM-fix memory) rather than inventing a new one.

## Alternatives Considered
- Rely solely on the existing per-session reactive detection (`session/detection/ratelimit`) without any account-wide aggregation or threshold config: rejected as insufficient — it only reacts after a session is already rate-limited, doesn't prevent *new* backlog sessions from spinning up into an already-tight quota window, and gives no configurable threshold.
- Do nothing and let `session/feature_controller.go`'s manual toggle remain manual: rejected per the item's explicit ask for automatic pause/resume.

## Feasibility Risks
- **Primary risk**: whether "session quota remaining" is observable at all for Claude Code today. If research confirms no reliable source exists beyond reactive rate-limit-message detection, the fallback increment (hard pause off reactive signal, no percentage threshold) is the realistic scope — this must be confirmed in Phase 2 research before Phase 3 planning commits to a percentage-threshold design.
- Hysteresis and threshold values need defensible defaults; picking these without real usage data risks under- or over-throttling.

## Observability Requirements
Standard structured logging (existing `log.InfoLog`/`log.WarningLog` patterns) for every gate state transition (backlog paused for quota, backlog resumed, background session throttled), plus the user-visible notification already required by Success Metrics. No new metrics/alerting infrastructure required beyond that.

## Risk Control
Low-to-moderate risk: an incorrect gate could either (a) uselessly pause backlog when quota is actually fine (annoying but safe — recoverable by the resume logic or the existing manual toggle) or (b) fail to pause and let backlog exhaust quota mid-task (the status quo today, not a regression). Both failure directions are recoverable via the existing manual `BacklogController` toggle as a fallback override; no additional rollback machinery needed.

## Open Questions
- Where does "session quota remaining" actually come from for Claude Code specifically — is it observable at all today (only inferred reactively via `session/detection/ratelimit`), or does it need a new detection/polling source? (Deferred to Phase 2 research, per the item's own framing.)
- Should threshold(s) be single (e.g. 80%) or tiered (warn/auto, mirroring `CapacityConfig`)? Deferred to planning.
- Should background-session throttling be a hard pause of backlog session creation, or a soft priority demotion (e.g. cap concurrent backlog sessions lower) when a foreground session is active? Deferred to planning per the item's own open question.
