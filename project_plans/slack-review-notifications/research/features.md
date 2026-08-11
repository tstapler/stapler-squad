# Research: Similar Features & Patterns — slack-review-notifications

## 1. Existing codebase conventions to hook into

### Notification pipeline (the thing to extend, not replace)
- `server/events/` — `EventBus.Publish(events.NewNotificationEvent(...))` is the single
  fan-out point for all notifications today. `NewNotificationEvent` takes
  `(sessionID, sessionName, notificationID, notificationType, priority, title, message, metadata)`.
- `server/notifications/subscriber.go` (`StartSubscriber` /
  `StartSubscriberWithInterval`) — subscribes to `EventBus`, filters for
  `events.EventNotification`, and **coalesces rapid-fire events for the same
  `(sessionID, notificationType)` key within a 500ms window** (`DefaultCoalesceInterval`,
  latest-wins buffer, hard flush at `maxBufferSize = 1000`) before writing to
  `NotificationHistoryStore`. This is the direct precedent for Slack-side
  dedup/coalescing raised in the requirements' Rabbit Holes section — a second
  subscriber on the same bus, with the same coalesce-key shape, is the natural
  place to hang a Slack sink rather than inventing new dedup logic.
- `server/services/notification_service.go` (`NotificationService`) — RPC-facing
  service wrapping the store + `NotificationRateLimiter` (`server/services/rate_limiter.go`,
  per-session token bucket, `Allow(sessionID)`/`Reset`/`Cleanup`). Confirms this codebase
  already treats "don't spam a channel" as a first-class concern with its own
  primitive — worth reusing/mirroring rather than building bespoke Slack throttling.
