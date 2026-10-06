# Requirements: github-call-optimization

**Date**: 2026-09-08
**Type**: feature addition (cross-cutting refactor of an existing subsystem)
**Complexity**: 4 — high-stakes / cross-cutting

## Problem Statement

stapler-squad talks to GitHub through three divergent, uncoordinated paths — native HTTP
(`github/http_client.go`, rate-limited + ETag-cached), 22 `gh` CLI subprocess sites
(`github/client.go`, `session/git/worktree_git.go`, and 3 one-off sites — none rate-limited,
cached, or traced), and a webhook receiver (`server/services/github_webhook_handler.go`) that's
wired only to the autonomous PR-fix loop, not to the PR-status pollers. There is no distinction
between an interactive, user-initiated GitHub call (merge, comment, refresh, view PR) and a
background one (the two PR-status pollers, backlog issue sync) — they share one global
`DefaultRateLimiter` with no priority separation. When background usage exhausts the shared
token's quota, interactive actions fail with the identical "rate limited until X" error a
background poller would get, instead of the background paths absorbing the wait.

For Tyler, running this against his own GitHub account/token — **across multiple machines that
share the same token**, not just multiple sessions on one host — this has already caused
user-visible breakage (see Baseline), not just a theoretical inefficiency. That multi-machine
fact matters architecturally: `DefaultRateLimiter` and every mitigation in this project are
per-process state. A rate limiter on machine A has no visibility into quota machine B just spent
against the same shared token, so this project's priority-admission-control workstream can only
ever prioritize *within* one instance — it cannot prevent one machine's background traffic from
starving another machine's interactive request. Closing that gap for real requires actual
cross-machine coordination, which is deferred to a follow-on project (see Out of Scope and Open
Questions).

## Baseline

Observed, not hypothetical — this has already surfaced three ways in the live deployed instance:

- **PR status went stale/wrong**: session cards and the diff viewer show outdated
  priority/checks because a poller tick got rate-limited and silently skipped
  (`checkAllSessions`/`pollWorktrees`'s `IsLimited()` early-return in
  `session/pr_status_poller.go:205` and `session/worktree_pr_poller.go:187`).
