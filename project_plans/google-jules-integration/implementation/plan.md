# Implementation Plan: google-jules-integration

**Feature**: Dispatch a backlog item's already-pushed GitHub branch to Google Jules from stapler-squad, poll the cloud session's status, and let the PR Jules opens converge through the existing backlog PR-review path.
**Date**: 2026-09-01
**Status**: Ready for implementation
**ADRs**: [ADR-001](../decisions/ADR-001-dispatch-and-poll-non-tmux-item-session.md), [ADR-002](../decisions/ADR-002-hand-written-jules-gateway.md), [ADR-003](../decisions/ADR-003-jules-credential-keychain-tokensource.md), [ADR-004](../decisions/ADR-004-defensive-spend-controls.md)

---

## Alternatives Explored (Step 0.5 — creative pass)

Four high-level shapes were considered before committing. Recorded here so a
reviewer can challenge the choice rather than the first idea; the rejections are
also carried into the Pattern Decisions table below.

| # | Approach | Key strength | Key weakness | Verdict |
|---|---|---|---|---|
| **A** | **Jules as a non-tmux `ItemSession` row + a dedicated `JulesSessionPoller`** — dispatch via a new push-shaped `JulesDispatcher`, poll `GetSession`, record the PR through `Storage.SetBacklogItemPRAndTransition` | Reuses the headless-triage precedent verbatim (`headlessTriageUUIDPrefix`, `IsTmuxBackedSessionRole`) so **no ent schema migration** is needed, and the PR then flows through `ReconcilePRPending` with zero new PR code | Adds a second long-lived background poller goroutine whose lifecycle/backoff/error-classification partly duplicates `WorktreePRPoller`'s shape | **CHOSEN** |
| **B** | **Jules as an `ItemSourcePlugin` (pull-only PR import)** — requirements' option (b): never call `Sessions.create`, just ingest whatever PRs Jules' GitHub App opens | Near-zero new backend code — `GitHubPRsPlugin` (`session/backlog_plugin_github_prs.go`) already ingests any PR regardless of author | Fails the primary success metric outright: `ItemSourcePlugin.Fetch` has no create direction, so the user still has to visit jules.google.com to start the work | Rejected |
| **C** | **Jules as another agent "program" behind `SessionCreator`** — make a Jules session look like a Claude Code/Aider session so every existing session surface works unchanged | Maximum UI reuse — session list, `SessionDetailView`, badges all work with no new frontend | `SessionCreator` (`server/services/backlog_service.go:30-36`) returns `*session.Instance`, which unconditionally builds a tmux `ProcessManager`; faking a PTY for a cloud VM is exactly the `session/`-core ripple requirements.md's isolation constraint forbids | Rejected |
| **D** | **Poll on the existing backlog reconciliation tick** instead of a dedicated poller (a variant of A) | No new goroutine lifecycle; inherits existing scheduling for free | Couples Jules' alpha-API latency into the sweep that also drives *local* sessions — one hung Jules call stalls local-session reconciliation, violating pitfalls' "fail soft, don't crash the loop that serves local sessions" | Rejected |

---

## Domain Glossary

*(Ubiquitous language. These exact names must appear in code, tests, and comments. Where "Newtype" is noted, do **not** use a bare `string` — see the `primitive-obsession-checklist` skill.)*

