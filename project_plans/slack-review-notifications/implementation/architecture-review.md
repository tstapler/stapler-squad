# Architecture Review: slack-review-notifications
**Date**: 2026-08-06
**Verdict**: CONCERNS (blocker resolved by plan.md patch — see below; remaining items are
non-blocking Concerns/Nitpicks)

`docs/adr/ADR-000-architecture-constitution.md` does not exist in this repo — constitution
check skipped.

Plan claims were spot-checked against the actual codebase (line numbers, existing patterns
`OneShotPRCreator`, `defaults_service.go`, `approval_handler.go`, `session/backlog_crypto.go`,
`config.GetOrCreateEncryptionKey`, `notification_service.go`'s lack of a sink abstraction) and
all confirmed accurate, with only minor (±5 line) drift. The plan's own Step 0.5 alternatives
analysis and its interface-pollution-checklist application (`SlackNotifierWiring` declared in
the consumer package `server/review_queue_manager.go`, narrowly scoped to 2 methods, mirroring
the existing `OneShotPRCreator` precedent; `ApprovalHandler` holding a concrete `*SlackNotifier`
since it's same-package) are both correct and not re-flagged below.

## Blockers (resolved)

- [x] **RESOLVED** (Task 1.4.1a adds `clear_webhook_url`/`clear_signing_secret` bool fields to
  `UpdateSlackConfigRequest`; Task 1.4.2b's three-way branch — clear / set / leave-unchanged —
  implements the resulting precedence; Task 1.4.3b adds a "Remove" UI affordance that sets the
  clear flag). Story 1.4.2 / Task 1.4.2b (`server/services/slack_config_service.go`,
  `proto/session/v1/session.proto` `UpdateSlackConfigRequest`) — `webhook_url: ""` /
  `signing_secret: ""` is defined to mean "leave unchanged," which makes it impossible to
  explicitly clear a configured secret through the settings UI/RPC. This directly contradicts
  the plan's own Risk Control → Rollback procedure (line 121-122): "unset `slack_webhook_url`
  **in the settings UI** (or delete the `slack` block from `config.json` and restart)." As
  specified, only the second half of that "or" is actually possible — the settings UI has no
  way to send "clear this secret" as distinct from "I didn't type anything, don't touch it."
  This is a classic illegal/missing-state bug: two real states ("leave unchanged," "clear")
  are collapsed onto one wire value (empty string), and the plan documents a user-facing
  capability that the API contract as designed cannot deliver.
  **Remediation**: give `UpdateSlackConfigRequest.webhook_url`/`signing_secret` proto3
  `optional` presence tracking (generates a `HasWebhookUrl()`/pointer-backed field) — field
  absent = leave unchanged, field present-and-empty = clear, field present-and-non-empty = set.
  Or, simpler to implement without relying on proto3 optional semantics reaching the frontend
  correctly: add explicit `bool clear_webhook_url` / `bool clear_signing_secret` fields to
  `UpdateSlackConfigRequest` in Task 1.4.1a, and have `SlackNotificationSettings.tsx` (Task
  1.4.3b) expose a "Remove" affordance next to the masked field that sets the clear flag. Fix
  this in the proto message shape now (Task 1.4.1a) — it is a wire-contract change, cheap
  before `make proto-gen` runs and expensive after a frontend is built against the wrong shape.

## Concerns

- [ ] Story 1.2.1 / Task 1.2.1c + Story 1.5.1 / Task 1.5.1d (`server/services/slack_notifier.go`)
  — the resolved webhook URL and signing secret are carried as plain `string` end-to-end
  (`resolveSlackWebhookURL(cfg) (string, error)`, `postToSlack(ctx, webhookURL string, ...)`).
  ADR-001 exists specifically because these are high-blast-radius bearer secrets, but the "never
  log the secret" invariant is enforced only by a single grep-based regression test (Task
  1.5.1d) that checks captured log output for one known test-fixture URL substring — not by the
  type system. Task 1.2.1c's own acceptance criterion ("never includes the webhook URL in any
  returned/logged error") is explicitly *not* verified by that task, only by a different,
  later-story test — so if Epic 1.5 slips under the feature's Medium appetite, Story 1.2.1 ships
  with zero enforcement of its stated security AC. A future call site (including any Phase 2
  code, which is a separate, higher-risk epic) that does `log.Warn("posting", "url", url)`
  compiles cleanly and leaks the secret; the grep test only catches what its author anticipated.
  **Remediation**: wrap the resolved value in a small type, e.g. `type slackSecret string` with
  a `String() string { return "[REDACTED]" }` (and `GoString()`) method, so `%s`/`%v` formatting
  and naive structured-log calls can't leak the plaintext regardless of call site. If this
  repo's `log` package (`github.com/tstapler/stapler-squad/log`) supports `slog.LogValuer` (or
  an equivalent), implement that too. This turns a test-enforced convention into a structural
  guarantee — directly the primitive-obsession fix the type-driven-design lens calls for on a
  value ADR-001 itself classifies as security-critical.

