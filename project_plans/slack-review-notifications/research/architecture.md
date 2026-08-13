# Architecture Research: slack-review-notifications

## Prior-art check

`Glob project_plans/*/research/architecture.md` + grep for `review_queue|approval_handler|notification_service` returned hits in ~15 other projects' architecture docs (e.g. `review-queue-event-driven`, `backend-di-refactor`, `review-gate-stale-session-rework`, `review-session-notification-cleanup`). None of these is `slack-review-notifications` and none covers an outbound Slack integration — they touch the same files for unrelated reasons (event-driven queue refactors, DI cleanup, notification dedup). Treated as a fresh investigation.

## 1. Where does the outbound Slack notifier live?

**`NotificationService` (`server/services/notification_service.go`) has no pluggable-sink abstraction to register with.** Its actual job (read in full: 332 lines) is narrow — `SendNotification` (RPC used by tmux sessions/external Claude processes), `GetNotificationHistory`, `MarkNotificationRead`, `ClearNotificationHistory`. It holds a `notificationStore`, a `notificationRateLimiter`, and an `eventBus`, and its `SendNotification` RPC does one thing: publish an `events.NewNotificationEvent(...)` onto `ns.eventBus`. There is no `Sink`/`Observer` interface anywhere in this file or its callers.

**The real fan-out point is the underlying `*events.EventBus`** (`pkg/events/bus.go`), a generic pub/sub bus with `Subscribe(ctx) (<-chan *Event, string)` / `Publish(event)`. Both existing "something needs a human" producers already funnel through it:

- Review-queue items: `ReactiveQueueManager.OnItemAdded` (`server/review_queue_manager.go:327-419`) — called synchronously from `ReviewQueue.Add`'s `exists==false` branch when a session enters the queue — builds a `sessionv1.ReviewQueueEvent` for `WatchReviewQueue` stream clients, then (skipping `ReasonApprovalPending` and hidden/routine-suppressed items) publishes an `events.NewNotificationEvent(...)` to `rqm.eventBus` (line ~411).
- Approval-pending items: `ApprovalHandler.broadcastApprovalNotification` (`server/services/approval_handler.go:506-538`) builds `title`/`message`/`metadata` from the `PendingApproval` and publishes its own `events.NewNotificationEvent(...)` to `h.eventBus` (line 537), with `NotificationType_NOTIFICATION_TYPE_APPROVAL_NEEDED` / `NOTIFICATION_PRIORITY_URGENT`.

So the two events this feature cares about are **already unified on one bus**, in principle. But treating "subscribe to `EventBus`, filter by `NotificationType`" as the Slack sink is **not clean enough to use as-is**: `events.NewNotificationEvent` is also called from `server/services/capacity_monitor.go`, `server/services/autonomous_orchestration_service.go`, `server/services/backlog_notifier.go`, `server/services/session_service.go`, `server/services/backlog_service_triage.go`, and `server/mcp/tools_backlog.go` — none of which are review-queue/approval events. `NotificationType` alone (`NOTIFICATION_TYPE_INFO`, `_STATUS_CHANGE`, `_ERROR`, etc. — see `mapReviewItemToNotification`, `server/review_queue_manager.go:853-889`) doesn't reliably distinguish "review queue item worth pinging Slack about" from routine churn (idle/stale/uncommitted-changes also map through the same `NotificationType_NOTIFICATION_TYPE_INFO`/`_STATUS_CHANGE` values other producers use). Building a generic bus-subscriber sink would require inventing a new discriminating metadata field on every producer just to make Slack's filter reliable — more invasive than the alternative.

**Recommendation: a small concrete `SlackNotifier` called directly from the two known-good producer call sites, not a generic bus subscriber.**

