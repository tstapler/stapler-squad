# Implementation Plan: github-call-optimization

**Feature**: Instrument, prioritize, and reduce the volume of stapler-squad's outbound GitHub API
calls (native HTTP + `gh` CLI subprocess) without regressing the existing ETag/singleflight
design, shipped as five staged, independently-mergeable PRs.
**Date**: 2026-09-08
**Status**: Ready for implementation
**ADRs**: ADR-001-defer-graphql-batching (this project), ADR-002-origin-tag-fail-closed-default (this project)

---

## Step 0.5 — Alternatives considered (creative pass)

Three architectural decisions in this project had real alternatives; each is recorded here and
in the Pattern Decisions table below rather than silently anchored on the first idea.

**1. How should priority-aware admission control be shaped?**
- *A — Reserved-headroom gate extending `RateLimiter`* (chosen). Strength: reuses the one thing
  the transport already observes for free (`X-RateLimit-Remaining`/`Limit`) and preserves the
  pollers' existing "skip the tick" semantics — no new blocking primitive. Weakness: still a
  single shared mutable resource, so it inherits `RateLimiter`'s own concurrency-hardening burden
  (mitigated via the atomic-snapshot pattern below).
- *B — Priority queue (block/reorder callers on a heap, à la `session/command_queue.go`)*.
  Strength: gives exact FIFO/priority ordering if calls ever need to be queued rather than
  dropped. Weakness: nothing in this codebase blocks on GitHub rate limits today — introducing a
  new blocking primitive where every existing caller already chooses to skip-and-retry-next-tick
  is a bigger behavior change than the problem requires.
- *C — Per-origin circuit breaker (open/half-open/closed with timers)*. Strength: well-understood
  pattern for protecting against an independently-flaky downstream. Weakness: every origin here
  shares one upstream (GitHub) and one already-observed signal (remaining quota) — a breaker's
  state-machine surface (half-open timers, trip thresholds) solves a problem this project doesn't
  have.

**2. How should the GraphQL migration for `GetPRInfoCtx` be scoped?**
- *A — Wholesale per-tick batched query replacing the pollers' REST+ETag loop.* Strength: fewest
  HTTP round-trips per tick. Weakness (fatal): GitHub's GraphQL endpoint has no ETag/304 support
  (`research/pitfalls.md` §0) — every tick becomes a full-cost fetch for every tracked PR, which
  can *increase* call volume and directly undermines Success Metric #2. Rejected.
- *B — Migrate only `GetPRInfoCtx`'s own implementation to a single-PR GraphQL query, leaving
  `GetPRInfoConditional`'s REST+ETag path as the unchanged primary per-tick mechanism* (chosen).
  Strength: keeps 100% of the existing 304 savings; still gets GraphQL's advantage (one round
  trip for reviews+checks+metadata instead of a `gh` subprocess) on the "PR changed" fallback path
  both pollers already take, plus every interactive RPC. Weakness: doesn't reduce the *number* of
  calls per tick, only their cost/shape.
- *C — "Changed subset" aliased batching* (the Rabbit Hole's proposed compromise). Strength: could
  coalesce multiple simultaneously-changed PRs into one call on a busy tick. Weakness: real added
  complexity (query-cost accounting, GitHub's Sept-2025 `RESOURCE_LIMITS_EXCEEDED` failure mode,
  partial-alias-failure handling, a grouping-by-repo design with no existing precedent per
  `research/stack.md` §2a) for a win that's marginal once (B)'s correction is priced in — most
  ticks have zero or one changed PR, not enough for batching to pay for its own complexity.
  **Rejected — scoped out of this plan and recorded as a documented future enhancement**
  (see ADR-001), per the coordinator's explicit permission to do so.

**3. How should the webhook-driven poller-invalidation consumer dedup against redelivery?**
- *A — Reuse `ExistsByDeliveryID` as a second consumer* (rejected). Its own doc comment
  (`session/trigger_fire_event_repository.go:49`) says it's a non-atomic check-then-act; adding a
  second racing consumer on top of the PR-fix router compounds a known-latent race
  (`research/pitfalls.md` §1c) instead of fixing it.
- *B — A new `(consumer, delivery_id)` unique-indexed dedup table*, mirroring the push-event
  path's `claimAndFireTrigger` pattern. Strength: closes the race properly. Weakness: new schema
  + ent migration for a problem that has a cheaper structural fix.
- *C — Design the consumer to be naturally idempotent (invalidate-and-refetch), so no dedup is
  needed at all* (chosen). Strength: zero new schema, and correctness doesn't depend on a race
  being closed — a duplicate delivery just triggers one extra harmless refetch. Weakness: none
  material — this is the "prefer this over adding new dedup infrastructure if it avoids the need
  entirely" case the coordinator's brief called for.

All three chosen options, plus every rejected alternative and its reason, are also recorded in
the Pattern Decisions table.

---

## Domain Glossary

| Term | Definition | Notes |
|------|-----------|-------|
| `CallOrigin` | A `string`-based type identifying who initiated an outbound GitHub call. | New, `github` package. |
| `OriginInteractive` | `CallOrigin` value for a user-triggered RPC (`GetPRInfo`, `GetPRComments`, `PostPRComment`, `MergePR`, `ClosePR`). | Never backs off under admission control. |
| `OriginPRStatusPoller` | `CallOrigin` value for `session.PRStatusPoller`'s tick. | Distinct from `OriginWorktreePRPoller` per `research/features.md` §1 (different structs, different backoff state). |
| `OriginWorktreePRPoller` | `CallOrigin` value for `session.WorktreePRPoller`'s tick. | |
| `OriginBacklogSync` | `CallOrigin` value for `session.SyncLoop`'s GitHub-issue sync tick. | |
| `OriginWebhookReconcile` | `CallOrigin` value for outbound calls made *by* a webhook-triggered reconciliation (not the inbound webhook receipt itself). | |
| `WithGitHubCallOrigin(ctx, origin)` | Tags a `context.Context` with a `CallOrigin`, following the `prFixTriggerSourceKey` shape (`session/backlog_lifecycle_pr.go:1662-1685`). | New, `github` package. |
| `GitHubCallOriginFrom(ctx)` | Reads the tagged `CallOrigin`, defaulting to `OriginPRStatusPoller` (background-tier) when untagged. | Default direction is the opposite of `prFixTriggerSourceFrom`'s — see ADR-002. |
| `RateLimiterSnapshot` | **Revised, pre-mortem P1 #2**: immutable struct `{Resources map[string]ResourceQuota; RateLimitedUntil time.Time}` published via `atomic.Pointer[RateLimiterSnapshot]`, where `ResourceQuota{Remaining, Limit int; ResetAt time.Time}` is keyed by GitHub's `X-RateLimit-Resource` value (`"core"`, `"search"`, `"graphql"`). The original flat `{Remaining, Limit int; ResetAt, RateLimitedUntil time.Time}` shape conflated core/search/graphql's independent quotas (`rate_limit.go`'s `Update()` already parses a distinct `resource` per response at line 50 but discards it after the warning log) — a low-limit `search` response could overwrite `core`'s numbers and silently defeat admission control. `RateLimitedUntil` stays a *single top-level field*, not per-resource: `rate_limit.go`'s own `setLimitedUntil`/`IsLimited` (lines 169-175, 143-150) already track one process-wide cooldown regardless of which resource's response triggered it, and `rateLimitTransport.RoundTrip`'s single `IsLimited()` pre-check (`github/http_client.go:48`) gates *every* outbound call, not just calls to the triggering resource — partitioning `RateLimitedUntil` by resource would diverge from that existing, deliberately conservative behavior, which this plan does not intend to change. | New; read-side of `RateLimiter`; see Story 3.1.1. |
| `AdmitOrigin(origin CallOrigin, resource string) (bool, string)` | **Revised, pre-mortem P1 #2**: decision method on `RateLimiter`: admits or rejects a call for `origin` against the quota of the specific `resource` the call targets, returning a reason string when rejected. `resource` is derived by the sole caller (`rateLimitTransport.RoundTrip`) from the outbound request's URL/method before dispatch, not passed down from elsewhere — see Story 3.2.1. | New method, `github/rate_limit.go`. |
| `backgroundHeadroomPercent` | Configurable % of a resource's `Limit` reserved from background origins for *that resource* (mirrors `rateLimitWarnPercent`'s existing percentage-of-limit math, `rate_limit.go:20`, now applied per-resource). | New constant. |
| `githubTelemetryTransport` | New `http.RoundTripper`, outermost wrapper around `otelhttp.NewTransport(&rateLimitTransport{...})`, emitting the GitHub-specific span attributes/metrics. | New, `github/telemetry_transport.go`. |
| `GHCallSite` | A fixed enum/string of the ~30 named wrapper call sites, used as a span attribute / metric label. | Bounded cardinality — see Observability Plan. |
| `runGHCLICommand` | Shared instrumentation core (records span/metric/origin/call-site, then delegates) used by both gh-CLI adapters. | New, `github/gh_exec.go`. |
| `ghCLIExecAdapter` | Thin wrapper around `safeexec.CommandContext` for `github/*.go`'s ~10 sites. | Calls `runGHCLICommand`. |
| `worktreeGHCommandAdapter` | Thin wrapper around `g.commandRunner().Run(...)` for `session/git/worktree_git.go`'s 7 `gh`-prefixed sites and `session/git/util.go`'s 1 site. | Calls `runGHCLICommand`; does **not** touch `tmux.CommandRunner`'s signature. |
| `ETagCache.Invalidate(owner, repo string, prNumber int)` | New exported method deleting a cache entry via `sync.Map.Delete`. | `github/etag_cache.go`. |
| `PRStatusPoller.InvalidateAndRefresh(ctx, owner, repo string, prNumber int) (matched bool)` | Invalidates the cache entry, finds the matching `*Instance`, and re-runs `fetchAndUpdatePRStatus` out of band. | New method. |
| `WorktreePRPoller.InvalidateCache(owner, repo string, prNumber int)` | Cache-only invalidation — no persistent per-worktree index exists to target a refetch at (see Phase 5 design note). | New method. |
| `GitHubPollerInvalidator` | Narrow interface in `server/services`, satisfied by a `session`-package adapter wrapping both pollers — mirrors `PRFixEventRouter`. | New. |
| `pollerInvalidationAdapter` | Concrete `session`-package type satisfying `GitHubPollerInvalidator`. | New. |
| `GetPRInfoGraphQL` | New function: single-PR GraphQL lookup replacing `gh pr view --json ...` as `GetPRInfoCtx`'s implementation when the flag is on. | `github/client_graphql.go`. |
| `githubGraphQLMigrationFlagName` | `"github:graphql-pr-info"` feature-flag constant. | Default off. |
| `githubPriorityAdmissionFlagName` | `"github:priority-admission-control"` feature-flag constant. | Default off. |
| `PRInfoParity` (test helper) | Field-by-field comparison of a gh-CLI-populated `*PRInfo` vs. a GraphQL-populated `*PRInfo` for the same live PR. | Gates the flag's eventual default-on; not shipped as a live default flip in this plan. |
| `norawghrequest` | New `go/analysis` linter flagging raw `http.NewRequest`/`http.NewRequestWithContext` to a GitHub host outside the approved constructors. | `tools/lint/norawghrequest/analyzer.go`, mirrors `tools/lint/norawgitopen`. |
| `GitHubRateLimitReason` | Frontend `string` union: `"transient"` \| `"exhausted"`, mirroring `FailureReason` (`web-app/src/lib/utils/sessionFailure.ts`). | New, `web-app/src/lib/vcs/`. |
| `getGitHubRateLimitMessage(reason)` | Frontend function mapping `GitHubRateLimitReason` to friendly copy, mirroring `getFailureReasonToastMessage` (`NotificationContext.tsx:35-48`). | New. |
| `WebhookPRSignalKind` | The 4 existing event types (`check_run`, `workflow_run`, `pull_request_review`, `issue_comment`) already extracted by `extractPRFixEvent` (`github_webhook_pr_fix.go:68`). | Reused, not new — the poller-invalidation consumer subscribes to the same 4 types. |

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| Admission control shape | Reserved-headroom gate extending `RateLimiter` | `research/architecture.md` §1a; same soft/hard hysteresis shape as `server/services/quota_gate.go` | Priority queue (block/reorder) | No existing caller blocks on GitHub limits today — both pollers already skip-the-tick; a queue is a new primitive the problem doesn't need |
| Admission control shape | (same) | | Per-origin circuit breaker | One shared upstream, one already-observed gauge — a breaker's state machine is unneeded surface |
| Admission control state | `atomic.Pointer[RateLimiterSnapshot]` published read model | `.claude/rules/instance-lock-free-reads.md`'s `Snapshot()` convention; `research/build-vs-buy.md` §6 | More fields under `RateLimiter`'s existing `sync.RWMutex` | BUG-080/081 precedent: this exact class of global mutable state has broken under concurrent test access before; a snapshot read model is cheaper and safer than growing the mutex-guarded struct |
| GraphQL migration scope | Single-PR `GetPRInfoCtx` migration only; `GetPRInfoConditional`'s REST+ETag path untouched | `research/pitfalls.md` §0; ADR-020 (hand-rolled GraphQL, no client lib) | Wholesale per-tick batched query replacing the REST+ETag loop | GitHub GraphQL has no ETag/304 — would discard 304 savings on unchanged PRs, undermining Success Metric #2 |
| GraphQL migration scope | (same) | | "Changed subset" aliased batching | Marginal win vs. real complexity (query-cost accounting, `RESOURCE_LIMITS_EXCEEDED`, partial-alias-failure handling); scoped out, see ADR-001 |
| gh-CLI instrumentation | Two adapters (`ghCLIExecAdapter`, `worktreeGHCommandAdapter`) converging on one `runGHCLICommand` core | GoF Adapter; `research/architecture.md` §4 | One literal wrapper for all 22 sites | 11 of 22 sites go through `tmux.CommandRunner`, deliberately execution-shape-generic per ADR-002 (ssh-remote-workspaces) — threading GitHub tags into `Run`'s signature pollutes a shared, higher-traffic interface |
| Conditional-requests enforcement | Custom `go/analysis` linter (`norawghrequest`) | `tools/lint/norawgitopen` precedent; `research/build-vs-buy.md` §5 | `.claude/rules/*.md`-only convention | A missing ETag header is silent at compile *and* test time — no race-detector-style safety net exists for this bug class the way it does for `instance-lock-free-reads.md`'s |
| Webhook → poller invalidation dedup | Invalidate-and-refetch (naturally idempotent); no new dedup mechanism | `research/pitfalls.md` §1c/§1d | Reuse `ExistsByDeliveryID` as a second consumer | Documented non-atomic check-then-act race; a second racing consumer compounds it |
| Webhook → poller invalidation dedup | (same) | | New `(consumer, delivery_id)` unique-indexed table | Unneeded schema/migration for a problem invalidate-and-refetch's idempotency already solves |
| Origin tagging mechanism | `context.Context` value (`WithGitHubCallOrigin`/`GitHubCallOriginFrom`) | Existing `prFixTriggerSourceKey` precedent (`session/backlog_lifecycle_pr.go:1662`) | Explicit parameter threaded through `RefreshPRInfo`/`SetCommitStatus`/`GeneratePRContextPrompt` | Would require invasive signature changes across `github`/`session`/`server/services` multi-hop chains — exactly what the existing precedent already avoids |
| Origin tagging default | Untagged context defaults to `OriginPRStatusPoller` (background tier) | Corrects the risk `research/architecture.md` §3 flags in its own sketch | Default to `OriginInteractive` (mirroring `prFixTriggerSourceFrom`'s log-only default) | This read site makes an admission-control *decision*, not a log tag — defaulting to the unrestricted tier would let a background caller that forgets to tag its context silently escape admission control entirely; see ADR-002 |
| GitHub API Go client | Continue hand-rolled `net/http` + JSON, extending `user_pr_cache.go`'s pattern | `research/build-vs-buy.md` §1 | `shurcooL/githubv4` | Unmaintained (last commit July 2024); its own open issue #17 names query batching — the exact capability this project needs — as unresolved |
| GitHub API Go client | (same) | | `google/go-github` | REST-only, no GraphQL surface; irrelevant to the batching/migration goal |
| Rate-limiting primitive | Bespoke reserved-headroom extension of `RateLimiter` | `research/build-vs-buy.md` §3 | `golang.org/x/time/rate`, `uber-go/ratelimit` | Both model self-paced QPS; GitHub's budget resets in a server-controlled lump sum, not a rate to spend against |
| OTel HTTP instrumentation | `otelhttp.NewTransport` composed outside `rateLimitTransport`, plus a hand-rolled `githubTelemetryTransport` for custom attributes/gauges | `research/build-vs-buy.md` §2 | Hand-roll all HTTP span/metric code | `otelhttp` is a zero-new-dependency, already-proven-in-repo (`server/server.go:1303`) win for the generic span/duration baseline |
| gh-CLI subprocess instrumentation | Hand-rolled span/metric wrapper | `research/stack.md` §1b | Any existing library | Confirmed no off-the-shelf OTel CLI-subprocess instrumentation exists anywhere in the ecosystem |
| Frontend error classification | Reason-code switch mirroring `getFailureReasonToastMessage` | `research/ux.md` §1 precedent (`NotificationContext.tsx:35-48`) | New toast/panel component | No new UI surface needed — `VcsPanel.tsx`'s error box and `VcsWidgetBlockingReasons.tsx`'s staleness notice already scaffold this |

---

## Tech Debt Disposition

| Area | Existing Issue | Disposition | Justification |
|------|----------------|--------------|----------------|
| `github/rate_limit.go`, `github/http_client.go`, `github/etag_cache.go` | None — already show correct `atomic`/`sync.Map`/`sync.RWMutex` idioms; BUG-022/023 fixed and re-verified 2026-07-22 | **Extend as-is** | Small, single-responsibility files; nothing structural blocks adding `AdmitOrigin`/`Invalidate` |
| 22 `gh`-CLI subprocess call sites (`github/*.go`'s `safeexec.CommandContext` vs. `session/git/*.go`'s `tmux.CommandRunner`) | Not architecturally uniform — two different execution primitives papered over by the requirements' "one wrapper" framing | **Isolate via seam** | Two thin adapters (`ghCLIExecAdapter`, `worktreeGHCommandAdapter`) converging on one shared `runGHCLICommand` core, without polluting `tmux.CommandRunner`'s deliberately execution-shape-generic interface (ADR-002, ssh-remote-workspaces) |
| `session/pr_status_poller.go`, `session/worktree_pr_poller.go` | None — both already went through BUG-023's atomic/lock-free conversion; clean ticker→snapshot→fan-out→apply shape | **Extend as-is** | No fan-out *restructuring* is needed once batching is scoped out (see ADR-001) — the ticker loop, its concurrency limiting, and its snapshot/apply shape are all untouched. This disposition does **not** claim zero signature changes: Phase 5 Tasks 5.1.2a-prime/5.2.1a-prime give `fetchAndUpdatePRStatus`/`fetchAndStore` a `ctx context.Context` parameter (moving each function's internally-built `context.WithTimeout` out to its one existing caller) so `InvalidateAndRefresh`'s webhook-triggered call can pass a ctx tagged `OriginWebhookReconcile` distinct from the ticker's own `OriginPRStatusPoller`/`OriginWorktreePRPoller` tag — a small, one-caller-each touch, not a structural one |
| `session/pr_tracking.go`'s `Instance` methods (`RefreshPRInfo`, `SetCommitStatus`, `GeneratePRContextPrompt`, etc.) take no `context.Context` today | Structural gap, not a violation — these predate this project's origin-tagging need | **Refactor-first**, scoped narrowly | Origin tagging via `context.Context` value requires these methods to accept a `ctx` for the first time; sequenced as Phase 1's own first story (1.1.1) so every later phase's origin-tagged call sites build on top of real `ctx` plumbing rather than a workaround |