- **Interactive action failed live**: a user-initiated merge/comment/refresh/view got the
  "rate limited until X" error in the moment (`github/http_client.go:48`'s fail-fast), with no
  distinction from a background-caused exhaustion.
- **Backlog/issue sync stalled**: GitHub-issue-backed backlog items stopped syncing/updating
  when the shared token's quota was exhausted by other traffic.

Today, the only mitigation is the global `DefaultRateLimiter` fail-fast plus each poller's
own backoff (`NoPRBackoff`) — both react *after* the token is already exhausted, and neither
protects interactive requests from background-caused exhaustion.

## Users / Consumers

- Tyler, using the web UI's session cards, diff viewer, and PR actions (merge/comment/close)
  interactively.
- The background subsystems themselves: `PRStatusPoller`, `WorktreePRPoller`,
  `GitHubIssuesPlugin` (backlog sync), and the webhook-driven PR-fix auto-loop
  (`server/services/github_webhook_pr_fix.go`).
- Future GitHub-calling code in this codebase — the "conditional requests by default"
  infrastructure requirement exists specifically so the next call site someone adds doesn't
  reintroduce this problem.

## Success Metrics

**Scoped to a single instance/process** — neither metric claims to detect or prevent a *different
machine* (sharing the same token) causing the exhaustion; that cross-machine case is explicitly
deferred (see Out of Scope).

1. **Zero interactive failures caused by same-process background exhaustion.** An interactive
   RPC (`GetPRInfo`, `GetPRComments`, `PostPRComment`, `MergePR`, `ClosePR`) never returns the
   "rate limited" error in a window where the limiting response was set by a background-origin
   call (poller/backlog-sync/webhook-reconcile) *on the same instance*. Measured via the
   origin-tagged metrics from the observability workstream (below) — this needs to be provable,
   not just believed. The observability workstream's per-origin/per-repo call-rate data is also
   what will tell us, empirically, how much of the *original* pain (Baseline) was same-process
   vs. cross-machine — informing how urgent the follow-on cross-machine project actually is.
2. **Reduced steady-state call volume** — split into two parts, per `implementation/plan.md`'s
   "Success Metrics Traceability" section (added during Phase 4 cross-artifact review,
   2026-09-09), so this document doesn't claim more than the plan itself backs up:
   - **(2a) Infrastructure this project delivers**, roughly equal weight with Metric #1:
     conditional-requests-by-default plumbing (Scope §5, so a future call site structurally can't
     skip ETag/304 semantics), the `GetPRInfoCtx` GraphQL migration's cost/shape improvement
     (Scope §3 — one round trip instead of a `gh` subprocess for reviews+checks+metadata, though
     shipped flagged OFF), and the Phase-1 observability metrics (`github.calls_total`,
     `github.rate_limit.remaining`, `github.cache.result_total`) that make a future volume
     reduction measurable at all. This is what actually ships and is exercised by `make ci` at
     merge time.
   - **(2b) Actual measured volume reduction** — total GitHub API calls/hour dropping measurably
     at steady state. **Not claimed as delivered by this project alone**, and not co-equal with
     Metric #1 or (2a). Per plan.md's Traceability section: GraphQL batching is deferred entirely
     (ADR-001 — call count per tick is unchanged), the fallback ticker stays at today's 60s
     regardless of webhook-invalidation activity (Risk Control, user decision 2026-09-08), and
     both feature flags (`github:priority-admission-control`, `github:graphql-pr-info`) ship OFF.
     Realizing (2b) requires follow-on work this project does not itself perform: manually
     flipping both flags per the dated, metric-grounded triggers in plan.md's Risk Control
     "Staged rollout" section, and a separate future change actually widening the fallback ticker
     once webhook-invalidation reliability is proven. Net effect at merge: steady-state call
     volume is expected to be **unchanged**, exactly as plan.md's Traceability section states.

## Appetite

**Large (3–6 weeks)**, shipped as staged, independently-mergeable PRs per workstream — not one
big-bang change. If real effort exceeds this, cut a workstream (see Rabbit Holes for which ones
are safest to defer), don't extend the timeline silently.

## Constraints

- No hard deadline. No compliance requirement — this is Tyler's own deployment (multiple
  machines, one GitHub token/account, not a multi-tenant service).
- Must not regress the already-solid parts of the current design: single shared ticker per
  poller (not per-session goroutines), the shared `ETagCache` reused across both pollers
  (`session/pr_status_poller.go:107`, explicitly to avoid doubling call volume per ADR-022),
  singleflight-coalesced token/auth caching (`github/http_client.go:96-136`,
  `github/client.go:172`).