- `server/services/backlog_notifier.go` (`EventBusNotifier`) — a **17-line adapter**
  that turns `session.Notifier` calls into `EventBus.Publish` calls, specifically so
  the `session` package (which can't import `server/events` without a cycle) can still
  emit notifications. Its doc comment calls out a real bug it fixed: omitting
  `sessionID` collapsed different items into the same coalescing bucket and silently
  dropped notifications. **Direct precedent/warning**: a Slack sink keyed the same way
  must thread a real per-item ID through, not reuse a shared/empty key.

### Review queue & approval (the trigger points)
- `session/review_queue_poller.go` (`ReviewQueuePoller`, `ReviewQueuePollerConfig`) —
  the poller already has `PollInterval` (2s fast path) / `SlowPollInterval` (8s,
  backs off when queue is empty) / `ReconcileInterval`. No existing "queue depth
  threshold" concept — the Phase 1 "queue depth exceeds N" trigger is new logic,
  not an extension of an existing field.
- `server/services/review_queue_service.go` (`ReviewQueueService`) — RPC layer over
  `session.ReviewQueue`; publishes `events.NewSessionAcknowledgedEvent` /
  `NewUserInteractionEvent` on the same bus. A "new review-queue item arrived" Slack
  trigger fits the same publish-on-state-change shape.
- `server/services/approval_handler.go` (`ApprovalHandler`) — `HandlePermissionRequest`
  is the HTTP endpoint (`/api/hooks/permission-request`) already doing
  approve/deny mechanics; `broadcastApprovalNotification`-style code (~line 530)
  publishes `NOTIFICATION_TYPE_APPROVAL_NEEDED` at `NOTIFICATION_PRIORITY_URGENT`
  with an `approval_id` in metadata (this is what the frontend uses to render
  Approve/Deny buttons on the in-app toast) — exactly the metadata shape a Phase-2
  Slack interactive button would need to carry through the webhook payload and back.
  Message truncation utilities already exist here and should be reused rather than
  reinvented for Slack block-size caps:
  - `truncateString(s string, maxRunes int) string` (rune-safe, appends `"..."`)
  - `maxNotificationMessageLen = 120` (toast message cap)
  - `maxEscalationReasonLen = 500` (with a documented rationale: escalation reasons
    are re-marshaled and written to disk on every approval Create/Resolve while
    holding a write lock, so an unbounded string scales that cost)
  - `sanitizeNotificationText` — strips newlines/control chars before embedding
    user/model-controlled strings into an OS-level notification (same injection
    concern applies to Slack `text`/block fields).
- `session/backlog_review.go` — `GetGitDiff`/`GetGitDiffRef` already truncate diffs
  at `headless.MaxDiffSizeReview` bytes and return a `truncated bool` alongside the
  diff. **This is the size-cap precedent for the "diff summary" field** called out
  in Rabbit Holes — reuse this truncation boundary (or a smaller Slack-specific one,
  since Slack's ~3000-char block-text limit is much tighter than the review-prompt
  diff cap) rather than inventing new diff-truncation logic.

### Config & secrets
- `config/config.go` / `config/types.go` — flat `Config` struct, JSON tags,
  `omitempty`. The established secret-field pattern is **`AnthropicAPIKey`**
  (`config/config.go:371`): plain `string` field, comment "Do not log this value",
  loaded from `config.json` OR overridden by an env var (`ANTHROPIC_API_KEY`) at
  `DefaultConfig()` time. **No 1Password/`op` integration exists anywhere in the Go
  codebase** — confirmed by grep for `1[Pp]assword|op read|op://` across
  `*.go`/`*.md`, and independently confirmed by a prior research doc
  (`project_plans/launchd-shell-sourcing/research/build-vs-buy.md`, section 2)
  that did the same grep and found only planning-doc mentions, none in application
  code. **This means the requirements' NFR line "must... follow existing
  secret-handling conventions (see 1Password usage elsewhere in the repo)" points at
  a convention that doesn't exist in this Go codebase** — the actual precedent to
  follow is the `AnthropicAPIKey` pattern (plaintext config field + env var
  override + "don't log" comment + likely a `SLACK_WEBHOOK_URL` env override,
  mirroring `ANTHROPIC_API_KEY`), not 1Password. Flag this gap explicitly in
  planning rather than silently building a 1Password integration that has no
  precedent to follow.
- `server/services/secret_scanner.go` already has a **Slack token regex**
  (`\bxox[boas]-[0-9A-Za-z\-]+\b`) in its list of patterns to redact from captured
  command output — worth checking whether the new `slack_webhook_url` value itself
  (a `https://hooks.slack.com/services/...` URL, not an `xox*` token) should be
  added to this scanner's pattern list so it's never echoed back in scrollback/logs
  either.

### Outbound HTTP precedent
- `server/services/domain_checker.go` (`DomainAgeChecker`) is the closest existing
  "call an external HTTP service, cache/degrade gracefully" example:
  `http.Client{Timeout: 3 * time.Second}`, an `enabled bool` no-op switch, and a
  24h result cache. Good shape to mirror for the Slack notifier's HTTP client
  (short timeout, explicit enabled/disabled switch driven by whether
  `slack_webhook_url` is configured, no retry-with-backoff machinery needed given
  fire-and-forget semantics).
- `server/services/circuit_breaker_handler.go` exists but is a read-only RPC handler
  exposing *existing* circuit-breaker state (for agent tool-call backoff), not a
  reusable outbound circuit-breaker component — not directly reusable for Slack
  webhook calls, but confirms the codebase already has a vocabulary/precedent for
  "don't hammer a failing external endpoint" if the Slack notifier needs one.

### Settings UI
- `web-app/src/app/settings/page.tsx` and sibling pages
  (`pipeline-modes/page.tsx`, `unfinished/page.tsx`, `backlog-sources/page.tsx`) are
  the settings-page structural precedent to follow for the new webhook/toggle UI.
- **No existing "test this webhook/connection" UI affordance** was found anywhere
  in `web-app/src` (grepped for `webhook`, `TestConnection`, `VerifyWebhook`) —
  the closest analog is `PreviewBackwardSyncImpact`
  (`server/services/backlog_sources_preview.go`), a dry-run RPC that reports the
  *impact* of a backlog-source config change before committing it, not a
  connectivity test. There is no reusable "send a test ping" pattern to copy;
  a Slack "Send test message" button would be new UI/RPC surface, not a
  mirror of an existing one (see Unstated Needs below).

## 2. Industry patterns for external chat/mobile push in CI/agent-review tools

- **GitHub Actions → Slack (`slackapi/slack-github-action`, and the older
  `8398a7/action-slack`)**: message content is built from a fixed template (repo,
  branch, actor, status, workflow run URL) using Slack's Block Kit; failure vs.
  success determines color/emoji; a single webhook URL is stored as a repo/org
  secret, never in workflow YAML. Pattern: **template-per-event-type**, not
  freeform text, so payload size stays bounded and messages are visually scannable.
- **PagerDuty (and Opsgenie) style alerting**: **deduplication key** per alert
  (equivalent to this codebase's `coalesceKey(sessionID, notificationType)`) so
  repeated triggers of the "same" underlying condition collapse into one open
  incident with an update, not N separate pages. Also: alerts have explicit
  severity, and low-severity/noisy alerts are batched into a digest rather than
  paging immediately — directly analogous to the "queue depth threshold" idea in
  the requirements (a burst is a digest, not N separate messages).
  PagerDuty/Opsgenie also both support **snooze/mute** per alert source, which this
  codebase has no equivalent for yet (see Unstated Needs).
- **CI bots posting to Slack (Buildkite, CircleCI, GitLab)**: universally
  rate-limit-aware — they batch multiple job-status changes that land within a
  short window into a single message update (using Slack's `chat.update` on a
  previously-posted message timestamp) rather than posting a new message per
  event, specifically because Slack's incoming-webhook rate limit is ~1 msg/sec
  and CI bursts (e.g. a monorepo's 40 jobs finishing near-simultaneously) will
  otherwise 429. This codebase's Phase 1 is webhook-only (no `chat.update`
  capability, which requires a bot token + `channel`+`ts`, not just an incoming
  webhook) — so the mitigation available here is coalescing/digesting *before*
  sending, not updating-in-place after.
- **Slack Block Kit hard limits** (documented, not just a rabbit-hole guess):
  ~3000 characters per text block, ~50 blocks per message, ~4000 chars for the
  top-level `text` fallback field. Any diff/summary content must be truncated
  client-side before POST — Slack does not truncate gracefully, it rejects the
  whole message (400) if oversized, which would silently drop the *entire*
  notification, not just the oversized field, if not handled defensively.
- **Interactive buttons (Phase 2 precedent)**: GitHub's own "Deploy to
  production?" Slack approval bots and PagerDuty's Slack app both verify the
  request signature (`X-Slack-Signature` + `X-Slack-Request-Timestamp` HMAC,
  using the app's signing secret) on every inbound interactive-callback POST,
  and reject requests older than 5 minutes to prevent replay — this is the
  well-established shape for the Phase 2 signing-secret requirement already
  flagged as non-negotiable in the requirements' Constraints section.

## 3. Edge cases & failure modes the design must handle

- **Webhook URL invalid/malformed** — validate at config-save time (settings UI
  and/or config load) rather than only discovering it on first failed POST;
  Slack returns a 404/`no_service` body for a URL that's syntactically valid but
  not a real registered webhook.
- **Slack down / network error / DNS failure** — must be fire-and-forget per the
  NFR ("must not block or fail the underlying review-queue/approval flow"). Needs
  a bounded timeout (mirror `domain_checker.go`'s `3 * time.Second`) so a hung
  connection doesn't pile up goroutines during an outage, plus structured log-only
  failure (no retry queue required given Medium appetite, but the failure must be
  visibly logged so silent total failure isn't invisible forever — see Unstated
  Needs for a "delivery history" surface).
- **Slack rate limit (~1 msg/sec informal)** — a burst of review-queue items
  (e.g. many sessions going idle near-simultaneously, or a queue-depth-threshold
  crossing during a spike) must not fire one Slack POST per item. This is exactly
  what `server/notifications/subscriber.go`'s 500ms coalescing window already
  solves for the in-app path; the Slack sink should either share that coalescing
  stage or implement an equivalent buffer-and-flush of its own tuned to Slack's
  rate limit (500ms is actually *too fast* for a hard 1/sec cap — needs its own
  interval, not a blind reuse of 500ms).
- **Message too large** — diff/summary content must be truncated before
  serialization (see Block Kit limits above), with truncation applied to the
  Slack-specific field, not assumed to be already safe because the review-prompt
  diff truncation (`MaxDiffSizeReview`) ran upstream — that cap is sized for an
  LLM prompt, not a Slack block.
- **Config missing / feature disabled** — `slack_webhook_url` empty must be a
  clean no-op (mirror `DomainAgeChecker`'s `enabled bool` early-return), not an
  error path, since Phase 1 is opt-in.
- **Secret leakage** — webhook URL must never be logged (mirror the
  `AnthropicAPIKey` "do not log" comment convention) and should be added to
  `secret_scanner.go`'s redaction patterns so it can't leak via captured
  command/scrollback output either if a user pastes it into a terminal.
  `/api/config` or equivalent GET endpoints must not echo the raw webhook URL
  back to the frontend after it's saved (mask it, similar to how API keys are
  typically shown as `sk-...redacted` in other tools) — needs an explicit design
  decision since `AnthropicAPIKey`'s existing config RPC precedent should be
  checked for whether *it* already masks on read (worth verifying in planning,
  not assumed here).
- **Session/item deleted between trigger and Slack POST** — if the notifier is
  async (goroutine dispatched off the event bus), the underlying review-queue
  item could be acknowledged/resolved by the time the Slack POST actually lands,
  producing a message with a dashboard link to something already handled. Not
  fatal (user just sees it's already gone) but worth a design note.
- **Duplicate notifications across restarts** — if the trigger is poll-based
  (queue depth check on each poll tick) rather than edge-triggered (only on
  state transition), a naive implementation would re-fire the threshold message
  every poll cycle while the condition remains true. Must be edge-triggered
  (fire once on crossing into the condition, not on every tick where it holds).

## 4. Unstated needs beyond the explicit requirements

- **Test/verify webhook button** — requirements ask for a settings UI to
  "configure the webhook URL and toggle(s)" but don't mention verifying it
  works. Given there's no existing "test connection" UI pattern in this repo
  (confirmed by grep, see above) and Slack webhook URLs are easy to typo/copy
  wrong, a "Send test message" button (small RPC that POSTs a canned test
  payload) is a near-certain follow-up ask once Phase 1 ships without it —
  worth scoping in now rather than as a fast-follow, since the requirements'
  own Success Metrics section describes manually verifying delivery, which is
  exactly what a test button automates.
- **Delivery failure visibility** — the NFR says Slack failures must not block
  the underlying flow and should be "logged," but a log line the user never
  reads doesn't satisfy the actual goal (the user finding out an agent needs
  them). If Slack delivery silently fails (bad URL, Slack outage) the user is
  back to square one with zero signal — worse than before, because they now
  *believe* they'll be notified. Consider surfacing last-delivery-failure state
  in the settings UI itself (e.g. "Last Slack delivery: 2 hours ago" / "Last
  attempt failed: <reason>") so a broken webhook doesn't fail invisibly forever.
- **Per-item vs. digest notification granularity** — the requirements list two
  independent triggers (new item arrives / queue depth threshold) but don't say
  what happens when both would fire close together (e.g. threshold crossed by
  the same item that just triggered a per-item message) — needs a decision on
  whether these are mutually exclusive per event or can double-fire.
- **Snooze/mute** — every industry precedent researched (PagerDuty, Opsgenie,
  even GitHub notification settings) treats "temporarily stop paging me" as
  core, not optional, once push notifications exist — otherwise a
  work-from-elsewhere burst becomes phone-buzzing spam with the only recourse
  being to disable the feature entirely. Out of explicit scope for this
  Medium-appetite Phase 1, but worth naming as a likely Phase 1.5/2 ask so the
  config schema (e.g. a `slack_notifications_paused_until` timestamp field)
  isn't designed in a way that makes adding it later awkward.
- **Link-back correctness on a non-localhost deployment** — the requirement says
  the Slack message needs "a working link back to the dashboard." This repo's
  default is `localhost:8543`, which is not reachable from a phone/Slack click
  unless the user is already set up for remote access. Worth confirming in
  planning what base URL the notifier should use (a configurable
  `public_base_url`-style field, distinct from `slack_webhook_url`) so the
  generated link isn't silently `http://localhost:8543/...` and useless from a
  phone — this is a real gap between the stated deliverable ("a working link")
  and the single-user localhost-first baseline described in the requirements.
</content>
