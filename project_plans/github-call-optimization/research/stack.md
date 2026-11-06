# Stack Research: github-call-optimization

Agent 1 (Stack), Phase 2. Scope: OTel SDK specifics, GitHub GraphQL specifics,
concurrency-primitive consistency, dependency versions, feature-flag mechanism.
All line/version citations below are from this worktree unless noted otherwise.

## 1. OpenTelemetry Go SDK

**Versions in `go.mod`:**
```
go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.67.0
go.opentelemetry.io/otel                                       v1.44.0
go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc v1.44.0
go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc  v1.39.0
go.opentelemetry.io/otel/sdk                                    v1.44.0
go.opentelemetry.io/otel/sdk/metric                             v1.44.0
go.opentelemetry.io/otel/trace                                  v1.44.0
go.opentelemetry.io/otel/metric                                 v1.44.0
```
Semconv package pinned in `telemetry/telemetry.go:21`: `semconv "go.opentelemetry.io/otel/semconv/v1.41.0"`.

**Existing entry points** (`telemetry/telemetry.go`):
- `telemetry.GetTracer()` / `telemetry.GetMeter()` (lines 229, 240) — safe to call before `Initialize`; return real no-op impls, not nil, so instrumentation code never needs an `IsEnabled()` guard.
- `telemetry.StartSpan(ctx, name, opts...)` (line 248) is the existing convenience wrapper already used at several call sites (`server/services/search_service.go:178,210,546,606`, `server/services/connectrpc_websocket.go:2879`) — new GitHub instrumentation should call this rather than `otel.Tracer(...)` directly, for consistency.
- `telemetry.StartLinkedBackgroundSpan` (line 266) exists for goroutine-outlives-request cases (ADR-003) — relevant since the two PR-status pollers and backlog sync run in background goroutines outside any inbound request's span.

### 1a. Wrapping the `http.RoundTripper` (native HTTP path)

`github/http_client.go:22-25` already wraps `http.DefaultTransport` with a custom `rateLimitTransport` (a plain `struct{ next http.RoundTripper }` implementing `RoundTrip`). This is the pattern to extend, not replace — chain a new telemetry transport with it rather than inventing a second wrapping style.

Confirmed via `go doc`: `go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp` (already a direct dependency, v0.67.0) exports:
```go
func NewTransport(base http.RoundTripper, opts ...Option) *Transport
```
This gives free, semconv-correct generic HTTP client spans/metrics (duration, request/response size, `http.request.method`, `http.response.status_code`, etc.) with zero hand-rolled attribute code — `otelhttp.NewTransport(&rateLimitTransport{next: http.DefaultTransport})` layered into `ghHTTPClient.Transport` is the lowest-effort way to get baseline spans.

**However**, `otelhttp.NewTransport`'s built-in attributes do not cover this project's GitHub-specific requirements (origin, call-site, resource, repo, rate-limit-remaining gauge, 304-vs-200 cache hit/miss) — those need custom attributes read from GitHub's response headers (`X-RateLimit-Remaining`, `X-RateLimit-Resource`, status code 304) exactly where `rate_limit.go`'s `RateLimiter.Update` already parses them. Recommendation: a **third, purpose-built transport** (`githubTelemetryTransport`, same one-struct-one-method shape as `rateLimitTransport`) that:
- Starts a span via `telemetry.StartSpan(ctx, "github.http.<verb-ish-name>", trace.WithSpanKind(trace.SpanKindClient))` — `SpanKindClient` is correct per OTel spec (this process initiates the call and waits synchronously for the response) — no ambiguity here.
- Sets attributes: call-site (needs to be threaded in via request context — see 1c), origin (ditto), resource (parseable from `X-RateLimit-Resource` response header, already extracted in `rate_limit.go:50`), repo (from the request path/GraphQL variables), plus standard `http.response.status_code`.
- Records latency as a histogram and rate-limit-remaining as an `Int64ObservableGauge` (or a synchronous gauge-via-histogram if async gauges prove awkward to wire through a per-request transport — needs a design decision in Phase 3, not resolved here).
- Records cache hit/miss: `resp.StatusCode == http.StatusNotModified` → hit, `200` → miss. This aligns with the existing `ETagCache`/`GetPRInfoConditional` pattern in `github/etag_cache.go:53-80`.

