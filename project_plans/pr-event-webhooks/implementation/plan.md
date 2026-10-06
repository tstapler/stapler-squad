# Implementation Plan: pr-event-webhooks

**Feature**: Extend the existing `/webhooks/github` receiver to react to
`check_run`/`workflow_run` (CI completion) and `pull_request_review`/
`issue_comment` (review feedback) events by nudging the existing PR-fix
reconciliation loop to run immediately for the matched item, instead of
waiting up to `PRStatusPoller`'s 60s tick.
**Date**: 2026-08-23
**Status**: Ready for implementation
**ADRs**:
- `decisions/ADR-001-self-actor-filter-for-bot-loop-avoidance.md`
- `decisions/ADR-002-extract-full-per-item-reconciliation-body.md`

---

## Step 0.5 — Creative Pass: Webhook Routing Alternatives

**Alternative A (chosen) — same struct/route, branch on `X-GitHub-Event`,
dispatch to a new consumer-defined `PRFixEventRouter` calling a single-item,
on-demand invocation of the existing reconciliation body.**
Strength: reuses the proven per-candidate signature-verification loop
(`server/services/github_webhook_handler.go:93-117`) and the existing
backoff/dedup gate (`remediatePRFixWithBackoffGate`) with zero duplication —
the new code is purely additive.
Weakness: `Handle` (or its immediate callees) now carries two responsibilities
(push→session-creation, PR-event→fix-trigger) in one file, though splitting
the PR-fix branch into its own sibling method/file keeps each piece small.

**Alternative B (rejected) — a second `GitHubPRFixWebhookHandler` struct/route.**
Strength: cleanly separates the two concerns at the type level.
Weakness: GitHub sends every subscribed event type to one webhook
configuration with one secret; the per-repo secret is stored on
`github_push`-type `Workflow` rows and matched via the existing
per-candidate loop. A second handler would have to duplicate that
signature-verification loop verbatim — exactly the struct-duplication /
forwarding-wrapper smell `.claude/rules/interface-pollution-checklist.md`
flags, for a route that isn't actually a different route.