- New file `server/services/slack_notifier.go` — a concrete type (no interface; only one implementation exists or is planned, per `.claude/rules/interface-pollution-checklist.md`). Two methods matching the two real call sites, e.g. `NotifyReviewQueueItem(ctx, item *session.ReviewItem, dashboardURL string)` and `NotifyApprovalPending(ctx, approval *PendingApproval, dashboardURL string)`. Each formats a Slack Block Kit payload and POSTs it.
- `ApprovalHandler` (already in `package services`) holds `*SlackNotifier` directly and calls `NotifyApprovalPending` right next to its existing `h.eventBus.Publish(event)` in `broadcastApprovalNotification` (line 537), wired via a new `SetSlackNotifier(...)` late-wire setter — same pattern as its existing `SetNotificationStamper`, `SetAnalyticsStore`, `SetDomainChecker`, `SetHeadlessPool` (`server/services/approval_handler.go:134-177`).
- `ReactiveQueueManager` lives in **package `server`**, not `server/services` (`server/review_queue_manager.go:1`). This repo has an explicit, already-applied precedent for exactly this cross-package situation: `OneShotPRCreator` (`server/review_queue_manager.go:19-26`) is a narrow interface **declared in the consumer package** ("`server`") with the comment *"Defined here — the consumer — rather than in server/services, per this repo's anti-interface-pollution convention"*, satisfied structurally by `*services.SessionService`. Follow the same shape: declare a narrow `SlackNotifier` interface in `server/review_queue_manager.go` with just the one method `ReactiveQueueManager` needs, and let `*services.SlackNotifier` satisfy it implicitly. Call it from `OnItemAdded` right next to the existing `rqm.eventBus.Publish(notifEvent)` call (line ~411), under the same `item.Reason != session.ReasonApprovalPending && !suppressForHidden` guard (approval-pending items are already excluded here to avoid a duplicate card in the in-app notification panel — the same de-dup logic applies to Slack: `ApprovalHandler` is the sole source of truth for approval-pending Slack pings, `OnItemAdded` for everything else).
- Both are wired in `server/server.go` alongside the existing approval-handler/reactive-queue-manager construction block (`server.go:454-506` for `approvalHandler`; the `ReactiveQueueManager` construction is earlier — `NewReactiveQueueManager` at `server/review_queue_manager.go:120`), via new `Set*` calls guarded by `cfg.Slack.WebhookURL != ""` (don't even construct a `SlackNotifier` if unconfigured).

## 2. Integration points

| System | Integration point | What changes |
|---|---|---|
| `session.ReviewQueuePoller` | Not touched directly. It owns the poll loop and `queue.Add()` (`session/review_queue_poller.go:865`), but the notification fan-out already happens one layer up, in `ReactiveQueueManager.OnItemAdded`, which the poller doesn't know about (`ReviewQueue` is a plain data structure with no built-in observer list — `ReactiveQueueManager` is wired as the consumer at server startup). |
| `ReactiveQueueManager.OnItemAdded` | `server/review_queue_manager.go:327-419`, next to the existing `rqm.eventBus.Publish(notifEvent)` (~line 411) | Add `rqm.slackNotifier.NotifyReviewQueueItem(...)` call, config-gated, same exclusions as the existing eventBus publish |
| `ApprovalHandler.broadcastApprovalNotification` | `server/services/approval_handler.go:506-538`, next to `h.eventBus.Publish(event)` (line 537) | Add `h.slackNotifier.NotifyApprovalPending(...)` call, config-gated |
| `config.Config` | `config/config.go` (struct definition, line 229) + `config/types.go` (nested config structs) | New `SlackConfig` struct in `types.go`, new `Slack SlackConfig` field in `Config` |
| Queue-depth threshold trigger | `ReactiveQueueManager.OnQueueUpdated` (`server/review_queue_manager.go:527`) or `OnItemAdded` reading `rqm.queue.Count()`-equivalent | Second, independent trigger path — see §5 |
| Wiring | `server/server.go` (near `approvalHandler` construction, ~line 473-506, and wherever `ReactiveQueueManager` is constructed) | New `services.NewSlackNotifier(cfg)` + two `Set*` calls |

## 3. Data flow / non-blocking design

The exact pattern this feature needs already exists in the same function it needs to hook into. `OnItemAdded`'s last line calls `rqm.maybeAutoCreatePR(item)` (`server/review_queue_manager.go:418`), documented as: *"Runs async so a slow/failing LLM call never blocks queue-add notification delivery."* (line 417-418). `maybeAutoCreatePR` launches its work in a goroutine and never returns an error to the caller.

Copy that shape for Slack:

```go
// in OnItemAdded, after the eventBus.Publish call, under the same guard:
if rqm.slackNotifier != nil {
    go rqm.slackNotifier.NotifyReviewQueueItem(rqm.baseContext(), item, dashboardURL)
}
```

Inside `SlackNotifier.NotifyReviewQueueItem`/`NotifyApprovalPending`:
- Derive a short `context.WithTimeout` (e.g. 5s) from the passed context — independent of whatever deadline the caller's context carries, since `OnItemAdded` runs inline on the poll/reactive path and must not be held open by a slow webhook.
- POST the formatted Block Kit payload; on any error (non-2xx, timeout, DNS failure, rate-limit `429`), `log.Warn(...)` and return — never propagate an error, never retry synchronously (retries would risk pushing well past the poll cycle and violate "must not block").
- Never `log` the webhook URL itself (NFR: "must not be logged").

This satisfies the NFR directly: *"a Slack delivery failure ... must not block or fail the underlying review-queue/approval flow — notification delivery is fire-and-forget/best-effort with logging, not a blocking dependency."*

`rqm.baseContext()` already exists (`server/review_queue_manager.go:505`) as the server-lifetime context `maybeAutoCreatePR` uses for its own async work — reuse it rather than inventing a second one.

## 4. Phase 2: inbound Slack button clicks

**Do not extend `/api/hooks/permission-request`.** That endpoint (`server/server.go:511`, `ApprovalHandler.HandlePermissionRequest`, `server/services/approval_handler.go:199`) is the *creation* side of an approval — Claude Code's hook calls it to ask "may I run this tool," and it either auto-decides or queues a `PendingApproval` and blocks the hook process waiting for a decision. A Slack button click is a *resolution* event for an approval that's already queued — the opposite direction, a different payload shape (Slack's signed interactive-component JSON vs. Claude Code's hook JSON), and a different trust model (this endpoint currently has no auth beyond implicit localhost trust from the hook process; Slack's payload needs signing-secret verification). Merging the two into one handler would mean branching on payload shape before you know whether you can trust the request — worse for the "verify the signature before doing anything" hard requirement in the constraints.

**What to reuse instead:** the resolution path the web dashboard's own Approve/Deny buttons already call — `ApprovalService.ResolveApproval` (`server/services/approval_service.go:61-...`), which validates `decision` is `"allow"`/`"deny"` and ultimately calls `ApprovalStore.Resolve(id, decision)` (`server/services/approval_store.go:171`). This is the single choke point that unblocks the waiting `HandlePermissionRequest` HTTP handler and broadcasts the resolution event.

**Precedent for the new endpoint's shape:** `/api/external/approvals/respond` (`server/server.go:462`, `ExternalWebSocketHandler.HandleApprovalResponse`, `server/services/external_websocket.go:86`) is already exactly this pattern — a small, dedicated `http.HandlerFunc` (not a ConnectRPC service) that exists solely to let a non-browser caller resolve something the browser normally resolves, translating an external signal into the same underlying store call. (Note: that specific handler resolves a *different* store — `ExternalApprovalMonitor`/socket-path based, for external CLI sessions — so it is not directly reusable code, only a structural precedent for "dedicated raw-HTTP endpoint that closes the loop back into the approval store.")

Recommended shape for Phase 2:
- New route, e.g. `srv.mux.HandleFunc("/api/hooks/slack-interactive", slackInteractiveHandler.Handle)`, registered next to the existing hook/external-approval registrations in `server.go`.
- First and only thing the handler does before touching any store: verify `X-Slack-Signature` / `X-Slack-Request-Timestamp` against `cfg.Slack.SigningSecret` (HMAC-SHA256 per Slack's documented algorithm) and reject anything that doesn't check out. This is the "hard requirement, not optional hardening" from the constraints — anything that can hit this endpoint successfully has agent-approval authority, same as `ApprovalStore.Resolve` itself.
- On success, parse Slack's `payload` form field, map the button's `action_id`/`value` (approval ID + allow/deny, encoded when the button was built in Phase 1's outbound message) to a call into `ApprovalStore.Resolve(approvalID, decision)` directly (same call `ApprovalService.ResolveApproval` makes) — either call the store directly, or construct an in-process `connect.Request` and call `approvalService.ResolveApproval(ctx, req)` to get its existing event-bus broadcast/notification-stamping side effects for free. Prefer the latter (calling the existing RPC method in-process) so Slack-resolved approvals get exactly the same "connected browser clients see it resolve in real time" behavior as dashboard-resolved ones, without re-implementing `ResolveApproval`'s side effects.
- Gate the whole feature behind `cfg.Slack.ApprovalEnabled` (default false, per requirements) — when false, don't even register the route.

## 5. Trigger policy — queue-depth threshold (small note, not a full ECP table)

Per the prompt's guidance, this integration doesn't warrant a full Event-Command-Policy/EventStorming table — it's two POST triggers off two existing, already-understood call sites, not a multi-actor domain. The one piece of genuinely non-trivial policy logic is the **queue-depth threshold** (requirements: "queue depth exceeds a configured threshold N (alternative/additional trigger)"):

- **Where to read queue depth:** `rqm.queue.Count()` (already used at `server/review_queue_manager.go:343` in the poll-backoff logic) or the `List()`/`Count()` accessors on `session.ReviewQueue`.
- **Where to fire it:** most natural is inside `OnItemAdded` itself (after the per-item notify), or `OnQueueUpdated` (`server/review_queue_manager.go:527`) which already runs on every queue-state recompute.
- **Dedup concern (flagged in requirements' Rabbit Holes, ~1msg/sec Slack rate limit):** if per-item notifications are also enabled, a burst of N items arriving together would fire N per-item messages *and* a threshold message back-to-back. Planning should decide whether the two triggers are mutually exclusive per config (`slack_notify_on_queue_item` XOR threshold) or whether threshold-crossing needs its own dedup latch (e.g. "only fire once per threshold-crossing, reset when queue drops back below N") to avoid re-firing on every subsequent item while already over threshold. This is a config/planning decision, not an architecture one — no new component is needed to implement either choice, just a bit of state (last-known depth, or a "already notified this crossing" bool) on `SlackNotifier` or `ReactiveQueueManager`.

## 6. Config placement

`config/types.go` already has an established convention for feature-scoped nested config structs embedded into the top-level `Config` (`config/config.go:229`): `NotificationPrefs` (line 6), `HibernationConfig` (line 12), `SessionRetentionConfig` (line 38), `CapacityConfig`, `BrowserPassthroughConfig`, etc. — each embedded as a named field (e.g. `Hibernation HibernationConfig `json:"hibernation,omitempty"`` at `config.go:337`), several with an `XxxOrDefault()` helper for zero-value fallback (e.g. `SessionRetentionConfig.RetentionDaysOrDefault()`, `types.go:58`).

Follow the same shape:

```go
// config/types.go
type SlackConfig struct {
    WebhookURL          string `json:"webhook_url,omitempty"`
    NotifyOnQueueItem   bool   `json:"notify_on_queue_item,omitempty"`
    QueueDepthThreshold int    `json:"queue_depth_threshold,omitempty"`
    // Phase 2:
    ApprovalEnabled bool   `json:"approval_enabled,omitempty"`
    SigningSecret   string `json:"signing_secret,omitempty"`
}
```

```go
// config/config.go, inside Config struct, near Notifications/Hibernation/Capacity:
Slack SlackConfig `json:"slack,omitempty"`
```

**On the "must follow existing secret-handling conventions" constraint:** this codebase's actual existing precedent for a comparable secret in `Config` is `AnthropicAPIKey string `json:"anthropicApiKey,omitempty"`` (`config/config.go:371`, read directly by `ConfigFileCredentialSource.Resolve`, `server/services/credentials.go:195-203`) — a **plain JSON string field**, not a 1Password/vault-indirected reference. There is no existing abstraction in this repo's `config` package for vault-backed secrets; the "1Password usage elsewhere in the repo" the requirements point to is this *repo's own dev/bootstrap tooling* (`bootstrap/roles/secrets`), not a pattern `config.Config` itself implements for any existing field. Given that, `WebhookURL`/`SigningSecret` as plain `Config` string fields is consistent with existing practice, not a regression — but planning should still explicitly confirm no code path logs the full `Config` struct wholesale (e.g. a `log.InfoS("config loaded", cfg)`-style call) the way `AnthropicAPIKey` presumably already relies on not happening; this is worth a quick grep during implementation rather than assuming.

## Summary of concrete file touches

- `config/types.go` — new `SlackConfig` struct
- `config/config.go` — new `Slack SlackConfig` field on `Config`
- `server/services/slack_notifier.go` — new file, concrete `SlackNotifier` type, `NotifyReviewQueueItem` / `NotifyApprovalPending` methods, Block Kit formatting, size-capped diff summary (Rabbit Holes: ~3000 char block text limit)
- `server/services/approval_handler.go` — `slackNotifier` field + `SetSlackNotifier` setter; one call in `broadcastApprovalNotification`
- `server/review_queue_manager.go` — narrow consumer-side `SlackNotifier` interface (mirrors `OneShotPRCreator`); `slackNotifier` field + setter; one call in `OnItemAdded`; queue-depth-threshold check (in `OnItemAdded` or `OnQueueUpdated`)
- `server/server.go` — construct `*services.SlackNotifier` when `cfg.Slack.WebhookURL != ""`, wire into both consumers
- Phase 2 only: `server/services/slack_interactive_handler.go` (new), route registration in `server.go`, signature verification against `cfg.Slack.SigningSecret`, call into `ApprovalService.ResolveApproval` (in-process) or `ApprovalStore.Resolve` directly
