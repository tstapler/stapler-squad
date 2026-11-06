# Spike S4: which PR/CI/webhook events exist and can wake a session

**Date**: 2026-10-18 | **Worktree**: `nam-spikeB` at `497016cde` | **Verdict: FAIL (typed bus events) -> poller fallback applies**

PASS required "PR-merged and check-run-completed reach the `EventBus` as typed events". They do not, so the plan's FAIL fallback is the path: the dispatcher derives `pr_event` / `ci_event` from the server-side PR poller (`github.UserPRCache`, the "Up Next" PR tab source), not from the bus and not from session polling.

Live exercise of a real PR/CI event: **not run: no test repo** (creating GitHub objects was out of scope for this spike). Everything below is a code-level inventory; each row is backed by a command.

## 1. `pkg/events/types.go`: no PR/CI event type

```
$ grep -n 'EventType = ' pkg/events/types.go
EventSessionCreated      "session.created"
EventSessionUpdated      "session.updated"
EventSessionDeleted      "session.deleted"
EventSessionArchived     "session.archived"
EventUserInteraction     "session.user_interaction"
EventSessionAcknowledged "session.acknowledged"
EventApprovalResponse    "session.approval_response"
EventNotification        "session.notification"
EventBacklogItemChanged  "backlog_item_changed"
EventRemoteHealthChanged "remote.health_changed"
EventWorkflowChanged     "workflow_changed"
```

Payload types present: `BacklogItemEventPayload`, `WorkflowEventPayload`, `RemoteHealthEventPayload`. No `PR*`/`CI*`/`CheckRun*` payload. (`git grep -n 'Publish(' -- server/services/github_webhook*.go github/user_pr_cache.go` returns nothing: neither the webhook handler nor the poller publishes to the bus.)

## 2. Event source table

| Event source | Reaches `EventBus`? | Payload fields available | Entry point |
|---|---|---|---|
| `POST /webhooks/github` push-event deliveries | no | raw GitHub payload, matched to `github_push` Workflow rows | `GitHubWebhookHandler.Handle`, `server/services/github_webhook_handler.go:84` (route), fires `workflows.Scheduler`, persists `TriggerFireEvent` |
| `check_run`, `workflow_run`, `pull_request_review`, `issue_comment` webhook deliveries (`prFixEventTypes`, `github_webhook_pr_fix.go:33`) | no | `eventType`, `fullName`, `prNumber(s)`, `deliveryID`; only reconciles a tracked `pr_pending` backlog item | `handlePRFixEvent` -> `PRFixEventRouter.TriggerPRFixForEvent(ctx, repoFullName, prNumber) (matched bool, err)` (`github_webhook_handler.go:21`), implemented by `*session.BacklogLifecycleListener`; second consumer `GitHubPollerInvalidator.InvalidateForEvent` |
| `github.UserPRCache` poller (Up Next PR tab) | no (own fan-out) | `UserPR{Owner, Repo, Number, State, IsDraft, MergedAt, ClosedAt, ApprovedCount, ChangesReqCount, CheckConclusion ("success"/"failure"/"pending"/""), FailingChecks[], UnresolvedThreadCount, HasMergeConflict, LinkedSessions[], Host, AccountLogin}` (`github/user_pr_cache.go:24-57`) | `Subscribe(id, chan []UserPR)` (`:315`), `SetOnUpdated(fn)` (`:280`), `GetAll()` (`:290`); fan-out in `publish` (`:706`) delivers the full merged PR list on each poll |
| `session.WorktreePRPoller` | no | `*githubpkg.PRInfo` via `SetOnUpdated(func(repoPath, branch string, info *PRInfo))` (`server/dependencies.go:1289`) | worktree-without-session PRs only |
| `TriggerFireEvent` ent schema | n/a (audit table) | `workflow_id, outcome, delivery_id, session_id, error_message, created_at` (`session/ent/schema/trigger_fire_event.go`) | audit trail, not a source |
| `webhook_registration`, `webhook_request_ledger` ent schemas | n/a | principal/workspace/instance ownership + idempotency ledger for the webhook-management API | not event sources |

## 3. Consequence for the plan

- **Task 4.4a**: use the `UserPRCache` subscription as the single `pr_event`/`ci_event` source, with one correction found in this spike: the poller's GraphQL query is `viewer { pullRequests(first: 100, states: [OPEN], ...) }` (`github/user_pr_cache.go:854-855`), so **merged and closed PRs leave the list instead of appearing with `State=MERGED`**. `ci_event` is a diff of `CheckConclusion` (pending -> success|failure) per `(Host, Owner, Repo, Number)` across snapshots. `pr_event(merged|closed)` is "PR present in the previous snapshot, absent now", confirmed by one by-number lookup (`github.GetPRByNumber`, `github/client.go:543`, or the ETag-conditional `GetPRInfoConditional`, `github/etag_cache.go:106`, which costs zero quota on 304) to read `State`/`MergedAt`. Task 4.4a must budget that lookup and its failure mode (lookup fails: keep the wake pending, retry next poll).
- Cost: no extra quota for the diff; the confirm lookup is one request per vanished PR that has an armed wake. Coverage is the connected accounts' own open PRs (viewer's, first 100), and latency is the poll interval unless a webhook invalidation arrives (`GitHubPollerInvalidator`).
- If a wake needs a PR the cache does not hold, `schedule_wake` returns the typed `unavailable` error for that trigger type (plan fallback), everything else proceeds.
- No typed `EventBus` PR event is added by this project.
