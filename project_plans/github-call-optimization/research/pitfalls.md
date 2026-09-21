# Research: Pitfalls & Risks — github-call-optimization

Agent 4 (Pitfalls). Findings go beyond requirements.md's own Rabbit Holes/Feasibility
Risks — those are assumed read and are not restated except where a deeper dig changes
their shape. Every claim below is grounded in either this repo's code (file:line) or a
web-search-verified GitHub API fact (cited).

## 0. Load-bearing finding: GraphQL migration and "conditional-requests-by-default" are in direct tension

**GitHub's GraphQL API does not support ETag/If-None-Match conditional requests or 304
responses at all — this is REST-only** (confirmed via GitHub's own community discussion
and REST docs: conditional GETs "not count against your primary rate limit" is a REST-API
statement with no GraphQL equivalent; "HTTP caching over GraphQL is not a simple problem,
and it's unlikely GitHub will ever support it").

This collides with the repo's own architecture:

- `github/etag_cache.go`'s `GetPRInfoConditional` (the function both pollers actually call
  every tick — `session/pr_status_poller.go:347`, `session/worktree_pr_poller.go:252`,
  not `GetPRInfoCtx` directly) does a conditional REST GET
  (`repos/%s/%s/pulls/%d` with `If-None-Match`), and only falls through to
  `GetPRInfoCtx` (the gh-CLI-shelling function Scope item 3 targets for GraphQL
  migration) as its "PR changed" full-refetch path.
- Requirements' Scope item 5 asks for "conditional-requests-by-default infrastructure"
  and the Constraints section requires reusing the shared `ETagCache` "per ADR-022"
  (`docs/adr/022-worktree-pr-poller-extends-pr-status-poller.md`) without doubling call
  volume.
- Scope item 3 asks to migrate `GetPRInfoCtx` itself to GraphQL and to batch a
  GraphQL query per poller tick aliasing all tracked PR numbers.

If "batch via GraphQL" is read as "replace the per-tick polling loop's REST+ETag call
with one aliased GraphQL query," the feature **loses conditional-request savings
entirely** — every poller tick becomes a full-cost GraphQL fetch for every tracked PR,
with no 304 path, which could easily *increase* steady-state call volume/cost despite
fewer physical round-trips, directly undermining Success Metric #2. The only way to keep
both wins is to keep `GetPRInfoConditional`'s REST+ETag path as the *primary* per-tick
mechanism and scope the GraphQL migration strictly to `GetPRInfoCtx`'s current caller —
the "PR changed" full-refetch fallback, and any other direct `GetPRInfoCtx` callers
outside the two pollers (grep `GetPRInfoCtx(` call sites before assuming pollers are the
only ones) — not a wholesale replacement of the tick loop. **This distinction must be
explicit in the plan**, since "migrate GetPRInfoCtx to GraphQL" and "batch a GraphQL
query per poller tick" read as the same sentence but are architecturally very different
asks once ETag/REST-only-caching is accounted for.

## 1. GitHub API pitfalls

### 1a. GraphQL cost is response-node-count-driven, not alias-count-driven — but the two correlate here, and GitHub added a second resource-limit layer in 2025

Per GitHub's GraphQL docs (verified via search): the primary limit is 5,000 points/hour,
and a query's cost is computed from **total nodes traversed** (each connection's `first`/
`last` multiplies down its subtree — e.g. 50 repos × 20 PRs × 10 comments = 10,000 nodes
from one branch alone), not simply "1 point per top-level alias." For the aliasing
technique this project wants (one query aliasing N tracked PR numbers, each a single
`repository(owner,name){ pullRequest(number){...} }` object-fetch, not a list
connection), **cost does scale roughly linearly with N** — but each aliased PR subtree
still carries its own nested connections (`reviews(last: 20)`, `commits(last: 1){
statusCheckRollup }`, mirroring `github/client.go`'s existing REST field list at
`client.go:266`+ and the existing per-user query at `github/user_pr_cache.go:516`'s
`reviews(last: 20)`/`commits(last: 1)`), so the per-PR-subtree cost is not exactly 1 —
budget accordingly, not "N aliases = N points."