- Risk control: gate the two highest-blast-radius changes — priority-aware admission control,
  and the `GetPRInfoCtx` GraphQL migration — behind the existing live-settable rollout-flag
  mechanism (`server/features/flags.go`'s pattern, same as the BackendTymux canary flag), not an
  env var, so either can be flipped off without a redeploy if it misbehaves. A bug in the
  rate-limiter's priority logic has real blast radius: it's shared across every session on one
  token.

## Non-functional Requirements

- **Performance SLO**: not specified numerically. Directionally: interactive PR actions should
  never wait behind a background call's backoff; background paths should never block longer than
  their own existing backoff windows (`NoPRBackoff`, 15-min backlog-sync interval) plus whatever
  headroom the priority admission control reserves.
- **Scalability**: current scale (single user, N concurrent sessions/worktrees across multiple
  machines, all sharing one GitHub token/PAT). Not designed for multi-tenant/multi-org scale.
  This project's mitigations are per-process/per-instance only; the cross-machine dimension of
  that same shared-token reality is the deferred follow-on project's job (see Out of Scope).
- **Security classification**: internal. No new secrets; reuses existing token resolution
  (`getGHToken`/`getGHTokenForAccount`).
- **Data residency**: not applicable.

## Scope

### In Scope

1. **Observability**: OTel spans + metrics on every outbound GitHub call path — both
   `ghHTTPClient`'s `RoundTripper` and a new single instrumented wrapper collapsing the 22 raw
   `gh` CLI subprocess sites. Attributes/tags: call-site, origin (`interactive` / `poller` /
   `backlog_sync` / `webhook_reconcile`), resource (core/search/graphql), repo. Metrics: call
   rate by origin+call-site, latency, rate-limit-remaining gauge, cache hit/miss (304 vs 200).
   Rides the existing OTLP/Datadog pipeline (`telemetry.GetTracer()`/`GetMeter()`,
   `docs/how-to/enable-opentelemetry.md`) — no new storage, no new UI panel.
2. **Priority-aware admission control**: split the single global `DefaultRateLimiter` gate so
   background-origin callers (pollers, backlog sync) back off *before* interactive callers start
   failing, using the same origin tag as the observability workstream.
3. **`GetPRInfoCtx` GraphQL migration** *(batching deferred — see correction below and
   `decisions/ADR-001-defer-graphql-batching.md`)*: migrate `GetPRInfoCtx`'s
   `gh pr view --json reviews,reviewDecision,statusCheckRollup` (the hottest single call site,
   hit by both pollers' "changed" path and every interactive PR RPC) to native GraphQL, folding
   it into the rate limiter for the first time. This supersedes ADR-028's "wrap, don't migrate"
   verdict for this one call site — GraphQL doesn't carry the REST 3-call penalty ADR-028
   rejected migration over.
   *(Corrected by Phase 2 research, 2026-09-08: `github/user_pr_cache.go:516`'s existing query
   does NOT use GraphQL aliases — it's a single list field — so there is no existing
   batching-via-aliases precedent to reuse as originally stated; a real alias-based batch query
   is new design work. More importantly, GitHub's GraphQL endpoint has no ETag/If-None-Match/304
   support, and both pollers' actual hot-path call is the REST+ETag `GetPRInfoConditional` —
   `GetPRInfoCtx` only fires on a detected change. Batching the full per-tick fan-out into one
   GraphQL query would therefore discard the 304 savings on the common "nothing changed" case
   and could increase call volume, undermining Success Metric #2. Batching, if pursued at all,
   applies only to the "changed" subset each tick, not the full tracked-PR set — Phase 3 must
   design around this, not the original per-tick-single-query framing.)*
4. **Webhook-driven poller invalidation**: feed already-arriving `pull_request`,
   `pull_request_review`, `check_run`/`workflow_run` deliveries
   (`server/services/github_webhook_handler.go`) into `PRStatusPoller`/`WorktreePRPoller` to
   invalidate the affected session's ETag entry and trigger an immediate targeted refresh;
   widen the fallback ticker once push-based invalidation covers the common case.
5. **Conditional-requests-by-default infrastructure**: make it structurally hard for a future
   GitHub call site to skip conditional-request semantics — e.g. a single constructor/helper
   for new native call sites that requires an ETag-cache-or-explicit-opt-out argument, and the
   same discipline built into the `gh` CLI wrapper from item 1 (conditional where GitHub's API
   supports it for that endpoint). This is infrastructure, not a one-time migration of existing
   call sites beyond what items 1–3 already touch.

### Out of Scope

- Reviving `project_plans/github-api-usage-tracking/`'s analytics-DB event log or its web UI
  panel (ADR-027). This project does lightweight OTel-only observability instead; the old
  project's storage design is not being re-validated or built.
- Migrating the other 21 `gh` CLI subprocess sites (merge, close, comment, diff, clone, repo
  sync, browse, etc.) to native HTTP/GraphQL — they get wrapped and instrumented, not migrated.
- Multi-tenant / multi-org rate-limit accounting, or supporting more than one GitHub token per
  instance concurrently.
- Any change to GitHub Enterprise Server (GHES) host handling beyond what already exists
  (`RestBaseURLForHost`/`graphQLURLForHost`) — not touching multi-host support.
- UI changes beyond what's needed to observe the new metrics via the existing OTel/Datadog
  pipeline (no new in-app panel), **and beyond the friendlier error-message copy Success Metric
  #1 itself already calls for** ("fail with a clearer, more actionable message than today's raw
  'rate limited until X'"). *(Clarified during Phase 4 cross-artifact review, 2026-09-09: Phase
  6's reason-coded error copy in `VcsPanel.tsx`/`VcsWidgetComments.tsx` — including a Retry
  action where one is missing — implements that already-stated success metric using existing UI
  scaffolding; it is not a new panel and is not scope creep. A genuinely new metrics-observation
  UI surface remains out of scope.)*
- **Cross-machine GitHub call/cache coordination** — explicitly deferred to a follow-on project,
  by user decision (2026-09-08). Covers: gossip-based shard ownership over which host polls
  which repo/account (Akka-cluster-sharding-style, single owner per shard key), sharing
  fetched PR/issue state between instances so a second machine can defer to a first instead of
  re-fetching, and a UI flow for a user to explicitly approve/trust a detected peer's identity
  (upgrading today's fully-automatic TOFU pinning to a human-in-the-loop step). This is not
  starting from zero: `session/host_identity.go`, `session/host_registry.go`,
  `session/host_advertiser.go`, and `server/auth/host_advertisement.go` already implement a
  gossip-style advertisement protocol (Ed25519-signed records, TOFU-pinned public keys, bounded
  one-hop re-gossip, TLS-transport-only — see
  `project_plans/backlog-deep-linking/decisions/ADR-002-gossip-based-host-registry.md`) built
  for deep-link host resolution. The follow-on project's job is extending that existing identity
  layer with GitHub-shard ownership, state sync, and the peer-approval UI — not designing a new
  protocol. It should start from this project's Phase-1 observability data (call rate by
  origin/repo) to know how much cross-machine redundancy actually exists before designing for it.

## Rabbit Holes

- **GraphQL query complexity/cost limits**: GitHub's GraphQL API has its own point-based rate
  limit distinct from REST's request-count limit. Aliasing many PRs into one query could hit a
  query-cost ceiling before it hits a request-count ceiling — needs verification during
  research, not assumed away.
- **`gh` CLI's own internal rate-limit handling**: the CLI does its own retry/backoff
  internally; wrapping it with our own `DefaultRateLimiter` fail-fast could double up or fight
  with that behavior in ways that aren't obvious until tested against a real exhausted token.
- **Webhook delivery reliability**: webhook-driven invalidation is only as good as delivery
  reliability (network blips, GitHub outages, replay/dedup). The poller's fallback ticker has to
  be genuinely a safety net, not something whose interval quietly assumes webhooks always
  arrive.
- **Origin tagging plumbing**: threading an "origin" tag (interactive vs poller vs backlog_sync
  vs webhook_reconcile) through to every call site — including the ones several layers deep,
  like `SetCommitStatus` calling `RefreshPRInfo` calling `GetPRInfoCtx` — could turn into
  invasive signature changes across `github/`, `session/`, and `server/services/` if not
  designed carefully (e.g. via `context.Context` values vs explicit parameters is a real
  decision for Phase 3 planning, not this phase).
- **Feature-flag interaction with the ETag cache**: flipping the GraphQL migration or the
  admission-control flag off mid-session needs to not corrupt or bypass the shared `ETagCache`
  state pollers already depend on.

## Alternatives Considered

- **Migrate all `gh` CLI sites to native HTTP/GraphQL** (full ADR-028 reversal) — rejected for
  scope reasons beyond `GetPRInfoCtx`; the other 21 sites (merge, comment, clone, repo sync,
  browse) don't have GraphQL's cost advantage and gh CLI already handles auth/host resolution
  correctly for them.
- **Poll GitHub's own `GET /rate_limit` on a timer** (Approach C from the old
  github-api-usage-tracking research) as the quota-tracking source of truth — not adopted; this
  project doesn't need token-global reconciliation since it isn't building the analytics-DB
  attribution panel that made that approach valuable.