Chaining order matters: put this new transport **outside** `rateLimitTransport` (i.e. `ghHTTPClient.Transport = &githubTelemetryTransport{next: &rateLimitTransport{next: http.DefaultTransport}}`) so the fail-fast "already rate limited, skipping request" short-circuit in `rateLimitTransport.RoundTrip` (`http_client.go:48-50`) still gets a span recording that skip — otherwise skipped calls would be invisible to the new observability, defeating part of the point (interactive vs background origin attribution needs to see attempts that never left the process, too).

### 1b. Instrumenting the `safeexec.CommandContext` subprocess boundary

**Confirmed: there is no off-the-shelf OTel instrumentation for CLI subprocess calls.** OTel's semantic conventions do define generic `process.*` resource/span attributes (verified against the vendored `go.opentelemetry.io/otel/semconv/v1.41.0/attribute_group.go` — same version this repo pins): `process.command` (`process.command.key`), `process.command_args`, `process.command_line`, `process.executable.name`, `process.executable.path`, `process.exit.code`, `process.pid`. These are the right attribute *keys* to reuse (don't invent new attribute names for command/args/exit-code — semconv already has them), but there is no blessed span-kind/name convention for "a CLI invocation as an RPC-shaped client call" the way there is for `rpc.*`/`http.*` — this part is genuinely uninstrumented territory across the OTel ecosystem, not just this codebase. Flag for Phase 3: the span name convention and exact attribute set is a design decision this research cannot settle by itself, only bound it.

