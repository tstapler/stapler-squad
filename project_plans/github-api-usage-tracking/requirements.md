# Requirements: github-api-usage-tracking

**Date**: 2026-08-10
**Type**: feature addition
**Complexity**: 4 — high-stakes / cross-cutting

## Problem Statement
Tyler runs stapler-squad locally and intermittently hits GitHub API rate limits, but has no visibility into what's consuming quota. Background pollers, `gh` CLI shell-outs, and on-demand RPCs all draw from the same shared GitHub token with no historical accounting, so there's no way to tell which source to throttle when it happens.

## Baseline
Today, `github/rate_limit.go`'s `DefaultRateLimiter` only logs a WARN when remaining quota drops below ~10% of a resource's limit (`rateLimitWarnPercent`), and pauses dispatch after a hard 403/429. There is no running count of requests made, no breakdown by source, and no historical record surviving a restart. Poller intervals (`session/pr_status_poller.go`, `session/worktree_pr_poller.go`) are hardcoded — tuning them down requires a code change and rebuild. `gh` CLI shell-outs (`github/client.go`'s `GetPRInfoCtx`, and dead code `IsForkRepo`) draw from the same token but are entirely invisible to `DefaultRateLimiter`.

Separately, CI's own unauthenticated-`buf-setup-action` rate-limit exposure was already fixed (added `github_token` to all 3 workflows) — that is not part of this problem; this is about local/background usage.

## Users / Consumers
Tyler, running stapler-squad locally as a systemd user service, often with many concurrent worktree-backed agent sessions — each of which may trigger GitHub API activity (PR status polling, issue import, backlog operations) sharing one GitHub token/quota.

## Success Metrics
Zero occurrences of `"github API: primary rate limit exhausted"` in logs over a 14-day trial window post-ship, verified using the new tracking data itself (not blind log-grepping). This requires the tracking data to actually exist and be trustworthy enough to attribute cause when/if a near-miss occurs.

### Known limitations of this metric (acknowledged, not silently passed over)
Two gaps in the primary metric above, flagged by Product review and accepted rather than fixed by adding a heavier measurement framework (this is a personal side project):

- **No baseline exists.** There's no data today on how often exhaustion currently happens, so "zero over 14 days" has no prior to compare against — a quiet 14 days could mean the fix worked, or could mean exhaustion was already rare before this feature shipped. Absent a baseline, this metric can only be read as "did the trial window stay clean," not "how much better is this than before."
- **The metric alone could be satisfied by the minimal fix.** Reviving `DefaultRateLimiter.Update()`/`IsLimited()` (Epic 1.2) so pollers actually pause is sufficient on its own to drive `exhaustion_events` to 0 — the visibility/attribution/tracking work the rest of this plan builds could ship broken, unused, or never be exercised during the trial, and the primary metric would still read "success."

**Secondary verification criterion**, to close that gap without inventing a full metrics framework: the 14-day trial verification must also confirm `dropped_events` stayed low and per-source attribution data was actually being recorded throughout the window — not just `exhaustion_events == 0` checked in isolation. A "clean" result should be attributable to the tracking system actually working, not merely to pollers going quiet or the feature going unused.

## Appetite
Large (3–6 weeks)
*(Scope must fit the appetite. If it doesn't fit, cut scope — do not move the deadline.)*

## Constraints
No externally imposed deadline or compliance requirement (personal project). Must not require a new external dependency to view basic status — OTel/Datadog export already exists in this app but is opt-in (`OTEL_ENABLED=true`) and should not become a hard requirement for this feature to be useful standalone.

## Non-functional Requirements
- **Performance SLO**: not specified — tracking overhead must be negligible relative to poll intervals (seconds), not a concern for sub-ms request paths.
- **Scalability**: low volume expected (single local dev instance, one shared token, on the order of hundreds to low thousands of requests/day) — no need to design for multi-tenant or high-cardinality storage.
- **Security classification**: internal/local-only. This data reflects the developer's own local GitHub usage; not sensitive beyond the token itself (already handled by `github/keychain.go`).
- **Data residency**: not applicable (local machine only).

## Scope
### In Scope
- Persisted (survives restart) historical tracking of GitHub API request volume, attributed to originating call site/poller and GitHub resource type (core/search).
- Instrument every call site identified in the prior audit:
  - `session/pr_status_poller.go`, `session/worktree_pr_poller.go` (timer-driven)
  - `github/client.go` `gh`-CLI shell-outs — all 7 real call sites: `GetPRInfoCtx`, `GetPRComments`, `GetPRDiff`, `PostPRComment`, `MergePR`, `ClosePR`, `CloneRepository` (resolved 2026-08-10 after Phase 2 research found 8 total call sites, not the 1 originally named; `IsForkRepo` is dead code, deleted rather than instrumented)
  - `session/backlog_plugin_github.go` (issue import)
  - `server/mcp/tools_backlog*.go`, `server/services/backlog_github_rpc*.go` (on-demand RPCs)
- Config-driven poll intervals for the two timer-driven pollers, defaulting to today's hardcoded values (backward compatible), adjustable without a rebuild.
- Delete dead code: `github/client.go`'s `IsForkRepo` (confirmed no real callers outside tests).
- Web UI panel (following `ApprovalAnalyticsPanel.tsx` conventions) showing current quota remaining/limit per resource, request volume over time, and breakdown by source.
- Given the Large appetite: make the existing WARN-at-10%-remaining threshold user-configurable, add historical charts with a time-range selector, and per-worktree-session attribution where a call is triggered by a specific session/worktree.

### Out of Scope
- OTel/Datadog metric export for this data (existing pipe is separate and opt-in; a future addition, not required now).
- Any further change to CI's GitHub API usage (already fixed separately).
- Slack/email/push alerting channels — UI-visible warnings only for this iteration.
- Changing GitHub token/auth strategy (`github/keychain.go`, `github/hosts.go`) — this is purely about visibility and pacing, not credentials.

## Rabbit Holes
- **Per-worktree-session attribution**: requires plumbing a caller identity through call sites that don't currently carry one (e.g. `gh` CLI shell-outs happen with no session context). Phase 3 planning must explicitly decide the minimum viable attribution granularity (call-site name may be sufficient without full session-ID plumbing) before committing to full attribution.
- **Historical storage choice**: this repo already has a precedent (`server/services/analytics_store.go`, used for approval analytics) — reusing that pattern vs. introducing a new one is a real design decision that could rabbit-hole if not scoped tightly in Phase 3.
- **"Config-driven, no rebuild"** implies either a config hot-reload mechanism or a restart-required value — Phase 2 research must confirm whether `config/` already supports hot-reload before planning assumes it's free.
- **`gh` CLI shell-outs are outside the native `http.Client` transport** — tracking them requires either wrapping every `gh` invocation with instrumentation, or migrating those call sites to the native client (`github/http_client.go`), which is arguably the more correct fix. Planning should evaluate migration vs. wrapping rather than defaulting to the more complex option.

## Alternatives Considered
- Log-only counters, tailable via `journalctl` — rejected: no history, no UI, harder to correlate trends over time.
- A small in-memory debug HTTP endpoint — rejected: no persistence across restarts, no historical trend.
(Both were offered directly and explicitly declined in favor of this full-feature scope.)

## Feasibility Risks
- `gh` CLI shell-outs sit outside the instrumented native transport (see Rabbit Holes) — the fix approach affects how much of the rest of the feature can reuse existing `rateLimitTransport` instrumentation vs. needing a parallel counting path.
- Config hot-reload may not exist yet in `config/` — if it doesn't, "no rebuild required" may still require a service restart, which changes the UX claim in Success Metrics slightly and should be called out explicitly rather than silently assumed.

## Observability Requirements
This feature's own operational footprint should be zero-dependency — it must be useful without `OTEL_ENABLED=true`. Historical storage location/retention period is a Phase 2 research question (evaluate reusing `analytics_store.go`'s pattern first, per Rabbit Holes). Extend, don't duplicate, the existing WARN-at-threshold log line in `github/rate_limit.go` to also persist the event.

## Risk Control
No feature flag needed — this is additive (new tracking + new UI panel), and config-driven poll intervals default to today's hardcoded values, so existing poller behavior is unchanged unless the user explicitly opts into a different interval. Rollback = revert the PR; no migration risk to existing data since this introduces new storage rather than altering existing storage.

## Open Questions
- ~~Does `config/` already support hot-reload...~~ **Resolved by research**: no hot-reload exists anywhere in `config/`; poll-interval and warn-threshold config changes require a service restart, consistent with the existing `DaemonPollInterval` precedent.
- ~~What's the right persisted-storage mechanism...~~ **Resolved by research, with a sub-decision for Phase 3**: persist via the existing ent/SQLite pattern (not a flat file). Two research agents converged on slightly different reuse targets — `architecture.md`/`features.md` recommend forking `server/services/analytics_store.go`'s shape into a new `APIUsageStore`; `build-vs-buy.md` found `server/analytics/` (a separate, already-generic `AnalyticsEvent` schema with its own DB + retention policy) may be a closer direct fit. Phase 3 planning must pick one explicitly and justify it, not silently default.
- Should the `gh`-CLI-shelling-out call sites be migrated to the native `http.Client` instead of wrapped separately? **Partially resolved**: scope widened from 1 to 7 real call sites (user decision 2026-08-10, see Scope). The migrate-vs-wrap technical approach itself is still open — `architecture.md` enumerated migration risks (field-shape parity, auth divergence, error-surface change, test-seam swap) without a firm verdict — left for Phase 3 planning + adversarial review to decide.
- **New, surfaced by pitfalls.md + architecture.md research**: `github/rate_limit.go`'s `DefaultRateLimiter.Update()` is documented as wired into an automatic `rateLimitTransport`, but that transport does not exist anywhere in the codebase — `Update()` has zero callers today, so `IsLimited()` (checked by both pollers) always returns `false` in production. This is not just a missing counter; Phase 3 must design and build this transport hook for the first time as a prerequisite for everything else in this feature.