- **A single unified GitHub client abstraction replacing both the native path and gh CLI** —
  considered and rejected as premature; the wrap-and-instrument approach gets the coordination
  benefits (rate limiting, tracing, conditional-by-default) without a full API redesign.

## Feasibility Risks

- GraphQL query-cost limits (see Rabbit Holes) could mean the batching workstream needs a
  smaller batch size or a fallback split-query path, changing its expected call-volume savings.
- The priority-admission-control split is new, security/correctness-adjacent logic on a shared,
  single-token resource — a bug could make things *worse* than today's undifferentiated
  fail-fast (e.g. starving background paths forever, or failing to actually protect interactive
  calls). This is exactly why it's gated behind a rollout flag.
- Webhook-driven invalidation depends on webhook delivery actually reaching this instance
  reliably in Tyler's deployment (see `docs/how-to/expose-github-webhook-endpoint.md`) — if
  that's flaky in practice, this workstream's payoff shrinks and the fallback ticker interval
  needs to stay conservative.

## Observability Requirements

Covered as its own in-scope workstream (see Scope §1) — not a bolt-on for this project, since
proving Success Metric #1 (zero interactive failures from background exhaustion) requires
origin-tagged data to exist in the first place. Specifically:

- Spans: one per outbound call (native HTTP RoundTrip and gh-CLI-wrapper exec), attributes
  `github.call_site`, `github.origin`, `github.resource`, `github.repo`.
