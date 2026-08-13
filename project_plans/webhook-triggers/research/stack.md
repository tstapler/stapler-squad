# Stack Research: Webhook / Event-Driven Session Creation

Agent: Stack (Agent 1) | Scope: cron scheduling, HMAC verification, outbound retry HTTP,
templating, and HTTP route registration for the webhook-triggers feature.

## Correction to requirements.md

`requirements.md` states: *"Confirmed by repo search: no `robfig/cron`/`gocron` dependency
... anywhere in `server/`."* **This is incorrect.** `robfig/cron/v3 v3.0.1` is already a
direct dependency (`go.mod:26`) and is already used in production for cron-based session
automation:

- [`server/workflows/scheduler.go`](server/workflows/scheduler.go) — `Scheduler` wraps
  `*cron.Cron`, uses `cron.WithParser(cron.NewParser(cron.Minute|cron.Hour|cron.Dom|cron.Month|cron.Dow))`
  (standard 5-field expressions, matching FR3 exactly), tracks `entryMap map[string]cron.EntryID`
  for add/remove-by-ID, and calls `CreateSession` via a narrow `SessionServiceInterface`
  (defined locally to avoid a `server/workflows` → `server/services` circular import — the
  same interface-scoping pattern `.claude/rules/interface-pollution-checklist.md` calls for).
  It already has `Start(ctx)`, `Stop()`, `Reload(ctx, wf)`, `Remove(workflowID)`, and
  `FireNow(ctx, wf, arg)` — i.e. dynamic add/remove without restart, which is exactly FR6's
  "no full redeploy" requirement.

**Implication for planning**: the `cron` trigger type (FR3, AC2) does not need a new
scheduler. It should reuse or directly extend `server/workflows/scheduler.go`'s `Scheduler`
(or generalize it to accept a `Trigger`-shaped source, not just `*ent.Workflow`), not
introduce a second cron dependency or a parallel scheduler type. This also changes the
research question for Agent 2 (architecture) — the design should show how the new trigger
config maps onto/reuses this existing `Scheduler`, not treat cron as greenfield.

## HMAC / webhook signature verification (FR2, FR4, AC1, AC8)

No existing HMAC verification code in the repo (`grep -rl "X-Hub-Signature\|hmac.Equal"` —
no matches; the two `sha256.New()` hits, `session/pipeline_engine.go:231` and
`session/tmux/tmux.go:1302`, are unrelated content-hashing, not signature verification).

**Recommendation: stdlib only, no new dependency.** `crypto/hmac` + `crypto/sha256` +
`encoding/hex` is sufficient and is the standard idiom for GitHub's
`X-Hub-Signature-256: sha256=<hex>` scheme:

```go
mac := hmac.New(sha256.New, []byte(secret))
mac.Write(body)
expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))
if !hmac.Equal([]byte(expected), []byte(received)) {
    return errInvalidSignature
}
```

`hmac.Equal` (not `==` or `bytes.Equal`) is required for constant-time comparison to avoid a
timing side-channel — this is the one detail worth calling out explicitly in the plan, since
it's an easy thing to get subtly wrong (e.g. a naive `subtle.ConstantTimeCompare` misuse or
a plain `==`). No third-party library adds value here; `crypto/hmac` is exactly this API.

## Outbound HTTP client with retry/backoff (FR7, FR8, FR9, AC4)

**No `hashicorp/go-retryablehttp` (or similar) is currently a dependency** (checked
`go.mod`/`go.sum` — no match). The repo has no existing generalized outbound-retry HTTP
client; the ~30 files matching `http.Post|http.NewRequest.*POST|retry|backoff` are either
unrelated (git/GitHub API calls via other means, poller loops with their own ad hoc
backoff, e.g. `session/worktree_pr_poller.go`, `session/pr_status_poller.go`) or use
one-shot calls.

