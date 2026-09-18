# Pitfalls & Risks: webhook-triggers

Agent 4 (Pitfalls) research for Phase 2. Scope: inbound webhook receivers that create
sessions, cron-based triggers, outbound lifecycle callbacks, and completion-triggered
session chaining. Findings are grounded in this repo's actual code where cited;
general security/reliability guidance is marked as such.

## 0. Prior art already in the repo — read this before designing anything new

The requirements doc (`requirements.md:17-18`) states "no `robfig/cron`/`gocron`
dependency ... anywhere in `server/`" — **this is stale/incorrect and matters for
scope**. `server/workflows/scheduler.go` already implements a `robfig/cron/v3`-backed
`Scheduler` wired to ent-backed `Workflow` records (`session/ent/schema/workflow.go`),
with `Start`/`Reload`/`Remove`/`FireNow` and a `ValidateCronExpression` helper. It is
driven today by the `create_workflow`/`run_workflow` MCP tools
(`server/mcp/tools_workflow.go`), not by anything user-facing under "triggers." FR3
(cron triggers) should almost certainly **extend this existing `Scheduler`** (new
trigger-type row referencing it, or a second lightweight scheduler instance reusing
`ValidateCronExpression` and the same `robfig/cron` idiom) rather than hand-rolling a
second cron engine — but every pitfall below in the "reliability" and "runaway loop"
sections is drawn from reading this actual implementation, not a hypothetical one, and
several are pre-existing gaps in it that this feature would inherit or amplify if
copied uncritically.

## 1. Security — inbound webhook receiver

### 1.1 Signature verification: use `hmac.Equal`, not `==` or `bytes.Equal`

Confirmed via `grep -rln 'hmac\.'` across the repo: **there is no existing HMAC
verification code anywhere in stapler-squad** — this is greenfield, so there's no
precedent to copy correctly *or* incorrectly, but it's the single most common way this
class of feature ships a real vulnerability. GitHub's `X-Hub-Signature-256` (FR2) and
the generic webhook shared-secret (FR4) both require comparing an attacker-supplied
MAC against a computed one. Comparing with `==`/`bytes.Equal` is a **timing side
channel**: those compare byte-by-byte and return early on first mismatch, so response
latency leaks how many leading bytes matched, letting an attacker recover the correct
signature byte-by-byte over enough requests. Always use `hmac.Equal(a, b []byte) bool`
(constant-time by construction) — never compare the hex/base64-decoded MAC with `==`.
Also compute the HMAC over the **raw request body bytes**, not a re-serialized/
re-marshaled version of the parsed JSON — re-marshaling can silently change field
order/whitespace and break verification for legitimate requests, or (worse) if the
verification path and the parsing path use two different byte representations, an
attacker could in principle craft a payload that hashes differently than it parses.

### 1.2 SSRF risk from webhook-payload-derived outbound requests

FR4/FR10 mean payload fields can end up interpolated into a `prompt_template` that an
agent session then acts on — that's prompt injection (§4), a separate risk. The
*SSRF*-specific case is narrower but still real: if any part of the trigger evaluation
pipeline itself (not the eventual agent session) makes an outbound HTTP call using
attacker-controlled data — e.g. a "verify the referenced GitHub PR/JIRA ticket exists"
enrichment step, or resolving a URL embedded in the payload before templating — that
call must not be allowed to target internal/link-local addresses
(`127.0.0.0/8`, `169.254.169.254` cloud metadata, `10.0.0.0/8`, `192.168.0.0/16`,
`::1`). Nothing in the requirements doc currently calls for this kind of enrichment
fetch, so the safest design is simply **not to add one** — evaluate `event`/
`label_filter` match criteria and render the template purely from the already-received
JSON body, with zero outbound calls in the trigger-evaluation path itself. If a future
enrichment step is added, it needs the same URL-allowlist/DNS-rebinding-resistant
dialer discussed in §5 for outbound callbacks.

### 1.3 Unbounded payload size (DoS)

