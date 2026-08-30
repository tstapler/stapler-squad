# Architecture Research: pr-event-webhooks

## Prior art incorporated (cited, not re-derived)

`project_plans/webhook-triggers/research/architecture.md` already covers, and is not
re-derived here:

- `GitHubWebhookHandler`'s structure and constructor (`server/services/github_webhook_handler.go:14-32`,
  cited in that doc's §1) — concrete type, not an interface, injected with `repo`/`scheduler`/
  `fireEvents`/`cfg`.
- HMAC verification (`crypto/hmac`/`crypto/sha256` via `VerifyGitHubSignature`,
  `server/services/webhook_signature.go:15-24`) is genuinely new code with no prior in-repo
  precedent — that doc's §1 already established this; still true here, no new verifier needed.
- The `TriggerFireEvent` audit-trail model (`session/ent/schema/trigger_fire_event.go`) and its
  Event-Command-Policy table (that doc, lines 302-315) for the `push`→session-creation flow.
- Route registration trust-boundary framing (that doc's §1 table) — `/webhooks/github` sits
  alongside `/api/hooks/permission-request` as an external-POST-verify-signature-first route.

This item extends the **same** `GitHubWebhookHandler`/`/webhooks/github` surface with four new
GitHub event types (`check_run`, `workflow_run`, `pull_request_review`, `issue_comment`) that
route to `session.PRFixSpawner`, not to `Scheduler.FireTrigger`/session creation. The two flows
share a route and a signature-verification loop but diverge completely after that point — this
doc covers only that divergence and the new questions it raises.

## 1. Integration point — how does an event reach `AutoReopenForPRFix`?

### `PRFixSpawner`'s actual shape

`session/backlog_lifecycle_pr.go:18-23`:

```go
type PRFixSpawner interface {
	AutoReopenForPRFix(ctx context.Context, itemID string, fixContext string) error
}
```

One method, two arguments the caller must supply: `itemID` (which `pr_pending` `BacklogItem` to
reopen) and `fixContext` (free text prepended to the item's notes before the fix session spawns —
`server/services/backlog_service_triage.go:2097-2109`). `*BacklogService` implements it at
`server/services/backlog_service_triage.go:2018-2138`.

**`AutoReopenAfterFailedReview` is not a fit for this feature — this resolves requirements.md's
open question.** It implements a *different* interface, `session.AutoReopenSpawner`
(`session/backlog_lifecycle_review.go:21-25`, `AutoReopenAfterFailedReview(ctx, itemID) error` —
no `fixContext` parameter at all), and its precondition is `item.Status ==
BacklogStatusReview` (`server/services/backlog_service_triage.go:1780-1785`), not `pr_pending`.
It reopens an item after this repo's *own internal* review-verdict cycle (a review session calling
`submit_review_verdict` with FAIL — see `taskProtocolBlock`), which is unrelated to a GitHub PR's
CI/review state. A GitHub `pull_request_review`/`issue_comment` event on an already-shipped
`pr_pending` item has no way to reach `AutoReopenAfterFailedReview` at all (wrong precondition,
wrong interface, no `fixContext` slot to carry the review comment into) — **all four new GitHub
event types funnel into `PRFixSpawner.AutoReopenForPRFix` only.**

### The existing caller already does exactly this job — reuse its shape, not its signature

`ReconcilePRPending` (`session/backlog_lifecycle_pr.go:1235-1541`, the `PRStatusPoller`'s
60s-tick consumer) is the existing, working template:

1. For each `pr_pending` item with `PrNumber > 0`, call `g.GetPRStatus(item.PrNumber)`
   (`session/backlog_lifecycle_pr.go:1355`) — a live `gh pr view` call returning
   `*git.PRStatus{CIFailing, HasBlockingReviews, HasConflicts, IsClosed, HasReviewFeedback,
   LatestFeedbackAt, FeedbackText, ...}` (`session/git/worktree_git.go:500+`).