**Recommendation: do not add `go-retryablehttp`.** FR8 requirements are narrow (best-effort,
bounded retry, must not block the session lifecycle transition) and don't need
retryablehttp's full feature set (configurable backoff policies, response-body replay,
context propagation across retries, etc.). A small hand-rolled retry loop is more in
keeping with `.claude/rules/interface-pollution-checklist.md`'s "don't reach for a
library/abstraction until the plain version is proven insufficient" spirit, and keeps the
callback dispatcher's behavior (bounded attempts, jittered backoff, timeout per attempt)
fully visible and testable without mocking a third-party client. Two building blocks
already in `go.mod` help:

- `golang.org/x/time v0.15.0` — already a dependency, has `rate.Limiter`, useful if callback
  dispatch needs a global outbound rate limit (unclear yet if needed — flag as an open
  question for the architecture agent).
- Standard pattern: `http.Client{Timeout: N}` + a small `for attempt := range maxAttempts`
  loop with `time.Sleep(backoff)` (or `context.WithTimeout` per attempt), dispatched via
  `go func() { ... }()` (or a bounded worker queue) so the caller (the lifecycle transition
  path) never blocks on delivery — satisfying FR8 directly. Log failures via the existing
  `log` package (used everywhere, e.g. `log.Error("[WorkflowScheduler] ...")` in
  scheduler.go) to satisfy FR9 ("failed deliveries are logged/surfaced, not silently
  dropped").

If retry logic grows non-trivial (e.g. exponential backoff with jitter across many callback
targets), revisit `go-retryablehttp` then — but start minimal per FR8's actual scope.

## Templating for `prompt_template` (FR4)

Repo search for `text/template` turns up exactly one hit, and it's a **deliberate
non-adoption**: [`session/pipeline_engine.go:253-256`](session/pipeline_engine.go#L253):

> `// renderTemplate performs fixed-placeholder substitution on tmpl using`
> `// strings.NewReplacer — deliberately NOT text/template: no conditionals, no`
> `// loops, not Turing-complete, to resist the "templating engine" rabbit hole`

That function (`renderTemplate`, `session/pipeline_engine.go:264`) uses a fixed allow-list
of 7 known placeholder names (`item_id`, `item_title`, `item_description`,
`criteria_index`, `criteria_count`, `criteria_text`, `repo_path`) with `strings.NewReplacer`,
specifically to avoid the security/complexity surface of a Turing-complete template
language, since the placeholder set is known and closed at write time.

**This precedent does not directly apply to FR4, and the plan should say why explicitly**
rather than silently diverging: FR4's `prompt_template` interpolates fields from an
**arbitrary inbound JSON payload** (`{{issue.key}}`-style, keyed by whatever the external
webhook sender includes) — the key set is not closed or knowable at write time the way
`renderTemplate`'s 7 fixed fields are. A fixed allow-list can't express "any field the
sender happens to send." This is the actual reason the issue's original spec calls for real
`{{issue.key}}` dotted-path syntax — it needs a general JSON→template binding, not a fixed
substitution table.

**Recommendation:** use stdlib `text/template` (not `html/template` — output is a prompt
string for an LLM session, not HTML to render in a browser, so HTML-escaping would corrupt
the prompt) with the parsed webhook JSON payload as the data context (e.g.
`map[string]interface{}` via `encoding/json.Unmarshal`, giving `{{.issue.key}}`-style
access, or `{{index .issue "key"}}` for keys with special characters). Mitigate
`text/template`'s Turing-completeness concern (the exact thing `pipeline_engine.go` avoided)
by:
- Not exposing arbitrary Go functions via `Funcs()` — use the zero-value `FuncMap` (default
  builtins only: `and`, `or`, `not`, `len`, `index`, etc. — no shell/file/network access).
  `text/template` alone cannot reach outside its data context; the risk model is different
  from e.g. Jinja2/ERB with code-exec sandboxes to escape.
- Validating the template parses at trigger-config write time (`template.New(...).Parse(tmpl)`
  returning an error rejects malformed templates before they're persisted) — this is cheap
  and prevents AC8-style "malformed input" from reaching runtime.
- Executing with a timeout / bounded output size as defense-in-depth against a pathological
  `{{range}}` over attacker-controlled payload data (unlikely but cheap to guard).

Flag for the architecture agent: whether `prompt_template` rendering should share any code
with `pipeline_engine.go`'s `renderTemplate`, or be a fully separate function (recommended —
they solve different problems: closed fixed-fields vs. open arbitrary-JSON) to avoid forcing
one function to serve two different security models.

## HTTP route registration pattern (webhook receiver, FR2/FR4)

ConnectRPC handlers are registered in `server/server.go`, but **plain (non-RPC) HTTP routes
already have an established idiom** — a `RegisterRoutes(mux *http.ServeMux)` method on a
handler struct, called from `server.go`'s setup sequence. Confirmed examples:

| Handler | File | Registered at |
|---|---|---|
| `PushHandler` | `server/services/push_handler.go` | `server/server.go:237` |
| `HookReceiver` (Claude Code hooks — closest precedent for an external-caller POST endpoint) | `server/services/hook_receivers.go:44` | `server/server.go:518` |
| `BacklogDebugSeedHandler` | `server/services/backlog_debug_seed_handler.go:41` | `server/server.go:583` |
| `EscapeCodeHandler` | `server/services/escape_code_handler.go:167` | `server/server.go:563` |
| `CircuitBreakerHandler` | `server/services/circuit_breaker_handler.go:82` | `server/server.go:573` |
| `AnalyticsHandler` | `server/handlers/analytics_handler.go:103` | `server/server.go:648` |

`server.go` uses Go 1.22+ method-specific mux patterns where relevant, e.g.
`srv.mux.HandleFunc("POST /api/v1/upload-image", ...)` (`server/server.go:523`).

**Recommendation:** a new `WebhookReceiver` (or similar) struct in `server/services/`
following this exact pattern:

```go
func (h *WebhookReceiver) RegisterRoutes(mux *http.ServeMux) {
    mux.HandleFunc("POST /webhooks/github", h.HandleGitHubPush)
    mux.HandleFunc("POST /webhooks/{slug}", h.HandleGenericWebhook) // FR4's per-trigger path
}
```

registered alongside the other `RegisterRoutes(srv.mux)` calls in `server/server.go`'s setup
sequence (near `hookReceiver.RegisterRoutes(srv.mux)` at line 518, since both are
externally-triggered POST receivers with signature/secret verification, conceptually
adjacent). Note: this is intentionally **not** a ConnectRPC service — GitHub/generic webhook
senders POST raw JSON per their own wire format (GitHub's push event schema, or arbitrary
third-party JSON for FR4), not a `connect.Request[T]`; a plain `net/http` handler on the
existing `*http.ServeMux` is the correct fit, matching every other externally-facing
non-RPC receiver in this codebase.

## Version / license / maintenance status of `robfig/cron/v3`

Not newly relevant to add — it's already vendored at `v3.0.1` (`go.mod:26`), which is also
the latest tagged release on the module (no newer major/minor exists as of this research;
v3 has been the stable API since 2019, MIT licensed, low-churn/maintenance-mode project —
appropriate for a stable, narrow-scope cron parser/scheduler with no need to chase a moving
target). No version bump needed; reuse as-is.

## Summary of dependency decisions

| Need | Decision | New dependency? |
|---|---|---|
| Cron scheduling | Reuse `server/workflows/scheduler.go`'s `*cron.Cron` (robfig/cron/v3, already vendored) | No |
| HMAC signature verification | stdlib `crypto/hmac` + `crypto/sha256` + `hmac.Equal` | No |
| Outbound retry HTTP | Hand-rolled bounded retry loop over `net/http.Client`; optionally `golang.org/x/time/rate` (already vendored) for global outbound rate limiting | No |
| `prompt_template` rendering | stdlib `text/template` (not `html/template`; not `pipeline_engine.go`'s `strings.NewReplacer`, whose fixed-placeholder model doesn't fit arbitrary JSON payloads) | No |
| Webhook HTTP route | Plain `*http.ServeMux` handler with `RegisterRoutes(mux)`, matching `HookReceiver`/`PushHandler`/etc. | No |

**No new third-party dependencies are needed for this feature.** Everything required is
either already vendored (`robfig/cron/v3`, `golang.org/x/time`) or covered by Go stdlib
(`crypto/hmac`, `text/template`, `net/http`).