Recommended manual pattern (bounding the design space, not prescribing the final answer):
- Span name: something like `"gh.<subcommand>"` (e.g. `"gh.pr.view"`, `"gh.pr.merge"`) mirroring the `rpc.system`/`rpc.method` naming shape (`{system} {method}`) even though `rpc.*` semconv doesn't technically apply to a subprocess — closer fit than borrowing HTTP conventions, since there's no HTTP request/response.
- `trace.WithSpanKind(trace.SpanKindClient)` — same reasoning as 1a: this process is the initiator, synchronously waiting on a "remote" operation (even though the remote hop here is `gh`'s own HTTP calls, opaque to us).
- Attributes: `process.command` = `"gh"`, `process.command_args` = the subcommand args (redact anything sensitive — check whether any `gh` invocation ever takes a token/secret positionally; `--body`/`--repo`/etc. args look safe from the call sites seen), `process.exit.code` on completion, plus this project's own `call-site`/`origin`/`resource`/`repo` attributes (same shape as 1a, for a unified query surface across both HTTP and CLI paths).
- Duration: wrap `cmd.Run()`/`cmd.CombinedOutput()`/`cmd.Output()` (whichever `safeexec.CommandContext` call site uses) with `time.Since(start)` and record as a histogram, same instrument used for the HTTP path so origin/call-site cross-cut both.
- **Single wrapper, 22 call sites**: requirements.md calls for "a new single instrumented wrapper collapsing 22 raw `gh` CLI subprocess sites". Confirmed call sites via `safeexec.CommandContext` grep: 11 in `github/user_pr_cache.go` (`GetPRInfoCtx` at line 290/299, plus 10 more at lines 573, 600, 644, 667, 700, 722, 742, 758, 773, 795 spanning `gh` and `git` subcommands), with the remainder split across `github/client.go`, `session/git/worktree_git.go`, `github/commit_status.go`, `github/cli_import.go`, `session/git/util.go` per requirements.md's own accounting. A single `func runGHCommand(ctx, callSite, origin string, args ...string) ([]byte, error)`-shaped wrapper (analogous to how `rateLimitTransport` centralizes the HTTP path) is the natural collapse point — but note it needs to handle both `gh` and plain `git` invocations (`git -C <path> fetch/checkout/remote` also appear in the grep above), so the wrapper's first positional arg (the binary name) can't be hardcoded to `"gh"`.

### 1c. Origin/call-site propagation — a new piece of infrastructure, not yet built

Neither `github/*.go` nor the two pollers currently thread an "origin" (interactive/poller/backlog_sync/webhook_reconcile) value through `context.Context` — confirmed via grep (`context.WithValue`, `ctxKey`, `contextKey` — zero hits in `github/`, `session/pr_status_poller.go`, `session/worktree_pr_poller.go`, `server/services/github_webhook_handler.go`). This will need to be built from scratch in Phase 3.

The codebase does have one precedent for the idiom to follow: `executor/audit.go:85-99` —
```go
type ctxKey struct{}
func WithAuditHook(ctx context.Context, hook AuditHook) context.Context {
    return context.WithValue(ctx, ctxKey{}, hook)
}
func AuditHookFromContext(ctx context.Context) (AuditHook, bool) {
    hook, ok := ctx.Value(ctxKey{}).(AuditHook)
    return hook, ok
}
```
An unexported empty-struct key type + `With<Thing>`/`<Thing>FromContext` accessor pair. A new `github` (or shared) package-level `WithCallOrigin(ctx, origin)` / `CallOriginFromContext(ctx)` following this exact shape is the consistent choice — both for the telemetry attributes (1a/1b) and for priority-aware admission control (scope item 2), which needs the same origin value to decide whether to back off.

## 2. GitHub GraphQL API specifics

### 2a. Alias syntax for batching multiple PR lookups

**Important correction to requirements.md's framing**: the existing query at `github/user_pr_cache.go:516-549` (`userPRGraphQLQuery`, ADR-020) does **not** use GraphQL aliases. It's a single field access — `viewer { pullRequests(first: 100, ...) { nodes { ... } } }` — which works because "give me all PRs the viewer has open" is naturally a *list* field. It doesn't need aliasing because it's not requesting N distinct named resources.

What scope item 3 actually needs (pollers fanning out per-session, replaced by one query aliasing all tracked PR numbers) is a genuinely different GraphQL shape: looking up N *specific, distinct* PRs by (owner, repo, number) in one round trip requires one aliased field per PR, e.g.:
```graphql
query BatchedPRs {
  pr0: repository(owner: "tstapler", name: "stapler-squad") { pullRequest(number: 704) { ...PRFields } }
  pr1: repository(owner: "tstapler", name: "stapler-squad") { pullRequest(number: 705) { ...PRFields } }
  pr2: repository(owner: "someorg", name: "otherrepo")      { pullRequest(number: 12)  { ...PRFields } }
}
```
GraphQL alias syntax (`alias: fieldName(args) { ... }`) is standard and does work this way against GitHub's schema — this is a well-documented GraphQL capability, not GitHub-specific, so there's no ambiguity about whether the syntax itself works. What's **not yet proven in this codebase** is the batching/grouping strategy requirements.md asks for ("grouped by repo") — e.g. whether to alias per-repo (one aliased `repository(owner,name)` per distinct repo, with multiple `pullRequest` sub-aliases or a `pullRequests(...)` filtered list under each) versus one alias per PR flatly. That's a Phase 3 design decision; this research only confirms the primitive (aliasing) is real and usable, not which grouping shape to build. **Do not cite `user_pr_cache.go:516` as prior art for the alias syntax itself** — it's prior art only for "one GraphQL round trip via `ghHTTPClient` beats N REST calls," the general principle, not the alias mechanic.

A GitHub-specific constraint to carry into Phase 3, not resolved here: GitHub's GraphQL query complexity/depth limits (and the 500-alias soft ceiling some GitHub GraphQL docs mention for very wide aliased queries) bound how many PRs can be batched into one query — if a poller ever tracks more PRs than that ceiling, chunking logic (similar to `user_pr_cache.go`'s existing `pageInfo.hasNextPage` cursor pagination, ADR-020) is still needed. **Flagged as uncertain / needing verification against current GitHub docs before implementation** — not verified in this research pass (no live network fetch against GitHub's GraphQL docs was performed).

### 2b. Fields needed to replicate `gh pr view --json reviews,reviewDecision,statusCheckRollup`

`GetPRInfoCtx` (`github/client.go:290-299`) currently shells out to `gh pr view <ref> --repo <repo> --json reviews,reviewDecision,statusCheckRollup`. The existing `userPRGraphQLQuery` already demonstrates the GraphQL shape for two of these three fields on a `PullRequest` node:
- `reviewDecision` — direct scalar field on `PullRequest` (used as-is at `user_pr_cache.go:535`).
- `reviews` — the existing query uses `reviews(last: 20, states: [APPROVED, CHANGES_REQUESTED]) { nodes { state } }` (line 536-538). The `gh pr view --json reviews` CLI field returns more per-review detail (author, body, submittedAt) than the existing query fetches — replicating `--json reviews` exactly (not just `reviewDecision`) will need a wider set of sub-fields (`nodes { author { login } state submittedAt body }` and likely more, depending on what the current CLI-based `GetPRInfoCtx` callers actually consume from the `reviews` array — **check `github/client.go`'s `PRInfo` struct fields before finalizing this list**, not done in this pass).
- `statusCheckRollup` — the existing query only reads `commits(last: 1) { nodes { commit { statusCheckRollup { state } } } }`, i.e. just the aggregate `state` enum. The CLI's `--json statusCheckRollup` field returns a richer structure (per-check name, status, conclusion, detailsUrl for each check run/status context) rather than just the rolled-up state — GitHub's GraphQL schema exposes this via `commits(last: 1) { nodes { commit { statusCheckRollup { contexts(first: N) { nodes { ... on CheckRun { name, conclusion, status } ... on StatusContext { context, state } } } } } } }` (a union type over `CheckRun`/`StatusContext`, since GitHub Checks API and legacy Commit Status API both feed the rollup). **This union-type shape is inferred from general familiarity with GitHub's GraphQL schema, not verified via a live schema introspection call in this session — flag as needing a real query test against GitHub before Phase 3 locks the field list.**

### 2c. GraphQL rate-limit cost model (points vs REST request count)

This directly informs an Open Question in requirements.md. From general knowledge of GitHub's public GraphQL API documentation (**not verified live in this session — no network fetch was performed against GitHub's docs; treat as INFERRED**):
- REST: each request costs exactly 1 point against the primary rate limit (5000/hr for a PAT).
- GraphQL: each query costs a *computed* number of points based on the query's shape — roughly, the sum of "child nodes requested" across connections, with a formula that penalizes nested connections/pagination width (first/last arguments) rather than raw field count. A query is rejected outright (before execution) if its estimated cost exceeds a max (historically documented as 500,000 for the *estimated max* single-query cost, separate from the 5000/hr budget the query then draws down from).
- The two limits are **tracked separately but drawn from related quotas** — GitHub exposes both via the `rateLimit { limit, cost, remaining, resetAt }` GraphQL field (queryable in the same request) and via REST's `GET /rate_limit`, which reports `resources.graphql` and `resources.core` as distinct buckets, each with its own `limit`/`remaining`/`reset`.
- Practical implication for this project: `DefaultRateLimiter` (`github/rate_limit.go`) currently tracks one undifferentiated `rateLimitedUntil` state, fed by the `X-RateLimit-*` HTTP response headers shared across REST and GraphQL responses on `api.github.com`. Since GraphQL responses **do** carry the same `X-RateLimit-Remaining`/`X-RateLimit-Reset` headers (confirmed pattern used already for `userPRGraphQLQuery`'s HTTP round trip, which flows through the same `ghHTTPClient`/`rateLimitTransport`), the existing rate limiter mechanism already works correctly for GraphQL cost tracking *at the HTTP-response-header level* without code changes — the open question is whether `resource` attribution (needed for the observability scope item's `resource` attribute: core/search/graphql) needs the response body's `data.rateLimit.cost` field too, since the header only reports remaining/limit, not what a specific query just cost. Recommend requesting `rateLimit { cost, remaining, resetAt }` as a top-level field alongside every batched-PR GraphQL query in Phase 3 so the observability layer can record actual per-query cost, not just derive it from remaining-before minus remaining-after (racy under concurrent GraphQL calls sharing one token).

## 3. Concurrency primitives already in use — patterns to stay consistent with

Confirmed via grep across `github/*.go` and the two pollers:

- **`golang.org/x/sync/singleflight`** (`golang.org/x/sync v0.22.0` per `go.mod:209`) — used in three places: `github/http_client.go:101` (`ghTokenSF`, coalescing keychain token reads), `github/client.go:34` (`ghAuthGroup`, coalescing `gh auth` checks), `github/user_pr_cache.go:121-122` (`loginGroup`, `refreshGroup`, coalescing login/PR-list refreshes). Pattern: a package-level or struct-field `singleflight.Group` (marked `//nolint:exhaustruct`), keyed by a static string when there's genuinely one shared resource (e.g. `"keychain-token"`, `"login"`), or a dynamic key when coalescing per-entity work. **New GraphQL batching code should use singleflight the same way** to coalesce concurrent callers hitting the same batched-PR-lookup tick, rather than introducing a different coalescing primitive.
- **`puzpuzpuz/xsync/v4`** (`v4.5.0` per `go.mod:39`) — used for concurrent maps needing lock-free reads with in-place compute, not for anything rate-limiter-shaped. Examples: `session/shell_registry.go` (a generic `xsync.Map` keyed by string with `shellEntry` values, using `.Compute(key, func(v, loaded) (v, xsync.ComputeOp)` with `xsync.UpdateOp`/`xsync.CancelOp`) and `session/review_queue_poller.go:164-165` (a string-keyed `xsync.Map` of `contentCacheEntry`, explicitly commented "replaces 4 map + RWMutex fields — lock-free reads across sessions"). **Relevant if priority-admission-control needs a per-origin or per-repo concurrent map** (e.g. tracking in-flight background call counts per origin) — reach for `xsync.Map` with `.Compute`, not a hand-rolled `sync.Map` with type assertions or a plain map guarded by `sync.RWMutex`.
- **`sync/atomic`** — the dominant pattern for simple scalar/pointer state shared across goroutines with no compound invariant to protect: `session/pr_status_poller.go:63` and `session/worktree_pr_poller.go:85-86` both store poller auth state in `atomic.Value` (documented at `worktree_pr_poller.go:75-76`: "auth state is an atomic.Value ... same pattern as PRStatusPoller" and "onUpdated callback is an atomic.Value — writers Store, readers Load, no lock"). `github/http_client.go:99-100` uses `atomic.Value`/`atomic.Int64` for the token cache. **A new priority-admission-control gate that just needs "is background currently backed off" as a single readable boolean/timestamp should follow `RateLimiter`'s own existing `sync.RWMutex`-guarded struct pattern** (`github/rate_limit.go:41-44`, `IsLimited`/`setLimitedUntil`) rather than switching to atomics mid-file — the existing rate limiter isn't atomic-based (it protects a compound read: check-then-return `(bool, time.Time)`), so a priority-aware extension of it should stay `sync.RWMutex`-based for consistency with the type it's extending, even though atomics are the norm elsewhere for single-value state.

## 4. Dependency confirmation

- `go.opentelemetry.io/otel` and friends: **v1.44.0** (SDK/API), `otel/trace` v1.44.0, `otel/metric` v1.44.0, otlp exporters mostly v1.44.0 except `otlptracegrpc`/`otlptrace` pinned at v1.39.0 (pre-existing minor version skew in `go.mod`, not introduced by this project — worth a note but out of this project's scope to fix).
- `golang.org/x/sync`: **v0.22.0** (`go.mod:209`), providing the `singleflight` package already in use.
- `github.com/puzpuzpuz/xsync/v4`: **v4.5.0** (`go.mod:39`).
- **No `google/go-github`, no `shurcooL/githubv4`, no other GitHub API Go client dependency exists in `go.mod`/`go.sum`** — confirmed via direct grep (zero hits for `go-github`, `githubv4`). This matches the "earlier findings this session" the task description referenced: all native HTTP GitHub calls in this codebase are genuinely hand-rolled (`net/http` + manual JSON marshaling of GraphQL query strings, e.g. `user_pr_cache.go:604`), not routed through a client library. Any new GraphQL batching code will continue that hand-rolled convention rather than introducing a new dependency, matching the existing `ADR-020` design mandate ("Use a direct `POST https://api.github.com/graphql` HTTP call").

## 5. Feature-flag mechanism — concrete API for Phase 3 to cite

Two distinct registries exist; requirements.md's Risk Control section ("the existing live-settable rollout-flag mechanism, `server/features/flags.go`'s pattern") actually points at the **`FeatureFlagService`/`knownFeatureFlags`** mechanism in `server/services/feature_flag_service.go`, not `server/features/flags.go` itself (that file only registers the `GetFeatureFlags`/`UpdateFeatureFlag` RPCs into the separate `featureregistry` discovery system used for docs/registry tooling — it is not where flag *values* live). Phase 3 should cite the service file, not `server/features/flags.go`.

Concrete steps to add the two flagged workstreams (priority-admission-control, GraphQL migration for `GetPRInfoCtx`), following the existing pattern exactly:

1. Add a name constant, e.g. `const githubPriorityAdmissionFlagName = "github:priority-admission-control"` and `const githubGraphQLMigrationFlagName = "github:graphql-pr-info"`, next to the existing constants at the top of `server/services/feature_flag_service.go` (lines 15-44 show the convention: a doc comment explaining why the constant is shared between the registry entry and its real call site, to prevent name drift).
2. Add a corresponding entry to the `knownFeatureFlags` slice (`feature_flag_service.go:127-197`), each entry being `{name, description, defaultValue}` — `defaultValue` omitted (defaults to `false`/off) for both, consistent with how every other risk-gated flag in the list defaults off (e.g. `blockApprovalOnCIFailureFlagName` at line 152-155) unless Phase 3 decides one should ship default-on.
3. **Read side** at the real call site: `config.LoadConfig().GetFeatureFlagWithDefault(name, defaultValue)` — this is the exact call `GetFeatureFlags` itself uses at line 271, and it's the pattern already established for gating actual behavior elsewhere in the codebase (e.g. `workspacePeersBlockFor`, line 113-118, calls `config.LoadConfig().GetFeatureFlag(workspacePeersNudgeFlagName)` directly — note the two-arg vs one-arg method name difference: `GetFeatureFlag` has no explicit default param (implicit `false`), `GetFeatureFlagWithDefault` takes one — use whichever matches whether the flag's `knownFeatureFlags` entry sets a non-`false` `defaultValue`).
4. **No `FeatureController` is required** unless the flag needs an in-process runtime toggle beyond a config read (e.g. something with live component state to start/stop) — most of the simple boolean-gated flags in the list (e.g. `terminalResyncStaggerFlagName`) have no wired controller at all; `SetFeatureController`/`SetStatusDetailProvider` (lines 244-259) are opt-in. For a rate-limiter-shaped or GraphQL-migration-shaped flag that's read once per call rather than needing to react to being toggled mid-session, a bare config read (step 3) is sufficient and simpler — skip the controller machinery unless Phase 3 finds a concrete need (e.g. resetting in-flight state when the GraphQL migration flag flips).
5. This is exposed automatically via `GetFeatureFlags`/`UpdateFeatureFlag` RPCs (already wired, no server.go changes needed) once added to `knownFeatureFlags` — satisfying "live-settable" without new plumbing.

## Open items / uncertainties flagged for Phase 3

- Exact GraphQL field list for `reviews` and `statusCheckRollup` (2b) needs verification against a live GitHub GraphQL schema query or the `gh api graphql` introspection, not just recalled schema shape.
- GraphQL cost model specifics (2c) are inferred from general GitHub API documentation familiarity, not a live fetch in this research pass — verify against `docs.github.com`'s GraphQL rate-limit page before finalizing the observability `resource` attribution design.
- Batching/grouping strategy for aliased multi-PR queries (2a) — the "grouped by repo" phrase in requirements.md needs a concrete design (one alias per PR vs. nested aliasing per repo); not resolved here, and GitHub's alias-count/query-depth ceiling for very wide queries needs confirming against current docs.
- Async observable gauge vs. synchronous recording for rate-limit-remaining (1a) — needs a design decision once Phase 3 picks how metrics get wired into a per-request transport (observable gauges are normally registered once at startup with a callback, which fits awkwardly with a per-response value from an ad hoc client transport).
- Pre-existing `otlptracegrpc`/`otlptrace` v1.39.0 vs. rest-of-otel v1.44.0 version skew in `go.mod` — not caused by this project, but worth a one-line mention if Phase 3 touches `go.mod` anyway.