Separately: GitHub shipped **GraphQL API resource limits** as a distinct changelog item
in September 2025 — a query that requests "very large numbers of objects," "deeply
nested relationships," or "large `first`/`last` across multiple connections at once" can
be terminated mid-execution with **`RESOURCE_LIMITS_EXCEEDED` and partial results**, a
failure mode independent of and additional to the points-based rate limiter. A batched
query aliasing every tracked PR (which, for an active multi-machine backlog-driven setup,
could be dozens) each carrying 2 nested connections is exactly the query shape this
newer limit targets. The already-flagged rabbit hole ("batching needs a smaller batch
size or a fallback split-query path") is correct, but the trigger condition is broader
than pure point-cost — plan for `RESOURCE_LIMITS_EXCEEDED` as its own distinct
error-classification branch (partial results returned, not a clean 4xx), not folded into
the existing rate-limit-detection logic (`isGHRateLimited` in `github/http_client.go:183`
only recognizes 429/Retry-After/`X-RateLimit-Remaining: 0` — none of which describe this
failure mode).

### 1b. GitHub's *documented* secondary-limit numbers give a hard ceiling to design admission control against

Verified via GitHub REST/GraphQL docs: secondary/abuse limits cap concurrency at **100
concurrent requests shared across REST+GraphQL**, **900 points/minute for REST**, **2,000
points/minute for GraphQL**, and **90s of CPU time per 60s wall-clock**. The existing
`pr_status_poller.go`'s `ConcurrentFetches` (default 5, `pr_status_poller.go:42`) is
nowhere near the 100-concurrent ceiling on its own — but it is **not the only concurrent
consumer of the same token**: `github/user_pr_cache.go:352`'s `fetch()` fans out one
goroutine per connected account/token with no shared limiter at all, and per the
requirements' own problem statement, multiple *machines* share one token outside this
process entirely. Priority-aware admission control (Scope item 2) that only gates
`DefaultRateLimiter`'s callers inside *this* process cannot see the other process's or
other machine's concurrent usage — the 100-concurrent ceiling is global to the token,
not per-process. Any admission-control design that assumes "we control all the
concurrency" will under-protect in exactly the two-machine scenario the requirements
call out as in-scope for "per-machine mitigation."

### 1c. Webhook dedup: the existing single-consumer dedup has a documented, un-closed race — a second consumer inherits it, doesn't just "generalize" it

`server/services/github_webhook_pr_fix.go:536-548`'s delivery-level dedup is **not**
`firstPRFixDelivery` (that's a `sync.Once`-per-event-type used only to log "first verified
delivery received" once per process lifetime — `github_webhook_handler.go:38-41`,
`:530-534` — not a dedup mechanism at all). The real dedup is
`h.fireEvents.ExistsByDeliveryID(ctx, deliveryID)`, and its own doc comment
(`github_webhook_pr_fix.go:536-537`) and the repository's doc comment
(`session/trigger_fire_event_repository.go:49-57`) state explicitly: this is **"a
separate, non-unique-index-backed check"** — i.e. a plain SELECT-then-proceed with no
atomic claim, unlike the push-event path's `(workflow_id, delivery_id)` unique index
(`session/ent/schema/trigger_fire_event.go:61`) which *does* get an atomic
insert-or-conflict via `claimAndFireTrigger`. Two genuinely concurrent deliveries of the
same `X-GitHub-Delivery` ID (GitHub's docs explicitly do not guarantee exactly-once
delivery, and a delivery can be redelivered) can both pass `ExistsByDeliveryID`'s read
before either's `persistTriggerFireEvent` write lands, both invoking
`prFixRouter.TriggerPRFixForEvent` concurrently. This is a real, currently-latent race
in the one existing consumer (mitigated today only by low duplicate-delivery frequency
in practice, not by design).

**Implication for this project's poller-invalidation feature (Scope item 4):** a second
consumer of the same webhook stream needs its *own* dedup key scoped to
"already invalidated cache entry for (repo, PR, event-kind)" — reusing
`ExistsByDeliveryID` as-is for poller invalidation would (a) inherit the same
check-then-act race, now with a second racing consumer on top of the first, and
(b) conflate two different "have I handled this?" questions (PR-fix's fire-a-workflow
semantics vs. a poller's invalidate-and-refetch semantics) under one delivery-ID
table not designed for multiple independent consumers. If a shared table is reused,
give it a `(consumer, delivery_id)` composite key with a real unique index (the pattern
the push path already has, at `trigger_fire_event.go:61`) rather than copying the PR-fix
path's known-non-atomic check.

### 1d. GitHub does not guarantee webhook delivery ordering — a poller invalidation must be resilient to out-of-order events, not just duplicates

A `pull_request` "synchronize" event and a later `check_run` "completed" event for the
same PR can arrive out of order (GitHub explicitly documents no ordering guarantee across
event types, and same-type ordering is also not guaranteed under retry/redelivery). A
naive "webhook arrived → mark this PR's poller cache stale → refetch" design is fine
under reordering (worst case: one extra refetch), but a design that tries to be clever
and **skip** a refetch based on the webhook payload's own state (e.g. "the payload says
CI passed, so update the cache directly instead of refetching") is not safe under
reordering — a stale "CI passed" webhook arriving after a newer "CI failed" webhook
would corrupt the cache with older data. Invalidate-and-refetch (treat the webhook purely
as a "go check now" signal, never as a source of truth to write into the cache directly)
sidesteps this; anything that shortcuts the refetch by trusting webhook payload content
as authoritative does not.

## 2. Go-specific pitfalls

### 2a. The 22 `gh`-CLI call sites are not homogeneous — 11 of them don't call `safeexec.CommandContext` at all

`github/client.go`'s 8 `gh` (and 3 `git`) sites call `safeexec.CommandContext` directly
(`client.go:299,573,600,644,667,700,722,742` for `gh`). But
`session/git/worktree_git.go`'s 11 `gh` sites (`worktree_git.go:71,84,301,356,390,404,674,
834,854,871,885`) go through `g.commandRunner().Run(ctx, dir, "gh", args...)` — an
interface (`session.tmux.CommandRunner`, `session/tmux/command_runner.go:52`) shared with
**`session/tmux`'s own tmux-server subprocess calls**, not a gh-specific seam. Its only
concrete implementation today, `LocalRunner.Run` (`command_runner.go:81-85`), calls
`safeexec.CommandContext(ctx, name, args...)` generically for *any* `name` (git, gh, or
tmux) — it has no gh-specific hook point. A wrapper built purely around
"`safeexec.CommandContext` call sites" (as Scope item 1 literally names it) will find 8,
not 22, unless it also instruments `CommandRunner.Run`/`Start` with a `name == "gh"`
branch. That branch point is shared, higher-traffic infrastructure (every tmux
has-session/kill-session/list-sessions call flows through the same interface) — touching
it for GitHub-specific tagging (call-site, origin, resource, repo) risks either (a)
polluting non-GitHub `CommandRunner` calls with irrelevant attempted GitHub tagging, or
(b) needing the 11 `worktree_git.go` call sites to pass GitHub-specific metadata down
through an interface (`CommandRunner`) explicitly designed (per `command_runner.go:10-21`
and ADR-002) to stay execution-shape-generic ("run this, get output") so a future
SSH-backed remote runner can substitute in with no signature change. Threading an
"origin"/call-site tag through `Run`'s signature is exactly the kind of interface
pollution ADR-002 was written to avoid — expect this to need a call-site-level wrapper
*above* `commandRunner().Run(...)` in `worktree_git.go` itself (tag at the call site,
not inside the generic runner), separate in shape from the `client.go` wrapper even
though both ultimately dispatch to `safeexec.CommandContext`.

### 2b. A future SSH-backed `CommandRunner` breaks the assumption that "this process's admission control sees every gh call"

`command_runner.go:10-21`'s own doc comment states the seam exists specifically so a
future SSH-backed implementation (`project_plans/ssh-remote-workspaces/`) can run these
same `gh`/`git` commands **on a different host**. Priority-admission-control and
per-call-site OTel spans built today around "gh calls happen in this process, against
this process's token/rate state" are a local-process view; once (if) a worktree's
`CommandRunner` is SSH-backed, its `gh` calls execute on a remote host, sharing whatever
GitHub token *that* host resolves — outside `DefaultRateLimiter`'s (or any
priority-admission-control state's) visibility entirely, even though the call still
counts against the same organization's/user's GitHub quota if it's the same token. Not
an immediate concern (SSH-backed workspaces aren't shipped yet), but worth a one-line
note in the design so a future implementer doesn't assume the new admission control is a
complete accounting of "all gh calls this fleet makes."

### 2c. Compile-time OTel auto-instrumentation already wraps *every* `safeexec.CommandContext` call — with a span, in a separate opt-in build

`instrumentation/otelc/safeexec/hook.go` (build-tagged `otelcauto`) already starts a span
per `safeexec.CommandContext` invocation, named for the subprocess, with
`telemetry.AttrSubprocessCommand`/`AttrSubprocessArgCount` attributes — but only in the
separately-built `stapler-squad-otel` binary (`docs/how-to/enable-otel-auto-instrumentation.md`;
this binary is structurally excluded from `ci`/`ready`/`quick-check`/`install-service` by
the `otel-auto-isolation-guard`, so it never reaches production). Two consequences:

- The **default production binary has zero existing subprocess-level OTel
  instrumentation** — this project's gh-CLI wrapper is not adding a second layer on top
  of an existing one in the binary that actually runs; it's the first.
- Anyone verifying this feature's spans by running `stapler-squad-otel` (the documented
  way to see any span at all today, per `enable-otel-auto-instrumentation.md`) **will
  see two nested spans per `gh` invocation** — the generic auto-instrumented one and the
  new gh-specific one — once both exist. That's expected/correct as trace *nesting*, not
  a bug, but if the new work also emits a **metric** (a counter, not just a span) at the
  gh-CLI wrapper layer, verify there is no equivalent auto-instrumented metric already
  being emitted in that build path that would double-count when both binaries' code
  paths are compiled together in some future unification — currently there is none
  (the otelc hook only creates spans, no metrics), so this is a "watch for it," not a
  live bug today.

### 2d. `http.RoundTripper` wrapping: `rateLimitTransport` already exists and is already careful — the risk is in what gets *added* to it

`github/http_client.go:40-56`'s `rateLimitTransport.RoundTrip` already handles the classic
gotchas correctly: it doesn't read/consume `resp.Body` itself (only headers), so it
doesn't double-read the body or interfere with callers' own `io.Copy`/`json.Decode` on
`resp.Body`, and it passes `req` through unmodified (context cancellation propagates
normally via `t.next.RoundTrip(req)`, whose context is unmodified — the transport never
wraps `req.Context()`). The risk for Scope item 1's "OTel spans + metrics on
`ghHTTPClient`'s RoundTripper" is if the new tracing wrapper is layered as *another*
`RoundTripper` around `rateLimitTransport` (composing transports) rather than
instrumentation added inside the existing `rateLimitTransport.RoundTrip` body: a new
wrapper that reads `req.Body` for span attributes (e.g. logging the GraphQL query text)
must restore it via `io.NopCloser(bytes.NewReader(...))` after reading, or the *next*
transport in the chain (`rateLimitTransport`, then `http.DefaultTransport`) gets an
already-drained body — this is the single most common `RoundTripper`-chaining bug in Go.
Given `rateLimitTransport` is already a working, tested seam, the lower-risk path is
extending it in place (add span/metric calls inside its existing `RoundTrip`) rather than
composing a second `RoundTripper` around it.

### 2e. Cardinality: `repo` is genuinely bounded at Tyler's scale; `call_site` and a raw PR-body/query-text label are not

Given this is a single-operator tool against a bounded set of repos Tyler actively works
in (dozens at most, not thousands), a `repo` tag is not a real cardinality risk — this
part of the "is this overthinking it?" question in the brief can be answered **no, not a
risk, don't spend design effort bounding it**. The two things worth actually checking
before shipping: (1) `call_site` must be a fixed enum of wrapper call sites (the ~30
named functions across `client.go`/`worktree_git.go`/pollers), not a free-form string
built from e.g. a stack trace or a formatted function+line — an enum of ~30 values times
~4 origins times a bounded repo count is a few hundred series, trivially fine for any
backend; (2) nothing in the GraphQL migration should end up tagging a span/metric with
the *PR number* or *query text* as a label (as opposed to a span *attribute*, which OTel
distinguishes from a Prometheus-style metric label) — PR numbers are unbounded over time
and would be the actual cardinality bomb, not `repo`. This distinguishes "attribute on a
trace span" (fine, cardinality-unconstrained) from "label on a metric" (must stay
bounded) — a distinction worth stating explicitly in the plan since OTel's API makes both
look like the same `WithAttributes(...)` call syntactically.

### 2f. `safeexec.CommandContext` already solves zombie accumulation; it does not solve output-buffering-under-cancellation

`executor/safeexec/safeexec.go` pre-sets `WaitDelay` (2s) specifically to bound
`cmd.Wait()` blocking on a held-open pipe after `SIGKILL` — the zombie-process hazard is
already handled repo-wide, nothing new needed there. What it does *not* address: `gh pr
view --json ...`'s stdout is read via `cmd.Output()` (`client.go:301`) or
`.CombinedOutput()` (`command_runner.go:84`) — both buffer the *entire* output in memory
before returning, and both are all-or-nothing: if `ctx` is cancelled mid-write (e.g. a
poller tick's per-fetch context times out while `gh` is mid-response), the caller gets a
context-cancellation error with **no partial output**, which is fine for today's
"call fails, log and retry next tick" pattern but is worth confirming still holds once a
GraphQL migration's aliased query response is larger (more PRs' worth of JSON to buffer)
— a slow/cancelled read now discards more accumulated work per cancellation than before.

## 3. Rollout/migration-specific pitfalls

### 3a. Flipping priority-admission-control ON mid-session: in-flight requests aren't the risk, the *next* tick after flip is

`DefaultRateLimiter` (per BUG-080/081's precedent) is a single package-level
`*RateLimiter` already shared by every caller. Any new priority-admission-control state
added "to/beside it" (per the Feasibility Risk already flagged) that is *also* a
package-level global inherits the exact same test-pollution hazard those two bugs fixed
for the existing limiter — a new global needs the same `ResetForTest`-style exported
reset helper (`github/testing.go`'s existing `ResetRateLimiterForTest` pattern) from day
one, not bolted on after the first flaky-CI report. Beyond tests: mid-session,
in-flight *requests* aren't corrupted by a flag flip (an HTTP request already dispatched
before the flip completes normally either way) — the real risk is state
**initialization order** at the flip instant: if background-origin callers are already
mid-backoff (e.g. `WaitIfLimited`-style sleep) under the *old* undifferentiated gate when
the flag flips, and the new priority-aware gate's initial state assumes "no one is
currently waiting," an interactive call arriving in that window could be double-gated
(blocked by both the stale wait and the new gate) or, worse, under-gated if the new
gate's zero-value state defaults to "not limited" while the old gate still has requests
genuinely rate-limited. Concretely: **initialize the new gate's state from
`DefaultRateLimiter.IsLimited()`'s current value at flip time**, don't let it start cold.

### 3b. `PRInfo` schema parity between gh-CLI-shaped and GraphQL-shaped population is a real risk, but it's a mapping-correctness risk, not a cache-versioning risk

`ETagCache` (`github/etag_cache.go:17-19`) is **in-memory only** (`sync.Map`, no disk
persistence found in this package) and scoped to one process's lifetime — so "what
happens to already-cached entries when `GetPRInfoCtx` migrates" has a simpler answer than
the assignment's framing suggests: there is no on-disk cache to version or migrate: a
process restart (which any code deploy already requires) empties it naturally. The
actual risk is narrower and more mundane: `PRInfo` (`github/client.go:50-76`) has fields
today parsed from `gh pr view --json ...`'s specific JSON shape (`Mergeable string` from
gh's `mergeable` field, `Author string` flattened from a nested JSON object, `Labels
[]string` flattened from `[]{name}` — see `client.go`'s post-`json.Unmarshal` flattening
code right after the struct). GitHub's GraphQL schema represents several of these
differently (`mergeable` is a GraphQL enum `MERGEABLE`/`CONFLICTING`/`UNKNOWN` — need to
confirm it matches gh CLI's string values exactly, not assume it does; `author` is a
`Actor` interface type requiring an inline fragment to get `login`). A GraphQL-populated
`PRInfo` that gets even one field's mapping subtly wrong (e.g. an empty string vs. a
different enum casing) is not a cache-corruption bug — it's silently wrong data reaching
`DerivePRPriority` (`github/priority.go:21`), which switches on exact lowercase string
comparisons (`priority.go:26,44-46,51-52`) — a mismatched `CheckConclusion` casing from a
new GraphQL mapping path would silently misclassify PR priority (e.g. a real CI failure
read as "pending" forever) with no error surfaced anywhere. **Require field-by-field
parity tests comparing GraphQL-populated vs. gh-CLI-populated `PRInfo` for the same real
PR** before cutting over, not just "it compiles and returns a `*PRInfo`."

### 3c. Flag-scoped rollout for the GraphQL migration needs both code paths live simultaneously, which the ETag cache doesn't currently support cleanly

If `GetPRInfoCtx`'s GraphQL migration is gated behind `server/features/flags.go`'s
live-settable mechanism (per Constraints), the flag can flip while the *other* poller
(if `PRStatusPoller` and `WorktreePRPoller` ever read the flag independently rather than
from one shared read) is mid-tick on the old path — not a correctness bug given 3b's
"cache is process-lifetime, not versioned," but a **transient logging/debugging
confusion** risk: two pollers, sharing one `ETagCache`, populating entries via two
different code paths (gh-CLI JSON parse vs. GraphQL response parse) in the same few
seconds around a flag flip, makes any "why did this PR's priority just look different"
investigation harder without emitting which population path each `ETagCache` entry came
from. Cheap mitigation: tag the OTel span (from Scope item 1, already being added) with
which path populated a given entry, so the flag-flip window is diagnosable after the
fact rather than only reproducible by re-flipping the flag under a debugger.

## 4. General failure patterns for this feature class, applied concretely

1. **Token-bucket/leaky-bucket admission control that "fails closed" under its own bug
   is worse than no admission control** — this is the Feasibility Risk already named,
   but the concrete well-known failure shape (seen repeatedly in rate-limiter libraries,
   e.g. `golang.org/x/time/rate` misuse reports) is: a burst allowance computed once at
   construction and never replenished correctly under clock skew or a paused process
   (this process can be suspended/resumed on a laptop, not just a server) silently
   starves *all* traffic, interactive included, indistinguishable from a hung network.
   Concretely test: suspend-and-resume (or a simulated large `time.Now()` jump) behavior
   for whatever bucket/window implementation gets chosen — laptops sleeping mid-poll is
   a real condition for this specific (personal, multi-machine) deployment, not an edge
   case to skip.
2. **Webhook-driven cache invalidation that treats "no webhook received" as "nothing
   changed" rather than "unknown"** is the single most common bug in this feature class
   across the ecosystem (it's why every mature system — Stripe, GitHub Apps themselves —
   documents "always run a reconciliation poll, webhooks are a latency optimization, not
   a source of truth"). The requirements already flag this as a rabbit hole
   ("fallback ticker must be a genuine safety net") — the concrete failure mode to design
   against is a webhook subscription that silently stops delivering (e.g. this
   project's own public-reachability tunnel, per
   `docs/how-to/expose-github-webhook-endpoint.md`, going down) with **no signal
   distinguishing "quiet because nothing changed" from "quiet because delivery is
   broken."** Emit a metric/log for "time since last webhook delivery of type X" so a
   silent tunnel failure is observable before the widened fallback ticker (Scope item 4)
   is the only thing standing between a broken tunnel and stale PR status for the full
   new (wider) interval.
3. **OTel instrumentation retrofits regress performance on the exact hot path they're
   meant to observe**, because "just add a span" reads as free but a `tracer.Start()`
   call that isn't sampled-out still allocates and, more importantly, a *synchronous*
   metric export or a span exporter with a blocking queue-full policy inserted into
   `RoundTrip` or `CommandRunner.Run` adds real latency to every GitHub call, including
   ones on a poller's already-tight per-fetch context deadline. The
   `github-api-usage-tracking` pitfalls doc (`project_plans/github-api-usage-tracking/
   research/pitfalls.md`, §3c) already made exactly this point for a *storage* hook on
   the same seam; the same rule applies unchanged to an *OTel* hook on the same seam —
   whatever library is used for the new metrics/spans, confirm its default exporter
   behavior on a full/unavailable collector is non-blocking (drop, don't block) before
   wiring it into `rateLimitTransport.RoundTrip` or the new gh-CLI wrapper.

## What's still valid vs. stale from the earlier `github-api-usage-tracking` pitfalls pass

That pass (2026-08-10) is now **stale on its central premise** (§0 there: "no
`rateLimitTransport` exists, `DefaultRateLimiter.Update()` has zero callers") —
`rateLimitTransport` ships today (`github/http_client.go:36-56`), fully wired, and is the
thing this project's OTel instrumentation (Scope item 1) extends rather than builds from
scratch. Its downstream findings that remain valid and directly relevant here:
double-counting risk if instrumentation is added at both the transport layer and
individual call sites (§1b there — same shape as this doc's §2a/§2c), the
`RWMutex`/atomic-counter concurrency-primitive guidance (§2a there), and the "hot-path
hook must never do synchronous I/O" rule (§3c there, restated as this doc's §4.3). Its
config-hot-reload findings (§4) are orthogonal to this project's scope (poll intervals,
not GitHub call optimization) and not relevant here.