- Metrics: counter (calls by origin+call_site), histogram (latency), gauge (rate-limit
  remaining), counter (cache hit/miss).
- No new alert conditions specified yet — Phase 2 research should check whether the existing
  Datadog/OTLP pipeline has a natural place for a rate-limit-exhaustion alert, or whether that's
  follow-up work.

## Risk Control

- Gate priority-aware admission control (Scope §2) and the `GetPRInfoCtx` GraphQL migration
  (Scope §3) behind the existing live-settable rollout-flag mechanism (same pattern as the
  BackendTymux canary flag, `server/features/flags.go`) — flippable without a redeploy.
- Observability (Scope §1) and conditional-requests-by-default infrastructure (Scope §5) ship
  unflagged — both are additive/structural and don't change existing request behavior.
- Webhook-driven invalidation (Scope §4) ships with the fallback ticker kept conservative
  initially (not widened until the new metrics show invalidation is actually firing reliably),
  rather than trusting it blind on day one.

## Open Questions

- ~~Does GitHub's GraphQL API's point-cost model make per-tick PR-batch aliasing actually
  cheaper than the current per-session REST/gh-CLI calls...?~~ **Resolved by Phase 2 research**
  (see `research/pitfalls.md`, `research/architecture.md`): the premise was wrong — the pollers'
  hot path is REST+ETag (`GetPRInfoConditional`), which GraphQL can't replace without losing
  304 savings. Batching only makes sense for the "changed" subset per tick, not a full-tick
  single query. See the correction now recorded in Scope §3 above.
- ~~Where exactly should the "origin" tag be threaded — `context.Context` value vs. explicit
  parameter...?~~ **Resolved by Phase 2 research** (`research/architecture.md`): reuse the
  existing `prFixTriggerSourceKey` context-value pattern from
  `session/backlog_lifecycle_pr.go:1662-1685` — there's already a working precedent, not a new
  decision to make.
- ~~Is `docs/how-to/expose-github-webhook-endpoint.md`'s current reachability setup reliable
  enough in practice...?~~ **Unresolved by research (accepted), decided by user (2026-09-08)**:
  ship conservative — the fallback ticker stays at today's 60s for the initial ship regardless
  of webhook activity; revisit widening it once observability data shows invalidation firing
  reliably.
- ~~Should the rollout-flag default (Scope §2/§3) ship *on* or *off* at merge time?~~ **Decided
  by user (2026-09-08)**: OFF by default for both priority-admission-control and the
  `GetPRInfoCtx` GraphQL migration. Flip on manually after observing the new metrics.
- How much of the observed cross-machine rate-limit pain (Baseline) is actually cross-machine
  vs. same-process, once Phase 1's origin/repo-tagged metrics exist? This determines how urgent
  the follow-on cross-machine coordination project (see Out of Scope) really is — it may turn
  out same-process fixes alone (Scopes §1-5) resolve most of the observed pain.
- For the follow-on project: does extending `HostRegistry`'s existing advertisement record
  (`AdvertisementRecord`) to also carry "which repos/accounts I'm actively polling" fit its
  bounded, non-flooding gossip design (ADR-002 explicitly scoped out multi-hop convergence
  guarantees and full anti-entropy gossip), or does shard-ownership data need a different
  propagation mechanism than identity/address advertisement?
- For the follow-on project's peer-trust UI: does upgrading from automatic TOFU to a
  human-approval step change the trust model for the *existing* deep-linking feature too (they'd
  share the same `HostRegistry`), or does GitHub-call sharding need its own, separate trust
  tier layered on top of the same identities?