No existing inbound HTTP handler in `server/` currently caps request body size for a
webhook-shaped POST (this is a new route). Without an explicit limit,
`io.ReadAll(r.Body)` (or ConnectRPC's default if this is wired as a plain
`http.Handler` alongside ConnectRPC per Go's `net/http` mux) will buffer an arbitrarily
large body into memory per request, and concurrent large POSTs are a straightforward
memory-exhaustion DoS — the same class of resource risk the 2026-07-12 OOM incident
already demonstrated this machine is not resilient to (`feedback_backlog_wip_limit`
memory: 57GB/61GB used, swap exhausted, a `go test -race` run OOM-killed). Wrap the
body reader in `http.MaxBytesReader(w, r.Body, maxWebhookBodyBytes)` (GitHub's own push
webhook payloads can exceed 1MB for large diffs; pick a limit generous enough for real
payloads — a few MB — but bounded) before reading, and reject with `413` on overflow.

### 1.4 Replay attacks (no nonce/timestamp check)

HMAC signature verification alone proves the payload came from a holder of the shared
secret **at some point** — it does not prove the request is fresh. An attacker who
captures one valid webhook request (network logging, a compromised intermediate proxy,
a leaked request log) can replay it indefinitely, each replay creating a new session
(FR1) exactly as if the real event recurred. GitHub's webhook payloads include a
`X-GitHub-Delivery` UUID (unique per delivery, suitable for a dedup cache) but *not* a
timestamp header on the `push` event type itself — so timestamp-window rejection isn't
available for FR2 without also tracking delivery IDs. Recommended: maintain a
short-TTL (e.g. 24h) set of recently-seen delivery IDs (GitHub) / a
provider-independent request digest (generic webhook, FR4) and reject exact repeats.
This also directly overlaps with §2.2 (duplicate delivery) — the same dedup mechanism
solves both "GitHub legitimately retries" and "an attacker/proxy replays a captured
request," so build it once.

### 1.5 Secret storage and rotation

`config/` uses JSON persistence (per requirements.md's own FR1 framing) — confirm at
plan time whether that JSON file (or its DB-backed successor, following the same
migration `RulesStore` already went through — see
`project_plans/dynamic-rule-reload/research/pitfalls.md` §1) is otherwise
plaintext-readable on disk. If trigger secrets (GitHub webhook HMAC key, generic
webhook shared secret) are stored inline in that same config surface, anyone with
filesystem read access to the config path (or a `GetConfig`-shaped RPC that doesn't
redact it) gets the secret — which then lets them forge signed requests, defeating §1.1
entirely. At minimum: never return the raw secret value in any RPC response used to
*display* trigger config (only a redacted/masked form, same pattern PR review UIs use
for tokens); prefer generating the secret server-side (not user-typed) so it's high
entropy by construction; and support rotation (a trigger can hold two valid secrets
during a cutover window) since GitHub's own webhook UI requires re-entering the secret
on both ends simultaneously with no overlap — a bad rotation UX otherwise causes a
silent outage where every push webhook 401s until someone notices.

## 2. Reliability

### 2.1 Cron drift and missed fires across restarts

Read directly from `server/workflows/scheduler.go:53-75` (`Start`): on boot, it loads
enabled workflows and calls `s.c.AddFunc(cronExpr, ...)` for each — `robfig/cron`
computes each entry's *next* fire time from wall-clock "now" at registration. **There
is no missed-fire replay**: if the process was down across a scheduled tick (service
restart via `make install-service`, which this repo's own
`.claude/rules/tmux-keep-server-on-restart.md` confirms happens routinely and kills
the tmux server), that tick is silently skipped — the next fire is simply the next
occurrence of the cron expression after restart. This is the *safe* choice with
respect to double-firing (confirms the "does a fired-but-uncommitted cron job replay
on next boot and double-fire?" question in the negative — it does not double-fire), but
it is a silent skip with **zero record that a fire was missed** (no log line, no
event). For FR3's stated use case ("nightly audits"), a restart that straddles the
nightly window means the audit silently doesn't run that day, discoverable only by a
human noticing the absence — this is exactly the kind of silent gap
`feedback_document_ai_decisions_in_edge_cases` says the system should not create.
Recommend: on `Scheduler.Start`, compare each entry's last-known-fired timestamp
(needs a new persisted field — `Workflow`/trigger schema doesn't currently track
`last_fired_at`) against its cron schedule's previous expected occurrence, and log/
surface (not silently replay-fire — replaying is its own risk, see §3) when a window
was missed.

### 2.2 Duplicate delivery from provider retries (GitHub retries on non-2xx)

GitHub retries webhook deliveries that receive a non-2xx response (and can also
manually be redelivered from the UI). If the receiver's handler creates the session
*before* returning 200 — or if session creation succeeds but the response write fails/
times out for an unrelated reason (slow ConnectRPC call inside the handler, network
blip) — GitHub will see the delivery as failed and retry, creating a second session
for the same push. The fix is the delivery-ID dedup cache from §1.4: key it on
`X-GitHub-Delivery` (GitHub) or a payload digest (generic webhook, which has no
provider-assigned delivery ID) and short-circuit a repeat to "already processed,
return 200" without creating a second session — check-and-set this **before** doing
any session-creation work, not after, so a retry that arrives while the first request
is still in flight doesn't race past the check.

### 2.3 Thundering herd — many triggers firing simultaneously

Two concrete versions of this in the current design shape:
- **Cron.** Multiple trigger cron expressions with the same or overlapping schedule
  (e.g. several `"0 9 * * *"` nightly-audit triggers across different repos) fire
  within the same tick. `Scheduler.addCronEntry` (`scheduler.go:197-203`) runs each
  fire in its own goroutine dispatched by the underlying `cron.Cron` — there's no
  shared concurrency gate across entries, only the per-fire
  `5*time.Minute` timeout. N simultaneous fires means N simultaneous `CreateSession`
  calls, each of which spins up a tmux session/git worktree — expensive operations.
- **Webhook burst.** A single GitHub push that touches many branches, or a JIRA bulk
  update, can emit many webhook deliveries in a tight window, each independently
  matching a trigger and creating a session.

Both need to flow through the **same admission gate** described in §3 below
(`MaxConcurrentBacklogWorkItems`) rather than calling `CreateSession` directly and
unconditionally — that gate is a queue/backpressure point as much as a safety cap, and
today's `Scheduler.FireNow` bypasses it entirely (see §3).

## 3. Runaway-loop pitfall — pipeline chaining vs. the existing WIP limit (MUST-avoid)

**This is not hypothetical for this repo — it already happened once.**
`feedback_backlog_wip_limit` memory: on 2026-07-12 this machine hit a kernel OOM
(57GB/61GB used, swap exhausted) driven substantially by too many concurrent
backlog-spawned `claude` agent processes; a `go test -race` run was OOM-killed as a
direct symptom. The user's fix was a hard concurrency cap, which **is already
code-enforced today, but only on one specific path**:

- `server/services/backlog_service.go:283-290` (`maxConcurrentBacklogWorkItems`) reads
  `cfg.MaxConcurrentBacklogWorkItemsOrDefault()` and gates
  auto-spawn-from-backlog-item flows (`server/services/backlog_service.go:121`,
  `server/services/backlog_service_triage.go:331` reference this same cap).
- `server/workflows/scheduler.go:127-188` (`FireNow`) — used by **both** the manual
  `run_workflow` MCP tool and cron's own internal fire path
  (`scheduler.go:126` comment: "Used by RunWorkflow RPC and internal cron trigger") —
  calls `s.sessionSvc.CreateSession(ctx, req)` **directly**, with no reference to
  `MaxConcurrentBacklogWorkItems`, `BacklogService`, or any other admission check.
  Confirmed by reading the full call path: nothing in `Scheduler` holds a reference to
  `BacklogService` or its config gate at all.

That means: **the one existing automatic session-creation path in this codebase today
already bypasses the WIP limit that exists specifically because of the 2026-07-12
incident.** If FR3 (cron triggers) and FR10 (chaining) are implemented by copying
`Scheduler.FireNow`'s pattern — call `CreateSession` straight from the trigger/
completion-hook code — the new feature inherits that gap and makes it worse, because
now the trigger source is external (webhook, chained completion) rather than a human
manually clicking "run workflow," removing the last implicit rate limit (a human has
to be present to click).

The specific chaining loop shape to guard against (FR10: session A completes → creates
session B; requirements.md explicitly flags this as "really FR7 wired back into FR3"):
- **Direct cycle**: B's own completion trigger matches the same pattern that created A,
  re-firing A-shaped work indefinitely. Concretely plausible if a "plan → implement →
  review → merge" chain (the example given in the requirements doc's own "Why This
  Matters" section) has any step whose *completion* criteria overlaps its own *trigger*
  criteria — e.g. if "review" and "plan" ever converge on the same repo/branch
  pattern after a merge, a merge could look like a fresh "needs a plan" trigger match.
- **Amplifying fan-out**: if a single completion is allowed to satisfy multiple chained
  triggers (not just one "next" step), each with its own "next," session count grows
  combinatorially rather than linearly, even without a literal cycle.
- **Failure-loop**: a session that fails/is marked stale and whose *stale* callback
  (FR7's `on_session_stale`) is also wired to a "retry" chain creates unbounded retries
  if the retry keeps landing in the same stale state — this needs the same bounded-
  retry ceiling FR8 already requires for callback *delivery*, applied to chained
  *session creation* too, not just to the HTTP POST layer.

**Concrete mitigation, testable against AC-level requirements**: every session/backlog
item created by a trigger — cron, webhook, or completion-chained — must go through the
*same* admission path as manual backlog-driven auto-spawn (i.e., `BacklogService`'s
`MaxConcurrentBacklogWorkItems` gate), not a direct `CreateSession` call analogous to
`Scheduler.FireNow`. This is also what FR4/AC1's "triggers create sessions/backlog
items through the existing approval/review path... they do not bypass it by default"
already requires — treat that requirement as covering *this* specific bypass, not just
the human-approval angle, and add an explicit chain-depth counter (e.g. a
`triggered_by_chain_depth` field propagated session→session, hard-capped at a small N
like 3-5) as a second, independent backstop that doesn't rely on the WIP gate alone —
a WIP gate limits *concurrency* but a slow drip of one-at-a-time chained sessions could
still loop forever within the concurrency limit if nothing tracks chain depth.

## 4. Template injection / prompt injection via untrusted webhook JSON

FR4/FR1 render `prompt_template` (Go `text/template`) against the inbound JSON payload
(e.g. `{{issue.summary}}`). Two distinct risk classes, often conflated:

- **Renderer crashes/errors (mechanical).** `text/template`'s `Execute` returns most
  malformed-access errors (missing map key, nil field access) as an `error`, not a
  panic — `Execute` has an internal `recover()` that converts most execution-time
  failures raised via the template engine's own error path into a returned `error`.
  Practical risk is lower than a hard crash, but still needs handling: (a) a
  **parse-time** error (malformed template syntax in the trigger's own configured
  `prompt_template` — an *operator* mistake, not attacker-controlled, since the
  template string itself comes from trigger config not the webhook payload) should be
  caught at trigger-save time (FR6/FR7's config validation), not deferred to fire time;
  (b) an **execution-time** error (payload JSON missing a field the template
  references, e.g. `{{.issue.summary}}` when the payload has no `issue.summary`) will
  happen routinely for a webhook receiving multiple event shapes and must fail the
  trigger match cleanly (log + skip, matching FR9's "failed callback... logged/
  surfaced" visibility principle) rather than 500ing the webhook receiver or silently
  creating a session with a partially-rendered/broken prompt.
- **Prompt injection (semantic, the more serious one).** Interpolating attacker-
  controlled webhook field content directly into an agent session's initial prompt is
  the textbook indirect-prompt-injection vector: a GitHub push's commit message, a
  JIRA ticket's description/summary field, or any other webhook-supplied string an
  attacker can influence (anyone who can open a PR against a watched repo, or file a
  ticket in a watched JIRA project) can contain text designed to redirect the agent's
  behavior once it's running with real tool access (bash, git, the same worktree/PR
  push capabilities every other stapler-squad session has). This is **not** mitigated
  by HMAC signature verification (§1.1) — the signature proves *GitHub* sent the
  payload, not that the payload's *content* (a commit message anyone with push access
  wrote) is safe to treat as instructions. The primary mitigation available today is
  the same one requirements.md already calls out as a hard constraint (Goal 4, AC1-8):
  trigger-created sessions must go through the **same approval/review gate** as
  manual sessions, with **no elevated auto-approve permissions** granted just because
  a session originated from a trigger — if anything, trigger-originated sessions are a
  candidate for *stricter* default classifier rules (lower auto-allow priority), not
  equal or looser ones, precisely because their initial prompt contains attacker-
  reachable content that a manually-typed prompt doesn't.

## 5. Outbound callback pitfalls (FR7-FR9)

No existing outbound-webhook/callback dispatcher exists in this codebase today
(confirmed: `grep -rln 'http.Post\|http.Client{'` across `server/`, `session/`, `pkg/`
turns up only inbound-facing API clients — Anthropic/Gemini rate-limit checkers,
GitHub PR plugins — none of which POST to a user-configured arbitrary URL). This is
greenfield, so get it right the first time rather than retrofitting:

- **Blocking the lifecycle transition on a slow/dead URL.** FR8 already states this
  requirement explicitly ("must not stall the session state machine") — the concrete
  failure mode to design against is a callback URL that accepts the TCP connection but
  never responds (a classic slowloris-style hang), not just a DNS/connection-refused
  fast-fail. Use a bounded per-request timeout (`http.Client{Timeout: N}`, a handful of
  seconds) **and** dispatch the POST from a goroutine/queue decoupled from the
  synchronous lifecycle-transition code path — the transition itself (marking a
  session `complete`/`stale` in storage) must commit and return before the callback
  attempt even starts, matching FR8's "best-effort with bounded retry" framing.
- **Logging secrets/URLs.** Callback URLs are user-configured and may embed
  credentials (`https://user:token@host/hook`, or a `?token=...` query param — a
  common pattern for simple receivers like Zapier/n8n/Slack incoming webhooks). Any
  log line, error message, or FR9 "failed delivery" surfacing must redact the URL's
  userinfo/query string before logging or displaying it, the same way the secret-
  redaction concern in §1.5 applies to inbound trigger secrets.
- **SSRF via user-configured callback URLs.** The callback URL is operator-supplied
  config, not attacker-controlled input directly — but in a multi-tenant or
  shared-instance deployment (or simply "a compromised/malicious trigger config"),
  nothing today would stop a callback URL of `http://169.254.169.254/latest/meta-data/`
  (cloud instance metadata) or `http://localhost:<internal-port>/` (any other service
  reachable from the stapler-squad host, including stapler-squad's own unauthenticated
  internal endpoints if any exist) from being configured and then having the server's
  own credentials/network position used to reach it on the operator's behalf every
  time a session completes. Since this repo's stated deployment model is a
  single-operator local/home service (per `CLAUDE.md`'s `localhost:8543` framing) the
  practical risk is lower than a multi-tenant SaaS, but the mitigation is cheap enough
  to include regardless: validate the configured callback URL is `http`/`https`,
  resolve and reject link-local/loopback/private-range targets at send time (not just
  save time — DNS can change between config-save and each future callback fire, a
  classic TOCTOU/DNS-rebinding gap), and document that self-callbacks (pointing back at
  stapler-squad's own port) are explicitly out of scope or handled specially if ever
  needed for chaining (FR10 should use the in-process chaining path in §3, not an
  HTTP round-trip to itself).

## 6. Operational — debugging "why didn't my trigger fire" without a UI/audit trail

This repo already has a working precedent for the *right* answer here, and a
documented incident showing what happens when it's skipped. Per
`feedback_document_ai_decisions_in_edge_cases`, `session/backlog_lifecycle.go`'s
`closeIfSupersededByMain` self-heal path is the reference pattern: it posts a
human-readable comment explaining the automated decision **and** calls the existing
`notify()`/event mechanism, so the reasoning survives in the item's own history rather
than only in ephemeral scrollback. Applied to this feature, every trigger evaluation —
not just successful fires — needs a durable record:

- **Non-matches need a record too, not just fires.** AC3 explicitly requires
  "non-matching events/labels are ignored" — but "ignored" must not mean "left no
  trace." Without at least a debug-level audit log per inbound request (trigger
  evaluated, matched: yes/no, reason if no), the only way to answer "why didn't my
  webhook fire a session" is to reproduce the request and add print statements —
  exactly the kind of silent gap this repo's own conventions call out. A minimal
  per-trigger "last evaluated at / last match at / last skip reason" surfaced via an
  existing status RPC (or piggybacked on FR6's enable/disable UI) closes this cheaply.
- **Signature-rejected and malformed requests (AC8) are a security-relevant signal,
  not just a validation failure** — a burst of 401s on a webhook endpoint is either a
  misconfigured legitimate sender (operator needs to know) or an attacker probing for
  a working secret (operator *really* needs to know, ties back to §1.1's timing-attack
  concern: if rejections aren't logged, no one notices a slow brute-force in progress).
  Log rejections distinctly from successful-but-non-matching requests.
- **Cron's silent-skip gap (§2.1) is the same class of problem** — a scheduled trigger
  that "should have" fired and didn't (process was down, or was misconfigured) needs
  the same kind of visible signal as a rejected webhook, not silent absence.
