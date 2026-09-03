# Research: Build vs. Buy — Webhook / Trigger / Callback Components

Agent 6 (Build vs. Buy). Scope: inbound trigger receiver + matcher, cron scheduler,
outbound callback dispatcher with retry, prompt templating.

## Key correction to requirements.md

requirements.md states: *"Confirmed by repo search: no `robfig/cron`/`gocron`
dependency, no webhook receiver route, no outbound callback dispatch anywhere in
`server/`."* This is **only half true**. `go.mod:26` already declares
`github.com/robfig/cron/v3 v3.0.1`, and `server/workflows/scheduler.go` is a
complete, production, in-process cron scheduler already wired into the app for
`create_workflow`/`run_workflow`. It:

- Wraps `cron.Cron` with a 5-field parser (`cron.Minute|Hour|Dom|Month|Dow`),
  matching FR3's "standard 5-field cron expression."
- Supports add/reload/remove of cron entries **without a restart**
  (`Scheduler.Reload`, `Scheduler.Remove`) — this already satisfies FR6/AC7's
  "add/edit/disable without redeploy" requirement for the cron trigger type.
  It does this by holding an `entryMap[workflowID]cron.EntryID` under a mutex
  and calling `s.c.Remove`/`s.c.AddFunc` — reuse this pattern rather than
  building a second reload mechanism, consistent with FR6's explicit
  instruction to check `dynamic-rule-reload`'s precedent.
- Already renders a prompt via `{{input}}`-style string substitution
  (`FireNow`, `scheduler.go:127-153`) — this is a hand-rolled placeholder
  replace, not Go `text/template`, but establishes the "prompt built from
  workflow config + runtime data" pattern this feature needs to extend.
- Fires session creation via `SessionServiceInterface.CreateSession` — exactly
  the same path FR3 asks the cron trigger type to use.
- Already validates cron expressions at config-write time
  (`ValidateCronExpression`, exported specifically so callers avoid importing
  `robfig/cron` directly).

**Implication for planning**: the cron-trigger requirement (FR3, AC2) is not a
new build — it's largely already built. The webhook-triggers plan should adapt
`server/workflows/scheduler.go` (e.g., add a `cron`-type trigger record that
reuses `*Scheduler`, or generalize `Scheduler` to accept a `Fire` closure per
trigger type instead of being workflow-specific) rather than standing up a
second `cron.Cron` instance. Two independent in-process cron schedulers in the
same binary would be redundant and a maintenance footgun.

## 1. Existing OSS Go library or framework

### 1a. Cron scheduling — `robfig/cron/v3`

- **Pros**: Already a direct dependency (`go.mod:26`, `v3.0.1`) and already in
  production use via `server/workflows/scheduler.go`. Pure in-process, zero
  external services (fits single-binary self-hosted architecture). MIT
  license. Mature (10+ years, de facto standard for Go cron — used by
  Kubernetes-adjacent and countless production Go services), low maintenance
  burden, minimal API surface (`cron.New`, `AddFunc`, `Remove`, `Start`,
  `Stop`). Already handles the parser configuration this project wants
  (5-field, no seconds).
- **Cons**: None meaningful for this use case — it does not support
  distributed/HA scheduling (irrelevant for a single-binary self-hosted app
  with one active instance) and has no built-in persistence (persistence is
  already handled by this project's own `ent`-backed `WorkflowRepository`
  pattern, which the new trigger store should mirror).
- **Verdict: Recommended.** Not just "use the library" — **reuse the existing
  `Scheduler` type** in `server/workflows/scheduler.go` rather than
  introducing a second cron engine. If cron-trigger and workflow-cron
  ultimately need different entry lifecycles, generalize `Scheduler` (e.g.
  extract an interface for "what fires on tick") rather than duplicating the
  mutex/entryMap/reload machinery.

### 1b. Webhook signature verification — stdlib `crypto/hmac` + `crypto/sha256`

- **Pros**: This is exactly what GitHub's `X-Hub-Signature-256` scheme is (
  `sha256=` + hex-encoded HMAC-SHA256 of the raw body, keyed by the shared
  webhook secret) and what a generic shared-secret HMAC check needs. Stdlib,
  zero new dependency, no version/CVE surface to track. `hmac.Equal` is
  built-in and does exactly the constant-time comparison this needs — no
  library adds value over 15 lines of stdlib code. Repo search confirms **zero
  existing HMAC usage anywhere in the codebase** (`grep -rl "hmac\."` across
  all `.go` files returns nothing), so there's no existing pattern to diverge
  from either way — this would be the first, and it should set the pattern
  for stdlib-only.
- **Cons**: None — a dedicated library (e.g. `go-playground/webhooks`) would
  add a dependency for something that's a 10-15 line function, and most such
  libraries are themselves thin wrappers over `crypto/hmac` with provider-
  specific payload parsing bolted on that this project doesn't need (it only
  needs GitHub push events + a generic shared-secret scheme).
