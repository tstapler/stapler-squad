# Research: Build vs. Buy — PR Event Webhooks (check_run/workflow_run/pull_request_review/issue_comment)

Scope: only the NEW surface this item adds on top of the existing
`/webhooks/github` receiver. The base receiver (HMAC verification, delivery
dedup, cron scheduling precedent, outbound-callback retry) was already
decided in `project_plans/webhook-triggers/research/build-vs-buy.md` and is
not re-litigated here — see its Summary Table. This item **does not add** an
outbound callback dispatcher or a cron trigger, so those rows don't apply.

## 1. GitHub event payload parsing (check_run, workflow_run, pull_request_review, issue_comment)

**Confirmed by reading the code**: `server/services/github_webhook_handler.go`
does not use typed structs at all. `extractGitHubRepoAndBranch` (line 135)
takes `payload map[string]interface{}` and pulls two fields via type
assertion (`payload["repository"].(map[string]interface{})`,
`repoObj["full_name"].(string)`, `payload["ref"].(string)`).
`readAndDecodeWebhookBody` (`server/services/webhook_trigger_common.go:64`)
decodes the raw body into that same `map[string]interface{}` shape — there is
no `json.Unmarshal` into a named struct anywhere in this file. This is a
deliberate minimal-surface pattern, not an oversight: the handler only reads
the 1-3 fields it needs per event type rather than modeling GitHub's full
webhook schema.

**`google/go-github` dependency check**: confirmed absent — `grep -n
"go-github" go.mod go.sum` returns nothing, and `go.mod`'s full dependency
list has no GitHub API client of any kind. Adding it would be a new,
non-trivial dependency (the library today is one of the larger typed API
clients in the Go ecosystem — REST + GraphQL surface, its own pagination
helpers, auth transports) pulled in for a narrow need: typed structs for 4
webhook payload shapes, of which this feature only reads a handful of fields
from each (`check_run.conclusion`, `check_run.pull_requests[].number`,
`workflow_run.conclusion`, `workflow_run.pull_requests[].number`,
`pull_request_review.state`, `pull_request_review.pull_request.number`,
`issue_comment.issue.pull_request` presence + `comment.user.type`).

- **Pros of `go-github`**: `github.ParseWebHook(eventType, payload)` +
  `github.CheckRunEvent`/`WorkflowRunEvent`/`PullRequestReviewEvent`/
  `IssueCommentEvent` structs would save writing ~4 small struct
  definitions, and get GitHub's full field set (including fields this
  feature doesn't need) with compile-time typing instead of
  `map[string]interface{}` assertions that fail silently (wrong type → zero
  value, not an error) unless each extraction function checks `ok` carefully
  (the existing code does check `ok`, so this isn't a correctness gap today,
  just a verbosity one).
- **Cons of `go-github`**: New dependency with real weight (transitive
  deps, larger attack/CVE surface, a client type (`github.Client`) this
  feature doesn't need — only the webhook-payload structs and
  `ParseWebHook` are relevant, so most of the library would be dead
  weight). It also doesn't eliminate hand-written code — call sites still
  need to type-switch on the `interface{}` `ParseWebHook` returns and pull
  the same handful of fields, just off a typed struct instead of a map.
  Introducing it diverges from this file's own established pattern
  (`extractGitHubRepoAndBranch`-style minimal extraction) for marginal
  benefit on 4 event types this pass touches once.
- **Verdict: Not recommended.** Hand-roll 4 small extraction functions
  (`extractCheckRunConclusionAndPR`, `extractWorkflowRunConclusionAndPR`,
  `extractPullRequestReviewState`, `extractIssueCommentContext`) following
  the exact `extractGitHubRepoAndBranch` pattern already in this file —
  same file, same style, ~10-15 lines each. This is squarely "a few
  fields, not an algorithm" (see §4). Revisit only if a future pass needs
  GitHub's *outbound* REST/GraphQL API (posting comments, querying check
  runs) for reasons beyond webhook parsing — that's the point at which
  `go-github`'s client, not just its structs, would pay for itself.