| Term | Definition | Notes |
|------|-----------|-------|
| `JulesAPIKey` | The per-user secret from jules.google.com/settings that authorizes calls to `jules.googleapis.com`. | Newtype over `string`. Implements `fmt.Stringer` returning `"jules-api-key(redacted)"` so it can never be interpolated into a log line by accident. |
| `JulesSourceName` | Jules' resource name for a GitHub repo connected through Jules' own web app, format `sources/github-{owner}-{repo}`. | Newtype. Constructed only by parsing an API response or by `ParseJulesSourceName`; never string-concatenated at call sites. |
| `JulesSessionName` | Jules' resource name for one cloud coding session, format `sessions/{id}`. | Newtype. This is the value stored in `ItemSession.session_uuid` (prefixed — see `julesSessionUUIDPrefix`). |
| `JulesSessionState` | The lifecycle state Jules reports for a session: one of `QUEUED`, `PLANNING`, `AWAITING_PLAN_APPROVAL`, `IN_PROGRESS`, `COMPLETED`, `FAILED`. | Sum type (sealed newtype + exported constants + `IsTerminal()`/`IsKnown()`). An unrecognized wire value parses to `JulesStateUnknown`, never a zero-value that silently reads as `QUEUED`. |
| `GitHubBranchRef` | A branch name that must already exist on the GitHub remote before Jules can start on it. | Newtype. MVP never pushes a branch — it validates one was supplied and refuses otherwise (see ADR-001). |
| `JulesSession` | The decoded `Sessions.get`/`Sessions.create` response: `Name`, `State`, `Title`, `Outputs`, `CreateTime`, `UpdateTime`, `WebURL`. | DTO in `jules/`. Holds no `session/` types. |
| `JulesPullRequestOutput` | The `outputs[].pullRequest` sub-object: `URL`, `Title`, `Description`. | DTO. `URL` is parsed into a PR number by `ParsePRNumberFromURL`. |
| `JulesClient` | The Gateway over Jules' REST API. Exposes exactly `ListSources`, `CreateSession`, `GetSession` — the three endpoints MVP needs. | Lives in `jules/client.go`. Imports nothing from `session/` or `server/`. |
| `JulesTokenSource` | One-method interface `APIKey(ctx) (JulesAPIKey, error)` that `JulesClient` depends on for its credential. | The seam that keeps `jules/` free of a `server/services` import (ADR-003). |
| `KeyringTokenSource` | The default `JulesTokenSource`: reads the key from the OS keychain under service `stapler-squad-jules`. | `jules/keychain.go`. Reuses `session/sshremote/keystore.go`'s 5s timeout race for the underlying keyring call, but **not** its always-serialize-behind-one-mutex shape: adds a short-TTL cache (default 5m) and a single-in-flight-probe circuit breaker so a genuinely wedged Secret Service call can pile up at most one blocked goroutine for the life of the process, not one per HTTP request / poll tick (pre-mortem P1 #4; see Story 1.2.1). |
| `ErrJulesKeychainPaused` | Sentinel: the keychain circuit is open (a prior read timed out and the cooldown hasn't elapsed). Wraps `ErrJulesNotConfigured` so existing "feature off" handling applies unchanged. | `jules/keychain.go`. Logged once when the circuit opens, not once per call. |
| `JulesCredentialSource` | A `server/services.CredentialSource` for provider `"jules"` that delegates to `KeyringTokenSource`, registered into `CredentialChain`. | Keeps credential resolution uniform with Gemini/Anthropic (`server/services/credentials.go`). |
| `JulesSourceRegistry` | In-memory `owner/repo → JulesSourceName` cache, refreshed from `ListSources` on miss and on a TTL. | Registry pattern; `sync.Map` + TTL, no ent table. |
| `SessionRoleJulesWork` | New `session` package constant, value `"jules_work"` — the `ItemSession.session_role` for a Jules-backed unit of work. | Added to `session/backlog.go` beside `SessionRoleWork`/`Triage`/`Review`. **Excluded** from `IsTmuxBackedSessionRole`. |
| `session.HasActiveJulesSession` | New exported `session`-package predicate: does this item have an open (not yet ended) `jules_work` `ItemSession`? | `session/backlog.go` (Story 2.1.3, pre-mortem P1 #1). Folded directly into `hasActiveSession` (`session/backlog_lifecycle.go`); called explicitly alongside `findActiveWorkSession`/`hasActiveWorkSession` at the `server/services` spawn/respawn gates that decide whether to start a **new local** session, so an open Jules session blocks a competing local agent the same way an open `work` session already does. |
| `julesSessionUUIDPrefix` | `"jules-"` — prefixed onto a `JulesSessionName` to form `ItemSession.session_uuid`. | **Declared independently in both `server/services/jules_dispatch_service.go` (write side, Task 2.2.1a) and `session/jules_session_poller.go` (read side, Task 2.3.1a)** — `session/` cannot import `server/services` (import-direction constraint), so this duplicates the codebase's existing precedent: `headlessTriageUUIDPrefix` (`server/services/backlog_service_triage.go:423`) is re-declared as `headlessTriageSessionUUIDPrefix` for the read side at `session/backlog_lifecycle_triage.go:43-50`, with a comment there explaining the cycle. Do the same here rather than importing across the boundary. |
| `julesPendingUUIDPrefix` | `"jules-pending-"` — the reservation UUID written *before* the `CreateSession` POST, replaced with the real name after. | The idempotency reservation (ADR-004). **Also declared independently in both packages** — same duplication, and same reason, as `julesSessionUUIDPrefix` above. |
| `JulesDispatchRequest` | The dispatch input: `ItemID`, `Branch GitHubBranchRef`, `Prompt`. | Value object; validated once at construction (`NewJulesDispatchRequest`) — parse, don't validate. **Deliberately carries no `EgressAcknowledged` field** (pre-mortem P1 #3): consent is a durable, server-side fact (`JulesConfig.EgressAcknowledgedRepos`) that only `ConfirmEgressConsent` can write, never a caller-supplied boolean `DispatchToJules` could trust. |
| `JulesDispatcher` | Consumer-defined interface with one method `DispatchToJules(ctx, *session.BacklogItemData, JulesDispatchRequest) (JulesSessionName, error)`. | Deliberately **not** a widening of `SessionCreator` (ADR-001). `DispatchToJules`'s egress check (`checkEgressConsent`) only ever **reads** `EgressAcknowledgedRepos` — it has no code path that appends to it. |
| `ConfirmEgressConsent` | RPC + `JulesConfigService` method that appends a repo to `JulesConfig.EgressAcknowledgedRepos` and persists it. The **only** write path for that slice. | `server/services/jules_config_service.go` (Story 2.4.2). Called exclusively from `JulesDispatchDialog`'s checkbox-confirm handler — never from `DispatchToJules`. |
| `julesSessionCreator` | Narrow, **locally-declared** interface `{ CreateSession(ctx, jules.CreateSessionRequest) (jules.JulesSession, error) }` that `JulesDispatchService` depends on instead of the concrete `*jules.Client`. | `server/services/jules_dispatch_service.go`, unexported. Satisfied structurally by `*jules.Client`; a test fake implements it directly, which is what lets Task 2.2.1c inject a fake. Mirrors `JulesSessionPoller`'s `julesStatusClient` (Task 2.3.1a) — Dependency Inversion: the consumer, not the gateway, owns the interface it depends on. |
| `julesTransitionGuard` | Narrow, **locally-declared** interface `{ transitionWithGuard(ctx, item, to, precondition, triggeredBy string, hasUnresolvedBlockers bool) (*session.BacklogItemData, error); hasUnresolvedBlockers(ctx, itemID string) (bool, error) }` that `JulesDispatchService` depends on for the guarded status transition, instead of holding `*BacklogService` or duplicating its `storage`/`engine` fields (Task 2.2.1a, closing the gap flagged in Phase 4 triad-repair iteration 2). | `server/services/jules_dispatch_service.go`, unexported; satisfied structurally by `*BacklogService` (same package, so the unexported method names are accessible). Wired at construction in `server/dependencies.go` (Task 2.4.4a) by passing the already-built `backlogSvc`. Mirrors `AutonomousStuckRespawner` (`server/services/autonomous_orchestration_service.go:30`) — the codebase's existing convention for one `server/services` type to reach `BacklogService` behavior: a narrow consumer-owned interface, never a concrete sibling-service pointer. |
| `JulesDispatchService` | The `JulesDispatcher` implementation: guards (in-flight mutex → persisted-open-session check → egress → spend caps → blocker check) → reserve `ItemSession` → `CreateSession` → confirm → `transitionWithGuard` to `in_progress`. | `server/services/jules_dispatch_service.go`. Holds a `julesSessionCreator` and a `julesTransitionGuard`, not a concrete `*jules.Client` or `*BacklogService`. |
| `JulesSessionPoller` | Background loop that polls every open `jules_work` `ItemSession` and applies its `JulesSessionState`. | `session/jules_session_poller.go`, config shape mirrors `WorktreePRPollerConfig` (`session/worktree_pr_poller.go:37`). |
| `JulesSessionPollerConfig` | `PollInterval`, `CallTimeout`, `MaxSessionAge`, `NoChangeBackoff`. | Defaults: 60s / 20s / 24h / 5m. |
| `applyJulesState` | The single exhaustive mapping from `JulesSessionState` to a backlog effect (touch progress / record PR / fail). | The one place a new Jules enum value must be handled; guarded by an exhaustiveness test. |
| `JulesAuthReconnectRequired` | Process-level flag on `JulesSessionPoller` (`atomic.Bool`, exported via `AuthReconnectRequired() bool`), set when any poll tick's `GetSession` call returns `errors.Is(err, jules.ErrJulesNotConfigured)` — a 401/403, i.e. the key was resolvable but Jules rejected it, distinct from no-key-configured at startup — and cleared automatically the next time any `GetSession` call succeeds. | Global, not per-session (Story 2.3.4): a revoked/expired key invalidates the whole account, so every open session is affected identically at once. Surfaced to the frontend via `GetJulesConfig`'s `auth_reconnect_required` field (Story 2.4.1) and combined client-side with each item's own `jules_work` session state to compute `JulesSessionPhase == "reconnect-required"` (ux.md §4). |
| `JulesEgressAcknowledgement` | The user's explicit, per-repo, recorded consent that this repo's source may leave the machine for Google's cloud VM. | Stored as `config.JulesConfig.EgressAcknowledgedRepos []string`. Distinct from "an API key is configured" (pitfalls §3). Minted **only** by `ConfirmEgressConsent` (Story 2.4.2), which only `JulesDispatchDialog`'s checkbox-confirm handler calls — see `ConfirmEgressConsent` glossary entry above. |
| `JulesConfig` | The `config.Config` sub-struct: `Enabled`, `EgressAcknowledgedRepos`, `MaxConcurrentJulesSessions`, `MaxJulesSessionsPerDay`. | The API key is **not** in here — it lives in the keychain (ADR-003). |
| `MaxConcurrentJulesSessions` | Hard ceiling on simultaneously-open `jules_work` `ItemSession`s. Default 2, clamped to `maxConcurrentJulesSessionsHardCeiling` = 10. | Extends `MaxConcurrentBacklogWorkItems`'s shape (`config/config.go:381-384`, ceiling at `:954`). |
| `MaxJulesSessionsPerDay` | Hard ceiling on `jules_work` `ItemSession` rows *created* in the trailing 24h. Default 15. | Caps creation *rate*, which a concurrency ceiling does not (pitfalls §5). |
| `ErrJulesNotConfigured` | Sentinel: no API key resolvable. Treated as "feature off", logged once, never a repeating hard error. | Mirrors `session/backlog_plugin_github.go:149`'s "absent token = source disabled". |
| `ErrJulesRateLimited` | Sentinel: a `429` from Jules. Carries the parsed `Retry-After` when present. | |
| `ErrJulesSourceNotRegistered` | Sentinel: the item's `owner/repo` is not among `ListSources` — the user must connect it at jules.google.com first. | User-facing copy names this exact prerequisite. |
| `ErrJulesDispatchInFlight` | Sentinel: this item already has an open `jules_work` `ItemSession`. | The pre-call duplicate guard. |
| `julesRateLimitTransport` | `http.RoundTripper` decorator that records 429/`Retry-After` and exposes `IsLimited()` so a poll tick is skipped rather than guaranteed to fail. | Mirrors `github/http_client.go`'s `rateLimitTransport` and `github/rate_limit.go`'s `DefaultRateLimiter`. |
| `JulesUsageCounter` | Process-level counters for `jules.session.dispatched` / `.polled` / `.failed`, surfaced in the settings panel. | Satisfies pitfalls §5's "not only log lines nobody reads until there's a bill". |
| `JulesStatusBadge` | React component rendering `JulesSessionState` as icon + text label + color (never color-alone), with the polite/assertive live-region split. | `web-app/src/components/backlog/JulesStatusBadge.tsx`, modeled on `RemoteConnectionIndicator.tsx`. |
| `JulesDispatchDialog` | The modal that collects branch + prompt and carries the first-use egress confirmation naming the specific repo. | `web-app/src/components/backlog/JulesDispatchDialog.tsx`. |
| `JulesSessionPhase` | Frontend phase union `"queued" \| "running" \| "needs-review" \| "done" \| "failed" \| "stale" \| "reconnect-required"`. | Copied in shape from `SessionSummaryPanel.tsx:32`'s `Phase`; `"stale"` preserves the poller-hiccup-vs-task-failed distinction (ux.md §4.3). |

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| `jules/` REST adapter | **Gateway** | PoEAA | Generated client (OpenAPI codegen) or `github.com/yuyu1815/jules-api` | Google publishes no OpenAPI spec for Jules and this repo has no codegen precedent for third-party REST (`build-vs-buy.md` §3); the community module has 1 star, one maintainer, ~11mo stale, and previously mislabeled itself as official — not trustworthy with a repo-write-scoped key. |
| Overall integration shape | **Dispatch-and-poll over a non-tmux `ItemSession`** (approach A) | — | Approach C: widen `SessionCreator` with `CreateJulesSession` | `SessionCreator` returns `*session.Instance`, which always constructs a tmux `ProcessManager`; a Jules method on it is textbook interface pollution *and* forces `session/` core changes the isolation constraint forbids. |
| Dispatch entry point | **Service Layer** + narrow consumer-defined interface (`JulesDispatcher`) | PoEAA + `interface-pollution-checklist` | Adding the method to `ItemSourcePlugin` | `ItemSourcePlugin.Fetch` is a pull contract ("what's new since this cursor"); dispatch is a push with no external item to map and no cursor to advance. |
| Status ingestion | **Dedicated poller goroutine** | — | Approach D: piggyback on the backlog reconciliation tick | An alpha API's hung call would stall the sweep that also drives local sessions. |
| `JulesSessionState` handling | **State as a sum type** + one exhaustive `applyJulesState` mapper | Type-driven design | `switch` on a raw `string` with a silent `default` | A new alpha-API enum value must fail loudly in one known place, not fall through into "still running" forever. |
| `JulesAPIKey`, `JulesSourceName`, `JulesSessionName`, `GitHubBranchRef` | **Value objects (newtypes)** | Type-driven design / `primitive-obsession-checklist` | Bare `string` parameters | `DispatchToJules(ctx, item, "main", "sources/github-a-b", key)` is four same-typed strings — the exact call-site transposition hazard the repo's own checklist targets. `JulesAPIKey.String()` also makes accidental log interpolation impossible. |
| Rate-limit handling | **Decorator** (`http.RoundTripper`) | GoF | Per-call inline 429 checks | Mirrors the already-proven `github/http_client.go` `rateLimitTransport`; keeps the check in one place so no new endpoint can forget it. |
| Credential resolution | **Chain of Responsibility** (`CredentialChain` + `JulesCredentialSource` → keychain) | GoF, existing `server/services/credentials.go:100` | `EncryptToken`/`config.json` (`session/backlog_crypto.go`) | That path's AES key is persisted in the same `config.json` as the ciphertext (`config/config.go:1426`), so it only defends against partial reads — insufficient for a key granting write access to every repo in the user's Jules account. |
| `jules` ↔ `server/services` credential boundary | **Dependency inversion via `JulesTokenSource`** | Clean Architecture | `JulesClient` holding a `*CredentialChain` directly | `server/services` imports `jules`; the reverse would be an import cycle, and would drag `server/` types into the isolation package pitfalls §1 requires to stay self-contained. |
| Spend guards | **Transaction Script** guard sequence at the dispatch entry | PoEAA | A `JulesQuotaPolicy` Domain Model object | Two counting rules and one boolean; a policy object would be ceremony with no invariants to protect. |
| Duplicate-dispatch prevention | **Reservation row** (write `julesPendingUUIDPrefix` `ItemSession` *before* the POST, then `UpdateItemSessionSessionUUID`), gated behind an explicit **persisted-state check** (query for an existing open `jules_work` `ItemSession` on the item) run in addition to, not instead of, the in-process mutex | — | Post-hoc dedup on the returned `JulesSessionName`; relying on the in-process mutex alone | A billed session created between the POST and a crashed DB write would be invisible and unreconcilable; the reservation makes the local row the source of truth for "a dispatch is in flight". The mutex alone only catches two calls racing at the same instant in the same process — it is released once the first call finishes, so a genuinely *sequential* second dispatch (a different code path, or a call after a process restart) sails past it; the persisted-state check is what actually enforces "at most one open Jules session per item" (Task 2.2.1b). |
| Double-click dispatch guard (in-process, ahead of the reservation row) | **Per-item mutex** (`sync.Map` of `*sync.Mutex`, non-blocking `TryLock`) | — | `singleflight.Group` | `singleflight` coalesces concurrent calls and returns the *first* caller's result to every waiter; Story 2.2.1's AC requires the second concurrent caller to receive `ErrJulesDispatchInFlight`, not a fabricated success. |
| `owner/repo → JulesSourceName` lookup | **Registry** (in-memory, TTL-refreshed) | PoEAA | A new ent table | Sources are read-only and few; a schema migration for a cacheable lookup is unwarranted at single-user scale. |
| Frontend status updates | **Observer** via existing ConnectRPC query invalidation on backlog item refresh | GoF | A component-local `setInterval` polling loop | `RemoteConnectionIndicator.tsx` already establishes "Redux/store-driven, no fetch of its own inside the component" as the house pattern. |
| Jules session UI surface | **Reuse `SessionsSection` + a `JulesStatusBadge`** on `BacklogItemDetail` | — | A Jules "Activity" tab inside `SessionDetailView.tsx` (ux.md §2's recommendation) | `SessionDetailView` renders a `Session`/`Instance`; a Jules session has no `Session` ent row at all (ADR-001), so that view is never reachable for one. The item-detail page is where headless (triage) sessions already surface — see "Deviation from ux.md" below. |

### Deviation from `research/ux.md` §2 (recorded deliberately)

ux.md recommends replacing the Terminal tab with an Activity timeline in
`SessionDetailView.tsx:579-589`. Verified during planning: that component is
driven by a `Session` object, and a Jules-backed unit of work never creates a
`Session` ent row (it is an `ItemSession` only — the headless-triage pattern,
`server/services/backlog_service_triage.go:423`). So `SessionDetailView` is
unreachable for a Jules session and editing its `tabs` array would be dead code.

What is kept from ux.md, unchanged: the badge design rules (icon + label +
color, never color alone; `role="status"`/`aria-live="polite"` for routine
transitions and `role="alert"` only for failure), the `Phase`-shaped
stale-vs-failed distinction, and reusing `GitHubBadge` untouched for the PR. The
"discrete timeline, not a fake PTY" idea is honoured by rendering Jules state
transitions into the existing append-only progress-note history
(`Storage.AppendProgressNote`, `session/storage.go:1235`), which
`ProgressHistorySection.tsx` already renders as a timeline. A richer feed from
the `Activities` API stays out of scope, as requirements.md already cut it.

---

## Migration Plan

**No ent schema migration and no data backfill.**

- `ItemSession` gains no columns. `session_role` is a plain string column;
  `SessionRoleJulesWork = "jules_work"` is a new *value*, not a new field —
  exactly how `"triage"` already coexists with `"work"`/`"review"`.
- `session_uuid` is documented as a "loose FK to Session; not an ent edge"
  (`session/ent/schema/item_session.go:23-24`), so storing `"jules-sessions/{id}"`
  there needs no relation change.
- `config.Config` gains a `JulesConfig` sub-struct with `omitempty` JSON tags;
  absent config decodes to the zero value, which means "Jules disabled". Existing
  `config.json` files load unchanged.
- **Forward-compat check to run once during Story 2.1.1**: every existing caller
  of `IsTmuxBackedSessionRole` must be re-read to confirm "not tmux-backed"
  really means "skip the kill", not "assume triage". Known callers:
  `reconcileTerminalItemSessions` (`session/backlog_lifecycle_archive.go:92`) and
  `archiveItemWorkSessions` (`server/services/backlog_service.go:1018`).
  **Note**: `IsTmuxBackedSessionRole`'s own doc comment
  (`session/backlog.go:68`) currently cites the wrong file for the first caller
  (`session/backlog_lifecycle.go`); correct it to
  `session/backlog_lifecycle_archive.go` while editing that comment in Task 2.1.1a.
  **This check is scoped to `IsTmuxBackedSessionRole` only** — the separate,
  broader "does this item have an active session" predicate family
  (`hasActiveSession`, `hasActiveWorkSession`/`findActiveWorkSession`) is
  audited and updated in its own story, Story 2.1.3, per pre-mortem P1 #1.
- **Rollback**: deleting `JulesConfig` from `config.json` and clearing the
  keychain entry disables the feature. Any `jules_work` rows left behind are
  inert — no sweep tries to kill a tmux pane for them, and `ReconcilePRPending`
  keeps working on any item whose PR was already recorded.

---

## Observability Plan

- **Logs** (JSON-lines via `slog`, per `docs/how-to/debug-with-logs.md`), all under a `jules` logger:
  - `jules dispatch requested` — `item_id`, `repo`, `branch`, `source_name`; at `Info`.
  - `jules session created` — `item_id`, `jules_session` (`sessions/{id}`), `item_session_id`; at `Info`.
  - `jules dispatch rejected` — `item_id`, `reason` (one of `not_configured`/`no_egress_ack`/`source_not_registered`/`in_flight`/`concurrency_cap`/`daily_cap`/`no_branch`/`unresolved_blockers`); at `Info`, not `Error` — these are expected user-facing outcomes. `in_flight` covers both the in-process mutex rejection and the persisted-open-session rejection (Task 2.2.1b) — they share one reason value since both mean the same thing to the user ("a dispatch is already in flight for this item"); the log line's caller (mutex vs. persisted check) is distinguishable structurally in code, not by a second reason value.
  - `jules poll tick` — `open_sessions`, `polled`, `skipped_rate_limited`; at `Debug`.
  - `jules session state changed` — `jules_session`, `from`, `to`; at `Info`.
  - `jules poll failed` — `jules_session`, `status_code`, `error`; at `Warn`, and **never** the request headers, body, or `JulesAPIKey`.
  - `jules unknown session state` — `jules_session`, `raw_state`; at `Error` — the alpha-API-drift alarm.
- **Metrics** (counters exposed by `JulesUsageCounter`, read by `GetJulesConfig` for the settings panel): `jules.session.dispatched`, `jules.session.completed`, `jules.session.failed`, `jules.api.rate_limited`, `jules.api.error`. Every Jules API call exceeds 100ms, so `jules.api.latency_ms` is recorded as a histogram on the existing OTel path when tracing is enabled (`otelhttp.NewTransport` wrapping the client transport — no new dependency, `research/stack.md` §4).
- **Alerts**: no new alerts required. Per requirements.md's Observability
  Requirements this is a best-effort, opt-in, single-user feature with no oncall.

---

## Risk Control

- **Feature flag**: `config.JulesConfig.Enabled` (default `false`) **and** a
  resolvable `JulesAPIKey` **and** the item's repo present in
  `EgressAcknowledgedRepos`. All three must hold; any one absent means the
  dispatch action is not offered and the RPC returns `FailedPrecondition`. This
  is deliberately stricter than requirements.md's "key configured = on", per
  pitfalls §3 — a user may paste a key long before understanding that source
  leaves the machine.
- **Rollback procedure**: (1) set `jules.enabled=false` in the settings panel —
  the poller stops on its next tick and no new dispatches are accepted;
  (2) if a full revert is needed, standard PR revert — no schema or data
  migration to undo (see Migration Plan); (3) any in-flight Jules sessions
  continue on Google's side and can be finished at jules.google.com, and any
  already-recorded PR still completes through `ReconcilePRPending` untouched.
- **Staged rollout**: full rollout on merge, gated by the opt-in above. There is
  no cohort mechanism in a single-user/small-team tool, and the opt-in already
  makes merge a no-op for anyone who does not configure a key.
- **Blast-radius guard**: `MaxConcurrentJulesSessions` (default 2) and
  `MaxJulesSessionsPerDay` (default 15) are enforced *before* any billed API
  call, so a retry-loop bug costs at most one day's cap rather than an
  unbounded bill (ADR-004).

---

## Unresolved Questions

- [ ] **Jules' real rate-limit thresholds, `429` headers, and per-plan quotas
      are not published.** Carried forward unresolved from Phase 2 and
      deliberately *not* blocking: the plan is defensive by construction
      (conservative 60s poll, reactive `julesRateLimitTransport`, proactive
      concurrency + daily caps with config overrides). **Does not block any
      story** — but the defaults chosen in Story 2.2.2 should be revisited once
      real usage data exists. Owner: whoever runs the first real dispatch;
      record findings back into `research/stack.md` §Rate limits.
- [ ] **Does `Session.outputs[]` ever contain more than one pull request, and
      can it appear before `state == COMPLETED`?** — blocks Story 2.3.2 (the
      mapper must know whether to take `outputs[0]` or search). Owner:
      implementer of Story 1.1.4, by recording a real `Sessions.get` fixture from
      one live end-to-end dispatch. Interim behavior if still unknown: take the
      **first** output carrying a non-empty `pullRequest.url`, and log
      `jules multiple pr outputs` at `Warn` if `len(outputs) > 1` so the
      assumption is observable rather than silent.
- [ ] **Whether Jules exposes any per-session cost/usage figure** — blocks
      nothing in MVP (`ItemSession.estimated_cost_usd` simply stays `0` for
      `jules_work` rows), but features.md §4 flags that without *something*
      recorded, a later "was Jules worth it vs. a local agent" comparison needs
      a backfill that will be impossible. Owner: implementer of Story 1.1.4 —
      note in the recorded fixture whether any cost field is present.

---

## Dependency Visualization

```
                      ┌────────────────────────────┐
                      │ 1.1 jules/ Gateway + types │
                      │  (client, errors, states)  │
                      └───────┬───────────┬────────┘
                              │           │
              ┌───────────────┘           └───────────────┐
              ▼                                           ▼
   ┌──────────────────────┐                    ┌─────────────────────┐
   │ 1.2 Credential       │                    │ 1.3 Source registry │
   │ (keychain + chain)   │                    │ (owner/repo cache)  │
   └──────────┬───────────┘                    └──────────┬──────────┘
              │                                            │
              └──────────────────┬─────────────────────────┘
                                 ▼
                   ┌───────────────────────────┐
                   │ 2.1 SessionRoleJulesWork  │
                   │   + storage primitives    │   (independent of 1.x —
                   └───────┬───────────┬───────┘    can start in parallel)
                           │           │
              ┌────────────┘           └────────────┐
              ▼                                     ▼
   ┌────────────────────┐                ┌────────────────────────┐
   │ 2.2 Dispatch svc   │                │ 2.3 JulesSessionPoller │
   │  + spend guards    │                │  + applyJulesState     │
   │  + egress consent  │                │  + stale reconcile     │
   └─────────┬──────────┘                └───────────┬────────────┘
             │                                       │
             └──────────────────┬────────────────────┘
                                ▼
                  ┌──────────────────────────────┐
                  │ 2.4 Proto/RPC + config +     │
                  │     server wiring            │
                  └───────┬──────────────┬───────┘
                          │              │
        ┌─────────────────┘              └──────────────┐
        ▼                                                ▼
┌───────────────┐   ┌─────────────────────┐   ┌────────────────────┐
│ 3.1 Settings  │──▶│ 3.2 Dispatch dialog │──▶│ 3.3 Status badge / │
│    panel      │   │   + egress consent  │   │   session row      │
└───────────────┘   └─────────────────────┘   └─────────┬──────────┘
                                                        ▼
                                        ┌───────────────────────────┐
                                        │ 4.1 Observability, docs,  │
                                        │     registry, E2E         │
                                        └───────────────────────────┘
```

Critical path: **1.1 → 2.2/2.3 → 2.4 → 3.2 → 4.1**. Epics 1.2, 1.3, and 2.1 are
parallelizable against 1.1 once 1.1's type definitions land (Story 1.1.1).

---

## Phase 1: The `jules/` adapter package

### Epic 1.1: Typed Jules API gateway

**Goal**: A self-contained `jules` package that speaks the three endpoints MVP
needs, with no `session/` or `server/` import, so alpha-API churn is confined to
one directory (requirements.md Constraints; pitfalls §1).

#### Story 1.1.1: Domain value types for the Jules API

**As a** developer integrating an alpha API, **I want** Jules' identifiers and
states expressed as newtypes and a sum type, **so that** a wire-format change or
a transposed call-site argument fails at compile time or at one known parse
point, not silently at runtime.

**Acceptance Criteria**:
- `JulesSourceName`, `JulesSessionName`, `GitHubBranchRef`, and `JulesAPIKey` are distinct named types over `string`, each with a `Parse*` constructor that rejects malformed input.
  - *Given* the raw string `"github-tstapler-stapler-squad"` (missing the `sources/` prefix), *When* `jules.ParseJulesSourceName("github-tstapler-stapler-squad")` is called, *Then* it returns a zero `JulesSourceName` and a non-nil error whose message contains `sources/`.
- `JulesAPIKey` cannot be printed in full.
  - *Given* a `JulesAPIKey` parsed from `"AIzaSyD-EXAMPLE-KEY-VALUE"`, *When* it is formatted with `fmt.Sprintf("%v %s", k, k)`, *Then* the result is exactly `"jules-api-key(redacted) jules-api-key(redacted)"` and contains no substring of the original key.
- `JulesSessionState` is a sum type where an unrecognized wire value is distinguishable from every known one.
  - *Given* the JSON fragment `{"state":"AWAITING_HUMAN_TEA_BREAK"}`, *When* it is decoded into a `JulesSession`, *Then* `session.State == jules.JulesStateUnknown`, `session.State.IsKnown() == false`, and `session.State.Raw() == "AWAITING_HUMAN_TEA_BREAK"`.
- `IsTerminal()` is true for exactly `COMPLETED` and `FAILED`.
  - *Given* each of the six known `JulesSessionState` constants plus `JulesStateUnknown`, *When* `IsTerminal()` is called on each, *Then* it returns `true` only for `JulesStateCompleted` and `JulesStateFailed`.

**Files**: `jules/types.go`, `jules/types_test.go`

##### Task 1.1.1a: Define the identifier newtypes (~4 min)
- Create `jules/types.go` with `package jules`.
- Define `JulesSourceName`, `JulesSessionName`, `GitHubBranchRef`, `JulesAPIKey` as `type X string`, each with a doc comment naming the wire format.
- Add `ParseJulesSourceName`, `ParseJulesSessionName`, `ParseGitHubBranchRef`, `ParseJulesAPIKey` returning `(T, error)`; reject empty strings and, for the two resource names, a missing `sources/` / `sessions/` prefix.
- Give `JulesAPIKey` a `String() string` returning `"jules-api-key(redacted)"` and a separate unexported `reveal() string` used only by the request builder.
- Files: `jules/types.go`

##### Task 1.1.1b: Define `JulesSessionState` as a sum type (~4 min)
- In `jules/types.go`, add `type JulesSessionState struct { raw string }` with exported package-level constants `JulesStateQueued`, `JulesStatePlanning`, `JulesStateAwaitingPlanApproval`, `JulesStateInProgress`, `JulesStateCompleted`, `JulesStateFailed`, `JulesStateUnknown`.
- Add `ParseJulesSessionState(string) JulesSessionState` (never errors — unknown maps to `JulesStateUnknown` while preserving `raw`), plus `Raw()`, `IsKnown()`, `IsTerminal()`, `String()`, and `UnmarshalJSON`.
- Files: `jules/types.go`

##### Task 1.1.1c: Table-driven tests for the value types (~5 min)
- Create `jules/types_test.go` with table tests covering: every `Parse*` happy and malformed case from the acceptance criteria, the redaction assertion (assert the original key substring is absent from the formatted output), the unknown-state decode, and the `IsTerminal()` truth table.
- Files: `jules/types_test.go`

---

#### Story 1.1.2: `JulesClient` gateway over the three MVP endpoints

**As a** stapler-squad server, **I want** typed `ListSources`, `CreateSession`,
and `GetSession` calls, **so that** dispatch and polling never build URLs or
parse JSON at their own call sites.

**Acceptance Criteria**:
- The client authenticates with the `x-goog-api-key` header against `https://jules.googleapis.com/v1alpha`.
  - *Given* a `JulesClient` built against an `httptest.Server` with a `JulesTokenSource` returning `JulesAPIKey("test-key-123")`, *When* `GetSession(ctx, JulesSessionName("sessions/abc"))` is called, *Then* the test server observes exactly one `GET /v1alpha/sessions/abc` whose `x-goog-api-key` header equals `test-key-123` and whose `Authorization` header is empty.
- `CreateSession` sends the fire-and-forget MVP body shape.
  - *Given* a `CreateSessionRequest{Prompt: "Fix the flaky poller test", Source: "sources/github-tstapler-stapler-squad", StartingBranch: "backlog/fix-flaky-poller"}`, *When* `CreateSession` is called, *Then* the request body decodes to `{"prompt":"Fix the flaky poller test","sourceContext":{"source":"sources/github-tstapler-stapler-squad","githubRepoContext":{"startingBranch":"backlog/fix-flaky-poller"}},"requirePlanApproval":false,"automationMode":"AUTO_CREATE_PR"}`.
- `GetSession` surfaces the PR from `outputs[]`.
  - *Given* the server responds `{"name":"sessions/abc","state":"COMPLETED","outputs":[{"pullRequest":{"url":"https://github.com/tstapler/stapler-squad/pull/700","title":"Fix flaky poller test"}}]}`, *When* `GetSession` is called, *Then* the returned `JulesSession.State == JulesStateCompleted` and `Outputs[0].PullRequest.URL == "https://github.com/tstapler/stapler-squad/pull/700"`.
- The package imports nothing from `session/` or `server/`.
  - *Given* the repo at HEAD, *When* `go list -deps ./jules` is run, *Then* no line contains `github.com/tstapler/stapler-squad/session` or `.../server`.

**Files**: `jules/client.go`, `jules/client_test.go`

##### Task 1.1.2a: Client struct, constructor, and request builder (~5 min)
- Create `jules/client.go`: `type Client struct { httpClient *http.Client; baseURL string; tokens JulesTokenSource; limiter *rateLimiter }`.
- Add `type JulesTokenSource interface { APIKey(ctx context.Context) (JulesAPIKey, error) }`.
- Add `NewClient(tokens JulesTokenSource, opts ...Option)` with functional options `WithBaseURL` (test seam) and `WithHTTPClient`; default `&http.Client{Timeout: 30 * time.Second}` (mirroring `github/http_client.go`'s shared-client shape).
- Add unexported `newRequest(ctx, method, path string, body any) (*http.Request, error)` that resolves the key, sets `x-goog-api-key` from `key.reveal()`, sets `Content-Type: application/json` for non-nil bodies, and never logs.
- Files: `jules/client.go`

##### Task 1.1.2b: `ListSources`, `CreateSession`, `GetSession` (~5 min)
- Add the three DTOs (`JulesSource`, `JulesSession`, `JulesPullRequestOutput`, `CreateSessionRequest`) and the three methods, each: build request → `do` → `classifyJulesResponse` → `json.NewDecoder(resp.Body).Decode`.
- `CreateSession` hardcodes `requirePlanApproval:false` and `automationMode:"AUTO_CREATE_PR"` (no caller knob — MVP is fire-and-forget per requirements.md).
- `ListSources` handles `nextPageToken` by looping until empty, capping at 10 pages.
- Files: `jules/client.go`

##### Task 1.1.2c: `httptest`-backed client tests (~5 min)
- Create `jules/client_test.go` covering each acceptance criterion above with an `httptest.Server` asserting method, path, headers, and body.
- Add `TestJulesPackage_should_NotImportSessionOrServer_When_DepsListed` running `go list -deps ./jules` via `exec.Command` and asserting the absence of both import paths.
- Files: `jules/client_test.go`

---

#### Story 1.1.3: Typed error classification and reactive rate-limit handling

**As a** poller running unattended, **I want** HTTP failures classified into
sentinels and 429s to back the client off, **so that** an expired key logs once
instead of every 60 seconds, and a rate-limited tick is skipped instead of
guaranteed to fail.

**Acceptance Criteria**:
- A `401` or `403` maps to `ErrJulesNotConfigured`.
  - *Given* the Jules API responds `403 {"error":{"message":"API key not valid"}}`, *When* `GetSession` is called, *Then* the returned error satisfies `errors.Is(err, jules.ErrJulesNotConfigured)` and its message contains no key material.
- A `429` maps to `ErrJulesRateLimited` and arms the limiter.
  - *Given* the API responds `429` with header `Retry-After: 120`, *When* `GetSession` is called and then `client.IsLimited()` is checked, *Then* the error satisfies `errors.Is(err, jules.ErrJulesRateLimited)`, `IsLimited()` returns `true`, and `RetryAfter()` returns `120 * time.Second`.
- The limiter disarms once the window passes.
  - *Given* a client armed by a `429` with `Retry-After: 1`, *When* the test clock advances 2 seconds, *Then* `IsLimited()` returns `false`.
- A `404` on `GetSession` maps to a distinct `ErrJulesSessionNotFound` so the poller can end a vanished session rather than retry forever.
  - *Given* the API responds `404` for `sessions/gone`, *When* `GetSession(ctx, "sessions/gone")` is called, *Then* `errors.Is(err, jules.ErrJulesSessionNotFound)` is true.

**Files**: `jules/errors.go`, `jules/rate_limit.go`, `jules/errors_test.go`

##### Task 1.1.3a: Sentinel errors and `classifyJulesResponse` (~4 min)
- Create `jules/errors.go` with `ErrJulesNotConfigured`, `ErrJulesRateLimited`, `ErrJulesSessionNotFound`, `ErrJulesSourceNotRegistered`, `ErrJulesTransient`.
- Add `classifyJulesResponse(resp *http.Response) error` mapping 401/403 → not-configured, 404 → not-found, 429 → rate-limited, 5xx → transient, other non-2xx → a wrapped generic error carrying **status code and path only** (never body beyond a 512-byte truncated, header-free excerpt).
- Files: `jules/errors.go`

##### Task 1.1.3b: `julesRateLimitTransport` decorator (~5 min)
- Create `jules/rate_limit.go` with a `rateLimiter` holding `mu sync.Mutex`, `until time.Time`, `now func() time.Time` (test seam), plus `IsLimited()`, `RetryAfter()`, `observe(*http.Response)`.
- Wrap it as an `http.RoundTripper` decorator set as the client's `Transport`, so every endpoint is covered without per-call code (mirrors `github/http_client.go`'s `rateLimitTransport`).
- Parse `Retry-After` as seconds; fall back to a 60s default when absent or unparseable.
- Files: `jules/rate_limit.go`, `jules/client.go`

##### Task 1.1.3c: Error and limiter tests (~5 min)
- Create `jules/errors_test.go` with a table over status codes → expected sentinel, plus the arm/disarm test using an injected clock.
- Add an assertion that the error string for a 403 whose body echoes the key contains neither the key nor the `x-goog-api-key` header name.
- Files: `jules/errors_test.go`

---

#### Story 1.1.4: Golden-fixture contract tests against recorded responses

**As a** maintainer of an integration against an alpha API, **I want** decoding
asserted against recorded JSON fixtures, **so that** a schema change is caught by
a failing fixture rather than absorbed silently by an out-of-date hand-written mock.

**Acceptance Criteria**:
- Fixtures for the three endpoints live on disk and are decoded by the real DTOs.
  - *Given* `jules/testdata/session_completed.json` recorded from a real `Sessions.get`, *When* `TestGoldenFixtures_should_DecodeIntoJulesSession_When_SessionCompleted` runs, *Then* it decodes without error and asserts `State == JulesStateCompleted` and a non-empty `Outputs[0].PullRequest.URL`.
- An unknown field in a fixture is visible, not swallowed.
  - *Given* a fixture containing a top-level key `"unexpectedNewField"`, *When* the fixture is decoded with `json.Decoder.DisallowUnknownFields()` in the drift test, *Then* the test **fails with a message naming the field**, prompting a deliberate DTO update rather than silent absorption.
- Fixtures carry provenance.
  - *Given* any file in `jules/testdata/`, *When* it is opened, *Then* a sibling `jules/testdata/README.md` records which endpoint, which date, and how to re-record it.

**Files**: `jules/golden_test.go`, `jules/testdata/session_created.json`, `jules/testdata/session_completed.json`, `jules/testdata/session_failed.json`, `jules/testdata/sources_list.json`, `jules/testdata/README.md`

##### Task 1.1.4a: Author the fixtures (~5 min)
- Create the four JSON fixtures using the exact field shapes documented in `research/stack.md` §Sessions and `research/architecture.md` §0 (`name`, `id`, `state`, `title`, `outputs[].pullRequest.{url,title,description}`, `createTime`, `updateTime`, `url`; sources as `sources/github-{owner}-{repo}`).
- Write `jules/testdata/README.md` with the re-record command: `curl -H "x-goog-api-key: $JULES_API_KEY" https://jules.googleapis.com/v1alpha/sessions/<id>` and the date recorded.
- Files: `jules/testdata/*.json`, `jules/testdata/README.md`

##### Task 1.1.4b: Golden decode + drift tests (~5 min)
- Create `jules/golden_test.go`: one test per fixture asserting the decoded values, plus `TestGoldenFixtures_should_RejectUnknownFields_When_SchemaDrifts` using `DisallowUnknownFields`.
- While recording, note in `README.md` whether any cost field and whether more than one `outputs[]` entry appeared — this closes two Unresolved Questions.
- Files: `jules/golden_test.go`

---

### Epic 1.2: Credential storage and leak prevention

**Goal**: The `JulesAPIKey` lives in the OS keychain and is reachable through the
repo's existing `CredentialChain`, with no path that can log it.

#### Story 1.2.1: Keychain-backed `JulesTokenSource`

**As a** user, **I want** my Jules API key stored in the OS keychain, **so that**
it is not sitting in `config.json` where a screenshot, backup, or sync would
expose write access to every repo my Jules account can reach.

**Acceptance Criteria**:
- The key round-trips through `go-keyring` under a dedicated service name.
  - *Given* a `KeyringTokenSource` with the keyring test seam injected, *When* `SetJulesAPIKey(ctx, JulesAPIKey("AIzaSyD-EXAMPLE"))` then `APIKey(ctx)` are called, *Then* `APIKey` returns `JulesAPIKey("AIzaSyD-EXAMPLE")` and the underlying write targeted service `"stapler-squad-jules"` — not `"stapler-squad"` (GitHub's) or `"stapler-squad-ssh"`.
- An absent key is "feature off", not an error to retry.
  - *Given* an empty keyring, *When* `APIKey(ctx)` is called, *Then* it returns `errors.Is(err, ErrJulesNotConfigured) == true`.
- A hung Secret Service does not hang the server, the *first* time.
  - *Given* a keyring stub whose read blocks forever, *When* `APIKey(ctx)` is called, *Then* it returns within ~5 seconds with a timeout error (mirrors `session/sshremote/keystore.go:36`'s `defaultIdentityTimeout`).
- **(pre-mortem P1 #4) A wedged keychain degrades to at most one blocked goroutine, ever — not one per caller.** Task 1.2.1a's original design copied `session/sshremote/keystore.go` verbatim, including its package-level `keyringMu` that a single hung call holds forever, serializing every future read behind it. That is tolerable for SSH identity reads (rare/one-off) but not for Jules: the credential is re-resolved on every outbound HTTP request (Task 1.1.2a) **and** by the poller once per open session per ~60s tick (Task 2.3.1b), so the same bug here is a creeping goroutine leak that permanently wedges both dispatch and polling. Fixed by adding a short-TTL cache plus a single-in-flight-probe circuit breaker (Task 1.2.1a below) so a hang can pile up at most one background goroutine for the life of the process, regardless of call volume.
  - *Given* a keyring stub whose read blocks forever, *When* `APIKey(ctx)` is called once (which times out and opens the circuit) and then called 50 more times in a row, simulating 50 poll ticks/HTTP requests during the outage, *Then* only the first call waits ~5s; every subsequent call returns within 1ms with an error satisfying `errors.Is(err, ErrJulesKeychainPaused)`, and an exported-for-test probe-count hook (not raw `runtime.NumGoroutine()`, which is flake-prone) shows exactly one probe goroutine was ever started for the underlying keyring call across all 51 calls.
- A resolved key is cached so a healthy keychain is hit once per TTL, not once per call.
  - *Given* a keyring stub that succeeds and `cacheTTL = 5 * time.Minute` (test-injected shorter), *When* `APIKey(ctx)` is called 10 times within the TTL window, *Then* the underlying keyring `Get` seam is invoked exactly once, and the other 9 calls return the cached value.
- The circuit reopens for exactly one probe after cooldown, and closes on success.
  - *Given* the circuit opened after a timeout with a test-injected `cooldown = 1s`, *When* the injected clock advances past the cooldown and `APIKey(ctx)` is called against a keyring stub that now succeeds, *Then* exactly one probe is attempted, it succeeds, the cache is populated, the circuit closes, and a subsequent call within the new TTL hits the cache with zero further keyring calls.

**Files**: `jules/keychain.go`, `jules/keychain_test.go`

##### Task 1.2.1a: `KeyringTokenSource` with a bounded single-probe circuit breaker and TTL cache (~7 min)
- Create `jules/keychain.go`: `const keychainService = "stapler-squad-jules"`, `const keychainAccount = "api-key"`, `const defaultKeyringTimeout = 5 * time.Second`, `const defaultCacheTTL = 5 * time.Minute`, `const defaultCircuitCooldown = 5 * time.Minute`, package-level `keyringGet`/`keyringSet`/`keyringDelete` vars (test seams) defaulting to `zalando/go-keyring`.
- `KeyringTokenSource` holds its own `stateMu sync.Mutex` (guards only the four fields below — cheap, never held across the actual keyring call) plus `cachedKey JulesAPIKey`, `cachedAt time.Time`, `circuitOpenUntil time.Time`, `probeInFlight bool`.
- `APIKey(ctx)` logic, in order: (1) if `now - cachedAt < cacheTTL`, return the cached key with no keyring call; (2) if `now < circuitOpenUntil` (circuit open): if `!probeInFlight`, set it `true` and launch exactly one background goroutine that runs the same timeout-raced keyring read as before — on success it updates the cache and closes the circuit, on failure it re-extends `circuitOpenUntil` and logs `jules keychain paused` once at `Warn`, and either way clears `probeInFlight` when it finally returns — then, regardless of whether a probe was just launched or one was already in flight, return `ErrJulesKeychainPaused` (wraps `ErrJulesNotConfigured`) immediately, without blocking; (3) otherwise (cache stale, circuit closed): run the timeout-raced keyring read *synchronously* for this call, exactly as `raceKeyringOp` does at `session/sshremote/keystore.go:171` — on success, populate the cache and return the key; on timeout, set `circuitOpenUntil = now + cooldown`, log `jules keychain paused` once at `Warn`, and return the timeout error to this caller only.
- The key structural property this buys over the `sshremote` pattern: at most one goroutine is ever blocked inside the underlying keyring call at a time — step (2) never starts a second probe while `probeInFlight` is true, and step (3) (the only other path that calls the keyring) is only reachable when the circuit is closed, i.e. no probe is running. A permanently wedged Secret Service therefore leaks at most one goroutine for the life of the process, not one per HTTP request or poll tick.
- `SetJulesAPIKey`/`DeleteJulesAPIKey` invalidate the cache (`cachedAt = time.Time{}`) and reset the circuit (`circuitOpenUntil = time.Time{}`) so a freshly-entered key is tried immediately rather than waiting out a stale cooldown; each still runs its own keyring op through the same timeout race (not the cache/circuit path, since a write must reach the keyring or fail honestly).
- Add `ErrJulesKeychainPaused` to `jules/errors.go` (Task 1.1.3a), wrapping `ErrJulesNotConfigured` via `errors.Join` or a custom `Is`, so all existing "feature off" handling (dispatch guard, poller skip) treats it identically without a new branch.
- Files: `jules/keychain.go`, `jules/errors.go`

##### Task 1.2.1b: Keychain tests including the bounded-leak and cache/circuit cases (~6 min)
- Create `jules/keychain_test.go` with the original three acceptance criteria plus the three new ones above, using the seam vars. For the bounded-leak assertion, add an unexported test hook (e.g. an injectable `onProbeStart func()` called at the top of the background probe goroutine) rather than asserting on `runtime.NumGoroutine()`, which is flaky under `go test -race`/parallel tests; the timeout test uses a stub that sleeps and asserts elapsed < 6s for the first call and < 1ms for calls made while the circuit is open.
- Files: `jules/keychain_test.go`

---

#### Story 1.2.2: `CredentialChain` integration and a no-secret-logging guard

**As a** developer, **I want** the Jules key resolved through the same
`CredentialChain` as Gemini/Anthropic and a test that fails if any `jules/` code
path can log it, **so that** credential handling stays uniform and the repo's
missing outbound-HTTP redaction (pitfalls §2) is compensated for explicitly.

**Acceptance Criteria**:
- Provider `"jules"` resolves through the chain to the keychain value.
  - *Given* a `CredentialChain` including `JulesCredentialSource` and a keyring holding `"AIzaSyD-EXAMPLE"`, *When* `chain.Resolve(ctx, "jules")` is called, *Then* it returns a `Credential` whose source name is `"jules_keychain"` and whose token equals `"AIzaSyD-EXAMPLE"`.
- Env-var override still wins for scripted/CI use, matching the existing chain order.
  - *Given* `JULES_API_KEY=env-key` set and a keyring holding `"keychain-key"`, *When* `chain.Resolve(ctx, "jules")` is called, *Then* the returned token is `"env-key"`.
- No `jules/` source file can emit the key.
  - *Given* the `jules/` package at HEAD, *When* `TestJulesPackage_should_NotLogSecrets_When_SourceScanned` greps every `.go` file for `reveal()` occurrences, *Then* `reveal()` appears exactly once, inside `newRequest` in `jules/client.go`, and appears in no `slog`/`fmt.Print`/`log.` call.

**Files**: `server/services/credentials.go`, `server/services/credentials_test.go`, `jules/secrets_guard_test.go`

##### Task 1.2.2a: Add `JulesCredentialSource` to the chain (~5 min)
- In `server/services/credentials.go`, add `JulesCredentialSource` implementing `CredentialSource` (`Name() string` → `"jules_keychain"`, `Resolve(ctx, provider)` returning `(Credential, bool, error)`, no-op unless `provider == "jules"`), delegating to `jules.KeyringTokenSource`.
- Register it in `NewDefaultChain` (`server/services/credentials.go:110`) after `EnvVarCredentialSource` so an env var still wins; extend `EnvVarCredentialSource.Resolve` to recognize `JULES_API_KEY` for provider `"jules"`.
- Files: `server/services/credentials.go`

##### Task 1.2.2b: Chain tests plus the secrets-guard test (~5 min)
- Add the two chain-order cases to `server/services/credentials_test.go`.
- Create `jules/secrets_guard_test.go` implementing the `reveal()` source scan (walk `jules/*.go` excluding `_test.go`, count occurrences, assert file and line context).
- Files: `server/services/credentials_test.go`, `jules/secrets_guard_test.go`

---

### Epic 1.3: Source registry

**Goal**: Resolve a repo to its `JulesSourceName` cheaply and give the user a
precise error when the repo was never connected at jules.google.com.

#### Story 1.3.1: `owner/repo → JulesSourceName` cache

**As a** dispatcher, **I want** an `owner/repo` lookup cached in memory, **so that**
every dispatch does not re-list every source, and an unconnected repo produces
an actionable message rather than a generic API error.

**Acceptance Criteria**:
- A hit is served without a second API call.
  - *Given* a `JulesSourceRegistry` whose backing client has already returned `sources/github-tstapler-stapler-squad` once, *When* `Resolve(ctx, "tstapler", "stapler-squad")` is called a second time within the TTL, *Then* the result is the same `JulesSourceName` and the client recorded exactly one `ListSources` call.
- A miss produces the actionable sentinel.
  - *Given* a registry whose `ListSources` returns only `sources/github-tstapler-dotfiles`, *When* `Resolve(ctx, "tstapler", "stapler-squad")` is called, *Then* the error satisfies `errors.Is(err, jules.ErrJulesSourceNotRegistered)` and its message names `tstapler/stapler-squad` and instructs the user to connect the repo at jules.google.com.
- The cache expires.
  - *Given* a registry with `TTL = 10 * time.Minute` populated at T0, *When* the injected clock is advanced to T0+11m and `Resolve` is called, *Then* a second `ListSources` call is made.

**Files**: `jules/source_registry.go`, `jules/source_registry_test.go`

##### Task 1.3.1a: Implement `JulesSourceRegistry` (~5 min)
- Create `jules/source_registry.go`: `sync.Map` keyed `"owner/repo"` holding `{name JulesSourceName; at time.Time}`, `TTL` field (default 10m), injected `now func() time.Time`, and `Resolve(ctx, owner, repo string) (JulesSourceName, error)`.
- On miss: call `ListSources`, populate every returned source (parsing `sources/github-{owner}-{repo}` back into owner/repo), then re-check; return `ErrJulesSourceNotRegistered` wrapped with the owner/repo if still absent.
- Files: `jules/source_registry.go`

##### Task 1.3.1b: Registry tests (~4 min)
- Create `jules/source_registry_test.go` with a fake client counting `ListSources` calls; cover the three criteria above.
- Files: `jules/source_registry_test.go`

---

## Phase 2: Backend integration

### Epic 2.1: Session role and storage primitives

**Goal**: Give a Jules unit of work a first-class, non-tmux `ItemSession`
identity, the two narrow queries the dispatcher and poller need, and make
every existing "does this item already have an active session" predicate see
it (Story 2.1.3).

#### Story 2.1.1: `SessionRoleJulesWork`, excluded from tmux cleanup

**As a** cleanup sweep, **I want** `jules_work` classified as non-tmux-backed,
**so that** archiving a done Jules item does not try to kill a tmux pane that
never existed.

**Acceptance Criteria**:
- The constant exists and is excluded from the tmux predicate.
  - *Given* `session.SessionRoleJulesWork`, *When* `session.IsTmuxBackedSessionRole(session.SessionRoleJulesWork)` is called, *Then* it returns `false`, while `IsTmuxBackedSessionRole("work")` and `("review")` still return `true`.
- The exclusion is locked in by a test that enumerates every role, so a future role addition cannot silently default into the tmux path.
  - *Given* the four role constants `work`, `triage`, `review`, `jules_work`, *When* `TestIsTmuxBackedSessionRole_should_CoverEveryDeclaredRole_When_RolesEnumerated` runs, *Then* it asserts the exact expected truth value for each and fails if the declared-role set has grown without the table growing.
- The terminal-item sweep skips Jules rows without error.
  - *Given* a backlog item in `done` with one ended `jules_work` `ItemSession`, *When* `reconcileTerminalItemSessions` runs, *Then* it logs no tmux-kill attempt for that row and the item stays `done`.

**Files**: `session/backlog.go`, `session/backlog_test.go`

##### Task 2.1.1a: Add the constant and update the predicate doc comment (~4 min)
- In `session/backlog.go:48-53`, add `SessionRoleJulesWork = "jules_work"` to the role const block.
- Extend `IsTmuxBackedSessionRole`'s doc comment (`session/backlog.go:55-75`) with one sentence: Jules sessions run on Google's infrastructure, so like triage they have no local pane to kill — do **not** narrate the whole integration there (per the comment-proportionality rule).
- The predicate body needs no change (it is an allowlist of `work`/`review`); add the test below to make that deliberate rather than incidental.
- Files: `session/backlog.go`

##### Task 2.1.1b: Role-coverage test and sweep verification (~5 min)
- Add `TestIsTmuxBackedSessionRole_should_CoverEveryDeclaredRole_When_RolesEnumerated` to `session/backlog_test.go`.
- Re-read the two callers named in the Migration Plan (`reconcileTerminalItemSessions` at `session/backlog_lifecycle_archive.go:92`, `archiveItemWorkSessions` at `server/services/backlog_service.go:1018`) and confirm each treats `false` as "skip the kill" rather than "assume triage"; if either branches on `role == SessionRoleTriage` specifically, change it to use the predicate.
- Files: `session/backlog_test.go`, plus whichever of the two callers needs the predicate

---

#### Story 2.1.2: Repository queries for open and recent Jules sessions

**As a** poller and a spend guard, **I want** two narrow queries — "all open
`jules_work` sessions" and "count of `jules_work` sessions created since T" —
**so that** neither has to load every item session in the database.

**Acceptance Criteria**:
- Open-session listing returns only unfinished Jules rows, across all items.
  - *Given* three `ItemSession` rows — one `jules_work` with `ended_at` nil, one `jules_work` with `ended_at` set, one `work` with `ended_at` nil, *When* `ListOpenJulesItemSessions(ctx)` is called, *Then* it returns exactly one row, the first one, carrying its `BacklogItemID`.
- The daily count respects the window.
  - *Given* two `jules_work` rows created 2h ago and one created 30h ago, *When* `CountJulesItemSessionsSince(ctx, time.Now().Add(-24*time.Hour))` is called, *Then* it returns `2`.
- **(pre-mortem P2 #5) The daily count excludes attempts that never billed.** `CountJulesItemSessionsSince` feeds `MaxJulesSessionsPerDay` (Story 2.2.2), which exists to cap real cloud spend. A row that never reached a confirmed `jules-sessions/{id}` — still carrying the `julesPendingUUIDPrefix` reservation, or ended with `end_reason = "dispatch_failed"` — was never billed, so it must not consume the cap; otherwise a bad key or a Jules outage burns the whole day's cap on failed attempts with nothing actually running.
  - *Given* one `jules_work` row created 2h ago that reached a real session (`session_uuid` starts `"jules-sessions/"`), one created 1h ago still reserved (`session_uuid` starts with `julesPendingUUIDPrefix`), and one created 30 min ago ended with `end_reason = "dispatch_failed"`, *When* `CountJulesItemSessionsSince(ctx, time.Now().Add(-24*time.Hour))` is called, *Then* it returns `1`.
- A progress touch updates only `last_progress_at`.
  - *Given* a `jules_work` `ItemSession` with `last_commit_sha == ""` and `last_progress_at == nil`, *When* `TouchItemSessionProgress(ctx, id, ts)` is called with `ts = 2026-09-01T10:00:00Z`, *Then* `last_progress_at == ts` and `last_commit_sha`, `base_commit_sha`, `commit_count_since_spawn` are all still at their zero values.

**Files**: `session/storage_backlog.go`, `session/storage.go`, `session/repository.go`, `session/storage_backlog_test.go`

##### Task 2.1.2a: Add the two `EntRepository` queries (~5 min)
- In `session/storage_backlog.go`, add `ListOpenJulesItemSessions(ctx) ([]ItemSessionBacklogEntry, error)` predicated on `session_role == SessionRoleJulesWork` and `ended_at IS NULL`, and `CountJulesItemSessionsSince(ctx, since time.Time) (int, error)`.
- `CountJulesItemSessionsSince`'s WHERE clause: `session_role == SessionRoleJulesWork AND created_at >= since AND NOT (session_uuid HAS PREFIX julesPendingUUIDPrefix) AND end_reason != "dispatch_failed"` — excludes reservations never confirmed and attempts that failed at `CreateSession`, so the count reflects confirmed billed creations only (pre-mortem P2 #5).
- Reuse the existing `ItemSessionBacklogEntry` shape already returned by `GetAllItemSessionsWithBacklogInfo` (`session/storage.go:1281`) so the poller gets the item ID without a second query.
- Files: `session/storage_backlog.go`

##### Task 2.1.2b: Add `TouchItemSessionProgress` (~4 min)
- In `session/storage_backlog.go`, add `TouchItemSessionProgress(ctx, id string, at time.Time) error` setting **only** `last_progress_at`. Do **not** reuse `UpdateItemSessionFileTouch` (`session/storage_backlog.go:601`) — its name asserts a filesystem event that never happens for a Jules session.
- Files: `session/storage_backlog.go`

##### Task 2.1.2c: `Storage` wrappers, interface entries, and tests (~5 min)
- Add the three pass-through methods on `*Storage` in `session/storage.go` beside the existing `UpdateItemSession*` block (`session/storage.go:1113-1175`), and to whichever repository interface in `session/repository.go` declares the others.
- Add the three acceptance-criterion tests to `session/storage_backlog_test.go`.
- Files: `session/storage.go`, `session/repository.go`, `session/storage_backlog_test.go`

---

#### Story 2.1.3: An open Jules session blocks local respawn/reopen the same way an open work session does

**As a** user, **I want** the autonomous driver and every manual respawn/reopen
path to see an open `jules_work` session as "this item is already being worked
on", **so that** stapler-squad never starts a second, competing local agent on
the same branch Jules already has open.

**Context (pre-mortem P1 #1, not folded into Story 2.1.1 per the pre-mortem's
explicit instruction)**: the Migration Plan (above) only re-checked
`IsTmuxBackedSessionRole`'s two callers. A separate, broader family of "does
this item have an active session" predicates is hardcoded to
`SessionRoleWork`/`SessionRoleReview` and never sees `jules_work`:
- `hasActiveSession` (`session/backlog_lifecycle.go:2037-2044`) — used by
  `reconcileRespawnBlockedActiveResolution` (`session/backlog_lifecycle_review.go:778`),
  `reconcileOrphanedAgentPRs` (`session/backlog_lifecycle_pr.go:269`), and
  `handlePRPendingTransitionFailed` (`session/backlog_lifecycle_pr.go:824`).
- `hasActiveWorkSession`/`findActiveWorkSession`
  (`server/services/backlog_service_triage.go:1255-1271`) — used by
  `spawnSessionAfterGates`'s 8b duplicate-spawn guard (`:1007`) and
  `AutoRespawnAutonomousWork` (`:1909`), the latter reachable from the
  default-on `autoSpawnReadyItemsEnabled` sweep
  (`server/services/backlog_service.go:370-372` →
  `server/services/autonomous_orchestration_service.go:31,398`) with **no
  user action required** — an item with only an open `jules_work` session
  reads as having no active session to this whole family, so the autonomous
  driver can start a local Claude Code session on the same branch Jules is
  already working, unattended.

**Design note — why this adds a new predicate rather than widening
`findActiveWorkSession` itself**: `findActiveWorkSession`'s result is not only
used to *block* a new spawn — at `AutoReopenForPRFix`
(`server/services/backlog_service_triage.go:2116`) it is used to *steer* the
found session via a live tmux write (`steerActiveSessionForPRFix`). A
`jules_work` `ItemSession` has no tmux pane, so if `findActiveWorkSession`
were widened to match it, that steer call would silently misbehave the one
time this invariant is ever exercised. By construction this can't happen
today — `JulesSessionPoller` always ends the `jules_work` session as part of
recording the PR (Story 2.3.2), before the item can ever reach `pr_pending`
where `AutoReopenForPRFix` operates — but the fix should not depend on that
invariant holding forever without a guard. So: add a narrow, purpose-built
`session.HasActiveJulesSession` predicate, fold it directly into
`hasActiveSession` (safe — none of its three call sites steer anything, they
only decide "is this item still legitimately in flight"), and call it
*alongside*, not instead of, `findActiveWorkSession` at the two blocking gates
that decide whether to start brand-new local work. Leave
`findActiveWorkSession`/`hasActiveWorkSession` itself unchanged, and add a
defensive assertion at the one call site that steers a session so a future
violation of the invariant fails loudly instead of silently.

**Acceptance Criteria**:
- `hasActiveSession` recognizes an open Jules session.
  - *Given* `[]ItemSessionSummary{{Role: SessionRoleJulesWork, EndedAt: nil}}`, *When* `hasActiveSession(sessions)` is called, *Then* it returns `true`; *and given* the same row with `EndedAt` set, *Then* it returns `false`.
- The 8b spawn guard rejects spawning a local session over an open Jules session.
  - *Given* an item with one open `jules_work` `ItemSession` and no `work` session, *When* `spawnSessionAfterGates` is called for that item, *Then* it returns `connect.CodeAlreadyExists` with a message stating a Jules session is already running for this item, and `SessionCreator.CreateSession` (the local tmux path) is never called.
- `AutoRespawnAutonomousWork` does not respawn over an open Jules session.
  - *Given* an item in `in_progress` with one open `jules_work` `ItemSession` and no `work` session, *When* `AutoRespawnAutonomousWork(ctx, itemID)` is called (as it would be from the autonomous sweep), *Then* it returns without spawning a session, logs the same shape of "respawn blocked, active session found" line the existing work-session case logs, and no new `ItemSession` row is created.
- `AutoRespawnReview` does not respawn a review session over an open Jules session.
  - *Given* an item with one open `jules_work` `ItemSession`, *When* `AutoRespawnReview` is called for it, *Then* it returns without spawning a review session, mirroring the existing `findActiveWorkSession`/`findActiveReviewSession` block.
- Steering a Jules session (should the invariant above ever be violated) fails loudly, not silently.
  - *Given* `AutoReopenForPRFix`'s active-session lookup somehow returns a session with `Role == SessionRoleJulesWork` (constructed directly in the test — the poller's own end-before-PR invariant is not re-tested here), *When* `steerActiveSessionForPRFix` is called with it, *Then* it returns an error / logs at `Error` naming the invariant violation and makes no tmux write, rather than attempting one.
- Explicitly out of scope, and why (recorded so a future reader doesn't assume an oversight): `countLiveBacklogWorkSessions` (`backlog_service_triage.go:1246`) is **not** changed — it feeds `MaxConcurrentBacklogWorkItems`, a separate WIP pool from `MaxConcurrentJulesSessions` (Story 2.2.2); double-counting a `jules_work` row there would incorrectly shrink local capacity for cloud work. `AutoReopenAfterFailedReview`'s stale-session notice (`:1744`) and `remediatePRFixWithBackoffGate`'s `HasActiveWorkSession` query (`:2058`) are also unchanged: both operate on items already in `review`/`pr_pending`, a state a `jules_work` session cannot be open in by the poller's own end-before-transition invariant (Story 2.3.2), so there is nothing for them to miss.

**Files**: `session/backlog.go`, `session/backlog_lifecycle.go`, `session/backlog_lifecycle_test.go`, `server/services/backlog_service_triage.go`, `server/services/backlog_service_triage_test.go`

##### Task 2.1.3a: `session.HasActiveJulesSession` and `hasActiveSession` (~4 min)
- In `session/backlog.go`, beside `IsTmuxBackedSessionRole`, add exported `HasActiveJulesSession(sessions []ItemSessionSummary) bool` checking `EndedAt == nil && Role == SessionRoleJulesWork`.
- In `session/backlog_lifecycle.go:2037-2044`, extend `hasActiveSession`'s condition to `s.EndedAt == nil && (s.Role == SessionRoleWork || s.Role == SessionRoleReview || s.Role == SessionRoleJulesWork)` (or call the new helper internally — either is fine, same package); update its doc comment's one-line summary to say "work-, review-, or Jules-role session" rather than narrating the whole rationale (per the comment-proportionality rule).
- Files: `session/backlog.go`, `session/backlog_lifecycle.go`

##### Task 2.1.3b: Gate `spawnSessionAfterGates` (8b) and `AutoRespawnAutonomousWork` on an open Jules session (~5 min)
- In `server/services/backlog_service_triage.go`, immediately alongside the existing `findActiveWorkSession(priorSessions)` check at the 8b guard (`:1007`) and at `AutoRespawnAutonomousWork` (`:1909`), add `session.HasActiveJulesSession(priorSessions)` / `(sessions)` to the blocking condition, returning the existing error/notification shape with a Jules-specific message (`"a Jules session is already running for this item"`) so the two cases are distinguishable in logs and UI.
- Do the same at `AutoRespawnReview` (`:2260`), which already ORs `findActiveWorkSession`/`findActiveReviewSession`.
- Files: `server/services/backlog_service_triage.go`

##### Task 2.1.3c: Defensive guard at the one steering call site (~3 min)
- In `steerActiveSessionForPRFix` (or immediately before its call in `AutoReopenForPRFix`, `:2116`), add a one-line check: if the resolved active session's `Role == SessionRoleJulesWork`, log at `Error` naming the invariant violation and return early instead of attempting a tmux write. This is defense-in-depth, not an expected path — see the Design note above.
- Files: `server/services/backlog_service_triage.go`

##### Task 2.1.3d: Tests (~5 min)
- Add the five acceptance-criterion cases across `session/backlog_lifecycle_test.go` and `server/services/backlog_service_triage_test.go`.
- Files: `session/backlog_lifecycle_test.go`, `server/services/backlog_service_triage_test.go`

---

### Epic 2.2: Dispatch service

**Goal**: One guarded, idempotent path from "user clicks Dispatch to Jules" to
"a billed Jules session exists and a local `ItemSession` records it".

#### Story 2.2.1: `JulesDispatchService` with the reservation-based create

**As a** user, **I want** dispatching to Jules to create exactly one cloud
session even if I double-click or the write fails halfway, **so that** I am never
billed twice for the same backlog item.

**Acceptance Criteria**:
- A successful dispatch reserves, creates, confirms, and transitions.
  - *Given* a backlog item `ID=item-1` in `ready` with `RepoPath=/home/tstapler/code/github.com/tstapler/stapler-squad`, that repo already present in `JulesConfig.EgressAcknowledgedRepos` (set up via a prior `ConfirmEgressConsent` call — see Story 2.4.2 — never via the request itself, per pre-mortem P1 #3), no unresolved blockers, and a `JulesDispatchRequest{Branch:"backlog/item-1", Prompt:"Fix the flaky poller test"}`, *When* `DispatchToJules` succeeds against a fake client returning `sessions/xyz`, *Then* in order: an `ItemSession{Role:"jules_work", SessionUUID:"jules-pending-<uuid>"}` is created, `CreateSession` is called exactly once, the row's `session_uuid` becomes `"jules-sessions/xyz"`, and the item transitions `ready → in_progress` via `transitionWithGuard` with `triggeredBy = "user"` and `hasUnresolvedBlockers = false` sourced from the real `julesTransitionGuard.hasUnresolvedBlockers` check (not a hardcoded literal).
- **(persisted-state guard)** A second, sequential dispatch for the same item is refused by storage-backed state alone — this must hold even with no mutex contention, e.g. across two independently-constructed `JulesDispatchService` instances or after the first call's mutex has already been released.
  - *Given* `item-1` already has a `jules_work` `ItemSession` with `ended_at` nil (created by a prior dispatch call that has already returned, so its `TryLock` has already been released), *When* `DispatchToJules` is called again for `item-1`, *Then* it returns `errors.Is(err, ErrJulesDispatchInFlight)` and the fake client records **zero** `CreateSession` calls. This is the persisted-state guard (Task 2.2.1b), distinct from the in-process mutex below — a test that only ever calls `DispatchToJules` once per `JulesDispatchService` instance while the mutex is held cannot distinguish this guard from the mutex, so this case must exercise the guard with the mutex already free.
- **(in-process mutex guard)** Concurrent double-clicks collapse to one create.
  - *Given* two goroutines calling `DispatchToJules` for `item-1` simultaneously against a fake client whose `CreateSession` blocks 50ms, *When* both return, *Then* exactly one succeeds, the other returns `ErrJulesDispatchInFlight` from the `TryLock` guard (not the persisted-state check, which only observes a row after the winner's reservation write lands), and `CreateSession` was called exactly once.
- **(blocker gate)** An item with an unresolved blocker is rejected before any billed call, the same way a local session spawn is rejected.
  - *Given* `item-1` in `ready` with an unresolved blocking dependency (`storage.UnresolvedBlockerItemIDs` would report it as blocked) and otherwise-satisfied egress/spend guards, *When* `DispatchToJules` is called, *Then* it returns an error satisfying `errors.Is(err, session.ErrUnresolvedBlockers)`, no `ItemSession` reservation row is created, and the fake client records **zero** `CreateSession` calls.
- A create failure leaves no orphan claim.
  - *Given* a fake client whose `CreateSession` returns `ErrJulesTransient`, *When* `DispatchToJules` is called, *Then* the reservation row is ended with `end_reason = "dispatch_failed"`, the item stays in `ready`, and a visible progress note is appended naming the failure.

**Files**: `server/services/jules_dispatch_service.go`, `server/services/jules_dispatch_service_test.go`

##### Task 2.2.1a: Service skeleton, narrow client/transition interfaces, and request value object (~6 min)
- Create `server/services/jules_dispatch_service.go` with the `JulesDispatcher` interface (exact signature from the glossary) and a narrow, **locally-declared** `julesSessionCreator interface { CreateSession(ctx context.Context, req jules.CreateSessionRequest) (jules.JulesSession, error) }`, satisfied structurally by `*jules.Client`.
- Also declare a narrow, **locally-declared** `julesTransitionGuard interface { transitionWithGuard(ctx context.Context, item *session.BacklogItemData, to session.BacklogStatus, precondition *session.BacklogItemPrecondition, triggeredBy string, hasUnresolvedBlockers bool) (*session.BacklogItemData, error); hasUnresolvedBlockers(ctx context.Context, itemID string) (bool, error) }`. `*BacklogService` (`backlog_service_triage.go:720,738`) satisfies this structurally — both methods live in the same `server/services` package as `JulesDispatchService`, so the unexported names resolve — without `JulesDispatchService` needing to hold a `*BacklogService` or duplicate its `storage`/`engine` fields. This mirrors `AutonomousStuckRespawner` (`server/services/autonomous_orchestration_service.go:30`), the codebase's existing pattern for one `server/services` type reaching `BacklogService` behavior via a consumer-owned interface rather than a concrete sibling-service pointer.
- `JulesDispatchService` holds `storage *session.Storage` (for `CreateItemSession`/`UpdateItemSessionSessionUUID`/`UpdateItemSessionEndedWithReason`/`AppendProgressNote`/`ListOpenJulesItemSessions`/`CountJulesItemSessionsSince` — these are storage-layer calls, not a sibling-service dependency, so holding the concrete `*session.Storage` is consistent with how `BacklogService` itself holds it), `transitionGuard julesTransitionGuard` (the guarded-transition + blocker-check dependency — Gap 1/Gap 2 below), `client julesSessionCreator` (the interface, **not** the concrete `*jules.Client` — mirrors `JulesSessionPoller`'s `julesStatusClient` pattern from Task 2.3.1a, which is exactly what lets Task 2.2.1c inject a fake), `sources *jules.JulesSourceRegistry`, `cfg *config.Config`, `counters *JulesUsageCounter`, and `itemLocks sync.Map` holding one `*sync.Mutex` per item ID (lazily created on first use) for the double-click guard.
- `NewJulesDispatchService(storage *session.Storage, transitionGuard julesTransitionGuard, client julesSessionCreator, sources *jules.JulesSourceRegistry, cfg *config.Config, counters *JulesUsageCounter) *JulesDispatchService` — `transitionGuard` is passed as `backlogSvc` at the call site (Task 2.4.4a), which is already constructed by the time Jules dependencies are built, so no late-binding setter is needed (unlike `AutonomousStuckRespawner`, which needs one only because of its own construction-order constraints).
- The double-click guard is a **per-item mutex with `TryLock`, not `singleflight.Group`**: `DispatchToJules` calls `TryLock()` on the item's mutex and, if it is already held, returns `ErrJulesDispatchInFlight` immediately without blocking or waiting on the in-flight call; the caller that wins releases the lock via `defer` once the reserve→create→confirm sequence (Task 2.2.1b) finishes. `singleflight.Group` is rejected, not an interchangeable option: it coalesces concurrent calls into one execution and hands the *first* caller's result to every waiter, so the second concurrent caller would receive a fabricated success instead of `ErrJulesDispatchInFlight` — directly violating Story 2.2.1's own "concurrent double-clicks collapse to one create" acceptance criterion. See also the Pattern Decisions table. **This mutex is process-local and released once each call returns — it does not by itself satisfy "second dispatch refused" for a sequential (non-racing) second call; see Task 2.2.1b's persisted-state check for that.**
- Add `NewJulesDispatchRequest(itemID, branch, prompt string) (JulesDispatchRequest, error)` validating non-empty branch and prompt at construction (parse, don't validate). **No `egressAck` parameter** (pre-mortem P1 #3) — egress consent is read from durable config, never accepted from a caller; see Story 2.2.3 and Story 2.4.2.
- Declare `julesSessionUUIDPrefix = "jules-"` and `julesPendingUUIDPrefix = "jules-pending-"` as local constants in this file. **Do not import them from `session/`** — `server/services` imports `session`, so the reverse import would cycle. This duplicates the codebase's existing precedent for `headlessTriageUUIDPrefix` (`server/services/backlog_service_triage.go:423`, re-declared as `headlessTriageSessionUUIDPrefix` for the read side at `session/backlog_lifecycle_triage.go:43-50` with a comment explaining the cycle). `session/jules_session_poller.go` (Task 2.3.1a) must declare byte-identical copies of both constants; put a comment in each file pointing at the other's copy so a future value change is not made in only one place.
- Files: `server/services/jules_dispatch_service.go`

##### Task 2.2.1b: The reserve → create → confirm sequence (~7 min)
- Implement `DispatchToJules` as an ordered guard sequence, each guard returning immediately on failure and none of it reaching `client.CreateSession` (the only billed call) unless every guard before it passed:
  1. `TryLock()` the per-item mutex → `ErrJulesDispatchInFlight` if already held (in-process race guard, Task 2.2.1a). `defer` the unlock.
  2. **Persisted-state duplicate check (Gap 3):** query `storage.ListOpenJulesItemSessions(ctx)` (the same query Task 2.2.2b's spend guard needs for its concurrency count — fetch it once here and pass the result into `checkSpendGuards` rather than querying twice) and check whether any returned row's `BacklogItemID == item.ID`. If so, return `ErrJulesDispatchInFlight`. This is what actually enforces "at most one open Jules session per item" — the mutex above only ever catches two calls racing at the exact same instant in the same process; it is released as soon as each call returns, so a second, later, non-racing call (a different code path, a retried request, or a call after a process restart) sails past the mutex and is caught here instead, against durable state.
  3. `checkEgressConsent(item)` (Story 2.2.3).
  4. `checkSpendGuards(ctx, openSessions)` — concurrency ceiling + daily cap (Story 2.2.2), reusing the `openSessions` slice fetched in step 2.
  5. **Blocker gate (Gap 2):** `hasBlockers, err := s.transitionGuard.hasUnresolvedBlockers(ctx, item.ID)` — the exact same `storage.UnresolvedBlockerItemIDs`-backed check every other single-item transition in this codebase uses (`backlog_service_triage.go:720`), not a hardcoded value. If `hasBlockers`, return an error satisfying `errors.Is(err, session.ErrUnresolvedBlockers)` **before** any reservation is written or any billed call is made — this is what makes a Jules dispatch respect the same blocker gate a local session spawn already respects (a local spawn's blocker check also runs before work starts, not after — see `DequeueNextQueuedItems`'s pre-claim `UnresolvedBlockerItemIDs` call, `backlog_service_triage.go:865`).
  6. Resolve `owner/repo` from `item.RepoPath` and look up the `JulesSourceName` → `storage.CreateItemSession` with `SessionUUID = julesPendingUUIDPrefix + uuid.New().String()` → `client.CreateSession` → `storage.UpdateItemSessionSessionUUID(id, julesSessionUUIDPrefix + name)` → `s.transitionGuard.transitionWithGuard(ctx, item, session.BacklogStatusInProgress, precondition{ExpectedStatus: ready}, "user", hasBlockers)` — passing the **same `hasBlockers` variable computed in step 5** (guaranteed `false` at this point, since step 5 already returned early otherwise), not a bare `false` literal, so this call site can never drift back out of sync with the real check if the guard sequence is ever reordered.
- On any error after the reservation (step 6's `CreateItemSession` onward): `UpdateItemSessionEndedWithReason(id, now, "dispatch_failed")` **and** `AppendProgressNote(itemID, -1, "Jules dispatch failed: <reason>", "blocked")` so the failure is visible in the UI, never silent (per `feedback_document_ai_decisions_in_edge_cases`).
- Files: `server/services/jules_dispatch_service.go`

##### Task 2.2.1c: Dispatch tests including the concurrency, persisted-duplicate, and blocker-gate cases (~6 min)
- Create `server/services/jules_dispatch_service_test.go` with a fake implementing `julesSessionCreator` (call counter + injectable error/delay), a fake implementing `julesTransitionGuard` (records calls, lets a test set `hasUnresolvedBlockers` and the transition outcome), and an in-memory storage; cover all six acceptance criteria.
- The persisted-state case and the mutex case must be tested so a reviewer can see they are exercising different code paths: the persisted-state case calls `DispatchToJules` once, lets it return (mutex now free), then calls it again for the same item and asserts `ErrJulesDispatchInFlight` came from the `ListOpenJulesItemSessions` lookup finding the first call's row (not from the mutex, which is unlocked by then); the mutex case uses `sync.WaitGroup` with two goroutines racing `CreateSession`'s 50ms delay, where the persisted row does not yet exist for either goroutine to find.
- The blocker-gate case sets the fake `julesTransitionGuard.hasUnresolvedBlockers` to return `true` and asserts zero `ItemSession` rows are created and zero `CreateSession` calls happen.
- The fakes satisfy their interfaces directly — no `*jules.Client`, real `*BacklogService`, or `httptest.Server` is needed for these tests.
- Files: `server/services/jules_dispatch_service_test.go`

---

#### Story 2.2.2: Proactive spend guards

**As a** user of a metered product, **I want** hard concurrency and daily caps
enforced before any billed call, **so that** a retry-loop bug costs at most one
day's cap instead of an unbounded bill.

**Acceptance Criteria**:
- The concurrency ceiling blocks a dispatch.
  - *Given* `JulesConfig.MaxConcurrentJulesSessions = 2` and two open `jules_work` sessions on other items, *When* `DispatchToJules` is called for a third item, *Then* it returns a `FailedPrecondition`-mapped error whose message reads `2 Jules sessions are already running (limit 2)` and no `CreateSession` call is made.
- The daily cap blocks a dispatch even when nothing is currently running.
  - *Given* `JulesConfig.MaxJulesSessionsPerDay = 15`, zero open sessions, and 15 `jules_work` rows created in the last 24h, *When* `DispatchToJules` is called, *Then* it returns an error naming the daily limit and no `CreateSession` call is made.
- Config values are clamped, never trusted raw.
  - *Given* `JulesConfig.MaxConcurrentJulesSessions = 500`, *When* `cfg.MaxConcurrentJulesSessionsOrDefault()` is called, *Then* it returns `10` (`maxConcurrentJulesSessionsHardCeiling`); *and* given `0`, it returns the default `2`.

**Files**: `config/config.go`, `config/config_test.go`, `server/services/jules_dispatch_service.go`

##### Task 2.2.2a: `JulesConfig` struct and clamped accessors (~5 min)
- In `config/config.go`, add `JulesConfig` (fields per the glossary, all `omitempty`) as a field on `Config`, plus `MaxConcurrentJulesSessionsOrDefault()` and `MaxJulesSessionsPerDayOrDefault()` with `maxConcurrentJulesSessionsHardCeiling = 10` and `maxJulesSessionsPerDayHardCeiling = 300`, modeled exactly on `MaxConcurrentBacklogWorkItems` (`config/config.go:381-384`, ceiling at `:954`).
- Files: `config/config.go`

##### Task 2.2.2b: Wire the guards into `DispatchToJules` (~4 min)
- In `jules_dispatch_service.go`, add `checkSpendGuards(ctx, openSessions []session.ItemSessionBacklogEntry)` taking the already-fetched open-sessions slice (Task 2.2.1b step 2 fetches it once for the persisted-duplicate check; `checkSpendGuards` reuses it for the concurrency-ceiling count instead of calling `ListOpenJulesItemSessions` a second time) and calling `CountJulesItemSessionsSince(now-24h)` itself for the daily cap; run it after the in-flight guards (mutex + persisted-duplicate) and before the source lookup so a rejected dispatch never even lists sources.
- Log `jules dispatch rejected` with the `reason` field values enumerated in the Observability Plan.
- Files: `server/services/jules_dispatch_service.go`

##### Task 2.2.2c: Clamp and guard tests (~4 min)
- Add the clamp table test to `config/config_test.go` and the two guard cases to `jules_dispatch_service_test.go`.
- Files: `config/config_test.go`, `server/services/jules_dispatch_service_test.go`

---

#### Story 2.2.3: Data-residency opt-in, enforced read-only at dispatch

**As a** user with proprietary code checked out locally, **I want** the dispatch
path to only ever *check* whether I've consented for a repo, never *grant* that
consent itself, **so that** no future caller of `DispatchToJules` — an MCP tool,
a script, the autonomous driver "acting on my behalf" — can silently mint real
cloud-egress consent for a repo I never consciously confirmed.

**Redesign (pre-mortem P1 #3)**: the original design had `DispatchToJules`
trust and persist a plain `EgressAcknowledged` boolean from the request
unconditionally — any caller reaching the RPC could flip
`EgressAcknowledgedRepos` from absent to present with one field. `JulesDispatchRequest`
(Domain Glossary) now carries **no** `EgressAcknowledged` field at all, so there
is nothing for a caller to set even by mistake. Granting consent is split into
its own RPC, `ConfirmEgressConsent` (Story 2.4.2), callable only from
`JulesDispatchDialog`'s checkbox-confirm handler. `checkEgressConsent` here is
reduced to a pure membership check — it has no code path that can write to
`EgressAcknowledgedRepos`.

**Acceptance Criteria**:
- An unacknowledged repo is refused.
  - *Given* `JulesConfig.EgressAcknowledgedRepos = ["/home/tstapler/code/github.com/tstapler/dotfiles"]` and an item whose `RepoPath` is `/home/tstapler/code/github.com/tstapler/stapler-squad`, *When* `DispatchToJules` is called, *Then* it returns an error whose message names `tstapler/stapler-squad`, states its contents would be sent to Google's cloud VM, and directs the user to confirm via the dispatch dialog; no `CreateSession` call is made.
- `DispatchToJules` cannot itself create a new `EgressAcknowledgedRepos` entry, under any call pattern.
  - *Given* the same unacknowledged item, *When* `DispatchToJules` is called 20 times in a row (simulating repeated/retried/scripted calls), *Then* `JulesConfig.EgressAcknowledgedRepos` is unchanged across all 20 calls — asserted by reading the persisted config after each call, not just the last. `checkEgressConsent`'s implementation takes no request-shaped consent argument at all (see Task 2.2.3a), so this isn't merely "no test currently exercises the write path" — there is no write path to exercise.
- An already-acknowledged repo (acknowledged via `ConfirmEgressConsent`, never via `DispatchToJules`) proceeds without re-confirmation.
  - *Given* `/home/tstapler/code/github.com/tstapler/stapler-squad` already present in `JulesConfig.EgressAcknowledgedRepos`, *When* `DispatchToJules` is called for that item, *Then* the dispatch proceeds normally.
- `Enabled: false` refuses regardless of key or acknowledgement.
  - *Given* `JulesConfig.Enabled == false`, a valid key, and an acknowledged repo, *When* `DispatchToJules` is called, *Then* it returns an error satisfying `errors.Is(err, jules.ErrJulesNotConfigured)`.

**Files**: `server/services/jules_dispatch_service.go`, `server/services/jules_dispatch_service_test.go`

##### Task 2.2.3a: Implement the read-only egress gate (~4 min)
- Add `checkEgressConsent(item *session.BacklogItemData) error` — deliberately takes **no** request argument, so it structurally cannot read a caller-supplied consent flag — running as step 3 of `DispatchToJules`'s guard sequence (Task 2.2.1b), after the in-process mutex and the persisted-open-session check but before the spend caps and blocker gate. Internally it checks, in order: `Enabled` → key resolvable → repo in `EgressAcknowledgedRepos` (membership check only; no append, no write, no config mutation of any kind).
- Message copy must name the concrete `owner/repo` and point at the dispatch dialog's confirmation step, not an abstract feature description (pitfalls §3's "warn about proprietary repos specifically").
- Files: `server/services/jules_dispatch_service.go`

##### Task 2.2.3b: Consent tests, including the cannot-self-grant proof (~5 min)
- Add the four acceptance-criterion cases, including the 20-call no-write-path proof and a `go vet`/compile-level check that `checkEgressConsent`'s signature has no boolean or `EgressAcknowledged`-shaped parameter (a signature test, so a future edit re-adding one is caught at review, not just at runtime).
- Files: `server/services/jules_dispatch_service_test.go`

---

### Epic 2.3: `JulesSessionPoller`

**Goal**: Convert Jules' remote state into backlog state, fail soft, and never
leave an item stuck in `in_progress` forever.

#### Story 2.3.1: The poll loop

**As a** server, **I want** a dedicated poller that ticks every 60s over open
`jules_work` sessions, **so that** Jules status reaches the backlog without a
webhook (which Jules does not offer) and without coupling to local-session
reconciliation.

**Acceptance Criteria**:
- One tick polls each open session exactly once.
  - *Given* three open `jules_work` `ItemSession`s, *When* one tick runs, *Then* the fake client records exactly three `GetSession` calls, one per `JulesSessionName`.
- A rate-limited client skips the whole tick rather than firing doomed calls.
  - *Given* a client whose `IsLimited()` returns `true`, *When* a tick runs, *Then* zero `GetSession` calls are made and one `jules poll tick` log line records `skipped_rate_limited`.
- One failing session does not abort the others.
  - *Given* three open sessions where the second's `GetSession` returns `ErrJulesTransient`, *When* a tick runs, *Then* all three are attempted, the first and third are applied normally, and one `jules poll failed` line is logged at `Warn` (mirroring `WorktreePRPoller.handleFetchError`, `session/worktree_pr_poller.go:327`).
- `Start` is cancellable and idempotent.
  - *Given* a started poller, *When* its context is cancelled, *Then* the goroutine returns within one tick interval and a second `Start` on the same instance is a no-op.

**Files**: `session/jules_session_poller.go`, `session/jules_session_poller_test.go`

##### Task 2.3.1a: Config struct and defaults (~4 min)
- Create `session/jules_session_poller.go` with `JulesSessionPollerConfig{PollInterval, CallTimeout, MaxSessionAge, NoChangeBackoff}` and `DefaultJulesSessionPollerConfig()` returning 60s / 20s / 24h / 5m, mirroring `WorktreePRPollerConfig` (`session/worktree_pr_poller.go:37-55`).
- Add `NewJulesSessionPoller(client julesStatusClient, storage julesPollerStorage, cfg JulesSessionPollerConfig)` where both dependencies are **narrow, locally-declared interfaces** (`GetSession`; `ListOpenJulesItemSessions`/`TouchItemSessionProgress`/`SetBacklogItemPRAndTransition`/`UpdateItemSessionEndedWithReason`/`AppendProgressNote`) so tests need no real client or DB.
- Declare `julesSessionUUIDPrefix = "jules-"` and `julesPendingUUIDPrefix = "jules-pending-"` as local constants in this file, byte-identical to the copies declared in `server/services/jules_dispatch_service.go` (Task 2.2.1a). **Do not import them** — `session/` cannot import `server/services` (import-direction constraint). This is the same duplication the codebase already uses for `headlessTriageUUIDPrefix`/`headlessTriageSessionUUIDPrefix` (`server/services/backlog_service_triage.go:423`, `session/backlog_lifecycle_triage.go:43-50`); copy that file's comment style to explain why the constant is redeclared here rather than imported, and point at the other file's copy so a future value change is not made in only one place.
- Files: `session/jules_session_poller.go`

##### Task 2.3.1b: `Start` and `tick` (~5 min)
- Implement `Start(ctx)` with a `time.Ticker` and a `sync.Once` guard, and `tick(ctx)` that early-returns when `IsLimited()`, then iterates open sessions applying `applyJulesState` per row under a `CallTimeout`-scoped child context.
- Every per-session error is logged and swallowed — the loop never returns early (fail soft, pitfalls §1).
- Files: `session/jules_session_poller.go`

##### Task 2.3.1c: Poller loop tests (~5 min)
- Create `session/jules_session_poller_test.go` with fakes for both narrow interfaces; cover the four criteria, driving ticks directly via the exported-for-test `tick` rather than sleeping.
- Files: `session/jules_session_poller_test.go`

---

#### Story 2.3.2: `applyJulesState` — the one exhaustive state mapper

**As a** maintainer, **I want** every `JulesSessionState` mapped to a backlog
effect in exactly one function with an exhaustiveness test, **so that** a new
alpha-API enum value produces a loud, findable failure instead of an item stuck
in `in_progress` forever.

**Acceptance Criteria**:
- Non-terminal states touch progress only.
  - *Given* an open `jules_work` `ItemSession` for `item-1` (status `in_progress`) and `GetSession` returning `state: "IN_PROGRESS"`, *When* `applyJulesState` runs, *Then* `TouchItemSessionProgress` is called with the tick time, and neither `SetBacklogItemPRAndTransition` nor any status transition occurs.
- A state *change* is recorded as a visible note, an unchanged state is not.
  - *Given* a session last seen `QUEUED`, *When* the poll returns `PLANNING`, *Then* one `AppendProgressNote(item-1, -1, "Jules session is now planning.", "in_progress")` is written; *and* when the next poll also returns `PLANNING`, *Then* no additional note is written.
- `COMPLETED` with a PR records the PR and hands off to the existing review path.
  - *Given* `GetSession` returns `state:"COMPLETED"` with `outputs[0].pullRequest.url = "https://github.com/tstapler/stapler-squad/pull/700"`, *When* `applyJulesState` runs, *Then* `SetBacklogItemPRAndTransition(ctx, item, "https://github.com/tstapler/stapler-squad/pull/700", 700, <summary>, nil)` is called once, the `ItemSession` is ended with reason `"jules_completed"`, and no Jules-specific merge polling is added (`ReconcilePRPending` takes over unmodified).
- `COMPLETED` with **no** PR is surfaced, not silently treated as success.
  - *Given* `state:"COMPLETED"` with `outputs: []`, *When* `applyJulesState` runs, *Then* the item does **not** move to `pr_pending`, the session ends with reason `"jules_completed_no_pr"`, and a progress note is appended telling the user to check the session at its Jules web URL.
- An unknown state is loud.
  - *Given* `state:"AWAITING_HUMAN_TEA_BREAK"`, *When* `applyJulesState` runs, *Then* it logs `jules unknown session state` at `Error` with `raw_state`, touches progress (so the session is not considered stale), and makes no transition.
- Exhaustiveness is enforced.
  - *Given* the set of exported `JulesSessionState` constants, *When* `TestApplyJulesState_should_HandleEveryDeclaredState_When_StatesEnumerated` runs, *Then* it asserts a non-default effect for each and fails if a new constant is added without a table row.

**Files**: `session/jules_session_poller.go`, `session/jules_session_poller_test.go`

##### Task 2.3.2a: Implement `applyJulesState` (~5 min)
- Add `applyJulesState(ctx, entry ItemSessionBacklogEntry, s jules.JulesSession) error` with an explicit `switch` over the sum type, no `default` that silently succeeds — `JulesStateUnknown` gets its own named branch.
- Add `ParsePRNumberFromURL(url string) (int, error)` (or reuse an existing helper if `session/` already has one — check `session/backlog_lifecycle_pr.go` first) for the `SetBacklogItemPRAndTransition` call's `prNumber`.
- Files: `session/jules_session_poller.go`

##### Task 2.3.2b: State-change note deduplication (~4 min)
- Track last-seen state per `JulesSessionName` in an in-poller `map[string]jules.JulesSessionState` guarded by a mutex; only append a progress note when the state differs from the last observation. On process restart the map is empty, so at most one duplicate note per session — acceptable, and preferable to a schema column.
- Files: `session/jules_session_poller.go`

##### Task 2.3.2c: Mapper tests incl. the exhaustiveness guard (~5 min)
- Add all six acceptance-criterion cases plus the exhaustiveness test to `session/jules_session_poller_test.go`.
- Files: `session/jules_session_poller_test.go`

---

#### Story 2.3.3: Failure, staleness, and abandoned-reservation reconciliation

**As a** user, **I want** a failed, vanished, or never-confirmed Jules session to
end in a visible, fixable state, **so that** no backlog item sits in
`in_progress` forever waiting on a cloud job that will never report back.

**Acceptance Criteria**:
- `FAILED` ends the session and returns the item to a fixable state with Jules' own message attributed.
  - *Given* `state:"FAILED"` on `item-1` (currently `in_progress`) and a session `url` of `https://jules.google.com/session/xyz`, *When* `applyJulesState` runs, *Then* the `ItemSession` ends with reason `"jules_failed"`, the item transitions `in_progress → ready`, and a progress note is appended containing both Jules' own failure text and the `https://jules.google.com/session/xyz` link.
- A vanished session does not retry forever.
  - *Given* `GetSession` returns `ErrJulesSessionNotFound` for an open row, *When* the tick runs, *Then* the `ItemSession` ends with reason `"jules_session_missing"`, the item returns to `ready`, and a note explains the session is no longer visible in Jules.
- A session exceeding `MaxSessionAge` is failed rather than polled indefinitely.
  - *Given* an open `jules_work` `ItemSession` with `started_at` 25 hours ago and `MaxSessionAge = 24h`, *When* the tick runs, *Then* it ends with reason `"jules_timed_out"`, the item returns to `ready`, and no further `GetSession` call is made for it on the next tick.
- An abandoned reservation is cleaned up.
  - *Given* an `ItemSession` whose `session_uuid` still begins `jules-pending-` and whose `created_at` is 15 minutes ago, *When* the tick runs, *Then* it ends with reason `"dispatch_incomplete"`, the item returns to `ready`, and a note tells the user to check jules.google.com in case a session was created but not recorded.

**Files**: `session/jules_session_poller.go`, `session/jules_session_poller_test.go`

##### Task 2.3.3a: `FAILED` and not-found handling (~5 min)
- Add the `JulesStateFailed` branch and an `errors.Is(err, jules.ErrJulesSessionNotFound)` branch in `tick`, both routing through a shared `failJulesSession(ctx, entry, reason, note string)` helper that ends the session, transitions the item back to `ready` via the storage seam, and appends the note.
- Files: `session/jules_session_poller.go`

##### Task 2.3.3b: Age and reservation sweeps (~5 min)
- At the top of each per-row iteration, before calling `GetSession`: if `session_uuid` has the `julesPendingUUIDPrefix` and `created_at` is older than 10 minutes → `failJulesSession(..., "dispatch_incomplete", ...)`; if `started_at` is older than `MaxSessionAge` → `failJulesSession(..., "jules_timed_out", ...)`.
- Note in a one-line comment why Jules gets its own age model rather than reusing `session/backlog_lifecycle_stale.go`: that sweep is tuned to local tmux processes going quiet, and would misfire on a healthy long-running cloud task (pitfalls §4).
- Files: `session/jules_session_poller.go`

##### Task 2.3.3c: Reconciliation tests (~5 min)
- Add the four acceptance-criterion cases to `session/jules_session_poller_test.go`, using an injected clock for the two age-based ones.
- Files: `session/jules_session_poller_test.go`

---

#### Story 2.3.4: Reconnect-required — auth failure mid-poll, and automatic recovery

**As a** user whose Jules API key was revoked or expired after sessions were
already dispatched, **I want** the poller to recognize that distinctly from a
task failure or a network blip, and to resume automatically once I fix my key,
**so that** I'm told to reconnect rather than made to think my work failed, and
I don't have to click anything extra once I've fixed the key.

**Context**: `classifyJulesResponse` (Task 1.1.3a) already maps a 401/403 to
`ErrJulesNotConfigured`, but that sentinel is otherwise used for "no key
resolvable at all" (feature off, `server/dependencies.go`'s startup gate, Task
2.4.4a). Until now the poller had no branch for seeing that same sentinel
*mid-tick*, from a session that was dispatched successfully and is actively
being polled — it silently fell into the generic logged-and-swallowed path
(Task 2.3.1b), which is indistinguishable from a transient network blip and
never resolves the item, leaving the badge showing stale "retrying…" forever
even after the user fixes their key. This story adds that missing branch. The
underlying cause (an invalid/expired key) is account-wide, not
session-specific — every open session hits the same 401/403 in the same
tick — so the signal is tracked once, process-level, on the poller, not
per-`ItemSession` (avoids an ent schema change entirely: no new column, no
migration).

**Acceptance Criteria**:
- A 401/403 during a poll tick is distinguished from staleness and from a task failure.
  - *Given* an open `jules_work` `ItemSession` whose `GetSession` call returns an error satisfying `errors.Is(err, jules.ErrJulesNotConfigured)`, *When* the tick processes that session, *Then* the poller does **not** end the session, does **not** transition the item, and does **not** call `TouchItemSessionProgress` for it — it sets `JulesSessionPoller.AuthReconnectRequired() == true` instead and moves on to the next session.
- The condition is visible exactly once per occurrence, not once per tick.
  - *Given* three consecutive ticks all returning `ErrJulesNotConfigured` for the same open session, *When* all three run, *Then* exactly one `AppendProgressNote(itemID, -1, "Jules session needs reauthentication — update your API key in Settings.", "blocked")` is written for that item (dedup mirrors Task 2.3.2b's per-session state-change map, keyed per item here since the underlying cause is account-wide).
- Recovery is automatic on the next successful poll — no manual retry action.
  - *Given* `AuthReconnectRequired() == true` and a subsequent tick whose `GetSession` call for **any** open session succeeds (200 OK), *When* that tick completes, *Then* `AuthReconnectRequired()` returns `false` again, one `AppendProgressNote(itemID, -1, "Jules reconnected — resuming normal polling.", "in_progress")` is written for each item that had the blocked note outstanding, and every open session resumes ordinary `applyJulesState` handling starting the very next tick, with no user action beyond having saved a working key.
- The flag is process-level, not per-session.
  - *Given* two open `jules_work` sessions on different items, both returning `ErrJulesNotConfigured` in the same tick, *When* the tick completes, *Then* `AuthReconnectRequired()` is `true` exactly once (not per-session), and both items receive their own dedup'd progress note.
- Every other error path is unaffected.
  - *Given* an open session whose `GetSession` returns `ErrJulesTransient` (a 5xx) or `ErrJulesSessionNotFound`, *When* the tick runs, *Then* the existing staleness/failure handling (Task 2.3.1b / Story 2.3.3) applies unchanged and `AuthReconnectRequired()` is not touched.

**Files**: `session/jules_session_poller.go`, `session/jules_session_poller_test.go`

##### Task 2.3.4a: `AuthReconnectRequired` state and the tick branch (~5 min)
- On `JulesSessionPoller`, add `authReconnectRequired atomic.Bool` and exported `AuthReconnectRequired() bool`, plus an unexported per-item dedup set (`map[string]bool`, mutex-guarded, mirroring Task 2.3.2b's shape) tracking which items currently have the "needs reauthentication" note outstanding.
- In `tick`, ahead of the existing `ErrJulesSessionNotFound`/age/reservation branches: if a session's `GetSession` error satisfies `errors.Is(err, jules.ErrJulesNotConfigured)`, set `authReconnectRequired.Store(true)`, append the dedup'd note for that item (skip if already outstanding), log `jules session auth invalid` once per item at `Warn`, and `continue` to the next session without touching progress/ending/transitioning.
- On any tick where at least one `GetSession` call succeeds (200 OK) while `authReconnectRequired` was `true`: `Store(false)`, append the recovery note for every item whose dedup entry is still outstanding, clear those entries, and log `jules session auth restored` at `Info`.
- Files: `session/jules_session_poller.go`

##### Task 2.3.4b: Tests (~5 min)
- Add the five acceptance-criterion cases to `session/jules_session_poller_test.go`, using fakes that return `ErrJulesNotConfigured` then a success across two `tick()` calls to drive the recovery path.
- Files: `session/jules_session_poller_test.go`

---

### Epic 2.4: Proto, RPC, and server wiring

**Goal**: Expose configuration and dispatch over ConnectRPC and start the poller.

#### Story 2.4.1: Jules configuration RPCs

**As a** user, **I want** to store my Jules API key and see whether it works,
**so that** I can set the feature up entirely from the UI without editing files.

**Acceptance Criteria**:
- The key is never returned to the client.
  - *Given* a stored key `"AIzaSyD-EXAMPLE"`, *When* `GetJulesConfig` is called, *Then* the response has `has_api_key == true` and contains no field holding the key or any prefix of it.
- Updating the key writes the keychain, not `config.json`.
  - *Given* `UpdateJulesConfig{api_key:"AIzaSyD-NEW", enabled:true}`, *When* it is handled, *Then* `KeyringTokenSource` holds `"AIzaSyD-NEW"`, `config.json`'s serialized bytes contain no occurrence of `"AIzaSyD-NEW"`, and `JulesConfig.Enabled == true`.
- The connection test reports the concrete prerequisite when a repo is not connected.
  - *Given* a valid key whose `ListSources` returns only `sources/github-tstapler-dotfiles`, *When* `TestJulesConnection{repo_path:"/home/tstapler/code/github.com/tstapler/stapler-squad"}` is called, *Then* the response is `ok:false` with `message` naming `tstapler/stapler-squad` and instructing the user to connect it at jules.google.com.
- The reconnect-required flag is surfaced live, without a separate poll.
  - *Given* `deps.JulesSessionPoller.AuthReconnectRequired() == true` (Story 2.3.4), *When* `GetJulesConfig` is called, *Then* the response's `auth_reconnect_required` field is `true`; *and* once the poller's flag clears on its own next successful tick, *Then* the next `GetJulesConfig` call returns `false`. When `deps.JulesSessionPoller` is `nil` (feature off / not started), the field is always `false`.

**Files**: `proto/session/v1/session.proto`, `server/services/jules_config_service.go`, `server/services/jules_config_service_test.go`

##### Task 2.4.1a: Proto messages and RPCs (~5 min)
- In `proto/session/v1/session.proto`, beside the Slack config RPCs (`:277-288`), add `GetJulesConfig`, `UpdateJulesConfig`, `TestJulesConnection` with request/response messages; `JulesConfigProto` carries `enabled`, `has_api_key`, `egress_acknowledged_repos`, `max_concurrent_jules_sessions`, `max_jules_sessions_per_day`, `auth_reconnect_required` (bool, read live from `deps.JulesSessionPoller.AuthReconnectRequired()`, nil-safe to `false` — not persisted config; see `JulesAuthReconnectRequired` glossary entry and Story 2.3.4), and the `JulesUsageCounter` totals — **never** the key.
- Run `make proto-gen`.
- Files: `proto/session/v1/session.proto`

##### Task 2.4.1b: Handler implementation (~5 min)
- Create `server/services/jules_config_service.go` modeled on `server/services/slack_config_service.go` (Get/Update/Test triple, `julesConfigToProto` helper), with `// +api: jules:get-config` / `jules:update-config` / `jules:test-connection` markers.
- `UpdateJulesConfig` routes `api_key` to `jules.KeyringTokenSource.SetJulesAPIKey` and everything else to `config.Config`; an empty `api_key` means "leave unchanged", a sentinel `"__clear__"` means delete.
- `julesConfigToProto` reads `auth_reconnect_required` from the poller dependency at response-build time, not from `config.Config` — it is process/runtime state, never persisted.
- Files: `server/services/jules_config_service.go`

##### Task 2.4.1c: Config-service tests (~4 min)
- Create `server/services/jules_config_service_test.go` covering the three criteria, including asserting the marshalled `config.json` bytes do not contain the key.
- Files: `server/services/jules_config_service_test.go`

---

#### Story 2.4.2: `ConfirmEgressConsent` RPC — the only path that can grant Jules egress consent

**As a** user, **I want** a single, narrow RPC that only `JulesDispatchDialog`'s
checkbox-confirm handler calls, **so that** "this repo's code may leave my
machine for Google's cloud VM" is always a deliberate, UI-originated action —
never a side effect of calling `DispatchToJules` (pre-mortem P1 #3, and the
counterpart to Story 2.2.3's read-only `checkEgressConsent`).

**Acceptance Criteria**:
- Confirming appends and persists the repo.
  - *Given* `JulesConfig.EgressAcknowledgedRepos` does not contain `/home/tstapler/code/github.com/tstapler/stapler-squad`, *When* `ConfirmEgressConsent{repo_path:"/home/tstapler/code/github.com/tstapler/stapler-squad"}` is called, *Then* the repo is appended to `EgressAcknowledgedRepos`, the change is persisted to `config.json`, and the response echoes the updated list.
- Confirming is idempotent.
  - *Given* the same repo already present, *When* `ConfirmEgressConsent` is called again for it, *Then* the list gains no duplicate entry and the call still succeeds.
- `DispatchToJules` cannot reach this write path.
  - *Given* the repo of Story 2.2.3's grep/signature check, *When* `server/services/jules_dispatch_service.go` is searched for any call to the config-mutation function backing `ConfirmEgressConsent`, *Then* there are zero occurrences outside `jules_config_service.go` itself — the dispatch service has no way to invoke it.
- A malformed repo path is rejected, not silently accepted into the allowlist.
  - *Given* `repo_path: ""`, *When* `ConfirmEgressConsent` is called, *Then* it returns `connect.CodeInvalidArgument` and `EgressAcknowledgedRepos` is unchanged.

**Files**: `proto/session/v1/session.proto`, `server/services/jules_config_service.go`, `server/services/jules_config_service_test.go`

##### Task 2.4.2a: Proto RPC (~3 min)
- In `proto/session/v1/session.proto`, beside `GetJulesConfig`/`UpdateJulesConfig`/`TestJulesConnection` (Task 2.4.1a), add `rpc ConfirmEgressConsent(ConfirmEgressConsentRequest) returns (ConfirmEgressConsentResponse) {}` with `ConfirmEgressConsentRequest{string repo_path}` and `ConfirmEgressConsentResponse{repeated string egress_acknowledged_repos}`.
- Run `make proto-gen`.
- Files: `proto/session/v1/session.proto`

##### Task 2.4.2b: Handler implementation (~4 min)
- In `server/services/jules_config_service.go` (Task 2.4.1b), add `ConfirmEgressConsent`: validate `repo_path` non-empty, append to `JulesConfig.EgressAcknowledgedRepos` under the existing `cfgMu` write lock if not already present, persist via the same config-save path `UpdateJulesConfig` uses, with `// +api: jules:confirm-egress-consent`.
- This is the **only** function in the codebase that appends to `EgressAcknowledgedRepos` — `checkEgressConsent` in `jules_dispatch_service.go` (Story 2.2.3) only reads it.
- Files: `server/services/jules_config_service.go`

##### Task 2.4.2c: Tests (~4 min)
- Add the four acceptance-criterion cases to `server/services/jules_config_service_test.go`, including the grep/signature-scan proof that `jules_dispatch_service.go` never calls the mutation path.
- Files: `server/services/jules_config_service_test.go`

---

#### Story 2.4.3: `DispatchToJules` RPC

**As a** user, **I want** a single RPC that dispatches an item to Jules, **so
that** the web UI (and later an MCP tool) has one guarded entry point.

**Acceptance Criteria**:
- A valid request dispatches and returns the created `ItemSession`.
  - *Given* item `item-1` in `ready`, its repo already present in `EgressAcknowledgedRepos` (via a prior `ConfirmEgressConsent` call — Story 2.4.2), and a valid key, *When* `DispatchToJules{item_id:"item-1", branch:"backlog/item-1", prompt:"Fix the flaky poller test"}` is called, *Then* the response carries an `ItemSession` with `role == "jules_work"` and `session_uuid` starting `"jules-sessions/"`, and the item's status is `in_progress`.
- A missing branch is rejected with a precise message, since MVP never pushes one.
  - *Given* the same item and `branch:""`, *When* the RPC is called, *Then* it returns `connect.CodeInvalidArgument` with a message stating Jules can only start from a branch already pushed to GitHub.
- Guard rejections map to `FailedPrecondition`, not `Internal`.
  - *Given* the concurrency ceiling is already reached, *When* the RPC is called, *Then* it returns `connect.CodeFailedPrecondition` (so the UI can show the reason inline rather than a generic error toast).
- The service is unavailable, not broken, when Jules is unconfigured.
  - *Given* `JulesConfig.Enabled == false`, *When* the RPC is called, *Then* it returns `connect.CodeFailedPrecondition` with a message pointing at Settings → Jules.
- An unacknowledged repo is rejected by the RPC the same way the service rejects it.
  - *Given* the item's repo is **not** in `EgressAcknowledgedRepos`, *When* the RPC is called, *Then* it returns `connect.CodeFailedPrecondition` with a message directing the user to confirm cloud egress in the dispatch dialog — there is no request field the caller could set instead to bypass this (pre-mortem P1 #3).

**Files**: `proto/session/v1/backlog.proto`, `server/services/jules_dispatch_service.go`, `server/services/jules_dispatch_service_test.go`

##### Task 2.4.3a: Proto RPC on `BacklogService` (~4 min)
- In `proto/session/v1/backlog.proto`, add `rpc DispatchToJules(DispatchToJulesRequest) returns (DispatchToJulesResponse) {}` to `service BacklogService` (`:894`), beside `TriggerShipPR` (`:967`), with `DispatchToJulesRequest{item_id, branch, prompt}` (**no `egress_acknowledged` field** — pre-mortem P1 #3, mirrors `JulesDispatchRequest`'s glossary entry) and `DispatchToJulesResponse{ItemSession item_session}` (matching `TriggerTriageResponse`'s shape, `:558`).
- Run `make proto-gen`.
- Files: `proto/session/v1/backlog.proto`

##### Task 2.4.3b: Connect handler (~5 min)
- Add the `DispatchToJules` Connect handler to `server/services/jules_dispatch_service.go` (or a thin `BacklogService` method delegating to it, matching how `TriggerShipPR` sits in `backlog_service_ship.go`), with a `// +api: backlog:dispatch-to-jules` marker and the guard-clause style of `TriggerShipPR` (`server/services/backlog_service_ship.go:64-89`).
- Map sentinels to Connect codes: `ErrJulesNotConfigured`/`ErrJulesSourceNotRegistered`/`ErrJulesDispatchInFlight`/cap rejections/unacknowledged-repo/`session.ErrUnresolvedBlockers` (Task 2.2.1b's blocker gate) → `CodeFailedPrecondition`; validation → `CodeInvalidArgument`; transient → `CodeUnavailable`.
- Files: `server/services/jules_dispatch_service.go`

##### Task 2.4.3c: RPC-level tests (~4 min)
- Add the five acceptance-criterion cases to `server/services/jules_dispatch_service_test.go`, asserting the Connect code, not just the error text.
- Files: `server/services/jules_dispatch_service_test.go`

---

#### Story 2.4.4: Dependency wiring and poller startup

**As a** server operator, **I want** the Jules client, dispatch service, and
poller constructed and started alongside the existing pollers, **so that** the
feature is live without any manual step beyond configuration.

**Acceptance Criteria**:
- The poller starts only when Jules is enabled.
  - *Given* `JulesConfig.Enabled == false` at startup, *When* the server starts, *Then* no `jules poll tick` line is ever logged and `deps.JulesSessionPoller` is nil.
  - *Given* `JulesConfig.Enabled == true` and a resolvable key, *When* the server starts, *Then* `"JulesSessionPoller started"` is logged beside the existing `"WorktreePRPoller started"` (`server/server.go:232-234`).
- Construction failures degrade the feature, not the server.
  - *Given* the keychain is unreadable at startup, *When* the server starts, *Then* it starts normally, logs `jules disabled` once at `Info`, and every other subsystem is unaffected.

**Files**: `server/dependencies.go`, `server/server.go`, `server/server_test.go`

##### Task 2.4.4a: Build the dependencies (~5 min)
- In `server/dependencies.go`, near where `worktreePRPoller` is built (`:1517`) and `syncRegistry`/`backlogCtrl` are wired (`:1133-1140`), construct `jules.NewClient(...)`, `jules.NewJulesSourceRegistry(...)`, `NewJulesDispatchService(storage, backlogSvc, julesClient, sourceRegistry, cfg, counters)`, and `session.NewJulesSessionPoller(...)`; leave all four nil when `JulesConfig.Enabled` is false or the key is unresolvable, logging `jules disabled` once. `backlogSvc` (built at `:1191`, well before this point) is passed directly as the `julesTransitionGuard` argument — it satisfies that interface structurally (Task 2.2.1a), so no additional wiring or setter call is needed.
- Files: `server/dependencies.go`

##### Task 2.4.4b: Start the poller and register the services (~4 min)
- In `server/server.go`, after the `WorktreePRPoller` start block (`:232-234`), add the nil-guarded `deps.JulesSessionPoller.Start(serverCtx)` + log line; register `JulesConfigService`'s handlers where the other Connect services are registered.
- Files: `server/server.go`

---

## Phase 3: Frontend

### Epic 3.1: Settings

#### Story 3.1.1: Jules settings panel

**As a** user, **I want** a Settings page to enter my Jules API key, enable the
feature, review which repos I've allowed cloud egress for, and see my usage
counts, **so that** setup and spend visibility live in one place.

**Acceptance Criteria**:
- The key input is write-only and masked.
  - *Given* a stored key, *When* the Jules settings panel loads, *Then* the key field renders empty with placeholder `Key stored — enter a new key to replace it`, has `type="password"`, and the DOM contains no key characters.
- The connection test surfaces the actionable prerequisite.
  - *Given* a key whose account has not connected `tstapler/stapler-squad`, *When* the user clicks `Test connection` with that repo selected, *Then* an inline message reads `tstapler/stapler-squad is not connected to Jules. Connect it at jules.google.com, then test again.` inside a `role="status"` region.
- Acknowledged repos are listed and revocable.
  - *Given* `EgressAcknowledgedRepos = ["/home/tstapler/code/github.com/tstapler/stapler-squad"]`, *When* the panel renders, *Then* the repo is listed with a `Revoke` button whose `aria-label` is `Revoke cloud-egress consent for tstapler/stapler-squad`, and clicking it calls `UpdateJulesConfig` with that repo removed.
- Usage counts are visible.
  - *Given* counters `dispatched=7, completed=5, failed=2`, *When* the panel renders, *Then* it shows `7 dispatched · 5 completed · 2 failed` as text, not only as a chart.

**Files**: `web-app/src/components/settings/JulesSettings.tsx`, `web-app/src/components/settings/JulesSettings.css.ts`, `web-app/src/app/settings/jules/page.tsx`, `web-app/src/components/settings/JulesSettings.test.tsx`

##### Task 3.1.1a: Panel component (~5 min)
- Create `JulesSettings.tsx` modeled on `web-app/src/components/settings/SlackNotificationSettings.tsx`'s structure (form + save + test action), with the masked key field, enable toggle, cap inputs, acknowledged-repo list, and counter line.
- Add `// +feature: jules-settings` in the first 10 lines.
- Files: `web-app/src/components/settings/JulesSettings.tsx`

##### Task 3.1.1b: Styles and route (~4 min)
- Create `JulesSettings.css.ts` using existing vanilla-extract tokens only (`vars.color.*`) — no one-off hex values, per `docs/reference/css-architecture.md` and ux.md §3.
- Create `web-app/src/app/settings/jules/page.tsx` following `web-app/src/app/settings/defaults/page.tsx`'s shape, and add the nav entry wherever the settings pages are listed.
- Files: `web-app/src/components/settings/JulesSettings.css.ts`, `web-app/src/app/settings/jules/page.tsx`

##### Task 3.1.1c: Panel tests (~5 min)
- Create `JulesSettings.test.tsx` covering all four criteria, asserting on `data-testid`/ARIA roles only (never CSS classes).
- Files: `web-app/src/components/settings/JulesSettings.test.tsx`

---

### Epic 3.2: Dispatch UX

#### Story 3.2.1: `JulesDispatchDialog` with first-use egress confirmation

**As a** user, **I want** a dialog that asks for the branch and prompt and, the
first time for a given repo, makes me confirm that this repo's code will be sent
to Google, **so that** cloud egress is a deliberate choice, never a side effect.

**Acceptance Criteria**:
- The branch field is pre-filled from the item's own tracked branch, never opened blank when one is known.
  - *Given* an item whose most recently created `ItemSession` has `worktree_branch == "backlog/fix-flaky-poller-test"` (the same field `SessionsSection.tsx`'s branch badge already reads, populated from `GitWorktreeData.BranchName` via `GetWorktreeDataBySessionUUID` — `server/services/backlog_service_query.go:83-93`), *When* the dialog opens, *Then* the Branch field's initial value is `"backlog/fix-flaky-poller-test"`, focusable and editable before submit — this is a starting value, not a locked field, since the user may want to dispatch a different already-pushed branch.
- The confirmation names the concrete repo.
  - *Given* an item whose `RepoPath` is `/home/tstapler/code/github.com/tstapler/stapler-squad` and which is **not** in `EgressAcknowledgedRepos`, *When* the dialog opens, *Then* it shows `The contents of tstapler/stapler-squad will be sent to Google's cloud VM to run this session.` and the `Dispatch` button is disabled until the confirmation checkbox is checked.
- An already-acknowledged repo does not re-prompt.
  - *Given* the same repo present in `EgressAcknowledgedRepos`, *When* the dialog opens, *Then* the confirmation block is absent and `Dispatch` is enabled as soon as a branch and prompt are entered.
- The branch requirement is explained, not just enforced.
  - *Given* the dialog with an empty branch field, *When* the user focuses it, *Then* the helper text reads `Jules starts from a branch already pushed to GitHub — local-only branches won't work.` and `Dispatch` stays disabled.
- Focus is trapped and returned, matching the modal convention already enforced in this repo.
  - *Given* the dialog is open, *When* the user presses `Tab` past the last control, *Then* focus wraps to the first control; *and* when the dialog closes, focus returns to the `Dispatch to Jules` button that opened it (same guarantee as `BacklogItemDetail.focusReturn.test.tsx`).

**Files**: `web-app/src/components/backlog/JulesDispatchDialog.tsx`, `web-app/src/components/backlog/JulesDispatchDialog.css.ts`, `web-app/src/components/backlog/JulesDispatchDialog.test.tsx`

##### Task 3.2.1a: Dialog component (~6 min)
- Create `JulesDispatchDialog.tsx` with the branch field, prompt textarea (prefilled from the item's title + acceptance criteria the same way the spawn flow builds a prompt), and the conditional egress block.
- Branch field initial value: the `worktree_branch` of the item's most recently created `ItemSession` that has a non-empty one (already present on each entry in the `sessions` array the item detail page loads — no new RPC field). This is the same signal `SessionsSection.tsx`'s branch badge reads (`:253-255`); the dialog just takes the newest non-empty one rather than rendering it per-row. There is deliberately no attempt to verify the branch was actually **pushed** — the repo has no such signal (`git.BranchAheadBehind`, `session/git/ops.go:203`, only resolves local refs) — so the standing helper text under the field ("Jules starts from a branch already pushed to GitHub — local-only branches won't work.") stays the mitigation, and a rejection is handled the same as any other dispatch-time server error (§3.4/Story 2.4.3's `CodeInvalidArgument`/`CodeFailedPrecondition` mapping). Story 3.2.2's gating (below) is what prevents the dialog from ever opening with a genuinely blank field.
- Submit handler (pre-mortem P1 #3): if the confirmation block was shown (repo not yet acknowledged), first call `ConfirmEgressConsent{repo_path}` (Story 2.4.2) and only on its success proceed to call `DispatchToJules` (Story 2.4.3) — which no longer has an `egress_acknowledged` field to set. If `ConfirmEgressConsent` fails, surface its error inline and do not call `DispatchToJules`. If the repo was already acknowledged, skip straight to `DispatchToJules`. Both calls go through `useBacklogService`.
- Add `// +feature: jules-dispatch` in the first 10 lines.
- Files: `web-app/src/components/backlog/JulesDispatchDialog.tsx`

##### Task 3.2.1b: Focus trap and styles (~4 min)
- Reuse the existing modal focus-trap utility used by the backlog modals (see `BacklogItemDetail.focusReturn.test.tsx` for which one) rather than writing a new trap; add `JulesDispatchDialog.css.ts` with token-only colors.
- Files: `web-app/src/components/backlog/JulesDispatchDialog.tsx`, `web-app/src/components/backlog/JulesDispatchDialog.css.ts`

##### Task 3.2.1c: Dialog tests (~5 min)
- Create `JulesDispatchDialog.test.tsx` covering all four criteria, including the tab-wrap and focus-return assertions.
- Files: `web-app/src/components/backlog/JulesDispatchDialog.test.tsx`

---

#### Story 3.2.2: Gated `Dispatch to Jules` action on the item detail page

**As a** user, **I want** the Jules action to appear only when it can actually
work, and to explain itself when it cannot, **so that** I never start something
that immediately fails.

**Acceptance Criteria**:
- Hidden when the feature is off.
  - *Given* `GetJulesConfig` returns `enabled:false`, *When* a `ready` item's detail page renders, *Then* no element with `data-testid="dispatch-to-jules"` exists.
- Shown but disabled with a reason when enabled without a key.
  - *Given* `enabled:true, has_api_key:false`, *When* the page renders, *Then* the button exists, is `disabled`, and its accessible description reads `Add a Jules API key in Settings to enable cloud sessions.`
- Disabled while a Jules session is already open for that item.
  - *Given* the item has an `ItemSession` with `role === "jules_work"` and no `endedAt`, *When* the page renders, *Then* the button is disabled with the description `A Jules session is already running for this item.`
- Disabled when the item has no branch to dispatch.
  - *Given* `enabled:true, has_api_key:true`, item status `ready`, no open Jules session, and **zero** `ItemSession` rows carrying a non-empty `worktree_branch`, *When* the page renders, *Then* the button is disabled with the description `This item has no branch yet — spawn a local session (or push a branch) before dispatching to Jules.`
- Enabled for a `ready` item with everything configured.
  - *Given* `enabled:true, has_api_key:true`, item status `ready`, no open Jules session, and at least one `ItemSession` with a non-empty `worktree_branch`, *When* the page renders, *Then* the button is enabled and clicking it opens `JulesDispatchDialog` pre-filled with that branch.
- Gating precedence is deterministic when more than one condition applies.
  - *Given* `enabled:true, has_api_key:false` on an item that also has no known branch, *When* the page renders, *Then* the button shows only the "Add a Jules API key…" description (key check precedes the branch check) — never two reasons at once. Full order: feature off (hidden) → no key (disabled) → Jules session already open (disabled) → no known branch (disabled) → enabled.

**Files**: `web-app/src/components/backlog/detail/ActionsSection.tsx`, `web-app/src/components/backlog/BacklogItemDetail.tsx`, `web-app/src/components/backlog/detail/ActionsSection.jules.test.tsx`

##### Task 3.2.2a: Add the gated button (~5 min)
- In `ActionsSection.tsx`, add the `Dispatch to Jules` button beside the existing status-conditional actions, taking its gating state as props (props-down/callbacks-up, per the component's own Story 3.1.4 note) so `ActionsSection` stays free of data fetching.
- In `BacklogItemDetail.tsx`, resolve the gating state — Jules config, open-Jules-session lookup, and "most recent non-empty `worktree_branch`" — all over the already-loaded item sessions (no new RPC field), apply the precedence order above, and render `JulesDispatchDialog` on click, passing the resolved branch as its initial value.
- Files: `web-app/src/components/backlog/detail/ActionsSection.tsx`, `web-app/src/components/backlog/BacklogItemDetail.tsx`

##### Task 3.2.2b: Gating tests (~4 min)
- Create `ActionsSection.jules.test.tsx` covering the four criteria.
- Files: `web-app/src/components/backlog/detail/ActionsSection.jules.test.tsx`

---

### Epic 3.3: Status surfacing

#### Story 3.3.1: `JulesStatusBadge`

**As a** user who is not watching a terminal, **I want** an honest, accessible
status chip for a Jules session, **so that** I can trust the tool's own reporting
— the only signal available for cloud work.

**Acceptance Criteria**:
- Never color-alone.
  - *Given* `phase="running"`, *When* the badge renders, *Then* it contains a distinct icon element, the visible text `Jules: Running`, and `role="img"` with an `aria-label` matching that text — asserted without reference to any color value.
- Routine transitions are polite, failure is assertive.
  - *Given* a badge transitioning `queued → running`, *When* it re-renders, *Then* the announcement lands in an element with `aria-live="polite"`; *and* given a transition to `failed`, *Then* a separate element with `role="alert"` is rendered (matching `RemoteConnectionIndicator.tsx`'s split).
- Nothing renders before a real state is known.
  - *Given* `phase={undefined}`, *When* the badge renders, *Then* it returns `null` — no neutral or optimistic placeholder chip.
- A stale poll is labeled, not disguised as failure.
  - *Given* `phase="running"` with `lastPolledAt` 8 minutes ago and `pollHealthy=false`, *When* the badge renders, *Then* it still reads `Jules: Running` and adds the secondary text `Last updated 8m ago, retrying…` — and does **not** switch to the failed variant.
- Reconnect-required renders distinctly, with a one-click fix, and is neither `Running` nor `Failed`.
  - *Given* `phase="reconnect-required"`, *When* the badge renders, *Then* it shows the text `Jules: Reconnect required`, an icon variant distinct from both the `Running` icon and the red `Failed` icon (amber), and an adjacent link with accessible name `Update key` whose `href` points at `/settings/jules`; the announcement region is `role="status"`/`aria-live="polite"` (this is a recoverable, expected state — not the interrupting `role="alert"` reserved for `Failed`, ux.md §4.2).

**Files**: `web-app/src/components/backlog/JulesStatusBadge.tsx`, `web-app/src/components/backlog/JulesStatusBadge.css.ts`, `web-app/src/components/backlog/JulesStatusBadge.test.tsx`

##### Task 3.3.1a: Badge component (~5 min)
- Create `JulesStatusBadge.tsx` taking `phase: JulesSessionPhase | undefined`, `lastPolledAt`, `pollHealthy`, `julesWebUrl`; structure copied from `RemoteConnectionIndicator.tsx` (null-until-known, polite region + separate alert region).
- Add the `reconnect-required` phase's amber variant (icon + `Jules: Reconnect required` text + `Update key` link to `/settings/jules`), rendered through the polite region alongside the other non-failure phases.
- Add `// +feature: jules-status-badge` in the first 10 lines.
- Files: `web-app/src/components/backlog/JulesStatusBadge.tsx`

##### Task 3.3.1b: Token-only styles for all phases (~4 min)
- Create `JulesStatusBadge.css.ts` with a variant per phase pulling from `vars.color.*` only, and verify both light and dark themes render (the design system's standard check).
- Files: `web-app/src/components/backlog/JulesStatusBadge.css.ts`

##### Task 3.3.1c: Badge tests (~5 min)
- Create `JulesStatusBadge.test.tsx` covering the four criteria via roles and text, never CSS classes.
- Files: `web-app/src/components/backlog/JulesStatusBadge.test.tsx`

---

#### Story 3.3.2: Jules session row and PR provenance

**As a** user, **I want** a Jules session to appear in the item's session list
with its own badge, a link to jules.google.com, and — once it opens a PR — the
ordinary PR chip plus a small Jules marker, **so that** cloud work is visible in
the same place as local work without a parallel UI.

**Acceptance Criteria**:
- A Jules row renders the badge instead of a branch chip.
  - *Given* an `ItemSession` with `role:"jules_work"`, `sessionId:"jules-sessions/xyz"`, no `endedAt`, *When* `SessionsSection` renders, *Then* the row shows `JulesStatusBadge` with `phase="running"` and shows **no** branch badge and **no** `SessionMonitor` (there is no PTY to monitor).
- The escape hatch to Jules' own UI is present.
  - *Given* the same row, *When* it renders, *Then* it contains a link whose accessible name is `View this session on jules.google.com` pointing at the session's stored web URL.
- The resulting PR uses the existing chip, unchanged.
  - *Given* the item has `prUrl:"https://github.com/tstapler/stapler-squad/pull/700"` recorded by the poller, *When* `PullRequestSection` renders, *Then* it renders the existing `GitHubBadge` component (no new PR component) with an adjacent `Jules` provenance marker.
- Terminal Jules rows read as ended, not stuck.
  - *Given* an `ItemSession` with `role:"jules_work"`, `endedAt` set, and `endReason:"jules_failed"`, *When* the row renders, *Then* the badge shows `Jules: Failed` and the row does **not** show the generic orphan/`ended` treatment that implies a leaked local session.
- A revoked/expired key overrides the row's own state for every open Jules session at once.
  - *Given* an open `jules_work` `ItemSession` (would otherwise render `phase="running"` or `"queued"` from its own role/`endedAt`) and `GetJulesConfig` reporting `auth_reconnect_required:true`, *When* `SessionsSection` renders, *Then* the row shows `JulesStatusBadge` with `phase="reconnect-required"`, not its own otherwise-computed phase; *and* once `auth_reconnect_required` flips back to `false` (Story 2.3.4's automatic recovery), *Then* the row reverts to its normally-computed phase on the next data refresh, with no separate user action.

**Files**: `web-app/src/components/backlog/detail/SessionsSection.tsx`, `web-app/src/components/backlog/detail/PullRequestSection.tsx`, `web-app/src/components/backlog/detail/SessionsSection.jules.test.tsx`

##### Task 3.3.2a: Jules branch in `SessionsSection` (~5 min)
- In `SessionsSection.tsx`, branch on `s.role === "jules_work"` at the row level (around the existing role/branch/orphan rendering at `:214-264`) to render `JulesStatusBadge` + the jules.google.com link, and suppress the branch badge, `SessionMonitor` (`:442`), and the orphan `ended` badge for that role.
- Phase precedence: for an open (`endedAt` null) `jules_work` row, if `GetJulesConfig`'s `auth_reconnect_required` is `true`, pass `phase="reconnect-required"` regardless of the row's own state; otherwise compute phase from the row's own role/`endedAt`/`endReason` as already specified. A closed row's phase is never overridden — the account-wide flag only affects sessions still open and actually being polled.
- Files: `web-app/src/components/backlog/detail/SessionsSection.tsx`

##### Task 3.3.2b: PR provenance marker (~4 min)
- In `PullRequestSection.tsx`, add a small `Jules` text/icon marker beside the existing `GitHubBadge` when the item's most recent session role is `jules_work` — do not fork `GitHubBadge` (ux.md §0/§5).
- Files: `web-app/src/components/backlog/detail/PullRequestSection.tsx`

##### Task 3.3.2c: Session-row tests (~5 min)
- Create `SessionsSection.jules.test.tsx` covering the four criteria.
- Files: `web-app/src/components/backlog/detail/SessionsSection.jules.test.tsx`

---

## Phase 4: Observability, registry, and end-to-end coverage

### Epic 4.1: Ship-readiness

#### Story 4.1.1: Structured logging and the usage counter

**As an** operator, **I want** every Jules call logged distinctly with no secret
material and counted, **so that** debugging and spend awareness do not require
reading raw logs.

**Acceptance Criteria**:
- Every log line named in the Observability Plan exists and none carries a secret.
  - *Given* a full dispatch → poll → complete cycle run against fakes with a captured `slog` handler, *When* the cycle finishes, *Then* the captured records include `jules dispatch requested`, `jules session created`, `jules session state changed`, and none of the captured records' attributes or messages contain the test API key `"AIzaSyD-EXAMPLE"` or the string `x-goog-api-key`.
- Counters increment exactly once per event.
  - *Given* one successful dispatch and one failed poll, *When* the counters are read, *Then* `jules.session.dispatched == 1` and `jules.api.error == 1`.

**Files**: `server/services/jules_usage_counter.go`, `server/services/jules_usage_counter_test.go`, `session/jules_session_poller.go`

##### Task 4.1.1a: `JulesUsageCounter` (~4 min)
- Create `server/services/jules_usage_counter.go` with atomic counters and a `Snapshot()` returning a value struct for `GetJulesConfig`; increment from the dispatch service and the poller.
- Files: `server/services/jules_usage_counter.go`, `server/services/jules_dispatch_service.go`, `session/jules_session_poller.go`

##### Task 4.1.1b: Log-content test (~5 min)
- Create `server/services/jules_usage_counter_test.go` with a capturing `slog.Handler` asserting both criteria — the secret-absence assertion covers the whole captured record set, not just messages.
- Files: `server/services/jules_usage_counter_test.go`

---

#### Story 4.1.2: Feature registry and documentation

**As a** maintainer, **I want** the new RPCs and components registered and the
one manual prerequisite documented, **so that** `make registry-generate` stays
clean and a new user knows they must connect the repo at jules.google.com first.

**Acceptance Criteria**:
- The registry is up to date and idempotent.
  - *Given* the completed implementation, *When* `make registry-generate` is run twice, *Then* the second run produces no diff, and `docs/registry/features/backend/backlog/dispatch-to-jules.json` exists with `"markerFound": true` and `"handlerFile": "server/services/jules_dispatch_service.go"`.
- The manual prerequisite is documented where a user will look.
  - *Given* `docs/how-to/dispatch-work-to-google-jules.md`, *When* it is read, *Then* it states that the repo must first be connected through Jules' GitHub App at jules.google.com (not automatable via the API), that the branch must already be pushed, and that source code leaves the machine.
- The docs index links it.
  - *Given* the root `CLAUDE.md` Reference Documents Index, *When* it is read, *Then* it contains a row pointing at `docs/how-to/dispatch-work-to-google-jules.md`.

**Files**: `docs/registry/features/backend/backlog/dispatch-to-jules.json`, `docs/registry/features/backend/jules/*.json`, `docs/registry/features/frontend/*.json`, `docs/how-to/dispatch-work-to-google-jules.md`, `CLAUDE.md`

##### Task 4.1.2a: Run the registry generator (~3 min)
- Run `make registry-diff`, then `make registry-generate`, and commit only the changed per-feature files.
- Files: `docs/registry/features/**`

##### Task 4.1.2b: How-to doc and index row (~5 min)
- Write `docs/how-to/dispatch-work-to-google-jules.md` (Diataxis how-to): prerequisites (connect repo at jules.google.com, push the branch, add the API key), the dispatch steps, what each badge state means, and the failure escape hatch. Keep it short — prerequisites and steps, not a design narrative.
- Add the index row to `CLAUDE.md`'s Reference Documents Index table.
- Files: `docs/how-to/dispatch-work-to-google-jules.md`, `CLAUDE.md`

---

#### Story 4.1.3: End-to-end coverage of the gated dispatch flow

**As a** maintainer, **I want** a Playwright spec covering the gating and the
egress confirmation against a stubbed Jules API, **so that** a regression in the
opt-in path — the one with a real privacy consequence — is caught in CI.

**Acceptance Criteria**:
- The gated-off state is asserted end to end.
  - *Given* the isolated test server started with Jules disabled, *When* a `ready` item's detail page loads, *Then* `[data-testid="dispatch-to-jules"]` is not attached.
- The egress confirmation blocks dispatch until checked.
  - *Given* Jules enabled with a stubbed key and an unacknowledged repo, *When* the user opens the dialog, fills branch `backlog/e2e-1` and a prompt, and does **not** check the confirmation, *Then* the `Dispatch` button has `[disabled]`; *and* when the box is checked, *Then* it becomes enabled.
- The spec follows the repo's E2E conventions.
  - *Given* `tests/e2e/jules-dispatch.spec.ts`, *When* the convention linter runs, *Then* the file starts with `// @feature backlog:dispatch-to-jules, jules-settings`, uses only `data-testid`/ARIA locators, and contains no `waitForTimeout`.

**Files**: `tests/e2e/jules-dispatch.spec.ts`, `tests/e2e/pages/JulesDispatchPage.ts`

##### Task 4.1.3a: Page helper (~4 min)
- Create `tests/e2e/pages/JulesDispatchPage.ts` exposing `openDialog()`, `fillBranch()`, `fillPrompt()`, `acknowledgeEgress()`, `dispatchButton()` — new page helpers go in `tests/e2e/pages/` per the conventions.
- Files: `tests/e2e/pages/JulesDispatchPage.ts`

##### Task 4.1.3b: Spec (~5 min)
- Create `tests/e2e/jules-dispatch.spec.ts` with the two scenarios, driving config through the isolated test server's own settings RPC rather than editing files.
- Files: `tests/e2e/jules-dispatch.spec.ts`

---

## Definition of Done

- `make ready` is green (which includes `make ci`, the `dupl` new-code duplication gate, and `jscpd` for `web-app/`).
- `cd web-app && npx jest --no-coverage` is green.
- `make registry-generate` produces no diff.
- `go list -deps ./jules` contains no `session/` or `server/` import (Story 1.1.2's own test enforces this in CI).
- The three Unresolved Questions are either answered in `research/stack.md` or explicitly re-recorded as still open, with the interim behavior named.