- **Verdict: Recommended (stdlib, no library).** This is the clearest
  "stdlib first" case in this feature. Implementation sketch:
  ```go
  func VerifyGitHubSignature(secret string, body []byte, sigHeader string) bool {
      const prefix = "sha256="
      if !strings.HasPrefix(sigHeader, prefix) {
          return false
      }
      mac := hmac.New(sha256.New, []byte(secret))
      mac.Write(body)
      expected := hex.EncodeToString(mac.Sum(nil))
      return hmac.Equal([]byte(strings.TrimPrefix(sigHeader, prefix)), []byte(expected))
  }
  ```

### 1c. HTTP retry for outbound callbacks — stdlib loop vs. `hashicorp/go-retryablehttp`

- **Pros of stdlib `net/http` + small backoff loop**: No new dependency.
  `go.mod` confirms `hashicorp/go-retryablehttp` (or any retry-http library)
  is **not currently a dependency** — `grep -rln "retryablehttp"` across the
  repo returns nothing. The repo already has a hand-rolled exponential-backoff
  pattern to mirror (`executor/circuit_breaker.go`'s `consecutiveOpenTrips` /
  `RecoveryTimeout` / `MaxRecoveryTimeout` fields), so a bespoke retry loop for
  outbound callbacks would be consistent with existing house style rather than
  introducing a second backoff idiom. FR8 requires this be strictly
  best-effort and non-blocking of the session lifecycle (fire-and-forget via
  a bounded goroutine pool or queue) — a full HTTP-client-replacement library
  is more machinery than that requires; the callback dispatcher mainly needs
  "try, backoff, give up after N attempts, log the final failure" (FR9), which
  is ~40 lines.
- **Pros of `hashicorp/go-retryablehttp`**: Handles more retry edge cases out
  of the box (respecting `Retry-After`, configurable backoff/jitter
  strategies, distinguishing retryable vs. non-retryable status codes),
  MPL-2.0 licensed, well-maintained, widely used. Would save writing/testing
  the retry loop.
- **Cons of `go-retryablehttp`**: New dependency for a narrow use case;
  pulls in `hashicorp/go-cleanhttp` and a logging-interface shim that need
  adapting to this project's `log` package. Its default retry policy
  (`DefaultRetryPolicy`) retries on most 5xx/connection errors — fine — but
  the library's `*retryablehttp.Client` is designed to be a long-lived,
  general-purpose HTTP client, which is more surface than "POST a JSON
  payload to a user-configured URL with bounded retry."
- **Verdict: Viable either way, lean stdlib.** Given (a) no retry-HTTP
  dependency currently exists in `go.mod`, (b) the repo's own precedent
  (`circuit_breaker.go`) is a hand-rolled backoff, and (c) the requirement is
  narrow (fixed small N retries, exponential backoff, non-blocking, log on
  final failure — no need for `Retry-After` parsing or pluggable policies),
  a ~40-line stdlib implementation is the better fit and keeps the dependency
  surface flat. `go-retryablehttp` is not wrong, just unnecessary here — pull
  it in only if retry semantics grow materially more complex later (jitter
  tuning, per-endpoint policies, etc.), not preemptively.

## 2. SaaS/managed webhook relay or workflow orchestration service

Candidates considered: Hookdeck (webhook relay/queue), Zapier/Make (workflow
orchestration), Temporal Cloud (durable workflow engine), GitHub Actions as
the orchestrator instead of building this in-app.

- **Pros**: Offloads retry/queueing/observability (Hookdeck), or offers a
  mature DAG/step engine with battle-tested durability semantics (Temporal).
  Zero code to write for the receiver/matcher plumbing itself.
- **Cons — decisive for this project**:
  - **Architecture mismatch**: stapler-squad is explicitly a **self-hosted,
    single-binary** app (`CLAUDE.md`: "Web Server `server/` ... Single-binary
    deployment with embedded tmux"). Every SaaS option here is an external,
    internet-reachable service the self-hosted binary would need outbound
    network access to and a paid account for — directly against the
    project's own architectural stance of minimizing external service
    dependencies (see e.g. the `secrets` Ansible role's explicit preference
    for a signature-verified native install over Flatpak/Snap sandboxing, and
    the whole point of this being an on-machine agent-session manager).
  - **The action can't live in the SaaS anyway**: the actual effect of every
    trigger in this feature is "create a tmux-backed agent session on this
    machine" — a local process/filesystem/git-worktree operation
    (`session/`). No SaaS orchestrator can perform that; it would have to
    call back into this same app's webhook API to ask it to do so. That
    reduces "buy an orchestrator" to "buy a relay that forwards to the
    webhook receiver we have to build anyway" — it doesn't remove the inbound
    receiver/matcher/HMAC-check work (FR2, FR4), it just adds a hop and a
    third-party dependency in front of it.
  - **Data residency / secrets**: GitHub webhook payloads (repo names,
    branches, potentially commit messages) and JIRA-issue-shaped webhook
    payloads would transit a third party unnecessarily for what's fundamentally
    a local automation trigger.
  - **Cost**: Hookdeck/Zapier/Temporal Cloud all have paid tiers once volume
    or feature needs (multiple endpoints, retries, DAG steps) go beyond free
    tier — an ongoing cost for a feature whose core mechanism (HTTP receiver
    + cron + HTTP POST) is cheap to self-host.
  - **GitHub Actions as orchestrator**: workable for the `github_push`
    trigger type specifically (a workflow step could `curl` this app's
    webhook endpoint on push) but doesn't help `cron` (this app already needs
    its own scheduler per FR3) or generic `webhook` triggers from non-GitHub
    sources (JIRA, etc.), and still requires the same inbound receiver this
    plan already needs to build.
