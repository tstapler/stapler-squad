# Pitfalls Research: google-jules-integration

Agent 4 (Pitfalls), SDD Phase 2. Scope: known-risk analysis for integrating
Jules' alpha REST API into the backlog/session pipeline, checked against how
this repo already handles analogous risk (credentials, rate limits, dedup,
concurrency).

## 1. Alpha API risk

Common failure modes when a production integration is built against a
vendor's alpha/beta API:

- **Breaking schema/field changes without a deprecation window** — alpha
  APIs version by fiat; a field rename or enum addition can silently change
  behavior (e.g. a new `Session.state` value your switch statement doesn't
  handle falls into a default case).
- **Auth churn** — alpha products commonly rotate from API-key to
  OAuth/service-account auth as they mature; an integration hardcoded to one
  auth shape breaks on migration.
- **Undocumented rate limits/quotas** — alpha APIs frequently don't publish
  limits at all; the first signal is a 429 or, worse, a billing surprise.
- **Endpoint/resource removal** — `Sources`/`Sessions`/`Activities` are named
  as the current surface; an alpha product can rename or merge resources
  between announcement and GA.

Defensive patterns, cross-checked against what this repo already does for
its one other alpha-ish external dependency (GitHub's REST API, which is
stable but still has its own poller pitfalls to model against):

- **Adapter isolation (already the plan)**: the requirements doc already
  calls for a single-package adapter so churn doesn't ripple into `session/`
  core — confirmed as the right shape by precedent: `github/` is exactly
  this kind of isolated adapter package (`github/keychain.go`,
  `github/rate_limit.go`, `github/http_client.go`), and `session/` code
  never touches `net/http` directly for GitHub calls, only the `github`
  package's typed functions. A `jules/` package following the same shape
  (own HTTP client, own error/rate-limit types, no `session/`-internal type
  leaking into it) is the precedent to match, not invent.
- **Contract/golden tests against recorded fixtures** — since the live API
  can drift, tests should assert against recorded response fixtures (JSON
  golden files) rather than only mocking Go structs, so a real schema change
  is caught by re-recording, not silently absorbed by an out-of-date mock.
- **Version pinning where the API allows it** (e.g. an API-version header or
  `?v=` param) — check at implementation time whether Jules' alpha API
  exposes one; if so, pin it explicitly rather than floating.
- **Fail soft, not loud** — an adapter error (5xx, schema mismatch, timeout)
  should degrade the Jules session to a visible "status unknown" state in
  the UI, not crash the backlog poller loop that also serves local-agent
  sessions. `WorktreePRPoller.handleFetchError` (`session/worktree_pr_poller.go:327`)
  is the existing pattern: classify by error string, log distinctly, keep
  polling other items.

## 2. Credential handling risk