**Alternative C (rejected) — decide fix-or-not directly from each event's own
payload fields (e.g. `check_run.conclusion == "failure"` ⇒ call
`AutoReopenForPRFix` with a payload-built `fixContext`), bypassing
`GetPRStatus`/`remediatePRFixWithBackoffGate`.**
Strength: fastest to implement, no per-item extraction refactor, no fresh
`GetPRStatus` API call per event.
Weakness: reimplements CI/review aggregation with an inferior, single-event
view (misses other still-failing checks, other blocking reviewers, conflict
status — `architecture.md` §1's case (a)/(b)) and bypasses the only
backoff/dedup gate that prevents CI-job-fanout from spawning N redundant fix
sessions per PR (`pitfalls.md` §2) — a correctness and safety regression
versus the poller path.

**Decision: Alternative A**, confirming the research's recommendation. See
the Pattern Decisions table below for the specific sub-decisions this
implies (interface placement, dispatch mechanism, actor filtering).

---

## Domain Glossary

| Term | Definition | Notes |
|------|-----------|-------|
| `GitHubWebhookHandler` | Existing struct handling `POST /webhooks/github`. | `server/services/github_webhook_handler.go:17` — extended, not replaced. |
| `X-GitHub-Event` | HTTP header GitHub sends identifying the webhook event type (`"push"`, `"check_run"`, etc.). | Currently never read by `Handle`; this feature adds the first read of it. |
| `check_run` / `workflow_run` | GitHub CI-completion webhook event types. | Only the `action == "completed"` delivery carries a non-null `conclusion`. |
| `pull_request_review` | GitHub webhook event type for a submitted/edited/dismissed PR review. | Only `action == "submitted"` is actionable here. |
| `issue_comment` | GitHub webhook event type for both issue and PR conversation-tab comments. | Only actionable when `issue.pull_request` is present and `action == "created"`. |
| `action` | The payload field naming the specific sub-event within an event type (e.g. `"completed"`, `"submitted"`, `"created"`). | Distinct from `conclusion`/`state`. |
| `conclusion` | The payload field on `check_run`/`workflow_run` giving the terminal CI result (`"success"`, `"failure"`, etc.). | Used only as a cheap pre-filter, not the final fix-or-not decision (see Pattern Decisions). |
| `PRFixEventRouter` | New narrow interface, defined in `server/services` (the consumer), with one method `TriggerPRFixForEvent`. | Satisfied by `*session.BacklogLifecycleListener`. Replaces the idea of injecting raw `session.PRFixSpawner`. |
| `TriggerPRFixForEvent` | New exported method on `*session.BacklogLifecycleListener`: `(ctx, repoFullName string, prNumber int) (matched bool, err error)`. | Looks up the matching `pr_pending` item and, if found, runs `reconcilePRPendingItem` for it immediately. |
| `reconcilePRPendingItem` | New private method on `*session.BacklogLifecycleListener` — the full per-item body extracted from `ReconcilePRPending`'s loop. | See ADR-002. Called both by `ReconcilePRPending`'s loop and by `TriggerPRFixForEvent`. |
| `findPRPendingItemForEvent` | New package-private function in `session/backlog_lifecycle_pr.go`: `(er *EntRepository, repoFullName string, prNumber int) (*ent.BacklogItem, bool)`. | Filters `FindPRPendingItems`' results by `PrNumber` + `github.GetOwnerRepoFromRemote(item.RepoPath).String()`. |
| `handlePRFixEvent` | New method on `GitHubWebhookHandler` — dispatch target for the 4 new event types, called from `Handle` after signature verification. | Lives in a new file `server/services/github_webhook_pr_fix.go`. |
| `PRFixSpawner` | Existing interface, unchanged. | `session/backlog_lifecycle_pr.go:21-23`. Still the only thing that actually spawns a fix session. |
| `AutoReopenForPRFix` | Existing method, unchanged. | `server/services/backlog_service_triage.go:2018-2138`. |
| `remediatePRFixWithBackoffGate` | Existing backoff/dedup gate, unchanged, reused. | `session/backlog_lifecycle_pr.go:1181-1222`. The real correctness guard against CI-job-fanout duplicate spawns. |
| `RemediationDue` | Existing backoff-check method the gate above calls. | `session/backlog_remediation.go`. |
| `pr_event_webhooks` | New sibling feature flag, checked only inside `handlePRFixEvent` (not at route-registration time). | `config.Config.GetFeatureFlag`/`SetFeatureFlag`, `config/config.go:1329`/`:1337`. |
| `selfLoginCache` | New small TTL-cached wrapper around `github.GetCurrentUserLogin(ctx)`, used for the self-actor filter. | See ADR-001. |
| `TriggerFireEvent` | Existing audit-trail row, unchanged schema. | `session/ent/schema/trigger_fire_event.go`. `workflow_id` is nullable — new outcome rows for PR-fix events carry `WorkflowID: nil`. |
| `matched` (return value) | `TriggerPRFixForEvent`'s bool: whether a tracked `pr_pending` item was found for `(repoFullName, prNumber)`. | Drives the webhook handler's `no_match` vs. `fired_success` audit outcome — see Observability Plan for what "fired_success" does and doesn't guarantee. |
| `RepoRef` | Existing value type, unchanged. | `github/repo_ref.go:11`. `.String()` (`:35`) produces the exact `owner/repo` shape to compare against `repository.full_name`. |

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| Event-type dispatch in `Handle` | Plain `switch r.Header.Get("X-GitHub-Event")` / early-return dispatch — **not** a GoF Strategy interface per event type | `research/architecture.md` §3, `research/build-vs-buy.md` §3 | A `PRFixEventHandler` interface with one implementation per event type | Each branch runs once per request with no swappable runtime behavior — a Strategy interface here would be a speculative, one-implementation-each abstraction (`interface-pollution-checklist.md` smell #1), adding indirection with no polymorphic benefit. |
| Fix-loop integration | Facade/reuse: single-item, on-demand invocation of the existing per-item reconciliation body | `research/architecture.md` §1 | A new decision engine parsing payload fields directly into `fixContext` | Would duplicate `GetPRStatus`'s aggregate CI/review/conflict synthesis with an inferior, payload-only implementation (misses other failing checks/reviewers/conflicts). |
| Routing target injected into `GitHubWebhookHandler` | Narrow consumer-defined interface `PRFixEventRouter` (one method) | `research/architecture.md` §3, `.claude/rules/interface-pollution-checklist.md` | Inject `session.PRFixSpawner` directly | Bypasses `remediatePRFixWithBackoffGate`'s backoff/dedup gate — the actual protection against CI-job-fanout duplicate spawns (`research/architecture.md` §4). |
| Actor/bot-loop filtering (`issue_comment`/`pull_request_review`) | Self-login equality check via a small TTL-cached `github.GetCurrentUserLogin` wrapper | ADR-001 | Filter any `sender.type == "Bot"` | Would drop legitimate bot-authored feedback a human wants acted on (`research/pitfalls.md` §5); `isExcludedBotAuthor` (`session/git/worktree_git.go:469-479`) already establishes "not all bots" as this repo's rule for a sibling problem. |
| Payload parsing (4 new event types) | Hand-rolled `map[string]interface{}` extractors mirroring `extractGitHubRepoAndBranch` | `research/build-vs-buy.md` §1 | `google/go-github`'s typed webhook structs + `ParseWebHook` | New heavyweight dependency for 4-6 field reads; diverges from this file's established minimal-extraction style for marginal benefit. |
| Fix-spawn dedup for rapid/duplicate deliveries | Reuse `RemediationDue`'s itemID-scoped backoff gate; the natural DB re-query in `findPRPendingItemForEvent` (fresh per request, filtered on live `status == pr_pending`) already supplies the "is this item still pending" short-circuit `pitfalls.md` §2 recommends, at zero extra cost | `research/pitfalls.md` §2, §3 | A new short-TTL in-process debounce keyed by `(repo, prNumber)` | `RemediationDue` is already proven under concurrency (CAS-tested regression tests cited in `pitfalls.md` §2); a second debounce layer is redundant complexity for marginal additional API-quota savings — flagged as a possible future optimization, not required for this pass. |
| Merge-check inclusion in the extracted per-item body | Extract the **full** per-item body (merge detection + CI/review-check), not just the CI/review half | ADR-002 | Extract only `session/backlog_lifecycle_pr.go:1355-1541` (`research/architecture.md`'s summary-table citation) | The narrower extraction would misclassify a just-merged PR as "closed without merging" for any webhook-triggered call arriving before the item's status catches up — see ADR-002 for the concrete mechanism. |
| Audit-trail persistence for the 4 new event types | Reuse `persistTriggerFireEvent`/`TriggerFireEventRepository` directly (single `Create` call per outcome), `WorkflowID: nil` | `research/pitfalls.md` §6 | A new dedicated audit table/mechanism for PR-fix-webhook events | `TriggerFireEvent.workflow_id` is already nullable (`session/ent/schema/trigger_fire_event.go:27-29`) and carries no DB-level enum constraint on `outcome` — no schema change needed; a parallel mechanism would duplicate a working pattern. |
| Feature flag scope | New sibling flag `pr_event_webhooks`, checked only inside `handlePRFixEvent` | `requirements.md` Goal 5, `research/architecture.md` §3 | Reuse `webhook_triggers` for both push and PR-fix events | PR-fix auto-reopen has materially higher blast radius (spawns work sessions, pushes commits) than push-triggered session creation — an operator needs independent on/off control. |

---

## Migration Plan

**N/A — confirmed, no ent schema changes needed.**

- `TriggerFireEvent.workflow_id` is already `.Optional().Nillable()`
  (`session/ent/schema/trigger_fire_event.go:27-29`) — rows for the 4 new
  event types simply carry `WorkflowID: nil`, exactly like any other
  rejected-before-a-Workflow-resolves row already does today.
- `TriggerFireEvent.outcome` is `field.String("outcome").NotEmpty()`
  (`session/ent/schema/trigger_fire_event.go:31-32`) with **no** enum/Values
  constraint and no DB-level check constraint in the schema file — new
  outcome string literals (see Observability Plan) need no migration, only
  a doc-comment update on `TriggerFireEventInput.Outcome`
  (`session/trigger_fire_event_repository.go:21-31`).
- `BacklogItem`'s existing `pr_url`/`pr_number` fields
  (`session/ent/schema/backlog_item.go:92-96`) and `FindPRPendingItems`
  (`session/storage_backlog.go:980`) already provide everything
  `findPRPendingItemForEvent` needs — no new index.

## Observability Plan

- **Logs**: the extracted `reconcilePRPendingItem` keeps every existing
  `log.InfoLog()`/`log.WarningLog()`/`log.ErrorLog()` call verbatim (ADR-002
  is a pure move) — a webhook-triggered call logs identically to a
  poller-triggered one today, with one addition: `TriggerPRFixForEvent`
  logs one `InfoLog` line on entry naming the trigger source (e.g.
  `"[BacklogLifecycle] TriggerPRFixForEvent repo=%s pr=%d event=%s"`) so an
  operator can grep for "did the webhook fire, or did only the poller
  catch this" — directly answering `research/features.md` §6's concern
  that the trigger source is otherwise invisible.
- **Audit trail** (`TriggerFireEvent`, `WorkflowID: nil`): four outcomes,
  written by `handlePRFixEvent`:
  - `"rejected"` — signature invalid (shared with the push path's existing
    per-candidate loop) or payload extraction failed (missing
    `repository.full_name`). These are two different failure causes (operator
    misconfiguration vs. a parsing bug) that the outcome string alone can't
    distinguish; the `ErrorMessage` field on `TriggerFireEventInput` (already
    populated by both branches in Task 2.3.2b's code sketch — `"invalid
    signature"` vs. `"missing repository.full_name"`) is what disambiguates
    them, no new field needed.
  - `"no_match"` — event not actionable (wrong `action`/`conclusion`/
    `state`, self-authored, or `issue_comment` on a non-PR issue) **or**
    `TriggerPRFixForEvent` returned `matched == false` (no tracked
    `pr_pending` item for that `(repoFullName, prNumber)` — this
    subsumes the "item already left `pr_pending`" case for free, since
    `findPRPendingItemForEvent` re-queries `FindPRPendingItems` fresh on
    every request, filtered on live `status == pr_pending`: by the time a
    second/third/Nth CI-job-fanout delivery for the same PR arrives after
    the first already flipped the item to `in_progress`, the query simply
    no longer returns it).
  - `"fired_success"` — `matched == true` and `TriggerPRFixForEvent`
    returned `err == nil`. **Scoping note, stated explicitly so it isn't
    over-read later**: because `reconcilePRPendingItem` preserves
    `ReconcilePRPending`'s existing all-internal-error-logging contract
    (errors are logged, not returned, matching today's behavior), this
    outcome means "the on-demand reconciliation ran without a router-level
    error" — it does **not** by itself distinguish "a fix session was
    spawned" from "the backoff gate wasn't due yet" from "the PR turned out
    healthy, no-op." Those finer distinctions remain visible only in
    `reconcilePRPendingItem`'s own log lines (unchanged from today). This
    is a deliberate scope limit, not an oversight — building return-value
    plumbing to thread that distinction back through `TriggerPRFixForEvent`
    would diverge from `ReconcilePRPending`'s proven contract for a
    benefit no requirement asks for. **Concretely**: an operator or
    dashboard should not treat a `fired_success` count as a count of fix
    sessions spawned — see `reconcilePRPendingItem`'s own logs for that
    finer distinction.
  - `"fired_failed"` — reserved for router-level errors only (e.g. `nil`
    `PRFixEventRouter` wiring) — expected to be rare/never in practice
    given `reconcilePRPendingItem`'s error-swallowing contract above.
- **Metrics**: none new. This repo has no engineered GitHub-API-quota or
  webhook-throughput metrics today (`research/pitfalls.md` §5) — out of
  scope for this pass, consistent with Non-Goals.
- **Alerts**: none new — the audit trail above is queryable via existing
  `TriggerFireEventRepository` access, same as the `push` path.

## Risk Control

- **Pre-mortem P1 #1 — missing `github_push` Workflow row is a silent dead
  end.** An operator can enable `pr_event_webhooks` for a repo that has an
  active `pr_pending` item but no `github_push`-type `Workflow` row (the
  only secret source the signature-verification loop checks, §3's
  `verifySignatureForRepo`) — every delivery for that repo then 401s and is
  persisted as `outcome: "rejected"` forever, with no signal distinguishing
  "misconfigured" from "nothing needed fixing." **Prevention, added to this
  plan**: `TriggerPRFixForEvent` (Task 1.2.2a) logs one `WarningLog` line
  the first time it is asked to resolve an item whose repo has zero
  `github_push`-type `Workflow` rows (checked via the same
  `WorkflowRepository.ListByTriggerType("github_push")` the push path
  already calls) — e.g. `"[BacklogLifecycle] repo=%s has an active
  pr_pending item but no github_push Workflow row configured; PR-fix
  webhook events for it will 401 until one exists"`. This is a log-only
  addition (no new task/story — folded into Task 1.2.2a's implementation),
  since a full startup-time audit across all tracked repos is out of scope
  for this pass; the per-event log line is the cheapest signal that closes
  the "operator has no way to tell" gap. Add this note to Task 1.2.2a
  directly.
- **Pre-mortem P1 #2 — the public-reachability doc has no code-level
  "did this ever actually fire" signal.** An operator can flip
  `pr_event_webhooks` on without ever completing the tunnel/nginx setup
  from Epic 3.2's doc, and the feature will look fully healthy (tests pass,
  flag is on, `PRStatusPoller` keeps catching everything on its normal
  cadence) while silently never receiving a single real GitHub delivery —
  row *absence* is indistinguishable from "nothing needed fixing."
  **Prevention, added to this plan**: `handlePRFixEvent` (Task 2.3.2b) logs
  one `InfoLog` line on the **first** successfully-verified delivery of
  each of the 4 new event types per process lifetime (a small in-memory
  `sync.Once`-per-event-type set on `GitHubWebhookHandler`, not persisted)
  — e.g. `"[GitHubWebhookHandler] first verified check_run delivery
  received — /webhooks/github reachability confirmed"` — so an operator can
  `grep` logs for "has this ever fired" instead of reasoning from absence.
  Story 3.2.1's doc (Task 3.2.1a) gains a matching "verify reachability"
  step: after completing the tunnel setup, trigger one real GitHub
  delivery (e.g. re-deliver a past `check_run` event from the repo's
  webhook settings "Recent Deliveries" panel) and confirm the new log line
  appears. Add this to Task 2.3.2b's implementation and Task 3.2.1a's doc
  checklist directly.
- **Dependency between the two flags is a silent-404 trap.** If an operator
  enables `pr_event_webhooks` while `webhook_triggers` is still off, the
  `/webhooks/github` route itself is never registered (`404`, per Task
  3.1.1c/`server.go:763`'s existing gate) — a fully silent failure with zero
  audit rows and zero logs, the same "operator has no signal" shape the two
  pre-mortem P1 fixes above already closed for other misconfiguration states.
  `pr_event_webhooks` has no effect unless `webhook_triggers` is also enabled
  — document this dependency in the reachability doc (Task 3.2.1a) so an
  operator doesn't enable the wrong flag and see silent 404s. **UX re-review
  follow-up**: treat this the same as the two P1s, not just as a doc note —
  add a startup-time (or flag-set-time) `WarningLog` when `pr_event_webhooks
  == true` and `webhook_triggers == false` is detected, e.g. `"[server]
  pr_event_webhooks is enabled but webhook_triggers is not — /webhooks/github
  is not registered, PR-fix webhook events will silently 404"`. Fold this
  into Task 3.1.1c's implementation alongside the route-registration change.
- **Pre-mortem P2 #5 (matrix-CI quota burn) — deferral reconciled.** The
  Pattern Decisions table (`Fix-spawn dedup for rapid/duplicate deliveries`
  row, above) still defers the in-process `(repo, prNumber)` debounce as "a
  possible future optimization, not required for this pass," which pre-
  mortem.md's Failure #5 recommended promoting to required. Explicit
  accept-and-track decision: **deferral stands** — `RemediationDue`'s CAS
  gate already prevents duplicate *spawns* (the correctness property that
  matters), and the debounce would only save redundant `GetPRStatus` API
  calls during a duplicate-delivery burst (a quota/cost concern, not a
  correctness one). Accepted as a residual P2 risk for this pass; revisit if
  `research/pitfalls.md` §5's predicted quota pressure is actually observed
  post-rollout (the `TriggerFireEvent` audit trail this plan already builds
  is sufficient to detect that after the fact).
- **Feature flag**: `pr_event_webhooks`, default off (absent key ⇒ `false`,
  `config/config.go:1329-1335`). Route registration stays gated on the
  existing `webhook_triggers` flag (`server/server.go:763`) — flipping
  `pr_event_webhooks` alone (no restart needed, since the check happens
  inside `Handle` on every request, not at registration time) is how an
  operator opts in.
- **Rollback procedure**: flip `pr_event_webhooks` back to `false` via
  `config.SetFeatureFlag` (no restart required — the new branch simply
  stops matching and 200s with `outcome: "no_match"` for every PR-fix event
  type, same shape as today's pre-feature behavior). A full code revert is
  the fallback if the flag path itself is implicated.
- **Staged rollout**: this is a single-operator local-instance deployment
  (`.claude/docs/state-isolation.md`) — "staged rollout" is "flag off by
  default, operator opts in on their own instance and watches
  `TriggerFireEvent` outcomes for a few days" rather than a multi-tenant
  phased release.

## Unresolved Questions

- [ ] Does every repo the operator wants PR-fix reactions on already have a
      `github_push`-type `Workflow` row configured (the only existing
      secret-storage source the per-candidate verification loop can check
      against)? If not, PR-fix events for that repo 401 (fail closed) with
      no stored secret to verify against. This is an **operational
      prerequisite to document** (Story 3.2.1's doc addition), not a code
      change this plan needs to make — blocks nothing in the plan itself,
      but blocks a given repo's events from ever passing signature
      verification until a `github_push` Workflow exists for it. Owner:
      operator, at rollout time.
- [ ] GitHub's documented `action`/`conclusion`/`state` enum values are
      pinned as Go consts (Task 2.1.2a) based on GitHub's *current*
      documentation as of this research pass; GitHub has occasionally added
      new values without a breaking-change notice
      (`research/build-vs-buy.md` §4). An unrecognized value safely falls
      through to "not actionable" (`no_match`) rather than erroring, so this
      is a watch item, not a blocker. Owner: whoever revisits this feature
      if GitHub adds a new terminal `conclusion`/`state` value that should
      be actionable.

## Dependency Visualization

```
GitHub ──POST /webhooks/github──▶ GitHubWebhookHandler.Handle
                                          │
                          ┌───────────────┴────────────────┐
                          │ X-GitHub-Event == "push"        │ X-GitHub-Event ∈ {check_run,
                          │ (unchanged existing path)       │ workflow_run, pull_request_review,
                          ▼                                 │ issue_comment}
                 extractGitHubRepoAndBranch                 ▼
                 → github_push Workflow match       handlePRFixEvent (new)
                 → claimAndFireTrigger                       │
                                                    pr_event_webhooks flag check
                                                               │
                                              per-event-type extractor (new, ×4)
                                              (action/conclusion/state pre-filter;
                                               self-actor filter for review/comment)
                                                               │
                                                    actionable? ──no──▶ persist "no_match" ─▶ 200
                                                               │yes
                                                    PRFixEventRouter.TriggerPRFixForEvent
                                                    (new interface; *session.BacklogLifecycleListener)
                                                               │
                                              findPRPendingItemForEvent(er, repoFullName, prNumber)
                                                               │
                                                   matched? ──no──▶ persist "no_match" ─▶ 200
                                                               │yes
                                                    reconcilePRPendingItem (extracted, ADR-002)
                                                               │
                                          ┌────────────────────┴────────────────────┐
                                          │ merged → done transition,               │ CI failing / blocked review /
                                          │ ship snapshot, cleanup (step 1,         │ conflict / new feedback (step 2)
                                          │ unchanged)                              │
                                          ▼                                         ▼
                                     (unchanged)                     remediatePRFixWithBackoffGate
                                                                          (existing backoff/dedup gate)
                                                                                   │
                                                                       AutoReopenForPRFix (unchanged)
                                                                                   │
                                                              persist "fired_success"/"fired_failed" ─▶ 200

PRStatusPoller (session/pr_status_poller.go) ── unchanged, still ticks every 60s ──▶ ReconcilePRPending
                                                     (independent backstop; not modified by this feature)
```

---

## Phase 1: Fix-Loop Trigger Integration (session package)

### Epic 1.1: Extract a reusable per-item PR reconciliation body

**Goal**: Make `ReconcilePRPending`'s per-item logic callable for exactly
one item, on demand, with zero behavior change to the existing 60s-tick
path. This is the foundation everything else in this plan builds on.

#### Story 1.1.1: Extract `reconcilePRPendingItem`
**As a** maintainer, **I want** the per-item body factored out of
`ReconcilePRPending`'s loop, **so that** a webhook-triggered caller can
invoke the identical logic for one item without duplicating it.
**Acceptance Criteria**:
- `reconcilePRPendingItem` exists as a private method with signature
  `func (l *BacklogLifecycleListener) reconcilePRPendingItem(ctx
  context.Context, er *EntRepository, item *ent.BacklogItem)` and contains
  exactly the logic currently at `session/backlog_lifecycle_pr.go:1249-1541`
  (everything from `g := l.getPRPendingCheckerFactory()(repoPath)` through
  the loop body's end), with every `continue` rewritten to `return`.
  - *Given* an item with `PrNumber=189`, `PrURL="https://github.com/tstapler/stapler-squad/pull/189"`, whose PR was just merged, *When* `reconcilePRPendingItem` runs, *Then* it performs the same `IsPRMerged` → ship-snapshot → `done`-transition sequence `ReconcilePRPending` performs today (verified by the existing `TestReconcilePRPending_should_TransitionToDone_When_PRMerged`-shaped test in `session/backlog_lifecycle_test.go` continuing to pass unmodified against the new call path).
- `ReconcilePRPending`'s loop body becomes `for _, item := range items {
  l.reconcilePRPendingItem(ctx, er, item) }` — no other change to
  `ReconcilePRPending`'s signature or the `//nolint:gocognit,gocyclo,funlen`
  comment's applicability (it moves with the body).
**Files**: `session/backlog_lifecycle_pr.go`

##### Task 1.1.1a: Move the per-item body into `reconcilePRPendingItem` (~5 min)
- Cut `session/backlog_lifecycle_pr.go:1249-1541` (the loop body) into a new
  method `reconcilePRPendingItem(ctx context.Context, er *EntRepository,
  item *ent.BacklogItem)` placed immediately above `ReconcilePRPending`.
- Replace every `continue` inside the moved body with `return`.
- Files: `session/backlog_lifecycle_pr.go`

##### Task 1.1.1b: Replace `ReconcilePRPending`'s loop body with the call (~2 min)
- `for _, item := range items { l.reconcilePRPendingItem(ctx, er, item) }`.
- Move the `//nolint:gocognit,gocyclo,funlen` comment to
  `reconcilePRPendingItem`'s doc comment (the complexity moved with the
  body; `ReconcilePRPending` itself is now trivially simple and shouldn't
  carry the suppression).
- Files: `session/backlog_lifecycle_pr.go`

##### Task 1.1.1c: Run the existing regression suite (~3 min)
- `go test ./session/... -run 'ReconcilePRPending|PRPending' -v` covering
  `session/backlog_lifecycle_test.go`,
  `session/backlog_lifecycle_stuck_test.go`,
  `session/backlog_lifecycle_superseded_test.go`,
  `session/backlog_lifecycle_pr_branch_guard_test.go` — all must pass
  unmodified (no test file changes in this task; a failure means the
  extraction changed behavior, not that a test needs updating).
- Files: none changed (verification only)

### Epic 1.2: PR-to-item lookup and the on-demand trigger entry point

**Goal**: Give the webhook handler a way to ask "is there a `pr_pending`
item for PR #N in repo R, and if so, react to it right now" without
touching `EntRepository` internals directly.

#### Story 1.2.1: `findPRPendingItemForEvent` lookup
**As a** webhook handler, **I want** to resolve `(repoFullName, prNumber)`
to a tracked `pr_pending` `BacklogItem`, **so that** I know whether this
instance is tracking the PR the event is about.
**Acceptance Criteria**:
- Given `repoFullName = "tstapler/stapler-squad"` and `prNumber = 189`,
  and a `pr_pending` `BacklogItem` exists with `PrNumber: 189` and
  `RepoPath` resolving (via `github.GetOwnerRepoFromRemote`) to
  `RepoRef{owner: "tstapler", repo: "stapler-squad"}`, *When*
  `findPRPendingItemForEvent(er, "tstapler/stapler-squad", 189)` is called,
  *Then* it returns `(item, true)` for that item.
- Given the same inputs but no `pr_pending` item has `PrNumber == 189`,
  *When* called, *Then* it returns `(nil, false)`.
- Given a `pr_pending` item with `PrNumber == 189` but `RepoPath` resolving
  to `RepoRef{owner: "someone-else", repo: "unrelated-fork"}` (a PR number
  collision across two different tracked repos), *When* called with
  `repoFullName = "tstapler/stapler-squad"`, *Then* it returns `(nil,
  false)` — the repo identity, not just the PR number, must match.
**Files**: `session/backlog_lifecycle_pr.go`

##### Task 1.2.1a: Implement `findPRPendingItemForEvent` (~5 min)
```go
func findPRPendingItemForEvent(ctx context.Context, er *EntRepository, repoFullName string, prNumber int) (*ent.BacklogItem, bool) {
    items, err := er.FindPRPendingItems(ctx)
    if err != nil {
        log.WarningLog().Printf("[BacklogLifecycle] findPRPendingItemForEvent FindPRPendingItems: %v", err)
        return nil, false
    }
    for _, item := range items {
        if item.PrNumber != prNumber {
            continue
        }
        ref, refErr := github.GetOwnerRepoFromRemote(item.RepoPath)
        if refErr != nil || !ref.IsValid() {
            continue
        }
        if ref.String() == repoFullName {
            return item, true
        }
    }
    return nil, false
}
```
- Files: `session/backlog_lifecycle_pr.go`

##### Task 1.2.1b: Unit tests for `findPRPendingItemForEvent` (~5 min)
- New test function(s) in `session/backlog_lifecycle_pr_test.go` (or a new
  `session/backlog_lifecycle_pr_trigger_test.go` if that file is large —
  check line count first) covering: exact match; no `PrNumber` match;
  `PrNumber` match but repo mismatch (collision case above); zero
  `pr_pending` items.
- Files: `session/backlog_lifecycle_pr_trigger_test.go` (new)

#### Story 1.2.2: `TriggerPRFixForEvent` on `*BacklogLifecycleListener`
**As a** webhook handler, **I want** one call that finds the matching item
and runs its reconciliation immediately, **so that** I don't need to know
anything about `EntRepository`, `ent.BacklogItem`, or the reconciliation
internals.
**Acceptance Criteria**:
- Given the listener is enabled (`l.enabled.Load() == true`) and a
  `pr_pending` item matches `(repoFullName="tstapler/stapler-squad",
  prNumber=189)`, *When* `TriggerPRFixForEvent(ctx,
  "tstapler/stapler-squad", 189)` is called, *Then* it returns `(true,
  nil)` and `reconcilePRPendingItem` has run for that item (verified via a
  fake `prPendingChecker`/`PRFixSpawner` recording the call, same fakes
  `ReconcilePRPending`'s existing tests already use).
- Given the listener is **disabled** (`l.enabled.Load() == false`), *When*
  called with the same inputs, *Then* it returns `(false, nil)` without
  querying `FindPRPendingItems` at all — mirrors `ReconcileStuck`'s
  existing `if !l.enabled.Load() { return }` gate
  (`session/backlog_lifecycle.go:918-920`).
- Given no `pr_pending` item matches, *When* called, *Then* it returns
  `(false, nil)`.
**Files**: `session/backlog_lifecycle_pr.go`

##### Task 1.2.2a: Implement `TriggerPRFixForEvent` (~5 min)
```go
// TriggerPRFixForEvent satisfies services.PRFixEventRouter (defined in the
// consuming package, per .claude/rules/interface-pollution-checklist.md). It
// looks up the pr_pending item tracking (repoFullName, prNumber) and, if
// found, runs the same per-item reconciliation ReconcilePRPending's 60s tick
// would eventually run for it — see ADR-002 for why the full body (merge
// check included) is reused, not just the CI/review-check half.
func (l *BacklogLifecycleListener) TriggerPRFixForEvent(ctx context.Context, repoFullName string, prNumber int) (matched bool, err error) {
    if !l.enabled.Load() {
        return false, nil
    }
    er := l.storage.repo
    item, found := findPRPendingItemForEvent(ctx, er, repoFullName, prNumber)
    if !found {
        return false, nil
    }
    log.InfoLog().Printf("[BacklogLifecycle] TriggerPRFixForEvent item=%s repo=%s pr=%d: reconciling now (webhook-triggered)", item.ID, repoFullName, prNumber)
    l.reconcilePRPendingItem(ctx, er, item)
    return true, nil
}
```
- Files: `session/backlog_lifecycle_pr.go`

##### Task 1.2.2b: Unit tests for `TriggerPRFixForEvent` (~5 min)
- Cover: disabled listener short-circuits before any DB query (assert via a
  fake `EntRepository`/query-counting hook, or by disabling and confirming
  no fake `PRFixSpawner`/`prPendingChecker` call happened); matched item
  reconciles (fake spawner/checker called); no match returns `(false,
  nil)`.
- Files: `session/backlog_lifecycle_pr_trigger_test.go`

---

## Phase 2: Webhook Ingestion (server/services package)

### Epic 2.1: Event-type routing and payload extraction

**Goal**: Give `Handle` a second path for the 4 new event types, gated by
its own flag, with small hand-rolled extractors per event type.

#### Story 2.1.1: `X-GitHub-Event` branching in `Handle`
**As a** webhook receiver, **I want** to read `X-GitHub-Event` and route
accordingly, **so that** the existing `push` path is unaffected and the 4
new event types get a distinct code path.
**Acceptance Criteria**:
- Given `X-GitHub-Event: push`, *When* `Handle` processes the request,
  *Then* behavior is byte-for-byte identical to today (all 9 existing
  tests in `server/services/github_webhook_handler_test.go` pass
  unmodified).
- Given `X-GitHub-Event: check_run` (or `workflow_run`,
  `pull_request_review`, `issue_comment`), *When* `Handle` processes the
  request, *Then* it dispatches to `h.handlePRFixEvent(w, r, payload,
  body, deliveryID)` instead of the push-matching logic, after the same
  `readAndDecodeWebhookBody` prologue.
- Given `X-GitHub-Event: <anything else>` (e.g. `"ping"`), *When* `Handle`
  processes the request, *Then* it returns `200 OK` with no audit row (a
  GitHub `ping` delivery on webhook setup needs no signature verification
  or persistence — matches GitHub's own expectation that `ping` succeeds
  trivially).
**Files**: `server/services/github_webhook_handler.go`

##### Task 2.1.1a: Branch on the header after body decode (~5 min)
- In `Handle`, after `readAndDecodeWebhookBody` succeeds (line 49-52),
  insert:
  ```go
  eventType := r.Header.Get("X-GitHub-Event")
  switch eventType {
  case "", "push":
      // existing logic unchanged, falls through below
  case "check_run", "workflow_run", "pull_request_review", "issue_comment":
      h.handlePRFixEvent(w, r, payload, body, deliveryID, eventType)
      return
  default:
      w.WriteHeader(http.StatusOK)
      return
  }
  ```
  (`""` is kept as a synonym for `"push"` since existing tests, e.g.
  `server/services/github_webhook_handler_test.go`, may not currently set
  the header at all — confirm by reading those tests' request construction
  before finalizing; if they don't set it, this default preserves their
  passing behavior with zero test changes.)
- Files: `server/services/github_webhook_handler.go`

##### Task 2.1.1b: Confirm existing test behavior re: the header (~3 min)
- Read `server/services/github_webhook_handler_test.go`'s request
  construction (all 9 test functions) to confirm whether `X-GitHub-Event`
  is set. If any test doesn't set it, Task 2.1.1a's `"", "push"` case
  keeps it passing; if all tests already set `"push"` explicitly, drop the
  `""` case for a stricter match. Run `go test ./server/services/... -run
  TestGitHubWebhookHandler -v` to confirm.
- Files: none changed (verification only)

#### Story 2.1.2: Payload extractors for the 4 new event types
**As a** webhook handler, **I want** each event type's PR number and
actionability pre-filtered from its own payload shape, **so that**
non-actionable deliveries (e.g. `in_progress` check runs, `approved`
reviews) never reach the DB lookup or `GetPRStatus`.
**Acceptance Criteria**:
- Given a `check_run` payload with `action: "completed"`,
  `check_run.conclusion: "failure"`, `check_run.pull_requests: [{"number":
  189}]`, `repository.full_name: "tstapler/stapler-squad"`, *When*
  `extractCheckRunEvent(payload)` is called, *Then* it returns
  `(repoFullName="tstapler/stapler-squad", prNumbers=[189],
  actionable=true, ok=true)`.
- Given the same payload but `check_run.conclusion: "success"`, *When*
  called, *Then* `actionable=false` (still `ok=true` — the payload parsed
  fine, it's just not worth reacting to).
- Given `action: "in_progress"` (conclusion is `null` per GitHub's
  documented shape), *When* called, *Then* `actionable=false`.
- Given `check_run.pull_requests: []` (fork PR, GitHub's documented
  limitation per `research/stack.md`), *When* called, *Then*
  `prNumbers=[]`, `actionable=false` — nothing to match against, no error.
- Given a `pull_request_review` payload with `action: "submitted"`,
  `review.state: "changes_requested"`, `pull_request.number: 189`, *When*
  `extractPullRequestReviewEvent(payload)` is called, *Then* it returns
  `(repoFullName, prNumbers=[189], actionable=true, ok=true)`.
- Given `review.state: "approved"`, *When* called, *Then*
  `actionable=false`.
- Given an `issue_comment` payload with `action: "created"`,
  `issue.pull_request: {"url": "..."}`  present, `issue.number: 189`,
  *When* `extractIssueCommentEvent(payload)` is called, *Then* it returns
  `(repoFullName, prNumbers=[189], actionable=true, ok=true)`.
- Given `issue.pull_request` absent (a plain issue comment), *When*
  called, *Then* `actionable=false`, `ok=true`.
- Given `repository.full_name` missing entirely (any event type), *When*
  called, *Then* `ok=false` (mirrors `extractGitHubRepoAndBranch`'s
  existing `ok bool` degrade-gracefully contract).
**Files**: `server/services/github_webhook_pr_fix.go` (new)

##### Task 2.1.2a: Pin GitHub's enum values as consts (~3 min)
```go
const (
    ghActionCompleted        = "completed"
    ghActionSubmitted        = "submitted"
    ghActionCreated          = "created"
    ghConclusionFailure      = "failure"
    ghConclusionTimedOut     = "timed_out"
    ghConclusionCancelled    = "cancelled"
    ghConclusionActionRequired = "action_required"
    ghReviewStateChangesRequested = "changes_requested"
    ghReviewStateCommented   = "commented"
)
```
- Files: `server/services/github_webhook_pr_fix.go` (new)

##### Task 2.1.2b: `extractCheckRunEvent` / `extractWorkflowRunEvent` (~5 min)
- Two near-identical extractors (payload key differs: `check_run` vs.
  `workflow_run`), each pulling `action`, `<key>.conclusion`,
  `<key>.pull_requests[].number` (array → `[]int`), and
  `repository.full_name`. `actionable` is true iff `action ==
  ghActionCompleted` and `conclusion` is one of the 4 pinned failure-shaped
  consts.
- Files: `server/services/github_webhook_pr_fix.go` (new)

##### Task 2.1.2c: `extractPullRequestReviewEvent` (~4 min)
- Pulls `action`, `review.state`, `pull_request.number`,
  `repository.full_name`, and (for the actor filter, Story 2.2.1)
  `review.user.login`. `actionable` is true iff `action ==
  ghActionSubmitted` and `state` is `changes_requested` or `commented`.
- Files: `server/services/github_webhook_pr_fix.go` (new)

##### Task 2.1.2d: `extractIssueCommentEvent` (~4 min)
- Pulls `action`, `issue.pull_request` (presence check only), `issue.number`,
  `repository.full_name`, and `comment.user.login` (actor filter). `actionable`
  is true iff `action == ghActionCreated` and `issue.pull_request` is present.
- Files: `server/services/github_webhook_pr_fix.go` (new)

##### Task 2.1.2e: Table-driven tests for all 4 extractors (~5 min per extractor, ~20 min total)
- One `TestExtractCheckRunEvent_should_...` table per extractor, covering
  every case in Story 2.1.2's acceptance criteria plus GitHub's
  non-triggering enum values explicitly named in `research/build-vs-buy.md`
  §4 (`neutral`, `skipped`, `stale`, `cancelled` — note `cancelled` IS
  actionable per the pinned consts above, matching `CIFailing`'s own
  terminal-failure semantics in `session/git/worktree_git.go:500-502`; the
  test table should assert this explicitly so it isn't miscategorized as
  a "should NOT trigger" case during review).
- Files: `server/services/github_webhook_pr_fix_test.go` (new)

#### Story 2.1.3: `pr_event_webhooks` feature flag
**As an** operator, **I want** to enable PR-fix webhook reactions
independently of `webhook_triggers`, **so that** I can dark-launch this
feature without touching the already-stable push path.
**Acceptance Criteria**:
- Given `pr_event_webhooks` is unset (default `false`) and a valid,
  signed `check_run` delivery arrives, *When* `handlePRFixEvent` runs,
  *Then* it returns `200 OK` with no `TriggerFireEvent` row persisted at
  all (not even `no_match` — the flag-off case is a true no-op, consistent
  with the push path's flag check returning `http.NotFound` before any
  processing, `github_webhook_handler.go:40-43`, except here the route is
  already registered for `push` so a `404` would be misleading for a
  different-but-adjacent event type — `200 OK` silent no-op is correct
  per this Story's own framing).
- Given `pr_event_webhooks` is `true`, *When* the same delivery arrives,
  *Then* processing proceeds to the extractor/actionability checks.
**Files**: `server/services/github_webhook_pr_fix.go` (new)

##### Task 2.1.3a: Flag check at the top of `handlePRFixEvent` (~2 min)
```go
if h.cfg == nil || !h.cfg.GetFeatureFlag("pr_event_webhooks") {
    w.WriteHeader(http.StatusOK)
    return
}
```
- Files: `server/services/github_webhook_pr_fix.go` (new)

##### Task 2.1.3b: Update `config/config.go`'s flag doc comment (~2 min)
- Add `pr_event_webhooks` to the "Currently recognized flags" list at
  `config/config.go:1326-1328` (already noted in research as stale/missing
  `webhook_triggers` too — add both while touching this comment, since
  leaving a doc comment more wrong than before found is exactly the kind
  of collateral debt this repo's conventions ask not to walk past).
- Files: `config/config.go`

### Epic 2.2: Actor/bot-loop filtering

**Goal**: Implement ADR-001's self-login filter for `issue_comment`/
`pull_request_review` events.

#### Story 2.2.1: Self-login cache and filter
**As a** webhook handler, **I want** to skip `issue_comment`/
`pull_request_review` events authored by this instance's own GitHub
identity, **so that** `/github:pr-ship`'s own status-update comments never
re-trigger the fix loop.
**Acceptance Criteria**:
- Given this instance's cached login is `"stapler-squad-bot"` and an
  `issue_comment` event has `comment.user.login: "stapler-squad-bot"`,
  *When* `handlePRFixEvent` processes it, *Then* it is treated as
  `actionable=false` regardless of the extractor's own actionability
  result, and a `TriggerFireEvent` with `outcome: "no_match"` is persisted.
- Given `comment.user.login: "some-human-reviewer"` (different from the
  cached self-login), *When* processed, *Then* the self-filter does not
  suppress it.
- Given `github.GetCurrentUserLogin` returns `("", nil)` (unauthenticated)
  or errors, *When* any `issue_comment`/`pull_request_review` event
  arrives, *Then* the self-filter does not suppress anything (fails open
  per ADR-001) and a `Warn`-level log line is emitted once per cache
  refresh (not once per event).
**Files**: `server/services/github_webhook_pr_fix.go` (new)

##### Task 2.2.1a: `selfLoginCache` type (~5 min)
- A small struct: `mu sync.RWMutex`, `login string`, `fetchedAt time.Time`,
  `ttl time.Duration` (e.g. 5 minutes, matching `user_pr_cache.go`'s
  `LoginCacheTTL` order of magnitude). Method `Get(ctx context.Context)
  string` — returns the cached login if fresh, else calls
  `github.GetCurrentUserLogin(ctx)`, logs a `Warn` on error, caches the
  result (including `""`) with a fresh `fetchedAt`.
- Files: `server/services/github_webhook_pr_fix.go` (new)

##### Task 2.2.1b: Wire the cache into `GitHubWebhookHandler` (~3 min)
- Add field `selfLogin *selfLoginCache` to `GitHubWebhookHandler`,
  initialized in `NewGitHubWebhookHandler` (Story 2.3.1 touches this
  constructor anyway — combine the two field additions in one edit to
  `github_webhook_handler.go`).
- Files: `server/services/github_webhook_handler.go`

##### Task 2.2.1c: Apply the filter in `handlePRFixEvent` (~3 min)
- For `issue_comment`/`pull_request_review` only: after extraction,
  compare `strings.EqualFold(actorLogin, h.selfLogin.Get(ctx))`; if equal,
  force `actionable = false`.
- Files: `server/services/github_webhook_pr_fix.go` (new)

##### Task 2.2.1d: Unit tests (~6 min)
- Cover: self-login match suppresses; non-match doesn't; empty/error
  cached login doesn't suppress anything (fail-open); `check_run`/
  `workflow_run` events are never passed through this filter at all
  (assert by confirming the filter function isn't even called for those
  event types, or that CI-completion actionability is determined solely by
  Task 2.1.2b's logic).
- Files: `server/services/github_webhook_pr_fix_test.go`

### Epic 2.3: `PRFixEventRouter` interface, wiring, and audit trail

**Goal**: Connect the extractor/filter pipeline to
`session.BacklogLifecycleListener.TriggerPRFixForEvent`, and persist the
outcome.

#### Story 2.3.1: `PRFixEventRouter` interface + constructor wiring
**As a** maintainer, **I want** `GitHubWebhookHandler` to depend on a
narrow interface rather than a concrete `*session.BacklogLifecycleListener`
or the raw `session.PRFixSpawner`, **so that** tests can fake it and the
dependency respects this repo's interface-pollution convention.
**Acceptance Criteria**:
- `PRFixEventRouter` is defined in `server/services` (the consumer) with
  exactly one method: `TriggerPRFixForEvent(ctx context.Context,
  repoFullName string, prNumber int) (matched bool, err error)`.
- `NewGitHubWebhookHandler`'s signature gains one parameter,
  `prFixRouter PRFixEventRouter`, stored on a new field `prFixRouter` on
  `GitHubWebhookHandler`.
- `*session.BacklogLifecycleListener` satisfies `PRFixEventRouter` with no
  further changes (structural typing — Task 1.2.2a's method signature
  already matches).
**Files**: `server/services/github_webhook_handler.go`,
`server/services/github_webhook_handler_test.go`

##### Task 2.3.1a: Define `PRFixEventRouter` and extend the struct/constructor (~4 min)
```go
// PRFixEventRouter is satisfied by *session.BacklogLifecycleListener.
// Defined here (the consumer), per .claude/rules/interface-pollution-checklist.md.
type PRFixEventRouter interface {
    TriggerPRFixForEvent(ctx context.Context, repoFullName string, prNumber int) (matched bool, err error)
}
```
- Add `prFixRouter PRFixEventRouter` and `selfLogin *selfLoginCache`
  (Task 2.2.1b) fields to `GitHubWebhookHandler`; extend
  `NewGitHubWebhookHandler`'s parameter list; initialize `selfLogin` inside
  the constructor.
- Files: `server/services/github_webhook_handler.go`

##### Task 2.3.1b: Update all existing constructor call sites (~5 min)
- `server/server.go:764` (Story 3.1.1 handles the actual value passed) and
  all 9 test functions in `server/services/github_webhook_handler_test.go`
  need one more argument — pass `nil` in tests that don't exercise the new
  event types (a `nil` interface is fine since those tests only send
  `push` events, which never reach `handlePRFixEvent`).
- Files: `server/services/github_webhook_handler_test.go`

#### Story 2.3.2: `handlePRFixEvent` dispatch + outcome persistence
**As a** webhook handler, **I want** one method that ties extraction,
filtering, lookup, and audit-trail persistence together, **so that**
`Handle`'s new branch (Task 2.1.1a) has a single, testable entry point.
**Acceptance Criteria**:
- Given a valid, actionable, signed `check_run` delivery for a repo/PR
  this instance tracks, *When* `handlePRFixEvent` runs, *Then* it calls
  `h.prFixRouter.TriggerPRFixForEvent(ctx, "tstapler/stapler-squad", 189)`
  exactly once and persists `outcome: "fired_success"`.
- Given the same delivery but for a PR this instance does not track,
  *When* run, *Then* `TriggerPRFixForEvent` is still called (it's the
  thing that determines "not tracked"), returns `(false, nil)`, and
  `outcome: "no_match"` is persisted.
- Given a non-actionable delivery (e.g. `conclusion: "success"`), *When*
  run, *Then* `TriggerPRFixForEvent` is **never called** (short-circuit
  before the DB lookup) and `outcome: "no_match"` is persisted.
- Given `h.prFixRouter == nil` (misconfigured wiring), *When* an
  otherwise-actionable delivery arrives, *Then* `outcome: "fired_failed"`
  is persisted with `ErrorMessage: "no PRFixEventRouter configured"`, and
  the handler still returns `200 OK` (never a 5xx for a router-wiring gap
  — matches `renderAndFireTrigger`'s existing `scheduler == nil` handling
  shape, `webhook_trigger_common.go:134-139`).
- Signature verification for the 4 new event types reuses the exact same
  per-candidate loop the `push` path uses (same `repoCandidates`/
  `VerifyGitHubSignature` logic) — this is verified by Task 2.3.2's
  reuse of the existing helper, not duplicated code (see Pattern
  Decisions, "Alternative B rejected").
**Files**: `server/services/github_webhook_pr_fix.go` (new)

##### Task 2.3.2a: Implement `handlePRFixEvent`'s signature-verification prologue (~5 min)
- Mirror `Handle`'s existing `repoCandidates`/signature-loop
  (`github_webhook_handler.go:63-117`) exactly, factored so both `Handle`
  and `handlePRFixEvent` call one shared helper (e.g.
  `verifySignatureForRepo(ctx, h.repo, h.cfg, fullName, body,
  sigHeader) (verified bool)`) rather than copy-pasting the loop — this
  keeps Alternative B's rejected duplication out of the implementation
  even though the two methods live in different files now.
- Files: `server/services/github_webhook_handler.go` (extract the shared
  helper), `server/services/github_webhook_pr_fix.go` (new, calls it)

##### Task 2.3.2b: Implement the extract → filter → route → persist body (~5 min)
```go
func (h *GitHubWebhookHandler) handlePRFixEvent(w http.ResponseWriter, r *http.Request, payload map[string]interface{}, body []byte, deliveryID, eventType string) {
    ctx := r.Context()
    if h.cfg == nil || !h.cfg.GetFeatureFlag("pr_event_webhooks") {
        w.WriteHeader(http.StatusOK)
        return
    }
    fullName, prNumbers, actionable, ok := extractPRFixEvent(eventType, payload) // dispatches to the 4 extractors from Task 2.1.2
    if !ok {
        persistTriggerFireEvent(ctx, h.fireEvents, session.TriggerFireEventInput{Outcome: "rejected", DeliveryID: deliveryID, ErrorMessage: "missing repository.full_name"})
        http.Error(w, "missing repository.full_name", http.StatusBadRequest)
        return
    }
    if actionable && (eventType == "issue_comment" || eventType == "pull_request_review") {
        actorLogin := extractActorLogin(eventType, payload)
        if actorLogin != "" && strings.EqualFold(actorLogin, h.selfLogin.Get(ctx)) {
            actionable = false
        }
    }
    if !actionable || len(prNumbers) == 0 {
        persistTriggerFireEvent(ctx, h.fireEvents, session.TriggerFireEventInput{Outcome: "no_match", DeliveryID: deliveryID})
        w.WriteHeader(http.StatusOK)
        return
    }
    if !h.verifySignatureForRepo(ctx, fullName, body, r.Header.Get("X-Hub-Signature-256")) {
        persistTriggerFireEvent(ctx, h.fireEvents, session.TriggerFireEventInput{Outcome: "rejected", DeliveryID: deliveryID, ErrorMessage: "invalid signature"})
        http.Error(w, "invalid signature", http.StatusUnauthorized)
        return
    }
    if h.prFixRouter == nil {
        persistTriggerFireEvent(ctx, h.fireEvents, session.TriggerFireEventInput{Outcome: "fired_failed", DeliveryID: deliveryID, ErrorMessage: "no PRFixEventRouter configured"})
        w.WriteHeader(http.StatusOK)
        return
    }
    for _, prNumber := range prNumbers {
        matched, err := h.prFixRouter.TriggerPRFixForEvent(ctx, fullName, prNumber)
        outcome := "no_match"
        errMsg := ""
        if err != nil {
            outcome, errMsg = "fired_failed", err.Error()
        } else if matched {
            outcome = "fired_success"
        }
        persistTriggerFireEvent(ctx, h.fireEvents, session.TriggerFireEventInput{Outcome: outcome, DeliveryID: deliveryID, ErrorMessage: errMsg})
    }
    w.WriteHeader(http.StatusOK)
}
```
  (Note: signature verification is deliberately checked *after* the
  actionability short-circuit — an unsigned, non-actionable request costs
  nothing beyond a map lookup, matching this Story's "no_match" acceptance
  criterion; a signature check would otherwise run on every single
  `in_progress`/`success` check-run delivery, which per `pitfalls.md` §2 is
  the dominant volume.)
  **Engineering re-review follow-up (primitive-obsession, low-priority)**:
  `handlePRFixEvent`'s `deliveryID, eventType string` are two adjacent
  same-typed params representing distinct concepts
  (`.claude/rules/primitive-obsession-checklist.md`). Single call site
  (Task 2.1.1a) keeps the swap risk low, but if a second call site is ever
  added, prefer bundling them into a small unexported struct (e.g.
  `webhookDelivery{ID, EventType string}`) rather than growing the bare
  parameter list further — not required for this pass's single-caller shape.
- Files: `server/services/github_webhook_pr_fix.go` (new)

##### Task 2.3.2c: Tests for `handlePRFixEvent`'s outcome persistence (~8 min)
- Table-driven, one row per Story 2.3.2 acceptance criterion, using a fake
  `PRFixEventRouter` (records calls, returns configurable
  `matched`/`err`) and the existing in-memory/fake `TriggerFireEventRepository`
  test double already used by `github_webhook_handler_test.go`.
- Files: `server/services/github_webhook_pr_fix_test.go`

---

## Phase 3: Dependency Wiring, Reachability Docs, and Test Matrix

### Epic 3.1: Expose `BacklogLifecycleListener` on the dependency graph

**Goal**: Close the gap `research/architecture.md` §3 flags —
`backlogLifecycleListener` is a local variable inside `BuildRuntimeDeps`
today, not reachable from `server.go`'s webhook-route-registration block.

#### Story 3.1.1: Thread `BacklogLifecycleListener` through the dependency structs
**As a** server bootstrap, **I want** `deps.BacklogLifecycleListener`
available at route-registration time, **so that**
`NewGitHubWebhookHandler` can be given a real `PRFixEventRouter`.
**Acceptance Criteria**:
- Given a normal server boot, *When* `BuildDependencies()` runs, *Then*
  `ServerDependencies.BacklogLifecycleListener` is non-nil and is the same
  object `SetPRFixSpawner`/`SetAutoReopener`/etc. were wired onto
  (`server/dependencies.go:1180-1182`).
- Given `server.go`'s webhook-registration block runs with
  `webhook_triggers` enabled, *When* it constructs
  `NewGitHubWebhookHandler(...)`, *Then* it passes
  `deps.BacklogLifecycleListener` as the `PRFixEventRouter` argument (a
  `*session.BacklogLifecycleListener` satisfies the interface structurally
  — no explicit cast needed).
**Files**: `server/dependencies.go`, `server/server.go`

##### Task 3.1.1a: Add the field to `RuntimeDeps` and `ServerDependencies` (~3 min)
- `RuntimeDeps` (`server/dependencies.go:404+`, near `TriggerFireEventRepo`):
  add `BacklogLifecycleListener *session.BacklogLifecycleListener`.
- `ServerDependencies` (`server/dependencies.go:36+`, near
  `TriggerFireEventRepo`): add the identical field.
- Files: `server/dependencies.go`

##### Task 3.1.1b: Populate the field in `BuildRuntimeDeps`'s return and `ToServerDeps` (~3 min)
- `BuildRuntimeDeps`'s `return &RuntimeDeps{...}` (`server/dependencies.go:1453+`):
  add `BacklogLifecycleListener: backlogLifecycleListener,`.
- `(rt *RuntimeDeps) ToServerDeps()` (`server/dependencies.go:126+`): add
  `BacklogLifecycleListener: rt.BacklogLifecycleListener,`.
- Files: `server/dependencies.go`

##### Task 3.1.1c: Pass it into `NewGitHubWebhookHandler` in `server.go` (~3 min)
- `server/server.go:764`: change
  `services.NewGitHubWebhookHandler(deps.WorkflowRepo, deps.WorkflowScheduler, deps.TriggerFireEventRepo, webhookCfg)`
  to also pass `deps.BacklogLifecycleListener` as the new
  `PRFixEventRouter` parameter (order per Task 2.3.1a's constructor
  signature — place it last to minimize diff noise at existing call
  sites, consistent with Go convention of appending new params rather than
  inserting mid-list).
- Files: `server/server.go`

##### Task 3.1.1d: Build + targeted test run (~3 min)
- `go build ./... && go test ./server/... ./session/... -run
  'GitHubWebhookHandler|BacklogLifecycleListener|ReconcilePRPending' -v`
- Files: none changed (verification only)

### Epic 3.2: Public reachability documentation

**Goal**: Document the path-scoped tunnel/reverse-proxy pattern for
`/webhooks/github`, extending the Slack Phase 2 precedent rather than
inventing a new one (Goal 4, `research/build-vs-buy.md` §2).

#### Story 3.2.1: `/webhooks/github` reachability doc
**As an** operator, **I want** a documented, path-scoped way to expose
`/webhooks/github` to the internet, **so that** I don't naively tunnel the
whole `:8543` port and expose the ConnectRPC session API alongside it.
**Acceptance Criteria**:
- A new doc exists cross-referencing
  `.claude/docs/slack-phase2-public-reachability.md`'s pattern, with an
  `nginx` `location = /webhooks/github { proxy_pass http://127.0.0.1:8543;
  }` block analogous to that doc's existing
  `location = /api/hooks/slack-interactive` block, plus a checklist
  covering: path-scoping verification, `pr_event_webhooks` flag state,
  `webhook_triggers` flag + restart requirement (route registration is
  boot-time only), the GitHub repo webhook's configured URL/secret, and
  the operational prerequisite flagged in Unresolved Questions (a
  `github_push`-type `Workflow` row must exist for the repo, since that's
  the only secret source the signature-verification loop checks).
  - *Given* an operator running both Slack Phase 2 and this feature, *When*
    they follow the new doc, *Then* they add one more `location` block to
    the *same* nginx config already serving `/api/hooks/slack-interactive`
    (per `research/build-vs-buy.md` §2's "one tunnel process, one nginx
    instance, two scoped paths" recommendation) rather than standing up a
    second proxy.
**Files**: `.claude/docs/github-webhook-public-reachability.md` (new)

##### Task 3.2.1a: Write the new doc (~5 min)
- Mirror `.claude/docs/slack-phase2-public-reachability.md`'s structure
  (why this route differs from localhost-only `/api/hooks/*`; "do not
  tunnel the whole port"; ngrok-via-local-nginx example; direct
  reverse-proxy example; checklist) with `/webhooks/github` substituted
  for `/api/hooks/slack-interactive`, and an explicit note that `push` and
  the 4 new PR-fix event types share this one path/config — no per-event
  path-scoping needed since GitHub multiplexes event type via header, not
  URL. Mention `smee.io` as a dev-only convenience per
  `research/build-vs-buy.md` §2, not the production guidance.
- Files: `.claude/docs/github-webhook-public-reachability.md` (new)

##### Task 3.2.1b: Add the doc to CLAUDE.md's Reference Documents Index (~2 min)
- Add a row to the table in `/home/tstapler/.stapler-squad/workspaces/d685c4b1a423cca3/worktrees/triage-1f6150ad-1eef-481a-b16a-76153b037762_18ce7db91956f4b0/CLAUDE.md`'s "Reference Documents Index"
  section, alongside the existing "Slack Phase 2 interactive-approvals
  public reachability" row, matching that row's format.
- Files: `CLAUDE.md`

### Epic 3.3: Test coverage matrix

**Goal**: Fill in the remaining coverage `pitfalls.md`/`build-vs-buy.md`
call for explicitly, beyond what Stories 1.2.1/1.2.2/2.1.2/2.2.1/2.3.2
already specify.

#### Story 3.3.1: Empty-secret-fails-closed, extended to the new event types
**As a** security reviewer, **I want** confirmation that an empty/missing
webhook secret fails closed for `check_run`/`workflow_run`/
`pull_request_review`/`issue_comment` exactly as it does for `push`,
**so that** the fail-closed property isn't silently scoped to only the
original event type.
**Acceptance Criteria**:
- Given a `github_push`-type `Workflow` row with
  `WebhookSecretEncrypted: ""` for the matching repo, and a `check_run`
  delivery with `action: "completed"`, `conclusion: "failure"`, *When*
  `handlePRFixEvent` runs, *Then* it returns `401` and persists
  `outcome: "rejected"` — the same `decryptWorkflowSecret` early-return
  (`webhook_trigger_common.go:30-32`) this handler's shared
  `verifySignatureForRepo` helper (Task 2.3.2a) reuses.
**Files**: `server/services/github_webhook_pr_fix_test.go`

##### Task 3.3.1a: Add the test (~4 min)
- One test function,
  `TestHandlePRFixEvent_should_Return401AndRecordRejected_When_WorkflowSecretEmpty`,
  reusing the existing `push` test's fake-Workflow-repo setup pattern from
  `github_webhook_handler_test.go`.
- **BUG-076 defensiveness note**: `TestCreateWorkflow_WebhookSecret_RoundTripsThroughHMACVerification`
  is a confirmed open flake (`docs/bugs/open/BUG-076-flaky-testcreateworkflow-webhooksecret-full-suite-only.md`)
  in this same `decryptWorkflowSecret`/webhook-secret path — it fails only in
  the full `server/services` suite, never in isolation, hypothesized to be
  shared/global config or encryption-key-resolution state (`infra.cfg`)
  mutated by a preceding test rather than an HMAC-logic bug itself. This new
  test should build its own isolated, per-test config/encryption-key fixture
  (e.g. `t.TempDir()`-scoped, not a package-level shared one) rather than
  reuse a shared `infra.cfg`, so it doesn't inherit the same intermittent
  failure.
- Files: `server/services/github_webhook_pr_fix_test.go`

#### Story 3.3.2: CAS/duplicate-delivery behavior
**As a** reviewer, **I want** confirmation that a burst of duplicate
`check_run` deliveries for the same PR doesn't spawn duplicate fix
sessions, **so that** the design's reliance on `RemediationDue` (Pattern
Decisions table) is verified, not just asserted in prose.
**Acceptance Criteria**:
- Given two concurrent `TriggerPRFixForEvent` calls for the same
  `(repoFullName, prNumber)` racing against each other, *When* both run,
  *Then* at most one results in an actual `AutoReopenForPRFix` call
  succeeding past the CAS transition — verified by reusing the existing
  regression tests already cited in `research/pitfalls.md` §2
  (`TestTransitionBacklogItemStatus_should_letExactlyOneWinnerThrough_When_TwoWritersRaceConcurrently`,
  `session/ent_repository_backlog_transition_test.go`;
  `TestTransitionBacklogItemStatus_should_FailCASForLoser_When_ConcurrentOverrideRaces`,
  `server/services/backlog_service_lifecycle_test.go`) — this Story adds
  **no new CAS test**, it confirms via a short review pass that
  `TriggerPRFixForEvent`'s call path (`reconcilePRPendingItem` →
  `remediatePRFixWithBackoffGate` → `AutoReopenForPRFix`) is the exact
  same call path those existing tests already cover, so no new coverage
  gap exists.
**Files**: none (review-only story)

##### Task 3.3.2a: Confirm call-path identity (~3 min)
- Read `session/ent_repository_backlog_transition_test.go`'s and
  `server/services/backlog_service_lifecycle_test.go`'s test setup to
  confirm they exercise `AutoReopenForPRFix` (or the same
  `TransitionBacklogItemStatus` precondition path) directly — not a
  different transition helper — so this Story's claim holds.
- Files: none changed (verification only)

### Epic 3.4: Explicit non-change

#### Story 3.4.1: `PRStatusPoller` remains unmodified
**As a** reviewer, **I want** it explicit in the plan (not just implied)
that `PRStatusPoller` is untouched, **so that** Goal 3's "keep the poller
as backstop" requirement is checkable against the diff.
**Acceptance Criteria**:
- The diff for this feature contains **zero** changes to
  `session/pr_status_poller.go`. `PRStatusPoller` and
  `BacklogLifecycleListener`/`ReconcilePRPending` are, and remain,
  independent (confirmed during research: `PRStatusPoller` never calls
  `ReconcilePRPending` and has no reference to `EntRepository`/
  `BacklogLifecycleListener` today) — this feature adds a second,
  independent trigger path into the same eventual `AutoReopenForPRFix`
  target, it does not modify or replace the first.
**Files**: none (explicit non-task — do not touch `session/pr_status_poller.go`)
