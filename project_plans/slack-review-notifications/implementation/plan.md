# Implementation Plan: slack-review-notifications

**Feature**: Push review-queue and approval-pending notifications to Slack via an incoming
webhook (Phase 1), with an optional gated inbound approve/deny callback (Phase 2).
**Date**: 2026-08-06
**Status**: Ready for implementation
**ADRs**: [ADR-001: Encrypt Slack Webhook URL and Signing Secret at Rest](../decisions/ADR-001-slack-secret-storage-encryption.md)

---

## Step 0.5 — Alternatives Considered (Creative Pass)

Three distinct ways the Slack notifier could hook into the codebase were evaluated before
committing to an architecture:

1. **Direct calls from the two known producer call sites** (`ReactiveQueueManager.OnItemAdded`,
   `ApprovalHandler.broadcastApprovalNotification`).
   *Strength*: both call sites already know exactly which event is "review-queue item" vs.
   "approval pending" — no new discriminating signal has to be invented anywhere else in the
   codebase.
   *Weakness*: touches two call sites instead of one central chokepoint, so a third future
   producer of "needs Slack" events would need its own direct call too.

2. **Generic `EventBus` subscriber** (a third subscriber alongside
   `server/notifications/subscriber.go`, filtering `events.Event` by `NotificationType`).
   *Strength*: one subscription point, structurally identical to the existing in-app
   notification coalescer — reuses a channel-based pattern already proven in this codebase.
   *Weakness*: `events.NewNotificationEvent` is also published by `capacity_monitor.go`,
   `autonomous_orchestration_service.go`, `backlog_notifier.go`, `session_service.go`,
   `backlog_service_triage.go`, and `mcp/tools_backlog.go` — none of them Slack-worthy —  and
   `NotificationType` alone (`_INFO`, `_STATUS_CHANGE`, etc.) does not reliably separate
   "review queue item worth a Slack ping" from routine idle/stale churn that maps through the
   same type values. Making this filter reliable would require adding a new discriminating
   field to every existing producer — a wider, more invasive change than the feature itself.

3. **Generic `NotificationSink` interface on `NotificationService`**, with Slack as one
   pluggable implementation.
   *Strength*: textbook "open for extension" shape, would make a second future sink (e.g.
   Discord) purely additive.
   *Weakness*: `NotificationService` (`server/services/notification_service.go`, read in full —
   332 lines) has no sink abstraction today, and only one Slack implementation exists or is
   planned. Per `.claude/rules/interface-pollution-checklist.md`'s "speculative interface"
   smell, introducing an interface for a single implementer that solves no current problem is
   the exact anti-pattern this repo actively guards against.

**Decision**: Option 1 — direct calls from the two producer call sites — confirmed as the
strongest approach, validating (not merely accepting) `research/architecture.md`'s
recommendation. Options 2 and 3 are recorded as rejected in the Pattern Decisions table below.

---

## Domain Glossary

| Term | Definition | Notes |
|------|-----------|-------|
| `SlackConfig` | Nested config struct (`config/types.go`) embedded on `config.Config` as `Slack SlackConfig`, holding all Slack integration settings. | Mirrors `HibernationConfig`/`CapacityConfig` convention. |
| `WebhookURLEncrypted` | `SlackConfig` field: AES-256-GCM ciphertext (base64) of the Slack Incoming Webhook URL, produced by `session.EncryptToken`. | Never the plaintext; see ADR-001. |
| `SigningSecretEncrypted` | `SlackConfig` field: ciphertext of the Phase 2 Slack app Signing Secret. | Only meaningful when `ApprovalEnabled` is true. |
| `SlackWebhookURLOverride` / `SlackSigningSecretOverride` | Unexported `config.Config` fields (and their exported getter methods) populated from `SLACK_WEBHOOK_URL`/`SLACK_SIGNING_SECRET` env vars at load time. Never persisted to `config.json`. | Mirrors `ANTHROPIC_API_KEY`'s env-override convention. |
| `QueueDepthThreshold` | `SlackConfig` int field: review-queue item count that, once crossed, fires a single digest Slack message instead of (or in addition to) per-item messages. | 0 = disabled. |
| `DashboardBaseURL` | `SlackConfig` string field: the externally-reachable base URL used to build "view in dashboard" links in Slack messages. Falls back to `http://<listen address>` (unusable from a phone) when unset. | Named gap from `research/ux.md` §"Link-back correctness." |
| `SlackNotifier` | Concrete type, `server/services/slack_notifier.go`. Owns the `http.Client`, builds Block Kit payloads, POSTs to the configured webhook, tracks delivery status and the queue-depth-threshold latch. No interface — single implementation (interface-pollution-checklist). | Constructed once in `server/dependencies.go`, shared across all consumers. |
| `NotifyReviewQueueItem` | `SlackNotifier` method called from `ReactiveQueueManager.OnItemAdded` for a new review-queue item. | |
| `NotifyApprovalPending` | `SlackNotifier` method called from `ApprovalHandler.broadcastApprovalNotification` for a new pending approval. | |
| `MaybeNotifyQueueDepthThreshold` | `SlackNotifier` method implementing the edge-triggered digest trigger — fires once per crossing above `QueueDepthThreshold`, resets when depth drops back below. | |
| `SlackNotifierWiring` (interface) | Narrow interface declared in `server/review_queue_manager.go` (consumer package), with just the one method `ReactiveQueueManager` needs. Mirrors `OneShotPRCreator`. Satisfied implicitly by `*services.SlackNotifier`. | `ApprovalHandler` needs no equivalent interface — it's in the same package (`services`) as `SlackNotifier`, so it holds the concrete type directly. |
| `slackWebhookPayload` / `slackBlock` / `slackBlockText` | Plain `encoding/json`-tagged structs representing a Slack Block Kit message. | No SDK; see build-vs-buy.md. |
| `truncateForSlackBlock` | Rune-safe truncation helper capping a string at `maxSlackBlockTextLen` runes with a `"... truncated, see dashboard"` suffix. | Mirrors `truncateString`/`maxNotificationMessageLen` in `approval_handler.go`. |
| `SlackDeliveryStatus` | In-memory state on `SlackNotifier` (`lastDeliveryAt`, `lastDeliverySuccess`, `lastDeliveryError`, mutex-guarded) plus its proto projection `SlackDeliveryStatus` message. Surfaced by `GetSlackConfig`. | Closes the "silent failure" gap named in `research/ux.md` §4 and `research/pitfalls.md` §6. |
| `SlackConfigService` | New concrete type, `server/services/slack_config_service.go`, implementing `GetSlackConfig`/`UpdateSlackConfig`/`TestSlackWebhook`. Delegated to from `SessionService` exactly like `DefaultsService`. | |
| `SlackConfigProto` | New proto message (`proto/session/v1/session.proto`) — the masked, frontend-safe projection of `SlackConfig` (booleans for "is a secret configured," never the secret itself). | |
| `SlackNotificationSettings` | New React component, `web-app/src/components/settings/SlackNotificationSettings.tsx`. Webhook URL field, two toggles, queue-depth-threshold input, "Send test message" button, last-delivery status line. | Follows `PushNotificationSettings.tsx`/`BacklogSourcesSettings.tsx`/`CronScheduleInput.tsx` conventions. |
| `SlackInteractivePayload` (Phase 2) | Decoded Go struct for the JSON Slack sends URL-encoded in the `payload` form field of an interactive-component callback. | `server/services/slack_interactive_handler.go`. |
| `verifySlackSignature` (Phase 2) | Stdlib HMAC-SHA256 verification function implementing Slack's `v0:{timestamp}:{raw_body}` scheme with `hmac.Equal` and a 5-minute replay window. | `server/services/slack_signature.go`. |
| `SlackApprovalActionValue` (Phase 2) | The `value` string encoded on a Slack Block Kit button (`approvalID + ":" + decision`), decoded by the interactive handler to call `ApprovalService.ResolveApproval`. | |

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| Notifier hookup | Direct calls from `ReactiveQueueManager.OnItemAdded` / `ApprovalHandler.broadcastApprovalNotification` | Step 0.5, `research/architecture.md` | Generic `EventBus` subscriber (3rd bus subscriber) | `NotificationType` doesn't reliably discriminate review-queue/approval events from routine idle/stale/status-change churn published by 6+ other producers; making it reliable would require adding a new field to every existing producer — more invasive than two direct calls. |
| Notifier hookup | Same as above | Step 0.5 | Generic `NotificationSink` interface on `NotificationService` | Single implementer, no second sink planned — textbook speculative interface per `.claude/rules/interface-pollution-checklist.md`. |
| Cross-package wiring | Narrow consumer-declared interface (`SlackNotifierWiring`, mirrors `OneShotPRCreator`) in `server/review_queue_manager.go`; concrete `*SlackNotifier` field (no interface) in `server/services/approval_handler.go` | `research/architecture.md`, `.claude/rules/interface-pollution-checklist.md` | Interface declared in `server/services` next to `SlackNotifier` | Violates "define interfaces in the consumer package" convention; also unnecessary for `ApprovalHandler`, which is in the same package as `SlackNotifier` and can hold the concrete type directly. |
| Secret storage | AES-256-GCM via existing `MachineEncryptionKey` + `session.EncryptToken`/`DecryptToken` | ADR-001, `research/stack.md` | Plaintext field like `AnthropicAPIKey` | Slack webhook URL is a bare bearer credential (whole capability in the URL); Phase 2 signing secret gates agent-approval authority — higher blast radius than an API key if `config.json` leaks. Reuses an existing primitive at near-zero cost. |
| Secret storage | Fresh `config.LoadConfig()` + resolve-on-every-send (no cache) | ADR-001 Consequences | Cache decrypted secret in `SlackNotifier` at construction, refresh on config save | Avoids inventing a cache-invalidation/refresh path; matches the repo's existing "fresh-load, last-write-wins" convention (`defaults_service.go`'s own doc comment on `UpdateGlobalDefaults`); AES-GCM decrypt cost is microseconds at human-latency notification cadence. |
| Message construction | Hand-rolled `encoding/json` structs (Transaction Script) | `research/build-vs-buy.md`, `research/stack.md` | `github.com/slack-go/slack` SDK | Matches repo's established house style (stdlib-first, no SDK for a well-documented single-purpose recipe — see `push_service.go` precedent); avoids widening audit surface on a security-adjacent feature for near-zero benefit. |
| Burst handling | Edge-triggered threshold latch only (fire once per crossing, reset below threshold) — **correction (triad engineering review):** no separate coalescing window is implemented anywhere in Epic 1.2; per-item sends via `NotifyReviewQueueItem` are not rate-limited. A burst of items arriving with `QueueDepthThreshold` disabled (`0`) or below threshold sends one immediate POST per item, which can trip Slack's ~1/sec limit. This is an accepted Medium-appetite gap, not a built mitigation — the digest latch is the only relief valve, and only once depth crosses the configured threshold. | `research/pitfalls.md` §2, `research/architecture.md` §5 | Directly reuse `server/notifications/subscriber.go`'s 500ms coalescing window; a real global 1/sec token bucket | 500ms is documented in `research/pitfalls.md` as "too fast" for Slack's 1/sec cap. A token-bucket rate limiter was considered but not built — out of scope for Medium appetite; single-user send volume (at most a few/hour) makes sustained 429s unlikely outside a large queue-depth burst, which the digest latch already dampens. |
| Settings RPC surface | New `GetSlackConfig`/`UpdateSlackConfig`/`TestSlackWebhook` RPCs on `SessionService`, delegated to a new `SlackConfigService` (mirrors `DefaultsService`/`GetSessionDefaults`/`UpdateGlobalDefaults`) | This plan's grounding pass (`server/services/defaults_service.go`) | Extend `ConfigService` (`server/services/config_service.go`) | `ConfigService` is scoped specifically to Claude CLI config *files* (`GetClaudeConfig`/`UpdateClaudeConfig`) — a different concern from `config.json`-backed app settings, which is what `DefaultsService` already handles for an analogous feature. |
| Phase 2 endpoint | New dedicated `http.HandlerFunc` at `/api/hooks/slack-interactive`, verifies signature first, then calls `ApprovalService.ResolveApproval` in-process | `research/architecture.md` §4 | Extend `/api/hooks/permission-request` | Opposite direction (creation vs. resolution) and opposite trust model (implicit-localhost vs. signed-internet-facing); merging would force branching on payload shape before establishing trust — the constraints explicitly forbid that ordering. |