2. Build `fixCtx` from that status: `fmt.Sprintf("PR #%d (%s) needs fixes:\n\n%s", item.PrNumber,
   item.PrURL, prStatus.FeedbackText)` (`session/backlog_lifecycle_pr.go:1517`), or the
   closed-without-merging variant (`:1396`).
3. Call through `remediatePRFixWithBackoffGate` (**not** `fixSpawner.AutoReopenForPRFix`
   directly — see §4), which wraps the spawn with a stuck-row + backoff gate.

**Recommendation for the webhook path: don't parse per-event-type payload fields into
`fixContext` text at all.** `check_run`/`workflow_run`/`pull_request_review`/`issue_comment`
payloads have four different shapes, and hand-building `fixContext` prose per shape would
duplicate `GetPRStatus`'s existing CI/review/conflict synthesis (`session/git/worktree_git.go`)
with a second, less complete implementation (payload alone doesn't tell you about *other*
in-flight checks, blocking reviews from *other* reviewers, or conflict status — `GetPRStatus`
does one `gh pr view` call that aggregates all of it). Instead: an inbound event of any of the
four types should be treated purely as **"something changed on PR #N in repo R — re-check it
now instead of waiting up to `PollInterval` (default 60s)."** The webhook handler's job reduces
to (a) verify signature, (b) extract `repository.full_name` + a PR number from the event payload
(§2), (c) find the matching `pr_pending` item, (d) call the **same** `GetPRStatus` +
`fixCtx`-building + `remediatePRFixWithBackoffGate` logic `ReconcilePRPending` already runs for
that one item, immediately instead of on the next tick. This is a *single-item, on-demand
invocation of `ReconcilePRPending`'s existing per-item body*, not a new decision engine.

Concretely: extract the per-item logic inside `ReconcilePRPending`'s loop (lines
~1355-1541, everything after `g := l.getPRPendingCheckerFactory()(repoPath)`) into an
already-latent unit (it is already a self-contained block operating on one `item`/`g` pair) and
give it a second caller. This is additive to `session/backlog_lifecycle_pr.go`, not a new package.

## 2. PR-to-item lookup — no new index needed, but no ready-made query either

`BacklogItem`'s ent schema already carries the fields needed
(`session/ent/schema/backlog_item.go:37,92,94`): `repo_path` (string, local filesystem path),
`pr_url` (string), `pr_number` (int). `FindPRPendingItems`
(`session/storage_backlog.go:980-991`) already queries exactly the candidate set:

```go
r.client.BacklogItem.Query().Where(
    backlogitem.Status(string(BacklogStatusPRPending)),
    backlogitem.PrNumberGT(0),
).All(ctx)
```

**This is the same candidate set `ReconcilePRPending` iterates every tick** — there is no
separate "PR→session→item" index; `PRStatusPoller`/`ReconcilePRPending` associate a PR with an
item directly via the item's own `PrNumber`/`RepoPath` fields, not through session state. So the
webhook path can reuse `FindPRPendingItems` verbatim.

**What's missing**: a query/filter keyed by `(repoFullName, prNumber)` — `FindPRPendingItems`
returns every `pr_pending` item across all repos, and `RepoPath` is a local filesystem path, not
a GitHub `owner/repo` string, so matching a webhook's `repository.full_name` requires resolving
each candidate's local repo path to its GitHub identity. **This resolution already exists**:
`github.GetOwnerRepoFromRemote(repoPath)` (used by `defaultPRByNumberFinder`,
`session/backlog_lifecycle_pr.go:134-143`, and `defaultOrphanedPRFinder`, `:118-127`) parses the
git remote — a local, no-API-call operation, unlike the *verification* helpers
(`verifyPRHeadBranchMatchesTracked`, `:1572-1581`) which additionally hit the GitHub API via
`GetPRByNumber`. For matching (not verifying) purposes, the cheap local-only resolution is enough:
filter `FindPRPendingItems`'s results in Go by `item.PrNumber == extractedPRNumber`, then confirm
`GetOwnerRepoFromRemote(item.RepoPath)`'s `owner/repo` equals the webhook's `repository.full_name`
— an in-process linear scan over a normally-small `pr_pending` set, exactly the shape
`ReconcilePRPending`'s own loop and `closeIfSupersededByMain` already use elsewhere in this file
for "iterate the small pr_pending set and check something." No new ent index or migration is
needed; this is a small new Go function (e.g. `findPRPendingItemForEvent(ctx, repoFullName string,
prNumber int) (*ent.BacklogItem, bool)`), not a new query capability.

