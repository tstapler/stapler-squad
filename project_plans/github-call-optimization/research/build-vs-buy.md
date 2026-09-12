# Build vs. Buy — github-call-optimization (Agent 6, Phase 2)

Evaluated against the current codebase state (branch `stapler-squad-github-call-optimization`,
commit `db3ecfc84`). Verified via `Read`/`Grep` of `go.mod`, `github/rate_limit.go`,
`github/http_client.go`, `github/client.go`, `github/user_pr_cache.go`,
`server/services/github_webhook_handler.go`, `tools/lint/*`, and `Makefile`'s `lint-custom`
target, plus a `WebSearch` for the two GitHub-client-library licenses/maintenance status.

## 1. GitHub API Go client (`shurcooL/githubv4`, `google/go-github`) — scope item 3

**Current state**: `github/user_pr_cache.go:516` already hand-rolls one GraphQL query as a Go
string constant (`userPRGraphQLQuery`) plus a hand-decoded response struct (`graphQLResponse`,
line 551). `github/client.go:290`'s `GetPRInfoCtx` shells to `gh pr view --json ...` via
`safeexec.CommandContext`. No `google/go-github` or `githubv4` import exists anywhere in the
module (`grep go.mod` — zero hits for either).

**`shurcooL/githubv4`**:
- Pros: typed query builder via Go struct tags + GraphQL alias support (exactly what "one query
  per poller tick aliasing all tracked PR numbers" needs) — would remove the hand-written
  aliasing string-concatenation this project is about to write a *second* time.
- Cons: MIT-licensed but effectively unmaintained — last commit July 2024, 6 open issues,
  including #17 "Query batching and/or dynamic queries" left unresolved, which is precisely the
  batching capability scope item 3 needs most. Adopting a library specifically for the one
  feature its own open issue tracker says is unsettled is a bad bet. It also only wraps query
  construction/decoding — the rate-limiter, ETag cache, and OTel span/metric wrapping (scope
  items 1, 2, 5) are 100% custom to this codebase either way and would have to wrap
  `githubv4.Client`'s `http.Client` field regardless, so it saves work only in the query-string
  layer, not the parts of this project that carry the real complexity.
- Verdict: **Not recommended.** Low activity + the exact gap (batching) this project needs it
  for is an open, unaddressed issue upstream.

**`google/go-github`**:
- Pros: BSD-3-Clause, actively maintained by Google with a fast release cadence tracking the
  GitHub REST API version.
- Cons: REST-only — it has no GraphQL surface, so it cannot serve scope item 3's GraphQL
  batching goal at all; it would only be relevant if this project pivoted the whole PR-info path
  to REST (out of scope — GraphQL was chosen specifically to batch N PR lookups into 1 call, a
  REST client can't do that). Pulling in a large, general-purpose REST client whose types this
  project would use for a handful of already-covered call sites (`GetPRInfoCtx`, etc.) is a
  heavyweight, mostly-unused dependency for the REST side, and does nothing for the GraphQL side.
- Verdict: **Not recommended** for this project's REST call sites (marginal win, adds a large
  surface for a handful of calls) and **irrelevant** to the GraphQL batching goal.

**Consistency argument**: `user_pr_cache.go` already set the "hand-rolled GraphQL string +
manual struct decode" precedent. Introducing `githubv4` now for the *second* query but leaving
the first one hand-rolled makes the codebase less consistent, not more — two different ways to
talk to the same API, split by decision timing rather than by problem shape. Given `githubv4`'s
maintenance status doesn't even buy the batching capability this project needs, extending the
existing hand-rolled pattern to the new query (reusing `user_pr_cache.go`'s decode helpers where
shapes overlap) is both the lower-risk and the more consistent choice.

## 2. OTel HTTP client instrumentation (`otelhttp.NewTransport`) — scope item 1 (HTTP path)

**Current state**: `go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.67.0` is
already a **direct** dependency (`go.mod`) and already imported in `server/server.go:43`, used
via `otelhttp.NewHandler` for inbound request instrumentation (lines 1303, 1507). It is not yet
used anywhere as a client-side `RoundTripper`.

- Pros: zero new dependency — it's already vendored and already proven in this codebase for the
  inbound side. `otelhttp.NewTransport(next http.RoundTripper, opts...)` composes cleanly with
  the existing `rateLimitTransport` in `github/http_client.go:36` (wrap
  `rateLimitTransport{next: otelhttp.NewTransport(http.DefaultTransport)}`, or the reverse order
  if the rate-limiter's fail-fast short-circuit — line 48's `IsLimited()` check — should itself
  be a span). Gets span creation, `http.request_duration`-style base metrics, and W3C trace
  propagation for outbound calls for free, matching the existing OTel pipeline
  (`telemetry/telemetry.go`) with no new SDK surface to learn.
- Cons/gaps it does *not* cover, confirmed by its API surface: it has no concept of
  `call-site`/`origin` (interactive/poller/backlog_sync/webhook_reconcile)/`resource`/`repo` tags
  — those are this project's custom attributes and need `otelhttp.WithSpanNameFormatter` plus
  manual `trace.SpanFromContext(...).SetAttributes(...)` in a thin wrapping layer, or a
  `context.Context`-carried attribute set. It also emits no rate-limit-remaining gauge or
  cache-hit/miss metric — those still have to be hand-emitted via `telemetry.GetMeter()`
  alongside `RateLimiter.Update` (`github/rate_limit.go:49`), since that's domain logic
  `otelhttp` has no way to know about.
- Verdict: **Recommended** for the base span/metric layer of scope item 1's HTTP path — it's a
  zero-new-dependency, already-proven-in-repo win. It does not replace the custom
  attribute/gauge work item 1 also requires; it only removes the boilerplate of span
  creation/propagation around it. The 22 `gh` CLI subprocess sites (scope item 1's other half)
  get no benefit from `otelhttp` at all — those need a hand-rolled `exec.CommandContext`
  wrapper emitting spans/metrics manually, since there's no HTTP transport to instrument.

## 3. Rate limiting library (`golang.org/x/time/rate`, `uber-go/ratelimit`) — scope item 2

**Current state**: `golang.org/x/time v0.15.0` is already a direct dependency and its `rate`
subpackage (token bucket) is already used in three places: `server/auth/hostname_guard.go`,
`server/services/rate_limiter.go`, `server/services/trigger_rate_limiter.go` — all *inbound*
request throttling (classic requests-per-second admission control). `github/rate_limit.go`'s
`DefaultRateLimiter`, by contrast, tracks exactly one thing: a single `rateLimitedUntil`
deadline derived from GitHub's own `X-RateLimit-Reset`/`Retry-After` response headers
(lines 41-44, 128-139).

- This is the crux of the requirements doc's own caution, and it holds up under inspection: a
  token bucket models *self-imposed* request pacing (e.g. "don't exceed N req/s of our own
  choosing"). `DefaultRateLimiter` isn't pacing anything — it's a deadline gate reacting to a
  budget GitHub itself reports, which already comes as a hard reset timestamp, not a rate to
  spend against. Wrapping that in `x/time/rate.Limiter` (or `uber-go/ratelimit`, which is a
  leaky-bucket construct for smoothing *steady* outbound QPS) would model the wrong primitive —
  the token bucket's refill rate has no natural value to set here, because the "budget" already
  resets in one lump sum at a GitHub-controlled instant, not continuously.
- What scope item 2 actually needs is **priority-aware admission**, i.e. multiple *consumers* of
  the same shared, externally-reported budget who should back off at different remaining-budget
  thresholds (background origins stop before interactive origins do). That's a reservation/
  headroom-split problem layered on top of the existing single `rateLimitedUntil`/remaining-count
  read, not a token-bucket-per-origin problem — origins don't get independent quotas from GitHub,
  they share one.
- `uber-go/ratelimit`: same objection (still modeling self-paced QPS), plus it's had periods of
  low maintenance activity historically; not evaluated further given the primitive mismatch
  above makes it moot regardless of maintenance status.
- Verdict: **Not recommended** to adopt either as the core mechanism. The correct extension is
  evolving `RateLimiter` to track `remaining`/`limit` (not just `rateLimitedUntil`) and add
  per-origin reserved-headroom thresholds (e.g. background origins treat "remaining < 20% of
  limit" as limited; interactive origins only back off at "remaining == 0") — a bespoke
  extension of the existing struct, not a library swap. `x/time/rate` *could* still be reused
  narrowly as an internal implementation detail for smoothing burst admission within one
  priority tier, but that's optional polish, not the core fix, and isn't worth adding scope for.

## 4. Webhook framework (`go-playground/webhooks`, etc.) — scope item 4

**Current state**: `server/services/github_webhook_handler.go` (227 lines) already does
HMAC-SHA256 signature verification (`verifySignatureForRepo`, `verifiedWorkflowCandidates`) and
manual JSON payload parsing (`extractGitHubRepoAndBranch` unmarshals into
`map[string]interface{}`) via raw `net/http`, working in production today. Scope item 4 only
asks this project to **add a new consumer** — invalidate poller cache entries — of payloads this
handler already verifies and parses; it does not touch signature verification or payload
receiving.

- A webhook-handling library's entire value proposition is signature verification + event-type
  routing/parsing, both of which are already done and already correct here. Since scope item 4
  is strictly downstream of that (a new call inside/after `Handle`, e.g. after
  `extractGitHubRepoAndBranch` succeeds, invalidate the relevant poller cache key), there is no
  surface for a library to attach to without a rewrite of already-working code that isn't in
  scope.
- Verdict: **Not applicable / not recommended.** No library evaluation changes this — the work
  is "call `pollerCache.Invalidate(repo, ...)` from the existing handler," not "receive
  webhooks."

## 5. Structural enforcement for conditional-requests-by-default — scope item 5

**Current state**: the repo already has an established, working pattern for exactly this shape
of guardrail, in two forms:
- **Custom `go/analysis` linters**: `tools/lint/{norawgitopen,norawexec,nocommandpattern,
  entfullscan,hotpolllog,silenttransition,tmuxsocketscope}/analyzer.go` — each a small
  `analysis.Analyzer` (see `norawgitopen/analyzer.go`'s ~120 lines) that walks the AST for a
  specific forbidden call pattern, checks a `//nolint:<name>` escape hatch
  (`tools/lint/internal/nolintcomment`), and reports a violation with an actionable message
  pointing at the approved wrapper. These are NOT golangci-lint plugins — they're compiled into
  a standalone `multichecker`-style binary via `tools/lint/cmd/linter` (own `go.mod`, own
  `golang.org/x/tools` dependency) and run by `make lint-custom`, wired into `make lint`'s
  prerequisites (`Makefile:766`). Zero new dependency needed — `tools/lint` already imports
  `golang.org/x/tools/go/analysis` and its subpackages.
- **Rule doc convention**: `.claude/rules/instance-lock-free-reads.md` — a glob-scoped
  (`globs: ["session/instance*.go"]`) Markdown convention doc, auto-loaded by Claude Code only
  when a matching path is touched, for a guardrail that isn't (yet) enforced by tooling.

- `norawgitopen` is the closest precedent in *shape*: "always call the approved wrapper
  (`session/git.OpenRepo`), never the raw underlying call directly" is structurally identical to
  "always construct GitHub HTTP requests through the approved conditional-request-aware
  constructor, never build a bare `http.NewRequest` to a GitHub host directly." The same
  AST-walk-for-a-disallowed-call-outside-an-allowlist technique transfers directly: flag any
  `http.NewRequest`/`http.NewRequestWithContext` call whose URL argument resolves to
  `github.GhBaseURL()`/a GitHub host, outside `github/http_client.go`'s own constructors
  (`newGHRequest`, `newGHRequestForHostWithToken`), just as `norawgitopen` flags `git.PlainOpen`
  outside `session/git.OpenRepo`.
- A `.claude/rules/*.md`-only convention (no tooling) is weaker here than it was for
  `instance-lock-free-reads.md`: that rule's violations are only surfaced by `go test -race`
  after the fact (a data race needs a race detector run to manifest), so a doc-only rule paired
  with race-detected CI was an acceptable stopgap. A missing ETag/conditional-request header is
  silent at compile *and* test time — nothing fails, it just quietly costs an extra full-fidelity
  GitHub API call forever. That "no natural test failure surfaces it" property is exactly the
  case the repo's own custom-linter pattern exists to close, and doing so costs one more file in
  an already-established, low-friction pattern (7 precedents, one Makefile target, no plugin
  framework to learn).
- Verdict: **Recommended: a new custom `go/analysis` linter** (e.g.
  `tools/lint/norawghrequest/`) following the `norawgitopen` pattern exactly, not a
  `.claude/rules/*.md`-only convention. Add the rule doc too (cheap, and useful as the
  human-readable rationale the lint error message points back to), but don't rely on it alone.

## 6. LLM-generated vs. battle-tested — priority-admission-control concurrency (scope item 2)

**Current state / history**: `DefaultRateLimiter` (`github/rate_limit.go:25`) is explicitly
documented as "the shared GitHub API rate limiter used by all native HTTP calls" — global
mutable state guarded by a single `sync.RWMutex`. The requirements doc cites prior BUG-080/081
history around exactly this struct being global mutable state; `Reset()`'s doc comment
(lines 177-181) independently confirms the failure mode is real: tests already had to add a
manual reset hook because the *existing single-limiter* design leaks state across test
boundaries in the same binary — a small-scale preview of the class of bug that gets worse, not
better, once multiple priority tiers share the same struct.

- No prior-art library exists that solves *this specific* problem (a single externally-reported
  budget split into priority tiers with independent backoff thresholds) — this was already
  established in item 3 above: it's not a rate-limiting problem in the library sense, so there's
  nothing to "adopt" here. The choice isn't "library vs. hand-roll," it's "how carefully to
  hand-roll," which makes this a pure code-quality/review-rigor question, not a build-vs-buy one.
- Given BUG-080/081 is direct evidence this exact class of code (global mutable rate-limiter
  state under concurrent access) has broken before in this codebase, the risk-mitigation lever
  available is not "find a library" but: (a) keep the existing lock-free-snapshot style already
  used elsewhere in this repo for concurrent state (`Snapshot()`/`atomic.Pointer` pattern
  documented in `.claude/rules/instance-lock-free-reads.md` for `*Instance` — the same technique
  generalizes to a per-origin rate-limiter state struct: publish an immutable
  `RateLimiterSnapshot{RemainingByOrigin map[string]int, ...}` via `atomic.Pointer` rather than
  adding more fields under the existing single `sync.RWMutex`); (b) write a `go test -race`
  suite exercising concurrent origin-tier reads/writes *before* implementation, mirroring how
  `instance-lock-free-reads.md`'s bug was actually caught (a race-detector run, not code review);
  (c) run this specific piece through the `code-review` / `quality:is-it-ready` skills' adversarial
  pass given its history, rather than treating it as routine.
- Verdict: **Hand-roll, but with elevated rigor** — mandatory `-race` coverage for the new
  priority-tier logic as a merge gate (not just a style suggestion), reuse the codebase's own
  `atomic.Pointer` snapshot pattern rather than extending the existing `sync.RWMutex` struct with
  more fields, and treat this specific change as warranting the adversarial review pass given
  the BUG-080/081 precedent.

## Summary table

| # | Scope item | Library considered | Verdict |
|---|---|---|---|
| 3 | GraphQL batching + `GetPRInfoCtx` | `shurcooL/githubv4` | Not recommended — unmaintained, its own open issue tracker names batching as unsolved |
| 3 | GraphQL batching + `GetPRInfoCtx` | `google/go-github` | Not recommended — REST-only, no GraphQL surface |
| 1 | OTel spans (HTTP path) | `otelhttp.NewTransport` | **Recommended** — zero new dep, already used inbound; doesn't cover custom tags/gauges (still hand-rolled) |
| 1 | OTel spans (`gh` CLI path) | n/a | No library applies — hand-roll `exec.CommandContext` wrapper |
| 2 | Priority admission control | `x/time/rate`, `uber-go/ratelimit` | Not recommended — token/leaky bucket models the wrong primitive for a server-reported reset-based budget |
| 4 | Webhook invalidation | `go-playground/webhooks` | Not applicable — scope item is a new consumer of an already-working hand-rolled verifier/parser |
| 5 | Conditional-requests enforcement | custom `go/analysis` linter (in-repo pattern) | **Recommended** — follow `tools/lint/norawgitopen` exactly, zero new dependency |
| 2 | Priority admission control (concurrency risk) | prior-art library | None exists for this shape of problem — hand-roll with mandatory `-race` gate + snapshot pattern + adversarial review |