---

## Migration Plan

*(Omitted — no schema or data changes. `config.json` gains new optional/`omitempty` fields;
existing configs load unchanged with `Slack` as its zero value, which the "clean no-op when
unconfigured" design in Epic 1.2 already requires.)*

## Observability Plan

- **Logs**: `SlackNotifier` logs at `log.Warn` on any send failure (non-2xx, timeout, DNS
  error) and `log.Info` on the first successful send after a prior failure (state transition,
  not every success — avoids log spam on a working integration). **Never** logs
  `webhookURL`/`signingSecret`/decoded payload values — enforced by Task 1.5.1d's grep-based
  test.
- **Metrics**: none added — feature volume (at most a few sends/hour for a single user) does
  not justify new metrics infrastructure; `SlackDeliveryStatus` (surfaced in the settings UI)
  is the user-facing observability signal instead.
- **Alerts**: none — this feature *is* the alerting mechanism for other conditions; alerting on
  its own failure would be infinite regress (named explicitly in `research/ux.md` §4). The
  settings-page "last delivery failed" banner is the terminal signal.

## Risk Control

- **Feature flag**: `cfg.Slack.WebhookURLEncrypted == ""` (post-decrypt: no webhook configured)
  is the Phase 1 kill switch — every send path no-ops cleanly, mirroring `DomainAgeChecker`'s
  `enabled bool` early-return pattern. `cfg.Slack.ApprovalEnabled` (default `false`) gates all
  of Phase 2, including whether the `/api/hooks/slack-interactive` route is even registered.
- **Rollback procedure**: unset `slack_webhook_url` in the settings UI (or delete the `slack`
  block from `config.json` and restart) — no data migration to reverse, no schema to roll back.
- **Staged rollout**: not applicable (single-user, self-hosted, no fleet). Manual verification
  per `requirements.md`'s Success Metrics: trigger one review-queue item and one approval
  request against a real (or throwaway) Slack webhook before considering Epic 1.3 done.
- **Rollout sequencing (pre-mortem.md Failure #1, P1)**: do not let the first-ever activation of
  `NotifyOnQueueItem`/`QueueDepthThreshold` coincide with the `make install-service` deploy that
  ships this feature. On Linux, a service restart currently kills the tmux server and every live
  session (`.claude/rules/tmux-keep-server-on-restart.md`), which get silently rebuilt and can
  land in the review queue simultaneously — producing a false-alarm burst of Slack messages for
  sessions that didn't actually fail, as the very first thing the user sees from this feature.
  Ship with both toggles defaulted off (already true — `SlackConfig{}`'s zero value); the user
  should enable them only *after* confirming the queue is in a normal, non-restart-churned state
  post-deploy, not as part of the same install step.

## Unresolved Questions

- [x] Should `DashboardBaseURL` have a required-if-webhook-configured validation, or ship as a
      soft, documented gap? — **Resolved**: soft, documented gap (UX-9 option (b)) — a visible
      `dashboard_base_url` field (Task 1.4.3f) plus a dismissable warning when unset, not a hard
      gate. See Task 1.4.3f.
- [ ] Should the per-item and queue-depth-threshold triggers be mutually exclusive per config,
      or allowed to both fire for the same burst (`research/architecture.md` §5 leaves this
      open)? This plan defaults to **mutually exclusive when both would fire in the same
      `OnItemAdded` call** (threshold digest wins, per-item suppressed) — confirm this is the
      desired behavior before Task 1.2.4b ships. — blocks Story 1.2.4 — owner: Tyler.

## Dependency Visualization

```
Epic 1.1 (Config)
   |
   v
Epic 1.2 (SlackNotifier) ---------+
   |                              |
   v                              v
Epic 1.3 (Wiring)          Epic 1.4 (Settings RPC + UI)
   |                              |
   +--------------+---------------+
                  v
           Epic 1.5 (Tests)
                  |
                  v
   Phase 2 / Epic 2.1 (Inbound, gated by ApprovalEnabled)
```

---

# Phase 1

## Epic 1.1: Config

**Goal**: `config.Config` gains a `Slack` block covering both phases' settings, with the
encrypted-secret storage decided in ADR-001 and an env-var override escape hatch matching
`AnthropicAPIKey`'s convention.

### Story 1.1.1: `SlackConfig` struct
**As a** developer wiring the Slack notifier, **I want** a single typed config struct for all
Slack settings, **so that** Phase 1 and Phase 2 fields live in one place following the
`HibernationConfig`/`CapacityConfig` convention.
**Acceptance Criteria**:
- `config.SlackConfig` exists with `WebhookURLEncrypted`, `SigningSecretEncrypted`,
  `NotifyOnQueueItem`, `QueueDepthThreshold`, `ApprovalEnabled`, `DashboardBaseURL`.
  - *Given* a fresh `config.json` with no `slack` key, *when* `config.LoadConfig()` runs,
    *then* `cfg.Slack` is the zero-value `SlackConfig{}` (all fields empty/false/0) and no
    error is returned.
**Files**: `config/types.go`

##### Task 1.1.1a: Add `SlackConfig` struct (~3 min)
- Add to `config/types.go`, near `HibernationConfig` (line 12), with a doc comment
  referencing ADR-001 for why the two secret fields are ciphertext.
- Files: `config/types.go`

### Story 1.1.2: Wire `SlackConfig` into `Config` + env var overrides
**As a** developer running stapler-squad from a dev shell, **I want** `SLACK_WEBHOOK_URL` /
`SLACK_SIGNING_SECRET` env vars to override the stored config, **so that** local testing
doesn't require going through the encrypted settings UI, matching `ANTHROPIC_API_KEY`.
**Acceptance Criteria**:
- `cfg.Slack SlackConfig` field exists on `config.Config`.
  - *Given* `config.json` contains `"slack": {"notify_on_queue_item": true, "queue_depth_threshold": 5}`, *when* `config.LoadConfig()` runs, *then* `cfg.Slack.NotifyOnQueueItem == true` and `cfg.Slack.QueueDepthThreshold == 5`.
- `SLACK_WEBHOOK_URL` env var, when set, is readable via `cfg.SlackWebhookURLOverride()`.
  - *Given* the process env has `SLACK_WEBHOOK_URL=https://hooks.slack.com/services/T0/B0/X0`
    and `config.json` has no `slack.webhook_url_encrypted`, *when* `config.LoadConfig()` runs,
    *then* `cfg.SlackWebhookURLOverride() == "https://hooks.slack.com/services/T0/B0/X0"`.
**Files**: `config/config.go`

##### Task 1.1.2a: Add `Slack` field + unexported override fields to `Config` (~4 min)
- Add `Slack SlackConfig \`json:"slack,omitempty"\`` to the `Config` struct near the
  `Hibernation HibernationConfig` field (`config/config.go:337`).
- Add unexported `slackWebhookURLOverride string` / `slackSigningSecretOverride string`
  fields near the existing unexported `executor CommandExecutor` field (`config/config.go:232`)
  — same "not serialized" pattern, no `json:"-"` tag needed since the field is unexported.
- Files: `config/config.go`

##### Task 1.1.2b: Read env var overrides in both config-load paths (~4 min)
- Mirror the two existing `ANTHROPIC_API_KEY` read sites (`config/config.go:481` in
  `DefaultConfig()`, and `config/config.go:919`): add
  `if v := os.Getenv("SLACK_WEBHOOK_URL"); v != "" { cfg.slackWebhookURLOverride = v }` and the
  same for `SLACK_SIGNING_SECRET` → `slackSigningSecretOverride`, at both sites.
- Files: `config/config.go`

##### Task 1.1.2c: Add exported getter methods (~2 min)
- `func (c *Config) SlackWebhookURLOverride() string { return c.slackWebhookURLOverride }` and
  the signing-secret equivalent — needed because `server/services` (a different package)
  cannot read unexported fields directly.
- Files: `config/config.go`

### Story 1.1.3: Config round-trip test
**As a** developer, **I want** a test proving env override precedence and JSON round-tripping
of `SlackConfig`, **so that** a future refactor of `LoadConfig`/`DefaultConfig` can't silently
break the override escape hatch.
**Acceptance Criteria**:
- A test asserts override precedence over a stored (encrypted) value.
  - *Given* `config.json` has a non-empty `slack.webhook_url_encrypted` AND the env has
    `SLACK_WEBHOOK_URL` set, *when* `resolveSlackWebhookURL` (Task 1.2.1c, tested here via a
    config-level fixture) is invoked, *then* the env value wins — this is asserted at the
    `config` package level for the override getter, and end-to-end in Task 1.5.1a.
**Files**: `config/config_test.go`

##### Task 1.1.3a: `TestLoadConfig_SlackEnvOverride_TakesPrecedenceOverStoredValue` (~4 min)
- Set `t.Setenv("SLACK_WEBHOOK_URL", "https://hooks.slack.com/services/T0/B0/TEST")`, write a
  temp `config.json` with a dummy `slack.webhook_url_encrypted` value, load via
  `config.LoadConfigFromPath`, assert `cfg.SlackWebhookURLOverride()` returns the env value.
- Files: `config/config_test.go`

### Story 1.1.4: Extend `secret_scanner.go`'s redaction pattern to Slack webhook URLs
**As a** user pasting a webhook URL into a terminal/session that gets captured, **I want** it
redacted the same way an `xox[boas]-` Slack token already is, **so that** a pasted-then-visible
webhook URL doesn't leak via scrollback capture (`research/features.md`'s recommendation,
flagged as a Minor in adversarial-review.md with zero prior plan coverage).
**Acceptance Criteria**:
- `secret_scanner.go`'s pattern list redacts `https://hooks.slack.com/services/...` the same way
  it already redacts `xox[boas]-...` tokens.
  - *Given* captured text containing `https://hooks.slack.com/services/T0/B0/XXXXXXXXXXXXXXXXXXXXXXXX`,
    *when* the secret scanner processes it, *then* the URL is replaced with a redaction marker,
    matching the existing token-redaction behavior's shape.
**Files**: `server/services/secret_scanner.go`

##### Task 1.1.4a: Add the webhook-URL redaction pattern + test (~4 min)
- Add a regex alongside the existing `xox[boas]-` pattern; add one test case to whatever test
  file already covers `secret_scanner.go`'s existing patterns.
- Files: `server/services/secret_scanner.go`, its existing test file

---

## Epic 1.2: Backend Notifier

**Goal**: `server/services/slack_notifier.go` — a concrete `SlackNotifier` that formats Block
Kit messages, POSTs them non-blockingly, truncates oversized content, coalesces queue-depth
bursts, and tracks delivery status. No interface (single implementation).

### Story 1.2.1: `SlackNotifier` core type + secret resolution + HTTP POST
**As a** developer wiring notifications, **I want** a `SlackNotifier` that resolves the
configured webhook and performs the actual POST, **so that** both trigger points (Epic 1.3)
have one thing to call.
**Acceptance Criteria**:
- `NewSlackNotifier()` constructs a notifier with a 5-second-timeout `http.Client`.
  - *Given* `NewSlackNotifier()` is called with no arguments, *when* the returned
    `*SlackNotifier`'s `httpClient.Timeout` is inspected, *then* it equals `5 * time.Second`.
- `resolveSlackWebhookURL(cfg)` prefers the env override, else decrypts
  `cfg.Slack.WebhookURLEncrypted`, else returns `""` with no error.
  - *Given* `cfg.Slack.WebhookURLEncrypted == ""` and `cfg.SlackWebhookURLOverride() == ""`,
    *when* `resolveSlackWebhookURL(cfg)` is called, *then* it returns `("", nil)` — the "not
    configured" clean no-op case.
- `postToSlack` never includes the webhook URL in any returned/logged error.
  - *Given* a POST to an unreachable URL fails with a network error, *when* `postToSlack`
    logs the failure via `log.Warn`, *then* the log call's field list contains no
    `webhookURL`/`url` key (verified by Task 1.5.1d's grep test, not by this task).
**Files**: `server/services/slack_notifier.go` (new)

##### Task 1.2.1a: Scaffold `SlackNotifier` struct + constructor (~4 min)
- `type SlackNotifier struct { httpClient *http.Client; mu sync.Mutex; thresholdCrossed bool; lastDeliveryAt time.Time; lastDeliverySuccess bool; lastDeliveryError string }`.
- `func NewSlackNotifier() *SlackNotifier` — `httpClient: &http.Client{Timeout: 5 * time.Second}`,
  mirrors `domain_checker.go`'s `http.Client{Timeout: 3 * time.Second}` shape.
- Files: `server/services/slack_notifier.go`

##### Task 1.2.1b: `resolveSlackWebhookURL` / `resolveSlackSigningSecret` (~5 min)
- Package-level functions (not methods — they only need `*config.Config`):
  ```go
  func resolveSlackWebhookURL(cfg *config.Config) (string, error) {
      if v := cfg.SlackWebhookURLOverride(); v != "" {
          return v, nil
      }
      if cfg.Slack.WebhookURLEncrypted == "" {
          return "", nil
      }
      key, err := cfg.GetOrCreateEncryptionKey()
      if err != nil {
          return "", fmt.Errorf("resolve slack webhook url: %w", err)
      }
      return session.DecryptToken(key, cfg.Slack.WebhookURLEncrypted)
  }
  ```
  Mirrors `backlog_service_lifecycle.go:67-71`'s exact `cfg.GetOrCreateEncryptionKey()` →
  `session.DecryptToken(...)` call shape. `resolveSlackSigningSecret` is the same shape against
  `cfg.Slack.SigningSecretEncrypted`/`SlackSigningSecretOverride()`.
- Files: `server/services/slack_notifier.go`

##### Task 1.2.1c: `postToSlack` HTTP POST helper (~5 min)
- `func (n *SlackNotifier) postToSlack(ctx context.Context, webhookURL string, payload slackWebhookPayload) error` —
  `json.Marshal`, `http.NewRequestWithContext`, `Content-Type: application/json`, `n.httpClient.Do`.
  **Never propagate the raw `error` returned by `n.httpClient.Do`** — Go's stdlib wraps transport
  failures (DNS, connection refused, TLS) in `*url.Error`, whose `Error()` string embeds the full
  request URL (`fmt.Sprintf("%q: %s", e.URL, e.Err)`), so passing it through `%w`/`log.Warn`
  leaks the webhook credential on exactly the failure paths this AC exists to protect. Instead,
  on any `Do` error, construct a fresh sanitized error (`errors.New("slack webhook request
  failed: network error")` — no wrapping) and log that. On non-2xx, construct an error carrying
  only the status code (never the URL). Updates `n.lastDeliveryAt`/`lastDeliverySuccess`/
  `lastDeliveryError` under `n.mu` on every call (success or failure) with only the sanitized
  message — this is the single write point `GetSlackConfig` (Epic 1.4) reads.
- Files: `server/services/slack_notifier.go`

##### Task 1.2.1d: Regression test for transport-level URL leakage (~4 min, folds into Task 1.5.1d)
- Task 1.5.1d's log-capture test must include a case that forces a genuine transport-level
  failure (e.g. `httptest.NewServer(...)` immediately `.Close()`d, or `http://127.0.0.1:1`) —
  not only a non-2xx `httptest.Server` response — since `*url.Error` leakage only occurs on the
  `httpClient.Do` error path, a different code path than a non-2xx status. Both cases must be
  asserted free of the webhook URL substring in captured log output.
- Files: `server/services/slack_notifier_test.go`

### Story 1.2.2: Block Kit message builders + truncation
**As a** user receiving a Slack notification, **I want** a concise message with session name,
reason, and a working dashboard link, **so that** I can act without opening the app first.
**Acceptance Criteria**:
- `NotifyReviewQueueItem` builds a message whose primary block text includes the session name
  and reason, and whose diff-stats content (if present) is capped at `maxSlackBlockTextLen`.
  - *Given* a `session.ReviewItem{SessionName: "fix-login-bug", Reason: session.ReasonTestsFailing, DiffStats: &git.DiffStats{Content: strings.Repeat("+line\n", 2000)}}` (`session.ReviewItem`/`ReasonTestsFailing` are type aliases for `session/queue.ReviewItem`/`ReasonTestsFailing`, `session/review_queue.go:12,38`),
    *when* `NotifyReviewQueueItem` builds the payload, *then* the block containing the diff
    content has `len(text.Text) <= maxSlackBlockTextLen` and ends with
    `"... truncated, see dashboard"`.
- `NotifyApprovalPending` builds a message using the approval's tool name and a link built from
  `routes.sessionDetail`-equivalent (`/?session=<id>`).
  - *Given* `approval := &PendingApproval{ID: "appr-123", SessionID: "sess-1", ToolName: "Bash"}`
    and `dashboardURL := "https://home.example.com"`, *when* `NotifyApprovalPending` builds the
    payload, *then* one block's text contains the literal link
    `"https://home.example.com/?session=sess-1"`.
**Files**: `server/services/slack_notifier.go`

##### Task 1.2.2a: `slackWebhookPayload`/`slackBlock`/`slackBlockText` structs (~3 min)
- Plain JSON structs per `research/stack.md`'s recommended shape (`Text`, `Blocks []slackBlock`;
  `slackBlock{Type, Text *slackBlockText}`; `slackBlockText{Type, Text}`).
- Files: `server/services/slack_notifier.go`

##### Task 1.2.2b: `maxSlackBlockTextLen` const + `truncateForSlackBlock` (~3 min)
- `const maxSlackBlockTextLen = 2900` (headroom under Slack's 3000-char hard limit, mirrors
  `approval_handler.go`'s `maxNotificationMessageLen`/`maxEscalationReasonLen` pattern).
- `func truncateForSlackBlock(s string, maxRunes int) string` — rune-safe, same shape as
  `truncateString` (`approval_handler.go:568`) but with the `"... truncated, see dashboard"`
  suffix instead of `"..."`.
- Files: `server/services/slack_notifier.go`

##### Task 1.2.2c: `NotifyReviewQueueItem` (~5 min)
- `func (n *SlackNotifier) NotifyReviewQueueItem(ctx context.Context, cfg *config.Config, item *session.ReviewItem, dashboardURL string)`
  — resolves webhook URL, no-ops if empty; builds blocks (session name + reason as primary
  line, `item.Context` truncated via `sanitizeNotificationText`-equivalent, diff stats via
  Task 1.2.2b if `item.DiffStats != nil`); calls `postToSlack`; logs+swallows any error
  (`log.Warn`, never returns an error to the caller — enforced by the caller always invoking
  this via the dispatch wrapper in Story 1.2.3, not by this method's own signature).
- Files: `server/services/slack_notifier.go`

##### Task 1.2.2d: `NotifyApprovalPending` (~5 min)
- `func (n *SlackNotifier) NotifyApprovalPending(ctx context.Context, cfg *config.Config, approval *PendingApproval, sessionName, dashboardURL string)`
  — resolves webhook URL, no-ops if empty; builds blocks using `approval.ToolName` and
  `buildApprovalMessage(approval)` (reused from `approval_handler.go:602`, truncated via Task
  1.2.2b); link is `dashboardURL + "/?session=" + approval.SessionID` (mirrors
  `web-app/src/lib/routes.ts:31`'s `sessionDetail`).
- Files: `server/services/slack_notifier.go`

### Story 1.2.3: Non-blocking dispatch wrapper
**As a** review-queue/approval code path, **I want** the Slack POST to never add latency or
crash the process, **so that** the NFR ("must not block or fail the underlying flow") holds
even during a Slack outage.
**Acceptance Criteria**:
- **Ownership (resolves adversarial-review.md blocker #2 — the plan previously implied three
  different owners for this guarantee across Tasks 1.2.3a/1.3.1b/1.3.2b):** `NotifyReviewQueueItem`,
  `NotifyApprovalPending`, and `MaybeNotifyQueueDepthThreshold` each wrap their own body in
  `n.dispatchAsync` internally and return `void`/`bool` immediately, before the HTTP POST
  completes. Callers (`OnItemAdded`, `broadcastApprovalNotification`) call these methods
  **directly, synchronously, with no `go`/wrapper of their own** — the non-blocking guarantee is
  entirely inside `SlackNotifier`, never the caller's responsibility. This is the single
  authoritative statement; Tasks 1.3.1b and 1.3.2b's code samples are written to match it exactly.
  - *Given* `NotifyReviewQueueItem` is called directly (no external `go`/dispatch wrapper) against
    a webhook URL whose server sleeps 10s before responding, *when* the call returns, *then* it
    returns in well under 10s (proves the method itself is fire-and-forget, not the caller).
  - *Given* `NotifyReviewQueueItem` panics internally (e.g. a nil `item.DiffStats.Content` edge
    case not otherwise reachable), *when* it is called, *then* the panic is recovered inside the
    method, logged via `log.Error`, and the calling goroutine (`OnItemAdded`) is unaffected.
**Files**: `server/services/slack_notifier.go`

##### Task 1.2.3a: `dispatchAsync` helper (~4 min)
- `func (n *SlackNotifier) dispatchAsync(ctx context.Context, fn func(ctx context.Context))`:
  ```go
  go func() {
      defer func() {
          if r := recover(); r != nil {
              log.Error("SlackNotifier: recovered from panic", "panic", r)
          }
      }()
      sendCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
      defer cancel()
      fn(sendCtx)
  }()
  ```
  `NotifyReviewQueueItem`/`NotifyApprovalPending`/`MaybeNotifyQueueDepthThreshold` (Stories 1.2.2,
  1.2.4) each call `n.dispatchAsync(ctx, ...)` as the **first line of their own method body** and
  return immediately after — `dispatchAsync` is a private implementation detail of `SlackNotifier`,
  never something a caller in Epic 1.3 invokes or wraps itself. This guarantees non-blocking +
  panic-safe behavior at every call site by construction, not by convention each call site must
  remember to apply correctly.
- Files: `server/services/slack_notifier.go`

### Story 1.2.4: Queue-depth threshold latch (edge-triggered digest)
**As a** user, **I want** a burst of review-queue items to produce one digest Slack message,
**so that** I'm not spammed and don't trip Slack's ~1 msg/sec rate limit.
**Acceptance Criteria**:
- Crossing `QueueDepthThreshold` fires exactly one message; staying above it fires none more;
  dropping below and re-crossing fires again.
  - *Given* `QueueDepthThreshold = 5` and the queue depth sequence `4, 5, 6, 7, 4, 6` is fed via
    successive `MaybeNotifyQueueDepthThreshold` calls, *when* all six calls complete, *then*
    exactly 2 Slack POSTs were made (one at depth `5`, one at the re-crossing at depth `6`).
- When a threshold digest fires for the same `OnItemAdded` call that would also fire a per-item
  message, only the digest fires (per this plan's Unresolved-Questions default).
  - *Given* `NotifyOnQueueItem = true`, `QueueDepthThreshold = 3`, and a new item arrives making
    the queue depth `3` (a fresh crossing), *when* `ReactiveQueueManager.OnItemAdded` processes
    it (Task 1.3.1b), *then* `NotifyApprovalPending`/`NotifyReviewQueueItem`'s per-item send is
    skipped for that call and only the digest send happens.
**Files**: `server/services/slack_notifier.go`

##### Task 1.2.4a: `MaybeNotifyQueueDepthThreshold` (~5 min)
- `func (n *SlackNotifier) MaybeNotifyQueueDepthThreshold(ctx context.Context, cfg *config.Config, depth, threshold int, dashboardURL string) (fired bool)`
  — under `n.mu`: if `threshold <= 0`, return `false`. If `depth >= threshold && !n.thresholdCrossed`,
  set `n.thresholdCrossed = true`, build and dispatch (via `dispatchAsync`) a digest message
  ("N items pending in the review queue" + dashboard link to `/review-queue`, per
  `web-app/src/lib/routes.ts:8`'s `reviewQueue` route), return `true`. If `depth < threshold`,
  reset `n.thresholdCrossed = false`, return `false`.
  **Concurrency note (pre-mortem.md Failure #2, P2):** the `depth` value itself is read by the
  caller (`OnItemAdded`, via `rqm.queue.GetStatistics().TotalItems`) *before* this method's own
  lock is taken, so two `OnItemAdded` calls racing on near-simultaneous queue mutations can each
  read a stale depth and both decide `!n.thresholdCrossed`, double-firing the digest (or, in the
  opposite race, both seeing a pre-crossing depth and neither firing). Because the entire
  check-and-set (`depth >= threshold && !n.thresholdCrossed` → set `true`) happens inside one
  `n.mu`-held critical section in this method, the digest itself is race-free *given* an accurate
  `depth` input — the residual risk is solely a stale `depth` snapshot racing another goroutine's
  queue mutation between the read in `OnItemAdded` and the call into this method. Task 1.5.1e's
  test must include a concurrent variant (N goroutines calling `MaybeNotifyQueueDepthThreshold`
  with a shared depth crossing the threshold) asserting exactly one digest fires under
  `go test -race`, in addition to the existing sequential-depth-sequence case.
- Files: `server/services/slack_notifier.go`

### Story 1.2.5: Delivery status accessor
**As a** settings-UI RPC handler, **I want** to read the notifier's last-send outcome, **so
that** `GetSlackConfig` (Epic 1.4) can surface it without re-implementing state tracking.
**Acceptance Criteria**:
- `GetDeliveryStatus()` returns a thread-safe snapshot.
  - *Given* the most recent `postToSlack` call failed with `"slack returned 404"`, *when*
    `n.GetDeliveryStatus()` is called concurrently from another goroutine, *then* it returns
    `(attempted=true, success=false, errMsg="slack returned 404", at=<that call's timestamp>)`
    without a data race (verified under `go test -race`).
**Files**: `server/services/slack_notifier.go`

##### Task 1.2.5a: `GetDeliveryStatus` accessor (~3 min)
- `func (n *SlackNotifier) GetDeliveryStatus() (attempted, success bool, errMsg string, at time.Time)`
  — reads the four fields set in Task 1.2.1c's `postToSlack` under `n.mu.Lock()`.
- Files: `server/services/slack_notifier.go`

---

## Epic 1.3: Wiring

**Goal**: One shared `*services.SlackNotifier` instance, always constructed and always wired
(self-gating per-call per Story 1.2.1/1.2.2's "no-op when unconfigured" behavior — no
conditional construction needed).

### Story 1.3.1: `ReactiveQueueManager` wiring
**As a** review-queue subsystem, **I want** `OnItemAdded` to notify Slack the same way it
already notifies the in-app `eventBus`, **so that** a new review-queue item reaches the user
even when the dashboard is closed.
**Acceptance Criteria**:
- `OnItemAdded` calls the Slack notifier under the same `item.Reason != session.ReasonApprovalPending && !suppressForHidden` guard as its existing `eventBus.Publish` call.
  - *Given* a `*session.ReviewItem{SessionID: "sess-1", Reason: session.ReasonTestsFailing}`
    is added to a `ReactiveQueueManager` with a wired `SlackNotifierWiring`, *when*
    `OnItemAdded(item)` runs, *then* `NotifyReviewQueueItem` is invoked exactly once, called
    directly/synchronously per Story 1.2.3's ownership model — the method itself is
    fire-and-forget internally (verified with a fake `SlackNotifierWiring` in the unit test).
  - *Given* the same setup but `item.Reason == session.ReasonApprovalPending`, *when*
    `OnItemAdded(item)` runs, *then* the Slack notifier is **not** invoked (avoids the
    duplicate-card problem the existing `eventBus.Publish` guard already documents at
    `server/review_queue_manager.go:389-391`).
**Files**: `server/review_queue_manager.go`, `server/dependencies.go`

##### Task 1.3.1a: `SlackNotifierWiring` interface + field + setter (~4 min)
- In `server/review_queue_manager.go`, near `OneShotPRCreator` (line 19-26): declare
  ```go
  // SlackNotifierWiring is the narrow subset of *services.SlackNotifier that
  // ReactiveQueueManager needs. Defined here — the consumer — per this repo's
  // anti-interface-pollution convention (mirrors OneShotPRCreator above).
  type SlackNotifierWiring interface {
      NotifyReviewQueueItem(ctx context.Context, cfg *config.Config, item *session.ReviewItem, dashboardURL string)
      MaybeNotifyQueueDepthThreshold(ctx context.Context, cfg *config.Config, depth, threshold int, dashboardURL string) bool
  }
  ```
  Add `slackNotifier SlackNotifierWiring` field to `ReactiveQueueManager` struct and
  `func (rqm *ReactiveQueueManager) SetSlackNotifier(n SlackNotifierWiring) { rqm.slackNotifier = n }`.
- Files: `server/review_queue_manager.go`

##### Task 1.3.1b: Call site in `OnItemAdded` + threshold check (~5 min)
- After the existing `rqm.eventBus.Publish(notifEvent)` block (`server/review_queue_manager.go:~411`),
  under the same `if` guard:
  ```go
  if rqm.slackNotifier != nil {
      cfg := config.LoadConfig()
      dashboardURL := cfg.Slack.DashboardBaseURL
      if dashboardURL == "" {
          dashboardURL = "http://" + /* srv addr, threaded in per Task 1.3.1c */
      }
      fired := rqm.slackNotifier.MaybeNotifyQueueDepthThreshold(rqm.baseContext(), cfg, rqm.queue.GetStatistics().TotalItems, cfg.Slack.QueueDepthThreshold, dashboardURL)
      if !fired && cfg.Slack.NotifyOnQueueItem {
          rqm.slackNotifier.NotifyReviewQueueItem(rqm.baseContext(), cfg, item, dashboardURL)
      }
  }
  ```
  (Implements the Unresolved Questions default: digest suppresses per-item on the same call.)
- Files: `server/review_queue_manager.go`

##### Task 1.3.1c: Wire in `server/dependencies.go` (~4 min)
- Add `SlackNotifier *services.SlackNotifier` field to `ServerDependencies` (near
  `ReactiveQueueMgr`, `server/dependencies.go:45`).
- Near `NewReactiveQueueManager` (`server/dependencies.go:800`): construct
  `slackNotifier := services.NewSlackNotifier()`, call
  `reactiveQueueMgr.SetSlackNotifier(slackNotifier)` (mirrors the adjacent
  `reactiveQueueMgr.SetOneShotRunner(sessionService)` at line 804), and assign
  `deps.SlackNotifier = slackNotifier` on the returned `ServerDependencies`.
- Thread a base-URL resolver into the notifier's call sites the same way `hookBaseURLFn` does
  for hook callbacks (`server/server.go:467-470`) — **pinned signature (architecture-review.md
  nitpick + triad engineering review both flagged this as needing to be pinned, not left open):**
  add a `dashboardBaseURLFn func() string` field to `ReactiveQueueManager` (mirrors
  `hookBaseURLFn`'s exact shape), set via a `SetDashboardBaseURLFn(fn func() string)` setter
  called from `server/dependencies.go` right after `Server` is constructed — `ReactiveQueueManager`
  is built before `Server` in the dependency wiring order (confirmed in `server/dependencies.go`),
  which is why this is a late-bound setter rather than a constructor argument, exactly like
  `hookBaseURLFn`'s own wiring. Task 1.3.1b's fallback becomes
  `dashboardURL := cfg.Slack.DashboardBaseURL; if dashboardURL == "" && rqm.dashboardBaseURLFn != nil { dashboardURL = rqm.dashboardBaseURLFn() }`.
- Files: `server/dependencies.go`, `server/review_queue_manager.go`

### Story 1.3.2: `ApprovalHandler` wiring
**As a** the approval flow, **I want** `broadcastApprovalNotification` to also notify Slack,
**so that** a pending permission request reaches the user even when the dashboard is closed.
**Acceptance Criteria**:
- `broadcastApprovalNotification` calls the Slack notifier right after its existing
  `h.eventBus.Publish(event)` call, nil-guarded.
  - *Given* an `ApprovalHandler` with a wired `*services.SlackNotifier` and
    `PendingApproval{ID: "appr-1", ToolName: "Bash", SessionID: "sess-1"}`, *when*
    `broadcastApprovalNotification("sess-1", approval)` runs, *then* `NotifyApprovalPending` is
    invoked exactly once, called directly/synchronously — the method is fire-and-forget
    internally per Story 1.2.3's ownership model.
  - *Given* `h.slackNotifier == nil` (not wired, e.g. in an existing unit test that doesn't call
    `SetSlackNotifier`), *when* `broadcastApprovalNotification` runs, *then* no panic occurs and
    behavior is identical to today (nil-guard, matching every other `Set*` optional dependency
    in this file).
**Files**: `server/services/approval_handler.go`, `server/server.go`

##### Task 1.3.2a: `slackNotifier` field + `SetSlackNotifier` setter (~3 min)
- Add `slackNotifier *SlackNotifier` to the `ApprovalHandler` struct (`server/services/approval_handler.go:68-83`,
  same package as `SlackNotifier` — concrete type, no interface, per the Pattern Decisions
  table) and `func (h *ApprovalHandler) SetSlackNotifier(n *SlackNotifier) { h.slackNotifier = n }`,
  placed alongside `SetDomainChecker`/`SetHeadlessPool` (line ~145-177).
- Files: `server/services/approval_handler.go`

##### Task 1.3.2b: Call site in `broadcastApprovalNotification` (~4 min)
- Right after `h.eventBus.Publish(event)` (`server/services/approval_handler.go:537`):
  ```go
  if h.slackNotifier != nil {
      cfg := config.LoadConfig()
      dashboardURL := cfg.Slack.DashboardBaseURL // fallback handled inside the notifier
      h.slackNotifier.NotifyApprovalPending(context.Background(), cfg, approval, h.resolveSessionName(sessionID), dashboardURL)
  }
  ```
  (`NotifyApprovalPending` itself performs the `dispatchAsync` wrapping internally per Task
  1.2.3a's design — no separate `go func` needed at this call site.)
- Files: `server/services/approval_handler.go`

##### Task 1.3.2c: Wire in `server.go` (~3 min)
- Near the other `approvalHandler.Set*` calls (`server/server.go:483-510`), add
  `approvalHandler.SetSlackNotifier(deps.SlackNotifier)` — reusing the *same* instance
  Task 1.3.1c constructed and stored on `deps.SlackNotifier`, so `GetDeliveryStatus()`
  (Epic 1.4) reflects sends from both trigger points.
- Files: `server/server.go`

---

## Epic 1.4: Settings UI + RPC + Feature Registry + E2E

**Goal**: A settings-page section to configure the webhook, toggles, and threshold; a "Send
test message" button; a passive last-delivery-status line — backed by new RPCs that never
round-trip the plaintext secret.

### Story 1.4.1: Proto RPCs
**As a** frontend developer, **I want** typed RPCs for reading/writing/testing Slack config,
**so that** the settings UI has a masked, safe contract to build against.
**Acceptance Criteria**:
- `GetSlackConfigResponse` never contains a plaintext secret field — only booleans.
  - *Given* `cfg.Slack.WebhookURLEncrypted` is a non-empty ciphertext, *when*
    `GetSlackConfig` is called, *then* the response's `SlackConfigProto.webhook_configured ==
    true` and no field in the response contains the decrypted URL or the ciphertext.
**Files**: `proto/session/v1/session.proto`

##### Task 1.4.1a: Add RPC signatures + request/response messages (~5 min)
- Add to `service SessionService` (near `GetSessionDefaults`/`UpdateGlobalDefaults`,
  `proto/session/v1/session.proto:248-255`):
  `rpc GetSlackConfig`, `rpc UpdateSlackConfig`, `rpc TestSlackWebhook`.
- Add messages `SlackConfigProto` (`webhook_configured`, `signing_secret_configured`,
  `notify_on_queue_item`, `queue_depth_threshold`, `approval_enabled`, `dashboard_base_url`,
  `last_delivery SlackDeliveryStatus`), `SlackDeliveryStatus` (`attempted`, `success`, `error`,
  `attempted_at google.protobuf.Timestamp`), `GetSlackConfigRequest/Response`,
  `UpdateSlackConfigRequest` (`webhook_url`, `signing_secret`, `notify_on_queue_item`,
  `queue_depth_threshold`, `approval_enabled`, `dashboard_base_url`, plus
  **`bool clear_webhook_url`, `bool clear_signing_secret`**), `UpdateSlackConfigResponse`,
  `TestSlackWebhookRequest` (`string webhook_url` — **not empty `{}`, fixed by triad UX review
  round 2's blocker**: `design/ux.md`'s Surface 1 interaction flow step 3 requires "Send test
  message" to test the value *currently typed in the form*, not necessarily the already-saved
  one — an empty request message can only ever test the persisted config, contradicting the
  documented "test before save" trust-building flow), `TestSlackWebhookResponse` (`success`, `error`).
  **Resolves architecture-review.md blocker**: empty string on `webhook_url`/`signing_secret`
  means "leave unchanged" (unedited field); the corresponding `clear_*` bool set true means
  "explicitly remove this secret" — collapsing both onto empty-string alone made "clear" an
  unreachable state, contradicting the plan's own Rollback procedure ("unset ... in the settings
  UI"). Simpler than proto3 `optional` presence-tracking and equally sufficient.
- Files: `proto/session/v1/session.proto`

##### Task 1.4.1b: Regenerate bindings (~2 min)
- Run `make proto-gen`; verify `session/gen/session/v1/*.go` and
  `web-app/src/gen/session/v1/*_pb.ts` regenerated cleanly (`go build ./...` passes).
- Files: `session/gen/session/v1/*.go` (generated), `web-app/src/gen/session/v1/*_pb.ts` (generated)

### Story 1.4.2: `SlackConfigService` backend implementation
**As a** the settings RPC layer, **I want** a service that encrypts on save, masks on read,
and performs the test send, **so that** the frontend never handles plaintext secrets or POST
logic directly.
**Acceptance Criteria**:
- `UpdateSlackConfig` with an empty `webhook_url` leaves the stored ciphertext unchanged.
  - *Given* `cfg.Slack.WebhookURLEncrypted` already holds a valid ciphertext, *when*
    `UpdateSlackConfig` is called with `webhook_url: ""` and `queue_depth_threshold: 8`,
    *then* the saved config's `WebhookURLEncrypted` is byte-identical to before, and
    `QueueDepthThreshold == 8`.
- `UpdateSlackConfig` with a non-empty `webhook_url` re-encrypts and persists it.
  - *Given* `UpdateSlackConfig` is called with `webhook_url: "https://hooks.slack.com/services/T0/B0/NEW"`,
    *when* the call returns successfully, *then* `session.DecryptToken(key, cfg.Slack.WebhookURLEncrypted)`
    (loaded fresh from disk) equals `"https://hooks.slack.com/services/T0/B0/NEW"`.
- `TestSlackWebhook` performs a real synchronous POST using the currently-saved config and
  reports the result without requiring the caller to have just called `UpdateSlackConfig`.
  - *Given* a saved webhook URL that returns HTTP 404 (revoked), *when* `TestSlackWebhook` is
    called, *then* the response has `success: false` and `error` containing `"404"` (Slack's
    literal status, not a generic message — per `research/ux.md`'s "surface Slack's actual
    error text" guidance).
**Files**: `server/services/slack_config_service.go` (new), `server/services/session_service.go`

##### Task 1.4.2a: `SlackConfigService` scaffold + `GetSlackConfig` (~5 min)
- `type SlackConfigService struct { slackNotifier *SlackNotifier }`,
  `func NewSlackConfigService(n *SlackNotifier) *SlackConfigService`.
- `GetSlackConfig`: `cfg := config.LoadConfig()`; build `SlackConfigProto` with
  `webhook_configured: cfg.Slack.WebhookURLEncrypted != "" || cfg.SlackWebhookURLOverride() != ""`
  (same for signing secret), plain-copy the non-secret fields, and
  `last_delivery` from `s.slackNotifier.GetDeliveryStatus()` (Task 1.2.5a).
  **Decrypt-health note (pre-mortem.md Failure #3, P2):** `webhook_configured` reflects
  ciphertext *presence*, not decrypt *success* — a machine-bound `MachineEncryptionKey` (Story
  1.1.1's ADR-001 reference) means restoring `config.json` onto a new/reinstalled host leaves
  the ciphertext permanently undecryptable while `webhook_configured` still reports `true`.
  Accepted for Phase 1: `GetDeliveryStatus()`'s `last_delivery` line already surfaces this
  indistinguishably from any other send failure (both look like "failed" with an error string),
  which is sufficient self-diagnosis for a single-user instance — a distinct "decryption failed,
  re-enter webhook URL" error variant is deferred as a future enhancement, not required for this
  Medium-appetite ship, since the existing `resolveSlackWebhookURL` error path (Task 1.2.1b) is
  the doctor to consult regardless of exact wording.
- Files: `server/services/slack_config_service.go`

##### Task 1.4.2b: `UpdateSlackConfig` (~5 min)
- `cfg := config.LoadConfig()`; precedence order: if `req.Msg.ClearWebhookUrl`, set
  `cfg.Slack.WebhookURLEncrypted = ""` and skip the validate/encrypt step below entirely; else if
  `req.Msg.WebhookUrl != ""`: validate it starts with `"https://hooks.slack.com/services/"`
  (return `connect.CodeInvalidArgument` otherwise, per `research/ux.md`'s "block save on
  client-side-shaped validation" — this is the server-side backstop), then
  `key, _ := cfg.GetOrCreateEncryptionKey()` →
  `cfg.Slack.WebhookURLEncrypted, _ = session.EncryptToken(key, req.Msg.WebhookUrl)`; else
  (`WebhookUrl == "" && !ClearWebhookUrl`) leave `cfg.Slack.WebhookURLEncrypted` untouched. Same
  three-way branch for `SigningSecret`/`ClearSigningSecret` (no format validation — opaque
  string). Copy toggle/threshold/dashboard-URL fields unconditionally (they have no "leave
  unchanged" semantics — always sent by the frontend form). `config.SaveConfig(cfg)`; return the
  same shape as `GetSlackConfig`.
- Files: `server/services/slack_config_service.go`

##### Task 1.4.2c: `TestSlackWebhook` (~5 min)
- If `req.Msg.WebhookUrl != ""` (the frontend sends whatever is currently typed in the form —
  tests the *unsaved* value first, per `design/ux.md` Surface 1 step 3/UX-1's "test before save"
  flow), validate it starts with `"https://hooks.slack.com/services/"` (same check as Task
  1.4.2b, `connect.CodeInvalidArgument` on mismatch) and use it directly — no decrypt needed,
  it's already plaintext from the form. Otherwise (`WebhookUrl == ""`, e.g. the user clicks
  "Send test message" without editing an already-configured field): `cfg := config.LoadConfig()`;
  `url, err := resolveSlackWebhookURL(cfg)`; if empty, return `success: false, error: "no webhook
  configured"`. Either way, build a canned payload (`"stapler-squad test message — if you can see
  this, your webhook is configured correctly."`) and call `s.slackNotifier.postToSlack(ctx, url,
  payload)` **synchronously** (not via `dispatchAsync` — the whole point is the caller waits for
  a real result), map the error (if any) to `success: false, error: err.Error()`. Note: testing
  an in-form URL never persists it — that only happens via a subsequent `UpdateSlackConfig` call
  (Task 1.4.2b), keeping "test" and "save" as the two distinct actions the UX flow describes.
- Files: `server/services/slack_config_service.go`

##### Task 1.4.2d: Delegate from `SessionService` (~3 min)
- Add `slackConfigSvc *SlackConfigService` field (near `defaultsSvc`,
  `server/services/session_service.go:125-126`), construct in `NewSessionService` alongside
  `defaultsSvc: NewDefaultsService()` (line 373), add three one-line delegating methods
  (`GetSlackConfig`/`UpdateSlackConfig`/`TestSlackWebhook`) mirroring
  `session_service.go:3362-3374`'s `GetSessionDefaults`/`UpdateGlobalDefaults` pattern.
- Files: `server/services/session_service.go`

### Story 1.4.3: `SlackNotificationSettings` component
**As a** user, **I want** a settings section to paste my webhook URL, toggle triggers, set a
threshold, and verify it works, **so that** I trust the integration before walking away from
the dashboard (per `research/ux.md`'s "I want to verify before I trust it" mental model).
**Acceptance Criteria**:
- Webhook URL field is masked after save (shows a "configured" indicator, not the value).
  - *Given* `GetSlackConfig` returns `webhook_configured: true`, *when* the settings page
    renders, *then* the URL input shows placeholder text like `"•••• (configured)"` rather
    than any real or fake URL value, and is empty-by-default for typing a replacement.
- "Send test message" button shows inline success/failure with Slack's actual error text.
  - *Given* the user clicks "Send test message" and `TestSlackWebhook` returns
    `{success: false, error: "slack returned 404: no_service"}`, *when* the response arrives,
    *then* a `role="alert"` region adjacent to the button displays text containing
    `"no_service"`.
- Toggles cannot be enabled without a configured webhook.
  - *Given* `webhook_configured: false` and the URL field is empty, *when* the user attempts to
    check "Notify on new review-queue item," *then* the checkbox is `disabled`.
**Files**: `web-app/src/components/settings/SlackNotificationSettings.tsx` (new), `web-app/src/components/settings/SlackNotificationSettings.css.ts` (new), `web-app/src/app/settings/page.tsx`

##### Task 1.4.3a: Component scaffold + RPC client wiring (~5 min)
- Mirror `GlobalDefaultsForm.tsx`'s `createClient(SessionService, transport)` pattern
  (`web-app/src/components/settings/GlobalDefaultsForm.tsx:4-76`) — `useEffect` to call
  `getSlackConfig` on mount, local `useState` for form fields. **Load-failure state
  (`design/ux.md`'s error/edge-case table, "never a silently blank/frozen panel"):** if the
  mount-time `getSlackConfig` call rejects, render a `role="alert"` region in place of the form
  reading `"Couldn't load Slack settings."` with a "Retry" button that re-fires the same call —
  this replaces the form entirely rather than rendering it against empty/default data.
- Files: `web-app/src/components/settings/SlackNotificationSettings.tsx`

##### Task 1.4.3b: Webhook URL field + masking + remove (~5 min)
- `<label htmlFor="slack-webhook-url">` + `<input id="slack-webhook-url" type="text" aria-describedby="slack-webhook-hint slack-webhook-error" aria-invalid={...}>`,
  following `CronScheduleInput.tsx:170-189`'s validated-input template exactly. On-blur
  client-side shape check (UX-4): `/^https:\/\/hooks\.slack\.com\/services\//.test(value)`;
  when it fails on a non-empty value, set the `slack-webhook-error` region's text to the exact
  string from `design/ux.md`'s error table — `"This doesn't look like a Slack Incoming Webhook
  URL (expected https://hooks.slack.com/services/...)"` — and set `aria-invalid="true"`; clears
  live as soon as the regex passes again, no re-submit needed (same regex shape as Task 1.4.2b's
  server-side backstop, kept in sync by both citing this literal pattern). Placeholder
  reflects `webhook_configured` (Story 1.4.3 AC1); never pre-fills a real value (mirrors
  `BacklogSourcesSettings.tsx`'s token field not round-tripping the value). When
  `webhook_configured` is true, render a "Remove" button next to the masked field that calls
  `UpdateSlackConfig` with `clear_webhook_url: true` (Task 1.4.1a/1.4.2b) — the only way to
  reach the "cleared" state distinct from "field left blank, unchanged."
- Files: `web-app/src/components/settings/SlackNotificationSettings.tsx`

##### Task 1.4.3c: Toggles + queue-depth threshold input (~4 min)
- Two native `<input type="checkbox">` (per `PushNotificationSettings.tsx:54-63`'s toggle
  pattern) for `notify_on_queue_item` and (Phase 2, rendered but visibly marked "requires
  public reachability — see docs") `approval_enabled`; a number input for
  `queue_depth_threshold`, with the literal hint text `"(0 = off)"` next to the label plus
  UX-11's edge-triggered-digest explanation (`design/ux.md` UX-11 — "You'll get one digest per
  burst; a persistently full queue won't nudge you again until it drops below the threshold and
  re-crosses"), both required per `design/ux.md`'s wireframe and UX-11's acceptance criterion, not
  optional polish. Both notify-type checkboxes `disabled={!webhookConfigured}` (Story
  1.4.3 AC3).
- Files: `web-app/src/components/settings/SlackNotificationSettings.tsx`

##### Task 1.4.3f: `dashboard_base_url` field (~4 min) — resolves adversarial-review.md blocker #3
- `<label htmlFor="slack-dashboard-base-url">` + plain (non-secret, unmasked) text input for
  `dashboard_base_url`, same validated-input template as Task 1.4.3b minus the masking (this is
  not a secret). Hint text: "Used to build 'view in dashboard' links in Slack messages. Leave
  blank and links will only work on your home network." Per UX-9's option (b) (chosen over (a) —
  hard-requiring it before enabling toggles — to avoid blocking the Medium-appetite ship on a
  product decision with no wrong answer either way): when a notify toggle (Task 1.4.3c) is
  checked and `dashboard_base_url` is empty, render a persistent, dismissable `role="status"`
  warning: "Your Slack links may not work outside your home network — set a Dashboard URL below."
  This resolves plan Unresolved Question #1 (`DashboardBaseURL` validation) as: soft warning, not
  a hard gate.
- Files: `web-app/src/components/settings/SlackNotificationSettings.tsx`

##### Task 1.4.3d: "Send test message" button + status region (~5 min)
- Button with `submitting` state (mirrors `BacklogSourcesSettings.tsx:102`'s pattern) that
  calls `testSlackWebhook({webhookUrl: formState.webhookUrl})` — **passes the value currently
  typed in the form field (Task 1.4.2c's `webhook_url`), not a config re-fetch**, so editing the
  URL and immediately testing it works before any save happens (UX-1's "test before save" flow).
  If the field is untouched/empty (masked-configured state, nothing typed), `formState.webhookUrl`
  is `""` and Task 1.4.2c falls back to the persisted config. **Precondition-disabled, not
  clickable-then-erroring (UX-8):** the button is `disabled` (not just non-functional) whenever
  there is neither a valid-shaped typed URL nor an already-configured webhook — mirrors UX-8's
  existing "toggle disabled without precondition" pattern already required for the notify
  checkboxes, applied here to this control too, per `design/ux.md`'s unconfigured-state wireframe
  ("[ Send test message ] (disabled — no URL yet)"). Result rendered in a `role="alert"`
  (failure) or `role="status"` (success) region adjacent to the button, per `InlineNotice.tsx`'s
  two-tier convention. `data-testid="slack-test-webhook-result"` for the e2e test (Story 1.4.5).
- Files: `web-app/src/components/settings/SlackNotificationSettings.tsx`

##### Task 1.4.3e: Last-delivery status line + settings-page registration (~4 min)
- Passive `role="status"` line: `"Last Slack delivery: <relative time> — <success|failed:
  reason>"` sourced from `GetSlackConfig`'s `last_delivery`, rendered on every settings-page
  load (closes `research/ux.md` §4's "silently stops working" gap). Register the component in
  `web-app/src/app/settings/page.tsx` near `<PushNotificationSettings />` (line 126).
- Files: `web-app/src/components/settings/SlackNotificationSettings.tsx`, `web-app/src/app/settings/page.tsx`, `web-app/src/components/settings/SlackNotificationSettings.css.ts`

### Story 1.4.4: Feature registry entries
**As a** repo maintainer, **I want** the new RPCs and UI feature registered per
`.claude/rules/feature-registry.md`, **so that** `make registry-generate` doesn't flag a
coverage gap.
**Acceptance Criteria**:
- Three new backend entries and one frontend entry exist, `markerFound: true` for each.
  - *Given* `// +api: session:get-slack-config` (and the update/test equivalents) markers are
    added to the three `SlackConfigService` handler methods, *when* `make registry-generate`
    runs, *then* `docs/registry/features/backend/GetSlackConfig.json` (etc.) exist with
    `"markerFound": true`.
**Files**: `docs/registry/features/backend/GetSlackConfig.json` (new), `docs/registry/features/backend/UpdateSlackConfig.json` (new), `docs/registry/features/backend/TestSlackWebhook.json` (new), `docs/registry/features/frontend/slack-notification-settings.json` (new)

##### Task 1.4.4a: Add `// +api:` markers + backend registry files (~4 min)
- Add `// +api: session:get-slack-config` etc. above each of the three handler methods (Task
  1.4.2a-c); create the three per-feature JSON files matching the real field set used by
  existing entries (e.g. `docs/registry/features/backend/GetInsightsSummary.json`'s `id`,
  `type: "backend"`, `service: "SessionService"`, `method`, `protoFile:
  "proto/session/v1/session.proto"`, `markerFound: true`, `tested: false`, `testIds: []`,
  `lastModified` — `tested`/`testIds` flipped to real values in Epic 1.5).
- Files: `server/services/slack_config_service.go`, `docs/registry/features/backend/GetSlackConfig.json`, `docs/registry/features/backend/UpdateSlackConfig.json`, `docs/registry/features/backend/TestSlackWebhook.json`

##### Task 1.4.4b: Add `// +feature:` marker + frontend registry file (~3 min)
- Add `// +feature: slack-notification-settings` in the first 10 lines of
  `SlackNotificationSettings.tsx`; create `docs/registry/features/frontend/slack-notification-settings.json`.
- Files: `web-app/src/components/settings/SlackNotificationSettings.tsx`, `docs/registry/features/frontend/slack-notification-settings.json`

##### Task 1.4.4c: Regenerate + verify no coverage-gap growth (~3 min)
- Run `make registry-generate`; confirm `docs/registry/coverage-gaps.json`'s count does not
  increase (the four new entries start `tested: false`, which is expected pre-Epic-1.5 —
  verify this is the *only* source of any transient gap growth, not an unrelated regression).
- Files: `docs/registry/backend-features.json` (generated), `docs/registry/frontend-features.json` (generated), `docs/registry/coverage-gaps.json` (generated)

### Story 1.4.5: E2E test
**As a** CI pipeline, **I want** a Playwright test covering the settings flow, **so that** a
future regression in the form or RPC wiring is caught automatically.
**Acceptance Criteria**:
- The spec starts with the required `// @feature` annotation and uses only `data-testid`/ARIA
  locators (no CSS class selectors), per `.claude/rules/e2e-test-conventions.md`.
  - *Given* the test server is running (per `tests/e2e/global-setup.ts`), *when*
    `slack-notifications.spec.ts` navigates to `/settings`, enters a syntactically-valid test
    webhook URL, and clicks "Send test message," *then*
    `page.getByTestId("slack-test-webhook-result")` eventually shows either success or a
    Slack-error string (no `waitForTimeout` — uses `expect(...).toHaveText(...)` polling).
**Files**: `tests/e2e/slack-notifications.spec.ts` (new)

##### Task 1.4.5a: Write the e2e spec (~5 min)
- `// @feature slack:get-config, slack:update-config, slack:test-webhook`; fill the webhook URL
  field, toggle "notify on queue item," save, click "Send test message," assert the result
  region updates. Use `tests/e2e/pages/` helper if a `SettingsPage` page-object already exists,
  else inline locators (`page.getByRole`, `page.getByTestId`) per convention.
- Files: `tests/e2e/slack-notifications.spec.ts`

---

## Epic 1.5: Tests

**Goal**: Unit-test the notifier's formatting/truncation/non-blocking/coalescing behavior and
the settings component's key interactions — the four things `research/pitfalls.md` calls out
as most likely to regress silently.

### Story 1.5.1: `SlackNotifier` unit tests
**As a** developer, **I want** the notifier's core guarantees pinned by tests, **so that** a
future refactor can't silently reintroduce a blocking call, a leaked secret, or a burst of
messages.
**Acceptance Criteria**:
- Message formatting, truncation, non-blocking-on-failure, and threshold-latch behavior are
  each covered by at least one test (see individual Given-When-Then examples in Stories
  1.2.1-1.2.4 above — these tests are what proves those criteria, not new criteria).
**Files**: `server/services/slack_notifier_test.go` (new)

##### Task 1.5.1a: `TestNotifyReviewQueueItem_TruncatesOversizedDiff` (~5 min)
- Uses an `httptest.Server` capturing the POST body; asserts the captured JSON's block text
  length and truncation suffix per Story 1.2.2's first Given-When-Then.
- Files: `server/services/slack_notifier_test.go`

##### Task 1.5.1b: `TestNotifyApprovalPending_BuildsCorrectDashboardLink` (~4 min)
- Asserts the exact link format per Story 1.2.2's second Given-When-Then.
- Files: `server/services/slack_notifier_test.go`

##### Task 1.5.1c: `TestSlackNotifier_SendFailure_DoesNotBlockCaller` (~5 min)
- `httptest.Server` that sleeps 10s before responding (or a closed listener for immediate
  connection refusal); asserts `dispatchAsync`'s wrapped call returns to the caller (i.e. the
  test itself, standing in for `OnItemAdded`) in well under the 5s context timeout — proves the
  goroutine + context.WithTimeout shape from Task 1.2.3a actually bounds latency.
- Files: `server/services/slack_notifier_test.go`

##### Task 1.5.1d: `TestSlackNotifier_NeverLogsWebhookURL` (~4 min)
- Captures log output (via the repo's existing log-capture test helper, if one exists — else a
  package-level log sink swap) during a forced failure; greps captured output for the literal
  test webhook URL substring; asserts absence. Directly enforces `research/pitfalls.md` §1's
  top concrete failure mode.
- Files: `server/services/slack_notifier_test.go`

##### Task 1.5.1e: `TestMaybeNotifyQueueDepthThreshold_FiresOncePerCrossing` (~5 min)
- Feeds the depth sequence `4, 5, 6, 7, 4, 6` per Story 1.2.4's Given-When-Then; asserts exactly
  2 POSTs via a request-counting `httptest.Server`. Includes the concurrent variant named in
  Task 1.2.4a's concurrency note (N goroutines racing a shared crossing depth, `go test -race`,
  asserts exactly one digest fires).
- Files: `server/services/slack_notifier_test.go`

##### Task 1.5.1f: `resolveSlackWebhookURL`/`resolveSlackSigningSecret` precedence tests (~5 min)
- **Closes the dangling Story 1.1.3 cross-reference (flagged by adversarial-review.md and by
  validation.md's REQ-17, which had no corresponding Task ID until now):**
  `TestResolveSlackWebhookURL_EnvOverride_TakesPrecedenceOverDecryptedValue`,
  `TestResolveSlackWebhookURL_DecryptsStoredCiphertext_WhenNoOverride`,
  `TestResolveSlackWebhookURL_ReturnsEmptyString_When_NeitherOverrideNorCiphertextSet` — exercise
  all three branches of Task 1.2.1b at the `server/services` level (the `config`-package-level
  override-getter test in Task 1.1.3a only covers the getter itself, not the decrypt path).
- Files: `server/services/slack_notifier_test.go`

### Story 1.5.2: Settings UI component test
**As a** developer, **I want** a Jest/RTL test for the masking and disabled-toggle behavior,
**so that** the two most security/trust-relevant UI behaviors are pinned outside of e2e.
**Acceptance Criteria**:
- Masking and disabled-without-webhook behavior are both covered (Story 1.4.3's AC1 and AC3).
**Files**: `web-app/src/components/settings/SlackNotificationSettings.test.tsx` (new)

##### Task 1.5.2a: `SlackNotificationSettings_should_MaskWebhookField_When_AlreadyConfigured` (~4 min)
- Mocks `GetSlackConfig` returning `webhook_configured: true`; asserts the input shows no real
  URL value.
- Files: `web-app/src/components/settings/SlackNotificationSettings.test.tsx`

##### Task 1.5.2b: `SlackNotificationSettings_should_DisableToggles_When_NoWebhookConfigured` (~4 min)
- Mocks `webhook_configured: false`; asserts both notify checkboxes render `disabled`.
- Files: `web-app/src/components/settings/SlackNotificationSettings.test.tsx`
- Run: `cd web-app && npx jest --no-coverage --testPathPatterns="SlackNotificationSettings.test"` to verify.

---

# Phase 2 (separately shippable — gated by `cfg.Slack.ApprovalEnabled`, default `false`)

## Epic 2.1: Inbound Slack Interactive Approvals

**Goal**: A Slack button click on an outbound approval message resolves the pending approval
via the same path the web dashboard's Approve/Deny buttons use — after a hard signature-
verification gate. Entirely inert (route not even registered) unless `ApprovalEnabled` is true.

### Story 2.1.1: Signature verification
**As a** the inbound endpoint, **I want** to reject anything not genuinely signed by Slack
before touching any store, **so that** this new HTTP surface can't be used to forge
approve/deny decisions (the hard requirement from `requirements.md`'s Constraints).
**Acceptance Criteria**:
- Valid signature, correct timestamp → accepted.
  - *Given* a raw body `b`, a signing secret `s`, and a timestamp `t` within the last 5
    minutes, and a signature computed exactly as `"v0=" + hex(hmac_sha256(s, "v0:"+t+":"+b))`,
    *when* `verifySlackSignature(s, headers{X-Slack-Request-Timestamp: t, X-Slack-Signature: sig}, b)`
    is called, *then* it returns `nil`.
- Tampered body → rejected.
  - *Given* the same inputs as above but `b` is modified by one byte after the signature was
    computed, *when* `verifySlackSignature` is called with the *original* signature header,
    *then* it returns a non-nil error.
- Stale timestamp → rejected even with a correct signature.
  - *Given* `t` is `time.Now().Add(-10 * time.Minute)` and the signature is computed correctly
    against that stale `t`, *when* `verifySlackSignature` is called, *then* it returns an error
    mentioning replay/staleness, not a signature-mismatch error (distinct failure reasons for
    debuggability).
- Wrong secret → rejected.
  - *Given* the signature was computed with secret `s1` but verification is attempted with
    `s2 != s1`, *when* `verifySlackSignature` is called, *then* it returns a non-nil error.
**Files**: `server/services/slack_signature.go` (new)

##### Task 2.1.1a: `verifySlackSignature` implementation (~5 min)
- Per `research/stack.md`'s reference shape: parse `X-Slack-Request-Timestamp`, reject if
  `abs(time.Now().Unix() - ts) > 300`, build `"v0:" + ts + ":" + string(rawBody)`, HMAC-SHA256
  with the secret, hex-encode with `"v0="` prefix, compare via `hmac.Equal` (never `==`/
  `bytes.Equal`).
- Files: `server/services/slack_signature.go`

##### Task 2.1.1b: Fixed-vector unit tests (~5 min)
- One test per Given-When-Then above, using a fixed secret/timestamp/body/expected-signature
  quadruple computed once and hardcoded (not regenerated at test time) so a broken
  implementation can't accidentally self-verify.
- Files: `server/services/slack_signature_test.go` (new)

### Story 2.1.2: Interactive payload parsing + handler
**As a** the inbound endpoint, **I want** to correctly parse Slack's form-encoded interactive
payload and resolve the referenced approval, **so that** a verified button click actually
unblocks the waiting agent.
**Acceptance Criteria**:
- The raw body is captured and verified *before* any form/JSON parsing consumes it.
  - *Given* a request with `Content-Type: application/x-www-form-urlencoded` and a `payload`
    field containing valid interactive-component JSON, *when* the handler processes it, *then*
    `io.ReadAll(r.Body)` is called exactly once (into a buffer reused for both signature
    verification and `r.ParseForm`), never `r.ParseForm()` first (research/pitfalls.md §5's
    named bug class).
- A verified click with `action_id="approve"` and `value="appr-123:allow"` resolves that
  approval.
  - *Given* `ApprovalStore` has a pending approval with `ID: "appr-123"`, *when* the handler
    receives a verified payload whose action `value == "appr-123:allow"`, *then*
    `ApprovalService.ResolveApproval` is called with `id: "appr-123", decision: "allow"`.
**Files**: `server/services/slack_interactive_handler.go` (new)

##### Task 2.1.2a: `SlackInteractivePayload` struct + parsing (~4 min)
- Minimal struct for the fields needed: `Actions []struct{ ActionID, Value string }`. Parse via
  `r.FormValue("payload")` (after `r.ParseForm()`, which is only called *after* the raw-body
  signature check per Story 2.1.2's first AC) + `json.Unmarshal`.
- Files: `server/services/slack_interactive_handler.go`

##### Task 2.1.2b: `SlackInteractiveHandler.Handle` (~5 min)
- `func (h *SlackInteractiveHandler) Handle(w http.ResponseWriter, r *http.Request)`: read raw
  body, call `verifySlackSignature` (reject with 401 + generic body on failure — no verbose
  internals leaked to an internet-facing caller per `research/pitfalls.md` §5 item 7), parse
  form + payload, split `value` on `":"` into `approvalID, decision`, map `"allow"`/`"deny"` to
  the values `ApprovalService.ResolveApproval` expects, call it in-process, respond `200 OK`.
- Files: `server/services/slack_interactive_handler.go`

### Story 2.1.3: Route registration + wiring
**As a** the server, **I want** the route registered only when Phase 2 is enabled, **so that**
an unconfigured instance exposes zero additional attack surface.
**Acceptance Criteria**:
- Route is absent unless `cfg.Slack.ApprovalEnabled`.
  - *Given* `cfg.Slack.ApprovalEnabled == false` at server startup, *when* a request is sent to
    `/api/hooks/slack-interactive`, *then* it 404s (the standard `http.ServeMux` "not
    registered" response, not a custom handler response) because `srv.mux.HandleFunc` was never
    called for that path.
**Files**: `server/server.go`

##### Task 2.1.3a: Conditional route registration (~4 min)
- Near the existing hook/external-approval registrations (`server/server.go:462`,
  `511`): `if cfg.Slack.ApprovalEnabled { srv.mux.HandleFunc("/api/hooks/slack-interactive", slackInteractiveHandler.Handle) }`,
  constructing `slackInteractiveHandler` with `deps.SessionService`'s `ApprovalService` (or
  equivalent accessor) and `resolveSlackSigningSecret`-resolved secret.
- Files: `server/server.go`

##### Task 2.1.3b: Integration test — end-to-end through the real mux (~5 min)
- Spin up the server with `ApprovalEnabled: true`, POST a validly-signed payload, assert the
  targeted `PendingApproval` is resolved; POST the same payload with `ApprovalEnabled: false`
  at a fresh server instance, assert 404.
- Files: `server/slack_interactive_handler_integration_test.go` (new) or co-located with existing server integration tests — exact location TBD by whichever existing integration-test file (if any) already stands up a real `*http.ServeMux` server for `/api/hooks/*`.

### Story 2.1.4: Outbound Block Kit actions block (buttons)
**As a** user, **I want** the Slack message itself to carry Approve/Deny buttons when Phase 2
is enabled, **so that** I don't have to open the dashboard to resolve simple approvals.
**Acceptance Criteria**:
- When `ApprovalEnabled`, `NotifyApprovalPending`'s payload includes an `actions` block with
  two buttons whose `value` encodes `"<approvalID>:allow"` / `"<approvalID>:deny"`.
  - *Given* `cfg.Slack.ApprovalEnabled == true` and `approval.ID == "appr-9"`, *when*
    `NotifyApprovalPending` builds the payload, *then* one block has `type: "actions"`
    containing two button elements with `value` fields `"appr-9:allow"` and `"appr-9:deny"`.
- When `!ApprovalEnabled`, no actions block is added (Phase 1 behavior unchanged).
**Files**: `server/services/slack_notifier.go`

##### Task 2.1.4a: Extend `NotifyApprovalPending` with a conditional actions block (~4 min)
- Add `slackActionsBlock`/`slackButtonElement` structs; append to `payload.Blocks` only when
  `cfg.Slack.ApprovalEnabled`.
- Files: `server/services/slack_notifier.go`

##### Task 2.1.4b: Unit test for the conditional block (~3 min)
- `TestNotifyApprovalPending_IncludesActionsBlock_When_ApprovalEnabled` and the negative case.
- Files: `server/services/slack_notifier_test.go`

### Story 2.1.5: Public-reachability documentation
**As a** user considering enabling Phase 2, **I want** a written doc covering exactly how to
expose `/api/hooks/slack-interactive` to Slack's servers, **so that** I don't default to a
quick-start ngrok/reverse-proxy setup that tunnels the entire app.
**Acceptance Criteria** (closes requirements.md Phase 2 In-Scope's "Documentation covering the
public-reachability requirement" — flagged as an uncovered requirement by the sdd:4-validate
cross-artifact consistency pass — and pre-mortem.md Failure #4, P1):
- A doc exists describing the public-reachability requirement and, explicitly, how to scope any
  tunnel/reverse-proxy to the single path `/api/hooks/slack-interactive` rather than the whole
  app — with a concrete example (e.g. an ngrok path-restricted config or a reverse-proxy
  `location` block for just that path), not just "you need public reachability" in the abstract.
  - *Given* a user follows the doc to enable `ApprovalEnabled`, *when* they complete the steps
    as written, *then* only `/api/hooks/slack-interactive` is internet-reachable — not the
    session list, diffs, or any other `/api/hooks/*` route.
**Files**: `.claude/docs/slack-phase2-public-reachability.md` (new, following this repo's existing
`.claude/docs/*.md` convention for feature-specific operational docs)

##### Task 2.1.5a: Write the public-reachability doc (~5 min)
- Cover: why public reachability is required (Slack calls this endpoint from Slack's servers,
  not from the user's LAN), the security implication (this is a new authenticated-by-signature
  but internet-facing surface), and a worked example scoping an ngrok tunnel or reverse-proxy
  rule to exactly `/api/hooks/slack-interactive` — explicitly warning against tunneling the
  whole `:8543` port, since default ngrok/reverse-proxy quick-starts do exactly that and would
  expose the rest of the unauthenticated `/api/hooks/*` surface and the dashboard itself.
- Files: `.claude/docs/slack-phase2-public-reachability.md`
- Add a reference-index row for this doc (mirrors every other `.claude/docs/*.md` entry) in
  `CLAUDE.md`'s Reference Documents Index table.