## 2. Public webhook reachability / tunneling for THIS endpoint

`.claude/docs/slack-phase2-public-reachability.md` establishes the pattern:
never tunnel the whole port, front the tunnel with a path-scoped local
reverse proxy (nginx example given), register only
`/api/hooks/slack-interactive` publicly. `/webhooks/github` needs the same
treatment for real GitHub deliveries (Slack calls happen from Slack's
servers; GitHub deliveries happen from GitHub's servers — same "genuinely
internet-facing, existing localhost-only trust boundary doesn't apply"
shape, per that doc's own framing).

Options considered for whether something more turnkey fits *this* case
specifically (single-operator, local-instance deployment — no multi-tenant
SaaS scenario, per `.claude/docs/state-isolation.md`):

- **Replicate the exact ngrok+nginx path-scoping recipe** (as documented for
  Slack Phase 2).
  - Pros: Already documented, already a checklist the operator has
    presumably run once for Slack. Reusing one nginx config with two
    `location` blocks (`/api/hooks/slack-interactive` and
    `/webhooks/github`) instead of standing up a second proxy/tunnel is the
    natural extension if the operator already has this running — one tunnel
    process, one nginx instance, two scoped paths.
  - Cons: ngrok free tier rotates the public URL on every restart, which
    means the GitHub webhook URL registered in the repo's webhook settings
    goes stale on every tunnel restart (same limitation as Slack Phase 2)
    unless a paid ngrok static domain is used.
  - **Verdict: Recommended** — as an *extension* of the existing setup, not
    a new mechanism. If the operator already runs the Slack tunnel, add one
    more `location` block for `/webhooks/github` to the same nginx config
    and reuse the same ngrok process (ngrok can front multiple paths through
    one nginx listener). If the operator does *not* already have Slack
    Phase 2 running, this is still the lowest-new-moving-parts option since
    the pattern and docs already exist in this repo.

- **GitHub App with built-in webhook redelivery**: GitHub Apps get
  automatic retry/redelivery semantics from GitHub's side (failed
  deliveries are queryable and manually or automatically redeliverable via
  the App's "Advanced" webhook deliveries UI/API) plus a scoped, App-level
  webhook secret.
  - Pros: Redelivery is a genuine advantage this item's requirements
    already care about (Goal 3: keep `PRStatusPoller` as backstop for
    "missed, delayed, or fails signature verification" deliveries) —
    GitHub's own redelivery UI/API reduces how often that backstop is
    needed for deliveries that *did* eventually succeed on GitHub's side
    but hit the receiver while it was down.
  - Cons: **Does not address reachability at all** — a GitHub App's webhook
    URL still has to be a real internet-reachable endpoint; App-based
    webhooks reach the same "your `POST /webhooks/github` needs to be
    publicly callable" requirement as a repo-level webhook. Converting from
    a repo webhook to a full GitHub App is also a materially larger scope
    change (App registration, installation flow, different auth/permissions
    model) than this item's goals ask for — Goal 5 only asks to gate new
    *event-type handling* behind a flag, not to replace the webhook
    delivery mechanism.
  - **Verdict: Not recommended for this pass** — orthogonal to the
    reachability question, and a scope expansion beyond what the
    requirements ask. Worth a note for a future item if delivery
    reliability becomes a recurring problem, not this one.

- **smee.io (or a self-hosted smee-client) for local dev**: a
  webhook-proxy service purpose-built for exactly "GitHub webhook → local
  dev machine" without exposing a port.
  - Pros: Zero infra to stand up (`npx smee-client --url <smee-channel>
    --target http://localhost:8543/webhooks/github`), free, widely used
    for GitHub App/webhook local development, naturally path-scoped since
    it only forwards to the one target URL you give it.
  - Cons: smee.io itself is a third-party relay — every payload transits
    smee's servers unencrypted-at-rest-on-their-side (same data-residency
    concern the sibling doc raised for Hookdeck/Zapier, §2, just smaller
    blast radius since it's dev-only). Not intended for production/durable
    use — smee's own docs frame it as a development tool, and this
    project's target deployment is a standing local instance receiving
    real GitHub deliveries continuously, not a dev loop.
  - **Verdict: Viable, but only as a *local development/testing* aid**,
    not the production reachability story — e.g. useful during
    implementation to iterate on the handler without registering/re-
    registering a real webhook URL each time. Document it as a dev-loop
    tip, not the recommended production path.

- **Cloudflare Tunnel with path rules**: `cloudflared` can expose a named
  tunnel with ingress rules that route by hostname *and* path to a local
  service, without a separate reverse proxy in front.
  - Pros: Built-in path-scoping (ingress `path:` matching) removes the need
    for a separate nginx instance just to scope the path — one YAML config
    does what ngrok+nginx needs two processes for. Persistent named tunnel
    (no URL rotation on restart, unlike ngrok free tier) if the operator has
    a domain to attach it to. No account-tier URL-rotation problem.
  - Cons: Requires a Cloudflare account + a domain routed through
    Cloudflare DNS — more setup than ngrok for an operator who doesn't
    already have that (ngrok needs neither). Introduces a second
    tunnel-vendor pattern alongside the ngrok one already documented for
    Slack Phase 2 (inconsistent operator experience if some hooks use ngrok
    and others use Cloudflare).
  - **Verdict: Viable**, and arguably technically nicer (native path
    scoping, stable URL) — but **recommend against introducing it for this
    item specifically** given ngrok+nginx is already the established,
    documented pattern in this repo for the structurally identical Slack
    Phase 2 problem. Consistency with existing operator setup outweighs
    Cloudflare's marginal technical edges here. Worth a standalone future
    ADR if the ngrok URL-rotation pain becomes a recurring complaint across
    *multiple* hooks (Slack + GitHub + any future one) rather than solved
    per-hook.

**Recommendation for this item**: extend the existing ngrok+nginx path-scoped
recipe (§2 first option) with one more `location = /webhooks/github { ... }`
block, document it as a checklist addition to (or cross-reference from) the
existing `.claude/docs/slack-phase2-public-reachability.md`-style doc.
Mention smee.io as an optional local-dev convenience in the implementation
notes, not as the shipped guidance.

## 3. Event routing/dedup — bespoke type-switch vs. a webhook-routing library

The existing receiver already has all the routing/dedup machinery this item
needs: `readAndDecodeWebhookBody` (signature verification + delivery-ID
dedup + JSON decode, `server/services/webhook_trigger_common.go:64`) and
`claimAndFireTrigger` (`webhook_trigger_common.go:158`) are shared,
already-tested helpers. Adding new event types is a matter of reading
`X-GitHub-Event` (not currently read at all — `Handle` in
`github_webhook_handler.go` never inspects it, since `push` is GitHub's
only event type sent without ambiguity today) and type-switching to the
right extraction + dispatch path, reusing the same `readAndDecodeWebhookBody`
call.

- **`go-playground/webhooks`** (or similar Go webhook-routing
  micro-libraries): provide a `webhooks.New(...)` handler that parses the
  `X-GitHub-Event` header, verifies signature, and dispatches to registered
  per-event-type callbacks.
  - Pros: Would replace the header-read + type-switch boilerplate this item
    needs to add, and ships its own typed payload structs (same tradeoff as
    §1's `go-github` discussion — most such libraries wrap `go-github`'s or
    their own duplicate structs).
  - Cons: **Would replace, not extend, the existing bespoke
    signature-verification + dedup pipeline** — `readAndDecodeWebhookBody`
    already does HMAC verification (via `VerifyGitHubSignature`, using
    `hmac.Equal` per the sibling doc's explicit correctness callout),
    delivery-ID dedup, and `TriggerFireEvent` audit persistence, all wired
    to this project's own `ent`-backed repositories and multi-workflow
    (multi-secret) matching model (`repoCandidates` loop trying each
    `Workflow`'s own secret — a shape no generic webhook library
    anticipates, since it assumes one fixed secret per app). Swapping in a
    library here means either (a) using only its payload-struct/routing
    layer and discarding its own signature verification (most of the
    library's value), or (b) replacing the already-built, already-tested
    per-workflow-secret matching logic with the library's single-secret
    model — a regression. Either way this is net-negative churn for a
    receiver that's ~130 lines and already working.
  - **Verdict: Not recommended.** Extend the existing type-switch in
    `Handle` (or split into a per-event-type dispatch table keyed on
    `X-GitHub-Event`, still calling the shared `readAndDecodeWebhookBody`)
    rather than adopting a routing library. This mirrors the sibling doc's
    stdlib-first verdict on HMAC (§1b there) — the value-add of a library
    here is entirely in payload structs and dispatch sugar, and the
    dispatch sugar doesn't fit this repo's per-workflow-secret model
    without modification anyway.

## 4. LLM-generated bespoke logic vs. library — event-shape conditionals

Confirmed this is the fast section the prompt expected. The logic in
question — `check_run.conclusion == "failure"`,
`workflow_run.conclusion == "failure"`,
`pull_request_review.state == "changes_requested"`,
`issue_comment.comment.user.type == "Bot"` (or similar bot-exclusion) — is
straight-line conditional logic over fields already extracted as plain Go
strings, not an algorithm with edge cases comparable to cron-expression
parsing or constant-time HMAC comparison (the two cases the sibling doc
correctly flagged as *not* safe to hand-roll). There is no timing-sensitive,
security-sensitive, or combinatorially-tricky parsing involved — it's value
equality checks against a small fixed set of GitHub's documented enum
values.

- **Verdict: Bespoke code is clearly correct here — Recommended.** No
  library evaluation needed beyond confirming (as this section does) that
  the "textbook LLM-generated-code correctness bug" concerns the sibling
  doc raised for HMAC comparison and cron parsing don't apply to this kind
  of logic. The one thing worth calling out for implementation/review: pin
  the exact GitHub enum string values in a small `const` block (e.g.
  `conclusionFailure = "failure"`, `reviewStateChangesRequested =
  "changes_requested"`) rather than repeating string literals at each call
  site, and add a table-driven test enumerating GitHub's documented
  `conclusion`/`state` values (including ones that should NOT trigger,
  like `neutral`, `cancelled`, `commented`) — ordinary Go testing hygiene,
  not a build-vs-buy concern.

## 5. Alternative: just lower PRStatusPoller's interval instead of adding webhooks

**The alternative, stated fairly**: `DefaultPRStatusPollerConfig.PollInterval`
(`session/pr_status_poller.go:41`, default `60 * time.Second`) is a single
config value. Lowering it to, say, 10-15s is a one-line change: no new HTTP
route, no HMAC verification code, no public-facing attack surface, no
tunnel/reverse-proxy to stand up and maintain (the exact operational burden
requirements.md's own Goal 4 flags for a single-operator box). If the goal is
"react faster than today's worst-case 60s," this gets there for close to zero
implementation cost.

**Its real limits**:

- **Rate-limit budget scales linearly with poll frequency.** `checkAllSessions`
  (`session/pr_status_poller.go:204`) makes one ETag-conditional `gh`/API call
  per tracked `pr_pending` item, every tick, regardless of whether anything
  changed — a conditional request that returns 304 still counts against
  GitHub's primary rate-limit budget, it just costs no *processing* work.
  Halving the interval doubles the call volume for the same tracked-item
  count; cutting 60s → 10s is a 6x multiplier. On a single-operator box where
  the same PAT/`gh` auth may also serve other repos' polling, human `gh`
  usage, and CI, that headroom is not free, and `pitfalls.md` §5 already notes
  this repo has **no engineered rate-limit tracking or alerting today** — a
  quota exhaustion from over-aggressive polling would surface only as opaque
  API 403s.
- **There's a floor below which polling can't safely go.** `checkAllSessions`
  is bounded by a `ConcurrentFetches` semaphore (default 5,
  `session/pr_status_poller.go:42`) and a `CallTimeout` of 10s per call
  (`:43`); nothing in `pollLoop` (`:184-192`) guards against a slow check
  cycle overlapping the next ticker fire. Pushing `PollInterval` below
  roughly the time one full `checkAllSessions` cycle takes to complete risks
  overlapping runs against the same items — a correctness risk this
  alternative would introduce, not just a diminishing-returns one.
- **A webhook has neither limit.** GitHub pushes an event only when something
  actually changes — idle PRs cost zero requests, not one-per-tick — and
  delivery latency is sub-second, not bounded by any interval at all. Polling
  harder narrows the gap; it can't close it, and it gets more expensive, not
  cheaper, the more it narrows it.

**Recommendation**: keep webhooks as this project's goal, but treat a poll-
interval reduction as an **independent, low-cost complementary step, not a
substitute** — and ship it regardless of this feature's timeline. Concretely:
lowering `PollInterval` from 60s to ~15-20s costs one config change, adds no
attack surface, and meaningfully shortens the poller's worst-case latency for
every operator, including ones who never complete the Goal 4
tunnel/reverse-proxy setup. It does not, however, remove the case for
webhooks: this project's marginal implementation cost is genuinely low
precisely because it *extends* already-built, already-tested infrastructure
(`GitHubWebhookHandler`'s HMAC verification pipeline, `TriggerFireEvent` audit
trail, `remediatePRFixWithBackoffGate`'s dedup gate — see §1-4 above and
`architecture.md`) rather than standing up something new, so the "far lower
cost/risk" framing in this section's premise undersells how much of the
webhook path's cost is already sunk. Net: do both, in either order, but don't
let polling-harder substitute for webhooks — it only ever narrows a gap that
webhooks close outright, and it gets more expensive as it narrows it.

## Summary Table

| Component | Decision | Verdict |
|---|---|---|
| check_run/workflow_run/pull_request_review/issue_comment payload parsing | Hand-roll 4 small extraction functions in `github_webhook_handler.go`, matching `extractGitHubRepoAndBranch`'s existing `map[string]interface{}` pattern; do not add `google/go-github` | Not recommended (`go-github`) / Recommended (hand-roll, matches existing style) |
| Public reachability | Extend the existing ngrok+nginx path-scoped recipe (`.claude/docs/slack-phase2-public-reachability.md`) with one more `location` block for `/webhooks/github`; mention smee.io as a dev-only convenience | Recommended (extend existing pattern) |
| GitHub App redelivery semantics | Orthogonal to reachability, larger scope than this item asks for | Not recommended (this pass) |
| Cloudflare Tunnel with path rules | Technically viable, inconsistent with existing ngrok-based operator setup | Viable, not recommended for this item |
| Event routing/dedup library (`go-playground/webhooks` etc.) | Extend the existing `readAndDecodeWebhookBody`/type-switch dispatch in `Handle`; do not adopt a routing library | Not recommended |
| Bespoke conditional logic (conclusion/state/bot-comment checks) | Plain Go equality checks + const block + table-driven test — not algorithmically risky like HMAC compare or cron parsing | Recommended (bespoke) |
| Lowering `PRStatusPoller.PollInterval` instead of building webhooks | Ship as an independent, low-cost complement (e.g. 60s → ~15-20s); do not treat as a substitute — rate-limit cost scales linearly with poll frequency and has no latency floor near a webhook's | Complementary, not a substitute (§5) |