- **Verdict: Not recommended**, for all four options. This is a case where
  "the SaaS still needs to call the thing we're building" makes buying
  strictly worse than building — it adds cost, a network dependency, and data
  egress without removing any of the required in-app work.

## 3. LLM-generated implementation vs. battle-tested library

- **HMAC signature verification**: **Use `hmac.Equal`, never `==` or a
  manual byte loop.** This is a textbook LLM-generated-code correctness bug —
  a naive `expectedSig == computedSig` string comparison or a hand-rolled
  byte-by-byte loop is timing-unsafe (allows a timing side-channel attack to
  recover the correct signature byte-by-byte). `hmac.Equal` is stdlib,
  constant-time, and exists specifically to prevent this class of bug — there
  is no justification for not using it. This must be called out explicitly in
  the implementation plan/task breakdown and checked in review (per
  `.claude/rules/interface-pollution-checklist.md`'s spirit of catching
  LLM-shaped mistakes before merge — this is the crypto-equivalent smell).
  `bytes.Equal` is *not* timing-safe either (it short-circuits on first
  mismatching byte) — `hmac.Equal` specifically must be used, not `bytes.Equal`.
- **Cron expression parser**: **Do not write a bespoke parser.** Parsing
  5-field cron syntax correctly (ranges, steps, lists, month/day-of-week
  aliases, edge cases like `*/5`, `1-5,10-15`) is a well-known source of
  subtle bugs when hand-rolled, and `robfig/cron/v3` already solves this,
  is already a dependency, and is already used correctly in this exact repo
  (`scheduler.go`'s `cron.NewParser(cron.Minute|cron.Hour|cron.Dom|cron.Month|cron.Dow)`
  and the exported `ValidateCronExpression` helper). Writing a second/custom
  parser here would be pure reckless-custom-code risk with a returns of zero
  — reuse `ValidateCronExpression` and the `cron.Parser` directly.

## 4. Fork or adapt existing code

- **`server/workflows/scheduler.go`** — the primary adapt target, detailed
  above under "Key correction." This is Go-specific, lives in this exact
  repo, and already implements dynamic add/reload/remove of cron entries plus
  session-creation firing. Recommend generalizing or directly reusing this
  rather than writing a new scheduler.
- **`session/domain/backlog.go` / `session/backlog_lifecycle.go`** — worth
  checking during planning (not build-vs-buy, but adjacent) for existing
  lifecycle-event hook points that FR7's `on_session_complete`/`on_session_stale`
  callbacks would need to tap into; out of scope for this document but flagged
  for the architecture/plan phase.
- **`executor/circuit_breaker.go`** — not a trigger/scheduler component, but
  its exponential-backoff field pattern (`RecoveryTimeout`,
  `MaxRecoveryTimeout`, consecutive-failure counter) is a reasonable style
  precedent for the outbound callback dispatcher's retry loop (see 1c).
- **`server/push/notifier.go`** (uses `webpush-go`) — inspected and ruled
  out as a reuse target: it's browser Web Push (VAPID-signed push to
  subscribed browser endpoints), a different protocol/purpose than posting
  JSON to an arbitrary user-configured callback URL. No overlap worth
  adapting.
- **`bootstrap-pyinfra/` or other stapler tooling** — not relevant; those are
  Python-based machine-provisioning tools with no scheduling/webhook
  functionality applicable to this Go server feature. Confirmed by the task's
  own framing and a quick mental check of that directory's purpose (Ansible/
  pyinfra bootstrap, unrelated domain) — no further search needed.

## Summary Table

| Component | Decision | Verdict |
|---|---|---|
| Cron scheduling | Reuse/generalize `server/workflows/scheduler.go` (`robfig/cron/v3`, already a dependency) | Recommended |
| Webhook HMAC verification | stdlib `crypto/hmac` + `crypto/sha256`, must use `hmac.Equal` | Recommended |
| Outbound callback retry | Hand-rolled backoff loop (stdlib `net/http`), style-matched to `executor/circuit_breaker.go` | Viable (lean stdlib over `go-retryablehttp`) |
| Prompt templating | Go `text/template` per FR4 (stdlib) — `scheduler.go`'s current `{{input}}` string-replace is a lighter-weight precedent but FR4 explicitly asks for real `text/template` semantics | Recommended (stdlib) |
| SaaS webhook relay / orchestrator (Hookdeck, Zapier, Temporal Cloud, GitHub Actions) | Reject — architecture mismatch, action can't leave the local machine anyway | Not recommended |
| Bespoke cron parser | Reject — use `robfig/cron/v3`'s parser | Not recommended |
| Naive/timing-unsafe HMAC compare | Reject — use `hmac.Equal` | Not recommended |