### PR-number extraction per event type

All four target event types carry a PR number, but at different payload paths (GitHub's own
webhook shapes, not something to re-derive from this repo's code):

| Event | PR number path | Notes |
|---|---|---|
| `check_run` | `check_run.pull_requests[].number` | Array — a check run on a commit can be associated with 0+ open PRs; only fires meaningfully when non-empty. |
| `workflow_run` | `workflow_run.pull_requests[].number` | Same array shape as `check_run`. |
| `pull_request_review` | `pull_request.number` | Single value. |
| `issue_comment` | `issue.number`, gated on `issue.pull_request` being present | GitHub fires `issue_comment` for both issues and PR conversations; a plain issue comment must be ignored (no `issue.pull_request` key). |

`repository.full_name` is present at the same top-level path in all four (as it already is for
`push`, per `extractGitHubRepoAndBranch`, `server/services/github_webhook_handler.go:135-150`) —
a sibling extractor per event type, following that function's exact pattern, is the natural shape
rather than one generic parser.

## 3. Where the new routing logic lives

### The signature-verification loop has no natural secret source for this event type

The existing loop verifies each repo-matching **`github_push`-type `Workflow`'s own secret**
(`server/services/github_webhook_handler.go:93-117`) — that secret exists because a `Workflow`
row is what gets fired. For `check_run`/`workflow_run`/`pull_request_review`/`issue_comment`,
there is no `Workflow` being fired — the target is a `pr_pending` `BacklogItem`, which has no
secret field of its own. **In practice this is not a gap**: a GitHub repo's webhook
configuration is one subscription with one shared secret covering every event type it's
subscribed to, all delivered to the same URL — so the *same* secret already stored on that
repo's `github_push`-type `Workflow` row(s) is the correct secret to verify **all** event types
against, including these four. This only breaks down if a repo wants PR-fix reactions but has
never configured a `github_push` trigger for it (no `Workflow` row exists at all, hence no stored
secret) — worth flagging to planning as a real configuration gap (either require a `github_push`
Workflow to exist per repo as a prerequisite, documented as such, or add a repo-scoped secret
storage independent of `Workflow` — the latter duplicates a secret-storage shape for no clear
benefit given the "one webhook subscription per repo" reality).

### Recommendation: same struct, same route, new injected dependency — not raw `PRFixSpawner`

Following `.claude/rules/interface-pollution-checklist.md` (define interfaces at the consumer,
avoid a second forwarding-only type) and the existing `NewGitHubWebhookHandler` constructor
pattern (`server/services/github_webhook_handler.go:25-27`):

- **(a) is right, with a caveat on which interface.** Extend the same `GitHubWebhookHandler`
  struct/constructor/route (`POST /webhooks/github` already receives every subscribed event type
  GitHub sends — nothing about the route itself is push-specific, only today's `Handle` body is).
  `Handle` should branch on the `X-GitHub-Event` header: `"push"` keeps today's path exactly;
  `"check_run"`, `"workflow_run"`, `"pull_request_review"`, `"issue_comment"` go to a new
  sibling method (e.g. `handlePRFixEvent`) after the same signature-verification loop.
  A **sibling handler type (b) is the wrong shape**: it would duplicate the entire
  signature-verification-per-repo-candidate loop (lines 93-117) verbatim, which is exactly the
  kind of struct-wraps-struct/forwarding duplication the checklist flags — there is no
  behavioral reason for the CI-reaction path to re-derive "which repo-matching Workflow's secret
  verifies this body" differently than the push path does.
- **Do not inject raw `session.PRFixSpawner`** into `GitHubWebhookHandler`. `PRFixSpawner`'s
  single method (`AutoReopenForPRFix`) has no backoff/dedup gate of its own — calling it directly
  bypasses `remediatePRFixWithBackoffGate`'s stuck-row + backoff protection that
  `ReconcilePRPending` always goes through (see §4, this is the actual reason (a) needs a
  caveat). Define a **narrower, consumer-scoped interface** in `server/services`, satisfied by
  `*session.BacklogLifecycleListener` once it gains a new exported method (§4):

  ```go
  // PRFixEventRouter is satisfied by *session.BacklogLifecycleListener. Defined here
  // (the consumer) per .claude/rules/interface-pollution-checklist.md.
  type PRFixEventRouter interface {
      TriggerPRFixForEvent(ctx context.Context, repoFullName string, prNumber int) (matched bool, err error)
  }
  ```

  `TriggerPRFixForEvent` takes only `repoFullName`/`prNumber` — no `fixContext` parameter — since
  per §1, the fix-context text is derived internally from a fresh `GetPRStatus` call, not from
  webhook payload parsing. `GitHubWebhookHandler` gains one new field (`prFixRouter
  PRFixEventRouter`), wired in `server/server.go`'s existing construction block
  (`:764`) once `ServerDependencies` exposes `backlogLifecycleListener` (today it's a local
  variable inside `server/dependencies.go`'s constructor, line 658 — not yet on the
  `ServerDependencies` struct at all; `WorkflowRepo`/`WorkflowScheduler`/`TriggerFireEventRepo`
  are the only webhook-relevant fields currently exposed there, lines 102-111/461-470). Adding a
  `BacklogLifecycleListener` (or a narrower interface over it) field to `ServerDependencies` is a
  small, mechanical addition — the object already exists and is fully constructed by the time
  the webhook-route block runs (line 658 vs. line 748 for route registration).

### Feature flag

Goal 5 asks for "the same (or a sibling) flag." Recommend a **sibling flag** (e.g.
`pr_event_webhooks`), checked only inside the new `check_run`/`workflow_run`/
`pull_request_review`/`issue_comment` branch of `Handle` — **not** at route-registration time
(that stays gated on `webhook_triggers`, since the route itself, and the `push` handling, must
keep working independently). This lets an operator disable CI-reaction webhooks without
disabling `push`-triggered session creation and vice versa, mirroring the "defense in depth,
handler re-checks its own flag" pattern the `push` path already established
(`server/services/github_webhook_handler.go:38-43`) — the new branch does the identical
self-check with its own flag name before doing anything else.

## 4. Race safety — the active-session guard helps, but is not sufficient alone; reuse the existing backoff gate

Two independent layers already exist in this codebase, and the webhook path needs to go through
**both**, not just the first:

1. **`AutoReopenForPRFix`'s own active-work-session guard**
   (`server/services/backlog_service_triage.go:2048-2051`): if a work session is already live
   for the item, the call is a documented no-op (`notifyRespawnBlockedByActiveSession`, returns
   `nil`). This makes a webhook firing *while a fix session from a prior trigger is still
   running* safe — no double-spawn, no error. This covers the "poller and webhook fire at the
   literal same instant for the same still-being-fixed PR" case.
2. **`remediatePRFixWithBackoffGate`'s stuck-row + backoff gate**
   (`session/backlog_lifecycle_pr.go:1154-1222`, doc comment explains the exact bug this fixed:
   "ReconcilePRPending's CI-failing/blocked-review/conflict branch ... called
   `AutoReopenForPRFix` directly on every ~60s reconciliation tick with no backoff ... a PR that
   keeps failing CI could get a fresh fix session respawned indefinitely"). This is the layer (1)
   does **not** provide: once a spawned fix session *finishes* (successfully or not) and the item
   is back at `pr_pending`, guard (1) no longer blocks anything — a burst of *new* events (e.g.
   five `check_run` deliveries for five different checks on the same commit, each arriving
   seconds apart) would, without this gate, spawn a fresh rework cycle per event. The gate is
   keyed on `(itemID, domain.StuckReasonPRNeedsFix)` via `Storage.RemediationDue`
   (`session/backlog_remediation.go`) — itemID-scoped, not event- or delivery-ID-scoped, so it
   uniformly suppresses both "the poller and a webhook fire close together" and "N webhook
   deliveries fire in a burst" the same way.

**`TriggerFireEvent`'s delivery-ID dedup does not help here and should not be relied on for it.**
Its uniqueness constraint is `(workflow_id, delivery_id)`
(`session/ent/schema/trigger_fire_event.go:61`, composite, explicitly *"NOT a bare global unique
on delivery_id alone"* per that line's own comment) — scoped to a `Workflow` row being fired.
PR-fix events fire no `Workflow`, so persisting an audit row for them would carry
`workflow_id = NULL`; since SQL unique constraints treat `NULL != NULL`, every such row is
distinct regardless of `delivery_id` — writing one is fine as an audit trail (reusing
`persistTriggerFireEvent`/`TriggerFireEventRepository` for observability, consistent with the
"failed/malformed requests are visible, not silently dropped" requirement), but it provides
**zero deduplication** for this flow. **Conclusion: don't build new dedup — `itemID`-scoped
`RemediationDue` already is the right dedup key, already exists, and already covers both races
this section identifies.** `TriggerPRFixForEvent` (§3) should internally call the exact
`remediatePRFixWithBackoffGate` machinery (today private to `session/backlog_lifecycle_pr.go`;
the new exported method lives in the same package and file, so it can call the private helper
directly with no new exports needed beyond `TriggerPRFixForEvent` itself), not
`fixSpawner.AutoReopenForPRFix` raw.

One CAS-level detail worth naming: `remediatePRFixWithBackoffGate`'s eventual
`TransitionBacklogItemStatus(pr_pending → in_progress, ...)` call
(inside `AutoReopenForPRFix`, `server/services/backlog_service_triage.go:2072`) carries an
`ExpectedUpdatedAt` precondition — so even in the narrow window where two concurrent calls both
pass the backoff gate (a genuine TOCTOU on `RemediationDue`'s read-then-write), only one CAS
transition wins; the loser's `TransitionBacklogItemStatus` call errors and
`AutoReopenForPRFix` returns that error without side effects beyond the note-prepend (which is
restored regardless of spawn outcome, `:2117-2121`) — not a duplicate spawn. This is the same
"anchor on reality, never force" discipline `recoverDriftedPRItem`'s doc comment names
(`session/backlog_lifecycle_pr.go:743-745`), applying unchanged here with no new code needed for
it.

## 5. Event-Command-Policy table

Mirrors `project_plans/webhook-triggers/research/architecture.md`'s grammar (lines 302-315) —
deliberately not repeating that doc's `push`/generic-webhook/cron rows, only the new flow this
item adds.

| Domain Event (what happened) | Policy trigger (whenever X, then…) | Command (intent to change state) | Actor / System |
|---|---|---|---|
| GitHub `check_run`/`workflow_run`/`pull_request_review`/`issue_comment` webhook received | whenever `X-GitHub-Event` is one of the four new types | `VerifyGitHubSignature` (against the repo's `github_push`-Workflow-stored secret, §3) | GitHub (external) → `GitHubWebhookHandler.Handle` |
| Signature invalid for every repo-matching candidate | always | `RejectRequest(401)` + `persistTriggerFireEvent(outcome=rejected)` | `GitHubWebhookHandler` (identical to existing `push` rejection path, line 111-117) |
| Signature valid | whenever `pr_event_webhooks` flag is off | no-op, `200 OK` (silently ignored, not an error — matches AC3's "unmatched request is not an error" precedent from the sibling feature) | `GitHubWebhookHandler` |
| Signature valid, flag on | whenever payload has no extractable PR number (or, for `issue_comment`, no `issue.pull_request` key) | no-op, `200 OK` | `GitHubWebhookHandler` |
| Signature valid, flag on, PR number extracted | whenever no `pr_pending` `BacklogItem` matches `(repository.full_name, prNumber)` | no-op, `200 OK` — this instance isn't tracking that PR | `GitHubWebhookHandler` via `findPRPendingItemForEvent` (§2) |
| Matching `pr_pending` item found | always | `GetPRStatus(prNumber)` (fresh, on-demand — same call `ReconcilePRPending` makes on its tick) | `TriggerPRFixForEvent` → `git.GitWorktree`/`gh` (reused, §1) |
| Fresh `GetPRStatus` shows CI failing / blocking review / conflict / new feedback / closed-without-merging | whenever `RemediationDue(itemID, StuckReasonPRNeedsFix)` is true (backoff gate open, §4) | `AutoReopenForPRFix(itemID, fixCtx)` — reused verbatim, unmodified | `BacklogLifecycleListener.remediatePRFixWithBackoffGate` → `*BacklogService` |
| Fresh `GetPRStatus` shows CI failing etc., but backoff not yet due | always | no-op this event (already-pending remediation covers it); `MarkStuck`/notify-once still refreshes the durable `pr_needs_fix` row (§4, existing behavior, unchanged) | `remediatePRFixWithBackoffGate` |
| Fresh `GetPRStatus` shows the PR is healthy (or already merged) | always | no-op — nothing to fix; resolves any open `pr_needs_fix` row early instead of waiting for the next 60s poll tick (same "poll-shaped resolve" the existing code already does at line 1488) | `TriggerPRFixForEvent` |
| `PRStatusPoller`'s own 60s tick (unchanged backstop, per Goal 3 — not disabled) | always | identical `ReconcilePRPending` per-item body, on a fixed interval instead of event-triggered | `PRStatusPoller` (unchanged) |

## Integration points summary

| Area | File(s) |
|---|---|
| `PRFixSpawner` interface (unchanged, reused) | `session/backlog_lifecycle_pr.go:18-23` |
| `AutoReopenForPRFix` implementation (unchanged, reused) | `server/services/backlog_service_triage.go:2015-2138` |
| Existing per-item CI/review-check + fix-spawn logic to extract/reuse (§1) | `session/backlog_lifecycle_pr.go:1355-1541` (`ReconcilePRPending`'s per-item body) |
| Backoff/dedup gate to reuse, not reinvent (§4) | `session/backlog_lifecycle_pr.go:1154-1222` (`remediatePRFixWithBackoffGate`), `session/backlog_remediation.go` (`RemediationDue`) |
| New exported entry point (§1, §3) | New method on `*session.BacklogLifecycleListener`, e.g. `TriggerPRFixForEvent`, same package/file as `remediatePRFixWithBackoffGate` so it can call it directly |
| PR-pending candidate lookup (§2, reused) | `session/storage_backlog.go:980-991` (`FindPRPendingItems`) |
| Local repo-identity resolution (§2, reused) | `github.GetOwnerRepoFromRemote` (used today at `session/backlog_lifecycle_pr.go:119,135`) |
| New PR-number/repo extraction per event type (§2, new, small) | New sibling functions to `extractGitHubRepoAndBranch` in `server/services/github_webhook_handler.go` |
| Route/handler extension (§3) | `server/services/github_webhook_handler.go` — branch on `X-GitHub-Event`, new `PRFixEventRouter` field + interface |
| Dependency wiring gap to close (§3) | `server/dependencies.go` — `backlogLifecycleListener` (local var, line 658) needs exposing on `ServerDependencies` (currently only `WorkflowRepo`/`WorkflowScheduler`/`TriggerFireEventRepo` are exposed, lines 102-111/461-470); `server/server.go:764`'s construction block gains the new field |
| Feature flag (§3) | New sibling flag (e.g. `pr_event_webhooks`), checked inside `Handle`'s new event-type branch only — route registration stays gated on `webhook_triggers` |
| Audit trail (§4) | Reuse `persistTriggerFireEvent`/`TriggerFireEventRepository` for observability only (`workflow_id = NULL` rows) — not a dedup mechanism for this flow |
| Public reachability (Goal 4, not re-derived) | `.claude/docs/slack-phase2-public-reachability.md` — same path-scoped-tunnel pattern applies unchanged to `/webhooks/github`, per requirements.md's own citation |