Concrete risks for a Jules API key (per requirements: "an API key that
grants an agent write access to a user's GitHub repos"):

- **Leaking in logs** — request/response logging that includes headers, or
  error messages that interpolate the raw key, or debug dumps
  (`httputil.DumpRequest`) enabled in dev and left on.
- **Plaintext-at-rest** — storing the key directly in `config.json` (which
  gets included in support-ticket screenshots, backed up, or synced) rather
  than an OS credential store.
- **Scope creep** — Jules API keys are almost certainly account-wide, not
  per-repo; a key stored once grants write access to every repo the user's
  Jules account can reach, not just the ones stapler-squad manages.
- **No revocation/rotation story** — if the key leaks, does the UI make it
  obvious how to invalidate and re-enter a new one, or does a stale key sit
  in storage indefinitely.

**How this repo already handles GitHub App/OAuth tokens (two coexisting
patterns, not one)** — both are real, working precedent to reuse:

1. **OS keychain, for the primary GitHub credential.**
   `github/keychain.go` wraps `github.com/zalando/go-keyring` (real
   OS-native secret storage — macOS Keychain / Secret Service / Windows
   Credential Manager, not a file). `GetKeychainTokenForHost(host)`
   (`github/keychain.go:131`) is the read path; `session/backlog_plugin_github.go:161`
   shows the call pattern each plugin already uses: prefer the shared
   keychain token over any per-source config token, and treat an absent
   token as "source disabled," not an error
   (`session/backlog_plugin_github.go:149`). **This is the strongest-fit
   precedent for a Jules API key** — same shape (single credential, host- or
   service-scoped, grants write access to repos) and same intended UX
   (settings-configured, absence = feature off, not a hard error).
2. **AES-256-GCM encryption-at-rest, for config-embedded secrets**
   (Slack webhook URL/signing secret, workflow webhook secrets —
   `session/backlog_crypto.go:12` `EncryptToken`/`DecryptToken`, called from
   `server/services/slack_config_service.go:77`,
   `server/services/workflow_service.go:167`). This is the pattern for
   secrets that must live *inside* `config.json` because they're structured
   config, not a single bearer token — **this is the weaker precedent to
   flag, not copy uncritically**: the AES key itself
   (`Config.MachineEncryptionKey`) is generated and persisted in the same
   `config.json` file via `GetOrCreateEncryptionKey` (`config/config.go:1426`).
   That means the "encryption" only raises the bar against things that don't
   read the whole config file (accidental grep, partial log dumps) — it does
   **not** protect the secret from anyone who can read `config.json` itself,
   since the decryption key sits right next to the ciphertext. Fine for a
   webhook signing secret; a Jules key is materially higher-value (repo
   write access across the user's whole account) and should go through the
   **keychain** path (pattern 1), not this one.

**Recommendation for the pitfalls checklist**: store the Jules API key via
`go-keyring` under its own service/account key (mirroring
`GetKeychainTokenForHost`/`SetKeychainTokenForAccount` in
`github/keychain.go`), not via `EncryptToken`/config.json. Audit whatever
HTTP client the `jules/` adapter builds to confirm it never logs the
`Authorization` header — grep found no existing centralized
"redact secrets from logs" middleware for outbound HTTP (the only redaction
infra found, `executor/audit.go:116` `redactArgs`, is for subprocess argv,
not HTTP requests) — this is a **gap**, not a solved problem, and the new
adapter needs its own discipline (log the URL path and status code, never
headers or body verbatim) rather than inheriting a guard that doesn't exist.

## 3. Security/privacy risk specific to Jules

Unlike Claude Code/Aider (local tmux processes, code never leaves the
machine except via `git push`), Jules executes on Google's cloud VM — the
source tree itself is sent to a third party as part of session creation,
independent of whether/when a PR is ever opened.

What a responsible integration should do, grounded in what the requirements
doc already commits to ("opt-in, not silent") and what's structurally
absent today:

- **Explicit per-feature opt-in gate**, not just "key configured = on."
  Requirements' Risk Control section conflates "key configured" with
  consent; recommend the UI surface a one-time confirmation
  ("Jules sessions run on Google's infrastructure — your source code will be
  sent to Jules. Continue?") the first time a user tries to create a Jules
  session, separate from just pasting in an API key (a user might configure
  the key well before they understand the data-residency implication).
- **Per-repo allowlist**, not implicit "any repo stapler-squad manages."
  Nothing in the current `ItemSourcePlugin`/backlog-item model scopes a
  source by "is cloud-egress allowed for this repo" — that's a new field,
  not something to infer from existing GitHub-source config
  (`session/backlog_plugin_github.go:34`'s `PluginConfig` has no such flag
  today).
- **No silent fallback to Jules.** If a Jules session creation is wired into
  any auto-dispatch/routing logic later (explicitly out of scope for MVP per
  requirements, but worth flagging for the plan phase), a bug that routes a
  backlog item to Jules instead of a local agent because of a
  misconfiguration would violate the opt-in principle without the user ever
  choosing it for that specific item.
- **Warn about proprietary/sensitive repos specifically** — a generic
  disclaimer undersells the risk for a user with e.g. work code checked out
  locally; the UI copy should be concrete about "this repo's contents,"
  not an abstract feature description.

## 4. Data consistency pitfalls

The core race: stapler-squad's local worktree state and a Jules session
operating on its own Google-side clone/branch are **two independent sources
of truth on the same backlog item**, with no coordination primitive between
them today.

Concrete failure scenarios:

- **Duplicate PRs for the same backlog item.** If a user (or the pipeline)
  starts both a local Claude Code session and a Jules session against the
  same backlog item, both can independently open a PR. The repo does have a
  **dedup precedent** worth reusing: `ImportGitHubIssue` dedups by
  `external_url` (PR #663, `ccabdf05d`) to prevent re-importing the same
  GitHub issue as a second backlog item. The same idea — dedup/reconcile
  by a stable external identifier before creating a new backlog artifact —
  should apply to "a PR already exists for this item's branch," not just to
  issue import.
- **Branch/worktree collision.** If option (c) (API-driven session backend)
  is chosen and Jules is pointed at a pushed branch that a local worktree
  session is also using, a local `git push` racing Jules' own push to that
  branch is a real conflict, not just a duplicate-PR annoyance — force-push
  semantics on either side could silently discard the other's commits.
- **No single-writer guarantee at the backlog-item level for cross-backend
  work.** The existing state machine (`BacklogStatusInProgress` with
  `ExpectedStatus`-gated transitions, e.g. `session/backlog_lifecycle.go:766`)
  is optimistic-concurrency-safe *within* stapler-squad's own status
  transitions, but it has no visibility into a manually-started local agent
  session outside the backlog pipeline (e.g. a developer working the same
  worktree by hand) — that's a pre-existing gap for local agents too, not
  new to Jules, but Jules makes the blast radius worse because it can open a
  PR without the user watching a terminal.
- **Stale-work detection won't catch a Jules session that's just slow.**
  `session/backlog_lifecycle_stale.go`'s stale-work reconciliation is tuned
  for local tmux processes going quiet; a fire-and-forget Jules poll needs
  its own timeout/staleness model calibrated to Jules' actual task duration
  (which can be much longer than a local agent's), or it will misfire and
  mark healthy Jules work "stuck."

**Recommendation**: before creating a Jules session for a backlog item,
check (a) no existing open PR references that item (reuse the
`external_url`/PR-link dedup pattern) and (b) no other in-progress work
session (local or Jules) already claims that item — surface a warning
rather than silently allowing both.

## 5. Cost/quota pitfalls

Jules is a metered, billed product — this repo has **no existing pattern
for proactively capping spend on a paid external API call**. What exists is
reactive only:

- `session/detection/ratelimit/detector.go` is **not** an outbound
  rate-limiter — it's pattern-matching on *local agent terminal output* to
  detect when Claude Code/Aider/etc. report hitting *their own* provider
  rate limit, so stapler-squad can pause and retry. It has no bearing on
  throttling stapler-squad's own outbound calls to a third-party API.
- `github.DefaultRateLimiter` (`github/rate_limit.go:25`) is the closer
  analog — it parses `X-RateLimit-*` response headers and backs off once
  the *server* says to (`WaitIfLimited`, `github/rate_limit.go:153`). This
  is a solid pattern to copy for handling Jules' 429s gracefully, **but it
  is purely reactive**: nothing stops a client from issuing 500
  session-creation calls before the first 429 ever comes back. For a free
  API (GitHub, rate-limited not billed) reactive-only is fine. For a
  **billed** API, reactive-only means the first sign of a problem is a bill,
  not a blocked request.
- The one **proactive cap** that exists in this codebase is
  `MaxConcurrentBacklogWorkItems` (`config/config.go:381`, hard ceiling of
  10 — `maxConcurrentBacklogWorkItemsHardCeiling`,
  `config/config.go:954`) — a config-driven ceiling on concurrently
  in-progress backlog items, unrelated to Jules but structurally the right
  shape to extend: a similar `MaxConcurrentJulesSessions` (or a shared
  budget across both) would give a hard stop before hitting Google's
  billing, not just before hitting stapler-squad's own compute.

Concrete abuse/bug scenarios to design against:

- A retry loop (e.g. the stale-work reconciler, or a webhook handler firing
  more than once) that keeps calling "create Jules session" for the same
  item without idempotency — needs a dedup/idempotency key (analogous to
  the `external_url` PR-import dedup) so a retried create doesn't spawn a
  second billed session.
- No per-user/per-day session-creation cap distinct from the general
  concurrency cap — concurrency caps in-flight work but not creation *rate*
  (e.g. a bug that creates-then-immediately-completes many sessions in a
  tight loop wouldn't be caught by a concurrency ceiling).
- No cost/usage visibility surfaced in stapler-squad's UI — the user has no
  way to see "N Jules sessions created this week" from inside the tool,
  which is where the observability requirement (log create/poll calls
  distinctly, per the requirements doc) should feed a simple counter, not
  just log lines.

## 6. What should be explicitly designed against (reviewer checklist)

- [ ] Jules API key is stored via OS keychain (`go-keyring`, matching
      `github/keychain.go`'s `GetKeychainTokenForHost`/
      `SetKeychainTokenForAccount` shape), **not** via
      `EncryptToken`/`config.json` (that path's AES key lives in the same
      file as the ciphertext — insufficient for a repo-write-scoped
      credential).
- [ ] No code path logs the Jules `Authorization` header, full request, or
      raw API key — verified by grep/test, not assumption, since no
      centralized outbound-HTTP redaction exists in this repo today.
- [ ] `jules/` adapter is a self-contained package (own HTTP client, own
      error/rate-limit types) with zero `session/`-internal types imported
      into it, mirroring `github/`'s isolation.
- [ ] Adapter has contract/golden-fixture tests, not just mocked Go structs,
      so an alpha API schema change is caught by fixture staleness.
- [ ] A Jules-specific reactive rate-limiter exists (mirroring
      `github.RateLimiter`'s `Update`/`WaitIfLimited`) for handling 429s.
- [ ] A **proactive** cap exists before that: a hard ceiling on concurrent
      and/or per-day Jules session creation (extending
      `MaxConcurrentBacklogWorkItems`'s pattern), independent of whether
      Google's API has started rejecting requests yet.
- [ ] Session/PR creation is idempotent — a retried "create Jules session"
      call for the same backlog item does not spawn a second billed
      session (dedup key, matching the `external_url` PR-import dedup
      precedent from PR #663).
- [ ] Before creating a Jules session for a backlog item, check for an
      existing open PR or in-progress work session on that item (local or
      Jules) and warn rather than silently double-working it.
- [ ] Data-residency opt-in is a distinct, explicit user action (not
      inferred from "API key is configured") — first-use confirmation that
      names the specific repo whose code is about to leave the machine.
- [ ] No auto-routing of backlog items to Jules without a per-item or
      per-repo explicit choice — silent fallback would violate the opt-in
      principle even if the key is configured.
- [ ] Observability: Jules create/poll calls and errors are logged
      distinctly (per requirements' Observability Requirements) and feed at
      minimum a simple usage counter surfaced somewhere in-tool, not only
      log lines nobody reads until there's a bill.