- [ ] Story 1.3.1 / Task 1.3.1b (`server/review_queue_manager.go`'s `OnItemAdded`) —
  the "digest suppresses per-item on the same call" precedence policy (this plan's default for
  Unresolved Question #2) is implemented in the caller: `OnItemAdded` calls
  `MaybeNotifyQueueDepthThreshold` and then, only `if !fired && cfg.Slack.NotifyOnQueueItem`,
  calls `NotifyReviewQueueItem` — two separate `SlackNotifierWiring` methods with their
  interaction encoded in a general review-queue lifecycle method that has nothing else to do
  with Slack. This leaks a Slack-notifier business rule (which trigger wins) into an unrelated
  consumer, and it means the interface (`SlackNotifierWiring`) doesn't actually encapsulate
  the notifier's own coalescing behavior — two callers of `SlackNotifierWiring` in the future
  would each have to re-implement the same suppression logic correctly.
  **Remediation**: collapse `MaybeNotifyQueueDepthThreshold` + `NotifyReviewQueueItem` into one
  `SlackNotifierWiring` method (e.g. `NotifyQueueItemAdded(ctx, cfg, item, depth, threshold,
  dashboardURL)`) that owns the digest-vs-per-item decision internally. `OnItemAdded` then just
  calls one method without knowing the precedence rule, and `SlackNotifierWiring` shrinks from
  2 methods to 1 — tighter Interface Segregation and the policy lives where it belongs.

## Nitpicks

- Story 1.2.1-1.2.4 (`SlackNotifier`): the type accumulates four responsibilities — Block Kit
  message formatting, HTTP transport, delivery-status tracking, and burst-coalescing (threshold
  latch). Reasonable for the current scope and consistent with this repo's preference for
  concrete single-purpose service types over premature decomposition (`DomainAgeChecker` etc.).
  If a second notification channel or Phase 2 growth pushes this type much larger, split the
  threshold-latch/coalescing logic out once a second consumer of that exact shape exists — not
  before, per `.claude/rules/interface-pollution-checklist.md`'s "don't generalize from one
  call site" spirit applied to decomposition, not just interfaces.
- Task 1.3.1c: the dashboard-base-URL fallback plumbing into `OnItemAdded` is explicitly left
  unresolved ("exact plumbing left to implementation... not a specific closure signature").
  `ReactiveQueueManager` is constructed before `Server` in `server/dependencies.go`'s wiring
  order, which structurally rules out a direct `*Server` handle — but pin the actual signature
  now (a `func() string`, mirroring the plan's own cited `hookBaseURLFn` precedent at
  `server/server.go:467-470`) rather than leaving it open for the implementer to improvise.
- Unresolved Question #2 (digest vs. per-item mutual exclusivity) is already correctly flagged
  in the plan with an owner (Tyler) — once resolved, encode the decision as a named trigger-mode
  type (e.g. an enum-shaped `NotificationTriggerMode`) rather than the implicit
  bool-plus-int-plus-branch combination in Task 1.3.1b, so a future third mode ("both") doesn't
  require another ad hoc boolean layered on top.
- Task 1.1.2c: `SlackWebhookURLOverride()`/`SlackSigningSecretOverride()` are plain
  pass-through getters with no validation, consistent with existing `AnthropicAPIKey`-style
  getters elsewhere in `config.go` — not a new pattern, but worth confirming during
  implementation that the env-var path and the UI path (which does validate the
  `https://hooks.slack.com/services/` prefix in Task 1.4.2b) don't silently diverge in what
  they accept.