---

## Observability Plan

- **Logs**: structured entry/exit at each new boundary (`githubTelemetryTransport`, `runGHCLICommand`, `PRStatusPoller.InvalidateAndRefresh`) — one line per call, following `RateLimiter.Update`'s existing "log on state *transition* only" discipline (`rate_limit.go:82-89`) for anything per-tick; no new per-tick log spam (`research/features.md` §3).
- **Metrics** (via `telemetry.GetMeter()`, rides the existing OTLP/Datadog pipeline, no new storage):
  - `github.calls_total` (counter) — labels: `origin`, `call_site`, `resource`. Cardinality: `call_site` is a fixed enum of ~30 wrapper-owned names (never a formatted string/stack trace); `origin` is 5 values; `repo` is *not* a metric label (see below) — bounded to a few hundred series total.
  - `github.call.duration_ms` (histogram) — same labels as above.
  - `github.rate_limit.remaining` (gauge) — label: `resource` (core/search/graphql). Implemented as an `Int64ObservableGauge` with a callback that atomically reads `RateLimiterSnapshot.Resources` (per-resource map, pre-mortem P1 #2) and emits one observation per resource key present, so `{resource="core"}` and `{resource="search"}` report their own independent `Remaining` values rather than one conflated number (resolves `research/stack.md`'s "async vs sync" open item: async, callback-based, backed by the snapshot so the callback never blocks on I/O).
  - `github.cache.result_total` (counter) — labels: `call_site`, `result` (`hit`|`miss`), for 304-vs-200 on the REST conditional path.
  - `github.admission.rejected_total` (counter, Phase 3 only) — label: `origin`.
  - `github.webhook.last_delivery_age_seconds` (gauge, Phase 5) — label: `event_type`. Answers `research/pitfalls.md` §4.2's "quiet because nothing changed vs. quiet because delivery is broken" distinction.
  - **Cardinality rule, stated explicitly per `research/pitfalls.md` §2e**: `repo` and any PR number/query text are span *attributes* (unbounded cardinality is fine on a trace span) but must never become metric *labels* (where unbounded cardinality is a real cost). `call_site`/`origin`/`resource`/`event_type` are the only metric labels in this plan.
- **Alerts**: none proposed yet, per requirements' own "Phase 2 research should check" note — `research/stack.md`/`features.md` found no existing rate-limit-exhaustion alert wiring to extend. Left as explicit follow-up, not silently dropped: **Task 5.4.1b** below adds the raw metric; wiring a Datadog monitor on top of it is out of scope for this plan (no existing alert-authoring tooling in this repo to reuse) and is noted as a natural next step for whoever owns the Datadog dashboard.

## Risk Control

- **Feature flags**:
  - `github:priority-admission-control` (Phase 3) — default **false**.
  - `github:graphql-pr-info` (Phase 4) — default **false**.
  - Phases 1, 2, 5 ship unflagged (additive/structural or fail-open-by-construction, per requirements' Risk Control section).
  - **Phase 5 caveat**: "unflagged" here means Phase 5 introduces no *new* feature flag of its own —
    it does not mean the webhook-invalidation path is always active. `GitHubWebhookHandler` (which
    Phase 5's `pollerInvalidationAdapter` wiring hangs off, Task 5.3.1e) is itself only constructed
    when the pre-existing `webhook_triggers` feature flag is on (`server/server.go:837`,
    `if webhookCfg.GetFeatureFlag("webhook_triggers")`); when that flag is off, `/webhooks/github`
    is never registered and Phase 5's invalidation code paths are simply never reached — verified by
    reading `server/server.go:823-846`. This is an existing, unrelated flag this project doesn't
    control or change; it's noted here so "unflagged" isn't misread as "always active."
    **Second, independent gate (pre-mortem P2 #4)**: even when `webhook_triggers` is on and
    `/webhooks/github` is registered, `handlePRFixEvent` — the function Phase 5's
    `InvalidateForEvent` call is wired inside (Task 5.3.1c) — itself early-returns with a silent
    `200 OK` no-op when the pre-existing `pr_event_webhooks` feature flag is off
    (`server/services/github_webhook_pr_fix.go:461`: `if h.cfg == nil ||
    !h.cfg.GetFeatureFlag("pr_event_webhooks")`), verified by direct read. This plan previously named
    only `webhook_triggers` here; **both** flags gate Phase 5 independently, and either being off is
    enough to make Phase 5 ship completely inert with `make ci` green. `server/server.go:863`
    already logs a warning for the inverse case (`pr_event_webhooks` on, `webhook_triggers` off — the
    `else` branch of the `webhook_triggers` check at `server/server.go:837-865`), but there is no
    equivalent signal for *this* case (`webhook_triggers` on, `pr_event_webhooks` off), where routes
    register successfully and every PR-fix webhook delivery 200s while silently doing nothing. Task
    5.3.1g (Epic 5.3) adds the missing handler-level log line to close this gap.
- **Rollback procedure**: standard revert via PR close + revert commit for Phases 1, 2, 5. For Phases 3/4, flip the flag back to `false` via `UpdateFeatureFlag` first (`config.Config.SetFeatureFlag`, live, no redeploy) — a full code revert is the fallback only if the flag itself is misbehaving.
- **Staged rollout**: both flags ship OFF at merge (user decision, 2026-09-08). **Forcing function
  (pre-mortem P1 #1)**: "Tyler flips them manually after observing metrics" has no owner or date, so
  it is not itself a plan — replaced here with two concrete, dated triggers tied to metrics this
  project actually emits:
  - **`github:priority-admission-control`** — Owner: Tyler. Starting the first full calendar week
    after Phase 3 merges, check *weekly* (a standing action item, not a one-time check) whether
    `github.rate_limit.remaining{resource="core"}` (Task 1.2.1d's gauge, live since Phase 1) dropped
    below `backgroundHeadroomPercent` of `core`'s limit (i.e. below ~500 of 5000) at least once in
    the preceding 7 days **while** `github.calls_total{origin!="interactive",resource="core"}`
    (Task 1.2.1a's counter, also live since Phase 1) shows non-zero background-origin call volume in
    that same window — i.e., background traffic was actually present during a low-remaining episode,
    not merely a theoretical risk. If observed, flip the flag that same week via `UpdateFeatureFlag`
    (per the Rollback procedure above — live, no redeploy) and record the flip date plus the
    triggering metric values wherever this project's follow-up work is tracked. If 4 consecutive
    weeks pass with no such observation, that is itself a data point (interactive/background
    contention on `core` isn't materializing in practice) — revisit whether flipping Phase 3 on is
    still worth doing, rather than defaulting to silent indefinite inaction either way.
  - **`github:graphql-pr-info`** — Owner: Tyler. No later than 2 weeks after Phase 4 merges,
    re-run Task 4.1.2's parity test against a *live* PR (not the frozen fixtures — the same
    live-verification step Phase 4's Cross-cutting Acceptance and pre-mortem #5 both call for). If it
    passes, flip the flag that week. If it fails, file the schema-drift finding as a follow-up bug and
    leave the flag off — but the 2-week check itself is not optional, so the flag's fate is decided
    on a fixed date rather than drifting indefinitely.
  - Neither trigger is "automatic" (no code auto-flips a flag) — both are dated, metric-grounded
    human action items with a named owner, closing the gap the pre-mortem identified: without this,
    Success Metrics #1 and #2 had a mechanism but no path to ever activating it.

## Unresolved Questions

- [ ] Exact GraphQL field mapping for `reviews`/`statusCheckRollup`'s union types (`CheckRun`/`StatusContext`) needs live-schema verification via `gh api graphql` introspection, not just recalled shape (`research/stack.md` §2b flags this as unverified). — blocks Task 4.1.1a — owner: implementer, during Phase 4 Task 4.1.1a itself (first sub-step is the introspection query, not an external dependency).
- [ ] Full enumeration of `Instance.SetCommitStatus`'s and `Instance.GeneratePRContextPrompt`'s own callers (to assign each an origin) was not completed in this research pass — both currently show zero non-test call sites via grep, suggesting either dynamic dispatch (hook registration) or genuinely dead/rarely-exercised paths. — blocks Task 1.1.1c — owner: implementer, via a repo-wide grep (including hook-registration tables) at the start of that task; if truly zero callers exist, tag with `OriginPRStatusPoller`'s background default and note it in a code comment rather than blocking the task.

---

## Dependency Visualization

```
Phase 1 (Observability, unflagged)
  Epic 1.1 ctx+origin plumbing ──┐
  Epic 1.2 HTTP transport telemetry ──┤
  Epic 1.3 gh-CLI adapters (2 seams) ──┤──> Phase 3 (admission control, needs origin tag + RateLimiterSnapshot)
                                       │──> Phase 4 (GraphQL migration, needs call-site telemetry to compare against)
                                       └──> Phase 5 (webhook invalidation, needs InvalidateAndRefresh telemetry to verify it fires)

Phase 2 (conditional-requests infra + linter, unflagged) ── independent of 1/3/4/5, can ship in parallel with Phase 1

Phase 3 (admission control, flagged OFF) ── depends on Phase 1 (origin tag, RateLimiterSnapshot)

Phase 4 (GraphQL migration, flagged OFF) ── depends on Phase 1 (telemetry to run the parity test against)
                                          ── independent of Phase 3 (different code paths)

Phase 5 (webhook invalidation, unflagged) ── depends on Phase 1 (span coverage on InvalidateAndRefresh)
                                           ── independent of Phase 3/4

Phase 6 (frontend error copy, unflagged) ── depends on Phase 1 only (needs RateLimiterSnapshot's
                                             per-resource reset time already available); independent of 3/4/5
```

Phases 2 and 6 can run concurrently with Phase 1 once Phase 1's Epic 1.1 (ctx plumbing) lands;
Phases 3, 4, 5 each depend only on Phase 1, not on each other — they can proceed in parallel as
separate PRs once Phase 1 merges.

---

## Phase 1: Observability

### Epic 1.1: Context + origin-tag plumbing
**Goal**: Every GitHub-call code path can carry a `CallOrigin` through `context.Context`, including the three `Instance` methods that today take no `ctx` at all.

#### Story 1.1.1: Add `ctx` to `Instance`'s PR-tracking methods and thread `CallOrigin`
**As a** developer instrumenting GitHub calls, **I want** every `Instance` PR-tracking method to accept a `context.Context`, **so that** an origin tag set by the caller reaches `github.GetPRInfoCtx`/`GetPRInfoConditional` without a parallel parameter.
**Acceptance Criteria**:
- `RefreshPRInfo`, `GetPRComments`, `GetPRDiff`, `PostComment`, `SetCommitStatus`, `MergePR`, `ClosePR`, `GeneratePRContextPrompt` (`session/pr_tracking.go`) all accept `ctx context.Context` as their first parameter.
  - *Given* `Instance.RefreshPRInfo` today has signature `func (i *Instance) RefreshPRInfo() (*github.PRInfo, error)`, *When* Task 1.1.1a lands, *Then* its signature is `func (i *Instance) RefreshPRInfo(ctx context.Context) (*github.PRInfo, error)` and it calls `github.GetPRInfoCtx(ctx, i.GitHubOwner, i.GitHubRepo, i.GitHubPRNumber)` instead of `github.GetPRInfo(...)`.
- All 3 existing callers of `RefreshPRInfo` (`server/services/github_service.go:62`, `session/pr_tracking.go:100` inside `SetCommitStatus`, `session/pr_tracking.go:184` inside `GeneratePRContextPrompt`) pass a tagged context.
  - *Given* `GitHubService.GetPRInfo` (`server/services/github_service.go:45`) receives an inbound RPC context `ctx`, *When* it calls `instance.RefreshPRInfo(ctx)`, *Then* `ctx` was already tagged `OriginInteractive` by Task 1.1.1c's wiring before reaching `RefreshPRInfo`.
**Files**: `session/pr_tracking.go`, `server/services/github_service.go`, `session/pr_tracking_test.go`, `github/client.go`, and any test files exercising `MergePR`/`ClosePR`/`PostComment` directly.

##### Task 1.1.1a: Add `ctx` parameter to `RefreshPRInfo`, `SetCommitStatus`, `GeneratePRContextPrompt` (~5 min)
- Change all three signatures in `session/pr_tracking.go` to accept `ctx context.Context` first; update their internal `github.GetPRInfo(...)` calls to `github.GetPRInfoCtx(ctx, ...)`.
- Files: `session/pr_tracking.go`

##### Task 1.1.1a-prime: Add `ctx` parameter directly to `github.GetPRComments`, `GetPRDiff`, `PostPRComment`, `MergePR`, `ClosePR` (~5 min)
- Unlike `GetPRInfo`/`GetPRInfoCtx` (`github/client.go:284/290`), these five functions
  (`github/client.go:589,634,657,680,712`) have no Ctx-suffixed sibling today — each builds its
  own internal `context.WithTimeout(context.Background(), ...)` (lines 598, 642, 665, 698, 720).
  Add `ctx context.Context` as the first parameter to all five signatures directly (no new
  Ctx-suffixed pair — these aren't reachable without a ctx elsewhere in the codebase, per the grep
  below, so a breaking signature change is cheaper than a parallel `*Ctx` function), and replace
  each internal `context.WithTimeout(context.Background(), ...)` with
  `context.WithTimeout(ctx, ...)`, mirroring `GetPRInfoCtx`'s existing shape.
- A repo-wide grep (`grep -rn "github\.GetPRComments(\|github\.GetPRDiff(\|github\.PostPRComment(\|github\.MergePR(\|github\.ClosePR("`) confirms `session/pr_tracking.go` is the only non-test caller of all five today (lines 40, 59, 82, 146, 164) — Task 1.1.1b below updates those same call sites, so no other production code needs touching. `server/services/github_service_test.go` and `session/backlog_lifecycle_test.go` reference these functions/methods indirectly and may need a `context.Background()`/`t.Context()` argument added.
- Files: `github/client.go`

##### Task 1.1.1b: Thread `ctx` from `Instance.GetPRComments`, `GetPRDiff`, `PostComment`, `MergePR`, `ClosePR` into the now-ctx-taking `github.GetPRComments`/`GetPRDiff`/`PostPRComment`/`MergePR`/`ClosePR` (Task 1.1.1a-prime) (~5 min)
- Same mechanical change as Task 1.1.1a for the remaining 5 `Instance` methods in `session/pr_tracking.go`; pass the method's own `ctx` parameter straight through to the corresponding `github.*` call added in Task 1.1.1a-prime.
- Files: `session/pr_tracking.go`

##### Task 1.1.1c: Update the 3 known callers + grep for any remaining callers of the 8 changed methods (~5 min)
- Update `server/services/github_service.go:62` (and the other 4 `GitHubService` RPC methods at lines 96, 149, 180, 213) to pass their RPC's `ctx`.
- Grep the full repo (`session/`, `server/`, tests) for the other 7 methods' call sites; update each. Per the Unresolved Questions entry, if `SetCommitStatus`/`GeneratePRContextPrompt` truly have zero non-test callers, leave a one-line comment noting they're currently unreached and will inherit the background default until a caller exists.
- Files: `server/services/github_service.go`, plus whatever call sites the grep surfaces.

#### Story 1.1.2: `CallOrigin` type + context accessors
**As a** developer, **I want** a package-level `WithGitHubCallOrigin`/`GitHubCallOriginFrom` pair, **so that** every origin-tagged call site uses one consistent mechanism.
**Acceptance Criteria**:
- `github.CallOrigin` is a defined string type with 5 exported constants; `WithGitHubCallOrigin`/`GitHubCallOriginFrom` exist and `GitHubCallOriginFrom` defaults to `OriginPRStatusPoller` when the context carries no tag.
  - *Given* an untagged `context.Background()`, *When* `github.GitHubCallOriginFrom(ctx)` is called, *Then* it returns `github.OriginPRStatusPoller`, not `github.OriginInteractive`.
**Files**: `github/call_origin.go` (new).

##### Task 1.1.2a: Implement `CallOrigin` type, constants, `With`/`From` pair (~5 min)
- New file `github/call_origin.go`: unexported `githubCallOriginKey struct{}`, `type CallOrigin string`, 5 constants, `WithGitHubCallOrigin(ctx, origin) context.Context`, `GitHubCallOriginFrom(ctx) CallOrigin` (default `OriginPRStatusPoller`, per ADR-002).
- Files: `github/call_origin.go`

##### Task 1.1.2b: Unit test the default and round-trip (~3 min)
- Table test: untagged ctx → `OriginPRStatusPoller`; tagged with each of the 5 constants → round-trips exactly.
- Files: `github/call_origin_test.go` (new)

#### Story 1.1.3: Wire origin tags at every known call site
**As a** developer, **I want** every existing background/interactive call site to tag its own context, **so that** telemetry (this phase) and admission control (Phase 3) see accurate attribution from day one.
**Acceptance Criteria**:
- `PRStatusPoller.checkAllSessions`/`fetchAndUpdatePRStatus`, `WorktreePRPoller.pollWorktrees`/`fetchAndStore`, `SyncLoop.runAllSources`, and all 5 `GitHubService` RPC methods wrap their contexts before making a GitHub call.
  - *Given* `PRStatusPoller.fetchAndUpdatePRStatus` builds `ctx, cancel := context.WithTimeout(p.ctx, p.config.CallTimeout)` (`session/pr_status_poller.go:285`), *When* Task 1.1.3a lands, *Then* the next line becomes `ctx = github.WithGitHubCallOrigin(ctx, github.OriginPRStatusPoller)` before any `github.Get*` call in that function.
**Files**: `session/pr_status_poller.go`, `session/worktree_pr_poller.go`, `session/backlog_sync.go`, `server/services/github_service.go`.

##### Task 1.1.3a: Tag `PRStatusPoller`'s call sites (~4 min)
- Wrap `ctx` with `OriginPRStatusPoller` at the top of `fetchAndUpdatePRStatus` (`session/pr_status_poller.go:285`), covering both the auto-discovery (`GetPRForBranchConditional`) and conditional-fetch (`GetPRInfoConditional`) branches.
- Files: `session/pr_status_poller.go`

##### Task 1.1.3b: Tag `WorktreePRPoller`'s call sites (~4 min)
- Same for `fetchAndStore` (`session/worktree_pr_poller.go:~230`), tag `OriginWorktreePRPoller`.
- Files: `session/worktree_pr_poller.go`

##### Task 1.1.3c: Tag `SyncLoop`'s GitHub-issue-sync call sites (~4 min)
- Tag `OriginBacklogSync` at the entry of `runAllSources` (`session/backlog_sync.go:101`) so it propagates to `session/backlog_plugin_github.go`'s `github.HTTPClient().Do(req)` call sites (lines 187, 330, 396).
- Files: `session/backlog_sync.go`

##### Task 1.1.3d: Tag all 5 interactive RPC handlers (~5 min)
- At the top of `GetPRInfo`, `GetPRComments`, `PostPRComment`, `MergePR`, `ClosePR` (`server/services/github_service.go:45,96,149,180,213`), wrap the inbound `ctx` with `OriginInteractive` before calling into `session`.
- Files: `server/services/github_service.go`

### Epic 1.2: Native HTTP transport telemetry
**Goal**: Every native GitHub HTTP call gets a span, and rate-limit/cache-hit metrics flow to `telemetry.GetMeter()`.

#### Story 1.2.1: Compose `otelhttp` + a custom `githubTelemetryTransport`
**As an** operator, **I want** OTel spans on every native GitHub HTTP call, **so that** I can see latency/status/origin/call-site attribution in the existing Datadog pipeline.
**Acceptance Criteria**:
- `ghHTTPClient.Transport` is `&githubTelemetryTransport{next: otelhttp.NewTransport(&rateLimitTransport{next: http.DefaultTransport})}`, and a request that hits `rateLimitTransport`'s fail-fast branch still produces a span.
  - *Given* `DefaultRateLimiter.IsLimited()` returns `true`, *When* a caller invokes `ghHTTPClient.Do(req)`, *Then* `githubTelemetryTransport.RoundTrip` records a span named `github.http.request` with attribute `github.admission_skip=true` before `rateLimitTransport` returns its "skipping request" error — the skip is visible in the trace, not silently invisible per `research/stack.md` §1a.
- A 304 response increments `github.cache.result_total{call_site=...,result="hit"}`; a 200 increments the same counter with `result="miss"`.
  - *Given* `GetPRInfoConditional` sends an `If-None-Match` header and GitHub responds `304`, *When* `githubTelemetryTransport.RoundTrip` observes `resp.StatusCode == http.StatusNotModified`, *Then* it records one `github.cache.result_total` increment with `result="hit"`.
**Files**: `github/telemetry_transport.go` (new), `github/http_client.go`.

##### Task 1.2.1a: Implement `githubTelemetryTransport.RoundTrip` (~5 min)
- New struct wrapping `next http.RoundTripper`; starts a span via `telemetry.StartSpan(req.Context(), "github.http.request", trace.WithSpanKind(trace.SpanKindClient))`, sets `github.call.origin` (from `GitHubCallOriginFrom(req.Context())`), `github.resource` (from `X-RateLimit-Resource` response header, mirroring `rate_limit.go:50`), records `github.calls_total`/`github.call.duration_ms`, ends the span.
- Files: `github/telemetry_transport.go`

##### Task 1.2.1b: Compose the transport chain in `ghHTTPClient` (~3 min)
- Update `github/http_client.go:22-25`'s `ghHTTPClient` var to the 3-layer chain above.
- Files: `github/http_client.go`

##### Task 1.2.1c: Record cache hit/miss counter (~4 min)
- Inside `githubTelemetryTransport.RoundTrip`, after the inner `RoundTrip` returns, classify `resp.StatusCode == 304` → `"hit"`, `200` → `"miss"` and increment `github.cache.result_total`; skip the increment for non-200/304 responses (errors are covered by span status, not this counter).
- Files: `github/telemetry_transport.go`

##### Task 1.2.1d: Register the `github.rate_limit.remaining` observable gauge (~5 min)
- Register an `Int64ObservableGauge` at telemetry-init time (or lazily on first use) with a callback
  that iterates a resource-quota map (Task 1.2.1e's stub before Phase 3 lands; `RateLimiter.Snapshot().Resources`
  after Phase 3's Story 3.1.1 partitions `RateLimiterSnapshot` by resource — pre-mortem P1 #2) and
  emits one observation per resource key present, each carrying its own `resource` label
  (`{resource="core"}`, `{resource="search"}`, `{resource="graphql"}`), matching the Observability
  Plan's `resource`-label claim for this metric. **Correction**: this task's original wording
  assumed a single flat `(remaining, limit)` value, which cannot carry a per-resource label at all —
  the per-resource iteration described here is what actually satisfies the Observability Plan's
  existing claim.
- Files: `github/telemetry_transport.go` or `github/rate_limit.go`

##### Task 1.2.1e: Add a nil-safe `RateLimiter.currentResourceQuotas()` stub ahead of Phase 3 (~3 min)
- To let Task 1.2.1d compile and ship before Phase 3's real per-resource `RateLimiterSnapshot` exists,
  add a minimal `func (r *RateLimiter) currentResourceQuotas() map[string]struct{ Remaining, Limit int }`
  returning an empty map (no resources observed yet, so the gauge callback simply emits zero
  observations) until Phase 3 adds real per-resource storage. **Correction (pre-mortem P1 #2
  repair)**: the original version of this task specified a single-value `(remaining, limit int, ok
  bool)` stub and claimed "Phase 3's Story 3.1.1 replaces this stub's body, not its signature" — that
  claim no longer holds once `RateLimiterSnapshot` is partitioned by resource, since a single
  `(remaining, limit)` pair cannot represent multiple resources' quotas. Phase 3 instead replaces this
  stub outright with the real `Snapshot() RateLimiterSnapshot` accessor and its `.Resources` map;
  Task 1.2.1d's gauge callback is written from the start to iterate over a map-shaped value (stub or
  real), so only the one plumbing line that calls into `RateLimiter` changes in Phase 3, not the
  callback's iteration logic itself.
- Files: `github/rate_limit.go`

### Epic 1.3: gh-CLI subprocess instrumentation (two adapters)
**Goal**: Every `gh` CLI subprocess call — both the `safeexec.CommandContext` sites in `github/*.go` and the `tmux.CommandRunner`-routed sites in `session/git/*.go` — gets a span/metric via one shared core, without polluting `tmux.CommandRunner`'s signature.

#### Story 1.3.1: `runGHCLICommand` shared instrumentation core
**As a** developer, **I want** one function recording span/metric/origin/call-site for a gh-CLI invocation, **so that** both adapters share identical telemetry semantics.
**Acceptance Criteria**:
- `runGHCLICommand(ctx, callSite string, exec func() ([]byte, error)) ([]byte, error)` starts a span named `gh.<callSite>` with `trace.SpanKindClient`, attributes `process.command="gh"`, `github.call.origin`, `github.call_site`, records duration and `github.calls_total`, and returns `exec()`'s result unchanged.
  - *Given* a call site named `"pr.view"` and `ctx` tagged `OriginPRStatusPoller`, *When* `runGHCLICommand(ctx, "pr.view", execFn)` runs, *Then* it emits a span `gh.pr.view` with `github.call.origin="poller"` and `github.call_site="pr.view"`, and returns exactly what `execFn()` returned (no error wrapping).
**Files**: `github/gh_exec.go` (new).

##### Task 1.3.1a: Implement `runGHCLICommand` (~5 min)
- Files: `github/gh_exec.go`

##### Task 1.3.1b: Unit test span attributes + passthrough of exec's return value (~4 min)
- Files: `github/gh_exec_test.go` (new)

#### Story 1.3.2: `ghCLIExecAdapter` for `github/*.go`'s `safeexec.CommandContext` sites
**As a** developer, **I want** `github/*.go`'s `gh` subprocess sites instrumented, **so that** their telemetry matches the native HTTP path's.
**Acceptance Criteria**:
- `GetPRInfoCtx` (`github/client.go:290`) and the other `github/*.go` gh-CLI sites (`commit_status.go:90`, `cli_import.go:80`, and `github/client.go`'s remaining `safeexec.CommandContext(ctx, "gh", ...)` sites) route through `runGHCLICommand`.
  - *Given* `GetPRInfoCtx(ctx, "tstapler", "stapler-squad", 704)` is called with `ctx` tagged `OriginInteractive`, *When* the underlying `cmd.Output()` succeeds, *Then* a span `gh.pr.view` is recorded with `github.call.origin="interactive"` and `github.call_site="pr.view"`, wrapping the *existing*, unmodified `safeexec.CommandContext(ctx, "gh", "pr", "view", ...)` invocation.
**Files**: `github/client.go`, `github/commit_status.go`, `github/cli_import.go`.

##### Task 1.3.2a: Wrap `GetPRInfoCtx`'s `safeexec.CommandContext` call (~4 min)
- At `github/client.go:299`, wrap the `cmd.Output()` call in `runGHCLICommand(ctx, "pr.view", func() ([]byte, error) { return cmd.Output() })`.
- Files: `github/client.go`

##### Task 1.3.2b: Wrap the remaining `github/client.go` gh-CLI sites (lines 573, 600, 644, 667/700, 722, 742) (~5 min)
- Each gets its own `callSite` name (e.g. `"pr.list"`, `"pr.merge"` — name per the actual `gh` subcommand invoked at that line).
- Files: `github/client.go`

##### Task 1.3.2c: Wrap `commit_status.go:90` and `cli_import.go:80` (~4 min)
- Files: `github/commit_status.go`, `github/cli_import.go`

#### Story 1.3.3: `worktreeGHCommandAdapter` for `session/git/worktree_git.go` + `util.go`
**As a** developer, **I want** the `gh`-prefixed `commandRunner().Run(...)` call sites instrumented **without** changing `tmux.CommandRunner`'s signature, **so that** `session/tmux`'s non-GitHub callers (has-session, kill-session, git) are untouched.
**Acceptance Criteria**:
- A new unexported helper `(g *GitWorktree) runGHCommand(ctx context.Context, callSite string, args ...string) ([]byte, error)` wraps `g.commandRunner().Run(ctx, g.worktreePath, "gh", args...)` via `runGHCLICommand`, and all **11** `gh`-prefixed call sites in `worktree_git.go` (lines 71, 84, 301, 356, 390, 404, 674, 834, 854, 871, 885 — verified via `grep -n 'commandRunner().Run(.*"gh"' session/git/worktree_git.go`; the original 7-line list omitted the two `gh repo sync` sites at lines 71/84, the `gh browse` site at line 301, and the `CreatePR` fallback's `gh pr view` at line 390) plus `util.go:194`'s `gh auth status` use it instead of calling `g.commandRunner().Run` directly.
  - *Given* `worktree_git.go:834`'s existing call `g.commandRunner().Run(ctx, g.worktreePath, "gh", "pr", "merge", strconv.Itoa(prNumber), "--auto", "--squash")`, *When* Task 1.3.3b lands, *Then* the call site becomes `g.runGHCommand(ctx, "pr.merge", "pr", "merge", strconv.Itoa(prNumber), "--auto", "--squash")`, and `tmux.CommandRunner`'s interface definition (`session/tmux/command_runner.go:52-69`) is unchanged.
**Files**: `session/git/worktree_git.go`, `session/git/util.go`.

##### Task 1.3.3a: Implement `(g *GitWorktree) runGHCommand` helper (~4 min)
- Files: `session/git/worktree_git.go`

##### Task 1.3.3b: Replace all 11 `worktree_git.go` call sites (lines 71, 84, 301, 356, 390, 404, 674, 834, 854, 871, 885) (~8 min)
- Time estimate widened from ~5 min to ~8 min to account for the 4 additional sites (71, 84, 301,
  390) the original 7-line enumeration missed.
- Files: `session/git/worktree_git.go`

##### Task 1.3.3c: Replace `util.go:194`'s `gh auth status` call site (~3 min)
- Files: `session/git/util.go`

---

## Phase 2: Conditional-requests-by-default infrastructure + `norawghrequest` linter

### Epic 2.1: `norawghrequest` custom linter
**Goal**: Make it structurally hard for a future native GitHub HTTP call site to skip conditional-request semantics.

#### Story 2.1.1: Analyzer implementation, following `norawgitopen` exactly
**As a** developer adding a new GitHub API call site, **I want** the linter to flag a raw `http.NewRequest(WithContext)` to a GitHub host outside the approved constructors, **so that** I'm pointed at `github.newGHRequest`/`newGHRequestForHostWithToken` (or their conditional-aware successor) instead of silently skipping ETags.
**Acceptance Criteria**:
- `tools/lint/norawghrequest/analyzer.go` flags any `http.NewRequest`/`http.NewRequestWithContext` call whose URL argument's package-level constant/variable resolves to `github.GhBaseURL()`-derived text, outside `github/http_client.go`'s own constructors.
  - *Given* a hypothetical new file `github/new_feature.go` containing `req, _ := http.NewRequestWithContext(ctx, "GET", GhBaseURL()+"repos/foo/bar", nil)`, *When* `make lint-custom` runs, *Then* it reports `direct call to http.NewRequestWithContext to a GitHub host — use github.newGHRequest()/newGHRequestForHostWithToken() so conditional-request semantics are available; add //nolint:norawghrequest with a justification if this genuinely cannot use the wrapper` at that line.
  - *Given* `github/http_client.go:166`'s own `newGHRequestForHostWithToken` (the approved constructor) calls `http.NewRequestWithContext` directly, *When* the linter runs, *Then* it does not flag that line (self-exempted, same as `norawgitopen`'s `OpenRepo`).
**Files**: `tools/lint/norawghrequest/analyzer.go` (new), `tools/lint/cmd/linter/main.go` (registration), `Makefile` (`lint-custom` target — already runs the multichecker, no new target needed).

##### Task 2.1.1a: Implement the analyzer (mirror `tools/lint/norawgitopen/analyzer.go`'s structure) (~5 min)
- `pkgPath`/exempt-suffix check → AST walk for `*ast.CallExpr` → type-resolve to `net/http.NewRequest`/`NewRequestWithContext` → check the URL arg literal/const for a GitHub-host substring → `nolintcomment.Contains` escape hatch → `pass.Reportf`.
- Files: `tools/lint/norawghrequest/analyzer.go`

##### Task 2.1.1b: Self-exempt `github/http_client.go`'s own constructors (~3 min)
- Add `github/http_client.go`'s `newGHRequestForHostWithToken` call to the exempt list (by function-declaration containment, same technique `norawgitopen` uses for `OpenRepo`).
- Files: `tools/lint/norawghrequest/analyzer.go`

##### Task 2.1.1c: Register the analyzer in the multichecker binary (~3 min)
- Add `norawghrequest.Analyzer` to `tools/lint/cmd/linter/main.go`'s analyzer list, alongside the other 7.
- Files: `tools/lint/cmd/linter/main.go`

##### Task 2.1.1d: Write a golden-file test (violation + exempted case) (~5 min)
- Mirror `norawgitopen`'s own `testdata/` pattern: one file with a flagged violation, one with a `//nolint:norawghrequest`-suppressed call, one with the exempted constructor.
- Files: `tools/lint/norawghrequest/testdata/src/a/a.go` (new), `tools/lint/norawghrequest/analyzer_test.go` (new)

#### Story 2.1.2: Rule doc for human-readable rationale
**As a** developer who trips the linter, **I want** a doc the error message can point to, **so that** I understand *why*, not just *what*.
**Acceptance Criteria**:
- `.claude/rules/norawghrequest.md` exists, glob-scoped to `github/*.go`, explaining the ETag/conditional-request rationale.
  - *Given* a developer's editor shows the lint failure from Task 2.1.1a, *When* they open `.claude/rules/norawghrequest.md`, *Then* it explains, in the same style as `instance-lock-free-reads.md`, why a raw request bypasses conditional-request savings and what to use instead.
**Files**: `.claude/rules/norawghrequest.md` (new).

##### Task 2.1.2a: Write the rule doc (~4 min)
- Files: `.claude/rules/norawghrequest.md`

### Epic 2.2: Explicit-opt-out constructor for future native call sites
**Goal**: A single constructor path for *new* native GitHub HTTP call sites that requires either an `*ETagCache` or an explicit opt-out, so the "conditional by default" discipline extends beyond `GetPRInfoConditional`'s current one caller.

#### Story 2.2.1: `newConditionalGHRequest` helper
**As a** developer adding a new GitHub GET endpoint, **I want** a constructor that forces me to either pass an `*ETagCache` (making the call automatically conditional) or explicitly call a `SkipConditionalRequest()` opt-out, **so that** skipping conditional semantics is a visible, reviewable decision rather than a silent omission.
**Acceptance Criteria**:
- `github.NewConditionalRequest(ctx context.Context, path string, cache *ETagCache) (*http.Request, error)` sets `If-None-Match` when `cache` has a cached entry for `path`'s derived key, and a sibling `github.NewConditionalRequestNoCache(ctx, path string) (*http.Request, error)` exists for the deliberate opt-out (flagged by `norawghrequest`'s allowlist as the *only* other approved constructor besides the cache-backed one).
  - *Given* a new call site calls `github.NewConditionalRequest(ctx, "repos/x/y/issues/1", cache)` and `cache` has no entry yet, *When* the request is built, *Then* no `If-None-Match` header is set (first-fetch case), and the response's `ETag` header should be stored via `cache.set(...)` by the caller (this constructor only builds the request, it does not itself manage cache writes — consistent with `GetPRInfoConditional`'s existing pattern of building the request and handling the response separately).
**Files**: `github/http_client.go`.

##### Task 2.2.1a: Implement `NewConditionalRequest` and `NewConditionalRequestNoCache` (~5 min)
- Files: `github/http_client.go`

##### Task 2.2.1b: Add both to `norawghrequest`'s exempt-call list (~3 min)
- Files: `tools/lint/norawghrequest/analyzer.go`

##### Task 2.2.1c: Unit test both constructors' header behavior (~4 min)
- Files: `github/http_client_test.go`

---

## Phase 3: Priority-aware admission control (flagged OFF)

### Epic 3.1: `RateLimiter` tracks `remaining`/`limit` via a published snapshot
**Goal**: Stop discarding the `remaining`/`limit` headers `Update()` already parses; publish them lock-free.

#### Story 3.1.1: `RateLimiterSnapshot` + `atomic.Pointer` publish, partitioned by resource
**As a** developer building admission control, **I want** `RateLimiter` to expose a lock-free `Snapshot()` keyed by resource (core/search/graphql), **so that** `AdmitOrigin` and the OTel gauge (Task 1.2.1d) read consistent, race-free, *per-resource* state instead of one conflated global reading (pre-mortem P1 #2 — a low-limit `search` response must never overwrite or mask `core`'s numbers).
**Acceptance Criteria**:
- `RateLimiter` gains `snapshot atomic.Pointer[RateLimiterSnapshot]`, where `RateLimiterSnapshot` is
  `{Resources map[string]ResourceQuota; RateLimitedUntil time.Time}` and `ResourceQuota` is
  `{Remaining, Limit int; ResetAt time.Time}`, published unconditionally (not behind the feature flag
  — see the flag-scoping rule below) at the end of every `Update()` call. Each `Update()` call
  overwrites *only* the map entry for that response's own resource
  (`resp.Header.Get("X-RateLimit-Resource")`, already parsed at line 50), leaving every other
  resource's last-known quota untouched.
  - *Given* a response with `X-RateLimit-Resource: core`, `X-RateLimit-Remaining: 1234`,
    `X-RateLimit-Limit: 5000`, `X-RateLimit-Reset: <unix-ts>`, *When* `RateLimiter.Update(resp)` runs
    (with the admission-control flag OFF), *Then* `RateLimiter.Snapshot().Resources["core"].Remaining
    == 1234` and `.Limit == 5000` — the storing side is unconditional per `research/pitfalls.md` §3,
    item 3's explicit statement-level warning.
  - *Given* the sequence: a `search`-resource response (`Remaining: 2, Limit: 30`) immediately
    followed by a `core`-resource response (`Remaining: 4800, Limit: 5000`), *When* both `Update()`
    calls complete, *Then* `RateLimiter.Snapshot().Resources["search"].Remaining == 2` **and**
    `.Resources["core"].Remaining == 4800` — neither entry overwrites the other, closing the exact
    cross-resource-clobbering failure pre-mortem P1 #2 describes. See Task 3.2.1c for the
    `AdmitOrigin`-level regression test built on top of this.
- `go test -race` passes for a test that concurrently calls `Update()` (with interleaved resources)
  from 50 goroutines while another 50 goroutines call `Snapshot()`.
  - *Given* 100 goroutines hammering `Update`/`Snapshot` concurrently for 1 second, with `Update`
    calls randomly split between `core`/`search`/`graphql` resources, *When* `go test -race
    ./github/... -run TestRateLimiterSnapshot_Concurrent` runs, *Then* it reports no data race and no
    panic, and no resource's map entry is ever observed partially written (verified via the
    copy-on-write publish pattern in Task 3.1.1b, not a per-field mutex).
**Files**: `github/rate_limit.go`, `github/rate_limit_test.go`.

##### Task 3.1.1a: Add `RateLimiterSnapshot`/`ResourceQuota` structs + `snapshot atomic.Pointer[RateLimiterSnapshot]` field (~4 min)
- `RateLimiterSnapshot{Resources map[string]ResourceQuota; RateLimitedUntil time.Time}`;
  `ResourceQuota{Remaining, Limit int; ResetAt time.Time}`.
- Files: `github/rate_limit.go`

##### Task 3.1.1b: Publish the snapshot unconditionally at the end of `Update()`, merging by resource (~5 min)
- Add per-resource `remaining`/`limit`/`resetAt` local capture (already parsed at lines 53-64, keyed
  by the existing `resource := resp.Header.Get("X-RateLimit-Resource")` at line 50) into the new
  struct. Because `RateLimiterSnapshot.Resources` is a `map[string]ResourceQuota` and a map cannot be
  mutated in place behind an `atomic.Pointer` without racing readers, publish by copy-on-write: load
  the current snapshot (treat a nil/never-published pointer as an empty map), shallow-copy its
  `Resources` map, set/overwrite only the entry for the current response's `resource` key, then
  `r.snapshot.Store(&RateLimiterSnapshot{Resources: newMap, RateLimitedUntil: r.rateLimitedUntil})` as
  the last line of `Update` — every `return` path in `Update` must still publish, not just the
  fall-through end (this closes the exact statement-level gap `research/pitfalls.md` §3 item 3 warns
  about, now for the per-resource shape).
- Files: `github/rate_limit.go`

##### Task 3.1.1c: Add `Snapshot()` accessor; remove/replace Task 1.2.1e's stub (~3 min)
- `func (r *RateLimiter) Snapshot() RateLimiterSnapshot` dereferences the pointer, returning a
  zero-value struct (`Resources: nil`) if never published — callers doing `Snapshot().Resources["core"]`
  on a nil map get `ResourceQuota{}`'s zero value via Go's safe nil-map-read semantics, not a panic.
- Files: `github/rate_limit.go`

##### Task 3.1.1d: `-race` concurrency test (mandatory merge gate) (~5 min)
- Per `research/build-vs-buy.md` §6's explicit call for "mandatory `-race` coverage for the new priority-tier logic as a merge gate, not a style suggestion."
- Files: `github/rate_limit_test.go`

### Epic 3.2: `AdmitOrigin` decision + flag-gated wiring
**Goal**: Background origins back off before interactive origins fail, gated behind `github:priority-admission-control`.

#### Story 3.2.1: `AdmitOrigin` method, resource-scoped
**As a** developer, **I want** `RateLimiter.AdmitOrigin(origin, resource)` to reject background origins once *that resource's* remaining quota drops below a reserved headroom, **so that** interactive calls keep succeeding and a low-limit resource (e.g. `search`) can't falsely starve or mask a healthy/exhausted `core` bucket (pre-mortem P1 #2).
**Acceptance Criteria**:
- **Resource determination**: `AdmitOrigin`'s only caller in this plan is `rateLimitTransport.RoundTrip` (Task 3.2.2a), which already has the outbound `*http.Request` in scope *before* dispatch — before any response (and thus before any `X-RateLimit-Resource` header) exists. `resource` is therefore derived from the *request*, not passed as an explicit parameter threaded from further up the call stack: a small classifier mirrors GitHub's own per-endpoint resource assignment — `req.URL.Path` containing `"search/"` → `"search"`; `req.URL.Path` ending in `"graphql"` → `"graphql"`; otherwise → `"core"` (verified against this codebase's actual call shapes: `github/repos.go`'s `search/repositories`/`search/issues` paths, and `github/hosts.go:81`'s `GhBaseURL()+"graphql"`). This keeps `AdmitOrigin` itself a pure, easily-testable function taking `resource` explicitly, while avoiding an invasive parameter threaded through every existing call site — there is exactly one caller today.
- `AdmitOrigin(origin CallOrigin, resource string) (bool, string)` returns `(true, "")` for
  `OriginInteractive` whenever `!IsLimited()`; for any other origin, additionally returns `(false,
  "background headroom reserved for resource <resource>")` once
  `Snapshot().Resources[resource].Remaining < Snapshot().Resources[resource].Limit *
  backgroundHeadroomPercent / 100` — the comparison is scoped to `resource`'s own bucket, never to
  any other resource's numbers, and a resource never observed yet (`ok == false` on the map lookup)
  is treated as "no data — admit" rather than "exhausted — reject," since a zero-value `ResourceQuota`
  would otherwise falsely read as `Remaining: 0`.
  - *Given* `Snapshot().Resources["core"] == {Remaining: 400, Limit: 5000}` (8%, below a 10%
    `backgroundHeadroomPercent` threshold) and `IsLimited() == false`, *When*
    `AdmitOrigin(OriginPRStatusPoller, "core")` is called, *Then* it returns `(false, "background
    headroom reserved for resource core")`.
  - *Given* the same `"core"` snapshot, *When* `AdmitOrigin(OriginInteractive, "core")` is called,
    *Then* it returns `(true, "")`.
  - *Given* `Snapshot().Resources["search"] == {Remaining: 2, Limit: 30}` (6.7%, also below
    threshold) **and** `Snapshot().Resources["core"] == {Remaining: 4800, Limit: 5000}` (96%,
    healthy) in the *same* snapshot, *When* `AdmitOrigin(OriginPRStatusPoller, "core")` is called,
    *Then* it returns `(true, "")` — the unhealthy `search` bucket does not leak into a `core`-scoped
    decision. See Task 3.2.1c for the full interleaved-resource regression test.
**Files**: `github/rate_limit.go`.

##### Task 3.2.1a: Implement `AdmitOrigin` and its request-to-resource classifier (~5 min)
- Files: `github/rate_limit.go`

##### Task 3.2.1b: Unit tests for interactive-always-admitted vs. background-headroom-rejected, single resource (~5 min)
- Files: `github/rate_limit_test.go`

##### Task 3.2.1c: Interleaved-resource regression test (mandatory Phase 3 merge gate, alongside Task 3.1.1d's `-race` test) (~5 min)
- Per pre-mortem P1 #2's explicit ask: interleave a low-limit `search`-resource `Update()` call
  (`Remaining: 2, Limit: 30`) with a healthy `core`-resource `Update()` call (`Remaining: 4800,
  Limit: 5000`), in both orderings (search-then-core and core-then-search), and assert
  `AdmitOrigin(OriginPRStatusPoller, "core")` returns `(true, "")` in both cases — the `search`
  bucket's unhealthy ~7% remaining must never leak into a `core`-targeted admission decision. Also
  assert the converse: `AdmitOrigin(OriginPRStatusPoller, "search")` returns `(false, "background
  headroom reserved for resource search")` in both orderings, unaffected by `core`'s healthy numbers.
  This is a **required** Phase 3 merge gate — not optional, not a nice-to-have — listed explicitly in
  Phase 3's Cross-cutting Acceptance alongside Task 3.1.1d.
- Files: `github/rate_limit_test.go`

#### Story 3.2.2: Flag-gated rejection branch in `rateLimitTransport`/`githubTelemetryTransport`
**As an** operator, **I want** the admission-control rejection to only take effect when the flag is on, **so that** the feature ships dark and Tyler flips it manually.
**Acceptance Criteria**:
- `rateLimitTransport.RoundTrip` classifies the outbound request's `resource` (Story 3.2.1's
  classifier) and calls `AdmitOrigin(origin, resource)` only when
  `config.LoadConfig().GetFeatureFlagWithDefault(githubPriorityAdmissionFlagName, false)` is true;
  when false, behavior is byte-identical to today's fail-fast-only check.
  - *Given* the flag is off and `Snapshot().Resources["core"].Remaining` is below headroom for
    `OriginPRStatusPoller`, *When* a poller call targeting a `core` endpoint is made, *Then* it
    proceeds exactly as today (only `IsLimited()` gates it) — `AdmitOrigin` is not even called.
  - *Given* the flag is on and the same low-`core`-remaining state, *When* a poller call targeting a
    `core` endpoint is made, *Then* `rateLimitTransport.RoundTrip` returns an error before dispatching,
    and `github.admission.rejected_total{origin="poller"}` increments.
  - *Given* the flag is on, `Snapshot().Resources["search"]` is exhausted, but the outbound request
    targets a `core` endpoint (e.g. `repos/{owner}/{repo}/pulls/{n}`), *When* a poller call is made,
    *Then* it is admitted — the exhausted `search` bucket does not affect a `core`-targeted request
    (pre-mortem P1 #2, verified by Task 3.2.1c).
**Files**: `github/http_client.go`.

##### Task 3.2.2a: Add the flag-gated `AdmitOrigin` check inside `rateLimitTransport.RoundTrip`, after the existing `IsLimited()` check (~5 min)
- Classify the request's `resource` (Story 3.2.1's path-based classifier) before calling
  `AdmitOrigin(GitHubCallOriginFrom(req.Context()), resource)`.
- Files: `github/http_client.go`

##### Task 3.2.2b: Increment `github.admission.rejected_total` on rejection (~3 min)
- Files: `github/http_client.go` or `github/telemetry_transport.go`

##### Task 3.2.2c: Feature-flag registration, including a scope caveat in its description (~4 min)
- Add `githubPriorityAdmissionFlagName = "github:priority-admission-control"` constant and a
  `knownFeatureFlags` entry (default `false`) in `server/services/feature_flag_service.go`,
  following the exact convention at lines 15-44/127-197. Give the flag's `Description` field an
  explicit, one-line scope caveat, not just a name — e.g. "Protects this stapler-squad instance's
  own interactive GitHub calls only — does not coordinate with other machines sharing the same
  GitHub token." — so the caveat is visible at the exact point Tyler acts (flipping the flag via
  the `GetFeatureFlags`/`UpdateFeatureFlag` RPC's description field), not buried in a doc he won't
  be looking at in that moment.
- **Scope note (pre-mortem P2 #3, deliberate partial address)**: pre-mortem finding #3
  (`implementation/pre-mortem.md`) is that this admission control is per-process only, but nothing
  surfaces that at the point of action — its Prevention text suggests both a one-line flag-UI
  caveat *and* a Phase-1 metric flagging "remaining dropped by more than this instance's own
  tagged calls can account for" as a distinct cross-machine-consumption signal. Only the cheaper
  half — this flag-description caveat — is implemented here. The fuller metric-based detection is
  intentionally left to the deferred cross-machine coordination follow-on project
  (`requirements.md`'s Out of Scope section), not built as part of this task.
- Files: `server/services/feature_flag_service.go`

#### Story 3.2.3: Initialize new gate state from `IsLimited()` at flip time
**As an** operator flipping the flag mid-session, **I want** the gate to not start "cold," **so that** an interactive call isn't double-gated or under-gated in the flip window.
**Acceptance Criteria**:
- Because `AdmitOrigin` reads the *same* `Snapshot()`/`IsLimited()` state `rateLimitTransport` already checks (not a separate parallel state machine), there is no separate "cold start" to initialize — this closes `research/pitfalls.md` §3a's flagged risk by construction rather than by an explicit init step.
  - *Given* `DefaultRateLimiter` is currently limited (`IsLimited() == true`) when the flag flips from false to true, *When* the next call arrives, *Then* the existing `IsLimited()` check (unconditional, unaffected by the flag) still rejects it first — `AdmitOrigin` is never reached for an already-limited state, so there's no window where the two checks disagree.
**Files**: none (documents a design property, not new code) — verified by Task 3.2.2a's ordering (`IsLimited()` check precedes the flag-gated `AdmitOrigin` check, never the reverse).

##### Task 3.2.3a: Add a regression test asserting the check order (`IsLimited()` before `AdmitOrigin`) (~4 min)
- Files: `github/http_client_test.go`

### Epic 3.3: Poller/backlog-sync integration
**Goal**: Background origins actually back off using the new admission decision, matching the pollers' existing "skip the tick" pattern.

#### Story 3.3.1: No poller code changes needed — verify existing skip-on-error path covers rejection
**As a** developer, **I want** to confirm `AdmitOrigin`'s rejection surfaces as the same kind of error `checkAllSessions`/`pollWorktrees` already handle, **so that** no new skip-logic needs to be added to the pollers themselves.
**Acceptance Criteria**:
- A rejected `AdmitOrigin` call returns an error from `rateLimitTransport.RoundTrip` (same shape as today's `IsLimited()` fail-fast error), which `GetPRInfoConditional`/`GetPRForBranchConditional` already propagate up to `fetchAndUpdatePRStatus`'s existing `handleFetchError`/`log.Warn` path — no new code needed in `pr_status_poller.go`/`worktree_pr_poller.go`.
  - *Given* the flag is on and a poller tick's `GetPRInfoConditional` call gets rejected by `AdmitOrigin`, *When* `fetchAndUpdatePRStatus` receives the resulting error, *Then* it falls into the existing `if p.handleFetchError(err) { return }` branch (`session/pr_status_poller.go:349`) exactly as a real GitHub-side rate-limit error would today — confirmed by a test, not just code inspection.
**Files**: `session/pr_status_poller_test.go` (test only — no production code change in this story).

##### Task 3.3.1a: Add an integration test: flag on + low headroom → poller tick skips cleanly (~5 min)
- Files: `session/pr_status_poller_test.go`

---

## Phase 4: `GetPRInfoCtx` GraphQL migration (flagged OFF)

*(Scope reminder: this phase touches only `GetPRInfoCtx`'s own implementation. `GetPRInfoConditional`'s REST+ETag call, and both pollers' fan-out loops, are unchanged — see ADR-001.)*

### Epic 4.1: GraphQL query + field mapping
**Goal**: A `GetPRInfoGraphQL` function producing a `*PRInfo` field-identical to `GetPRInfoCtx`'s gh-CLI output.

#### Story 4.1.1: Single-PR GraphQL query + response decode
**As a** developer, **I want** a GraphQL query replicating `gh pr view --json number,title,...,reviews,reviewDecision,statusCheckRollup`'s fields, **so that** `GetPRInfoCtx` can dispatch to it without changing its callers' expectations.
**Acceptance Criteria**:
- `GetPRInfoGraphQL(ctx, owner, repo string, prNumber int) (*PRInfo, error)` issues one `POST /graphql` via `ghHTTPClient`, decodes `repository(owner,name){ pullRequest(number){ ... } }` into the same `*PRInfo` struct `GetPRInfoCtx` returns today, including `Mergeable` (GraphQL enum `MERGEABLE`/`CONFLICTING`/`UNKNOWN` mapped to gh-CLI's lowercase string equivalents), `Author.login`, and `statusCheckRollup`'s per-check contexts (verified against a live `gh api graphql` introspection query first — see Unresolved Questions).
  - *Given* a real open PR `tstapler/stapler-squad#704` with 2 approvals and passing CI, *When* `GetPRInfoGraphQL(ctx, "tstapler", "stapler-squad", 704)` is called, *Then* the returned `*PRInfo.ApprovedCount == 2` and `.CheckConclusion == "success"`, matching what `GetPRInfoCtx`'s gh-CLI path would return for the same PR at the same moment (verified by Story 4.1.2's parity test, not asserted in isolation).
**Files**: `github/client_graphql.go` (new).

##### Task 4.1.1a: Run a live `gh api graphql` introspection query against `PullRequest.mergeable`, `.author`, and `.commits.nodes.commit.statusCheckRollup.contexts` to confirm exact field/enum shapes (~5 min)
- Resolves the Unresolved Questions item; not a code change, but a required first sub-step before Task 4.1.1b.
- Files: none (research artifact — record findings as a comment in Task 4.1.1b's new file)

##### Task 4.1.1b: Write the GraphQL query string + response struct (~5 min)
- Files: `github/client_graphql.go`

##### Task 4.1.1c: Implement `GetPRInfoGraphQL` decode + field mapping (mergeable enum, author, labels, checks union) (~5 min)
- Files: `github/client_graphql.go`

##### Task 4.1.1d: Wire origin/call-site telemetry onto the GraphQL POST (reuse `githubTelemetryTransport`'s existing hook — no new instrumentation code, since it already rides `ghHTTPClient`) (~3 min)
- Files: `github/client_graphql.go` (verify call-site attribute is set to `"pr.view.graphql"` to distinguish from the gh-CLI path in metrics)

#### Story 4.1.2: Field-by-field parity test
**As a** developer, **I want** a test proving `GetPRInfoGraphQL` and `GetPRInfoCtx` produce byte-identical `PRInfo` for the same real PR, **so that** the flag can eventually default to on without a silent field-mapping regression (per `research/pitfalls.md` §3b's `DerivePRPriority` misclassification risk).
**Acceptance Criteria**:
- A test fetches the same live PR via both paths and asserts `reflect.DeepEqual` (or field-by-field `assert.Equal`) on the resulting `*PRInfo` structs, excluding only fields both paths are expected to legitimately differ on (none currently expected).
  - *Given* a real PR fixture (recorded HTTP/CLI fixtures, not a live network call in CI — see Task 4.1.2b), *When* `TestGetPRInfo_GraphQLParityWithCLI` runs, *Then* both paths' `*PRInfo.CheckConclusion`, `.ReviewDecision`, `.Mergeable`, `.Author`, `.Labels` are asserted equal field-by-field, and the test fails loudly (not silently) on any mismatch.
- This test is a required merge gate for Task 4.2.1a's flag-check wiring — the flag ships OFF regardless, but the test itself must be green before Phase 4 merges.
**Files**: `github/client_graphql_test.go` (new).

##### Task 4.1.2a: Record fixture data (gh-CLI JSON output + GraphQL response) for 2-3 representative PRs covering draft/approved/CI-failing states (~5 min)
- Files: `github/testdata/pr_info_parity/` (new)

##### Task 4.1.2b: Implement the parity test against fixtures (not live network) (~5 min)
- Files: `github/client_graphql_test.go`

### Epic 4.2: Flag-gated dispatch inside `GetPRInfoCtx`
**Goal**: `GetPRInfoCtx` internally routes to GraphQL only when the flag is on; `ETagCache`'s "PR changed" fallback caller is unaffected either way.

#### Story 4.2.1: Flag check inside `GetPRInfoCtx`
**As an** operator, **I want** `GetPRInfoCtx` to dispatch to `GetPRInfoGraphQL` only when `github:graphql-pr-info` is on, **so that** the migration ships dark.
**Acceptance Criteria**:
- `GetPRInfoCtx`'s body becomes: `if config.LoadConfig().GetFeatureFlagWithDefault(githubGraphQLMigrationFlagName, false) { return GetPRInfoGraphQL(ctx, owner, repo, prNumber) }` followed by the existing gh-CLI implementation unchanged.
  - *Given* the flag is off, *When* `GetPRInfoConditional`'s "PR changed" fallback (`github/etag_cache.go:109` or `:122`) calls `GetPRInfoCtx`, *Then* it executes the exact same `gh pr view` shell-out as today — zero behavior change.
  - *Given* the flag is on, *When* the same fallback fires, *Then* `GetPRInfoCtx` returns `GetPRInfoGraphQL`'s result instead, and the OTel span's `call_site` attribute reads `"pr.view.graphql"` (Task 4.1.1d) so the flag-flip window is diagnosable in traces per `research/pitfalls.md` §3c.
**Files**: `github/client.go`.

##### Task 4.2.1a: Add the flag check at the top of `GetPRInfoCtx` (~4 min)
- Files: `github/client.go`

##### Task 4.2.1b: Feature-flag registration (~3 min)
- Add `githubGraphQLMigrationFlagName = "github:graphql-pr-info"` constant + `knownFeatureFlags` entry (default `false`).
- Files: `server/services/feature_flag_service.go`

##### Task 4.2.1c: Regression test: flag off → gh-CLI path unchanged; flag on → GraphQL path invoked (mock both, assert dispatch) (~5 min)
- Files: `github/client_test.go`

---

## Phase 5: Webhook-driven poller invalidation

*(Decision, resolving the adversarial review's Phase 5 Blocker: `PRStatusPoller.fetchAndUpdatePRStatus`
and `WorktreePRPoller.fetchAndStore` both build their own internal
`ctx, cancel := context.WithTimeout(p.ctx, p.config.CallTimeout)` today (verified,
`session/pr_status_poller.go:286`, `session/worktree_pr_poller.go:245`) — there is nothing external
to "tag" before calling them, so Task 5.1.2b's original wording was impossible as written. This plan
chooses **option (a): give both functions a `ctx context.Context` parameter**, over option (b)
(accepting that webhook-triggered refreshes silently inherit the ticker's origin tag), because Phase
5's own cross-cutting acceptance criterion requires webhook-triggered refreshes to be observable as
distinct from ticker refreshes in telemetry — (b) would make that criterion permanently
unsatisfiable. The blast radius is minimal: `grep -rn "fetchAndUpdatePRStatus(\|fetchAndStore("
session/*.go` (excluding tests) shows exactly **one** production caller of each — the ticker's own
fan-out loop (`pr_status_poller.go:257`, `worktree_pr_poller.go:228`) — so this is a one-caller-each
signature change, not a fan-out restructuring. See the corrected Tech Debt Disposition entry below
for the resulting, now-accurate justification text.)*

### Epic 5.1: `ETagCache.Invalidate` + `PRStatusPoller.InvalidateAndRefresh`
**Goal**: A webhook delivery for a tracked PR triggers an immediate, targeted refresh instead of waiting up to 60s for the next tick.

#### Story 5.1.1: `ETagCache.Invalidate`
**As a** developer, **I want** a safe way to drop a cached ETag entry, **so that** the next fetch for that PR is unconditional (forces a fresh 200, not a stale 304).
**Acceptance Criteria**:
- `ETagCache.Invalidate(owner, repo string, prNumber int)` deletes the entry via `sync.Map.Delete`, safe to call concurrently with `get`/`set`.
  - *Given* `ETagCache` has a cached entry for `("tstapler", "stapler-squad", 704)`, *When* `Invalidate("tstapler", "stapler-squad", 704)` is called, *Then* the next `GetPRInfoConditional` call for that key sends no `If-None-Match` header and receives a full `200` response.
**Files**: `github/etag_cache.go`.

##### Task 5.1.1a: Implement `Invalidate` (~3 min)
- Files: `github/etag_cache.go`

##### Task 5.1.1b: Unit test invalidate-then-refetch produces no `If-None-Match` (~4 min)
- Files: `github/etag_cache_test.go`

#### Story 5.1.2: `PRStatusPoller.InvalidateAndRefresh`
**As a** developer, **I want** a webhook event to trigger `PRStatusPoller`'s existing per-instance fetch logic immediately, **so that** session-backed PRs refresh without waiting for the ticker.
**Acceptance Criteria**:
- `PRStatusPoller.InvalidateAndRefresh(ctx context.Context, owner, repo string, prNumber int) (matched bool)` invalidates the shared `ETagCache`, linear-scans `GetInstances()` for a matching `(GitHubOwner, GitHubRepo, GitHubPRNumber)` triple, and if found, calls `fetchAndUpdatePRStatus` for each match in a goroutine (plural, per `research/architecture.md` §3's note that two `*Instance`s can track the same PR).
  - *Given* `PRStatusPoller` tracks one `*Instance` with `GitHubOwner="tstapler", GitHubRepo="stapler-squad", GitHubPRNumber=704`, *When* `InvalidateAndRefresh(ctx, "tstapler", "stapler-squad", 704)` is called, *Then* it returns `matched=true` and `fetchAndUpdatePRStatus` runs for that instance within the same call (dispatched to a goroutine, not blocking the webhook handler's response).
  - *Given* no tracked instance matches, *When* `InvalidateAndRefresh` is called, *Then* it still invalidates the cache entry (harmless no-op on a `sync.Map` miss) and returns `matched=false` — a documented no-op, not an error, per `research/features.md` §2's edge case.
**Files**: `session/pr_status_poller.go`.

##### Task 5.1.2a-prime: Add `ctx context.Context` as `fetchAndUpdatePRStatus`'s first parameter; update its ticker fan-out call site (~4 min)
- Change `func (p *PRStatusPoller) fetchAndUpdatePRStatus(inst *Instance)` to
  `func (p *PRStatusPoller) fetchAndUpdatePRStatus(ctx context.Context, inst *Instance)`, moving both
  the `ctx, cancel := context.WithTimeout(p.ctx, p.config.CallTimeout)` construction (currently the
  function's first line, `pr_status_poller.go:286`) *and* Task 1.1.3a's `ctx =
  github.WithGitHubCallOrigin(ctx, github.OriginPRStatusPoller)` tagging line out to its one caller
  (`pr_status_poller.go:257`, inside the ticker's fan-out loop), which builds and tags `ctx` exactly
  as `fetchAndUpdatePRStatus` did internally before, then passes it in. This is the signature change
  the Tech Debt Disposition table's "Extend as-is" entry now explicitly accounts for (see below).
- Files: `session/pr_status_poller.go`

##### Task 5.1.2a: Implement `InvalidateAndRefresh` (~5 min)
- Calls `fetchAndUpdatePRStatus(ctx, inst)` for each matched instance (per Task 5.1.2a-prime's new
  signature), inside its own goroutine, not blocking the webhook handler's response.
- Files: `session/pr_status_poller.go`

##### Task 5.1.2b: Build the dispatched `ctx` tagged `OriginWebhookReconcile` before calling `fetchAndUpdatePRStatus` (~3 min)
- `InvalidateAndRefresh` builds its own `ctx, cancel := context.WithTimeout(p.ctx, p.config.CallTimeout)`
  (matching the ticker path's timeout), tags it `github.WithGitHubCallOrigin(ctx, github.OriginWebhookReconcile)`,
  and passes that into each `fetchAndUpdatePRStatus(ctx, inst)` call — now possible because Task
  5.1.2a-prime made `ctx` a real parameter instead of an internal-only local.
- Files: `session/pr_status_poller.go`

##### Task 5.1.2c: Unit test matched/unmatched cases, including the plural-match case (two instances, same PR) (~5 min)
- Files: `session/pr_status_poller_test.go`

### Epic 5.2: `WorktreePRPoller.InvalidateCache` (cache-only)
**Goal**: Worktree-only PRs get their cache entry cleared so the *next* natural trigger (ticker or `ScanDone()`) fetches fresh — no immediate refetch, per the design decision below.

#### Story 5.2.1: Cache-only invalidation for `WorktreePRPoller`
**As a** developer, **I want** `WorktreePRPoller` to clear its stale cache entry on a webhook event, **so that** the next tick or scan doesn't serve a 304-cached stale value, without building a persistent per-worktree index this poller doesn't otherwise need.
**Acceptance Criteria**:
- `WorktreePRPoller.InvalidateCache(owner, repo string, prNumber int)` calls the shared `ETagCache.Invalidate` only — no immediate `fetchAndStore` dispatch, accepting up to one `PollInterval` (60s, unchanged) of staleness for the worktree-only case.
  - *Given* a worktree with no live session tracks PR 42 in `acme/widgets`, *When* a webhook event fires for that PR and `InvalidateCache("acme", "widgets", 42)` is called, *Then* the shared `ETagCache` entry is cleared, and the *next* scheduled tick (ticker or `ScanDone()`) performs a full fetch instead of a 304 — verified by asserting the cache miss on the next `fetchAndStore` call in a test, not by asserting an immediate refetch happened.
  - **Design rationale** (per `research/architecture.md` §3's flagged decision): `WorktreePRPoller` has no persistent per-item lookup the way `PRStatusPoller.GetInstances()` does — its worktrees are re-derived fresh every tick from `p.source.GetWorktrees()`. Building a `RefreshOne(ctx, repoPath, branch)` entry point purely for this case is not justified: worktree-only PRs have no interactive session watching them, so up to 60s of added staleness is an acceptable, explicitly accepted trade-off, not a gap.
**Files**: `session/worktree_pr_poller.go`.

##### Task 5.2.1a-prime: Add `ctx context.Context` as `fetchAndStore`'s first parameter; update its ticker fan-out call site (~4 min)
- For consistency with Task 5.1.2a-prime's signature change (both entries share one Tech Debt
  Disposition row) — change `func (p *WorktreePRPoller) fetchAndStore(item WorktreeScanItem)` to
  `func (p *WorktreePRPoller) fetchAndStore(ctx context.Context, item WorktreeScanItem)`, moving both
  the `ctx, cancel := context.WithTimeout(p.ctx, p.config.CallTimeout)` construction (currently
  `worktree_pr_poller.go:245`) *and* Task 1.1.3b's `OriginWorktreePRPoller` tagging line out to its
  one caller (`worktree_pr_poller.go:228`), which builds and tags `ctx` exactly as before, then
  passes it in. Note this signature change is not
  exercised by any `OriginWebhookReconcile`-tagged call today — `InvalidateCache` (Task 5.2.1a) stays
  cache-only, per Story 5.2.1's own design rationale (no persistent per-worktree index to target an
  immediate refetch at) — it's made purely for symmetry with `fetchAndUpdatePRStatus` and to leave
  the door open for a future `RefreshOne`-style entry point without another signature-breaking change.
- Files: `session/worktree_pr_poller.go`

##### Task 5.2.1a: Implement `InvalidateCache` (~3 min)
- Files: `session/worktree_pr_poller.go`

##### Task 5.2.1b: Unit test: invalidate → next `fetchAndStore` call sees a cache miss (~4 min)
- Files: `session/worktree_pr_poller_test.go`

### Epic 5.3: Webhook handler wiring
**Goal**: `GitHubWebhookHandler` calls the new invalidation path as a second consumer of the same 4 already-verified event types, alongside the existing `TriggerPRFixForEvent` call — no new route, no new signature verification.

#### Story 5.3.1: `GitHubPollerInvalidator` interface + `pollerInvalidationAdapter`
**As a** developer, **I want** a narrow interface in `server/services` satisfied by a `session`-package adapter, **so that** `GitHubWebhookHandler` doesn't import `session.PRStatusPoller`/`WorktreePRPoller` concrete types directly — mirroring `PRFixEventRouter`'s existing shape.
**Acceptance Criteria**:
- **Signature decision** (resolves the adversarial review's Story 5.3.1/Task 5.3.1c concern): the
  batch-shaped `InvalidateForEvent(ctx, repoFullName string, prNumbers []int)` originally specified
  here conflicted with its actual call site — `handlePRFixEvent`'s existing loop
  (`server/services/github_webhook_pr_fix.go:558-568`, verified by direct read) is
  `for _, prNumber := range prNumbers { h.prFixRouter.TriggerPRFixForEvent(ctx, fullName, prNumber) }`,
  a per-single-`int` loop, because `TriggerPRFixForEvent` itself takes one `int`, not a slice. This
  plan chooses **option (a): a single-`prNumber int` signature**, mirroring `TriggerPRFixForEvent`'s
  shape it sits right next to, over option (b) (calling a batch-shaped method once outside the loop)
  — because Task 5.3.1c already wires the new call directly alongside `TriggerPRFixForEvent` inside
  that per-PR loop (both calls naturally happen at the same iteration, right next to the existing
  `persistTriggerFireEvent` call that's also per-PR), so matching its shape avoids constructing a
  throwaway `[]int{prNumber}` on every iteration.
- `server/services/github_webhook_handler.go` defines `type GitHubPollerInvalidator interface { InvalidateForEvent(ctx context.Context, repoFullName string, prNumber int) }`; `session.pollerInvalidationAdapter` (wrapping both pollers) satisfies it.
  - *Given* `pollerInvalidationAdapter{prPoller, worktreePoller}` and a call `InvalidateForEvent(ctx, "tstapler/stapler-squad", 704)`, *When* it runs, *Then* it parses `repoFullName` into `(owner, repo)`, calls `prPoller.InvalidateAndRefresh(ctx, owner, repo, 704)`, and — regardless of whether that matched — also calls `worktreePoller.InvalidateCache(owner, repo, 704)` (both pollers' caches share one `*github.ETagCache`, so this is a harmless double-invalidate of the same underlying entry, not two separate cache systems to keep in sync).
**Files**: `server/services/github_webhook_handler.go`, `session/poller_invalidation_adapter.go` (new).

##### Task 5.3.1a: Define `GitHubPollerInvalidator` interface in `server/services` (~3 min)
- Files: `server/services/github_webhook_handler.go`

##### Task 5.3.1b: Implement `session.pollerInvalidationAdapter` (~5 min)
- Files: `session/poller_invalidation_adapter.go` (new)

##### Task 5.3.1c: Wire `InvalidateForEvent` into `handlePRFixEvent`, alongside the existing `TriggerPRFixForEvent` call, reusing `extractPRFixEvent`'s already-parsed `(repoFullName, prNumbers)` (~5 min)
- Add the call inside `server/services/github_webhook_pr_fix.go:558-568`'s existing
  `for _, prNumber := range prNumbers` loop, right next to the existing
  `h.prFixRouter.TriggerPRFixForEvent(ctx, fullName, prNumber)` call — same per-iteration shape, one
  `prNumber int` per call, per Story 5.3.1's signature decision above. Tag the adapter's internal
  outbound calls `OriginWebhookReconcile`.
- Files: `server/services/github_webhook_pr_fix.go`

##### Task 5.3.1d: Add `prPollerInvalidator GitHubPollerInvalidator` field + constructor parameter to `GitHubWebhookHandler` (~4 min)
- Files: `server/services/github_webhook_handler.go`

##### Task 5.3.1e: Wire the adapter at `GitHubWebhookHandler`'s real construction site, `server/server.go:846` (~5 min)
- `server/dependencies.go:400`/`:1135` (`session.NewPRStatusPoller(...)`/`session.NewWorktreePRPoller(...)`)
  are the *pollers'* own construction sites, not `GitHubWebhookHandler`'s — verified by reading both
  files. `GitHubWebhookHandler` is actually constructed at
  `server/server.go:846`: `services.NewGitHubWebhookHandler(deps.WorkflowRepo, deps.WorkflowScheduler,
  deps.TriggerFireEventRepo, webhookCfg, prFixRouter)`, inside the `if webhookCfg.GetFeatureFlag("webhook_triggers")`
  block. Construct `session.pollerInvalidationAdapter{prStatusPoller, worktreePRPoller}` from the
  already-in-scope `deps.PRStatusPoller`/`deps.WorktreePRPoller` (or equivalent `ServerDependencies`
  fields — verify their exact field names before writing this task's implementation) and pass it as
  `NewGitHubWebhookHandler`'s new final parameter at this call site.
- Also update this constructor's test doubles: `server/services/github_webhook_handler_test.go` and
  `server/services/github_webhook_pr_fix_test.go` (verified by grep — these are the two files under
  `server/services/` that reference `NewGitHubWebhookHandler`/`GitHubWebhookHandler` in tests) need a
  `GitHubPollerInvalidator` argument (real or spy) added everywhere they construct a
  `GitHubWebhookHandler` directly, or the package will not compile once the constructor's signature
  changes.
- Files: `server/server.go`, `server/services/github_webhook_handler_test.go`, `server/services/github_webhook_pr_fix_test.go`

##### Task 5.3.1f: Integration test: a `check_run` webhook delivery for a tracked PR triggers `InvalidateAndRefresh`, verified via a spy `GitHubPollerInvalidator` (~5 min)
- Files: `server/services/github_webhook_pr_fix_test.go`

##### Task 5.3.1g: Add a handler-level log line for the `pr_event_webhooks`-off silent no-op (pre-mortem P2 #4) (~3 min)
- `handlePRFixEvent`'s early return (`server/services/github_webhook_pr_fix.go:461`) currently 200s
  silently when `pr_event_webhooks` is off, with zero log signal anywhere — the reachable-handler
  case (`webhook_triggers` on, so this code executes at all) has no equivalent to
  `server/server.go:863`'s existing warning for the *other* flag combination. Add a log line inside
  that early return, gated so it fires only when the request actually reached the handler (proving
  `webhook_triggers` is on) and `pr_event_webhooks` is off, identifying that the delivery was
  accepted (`200 OK`) but silently dropped because `pr_event_webhooks` is disabled — so an operator
  debugging "PR status still lags after Phase 5 shipped" finds a paper trail instead of clean logs.
  Dedupe it (e.g. a `sync.Once`-guarded first-occurrence log, mirroring `firstPRFixDelivery`'s
  existing once-per-boot pattern in the same file) so a live but misconfigured webhook tunnel doesn't
  spam one line per delivery.
- Files: `server/services/github_webhook_pr_fix.go`

##### Task 5.3.1h: Unit test: `pr_event_webhooks` off logs the new warning once, not per-delivery (~4 min)
- Files: `server/services/github_webhook_pr_fix_test.go`

### Epic 5.4: Webhook-delivery-staleness metric
**Goal**: Detect a silently-broken webhook tunnel before the (still-conservative, unchanged) fallback ticker is the only thing standing between it and stale PR status.

#### Story 5.4.1: `github.webhook.last_delivery_age_seconds` gauge
**As an** operator, **I want** a metric showing how long since the last verified webhook delivery of each type, **so that** a broken tunnel is observable, not silently indistinguishable from "nothing changed."
**Acceptance Criteria**:
- Each of the 4 tracked event types updates a `time.Time` (atomic) on every *verified* delivery (reusing the existing `firstPRFixDelivery`-adjacent verification point, not the raw inbound request); an `Int64ObservableGauge` callback reports `time.Since(lastSeen).Seconds()` per `event_type`.
  - *Given* a `workflow_run` delivery was last verified 3 hours ago and none have arrived since, *When* the metric is scraped, *Then* `github.webhook.last_delivery_age_seconds{event_type="workflow_run"} ≈ 10800`.
**Files**: `server/services/github_webhook_pr_fix.go`.

##### Task 5.4.1a: Track last-verified-delivery timestamp per event type (~4 min)
- Add a `map[string]*atomic.Int64` (unix-nano) alongside `firstPRFixDelivery`, updated at the same point that map's `once.Do` fires (verified-delivery boundary).
- Files: `server/services/github_webhook_pr_fix.go`

##### Task 5.4.1b: Register the observable gauge (~4 min)
- Files: `server/services/github_webhook_pr_fix.go` or `github/telemetry_transport.go`'s meter-registration site

##### Task 5.4.1c: Unit test the timestamp updates on verified delivery, not on rejected/unverified ones (~4 min)
- Files: `server/services/github_webhook_pr_fix_test.go`

---

## Phase 6: Frontend GitHub rate-limit error copy (small, independently mergeable)

### Epic 6.1: Reason-coded friendly error copy
**Goal**: Replace the raw backend rate-limit string with a two-bucket friendly message, reusing existing UI scaffolding — no new components.

#### Story 6.1.1: `GitHubRateLimitReason` + `getGitHubRateLimitMessage`
**As** Tyler, **I want** a rate-limited GitHub action to show "try again in ~N minutes" instead of a raw ISO timestamp string, **so that** I don't have to do mental math mid-task.
**Acceptance Criteria**:
- Backend errors originating from `rateLimitTransport`'s fail-fast (`github/http_client.go:49`) or `AdmitOrigin`'s rejection are classified server-side into `"transient"` (secondary rate limit, resets in under a threshold) vs. `"exhausted"` (primary rate limit) before reaching the RPC response's `error` string; the frontend's `getGitHubRateLimitMessage` maps each to plain language with a relative time via the existing `formatRelativeTime` utility.
  - *Given* a `VcsPanel.tsx` error state populated from `useSessionVcs.ts:87-88`'s `setError(new Error(response.error))` where `response.error` now carries a `reason=exhausted` marker and a reset timestamp, *When* `VcsPanel.tsx` renders its error box (`VcsPanel.tsx:45-51`), *Then* it shows `"GitHub rate limit reached — try again in ~4 minutes"` instead of the raw `"github: rate limited until 2026-09-08T15:23:00Z..."` string, using `formatRelativeTime` for the "~4 minutes" part.
- The changed error copy carries `role="status" aria-live="polite"`, matching `VcsWidget.tsx:70-79`'s existing live-region pattern.
  - *Given* the error box re-renders with new reason-coded copy, *When* a screen reader is active, *Then* it announces the new message without requiring the user to refocus, verified by the presence of `role="status" aria-live="polite"` on the containing element.
- `VcsWidgetComments.tsx:108`'s `"Failed to load comments"` gets the same reason-code treatment (its `catch` block currently discards the error entirely at line 59-63).
- Per `design/ux.md`'s "No dead ends" acceptance criterion: `VcsWidgetComments.tsx`'s failure state
  gains a visible, keyboard-reachable Retry action (today it has none — a true dead end), reachable
  in ≤ 1 click, matching `VcsPanel.tsx`'s existing Retry button's behavior (re-invokes the failed
  fetch on click).
**Files**: `web-app/src/lib/vcs/githubRateLimit.ts` (new), `web-app/src/components/sessions/VcsPanel.tsx`, `web-app/src/components/shared/vcs-widget/VcsWidgetComments.tsx`, `web-app/src/lib/hooks/useSessionVcs.ts`.

##### Task 6.1.1a: Define `GitHubRateLimitReason` type + `getGitHubRateLimitMessage` (~4 min)
- Files: `web-app/src/lib/vcs/githubRateLimit.ts` (new)

##### Task 6.1.1b: Backend: classify rate-limit errors into `reason=transient|exhausted` before returning them over RPC (~5 min)
- At the point `rateLimitTransport`'s error (or `AdmitOrigin`'s rejection) surfaces to a `GitHubService` RPC handler, wrap it with a structured marker (e.g. a typed error or a prefix convention) the RPC layer already has precedent for (check existing `connect.Code`/error-detail usage in `server/services/github_service.go` before inventing a new mechanism).
- Files: `server/services/github_service.go`

##### Task 6.1.1c: Update `VcsPanel.tsx`'s error box to use `getGitHubRateLimitMessage` when the error carries a reason code (~4 min)
- Files: `web-app/src/components/sessions/VcsPanel.tsx`

##### Task 6.1.1d: Add `role="status" aria-live="polite"` to the error box (~2 min)
- Files: `web-app/src/components/sessions/VcsPanel.tsx` (`VcsPanel.css.ts` if a new class is needed)

##### Task 6.1.1e: Apply the same treatment to `VcsWidgetComments.tsx`'s `"Failed to load comments"`, and add the Retry action `design/ux.md` requires (~6 min)
- Update the `catch` block (`VcsWidgetComments.tsx:59-63`) to capture and classify the error instead
  of discarding it, then render via `getGitHubRateLimitMessage` when applicable, falling back to the
  existing generic string otherwise (State C, unchanged copy, per `design/ux.md` §2).
- Per `design/ux.md` §2/§4/§5 (Acceptance Criteria 1 and 8): today this surface has **no retry
  action at all** on failure — a true dead end for the account-wide-exhausted case. Add a real
  `<button>` (not a `<div onClick>`, keyboard-reachable via Tab/Enter/Space) labeled "Retry" to the
  error state, wired to reset `fetchedRef.current` and re-invoke `fetchComments()` — the same pattern
  today's only escape hatch (collapsing/re-expanding the section) does implicitly, made explicit and
  visible. This closes `design/ux.md`'s "No dead ends" acceptance criterion for this surface, which
  is not yet met and which Epic 6.1 must close per that doc.
- Add `role="status" aria-live="polite"` to this surface's error container too (mirroring Task
  6.1.1d's treatment of `VcsPanel.tsx`, per `design/ux.md` §5's Acceptance Criterion 5, which applies
  to both surfaces, not just `VcsPanel.tsx`).
- Files: `web-app/src/components/shared/vcs-widget/VcsWidgetComments.tsx`

##### Task 6.1.1f: Frontend unit tests for both reason buckets + fallback to generic copy for a non-rate-limit error (~5 min)
- Files: `web-app/src/lib/vcs/githubRateLimit.test.ts` (new)

---

## Success Metrics Traceability

*(Added during Phase 4 cross-artifact review, 2026-09-09 — the adversarial review already
recommended this and it was missed in the first repair pass; restated here plainly rather than
left implicit across ADR-001/Step 0.5/Pattern Decisions.)*

- **Success Metric #1** ("zero interactive failures from same-process background exhaustion"):
  delivered by Phase 3, but only once `github:priority-admission-control` is manually flipped on
  (default OFF). At merge time, this metric shows **no movement** — the mechanism exists but is
  dark. Its fallback half ("clearer, more actionable message" when a call genuinely can't
  proceed) is delivered by Phase 6, unflagged, so that half *does* land at merge time regardless
  of the Phase 3 flag.
- **Success Metric #2** ("reduced steady-state call volume"): **not delivered by anything this
  plan ships at merge time.** Tracing every lever requirements.md names for it: GraphQL batching
  is deferred entirely (ADR-001 — call-count-per-tick is unchanged by this project, it was never
  the lever); Phase 2's conditional-requests-by-default infrastructure applies only to *future*
  call sites, not any call site that exists today; the fallback-ticker widening Scope item 4
  names as webhook invalidation's actual volume-reduction mechanism is explicitly *not* shipped
  (Risk Control keeps it at today's 60s regardless of webhook activity, per the user's own
  2026-09-08 decision); and Phase 4's GraphQL migration changes call *shape/cost*, not *count*,
  and ships flagged OFF besides. **Net effect: steady-state call volume is expected to be
  unchanged at the end of this plan.** Moving it requires, as explicit follow-on work not
  contained in this plan: (1) flipping both feature flags per the dated, metric-grounded triggers
  now specified in Risk Control's "Staged rollout" bullet (pre-mortem P1 #1 — no longer an
  indefinite "after observing metrics"), and
  (2) a separate future change actually widening the fallback ticker once webhook-invalidation
  reliability is proven. This is a deliberate, reasoned deferral (see ADR-001 and Rabbit
  Holes/Feasibility Risks in requirements.md), not an oversight — but a reviewer reading only
  this plan's phase list should not come away believing call volume drops on merge.

---

## Cross-cutting acceptance: what "done" means per phase

- **Phase 1**: `make ci` green; a manual `stapler-squad-otel` run (per `docs/how-to/enable-otel-auto-instrumentation.md`) shows spans for at least one native HTTP call and one gh-CLI call, each with `github.call.origin`/`github.call_site` attributes populated.
- **Phase 2**: `make lint-custom` includes `norawghrequest`; a deliberately-introduced raw `http.NewRequest` to a GitHub host in a scratch branch is caught by CI.
- **Phase 3**: `go test -race ./github/...` green, **including Task 3.2.1c's interleaved-resource regression test** alongside Task 3.1.1d's `-race` test — both are required merge gates, neither optional (pre-mortem P1 #2); flag defaults to `false` in `GetFeatureFlags`' RPC response; manually flipping it via `UpdateFeatureFlag` and re-running a poller tick under simulated low headroom shows the poller skip in logs/metrics, scoped to the resource that poller tick actually targets.
- **Phase 4**: `github.client_graphql_test.go`'s parity test green; flag defaults to `false`; manually flipping it and calling `GetPRInfoCtx` for a real PR returns a `*PRInfo` indistinguishable from the gh-CLI path's (per the parity test, not a one-off manual check).
- **Phase 5**: A real `check_run` webhook delivery (or a replayed fixture) for a tracked PR causes `PRStatusPoller.InvalidateAndRefresh` to fire within the same request, observable via the new span/log line, without waiting for the 60s ticker.
- **Phase 6**: A simulated rate-limited RPC response in `web-app`'s dev environment renders the friendly message, not the raw backend string, in both `VcsPanel.tsx` and `VcsWidgetComments.tsx`.
